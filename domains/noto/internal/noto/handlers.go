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
	"math/big"

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
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

// NotoDomainHandler is Noto's transaction handler contract. It narrows the toolkit's
// domain.DomainHandler[types.NotoParsedConfig] in one respect: the phases whose proto
// response can carry a revert return NotoDomainError instead of error, so a handler cannot
// compile without saying whose fault each of its failures is. Init and Prepare have no
// revert in their proto responses, so they return a plain error and any classification
// their callees made is deliberately dropped.
type NotoDomainHandler interface {
	ValidateParams(ctx context.Context, domainConfig *types.NotoParsedConfig, paramsJson string) (any, NotoDomainError)
	Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error)
	Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, NotoDomainError)
	Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, NotoDomainError)
	Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error)
}

// NotoDomainCallHandler is Noto's read-only call handler contract. Calls have no revert in their proto
// responses but ValidateParams needs to match ValidateParams on NotoDomainHandler so that they can both implement
// ParamValidator, so call handler ValidateParams functions do classify errors, even if the result is discarded.
// TODO: why doesn't Paladin accept a revert response for calls?
type NotoDomainCallHandler interface {
	ValidateParams(ctx context.Context, domainConfig *types.NotoParsedConfig, paramsJson string) (any, NotoDomainError)
	InitCall(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitCallRequest) (*prototk.InitCallResponse, error)
	ExecCall(ctx context.Context, tx *types.ParsedTransaction, req *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error)
}

func (n *Noto) GetHandler(method string) NotoDomainHandler {
	switch method {
	case "mint":
		return &mintHandler{noto: n}
	case "transfer":
		return &transferHandler{transferCommon: transferCommon{noto: n}}
	case "transferFrom":
		return &transferFromHandler{transferCommon: transferCommon{noto: n}}
	case "burn":
		return &burnHandler{burnCommon: burnCommon{noto: n}}
	case "burnFrom":
		return &burnFromHandler{burnCommon: burnCommon{noto: n}}
	case "lock", "createLock":
		return &lockHandler{noto: n}
	case "unlock":
		return &unlockHandler{unlockCommon: unlockCommon{lockCommon: lockCommon{noto: n}}}
	case "createTransferLock":
		return &createTransferLockHandler{lockCommon: lockCommon{noto: n}}
	case "createMintLock":
		return &createMintLockHandler{lockCommon: lockCommon{noto: n}}
	case "createBurnLock":
		return &createBurnLockHandler{lockCommon: lockCommon{noto: n}}
	case "prepareUnlock":
		return &prepareUnlockHandler{unlockCommon: unlockCommon{lockCommon: lockCommon{noto: n}}}
	case "prepareMintUnlock":
		return &prepareMintUnlockHandler{lockCommon: lockCommon{noto: n}}
	case "prepareBurnUnlock":
		return &prepareBurnUnlockHandler{lockCommon: lockCommon{noto: n}}
	case "delegateLock":
		return &delegateLockHandler{noto: n}
	default:
		return nil
	}
}

func (n *Noto) GetCallHandler(method string) NotoDomainCallHandler {
	switch method {
	case "name":
		return &nameHandler{noto: n}
	case "symbol":
		return &symbolHandler{noto: n}
	case "decimals":
		return &decimalsHandler{noto: n}
	case "balanceOf":
		return &balanceOfHandler{noto: n}
	default:
		return nil
	}
}

// Check that a mint has no inputs, and an output matching the requested amount
func (n *Noto) validateMintAmounts(ctx context.Context, params *types.MintParams, inputs, outputs *parsedCoins) NotoDomainError {
	if len(inputs.coins) > 0 {
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidInputs, "mint", inputs.coins))
	}
	if outputs.total.Cmp(params.Amount.Int()) != 0 {
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidAmount, "mint", params.Amount.Int().Text(10), outputs.total.Text(10)))
	}
	return nil
}

// Check that a transfer has at least one input and output, and they net out to zero
func (n *Noto) validateTransferAmounts(ctx context.Context, inputs, outputs *parsedCoins) NotoDomainError {
	if len(inputs.coins) == 0 {
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidInputs, "transfer", inputs.coins))
	}
	if inputs.total.Cmp(outputs.total) != 0 {
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidAmount, "transfer", inputs.total, outputs.total))
	}
	return nil
}

// Check that a burn has at least one input, and a net output matching the requested amount
func (n *Noto) validateBurnAmounts(ctx context.Context, params *types.BurnParams, inputs, outputs *parsedCoins) NotoDomainError {
	if len(inputs.coins) == 0 {
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidInputs, "burn", inputs.coins))
	}
	amount := big.NewInt(0).Sub(inputs.total, outputs.total)
	if amount.Cmp(params.Amount.Int()) != 0 {
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidAmount, "burn", params.Amount.Int().Text(10), amount.Text(10)))
	}
	return nil
}

// Check that a lock produces locked coins matching the difference between the inputs and outputs
func (n *Noto) validateLockAmounts(ctx context.Context, tx *types.ParsedTransaction, inputs, outputs *parsedCoins) NotoDomainError {
	if tx.DomainConfig.IsV0() && len(inputs.coins) == 0 {
		// V0 did not support empty locks
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidInputs, "lock", inputs.coins))
	}
	amount := big.NewInt(0).Sub(inputs.total, outputs.total)
	if amount.Cmp(outputs.lockedTotal) != 0 {
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidAmount, "lock", outputs.lockedTotal.Text(10), amount.Text(10)))
	}
	return nil
}

// Check that an unlock produces unlocked coins matching the difference between the locked inputs and outputs
// Note that mint & burn uses a different function (this is only used for transfers)
func (n *Noto) validateUnlockAmounts(ctx context.Context, tx *types.ParsedTransaction, inputs, outputs *parsedCoins) NotoDomainError {
	if tx.DomainConfig.IsV0() && len(inputs.lockedCoins) == 0 {
		// In V0 there was no lock object to check
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidInputs, "unlock", inputs.lockedCoins))
	}
	amount := big.NewInt(0).Sub(inputs.lockedTotal, outputs.lockedTotal)
	if amount.Cmp(outputs.total) != 0 {
		return validationErr(i18n.NewError(ctx, msgs.MsgInvalidAmount, "unlock", outputs.total.Text(10), amount.Text(10)))
	}
	return nil
}

// Check that no two coins in the transaction derive the same nullifier.
//
// The nullifier derivation covers every field of a coin, so a collision means a duplicate
// coin - which is already rejected by the base ledger and the state store. This check is
// belt and braces: it catches any regression in the derivation, and turns what would be a
// base ledger revert (or worse, an unspendable coin) into a clear endorsement failure.
//
// Both inputs and outputs are checked as one set, because an output that collides with an
// input is nullified by the very transaction that creates it.
func (n *Noto) validateDistinctNullifiers(ctx context.Context, contract *pldtypes.EthAddress, stateLists ...[]*prototk.EndorsableState) NotoDomainError {
	nullifiers := make(map[string]string) // nullifier -> first state ID that derived it
	seenStates := make(map[string]bool)
	for _, states := range stateLists {
		for _, state := range states {
			if seenStates[state.Id] {
				// The same state appearing twice is checked separately (see parseCoinList)
				continue
			}
			seenStates[state.Id] = true

			nullifier, isCoin, err := n.stateNullifier(ctx, contract, state)
			if err != nil {
				return validationErr(err)
			}
			if !isCoin {
				// Identified on-chain by ID, so it has no nullifier
				continue
			}
			if existing, found := nullifiers[nullifier]; found {
				return validationErr(i18n.NewError(ctx, msgs.MsgDuplicateNullifierInList, existing, state.Id, nullifier))
			}
			nullifiers[nullifier] = state.Id
		}
	}
	return nil
}

// Check that every new unlocked coin carries the nullifier spec that makes it spendable.
//
// Only unlocked coins are nullified: locked coins and lock info states are spent by ID, so they
// are skipped. Note the state data is deliberately not included in the error - it holds the
// owner and amount.
func (n *Noto) validateAssembledNullifierSpecs(ctx context.Context, contract *pldtypes.EthAddress, assembled *prototk.AssembledTransaction) NotoDomainError {
	// all errors are internal errors as this function only operates on nullifiers inside a
	// prototk.AssembledTransaction that this node has just built
	if assembled == nil || n.coinSchema == nil {
		return nil
	}
	expectedPayloadType := types.NullifierPayloadType(contract)
	for _, states := range [][]*prototk.NewState{assembled.OutputStates, assembled.InfoStates} {
		for i, state := range states {
			if state.SchemaId != n.coinSchema.Id {
				continue
			}
			if len(state.NullifierSpecs) == 0 {
				return internalErr(i18n.NewError(ctx, msgs.MsgMissingNullifierSpec, i))
			}
			// The spec must name this contract, or the owner's node would derive a nullifier
			// bound to a different one - which the base ledger here would never recognise
			for _, spec := range state.NullifierSpecs {
				if spec.PayloadType != expectedPayloadType {
					return internalErr(i18n.NewError(ctx, msgs.MsgNullifierWrongContract, i, expectedPayloadType, spec.PayloadType))
				}
			}
		}
	}
	return nil
}

// Check that the originator of a transaction provided a signature on the input details
func (n *Noto) validateSignature(ctx context.Context, name string, attestations []*prototk.AttestationResult, encodedMessage []byte) NotoDomainError {
	signature := domain.FindAttestation(name, attestations)
	if signature == nil {
		return validationErr(i18n.NewError(ctx, msgs.MsgAttestationNotFound, name))
	}
	recoveredSignature, err := n.recoverSignature(ctx, encodedMessage, signature.Payload)
	if err != nil {
		// The signature was supplied by the originator, so one that will not recover is the
		// a validation error
		return validationErr(err)
	}
	if recoveredSignature.String() != signature.Verifier.Verifier {
		return validationErr(i18n.NewError(ctx, msgs.MsgSignatureDoesNotMatch, name, signature.Verifier.Verifier, recoveredSignature.String()))
	}
	return nil
}

// Check that all coins are owned by the transaction sender
func (n *Noto) validateOwners(ctx context.Context, owner string, verifiers []*prototk.ResolvedVerifier, coins []*types.NotoCoin, states []*prototk.StateRef) NotoDomainError {
	fromAddress, err := n.findEthAddressVerifier(ctx, "from", owner, verifiers)
	if err != nil {
		return err
	}

	for i, coin := range coins {
		if !coin.Owner.Equals(fromAddress.address) {
			return validationErr(i18n.NewError(ctx, msgs.MsgStateWrongOwner, states[i].Id, owner))
		}
	}
	return nil
}

// Check that all locked coins are owned by the transaction sender
func (n *Noto) validateLockOwners(ctx context.Context, owner string, verifiers []*prototk.ResolvedVerifier, coins []*types.NotoLockedCoin, states []*prototk.StateRef) NotoDomainError {
	fromAddress, err := n.findEthAddressVerifier(ctx, "from", owner, verifiers)
	if err != nil {
		return err
	}
	for i, coin := range coins {
		if !coin.Owner.Equals(fromAddress.address) {
			return validationErr(i18n.NewError(ctx, msgs.MsgStateWrongOwner, states[i].Id, owner))
		}
	}
	return nil
}

// findEthAddressVerifier parses a resolved verifier as an eth address.
//
// Both failures are endorse-only reverts, because the resolved verifier list has a different
// owner in each phase. On the originator this node resolved the list from the verifiers the
// handler's own Init declared, and resolution is all-or-nothing, so a lookup missing from it
// means Init and Assemble disagree about what the transaction needs - a bug in Noto rather
// than anything the application did. At endorse, a missing lookup means the assembly is invalid
// and the error is a revert.
func (n *Noto) findEthAddressVerifier(ctx context.Context, errorDescription, lookup string, verifierList []*prototk.ResolvedVerifier) (*identityPair, NotoDomainError) {
	verifier := domain.FindVerifier(lookup, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, verifierList)
	if verifier == nil {
		return nil, endorseOnlyValidationErr(i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, errorDescription))
	}
	address, err := pldtypes.ParseEthAddress(verifier.Verifier)
	if err != nil {
		return nil, endorseOnlyValidationErr(err)
	}
	return &identityPair{identifier: lookup, address: address}, nil
}

type TransactionWrapper struct {
	transactionType prototk.PreparedTransaction_TransactionType
	functionABI     *abi.Entry
	paramsJSON      []byte
	contractAddress *pldtypes.EthAddress
}

func (tw *TransactionWrapper) prepare() (*prototk.PrepareTransactionResponse, error) {
	functionJSON, err := json.Marshal(tw.functionABI)
	if err != nil {
		return nil, err
	}
	var contractAddress *string
	if tw.contractAddress != nil {
		addr := tw.contractAddress.String()
		contractAddress = &addr
	}
	res := &prototk.PrepareTransactionResponse{
		Transaction: &prototk.PreparedTransaction{
			Type:            tw.transactionType,
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(tw.paramsJSON),
			ContractAddress: contractAddress,
		},
	}
	return res, nil
}

func (tw *TransactionWrapper) encode(ctx context.Context) ([]byte, error) {
	return tw.functionABI.EncodeCallDataJSONCtx(ctx, tw.paramsJSON)
}

type resolvedIdentities struct {
	notary *identityPair
	sender *identityPair
	from   *identityPair
	to     *identityPair
}

// resolveIdentities resolves notary and sender from the transaction, plus optional from/to lookups.
func resolveIdentities(ctx context.Context, n *Noto, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest, fromLookup, toLookup string) (*resolvedIdentities, NotoDomainError) {
	notaryID, err := n.findEthAddressVerifier(ctx, "notary", tx.DomainConfig.NotaryLookup, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	senderID, err := n.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	ids := &resolvedIdentities{notary: notaryID, sender: senderID}
	if fromLookup != "" {
		ids.from, err = n.findEthAddressVerifier(ctx, "from", fromLookup, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
	}
	if toLookup != "" {
		ids.to, err = n.findEthAddressVerifier(ctx, "to", toLookup, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// buildEndorsePlan returns the standard Noto attestation plan:
// sender signs the payload, notary endorses.
func buildEndorsePlan(notaryParty, senderParty string, signPayload []byte) []*prototk.AttestationRequest {
	return []*prototk.AttestationRequest{
		{
			Name:            "sender",
			AttestationType: prototk.AttestationType_SIGN,
			Algorithm:       algorithms.ECDSA_SECP256K1,
			VerifierType:    verifiers.ETH_ADDRESS,
			Payload:         signPayload,
			PayloadType:     signpayloads.OPAQUE_TO_RSV,
			Parties:         []string{senderParty},
		},
		{
			Name:            "notary",
			AttestationType: prototk.AttestationType_ENDORSE,
			Algorithm:       algorithms.ECDSA_SECP256K1,
			VerifierType:    verifiers.ETH_ADDRESS,
			Parties:         []string{notaryParty},
		},
	}
}

// assembleRevertOrError turns a classified assemble failure into either a terminal REVERT response
// carrying the reason, or an error for the platform to retry.
func assembleRevertOrError(err NotoDomainError) (*prototk.AssembleTransactionResponse, error) {
	if err == nil {
		return nil, nil
	}
	if err.IsAssembleRevert() {
		reason := err.Error()
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   &reason,
		}, nil
	}
	return nil, err
}

// endorseRevertOrError turns a classified endorse failure into either a terminal REVERT response
// carrying the reason, or an error for the platform to retry.
func endorseRevertOrError(err NotoDomainError) (*prototk.EndorseTransactionResponse, error) {
	if err == nil {
		return nil, nil
	}
	if err.IsEndorseRevert() {
		reason := err.Error()
		return &prototk.EndorseTransactionResponse{
			EndorsementResult: prototk.EndorseTransactionResponse_REVERT,
			RevertReason:      &reason,
		}, nil
	}
	return nil, err
}
