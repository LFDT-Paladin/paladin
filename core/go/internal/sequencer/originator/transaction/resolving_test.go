//go:build !generate_mocks

/*
 * Copyright © 2026 Kaleido, Inc.
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
	"time"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_handleResolveVerifiers_Success_EmitsVerifiersResolvedEvent(t *testing.T) {
	ctx := context.Background()
	resolved := []*prototk.ResolvedVerifier{{Lookup: "alice@node1", Verifier: "0xabc"}}
	builder := NewTransactionBuilderForTesting(t, State_Resolving).WithResolveVerifiersResult(resolved, nil)
	txn, mocks := builder.BuildWithMocks()

	txn.handleResolveVerifiers(ctx, txn.pt.ID, txn.pt.PreAssembly.GetRequiredVerifiers())

	event := <-mocks.Events
	resolvedEvent, ok := event.(*VerifiersResolvedEvent)
	require.True(t, ok, "Event should be VerifiersResolvedEvent")
	assert.Equal(t, txn.pt.ID, resolvedEvent.TransactionID)
	assert.Equal(t, resolved, resolvedEvent.ResolvedVerifiers)
}

func Test_handleResolveVerifiers_Error_EmitsFailedEvent(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Resolving).
		WithResolveVerifiersResult(nil, errors.New("resolver offline"))
	txn, mocks := builder.BuildWithMocks()

	txn.handleResolveVerifiers(ctx, txn.pt.ID, txn.pt.PreAssembly.GetRequiredVerifiers())

	event := <-mocks.Events
	failedEvent, ok := event.(*VerifierResolutionFailedEvent)
	require.True(t, ok, "Event should be VerifierResolutionFailedEvent")
	assert.Equal(t, txn.pt.ID, failedEvent.TransactionID)
}

func Test_action_ResolveVerifiers_SpawnsGoroutineAndEmitsEvent(t *testing.T) {
	ctx := context.Background()
	resolved := []*prototk.ResolvedVerifier{{Lookup: "alice@node1", Verifier: "0xabc"}}
	builder := NewTransactionBuilderForTesting(t, State_Resolving).WithResolveVerifiersResult(resolved, nil)
	txn, mocks := builder.BuildWithMocks()

	require.NoError(t, action_ResolveVerifiers(ctx, txn, nil))

	event := <-mocks.Events
	resolvedEvent, ok := event.(*VerifiersResolvedEvent)
	require.True(t, ok, "Event should be VerifiersResolvedEvent")
	assert.Equal(t, resolved, resolvedEvent.ResolvedVerifiers)
}

func Test_action_VerifiersResolved_StoresOnPreAssembly(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Resolving).Build()
	resolved := []*prototk.ResolvedVerifier{{Lookup: "alice@node1", Verifier: "0xabc"}}

	require.NoError(t, action_VerifiersResolved(ctx, txn, &VerifiersResolvedEvent{
		BaseEvent:         BaseEvent{TransactionID: txn.pt.ID},
		ResolvedVerifiers: resolved,
	}))

	assert.Equal(t, resolved, txn.pt.ResolvedVerifiers)
}

func Test_action_ScheduleResolveRetry_SchedulesTimerThatQueuesRetry(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Resolving).
		WithMockClock().
		WithResolveRetryBackoff(2 * time.Second)
	txn, mocks := builder.BuildWithMocks()

	var timerFn func()
	mocks.Clock.On("ScheduleTimer", mock.Anything, 2*time.Second, mock.Anything).
		Run(func(args mock.Arguments) {
			timerFn = args.Get(2).(func())
		}).
		Return(func() {}).Once()

	require.NoError(t, action_ScheduleResolveRetry(ctx, txn, nil))
	require.NotNil(t, txn.cancelResolveRetry, "a retry cancel func must be stored")
	require.NotNil(t, timerFn, "the scheduled timer func must be captured")

	// Firing the scheduled timer queues a retry event back to the originator.
	timerFn()
	event := <-mocks.Events
	_, ok := event.(*VerifierResolutionRetryEvent)
	assert.True(t, ok, "scheduled timer must queue a VerifierResolutionRetryEvent")
}

func Test_action_CancelResolveRetry_CancelsAndClears(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Resolving).Build()
	cancelled := false
	txn.cancelResolveRetry = func() { cancelled = true }

	require.NoError(t, action_CancelResolveRetry(ctx, txn, nil))

	assert.True(t, cancelled, "the pending retry timer must be cancelled")
	assert.Nil(t, txn.cancelResolveRetry, "the cancel func must be cleared")
}

func Test_action_CancelResolveRetry_NoopWhenNoTimerScheduled(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Resolving).Build()
	require.Nil(t, txn.cancelResolveRetry)
	// Must not panic when no timer was ever scheduled.
	require.NoError(t, action_CancelResolveRetry(ctx, txn, nil))
	assert.Nil(t, txn.cancelResolveRetry)
}

func Test_scheduleResolveRetry_ClearsPreviousScheduleBeforeRearming(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Resolving).WithMockClock()
	txn, mocks := builder.BuildWithMocks()

	firstCancelled := false
	txn.cancelResolveRetry = func() { firstCancelled = true }

	mocks.Clock.On("ScheduleTimer", mock.Anything, mock.Anything, mock.Anything).
		Return(func() {}).Once()

	txn.scheduleResolveRetry(ctx)
	assert.True(t, firstCancelled, "a previously scheduled retry must be cancelled before re-arming")
}
