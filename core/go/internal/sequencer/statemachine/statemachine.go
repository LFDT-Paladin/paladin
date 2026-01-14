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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

type StateMachine[S State, T any] struct {
	config StateMachineConfig[S, T]
	state  StateMachineState[S]
}

// NewStateMachine creates a new state machine with the given configuration and initial state
func NewStateMachine[S State, T any](config StateMachineConfig[S, T], initialState S) *StateMachine[S, T] {
	return &StateMachine[S, T]{
		config: config,
		state: StateMachineState[S]{
			CurrentState:    initialState,
			LastStateChange: time.Now(),
		},
	}
}

// GetCurrentState returns the current state
func (sm *StateMachine[S, T]) GetCurrentState() S {
	return sm.state.CurrentState
}

// GetLastStateChange returns the time of the last state change
func (sm *StateMachine[S, T]) GetLastStateChange() time.Time {
	return sm.state.LastStateChange
}

// GetLatestEvent returns the type string of the last processed event that caused a transition
func (sm *StateMachine[S, T]) GetLatestEvent() string {
	return sm.state.LatestEvent
}

// SetState directly sets the current state (use with caution - bypasses normal transition logic)
// This is primarily useful for initialization or testing
func (sm *StateMachine[S, T]) SetState(state S) {
	sm.state.CurrentState = state
	sm.state.LastStateChange = time.Now()
}

// ProcessEvent processes an event through the state machine
// Parameters:
//   - ctx: context for logging and cancellation
//   - subject: the entity that owns this state machine
//   - event: the event to process
//
// Returns an error if event processing fails (validation errors, action errors, or transition errors)
func (sm *StateMachine[S, T]) ProcessEvent(
	ctx context.Context,
	subject T,
	event common.Event,
) error {
	// Step 1: Evaluate whether this event is relevant to the current state
	eventHandler, err := sm.evaluateEvent(ctx, subject, event)
	if err != nil || eventHandler == nil {
		return err
	}

	// Step 2: Apply the event to update the subject's internal state
	// This happens before guards are evaluated so guards can reference the updated state
	if eventHandler.OnHandleEvent != nil {
		if err := eventHandler.OnHandleEvent(ctx, subject, event); err != nil {
			log.L(ctx).Errorf("error in OnHandleEvent for %s: %v", event.TypeString(), err)
			return err
		}
	}

	// Step 3: Execute any actions defined for this event
	if err := sm.performActions(ctx, subject, *eventHandler); err != nil {
		return err
	}

	// Step 4: Evaluate and execute transitions
	if err := sm.evaluateTransitions(ctx, subject, event, *eventHandler); err != nil {
		return err
	}

	return nil
}

// evaluateEvent checks if the given event is relevant for the current state
// Returns the event handler if the event should be processed, nil if it should be ignored
func (sm *StateMachine[S, T]) evaluateEvent(ctx context.Context, subject T, event common.Event) (*EventHandler[S, T], error) {
	currentState := sm.state.CurrentState

	// Get the state definition for the current state
	stateDefinition, exists := sm.config.Definitions[currentState]
	if !exists {
		log.L(ctx).Tracef("no definition for current state %s, ignoring event %s", currentState.String(), event.TypeString())
		return nil, nil
	}

	// Check if this event type is handled in the current state
	eventHandler, isHandlerDefined := stateDefinition.Events[event.Type()]
	if !isHandlerDefined {
		log.L(ctx).Tracef("event %s not handled in state %s, ignoring", event.TypeString(), currentState.String())
		return nil, nil
	}

	// If there's a validator, check if this specific event instance is valid
	if eventHandler.Validator != nil {
		valid, err := eventHandler.Validator(ctx, subject, event)
		if err != nil {
			log.L(ctx).Errorf("error validating event %s: %v", event.TypeString(), err)
			return nil, err
		}
		if !valid {
			log.L(ctx).Warnf("event %s is not valid for current state %s", event.TypeString(), currentState.String())
			return nil, nil
		}
	}

	return &eventHandler, nil
}

// performActions executes all applicable actions for the event
func (sm *StateMachine[S, T]) performActions(ctx context.Context, subject T, eventHandler EventHandler[S, T]) error {
	for _, rule := range eventHandler.Actions {
		// Check guard condition if present
		if rule.If != nil && !rule.If(ctx, subject) {
			continue
		}

		// Execute the action
		if err := rule.Action(ctx, subject); err != nil {
			log.L(ctx).Errorf("error executing action: %v", err)
			return err
		}
	}
	return nil
}

// evaluateTransitions checks each transition rule and executes the first matching one
func (sm *StateMachine[S, T]) evaluateTransitions(
	ctx context.Context,
	subject T,
	event common.Event,
	eventHandler EventHandler[S, T],
) error {
	for _, transition := range eventHandler.Transitions {
		// Check guard condition if present
		if transition.If != nil && !transition.If(ctx, subject) {
			continue
		}

		// Guard passed (or no guard) - execute this transition
		previousState := sm.state.CurrentState
		newState := transition.To

		log.L(ctx).Debugf("state transition: %s -> %s (event: %s)",
			previousState.String(), newState.String(), event.TypeString())

		// Execute transition-specific action first (if defined)
		if transition.On != nil {
			if err := transition.On(ctx, subject); err != nil {
				log.L(ctx).Errorf("error executing transition action: %v", err)
				return err
			}
		}

		// Update state
		sm.state.CurrentState = newState
		sm.state.LastStateChange = time.Now()
		sm.state.LatestEvent = event.TypeString()

		// Execute OnTransitionTo action for the new state (if defined)
		if newStateDefinition, exists := sm.config.Definitions[newState]; exists {
			if newStateDefinition.OnTransitionTo != nil {
				if err := newStateDefinition.OnTransitionTo(ctx, subject); err != nil {
					log.L(ctx).Errorf("error executing OnTransitionTo for state %s: %v", newState.String(), err)
					return err
				}
			}
		}

		// Call the transition callback if configured
		if sm.config.OnTransition != nil {
			sm.config.OnTransition(ctx, subject, previousState, newState, event)
		}

		// Only the first matching transition is executed
		break
	}

	return nil
}

// TimeSinceStateChange returns the duration since the last state change
func (sm *StateMachine[S, T]) TimeSinceStateChange() time.Duration {
	return time.Since(sm.state.LastStateChange)
}

// IsInState returns true if the state machine is currently in the given state
func (sm *StateMachine[S, T]) IsInState(state S) bool {
	return sm.state.CurrentState == state
}

// IsInAnyState returns true if the state machine is in any of the given states
func (sm *StateMachine[S, T]) IsInAnyState(states ...S) bool {
	for _, state := range states {
		if sm.state.CurrentState == state {
			return true
		}
	}
	return false
}
