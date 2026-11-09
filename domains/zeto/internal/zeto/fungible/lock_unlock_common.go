/*
 * Copyright © 2026 Kaleido, Inc.
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
	"github.com/hyperledger-firefly/signer/pkg/abi"
	"google.golang.org/protobuf/proto"
)

// zetoSpendLockArgsTupleABI matches ethers AbiCoder.encode(["tuple(...)"], [args]) for spendArgs/cancelArgs; it must be a
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

// zetoSpendLockArgsWireJSON is the inner struct for spendArgs/cancelArgs before json.Marshal([]any{wire}) + EncodeABIDataJSONCtx.
type zetoSpendLockArgsWireJSON struct {
	TxID          string   `json:"txId"`
	LockedOutputs []string `json:"lockedOutputs"`
	Outputs       []string `json:"outputs"`
	Proof         string   `json:"proof"`
	Data          string   `json:"data"`
}

// unlockOperation selects which of the two outcomes pinned in the lock info at createLock a transaction carries out
type unlockOperation struct {
	outputs           func(li *types.ZetoLockInfoState) []string
	data              func(li *types.ZetoLockInfoState) pldtypes.HexBytes
	dataName          string
	emptyOutputsError i18n.ErrorMessageKey
	missingDataError  i18n.ErrorMessageKey
	functionABI       *abi.Entry
	argsParam         string
}

var spendLockOperation = &unlockOperation{
	outputs:           func(li *types.ZetoLockInfoState) []string { return li.SpendOutputs },
	data:              func(li *types.ZetoLockInfoState) pldtypes.HexBytes { return li.SpendData },
	dataName:          "spendData",
	emptyOutputsError: msgs.MsgErrorLockInfoEmptySpendOutputs,
	missingDataError:  msgs.MsgErrorLockInfoMissingSpendData,
	functionABI:       types.LockableCapabilitySpendLockABI,
	argsParam:         "spendArgs",
}

var cancelLockOperation = &unlockOperation{
	outputs:           func(li *types.ZetoLockInfoState) []string { return li.CancelOutputs },
	data:              func(li *types.ZetoLockInfoState) pldtypes.HexBytes { return li.CancelData },
	dataName:          "cancelData",
	emptyOutputsError: msgs.MsgErrorLockInfoEmptyCancelOutputs,
	missingDataError:  msgs.MsgErrorLockInfoMissingCancelData,
	functionABI:       types.LockableCapabilityCancelLockABI,
	argsParam:         "cancelArgs",
}

type pinnedOutputLoad struct {
	coins  []*types.ZetoCoin
	stored []*prototk.StoredState
	total  *big.Int
}

// FindLockInfo returns a nil lock info when no lock info state for the lock is available to this node
func FindLockInfo(ctx context.Context, callbacks plugintk.DomainCallbacks, lockInfoSchema *prototk.StateSchema, stateQueryContext string, lockID pldtypes.Bytes32) (info *types.ZetoLockInfoState, stateID string, err error) {
	queryJSON := query.NewQueryBuilder().Limit(1).Equal("lockId", lockID).Query().String()
	states, err := findAvailableStates(ctx, callbacks, lockInfoSchema, false, stateQueryContext, queryJSON)
	if err != nil {
		return nil, "", err
	}
	if len(states) == 0 {
		return nil, "", nil
	}
	info = &types.ZetoLockInfoState{}
	if err := json.Unmarshal([]byte(states[0].DataJson), info); err != nil {
		return nil, "", i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
	}
	return info, states[0].Id, nil
}

func loadLockedCoins(ctx context.Context, callbacks plugintk.DomainCallbacks, coinSchema *prototk.StateSchema, lockedOutputStrs []string, useNullifiers bool, stateQueryContext string) (*preparedInputs, bool, error) {
	lockedInputIDs, err := coinStateIDsFromPersisted(ctx, lockedOutputStrs)
	if err != nil {
		return nil, false, err
	}
	inputs, _, revert, err := loadCoinStatesByIDs(ctx, callbacks, coinSchema, useNullifiers, stateQueryContext, lockedInputIDs, true)
	return inputs, revert, err
}

func loadPinnedOutputs(ctx context.Context, callbacks plugintk.DomainCallbacks, coinSchema *prototk.StateSchema, outputIDs []string, useNullifiers bool, stateQueryContext string) (*pinnedOutputLoad, bool, error) {
	// Pinned coins are InfoStates at createLock; load by id even before ZetoLockSpent/ZetoLockCancelled confirms them.
	prepared, stored, revert, err := loadCoinStatesByIDs(ctx, callbacks, coinSchema, useNullifiers, stateQueryContext, outputIDs, false)
	if err != nil {
		return nil, revert, err
	}
	return &pinnedOutputLoad{coins: prepared.coins, stored: stored, total: prepared.total}, false, nil
}

func coinStateIDsFromPersisted(ctx context.Context, persisted []string) ([]string, error) {
	ids := make([]string, 0, len(persisted))
	for _, s := range persisted {
		id, err := common.CoinStateIDFromPersistedString(ctx, s)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// pinnedOutputStatesForNullifiers builds OutputStates for pinned coins so dispatch can UpsertNullifiers
// (info states from createLock cannot). State ids must match the rows pre-allocated at createLock.
func pinnedOutputStatesForNullifiers(ctx context.Context, coinSchema *prototk.StateSchema, domainName string, stateIDs []string, coins []*types.ZetoCoin, recipients []*types.FungibleTransferParamEntry) ([]*pb.NewState, error) {
	if len(stateIDs) != len(coins) || len(coins) != len(recipients) {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "pinned outputs/recipients length mismatch")
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

// recipientsFromLockInfo parses spendData/cancelData JSON pinned at createLock.
func recipientsFromLockInfo(ctx context.Context, li *types.ZetoLockInfoState, op *unlockOperation) ([]*types.FungibleTransferParamEntry, error) {
	data := op.data(li)
	if len(data) == 0 {
		return nil, i18n.NewError(ctx, op.missingDataError, li.LockID.HexString0xPrefix())
	}
	var parsed []*types.FungibleTransferParamEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "invalid "+op.dataName+" (recipients JSON) in lock info")
	}
	if len(parsed) == 0 {
		return nil, i18n.NewError(ctx, op.missingDataError, li.LockID.HexString0xPrefix())
	}
	return parsed, nil
}

func validateLockUnlockParams(ctx context.Context, lockID pldtypes.Bytes32, from string) error {
	if strings.TrimSpace(from) == "" {
		return i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "parameter 'from' is required")
	}
	if lockID.IsZero() {
		return i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "parameter 'lockId' is required")
	}
	return nil
}

func assembleRevert(err error) *pb.AssembleTransactionResponse {
	message := err.Error()
	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_REVERT,
		RevertReason:   &message,
	}
}

type unlockCommon struct {
	baseHandler
	callbacks plugintk.DomainCallbacks
}

func newUnlockCommon(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema, lockInfoSchema *pb.StateSchema) unlockCommon {
	return unlockCommon{
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

func (h *unlockCommon) loadLockInfo(ctx context.Context, lockID pldtypes.Bytes32, stateQueryContext string) (*types.ZetoLockInfoState, error) {
	if h.stateSchemas.LockInfoSchema == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMissingLockInputs)
	}
	li, _, err := FindLockInfo(ctx, h.callbacks, h.stateSchemas.LockInfoSchema, stateQueryContext, lockID)
	if err != nil {
		return nil, err
	}
	if li == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoNotFound, lockID.HexString0xPrefix())
	}
	return li, nil
}

func (h *unlockCommon) init(tx *types.ParsedTransaction) *pb.InitTransactionResponse {
	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}
}

func (h *unlockCommon) assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest, op *unlockOperation, lockID pldtypes.Bytes32) (*pb.AssembleTransactionResponse, error) {
	if domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers) == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	li, err := h.loadLockInfo(ctx, lockID, req.StateQueryContext)
	if err != nil {
		return nil, err
	}
	if len(li.LockedOutputs) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoEmptyLockedInputs, lockID.HexString0xPrefix())
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

	// useNullifiers only steers state-query indexing for this token; locked inputs are resolved by commitment id
	// (lockedOutputs → coin .id), not by treating them as nullifier spends of the unlocked UTXO tree.
	inputStates, revert, err := loadLockedCoins(ctx, h.callbacks, h.stateSchemas.CoinSchema, li.LockedOutputs, useNullifiers, req.StateQueryContext)
	if err != nil {
		if revert {
			return assembleRevert(err), nil
		}
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxInputs, err)
	}
	if len(op.outputs(li)) == 0 {
		return nil, i18n.NewError(ctx, op.emptyOutputsError, lockID.HexString0xPrefix())
	}
	outputIDs, err := coinStateIDsFromPersisted(ctx, op.outputs(li))
	if err != nil {
		return nil, err
	}
	outputPrepared, revert, err := loadPinnedOutputs(ctx, h.callbacks, h.stateSchemas.CoinSchema, outputIDs, useNullifiers, req.StateQueryContext)
	if err != nil {
		if revert {
			return assembleRevert(err), nil
		}
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}
	recipients, err := recipientsFromLockInfo(ctx, li, op)
	if err != nil {
		return nil, err
	}
	if err := validateTransferParams(ctx, recipients); err != nil {
		return nil, err
	}
	total := big.NewInt(0)
	for _, param := range recipients {
		total = total.Add(total, param.Amount.Int())
	}
	if inputStates.total.Cmp(total) < 0 {
		return assembleRevert(i18n.NewError(ctx, msgs.MsgErrorInsufficientInputAmount, inputStates.total.Text(10), total.Text(10))), nil
	}
	if outputPrepared.total.Cmp(total) != 0 {
		return assembleRevert(i18n.NewError(ctx, msgs.MsgErrorInsufficientInputAmount, outputPrepared.total.Text(10), total.Text(10))), nil
	}
	// Recipients were pre-pinned at createLock (InfoStates; confirmed by the outputs of ZetoLockSpent or ZetoLockCancelled). The proof spends locked
	// collateral into those pinned outputs — do not mint a separate return-to-owner output (that duplicates value).
	var outputStates []*pb.NewState
	if useNullifiers {
		outputStates, err = pinnedOutputStatesForNullifiers(ctx, h.stateSchemas.CoinSchema, h.name, outputIDs, outputPrepared.coins, recipients)
		if err != nil {
			return nil, err
		}
	}
	contractAddress, err := pldtypes.ParseEthAddress(req.Transaction.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}
	// Locked inputs are spent by commitment (lock storage + ZetoSpendLockArgs), not via nullifier SMT membership.
	// v0.5 spendLock/cancelLock uses METHOD_TRANSFER_LOCKED with UsesNullifiers=false (anon / anon_nullifier_kyc_transferLocked per token config).
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

// proofOutputs returns the unlocked outputs the proof commits to: those assembled as output states when there are any,
// otherwise the outputs pinned in the lock info
func (h *unlockCommon) proofOutputs(ctx context.Context, req *pb.PrepareTransactionRequest, op *unlockOperation, li *types.ZetoLockInfoState, useNullifiers bool, inputSize int) ([]string, error) {
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
	outputIDs, err := coinStateIDsFromPersisted(ctx, op.outputs(li))
	if err != nil {
		return nil, err
	}
	pinned, _, err := loadPinnedOutputs(ctx, h.callbacks, h.stateSchemas.CoinSchema, outputIDs, useNullifiers, req.StateQueryContext)
	if err != nil {
		return nil, err
	}
	outputs, err := utxosFromCoins(ctx, pinned.coins, inputSize)
	if err != nil {
		return nil, err
	}
	return trimZeroUtxos(outputs), nil
}

func (h *unlockCommon) prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest, op *unlockOperation, lockID pldtypes.Bytes32) (*pb.PrepareTransactionResponse, error) {
	var proofRes corepb.ProvingResponse
	result := domain.FindAttestation("sender", req.AttestationResult)
	if result == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindSenderAttestation)
	}
	if err := proto.Unmarshal(result.Payload, &proofRes); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalProvingRes, err)
	}

	inputSize := common.GetInputSize(len(req.InputStates))
	li, err := h.loadLockInfo(ctx, lockID, req.StateQueryContext)
	if err != nil {
		return nil, err
	}
	data, err := common.EncodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}

	var prepParams map[string]any
	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	if types.UseZetoOnchainPackedProofCalldata(types.ZetoFungibleABIVersion(tx.DomainConfig.ZetoVariant)) {
		// lockedInputs=true: for nullifier *tokens*, on-chain proof bytes are tuple-only (no root+tuple) because
		// lock spend does not use unlocked-UTXO nullifier public inputs; spendArgs/cancelArgs is ABI-encoded ZetoSpendLockArgs.
		proofBytes, err := common.EncodeZetoOnchainTransferProofBytes(ctx, tx.DomainConfig.TokenName, proofRes.Proof, proofRes.PublicInputs, true)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		outputs, err := h.proofOutputs(ctx, req, op, li, useNullifiers, inputSize)
		if err != nil {
			return nil, err
		}
		txID32, err := pldtypes.ParseBytes32Ctx(ctx, req.Transaction.TransactionId)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorParseTxId)
		}
		// Inner tuple `data` is the commitment preimage (SpendData/CancelData); outer `data` carries Paladin tx metadata.
		argsWire := zetoSpendLockArgsWireJSON{
			TxID:          txID32.HexString0xPrefix(),
			LockedOutputs: zetoSpendLockEmptyLockedOutputs,
			Outputs:       outputs,
			Proof:         pldtypes.HexBytes(proofBytes).HexString0xPrefix(),
			// Inner ZetoSpendLockArgs.data must match the spendCommitment/cancelCommitment preimage (SpendData/CancelData) pinned at createLock.
			Data: op.data(li).HexString0xPrefix(),
		}
		argsJSON, err := json.Marshal([]any{argsWire})
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		argsBytes, err := zetoSpendLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, argsJSON)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		// ILockableCapability.spendLock/cancelLock(lockId, spendArgs/cancelArgs, data): the args are ABI-encoded ZetoSpendLockArgs; outer
		// `data` carries Paladin tx metadata for on-chain events (inner tuple already embeds proof + state transition).
		prepParams = map[string]any{
			"lockId":     lockID.HexString0xPrefix(),
			op.argsParam: pldtypes.HexBytes(argsBytes).HexString0xPrefix(),
			"data":       pldtypes.HexBytes(data).HexString0xPrefix(),
		}
	} else {
		inputs, err := utxosFromInputStates(ctx, req.InputStates, inputSize)
		if err != nil {
			return nil, err
		}
		outputs, err := h.proofOutputs(ctx, req, op, li, useNullifiers, inputSize)
		if err != nil {
			return nil, err
		}
		prepParams = map[string]any{
			"lockId":  lockID.HexString0xPrefix(),
			"inputs":  inputs,
			"outputs": outputs,
			"proof":   common.EncodeProof(proofRes.Proof),
			"data":    data,
		}
		if useNullifiers {
			delete(prepParams, "inputs")
			prepParams["nullifiers"] = strings.Split(proofRes.PublicInputs["nullifiers"], ",")
			prepParams["root"] = proofRes.PublicInputs["root"]
		}
	}
	paramsJSON, err := json.Marshal(prepParams)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalPrepedParams, err)
	}
	functionJSON, err := json.Marshal(op.functionABI)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
			RequiredSigner:  &req.Transaction.From,
		},
	}, nil
}
