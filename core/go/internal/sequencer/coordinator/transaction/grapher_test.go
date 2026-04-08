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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_grapher_Add_TransactionByID(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, txn)

	lookup := grapher.TransactionByID(ctx, txn.pt.ID)
	require.NotNil(t, lookup)
	assert.Equal(t, txn.pt.ID, lookup.GetPrivateTransaction().ID)
}

func Test_grapher_Forget_RemovesTransaction(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, txn)

	err := grapher.Forget(txn.pt.ID)
	require.NoError(t, err)

	lookup := grapher.TransactionByID(ctx, txn.pt.ID)
	assert.Nil(t, lookup)
}

func Test_grapher_ForgetMints_RemovesMinterLookup(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Build txn with nil grapher so Build() does not register output state; we add it ourselves
	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()

	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))

	grapher.Add(ctx, txn)
	err := grapher.AddMinter(ctx, stateID, txn)
	require.NoError(t, err)

	minter, err := grapher.LookupMinter(ctx, stateID)
	require.NoError(t, err)
	assert.Equal(t, txn.pt.ID, minter.GetPrivateTransaction().ID)

	grapher.ForgetMints(txn.pt.ID)

	minter, err = grapher.LookupMinter(ctx, stateID)
	require.NoError(t, err)
	assert.Nil(t, minter)
}

func Test_grapher_AddMinter_DuplicateMinter(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create two different transactions
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()

	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))

	err := grapher.AddMinter(ctx, stateID, txn1)
	require.NoError(t, err)

	minter, err := grapher.LookupMinter(ctx, stateID)
	require.NoError(t, err)
	assert.Equal(t, txn1.pt.ID, minter.pt.ID)

	err = grapher.AddMinter(ctx, stateID, txn2)
	require.Error(t, err)

	expectedMsg := fmt.Sprintf("Duplicate minter. stateID %s already indexed as minted by %s but attempted to add minter %s", stateID.String(), txn1.pt.ID.String(), txn2.pt.ID.String())
	assert.ErrorContains(t, err, expectedMsg)

	assert.Contains(t, err.Error(), msgs.MsgSequencerInternalError)

	minter, err = grapher.LookupMinter(ctx, stateID)
	require.NoError(t, err)
	assert.Equal(t, txn1.pt.ID, minter.pt.ID, "First transaction should still be the minter")
}

func Test_pruneDependencyLinks_NilDependencies(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, txn)

	err := grapher.Forget(txn.pt.ID)
	require.NoError(t, err)

	assert.Nil(t, grapher.TransactionByID(ctx, txn.pt.ID))
}

func Test_pruneDependencyLinks_PrereqOfNotInGrapher(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000001")},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, txn)

	err := grapher.Forget(txn.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn.pt.ID))
}

// When a dependent is finalized before its prerequisite is still in the grapher (chained dispatch),
// DependsOn may list a prereq ID that is no longer indexed — prune must skip updating that prereq.
func Test_pruneDependencyLinks_DependsOnPrereqNotInGrapher(t *testing.T) {
	ctx := context.Background()

	prereqID := uuid.New()
	dependentID := uuid.New()

	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(dependentID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{prereqID},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, dependentTxn)

	err := grapher.Forget(dependentID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, dependentID))
}

func Test_pruneDependencyLinks_DependentHasNilDependencies(t *testing.T) {
	ctx := context.Background()

	tx2ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{tx2ID},
		}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx2ID).
		Build()

	txn2.dependencies = nil

	grapher := NewGrapher(ctx)

	grapher.Add(ctx, txn1)
	grapher.Add(ctx, txn2)

	err := grapher.Forget(txn1.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn1.pt.ID))
	assert.Nil(t, txn2.dependencies)
}

func Test_pruneDependencyLinks_RemovesDependsOnLink(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	tx1ID := uuid.New()
	tx2ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx1ID).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{tx2ID},
		}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx2ID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{tx1ID},
		}).
		Build()

	grapher.Add(ctx, txn1)
	grapher.Add(ctx, txn2)

	err := grapher.Forget(txn1.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn1.pt.ID))
	assert.Empty(t, txn2.dependencies.DependsOn)
}

func Test_pruneDependencyLinks_MultipleDependents(t *testing.T) {
	ctx := context.Background()

	tx1ID := uuid.New()
	tx2ID := uuid.New()
	tx3ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx1ID).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{tx2ID, tx3ID},
		}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx2ID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{tx1ID},
		}).
		Build()
	txn3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx3ID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{tx1ID},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, txn1)
	grapher.Add(ctx, txn2)
	grapher.Add(ctx, txn3)

	err := grapher.Forget(txn1.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn1.pt.ID))
	assert.Empty(t, txn2.dependencies.DependsOn)
	assert.Empty(t, txn3.dependencies.DependsOn)
}

func Test_pruneDependencyLinks_DependsOnRetainsOtherIDs(t *testing.T) {
	ctx := context.Background()

	otherID := uuid.New()
	tx1ID := uuid.New()
	tx2ID := uuid.New()

	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx1ID).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{tx2ID},
		}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx2ID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{tx1ID, otherID},
		}).
		Build()

	grapher := NewGrapher(ctx)

	grapher.Add(ctx, txn1)
	grapher.Add(ctx, txn2)
	err := grapher.Forget(txn1.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn1.pt.ID))
	require.Len(t, txn2.dependencies.DependsOn, 1)
	assert.Equal(t, otherID, txn2.dependencies.DependsOn[0])
}

func Test_pruneDependencyLinks_RemovesSelfFromPrerequisitePrereqOf(t *testing.T) {
	ctx := context.Background()

	txPrereqID := uuid.New()
	txDependentID := uuid.New()

	prereqTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txPrereqID).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{txDependentID},
		}).
		Build()
	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txDependentID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txPrereqID},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, prereqTxn)
	grapher.Add(ctx, dependentTxn)

	err := grapher.Forget(txDependentID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txDependentID))
	assert.Empty(t, prereqTxn.dependencies.PrereqOf)
}

func Test_pruneDependencyLinks_PrereqOfRetainsOtherDependents(t *testing.T) {
	ctx := context.Background()

	txPrereqID := uuid.New()
	txDependentID := uuid.New()
	otherDependentID := uuid.New()

	prereqTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txPrereqID).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{txDependentID, otherDependentID},
		}).
		Build()
	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txDependentID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txPrereqID},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, prereqTxn)
	grapher.Add(ctx, dependentTxn)

	err := grapher.Forget(txDependentID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txDependentID))
	require.Len(t, prereqTxn.dependencies.PrereqOf, 1)
	assert.Equal(t, otherDependentID, prereqTxn.dependencies.PrereqOf[0])
}
