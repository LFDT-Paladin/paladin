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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_ResetTransactionLocks(t *testing.T) {
	ctx := context.Background()
	mockGrapher := grapher.NewMockGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Grapher(mockGrapher).Build()

	mockGrapher.EXPECT().ForgetLocks(txn.pt.ID)
	mockGrapher.EXPECT().ForgetMints(txn.pt.ID)

	err := action_ResetTransactionLocks(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_InitializeForNewAssembly_Success(t *testing.T) {
	ctx := context.Background()
	mockGrapher := grapher.NewMockGrapher(t)

	// Create a transaction with PreAssembly and dependencies pointing to the dependency transaction
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(mockGrapher).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		Build()

	mockGrapher.EXPECT().ForgetLocks(txn.pt.ID)
	mockGrapher.EXPECT().ForgetMints(txn.pt.ID)

	err := action_InitializeForNewAssembly(ctx, txn, nil)
	require.NoError(t, err)

	require.Nil(t, txn.pt.PreparedPublicTransaction)
	require.Nil(t, txn.pt.PreparedPrivateTransaction)
}

func Test_action_InitializeForNewAssembly_MissingDependency(t *testing.T) {
	ctx := context.Background()
	mockGrapher := grapher.NewMockGrapher(t)
	// Create a transaction with a dependency that doesn't exist in grapher
	unknownDependencyID := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(mockGrapher).
		PredefinedDependencies(unknownDependencyID).
		Build()

	mockGrapher.EXPECT().ForgetLocks(txn.pt.ID)
	mockGrapher.EXPECT().ForgetMints(txn.pt.ID)
	// Call action_InitializeForNewAssembly - should not error, just log
	err := action_InitializeForNewAssembly(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_guard_HasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	t.Run("no dependencies", func(t *testing.T) {
		mockG := grapher.NewMockGrapher(t)
		txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(mockG).Build()
		mockG.EXPECT().GetDependencies(mock.Anything, txn1.pt.ID).Return(nil)
		assert.False(t, guard_HasDependenciesNotReady(ctx, txn1))
	})

	t.Run("has dependency not ready", func(t *testing.T) {
		g := grapher.NewGrapher(ctx)

		dep2, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
			Grapher(g).
			NumberOfOutputStates(1).
			NumberOfRequiredEndorsers(3).
			NumberOfEndorsements(2).
			Build()

		txn2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
			Grapher(g).
			AddPendingAssembleRequest().
			InputStateIDs(dep2.pt.PostAssembly.OutputStates[0].ID)
		txn2, txn2Mocks := txn2Builder.Build()

		txByID := map[uuid.UUID]CoordinatorTransaction{
			dep2.pt.ID: dep2,
			txn2.pt.ID: txn2,
		}
		lookup := func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
			return txByID[id]
		}
		dep2.getCoordinatorTransaction = lookup
		txn2.getCoordinatorTransaction = lookup

		txn2Mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)
		txn2Mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

		err := txn2.HandleEvent(ctx, txn2Builder.BuildAssembleSuccessEvent())
		require.NoError(t, err)
		assert.True(t, guard_HasDependenciesNotReady(ctx, txn2))
	})

	t.Run("has dependency ready for dispatch", func(t *testing.T) {
		g := grapher.NewGrapher(ctx)

		dep3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
			Grapher(g).
			NumberOfOutputStates(1).
			NumberOfRequiredEndorsers(3).
			NumberOfEndorsements(3).
			Build()

		txn3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
			Grapher(g).
			AddPendingAssembleRequest().
			InputStateIDs(dep3.pt.PostAssembly.OutputStates[0].ID)
		txn3, txn3Mocks := txn3Builder.Build()

		txByID := map[uuid.UUID]CoordinatorTransaction{
			dep3.pt.ID: dep3,
			txn3.pt.ID: txn3,
		}
		lookup := func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
			return txByID[id]
		}
		dep3.getCoordinatorTransaction = lookup
		txn3.getCoordinatorTransaction = lookup

		txn3Mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)
		txn3Mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

		err := txn3.HandleEvent(ctx, txn3Builder.BuildAssembleSuccessEvent())
		require.NoError(t, err)
		assert.False(t, guard_HasDependenciesNotReady(ctx, txn3))
	})
}

func Test_action_NotifyDependentsOfReset_WithDependents(t *testing.T) {
	ctx := context.Background()
	mockG := grapher.NewMockGrapher(t)

	dependentID := uuid.New()
	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		TransactionID(dependentID).
		Grapher(mockG).
		Build()

	mainTxnID := uuid.New()
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		Grapher(mockG).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	txByID := map[uuid.UUID]CoordinatorTransaction{
		mainTxnID:    mainTxn,
		dependentID: dependentTxn,
	}
	lookup := func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
		return txByID[id]
	}
	mainTxn.getCoordinatorTransaction = lookup
	dependentTxn.getCoordinatorTransaction = lookup

	mockG.EXPECT().GetDependants(mock.Anything, mainTxnID).Return([]uuid.UUID{dependentID})
	mockG.EXPECT().GetDependants(mock.Anything, dependentID).Return(nil)
	mockG.EXPECT().RemoveAllDependencyLinks(dependentID)
	mockG.EXPECT().ForgetMints(dependentID)
	mockG.EXPECT().ForgetLocks(dependentID)
	mockG.EXPECT().RemoveAllDependencyLinks(mainTxnID)

	err := action_NotifyDependentsOfReset(ctx, mainTxn, nil)
	require.NoError(t, err)

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
	mockGrapher := grapher.NewMockGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(mockGrapher).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	mockGrapher.EXPECT().GetDependants(mock.Anything, txn.pt.ID).Return([]uuid.UUID{})

	err := txn.notifyDependentsOfReset(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfReset_HandleEventReturnsError(t *testing.T) {
	ctx := context.Background()
	mockGrapher := grapher.NewMockGrapher(t)
	dependentID := uuid.New()

	mockGrapher.EXPECT().GetDependants(mock.Anything, mock.Anything).Return([]uuid.UUID{dependentID})

	mockDependentTxn := NewMockCoordinatorTransaction(t)
	expectedError := errors.New("dependency reset notification failed")
	mockDependentTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DependencyResetEvent")).Return(expectedError)
	mockDependentTxn.EXPECT().GetPrivateTransaction().Return(&components.PrivateTransaction{ID: dependentID})

	mainTxnID := uuid.New()
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		CoordinatorTransactions(mockDependentTxn).
		Grapher(mockGrapher).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := mainTxn.notifyDependentsOfReset(ctx)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func Test_action_NotifyDependentsOfReset_propagatesNotifyDependentsError(t *testing.T) {
	ctx := context.Background()
	grapher := grapher.NewGrapher(ctx)
	dependentID := uuid.New()
	mainTxnID := uuid.New()

	// Create a dependency between main TX and dependent TX
	state := &components.FullState{ID: pldtypes.HexBytes(uuid.New().String())}

	// Main TX mints some state
	grapher.AddMinter(ctx, []*components.FullState{state}, mainTxnID)

	// Dependent TX locks it, so and hence becomes dependent on the main TX
	grapher.LockMintsOnSpend(ctx, []*components.FullState{state}, dependentID)

	// Check the dependency chain has been created
	dependencies := grapher.GetDependencies(ctx, dependentID)
	require.Len(t, dependencies, 1)
	assert.Equal(t, mainTxnID, dependencies[0])
	dependants := grapher.GetDependants(ctx, mainTxnID)
	require.Len(t, dependants, 1)
	assert.Equal(t, dependentID, dependants[0])

	mockDependentTxn := NewMockCoordinatorTransaction(t)
	expectedError := errors.New("dependency reset notification failed")
	mockDependentTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DependencyResetEvent")).Return(expectedError)
	mockDependentTxn.EXPECT().GetPrivateTransaction().Return(&components.PrivateTransaction{ID: dependentID})
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(mainTxnID).
		CoordinatorTransactions(mockDependentTxn).
		Grapher(grapher).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := action_NotifyDependentsOfReset(ctx, mainTxn, nil)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
	require.Len(t, grapher.GetDependants(ctx, mainTxnID), 1, "dependencies must not be cleared when notify fails")
	assert.Equal(t, dependentID, grapher.GetDependants(ctx, mainTxnID)[0])
	require.Len(t, grapher.GetDependencies(ctx, dependentID), 1, "dependencies must not be cleared when notify fails")
	assert.Equal(t, mainTxnID, grapher.GetDependencies(ctx, dependentID)[0])
}

func Test_notifyDependentsOfRepool_DependentTxNotKnownToCoordinator(t *testing.T) {
	ctx := context.Background()
	missingID := uuid.New()
	mockGrapher := grapher.NewMockGrapher(t)
	mockGrapher.EXPECT().GetDependants(mock.Anything, mock.Anything).Return([]uuid.UUID{missingID})

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(mockGrapher).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := txn.notifyDependentsOfReset(ctx)
	assert.NoError(t, err)
}

func Test_action_RemovePreAssembleDependency(t *testing.T) {
	ctx := context.Background()
	dependencyID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).Build()
	txn.preAssembleDependsOn = &dependencyID

	require.NotNil(t, txn.preAssembleDependsOn)

	err := action_RemovePreAssembleDependency(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.preAssembleDependsOn)
}

func Test_action_RemovePreAssembleDependency_AlreadyNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.preAssembleDependsOn = nil

	err := action_RemovePreAssembleDependency(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.preAssembleDependsOn)
}

func Test_action_AddPreAssemblePrereqOf(t *testing.T) {
	ctx := context.Background()
	prereqTxnID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	require.Nil(t, txn.preAssemblePrereqOf)

	event := &NewPreAssembleDependencyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		PrereqTransactionID: prereqTxnID,
	}

	err := action_AddPreAssemblePrereqOf(ctx, txn, event)
	require.NoError(t, err)
	require.NotNil(t, txn.preAssemblePrereqOf)
	assert.Equal(t, prereqTxnID, *txn.preAssemblePrereqOf)
}

func Test_action_AddPreAssemblePrereqOf_OverwritesExisting(t *testing.T) {
	ctx := context.Background()
	oldPrereqID := uuid.New()
	newPrereqID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.preAssemblePrereqOf = &oldPrereqID

	event := &NewPreAssembleDependencyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		PrereqTransactionID: newPrereqID,
	}

	err := action_AddPreAssemblePrereqOf(ctx, txn, event)
	require.NoError(t, err)
	require.NotNil(t, txn.preAssemblePrereqOf)
	assert.Equal(t, newPrereqID, *txn.preAssemblePrereqOf)
}

func Test_action_RemovePreAssemblePrereqOf(t *testing.T) {
	ctx := context.Background()
	prereqID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	txn.preAssemblePrereqOf = &prereqID

	require.NotNil(t, txn.preAssemblePrereqOf)

	err := action_RemovePreAssemblePrereqOf(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.preAssemblePrereqOf)
}

func Test_action_RemovePreAssemblePrereqOf_AlreadyNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	txn.preAssemblePrereqOf = nil

	err := action_RemovePreAssemblePrereqOf(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.preAssemblePrereqOf)
}

func Test_guard_HasUnassembledDependencies_False(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.preAssembleDependsOn = nil

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn))
}

func Test_guard_HasUnassembledDependencies_True(t *testing.T) {
	ctx := context.Background()
	dependencyID := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).Build()
	txn.preAssembleDependsOn = &dependencyID

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))
}
