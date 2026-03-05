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
	"github.com/stretchr/testify/require"
)

func Test_action_ReinitializeForNewAssembly_Success(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create a transaction with PreAssembly and dependencies pointing to the dependency transaction
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		Build()

	mocks.EngineIntegration.EXPECT().ResetTransactions(ctx, txn.pt.ID).Return()

	err := action_InitializeForNewAssembly(ctx, txn, nil)
	require.NoError(t, err)

	require.Nil(t, txn.pt.PreparedPublicTransaction)
	require.Nil(t, txn.pt.PreparedPrivateTransaction)
}

func Test_guard_HasUnassembledDependencies(t *testing.T) {
	ctx := context.Background()

	// Test 1: No dependencies - should return false
	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	assert.False(t, guard_HasUnassembledDependencies(ctx, txn1))

	// Test 2: Has unassembled dependency in PreAssembly
	dependencyID := uuid.New()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PreAssembleDependsOn(&dependencyID).
		Build()
	assert.True(t, guard_HasUnassembledDependencies(ctx, txn2))
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
