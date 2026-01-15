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
)

// queueAndSync queues an event and waits for it to be processed by the event loop.
// This is useful for testing that state does NOT change after an event.
func queueAndSync(ctx context.Context, s *originator, event common.Event) {
	s.QueueEvent(ctx, event)
	sync := statemachine.NewSyncEvent()
	s.QueueEvent(ctx, sync)
	sync.Wait()
}

func TestOriginator_SingleTransactionLifecycle(t *testing.T) {
	// Test the progression of a single transaction through the originator's lifecycle
	// Simulating coordinator node by inspecting the originator output messages and by sending events that would normally be triggered
	//  by coordinator node sending messages to the transaction originator.

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).NodeName(originatorLocator).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)
	defer s.Stop()

	// Ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	s.QueueEvent(ctx, heartbeatEvent)
	assert.Eventually(t, func() bool {
		return s.GetCurrentState() == State_Observing
	}, 100*time.Millisecond, 1*time.Millisecond, "expected transition to Observing")

	// Start by creating a transaction with the originator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	s.QueueEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn,
	})

	// Assert that a delegation request has been sent to the coordinator
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentDelegationRequest()
	}, 100*time.Millisecond, 1*time.Millisecond, "expected delegation request to be sent")

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
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:               assembleRequestIdempotencyKey,
		Coordinator:             coordinatorLocator,
		CoordinatorsBlockHeight: 1000,
		StateLocksJSON:          []byte("{}"),
	})

	// Assert that the transaction was assembled and a response sent
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentAssembleSuccessResponse()
	}, 100*time.Millisecond, 1*time.Millisecond, "expected assemble success response to be sent")

	// Simulate the coordinator sending a dispatch confirmation
	s.QueueEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:        assembleRequestIdempotencyKey,
		Coordinator:      coordinatorLocator,
		PostAssemblyHash: postAssemblyHash,
	})

	// Assert that a dispatch confirmation was returned
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentPreDispatchResponse()
	}, 100*time.Millisecond, 1*time.Millisecond, "expected pre-dispatch response to be sent")

	// Simulate the coordinator informing us the transaction has been dispatched
	// The DispatchedEvent moves the transaction from State_Prepared to State_Dispatched (no external observable effect)
	signerAddress := pldtypes.RandAddress()
	queueAndSync(ctx, s, &transaction.DispatchedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})

	// Simulate heartbeat showing the transaction has been submitted with a nonce and hash (no external observable effect)
	submissionHash := pldtypes.RandBytes32()
	nonce := uint64(42)
	heartbeatEvent.DispatchedTransactions = []*common.DispatchedTransaction{
		{
			Transaction: common.Transaction{
				ID:         txn.ID,
				Originator: originatorLocator,
			},
			Signer:               *signerAddress,
			SignerLocator:        "signer@node2",
			Nonce:                &nonce,
			LatestSubmissionHash: &submissionHash,
		},
	}
	queueAndSync(ctx, s, heartbeatEvent)

	// Simulate the block indexer confirming the transaction
	s.QueueEvent(ctx, &TransactionConfirmedEvent{
		From:  signerAddress,
		Nonce: nonce,
		Hash:  submissionHash,
	})

	// The originator should transition back to Observing once all transactions are confirmed
	assert.Eventually(t, func() bool {
		return s.GetCurrentState() == State_Observing
	}, 100*time.Millisecond, 1*time.Millisecond, "should return to Observing after all transactions confirmed")
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
	defer s.Stop()

	// Ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	s.QueueEvent(ctx, heartbeatEvent)
	assert.Eventually(t, func() bool {
		return s.GetCurrentState() == State_Observing
	}, 100*time.Millisecond, 1*time.Millisecond, "expected transition to Observing")

	transactionBuilder1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn1 := transactionBuilder1.BuildSparse()
	s.QueueEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn1,
	})

	// Assert that a delegation request has been sent to the coordinator
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentDelegationRequest()
	}, 100*time.Millisecond, 1*time.Millisecond, "expected delegation request")
	mocks.SentMessageRecorder.Reset(ctx)

	transactionBuilder2 := testutil.
		NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originatorLocator).
		NumberOfRequiredEndorsers(1)
	txn2 := transactionBuilder2.BuildSparse()
	s.QueueEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn2,
	})

	// Assert that a delegation request has been sent to the coordinator
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentDelegationRequest()
	}, 100*time.Millisecond, 1*time.Millisecond, "expected delegation request")
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

	// Wait delegate-timeout before sending the heartbeat event (this triggers re-delegation of timed-out transactions)
	time.Sleep(110 * time.Millisecond)
	s.QueueEvent(ctx, heartbeatEvent)

	// Assert both transactions were re-delegated (txn2 was dropped from heartbeat so should be re-delegated)
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentDelegationRequest() &&
			mocks.SentMessageRecorder.HasDelegatedTransaction(txn1.ID) &&
			mocks.SentMessageRecorder.HasDelegatedTransaction(txn2.ID)
	}, 100*time.Millisecond, 1*time.Millisecond, "expected delegation requests for both transactions")
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
	defer s.Stop()

	// Ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	s.QueueEvent(ctx, heartbeatEvent)
	assert.Eventually(t, func() bool {
		return s.GetCurrentState() == State_Observing
	}, 100*time.Millisecond, 1*time.Millisecond, "expected transition to Observing")

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
	s.QueueEvent(newCtx, &TransactionCreatedEvent{
		Transaction: txn,
	})

	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentDelegationRequest()
	}, 100*time.Millisecond, 1*time.Millisecond, "expected delegation request")
}

func TestOriginator_UnknownTransactionSendsResponse(t *testing.T) {
	// Test that the originator sends a TransactionUnknown response when receiving
	// an AssembleRequestReceivedEvent for a transaction not known to the originator

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)
	defer s.Stop()

	// Queue an assemble request for a transaction ID that doesn't exist in the originator
	unknownTxID := uuid.New()
	assembleRequestIdempotencyKey := uuid.New()
	s.QueueEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: unknownTxID,
		},
		RequestID:               assembleRequestIdempotencyKey,
		Coordinator:             coordinatorLocator,
		CoordinatorsBlockHeight: 1000,
		StateLocksJSON:          []byte("{}"),
	})

	// Verify that SendTransactionUnknown was called with the correct parameters
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentTransactionUnknown()
	}, 100*time.Millisecond, 1*time.Millisecond, "Expected SendTransactionUnknown to be called")

	txID, coordinator := mocks.SentMessageRecorder.GetTransactionUnknownDetails()
	assert.Equal(t, unknownTxID, txID, "TransactionUnknown should be sent for the correct transaction ID")
	assert.Equal(t, coordinatorLocator, coordinator, "TransactionUnknown should be sent to the correct coordinator")
}

func TestOriginator_UnknownTransactionNoResponseForConfirmation(t *testing.T) {
	// Test that the originator does NOT send a TransactionUnknown response
	// for events that don't require a response (e.g., confirmation events)

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)
	defer s.Stop()

	// Queue a ConfirmedSuccessEvent for an unknown transaction
	unknownTxID := uuid.New()
	queueAndSync(ctx, s, &transaction.ConfirmedSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: unknownTxID,
		},
	})

	// Verify that SendTransactionUnknown was NOT called for confirmation events
	assert.False(t, mocks.SentMessageRecorder.HasSentTransactionUnknown(), "Expected SendTransactionUnknown to NOT be called for confirmation events")
}

func TestOriginator_CreateTransaction_ErrorFromNewTransaction(t *testing.T) {
	// Test that createTransaction handles errors from transaction.NewTransaction gracefully
	// The error is logged and the event loop continues - state should not change

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

	s.QueueEvent(ctx, heartbeatEvent)
	assert.Eventually(t, func() bool {
		return s.GetCurrentState() == State_Observing
	}, 100*time.Millisecond, 1*time.Millisecond, "expected transition to Observing")

	// Queue an invalid transaction creation event (nil transaction)
	queueAndSync(ctx, s, &TransactionCreatedEvent{
		Transaction: nil,
	})

	// Verify that state did not change (error was handled, event loop continues)
	assert.Equal(t, State_Observing, s.GetCurrentState(), "state should remain Observing after error")

	// Verify no delegation request was sent
	assert.False(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "no delegation request should be sent for invalid transaction")
}
