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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatortransactionmocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_refreshBlockHeight_SetsEffectiveBlockHeight(t *testing.T) {
	ctx := context.Background()
	o, mocks := NewOriginatorBuilderForTesting(t, State_Idle).
		BlockRange(100).
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(1000))

	o.refreshBlockHeight(ctx)
	assert.Equal(t, int64(1000), o.currentBlockHeight)
	assert.Equal(t, uint64(1000), o.effectiveBlockHeight)
}

func Test_action_UpdateEndorserCandidates_ReplacesCandidates(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("coordinator").
		Build()

	eventNodes := []string{"node1", "node2", "node3"}
	err := action_UpdateEndorserCandidates(ctx, o, &common.EndorserNodesDiscoveredEvent{Nodes: eventNodes})
	require.NoError(t, err)

	assert.Equal(t, eventNodes, o.endorserCandidates)
	assert.Empty(t, o.coordinatorPriorityList, "priority list is computed by action_CalculateCoordinatorPriorities, not this action")
	assert.Equal(t, "coordinator", o.currentActiveCoordinator, "action_UpdateEndorserCandidates must not change currentActiveCoordinator")
}

func Test_action_UpdateActiveCoordinatorFromHeartbeat_SetsCoordinator(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).Build()

	err := action_UpdateActiveCoordinatorFromHeartbeat(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode: "new-coordinator@node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "new-coordinator@node2", o.currentActiveCoordinator)
}

func Test_action_HandleDelegationRejected_HigherPriorityCoordinator_Redirects(t *testing.T) {
	// The rejection names a coordinator that has higher priority (lower index) than the current one.
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	err := action_HandleDelegationRejected(ctx, o, &DelegationRequestRejectedEvent{
		ActiveCoordinator: "node1",
	})
	require.NoError(t, err)

	assert.Equal(t, "node1", o.currentActiveCoordinator, "coordinator must be redirected to the higher-priority node")
}

func Test_action_HandleDelegationRejected_LowerPriorityCoordinator_NoChange(t *testing.T) {
	// The rejection names a coordinator with lower priority than the current one; we ignore it.
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	err := action_HandleDelegationRejected(ctx, o, &DelegationRequestRejectedEvent{
		ActiveCoordinator: "node3",
	})
	require.NoError(t, err)

	assert.Equal(t, "node1", o.currentActiveCoordinator, "coordinator must not change when named node has lower priority")
}

func Test_action_HandleDelegationRejected_NoActiveCoordinator_NoChange(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		Build()

	err := action_HandleDelegationRejected(ctx, o, &DelegationRequestRejectedEvent{
		ActiveCoordinator: "",
	})
	require.NoError(t, err)

	assert.Equal(t, "node1", o.currentActiveCoordinator)
}

func Test_validator_IsHeartbeatSenderLive_ActiveState_ReturnsTrue(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).Build()
	valid, err := validator_IsHeartbeatSenderLive(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode: "coordinator@node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	})
	require.NoError(t, err)
	assert.True(t, valid)
}

func Test_validator_IsHeartbeatSenderLive_PreparedState_ReturnsTrue(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).Build()
	valid, err := validator_IsHeartbeatSenderLive(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode: "coordinator@node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Prepared,
		},
	})
	require.NoError(t, err)
	assert.True(t, valid)
}

func Test_validator_IsHeartbeatSenderLive_ActiveFlushState_ReturnsTrue(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).Build()
	valid, err := validator_IsHeartbeatSenderLive(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode: "coordinator@node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active_Flush,
		},
	})
	require.NoError(t, err)
	assert.True(t, valid)
}

func Test_validator_IsHeartbeatSenderLive_ClosingFlushState_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).Build()
	valid, err := validator_IsHeartbeatSenderLive(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode: "coordinator@node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing_Flush,
		},
	})
	require.NoError(t, err)
	assert.False(t, valid)
}

func Test_validator_IsHeartbeatSenderLive_ClosingState_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).Build()
	valid, err := validator_IsHeartbeatSenderLive(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode: "coordinator@node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing,
		},
	})
	require.NoError(t, err)
	assert.False(t, valid)
}

func Test_hasDroppedTransactions_TrueWhenDelegatedTxnNotInSnapshot(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Delegated)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn).
		Build()
	snapshot := &common.CoordinatorSnapshot{
		PooledTransactions: []*common.SnapshotPooledTransaction{},
	}
	assert.True(t, o.hasDroppedTransactions(ctx, snapshot))
}
func Test_hasDroppedTransactions_FalseWhenDelegatedTxnInSnapshot(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Delegated)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn).
		Build()
	snapshot := &common.CoordinatorSnapshot{
		PooledTransactions: []*common.SnapshotPooledTransaction{
			{ID: txID},
		},
	}
	assert.False(t, o.hasDroppedTransactions(ctx, snapshot))
}
func Test_transactionFoundInSnapshot_TrueWhenInDispatchedTransactions(t *testing.T) {
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	snapshot := &common.CoordinatorSnapshot{
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{
			{SnapshotPooledTransaction: common.SnapshotPooledTransaction{ID: txID}},
		},
		PooledTransactions:    []*common.SnapshotPooledTransaction{},
		ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{},
	}
	assert.True(t, transactionFoundInSnapshot(snapshot, mockTxn))
}
func Test_transactionFoundInSnapshot_TrueWhenInPooledTransactions(t *testing.T) {
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	snapshot := &common.CoordinatorSnapshot{
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{},
		PooledTransactions: []*common.SnapshotPooledTransaction{
			{ID: txID},
		},
		ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{},
	}
	assert.True(t, transactionFoundInSnapshot(snapshot, mockTxn))
}
func Test_transactionFoundInSnapshot_TrueWhenInConfirmedTransactions(t *testing.T) {
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	snapshot := &common.CoordinatorSnapshot{
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{},
		PooledTransactions:     []*common.SnapshotPooledTransaction{},
		ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{
			{SnapshotDispatchedTransaction: common.SnapshotDispatchedTransaction{
				SnapshotPooledTransaction: common.SnapshotPooledTransaction{ID: txID},
			}},
		},
	}
	assert.True(t, transactionFoundInSnapshot(snapshot, mockTxn))
}

func Test_transactionFoundInSnapshot_FalseWhenOnlyOtherTxnsInSnapshot(t *testing.T) {
	txID := uuid.New()
	otherID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	snapshot := &common.CoordinatorSnapshot{
		PooledTransactions: []*common.SnapshotPooledTransaction{
			{ID: otherID},
		},
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{},
		ConfirmedTransactions:  []*common.SnapshotConfirmedTransaction{},
	}
	assert.False(t, transactionFoundInSnapshot(snapshot, mockTxn))
}

func Test_addToTransactions_HandleCreatedEventError_ReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Idle)
	o, _ := builder.Build()
	txnID := uuid.New()
	pt := &components.PrivateTransaction{ID: txnID}
	expectedErr := fmt.Errorf("created event handling failed")
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(expectedErr)
	createTransaction := func(context.Context, *components.PrivateTransaction) (transaction.OriginatorTransaction, error) {
		return mockTxn, nil
	}
	err := o.addToTransactions(ctx, pt, createTransaction)
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}
func Test_sendDelegationRequest_HandleEventError_ReturnsWrappedError(t *testing.T) {
	ctx := context.Background()
	txnID := uuid.New()
	pt := &components.PrivateTransaction{ID: txnID}
	expectedErr := fmt.Errorf("delegated event handling failed")
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetPrivateTransaction").Return(pt)
	mockTxn.On("GetID").Return(txnID)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(expectedErr)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn).
		CurrentActiveCoordinator("coordinator@coordinatorNode").
		Build()
	err := sendDelegationRequest(ctx, o)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error handling delegated event for transaction")
	assert.Contains(t, err.Error(), txnID.String())
	assert.Contains(t, err.Error(), expectedErr.Error())
}
func Test_validator_TransactionDoesNotExist_InvalidEventTypeReturnsFalse(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Observing)
	o, _ := builder.Build()
	valid, err := validator_TransactionDoesNotExist(ctx, o, &common.HeartbeatReceivedEvent{})
	assert.NoError(t, err)
	assert.False(t, valid)
}
func Test_validator_TransactionDoesNotExist_NilTransactionReturnsTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Observing)
	o, _ := builder.Build()
	valid, err := validator_TransactionDoesNotExist(ctx, o, &TransactionCreatedEvent{Transaction: nil})
	assert.NoError(t, err)
	assert.True(t, valid)
}
func Test_validator_TransactionDoesNotExist_TransactionAlreadyExistsReturnsFalse(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).Transactions(mockTxn).Build()
	require.NotNil(t, o.transactionsByID[txID])
	valid, err := validator_TransactionDoesNotExist(ctx, o, &TransactionCreatedEvent{
		Transaction: &components.PrivateTransaction{ID: txID},
	})
	assert.NoError(t, err)
	assert.False(t, valid)
}
func Test_validator_TransactionDoesNotExist_NewTransactionReturnsTrue(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).Build()
	pt := &components.PrivateTransaction{ID: uuid.New()}
	valid, err := validator_TransactionDoesNotExist(ctx, o, &TransactionCreatedEvent{Transaction: pt})
	assert.NoError(t, err)
	assert.True(t, valid)
}
func Test_validator_OriginatorTransactionStateTransitionToFinal(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_OriginatorTransactionStateTransitionToFinal(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{ToState: transaction.State_Final})
	require.NoError(t, err)
	assert.True(t, valid)
	valid, err = validator_OriginatorTransactionStateTransitionToFinal(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{ToState: transaction.State_Confirmed})
	require.NoError(t, err)
	assert.False(t, valid)
}
func Test_validator_OriginatorTransactionStateTransitionToConfirmed(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_OriginatorTransactionStateTransitionToConfirmed(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{ToState: transaction.State_Confirmed})
	require.NoError(t, err)
	assert.True(t, valid)
	valid, err = validator_OriginatorTransactionStateTransitionToConfirmed(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{ToState: transaction.State_Reverted})
	require.NoError(t, err)
	assert.False(t, valid)
}
func Test_validator_OriginatorTransactionStateTransitionToReverted(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_OriginatorTransactionStateTransitionToReverted(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{ToState: transaction.State_Reverted})
	require.NoError(t, err)
	assert.True(t, valid)
	valid, err = validator_OriginatorTransactionStateTransitionToReverted(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{ToState: transaction.State_Final})
	require.NoError(t, err)
	assert.False(t, valid)
}
func Test_guard_InactiveGracePeriodExceeded_WhileSending_TrueWhenCounterExceedsThreshold(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		HeartbeatIntervalsSinceLastReceive(3).
		InactiveGracePeriod(2).
		Build()
	assert.True(t, guard_InactiveGracePeriodExceeded(ctx, o))
}
func Test_guard_InactiveGracePeriodExceeded_WhileSending_FalseWhenCounterBelowThreshold(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		HeartbeatIntervalsSinceLastReceive(1).
		InactiveGracePeriod(2).
		Build()
	assert.False(t, guard_InactiveGracePeriodExceeded(ctx, o))
}

func Test_sendDelegationRequest_TransportError_ReturnsError(t *testing.T) {
	ctx := t.Context()
	builder := NewOriginatorBuilderForTesting(t, State_Sending).WithMockTransportWriter(t)
	txn := testutil.NewPrivateTransactionBuilderForTesting().Build()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txn.ID)
	mockTxn.On("GetPrivateTransaction").Return(txn)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	o, mocks := builder.Transactions(mockTxn).CurrentActiveCoordinator("coordinator@node1").Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, mock.Anything, mock.Anything).
		Return(fmt.Errorf("transport error"))

	err := sendDelegationRequest(ctx, o)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transport error")
}


func Test_action_UpdateEndorserCandidates_DoesNotChangeCurrentActiveCoordinator(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		CoordinatorPriorityList("node1", "node2").
		CurrentActiveCoordinator("node2").
		Build()

	err := action_UpdateEndorserCandidates(ctx, o, &common.EndorserNodesDiscoveredEvent{
		Nodes: []string{"node1", "node2"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, o.coordinatorPriorityList)
	assert.Equal(t, "node2", o.currentActiveCoordinator, "action_UpdateEndorserCandidates must not change currentActiveCoordinator")
}

func Test_action_UpdateEndorserCandidatesFromHeartbeat_AddsNewSenderAndRecomputesPriorityList(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		WithEndorserCandidates("node1").
		CoordinatorPriorityList("node1").
		CurrentActiveCoordinator("node1").
		Build()

	err := action_UpdateEndorserCandidatesFromHeartbeat(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode:            "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{CoordinatorState: common.CoordinatorState_Active},
	})
	require.NoError(t, err)

	assert.Contains(t, o.endorserCandidates, "node2")
	assert.Len(t, o.coordinatorPriorityList, 2, "priority list must reflect the grown pool")
}

func Test_action_UpdateEndorserCandidatesFromHeartbeat_NoOpWhenSenderAlreadyKnown(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		WithEndorserCandidates("node1", "node2").
		CoordinatorPriorityList("node1", "node2").
		CurrentActiveCoordinator("node1").
		Build()

	before := len(o.coordinatorPriorityList)
	err := action_UpdateEndorserCandidatesFromHeartbeat(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode:            "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{CoordinatorState: common.CoordinatorState_Active},
	})
	require.NoError(t, err)

	assert.Len(t, o.endorserCandidates, 2, "duplicate must not be added")
	assert.Len(t, o.coordinatorPriorityList, before, "priority list must be unchanged when pool does not grow")
}

func Test_action_UpdateEndorserCandidatesFromHeartbeat_UsesSnapshotCandidatesWhenPresent(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		WithEndorserCandidates("node1").
		CoordinatorPriorityList("node1").
		CurrentActiveCoordinator("node1").
		Build()

	err := action_UpdateEndorserCandidatesFromHeartbeat(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState:   common.CoordinatorState_Active,
			EndorserCandidates: []string{"node1", "node2", "node3"},
		},
	})
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"node1", "node2", "node3"}, o.endorserCandidates,
		"snapshot candidates must supersede the sender-only candidate list")
}

func Test_action_UpdateEndorserCandidatesFromHeartbeat_NoOpInSenderMode(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).Build() // empty endorserCandidates = SENDER mode

	err := action_UpdateEndorserCandidatesFromHeartbeat(ctx, o, &common.HeartbeatReceivedEvent{
		FromNode:            "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{CoordinatorState: common.CoordinatorState_Active},
	})
	require.NoError(t, err)

	assert.Empty(t, o.endorserCandidates, "endorserCandidates must remain empty in SENDER mode")
}

// action_CalculateCoordinatorPriorities sets currentActiveCoordinator to priorityList[0] on startup
// (the empty-string guard path), and recalibrates failoverIndex.
func Test_action_CalculateCoordinatorPriorities_EmptyActiveCoordinator_SetsTopPriorityAndRecalibrates(t *testing.T) {
	ctx := context.Background()
	// Use "" as the explicit coordinator so the empty-guard branch runs.
	o, _ := NewOriginatorBuilderForTesting(t, State_Initial).
		WithEndorserCandidates("node1", "node2").
		CurrentActiveCoordinator("").
		Build()

	err := action_CalculateCoordinatorPriorities(ctx, o, nil)
	require.NoError(t, err)

	require.NotEmpty(t, o.coordinatorPriorityList)
	assert.Equal(t, o.coordinatorPriorityList[0], o.currentActiveCoordinator,
		"when active coordinator is empty, must initialise to top-priority candidate")
	assert.Equal(t, 1, o.failoverIndex,
		"failoverIndex must be 1 after initialising to top-priority candidate")
}

// ── resetFailoverIndex ──────────────────────────────────────────────────

func Test_resetFailoverIndex_ActiveIsTopPriority_SetsIndexToOne(t *testing.T) {
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CoordinatorPriorityList("A", "B", "C").
		CurrentActiveCoordinator("A").
		FailoverIndex(0).
		Build()

	o.resetFailoverIndex()

	assert.Equal(t, 1, o.failoverIndex, "when active is top priority, next walk step should be index 1")
}

func Test_resetFailoverIndex_ActiveIsNotTopPriority_SetsIndexToZero(t *testing.T) {
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CoordinatorPriorityList("A", "B", "C").
		CurrentActiveCoordinator("C").
		FailoverIndex(2).
		Build()

	o.resetFailoverIndex()

	assert.Equal(t, 0, o.failoverIndex, "when active is not top priority, next walk step should start from index 0")
}

func Test_resetFailoverIndex_EmptyPriorityList_IsNoOp(t *testing.T) {
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		FailoverIndex(5).
		Build()

	o.resetFailoverIndex()

	assert.Equal(t, 5, o.failoverIndex, "empty priority list (STATIC/SENDER mode) must be a no-op")
}

func Test_resetFailoverIndex_SingleNodePriorityList_IsNoOp(t *testing.T) {
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CoordinatorPriorityList("A").
		CurrentActiveCoordinator("A").
		FailoverIndex(3).
		Build()

	o.resetFailoverIndex()

	assert.Equal(t, 3, o.failoverIndex, "single-node pool cannot failover so recalibrate must be a no-op")
}

func Test_resetFailoverIndex_CalledByHandleDelegationRejected_RecalibratesOnRedirect(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CoordinatorPriorityList("A", "B", "C").
		CurrentActiveCoordinator("C").
		FailoverIndex(2).
		Build()

	// Rejection that names a higher-priority coordinator → redirect and recalibrate.
	err := action_HandleDelegationRejected(ctx, o, &DelegationRequestRejectedEvent{ActiveCoordinator: "A"})
	require.NoError(t, err)

	assert.Equal(t, "A", o.currentActiveCoordinator)
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must be recalibrated to 1 after redirect to top priority")
}

func Test_resetFailoverIndex_CalledByHandleDelegationRejected_NoChangeOnLowerPriority(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CoordinatorPriorityList("A", "B", "C").
		CurrentActiveCoordinator("A").
		FailoverIndex(1).
		Build()

	// Rejection names a lower-priority coordinator → no redirect, no recalibrate.
	err := action_HandleDelegationRejected(ctx, o, &DelegationRequestRejectedEvent{ActiveCoordinator: "C"})
	require.NoError(t, err)

	assert.Equal(t, "A", o.currentActiveCoordinator)
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must not change when rejection names lower-priority coordinator")
}

// ── action_FailoverToNextCoordinator ─────────────────────────────────────────

func Test_action_FailoverToNextCoordinator_WithPriorityList_AdvancesCoordinatorAndResetsCounter(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	pt := &components.PrivateTransaction{ID: txID}
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetPrivateTransaction").Return(pt)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		CoordinatorPriorityList("A", "B", "C").
		CurrentActiveCoordinator("A").
		FailoverIndex(1).
		HeartbeatIntervalsSinceLastReceive(5).
		Transactions(mockTxn).
		WithMockTransportWriter(t).
		Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "B", mock.Anything).
		Return(nil).Once()

	err := action_FailoverToNextCoordinator(ctx, o, nil)
	require.NoError(t, err)

	assert.Equal(t, "B", o.currentActiveCoordinator)
	assert.Equal(t, 2, o.failoverIndex)
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness counter must be reset on failover")
}

func Test_action_FailoverToNextCoordinator_WrapAround_CyclesBackToStart(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	pt := &components.PrivateTransaction{ID: txID}
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetPrivateTransaction").Return(pt)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		CoordinatorPriorityList("A", "B", "C").
		CurrentActiveCoordinator("B").
		FailoverIndex(2). // pointing to last slot
		Transactions(mockTxn).
		WithMockTransportWriter(t).
		Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "C", mock.Anything).
		Return(nil).Once()

	err := action_FailoverToNextCoordinator(ctx, o, nil)
	require.NoError(t, err)

	assert.Equal(t, "C", o.currentActiveCoordinator)
	assert.Equal(t, 0, o.failoverIndex, "failoverIndex must wrap to 0 after the last position")
}

func Test_action_FailoverToNextCoordinator_EmptyPriorityList_DelegatesWithoutReset(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	pt := &components.PrivateTransaction{ID: txID}
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetPrivateTransaction").Return(pt)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	o, mocks := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("static-coordinator").
		FailoverIndex(0).
		HeartbeatIntervalsSinceLastReceive(3).
		Transactions(mockTxn).
		WithMockTransportWriter(t).
		Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "static-coordinator", mock.Anything).
		Return(nil).Once()

	err := action_FailoverToNextCoordinator(ctx, o, nil)
	require.NoError(t, err)

	assert.Equal(t, "static-coordinator", o.currentActiveCoordinator, "STATIC/SENDER mode: coordinator must not change")
	assert.Equal(t, 0, o.failoverIndex, "STATIC/SENDER mode: failoverIndex must not change")
	assert.Equal(t, 3, o.heartbeatIntervalsSinceLastReceive, "STATIC/SENDER mode: counter must not be reset")
}

// ── action_ResetToTopPriorityCoordinator ─────────────────────────────────────

func Test_action_ResetToTopPriorityCoordinator_EmptyPriorityList_IsNoOp(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("static-coordinator").
		FailoverIndex(5).
		Build()

	err := action_ResetToTopPriorityCoordinator(ctx, o, nil)
	require.NoError(t, err)

	assert.Equal(t, "static-coordinator", o.currentActiveCoordinator, "must be a no-op with empty priority list")
	assert.Equal(t, 5, o.failoverIndex, "failoverIndex must not change")
}

func Test_action_ResetToTopPriorityCoordinator_ResetsToTopAndRecalibrates(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		CoordinatorPriorityList("A", "B", "C").
		CurrentActiveCoordinator("C").
		FailoverIndex(2).
		Build()

	err := action_ResetToTopPriorityCoordinator(ctx, o, nil)
	require.NoError(t, err)

	assert.Equal(t, "A", o.currentActiveCoordinator, "must reset to priorityList[0]")
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must be 1 after reset to top priority")
}

func Test_action_ResetToTopPriorityCoordinator_AlreadyAtTop_IdempotentAndNoLog(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		CoordinatorPriorityList("A", "B", "C").
		CurrentActiveCoordinator("A").
		FailoverIndex(1).
		Build()

	err := action_ResetToTopPriorityCoordinator(ctx, o, nil)
	require.NoError(t, err)

	assert.Equal(t, "A", o.currentActiveCoordinator, "coordinator must remain at top priority (idempotent)")
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must remain 1 (idempotent)")
}
