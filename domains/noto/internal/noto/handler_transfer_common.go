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

type transferCommon struct {
	noto *Noto
}

func (h *transferCommon) validateTransferParams(ctx context.Context, to string, amount *pldtypes.HexUint256) error {
	if to == "" {
		return i18n.NewError(ctx, msgs.MsgParameterRequired, "to")
	}
	if amount == nil || amount.Int().Sign() != 1 {
		return i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}
	return nil
}

func (h *transferCommon) initTransfer(ctx context.Context, tx *types.ParsedTransaction, from, to string) (*prototk.InitTransactionResponse, error) {
	notary := tx.DomainConfig.NotaryLookup

	requests := h.noto.ethAddressVerifiers(notary, tx.Transaction.From, from, to)
	return &prototk.InitTransactionResponse{
		RequiredVerifiers: requests,
	}, nil
}

func (h *transferCommon) assembleTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest, from, to string, amount *pldtypes.HexUint256, data pldtypes.HexBytes) (*prototk.AssembleTransactionResponse, error) {
	notary := tx.DomainConfig.NotaryLookup
	useNullifiers := tx.DomainConfig.IsNullifierVariant()

	notaryID, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", from, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	toID, err := h.noto.findEthAddressVerifier(ctx, "to", to, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	inputStates, revert, err := h.noto.prepareInputs(ctx, req.StateQueryContext, fromID, amount, useNullifiers)
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

	// Avoid duplicating the sender in distribution lists
	outputDistributionList := identityList{notaryID, senderID, fromID, toID} // de-dup done on query
	outputStates, err := h.noto.prepareOutputs(toID, amount, outputDistributionList)
	if err != nil {
		return nil, err
	}
	if useNullifiers {
		for _, newState := range outputStates.states {
			newState.NullifierSpecs = []*prototk.NullifierSpec{
				{
					Party:        to,
					Algorithm:    types.AlgoDomainNullifier(h.noto.name),
					VerifierType: types.VERIFIER_DOMAIN_NOTO_NULLIFIER,
					PayloadType:  types.PAYLOAD_DOMAIN_NOTO_NULLIFIER,
				},
			}
		}
	}

	infoDistribution := identityList{notaryID, senderID, fromID, toID}
	infoStates, err := h.noto.prepareDataInfo(data, tx.DomainConfig.Variant, infoDistribution.identities())
	if err != nil {
		return nil, err
	}

	if inputStates.total.Cmp(amount.Int()) == 1 {
		remainder := big.NewInt(0).Sub(inputStates.total, amount.Int())
		remainderDistributionList := identityList{notaryID, senderID, fromID}
		returnedStates, err := h.noto.prepareOutputs(fromID, (*pldtypes.HexUint256)(remainder), remainderDistributionList)
		if err != nil {
			return nil, err
		}
		if useNullifiers {
			for _, newState := range returnedStates.states {
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
		outputStates.distributions = append(outputStates.distributions, returnedStates.distributions...)
		outputStates.coins = append(outputStates.coins, returnedStates.coins...)
		outputStates.states = append(outputStates.states, returnedStates.states...)
	}

	encodedTransfer, err := h.noto.encodeTransferUnmasked(ctx, tx.ContractAddress, inputStates.coins, outputStates.coins)
	if err != nil {
		return nil, err
	}
	attestation := []*prototk.AttestationRequest{
		// Sender confirms the initial request with a signature
		{
			Name:            "sender",
			AttestationType: prototk.AttestationType_SIGN,
			Algorithm:       algorithms.ECDSA_SECP256K1,
			VerifierType:    verifiers.ETH_ADDRESS,
			Payload:         encodedTransfer,
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

	if !tx.DomainConfig.IsV0() {
		manifestState, err := h.noto.newManifestBuilder().
			addOutputs(outputStates).
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
			OutputStates: outputStates.states,
			InfoStates:   infoStates,
		},
		AttestationPlan: attestation,
	}, nil
}

func (h *transferCommon) endorseTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest, from string) (*prototk.EndorseTransactionResponse, error) {
	inputs, err := h.noto.parseCoinList(ctx, "input", req.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, err := h.noto.parseCoinList(ctx, "output", req.Outputs)
	if err != nil {
		return nil, err
	}

	// Validate the amounts, and sender's ownership of the inputs
	if err := h.noto.validateTransferAmounts(ctx, inputs, outputs); err != nil {
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

func (h *transferCommon) baseLedgerInvokeTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, withApproval bool, useNullifier bool) (*TransactionWrapper, error) {
	useNullifiers := tx.DomainConfig.IsNullifierVariant()
	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	signature := domain.FindAttestation("sender", req.AttestationResult)
	if signature == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	data, err := h.noto.encodeTransactionData(ctx, tx.DomainConfig, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}

	payload := signature.Payload
	if useNullifier {
		encoded, err := h.noto.encodeRootAndSignature(ctx, tx.ContractAddress.String(), req.StateQueryContext, payload)
		if err != nil {
			return nil, err
		}
		payload = encoded
	}

	var interfaceABI abi.ABI
	var functionName string
	var paramsJSON []byte

	switch tx.DomainConfig.Variant {
	case types.NotoVariantDefault, types.NotoVariantNullifier:
		interfaceABI = h.noto.getInterfaceABI(types.NotoVariantDefault)
		functionName = "transfer"
		if withApproval {
			functionName = "transferWithApproval"
		}
		params := &NotoTransferParams{
			TxId:    req.Transaction.TransactionId,
			Inputs:  endorsableStateIDs(req.InputStates, useNullifiers),
			Outputs: endorsableStateIDs(req.OutputStates, false),
			Proof:   payload,
			Data:    data,
		}
		paramsJSON, err = json.Marshal(params)
	default:
		interfaceABI = h.noto.getInterfaceABI(types.NotoVariantLegacy)
		functionName = "transfer"
		params := &NotoTransfer_V0_Params{
			TxId:      req.Transaction.TransactionId,
			Inputs:    endorsableStateIDs(req.InputStates, false),
			Outputs:   endorsableStateIDs(req.OutputStates, false),
			Signature: signature.Payload,
			Data:      data,
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

func (h *transferCommon) hookInvokeTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper, from, to string, amount *pldtypes.HexUint256, data pldtypes.HexBytes) (*TransactionWrapper, error) {
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", req.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromID, err := h.noto.findEthAddressVerifier(ctx, "from", from, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	toID, err := h.noto.findEthAddressVerifier(ctx, "to", to, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &TransferHookParams{
		Sender: senderID.address,
		From:   fromID.address,
		To:     toID.address,
		Amount: amount,
		Data:   data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onTransfer"],
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

func (h *transferCommon) prepareTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, from, to string, amount *pldtypes.HexUint256, data pldtypes.HexBytes) (*prototk.PrepareTransactionResponse, error) {
	useNullifier := tx.DomainConfig.IsNullifierVariant()
	endorsement := domain.FindAttestation("notary", req.AttestationResult)
	if endorsement == nil || endorsement.Verifier.Lookup != tx.DomainConfig.NotaryLookup {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "notary")
	}

	baseTransaction, err := h.baseLedgerInvokeTransfer(ctx, tx, req, false, useNullifier)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() {
		hookTransaction, err := h.hookInvokeTransfer(ctx, tx, req, baseTransaction, from, to, amount, data)
		if err != nil {
			return nil, err
		}
		return hookTransaction.prepare()
	}

	return baseTransaction.prepare()
}
