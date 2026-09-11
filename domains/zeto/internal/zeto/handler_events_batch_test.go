/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package zeto

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleEventBatch_V1TransferEvent(t *testing.T) {
	ctx := context.Background()
	z, testCallbacks := newTestZeto()
	require.NotEmpty(t, z.eventsV1.transfer)

	req := &prototk.HandleEventBatchRequest{
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
			ContractConfigJson: pldtypes.JSONString(map[string]interface{}{
				"circuitId": "anon_nullifier",
				"tokenName": "Zeto_AnonNullifier",
			}).Pretty(),
		},
		StateQueryContext: "ctx-v1-transfer",
	}
	testCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{}, nil
	}

	encodedData, err := common.EncodeTransactionData(ctx, &prototk.TransactionSpecification{
		TransactionId: "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000",
	}, nil)
	require.NoError(t, err)

	data, err := json.Marshal(map[string]any{
		"data":    encodedData,
		"inputs":  []string{"100"},
		"outputs": []string{"7980718117603030807695495350922077879582656644717071592146865497574198464253"},
		"proof":   "0x",
	})
	require.NoError(t, err)
	req.Events = []*prototk.OnChainEvent{{
		DataJson: string(data), SoliditySignature: z.eventsV1.transfer,
	}}
	res, err := z.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	assert.NotEmpty(t, res.SpentStates)
	assert.NotEmpty(t, res.ConfirmedStates)
}

func TestHandleEventBatch_KycIdentityAppendsNewStates(t *testing.T) {
	ctx := context.Background()
	z, testCallbacks := newTestZeto()
	require.NotEmpty(t, z.events.identityRegistered)

	rootJSON, _ := json.Marshal(map[string]string{
		"rootIndex": "0x1234567890123456789012345678901234567890123456789012345678901234",
	})
	kycCalls := 0
	testCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		if kycCalls == 0 {
			kycCalls++
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{{DataJson: string(rootJSON)}},
			}, nil
		}
		return &prototk.FindAvailableStatesResponse{}, nil
	}

	encodedData, err := common.EncodeTransactionData(ctx, &prototk.TransactionSpecification{
		TransactionId: "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000",
	}, nil)
	require.NoError(t, err)
	identityData, err := json.Marshal(map[string]any{
		"data":      encodedData,
		"publicKey": []string{"7980718117603030807695495350922077879582656644717071592146865497574198464252", "7980718117603030807695495350922077879582656644717071592146865497574198464251"},
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
			ContractConfigJson: pldtypes.JSONString(map[string]interface{}{
				"circuitId": "anon_nullifier_kyc_transfer",
				"tokenName": "Zeto_AnonNullifierKyc",
			}).Pretty(),
		},
		StateQueryContext: "ctx-kyc-batch",
		Events: []*prototk.OnChainEvent{{
			DataJson:          string(identityData),
			SoliditySignature: z.events.identityRegistered,
		}},
	}
	res, err := z.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	assert.NotEmpty(t, res.NewStates)
}

func batchNullifierKycMerkleMock() func(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	data0, _ := json.Marshal(map[string]string{"rootIndex": "0x1234567890123456789012345678901234567890123456789012345678901234"})
	data1, _ := json.Marshal(map[string]string{
		"index": "0x5f5d5e50a650a20986d496e6645ea31770758d924796f0dfc5ac2ad234b03e30",
		"refKey": "0x789c99b9a2196addb3ac11567135877e8b86bc9b5f7725808a79757fd36b2a2a", "type": "0x02",
		"leftChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
	})
	data2, _ := json.Marshal(map[string]string{
		"index": "0x8bdc1e9686bc722ac480c60b35090ec521a2d72102b9bbb3043982a138d27514",
		"refKey": "0xb2479166472a0635433159a876d6d8f9b904aa0b9249cd1b596750205a2e2c01", "type": "0x02",
		"leftChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
	})
	kycCount := 0
	return func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		switch kycCount {
		case 0, 5, 10, 15:
			kycCount++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data0)}}}, nil
		case 1, 3, 6, 8, 11, 13, 16, 18:
			kycCount++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data1)}}}, nil
		case 2, 4, 7, 9, 12, 14, 17, 19:
			kycCount++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data2)}}}, nil
		}
		kycCount++
		return &prototk.FindAvailableStatesResponse{}, nil
	}
}

func TestHandleEventBatch_KycTokenMintAndIdentity(t *testing.T) {
	ctx := context.Background()
	z, testCallbacks := newTestZeto()
	testCallbacks.MockFindAvailableStates = batchNullifierKycMerkleMock()

	encodedData, err := common.EncodeTransactionData(ctx, &prototk.TransactionSpecification{
		TransactionId: "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000",
	}, nil)
	require.NoError(t, err)

	mintData, err := json.Marshal(map[string]any{
		"data":      encodedData,
		"outputs":   []string{"7980718117603030807695495350922077879582656644717071592146865497574198464253"},
		"submitter": "0x74e71b05854ee819cb9397be01c82570a178d019",
	})
	require.NoError(t, err)
	identityData, err := json.Marshal(map[string]any{
		"data":      encodedData,
		"publicKey": []string{"7980718117603030807695495350922077879582656644717071592146865497574198464252", "7980718117603030807695495350922077879582656644717071592146865497574198464251"},
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
			ContractConfigJson: pldtypes.JSONString(map[string]interface{}{
				"circuitId": "anon_nullifier_kyc_transfer",
				"tokenName": "Zeto_AnonNullifierKyc",
			}).Pretty(),
		},
		StateQueryContext: "ctx-kyc-mint-identity",
		Events: []*prototk.OnChainEvent{
			{DataJson: string(mintData), SoliditySignature: z.events.mint},
			{DataJson: string(identityData), SoliditySignature: z.events.identityRegistered},
		},
	}
	res, err := z.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	assert.Len(t, res.TransactionsComplete, 1)
	assert.NotEmpty(t, res.NewStates)
}

func TestHandleEventBatch_LockSpentConfirmsPinnedOutputs(t *testing.T) {
	ctx := context.Background()
	z, testCallbacks := newTestZeto()
	z.lockInfoSchema = &prototk.StateSchema{Id: "lock_info"}
	testCallbacks.MockFindAvailableStates = batchNullifierKycMerkleMock()

	lockID := pldtypes.MustParseBytes32("0xccdd000000000000000000000000000000000000000000000000000000000002")
	spendOut := "0x0d7b11e7bb9f808761aba8e35b8c57839d8c17b2479f7a89b88f5dfa58d0df13"
	li := &types.ZetoLockInfoState{LockID: lockID, SpendOutputs: []string{spendOut}}
	liJSON, err := json.Marshal(li)
	require.NoError(t, err)
	origFind := testCallbacks.MockFindAvailableStates
	testCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		if req.SchemaId == "lock_info" {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{{Id: lockID.HexString0xPrefix(), DataJson: string(liJSON)}},
			}, nil
		}
		return origFind(ctx, req)
	}

	encodedData, err := common.EncodeTransactionData(ctx, &prototk.TransactionSpecification{
		TransactionId: "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000",
	}, nil)
	require.NoError(t, err)
	lockSpent, err := json.Marshal(map[string]any{
		"txId":          "0xaabb000000000000000000000000000000000000000000000000000000000001",
		"lockId":        lockID.HexString0xPrefix(),
		"spender":       "0x74e71b05854ee819cb9397be01c82570a178d019",
		"lockedInputs":  []string{"300"},
		"lockedOutputs": []string{},
		"outputs":       []string{"7980718117603030807695495350922077879582656644717071592146865497574198464253"},
		"proof":         "0x",
		"data":          encodedData,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
			ContractConfigJson: pldtypes.JSONString(map[string]interface{}{
				"circuitId": "anon_nullifier",
				"tokenName": "Zeto_AnonNullifier",
			}).Pretty(),
		},
		StateQueryContext: "ctx-lock-spent-confirm",
		Events: []*prototk.OnChainEvent{{
			DataJson: string(lockSpent), SoliditySignature: z.eventsV1.zetoLockSpent,
		}},
	}
	res, err := z.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	assert.Len(t, res.TransactionsComplete, 1)
	assert.NotEmpty(t, res.ConfirmedStates)
}

func TestHandleEventBatch_LockCancelledSpendsLockInfo(t *testing.T) {
	ctx := context.Background()
	z, testCallbacks := newTestZeto()
	z.lockInfoSchema = &prototk.StateSchema{Id: "lock_info"}
	testCallbacks.MockFindAvailableStates = batchNullifierKycMerkleMock()

	lockID := pldtypes.MustParseBytes32("0xeeff000000000000000000000000000000000000000000000000000000000003")
	li := &types.ZetoLockInfoState{LockID: lockID}
	liJSON, err := json.Marshal(li)
	require.NoError(t, err)
	origFind := testCallbacks.MockFindAvailableStates
	testCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		if req.SchemaId == "lock_info" {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{{Id: lockID.HexString0xPrefix(), DataJson: string(liJSON)}},
			}, nil
		}
		return origFind(ctx, req)
	}

	encodedData, err := common.EncodeTransactionData(ctx, &prototk.TransactionSpecification{
		TransactionId: "0x40e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000",
	}, nil)
	require.NoError(t, err)
	lockCancelled, err := json.Marshal(map[string]any{
		"txId":          "0xbbcc000000000000000000000000000000000000000000000000000000000002",
		"lockId":        lockID.HexString0xPrefix(),
		"spender":       "0x74e71b05854ee819cb9397be01c82570a178d019",
		"lockedInputs":  []string{"300"},
		"lockedOutputs": []string{},
		"outputs":       []string{"7980718117603030807695495350922077879582656644717071592146865497574198464253"},
		"proof":         "0x",
		"data":          encodedData,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
			ContractConfigJson: pldtypes.JSONString(map[string]interface{}{
				"circuitId": "anon_nullifier",
				"tokenName": "Zeto_AnonNullifier",
			}).Pretty(),
		},
		StateQueryContext: "ctx-lock-cancelled",
		Events: []*prototk.OnChainEvent{{
			DataJson: string(lockCancelled), SoliditySignature: z.eventsV1.zetoLockCancelled,
		}},
	}
	res, err := z.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	assert.Len(t, res.TransactionsComplete, 1)
	assert.NotEmpty(t, res.SpentStates)
}

func TestHandleEventBatch_LockEventUpdatesBothSMTs(t *testing.T) {
	ctx := context.Background()
	z, testCallbacks := newTestZeto()
	testCallbacks.MockFindAvailableStates = batchNullifierKycMerkleMock()

	encodedData, err := common.EncodeTransactionData(ctx, &prototk.TransactionSpecification{
		TransactionId: "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000",
	}, nil)
	require.NoError(t, err)
	lockData, err := json.Marshal(map[string]any{
		"data":          encodedData,
		"inputs":        []string{"100"},
		"outputs":       []string{"7980718117603030807695495350922077879582656644717071592146865497574198464253"},
		"lockedOutputs": []string{"7980718117603030807695495350922077879582656644717071592146865497574198464254"},
		"submitter":     "0x74e71b05854ee819cb9397be01c82570a178d019",
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
			ContractConfigJson: pldtypes.JSONString(map[string]interface{}{
				"circuitId": "anon_nullifier",
				"tokenName": "Zeto_AnonNullifier",
			}).Pretty(),
		},
		StateQueryContext: "ctx-lock-dual-smt",
		Events: []*prototk.OnChainEvent{{
			DataJson: string(lockData), SoliditySignature: z.events.lock,
		}},
	}
	res, err := z.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	assert.Len(t, res.TransactionsComplete, 1)
	assert.NotEmpty(t, res.NewStates)
}

func TestHandleEventBatch_WithdrawUpdatesSMT(t *testing.T) {
	ctx := context.Background()
	z, testCallbacks := newTestZeto()
	testCallbacks.MockFindAvailableStates = batchNullifierKycMerkleMock()

	encodedData, err := common.EncodeTransactionData(ctx, &prototk.TransactionSpecification{
		TransactionId: "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000",
	}, nil)
	require.NoError(t, err)
	withdrawData, err := json.Marshal(map[string]any{
		"data":      encodedData,
		"inputs":    []string{"7980718117603030807695495350922077879582656644717071592146865497574198464253"},
		"output":    "7980718117603030807695495350922077879582656644717071592146865497574198464253",
		"submitter": "0x74e71b05854ee819cb9397be01c82570a178d019",
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
			ContractConfigJson: pldtypes.JSONString(map[string]interface{}{
				"circuitId": "anon_nullifier",
				"tokenName": "Zeto_AnonNullifier",
			}).Pretty(),
		},
		StateQueryContext: "ctx-withdraw-batch",
		Events: []*prototk.OnChainEvent{{
			DataJson: string(withdrawData), SoliditySignature: z.events.withdraw,
		}},
	}
	res, err := z.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	assert.Len(t, res.TransactionsComplete, 1)
	assert.NotEmpty(t, res.NewStates)
}
