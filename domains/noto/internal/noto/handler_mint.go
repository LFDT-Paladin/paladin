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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type mintHandler struct {
	noto *Noto
}

func (h *mintHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var mintParams types.MintParams
	if err := json.Unmarshal([]byte(params), &mintParams); err != nil {
		return nil, err
	}
	if mintParams.To == "" {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "to")
	}
	if mintParams.Amount == nil || mintParams.Amount.Int().Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}
	return &mintParams, nil
}

func (h *mintHandler) checkAllowed(ctx context.Context, tx *types.ParsedTransaction, from string) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if !*tx.DomainConfig.Options.Basic.RestrictMint {
		return nil
	}
	if from == tx.DomainConfig.NotaryLookup {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgMintOnlyNotary, tx.DomainConfig.NotaryLookup, from)
}

func (h *mintHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.MintParams)
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx, req.Transaction.From); err != nil {
		return nil, err
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(notary, tx.Transaction.From, params.To),
	}, nil
}

func (h *mintHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.MintParams)
	useNullifiers := tx.DomainConfig.IsNullifierVariant()

	ids, err := resolveIdentities(ctx, h.noto, tx, req, "", params.To)
	if err != nil {
		return nil, err
	}
	notaryID, senderID, toID := ids.notary, ids.sender, ids.to

	outputStates, err := h.noto.prepareOutputs(toID, params.Amount, identityList{notaryID, toID})
	if err != nil {
		return nil, err
	}
	if useNullifiers {
		// for new output states, while we create them, we add the corresponding nullifier to the new state,
		// which will be persisted in the state DB. This allows us to track which states have been spent,
		// because the spending transactions will include the nullifier IDs, rather than the state IDs, in
		// the receipt.
		for _, newState := range outputStates.states {
			// Here the new output state could be for the minter (notaryID) or the receiver (toID).
			// regardless of the owner, the notary always knows about the nullifier. So we always
			// add the nullifier spec for the notary.
			newState.NullifierSpecs = []*prototk.NullifierSpec{
				{
					Party:        notaryID.identifier,
					Algorithm:    types.AlgoDomainNullifier(h.noto.name),
					VerifierType: types.VERIFIER_DOMAIN_NOTO_NULLIFIER,
					PayloadType:  types.PAYLOAD_DOMAIN_NOTO_NULLIFIER,
				},
			}
			// In addition, Paladin also puts the responsibility to generate the nullifier for the states
			// to be owned by the receiver, on the minter. So if the receiver is not the notary, we add
			// another nullifier spec with the distribution to the receiver.
			if toID.identifier != notaryID.identifier {
				newState.NullifierSpecs = append(newState.NullifierSpecs, &prototk.NullifierSpec{
					Party:        toID.identifier,
					Algorithm:    types.AlgoDomainNullifier(h.noto.name),
					VerifierType: types.VERIFIER_DOMAIN_NOTO_NULLIFIER,
					PayloadType:  types.PAYLOAD_DOMAIN_NOTO_NULLIFIER,
				})
			}
		}
	}
	infoDistribution := identityList{notaryID, senderID, toID}
	infoStates, err := h.noto.prepareDataInfo(ctx, params.Data, tx.DomainConfig.Variant, infoDistribution.identities(), tx.Transaction, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedTransfer, err := h.noto.encodeTransferUnmasked(ctx, tx.ContractAddress, nil, outputStates.coins)
	if err != nil {
		return nil, err
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
			OutputStates: outputStates.states,
			InfoStates:   infoStates,
		},
		AttestationPlan: buildEndorsePlan(tx.DomainConfig.NotaryLookup, req.Transaction.From, encodedTransfer),
	}, nil
}

func (h *mintHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.MintParams)
	if err := h.checkAllowed(ctx, tx, req.Transaction.From); err != nil {
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

	// Validate the amounts
	if err := h.noto.validateMintAmounts(ctx, params, inputs, outputs); err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	encodedTransfer, err := h.noto.encodeTransferUnmasked(ctx, tx.ContractAddress, nil, outputs.coins)
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

func (h *mintHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
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

	payload := sender.Payload
	if tx.DomainConfig.IsNullifierVariant() {
		encoded, encErr := h.noto.encodeRootAndSignature(ctx, tx.ContractAddress.String(), req.StateQueryContext, payload)
		if encErr != nil {
			return nil, encErr
		}
		payload = encoded
	}

	interfaceABI := h.noto.getInterfaceABI(tx.DomainConfig.Variant)
	functionName := "mint"
	var paramsJSON []byte

	if tx.DomainConfig.IsV0() {
		paramsJSON, err = json.Marshal(&NotoMint_V0_Params{
			TxId:      req.Transaction.TransactionId,
			Outputs:   endorsableStateIDs(ctx, req.OutputStates, false),
			Signature: sender.Payload,
			Data:      data,
		})
	} else if tx.DomainConfig.IsV1() || tx.DomainConfig.IsV2() {
		paramsJSON, err = json.Marshal(&NotoMintParams{
			TxId:    req.Transaction.TransactionId,
			Outputs: endorsableStateIDs(ctx, req.OutputStates, false),
			Proof:   payload,
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

func (h *mintHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.MintParams)

	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	toID, err := h.noto.findEthAddressVerifier(ctx, "to", inParams.To, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &MintHookParams{
		Sender: senderID.address,
		To:     toID.address,
		Amount: inParams.Amount,
		Data:   inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onMint"],
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

func (h *mintHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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
