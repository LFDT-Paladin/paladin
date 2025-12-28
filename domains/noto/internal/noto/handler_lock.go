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

type lockHandler struct {
	noto *Noto
}

func (h *lockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var lockParams types.LockParams
	if err := json.Unmarshal([]byte(params), &lockParams); err != nil {
		return nil, err
	}
	if config.IsV0() {
		// V0 did not support empty locks
		if lockParams.Amount == nil || lockParams.Amount.Int().Sign() != 1 {
			return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
		}
	}
	return &lockParams, nil
}

func (h *lockHandler) checkAllowed(ctx context.Context, tx *types.ParsedTransaction) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if *tx.DomainConfig.Options.Basic.AllowLock {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgLockNotAllowed)
}

func (h *lockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx); err != nil {
		return nil, err
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(notary, tx.Transaction.From),
	}, nil
}

func (h *lockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.LockParams)
	notary := tx.DomainConfig.NotaryLookup

	notaryID, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	inputStates, revert, err := h.noto.prepareInputs(ctx, req.StateQueryContext, senderID, params.Amount)
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

	// Pre-compute the lockId as it will be generated on the smart contract
	lockID, err := h.noto.computeLockIDForLockTX(ctx, tx, notaryID)
	if err != nil {
		return nil, err
	}

	lockedOutputStates, err := h.noto.prepareLockedOutputs(lockID, senderID, params.Amount, identityList{notaryID, senderID})
	if err != nil {
		return nil, err
	}

	unlockedOutputStates := &preparedOutputs{}
	if inputStates.total.Cmp(params.Amount.Int()) == 1 {
		remainder := big.NewInt(0).Sub(inputStates.total, params.Amount.Int())
		returnedStates, err := h.noto.prepareOutputs(senderID, (*pldtypes.HexUint256)(remainder), identityList{notaryID, senderID})
		if err != nil {
			return nil, err
		}
		unlockedOutputStates.distributions = append(unlockedOutputStates.distributions, returnedStates.distributions...)
		unlockedOutputStates.coins = append(unlockedOutputStates.coins, returnedStates.coins...)
		unlockedOutputStates.states = append(unlockedOutputStates.states, returnedStates.states...)
	}

	infoDistribution := identityList{notaryID, senderID}
	infoStates, err := h.noto.prepareDataInfo(params.Data, tx.DomainConfig.Variant, infoDistribution.identities())
	if err != nil {
		return nil, err
	}
	var outputStates []*prototk.NewState
	outputStates = append(outputStates, lockedOutputStates.states...)
	outputStates = append(outputStates, unlockedOutputStates.states...)

	var lock *preparedLockInfo
	if tx.DomainConfig.IsV0() {
		lock, err = h.noto.prepareLockInfo_V0(lockID, senderID.address, nil, infoDistribution)
		if err == nil {
			infoStates = append(infoStates, lock.state) // in V0 lock states were just published as info
		}
	} else {
		lock, err = h.noto.prepareLockInfo_V1(&types.NotoLockInfo_V1{
			Salt:    pldtypes.RandBytes32(),
			LockID:  lockID,
			Owner:   senderID.address,
			Spender: senderID.address,
		}, identityList{notaryID, senderID})
		if err == nil {
			outputStates = append(outputStates, lock.state) // as of V1 it is a first class transitioned state
		}
	}
	if err != nil {
		return nil, err
	}

	encodedLock, err := h.noto.encodeLock(ctx, tx.ContractAddress, inputStates.coins, unlockedOutputStates.coins, lockedOutputStates.coins)
	if err != nil {
		return nil, err
	}

	if !tx.DomainConfig.IsV0() {
		manifestState, err := h.noto.newManifestBuilder().
			addLockedOutputs(lockedOutputStates).
			addOutputs(unlockedOutputStates).
			addInfoStates(infoDistribution, infoStates...).
			addLockInfo(lock).
			buildManifest(ctx, req.StateQueryContext)
		if err != nil {
			return nil, err
		}
		infoStates = append([]*prototk.NewState{manifestState} /* manifest first */, infoStates...)
	}

	attestation := []*prototk.AttestationRequest{
		// Sender confirms the initial request with a signature
		{
			Name:            "sender",
			AttestationType: prototk.AttestationType_SIGN,
			Algorithm:       algorithms.ECDSA_SECP256K1,
			VerifierType:    verifiers.ETH_ADDRESS,
			Payload:         encodedLock,
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
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			InputStates:  inputStates.states,
			OutputStates: outputStates,
			InfoStates:   infoStates,
		},
		AttestationPlan: attestation,
	}, nil
}

func (h *lockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	notary := tx.DomainConfig.NotaryLookup

	if err := h.checkAllowed(ctx, tx); err != nil {
		return nil, err
	}

	notaryID, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	inputs, err := h.noto.parseCoinList(ctx, "input", req.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, err := h.noto.parseCoinList(ctx, "output", req.Outputs)
	if err != nil {
		return nil, err
	}

	if !tx.DomainConfig.IsV0() {
		lockID, err := h.noto.computeLockIDForLockTX(ctx, tx, notaryID)
		if err == nil {
			// TODO: Support preparation of target operation directly during lock (avoiding extra call)
			_, _, _, err = h.noto.decodeV1LockTransitionWithOutputs(ctx, LOCK_CREATE, senderID, &lockID, req.Inputs, req.Outputs, req.Info)
		}
		if err != nil {
			return nil, err
		}
	}

	// Validate the amounts, and sender's ownership of the inputs and locked outputs
	if err := h.noto.validateLockAmounts(ctx, tx, inputs, outputs); err != nil {
		return nil, err
	}
	if err := h.noto.validateOwners(ctx, tx.Transaction.From, req.ResolvedVerifiers, inputs.coins, inputs.states); err != nil {
		return nil, err
	}
	if err := h.noto.validateLockOwners(ctx, tx.Transaction.From, req.ResolvedVerifiers, outputs.lockedCoins, outputs.lockedStates); err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	encodedLock, err := h.noto.encodeLock(ctx, tx.ContractAddress, inputs.coins, outputs.coins, outputs.lockedCoins)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateSignature(ctx, "sender", req.Signatures, encodedLock); err != nil {
		return nil, err
	}
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *lockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, lockID pldtypes.Bytes32, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inputs := req.InputStates
	outputs, lockedOutputs := h.noto.splitStates(req.OutputStates)

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	lockSignature := domain.FindAttestation("sender", req.AttestationResult)
	if lockSignature == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	data, err := h.noto.encodeTransactionData(ctx, tx.DomainConfig, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}

	var interfaceABI abi.ABI
	var functionName string
	var paramsJSON []byte

	switch tx.DomainConfig.Variant {
	case types.NotoVariantDefault:
		var notoLockOpEncoded []byte
		lockStates := h.noto.filterSchema(req.OutputStates, []string{h.noto.lockInfoSchemaV1.Id})
		notoLockOpEncoded, err = h.noto.encodeNotoLockOperation(ctx, &types.NotoLockOperation{
			TxId:          req.Transaction.TransactionId,
			Inputs:        endorsableStateIDs(inputs),
			Outputs:       append(endorsableStateIDs(lockStates), endorsableStateIDs(outputs)...),
			LockedOutputs: endorsableStateIDs(lockedOutputs),
			Proof:         lockSignature.Payload,
		})
		if err == nil {
			interfaceABI = h.noto.getInterfaceABI(types.NotoVariantDefault)
			functionName = "createLock"
			params := &CreateLockParams{
				CreateInputs: notoLockOpEncoded,
				Params:       LockParams{},
				Data:         data,
			}
			paramsJSON, err = json.Marshal(params)
		}
	default:
		interfaceABI = h.noto.getInterfaceABI(types.NotoVariantLegacy)
		functionName = "lock"
		params := &NotoLock_V0_Params{
			TxId:          req.Transaction.TransactionId,
			Inputs:        endorsableStateIDs(inputs),
			Outputs:       endorsableStateIDs(outputs),
			LockedOutputs: endorsableStateIDs(lockedOutputs),
			Signature:     lockSignature.Payload,
			Data:          data,
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

func (h *lockHandler) hookInvoke(ctx context.Context, lockID pldtypes.Bytes32, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.LockParams)

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &LockHookParams{
		Sender: senderID.address,
		LockID: lockID,
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
		hooksBuild.ABI.Functions()["onLock"],
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

func (h *lockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (_ *prototk.PrepareTransactionResponse, err error) {
	var lockID *pldtypes.Bytes32
	if tx.DomainConfig.IsV0() {
		lockID, _, _, err = h.noto.extractLockInfoV0(ctx, req.InfoStates, true)
		if err != nil {
			return nil, err
		}
	} else {
		var lt *lockTransition
		lt, err = h.noto.validateV1LockTransition(ctx, LOCK_CREATE, nil, nil, req.InputStates, req.OutputStates)
		if err != nil {
			return nil, err
		}
		lockID = &lt.newLockInfo.LockID
	}

	endorsement := domain.FindAttestation("notary", req.AttestationResult)
	if endorsement == nil || endorsement.Verifier.Lookup != tx.DomainConfig.NotaryLookup {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "notary")
	}

	baseTransaction, err := h.baseLedgerInvoke(ctx, tx, *lockID, req)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() {
		hookTransaction, err := h.hookInvoke(ctx, *lockID, tx, req, baseTransaction)
		if err != nil {
			return nil, err
		}
		return hookTransaction.prepare()
	}

	return baseTransaction.prepare()
}
