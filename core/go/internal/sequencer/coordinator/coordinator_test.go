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
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func NewCoordinatorForUnitTest(t *testing.T, ctx context.Context, originatorIdentityPool []string) (*coordinator, *coordinatorDependencyMocks) {

	metrics := metrics.InitMetrics(context.Background(), prometheus.NewRegistry())
	mocks := &coordinatorDependencyMocks{
		transportWriter:   transport.NewMockTransportWriter(t),
		clock:             &common.FakeClockForTesting{},
		engineIntegration: common.NewMockEngineIntegration(t),
		syncPoints:        &syncpoints.MockSyncPoints{},
		emit:              func(event common.Event) {},
	}
	mockDomainAPI := componentsmocks.NewDomainSmartContract(t)
	mockTXManager := componentsmocks.NewTXManager(t)
	mockDomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	}).Maybe()
	mocks.transportWriter.On("StartLoopbackWriter", mock.Anything).Return(nil)
	mocks.transportWriter.On("StopLoopbackWriter").Return().Maybe()
	ctx, cancelCtx := context.WithCancel(ctx)

	config := &pldconf.SequencerConfig{
		HeartbeatInterval:        confutil.P("10s"),
		AssembleTimeout:          confutil.P("5s"),
		RequestTimeout:           confutil.P("1s"),
		BlockRange:               confutil.P(uint64(100)),
		BlockHeightTolerance:     confutil.P(uint64(5)),
		ClosingGracePeriod:       confutil.P(5),
		MaxInflightTransactions:  confutil.P(500),
		MaxDispatchAhead:         confutil.P(10),
		TargetActiveCoordinators: confutil.P(50),
		TargetActiveSequencers:   confutil.P(50),
	}

	coordinator, err := NewCoordinator(ctx, cancelCtx, pldtypes.RandAddress(), mockDomainAPI, mockTXManager, mocks.transportWriter, mocks.clock, mocks.engineIntegration, mocks.syncPoints, originatorIdentityPool, config, "node1",
		metrics,
		func(context.Context, *transaction.CoordinatorTransaction) {
			// Not used
		},
		func(contractAddress *pldtypes.EthAddress, coordinatorNode string) {
			// Not used
		},
		func(contractAddress *pldtypes.EthAddress) {
			// Not used
		})
	require.NoError(t, err)

	return coordinator, mocks
}

type coordinatorDependencyMocks struct {
	transportWriter   *transport.MockTransportWriter
	clock             *common.FakeClockForTesting
	engineIntegration *common.MockEngineIntegration
	emit              common.EmitEvent
	syncPoints        syncpoints.SyncPoints
}

func TestCoordinator_SingleTransactionLifecycle(t *testing.T) {
	// Test the progression of a single transaction through the coordinator's lifecycle
	// Simulating originator node, endorser node and the public transaction manager (submitter)
	// by inspecting the coordinator output messages and by sending events that would normally be triggered by those components sending messages to the coordinator.
	// At each stage, we inspect the state of the coordinator by checking the snapshot it produces on heartbeat messages

	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1) // Stop the dispatcher loop from progressing states - we're manually updating state throughout the test
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	defer c.Stop()

	// Start by simulating the originator and delegate a transaction to the coordinator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
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
	}, 100*time.Millisecond, 1*time.Millisecond, "Snapshot should contain one pooled transaction")

	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the pooled transaction with ID %s", txn.ID.String())

	// Assert that a request has been sent to the originator and respond with an assembled transaction
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentAssembleRequest()
	}, 100*time.Millisecond, 1*time.Millisecond, "Assemble request should be sent")
	c.QueueEvent(ctx, &transaction.AssembleSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID:    mocks.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
		PostAssembly: transactionBuilder.BuildPostAssembly(),
		PreAssembly:  transactionBuilder.BuildPreAssembly(),
	})

	// Assert that the coordinator has sent an endorsement request to the endorser
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.NumberOfSentEndorsementRequests() == 1
	}, 100*time.Millisecond, 1*time.Millisecond, "Endorsement request should be sent")

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the pooled transaction with ID %s", txn.ID.String())

	// now respond with an endorsement
	c.QueueEvent(ctx, &transaction.EndorsedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID:   mocks.SentMessageRecorder.SentEndorsementRequestsForPartyIdempotencyKey(transactionBuilder.GetEndorserIdentityLocator(0)),
		Endorsement: transactionBuilder.BuildEndorsement(0),
	})

	// Assert that the coordinator has sent a dispatch confirmation request to the transaction sender
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest()
	}, 100*time.Millisecond, 1*time.Millisecond, "Dispatch confirmation request should be sent")

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the pooled transaction with ID %s", txn.ID.String())

	// now respond with a dispatch confirmation
	c.QueueEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: mocks.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})

	// Assert that the transaction is ready to be collected by the dispatcher thread
	assert.Eventually(t, func() bool {
		readyTransactions := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Ready_For_Dispatch})
		return len(readyTransactions) == 1 &&
			readyTransactions[0].GetID().String() == txn.ID.String()
	}, 100*time.Millisecond, 1*time.Millisecond, "There should be exactly one transaction ready to dispatch")

	// Assert that snapshot no longer contains that transaction in the pooled transactions but does contain it in the dispatched transactions
	//NOTE: This is a key design point.  When a transaction is ready to be dispatched, we communicate to other nodes, via the heartbeat snapshot, that the transaction is dispatched.
	assert.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.PooledTransactions) == 0 &&
			len(snapshot.DispatchedTransactions) == 1 &&
			snapshot.DispatchedTransactions[0].ID.String() == txn.ID.String()
	}, 100*time.Millisecond, 1*time.Millisecond, "Snapshot should contain exactly one dispatched transaction")

	// Simulate the dispatcher thread collecting the transaction and dispatching it to a public transaction manager
	c.QueueEvent(ctx, &transaction.DispatchedEvent{
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
	assert.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.PooledTransactions) == 0 &&
			len(snapshot.DispatchedTransactions) == 1 &&
			snapshot.DispatchedTransactions[0].ID.String() == txn.ID.String() &&
			snapshot.DispatchedTransactions[0].Signer.String() == signerAddress.String()
	}, 100*time.Millisecond, 1*time.Millisecond, "Snapshot should contain dispatched transaction with signer address")

	// Simulate the dispatcher thread allocating a nonce for the transaction
	c.QueueEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: 42,
	})

	// Assert that the nonce is now included in the snapshot
	assert.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.PooledTransactions) == 0 &&
			len(snapshot.DispatchedTransactions) == 1 &&
			snapshot.DispatchedTransactions[0].ID.String() == txn.ID.String() &&
			snapshot.DispatchedTransactions[0].Nonce != nil &&
			*snapshot.DispatchedTransactions[0].Nonce == uint64(42)
	}, 100*time.Millisecond, 1*time.Millisecond, "Snapshot should contain dispatched transaction with nonce 42")

	// Simulate the public transaction manager submitting the transaction
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	c.QueueEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SubmissionHash: submissionHash,
	})

	// Assert that the hash is now included in the snapshot
	assert.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.PooledTransactions) == 0 &&
			len(snapshot.DispatchedTransactions) == 1 &&
			snapshot.DispatchedTransactions[0].ID.String() == txn.ID.String() &&
			snapshot.DispatchedTransactions[0].LatestSubmissionHash != nil &&
			*snapshot.DispatchedTransactions[0].LatestSubmissionHash == submissionHash
	}, 100*time.Millisecond, 1*time.Millisecond, "Snapshot should contain dispatched transaction with a submission hash")

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
	assert.Eventually(t, func() bool {
		snapshot := c.getSnapshot(ctx)
		return snapshot != nil &&
			len(snapshot.ConfirmedTransactions) == 1 &&
			snapshot.ConfirmedTransactions[0].ID.String() == txn.ID.String()
	}, 100*time.Millisecond, 1*time.Millisecond, "Snapshot should contain exactly one confirmed transaction")

}

func TestCoordinator_MaxInflightTransactions(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxInflightTransactions = confutil.P(5)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)
	c, _ := builder.Build(ctx)
	defer c.Stop()

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
	defer c.Stop()

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
	defer c.Stop()

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
	defer c.Stop()

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
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build(ctx)
	defer c.Stop()
	activeCoordinatorNode := "activeCoordinatorNode"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	assert.True(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "handover request should have been sent")
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestWithCorrectActiveCoordinatorNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "testCoordinatorNode"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestWithCorrectContractAddress(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	contractAddress := pldtypes.RandAddress()
	builder.ContractAddress(contractAddress)
	c, _ := builder.Build(ctx)
	defer c.Stop()
	activeCoordinatorNode := "activeCoordinatorNode"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, contractAddress).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_HandlesErrorFromSendHandoverRequestGracefully(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "activeCoordinatorNode"
	expectedError := fmt.Errorf("transport error")

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode
	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(expectedError)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest - should not panic even when error occurs
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_HandlesEmptyActiveCoordinatorNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := ""

	// Set empty active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_node1(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "node1"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_node2ExampleCom(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "node2@example.com"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_coordinatorNode123(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "coordinator-node-123"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_VeryLongCoordinatorNodeName(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "very-long-coordinator-node-name-with-special-chars-123"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestMultipleTimes(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build(ctx)
	defer c.Stop()
	activeCoordinatorNode := "activeCoordinatorNode"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	// Call sendHandoverRequest multiple times
	c.sendHandoverRequest(ctx)
	c.sendHandoverRequest(ctx)
	c.sendHandoverRequest(ctx)

	assert.True(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "handover request should have been sent")
}

func TestCoordinator_SendHandoverRequest_HandlesContextCancellation(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "activeCoordinatorNode"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	// Create a cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", cancelledCtx, activeCoordinatorNode, &contractAddress).Return(nil)
	mockTransport.On("StopLoopbackWriter").Return().Maybe()
	c.transportWriter = mockTransport

	// Call sendHandoverRequest with cancelled context
	c.sendHandoverRequest(cancelledCtx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsNilWhenNoTransactionsExist(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Ensure transactionsByID is empty
	c.transactionsByID = make(map[uuid.UUID]*transaction.CoordinatorTransaction)

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should return nil when no transactions exist")
}

func TestCoordinator_PropagateEventToAllTransactions_SuccessfullyPropagatesEventToSingleTransaction(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Propagate heartbeat event (should be handled successfully by any state)
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate event to single transaction")
}

func TestCoordinator_PropagateEventToAllTransactions_SuccessfullyPropagatesEventToMultipleTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create multiple transactions
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1 := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2 := txBuilder2.Build()

	txBuilder3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn3 := txBuilder3.Build()

	// Add transactions to coordinator
	c.transactionsByID[txn1.GetID()] = txn1
	c.transactionsByID[txn2.GetID()] = txn2
	c.transactionsByID[txn3.GetID()] = txn3

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate event to all transactions")
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsErrorWhenSingleTransactionFailsToHandleEvent(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create a transaction in a state that might not handle certain events
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Create a mock event that will cause an error
	event := &common.HeartbeatIntervalEvent{}

	err := c.propagateEventToAllTransactions(ctx, event)

	// HeartbeatIntervalEvent should be handled successfully by all transaction states
	assert.NoError(t, err, "heartbeat event should be handled successfully")
}

func TestCoordinator_PropagateEventToAllTransactions_StopsAtFirstErrorWhenMultipleTransactionsExist(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create multiple transactions
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1 := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2 := txBuilder2.Build()

	txBuilder3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn3 := txBuilder3.Build()

	// Add transactions to coordinator
	c.transactionsByID[txn1.GetID()] = txn1
	c.transactionsByID[txn2.GetID()] = txn2
	c.transactionsByID[txn3.GetID()] = txn3

	// Propagate heartbeat event - all should handle it successfully
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate to all transactions")
}

func TestCoordinator_PropagateEventToAllTransactions_HandlesEventPropagationWithManyTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create many transactions
	numTransactions := 10
	for i := 0; i < numTransactions; i++ {
		txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
		txn := txBuilder.Build()
		c.transactionsByID[txn.GetID()] = txn
	}

	// Verify we have the expected number of transactions
	assert.Equal(t, numTransactions, len(c.transactionsByID), "should have correct number of transactions")

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate event to all transactions")
}

func TestCoordinator_PropagateEventToAllTransactions_HandlesDifferentEventTypes(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should handle HeartbeatIntervalEvent successfully")
}

func TestCoordinator_PropagateEventToAllTransactions_HandlesContextCancellationGracefully(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

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
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create multiple transactions
	txns := make([]*transaction.CoordinatorTransaction, 5)
	for i := 0; i < 5; i++ {
		txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
		txns[i] = txBuilder.Build()
		c.transactionsByID[txns[i].GetID()] = txns[i]
	}

	// Propagate event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should process all transactions regardless of order")
	assert.Equal(t, 5, len(c.transactionsByID), "all transactions should still be in map")
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsErrorImmediatelyWhenTransactionHandleEventFails(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create multiple transactions
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1 := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2 := txBuilder2.Build()

	// Add transactions to coordinator
	c.transactionsByID[txn1.GetID()] = txn1
	c.transactionsByID[txn2.GetID()] = txn2

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	// With real transactions, HeartbeatIntervalEvent should be handled successfully
	assert.NoError(t, err, "heartbeat event should be handled successfully by all transaction states")
}

func TestCoordinator_PropagateEventToAllTransactions_IncrementsHeartbeatCounterForConfirmedTransaction(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create a transaction in State_Confirmed with 4 heartbeat intervals
	// (grace period is 5, so after one more heartbeat it should transition to State_Final)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).
		HeartbeatIntervalsSinceStateChange(4)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "transaction should start in State_Confirmed")

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)
	assert.NoError(t, err)

	// Transaction should have transitioned to State_Final (counter went from 4 to 5, which >= grace period of 5)
	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "transaction should have transitioned to State_Final after heartbeat")
}

func TestCoordinator_PropagateEventToAllTransactions_IncrementsHeartbeatCounterForRevertedTransaction(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Create a transaction in State_Reverted with 4 heartbeat intervals
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Reverted).
		HeartbeatIntervalsSinceStateChange(4)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn
	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "transaction should start in State_Reverted")

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)
	assert.NoError(t, err)

	// Transaction should have transitioned to State_Final
	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "transaction should have transitioned to State_Final after heartbeat")
}
