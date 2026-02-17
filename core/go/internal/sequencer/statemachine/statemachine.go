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

// Package statemachine provides a generic, reusable state machine implementation
// that can be used across different packages in the sequencer module.
//
// Events must implement common.Event, which includes TypeString() string; the state
// machine uses TypeString() for consistent event labels in logs. State types must
// be comparable and implement String() string for transition log output.
//
// The state machine supports:
//   - Typed states and events
//   - Guards (conditions) for transitions
//   - Actions to be executed on events and transitions
//   - Event validation
//   - Entry actions when transitioning into a state
//
// Example usage:
//
//	type MyState int
//	const (
//	    State_Idle MyState = iota
//	    State_Active
//	)
//
//	type MyEntity struct {
//	    sync.Mutex  // implements Lockable for thread-safe ProcessEvent
//	    sm *statemachine.StateMachine[MyState, *MyEntity]
//	    counter int
//	}
//
//	// Define state definitions
//	definitions := statemachine.StateDefinitions[MyState, *MyEntity]{
//	    State_Idle: {
//	        Events: map[common.EventType]statemachine.EventHandler[MyState, *MyEntity]{
//	            Event_Activate: {
//	                Actions: []statemachine.ActionRule[*MyEntity]{{
//	                    Action: func(ctx context.Context, e *MyEntity, event common.Event) error {
//	                        e.counter++
//	                        return nil
//	                    },
//	                }},
//	                Transitions: []statemachine.Transition[MyState, *MyEntity]{{
//	                    To: State_Active,
//	                }},
//	            },
//	        },
//	    },
//	}
//
//	// Create a state machine
//	sm := statemachine.NewStateMachine(State_Idle, definitions)
//
//	// Process events
//	sm.ProcessEvent(ctx, entity, event)
package statemachine

import (
	"context"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

// State is a constraint for state types - must be comparable (typically int-based enums)
// and implement String() for use in transition logging.
type State interface {
	comparable
	String() string
}

// Lockable is an interface that entities must implement to support thread-safe
// event processing. The Lock is acquired before processing each event and
// released after the event has been fully processed.
type Lockable interface {
	Lock()
	Unlock()
}

// Action is a function that performs an action on an entity.
// All actions receive the event so they can apply event-specific data or perform side effects.
// Actions can be specified for:
//   - Event handling (Actions field in EventHandler; first action often applies event data)
//   - Specific transitions (Action field in Transition struct)
//   - Entry to a state (OnTransitionTo in StateDefinition)
type Action[E any] func(ctx context.Context, entity E, event common.Event) error

// Guard is a condition function that determines if a transition should be taken
// or if an action should be executed.
type Guard[E any] func(ctx context.Context, entity E) bool

// ActionRule pairs an action with an optional guard condition.
// If the guard (If) is nil, the action is always executed.
// If the guard returns true, the action is executed.
type ActionRule[E any] struct {
	Action Action[E]
	If     Guard[E]
}

// Transition defines a possible state transition.
// To: The target state to transition to
// If: Optional guard condition - if nil, transition is always taken (when matched)
// Action: Optional action to execute during this specific transition
type Transition[S State, E any] struct {
	To     S         // Target state
	If     Guard[E]  // Guard condition (optional)
	Action Action[E] // Transition-specific action (optional)
}

// Validator is a function that validates whether an event is valid for the current
// state of the entity. Returns true if valid, false if the event should be ignored.
// An error return indicates an unexpected validation failure.
type Validator[E any] func(ctx context.Context, entity E, event common.Event) (bool, error)

// EventHandler defines how an event is handled in a particular state.
// Validator: Optional function to validate the event
// Actions: List of guarded actions to execute when the event is received
// Transitions: Ordered list of possible transitions - first matching transition is taken
type EventHandler[S State, E any] struct {
	Validator   Validator[E]
	Actions     []ActionRule[E]
	Transitions []Transition[S, E]
}

// StateDefinition defines the behavior of a particular state.
// OnTransitionTo: Action executed when entering this state (after transition-specific actions)
// Events: Map of event types to their handlers in this state
type StateDefinition[S State, E any] struct {
	OnTransitionTo Action[E]
	Events         map[common.EventType]EventHandler[S, E]
}

// StateDefinitions is a map from states to their definitions.
type StateDefinitions[S State, E any] map[S]StateDefinition[S, E]

// TransitionCallback is called when a state transition occurs.
// It receives the entity, old state, new state, and the event that triggered the transition.
type TransitionCallback[S State, E any] func(ctx context.Context, entity E, from S, to S, event common.Event)

// OnStopCallback is called when the event loop receives a stop signal.
// It can optionally return a final event to process before stopping.
type OnStopCallback func(ctx context.Context) common.Event

// StateMachine holds the current state, metadata, and processing logic for a state machine instance.
// The entity type E must implement Lockable; the state machine holds the entity's lock
// for the duration of each ProcessEvent call.
type StateMachine[S State, E Lockable] struct {
	CurrentState       S
	LastStateChange    time.Time
	LatestEvent        string
	definitions        StateDefinitions[S, E]
	transitionCallback TransitionCallback[S, E]
	name               string // used in logging (e.g. transition logs)
}

// StateMachineOption is a functional option for configuring a StateMachine.
type StateMachineOption[S State, E Lockable] func(*StateMachine[S, E])

// WithTransitionCallback sets a callback that is invoked on state transitions.
func WithTransitionCallback[S State, E Lockable](cb TransitionCallback[S, E]) StateMachineOption[S, E] {
	return func(sm *StateMachine[S, E]) {
		sm.transitionCallback = cb
	}
}

// NewStateMachine creates a new state machine with the given initial state, definitions, and name.
// The entity type E must implement Lockable.
func NewStateMachine[S State, E Lockable](
	initialState S,
	definitions StateDefinitions[S, E],
	name string,
	opts ...StateMachineOption[S, E],
) *StateMachine[S, E] {
	sm := &StateMachine[S, E]{
		CurrentState:    initialState,
		LastStateChange: time.Now(),
		definitions:     definitions,
		name:            name,
	}
	for _, opt := range opts {
		opt(sm)
	}
	return sm
}

// ProcessEvent handles an event for the given entity.
// Returns nil if the event was processed successfully or was not applicable.
// Returns an error if validation, application, or actions fail.
//
// Processing order:
//  1. Evaluate if event is handled in current state
//  2. Validate the event (if validator defined)
//  3. Run Actions in order
//  4. Evaluate and perform transitions
func (sm *StateMachine[S, E]) ProcessEvent(
	ctx context.Context,
	entity E,
	event common.Event,
) error {
	entity.Lock()
	defer entity.Unlock()

	log.L(ctx).Debugf("%s | %s | processing event %s", sm.name, sm.CurrentState.String(), event.TypeString())

	// Evaluate whether this event is relevant for the current state
	eventHandler, err := sm.evaluateEvent(ctx, entity, event)
	if err != nil || eventHandler == nil {
		return err
	}

	// Execute Actions
	err = sm.performActions(ctx, entity, event, *eventHandler)
	if err != nil {
		return err
	}

	// Evaluate and perform any triggered transitions
	err = sm.evaluateTransitions(ctx, entity, event, *eventHandler)
	return err
}

// evaluateEvent determines if the event is relevant for the current state
// and returns the event handler if applicable.
func (sm *StateMachine[S, E]) evaluateEvent(
	ctx context.Context,
	entity E,
	event common.Event,
) (*EventHandler[S, E], error) {
	stateDefinition, exists := sm.definitions[sm.CurrentState]
	if !exists {
		return nil, nil
	}

	eventHandler, isHandlerDefined := stateDefinition.Events[event.Type()]
	if !isHandlerDefined {
		return nil, nil
	}

	// Validate the event if a validator is defined
	if eventHandler.Validator != nil {
		valid, err := eventHandler.Validator(ctx, entity, event)
		if err != nil {
			log.L(ctx).Errorf("%s | %s | error validating event %s: %v", sm.name, sm.CurrentState.String(), event.TypeString(), err)
			return nil, err
		}
		if !valid {
			log.L(ctx).Warnf("%s | %s | event %s not valid for current state", sm.name, sm.CurrentState.String(), event.TypeString())
			return nil, nil
		}
	}

	return &eventHandler, nil
}

// performActions executes the actions defined in the event handler (in order).
func (sm *StateMachine[S, E]) performActions(
	ctx context.Context,
	entity E,
	event common.Event,
	eventHandler EventHandler[S, E],
) error {
	for _, rule := range eventHandler.Actions {
		if rule.If == nil || rule.If(ctx, entity) {
			err := rule.Action(ctx, entity, event)
			if err != nil {
				log.L(ctx).Errorf("%s | %s | %s | error applying action: %v", sm.name, sm.CurrentState.String(), event.TypeString(), err)
				return err
			}
		}
	}
	return nil
}

// evaluateTransitions evaluates the transition rules and performs the first matching transition.
func (sm *StateMachine[S, E]) evaluateTransitions(
	ctx context.Context,
	entity E,
	event common.Event,
	eventHandler EventHandler[S, E],
) error {
	for _, rule := range eventHandler.Transitions {
		// Check if transition guard passes (or is nil)
		if rule.If == nil || rule.If(ctx, entity) {
			previousState := sm.CurrentState
			sm.CurrentState = rule.To
			sm.LatestEvent = event.TypeString()
			sm.LastStateChange = time.Now()

			// Execute transition-specific action first
			if rule.Action != nil {
				err := rule.Action(ctx, entity, event)
				if err != nil {
					log.L(ctx).Errorf("%s | %s | %s | error executing transition to state %s : %v", sm.name, previousState.String(), event.TypeString(), sm.CurrentState.String(), err)
					return err
				}
			}

			// Execute state entry action
			newStateDefinition, exists := sm.definitions[sm.CurrentState]
			if exists && newStateDefinition.OnTransitionTo != nil {
				err := newStateDefinition.OnTransitionTo(ctx, entity, event)
				if err != nil {
					log.L(ctx).Errorf("%s | %s | %s | error executing state entry action for transition to state %s : %v", sm.name, previousState.String(), event.TypeString(), sm.CurrentState.String(), err)
					return err
				}
			}

			// Transition logging (state machine is sequencer-only; uses state category)
			logCtx := log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)
			log.L(logCtx).Debugf("%s | %s | %s | transition to state %s",
				sm.name,
				previousState.String(),
				event.TypeString(),
				sm.CurrentState.String())

			// Invoke transition callback if set
			if sm.transitionCallback != nil {
				sm.transitionCallback(ctx, entity, previousState, sm.CurrentState, event)
			}

			// Only take the first matching transition
			break
		}
	}
	return nil
}

// GetCurrentState returns the current state of the state machine.
func (sm *StateMachine[S, E]) GetCurrentState() S {
	return sm.CurrentState
}

// GetLastStateChange returns the time of the last state change.
func (sm *StateMachine[S, E]) GetLastStateChange() time.Time {
	return sm.LastStateChange
}

// GetLatestEvent returns the type string of the last event that caused a transition.
func (sm *StateMachine[S, E]) GetLatestEvent() string {
	return sm.LatestEvent
}

// StateMachineEventLoop combines a StateMachine and an event loop into a single
// coordinated unit. This is the recommended way to use the state machine package
// as it handles all the wiring between components.
// The entity type E must implement Lockable to ensure thread-safe event processing.
// The priority queue is always fully drained before taking work from the main queue.
type StateMachineEventLoop[S State, E Lockable] struct {
	stateMachine   *StateMachine[S, E]
	entity         E
	events         chan common.Event
	eventsPriority chan common.Event
	stopLoop       chan struct{}
	loopStopped    chan struct{}
	onStop         OnStopCallback
	name           string
	running        bool
	processEvent   func(ctx context.Context, event common.Event) error
}

// StateMachineEventLoopConfig holds configuration for creating a StateMachineEventLoop.
// The entity type E must implement Lockable to ensure thread-safe event processing.
type StateMachineEventLoopConfig[S State, E Lockable] struct {
	// InitialState is the starting state for the state machine
	InitialState S

	// Definitions contains the state machine definitions
	Definitions StateDefinitions[S, E]

	// Entity is the entity that the state machine manages
	Entity E

	// EventQueueSize is the size of the event channel buffer.
	EventQueueSize int

	// PriorityEventQueueSize is the size of the priority event channel buffer.
	PriorityEventQueueSize int

	// Name for the event loop and the state machine; required. Used in logging (e.g. transition logs).
	// The same name is applied to both.
	Name string

	// OnStop callback invoked when the event loop stops, can return a final event to process
	OnStop OnStopCallback

	// TransitionCallback is invoked on state transitions (optional)
	TransitionCallback TransitionCallback[S, E]

	// PreProcess is an optional function called before the state machine handles each event.
	// If it returns an error, the event is not processed by the state machine.
	// If it returns true, the event was fully handled and should not be passed to the state machine.
	PreProcess func(ctx context.Context, entity E, event common.Event) (handled bool, err error)
}

// NewStateMachineEventLoop creates a new StateMachineEventLoop with all components wired together.
// This is the recommended way to create a state machine with event loop support.
// The entity type E must implement Lockable to ensure thread-safe event processing.
func NewStateMachineEventLoop[S State, E Lockable](config StateMachineEventLoopConfig[S, E]) *StateMachineEventLoop[S, E] {
	// Create the state machine; name is required and used for both the loop and the state machine.
	// The state machine holds the entity's lock for the duration of each ProcessEvent call.
	var smOpts []StateMachineOption[S, E]
	if config.TransitionCallback != nil {
		smOpts = append(smOpts, WithTransitionCallback(config.TransitionCallback))
	}
	sm := NewStateMachine(config.InitialState, config.Definitions, config.Name, smOpts...)

	// Event processor: PreProcess (own lock scope) then state machine ProcessEvent.
	processEvent := func(ctx context.Context, event common.Event) error {
		if config.PreProcess != nil {
			config.Entity.Lock()
			handled, err := config.PreProcess(ctx, config.Entity, event)
			config.Entity.Unlock()
			if err != nil {
				return err
			}
			if handled {
				return nil
			}
		}
		return sm.ProcessEvent(ctx, config.Entity, event)
	}

	sel := &StateMachineEventLoop[S, E]{
		stateMachine:   sm,
		entity:         config.Entity,
		events:         make(chan common.Event, config.EventQueueSize),
		eventsPriority: make(chan common.Event, config.PriorityEventQueueSize),
		stopLoop:       make(chan struct{}, 1),
		loopStopped:    make(chan struct{}),
		onStop:         config.OnStop,
		name:           config.Name,
		processEvent:   processEvent,
	}

	return sel
}

// Start begins the event processing loop. This should be called as a goroutine.
// The priority queue is fully drained on each iteration before taking work from the main queue.
func (sel *StateMachineEventLoop[S, E]) Start(ctx context.Context) {
	defer close(sel.loopStopped)
	sel.running = true

	log.L(ctx).Debugf("%s | %s | event loop started", sel.name, sel.stateMachine.CurrentState.String())

	for {
		// Drain the priority queue fully before taking work from the main queue
	drainPriority:
		for {
			select {
			case event := <-sel.eventsPriority:
				if syncEv, ok := isSyncEvent(event); ok {
					log.L(ctx).Debugf("%s | %s | sync event processed (priority)", sel.name, sel.stateMachine.CurrentState.String())
					close(syncEv.Done)
					continue
				}
				log.L(ctx).Debugf("%s | %s | %s | processing priority", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString())
				err := sel.processEvent(ctx, event)
				if err != nil {
					log.L(ctx).Errorf("%s | %s | %s | error: %v", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString(), err)
				}
			default:
				break drainPriority
			}
		}

		select {
		case event := <-sel.eventsPriority:
			if syncEv, ok := isSyncEvent(event); ok {
				log.L(ctx).Debugf("%s | %s | sync event processed (priority)", sel.name, sel.stateMachine.CurrentState.String())
				close(syncEv.Done)
				continue
			}
			log.L(ctx).Debugf("%s | %s | %s | processing priority", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString())
			err := sel.processEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("%s | %s | %s | error: %v", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString(), err)
			}
		case event := <-sel.events:
			if syncEv, ok := isSyncEvent(event); ok {
				log.L(ctx).Debugf("%s | %s | sync event processed", sel.name, sel.stateMachine.CurrentState.String())
				close(syncEv.Done)
				continue
			}

			log.L(ctx).Debugf("%s | %s | %s | processing", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString())
			err := sel.processEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("%s | %s | %s | error: %v", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString(), err)
			}
		case <-sel.stopLoop:
			if sel.onStop != nil {
				if finalEvent := sel.onStop(ctx); finalEvent != nil {
					log.L(ctx).Debugf("%s | %s | %s | processing final", sel.name, sel.stateMachine.CurrentState.String(), finalEvent.TypeString())
					err := sel.processEvent(ctx, finalEvent)
					if err != nil {
						log.L(ctx).Errorf("%s | %s | %s | error: %v", sel.name, sel.stateMachine.CurrentState.String(), finalEvent.TypeString(), err)
					}
				}
			}
			log.L(ctx).Debugf("%s | %s | event loop stopped", sel.name, sel.stateMachine.CurrentState.String())
			sel.running = false
			return
		case <-ctx.Done():
			log.L(ctx).Debugf("%s | %s | context cancelled, stopping", sel.name, sel.stateMachine.CurrentState.String())
			sel.running = false
			return
		}
	}
}

// QueueEvent asynchronously queues an event for processing.
func (sel *StateMachineEventLoop[S, E]) QueueEvent(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("%s | %s | queueing event %s", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString())
	sel.events <- event
}

// TryQueueEvent attempts to queue an event without blocking.
// Returns true if the event was queued, false if the buffer is full.
// This function should only be used if it is acceptable for the event to never be processed, e.g. because it is a periodic event,
func (sel *StateMachineEventLoop[S, E]) TryQueueEvent(ctx context.Context, event common.Event) bool {
	select {
	case sel.events <- event:
		log.L(ctx).Tracef("%s | %s | queued event %s", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString())
		return true
	default:
		log.L(ctx).Warnf("%s | %s | buffer full, dropping event %s", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString())
		return false
	}
}

// QueuePriorityEvent asynchronously queues an event on the priority queue.
// Priority events are always fully drained before the main queue is read.
func (sel *StateMachineEventLoop[S, E]) QueuePriorityEvent(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("%s | %s | queueing priority event %s", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString())
	sel.eventsPriority <- event
}

// TryQueuePriorityEvent attempts to queue an event on the priority queue without blocking.
// Returns true if the event was queued, false if the priority buffer is full.
func (sel *StateMachineEventLoop[S, E]) TryQueuePriorityEvent(ctx context.Context, event common.Event) bool {
	select {
	case sel.eventsPriority <- event:
		log.L(ctx).Tracef("%s | %s | queued priority event %s", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString())
		return true
	default:
		log.L(ctx).Warnf("%s | %s | priority buffer full, dropping event %s", sel.name, sel.stateMachine.CurrentState.String(), event.TypeString())
		return false
	}
}

// ProcessEvent synchronously processes an event. This bypasses the event loop
// and should only be used in tests or when you need synchronous processing.
func (sel *StateMachineEventLoop[S, E]) ProcessEvent(ctx context.Context, event common.Event) error {
	return sel.stateMachine.ProcessEvent(ctx, sel.entity, event)
}

// Stop signals the event loop to stop and waits for it to complete.
func (sel *StateMachineEventLoop[S, E]) Stop() {
	select {
	case <-sel.loopStopped:
		return
	default:
	}

	select {
	case sel.stopLoop <- struct{}{}:
	default:
	}

	<-sel.loopStopped
}

// StopAsync signals the event loop to stop but does not wait for completion.
func (sel *StateMachineEventLoop[S, E]) StopAsync() {
	select {
	case <-sel.loopStopped:
		return
	default:
	}

	select {
	case sel.stopLoop <- struct{}{}:
	default:
	}
}

// WaitForStop waits for the event loop to complete after Stop or StopAsync was called.
func (sel *StateMachineEventLoop[S, E]) WaitForStop() {
	<-sel.loopStopped
}

// IsStopped returns true if the event loop has been stopped.
func (sel *StateMachineEventLoop[S, E]) IsStopped() bool {
	select {
	case <-sel.loopStopped:
		return true
	default:
		return false
	}
}

// IsRunning returns true if the event loop is currently running.
func (sel *StateMachineEventLoop[S, E]) IsRunning() bool {
	return sel.running && !sel.IsStopped()
}

// StateMachine returns the underlying state machine for direct access.
func (sel *StateMachineEventLoop[S, E]) StateMachine() *StateMachine[S, E] {
	return sel.stateMachine
}

// GetCurrentState returns the current state of the state machine.
func (sel *StateMachineEventLoop[S, E]) GetCurrentState() S {
	return sel.stateMachine.GetCurrentState()
}
