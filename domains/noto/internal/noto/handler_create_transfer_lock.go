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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/signpayloads"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type createTransferLockHandler struct {
	unlockCommon
}

func (h *createTransferLockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	if config.IsV0() {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, "createTransferLock is not supported in Noto V0")
	}

	var createTransferLockParams types.CreateTransferLockParams
	err := json.Unmarshal([]byte(params), &createTransferLockParams)
	if len(createTransferLockParams.From) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "from")
	}
	if len(createTransferLockParams.Recipients) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "recipients")
	}
	for _, entry := range createTransferLockParams.Recipients {
		if entry.Amount == nil || entry.Amount.Int().Sign() != 1 {
			return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "recipient amount")
		}
	}
	return &createTransferLockParams, err
}

func (h *createTransferLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.CreateTransferLockParams)
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx, params.From); err != nil {
		return nil, err
	}

	lookups := []string{notary, tx.Transaction.From, params.From}
	for _, entry := range params.Recipients {
		lookups = append(lookups, entry.To)
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(lookups...),
	}, nil
}

func (h *createTransferLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.CreateTransferLockParams)
	notary := tx.DomainConfig.NotaryLookup
	spendTxId := pldtypes.Bytes32UUIDFirst16(uuid.New())

	notaryID, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// Work out the amount we need
	requiredTotal := big.NewInt(0)
	for _, entry := range params.Recipients {
		requiredTotal = requiredTotal.Add(requiredTotal, entry.Amount.Int())
	}

	// Prepare the input coins
	inputStates, revert, err := h.noto.prepareInputs(ctx, req.StateQueryContext, senderID, (*pldtypes.HexUint256)(requiredTotal))
	if err != nil {
		if revert {
			message := err.Error()
			return &prototk.AssembleTransactionResponse{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   &message,
			}, nil
		}
		return nil, err
	}
	remainder := new(big.Int).Sub(inputStates.total, requiredTotal)

	// Pre-compute the lockId as it will be generated on the smart contract
	lockID, err := h.noto.computeLockIDForLockTX(ctx, tx, notaryID)
	if err != nil {
		return nil, err
	}

	// Create the locked output states
	lockedOutputStates, err := h.noto.prepareLockedOutputs(lockID, senderID, (*pldtypes.HexUint256)(requiredTotal), identityList{notaryID, senderID, fromID})
	if err != nil {
		return nil, err
	}

	// Build the outputs for unlock
	spendOutputs, err := h.assembleUnlockOutputs_V1(ctx, tx, notaryID, fromID, params.Recipients, req.ResolvedVerifiers, remainder)
	if err != nil {
		return nil, err
	}

	// If we have a remainder, that output gets unlocked immediately
	var remainderOutputs preparedOutputs
	if remainder.Sign() > 0 {
		spendOutputs.distributions = spendOutputs.distributions[0 : len(spendOutputs.distributions)-1]
		spendOutputs.states = spendOutputs.states[0 : len(spendOutputs.states)-1]
		spendOutputs.coins = spendOutputs.coins[0 : len(spendOutputs.coins)-1]
		remainderOutputs.distributions = []identityList{spendOutputs.distributions[len(spendOutputs.distributions)-1]}
		remainderOutputs.states = []*prototk.NewState{spendOutputs.states[len(spendOutputs.states)-1]}
		remainderOutputs.coins = []*types.NotoCoin{spendOutputs.coins[len(spendOutputs.coins)-1]}
	}

	// Build the info for the initiating transaction
	infoDistribution := identityList{notaryID, senderID, fromID}
	infoStates, err := h.noto.prepareDataInfo(params.Data, tx.DomainConfig.Variant, infoDistribution.identities())
	if err != nil {
		return nil, err
	}

	// We build the cancel outputs
	cancelOutputs, err := h.noto.prepareOutputs(fromID, (*pldtypes.HexUint256)(requiredTotal), identityList{notaryID, senderID, fromID})
	// ... and allocate ids to all the new outputs, so we can build the transaction we need to hash
	if err == nil {
		err = h.noto.allocateStateIDs(ctx, req.StateQueryContext, spendOutputs.states, cancelOutputs.states)
	}
	// ... and the txData that would be emitted for either cancel or spend
	var txData pldtypes.HexBytes
	if err == nil {
		txData, err = h.noto.encodeTransactionDataV1(ctx, []*prototk.EndorsableState{})
	}
	// ... and the new lock state as an output
	var lock *preparedLockInfo
	if err == nil {
		lock, err = h.noto.prepareLockInfo_V1(&types.NotoLockInfo_V1{
			Salt:          pldtypes.RandBytes32(),
			LockID:        lockID,
			SpendOutputs:  newStateAllocatedIDs(spendOutputs.states),
			SpendData:     txData,
			CancelOutputs: newStateAllocatedIDs(cancelOutputs.states),
			CancelData:    txData,
			SpendTxId:     spendTxId,
		}, identityList{notaryID, senderID, fromID})
	}
	// .. and then the manifest
	var manifestState *prototk.NewState
	if err == nil {
		manifestState, err = h.noto.newManifestBuilder().
			addOutputs(spendOutputs).
			addOutputs(cancelOutputs).
			addOutputs(&remainderOutputs).
			addLockInfo(lock).
			buildManifest(ctx, req.StateQueryContext)
	}
	if err != nil {
		return nil, err
	}

	// The sender signs the spending of the locked outputs to the target account
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, lockedOutputStates.coins, nil, spendOutputs.coins)
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
	assembly.InfoStates = append(assembly.InfoStates, spendOutputs.states...)
	assembly.InfoStates = append(assembly.InfoStates, cancelOutputs.states...)

	return &prototk.AssembleTransactionResponse{
		AssemblyResult:       prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: assembly,
		AttestationPlan: []*prototk.AttestationRequest{
			// Sender confirms the initial request with a signature
			{
				Name:            "sender",
				AttestationType: prototk.AttestationType_SIGN,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				Payload:         encodedUnlock,
				PayloadType:     signpayloads.OPAQUE_TO_RSV,
				Parties:         []string{req.Transaction.From},
			},
			// Notary will endorse the assembled transaction (by submitting to the ledger)
			{
				Name:            "notary",
				AttestationType: prototk.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				Parties:         []string{notary},
			},
		},
	}, nil
}

func (h *createTransferLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.CreateTransferLockParams)

	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	_, spendOutputs, cancelOutputs, err := h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_CREATE, fromID, nil, req.Inputs, req.Outputs, req.Info)
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
	if outputs.lockedTotal.Cmp(parsedSpendOutputs.total) != 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidAmount, "spendOutputs", inputs.total, parsedSpendOutputs.total)
	}
	if outputs.lockedTotal.Cmp(parsedCancelOutputs.total) != 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidAmount, "cancelOutputs", inputs.total, parsedCancelOutputs.total)
	}
	if err := h.noto.validateOwners(ctx, fromID.identifier, req.ResolvedVerifiers, inputs.coins, inputs.states); err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	encodedTransfer, err := h.noto.encodeTransferUnmasked(ctx, tx.ContractAddress, inputs.coins, parsedSpendOutputs.coins)
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

func (h *createTransferLockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (_ *TransactionWrapper, err error) {
	inParams := tx.Params.(*types.CreateTransferLockParams)
	inputCoinStates := h.noto.filterSchema(req.InputStates, []string{h.noto.coinSchema.Id})
	lockedCoinStates := h.noto.filterSchema(req.OutputStates, []string{h.noto.lockedCoinSchema.Id})
	remainderCoinStates := h.noto.filterSchema(req.OutputStates, []string{h.noto.coinSchema.Id})

	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", inParams.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	lockTransition, spendOutputs, cancelOutputs, err := h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_CREATE, fromID, nil, req.InputStates, req.OutputStates, req.InfoStates)
	if err != nil {
		return nil, err
	}

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	sender := domain.FindAttestation("sender", req.AttestationResult)
	if sender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	var interfaceABI abi.ABI
	var functionName string
	var paramsJSON []byte

	var lockParams *CreateLockParams
	lockParams, err = h.buildCreateLockParams(ctx,
		tx,
		lockTransition,
		sender.Payload,
		inputCoinStates,
		lockedCoinStates,
		remainderCoinStates,
		spendOutputs,
		cancelOutputs,
		req.InfoStates,
	)
	if err == nil {
		interfaceABI = h.noto.getInterfaceABI(types.NotoVariantDefault)
		functionName = "createLock"
		params := lockParams
		paramsJSON, err = json.Marshal(params)
	}
	if err != nil {
		return nil, err
	}

	return &TransactionWrapper{
		functionABI: interfaceABI.Functions()[functionName],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *createTransferLockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)

	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	recipients := make([]*ResolvedUnlockRecipient, len(inParams.Recipients))
	for i, entry := range inParams.Recipients {
		toID, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
		recipients[i] = &ResolvedUnlockRecipient{To: toID.address, Amount: entry.Amount}
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &UnlockHookParams{
		Sender:     fromID.address,
		LockID:     inParams.LockID,
		Recipients: recipients,
		Data:       inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onCreateTransferLock"],
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

func (h *createTransferLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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
