/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestCancelLockValidateParams(t *testing.T) {
	ctx := context.Background()
	h := NewCancelLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32))
	paramsJSON := `{"lockId":"` + lockID.HexString0xPrefix() + `","from":"alice@node","data":"0x"}`
	parsed, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, paramsJSON)
	require.NoError(t, err)
	cp, ok := parsed.(*types.CancelLockParams)
	require.True(t, ok)
	assert.Equal(t, lockID, cp.LockId)
	assert.Equal(t, "alice@node", cp.From)
}

func TestGetCancelLockABI_V1UsesLockableCapability(t *testing.T) {
	abi := types.LockableCapabilityCancelLockABI
	require.Equal(t, types.METHOD_CANCEL_LOCK, abi.Name)
	require.Len(t, abi.Inputs, 3)
	assert.Equal(t, "lockId", abi.Inputs[0].Name)
	assert.Equal(t, "cancelArgs", abi.Inputs[1].Name)
	assert.Equal(t, "data", abi.Inputs[2].Name)
}

func TestCancelLockInitAndEndorse(t *testing.T) {
	ctx := context.Background()
	h := NewCancelLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, nil)
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{From: "alice@node"},
		Params:      &types.CancelLockParams{From: "alice@node"},
	}
	res, err := h.Init(ctx, tx, &prototk.InitTransactionRequest{})
	require.NoError(t, err)
	require.Len(t, res.RequiredVerifiers, 1)
	endorseRes, err := h.Endorse(ctx, tx, &prototk.EndorseTransactionRequest{})
	require.NoError(t, err)
	assert.Nil(t, endorseRes)
}

func TestCancelLockPrepare_EncodesCancelArgs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("88", 32))
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{LockID: lockID, CancelData: cancelData}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewCancelLockHandler("zeto", mockLockInfoCallbacks(string(lockInfoJSON)), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
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
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_AnonNullifier", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("99", 32)},
		OutputStates: []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(false)}},
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
	assert.NotEmpty(t, params["cancelArgs"])
	assert.Equal(t, lockID.HexString0xPrefix(), params["lockId"])
}

func TestCancelLockPrepare_V0DiscreteProofParams(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("77", 32))
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{LockID: lockID, CancelData: cancelData}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewCancelLockHandler("zeto", mockLockInfoCallbacks(string(lockInfoJSON)), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
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
		DomainConfig: &types.DomainInstanceConfig{TokenName: constants.TOKEN_ANON, ZetoVariant: types.ZetoFungibleV0ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("88", 32)},
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
	_, hasCancelArgs := params["cancelArgs"]
	assert.False(t, hasCancelArgs)
	assert.NotNil(t, params["proof"])
}

func TestCancelLockPrepare_V1PinnedCancelOutputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("99", 32))
	cancelID := "0x" + strings.Repeat("dd", 32)
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{
		LockID: lockID, CancelData: cancelData, CancelOutputs: []string{cancelID},
	}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})

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
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("aa", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.NoError(t, err)
}

func TestCancelLockPrepare_NullifierTokenUsesPublicInputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("aa", 32))
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{LockID: lockID, CancelData: cancelData}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewCancelLockHandler("zeto", mockLockInfoCallbacks(string(lockInfoJSON)), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
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
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("bb", 32)},
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
	assert.Equal(t, "0x0b", params["root"])
}

func TestCancelLockPinnedCoinsForPrepare_Revert(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("cc", 32))
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks("", map[string]string{}),
		&prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	_, err := h.pinnedCoinsForPrepare(ctx, lockID, []string{"0x" + strings.Repeat("11", 32)}, "ctx", false)
	require.Error(t, err)
}

func TestCancelLockValidateParams_Errors(t *testing.T) {
	ctx := context.Background()
	h := NewCancelLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, nil)
	_, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, `{"lockId":"0x`+strings.Repeat("00", 32)+`"}`)
	require.Error(t, err)
}
