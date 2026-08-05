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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatortransactionmocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
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

// On the golden (partial) path only Pending transactions are sent: transactions the coordinator
// already holds — acknowledged (Delegated) or assembled and beyond — get neither a DelegatedEvent
// nor a protobuf entry. The most recent Delegated transaction is named as the request's
// last_delegated_transaction_id instead of being resent.
func Test_sendDelegationRequest_Partial_SendsPendingOnlyWithLastDelegated(t *testing.T) {
	ctx := context.Background()
	assembledTxn, assembledID := newExcludedMockTxn(t, transaction.State_Assembling)
	delegatedTxn, delegatedID := newExcludedMockTxn(t, transaction.State_Delegated)
	pendingTxn, pendingID := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(assembledTxn, delegatedTxn, pendingTxn). // Order here reflects when these txs were created
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	err := sendDelegationRequest(ctx, o, false)
	require.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(pendingID), "Pending txn must be included")
	assert.False(t, mocks.SentMessageRecorder.HasDelegatedTransaction(delegatedID), "Delegated txn must not be resent on the partial path")
	assert.False(t, mocks.SentMessageRecorder.HasDelegatedTransaction(assembledID), "Assembling txn must be excluded on the partial path")

	requests := mocks.SentMessageRecorder.SentDelegationRequests()
	require.Len(t, requests, 1)
	assert.Equal(t, delegatedID.String(), requests[0].LastDelegatedTransactionId, "the most recent Delegated txn must be named as the predecessor")
	assert.NotEmpty(t, requests[0].DelegationId, "every request must carry a delegation ID for its acknowledgement")
}

// A partial send with no Delegated predecessor (all earlier transactions have moved past
// pre-assembly) leaves last_delegated_transaction_id empty.
func Test_sendDelegationRequest_Partial_NoDelegatedPredecessor_EmptyLastDelegated(t *testing.T) {
	ctx := context.Background()
	assembledTxn, _ := newExcludedMockTxn(t, transaction.State_Assembling)
	pendingTxn, pendingID := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(assembledTxn, pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	require.NoError(t, sendDelegationRequest(ctx, o, false))

	requests := mocks.SentMessageRecorder.SentDelegationRequests()
	require.Len(t, requests, 1)
	assert.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(pendingID))
	assert.Empty(t, requests[0].LastDelegatedTransactionId)
}

// The full (recovery) path re-delegates everything in the resolved prefix, including already-assembled
// transactions, because the coordinator may be missing state. A full request carries the complete
// order itself, so it names no predecessor.
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

	requests := mocks.SentMessageRecorder.SentDelegationRequests()
	require.Len(t, requests, 1)
	assert.Empty(t, requests[0].LastDelegatedTransactionId, "a full request must not name a predecessor")
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

	err := action_SendDelegation(ctx, o, &DelegateSendBatchEvent{})
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

// A delegation response with every transaction acknowledged moves each one Pending -> Delegated
// (via a DelegationAcknowledgedEvent) and cancels the request's in-flight entry, and no follow-up
// delegation is raised.
func Test_action_HandleDelegationAcknowledged_AllAcked(t *testing.T) {
	ctx := context.Background()
	txn1, txnID1 := newAckExpectingMockTxn(t)
	txn2, txnID2 := newAckExpectingMockTxn(t)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(txn1, txn2).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	o.partialInFlight = &inFlightDelegation{delegationID: "d1"}

	err := action_HandleDelegationAcknowledged(ctx, o, &DelegationRequestAcknowledgedEvent{
		FromNode:       "coordinator@node1",
		DelegationID:   "d1",
		TransactionIDs: []string{txnID1.String(), txnID2.String()},
		Results:        []engineProto.DelegationAcknowledgementResult{engineProto.DelegationAcknowledgementResult_DELEGATION_ACCEPTED, engineProto.DelegationAcknowledgementResult_DELEGATION_ACCEPTED},
	})
	require.NoError(t, err)

	assert.Nil(t, o.partialInFlight, "the response must cancel the request's in-flight entry")
	assert.Nil(t, o.fullInFlight)
	assert.Len(t, o.notifyPartialDelegation, 0)
	assert.Len(t, o.notifyFullDelegation, 0)
}

// The coordinator applies prefix-acceptance, so error entries are the un-acknowledged remainder:
// the acknowledged prefix moves to Delegated and a partial delegation is raised to re-send exactly
// the transactions still Pending.
func Test_action_HandleDelegationAcknowledged_PrefixAck_RaisesPartialDelegation(t *testing.T) {
	ctx := context.Background()
	ackedTxn, ackedID := newAckExpectingMockTxn(t)
	rejectedTxn, rejectedID := newInertMockTxn(t)
	skippedTxn, skippedID := newInertMockTxn(t)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(ackedTxn, rejectedTxn, skippedTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	o.partialInFlight = &inFlightDelegation{delegationID: "d1"}

	err := action_HandleDelegationAcknowledged(ctx, o, &DelegationRequestAcknowledgedEvent{
		FromNode:       "coordinator@node1",
		DelegationID:   "d1",
		TransactionIDs: []string{ackedID.String(), rejectedID.String(), skippedID.String()},
		Results: []engineProto.DelegationAcknowledgementResult{
			engineProto.DelegationAcknowledgementResult_DELEGATION_ACCEPTED,
			engineProto.DelegationAcknowledgementResult_MAX_INFLIGHT_TRANSACTIONS,
			engineProto.DelegationAcknowledgementResult_PREVIOUS_TRANSACTION_ERROR,
		},
	})
	require.NoError(t, err)

	assert.Len(t, o.notifyPartialDelegation, 1, "the un-acknowledged remainder must trigger a partial delegation")
	assert.Len(t, o.notifyFullDelegation, 0)
	assert.Nil(t, o.partialInFlight, "a response with per-transaction errors still proves the request landed, so the in-flight entry is cancelled")
	assert.Nil(t, o.fullInFlight)
}

// When the coordinator does not recognise the request's last delegated predecessor it cannot
// guarantee FIFO ordering, so the originator falls back to a full delegation.
func Test_action_HandleDelegationAcknowledged_UnknownLastDelegated_RaisesFullDelegation(t *testing.T) {
	ctx := context.Background()
	txn1, txnID1 := newInertMockTxn(t)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(txn1).
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	err := action_HandleDelegationAcknowledged(ctx, o, &DelegationRequestAcknowledgedEvent{
		FromNode:       "coordinator@node1",
		DelegationID:   "d1",
		TransactionIDs: []string{txnID1.String()},
		Results:        []engineProto.DelegationAcknowledgementResult{engineProto.DelegationAcknowledgementResult_UNKNOWN_LAST_DELEGATED_TRANSACTION},
	})
	require.NoError(t, err)

	assert.Len(t, o.notifyFullDelegation, 1, "an unknown predecessor must trigger a full delegation")
	assert.Len(t, o.notifyPartialDelegation, 0)
}

// The acknowledged delegation ID can match the full slot rather than the partial one; the response
// must clear whichever slot holds it.
func Test_action_HandleDelegationAcknowledged_FullSlotAck_CancelsFullInFlight(t *testing.T) {
	ctx := context.Background()
	txn1, txnID1 := newAckExpectingMockTxn(t)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(txn1).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	o.fullInFlight = &inFlightDelegation{delegationID: "d1"}

	err := action_HandleDelegationAcknowledged(ctx, o, &DelegationRequestAcknowledgedEvent{
		FromNode:       "coordinator@node1",
		DelegationID:   "d1",
		TransactionIDs: []string{txnID1.String()},
		Results:        []engineProto.DelegationAcknowledgementResult{engineProto.DelegationAcknowledgementResult_DELEGATION_ACCEPTED},
	})
	require.NoError(t, err)

	assert.Nil(t, o.fullInFlight, "the response must cancel the full in-flight entry it acknowledges")
	assert.Nil(t, o.partialInFlight)
}

// A response carrying fewer acknowledgement results than transactions is malformed: the transaction
// at the short index and everything after it are treated as un-acknowledged and re-delegated.
func Test_action_HandleDelegationAcknowledged_FewerResultsThanTransactions_RaisesPartial(t *testing.T) {
	ctx := context.Background()
	txn1, txnID1 := newInertMockTxn(t)
	txn2, txnID2 := newInertMockTxn(t)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(txn1, txn2).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	o.partialInFlight = &inFlightDelegation{delegationID: "d1"}

	err := action_HandleDelegationAcknowledged(ctx, o, &DelegationRequestAcknowledgedEvent{
		FromNode:       "coordinator@node1",
		DelegationID:   "d1",
		TransactionIDs: []string{txnID1.String(), txnID2.String()},
		Results:        []engineProto.DelegationAcknowledgementResult{},
	})
	require.NoError(t, err)

	assert.Len(t, o.notifyPartialDelegation, 1, "a short results list must re-delegate the un-acknowledged transactions")
	assert.Len(t, o.notifyFullDelegation, 0)
}

// An acknowledgement carrying a malformed transaction ID is skipped without failing the whole batch.
func Test_action_HandleDelegationAcknowledged_InvalidTransactionID_Skips(t *testing.T) {
	ctx := context.Background()

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	err := action_HandleDelegationAcknowledged(ctx, o, &DelegationRequestAcknowledgedEvent{
		FromNode:       "coordinator@node1",
		DelegationID:   "d1",
		TransactionIDs: []string{"not-a-uuid"},
		Results:        []engineProto.DelegationAcknowledgementResult{engineProto.DelegationAcknowledgementResult_DELEGATION_ACCEPTED},
	})
	require.NoError(t, err)

	assert.Len(t, o.notifyPartialDelegation, 0)
	assert.Len(t, o.notifyFullDelegation, 0)
}

// An acknowledgement for a transaction that has already completed and been cleaned up is skipped:
// there is no live transaction left to move to Delegated.
func Test_action_HandleDelegationAcknowledged_UnknownTransaction_Skips(t *testing.T) {
	ctx := context.Background()
	goneID := uuid.New()

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	err := action_HandleDelegationAcknowledged(ctx, o, &DelegationRequestAcknowledgedEvent{
		FromNode:       "coordinator@node1",
		DelegationID:   "d1",
		TransactionIDs: []string{goneID.String()},
		Results:        []engineProto.DelegationAcknowledgementResult{engineProto.DelegationAcknowledgementResult_DELEGATION_ACCEPTED},
	})
	require.NoError(t, err)

	assert.Len(t, o.notifyPartialDelegation, 0)
	assert.Len(t, o.notifyFullDelegation, 0)
}

// A failure moving an acknowledged transaction Pending → Delegated surfaces as an error from the action.
func Test_action_HandleDelegationAcknowledged_TransactionHandleEventError_ReturnsError(t *testing.T) {
	ctx := context.Background()
	txn1, txnID1 := newAckErroringMockTxn(t)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(txn1).
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	err := action_HandleDelegationAcknowledged(ctx, o, &DelegationRequestAcknowledgedEvent{
		FromNode:       "coordinator@node1",
		DelegationID:   "d1",
		TransactionIDs: []string{txnID1.String()},
		Results:        []engineProto.DelegationAcknowledgementResult{engineProto.DelegationAcknowledgementResult_DELEGATION_ACCEPTED},
	})
	require.Error(t, err)
}

// An acknowledgement from a node that is not the current active coordinator (e.g. one we have since
// failed away from) is dropped by the state machine: no transaction events, no follow-up delegation,
// and the in-flight entry for the current coordinator is untouched.
func Test_stateMachine_Sending_DelegationAckFromStaleCoordinator_Dropped(t *testing.T) {
	ctx := context.Background()
	txn1, txnID1 := newInertMockTxn(t)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(txn1).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	o.partialInFlight = &inFlightDelegation{delegationID: "d1"}

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &DelegationRequestAcknowledgedEvent{
		FromNode:       "stale-coordinator@node2",
		DelegationID:   "d1",
		TransactionIDs: []string{txnID1.String()},
		Results:        []engineProto.DelegationAcknowledgementResult{engineProto.DelegationAcknowledgementResult_DELEGATION_ACCEPTED},
	}))

	assert.NotNil(t, o.partialInFlight, "a stale acknowledgement must not cancel the in-flight entry")
	assert.Len(t, o.notifyPartialDelegation, 0)
	assert.Len(t, o.notifyFullDelegation, 0)
}

// A partial send is recorded in the partial in-flight slot under the sent delegation ID, so its timeout
// can re-delegate the transactions if the acknowledgement never arrives.
func Test_sendDelegationRequest_RecordsInFlightDelegation(t *testing.T) {
	ctx := context.Background()
	pendingTxn, _ := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	require.NoError(t, sendDelegationRequest(ctx, o, false))

	requests := mocks.SentMessageRecorder.SentDelegationRequests()
	require.Len(t, requests, 1)
	require.NotNil(t, o.partialInFlight, "the partial send must be recorded in the partial slot")
	assert.Nil(t, o.fullInFlight, "a partial send must not record a full")
	assert.Equal(t, requests[0].DelegationId, o.partialInFlight.delegationID, "the in-flight entry must carry the sent delegation ID")
}

// A send rebuilds from live state, so it supersedes every request already in flight: the prior entry is
// cancelled and a single fresh one recorded. This is what a request timeout relies on — the timer pokes
// the notify channel, and the resulting send re-delegates the still-Pending transactions under a fresh
// delegation ID, with the predecessor computed at send time.
func Test_action_SendDelegation_SupersedesInFlightUnderFreshDelegationID(t *testing.T) {
	ctx := context.Background()
	delegatedTxn, delegatedID := newExcludedMockTxn(t, transaction.State_Delegated)
	pendingTxn, pendingID := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(delegatedTxn, pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(100))
	o.partialInFlight = &inFlightDelegation{delegationID: "d1"}

	require.NoError(t, action_SendDelegation(ctx, o, &DelegateSendBatchEvent{}))

	requests := mocks.SentMessageRecorder.SentDelegationRequests()
	require.Len(t, requests, 1, "the still-outstanding transactions must be re-delegated")
	assert.NotEqual(t, "d1", requests[0].DelegationId, "the re-delegation must use a fresh delegation ID")
	require.Len(t, requests[0].Transactions, 1)
	assert.Equal(t, pendingID.String(), requests[0].Transactions[0].Id, "the still-Pending transaction must be re-delegated")
	assert.Equal(t, delegatedID.String(), requests[0].LastDelegatedTransactionId, "the predecessor is computed from live state at send time")

	require.NotNil(t, o.partialInFlight, "a fresh in-flight entry replaces the superseded one")
	assert.NotEqual(t, "d1", o.partialInFlight.delegationID, "the superseded entry must be dropped")
}

// A partial send supersedes only a prior partial: an in-flight full delegation (a recovery send that
// re-pushes transactions a partial would not resend) keeps its slot and timer, so its own timeout still
// fires independently.
func Test_action_SendDelegation_Partial_PreservesInFlightFull(t *testing.T) {
	ctx := context.Background()
	pendingTxn, _ := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(100))
	full := &inFlightDelegation{delegationID: "full-1"}
	o.fullInFlight = full

	require.NoError(t, action_SendDelegation(ctx, o, &DelegateSendBatchEvent{}))

	assert.Same(t, full, o.fullInFlight, "a partial send must not disturb an in-flight full delegation")
	require.NotNil(t, o.partialInFlight, "the partial send is recorded in the partial slot")
}

// A full send supersedes everything in flight (it re-delegates a superset): both slots collapse to the
// single new full request.
func Test_action_SendDelegation_Full_SupersedesBoth(t *testing.T) {
	ctx := context.Background()
	pendingTxn, _ := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(100))
	o.fullInFlight = &inFlightDelegation{delegationID: "full-old"}
	o.partialInFlight = &inFlightDelegation{delegationID: "partial-old"}

	require.NoError(t, action_SendDelegation(ctx, o, &DelegateSendBatchEvent{Full: true}))

	require.NotNil(t, o.fullInFlight, "the full send is recorded in the full slot")
	assert.NotEqual(t, "full-old", o.fullInFlight.delegationID, "the prior full is superseded")
	assert.Nil(t, o.partialInFlight, "a full send supersedes any in-flight partial")
}

// Switching the active coordinator discards every in-flight delegation request: the full delegation
// to the new coordinator supersedes them all.
func Test_action_SwitchActiveCoordinator_CancelsInFlightDelegations(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	o.partialInFlight = &inFlightDelegation{delegationID: "d1"}
	o.fullInFlight = &inFlightDelegation{delegationID: "d2"}

	err := action_SwitchActiveCoordinator(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode: "new-coordinator@node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	})
	require.NoError(t, err)

	assert.Nil(t, o.partialInFlight)
	assert.Nil(t, o.fullInFlight)
}

// The request timer pokes the notify channel it was armed with when it fires: the partial channel for a
// partial delegation's timer, the full channel for a full one, so the batching loop re-delegates at the
// same scope.
func Test_armInFlightDelegationTimer_PokesNotifyChannelForScope(t *testing.T) {
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	o.delegationLoopCtx = context.Background()
	o.requestTimeout = time.Millisecond

	o.startInFlightDelegationTimer(&inFlightDelegation{delegationID: "d1"}, o.notifyPartialDelegation)
	<-o.notifyPartialDelegation
	assert.Len(t, o.notifyFullDelegation, 0, "a partial request's timeout must not poke the full channel")

	o.startInFlightDelegationTimer(&inFlightDelegation{delegationID: "d2"}, o.notifyFullDelegation)
	<-o.notifyFullDelegation
}

// End to end through the batching goroutine: a request whose acknowledgement never arrives is
// re-delegated under a fresh delegation ID once its request timeout fires.
func Test_delegationLoop_Timeout_ReDelegatesRequest(t *testing.T) {
	pendingTxn, _ := newDelegatableMockTxn(t, transaction.State_Pending)

	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(pendingTxn).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(100)).Maybe()
	o.ctx = context.Background()
	o.requestTimeout = time.Millisecond

	// startDelegationLoop replaces the builder's channels and runs the real goroutine; entry to
	// Sending raises nothing here, so raise the partial flag to drive the first send.
	o.startDelegationLoop()
	defer o.stopDelegationLoop()
	o.notifyPartialDelegation <- struct{}{}

	require.Eventually(t, func() bool {
		if err := o.stateMachineEventLoop.DrainPendingEvents(context.Background()); err != nil {
			return false
		}
		return len(mocks.SentMessageRecorder.SentDelegationRequests()) >= 2
	}, 5*time.Second, time.Millisecond, "the un-acknowledged request must be re-delegated")

	requests := mocks.SentMessageRecorder.SentDelegationRequests()
	assert.NotEqual(t, requests[0].DelegationId, requests[1].DelegationId, "the re-delegation must use a fresh delegation ID")
}

// When a request's timeout fires but a wake is already queued on the notify channel, the redundant
// wake is dropped rather than blocking the timer goroutine.
func Test_startInFlightDelegationTimer_ChannelFull_CoalescesTimeout(t *testing.T) {
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	clk := &captureClock{Clock: common.RealClock()}
	o.clock = clk
	o.delegationLoopCtx = context.Background()
	o.requestTimeout = time.Millisecond

	// Fill the single-slot buffer so a wake is already pending when the timeout fires.
	o.notifyPartialDelegation <- struct{}{}

	o.startInFlightDelegationTimer(&inFlightDelegation{delegationID: "d1"}, o.notifyPartialDelegation)
	require.NotNil(t, clk.scheduled, "the timer must have been scheduled")

	clk.scheduled()

	assert.Len(t, o.notifyPartialDelegation, 1, "the redundant timeout wake must be coalesced, not queued")
}

// captureClock records the function passed to ScheduleTimer so a test can fire the timeout
// synchronously, deferring every other clock method to the embedded real clock.
type captureClock struct {
	common.Clock
	scheduled func()
}

func (c *captureClock) ScheduleTimer(_ context.Context, _ time.Duration, f func()) func() {
	c.scheduled = f
	return func() {}
}

// newInertMockTxn builds a mock transaction that must receive no events at all; read-only
// accessors are permitted but not required.
func newInertMockTxn(t *testing.T) (*originatortransactionmocks.OriginatorTransaction, uuid.UUID) {
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID).Maybe()
	mockTxn.On("GetCurrentState").Return(transaction.State_Pending).Maybe()
	return mockTxn, txID
}

// newAckExpectingMockTxn builds a mock Pending transaction that must receive exactly one
// DelegationAcknowledgedEvent.
func newAckExpectingMockTxn(t *testing.T) (*originatortransactionmocks.OriginatorTransaction, uuid.UUID) {
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID).Maybe()
	mockTxn.On("GetCurrentState").Return(transaction.State_Pending).Maybe()
	mockTxn.On("HandleEvent", mock.Anything, mock.AnythingOfType("*transaction.DelegationAcknowledgedEvent")).Return(nil).Once()
	return mockTxn, txID
}

// newAckErroringMockTxn builds a mock Pending transaction whose DelegationAcknowledgedEvent handling
// fails, so the action's error path can be exercised.
func newAckErroringMockTxn(t *testing.T) (*originatortransactionmocks.OriginatorTransaction, uuid.UUID) {
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID).Maybe()
	mockTxn.On("GetCurrentState").Return(transaction.State_Pending).Maybe()
	mockTxn.On("HandleEvent", mock.Anything, mock.AnythingOfType("*transaction.DelegationAcknowledgedEvent")).Return(fmt.Errorf("pop")).Once()
	return mockTxn, txID
}
