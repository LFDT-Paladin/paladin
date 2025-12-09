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
	"time"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransaction_GetLastDelegatedTime_InitiallyNil(t *testing.T) {
	// Test that GetLastDelegatedTime returns nil for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	lastDelegatedTime := txn.GetLastDelegatedTime()
	assert.Nil(t, lastDelegatedTime, "GetLastDelegatedTime should return nil for a newly created transaction")
}

func TestTransaction_UpdateLastDelegatedTime_SetsTime(t *testing.T) {
	// Test that UpdateLastDelegatedTime sets a non-nil time value
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// Initially should be nil
	require.Nil(t, txn.GetLastDelegatedTime(), "LastDelegatedTime should be nil initially")

	// Update the time
	txn.UpdateLastDelegatedTime()

	// Verify that the time is now set
	lastDelegatedTime := txn.GetLastDelegatedTime()
	assert.NotNil(t, lastDelegatedTime, "GetLastDelegatedTime should return a non-nil value after UpdateLastDelegatedTime")

	// Verify that the time is a valid time.Time (since RealClock returns time.Now())
	realTime, ok := (*lastDelegatedTime).(time.Time)
	require.True(t, ok, "LastDelegatedTime should be a time.Time")
	assert.False(t, realTime.IsZero(), "LastDelegatedTime should not be zero")
}

func TestTransaction_UpdateLastDelegatedTime_UpdatesTime(t *testing.T) {
	// Test that multiple calls to UpdateLastDelegatedTime update the time value
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// First update
	txn.UpdateLastDelegatedTime()
	firstTime := txn.GetLastDelegatedTime()
	require.NotNil(t, firstTime, "First update should set a time")

	// Wait a small amount to ensure the next time will be different
	time.Sleep(10 * time.Millisecond)

	// Second update
	txn.UpdateLastDelegatedTime()
	secondTime := txn.GetLastDelegatedTime()
	require.NotNil(t, secondTime, "Second update should set a time")

	// Verify that the times are different
	firstRealTime := (*firstTime).(time.Time)
	secondRealTime := (*secondTime).(time.Time)
	assert.True(t, secondRealTime.After(firstRealTime), "Second time should be after the first time")
}

func TestTransaction_GetLastDelegatedTime_ReturnsUpdatedTime(t *testing.T) {
	// Test that GetLastDelegatedTime returns the time set by UpdateLastDelegatedTime
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// Update the time
	txn.UpdateLastDelegatedTime()

	// Get the time multiple times and verify it returns the same value
	time1 := txn.GetLastDelegatedTime()
	time2 := txn.GetLastDelegatedTime()

	assert.Equal(t, time1, time2, "GetLastDelegatedTime should return the same value when called multiple times without updating")
	assert.NotNil(t, time1, "GetLastDelegatedTime should return a non-nil value")
}

func TestTransaction_Hash_ErrorWhenPrivateTransactionIsNil(t *testing.T) {
	// Test that Hash returns an error when PrivateTransaction is nil
	ctx := context.Background()

	// Create a transaction with nil PrivateTransaction by manually constructing it
	txn := &Transaction{
		PrivateTransaction: nil,
		stateMachine: &StateMachine{
			currentState: State_Initial,
		},
	}

	hash, err := txn.Hash(ctx)

	assert.Error(t, err, "Hash should return an error when PrivateTransaction is nil")
	assert.Nil(t, hash, "Hash should return nil hash when PrivateTransaction is nil")
	assert.Contains(t, err.Error(), "cannot hash transaction without PrivateTransaction", "Error message should indicate the validation failure")
}

func TestTransaction_Hash_ErrorWhenPostAssemblyIsNil(t *testing.T) {
	// Test that Hash returns an error when PostAssembly is nil
	ctx := context.Background()

	// Create a transaction with a PrivateTransaction that has nil PostAssembly
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// Set PostAssembly to nil to test the error case
	txn.PostAssembly = nil

	hash, err := txn.Hash(ctx)

	assert.Error(t, err, "Hash should return an error when PostAssembly is nil")
	assert.Nil(t, hash, "Hash should return nil hash when PostAssembly is nil")
	assert.Contains(t, err.Error(), "cannot hash transaction without PostAssembly", "Error message should indicate the validation failure")
}

func TestTransaction_GetCurrentState_ReturnsInitialState(t *testing.T) {
	// Test that GetCurrentState returns the initial state for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	state := txn.GetCurrentState()
	assert.Equal(t, State_Initial, state, "GetCurrentState should return State_Initial for a newly created transaction")
}

func TestTransaction_GetCurrentState_ReturnsDifferentStates(t *testing.T) {
	// Test that GetCurrentState returns the correct state for different state values
	testCases := []struct {
		name  string
		state State
	}{
		{"Initial", State_Initial},
		{"Pending", State_Pending},
		{"Delegated", State_Delegated},
		{"Assembling", State_Assembling},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := NewTransactionBuilderForTesting(t, tc.state)
			txn, _ := builder.BuildWithMocks()

			state := txn.GetCurrentState()
			assert.Equal(t, tc.state, state, "GetCurrentState should return the expected state")
		})
	}
}

func TestTransaction_GetLatestEvent_ReturnsEmptyStringInitially(t *testing.T) {
	// Test that GetLatestEvent returns an empty string for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	event := txn.GetLatestEvent()
	assert.Equal(t, "", event, "GetLatestEvent should return an empty string for a newly created transaction")
}

func TestTransaction_GetLatestEvent_ReturnsSetEvent(t *testing.T) {
	// Test that GetLatestEvent returns the event that was set on the state machine
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	expectedEvent := "test-event"
	txn.stateMachine.latestEvent = expectedEvent

	event := txn.GetLatestEvent()
	assert.Equal(t, expectedEvent, event, "GetLatestEvent should return the event that was set")
}

func TestTransaction_GetSignerAddress_ReturnsNilInitially(t *testing.T) {
	// Test that GetSignerAddress returns nil for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	signerAddress := txn.GetSignerAddress()
	assert.Nil(t, signerAddress, "GetSignerAddress should return nil for a newly created transaction")
}

func TestTransaction_GetSignerAddress_ReturnsSetAddress(t *testing.T) {
	// Test that GetSignerAddress returns the address that was set
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	expectedAddress := pldtypes.RandAddress()
	txn.signerAddress = expectedAddress

	signerAddress := txn.GetSignerAddress()
	assert.Equal(t, expectedAddress, signerAddress, "GetSignerAddress should return the address that was set")
	assert.NotNil(t, signerAddress, "GetSignerAddress should return a non-nil address")
}

func TestTransaction_GetLatestSubmissionHash_ReturnsNilInitially(t *testing.T) {
	// Test that GetLatestSubmissionHash returns nil for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	hash := txn.GetLatestSubmissionHash()
	assert.Nil(t, hash, "GetLatestSubmissionHash should return nil for a newly created transaction")
}

func TestTransaction_GetLatestSubmissionHash_ReturnsSetHash(t *testing.T) {
	// Test that GetLatestSubmissionHash returns the hash that was set
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	expectedHash := ptrTo(pldtypes.RandBytes32())
	txn.latestSubmissionHash = expectedHash

	hash := txn.GetLatestSubmissionHash()
	assert.Equal(t, expectedHash, hash, "GetLatestSubmissionHash should return the hash that was set")
	assert.NotNil(t, hash, "GetLatestSubmissionHash should return a non-nil hash")
}

func TestTransaction_GetNonce_ReturnsNilInitially(t *testing.T) {
	// Test that GetNonce returns nil for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	nonce := txn.GetNonce()
	assert.Nil(t, nonce, "GetNonce should return nil for a newly created transaction")
}

func TestTransaction_GetNonce_ReturnsSetNonce(t *testing.T) {
	// Test that GetNonce returns the nonce that was set
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	expectedNonce := uint64(12345)
	txn.nonce = &expectedNonce

	nonce := txn.GetNonce()
	assert.Equal(t, &expectedNonce, nonce, "GetNonce should return the nonce that was set")
	assert.NotNil(t, nonce, "GetNonce should return a non-nil nonce")
	assert.Equal(t, expectedNonce, *nonce, "GetNonce should return the correct nonce value")
}
