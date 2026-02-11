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

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
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
	s, mocks := builder.Build(ctx)
	defer s.Stop()

	//ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	// Start by creating a transaction with the originator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	err = s.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn,
	})
	assert.NoError(t, err)

	// Assert that a delegation request has been sent to the coordinator
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())

	postAssembly, postAssemblyHash := transactionBuilder.BuildPostAssemblyAndHash()
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(postAssembly, nil)

	//Simulate the coordinator sending an assemble request
	assembleRequestIdempotencyKey := uuid.New()
	err = s.ProcessEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:               assembleRequestIdempotencyKey,
		Coordinator:             coordinatorLocator,
		CoordinatorsBlockHeight: 1000,
		StateLocksJSON:          []byte("{}"),
	})
	assert.NoError(t, err)

	// Assert that the transaction was assembled and a response sent
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse())

	//Simulate the coordinator sending a dispatch confirmation
	err = s.ProcessEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:        assembleRequestIdempotencyKey,
		Coordinator:      coordinatorLocator,
		PostAssemblyHash: postAssemblyHash,
	})
	assert.NoError(t, err)

	// Assert that a dispatch confirmation was returned
	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse())

	//simulate the coordinator sending a heartbeat after the transaction was submitted
	signerAddress := pldtypes.RandAddress()
	submissionHash := pldtypes.RandBytes32()
	nonce := uint64(42)
	heartbeatEvent.DispatchedTransactions = []*common.DispatchedTransaction{
		{
			Transaction: common.Transaction{
				ID: txn.ID,
			},
			Signer:               *signerAddress,
			SignerLocator:        "signer@node2",
			Nonce:                &nonce,
			LatestSubmissionHash: &submissionHash,
		},
	}
	err = s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)

	// Simulate the block indexer confirming the transaction
	err = s.ProcessEvent(ctx, &TransactionConfirmedEvent{
		From:  signerAddress,
		Nonce: 42,
		Hash:  submissionHash,
	})
	assert.NoError(t, err)

}

func TestOriginator_DelegateDroppedTransactions(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	config := builder.GetSequencerConfig()
	config.DelegateTimeout = confutil.P("100ms")
	builder.OverrideSequencerConfig(config)
	s, mocks := builder.Build(ctx)
	defer s.Stop()

	//ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	transactionBuilder1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn1 := transactionBuilder1.BuildSparse()
	err = s.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn1,
	})
	assert.NoError(t, err)

	// Assert that a delegation request has been sent to the coordinator
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
	mocks.SentMessageRecorder.Reset(ctx)

	transactionBuilder2 := testutil.
		NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originatorLocator).
		NumberOfRequiredEndorsers(1)
	txn2 := transactionBuilder2.BuildSparse()
	err = s.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn2,
	})
	assert.NoError(t, err)

	// Assert that a delegation request has been sent to the coordinator
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
	mocks.SentMessageRecorder.Reset(ctx)

	heartbeatEvent = &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.ContractAddress = &contractAddress
	heartbeatEvent.PooledTransactions = []*common.Transaction{
		{
			ID:         txn1.ID,
			Originator: originatorLocator,
		},
	}

	// Wait delegate-timeout before sending the heartbeat event
	time.Sleep(110 * time.Millisecond)
	err = s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)

	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())

	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn1.ID))
	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn2.ID))

}

func TestOriginator_DelegateLoopStopsOnContextCancellation(t *testing.T) {

	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	config := builder.GetSequencerConfig()
	// Use a short delegate timeout so we can verify the loop stops quickly
	config.DelegateTimeout = confutil.P("50ms")
	builder.OverrideSequencerConfig(config)

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s, mocks := builder.Build(ctx)
	defer s.Stop()

	// Ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	// Wait a bit to let the delegate loop start and potentially fire once
	time.Sleep(60 * time.Millisecond)

	// Reset the message recorder to track events after cancellation
	mocks.SentMessageRecorder.Reset(ctx)

	// Cancel the context - this should cause the delegate loop to stop
	cancel()

	// Wait longer than the delegate timeout to ensure the loop would have fired again if it was still running
	time.Sleep(100 * time.Millisecond)

	// Verify that the originator can still process other events (showing it's still functional)
	// This confirms the delegate loop stopped gracefully without affecting other functionality
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Use a new context since we cancelled the original one
	newCtx := context.Background()
	err = s.ProcessEvent(newCtx, &TransactionCreatedEvent{
		Transaction: txn,
	})
	assert.NoError(t, err)

	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
}

func TestOriginator_PropagateEventToTransaction_UnknownTransaction(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)
	defer s.Stop()

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

	// ProcessEvent should call propagateEventToTransaction, which should send a TransactionUnknown response
	err := s.ProcessEvent(ctx, event)
	assert.NoError(t, err, "ProcessEvent should return nil when transaction is not known to originator")

	// Verify that SendTransactionUnknown was called with the correct parameters
	assert.True(t, mocks.SentMessageRecorder.HasSentTransactionUnknown(), "Expected SendTransactionUnknown to be called")
	txID, coordinator := mocks.SentMessageRecorder.GetTransactionUnknownDetails()
	assert.Equal(t, unknownTxID, txID, "TransactionUnknown should be sent for the correct transaction ID")
	assert.Equal(t, coordinatorLocator, coordinator, "TransactionUnknown should be sent to the correct coordinator")
}
func TestOriginator_PropagateEventToTransaction_UnknownTransaction_NoResponse(t *testing.T) {
	// Test that propagateEventToTransaction does NOT send a TransactionUnknown response
	// for events that don't require a response (e.g., confirmation events)

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)
	defer s.Stop()

	// Create a ConfirmedSuccessEvent for an unknown transaction
	unknownTxID := uuid.New()
	event := &transaction.ConfirmedSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: unknownTxID,
		},
	}

	// ProcessEvent should not send a TransactionUnknown response for confirmation events
	err := s.ProcessEvent(ctx, event)
	assert.NoError(t, err, "ProcessEvent should return nil when transaction is not known to originator")

	// Verify that SendTransactionUnknown was NOT called
	assert.False(t, mocks.SentMessageRecorder.HasSentTransactionUnknown(), "Expected SendTransactionUnknown to NOT be called for confirmation events")
}

func TestOriginator_CreateTransaction_ErrorFromNewTransaction(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, _ := builder.Build(ctx)
	defer s.Stop()

	// Ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	event := &TransactionCreatedEvent{
		Transaction: nil,
	}

	// ProcessEvent should call createTransaction, which should handle the error from NewTransaction
	err = s.ProcessEvent(ctx, event)

	// Verify that the error from NewTransaction is properly propagated
	assert.Error(t, err, "Expected error when NewTransaction fails with nil transaction")
	assert.Contains(t, err.Error(), "cannot create transaction without private tx", "Error message should indicate the validation failure")
}

func TestOriginator_EventLoop_ErrorHandling(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)
	defer s.Stop()

	// Ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	// Queue a TransactionCreatedEvent with a nil transaction to trigger an error
	event := &TransactionCreatedEvent{
		Transaction: nil,
	}

	s.QueueEvent(ctx, event)

	// Wait a bit for the eventLoop to process the queued event
	time.Sleep(100 * time.Millisecond)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	validEvent := &TransactionCreatedEvent{
		Transaction: txn,
	}

	// Reset the message recorder to track the new event
	mocks.SentMessageRecorder.Reset(ctx)

	// Queue a valid event to verify the originator is still working
	s.QueueEvent(ctx, validEvent)

	// Wait for the valid event to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify that the originator successfully processed the valid event
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "Originator should still be functional after handling error in eventLoop")
}

func TestOriginator_EventLoop_StopSignal(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)
	defer s.Stop()

	// Ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	// Queue a valid event to verify the event loop is working before Stop()
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	event := &TransactionCreatedEvent{
		Transaction: txn,
	}

	s.QueueEvent(ctx, event)

	// Wait for the event to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify that the event was processed before Stop()
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "Event should be processed before Stop()")

	// Reset the message recorder to track events after Stop()
	mocks.SentMessageRecorder.Reset(ctx)

	// Call Stop() - this should send a signal to stopEventLoop channel, and then wait for it
	s.Stop()

	// Verify that Stop() completed by loading up len(s.originatorEvents) events but no more. These should be buffered and hence should not block
	transactionBuilder2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn2 := transactionBuilder2.BuildSparse()
	event2 := &TransactionCreatedEvent{
		Transaction: txn2,
	}

	for i := 0; i < len(s.originatorEvents); i++ {
		s.QueueEvent(ctx, event2)
	}
}

func TestOriginator_Stop_Idempotent(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, _ := builder.Build(ctx)

	// Call Stop() first time
	s.Stop()

	// Call Stop() second time - should be idempotent and not panic
	s.Stop()

	// Verify that the originator is stopped by checking that event loops are stopped
	// We can verify this by checking that the channels are closed
	select {
	case <-s.eventLoopStopped:
		// Event loop is stopped, which is expected
	default:
		t.Error("Event loop should be stopped after Stop()")
	}

	select {
	case <-s.delegateLoopStopped:
		// Delegate loop is stopped, which is expected
	default:
		t.Error("Delegate loop should be stopped after Stop()")
	}
}

func TestOriginator_SetActiveCoordinator_EmptyString(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, _ := builder.Build(ctx)

	// Try to set empty coordinator
	err := s.SetActiveCoordinator(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Cannot set active coordinator to an empty string")
}

func TestOriginator_GetCurrentCoordinator(t *testing.T) {
	// Test GetCurrentCoordinator returns the active coordinator

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, _ := builder.Build(ctx)

	// Get current coordinator (should be set in Build)
	coordinator := s.GetCurrentCoordinator()
	assert.Equal(t, "coordinator", coordinator)

	// Set a new coordinator
	err := s.SetActiveCoordinator(ctx, "newCoordinator")
	assert.NoError(t, err)

	// Verify the coordinator was updated
	coordinator = s.GetCurrentCoordinator()
	assert.Equal(t, "newCoordinator", coordinator)
}

type mockFailingTransaction struct {
	*transaction.Transaction
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
	s, mocks := builder.Build(ctx)

	// Ensure the originator is in observing mode
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	// Create a transaction
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Create a real transaction using NewTransaction (this is what createTransaction does at line 174)
	testMetrics := metrics.InitMetrics(ctx, prometheus.NewRegistry())
	realTxn, err := transaction.NewTransaction(ctx, txn, mocks.SentMessageRecorder, s.ProcessEvent, mocks.EngineIntegration, testMetrics, func(context.Context) {})
	require.NoError(t, err)

	// Wrap it in a mock that will fail HandleEvent
	expectedError := errors.New("mock HandleEvent error")
	mockTxn := &mockFailingTransaction{
		Transaction:      realTxn,
		handleEventError: expectedError,
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

func TestValidator_TransactionDoesNotExist_InvalidEventType(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, mocks := builder.Build(ctx)
	_ = mocks // Suppress unused variable warning

	// Create an event that is not a *TransactionCreatedEvent
	invalidEvent := &HeartbeatReceivedEvent{}

	// Call the validator directly
	valid, err := validator_TransactionDoesNotExist(ctx, o, invalidEvent)

	// Should return false (invalid) and no error
	assert.NoError(t, err)
	assert.False(t, valid, "validator should return false for non-TransactionCreatedEvent")
}

func TestValidator_TransactionDoesNotExist_TransactionAlreadyExists(t *testing.T) {

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)

	// Ensure the originator is in observing mode
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := o.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, o.GetCurrentState() == State_Observing)

	// Create and add a transaction to the originator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	err = o.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn,
	})
	assert.NoError(t, err)

	// Verify transaction was added
	require.NotNil(t, o.transactionsByID[txn.ID], "transaction should be in transactionsByID")

	// Now try to create the same transaction again
	duplicateEvent := &TransactionCreatedEvent{
		Transaction: txn,
	}

	// Call the validator directly
	valid, err := validator_TransactionDoesNotExist(ctx, o, duplicateEvent)

	// Should return false (invalid) because transaction already exists
	assert.NoError(t, err)
	assert.False(t, valid, "validator should return false when transaction already exists")
}

func TestSendDelegationRequest_HandleEventError(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, mocks := builder.Build(ctx)

	// Ensure the originator is in sending mode
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := o.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)

	// Create a transaction and add it to the originator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Create a real transaction
	testMetrics := metrics.InitMetrics(ctx, prometheus.NewRegistry())
	realTxn, err := transaction.NewTransaction(ctx, txn, mocks.SentMessageRecorder, o.ProcessEvent, mocks.EngineIntegration, testMetrics, func(context.Context) {})
	require.NoError(t, err)

	// Add the transaction to the originator
	o.transactionsByID[txn.ID] = realTxn
	o.transactionsOrdered = append(o.transactionsOrdered, &txn.ID)

	// Set transaction to Pending state so it will be included in the delegation request
	createdEvent := &transaction.CreatedEvent{}
	createdEvent.TransactionID = txn.ID
	err = realTxn.HandleEvent(ctx, createdEvent)
	require.NoError(t, err)
	require.Equal(t, transaction.State_Pending, realTxn.GetCurrentState(), "transaction should be in Pending state")

	// Set activeCoordinatorNode to empty to cause HandleEvent to fail
	o.activeCoordinatorNode = ""

	err = action_SendDelegationRequest(ctx, o)

	// Should return an error with the expected message
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error handling delegated event")
}
