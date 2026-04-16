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

func Test_getTransactionsInStates_ReturnsOnlyTransactionsWhoseStateIsListed(t *testing.T) {
	ctx := context.Background()
	tbPending := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pending)
	tbDelegated := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	tbAssembling := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)

	builder := NewOriginatorBuilderForTesting(State_Sending).
		CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode").
		TransactionBuilders(tbPending, tbDelegated, tbAssembling)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	matching := o.getTransactionsInStates([]transaction.State{transaction.State_Pending, transaction.State_Delegated})
	require.Len(t, matching, 2)

	byID := make(map[uuid.UUID]transaction.State)
	for _, txn := range matching {
		byID[txn.GetID()] = txn.GetCurrentState()
	}
	assert.Equal(t, transaction.State_Pending, byID[tbPending.GetBuiltTransaction().GetID()])
	assert.Equal(t, transaction.State_Delegated, byID[tbDelegated.GetBuiltTransaction().GetID()])
}

func Test_getTransactionsInStates_EmptyStateListReturnsNoTransactions(t *testing.T) {
	ctx := context.Background()
	tb := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pending)
	builder := NewOriginatorBuilderForTesting(State_Sending).
		CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode").
		TransactionBuilders(tb)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	assert.Empty(t, o.getTransactionsInStates(nil))
	assert.Empty(t, o.getTransactionsInStates([]transaction.State{}))
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
