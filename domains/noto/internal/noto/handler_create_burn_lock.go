/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package noto

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type createBurnLockHandler struct {
	lockCommon
}

func (h *createBurnLockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, paramsJSON string) (interface{}, error) {
	if config.IsV0() {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, "createBurnLock is not supported in Noto V0")
	}

	var params types.CreateBurnLockParams
	err := json.Unmarshal([]byte(paramsJSON), &params)
	if err != nil {
		return nil, err
	}
	if params.Amount == nil || params.Amount.Int().Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}

	return &params, err
}

func (h *createBurnLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.CreateBurnLockParams)
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx); err != nil {
		return nil, err
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers([]string{notary, tx.Transaction.From, params.From}...),
	}, nil
}

func (h *createBurnLockHandler) checkAllowed(ctx context.Context, tx *types.ParsedTransaction) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if *tx.DomainConfig.Options.Basic.AllowBurn {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgBurnNotAllowed)
}

func (h *createBurnLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.CreateBurnLockParams)
	spendTxId := pldtypes.Bytes32UUIDFirst16(uuid.New())

	ids, err := resolveIdentities(ctx, h.noto, tx, req, params.From, "")
	if err != nil {
		return nil, err
	}
	notaryID, senderID, fromID := ids.notary, ids.sender, ids.from

	// Prepare the input coins
	inputStates, revert, err := h.noto.prepareInputs(ctx, req.StateQueryContext, senderID, (*pldtypes.HexUint256)(params.Amount))
	if res, err := assembleRevertOrError(revert, err); res != nil || err != nil {
		return res, err
	}
	remainder := new(big.Int).Sub(inputStates.total, (*big.Int)(params.Amount))

	// Pre-compute the lockId as it will be generated on the smart contract
	lockID, err := h.noto.computeLockIDForLockTX(ctx, tx, notaryID)
	if err != nil {
		return nil, err
	}

	// Create the locked output states
	lockedOutputStates, err := h.noto.prepareLockedOutputs(lockID, senderID, (*pldtypes.HexUint256)(params.Amount), identityList{notaryID, senderID, fromID})
	if err != nil {
		return nil, err
	}

	// Build the outputs for remainder
	remainderOutputs := &preparedOutputs{}
	if remainder.Sign() > 0 {
		remainderOutputs, err = h.noto.prepareOutputs(fromID, (*pldtypes.HexUint256)(remainder), identityList{notaryID, fromID})
		if err != nil {
			return nil, err
		}
	}

	// Build the cancel outputs before unlock data so they can be referenced in the cancel manifest
	cancelOutputs, err := h.noto.prepareOutputs(fromID, (*pldtypes.HexUint256)(params.Amount), identityList{notaryID, senderID, fromID})
	if err != nil {
		return nil, err
	}

	// Build and encode the unlock data (separate to the data for this TX)
	unlockInfo, err := h.buildUnlockInfo(ctx, tx, req.ResolvedVerifiers, req.StateQueryContext, &unlockInfoInput{
		resolvedIdentities: ids,
		unlockData:         params.UnlockData,
		cancelOutputs:      cancelOutputs,
		// no recipients, no spend outputs for burn
	})
	if err != nil {
		return nil, err
	}

	// Build the info for the initiating transaction
	infoStates := unlockInfo.infoStates
	createDataInfo, err := h.noto.prepareDataInfo(ctx, params.Data, tx.DomainConfig.Variant, unlockInfo.infoDistribution.identities(), tx.Transaction, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	infoStates = append(infoStates, createDataInfo...)

	// Build the new lock state as an output
	lock, err := h.noto.prepareLockInfo_V1(&types.NotoLockInfo_V1{
		Salt:          pldtypes.RandBytes32(),
		LockID:        lockID,
		Owner:         senderID.address,
		Spender:       senderID.address,
		SpendOutputs:  []pldtypes.Bytes32{ /* none for burn */ },
		SpendData:     unlockInfo.spendData,
		CancelOutputs: newStateAllocatedIDs(cancelOutputs.states),
		CancelData:    unlockInfo.cancelData,
		SpendTxId:     spendTxId,
	}, identityList{notaryID, senderID, fromID})
	if err != nil {
		return nil, err
	}

	// .. and then the manifest
	manifestState, err := h.noto.newManifestBuilder().
		addLockedOutputs(lockedOutputStates).
		addOutputs(cancelOutputs).
		addOutputs(remainderOutputs).
		addLockInfo(lock).
		addInfoStates(unlockInfo.infoDistribution, infoStates...).
		buildManifest(ctx, req.StateQueryContext)
	if err != nil {
		return nil, err
	}

	// The sender signs the spending of the locked outputs to the target account
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, lockedOutputStates.coins, nil, nil)
	if err != nil {
		return nil, err
	}

	// Create the assembly with the full set of stats
	assembly := &prototk.AssembledTransaction{}
	assembly.InputStates = append(assembly.InputStates, inputStates.states...)
	assembly.OutputStates = []*prototk.NewState{lock.state}
	assembly.OutputStates = append(assembly.OutputStates, lockedOutputStates.states...)
	assembly.OutputStates = append(assembly.OutputStates, remainderOutputs.states...)
	assembly.InfoStates = append([]*prototk.NewState{manifestState} /* manifest first */, infoStates...)
	assembly.InfoStates = append(assembly.InfoStates, cancelOutputs.states...)

	return &prototk.AssembleTransactionResponse{
		AssemblyResult:       prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: assembly,
		AttestationPlan:      buildEndorsePlan(tx.DomainConfig.NotaryLookup, req.Transaction.From, encodedUnlock),
	}, nil
}

func (h *createBurnLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	if err := h.checkAllowed(ctx, tx); err != nil {
		return nil, err
	}

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	_, spendOutputs, cancelOutputs, err := h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_CREATE, senderID, nil, req.Inputs, req.Outputs, req.Info)
	if err != nil {
		return nil, err
	}

	inputs, err := h.noto.parseCoinList(ctx, "inputs", req.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, err := h.noto.parseCoinList(ctx, "outputs", req.Outputs)
	if err != nil {
		return nil, err
	}
	parsedSpendOutputs, err := h.noto.parseCoinList(ctx, "spendOutputs", spendOutputs)
	if err != nil {
		return nil, err
	}
	parsedCancelOutputs, err := h.noto.parseCoinList(ctx, "cancelOutputs", cancelOutputs)
	if err != nil {
		return nil, err
	}

	// Validate the amounts, and sender's ownership of the inputs
	totalOutputs := new(big.Int).Add(outputs.lockedTotal, outputs.total)
	if inputs.total.Cmp(totalOutputs) != 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidAmount, "totalOutputs", inputs.total, totalOutputs)
	}
	if parsedSpendOutputs.total.Sign() != 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidAmount, "spendOutputs", "0", parsedCancelOutputs.total)
	}
	if outputs.lockedTotal.Cmp(parsedCancelOutputs.total) != 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidAmount, "cancelOutputs", inputs.total, parsedCancelOutputs.total)
	}
	if err := h.noto.validateOwners(ctx, senderID.identifier, req.ResolvedVerifiers, inputs.coins, inputs.states); err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	encodedTransfer, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, outputs.lockedCoins, nil, parsedSpendOutputs.coins)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateSignature(ctx, "sender", req.Signatures, encodedTransfer); err != nil {
		return nil, err
	}
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *createBurnLockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (_ *TransactionWrapper, err error) {
	inputCoinStates := h.noto.filterSchema(req.InputStates, []string{h.noto.coinSchema.Id})
	lockedCoinStates := h.noto.filterSchema(req.OutputStates, []string{h.noto.lockedCoinSchema.Id})
	remainderCoinStates := h.noto.filterSchema(req.OutputStates, []string{h.noto.coinSchema.Id})

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	lockTransition, _, cancelOutputs, err := h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_CREATE, senderID, nil, req.InputStates, req.OutputStates, req.InfoStates)
	if err != nil {
		return nil, err
	}

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	sender := domain.FindAttestation("sender", req.AttestationResult)
	if sender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	interfaceABI := h.noto.getInterfaceABI(tx.DomainConfig.Variant)
	functionName := "createLock"
	paramsJSON, err := h.buildCreateLockParams(ctx,
		tx,
		lockTransition,
		sender.Payload,
		inputCoinStates,
		lockedCoinStates,
		remainderCoinStates,
		[]*prototk.EndorsableState{ /* none for burn */ },
		cancelOutputs,
		req.InfoStates,
	)
	if err != nil {
		return nil, err
	}

	return &TransactionWrapper{
		functionABI: interfaceABI.Functions()[functionName],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *createBurnLockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.CreateBurnLockParams)

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	lockTransition, err := h.noto.validateV1LockTransition(ctx, LOCK_CREATE, senderID, nil, req.InputStates, req.OutputStates)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &CreateBurnLockHookParams{
		Sender: senderID.address,
		LockID: lockTransition.newLockInfo.LockID,
		From:   senderID.address,
		Amount: inParams.Amount,
		Data:   inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksV1Build.ABI.Functions()["onCreateBurnLock"],
		params,
	)
	if err != nil {
		return nil, err
	}

	return &TransactionWrapper{
		transactionType: mapPrepareTransactionType(transactionType),
		functionABI:     functionABI,
		paramsJSON:      paramsJSON,
		contractAddress: tx.DomainConfig.Options.Hooks.PublicAddress,
	}, nil
}

func (h *createBurnLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	baseTransaction, err := h.baseLedgerInvoke(ctx, tx, req)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() {
		hookTransaction, err := h.hookInvoke(ctx, tx, req, baseTransaction)
		if err != nil {
			return nil, err
		}
		return hookTransaction.prepare()
	}

	return baseTransaction.prepare()
}
