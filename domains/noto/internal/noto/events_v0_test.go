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
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleV0Data(t *testing.T, n *Noto) (txID pldtypes.Bytes32, data pldtypes.HexBytes) {
	txID = pldtypes.RandBytes32()
	data, err := n.encodeTransactionData(
		context.Background(),
		&types.NotoParsedConfig{Variant: types.NotoVariantLegacy},
		&prototk.TransactionSpecification{
			TransactionId: txID.String(),
		}, []*prototk.EndorsableState{
			{Id: pldtypes.RandBytes32().String()},
		},
	)
	require.NoError(t, err)
	return txID, data
}

func TestHandleEventBatchV0_NotoTransfer(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	txID, data := sampleV0Data(t, n)
	input := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	event := &NotoTransfer_V0_Event{
		Inputs:    []pldtypes.Bytes32{input},
		Outputs:   []pldtypes.Bytes32{output},
		Signature: pldtypes.MustParseHexBytes("0x1234"),
		Data:      data,
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoTransfer],
				DataJson:          string(notoEventJson),
			},
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Equal(t, txID.String(), res.TransactionsComplete[0].TransactionId)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, input.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 1)
	assert.Equal(t, output.String(), res.ConfirmedStates[0].Id)
	assert.Len(t, res.InfoStates, 1)
}

func TestHandleEventBatchV0_NotoTransferBadData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoTransfer],
				DataJson:          "!!wrong",
			}},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatchV0_NotoTransferBadTransactionData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	event := &NotoTransfer_V0_Event{
		Data: pldtypes.MustParseHexBytes("0x00010000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoTransfer],
				DataJson:          string(notoEventJson),
			}},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22047")
}

func TestHandleEventBatchV0_NotoLock(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	txID, data := sampleV0Data(t, n)
	input := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	lockedOutput := pldtypes.RandBytes32()
	event := &NotoLock_V0_Event{
		Inputs:        []pldtypes.Bytes32{input},
		Outputs:       []pldtypes.Bytes32{output},
		LockedOutputs: []pldtypes.Bytes32{lockedOutput},
		Signature:     pldtypes.MustParseHexBytes("0x1234"),
		Data:          data,
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoLock],
				DataJson:          string(notoEventJson),
			},
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Equal(t, txID.String(), res.TransactionsComplete[0].TransactionId)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, input.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 2)
	assert.Equal(t, output.String(), res.ConfirmedStates[0].Id)
	assert.Equal(t, lockedOutput.String(), res.ConfirmedStates[1].Id)
	assert.Len(t, res.InfoStates, 1)
}

func TestHandleEventBatchV0_NotoLockBadData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoLock],
				DataJson:          "!!wrong",
			}},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatchV0_NotoLockBadTransactionData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	event := &NotoTransfer_V0_Event{
		Data: pldtypes.MustParseHexBytes("0x00010000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoLock],
				DataJson:          string(notoEventJson),
			}},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22047")
}

func TestHandleEventBatchV0_NotoUnlock(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	txID, data := sampleV0Data(t, n)
	lockedInput := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	lockedOutput := pldtypes.RandBytes32()
	event := &NotoUnlock_V0_Event{
		LockedInputs:  []pldtypes.Bytes32{lockedInput},
		LockedOutputs: []pldtypes.Bytes32{lockedOutput},
		Outputs:       []pldtypes.Bytes32{output},
		Signature:     pldtypes.MustParseHexBytes("0x1234"),
		Data:          data,
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoUnlock],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{}`,
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Equal(t, txID.String(), res.TransactionsComplete[0].TransactionId)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, lockedInput.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 2)
	assert.Equal(t, lockedOutput.String(), res.ConfirmedStates[0].Id)
	assert.Equal(t, output.String(), res.ConfirmedStates[1].Id)
	assert.Len(t, res.InfoStates, 1)
}

func TestHandleEventBatchV0_NotoUnlockBadData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoUnlock],
				DataJson:          "!!wrong",
			}},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatchV0_NotoUnlockBadTransactionData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	event := &NotoTransfer_V0_Event{
		Data: pldtypes.MustParseHexBytes("0x00010000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoUnlock],
				DataJson:          string(notoEventJson),
			}},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22047")
}

func TestHandleEventBatchV0_NotoUnlockPrepared(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	txID, data := sampleV0Data(t, n)
	lockedInput := pldtypes.RandBytes32()
	event := &NotoUnlockPrepared_V0_Event{
		LockedInputs: []pldtypes.Bytes32{lockedInput},
		Signature:    pldtypes.MustParseHexBytes("0x1234"),
		Data:         data,
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoUnlockPrepared],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{}`,
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Equal(t, txID.String(), res.TransactionsComplete[0].TransactionId)
	assert.Len(t, res.InfoStates, 1)
}

func TestHandleEventBatchV0_NotoUnlockPreparedBadData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoUnlockPrepared],
				DataJson:          "!!wrong",
			}},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatchV0_NotoUnlockPreparedBadTransactionData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	event := &NotoUnlockPrepared_V0_Event{
		Data: pldtypes.MustParseHexBytes("0x00010000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoUnlockPrepared],
				DataJson:          string(notoEventJson),
			}},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22047")
}

func TestHandleEventBatchV0_NotoLockDelegated(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	txID, data := sampleV0Data(t, n)
	event := &NotoLockDelegated_V0_Event{
		Signature: pldtypes.MustParseHexBytes("0x1234"),
		Data:      data,
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoLockDelegated],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{}`,
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Equal(t, txID.String(), res.TransactionsComplete[0].TransactionId)
}

func TestHandleEventBatchV0_NotoLockDelegatedBadData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoLockDelegated],
				DataJson:          "!!wrong",
			}},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatchV0_NotLockDelegatedBadTransactionData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := t.Context()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	event := &NotoLockDelegated_V0_Event{
		Data: pldtypes.MustParseHexBytes("0x00010000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignaturesV0[EventNotoLockDelegated],
				DataJson:          string(notoEventJson),
			}},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22047")
}
