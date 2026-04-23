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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/dependencytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// pooledTestGrapher returns a grapher backed by the same dependency tracker instance
// transactions must use via DependencyTracker(...) so post-/chained-/pre-assembly edges stay consistent.
func pooledTestGrapher(ctx context.Context) (g grapher.Grapher, dt dependencytracker.DependencyTracker) {
	dt = newTestDependencyTracker(ctx)
	g = grapher.NewGrapher(ctx, dt)
	return g, dt
}

func wireCoordinatorLookups(transactions ...CoordinatorTransaction) {
	m := make(map[uuid.UUID]CoordinatorTransaction, len(transactions))
	for _, t := range transactions {
		m[t.GetPrivateTransaction().ID] = t
	}
	lookup := func(_ context.Context, id uuid.UUID) CoordinatorTransaction { return m[id] }
	for _, t := range transactions {
		if ct, ok := t.(*coordinatorTransaction); ok {
			ct.getCoordinatorTransaction = lookup
		}
	}
}

func removeFromDependencyPrereqOf(_ context.Context, txn *coordinatorTransaction) {
	txn.dependencyTracker.GetPostAssemblyDeps().ClearPrerequisites(txn.pt.ID)
}

func Test_action_ResetTransactionLocks(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	err := action_ResetTransactionLocks(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_InitializeForNewAssembly_Success(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		Build()

	err := action_InitializeForNewAssembly(ctx, txn, nil)
	require.NoError(t, err)

	require.Nil(t, txn.pt.PreparedPublicTransaction)
	require.Nil(t, txn.pt.PreparedPrivateTransaction)
}

func Test_guard_HasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	dep2, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	txn2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		AddPendingAssembleRequest().
		InputStateIDs(dep2.pt.PostAssembly.OutputStates[0].ID)
	txn2, txn2Mocks := txn2Builder.Build()

	dep3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).DependencyTracker(depTracker).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(3).
		Build()

	txn3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		AddPendingAssembleRequest().
		InputStateIDs(dep3.pt.PostAssembly.OutputStates[0].ID)
	txn3, txn3Mocks := txn3Builder.Build()

	wireCoordinatorLookups(dep2, txn2, dep3, txn3)

	assert.False(t, guard_HasDependenciesNotReady(ctx, txn1))

	txn2Mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	txn2Mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := txn2.HandleEvent(ctx, txn2Builder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.True(t, guard_HasDependenciesNotReady(ctx, txn2))

	txn3Mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	txn3Mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err = txn3.HandleEvent(ctx, txn3Builder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.False(t, guard_HasDependenciesNotReady(ctx, txn3))
}

func Test_guard_HasDependenciesNotReady_DependencyNotReady(t *testing.T) {
	ctx := context.Background()
	g, depTracker := pooledTestGrapher(ctx)

	dep2, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(g).DependencyTracker(depTracker).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	txn2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(g).DependencyTracker(depTracker).
		AddPendingAssembleRequest().
		InputStateIDs(dep2.pt.PostAssembly.OutputStates[0].ID)
	txn2, txn2Mocks := txn2Builder.Build()
	txn2Mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	txn2Mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	txByID := map[uuid.UUID]CoordinatorTransaction{
		dep2.pt.ID: dep2,
		txn2.pt.ID: txn2,
	}
	lookup := func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
		return txByID[id]
	}
	dep2.getCoordinatorTransaction = lookup
	txn2.getCoordinatorTransaction = lookup

	err := txn2.HandleEvent(ctx, txn2Builder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.True(t, guard_HasDependenciesNotReady(ctx, txn2))
}

func Test_guard_HasDependenciesNotReady_DependencyReadyForDispatch(t *testing.T) {
	ctx := context.Background()
	g, depTracker := pooledTestGrapher(ctx)

	dep3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(g).DependencyTracker(depTracker).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(3).
		Build()

	txn3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(g).DependencyTracker(depTracker).
		AddPendingAssembleRequest().
		InputStateIDs(dep3.pt.PostAssembly.OutputStates[0].ID)
	txn3, txn3Mocks := txn3Builder.Build()
	txn3Mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	txn3Mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	txByID := map[uuid.UUID]CoordinatorTransaction{
		dep3.pt.ID: dep3,
		txn3.pt.ID: txn3,
	}
	lookup := func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
		return txByID[id]
	}
	dep3.getCoordinatorTransaction = lookup
	txn3.getCoordinatorTransaction = lookup

	err := txn3.HandleEvent(ctx, txn3Builder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.False(t, guard_HasDependenciesNotReady(ctx, txn3))
}

func Test_action_NotifyDependentsOfReset_WithDependents(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	dependentID := uuid.New()
	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		TransactionID(dependentID).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	mainTxnID := uuid.New()
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		Grapher(grapher).DependencyTracker(depTracker).
		PreAssembly(&components.TransactionPreAssembly{}).
		CoordinatorTransactions(dependentTxn).
		Build()

	wireCoordinatorLookups(dependentTxn, mainTxn)
	depTracker.GetPostAssemblyDeps().AddPrerequisites(dependentTxn.pt.ID, mainTxn.pt.ID)

	err := action_NotifyDependentsOfReset(ctx, mainTxn, nil)
	require.NoError(t, err)

	assert.Equal(t, State_Pooled, dependentTxn.GetCurrentState())
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
	grapher, depTracker := pooledTestGrapher(ctx)
	dependentID := uuid.New()
	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		TransactionID(dependentID).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		PreAssembly(&components.TransactionPreAssembly{}).
		CoordinatorTransactions(dependentTxn).
		Build()

	wireCoordinatorLookups(dependentTxn, txn)
	depTracker.GetPostAssemblyDeps().AddPrerequisites(dependentTxn.pt.ID, txn.pt.ID)

	err := txn.notifyDependentsOfReset(ctx)
	assert.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, dependentTxn.GetCurrentState())
}

func Test_notifyDependentsOfReset_HandleEventReturnsError(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	mockDependentTxn := NewMockCoordinatorTransaction(t)
	expectedError := errors.New("dependency reset notification failed")
	mockDependentID := uuid.New()
	mockDependentTxn.EXPECT().GetPrivateTransaction().Return(&components.PrivateTransaction{ID: mockDependentID}).Maybe()
	mockDependentTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DependencyResetEvent")).Return(expectedError)

	mainTxnID := uuid.New()
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		Grapher(grapher).DependencyTracker(depTracker).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(mockDependentID, mainTxn.pt.ID)
	mainTxn.getCoordinatorTransaction = func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
		if id == mainTxn.pt.ID {
			return mainTxn
		}
		if id == mockDependentID {
			return mockDependentTxn
		}
		return nil
	}

	err := mainTxn.notifyDependentsOfReset(ctx)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func Test_action_NotifyDependentsOfReset_propagatesNotifyDependentsError(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	mockDependentTxn := NewMockCoordinatorTransaction(t)
	expectedError := errors.New("dependency reset notification failed")
	mockDependentID := uuid.New()
	mockDependentTxn.EXPECT().GetPrivateTransaction().Return(&components.PrivateTransaction{ID: mockDependentID}).Maybe()
	mockDependentTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DependencyResetEvent")).Return(expectedError)

	mainTxnID := uuid.New()
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		Grapher(grapher).DependencyTracker(depTracker).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(mockDependentID, mainTxn.pt.ID)
	mainTxn.getCoordinatorTransaction = func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
		if id == mainTxn.pt.ID {
			return mainTxn
		}
		if id == mockDependentID {
			return mockDependentTxn
		}
		return nil
	}

	err := action_NotifyDependentsOfReset(ctx, mainTxn, nil)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func Test_notifyDependentsOfRepool_DependentNotFound(t *testing.T) {
	ctx := context.Background()
	missingID := uuid.New()
	grapher, depTracker := pooledTestGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(missingID, txn.pt.ID)

	err := txn.notifyDependentsOfReset(ctx)
	assert.NoError(t, err)
}

func Test_action_RemovePreAssembleDependency(t *testing.T) {
	ctx := context.Background()
	dt := newTestDependencyTracker(ctx)
	dependencyID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).DependencyTracker(dt).Build()
	dt.GetPreassemblyDeps().AddPrerequisites(txn.pt.ID, dependencyID)
	require.Equal(t, []uuid.UUID{dependencyID}, dt.GetPreassemblyDeps().GetPrerequisites(txn.pt.ID))

	err := action_RemovePreAssembleDependency(ctx, txn, nil)
	require.NoError(t, err)
	assert.Empty(t, dt.GetPreassemblyDeps().GetPrerequisites(txn.pt.ID))
}

func Test_action_RemovePreAssembleDependency_AlreadyNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()

	err := action_RemovePreAssembleDependency(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_AddPreAssemblePrereqOf(t *testing.T) {
	ctx := context.Background()
	prereqTxnID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()

	event := &NewPreAssembleDependencyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		PrereqTransactionID: prereqTxnID,
	}

	err := action_AddPreAssemblePrereqOf(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{prereqTxnID}, txn.dependencyTracker.GetPreassemblyDeps().GetDependents(txn.pt.ID))
}

func Test_action_AddPreAssemblePrereqOf_OverwritesExisting(t *testing.T) {
	ctx := context.Background()
	oldPrereqID := uuid.New()
	newPrereqID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.dependencyTracker.GetPreassemblyDeps().AddPrerequisites(oldPrereqID, txn.pt.ID)
	txn.dependencyTracker.GetPreassemblyDeps().ClearDependents(txn.pt.ID)

	event := &NewPreAssembleDependencyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		PrereqTransactionID: newPrereqID,
	}

	err := action_AddPreAssemblePrereqOf(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{newPrereqID}, txn.dependencyTracker.GetPreassemblyDeps().GetDependents(txn.pt.ID))
}

func Test_action_RemovePreAssemblePrereqOf(t *testing.T) {
	ctx := context.Background()
	prereqID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	txn.dependencyTracker.GetPreassemblyDeps().AddPrerequisites(prereqID, txn.pt.ID)
	require.Contains(t, txn.dependencyTracker.GetPreassemblyDeps().GetDependents(txn.pt.ID), prereqID)

	err := action_RemovePreAssemblePrereqOf(ctx, txn, nil)
	require.NoError(t, err)
	assert.Empty(t, txn.dependencyTracker.GetPreassemblyDeps().GetDependents(txn.pt.ID))
}

func Test_action_RemovePreAssemblePrereqOf_AlreadyNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	err := action_RemovePreAssemblePrereqOf(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_guard_HasUnassembledDependencies_False(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_True(t *testing.T) {
	ctx := context.Background()
	dependencyID := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.dependencyTracker.GetPreassemblyDeps().AddPrerequisites(txn.pt.ID, dependencyID)

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}

func TestDependsOn_SurviveRepool_InitializeForNewAssembly(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.initializeForNewAssembly(ctx)
	require.NoError(t, err)

	assert.Equal(t, []uuid.UUID{depTx.pt.ID}, depTracker.GetChainedDeps().GetPrerequisites(txn.pt.ID))
	assert.Empty(t, depTracker.GetPostAssemblyDeps().GetPrerequisites(txn.pt.ID))
}

func TestDependsOn_SurviveRepool_ActionNotifyDependentsOfReset(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := action_NotifyDependentsOfReset(ctx, txn, nil)
	require.NoError(t, err)

	assert.Empty(t, grapher.GetDependencies(ctx, txn.pt.ID))
}

func Test_guard_HasUnassembledDependencies_WithUnassembledChainedDep(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	txn.dependencyTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depID)
	txn.dependencyTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, depID)

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_NoUnassembledChainedDeps(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	txn.dependencyTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depID)

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_PreAssembleDep(t *testing.T) {
	ctx := context.Background()
	depID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	txn.dependencyTracker.GetPreassemblyDeps().AddPrerequisites(txn.pt.ID, depID)

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_ChainedDep_DelegatedGoesToPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_ChainedDep_SelectionEventUnblocksPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencySelectedForAssemblyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_ChainedDep_SelectionEventStaysBlockedIfOtherDepsNotSelected(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTxSelected, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()
	depTxNotSelected, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTxSelected, depTxNotSelected).
		Build()

	wireCoordinatorLookups(txn, depTxSelected, depTxNotSelected)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTxSelected.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, depTxSelected.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, depTxNotSelected.pt.ID)

	err := txn.HandleEvent(ctx, &DependencySelectedForAssemblyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTxSelected.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyResetBlocksIfChainedDepUnassembled(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_DependencyResetToPreAssemblyBlocked_ForgetsMints(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		AddPendingAssembleRequest().
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyResetFromChainedDepAlwaysBlocks(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyResetFromNonChainedDepStaysPooled(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetPostAssemblyDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_Pooled_DependencyConfirmedRevertedBlocksIfChainedDepUnassembled(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyConfirmedRevertedFromChainedDepBlocks(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyConfirmedRevertedFromNonChainedDepStaysPooled(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetPostAssemblyDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_ChainedDep_RepoolGoesToPreAssemblyBlockedIfChainedDepUnassembled(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_ChainedDep_RepoolGoesToPreAssemblyBlockedIfChainedDepResets(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_ChainedDep_RepoolGoesToPooledIfNonChainedDepResets(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetPostAssemblyDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_guard_HasRevertedChainedDependency_True(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	assert.True(t, guard_HasRevertedChainedDependency(ctx, txn))
}

func Test_guard_HasRevertedChainedDependency_False(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	assert.False(t, guard_HasRevertedChainedDependency(ctx, txn))
}

func Test_guard_HasRevertedChainedDependency_MissingDep(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	missing := uuid.New()
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, missing)

	assert.False(t, guard_HasRevertedChainedDependency(ctx, txn))
}

func Test_guard_HasEvictedChainedDependency_True(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Evicted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	assert.True(t, guard_HasEvictedChainedDependency(ctx, txn))
}

func Test_guard_HasEvictedChainedDependency_False(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	assert.False(t, guard_HasEvictedChainedDependency(ctx, txn))
}

func Test_guard_HasEvictedChainedDependency_NoDeps(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	assert.False(t, guard_HasEvictedChainedDependency(ctx, txn))
}

func Test_action_FinalizeOnRevertedChainedDependencyAtCreation(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

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
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Reverted, txn.GetCurrentState())
}

func Test_ChainedDep_DelegatedGoesToEvictedIfDepEvicted(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Evicted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Evicted, txn.GetCurrentState())
}

func Test_RemoveFromDependencyPrereqOf_CleansReverseLinks(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		NumberOfOutputStates(1).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetPostAssemblyDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)
	require.Contains(t, depTracker.GetPostAssemblyDeps().GetDependents(depTx.pt.ID), txn.pt.ID)

	removeFromDependencyPrereqOf(ctx, txn)
	assert.NotContains(t, depTracker.GetPostAssemblyDeps().GetDependents(depTx.pt.ID), txn.pt.ID)
}

func Test_RemoveFromDependencyPrereqOf_PreservesOtherPrereqs(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	otherID := uuid.New()
	depTx, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		NumberOfOutputStates(1).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(depTx).
		Build()

	wireCoordinatorLookups(depTx, txn)
	depTracker.GetPostAssemblyDeps().AddPrerequisites(otherID, depTx.pt.ID)
	depTracker.GetPostAssemblyDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	removeFromDependencyPrereqOf(ctx, txn)
	assert.ElementsMatch(t, []uuid.UUID{otherID}, depTracker.GetPostAssemblyDeps().GetDependents(depTx.pt.ID))
}

func Test_RemoveFromDependencyPrereqOf_DependencyNotInGrapher(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(txn.pt.ID, uuid.New())

	removeFromDependencyPrereqOf(ctx, txn)
}

func Test_PreAssembleDependencyFinalized_UnblocksPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	prereqTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(prereqTx).
		Build()

	wireCoordinatorLookups(prereqTx, txn)
	depTracker.GetPreassemblyDeps().AddPrerequisites(txn.pt.ID, prereqTx.pt.ID)

	err := txn.HandleEvent(ctx, &PreAssembleDependencyTerminatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Empty(t, depTracker.GetPreassemblyDeps().GetPrerequisites(txn.pt.ID))
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_PreAssembleDependencyFinalized_StaysBlockedWithChainedDeps(t *testing.T) {
	ctx := context.Background()
	grapher, depTracker := pooledTestGrapher(ctx)

	prereqTx, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	chainedDepTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(prereqTx, chainedDepTx).
		Build()

	wireCoordinatorLookups(prereqTx, chainedDepTx, txn)
	depTracker.GetPreassemblyDeps().AddPrerequisites(txn.pt.ID, prereqTx.pt.ID)
	depTracker.GetChainedDeps().AddPrerequisites(txn.pt.ID, chainedDepTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(txn.pt.ID, chainedDepTx.pt.ID)

	err := txn.HandleEvent(ctx, &PreAssembleDependencyTerminatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Empty(t, depTracker.GetPreassemblyDeps().GetPrerequisites(txn.pt.ID))
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}
