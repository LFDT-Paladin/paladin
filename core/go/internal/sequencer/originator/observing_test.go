/*
 * Copyright © 2026 Kaleido, Inc.
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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatortransactionmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ── validator_IsFromCurrentCoordinator ───────────────────────────────────────

func Test_validator_IsFromCurrentCoordinator_TrueWhenSenderIsCurrentCoordinator(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("nodeB").
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode:            "nodeB",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{CoordinatorState: common.CoordinatorState_Active},
	}
	ok, err := validator_IsFromCurrentCoordinator(ctx, o, event)
	require.NoError(t, err)
	assert.True(t, ok)
}

func Test_validator_IsFromCurrentCoordinator_TrueRegardlessOfLiveness(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("nodeB").
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode:            "nodeB",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{CoordinatorState: common.CoordinatorState_Closing},
	}
	ok, err := validator_IsFromCurrentCoordinator(ctx, o, event)
	require.NoError(t, err)
	assert.True(t, ok, "identity check does not require liveness")
}

func Test_validator_IsFromCurrentCoordinator_FalseWhenSenderIsOtherNode(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("nodeB").
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode:            "nodeC",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{CoordinatorState: common.CoordinatorState_Active},
	}
	ok, err := validator_IsFromCurrentCoordinator(ctx, o, event)
	require.NoError(t, err)
	assert.False(t, ok)
}

// ── validator_HasDroppedTransactions ─────────────────────────────────────────

func Test_validator_HasDroppedTransactions_TrueWhenInFlightTransactionAbsentFromSnapshot(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Delegated)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Transactions(mockTxn).
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode:            "coordinator@node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{},
	}
	ok, err := validator_HasDroppedTransactions(ctx, o, event)
	require.NoError(t, err)
	assert.True(t, ok, "transaction absent from snapshot must register as dropped")
}

func Test_validator_HasDroppedTransactions_FalseWhenAllTransactionsPresentInSnapshot(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Delegated)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Transactions(mockTxn).
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode: "coordinator@node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			DispatchedTransactions: []*common.SnapshotDispatchedTransaction{
				{SnapshotPooledTransaction: common.SnapshotPooledTransaction{ID: txID}},
			},
		},
	}
	ok, err := validator_HasDroppedTransactions(ctx, o, event)
	require.NoError(t, err)
	assert.False(t, ok)
}

func Test_validator_HasDroppedTransactions_FalseWhenTransactionPresentInRevertedList(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Delegated)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Transactions(mockTxn).
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode: "coordinator@node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			RevertedTransactions: []*common.SnapshotRevertedTransaction{
				{SnapshotPooledTransaction: common.SnapshotPooledTransaction{ID: txID}},
			},
		},
	}
	ok, err := validator_HasDroppedTransactions(ctx, o, event)
	require.NoError(t, err)
	assert.False(t, ok, "transaction in reverted list must not be treated as dropped")
}

func Test_validator_HasDroppedTransactions_FalseWhenNoInFlightTransactions(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode:            "coordinator@node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{},
	}
	ok, err := validator_HasDroppedTransactions(ctx, o, event)
	require.NoError(t, err)
	assert.False(t, ok)
}

// ── guard_InactiveGracePeriodExceeded ─────────────────────────────────────────

func Test_guard_InactiveGracePeriodExceeded_WhileObserving_TrueWhenCounterExceedsThreshold(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		HeartbeatIntervalsSinceLastReceive(11).
		InactiveGracePeriod(10).
		Build()
	assert.True(t, guard_InactiveGracePeriodExceeded(ctx, o))
}
func Test_guard_InactiveGracePeriodExceeded_WhileObserving_FalseWhenCounterBelowThreshold(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		HeartbeatIntervalsSinceLastReceive(5).
		InactiveGracePeriod(10).
		Build()
	assert.False(t, guard_InactiveGracePeriodExceeded(ctx, o))
}

// ── State_Observing integration tests ─────────────────────────────────────────

func Test_ProcessEvent_HeartbeatIntervalWhileObserving_IncrementsHeartbeatIntervalsSinceLastReceive(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		HeartbeatIntervalsSinceLastReceive(4).
		InactiveGracePeriod(100).
		Build()
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{}))
	assert.Equal(t, 5, o.heartbeatIntervalsSinceLastReceive)
	assert.Equal(t, State_Observing, o.GetCurrentState())
}

func Test_ProcessEvent_HeartbeatReceivedWhileObserving_FromCurrentActiveCoordinator_ActiveState_ResetsLivenessCounter(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("nodeB").
		NodeName("self@selfNode").
		HeartbeatIntervalsSinceLastReceive(5).
		InactiveGracePeriod(100).
		Build()
	heartbeatEvent := &common.HeartbeatReceivedEvent{}
	heartbeatEvent.FromNode = "nodeB"
	heartbeatEvent.ContractAddress = o.contractAddress
	heartbeatEvent.CoordinatorSnapshot = &common.CoordinatorSnapshot{CoordinatorState: common.CoordinatorState_Active}
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, heartbeatEvent))
	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness counter must be reset on Active heartbeat from current coordinator")
}

func Test_ProcessEvent_HeartbeatReceivedWhileObserving_FromAnotherActiveNode_UpdatesCoordinatorAndResetsTimer(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("nodeB").
		NodeName("self@selfNode").
		HeartbeatIntervalsSinceLastReceive(5).
		InactiveGracePeriod(100).
		Build()
	heartbeatEvent := &common.HeartbeatReceivedEvent{}
	heartbeatEvent.FromNode = "nodeC"
	heartbeatEvent.ContractAddress = o.contractAddress
	heartbeatEvent.CoordinatorSnapshot = &common.CoordinatorSnapshot{CoordinatorState: common.CoordinatorState_Active}
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, heartbeatEvent))
	assert.Equal(t, State_Observing, o.GetCurrentState())
	assert.Equal(t, "nodeC", o.currentActiveCoordinator, "Active heartbeat from a new node must update currentActiveCoordinator")
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness counter must be reset on Active heartbeat")
}

// ── action_ProcessConfirmedTransactions ───────────────────────────────────────

func Test_action_ProcessConfirmedTransactions_ConfirmedSuccess(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()

	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("HandleEvent", mock.Anything, mock.MatchedBy(func(e transaction.Event) bool {
		_, ok := e.(*transaction.ConfirmedSuccessEvent)
		return ok
	})).Return(nil)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Transactions(mockTxn).
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "any@node",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{
				{
					SnapshotDispatchedTransaction: common.SnapshotDispatchedTransaction{
						SnapshotPooledTransaction: common.SnapshotPooledTransaction{
							ID: txID,
						},
					},
				},
			},
		},
	}

	err := action_ProcessConfirmedTransactions(ctx, o, event)
	require.NoError(t, err)
	mockTxn.AssertExpectations(t)
}

func Test_action_ProcessConfirmedTransactions_NotInMemory_Skipped(t *testing.T) {
	ctx := context.Background()

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "any@node",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{
				{
					SnapshotDispatchedTransaction: common.SnapshotDispatchedTransaction{
						SnapshotPooledTransaction: common.SnapshotPooledTransaction{
							ID: uuid.New(),
						},
					},
				},
			},
		},
	}

	err := action_ProcessConfirmedTransactions(ctx, o, event)
	require.NoError(t, err)
}

func Test_action_ProcessConfirmedTransactions_HandleEventError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()

	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(fmt.Errorf("handle event error"))

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Transactions(mockTxn).
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "any@node",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{
				{
					SnapshotDispatchedTransaction: common.SnapshotDispatchedTransaction{
						SnapshotPooledTransaction: common.SnapshotPooledTransaction{
							ID: txID,
						},
					},
				},
			},
		},
	}

	err := action_ProcessConfirmedTransactions(ctx, o, event)
	require.Error(t, err)
}

// ── action_ProcessRevertedTransactions ───────────────────────────────────────

func Test_action_ProcessRevertedTransactions_FiresConfirmedRevertedEvent(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	revertReason := pldtypes.HexBytes{0x01, 0x02}

	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("HandleEvent", mock.Anything, mock.MatchedBy(func(e transaction.Event) bool {
		rev, ok := e.(*transaction.ConfirmedRevertedEvent)
		return ok && !rev.WillRetry && rev.RevertReason.Equals(revertReason)
	})).Return(nil)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Transactions(mockTxn).
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "coordinator@node1",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			RevertedTransactions: []*common.SnapshotRevertedTransaction{
				{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{
						ID: txID,
					},
					RevertReason: revertReason,
				},
			},
		},
	}

	err := action_ProcessRevertedTransactions(ctx, o, event)
	require.NoError(t, err)
	mockTxn.AssertExpectations(t)
}

func Test_action_ProcessRevertedTransactions_NilRevertReason_StillFiresEvent(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()

	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("HandleEvent", mock.Anything, mock.MatchedBy(func(e transaction.Event) bool {
		rev, ok := e.(*transaction.ConfirmedRevertedEvent)
		return ok && !rev.WillRetry && rev.RevertReason == nil
	})).Return(nil)

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Transactions(mockTxn).
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "coordinator@node1",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			RevertedTransactions: []*common.SnapshotRevertedTransaction{
				{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{
						ID: txID,
					},
				},
			},
		},
	}

	err := action_ProcessRevertedTransactions(ctx, o, event)
	require.NoError(t, err)
	mockTxn.AssertExpectations(t)
}

func Test_action_ProcessRevertedTransactions_NotInMemory_Skipped(t *testing.T) {
	ctx := context.Background()

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "coordinator@node1",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			RevertedTransactions: []*common.SnapshotRevertedTransaction{
				{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{
						ID: uuid.New(),
					},
				},
			},
		},
	}

	err := action_ProcessRevertedTransactions(ctx, o, event)
	require.NoError(t, err)
}

func Test_action_ProcessRevertedTransactions_HandleEventError_ReturnsError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()

	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("HandleEvent", mock.Anything, mock.Anything).Return(fmt.Errorf("revert handle error"))

	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName("member1@node1").
		CurrentActiveCoordinator("coordinator@node1").
		Transactions(mockTxn).
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "coordinator@node1",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			RevertedTransactions: []*common.SnapshotRevertedTransaction{
				{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{
						ID: txID,
					},
					RevertReason: pldtypes.HexBytes{0xde, 0xad},
				},
			},
		},
	}

	err := action_ProcessRevertedTransactions(ctx, o, event)
	require.Error(t, err)
}

// ── action_ProcessCurrentCoordinatorHeartbeat ─────────────────────────────────

func Test_action_ProcessCurrentCoordinatorHeartbeat_ResetsLivenessTimer(t *testing.T) {
	ctx := context.Background()
	coordinatorLocator := "coordinator@coordinatorNode"
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator(coordinatorLocator).
		Build()
	o.heartbeatIntervalsSinceLastReceive = 5

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        coordinatorLocator,
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			BlockHeight: 1000,
		},
	}
	err := action_ProcessCurrentCoordinatorHeartbeat(ctx, o, event)
	require.NoError(t, err)

	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness counter must be reset")
	assert.Equal(t, coordinatorLocator, o.currentActiveCoordinator, "coordinator must be unchanged")
}

func Test_action_ProcessCurrentCoordinatorHeartbeat_DispatchedTransactionNotFoundContinues(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName(originatorLocator).
		CurrentActiveCoordinator(coordinatorLocator).
		Build()

	unknownTxID := uuid.New()
	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        coordinatorLocator,
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			DispatchedTransactions: []*common.SnapshotDispatchedTransaction{
				{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{
						ID: unknownTxID,
					},
				},
			},
		},
	}
	err := action_ProcessCurrentCoordinatorHeartbeat(ctx, o, event)
	assert.NoError(t, err)
}

func Test_action_ProcessCurrentCoordinatorHeartbeat_DispatchedTransactionWithHashUpdatesSubmitted(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName(originatorLocator).
		CurrentActiveCoordinator(coordinatorLocator)
	o, _ := builder.Build()

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originatorLocator).
		NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	require.NoError(t, o.addToTransactions(ctx, txn, nil, o.newOriginatorTransaction))

	signerAddress := pldtypes.RandAddress()
	submissionHash := pldtypes.RandBytes32()
	nonce := uint64(42)
	contractAddress := builder.GetContractAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        coordinatorLocator,
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			DispatchedTransactions: []*common.SnapshotDispatchedTransaction{
				{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{
						ID: txn.ID,
					},
					Signer:               *signerAddress,
					LatestSubmissionHash: &submissionHash,
					Nonce:                &nonce,
				},
			},
		},
	}
	err := action_ProcessCurrentCoordinatorHeartbeat(ctx, o, event)
	assert.NoError(t, err)
}

func Test_action_ProcessCurrentCoordinatorHeartbeat_DispatchedTransactionWithNonceOnly(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName(originatorLocator).
		CurrentActiveCoordinator(coordinatorLocator)
	o, _ := builder.Build()

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originatorLocator).
		NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	require.NoError(t, o.addToTransactions(ctx, txn, nil, o.newOriginatorTransaction))

	nonce := uint64(42)
	contractAddress := builder.GetContractAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        coordinatorLocator,
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			DispatchedTransactions: []*common.SnapshotDispatchedTransaction{
				{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{
						ID: txn.ID,
					},
					Nonce: &nonce,
				},
			},
		},
	}
	err := action_ProcessCurrentCoordinatorHeartbeat(ctx, o, event)
	assert.NoError(t, err)
}

func Test_action_ProcessCurrentCoordinatorHeartbeat_SubmittedHandleEventError(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	txnID := uuid.New()
	innerErr := fmt.Errorf("simulated submitted handling failure")
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.EXPECT().GetID().Return(txnID)
	mockTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.SubmittedEvent")).Return(innerErr)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName(originatorLocator).
		CurrentActiveCoordinator(coordinatorLocator).
		Transactions(mockTxn).
		Build()

	signerAddress := pldtypes.RandAddress()
	submissionHash := pldtypes.RandBytes32()
	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        coordinatorLocator,
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			DispatchedTransactions: []*common.SnapshotDispatchedTransaction{
				{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{
						ID: txnID,
					},
					Signer:               *signerAddress,
					LatestSubmissionHash: &submissionHash,
				},
			},
		},
	}
	err := action_ProcessCurrentCoordinatorHeartbeat(ctx, o, event)
	require.Error(t, err)
	assert.ErrorContains(t, err, "error handling transaction submitted event")
	assert.Contains(t, err.Error(), txnID.String())
	assert.Contains(t, err.Error(), innerErr.Error())
}

func Test_action_ProcessCurrentCoordinatorHeartbeat_NonceAssignedHandleEventError(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	txnID := uuid.New()
	innerErr := fmt.Errorf("simulated nonce handling failure")
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.EXPECT().GetID().Return(txnID)
	mockTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NonceAssignedEvent")).Return(innerErr)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		NodeName(originatorLocator).
		CurrentActiveCoordinator(coordinatorLocator).
		Transactions(mockTxn).
		Build()

	nonce := uint64(99)
	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        coordinatorLocator,
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			DispatchedTransactions: []*common.SnapshotDispatchedTransaction{
				{
					SnapshotPooledTransaction: common.SnapshotPooledTransaction{
						ID: txnID,
					},
					Nonce: &nonce,
				},
			},
		},
	}
	err := action_ProcessCurrentCoordinatorHeartbeat(ctx, o, event)
	require.Error(t, err)
	assert.ErrorContains(t, err, "error handling nonce assigned event")
	assert.Contains(t, err.Error(), txnID.String())
	assert.Contains(t, err.Error(), innerErr.Error())
}

// ── validator_IsSenderHigherPriorityThanCurrentCoordinator ───────────────────

func Test_validator_IsSenderHigherPriorityThanCurrentCoordinator_TrueWhenHigherPriority(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode:            "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{},
	}
	ok, err := validator_IsSenderHigherPriorityThanCurrentCoordinator(ctx, o, event)
	require.NoError(t, err)
	assert.True(t, ok, "node1 (idx 0) is higher priority than node2 (idx 1)")
}

func Test_validator_IsSenderHigherPriorityThanCurrentCoordinator_FalseWhenLowerPriority(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()
	event := &common.HeartbeatReceivedEvent{
		FromNode:            "node2",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{},
	}
	ok, err := validator_IsSenderHigherPriorityThanCurrentCoordinator(ctx, o, event)
	require.NoError(t, err)
	assert.False(t, ok, "node2 (idx 1) is not higher priority than node1 (idx 0)")
}

// ── action_SwitchActiveCoordinator ────────────────────────────────────────────

func Test_action_SwitchActiveCoordinator_UpdatesCoordinatorAndResetsLivenessTimer(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node2").
		HeartbeatIntervalsSinceLastReceive(7).
		Build()

	event := &common.HeartbeatReceivedEvent{
		FromNode:            "node1",
		CoordinatorSnapshot: &common.CoordinatorSnapshot{},
	}

	err := action_SwitchActiveCoordinator(ctx, o, event)
	require.NoError(t, err)

	assert.Equal(t, "node1", o.currentActiveCoordinator)
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness timer must be reset when switching coordinator")
}

// ── State_Sending integration: coordinator switching ─────────────────────────

// A live heartbeat from a higher-priority node redirects the active coordinator (step 2) and then
// immediately processes that same heartbeat as the new coordinator's heartbeat (step 4), because
// step 2 updates currentActiveCoordinator before step 4's validator runs.
func Test_ProcessEvent_HeartbeatReceived_HigherPriorityNode_RedirectsAndProcessesHeartbeat(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		HeartbeatIntervalsSinceLastReceive(5).
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "node1",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, "node1", o.currentActiveCoordinator, "must redirect to higher-priority node")
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness timer must be reset")
	assert.Equal(t, State_Sending, o.GetCurrentState())
}

// A live heartbeat from any node redirects when the current coordinator has been silent for at
// least the inactive grace period (step 3), and then fires step 4 for the same reason as above.
func Test_ProcessEvent_HeartbeatReceived_InactiveFallback_RedirectsAndProcessesHeartbeat(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		HeartbeatIntervalsSinceLastReceive(11).
		InactiveGracePeriod(10).
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "node2",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, "node2", o.currentActiveCoordinator, "must switch to live node when current is inactive")
	assert.Equal(t, 0, o.heartbeatIntervalsSinceLastReceive, "liveness timer must be reset")
	assert.Equal(t, State_Sending, o.GetCurrentState())
}

// A live heartbeat from a lower-priority node when the current coordinator is still within the
// grace period must be a no-op for coordinator selection but still process confirmed transactions.
func Test_ProcessEvent_HeartbeatReceived_LowerPriorityWithinGracePeriod_NoRedirect(t *testing.T) {
	ctx := context.Background()
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		CurrentActiveCoordinator("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		HeartbeatIntervalsSinceLastReceive(3).
		InactiveGracePeriod(10).
		Build()

	contractAddress := *pldtypes.RandAddress()
	event := &common.HeartbeatReceivedEvent{
		FromNode:        "node2",
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}

	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.Equal(t, "node1", o.currentActiveCoordinator, "must not redirect while current is within grace period")
	assert.Equal(t, 3, o.heartbeatIntervalsSinceLastReceive, "liveness timer must not be reset for other node")
	assert.Equal(t, State_Sending, o.GetCurrentState())
}
