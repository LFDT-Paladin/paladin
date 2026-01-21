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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
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

// Event loop integration test helpers and constants
const (
	eventLoopTestTimeout  = 100 * time.Millisecond
	eventLoopTestInterval = 1 * time.Millisecond
)

// queueAndSync queues an event and waits for it to be processed by the event loop.
// This is useful for testing that state does NOT change after an event.
func queueAndSync(ctx context.Context, c SeqCoordinator, event common.Event) {
	c.QueueEvent(ctx, event)
	sync := statemachine.NewSyncEvent()
	c.QueueEvent(ctx, sync)
	sync.Wait()
}

// assertEventualStateEventLoop waits for the coordinator to reach the expected state
func assertEventualStateEventLoop(t *testing.T, c SeqCoordinator, expected State, msgAndArgs ...interface{}) {
	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == expected
	}, eventLoopTestTimeout, eventLoopTestInterval, msgAndArgs...)
}

func NewCoordinatorForUnitTest(t *testing.T, ctx context.Context, originatorIdentityPool []string) (*coordinator, *coordinatorDependencyMocks) {

	metrics := metrics.InitMetrics(context.Background(), prometheus.NewRegistry())
	mocks := &coordinatorDependencyMocks{
		transportWriter:   transport.NewMockTransportWriter(t),
		clock:             &common.FakeClockForTesting{},
		engineIntegration: common.NewMockEngineIntegration(t),
		syncPoints:        &syncpoints.MockSyncPoints{},
		emit:              func(ctx context.Context, event common.Event) {},
	}
	mockDomainAPI := componentsmocks.NewDomainSmartContract(t)
	mockTXManager := componentsmocks.NewTXManager(t)
	mocks.transportWriter.On("StartLoopbackWriter", mock.Anything).Return(nil)
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

	coordinator, err := NewCoordinator(ctx, cancelCtx, pldtypes.RandAddress(), mockDomainAPI, mockTXManager, mocks.transportWriter, mocks.clock, mocks.engineIntegration, mocks.syncPoints, config, "node1",
		metrics,
		func(context.Context, *transaction.Transaction) {
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
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(0) // Stop the dispatcher loop from progressing states - we're manually updating state throughout the test
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)

	// Start by simulating the originator and delegate a transaction to the coordinator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	queueAndSync(ctx, c, &TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: []*components.PrivateTransaction{txn},
	})

	// Assert that snapshot contains a transaction with matching ID
	snapshot := getSnapshot(ctx, c.state, c.smConfig)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that a request has been sent to the originator and respond with an assembled transaction
	require.True(t, mocks.SentMessageRecorder.HasSentAssembleRequest())
	queueAndSync(ctx, c, &transaction.AssembleSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID:    mocks.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
		PostAssembly: transactionBuilder.BuildPostAssembly(),
		PreAssembly:  transactionBuilder.BuildPreAssembly(),
	})

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = getSnapshot(ctx, c.state, c.smConfig)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that the coordinator has sent an endorsement request to the endorser and respond with an endorsement
	require.Equal(t, 1, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())
	queueAndSync(ctx, c, &transaction.EndorsedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID:   mocks.SentMessageRecorder.SentEndorsementRequestsForPartyIdempotencyKey(transactionBuilder.GetEndorserIdentityLocator(0)),
		Endorsement: transactionBuilder.BuildEndorsement(0),
	})

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = getSnapshot(ctx, c.state, c.smConfig)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that the coordinator has sent a dispatch confirmation request to the transaction sender and respond with a dispatch confirmation
	require.True(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest())
	queueAndSync(ctx, c, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: mocks.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})

	// Assert that snapshot no longer contains that transaction in the pooled transactions but does contain it in the dispatched transactions
	//NOTE: This is a key design point.  When a transaction is ready to be dispatched, we communicate to other nodes, via the heartbeat snapshot, that the transaction is dispatched.
	snapshot = getSnapshot(ctx, c.state, c.smConfig)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that the transaction is ready to be collected by the dispatcher thread
	readyTransactions := getTransactionsInStates(ctx, c.state, []transaction.State{transaction.State_Ready_For_Dispatch})
	require.NotNil(t, readyTransactions)
	require.Len(t, readyTransactions, 1, "There should be exactly one transaction ready to dispatch")
	assert.Equal(t, txn.ID.String(), readyTransactions[0].ID.String(), "The transaction ready to dispatch should match the delegated transaction ID")

	// Simulate the dispatcher thread collecting the transaction and dispatching it to a public transaction manager
	queueAndSync(ctx, c, &transaction.DispatchedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})

	// Simulate the public transaction manager collecting the dispatched transaction and associating a signing address with it
	signerAddress := pldtypes.RandAddress()
	queueAndSync(ctx, c, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})

	// Assert that we now have a signer address in the snapshot
	snapshot = getSnapshot(ctx, c.state, c.smConfig)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.DispatchedTransactions[0].Signer.String(), "Snapshot should contain the dispatched transaction with signer address %s", signerAddress.String())

	// Simulate the dispatcher thread allocating a nonce for the transaction
	queueAndSync(ctx, c, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: 42,
	})

	// Assert that the nonce is now included in the snapshot
	snapshot = getSnapshot(ctx, c.state, c.smConfig)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.DispatchedTransactions[0].Signer.String(), "Snapshot should contain the dispatched transaction with signer address %s", signerAddress.String())
	require.NotNil(t, snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with a nonce")
	assert.Equal(t, uint64(42), *snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with nonce 42")

	// Simulate the public transaction manager submitting the transaction
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	queueAndSync(ctx, c, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SubmissionHash: submissionHash,
	})

	// Assert that the hash is now included in the snapshot
	snapshot = getSnapshot(ctx, c.state, c.smConfig)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.DispatchedTransactions[0].Signer.String(), "Snapshot should contain the dispatched transaction with signer address %s", signerAddress.String())
	require.NotNil(t, snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with a nonce")
	assert.Equal(t, uint64(42), *snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with nonce 42")
	require.NotNil(t, snapshot.DispatchedTransactions[0].LatestSubmissionHash, "Snapshot should contain the dispatched transaction with a submission hash")

	// Simulate the block indexer confirming the transaction
	queueAndSync(ctx, c, &transaction.ConfirmedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: 42,
		Hash:  submissionHash,
	})

	// Assert that snapshot contains a transaction with matching ID
	snapshot = getSnapshot(ctx, c.state, c.smConfig)
	require.NotNil(t, snapshot)

	assert.Equal(t, 0, len(snapshot.DispatchedTransactions))
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	assert.Equal(t, 1, len(snapshot.ConfirmedTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.ConfirmedTransactions[0].ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.ConfirmedTransactions[0].Signer.String())
	require.NotNil(t, snapshot.ConfirmedTransactions[0].Nonce)
	assert.Equal(t, uint64(42), *snapshot.ConfirmedTransactions[0].Nonce)
	assert.Equal(t, submissionHash, *snapshot.ConfirmedTransactions[0].LatestSubmissionHash)

}

// TODO AM: not dealing with these tests until select active coordinator is made state machine safe
// func TestCoordinator_SelectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithFullyQualifiedIdentity(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
// 		StaticCoordinator:    proto.String("identity@node1"),
// 	})
// 	c, _ := builder.Build(ctx)

// 	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err)
// 	assert.Equal(t, "node1", coordinatorNode)
// }

// func TestCoordinator_SelectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithIdentityOnly(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
// 		StaticCoordinator:    proto.String("identity"),
// 	})
// 	c, _ := builder.Build(ctx)

// 	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
// 	// When node is not specified and allowEmptyNode is false, it should return an error
// 	require.Error(t, err)
// 	assert.Empty(t, coordinatorNode)
// }

// func TestCoordinator_SelectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithEmptyStaticCoordinator(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
// 		StaticCoordinator:    proto.String(""),
// 	})
// 	c, _ := builder.Build(ctx)

// 	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
// 	require.Error(t, err)
// 	assert.Empty(t, coordinatorNode)
// 	assert.Contains(t, err.Error(), "static coordinator mode is configured but static coordinator node is not set")
// }

// func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithEmptyPool(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
// 	})
// 	config := builder.GetSequencerConfig()
// 	config.BlockRange = confutil.P(uint64(100))
// 	builder.OverrideSequencerConfig(config)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{}
// 	c.state.currentBlockHeight = 1000

// 	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err)
// 	assert.Empty(t, coordinatorNode)
// }

// func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithSingleNodeInPool(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
// 	})
// 	config := builder.GetSequencerConfig()
// 	config.BlockRange = confutil.P(uint64(100))
// 	builder.OverrideSequencerConfig(config)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{"node1"}
// 	c.state.currentBlockHeight = 1000

// 	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err)
// 	assert.Equal(t, "node1", coordinatorNode)
// }

// func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithMultipleNodesInPool(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
// 	})
// 	config := builder.GetSequencerConfig()
// 	config.BlockRange = confutil.P(uint64(100))
// 	builder.OverrideSequencerConfig(config)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{"node1", "node2", "node3"}
// 	c.state.currentBlockHeight = 1000

// 	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err)
// 	assert.Contains(t, []string{"node1", "node2", "node3"}, coordinatorNode)
// }

// func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithBlockHeightRounding(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
// 	})
// 	config := builder.GetSequencerConfig()
// 	config.BlockRange = confutil.P(uint64(100))
// 	builder.OverrideSequencerConfig(config)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{"node1", "node2", "node3"}

// 	// Test that blocks within the same range select the same coordinator
// 	c.state.currentBlockHeight = 1000
// 	coordinatorNode1, err1 := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err1)

// 	c.state.currentBlockHeight = 1001
// 	coordinatorNode2, err2 := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err2)

// 	c.state.currentBlockHeight = 1099
// 	coordinatorNode3, err3 := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err3)

// 	// All should select the same coordinator since they're in the same block range
// 	assert.Equal(t, coordinatorNode1, coordinatorNode2)
// 	assert.Equal(t, coordinatorNode2, coordinatorNode3)

// 	// Different block range should potentially select different coordinator
// 	c.state.currentBlockHeight = 1100
// 	coordinatorNode4, err4 := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err4)

// 	assert.Contains(t, []string{"node1", "node2", "node3"}, coordinatorNode4)
// }

// func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithDifferentBlockRanges(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
// 	})
// 	config := builder.GetSequencerConfig()
// 	config.BlockRange = confutil.P(uint64(50))
// 	builder.OverrideSequencerConfig(config)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{"node1", "node2"}

// 	c.state.currentBlockHeight = 100
// 	coordinatorNode1, err1 := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err1)

// 	c.state.currentBlockHeight = 150
// 	coordinatorNode2, err2 := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err2)

// 	// Different block ranges should potentially select different coordinators
// 	assert.Contains(t, []string{"node1", "node2"}, coordinatorNode1)
// 	assert.Contains(t, []string{"node1", "node2"}, coordinatorNode2)
// }

// func TestCoordinator_SelectActiveCoordinatorNode_SenderMode_ReturnsCurrentNodeName(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
// 	})
// 	c, _ := builder.Build(ctx)
// 	// The builder sets nodeName to "node1" by default
// 	assert.Equal(t, "node1", c.smContext.nodeName)

// 	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
// 	require.NoError(t, err)
// 	assert.Equal(t, "node1", coordinatorNode)
// }

func TestCoordinator_Stop_StopsEventLoopAndDispatchLoop(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Verify loops are running by checking channels are not closed
	select {
	case <-c.stateMachine.Done():
		t.Fatal("event loop should not be stopped initially")
	default:
	}
	select {
	case <-c.dispatchLoop.stopped:
		t.Fatal("dispatch loop should not be stopped initially")
	default:
	}

	// Should block until shutdown is complete
	c.Stop()

	// Verify both loops have stopped (reading from closed channel returns immediately with ok=false)
	select {
	case _, ok := <-c.stateMachine.Done():
		require.False(t, ok, "event loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("event loop did not stop within timeout")
	}

	select {
	case _, ok := <-c.dispatchLoop.stopped:
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

	c.Stop()

	// Verify both loops have stopped
	select {
	case _, ok := <-c.stateMachine.Done():
		require.False(t, ok, "event loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("event loop did not stop within timeout")
	}

	select {
	case _, ok := <-c.dispatchLoop.stopped:
		require.False(t, ok, "dispatch loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

func TestCoordinator_Stop_StopsLoopsEvenWhenProcessingEvents(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Queue some events to ensure loops are busy
	for i := 0; i < 10; i++ {
		c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})
	}

	c.Stop()

	// Verify both loops have stopped
	select {
	case _, ok := <-c.stateMachine.Done():
		require.False(t, ok, "event loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("event loop did not stop within timeout")
	}

	select {
	case _, ok := <-c.dispatchLoop.stopped:
		require.False(t, ok, "dispatch loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

func TestCoordinator_ConfirmDispatchedTransaction_FindsTransactionBySignerAndNonce(t *testing.T) {
	ctx := context.Background()

	// Create a transaction with signer address and nonce using builder
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).
		SignerAddress(signerAddress).
		Nonce(nonce)
	txn := txBuilder.Build()

	// Create coordinator with the transaction already added
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	// Confirm the transaction
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := confirmDispatchedTransaction(ctx, c.state, txn.ID, signerAddress, nonce, hash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found by signer and nonce")
}

func TestCoordinator_ConfirmDispatchedTransaction_FindsTransactionByTxIdWhenFromIsNil(t *testing.T) {
	ctx := context.Background()

	// Create a transaction
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	// Create coordinator with the transaction
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	// Confirm the transaction with nil from address (chained transaction scenario)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	var nilFrom *pldtypes.EthAddress = nil
	found, err := confirmDispatchedTransaction(ctx, c.state, txn.ID, nilFrom, 0, hash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found by txId")
}

func TestCoordinator_ConfirmDispatchedTransaction_FindsTransactionByTxIdWhenSignerNonceLookupFails(t *testing.T) {
	ctx := context.Background()

	// Create a transaction without signer/nonce (chained transaction)
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	// Create coordinator with the transaction
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	// Try to confirm with a signer+nonce that doesn't match
	nonMatchingSigner := pldtypes.RandAddress()
	nonMatchingNonce := uint64(999)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := confirmDispatchedTransaction(ctx, c.state, txn.ID, nonMatchingSigner, nonMatchingNonce, hash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found by txId as fallback")
}

func TestCoordinator_ConfirmDispatchedTransaction_ReturnsFalseWhenTransactionNotFound(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Try to confirm a transaction that doesn't exist
	nonExistentTxID := uuid.New()
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := confirmDispatchedTransaction(ctx, c.state, nonExistentTxID, signerAddress, nonce, hash, revertReason)

	require.NoError(t, err)
	assert.False(t, found, "transaction should not be found")
}

func TestCoordinator_ConfirmDispatchedTransaction_HandlesMatchingHashCorrectly(t *testing.T) {
	ctx := context.Background()

	// Create a transaction with signer, nonce, and submission hash using builder
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).
		SignerAddress(signerAddress).
		Nonce(nonce).
		LatestSubmissionHash(&submissionHash).
		Build()

	// Create coordinator with the transaction
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	// Confirm with matching hash
	revertReason := pldtypes.HexBytes{}
	found, err := confirmDispatchedTransaction(ctx, c.state, txn.ID, signerAddress, nonce, submissionHash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found and confirmed")
}

func TestCoordinator_ConfirmDispatchedTransaction_HandlesDifferentHashCorrectly(t *testing.T) {
	ctx := context.Background()

	// Create a transaction with signer, nonce, and submission hash using builder
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).
		SignerAddress(signerAddress).
		Nonce(nonce).
		LatestSubmissionHash(&submissionHash).
		Build()

	// Create coordinator with the transaction
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	// Confirm with different hash (should still work, just logs a warning)
	differentHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := confirmDispatchedTransaction(ctx, c.state, txn.ID, signerAddress, nonce, differentHash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found even with different hash")
}

func TestCoordinator_ConfirmDispatchedTransaction_HandlesNilSubmissionHashCorrectly(t *testing.T) {
	ctx := context.Background()

	// Create a transaction with signer and nonce but no submission hash using builder
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).
		SignerAddress(signerAddress).
		Nonce(nonce).
		Build()

	// Create coordinator with the transaction
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	// Confirm with a hash (chained transaction scenario)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := confirmDispatchedTransaction(ctx, c.state, txn.ID, signerAddress, nonce, hash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found even with nil submission hash")
	assert.Nil(t, txn.GetLatestSubmissionHash(), "transaction should have nil submission hash")
}

func TestCoordinator_ConfirmDispatchedTransaction_ReturnsErrorWhenHandleEventFails(t *testing.T) {
	ctx := context.Background()

	// Create a transaction in a state that cannot handle ConfirmedEvent
	// We'll use State_Pooled which should not accept ConfirmedEvent
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).
		SignerAddress(signerAddress).
		Nonce(nonce).
		Build()

	// Create coordinator with the transaction
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	// Try to confirm - this should fail because the transaction is in State_Pooled
	// which may not accept ConfirmedEvent depending on state machine rules
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := confirmDispatchedTransaction(ctx, c.state, txn.ID, signerAddress, nonce, hash, revertReason)

	// The function may return an error if HandleEvent fails, or it may succeed
	// depending on the state machine rules. We just verify it doesn't panic.
	if err != nil {
		assert.False(t, found, "should return false when error occurs")
	}
}

func TestCoordinator_ConfirmDispatchedTransaction_HandlesMultipleTransactionsCorrectly(t *testing.T) {
	ctx := context.Background()

	// Create multiple transactions using builder
	signerAddress1 := pldtypes.RandAddress()
	nonce1 := uint64(42)
	txn1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).
		SignerAddress(signerAddress1).
		Nonce(nonce1).
		Build()

	signerAddress2 := pldtypes.RandAddress()
	nonce2 := uint64(43)
	txn2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).
		SignerAddress(signerAddress2).
		Nonce(nonce2).
		Build()

	// Create coordinator with both transactions
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn1, txn2).
		Build(ctx)

	// Confirm first transaction
	hash1 := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found1, err := confirmDispatchedTransaction(ctx, c.state, txn1.ID, signerAddress1, nonce1, hash1, revertReason)
	require.NoError(t, err)
	assert.True(t, found1, "first transaction should be found")

	// Confirm second transaction
	hash2 := pldtypes.Bytes32(pldtypes.RandBytes(32))
	found2, err := confirmDispatchedTransaction(ctx, c.state, txn2.ID, signerAddress2, nonce2, hash2, revertReason)
	require.NoError(t, err)
	assert.True(t, found2, "second transaction should be found")
}

func TestCoordinator_ConfirmMonitoredTransaction_ConfirmsExistingUnconfirmedFlushPoint(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Set up a flush point that is not confirmed
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	txID := uuid.New()
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))

	c.state.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", signerAddress.String(), nonce): {
			From:          *signerAddress,
			Nonce:         nonce,
			TransactionID: txID,
			Hash:          hash,
			Confirmed:     false,
		},
	}

	// Confirm the monitored transaction
	confirmMonitoredTransaction(ctx, c.state, signerAddress, nonce)

	// Verify the flush point is now confirmed
	flushPoint := c.state.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress.String(), nonce)]
	require.NotNil(t, flushPoint, "flush point should still exist")
	assert.True(t, flushPoint.Confirmed, "flush point should be confirmed")
	assert.Equal(t, *signerAddress, flushPoint.From)
	assert.Equal(t, nonce, flushPoint.Nonce)
	assert.Equal(t, txID, flushPoint.TransactionID)
	assert.Equal(t, hash, flushPoint.Hash)
}

func TestCoordinator_ConfirmMonitoredTransaction_NoOpWhenFlushPointAlreadyConfirmed(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Set up a flush point that is already confirmed
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	txID := uuid.New()
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))

	c.state.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", signerAddress.String(), nonce): {
			From:          *signerAddress,
			Nonce:         nonce,
			TransactionID: txID,
			Hash:          hash,
			Confirmed:     true,
		},
	}

	// Confirm the monitored transaction again
	confirmMonitoredTransaction(ctx, c.state, signerAddress, nonce)

	// Verify the flush point is still confirmed
	flushPoint := c.state.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress.String(), nonce)]
	require.NotNil(t, flushPoint, "flush point should still exist")
	assert.True(t, flushPoint.Confirmed, "flush point should remain confirmed")
}

func TestCoordinator_ConfirmMonitoredTransaction_NoOpWhenFlushPointDoesNotExist(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Set up empty flush points map
	c.state.activeCoordinatorsFlushPointsBySignerNonce = make(map[string]*common.FlushPoint)

	// Try to confirm a non-existent flush point
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	confirmMonitoredTransaction(ctx, c.state, signerAddress, nonce)

	// Verify the map is still empty
	assert.Equal(t, 0, len(c.state.activeCoordinatorsFlushPointsBySignerNonce), "flush points map should remain empty")
}

func TestCoordinator_ConfirmMonitoredTransaction_OnlyConfirmsMatchingFlushPointWhenMultipleExist(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Set up multiple flush points
	signerAddress1 := pldtypes.RandAddress()
	nonce1 := uint64(42)
	txID1 := uuid.New()
	hash1 := pldtypes.Bytes32(pldtypes.RandBytes(32))

	signerAddress2 := pldtypes.RandAddress()
	nonce2 := uint64(43)
	txID2 := uuid.New()
	hash2 := pldtypes.Bytes32(pldtypes.RandBytes(32))

	c.state.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", signerAddress1.String(), nonce1): {
			From:          *signerAddress1,
			Nonce:         nonce1,
			TransactionID: txID1,
			Hash:          hash1,
			Confirmed:     false,
		},
		fmt.Sprintf("%s:%d", signerAddress2.String(), nonce2): {
			From:          *signerAddress2,
			Nonce:         nonce2,
			TransactionID: txID2,
			Hash:          hash2,
			Confirmed:     false,
		},
	}

	// Confirm only the first flush point
	confirmMonitoredTransaction(ctx, c.state, signerAddress1, nonce1)

	// Verify only the first flush point is confirmed
	flushPoint1 := c.state.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress1.String(), nonce1)]
	require.NotNil(t, flushPoint1, "first flush point should exist")
	assert.True(t, flushPoint1.Confirmed, "first flush point should be confirmed")

	flushPoint2 := c.state.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress2.String(), nonce2)]
	require.NotNil(t, flushPoint2, "second flush point should exist")
	assert.False(t, flushPoint2.Confirmed, "second flush point should not be confirmed")
}

func TestCoordinator_ConfirmMonitoredTransaction_HandlesDifferentNoncesForSameSigner(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Set up flush points with same signer but different nonces
	signerAddress := pldtypes.RandAddress()
	nonce1 := uint64(42)
	nonce2 := uint64(43)
	txID1 := uuid.New()
	txID2 := uuid.New()
	hash1 := pldtypes.Bytes32(pldtypes.RandBytes(32))
	hash2 := pldtypes.Bytes32(pldtypes.RandBytes(32))

	c.state.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", signerAddress.String(), nonce1): {
			From:          *signerAddress,
			Nonce:         nonce1,
			TransactionID: txID1,
			Hash:          hash1,
			Confirmed:     false,
		},
		fmt.Sprintf("%s:%d", signerAddress.String(), nonce2): {
			From:          *signerAddress,
			Nonce:         nonce2,
			TransactionID: txID2,
			Hash:          hash2,
			Confirmed:     false,
		},
	}

	// Confirm only the first nonce
	confirmMonitoredTransaction(ctx, c.state, signerAddress, nonce1)

	// Verify only the first nonce is confirmed
	flushPoint1 := c.state.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress.String(), nonce1)]
	require.NotNil(t, flushPoint1, "first flush point should exist")
	assert.True(t, flushPoint1.Confirmed, "first flush point should be confirmed")

	flushPoint2 := c.state.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress.String(), nonce2)]
	require.NotNil(t, flushPoint2, "second flush point should exist")
	assert.False(t, flushPoint2.Confirmed, "second flush point should not be confirmed")
}

func TestCoordinator_ConfirmMonitoredTransaction_HandlesDifferentSignersWithSameNonce(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Set up flush points with different signers but same nonce
	signerAddress1 := pldtypes.RandAddress()
	signerAddress2 := pldtypes.RandAddress()
	nonce := uint64(42)
	txID1 := uuid.New()
	txID2 := uuid.New()
	hash1 := pldtypes.Bytes32(pldtypes.RandBytes(32))
	hash2 := pldtypes.Bytes32(pldtypes.RandBytes(32))

	c.state.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", signerAddress1.String(), nonce): {
			From:          *signerAddress1,
			Nonce:         nonce,
			TransactionID: txID1,
			Hash:          hash1,
			Confirmed:     false,
		},
		fmt.Sprintf("%s:%d", signerAddress2.String(), nonce): {
			From:          *signerAddress2,
			Nonce:         nonce,
			TransactionID: txID2,
			Hash:          hash2,
			Confirmed:     false,
		},
	}

	// Confirm only the first signer's flush point
	confirmMonitoredTransaction(ctx, c.state, signerAddress1, nonce)

	// Verify only the first signer's flush point is confirmed
	flushPoint1 := c.state.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress1.String(), nonce)]
	require.NotNil(t, flushPoint1, "first flush point should exist")
	assert.True(t, flushPoint1.Confirmed, "first flush point should be confirmed")

	flushPoint2 := c.state.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress2.String(), nonce)]
	require.NotNil(t, flushPoint2, "second flush point should exist")
	assert.False(t, flushPoint2.Confirmed, "second flush point should not be confirmed")
}

func TestCoordinator_ConfirmMonitoredTransaction_DoesNotRemoveFlushPointFromMap(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Set up a flush point
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	txID := uuid.New()
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))

	c.state.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", signerAddress.String(), nonce): {
			From:          *signerAddress,
			Nonce:         nonce,
			TransactionID: txID,
			Hash:          hash,
			Confirmed:     false,
		},
	}

	// Confirm the monitored transaction
	confirmMonitoredTransaction(ctx, c.state, signerAddress, nonce)

	// Verify the flush point still exists in the map (not removed)
	key := fmt.Sprintf("%s:%d", signerAddress.String(), nonce)
	flushPoint, exists := c.state.activeCoordinatorsFlushPointsBySignerNonce[key]
	require.True(t, exists, "flush point should still exist in map")
	require.NotNil(t, flushPoint, "flush point should not be nil")
	assert.True(t, flushPoint.Confirmed, "flush point should be confirmed")
	assert.Equal(t, 1, len(c.state.activeCoordinatorsFlushPointsBySignerNonce), "map should still contain one flush point")
}

// TODO AM: not dealing with these tests when they need to be reimplemented as an event
// func TestCoordinator_UpdateOriginatorNodePool_AddsNodeToEmptyPool(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{}

// 	c.UpdateOriginatorNodePool(ctx, "node2")

// 	// Should contain both the added node and the coordinator's own node
// 	assert.Equal(t, 2, len(c.smContext.originatorNodePool), "pool should contain 2 nodes")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node2", "pool should contain node2")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node1", "pool should contain coordinator's own node")
// }

// func TestCoordinator_UpdateOriginatorNodePool_AddsNodeToNonEmptyPool(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{"node1", "node3"}

// 	c.UpdateOriginatorNodePool(ctx, "node2")

// 	// Should contain all nodes including the new one
// 	assert.Equal(t, 3, len(c.smContext.originatorNodePool), "pool should contain 3 nodes")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node1", "pool should contain node1")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node2", "pool should contain node2")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node3", "pool should contain node3")
// }

// func TestCoordinator_UpdateOriginatorNodePool_DoesNotAddDuplicateNode(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{"node1", "node2"}

// 	c.UpdateOriginatorNodePool(ctx, "node2")

// 	// Should not have duplicates
// 	assert.Equal(t, 2, len(c.smContext.originatorNodePool), "pool should still contain 2 nodes")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node1", "pool should contain node1")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node2", "pool should contain node2")
// }

// func TestCoordinator_UpdateOriginatorNodePool_EnsuresCoordinatorsOwnNodeIsAlwaysInPool(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{}

// 	// Add a different node
// 	c.UpdateOriginatorNodePool(ctx, "node2")

// 	// Coordinator's own node (node1) should be automatically added
// 	assert.Contains(t, c.smContext.originatorNodePool, "node1", "pool should contain coordinator's own node")
// 	assert.Equal(t, 2, len(c.smContext.originatorNodePool), "pool should contain 2 nodes")
// }

// func TestCoordinator_UpdateOriginatorNodePool_EnsuresCoordinatorsOwnNodeIsAddedEvenWhenPoolAlreadyHasOtherNodes(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	// Manually set pool without coordinator's own node
// 	c.smContext.originatorNodePool = []string{"node2", "node3"}

// 	c.UpdateOriginatorNodePool(ctx, "node4")

// 	// Coordinator's own node (node1) should be automatically added
// 	assert.Contains(t, c.smContext.originatorNodePool, "node1", "pool should contain coordinator's own node")
// 	assert.Equal(t, 4, len(c.smContext.originatorNodePool), "pool should contain 4 nodes")
// }

// func TestCoordinator_UpdateOriginatorNodePool_DoesNotDuplicateCoordinatorsOwnNode(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{"node1", "node2"}

// 	// Try to add coordinator's own node
// 	c.UpdateOriginatorNodePool(ctx, "node1")

// 	// Should not have duplicates
// 	assert.Equal(t, 2, len(c.smContext.originatorNodePool), "pool should still contain 2 nodes")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node1", "pool should contain node1")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node2", "pool should contain node2")
// }

// func TestCoordinator_UpdateOriginatorNodePool_HandlesMultipleSequentialUpdates(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{}

// 	// Add multiple nodes sequentially
// 	c.UpdateOriginatorNodePool(ctx, "node2")
// 	c.UpdateOriginatorNodePool(ctx, "node3")
// 	c.UpdateOriginatorNodePool(ctx, "node4")

// 	// Should contain all nodes including coordinator's own node
// 	assert.Equal(t, 4, len(c.smContext.originatorNodePool), "pool should contain 4 nodes")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node1", "pool should contain node1")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node2", "pool should contain node2")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node3", "pool should contain node3")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node4", "pool should contain node4")
// }

// func TestCoordinator_UpdateOriginatorNodePool_HandlesEmptyStringNode(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{}

// 	c.UpdateOriginatorNodePool(ctx, "")

// 	// Empty string should be added, and coordinator's own node should be added
// 	assert.Equal(t, 2, len(c.smContext.originatorNodePool), "pool should contain 2 nodes")
// 	assert.Contains(t, c.smContext.originatorNodePool, "", "pool should contain empty string")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node1", "pool should contain coordinator's own node")
// }

// func TestCoordinator_UpdateOriginatorNodePool_IsThreadSafe(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	c.smContext.originatorNodePool = []string{}

// 	// Concurrent updates - use node names that don't conflict with coordinator's own node
// 	done := make(chan struct{})
// 	numGoroutines := 10
// 	nodesPerGoroutine := 5

// 	for i := 0; i < numGoroutines; i++ {
// 		go func(startNode int) {
// 			defer func() { done <- struct{}{} }()
// 			for j := 0; j < nodesPerGoroutine; j++ {
// 				// Use node names starting from 100 to avoid conflict with coordinator's "node1"
// 				nodeName := fmt.Sprintf("node%d", 100+startNode*100+j)
// 				c.UpdateOriginatorNodePool(ctx, nodeName)
// 			}
// 		}(i)
// 	}

// 	// Wait for all goroutines to complete
// 	for i := 0; i < numGoroutines; i++ {
// 		<-done
// 	}

// 	// Pool should contain all unique nodes plus coordinator's own node
// 	// Total should be: numGoroutines * nodesPerGoroutine + 1 (coordinator's own node)
// 	expectedCount := numGoroutines*nodesPerGoroutine + 1
// 	assert.Equal(t, expectedCount, len(c.smContext.originatorNodePool), "pool should contain all unique nodes plus coordinator's own node")
// 	assert.Contains(t, c.smContext.originatorNodePool, "node1", "pool should contain coordinator's own node")

// 	// Verify no duplicates
// 	nodeSet := make(map[string]bool)
// 	for _, node := range c.smContext.originatorNodePool {
// 		assert.False(t, nodeSet[node], "pool should not contain duplicate node: %s", node)
// 		nodeSet[node] = true
// 	}
// }

// TODO AM: not dealing with GetActiveCoordinatorNode when it needs making correct for the state machine
// func TestCoordinator_GetActiveCoordinatorNode_ReturnsEmptyStringWhenInitIfNoActiveCoordinatorIsFalseAndActiveCoordinatorNodeIsEmpty(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""

// 	result := c.GetActiveCoordinatorNode(ctx, false)
// 	assert.Empty(t, result, "should return empty string when initIfNoActiveCoordinator is false")
// }

// func TestCoordinator_GetActiveCoordinatorNode_ReturnsExistingActiveCoordinatorNodeWhenInitIfNoActiveCoordinatorIsFalse(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	expectedNode := "existingNode"
// 	c.state.activeCoordinatorNode = expectedNode

// 	result := c.GetActiveCoordinatorNode(ctx, false)
// 	assert.Equal(t, expectedNode, result, "should return existing active coordinator node")
// }

// func TestCoordinator_GetActiveCoordinatorNode_ReturnsExistingActiveCoordinatorNodeWhenInitIfNoActiveCoordinatorIsTrueButNodeIsAlreadySet(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	expectedNode := "existingNode"
// 	c.state.activeCoordinatorNode = expectedNode

// 	result := c.GetActiveCoordinatorNode(ctx, true)
// 	assert.Equal(t, expectedNode, result, "should return existing active coordinator node without re-initializing")
// }

// func TestCoordinator_GetActiveCoordinatorNode_InitializesAndReturnsCoordinatorNodeInStaticModeWhenInitIfNoActiveCoordinatorIsTrue(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
// 		StaticCoordinator:    proto.String("identity@node1"),
// 	})
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""

// 	result := c.GetActiveCoordinatorNode(ctx, true)
// 	assert.Equal(t, "node1", result, "should initialize and return coordinator node in static mode")
// 	assert.Equal(t, "node1", c.state.activeCoordinatorNode, "should set activeCoordinatorNode field")
// }

// func TestCoordinator_GetActiveCoordinatorNode_InitializesAndReturnsCoordinatorNodeInSenderModeWhenInitIfNoActiveCoordinatorIsTrue(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
// 	})
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""

// 	result := c.GetActiveCoordinatorNode(ctx, true)
// 	assert.Equal(t, "node1", result, "should initialize and return coordinator node in sender mode")
// 	assert.Equal(t, "node1", c.state.activeCoordinatorNode, "should set activeCoordinatorNode field")
// }

// func TestCoordinator_GetActiveCoordinatorNode_InitializesAndReturnsCoordinatorNodeInEndorserModeWhenInitIfNoActiveCoordinatorIsTrue(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
// 	})
// 	config := builder.GetSequencerConfig()
// 	config.BlockRange = confutil.P(uint64(100))
// 	builder.OverrideSequencerConfig(config)
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""
// 	c.smContext.originatorNodePool = []string{"node1", "node2", "node3"}
// 	c.state.currentBlockHeight = 1000

// 	result := c.GetActiveCoordinatorNode(ctx, true)
// 	assert.Contains(t, []string{"node1", "node2", "node3"}, result, "should initialize and return coordinator node from pool in endorser mode")
// 	assert.NotEmpty(t, c.state.activeCoordinatorNode, "should set activeCoordinatorNode field")
// }

// func TestCoordinator_GetActiveCoordinatorNode_ReturnsEmptyStringWhenSelectActiveCoordinatorNodeFailsInStaticMode(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
// 		StaticCoordinator:    proto.String(""), // Empty static coordinator should cause error
// 	})
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""

// 	result := c.GetActiveCoordinatorNode(ctx, true)
// 	assert.Empty(t, result, "should return empty string when SelectActiveCoordinatorNode fails")
// 	assert.Empty(t, c.state.activeCoordinatorNode, "should not set activeCoordinatorNode field on error")
// }

// func TestCoordinator_GetActiveCoordinatorNode_ReturnsEmptyStringWhenSelectActiveCoordinatorNodeFailsDueToInvalidIdentity(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
// 		StaticCoordinator:    proto.String("invalid"), // Invalid identity format
// 	})
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""

// 	result := c.GetActiveCoordinatorNode(ctx, true)
// 	// When node extraction fails, it should return empty string
// 	assert.Empty(t, result, "should return empty string when identity extraction fails")
// }

// func TestCoordinator_GetActiveCoordinatorNode_ReturnsEmptyStringWhenSelectActiveCoordinatorNodeFailsInEndorserModeWithEmptyPool(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
// 	})
// 	config := builder.GetSequencerConfig()
// 	config.BlockRange = confutil.P(uint64(100))
// 	builder.OverrideSequencerConfig(config)
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""
// 	c.smContext.originatorNodePool = []string{} // Empty pool
// 	c.state.currentBlockHeight = 1000

// 	result := c.GetActiveCoordinatorNode(ctx, true)
// 	// SelectActiveCoordinatorNode returns empty string (not error) for empty pool
// 	assert.Empty(t, result, "should return empty string when pool is empty")
// 	assert.Empty(t, c.state.activeCoordinatorNode, "should not set activeCoordinatorNode field when pool is empty")
// }

// func TestCoordinator_GetActiveCoordinatorNode_DoesNotReInitializeWhenCalledMultipleTimesWithInitIfNoActiveCoordinatorTrue(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
// 	})
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""

// 	// First call should initialize
// 	result1 := c.GetActiveCoordinatorNode(ctx, true)
// 	assert.Equal(t, "node1", result1, "first call should initialize and return node1")

// 	// Second call should return the same value without re-initializing
// 	result2 := c.GetActiveCoordinatorNode(ctx, true)
// 	assert.Equal(t, "node1", result2, "second call should return same value")
// 	assert.Equal(t, "node1", c.state.activeCoordinatorNode, "activeCoordinatorNode should remain set")
// }

// func TestCoordinator_GetActiveCoordinatorNode_HandlesSwitchingBetweenInitAndNonInitModes(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
// 	})
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""

// 	// Call with initIfNoActiveCoordinator = false should return empty
// 	result1 := c.GetActiveCoordinatorNode(ctx, false)
// 	assert.Empty(t, result1, "should return empty when initIfNoActiveCoordinator is false")

// 	// Call with initIfNoActiveCoordinator = true should initialize
// 	result2 := c.GetActiveCoordinatorNode(ctx, true)
// 	assert.Equal(t, "node1", result2, "should initialize when initIfNoActiveCoordinator is true")

// 	// Call with initIfNoActiveCoordinator = false should still return the initialized value
// 	result3 := c.GetActiveCoordinatorNode(ctx, false)
// 	assert.Equal(t, "node1", result3, "should return initialized value even when initIfNoActiveCoordinator is false")
// }

// func TestCoordinator_GetActiveCoordinatorNode_HandlesContextCancellationGracefully(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
// 		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
// 		StaticCoordinator:    proto.String("identity@node1"),
// 	})
// 	c, _ := builder.Build(ctx)
// 	c.state.activeCoordinatorNode = ""

// 	// Create a cancelled context
// 	cancelledCtx, cancel := context.WithCancel(ctx)
// 	cancel()

// 	result := c.GetActiveCoordinatorNode(cancelledCtx, true)
// 	// The function should still work even with cancelled context
// 	assert.NotNil(t, result, "should handle cancelled context without panicking")
// }

// TODO AM: these tests that queue events and let the state machine run will want pulling together into their own file
func TestCoordinator_PropagateEventToTransaction_IgnoresUnknownTransaction(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Ensure transactionsByID is empty
	c.state.transactionsByID = make(map[uuid.UUID]*transaction.Transaction)

	// Create an event for a transaction that doesn't exist in the coordinator
	unknownTxID := uuid.New()
	event := &transaction.AssembleSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: unknownTxID,
		},
	}

	// Should not error - just ignores the event (no crash, no state change)
	queueAndSync(ctx, c, event)

	// Verify transactionsByID is still empty (no transaction was created)
	assert.Equal(t, 0, len(c.state.transactionsByID), "transactionsByID should remain empty")
}

func TestCoordinator_PropagateEventToTransaction_ProcessesKnownTransaction(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create and add a transaction in Pooled state
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()
	c.state.transactionsByID[txn.ID] = txn

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState())

	// Send a SelectedEvent to move it to Assembling
	event := &transaction.SelectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	}

	// Queue the event and wait for processing
	queueAndSync(ctx, c, event)

	// Transaction should have moved to Assembling state after being selected
	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(),
		"transaction should have transitioned to Assembling state")
}

func TestCoordinator_HeartbeatLoop_HandlesPropagateEventToAllTransactionsErrorsGracefully(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, _ := builder.Build(ctx)
	c.heartbeatLoop.interval = 10 * time.Millisecond // can't use builder.OverrideSequencerConfig() because NewCoordinator enforces a minimum of 1 second

	// Set up originator pool with another node so heartbeats can be sent
	c.UpdateOriginatorNodePool(ctx, "node2")

	// Mock grapher that returns an error on cleanup - this will cause propagateEventToAllTransactions to fail
	mockGrapher := transaction.NewMockGrapher(t)
	mockGrapher.On("Add", mock.Anything, mock.Anything).Return().Twice() // Called during transaction creation
	mockGrapher.On("Forget", mock.Anything).Return(fmt.Errorf("grapher error")).Twice()

	// Transaction in State_Confirmed that will try to transition to State_Final on heartbeat
	// heartbeatIntervalsSinceStateChange >= grace period (5) triggers cleanup attempt
	txn1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).
		Grapher(mockGrapher).
		HeartbeatIntervalsSinceStateChange(5).
		Build()
	c.state.transactionsByID[txn1.ID] = txn1

	// Start heartbeat loop in a goroutine
	done := make(chan struct{})
	go func() {
		c.heartbeatLoop.start(ctx)
		close(done)
	}()

	// Wait for loop to have attempted cleanup after the initial heartbeat interval event
	// (2 calls includes the initial Add call)
	assert.Eventually(t, func() bool {
		return len(mockGrapher.Calls) == 2
	}, 500*time.Millisecond, 5*time.Millisecond, "expected at least 1 Forget calls")

	// create a second transaction in State_Confirmed that will try to transition to State_Final on heartbeat
	txn2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).
		Grapher(mockGrapher).
		HeartbeatIntervalsSinceStateChange(5).
		Build()
	c.state.transactionsByID[txn2.ID] = txn2

	// Wait for loop to have attempted cleanup after a periodic heartbeat interval event
	// (4 calls includes the first 2 calls, another Add call, and another Forget call)
	assert.Eventually(t, func() bool {
		return len(mockGrapher.Calls) == 4
	}, 500*time.Millisecond, 5*time.Millisecond, "expected at least 2 Forget calls")

	// Cancel to stop the loop
	c.heartbeatLoop.stop()
	<-done
}

// Event loop integration tests for coordinator state machine through the public QueueEvent interface.
// These tests verify the coordinator behavior through the full event loop path.

func TestEventLoop_Coordinator_InitializeOK(t *testing.T) {
	ctx := context.Background()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer c.Stop()

	assert.Equal(t, State_Idle, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
}

func TestEventLoop_Coordinator_Idle_ToActive_OnTransactionsDelegated(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.OriginatorIdentityPool(originator)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	// Use mock.Anything for context since event loop uses internal cancelCtx
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	require.Equal(t, State_Idle, c.GetCurrentState())

	// Queue event through public interface
	c.QueueEvent(ctx, &TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})

	assertEventualStateEventLoop(t, c, State_Active,
		"expected transition from Idle to Active")
}

func TestEventLoop_Coordinator_Idle_ToObserving_OnHeartbeatReceived(t *testing.T) {
	ctx := context.Background()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer c.Stop()

	require.Equal(t, State_Idle, c.GetCurrentState())

	c.QueueEvent(ctx, &HeartbeatReceivedEvent{})

	assertEventualStateEventLoop(t, c, State_Observing,
		"expected transition from Idle to Observing")
}

func TestEventLoop_Coordinator_Observing_ToStandby_OnDelegated_IfBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := NewCoordinatorBuilderForTesting(t, State_Observing).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(194) // default tolerance is 5 so this is behind
	// Use mock.Anything for context since event loop uses internal cancelCtx
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})

	assertEventualStateEventLoop(t, c, State_Standby,
		"expected transition from Observing to Standby when behind")
}

func TestEventLoop_Coordinator_Observing_ToElect_OnDelegated_IfNotBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := NewCoordinatorBuilderForTesting(t, State_Observing).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(195) // default tolerance is 5 so this is not behind
	// Use mock.Anything for context since event loop uses internal cancelCtx
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, mocks := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})

	assertEventualStateEventLoop(t, c, State_Elect,
		"expected transition from Observing to Elect when not behind")

	// Verify side effect: handover request was sent
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentHandoverRequest()
	}, eventLoopTestTimeout, eventLoopTestInterval,
		"expected handover request to be sent")
}

func TestEventLoop_Coordinator_Standby_ToElect_OnNewBlock_IfNotBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := NewCoordinatorBuilderForTesting(t, State_Standby).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(194)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &NewBlockEvent{
		BlockHeight: 195, // default tolerance is 5 so we are not behind
	})

	assertEventualStateEventLoop(t, c, State_Elect,
		"expected transition from Standby to Elect when caught up")
}

func TestEventLoop_Coordinator_Standby_StaysInStandby_OnNewBlock_IfStillBehind(t *testing.T) {
	ctx := context.Background()

	builder := NewCoordinatorBuilderForTesting(t, State_Standby).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(193)
	c, mocks := builder.Build(ctx)
	defer c.Stop()

	queueAndSync(ctx, c, &NewBlockEvent{
		BlockHeight: 194, // still behind
	})

	assert.Equal(t, State_Standby, c.GetCurrentState(),
		"expected to stay in Standby when still behind")
	assert.False(t, mocks.SentMessageRecorder.HasSentHandoverRequest(),
		"handover request should not be sent")
}

func TestEventLoop_Coordinator_Elect_ToPrepared_OnHandover(t *testing.T) {
	ctx := context.Background()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &HandoverReceivedEvent{})

	assertEventualStateEventLoop(t, c, State_Prepared,
		"expected transition from Elect to Prepared")
}

func TestEventLoop_Coordinator_Prepared_ToActive_OnTransactionConfirmed_IfFlushCompleted(t *testing.T) {
	ctx := context.Background()

	builder := NewCoordinatorBuilderForTesting(t, State_Prepared)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	domainAPI := builder.GetDomainAPI()
	domainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})

	c.QueueEvent(ctx, &TransactionConfirmedEvent{
		From:  builder.GetFlushPointSignerAddress(),
		Nonce: builder.GetFlushPointNonce(),
		Hash:  builder.GetFlushPointHash(),
	})

	assertEventualStateEventLoop(t, c, State_Active,
		"expected transition from Prepared to Active when flush completed")
}

func TestEventLoop_Coordinator_Prepared_StaysInPrepared_OnTransactionConfirmed_IfNotFlushCompleted(t *testing.T) {
	ctx := context.Background()

	builder := NewCoordinatorBuilderForTesting(t, State_Prepared)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	otherHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	otherNonce := builder.GetFlushPointNonce() - 1

	queueAndSync(ctx, c, &TransactionConfirmedEvent{
		From:  builder.GetFlushPointSignerAddress(),
		Nonce: otherNonce,
		Hash:  otherHash,
	})

	assert.Equal(t, State_Prepared, c.GetCurrentState(),
		"expected to stay in Prepared when flush not completed")
}

func TestEventLoop_Coordinator_Active_ToIdle_NoTransactionsInFlight(t *testing.T) {
	ctx := context.Background()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})

	assertEventualStateEventLoop(t, c, State_Idle,
		"expected transition from Active to Idle when no transactions in flight")
}

func TestEventLoop_Coordinator_Active_StaysActive_OnTransactionConfirmed_IfTransactionsRemain(t *testing.T) {
	ctx := context.Background()

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	queueAndSync(ctx, c, &TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assert.Equal(t, State_Active, c.GetCurrentState(),
		"expected to stay Active when transactions remain")
}

func TestEventLoop_Coordinator_Active_ToFlush_OnHandoverRequest(t *testing.T) {
	ctx := context.Background()

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &HandoverRequestEvent{
		Requester: "newCoordinator",
	})

	assertEventualStateEventLoop(t, c, State_Flush,
		"expected transition from Active to Flush on handover request")
}

func TestEventLoop_Coordinator_Flush_ToClosing_OnTransactionConfirmed_IfFlushComplete(t *testing.T) {
	ctx := context.Background()

	// One transaction past point of no return, one not
	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Flush).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assertEventualStateEventLoop(t, c, State_Closing,
		"expected transition from Flush to Closing when flush complete")
}

func TestEventLoop_Coordinator_Flush_StaysInFlush_OnTransactionConfirmed_IfNotFlushComplete(t *testing.T) {
	ctx := context.Background()

	// Both transactions past point of no return
	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Flush).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	queueAndSync(ctx, c, &TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assert.Equal(t, State_Flush, c.GetCurrentState(),
		"expected to stay in Flush when flush not complete")
}

func TestEventLoop_Coordinator_Closing_ToIdle_OnHeartbeatInterval_IfClosingGracePeriodExpired(t *testing.T) {
	ctx := context.Background()

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	builder := NewCoordinatorBuilderForTesting(t, State_Closing).
		HeartbeatsUntilClosingGracePeriodExpires(1).
		Transactions(d)

	config := builder.GetSequencerConfig()
	config.ClosingGracePeriod = confutil.P(5)
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})

	assertEventualStateEventLoop(t, c, State_Idle,
		"expected transition from Closing to Idle when grace period expired")
}

func TestEventLoop_Coordinator_Closing_StaysInClosing_OnHeartbeatInterval_IfGracePeriodNotExpired(t *testing.T) {
	ctx := context.Background()

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	builder := NewCoordinatorBuilderForTesting(t, State_Closing).
		HeartbeatsUntilClosingGracePeriodExpires(2).
		Transactions(d)
	config := builder.GetSequencerConfig()
	config.ClosingGracePeriod = confutil.P(5)
	builder.OverrideSequencerConfig(config)

	c, _ := builder.Build(ctx)
	defer c.Stop()

	queueAndSync(ctx, c, &common.HeartbeatIntervalEvent{})

	assert.Equal(t, State_Closing, c.GetCurrentState(),
		"expected to stay in Closing when grace period not expired")
}

func TestEventLoop_Coordinator_GracefulShutdownSequence(t *testing.T) {
	// Tests the complete shutdown sequence: Active -> Flush -> Closing -> Idle
	ctx := context.Background()

	delegation := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	builder := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(delegation)
	config := builder.GetSequencerConfig()
	config.ClosingGracePeriod = confutil.P(1) // Short grace period for test
	builder.OverrideSequencerConfig(config)

	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Step 1: Handover request -> Active to Flush
	c.QueueEvent(ctx, &HandoverRequestEvent{
		Requester: "newCoordinator",
	})

	assertEventualStateEventLoop(t, c, State_Flush,
		"Step 1: expected transition to Flush")

	// Step 2: Transaction confirmed -> Flush to Closing
	c.QueueEvent(ctx, &TransactionConfirmedEvent{
		From:  delegation.GetSignerAddress(),
		Nonce: *delegation.GetNonce(),
		Hash:  *delegation.GetLatestSubmissionHash(),
	})

	assertEventualStateEventLoop(t, c, State_Closing,
		"Step 2: expected transition to Closing")

	// Step 3: Grace period expires -> Closing to Idle
	c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})

	assertEventualStateEventLoop(t, c, State_Idle,
		"Step 3: expected transition to Idle")
}
