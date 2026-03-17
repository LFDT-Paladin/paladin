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
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetSnapshot_OK(t *testing.T) {
	ctx := context.Background()
	c, _, done := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer done()
	snapshot := c.getSnapshot(ctx)
	assert.NotNil(t, snapshot)
}

func TestGetSnapshot_AggregatesTransactionsBySnapshotType(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	c, _, done := NewCoordinatorForUnitTest(t, ctx, []string{originator})
	defer done()

	pooledTxn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	dispatchedTxn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	confirmedTxn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	excludedTxn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Reverted).Build()
	c.transactionsByID[pooledTxn.GetID()] = pooledTxn
	c.transactionsByID[dispatchedTxn.GetID()] = dispatchedTxn
	c.transactionsByID[confirmedTxn.GetID()] = confirmedTxn
	c.transactionsByID[excludedTxn.GetID()] = excludedTxn

	snapshot := c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Len(t, snapshot.PooledTransactions, 1)
	assert.Len(t, snapshot.DispatchedTransactions, 1)
	assert.Len(t, snapshot.ConfirmedTransactions, 1)
	assert.Equal(t, pooledTxn.GetID(), snapshot.PooledTransactions[0].ID)
	assert.Equal(t, dispatchedTxn.GetID(), snapshot.DispatchedTransactions[0].ID)
	assert.Equal(t, confirmedTxn.GetID(), snapshot.ConfirmedTransactions[0].ID)
}

func TestGetSnapshot_IncludesFlushPoints(t *testing.T) {
	ctx := context.Background()
	c, _, done := NewCoordinatorBuilderForTesting(t, State_Prepared).Build(ctx)
	defer done()

	snapshot := c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Greater(t, len(snapshot.FlushPoints), 0)
}

func TestGetSnapshot_IncludesCoordinatorStateAndBlockHeight(t *testing.T) {
	ctx := context.Background()
	blockHeight := uint64(12345)
	c, _, done := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer done()
	// Set block height directly since CurrentBlockHeight only works for certain states
	c.currentBlockHeight = blockHeight

	snapshot := c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, c.GetCurrentState().String(), snapshot.CoordinatorState)
	assert.Equal(t, blockHeight, snapshot.BlockHeight)
}

func TestSendHeartbeat_Success(t *testing.T) {
	ctx := context.Background()
	c, mocks, done := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer done()

	// Set nodeName and originatorNodePool directly
	c.nodeName = "node1"
	c.originatorNodePool = []string{"node1", "node2", "node3"}

	err := c.sendHeartbeat(ctx, c.contractAddress)
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func TestSendHeartbeat_SkipsCurrentNode(t *testing.T) {
	ctx := context.Background()
	c, mocks, done := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer done()

	// Set nodeName and originatorNodePool directly
	c.nodeName = "node1"
	c.originatorNodePool = []string{"node1"}

	err := c.sendHeartbeat(ctx, c.contractAddress)
	assert.NoError(t, err)
	// Should not send heartbeat since only node1 is in pool and it's the current node
	assert.False(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func TestSendHeartbeat_HandlesError(t *testing.T) {
	ctx := context.Background()
	c, _, done := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer done()

	// Set nodeName and originatorNodePool directly
	c.nodeName = "node1"
	c.originatorNodePool = []string{"node1", "node2"}

	// Create a mock transport writer that returns an error
	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.EXPECT().SendHeartbeat(mock.Anything, "node2", mock.Anything, mock.Anything).
		Return(fmt.Errorf("transport error"))
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	mockTransport.On("WaitForDone", mock.Anything).Return().Maybe()
	c.transportWriter = mockTransport

	err := c.sendHeartbeat(ctx, c.contractAddress)
	// Should return the error but continue processing
	assert.Error(t, err)
	assert.Equal(t, "transport error", err.Error())
	mockTransport.AssertExpectations(t)
}

func TestAction_SendHeartbeat(t *testing.T) {
	ctx := context.Background()
	c, mocks, done := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer done()

	// Set nodeName and originatorNodePool directly
	c.nodeName = "node1"
	c.originatorNodePool = []string{"node1", "node2"}

	err := action_SendHeartbeat(ctx, c, nil)
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func Test_heartbeatLoop_StartsAndSendsInitialHeartbeat(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, mocks, done := builder.Build(ctx)
	defer done()
	c.updateOriginatorNodePool("node2")
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.GetID()] = txn
	require.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil initially")

	hbDone := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(hbDone)
	}()

	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentHeartbeat()
	}, 100*time.Millisecond, 5*time.Millisecond)
	c.heartbeatCancel()
	<-hbDone
	assert.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil after loop ends")
	assert.Nil(t, c.heartbeatCancel, "heartbeatCancel should be nil after loop ends")
}

func Test_heartbeatLoop_SendsPeriodicHeartbeats(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, mocks, done := builder.Build(ctx)
	defer done()
	c.heartbeatInterval = 10 * time.Millisecond
	c.updateOriginatorNodePool("node2")
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.GetID()] = txn

	hbDone := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(hbDone)
	}()

	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.SentHeartbeatCount() >= 2
	}, 500*time.Millisecond, 10*time.Millisecond)
	c.heartbeatCancel()
	<-hbDone
}

func Test_heartbeatLoop_ExitsWhenHeartbeatCtxIsCancelled(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.GetID()] = txn

	hbDone := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(hbDone)
	}()

	require.Eventually(t, func() bool {
		return c.heartbeatCtx != nil
	}, 50*time.Millisecond, 1*time.Millisecond, "heartbeatCancel should be set")
	c.heartbeatCancel()
	select {
	case <-hbDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("heartbeat loop should exit when heartbeatCtx is cancelled")
	}
	assert.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil after loop ends")
	assert.Nil(t, c.heartbeatCancel, "heartbeatCancel should be nil after loop ends")
}

func Test_heartbeatLoop_ExitsWhenParentCtxIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.GetID()] = txn

	hbDone := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(hbDone)
	}()

	assert.Eventually(t, func() bool {
		return c.heartbeatCtx != nil
	}, 50*time.Millisecond, 1*time.Millisecond, "heartbeatCtx should be set")
	cancel()
	select {
	case <-hbDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("heartbeat loop should exit when parent ctx is cancelled")
	}
	assert.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil after loop ends")
	assert.Nil(t, c.heartbeatCancel, "heartbeatCancel should be nil after loop ends")
}

func Test_heartbeatLoop_DoesNotStartIfHeartbeatCtxAlreadySet(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, mocks, done := builder.Build(ctx)
	defer done()
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	c.heartbeatCtx = heartbeatCtx
	c.heartbeatCancel = heartbeatCancel
	mocks.SentMessageRecorder.Reset(ctx)

	c.heartbeatLoop(ctx)

	assert.False(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "heartbeat should not be sent if loop already running")
	heartbeatCancel()
}

func Test_heartbeatLoop_CreatesNewContextOnStart(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.GetID()] = txn
	require.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil initially")
	require.Nil(t, c.heartbeatCancel, "heartbeatCancel should be nil initially")

	hbDone := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(hbDone)
	}()

	assert.Eventually(t, func() bool {
		return c.heartbeatCtx != nil
	}, 50*time.Millisecond, 1*time.Millisecond, "heartbeatCtx should be created when loop starts")
	assert.NotNil(t, c.heartbeatCancel, "heartbeatCancel should be created when loop starts")
	c.heartbeatCancel()
	<-hbDone
}

func Test_heartbeatLoop_StopsTickerOnExit(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.HeartbeatInterval = confutil.P("50ms")
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	hbDone := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(hbDone)
	}()

	for c.heartbeatCtx == nil {
		time.Sleep(1 * time.Millisecond)
	}
	c.heartbeatCancel()
	<-hbDone
}

func Test_heartbeatLoop_CanBeRestartedAfterCancellation(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	config := builder.GetSequencerConfig()
	config.HeartbeatInterval = confutil.P("100ms")
	builder.OverrideSequencerConfig(config)
	c, mocks, done := builder.Build(ctx)
	defer done()
	c.updateOriginatorNodePool("node2")
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.GetID()] = txn

	done1 := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done1)
	}()
	for c.heartbeatCtx == nil {
		time.Sleep(1 * time.Millisecond)
	}
	c.heartbeatCancel()
	<-done1

	mocks.SentMessageRecorder.Reset(ctx)
	done2 := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done2)
	}()
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentHeartbeat()
	}, 200*time.Millisecond, 10*time.Millisecond, "second heartbeat loop should send heartbeats")
	c.heartbeatCancel()
	<-done2
}
