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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuard_HasGracePeriodPassedSinceStateChange_FalseWhenLessThan(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 5 and heartbeat intervals to 3 (less than grace period)
	txn.finalizingGracePeriod = 5
	txn.heartbeatIntervalsSinceStateChange = 3

	// Should return false when heartbeat intervals is less than grace period
	assert.False(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func TestGuard_HasGracePeriodPassedSinceStateChange_TrueWhenEqual(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 5 and heartbeat intervals to 5 (equal to grace period)
	txn.finalizingGracePeriod = 5
	txn.heartbeatIntervalsSinceStateChange = 5

	// Should return true when heartbeat intervals equals grace period
	assert.True(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func TestGuard_HasGracePeriodPassedSinceStateChange_TrueWhenGreaterThan(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 5 and heartbeat intervals to 7 (greater than grace period)
	txn.finalizingGracePeriod = 5
	txn.heartbeatIntervalsSinceStateChange = 7

	// Should return true when heartbeat intervals is greater than grace period
	assert.True(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func TestGuard_HasGracePeriodPassedSinceStateChange_ZeroGracePeriod(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 0 and heartbeat intervals to 0
	txn.finalizingGracePeriod = 0
	txn.heartbeatIntervalsSinceStateChange = 0

	// Should return true when both are zero (0 >= 0)
	assert.True(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func TestGuard_HasGracePeriodPassedSinceStateChange_ZeroHeartbeatIntervals(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 5 and heartbeat intervals to 0
	txn.finalizingGracePeriod = 5
	txn.heartbeatIntervalsSinceStateChange = 0

	// Should return false when heartbeat intervals is 0 and grace period is positive
	assert.False(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func TestAction_Cleanup_Success(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)

	// Add transaction to grapher so we can verify it's removed
	grapher.Add(ctx, txn)

	// Track if onCleanup was called
	cleanupCalled := false
	txn.onCleanup = func(ctx context.Context) {
		cleanupCalled = true
	}

	// Call action_Cleanup
	err := action_Cleanup(ctx, txn)
	require.NoError(t, err)

	// Verify onCleanup was called
	assert.True(t, cleanupCalled, "onCleanup should have been called")

	// Verify transaction was removed from grapher
	assert.Nil(t, grapher.TransactionByID(ctx, txn.ID), "Transaction should be removed from grapher")
}

func TestAction_Cleanup_ForgetError(t *testing.T) {
	ctx := context.Background()
	// Create a transaction first to get its ID
	txn, _ := newTransactionForUnitTesting(t, nil)
	
	// Create a mock grapher that returns an error
	mockGrapher := NewMockGrapher(t)
	expectedError := errors.New("forget error")
	mockGrapher.EXPECT().Forget(txn.ID).Return(expectedError)
	
	// Set the mock grapher on the transaction
	txn.grapher = mockGrapher

	// Track if onCleanup was called
	cleanupCalled := false
	txn.onCleanup = func(ctx context.Context) {
		cleanupCalled = true
	}

	// Call action_Cleanup
	err := action_Cleanup(ctx, txn)
	
	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Verify onCleanup was still called (cleanup should call onCleanup before grapher.Forget)
	assert.True(t, cleanupCalled, "onCleanup should have been called even if Forget returns error")
}

