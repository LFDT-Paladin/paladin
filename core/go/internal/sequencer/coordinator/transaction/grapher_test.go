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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SortTransactions_EmptyInput(t *testing.T) {
	ctx := t.Context()

	sortedTransactions, err := SortTransactions(ctx, []*CoordinatorTransaction{})
	assert.NoError(t, err)
	assert.Len(t, sortedTransactions, 0)

}

func Test_SortTransactions_SingleTransaction(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build(ctx)
	sortedTransactions, err := SortTransactions(ctx, []*CoordinatorTransaction{txn1})
	assert.NoError(t, err)
	require.Len(t, sortedTransactions, 1)
	assert.Equal(t, txn1.pt.ID, sortedTransactions[0].pt.ID)

}

func Test_SortTransactions_SameOrder(t *testing.T) {
	ctx := t.Context()

	grapher := NewGrapher(ctx)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build(ctx)

	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID).
		Build(ctx)

	sortedTransactions, err := SortTransactions(ctx, []*CoordinatorTransaction{txn1, txn2})
	require.NoError(t, err)
	require.Len(t, sortedTransactions, 2)
	assert.Equal(t, txn1.pt.ID, sortedTransactions[0].pt.ID)
	assert.Equal(t, txn2.pt.ID, sortedTransactions[1].pt.ID)

}

func Test_SortTransactions_ReverseOrder(t *testing.T) {
	ctx := t.Context()

	grapher := NewGrapher(ctx)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build(ctx)

	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID).
		Build(ctx)

	//Provide the transactions in reverse order to test sorting
	sortedTransactions, err := SortTransactions(ctx, []*CoordinatorTransaction{txn2, txn1})
	require.NoError(t, err)
	require.Len(t, sortedTransactions, 2)
	assert.Equal(t, txn1.pt.ID, sortedTransactions[0].pt.ID)
	assert.Equal(t, txn2.pt.ID, sortedTransactions[1].pt.ID)

}

func Test_SortTransactions_EndlessLoopPrevention(t *testing.T) {
	ctx := t.Context()

	//When a dependency exists that is not in the input list, it should not cause an endless loop
	// Under normal usage, this should not happen but if it does, we should handle it gracefully with an error

	grapher := NewGrapher(ctx)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build(ctx)

	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID).
		Build(ctx)

	txn3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID).
		Build(ctx)

	//Provide the transactions in reverse order to test sorting
	sortedTransactions, err := SortTransactions(ctx, []*CoordinatorTransaction{txn2, txn3})
	assert.Error(t, err)
	assert.Len(t, sortedTransactions, 0)

}

func Test_SortTransactions_ConfirmedDependency(t *testing.T) {
	ctx := t.Context()

	//When a dependency exists that is not in the input list, but the grapher has forgotten it, then it should not cause an error
	// This is most likely when a dependency exists but has long since been confirmed and removed from the grapher

	grapher := NewGrapher(ctx)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build(ctx)

	err := grapher.Forget(txn1.pt.ID) // Simulate that the grapher has been instructed to forget the transaction as a result of it being confirmed
	assert.NoError(t, err)

	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID).
		Build(ctx)

	txn3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID).
		Build(ctx)

	//Provide the transactions in reverse order to test sorting
	sortedTransactions, err := SortTransactions(ctx, []*CoordinatorTransaction{txn2, txn3})
	require.NoError(t, err)
	require.Len(t, sortedTransactions, 2)
	//Check both transactions are in the sorted transactions but we cannot guarantee the order since neither is dependent on the other
	assert.Condition(t, func() bool {
		for _, txn := range sortedTransactions {
			if txn.pt.ID == txn2.pt.ID {
				return true // txn1 should not be in the sorted transactions
			}
		}
		return false
	}, "txn2 should be in the sorted transactions")
	assert.Condition(t, func() bool {
		for _, txn := range sortedTransactions {
			if txn.pt.ID == txn3.pt.ID {
				return true // txn1 should not be in the sorted transactions
			}
		}
		return false
	}, "txn3 should be in the sorted transactions")

}
func Test_SortTransactions_CircularDependency(t *testing.T) {
	ctx := t.Context()
	// When a circular dependency exists, it should not cause an endless loop

	grapher := NewGrapher(ctx)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build(ctx)

	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID).
		Build(ctx)

	txn3, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID).
		Build(ctx)

	//Provide the transactions in reverse order to test sorting
	sortedTransactions, err := SortTransactions(ctx, []*CoordinatorTransaction{txn2, txn3})
	assert.Error(t, err)
	assert.Len(t, sortedTransactions, 0)

}

func Test_grapher_Add_TransactionByID(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		NumberOfOutputStates(1).
		Build(ctx)

	grapher.Add(ctx, txn)

	lookup := grapher.TransactionByID(ctx, txn.pt.ID)
	require.NotNil(t, lookup)
	assert.Equal(t, txn.pt.ID, lookup.pt.ID)
}

func Test_grapher_Forget_RemovesTransaction(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		NumberOfOutputStates(1).
		Build(ctx)
	grapher.Add(ctx, txn)

	err := grapher.Forget(txn.pt.ID)
	require.NoError(t, err)

	lookup := grapher.TransactionByID(ctx, txn.pt.ID)
	assert.Nil(t, lookup)
}

func Test_grapher_ForgetMints_RemovesMinterLookup(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Build txn with nil grapher so Build() does not register output state; we add it ourselves
	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		NumberOfOutputStates(1).
		Build(ctx)
	stateID := txn.pt.PostAssembly.OutputStates[0].ID

	grapher.Add(ctx, txn)
	err := grapher.AddMinter(ctx, stateID, txn)
	require.NoError(t, err)

	minter, err := grapher.LookupMinter(ctx, stateID)
	require.NoError(t, err)
	assert.Equal(t, txn.pt.ID, minter.pt.ID)

	grapher.ForgetMints(txn.pt.ID)

	minter, err = grapher.LookupMinter(ctx, stateID)
	require.NoError(t, err)
	assert.Nil(t, minter)
}

func Test_grapher_AddMinter_DuplicateMinter(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	// Create two different transactions
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build(ctx)

	txn2, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build(ctx)

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
