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
)

func TestGuard_FlushComplete_NoTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.transactionsByID = make(map[uuid.UUID]transaction.CoordinatorTransaction)
	result := guard_FlushComplete(ctx, c)
	assert.True(t, result, "no transactions should return true")
}

func TestGuard_FlushComplete_TransactionsInOtherStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx3, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
		tx3.GetID(): tx3,
	}
	result := guard_FlushComplete(ctx, c)
	assert.True(t, result, "transactions in other states should return true")
}

func TestGuard_FlushComplete_TransactionInReadyForDispatchState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx.GetID(): tx,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "transaction in Ready_For_Dispatch should return false")
}

func TestGuard_FlushComplete_TransactionInDispatchedState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx.GetID(): tx,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "transaction in Dispatched should return false")
}

func TestGuard_FlushComplete_TransactionInSubmittedState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx.GetID(): tx,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "transaction in Submitted should return false")
}

func TestGuard_FlushComplete_MultipleTransactionsInFlushStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	tx3, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
		tx3.GetID(): tx3,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "transactions in flush states should return false")
}

func TestGuard_FlushComplete_MixOfFlushAndNonFlushStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "mix with flush state should return false")
}

func TestGuard_HasTransactionsInflight_NoTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.transactionsByID = make(map[uuid.UUID]transaction.CoordinatorTransaction)
	result := guard_HasTransactionsInflight(ctx, c)
	assert.False(t, result, "no transactions should return false")
}

func TestGuard_HasTransactionsInflight_OnlyConfirmedTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "only confirmed should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInPooledState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx.GetID(): tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Pooled should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInAssemblingState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx.GetID(): tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Assembling should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInReadyForDispatchState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx.GetID(): tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Ready_For_Dispatch should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInDispatchedState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx.GetID(): tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Dispatched should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInSubmittedState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx.GetID(): tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Submitted should return true")
}

func TestGuard_HasTransactionsInflight_MixOfConfirmedAndInflight(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "mix with inflight should return true")
}

func TestGuard_HasTransactionAssembling_NoTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.transactionsByID = make(map[uuid.UUID]transaction.CoordinatorTransaction)
	result := guard_HasTransactionAssembling(ctx, c)
	assert.False(t, result, "no transactions should return false")
}

func TestGuard_HasTransactionAssembling_TransactionInAssemblingState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx.GetID(): tx,
	}
	result := guard_HasTransactionAssembling(ctx, c)
	assert.True(t, result, "transaction in Assembling should return true")
}

func TestGuard_HasTransactionAssembling_MultipleTransactionsInAssemblingState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
	}
	result := guard_HasTransactionAssembling(ctx, c)
	assert.True(t, result, "multiple transactions in Assembling should return true")
}

func TestGuard_HasTransactionAssembling_TransactionInOtherStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	tx3, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
		tx3.GetID(): tx3,
	}
	result := guard_HasTransactionAssembling(ctx, c)
	assert.False(t, result, "transactions in other states should return false")
}

func TestGuard_HasTransactionAssembling_MixOfAssemblingAndOtherStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	tx1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]transaction.CoordinatorTransaction{
		tx1.GetID(): tx1,
		tx2.GetID(): tx2,
	}
	result := guard_HasTransactionAssembling(ctx, c)
	assert.True(t, result, "mix with Assembling should return true")
}
