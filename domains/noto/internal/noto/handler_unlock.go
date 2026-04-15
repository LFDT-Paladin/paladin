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
)

type unlockHandler struct {
	unlockCommon
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

	res, mb, states, err := h.assembleStates(ctx, tx, nil, params, req, params.Data /* we're directly unlocking, so the unlock data is just the "data" param */)
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
		senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}

		// In V1 onwards the lock itself needs to be checked (it's owned by the from address)
		_, err = h.noto.validateV1LockTransition(ctx, LOCK_SPEND, senderID, &params.LockID, req.Inputs, req.Outputs)
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

	interfaceABI := h.noto.getInterfaceABI(tx.DomainConfig.Variant)
	var functionName string
	var paramsJSON []byte

	if tx.DomainConfig.IsV0() {
		functionName = "unlock"
		paramsJSON, err = json.Marshal(&NotoUnlock_V0_Params{
			TxId:          req.Transaction.TransactionId,
			LockedInputs:  endorsableStateIDs(lockedInputs),
			LockedOutputs: endorsableStateIDs(lockedOutputs),
			Outputs:       endorsableStateIDs(outputs),
			Signature:     unlockSignature.Payload,
			Data:          txData,
		})
	} else {
		functionName = "spendLock"
		var notoUnlockOpEncoded []byte
		notoUnlockOpEncoded, err = h.noto.encodeNotoUnlockOperation(ctx, inParams.LockID, &types.NotoUnlockOperation{
			TxId:    req.Transaction.TransactionId,
			Inputs:  endorsableStateIDs(lockedInputs),
			Outputs: endorsableStateIDs(outputs),
			Data:    txData,
			Proof:   unlockSignature.Payload,
		})
		if err == nil {
			paramsJSON, err = json.Marshal(&SpendLockParams{
				LockID:      inParams.LockID,
				SpendInputs: notoUnlockOpEncoded,
				Data:        []byte{}, // we don't need this outer data
			})
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
