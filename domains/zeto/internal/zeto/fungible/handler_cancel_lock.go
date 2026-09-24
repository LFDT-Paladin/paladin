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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	pb "github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"google.golang.org/protobuf/proto"
)

var _ types.DomainHandler = &cancelLockHandler{}

type cancelLockHandler struct {
	lockUnlockHandler
}

func NewCancelLockHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema, lockInfoSchema *pb.StateSchema) *cancelLockHandler {
	return &cancelLockHandler{
		lockUnlockHandler: lockUnlockHandler{
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
		},
	}
}

func (h *cancelLockHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var p types.CancelLockParams
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return nil, err
	}
	if err := validateLockUnlockParams(ctx, p.LockId, p.From); err != nil {
		return nil, err
	}
	return &p, nil
}

func (h *cancelLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}, nil
}

func (h *cancelLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.CancelLockParams)

	if domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers) == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	if h.stateSchemas.LockInfoSchema == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMissingLockInputs)
	}
	li, err := loadLockInfoByLockID(ctx, h.callbacks, h.stateSchemas.LockInfoSchema, params.LockId, req.StateQueryContext)
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

	inputStates, revert, err := loadLockedCoins(ctx, h.callbacks, h.stateSchemas.CoinSchema, li.LockedOutputs, useNullifiers, req.StateQueryContext)
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
	if len(li.CancelOutputs) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoEmptyCancelOutputs, params.LockId.HexString0xPrefix())
	}
	cancelOutputIDs := make([]string, 0, len(li.CancelOutputs))
	for _, s := range li.CancelOutputs {
		id, err := common.CoinStateIDFromPersistedString(ctx, s)
		if err != nil {
			return nil, err
		}
		cancelOutputIDs = append(cancelOutputIDs, id)
	}
	outputPrepared, revert, err := h.loadPinnedOutputsByIDs(ctx, cancelOutputIDs, useNullifiers, req.StateQueryContext)
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
	recipients, err := cancelRecipientsFromLockInfo(ctx, li)
	if err != nil {
		return nil, err
	}
	if err := validateTransferParams(ctx, recipients); err != nil {
		return nil, err
	}
	cancelTotal := big.NewInt(0)
	for _, param := range recipients {
		cancelTotal = cancelTotal.Add(cancelTotal, param.Amount.Int())
	}
	if inputStates.total.Cmp(cancelTotal) < 0 {
		message := i18n.NewError(ctx, msgs.MsgErrorInsufficientInputAmount, inputStates.total.Text(10), cancelTotal.Text(10)).Error()
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   &message,
		}, nil
	}
	if outputPrepared.total.Cmp(cancelTotal) != 0 {
		message := i18n.NewError(ctx, msgs.MsgErrorInsufficientInputAmount, outputPrepared.total.Text(10), cancelTotal.Text(10)).Error()
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   &message,
		}, nil
	}

	var outputStates []*pb.NewState
	if useNullifiers {
		outputStates, err = pinnedOutputStatesForNullifiers(ctx, h.stateSchemas.CoinSchema, h.name, cancelOutputIDs, outputPrepared.coins, recipients)
		if err != nil {
			return nil, err
		}
	}

	contractAddress, err := pldtypes.ParseEthAddress(req.Transaction.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}
	payloadBytes, err := formatTransferProvingRequest(ctx, h.callbacks, h.stateSchemas.MerkleTreeRootSchema, h.stateSchemas.MerkleTreeNodeSchema, signercommon.GetHasher(), inputStates.coins, outputPrepared.coins, (*tx.DomainConfig.Circuits)[types.METHOD_TRANSFER_LOCKED], tx.DomainConfig.TokenName, req.StateQueryContext, contractAddress, false, delegateAddr)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFormatProvingReq, err)
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			InputStates:  inputStates.states,
			OutputStates: outputStates,
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

func (h *cancelLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *cancelLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	var proofRes corepb.ProvingResponse
	result := domain.FindAttestation("sender", req.AttestationResult)
	if result == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindSenderAttestation)
	}
	if err := proto.Unmarshal(result.Payload, &proofRes); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalProvingRes, err)
	}

	inputSize := common.GetInputSize(len(req.InputStates))
	cancelParams := tx.Params.(*types.CancelLockParams)
	li, err := loadLockInfoByLockID(ctx, h.callbacks, h.stateSchemas.LockInfoSchema, cancelParams.LockId, req.StateQueryContext)
	if err != nil {
		return nil, err
	}
	if li == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoNotFound, cancelParams.LockId.HexString0xPrefix())
	}
	innerUnlockData := append(pldtypes.HexBytes(nil), li.CancelData...)
	data, err := common.EncodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}

	fnABI := types.LockableCapabilityCancelLockABI
	var prepParams map[string]any
	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	if types.UseZetoOnchainPackedProofCalldata(types.ZetoFungibleABIVersion(tx.DomainConfig.ZetoVariant)) {
		proofBytes, err := common.EncodeZetoOnchainTransferProofBytes(ctx, tx.DomainConfig.TokenName, proofRes.Proof, proofRes.PublicInputs, true)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		cancelOutputIDs := make([]string, 0, len(li.CancelOutputs))
		for _, s := range li.CancelOutputs {
			id, err := common.CoinStateIDFromPersistedString(ctx, s)
			if err != nil {
				return nil, err
			}
			cancelOutputIDs = append(cancelOutputIDs, id)
		}
		cancelCoins, err := h.pinnedCoinsForPrepare(ctx, cancelParams.LockId, cancelOutputIDs, req.StateQueryContext, useNullifiers)
		if err != nil {
			return nil, err
		}
		outputs, err := utxosFromCoins(ctx, cancelCoins, inputSize)
		if err != nil {
			return nil, err
		}
		outputs = trimZeroUtxos(outputs)
		txID32, err := pldtypes.ParseBytes32Ctx(ctx, req.Transaction.TransactionId)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorParseTxId)
		}
		cancelArgsWire := zetoLockUnlockArgsWireJSON{
			TxID:          txID32.HexString0xPrefix(),
			LockedOutputs: zetoSpendLockEmptyLockedOutputs,
			Outputs:       outputs,
			Proof:         pldtypes.HexBytes(proofBytes).HexString0xPrefix(),
			Data:          pldtypes.HexBytes(innerUnlockData).HexString0xPrefix(),
		}
		cancelArgsJSON, err := json.Marshal([]any{cancelArgsWire})
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		cancelArgsBytes, err := zetoLockUnlockArgsTupleABI.EncodeABIDataJSONCtx(ctx, cancelArgsJSON)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		prepParams = map[string]any{
			"lockId":     cancelParams.LockId.HexString0xPrefix(),
			"cancelArgs": pldtypes.HexBytes(cancelArgsBytes).HexString0xPrefix(),
			"data":       pldtypes.HexBytes(data).HexString0xPrefix(),
		}
	} else {
		inputs, err := utxosFromInputStates(ctx, req.InputStates, inputSize)
		if err != nil {
			return nil, err
		}
		cancelOutputIDs := make([]string, 0, len(li.CancelOutputs))
		for _, s := range li.CancelOutputs {
			id, err := common.CoinStateIDFromPersistedString(ctx, s)
			if err != nil {
				return nil, err
			}
			cancelOutputIDs = append(cancelOutputIDs, id)
		}
		cancelCoins, err := h.pinnedCoinsForPrepare(ctx, cancelParams.LockId, cancelOutputIDs, req.StateQueryContext, useNullifiers)
		if err != nil {
			return nil, err
		}
		outputs, err := utxosFromCoins(ctx, cancelCoins, inputSize)
		if err != nil {
			return nil, err
		}
		outputs = trimZeroUtxos(outputs)
		prepParams = map[string]any{
			"lockId":  cancelParams.LockId.HexString0xPrefix(),
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
	functionJSON, err := json.Marshal(fnABI)
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

func (h *cancelLockHandler) pinnedCoinsForPrepare(ctx context.Context, lockID pldtypes.Bytes32, outputStateIDs []string, stateQueryContext string, useNullifiers bool) ([]*types.ZetoCoin, error) {
	prepared, revert, err := h.loadPinnedOutputsByIDs(ctx, outputStateIDs, useNullifiers, stateQueryContext)
	if err != nil {
		return nil, err
	}
	if revert {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoEmptyCancelOutputs, lockID.HexString0xPrefix())
	}
	return prepared.coins, nil
}
