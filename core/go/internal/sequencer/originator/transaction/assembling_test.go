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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAction_AssembleAndSign_NoAssembleRequest(t *testing.T) {
	// Test that action_AssembleAndSign returns error when latestAssembleRequest is nil
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()

	// Explicitly set latestAssembleRequest to nil
	txn.latestAssembleRequest = nil

	// Execute the action
	err := action_AssembleAndSign(ctx, txn, nil)

	// Verify error is returned
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "No assemble request found")
}

func Test_handleAssembleAndSign_EngineIntegrationError(t *testing.T) {
	// Test that handleAssembleAndSign queues AssembleErrorEvent when AssembleAndSign fails
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set (should be set by builder for State_Assembling)
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")
	req := *txn.latestAssembleRequest

	// Set up PreAssembly
	preAssembly := &prototk.TransactionPreAssembly{}
	txn.pt.PreAssembly = preAssembly

	// Mock AssembleAndSign to return an error
	expectedError := errors.New("assembly failed")
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.pt.ID,
		preAssembly,
		mock.Anything,
		mock.Anything,
	).Return(nil, expectedError)

	// Execute the method
	txn.handleAssembleAndSign(ctx, txn.pt.ID, req, preAssembly)

	// Verify AssembleErrorEvent was emitted so coordinator can park or discard the transaction
	event := <-mocks.Events
	errorEvent, ok := event.(*AssembleErrorEvent)
	require.True(t, ok, "Event should be AssembleErrorEvent")
	assert.Equal(t, txn.pt.ID, errorEvent.TransactionID)
	assert.Equal(t, req.requestID, errorEvent.RequestID)
}

func Test_handleAssembleAndSign_Success_OK(t *testing.T) {
	// Test that handleAssembleAndSign emits AssembleAndSignSuccessEvent when AssembleAndSign returns OK
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")
	req := *txn.latestAssembleRequest

	// Set up PreAssembly
	preAssembly := &prototk.TransactionPreAssembly{}

	// Create expected assembly response with OK result
	expectedResponse := &prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	// Mock AssembleAndSign to return OK
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.pt.ID,
		preAssembly,
		mock.Anything,
		mock.Anything,
	).Return(expectedResponse, nil)

	// Execute the method
	txn.handleAssembleAndSign(ctx, txn.pt.ID, req, preAssembly)

	// Verify AssembleAndSignSuccessEvent was emitted
	event := <-mocks.Events

	successEvent, ok := event.(*AssembleAndSignSuccessEvent)
	require.True(t, ok, "Event should be AssembleAndSignSuccessEvent")
	assert.Equal(t, txn.pt.ID, successEvent.TransactionID)
	assert.Equal(t, req.requestID, successEvent.RequestID)
	assert.Equal(t, expectedResponse, successEvent.PostAssembly.AssembleResponse)
}

func Test_handleAssembleAndSign_Success_REVERT(t *testing.T) {
	// Test that handleAssembleAndSign emits AssembleRevertEvent when AssembleAndSign returns REVERT
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")
	req := *txn.latestAssembleRequest

	// Set up PreAssembly
	preAssembly := &prototk.TransactionPreAssembly{}

	// Create expected assembly response with REVERT result
	revertReason := "transaction reverted"
	expectedResponse := &prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   &revertReason,
	}

	// Mock AssembleAndSign to return REVERT
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.pt.ID,
		preAssembly,
		mock.Anything,
		mock.Anything,
	).Return(expectedResponse, nil)

	// Execute the method
	txn.handleAssembleAndSign(ctx, txn.pt.ID, req, preAssembly)

	// Verify AssembleRevertEvent was emitted
	event := <-mocks.Events

	revertEvent, ok := event.(*AssembleRevertEvent)
	require.True(t, ok, "Event should be AssembleRevertEvent")
	assert.Equal(t, txn.pt.ID, revertEvent.TransactionID)
	assert.Equal(t, req.requestID, revertEvent.RequestID)
	assert.Equal(t, expectedResponse, revertEvent.PostAssembly.AssembleResponse)
}

func Test_handleAssembleAndSign_PARK(t *testing.T) {
	// Test that handleAssembleAndSign emits AssembleParkEvent when AssembleAndSign returns PARK
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")
	req := *txn.latestAssembleRequest

	// Set up PreAssembly
	preAssembly := &prototk.TransactionPreAssembly{}

	// Create expected assembly response with PARK result
	expectedResponse := &prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_PARK,
	}

	// Mock AssembleAndSign to return PARK
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.pt.ID,
		preAssembly,
		mock.Anything,
		mock.Anything,
	).Return(expectedResponse, nil)

	// Execute the method
	txn.handleAssembleAndSign(ctx, txn.pt.ID, req, preAssembly)

	// Verify AssembleParkEvent was emitted
	event := <-mocks.Events

	parkEvent, ok := event.(*AssembleParkEvent)
	require.True(t, ok, "Event should be AssembleParkEvent")
	assert.Equal(t, txn.pt.ID, parkEvent.TransactionID)
	assert.Equal(t, req.requestID, parkEvent.RequestID)
	assert.Equal(t, expectedResponse, parkEvent.PostAssembly.AssembleResponse)
}

func Test_handleAssembleAndSign_CalledWithCorrectParameters(t *testing.T) {
	// Test that AssembleAndSign is called with correct parameters
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")
	req := *txn.latestAssembleRequest

	// Set up PreAssembly
	preAssembly := &prototk.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txn.pt.ID.String(),
		},
	}

	// Create expected assembly response
	expectedResponse := &prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	// Mock AssembleAndSign with specific parameter expectations
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		ctx,
		txn.pt.ID,
		preAssembly,
		req.stateSnapshot,
		req.coordinatorsBlockHeight,
	).Return(expectedResponse, nil)

	// Execute the method
	txn.handleAssembleAndSign(ctx, txn.pt.ID, req, preAssembly)

	// Verify AssembleAndSign was called with correct parameters
	mocks.EngineIntegration.AssertExpectations(t)

	// Verify event was emitted with correct request ID
	event := <-mocks.Events
	successEvent, ok := event.(*AssembleAndSignSuccessEvent)
	require.True(t, ok, "Event should be AssembleAndSignSuccessEvent")
	assert.Equal(t, req.requestID, successEvent.RequestID)
}

func Test_action_AssembleAndSign_SpawnsGoroutineThatQueuesEvent(t *testing.T) {
	ctx := t.Context()

	done := make(chan struct{})
	builder := NewTransactionBuilderForTesting(t, State_Assembling).
		QueueEventsTo(func(_ context.Context, _ common.Event) { close(done) })
	txn := builder.Build()

	builder.fakeEngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.pt.ID,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(&prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}, nil)

	err := action_AssembleAndSign(ctx, txn, nil)
	require.NoError(t, err)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for assemble goroutine to queue event")
	}
}

func Test_action_AssembleRequestReceived_SetsDelegateAndLatestRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()
	requestID := uuid.New()
	coordinator := "coord@node1"
	event := &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.pt.ID},
		RequestID:              requestID,
		Coordinator:            coordinator,
		CoordinatorBlockHeight: 100,
		StateSnapshot:          &prototk.StateSnapshot{},
	}
	err := action_AssembleRequestReceived(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, coordinator, txn.currentDelegate)
	require.NotNil(t, txn.latestAssembleRequest)
	assert.Equal(t, requestID, txn.latestAssembleRequest.requestID)
	assert.Equal(t, int64(100), txn.latestAssembleRequest.coordinatorsBlockHeight)
}

func Test_action_AssembleAndSignSuccess_SetsPostAssemblyAndRequestID(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()
	requestID := uuid.New()
	postAssembly := &components.TransactionPostAssembly{AssembleResponse: &prototk.TransactionPostAssembly{AssemblyResult: prototk.AssembleTransactionResponse_OK}}
	event := &AssembleAndSignSuccessEvent{
		BaseEvent:    BaseEvent{TransactionID: txn.pt.ID},
		RequestID:    requestID,
		PostAssembly: postAssembly,
	}
	err := action_AssembleAndSignSuccess(ctx, txn, event)
	require.NoError(t, err)
	assert.Same(t, postAssembly, txn.pt.PostAssembly)
	assert.Equal(t, requestID, txn.latestFulfilledAssembleRequestID)
}

func Test_action_AssembleRevert_SetsPostAssemblyAndRequestID(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()
	requestID := uuid.New()
	postAssembly := &components.TransactionPostAssembly{AssembleResponse: &prototk.TransactionPostAssembly{AssemblyResult: prototk.AssembleTransactionResponse_REVERT}}
	event := &AssembleRevertEvent{
		BaseEvent:    BaseEvent{TransactionID: txn.pt.ID},
		RequestID:    requestID,
		PostAssembly: postAssembly,
	}
	err := action_AssembleRevert(ctx, txn, event)
	require.NoError(t, err)
	assert.Same(t, postAssembly, txn.pt.PostAssembly)
	assert.Equal(t, requestID, txn.latestFulfilledAssembleRequestID)
}

func Test_action_AssemblePark_SetsPostAssemblyAndRequestID(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()
	requestID := uuid.New()
	postAssembly := &components.TransactionPostAssembly{AssembleResponse: &prototk.TransactionPostAssembly{AssemblyResult: prototk.AssembleTransactionResponse_PARK}}
	event := &AssembleParkEvent{
		BaseEvent:    BaseEvent{TransactionID: txn.pt.ID},
		RequestID:    requestID,
		PostAssembly: postAssembly,
	}
	err := action_AssemblePark(ctx, txn, event)
	require.NoError(t, err)
	assert.Same(t, postAssembly, txn.pt.PostAssembly)
	assert.Equal(t, requestID, txn.latestFulfilledAssembleRequestID)
}

func Test_action_AssembleError_SetsLatestFulfilledAssembleRequestID(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()
	requestID := uuid.New()
	event := &AssembleErrorEvent{
		BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
		RequestID: requestID,
	}
	err := action_AssembleError(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, requestID, txn.latestFulfilledAssembleRequestID)
}

func Test_action_SendAssembleError_Success(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	mocks.SentMessageRecorder.Reset(ctx)

	err := action_SendAssembleError(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleError(), "SendAssembleError should have been called")
}

func Test_action_SendAssembleError_TransportError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).WithMockTransportWriter()
	txn, mocks := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	expectedError := errors.New("transport error")
	mocks.TransportWriter.EXPECT().SendAssembleError(
		mock.Anything,
		coordinator,
		mock.MatchedBy(func(msg *engineProto.AssembleError) bool {
			return msg.TransactionId == txn.GetID().String() && msg.AssembleRequestId == requestID.String()
		}),
	).Return(expectedError)

	err := action_SendAssembleError(ctx, txn, nil)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func Test_handleAssembleAndSign_AbandonsSilently_WhenContextExpired(t *testing.T) {
	// When the context is already expired (deadline elapsed), handleAssembleAndSign must not
	// queue any event back to the originator.  The coordinator that sent the request will have
	// timed out and discarded it, so sending an error event would be spurious.
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	require.NotNil(t, txn.latestAssembleRequest)
	req := *txn.latestAssembleRequest
	preAssembly := &prototk.TransactionPreAssembly{}

	// Context that is already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Mock AssembleAndSign to return an error (as would happen when ctx is cancelled)
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.pt.ID,
		preAssembly,
		mock.Anything,
		mock.Anything,
	).Return(nil, context.Canceled)

	txn.handleAssembleAndSign(ctx, txn.pt.ID, req, preAssembly)

	// No event should have been queued — the mock will fail if any unexpected call happens
	select {
	case e := <-mocks.Events:
		t.Fatalf("unexpected event queued when context was cancelled: %T", e)
	default:
	}
}

func Test_action_AssembleAndSign_UsesDeadlineContext_WhenExpirySet(t *testing.T) {
	// When the assemble request carries a non-zero expiry, action_AssembleAndSign must pass a
	// context with that deadline to the goroutine.  Verify this by setting an already-elapsed
	// expiry: the goroutine's AssembleAndSign call will see a cancelled context.
	ctx := t.Context()

	expiry := time.Now().Add(-time.Second) // already expired

	done := make(chan struct{})
	builder := NewTransactionBuilderForTesting(t, State_Assembling).
		QueueEventsTo(func(_ context.Context, _ common.Event) { close(done) })
	txn := builder.Build()
	txn.latestAssembleRequest.expiry = expiry

	builder.fakeEngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.pt.ID,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil, context.DeadlineExceeded)

	err := action_AssembleAndSign(ctx, txn, nil)
	require.NoError(t, err)

	// The goroutine should exit without queuing any event (no close(done) call).
	select {
	case <-done:
		t.Fatal("unexpected event queued when context was already expired")
	case <-time.After(200 * time.Millisecond):
		// Expected: goroutine completed silently
	}
}

func Test_validator_IsPrivateStateDataPendingForAssembly_Complete_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Delegated).BuildWithMocks() // default: checkStateComplete=true

	event := &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.pt.ID},
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10,
	}
	result, err := validator_IsPrivateStateDataPendingForAssembly(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, result)
}

func Test_validator_IsPrivateStateDataPendingForAssembly_Incomplete_ReturnsTrue(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Delegated).
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	event := &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.pt.ID},
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10,
	}
	result, err := validator_IsPrivateStateDataPendingForAssembly(ctx, txn, event)
	require.NoError(t, err)
	assert.True(t, result)
}

func Test_validator_IsPrivateStateDataPendingForAssembly_Error_Propagates(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("db error")
	txn, _ := NewTransactionBuilderForTesting(t, State_Delegated).
		WithCheckPendingPrivateStateDataError(dbErr).
		BuildWithMocks()

	event := &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.pt.ID},
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10,
	}
	_, err := validator_IsPrivateStateDataPendingForAssembly(ctx, txn, event)
	assert.ErrorIs(t, err, dbErr)
}

func Test_action_AssembleAndSign_NilCancelIsNoOp(t *testing.T) {
	// Calling action_AssembleAndSign when cancelCurrentAssembly is nil (first call) must not
	// panic and must start the goroutine normally.
	ctx := t.Context()

	done := make(chan struct{})
	builder := NewTransactionBuilderForTesting(t, State_Assembling).
		QueueEventsTo(func(_ context.Context, _ common.Event) { close(done) })
	txn := builder.Build()

	txn.cancelCurrentAssembly = nil // explicit nil to document intent

	builder.fakeEngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, txn.pt.ID, mock.Anything, mock.Anything, mock.Anything,
	).Return(&prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}, nil)

	err := action_AssembleAndSign(ctx, txn, nil)
	require.NoError(t, err)

	// cancelCurrentAssembly and currentAssemblyRequestID must be populated after the call
	assert.NotNil(t, txn.cancelCurrentAssembly)
	assert.Equal(t, txn.latestAssembleRequest.requestID, txn.currentAssemblyRequestID)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for assemble goroutine")
	}
}

func Test_action_AssembleAndSign_CancelsPreviousGoroutine(t *testing.T) {
	// A second call to action_AssembleAndSign must cancel the first goroutine's context so
	// that the stale goroutine exits without queuing an event. Only the second goroutine's
	// success event should appear.
	ctx := t.Context()

	eventCh := make(chan common.Event, 2) // buffer 2 so a spurious second event is detectable
	builder := NewTransactionBuilderForTesting(t, State_Assembling).
		QueueEventsTo(func(_ context.Context, e common.Event) { eventCh <- e })
	txn := builder.Build()

	firstReqID := txn.latestAssembleRequest.requestID

	// firstGoroutineDone is closed once the first goroutine's AssembleAndSign observes
	// context cancellation and the mock call returns.
	firstGoroutineDone := make(chan struct{})
	blocked := make(chan struct{})

	builder.fakeEngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, txn.pt.ID, mock.Anything, mock.Anything, mock.Anything,
	).Once().Run(func(args mock.Arguments) {
		assembleCtx := args.Get(0).(context.Context)
		close(blocked)
		<-assembleCtx.Done() // block until action_AssembleAndSign cancels us
		close(firstGoroutineDone)
	}).Return(nil, context.Canceled)

	secondReqID := uuid.New()

	builder.fakeEngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, txn.pt.ID, mock.Anything, mock.Anything, mock.Anything,
	).Once().Return(&prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}, nil)

	err := action_AssembleAndSign(ctx, txn, nil)
	require.NoError(t, err)

	// Wait until the first goroutine is actually blocked inside AssembleAndSign
	<-blocked

	// Spawn second goroutine with a new request ID — this must cancel the first
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{requestID: secondReqID}
	err = action_AssembleAndSign(ctx, txn, nil)
	require.NoError(t, err)

	// Wait for first goroutine to observe cancellation (AssembleAndSign returned)
	<-firstGoroutineDone

	// The only event in the channel must be from the second goroutine
	event := <-eventCh
	successEvent, ok := event.(*AssembleAndSignSuccessEvent)
	require.True(t, ok, "expected AssembleAndSignSuccessEvent from second goroutine, got %T", event)
	assert.Equal(t, secondReqID, successEvent.RequestID)
	assert.NotEqual(t, firstReqID, successEvent.RequestID, "event must not be from the cancelled first goroutine")

	// No second event should have been queued
	select {
	case e := <-eventCh:
		t.Fatalf("unexpected second event queued: %T", e)
	default:
	}
}

func Test_action_AssembleAndSign_SetsCurrentAssemblyRequestID(t *testing.T) {
	// action_AssembleAndSign must record the in-flight request ID so that a coordinator nudge
	// carrying the same idempotency key can be detected by guard_AssembleRequestMatchesInProgressAssembly.
	ctx := t.Context()

	done := make(chan struct{})
	builder := NewTransactionBuilderForTesting(t, State_Assembling).
		QueueEventsTo(func(_ context.Context, _ common.Event) { close(done) })
	txn := builder.Build()

	expectedRequestID := txn.latestAssembleRequest.requestID

	builder.fakeEngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, txn.pt.ID, mock.Anything, mock.Anything, mock.Anything,
	).Return(&prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}, nil)

	err := action_AssembleAndSign(ctx, txn, nil)
	require.NoError(t, err)

	assert.Equal(t, expectedRequestID, txn.currentAssemblyRequestID)

	<-done
}

func Test_action_AssembleAndSign_NudgeDoesNotCancelInFlightAssembly(t *testing.T) {
	// When a coordinator nudge arrives (same idempotency key) while the originator is still
	// assembling, guard_AssembleRequestMatchesInProgressAssembly must return true so that the state
	// machine skips action_AssembleAndSign.  This test verifies the guard directly and also
	// confirms that the in-flight goroutine is not interrupted.
	ctx := t.Context()

	eventCh := make(chan common.Event, 2)
	builder := NewTransactionBuilderForTesting(t, State_Assembling).
		QueueEventsTo(func(_ context.Context, e common.Event) { eventCh <- e })
	txn := builder.Build()

	firstReqID := txn.latestAssembleRequest.requestID

	blocked := make(chan struct{})
	unblock := make(chan struct{})

	var firstCancelCalled bool
	builder.fakeEngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, txn.pt.ID, mock.Anything, mock.Anything, mock.Anything,
	).Once().Run(func(_ mock.Arguments) {
		close(blocked)
		<-unblock
	}).Return(&prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}, nil)

	// Start first goroutine
	err := action_AssembleAndSign(ctx, txn, nil)
	require.NoError(t, err)

	// Wrap the real cancel so we can detect if it gets called prematurely
	realCancel := txn.cancelCurrentAssembly
	txn.cancelCurrentAssembly = func() {
		firstCancelCalled = true
		realCancel()
	}

	// Wait until the goroutine is inside AssembleAndSign
	<-blocked

	// The guard must recognise this as a nudge (same request ID, cancel func set)
	assert.True(t, guard_AssembleRequestMatchesInProgressAssembly(ctx, txn),
		"guard must return true for a nudge with the same request ID while assembly is in flight")
	assert.Equal(t, firstReqID, txn.currentAssemblyRequestID,
		"currentAssemblyRequestID must not have changed")

	// The original goroutine must NOT have been cancelled by the nudge
	assert.False(t, firstCancelCalled, "in-flight assembly must not be cancelled by a nudge")

	// Let the first goroutine finish normally
	close(unblock)

	event := <-eventCh
	successEvent, ok := event.(*AssembleAndSignSuccessEvent)
	require.True(t, ok)
	assert.Equal(t, firstReqID, successEvent.RequestID)

	// No second event
	select {
	case e := <-eventCh:
		t.Fatalf("unexpected second event: %T", e)
	default:
	}
}

func Test_validator_AssembleAndSignSuccessMatchesCurrentRequest_Match(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()

	requestID := txn.latestAssembleRequest.requestID
	event := &AssembleAndSignSuccessEvent{
		BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
		RequestID: requestID,
	}
	ok, err := validator_AssembleAndSignSuccessMatchesCurrentRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.True(t, ok)
}

func Test_validator_AssembleAndSignSuccessMatchesCurrentRequest_Mismatch(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()

	event := &AssembleAndSignSuccessEvent{
		BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
		RequestID: uuid.New(), // different ID
	}
	ok, err := validator_AssembleAndSignSuccessMatchesCurrentRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, ok)
}

func Test_validator_AssembleAndSignSuccessMatchesCurrentRequest_NilRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()

	txn.latestAssembleRequest = nil
	event := &AssembleAndSignSuccessEvent{
		BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
		RequestID: uuid.New(),
	}
	ok, err := validator_AssembleAndSignSuccessMatchesCurrentRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, ok)
}

func Test_action_RejectAssemblyPrivateStateDataPending_SendsRejection(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Delegated).BuildWithMocks()

	coordinator := txn.currentDelegate
	requestID := uuid.New()
	event := &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.pt.ID},
		RequestID:              requestID,
		Coordinator:            coordinator,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10,
	}

	err := action_RejectAssemblyPrivateStateDataPending(ctx, txn, event)
	require.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "expected assemble rejection to be sent")
}

func TestGuard_AssembleRequestMatchesCurrentAssembly_Matches(t *testing.T) {
	// Returns true when the latest request ID matches the in-flight assembly request ID and a cancel func is set.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()

	requestID := uuid.New()
	txn.currentAssemblyRequestID = requestID
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{requestID: requestID}
	txn.cancelCurrentAssembly = func() {}

	assert.True(t, guard_AssembleRequestMatchesInProgressAssembly(ctx, txn))
}

func TestGuard_AssembleRequestMatchesCurrentAssembly_DoesNotMatch(t *testing.T) {
	// Returns false when the latest request ID differs from the in-flight one.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()

	txn.currentAssemblyRequestID = uuid.New()
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{requestID: uuid.New()}
	txn.cancelCurrentAssembly = func() {}

	assert.False(t, guard_AssembleRequestMatchesInProgressAssembly(ctx, txn))
}

func TestGuard_AssembleRequestMatchesCurrentAssembly_NoCancelFunc(t *testing.T) {
	// Returns false when there is no in-flight goroutine (cancelCurrentAssembly is nil), even if
	// the request IDs happen to match, because there is nothing currently being assembled.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()

	requestID := uuid.New()
	txn.currentAssemblyRequestID = requestID
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{requestID: requestID}
	txn.cancelCurrentAssembly = nil

	assert.False(t, guard_AssembleRequestMatchesInProgressAssembly(ctx, txn))
}
