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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_guard_HasFinalizingGracePeriodPassedSinceStateChange_FalseWhenLessThan(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		FinalizingGracePeriod(5).
		HeartbeatIntervalsSinceStateChange(3).
		Build()

	// Should return false when heartbeat intervals is less than grace period
	assert.False(t, guard_HasFinalizingGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_guard_HasFinalizingGracePeriodPassedSinceStateChange_TrueWhenEqual(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		FinalizingGracePeriod(5).
		HeartbeatIntervalsSinceStateChange(5).
		Build()

	// Should return true when heartbeat intervals equals grace period
	assert.True(t, guard_HasFinalizingGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_guard_HasFinalizingGracePeriodPassedSinceStateChange_TrueWhenGreaterThan(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		FinalizingGracePeriod(5).
		HeartbeatIntervalsSinceStateChange(7).
		Build()

	// Should return true when heartbeat intervals is greater than grace period
	assert.True(t, guard_HasFinalizingGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_guard_HasFinalizingGracePeriodPassedSinceStateChange_ZeroGracePeriod(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		FinalizingGracePeriod(0).
		HeartbeatIntervalsSinceStateChange(0).
		Build()

	// Should return true when both are zero (0 >= 0)
	assert.True(t, guard_HasFinalizingGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_guard_HasFinalizingGracePeriodPassedSinceStateChange_ZeroHeartbeatIntervals(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		FinalizingGracePeriod(5).
		HeartbeatIntervalsSinceStateChange(0).
		Build()

	// Should return false when heartbeat intervals is 0 and grace period is positive
	assert.False(t, guard_HasFinalizingGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_action_FinalizeAsUnknownByOriginator_CancelsRequestStateTimeoutSchedules(t *testing.T) {
	ctx := context.Background()
	cancelCalled := false
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		CancelRequestTimeoutSchedule(func() { cancelCalled = true }).
		Build()

	// Call action_FinalizeAsUnknownByOriginator
	err := action_FinalizeAsUnknownByOriginator(ctx, txn, nil)
	require.NoError(t, err)

	// Verify the cancel function was called
	assert.True(t, cancelCalled, "assemble request timeout cancel should have been called")
}

func Test_guard_HasConfirmedLockRetentionGracePeriodPassedSinceStateChange(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		ConfirmedLockRetentionGracePeriod(2).
		HeartbeatIntervalsSinceStateChange(1).
		Build()
	assert.False(t, guard_HasConfirmedLockRetentionGracePeriodPassedSinceStateChange(ctx, txn))

	txn, _ = NewTransactionBuilderForTesting(t, State_Confirmed).
		ConfirmedLockRetentionGracePeriod(2).
		HeartbeatIntervalsSinceStateChange(2).
		Build()
	assert.True(t, guard_HasConfirmedLockRetentionGracePeriodPassedSinceStateChange(ctx, txn))

	txn, _ = NewTransactionBuilderForTesting(t, State_Confirmed).
		ConfirmedLockRetentionGracePeriod(2).
		HeartbeatIntervalsSinceStateChange(2).
		ConfirmedLocksReleased(true).
		Build()
	assert.True(t, guard_HasConfirmedLockRetentionGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_action_ResetConfirmedTransactionLocksOnce_CallsResetAtMostOnce(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return().Once()

	err := action_ResetConfirmedTransactionLocksOnce(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, txn.confirmedLocksReleased)

	err = action_ResetConfirmedTransactionLocksOnce(ctx, txn, nil)
	require.NoError(t, err)
}
