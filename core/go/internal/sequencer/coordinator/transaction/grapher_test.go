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

	err := grapher.Forget(ctx, txn.pt.ID)
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

func Test_pruneDependencyLinks_PrereqOfNotInGrapher(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000001")},
			},
			Chained: ChainedDependencies{
				PrereqOf: []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000002")},
			},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, txn)

	err := grapher.Forget(ctx, txn.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn.pt.ID))
}

// DependsOn may list a prereq ID that is no longer indexed — prune must skip updating that prereq.
func Test_pruneDependencyLinks_PrereqsNotInGrapher(t *testing.T) {
	ctx := context.Background()

	postAssemblePrereqID := uuid.New()
	externalPrereqID := uuid.New()
	dependentID := uuid.New()

	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(dependentID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				DependsOn: []uuid.UUID{postAssemblePrereqID},
			},
			Chained: ChainedDependencies{
				PrereqOf: []uuid.UUID{externalPrereqID},
			},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, dependentTxn)

	err := grapher.Forget(ctx, dependentID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, dependentID))
}

func Test_pruneDependencyLinks_DependentHasEmptyDependencies(t *testing.T) {
	ctx := context.Background()

	tx2ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{tx2ID},
			},
		}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx2ID).
		Build()

	grapher := NewGrapher(ctx)

	grapher.Add(ctx, txn1)
	grapher.Add(ctx, txn2)

	err := grapher.Forget(ctx, txn1.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn1.pt.ID))
}

func Test_pruneDependencyLinks_RemovesDependsOnLinks(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	tx1ID := uuid.New()
	tx2ID := uuid.New()
	tx3ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx1ID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{tx2ID},
			},
			Chained: ChainedDependencies{
				PrereqOf: []uuid.UUID{tx3ID},
			},
		}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx2ID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				DependsOn: []uuid.UUID{tx1ID},
			},
		}).
		Build()
	txn3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx3ID).
		Dependencies(&TransactionDependencies{
			Chained: ChainedDependencies{
				DependsOn: []uuid.UUID{tx1ID},
			},
		}).
		Build()

	grapher.Add(ctx, txn1)
	grapher.Add(ctx, txn2)
	grapher.Add(ctx, txn3)

	err := grapher.Forget(ctx, txn1.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn1.pt.ID))
	assert.Empty(t, txn2.dependencies.PostAssemble.DependsOn)
	assert.Empty(t, txn3.dependencies.Chained.DependsOn)
}

func Test_pruneDependencyLinks_MultipleDependents(t *testing.T) {
	ctx := context.Background()

	tx1ID := uuid.New()
	tx2ID := uuid.New()
	tx3ID := uuid.New()
	tx4ID := uuid.New()
	tx5ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx1ID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{tx2ID, tx3ID},
			},
			Chained: ChainedDependencies{
				PrereqOf: []uuid.UUID{tx4ID, tx5ID},
			},
		}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx2ID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				DependsOn: []uuid.UUID{tx1ID},
			},
		}).
		Build()
	txn3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx3ID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				DependsOn: []uuid.UUID{tx1ID},
			},
		}).
		Build()
	txn4, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx4ID).
		Dependencies(&TransactionDependencies{
			Chained: ChainedDependencies{
				DependsOn: []uuid.UUID{tx1ID},
			},
		}).
		Build()
	txn5, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx5ID).
		Dependencies(&TransactionDependencies{
			Chained: ChainedDependencies{
				DependsOn: []uuid.UUID{tx1ID},
			},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, txn1)
	grapher.Add(ctx, txn2)
	grapher.Add(ctx, txn3)
	grapher.Add(ctx, txn4)
	grapher.Add(ctx, txn5)

	err := grapher.Forget(ctx, txn1.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn1.pt.ID))
	assert.Empty(t, txn2.dependencies.PostAssemble.DependsOn)
	assert.Empty(t, txn3.dependencies.PostAssemble.DependsOn)
	assert.Empty(t, txn4.dependencies.Chained.DependsOn)
	assert.Empty(t, txn5.dependencies.Chained.DependsOn)
}

func Test_pruneDependencyLinks_DependsOnRetainsOtherIDs(t *testing.T) {
	ctx := context.Background()

	otherID := uuid.New()
	tx1ID := uuid.New()
	tx2ID := uuid.New()
	tx3ID := uuid.New()

	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx1ID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{tx2ID},
			},
			Chained: ChainedDependencies{
				PrereqOf: []uuid.UUID{tx3ID},
			},
		}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx2ID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				DependsOn: []uuid.UUID{tx1ID, otherID},
			},
		}).
		Build()
	txn3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx3ID).
		Dependencies(&TransactionDependencies{
			Chained: ChainedDependencies{
				DependsOn: []uuid.UUID{tx1ID, otherID},
			},
		}).
		Build()

	grapher := NewGrapher(ctx)

	grapher.Add(ctx, txn1)
	grapher.Add(ctx, txn2)
	grapher.Add(ctx, txn3)
	err := grapher.Forget(ctx, txn1.pt.ID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txn1.pt.ID))
	require.Len(t, txn2.dependencies.PostAssemble.DependsOn, 1)
	assert.Equal(t, otherID, txn2.dependencies.PostAssemble.DependsOn[0])
	require.Len(t, txn3.dependencies.Chained.DependsOn, 1)
	assert.Equal(t, otherID, txn3.dependencies.Chained.DependsOn[0])
}

func Test_pruneDependencyLinks_RemovesSelfFromPrerequisitePrereqOf(t *testing.T) {
	ctx := context.Background()

	txPrereqID := uuid.New()
	txPostAssembleDependentID := uuid.New()
	txExternalDependentID := uuid.New()

	prereqTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txPrereqID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{txPostAssembleDependentID},
			},
			Chained: ChainedDependencies{
				PrereqOf: []uuid.UUID{txExternalDependentID},
			},
		}).
		Build()
	postAssembleDependentTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txPostAssembleDependentID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				DependsOn: []uuid.UUID{txPrereqID},
			},
		}).
		Build()

	externalDependentTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txExternalDependentID).
		Dependencies(&TransactionDependencies{
			Chained: ChainedDependencies{
				DependsOn: []uuid.UUID{txPrereqID},
			},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, prereqTxn)
	grapher.Add(ctx, postAssembleDependentTxn)
	grapher.Add(ctx, externalDependentTxn)

	err := grapher.Forget(ctx, txPostAssembleDependentID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txPostAssembleDependentID))
	assert.Empty(t, prereqTxn.dependencies.PostAssemble.PrereqOf)

	err = grapher.Forget(ctx, txExternalDependentID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txExternalDependentID))
	assert.Empty(t, prereqTxn.dependencies.Chained.PrereqOf)
}

func Test_pruneDependencyLinks_PrereqOfRetainsOtherDependents(t *testing.T) {
	ctx := context.Background()

	txPrereqID := uuid.New()
	txPostAssembleDependentID := uuid.New()
	txExternalDependentID := uuid.New()
	otherDependentID := uuid.New()

	prereqTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txPrereqID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{txPostAssembleDependentID, otherDependentID},
			},
			Chained: ChainedDependencies{
				PrereqOf: []uuid.UUID{txExternalDependentID, otherDependentID},
			},
		}).
		Build()
	postAssembleDependentTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txPostAssembleDependentID).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				DependsOn: []uuid.UUID{txPrereqID},
			},
		}).
		Build()
	externalDependentTxn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(txExternalDependentID).
		Dependencies(&TransactionDependencies{
			Chained: ChainedDependencies{
				DependsOn: []uuid.UUID{txPrereqID},
			},
		}).
		Build()

	grapher := NewGrapher(ctx)
	grapher.Add(ctx, prereqTxn)
	grapher.Add(ctx, postAssembleDependentTxn)
	grapher.Add(ctx, externalDependentTxn)

	err := grapher.Forget(ctx, txPostAssembleDependentID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txPostAssembleDependentID))
	require.Len(t, prereqTxn.dependencies.PostAssemble.PrereqOf, 1)
	assert.Equal(t, otherDependentID, prereqTxn.dependencies.PostAssemble.PrereqOf[0])

	err = grapher.Forget(ctx, txExternalDependentID)
	require.NoError(t, err)
	assert.Nil(t, grapher.TransactionByID(ctx, txExternalDependentID))
	require.Len(t, prereqTxn.dependencies.Chained.PrereqOf, 1)
	assert.Equal(t, otherDependentID, prereqTxn.dependencies.Chained.PrereqOf[0])
}
