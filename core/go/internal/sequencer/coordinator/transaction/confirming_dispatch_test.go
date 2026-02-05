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
	"sync"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_NudgePreDispatchRequest_NilPendingRequest_ReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build(ctx)

	err := action_NudgePreDispatchRequest(ctx, txn, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nudgePreDispatchRequest called with no pending request")
}

func Test_action_NudgePreDispatchRequest_WithPendingRequest_Success(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest().
		Build(ctx)

	err := action_NudgePreDispatchRequest(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_validator_MatchesPendingPreDispatchRequest_DispatchRequestApproved_Match(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest().
		Build(ctx)

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            txn.pendingPreDispatchRequest.IdempotencyKey(),
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.True(t, matched)
}

func Test_validator_MatchesPendingPreDispatchRequest_DispatchRequestApproved_NoMatch_WrongRequestID(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest().
		Build(ctx)

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(), // different from pending request
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, matched)
}

func Test_validator_MatchesPendingPreDispatchRequest_DispatchRequestApproved_NilPendingRequest(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build(ctx)

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(),
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, matched)
}

func Test_validator_MatchesPendingPreDispatchRequest_OtherEventType_ReturnsFalse(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest().
		Build(ctx)

	// Pass a different event type (e.g. ConfirmedEvent)
	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, matched)
}

func Test_hash_NilPrivateTransaction_ReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build(ctx)
	txn.pt = nil

	hash, err := txn.hash(ctx)

	require.Error(t, err)
	assert.Nil(t, hash)
	assert.Contains(t, err.Error(), "Cannot hash transaction without PrivateTransaction")
}

func Test_sendPreDispatchRequest_RequestTimeoutSchedulesTimer_QueueEventCalled(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		RequestTimeout(1).
		TransportWriter(transport.NewMockTransportWriter(t)).
		Build(ctx)

	mocks.TransportWriter.EXPECT().SendPreDispatchRequest(
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

	err := txn.sendPreDispatchRequest(ctx)
	require.NoError(t, err)

	// Advance past request timeout (1ms)
	mocks.Clock.Advance(10)

	mu.Lock()
	assert.True(t, timeoutEventReceived, "queueEventForCoordinator should have been called with RequestTimeoutIntervalEvent")
	mu.Unlock()
}
