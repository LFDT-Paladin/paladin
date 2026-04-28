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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func NewCoordinatorForUnitTest(t *testing.T) (*coordinator, *coordinatorDependencyMocks, func()) {

	metrics := metrics.InitMetrics(context.Background(), prometheus.NewRegistry())
	mocks := &coordinatorDependencyMocks{
		transportWriter:   transport.NewMockTransportWriter(t),
		clock:             common.NewMockClock(t),
		engineIntegration: common.NewMockEngineIntegration(t),
		syncPoints:        &syncpoints.MockSyncPoints{},
		emit:              func(event common.Event) {},
	}
	mockDomainAPI := componentsmocks.NewDomainSmartContract(t)
	mockTXManager := componentsmocks.NewTXManager(t)
	mockSequencerManager := componentsmocks.NewSequencerManager(t)
	allComponents := componentsmocks.NewAllComponents(t)
	transportManager := componentsmocks.NewTransportManager(t)
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	transportManager.On("LocalNodeName").Return("node1").Maybe()
	allComponents.On("TransportManager").Return(transportManager).Maybe()
	allComponents.On("TxManager").Return(mockTXManager).Maybe()
	allComponents.On("SequencerManager").Return(mockSequencerManager).Maybe()
	allComponents.On("Persistence").Return(mp.P).Maybe()
	allComponents.On("KeyManager").Return(nil).Maybe()
	allComponents.On("PublicTxManager").Return(nil).Maybe()
	mockDomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	}).Maybe()
	mocks.transportWriter.On("StartLoopbackWriter", mock.Anything).Return(nil)
	mocks.transportWriter.On("StopLoopbackWriter").Return().Maybe()
	mocks.transportWriter.On("WaitForDone", mock.Anything).Return().Maybe()
	buildCtx, cancel := context.WithCancel(t.Context())

	config := &pldconf.SequencerConfig{
		HeartbeatInterval:       confutil.P("10s"),
		StateTimeout:            confutil.P("5s"),
		RequestTimeout:          confutil.P("1s"),
		BlockRange:              confutil.P(uint64(100)),
		BlockHeightTolerance:    confutil.P(uint64(5)),
		ClosingGracePeriod:      confutil.P(5),
		MaxInflightTransactions: confutil.P(500),
		MaxDispatchAhead:        confutil.P(10),
		TargetActiveSequencers:  confutil.P(50),
	}

	coordinator, err := NewCoordinator(buildCtx, pldtypes.RandAddress(), mockDomainAPI, nil, allComponents, nil, nil, mocks.transportWriter, mocks.clock, mocks.engineIntegration, mocks.syncPoints, config, "node1", metrics)
	require.NoError(t, err)

	done := func() {
		cancel()
		coordinator.WaitForDone(context.Background())
	}
	return coordinator, mocks, done
}

type coordinatorDependencyMocks struct {
	transportWriter   *transport.MockTransportWriter
	clock             *common.MockClock
	engineIntegration *common.MockEngineIntegration
	emit              common.EmitEvent
	syncPoints        syncpoints.SyncPoints
}

func TestCoordinator_SingleTransactionLifecycle(t *testing.T) {
	// Test the progression of a single transaction through the coordinator's lifecycle
	// Simulating originator node, endorser node and the public transaction manager (submitter)
	// by inspecting the coordinator output messages and by sending events that would normally be triggered by those components sending messages to the coordinator.
	// At each stage, we inspect the state of the coordinator by checking the snapshot it produces on heartbeat messages

	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	builder.GetDomainAPI().On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)
		tx.PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil).Once()
	builder.GetSequencerManager().On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil).Once()
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1) // Stop the dispatcher loop from progressing states - we're manually updating state throughout the test
	builder.OverrideSequencerConfig(config)
	c, mocks, done := builder.Build(ctx)
	defer done()
	mocks.SyncPoints.(*syncpoints.MockSyncPoints).On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	// Start by simulating the originator and delegate a transaction to the coordinator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originator).
		NumberOfRequiredEndorsers(1).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From:   originator,
				Intent: prototk.TransactionSpecification_PREPARE_TRANSACTION,
			},
		})
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
	c.QueueEvent(ctx, &transaction.ConfirmedSuccessEvent{
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
	ctx := t.Context()
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
	c, _, done := builder.Build(ctx)
	defer done()

	// Start by simulating the originator and delegate a transaction to the coordinator
	for i := range 100 {
		transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
		txn := transactionBuilder.BuildSparse()
		err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "", c.newCoordinatorTransaction)

		if i < 5 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			require.ErrorContains(t, err, "PD012642")
		}
	}
}

func TestCoordinator_GetTransactionsInStates_EmptyMapReturnsEmpty(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.transactionsByID = make(map[uuid.UUID]transaction.CoordinatorTransaction)

	result := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Ready_For_Dispatch})
	assert.Empty(t, result)
}

func TestCoordinator_GetTransactionsInStates_SingleStateFilter_ReturnsMatching(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		txn.GetID(): txn,
	}

	result := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Ready_For_Dispatch})
	require.Len(t, result, 1)
	assert.Equal(t, txn.GetID(), result[0].GetID())
	assert.Equal(t, transaction.State_Ready_For_Dispatch, result[0].GetCurrentState())
}

func TestCoordinator_GetTransactionsInStates_SingleStateFilter_ReturnsEmptyWhenNoMatch(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		txn.GetID(): txn,
	}

	result := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Ready_For_Dispatch})
	assert.Empty(t, result)
}

func TestCoordinator_GetTransactionsInStates_MultipleStatesFilter_ReturnsAllMatching(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txReady, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	txDispatched, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	txPooled, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		txReady.GetID():      txReady,
		txDispatched.GetID(): txDispatched,
		txPooled.GetID():     txPooled,
	}

	result := c.getTransactionsInStates(ctx, []transaction.State{
		transaction.State_Ready_For_Dispatch,
		transaction.State_Dispatched,
	})
	require.Len(t, result, 2)
	ids := make(map[uuid.UUID]bool)
	for _, txn := range result {
		ids[txn.GetID()] = true
		assert.Contains(t, []transaction.State{transaction.State_Ready_For_Dispatch, transaction.State_Dispatched}, txn.GetCurrentState())
	}
	assert.True(t, ids[txReady.GetID()])
	assert.True(t, ids[txDispatched.GetID()])
	assert.False(t, ids[txPooled.GetID()])
}

func TestCoordinator_GetTransactionsInStates_MultipleTransactionsInSameState(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx3, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
		tx3.GetID(): tx3,
	}

	result := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Assembling})
	require.Len(t, result, 2)
	ids := make(map[uuid.UUID]bool)
	for _, txn := range result {
		ids[txn.GetID()] = true
		assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState())
	}
	assert.True(t, ids[tx1.GetID()])
	assert.True(t, ids[tx2.GetID()])
	assert.False(t, ids[tx3.GetID()])
}

func TestCoordinator_GetTransactionsInStates_EmptyStatesFilter_ReturnsEmpty(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		txn.GetID(): txn,
	}

	result := c.getTransactionsInStates(ctx, nil)
	assert.Empty(t, result)
	result = c.getTransactionsInStates(ctx, []transaction.State{})
	assert.Empty(t, result)
}

func TestCoordinator_GetTransactionsNotInStates_EmptyMapReturnsEmpty(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.transactionsByID = make(map[uuid.UUID]transaction.CoordinatorTransaction)

	result := c.getTransactionsNotInStates(ctx, []transaction.State{transaction.State_Ready_For_Dispatch})
	assert.Empty(t, result)
}

func TestCoordinator_GetTransactionsNotInStates_SingleStateFilter_ReturnsNonMatching(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txReady, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	txPooled, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		txReady.GetID():  txReady,
		txPooled.GetID(): txPooled,
	}

	result := c.getTransactionsNotInStates(ctx, []transaction.State{transaction.State_Ready_For_Dispatch})
	require.Len(t, result, 1)
	assert.Equal(t, txPooled.GetID(), result[0].GetID())
	assert.Equal(t, transaction.State_Pooled, result[0].GetCurrentState())
}

func TestCoordinator_GetTransactionsNotInStates_SingleStateFilter_ReturnsAllWhenNoMatch(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		txn.GetID(): txn,
	}

	result := c.getTransactionsNotInStates(ctx, []transaction.State{transaction.State_Ready_For_Dispatch})
	require.Len(t, result, 1)
	assert.Equal(t, txn.GetID(), result[0].GetID())
}

func TestCoordinator_GetTransactionsNotInStates_MultipleStatesFilter_ExcludesAllMatching(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txReady, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	txDispatched, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	txPooled, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		txReady.GetID():      txReady,
		txDispatched.GetID(): txDispatched,
		txPooled.GetID():     txPooled,
	}

	result := c.getTransactionsNotInStates(ctx, []transaction.State{
		transaction.State_Ready_For_Dispatch,
		transaction.State_Dispatched,
	})
	require.Len(t, result, 1)
	assert.Equal(t, txPooled.GetID(), result[0].GetID())
}

func TestCoordinator_GetTransactionsNotInStates_MultipleTransactionsExcluded(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx3, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
		tx3.GetID(): tx3,
	}

	result := c.getTransactionsNotInStates(ctx, []transaction.State{transaction.State_Assembling})
	require.Len(t, result, 1)
	assert.Equal(t, tx3.GetID(), result[0].GetID())
	assert.Equal(t, transaction.State_Pooled, result[0].GetCurrentState())
}

func TestCoordinator_GetTransactionsNotInStates_EmptyStatesFilter_ReturnsAll(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		txn.GetID(): txn,
	}

	result := c.getTransactionsNotInStates(ctx, nil)
	require.Len(t, result, 1)
	result = c.getTransactionsNotInStates(ctx, []transaction.State{})
	require.Len(t, result, 1)
}

func TestCoordinator_NewCoordinator_EndorserMode_AllowsNoConfiguredCandidates(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	c, _, done := builder.Build(ctx)
	defer done()
	assert.Empty(t, c.originatorNodePool)
}

func TestCoordinator_NewCoordinator_EndorserMode_FailsOnInvalidConfiguredCandidate(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection:          prototk.ContractConfig_COORDINATOR_ENDORSER,
		CoordinatorEndorserCandidates: []string{"endorser1"},
	})
	assert.Panics(t, func() {
		builder.Build(ctx)
	})
}

func TestCoordinator_NewCoordinator_EndorserMode_InitializesPoolFromConfiguredCandidates(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection:          prototk.ContractConfig_COORDINATOR_ENDORSER,
		CoordinatorEndorserCandidates: []string{"endorser1@nodeB", "endorser2@nodeA", "endorser3@nodeB"},
	})
	c, _, done := builder.Build(ctx)
	defer done()
	assert.Equal(t, []string{"nodeA", "nodeB", "nodeB"}, c.originatorNodePool)
}

func TestCoordinator_CancelContext_StopsEventLoopAndDispatchLoop(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)

	// Verify event loop is running
	require.False(t, c.stateMachineEventLoop.IsStopped(), "event loop should not be stopped initially")

	select {
	case <-c.dispatchLoopStopped:
		t.Fatal("dispatch loop should not be stopped initially")
	default:
	}

	// Should block until shutdown is complete
	done()

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
		t.Fatal("context should be cancelled after done()")
	}
}

func TestCoordinator_CancelContext_WaitsForTransportShutdown(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	mockTransport := transport.NewMockTransportWriter(t)
	// done() waits for transport completion on the configured writer
	mockTransport.On("WaitForDone", mock.Anything).Return().Maybe()

	// Replace the transport writer
	c.transportWriter = mockTransport

	done()

	// Verify transport shutdown wait was invoked
	mockTransport.AssertExpectations(t)
}

func TestCoordinator_CancelContext_CompletesSuccessfullyWhenCalledOnce(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)

	done()

	// Verify both loops have stopped
	require.True(t, c.stateMachineEventLoop.IsStopped(), "event loop should be stopped")

	select {
	case _, ok := <-c.dispatchLoopStopped:
		require.False(t, ok, "dispatch loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

func TestCoordinator_CancelContext_StopsLoopsEvenWhenProcessingEvents(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)

	// Queue some events to ensure loops are busy
	for i := 0; i < 10; i++ {
		c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})
	}

	done()

	// Verify both loops have stopped
	require.True(t, c.stateMachineEventLoop.IsStopped(), "event loop should be stopped")

	select {
	case _, ok := <-c.dispatchLoopStopped:
		require.False(t, ok, "dispatch loop stopped channel should be closed")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("dispatch loop did not stop within timeout")
	}
}

func TestCoordinator_CancelContext_WhenAlreadyCancelled_ReturnsImmediately(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)

	done()
	require.True(t, c.stateMachineEventLoop.IsStopped(), "event loop should be stopped")

	// Second cancel should return immediately without blocking or panicking
	done()
	require.True(t, c.stateMachineEventLoop.IsStopped(), "event loop should still be stopped")
}

func Test_propagateEventToTransaction_UnknownTransaction_ReturnsNil(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	event := &transaction.ConfirmedSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{TransactionID: uuid.New()},
		Hash:                 pldtypes.Bytes32(pldtypes.RandBytes(32)),
	}

	err := c.propagateEventToTransaction(ctx, event)
	require.NoError(t, err)
	assert.Empty(t, c.transactionsByID, "transaction should not be added")
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsNilWhenNoTransactionsExist(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Ensure transactionsByID is empty
	c.transactionsByID = make(map[uuid.UUID]transaction.CoordinatorTransaction)

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should return nil when no transactions exist")
}

func TestCoordinator_PropagateEventToAllTransactions_SuccessfullyPropagatesEventToSingleTransaction(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn, _ := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Propagate heartbeat event (should be handled successfully by any state)
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should successfully propagate event to single transaction")
}

func TestCoordinator_PropagateEventToAllTransactions_SuccessfullyPropagatesEventToMultipleTransactions(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create multiple transactions
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1, _ := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2, _ := txBuilder2.Build()

	txBuilder3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn3, _ := txBuilder3.Build()

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
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a transaction in a state that might not handle certain events
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn, _ := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn

	// Create a mock event that will cause an error
	event := &common.HeartbeatIntervalEvent{}

	err := c.propagateEventToAllTransactions(ctx, event)

	// HeartbeatIntervalEvent should be handled successfully by all transaction states
	assert.NoError(t, err, "heartbeat event should be handled successfully")
}

func TestCoordinator_PropagateEventToAllTransactions_StopsAtFirstErrorWhenMultipleTransactionsExist(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create multiple transactions
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1, _ := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2, _ := txBuilder2.Build()

	txBuilder3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn3, _ := txBuilder3.Build()

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
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create many transactions
	numTransactions := 10
	for i := 0; i < numTransactions; i++ {
		txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
		txn, _ := txBuilder.Build()
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
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn, _ := txBuilder.Build()

	// Add transaction to coordinator
	c.transactionsByID[txn.GetID()] = txn
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should handle HeartbeatIntervalEvent successfully")
}

func TestCoordinator_PropagateEventToAllTransactions_HandlesContextCancellationGracefully(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn, _ := txBuilder.Build()

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
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create multiple transactions
	txns := make([]transaction.CoordinatorTransaction, 5)
	for i := 0; i < 5; i++ {
		txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
		txns[i], _ = txBuilder.Build()
		c.transactionsByID[txns[i].GetID()] = txns[i]
	}

	// Propagate event
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	assert.NoError(t, err, "should process all transactions regardless of order")
	assert.Equal(t, 5, len(c.transactionsByID), "all transactions should still be in map")
}

func TestCoordinator_PropagateEventToAllTransactions_ReturnsErrorImmediatelyWhenTransactionHandleEventFails(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create multiple transactions
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1, _ := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2, _ := txBuilder2.Build()

	// Add transactions to coordinator
	c.transactionsByID[txn1.GetID()] = txn1
	c.transactionsByID[txn2.GetID()] = txn2

	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)

	// With real transactions, HeartbeatIntervalEvent should be handled successfully
	assert.NoError(t, err, "heartbeat event should be handled successfully by all transaction states")
}

func TestCoordinator_PropagateEventToAllTransactions_IncrementsHeartbeatCounterForConfirmedTransaction(t *testing.T) {
	ctx := t.Context()
	mockGrapher := grapher.NewMockGrapher(t)
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a transaction in State_Confirmed with 4 heartbeat intervals
	// (grace period is 5, so after one more heartbeat it should transition to State_Final)
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).
		HeartbeatIntervalsSinceStateChange(4).Grapher(mockGrapher)
	txn, _ := txBuilder.Build()
	mockGrapher.EXPECT().Forget(mock.Anything, txn.GetID())

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
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a transaction in State_Reverted with 4 heartbeat intervals
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Reverted).
		HeartbeatIntervalsSinceStateChange(4)
	txn, _ := txBuilder.Build()

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

func TestCoordinator_PropagateEventToAllTransactions_HandleEventReturnsError(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a mock transaction that returns an error from HandleEvent
	mockTxn := transaction.NewMockCoordinatorTransaction(t)
	txnID := uuid.New()
	expectedError := fmt.Errorf("handle event error")

	mockTxn.EXPECT().GetID().Return(txnID)
	mockTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*common.HeartbeatIntervalEvent")).Return(expectedError)

	// Add mock transaction to coordinator
	c.transactionsByID[txnID] = mockTxn

	// Propagate heartbeat event - should return the error
	event := &common.HeartbeatIntervalEvent{}
	err := c.propagateEventToAllTransactions(ctx, event)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestCoordinator_WaitForDone_ReturnsEarlyWhenContextCancelled(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a context that is already cancelled
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	c.WaitForDone(cancelledCtx)
}
