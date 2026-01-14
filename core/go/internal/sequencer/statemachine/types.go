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

// Action is a function that performs an action on the subject when a transition occurs or when entering a state
// The subject T is the entity that owns/embeds this state machine (e.g., coordinator, transaction)
type Action[T any] func(ctx context.Context, subject T) error

// Guard is a predicate function that determines whether a transition should be taken
// Guards evaluate conditions on the subject to decide if the transition is allowed
type Guard[T any] func(ctx context.Context, subject T) bool

// Validator is a function that validates whether an event is applicable given the current state
// Returns true if the event should be processed, false if it should be ignored
// An error return indicates an unexpected validation failure (not just invalid event)
type Validator[T any] func(ctx context.Context, subject T, event common.Event) (bool, error)

// Transition defines a possible state transition
type Transition[S State, T any] struct {
	To S         // Target state to transition to if guard passes
	If Guard[T]  // Optional guard condition - if nil, transition is always taken
	On Action[T] // Optional action to execute during this specific transition (before OnTransitionTo)
}

// ActionRule pairs an action with an optional guard condition
// Used for actions that should be executed when an event is received (before transitions are evaluated)
type ActionRule[T any] struct {
	Action Action[T] // The action to execute
	If     Guard[T]  // Optional guard - if nil, action always executes
}

// StateUpdate is a function that updates the subject's internal state based on event data
// This is called after event validation but before actions and transitions are processed
type StateUpdate[T any] func(ctx context.Context, subject T, event common.Event) error

// EventHandler defines how a specific event type is handled in a particular state
type EventHandler[S State, T any] struct {
	// Validator optionally validates whether this event instance is applicable
	// If nil, the event is always considered valid when received in the appropriate state
	Validator Validator[T]

	// OnHandleEvent optionally applies event-specific state updates to the subject
	// This is called after validation but before actions and transitions are evaluated,
	// allowing guards to reference the updated state
	// If nil, no state update is performed for this event handler
	OnHandleEvent StateUpdate[T]

	// Actions are executed (in order) when this event is received, before transitions are evaluated
	// Each action may have its own guard condition
	Actions []ActionRule[T]

	// Transitions define the possible state changes this event can trigger
	// Transitions are evaluated in order - the first matching transition (guard passes) is taken
	Transitions []Transition[S, T]
}

// StateDefinition defines the behavior for a particular state
type StateDefinition[S State, T any] struct {
	// OnTransitionTo is executed when entering this state (after any transition-specific On action)
	OnTransitionTo Action[T]

	// Events maps event types to their handlers for this state
	// Events not in this map are ignored when in this state
	Events map[EventType]EventHandler[S, T]
}

// TransitionCallback is called after a successful state transition
// Useful for logging, metrics, or notifying other components
type TransitionCallback[S State, T any] func(ctx context.Context, subject T, from S, to S, event common.Event)

// StateMachineConfig holds the configuration for a state machine instance
type StateMachineConfig[S State, T any] struct {
	// Definitions maps states to their definitions
	Definitions map[S]StateDefinition[S, T]

	// OnTransition is called after every successful state transition (optional)
	OnTransition TransitionCallback[S, T]
}

// StateMachineState holds the runtime state of a state machine
// This is separated from config to allow the config to be shared across instances
type StateMachineState[S State] struct {
	CurrentState    S
	LastStateChange time.Time
	LatestEvent     string
}
