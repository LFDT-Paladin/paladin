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

type createMintLockHandler struct {
	unlockCommon
}

func (h *createMintLockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	if config.IsV0() {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, "createMintLock is not supported in Noto V0")
	}

	var createMintLockParams types.CreateMintLockParams
	err := json.Unmarshal([]byte(params), &createMintLockParams)
	if len(createMintLockParams.Recipients) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "recipients")
	}
	for _, entry := range createMintLockParams.Recipients {
		if entry.Amount == nil || entry.Amount.Int().Sign() != 1 {
			return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "recipient amount")
		}
	}
	return &createMintLockParams, err
}

func (h *createMintLockHandler) checkAllowed(ctx context.Context, tx *types.ParsedTransaction, from string) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if !*tx.DomainConfig.Options.Basic.RestrictMint {
		return nil
	}
	if from == tx.DomainConfig.NotaryLookup {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgMintOnlyNotary, tx.DomainConfig.NotaryLookup, from)
}

func (h *createMintLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.CreateMintLockParams)
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx, req.Transaction.From); err != nil {
		return nil, err
	}

	lookups := []string{notary, tx.Transaction.From}
	for _, entry := range params.Recipients {
		lookups = append(lookups, entry.To)
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(lookups...),
	}, nil
}

func (h *createMintLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.CreateMintLockParams)
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

	// Pre-compute the lockId as it will be generated on the smart contract
	lockID, err := h.noto.computeLockIDForLockTX(ctx, tx, notaryID)
	if err != nil {
		return nil, err
	}

	// Build the outputs for unlock
	spendOutputs, err := h.assembleUnlockOutputs_V1(ctx, tx, notaryID, nil, params.Recipients, req.ResolvedVerifiers, big.NewInt(0))
	if err != nil {
		return nil, err
	}

	// Build the info for the initiating transaction
	infoDistribution := identityList{notaryID, senderID}
	infoStates, err := h.noto.prepareDataInfo(params.Data, tx.DomainConfig.Variant, infoDistribution.identities())
	if err != nil {
		return nil, err
	}

	// Create the txData that would be emitted for either cancel or spend
	err = h.noto.allocateStateIDs(ctx, req.StateQueryContext, spendOutputs.states)
	var txData pldtypes.HexBytes
	if err == nil {
		txData, err = h.noto.encodeTransactionDataV1(ctx, []*prototk.EndorsableState{}, false)
	}
	// ... and the new lock state as an output
	var lock *preparedLockInfo
	if err == nil {
		lock, err = h.noto.prepareLockInfo_V1(&types.NotoLockInfo_V1{
			Salt:          pldtypes.RandBytes32(),
			LockID:        lockID,
			Owner:         senderID.address,
			Spender:       senderID.address,
			SpendOutputs:  newStateAllocatedIDs(spendOutputs.states),
			SpendData:     txData,
			CancelOutputs: []pldtypes.Bytes32{ /* nothing to return */ },
			CancelData:    txData,
			SpendTxId:     spendTxId,
		}, identityList{notaryID, senderID})
	}
	// .. and then the manifest
	var manifestState *prototk.NewState
	if err == nil {
		manifestState, err = h.noto.newManifestBuilder().
			addOutputs(spendOutputs).
			addLockInfo(lock).
			addInfoStates(infoDistribution, infoStates...).
			buildManifest(ctx, req.StateQueryContext)
	}
	if err != nil {
		return nil, err
	}

	// The sender signs the spending of the locked outputs to the target account
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, nil, nil, spendOutputs.coins)
	if err != nil {
		return nil, err
	}

	// Create the assembly with the full set of stats
	assembly := &prototk.AssembledTransaction{}
	assembly.OutputStates = []*prototk.NewState{lock.state}
	assembly.InfoStates = append([]*prototk.NewState{manifestState} /* manifest first */, infoStates...)
	assembly.InfoStates = append(assembly.InfoStates, spendOutputs.states...)

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

func (h *createMintLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.CreateMintLockParams)
	if err := h.checkAllowed(ctx, tx, req.Transaction.From); err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	_, spendOutputs, cancelOutputs, err := h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_CREATE, nil, nil, req.Inputs, req.Outputs, req.Info)
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

	// Work out the amount we need
	requiredTotal := big.NewInt(0)
	for _, entry := range params.Recipients {
		requiredTotal = requiredTotal.Add(requiredTotal, entry.Amount.Int())
	}
	if len(inputs.coins) > 0 || len(outputs.lockedCoins) > 0 || len(outputs.coins) > 0 || len(parsedCancelOutputs.coins) > 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidInputs, "mint", inputs.coins)
	}
	if requiredTotal.Cmp(parsedSpendOutputs.total) != 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidAmount, "mint", requiredTotal.Text(10), parsedSpendOutputs.total.Text(10))
	}

	// Notary checks the signature from the sender, then submits the transaction
	encodedTransfer, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, nil, nil, parsedSpendOutputs.coins)
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

func (h *createMintLockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (_ *TransactionWrapper, err error) {

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	lockTransition, spendOutputs, _, err := h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_CREATE, nil, nil, req.InputStates, req.OutputStates, req.InfoStates)
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
		[]*prototk.EndorsableState{},
		[]*prototk.EndorsableState{},
		[]*prototk.EndorsableState{},
		spendOutputs,
		[]*prototk.EndorsableState{},
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

func (h *createMintLockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.CreateMintLockParams)

	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	lockTransition, err := h.noto.validateV1LockTransition(ctx, LOCK_CREATE, fromID, nil, req.InputStates, req.OutputStates)
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
	params := &CreateMintLockHookParams{
		Sender:     fromID.address,
		LockID:     lockTransition.newLockInfo.LockID,
		Recipients: recipients,
		Data:       inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onCreateMintLock"],
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

func (h *createMintLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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
