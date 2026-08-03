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
	"slices"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// lockCommon is shared by all lock-family handlers (create/prepare/unlock)
// TODO: split out to a separate file
type lockCommon struct {
	noto *Noto
}

type unlockInfoInput struct {
	*resolvedIdentities
	recipients         []*types.UnlockRecipient
	unlockData         []byte
	spendOutputs       *preparedOutputs
	cancelOutputs      *preparedOutputs
	omitCancelManifest bool
}

type unlockInfo struct {
	spendData        []byte
	cancelData       []byte
	infoStates       []*prototk.NewState
	infoDistribution identityList
}

func (h *lockCommon) checkAllowed(ctx context.Context, tx *types.ParsedTransaction, from string) error {
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

// buildUnlockOperationData builds a manifest for one operation (spend or cancel) and encodes
// the transaction data referencing the operation manifest + info states.
// v1 Paladin always builds a manifest
func (h *lockCommon) buildUnlockOperationData(
	ctx context.Context,
	tx *types.ParsedTransaction,
	stateQueryContext string,
	outputs *preparedOutputs,
	infoDistribution identityList,
	infoStates []*prototk.NewState,
) (encodedData []byte, manifestState *prototk.NewState, err error) {

	operationInfoStates := slices.Clone(infoStates)

	if !tx.DomainConfig.IsV0() {
		manifestState, err = h.noto.newManifestBuilder().
			addOutputs(outputs).
			addInfoStates(infoDistribution, infoStates...).
			buildManifest(ctx, stateQueryContext)
		if err != nil {
			return nil, nil, err
		}

		if err = h.noto.allocateStateIDs(ctx, stateQueryContext, []*prototk.NewState{manifestState}); err != nil {
			return nil, nil, err
		}

		operationInfoStates = append([]*prototk.NewState{manifestState}, operationInfoStates...)
	}

	endorsableInfoStates := make([]*prototk.EndorsableState, len(operationInfoStates))
	for i, s := range operationInfoStates {
		endorsableInfoStates[i] = &prototk.EndorsableState{
			Id:            *s.Id,
			SchemaId:      s.SchemaId,
			StateDataJson: s.StateDataJson,
		}
	}
	encodedData, err = h.noto.encodeTransactionData(ctx, tx.DomainConfig, tx.Transaction, endorsableInfoStates)
	return
}

// buildUnlockInfo builds the encoded data for spend and cancel operations
func (h *lockCommon) buildUnlockInfo(ctx context.Context, tx *types.ParsedTransaction, resolvedVerifiers []*prototk.ResolvedVerifier, stateQueryContext string, in *unlockInfoInput) (*unlockInfo, error) {
	infoDistribution, err := h.getAllRecipientsDistribution(ctx, tx, in.notary, in.sender, in.from, in.recipients, resolvedVerifiers)
	if err != nil {
		return nil, err
	}

	infoStates, err := h.noto.prepareDataInfo(ctx, in.unlockData, tx.DomainConfig.Variant, infoDistribution.identities(), tx.Transaction, resolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// we need to allocate the IDs of the states at this point
	err = h.noto.allocateStateIDs(ctx, stateQueryContext, infoStates)
	if err != nil {
		return nil, err
	}

	spendData, spendManifest, err := h.buildUnlockOperationData(ctx, tx, stateQueryContext, in.spendOutputs, infoDistribution, infoStates)
	if err != nil {
		return nil, err
	}

	var cancelData []byte
	var cancelManifest *prototk.NewState
	if !in.omitCancelManifest {
		cancelData, cancelManifest, err = h.buildUnlockOperationData(ctx, tx, stateQueryContext, in.cancelOutputs, infoDistribution, infoStates)
		if err != nil {
			return nil, err
		}
	}

	if !tx.DomainConfig.IsV0() {
		manifestStates := []*prototk.NewState{spendManifest}
		if cancelManifest != nil {
			manifestStates = append(manifestStates, cancelManifest)
		}
		infoStates = append(manifestStates, infoStates...)
	}

	return &unlockInfo{
		spendData:        spendData,
		cancelData:       cancelData,
		infoStates:       infoStates,
		infoDistribution: infoDistribution,
	}, nil
}

func (h *lockCommon) getAllRecipientsDistribution(ctx context.Context, tx *types.ParsedTransaction, notaryID, senderID, fromID *identityPair, recipients []*types.UnlockRecipient, resolvedVerifiers []*prototk.ResolvedVerifier) (identityList, error) {
	distribution := make(identityList, 0, len(recipients)+2)
	distribution = append(distribution, notaryID, senderID)
	if fromID != nil {
		distribution = append(distribution, fromID)
	}
	for _, entry := range recipients {
		toID, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, resolvedVerifiers)
		if err != nil {
			return nil, err
		}
		distribution = append(distribution, toID)
	}
	return distribution, nil
}

func (h *lockCommon) assembleUnlockOutputs_V1(ctx context.Context, tx *types.ParsedTransaction, notaryID, fromID *identityPair, recipients []*types.UnlockRecipient, resolvedVerifiers []*prototk.ResolvedVerifier, remainder *big.Int) (*preparedOutputs, error) {
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

func (h *lockCommon) buildPrepareUnlockParams(ctx context.Context, tx *types.ParsedTransaction, lt *lockTransition, proof pldtypes.HexBytes, lockedInputs, spendOutputs, cancelOutputs, infoStates []*prototk.EndorsableState) (_ []byte, err error) {
	useNullifiers := tx.DomainConfig.IsNullifierVariant()

	lockID := lt.prevLockInfo.LockID
	spendData := lt.newLockInfo.SpendData
	cancelData := lt.newLockInfo.CancelData
	spendTxId := lt.newLockInfo.SpendTxId

	options := types.NotoLockOptions{
		SpendTxId: spendTxId,
	}

	var spendCommitment pldtypes.Bytes32
	var cancelCommitment pldtypes.Bytes32
	var updateLockArgs []byte

	spendCommitment, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, lockID, spendTxId.String(), endorsableStateIDs(ctx, lockedInputs, useNullifiers), endorsableStateIDs(ctx, spendOutputs, false), spendData)
	if err != nil {
		return nil, err
	}
	cancelCommitment, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, lockID, spendTxId.String(), endorsableStateIDs(ctx, lockedInputs, useNullifiers), endorsableStateIDs(ctx, cancelOutputs, false), cancelData)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.IsV1() {
		updateLockArgs, err = h.noto.encodeNotoUpdateLockArgsV1(ctx, &types.NotoUpdateLockArgs_V1{
			TxId:         tx.Transaction.TransactionId,
			OldLockState: lt.prevLockStateID,
			NewLockState: lt.newLockStateID,
			Proof:        proof,
		})
	} else if tx.DomainConfig.IsV2() {
		updateLockArgs, err = h.noto.encodeNotoUpdateLockArgs(ctx, &types.NotoUpdateLockArgs{
			TxId:         tx.Transaction.TransactionId,
			Contents:     endorsableStateIDs(ctx, lockedInputs, useNullifiers),
			OldLockState: lt.prevLockStateID,
			NewLockState: lt.newLockStateID,
			Options:      options,
			Proof:        proof,
		})
	} else {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
	}
	if err != nil {
		return nil, err
	}

	txData, err := h.noto.encodeTransactionData(ctx, tx.DomainConfig, tx.Transaction, infoStates)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.IsV1() {
		optionsEncoded, encErr := h.noto.encodeNotoLockOptions(ctx, &types.NotoLockOptions{
			SpendTxId: lt.newLockInfo.SpendTxId,
		})
		if encErr != nil {
			return nil, encErr
		}
		return json.Marshal(&UpdateLockParams_V1{
			LockID:     lockID,
			UpdateArgs: updateLockArgs,
			Params: LockParams_V1{
				SpendHash:  spendCommitment,
				CancelHash: cancelCommitment,
				Options:    optionsEncoded,
			},
			Data: txData,
		})
	} else if tx.DomainConfig.IsV2() {
		return json.Marshal(&UpdateLockParams{
			LockID:           lockID,
			UpdateArgs:       updateLockArgs,
			SpendCommitment:  spendCommitment,
			CancelCommitment: cancelCommitment,
			Data:             txData,
		})
	}
	return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
}

func (h *lockCommon) buildCreateLockParams(ctx context.Context, tx *types.ParsedTransaction, lockTransition *lockTransition, proof pldtypes.HexBytes, inputs, lockedOutputs, additionalOutputs, spendOutputs, cancelOutputs, infoStates []*prototk.EndorsableState) (_ []byte, err error) {
	useNullifiers := tx.DomainConfig.IsNullifierVariant()

	lockID := lockTransition.newLockInfo.LockID
	spendData := lockTransition.newLockInfo.SpendData
	cancelData := lockTransition.newLockInfo.CancelData
	spendTxId := lockTransition.newLockInfo.SpendTxId

	options := &types.NotoLockOptions{
		SpendTxId: spendTxId,
	}

	var spendCommitment pldtypes.Bytes32
	var cancelCommitment pldtypes.Bytes32
	var createLockArgs []byte

	spendCommitment, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, lockID, spendTxId.String(), endorsableStateIDs(ctx, lockedOutputs, false), endorsableStateIDs(ctx, spendOutputs, false), spendData)
	if err != nil {
		return nil, err
	}
	cancelCommitment, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, lockID, spendTxId.String(), endorsableStateIDs(ctx, lockedOutputs, false), endorsableStateIDs(ctx, cancelOutputs, false), cancelData)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.IsV1() {
		createLockArgs, err = h.noto.encodeNotoCreateLockArgsV1(ctx, &types.NotoCreateLockArgs_V1{
			TxId:         tx.Transaction.TransactionId,
			Inputs:       endorsableStateIDs(ctx, inputs, false),
			Outputs:      endorsableStateIDs(ctx, additionalOutputs, false),
			Contents:     endorsableStateIDs(ctx, lockedOutputs, false),
			NewLockState: lockTransition.newLockStateID,
			Proof:        proof,
		})
	} else if tx.DomainConfig.IsV2() {
		createLockArgs, err = h.noto.encodeNotoCreateLockArgs(ctx, &types.NotoCreateLockArgs{
			TxId:         tx.Transaction.TransactionId,
			Inputs:       endorsableStateIDs(ctx, inputs, useNullifiers),
			Outputs:      endorsableStateIDs(ctx, additionalOutputs, false),
			Contents:     endorsableStateIDs(ctx, lockedOutputs, false),
			NewLockState: lockTransition.newLockStateID,
			Options:      options,
			Proof:        proof,
		})
	} else {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
	}
	if err != nil {
		return nil, err
	}

	txData, err := h.noto.encodeTransactionData(ctx, tx.DomainConfig, tx.Transaction, infoStates)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.IsV1() {
		optionsEncoded, encErr := h.noto.encodeNotoLockOptions(ctx, &types.NotoLockOptions{
			SpendTxId: lockTransition.newLockInfo.SpendTxId,
		})
		if encErr != nil {
			return nil, encErr
		}
		return json.Marshal(&CreateLockParams_V1{
			CreateArgs: createLockArgs,
			Params: LockParams_V1{
				SpendHash:  spendCommitment,
				CancelHash: cancelCommitment,
				Options:    optionsEncoded,
			},
			Data: txData,
		})
	} else if tx.DomainConfig.IsV2() {
		return json.Marshal(&CreateLockParams{
			CreateArgs:       createLockArgs,
			SpendCommitment:  spendCommitment,
			CancelCommitment: cancelCommitment,
			Data:             txData,
		})
	}
	return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
}

// unlockCommon is shared by unlock + prepareUnlock only
type unlockCommon struct {
	lockCommon
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

// In V0 the remainder was returned locked
// Note that V0 did not include the newer create/prepare methods
// (hence this version is only needed for unlock/prepareUnlock)
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
