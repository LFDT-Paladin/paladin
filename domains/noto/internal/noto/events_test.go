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
	"encoding/json"
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleV1Data(t *testing.T, n *Noto) (data pldtypes.HexBytes) {
	data, err := n.encodeTransactionData(
		context.Background(),
		&types.NotoParsedConfig{Variant: types.NotoVariantDefault},
		&prototk.TransactionSpecification{
			TransactionId: pldtypes.RandBytes32().String(), // not used
		}, []*prototk.EndorsableState{
			{Id: pldtypes.RandBytes32().String()},
		},
	)
	require.NoError(t, err)
	return data
}

func TestHandleEventBatch_NotoTransfer(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: mustParseJSON(types.NotoParsedConfig{Variant: types.NotoVariantDefault}),
	})
	require.NoError(t, err)

	txId := pldtypes.RandBytes32()
	input := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	event := &NotoTransfer_Event{
		TxId:    txId,
		Inputs:  []pldtypes.Bytes32{input},
		Outputs: []pldtypes.Bytes32{output},
		Proof:   pldtypes.MustParseHexBytes("0x1234"),
		Data:    sampleV1Data(t, n),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventTransfer],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				Variant: types.NotoVariantDefault,
			}),
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, input.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 1)
	assert.Equal(t, output.String(), res.ConfirmedStates[0].Id)
	require.Len(t, res.InfoStates, 1)
}

func TestHandleEventBatch_NotoTransfer_Nullifiers(t *testing.T) {
	callIdx := 0
	mockCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			defer func() { callIdx++ }()
			if callIdx == 0 {
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{
						{
							DataJson: `{"rootIndex":"0x9bc7adede8e6ef3f5a6a3a466a9d9f115d040e8891f77023ebc4825196b55726","smtName":"smt_noto_0xfc401339d61baabc22091432c1148d9ccd1088d0"}`,
						},
					},
				}, nil
			} else {
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{
						{
							DataJson: `{"index":"0x78f5fe3a8fd47d7bb4e9f71c4c9318cf83df700499b683201491c9459194cff5","leftChild":"0x0000000000000000000000000000000000000000000000000000000000000000","refKey":"0xc95e1f2a738628320dbc9c5378df861ff2baf2343acc7b4ac8c043f2e6f58209","rightChild":"0x0000000000000000000000000000000000000000000000000000000000000000","type":"0x02"}`,
						},
					},
				}, nil
			}
		},
		MockLocalNodeName: func() (*prototk.LocalNodeNameResponse, error) {
			return &prototk.LocalNodeNameResponse{
				Name: "node1",
			}, nil
		},
	}
	n := &Noto{
		Callbacks: mockCallbacks,
		merkleTreeRootSchema: &prototk.StateSchema{
			Id: "merkle_tree_root",
		},
		merkleTreeNodeSchema: &prototk.StateSchema{
			Id: "merkle_tree_node",
		},
	}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: mustParseJSON(types.NotoParsedConfig{Variant: types.NotoVariantNullifier}),
	})
	require.NoError(t, err)

	input := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	event := &NotoTransfer_Event{
		Inputs:  []pldtypes.Bytes32{input},
		Outputs: []pldtypes.Bytes32{output},
		Proof:   pldtypes.MustParseHexBytes("0x1234"),
		Data:    sampleV1Data(t, n),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventTransfer],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				Variant: types.NotoVariantNullifier,
			}),
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, input.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 1)
	assert.Equal(t, output.String(), res.ConfirmedStates[0].Id)
	require.Len(t, res.InfoStates, 1)
}

func TestHandleEventBatch_NotoTransfer_Nullifiers_Error1(t *testing.T) {
	mockCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("bad call")
		},
		MockLocalNodeName: func() (*prototk.LocalNodeNameResponse, error) {
			return &prototk.LocalNodeNameResponse{
				Name: "node1",
			}, nil
		},
	}
	n := &Noto{
		Callbacks: mockCallbacks,
		merkleTreeRootSchema: &prototk.StateSchema{
			Id: "merkle_tree_root",
		},
		merkleTreeNodeSchema: &prototk.StateSchema{
			Id: "merkle_tree_node",
		},
	}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: mustParseJSON(types.NotoParsedConfig{Variant: types.NotoVariantNullifier}),
	})
	require.NoError(t, err)

	input := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	event := &NotoTransfer_Event{
		Inputs:  []pldtypes.Bytes32{input},
		Outputs: []pldtypes.Bytes32{output},
		Proof:   pldtypes.MustParseHexBytes("0x1234"),
		Data:    sampleV1Data(t, n),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventTransfer],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				Variant: types.NotoVariantNullifier,
			}),
		},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.EqualError(t, err, "bad call")
}

func TestHandleEventBatch_NotoTransferBadData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: mustParseJSON(types.NotoParsedConfig{Variant: types.NotoVariantDefault}),
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventTransfer],
				DataJson:          "!!wrong",
			}},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				Variant: types.NotoVariantDefault,
			}),
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatch_NotoTransferBadTransactionData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: mustParseJSON(types.NotoParsedConfig{Variant: types.NotoVariantDefault}),
	})
	require.NoError(t, err)

	txId := pldtypes.RandBytes32()
	event := &NotoTransfer_Event{
		TxId: txId,
		Data: pldtypes.MustParseHexBytes("0x00020000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventTransfer],
				DataJson:          string(notoEventJson),
			}},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				Variant: types.NotoVariantDefault,
			}),
		},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22045")
}

func TestHandleEventBatch_NotoLock(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: mustParseJSON(types.NotoParsedConfig{Variant: types.NotoVariantDefault}),
	})
	require.NoError(t, err)

	txId := pldtypes.RandBytes32()
	lockId := pldtypes.RandBytes32()
	input := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	lockedOutput := pldtypes.RandBytes32()
	owner := (*pldtypes.EthAddress)(pldtypes.RandAddress())
	event := &NotoLockCreated_Event{
		TxId:          txId,
		LockID:        lockId,
		Owner:         owner,
		Inputs:        []pldtypes.Bytes32{input},
		Outputs:       []pldtypes.Bytes32{output},
		LockedOutputs: []pldtypes.Bytes32{lockedOutput},
		Proof:         pldtypes.MustParseHexBytes("0x1234"),
		Data:          sampleV1Data(t, n),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventNotoLockCreated],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				Variant: types.NotoVariantDefault,
			}),
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, input.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 2)
	assert.Equal(t, output.String(), res.ConfirmedStates[0].Id)
	assert.Equal(t, lockedOutput.String(), res.ConfirmedStates[1].Id)
	require.Len(t, res.InfoStates, 1)
}

func TestHandleEventBatch_NotoLockBadData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: mustParseJSON(types.NotoParsedConfig{Variant: types.NotoVariantDefault}),
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventNotoLockCreated],
				DataJson:          "!!wrong",
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				Variant: types.NotoVariantDefault,
			}),
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatch_NotoLockBadTransactionData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: mustParseJSON(types.NotoParsedConfig{Variant: types.NotoVariantDefault}),
	})
	require.NoError(t, err)

	txId := pldtypes.RandBytes32()
	lockId := pldtypes.RandBytes32()
	owner := (*pldtypes.EthAddress)(pldtypes.RandAddress())
	event := &NotoLockCreated_Event{
		TxId:          txId,
		LockID:        lockId,
		Owner:         owner,
		Inputs:        []pldtypes.Bytes32{},
		Outputs:       []pldtypes.Bytes32{},
		LockedOutputs: []pldtypes.Bytes32{},
		Proof:         pldtypes.MustParseHexBytes("0x1234"),
		Data:          pldtypes.MustParseHexBytes("0x00020000"), // Bad transaction data
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventNotoLockCreated],
				DataJson:          string(notoEventJson),
			}},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				Variant: types.NotoVariantDefault,
			}),
		},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22045")
}

func TestHandleEventBatch_LockSpent(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: mustParseJSON(types.NotoParsedConfig{Variant: types.NotoVariantDefault}),
	})
	require.NoError(t, err)

	txId := pldtypes.RandBytes32()
	lockId := pldtypes.RandBytes32()
	lockedInput := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	spender := (*pldtypes.EthAddress)(pldtypes.RandAddress())
	outerData := pldtypes.RandHex(32)
	proof := pldtypes.RandHex(32)

	event := &NotoLockSpentOrCancelled_Event{
		TxId:    txId,
		LockID:  lockId,
		Spender: spender,
		Inputs:  []pldtypes.Bytes32{lockedInput},
		Outputs: []pldtypes.Bytes32{output},
		TxData:  sampleV1Data(t, n),
		Proof:   pldtypes.HexBytes(proof),
		Data:    pldtypes.HexBytes(outerData),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventNotoLockSpent],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				Variant: types.NotoVariantDefault,
			}),
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, lockedInput.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 1)
	assert.Equal(t, output.String(), res.ConfirmedStates[0].Id)
	require.Len(t, res.InfoStates, 1)
}
