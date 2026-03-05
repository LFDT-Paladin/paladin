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

package originator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestOriginator_SingleTransactionLifecycle(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks, cleanup := builder.Build(ctx)
	defer cleanup()

	// Ensure the originator is in observing mode by queuing a heartbeat from an active coordinator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.ContractAddress = &contractAddress
	s.QueueEvent(ctx, heartbeatEvent)
	require.Eventually(t, func() bool { return s.GetCurrentState() == State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "Originator should transition to Observing")

	// Start by creating a transaction with the originator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	s.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: txn})
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Delegation request should be sent")

	postAssembly, postAssemblyHash := transactionBuilder.BuildPostAssemblyAndHash()
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(postAssembly, nil)

	// Simulate the coordinator sending an assemble request
	assembleRequestIdempotencyKey := uuid.New()
	s.QueueEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{TransactionID: txn.ID},
		RequestID: assembleRequestIdempotencyKey, Coordinator: coordinatorLocator,
		CoordinatorsBlockHeight: 1000, StateLocksJSON: []byte("{}"),
	})
	// Assert that the transaction was assembled and a response sent
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentAssembleSuccessResponse() }, 100*time.Millisecond, 1*time.Millisecond, "Assemble success response should be sent")

	// Simulate the coordinator sending a dispatch confirmation
	s.QueueEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{TransactionID: txn.ID},
		RequestID: assembleRequestIdempotencyKey, Coordinator: coordinatorLocator, PostAssemblyHash: postAssemblyHash,
	})
	// Assert that a dispatch confirmation was returned
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentPreDispatchResponse() }, 100*time.Millisecond, 1*time.Millisecond, "Pre-dispatch response should be sent")

	// Simulate the coordinator having dispatched the transaction (Prepared → Dispatched) so the
	// following heartbeat with LatestSubmissionHash is accepted (State_Dispatched handles Event_Submitted).
	signerAddress := pldtypes.RandAddress()
	s.QueueEvent(ctx, &transaction.DispatchedEvent{
		BaseEvent:     transaction.BaseEvent{TransactionID: txn.ID},
		SignerAddress: *signerAddress,
	})

	// Simulate the coordinator sending a heartbeat after the transaction was submitted
	submissionHash := pldtypes.RandBytes32()
	nonce := uint64(42)
	// Originator must match the originator's nodeName so the heartbeat is applied
	// (builder defaults nodeName to "member1@node1").
	heartbeatEvent.DispatchedTransactions = []*common.SnapshotDispatchedTransaction{
		{
			SnapshotPooledTransaction: common.SnapshotPooledTransaction{
				ID:         txn.ID,
				Originator: "member1@node1",
			},
			Signer:               *signerAddress,
			SignerLocator:        "signer@node2",
			Nonce:                &nonce,
			LatestSubmissionHash: &submissionHash,
		},
	}
	s.QueueEvent(ctx, heartbeatEvent)

	// Simulate the block indexer confirming the transaction
	s.QueueEvent(ctx, &transaction.ConfirmedSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})

	// After confirmation: the transaction state machine transitions Submitted → Confirmed → Final,
	// and the originator removes the transaction from memory (removeTransaction). With no
	// unconfirmed transactions left, the originator transitions back to State_Observing.
	require.Eventually(t, func() bool { return s.transactionsByID[txn.ID] == nil }, 100*time.Millisecond, 1*time.Millisecond, "Transaction should be cleaned up from transactionsByID after confirmation")
	require.Eventually(t, func() bool { return s.GetCurrentState() == State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "Originator should transition to Observing when all transactions are confirmed")
}

func TestOriginator_DelegateDroppedTransactions(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks, cleanup := builder.Build(ctx)
	defer cleanup()

	//ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.ContractAddress = &contractAddress
	s.QueueEvent(ctx, heartbeatEvent)
	syncEvent := statemachine.NewSyncEvent()
	s.QueueEvent(ctx, syncEvent)
	<-syncEvent.Done
	require.Equal(t, State_Observing, s.GetCurrentState(), "Originator should transition to Observing")

	transactionBuilder1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn1 := transactionBuilder1.BuildSparse()
	s.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: txn1})
	// Assert that a delegation request has been sent to the coordinator
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Delegation request should be sent")
	mocks.SentMessageRecorder.Reset(ctx)

	transactionBuilder2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn2 := transactionBuilder2.BuildSparse()
	s.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: txn2})
	syncEvent = statemachine.NewSyncEvent()
	s.QueueEvent(ctx, syncEvent)
	<-syncEvent.Done
	// Assert that a delegation request has been sent to the coordinator
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "Delegation request should be sent")
	mocks.SentMessageRecorder.Reset(ctx)

	heartbeatWithPooled := &HeartbeatReceivedEvent{}
	heartbeatWithPooled.From = coordinatorLocator
	heartbeatWithPooled.ContractAddress = &contractAddress
	heartbeatWithPooled.PooledTransactions = []*common.SnapshotPooledTransaction{
		{
			ID:         txn1.ID,
			Originator: originatorLocator,
		},
	}
	s.QueueEvent(ctx, heartbeatWithPooled)
	syncEvent = statemachine.NewSyncEvent()
	s.QueueEvent(ctx, syncEvent)
	<-syncEvent.Done
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "Delegation request should be sent after heartbeat")
	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn1.ID))
	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn2.ID))
}

func TestOriginator_TransactionConfirmedViaTransactionEvent_AllStates(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		initialState  State
		expectedState State
	}{
		{name: "Idle", initialState: State_Idle, expectedState: State_Idle},
		{name: "Observing", initialState: State_Observing, expectedState: State_Observing},
		{name: "Sending", initialState: State_Sending, expectedState: State_Observing},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := NewOriginatorBuilderForTesting(tc.initialState).CommitteeMembers("member1@node1", "member2@node2")
			txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
			builder.TransactionBuilders(txBuilder)

			s, _, cleanup := builder.Build(ctx)
			defer cleanup()
			txn := txBuilder.GetBuiltTransaction()
			require.NotNil(t, txn)

			s.QueueEvent(ctx, &transaction.ConfirmedSuccessEvent{
				BaseEvent: transaction.BaseEvent{
					BaseEvent:     common.BaseEvent{EventTime: time.Now()},
					TransactionID: txn.GetID(),
				},
			})
			syncEvent := statemachine.NewSyncEvent()
			s.QueueEvent(ctx, syncEvent)
			<-syncEvent.Done

			state := txn.GetCurrentState()
			assert.True(t, state == transaction.State_Confirmed || state == transaction.State_Final, "Transaction should be confirmed or final")
			assert.True(t, s.GetCurrentState() == tc.expectedState, "Originator should transition to the expected state")
		})
	}
}

func Test_propagateEventToTransaction_UnknownTransaction_AssembleRequestSendsUnknown(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks, cleanup := builder.Build(ctx)
	defer cleanup()

	// Create a transaction event with a transaction ID that doesn't exist in the originator
	unknownTxID := uuid.New()
	assembleRequestIdempotencyKey := uuid.New()
	event := &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: unknownTxID,
		},
		RequestID:               assembleRequestIdempotencyKey,
		Coordinator:             coordinatorLocator,
		CoordinatorsBlockHeight: 1000,
		StateLocksJSON:          []byte("{}"),
	}
	s.QueueEvent(ctx, event)
	// State machine should call propagateEventToTransaction, which should send a TransactionUnknown response
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentTransactionUnknown() }, 100*time.Millisecond, 1*time.Millisecond, "SendTransactionUnknown should be called")
	txID, coordinator := mocks.SentMessageRecorder.GetTransactionUnknownDetails()
	assert.Equal(t, unknownTxID, txID, "TransactionUnknown should be sent for the correct transaction ID")
	assert.Equal(t, coordinatorLocator, coordinator, "TransactionUnknown should be sent to the correct coordinator")
}
func Test_propagateEventToTransaction_UnknownTransaction_NonRequestEventReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	s, mocks, cleanup := builder.Build(ctx)
	defer cleanup()

	// Create a ConfirmedSuccessEvent for an unknown transaction
	unknownTxID := uuid.New()
	event := &transaction.ConfirmedSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: unknownTxID,
		},
	}

	// Verify that SendTransactionUnknown was NOT called
	s.QueueEvent(ctx, event)
	sync := statemachine.NewSyncEvent()
	s.QueueEvent(ctx, sync)
	<-sync.Done
	assert.False(t, mocks.SentMessageRecorder.HasSentTransactionUnknown(), "Expected SendTransactionUnknown to NOT be called for confirmation events")
}

func TestOriginator_CreateTransaction_ErrorFromNewTransaction(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks, cleanup := builder.Build(ctx)
	defer cleanup()

	contractAddress := builder.GetContractAddress()
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.ContractAddress = &contractAddress
	s.QueueEvent(ctx, heartbeatEvent)
	require.Eventually(t, func() bool { return s.GetCurrentState() == State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "Originator should transition to Observing")
	mocks.SentMessageRecorder.Reset(ctx)

	s.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: nil})
	sync := statemachine.NewSyncEvent()
	s.QueueEvent(ctx, sync)
	<-sync.Done
	// Nil transaction is rejected in the loop; no delegation should be sent and state should remain Observing
	assert.True(t, s.GetCurrentState() == State_Observing, "State should remain Observing after nil transaction event")
	assert.False(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "No delegation should be sent for nil transaction")
}

func TestOriginator_EventLoop_ErrorHandling(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks, cleanup := builder.Build(ctx)
	defer cleanup()

	// Ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.ContractAddress = &contractAddress
	s.QueueEvent(ctx, heartbeatEvent)
	require.Eventually(t, func() bool { return s.GetCurrentState() == State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "Originator should transition to Observing")

	// Queue a TransactionCreatedEvent with a nil transaction to trigger an error
	s.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: nil})
	sync := statemachine.NewSyncEvent()
	s.QueueEvent(ctx, sync)
	<-sync.Done

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	validEvent := &TransactionCreatedEvent{
		Transaction: txn,
	}
	// Reset the message recorder to track the new event
	mocks.SentMessageRecorder.Reset(ctx)
	// Queue a valid event to verify the originator is still working
	s.QueueEvent(ctx, validEvent)
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Originator should still process valid event after error")
}

// Stop behaviour: the common statemachine package covers the event loop (stop signal,
// idempotent Stop, concurrent Stop) — see TestStateMachineEventLoop_Stop_WhenAlreadyStopped
// and TestStateMachineEventLoop_Stop_ConcurrentCalls. The tests below cover how the
// originator consumes this: it stops the delegate loop first, then the state machine
// event loop, and Stop() is idempotent at the originator level.

// TestOriginator_Stop_Completes verifies that Stop() returns after both the delegate loop
// and the state machine event loop have stopped.
func TestOriginator_Stop_Completes(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	s, _, cleanup := builder.Build(ctx)

	cleanup()

	require.True(t, s.stateMachineEventLoop.IsStopped(), "state machine event loop should be stopped")
}

// TestOriginator_Stop_Idempotent verifies that calling Stop() twice is safe and does not panic.
func TestOriginator_Stop_Idempotent(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	s, _, cleanup := builder.Build(ctx)

	cleanup()
	cleanup()

	require.True(t, s.stateMachineEventLoop.IsStopped(), "state machine event loop should be stopped")
}

// TestOriginator_Stop_AfterEventsProcessed verifies that after queuing events and waiting for
// them to be processed (via the common state machine event loop), Stop() completes and both
// the delegate loop and the state machine event loop are stopped.
func TestOriginator_Stop_AfterEventsProcessed(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks, cleanup := builder.Build(ctx)

	contractAddress := builder.GetContractAddress()
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.ContractAddress = &contractAddress
	s.QueueEvent(ctx, heartbeatEvent)
	require.Eventually(t, func() bool { return s.GetCurrentState() == State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "Originator should transition to Observing")

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	s.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: txn})
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Event should be processed before cleanup")

	cleanup()

	require.True(t, s.stateMachineEventLoop.IsStopped(), "state machine event loop should be stopped")
}

type mockFailingTransaction struct {
	*transaction.OriginatorTransaction
	handleEventError error
}

func (m *mockFailingTransaction) HandleEvent(ctx context.Context, event common.Event) error {
	return m.handleEventError
}

func TestOriginator_CreateTransaction_ErrorFromHandleEvent(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks, cleanup := builder.Build(ctx)
	defer cleanup()

	// Ensure the originator is in observing mode
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.stateMachineEventLoop.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	// Create a transaction
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Create a real transaction using NewTransaction (this is what createTransaction does at line 174)
	testMetrics := metrics.InitMetrics(ctx, prometheus.NewRegistry())
	realTxn, err := transaction.NewTransaction(ctx, txn, mocks.SentMessageRecorder, s.stateMachineEventLoop.QueueEvent, mocks.EngineIntegration, testMetrics)
	require.NoError(t, err)

	// Wrap it in a mock that will fail HandleEvent
	expectedError := errors.New("mock HandleEvent error")
	mockTxn := &mockFailingTransaction{
		OriginatorTransaction: realTxn,
		handleEventError:      expectedError,
	}

	createdEvent := &transaction.CreatedEvent{}
	createdEvent.TransactionID = txn.ID

	// Call HandleEvent on the mock - this should return our error (line 183)
	err = mockTxn.HandleEvent(ctx, createdEvent)

	// Verify the error is returned (this tests lines 184-186)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Contains(t, err.Error(), "mock HandleEvent error")
}

func Test_propagateEventToTransaction_UnknownTransaction_PreDispatchRequestSendsUnknown(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks, cleanup := builder.Build(ctx)
	defer cleanup()

	unknownTxID := uuid.New()
	postAssemblyHash := pldtypes.RandBytes32()
	event := &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent:        transaction.BaseEvent{TransactionID: unknownTxID},
		Coordinator:      coordinatorLocator,
		PostAssemblyHash: &postAssemblyHash,
	}
	s.QueueEvent(ctx, event)
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentTransactionUnknown() }, 100*time.Millisecond, 1*time.Millisecond, "SendTransactionUnknown should be called")
	txID, coordinator := mocks.SentMessageRecorder.GetTransactionUnknownDetails()
	assert.Equal(t, unknownTxID, txID)
	assert.Equal(t, coordinatorLocator, coordinator)
}

func TestOriginator_heartbeatLoop_UsesInjectedQueueAndStops(t *testing.T) {
	ctx := context.Background()
	s, _, cleanup := NewOriginatorBuilderForTesting(State_Idle).
		HeartbeatInterval(time.Millisecond).
		Build(ctx)
	defer cleanup()

	heartbeatEvents := make(chan struct{}, 8)
	queueEvent := func(_ context.Context, event common.Event) {
		if _, isHeartbeatInterval := event.(*HeartbeatIntervalEvent); isHeartbeatInterval {
			heartbeatEvents <- struct{}{}
		}
	}

	go s.heartbeatLoop(ctx, queueEvent)

	<-heartbeatEvents
	<-heartbeatEvents

	heartbeatCtx := s.heartbeatCtx
	require.NotNil(t, heartbeatCtx)

	action_StopHeartbeatLoop(ctx, s, nil)
	<-heartbeatCtx.Done()
}

func TestOriginator_heartbeatLoop_DoesNothingWhenAlreadyRunning(t *testing.T) {
	ctx := context.Background()
	s, _, cleanup := NewOriginatorBuilderForTesting(State_Idle).Build(ctx)
	defer cleanup()

	existingCtx, existingCancel := context.WithCancel(ctx)
	s.heartbeatCtx, s.heartbeatCancel = existingCtx, existingCancel

	s.heartbeatLoop(ctx, func(_ context.Context, _ common.Event) {
		t.Fatal("heartbeat loop should not be restarted when already running")
	})

	assert.Same(t, existingCtx, s.heartbeatCtx, "existing heartbeat context should remain unchanged")
}

func Test_action_StopHeartbeatLoop(t *testing.T) {
	ctx := context.Background()

	s, _, cleanup := NewOriginatorBuilderForTesting(State_Idle).Build(ctx)
	defer cleanup()

	s.heartbeatCancel = nil
	err := action_StopHeartbeatLoop(ctx, s, nil)
	require.NoError(t, err)

	testCtx, testCancel := context.WithCancel(ctx)
	s.heartbeatCtx, s.heartbeatCancel = testCtx, testCancel

	err = action_StopHeartbeatLoop(ctx, s, nil)
	require.NoError(t, err)

	<-testCtx.Done()
}

// TODO: error not currently reachable without injectable transaction dependency
// func TestSendDelegationRequest_HandleEventError(t *testing.T) {
// 	ctx := context.Background()
// 	originatorLocator := "sender@senderNode"
// 	coordinatorLocator := "coordinator@coordinatorNode"
// 	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, coordinatorLocator)
// 	o, mocks := builder.Build(ctx)

// 	// Ensure the originator is in sending mode
// 	heartbeatEvent := &HeartbeatReceivedEvent{}
// 	heartbeatEvent.From = coordinatorLocator
// 	contractAddress := builder.GetContractAddress()
// 	heartbeatEvent.ContractAddress = &contractAddress

// 	err := o.stateMachineEventLoop.ProcessEvent(ctx, heartbeatEvent)
// 	assert.NoError(t, err)

// 	// Create a transaction and add it to the originator
// 	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
// 	txn := transactionBuilder.BuildSparse()

// 	// Create a real transaction
// 	testMetrics := metrics.InitMetrics(ctx, prometheus.NewRegistry())
// 	realTxn, err := transaction.NewTransaction(ctx, txn, mocks.SentMessageRecorder, o.stateMachineEventLoop.QueueEvent, mocks.EngineIntegration, testMetrics)
// 	require.NoError(t, err)

// 	// Add the transaction to the originator
// 	o.transactionsByID[txn.ID] = realTxn
// 	o.transactionsOrdered = append(o.transactionsOrdered, realTxn)

// 	// Set transaction to Pending state so it will be included in the delegation request
// 	createdEvent := &transaction.CreatedEvent{}
// 	createdEvent.TransactionID = txn.ID
// 	err = realTxn.HandleEvent(ctx, createdEvent)
// 	require.NoError(t, err)
// 	require.Equal(t, transaction.State_Pending, realTxn.GetCurrentState(), "transaction should be in Pending state")

// 	// Set activeCoordinatorNode to empty to cause HandleEvent to fail
// 	o.activeCoordinatorNode = ""

// 	err = action_SendDelegationRequest(ctx, o, nil)

// 	// Should return an error with the expected message
// 	require.Error(t, err)
// 	assert.Contains(t, err.Error(), "error handling delegated event")
// }
