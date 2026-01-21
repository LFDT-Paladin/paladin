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
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

// State is a constraint for state types - must be an integer type with a String method
type State interface {
	~int
	String() string
}

// EventType is the type used for event identifiers
type EventType = common.EventType

// StateReader is the interface that state reader types must implement
type StateReader[S State] interface {
	RLock()
	RUnlock()
	GetCurrentState() S
	GetLastStateChange() time.Time
	GetLatestEvent() string
}

// StateMutator is the interface that mutable state types must implement
type StateMutator[S State] interface {
	Lock()
	Unlock()
	SetCurrentState(state S)
	SetLastStateChange(time.Time)
	SetLatestEvent(string)
}

// Subject is the interface that state machine subjects must implement
// It provides access to the mutable state, read-only view, config, and callbacks
// Type parameters:
//   - S: state enum type
//   - M: state mutator type (modified by StateUpdates)
//   - R: state reader type (read-only view, used by Guards/Actions)
//   - Cfg: config type (immutable configuration)
//   - Cb: callbacks type (side effect callbacks)
type Subject[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any] interface {
	// GetStateMutator returns the mutable state that StateUpdates can modify
	GetStateMutator() M
	// GetStateReader returns the read-only view of the state for Guards/Actions
	GetStateReader() R
	// GetConfig returns the immutable configuration
	GetConfig() Cfg
	// GetCallbacks returns the callbacks for side effects
	GetCallbacks() Cb
}

// StateUpdate modifies the subject's mutable state based on event data
// This is the ONLY place where state modification should occur
// Parameters:
//   - S: state enum type
//   - M: state mutator type (modified by StateUpdates)
//   - Cfg: config type (immutable configuration)
//   - Cb: callbacks type (side effect callbacks)
type StateUpdate[S State, M StateMutator[S], Cfg any, Cb any] func(ctx context.Context, mutator M, config Cfg, callbacks Cb, event common.Event) error

// Action performs side effects (e.g., sending messages, starting goroutines)
// Actions should NOT modify state - use StateUpdate for state changes
// Parameters:
//   - S: state enum type
//   - R: the state reader interface (read-only view of state)
//   - Cfg: config type (immutable configuration)
//   - Cb: callbacks type (side effect callbacks)
type Action[S State, R StateReader[S], Cfg any, Cb any] func(ctx context.Context, reader R, config Cfg, callbacks Cb, event common.Event) error

// Guard is a predicate function that determines whether a transition should be taken
// Guards should NOT modify state - they only evaluate conditions based on state
// For event-based conditions, use EventValidator instead
// Parameters:
//   - S: state enum type
//   - R: the state reader interface (read-only view of state)
//   - Cfg: config type (immutable configuration)
type Guard[S State, R StateReader[S], Cfg any] func(ctx context.Context, reader R, config Cfg) bool

// Validator validates whether an event is applicable given the current state
// Returns true if the event should be processed, false if it should be ignored
// Parameters:
//   - S: state enum type
//   - R: the state reader interface (read-only view of state)
//   - Cfg: config type (immutable configuration)
//   - Cb: callbacks type (side effect callbacks) // TODO AM: remove this
type Validator[S State, R StateReader[S], Cfg any, Cb any] func(ctx context.Context, reader R, config Cfg, callbacks Cb, event common.Event) (bool, error)

// Transition defines a possible state transition
// S = state enum, M = mutator interface, R = reader interface, Cfg = config, Cb = callbacks
type Transition[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any] struct {
	To           S                            // Target state to transition to if guard passes
	If           Guard[S, R, Cfg]             // Optional guard condition - if nil, transition is always taken
	StateUpdates []StateUpdate[S, M, Cfg, Cb] // State updates to apply when this transition is taken (before On action)
	On           Action[S, R, Cfg, Cb]        // Optional action to execute during this transition (before OnTransitionTo)
}

// ActionRule pairs an action with optional guard and event validator conditions
// Evaluation order: EventValidator (if present) -> If guard (if present) -> Action
type ActionRule[S State, R StateReader[S], Cfg any, Cb any] struct {
	Action         Action[S, R, Cfg, Cb]    // The action to execute
	If             Guard[S, R, Cfg]         // Optional guard based on state - if nil, guard passes
	EventValidator Validator[S, R, Cfg, Cb] // Optional validator based on event - if nil, validation passes
}

// StateUpdateRule pairs a state update with optional guard and event validator conditions
// Evaluation order: EventValidator (if present) -> If guard (if present) -> StateUpdate
type StateUpdateRule[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any] struct {
	StateUpdate    StateUpdate[S, M, Cfg, Cb] // The state update to execute
	If             Guard[S, R, Cfg]           // Optional guard based on state - if nil, guard passes
	EventValidator Validator[S, R, Cfg, Cb]   // Optional validator based on event - if nil, validation passes
}

// EventHandler defines how a specific event type is handled in a particular state
type EventHandler[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any] struct {
	// Validator optionally validates whether this event instance is applicable
	// This validates the entire event handler - if false, nothing in this handler runs
	Validator Validator[S, R, Cfg, Cb]

	// StateUpdates apply event-specific state updates (executed in order)
	// Called after validation but before actions and transitions
	// Each StateUpdateRule can have its own EventValidator and If guard
	StateUpdates []StateUpdateRule[S, M, R, Cfg, Cb]

	// Actions are executed (in order) before transitions are evaluated
	// Each ActionRule can have its own EventValidator and If guard
	Actions []ActionRule[S, R, Cfg, Cb]

	// Transitions define possible state changes this event can trigger
	Transitions []Transition[S, M, R, Cfg, Cb]
}

// TransitionToHandler defines what happens when entering a state
type TransitionToHandler[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any] struct {
	// StateUpdates apply state updates when entering this state (before actions)
	// Each StateUpdateRule can have its own EventValidator and If guard
	StateUpdates []StateUpdateRule[S, M, R, Cfg, Cb]

	// Actions are executed (in order) after state updates
	// Each ActionRule can have its own EventValidator and If guard
	Actions []ActionRule[S, R, Cfg, Cb]
}

// StateDefinition defines the behavior for a particular state
type StateDefinition[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any] struct {
	// OnTransitionTo is executed when entering this state
	OnTransitionTo TransitionToHandler[S, M, R, Cfg, Cb]

	// Events maps event types to their handlers for this state
	Events map[EventType]EventHandler[S, M, R, Cfg, Cb]
}

// TransitionCallback is called after a successful state transition
type TransitionCallback[S State, R StateReader[S], Cfg any, Cb any] func(ctx context.Context, reader R, config Cfg, callbacks Cb, from S, to S, event common.Event)

// StateMachineConfig holds the configuration for a state machine instance
// Type parameters:
//   - S: state enum type
//   - M: state mutator type (modified by StateUpdates)
//   - R: state reader type (read-only view, used by Guards/Actions)
//   - Cfg: config type (immutable configuration)
//   - Cb: callbacks type (side effect callbacks)
type StateMachineConfig[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any] struct {
	// Definitions maps states to their definitions
	Definitions map[S]StateDefinition[S, M, R, Cfg, Cb]

	// OnTransition is called after every successful state transition (optional)
	OnTransition TransitionCallback[S, R, Cfg, Cb]
}

// StateMachineState holds the runtime state of a state machine
type StateMachineState[S State] struct {
	currentState    S
	lastStateChange time.Time
	latestEvent     string
	mu              sync.RWMutex
}

func (s *StateMachineState[S]) Lock() {
	s.mu.Lock()
}

func (s *StateMachineState[S]) Unlock() {
	s.mu.Unlock()
}

func (s *StateMachineState[S]) RLock() {
	s.mu.RLock()
}

func (s *StateMachineState[S]) RUnlock() {
	s.mu.RUnlock()
}

func (s *StateMachineState[S]) GetCurrentState() S {
	return s.currentState
}

func (s *StateMachineState[S]) GetLastStateChange() time.Time {
	return s.lastStateChange
}

func (s *StateMachineState[S]) GetLatestEvent() string {
	return s.latestEvent
}

func (s *StateMachineState[S]) SetCurrentState(state S) {
	s.currentState = state
}

func (s *StateMachineState[S]) SetLastStateChange(lastStateChange time.Time) {
	s.lastStateChange = lastStateChange
}

func (s *StateMachineState[S]) SetLatestEvent(event string) {
	s.latestEvent = event
}
