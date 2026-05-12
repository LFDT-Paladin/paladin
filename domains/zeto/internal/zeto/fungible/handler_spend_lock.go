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

var spendLockABI = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_SPEND_LOCK,
	Inputs: abi.ParameterArray{
		{Name: "lockId", Type: "bytes32"},
		{Name: "inputs", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

var spendLockABINullifiers = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_SPEND_LOCK,
	Inputs: abi.ParameterArray{
		{Name: "lockId", Type: "bytes32"},
		{Name: "nullifiers", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "root", Type: "uint256"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

var spendLockABIOnchainPacked = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_SPEND_LOCK,
	Inputs: abi.ParameterArray{
		{Name: "lockId", Type: "bytes32"},
		{Name: "inputs", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "proof", Type: "bytes"},
		{Name: "data", Type: "bytes"},
	},
}

func NewSpendLockHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema *pb.StateSchema) *spendLockHandler {
	return &spendLockHandler{
		baseHandler: baseHandler{
			name: name,
			stateSchemas: &common.StateSchemas{
				CoinSchema:           coinSchema,
				MerkleTreeRootSchema: merkleTreeRootSchema,
				MerkleTreeNodeSchema: merkleTreeNodeSchema,
				DataSchema:           dataSchema,
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

	if err := validateSpendLockParams(ctx, p); err != nil {
		return nil, err
	}
	if err := validateTransferParams(ctx, p.Recipients); err != nil {
		return nil, err
	}

	return &p, nil
}

func (h *spendLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.Params.(*types.SpendLockParams)

	res := &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}
	_, err := pldtypes.ParseEthAddress(params.Delegate)
	if err != nil {
		res.RequiredVerifiers = append(res.RequiredVerifiers, &pb.ResolveVerifierRequest{
			Lookup:       params.Delegate,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		})
	}
	for _, r := range params.Recipients {
		res.RequiredVerifiers = append(res.RequiredVerifiers, &pb.ResolveVerifierRequest{
			Lookup:       r.To,
			Algorithm:    h.getAlgoZetoSnarkBJJ(),
			VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		})
	}

	return res, nil
}

func (h *spendLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.SpendLockParams)

	resolvedSender := domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}
	var delegateAddr string
	_, err := pldtypes.ParseEthAddress(params.Delegate)
	if err != nil {
		resolvedDelegate := domain.FindVerifier(params.Delegate, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
		if resolvedDelegate == nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, params.Delegate)
		}
		delegateAddr = resolvedDelegate.Verifier
	} else {
		delegateAddr = params.Delegate
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	inputStates, revert, err := h.loadCoins(ctx, params.LockedInputs, useNullifiers, req.StateQueryContext)
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

	inputTotal := big.NewInt(0)
	for _, coin := range inputStates.coins {
		inputTotal = inputTotal.Add(inputTotal, coin.Amount.Int())
	}
	transferTotal := big.NewInt(0)
	for _, param := range params.Recipients {
		transferTotal = transferTotal.Add(transferTotal, param.Amount.Int())
	}
	if inputTotal.Cmp(transferTotal) < 0 {
		message := i18n.NewError(ctx, msgs.MsgErrorInsufficientInputAmount, inputTotal.Text(10), transferTotal.Text(10)).Error()
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   &message,
		}, nil
	}

	outputCoins, outputStates, err := prepareOutputsForTransfer(ctx, useNullifiers, params.Recipients, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}
	remainder := big.NewInt(0).Sub(inputTotal, transferTotal)
	if remainder.Sign() > 0 {
		remainderHex := pldtypes.HexUint256(*remainder)
		remainderParams := []*types.FungibleTransferParamEntry{
			{
				To:     tx.Transaction.From,
				Amount: &remainderHex,
			},
		}
		returnedCoins, returnedStates, err := prepareOutputsForTransfer(ctx, useNullifiers, remainderParams, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxChange, err)
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}

	infoStates := make([]*pb.NewState, 0, len(params.Recipients))
	for _, param := range params.Recipients {
		info, err := prepareTransactionInfoStates(ctx, param.Data, []string{tx.Transaction.From, param.To}, h.stateSchemas.DataSchema)
		if err != nil {
			return nil, err
		}
		infoStates = append(infoStates, info...)
	}

	contractAddress, err := pldtypes.ParseEthAddress(req.Transaction.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}
	payloadBytes, err := formatTransferProvingRequest(ctx, h.callbacks, h.stateSchemas.MerkleTreeRootSchema, h.stateSchemas.MerkleTreeNodeSchema, signercommon.GetHasher(), inputStates.coins, outputCoins, (*tx.DomainConfig.Circuits)[types.METHOD_TRANSFER_LOCKED], tx.DomainConfig.TokenName, req.StateQueryContext, contractAddress, delegateAddr)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFormatProvingReq, err)
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			InputStates:  inputStates.states,
			OutputStates: outputStates,
			InfoStates:   infoStates,
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
	inputs, err := utxosFromInputStates(ctx, req.InputStates, inputSize)
	if err != nil {
		return nil, err
	}
	outputs, err := utxosFromOutputStates(ctx, req.OutputStates, inputSize)
	if err != nil {
		return nil, err
	}

	data, err := common.EncodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}
	spendParams := tx.Params.(*types.SpendLockParams)
	fnABI := getSpendLockABI(tx.DomainConfig.TokenName, tx.DomainConfig.ZetoVariant)
	var prepParams map[string]any
	if types.UseZetoOnchainPackedProofCalldata(types.ZetoFungibleABIVersion(tx.DomainConfig.ZetoVariant)) {
		proofBytes, err := common.EncodeZetoOnchainTransferProofBytes(ctx, tx.DomainConfig.TokenName, proofRes.Proof, proofRes.PublicInputs, true)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgErrorMarshalPrepedParams)
		}
		inCol := inputs
		if common.IsNullifiersToken(tx.DomainConfig.TokenName) {
			inCol = strings.Split(proofRes.PublicInputs["nullifiers"], ",")
		}
		prepParams = map[string]any{
			"lockId":  spendParams.LockId.HexString0xPrefix(),
			"inputs":  inCol,
			"outputs": outputs,
			"proof":   pldtypes.HexBytes(proofBytes).HexString0xPrefix(),
			"data":    data,
		}
	} else {
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

	signer := &spendParams.Delegate
	if req.Transaction.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION {
		signer = &req.Transaction.From
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
			RequiredSigner:  signer,
		},
	}, nil
}

func (h *spendLockHandler) loadCoins(ctx context.Context, ids []*pldtypes.HexUint256, useNullifiers bool, stateQueryContext string) (inputs *preparedInputs, revert bool, err error) {
	inputIDs := make([]any, 0, len(ids))
	for _, input := range ids {
		if !input.NilOrZero() {
			inputIDs = append(inputIDs, common.HexUint256To32ByteHexString(input))
		}
	}

	queryBuilder := query.NewQueryBuilder().In(".id", inputIDs)
	inputStates, err := findAvailableStates(ctx, h.callbacks, h.stateSchemas.CoinSchema, useNullifiers, stateQueryContext, queryBuilder.Query().String())
	if err != nil {
		return nil, false, err
	}
	if len(inputStates) != len(inputIDs) {
		return nil, true, i18n.NewError(ctx, msgs.MsgFailedToQueryStatesById, len(inputIDs), len(inputStates))
	}

	inputCoins := make([]*types.ZetoCoin, len(inputStates))
	stateRefs := make([]*pb.StateRef, 0, len(inputStates))
	for i, state := range inputStates {
		err := json.Unmarshal([]byte(state.DataJson), &inputCoins[i])
		if err != nil {
			return nil, true, i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
		}
		if inputCoins[i].Locked == false {
			return nil, true, i18n.NewError(ctx, msgs.MsgErrorInputNotLocked, state.Id)
		}
		stateRefs = append(stateRefs, &pb.StateRef{
			SchemaId: state.SchemaId,
			Id:       state.Id,
		})

	}
	return &preparedInputs{
		coins:  inputCoins,
		states: stateRefs,
	}, false, nil
}

func validateSpendLockParams(ctx context.Context, params types.SpendLockParams) error {
	if params.LockId.IsZero() {
		return i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "parameter 'lockId' is required")
	}
	if len(params.LockedInputs) == 0 {
		return i18n.NewError(ctx, msgs.MsgErrorMissingLockInputs)
	}
	if params.Delegate == "" {
		return i18n.NewError(ctx, msgs.MsgErrorMissingLockDelegate)
	}
	return nil
}

func getSpendLockABI(tokenName string, zetoVariant pldtypes.HexUint64) *abi.Entry {
	if types.UseZetoOnchainPackedProofCalldata(types.ZetoFungibleABIVersion(zetoVariant)) {
		return spendLockABIOnchainPacked
	}
	fnABI := spendLockABI
	if common.IsNullifiersToken(tokenName) {
		fnABI = spendLockABINullifiers
	}
	return fnABI
}
