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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAction_RecordRevert(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Initially revertTime should be nil
	assert.Nil(t, txn.revertTime)

	// Call action_recordRevert
	err := action_recordRevert(ctx, txn)
	require.NoError(t, err)

	// Verify revertTime is set
	assert.NotNil(t, txn.revertTime)
	assert.WithinDuration(t, time.Now(), txn.revertTime.Time(), 1*time.Second)
}

func TestAction_InitializeDependencies(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create the dependency transaction first and add it to grapher
	dependencyBuilder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher)
	dependencyTxn := dependencyBuilder.Build()
	dependencyID := dependencyTxn.ID

	// Create a transaction with PreAssembly and dependencies pointing to the dependency transaction
	txnBuilder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependencyID)
	txn := txnBuilder.Build()

	// Verify PreAssembly exists
	require.NotNil(t, txn.PreAssembly)
	require.NotNil(t, txn.PreAssembly.Dependencies)
	require.Len(t, txn.PreAssembly.Dependencies.DependsOn, 1)
	require.Equal(t, dependencyID, txn.PreAssembly.Dependencies.DependsOn[0])

	// Call action_initializeDependencies
	err := action_initializeDependencies(ctx, txn)
	require.NoError(t, err)

	// Verify that the dependency transaction has been updated with this transaction as a dependent
	require.NotNil(t, dependencyTxn.PreAssembly.Dependencies)
	require.Contains(t, dependencyTxn.PreAssembly.Dependencies.PrereqOf, txn.ID)
}

func TestAction_InitializeDependencies_NoPreAssembly(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Remove PreAssembly to test error case
	txn.PreAssembly = nil

	// Call action_initializeDependencies - should return error
	err := action_initializeDependencies(ctx, txn)
	assert.Error(t, err)
}

func TestAction_InitializeDependencies_MissingDependency(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create a transaction with a dependency that doesn't exist in grapher
	unknownDependencyID := uuid.New()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(unknownDependencyID)
	txn := txnBuilder.Build()

	// Call action_initializeDependencies - should not error, just log
	err := action_initializeDependencies(ctx, txn)
	require.NoError(t, err)
}

func TestGuard_HasUnassembledDependencies(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Test 1: No dependencies - should return false
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	// Ensure PreAssembly exists (it should be set by newTransactionForUnitTesting)
	if txn1.PreAssembly == nil {
		txn1.PreAssembly = &components.TransactionPreAssembly{}
	}
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn1))

	// Test 2: Has unassembled previous transaction
	prevTxnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher)
	prevTxn := prevTxnBuilder.Build()

	txn2Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher)
	txn2 := txn2Builder.Build()
	txn2.SetPreviousTransaction(ctx, prevTxn)

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn2))

	// Test 3: Has assembled previous transaction
	prevTxn3Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher)
	prevTxn3 := prevTxn3Builder.Build()

	txn3Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher)
	txn3 := txn3Builder.Build()
	txn3.SetPreviousTransaction(ctx, prevTxn3)

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn3))

	// Test 4: Has unassembled dependency in PreAssembly
	dependencyBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher)
	dependencyTxn := dependencyBuilder.Build()
	dependencyID := dependencyTxn.ID

	txn4Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependencyID)
	txn4 := txn4Builder.Build()

	assert.True(t, guard_HasUnassembledDependencies(ctx, txn4))

	// Test 5: Has assembled dependency in PreAssembly
	dependency5Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher)
	dependency5Txn := dependency5Builder.Build()
	dependency5ID := dependency5Txn.ID

	txn5Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(dependency5ID)
	txn5 := txn5Builder.Build()

	assert.False(t, guard_HasUnassembledDependencies(ctx, txn5))

	// Test 6: Has missing dependency in PreAssembly (dependency not in grapher)
	missingDependencyID := uuid.New()
	txn6Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(missingDependencyID)
	txn6 := txn6Builder.Build()

	// The missing dependency should not cause hasDependenciesNotAssembled to return true
	// because the code assumes it's been confirmed and continues to the next dependency
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn6))

	// Test 7: Has both missing and unassembled dependencies in PreAssembly
	unassembledDependencyBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher)
	unassembledDependencyTxn := unassembledDependencyBuilder.Build()
	unassembledDependencyID := unassembledDependencyTxn.ID

	// Create transaction with both missing and unassembled dependencies
	txn7Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(missingDependencyID, unassembledDependencyID)
	txn7 := txn7Builder.Build()

	// Should return true because one dependency is unassembled (missing one is skipped)
	assert.True(t, guard_HasUnassembledDependencies(ctx, txn7))
}

func TestSetPreviousTransaction(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create a transaction
	txn, _ := newTransactionForUnitTesting(t, grapher)

	// Initially previousTransaction should be nil
	// We can verify this indirectly by checking hasDependenciesNotAssembled behavior
	if txn.PreAssembly == nil {
		txn.PreAssembly = &components.TransactionPreAssembly{}
	}
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn))

	// Create a previous transaction
	prevTxnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher)
	prevTxn := prevTxnBuilder.Build()

	// Set the previous transaction (tests line 31)
	txn.SetPreviousTransaction(ctx, prevTxn)

	// Verify the previous transaction was set by checking behavior
	assert.True(t, guard_HasUnassembledDependencies(ctx, txn))

	// Test setting to nil
	txn.SetPreviousTransaction(ctx, nil)
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn))
}

func TestSetNextTransaction(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create a transaction
	txn, _ := newTransactionForUnitTesting(t, grapher)

	// Initially nextTransaction should be nil
	// We verify this by assembling the transaction - it should not notify any next transaction
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher)
	txnToAssemble := txnBuilder.Build()

	// Assemble without setting nextTransaction - should not error
	err := txnToAssemble.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txnToAssemble.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		PreAssembly:  txnBuilder.BuildPreAssembly(),
		RequestID:    txnToAssemble.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)

	// Create a next transaction
	nextTxnBuilder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher)
	nextTxn := nextTxnBuilder.Build()

	// Set the next transaction (tests line 37)
	txn.SetNextTransaction(ctx, nextTxn)

	// Verify the setter works by assembling a transaction with nextTransaction set
	// This should notify the next transaction without error
	txnToAssemble2 := txnBuilder.Build()
	txnToAssemble2.SetNextTransaction(ctx, nextTxn)

	err = txnToAssemble2.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txnToAssemble2.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		PreAssembly:  txnBuilder.BuildPreAssembly(),
		RequestID:    txnToAssemble2.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)

	// Test setting to nil
	txn.SetNextTransaction(ctx, nil)

	// Verify setting to nil works - assembling should not error
	txnToAssemble3 := txnBuilder.Build()
	txnToAssemble3.SetNextTransaction(ctx, nil)

	err = txnToAssemble3.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txnToAssemble3.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		PreAssembly:  txnBuilder.BuildPreAssembly(),
		RequestID:    txnToAssemble3.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)
}

func TestGuard_HasUnknownDependencies(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Test 1: No dependencies - should return false
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	assert.False(t, guard_HasUnknownDependencies(ctx, txn1))

	// Test 2: Has unknown dependency in PreAssembly
	unknownDependencyID := uuid.New()
	txn2Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(unknownDependencyID)
	txn2 := txn2Builder.Build()

	assert.True(t, guard_HasUnknownDependencies(ctx, txn2))

	// Test 3: Has known dependency in PreAssembly
	knownDependencyBuilder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher)
	knownDependencyTxn := knownDependencyBuilder.Build()
	knownDependencyID := knownDependencyTxn.ID

	txn3Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(knownDependencyID)
	txn3 := txn3Builder.Build()

	assert.False(t, guard_HasUnknownDependencies(ctx, txn3))

	// Test 4: Has unknown dependency in dependencies field
	txn4, _ := newTransactionForUnitTesting(t, grapher)
	unknownID := uuid.New()
	txn4.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{unknownID},
	}

	assert.True(t, guard_HasUnknownDependencies(ctx, txn4))

	// Test 5: Has both PreAssembly and dependencies field with mixed known/unknown
	knownID1 := uuid.New()
	knownTxn1Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher)
	knownTxn1 := knownTxn1Builder.Build()
	knownID1 = knownTxn1.ID

	unknownID2 := uuid.New()

	txn5Builder := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PredefinedDependencies(unknownID2)
	txn5 := txn5Builder.Build()
	txn5.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{knownID1},
	}

	// Should return true because one dependency is unknown
	assert.True(t, guard_HasUnknownDependencies(ctx, txn5))
}

func TestGuard_HasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Test 1: No dependencies - should return false
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	assert.False(t, guard_HasDependenciesNotReady(ctx, txn1))

	// Test 2: Has dependency not ready
	dep1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	dep1 := dep1Builder.Build()

	txn2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(dep1.PostAssembly.OutputStates[0].ID)
	txn2 := txn2Builder.Build()

	err := txn2.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn2.ID,
		},
		PostAssembly: txn2Builder.BuildPostAssembly(),
		PreAssembly:  txn2Builder.BuildPreAssembly(),
		RequestID:    txn2.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)

	assert.True(t, guard_HasDependenciesNotReady(ctx, txn2))

	// Test 3: Has dependency ready for dispatch
	dep3Builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(3)
	dep3 := dep3Builder.Build()

	txn3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(dep3.PostAssembly.OutputStates[0].ID)
	txn3 := txn3Builder.Build()

	err = txn3.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn3.ID,
		},
		PostAssembly: txn3Builder.BuildPostAssembly(),
		PreAssembly:  txn3Builder.BuildPreAssembly(),
		RequestID:    txn3.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)

	assert.False(t, guard_HasDependenciesNotReady(ctx, txn3))
}

func TestGuard_HasChainedTxInProgress(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Test 1: Initially false
	assert.False(t, guard_HasChainedTxInProgress(ctx, txn))
	assert.False(t, txn.chainedTxAlreadyDispatched)

	// Test 2: After setting chained transaction in progress
	txn.SetChainedTxInProgress()
	assert.True(t, guard_HasChainedTxInProgress(ctx, txn))
	assert.True(t, txn.chainedTxAlreadyDispatched)
}
