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

	// Create the dependency transaction first and add it to grapher
	dependencyID := uuid.New()
	dependencyTxn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		TransactionID(dependencyID).
		Build()

	// Create a transaction with PreAssembly and dependencies pointing to the dependency transaction
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependencyID).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		Build()

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()
	// Verify PreAssembly exists
	require.NotNil(t, txn.pt.PreAssembly)
	require.NotNil(t, txn.pt.PreAssembly.Dependencies)
	require.Len(t, txn.pt.PreAssembly.Dependencies.DependsOn, 1)
	require.Equal(t, dependencyID, txn.pt.PreAssembly.Dependencies.DependsOn[0])

	// Call action_InitializeForNewAssembly
	err := action_InitializeForNewAssembly(ctx, txn, nil)
	require.NoError(t, err)

	// Verify that the dependency transaction has been updated with this transaction as a dependent
	require.NotNil(t, dependencyTxn.pt.PreAssembly.Dependencies)
	require.Contains(t, dependencyTxn.pt.PreAssembly.Dependencies.PrereqOf, txn.pt.ID)
	require.Nil(t, txn.pt.PreparedPublicTransaction)
	require.Nil(t, txn.pt.PreparedPrivateTransaction)
}

func Test_action_InitializeForNewAssembly_NoPreAssembly(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	// Remove PreAssembly to test error case
	txn.pt.PreAssembly = nil

	// Call action_InitializeForNewAssembly - should return error
	err := action_InitializeForNewAssembly(ctx, txn, nil)
	assert.Error(t, err)
}

func Test_action_InitializeForNewAssembly_MissingDependency(t *testing.T) {
	ctx := context.Background()

	// Create a transaction with a dependency that doesn't exist in grapher
	unknownDependencyID := uuid.New()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		PredefinedDependencies(unknownDependencyID).
		Build()

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()
	// Call action_InitializeForNewAssembly - should not error, just log
	err := action_InitializeForNewAssembly(ctx, txn, nil)
	require.NoError(t, err)
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
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{dep2.pt.ID},
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
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{dependentID},
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
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{},
		}).
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
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{dependentID},
		}).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := txn.notifyDependentsOfReset(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfRepool_DependentNotFound(t *testing.T) {
	ctx := context.Background()
	missingID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{missingID},
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

func Test_notifyDependentsOfRepool_HandleEventReturnsError(t *testing.T) {
	ctx := context.Background()
	mockGrapher := NewMockGrapher(t)
	dependentID := uuid.New()

	// Set up mock expectations for grapher operations used during transaction creation
	mockGrapher.EXPECT().Add(mock.Anything, mock.Anything).Return().Maybe()
	mockGrapher.EXPECT().ForgetMints(mock.Anything).Return().Maybe()

	// Create a mock dependent transaction that returns an error from HandleEvent
	mockDependentTxn := NewMockCoordinatorTransaction(t)
	expectedError := errors.New("handle event error")
	mockDependentTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DependencyRepooledEvent")).Return(expectedError)

	// Configure mock grapher to return the mock dependent transaction
	mockGrapher.EXPECT().TransactionByID(ctx, dependentID).Return(mockDependentTxn)

	// Create main transaction with the mock grapher and a dependent
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(mockGrapher).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{dependentID},
		}).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	// txn2.stateMachine.CurrentState = State_Blocked
	txn2.pt.PreAssembly = nil // This will cause action_initializeDependencies to fail when transitioning to State_Pooled

	// Call notifyDependentsOfRevert - it should return the error from HandleEvent
	err := txn1.notifyDependentsOfReset(ctx)
	assert.Error(t, err)
	// Verify the error is returned (the error will be from action_initializeDependencies failing)
	assert.NotNil(t, err)
}
