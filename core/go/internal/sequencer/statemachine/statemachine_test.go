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

package statemachine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test state type
type TestState int

const (
	State_Idle TestState = iota
	State_Active
	State_Processing
	State_Complete
	State_Error
)

func (s TestState) String() string {
	switch s {
	case State_Idle:
		return "Idle"
	case State_Active:
		return "Active"
	case State_Processing:
		return "Processing"
	case State_Complete:
		return "Complete"
	case State_Error:
		return "Error"
	}
	return "Unknown"
}

// Test event types
const (
	Event_Start     EventType = 100
	Event_Process   EventType = 101
	Event_Complete  EventType = 102
	Event_Error     EventType = 103
	Event_Reset     EventType = 104
	Event_Heartbeat EventType = 105
)

// Test event implementations
type TestEvent struct {
	common.BaseEvent
	eventType EventType
	data      string
}

func NewTestEvent(eventType EventType, data string) *TestEvent {
	return &TestEvent{
		BaseEvent: common.BaseEvent{EventTime: time.Now()},
		eventType: eventType,
		data:      data,
	}
}

func (e *TestEvent) Type() EventType {
	return e.eventType
}

func (e *TestEvent) TypeString() string {
	switch e.eventType {
	case Event_Start:
		return "Event_Start"
	case Event_Process:
		return "Event_Process"
	case Event_Complete:
		return "Event_Complete"
	case Event_Error:
		return "Event_Error"
	case Event_Reset:
		return "Event_Reset"
	case Event_Heartbeat:
		return "Event_Heartbeat"
	}
	return "Unknown"
}

// Test subject type
type TestSubject struct {
	Name           string
	ProcessedCount int
	LastData       string
	ActionsCalled  []string
}

func TestNewStateMachine(t *testing.T) {
	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)

	assert.Equal(t, State_Idle, sm.GetCurrentState())
	assert.NotZero(t, sm.GetLastStateChange())
	assert.Empty(t, sm.GetLatestEvent())
}

func TestBasicTransition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.GetCurrentState())
	assert.Equal(t, "Event_Start", sm.GetLatestEvent())
}

func TestTransitionWithGuard(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 0}

	guardProcessedEnough := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount >= 3
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Processing: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Complete: {
						Transitions: []Transition[TestState, *TestSubject]{
							{
								To: State_Complete,
								If: guardProcessedEnough,
							},
						},
					},
				},
			},
			State_Complete: {},
		},
	}

	sm := NewStateMachine(config, State_Processing)

	// First attempt - guard fails (ProcessedCount < 3)
	event := NewTestEvent(Event_Complete, "")
	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Processing, sm.GetCurrentState()) // No transition

	// Set ProcessedCount to meet guard condition
	subject.ProcessedCount = 3

	// Second attempt - guard passes
	err = sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Complete, sm.GetCurrentState())
}

func TestTransitionWithOnAction(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	transitionAction := func(ctx context.Context, s *TestSubject) error {
		s.ActionsCalled = append(s.ActionsCalled, "transition_action")
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Transitions: []Transition[TestState, *TestSubject]{
							{
								To: State_Active,
								On: transitionAction,
							},
						},
					},
				},
			},
			State_Active: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.GetCurrentState())
	assert.Contains(t, subject.ActionsCalled, "transition_action")
}

func TestOnTransitionToAction(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	entryAction := func(ctx context.Context, s *TestSubject) error {
		s.ActionsCalled = append(s.ActionsCalled, "entry_action")
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {
				OnTransitionTo: entryAction,
			},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Contains(t, subject.ActionsCalled, "entry_action")
}

func TestEventActions(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	action1 := func(ctx context.Context, s *TestSubject) error {
		s.ActionsCalled = append(s.ActionsCalled, "action1")
		return nil
	}

	action2 := func(ctx context.Context, s *TestSubject) error {
		s.ActionsCalled = append(s.ActionsCalled, "action2")
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Active: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Heartbeat: {
						Actions: []ActionRule[*TestSubject]{
							{Action: action1},
							{Action: action2},
						},
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Active)
	event := NewTestEvent(Event_Heartbeat, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, []string{"action1", "action2"}, subject.ActionsCalled)
}

func TestEventActionsWithGuards(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 0}

	action1 := func(ctx context.Context, s *TestSubject) error {
		s.ActionsCalled = append(s.ActionsCalled, "action1")
		return nil
	}

	action2 := func(ctx context.Context, s *TestSubject) error {
		s.ActionsCalled = append(s.ActionsCalled, "action2")
		return nil
	}

	guardHasProcessed := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount > 0
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Active: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Heartbeat: {
						Actions: []ActionRule[*TestSubject]{
							{Action: action1},                        // No guard - always runs
							{Action: action2, If: guardHasProcessed}, // Guarded
						},
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Active)
	event := NewTestEvent(Event_Heartbeat, "")

	// First call - guard fails for action2
	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, []string{"action1"}, subject.ActionsCalled)

	// Set ProcessedCount
	subject.ProcessedCount = 1
	subject.ActionsCalled = nil

	// Second call - both actions run
	err = sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, []string{"action1", "action2"}, subject.ActionsCalled)
}

func TestEventValidator(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	validator := func(ctx context.Context, s *TestSubject, event common.Event) (bool, error) {
		testEvent := event.(*TestEvent)
		return testEvent.data == "valid", nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Validator: validator,
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)

	// Invalid event - should be ignored
	invalidEvent := NewTestEvent(Event_Start, "invalid")
	err := sm.ProcessEvent(ctx, subject, invalidEvent)
	require.NoError(t, err)
	assert.Equal(t, State_Idle, sm.GetCurrentState())

	// Valid event - should trigger transition
	validEvent := NewTestEvent(Event_Start, "valid")
	err = sm.ProcessEvent(ctx, subject, validEvent)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.GetCurrentState())
}

func TestOnHandleEventUpdatesState(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	stateupdate_Start := func(ctx context.Context, s *TestSubject, event common.Event) error {
		testEvent := event.(*TestEvent)
		s.LastData = testEvent.data
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						OnHandleEvent: stateupdate_Start,
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "applied_data")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, "applied_data", subject.LastData)
}

func TestTransitionCallback(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	var callbackCalled bool
	var callbackFrom, callbackTo TestState

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {},
		},
		OnTransition: func(ctx context.Context, s *TestSubject, from, to TestState, event common.Event) {
			callbackCalled = true
			callbackFrom = from
			callbackTo = to
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.True(t, callbackCalled)
	assert.Equal(t, State_Idle, callbackFrom)
	assert.Equal(t, State_Active, callbackTo)
}

func TestIgnoredEvent(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Idle)

	// Event_Complete is not handled in State_Idle - should be ignored
	event := NewTestEvent(Event_Complete, "")
	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Idle, sm.GetCurrentState())
}

func TestActionError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("action failed")
	failingAction := func(ctx context.Context, s *TestSubject) error {
		return expectedErr
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Actions: []ActionRule[*TestSubject]{
							{Action: failingAction},
						},
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	assert.ErrorIs(t, err, expectedErr)
	// State should not change on error
	assert.Equal(t, State_Idle, sm.GetCurrentState())
}

func TestMultipleTransitionsFirstWins(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 5}

	guardLow := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount < 3
	}

	guardMedium := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount >= 3 && s.ProcessedCount < 10
	}

	guardHigh := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount >= 10
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Processing: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Complete: {
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Error, If: guardLow},
							{To: State_Active, If: guardMedium}, // This should match
							{To: State_Complete, If: guardHigh},
						},
					},
				},
			},
			State_Active:   {},
			State_Complete: {},
			State_Error:    {},
		},
	}

	sm := NewStateMachine(config, State_Processing)
	event := NewTestEvent(Event_Complete, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.GetCurrentState())
}

func TestIsInState(t *testing.T) {
	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{},
	}

	sm := NewStateMachine(config, State_Active)

	assert.True(t, sm.IsInState(State_Active))
	assert.False(t, sm.IsInState(State_Idle))
}

func TestIsInAnyState(t *testing.T) {
	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{},
	}

	sm := NewStateMachine(config, State_Active)

	assert.True(t, sm.IsInAnyState(State_Idle, State_Active, State_Processing))
	assert.False(t, sm.IsInAnyState(State_Idle, State_Complete))
}

// Test guard combinators

func TestGuardNot(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{ProcessedCount: 5}

	guard := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount > 10
	}

	notGuard := Not(guard)

	assert.False(t, guard(ctx, subject))
	assert.True(t, notGuard(ctx, subject))
}

func TestGuardAnd(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{ProcessedCount: 5, Name: "test"}

	guard1 := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount > 3
	}

	guard2 := func(ctx context.Context, s *TestSubject) bool {
		return s.Name == "test"
	}

	guard3 := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount > 10
	}

	assert.True(t, And(guard1, guard2)(ctx, subject))
	assert.False(t, And(guard1, guard3)(ctx, subject))
	assert.False(t, And(guard2, guard3)(ctx, subject))
}

func TestGuardOr(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{ProcessedCount: 5, Name: "test"}

	guard1 := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount > 10
	}

	guard2 := func(ctx context.Context, s *TestSubject) bool {
		return s.Name == "test"
	}

	guard3 := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount > 100
	}

	assert.True(t, Or(guard1, guard2)(ctx, subject))
	assert.True(t, Or(guard2, guard3)(ctx, subject))
	assert.False(t, Or(guard1, guard3)(ctx, subject))
}

func TestComplexGuardCombination(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{ProcessedCount: 5, Name: "test"}

	isActive := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount > 0
	}

	isNamed := func(ctx context.Context, s *TestSubject) bool {
		return s.Name != ""
	}

	isBusy := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount > 10
	}

	// (isActive AND isNamed) OR isBusy
	complexGuard := Or(And(isActive, isNamed), isBusy)
	assert.True(t, complexGuard(ctx, subject))

	// NOT(isActive AND isNamed)
	negatedGuard := Not(And(isActive, isNamed))
	assert.False(t, negatedGuard(ctx, subject))
}

func TestSetState(t *testing.T) {
	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{},
	}

	sm := NewStateMachine(config, State_Idle)
	assert.Equal(t, State_Idle, sm.GetCurrentState())

	sm.SetState(State_Active)

	assert.Equal(t, State_Active, sm.GetCurrentState())
}

func TestTimeSinceStateChange(t *testing.T) {
	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{},
	}

	sm := NewStateMachine(config, State_Idle)

	time.Sleep(5 * time.Millisecond)
	duration := sm.TimeSinceStateChange()

	assert.True(t, duration >= 5*time.Millisecond)
}

func TestOnHandleEventErrorPreventsTransition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("state update failed")
	stateupdate_Failing := func(ctx context.Context, s *TestSubject, event common.Event) error {
		return expectedErr
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						OnHandleEvent: stateupdate_Failing,
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	assert.ErrorIs(t, err, expectedErr)
	// State should not change on OnHandleEvent error
	assert.Equal(t, State_Idle, sm.GetCurrentState())
}

func TestValidatorError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("validator error")
	validator := func(ctx context.Context, s *TestSubject, event common.Event) (bool, error) {
		return false, expectedErr
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Validator: validator,
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, State_Idle, sm.GetCurrentState())
}

func TestNoStateDefinition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	// Config with no definition for State_Idle
	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Active: {}, // Only Active is defined
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	// Event should be ignored since State_Idle has no definition
	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Idle, sm.GetCurrentState())
}

func TestTransitionOnActionError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("transition action failed")
	failingTransitionAction := func(ctx context.Context, s *TestSubject) error {
		return expectedErr
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Transitions: []Transition[TestState, *TestSubject]{
							{
								To: State_Active,
								On: failingTransitionAction,
							},
						},
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	assert.ErrorIs(t, err, expectedErr)
	// State should not change on transition action error
	assert.Equal(t, State_Idle, sm.GetCurrentState())
}

func TestOnTransitionToError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("OnTransitionTo failed")
	failingEntryAction := func(ctx context.Context, s *TestSubject) error {
		return expectedErr
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {
				OnTransitionTo: failingEntryAction,
			},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	assert.ErrorIs(t, err, expectedErr)
	// Note: state has already changed to Active before OnTransitionTo runs
	assert.Equal(t, State_Active, sm.GetCurrentState())
}

func TestTransitionToStateWithNoDefinition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active}, // State_Active has no definition
						},
					},
				},
			},
			// No definition for State_Active
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	// Should succeed even if target state has no definition
	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.GetCurrentState())
}

func TestEventWithNoTransitions(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	actionCalled := false
	action := func(ctx context.Context, s *TestSubject) error {
		actionCalled = true
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Active: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Heartbeat: {
						Actions: []ActionRule[*TestSubject]{
							{Action: action},
						},
						// No transitions defined - event only triggers actions
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Active)
	event := NewTestEvent(Event_Heartbeat, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.True(t, actionCalled)
	assert.Equal(t, State_Active, sm.GetCurrentState()) // No state change
}

func TestGuardAndEmptyList(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{}

	// And with no guards should return true (vacuous truth)
	emptyAnd := And[*TestSubject]()
	assert.True(t, emptyAnd(ctx, subject))
}

func TestGuardOrEmptyList(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{}

	// Or with no guards should return false
	emptyOr := Or[*TestSubject]()
	assert.False(t, emptyOr(ctx, subject))
}

// Tests for OnHandleEvent functionality

func TestOnHandleEventBasic(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	stateupdate_Start := func(ctx context.Context, s *TestSubject, event common.Event) error {
		testEvent := event.(*TestEvent)
		s.LastData = testEvent.data
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						OnHandleEvent: stateupdate_Start,
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "event_data")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.GetCurrentState())
	assert.Equal(t, "event_data", subject.LastData)
}

func TestOnHandleEventCalledBeforeGuards(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 0}

	// OnHandleEvent sets ProcessedCount, guard checks it
	stateupdate_Process := func(ctx context.Context, s *TestSubject, event common.Event) error {
		s.ProcessedCount = 10
		return nil
	}

	guardProcessedEnough := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount >= 5
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Processing: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Process: {
						OnHandleEvent: stateupdate_Process,
						Transitions: []Transition[TestState, *TestSubject]{
							{
								To: State_Complete,
								If: guardProcessedEnough,
							},
						},
					},
				},
			},
			State_Complete: {},
		},
	}

	sm := NewStateMachine(config, State_Processing)
	event := NewTestEvent(Event_Process, "")

	// Before processing, ProcessedCount is 0, but OnHandleEvent will set it to 10
	// The guard should see the updated value and allow the transition
	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, State_Complete, sm.GetCurrentState())
	assert.Equal(t, 10, subject.ProcessedCount)
}

func TestOnHandleEventError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("state update failed")
	stateupdate_Failing := func(ctx context.Context, s *TestSubject, event common.Event) error {
		return expectedErr
	}

	actionCalled := false
	action := func(ctx context.Context, s *TestSubject) error {
		actionCalled = true
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						OnHandleEvent: stateupdate_Failing,
						Actions: []ActionRule[*TestSubject]{
							{Action: action},
						},
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	assert.ErrorIs(t, err, expectedErr)
	// State should not change on OnHandleEvent error
	assert.Equal(t, State_Idle, sm.GetCurrentState())
	// Actions should not be called when OnHandleEvent fails
	assert.False(t, actionCalled)
}

func TestOnHandleEventWithActions(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	callOrder := []string{}

	stateupdate_Track := func(ctx context.Context, s *TestSubject, event common.Event) error {
		callOrder = append(callOrder, "onhandle")
		return nil
	}

	action := func(ctx context.Context, s *TestSubject) error {
		callOrder = append(callOrder, "action")
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						OnHandleEvent: stateupdate_Track,
						Actions: []ActionRule[*TestSubject]{
							{Action: action},
						},
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	// Verify order: OnHandleEvent called before actions
	assert.Equal(t, []string{"onhandle", "action"}, callOrder)
}

func TestOnHandleEventReceivesCorrectEvent(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	var receivedEventType EventType
	var receivedEventData string

	stateupdate_Capture := func(ctx context.Context, s *TestSubject, event common.Event) error {
		testEvent := event.(*TestEvent)
		receivedEventType = testEvent.Type()
		receivedEventData = testEvent.data
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						OnHandleEvent: stateupdate_Capture,
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "test_payload")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.Equal(t, Event_Start, receivedEventType)
	assert.Equal(t, "test_payload", receivedEventData)
}

func TestOnHandleEventWithValidator(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	validatorCalled := false
	onHandleEventCalled := false

	validator := func(ctx context.Context, s *TestSubject, event common.Event) (bool, error) {
		validatorCalled = true
		testEvent := event.(*TestEvent)
		return testEvent.data == "valid", nil
	}

	stateupdate_Track := func(ctx context.Context, s *TestSubject, event common.Event) error {
		onHandleEventCalled = true
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						Validator:     validator,
						OnHandleEvent: stateupdate_Track,
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)

	// Test with invalid event - validator fails, OnHandleEvent should NOT be called
	invalidEvent := NewTestEvent(Event_Start, "invalid")
	err := sm.ProcessEvent(ctx, subject, invalidEvent)
	require.NoError(t, err)
	assert.True(t, validatorCalled)
	assert.False(t, onHandleEventCalled)
	assert.Equal(t, State_Idle, sm.GetCurrentState())

	// Reset for next test
	validatorCalled = false
	onHandleEventCalled = false

	// Test with valid event - validator passes, OnHandleEvent should be called
	validEvent := NewTestEvent(Event_Start, "valid")
	err = sm.ProcessEvent(ctx, subject, validEvent)
	require.NoError(t, err)
	assert.True(t, validatorCalled)
	assert.True(t, onHandleEventCalled)
	assert.Equal(t, State_Active, sm.GetCurrentState())
}

func TestOnHandleEventWithTransitionOnAction(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	callOrder := []string{}

	stateupdate_Track := func(ctx context.Context, s *TestSubject, event common.Event) error {
		callOrder = append(callOrder, "onhandle")
		return nil
	}

	transitionAction := func(ctx context.Context, s *TestSubject) error {
		callOrder = append(callOrder, "transition_on")
		return nil
	}

	entryAction := func(ctx context.Context, s *TestSubject) error {
		callOrder = append(callOrder, "entry")
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						OnHandleEvent: stateupdate_Track,
						Transitions: []Transition[TestState, *TestSubject]{
							{
								To: State_Active,
								On: transitionAction,
							},
						},
					},
				},
			},
			State_Active: {
				OnTransitionTo: entryAction,
			},
		},
	}

	sm := NewStateMachine(config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	// Verify order: OnHandleEvent -> transition On action -> OnTransitionTo
	assert.Equal(t, []string{"onhandle", "transition_on", "entry"}, callOrder)
}

func TestOnHandleEventNoTransition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	onHandleEventCalled := false
	stateupdate_NoTransition := func(ctx context.Context, s *TestSubject, event common.Event) error {
		onHandleEventCalled = true
		s.ProcessedCount++
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Active: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Heartbeat: {
						OnHandleEvent: stateupdate_NoTransition,
						// No transitions defined - event only triggers state update
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Active)
	event := NewTestEvent(Event_Heartbeat, "")

	err := sm.ProcessEvent(ctx, subject, event)
	require.NoError(t, err)
	assert.True(t, onHandleEventCalled)
	assert.Equal(t, 1, subject.ProcessedCount)
	assert.Equal(t, State_Active, sm.GetCurrentState()) // No state change
}

func TestMultipleEventsWithDifferentOnHandleEvents(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	stateupdate_Start := func(ctx context.Context, s *TestSubject, event common.Event) error {
		s.ActionsCalled = append(s.ActionsCalled, "stateupdate_start")
		return nil
	}

	stateupdate_Process := func(ctx context.Context, s *TestSubject, event common.Event) error {
		s.ActionsCalled = append(s.ActionsCalled, "stateupdate_process")
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Idle: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Start: {
						OnHandleEvent: stateupdate_Start,
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Active},
						},
					},
				},
			},
			State_Active: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Process: {
						OnHandleEvent: stateupdate_Process,
						Transitions: []Transition[TestState, *TestSubject]{
							{To: State_Processing},
						},
					},
				},
			},
			State_Processing: {},
		},
	}

	sm := NewStateMachine(config, State_Idle)

	// First event
	err := sm.ProcessEvent(ctx, subject, NewTestEvent(Event_Start, ""))
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.GetCurrentState())
	assert.Equal(t, []string{"stateupdate_start"}, subject.ActionsCalled)

	// Second event - different OnHandleEvent
	err = sm.ProcessEvent(ctx, subject, NewTestEvent(Event_Process, ""))
	require.NoError(t, err)
	assert.Equal(t, State_Processing, sm.GetCurrentState())
	assert.Equal(t, []string{"stateupdate_start", "stateupdate_process"}, subject.ActionsCalled)
}

func TestOnHandleEventWithGuardedActions(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 0}

	// OnHandleEvent sets the state that guards will check
	stateupdate_SetCount := func(ctx context.Context, s *TestSubject, event common.Event) error {
		testEvent := event.(*TestEvent)
		if testEvent.data == "high" {
			s.ProcessedCount = 10
		}
		return nil
	}

	guardHigh := func(ctx context.Context, s *TestSubject) bool {
		return s.ProcessedCount >= 5
	}

	highAction := func(ctx context.Context, s *TestSubject) error {
		s.ActionsCalled = append(s.ActionsCalled, "high_action")
		return nil
	}

	lowAction := func(ctx context.Context, s *TestSubject) error {
		s.ActionsCalled = append(s.ActionsCalled, "low_action")
		return nil
	}

	config := StateMachineConfig[TestState, *TestSubject]{
		Definitions: map[TestState]StateDefinition[TestState, *TestSubject]{
			State_Active: {
				Events: map[EventType]EventHandler[TestState, *TestSubject]{
					Event_Process: {
						OnHandleEvent: stateupdate_SetCount,
						Actions: []ActionRule[*TestSubject]{
							{Action: highAction, If: guardHigh},
							{Action: lowAction, If: Not(guardHigh)},
						},
					},
				},
			},
		},
	}

	sm := NewStateMachine(config, State_Active)

	// Low event - guard should fail, lowAction should be called
	err := sm.ProcessEvent(ctx, subject, NewTestEvent(Event_Process, "low"))
	require.NoError(t, err)
	assert.Equal(t, []string{"low_action"}, subject.ActionsCalled)

	// Reset
	subject.ActionsCalled = nil
	subject.ProcessedCount = 0

	// High event - OnHandleEvent sets count, guard should pass, highAction should be called
	err = sm.ProcessEvent(ctx, subject, NewTestEvent(Event_Process, "high"))
	require.NoError(t, err)
	assert.Equal(t, []string{"high_action"}, subject.ActionsCalled)
}
