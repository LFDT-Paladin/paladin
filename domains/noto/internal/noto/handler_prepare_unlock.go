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
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

type prepareUnlockHandler struct {
	unlockCommon
}

func (h *prepareUnlockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var unlockParams types.PrepareUnlockParams
	err := json.Unmarshal([]byte(params), &unlockParams)
	if err == nil {
		err = h.validateParams(ctx, &unlockParams.UnlockParams)
	}
	return &unlockParams, err
}

func (h *prepareUnlockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.PrepareUnlockParams)
	return h.init(ctx, tx, &params.UnlockParams)
}

func (h *prepareUnlockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.PrepareUnlockParams)
	unlockParams := &params.UnlockParams
	spendTxId := pldtypes.Bytes32UUIDFirst16(uuid.New())

	unlockData := params.UnlockData
	if tx.DomainConfig.IsV0() {
		unlockData = params.Data // in V0 we used to use the same data in the unlock, so we preserve this behavior.
	}

	ids, err := resolveIdentities(ctx, h.noto, tx, req, unlockParams.From, "")
	if err != nil {
		return nil, err
	}
	notaryID, senderID, fromID := ids.notary, ids.sender, ids.from

	var existingLock *loadedLockInfo
	if !tx.DomainConfig.IsV0() {
		var revert bool
		existingLock, revert, err = h.noto.loadLockInfoV1(ctx, req.StateQueryContext, unlockParams.LockID)
		if res, err := assembleRevertOrError(revert, err); res != nil || err != nil {
			return res, err
		}
	}

	requiredTotal := big.NewInt(0)
	for _, entry := range unlockParams.Recipients {
		requiredTotal = requiredTotal.Add(requiredTotal, entry.Amount.Int())
	}

	lockedInputs, revert, err := h.noto.prepareLockedInputs(ctx, req.StateQueryContext, unlockParams.LockID, fromID.address, requiredTotal, true)
	if res, err := assembleRevertOrError(revert, err); res != nil || err != nil {
		return res, err
	}

	remainder := big.NewInt(0).Sub(lockedInputs.total, requiredTotal)

	var outputs *preparedOutputs
	var v0LockedOutputs *preparedLockedOutputs
	if tx.DomainConfig.IsV0() {
		outputs, v0LockedOutputs, err = h.assembleUnlockOutputs_V0(ctx, tx, unlockParams, req, fromID.address, remainder)
	} else {
		outputs, err = h.assembleUnlockOutputs_V1(ctx, tx, notaryID, fromID, unlockParams.Recipients, req.ResolvedVerifiers, remainder)
	}
	if err != nil {
		return nil, err
	}

	var cancelOutputs *preparedOutputs
	if !tx.DomainConfig.IsV0() {
		cancelOutputs, err = h.noto.prepareOutputs(fromID, (*pldtypes.HexUint256)(lockedInputs.total), identityList{notaryID, fromID})
		if err != nil {
			return nil, err
		}
	}

	unlockInfo, err := h.buildUnlockInfo(ctx, tx, req.ResolvedVerifiers, req.StateQueryContext, &unlockInfoInput{
		resolvedIdentities: ids,
		recipients:         unlockParams.Recipients,
		unlockData:         unlockData,
		spendOutputs:       outputs,
		cancelOutputs:      cancelOutputs,
	})
	if err != nil {
		return nil, err
	}
	infoStates := unlockInfo.infoStates

	if tx.DomainConfig.IsV0() {
		lock, err := h.noto.prepareLockInfo_V0(unlockParams.LockID, fromID.address, nil, unlockInfo.infoDistribution)
		if err != nil {
			return nil, err
		}
		infoStates = append(infoStates, lock.state)
	}

	var lock *preparedLockInfo
	if !tx.DomainConfig.IsV0() {
		// The tx data for the prepareUnlock itself needs to be distributed (separate to the unlockData)
		var prepareInfoStates []*prototk.NewState
		prepareInfoStates, err = h.noto.prepareDataInfo(ctx, params.Data, tx.DomainConfig.Variant, unlockInfo.infoDistribution.identities(), tx.Transaction, req.ResolvedVerifiers)
		if err == nil {
			infoStates = append(infoStates, prepareInfoStates...)

			// ... and add the new lock state as an output
			newLockInfo := *existingLock.lockInfo
			newLockInfo.Replaces = existingLock.id
			newLockInfo.Salt = pldtypes.RandBytes32()
			newLockInfo.SpendOutputs = newStateAllocatedIDs(outputs.states)
			newLockInfo.SpendData = unlockInfo.spendData
			newLockInfo.CancelOutputs = newStateAllocatedIDs(cancelOutputs.states)
			newLockInfo.CancelData = unlockInfo.cancelData
			newLockInfo.SpendTxId = spendTxId
			lock, err = h.noto.prepareLockInfo_V1(&newLockInfo, identityList{notaryID, senderID, fromID})
		}

		// .. and then the manifest
		var manifestState *prototk.NewState
		if err == nil {
			manifestState, err = h.noto.newManifestBuilder().
				addOutputs(outputs).
				addInfoStates(unlockInfo.infoDistribution, infoStates...).
				addOutputs(cancelOutputs).
				addLockInfo(lock).
				addInfoStates(unlockInfo.infoDistribution, prepareInfoStates...).
				buildManifest(ctx, req.StateQueryContext)
		}
		if err != nil {
			return nil, err
		}
		infoStates = append([]*prototk.NewState{manifestState}, infoStates...)
	}

	assembledTransaction := &prototk.AssembledTransaction{}
	assembledTransaction.ReadStates = lockedInputs.states
	assembledTransaction.InfoStates = infoStates
	assembledTransaction.InfoStates = append(assembledTransaction.InfoStates, outputs.states...)
	var v0LockedCoins []*types.NotoLockedCoin
	if tx.DomainConfig.IsV0() {
		v0LockedCoins = v0LockedOutputs.coins
		assembledTransaction.InfoStates = append(assembledTransaction.InfoStates, v0LockedOutputs.states...)
	} else {
		assembledTransaction.InfoStates = append(assembledTransaction.InfoStates, cancelOutputs.states...)
		assembledTransaction.InputStates = append(assembledTransaction.InputStates, existingLock.stateRef)
		assembledTransaction.OutputStates = append(assembledTransaction.OutputStates, lock.state)
	}

	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, lockedInputs.coins, v0LockedCoins, outputs.coins)
	if err != nil {
		return nil, err
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult:       prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: assembledTransaction,
		AttestationPlan:      buildEndorsePlan(tx.DomainConfig.NotaryLookup, req.Transaction.From, encodedUnlock),
	}, nil
}

func (h *prepareUnlockHandler) endorse_V0(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.PrepareUnlockParams)
	lockedInputs := req.Reads
	allOutputs := h.noto.filterSchema(req.Info, []string{h.noto.coinSchema.Id, h.noto.lockedCoinSchema.Id})

	inputs, err := h.noto.parseCoinList(ctx, "input", lockedInputs)
	if err != nil {
		return nil, err
	}
	outputs, err := h.noto.parseCoinList(ctx, "output", allOutputs)
	if err != nil {
		return nil, err
	}

	return h.endorse(ctx, tx, &params.UnlockParams, req, inputs, outputs, nil)
}

func (h *prepareUnlockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	if tx.DomainConfig.IsV0() {
		return h.endorse_V0(ctx, tx, req)
	}

	params := tx.Params.(*types.PrepareUnlockParams)
	lockedInputs := req.Reads

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
	_, spendOutputs, cancelOutputs, err := h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_UPDATE, senderID, &params.LockID, req.Inputs, req.Outputs, req.Info)
	if err != nil {
		return nil, err
	}

	parsedInputs, err := h.noto.parseCoinList(ctx, "input", lockedInputs)
	if err != nil {
		return nil, err
	}
	parsedSpendOutputs, err := h.noto.parseCoinList(ctx, "output", spendOutputs)
	if err != nil {
		return nil, err
	}
	parsedCancelOutputs, err := h.noto.parseCoinList(ctx, "output", cancelOutputs)
	if err != nil {
		return nil, err
	}

	return h.endorse(ctx, tx, &params.UnlockParams, req, parsedInputs, parsedSpendOutputs, parsedCancelOutputs)
}

func (h *prepareUnlockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (_ *TransactionWrapper, err error) {
	inParams := tx.Params.(*types.PrepareUnlockParams)
	lockedInputs := h.noto.filterSchema(req.ReadStates, []string{h.noto.lockedCoinSchema.Id})
	spendOutputs, lockedOutputs := h.noto.splitStates(req.InfoStates)

	var lockTransition *lockTransition           // v1 only
	var cancelOutputs []*prototk.EndorsableState // v1 only
	if !tx.DomainConfig.IsV0() {
		senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}

		// We should have a valid lock transition, from which we can obtain the spend and cancel outputs
		lockTransition, spendOutputs, cancelOutputs, err = h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_UPDATE, senderID, &inParams.LockID, req.InputStates, req.OutputStates, req.InfoStates)
		if err != nil {
			return nil, err
		}
	}

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	sender := domain.FindAttestation("sender", req.AttestationResult)
	if sender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	interfaceABI := h.noto.getInterfaceABI(tx.DomainConfig.Variant)
	var functionName string
	var paramsJSON []byte

	if tx.DomainConfig.IsV0() {
		var unlockHash ethtypes.HexBytes0xPrefix
		unlockHash, err = h.noto.unlockHashFromIDs_V0(ctx, tx.ContractAddress, endorsableStateIDs(ctx, lockedInputs, false), endorsableStateIDs(ctx, lockedOutputs, false), endorsableStateIDs(ctx, spendOutputs, false), inParams.Data)
		if err != nil {
			return nil, err
		}
		// We must use the legacy encoding for the transaction data, because there is no other place
		// to pass in the transaction ID on this
		var txData pldtypes.HexBytes
		txData, err = h.noto.encodeTransactionData(ctx, tx.DomainConfig, tx.Transaction, req.InfoStates)
		if err == nil {
			functionName = "prepareUnlock"
			paramsJSON, err = json.Marshal(&NotoPrepareUnlock_V0_Params{
				LockedInputs: endorsableStateIDs(ctx, lockedInputs, false),
				UnlockHash:   unlockHash.String(),
				Signature:    sender.Payload,
				Data:         txData,
			})
		}
	} else if tx.DomainConfig.IsV1() || tx.DomainConfig.IsV2() {
		functionName = "updateLock"
		paramsJSON, err = h.buildPrepareUnlockParams(ctx, tx, lockTransition, sender.Payload, lockedInputs, spendOutputs, cancelOutputs, req.InfoStates)
	} else {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
	}
	if err != nil {
		return nil, err
	}

	return &TransactionWrapper{
		functionABI: interfaceABI.Functions()[functionName],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *prepareUnlockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.PrepareUnlockParams)

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
		hooksBuild.ABI.Functions()["onPrepareUnlock"],
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

func (h *prepareUnlockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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
