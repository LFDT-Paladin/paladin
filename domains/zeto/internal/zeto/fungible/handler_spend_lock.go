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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	pb "github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"google.golang.org/protobuf/proto"
)

var _ types.DomainHandler = &spendLockHandler{}

type spendLockHandler struct {
	baseHandler
	callbacks plugintk.DomainCallbacks
}

// zetoSpendLockArgsTupleABI matches ethers AbiCoder.encode(["tuple(...)"], [args]) for spendArgs; it must be a
// single wrapped tuple (not top-level ABI params) so the payload starts with the 0x20 offset word Solidity
// expects when abi.decode(spendArgs, (IZetoLockableCapability.ZetoSpendLockArgs)) (see zetoCreateLockArgsTupleABI / zeto_anon.ts).
//
// Components match IZetoLockableCapability.ZetoSpendLockArgs in upstream zeto
// solidity/contracts/lib/interfaces/IZetoLockableCapability.sol (txId, lockedOutputs, outputs, proof, data).
// lockedInputs are not in this struct; the contract reads them from lock storage (getLockedInputs / getLockContent).
var zetoSpendLockArgsTupleABI = abi.ParameterArray{
	{
		Type:         "tuple",
		InternalType: "struct IZetoLockableCapability.ZetoSpendLockArgs",
		Components: abi.ParameterArray{
			{Name: "txId", Type: "bytes32"},
			{Name: "lockedOutputs", Type: "uint256[]"},
			{Name: "outputs", Type: "uint256[]"},
			{Name: "proof", Type: "bytes"},
			{Name: "data", Type: "bytes"},
		},
	},
}

// zetoSpendLockArgsWireJSON is the inner struct for spendArgs before json.Marshal([]any{wire}) + EncodeABIDataJSONCtx.
type zetoSpendLockArgsWireJSON struct {
	TxID          string   `json:"txId"`
	LockedOutputs []string `json:"lockedOutputs"`
	Outputs       []string `json:"outputs"`
	Proof         string   `json:"proof"`
	Data          string   `json:"data"`
}

func NewSpendLockHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema, lockInfoSchema *pb.StateSchema) *spendLockHandler {
	return &spendLockHandler{
		baseHandler: baseHandler{
			name: name,
			stateSchemas: &common.StateSchemas{
				CoinSchema:           coinSchema,
				MerkleTreeRootSchema: merkleTreeRootSchema,
				MerkleTreeNodeSchema: merkleTreeNodeSchema,
				DataSchema:           dataSchema,
				LockInfoSchema:       lockInfoSchema,
			},
		},
		callbacks: callbacks,
	}
}

func (h *spendLockHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var p types.SpendLockParams
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return nil, err
	}

	if err := validateSpendLockParams(ctx, &p); err != nil {
		return nil, err
	}

	return &p, nil
}

func (h *spendLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	res := &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}

	return res, nil
}

func (h *spendLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.SpendLockParams)

	if domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers) == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	if h.stateSchemas.LockInfoSchema == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMissingLockInputs)
	}
	li, err := h.loadLockInfoByLockID(ctx, params.LockId, req.StateQueryContext)
	if err != nil {
		return nil, err
	}
	if li == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoNotFound, params.LockId.HexString0xPrefix())
	}
	if len(li.LockedOutputs) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoEmptyLockedInputs, params.LockId.HexString0xPrefix())
	}

	delegateLookup := strings.TrimSpace(li.Spender)
	if delegateLookup == "" {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "persisted lock info is missing spender")
	}
	var delegateAddr string
	if _, parseErr := pldtypes.ParseEthAddress(delegateLookup); parseErr != nil {
		resolvedDelegate := domain.FindVerifier(delegateLookup, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
		if resolvedDelegate == nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, delegateLookup)
		}
		delegateAddr = resolvedDelegate.Verifier
	} else {
		delegateAddr = delegateLookup
	}

	lockedInputIDs := make([]string, 0, len(li.LockedOutputs))
	for _, s := range li.LockedOutputs {
		id, err := common.CoinStateIDFromPersistedString(ctx, s)
		if err != nil {
			return nil, err
		}
		lockedInputIDs = append(lockedInputIDs, id)
	}
	// useNullifiers only steers state-query indexing for this token; locked inputs are resolved by commitment id
	// (lockedOutputs → coin .id), not by treating them as nullifier spends of the unlocked UTXO tree.
	inputStates, revert, err := h.loadCoins(ctx, lockedInputIDs, useNullifiers, req.StateQueryContext)
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
	if len(li.SpendOutputs) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoEmptySpendOutputs, params.LockId.HexString0xPrefix())
	}
	spendOutputIDs := make([]string, 0, len(li.SpendOutputs))
	for _, s := range li.SpendOutputs {
		id, err := common.CoinStateIDFromPersistedString(ctx, s)
		if err != nil {
			return nil, err
		}
		spendOutputIDs = append(spendOutputIDs, id)
	}
	outputPrepared, revert, err := h.loadSpendOutputs(ctx, spendOutputIDs, useNullifiers, req.StateQueryContext)
	if err != nil {
		if revert {
			message := err.Error()
			return &prototk.AssembleTransactionResponse{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   &message,
			}, nil
		}
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}
	recipients, err := spendRecipientsFromLockInfo(ctx, li)
	if err != nil {
		return nil, err
	}
	if err := validateTransferParams(ctx, recipients); err != nil {
		return nil, err
	}
	transferTotal := big.NewInt(0)
	for _, param := range recipients {
		transferTotal = transferTotal.Add(transferTotal, param.Amount.Int())
	}
	if inputStates.total.Cmp(transferTotal) < 0 {
		message := i18n.NewError(ctx, msgs.MsgErrorInsufficientInputAmount, inputStates.total.Text(10), transferTotal.Text(10)).Error()
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   &message,
		}, nil
	}
	if outputPrepared.total.Cmp(transferTotal) != 0 {
		message := i18n.NewError(ctx, msgs.MsgErrorInsufficientInputAmount, outputPrepared.total.Text(10), transferTotal.Text(10)).Error()
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   &message,
		}, nil
	}
	// Recipients were pre-pinned at createLock (InfoStates; confirmed on ZetoLockSpent). The proof spends locked
	// collateral into those pinned outputs — do not mint a separate return-to-owner output (that duplicates value).
	var outputStates []*pb.NewState
	if useNullifiers {
		outputStates, err = spendPinnedOutputStatesForNullifiers(ctx, h.stateSchemas.CoinSchema, h.name, spendOutputIDs, outputPrepared.coins, recipients)
		if err != nil {
			return nil, err
		}
	}
	contractAddress, err := pldtypes.ParseEthAddress(req.Transaction.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}
	// Locked inputs are spent by commitment (lock storage + ZetoSpendLockArgs), not via nullifier SMT membership.
	// v0.5 spendLock uses METHOD_TRANSFER_LOCKED with UsesNullifiers=false (anon / anon_nullifier_kyc_transferLocked per token config).
	payloadBytes, err := formatTransferProvingRequest(ctx, h.callbacks, h.stateSchemas.MerkleTreeRootSchema, h.stateSchemas.MerkleTreeNodeSchema, signercommon.GetHasher(), inputStates.coins, outputPrepared.coins, (*tx.DomainConfig.Circuits)[types.METHOD_TRANSFER_LOCKED], tx.DomainConfig.TokenName, req.StateQueryContext, contractAddress, false, delegateAddr)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFormatProvingReq, err)
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			InputStates:  inputStates.states,
			OutputStates: outputStates, // nullifier specs only; reuses pre-pinned state ids from createLock
		},
		AttestationPlan: []*pb.AttestationRequest{
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

func (h *spendLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *spendLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	var proofRes corepb.ProvingResponse
	result := domain.FindAttestation("sender", req.AttestationResult)
	if result == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindSenderAttestation)
	}
	if err := proto.Unmarshal(result.Payload, &proofRes); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalProvingRes, err)
	}

	inputSize := common.GetInputSize(len(req.InputStates))

	spendParams := tx.Params.(*types.SpendLockParams)
	li, err := h.loadLockInfoByLockID(ctx, spendParams.LockId, req.StateQueryContext)
	if err != nil {
		return nil, err
	}
	if li == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoNotFound, spendParams.LockId.HexString0xPrefix())
	}
	// Inner ZetoSpendLockArgs.data must match the spendCommitment preimage (SpendData) pinned at createLock.
	innerUnlockData := append(pldtypes.HexBytes(nil), li.SpendData...)
	data, err := common.EncodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}
	fnABI := types.LockableCapabilitySpendLockABI
	var prepParams map[string]any
	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	if types.UseZetoOnchainPackedProofCalldata(types.ZetoFungibleABIVersion(tx.DomainConfig.ZetoVariant)) {
		// lockedInputs=true: for nullifier *tokens*, on-chain proof bytes are tuple-only (no root+tuple) because
		// lock spend does not use unlocked-UTXO nullifier public inputs; spendArgs is ABI-encoded ZetoSpendLockArgs.
		proofBytes, err := common.EncodeZetoOnchainTransferProofBytes(ctx, tx.DomainConfig.TokenName, proofRes.Proof, proofRes.PublicInputs, true)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		outputs, err := h.spendLockProofOutputs(ctx, spendParams.LockId, req, useNullifiers, inputSize)
		if err != nil {
			return nil, err
		}
		lockedOutputs := []string{}
		txID32, err := pldtypes.ParseBytes32Ctx(ctx, req.Transaction.TransactionId)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorParseTxId)
		}
		// Inner tuple `data` is the spend commitment preimage (SpendData); outer spendLock `data` carries Paladin tx metadata.
		spendArgsWire := zetoSpendLockArgsWireJSON{
			TxID:          txID32.HexString0xPrefix(),
			LockedOutputs: lockedOutputs,
			Outputs:       outputs,
			Proof:         pldtypes.HexBytes(proofBytes).HexString0xPrefix(),
			Data:          pldtypes.HexBytes(innerUnlockData).HexString0xPrefix(),
		}
		spendArgsJSON, err := json.Marshal([]any{spendArgsWire})
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		spendArgsBytes, err := zetoSpendLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, spendArgsJSON)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		// ILockableCapability.spendLock(lockId, spendArgs, data): spendArgs is ABI-encoded ZetoSpendLockArgs; outer
		// `data` carries Paladin tx metadata for on-chain events (inner tuple already embeds proof + state transition).
		prepParams = map[string]any{
			"lockId":    spendParams.LockId.HexString0xPrefix(),
			"spendArgs": pldtypes.HexBytes(spendArgsBytes).HexString0xPrefix(),
			"data":      pldtypes.HexBytes(data).HexString0xPrefix(),
		}
	} else {
		inputs, err := utxosFromInputStates(ctx, req.InputStates, inputSize)
		if err != nil {
			return nil, err
		}
		outputs, err := h.spendLockProofOutputs(ctx, spendParams.LockId, req, useNullifiers, inputSize)
		if err != nil {
			return nil, err
		}
		prepParams = map[string]any{
			"lockId":  spendParams.LockId.HexString0xPrefix(),
			"inputs":  inputs,
			"outputs": outputs,
			"proof":   common.EncodeProof(proofRes.Proof),
			"data":    data,
		}
		if common.IsNullifiersToken(tx.DomainConfig.TokenName) {
			delete(prepParams, "inputs")
			prepParams["nullifiers"] = strings.Split(proofRes.PublicInputs["nullifiers"], ",")
			prepParams["root"] = proofRes.PublicInputs["root"]
		}
	}
	paramsJSON, err := json.Marshal(prepParams)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalPrepedParams, err)
	}
	functionJSON, err := json.Marshal(fnABI)
	if err != nil {
		return nil, err
	}

	signer := &req.Transaction.From

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
			RequiredSigner:  signer,
		},
	}, nil
}

func (h *spendLockHandler) loadLockInfoByLockID(ctx context.Context, lockID pldtypes.Bytes32, stateQueryContext string) (*types.ZetoLockInfoState, error) {
	if h.stateSchemas.LockInfoSchema == nil {
		return nil, nil
	}
	queryJSON := query.NewQueryBuilder().Limit(1).Equal("lockId", lockID).Query().String()
	states, err := findAvailableStates(ctx, h.callbacks, h.stateSchemas.LockInfoSchema, false, stateQueryContext, queryJSON)
	if err != nil {
		return nil, err
	}
	if len(states) == 0 {
		return nil, nil
	}
	var info types.ZetoLockInfoState
	if err := json.Unmarshal([]byte(states[0].DataJson), &info); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
	}
	return &info, nil
}

func (h *spendLockHandler) loadCoins(ctx context.Context, stateIDs []string, useNullifiers bool, stateQueryContext string) (*preparedInputs, bool, error) {
	inputs, _, revert, err := loadCoinStatesByIDs(ctx, h.callbacks, h.stateSchemas.CoinSchema, useNullifiers, stateQueryContext, stateIDs, true)
	return inputs, revert, err
}

// spendPinnedOutputStatesForNullifiers builds OutputStates for spend-pinned coins so dispatch can UpsertNullifiers
// (info states from createLock cannot). State ids must match the rows pre-allocated at createLock.
func spendPinnedOutputStatesForNullifiers(ctx context.Context, coinSchema *prototk.StateSchema, domainName string, stateIDs []string, coins []*types.ZetoCoin, recipients []*types.FungibleTransferParamEntry) ([]*pb.NewState, error) {
	if len(stateIDs) != len(coins) || len(coins) != len(recipients) {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "spend-pinned outputs/recipients length mismatch")
	}
	out := make([]*pb.NewState, 0, len(coins))
	for i, coin := range coins {
		ns, err := makeNewState(ctx, coinSchema, true, coin, domainName, recipients[i].To)
		if err != nil {
			return nil, err
		}
		id := stateIDs[i]
		ns.Id = &id
		out = append(out, ns)
	}
	return out, nil
}

func (h *spendLockHandler) spendLockProofOutputs(ctx context.Context, lockID pldtypes.Bytes32, req *pb.PrepareTransactionRequest, useNullifiers bool, inputSize int) ([]string, error) {
	if len(req.OutputStates) > 0 {
		var unlockedOutputStates []*pb.EndorsableState
		coinSchemaID := h.stateSchemas.CoinSchema.Id
		for _, state := range req.OutputStates {
			if sid := state.GetSchemaId(); sid != "" && sid != coinSchemaID {
				continue
			}
			var coin types.ZetoCoin
			if err := json.Unmarshal([]byte(state.StateDataJson), &coin); err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
			}
			if !coin.Locked {
				unlockedOutputStates = append(unlockedOutputStates, state)
			}
		}
		outputs, err := utxosFromOutputStates(ctx, unlockedOutputStates, inputSize)
		if err != nil {
			return nil, err
		}
		return trimZeroUtxos(outputs), nil
	}
	spendCoins, err := h.spendPinnedCoinsForPrepare(ctx, lockID, req.StateQueryContext, useNullifiers)
	if err != nil {
		return nil, err
	}
	outputs, err := utxosFromCoins(ctx, spendCoins, inputSize)
	if err != nil {
		return nil, err
	}
	return trimZeroUtxos(outputs), nil
}

func (h *spendLockHandler) spendPinnedCoinsForPrepare(ctx context.Context, lockID pldtypes.Bytes32, stateQueryContext string, useNullifiers bool) ([]*types.ZetoCoin, error) {
	li, err := h.loadLockInfoByLockID(ctx, lockID, stateQueryContext)
	if err != nil {
		return nil, err
	}
	if li == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoNotFound, lockID.HexString0xPrefix())
	}
	spendOutputIDs := make([]string, 0, len(li.SpendOutputs))
	for _, s := range li.SpendOutputs {
		id, err := common.CoinStateIDFromPersistedString(ctx, s)
		if err != nil {
			return nil, err
		}
		spendOutputIDs = append(spendOutputIDs, id)
	}
	prepared, revert, err := h.loadSpendOutputs(ctx, spendOutputIDs, useNullifiers, stateQueryContext)
	if err != nil {
		return nil, err
	}
	if revert {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoEmptySpendOutputs, lockID.HexString0xPrefix())
	}
	return prepared.coins, nil
}

func (h *spendLockHandler) loadSpendOutputs(ctx context.Context, stateIDs []string, useNullifiers bool, stateQueryContext string) (*pinnedOutputLoad, bool, error) {
	// Spend-pinned coins are InfoStates at createLock; load by id even before ZetoLockSpent confirms them.
	prepared, stored, revert, err := loadCoinStatesByIDs(ctx, h.callbacks, h.stateSchemas.CoinSchema, useNullifiers, stateQueryContext, stateIDs, false)
	if err != nil {
		return nil, revert, err
	}
	return &pinnedOutputLoad{coins: prepared.coins, stored: stored, total: prepared.total}, false, nil
}

func validateSpendLockParams(ctx context.Context, params *types.SpendLockParams) error {
	if strings.TrimSpace(params.From) == "" {
		return i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "parameter 'from' is required")
	}
	if params.LockId.IsZero() {
		return i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "parameter 'lockId' is required")
	}
	return nil
}

func spendRecipientsFromLockInfo(ctx context.Context, li *types.ZetoLockInfoState) ([]*types.FungibleTransferParamEntry, error) {
	if len(li.SpendData) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoMissingSpendData, li.LockID.HexString0xPrefix())
	}
	var parsed []*types.FungibleTransferParamEntry
	if err := json.Unmarshal(li.SpendData, &parsed); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "invalid spendData (recipients JSON) in lock info")
	}
	if len(parsed) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoMissingSpendData, li.LockID.HexString0xPrefix())
	}
	return parsed, nil
}
