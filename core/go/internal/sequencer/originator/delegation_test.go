// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package originator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatortransactionmocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_HandleDelegationRejected_HigherPriorityCoordinator_Redirects(t *testing.T) {
	// The rejection names a coordinator that has higher priority (lower index) than the current one.
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	err := action_HandleDelegationRejected(ctx, o, &DelegationRequestRejectedEvent{
		ActiveCoordinator: "node1",
	})
	require.NoError(t, err)

	assert.Equal(t, "node1", o.currentActiveCoordinator, "coordinator must be redirected to the higher-priority node")
}

func Test_action_HandleDelegationRejected_LowerPriorityCoordinator_NoChange(t *testing.T) {
	// The rejection names a coordinator with lower priority than the current one; we ignore it.
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	err := action_HandleDelegationRejected(ctx, o, &DelegationRequestRejectedEvent{
		ActiveCoordinator: "node3",
	})
	require.NoError(t, err)

	assert.Equal(t, "node1", o.currentActiveCoordinator, "coordinator must not change when named node has lower priority")
}

func Test_action_HandleDelegationRejected_NoActiveCoordinator_NoChange(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		Build()

	err := action_HandleDelegationRejected(ctx, o, &DelegationRequestRejectedEvent{
		ActiveCoordinator: "",
	})
	require.NoError(t, err)

	assert.Equal(t, "node1", o.currentActiveCoordinator)
}

func Test_sendDelegationRequest_HandleEventError_ReturnsWrappedError(t *testing.T) {
	ctx := context.Background()
	txnID := uuid.New()
	pt := &components.PrivateTransaction{ID: txnID}
	expectedErr := fmt.Errorf("delegated event handling failed")
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetCurrentState").Return(transaction.State_Pending)
	mockTxn.On("GetPrivateTransaction").Return(pt)
	mockTxn.On("GetID").Return(txnID)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(expectedErr)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn).
		CurrentActiveCoordinator("coordinator@coordinatorNode").
		Build()
	err := sendDelegationRequest(ctx, o, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error handling delegated event for transaction")
	assert.Contains(t, err.Error(), txnID.String())
	assert.Contains(t, err.Error(), expectedErr.Error())
}

// On the golden (partial) path, transactions that the coordinator already knows about (Assembling or
// beyond) are skipped: they get neither a DelegatedEvent nor a protobuf entry. Only Pending/Delegated
// transactions are (re)sent.
func Test_sendDelegationRequest_Partial_ExcludesAssembledTransactions(t *testing.T) {
	ctx := context.Background()
	assembledTxn, assembledID := newExcludedMockTxn(t, transaction.State_Assembling)
	delegatedTxn, delegatedID := newDelegatableMockTxn(t, transaction.State_Delegated)
	pendingTxn, pendingID := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(assembledTxn, delegatedTxn, pendingTxn). // Order here reflects when these txs were created
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	err := sendDelegationRequest(ctx, o, false)
	require.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(delegatedID), "Delegated txn must be included")
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(pendingID), "Pending txn must be included")
	assert.False(t, mocks.SentMessageRecorder.HasDelegatedTransaction(assembledID), "Assembling txn must be excluded on the partial path")
}

// The full (recovery) path re-delegates everything in the resolved prefix, including already-assembled
// transactions, because the coordinator may be missing state.
func Test_sendDelegationRequest_Full_IncludesAssembledTransactions(t *testing.T) {
	ctx := context.Background()
	assembledTxn, assembledID := newDelegatableMockTxn(t, transaction.State_Assembling)
	delegatedTxn, delegatedID := newDelegatableMockTxn(t, transaction.State_Delegated)
	pendingTxn, pendingID := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(assembledTxn, delegatedTxn, pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	err := sendDelegationRequest(ctx, o, true)
	require.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(assembledID), "full resend must include the assembled txn")
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(delegatedID))
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(pendingID))
}

// A partial delegation with nothing left to send (every resolved transaction is already assembled)
// must not emit a delegation request at all.
func Test_sendDelegationRequest_Partial_AllAssembled_DoesNotSend(t *testing.T) {
	ctx := context.Background()
	assembled1, _ := newExcludedMockTxn(t, transaction.State_Assembling)
	assembled2, _ := newExcludedMockTxn(t, transaction.State_Dispatched)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(assembled1, assembled2).
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	err := sendDelegationRequest(ctx, o, false)
	require.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "partial resend with nothing to delegate must not send a request")
}

// action_NotifyPartialDelegation is the golden-path action: it raises the partial dirty flag and must NOT
// send anything synchronously (the batching goroutine sends later).
func Test_action_NotifyPartialDelegation_RaisesPartialFlagOnly(t *testing.T) {
	ctx := context.Background()
	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	o.notifyFullDelegation = make(chan struct{}, 1)
	o.notifyPartialDelegation = make(chan struct{}, 1)

	err := action_NotifyPartialDelegation(ctx, o, nil)
	require.NoError(t, err)

	assert.Len(t, o.notifyPartialDelegation, 1)
	assert.Len(t, o.notifyFullDelegation, 0)
	assert.False(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "notifcation must not send synchronously")
}

// action_NotifyFullDelegation is the recovery-path action: it raises the full dirty flag and must NOT
// send synchronously.
func Test_action_NotifyFullDelegation_RaisesFullFlagOnly(t *testing.T) {
	ctx := context.Background()
	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	o.notifyFullDelegation = make(chan struct{}, 1)
	o.notifyPartialDelegation = make(chan struct{}, 1)

	err := action_NotifyFullDelegation(ctx, o, nil)
	require.NoError(t, err)

	assert.Len(t, o.notifyFullDelegation, 1)
	assert.Len(t, o.notifyPartialDelegation, 0)
	assert.False(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "notification must not send synchronously")
}

// The notify actions use a non-blocking send on a length-1 channel, so repeated notifications
// coalesce to a single pending flag.
func Test_action_NotifyPartialDelegation_Coalesces(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).Build()

	require.NoError(t, action_NotifyPartialDelegation(ctx, o, nil))
	require.NoError(t, action_NotifyPartialDelegation(ctx, o, nil))
	require.NoError(t, action_NotifyPartialDelegation(ctx, o, nil))

	assert.Len(t, o.notifyPartialDelegation, 1, "repeated partial notifications must coalesce")
}

func Test_action_NotifyFullDelegation_Coalesces(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).Build()

	require.NoError(t, action_NotifyFullDelegation(ctx, o, nil))
	require.NoError(t, action_NotifyFullDelegation(ctx, o, nil))
	require.NoError(t, action_NotifyFullDelegation(ctx, o, nil))

	assert.Len(t, o.notifyFullDelegation, 1, "repeated full notifications must coalesce")
}

// The notify actions are safe no-ops when the channels are nil (outside State_Sending / loop not
// running): a send on a nil channel is never ready, so the select falls through to default.
func Test_action_NotifyDelegation_NilChannelsIsNoOp(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).Build()
	require.Nil(t, o.notifyFullDelegation)
	require.Nil(t, o.notifyPartialDelegation)
	assert.NotPanics(t, func() {
		require.NoError(t, action_NotifyFullDelegation(ctx, o, nil))
		require.NoError(t, action_NotifyPartialDelegation(ctx, o, nil))
	})
}

// startDelegationLoop / stopDelegationLoop can be cycled repeatedly, are individually idempotent, and
// fully tear down their per-run state on stop.
func Test_delegationLoop_StartStopLifecycle(t *testing.T) {
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).Build()
	o.ctx = context.Background()

	o.startDelegationLoop()
	require.NotNil(t, o.notifyFullDelegation)
	require.NotNil(t, o.notifyPartialDelegation)
	require.NotNil(t, o.delegationLoopCancel)
	require.NotNil(t, o.delegationLoopDone)

	// A second start while running is a no-op: the channels are not replaced.
	full, partial := o.notifyFullDelegation, o.notifyPartialDelegation
	o.startDelegationLoop()
	assert.True(t, full == o.notifyFullDelegation, "second start must not replace the full channel")
	assert.True(t, partial == o.notifyPartialDelegation, "second start must not replace the partial channel")

	// Stop tears everything down and is idempotent.
	o.stopDelegationLoop()
	assert.Nil(t, o.notifyFullDelegation)
	assert.Nil(t, o.notifyPartialDelegation)
	assert.Nil(t, o.delegationLoopCancel)
	assert.Nil(t, o.delegationLoopDone)
	assert.NotPanics(t, o.stopDelegationLoop)

	// The loop can be started again after stopping (Sending is re-entered over the originator's life).
	o.startDelegationLoop()
	require.NotNil(t, o.delegationLoopCancel)
	o.stopDelegationLoop()
	assert.Nil(t, o.delegationLoopCancel)
}

// startDelegationLoop is a no-op before the originator has started (o.ctx not yet set).
func Test_startDelegationLoop_NoOpBeforeStart(t *testing.T) {
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).Build()
	o.ctx = nil
	o.startDelegationLoop()
	assert.Nil(t, o.delegationLoopCancel, "loop must not start before the originator context is set")
	assert.Nil(t, o.notifyFullDelegation)
}

// startDelegationLoopForTesting builds a State_Sending originator with one assembled and one pending
// transaction and runs the real delegationLoop goroutine against it with a short tick interval.
// Whether the assembled transaction is (re)delegated distinguishes a full send from a partial one.
// Returns a cancel func that stops the loop and waits for it to exit (also registered as a t.Cleanup).
func startDelegationLoopForTesting(t *testing.T, delegatable bool) (o *originator, mocks *OriginatorDependencyMocks, assembledID, pendingID uuid.UUID, stop func()) {
	var assembledTxn *originatortransactionmocks.OriginatorTransaction
	if delegatable {
		assembledTxn, assembledID = newDelegatableMockTxn(t, transaction.State_Assembling)
	} else {
		assembledTxn, assembledID = newExcludedMockTxn(t, transaction.State_Assembling)
	}
	pendingTxn, pendingID := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks = NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(assembledTxn, pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(100)).Maybe()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		o.delegationLoop(ctx, time.Millisecond, o.notifyFullDelegation, o.notifyPartialDelegation)
	}()
	stop = func() {
		cancel()
		<-done
	}
	t.Cleanup(stop)
	return o, mocks, assembledID, pendingID, stop
}

// waitForDelegationRequest pumps the internally-queued DelegateSendBatchEvents through the event loop
// until the recorder observes a delegation request (or times out).
func waitForDelegationRequest(t *testing.T, o *originator, mocks *OriginatorDependencyMocks) {
	require.Eventually(t, func() bool {
		if err := o.stateMachineEventLoop.DrainPendingEvents(context.Background()); err != nil {
			return false
		}
		return mocks.SentMessageRecorder.HasSentDelegationRequest()
	}, 5*time.Second, time.Millisecond, "the batching loop must send a delegation request")
}

// A tick with only the partial notification sends a partial batch (Full=false): the assembled
// transaction is excluded.
func Test_delegationLoop_PartialNotification_SendsPartialBatch(t *testing.T) {
	o, mocks, assembledID, pendingID, stop := startDelegationLoopForTesting(t, false)

	o.notifyPartialDelegation <- struct{}{}
	waitForDelegationRequest(t, o, mocks)
	stop()

	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(pendingID), "pending txn must be included")
	assert.False(t, mocks.SentMessageRecorder.HasDelegatedTransaction(assembledID), "a partial send must exclude the assembled txn")
}

// A tick with only the full notification sends a full batch (Full=true): the assembled transaction
// is (re)delegated too.
func Test_delegationLoop_FullNotification_SendsFullBatch(t *testing.T) {
	o, mocks, assembledID, pendingID, stop := startDelegationLoopForTesting(t, true)

	o.notifyFullDelegation <- struct{}{}
	waitForDelegationRequest(t, o, mocks)
	stop()

	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(pendingID))
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(assembledID), "a full send must include the assembled txn")
}

// When both channels are notified in the same batch window, full wins: a single full send is emitted and
// both channels are drained.
func Test_delegationLoop_BothNotifications_FullWins(t *testing.T) {
	o, mocks, assembledID, pendingID, stop := startDelegationLoopForTesting(t, true)

	o.notifyPartialDelegation <- struct{}{}
	o.notifyFullDelegation <- struct{}{}
	waitForDelegationRequest(t, o, mocks)
	stop()

	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(pendingID))
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(assembledID), "full must win when both channels were notified")
	assert.Len(t, o.notifyPartialDelegation, 0, "the stale partial flag must have been drained by the same tick")
	assert.Len(t, o.notifyFullDelegation, 0)
}

// Ticks during which channel has been notified must not queue any DelegateSendBatchEvent. No
// GetBlockHeight expectation is set, so if an empty tick spuriously queued a send event, draining
// it below would fail the strict EngineIntegration mock in addition to the recorder assertion.
func Test_delegationLoop_NoSignal_NoEvent(t *testing.T) {
	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		o.delegationLoop(ctx, time.Millisecond, o.notifyFullDelegation, o.notifyPartialDelegation)
	}()

	// Let several empty ticks elapse, then stop the loop and drain anything it queued.
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
	require.NoError(t, o.stateMachineEventLoop.DrainPendingEvents(context.Background()))

	assert.False(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "empty ticks must not send a delegation request")
}

// action_SendDelegation performs the coalesced send. A partial send excludes assembled transactions.
func Test_action_SendDelegation_Partial_ExcludesAssembled(t *testing.T) {
	ctx := context.Background()
	assembledTxn, assembledID := newExcludedMockTxn(t, transaction.State_Assembling)
	pendingTxn, pendingID := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(assembledTxn, pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(100))

	err := action_SendDelegation(ctx, o, &DelegateSendBatchEvent{Full: false})
	require.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(pendingID))
	assert.False(t, mocks.SentMessageRecorder.HasDelegatedTransaction(assembledID))
}

// A full send re-delegates the whole backlog including assembled transactions.
func Test_action_SendDelegation_Full_IncludesAssembled(t *testing.T) {
	ctx := context.Background()
	assembledTxn, assembledID := newDelegatableMockTxn(t, transaction.State_Assembling)
	pendingTxn, pendingID := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(assembledTxn, pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(100))

	err := action_SendDelegation(ctx, o, &DelegateSendBatchEvent{Full: true})
	require.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(assembledID), "full send must include the assembled txn")
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(pendingID))
}

func Test_sendDelegationRequest_TransportError_ReturnsError(t *testing.T) {
	ctx := t.Context()
	builder := NewOriginatorBuilderForTesting(t, State_Sending).WithMockTransportWriter(t)
	txn := testutil.NewPrivateTransactionBuilderForTesting().Build()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetCurrentState").Return(transaction.State_Pending)
	mockTxn.On("GetID").Return(txn.ID)
	mockTxn.On("GetPrivateTransaction").Return(txn)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	o, mocks := builder.Transactions(mockTxn).CurrentActiveCoordinator("coordinator@node1").Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, mock.Anything, mock.Anything).
		Return(fmt.Errorf("transport error"))

	err := sendDelegationRequest(ctx, o, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transport error")
}
