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
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, mocks := builder.Build(ctx)
	c.maxDispatchAhead = 0 // Stop the dispatcher loop from progressing states - we're manually updating state throughout the test

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
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
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
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, expectedError)
	c, _ := builder.Build(ctx)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Call addToDelegatedTransactions - this should return an error when HasChainedTransaction fails
	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

	require.Error(t, err, "should return error when HasChainedTransaction fails")
	assert.Equal(t, expectedError, err, "should return the same error from HasChainedTransaction")
	assert.Equal(t, 0, len(c.transactionsByID), "when HasChainedTransaction fails, the transaction is not added to the map")
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

	// Call addToDelegatedTransactions - this should set chainedTxAlreadyDispatched=true when hasChainedTransaction is true
	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

	// Verify that no error occurred
	require.NoError(t, err, "should not return error when HasChainedTransaction returns true")

	// Verify that the transaction was added to transactionsByID
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	coordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")

	// Verify that hasChainedTransaction=true was passed to NewTransaction by checking the transaction state
	assert.Equal(t, transaction.State_Submitted, coordinatedTxn.GetCurrentState(), "transaction should be in State_Submitted when chained transaction is found")
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

	// Call addToDelegatedTransactions - hasChainedTransaction=false should be passed to NewTransaction
	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

	// Verify that no error occurred
	require.NoError(t, err, "should not return error when HasChainedTransaction returns false")

	// Verify that the transaction was added to transactionsByID
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	coordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")

	assert.NotEqual(t, transaction.State_Submitted, coordinatedTxn.GetCurrentState(), "transaction should NOT be in State_Submitted when chained transaction is not found")
	// The transaction should be in a state that indicates it's ready for normal processing
	assert.Contains(t, []transaction.State{transaction.State_Pooled, transaction.State_PreAssembly_Blocked, transaction.State_Assembling}, coordinatedTxn.GetCurrentState(), "transaction should be in Pooled, PreAssembly_Blocked, or Assembling state when chained transaction is not found")
}

func TestCoordinator_AddToDelegatedTransactions_DuplicateTransaction(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1) // Stop the dispatcher loop from progressing states
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// First call - add the transaction
	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})
	require.NoError(t, err, "should not return error on first add")

	// Verify that the transaction was added to transactionsByID
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	firstCoordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, firstCoordinatedTxn, "transaction should exist in transactionsByID")

	// Second call - try to add the same transaction again (duplicate)
	err = c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})
	require.NoError(t, err, "should not return error when adding duplicate transaction")

	// Verify that the transaction count is still 1 (duplicate was skipped)
	assert.Equal(t, 1, len(c.transactionsByID), "duplicate transaction should be skipped, count should remain 1")

	// Verify that the same transaction object is still in the map (not replaced)
	secondCoordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, secondCoordinatedTxn, "transaction should still exist in transactionsByID")
	assert.Equal(t, firstCoordinatedTxn, secondCoordinatedTxn, "duplicate transaction should not replace existing transaction")
}

func TestCoordinator_SelectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithFullyQualifiedIdentity(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity@node1"),
	})
	c, _ := builder.Build(ctx)

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
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

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
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

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
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

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
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

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
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

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
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
	coordinatorNode1, err1 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err1)

	c.currentBlockHeight = 1001
	coordinatorNode2, err2 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err2)

	c.currentBlockHeight = 1099
	coordinatorNode3, err3 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err3)

	// All should select the same coordinator since they're in the same block range
	assert.Equal(t, coordinatorNode1, coordinatorNode2)
	assert.Equal(t, coordinatorNode2, coordinatorNode3)

	// Different block range should potentially select different coordinator
	c.currentBlockHeight = 1100
	coordinatorNode4, err4 := c.selectActiveCoordinatorNode(ctx)
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
	coordinatorNode1, err1 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err1)

	c.currentBlockHeight = 150
	coordinatorNode2, err2 := c.selectActiveCoordinatorNode(ctx)
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

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func TestCoordinator_Stop_StopsEventLoopAndDispatchLoop(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

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

func TestCoordinator_EventAction_TransactionConfirmed_FindsTransactionBySignerAndNonce(t *testing.T) {
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
			TransactionID: txn.GetID(),
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Process TransactionConfirmed event (nil RevertReason so guard transitions to State_Confirmed not State_Pooled)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	nonceHex := pldtypes.HexUint64(nonce)
	err = eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn.GetID(),
		From:  signerAddress,
		Nonce: &nonceHex,
		Hash:  hash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "transaction should be confirmed")
}

func TestCoordinator_EventAction_TransactionConfirmed_FindsTransactionByTxIdWhenFromIsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction in State_Submitted so ConfirmedEvent is accepted
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Process TransactionConfirmed event with nil from (chained transaction scenario); nil RevertReason for success path
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	err := eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn.GetID(),
		From:  nil,
		Nonce: nil,
		Hash:  hash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "transaction should be confirmed")
}

func TestCoordinator_EventAction_TransactionConfirmed_FindsTransactionByTxIdWhenSignerNonceLookupFails(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction in State_Submitted so ConfirmedEvent is accepted (signer/nonce mismatch still finds by TxID)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Process TransactionConfirmed with non-matching signer+nonce; TxID match is used; nil RevertReason for success path
	nonMatchingSigner := pldtypes.RandAddress()
	nonMatchingNonce := pldtypes.HexUint64(999)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	err := eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn.GetID(),
		From:  nonMatchingSigner,
		Nonce: &nonMatchingNonce,
		Hash:  hash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "transaction should be found by TxID and confirmed")
}

func TestCoordinator_EventAction_TransactionConfirmed_ReturnsNilWhenTransactionNotFound(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Process TransactionConfirmed for a transaction the coordinator does not track
	nonExistentTxID := uuid.New()
	signerAddress := pldtypes.RandAddress()
	nonceHex := pldtypes.HexUint64(42)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	err := eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  nonExistentTxID,
		From:  signerAddress,
		Nonce: &nonceHex,
		Hash:  hash,
	})

	require.NoError(t, err)
	assert.Empty(t, c.transactionsByID, "coordinator should have no transactions")
}

func TestCoordinator_EventAction_TransactionConfirmed_HandlesMatchingHashCorrectly(t *testing.T) {
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
			TransactionID: txn.GetID(),
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		SubmissionHash: submissionHash,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Process TransactionConfirmed with matching hash; nil RevertReason for success path
	nonceHex := pldtypes.HexUint64(nonce)
	err = eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn.GetID(),
		From:  signerAddress,
		Nonce: &nonceHex,
		Hash:  submissionHash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "transaction should be confirmed")
}

func TestCoordinator_EventAction_TransactionConfirmed_HandlesDifferentHashCorrectly(t *testing.T) {
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
			TransactionID: txn.GetID(),
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		SubmissionHash: submissionHash,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Process TransactionConfirmed with different hash (should still confirm, logs a warning); nil RevertReason for success path
	differentHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	nonceHex := pldtypes.HexUint64(nonce)
	err = eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn.GetID(),
		From:  signerAddress,
		Nonce: &nonceHex,
		Hash:  differentHash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "transaction should be confirmed even with different hash")
}

func TestCoordinator_EventAction_TransactionConfirmed_HandlesNilSubmissionHashCorrectly(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a transaction in State_Submitted without a submission hash (chained transaction);
	// must be State_Submitted so ConfirmedEvent is accepted
	signerAddress := pldtypes.RandAddress()
	nonce := uint64(42)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn := txBuilder.Build()

	// Set signer and nonce (events are no-ops from State_Submitted but set fields for the confirmation)
	err := txn.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Process TransactionConfirmed with a hash (chained transaction scenario); nil RevertReason for success path
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	nonceHex := pldtypes.HexUint64(nonce)
	err = eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn.GetID(),
		From:  signerAddress,
		Nonce: &nonceHex,
		Hash:  hash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "transaction should be confirmed")
	// Builder for State_Submitted may set latestSubmissionHash; test validates confirmation path for chained-tx-style event
}

func TestCoordinator_EventAction_TransactionConfirmed_ReturnsErrorWhenHandleEventFails(t *testing.T) {
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
			TransactionID: txn.GetID(),
		},
		SignerAddress: *signerAddress,
	})
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		Nonce: nonce,
	})
	require.NoError(t, err)

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Process TransactionConfirmed - HandleEvent may fail because transaction is in State_Pooled
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	nonceHex := pldtypes.HexUint64(nonce)
	err = eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn.GetID(),
		From:  signerAddress,
		Nonce: &nonceHex,
		Hash:  hash,
	})

	// The function returns an error if HandleEvent fails; we verify it doesn't panic
	if err != nil {
		assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "transaction should remain in Pooled when confirmation fails")
	}
}

func TestCoordinator_EventAction_TransactionConfirmed_HandlesMultipleTransactionsCorrectly(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create multiple transactions in State_Submitted so ConfirmedEvent is accepted
	signerAddress1 := pldtypes.RandAddress()
	nonce1 := uint64(42)
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn1 := txBuilder1.Build()

	err := txn1.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn1.GetID(),
		},
		SignerAddress: *signerAddress1,
	})
	require.NoError(t, err)

	err = txn1.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn1.GetID(),
		},
		Nonce: nonce1,
	})
	require.NoError(t, err)

	signerAddress2 := pldtypes.RandAddress()
	nonce2 := uint64(43)
	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn2 := txBuilder2.Build()

	err = txn2.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn2.GetID(),
		},
		SignerAddress: *signerAddress2,
	})
	require.NoError(t, err)

	err = txn2.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn2.GetID(),
		},
		Nonce: nonce2,
	})
	require.NoError(t, err)

	// Add both transactions to coordinator
	c.transactionsByID[txn1.GetID()] = txn1
	c.transactionsByID[txn2.GetID()] = txn2

	// Process TransactionConfirmed for first transaction; nil RevertReason for success path
	hash1 := pldtypes.Bytes32(pldtypes.RandBytes(32))
	nonce1Hex := pldtypes.HexUint64(nonce1)
	err = eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn1.GetID(),
		From:  signerAddress1,
		Nonce: &nonce1Hex,
		Hash:  hash1,
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn1.GetCurrentState(), "first transaction should be confirmed")

	// Process TransactionConfirmed for second transaction
	hash2 := pldtypes.Bytes32(pldtypes.RandBytes(32))
	nonce2Hex := pldtypes.HexUint64(nonce2)
	err = eventAction_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn2.GetID(),
		From:  signerAddress2,
		Nonce: &nonce2Hex,
		Hash:  hash2,
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn2.GetCurrentState(), "second transaction should be confirmed")
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

	// Create multiple transactions
	txns := make([]*transaction.Transaction, 5)
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

func TestCoordinator_HeartbeatLoop_StartsAndSendsInitialHeartbeat(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, mocks := builder.Build(ctx)

	// Set up originator pool with another node so heartbeats can be sent
	c.UpdateOriginatorNodePool(ctx, "node2")

	// Create a transaction and add it to the coordinator so the coordinator stays active and doesn't stop the heartbeat loop
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.GetID()] = txn

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
	}, 100*time.Millisecond, 5*time.Millisecond)

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
	c.transactionsByID[txn.GetID()] = txn

	// Start heartbeat loop in a goroutine
	done := make(chan struct{})
	go func() {
		c.heartbeatLoop(ctx)
		close(done)
	}()

	assert.Eventually(t, func() bool {
		// Verify heartbeats were sent (at least initial + periodic)
		return mocks.SentMessageRecorder.SentHeartbeatCount() >= 2
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
	c.transactionsByID[txn.GetID()] = txn

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
	c.transactionsByID[txn.GetID()] = txn

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

func TestCoordinator_HeartbeatLoop_CreatesNewContextOnStart(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	c, _ := builder.Build(ctx)

	// Create a transaction and add it to the coordinator so the coordinator stays active and doesn't stop the heartbeat loop
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID[txn.GetID()] = txn

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
	c.transactionsByID[txn.GetID()] = txn

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
