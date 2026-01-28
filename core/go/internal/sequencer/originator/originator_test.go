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
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestOriginator_SingleTransactionLifecycle(t *testing.T) {
	// Test the progression of a single transaction through the originator's lifecycle
	// Simulating coordinator node by inspecting the originator output messages and by sending events that would normally be triggered
	//  by coordinator node sending messages to the transaction originator.
	// At each stage, we inspect the state of the transaction by querying the seoriginatornder's status API

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

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

	// Simulate the coordinator sending a heartbeat after the transaction was submitted
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
	s.QueueEvent(ctx, heartbeatEvent)

	// Simulate the block indexer confirming the transaction
	s.QueueEvent(ctx, &TransactionConfirmedEvent{From: signerAddress, Nonce: 42, Hash: submissionHash})

	//TODO AM: what is the assertion here?
}

func TestOriginator_DelegateDroppedTransactions(t *testing.T) {
	// Delegate a transaction then receive a heartbeat that does not contain that transaction, and check that
	// it continues to get re-delegated until it is included in a heartbeat

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	config := builder.GetSequencerConfig()
	config.DelegateTimeout = confutil.P("100ms")
	builder.OverrideSequencerConfig(config)
	s, mocks := builder.Build(ctx)

	//ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.ContractAddress = &contractAddress
	s.QueueEvent(ctx, heartbeatEvent)
	require.Eventually(t, func() bool { return s.GetCurrentState() == State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "Originator should transition to Observing")

	transactionBuilder1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn1 := transactionBuilder1.BuildSparse()
	s.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: txn1})
	// Assert that a delegation request has been sent to the coordinator
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Delegation request should be sent")
	mocks.SentMessageRecorder.Reset(ctx)

	transactionBuilder2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn2 := transactionBuilder2.BuildSparse()
	s.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: txn2})
	// Assert that a delegation request has been sent to the coordinator
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Delegation request should be sent")
	mocks.SentMessageRecorder.Reset(ctx)

	heartbeatWithPooled := &HeartbeatReceivedEvent{}
	heartbeatWithPooled.From = coordinatorLocator
	heartbeatWithPooled.ContractAddress = &contractAddress
	heartbeatEvent.PooledTransactions = []*common.Transaction{
		{
			ID:         txn1.ID,
			Originator: originatorLocator,
		},
	}
	// Real-time wait for delegate timeout (100ms) to elapse so re-delegation can fire; sync events cannot advance the ticker
	time.Sleep(110 * time.Millisecond)
	s.QueueEvent(ctx, heartbeatWithPooled)
	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Delegation request should be sent after heartbeat")
	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn1.ID))
	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn2.ID))
}

// TODO AM: this needs more thought
// func TestOriginator_DelegateLoopStopsOnContextCancellation(t *testing.T) {
// 	// Test that the delegate loop stops gracefully when the context is cancelled

// 	originatorLocator := "sender@senderNode"
// 	coordinatorLocator := "coordinator@coordinatorNode"
// 	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
// 	config := builder.GetSequencerConfig()
// 	config.DelegateTimeout = confutil.P("100ms")
// 	builder.OverrideSequencerConfig(config)

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	s, mocks := builder.Build(ctx)

// 	contractAddress := builder.GetContractAddress()
// 	heartbeatEvent := &HeartbeatReceivedEvent{}
// 	heartbeatEvent.From = coordinatorLocator
// 	heartbeatEvent.ContractAddress = &contractAddress
// 	s.QueueEvent(ctx, heartbeatEvent)
// 	require.Eventually(t, func() bool { return s.GetCurrentState() == State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "Originator should transition to Observing")

// 	sync := statemachine.NewSyncEvent()
// 	s.QueueEvent(ctx, sync)
// 	<-sync.Done // ensure all queued events are processed before cancel
// 	mocks.SentMessageRecorder.Reset(ctx)
// 	cancel()

// 	// With a single shared context, the state machine event loop also exits when ctx is cancelled,
// 	// so we cannot use QueueEvent + Eventually to assert "originator still processes events".
// 	// This test verifies the shutdown scenario: after cancel, originator remains in a consistent state.
// 	require.Equal(t, State_Observing, s.GetCurrentState(), "Originator should remain in Observing after context cancellation")
// }

func TestOriginator_PropagateEventToTransaction_UnknownTransaction(t *testing.T) {
	// Test that propagateEventToTransaction sends a TransactionUnknown response when receiving
	// an AssembleRequestReceivedEvent for a transaction not known to the originator

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

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
func TestOriginator_PropagateEventToTransaction_UnknownTransaction_NoResponse(t *testing.T) {
	// Test that propagateEventToTransaction does NOT send a TransactionUnknown response
	// for events that don't require a response (e.g., confirmation events)

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

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
	// Test that createTransaction handles errors from transaction.NewTransaction: when QueueEvent receives
	// a TransactionCreatedEvent with nil transaction, the error is handled in the loop and no transaction
	// is created (originator stays in Observing, no delegation is sent for that event).

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

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
	// Test that the event loop handles errors (e.g. nil transaction) and continues to process later events

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

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

// TODO AM: some of this test needs to be in the common package
// func TestOriginator_EventLoop_StopSignal(t *testing.T) {
// 	// Test that the event loop properly handles the stop signal from Stop()

// 	ctx := context.Background()
// 	originatorLocator := "sender@senderNode"
// 	coordinatorLocator := "coordinator@coordinatorNode"
// 	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
// 	s, mocks := builder.Build(ctx)

// 	// Ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
// 	contractAddress := builder.GetContractAddress()
// 	heartbeatEvent := &HeartbeatReceivedEvent{}
// 	heartbeatEvent.From = coordinatorLocator
// 	heartbeatEvent.ContractAddress = &contractAddress
// 	s.QueueEvent(ctx, heartbeatEvent)
// 	require.Eventually(t, func() bool { return s.GetCurrentState() == State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "Originator should transition to Observing")

// 	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
// 	txn := transactionBuilder.BuildSparse()
// 	s.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: txn})
// 	require.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Event should be processed before Stop()")

// 	// Call Stop() - this should send a signal to stopEventLoop channel, and then wait for it
// 	s.Stop()

// 	// // Verify that Stop() completed by loading up len(s.originatorEvents) events but no more. These should be buffered and hence should not block
// 	// transactionBuilder2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
// 	// txn2 := transactionBuilder2.BuildSparse()
// 	// event2 := &TransactionCreatedEvent{
// 	// 	Transaction: txn2,
// 	// }

// 	// for i := 0; i < len(s.originatorEvents); i++ {
// 	// 	s.QueueEvent(ctx, event2)
// 	// }
// }
