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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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

	mocks.EngineIntegration.EXPECT().ResetTransactions(ctx, txn.pt.ID).Return()
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

	mocks.EngineIntegration.EXPECT().ResetTransactions(ctx, txn.pt.ID).Return()
	// Call action_InitializeForNewAssembly - should not error, just log
	err := action_InitializeForNewAssembly(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_InitializeForNewAssembly_DependencyWithNilDependencies(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create the dependency transaction first and add it to grapher
	// Make sure it has PreAssembly but PreAssembly.Dependencies is nil
	dependencyID := uuid.New()
	dependencyTxn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		TransactionID(dependencyID).
		Build()

	// Explicitly set PreAssembly.Dependencies to nil to test the nil check path
	dependencyTxn.pt.PreAssembly.Dependencies = nil

	// Create a transaction with PreAssembly and dependencies pointing to the dependency transaction
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependencyID).
		Build()

	mocks.EngineIntegration.EXPECT().ResetTransactions(ctx, txn.pt.ID).Return()

	// Verify PreAssembly exists
	require.NotNil(t, txn.pt.PreAssembly)
	require.NotNil(t, txn.pt.PreAssembly.Dependencies)
	require.Len(t, txn.pt.PreAssembly.Dependencies.DependsOn, 1)
	require.Equal(t, dependencyID, txn.pt.PreAssembly.Dependencies.DependsOn[0])

	// Verify dependency has nil Dependencies
	require.Nil(t, dependencyTxn.pt.PreAssembly.Dependencies)

	// Call action_InitializeForNewAssembly
	err := action_InitializeForNewAssembly(ctx, txn, nil)
	require.NoError(t, err)

	// Verify that the dependency transaction now has Dependencies initialized
	require.NotNil(t, dependencyTxn.pt.PreAssembly.Dependencies)
	require.Contains(t, dependencyTxn.pt.PreAssembly.Dependencies.PrereqOf, txn.pt.ID)
}

func Test_guard_HasUnassembledDependencies(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Test 1: No dependencies - should return false
	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn1))

	// Test 2: Has unassembled dependency in PreAssembly
	dependencyID := uuid.New()
	_, _ = NewTransactionBuilderForTesting(t, State_Assembling).
		TransactionID(dependencyID).
		Grapher(grapher).
		Build()

	txn2, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependencyID).
		Build()
	assert.True(t, guard_HasUnassembledDependencies(ctx, txn2))

	// Test 3: Has assembled dependency in PreAssembly
	dependency3ID := uuid.New()
	_, _ = NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		TransactionID(dependency3ID).
		Grapher(grapher).
		Build()

	txn3, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependency3ID).
		Build()
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn3))

	// Test 4: Has missing dependency in PreAssembly (dependency not in grapher)
	missingDependencyID := uuid.New()
	txn4, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(missingDependencyID).
		Build()
	// The missing dependency should not cause hasDependenciesNotAssembled to return true
	// because the code assumes it's been confirmed and continues to the next dependency
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn4))

	// Test 5: Has both missing and unassembled dependencies in PreAssembly
	unassembledDependencyID := uuid.New()
	_, _ = NewTransactionBuilderForTesting(t, State_Assembling).
		TransactionID(unassembledDependencyID).
		Grapher(grapher).
		Build()

	// Create transaction with both missing and unassembled dependencies
	txn5, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(missingDependencyID, unassembledDependencyID).
		Build()
	// Should return true because one dependency is unassembled (missing one is skipped)
	assert.True(t, guard_HasUnassembledDependencies(ctx, txn5))
}

func Test_guard_HasUnknownDependencies(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Test 1: No dependencies - should return false
	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Build()
	assert.False(t, guard_HasUnknownDependencies(ctx, txn1))

	// Test 2: Has unknown dependency in PreAssembly
	unknownDependencyID := uuid.New()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(unknownDependencyID).
		Build()
	assert.True(t, guard_HasUnknownDependencies(ctx, txn2))

	// Test 3: Has known dependency in PreAssembly
	knownDependencyID := uuid.New()
	_, _ = NewTransactionBuilderForTesting(t, State_Initial).
		TransactionID(knownDependencyID).
		Grapher(grapher).
		Build()

	txn3, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(knownDependencyID).
		Build()
	assert.False(t, guard_HasUnknownDependencies(ctx, txn3))

	// Test 4: Has unknown dependency in dependencies field
	unknownID := uuid.New()
	txn4, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{unknownID},
		}).
		Build()

	assert.True(t, guard_HasUnknownDependencies(ctx, txn4))

	// Test 5: Has both PreAssembly and dependencies field with mixed known/unknown
	knownID1 := uuid.New()
	_, _ = NewTransactionBuilderForTesting(t, State_Initial).
		TransactionID(knownID1).
		Grapher(grapher).
		Build()

	unknownID2 := uuid.New()

	txn5, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(unknownID2).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{knownID1},
		}).
		Build()

	// Should return true because one dependency is unknown
	assert.True(t, guard_HasUnknownDependencies(ctx, txn5))
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

func Test_guard_HasChainedTxInProgress(t *testing.T) {
	ctx := context.Background()

	// Test 1: Initially false (hasChainedTransaction=false passed to NewTransaction via test utils)
	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	assert.False(t, guard_HasChainedTxInProgress(ctx, txn1))

	// Test 2: When chainedTxAlreadyDispatched is true
	txn2, _ := NewTransactionBuilderForTesting(t, State_Initial).
		ChainedTxAlreadyDispatched(true).
		Build()
	assert.True(t, guard_HasChainedTxInProgress(ctx, txn2))
}

func Test_action_NotifyDependentsOfRepool_WithDependents(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create a dependent transaction
	dependentID := uuid.New()
	dependentTxn, dependentMocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		TransactionID(dependentID).
		Grapher(grapher).
		Build()
	dependentMocks.EngineIntegration.EXPECT().ResetTransactions(ctx, dependentID).Return()

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
	err := action_NotifyDependentsOfRepool(ctx, mainTxn, nil)
	require.NoError(t, err)

	// Verify the dependent transaction received the event
	assert.Equal(t, State_Pooled, dependentTxn.stateMachine.GetCurrentState())
}

func Test_action_NotifyDependentsOfRepool_InitialTransitionHasNoDependents(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	err := action_NotifyDependentsOfRepool(ctx, txn, nil)
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

	err := txn.notifyDependentsOfRepool(ctx)
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

	err := txn.notifyDependentsOfRepool(ctx)
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

	err := txn.notifyDependentsOfRepool(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfRepool_WithDependent_HandleEventError(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	tx2ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{tx2ID},
		}).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).
		TransactionID(tx2ID).
		Build()

	// txn2.stateMachine.CurrentState = State_Blocked
	txn2.pt.PreAssembly = nil // This will cause action_initializeDependencies to fail when transitioning to State_Pooled

	// Call notifyDependentsOfRevert - it should return the error from HandleEvent
	err := txn1.notifyDependentsOfRepool(ctx)
	assert.Error(t, err)
	// Verify the error is returned (the error will be from action_initializeDependencies failing)
	assert.NotNil(t, err)
}
