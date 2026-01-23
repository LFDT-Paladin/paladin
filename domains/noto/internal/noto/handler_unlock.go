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
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type unlockCommon struct {
	noto *Noto
}

type unlockHandler struct {
	unlockCommon
}

type unlockStates struct {
	oldLock         *loadedLockInfo // V1 only
	lockedInputs    *preparedLockedInputs
	v0LockedOutputs *preparedLockedOutputs // V0 only
	outputs         *preparedOutputs
	info            []*prototk.NewState
}

func (h *unlockCommon) validateParams(ctx context.Context, unlockParams *types.UnlockParams) error {
	if unlockParams.LockID.IsZero() {
		return i18n.NewError(ctx, msgs.MsgParameterRequired, "lockId")
	}
	if len(unlockParams.From) == 0 {
		return i18n.NewError(ctx, msgs.MsgParameterRequired, "from")
	}
	if len(unlockParams.Recipients) == 0 {
		return i18n.NewError(ctx, msgs.MsgParameterRequired, "recipients")
	}
	for _, entry := range unlockParams.Recipients {
		if entry.Amount == nil || entry.Amount.Int().Sign() != 1 {
			return i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "recipient amount")
		}
	}
	return nil
}

func (h *unlockCommon) checkAllowed(ctx context.Context, tx *types.ParsedTransaction, from string) error {
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

func (h *unlockCommon) init(ctx context.Context, tx *types.ParsedTransaction, params *types.UnlockParams) (*prototk.InitTransactionResponse, error) {
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

func (h *unlockCommon) assembleStates(ctx context.Context, tx *types.ParsedTransaction, spendTxId *pldtypes.Bytes32, params *types.UnlockParams, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, *manifestBuilder, *unlockStates, error) {
	notary := tx.DomainConfig.NotaryLookup

	notaryID, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, nil, nil, err
	}
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, nil, nil, err
	}
	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, nil, nil, err
	}

	// Load the existing lock
	var existingLock *loadedLockInfo
	if !tx.DomainConfig.IsV0() {
		existingLock, err = h.noto.loadLockInfoV1(ctx, req.StateQueryContext, params.LockID)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	requiredTotal := big.NewInt(0)
	for _, entry := range params.Recipients {
		requiredTotal = requiredTotal.Add(requiredTotal, entry.Amount.Int())
	}

	lockedInputStates, revert, err := h.noto.prepareLockedInputs(ctx, req.StateQueryContext, params.LockID, fromID.address, requiredTotal, true)
	if err != nil {
		if revert {
			message := err.Error()
			return &prototk.AssembleTransactionResponse{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   &message,
			}, nil, nil, nil
		}
		return nil, nil, nil, err
	}

	remainder := big.NewInt(0).Sub(lockedInputStates.total, requiredTotal)

	var unlockedOutputs *preparedOutputs
	var v0LockedOutputs *preparedLockedOutputs
	if tx.DomainConfig.IsV0() {
		unlockedOutputs, v0LockedOutputs, err = h.assembleUnlockOutputs_V0(ctx, tx, params, req, fromID.address, remainder)
	} else {
		unlockedOutputs, err = h.assembleUnlockOutputs_V1(ctx, tx, notaryID, fromID, params.Recipients, req.ResolvedVerifiers, remainder)
	}
	if err != nil {
		return nil, nil, nil, err
	}

	infoDistribution := identityList{notaryID, senderID, fromID}
	infoStates, err := h.noto.prepareDataInfo(params.Data, tx.DomainConfig.Variant, infoDistribution.identities())
	if err != nil {
		return nil, nil, nil, err
	}
	if tx.DomainConfig.IsV0() {
		// In V0 unlock repeats the lock state in the info
		var lock *preparedLockInfo
		lock, err = h.noto.prepareLockInfo_V0(params.LockID, fromID.address, nil, infoDistribution)
		if err == nil {
			infoStates = append(infoStates, lock.state)
		}
	}
	if err != nil {
		return nil, nil, nil, err
	}

	mb := h.noto.newManifestBuilder().
		addOutputs(unlockedOutputs). // note no v0UnlockedOutputs as we're V1 only for the manifest
		addInfoStates(infoDistribution, infoStates...)

	return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
		},
		mb,
		&unlockStates{
			oldLock:         existingLock,
			lockedInputs:    lockedInputStates,
			v0LockedOutputs: v0LockedOutputs,
			outputs:         unlockedOutputs,
			info:            infoStates,
		},
		nil
}

// In V0 the remainder was returned locked
func (h *unlockCommon) assembleUnlockOutputs_V0(ctx context.Context, tx *types.ParsedTransaction, params *types.UnlockParams, req *prototk.AssembleTransactionRequest, from *pldtypes.EthAddress, remainder *big.Int) (*preparedOutputs, *preparedLockedOutputs, error) {
	notary := tx.DomainConfig.NotaryLookup

	notaryID, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, nil, err
	}
	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, nil, err
	}

	unlockedOutputs := &preparedOutputs{}
	for _, entry := range params.Recipients {
		toID, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, req.ResolvedVerifiers)
		if err != nil {
			return nil, nil, err
		}
		outputs, err := h.noto.prepareOutputs(toID, entry.Amount, identityList{notaryID, fromID, toID})
		if err != nil {
			return nil, nil, err
		}
		unlockedOutputs.distributions = append(unlockedOutputs.distributions, outputs.distributions...)
		unlockedOutputs.coins = append(unlockedOutputs.coins, outputs.coins...)
		unlockedOutputs.states = append(unlockedOutputs.states, outputs.states...)
	}

	// Remainder is returned to the lock owner as LOCKED outputs (V0 only)
	lockedOutputs := &preparedLockedOutputs{}
	if remainder.Cmp(big.NewInt(0)) == 1 {
		var err error
		lockedOutputs, err = h.noto.prepareLockedOutputs(params.LockID, fromID, (*pldtypes.HexUint256)(remainder), identityList{notaryID, fromID})
		if err != nil {
			return nil, nil, err
		}
	}

	return unlockedOutputs, lockedOutputs, nil
}

func (h *unlockCommon) assembleUnlockOutputs_V1(ctx context.Context, tx *types.ParsedTransaction, notaryID, fromID *identityPair, recipients []*types.UnlockRecipient, resolvedVerifiers []*prototk.ResolvedVerifier, remainder *big.Int) (*preparedOutputs, error) {
	unlockedOutputs := &preparedOutputs{}
	for _, entry := range recipients {
		toID, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, resolvedVerifiers)
		if err != nil {
			return nil, err
		}
		var distribution identityList
		if fromID != nil {
			distribution = identityList{notaryID, fromID, toID}
		} else {
			distribution = identityList{notaryID, toID}
		}
		outputs, err := h.noto.prepareOutputs(toID, entry.Amount, distribution)
		if err != nil {
			return nil, err
		}
		unlockedOutputs.distributions = append(unlockedOutputs.distributions, outputs.distributions...)
		unlockedOutputs.coins = append(unlockedOutputs.coins, outputs.coins...)
		unlockedOutputs.states = append(unlockedOutputs.states, outputs.states...)
	}

	// Remainder is returned to the lock owner as unlocked outputs
	if remainder.Cmp(big.NewInt(0)) == 1 {
		remainderOutputs, err := h.noto.prepareOutputs(fromID, (*pldtypes.HexUint256)(remainder), identityList{notaryID, fromID})
		if err != nil {
			return nil, err
		}
		unlockedOutputs.distributions = append(unlockedOutputs.distributions, remainderOutputs.distributions...)
		unlockedOutputs.coins = append(unlockedOutputs.coins, remainderOutputs.coins...)
		unlockedOutputs.states = append(unlockedOutputs.states, remainderOutputs.states...)
	}

	return unlockedOutputs, nil
}

func (h *unlockCommon) endorse(
	ctx context.Context,
	tx *types.ParsedTransaction,
	params *types.UnlockParams,
	req *prototk.EndorseTransactionRequest,
	inputs, spendOutputs, cancelOutputs *parsedCoins,
) (*prototk.EndorseTransactionResponse, error) {
	if err := h.checkAllowed(ctx, tx, params.From); err != nil {
		return nil, err
	}

	// Validate the amounts, and lock creator's ownership of all locked inputs/outputs
	if err := h.noto.validateUnlockAmounts(ctx, tx, inputs, spendOutputs); err != nil {
		return nil, err
	}
	if err := h.noto.validateLockOwners(ctx, params.From, req.ResolvedVerifiers, inputs.lockedCoins, inputs.lockedStates); err != nil {
		return nil, err
	}
	if err := h.noto.validateLockOwners(ctx, params.From, req.ResolvedVerifiers, spendOutputs.lockedCoins, spendOutputs.lockedStates); err != nil {
		return nil, err
	}

	// If cancel outputs are present (for prepare unlock), validate the amounts and owners
	if cancelOutputs != nil {
		if err := h.noto.validateUnlockAmounts(ctx, tx, inputs, cancelOutputs); err != nil {
			return nil, err
		}
		if err := h.noto.validateLockOwners(ctx, params.From, req.ResolvedVerifiers, cancelOutputs.lockedCoins, cancelOutputs.lockedStates); err != nil {
			return nil, err
		}
		if err := h.noto.validateOwners(ctx, params.From, req.ResolvedVerifiers, cancelOutputs.coins, cancelOutputs.states); err != nil {
			return nil, err
		}
	}

	// Notary checks the signatures from the sender, then submits the transaction
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, inputs.lockedCoins, spendOutputs.lockedCoins, spendOutputs.coins)
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

func (h *unlockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var unlockParams types.UnlockParams
	err := json.Unmarshal([]byte(params), &unlockParams)
	if err == nil {
		err = h.validateParams(ctx, &unlockParams)
	}
	return &unlockParams, err
}

func (h *unlockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	return h.init(ctx, tx, params)
}

func (h *unlockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	notary := tx.DomainConfig.NotaryLookup

	res, mb, states, err := h.assembleStates(ctx, tx, nil, params, req)
	if err != nil || res.AssemblyResult != prototk.AssembleTransactionResponse_OK {
		return res, err
	}

	assembledTransaction := &prototk.AssembledTransaction{}
	assembledTransaction.InputStates = states.lockedInputs.states
	assembledTransaction.OutputStates = states.outputs.states
	var v0LockedCoins []*types.NotoLockedCoin
	if tx.DomainConfig.IsV0() {
		v0LockedCoins = states.v0LockedOutputs.coins
		assembledTransaction.OutputStates = append(assembledTransaction.OutputStates, states.v0LockedOutputs.states...)
	} else {
		manifestState, err := mb.buildManifest(ctx, req.StateQueryContext)
		if err != nil {
			return nil, err
		}
		states.info = append([]*prototk.NewState{manifestState} /* manifest first */, states.info...)
		assembledTransaction.InputStates = append(assembledTransaction.InputStates, states.oldLock.stateRef)
	}
	assembledTransaction.InfoStates = states.info

	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, states.lockedInputs.coins, v0LockedCoins, states.outputs.coins)
	if err != nil {
		return nil, err
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

func (h *unlockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	inputs, err := h.noto.parseCoinList(ctx, "input", req.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, err := h.noto.parseCoinList(ctx, "output", req.Outputs)
	if err != nil {
		return nil, err
	}

	if !tx.DomainConfig.IsV0() {
		fromID, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}

		// In V1 onwards the lock itself needs to be checked (it's owned by the from address)
		_, err = h.noto.validateV1LockTransition(ctx, LOCK_SPEND, fromID, &params.LockID, req.Inputs, req.Outputs)
		if err != nil {
			return nil, err
		}
	}

	return h.endorse(ctx, tx, params, req, inputs, outputs, nil)
}

func (h *unlockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)

	lockedInputs := req.InputStates
	outputs, lockedOutputs := h.noto.splitStates(req.OutputStates)

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	unlockSignature := domain.FindAttestation("sender", req.AttestationResult)
	if unlockSignature == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	txData, err := h.noto.encodeTransactionData(ctx, tx.DomainConfig, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}

	var interfaceABI abi.ABI
	var functionName string
	var paramsJSON []byte

	switch tx.DomainConfig.Variant {
	case types.NotoVariantDefault:
		interfaceABI = h.noto.getInterfaceABI(types.NotoVariantDefault)
		functionName = "spendLock"
		var notoUnlockOpEncoded []byte
		notoUnlockOpEncoded, err = h.noto.encodeNotoUnlockOperation(ctx, inParams.LockID, &types.NotoUnlockOperation{
			TxId:    req.Transaction.TransactionId,
			Inputs:  endorsableStateIDs(lockedInputs, false),
			Outputs: endorsableStateIDs(outputs, false),
			Data:    txData,
			Proof:   unlockSignature.Payload,
		})
		if err == nil {
			params := &SpendLockParams{
				LockID:      inParams.LockID,
				SpendInputs: notoUnlockOpEncoded,
				Data:        []byte{}, // we don't need this outer data
			}
			paramsJSON, err = json.Marshal(params)
		}
	default:
		interfaceABI = h.noto.getInterfaceABI(types.NotoVariantLegacy)
		functionName = "unlock"
		params := &NotoUnlock_V0_Params{
			TxId:          req.Transaction.TransactionId,
			LockedInputs:  endorsableStateIDs(lockedInputs, false),
			LockedOutputs: endorsableStateIDs(lockedOutputs, false),
			Outputs:       endorsableStateIDs(outputs, false),
			Signature:     unlockSignature.Payload,
			Data:          txData,
		}
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

func (h *unlockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	unlock := make([]*ResolvedUnlockRecipient, len(inParams.Recipients))
	for i, entry := range inParams.Recipients {
		toID, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
		unlock[i] = &ResolvedUnlockRecipient{To: toID.address, Amount: entry.Amount}
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &UnlockHookParams{
		Sender:     senderID.address,
		LockID:     inParams.LockID,
		Recipients: unlock,
		Data:       inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onUnlock"],
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

func (h *unlockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	endorsement := domain.FindAttestation("notary", req.AttestationResult)
	if endorsement == nil || endorsement.Verifier.Lookup != tx.DomainConfig.NotaryLookup {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "notary")
	}

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

func (h *unlockCommon) buildPrepareUnlockParams(ctx context.Context, tx *types.ParsedTransaction, lockTransition *lockTransition, proof pldtypes.HexBytes, lockedInputs, spendOutputs, cancelOutputs, infoStates []*prototk.EndorsableState) (_ *UpdateLockParams, err error) {
	lockID := lockTransition.prevLockInfo.LockID
	spendData := lockTransition.newLockInfo.SpendData
	cancelData := lockTransition.newLockInfo.CancelData
	spendTxId := lockTransition.newLockInfo.SpendTxId

	var lockParams LockParams
	var notoLockOpEncoded []byte
	lockParams.Options, err = h.noto.encodeNotoLockOptions(ctx, &types.NotoLockOptions{
		SpendTxId: spendTxId,
	})
	if err == nil {
		lockParams.SpendHash, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, lockID, spendTxId.String(), endorsableStateIDs(lockedInputs, false), endorsableStateIDs(spendOutputs, false), spendData)
	}
	if err == nil {
		lockParams.CancelHash, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, lockID, spendTxId.String(), endorsableStateIDs(lockedInputs, false), endorsableStateIDs(cancelOutputs, false), cancelData)
	}
	if err == nil {
		// The noto lock operation here is empty, as we are just modifying the lock
		notoLockOpEncoded, err = h.noto.encodeNotoLockOperation(ctx, &types.NotoLockOperation{
			TxId:          tx.Transaction.TransactionId,
			Inputs:        []string{lockTransition.prevLockState.Id},
			Outputs:       []string{lockTransition.newLockState.Id},
			LockedOutputs: []string{}, // must be empty for updateLock
			Proof:         proof,
		})
	}
	if err != nil {
		return nil, err
	}

	txData, err := h.noto.encodeTransactionData(ctx, tx.DomainConfig, tx.Transaction, infoStates)
	if err != nil {
		return nil, err
	}

	return &UpdateLockParams{
		LockID:       lockID,
		UpdateInputs: notoLockOpEncoded,
		Params:       lockParams,
		Data:         txData,
	}, nil

}

func (h *unlockCommon) buildCreateLockParams(ctx context.Context, tx *types.ParsedTransaction, lockTransition *lockTransition, proof pldtypes.HexBytes, inputs, lockedOutputs, additionalOutputs, spendOutputs, cancelOutputs, infoStates []*prototk.EndorsableState) (_ *CreateLockParams, err error) {
	lockID := lockTransition.newLockInfo.LockID
	spendData := lockTransition.newLockInfo.SpendData
	cancelData := lockTransition.newLockInfo.CancelData
	spendTxId := lockTransition.newLockInfo.SpendTxId

	var lockParams LockParams
	var notoLockOpEncoded []byte
	lockParams.Options, err = h.noto.encodeNotoLockOptions(ctx, &types.NotoLockOptions{
		SpendTxId: spendTxId,
	})
	if err == nil {
		lockParams.SpendHash, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, lockID, spendTxId.String(), endorsableStateIDs(lockedOutputs, false), endorsableStateIDs(spendOutputs, false), spendData)
	}
	if err == nil {
		lockParams.CancelHash, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, lockID, spendTxId.String(), endorsableStateIDs(lockedOutputs, false), endorsableStateIDs(cancelOutputs, false), cancelData)
	}
	if err == nil {
		// The noto lock operation here is empty, as we are just modifying the lock
		notoLockOpEncoded, err = h.noto.encodeNotoLockOperation(ctx, &types.NotoLockOperation{
			TxId:          tx.Transaction.TransactionId,
			Inputs:        endorsableStateIDs(inputs, false),
			Outputs:       append([]string{lockTransition.newLockState.Id}, endorsableStateIDs(additionalOutputs, false)...),
			LockedOutputs: endorsableStateIDs(lockedOutputs, false),
			Proof:         proof,
		})
	}
	if err != nil {
		return nil, err
	}

	txData, err := h.noto.encodeTransactionData(ctx, tx.DomainConfig, tx.Transaction, infoStates)
	if err != nil {
		return nil, err
	}

	return &CreateLockParams{
		CreateInputs: notoLockOpEncoded,
		Params:       lockParams,
		Data:         txData,
	}, nil

}
