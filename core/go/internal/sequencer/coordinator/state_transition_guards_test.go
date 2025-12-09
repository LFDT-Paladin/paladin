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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGuard_Not(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Create a guard that always returns true
	alwaysTrue := func(ctx context.Context, c *coordinator) bool {
		return true
	}

	// Create a guard that always returns false
	alwaysFalse := func(ctx context.Context, c *coordinator) bool {
		return false
	}

	// Test that guard_Not negates true to false
	notTrue := guard_Not(alwaysTrue)
	assert.False(t, notTrue(ctx, c))

	// Test that guard_Not negates false to true
	notFalse := guard_Not(alwaysFalse)
	assert.True(t, notFalse(ctx, c))
}

func TestGuard_Behind(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _ := builder.Build(ctx)

	// Set block height tolerance to 5
	c.blockHeightTolerance = 5

	tests := []struct {
		name                         string
		currentBlockHeight           uint64
		activeCoordinatorBlockHeight uint64
		expected                     bool
	}{
		{
			name:                         "behind by more than tolerance",
			currentBlockHeight:           10,
			activeCoordinatorBlockHeight: 20,
			expected:                     true, // 10 < 20 - 5 = 15
		},
		{
			name:                         "behind by exactly tolerance",
			currentBlockHeight:           15,
			activeCoordinatorBlockHeight: 20,
			expected:                     false, // 15 is not < 20 - 5 = 15
		},
		{
			name:                         "behind by less than tolerance",
			currentBlockHeight:           16,
			activeCoordinatorBlockHeight: 20,
			expected:                     false, // 16 is not < 20 - 5 = 15
		},
		{
			name:                         "ahead of active coordinator",
			currentBlockHeight:           25,
			activeCoordinatorBlockHeight: 20,
			expected:                     false, // 25 is not < 20 - 5 = 15
		},
		{
			name:                         "equal to active coordinator minus tolerance",
			currentBlockHeight:           15,
			activeCoordinatorBlockHeight: 20,
			expected:                     false, // 15 is not < 15
		},
		{
			name:                         "same height",
			currentBlockHeight:           20,
			activeCoordinatorBlockHeight: 20,
			expected:                     false, // 20 is not < 20 - 5 = 15
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.currentBlockHeight = tt.currentBlockHeight
			c.activeCoordinatorBlockHeight = tt.activeCoordinatorBlockHeight
			result := guard_Behind(ctx, c)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGuard_ActiveCoordinatorFlushComplete_EmptyFlushPointsMap(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Prepared)
	c, _ := builder.Build(ctx)
	c.activeCoordinatorsFlushPointsBySignerNonce = make(map[string]*common.FlushPoint)
	result := guard_ActiveCoordinatorFlushComplete(ctx, c)
	assert.True(t, result, "empty map should return true")
}

func TestGuard_ActiveCoordinatorFlushComplete_AllFlushPointsConfirmed(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Prepared)
	c, _ := builder.Build(ctx)
	signer1 := pldtypes.RandAddress()
	signer2 := pldtypes.RandAddress()
	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		"signer1:1": {
			From:      *signer1,
			Nonce:     1,
			Confirmed: true,
		},
		"signer2:2": {
			From:      *signer2,
			Nonce:     2,
			Confirmed: true,
		},
	}
	result := guard_ActiveCoordinatorFlushComplete(ctx, c)
	assert.True(t, result, "all confirmed should return true")
}

func TestGuard_ActiveCoordinatorFlushComplete_OneFlushPointNotConfirmed(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Prepared)
	c, _ := builder.Build(ctx)
	signer1 := pldtypes.RandAddress()
	signer2 := pldtypes.RandAddress()
	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		"signer1:1": {
			From:      *signer1,
			Nonce:     1,
			Confirmed: true,
		},
		"signer2:2": {
			From:      *signer2,
			Nonce:     2,
			Confirmed: false,
		},
	}
	result := guard_ActiveCoordinatorFlushComplete(ctx, c)
	assert.False(t, result, "one unconfirmed should return false")
}

func TestGuard_ActiveCoordinatorFlushComplete_AllFlushPointsNotConfirmed(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Prepared)
	c, _ := builder.Build(ctx)
	signer1 := pldtypes.RandAddress()
	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		"signer1:1": {
			From:      *signer1,
			Nonce:     1,
			Confirmed: false,
		},
	}
	result := guard_ActiveCoordinatorFlushComplete(ctx, c)
	assert.False(t, result, "all unconfirmed should return false")
}

func TestGuard_FlushComplete_NoTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.transactionsByID = make(map[uuid.UUID]*transaction.Transaction)
	result := guard_FlushComplete(ctx, c)
	assert.True(t, result, "no transactions should return true")
}

func TestGuard_FlushComplete_TransactionsInOtherStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx1.ID: tx1,
		tx2.ID: tx2,
		tx3.ID: tx3,
	}
	result := guard_FlushComplete(ctx, c)
	assert.True(t, result, "transactions in other states should return true")
}

func TestGuard_FlushComplete_TransactionInReadyForDispatchState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx.ID: tx,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "transaction in Ready_For_Dispatch should return false")
}

func TestGuard_FlushComplete_TransactionInDispatchedState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx.ID: tx,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "transaction in Dispatched should return false")
}

func TestGuard_FlushComplete_TransactionInSubmittedState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx.ID: tx,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "transaction in Submitted should return false")
}

func TestGuard_FlushComplete_MultipleTransactionsInFlushStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	tx3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx1.ID: tx1,
		tx2.ID: tx2,
		tx3.ID: tx3,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "transactions in flush states should return false")
}

func TestGuard_FlushComplete_MixOfFlushAndNonFlushStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx1.ID: tx1,
		tx2.ID: tx2,
	}
	result := guard_FlushComplete(ctx, c)
	assert.False(t, result, "mix with flush state should return false")
}

func TestGuard_HasTransactionsInflight_NoTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.transactionsByID = make(map[uuid.UUID]*transaction.Transaction)
	result := guard_HasTransactionsInflight(ctx, c)
	assert.False(t, result, "no transactions should return false")
}

func TestGuard_HasTransactionsInflight_OnlyConfirmedTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx1.ID: tx1,
		tx2.ID: tx2,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.False(t, result, "only confirmed should return false")
}

func TestGuard_HasTransactionsInflight_TransactionInPooledState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx.ID: tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Pooled should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInAssemblingState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx.ID: tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Assembling should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInReadyForDispatchState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx.ID: tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Ready_For_Dispatch should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInDispatchedState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx.ID: tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Dispatched should return true")
}

func TestGuard_HasTransactionsInflight_TransactionInSubmittedState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx.ID: tx,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "transaction in Submitted should return true")
}

func TestGuard_HasTransactionsInflight_MixOfConfirmedAndInflight(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx1.ID: tx1,
		tx2.ID: tx2,
	}
	result := guard_HasTransactionsInflight(ctx, c)
	assert.True(t, result, "mix with inflight should return true")
}

func TestGuard_ClosingGracePeriodExpired_GracePeriodNotExpired(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Closing)
	c, _ := builder.Build(ctx)
	c.closingGracePeriod = 5
	c.heartbeatIntervalsSinceStateChange = 3
	result := guard_ClosingGracePeriodExpired(ctx, c)
	assert.False(t, result, "3 < 5 should return false")
}

func TestGuard_ClosingGracePeriodExpired_GracePeriodExactlyExpired(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Closing)
	c, _ := builder.Build(ctx)
	c.closingGracePeriod = 5
	c.heartbeatIntervalsSinceStateChange = 5
	result := guard_ClosingGracePeriodExpired(ctx, c)
	assert.True(t, result, "5 >= 5 should return true")
}

func TestGuard_ClosingGracePeriodExpired_GracePeriodExceeded(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Closing)
	c, _ := builder.Build(ctx)
	c.closingGracePeriod = 5
	c.heartbeatIntervalsSinceStateChange = 10
	result := guard_ClosingGracePeriodExpired(ctx, c)
	assert.True(t, result, "10 >= 5 should return true")
}

func TestGuard_ClosingGracePeriodExpired_ZeroGracePeriod(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Closing)
	c, _ := builder.Build(ctx)
	c.closingGracePeriod = 0
	c.heartbeatIntervalsSinceStateChange = 0
	result := guard_ClosingGracePeriodExpired(ctx, c)
	assert.True(t, result, "0 >= 0 should return true")
}

func TestGuard_ClosingGracePeriodExpired_ZeroGracePeriodWithIntervals(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Closing)
	c, _ := builder.Build(ctx)
	c.closingGracePeriod = 0
	c.heartbeatIntervalsSinceStateChange = 1
	result := guard_ClosingGracePeriodExpired(ctx, c)
	assert.True(t, result, "1 >= 0 should return true")
}

func TestGuard_HasTransactionAssembling_NoTransactions(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	c.transactionsByID = make(map[uuid.UUID]*transaction.Transaction)
	result := guard_HasTransactionAssembling(ctx, c)
	assert.False(t, result, "no transactions should return false")
}

func TestGuard_HasTransactionAssembling_TransactionInAssemblingState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx.ID: tx,
	}
	result := guard_HasTransactionAssembling(ctx, c)
	assert.True(t, result, "transaction in Assembling should return true")
}

func TestGuard_HasTransactionAssembling_MultipleTransactionsInAssemblingState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx1.ID: tx1,
		tx2.ID: tx2,
	}
	result := guard_HasTransactionAssembling(ctx, c)
	assert.True(t, result, "multiple transactions in Assembling should return true")
}

func TestGuard_HasTransactionAssembling_TransactionInOtherStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()
	tx3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx1.ID: tx1,
		tx2.ID: tx2,
		tx3.ID: tx3,
	}
	result := guard_HasTransactionAssembling(ctx, c)
	assert.False(t, result, "transactions in other states should return false")
}

func TestGuard_HasTransactionAssembling_MixOfAssemblingAndOtherStates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	tx1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	tx2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		tx1.ID: tx1,
		tx2.ID: tx2,
	}
	result := guard_HasTransactionAssembling(ctx, c)
	assert.True(t, result, "mix with Assembling should return true")
}
