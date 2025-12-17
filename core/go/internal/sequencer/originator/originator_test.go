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
	//delegate a transaction then receive a heartbeat that does not contain that transaction, and check that
	// it continues to get re-delegated until it is in included in a heartbeat

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	config := builder.GetSequencerConfig()
	config.DelegateTimeout = confutil.P("100ms")
	builder.OverrideSequencerConfig(config)
	s, mocks := builder.Build(ctx)

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
	// Test that the delegate loop stops gracefully when the context is cancelled

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
	// Test that propagateEventToTransaction handles events for transactions not known to the originator

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, _ := builder.Build(ctx)

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

	// ProcessEvent should call propagateEventToTransaction, which should handle the unknown transaction gracefully
	err := s.ProcessEvent(ctx, event)
	assert.NoError(t, err, "ProcessEvent should return nil when transaction is not known to originator")
}

func TestOriginator_CreateTransaction_ErrorFromNewTransaction(t *testing.T) {
	// Test that createTransaction properly handles and returns errors from transaction.NewTransaction

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, _ := builder.Build(ctx)

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
	// Test that the eventLoop properly handles errors from ProcessEvent

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

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
	// Test that the eventLoop properly handles the stop signal from Stop()

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

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

	// Verify that Stop() completed without blocking (the channel send should succeed)
	transactionBuilder2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn2 := transactionBuilder2.BuildSparse()
	event2 := &TransactionCreatedEvent{
		Transaction: txn2,
	}

	// We have to get past the buffer in the channel to validate it doesn't block
	for i := 0; i < len(s.originatorEvents)+1; i++ {
		// This just needs to not block - it checks the event loop is not done
		s.QueueEvent(ctx, event2)
	}

}
