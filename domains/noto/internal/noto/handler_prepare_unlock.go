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
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

type prepareUnlockHandler struct {
	unlockCommon
}

func (h *prepareUnlockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var unlockParams types.UnlockParams
	err := json.Unmarshal([]byte(params), &unlockParams)
	if err == nil {
		err = h.validateParams(ctx, &unlockParams)
	}
	return &unlockParams, err
}

func (h *prepareUnlockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	return h.init(ctx, tx, params)
}

func (h *prepareUnlockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	notary := tx.DomainConfig.NotaryLookup
	spendTxId := pldtypes.Bytes32UUIDFirst16(uuid.New())

	res, mb, states, err := h.assembleStates(ctx, tx, &spendTxId, params, req)
	if err != nil || res.AssemblyResult != prototk.AssembleTransactionResponse_OK {
		return res, err
	}

	notaryID, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	cancelOutputs, err := h.noto.prepareOutputs(fromID, (*pldtypes.HexUint256)(states.lockedInputs.total), identityList{notaryID, fromID})
	if err != nil {
		return nil, err
	}

	if !tx.DomainConfig.IsV0() {
		manifestState, err := mb.
			addOutputs(cancelOutputs). // note no v0UnlockedOutputs as we're V1 only for the manifest
			buildManifest(ctx, req.StateQueryContext)
		if err != nil {
			return nil, err
		}
		states.info = append([]*prototk.NewState{manifestState} /* manifest first */, states.info...)
	}

	assembledTransaction := &prototk.AssembledTransaction{}
	assembledTransaction.ReadStates = states.lockedInputs.states
	assembledTransaction.InfoStates = states.info
	assembledTransaction.InfoStates = append(assembledTransaction.InfoStates, states.outputs.states...)
	var v0LockedCoins []*types.NotoLockedCoin
	if tx.DomainConfig.IsV1() {
		assembledTransaction.InfoStates = append(assembledTransaction.InfoStates, cancelOutputs.states...)
	} else {
		v0LockedCoins = states.v0LockedOutputs.coins
		assembledTransaction.InfoStates = append(assembledTransaction.InfoStates, states.v0LockedOutputs.states...)
	}

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

func (h *prepareUnlockHandler) endorse_V0(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
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

	return h.endorse(ctx, tx, params, req, inputs, outputs, nil)
}

func (h *prepareUnlockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	if tx.DomainConfig.IsV0() {
		return h.endorse_V0(ctx, tx, req)
	}

	params := tx.Params.(*types.UnlockParams)
	lockedInputs := req.Reads
	allOutputs := h.noto.filterSchema(req.Info, []string{h.noto.coinSchema.Id})
	spendOutputs, cancelOutputs, err := h.noto.splitUnlockOutputs(ctx, allOutputs)
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

	return h.endorse(ctx, tx, params, req, parsedInputs, parsedSpendOutputs, parsedCancelOutputs)
}

func (h *prepareUnlockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (_ *TransactionWrapper, err error) {
	inParams := tx.Params.(*types.UnlockParams)
	lockedInputs := h.noto.filterSchema(req.ReadStates, []string{h.noto.lockedCoinSchema.Id})
	spendOutputs, lockedOutputs := h.noto.splitStates(req.InfoStates)
	var cancelOutputs []*prototk.EndorsableState
	if !tx.DomainConfig.IsV0() {
		spendOutputs, cancelOutputs, err = h.noto.splitUnlockOutputs(ctx, spendOutputs)
	}
	if err != nil {
		return nil, err
	}

	// As of V1 the spentTxId is in the lock state
	var spendTxId pldtypes.Bytes32
	if !tx.DomainConfig.IsV0() {
		lockInfoStates := h.noto.filterSchema(req.InfoStates, []string{h.noto.lockInfoSchemaV1.Id})
		if len(lockInfoStates) > 0 {
			lock, err := h.noto.unmarshalLockV1(lockInfoStates[0].StateDataJson)
			if err == nil {
				spendTxId = lock.SpendTxId
			}
		}
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
	var txData pldtypes.HexBytes

	switch tx.DomainConfig.Variant {
	case types.NotoVariantDefault:
		var lockParams LockParams
		var notoLockOpEncoded []byte
		lockParams.Options, err = h.noto.encodeNotoLockOptions(ctx, &types.NotoLockOptions{
			SpendTxId: spendTxId,
		})
		if err == nil {
			lockParams.SpendHash, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, spendTxId.String(), endorsableStateIDs(lockedInputs), endorsableStateIDs(spendOutputs), inParams.Data)
		}
		if err == nil {
			lockParams.CancelHash, err = h.noto.unlockHashFromIDs_V1(ctx, tx.ContractAddress, spendTxId.String(), endorsableStateIDs(lockedInputs), endorsableStateIDs(cancelOutputs), inParams.Data)
		}
		if err == nil {
			// The noto lock operation here is empty, as we are just modifying the
			// TODO: Consider if we use a UTXO on-chain to track the
			notoLockOpEncoded, err = h.noto.encodeNotoLockOperation(ctx, &types.NotoLockOperation{
				TxId:          req.Transaction.TransactionId,
				Inputs:        []string{},
				Outputs:       []string{},
				LockedOutputs: []string{}, // must be empty for updateLock
				Proof:         pldtypes.HexBytes{},
			})
		}
		if err != nil {
			return nil, err
		}

		txData, err = h.noto.encodeTransactionData(ctx, tx.DomainConfig, tx.Transaction, req.InfoStates)
		if err == nil {
			interfaceABI = h.noto.getInterfaceABI(types.NotoVariantDefault)
			functionName = "updateLock"
			params := &UpdateLockParams{
				LockID:       inParams.LockID,
				UpdateInputs: notoLockOpEncoded,
				Params:       lockParams,
				Data:         txData,
			}
			paramsJSON, err = json.Marshal(params)
		}
	default:
		var unlockHash ethtypes.HexBytes0xPrefix
		unlockHash, err = h.noto.unlockHashFromIDs_V0(ctx, tx.ContractAddress, endorsableStateIDs(lockedInputs), endorsableStateIDs(lockedOutputs), endorsableStateIDs(spendOutputs), inParams.Data)
		if err != nil {
			return nil, err
		}
		// We must use the legacy encoding for the transaction data, because there is no other place
		// to pass in the transaction ID on this method
		txData, err = h.noto.encodeTransactionData(ctx, tx.DomainConfig, tx.Transaction, req.InfoStates)
		if err == nil {
			interfaceABI = h.noto.getInterfaceABI(types.NotoVariantLegacy)
			functionName = "prepareUnlock"
			params := &NotoPrepareUnlock_V0_Params{
				LockedInputs: endorsableStateIDs(lockedInputs),
				UnlockHash:   unlockHash.String(),
				Signature:    sender.Payload,
				Data:         txData,
			}
			paramsJSON, err = json.Marshal(params)
		}
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
