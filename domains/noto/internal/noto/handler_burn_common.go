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
)

type burnCommon struct {
	noto *Noto
}

func (h *burnCommon) validateBurnParams(ctx context.Context, amount *pldtypes.HexUint256) error {
	if amount == nil || amount.Int().Sign() != 1 {
		return i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}
	return nil
}

func (h *burnCommon) checkBurnAllowed(ctx context.Context, tx *types.ParsedTransaction) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if *tx.DomainConfig.Options.Basic.AllowBurn {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgBurnNotAllowed)
}

func (h *burnCommon) initBurn(ctx context.Context, tx *types.ParsedTransaction, from string) (*prototk.InitTransactionResponse, error) {
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkBurnAllowed(ctx, tx); err != nil {
		return nil, err
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(notary, tx.Transaction.From, from),
	}, nil
}

func (h *burnCommon) assembleBurn(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest, from string, amount *pldtypes.HexUint256, data pldtypes.HexBytes) (*prototk.AssembleTransactionResponse, error) {
	ids, err := resolveIdentities(ctx, h.noto, tx, req, from, "")
	if err != nil {
		return nil, err
	}
	notaryID, senderID, fromID := ids.notary, ids.sender, ids.from
	useNullifiers := tx.DomainConfig.IsNullifierVariant()

	inputStates, revert, err := h.noto.prepareInputs(ctx, req.StateQueryContext, fromID, amount, useNullifiers)
	if res, err := assembleRevertOrError(revert, err); res != nil || err != nil {
		return res, err
	}
	infoDistribution := identityList{notaryID, senderID, fromID}
	infoStates, err := h.noto.prepareDataInfo(ctx, data, tx.DomainConfig.Variant, infoDistribution.identities(), tx.Transaction, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	outputs := &preparedOutputs{}
	if inputStates.total.Cmp(amount.Int()) == 1 {
		remainder := big.NewInt(0).Sub(inputStates.total, amount.Int())
		outputs, err = h.noto.prepareOutputs(fromID, (*pldtypes.HexUint256)(remainder), identityList{notaryID, senderID, fromID})
		if err != nil {
			return nil, err
		}
		if useNullifiers {
			// add nullifier spec to each returned state (they are new states)
			for _, newState := range outputs.states {
				newState.NullifierSpecs = []*prototk.NullifierSpec{
					{
						Party:        from,
						Algorithm:    types.AlgoDomainNullifier(h.noto.name),
						VerifierType: types.VERIFIER_DOMAIN_NOTO_NULLIFIER,
						PayloadType:  types.PAYLOAD_DOMAIN_NOTO_NULLIFIER,
					},
				}
			}
		}
	}

	encodedTransfer, err := h.noto.encodeTransferUnmasked(ctx, tx.ContractAddress, inputStates.coins, outputs.coins)
	if err != nil {
		return nil, err
	}

	if !tx.DomainConfig.IsV0() {
		manifestState, err := h.noto.newManifestBuilder().
			addOutputs(outputs).
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
			InputStates:  inputStates.states,
			OutputStates: outputs.states,
			InfoStates:   infoStates,
		},
		AttestationPlan: buildEndorsePlan(tx.DomainConfig.NotaryLookup, req.Transaction.From, encodedTransfer),
	}, nil
}

func (h *burnCommon) endorseBurn(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest, from string, amount *pldtypes.HexUint256, data pldtypes.HexBytes) (*prototk.EndorseTransactionResponse, error) {
	if err := h.checkBurnAllowed(ctx, tx); err != nil {
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

	// Validate the amounts, and sender's ownership of the inputs
	if err := h.noto.validateBurnAmounts(ctx, &types.BurnParams{Amount: amount, Data: data}, inputs, outputs); err != nil {
		return nil, err
	}
	if err := h.noto.validateOwners(ctx, from, req.ResolvedVerifiers, inputs.coins, inputs.states); err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	encodedTransfer, err := h.noto.encodeTransferUnmasked(ctx, tx.ContractAddress, inputs.coins, outputs.coins)
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

func (h *burnCommon) baseLedgerInvokeBurn(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, useNullifier bool) (*TransactionWrapper, error) {
	// Include the signature from the sender/notary
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	sender := domain.FindAttestation("sender", req.AttestationResult)
	if sender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	data, err := h.noto.encodeTransactionData(ctx, tx.DomainConfig, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}

	interfaceABI := h.noto.getInterfaceABI(tx.DomainConfig.Variant)
	functionName := "transfer"
	var paramsJSON []byte

	proof := sender.Payload
	if useNullifier {
		encoded, encErr := h.noto.encodeRootAndSignature(ctx, tx.ContractAddress.String(), req.StateQueryContext, proof)
		if encErr != nil {
			return nil, encErr
		}
		proof = encoded
	}

	if tx.DomainConfig.IsV0() {
		paramsJSON, err = json.Marshal(&NotoTransfer_V0_Params{
			TxId:      req.Transaction.TransactionId,
			Inputs:    endorsableStateIDs(ctx, req.InputStates, false),
			Outputs:   endorsableStateIDs(ctx, req.OutputStates, false),
			Signature: sender.Payload,
			Data:      data,
		})
	} else if tx.DomainConfig.IsV1() || tx.DomainConfig.IsV2() {
		paramsJSON, err = json.Marshal(&NotoTransferParams{
			TxId:    req.Transaction.TransactionId,
			Inputs:  endorsableStateIDs(ctx, req.InputStates, useNullifier),
			Outputs: endorsableStateIDs(ctx, req.OutputStates, false),
			Proof:   proof,
			Data:    data,
		})
	} else {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
	}
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		transactionType: prototk.PreparedTransaction_PUBLIC,
		functionABI:     interfaceABI.Functions()[functionName],
		paramsJSON:      paramsJSON,
	}, nil
}

func (h *burnCommon) hookInvokeBurn(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper, from string, amount *pldtypes.HexUint256, data pldtypes.HexBytes) (*TransactionWrapper, error) {
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", from, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &BurnHookParams{
		Sender: senderID.address,
		From:   fromID.address,
		Amount: amount,
		Data:   data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onBurn"],
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

func (h *burnCommon) prepareBurn(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, from string, amount *pldtypes.HexUint256, data pldtypes.HexBytes) (*prototk.PrepareTransactionResponse, error) {
	endorsement := domain.FindAttestation("notary", req.AttestationResult)
	if endorsement == nil || endorsement.Verifier.Lookup != tx.DomainConfig.NotaryLookup {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "notary")
	}

	useNullifier := tx.DomainConfig.IsNullifierVariant()
	baseTransaction, err := h.baseLedgerInvokeBurn(ctx, tx, req, useNullifier)
	if err != nil {
		return nil, err
	}
	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() {
		hookTransaction, err := h.hookInvokeBurn(ctx, tx, req, baseTransaction, from, amount, data)
		if err != nil {
			return nil, err
		}
		return hookTransaction.prepare()
	}
	return baseTransaction.prepare()
}
