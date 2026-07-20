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
 * specific language governing permissions and limitations under this License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package coordinator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/mocks/coordinatortransactionmocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Test_stopDispatchLoop_StopsRunningLoop verifies the full body of stopDispatchLoop: it cancels the
// loop's context, signals the inFlightMutex to unblock any Wait(), waits for the goroutine to exit,
// and then nils both dispatchLoopCancel and dispatchLoopDone.
func Test_stopDispatchLoop_StopsRunningLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	c.Start(ctx)

	c.startDispatchLoop()
	require.NotNil(t, c.dispatchLoopDone, "dispatch loop must be running after startDispatchLoop")

	c.stopDispatchLoop()

	require.Nil(t, c.dispatchLoopDone, "dispatch loop must have stopped after stopDispatchLoop")
	assert.Nil(t, c.dispatchLoopCancel, "dispatchLoopCancel must be nilled by stopDispatchLoop")
	assert.Nil(t, c.dispatchLoopDone, "dispatchLoopDone must be nilled by stopDispatchLoop")
}

// TestDispatchLoop_StopWhileWaitingForInFlightSlot covers the path where the dispatch loop
// is blocked in the Wait() (too many in flight) and exits when stopDispatchLoop cancels and Signals.
// We pre-populate inFlightTxns so that when the loop pulls the queued tx it sees
// len(inFlightTxns) >= maxDispatchAhead and enters Wait().
func TestDispatchLoop_StopWhileWaitingForInFlightSlot(t *testing.T) {
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txnID := uuid.New()
	txn.EXPECT().GetID().Return(txnID)

	builder := NewCoordinatorBuilderForTesting(t, State_Active).Transactions(txn)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(1)
	builder.OverrideSequencerConfig(config)

	c, mocks := builder.Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))

	ctx, cancel := context.WithCancel(t.Context())
	c.Start(ctx)
	defer cancel()

	c.startDispatchLoop()

	// Pre-populate inFlightTxns so the dispatch loop will enter the Wait() when it pulls the tx
	c.inFlightMutex.L.Lock()
	c.inFlightTxns[uuid.New()] = struct{}{}
	c.inFlightMutex.L.Unlock()

	// Queue one tx: transition to Ready_For_Dispatch so it gets sent to dispatchQueue
	err := action_QueueTransactionForDispatch(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txnID,
		ToState:       transaction.State_Ready_For_Dispatch,
	})
	require.NoError(t, err)

	// Give the dispatch loop time to pull the tx and enter the Wait() (too many in flight).
	time.Sleep(50 * time.Millisecond)
}

// TestDispatchLoop_StopAtSelect covers the path where the dispatch loop is in the top-level
// select and exits when the context is cancelled.
func TestDispatchLoop_StopAtSelect(t *testing.T) {
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1) // no dispatch progress, so loop stays in select
	builder.OverrideSequencerConfig(config)

	ctx, cancel := context.WithCancel(t.Context())
	c, mocks := builder.Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	c.Start(ctx)
	c.startDispatchLoop()
	cancel()
	// Stop without ever queueing a tx; loop is blocked on the select waiting for dispatchQueue or ctx.Done()
}

// TestDispatchLoop_HandleEventError_ContinuesLoop verifies that when HandleEvent returns an error
// for a dispatched transaction, the loop logs the error and continues processing subsequent transactions.
func TestDispatchLoop_HandleEventError_ContinuesLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	// Register the same AfterFunc that Start() would register, so ctx cancellation wakes the loop
	context.AfterFunc(ctx, func() {
		c.inFlightMutex.L.Lock()
		c.inFlightMutex.Broadcast()
		c.inFlightMutex.L.Unlock()
	})

	// tx1: HandleEvent returns an error — the loop should log and continue
	tx1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id1 := uuid.New()
	tx1.EXPECT().GetID().Return(id1).Maybe()
	tx1.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		if de, ok := e.(*transaction.DispatchedEvent); ok {
			return de.TransactionID == id1
		}
		return false
	})).Return(fmt.Errorf("dispatch error"))

	// tx2: HandleEvent returns nil, and detaches no batch (repooled before its event) — verifies the loop
	// continued after tx1's error.
	tx2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id2 := uuid.New()
	tx2.EXPECT().GetID().Return(id2).Maybe()
	tx2.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		if de, ok := e.(*transaction.DispatchedEvent); ok {
			return de.TransactionID == id2
		}
		return false
	})).Return(nil)
	tx2.EXPECT().PendingDispatch(mock.Anything).Return(nil)

	// Pre-queue both transactions before starting the loop (buffered channel)
	c.dispatchQueue <- tx1
	c.dispatchQueue <- tx2

	done := make(chan struct{})
	c.dispatchLoopDone = done
	go func() {
		defer close(done)
		c.dispatchLoop(ctx)
	}()

	// Give the loop time to process both transactions
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-c.dispatchLoopDone:
	case <-time.After(time.Second):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

// TestDispatchLoop_SkipsRepooledTransaction_ContinuesLoop verifies that when a transaction detaches no
// prepared batch (it was repooled before its DispatchedEvent was processed, so prepare never ran), the
// loop skips it and continues processing subsequent transactions.
func TestDispatchLoop_SkipsRepooledTransaction_ContinuesLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	context.AfterFunc(ctx, func() {
		c.inFlightMutex.L.Lock()
		c.inFlightMutex.Broadcast()
		c.inFlightMutex.L.Unlock()
	})

	// tx1: HandleEvent succeeds but detaches no batch (repooled) — the loop should skip and continue
	tx1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id1 := uuid.New()
	tx1.EXPECT().GetID().Return(id1).Maybe()
	tx1.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		de, ok := e.(*transaction.DispatchedEvent)
		return ok && de.TransactionID == id1
	})).Return(nil)
	tx1.EXPECT().PendingDispatch(mock.Anything).Return(nil)

	// tx2: also detaches no batch — verifies the loop continued after skipping tx1
	tx2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id2 := uuid.New()
	tx2.EXPECT().GetID().Return(id2).Maybe()
	tx2.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		de, ok := e.(*transaction.DispatchedEvent)
		return ok && de.TransactionID == id2
	})).Return(nil)
	dispatched := make(chan struct{}, 1)
	tx2.EXPECT().PendingDispatch(mock.Anything).Run(func(context.Context) { dispatched <- struct{}{} }).Return(nil)

	c.dispatchQueue <- tx1
	c.dispatchQueue <- tx2

	done := make(chan struct{})
	c.dispatchLoopDone = done
	go func() {
		defer close(done)
		c.dispatchLoop(ctx)
	}()

	select {
	case <-dispatched:
	case <-time.After(time.Second):
		t.Fatal("second transaction was not dispatched after persist error")
	}

	cancel()
	select {
	case <-c.dispatchLoopDone:
	case <-time.After(time.Second):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

// TestDispatchLoop_DispatchesQueuedTransaction verifies the happy path: a queued transaction is
// pulled and handed to the state machine via a DispatchedEvent, and the loop continues.
func TestDispatchLoop_DispatchesQueuedTransaction(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	context.AfterFunc(ctx, func() {
		c.inFlightMutex.L.Lock()
		c.inFlightMutex.Broadcast()
		c.inFlightMutex.L.Unlock()
	})

	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id := uuid.New()
	tx.EXPECT().GetID().Return(id).Maybe()
	dispatched := make(chan struct{}, 1)
	tx.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		de, ok := e.(*transaction.DispatchedEvent)
		return ok && de.TransactionID == id
	})).Run(func(context.Context, common.Event) { dispatched <- struct{}{} }).Return(nil)
	tx.EXPECT().PendingDispatch(mock.Anything).Return(nil)

	c.dispatchQueue <- tx

	done := make(chan struct{})
	c.dispatchLoopDone = done
	go func() {
		defer close(done)
		c.dispatchLoop(ctx)
	}()

	select {
	case <-dispatched:
	case <-time.After(time.Second):
		t.Fatal("transaction was not dispatched")
	}

	cancel()
	select {
	case <-c.dispatchLoopDone:
	case <-time.After(time.Second):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

// TestDispatchLoop_WaitsAtCapacityThenProceeds verifies the loop blocks while len(inFlightTxns)
// has reached maxDispatchAhead and resumes once a slot is freed via setDispatchedInFlight.
func TestDispatchLoop_WaitsAtCapacityThenProceeds(t *testing.T) {
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(1)
	builder.OverrideSequencerConfig(config)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c, _ := builder.Build()

	context.AfterFunc(ctx, func() {
		c.inFlightMutex.L.Lock()
		c.inFlightMutex.Broadcast()
		c.inFlightMutex.L.Unlock()
	})

	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id := uuid.New()
	tx.EXPECT().GetID().Return(id).Maybe()
	dispatched := make(chan struct{}, 1)
	tx.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		_, ok := e.(*transaction.DispatchedEvent)
		return ok
	})).Run(func(context.Context, common.Event) { dispatched <- struct{}{} }).Return(nil)
	tx.EXPECT().PendingDispatch(mock.Anything).Return(nil).Maybe()

	// Fill the single dispatch-ahead slot so the loop must wait before dispatching tx.
	occupyingID := uuid.New()
	c.setDispatchedInFlight(occupyingID, true)

	c.dispatchQueue <- tx

	done := make(chan struct{})
	c.dispatchLoopDone = done
	go func() {
		defer close(done)
		c.dispatchLoop(ctx)
	}()

	// The loop should be blocked at capacity and must not dispatch yet.
	select {
	case <-dispatched:
		t.Fatal("transaction dispatched while at max dispatch ahead")
	case <-time.After(50 * time.Millisecond):
	}

	// Free the slot; the loop wakes and dispatches.
	c.setDispatchedInFlight(occupyingID, false)

	select {
	case <-dispatched:
	case <-time.After(time.Second):
		t.Fatal("transaction was not dispatched after slot freed")
	}

	cancel()
	select {
	case <-c.dispatchLoopDone:
	case <-time.After(time.Second):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

// TestDispatchLoop_PullCapRespectsInFlight verifies that the pull cap is maxDispatchAhead minus the
// number of in-flight transactions, so a batch never pulls more than the dispatch-ahead limit allows.
func TestDispatchLoop_PullCapRespectsInFlight(t *testing.T) {
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(5)
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build()

	// Two public transactions already in flight leave capacity for three more.
	c.setDispatchedInFlight(uuid.New(), true)
	c.setDispatchedInFlight(uuid.New(), true)

	capacity := c.awaitDispatchAheadCapacity(t.Context())
	require.Equal(t, 3, capacity)

	// Queue five transactions; the pull must cap the batch at the capacity and leave the rest queued.
	for i := 0; i < 5; i++ {
		tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
		tx.EXPECT().GetID().Return(uuid.New()).Maybe()
		c.dispatchQueue <- tx
	}
	first := <-c.dispatchQueue
	batch := c.pullDispatchBatch(first, capacity)
	assert.Len(t, batch, 3, "batch must be capped at the dispatch-ahead capacity")
	assert.Len(t, c.dispatchQueue, 2, "transactions beyond the cap must remain queued")
}

// TestSetDispatchedInFlight_AddAndRemove verifies the in-flight set is maintained by ID and that
// removal signals the dispatch loop, and is idempotent for unknown IDs.
func TestSetDispatchedInFlight_AddAndRemove(t *testing.T) {
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	id := uuid.New()
	c.setDispatchedInFlight(id, true)
	assert.Equal(t, 1, len(c.inFlightTxns))
	_, ok := c.inFlightTxns[id]
	assert.True(t, ok)

	c.setDispatchedInFlight(id, false)
	assert.Equal(t, 0, len(c.inFlightTxns))

	// Removing an unknown ID is a no-op.
	c.setDispatchedInFlight(uuid.New(), false)
	assert.Equal(t, 0, len(c.inFlightTxns))
}

// TestDispatchLoop_CapsBatchSize verifies the dispatch loop never pulls more than maxDispatchBatchSize
// transactions per batch, splitting a larger queue into multiple batches while preserving pull order.
func TestDispatchLoop_CapsBatchSize(t *testing.T) {
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(50)
	config.MaxDispatchBatchSize = confutil.P(2)
	builder.OverrideSequencerConfig(config)

	c, mocks := builder.Build()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	context.AfterFunc(ctx, func() {
		c.inFlightMutex.L.Lock()
		c.inFlightMutex.Broadcast()
		c.inFlightMutex.L.Unlock()
	})

	// Three transactions, each with its own plain (no chained children) pending dispatch.
	var wantOrder []uuid.UUID
	for i := 0; i < 3; i++ {
		pd := &syncpoints.PendingDispatch{Dispatch: &syncpoints.TransactionDispatch{}}
		tx := newDispatchTxMock(t, pd)
		// newDispatchTxMock stamps pd.TransactionID with the mock's own id.
		wantOrder = append(wantOrder, pd.TransactionID)
		c.dispatchQueue <- tx
	}

	// Capture each committed batch's transaction ids. The .Run runs on the dispatch-loop goroutine; the
	// buffered channel both hands the ids to the test goroutine and synchronises the two commits.
	committed := make(chan []uuid.UUID, 2)
	mocks.SyncPoints.EXPECT().PersistDispatchBatch(mock.Anything, mock.Anything).Run(func(_ context.Context, batch *syncpoints.DispatchBatch) {
		ids := make([]uuid.UUID, 0, len(batch.Dispatches()))
		for _, pd := range batch.Dispatches() {
			ids = append(ids, pd.TransactionID)
		}
		committed <- ids
	}).Return(nil)

	done := make(chan struct{})
	c.dispatchLoopDone = done
	go func() {
		defer close(done)
		c.dispatchLoop(ctx)
	}()

	var batches [][]uuid.UUID
	for len(batches) < 2 {
		select {
		case ids := <-committed:
			batches = append(batches, ids)
		case <-time.After(time.Second):
			t.Fatal("did not observe two committed batches within timeout")
		}
	}

	require.Len(t, batches, 2, "expected exactly two committed batches")
	assert.Len(t, batches[0], 2, "first batch must be capped at MaxDispatchBatchSize")
	assert.Len(t, batches[1], 1, "second batch holds the remaining transaction")

	var gotOrder []uuid.UUID
	gotOrder = append(gotOrder, batches[0]...)
	gotOrder = append(gotOrder, batches[1]...)
	assert.Equal(t, wantOrder, gotOrder, "pull order must be preserved across the batch split")

	cancel()
	select {
	case <-c.dispatchLoopDone:
	case <-time.After(time.Second):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

// newDispatchTxMock builds a CoordinatorTransaction mock that dispatches successfully and returns the given
// pending dispatch (its TransactionID is set to the mock's ID). Pass pd == nil to model a transaction that
// prepared no dispatch (repooled before its event).
func newDispatchTxMock(t *testing.T, pd *syncpoints.PendingDispatch) transaction.CoordinatorTransaction {
	id := uuid.New()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx.EXPECT().GetID().Return(id).Maybe()
	tx.EXPECT().HandleEvent(mock.Anything, mock.MatchedBy(func(e common.Event) bool {
		de, ok := e.(*transaction.DispatchedEvent)
		return ok && de.TransactionID == id
	})).Return(nil)
	if pd != nil {
		pd.TransactionID = id
	}
	tx.EXPECT().PendingDispatch(mock.Anything).Return(pd)
	return tx
}

func chainedDispatch() *syncpoints.PendingDispatch {
	return &syncpoints.PendingDispatch{
		Dispatch: &syncpoints.TransactionDispatch{
			PrivateDispatches: []*components.ChainedPrivateTransaction{
				{NewTransaction: &components.ValidatedTransaction{}},
			},
		},
	}
}

// TestDispatchBatch_HandsOffChainedChildren verifies that after the batch commits, a dispatch carrying a
// chained private child is handed off via SequencerManager.HandleNewTx.
func TestDispatchBatch_HandsOffChainedChildren(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	mocks.SyncPoints.EXPECT().PersistDispatchBatch(mock.Anything, mock.Anything).Return(nil)
	mocks.SequencerManager.EXPECT().HandleNewTx(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mocks.Persistence.Mock.ExpectBegin()
	mocks.Persistence.Mock.ExpectCommit()

	c.dispatchBatch(ctx, []transaction.CoordinatorTransaction{newDispatchTxMock(t, chainedDispatch())})
}

// TestDispatchBatch_NoChainedChildren_SkipsHandoff verifies that a dispatch with no chained children commits
// but triggers no chained hand-off (SequencerManager.HandleNewTx is not called).
func TestDispatchBatch_NoChainedChildren_SkipsHandoff(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	mocks.SyncPoints.EXPECT().PersistDispatchBatch(mock.Anything, mock.Anything).Return(nil)

	pd := &syncpoints.PendingDispatch{Dispatch: &syncpoints.TransactionDispatch{}}
	c.dispatchBatch(ctx, []transaction.CoordinatorTransaction{newDispatchTxMock(t, pd)})

	mocks.SequencerManager.AssertNotCalled(t, "HandleNewTx")
}

// TestDispatchBatch_PersistError_SkipsChainedHandoff verifies that when the batch commit fails, the loop
// logs and returns without handing off chained children.
func TestDispatchBatch_PersistError_SkipsChainedHandoff(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	mocks.SyncPoints.EXPECT().PersistDispatchBatch(mock.Anything, mock.Anything).Return(errors.New("persist failed"))

	c.dispatchBatch(ctx, []transaction.CoordinatorTransaction{newDispatchTxMock(t, chainedDispatch())})

	mocks.SequencerManager.AssertNotCalled(t, "HandleNewTx")
}

// TestDispatchBatch_ChainedChildError_LogsAndContinues verifies that a failure handing off a chained child
// is logged and does not abort the loop (the batch has already committed atomically).
func TestDispatchBatch_ChainedChildError_LogsAndContinues(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	mocks.SyncPoints.EXPECT().PersistDispatchBatch(mock.Anything, mock.Anything).Return(nil)
	mocks.SequencerManager.EXPECT().HandleNewTx(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("handle new tx failed"))
	mocks.Persistence.Mock.ExpectBegin()
	mocks.Persistence.Mock.ExpectRollback()

	c.dispatchBatch(ctx, []transaction.CoordinatorTransaction{newDispatchTxMock(t, chainedDispatch())})
}

// TestDispatchBatch_AllSkipped_TouchesNoSyncPoints verifies that a batch in which no transaction prepared a
// dispatch is created lazily and never touches syncPoints.
func TestDispatchBatch_AllSkipped_TouchesNoSyncPoints(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	c.dispatchBatch(ctx, []transaction.CoordinatorTransaction{newDispatchTxMock(t, nil), newDispatchTxMock(t, nil)})

	mocks.SyncPoints.AssertNotCalled(t, "PersistDispatchBatch")
}
