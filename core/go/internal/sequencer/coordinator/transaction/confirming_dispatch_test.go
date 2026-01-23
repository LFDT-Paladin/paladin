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

func Test_applyDispatchConfirmation(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Create a pending request first
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})

	requestID := uuid.New()
	err := txn.applyDispatchConfirmation(ctx, requestID)

	assert.NoError(t, err)
	assert.Nil(t, txn.pendingPreDispatchRequest, "pendingPreDispatchRequest should be cleared")
}

func Test_sendPreDispatchRequest_FirstTime_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	// Set up PreAssembly and PostAssembly so Hash() works
	txn.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	postAssembly := &components.TransactionPostAssembly{
		Signatures: []*prototk.AttestationResult{
			{Payload: []byte("test-signature")},
		},
	}
	// Mock WriteLockStatesForTransaction which is called by applyPostAssembly
	mocks.engineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.PrivateTransaction).Return(nil)
	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)

	// Mock the transport writer
	mocks.transportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil)

	err = txn.sendPreDispatchRequest(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, txn.pendingPreDispatchRequest, "pendingPreDispatchRequest should be created")
	assert.NotNil(t, txn.cancelDispatchConfirmationRequestTimeoutSchedule, "timeout schedule should be set")
}

func Test_sendPreDispatchRequest_FirstTime_HashError(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set up PreAssembly but no PostAssembly so Hash() fails
	txn.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	txn.PostAssembly = nil

	err := txn.sendPreDispatchRequest(ctx)

	assert.Error(t, err)
	assert.Nil(t, txn.pendingPreDispatchRequest, "pendingPreDispatchRequest should not be created on error")
}

func Test_sendPreDispatchRequest_FirstTime_TransportError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	// Set up PreAssembly and PostAssembly so Hash() works
	txn.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	postAssembly := &components.TransactionPostAssembly{
		Signatures: []*prototk.AttestationResult{
			{Payload: []byte("test-signature")},
		},
	}
	// Mock WriteLockStatesForTransaction which is called by applyPostAssembly
	mocks.engineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.PrivateTransaction).Return(nil)
	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)

	// Mock the transport writer to return an error
	transportError := errors.New("transport error")
	mocks.transportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(transportError)

	err = txn.sendPreDispatchRequest(ctx)

	assert.Error(t, err)
	assert.Equal(t, transportError, err)
	assert.NotNil(t, txn.pendingPreDispatchRequest, "pendingPreDispatchRequest should still be created even on error")
}

func Test_sendPreDispatchRequest_SubsequentCall_NudgesExistingRequest(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	// Set up PreAssembly and PostAssembly so Hash() works
	txn.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	postAssembly := &components.TransactionPostAssembly{
		Signatures: []*prototk.AttestationResult{
			{Payload: []byte("test-signature")},
		},
	}
	// Mock WriteLockStatesForTransaction which is called by applyPostAssembly
	mocks.engineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.PrivateTransaction).Return(nil)
	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)

	// Create a pending request first
	mocks.transportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil).Once()

	err = txn.sendPreDispatchRequest(ctx)
	require.NoError(t, err)
	require.NotNil(t, txn.pendingPreDispatchRequest)

	// Now call it again - should just nudge, not create a new request
	// The nudge will call SendPreDispatchRequest again if timeout expired
	mocks.transportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil).Once()

	// Advance time so the request has expired and will be resent
	// requestTimeout is 1000ms from newTransactionForUnitTesting
	mocks.clock.Advance(1001)

	err = txn.sendPreDispatchRequest(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, txn.pendingPreDispatchRequest, "pendingPreDispatchRequest should still exist")
}

func Test_sendPreDispatchRequest_TimeoutHandler_Called(t *testing.T) {
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
			ID: uuid.New(),
		},
		mockTransportWriter,
		realClock,
		func(ctx context.Context, event common.Event) error {
			return nil
		},
		mockEngineIntegration,
		mockSyncPoints,
		realClock.Duration(1), // Very short timeout for testing
		realClock.Duration(5000),
		5,
		"",
		prototk.ContractConfig_SUBMITTER_COORDINATOR,
		grapher,
		nil,
		func(context.Context, *Transaction) {},
		func(context.Context, *Transaction) {},
		func(ctx context.Context, txn *Transaction, from, to State) {},
		func(context.Context) {},
	)
	require.NoError(t, err)

	// Set up PreAssembly and PostAssembly so Hash() works
	txn.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	postAssembly := &components.TransactionPostAssembly{
		Signatures: []*prototk.AttestationResult{
			{Payload: []byte("test-signature")},
		},
	}
	// Mock WriteLockStatesForTransaction which is called by applyPostAssembly
	mockEngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.PrivateTransaction).Return(nil)
	err = txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)

	// Mock the transport writer
	mockTransportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil)

	// Set up event handler to track if RequestTimeoutIntervalEvent is received
	timeoutEventReceived := false
	var mu sync.Mutex
	txn.eventHandler = func(ctx context.Context, event common.Event) error {
		if _, ok := event.(*RequestTimeoutIntervalEvent); ok {
			mu.Lock()
			timeoutEventReceived = true
			mu.Unlock()
		}
		return nil
	}

	err = txn.sendPreDispatchRequest(ctx)
	require.NoError(t, err)

	// Wait for timeout to fire
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	assert.True(t, timeoutEventReceived, "RequestTimeoutIntervalEvent should be received")
	mu.Unlock()
}

func Test_sendPreDispatchRequest_TimeoutHandler_Error(t *testing.T) {
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
			ID: uuid.New(),
		},
		mockTransportWriter,
		realClock,
		func(ctx context.Context, event common.Event) error {
			return nil
		},
		mockEngineIntegration,
		mockSyncPoints,
		realClock.Duration(1), // Very short timeout for testing
		realClock.Duration(5000),
		5,
		"",
		prototk.ContractConfig_SUBMITTER_COORDINATOR,
		grapher,
		nil,
		func(context.Context, *Transaction) {},
		func(context.Context, *Transaction) {},
		func(ctx context.Context, txn *Transaction, from, to State) {},
		func(context.Context) {},
	)
	require.NoError(t, err)

	// Set up PreAssembly and PostAssembly so Hash() works
	txn.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	postAssembly := &components.TransactionPostAssembly{
		Signatures: []*prototk.AttestationResult{
			{Payload: []byte("test-signature")},
		},
	}
	// Mock WriteLockStatesForTransaction which is called by applyPostAssembly
	mockEngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.PrivateTransaction).Return(nil)
	err = txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)

	// Mock the transport writer
	mockTransportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil)

	// Set up event handler to return an error
	handlerError := errors.New("handler error")
	errorLogged := false
	txn.eventHandler = func(ctx context.Context, event common.Event) error {
		if _, ok := event.(*RequestTimeoutIntervalEvent); ok {
			errorLogged = true
			return handlerError
		}
		return nil
	}

	err = txn.sendPreDispatchRequest(ctx)
	require.NoError(t, err)

	// Wait for timeout to fire
	time.Sleep(10 * time.Millisecond)

	assert.True(t, errorLogged, "handler error should be logged")
}

func Test_nudgePreDispatchRequest_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	// Set up PreAssembly and PostAssembly so Hash() works
	txn.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	postAssembly := &components.TransactionPostAssembly{
		Signatures: []*prototk.AttestationResult{
			{Payload: []byte("test-signature")},
		},
	}
	// Mock WriteLockStatesForTransaction which is called by applyPostAssembly
	mocks.engineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.PrivateTransaction).Return(nil)
	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)

	// Create a pending request first
	mocks.transportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil)

	err = txn.sendPreDispatchRequest(ctx)
	require.NoError(t, err)
	require.NotNil(t, txn.pendingPreDispatchRequest)

	// Now nudge it - will call SendPreDispatchRequest again if timeout expired
	mocks.transportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil)

	// Advance time so the request has expired and will be resent
	// requestTimeout is 1000ms from newTransactionForUnitTesting
	mocks.clock.Advance(1001)

	err = txn.nudgePreDispatchRequest(ctx)

	assert.NoError(t, err)
}

func Test_nudgePreDispatchRequest_NoPendingRequest_Error(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	txn.pendingPreDispatchRequest = nil

	err := txn.nudgePreDispatchRequest(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nudgePreDispatchRequest called with no pending request")
}

func Test_validator_MatchesPendingPreDispatchRequest_MatchingRequestID(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Create a pending request
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})

	// Override the idempotency key to match our test
	// We can't directly set it, but we can verify it matches
	actualKey := txn.pendingPreDispatchRequest.IdempotencyKey()

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: actualKey,
	}

	matches, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)

	assert.NoError(t, err)
	assert.True(t, matches, "should match when request ID matches")
}

func Test_validator_MatchesPendingPreDispatchRequest_NonMatchingRequestID(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Create a pending request
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})

	// Use a different request ID
	differentRequestID := uuid.New()
	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: differentRequestID,
	}

	matches, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "should not match when request ID does not match")
}

func Test_validator_MatchesPendingPreDispatchRequest_NoPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	txn.pendingPreDispatchRequest = nil

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: uuid.New(),
	}

	matches, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "should not match when there is no pending request")
}

func Test_validator_MatchesPendingPreDispatchRequest_NonDispatchRequestApprovedEvent(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Create a pending request
	txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})

	// Use a different event type
	event := &common.HeartbeatIntervalEvent{}

	matches, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "should not match for non-DispatchRequestApprovedEvent")
}

func Test_action_SendPreDispatchRequest(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	// Set up PreAssembly and PostAssembly so Hash() works
	txn.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	postAssembly := &components.TransactionPostAssembly{
		Signatures: []*prototk.AttestationResult{
			{Payload: []byte("test-signature")},
		},
	}
	// Mock WriteLockStatesForTransaction which is called by applyPostAssembly
	mocks.engineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.PrivateTransaction).Return(nil)
	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)

	// Mock the transport writer
	mocks.transportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil)

	err = action_SendPreDispatchRequest(ctx, txn)

	assert.NoError(t, err)
	assert.NotNil(t, txn.pendingPreDispatchRequest, "pendingPreDispatchRequest should be created")
}

func Test_action_NudgePreDispatchRequest(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	// Set up PreAssembly and PostAssembly so Hash() works
	txn.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	postAssembly := &components.TransactionPostAssembly{
		Signatures: []*prototk.AttestationResult{
			{Payload: []byte("test-signature")},
		},
	}
	// Mock WriteLockStatesForTransaction which is called by applyPostAssembly
	mocks.engineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.PrivateTransaction).Return(nil)
	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)

	// Create a pending request first
	mocks.transportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil)

	err = txn.sendPreDispatchRequest(ctx)
	require.NoError(t, err)
	require.NotNil(t, txn.pendingPreDispatchRequest)

	// Now test the action wrapper
	// Advance time so the request has expired and will be resent
	// requestTimeout is 1000ms from newTransactionForUnitTesting
	mocks.clock.Advance(1001)

	mocks.transportWriter.EXPECT().SendPreDispatchRequest(
		ctx,
		txn.originatorNode,
		mock.AnythingOfType("uuid.UUID"),
		txn.PreAssembly.TransactionSpecification,
		mock.AnythingOfType("*pldtypes.Bytes32"),
	).Return(nil)

	err = action_NudgePreDispatchRequest(ctx, txn)

	assert.NoError(t, err)
}
