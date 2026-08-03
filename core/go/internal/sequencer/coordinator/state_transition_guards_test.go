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
	"github.com/LFDT-Paladin/paladin/core/mocks/coordinatortransactionmocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGuard_HasUnconfirmedDispatchedTransactions_NoTransactions(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()
	result := guard_HasUnconfirmedDispatchedTransactions(ctx, c)
	assert.False(t, result, "no transactions should return false")
}

func TestGuard_HasUnconfirmedDispatchedTransactions_TransactionsInOtherStates(t *testing.T) {
	ctx := context.Background()
	tx1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx1.EXPECT().GetID().Return(uuid.New())
	tx1.EXPECT().GetCurrentState().Return(transaction.State_Pooled)

	tx2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx2.EXPECT().GetID().Return(uuid.New())
	tx2.EXPECT().GetCurrentState().Return(transaction.State_Assembling)

	tx3 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx3.EXPECT().GetID().Return(uuid.New())
	tx3.EXPECT().GetCurrentState().Return(transaction.State_Confirmed)

	tx4 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx4.EXPECT().GetID().Return(uuid.New())
	tx4.EXPECT().GetCurrentState().Return(transaction.State_Ready_For_Dispatch)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx1, tx2, tx3, tx4).Build()
	result := guard_HasUnconfirmedDispatchedTransactions(ctx, c)
	assert.False(t, result, "transactions in other states should return false")
}

func TestGuard_HasUnconfirmedDispatchedTransactions_TransactionInDispatchedState(t *testing.T) {
	ctx := context.Background()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx.EXPECT().GetID().Return(uuid.New())
	tx.EXPECT().GetCurrentState().Return(transaction.State_Dispatched)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx).Build()
	result := guard_HasUnconfirmedDispatchedTransactions(ctx, c)
	assert.True(t, result, "transaction in Dispatched should return true")
}

func TestGuard_FlushComplete_TransactionInSubmittedState(t *testing.T) {
	ctx := context.Background()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx.EXPECT().GetID().Return(uuid.New())
	tx.EXPECT().GetCurrentState().Return(transaction.State_Dispatched)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx).Build()
	result := guard_HasUnconfirmedDispatchedTransactions(ctx, c)
	assert.True(t, result, "transaction in Submitted should return true")
}

func TestGuard_HasUnconfirmedDispatchedTransactions_MultipleTransactionsInFlushStates(t *testing.T) {
	ctx := context.Background()
	tx1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx1.EXPECT().GetID().Return(uuid.New())
	tx1.EXPECT().GetCurrentState().Return(transaction.State_Dispatched)

	tx2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx2.EXPECT().GetID().Return(uuid.New())
	tx2.EXPECT().GetCurrentState().Return(transaction.State_Dispatched)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx1, tx2).Build()
	result := guard_HasUnconfirmedDispatchedTransactions(ctx, c)
	assert.True(t, result, "transactions in flush states should return true")
}

func TestGuard_FlushComplete_MixOfFlushAndNonFlushStates(t *testing.T) {
	ctx := context.Background()
	tx1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx1.EXPECT().GetID().Return(uuid.New())
	tx1.EXPECT().GetCurrentState().Return(transaction.State_Ready_For_Dispatch)

	tx2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx2.EXPECT().GetID().Return(uuid.New())
	tx2.EXPECT().GetCurrentState().Return(transaction.State_Dispatched)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx1, tx2).Build()
	result := guard_HasUnconfirmedDispatchedTransactions(ctx, c)
	assert.True(t, result, "mix with flush state should return true")
}

func TestGuard_HasTransactionsInflight_NoTransactions(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()
	result := guard_HasTransactionsInflight(ctx, c)
	assert.False(t, result, "no transactions should return false")
}

func TestGuard_HasTransactionsInflight_OnlyConfirmedTransactions(t *testing.T) {
	ctx := context.Background()
	tx1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx1.EXPECT().GetID().Return(uuid.New())

	tx2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx2.EXPECT().GetID().Return(uuid.New())

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx1, tx2).Build()
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "only confirmed should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInPooledState(t *testing.T) {
	ctx := context.Background()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx.EXPECT().GetID().Return(uuid.New())

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx).Build()
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Pooled should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInAssemblingState(t *testing.T) {
	ctx := context.Background()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx.EXPECT().GetID().Return(uuid.New())

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx).Build()
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Assembling should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInReadyForDispatchState(t *testing.T) {
	ctx := context.Background()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx.EXPECT().GetID().Return(uuid.New())

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx).Build()
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Ready_For_Dispatch should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInDispatchedState(t *testing.T) {
	ctx := context.Background()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx.EXPECT().GetID().Return(uuid.New())

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx).Build()
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Dispatched should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInSubmittedState(t *testing.T) {
	ctx := context.Background()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx.EXPECT().GetID().Return(uuid.New())

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx).Build()
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Submitted should return true")
}

func TestGuard_HasTransactionsInflight_MixOfConfirmedAndInflight(t *testing.T) {
	ctx := context.Background()
	tx1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx1.EXPECT().GetID().Return(uuid.New())

	tx2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx2.EXPECT().GetID().Return(uuid.New())

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx1, tx2).Build()
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "mix with inflight should return true")
}

func TestGuard_HasTransactionAssembling_NoTransactions(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()
	result := guard_HasTransactionAssembling(ctx, c)
	assert.False(t, result, "no transactions should return false")
}

func TestGuard_HasTransactionAssembling_AssemblySlotOccupied(t *testing.T) {
	ctx := context.Background()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	tx.EXPECT().GetID().Return(txID)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx).AssemblingTransaction(txID).Build()
	result := guard_HasTransactionAssembling(ctx, c)
	assert.True(t, result, "occupied assembly slot should return true")
}

func TestGuard_HasTransactionAssembling_AssemblySlotFree(t *testing.T) {
	ctx := context.Background()
	tx := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	tx.EXPECT().GetID().Return(uuid.New())

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(tx).Build()
	result := guard_HasTransactionAssembling(ctx, c)
	assert.False(t, result, "free assembly slot should return false")
}

func TestGuard_ClosingGracePeriodExpired_FalseWhenLessThan(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		ClosingGracePeriod(5).
		HeartbeatIntervalsSinceStateChange(3).
		Build()
	assert.False(t, guard_ClosingGracePeriodExpired(ctx, c))
}

func TestGuard_ClosingGracePeriodExpired_FalseWhenEqual(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		ClosingGracePeriod(5).
		HeartbeatIntervalsSinceStateChange(5).
		Build()
	assert.False(t, guard_ClosingGracePeriodExpired(ctx, c))
}

func TestGuard_ClosingGracePeriodExpired_TrueWhenGreaterThan(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		ClosingGracePeriod(5).
		HeartbeatIntervalsSinceStateChange(7).
		Build()
	assert.True(t, guard_ClosingGracePeriodExpired(ctx, c))
}

func TestGuard_ClosingGracePeriodExpired_MinimumGracePeriod(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		ClosingGracePeriod(1).
		HeartbeatIntervalsSinceStateChange(2).
		Build()
	assert.True(t, guard_ClosingGracePeriodExpired(ctx, c))
}

func TestGuard_ClosingGracePeriodExpired_ZeroHeartbeatIntervals(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Closing).
		ClosingGracePeriod(5).
		HeartbeatIntervalsSinceStateChange(0).
		Build()
	assert.False(t, guard_ClosingGracePeriodExpired(ctx, c))
}
