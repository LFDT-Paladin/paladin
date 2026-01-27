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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SortTransactions_EmptyInput(t *testing.T) {
	ctx := context.Background()

	sortedTransactions, err := SortTransactions(ctx, []*Transaction{})
	assert.NoError(t, err)
	assert.Len(t, sortedTransactions, 0)

}

func Test_SortTransactions_SingleTransaction(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(context.Background())

	txnBuilder1 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1)
	txn1 := txnBuilder1.Build()
	sortedTransactions, err := SortTransactions(ctx, []*Transaction{txn1})
	assert.NoError(t, err)
	require.Len(t, sortedTransactions, 1)
	assert.Equal(t, txn1.pt.ID, sortedTransactions[0].pt.ID)

}

func Test_SortTransactions_SameOrder(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(context.Background())

	txnBuilder1 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1)
	txn1 := txnBuilder1.Build()

	txnBuilder2 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID)
	txn2 := txnBuilder2.Build()

	sortedTransactions, err := SortTransactions(ctx, []*Transaction{txn1, txn2})
	require.NoError(t, err)
	require.Len(t, sortedTransactions, 2)
	assert.Equal(t, txn1.pt.ID, sortedTransactions[0].pt.ID)
	assert.Equal(t, txn2.pt.ID, sortedTransactions[1].pt.ID)

}

func Test_SortTransactions_ReverseOrder(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(context.Background())

	txnBuilder1 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1)
	txn1 := txnBuilder1.Build()

	txnBuilder2 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID)
	txn2 := txnBuilder2.Build()

	//Provide the transactions in reverse order to test sorting
	sortedTransactions, err := SortTransactions(ctx, []*Transaction{txn2, txn1})
	require.NoError(t, err)
	require.Len(t, sortedTransactions, 2)
	assert.Equal(t, txn1.pt.ID, sortedTransactions[0].pt.ID)
	assert.Equal(t, txn2.pt.ID, sortedTransactions[1].pt.ID)

}

func Test_SortTransactions_EndlessLoopPrevention(t *testing.T) {
	ctx := context.Background()

	//When a dependency exists that is not in the input list, it should not cause an endless loop
	// Under normal usage, this should not happen but if it does, we should handle it gracefully with an error

	grapher := NewGrapher(context.Background())

	txnBuilder1 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1)
	txn1 := txnBuilder1.Build()

	txnBuilder2 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID)
	txn2 := txnBuilder2.Build()

	txnBuilder3 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID)
	txn3 := txnBuilder3.Build()

	//Provide the transactions in reverse order to test sorting
	sortedTransactions, err := SortTransactions(ctx, []*Transaction{txn2, txn3})
	assert.Error(t, err)
	assert.Len(t, sortedTransactions, 0)

}

func Test_SortTransactions_ConfirmedDependency(t *testing.T) {
	ctx := context.Background()

	//When a dependency exists that is not in the input list, but the grapher has forgotten it, then it should not cause an error
	// This is most likely when a dependency exists but has long since been confirmed and removed from the grapher

	grapher := NewGrapher(context.Background())

	txnBuilder1 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1)
	txn1 := txnBuilder1.Build()

	err := grapher.Forget(txn1.pt.ID) // Simulate that the grapher has been instructed to forget the transaction as a result of it being confirmed
	assert.NoError(t, err)

	txnBuilder2 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID)
	txn2 := txnBuilder2.Build()

	txnBuilder3 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID)
	txn3 := txnBuilder3.Build()

	//Provide the transactions in reverse order to test sorting
	sortedTransactions, err := SortTransactions(ctx, []*Transaction{txn2, txn3})
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
	ctx := context.Background()
	// When a circular dependency exists, it should not cause an endless loop

	grapher := NewGrapher(context.Background())

	txnBuilder1 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1)
	txn1 := txnBuilder1.Build()

	txnBuilder2 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID)
	txn2 := txnBuilder2.Build()

	txnBuilder3 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		InputStateIDs(txn1.pt.PostAssembly.OutputStates[0].ID)
	txn3 := txnBuilder3.Build()

	//Provide the transactions in reverse order to test sorting
	sortedTransactions, err := SortTransactions(ctx, []*Transaction{txn2, txn3})
	assert.Error(t, err)
	assert.Len(t, sortedTransactions, 0)

}

func TestGrapher_AddMinter_DuplicateMinter(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	// Create two different transactions
	txnBuilder1 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1)
	txn1 := txnBuilder1.Build()

	txnBuilder2 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1)
	txn2 := txnBuilder2.Build()

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
