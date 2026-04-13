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
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_ResetTransactionLocks(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := action_ResetTransactionLocks(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_InitializeForNewAssembly_Success(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create a transaction with PreAssembly and dependencies pointing to the dependency transaction
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		Build()

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := action_InitializeForNewAssembly(ctx, txn, nil)
	require.NoError(t, err)

	require.Nil(t, txn.pt.PreparedPublicTransaction)
	require.Nil(t, txn.pt.PreparedPrivateTransaction)
}

func Test_guard_HasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Test 1: No dependencies - should return false
	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	assert.False(t, guard_HasDependenciesNotReady(ctx, txn1))

	// Test 2: Has dependency not ready
	dep2, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	txn2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		AddPendingAssembleRequest().
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				DependsOn: []uuid.UUID{dep2.pt.ID},
			},
		}).
		InputStateIDs(dep2.pt.PostAssembly.OutputStates[0].ID)
	txn2, txn2Mocks := txn2Builder.Build()

	txn2Mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := txn2.HandleEvent(ctx, txn2Builder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.True(t, guard_HasDependenciesNotReady(ctx, txn2))

	// Test 3: Has dependency ready for dispatch
	dep3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(3).
		Build()

	txn3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		AddPendingAssembleRequest().
		InputStateIDs(dep3.pt.PostAssembly.OutputStates[0].ID)
	txn3, txn3Mocks := txn3Builder.Build()

	txn3Mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err = txn3.HandleEvent(ctx, txn3Builder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.False(t, guard_HasDependenciesNotReady(ctx, txn3))
}

func Test_action_NotifyDependentsOfReset_WithDependents(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create a dependent transaction
	dependentID := uuid.New()
	dependentTxn, dependentMocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		TransactionID(dependentID).
		Grapher(grapher).
		Build()
	dependentMocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, dependentID).Return()

	// Create the main transaction
	mainTxnID := uuid.New()
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		Grapher(grapher).
		PreAssembly(&components.TransactionPreAssembly{}).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{dependentID},
			},
		}).
		Build()

	// Call action_InitializeForNewAssembly - should re-pool dependents
	err := action_NotifyDependentsOfReset(ctx, mainTxn, nil)
	require.NoError(t, err)

	// Verify the dependent transaction received the event
	assert.Equal(t, State_Pooled, dependentTxn.stateMachine.GetCurrentState())
}

func Test_action_NotifyDependentsOfReset_InitialTransitionHasNoDependents(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := action_NotifyDependentsOfReset(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_notifyDependentsOfRepool_NoDependents(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := txn.notifyDependentsOfReset(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfRepool_WithDependenciesFromPreAssembly(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)
	dependentID := uuid.New()
	_, _ = NewTransactionBuilderForTesting(t, State_Assembling).
		TransactionID(dependentID).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{dependentID},
			},
		}).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := txn.notifyDependentsOfReset(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfReset_HandleEventReturnsError(t *testing.T) {
	ctx := context.Background()
	mockGrapher := NewMockGrapher(t)
	dependentID := uuid.New()

	mockGrapher.EXPECT().Add(mock.Anything, mock.Anything).Return().Maybe()
	mockGrapher.EXPECT().ForgetMints(mock.Anything).Return().Maybe()

	mockDependentTxn := NewMockCoordinatorTransaction(t)
	expectedError := errors.New("dependency reset notification failed")
	mockDependentTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DependencyResetEvent")).Return(expectedError)

	mockGrapher.EXPECT().TransactionByID(ctx, dependentID).Return(mockDependentTxn)

	mainTxnID := uuid.New()
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		Grapher(mockGrapher).
		PreAssembly(&components.TransactionPreAssembly{}).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{dependentID},
			},
		}).
		Build()

	err := mainTxn.notifyDependentsOfReset(ctx)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func Test_action_NotifyDependentsOfReset_propagatesNotifyDependentsError(t *testing.T) {
	ctx := context.Background()
	mockGrapher := NewMockGrapher(t)
	dependentID := uuid.New()

	mockGrapher.EXPECT().Add(mock.Anything, mock.Anything).Return().Maybe()
	mockGrapher.EXPECT().ForgetMints(mock.Anything).Return().Maybe()

	mockDependentTxn := NewMockCoordinatorTransaction(t)
	expectedError := errors.New("dependency reset notification failed")
	mockDependentTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DependencyResetEvent")).Return(expectedError)

	mockGrapher.EXPECT().TransactionByID(ctx, dependentID).Return(mockDependentTxn)

	mainTxnID := uuid.New()
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		Grapher(mockGrapher).
		PreAssembly(&components.TransactionPreAssembly{}).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{dependentID},
			},
		}).
		Build()

	err := action_NotifyDependentsOfReset(ctx, mainTxn, nil)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
	require.Len(t, mainTxn.dependencies.PostAssemble.PrereqOf, 1, "dependencies must not be cleared when notify fails")
	assert.Equal(t, dependentID, mainTxn.dependencies.PostAssemble.PrereqOf[0])
}

func Test_notifyDependentsOfRepool_DependentNotFound(t *testing.T) {
	ctx := context.Background()
	missingID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{missingID},
			},
		}).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := txn.notifyDependentsOfReset(ctx)
	assert.NoError(t, err)
}

func Test_action_RemovePreAssembleDependency(t *testing.T) {
	ctx := context.Background()
	dependencyID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).Build()
	txn.dependencies.PreAssemble.DependsOn = &dependencyID

	require.NotNil(t, txn.dependencies.PreAssemble.DependsOn)

	err := action_RemovePreAssembleDependency(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.dependencies.PreAssemble.DependsOn)
}

func Test_action_RemovePreAssembleDependency_AlreadyNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.dependencies.PreAssemble.DependsOn = nil

	err := action_RemovePreAssembleDependency(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.dependencies.PreAssemble.DependsOn)
}

func Test_action_AddPreAssemblePrereqOf(t *testing.T) {
	ctx := context.Background()
	prereqTxnID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	require.Nil(t, txn.dependencies.PreAssemble.PrereqOf)

	event := &NewPreAssembleDependencyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		PrereqTransactionID: prereqTxnID,
	}

	err := action_AddPreAssemblePrereqOf(ctx, txn, event)
	require.NoError(t, err)
	require.NotNil(t, txn.dependencies.PreAssemble.PrereqOf)
	assert.Equal(t, prereqTxnID, *txn.dependencies.PreAssemble.PrereqOf)
}

func Test_action_AddPreAssemblePrereqOf_OverwritesExisting(t *testing.T) {
	ctx := context.Background()
	oldPrereqID := uuid.New()
	newPrereqID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.dependencies.PreAssemble.PrereqOf = &oldPrereqID

	event := &NewPreAssembleDependencyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		PrereqTransactionID: newPrereqID,
	}

	err := action_AddPreAssemblePrereqOf(ctx, txn, event)
	require.NoError(t, err)
	require.NotNil(t, txn.dependencies.PreAssemble.PrereqOf)
	assert.Equal(t, newPrereqID, *txn.dependencies.PreAssemble.PrereqOf)
}

func Test_action_RemovePreAssemblePrereqOf(t *testing.T) {
	ctx := context.Background()
	prereqID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	txn.dependencies.PreAssemble.PrereqOf = &prereqID

	require.NotNil(t, txn.dependencies.PreAssemble.PrereqOf)

	err := action_RemovePreAssemblePrereqOf(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.dependencies.PreAssemble.PrereqOf)
}

func Test_action_RemovePreAssemblePrereqOf_AlreadyNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	txn.dependencies.PreAssemble.PrereqOf = nil

	err := action_RemovePreAssemblePrereqOf(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.dependencies.PreAssemble.PrereqOf)
}

func Test_guard_HasUnassembledDependencies_False(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.dependencies.PreAssemble.DependsOn = nil

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_True(t *testing.T) {
	ctx := context.Background()
	dependencyID := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.dependencies.PreAssemble.DependsOn = &dependencyID

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}

func TestDependsOn_SurviveRepool_InitializeForNewAssembly(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	depTx.dependencies.Chained.PrereqOf = []uuid.UUID{txn.pt.ID}

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.initializeForNewAssembly(ctx)
	require.NoError(t, err)

	assert.Equal(t, []uuid.UUID{depTx.pt.ID}, txn.dependencies.Chained.DependsOn)
	assert.Empty(t, txn.dependencies.PostAssemble.DependsOn)
}

func TestDependsOn_SurviveRepool_ActionNotifyDependentsOfReset(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}

	err := action_NotifyDependentsOfReset(ctx, txn, nil)
	require.NoError(t, err)

	assert.Empty(t, txn.dependencies.PostAssemble.DependsOn)
}

func Test_guard_HasUnassembledDependencies_WithUnassembledChainedDep(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{depID: {}}

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_NoUnassembledChainedDeps(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_PreAssembleDep(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	txn.dependencies.PreAssemble.DependsOn = &depID

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_ChainedDep_DelegatedGoesToPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{depTx.pt.ID: {}}

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_ChainedDep_SelectionEventUnblocksPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{depTx.pt.ID: {}}

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &DependencySelectedForAssemblyEvent{
		BaseCoordinatorEvent:    BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID: depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_ChainedDep_SelectionEventStaysBlockedIfOtherDepsNotSelected(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTxSelected, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()
	depTxNotSelected, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTxSelected.pt.ID, depTxNotSelected.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{
		depTxSelected.pt.ID:    {},
		depTxNotSelected.pt.ID: {},
	}

	err := txn.HandleEvent(ctx, &DependencySelectedForAssemblyEvent{
		BaseCoordinatorEvent:    BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID: depTxSelected.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyResetBlocksIfChainedDepUnassembled(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent:    BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID: depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyResetFromChainedDepAlwaysBlocks(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyResetFromNonChainedDepStaysPooled(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_Pooled_DependencyConfirmedRevertedBlocksIfChainedDepUnassembled(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent:    BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID: depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyConfirmedRevertedFromChainedDepBlocks(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyConfirmedRevertedFromNonChainedDepStaysPooled(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_ChainedDep_RepoolGoesToPreAssemblyBlockedIfChainedDepUnassembled(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_ChainedDep_RepoolGoesToPreAssemblyBlockedIfChainedDepResets(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_ChainedDep_RepoolGoesToPooledIfNonChainedDepResets(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_guard_HasRevertedChainedDependency_True(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}

	assert.True(t, guard_HasRevertedChainedDependency(ctx, txn))
}

func Test_guard_HasRevertedChainedDependency_False(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}

	assert.False(t, guard_HasRevertedChainedDependency(ctx, txn))
}

func Test_guard_HasRevertedChainedDependency_MissingDep(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{uuid.New()}

	assert.False(t, guard_HasRevertedChainedDependency(ctx, txn))
}

func Test_guard_HasEvictedChainedDependency_True(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Evicted).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}

	assert.True(t, guard_HasEvictedChainedDependency(ctx, txn))
}

func Test_guard_HasEvictedChainedDependency_False(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}

	assert.False(t, guard_HasEvictedChainedDependency(ctx, txn))
}

func Test_guard_HasEvictedChainedDependency_NoDeps(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	assert.False(t, guard_HasEvictedChainedDependency(ctx, txn))
}

func Test_action_FinalizeOnRevertedChainedDependencyAtCreation(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}

	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Run(func(_ context.Context, req *syncpoints.TransactionFinalizeRequest, _ func(context.Context), _ func(context.Context, error)) {
		assert.Equal(t, txn.pt.ID, req.TransactionID)
		assert.Contains(t, req.FailureMessage, depTx.pt.ID.String())
	}).Return()

	err := action_FinalizeOnRevertedChainedDependencyAtCreation(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_ChainedDep_DelegatedGoesToRevertedIfDepReverted(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}

	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Reverted, txn.GetCurrentState())
}

func Test_ChainedDep_DelegatedGoesToEvictedIfDepEvicted(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Evicted).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Evicted, txn.GetCurrentState())
}

func Test_PreAssembleDependencyFinalized_UnblocksPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	prereqTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).
		Build()
	txn.dependencies.PreAssemble.DependsOn = &prereqTx.pt.ID

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &PreAssembleDependencyTerminatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Nil(t, txn.dependencies.PreAssemble.DependsOn)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_PreAssembleDependencyFinalized_StaysBlockedWithChainedDeps(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	prereqTx, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(grapher).
		Build()

	chainedDepTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).
		Build()
	txn.dependencies.PreAssemble.DependsOn = &prereqTx.pt.ID
	txn.dependencies.Chained.DependsOn = []uuid.UUID{chainedDepTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{chainedDepTx.pt.ID: {}}

	err := txn.HandleEvent(ctx, &PreAssembleDependencyTerminatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Nil(t, txn.dependencies.PreAssemble.DependsOn)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}
