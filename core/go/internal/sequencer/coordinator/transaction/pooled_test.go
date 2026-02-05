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
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_recordRevert_Success(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(ctx)

	// Initially revertTime should be nil
	assert.Nil(t, txn.revertTime)

	// Call action_recordRevert
	err := action_recordRevert(ctx, txn, nil)
	require.NoError(t, err)

	// Verify revertTime is set
	assert.NotNil(t, txn.revertTime)
	assert.WithinDuration(t, time.Now(), txn.revertTime.Time(), 1*time.Second)
}

func Test_action_initializeDependencies_Success(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Create the dependency transaction first and add it to grapher
	dependencyTxn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		Build(ctx)
	dependencyID := dependencyTxn.pt.ID

	// Create a transaction with PreAssembly and dependencies pointing to the dependency transaction
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependencyID).
		Build(ctx)

	// Verify PreAssembly exists
	require.NotNil(t, txn.pt.PreAssembly)
	require.NotNil(t, txn.pt.PreAssembly.Dependencies)
	require.Len(t, txn.pt.PreAssembly.Dependencies.DependsOn, 1)
	require.Equal(t, dependencyID, txn.pt.PreAssembly.Dependencies.DependsOn[0])

	// Call action_initializeDependencies
	err := action_initializeDependencies(ctx, txn, nil)
	require.NoError(t, err)

	// Verify that the dependency transaction has been updated with this transaction as a dependent
	require.NotNil(t, dependencyTxn.pt.PreAssembly.Dependencies)
	require.Contains(t, dependencyTxn.pt.PreAssembly.Dependencies.PrereqOf, txn.pt.ID)
}

func Test_action_initializeDependencies_NoPreAssembly(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(ctx)

	// Remove PreAssembly to test error case
	txn.pt.PreAssembly = nil

	// Call action_initializeDependencies - should return error
	err := action_initializeDependencies(ctx, txn, nil)
	assert.Error(t, err)
}

func Test_action_initializeDependencies_MissingDependency(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Create a transaction with a dependency that doesn't exist in grapher
	unknownDependencyID := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(unknownDependencyID).
		Build(ctx)

	// Call action_initializeDependencies - should not error, just log
	err := action_initializeDependencies(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_initializeDependencies_DependencyWithNilDependencies(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Create the dependency transaction first and add it to grapher
	// Make sure it has PreAssembly but PreAssembly.Dependencies is nil
	dependencyTxn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		Build(ctx)
	dependencyID := dependencyTxn.pt.ID

	// Explicitly set PreAssembly.Dependencies to nil to test the nil check path
	dependencyTxn.pt.PreAssembly.Dependencies = nil

	// Create a transaction with PreAssembly and dependencies pointing to the dependency transaction
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependencyID).
		Build(ctx)

	// Verify PreAssembly exists
	require.NotNil(t, txn.pt.PreAssembly)
	require.NotNil(t, txn.pt.PreAssembly.Dependencies)
	require.Len(t, txn.pt.PreAssembly.Dependencies.DependsOn, 1)
	require.Equal(t, dependencyID, txn.pt.PreAssembly.Dependencies.DependsOn[0])

	// Verify dependency has nil Dependencies
	require.Nil(t, dependencyTxn.pt.PreAssembly.Dependencies)

	// Call action_initializeDependencies
	err := action_initializeDependencies(ctx, txn, nil)
	require.NoError(t, err)

	// Verify that the dependency transaction now has Dependencies initialized
	require.NotNil(t, dependencyTxn.pt.PreAssembly.Dependencies)
	require.Contains(t, dependencyTxn.pt.PreAssembly.Dependencies.PrereqOf, txn.pt.ID)
}

func Test_guard_HasUnassembledDependencies(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Test 1: No dependencies - should return false
	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build(ctx)
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn1))

	// Test 2: Has unassembled dependency in PreAssembly
	dependencyTxn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build(ctx)
	dependencyID := dependencyTxn.pt.ID

	txn2, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependencyID).
		Build(ctx)

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn2))

	// Test 3: Has assembled dependency in PreAssembly
	dependency3Txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		Build(ctx)
	dependency3ID := dependency3Txn.pt.ID

	txn3, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependency3ID).
		Build(ctx)

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn3))

	// Test 4: Has missing dependency in PreAssembly (dependency not in grapher)
	missingDependencyID := uuid.New()
	txn4, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(missingDependencyID).
		Build(ctx)

	// The missing dependency should not cause hasDependenciesNotAssembled to return true
	// because the code assumes it's been confirmed and continues to the next dependency
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn4))

	// Test 5: Has both missing and unassembled dependencies in PreAssembly
	unassembledDependencyTxn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build(ctx)
	unassembledDependencyID := unassembledDependencyTxn.pt.ID

	// Create transaction with both missing and unassembled dependencies
	txn5, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(missingDependencyID, unassembledDependencyID).
		Build(ctx)

	// Should return true because one dependency is unassembled (missing one is skipped)
	assert.True(t, guard_HasUnassembledDependencies(ctx, txn5))
}

func Test_guard_HasUnknownDependencies(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Test 1: No dependencies - should return false
	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build(ctx)
	assert.False(t, guard_HasUnknownDependencies(ctx, txn1))

	// Test 2: Has unknown dependency in PreAssembly
	unknownDependencyID := uuid.New()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(unknownDependencyID).
		Build(ctx)

	assert.True(t, guard_HasUnknownDependencies(ctx, txn2))

	// Test 3: Has known dependency in PreAssembly
	knownDependencyTxn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build(ctx)
	knownDependencyID := knownDependencyTxn.pt.ID

	txn3, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(knownDependencyID).
		Build(ctx)

	assert.False(t, guard_HasUnknownDependencies(ctx, txn3))

	// Test 4: Has unknown dependency in dependencies field
	txn4, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build(ctx)
	unknownID := uuid.New()
	txn4.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{unknownID},
	}

	assert.True(t, guard_HasUnknownDependencies(ctx, txn4))

	// Test 5: Has both PreAssembly and dependencies field with mixed known/unknown
	knownID1 := uuid.New()
	knownTxn1, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build(ctx)
	knownID1 = knownTxn1.pt.ID

	unknownID2 := uuid.New()

	txn5, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(unknownID2).
		Build(ctx)
	txn5.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{knownID1},
	}

	// Should return true because one dependency is unknown
	assert.True(t, guard_HasUnknownDependencies(ctx, txn5))
}

func Test_guard_HasDependenciesNotReady(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Test 1: No dependencies - should return false
	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build(ctx)
	assert.False(t, guard_HasDependenciesNotReady(ctx, txn1))

	// Test 2: Has dependency not ready
	dep1, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build(ctx)

	txn2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(dep1.pt.PostAssembly.OutputStates[0].ID).
		AddPendingAssemblyRequest()

	txn2, mocks2 := txn2Builder.Build(ctx)
	mocks2.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, mock.Anything).Return(nil)

	err := txn2.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn2.pt.ID,
		},
		PostAssembly: txn2Builder.BuildPostAssembly(),
		PreAssembly:  txn2Builder.BuildPreAssembly(),
		RequestID:    txn2.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)

	assert.True(t, guard_HasDependenciesNotReady(ctx, txn2))

	// Test 3: Has dependency ready for dispatch
	dep3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(3).
		Build(ctx)
	dep3.dynamicSigningIdentity = false

	txn3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(dep3.pt.PostAssembly.OutputStates[0].ID).
		AddPendingAssemblyRequest()

	txn3, mocks3 := txn3Builder.Build(ctx)
	mocks3.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, mock.Anything).Return(nil)

	err = txn3.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn3.pt.ID,
		},
		PostAssembly: txn3Builder.BuildPostAssembly(),
		PreAssembly:  txn3Builder.BuildPreAssembly(),
		RequestID:    txn3.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)

	assert.False(t, guard_HasDependenciesNotReady(ctx, txn3))
}

func Test_guard_HasChainedTxInProgress(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(ctx)

	// Test 1: Initially false (hasChainedTransaction=false passed to NewTransaction via test utils)
	assert.False(t, guard_HasChainedTxInProgress(ctx, txn))
	assert.False(t, txn.chainedTxAlreadyDispatched)

	// Test 2: When chainedTxAlreadyDispatched is true
	txn.chainedTxAlreadyDispatched = true
	assert.True(t, guard_HasChainedTxInProgress(ctx, txn))
}

func Test_rePoolDependents_EmptyPrereqOf(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build(ctx)

	// Ensure dependencies is initialized and PrereqOf is empty
	require.NotNil(t, txn.dependencies)
	require.Empty(t, txn.dependencies.PrereqOf)

	// Should return nil without error when there are no dependents
	err := txn.rePoolDependents(ctx)
	require.NoError(t, err)
}

func Test_rePoolDependents_WithDependents_Success(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Create the main transaction that will have dependents
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build(ctx)
	mainTxn.initializeStateMachine(State_Initial)

	// Create a dependent transaction that can handle DependencyRevertedEvent
	// It needs to be in State_Submitted to handle DependencyRevertedEvent
	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Submitted).
		Grapher(grapher).
		Build(ctx)
	dependentID := dependentTxn.pt.ID

	// Set up the main transaction to have the dependent as a PrereqOf
	mainTxn.dependencies.PrereqOf = []uuid.UUID{dependentID}

	// Call rePoolDependents - should successfully notify the dependent
	err := mainTxn.rePoolDependents(ctx)
	require.NoError(t, err)

	// Verify the dependent transaction received the event by checking it transitioned to State_Pooled
	assert.Equal(t, State_Pooled, dependentTxn.stateMachine.CurrentState)
}

func Test_rePoolDependents_WithDependents_MissingInGrapher(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Create the main transaction
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build(ctx)

	// Set up PrereqOf with an ID that doesn't exist in grapher
	missingDependentID := uuid.New()
	mainTxn.dependencies.PrereqOf = []uuid.UUID{missingDependentID}

	// Should return nil without error even when dependent is not found
	err := mainTxn.rePoolDependents(ctx)
	require.NoError(t, err)
}

func Test_rePoolDependents_WithMultipleDependents(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Create the main transaction
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build(ctx)
	mainTxn.initializeStateMachine(State_Initial)

	// Create multiple dependent transactions
	dependent1, _ := NewTransactionBuilderForTesting(t, State_Submitted).
		Grapher(grapher).
		Build(ctx)

	dependent2, _ := NewTransactionBuilderForTesting(t, State_Submitted).
		Grapher(grapher).
		Build(ctx)

	// Create one that doesn't exist in grapher
	missingDependentID := uuid.New()

	// Set up the main transaction to have multiple dependents
	mainTxn.dependencies.PrereqOf = []uuid.UUID{
		dependent1.pt.ID,
		dependent2.pt.ID,
		missingDependentID,
	}

	// Call rePoolDependents - should handle all dependents
	err := mainTxn.rePoolDependents(ctx)
	require.NoError(t, err)

	// Verify both existing dependents received the event
	assert.Equal(t, State_Pooled, dependent1.stateMachine.CurrentState)
	assert.Equal(t, State_Pooled, dependent2.stateMachine.CurrentState)
}

func Test_action_recordRevert_WithDependents(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Create the main transaction
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build(ctx)
	mainTxn.initializeStateMachine(State_Initial)

	// Create a dependent transaction
	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Submitted).
		Grapher(grapher).
		Build(ctx)
	dependentID := dependentTxn.pt.ID

	// Set up the main transaction to have the dependent as a PrereqOf
	mainTxn.dependencies.PrereqOf = []uuid.UUID{dependentID}

	// Initially revertTime should be nil
	assert.Nil(t, mainTxn.revertTime)

	// Call action_recordRevert - should re-pool dependents and set revertTime
	err := action_recordRevert(ctx, mainTxn, nil)
	require.NoError(t, err)

	// Verify revertTime is set
	assert.NotNil(t, mainTxn.revertTime)
	assert.WithinDuration(t, time.Now(), mainTxn.revertTime.Time(), 1*time.Second)

	// Verify the dependent transaction received the event
	assert.Equal(t, State_Pooled, dependentTxn.stateMachine.CurrentState)
}

func Test_action_recordRevert_WithDependents_ErrorHandling(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Create the main transaction
	mainTxn, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build(ctx)
	mainTxn.initializeStateMachine(State_Initial)

	// Create a dependent transaction that will fail when handling DependencyRevertedEvent
	// This happens when transitioning to State_Pooled triggers action_initializeDependencies
	// which fails if PreAssembly is nil
	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Submitted).
		Grapher(grapher).
		Build(ctx)
	dependentID := dependentTxn.pt.ID

	// Remove PreAssembly to cause action_initializeDependencies to fail
	// when the transaction transitions to State_Pooled
	dependentTxn.pt.PreAssembly = nil

	// Set up the main transaction to have the dependent as a PrereqOf
	mainTxn.dependencies.PrereqOf = []uuid.UUID{dependentID}

	// Initially revertTime should be nil
	assert.Nil(t, mainTxn.revertTime)

	// Call action_recordRevert - should log error but continue and set revertTime
	err := action_recordRevert(ctx, mainTxn, nil)
	require.NoError(t, err) // action_recordRevert always returns nil, even if rePoolDependents fails

	// Verify revertTime is set despite the error
	assert.NotNil(t, mainTxn.revertTime)
	assert.WithinDuration(t, time.Now(), mainTxn.revertTime.Time(), 1*time.Second)

	// Verify the dependent transaction transitioned to State_Pooled before the error occurred
	// The state is changed before OnTransitionTo is called, so even though action_initializeDependencies
	// failed, the state was already changed to State_Pooled
	assert.Equal(t, State_Pooled, dependentTxn.stateMachine.CurrentState)
}
