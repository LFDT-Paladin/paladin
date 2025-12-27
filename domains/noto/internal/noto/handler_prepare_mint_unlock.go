/*
 * Copyright © 2025 Kaleido, Inc.
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
)

type prepareMintUnlockHandler struct {
	unlockCommon
}

func (h *prepareMintUnlockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	if config.IsV0() {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, "prepareMintUnlock is not supported in Noto V0")
	}

	var mintLockParams types.PrepareMintUnlockParams
	if err := json.Unmarshal([]byte(params), &mintLockParams); err != nil {
		return nil, err
	}
	if len(mintLockParams.Recipients) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "recipients")
	}
	for _, entry := range mintLockParams.Recipients {
		if entry.Amount == nil || entry.Amount.Int().Sign() != 1 {
			return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "recipient amount")
		}
	}
	return &mintLockParams, nil
}

func (h *prepareMintUnlockHandler) checkAllowed(ctx context.Context, tx *types.ParsedTransaction, from string) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if *tx.DomainConfig.Options.Basic.RestrictMint && from != tx.DomainConfig.NotaryLookup {
		return i18n.NewError(ctx, msgs.MsgMintOnlyNotary, tx.DomainConfig.NotaryLookup, from)
	}
	if !*tx.DomainConfig.Options.Basic.AllowLock {
		return i18n.NewError(ctx, msgs.MsgLockNotAllowed)
	}
	return nil
}

func (h *prepareMintUnlockHandler) checkAllowedForFrom(ctx context.Context, tx *types.ParsedTransaction, from string) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}

	localNodeName, _ := h.noto.Callbacks.LocalNodeName(ctx, &prototk.LocalNodeNameRequest{})
	fromQualified, err := pldtypes.PrivateIdentityLocator(from).FullyQualified(ctx, localNodeName.Name)
	if err != nil {
		return err
	}
	if tx.Transaction.From == fromQualified.String() {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgUnlockOnlyCreator, tx.Transaction.From, from)
}

func (h *prepareMintUnlockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.PrepareMintUnlockParams)
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

func (h *prepareMintUnlockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.PrepareMintUnlockParams)
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

	// Load the existing lock
	existingLock, err := h.noto.loadLockInfoV1(ctx, req.StateQueryContext, params.LockID)
	if err != nil {
		return nil, err
	}

	// Build the data info for the parent transaction
	infoDistribution := identityList{notaryID, senderID}
	infoStates, err := h.noto.prepareDataInfo(params.Data, tx.DomainConfig.Variant, infoDistribution.identities())
	if err != nil {
		return nil, err
	}

	// Prepare the outputs to mint
	outputs := &preparedOutputs{}
	for _, entry := range params.Recipients {
		toID, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
		recipientOutputs, err := h.noto.prepareOutputs(toID, entry.Amount, identityList{notaryID, toID})
		if err != nil {
			return nil, err
		}
		outputs.distributions = append(outputs.distributions, recipientOutputs.distributions...)
		outputs.coins = append(outputs.coins, recipientOutputs.coins...)
		outputs.states = append(outputs.states, recipientOutputs.states...)
	}
	infoStates = append(infoStates, outputs.states...)

	// Build the data info for the prepared transaction
	txData, err := h.noto.encodeTransactionDataV1(ctx, []*prototk.EndorsableState{})
	if err != nil {
		return nil, err
	}

	err = h.noto.allocateStateIDs(ctx, req.StateQueryContext, outputs.states)
	if err != nil {
		return nil, err
	}

	// Build the prepared lock
	newLockInfo := *existingLock.lockInfo
	newLockInfo.Replaces = existingLock.id
	newLockInfo.Salt = pldtypes.RandBytes32()
	newLockInfo.SpendOutputs = newStateAllocatedIDs(outputs.states)
	newLockInfo.SpendData = txData
	newLockInfo.CancelOutputs = []pldtypes.Bytes32{} // no cancel outputs
	newLockInfo.CancelData = txData
	newLockInfo.SpendTxId = spendTxId
	lock, err := h.noto.prepareLockInfo_V1(&newLockInfo, identityList{notaryID, senderID})
	if err != nil {
		return nil, err
	}

	// Prepare unlock with no outputs (for burning)
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, nil, nil, outputs.coins)
	if err != nil {
		return nil, err
	}

	// Build the manifest
	manifestState, err := h.noto.newManifestBuilder().
		addOutputs(outputs).
		addInfoStates(infoDistribution, infoStates...).
		addLockInfo(lock).
		buildManifest(ctx, req.StateQueryContext)
	if err != nil {
		return nil, err
	}
	infoStates = append([]*prototk.NewState{manifestState} /* manifest first */, infoStates...)

	assembledTransaction := &prototk.AssembledTransaction{
		ReadStates:   nil,
		InfoStates:   infoStates,
		InputStates:  []*prototk.StateRef{existingLock.stateRef},
		OutputStates: []*prototk.NewState{lock.state},
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult:       prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: assembledTransaction,
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

func (h *prepareMintUnlockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.PrepareMintUnlockParams)

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	if err := h.checkAllowed(ctx, tx, req.Transaction.From); err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	_, spendOutputs, _, err := h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_UPDATE, senderID, &params.LockID, req.Inputs, req.Outputs, req.Info)
	if err != nil {
		return nil, err
	}

	parsedSpendOutputs, err := h.noto.parseCoinList(ctx, "output", spendOutputs)
	if err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	// No outputs - this will burn when unlocked
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, nil, nil, parsedSpendOutputs.coins)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateSignature(ctx, "sender", req.Signatures, encodedUnlock); err != nil {
		return nil, err
	}
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *prepareMintUnlockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	params := tx.Params.(*types.PrepareMintUnlockParams)

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	lockTransition, spendOutputs, _, err := h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_UPDATE, senderID, &params.LockID, req.InputStates, req.OutputStates, req.InfoStates)
	if err != nil {
		return nil, err
	}

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	sender := domain.FindAttestation("sender", req.AttestationResult)
	if sender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	interfaceABI := h.noto.getInterfaceABI(types.NotoVariantDefault)
	lockParams, err := h.buildPrepareUnlockParams(ctx, tx, lockTransition, sender.Payload, []*prototk.EndorsableState{}, spendOutputs, []*prototk.EndorsableState{}, req.InfoStates)
	var paramsJSON []byte
	if err == nil {
		paramsJSON, err = json.Marshal(lockParams)
	}
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: interfaceABI.Functions()["updateLock"],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *prepareMintUnlockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	params := tx.Params.(*types.PrepareMintUnlockParams)

	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	recipients := make([]*ResolvedUnlockRecipient, len(params.Recipients))
	for i, entry := range params.Recipients {
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
	hookParams := &PrepareMintUnlockHookParams{
		Sender:     fromID.address,
		LockId:     params.LockID,
		Recipients: recipients,
		Data:       params.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onPrepareMintUnlock"],
		hookParams,
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

func (h *prepareMintUnlockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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
