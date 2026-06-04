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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatortransactionmocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ── State_Initial ─────────────────────────────────────────────────────────────

// OriginatorCreatedEvent from Initial transitions unconditionally to Idle.
func TestStateMachine_Initial_OriginatorCreated_TransitionsToIdle(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Initial).Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &OriginatorCreatedEvent{}))

	assert.Equal(t, State_Idle, o.GetCurrentState())
	// currentActiveCoordinator is set during Start() → initializeFromContractConfig, not here.
	// So the field value is whatever was set on the builder (default "coordinator").
	assert.NotEmpty(t, o.currentActiveCoordinator)
}

// ── State_Idle ────────────────────────────────────────────────────────────────

// OnTransitionTo in Idle (ENDORSER mode): entering Idle resets currentActiveCoordinator to the
// top-priority candidate and sets failoverIndex=1. Triggered here via Observing→Idle transition.
func TestStateMachine_Idle_OnTransitionTo_EndorserMode_ResetsToTopPriorityAndSetsFailoverIndex(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("C").
		CoordinatorPriorityList("A", "B", "C").
		HeartbeatIntervalsSinceLastReceive(1).
		InactiveGracePeriod(2).
		Build()

	// One more HeartbeatInterval exceeds the grace period → transitions to Idle.
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))

	assert.Equal(t, State_Idle, o.GetCurrentState())
	assert.Equal(t, "A", o.currentActiveCoordinator, "must reset to priorityList[0] on entering Idle")
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must be 1 when active is at position 0")
}

// OnTransitionTo in Idle (no endorser mode / empty priority list): entering Idle is a no-op;
// currentActiveCoordinator and failoverIndex are unchanged.
func TestStateMachine_Idle_OnTransitionTo_NoEndorserMode_NoChange(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("coordinator@node1").
		HeartbeatIntervalsSinceLastReceive(1).
		InactiveGracePeriod(2).
		FailoverIndex(3).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))

	assert.Equal(t, State_Idle, o.GetCurrentState())
	assert.Equal(t, "coordinator@node1", o.currentActiveCoordinator, "must not change coordinator with no priority list")
	assert.Equal(t, 3, o.failoverIndex, "failoverIndex must not change when priority list is empty")
}

// NewBlock on an epoch boundary while already Idle resets to top-priority coordinator, giving a
// fresh start for any subsequent Sending entry.
func TestStateMachine_Idle_NewBlock_EpochBoundary_EndorserMode_ResetsToTopPriority(t *testing.T) {
	ctx := context.Background()
	// blockRange=10, currentHeight=5 → epoch 0; newHeight=10 → epoch 1 ⇒ boundary.
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("C").
		CoordinatorPriorityList("A", "B", "C").
		CurrentBlockHeight(5).
		BlockRange(10).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.NewBlockEvent{BlockHeight: 10}))

	assert.Equal(t, State_Idle, o.GetCurrentState())
	assert.Equal(t, "A", o.currentActiveCoordinator, "must reset to top priority on epoch boundary while idle")
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must be 1 after reset to top priority")
}

// HeartbeatReceived in Idle from a node in Active state → Observing; coordinator updated; timer reset;
// failoverIndex recalibrated (index=1 since new coordinator is at priorityList[0]).
func TestStateMachine_Idle_HeartbeatReceived_ActiveState_TransitionsToObserving(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("old-coordinator").
		CoordinatorPriorityList("node2", "node1").
		NodeName("node1")
	o, _ := builder.Build()
	o.heartbeatIntervalsSinceLastReceive = 5
	ca := builder.GetContractAddress()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "node2",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))

	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, "node2", o.currentActiveCoordinator, "coordinator must be updated to the Active heartbeat sender")
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness timer must be reset")
	// node2 is at index 0 of the priority list; recalibrate sets failoverIndex=1
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must be recalibrated: 1 when new active is top priority")
}

// HeartbeatReceived in Idle from a node in Elect state → Observing; Elect is a liveness-proving state.
func TestStateMachine_Idle_HeartbeatReceived_ElectState_TransitionsToObserving(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("existing-coordinator").
		NodeName("node1")
	o, _ := builder.Build()
	ca := builder.GetContractAddress()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "node2",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Elect,
		},
	}))

	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, "node2", o.currentActiveCoordinator, "coordinator must be updated to the Elect heartbeat sender")
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness timer must be reset")
}

// HeartbeatReceived in Idle from a node in Closing_Flush/Closing/Idle state → no state change.
func TestStateMachine_Idle_HeartbeatReceived_NonLiveState_StaysIdle(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("existing-coordinator").
		NodeName("node1")
	o, _ := builder.Build()
	ca := builder.GetContractAddress()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "node2",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing_Flush,
		},
	}))

	assert.Equal(t, State_Idle, o.GetCurrentState())
	assert.Equal(t, "existing-coordinator", o.currentActiveCoordinator, "coordinator must not change on non-live heartbeat in Idle")
}

// TransactionCreated in Idle → Sending; delegation request sent.
func TestStateMachine_Idle_TransactionCreated_TransitionsToSending_SendsDelegationRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("coordinator@node1").
		WithMockTransportWriter(t)
	o, mocks := builder.Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "coordinator@node1", mock.Anything, mock.Anything).
		Return(nil).Once()

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator("sender@node1").Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &TransactionCreatedEvent{Transaction: txn}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Len(t, o.transactionsByID, 1, "transaction must be tracked")
}

// Duplicate TransactionCreated (same ID) in Idle → no state change; no double tracking.
func TestStateMachine_Idle_TransactionCreated_DuplicateID_StaysIdle(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		Transactions(mockTxn).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: &components.PrivateTransaction{ID: txID},
	}))

	assert.Equal(t, State_Idle, o.GetCurrentState())
	assert.Len(t, o.transactionsByID, 1, "duplicate transaction must not be double-tracked")
}

// NewBlock event in Idle updates currentBlockHeight; stays Idle.
func TestStateMachine_Idle_NewBlock_UpdatesBlockHeight_StaysIdle(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).CurrentBlockHeight(100).Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.NewBlockEvent{BlockHeight: 200}))

	assert.Equal(t, State_Idle, o.GetCurrentState())
	assert.Equal(t, uint64(200), o.currentBlockHeight)
}

// EndorserNodesDiscovered in Idle updates endorserCandidates, recomputes the priority list, and
// resets currentActiveCoordinator to priorityList[0] via action_ResetToTopPriorityCoordinator.
func TestStateMachine_Idle_EndorserNodesDiscovered_UpdatesCandidatesResetsToTopPriority(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("old").
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.EndorserNodesDiscoveredEvent{
		Nodes: []string{"node1", "node2"},
	}))

	assert.Equal(t, State_Idle, o.GetCurrentState())
	require.NotEmpty(t, o.coordinatorPriorityList)
	assert.Equal(t, o.coordinatorPriorityList[0], o.currentActiveCoordinator, "should reset to top-priority coordinator")
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must be 1 when active coordinator is at position 0")
	assert.Equal(t, []string{"node1", "node2"}, o.endorserCandidates)
	assert.ElementsMatch(t, []string{"node1", "node2"}, o.coordinatorPriorityList)
}

// TransactionStateTransition to Final in Idle removes the transaction from memory.
func TestStateMachine_Idle_TransactionStateTransition_ToFinal_CleansUpTransaction(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		Transactions(mockTxn).
		Build()
	require.Len(t, o.transactionsByID, 1)
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Final,
	}))
	assert.Equal(t, State_Idle, o.GetCurrentState())
	assert.Empty(t, o.transactionsByID, "action_CleanUpTransaction must remove the transaction")
}

// TransactionStateTransition to Confirmed in Idle queues a FinalizeEvent; state stays Idle.
func TestStateMachine_Idle_TransactionStateTransition_ToConfirmed_FinalizesAndStaysIdle(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		Transactions(mockTxn).
		Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Confirmed,
	}))
	assert.Equal(t, State_Idle, o.GetCurrentState())
}

// TransactionStateTransition to Reverted in Idle queues a FinalizeEvent; state stays Idle.
func TestStateMachine_Idle_TransactionStateTransition_ToReverted_FinalizesAndStaysIdle(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).
		Transactions(mockTxn).
		Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Reverted,
	}))
	assert.Equal(t, State_Idle, o.GetCurrentState())
}

// ── State_Observing ───────────────────────────────────────────────────────────

// TransactionCreated in Observing → Sending; delegation request sent.
func TestStateMachine_Observing_TransactionCreated_TransitionsToSending_SendsDelegation(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("coordinator@node1").
		WithMockTransportWriter(t)
	o, mocks := builder.Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "coordinator@node1", mock.Anything, mock.Anything).
		Return(nil).Once()

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator("sender@node1").Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &TransactionCreatedEvent{Transaction: txn}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Len(t, o.transactionsByID, 1)
}

// HeartbeatReceived in Observing from Active node → coordinator updated; timer reset;
// resetFailoverIndex sets index=0 because sender is not at priorityList[0].
func TestStateMachine_Observing_HeartbeatReceived_ActiveState_UpdatesCoordinatorAndTimer(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("old-coordinator").
		CoordinatorPriorityList("other-node", "new-coordinator").
		FailoverIndex(1)
	o, _ := builder.Build()
	o.heartbeatIntervalsSinceLastReceive = 3
	ca := builder.GetContractAddress()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "new-coordinator",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))

	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, "new-coordinator", o.currentActiveCoordinator)
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "timer must be reset for Active heartbeat")
	// new-coordinator is at index 1 (not top priority) → failoverIndex=0
	assert.Equal(t, 0, o.failoverIndex, "failoverIndex must be recalibrated: 0 when active is not top priority")
}

// HeartbeatReceived in Observing from Flush node → timer reset; coordinator NOT updated.
// HeartbeatReceived in Observing from Active_Flush node → timer reset; currentActiveCoordinator updated.
func TestStateMachine_Observing_HeartbeatReceived_ActiveFlushState_ResetsTimerAndUpdatesCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("existing-coordinator")
	o, _ := builder.Build()
	o.heartbeatIntervalsSinceLastReceive = 3
	ca := builder.GetContractAddress()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "other-node",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active_Flush,
		},
	}))

	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, "other-node", o.currentActiveCoordinator, "Active_Flush heartbeat must update coordinator")
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "timer must be reset for Active_Flush heartbeat")
}

// HeartbeatReceived in Observing from Closing node → no timer reset; coordinator NOT updated.
func TestStateMachine_Observing_HeartbeatReceived_ClosingState_NoTimerResetAndNoCoordinatorUpdate(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("existing-coordinator")
	o, _ := builder.Build()
	o.heartbeatIntervalsSinceLastReceive = 3
	ca := builder.GetContractAddress()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "other-node",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing,
		},
	}))

	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, "existing-coordinator", o.currentActiveCoordinator, "Closing heartbeat must not update coordinator")
	assert.Equal(t, 3, o.heartbeatIntervalsSinceLastReceive, "timer must not be reset for Closing heartbeat")
}

// HeartbeatInterval in Observing within grace period → increments counter; stays Observing.
func TestStateMachine_Observing_HeartbeatInterval_WithinGrace_StaysObserving(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		HeartbeatIntervalsSinceLastReceive(0).
		InactiveGracePeriod(2).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))

	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, 1, o.heartbeatIntervalsSinceLastReceive, "counter must be incremented")
}

// HeartbeatInterval in Observing exceeding grace period → transitions to Idle.
func TestStateMachine_Observing_HeartbeatInterval_GraceExceeded_TransitionsToIdle(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		HeartbeatIntervalsSinceLastReceive(1).
		InactiveGracePeriod(2).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))

	assert.Equal(t, State_Idle, o.GetCurrentState())
	assert.Equal(t, 2, o.heartbeatIntervalsSinceLastReceive, "counter must be incremented before transition check")
}

// EndorserNodesDiscovered in Observing updates endorserCandidates and recomputes the priority list.
// currentActiveCoordinator is not changed.
func TestStateMachine_Observing_EndorserNodesDiscovered_UpdatesCandidatesAndRecomputesList(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("old").
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.EndorserNodesDiscoveredEvent{
		Nodes: []string{"nodeA", "nodeB"},
	}))

	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, "old", o.currentActiveCoordinator, "action_UpdateEndorserCandidates must not change currentActiveCoordinator")
	assert.Equal(t, []string{"nodeA", "nodeB"}, o.endorserCandidates)
	assert.ElementsMatch(t, []string{"nodeA", "nodeB"}, o.coordinatorPriorityList)
}

// Duplicate TransactionCreated (same ID) in Observing → no state change; no double tracking.
func TestStateMachine_Observing_TransactionCreated_DuplicateID_StaysObserving(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		Transactions(mockTxn).
		Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: &components.PrivateTransaction{ID: txID},
	}))
	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Len(t, o.transactionsByID, 1, "duplicate must not be tracked twice")
}

// NewBlock event in Observing updates currentBlockHeight; stays Observing.
func TestStateMachine_Observing_NewBlock_UpdatesBlockHeight_StaysObserving(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).CurrentBlockHeight(10).Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.NewBlockEvent{BlockHeight: 20}))
	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, uint64(20), o.currentBlockHeight)
}

// TransactionStateTransition to Final in Observing removes the transaction from memory.
func TestStateMachine_Observing_TransactionStateTransition_ToFinal_CleansUpTransaction(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		Transactions(mockTxn).
		Build()
	require.Len(t, o.transactionsByID, 1)
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Final,
	}))
	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Empty(t, o.transactionsByID, "action_CleanUpTransaction must remove the transaction")
}

// TransactionStateTransition to Confirmed in Observing queues a FinalizeEvent; state stays Observing.
func TestStateMachine_Observing_TransactionStateTransition_ToConfirmed_FinalizesAndStaysObserving(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		Transactions(mockTxn).
		Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Confirmed,
	}))
	assert.Equal(t, State_Observing, o.GetCurrentState())
}

// TransactionStateTransition to Reverted in Observing queues a FinalizeEvent; state stays Observing.
func TestStateMachine_Observing_TransactionStateTransition_ToReverted_FinalizesAndStaysObserving(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		Transactions(mockTxn).
		Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Reverted,
	}))
	assert.Equal(t, State_Observing, o.GetCurrentState())
}

// ── State_Sending ─────────────────────────────────────────────────────────────

// TransactionCreated in Sending → creates txn and sends delegation; stays Sending.
func TestStateMachine_Sending_TransactionCreated_CreatesTxnAndDelegates(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		WithMockTransportWriter(t)
	o, mocks := builder.Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "coordinator@node1", mock.Anything, mock.Anything).
		Return(nil).Once()

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator("sender@node1").Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &TransactionCreatedEvent{Transaction: txn}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Len(t, o.transactionsByID, 1, "new transaction must be tracked")
}

// Duplicate TransactionCreated in Sending → validator blocks; no double delegation.
func TestStateMachine_Sending_TransactionCreated_DuplicateID_NoDelegation(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: &components.PrivateTransaction{ID: txID},
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Len(t, o.transactionsByID, 1, "duplicate must not be tracked twice")
}

// HeartbeatReceived in Sending with dropped transaction → redelegate.
func TestStateMachine_Sending_HeartbeatReceived_DroppedTransaction_Redelegates(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Delegated)
	mockTxn.On("GetPrivateTransaction").Return(&components.PrivateTransaction{ID: txID})
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		WithMockTransportWriter(t).
		Transactions(mockTxn)
	o, mocks := builder.Build()
	ca := builder.GetContractAddress()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "coordinator@node1", mock.Anything, mock.Anything).
		Return(nil).Once()

	// Heartbeat with empty snapshot — txID is absent ⇒ dropped.
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "coordinator@node1",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
}

// HeartbeatReceived in Sending from all-present snapshot → no redelegate.
func TestStateMachine_Sending_HeartbeatReceived_NoDroppedTransactions_NoRedelegate(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Delegated)
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		Transactions(mockTxn)
	o, _ := builder.Build()
	ca := builder.GetContractAddress()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "coordinator@node1",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
			PooledTransactions: []*common.SnapshotPooledTransaction{
				{ID: txID, Originator: "sender@node1"},
			},
		},
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
}

// HeartbeatReceived in Sending from a higher-priority node in Active state → redirects (step 2),
// recalibrates failoverIndex, then step 5 fires because the empty snapshot indicates the new
// coordinator has no record of our transaction, triggering a redelegate to it.
func TestStateMachine_Sending_HeartbeatReceived_HigherPriorityActiveNode_RedirectsAndRedelegates(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Delegated) // called by validator_HasDroppedTransactions (step 5)
	mockTxn.On("GetPrivateTransaction").Return(&components.PrivateTransaction{ID: txID})
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node2"). // node1 is higher priority (index 0)
		CoordinatorPriorityList("node1", "node2", "node3").
		FailoverIndex(0). // will be recalibrated to 1 after switch to node1 (index 0)
		WithMockTransportWriter(t).
		Transactions(mockTxn)
	o, mocks := builder.Build()
	ca := builder.GetContractAddress()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "node1", mock.Anything, mock.Anything).
		Return(nil).Once()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "node1",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, "node1", o.currentActiveCoordinator, "coordinator must be redirected to higher-priority node")
	// node1 is at index 0 → recalibrate sets failoverIndex=1
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must be recalibrated to 1 when active is top priority")
}

// HeartbeatInterval in Sending within grace period → counter incremented; no redelegate.
func TestStateMachine_Sending_HeartbeatInterval_WithinGrace_NoRedelegate(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		HeartbeatIntervalsSinceLastReceive(0).
		InactiveGracePeriod(2).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, 1, o.heartbeatIntervalsSinceLastReceive)
}

// HeartbeatInterval in Sending exceeding grace → STATIC/SENDER mode (no priority list):
// falls through to redelegate to the same coordinator; heartbeat counter is NOT reset.
func TestStateMachine_Sending_HeartbeatInterval_GraceExceeded_NoEndorserMode_RedelegatesToSameCoordinator(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetPrivateTransaction").Return(&components.PrivateTransaction{ID: txID})
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("coordinator@node1").
		HeartbeatIntervalsSinceLastReceive(1).
		InactiveGracePeriod(2).
		WithMockTransportWriter(t).
		Transactions(mockTxn)
	o, mocks := builder.Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "coordinator@node1", mock.Anything, mock.Anything).
		Return(nil).Once()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	// Counter is incremented by action_IncrementHeartbeatIntervalCounts but NOT reset (no priority list).
	assert.Equal(t, 2, o.heartbeatIntervalsSinceLastReceive)
}

// HeartbeatInterval in Sending exceeding grace → ENDORSER mode: failover to next-priority
// coordinator, advance failoverIndex, reset liveness counter, send delegation to new coordinator.
func TestStateMachine_Sending_HeartbeatInterval_GraceExceeded_EndorserMode_FailoverToNextCoordinator(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetPrivateTransaction").Return(&components.PrivateTransaction{ID: txID})
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("A").
		CoordinatorPriorityList("A", "B", "C").
		FailoverIndex(1). // A is at index 0 → next failover target is index 1 = "B"
		HeartbeatIntervalsSinceLastReceive(1).
		InactiveGracePeriod(2).
		WithMockTransportWriter(t).
		Transactions(mockTxn)
	o, mocks := builder.Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "B", mock.Anything, mock.Anything).
		Return(nil).Once()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, "B", o.currentActiveCoordinator, "must failover to next priority node")
	assert.Equal(t, 2, o.failoverIndex, "failoverIndex must be incremented")
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness counter must be reset on failover")
}

// HeartbeatInterval in Sending exceeding grace → ENDORSER mode wrap-around: after the last
// position failoverIndex cycles back to 0.
func TestStateMachine_Sending_HeartbeatInterval_GraceExceeded_EndorserMode_WrapAroundFailover(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetPrivateTransaction").Return(&components.PrivateTransaction{ID: txID})
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("B").
		CoordinatorPriorityList("A", "B", "C").
		FailoverIndex(2). // pointing to last slot "C"
		HeartbeatIntervalsSinceLastReceive(1).
		InactiveGracePeriod(2).
		WithMockTransportWriter(t).
		Transactions(mockTxn)
	o, mocks := builder.Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "C", mock.Anything, mock.Anything).
		Return(nil).Once()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, "C", o.currentActiveCoordinator, "must failover to last-priority node")
	assert.Equal(t, 0, o.failoverIndex, "failoverIndex must wrap around to 0 after the last position")
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness counter must be reset on failover")
}

// DelegationRejected in Sending with higher-priority named coordinator → redirects, recalibrates
// failoverIndex, and redelegates.
func TestStateMachine_Sending_DelegationRejected_HigherPriority_RedirectsAndRedelegates(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetPrivateTransaction").Return(&components.PrivateTransaction{ID: txID})
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		FailoverIndex(0). // will be recalibrated to 1 after redirect to node1 (index 0)
		WithMockTransportWriter(t).
		Transactions(mockTxn)
	o, mocks := builder.Build()

	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "node1", mock.Anything, mock.Anything).
		Return(nil).Once()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &DelegationRejectedEvent{
		ActiveCoordinator: "node1",
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, "node1", o.currentActiveCoordinator)
	// node1 is at index 0 → recalibrate sets failoverIndex=1
	assert.Equal(t, 1, o.failoverIndex, "failoverIndex must be recalibrated to 1 when active is top priority")
}

// DelegationRejected in Sending with lower-priority named coordinator → no redirect; no redelegate.
func TestStateMachine_Sending_DelegationRejected_LowerPriority_NoChange(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &DelegationRejectedEvent{
		ActiveCoordinator: "node3",
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, "node1", o.currentActiveCoordinator, "lower-priority coordinator must be ignored")
}

// TransactionStateTransition to Final in Sending removes transaction; transitions to Observing
// when it was the last unconfirmed transaction.
func TestStateMachine_Sending_TransactionFinal_LastTxn_TransitionsToObserving(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		FromState:     transaction.State_Delegated,
		ToState:       transaction.State_Final,
	}))

	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Empty(t, o.transactionsByID, "final transaction must be removed")
}

// TransactionStateTransition to Final in Sending when other transactions remain → stays Sending.
func TestStateMachine_Sending_TransactionFinal_OtherTxnsRemain_StaysSending(t *testing.T) {
	ctx := context.Background()
	txID1 := uuid.New()
	txID2 := uuid.New()
	mockTxn1 := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn1.On("GetID").Return(txID1)
	mockTxn2 := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn2.On("GetID").Return(txID2)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn1, mockTxn2).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID1,
		FromState:     transaction.State_Delegated,
		ToState:       transaction.State_Final,
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Len(t, o.transactionsByID, 1, "only the finalized transaction is removed")
}

// EndorserNodesDiscovered in Sending updates endorserCandidates and recomputes the priority list.
// currentActiveCoordinator is not changed.
func TestStateMachine_Sending_EndorserNodesDiscovered_UpdatesCandidatesAndRecomputesList(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("old-coordinator").
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.EndorserNodesDiscoveredEvent{
		Nodes: []string{"new-node1", "new-node2"},
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, "old-coordinator", o.currentActiveCoordinator, "action_UpdateEndorserCandidates must not change currentActiveCoordinator")
	assert.Equal(t, []string{"new-node1", "new-node2"}, o.endorserCandidates)
	assert.ElementsMatch(t, []string{"new-node1", "new-node2"}, o.coordinatorPriorityList)
}

// NewBlock event in Sending updates currentBlockHeight; stays Sending.
func TestStateMachine_Sending_NewBlock_UpdatesBlockHeight_StaysSending(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentBlockHeight(50).
		Transactions(mockTxn).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.NewBlockEvent{BlockHeight: 100}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, uint64(100), o.currentBlockHeight)
}

// HeartbeatReceived in Sending from a live non-current node when the inactive grace period has NOT
// been exceeded (step-3 guard false) — the coordinator is not switched; state stays Sending.
func TestStateMachine_Sending_HeartbeatReceived_Step3_WithinGrace_NoCoordinatorSwitch(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		HeartbeatIntervalsSinceLastReceive(0).
		InactiveGracePeriod(2)
	o, _ := builder.Build()
	ca := builder.GetContractAddress()

	// "node3" is live (Active) but NOT higher priority than "node1" and grace is not exceeded.
	// Step 2 does not fire (not higher priority); step 3 guard is false (grace not exceeded);
	// steps 4 and 5 don't fire (not from current coordinator).
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "node3",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, "node1", o.currentActiveCoordinator, "coordinator must not switch when grace has not expired")
}

// HeartbeatReceived in Sending from a live non-current node when grace IS exceeded (step 3) —
// the coordinator switches to that node, recalibrates failoverIndex, and redelegates.
func TestStateMachine_Sending_HeartbeatReceived_Step3_GraceExceeded_SwitchesCoordinatorAndRedelegates(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Delegated)
	mockTxn.On("GetPrivateTransaction").Return(&components.PrivateTransaction{ID: txID})
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(nil)
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		FailoverIndex(1). // will be recalibrated to 0 after switch to node3 (index 2, not top)
		HeartbeatIntervalsSinceLastReceive(2).
		InactiveGracePeriod(2). // 2 >= 2 → exceeded
		WithMockTransportWriter(t).
		Transactions(mockTxn)
	o, mocks := builder.Build()
	ca := builder.GetContractAddress()

	// Step 3 fires: switches to "node3" (live, not current, grace exceeded).
	// Step 4 then fires because currentActiveCoordinator is now "node3".
	// Step 5 fires because empty snapshot means txID is dropped → redelegate.
	mocks.TransportWriter.EXPECT().
		SendDelegationRequest(mock.Anything, "node3", mock.Anything, mock.Anything).
		Return(nil).Once()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode:        "node3",
		ContractAddress: &ca,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Equal(t, "node3", o.currentActiveCoordinator, "coordinator must switch to the now-active node when grace expires")
	// node3 is not at index 0 → recalibrate sets failoverIndex=0
	assert.Equal(t, 0, o.failoverIndex, "failoverIndex must be recalibrated: 0 when active is not top priority")
}

// TransactionStateTransition to Confirmed in Sending runs action_FinalizeTransaction (queues an
// internal FinalizeEvent) but does not remove the transaction or trigger a state transition.
func TestStateMachine_Sending_TransactionConfirmed_LastTxn_StaysInSending(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Confirmed,
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Len(t, o.transactionsByID, 1, "action_FinalizeTransaction does not remove; transaction stays until Final")
}

// TransactionStateTransition to Reverted in Sending when other transactions remain → stays Sending.
// action_FinalizeTransaction queues a FinalizeEvent; the transaction stays in memory until Final.
// guard_HasTransactions checks len(transactionsByID), not transaction state, so no GetCurrentState needed.
func TestStateMachine_Sending_TransactionReverted_OtherTxnsRemain_StaysSending(t *testing.T) {
	ctx := context.Background()
	txID1 := uuid.New()
	txID2 := uuid.New()
	mockTxn1 := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn1.On("GetID").Return(txID1)
	mockTxn2 := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn2.On("GetID").Return(txID2)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn1, mockTxn2).
		Build()

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID1,
		ToState:       transaction.State_Reverted,
	}))

	assert.Equal(t, State_Sending, o.GetCurrentState())
	assert.Len(t, o.transactionsByID, 2, "action_FinalizeTransaction does not remove; transaction stays until Final")
}
