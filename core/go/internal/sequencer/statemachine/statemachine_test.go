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
	"sync"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test state types
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
	Event_Start    common.EventType = 100
	Event_Process  common.EventType = 101
	Event_Complete common.EventType = 102
	Event_Fail     common.EventType = 103
	Event_Reset    common.EventType = 104
)

// Test entity. Embeds sync.Mutex to implement Lockable for use with the state machine.
type TestEntity struct {
	sync.Mutex
	sm           *StateMachine[TestState, *TestEntity]
	counter      int
	lastAction   string
	shouldFail   bool
	canProcess   bool
	applyCounter int
	// ProcessOrder records event type strings in processing order (used by priority queue tests)
	ProcessOrder []string
}

func newTestEntity(definitions StateDefinitions[TestState, *TestEntity], opts ...StateMachineOption[TestState, *TestEntity]) *TestEntity {
	e := &TestEntity{
		canProcess: true,
	}
	e.sm = NewStateMachine(State_Idle, definitions, opts...)
	return e
}

// Test event
type testEvent struct {
	common.BaseEvent
	eventType common.EventType
	data      string
}

func (e *testEvent) Type() common.EventType {
	return e.eventType
}

func (e *testEvent) TypeString() string {
	switch e.eventType {
	case Event_Start:
		return "Event_Start"
	case Event_Process:
		return "Event_Process"
	case Event_Complete:
		return "Event_Complete"
	case Event_Fail:
		return "Event_Fail"
	case Event_Reset:
		return "Event_Reset"
	}
	return "Unknown"
}

func newTestEvent(eventType common.EventType) *testEvent {
	return &testEvent{
		BaseEvent: common.BaseEvent{EventTime: time.Now()},
		eventType: eventType,
	}
}

func TestBasicStateMachine(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
		State_Active: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Process: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Processing,
					}},
				},
			},
		},
		State_Processing: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Complete: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Complete,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	// Initial state
	assert.Equal(t, State_Idle, entity.sm.GetCurrentState())

	// Transition: Idle -> Active
	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, State_Active, entity.sm.GetCurrentState())

	// Transition: Active -> Processing
	err = entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Process))
	require.NoError(t, err)
	assert.Equal(t, State_Processing, entity.sm.GetCurrentState())

	// Transition: Processing -> Complete
	err = entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Complete))
	require.NoError(t, err)
	assert.Equal(t, State_Complete, entity.sm.GetCurrentState())
}

func TestGuardedTransitions(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{
						{
							To: State_Error,
							If: func(ctx context.Context, e *TestEntity) bool {
								return e.shouldFail
							},
						},
						{
							To: State_Active,
							If: func(ctx context.Context, e *TestEntity) bool {
								return !e.shouldFail
							},
						},
					},
				},
			},
		},
	}

	ctx := context.Background()

	// Test guard allowing transition to Active
	entity1 := newTestEntity(definitions)
	entity1.shouldFail = false
	err := entity1.sm.ProcessEvent(ctx, entity1, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, State_Active, entity1.sm.GetCurrentState())

	// Test guard allowing transition to Error
	entity2 := newTestEntity(definitions)
	entity2.shouldFail = true
	err = entity2.sm.ProcessEvent(ctx, entity2, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, State_Error, entity2.sm.GetCurrentState())
}

func TestActionsOnTransition(t *testing.T) {
	actionCalled := false
	entryActionCalled := false

	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							actionCalled = true
							e.lastAction = "transition_action"
							return nil
						},
					}},
				},
			},
		},
		State_Active: {
			OnTransitionTo: func(ctx context.Context, e *TestEntity, event common.Event) error {
				entryActionCalled = true
				e.lastAction = "entry_action"
				return nil
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	require.NoError(t, err)

	assert.True(t, actionCalled, "transition action should be called")
	assert.True(t, entryActionCalled, "entry action should be called")
	// Entry action runs after transition action
	assert.Equal(t, "entry_action", entity.lastAction)
}

func TestEventHandlerActions(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{
						{
							Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
								e.counter++
								return nil
							},
						},
						{
							Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
								e.counter += 10
								return nil
							},
							If: func(ctx context.Context, e *TestEntity) bool {
								return e.canProcess
							},
						},
					},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	ctx := context.Background()

	// Test with canProcess = true
	entity1 := newTestEntity(definitions)
	entity1.canProcess = true
	err := entity1.sm.ProcessEvent(ctx, entity1, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, 11, entity1.counter) // 1 + 10

	// Test with canProcess = false
	entity2 := newTestEntity(definitions)
	entity2.canProcess = false
	err = entity2.sm.ProcessEvent(ctx, entity2, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, 1, entity2.counter) // only first action
}

func TestEventValidator(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Validator: func(ctx context.Context, e *TestEntity, event common.Event) (bool, error) {
						return e.canProcess, nil
					},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	ctx := context.Background()

	// Test with valid event
	entity1 := newTestEntity(definitions)
	entity1.canProcess = true
	err := entity1.sm.ProcessEvent(ctx, entity1, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, State_Active, entity1.sm.GetCurrentState())

	// Test with invalid event
	entity2 := newTestEntity(definitions)
	entity2.canProcess = false
	err = entity2.sm.ProcessEvent(ctx, entity2, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, State_Idle, entity2.sm.GetCurrentState()) // No transition
}

func TestFirstActionAppliesEventData(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							e.applyCounter++
							return nil
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, 1, entity.applyCounter)
}

func TestTransitionCallback(t *testing.T) {
	var callbackFrom, callbackTo TestState
	var callbackCalled bool

	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	callback := func(ctx context.Context, e *TestEntity, from, to TestState, event common.Event) {
		callbackCalled = true
		callbackFrom = from
		callbackTo = to
	}

	entity := newTestEntity(definitions, WithTransitionCallback(callback))
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	require.NoError(t, err)

	assert.True(t, callbackCalled)
	assert.Equal(t, State_Idle, callbackFrom)
	assert.Equal(t, State_Active, callbackTo)
}

func TestUnhandledEvent(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	// Event_Process is not handled in State_Idle
	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Process))
	require.NoError(t, err)
	assert.Equal(t, State_Idle, entity.sm.GetCurrentState()) // No change
}

func TestActionError(t *testing.T) {
	expectedErr := errors.New("action failed")

	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{
						{
							Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
								return expectedErr
							},
						},
					},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	assert.Equal(t, expectedErr, err)
	// State should not change on error
	assert.Equal(t, State_Idle, entity.sm.GetCurrentState())
}

func TestStateMachineMetadata(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	beforeTransition := time.Now()
	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	require.NoError(t, err)

	assert.Equal(t, State_Active, entity.sm.GetCurrentState())
	assert.Equal(t, "Event_Start", entity.sm.GetLatestEvent())
	assert.True(t, entity.sm.GetLastStateChange().After(beforeTransition) || entity.sm.GetLastStateChange().Equal(beforeTransition))
}

func TestEventHandlerFirstAction(t *testing.T) {
	// Test that first action is called and can access event data
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							te := event.(*testEvent)
							e.lastAction = "first_" + te.data
							e.counter = 100
							return nil
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	event := &testEvent{
		BaseEvent: common.BaseEvent{EventTime: time.Now()},
		eventType: Event_Start,
		data:      "test_data",
	}

	err := entity.sm.ProcessEvent(ctx, entity, event)
	require.NoError(t, err)

	assert.Equal(t, State_Active, entity.sm.GetCurrentState())
	assert.Equal(t, "first_test_data", entity.lastAction)
	assert.Equal(t, 100, entity.counter)
}

func TestFirstActionBeforeGuardedActions(t *testing.T) {
	// Test that first action runs before guarded Actions
	var callOrder []string

	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{
						{
							Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
								callOrder = append(callOrder, "first")
								e.counter = 50 // Set a value that the guard can check
								return nil
							},
						},
						{
							Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
								callOrder = append(callOrder, "action")
								return nil
							},
							If: func(ctx context.Context, e *TestEntity) bool {
								return e.counter == 50
							},
						},
					},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	require.NoError(t, err)

	assert.Equal(t, []string{"first", "action"}, callOrder)
}

func TestFirstActionError(t *testing.T) {
	expectedErr := errors.New("first action failed")

	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							return expectedErr
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	assert.Equal(t, expectedErr, err)
	// State should not change on error
	assert.Equal(t, State_Idle, entity.sm.GetCurrentState())
}

func TestEventValidatorReturnsError(t *testing.T) {
	validationErr := errors.New("validation failed")

	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Validator: func(ctx context.Context, e *TestEntity, event common.Event) (bool, error) {
						return false, validationErr
					},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	assert.Equal(t, validationErr, err)
	assert.Equal(t, State_Idle, entity.sm.GetCurrentState())
}

func TestTransitionOnActionError(t *testing.T) {
	transitionErr := errors.New("transition action failed")

	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							return transitionErr
						},
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	assert.Equal(t, transitionErr, err)
	assert.Equal(t, State_Active, entity.sm.GetCurrentState())
}

func TestStateEntryActionError(t *testing.T) {
	entryErr := errors.New("entry action failed")

	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
		State_Active: {
			OnTransitionTo: func(ctx context.Context, e *TestEntity, event common.Event) error {
				return entryErr
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	assert.Equal(t, entryErr, err)
	assert.Equal(t, State_Active, entity.sm.GetCurrentState())
}

func TestTransitionToStateWithNoEntryAction(t *testing.T) {
	// Transition to a state that has no OnTransitionTo (or state not in definitions)
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Complete, // State_Complete has no entry in definitions
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	ctx := context.Background()

	err := entity.sm.ProcessEvent(ctx, entity, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, State_Complete, entity.sm.GetCurrentState())
}

// StateMachineEventLoop tests

func TestNewStateMachineEventLoop_Basic(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	require.NotNil(t, sel)
	assert.Equal(t, State_Idle, sel.GetCurrentState())
	assert.NotNil(t, sel.StateMachine())
}

func TestStateMachineEventLoop_StartStopAndMethods(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
		State_Active: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Process: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Complete,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState:        State_Idle,
		Definitions:         definitions,
		Entity:              entity,
		EventLoopBufferSize: 10,
		Name:                "pel-test",
	})

	ctx := context.Background()

	assert.False(t, sel.IsRunning())
	assert.False(t, sel.IsStopped())

	go sel.Start(ctx)
	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	assert.True(t, sel.IsRunning())

	sel.QueueEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	assert.Equal(t, State_Active, sel.GetCurrentState())

	ok := sel.TryQueueEvent(ctx, newTestEvent(Event_Process))
	assert.True(t, ok)
	syncEv3 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv3)
	<-syncEv3.Done

	assert.Equal(t, State_Complete, sel.GetCurrentState())

	sel.Stop()
	assert.True(t, sel.IsStopped())
	assert.False(t, sel.IsRunning())
}

func TestStateMachineEventLoop_ProcessEventSync(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()

	err := sel.ProcessEvent(ctx, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, State_Active, sel.GetCurrentState())
}

func TestStateMachineEventLoop_StopAsyncWaitForStop(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()

	go sel.Start(ctx)
	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.StopAsync()
	sel.WaitForStop()

	assert.True(t, sel.IsStopped())
}

func TestStateMachineEventLoop_WithOnStop(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
		OnStop: func(ctx context.Context) common.Event {
			return newTestEvent(Event_Reset)
		},
	})

	ctx := context.Background()

	go sel.Start(ctx)
	sel.QueueEvent(ctx, newTestEvent(Event_Start))
	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done
	sel.Stop()

	assert.True(t, sel.IsStopped())
}

func TestStateMachineEventLoop_WithTransitionCallback(t *testing.T) {
	var fromState, toState TestState
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
		TransitionCallback: func(ctx context.Context, e *TestEntity, from, to TestState, event common.Event) {
			fromState = from
			toState = to
		},
	})

	ctx := context.Background()

	err := sel.ProcessEvent(ctx, newTestEvent(Event_Start))
	require.NoError(t, err)
	assert.Equal(t, State_Idle, fromState)
	assert.Equal(t, State_Active, toState)
}

func TestStateMachineEventLoop_WithPreProcessHandled(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	var preHandled bool
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
		PreProcess: func(ctx context.Context, e *TestEntity, event common.Event) (bool, error) {
			if event.Type() == Event_Start {
				preHandled = true
				return true, nil // fully handled, don't pass to state machine
			}
			return false, nil
		},
	})

	ctx := context.Background()

	// PreProcess is used when events go through the event loop
	go sel.Start(ctx)
	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.QueueEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	sel.Stop()

	assert.True(t, preHandled)
	// State should still be Idle because PreProcess handled the event
	assert.Equal(t, State_Idle, sel.GetCurrentState())
}

func TestStateMachineEventLoop_WithPreProcessError(t *testing.T) {
	preErr := errors.New("preprocess failed")
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
		PreProcess: func(ctx context.Context, e *TestEntity, event common.Event) (bool, error) {
			return false, preErr
		},
	})

	ctx := context.Background()

	// PreProcess error is surfaced when events go through the event loop
	go sel.Start(ctx)
	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.QueueEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	sel.Stop()

	// Event loop processed the event; PreProcess returned error (logged, loop continues)
	assert.True(t, sel.IsStopped())
	assert.Equal(t, State_Idle, sel.GetCurrentState())
}

func TestStateMachineEventLoop_ZeroBufferSizeUsesDefault(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState:        State_Idle,
		Definitions:         definitions,
		Entity:              entity,
		EventLoopBufferSize: 0,
	})

	ctx := context.Background()
	go sel.Start(ctx)
	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.QueueEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done
	sel.Stop()

	assert.Equal(t, State_Active, sel.GetCurrentState())
}

// TestStateMachineEventLoop_PriorityQueueDrainedBeforeMain verifies that the priority queue
// is fully drained before any event from the main queue is processed.
func TestStateMachineEventLoop_PriorityQueueDrainedBeforeMain(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							e.ProcessOrder = append(e.ProcessOrder, event.TypeString())
							return nil
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
		State_Active: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Process: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							e.ProcessOrder = append(e.ProcessOrder, event.TypeString())
							return nil
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Complete,
					}},
				},
			},
		},
		State_Complete: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Reset: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							e.ProcessOrder = append(e.ProcessOrder, event.TypeString())
							return nil
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Idle,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	entity.ProcessOrder = make([]string, 0, 4)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState:        State_Idle,
		Definitions:         definitions,
		Entity:              entity,
		EventLoopBufferSize: 10,
		Name:                "priority-drain-test",
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	// Queue: two priority events (Start, Process) then one main event (Reset).
	// Priority queue must be drained first, so order must be Start, Process, Reset.
	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Start))
	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Process))
	sel.QueueEvent(ctx, newTestEvent(Event_Reset))

	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	sel.Stop()

	assert.Equal(t, []string{"Event_Start", "Event_Process", "Event_Reset"}, entity.ProcessOrder)
	assert.Equal(t, State_Idle, sel.GetCurrentState())
}

// TestStateMachineEventLoop_QueuePriorityEvent verifies that QueuePriorityEvent delivers
// events and they are processed by the state machine.
func TestStateMachineEventLoop_QueuePriorityEvent(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
		State_Active: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Process: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Complete,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
		Name:         "priority-queue-test",
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	sel.QueuePriorityEvent(ctx, syncEv2)
	<-syncEv2.Done

	assert.Equal(t, State_Active, sel.GetCurrentState())

	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Process))
	syncEv3 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv3)
	<-syncEv3.Done

	assert.Equal(t, State_Complete, sel.GetCurrentState())
	sel.Stop()
}

// TestStateMachineEventLoop_TryQueuePriorityEvent verifies TryQueuePriorityEvent returns
// true when the priority buffer has space and false when full.
func TestStateMachineEventLoop_TryQueuePriorityEvent(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	blockUntilRelease := make(chan struct{})
	blockStarted := make(chan struct{})
	blockCount := 0
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState:               State_Idle,
		Definitions:                 definitions,
		Entity:                      entity,
		EventLoopBufferSize:         10,
		PriorityEventLoopBufferSize: 2,
		Name:                        "try-priority-test",
		PreProcess: func(ctx context.Context, e *TestEntity, event common.Event) (bool, error) {
			blockCount++
			if blockCount == 1 {
				blockStarted <- struct{}{}
				<-blockUntilRelease
			}
			return false, nil
		},
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	// One priority event will be processed and block in PreProcess, leaving priority buffer empty.
	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Start))
	<-blockStarted // wait for loop to be blocking in PreProcess

	ok1 := sel.TryQueuePriorityEvent(ctx, newTestEvent(Event_Start))
	ok2 := sel.TryQueuePriorityEvent(ctx, newTestEvent(Event_Start))
	require.True(t, ok1)
	require.True(t, ok2)

	// Third should fail (buffer full)
	ok3 := sel.TryQueuePriorityEvent(ctx, newTestEvent(Event_Start))
	assert.False(t, ok3)

	close(blockUntilRelease)
	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done
	sel.Stop()
}

// TestStateMachineEventLoop_PrioritySyncEvent verifies that a sync event sent on the
// priority queue is processed and signals Done.
func TestStateMachineEventLoop_PrioritySyncEvent(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
		Name:         "priority-sync-test",
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueuePriorityEvent(ctx, syncEv)
	<-syncEv.Done

	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	sel.QueuePriorityEvent(ctx, syncEv2)
	<-syncEv2.Done

	assert.Equal(t, State_Active, sel.GetCurrentState())
	sel.Stop()
}

// TestStateMachineEventLoop_PriorityEventLoopBufferSize verifies that PriorityEventLoopBufferSize
// is used when set (buffer size 3 allows 3 queued, 4th TryQueuePriorityEvent fails).
func TestStateMachineEventLoop_PriorityEventLoopBufferSize(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	blockUntilRelease := make(chan struct{})
	blockStarted := make(chan struct{})
	blockCount := 0
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState:                State_Idle,
		Definitions:                 definitions,
		Entity:                      entity,
		EventLoopBufferSize:         50,
		PriorityEventLoopBufferSize: 3,
		Name:                        "priority-buffer-size-test",
		PreProcess: func(ctx context.Context, e *TestEntity, event common.Event) (bool, error) {
			blockCount++
			if blockCount == 1 {
				blockStarted <- struct{}{}
				<-blockUntilRelease
			}
			return false, nil
		},
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	// Block the loop on the first priority event so the buffer can fill
	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Start))
	<-blockStarted

	// Should be able to queue 3 more priority events (buffer size 3)
	require.True(t, sel.TryQueuePriorityEvent(ctx, newTestEvent(Event_Start)))
	require.True(t, sel.TryQueuePriorityEvent(ctx, newTestEvent(Event_Start)))
	require.True(t, sel.TryQueuePriorityEvent(ctx, newTestEvent(Event_Start)))
	// Fourth should fail
	assert.False(t, sel.TryQueuePriorityEvent(ctx, newTestEvent(Event_Start)))

	close(blockUntilRelease)
	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done
	sel.Stop()
}

// TestStateMachineEventLoop_TryQueueEvent_BufferFull verifies TryQueueEvent returns false when main buffer is full.
func TestStateMachineEventLoop_TryQueueEvent_BufferFull(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	blockUntilRelease := make(chan struct{})
	blockStarted := make(chan struct{})
	blockCount := 0
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState:               State_Idle,
		Definitions:                definitions,
		Entity:                     entity,
		EventLoopBufferSize:        2,
		PriorityEventLoopBufferSize: 10,
		Name:                       "try-queue-test",
		PreProcess: func(ctx context.Context, e *TestEntity, event common.Event) (bool, error) {
			blockCount++
			if blockCount == 1 {
				blockStarted <- struct{}{}
				<-blockUntilRelease
			}
			return false, nil
		},
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.QueueEvent(ctx, newTestEvent(Event_Start))
	<-blockStarted

	ok1 := sel.TryQueueEvent(ctx, newTestEvent(Event_Start))
	ok2 := sel.TryQueueEvent(ctx, newTestEvent(Event_Start))
	require.True(t, ok1)
	require.True(t, ok2)
	ok3 := sel.TryQueueEvent(ctx, newTestEvent(Event_Start))
	assert.False(t, ok3)

	close(blockUntilRelease)
	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done
	sel.Stop()
}

// TestStateMachineEventLoop_Stop_WhenAlreadyStopped verifies Stop returns immediately when loop is already stopped.
func TestStateMachineEventLoop_Stop_WhenAlreadyStopped(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.Stop()
	assert.True(t, sel.IsStopped())

	// Second Stop() should return immediately (already stopped)
	sel.Stop()
}

// TestStateMachineEventLoop_Stop_ConcurrentCalls verifies concurrent Stop() calls; one hits default when stopLoop is full.
func TestStateMachineEventLoop_Stop_ConcurrentCalls(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	// Use a barrier so two goroutines reach Stop()'s second select at the same time;
	// one send to stopLoop succeeds, the other hits default (buffer full).
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-barrier
		sel.Stop()
	}()
	go func() {
		defer wg.Done()
		<-barrier
		sel.Stop()
	}()
	close(barrier)
	wg.Wait()

	assert.True(t, sel.IsStopped())
}

// TestStateMachineEventLoop_StopAsync_WhenAlreadyStopped verifies StopAsync returns immediately when loop is already stopped.
func TestStateMachineEventLoop_StopAsync_WhenAlreadyStopped(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.Stop()
	assert.True(t, sel.IsStopped())

	// StopAsync when already stopped should return immediately
	sel.StopAsync()
}

// TestStateMachineEventLoop_StopAsync_ConcurrentCalls verifies concurrent StopAsync() calls; one hits default.
func TestStateMachineEventLoop_StopAsync_ConcurrentCalls(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	// Use a barrier so two goroutines reach StopAsync()'s second select at the same time;
	// one send succeeds, the other hits default (stopLoop buffer full).
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-barrier
		sel.StopAsync()
	}()
	go func() {
		defer wg.Done()
		<-barrier
		sel.StopAsync()
	}()
	close(barrier)
	wg.Wait()

	sel.WaitForStop()
	assert.True(t, sel.IsStopped())
}

// TestStateMachineEventLoop_ContextCancelled verifies the loop exits when context is cancelled.
func TestStateMachineEventLoop_ContextCancelled(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sel.Start(ctx)
		close(done)
	}()

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	cancel()
	<-done

	assert.True(t, sel.IsStopped())
}

// TestStateMachineEventLoop_OnStopFinalEventError verifies that when OnStop returns an event and processEvent
// returns an error, the error is logged and the loop still stops.
func TestStateMachineEventLoop_OnStopFinalEventError(t *testing.T) {
	processErr := errors.New("final event failed")
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
				Event_Reset: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							return processErr
						},
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
		OnStop: func(ctx context.Context) common.Event {
			return newTestEvent(Event_Reset)
		},
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.Stop()
	assert.True(t, sel.IsStopped())
}

// TestStateMachineEventLoop_ProcessEventError_Priority verifies that when a priority event causes
// processEvent to return an error, the error is logged and the loop continues.
func TestStateMachineEventLoop_ProcessEventError_Priority(t *testing.T) {
	processErr := errors.New("priority event failed")
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							return processErr
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	// State should still be Idle because the action returned error (transition not applied)
	assert.Equal(t, State_Idle, sel.GetCurrentState())
	sel.Stop()
}

// TestStateMachineEventLoop_ProcessEventError_Main verifies that when a main-queue event causes
// processEvent to return an error, the error is logged and the loop continues.
func TestStateMachineEventLoop_ProcessEventError_Main(t *testing.T) {
	processErr := errors.New("main event failed")
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							return processErr
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	sel.QueueEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	assert.Equal(t, State_Idle, sel.GetCurrentState())
	sel.Stop()
}

// TestStateMachineEventLoop_ProcessEventError_PriorityDrain verifies that when a priority event
// processed in the drain loop causes processEvent to return an error, the error is logged.
func TestStateMachineEventLoop_ProcessEventError_PriorityDrain(t *testing.T) {
	processErr := errors.New("priority drain failed")
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
		State_Active: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Process: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							return processErr
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Complete,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	// Two priority events: first (Start) transitions Idle->Active; second (Process) runs action that errors in drain.
	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Start))
	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Process))

	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	assert.Equal(t, State_Active, sel.GetCurrentState())
	sel.Stop()
}

// TestStateMachineEventLoop_SyncEventFromPrioritySelect verifies a sync event sent on the priority queue
// is processed when received from the blocking select (not the drain).
func TestStateMachineEventLoop_SyncEventFromPrioritySelect(t *testing.T) {
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done
	// Loop is blocking on select with empty priority queue. Send only a sync on priority so that case is taken.
	syncEv2 := NewSyncEvent()
	sel.QueuePriorityEvent(ctx, syncEv2)
	<-syncEv2.Done

	sel.Stop()
}

// TestStateMachineEventLoop_ProcessEventError_PriorityFromSelect verifies that when a priority event
// is received from the blocking select (not the drain) and processEvent returns an error, the error is logged.
func TestStateMachineEventLoop_ProcessEventError_PriorityFromSelect(t *testing.T) {
	processErr := errors.New("priority from select failed")
	definitions := StateDefinitions[TestState, *TestEntity]{
		State_Idle: {
			Events: map[common.EventType]EventHandler[TestState, *TestEntity]{
				Event_Start: {
					Actions: []ActionRule[*TestEntity]{{
						Action: func(ctx context.Context, e *TestEntity, event common.Event) error {
							return processErr
						},
					}},
					Transitions: []Transition[TestState, *TestEntity]{{
						To: State_Active,
					}},
				},
			},
		},
	}

	entity := newTestEntity(definitions)
	sel := NewStateMachineEventLoop(StateMachineEventLoopConfig[TestState, *TestEntity]{
		InitialState: State_Idle,
		Definitions:  definitions,
		Entity:       entity,
	})

	ctx := context.Background()
	go sel.Start(ctx)

	syncEv := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv)
	<-syncEv.Done
	// Loop is now blocking on select with empty priority queue. Send one priority event that errors.
	sel.QueuePriorityEvent(ctx, newTestEvent(Event_Start))

	syncEv2 := NewSyncEvent()
	sel.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	assert.Equal(t, State_Idle, sel.GetCurrentState())
	sel.Stop()
}
