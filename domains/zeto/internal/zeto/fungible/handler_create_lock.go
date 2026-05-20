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

package fungible

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	signercommon "github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/signer/common"
	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	pb "github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"google.golang.org/protobuf/proto"
)

var _ types.DomainHandler = &createLockHandler{}

type createLockHandler struct {
	baseHandler
	callbacks plugintk.DomainCallbacks
}

// zetoCreateLockArgsTupleABI matches ethers AbiCoder.encode(["tuple(...)"], [args]) used in zeto_anon.ts encodeCreateArgs.
// It must be a single wrapped tuple (not five top-level ABI params) so the payload starts with the 0x20 offset word
// Solidity expects when abi.decode(createArgs, (ZetoCreateLockArgs)).
var zetoCreateLockArgsTupleABI = abi.ParameterArray{
	{
		Type:         "tuple",
		InternalType: "struct ZetoCreateLockArgs",
		Components: abi.ParameterArray{
			{Name: "txId", Type: "bytes32"},
			{Name: "inputs", Type: "uint256[]"},
			{Name: "outputs", Type: "uint256[]"},
			{Name: "lockedOutputs", Type: "uint256[]"},
			{Name: "proof", Type: "bytes"},
		},
	},
}

// zetoCreateLockArgsWireJSON is the inner struct JSON for createArgs; EncodeABIDataJSONCtx is called with
// json.Marshal([]any{wire}) like NotoCreateLockArgsABI_V1 (see domains/noto/internal/noto/noto.go).
type zetoCreateLockArgsWireJSON struct {
	TxID          string   `json:"txId"`
	Inputs        []string `json:"inputs"`
	Outputs       []string `json:"outputs"`
	LockedOutputs []string `json:"lockedOutputs"`
	Proof         string   `json:"proof"`
}

func NewCreateLockHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, lockInfoSchema *pb.StateSchema) *createLockHandler {
	return &createLockHandler{
		baseHandler: baseHandler{
			name: name,
			stateSchemas: &common.StateSchemas{
				CoinSchema:           coinSchema,
				MerkleTreeRootSchema: merkleTreeRootSchema,
				MerkleTreeNodeSchema: merkleTreeNodeSchema,
				LockInfoSchema:       lockInfoSchema,
			},
		},
		callbacks: callbacks,
	}
}

// createLockRecipientsTotal returns the sum of recipients' amounts (locked value).
func createLockRecipientsTotal(p *types.CreateLockParams) *big.Int {
	sum := big.NewInt(0)
	if p == nil {
		return sum
	}
	for _, r := range p.Recipients {
		if r != nil && r.Amount != nil {
			sum.Add(sum, r.Amount.Int())
		}
	}
	return sum
}

func (h *createLockHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var p types.CreateLockParams
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalLockParams, err)
	}
	if strings.TrimSpace(p.From) == "" {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "parameter 'from' is required")
	}
	if len(p.Recipients) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "parameter 'recipients' is required")
	}
	if err := validateTransferParams(ctx, p.Recipients); err != nil {
		return nil, err
	}
	sum := createLockRecipientsTotal(&p)
	if sum.Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParamTotalAmountInRange)
	}
	if sum.Cmp(MAX_TRANSFER_AMOUNT) >= 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParamTotalAmountInRange)
	}
	return &p, nil
}

func (h *createLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	// Always resolve the Paladin sender's ETH address alongside BJJ: lock metadata, packed calldata, and
	// createLockPublicEthSender fallbacks depend on ECDSA/ETH being present in ResolvedVerifiers.
	rv := []*prototk.ResolveVerifierRequest{
		{
			Lookup:       tx.Transaction.From,
			Algorithm:    h.getAlgoZetoSnarkBJJ(),
			VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		},
		{
			Lookup:       tx.Transaction.From,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}
	// we need to construct the outputs for the intended recipients, as the lock info state will be constructed later
	// and the recipients will be pinned in the lock info state
	params := tx.Params.(*types.CreateLockParams)
	for _, recipient := range params.Recipients {
		rv = append(rv, &prototk.ResolveVerifierRequest{
			Lookup:       recipient.To,
			Algorithm:    h.getAlgoZetoSnarkBJJ(),
			VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		})
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: rv,
	}, nil
}

func (h *createLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.CreateLockParams)
	resolvedSender := domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	lockedTotal := createLockRecipientsTotal(params)
	lockedTotalHex := pldtypes.HexUint256(*lockedTotal)
	inputs, revert, err := buildInputsForExpectedTotal(ctx, h.callbacks, h.stateSchemas.CoinSchema, useNullifiers, req.StateQueryContext, resolvedSender.Verifier, lockedTotal, false)
	if err != nil {
		if revert {
			message := err.Error()
			return &prototk.AssembleTransactionResponse{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   &message,
			}, nil
		}
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxInputs, err)
	}
	inputCoins := inputs.coins
	inputStates := inputs.states

	var remainderOutputCoins []*types.ZetoCoin
	var remainderOutputStates []*pb.NewState
	var assembledOutputStates []*pb.NewState
	var assembledInfoStates []*pb.NewState
	remainderAmount := big.NewInt(0).Sub(inputs.total, lockedTotal)
	if remainderAmount.Sign() > 0 {
		remainderOutputEntries := []*types.FungibleTransferParamEntry{
			{
				To:     tx.Transaction.From,
				Amount: pldtypes.Uint64ToUint256(remainderAmount.Uint64()),
			},
		}
		remainderOutputCoins, remainderOutputStates, err = prepareOutputsForTransfer(ctx, useNullifiers, remainderOutputEntries, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
		}
		assembledOutputStates = append(assembledOutputStates, remainderOutputStates...)
	}

	// we create a single locked output for the sender themselves,
	// using the total amount of the intended entries for the recipients
	lockedOutputEntries := []*types.FungibleTransferParamEntry{
		{
			To:     tx.Transaction.From,
			Amount: &lockedTotalHex,
		},
	}
	lockedOutputCoins, lockedOutputStates, err := prepareOutputsForTransfer(ctx, useNullifiers, lockedOutputEntries, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name, true)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}
	// Locked collateral is spent by commitment on spendLock (ZetoLockSpent lockedInputs), not via the
	// unlocked-UTXO nullifier tree — do not register nullifiers here or spendLock cannot retire the input.
	clearNullifierSpecs(lockedOutputStates)
	assembledOutputStates = append(assembledOutputStates, lockedOutputStates...)

	contractAddress, err := pldtypes.ParseEthAddress(req.Transaction.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}
	// construct the proposed spend states for the recipients,
	// these will be used to construct the spendCommitment for the lock
	proposedSpendRecipients := make([]*types.FungibleTransferParamEntry, 0, len(params.Recipients))
	for _, r := range params.Recipients {
		if r == nil {
			continue
		}
		entry := *r
		entry.To = qualifyPartyLookup(entry.To, tx.Transaction.From)
		proposedSpendRecipients = append(proposedSpendRecipients, &entry)
	}
	var proposedSpendCoins []*types.ZetoCoin
	var proposedSpendStates []*pb.NewState
	proposedSpendCoins, proposedSpendStates, err = prepareOutputsForTransfer(ctx, useNullifiers, proposedSpendRecipients, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}
	// Proposed spend states (coin schema) are InfoStates until ZetoLockSpent; their Paladin state ids are
	// recorded in lock info SpendOutputs below. Lock info itself is an OutputState (lock-info schema).
	// No NullifierSpecs here — info states are not in creatingStates (PD010126); spendLock registers them.
	clearNullifierSpecs(proposedSpendStates)
	assembledInfoStates = append(assembledInfoStates, proposedSpendStates...)

	// Pre-pin cancel path: spend locked collateral back to the owner as unlocked outputs (cancelLock).
	proposedCancelOutputEntries := []*types.FungibleTransferParamEntry{
		{
			To:     tx.Transaction.From,
			Amount: &lockedTotalHex,
		},
	}
	var proposedCancelCoins []*types.ZetoCoin
	var proposedCancelStates []*pb.NewState
	proposedCancelCoins, proposedCancelStates, err = prepareOutputsForTransfer(ctx, useNullifiers, proposedCancelOutputEntries, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}
	clearNullifierSpecs(proposedCancelStates)
	assembledInfoStates = append(assembledInfoStates, proposedCancelStates...)
	if len(proposedSpendCoins) != len(proposedSpendStates) || len(proposedCancelCoins) != len(proposedCancelStates) {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "proposed spend/cancel coins and states length mismatch")
	}

	// Persist lock info under the lock-info schema (6th AbiStateSchema). spendLock / cancelLock load
	// lockedOutputs plus spendOutputs/spendData or cancelOutputs/cancelData from this row
	txID32, err := common.ParseTransactionIDBytes32(ctx, req.Transaction.TransactionId)
	if err != nil {
		return nil, err
	}
	resolvedEth := domain.FindVerifier(tx.Transaction.From, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if resolvedEth == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}
	resolvedEthAddr, err := pldtypes.ParseEthAddress(resolvedEth.Verifier)
	if err != nil {
		return nil, err
	}

	// compute the lock id for the lock using the same logic as the onchain Zeto contract
	lockID := types.ComputeZetoLockIDV1(*contractAddress, *resolvedEthAddr, txID32)
	lockedOutputCommitmentStrs := make([]string, 0, len(lockedOutputCoins))
	for _, coin := range lockedOutputCoins {
		hh, err := coin.Hash(ctx)
		if err != nil {
			return nil, err
		}
		lockedOutputCommitmentStrs = append(lockedOutputCommitmentStrs, common.HexUint256To32ByteHexString(hh))
	}
	proposedSpendStateIDs := make([]string, 0, len(proposedSpendStates))
	for _, st := range proposedSpendStates {
		if st.Id != nil {
			proposedSpendStateIDs = append(proposedSpendStateIDs, *st.Id)
		}
	}
	salt := crypto.NewSalt()
	proposedSpendDataJSON, err := recipientsForLockInfoJSON(params, tx.Transaction.From)
	if err != nil {
		return nil, err
	}
	proposedCancelStateIDs := make([]string, 0, len(proposedCancelStates))
	for _, st := range proposedCancelStates {
		if st.Id != nil {
			proposedCancelStateIDs = append(proposedCancelStateIDs, *st.Id)
		}
	}
	proposedCancelDataJSON, err := cancelRecipientsForLockInfoJSON(tx.Transaction.From, &lockedTotalHex)
	if err != nil {
		return nil, err
	}
	lockInfo := &types.ZetoLockInfoState{
		Salt:          (*pldtypes.HexUint256)(salt),
		LockID:        lockID,
		Owner:         resolvedEth.Verifier,
		Spender:       resolvedEthAddr.String(),
		LockedOutputs: lockedOutputCommitmentStrs,
		SpendOutputs:  proposedSpendStateIDs,
		SpendData:     append(pldtypes.HexBytes(nil), proposedSpendDataJSON...),
		CancelOutputs: proposedCancelStateIDs,
		CancelData:    append(pldtypes.HexBytes(nil), proposedCancelDataJSON...),
		UnlockData:    append(pldtypes.HexBytes(nil), params.UnlockData...),
	}
	lockInfoState, err := makeNewLockInfoState(ctx, h.stateSchemas.LockInfoSchema, lockInfo, []string{tx.Transaction.From})
	if err != nil {
		return nil, err
	}
	// Lock info is an off-chain output state (not a coin UTXO). Prepare skips non-coin outputs for proof encoding.
	assembledOutputStates = append(assembledOutputStates, lockInfoState)

	proofOutputCoins := lockTransitionOutputCoinsForProof(tx.DomainConfig.ZetoVariant, remainderOutputCoins, lockedOutputCoins)
	// createLock spends unlocked inputs, using the proof circuit for the transfer method
	circuit := (*tx.DomainConfig.Circuits)[types.METHOD_TRANSFER]
	payloadBytes, err := formatTransferProvingRequest(ctx, h.callbacks, h.stateSchemas.MerkleTreeRootSchema, h.stateSchemas.MerkleTreeNodeSchema, signercommon.GetHasher(), inputCoins, proofOutputCoins, circuit, tx.DomainConfig.TokenName, req.StateQueryContext, contractAddress, false)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFormatProvingReq, err)
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			InputStates:  inputStates,
			OutputStates: assembledOutputStates,
			InfoStates:   assembledInfoStates,
		},
		AttestationPlan: []*prototk.AttestationRequest{
			{
				Name:            "sender",
				AttestationType: pb.AttestationType_SIGN,
				Algorithm:       h.getAlgoZetoSnarkBJJ(),
				VerifierType:    zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
				PayloadType:     zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK,
				Payload:         payloadBytes,
				Parties:         []string{tx.Transaction.From},
			},
		},
	}, nil
}

func (h *createLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *createLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	var proofRes corepb.ProvingResponse
	result := domain.FindAttestation("sender", req.AttestationResult)
	if result == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindSenderAttestation)
	}
	if err := proto.Unmarshal(result.Payload, &proofRes); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalProvingRes, err)
	}

	inputSize := common.GetInputSize(len(req.InputStates))
	inputs, err := utxosFromInputStates(ctx, req.InputStates, inputSize)
	if err != nil {
		return nil, err
	}

	var remainderOutputStates []*pb.EndorsableState
	var lockedOutputStates []*pb.EndorsableState
	coinSchemaID := h.stateSchemas.CoinSchema.Id
	for _, state := range req.OutputStates {
		// createLock may append a lock-info state (different schema); do not unmarshal it as ZetoCoin — shared JSON
		// keys like "owner" would decode an ETH address as coin.owner (20 bytes) and break BJJ pubkey parsing.
		if sid := state.GetSchemaId(); sid != "" && sid != coinSchemaID {
			continue
		}
		var coin types.ZetoCoin
		if err := json.Unmarshal([]byte(state.StateDataJson), &coin); err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
		}
		if coin.Locked {
			lockedOutputStates = append(lockedOutputStates, state)
		} else {
			remainderOutputStates = append(remainderOutputStates, state)
		}
	}

	remainderOutputs, err := utxosFromOutputStates(ctx, remainderOutputStates, inputSize)
	if err != nil {
		return nil, err
	}
	remainderOutputs = trimZeroUtxos(remainderOutputs)

	lockedOutputs, err := utxosFromOutputStates(ctx, lockedOutputStates, inputSize)
	if err != nil {
		return nil, err
	}
	lockedOutputs = trimZeroUtxos(lockedOutputs)

	data, err := common.EncodeTransactionData(ctx, req.Transaction, nil)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}
	clParams := tx.Params.(*types.CreateLockParams)
	fnABI := types.LockableCapabilityCreateLockABI
	var prepParams map[string]any
	proofBytes, err := common.EncodeZetoOnchainTransferProofBytes(ctx, tx.DomainConfig.TokenName, proofRes.Proof, proofRes.PublicInputs, false)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
	}
	inCol := inputs
	if common.IsNullifiersToken(tx.DomainConfig.TokenName) {
		inCol = strings.Split(proofRes.PublicInputs["nullifiers"], ",")
	}
	txID32, err := pldtypes.ParseBytes32Ctx(ctx, req.Transaction.TransactionId)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgErrorParseTxId)
	}
	createArgsWire := zetoCreateLockArgsWireJSON{
		TxID:          txID32.HexString0xPrefix(),
		Inputs:        inCol,
		Outputs:       remainderOutputs,
		LockedOutputs: lockedOutputs,
		Proof:         pldtypes.HexBytes(proofBytes).HexString0xPrefix(),
	}
	createArgsJSON, err := json.Marshal([]any{createArgsWire})
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
	}
	createArgsBytes, err := zetoCreateLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, createArgsJSON)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
	}
	// ILockableCapability.createLock outer `data`: opaque unlockData from the request + Paladin tx metadata (events).
	// ZetoCreateLockArgs live only in createArgs; do not duplicate them here.
	outerData := append(pldtypes.HexBytes(nil), clParams.UnlockData...)
	outerData = append(outerData, data...)
	// Zero spend/cancel commitments = unrestricted (ZetoLockable._consumeLock skips hash check). Binding
	// spend/cancel to recipient-specific output commitments requires the same preimage as a future spendLock
	// proof assembly (salt-dependent);
	// TODO: wire that when the domain pins deterministic spend preimages.
	var zeroCommit pldtypes.Bytes32
	prepParams = map[string]any{
		"createArgs":       pldtypes.HexBytes(createArgsBytes).HexString0xPrefix(),
		"spendCommitment":  zeroCommit.HexString0xPrefix(),
		"cancelCommitment": zeroCommit.HexString0xPrefix(),
		"data":             pldtypes.HexBytes(outerData).HexString0xPrefix(),
	}

	paramsJSON, err := json.Marshal(prepParams)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalPrepedParams, err)
	}
	functionJSON, err := json.Marshal(fnABI)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
			// the new lock design is specific about the createLock caller, so we need to specify the signer
			RequiredSigner: &req.Transaction.From,
		},
	}, nil
}
