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
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/dependencytracker"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/core/mocks/graphermocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_IncrementHeartbeatIntervalsSinceStateChange_IncrementsCounter(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		HeartbeatIntervalsSinceStateChange(2).
		Build()

	err := action_IncrementHeartbeatIntervalsSinceStateChange(ctx, txn, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, txn.heartbeatIntervalsSinceStateChange)
}

func Test_StateConfirmed_HeartbeatIncreasesIntervalCounter(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		FinalizingGracePeriod(10).
		Build()

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())
	assert.Equal(t, 1, txn.heartbeatIntervalsSinceStateChange)

	err = txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())
	assert.Equal(t, 2, txn.heartbeatIntervalsSinceStateChange)

	err = txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())
}

func Test_StateConfirmed_TransitionsToFinalBasedOnFinalizingGracePeriod(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		FinalizingGracePeriod(2).
		Build()

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())

	err = txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())

	err = txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Final, txn.stateMachine.GetCurrentState())
}

func Test_ChainedDependencyFailed_AllStates_TransitionToReverted(t *testing.T) {
	ctx := t.Context()
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
	ctx := t.Context()

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
			depTracker := dependencytracker.NewDependencyTracker()
			txn, _ := NewTransactionBuilderForTesting(t, tt.fromState).
				DependencyTracker(depTracker).
				Build()
			depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depID)

			err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
				SourceTransactionID:  depID,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.toState, txn.GetCurrentState())
			assert.Contains(t, txn.dependencyTracker.GetChainedDeps().GetUnassembledDependencies(ctx, txn.pt.ID), depID)
		})
	}
}

func Test_DependencyReset_ChainedDependency_AllStates(t *testing.T) {
	ctx := t.Context()

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
			depTracker := dependencytracker.NewDependencyTracker()
			txn, _ := NewTransactionBuilderForTesting(t, tt.fromState).
				DependencyTracker(depTracker).
				Build()
			depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depID)

			err := txn.HandleEvent(ctx, &DependencyResetEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
				SourceTransactionID:  depID,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.toState, txn.GetCurrentState())
			assert.Contains(t, txn.dependencyTracker.GetChainedDeps().GetUnassembledDependencies(ctx, txn.pt.ID), depID)
		})
	}
}

func Test_DependencyReset_PostAssembleDependency_AllStates(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		fromState State
		toState   State
	}{
		{State_Endorsement_Gathering, State_Pooled},
		{State_Blocked, State_Pooled},
		{State_Confirming_Dispatchable, State_Pooled},
		{State_Ready_For_Dispatch, State_Pooled},
		{State_Dispatched, State_Dispatched},
	}

	for _, tt := range tests {
		t.Run(tt.fromState.String(), func(t *testing.T) {
			sourceID := uuid.New()
			txn, _ := NewTransactionBuilderForTesting(t, tt.fromState).Build()

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
	ctx := t.Context()

	tests := []struct {
		fromState State
		toState   State
	}{
		{State_Endorsement_Gathering, State_Pooled},
		{State_Blocked, State_Pooled},
		{State_Confirming_Dispatchable, State_Pooled},
		{State_Ready_For_Dispatch, State_Pooled},
		{State_Dispatched, State_Dispatched},
	}

	for _, tt := range tests {
		t.Run(tt.fromState.String(), func(t *testing.T) {
			sourceID := uuid.New()
			txn, _ := NewTransactionBuilderForTesting(t, tt.fromState).Build()

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
	ctx := t.Context()
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

func TestCoordinatorTransaction_Initial_ToReverted_OnDelegated_IfHasRevertedChainedDependency(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()
	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).TransactionID(depID).Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		ChainedDependencies(depID).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{depID: depTx}).
		Build()
	mocks.SyncPoints.On("QueueTransactionFinalize",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Reverted, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Initial_ToEvicted_OnDelegated_IfHasEvictedChainedDependency(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()
	depTx, _ := NewTransactionBuilderForTesting(t, State_Evicted).TransactionID(depID).Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		ChainedDependencies(depID).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{depID: depTx}).
		Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Evicted, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Initial_ToPreAssemblyBlocked_OnDelegated_IfHasUnassembledDependency(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()
	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).TransactionID(depID).Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		ChainedDependencies(depID).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{depID: depTx}).
		Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Initial_ToPooled_OnReceived_IfNoInflightDependencies(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_PreAssemblyBlocked_ToPooled_OnDependencySelectedForAssemble(t *testing.T) {
	// Chained dep in State_Pooled → unassembled. DependencySelectedForAssemble marks it
	// assembled, clearing the block.
	ctx := context.Background()
	depID := uuid.New()
	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).TransactionID(depID).Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		ChainedDependencies(depID).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{depID: depTx}).
		Build()

	err := txn.HandleEvent(ctx, &DependencySelectedForAssemblyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
		SourceTransactionID:  depID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func TestCoordinatorTransaction_PreAssemblyBlocked_ToPooled_OnPreAssembleDependencyTerminated(t *testing.T) {
	// Pre-assemble (FIFO ordering) predecessor terminates → sever the FIFO link and
	// move to Pooled if there are no remaining unassembled dependencies.
	ctx := context.Background()
	depTracker := dependencytracker.NewDependencyTracker()
	predecessorID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		DependencyTracker(depTracker).
		Build()
	depTracker.GetPreassemblyDeps().AddPrerequisite(ctx, txn.pt.ID, predecessorID)

	err := txn.HandleEvent(ctx, &PreAssembleDependencyTerminatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}
func TestCoordinatorTransaction_Pooled_ToAssembling_OnSelected(t *testing.T) {
	ctx := context.Background()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Pooled).
		WithCurrentBlockHeight(100).
		Build()

	err := txn.HandleEvent(ctx, &SelectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)

	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, true, mocks.SentMessageRecorder.HasSentAssembleRequest())
}

func TestCoordinatorTransaction_Assembling_ToEndorsing_OnAssembleResponse(t *testing.T) {
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	mockGrapher.EXPECT().AddMinter(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockGrapher.EXPECT().LockMintsOnCreate(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockGrapher.EXPECT().LockMintsOnReadAndSpend(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		NumberOfOutputStates(1).
		AddPendingAssembleRequest()
	txn, mocks := txnBuilder.Build()

	successEvent := txnBuilder.BuildAssembleSuccessEvent()
	outputState := successEvent.PostAssembly.OutputStates[0]
	mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Run(func(ctx context.Context, pt *components.PrivateTransaction) {
		assert.Equal(t, outputState.ID, pt.PostAssembly.OutputStates[0].ID)
	}).Return(nil)

	err := txn.HandleEvent(ctx, successEvent)
	require.NoError(t, err)
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 3, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())
}

func TestCoordinatorTransaction_Assembling_NoTransition_OnAssembleResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := txnBuilder.Build()

	err := txn.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    uuid.New(), //generate a new random request ID so that it won't match the pending request
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToConfirmingDispatch_OnAssembleSuccess_IfAttestationPlanFulfilled(t *testing.T) {
	// 0 required endorsers → attestation plan is immediately fulfilled → skip Endorsement_Gathering.
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		NumberOfRequiredEndorsers(0).
		AddPendingAssembleRequest()
	txn, mocks := txnBuilder.Build()

	successEvent := txnBuilder.BuildAssembleSuccessEvent()
	mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, successEvent)
	require.NoError(t, err)
	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Assembling_ToBlocked_OnAssembleSuccess_IfAttestationPlanFulfilledButHasDependenciesNotReady(t *testing.T) {
	// Dep in Endorsement_Gathering is not ready for dispatch → transaction goes to Blocked instead of Confirming_Dispatchable.
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(mockGrapher).
		Build()

	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		NumberOfRequiredEndorsers(0).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{txn1.pt.ID: txn1}).
		AddPendingAssembleRequest()
	txn2, mocks := txnBuilder.Build()

	successEvent := txnBuilder.BuildAssembleSuccessEvent()
	mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)
	mockGrapher.EXPECT().AddMinter(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockGrapher.EXPECT().LockMintsOnCreate(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	mockGrapher.EXPECT().LockMintsOnReadAndSpend(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	mockGrapher.EXPECT().GetDependencies(mock.Anything, txn2.pt.ID).Return([]uuid.UUID{txn1.GetID()})

	err := txn2.HandleEvent(ctx, successEvent)
	require.NoError(t, err)
	assert.Equal(t, State_Blocked, txn2.GetCurrentState())
}

func TestCoordinatorTransaction_Assembling_NoTransition_OnRequestTimeout(t *testing.T) {
	ctx := context.Background()
	hasNudged := false
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		RequestTimeout(1).
		AddPendingAssembleRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error {
			hasNudged = true
			return nil
		}).
		Build()

	err := txn.HandleEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.True(t, hasNudged)
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToPooled_OnStateTimeout_IfStateTimeoutExpired(t *testing.T) {
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		StateTimeout(1).
		Build()
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())

	err := txn.HandleEvent(ctx, &StateTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToPooled_OnAssembleCancelled(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		AddPendingAssembleRequest().
		Build()

	err := txn.HandleEvent(ctx, &AssembleCancelledEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Assembling_ToEvicted_OnAssembleRejected_NotCurrentDelegate(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	err := txn.HandleEvent(ctx, &AssembleRequestRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
		RejectionReason:      engineProto.RejectionReason_NOT_CURRENT_DELEGATE,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Evicted, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Assembling_ToReverted_OnAssembleRevertResponse(t *testing.T) {
	ctx := context.Background()

	mockGrapher := graphermocks.NewGrapher(t)
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		AddPendingAssembleRequest().
		Reverts("some revert reason")

	txn, mocks := txnBuilder.Build()

	mocks.SyncPoints.On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())

	err := txn.HandleEvent(ctx, txnBuilder.BuildAssembleRevertEvent())
	require.NoError(t, err)

	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_NoTransition_OnAssembleRevertResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		AddPendingAssembleRequest().
		Reverts("some revert reason")

	txn, _ := txnBuilder.Build()

	err := txn.HandleEvent(ctx, &AssembleRevertEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    uuid.New(), //generate a new random request ID so that it won't match the pending request,
	})
	require.NoError(t, err)

	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToPooled_OnAssembleError_IfBelowRetryThreshold(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		AddPendingAssembleRequest().
		AssembleErrorCount(0).
		AssembleErrorRetryThreshold(3).
		Build()

	err := txn.HandleEvent(ctx, &AssembleErrorEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
		RequestID:            txn.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Assembling_ToEvicted_OnAssembleError_IfAboveRetryThreshold(t *testing.T) {
	ctx := context.Background()
	// assembleErrorCount(4) > threshold(3) → cannot retry → Evicted
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		AddPendingAssembleRequest().
		AssembleErrorCount(4).
		AssembleErrorRetryThreshold(3).
		Build()

	err := txn.HandleEvent(ctx, &AssembleErrorEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
		RequestID:            txn.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)
	assert.Equal(t, State_Evicted, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Assembling_ToFinal_OnAssembleRejected_TransactionUnknown(t *testing.T) {
	// When the originator reports the transaction as unknown (it has been cleaned up),
	// the coordinator transitions to State_Final.
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	err := txn.HandleEvent(ctx, &AssembleRequestRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
		RejectionReason:      engineProto.RejectionReason_TRANSACTION_UNKNOWN,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Final, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToPooled_OnAssembleRequestRejected_BlockHeightTolerance(t *testing.T) {
	// When the originator rejects the assemble request due to block height tolerance, the coordinator
	// repools so that the block heights can resync before retrying.
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		Build()
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())

	err := txn.HandleEvent(ctx, &AssembleRequestRejectedEvent{
		BaseCoordinatorEvent:   BaseCoordinatorEvent{TransactionID: txn.GetID()},
		RejectionReason:        engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
		CoordinatorBlockHeight: 100,
		AssemblerBlockHeight:   200,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToPooled_OnAssembleRequestRejected_PrivateStateDataPending(t *testing.T) {
	// When the originator rejects the assemble request because it is missing private state
	// it is entitled to, the coordinator repools so that state distribution can complete
	// before retrying.
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		Build()
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())

	err := txn.HandleEvent(ctx, &AssembleRequestRejectedEvent{
		BaseCoordinatorEvent:   BaseCoordinatorEvent{TransactionID: txn.GetID()},
		RejectionReason:        engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING,
		CoordinatorBlockHeight: 100,
		AssemblerBlockHeight:   100,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Endorsement_Gathering_ToConfirmingDispatch_OnEndorsed_IfAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(1).
		AddPendingEndorsementRequest()

	txn, mocks := builder.Build()
	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(0))
	require.NoError(t, err)
	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.True(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "expected a dispatch confirmation request to be sent, but none were sent")
}

func TestCoordinatorTransaction_Endorsement_GatheringNoTransition_IfNotAttestationPlanComplete(t *testing.T) {
	// 2 separate requirements, only 1 endorsement received → plan not yet fulfilled.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(2).
		AddPendingEndorsementRequest()

	txn, mocks := builder.Build()

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(0))
	require.NoError(t, err)
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.False(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "did not expected a dispatch confirmation request to be sent, but one was sent")
}

// TestCoordinatorTransaction_Endorsement_Gathering_Threshold1of3_TransitionsAtThreshold verifies
// that when an AttestationRequest has three parties but threshold=1, receiving one endorsement
// is sufficient to fulfill the plan and transition to Confirming_Dispatchable.
func TestCoordinatorTransaction_Endorsement_Gathering_Threshold1of3_TransitionsAtThreshold(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).Build()

	threshold := int32(1)
	const attName = "group-endorse"
	const verifierType = "ETH_ADDRESS"
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AttestationPlan: []*prototk.AttestationRequest{{
			Name:            attName,
			AttestationType: prototk.AttestationType_ENDORSE,
			VerifierType:    verifierType,
			Parties:         []string{"p1@n1", "p2@n2", "p3@n3"},
			Threshold:       &threshold,
		}},
		Endorsements: []*prototk.AttestationResult{},
	}

	// Initialise pendingEndorsementRequests manually so applyEndorsement can match the key.
	clock := common.RealClock()
	noop := func(_ context.Context, _ uuid.UUID) error { return nil }
	req1 := common.NewIdempotentRequest(ctx, clock, 0, noop)
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		attName: {
			"p1@n1": req1,
			"p2@n2": common.NewIdempotentRequest(ctx, clock, 0, noop),
			"p3@n3": common.NewIdempotentRequest(ctx, clock, 0, noop),
		},
	}

	err := txn.HandleEvent(ctx, &EndorsedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            req1.IdempotencyKey(),
		Endorsement: &prototk.AttestationResult{
			Name:            attName,
			AttestationType: prototk.AttestationType_ENDORSE,
			Verifier:        &prototk.ResolvedVerifier{Lookup: "p1@n1", VerifierType: verifierType},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState(),
		"threshold=1 met after one endorsement — must transition to Confirming_Dispatchable")
}

func TestCoordinatorTransaction_Endorsement_Gathering_ToBlocked_OnEndorsed_IfAttestationPlanCompleteAndHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	// Create a mock grapher
	mockGrapher := graphermocks.NewGrapher(t)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(mockGrapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	builder2 := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(mockGrapher).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			txn1.pt.ID: txn1,
		}).
		NumberOfRequiredEndorsers(1).
		AddPendingEndorsementRequest()
	txn2, _ := builder2.Build()

	mockGrapher.EXPECT().GetDependencies(mock.Anything, mock.Anything).Return([]uuid.UUID{txn1.GetID()})

	err := txn2.HandleEvent(ctx, builder2.BuildEndorsedEvent(0))
	require.NoError(t, err)
	assert.Equal(t, State_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())
}

func TestCoordinatorTransaction_Endorsement_Gathering_ToPooled_OnEndorseRevert_ToleranceExceeded(t *testing.T) {
	// Single-party requirement → tolerance=0. Any revert exceeds tolerance → repool.
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(mockGrapher).
		AddPendingEndorsementRequest().
		EndorseTolerance(0)

	txn, _ := builder.Build()
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())

	err := txn.HandleEvent(ctx, builder.BuildEndorseRevertEvent())
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Endorsement_Gathering_StaysInState_OnEndorseRevert_WithinTolerance(t *testing.T) {
	// A 2-of-3 requirement (tolerance=1): one revert stays within tolerance, party is marked failed.
	ctx := context.Background()
	party1 := "party1@node1"
	party2 := "party2@node2"
	party3 := "party3@node3"
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(0). // suppress the standard per-endorser plan
		Build()
	txn.endorseToleranceByRequirement = map[string]int{"endorse-multisig": 1}

	// Manually populate pending requests for the 3 parties.
	newReq := func() *common.IdempotentRequest {
		return common.NewIdempotentRequest(ctx, common.RealClock(), time.Second, func(_ context.Context, _ uuid.UUID) error { return nil })
	}
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"endorse-multisig": {
			party1: newReq(),
			party2: newReq(),
			party3: newReq(),
		},
	}

	event := &EndorseRevertEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		Party:                party1,
		AttestationRequestName: "endorse-multisig",
		RevertReason:         "assembly state is stale",
	}
	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState())
	// party1 must be marked as failed (nil sentinel), others untouched.
	assert.Nil(t, txn.pendingEndorsementRequests["endorse-multisig"][party1])
	assert.NotNil(t, txn.pendingEndorsementRequests["endorse-multisig"][party2])
	assert.NotNil(t, txn.pendingEndorsementRequests["endorse-multisig"][party3])
	assert.Equal(t, 1, txn.endorseFailureCountByRequirement["endorse-multisig"])
}

func TestCoordinatorTransaction_Endorsement_Gathering_ToPooled_OnEndorseError_ToleranceExceeded(t *testing.T) {
	// Single-party requirement → tolerance=0. Any error exceeds tolerance → repool.
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(mockGrapher).
		AddPendingEndorsementRequest().
		EndorseTolerance(0)

	txn, _ := builder.Build()
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())

	err := txn.HandleEvent(ctx, builder.BuildEndorseErrorEvent())
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Endorsement_Gathering_StaysInState_OnEndorseError_WithinTolerance(t *testing.T) {
	// A 2-of-3 requirement (threshold=2, parties=3) gives tolerance=1.
	// One failure stays within tolerance → no transition, party is recorded as failed.
	ctx := context.Background()
	party1 := "party1@node1"
	party2 := "party2@node2"
	party3 := "party3@node3"
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(0). // suppress the standard per-endorser plan
		Build()
	txn.endorseToleranceByRequirement = map[string]int{"endorse-multisig": 1}

	threshold2 := int32(2)
	txn.pt.PostAssembly.AttestationPlan = []*prototk.AttestationRequest{
		{
			Name:            "endorse-multisig",
			AttestationType: prototk.AttestationType_ENDORSE,
			Threshold:       &threshold2,
			Parties:         []string{party1, party2, party3},
		},
	}
	// Seed a pending request for party1 so the event matches.
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"endorse-multisig": {
			party1: common.NewIdempotentRequest(ctx, common.RealClock(), time.Second, func(_ context.Context, _ uuid.UUID) error { return nil }),
		},
	}

	event := &EndorseErrorEvent{
		BaseCoordinatorEvent:   BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		Party:                  party1,
		AttestationRequestName: "endorse-multisig",
	}

	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	// failure count = 1, tolerance = 1: 1 > 1 is false → no transition
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 1, txn.endorseFailureCountByRequirement["endorse-multisig"])
	// party1 nil-sentinel so it won't be re-nudged
	req, exists := txn.pendingEndorsementRequests["endorse-multisig"][party1]
	assert.True(t, exists)
	assert.Nil(t, req)
}

func TestCoordinatorTransaction_Endorsement_Gathering_ToPooled_OnEndorseError_ToleranceExceededWithMultiSig(t *testing.T) {
	// Same 2-of-3 plan, but two failures → tolerance=1 exceeded → repool.
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	party1 := "party1@node1"
	party2 := "party2@node2"
	party3 := "party3@node3"
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(mockGrapher).
		NumberOfRequiredEndorsers(0). // suppress the standard per-endorser plan
		Build()
	txn.endorseToleranceByRequirement = map[string]int{"endorse-multisig": 1}

	threshold2b := int32(2)
	txn.pt.PostAssembly.AttestationPlan = []*prototk.AttestationRequest{
		{
			Name:            "endorse-multisig",
			AttestationType: prototk.AttestationType_ENDORSE,
			Threshold:       &threshold2b,
			Parties:         []string{party1, party2, party3},
		},
	}
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"endorse-multisig": {
			party1: nil, // nil sentinel: already failed within tolerance
			party2: common.NewIdempotentRequest(ctx, common.RealClock(), time.Second, func(_ context.Context, _ uuid.UUID) error { return nil }),
		},
	}
	// Reflect that party1 already failed (first failure, within tolerance=1).
	txn.endorseFailureCountByRequirement = map[string]int{"endorse-multisig": 1}

	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())

	event := &EndorseErrorEvent{
		BaseCoordinatorEvent:   BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		Party:                  party2,
		AttestationRequestName: "endorse-multisig",
	}
	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	// failures=2 > tolerance=1 → repool
	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Endorsement_Gathering_ToPooled_OnEndorseRequestRejected_ToleranceExceeded(t *testing.T) {
	// Single-party requirement → tolerance=0. Any rejection exceeds tolerance → repool.
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(mockGrapher).
		AddPendingEndorsementRequest().
		EndorseTolerance(0)

	txn, _ := builder.Build()
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())

	err := txn.HandleEvent(ctx, builder.BuildEndorseRequestRejectedEvent())
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Endorsement_Gathering_StaysInState_OnEndorseRequestRejected_WithinTolerance(t *testing.T) {
	// A 2-of-3 requirement (threshold=2, parties=3) gives tolerance=1.
	// One rejection stays within tolerance → no transition, party is recorded as failed.
	ctx := context.Background()
	party1 := "party1@node1"
	party2 := "party2@node2"
	party3 := "party3@node3"
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(0). // suppress the standard per-endorser plan
		Build()
	txn.endorseToleranceByRequirement = map[string]int{"endorse-multisig": 1}

	threshold2 := int32(2)
	txn.pt.PostAssembly.AttestationPlan = []*prototk.AttestationRequest{
		{
			Name:            "endorse-multisig",
			AttestationType: prototk.AttestationType_ENDORSE,
			Threshold:       &threshold2,
			Parties:         []string{party1, party2, party3},
		},
	}
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"endorse-multisig": {
			party1: common.NewIdempotentRequest(ctx, common.RealClock(), time.Second, func(_ context.Context, _ uuid.UUID) error { return nil }),
			party2: common.NewIdempotentRequest(ctx, common.RealClock(), time.Second, func(_ context.Context, _ uuid.UUID) error { return nil }),
			party3: common.NewIdempotentRequest(ctx, common.RealClock(), time.Second, func(_ context.Context, _ uuid.UUID) error { return nil }),
		},
	}

	err := txn.HandleEvent(ctx, &EndorseRequestRejectedEvent{
		BaseCoordinatorEvent:   BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		Party:                  party1,
		AttestationRequestName: "endorse-multisig",
		RejectionReason:        engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
		CoordinatorBlockHeight: 100,
		EndorserBlockHeight:    200,
		BlockHeightTolerance:   5,
	})
	require.NoError(t, err)
	// failure count = 1, tolerance = 1: 1 > 1 is false → no transition
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 1, txn.endorseFailureCountByRequirement["endorse-multisig"])
	// party1 nil-sentinel so it won't be re-nudged; others remain active
	req, exists := txn.pendingEndorsementRequests["endorse-multisig"][party1]
	assert.True(t, exists)
	assert.Nil(t, req)
	assert.NotNil(t, txn.pendingEndorsementRequests["endorse-multisig"][party2])
	assert.NotNil(t, txn.pendingEndorsementRequests["endorse-multisig"][party3])
}

func TestCoordinatorTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests(t *testing.T) {
	ctx := context.Background()
	var requestCount int
	incrementCount := func(ctx context.Context, idempotencyKey uuid.UUID) error {
		requestCount++
		return nil
	}
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		AddPendingEndorsementRequestWithCallback(0, incrementCount).
		AddPendingEndorsementRequestWithCallback(1, incrementCount).
		AddPendingEndorsementRequestWithCallback(2, incrementCount).
		Build()

	err := txn.HandleEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, requestCount)
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests_Partial(t *testing.T) {
	//emulate the case where only a subset of the endorsement requests have timed out
	ctx := context.Background()
	var requestCount int
	incrementCount := func(ctx context.Context, idempotencyKey uuid.UUID) error {
		requestCount++
		return nil
	}
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(4).
		AddPendingEndorsementRequestWithCallback(0, incrementCount).
		AddPendingEndorsementRequestWithCallback(1, incrementCount).
		AddPendingEndorsementRequestWithCallback(2, incrementCount).
		AddPendingEndorsementRequestWithCallback(3, incrementCount)

	txn, _ := builder.Build()

	//2 endorsements come back in a timely manner
	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(0))
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, builder.BuildEndorsedEvent(1))
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, requestCount)
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_EndorsementGathering_ToPooled_OnStateTimeout(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).Build()

	err := txn.HandleEvent(ctx, &StateTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Blocked_ToConfirmingDispatch_OnDependencyReady_IfNotHasDependenciesNotReady(t *testing.T) {
	//TODO rethink naming of this test and/or the guard function because we end up with a double negative
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  One of which (B) is ready for dispatch and the other (C) becomes ready for dispatch,
	// triggering a transition for A to move from blocked to confirming dispatch

	mockGrapher := graphermocks.NewGrapher(t)
	sharedTransactions := map[uuid.UUID]CoordinatorTransaction{}

	txAID := uuid.New()
	txBID := uuid.New()
	txCID := uuid.New()

	txnB, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(mockGrapher).
		CoordinatorTransactions(sharedTransactions).
		TransactionID(txBID).
		Build()
	sharedTransactions[txnB.pt.ID] = txnB

	builderC := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		Grapher(mockGrapher).
		CoordinatorTransactions(sharedTransactions).
		TransactionID(txCID).
		AddPendingPreDispatchRequest()
	txnC, _ := builderC.Build()
	sharedTransactions[txnC.pt.ID] = txnC

	builderA := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(mockGrapher).
		CoordinatorTransactions(sharedTransactions).
		TransactionID(txAID)
	txnA, _ := builderA.Build()
	sharedTransactions[txnA.pt.ID] = txnA

	mockGrapher.EXPECT().GetDependencies(mock.Anything, txAID).Return([]uuid.UUID{txBID, txCID}).Maybe()
	mockGrapher.EXPECT().GetDependents(mock.Anything, txCID).Return([]uuid.UUID{txAID}).Once()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing C to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnC.HandleEvent(ctx, builderC.BuildDispatchRequestApprovedEvent())
	require.NoError(t, err)
	assert.Equal(t, State_Confirming_Dispatchable, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())
}

func TestCoordinatorTransaction_BlockedNoTransition_OnDependencyReady_IfHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  Neither of which a ready for dispatch. One of them (B) becomes ready for dispatch, but the other is still not ready
	// thus gating the triggering of a transition for A to move from blocked to confirming dispatch

	mockGrapher := graphermocks.NewGrapher(t)
	txAID := uuid.New()
	txBID := uuid.New()
	txCID := uuid.New()

	builderB := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		Grapher(mockGrapher).
		TransactionID(txBID).
		AddPendingPreDispatchRequest()
	txnB, _ := builderB.Build()

	_, _ = NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		Grapher(mockGrapher).
		TransactionID(txCID).
		AddPendingPreDispatchRequest().
		Build()

	txnA, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(mockGrapher).
		TransactionID(txAID).
		Build()

	mockGrapher.EXPECT().GetDependencies(mock.Anything, txAID).Return([]uuid.UUID{txBID, txCID}).Maybe()
	mockGrapher.EXPECT().GetDependents(mock.Anything, txBID).Return([]uuid.UUID{txAID}).Once()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing B to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnB.HandleEvent(ctx, builderB.BuildDispatchRequestApprovedEvent())
	require.NoError(t, err)

	assert.Equal(t, State_Blocked, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_ToReadyForDispatch_OnDispatchConfirmed(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error {
			return nil
		})
	txn, _ := builder.Build()

	err := txn.HandleEvent(ctx, builder.BuildDispatchRequestApprovedEvent())
	require.NoError(t, err)
	assert.Equal(t, State_Ready_For_Dispatch, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_NoTransition_OnDispatchConfirmed_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build()

	err := txn.HandleEvent(ctx, &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RequestID: uuid.New(),
	})
	require.NoError(t, err)

	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_ToEvicted_OnPreDispatchRejected_NotCurrentDelegate(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build()

	err := txn.HandleEvent(ctx, &PreDispatchRequestRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
		RequestID:            uuid.New(),
		RejectionReason:      engineProto.RejectionReason_NOT_CURRENT_DELEGATE,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Evicted, txn.GetCurrentState())
}

func TestCoordinatorTransaction_ConfirmingDispatch_ToFinal_OnPreDispatchRejected_TransactionUnknown(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build()

	err := txn.HandleEvent(ctx, &PreDispatchRequestRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
		RequestID:            uuid.New(),
		RejectionReason:      engineProto.RejectionReason_TRANSACTION_UNKNOWN,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Final, txn.GetCurrentState())
}

func TestCoordinatorTransaction_ConfirmingDispatch_NudgeRequest_OnRequestTimeout(t *testing.T) {
	ctx := context.Background()
	var nudged bool
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error {
			nudged = true
			return nil
		}).
		Build()

	err := txn.HandleEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.True(t, nudged)
	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_ToPooled_OnStateTimeout(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build()

	err := txn.HandleEvent(ctx, &StateTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func TestCoordinatorTransaction_ReadyForDispatch_ToDispatched_OnDispatched(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_PREPARE_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		PostAssembly(&components.TransactionPostAssembly{}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)
		tx.PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, &DispatchedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_NoTransition_OnCollected(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &CollectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_NoTransition_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &SubmittedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToConfirmed_OnConfirmedSuccess(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &ConfirmedSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToPooled_OnConfirmedRevert_IfRetryable(t *testing.T) {
	ctx := context.Background()
	mockGrapher := graphermocks.NewGrapher(t)
	revertReason := pldtypes.HexBytes("0x01020304")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(mockGrapher).
		BaseLedgerRevertRetryThreshold(3).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "", nil)
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())

	err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RevertReason: revertReason,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToReverted_OnConfirmedRevert_IfNonRetryable(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.HexBytes("0x01020304")
	mockGrapher := graphermocks.NewGrapher(t)
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(mockGrapher).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(false, "decoded error", nil)
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())
	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()

	err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RevertReason: revertReason,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToReverted_OnConfirmedRevert_IfThresholdExceeded(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.HexBytes("0x01020304")
	mockGrapher := graphermocks.NewGrapher(t)
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(mockGrapher).
		BaseLedgerRevertRetryThreshold(1).
		RevertCount(1).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "", nil)
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.GetID())
	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()

	err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RevertReason: revertReason,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToPreAssemblyBlocked_OnConfirmedRevert_IfRetryableAndHasUnassembledDependency(t *testing.T) {
	// Trigger the retryable path by using FailureMessage with prefix "PD012256" and an empty
	// RevertReason. This avoids calling DomainAPI.IsBaseLedgerRevertRetryable.
	ctx := context.Background()
	depID := uuid.New()
	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).TransactionID(depID).Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(1).
		ChainedDependencies(depID).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{depID: depTx}).
		Build()

	err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.GetID()},
		FailureMessage:       "PD012256: chained dependency failed",
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Reverted_ToFinal_OnHeartbeat_WhenFinalizingGracePeriodPassed(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		FinalizingGracePeriod(1).
		Build()

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Final, txn.GetCurrentState())
}

func TestCoordinatorTransaction_Confirmed_ToFinal_OnHeartbeatInterval_IfHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		HeartbeatIntervalsSinceStateChange(4).
		Build()

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Final, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Confirmed_NoTransition_OnHeartbeatInterval_IfNotHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		HeartbeatIntervalsSinceStateChange(3).
		Build()

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}
