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
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestGetSpendLockABI_V1UsesLockableCapabilityOnChainShape(t *testing.T) {
	abi := types.LockableCapabilitySpendLockABI
	require.Equal(t, types.METHOD_SPEND_LOCK, abi.Name)
	require.Len(t, abi.Inputs, 3)
	assert.Equal(t, "lockId", abi.Inputs[0].Name)
	assert.Equal(t, "spendArgs", abi.Inputs[1].Name)
	assert.Equal(t, "data", abi.Inputs[2].Name)
}

func TestSpendLockValidateParams_WireJSON(t *testing.T) {
	ctx := context.Background()
	h := spendLockHandler{baseHandler: baseHandler{name: "zeto"}}
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32))
	emptyData := pldtypes.HexBytes{}
	params := types.SpendLockParams{
		LockId: lockID,
		From:   "alice@node",
		Data:   emptyData,
	}
	paramsJSON, err := json.Marshal(&params)
	require.NoError(t, err)
	parsed, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, string(paramsJSON))
	require.NoError(t, err)
	sp, ok := parsed.(*types.SpendLockParams)
	require.True(t, ok)
	assert.Equal(t, lockID, sp.LockId)
	assert.Equal(t, "alice@node", sp.From)
}

func TestSpendLockPrepare_V1EncodesZetoSpendLockArgs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("11", 32))
	spendData := pldtypes.HexBytes{0xde, 0xad}
	lockInfoJSON, err := json.Marshal(&types.ZetoLockInfoState{
		LockID:    lockID,
		SpendData: spendData,
	})
	require.NoError(t, err)

	h := spendLockHandler{
		baseHandler: baseHandler{
			name: "zeto",
			stateSchemas: &common.StateSchemas{
				CoinSchema:     &prototk.StateSchema{Id: "coin"},
				LockInfoSchema: &prototk.StateSchema{Id: "lock_info"},
			},
		},
		callbacks: &domain.MockDomainCallbacks{
			MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
				if req.SchemaId == "lock_info" {
					return &prototk.FindAvailableStatesResponse{
						States: []*prototk.StoredState{{DataJson: string(lockInfoJSON)}},
					}, nil
				}
				return &prototk.FindAvailableStatesResponse{}, nil
			},
		},
	}

	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{
			From: "sender-identity",
		},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName:   "Zeto_AnonNullifier",
			ZetoVariant: types.ZetoFungibleV1ABI,
			Circuits: &zetosignerapi.Circuits{
				// v0.5 Zeto_AnonNullifier: lock spend uses the plain "anon" circuit + Groth16Verifier_Anon (see zeto ignition zeto_anon_nullifier).
				"transferLocked": {Name: "anon", UsesNullifiers: false},
			},
		},
		Params: &types.SpendLockParams{
			LockId: lockID,
			From:   "sender-identity",
			Data:   pldtypes.HexBytes{},
		},
	}

	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x01", "0x02"},
			B: []*corepb.B_Item{
				{Items: []string{"0x03", "0x04"}},
				{Items: []string{"0x05", "0x06"}},
			},
			C: []string{"0x07", "0x08"},
		},
		PublicInputs: map[string]string{
			"nullifiers": "0x09,0x0a",
			"root":       "0x0b",
		},
	}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)

	unlockedCoin := `{"salt":"0x01","owner":"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922","amount":"0x0a","locked":false}`
	lockedCoin := `{"salt":"0x02","owner":"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922","amount":"0x05","locked":true}`

	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "0x" + strings.Repeat("33", 32),
		},
		InputStates: []*prototk.EndorsableState{
			{StateDataJson: `{"salt":"0xaa","owner":"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922","amount":"0x0f","locked":true}`},
		},
		OutputStates: []*prototk.EndorsableState{
			{StateDataJson: unlockedCoin},
			{StateDataJson: lockedCoin},
		},
		AttestationResult: []*prototk.AttestationResult{{
			Name:            "sender",
			AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType:     strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK),
			Payload:         payload,
		}},
	}

	res, err := h.Prepare(ctx, tx, req)
	require.NoError(t, err)
	require.NotNil(t, res.Transaction)

	var fn abiJSONFields
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.FunctionAbiJson), &fn))
	assert.Equal(t, "spendLock", fn.Name)
	require.Len(t, fn.Inputs, 3)

	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	spendArgsHex, _ := params["spendArgs"].(string)
	assert.NotEmpty(t, spendArgsHex)
	assert.NotContains(t, params, "from")
	_, ok := params["data"]
	assert.True(t, ok)
	lockHex, _ := params["lockId"].(string)
	assert.Equal(t, lockID.HexString0xPrefix(), lockHex)

	spendArgsBytes := pldtypes.MustParseHexBytes(spendArgsHex)
	decoded, err := zetoSpendLockArgsTupleABI.DecodeABIData(spendArgsBytes, 0)
	require.NoError(t, err)
	require.Len(t, decoded.Children, 1)
	inner := decoded.Children[0]
	require.NotNil(t, inner)
	require.Len(t, inner.Children, 5)
	j, err := inner.JSON()
	require.NoError(t, err)
	js := string(j)
	assert.Contains(t, js, `"txId"`)
	assert.Contains(t, js, `"lockedOutputs"`)
	assert.Contains(t, js, `"outputs"`)
	assert.Contains(t, js, `"proof"`)
	assert.Contains(t, js, `"data"`)
	assert.Contains(t, js, `"data":"dead"`)
}

func TestZetoSpendLockArgsTupleABI_RoundTrip(t *testing.T) {
	ctx := context.Background()
	wire := zetoSpendLockArgsWireJSON{
		TxID:          "0x" + strings.Repeat("aa", 32),
		LockedOutputs: []string{"0x01"},
		Outputs:       []string{"0x02", "0x03"},
		Proof:         "0xabcd",
		Data:          "0xbeef",
	}
	j, err := json.Marshal([]any{wire})
	require.NoError(t, err)
	enc, err := zetoSpendLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, j)
	require.NoError(t, err)
	require.NotEmpty(t, enc)
	decoded, err := zetoSpendLockArgsTupleABI.DecodeABIDataCtx(ctx, enc, 0)
	require.NoError(t, err)
	require.Len(t, decoded.Children, 1)
	inner := decoded.Children[0]
	require.NotNil(t, inner)
	require.Len(t, inner.Children, 5)
	out, err := inner.JSON()
	require.NoError(t, err)
	js := string(out)
	assert.Contains(t, js, `"txId"`)
	assert.Contains(t, js, `"lockedOutputs"`)
	assert.Contains(t, js, `"outputs"`)
	assert.Contains(t, js, `"proof"`)
	assert.Contains(t, js, `"data"`)
}

func TestZetoSpendLockArgsTupleABI_FirstWordMatchesEthers(t *testing.T) {
	ctx := context.Background()
	wire := zetoSpendLockArgsWireJSON{
		TxID:          "0x" + strings.Repeat("11", 32),
		LockedOutputs: []string{"0x01"},
		Outputs:       []string{"0x02"},
		Proof:         "0xabcd",
		Data:          "0x",
	}
	j, err := json.Marshal([]any{wire})
	require.NoError(t, err)
	enc, err := zetoSpendLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, j)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(enc), 32)
	got := hex.EncodeToString(enc[:32])
	const ethersFirstWord = "0000000000000000000000000000000000000000000000000000000000000020"
	assert.Equal(t, ethersFirstWord, got, "must match ethers tuple encode; mismatch breaks Solidity abi.decode(spendArgs, (IZetoLockableCapability.ZetoSpendLockArgs))")
}

func strPtr(s string) *string { return &s }

func TestSpendRecipientsFromLockInfo(t *testing.T) {
	ctx := context.Background()
	amt := pldtypes.Uint64ToUint256(5)
	recipients := []*types.FungibleTransferParamEntry{
		{To: "alice@node", Amount: amt, Data: pldtypes.HexBytes{}},
	}
	enc, err := json.Marshal(recipients)
	require.NoError(t, err)

	lockID := pldtypes.RandBytes32()
	got, err := spendRecipientsFromLockInfo(ctx, &types.ZetoLockInfoState{LockID: lockID, SpendData: enc})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "alice@node", got[0].To)
	assert.Equal(t, uint64(5), got[0].Amount.Int().Uint64())

	_, err = spendRecipientsFromLockInfo(ctx, &types.ZetoLockInfoState{LockID: lockID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD210151")
}

func TestSpendLockInitAndEndorse(t *testing.T) {
	ctx := context.Background()
	h := NewSpendLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, nil)
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{From: "alice@node"},
		Params:      &types.SpendLockParams{From: "alice@node"},
	}
	res, err := h.Init(ctx, tx, &prototk.InitTransactionRequest{})
	require.NoError(t, err)
	require.Len(t, res.RequiredVerifiers, 1)
	endorseRes, err := h.Endorse(ctx, tx, &prototk.EndorseTransactionRequest{})
	require.NoError(t, err)
	assert.Nil(t, endorseRes)
}

func TestSpendLockPrepare_V0UsesDiscreteProofParams(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("55", 32))
	spendData := pldtypes.HexBytes{0x01}
	lockInfoJSON, err := json.Marshal(&types.ZetoLockInfoState{LockID: lockID, SpendData: spendData})
	require.NoError(t, err)

	h := spendLockHandler{
		baseHandler: baseHandler{
			name: "zeto",
			stateSchemas: &common.StateSchemas{
				CoinSchema:     &prototk.StateSchema{Id: "coin"},
				LockInfoSchema: &prototk.StateSchema{Id: "lock_info"},
			},
		},
		callbacks: mockLockInfoCallbacks(string(lockInfoJSON)),
	}
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
			TokenName:   constants.TOKEN_ANON,
			ZetoVariant: types.ZetoFungibleV0ABI,
		},
		Params: &types.SpendLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("66", 32)},
		InputStates: []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(true)}},
		OutputStates: []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(false)}},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	res, err := h.Prepare(ctx, tx, req)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	_, hasSpendArgs := params["spendArgs"]
	assert.False(t, hasSpendArgs)
	assert.NotNil(t, params["proof"])
}

func TestSpendLockPrepare_NullifierV0UsesPublicInputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("66", 32))
	lockInfoJSON, err := json.Marshal(&types.ZetoLockInfoState{LockID: lockID, SpendData: []byte{0x01}})
	require.NoError(t, err)

	h := spendLockHandler{
		baseHandler: baseHandler{
			name: "zeto",
			stateSchemas: &common.StateSchemas{
				CoinSchema:     &prototk.StateSchema{Id: "coin"},
				LockInfoSchema: &prototk.StateSchema{Id: "lock_info"},
			},
		},
		callbacks: mockLockInfoCallbacks(string(lockInfoJSON)),
	}
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
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: constants.TOKEN_ANON_NULLIFIER, ZetoVariant: types.ZetoFungibleV0ABI},
		Params:       &types.SpendLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("44", 32)},
		InputStates: []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(true)}},
		OutputStates: []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(false)}},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	res, err := h.Prepare(ctx, tx, req)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	_, hasInputs := params["inputs"]
	assert.False(t, hasInputs)
	assert.NotNil(t, params["nullifiers"])
}

func TestSpendLockLoadLockInfoByLockID_NilSchema(t *testing.T) {
	ctx := context.Background()
	h := spendLockHandler{
		baseHandler: baseHandler{
			name: "zeto",
			stateSchemas: &common.StateSchemas{CoinSchema: &prototk.StateSchema{Id: "coin"}},
		},
	}
	got, err := h.loadLockInfoByLockID(ctx, pldtypes.RandBytes32(), "ctx")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSpendLockLoadLockInfoByLockID_NotFoundAndUnmarshal(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("50", 32))
	h := spendLockHandler{
		baseHandler: baseHandler{
			name:         "zeto",
			stateSchemas: &common.StateSchemas{LockInfoSchema: &prototk.StateSchema{Id: "lock_info"}},
		},
		callbacks: &domain.MockDomainCallbacks{
			MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
				return &prototk.FindAvailableStatesResponse{}, nil
			},
		},
	}
	got, err := h.loadLockInfoByLockID(ctx, lockID, "ctx")
	require.NoError(t, err)
	assert.Nil(t, got)

	h.callbacks = &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{{DataJson: "not-json"}},
			}, nil
		},
	}
	_, err = h.loadLockInfoByLockID(ctx, lockID, "ctx")
	require.Error(t, err)
}

func TestSpendLockValidateParams_Errors(t *testing.T) {
	ctx := context.Background()
	h := spendLockHandler{baseHandler: baseHandler{name: "zeto"}}
	_, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, `{"lockId":"0x`+strings.Repeat("00", 32)+`"}`)
	require.Error(t, err)
}

type abiJSONFields struct {
	Name   string `json:"name"`
	Inputs []struct {
		Name string `json:"name"`
	} `json:"inputs"`
}
