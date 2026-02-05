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

package coordinator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_SingleTransactionLifecycle(t *testing.T) {
	// Test the progression of a single transaction through the coordinator's lifecycle
	// Simulating originator node, endorser node and the public transaction manager (submitter)
	// by inspecting the coordinator output messages and by sending events that would normally be triggered by those components sending messages to the coordinator.
	// At each stage, we inspect the state of the coordinator by checking the snapshot it produces on heartbeat messages

	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1) // Stop the dispatcher loop from progressing states - we're manually updating state throughout the test
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	c.Start()
	defer c.Stop()

	mocks.Domain.On("FixedSigningIdentity").Return("").Maybe()
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	mocks.TXManager.On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	gasVal := pldtypes.HexUint64(21000)
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args[1].(*components.PrivateTransaction)
		tx.Signer = originator
		if tx.PreAssembly != nil && tx.PreAssembly.TransactionSpecification != nil {
			tx.PreAssembly.TransactionSpecification.From = originator
		}
		var publicTxOptions pldapi.PublicTxOptions
		if tx.PreAssembly != nil {
			publicTxOptions = tx.PreAssembly.PublicTxOptions
		}
		toAddr := tx.Address
		tx.PreparedPublicTransaction = &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type:            pldapi.TransactionTypePublic.Enum(),
				Function:        "test()",
				From:            originator,
				To:              &toAddr,
				Data:            pldtypes.RawJSON("{}"),
				PublicTxOptions: publicTxOptions,
			},
			ABI: abi.ABI{&abi.Entry{Type: abi.Function, Name: "test", Inputs: abi.ParameterArray{}}},
		}
		tx.PreparedPublicTransaction.Gas = &gasVal
	}).Return(nil)
	resolvedAddr := pldtypes.RandAddress()
	mocks.KeyManager.On("ResolveEthAddressNewDatabaseTX", mock.Anything, mock.Anything).Return(resolvedAddr, nil)
	mocks.PublicTxManager.On("ValidateTransactionNOTX", mock.Anything, mock.Anything).Return(nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	waitTime := 100 * time.Millisecond
	pollingInterval := 1 * time.Millisecond

	// Start by simulating the originator and delegate a transaction to the coordinator.
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).NumberOfOutputStates(0)
	txn := transactionBuilder.BuildSparse()
	c.QueueEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "testNode",
		Originator:   originator,
		Transactions: []*components.PrivateTransaction{txn},
	})

	var snapshot *common.CoordinatorSnapshot

	// Assert that snapshot contains a transaction with matching ID
	require.Eventually(t, func() bool {
		snapshot = c.getSnapshot(ctx)
		return snapshot != nil && len(snapshot.PooledTransactions) == 1
	}, waitTime, pollingInterval, "Snapshot should contain one pooled transaction")

	require.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the pooled transaction with ID %s", txn.ID.String())

	// Assert that a request has been sent to the originator and respond with an assembled transaction
	require.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentAssembleRequest()
	}, waitTime, pollingInterval, "Assemble request should be sent")
	c.QueueEvent(ctx, &transaction.AssembleSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID:    mocks.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
		PostAssembly: transactionBuilder.BuildPostAssembly(),
		PreAssembly:  transactionBuilder.BuildPreAssembly(),
	})

	// Assert that the coordinator has sent an endorsement request to the endorser
	require.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.NumberOfSentEndorsementRequests() == 1
	}, waitTime, pollingInterval, "Endorsement request should be sent")

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	require.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the pooled transaction with ID %s", txn.ID.String())

	// now respond with an endorsement
	c.QueueEvent(ctx, &transaction.EndorsedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID:   mocks.SentMessageRecorder.SentEndorsementRequestsForPartyIdempotencyKey(transactionBuilder.GetEndorserIdentityLocator(0)),
		Endorsement: transactionBuilder.BuildEndorsement(0),
	})

	// Assert that the coordinator has sent a dispatch confirmation request to the transaction sender
	require.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest()
	}, waitTime, pollingInterval, "Dispatch confirmation request should be sent")

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	require.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the pooled transaction with ID %s", txn.ID.String())

	// now respond with a dispatch confirmation
	c.QueueEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: mocks.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})

	// Assert that the transaction is ready to be collected by the dispatcher thread
	require.Eventually(t, func() bool {
		readyTransactions := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Ready_For_Dispatch})
		return len(readyTransactions) == 1 &&
			readyTransactions[0].GetID().String() == txn.ID.String()
	}, waitTime, pollingInterval, "There should be exactly one transaction ready to dispatch")

	// Assert that snapshot no longer contains that transaction in the pooled transactions but does contain it in the dispatched transactions
	//NOTE: This is a key design point.  When a transaction is ready to be dispatched, we communicate to other nodes, via the heartbeat snapshot, that the transaction is dispatched.
	require.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.PooledTransactions) == 0 &&
			len(snapshot.DispatchedTransactions) == 1 &&
			snapshot.DispatchedTransactions[0].ID.String() == txn.ID.String()
	}, waitTime, pollingInterval, "Snapshot should contain exactly one dispatched transaction")

	// Simulate the dispatcher thread collecting the transaction and dispatching it to a public transaction manager
	c.QueueEvent(ctx, &transaction.DispatchEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})

	// Simulate the public transaction manager collecting the dispatched transaction and associating a signing address with it
	signerAddress := pldtypes.RandAddress()
	c.QueueEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})

	// Assert that we now have a signer address in the snapshot
	require.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.PooledTransactions) == 0 &&
			len(snapshot.DispatchedTransactions) == 1 &&
			snapshot.DispatchedTransactions[0].ID.String() == txn.ID.String() &&
			snapshot.DispatchedTransactions[0].Signer.String() == signerAddress.String()
	}, waitTime, pollingInterval, "Snapshot should contain dispatched transaction with signer address")

	// Simulate the dispatcher thread allocating a nonce for the transaction
	c.QueueEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: 42,
	})

	// Assert that the nonce is now included in the snapshot
	require.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.PooledTransactions) == 0 &&
			len(snapshot.DispatchedTransactions) == 1 &&
			snapshot.DispatchedTransactions[0].ID.String() == txn.ID.String() &&
			snapshot.DispatchedTransactions[0].Nonce != nil &&
			*snapshot.DispatchedTransactions[0].Nonce == uint64(42)
	}, waitTime, pollingInterval, "Snapshot should contain dispatched transaction with nonce 42")

	// Simulate the public transaction manager submitting the transaction
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	c.QueueEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SubmissionHash: submissionHash,
	})

	// Assert that the hash is now included in the snapshot
	require.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.PooledTransactions) == 0 &&
			len(snapshot.DispatchedTransactions) == 1 &&
			snapshot.DispatchedTransactions[0].ID.String() == txn.ID.String() &&
			snapshot.DispatchedTransactions[0].LatestSubmissionHash != nil &&
			*snapshot.DispatchedTransactions[0].LatestSubmissionHash == submissionHash
	}, waitTime, pollingInterval, "Snapshot should contain dispatched transaction with a submission hash")

	// Simulate the block indexer confirming the transaction
	nonce42 := pldtypes.HexUint64(42)
	c.QueueEvent(ctx, &transaction.ConfirmedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: &nonce42,
		Hash:  submissionHash,
	})

	// Assert that snapshot contains a confirmed transaction with matching ID
	require.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.ConfirmedTransactions) == 1 &&
			snapshot.ConfirmedTransactions[0].ID.String() == txn.ID.String()
	}, waitTime, pollingInterval, "Snapshot should contain exactly one confirmed transaction")

}

func TestCoordinator_MaxInflightTransactions(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxInflightTransactions = confutil.P(5)
	c, mocks := builder.Build(ctx)

	mocks.TXManager.On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)

	// Start by simulating the originator and delegate a transaction to the coordinator
	for i := range 100 {
		transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
		txn := transactionBuilder.BuildSparse()
		err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

		if i < 5 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			require.ErrorContains(t, err, "PD012642")
		}
	}
}

func TestCoordinator_Stop_StopsEventLoopAndDispatchLoop(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.Start()
	defer c.Stop()

	// Verify event loop is running
	require.False(t, c.stateMachineEventLoop.IsStopped(), "event loop should not be stopped initially")

	select {
	case <-c.dispatchLoopStopped:
		t.Fatal("dispatch loop should not be stopped initially")
	default:
	}

	// Should block until shutdown is complete
	c.Stop()

	// Verify both loops have stopped
	require.True(t, c.stateMachineEventLoop.IsStopped(), "event loop should be stopped")

	select {
	case _, ok := <-c.dispatchLoopStopped:
		require.False(t, ok, "dispatch loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("dispatch loop did not stop within timeout")
	}

	// Verify context was cancelled
	select {
	case <-c.ctx.Done():
		// Context was cancelled as expected
	default:
		t.Fatal("context should be cancelled after Stop()")
	}
}

func TestCoordinator_Stop_CallsStopLoopbackWriterOnTransport(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.Start()
	defer c.Stop()
	mockTransport := transport.NewMockTransportWriter(t)
	// StartLoopbackWriter was already called during NewCoordinator, so we don't expect it again
	mockTransport.On("StopLoopbackWriter").Return()

	// Replace the transport writer
	c.transportWriter = mockTransport

	c.Stop()

	// Verify StopLoopbackWriter was called
	mockTransport.AssertExpectations(t)
}

func TestCoordinator_Stop_CompletesSuccessfullyWhenCalledOnce(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.Start()

	c.Stop()

	// Verify both loops have stopped
	require.True(t, c.stateMachineEventLoop.IsStopped(), "event loop should be stopped")

	select {
	case _, ok := <-c.dispatchLoopStopped:
		require.False(t, ok, "dispatch loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

func TestCoordinator_Stop_StopsLoopsEvenWhenProcessingEvents(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.Start()
	defer c.Stop()

	// Queue some events to ensure loops are busy
	for i := 0; i < 10; i++ {
		c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})
	}

	c.Stop()

	// Verify both loops have stopped
	require.True(t, c.stateMachineEventLoop.IsStopped(), "event loop should be stopped")

	select {
	case _, ok := <-c.dispatchLoopStopped:
		require.False(t, ok, "dispatch loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

func TestCoordinator_Stop_WhenAlreadyStopped_ReturnsImmediately(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.Start()

	c.Stop()
	require.True(t, c.stateMachineEventLoop.IsStopped(), "event loop should be stopped")

	// Second Stop should return immediately without blocking or panicking
	c.Stop()
	require.True(t, c.stateMachineEventLoop.IsStopped(), "event loop should still be stopped")
}

func Test_propagateEventToTransaction_UnknownTransaction_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	event := &transaction.ConfirmedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{TransactionID: uuid.New()},
		Hash:                 pldtypes.Bytes32(pldtypes.RandBytes(32)),
	}

	err := c.propagateEventToTransaction(ctx, event)
	require.NoError(t, err)
	assert.Empty(t, c.transactionsByID, "transaction should not be added")
}

func TestCoordinator_SendHandoverRequest_SuccessfullySendsHandoverRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle).ActiveCoordinator("activeCoordinatorNode")
	c, mocks := builder.Build(ctx)

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	assert.True(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "handover request should have been sent")
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestWithCorrectActiveCoordinatorNode(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := "testCoordinatorNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode)
	c, _ := builder.Build(ctx)

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, addr).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestWithCorrectContractAddress(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := "activeCoordinatorNode"
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode).
		Build(ctx)

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, addr).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_HandlesErrorFromSendHandoverRequestGracefully(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := "activeCoordinatorNode"
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode).
		Build(ctx)
	expectedError := fmt.Errorf("transport error")

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, addr).Return(expectedError)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest - should not panic even when error occurs
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_HandlesEmptyActiveCoordinatorNode(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := ""
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode).
		Build(ctx)

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, addr).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_node1(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := "node1"
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode).
		Build(ctx)

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, addr).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_node2ExampleCom(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := "node2@example.com"
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode).
		Build(ctx)

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, addr).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_coordinatorNode123(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := "coordinator-node-123"
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode).
		Build(ctx)

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, addr).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_VeryLongCoordinatorNodeName(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := "very-long-coordinator-node-name-with-special-chars-123"
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode).
		Build(ctx)

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, addr).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestMultipleTimes(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := "activeCoordinatorNode"
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode).
		Build(ctx)

	// Call sendHandoverRequest multiple times
	c.sendHandoverRequest(ctx)
	c.sendHandoverRequest(ctx)
	c.sendHandoverRequest(ctx)

	assert.True(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "handover request should have been sent")
}

func TestCoordinator_SendHandoverRequest_HandlesContextCancellation(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	activeCoordinatorNode := "activeCoordinatorNode"
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		ContractAddress(addr).
		ActiveCoordinator(activeCoordinatorNode).
		Build(ctx)

	// Create a cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", cancelledCtx, activeCoordinatorNode, addr).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest with cancelled context
	c.sendHandoverRequest(cancelledCtx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsNilWhenNoTransactionsExist(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Build(ctx)

	require.Empty(t, c.transactionsByID)

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should return nil when no transactions exist")
}

func TestCoordinator_PropagateEventToAllTransactions_SuccessfullyPropagatesEventToSingleTransaction(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(txn).
		Build(ctx)

	// Propagate heartbeat event (should be handled successfully by any state)
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate event to single transaction")
}

func TestCoordinator_PropagateEventToAllTransactions_SuccessfullyPropagatesEventToMultipleTransactions(t *testing.T) {
	ctx := context.Background()
	txn1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	txn2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build(ctx)
	txn3, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(
			txn1, txn2, txn3,
		).
		Build(ctx)

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate event to all transactions")
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsErrorWhenSingleTransactionFailsToHandleEvent(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	// HeartbeatIntervalEvent should be handled successfully by all transaction states
	assert.NoError(t, err, "heartbeat event should be handled successfully")
}

func TestCoordinator_PropagateEventToAllTransactions_StopsAtFirstErrorWhenMultipleTransactionsExist(t *testing.T) {
	ctx := context.Background()
	txn1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	txn2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build(ctx)
	txn3, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(
			txn1, txn2, txn3,
		).
		Build(ctx)

	// Propagate heartbeat event - all should handle it successfully
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate to all transactions")
}

func TestCoordinator_PropagateEventToAllTransactions_HandlesEventPropagationWithManyTransactions(t *testing.T) {
	ctx := context.Background()

	transactions := make([]*transaction.CoordinatorTransaction, 10)
	for i := 0; i < 10; i++ {
		transactions[i], _ = transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	}

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(transactions...).
		Build(ctx)

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate event to all transactions")
}

func TestCoordinator_PropagateEventToAllTransactions_HandlesDifferentEventTypes(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(txn).
		Build(ctx)

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should handle HeartbeatIntervalEvent successfully")
}

func TestCoordinator_PropagateEventToAllTransactions_HandlesContextCancellationGracefully(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(txn).
		Build(ctx)

	// Create a cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Propagate event with cancelled context
	event := &common.HeartbeatIntervalEvent{}
	_ = c.propagateEventToAllTransactions(cancelledCtx, event)

	// Just verify it doesn't panic
}

func TestCoordinator_PropagateEventToAllTransactions_ProcessesTransactionsInMapIterationOrder(t *testing.T) {
	ctx := context.Background()

	// Create multiple transactions
	txns := make([]*transaction.CoordinatorTransaction, 5)
	for i := 0; i < 5; i++ {
		txns[i], _ = transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	}

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(txns...).
		Build(ctx)

	// Propagate event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should process all transactions regardless of order")
	assert.Equal(t, 5, len(c.transactionsByID), "all transactions should still be in map")
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsErrorImmediatelyWhenTransactionHandleEventFails(t *testing.T) {
	ctx := context.Background()
	txn1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	txn2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(
			txn1, txn2,
		).
		Build(ctx)

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	// With real transactions, HeartbeatIntervalEvent should be handled successfully
	assert.NoError(t, err, "heartbeat event should be handled successfully by all transaction states")
}

func TestCoordinator_PropagateEventToAllTransactions_IncrementsHeartbeatCounterForConfirmedTransaction(t *testing.T) {
	ctx := context.Background()
	// Create a transaction in State_Confirmed with 4 heartbeat intervals
	// (grace period is 5, so after one more heartbeat it should transition to State_Final)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).
		HeartbeatIntervalsSinceStateChange(4)
	txn, _ := txBuilder.Build(ctx)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "transaction should start in State_Confirmed")

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(txn).
		Build(ctx)

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)
	assert.NoError(t, err)

	// Transaction should have transitioned to State_Final (counter went from 4 to 5, which >= grace period of 5)
	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "transaction should have transitioned to State_Final after heartbeat")
}

func TestCoordinator_PropagateEventToAllTransactions_IncrementsHeartbeatCounterForRevertedTransaction(t *testing.T) {
	ctx := context.Background()

	// Create a transaction in State_Reverted with 4 heartbeat intervals
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Reverted).
		HeartbeatIntervalsSinceStateChange(4)
	txn, _ := txBuilder.Build(ctx)
	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "transaction should start in State_Reverted")

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(txn).
		Build(ctx)

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)
	assert.NoError(t, err)

	// Transaction should have transitioned to State_Final
	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "transaction should have transitioned to State_Final after heartbeat")
}
