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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_NudgePreDispatchRequest_NilPendingRequest_ReturnsError(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pendingPreDispatchRequest = nil

	err := action_NudgePreDispatchRequest(ctx, txn, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nudgePreDispatchRequest called with no pending request")
}

func Test_action_NudgePreDispatchRequest_WithPendingRequest_Success(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set up a real IdempotentRequest so Nudge can be called
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})

	err := action_NudgePreDispatchRequest(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_validator_MatchesPendingPreDispatchRequest_DispatchRequestApproved_Match(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	requestID := uuid.New()
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})
	// IdempotentRequest generates its own key; we need to match it
	requestID = txn.pendingPreDispatchRequest.IdempotencyKey()

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            requestID,
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.True(t, matched)
}

func Test_validator_MatchesPendingPreDispatchRequest_DispatchRequestApproved_NoMatch_WrongRequestID(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(), // different from pending request
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, matched)
}

func Test_validator_MatchesPendingPreDispatchRequest_DispatchRequestApproved_NilPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pendingPreDispatchRequest = nil

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(),
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, matched)
}

func Test_validator_MatchesPendingPreDispatchRequest_OtherEventType_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})

	// Pass a different event type (e.g. ConfirmedEvent)
	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, matched)
}

func Test_action_DispatchRequestRejected_ClearsPendingRequestAndTimers(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})
	txn.cancelRequestTimeoutSchedule = func() {}
	txn.cancelStateTimeoutSchedule = func() {}

	err := action_DispatchRequestRejected(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.pendingPreDispatchRequest)
	assert.Nil(t, txn.cancelRequestTimeoutSchedule)
	assert.Nil(t, txn.cancelStateTimeoutSchedule)
}

func Test_ConfirmingDispatch_Timeout_TransitionsToPooled_AndClearsPendingRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable)
	txn, mocks := builder.BuildWithMocks()
	require.NotNil(t, txn.pendingPreDispatchRequest)

	mocks.Clock.Advance(builder.GetStateTimeout() + 1)
	err := txn.HandleEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
	assert.Nil(t, txn.pendingPreDispatchRequest)
}

func Test_dispatchConfirmationStateTimeoutExceeded_NilPendingRequest_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pendingPreDispatchRequest = nil

	assert.False(t, txn.dispatchConfirmationStateTimeoutExceeded(ctx))
	assert.False(t, guard_DispatchConfirmationStateTimeoutExceeded(ctx, txn))
}

func Test_dispatchConfirmationStateTimeoutExceeded_NilFirstRequestTime_UsesStateEntryTime(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return errors.New("send failed")
	})
	_ = txn.pendingPreDispatchRequest.Nudge(ctx) // Keep FirstRequestTime nil

	assert.False(t, txn.dispatchConfirmationStateTimeoutExceeded(ctx))
	assert.False(t, guard_DispatchConfirmationStateTimeoutExceeded(ctx, txn))

	mocks.clock.Advance(6000) // greater than default state timeout (5000)
	assert.True(t, txn.dispatchConfirmationStateTimeoutExceeded(ctx))
	assert.True(t, guard_DispatchConfirmationStateTimeoutExceeded(ctx, txn))
}

func Test_hash_NilPrivateTransaction_ReturnsError(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt = nil

	hash, err := txn.hash(ctx)

	require.Error(t, err)
	assert.Nil(t, hash)
	assert.Contains(t, err.Error(), "Cannot hash transaction without PrivateTransaction")
}

func Test_sendPreDispatchRequest_RequestTimeoutSchedulesTimer_QueueEventCalled(t *testing.T) {
	ctx := context.Background()
	realClock := common.RealClock()
	grapher := NewGrapher(ctx)
	mockTransportWriter := transport.NewMockTransportWriter(t)
	mockEngineIntegration := common.NewMockEngineIntegration(t)
	mockSyncPoints := &syncpoints.MockSyncPoints{}

	txn, err := NewTransaction(
		ctx,
		fmt.Sprintf("%s@%s", uuid.NewString(), uuid.NewString()),
		&components.PrivateTransaction{
			ID:     uuid.New(),
			Domain: "test-domain",
			PreAssembly: &components.TransactionPreAssembly{
				TransactionSpecification: &prototk.TransactionSpecification{},
			},
			PostAssembly: &components.TransactionPostAssembly{
				Signatures: []*prototk.AttestationResult{},
			},
		},
		false,
		mockTransportWriter,
		realClock,
		func(ctx context.Context, event common.Event) {},
		mockEngineIntegration,
		mockSyncPoints,
		realClock.Duration(1), // Very short request timeout so timer fires quickly
		realClock.Duration(5000),
		5,
		"",
		prototk.ContractConfig_SUBMITTER_COORDINATOR,
		grapher,
		nil,
	)
	require.NoError(t, err)

	mockTransportWriter.EXPECT().SendPreDispatchRequest(
		ctx, txn.originatorNode, mock.Anything, txn.pt.PreAssembly.TransactionSpecification, mock.Anything,
	).Return(nil)

	timeoutEventReceived := false
	var mu sync.Mutex
	txn.queueEventForCoordinator = func(ctx context.Context, event common.Event) {
		if ev, ok := event.(*RequestTimeoutIntervalEvent); ok && ev.TransactionID == txn.pt.ID {
			mu.Lock()
			timeoutEventReceived = true
			mu.Unlock()
		}
	}

	err = txn.sendPreDispatchRequest(ctx)
	require.NoError(t, err)

	// Wait for the request timeout timer to fire (scheduled with 1ms duration)
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	assert.True(t, timeoutEventReceived, "queueEventForCoordinator should have been called with RequestTimeoutIntervalEvent")
	mu.Unlock()
}
