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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func removeFromDependencyPrereqOf(ctx context.Context, txn *coordinatorTransaction) {
	txn.dependencyTracker.GetPostAssemblyDeps().ClearPrerequisites(ctx, txn.pt.ID)
}

func Test_action_ResetTransactionLocks(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	err := action_ResetTransactionLocks(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_InitializeForNewAssembly_Success(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

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
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()
	sharedTransactions := map[uuid.UUID]CoordinatorTransaction{}

	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(sharedTransactions).
		Build()
	sharedTransactions[txn1.pt.ID] = txn1

	dep2, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(sharedTransactions).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()
	sharedTransactions[dep2.pt.ID] = dep2

	txn2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(sharedTransactions).
		AddPendingAssembleRequest().
		InputStateIDs(dep2.pt.PostAssembly.OutputStates[0].ID)
	txn2, txn2Mocks := txn2Builder.Build()
	sharedTransactions[txn2.pt.ID] = txn2

	dep3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(sharedTransactions).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(3).
		Build()
	sharedTransactions[dep3.pt.ID] = dep3

	txn3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(sharedTransactions).
		AddPendingAssembleRequest().
		InputStateIDs(dep3.pt.PostAssembly.OutputStates[0].ID)
	txn3, txn3Mocks := txn3Builder.Build()
	sharedTransactions[txn3.pt.ID] = txn3

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
	ctx := t.Context()
	g, depTracker := newTestGrapher()

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
	stateLookup := coordinatorTransactionStateLookup(txByID)
	dep2.getCoordinatorTransactionState = stateLookup
	txn2.getCoordinatorTransactionState = stateLookup

	err := txn2.HandleEvent(ctx, txn2Builder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.True(t, guard_HasDependenciesNotReady(ctx, txn2))
}

func Test_guard_HasDependenciesNotReady_DependencyReadyForDispatch(t *testing.T) {
	ctx := t.Context()
	g, depTracker := newTestGrapher()

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
	stateLookup := coordinatorTransactionStateLookup(txByID)
	dep3.getCoordinatorTransactionState = stateLookup
	txn3.getCoordinatorTransactionState = stateLookup

	err := txn3.HandleEvent(ctx, txn3Builder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.False(t, guard_HasDependenciesNotReady(ctx, txn3))
}

func Test_action_NotifyDependentsOfReset_WithDependents(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

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
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			dependentTxn.GetPrivateTransaction().ID: dependentTxn,
		}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, dependentTxn.pt.ID, mainTxn.pt.ID)

	err := action_NotifyDependentsOfReset(ctx, mainTxn, nil)
	require.NoError(t, err)

	assert.Equal(t, State_Pooled, dependentTxn.GetCurrentState())
}

func Test_action_NotifyDependentsOfReset_InitialTransitionHasNoDependents(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := action_NotifyDependentsOfReset(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_notifyDependentsOfRepool_WithDependenciesFromPreAssembly(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()
	dependentID := uuid.New()
	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		TransactionID(dependentID).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		PreAssembly(&components.TransactionPreAssembly{}).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			dependentTxn.GetPrivateTransaction().ID: dependentTxn,
		}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, dependentTxn.pt.ID, txn.pt.ID)

	txn.notifyDependentsOfReset(ctx)
	assert.Equal(t, State_PreAssembly_Blocked, dependentTxn.GetCurrentState())
}

func Test_notifyDependentsOfReset_QueuesWithoutExistenceCheck(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	mockDependentID := uuid.New()

	mainTxnID := uuid.New()
	var queued int
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		Grapher(grapher).DependencyTracker(depTracker).
		PreAssembly(&components.TransactionPreAssembly{}).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			mockDependentID: nil,
		}).
		QueueEventForCoordinator(func(context.Context, common.Event) { queued++ }).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, mockDependentID, mainTxn.pt.ID)

	mainTxn.notifyDependentsOfReset(ctx)
	assert.Equal(t, 0, queued)
}

func Test_action_NotifyDependentsOfReset_QueuesWithoutExistenceCheck(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	mockDependentID := uuid.New()

	mainTxnID := uuid.New()
	var queued int
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		Grapher(grapher).DependencyTracker(depTracker).
		PreAssembly(&components.TransactionPreAssembly{}).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			mockDependentID: nil,
		}).
		QueueEventForCoordinator(func(context.Context, common.Event) { queued++ }).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, mockDependentID, mainTxn.pt.ID)

	err := action_NotifyDependentsOfReset(ctx, mainTxn, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, queued)
}

func Test_action_RemovePreAssembleDependency(t *testing.T) {
	ctx := t.Context()
	_, dt := newTestGrapher()
	dependencyID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).DependencyTracker(dt).Build()
	dt.GetPreassemblyDeps().AddPrerequisite(ctx, txn.pt.ID, dependencyID)
	prereq, ok := dt.GetPreassemblyDeps().GetPrerequisite(ctx, txn.pt.ID)
	require.True(t, ok)
	require.Equal(t, dependencyID, prereq)

	err := action_RemovePreAssembleDependency(ctx, txn, nil)
	require.NoError(t, err)
	_, ok = dt.GetPreassemblyDeps().GetPrerequisite(ctx, txn.pt.ID)
	assert.False(t, ok)
}

func Test_action_RemovePreAssembleDependency_AlreadyNil(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()

	err := action_RemovePreAssembleDependency(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_RemovePreAssemblePrereqOf(t *testing.T) {
	ctx := t.Context()
	prereqID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	txn.dependencyTracker.GetPreassemblyDeps().AddPrerequisite(ctx, prereqID, txn.pt.ID)
	dependent, ok := txn.dependencyTracker.GetPreassemblyDeps().GetDependent(ctx, txn.pt.ID)
	require.True(t, ok)
	require.Equal(t, prereqID, dependent)

	err := action_RemovePreAssemblePrereqOf(ctx, txn, nil)
	require.NoError(t, err)
	_, ok = txn.dependencyTracker.GetPreassemblyDeps().GetDependent(ctx, txn.pt.ID)
	assert.False(t, ok)
}

func Test_action_RemovePreAssemblePrereqOf_AlreadyNil(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	err := action_RemovePreAssemblePrereqOf(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_guard_HasUnassembledDependencies_False(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_True(t *testing.T) {
	ctx := t.Context()
	dependencyID := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.dependencyTracker.GetPreassemblyDeps().AddPrerequisite(ctx, txn.pt.ID, dependencyID)

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}

func TestDependsOn_SurviveRepool_InitializeForNewAssembly(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.initializeForNewAssembly(ctx)
	require.NoError(t, err)

	assert.Equal(t, []uuid.UUID{depTx.pt.ID}, depTracker.GetChainedDeps().GetPrerequisites(ctx, txn.pt.ID))
	assert.Empty(t, depTracker.GetPostAssemblyDeps().GetPrerequisites(ctx, txn.pt.ID))
}

func TestDependsOn_SurviveRepool_ActionNotifyDependentsOfReset(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := action_NotifyDependentsOfReset(ctx, txn, nil)
	require.NoError(t, err)

	assert.Empty(t, grapher.GetDependencies(ctx, txn.pt.ID))
}

func Test_guard_HasUnassembledDependencies_WithUnassembledChainedDep(t *testing.T) {
	ctx := t.Context()
	depID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	txn.dependencyTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depID)
	txn.dependencyTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, depID)

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_NoUnassembledChainedDeps(t *testing.T) {
	ctx := t.Context()
	depID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	txn.dependencyTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depID)

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_PreAssembleDep(t *testing.T) {
	ctx := t.Context()
	depID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	txn.dependencyTracker.GetPreassemblyDeps().AddPrerequisite(ctx, txn.pt.ID, depID)

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_ChainedDep_DelegatedGoesToPreAssemblyBlocked(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_ChainedDep_SelectionEventUnblocksPreAssemblyBlocked(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencySelectedForAssemblyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_ChainedDep_SelectionEventStaysBlockedIfOtherDepsNotSelected(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTxSelected, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()
	depTxNotSelected, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTxSelected.GetPrivateTransaction().ID:    depTxSelected,
			depTxNotSelected.GetPrivateTransaction().ID: depTxNotSelected,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTxSelected.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, depTxSelected.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, depTxNotSelected.pt.ID)

	err := txn.HandleEvent(ctx, &DependencySelectedForAssemblyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTxSelected.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyResetBlocksIfChainedDepUnassembled(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_DependencyResetToPreAssemblyBlocked_ForgetsMints(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		AddPendingAssembleRequest().
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyResetFromChainedDepAlwaysBlocks(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyResetFromNonChainedDepStaysPooled(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_Pooled_DependencyConfirmedRevertedBlocksIfChainedDepUnassembled(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyConfirmedRevertedFromChainedDepBlocks(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_Pooled_DependencyConfirmedRevertedFromNonChainedDepStaysPooled(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_ChainedDep_RepoolGoesToPreAssemblyBlockedIfChainedDepUnassembled(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_ChainedDep_RepoolGoesToPreAssemblyBlockedIfChainedDepResets(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}

func Test_ChainedDep_RepoolGoesToPooledIfNonChainedDepResets(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_guard_HasRevertedChainedDependency_True(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	assert.True(t, guard_HasRevertedChainedDependency(ctx, txn))
}

func Test_guard_HasRevertedChainedDependency_False(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	assert.False(t, guard_HasRevertedChainedDependency(ctx, txn))
}

func Test_guard_HasRevertedChainedDependency_MissingDep(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	missing := uuid.New()
	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, missing)

	assert.False(t, guard_HasRevertedChainedDependency(ctx, txn))
}

func Test_guard_HasEvictedChainedDependency_True(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Evicted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	assert.True(t, guard_HasEvictedChainedDependency(ctx, txn))
}

func Test_guard_HasEvictedChainedDependency_False(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	assert.False(t, guard_HasEvictedChainedDependency(ctx, txn))
}

func Test_guard_HasEvictedChainedDependency_NoDeps(t *testing.T) {
	ctx := t.Context()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	assert.False(t, guard_HasEvictedChainedDependency(ctx, txn))
}

func Test_action_FinalizeOnRevertedChainedDependencyAtCreation(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Run(func(_ context.Context, req *syncpoints.TransactionFinalizeRequest, onSuccess func(context.Context), onFailure func(context.Context, error)) {
		assert.Equal(t, txn.pt.ID, req.TransactionID)
		assert.Contains(t, req.FailureMessage, depTx.pt.ID.String())
		if onSuccess != nil {
			onSuccess(ctx)
		}
		if onFailure != nil {
			onFailure(ctx, assert.AnError)
		}
	}).Return()

	err := action_FinalizeOnRevertedChainedDependencyAtCreation(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_FinalizeOnRevertedChainedDependencyAtCreation_NoRevertedDependency(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := action_FinalizeOnRevertedChainedDependencyAtCreation(ctx, txn, nil)
	require.NoError(t, err)
	mocks.SyncPoints.AssertNotCalled(t, "QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func Test_validator_IsChainedDependency_UnknownEventType(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	ok, err := validator_IsChainedDependency(ctx, txn, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.False(t, ok)
}

func Test_action_MarkChainedDependencyUnassembled_UnknownEventType(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	err := action_MarkChainedDependencyUnassembled(ctx, txn, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
}

func Test_ChainedDep_DelegatedGoesToRevertedIfDepReverted(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

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
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Evicted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Evicted, txn.GetCurrentState())
}

func Test_RemoveFromDependencyPrereqOf_CleansReverseLinks(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	depTx, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		NumberOfOutputStates(1).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)
	require.Contains(t, depTracker.GetPostAssemblyDeps().GetDependents(ctx, depTx.pt.ID), txn.pt.ID)

	removeFromDependencyPrereqOf(ctx, txn)
	assert.NotContains(t, depTracker.GetPostAssemblyDeps().GetDependents(ctx, depTx.pt.ID), txn.pt.ID)
}

func Test_RemoveFromDependencyPrereqOf_PreservesOtherPrereqs(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	otherID := uuid.New()
	depTx, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).DependencyTracker(depTracker).
		NumberOfOutputStates(1).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, otherID, depTx.pt.ID)
	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, txn.pt.ID, depTx.pt.ID)

	removeFromDependencyPrereqOf(ctx, txn)
	assert.ElementsMatch(t, []uuid.UUID{otherID}, depTracker.GetPostAssemblyDeps().GetDependents(ctx, depTx.pt.ID))
}

func Test_RemoveFromDependencyPrereqOf_DependencyNotInGrapher(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	depTracker.GetPostAssemblyDeps().AddPrerequisites(ctx, txn.pt.ID, uuid.New())

	removeFromDependencyPrereqOf(ctx, txn)
}

func Test_PreAssembleDependencyFinalized_UnblocksPreAssemblyBlocked(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	prereqTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			prereqTx.GetPrivateTransaction().ID: prereqTx,
		}).
		Build()

	depTracker.GetPreassemblyDeps().AddPrerequisite(ctx, txn.pt.ID, prereqTx.pt.ID)

	err := txn.HandleEvent(ctx, &PreAssembleDependencyTerminatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	_, ok := depTracker.GetPreassemblyDeps().GetPrerequisite(ctx, txn.pt.ID)
	assert.False(t, ok)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func Test_PreAssembleDependencyFinalized_StaysBlockedWithChainedDeps(t *testing.T) {
	ctx := t.Context()
	grapher, depTracker := newTestGrapher()

	prereqTx, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	chainedDepTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).DependencyTracker(depTracker).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).DependencyTracker(depTracker).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			prereqTx.GetPrivateTransaction().ID:     prereqTx,
			chainedDepTx.GetPrivateTransaction().ID: chainedDepTx,
		}).
		Build()

	depTracker.GetPreassemblyDeps().AddPrerequisite(ctx, txn.pt.ID, prereqTx.pt.ID)
	depTracker.GetChainedDeps().AddPrerequisites(ctx, txn.pt.ID, chainedDepTx.pt.ID)
	depTracker.GetChainedDeps().AddUnassembledDependencies(ctx, txn.pt.ID, chainedDepTx.pt.ID)

	err := txn.HandleEvent(ctx, &PreAssembleDependencyTerminatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	_, ok := depTracker.GetPreassemblyDeps().GetPrerequisite(ctx, txn.pt.ID)
	assert.False(t, ok)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
}
