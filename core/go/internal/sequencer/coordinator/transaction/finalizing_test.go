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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_guard_HasGracePeriodPassedSinceStateChange_FalseWhenLessThan(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 5 and heartbeat intervals to 3 (less than grace period)
	txn.finalizingGracePeriod = 5
	txn.heartbeatIntervalsSinceStateChange = 3

	// Should return false when heartbeat intervals is less than grace period
	assert.False(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_guard_HasGracePeriodPassedSinceStateChange_TrueWhenEqual(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 5 and heartbeat intervals to 5 (equal to grace period)
	txn.finalizingGracePeriod = 5
	txn.heartbeatIntervalsSinceStateChange = 5

	// Should return true when heartbeat intervals equals grace period
	assert.True(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_guard_HasGracePeriodPassedSinceStateChange_TrueWhenGreaterThan(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 5 and heartbeat intervals to 7 (greater than grace period)
	txn.finalizingGracePeriod = 5
	txn.heartbeatIntervalsSinceStateChange = 7

	// Should return true when heartbeat intervals is greater than grace period
	assert.True(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_guard_HasGracePeriodPassedSinceStateChange_ZeroGracePeriod(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 0 and heartbeat intervals to 0
	txn.finalizingGracePeriod = 0
	txn.heartbeatIntervalsSinceStateChange = 0

	// Should return true when both are zero (0 >= 0)
	assert.True(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_guard_HasGracePeriodPassedSinceStateChange_ZeroHeartbeatIntervals(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set grace period to 5 and heartbeat intervals to 0
	txn.finalizingGracePeriod = 5
	txn.heartbeatIntervalsSinceStateChange = 0

	// Should return false when heartbeat intervals is 0 and grace period is positive
	assert.False(t, guard_HasGracePeriodPassedSinceStateChange(ctx, txn))
}

func Test_action_FinalizeAsUnknownByOriginator_CallsQueueTransactionFinalize(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	// Set up the mock to verify QueueTransactionFinalize is called with correct parameters
	mockSyncPoints := mocks.syncPoints.(*syncpoints.MockSyncPoints)
	mockSyncPoints.On("QueueTransactionFinalize",
		ctx,
		txn.pt.Domain,
		pldtypes.EthAddress{},
		txn.originator,
		txn.pt.ID,
		"originator reported transaction as unknown",
		mock.Anything, // onSuccess callback
		mock.Anything, // onError callback
	).Return(nil)

	// Call action_FinalizeAsUnknownByOriginator
	err := action_FinalizeAsUnknownByOriginator(ctx, txn, nil)
	require.NoError(t, err)

	// Verify QueueTransactionFinalize was called
	mockSyncPoints.AssertExpectations(t)
}

func Test_action_FinalizeAsUnknownByOriginator_CancelsRequestStateTimeoutSchedules(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	// Set up a cancel function to track if it's called
	cancelCalled := false
	txn.cancelRequestTimeoutSchedule = func() { cancelCalled = true }

	// Set up the mock
	mockSyncPoints := mocks.syncPoints.(*syncpoints.MockSyncPoints)
	mockSyncPoints.On("QueueTransactionFinalize",
		ctx,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything,
	).Return(nil)

	// Call action_FinalizeAsUnknownByOriginator
	err := action_FinalizeAsUnknownByOriginator(ctx, txn, nil)
	require.NoError(t, err)

	// Verify the cancel function was called
	assert.True(t, cancelCalled, "assemble request timeout cancel should have been called")
}

func Test_finalizeAsUnknownByOriginator_OnSuccessCallback(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	var onSuccessCalled bool
	mockSyncPoints := mocks.syncPoints.(*syncpoints.MockSyncPoints)
	mockSyncPoints.On("QueueTransactionFinalize",
		ctx,
		txn.pt.Domain,
		pldtypes.EthAddress{},
		txn.originator,
		txn.pt.ID,
		"originator reported transaction as unknown",
		mock.Anything,
		mock.Anything,
	).Run(func(args mock.Arguments) {
		onSuccess := args.Get(6).(func(context.Context))
		onSuccess(ctx)
		onSuccessCalled = true
	}).Return(nil)

	err := action_FinalizeAsUnknownByOriginator(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, onSuccessCalled)
	mockSyncPoints.AssertExpectations(t)
}

func Test_finalizeAsUnknownByOriginator_OnErrorCallback_Retries(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	callCount := 0
	mockSyncPoints := mocks.syncPoints.(*syncpoints.MockSyncPoints)
	mockSyncPoints.On("QueueTransactionFinalize",
		ctx,
		txn.pt.Domain,
		pldtypes.EthAddress{},
		txn.originator,
		txn.pt.ID,
		"originator reported transaction as unknown",
		mock.Anything,
		mock.Anything,
	).Run(func(args mock.Arguments) {
		callCount++
		if callCount == 1 {
			onError := args.Get(7).(func(context.Context, error))
			onError(ctx, assert.AnError)
		}
	}).Return(nil)

	err := action_FinalizeAsUnknownByOriginator(ctx, txn, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, callCount, 1)
	mockSyncPoints.AssertExpectations(t)
}
