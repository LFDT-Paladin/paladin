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
package transaction

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_State_String_AllStates(t *testing.T) {
	tests := []struct {
		state  State
		expect string
	}{
		{State_Initial, "State_Initial"},
		{State_Pooled, "State_Pooled"},
		{State_PreAssembly_Blocked, "State_PreAssembly_Blocked"},
		{State_Assembling, "State_Assembling"},
		{State_Reverted, "State_Reverted"},
		{State_Endorsement_Gathering, "State_Endorsement_Gathering"},
		{State_Blocked, "State_Blocked"},
		{State_Confirming_Dispatchable, "State_Confirming_Dispatchable"},
		{State_Ready_For_Dispatch, "State_Ready_For_Dispatch"},
		{State_Dispatched, "State_Dispatched"},
		{State_Confirmed, "State_Confirmed"},
		{State_Final, "State_Final"},
		{State_Evicted, "State_Evicted"},
	}
	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.state.String())
		})
	}
}

func Test_State_String_Unknown(t *testing.T) {
	// State value beyond defined constants
	s := State(99)
	assert.Contains(t, s.String(), "Unknown")
	assert.Contains(t, s.String(), "99")
}

func Test_action_IncrementHeartbeatIntervalsSinceStateChange_IncrementsCounter(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		HeartbeatIntervalsSinceStateChange(2).
		Build()

	err := action_IncrementHeartbeatIntervalsSinceStateChange(ctx, txn, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, txn.heartbeatIntervalsSinceStateChange)
}

func Test_StateConfirmed_HeartbeatResetsLocksOnlyAtRetentionThreshold(t *testing.T) {
	ctx := context.Background()
	mockGrapher := grapher.NewMockGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(mockGrapher).
		ConfirmedLockRetentionGracePeriod(2).
		FinalizingGracePeriod(10).
		Build()
	mockGrapher.EXPECT().Forget(txn.pt.ID)()

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())
	assert.Equal(t, 1, txn.heartbeatIntervalsSinceStateChange)
	assert.False(t, txn.confirmedLocksReleased)
	mockGrapher.AssertNotCalled(t, "Forget", ctx, txn.pt.ID)

	err = txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())
	assert.Equal(t, 2, txn.heartbeatIntervalsSinceStateChange)
	assert.True(t, txn.confirmedLocksReleased)

	err = txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())
}

func Test_StateConfirmed_TransitionsToFinalBasedOnFinalizingGracePeriod(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		ConfirmedLockRetentionGracePeriod(100).
		FinalizingGracePeriod(2).
		Build()

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())

	err = txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Final, txn.stateMachine.GetCurrentState())
}

func Test_ChainedDependencyFailed_AllStates_TransitionToReverted(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()

	states := []State{
		State_PreAssembly_Blocked,
		State_Pooled,
		State_Assembling,
		State_Endorsement_Gathering,
		State_Blocked,
		State_Confirming_Dispatchable,
		State_Ready_For_Dispatch,
		State_Dispatched,
	}

	for _, fromState := range states {
		t.Run(fromState.String(), func(t *testing.T) {
			txn, mocks := NewTransactionBuilderForTesting(t, fromState).Build()

			mocks.SyncPoints.On("QueueTransactionFinalize",
				mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			).Return()
			mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

			err := txn.HandleEvent(ctx, &ChainedDependencyFailedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
				FailedTxID:           depID,
			})
			require.NoError(t, err)
			assert.Equal(t, State_Reverted, txn.GetCurrentState())
		})
	}
}

func Test_DependencyConfirmedReverted_ChainedDependency_AllStates(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		fromState State
		toState   State
	}{
		{State_Pooled, State_PreAssembly_Blocked},
		{State_Assembling, State_PreAssembly_Blocked},
		{State_Endorsement_Gathering, State_PreAssembly_Blocked},
		{State_Blocked, State_PreAssembly_Blocked},
		{State_Confirming_Dispatchable, State_PreAssembly_Blocked},
		{State_Ready_For_Dispatch, State_PreAssembly_Blocked},
		{State_Dispatched, State_Dispatched},
	}

	for _, tt := range tests {
		t.Run(tt.fromState.String(), func(t *testing.T) {
			depID := uuid.New()
			txn, mocks := NewTransactionBuilderForTesting(t, tt.fromState).
				Dependencies(&TransactionDependencies{
					Chained: ChainedDependencies{
						DependsOn:   []uuid.UUID{depID},
						Unassembled: map[uuid.UUID]struct{}{},
					},
				}).
				Build()

			mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

			err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
				SourceTransactionID:  depID,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.toState, txn.GetCurrentState())
			assert.Contains(t, txn.dependencies.Chained.Unassembled, depID)
		})
	}
}

func Test_DependencyReset_ChainedDependency_AllStates(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		fromState State
		toState   State
	}{
		{State_Pooled, State_PreAssembly_Blocked},
		{State_Assembling, State_PreAssembly_Blocked},
		{State_Endorsement_Gathering, State_PreAssembly_Blocked},
		{State_Blocked, State_PreAssembly_Blocked},
		{State_Confirming_Dispatchable, State_PreAssembly_Blocked},
		{State_Ready_For_Dispatch, State_PreAssembly_Blocked},
		{State_Dispatched, State_Dispatched},
	}

	for _, tt := range tests {
		t.Run(tt.fromState.String(), func(t *testing.T) {
			depID := uuid.New()
			txn, mocks := NewTransactionBuilderForTesting(t, tt.fromState).
				Dependencies(&TransactionDependencies{
					Chained: ChainedDependencies{
						DependsOn:   []uuid.UUID{depID},
						Unassembled: map[uuid.UUID]struct{}{},
					},
				}).
				Build()

			mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

			err := txn.HandleEvent(ctx, &DependencyResetEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
				SourceTransactionID:  depID,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.toState, txn.GetCurrentState())
			assert.Contains(t, txn.dependencies.Chained.Unassembled, depID)
		})
	}
}

func Test_DependencyReset_PostAssembleDependency_AllStates(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		fromState State
		toState   State
	}{
		{State_Pooled, State_Pooled},
		{State_Assembling, State_PreAssembly_Blocked},
		{State_Endorsement_Gathering, State_Pooled},
		{State_Blocked, State_Pooled},
		{State_Confirming_Dispatchable, State_Pooled},
		{State_Ready_For_Dispatch, State_Pooled},
		{State_Dispatched, State_Dispatched},
	}

	for _, tt := range tests {
		t.Run(tt.fromState.String(), func(t *testing.T) {
			sourceID := uuid.New()
			txn, mocks := NewTransactionBuilderForTesting(t, tt.fromState).Build()

			if tt.fromState != State_Pooled {
				mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()
			}

			err := txn.HandleEvent(ctx, &DependencyResetEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
				SourceTransactionID:  sourceID,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.toState, txn.GetCurrentState())
		})
	}
}

func Test_DependencyConfirmedReverted_PostAssembleDependency_AllStates(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		fromState State
		toState   State
	}{
		{State_Pooled, State_Pooled},
		{State_Assembling, State_PreAssembly_Blocked},
		{State_Endorsement_Gathering, State_Pooled},
		{State_Blocked, State_Pooled},
		{State_Confirming_Dispatchable, State_Pooled},
		{State_Ready_For_Dispatch, State_Pooled},
		{State_Dispatched, State_Dispatched},
	}

	for _, tt := range tests {
		t.Run(tt.fromState.String(), func(t *testing.T) {
			sourceID := uuid.New()
			txn, mocks := NewTransactionBuilderForTesting(t, tt.fromState).Build()

			if tt.fromState != State_Pooled {
				mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()
			}

			err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
				SourceTransactionID:  sourceID,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.toState, txn.GetCurrentState())
		})
	}
}

func Test_ChainedDependencyEvicted_AllStates_TransitionToEvicted(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()

	states := []State{
		State_PreAssembly_Blocked,
		State_Pooled,
		State_Assembling,
	}

	for _, fromState := range states {
		t.Run(fromState.String(), func(t *testing.T) {
			txn, _ := NewTransactionBuilderForTesting(t, fromState).Build()

			err := txn.HandleEvent(ctx, &ChainedDependencyEvictedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
				EvictedTxID:          depID,
			})
			require.NoError(t, err)
			assert.Equal(t, State_Evicted, txn.GetCurrentState())
		})
	}
}
