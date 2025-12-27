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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/require"
)

func TestLoadLockInfoOk(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfoV1"},
	}
	lockID := pldtypes.RandBytes32()
	existingState := &prototk.StoredState{
		Id:       pldtypes.RandBytes32().String(),
		SchemaId: "lockInfoV1",
		DataJson: fmt.Sprintf(`{"lockId": "%s"}`, lockID),
	}
	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{existingState},
		}, nil
	}
	ctx := context.Background()

	lock, err := n.loadLockInfoV1(ctx, "query-context", lockID)
	require.NoError(t, err)
	require.Equal(t, lockID, lock.lockInfo.LockID)
	require.Equal(t, existingState.Id, lock.id.String())
	require.Equal(t, existingState.Id, lock.stateRef.Id)
	require.Equal(t, existingState.SchemaId, lock.stateRef.SchemaId)
}

func TestLoadLockInfoBadData(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfoV1"},
	}
	lockID := pldtypes.RandBytes32()
	existingState := &prototk.StoredState{
		Id:       pldtypes.RandBytes32().String(),
		SchemaId: "lockInfoV1",
		DataJson: `{"lockId": {"wrong":true}}`,
	}
	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{existingState},
		}, nil
	}
	ctx := context.Background()

	_, err := n.loadLockInfoV1(ctx, "query-context", lockID)
	require.Regexp(t, "PD200040", err)
}

func TestLoadLockInfoNotFound(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfoV1"},
	}
	lockID := pldtypes.RandBytes32()
	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{}, nil
	}
	ctx := context.Background()

	_, err := n.loadLockInfoV1(ctx, "query-context", lockID)
	require.Regexp(t, "PD200028", err)
}

func TestLoadLockInfoLoadFail(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfoV1"},
	}
	lockID := pldtypes.RandBytes32()
	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return nil, fmt.Errorf("pop")
	}
	ctx := context.Background()

	_, err := n.loadLockInfoV1(ctx, "query-context", lockID)
	require.Regexp(t, "pop", err)
}

func newValidV1LockTransition(t *testing.T, transitionType lockTransitionType, mods ...func(sender *identityPair, in *types.NotoLockInfo_V1, out *types.NotoLockInfo_V1)) (*lockTransition, error) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       &prototk.StateSchema{Id: "coin"},
		lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfoV1"},
	}
	ctx := context.Background()
	lockID := pldtypes.RandBytes32()
	owner := pldtypes.RandAddress()
	inputStateID := pldtypes.RandBytes32()

	inputLockInfo := &types.NotoLockInfo_V1{
		Salt:    pldtypes.RandBytes32(),
		LockID:  lockID,
		Owner:   owner,
		Spender: owner,
	}
	outputLockInfo := &types.NotoLockInfo_V1{
		Salt:     pldtypes.RandBytes32(),
		LockID:   lockID,
		Owner:    owner,
		Spender:  owner,
		Replaces: inputStateID,
	}
	sender := &identityPair{
		address:    owner,
		identifier: "user1",
	}
	for _, mod := range mods {
		mod(sender, inputLockInfo, outputLockInfo)
	}

	return n.validateV1LockTransition(ctx,
		transitionType,
		sender,
		&lockID,
		[]*prototk.EndorsableState{
			{
				SchemaId:      "lockInfoV1",
				Id:            inputStateID.String(),
				StateDataJson: string(pldtypes.JSONString(inputLockInfo)),
			},
		},
		[]*prototk.EndorsableState{
			{
				SchemaId:      "lockInfoV1",
				Id:            pldtypes.RandBytes32().String(),
				StateDataJson: string(pldtypes.JSONString(outputLockInfo)),
			},
		},
	)
}

func TestDecodeV1LockTransitionOKNoChange(t *testing.T) {
	_, _ = newValidV1LockTransition(t, LOCK_UPDATE)
}

func TestDecodeV1LockTransitionOKSpenderChange(t *testing.T) {
	lt, err := newValidV1LockTransition(t, LOCK_UPDATE, func(sender *identityPair, in, out *types.NotoLockInfo_V1) {
		out.Spender = pldtypes.RandAddress()
	})
	require.NoError(t, err)
	require.NotEqual(t, lt.newLockInfo.Spender, lt.newLockInfo.Owner)
}

func TestDecodeV1LockTransitionInvalidInputLock(t *testing.T) {
	n := &Noto{lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfoV1"}}
	_, err := n.validateV1LockTransition(context.Background(),
		LOCK_SPEND,
		&identityPair{address: pldtypes.RandAddress(), identifier: "user1"},
		nil,
		[]*prototk.EndorsableState{
			{
				SchemaId:      "lockInfoV1",
				Id:            pldtypes.RandBytes32().String(),
				StateDataJson: string(pldtypes.JSONString(`{! wrong`)),
			},
		},
		[]*prototk.EndorsableState{},
	)
	require.Regexp(t, "PD200040", err)
}

func TestDecodeV1LockTransitionInvalidOutputLock(t *testing.T) {
	n := &Noto{lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfoV1"}}
	_, err := n.validateV1LockTransition(context.Background(),
		LOCK_CREATE,
		&identityPair{address: pldtypes.RandAddress(), identifier: "user1"},
		nil,
		[]*prototk.EndorsableState{},
		[]*prototk.EndorsableState{
			{
				SchemaId:      "lockInfoV1",
				Id:            pldtypes.RandBytes32().String(),
				StateDataJson: string(pldtypes.JSONString(`{! wrong`)),
			},
		},
	)
	require.Regexp(t, "PD200040", err)
}

func TestMissingLockCreate(t *testing.T) {
	n := &Noto{lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfoV1"}}
	_, err := n.validateV1LockTransition(context.Background(),
		LOCK_CREATE,
		&identityPair{address: pldtypes.RandAddress(), identifier: "user1"},
		nil,
		[]*prototk.EndorsableState{},
		[]*prototk.EndorsableState{},
	)
	require.Regexp(t, "PD200042", err)
}

func TestMissingLockSpend(t *testing.T) {
	n := &Noto{lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfoV1"}}
	_, err := n.validateV1LockTransition(context.Background(),
		LOCK_SPEND,
		&identityPair{address: pldtypes.RandAddress(), identifier: "user1"},
		nil,
		[]*prototk.EndorsableState{},
		[]*prototk.EndorsableState{},
	)
	require.Regexp(t, "PD200042", err)
}

func TestDecodeV1LockTransitionMissingSpendTxId(t *testing.T) {
	_, err := newValidV1LockTransition(t, LOCK_UPDATE, func(sender *identityPair, in, out *types.NotoLockInfo_V1) {
		out.SpendOutputs = []pldtypes.Bytes32{pldtypes.RandBytes32()}
	})
	require.Regexp(t, "PD200040", err)
}

func TestDecodeV1LockTransitionBadLockID(t *testing.T) {
	_, err := newValidV1LockTransition(t, LOCK_UPDATE, func(sender *identityPair, in, out *types.NotoLockInfo_V1) {
		out.LockID = pldtypes.RandBytes32()
	})
	require.Regexp(t, "PD200039", err)
}

func TestDecodeV1LockTransitionBadChain(t *testing.T) {
	_, err := newValidV1LockTransition(t, LOCK_UPDATE, func(sender *identityPair, in, out *types.NotoLockInfo_V1) {
		out.Replaces = pldtypes.RandBytes32()
	})
	require.Regexp(t, "PD200042", err)
}

func TestDecodeV1LockTransitionSplitOutputsOk(t *testing.T) {
	outputCoin := &prototk.EndorsableState{
		Id:            pldtypes.RandBytes32().String(),
		SchemaId:      "coin",
		StateDataJson: `{}`,
	}
	cancelCoin := &prototk.EndorsableState{
		Id:            pldtypes.RandBytes32().String(),
		SchemaId:      "coin",
		StateDataJson: `{}`,
	}
	spendData := pldtypes.HexBytes(pldtypes.RandHex(64))

	lt, err := newValidV1LockTransition(t, LOCK_UPDATE, func(sender *identityPair, in, out *types.NotoLockInfo_V1) {
		out.SpendTxId = pldtypes.RandBytes32()
		out.SpendOutputs = []pldtypes.Bytes32{pldtypes.MustParseBytes32(outputCoin.Id)}
		out.CancelOutputs = []pldtypes.Bytes32{pldtypes.MustParseBytes32(cancelCoin.Id)}
		out.CancelData = spendData
	})
	require.NoError(t, err)

	ctx := context.Background()
	outputCoins, cancelCoins, err := lt.splitOutputs(ctx, []*prototk.EndorsableState{outputCoin, cancelCoin})
	require.NoError(t, err)
	require.Equal(t, []*prototk.EndorsableState{outputCoin}, outputCoins)
	require.Equal(t, []*prototk.EndorsableState{cancelCoin}, cancelCoins)
}

func TestDecodeV1LockTransitionSplitOutputsMissing(t *testing.T) {
	outputCoin := &prototk.EndorsableState{
		Id:            pldtypes.RandBytes32().String(),
		SchemaId:      "coin",
		StateDataJson: `{}`,
	}
	cancelCoin := &prototk.EndorsableState{
		Id:            pldtypes.RandBytes32().String(),
		SchemaId:      "coin",
		StateDataJson: `{}`,
	}
	spendData := pldtypes.HexBytes(pldtypes.RandHex(64))

	lt, err := newValidV1LockTransition(t, LOCK_UPDATE, func(sender *identityPair, in, out *types.NotoLockInfo_V1) {
		out.SpendTxId = pldtypes.RandBytes32()
		out.SpendOutputs = []pldtypes.Bytes32{pldtypes.MustParseBytes32(outputCoin.Id)}
		out.CancelOutputs = []pldtypes.Bytes32{pldtypes.MustParseBytes32(cancelCoin.Id)}
		out.CancelData = spendData
	})
	require.NoError(t, err)

	_, _, err = lt.splitOutputs(context.Background(), []*prototk.EndorsableState{cancelCoin})
	require.Regexp(t, "PD200042", err)
}
