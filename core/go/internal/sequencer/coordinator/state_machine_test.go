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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/coordinatortransactionmocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newDispatchedTxMock creates a minimal mock coordinator transaction in Dispatched state.
// GetCurrentState() and GetSnapshot() use Maybe() since call counts depend on which actions run.

func newDispatchedTxMock(t *testing.T) (*coordinatortransactionmocks.CoordinatorTransaction, uuid.UUID) {
	t.Helper()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	tx.EXPECT().GetID().Return(txID).Maybe()
	tx.EXPECT().GetCurrentState().Return(transaction.State_Dispatched).Maybe()
	// GetSnapshot is called by action_SendHeartbeat when building the coordinator heartbeat payload.
	tx.EXPECT().GetSnapshot(mock.Anything).Return(nil, &engineProto.SnapshotDispatchedTransaction{Id: txID.String()}, nil, nil).Maybe()
	// HandleEvent is called by action_PropagateHeartbeatIntervalToTransactions on each heartbeat tick.
	tx.EXPECT().HandleEvent(mock.Anything, mock.AnythingOfType("*common.HeartbeatIntervalEvent")).Return(nil).Maybe()
	// GetOriginatorNode is called by updateOriginatorActivity in STATIC/SENDER modes.
	tx.EXPECT().GetOriginatorNode().Return("originator-node").Maybe()
	return tx, txID
}

func Test_queueEventInternal_QueuesPriorityEvent(t *testing.T) {
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	ctx, cancel := context.WithCancel(t.Context())
	c, mocks := builder.Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	c.Start(ctx)
	defer func() {
		cancel()
		c.WaitForDone(t.Context())
	}()

	syncEvent := statemachine.NewSyncEvent()
	c.queueEventInternal(ctx, syncEvent)
	<-syncEvent.Done
	require.False(t, c.stateMachineEventLoop.IsStopped(), "event loop should still be running")
}

func Test_TryQueueEvent_QueuesToEventLoop(t *testing.T) {
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	ctx, cancel := context.WithCancel(t.Context())
	c, mocks := builder.Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	c.Start(ctx)
	defer func() {
		cancel()
		c.WaitForDone(t.Context())
	}()

	event := &CoordinatorCreatedEvent{}
	ok := c.TryQueueEvent(ctx, event)
	require.True(t, ok, "TryQueueEvent should return true when event is queued")

	// Drain the event so the loop can process it and we can cleanly stop
	syncEvent := statemachine.NewSyncEvent()
	c.QueueEvent(ctx, syncEvent)
	<-syncEvent.Done
}

func TestCoordinator_WhenCreated_TransitionsToIdle(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Initial).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &CoordinatorCreatedEvent{}))
	assert.Equal(t, State_Idle, c.GetCurrentState())
	assert.NotEmpty(t, c.coordinatorPriorityList, "calculateCoordinatorPriorities must populate the priority list")
}

func TestCoordinator_WhenIdle_TransitionsToObserving_OnHeartbeatFromCurrentActive(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Build()
	// Pass a recognisable coordinator state so we can confirm action_HeartbeatReceived ran.
	event := &common.HeartbeatReceivedEvent{
		FromNode:            "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{CoordinatorState: State_Active},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Observing, c.GetCurrentState())
	assert.Equal(t, "node2", c.currentActiveCoordinator)
}

func TestCoordinator_WhenIdle_StaysIdle_OnHeartbeatFromNodeInNonActiveState(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode:            "node3",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{CoordinatorState: common.CoordinatorState_Closing},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Idle, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "non-live heartbeat must leave currentActiveCoordinator unchanged")
}

func TestCoordinator_WhenIdle_HeartbeatReceived_UpdatesEndorserCandidates(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorPriorityList("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState:   common.CoordinatorState_Idle,
			EndorserCandidates: []string{"node1", "node2"},
		},
	}))

	assert.Equal(t, State_Idle, c.GetCurrentState())
	assert.Contains(t, c.endorserCandidates, "node2")
}

func TestCoordinator_WhenIdleAndTransactionsDelegatedToSelf_TransitionsToActive(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		EndorserCandidates("node2"). // gives action_SendHeartbeat a recipient
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	event := &TransactionsDelegatedEvent{
		FromNode:     "senderNode",
		Originator:   "sender@senderNode",
		DelegationID: "delegation-1",
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.NotEmpty(t, c.signingIdentity.value, "OnTransitionTo Active must set signing identity")
	assert.Equal(t, "node1", c.currentActiveCoordinator, "OnTransitionTo Active must set currentActiveCoordinator to self")
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "OnTransitionTo Active must send an immediate heartbeat")
}

func TestCoordinator_WhenIdle_TransactionsDelegated_BlockHeightToleranceExceeded_RejectsAndStaysIdle(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	// action_RefreshBlockHeight fetches the live height and stores it in c.currentBlockHeight.
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	// diff = |100 - 0| = 100 > 10 → block height rejection.
	mocks.TransportWriter.EXPECT().SendDelegationRejection(
		mock.Anything, "node2", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
			return msg.DelegationId == "del-bh-test" &&
				msg.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
				msg.OriginatorBlockHeight == int64(0) &&
				msg.CoordinatorBlockHeight == int64(100) &&
				msg.BlockHeightTolerance == int64(10)
		}),
	).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, newDelegationBlockHeightExceedingEvent("node2")))
	assert.Equal(t, State_Idle, c.GetCurrentState())
}

func TestCoordinator_WhenIdle_EndorsementRequestReceived_UpdatesActiveCoordinatorAndTransitionsToObserving(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node2", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Observing, c.GetCurrentState())
	assert.Equal(t, "node2", c.currentActiveCoordinator)
}

func TestCoordinator_WhenIdle_EndorsementRequestReceived_BlockHeightToleranceExceeded_RejectsAndStaysIdle(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	// Block height difference (100 - 0 = 100) exceeds tolerance (10) → rejection.
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-bh-test", "ik-bh-test",
			engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, int64(0), int64(100), int64(10)),
	).Return(nil)

	event := newBlockHeightExceedingEndorsementEvent("node2", mocks)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	// Rejection path: no state change, no active-coordinator update.
	assert.Equal(t, State_Idle, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenIdle_EndorsementRequestReceived_UpdatesEndorserCandidatesFromSender(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Contains(t, c.endorserCandidates, "node3")
}

func TestCoordinator_WhenIdle_EndorsementRequestReceived_PrivateStateDataPending_RejectsAndStaysIdle(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		BlockHeightTolerance(1000). // prevent block-height check from firing first
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, nil)
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-1", "ik-1",
			engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING, int64(100), int64(0), int64(10)),
	).Return(nil)

	event := &EndorsementRequestReceivedEvent{
		FromNode:                  "node2",
		TransactionId:             "tx-1",
		IdempotencyKey:            "ik-1",
		Party:                     "party1@node2",
		CoordinatorBlockHeight:    100,
		BlockHeightTolerance:      10, // lowWatermark = 90
		AttestationRequest:        &prototk.AttestationRequest{Name: "att1"},
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Idle, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenObserving_TransitionsToIdle_OnHeartbeatIntervalInactiveGrace(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Observing).
		InactiveGracePeriod(3).
		HeartbeatIntervalsSinceLastReceive(3).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	// action_IncrementHeartbeatIntervalCounts bumps counter to 4; guard_InactiveGracePeriodExceeded = (4 > 3) = true
	assert.Equal(t, State_Idle, c.GetCurrentState())
	assert.Equal(t, 4, c.heartbeatIntervalsSinceLastReceive, "counter must be past the grace-period threshold after increment")
}

func TestCoordinator_WhenObserving_HeartbeatInterval_WithinGrace_IncrementsCounterAndStaysObserving(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Observing).
		HeartbeatIntervalsSinceLastReceive(1).
		InactiveGracePeriod(3).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	// Counter bumps to 2; guard_InactiveGracePeriodExceeded = (2 > 3) = false → stays Observing.
	assert.Equal(t, State_Observing, c.GetCurrentState())
	assert.Equal(t, 2, c.heartbeatIntervalsSinceLastReceive, "counter must be incremented")
}

func TestCoordinator_WhenObserving_DelegatedTransactions_HigherPriority_TransitionsToElect(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		CurrentActiveCoordinator("node2"). // node1 is higher priority than node2
		CoordinatorPriorityList("node1", "node2", "node3").
		WithMockTransportWriter().
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	// action_ProcessDelegatedTransactions sends an acknowledgment.
	mocks.TransportWriter.EXPECT().SendDelegationResponse(mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationResponse) bool {
		return msg.DelegationId == "del-1"
	})).Return(nil)
	// OnTransitionTo Elect fires action_SendHandoverRequest.
	mocks.TransportWriter.EXPECT().SendHandoverRequest(mock.Anything, "node2", mock.Anything).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-1",
	}))
	// guard_IsHigherPriorityThanCurrentActive = true (node1 at index 0 < node2 at index 1) → Elect.
	assert.Equal(t, State_Elect, c.GetCurrentState())
}

func TestCoordinator_WhenObserving_DelegatedTransactions_LowerPriority_RejectsAndStaysObserving(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node3").
		CurrentActiveCoordinator("node1"). // node3 is lower priority than node1
		CoordinatorPriorityList("node1", "node2", "node3").
		WithMockTransportWriter().
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	// action_RejectDelegationRequest sends a rejection naming the current active coordinator.
	mocks.TransportWriter.EXPECT().SendDelegationRejection(mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
		return msg.DelegationId == "del-1" && msg.ActiveCoordinator == "node1" && msg.RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE
	})).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-1",
	}))
	// guard_IsHigherPriorityThanCurrentActive = false (node3 at index 2 > node1 at index 0) → stays Observing.
	assert.Equal(t, State_Observing, c.GetCurrentState())
}

func TestCoordinator_WhenObserving_TransactionsDelegated_BlockHeightToleranceExceeded_RejectsAndStaysObserving(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node2").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendDelegationRejection(
		mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
			return msg.DelegationId == "del-bh-test" &&
				msg.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
				msg.OriginatorBlockHeight == int64(0) &&
				msg.CoordinatorBlockHeight == int64(100) &&
				msg.BlockHeightTolerance == int64(10)
		}),
	).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, newDelegationBlockHeightExceedingEvent("originator-node")))
	assert.Equal(t, State_Observing, c.GetCurrentState())
	// active coordinator must not be updated on a block height rejection
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenObserving_EndorsementRequestReceived_UpdatesActiveCoordinatorAndStaysObserving(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node2").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Observing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenObserving_EndorsementRequestReceived_BlockHeightToleranceExceeded_RejectsAndStaysObserving(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node2").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-bh-test", "ik-bh-test",
			engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, int64(0), int64(100), int64(10)),
	).Return(nil)

	event := newBlockHeightExceedingEndorsementEvent("node1", mocks)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Observing, c.GetCurrentState())
	// active coordinator must not be updated on rejection
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenObserving_EndorsementRequestReceived_UpdatesEndorserCandidatesFromSender(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node2").
		EndorserCandidates("node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Contains(t, c.endorserCandidates, "node3")
}

func TestCoordinator_WhenObserving_EndorsementRequestReceived_PrivateStateDataPending_RejectsAndStaysObserving(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node2").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		BlockHeightTolerance(1000). // prevent block-height check from firing first
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, nil)
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-1", "ik-1",
			engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING, int64(100), int64(0), int64(10)),
	).Return(nil)

	event := &EndorsementRequestReceivedEvent{
		FromNode:                  "node1",
		TransactionId:             "tx-1",
		IdempotencyKey:            "ik-1",
		Party:                     "party1@node1",
		CoordinatorBlockHeight:    100,
		BlockHeightTolerance:      10, // lowWatermark = 90
		AttestationRequest:        &prototk.AttestationRequest{Name: "att1"},
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	// Rejection path: active coordinator must not be updated.
	assert.Equal(t, State_Observing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenObserving_HeartbeatReceived_UpdatesEndorserCandidates(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorPriorityList("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState:   common.CoordinatorState_Idle,
			EndorserCandidates: []string{"node1", "node2"},
		},
	}))

	assert.Equal(t, State_Observing, c.GetCurrentState())
	assert.Contains(t, c.endorserCandidates, "node2")
}

func TestCoordinator_WhenElectRequestTimeoutFires_NudgesHandoverRequest(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		WithMockTransportWriter().
		Build()
	// Simulate Elect entry: sendHandoverRequest creates the pending request. A freshly constructed
	// IdempotentRequest has requestTime == nil, so its first Nudge() always sends immediately.
	mocks.TransportWriter.EXPECT().SendHandoverRequest(mock.Anything, "node2", mock.Anything).Return(nil).Once()
	c.pendingHandoverRequest = common.NewIdempotentRequest(ctx, c.clock, c.requestTimeout, func(ctx context.Context, _ uuid.UUID) error {
		return c.transportWriter.SendHandoverRequest(ctx, c.currentActiveCoordinator, &engineProto.CoordinatorHandoverRequest{
			FromNode:        c.nodeName,
			ContractAddress: c.contractAddress.HexString(),
		})
	})

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &RequestTimeoutIntervalEvent{}))
	assert.Equal(t, State_Elect, c.GetCurrentState(), "request timeout must not change state")
	// action_NudgeHandoverRequest does NOT re-arm the request timer.
	assert.Nil(t, c.cancelRequestTimeout, "request timeout must not be re-armed by nudge")
}

func TestCoordinator_WhenElectStateTimeoutFires_TransitionsToActive(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Build()
	// Firing the state-timeout event simulates the wall-clock timer expiring.
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &StateTimeoutIntervalEvent{}))
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.NotEmpty(t, c.signingIdentity.value, "OnTransitionTo Active must set signing identity")
	assert.Equal(t, "node1", c.currentActiveCoordinator, "OnTransitionTo Active must set currentActiveCoordinator to self")
	// action_ClearTimeoutSchedules ran on exit; both cancel funcs must be nil.
	assert.Nil(t, c.cancelRequestTimeout, "request timeout must be cleared on Elect exit")
	assert.Nil(t, c.cancelStateTimeout, "state timeout must be cleared on Elect exit")
	// action_ImportStatesAndLocks did NOT run: no HeartbeatReceivedEvent on this path, grapher stays empty.
	exported, err := c.grapher.ExportStatesAndLocks(ctx, "node1")
	require.NoError(t, err)
	assert.Empty(t, exported.GetStates(), "no states must be imported on state-timeout path")
	assert.Empty(t, exported.GetLocks(), "no locks must be imported on state-timeout path")
}

func TestCoordinator_WhenElect_HeartbeatInterval_PropagatesAndSendsHeartbeatAndStaysElect(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		EndorserCandidates("node2"). // gives action_SendHeartbeat a recipient
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	assert.Equal(t, State_Elect, c.GetCurrentState())
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "action_SendHeartbeat must fire in Elect HeartbeatInterval")
}

func TestCoordinator_WhenElect_ActiveCoordinatorStartsFlushing_TransitionsToPrepared(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Build()

	event := &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing_Flush,
		},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Prepared, c.GetCurrentState(), "Closing_Flush heartbeat from current active must drive Elect → Prepared")
	// action_ClearTimeoutSchedules ran on the Elect → Prepared transition.
	assert.Nil(t, c.cancelRequestTimeout, "request timeout must be cleared on Elect exit")
	assert.Nil(t, c.cancelStateTimeout, "state timeout must be cleared on Elect exit")
}

func TestCoordinator_WhenElect_ActiveCoordinatorClosing_TransitionsDirectlyToActiveAndImportsState(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Build()

	// Construct a confirmed lock + its output state so we can verify the grapher absorbed them.
	stateID := pldtypes.HexBytes{0x01, 0x02, 0x03, 0x04}
	confirmedAtBlock := uint64(99)

	event := &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing,
			StateSnapshot: &prototk.StateSnapshot{
				States: []*prototk.SnapshotState{{State: &prototk.EndorsableState{Id: stateID.String()}, AllowedNodes: []string{"node1"}}},
				Locks:  []*prototk.SnapshotStateLock{{StateId: stateID.String(), Type: prototk.SnapshotStateLock_CREATE, ConfirmedAtBlock: &confirmedAtBlock}},
			},
		},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Active, c.GetCurrentState(), "Closing heartbeat from current active must drive Elect → Active directly")
	assert.NotEmpty(t, c.signingIdentity.value, "OnTransitionTo Active must set signing identity")
	assert.Equal(t, "node1", c.currentActiveCoordinator, "OnTransitionTo Active must set currentActiveCoordinator to self")
	// action_ClearTimeoutSchedules ran on the Elect → Active transition.
	assert.Nil(t, c.cancelRequestTimeout, "request timeout must be cleared on Elect exit")
	assert.Nil(t, c.cancelStateTimeout, "state timeout must be cleared on Elect exit")
	// action_ImportStatesAndLocks ran: the grapher must now hold the imported state and lock.
	exported, err := c.grapher.ExportStatesAndLocks(ctx, "node1")
	require.NoError(t, err)
	assert.Len(t, exported.GetStates(), 1, "imported output state must be visible to node1")
	assert.Len(t, exported.GetLocks(), 1, "imported confirmed lock must be present in grapher")
}

func TestCoordinator_WhenElect_StaysElect_OnHeartbeatFromCurrentCoordinator_WhenStillActive(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("prev"). // the outgoing coordinator
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode: "prev",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active, // still Active — not flushing yet
		},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Elect, c.GetCurrentState())
	// guard_ActiveCoordinatorStartedFlushing is false (Active is not Flush/Closing); no transition.
}

func TestCoordinator_WhenElect_HigherPriorityHeartbeat_TransitionsToObserving(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node3"). // node3 is who we asked to hand over
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	// Heartbeat from node1 (higher priority, Active) — sets receivedHigherPriorityActiveHeartbeat.
	event := &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Observing, c.GetCurrentState(), "higher-priority active heartbeat must drive Elect → Observing")
	// action_ClearTimeoutSchedules ran on the Elect → Observing transition.
	assert.Nil(t, c.cancelRequestTimeout, "request timeout must be cleared on Elect exit")
	assert.Nil(t, c.cancelStateTimeout, "state timeout must be cleared on Elect exit")
}

func TestCoordinator_WhenElect_HigherPriorityHeartbeat_HasInflightNoDispatched_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	txConfirmed := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txConfirmed.EXPECT().GetID().Return(txID).Maybe()
	txConfirmed.EXPECT().GetCurrentState().Return(transaction.State_Confirmed).Maybe()
	// GetSnapshot is called by action_SendHeartbeat in OnTransitionTo Closing.
	txConfirmed.EXPECT().GetSnapshot(mock.Anything).Return(nil, nil, nil, nil).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Transactions(txConfirmed).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	// Confirmed txns survive action_CleanUpTransactionsNotYetDispatched (Confirmed is excluded).
	// guard_HasTransactionsInflight = true, guard_HasUnconfirmedDispatchedTransactions = false → Closing.
	assert.Equal(t, State_Closing, c.GetCurrentState())
}

func TestCoordinator_WhenElect_HigherPriorityHeartbeat_HasInflightAndDispatched_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	// guard_HasTransactionsInflight = true, guard_HasUnconfirmedDispatchedTransactions = true → Closing_Flush.
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
}

func TestCoordinator_WhenElect_HeartbeatFromCurrentActiveCoordinator_HigherPriorityThanSelf_TransitionsToObserving(t *testing.T) {
	// Regression: node2 is in Elect targeting node1 (currentActiveCoordinator), but node1 is also
	// higher-priority than node2. validator_IsHeartbeatFromHigherPriorityCoordinator must compare
	// against nodeName (node2), not currentActiveCoordinator (node1), so that IsHigherPriority
	// returns true and node2 steps back down.
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node1"). // the node we're trying to displace
		CoordinatorPriorityList("node1", "node2").
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	// validator_IsHeartbeatFromHigherPriorityCoordinator: IsHigherPriority(list, "node1", "node2") = 0 < 1 = true
	// guard_HasTransactionsInflight = false → Observing
	assert.Equal(t, State_Observing, c.GetCurrentState())
}

func TestCoordinator_WhenElect_HandoverRequest_HigherPriority_HasDispatched_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node3"). // for OnTransitionTo Closing_Flush heartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &HandoverRequestEvent{
		FromNode: "node1",
	}))
	// validator_IsHandoverRequestFromHigherPriorityCoordinator: IsHigherPriority(list, "node1", "node2") = true
	// guard_HasUnconfirmedDispatchedTransactions = true → Closing_Flush
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the requesting node")
}

func TestCoordinator_WhenElect_HandoverRequest_HigherPriority_NoDispatched_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node3"). // for OnTransitionTo Closing heartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &HandoverRequestEvent{
		FromNode: "node1",
	}))
	// guard_HasUnconfirmedDispatchedTransactions = false → Closing
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the requesting node")
}

func TestCoordinator_WhenElect_DelegatedTransactions_AcceptsAndStaysElect(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-1",
	}))
	assert.Equal(t, State_Elect, c.GetCurrentState())
}

func TestCoordinator_WhenElect_TransactionsDelegated_BlockHeightToleranceExceeded_RejectsAndStaysElect(t *testing.T) {
	// Block height check is evaluated before any delegation processing — even a valid originator
	// is rejected if the height difference exceeds tolerance.
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendDelegationRejection(
		mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
			return msg.DelegationId == "del-bh-test" &&
				msg.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
				msg.OriginatorBlockHeight == int64(0) &&
				msg.CoordinatorBlockHeight == int64(100) &&
				msg.BlockHeightTolerance == int64(10)
		}),
	).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, newDelegationBlockHeightExceedingEvent("originator-node")))
	assert.Equal(t, State_Elect, c.GetCurrentState())
}

func TestCoordinator_WhenElect_TransactionStateTransition_ToFinal_CleansUpAndStaysElect(t *testing.T) {
	ctx := t.Context()
	txFinal := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txFinal.EXPECT().GetID().Return(txID).Maybe()
	txFinal.EXPECT().GetCurrentState().Return(transaction.State_Final).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Transactions(txFinal).
		Build()
	require.Equal(t, 1, len(c.transactionsByID))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Final,
	}))
	assert.Equal(t, State_Elect, c.GetCurrentState())
	assert.Empty(t, c.transactionsByID, "action_CleanUpTransaction must remove the finalised transaction")
}

func TestCoordinator_WhenElect_TransactionStateTransition_ToPooled_AddsToPool(t *testing.T) {
	ctx := t.Context()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	tx.EXPECT().GetID().Return(txID).Maybe()
	tx.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Transactions(tx).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Pooled,
	}))
	assert.Equal(t, State_Elect, c.GetCurrentState())
	assert.Contains(t, c.pooledTransactions, tx, "action_PoolTransaction must add the transaction to the pool")
}

func TestCoordinator_WhenElect_EndorsementRequestReceived_HigherPriority_NoInflight_TransitionsToObserving(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()
	// No transactions in memory: guard_HasTransactionsInflight = false → Observing.

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Observing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenElect_EndorsementRequestReceived_HigherPriority_InflightAndUnconfirmed_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	tx, txID := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Transactions(tx).
		Build()
	c.inFlightTxns[txID] = struct{}{} // mark as unconfirmed dispatched

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenElect_EndorsementRequestReceived_HigherPriority_InflightNoUnconfirmed_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	// Use newDispatchedTxMock so GetSnapshot/.Maybe() is already set up for the OnTransitionTo
	// action_SendHeartbeat that fires when entering State_Closing.
	tx, _ := newDispatchedTxMock(t)
	// Override state to Confirmed so the tx counts as inflight-but-no-unconfirmed-dispatched.
	tx.EXPECT().GetCurrentState().Unset()
	tx.EXPECT().GetCurrentState().Return(transaction.State_Confirmed).Maybe()

	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Transactions(tx).
		Build()
	// Tx is inflight (in memory) but NOT in inFlightTxns (no unconfirmed dispatched).

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenElect_EndorsementRequestReceived_LowerPriority_RejectsAsActiveCoordinator(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-lp-test", "ik-lp-test",
			engineProto.RejectionReason_ENDORSER_IS_ACTIVE_COORDINATOR, int64(0), int64(0), int64(0)),
	).Return(nil)

	event := newLowerPriorityEndorsementEvent(t, "node3", mocks, true) // node3 < node1 in priority
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Elect, c.GetCurrentState())
	assert.Equal(t, "node2", c.currentActiveCoordinator, "lower-priority sender must not update active coordinator")
}

func TestCoordinator_WhenElect_EndorsementRequestReceived_BlockHeightToleranceExceeded_RejectsAndStaysElect(t *testing.T) {
	// Block height check is evaluated before the priority check — even a higher-priority sender
	// is rejected if the height difference exceeds tolerance.
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-bh-test", "ik-bh-test",
			engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, int64(0), int64(100), int64(10)),
	).Return(nil)

	event := newBlockHeightExceedingEndorsementEvent("node1", mocks) // node1 is higher priority
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	// Rejection path: stays in Elect, no step-down.
	assert.Equal(t, State_Elect, c.GetCurrentState())
	assert.Equal(t, "node2", c.currentActiveCoordinator)
}

func TestCoordinator_WhenElect_EndorsementRequestReceived_UpdatesEndorserCandidatesFromSender(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Contains(t, c.endorserCandidates, "node3")
}

func TestCoordinator_WhenElect_EndorsementRequestReceived_PrivateStateDataPending_RejectsAndStaysElect(t *testing.T) {
	// Private state check is evaluated before the priority check — even a higher-priority sender
	// is rejected if private state is incomplete.
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		BlockHeightTolerance(1000). // prevent block-height check from firing first
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, nil)
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-1", "ik-1",
			engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING, int64(100), int64(0), int64(10)),
	).Return(nil)

	event := &EndorsementRequestReceivedEvent{
		FromNode:                  "node1", // higher priority — rejected before priority check
		TransactionId:             "tx-1",
		IdempotencyKey:            "ik-1",
		Party:                     "party1@node1",
		CoordinatorBlockHeight:    100,
		BlockHeightTolerance:      10, // lowWatermark = 90
		AttestationRequest:        &prototk.AttestationRequest{Name: "att1"},
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	// Rejection path: stays in Elect, no step-down.
	assert.Equal(t, State_Elect, c.GetCurrentState())
	assert.Equal(t, "node2", c.currentActiveCoordinator)
}

func TestCoordinator_WhenPrepared_HeartbeatInterval_WithinGrace_SendsHeartbeatAndStaysPrepared(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		HeartbeatIntervalsSinceLastReceive(0).
		InactiveGracePeriod(3).      // after increment: 1 < 3 → no transition
		EndorserCandidates("node2"). // for action_SendHeartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	assert.Equal(t, State_Prepared, c.GetCurrentState())
	assert.Equal(t, 1, c.heartbeatIntervalsSinceLastReceive)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "action_SendHeartbeat must fire in Prepared HeartbeatInterval")
}

func TestCoordinator_WhenPrepared_HeartbeatInterval_GraceExceeded_TransitionsToActive(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		HeartbeatIntervalsSinceLastReceive(3).
		InactiveGracePeriod(3). // after increment: 4 > 3 → Active
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	// guard_InactiveGracePeriodExceeded = true → transitions to Active
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "OnTransitionTo Active must set currentActiveCoordinator to self")
	assert.NotEmpty(t, c.signingIdentity.value, "OnTransitionTo Active must call action_NewSigningIdentity")
}

func TestCoordinator_WhenPreparedReceivesClosingHeartbeat_TransitionsToActiveAndImportsState(t *testing.T) {
	ctx := t.Context()
	// Start in Prepared — this is the state after the outgoing coordinator acknowledged the handover
	// (Elect → Prepared) and is now completing its flush.
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2"). // node2 is the outgoing coordinator we're waiting on
		Build()

	// Construct a confirmed lock + its output state so we can verify the grapher absorbed them.
	stateID := pldtypes.HexBytes{0x01, 0x02, 0x03, 0x04}
	confirmedAtBlock := uint64(99)

	event := &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing,
			StateSnapshot: &prototk.StateSnapshot{
				States: []*prototk.SnapshotState{{State: &prototk.EndorsableState{Id: stateID.String()}, AllowedNodes: []string{"node1"}}},
				Locks:  []*prototk.SnapshotStateLock{{StateId: stateID.String(), Type: prototk.SnapshotStateLock_CREATE, ConfirmedAtBlock: &confirmedAtBlock}},
			},
		},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.NotEmpty(t, c.signingIdentity.value, "OnTransitionTo Active must set signing identity")
	assert.Equal(t, "node1", c.currentActiveCoordinator, "OnTransitionTo Active must set currentActiveCoordinator to self")
	// action_ImportStatesAndLocks ran: the grapher must now hold the imported state and lock.
	exported, err := c.grapher.ExportStatesAndLocks(ctx, "node1")
	require.NoError(t, err)
	assert.Len(t, exported.GetStates(), 1, "imported output state must be visible to node1")
	assert.Len(t, exported.GetLocks(), 1, "imported confirmed lock must be present in grapher")
}

func TestCoordinator_WhenPreparedReceivesClosingHeartbeat_ConfirmedTransactionsInSnapshot_CleanedUp(t *testing.T) {
	ctx := t.Context()

	// Seed a pooled transaction that is recorded as confirmed in the outgoing coordinator's snapshot.
	confirmedTx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	confirmedTxID := uuid.New()
	confirmedTx.EXPECT().GetID().Return(confirmedTxID).Maybe()
	confirmedTx.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	confirmedTx.EXPECT().GetSnapshot(mock.Anything).Return(&engineProto.SnapshotPooledTransaction{Id: confirmedTxID.String()}, nil, nil, nil).Maybe()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		PooledTransactions(confirmedTx).
		Build()

	event := &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing,
			ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{
				{SnapshotDispatchedTransaction: common.SnapshotDispatchedTransaction{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{ID: confirmedTxID},
				}},
			},
		},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "OnTransitionTo Active must set currentActiveCoordinator to self")
	// action_ProcessConfirmedTransactionsFromSnapshot must have cleaned up the confirmed transaction.
	assert.NotContains(t, c.transactionsByID, confirmedTxID, "confirmed transaction must be removed from transactionsByID")
}

func TestCoordinator_WhenPrepared_HeartbeatReceived_ClosingFlushFromCurrentActive_ResetsCounterAndStaysPrepared(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		HeartbeatIntervalsSinceLastReceive(5).
		InactiveGracePeriod(3).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing_Flush,
		},
	}))
	// ValidatorOr(IsFromCurrentActive AND IsClosingFlush, IsFromHigherPriority) → true
	// → action_ResetHeartbeatIntervalsSinceLastReceive fires; no transition.
	assert.Equal(t, State_Prepared, c.GetCurrentState())
	assert.Equal(t, 0, c.heartbeatIntervalsSinceLastReceive, "action_ResetHeartbeatIntervalsSinceLastReceive must clear the counter")
}

func TestCoordinator_WhenPrepared_HeartbeatReceived_HigherPriority_NoInflight_TransitionsToObserving(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	// guard_HasTransactionsInflight = false → GuardNot = true → Observing
	assert.Equal(t, State_Observing, c.GetCurrentState())
	assert.Nil(t, c.cancelRequestTimeout, "action_ClearTimeoutSchedules must run on Prepared → Observing")
	assert.Nil(t, c.cancelStateTimeout, "action_ClearTimeoutSchedules must run on Prepared → Observing")
}

func TestCoordinator_WhenPrepared_HeartbeatReceived_HigherPriority_HasInflightNoDispatched_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	txConfirmed := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txConfirmed.EXPECT().GetID().Return(txID).Maybe()
	txConfirmed.EXPECT().GetCurrentState().Return(transaction.State_Confirmed).Maybe()
	// GetSnapshot is called by action_SendHeartbeat in OnTransitionTo Closing.
	txConfirmed.EXPECT().GetSnapshot(mock.Anything).Return(nil, nil, nil, nil).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Transactions(txConfirmed).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	// action_CleanUpTransactionsNotYetDispatched preserves Confirmed txns.
	// guard_HasTransactionsInflight = true, guard_HasUnconfirmedDispatchedTransactions = false → Closing.
	assert.Equal(t, State_Closing, c.GetCurrentState())
}

func TestCoordinator_WhenPrepared_HeartbeatReceived_HigherPriority_HasInflightAndDispatched_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	// guard_HasTransactionsInflight = true, guard_HasUnconfirmedDispatchedTransactions = true → Closing_Flush.
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
}

func TestCoordinator_WhenPrepared_HeartbeatFromCurrentActiveCoordinator_HigherPriorityThanSelf_TransitionsToObserving(t *testing.T) {
	// Regression: node2 is in Prepared targeting node1 (currentActiveCoordinator), but node1 is also
	// higher-priority than node2. validator_IsHeartbeatFromHigherPriorityCoordinator must compare
	// against nodeName (node2), not currentActiveCoordinator (node1), so that IsHigherPriority
	// returns true and node2 steps back down.
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node1"). // the node we're trying to displace
		CoordinatorPriorityList("node1", "node2").
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	// validator_IsHeartbeatFromHigherPriorityCoordinator: IsHigherPriority(list, "node1", "node2") = 0 < 1 = true
	// guard_HasTransactionsInflight = false → Observing
	assert.Equal(t, State_Observing, c.GetCurrentState())
}

func TestCoordinator_WhenPrepared_HandoverRequest_HigherPriority_HasDispatched_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node3"). // for OnTransitionTo Closing_Flush heartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &HandoverRequestEvent{
		FromNode: "node1",
	}))
	// guard_HasUnconfirmedDispatchedTransactions = true → Closing_Flush
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the requesting node")
}

func TestCoordinator_WhenPrepared_HandoverRequest_HigherPriority_NoDispatched_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node3"). // for OnTransitionTo Closing heartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &HandoverRequestEvent{
		FromNode: "node1",
	}))
	// guard_HasUnconfirmedDispatchedTransactions = false → Closing
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the requesting node")
}

func TestCoordinator_WhenPrepared_DelegatedTransactions_AcceptsAndStaysPrepared(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-2",
	}))
	assert.Equal(t, State_Prepared, c.GetCurrentState())
}

func TestCoordinator_WhenPrepared_TransactionsDelegated_BlockHeightToleranceExceeded_RejectsAndStaysPrepared(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendDelegationRejection(
		mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
			return msg.DelegationId == "del-bh-test" &&
				msg.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
				msg.OriginatorBlockHeight == int64(0) &&
				msg.CoordinatorBlockHeight == int64(100) &&
				msg.BlockHeightTolerance == int64(10)
		}),
	).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, newDelegationBlockHeightExceedingEvent("originator-node")))
	assert.Equal(t, State_Prepared, c.GetCurrentState())
}

func TestCoordinator_WhenPrepared_TransactionStateTransition_ToFinal_CleansUpAndStaysPrepared(t *testing.T) {
	ctx := t.Context()
	txFinal := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txFinal.EXPECT().GetID().Return(txID).Maybe()
	txFinal.EXPECT().GetCurrentState().Return(transaction.State_Final).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Transactions(txFinal).
		Build()
	require.Equal(t, 1, len(c.transactionsByID))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Final,
	}))
	assert.Equal(t, State_Prepared, c.GetCurrentState())
	assert.Empty(t, c.transactionsByID, "action_CleanUpTransaction must remove the finalised transaction")
}

func TestCoordinator_WhenPrepared_TransactionStateTransition_ToPooled_AddsToPool(t *testing.T) {
	ctx := t.Context()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	tx.EXPECT().GetID().Return(txID).Maybe()
	tx.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Transactions(tx).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Pooled,
	}))
	assert.Equal(t, State_Prepared, c.GetCurrentState())
	assert.Contains(t, c.pooledTransactions, tx, "action_PoolTransaction must add the transaction to the pool")
}

func TestCoordinator_WhenPreparedTransitionsToActive_RefreshesSigningIdentityAndSelectsPooledTransactions(t *testing.T) {
	ctx := t.Context()
	pooledTx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	pooledTxID := uuid.New()
	pooledTx.EXPECT().GetID().Return(pooledTxID).Maybe()
	pooledTx.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	// action_SendHeartbeat (OnTransitionTo Active) builds the payload by calling GetSnapshot on each transaction.
	pooledTx.EXPECT().GetSnapshot(mock.Anything).Return(&engineProto.SnapshotPooledTransaction{Id: pooledTxID.String()}, nil, nil, nil).Maybe()
	pooledTx.EXPECT().HandleEvent(mock.Anything, mock.AnythingOfType("*transaction.SelectedEvent")).Return(nil).Once()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2"). // node2 is the outgoing coordinator
		PooledTransactions(pooledTx).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Closing,
		},
	}))
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.NotEmpty(t, c.signingIdentity.value)
	assert.Equal(t, "node1", c.currentActiveCoordinator, "OnTransitionTo Active must set currentActiveCoordinator to self")
	// mock expectation on pooledTx.HandleEvent(SelectedEvent) verified by testify
}

func TestCoordinator_WhenPrepared_EndorsementRequestReceived_HigherPriority_NoInflight_TransitionsToObserving(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Observing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenPrepared_EndorsementRequestReceived_HigherPriority_InflightAndUnconfirmed_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	tx, txID := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Transactions(tx).
		Build()
	c.inFlightTxns[txID] = struct{}{} // mark as unconfirmed dispatched

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenPrepared_EndorsementRequestReceived_HigherPriority_InflightNoUnconfirmed_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	tx, _ := newDispatchedTxMock(t)
	tx.EXPECT().GetCurrentState().Unset()
	tx.EXPECT().GetCurrentState().Return(transaction.State_Confirmed).Maybe()

	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Transactions(tx).
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenPrepared_EndorsementRequestReceived_LowerPriority_RejectsAsActiveCoordinator(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-lp-test", "ik-lp-test",
			engineProto.RejectionReason_ENDORSER_IS_ACTIVE_COORDINATOR, int64(0), int64(0), int64(0)),
	).Return(nil)

	event := newLowerPriorityEndorsementEvent(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Prepared, c.GetCurrentState())
	assert.Equal(t, "node2", c.currentActiveCoordinator, "lower-priority sender must not update active coordinator")
}

func TestCoordinator_WhenPrepared_EndorsementRequestReceived_BlockHeightToleranceExceeded_RejectsAndStaysPrepared(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-bh-test", "ik-bh-test",
			engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, int64(0), int64(100), int64(10)),
	).Return(nil)

	event := newBlockHeightExceedingEndorsementEvent("node1", mocks) // higher priority — rejected before priority check
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Prepared, c.GetCurrentState())
	assert.Equal(t, "node2", c.currentActiveCoordinator)
}

func TestCoordinator_WhenPrepared_EndorsementRequestReceived_UpdatesEndorserCandidatesFromSender(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Contains(t, c.endorserCandidates, "node3")
}

func TestCoordinator_WhenPrepared_EndorsementRequestReceived_PrivateStateDataPending_RejectsAndStaysPrepared(t *testing.T) {
	// Private state check is evaluated before the priority check — even a higher-priority sender
	// is rejected if private state is incomplete.
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Prepared).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		BlockHeightTolerance(1000). // prevent block-height check from firing first
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, nil)
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-1", "ik-1",
			engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING, int64(100), int64(0), int64(10)),
	).Return(nil)

	event := &EndorsementRequestReceivedEvent{
		FromNode:                  "node1", // higher priority — rejected before priority check
		TransactionId:             "tx-1",
		IdempotencyKey:            "ik-1",
		Party:                     "party1@node1",
		CoordinatorBlockHeight:    100,
		BlockHeightTolerance:      10, // lowWatermark = 90
		AttestationRequest:        &prototk.AttestationRequest{Name: "att1"},
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Prepared, c.GetCurrentState())
	assert.Equal(t, "node2", c.currentActiveCoordinator)
}

func TestCoordinator_WhenActive_TransitionsToIdle_OnHeartbeatInterval_WhenNoInflight(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		EndorserCandidates("node2"). // gives action_SendHeartbeat a recipient
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	// guard_HasTransactionsInflight = false (no txns); GuardNot(HasTransactionsInflight) = true
	assert.Equal(t, State_Idle, c.GetCurrentState())
	// action_SendHeartbeat ran during the Active HeartbeatInterval handler
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "action_SendHeartbeat must fire in Active HeartbeatInterval")
}

func TestCoordinator_WhenActive_HeartbeatInterval_WithInflight_SendsHeartbeatAndStaysActive(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		EndorserCandidates("node2"). // for action_SendHeartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	// guard_HasTransactionsInflight = true → GuardNot = false → no Idle transition
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "action_SendHeartbeat must fire in Active HeartbeatInterval")
}

func TestCoordinator_WhenActive_HigherPriorityHeartbeat_WithUnconfirmedDispatchedTx_CleansUpPooledAndTransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	pooledTx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	pooledTxID := uuid.New()
	pooledTx.EXPECT().GetID().Return(pooledTxID).Maybe()
	pooledTx.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	dispatchedTx, _ := newDispatchedTxMock(t)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node3"). // node3 is lower priority than node1
		CurrentActiveCoordinator("node3").
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		PooledTransactions(pooledTx).
		Transactions(dispatchedTx).
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()
	require.Equal(t, 2, len(c.transactionsByID), "pooled and dispatched txns must be registered before event")
	// node1 sends an Active heartbeat while node3 is coordinating → preemption
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	// action_CleanUpTransactionsNotYetDispatched removed the pooled tx; dispatched tx remains → flush required
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, uint64(1), uint64(len(c.transactionsByID)), "only pooled tx must be cleaned up; dispatched tx remains for flush")
}

func TestCoordinator_WhenActive_HigherPriorityHeartbeat_CleansUpPooledAndTransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	pooledTx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	pooledTxID := uuid.New()
	pooledTx.EXPECT().GetID().Return(pooledTxID).Maybe()
	pooledTx.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node3"). // node3 is lower priority than node1
		CurrentActiveCoordinator("node3").
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		PooledTransactions(pooledTx).
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()
	require.Equal(t, 1, len(c.transactionsByID), "pooled tx must be registered before event")
	// node1 sends an Active heartbeat while node3 is coordinating → preemption
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	// action_CleanUpTransactionsNotYetDispatched removed the pooled transaction; no dispatched txns → no flush needed
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, uint64(0), uint64(len(c.transactionsByID)), "pooled txns must be cleaned up on preemption")
}

func TestCoordinator_WhenActive_HeartbeatReceived_NotHigherPriority_IgnoresAndStaysActive(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()
	// node2 has lower priority than node1 (higher index); validator fails → event ignored
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	assert.Equal(t, State_Active, c.GetCurrentState())
}

func TestCoordinator_WhenActive_HandoverRequest_HigherPriority_HasDispatched_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node1"). // for OnTransitionTo Closing_Flush heartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &HandoverRequestEvent{
		FromNode: "node1",
	}))
	// guard_HasUnconfirmedDispatchedTransactions = true → Closing_Flush
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the requesting node")
}

func TestCoordinator_WhenActive_HandoverRequest_HigherPriority_NoDispatched_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node1"). // for OnTransitionTo Closing heartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &HandoverRequestEvent{
		FromNode: "node1",
	}))
	// guard_HasUnconfirmedDispatchedTransactions = false → Closing
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the requesting node")
}

func TestCoordinator_WhenActive_DelegatedTransactions_AcceptsAndStaysActive(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-3",
	}))
	assert.Equal(t, State_Active, c.GetCurrentState())
}

func TestCoordinator_WhenActive_TransactionsDelegated_BlockHeightToleranceExceeded_RejectsAndStaysActive(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendDelegationRejection(
		mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
			return msg.DelegationId == "del-bh-test" &&
				msg.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
				msg.OriginatorBlockHeight == int64(0) &&
				msg.CoordinatorBlockHeight == int64(100) &&
				msg.BlockHeightTolerance == int64(10)
		}),
	).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, newDelegationBlockHeightExceedingEvent("originator-node")))
	assert.Equal(t, State_Active, c.GetCurrentState())
}

func TestCoordinator_WhenActive_TransactionStateTransition_DispatchedToPooled_WithAssembling_CancelsAssembling(t *testing.T) {
	ctx := t.Context()
	txAssembling := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txAssemblingID := uuid.New()
	txAssembling.EXPECT().GetID().Return(txAssemblingID).Maybe()
	txAssembling.EXPECT().GetCurrentState().Return(transaction.State_Assembling).Maybe()
	// action_cancelCurrentlyAssemblingTransaction sends AssembleCancelledEvent to the assembling tx.
	txAssembling.EXPECT().HandleEvent(mock.Anything, mock.AnythingOfType("*transaction.AssembleCancelledEvent")).Return(nil).Once()

	txDispatched, txDispatchedID := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txAssembling, txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txDispatchedID,
		FromState:     transaction.State_Dispatched,
		ToState:       transaction.State_Pooled,
	}))
	// action_cancelCurrentlyAssemblingTransaction fired — testify verifies the .Once() expectation.
	// action_SelectTransaction is suppressed because guard_HasTransactionAssembling was still true.
	assert.Equal(t, State_Active, c.GetCurrentState())
}

func TestCoordinator_WhenActive_TransactionStateTransition_ToPooled_PoolsAndSelectsTransaction(t *testing.T) {
	ctx := t.Context()
	txReverting := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txReverting.EXPECT().GetID().Return(txID).Maybe()
	txReverting.EXPECT().GetCurrentState().Return(transaction.State_Dispatched).Maybe()
	txReverting.EXPECT().GetSnapshot(mock.Anything).Return(nil, &engineProto.SnapshotDispatchedTransaction{Id: txID.String()}, nil, nil).Maybe()
	// action_SelectTransaction will call HandleEvent(SelectedEvent) after pooling.
	txReverting.EXPECT().HandleEvent(mock.Anything, mock.AnythingOfType("*transaction.SelectedEvent")).Return(nil).Once()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txReverting).
		Build()
	require.Equal(t, 0, len(c.pooledTransactions))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Pooled,
	}))
	// action_PoolTransaction added it to pool; action_SelectTransaction immediately selected it.
	assert.Equal(t, State_Active, c.GetCurrentState())
}

func TestCoordinator_WhenActive_TransactionStateTransition_ToReadyForDispatch_QueuesForDispatch(t *testing.T) {
	ctx := t.Context()
	txReady := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txReady.EXPECT().GetID().Return(txID).Maybe()
	txReady.EXPECT().GetCurrentState().Return(transaction.State_Ready_For_Dispatch).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txReady).
		Build()
	require.Equal(t, 0, len(c.dispatchQueue))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Ready_For_Dispatch,
	}))
	// action_QueueTransactionForDispatch placed the tx on the dispatch channel.
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Equal(t, 1, len(c.dispatchQueue), "transaction must be queued for dispatch")
}

func TestCoordinator_WhenActive_TransactionStateTransition_ToFinal_CleansUpAndStaysActive(t *testing.T) {
	ctx := t.Context()
	txFinal := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txFinal.EXPECT().GetID().Return(txID).Maybe()
	txFinal.EXPECT().GetCurrentState().Return(transaction.State_Final).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txFinal).
		Build()
	require.Equal(t, 1, len(c.transactionsByID))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Final,
	}))
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Empty(t, c.transactionsByID, "action_CleanUpTransaction must remove the finalised transaction")
}

func TestCoordinator_WhenActive_TransactionStateTransition_ToEvicted_CleansUpAndStaysActive(t *testing.T) {
	ctx := t.Context()
	txEvicted := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txEvicted.EXPECT().GetID().Return(txID).Maybe()
	txEvicted.EXPECT().GetCurrentState().Return(transaction.State_Evicted).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txEvicted).
		Build()
	require.Equal(t, 1, len(c.transactionsByID))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Evicted,
	}))
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Empty(t, c.transactionsByID, "action_CleanUpTransaction must remove the evicted transaction")
}

func TestCoordinator_WhenActiveAndEpochChangesWithSigningUsedAndInflightDispatched_TransitionsToActiveFLush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CurrentBlockHeight(150). // effectiveBlockHeight is already updated by getAndRefreshBlockHeight
		CoordinatorSelectionBlockRange(50).
		SigningIdentityUsed(true). // a transaction used the signing identity since the last rotation
		Transactions(txDispatched).
		Build()
	// EpochBoundaryReachedEvent is queued by getAndRefreshBlockHeight when the epoch changes.
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &EpochBoundaryReachedEvent{}))
	// guard_SigningIdentityUsed = true, guard_FlushComplete = false → key-rotation Active_Flush
	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
}

func TestCoordinator_WhenActiveAndEpochChangesWithSigningUsedAndOnlyPooledTxs_RotatesKeyInPlace(t *testing.T) {
	ctx := t.Context()
	pooledTx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	pooledTxID := uuid.New()
	pooledTx.EXPECT().GetID().Return(pooledTxID).Maybe()
	pooledTx.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CurrentBlockHeight(150). // effectiveBlockHeight is already updated by getAndRefreshBlockHeight
		CoordinatorSelectionBlockRange(50).
		SigningIdentityUsed(true). // a transaction retrieved the signing identity since the last rotation
		PooledTransactions(pooledTx).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	prevIdentity := c.signingIdentity.value
	require.Equal(t, 1, len(c.transactionsByID), "pooled tx must be registered before event")
	// EpochBoundaryReachedEvent is queued by getAndRefreshBlockHeight when the epoch changes.
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &EpochBoundaryReachedEvent{}))
	// guard_SigningIdentityUsed = true, guard_FlushComplete = true → action_NewSigningIdentity (in-place rotation)
	assert.Equal(t, State_Active, c.GetCurrentState(), "key-rotation does not change state when flush is already complete")
	// action_NewSigningIdentity rotated the key in place
	assert.NotEqual(t, prevIdentity, c.signingIdentity.value, "signing identity must be refreshed on epoch boundary when used")
	// The pooled transaction stays in the pool: transaction selection is independent of key rotation.
	// When it is eventually dispatched it will call getCoordinatorSigningIdentity and receive the new key.
	assert.Equal(t, 1, len(c.transactionsByID), "transaction must remain registered after in-place key rotation")
	assert.Equal(t, 1, len(c.pooledTransactions), "pooled transaction stays in pool; selection is not triggered by key rotation")
}

func TestCoordinator_WhenActiveAndEpochChanges_SigningNotUsed_StaysActiveAndRotatesKey(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CurrentBlockHeight(150). // effectiveBlockHeight is already updated by getAndRefreshBlockHeight
		CoordinatorSelectionBlockRange(50).
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	prevIdentity := c.signingIdentity.value
	// EpochBoundaryReachedEvent is queued by getAndRefreshBlockHeight when the epoch changes.
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &EpochBoundaryReachedEvent{}))
	assert.Equal(t, State_Active, c.GetCurrentState())
	// Key is rotated on every epoch boundary, even when it was not used
	assert.NotEqual(t, prevIdentity, c.signingIdentity.value, "signing identity must be rotated on every epoch boundary")
	// Note: calculateCoordinatorPriorities is called in getAndRefreshBlockHeight (before the event),
	// not in the EpochBoundaryReachedEvent handler. effectiveBlockHeight is already set.
	assert.Equal(t, uint64(150), c.effectiveBlockHeight, "effective block height must have been set by builder")
}

func TestCoordinator_WhenActive_RestartDispatchLoop_StaysActive(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Build()

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &RestartDispatchLoopEvent{}))
	// action_StartDispatchLoop is a no-op when c.ctx == nil (coordinator not started), so no
	// goroutine cleanup is needed; the coordinator must remain in State_Active.
	assert.Equal(t, State_Active, c.GetCurrentState())
}

func TestCoordinator_WhenActive_EndorsementRequestReceived_HigherPriority_UnconfirmedDispatched_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	tx, txID := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2").
		Transactions(tx).
		Build()
	c.inFlightTxns[txID] = struct{}{}

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenActive_EndorsementRequestReceived_HigherPriority_NoUnconfirmedDispatched_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2").
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenActive_EndorsementRequestReceived_FromSelf_HandlesWithoutSteppingDown(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "self-request must not change active coordinator")
}

func TestCoordinator_WhenActive_EndorsementRequestReceived_LowerPriority_RejectsAsActiveCoordinator(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-lp-test", "ik-lp-test",
			engineProto.RejectionReason_ENDORSER_IS_ACTIVE_COORDINATOR, int64(0), int64(0), int64(0)),
	).Return(nil)
	// After rejecting a lower-priority sender, Active reasserts its coordinator status to all candidates.
	mocks.TransportWriter.EXPECT().SendHeartbeat(mock.Anything, "node1", mock.Anything).Return(nil)
	mocks.TransportWriter.EXPECT().SendHeartbeat(mock.Anything, "node2", mock.Anything).Return(nil)
	mocks.TransportWriter.EXPECT().SendHeartbeat(mock.Anything, "node3", mock.Anything).Return(nil)

	event := newLowerPriorityEndorsementEvent(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "lower-priority sender must not update active coordinator or trigger step-down")
}

func TestCoordinator_WhenActive_EndorsementRequestReceived_BlockHeightToleranceExceeded_RejectsAndStaysActive(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-bh-test", "ik-bh-test",
			engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, int64(0), int64(100), int64(10)),
	).Return(nil)

	event := newBlockHeightExceedingEndorsementEvent("node2", mocks)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenActive_EndorsementRequestReceived_UpdatesEndorserCandidatesFromSender(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Contains(t, c.endorserCandidates, "node3")
}

func TestCoordinator_WhenActive_EndorsementRequestReceived_PrivateStateDataPending_RejectsAndStaysActive(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		BlockHeightTolerance(1000). // prevent block-height check from firing first
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, nil)
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-1", "ik-1",
			engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING, int64(100), int64(0), int64(10)),
	).Return(nil)

	event := &EndorsementRequestReceivedEvent{
		FromNode:                  "node2",
		TransactionId:             "tx-1",
		IdempotencyKey:            "ik-1",
		Party:                     "party1@node2",
		CoordinatorBlockHeight:    100,
		BlockHeightTolerance:      10, // lowWatermark = 90
		AttestationRequest:        &prototk.AttestationRequest{Name: "att1"},
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenActiveFLush_HeartbeatInterval_SendsHeartbeatAndStaysActiveFLush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		EndorserCandidates("node2"). // for action_SendHeartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "action_SendHeartbeat must fire in Active_Flush HeartbeatInterval")
}

func TestCoordinator_WhenActiveFLush_HigherPriorityHeartbeat_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node1", "node2"). // for OnTransitionTo Closing_Flush heartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the new active node")
}

func TestCoordinator_WhenActiveFLush_HandoverRequest_HigherPriority_HasDispatched_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node1"). // for OnTransitionTo Closing_Flush heartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &HandoverRequestEvent{
		FromNode: "node1",
	}))
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the requesting node")
}

func TestCoordinator_WhenActiveFLush_HandoverRequest_HigherPriority_NoDispatched_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node1"). // for OnTransitionTo Closing heartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &HandoverRequestEvent{
		FromNode: "node1",
	}))
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the requesting node")
}

func TestCoordinator_WhenActiveFLush_DelegatedTransactions_AcceptsAndStaysActiveFLush(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-4",
	}))
	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
}

func TestCoordinator_WhenActiveFLush_TransactionsDelegated_BlockHeightToleranceExceeded_RejectsAndStaysActiveFlush(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendDelegationRejection(
		mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
			return msg.DelegationId == "del-bh-test" &&
				msg.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
				msg.OriginatorBlockHeight == int64(0) &&
				msg.CoordinatorBlockHeight == int64(100) &&
				msg.BlockHeightTolerance == int64(10)
		}),
	).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, newDelegationBlockHeightExceedingEvent("originator-node")))
	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
}

func TestCoordinator_WhenActiveFLushCompletesAndStillCurrentCoordinator_TransitionsToActive(t *testing.T) {
	ctx := t.Context()
	txDispatched, txID := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txDispatched).
		Build()
	event := &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		FromState:     transaction.State_Dispatched,
		ToState:       transaction.State_Final,
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.NotEmpty(t, c.signingIdentity.value, "OnTransitionTo Active must set signing identity")
}

func TestCoordinator_WhenActiveFLush_TransactionStateTransition_DispatchedToPooled_WithAssembling_StaysActiveFLush(t *testing.T) {
	ctx := t.Context()
	txAssembling := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txAssemblingID := uuid.New()
	txAssembling.EXPECT().GetID().Return(txAssemblingID).Maybe()
	txAssembling.EXPECT().GetCurrentState().Return(transaction.State_Assembling).Maybe()
	txAssembling.EXPECT().HandleEvent(mock.Anything, mock.AnythingOfType("*transaction.AssembleCancelledEvent")).Return(nil).Once()

	txDispatched1, txDispatched1ID := newDispatchedTxMock(t)
	txDispatched2, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txAssembling, txDispatched1, txDispatched2).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txDispatched1ID,
		FromState:     transaction.State_Dispatched,
		ToState:       transaction.State_Pooled,
	}))
	// txDispatched2 still in memory → guard_HasUnconfirmedDispatchedTransactions = true → stays Active_Flush.
	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
}

func TestCoordinator_WhenActiveFLush_TransactionStateTransition_ToReadyForDispatch_QueuesAndStaysActiveFLush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	txReady := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txReadyID := uuid.New()
	txReady.EXPECT().GetID().Return(txReadyID).Maybe()
	txReady.EXPECT().GetCurrentState().Return(transaction.State_Ready_For_Dispatch).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txDispatched, txReady).
		Build()
	require.Equal(t, 0, len(c.dispatchQueue))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txReadyID,
		ToState:       transaction.State_Ready_For_Dispatch,
	}))
	// txDispatched still in memory → stays Active_Flush; txReady was queued for dispatch.
	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
	assert.Equal(t, 1, len(c.dispatchQueue), "action_QueueTransactionForDispatch must enqueue the transaction")
}

func TestCoordinator_WhenActiveFLush_TransactionStateTransition_ToEvicted_CleansUpAndStaysActiveFLush(t *testing.T) {
	ctx := t.Context()
	txDispatched1, txDispatched1ID := newDispatchedTxMock(t)
	txDispatched2, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txDispatched1, txDispatched2).
		Build()
	require.Equal(t, 2, len(c.transactionsByID))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txDispatched1ID,
		ToState:       transaction.State_Evicted,
	}))
	// txDispatched2 still in memory → guard_HasUnconfirmedDispatchedTransactions = true → stays Active_Flush.
	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
	assert.Equal(t, 1, len(c.transactionsByID), "action_CleanUpTransaction must remove the evicted transaction")
}

func TestCoordinator_WhenActiveFLush_TransactionStateTransition_ToFinal_WithMoreDispatched_StaysActiveFLush(t *testing.T) {
	ctx := t.Context()
	txDispatched1, txDispatched1ID := newDispatchedTxMock(t)
	txDispatched2, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		Transactions(txDispatched1, txDispatched2).
		Build()
	require.Equal(t, 2, len(c.transactionsByID))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txDispatched1ID,
		ToState:       transaction.State_Final,
	}))
	// txDispatched2 still dispatched → stays Active_Flush.
	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
	assert.Equal(t, 1, len(c.transactionsByID))
}

func TestCoordinator_WhenActiveFLush_EndorsementRequestReceived_HigherPriority_TransitionsToClosingFlush(t *testing.T) {
	ctx := t.Context()
	tx, txID := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2").
		Transactions(tx).
		Build()
	c.inFlightTxns[txID] = struct{}{}

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenActiveFlush_EndorsementRequestReceived_FromSelf_HandlesWithoutSteppingDown(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "self-request must not change active coordinator")
}

func TestCoordinator_WhenActiveFlush_EndorsementRequestReceived_LowerPriority_RejectsAsActiveCoordinator(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-lp-test", "ik-lp-test",
			engineProto.RejectionReason_ENDORSER_IS_ACTIVE_COORDINATOR, int64(0), int64(0), int64(0)),
	).Return(nil)

	event := newLowerPriorityEndorsementEvent(t, "node2", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenActiveFLush_EndorsementRequestReceived_BlockHeightToleranceExceeded_RejectsAndStaysActiveFLush(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-bh-test", "ik-bh-test",
			engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, int64(0), int64(100), int64(10)),
	).Return(nil)

	event := newBlockHeightExceedingEndorsementEvent("node2", mocks)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenActiveFlush_EndorsementRequestReceived_UpdatesEndorserCandidatesFromSender(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Contains(t, c.endorserCandidates, "node3")
}

func TestCoordinator_WhenActiveFlush_EndorsementRequestReceived_PrivateStateDataPending_RejectsAndStaysActiveFlush(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		BlockHeightTolerance(1000). // prevent block-height check from firing first
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, nil)
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-1", "ik-1",
			engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING, int64(100), int64(0), int64(10)),
	).Return(nil)

	event := &EndorsementRequestReceivedEvent{
		FromNode:                  "node2",
		TransactionId:             "tx-1",
		IdempotencyKey:            "ik-1",
		Party:                     "party1@node2",
		CoordinatorBlockHeight:    100,
		BlockHeightTolerance:      10, // lowWatermark = 90
		AttestationRequest:        &prototk.AttestationRequest{Name: "att1"},
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Active_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenEnteringClosingFlush_OnTransitionTo_SendsImmediateHeartbeat(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node2").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Transactions(txDispatched).
		Build()
	// Trigger transition Active → Closing_Flush via a higher-priority heartbeat.
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	// OnTransitionTo Closing_Flush fires action_SendHeartbeat immediately.
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "OnTransitionTo Closing_Flush must emit an immediate heartbeat")
}

func TestCoordinator_WhenClosingFlush_HeartbeatInterval_IncrementsCounterAndSendsHeartbeat(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		EndorserCandidates("node2"). // for action_SendHeartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		HeartbeatIntervalsSinceStateChange(0).
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, 1, c.heartbeatIntervalsSinceStateChange)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func TestCoordinator_WhenClosingFlush_HeartbeatReceived_FromLiveSender_ResetsCounterAndStaysClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		HeartbeatIntervalsSinceLastReceive(5).
		Transactions(txDispatched).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, 0, c.heartbeatIntervalsSinceLastReceive, "action_ResetHeartbeatIntervalsSinceLastReceive must clear the counter")
}

func TestCoordinator_WhenClosingFlush_DelegatedTransactions_HigherPriority_TransitionsToElect(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node2"). // node1 re-selected as higher priority
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		WithMockTransportWriter().
		Transactions(txDispatched).
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	mocks.TransportWriter.EXPECT().SendDelegationResponse(mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationResponse) bool {
		return msg.DelegationId == "del-1"
	})).Return(nil)
	mocks.TransportWriter.EXPECT().SendHandoverRequest(mock.Anything, "node2", mock.Anything).Return(nil)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-1",
	}))
	assert.Equal(t, State_Elect, c.GetCurrentState())
}

func TestCoordinator_WhenClosingFlush_DelegatedTransactions_LowerPriority_RejectsAndStaysClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched, _ := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node3"). // lower priority than node1
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		WithMockTransportWriter().
		Transactions(txDispatched).
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	mocks.TransportWriter.EXPECT().SendDelegationRejection(mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
		return msg.DelegationId == "del-2" && msg.ActiveCoordinator == "node1" && msg.RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE
	})).Return(nil)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-2",
	}))
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
}

func TestCoordinator_WhenClosingFlush_TransactionsDelegated_BlockHeightToleranceExceeded_RejectsAndStaysClosingFlush(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node2").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendDelegationRejection(
		mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
			return msg.DelegationId == "del-bh-test" &&
				msg.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
				msg.OriginatorBlockHeight == int64(0) &&
				msg.CoordinatorBlockHeight == int64(100) &&
				msg.BlockHeightTolerance == int64(10)
		}),
	).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, newDelegationBlockHeightExceedingEvent("originator-node")))
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	// active coordinator must not be updated on a block height rejection
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenClosingFlushCompletesAndNotCurrentCoordinator_TransitionsToClosing(t *testing.T) {
	ctx := t.Context()
	txDispatched, txID := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node2"). // another node is now current
		Transactions(txDispatched).
		Build()
	event := &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		FromState:     transaction.State_Dispatched,
		ToState:       transaction.State_Final,
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, State_Closing, c.GetCurrentState())
}

func TestCoordinator_WhenClosingFlush_TransactionStateTransition_DispatchedToRetryable_CleansUpAndStaysClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched1, txDispatched1ID := newDispatchedTxMock(t)
	txDispatched2, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Transactions(txDispatched1, txDispatched2).
		Build()
	require.Equal(t, 2, len(c.transactionsByID))
	// From=Dispatched, To=Pooled is retryable (not Confirmed/Reverted) → action_CleanUpTransaction fires.
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txDispatched1ID,
		FromState:     transaction.State_Dispatched,
		ToState:       transaction.State_Pooled,
	}))
	// txDispatched2 still dispatched → guard_HasUnconfirmedDispatchedTransactions = true → stays Closing_Flush.
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, 1, len(c.transactionsByID), "action_CleanUpTransaction must remove the retryable transaction")
}

func TestCoordinator_WhenClosingFlush_TransactionStateTransition_ToFinal_WithMoreDispatched_StaysClosingFlush(t *testing.T) {
	ctx := t.Context()
	txDispatched1, txDispatched1ID := newDispatchedTxMock(t)
	txDispatched2, _ := newDispatchedTxMock(t)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Transactions(txDispatched1, txDispatched2).
		Build()
	require.Equal(t, 2, len(c.transactionsByID))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txDispatched1ID,
		ToState:       transaction.State_Final,
	}))
	// txDispatched2 still dispatched → stays Closing_Flush.
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, 1, len(c.transactionsByID))
}

func TestCoordinator_WhenClosingFlushCompletesAndNotCurrentCoordinator_EnteringClosingEmitsImmediateHeartbeat(t *testing.T) {
	ctx := t.Context()
	txDispatched, txID := newDispatchedTxMock(t)
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node1").
		CurrentActiveCoordinator("node2"). // another node is now active; node1 is stepping down
		EndorserCandidates("node2").       // gives action_SendHeartbeat a recipient
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Transactions(txDispatched).
		Build()
	// Finalising the last dispatched transaction triggers guard_FlushComplete = true
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		FromState:     transaction.State_Dispatched,
		ToState:       transaction.State_Final,
	}))
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat(), "OnTransitionTo Closing must emit an immediate heartbeat")
}

func TestCoordinator_WhenClosingFlush_EndorsementRequestReceived_UpdatesActiveCoordinatorAndStaysClosingFlush(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenClosingFlush_EndorsementRequestReceived_BlockHeightToleranceExceeded_RejectsAndStaysClosingFlush(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-bh-test", "ik-bh-test",
			engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, int64(0), int64(100), int64(10)),
	).Return(nil)

	event := newBlockHeightExceedingEndorsementEvent("node1", mocks)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	// Rejection path: active coordinator must not be updated.
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node3", c.currentActiveCoordinator)
}

func TestCoordinator_WhenClosingFlush_EndorsementRequestReceived_UpdatesEndorserCandidatesFromSender(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node2").
		EndorserCandidates("node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Contains(t, c.endorserCandidates, "node3")
}

func TestCoordinator_WhenClosingFlush_EndorsementRequestReceived_PrivateStateDataPending_RejectsAndStaysClosingFlush(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		BlockHeightTolerance(1000). // prevent block-height check from firing first
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, nil)
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-1", "ik-1",
			engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING, int64(100), int64(0), int64(10)),
	).Return(nil)

	event := &EndorsementRequestReceivedEvent{
		FromNode:                  "node1",
		TransactionId:             "tx-1",
		IdempotencyKey:            "ik-1",
		Party:                     "party1@node1",
		CoordinatorBlockHeight:    100,
		BlockHeightTolerance:      10, // lowWatermark = 90
		AttestationRequest:        &prototk.AttestationRequest{Name: "att1"},
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	// Rejection path: active coordinator must not be updated.
	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node3", c.currentActiveCoordinator)
}

func TestCoordinator_WhenClosingFlush_HeartbeatReceived_LiveSender_UpdatesActiveCoordinator(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		HeartbeatIntervalsSinceLastReceive(5).
		Build()

	event := &common.HeartbeatReceivedEvent{
		FromNode: "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator, "active coordinator must be updated on live heartbeat in Closing_Flush")
	assert.Equal(t, 0, c.heartbeatIntervalsSinceLastReceive, "liveness counter must be reset")
}

func TestCoordinator_WhenClosingFlush_HeartbeatReceived_UpdatesEndorserCandidates(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing_Flush).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorPriorityList("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState:   common.CoordinatorState_Idle,
			EndorserCandidates: []string{"node1", "node2"},
		},
	}))

	assert.Equal(t, State_Closing_Flush, c.GetCurrentState())
	assert.Contains(t, c.endorserCandidates, "node2")
}

func TestCoordinator_WhenClosing_HeartbeatReceived_FromLiveSender_ResetsCounterAndUpdatesCoordinator(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		HeartbeatIntervalsSinceLastReceive(5).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}))
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, 0, c.heartbeatIntervalsSinceLastReceive, "action_ResetHeartbeatIntervalsSinceLastReceive must clear the counter")
	assert.Equal(t, "node2", c.currentActiveCoordinator, "action_UpdateActiveCoordinator must record the sender")
}

func TestCoordinator_WhenClosing_HeartbeatReceived_UpdatesEndorserCandidates(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorPriorityList("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatReceivedEvent{
		FromNode: "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState:   common.CoordinatorState_Idle,
			EndorserCandidates: []string{"node1", "node2"},
		},
	}))

	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Contains(t, c.endorserCandidates, "node2")
}

func TestCoordinator_WhenClosingGraceExpires_WithNewActiveHeartbeatSeen_TransitionsToObserving(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		ClosingGracePeriod(1).
		HeartbeatIntervalsSinceStateChange(1).
		HeartbeatIntervalsSinceLastReceive(0). // recent heartbeat seen
		InactiveGracePeriod(5).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	// heartbeatIntervalsSinceStateChange → 2; closingGracePeriodExpired = 2>1 true
	// heartbeatIntervalsSinceLastReceive = 0; inactiveGracePeriodExceeded = 0>5 false
	assert.Equal(t, State_Observing, c.GetCurrentState())
}

func TestCoordinator_WhenClosingGraceExpires_WithoutNewActiveHeartbeat_TransitionsToIdle(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		ClosingGracePeriod(1).
		HeartbeatIntervalsSinceStateChange(1).
		HeartbeatIntervalsSinceLastReceive(6). // no heartbeat seen; counter already past grace
		InactiveGracePeriod(5).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	// heartbeatIntervalsSinceStateChange → 2; closingGracePeriodExpired = 2>1 true
	// heartbeatIntervalsSinceLastReceive = 6 (not incremented by this action); inactiveGracePeriodExceeded = 6>5 true
	assert.Equal(t, State_Idle, c.GetCurrentState())
}

func TestCoordinator_WhenClosing_HeartbeatInterval_GraceNotExpired_SendsHeartbeatAndStaysClosing(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing).
		ClosingGracePeriod(5).
		HeartbeatIntervalsSinceStateChange(0). // after increment: 1 < 5 → grace not expired
		HeartbeatIntervalsSinceLastReceive(0).
		InactiveGracePeriod(3).
		EndorserCandidates("node2"). // for action_SendHeartbeat
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, 1, c.heartbeatIntervalsSinceStateChange)
	assert.True(t, mocks.SentMessageRecorder.HasSentHeartbeat())
}

func TestCoordinator_WhenClosing_DelegationRequest_HigherPriorityThanCurrentActive_TransitionsToElect(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node1").
		CurrentActiveCoordinator("node2"). // node1 is now higher priority
		CoordinatorPriorityList("node1", "node2", "node3").
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		WithMockTransportWriter().
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	// action_ProcessDelegatedTransactions always sends an acknowledgment (even for empty transaction lists).
	mocks.TransportWriter.EXPECT().SendDelegationResponse(mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationResponse) bool {
		return msg.DelegationId == "del-1"
	})).Return(nil)
	mocks.TransportWriter.EXPECT().SendHandoverRequest(mock.Anything, "node2", mock.Anything).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-1",
	}))
	// guard_IsHigherPriorityThanCurrentActive = true (node1 < node2) → Elect
	assert.Equal(t, State_Elect, c.GetCurrentState())
	assert.Equal(t, "node2", c.currentActiveCoordinator, "currentActiveCoordinator unchanged; node2 is who we sent handover to")
}

func TestCoordinator_WhenClosing_DelegatedTransactions_LowerPriority_ActiveCoordinatorLive_Rejects(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node3").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		HeartbeatIntervalsSinceLastReceive(0).
		InactiveGracePeriod(5). // 0 < 5 → not exceeded
		WithMockTransportWriter().
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	mocks.TransportWriter.EXPECT().SendDelegationRejection(mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
		return msg.DelegationId == "del-3" && msg.ActiveCoordinator == "node1" && msg.RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE
	})).Return(nil)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-3",
	}))
	// GuardNot(IsHigherPriority) AND GuardNot(InactiveGraceExceeded) → action_RejectDelegationRequest fires.
	assert.Equal(t, State_Closing, c.GetCurrentState())
}

func TestCoordinator_WhenClosing_DelegatedTransactions_LowerPriority_ActiveCoordinatorGone_TransitionsToActive(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node3").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		HeartbeatIntervalsSinceLastReceive(6).
		InactiveGracePeriod(5). // 6 > 5 → exceeded
		Build()
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &TransactionsDelegatedEvent{
		FromNode:     "originator-node",
		Originator:   "sender@originator-node",
		DelegationID: "del-4",
	}))
	// GuardNot(IsHigherPriority) AND InactiveGraceExceeded → transitions to Active.
	assert.Equal(t, State_Active, c.GetCurrentState())
	assert.Equal(t, "node3", c.currentActiveCoordinator, "OnTransitionTo Active must set currentActiveCoordinator to self")
	assert.NotEmpty(t, c.signingIdentity.value, "OnTransitionTo Active must call action_NewSigningIdentity")
}

func TestCoordinator_WhenClosing_TransactionsDelegated_BlockHeightToleranceExceeded_RejectsAndStaysClosing(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node2").
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendDelegationRejection(
		mock.Anything, "originator-node", mock.MatchedBy(func(msg *engineProto.DelegationRejection) bool {
			return msg.DelegationId == "del-bh-test" &&
				msg.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
				msg.OriginatorBlockHeight == int64(0) &&
				msg.CoordinatorBlockHeight == int64(100) &&
				msg.BlockHeightTolerance == int64(10)
		}),
	).Return(nil)

	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, newDelegationBlockHeightExceedingEvent("originator-node")))
	assert.Equal(t, State_Closing, c.GetCurrentState())
	// active coordinator must not be updated on a block height rejection
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenClosing_TransactionStateTransition_ToFinal_CleansUpAndStaysClosing(t *testing.T) {
	ctx := t.Context()
	txFinal := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txFinal.EXPECT().GetID().Return(txID).Maybe()
	txFinal.EXPECT().GetCurrentState().Return(transaction.State_Final).Maybe()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		Transactions(txFinal).
		Build()
	require.Equal(t, 1, len(c.transactionsByID))
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Final,
	}))
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Empty(t, c.transactionsByID, "action_CleanUpTransaction must remove the finalised transaction")
}

func TestCoordinator_WhenClosing_EndorsementRequestReceived_UpdatesActiveCoordinatorAndStaysClosing(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node1", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node1", c.currentActiveCoordinator)
}

func TestCoordinator_WhenClosing_EndorsementRequestReceived_BlockHeightToleranceExceeded_RejectsAndStaysClosing(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		CurrentBlockHeight(100).
		BlockHeightTolerance(10).
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100))
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-bh-test", "ik-bh-test",
			engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, int64(0), int64(100), int64(10)),
	).Return(nil)

	event := newBlockHeightExceedingEndorsementEvent("node1", mocks)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	// Rejection path: active coordinator must not be updated.
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node3", c.currentActiveCoordinator)
}

func TestCoordinator_WhenClosing_EndorsementRequestReceived_UpdatesEndorserCandidatesFromSender(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node2").
		EndorserCandidates("node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	event := newEndorsementEventForStateMachineTest(t, "node3", mocks, true)
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	assert.Contains(t, c.endorserCandidates, "node3")
}

func TestCoordinator_WhenClosing_EndorsementRequestReceived_PrivateStateDataPending_RejectsAndStaysClosing(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Closing).
		NodeName("node2").
		CurrentActiveCoordinator("node3").
		CoordinatorPriorityList("node1", "node2", "node3").
		BlockHeightTolerance(1000). // prevent block-height check from firing first
		WithMockTransportWriter().
		Build()

	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, nil)
	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, mock.Anything, matchEndorsementRejectionMsg("tx-1", "ik-1",
			engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING, int64(100), int64(0), int64(10)),
	).Return(nil)

	event := &EndorsementRequestReceivedEvent{
		FromNode:                  "node1",
		TransactionId:             "tx-1",
		IdempotencyKey:            "ik-1",
		Party:                     "party1@node1",
		CoordinatorBlockHeight:    100,
		BlockHeightTolerance:      10, // lowWatermark = 90
		AttestationRequest:        &prototk.AttestationRequest{Name: "att1"},
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, event))

	// Rejection path: active coordinator must not be updated.
	assert.Equal(t, State_Closing, c.GetCurrentState())
	assert.Equal(t, "node3", c.currentActiveCoordinator)
}

func TestCoordinator_PreProcessEvent_OwnHeartbeat_IsFilteredOut(t *testing.T) {
	ctx := t.Context()
	// The coordinator is in Observing state so a non-self heartbeat would normally update the
	// current-active-coordinator field.  A self-originated heartbeat must be silently dropped.
	c, _ := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2").
		Build()

	selfHeartbeat := &common.HeartbeatReceivedEvent{
		FromNode: "node1", // own node name — must be filtered
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}
	require.NoError(t, c.stateMachineEventLoop.ProcessEvent(ctx, selfHeartbeat))

	// The event was filtered before reaching the state machine, so the state must be unchanged
	// and currentActiveCoordinator must not have been updated to "node1".
	assert.Equal(t, State_Observing, c.GetCurrentState(), "own heartbeat must not trigger a state transition")
	assert.Equal(t, "node2", c.currentActiveCoordinator, "own heartbeat must not update currentActiveCoordinator")
}

// newLowerPriorityEndorsementEvent creates a minimal EndorsementRequestReceivedEvent that passes
// the block height check (CoordinatorBlockHeight=0 is within tolerance of the live height=0) but
// fails the higher-priority check, triggering action_RejectEndorsementEndorserIsActiveCoordinator.
func newLowerPriorityEndorsementEvent(t *testing.T, fromNode string, mocks *CoordinatorDependencyMocks, stateComplete bool) *EndorsementRequestReceivedEvent {
	t.Helper()
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, mock.Anything).Return(stateComplete, nil).Maybe()
	return &EndorsementRequestReceivedEvent{
		FromNode:                  fromNode,
		TransactionId:             "tx-lp-test",
		IdempotencyKey:            "ik-lp-test",
		Party:                     "party@" + fromNode,
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
		AttestationRequest:        &prototk.AttestationRequest{Name: "att-lp"},
		CoordinatorBlockHeight:    0, // within tolerance of live height=0, so block height check passes
	}
}

// newBlockHeightExceedingEndorsementEvent creates a minimal EndorsementRequestReceivedEvent
// with CoordinatorBlockHeight=0 (far from the coordinator's currentBlockHeight of 100).
// The block-height difference (100) exceeds the configured tolerance (10), triggering
// action_RejectEndorsementBlockHeight. Because the state machine uses MatchAll, the
// private-state validator is also evaluated; CheckPendingPrivateStateData is mocked to return true
// so that handler does not fire a second rejection.
func newBlockHeightExceedingEndorsementEvent(fromNode string, mocks *CoordinatorDependencyMocks) *EndorsementRequestReceivedEvent {
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, mock.Anything).Return(true, nil).Maybe()
	return &EndorsementRequestReceivedEvent{
		FromNode:                  fromNode,
		TransactionId:             "tx-bh-test",
		IdempotencyKey:            "ik-bh-test",
		Party:                     "party@" + fromNode,
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
		AttestationRequest:        &prototk.AttestationRequest{Name: "att-bh"},
		CoordinatorBlockHeight:    0, // far from the coordinator's live height of 100
	}
}

// newDelegationBlockHeightExceedingEvent creates a TransactionsDelegatedEvent where
// OriginatorsBlockHeight=0. When the coordinator's stored block height is 100 and
// blockHeightTolerance is 10, the difference (100) exceeds the tolerance, triggering
// action_RejectDelegationRequestBlockHeight instead of action_ProcessDelegatedTransactions.
func newDelegationBlockHeightExceedingEvent(fromNode string) *TransactionsDelegatedEvent {
	return &TransactionsDelegatedEvent{
		FromNode:               fromNode,
		Originator:             "sender@" + fromNode,
		DelegationID:           "del-bh-test",
		OriginatorsBlockHeight: 0, // far from coordinator's stored height of 100
	}
}

// newEndorsementEventForStateMachineTest creates an EndorsementRequestReceivedEvent and
// wires the coordinator's mocks so the background endorsement goroutine exits immediately
// (at party key resolution) without touching SendEndorsementResponse.  All mock expectations
// use .Maybe() so they tolerate asynchronous execution after the test's synchronous assertions.
// stateComplete controls whether CheckPendingPrivateStateData returns true (state ready) or false (incomplete).
func newEndorsementEventForStateMachineTest(t *testing.T, fromNode string, mocks *CoordinatorDependencyMocks, stateComplete bool) *EndorsementRequestReceivedEvent {
	t.Helper()

	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, mock.Anything).Return(stateComplete, nil).Maybe()

	// Fail at party key resolution so the goroutine exits as early as possible.
	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("test: goroutine exit early")).Maybe()
	mocks.AllComponents.On("KeyManager").Return(mockKeyManager).Maybe()

	// action_RefreshBlockHeight calls GetBlockHeight for every endorsement and delegation event.
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0))

	return &EndorsementRequestReceivedEvent{
		FromNode:                  fromNode,
		TransactionId:             "tx-test",
		IdempotencyKey:            "ik-test",
		Party:                     "party@" + fromNode,
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
		AttestationRequest:        &prototk.AttestationRequest{},
	}
}
