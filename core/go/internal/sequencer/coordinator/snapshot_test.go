/*
 * Copyright © 2025 Kaleido, Inc.
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

package coordinator

import (
	"context"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/mocks/coordinatortransactionmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/graphermocks"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetSnapshot_OK(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()
	snapshot := c.getSnapshot(ctx)
	assert.NotNil(t, snapshot)
}

func TestGetSnapshot_AggregatesTransactionsBySnapshotType(t *testing.T) {
	ctx := context.Background()
	pooledTxnID, dispatchedTxnID, confirmedTxnID, revertedTxnID, excludedTxnID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	pooledSnapshot := &common.SnapshotPooledTransaction{
		ID: pooledTxnID,
	}
	dispatchedSnapshot := &common.SnapshotDispatchedTransaction{}
	dispatchedSnapshot.ID = dispatchedTxnID
	confirmedSnapshot := &common.SnapshotConfirmedTransaction{}
	confirmedSnapshot.ID = confirmedTxnID
	revertedSnapshot := &common.SnapshotRevertedTransaction{}
	revertedSnapshot.ID = revertedTxnID

	pooledTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	pooledTxn.EXPECT().GetID().Return(pooledTxnID)
	pooledTxn.EXPECT().GetSnapshot(mock.Anything).Return(pooledSnapshot, nil, nil, nil)
	dispatchedTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	dispatchedTxn.EXPECT().GetID().Return(dispatchedTxnID)
	dispatchedTxn.EXPECT().GetSnapshot(mock.Anything).Return(nil, dispatchedSnapshot, nil, nil)
	confirmedTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	confirmedTxn.EXPECT().GetID().Return(confirmedTxnID)
	confirmedTxn.EXPECT().GetSnapshot(mock.Anything).Return(nil, nil, confirmedSnapshot, nil)
	revertedTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	revertedTxn.EXPECT().GetID().Return(revertedTxnID)
	revertedTxn.EXPECT().GetSnapshot(mock.Anything).Return(nil, nil, nil, revertedSnapshot)
	excludedTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	excludedTxn.EXPECT().GetID().Return(excludedTxnID)
	excludedTxn.EXPECT().GetSnapshot(mock.Anything).Return(nil, nil, nil, nil)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(pooledTxn, dispatchedTxn, confirmedTxn, revertedTxn, excludedTxn).
		Build()

	snapshot := c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Len(t, snapshot.PooledTransactions, 1)
	assert.Len(t, snapshot.DispatchedTransactions, 1)
	assert.Len(t, snapshot.ConfirmedTransactions, 1)
	assert.Len(t, snapshot.RevertedTransactions, 1)
	assert.Equal(t, pooledTxnID, snapshot.PooledTransactions[0].ID)
	assert.Equal(t, dispatchedTxnID, snapshot.DispatchedTransactions[0].ID)
	assert.Equal(t, confirmedTxnID, snapshot.ConfirmedTransactions[0].ID)
	assert.Equal(t, revertedTxnID, snapshot.RevertedTransactions[0].ID)
}

func TestGetSnapshot_IncludesCoordinatorStateAndBlockHeight(t *testing.T) {
	ctx := context.Background()
	blockHeight := uint64(12345)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).CurrentBlockHeight(blockHeight).Build()

	snapshot := c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, c.GetCurrentState(), snapshot.CoordinatorState)
	assert.Equal(t, blockHeight, snapshot.BlockHeight)
}

func TestSendHeartbeat_Success(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	err := c.sendHeartbeat(ctx, false)
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func TestSendHeartbeat_IncludesCurrentNode(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	err := c.sendHeartbeat(ctx, false)
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func TestSendHeartbeat_HandlesError(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		WithMockTransportWriter().
		Build()
	mocks.TransportWriter.EXPECT().SendHeartbeat(mock.Anything, "node1", mock.Anything, mock.Anything).
		Return(nil)
	mocks.TransportWriter.EXPECT().SendHeartbeat(mock.Anything, "node2", mock.Anything, mock.Anything).
		Return(fmt.Errorf("transport error"))

	err := c.sendHeartbeat(ctx, false)
	// Should return the error but continue processing
	assert.Error(t, err)
	assert.Equal(t, "transport error", err.Error())
}

func TestAction_SendHeartbeat(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	err := action_SendHeartbeat(ctx, c, nil)
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func TestAction_SendHeartbeatWithLocks(t *testing.T) {
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	mockGrapher.EXPECT().ExportStatesAndLocks(mock.Anything, "node1").
		Return(grapher.ExportableStates{}, nil)
	mockGrapher.EXPECT().ExportStatesAndLocks(mock.Anything, "node2").
		Return(grapher.ExportableStates{}, nil)

	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Grapher(mockGrapher).
		Build()

	err := action_SendHeartbeatWithLocks(ctx, c, nil)
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func Test_action_IncrementHeartbeatIntervalsSinceStateChange(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).HeartbeatIntervalsSinceStateChange(2).Build()

	err := action_IncrementHeartbeatIntervalsSinceStateChange(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, c.heartbeatIntervalsSinceStateChange)
}

func Test_action_PropagateHeartbeatToTransactions_NoTransactions(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()

	err := action_PropagateHeartbeatIntervalToTransactions(ctx, c, nil)
	require.NoError(t, err)
}

func Test_action_PropagateHeartbeatToTransactions_WithTransactions(t *testing.T) {
	ctx := context.Background()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.EXPECT().GetID().Return(uuid.New())
	txn.EXPECT().HandleEvent(mock.Anything, mock.AnythingOfType("*common.HeartbeatIntervalEvent")).Return(nil)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txn).Build()

	err := action_PropagateHeartbeatIntervalToTransactions(ctx, c, nil)
	require.NoError(t, err)
}

func TestSendHeartbeat_StaticMode_WithOriginatorActivity_SendsHeartbeats(t *testing.T) {
	ctx := context.Background()
	// STATIC mode (default) with a non-empty originatorActivity exercises the
	// "nodes = append(nodes, node)" branch inside the else-block of sendHeartbeat.
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorActivity(map[string]int{"remoteNode": 0}).
		Build()

	err := c.sendHeartbeat(ctx, false)
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func TestSendHeartbeat_ExportStatesAndLocksError_ReturnsError(t *testing.T) {
	ctx := context.Background()

	mockGrapher := graphermocks.NewGrapher(t)
	mockGrapher.EXPECT().ExportStatesAndLocks(mock.Anything, "node1").
		Return(grapher.ExportableStates{}, fmt.Errorf("export error"))

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Grapher(mockGrapher).
		Build()

	// includeLocks=true causes ExportStatesAndLocks to be called.
	err := c.sendHeartbeat(ctx, true)
	assert.Error(t, err)
	assert.Equal(t, "export error", err.Error())
}
