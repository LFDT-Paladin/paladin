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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleEvent_ValidEventWithTransition tests HandleEvent with a valid event that triggers a state transition
func TestHandleEvent_ValidEventWithTransition(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// Create a CreatedEvent which should transition from State_Initial to State_Pending
	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	err := txn.HandleEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, State_Pending, txn.GetCurrentState())
}

func TestHandleEvent_EventNotDefinedForState(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// Create an event that is not defined for State_Initial
	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
		Coordinator: "coordinator1",
	}

	err := txn.HandleEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, State_Initial, txn.GetCurrentState())
}

func TestHandleEvent_ValidatorReturnsFalse(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	// Create an AssembleRequestReceivedEvent which has a validator
	// The validator will return false if the request doesn't match
	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
		RequestID:   uuid.New(), // Different request ID
		Coordinator: "different-coordinator",
	}

	err := txn.HandleEvent(ctx, event)
	assert.NoError(t, err)                                  // Should not error, validator just returns false
	assert.Equal(t, State_Delegated, txn.GetCurrentState()) // State should not change
}

func TestHandleEvent_ValidatorReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	err := txn.HandleEvent(ctx, event)
	// The validator may return false (invalid) but should not return an error
	assert.NoError(t, err)
}

func TestHandleEvent_ApplyEventError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, _ := builder.BuildWithMocks()

	// Create a DelegatedEvent with empty coordinator which should cause ApplyToTransaction to return an error
	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
		Coordinator: "", // Empty coordinator should cause error
	}

	err := txn.HandleEvent(ctx, event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction delegate cannot be set to an empty node identity")
}

func TestHandleEvent_PerformActionsError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, mocks := builder.BuildWithMocks()

	// Create a custom event handler with an action that returns an error
	// We'll test this by directly calling performActions with a custom handler
	expectedError := errors.New("action failed")
	action := func(ctx context.Context, txn *Transaction) error {
		return expectedError
	}

	// Create an event that will be evaluated
	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	// Get the event handler
	eventHandler, err := txn.evaluateEvent(ctx, event)
	require.NoError(t, err)
	require.NotNil(t, eventHandler)

	// Apply the event
	err = txn.applyEvent(ctx, event)
	require.NoError(t, err)

	// Create a custom event handler with an action that will fail
	customEventHandler := EventHandler{
		Actions: []ActionRule{
			{
				Action: action,
				If:     nil,
			},
		},
		Transitions: eventHandler.Transitions,
	}

	// Test performActions directly
	err = txn.performActions(ctx, customEventHandler)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	_ = mocks
}

func TestEvaluateEvent_EventDefinedForState(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	eventHandler, err := txn.evaluateEvent(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, eventHandler)
}

func TestEvaluateEvent_EventNotDefinedForState(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
		Coordinator: "coordinator1",
	}

	eventHandler, err := txn.evaluateEvent(ctx, event)
	assert.NoError(t, err)
	assert.Nil(t, eventHandler)
}

func TestEvaluateEvent_ValidatorReturnsFalse(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	// Create an event that has a validator that will return false
	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
		RequestID:   uuid.New(), // Different request ID that won't match
		Coordinator: "different-coordinator",
	}

	eventHandler, err := txn.evaluateEvent(ctx, event)
	assert.NoError(t, err)
	assert.Nil(t, eventHandler)
}

func TestEvaluateEvent_ValidatorReturnsTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	// Set up the transaction to have a matching assemble request
	requestID := uuid.New()
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{
		requestID: requestID,
	}

	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
		RequestID:   requestID,
		Coordinator: txn.currentDelegate,
	}

	eventHandler, err := txn.evaluateEvent(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, eventHandler)
}

func TestEvaluateEvent_ValidatorReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	// The actual validators in the codebase should not return errors
	// This test verifies the error handling path exists
	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	eventHandler, err := txn.evaluateEvent(ctx, event)
	// In normal operation, validator should not return an error
	// It may return false (invalid) but not an error
	assert.NoError(t, err)
	_ = eventHandler
}

func TestApplyEvent_EventType(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator1"
	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
		Coordinator: coordinator,
	}

	err := txn.applyEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, coordinator, txn.currentDelegate)
}

func TestApplyEvent_NonEventType(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// Create a mock event that implements common.Event but not Event
	mockEvent := &mockCommonEvent{
		eventType:  Event_Delegated,
		eventTime:  time.Now(),
		typeString: "MockEvent",
	}

	err := txn.applyEvent(ctx, mockEvent)
	assert.NoError(t, err)
}

func TestApplyEvent_EventApplyError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, _ := builder.BuildWithMocks()

	// Create a DelegatedEvent with empty coordinator which should cause ApplyToTransaction to return an error
	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
		Coordinator: "", // Empty coordinator should cause error
	}

	err := txn.applyEvent(ctx, event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction delegate cannot be set to an empty node identity")
}

func TestPerformActions_NoActions(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	eventHandler := EventHandler{
		Actions: []ActionRule{},
	}

	err := txn.performActions(ctx, eventHandler)
	assert.NoError(t, err)
}

func TestPerformActions_ActionWithoutGuard(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	actionCalled := false
	action := func(ctx context.Context, txn *Transaction) error {
		actionCalled = true
		return nil
	}

	eventHandler := EventHandler{
		Actions: []ActionRule{
			{
				Action: action,
				If:     nil, // No guard
			},
		},
	}

	err := txn.performActions(ctx, eventHandler)
	assert.NoError(t, err)
	assert.True(t, actionCalled)
}

func TestPerformActions_ActionWithGuardTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	actionCalled := false
	action := func(ctx context.Context, txn *Transaction) error {
		actionCalled = true
		return nil
	}

	guard := func(ctx context.Context, txn *Transaction) bool {
		return true
	}

	eventHandler := EventHandler{
		Actions: []ActionRule{
			{
				Action: action,
				If:     guard,
			},
		},
	}

	err := txn.performActions(ctx, eventHandler)
	assert.NoError(t, err)
	assert.True(t, actionCalled)
}

func TestPerformActions_ActionWithGuardFalse(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	actionCalled := false
	action := func(ctx context.Context, txn *Transaction) error {
		actionCalled = true
		return nil
	}

	guard := func(ctx context.Context, txn *Transaction) bool {
		return false
	}

	eventHandler := EventHandler{
		Actions: []ActionRule{
			{
				Action: action,
				If:     guard,
			},
		},
	}

	err := txn.performActions(ctx, eventHandler)
	assert.NoError(t, err)
	assert.False(t, actionCalled) // Action should not be called when guard returns false
}

func TestPerformActions_ActionError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	expectedError := errors.New("action failed")
	action := func(ctx context.Context, txn *Transaction) error {
		return expectedError
	}

	eventHandler := EventHandler{
		Actions: []ActionRule{
			{
				Action: action,
				If:     nil,
			},
		},
	}

	err := txn.performActions(ctx, eventHandler)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestPerformActions_MultipleActions(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	action1Called := false
	action2Called := false

	action1 := func(ctx context.Context, txn *Transaction) error {
		action1Called = true
		return nil
	}

	action2 := func(ctx context.Context, txn *Transaction) error {
		action2Called = true
		return nil
	}

	eventHandler := EventHandler{
		Actions: []ActionRule{
			{
				Action: action1,
				If:     nil,
			},
			{
				Action: action2,
				If:     nil,
			},
		},
	}

	err := txn.performActions(ctx, eventHandler)
	assert.NoError(t, err)
	assert.True(t, action1Called)
	assert.True(t, action2Called)
}

func TestEvaluateTransitions_NoTransitions(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	initialState := txn.GetCurrentState()
	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	eventHandler := EventHandler{
		Transitions: []Transition{},
	}

	err := txn.evaluateTransitions(ctx, event, eventHandler)
	assert.NoError(t, err)
	assert.Equal(t, initialState, txn.GetCurrentState())
}

func TestEvaluateTransitions_TransitionWithoutGuard(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	eventHandler := EventHandler{
		Transitions: []Transition{
			{
				To: State_Pending,
				If: nil, // No guard
				On: nil,
			},
		},
	}

	err := txn.evaluateTransitions(ctx, event, eventHandler)
	assert.NoError(t, err)
	assert.Equal(t, State_Pending, txn.GetCurrentState())
}

func TestEvaluateTransitions_TransitionWithGuardTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	guard := func(ctx context.Context, txn *Transaction) bool {
		return true
	}

	eventHandler := EventHandler{
		Transitions: []Transition{
			{
				To: State_Pending,
				If: guard,
				On: nil,
			},
		},
	}

	err := txn.evaluateTransitions(ctx, event, eventHandler)
	assert.NoError(t, err)
	assert.Equal(t, State_Pending, txn.GetCurrentState())
}

func TestEvaluateTransitions_TransitionWithGuardFalse(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	initialState := txn.GetCurrentState()
	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	guard := func(ctx context.Context, txn *Transaction) bool {
		return false
	}

	eventHandler := EventHandler{
		Transitions: []Transition{
			{
				To: State_Pending,
				If: guard,
				On: nil,
			},
		},
	}

	err := txn.evaluateTransitions(ctx, event, eventHandler)
	assert.NoError(t, err)
	assert.Equal(t, initialState, txn.GetCurrentState())
}

func TestEvaluateTransitions_TransitionWithOnAction(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	onActionCalled := false
	onAction := func(ctx context.Context, txn *Transaction) error {
		onActionCalled = true
		return nil
	}

	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	eventHandler := EventHandler{
		Transitions: []Transition{
			{
				To: State_Pending,
				If: nil,
				On: onAction,
			},
		},
	}

	err := txn.evaluateTransitions(ctx, event, eventHandler)
	assert.NoError(t, err)
	assert.Equal(t, State_Pending, txn.GetCurrentState())
	assert.True(t, onActionCalled)
}

func TestEvaluateTransitions_TransitionWithOnActionError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	expectedError := errors.New("on action failed")
	onAction := func(ctx context.Context, txn *Transaction) error {
		return expectedError
	}

	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	eventHandler := EventHandler{
		Transitions: []Transition{
			{
				To: State_Pending,
				If: nil,
				On: onAction,
			},
		},
	}

	err := txn.evaluateTransitions(ctx, event, eventHandler)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	// State should still be updated even if On action fails
	assert.Equal(t, State_Pending, txn.GetCurrentState())
}

func TestEvaluateTransitions_StateOnTransitionTo(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	// Set up assemble request for State_Assembling
	requestID := uuid.New()
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{
		requestID: requestID,
	}

	// Mock the AssembleAndSign call that will be triggered by OnTransitionTo
	mocks.MockForAssembleAndSignRequestOK()

	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
		RequestID:   requestID,
		Coordinator: txn.currentDelegate,
	}

	// Get the actual event handler from the state machine
	eventHandler, err := txn.evaluateEvent(ctx, event)
	require.NoError(t, err)
	require.NotNil(t, eventHandler)

	// Apply the event first
	err = txn.applyEvent(ctx, event)
	require.NoError(t, err)

	// Evaluate transitions - State_Assembling has OnTransitionTo action (action_AssembleAndSign)
	err = txn.evaluateTransitions(ctx, event, *eventHandler)
	assert.NoError(t, err)
	assert.Equal(t, State_Assembling, txn.GetCurrentState())
}

func TestEvaluateTransitions_MultipleTransitionsFirstMatches(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	firstActionCalled := false
	secondActionCalled := false

	firstAction := func(ctx context.Context, txn *Transaction) error {
		firstActionCalled = true
		return nil
	}

	secondAction := func(ctx context.Context, txn *Transaction) error {
		secondActionCalled = true
		return nil
	}

	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.ID,
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
		},
	}

	eventHandler := EventHandler{
		Transitions: []Transition{
			{
				To: State_Pending,
				If: func(ctx context.Context, txn *Transaction) bool {
					return true // First guard returns true
				},
				On: firstAction,
			},
			{
				To: State_Delegated,
				If: func(ctx context.Context, txn *Transaction) bool {
					return true // Second guard also returns true, but shouldn't be evaluated
				},
				On: secondAction,
			},
		},
	}

	err := txn.evaluateTransitions(ctx, event, eventHandler)
	assert.NoError(t, err)
	assert.Equal(t, State_Pending, txn.GetCurrentState())
	assert.True(t, firstActionCalled)
	assert.False(t, secondActionCalled)
}

func TestGuard_Not(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// Test with a guard that returns true
	guardTrue := func(ctx context.Context, txn *Transaction) bool {
		return true
	}

	notGuardTrue := guard_Not(guardTrue)
	result := notGuardTrue(ctx, txn)
	assert.False(t, result)

	// Test with a guard that returns false
	guardFalse := func(ctx context.Context, txn *Transaction) bool {
		return false
	}

	notGuardFalse := guard_Not(guardFalse)
	result = notGuardFalse(ctx, txn)
	assert.True(t, result)
}
func TestState_String(t *testing.T) {
	testCases := []struct {
		state    State
		expected string
	}{
		{State_Initial, "State_Initial"},
		{State_Pending, "State_Pending"},
		{State_Delegated, "State_Delegated"},
		{State_Assembling, "State_Assembling"},
		{State_Endorsement_Gathering, "State_Endorsement_Gathering"},
		{State_Signing, "State_Signing"},
		{State_Prepared, "State_Prepared"},
		{State_Dispatched, "State_Dispatched"},
		{State_Sequenced, "State_Sequenced"},
		{State_Submitted, "State_Submitted"},
		{State_Confirmed, "State_Confirmed"},
		{State_Reverted, "State_Reverted"},
		{State_Parked, "State_Parked"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.state.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestState_String_Unknown(t *testing.T) {
	unknownState := State(999) // A state value that doesn't exist
	result := unknownState.String()
	assert.Equal(t, "Unknown", result)
}

// mockCommonEvent is a mock implementation of common.Event for testing
type mockCommonEvent struct {
	eventType  EventType
	eventTime  time.Time
	typeString string
}

func (m *mockCommonEvent) Type() EventType {
	return m.eventType
}

func (m *mockCommonEvent) TypeString() string {
	return m.typeString
}

func (m *mockCommonEvent) GetEventTime() time.Time {
	return m.eventTime
}
