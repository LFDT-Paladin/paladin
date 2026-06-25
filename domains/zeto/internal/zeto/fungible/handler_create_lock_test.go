/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// TestZetoCreateLockArgsTupleABI_RoundTrip ensures createArgs bytes match Solidity abi.decode(createArgs, (ZetoCreateLockArgs)).
func TestZetoCreateLockArgsTupleABI_RoundTrip(t *testing.T) {
	ctx := context.Background()
	wire := zetoCreateLockArgsWireJSON{
		TxID:          "0x" + strings.Repeat("aa", 32),
		Inputs:        []string{"0x01", "0x02"},
		Outputs:       []string{"0x03"},
		LockedOutputs: []string{"0x04"},
		Proof:         "0xabcd",
	}
	j, err := json.Marshal([]any{wire})
	require.NoError(t, err)

	enc, err := zetoCreateLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, j)
	require.NoError(t, err)
	require.NotEmpty(t, enc)

	decoded, err := zetoCreateLockArgsTupleABI.DecodeABIDataCtx(ctx, enc, 0)
	require.NoError(t, err)
	require.Len(t, decoded.Children, 1)
	inner := decoded.Children[0]
	require.NotNil(t, inner)
	require.Len(t, inner.Children, 5)
	out, err := inner.JSON()
	require.NoError(t, err)
	js := string(out)
	assert.Contains(t, js, `"txId"`)
	assert.Contains(t, js, `"inputs"`)
	assert.Contains(t, js, `"outputs"`)
	assert.Contains(t, js, `"lockedOutputs"`)
	assert.Contains(t, js, `"proof"`)
}

// ethers v6: AbiCoder.encode(["tuple(bytes32,uint256[],uint256[],uint256[],bytes)"], [obj])
// prefixes the payload with a 0x20 offset word (see zeto_anon.ts encodeCreateArgs).
func TestZetoCreateLockArgsTupleABI_FirstWordMatchesEthers(t *testing.T) {
	ctx := context.Background()
	wire := zetoCreateLockArgsWireJSON{
		TxID:          "0x" + strings.Repeat("11", 32),
		Inputs:        []string{"0x01", "0x02"},
		Outputs:       []string{},
		LockedOutputs: []string{"0x03"},
		Proof:         "0xabcd",
	}
	j, err := json.Marshal([]any{wire})
	require.NoError(t, err)
	enc, err := zetoCreateLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, j)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(enc), 32)
	got := hex.EncodeToString(enc[:32])
	const ethersFirstWord = "0000000000000000000000000000000000000000000000000000000000000020"
	assert.Equal(t, ethersFirstWord, got, "must match ethers encodeCreateArgs / zeto_anon.ts; mismatch breaks Solidity abi.decode(createArgs, (ZetoCreateLockArgs))")
}

func TestCreateLockRecipientsTotal(t *testing.T) {
	assert.Equal(t, int64(0), createLockRecipientsTotal(nil).Int64())
	sum := createLockRecipientsTotal(&types.CreateLockParams{
		Recipients: []*types.FungibleTransferParamEntry{
			{Amount: pldtypes.Uint64ToUint256(3)},
			{Amount: pldtypes.Uint64ToUint256(7)},
		},
	})
	assert.Equal(t, int64(10), sum.Int64())
}

func TestCreateLockValidateParams(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	_, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, "bad")
	require.Error(t, err)

	_, err = h.ValidateParams(ctx, &types.DomainInstanceConfig{}, `{"from":"alice@node","recipients":[]}`)
	require.Error(t, err)

	_, err = h.ValidateParams(ctx, &types.DomainInstanceConfig{}, `{"from":"alice@node","recipients":[{"to":"bob@node","amount":1}]}`)
	require.NoError(t, err)

	_, err = h.ValidateParams(ctx, &types.DomainInstanceConfig{}, `{"from":"alice@node","recipients":[{"to":"bob@node","amount":0}]}`)
	require.Error(t, err)

	_, err = h.ValidateParams(ctx, &types.DomainInstanceConfig{}, `{"from":"","recipients":[{"to":"bob@node","amount":1}]}`)
	require.Error(t, err)

	tooLarge := new(big.Int).Exp(big.NewInt(2), big.NewInt(100), nil)
	_, err = h.ValidateParams(ctx, &types.DomainInstanceConfig{}, fmt.Sprintf(
		`{"from":"alice@node","recipients":[{"to":"bob@node","amount":%s}]}`, tooLarge.String()))
	require.Error(t, err)

	_, err = h.ValidateParams(ctx, &types.DomainInstanceConfig{}, `{"from":"alice@node","recipients":[{"to":"","amount":1}]}`)
	require.Error(t, err)
}

func TestCreateLockInitAndEndorse(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil)
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{From: "alice@node"},
		Params: &types.CreateLockParams{
			Recipients: []*types.FungibleTransferParamEntry{{To: "bob@node", Amount: pldtypes.Uint64ToUint256(1)}},
		},
	}
	res, err := h.Init(ctx, tx, &prototk.InitTransactionRequest{})
	require.NoError(t, err)
	require.Len(t, res.RequiredVerifiers, 3)
	endorseRes, err := h.Endorse(ctx, tx, &prototk.EndorseTransactionRequest{})
	require.NoError(t, err)
	assert.Nil(t, endorseRes)
}

func TestCreateLockPrepare_EncodesCreateArgs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("66", 32))
	info := types.ZetoLockInfoState{
		LockID:           lockID,
		SpendCommitment:  pldtypes.RandBytes32(),
		CancelCommitment: pldtypes.RandBytes32(),
	}
	infoJSON, err := json.Marshal(&info)
	require.NoError(t, err)

	h := NewCreateLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x01", "0x02"},
			B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
			C: []string{"0x07", "0x08"},
		},
		PublicInputs: map[string]string{"nullifiers": "0x09,0x0a", "root": "0x0b"},
	}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)

	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName:   "Zeto_AnonNullifier",
			ZetoVariant: types.ZetoFungibleV1ABI,
		},
		Params: &types.CreateLockParams{From: "sender@node", UnlockData: []byte{0x01}},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("77", 32)},
		InputStates: []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(true)}},
		OutputStates: []*prototk.EndorsableState{
			{StateDataJson: testCoinStateJSON(false)},
			{StateDataJson: testCoinStateJSON(true)},
			{StateDataJson: string(infoJSON), SchemaId: "lock_info"},
		},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	res, err := h.Prepare(ctx, tx, req)
	require.NoError(t, err)
	require.NotNil(t, res.Transaction)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	assert.NotEmpty(t, params["createArgs"])
	assert.NotEmpty(t, params["spendCommitment"])
	assert.NotEmpty(t, params["cancelCommitment"])
	_ = common.GetInputSize(1)
}

func TestCreateLockPrepare_BadOutputCoinJSON(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CreateLockParams{From: "sender@node", UnlockData: []byte{0x01}},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("77", 32)},
		OutputStates: []*prototk.EndorsableState{{StateDataJson: "not-json"}},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestCreateLockPrepare_InvalidTxID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("66", 32))
	info := types.ZetoLockInfoState{LockID: lockID, SpendCommitment: pldtypes.RandBytes32(), CancelCommitment: pldtypes.RandBytes32()}
	infoJSON, err := json.Marshal(&info)
	require.NoError(t, err)
	h := NewCreateLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CreateLockParams{From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "not-a-tx-id"},
		OutputStates: []*prototk.EndorsableState{
			{StateDataJson: testCoinStateJSON(false)},
			{StateDataJson: string(infoJSON), SchemaId: "lock_info"},
		},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestCreateLockPrepare_MissingLockInfoOutput(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x01", "0x02"},
			B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
			C: []string{"0x07", "0x08"},
		},
	}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_AnonNullifier", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CreateLockParams{From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("55", 32)},
		OutputStates: []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(false)}},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestCreateLockAssemble_MissingVerifiers(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", mockLockAssembleCallbacks("", nil),
		testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, testAssembleSchemas().LockInfoSchema)
	txSpec := &prototk.TransactionSpecification{
		From:          "controller@node",
		TransactionId: "0x" + strings.Repeat("56", 32),
		ContractInfo:  &prototk.ContractInfo{ContractAddress: "0x1234567890123456789012345678901234567890"},
	}
	tx := &types.ParsedTransaction{
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "Zeto_Anon",
			Circuits:  &zetosignerapi.Circuits{types.METHOD_TRANSFER: {Name: "anon", UsesNullifiers: false}},
		},
		Params: &types.CreateLockParams{
			From: "controller@node",
			Recipients: []*types.FungibleTransferParamEntry{
				{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
			},
		},
	}
	req := &prototk.AssembleTransactionRequest{Transaction: txSpec}
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)

	req.ResolvedVerifiers = []*prototk.ResolvedVerifier{{
		Lookup: "controller@node", Verifier: testCoinOwnerBJJ,
		Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
	}}
	_, err = h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCreateLockAssemble_BadContractAddress(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", mockLockAssembleCallbacks("", nil),
		testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, testAssembleSchemas().LockInfoSchema)
	txSpec := &prototk.TransactionSpecification{
		From:          "controller@node",
		TransactionId: "0x" + strings.Repeat("57", 32),
		ContractInfo:  &prototk.ContractInfo{ContractAddress: "bad"},
	}
	tx := &types.ParsedTransaction{
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "Zeto_Anon",
			Circuits:  &zetosignerapi.Circuits{types.METHOD_TRANSFER: {Name: "anon", UsesNullifiers: false}},
		},
		Params: &types.CreateLockParams{
			From: "controller@node",
			Recipients: []*types.FungibleTransferParamEntry{
				{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
			},
		},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: txSpec,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{Lookup: "controller@node", Verifier: testCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
			{Lookup: "controller@node", Verifier: "0x74e71b05854ee819cb9397be01c82570a178d019", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
			{Lookup: "recipient@node", Verifier: testCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
		},
	}
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}
