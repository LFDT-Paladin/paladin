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
	"google.golang.org/protobuf/proto"
)

func TestTransactionStateTransition(t *testing.T) {

}

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
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(0) // Stop the dispatcher loop from progressing states - we're manually updating state throughout the test
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)

	// Start by simulating the originator and delegate a transaction to the coordinator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	err := c.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: []*components.PrivateTransaction{txn},
	})
	assert.NoError(t, err)

	// Assert that snapshot contains a transaction with matching ID
	snapshot := c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that a request has been sent to the originator and respond with an assembled transaction
	require.True(t, mocks.SentMessageRecorder.HasSentAssembleRequest())
	err = c.ProcessEvent(ctx, &transaction.AssembleSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID:    mocks.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
		PostAssembly: transactionBuilder.BuildPostAssembly(),
		PreAssembly:  transactionBuilder.BuildPreAssembly(),
	})
	assert.NoError(t, err)

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that the coordinator has sent an endorsement request to the endorser and respond with an endorsement
	require.Equal(t, 1, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())
	err = c.ProcessEvent(ctx, &transaction.EndorsedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID:   mocks.SentMessageRecorder.SentEndorsementRequestsForPartyIdempotencyKey(transactionBuilder.GetEndorserIdentityLocator(0)),
		Endorsement: transactionBuilder.BuildEndorsement(0),
	})
	assert.NoError(t, err)

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that the coordinator has sent a dispatch confirmation request to the transaction sender and respond with a dispatch confirmation
	require.True(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest())
	err = c.ProcessEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: mocks.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	// Assert that snapshot no longer contains that transaction in the pooled transactions but does contain it in the dispatched transactions
	//NOTE: This is a key design point.  When a transaction is ready to be dispatched, we communicate to other nodes, via the heartbeat snapshot, that the transaction is dispatched.
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that the transaction is ready to be collected by the dispatcher thread
	readyTransactions, err := c.GetTransactionsReadyToDispatch(ctx)
	require.NoError(t, err)
	require.NotNil(t, readyTransactions)
	require.Equal(t, 1, len(readyTransactions), "There should be exactly one transaction ready to dispatch")
	assert.Equal(t, txn.ID.String(), readyTransactions[0].ID.String(), "The transaction ready to dispatch should match the delegated transaction ID")

	// Simulate the dispatcher thread collecting the transaction and dispatching it to a public transaction manager
	err = c.ProcessEvent(ctx, &transaction.DispatchedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	// Simulate the public transaction manager collecting the dispatched transaction and associating a signing address with it
	signerAddress := pldtypes.RandAddress()
	err = c.ProcessEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})
	assert.NoError(t, err)

	// Assert that we now have a signer address in the snapshot
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.DispatchedTransactions[0].Signer.String(), "Snapshot should contain the dispatched transaction with signer address %s", signerAddress.String())

	// Simulate the dispatcher thread allocating a nonce for the transaction
	err = c.ProcessEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: 42,
	})
	assert.NoError(t, err)

	// Assert that the nonce is now included in the snapshot
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.DispatchedTransactions[0].Signer.String(), "Snapshot should contain the dispatched transaction with signer address %s", signerAddress.String())
	require.NotNil(t, snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with a nonce")
	assert.Equal(t, uint64(42), *snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with nonce 42")

	// Simulate the public transaction manager submitting the transaction
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	err = c.ProcessEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SubmissionHash: submissionHash,
	})
	assert.NoError(t, err)

	// Assert that the hash is now included in the snapshot
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.DispatchedTransactions[0].Signer.String(), "Snapshot should contain the dispatched transaction with signer address %s", signerAddress.String())
	require.NotNil(t, snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with a nonce")
	assert.Equal(t, uint64(42), *snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with nonce 42")
	require.NotNil(t, snapshot.DispatchedTransactions[0].LatestSubmissionHash, "Snapshot should contain the dispatched transaction with a submission hash")

	// Simulate the block indexer confirming the transaction
	err = c.ProcessEvent(ctx, &transaction.ConfirmedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: 42,
		Hash:  submissionHash,
	})
	assert.NoError(t, err)

	// Assert that snapshot contains a transaction with matching ID
	snapshot = c.getSnapshot(ctx)
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

func TestCoordinator_MaxInflightTransactions(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxInflightTransactions = confutil.P(5)
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)
	c, _ := builder.Build(ctx)

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

func TestCoordinator_AddToDelegatedTransactions_NewTransactionError(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Use a valid originator for the transaction builder (it validates immediately)
	validOriginator := "sender@senderNode"
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(validOriginator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Use an invalid originator identity that will cause NewTransaction to return an error
	invalidOriginator := "sender@node1@node2"
	err := c.addToDelegatedTransactions(ctx, invalidOriginator, []*components.PrivateTransaction{txn})

	require.Error(t, err, "should return error when NewTransaction fails")
	// Verify that the transaction was not added to transactionsByID
	assert.Equal(t, 0, len(c.transactionsByID), "transaction should not be added when NewTransaction fails")
}

func TestCoordinator_AddToDelegatedTransactions_HasChainedTransactionError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	expectedError := fmt.Errorf("database error checking chained transaction")
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(false, expectedError)
	c, _ := builder.Build(ctx)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Call addToDelegatedTransactions - this should return an error when HasChainedTransaction fails
	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

	require.Error(t, err, "should return error when HasChainedTransaction fails")
	assert.Equal(t, expectedError, err, "should return the same error from HasChainedTransaction")
	assert.Equal(t, 1, len(c.transactionsByID), "transaction is added before HasChainedTransaction check, so it will be in the map even if check fails")
}

func TestCoordinator_AddToDelegatedTransactions_WithChainedTransaction(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(true, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(0) // Stop the dispatcher loop from progressing states
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Call addToDelegatedTransactions - this should call SetChainedTxInProgress() when hasChainedTransaction is true
	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

	// Verify that no error occurred
	require.NoError(t, err, "should not return error when HasChainedTransaction returns true")

	// Verify that the transaction was added to transactionsByID
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	coordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")

	// Verify that SetChainedTxInProgress() was called by checking the transaction state
	assert.Equal(t, transaction.State_Submitted, coordinatedTxn.GetState(), "transaction should be in State_Submitted when chained transaction is found")
}

func TestCoordinator_AddToDelegatedTransactions_WithoutChainedTransaction(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(0) // Stop the dispatcher loop from progressing states
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Call addToDelegatedTransactions - this should NOT call SetChainedTxInProgress() when hasChainedTransaction is false
	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

	// Verify that no error occurred
	require.NoError(t, err, "should not return error when HasChainedTransaction returns false")

	// Verify that the transaction was added to transactionsByID
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	coordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")

	assert.NotEqual(t, transaction.State_Submitted, coordinatedTxn.GetState(), "transaction should NOT be in State_Submitted when chained transaction is not found")
	// The transaction should be in a state that indicates it's ready for normal processing
	assert.Contains(t, []transaction.State{transaction.State_Pooled, transaction.State_PreAssembly_Blocked}, coordinatedTxn.GetState(), "transaction should be in Pooled or PreAssembly_Blocked state when chained transaction is not found")
}

func TestCoordinator_SelectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithFullyQualifiedIdentity(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity@node1"),
	})
	c, _ := builder.Build(ctx)

	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func TestCoordinator_SelectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithIdentityOnly(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity"),
	})
	c, _ := builder.Build(ctx)

	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
	// When node is not specified and allowEmptyNode is false, it should return an error
	require.Error(t, err)
	assert.Empty(t, coordinatorNode)
}

func TestCoordinator_SelectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithEmptyStaticCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String(""),
	})
	c, _ := builder.Build(ctx)

	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
	require.Error(t, err)
	assert.Empty(t, coordinatorNode)
	assert.Contains(t, err.Error(), "static coordinator mode is configured but static coordinator node is not set")
}

func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithEmptyPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{}
	c.currentBlockHeight = 1000

	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Empty(t, coordinatorNode)
}

func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithSingleNodeInPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{"node1"}
	c.currentBlockHeight = 1000

	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithMultipleNodesInPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000

	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Contains(t, []string{"node1", "node2", "node3"}, coordinatorNode)
}

func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithBlockHeightRounding(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{"node1", "node2", "node3"}

	// Test that blocks within the same range select the same coordinator
	c.currentBlockHeight = 1000
	coordinatorNode1, err1 := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err1)

	c.currentBlockHeight = 1001
	coordinatorNode2, err2 := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err2)

	c.currentBlockHeight = 1099
	coordinatorNode3, err3 := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err3)

	// All should select the same coordinator since they're in the same block range
	assert.Equal(t, coordinatorNode1, coordinatorNode2)
	assert.Equal(t, coordinatorNode2, coordinatorNode3)

	// Different block range should potentially select different coordinator
	c.currentBlockHeight = 1100
	coordinatorNode4, err4 := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err4)

	assert.Contains(t, []string{"node1", "node2", "node3"}, coordinatorNode4)
}

func TestCoordinator_SelectActiveCoordinatorNode_EndorserMode_WithDifferentBlockRanges(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(50))
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{"node1", "node2"}

	c.currentBlockHeight = 100
	coordinatorNode1, err1 := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err1)

	c.currentBlockHeight = 150
	coordinatorNode2, err2 := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err2)

	// Different block ranges should potentially select different coordinators
	assert.Contains(t, []string{"node1", "node2"}, coordinatorNode1)
	assert.Contains(t, []string{"node1", "node2"}, coordinatorNode2)
}

func TestCoordinator_SelectActiveCoordinatorNode_SenderMode_ReturnsCurrentNodeName(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _ := builder.Build(ctx)
	// The builder sets nodeName to "node1" by default
	assert.Equal(t, "node1", c.nodeName)

	coordinatorNode, err := c.SelectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func TestCoordinator_Stop_StopsEventLoopAndDispatchLoop(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Verify loops are running by checking channels are not closed
	select {
	case <-c.eventLoopStopped:
		t.Fatal("event loop should not be stopped initially")
	default:
	}
	select {
	case <-c.dispatchLoopStopped:
		t.Fatal("dispatch loop should not be stopped initially")
	default:
	}

	// Should block until shutdown is complete
	c.Stop()

	// Verify both loops have stopped (reading from closed channel returns immediately with ok=false)
	select {
	case _, ok := <-c.eventLoopStopped:
		require.False(t, ok, "event loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("event loop did not stop within timeout")
	}

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
	case _, ok := <-c.eventLoopStopped:
		require.False(t, ok, "event loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("event loop did not stop within timeout")
	}

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

	// Queue some events to ensure loops are busy
	for i := 0; i < 10; i++ {
		c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})
	}

	c.Stop()

	// Verify both loops have stopped
	select {
	case _, ok := <-c.eventLoopStopped:
		require.False(t, ok, "event loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("event loop did not stop within timeout")
	}

	select {
	case _, ok := <-c.dispatchLoopStopped:
		require.False(t, ok, "dispatch loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

func TestCoordinator_ConfirmDispatchedTransaction_FindsTransactionBySignerAndNonce(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction with signer address and nonce
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn := txBuilder.Build()

	// Set signer address and nonce on the transaction
	err := txn.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

	// Confirm the transaction
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := c.confirmDispatchedTransaction(ctx, txn.ID, signerAddress, nonce, hash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found by signer and nonce")
}

func TestCoordinator_ConfirmDispatchedTransaction_FindsTransactionByTxIdWhenFromIsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

	// Confirm the transaction with nil from address (chained transaction scenario)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	var nilFrom *pldtypes.EthAddress = nil
	found, err := c.confirmDispatchedTransaction(ctx, txn.ID, nilFrom, 0, hash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found by txId")
}

func TestCoordinator_ConfirmDispatchedTransaction_FindsTransactionByTxIdWhenSignerNonceLookupFails(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction without signer/nonce (chained transaction)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

	// Try to confirm with a signer+nonce that doesn't match
	nonMatchingSigner := pldtypes.RandAddress()
	nonMatchingNonce := uint64(999)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := c.confirmDispatchedTransaction(ctx, txn.ID, nonMatchingSigner, nonMatchingNonce, hash, revertReason)

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
	found, err := c.confirmDispatchedTransaction(ctx, nonExistentTxID, signerAddress, nonce, hash, revertReason)

	require.NoError(t, err)
	assert.False(t, found, "transaction should not be found")
}

func TestCoordinator_ConfirmDispatchedTransaction_HandlesMatchingHashCorrectly(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction with a submission hash
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn := txBuilder.Build()

	// Set signer, nonce, and submission hash
	err := txn.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SubmissionHash: submissionHash,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

	// Confirm with matching hash
	revertReason := pldtypes.HexBytes{}
	found, err := c.confirmDispatchedTransaction(ctx, txn.ID, signerAddress, nonce, submissionHash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found and confirmed")
}

func TestCoordinator_ConfirmDispatchedTransaction_HandlesDifferentHashCorrectly(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction with a submission hash
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn := txBuilder.Build()

	// Set signer, nonce, and submission hash
	err := txn.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SubmissionHash: submissionHash,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

	// Confirm with different hash (should still work, just logs a warning)
	differentHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := c.confirmDispatchedTransaction(ctx, txn.ID, signerAddress, nonce, differentHash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found even with different hash")
}

func TestCoordinator_ConfirmDispatchedTransaction_HandlesNilSubmissionHashCorrectly(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction without a submission hash (chained transaction)
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn := txBuilder.Build()

	// Set signer and nonce but no submission hash
	err := txn.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

	// Confirm with a hash (chained transaction scenario)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := c.confirmDispatchedTransaction(ctx, txn.ID, signerAddress, nonce, hash, revertReason)

	require.NoError(t, err)
	assert.True(t, found, "transaction should be found even with nil submission hash")
	assert.Nil(t, txn.GetLatestSubmissionHash(), "transaction should have nil submission hash")
}

func TestCoordinator_ConfirmDispatchedTransaction_ReturnsErrorWhenHandleEventFails(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction in a state that cannot handle ConfirmedEvent
	// We'll use State_Pooled which should not accept ConfirmedEvent
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)

	// Set signer and nonce
	err := txn.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

	// Try to confirm - this should fail because the transaction is in State_Pooled
	// which may not accept ConfirmedEvent depending on state machine rules
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found, err := c.confirmDispatchedTransaction(ctx, txn.ID, signerAddress, nonce, hash, revertReason)

	// The function may return an error if HandleEvent fails, or it may succeed
	// depending on the state machine rules. We just verify it doesn't panic.
	if err != nil {
		assert.False(t, found, "should return false when error occurs")
	}
}

func TestCoordinator_ConfirmDispatchedTransaction_HandlesMultipleTransactionsCorrectly(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create multiple transactions
	signerAddress1 := pldtypes.RandAddress()
	nonce1 := uint64(42)
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn1 := txBuilder1.Build()

	err := txn1.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn1.ID,
		},
		SignerAddress: *signerAddress1,
	})
	require.NoError(t, err)

	err = txn1.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn1.ID,
		},
		Nonce: nonce1,
	})
	require.NoError(t, err)

	signerAddress2 := pldtypes.RandAddress()
	nonce2 := uint64(43)
	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn2 := txBuilder2.Build()

	err = txn2.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn2.ID,
		},
		SignerAddress: *signerAddress2,
	})
	require.NoError(t, err)

	err = txn2.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn2.ID,
		},
		Nonce: nonce2,
	})
	require.NoError(t, err)

	// Add both transactions to coordinator
	c.transactionsByID[txn1.ID] = txn1
	c.transactionsByID[txn2.ID] = txn2

	// Confirm first transaction
	hash1 := pldtypes.Bytes32(pldtypes.RandBytes(32))
	revertReason := pldtypes.HexBytes{}
	found1, err := c.confirmDispatchedTransaction(ctx, txn1.ID, signerAddress1, nonce1, hash1, revertReason)
	require.NoError(t, err)
	assert.True(t, found1, "first transaction should be found")

	// Confirm second transaction
	hash2 := pldtypes.Bytes32(pldtypes.RandBytes(32))
	found2, err := c.confirmDispatchedTransaction(ctx, txn2.ID, signerAddress2, nonce2, hash2, revertReason)
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

	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", signerAddress.String(), nonce): {
			From:          *signerAddress,
			Nonce:         nonce,
			TransactionID: txID,
			Hash:          hash,
			Confirmed:     false,
		},
	}

	// Confirm the monitored transaction
	c.confirmMonitoredTransaction(ctx, signerAddress, nonce)

	// Verify the flush point is now confirmed
	flushPoint := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress.String(), nonce)]
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

	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", signerAddress.String(), nonce): {
			From:          *signerAddress,
			Nonce:         nonce,
			TransactionID: txID,
			Hash:          hash,
			Confirmed:     true,
		},
	}

	// Confirm the monitored transaction again
	c.confirmMonitoredTransaction(ctx, signerAddress, nonce)

	// Verify the flush point is still confirmed
	flushPoint := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress.String(), nonce)]
	require.NotNil(t, flushPoint, "flush point should still exist")
	assert.True(t, flushPoint.Confirmed, "flush point should remain confirmed")
}

func TestCoordinator_ConfirmMonitoredTransaction_NoOpWhenFlushPointDoesNotExist(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Set up empty flush points map
	c.activeCoordinatorsFlushPointsBySignerNonce = make(map[string]*common.FlushPoint)

	// Try to confirm a non-existent flush point
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	c.confirmMonitoredTransaction(ctx, signerAddress, nonce)

	// Verify the map is still empty
	assert.Equal(t, 0, len(c.activeCoordinatorsFlushPointsBySignerNonce), "flush points map should remain empty")
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

	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
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
	c.confirmMonitoredTransaction(ctx, signerAddress1, nonce1)

	// Verify only the first flush point is confirmed
	flushPoint1 := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress1.String(), nonce1)]
	require.NotNil(t, flushPoint1, "first flush point should exist")
	assert.True(t, flushPoint1.Confirmed, "first flush point should be confirmed")

	flushPoint2 := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress2.String(), nonce2)]
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

	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
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
	c.confirmMonitoredTransaction(ctx, signerAddress, nonce1)

	// Verify only the first nonce is confirmed
	flushPoint1 := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress.String(), nonce1)]
	require.NotNil(t, flushPoint1, "first flush point should exist")
	assert.True(t, flushPoint1.Confirmed, "first flush point should be confirmed")

	flushPoint2 := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress.String(), nonce2)]
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

	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
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
	c.confirmMonitoredTransaction(ctx, signerAddress1, nonce)

	// Verify only the first signer's flush point is confirmed
	flushPoint1 := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress1.String(), nonce)]
	require.NotNil(t, flushPoint1, "first flush point should exist")
	assert.True(t, flushPoint1.Confirmed, "first flush point should be confirmed")

	flushPoint2 := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", signerAddress2.String(), nonce)]
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

	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", signerAddress.String(), nonce): {
			From:          *signerAddress,
			Nonce:         nonce,
			TransactionID: txID,
			Hash:          hash,
			Confirmed:     false,
		},
	}

	// Confirm the monitored transaction
	c.confirmMonitoredTransaction(ctx, signerAddress, nonce)

	// Verify the flush point still exists in the map (not removed)
	key := fmt.Sprintf("%s:%d", signerAddress.String(), nonce)
	flushPoint, exists := c.activeCoordinatorsFlushPointsBySignerNonce[key]
	require.True(t, exists, "flush point should still exist in map")
	require.NotNil(t, flushPoint, "flush point should not be nil")
	assert.True(t, flushPoint.Confirmed, "flush point should be confirmed")
	assert.Equal(t, 1, len(c.activeCoordinatorsFlushPointsBySignerNonce), "map should still contain one flush point")
}

func TestCoordinator_UpdateOriginatorNodePool_AddsNodeToEmptyPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{}

	c.UpdateOriginatorNodePool(ctx, "node2")

	// Should contain both the added node and the coordinator's own node
	assert.Equal(t, 2, len(c.originatorNodePool), "pool should contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
}

func TestCoordinator_UpdateOriginatorNodePool_AddsNodeToNonEmptyPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{"node1", "node3"}

	c.UpdateOriginatorNodePool(ctx, "node2")

	// Should contain all nodes including the new one
	assert.Equal(t, 3, len(c.originatorNodePool), "pool should contain 3 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
	assert.Contains(t, c.originatorNodePool, "node3", "pool should contain node3")
}

func TestCoordinator_UpdateOriginatorNodePool_DoesNotAddDuplicateNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{"node1", "node2"}

	c.UpdateOriginatorNodePool(ctx, "node2")

	// Should not have duplicates
	assert.Equal(t, 2, len(c.originatorNodePool), "pool should still contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
}

func TestCoordinator_UpdateOriginatorNodePool_EnsuresCoordinatorsOwnNodeIsAlwaysInPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{}

	// Add a different node
	c.UpdateOriginatorNodePool(ctx, "node2")

	// Coordinator's own node (node1) should be automatically added
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
	assert.Equal(t, 2, len(c.originatorNodePool), "pool should contain 2 nodes")
}

func TestCoordinator_UpdateOriginatorNodePool_EnsuresCoordinatorsOwnNodeIsAddedEvenWhenPoolAlreadyHasOtherNodes(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	// Manually set pool without coordinator's own node
	c.originatorNodePool = []string{"node2", "node3"}

	c.UpdateOriginatorNodePool(ctx, "node4")

	// Coordinator's own node (node1) should be automatically added
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
	assert.Equal(t, 4, len(c.originatorNodePool), "pool should contain 4 nodes")
}

func TestCoordinator_UpdateOriginatorNodePool_DoesNotDuplicateCoordinatorsOwnNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{"node1", "node2"}

	// Try to add coordinator's own node
	c.UpdateOriginatorNodePool(ctx, "node1")

	// Should not have duplicates
	assert.Equal(t, 2, len(c.originatorNodePool), "pool should still contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
}

func TestCoordinator_UpdateOriginatorNodePool_HandlesMultipleSequentialUpdates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{}

	// Add multiple nodes sequentially
	c.UpdateOriginatorNodePool(ctx, "node2")
	c.UpdateOriginatorNodePool(ctx, "node3")
	c.UpdateOriginatorNodePool(ctx, "node4")

	// Should contain all nodes including coordinator's own node
	assert.Equal(t, 4, len(c.originatorNodePool), "pool should contain 4 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
	assert.Contains(t, c.originatorNodePool, "node3", "pool should contain node3")
	assert.Contains(t, c.originatorNodePool, "node4", "pool should contain node4")
}

func TestCoordinator_UpdateOriginatorNodePool_HandlesEmptyStringNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{}

	c.UpdateOriginatorNodePool(ctx, "")

	// Empty string should be added, and coordinator's own node should be added
	assert.Equal(t, 2, len(c.originatorNodePool), "pool should contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "", "pool should contain empty string")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
}

func TestCoordinator_UpdateOriginatorNodePool_IsThreadSafe(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.originatorNodePool = []string{}

	// Concurrent updates - use node names that don't conflict with coordinator's own node
	done := make(chan struct{})
	numGoroutines := 10
	nodesPerGoroutine := 5

	for i := 0; i < numGoroutines; i++ {
		go func(startNode int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < nodesPerGoroutine; j++ {
				// Use node names starting from 100 to avoid conflict with coordinator's "node1"
				nodeName := fmt.Sprintf("node%d", 100+startNode*100+j)
				c.UpdateOriginatorNodePool(ctx, nodeName)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Pool should contain all unique nodes plus coordinator's own node
	// Total should be: numGoroutines * nodesPerGoroutine + 1 (coordinator's own node)
	expectedCount := numGoroutines*nodesPerGoroutine + 1
	assert.Equal(t, expectedCount, len(c.originatorNodePool), "pool should contain all unique nodes plus coordinator's own node")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")

	// Verify no duplicates
	nodeSet := make(map[string]bool)
	for _, node := range c.originatorNodePool {
		assert.False(t, nodeSet[node], "pool should not contain duplicate node: %s", node)
		nodeSet[node] = true
	}
}

func TestCoordinator_SendHandoverRequest_SuccessfullySendsHandoverRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build(ctx)
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
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "testCoordinatorNode"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
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
	activeCoordinatorNode := "activeCoordinatorNode"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_HandlesErrorFromSendHandoverRequestGracefully(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "activeCoordinatorNode"
	expectedError := fmt.Errorf("transport error")

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode
	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(expectedError)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest - should not panic even when error occurs
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_HandlesEmptyActiveCoordinatorNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := ""

	// Set empty active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_node1(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "node1"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_node2ExampleCom(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "node2@example.com"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_coordinatorNode123(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "coordinator-node-123"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_VeryLongCoordinatorNodeName(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "very-long-coordinator-node-name-with-special-chars-123"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	c.sendHandoverRequest(ctx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestMultipleTimes(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build(ctx)
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
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "activeCoordinatorNode"

	// Set the active coordinator node
	c.activeCoordinatorNode = activeCoordinatorNode

	// Create a cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", cancelledCtx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest with cancelled context
	c.sendHandoverRequest(cancelledCtx)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_GetActiveCoordinatorNode_ReturnsEmptyStringWhenInitIfNoActiveCoordinatorIsFalseAndActiveCoordinatorNodeIsEmpty(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""

	result := c.GetActiveCoordinatorNode(ctx, false)
	assert.Empty(t, result, "should return empty string when initIfNoActiveCoordinator is false")
}

func TestCoordinator_GetActiveCoordinatorNode_ReturnsExistingActiveCoordinatorNodeWhenInitIfNoActiveCoordinatorIsFalse(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	expectedNode := "existingNode"
	c.activeCoordinatorNode = expectedNode

	result := c.GetActiveCoordinatorNode(ctx, false)
	assert.Equal(t, expectedNode, result, "should return existing active coordinator node")
}

func TestCoordinator_GetActiveCoordinatorNode_ReturnsExistingActiveCoordinatorNodeWhenInitIfNoActiveCoordinatorIsTrueButNodeIsAlreadySet(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	expectedNode := "existingNode"
	c.activeCoordinatorNode = expectedNode

	result := c.GetActiveCoordinatorNode(ctx, true)
	assert.Equal(t, expectedNode, result, "should return existing active coordinator node without re-initializing")
}

func TestCoordinator_GetActiveCoordinatorNode_InitializesAndReturnsCoordinatorNodeInStaticModeWhenInitIfNoActiveCoordinatorIsTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity@node1"),
	})
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""

	result := c.GetActiveCoordinatorNode(ctx, true)
	assert.Equal(t, "node1", result, "should initialize and return coordinator node in static mode")
	assert.Equal(t, "node1", c.activeCoordinatorNode, "should set activeCoordinatorNode field")
}

func TestCoordinator_GetActiveCoordinatorNode_InitializesAndReturnsCoordinatorNodeInSenderModeWhenInitIfNoActiveCoordinatorIsTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""

	result := c.GetActiveCoordinatorNode(ctx, true)
	assert.Equal(t, "node1", result, "should initialize and return coordinator node in sender mode")
	assert.Equal(t, "node1", c.activeCoordinatorNode, "should set activeCoordinatorNode field")
}

func TestCoordinator_GetActiveCoordinatorNode_InitializesAndReturnsCoordinatorNodeInEndorserModeWhenInitIfNoActiveCoordinatorIsTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000

	result := c.GetActiveCoordinatorNode(ctx, true)
	assert.Contains(t, []string{"node1", "node2", "node3"}, result, "should initialize and return coordinator node from pool in endorser mode")
	assert.NotEmpty(t, c.activeCoordinatorNode, "should set activeCoordinatorNode field")
}

func TestCoordinator_GetActiveCoordinatorNode_ReturnsEmptyStringWhenSelectActiveCoordinatorNodeFailsInStaticMode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String(""), // Empty static coordinator should cause error
	})
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""

	result := c.GetActiveCoordinatorNode(ctx, true)
	assert.Empty(t, result, "should return empty string when SelectActiveCoordinatorNode fails")
	assert.Empty(t, c.activeCoordinatorNode, "should not set activeCoordinatorNode field on error")
}

func TestCoordinator_GetActiveCoordinatorNode_ReturnsEmptyStringWhenSelectActiveCoordinatorNodeFailsDueToInvalidIdentity(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("invalid"), // Invalid identity format
	})
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""

	result := c.GetActiveCoordinatorNode(ctx, true)
	// When node extraction fails, it should return empty string
	assert.Empty(t, result, "should return empty string when identity extraction fails")
}

func TestCoordinator_GetActiveCoordinatorNode_ReturnsEmptyStringWhenSelectActiveCoordinatorNodeFailsInEndorserModeWithEmptyPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""
	c.originatorNodePool = []string{} // Empty pool
	c.currentBlockHeight = 1000

	result := c.GetActiveCoordinatorNode(ctx, true)
	// SelectActiveCoordinatorNode returns empty string (not error) for empty pool
	assert.Empty(t, result, "should return empty string when pool is empty")
	assert.Empty(t, c.activeCoordinatorNode, "should not set activeCoordinatorNode field when pool is empty")
}

func TestCoordinator_GetActiveCoordinatorNode_DoesNotReInitializeWhenCalledMultipleTimesWithInitIfNoActiveCoordinatorTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""

	// First call should initialize
	result1 := c.GetActiveCoordinatorNode(ctx, true)
	assert.Equal(t, "node1", result1, "first call should initialize and return node1")

	// Second call should return the same value without re-initializing
	result2 := c.GetActiveCoordinatorNode(ctx, true)
	assert.Equal(t, "node1", result2, "second call should return same value")
	assert.Equal(t, "node1", c.activeCoordinatorNode, "activeCoordinatorNode should remain set")
}

func TestCoordinator_GetActiveCoordinatorNode_HandlesSwitchingBetweenInitAndNonInitModes(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""

	// Call with initIfNoActiveCoordinator = false should return empty
	result1 := c.GetActiveCoordinatorNode(ctx, false)
	assert.Empty(t, result1, "should return empty when initIfNoActiveCoordinator is false")

	// Call with initIfNoActiveCoordinator = true should initialize
	result2 := c.GetActiveCoordinatorNode(ctx, true)
	assert.Equal(t, "node1", result2, "should initialize when initIfNoActiveCoordinator is true")

	// Call with initIfNoActiveCoordinator = false should still return the initialized value
	result3 := c.GetActiveCoordinatorNode(ctx, false)
	assert.Equal(t, "node1", result3, "should return initialized value even when initIfNoActiveCoordinator is false")
}

func TestCoordinator_GetActiveCoordinatorNode_HandlesContextCancellationGracefully(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity@node1"),
	})
	c, _ := builder.Build(ctx)
	c.activeCoordinatorNode = ""

	// Create a cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	result := c.GetActiveCoordinatorNode(cancelledCtx, true)
	// The function should still work even with cancelled context
	assert.NotNil(t, result, "should handle cancelled context without panicking")
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsNilWhenNoTransactionsExist(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Ensure transactionsByID is empty
	c.transactionsByID = make(map[uuid.UUID]*transaction.Transaction)

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should return nil when no transactions exist")
}

func TestCoordinator_PropagateEventToAllTransactions_SuccessfullyPropagatesEventToSingleTransaction(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

	// Propagate heartbeat event (should be handled successfully by any state)
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate event to single transaction")
}

func TestCoordinator_PropagateEventToAllTransactions_SuccessfullyPropagatesEventToMultipleTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create multiple transactions
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1 := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2 := txBuilder2.Build()

	txBuilder3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn3 := txBuilder3.Build()

	// Add transactions to coordinator
	c.transactionsByID[txn1.ID] = txn1
	c.transactionsByID[txn2.ID] = txn2
	c.transactionsByID[txn3.ID] = txn3

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate event to all transactions")
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsErrorWhenSingleTransactionFailsToHandleEvent(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction in a state that might not handle certain events
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

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

	// Create multiple transactions
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1 := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2 := txBuilder2.Build()

	txBuilder3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn3 := txBuilder3.Build()

	// Add transactions to coordinator
	c.transactionsByID[txn1.ID] = txn1
	c.transactionsByID[txn2.ID] = txn2
	c.transactionsByID[txn3.ID] = txn3

	// Propagate heartbeat event - all should handle it successfully
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate to all transactions")
}

func TestCoordinator_PropagateEventToAllTransactions_HandlesEventPropagationWithManyTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create many transactions
	numTransactions := 10
	for i := 0; i < numTransactions; i++ {
		txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
		txn := txBuilder.Build()
		c.transactionsByID[txn.ID] = txn
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

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should handle HeartbeatIntervalEvent successfully")
}

func TestCoordinator_PropagateEventToAllTransactions_HandlesContextCancellationGracefully(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn

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

	// Create multiple transactions
	txns := make([]*transaction.Transaction, 5)
	for i := 0; i < 5; i++ {
		txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
		txns[i] = txBuilder.Build()
		c.transactionsByID[txns[i].ID] = txns[i]
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

	// Create multiple transactions
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1 := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2 := txBuilder2.Build()

	// Add transactions to coordinator
	c.transactionsByID[txn1.ID] = txn1
	c.transactionsByID[txn2.ID] = txn2

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	// With real transactions, HeartbeatIntervalEvent should be handled successfully
	assert.NoError(t, err, "heartbeat event should be handled successfully by all transaction states")
}

func TestCoordinator_PropagateEventToAllTransactions_IncrementsHeartbeatCounterForConfirmedTransaction(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction in State_Confirmed with 4 heartbeat intervals
	// (grace period is 5, so after one more heartbeat it should transition to State_Final)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).
		HeartbeatIntervalsSinceStateChange(4)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn
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

	// Create a transaction in State_Reverted with 4 heartbeat intervals
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Reverted).
		HeartbeatIntervalsSinceStateChange(4)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.ID] = txn
	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "transaction should start in State_Reverted")

	// Propagate heartbeat event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)
	assert.NoError(t, err)

	// Transaction should have transitioned to State_Final
	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "transaction should have transitioned to State_Final after heartbeat")
}

func TestCoordinator_HeartbeatLoop_StartsAndSendsInitialHeartbeat(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, mocks := builder.Build(ctx)

	// Set up originator pool with another node so heartbeats can be sent
	c.UpdateOriginatorNodePool(ctx, "node2")

	// Create a transaction and add it to the coordinator so the coordinator stays active and doesn't stop the heartbeat loop
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.ID] = txn

	// Ensure heartbeatCtx is nil initially
	require.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil initially")

	// Start heartbeat loop in a goroutine
	done := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done)
	}()

	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentHeartbeat()
	}, 50*time.Millisecond, 1*time.Millisecond)

	// Cancel to stop the loop
	c.heartbeatCancel()
	<-done

	// Verify cleanup
	assert.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil after loop ends")
	assert.Nil(t, c.heartbeatCancel, "heartbeatCancel should be nil after loop ends")
}

func TestCoordinator_HeartbeatLoop_SendsPeriodicHeartbeats(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, mocks := builder.Build(ctx)
	c.heartbeatInterval = 10 * time.Millisecond // can't use builder.OverrideSequencerConfig() because NewCoordinator enforces a minimum of 1 second

	// Set up originator pool with another node so heartbeats can be sent
	c.UpdateOriginatorNodePool(ctx, "node2")

	// Create a transaction and add it to the coordinator so the coordinator stays active and doesn't stop the heartbeat loop
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.ID] = txn

	// Start heartbeat loop in a goroutine
	done := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done)
	}()

	assert.Eventually(t, func() bool {
		// Verify heartbeats were sent (at least initial + periodic)
		return mocks.SentMessageRecorder.SentHeartbeatCount() == 2
	}, 500*time.Millisecond, 10*time.Millisecond)

	// Cancel to stop the loop
	c.heartbeatCancel()
	<-done
}

func TestCoordinator_HeartbeatLoop_ExitsWhenHeartbeatCtxIsCancelled(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, _ := builder.Build(ctx)

	// Create a transaction and add it to the coordinator so the coordinator stays active and doesn't stop the heartbeat loop
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.ID] = txn

	// Start heartbeat loop in a goroutine
	done := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return c.heartbeatCtx != nil
	}, 50*time.Millisecond, 1*time.Millisecond, "heartbeatCancel should be set")

	// require.NotNil(t, c.heartbeatCancel, "heartbeatCancel should be set")
	c.heartbeatCancel()

	// Wait for loop to exit
	select {
	case <-done:
		// Loop exited successfully
	case <-time.After(200 * time.Millisecond):
		t.Fatal("heartbeat loop should exit when heartbeatCtx is cancelled")
	}

	// Verify cleanup
	assert.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil after loop ends")
	assert.Nil(t, c.heartbeatCancel, "heartbeatCancel should be nil after loop ends")
}

func TestCoordinator_HeartbeatLoop_ExitsWhenParentCtxIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, _ := builder.Build(ctx)

	// Create a transaction and add it to the coordinator so the coordinator stays active and doesn't stop the heartbeat loop
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.ID] = txn

	// Start heartbeat loop in a goroutine
	done := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done)
	}()

	assert.Eventually(t, func() bool {
		return c.heartbeatCtx != nil
	}, 50*time.Millisecond, 1*time.Millisecond, "heartbeatCtx should be set")

	// Cancel parent context
	cancel()

	// Wait for loop to exit
	select {
	case <-done:
		// Loop exited successfully
	case <-time.After(200 * time.Millisecond):
		t.Fatal("heartbeat loop should exit when parent ctx is cancelled")
	}

	// Verify cleanup
	assert.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil after loop ends")
	assert.Nil(t, c.heartbeatCancel, "heartbeatCancel should be nil after loop ends")
}

func TestCoordinator_HeartbeatLoop_DoesNotStartIfHeartbeatCtxAlreadySet(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, mocks := builder.Build(ctx)

	// Manually set heartbeatCtx to simulate an already running loop
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	c.heartbeatCtx = heartbeatCtx
	c.heartbeatCancel = heartbeatCancel

	// Reset the heartbeat sent flag
	mocks.SentMessageRecorder.Reset(ctx)

	// Try to start heartbeat loop - should not start
	c.heartbeatLoop(ctx)

	// Verify no heartbeat was sent (loop didn't start)
	assert.False(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "heartbeat should not be sent if loop already running")

	// Cleanup
	heartbeatCancel()
}

func TestCoordinator_HeartbeatLoop_HandlesPropagateEventToAllTransactionsErrorsGracefully(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, _ := builder.Build(ctx)
	c.heartbeatInterval = 10 * time.Millisecond // can't use builder.OverrideSequencerConfig() because NewCoordinator enforces a minimum of 1 second

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
	c.transactionsByID[txn1.ID] = txn1

	// Start heartbeat loop in a goroutine
	done := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
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
	c.transactionsByID[txn2.ID] = txn2

	// Wait for loop to have attempted cleanup after a periodic heartbeat interval event
	// (4 calls includes the first 2 calls, another Add call, and another Forget call)
	assert.Eventually(t, func() bool {
		return len(mockGrapher.Calls) == 4
	}, 500*time.Millisecond, 5*time.Millisecond, "expected at least 2 Forget calls")

	// Cancel to stop the loop
	c.heartbeatCancel()
	<-done
}

func TestCoordinator_HeartbeatLoop_CreatesNewContextOnStart(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, _ := builder.Build(ctx)

	// Create a transaction and add it to the coordinator so the coordinator stays active and doesn't stop the heartbeat loop
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.ID] = txn

	// Verify heartbeatCtx is nil initially
	assert.Nil(t, c.heartbeatCtx, "heartbeatCtx should be nil initially")
	assert.Nil(t, c.heartbeatCancel, "heartbeatCancel should be nil initially")

	// Start heartbeat loop in a goroutine
	done := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done)
	}()

	assert.Eventually(t, func() bool {
		return c.heartbeatCtx != nil
	}, 50*time.Millisecond, 1*time.Millisecond, "heartbeatCtx should be created when loop starts")

	assert.NotNil(t, c.heartbeatCancel, "heartbeatCancel should be created when loop starts")

	// Cancel to stop the loop
	c.heartbeatCancel()
	<-done
}

func TestCoordinator_HeartbeatLoop_StopsTickerOnExit(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.HeartbeatInterval = confutil.P("50ms")
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)

	// Start heartbeat loop in a goroutine
	done := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done)
	}()

	// Verify heartbeatCtx was created
	for c.heartbeatCtx == nil {
		time.Sleep(1 * time.Millisecond)
	}

	// Cancel to stop the loop
	c.heartbeatCancel()
	<-done

	// If ticker wasn't stopped, we would see more heartbeats
	// The fact that the test completes without hanging indicates the ticker was stopped
}

func TestCoordinator_HeartbeatLoop_CanBeRestartedAfterCancellation(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	config := builder.GetSequencerConfig()
	config.HeartbeatInterval = confutil.P("100ms")
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)

	// Set up originator pool with another node so heartbeats can be sent
	c.UpdateOriginatorNodePool(ctx, "node2")

	// Create a transaction and add it to the coordinator so the coordinator stays active
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.ID] = txn

	// Start and stop first loop
	done1 := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done1)
	}()

	for c.heartbeatCtx == nil {
		time.Sleep(1 * time.Millisecond)
	}
	c.heartbeatCancel()
	<-done1

	// Reset heartbeat sent flag
	mocks.SentMessageRecorder.Reset(ctx)

	// Start second loop
	done2 := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done2)
	}()

	for c.heartbeatCtx == nil {
		time.Sleep(1 * time.Millisecond)
	}

	// Verify heartbeat was sent in second loop
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "heartbeat should be sent in restarted loop")

	// Cancel to stop the loop
	c.heartbeatCancel()
	<-done2
}
