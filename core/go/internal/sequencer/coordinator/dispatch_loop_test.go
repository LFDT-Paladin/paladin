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
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
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

	// tx2: HandleEvent returns nil, no public transaction — verifies loop continued after error
	tx2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id2 := uuid.New()
	tx2.EXPECT().GetID().Return(id2).Maybe()
	tx2.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		if de, ok := e.(*transaction.DispatchedEvent); ok {
			return de.TransactionID == id2
		}
		return false
	})).Return(nil)
	tx2.EXPECT().PersistDispatch(mock.Anything).Return(nil)

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

// TestDispatchLoop_PersistDispatchError_ContinuesLoop verifies that when PersistDispatch returns an
// error for a dispatched transaction, the loop logs the error and continues processing subsequent
// transactions.
func TestDispatchLoop_PersistDispatchError_ContinuesLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).Build()

	context.AfterFunc(ctx, func() {
		c.inFlightMutex.L.Lock()
		c.inFlightMutex.Broadcast()
		c.inFlightMutex.L.Unlock()
	})

	// tx1: HandleEvent succeeds but PersistDispatch fails — the loop should log and continue
	tx1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id1 := uuid.New()
	tx1.EXPECT().GetID().Return(id1).Maybe()
	tx1.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		de, ok := e.(*transaction.DispatchedEvent)
		return ok && de.TransactionID == id1
	})).Return(nil)
	tx1.EXPECT().PersistDispatch(mock.Anything).Return(fmt.Errorf("persist error"))

	// tx2: dispatched successfully — verifies the loop continued after tx1's persist error
	tx2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id2 := uuid.New()
	tx2.EXPECT().GetID().Return(id2).Maybe()
	tx2.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		de, ok := e.(*transaction.DispatchedEvent)
		return ok && de.TransactionID == id2
	})).Return(nil)
	dispatched := make(chan struct{}, 1)
	tx2.EXPECT().PersistDispatch(mock.Anything).Run(func(context.Context) { dispatched <- struct{}{} }).Return(nil)

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
	tx.EXPECT().PersistDispatch(mock.Anything).Return(nil)

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
	tx.EXPECT().PersistDispatch(mock.Anything).Return(nil)

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
