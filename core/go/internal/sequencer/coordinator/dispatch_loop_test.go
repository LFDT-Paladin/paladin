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
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0), nil)
	require.NoError(t, c.Start(ctx))

	c.startDispatchLoop()
	require.NotNil(t, c.dispatchLoopDone, "dispatch loop must be running after startDispatchLoop")

	c.stopDispatchLoop()

	require.Nil(t, c.dispatchLoopDone, "dispatch loop must have stopped after stopDispatchLoop")
	assert.Nil(t, c.dispatchLoopCancel, "dispatchLoopCancel must be nilled by stopDispatchLoop")
	assert.Nil(t, c.dispatchLoopDone, "dispatchLoopDone must be nilled by stopDispatchLoop")
}

// TestDispatchLoop_StopWhileWaitingForInFlightSlot covers the path where the dispatch loop
// is blocked in the first Wait() (too many in flight) and exits when Stop() sends to
// stopDispatchLoop and Signals. We pre-populate inFlightTxns so that when the loop
// pulls the queued tx it sees len(inFlightTxns)+dispatchedAhead >= maxDispatchAhead and enters Wait().
func TestDispatchLoop_StopWhileWaitingForInFlightSlot(t *testing.T) {
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txnID := uuid.New()
	txn.EXPECT().GetID().Return(txnID)

	builder := NewCoordinatorBuilderForTesting(t, State_Active).Transactions(txn)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(1)
	builder.OverrideSequencerConfig(config)

	c, mocks := builder.Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0), nil).Maybe()

	ctx, cancel := context.WithCancel(t.Context())
	require.NoError(t, c.Start(ctx))
	defer cancel()

	c.startDispatchLoop()

	// Pre-populate inFlightTxns so the dispatch loop will enter the first Wait() when it pulls the tx
	c.inFlightMutex.L.Lock()
	c.inFlightTxns[uuid.New()] = coordinatortransactionmocks.NewCoordinatorTransaction(t)
	c.inFlightMutex.L.Unlock()

	// Queue one tx: transition to Ready_For_Dispatch so it gets sent to dispatchQueue
	err := action_QueueTransactionForDispatch(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txnID,
		ToState:       transaction.State_Ready_For_Dispatch,
	})
	require.NoError(t, err)

	// Give the dispatch loop time to pull the tx and enter the first Wait() (too many in flight).
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
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0), nil).Maybe()
	require.NoError(t, c.Start(ctx))
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
	tx2.EXPECT().HasDispatchedPublicTransaction().Return(false)

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

// TestDispatchLoop_TxnWithoutPublicDispatch_DoesNotCountAhead verifies that when a dispatched
// transaction has HasDispatchedPublicTransaction()==false, dispatchedAhead is not incremented.
func TestDispatchLoop_TxnWithoutPublicDispatch_DoesNotCountAhead(t *testing.T) {
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
	tx.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		_, ok := e.(*transaction.DispatchedEvent)
		return ok
	})).Return(nil)
	// HasDispatchedPublicTransaction returns false — dispatchedAhead stays 0
	tx.EXPECT().HasDispatchedPublicTransaction().Return(false)

	c.dispatchQueue <- tx

	done := make(chan struct{})
	c.dispatchLoopDone = done
	go func() {
		defer close(done)
		c.dispatchLoop(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-c.dispatchLoopDone:
	case <-time.After(time.Second):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

// TestDispatchLoop_CtxCancelledDuringSecondWait_Exits verifies that when the loop enters the
// second wait (waiting for the state machine to confirm the tx is in-flight) and the context
// is cancelled, the loop exits cleanly.
func TestDispatchLoop_CtxCancelledDuringSecondWait_Exits(t *testing.T) {
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(1) // exactly 1 slot — second wait fires after first dispatch
	builder.OverrideSequencerConfig(config)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c, _ := builder.Build()

	// Register the AfterFunc so context cancellation wakes the loop from its Wait()
	context.AfterFunc(ctx, func() {
		c.inFlightMutex.L.Lock()
		c.inFlightMutex.Broadcast()
		c.inFlightMutex.L.Unlock()
	})

	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id := uuid.New()
	tx.EXPECT().GetID().Return(id).Maybe()
	tx.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		_, ok := e.(*transaction.DispatchedEvent)
		return ok
	})).Return(nil)
	// HasDispatchedPublicTransaction returns true — dispatchedAhead becomes 1, hitting maxDispatchAhead
	tx.EXPECT().HasDispatchedPublicTransaction().Return(true)

	c.dispatchQueue <- tx

	done := make(chan struct{})
	c.dispatchLoopDone = done
	go func() {
		defer close(done)
		c.dispatchLoop(ctx)
	}()

	// Give the loop time to enter the second wait
	time.Sleep(50 * time.Millisecond)
	cancel() // cancelling context triggers AfterFunc → Broadcast → loop exits second wait via ctx.Done()

	select {
	case <-c.dispatchLoopDone:
	case <-time.After(time.Second):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

// TestAction_NudgeDispatchLoop_TxnWithoutPublicDispatch_NotCountedAsInFlight verifies that
// a State_Dispatched transaction where HasDispatchedPublicTransaction()==false is not added
// to inFlightTxns.
func TestAction_NudgeDispatchLoop_TxnWithoutPublicDispatch_NotCountedAsInFlight(t *testing.T) {
	ctx := context.Background()

	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id := uuid.New()
	tx.EXPECT().GetID().Return(id)
	tx.EXPECT().GetCurrentState().Return(transaction.State_Dispatched)
	tx.EXPECT().HasDispatchedPublicTransaction().Return(false)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).Transactions(tx).Build()

	err := action_NudgeDispatchLoop(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(c.inFlightTxns), "transaction without public dispatch should not be counted as in-flight")
}

// TestAction_NudgeDispatchLoop_TxnWithPublicDispatch_CountedAsInFlight verifies that
// a State_Dispatched transaction where HasDispatchedPublicTransaction()==true IS added
// to inFlightTxns (covers the true-branch body on line 101).
func TestAction_NudgeDispatchLoop_TxnWithPublicDispatch_CountedAsInFlight(t *testing.T) {
	ctx := context.Background()

	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	id := uuid.New()
	tx.EXPECT().GetID().Return(id)
	tx.EXPECT().GetCurrentState().Return(transaction.State_Dispatched)
	tx.EXPECT().HasDispatchedPublicTransaction().Return(true)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).Transactions(tx).Build()

	err := action_NudgeDispatchLoop(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, len(c.inFlightTxns), "transaction with public dispatch should be counted as in-flight")
	assert.Equal(t, tx, c.inFlightTxns[id])
}

// TestDispatchLoop_SecondWait_NormalExit covers the path where the loop enters the second wait
// (dispatchedAhead hits maxDispatchAhead), then the state machine signals inFlightTxns is updated
// while ctx is still active (taking the default: case), exits the for loop, and resets dispatchedAhead.
func TestDispatchLoop_SecondWait_NormalExit(t *testing.T) {
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
	tx.EXPECT().HandleEvent(ctx, mock.MatchedBy(func(e common.Event) bool {
		_, ok := e.(*transaction.DispatchedEvent)
		return ok
	})).Return(nil)
	tx.EXPECT().HasDispatchedPublicTransaction().Return(true)

	c.dispatchQueue <- tx

	done := make(chan struct{})
	c.dispatchLoopDone = done
	go func() {
		defer close(done)
		c.dispatchLoop(ctx)
	}()

	// Wait for the loop to dispatch the tx and enter the second wait
	time.Sleep(50 * time.Millisecond)

	// Simulate the state machine confirming the tx is in-flight, then signal
	c.inFlightMutex.L.Lock()
	c.inFlightTxns[id] = tx
	c.inFlightMutex.Signal()
	c.inFlightMutex.L.Unlock()

	// Give the loop time to exit the second wait (default: branch) and reset dispatchedAhead
	time.Sleep(50 * time.Millisecond)

	cancel()
	select {
	case <-c.dispatchLoopDone:
	case <-time.After(time.Second):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}
