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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/signpayloads"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

type delegateLockHandler struct {
	noto *Noto
}

func (h *delegateLockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var delegateParams types.DelegateLockParams
	if err := json.Unmarshal([]byte(params), &delegateParams); err != nil {
		return nil, err
	}
	if delegateParams.LockID.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "lockId")
	}
	if config.IsV0() && delegateParams.Unlock == nil {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "unlock")
	}
	if delegateParams.Delegate.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidDelegate, delegateParams.Delegate)
	}
	return &delegateParams, nil
}

func (h *delegateLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	notary := tx.DomainConfig.NotaryLookup
	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(notary, tx.Transaction.From),
	}, nil
}

func (h *delegateLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.DelegateLockParams)
	notary := tx.DomainConfig.NotaryLookup

	notaryID, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// Requester must own at least one locked coin state to show ownership of the lock
	lockedInputs, revert, err := h.noto.prepareLockedInputs(ctx, req.StateQueryContext, params.LockID, senderID.address, big.NewInt(1), false)
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

	infoDistribution := identityList{notaryID, senderID}
	infoStates, err := h.noto.prepareDataInfo(params.Data, tx.DomainConfig.Variant, infoDistribution.identities())
	if err != nil {
		return nil, err
	}
	var lockState *prototk.NewState
	if tx.DomainConfig.IsV0() {
		lockState, err = h.noto.prepareLockInfo_V0(params.LockID, senderID.address, params.Delegate, infoDistribution)
	} else {
		lockState, err = h.noto.prepareLockInfo_V1(params.LockID, senderID.address, nil, infoDistribution)
	}
	if err != nil {
		return nil, err
	}
	infoStates = append(infoStates, lockState)

	// This approval may leak the requesting signature on-chain, as all the inputs are visible on-chain
	// TODO: need to include the spend of the UTXO state for the lock in this as that masks the delegate
	encodedApproval, err := h.noto.encodeDelegateLock(ctx, tx.ContractAddress, params.LockID, params.Delegate, params.Data)
	if err != nil {
		return nil, err
	}

	if !tx.DomainConfig.IsV0() {
		manifestState, err := h.noto.newManifestBuilder().
			addInfoStates(infoDistribution, infoStates...).
			buildManifest(ctx, req.StateQueryContext)
		if err != nil {
			return nil, err
		}
		infoStates = append([]*prototk.NewState{manifestState} /* manifest first */, infoStates...)
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			ReadStates: lockedInputs.states,
			InfoStates: infoStates,
		},
		AttestationPlan: []*prototk.AttestationRequest{
			// Sender confirms the initial request with a signature
			{
				Name:            "sender",
				AttestationType: prototk.AttestationType_SIGN,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				PayloadType:     signpayloads.OPAQUE_TO_RSV,
				Payload:         encodedApproval,
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

func (h *delegateLockHandler) decodeStates(states []*pldapi.StateEncoded) []*prototk.EndorsableState {
	result := make([]*prototk.EndorsableState, len(states))
	for i, state := range states {
		result[i] = &prototk.EndorsableState{
			Id:            state.ID.String(),
			SchemaId:      state.Schema.String(),
			StateDataJson: pldtypes.RawJSON(state.Data).String(),
		}
	}
	return result
}

func (h *delegateLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.DelegateLockParams)
	inputs, err := h.noto.parseCoinList(ctx, "read", req.Reads)
	if err != nil {
		return nil, err
	}

	// Sender must specify at least one locked state, to show that they own the lock
	if len(inputs.lockedCoins) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgNoStatesSpecified)
	}
	if err := h.noto.validateLockOwners(ctx, tx.Transaction.From, req.ResolvedVerifiers, inputs.lockedCoins, inputs.lockedStates); err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	encodedApproval, err := h.noto.encodeDelegateLock(ctx, tx.ContractAddress, params.LockID, params.Delegate, params.Data)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateSignature(ctx, "sender", req.Signatures, encodedApproval); err != nil {
		return nil, err
	}
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *delegateLockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.DelegateLockParams)

	signature := domain.FindAttestation("sender", req.AttestationResult)
	if signature == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	txData, err := h.noto.encodeTransactionData(ctx, tx.DomainConfig, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}

	var interfaceABI abi.ABI
	var functionName string
	var paramsJSON []byte

	if tx.DomainConfig.IsV1() {
		interfaceABI = h.noto.getInterfaceABI(types.NotoVariantDefault)
		functionName = "delegateLock"

		var delegateInputsEncoded pldtypes.HexBytes
		delegateInputsEncoded, err = h.noto.encodeNotoDelegateOperation(ctx, &types.NotoDelegateOperation{
			TxId:  req.Transaction.TransactionId,
			Proof: signature.Payload,
		})
		if err == nil {
			params := &DelegateLockParams{
				LockID:         inParams.LockID,
				DelegateInputs: delegateInputsEncoded,
				NewSpender:     inParams.Delegate,
				Data:           txData,
			}
			paramsJSON, err = json.Marshal(params)
		}
	} else {
		interfaceABI = h.noto.getInterfaceABI(types.NotoVariantLegacy)
		functionName = "delegateLock"
		// V0: delegateLock requires unlockHash
		var unlockHash ethtypes.HexBytes0xPrefix
		unlockHash, err = h.noto.unlockHashFromIDs_V0(ctx, tx.ContractAddress, inParams.Unlock.LockedInputs, inParams.Unlock.LockedOutputs, inParams.Unlock.Outputs, inParams.Unlock.Data)
		if err != nil {
			return nil, err
		}
		unlockHashBytes32 := pldtypes.Bytes32(unlockHash)
		params := &NotoDelegateLock_V0_Params{
			TxId:       req.Transaction.TransactionId,
			UnlockHash: &unlockHashBytes32,
			Delegate:   inParams.Delegate,
			Signature:  signature.Payload,
			Data:       txData,
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

func (h *delegateLockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.DelegateLockParams)

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &ApproveUnlockHookParams{
		Sender:   senderID.address,
		LockID:   inParams.LockID,
		Delegate: inParams.Delegate,
		Data:     inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onDelegateLock"],
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

func (h *delegateLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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
