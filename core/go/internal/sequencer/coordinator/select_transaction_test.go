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

package coordinator

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTransactionPool is a simple implementation of TransactionPool for testing
type mockTransactionPool struct {
	pooledTransactions []*transaction.Transaction
	transactionsByID   map[uuid.UUID]*transaction.Transaction
}

func newMockTransactionPool() *mockTransactionPool {
	return &mockTransactionPool{
		pooledTransactions: make([]*transaction.Transaction, 0),
		transactionsByID:   make(map[uuid.UUID]*transaction.Transaction),
	}
}

func (m *mockTransactionPool) AddTransactionToBackOfPool(_ context.Context, txn *transaction.Transaction) {
	m.pooledTransactions = append(m.pooledTransactions, txn)
	m.transactionsByID[txn.ID] = txn
}

func (m *mockTransactionPool) GetNextPooledTransaction(_ context.Context) *transaction.Transaction {
	if len(m.pooledTransactions) == 0 {
		return nil
	}
	nextPooledTx := m.pooledTransactions[0]
	m.pooledTransactions = m.pooledTransactions[1:]
	return nextPooledTx
}

func (m *mockTransactionPool) GetTransactionByID(_ context.Context, txnID uuid.UUID) *transaction.Transaction {
	return m.transactionsByID[txnID]
}

func TestSelectNextTransactionToAssemble_NoPooledTransactions(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	tx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, tx, "should return nil when no pooled transactions")
}

func TestSelectNextTransactionToAssemble_SelectsFromPool(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create a test transaction
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx1)

	tx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, tx, "should return a transaction from pool")
	assert.Equal(t, tx1.ID, tx.ID, "should return the correct transaction")
	assert.NotNil(t, selector.currentAssemblingOriginator, "should set current assembling originator")
	assert.Equal(t, tx1.OriginatorNode(), selector.currentAssemblingOriginator.node)
	assert.Equal(t, tx1.OriginatorIdentity(), selector.currentAssemblingOriginator.identity)
	assert.Equal(t, tx1.ID, selector.currentAssemblingOriginator.txID)
}

func TestSelectNextTransactionToAssemble_FIFOOrdering(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create multiple test transactions
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	tx3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	tx4 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	tx5 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()

	// Add transactions to pool in order
	pool.AddTransactionToBackOfPool(ctx, tx1)
	pool.AddTransactionToBackOfPool(ctx, tx2)
	pool.AddTransactionToBackOfPool(ctx, tx3)
	pool.AddTransactionToBackOfPool(ctx, tx4)
	pool.AddTransactionToBackOfPool(ctx, tx5)

	// Select transactions and verify FIFO order
	expectedOrder := []uuid.UUID{tx1.ID, tx2.ID, tx3.ID, tx4.ID, tx5.ID}
	selectedOrder := make([]uuid.UUID, 0, 5)

	for i := 0; i < 5; i++ {
		// Simulate transaction moving out of assembling state
		if selector.currentAssemblingOriginator != nil {
			event := &TransactionStateTransitionEvent{
				TransactionID: selector.currentAssemblingOriginator.txID,
				From:          transaction.State_Assembling,
				To:            transaction.State_Endorsement_Gathering,
			}
			tx, err := selector.SelectNextTransactionToAssemble(ctx, event)
			require.NoError(t, err)
			if tx != nil {
				selectedOrder = append(selectedOrder, tx.ID)
			}
		} else {
			// First selection
			tx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
			require.NoError(t, err)
			require.NotNil(t, tx, "should return a transaction")
			selectedOrder = append(selectedOrder, tx.ID)
		}
	}

	// Verify FIFO ordering
	assert.Equal(t, expectedOrder, selectedOrder, "transactions should be selected in FIFO order")
}

func TestSelectNextTransactionToAssemble_BlocksWhenTransactionStillAssembling(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create and select first transaction
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx1)

	selectedTx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx)
	assert.Equal(t, tx1.ID, selectedTx.ID)

	// Add another transaction to pool
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx2)

	// Try to select again while first transaction is still assembling
	selectedTx2, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, selectedTx2, "should return nil when transaction is still being assembled")
}

func TestSelectNextTransactionToAssemble_AllowsSelectionAfterTransitionToEndorsementGathering(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create and select first transaction
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx1)

	selectedTx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx)

	// Add second transaction to pool
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx2)

	// Transition first transaction to endorsement gathering
	event := &TransactionStateTransitionEvent{
		TransactionID: tx1.ID,
		From:          transaction.State_Assembling,
		To:            transaction.State_Endorsement_Gathering,
	}

	selectedTx2, err := selector.SelectNextTransactionToAssemble(ctx, event)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx2, "should return next transaction after transition")
	assert.Equal(t, tx2.ID, selectedTx2.ID, "should return the next pooled transaction")
	assert.NotNil(t, selector.currentAssemblingOriginator, "should set current assembling originator to new transaction")
	assert.Equal(t, tx2.ID, selector.currentAssemblingOriginator.txID, "should set current assembling originator to new transaction ID")
}

func TestSelectNextTransactionToAssemble_AllowsSelectionAfterTransitionToPooled(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create and select first transaction
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx1)

	selectedTx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx)

	// Add second transaction to pool
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx2)

	// Transition first transaction back to pooled (e.g., timeout)
	event := &TransactionStateTransitionEvent{
		TransactionID: tx1.ID,
		From:          transaction.State_Assembling,
		To:            transaction.State_Pooled,
	}

	selectedTx2, err := selector.SelectNextTransactionToAssemble(ctx, event)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx2, "should return next transaction after transition to pooled")
	assert.Equal(t, tx2.ID, selectedTx2.ID, "should return the next pooled transaction")
}

func TestSelectNextTransactionToAssemble_AllowsSelectionAfterTransitionToReverted(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create and select first transaction
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx1)

	selectedTx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx)

	// Add second transaction to pool
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx2)

	// Transition first transaction to reverted
	event := &TransactionStateTransitionEvent{
		TransactionID: tx1.ID,
		From:          transaction.State_Assembling,
		To:            transaction.State_Reverted,
	}

	selectedTx2, err := selector.SelectNextTransactionToAssemble(ctx, event)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx2, "should return next transaction after transition to reverted")
	assert.Equal(t, tx2.ID, selectedTx2.ID, "should return the next pooled transaction")
}

func TestSelectNextTransactionToAssemble_AllowsSelectionAfterTransitionToConfirmingDispatchable(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create and select first transaction
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx1)

	selectedTx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx)

	// Add second transaction to pool
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx2)

	// Transition first transaction to confirming dispatchable
	event := &TransactionStateTransitionEvent{
		TransactionID: tx1.ID,
		From:          transaction.State_Assembling,
		To:            transaction.State_Confirming_Dispatchable,
	}

	selectedTx2, err := selector.SelectNextTransactionToAssemble(ctx, event)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx2, "should return next transaction after transition to confirming dispatchable")
	assert.Equal(t, tx2.ID, selectedTx2.ID, "should return the next pooled transaction")
}

func TestSelectNextTransactionToAssemble_ErrorWhenTransactionNotFound(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create and select first transaction
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx1)

	selectedTx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx)

	// Create event for transaction not in pool
	unknownTxID := uuid.New()
	event := &TransactionStateTransitionEvent{
		TransactionID: unknownTxID,
		From:          transaction.State_Assembling,
		To:            transaction.State_Endorsement_Gathering,
	}

	selectedTx2, err := selector.SelectNextTransactionToAssemble(ctx, event)
	assert.Error(t, err, "should return error when transaction not found in pool")
	assert.Nil(t, selectedTx2, "should return nil transaction on error")
}

func TestSelectNextTransactionToAssemble_IgnoresEventForDifferentTransaction(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create and select first transaction
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx1)

	selectedTx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx)

	// Add second transaction to pool
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx2)

	// Create event for different transaction (not the one currently assembling)
	otherTx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, otherTx)

	event := &TransactionStateTransitionEvent{
		TransactionID: otherTx.ID,
		From:          transaction.State_Assembling,
		To:            transaction.State_Endorsement_Gathering,
	}

	selectedTx2, err := selector.SelectNextTransactionToAssemble(ctx, event)
	require.NoError(t, err)
	assert.Nil(t, selectedTx2, "should return nil when event is for different transaction")
}

func TestSelectNextTransactionToAssemble_TransitionFromEndorsementGatheringToPooled(t *testing.T) {
	ctx := context.Background()
	pool := newMockTransactionPool()
	selector := NewTransactionSelector(ctx, pool).(*transactionSelector)

	// Create and select first transaction
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx1)

	selectedTx, err := selector.SelectNextTransactionToAssemble(ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx)

	// Transition to endorsement gathering first (this clears currentAssemblingOriginator)
	event1 := &TransactionStateTransitionEvent{
		TransactionID: tx1.ID,
		From:          transaction.State_Assembling,
		To:            transaction.State_Endorsement_Gathering,
	}

	// Add second transaction to pool
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	pool.AddTransactionToBackOfPool(ctx, tx2)

	// This should select tx2 and clear the currentAssemblingOriginator from tx1
	selectedTx2, err := selector.SelectNextTransactionToAssemble(ctx, event1)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx2)
	assert.Equal(t, tx2.ID, selectedTx2.ID)

	// Now transition tx1 from endorsement gathering back to pooled
	// Since tx1 is not the current assembling transaction, this event should be ignored
	// but we should still be able to select a new transaction since tx2 is now assembling
	event2 := &TransactionStateTransitionEvent{
		TransactionID: tx1.ID,
		From:          transaction.State_Endorsement_Gathering,
		To:            transaction.State_Pooled,
	}

	// Try to select - should return nil because tx2 is still assembling
	selectedTx3, err := selector.SelectNextTransactionToAssemble(ctx, event2)
	require.NoError(t, err)
	assert.Nil(t, selectedTx3, "should return nil when transaction is still being assembled")

	// Now transition tx2 out of assembling to allow selection
	event3 := &TransactionStateTransitionEvent{
		TransactionID: tx2.ID,
		From:          transaction.State_Assembling,
		To:            transaction.State_Endorsement_Gathering,
	}

	// Add tx1 back to pool (it was transitioned to pooled)
	pool.AddTransactionToBackOfPool(ctx, tx1)

	// Now we should be able to select tx1
	selectedTx4, err := selector.SelectNextTransactionToAssemble(ctx, event3)
	require.NoError(t, err)
	assert.NotNil(t, selectedTx4, "should allow selection after transition from assembling")
	assert.Equal(t, tx1.ID, selectedTx4.ID, "should return the transaction that was re-pooled")
}
