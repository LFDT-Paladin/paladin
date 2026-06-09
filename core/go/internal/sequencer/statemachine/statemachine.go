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
//	        Events: map[common.EventType]statemachine.EventHandlers[MyState, *MyEntity]{
//	            Event_Activate: {Handlers: []statemachine.EventHandler[MyState, *MyEntity]{{
//	                Actions: []statemachine.ActionRule[*MyEntity]{{
//	                    Action: func(ctx context.Context, e *MyEntity, event common.Event) error {
//	                        e.counter++
//	                        return nil
//	                    },
//	                }},
//	                Transitions: []statemachine.Transition[MyState, *MyEntity]{{
//	                    To: State_Active,
//	                }},
//	            }}},
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
	"sync"
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
//   - Specific transitions (Actions field in Transition struct)
//   - Entry to a state (OnTransitionTo in StateDefinition)
type Action[E any] func(ctx context.Context, entity E, event common.Event) error

// Guard is a condition function that determines if a transition should be taken
// or if an action should be executed.
type Guard[E any] func(ctx context.Context, entity E) bool

// MatchMode controls how handlers in an EventHandlers set are evaluated.
type MatchMode int

const (
	// MatchFirst (zero value, default): the first handler whose Validator passes is
	// executed exclusively. Subsequent handlers are not evaluated.
	// A nil Validator always passes; if it is not the last handler, a startup warning
	// is logged because subsequent handlers are unreachable.
	MatchFirst MatchMode = iota
	// MatchAll: every handler whose Validator passes is executed in order.
	// Execution stops early if any handler causes a state transition.
	MatchAll
)

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
// Actions: Optional transition-specific action rules to execute before state-entry actions
type Transition[S State, E any] struct {
	To      S               // Target state
	If      Guard[E]        // Guard condition (optional)
	Actions []ActionRule[E] // Transition-specific action rules (optional)
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

// EventHandlers groups the handlers for a single event type with their dispatch mode.
// The zero value (Match=MatchFirst, empty Handlers) is valid but handles nothing.
type EventHandlers[S State, E any] struct {
	Match    MatchMode
	Handlers []EventHandler[S, E]
}

// StateDefinition defines the behavior of a particular state.
// OnTransitionTo: Action rules executed when entering this state (after transition-specific actions)
// OnTransitionFrom: Action rules executed when leaving this state (after transition-specific actions, before entry actions)
// Events: Map of event types to their handler sets in this state
type StateDefinition[S State, E any] struct {
	OnTransitionTo   []ActionRule[E]
	OnTransitionFrom []ActionRule[E]
	Events           map[common.EventType]EventHandlers[S, E]
}

// StateDefinitions is a map from states to their definitions.
type StateDefinitions[S State, E any] map[S]StateDefinition[S, E]

// TransitionCallback is called when a state transition occurs.
// It receives the entity, old state, new state, and the event that triggered the transition.
type TransitionCallback[S State, E any] func(ctx context.Context, entity E, from S, to S, event common.Event)

// StateMachine holds the current state, metadata, and processing logic for a state machine instance.
// The entity type E must implement Lockable; the state machine holds the entity's lock
// for the duration of each ProcessEvent call.
type StateMachine[S State, E Lockable] struct {
	stateMu            sync.RWMutex
	currentState       S
	lastStateChange    time.Time
	latestEvent        string
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
		currentState:    initialState,
		lastStateChange: time.Now(),
		definitions:     definitions,
		name:            name,
	}
	for _, opt := range opts {
		opt(sm)
	}
	validateDefinitions(definitions, name)
	return sm
}

// validateDefinitions warns when a MatchFirst handler set has a nil-Validator handler
// that is not the last entry, because all subsequent handlers would be unreachable.
func validateDefinitions[S State, E Lockable](definitions StateDefinitions[S, E], name string) {
	for _, stateDef := range definitions {
		for _, handlerSet := range stateDef.Events {
			if handlerSet.Match == MatchAll {
				continue
			}
			for i, h := range handlerSet.Handlers {
				if h.Validator == nil && i < len(handlerSet.Handlers)-1 {
					log.L(context.Background()).Warnf(
						"%s: MatchFirst EventHandlers has a nil-Validator handler at index %d that is not the last — subsequent handlers are unreachable",
						name, i)
				}
			}
		}
	}
}

// ProcessEvent handles an event for the given entity.
// Returns nil if the event was processed successfully or was not applicable.
// Returns an error if action or transition execution fails.
//
// Dispatch depends on EventHandlers.Match:
//   - MatchFirst (default): the first handler whose Validator passes is executed exclusively.
//   - MatchAll: every handler whose Validator passes is executed; stops on first transition.
func (sm *StateMachine[S, E]) ProcessEvent(
	ctx context.Context,
	entity E,
	event common.Event,
) error {
	entity.Lock()
	defer entity.Unlock()

	currentState := sm.GetCurrentState()
	log.L(ctx).Debugf("%s | %s | processing event %s", sm.name, currentState.String(), event.TypeString())

	stateDefinition, exists := sm.definitions[currentState]
	if !exists {
		return nil
	}
	handlerSet, defined := stateDefinition.Events[event.Type()]
	if !defined {
		return nil
	}

	if handlerSet.Match == MatchAll {
		return sm.processMatchAll(ctx, entity, event, handlerSet.Handlers)
	}
	return sm.processMatchFirst(ctx, entity, event, handlerSet.Handlers)
}

// processMatchFirst executes the first handler whose Validator passes (MatchFirst mode).
func (sm *StateMachine[S, E]) processMatchFirst(
	ctx context.Context,
	entity E,
	event common.Event,
	handlers []EventHandler[S, E],
) error {
	for i := range handlers {
		h := &handlers[i]
		matched, err := sm.matchesHandler(ctx, entity, event, h, i)
		if err != nil {
			return err
		}
		if !matched {
			continue
		}
		if err = sm.executeActionRules(ctx, entity, event, h.Actions); err != nil {
			log.L(ctx).Errorf("%s | %s | %s | error applying action: %v", sm.name, sm.GetCurrentState().String(), event.TypeString(), err)
			return err
		}
		_, err = sm.evaluateTransitions(ctx, entity, event, *h)
		return err
	}
	return nil
}

// processMatchAll executes every handler whose Validator passes, stopping on the first transition (MatchAll mode).
func (sm *StateMachine[S, E]) processMatchAll(
	ctx context.Context,
	entity E,
	event common.Event,
	handlers []EventHandler[S, E],
) error {
	for i := range handlers {
		h := &handlers[i]
		matched, err := sm.matchesHandler(ctx, entity, event, h, i)
		if err != nil {
			return err
		}
		if !matched {
			continue
		}
		if err = sm.executeActionRules(ctx, entity, event, h.Actions); err != nil {
			log.L(ctx).Errorf("%s | %s | %s | error applying action: %v", sm.name, sm.GetCurrentState().String(), event.TypeString(), err)
			return err
		}
		transitioned, err := sm.evaluateTransitions(ctx, entity, event, *h)
		if err != nil || transitioned {
			return err
		}
	}
	return nil
}

// matchesHandler reports whether a handler's Validator passes for the given event.
// A nil Validator always passes.
func (sm *StateMachine[S, E]) matchesHandler(
	ctx context.Context,
	entity E,
	event common.Event,
	h *EventHandler[S, E],
	index int,
) (bool, error) {
	if h.Validator == nil {
		return true, nil
	}
	valid, err := h.Validator(ctx, entity, event)
	if err != nil {
		log.L(ctx).Errorf("%s | %s | error evaluating handler[%d] validator for event %s: %v",
			sm.name, sm.GetCurrentState().String(), index, event.TypeString(), err)
		return false, err
	}
	return valid, nil
}

// executeActionRules executes guarded action rules in order.
func (sm *StateMachine[S, E]) executeActionRules(
	ctx context.Context,
	entity E,
	event common.Event,
	actionRules []ActionRule[E],
) error {
	for _, rule := range actionRules {
		if rule.If == nil || rule.If(ctx, entity) {
			if err := rule.Action(ctx, entity, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// evaluateTransitions evaluates transition rules and performs the first matching transition.
// Returns (true, nil) when a transition was taken, (false, nil) when none matched.
func (sm *StateMachine[S, E]) evaluateTransitions(
	ctx context.Context,
	entity E,
	event common.Event,
	eventHandler EventHandler[S, E],
) (bool, error) {
	for _, rule := range eventHandler.Transitions {
		if rule.If != nil && !rule.If(ctx, entity) {
			continue
		}

		previousState := sm.GetCurrentState()
		sm.SetCurrentState(rule.To)
		sm.latestEvent = event.TypeString()
		sm.lastStateChange = time.Now()

		if err := sm.executeActionRules(ctx, entity, event, rule.Actions); err != nil {
			log.L(ctx).Errorf("%s | %s | %s | error executing transition to state %s : %v", sm.name, previousState.String(), event.TypeString(), sm.GetCurrentState().String(), err)
			return true, err
		}

		previousStateDefinition, exists := sm.definitions[previousState]
		if exists && len(previousStateDefinition.OnTransitionFrom) > 0 {
			if err := sm.executeActionRules(ctx, entity, event, previousStateDefinition.OnTransitionFrom); err != nil {
				log.L(ctx).Errorf("%s | %s | %s | error executing state exit action for transition from state %s : %v", sm.name, previousState.String(), event.TypeString(), previousState.String(), err)
				return true, err
			}
		}

		newState := sm.GetCurrentState()

		log.L(ctx).Debugf("%s | %s | %s | transition to state %s",
			sm.name, previousState.String(), event.TypeString(), newState.String())

		if sm.transitionCallback != nil {
			sm.transitionCallback(ctx, entity, previousState, newState, event)
		}

		newStateDefinition, exists := sm.definitions[newState]
		if exists && len(newStateDefinition.OnTransitionTo) > 0 {
			if err := sm.executeActionRules(ctx, entity, event, newStateDefinition.OnTransitionTo); err != nil {
				log.L(ctx).Errorf("%s | %s | %s | error executing state entry action for transition to state %s : %v", sm.name, previousState.String(), event.TypeString(), newState.String(), err)
				return true, err
			}
		}

		return true, nil
	}
	return false, nil
}

// GetCurrentState returns the current state of the state machine.
func (sm *StateMachine[S, E]) GetCurrentState() S {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()
	return sm.currentState
}

// SetCurrentState sets the state machine state.
// Intended for test setup and controlled internal transitions.
func (sm *StateMachine[S, E]) SetCurrentState(state S) {
	sm.stateMu.Lock()
	sm.currentState = state
	sm.stateMu.Unlock()
}

// GetLastStateChange returns the time of the last state change.
func (sm *StateMachine[S, E]) GetLastStateChange() time.Time {
	return sm.lastStateChange
}

// GetLatestEvent returns the type string of the last event that caused a transition.
func (sm *StateMachine[S, E]) GetLatestEvent() string {
	return sm.latestEvent
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
	loopStopped    chan struct{}
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
		loopStopped:    make(chan struct{}),
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

	log.L(ctx).Debugf("%s | %s | event loop started", sel.name, sel.stateMachine.GetCurrentState().String())

	for {
		// Drain the priority queue fully before taking work from the main queue
	drainPriority:
		for {
			select {
			case event := <-sel.eventsPriority:
				if syncEv, ok := isSyncEvent(event); ok {
					log.L(ctx).Debugf("%s | %s | sync event processed (priority)", sel.name, sel.stateMachine.GetCurrentState().String())
					close(syncEv.Done)
					continue
				}
				log.L(ctx).Debugf("%s | %s | %s | processing priority", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
				err := sel.processEvent(ctx, event)
				if err != nil {
					log.L(ctx).Errorf("%s | %s | %s | error: %v", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString(), err)
				}
			default:
				break drainPriority
			}
		}

		select {
		case event := <-sel.eventsPriority:
			if syncEv, ok := isSyncEvent(event); ok {
				log.L(ctx).Debugf("%s | %s | sync event processed (priority)", sel.name, sel.stateMachine.GetCurrentState().String())
				close(syncEv.Done)
				continue
			}
			log.L(ctx).Debugf("%s | %s | %s | processing priority", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
			err := sel.processEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("%s | %s | %s | error: %v", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString(), err)
			}
		case event := <-sel.events:
			if syncEv, ok := isSyncEvent(event); ok {
				log.L(ctx).Debugf("%s | %s | sync event processed", sel.name, sel.stateMachine.GetCurrentState().String())
				close(syncEv.Done)
				continue
			}

			log.L(ctx).Debugf("%s | %s | %s | processing", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
			err := sel.processEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("%s | %s | %s | error: %v", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString(), err)
			}
		case <-ctx.Done():
			log.L(ctx).Debugf("%s | %s | context cancelled, stopping", sel.name, sel.stateMachine.GetCurrentState().String())
			sel.running = false
			return
		}
	}
}

// QueueEvent asynchronously queues an event for processing.
func (sel *StateMachineEventLoop[S, E]) QueueEvent(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("%s | %s | queueing event %s", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
	select {
	case sel.events <- event:
	case <-ctx.Done():
		log.L(ctx).Warnf("%s | %s | context cancelled, dropping event %s", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
	}
}

// TryQueueEvent attempts to queue an event without blocking.
// Returns true if the event was queued, false if the buffer is full.
// This function should only be used if it is acceptable for the event to never be processed, e.g. because it is a periodic event,
func (sel *StateMachineEventLoop[S, E]) TryQueueEvent(ctx context.Context, event common.Event) bool {
	select {
	case sel.events <- event:
		log.L(ctx).Tracef("%s | %s | queued event %s", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
		return true
	default:
		log.L(ctx).Warnf("%s | %s | buffer full, dropping event %s", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
		return false
	}
}

// QueuePriorityEvent asynchronously queues an event on the priority queue.
// Priority events are always fully drained before the main queue is read.
func (sel *StateMachineEventLoop[S, E]) QueuePriorityEvent(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("%s | %s | queueing priority event %s", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
	select {
	case sel.eventsPriority <- event:
	case <-ctx.Done():
		log.L(ctx).Warnf("%s | %s | context cancelled, dropping priority event %s", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
	}
}

// TryQueuePriorityEvent attempts to queue an event on the priority queue without blocking.
// Returns true if the event was queued, false if the priority buffer is full.
func (sel *StateMachineEventLoop[S, E]) TryQueuePriorityEvent(ctx context.Context, event common.Event) bool {
	select {
	case sel.eventsPriority <- event:
		log.L(ctx).Tracef("%s | %s | queued priority event %s", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
		return true
	default:
		log.L(ctx).Warnf("%s | %s | priority buffer full, dropping event %s", sel.name, sel.stateMachine.GetCurrentState().String(), event.TypeString())
		return false
	}
}

// ProcessEvent synchronously processes an event through the same pre-processing pipeline
// used by the event loop goroutine. Use this in tests or other cases that need deterministic,
// synchronous event processing without starting the goroutine-based event loop.
func (sel *StateMachineEventLoop[S, E]) ProcessEvent(ctx context.Context, event common.Event) error {
	return sel.processEvent(ctx, event)
}

// DrainPendingEvents processes all events currently buffered in the priority and regular queues,
// using the same pipeline as the event loop. It stops when both queues are empty.
// Useful in tests to flush internally-queued follow-up events before making assertions.
func (sel *StateMachineEventLoop[S, E]) DrainPendingEvents(ctx context.Context) error {
	for {
		processed := false
		// Drain all priority events before touching the regular queue.
		for draining := true; draining; {
			select {
			case event := <-sel.eventsPriority:
				if err := sel.processEvent(ctx, event); err != nil {
					return err
				}
				processed = true
			default:
				draining = false
			}
		}
		// Process one regular event (priority events queued by it will be picked up next iteration).
		select {
		case event := <-sel.events:
			if err := sel.processEvent(ctx, event); err != nil {
				return err
			}
			processed = true
		default:
		}
		if !processed {
			return nil
		}
	}
}

// WaitForDone waits for the event loop to complete after context cancellation.
func (sel *StateMachineEventLoop[S, E]) WaitForDone(ctx context.Context) {
	select {
	case <-sel.loopStopped:
	case <-ctx.Done():
	}
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
