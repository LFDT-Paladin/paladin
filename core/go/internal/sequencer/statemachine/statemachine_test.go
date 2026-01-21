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

// Test subject type - implements Subject[*TestSubject, *TestSubject, *TestSubject]
type TestSubject struct {
	StateMachineState[TestState]
	Name           string
	ProcessedCount int
	LastData       string
	ActionsCalled  []string
}

// Implement Subject interface
func (s *TestSubject) GetStateMutator() *TestSubject { return s }
func (s *TestSubject) GetStateReader() *TestSubject  { return s }
func (s *TestSubject) GetConfig() *TestSubject       { return s }
func (s *TestSubject) GetCallbacks() *TestSubject    { return s }
func (s *TestSubject) Lock()                         {}
func (s *TestSubject) Unlock()                       {}
func (s *TestSubject) RLock()                        {}
func (s *TestSubject) RUnlock()                      {}

// Type aliases for tests - in tests, subject IS the mutable state, reader, config, and callbacks
type TestStateMachineConfig = StateMachineConfig[TestState, *TestSubject, *TestSubject, *TestSubject, *TestSubject]
type TestStateDefinition = StateDefinition[TestState, *TestSubject, *TestSubject, *TestSubject, *TestSubject]
type TestEventHandler = EventHandler[TestState, *TestSubject, *TestSubject, *TestSubject, *TestSubject]
type TestTransition = Transition[TestState, *TestSubject, *TestSubject, *TestSubject, *TestSubject]
type TestTransitionToHandler = TransitionToHandler[TestState, *TestSubject, *TestSubject, *TestSubject, *TestSubject]
type TestActionRule = ActionRule[TestState, *TestSubject, *TestSubject, *TestSubject]
type TestAction = Action[TestState, *TestSubject, *TestSubject, *TestSubject]
type TestGuard = Guard[TestState, *TestSubject, *TestSubject]
type TestStateUpdate = StateUpdate[TestState, *TestSubject, *TestSubject, *TestSubject]
type TestStateUpdateRule = StateUpdateRule[TestState, *TestSubject, *TestSubject, *TestSubject, *TestSubject]
type TestValidator = Validator[TestState, *TestSubject, *TestSubject, *TestSubject]
type TestTransitionCallbackFn = TransitionCallback[TestState, *TestSubject, *TestSubject, *TestSubject]

// Helper to create test config
func newTestConfig(definitions map[TestState]TestStateDefinition) TestStateMachineConfig {
	return TestStateMachineConfig{
		Definitions: definitions,
	}
}

func newTestConfigWithCallback(definitions map[TestState]TestStateDefinition, onTransition TransitionCallback[TestState, *TestSubject, *TestSubject, *TestSubject]) TestStateMachineConfig {
	return TestStateMachineConfig{
		Definitions:  definitions,
		OnTransition: onTransition,
	}
}

func TestNewStateMachine(t *testing.T) {
	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {},
	})

	sm := NewStateMachine(&TestSubject{}, config, State_Idle)

	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())
	assert.NotZero(t, sm.subject.GetStateReader().GetLastStateChange())
	assert.Empty(t, sm.subject.GetStateReader().GetLatestEvent())
}

func TestBasicTransition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
	assert.Equal(t, "Event_Start", sm.subject.GetStateReader().GetLatestEvent())
}

func TestTransitionWithGuard(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 0}

	guardProcessedEnough := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount >= 3
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Processing: {
			Events: map[EventType]TestEventHandler{
				Event_Complete: {
					Transitions: []TestTransition{
						{
							To: State_Complete,
							If: guardProcessedEnough,
						},
					},
				},
			},
		},
		State_Complete: {},
	})

	sm := NewStateMachine(subject, config, State_Processing)

	// First attempt - guard fails (ProcessedCount < 3)
	event := NewTestEvent(Event_Complete, "")
	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Processing, sm.subject.GetStateReader().GetCurrentState()) // No transition

	// Set ProcessedCount to meet guard condition
	subject.ProcessedCount = 3

	// Second attempt - guard passes
	err = sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Complete, sm.subject.GetStateReader().GetCurrentState())
}

func TestTransitionWithOnAction(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	transitionAction := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		reader.ActionsCalled = append(reader.ActionsCalled, "transition_action")
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Transitions: []TestTransition{
						{
							To: State_Active,
							On: transitionAction,
						},
					},
				},
			},
		},
		State_Active: {},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
	assert.Contains(t, subject.ActionsCalled, "transition_action")
}

func TestOnTransitionToAction(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	entryAction := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		reader.ActionsCalled = append(reader.ActionsCalled, "entry_action")
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {
			OnTransitionTo: TestTransitionToHandler{
				Actions: []TestActionRule{{Action: entryAction}},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Contains(t, subject.ActionsCalled, "entry_action")
}

func TestEventActions(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	action1 := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		reader.ActionsCalled = append(reader.ActionsCalled, "action1")
		return nil
	}

	action2 := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		reader.ActionsCalled = append(reader.ActionsCalled, "action2")
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Active: {
			Events: map[EventType]TestEventHandler{
				Event_Heartbeat: {
					Actions: []TestActionRule{
						{Action: action1},
						{Action: action2},
					},
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Active)
	event := NewTestEvent(Event_Heartbeat, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, []string{"action1", "action2"}, subject.ActionsCalled)
}

func TestEventActionsWithGuards(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 0}

	action1 := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		reader.ActionsCalled = append(reader.ActionsCalled, "action1")
		return nil
	}

	action2 := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		reader.ActionsCalled = append(reader.ActionsCalled, "action2")
		return nil
	}

	guardHasProcessed := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount > 0
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Active: {
			Events: map[EventType]TestEventHandler{
				Event_Heartbeat: {
					Actions: []TestActionRule{
						{Action: action1},                        // No guard - always runs
						{Action: action2, If: guardHasProcessed}, // Guarded
					},
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Active)
	event := NewTestEvent(Event_Heartbeat, "")

	// First call - guard fails for action2
	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, []string{"action1"}, subject.ActionsCalled)

	// Set ProcessedCount
	subject.ProcessedCount = 1
	subject.ActionsCalled = nil

	// Second call - both actions run
	err = sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, []string{"action1", "action2"}, subject.ActionsCalled)
}

func TestEventValidator(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	validator := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) (bool, error) {
		testEvent := event.(*TestEvent)
		return testEvent.data == "valid", nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Validator: validator,
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {},
	})

	sm := NewStateMachine(subject, config, State_Idle)

	// Invalid event - should be ignored
	invalidEvent := NewTestEvent(Event_Start, "invalid")
	err := sm.ProcessEvent(ctx, invalidEvent)
	require.NoError(t, err)
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())

	// Valid event - should trigger transition
	validEvent := NewTestEvent(Event_Start, "valid")
	err = sm.ProcessEvent(ctx, validEvent)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
}

func TestOnHandleEventUpdatesState(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	stateupdate_Start := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		testEvent := event.(*TestEvent)
		state.LastData = testEvent.data
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Start}},
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "applied_data")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, "applied_data", subject.LastData)
}

func TestTransitionCallback(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	var callbackCalled bool
	var callbackFrom, callbackTo TestState

	config := newTestConfigWithCallback(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {},
	}, func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, from, to TestState, event common.Event) {
		callbackCalled = true
		callbackFrom = from
		callbackTo = to
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.True(t, callbackCalled)
	assert.Equal(t, State_Idle, callbackFrom)
	assert.Equal(t, State_Active, callbackTo)
}

func TestIgnoredEvent(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Idle)

	// Event_Complete is not handled in State_Idle - should be ignored
	event := NewTestEvent(Event_Complete, "")
	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())
}

func TestActionError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("action failed")
	failingAction := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		return expectedErr
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Actions: []TestActionRule{
						{Action: failingAction},
					},
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	assert.ErrorIs(t, err, expectedErr)
	// State should not change on error
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())
}

func TestMultipleTransitionsFirstWins(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 5}

	guardLow := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount < 3
	}

	guardMedium := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount >= 3 && reader.ProcessedCount < 10
	}

	guardHigh := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount >= 10
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Processing: {
			Events: map[EventType]TestEventHandler{
				Event_Complete: {
					Transitions: []TestTransition{
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
	})

	sm := NewStateMachine(subject, config, State_Processing)
	event := NewTestEvent(Event_Complete, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
}

func TestIsInState(t *testing.T) {
	config := newTestConfig(map[TestState]TestStateDefinition{})

	sm := NewStateMachine(&TestSubject{}, config, State_Active)

	assert.True(t, sm.subject.GetStateReader().GetCurrentState() == State_Active)
	assert.False(t, sm.subject.GetStateReader().GetCurrentState() == State_Idle)
}

func TestIsInAnyState(t *testing.T) {
	config := newTestConfig(map[TestState]TestStateDefinition{})

	sm := NewStateMachine(&TestSubject{}, config, State_Active)

	assert.Contains(t, []TestState{State_Idle, State_Active, State_Processing}, sm.subject.GetStateReader().GetCurrentState(), "Current state is not in any of the allowed states")
}

// Test guard combinators

func TestGuardNot(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{ProcessedCount: 5}

	guard := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount > 10
	}

	notGuard := Not(guard)

	assert.False(t, guard(ctx, subject, subject))
	assert.True(t, notGuard(ctx, subject, subject))
}

func TestGuardAnd(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{ProcessedCount: 5, Name: "test"}

	guard1 := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount > 3
	}

	guard2 := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.Name == "test"
	}

	guard3 := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount > 10
	}

	assert.True(t, And(guard1, guard2)(ctx, subject, subject))
	assert.False(t, And(guard1, guard3)(ctx, subject, subject))
	assert.False(t, And(guard2, guard3)(ctx, subject, subject))
}

func TestGuardOr(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{ProcessedCount: 5, Name: "test"}

	guard1 := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount > 10
	}

	guard2 := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.Name == "test"
	}

	guard3 := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount > 100
	}

	assert.True(t, Or(guard1, guard2)(ctx, subject, subject))
	assert.True(t, Or(guard2, guard3)(ctx, subject, subject))
	assert.False(t, Or(guard1, guard3)(ctx, subject, subject))
}

func TestComplexGuardCombination(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{ProcessedCount: 5, Name: "test"}

	isActive := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount > 0
	}

	isNamed := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.Name != ""
	}

	isBusy := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount > 10
	}

	// (isActive AND isNamed) OR isBusy
	complexGuard := Or(And(isActive, isNamed), isBusy)
	assert.True(t, complexGuard(ctx, subject, subject))

	// NOT(isActive AND isNamed)
	negatedGuard := Not(And(isActive, isNamed))
	assert.False(t, negatedGuard(ctx, subject, subject))
}

func TestSetState(t *testing.T) {
	config := newTestConfig(map[TestState]TestStateDefinition{})

	sm := NewStateMachine(&TestSubject{}, config, State_Idle)
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())

	sm.subject.GetStateMutator().SetCurrentState(State_Active)

	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
}

func TestTimeSinceStateChange(t *testing.T) {
	config := newTestConfig(map[TestState]TestStateDefinition{})

	sm := NewStateMachine(&TestSubject{}, config, State_Idle)

	time.Sleep(5 * time.Millisecond)
	duration := sm.TimeSinceStateChange()

	assert.True(t, duration >= 5*time.Millisecond)
}

func TestOnHandleEventErrorPreventsTransition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("state update failed")
	stateupdate_Failing := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		return expectedErr
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Failing}},
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	assert.ErrorIs(t, err, expectedErr)
	// State should not change on OnHandleEvent error
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())
}

func TestValidatorError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("validator error")
	validator := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) (bool, error) {
		return false, expectedErr
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Validator: validator,
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())
}

func TestNoStateDefinition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	// Config with no definition for State_Idle
	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Active: {}, // Only Active is defined
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	// Event should be ignored since State_Idle has no definition
	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())
}

func TestTransitionOnActionError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("transition action failed")
	failingTransitionAction := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		return expectedErr
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Transitions: []TestTransition{
						{
							To: State_Active,
							On: failingTransitionAction,
						},
					},
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	assert.ErrorIs(t, err, expectedErr)
	// State should not change on transition action error
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())
}

func TestOnTransitionToError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("OnTransitionTo failed")
	failingEntryAction := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		return expectedErr
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {
			OnTransitionTo: TestTransitionToHandler{
				Actions: []TestActionRule{{Action: failingEntryAction}},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	assert.ErrorIs(t, err, expectedErr)
	// Note: state has already changed to Active before OnTransitionTo runs
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
}

func TestTransitionToStateWithNoDefinition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Transitions: []TestTransition{
						{To: State_Active}, // State_Active has no definition
					},
				},
			},
		},
		// No definition for State_Active
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	// Should succeed even if target state has no definition
	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
}

func TestEventWithNoTransitions(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	actionCalled := false
	action := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		actionCalled = true
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Active: {
			Events: map[EventType]TestEventHandler{
				Event_Heartbeat: {
					Actions: []TestActionRule{
						{Action: action},
					},
					// No transitions defined - event only triggers actions
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Active)
	event := NewTestEvent(Event_Heartbeat, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.True(t, actionCalled)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState()) // No state change
}

func TestGuardAndEmptyList(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{}

	// And with no guards should return true (vacuous truth)
	emptyAnd := And[TestState, *TestSubject, *TestSubject]()
	assert.True(t, emptyAnd(ctx, subject, subject))
}

func TestGuardOrEmptyList(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{}

	// Or with no guards should return false
	emptyOr := Or[TestState, *TestSubject, *TestSubject]()
	assert.False(t, emptyOr(ctx, subject, subject))
}

// Tests for OnHandleEvent functionality

func TestOnHandleEventBasic(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	stateupdate_Start := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		testEvent := event.(*TestEvent)
		state.LastData = testEvent.data
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Start}},
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "event_data")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
	assert.Equal(t, "event_data", subject.LastData)
}

func TestOnHandleEventCalledBeforeGuards(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 0}

	// OnHandleEvent sets ProcessedCount, guard checks it
	stateupdate_Process := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		state.ProcessedCount = 10
		return nil
	}

	guardProcessedEnough := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount >= 5
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Processing: {
			Events: map[EventType]TestEventHandler{
				Event_Process: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Process}},
					Transitions: []TestTransition{
						{
							To: State_Complete,
							If: guardProcessedEnough,
						},
					},
				},
			},
		},
		State_Complete: {},
	})

	sm := NewStateMachine(subject, config, State_Processing)
	event := NewTestEvent(Event_Process, "")

	// Before processing, ProcessedCount is 0, but OnHandleEvent will set it to 10
	// The guard should see the updated value and allow the transition
	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Complete, sm.subject.GetStateReader().GetCurrentState())
	assert.Equal(t, 10, subject.ProcessedCount)
}

func TestOnHandleEventError(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	expectedErr := errors.New("state update failed")
	stateupdate_Failing := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		return expectedErr
	}

	actionCalled := false
	action := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		actionCalled = true
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Failing}},
					Actions: []TestActionRule{
						{Action: action},
					},
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	assert.ErrorIs(t, err, expectedErr)
	// State should not change on OnHandleEvent error
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())
	// Actions should not be called when OnHandleEvent fails
	assert.False(t, actionCalled)
}

func TestOnHandleEventWithActions(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	callOrder := []string{}

	stateupdate_Track := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		callOrder = append(callOrder, "onhandle")
		return nil
	}

	action := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		callOrder = append(callOrder, "action")
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Track}},
					Actions: []TestActionRule{
						{Action: action},
					},
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	// Verify order: OnHandleEvent called before actions
	assert.Equal(t, []string{"onhandle", "action"}, callOrder)
}

func TestOnHandleEventReceivesCorrectEvent(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	var receivedEventType EventType
	var receivedEventData string

	stateupdate_Capture := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		testEvent := event.(*TestEvent)
		receivedEventType = testEvent.Type()
		receivedEventData = testEvent.data
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Capture}},
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "test_payload")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, Event_Start, receivedEventType)
	assert.Equal(t, "test_payload", receivedEventData)
}

func TestOnHandleEventWithValidator(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	validatorCalled := false
	onHandleEventCalled := false

	validator := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) (bool, error) {
		validatorCalled = true
		testEvent := event.(*TestEvent)
		return testEvent.data == "valid", nil
	}

	stateupdate_Track := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		onHandleEventCalled = true
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					Validator:    validator,
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Track}},
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {},
	})

	sm := NewStateMachine(subject, config, State_Idle)

	// Test with invalid event - validator fails, OnHandleEvent should NOT be called
	invalidEvent := NewTestEvent(Event_Start, "invalid")
	err := sm.ProcessEvent(ctx, invalidEvent)
	require.NoError(t, err)
	assert.True(t, validatorCalled)
	assert.False(t, onHandleEventCalled)
	assert.Equal(t, State_Idle, sm.subject.GetStateReader().GetCurrentState())

	// Reset for next test
	validatorCalled = false
	onHandleEventCalled = false

	// Test with valid event - validator passes, OnHandleEvent should be called
	validEvent := NewTestEvent(Event_Start, "valid")
	err = sm.ProcessEvent(ctx, validEvent)
	require.NoError(t, err)
	assert.True(t, validatorCalled)
	assert.True(t, onHandleEventCalled)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
}

func TestOnHandleEventWithTransitionOnAction(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	callOrder := []string{}

	stateupdate_Track := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		callOrder = append(callOrder, "onhandle")
		return nil
	}

	transitionAction := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		callOrder = append(callOrder, "transition_on")
		return nil
	}

	entryAction := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		callOrder = append(callOrder, "entry")
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Track}},
					Transitions: []TestTransition{
						{
							To: State_Active,
							On: transitionAction,
						},
					},
				},
			},
		},
		State_Active: {
			OnTransitionTo: TestTransitionToHandler{
				Actions: []TestActionRule{{Action: entryAction}},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Idle)
	event := NewTestEvent(Event_Start, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	// Verify order: OnHandleEvent -> transition On action -> OnTransitionTo
	assert.Equal(t, []string{"onhandle", "transition_on", "entry"}, callOrder)
}

func TestOnHandleEventNoTransition(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	onHandleEventCalled := false
	stateupdate_NoTransition := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		onHandleEventCalled = true
		state.ProcessedCount++
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Active: {
			Events: map[EventType]TestEventHandler{
				Event_Heartbeat: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_NoTransition}},
					// No transitions defined - event only triggers state update
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Active)
	event := NewTestEvent(Event_Heartbeat, "")

	err := sm.ProcessEvent(ctx, event)
	require.NoError(t, err)
	assert.True(t, onHandleEventCalled)
	assert.Equal(t, 1, subject.ProcessedCount)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState()) // No state change
}

func TestMultipleEventsWithDifferentOnHandleEvents(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test"}

	stateupdate_Start := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		state.ActionsCalled = append(state.ActionsCalled, "stateupdate_start")
		return nil
	}

	stateupdate_Process := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		state.ActionsCalled = append(state.ActionsCalled, "stateupdate_process")
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Idle: {
			Events: map[EventType]TestEventHandler{
				Event_Start: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Start}},
					Transitions: []TestTransition{
						{To: State_Active},
					},
				},
			},
		},
		State_Active: {
			Events: map[EventType]TestEventHandler{
				Event_Process: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_Process}},
					Transitions: []TestTransition{
						{To: State_Processing},
					},
				},
			},
		},
		State_Processing: {},
	})

	sm := NewStateMachine(subject, config, State_Idle)

	// First event
	err := sm.ProcessEvent(ctx, NewTestEvent(Event_Start, ""))
	require.NoError(t, err)
	assert.Equal(t, State_Active, sm.subject.GetStateReader().GetCurrentState())
	assert.Equal(t, []string{"stateupdate_start"}, subject.ActionsCalled)

	// Second event - different OnHandleEvent
	err = sm.ProcessEvent(ctx, NewTestEvent(Event_Process, ""))
	require.NoError(t, err)
	assert.Equal(t, State_Processing, sm.subject.GetStateReader().GetCurrentState())
	assert.Equal(t, []string{"stateupdate_start", "stateupdate_process"}, subject.ActionsCalled)
}

func TestOnHandleEventWithGuardedActions(t *testing.T) {
	ctx := context.Background()
	subject := &TestSubject{Name: "test", ProcessedCount: 0}

	// OnHandleEvent sets the state that guards will check
	stateupdate_SetCount := func(ctx context.Context, state *TestSubject, config *TestSubject, callbacks *TestSubject, event common.Event) error {
		testEvent := event.(*TestEvent)
		if testEvent.data == "high" {
			state.ProcessedCount = 10
		}
		return nil
	}

	guardHigh := func(ctx context.Context, reader *TestSubject, smContext *TestSubject) bool {
		return reader.ProcessedCount >= 5
	}

	highAction := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		reader.ActionsCalled = append(reader.ActionsCalled, "high_action")
		return nil
	}

	lowAction := func(ctx context.Context, reader *TestSubject, config *TestSubject, callbacks *TestSubject, _ common.Event) error {
		reader.ActionsCalled = append(reader.ActionsCalled, "low_action")
		return nil
	}

	config := newTestConfig(map[TestState]TestStateDefinition{
		State_Active: {
			Events: map[EventType]TestEventHandler{
				Event_Process: {
					StateUpdates: []TestStateUpdateRule{{StateUpdate: stateupdate_SetCount}},
					Actions: []TestActionRule{
						{Action: highAction, If: guardHigh},
						{Action: lowAction, If: Not(guardHigh)},
					},
				},
			},
		},
	})

	sm := NewStateMachine(subject, config, State_Active)

	// Low event - guard should fail, lowAction should be called
	err := sm.ProcessEvent(ctx, NewTestEvent(Event_Process, "low"))
	require.NoError(t, err)
	assert.Equal(t, []string{"low_action"}, subject.ActionsCalled)

	// Reset
	subject.ActionsCalled = nil
	subject.ProcessedCount = 0

	// High event - OnHandleEvent sets count, guard should pass, highAction should be called
	err = sm.ProcessEvent(ctx, NewTestEvent(Event_Process, "high"))
	require.NoError(t, err)
	assert.Equal(t, []string{"high_action"}, subject.ActionsCalled)
}
