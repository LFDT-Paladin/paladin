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

// TODO AM: review locking

// StateMachine manages state transitions for a subject
// Type parameters:
//   - S: state enum type
//   - M: state mutator type (modified by StateUpdates)
//   - R: state reader type (read-only view)
//   - Cfg: config type (immutable configuration)
//   - Cb: callbacks type (side effect callbacks)
type StateMachine[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any] struct {
	subject  Subject[S, M, R, Cfg, Cb]
	smConfig StateMachineConfig[S, M, R, Cfg, Cb]
}

// NewStateMachine creates a new state machine with the given configuration and initial state
func NewStateMachine[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any](subject Subject[S, M, R, Cfg, Cb], smConfig StateMachineConfig[S, M, R, Cfg, Cb], initialState S) *StateMachine[S, M, R, Cfg, Cb] {
	sm := &StateMachine[S, M, R, Cfg, Cb]{
		subject:  subject,
		smConfig: smConfig,
	}
	sm.subject.GetStateMutator().SetCurrentState(initialState)
	sm.subject.GetStateMutator().SetLastStateChange(time.Now())
	return sm
}

// ProcessEvent processes an event through the state machine
// The subject must implement Subject[S, M, R, Cfg, Cb] to provide access to mutator, reader, config, and callbacks
func (sm *StateMachine[S, M, R, Cfg, Cb]) ProcessEvent(
	ctx context.Context,
	event common.Event,
) error {
	// Get components from subject
	config := sm.subject.GetConfig()
	callbacks := sm.subject.GetCallbacks()
	reader := sm.subject.GetStateReader()

	// Step 1: Evaluate whether this event is relevant to the current state
	eventHandler, err := sm.evaluateEvent(ctx, event)
	if err != nil || eventHandler == nil {
		return err
	}

	// Step 2: Apply state updates to the mutable state (with lock held)
	if len(eventHandler.StateUpdates) > 0 {
		mutableState := sm.subject.GetStateMutator()
		mutableState.Lock()

		for _, rule := range eventHandler.StateUpdates {
			// Check event validator if present
			if rule.EventValidator != nil {
				valid, err := rule.EventValidator(ctx, reader, config, callbacks, event)
				if err != nil {
					mutableState.Unlock()
					log.L(ctx).Errorf("error in StateUpdate EventValidator for %s: %v", event.TypeString(), err)
					return err
				}
				if !valid {
					continue
				}
			}

			// Check guard condition if present
			if rule.If != nil && !rule.If(ctx, reader, config) {
				continue
			}

			// Execute the state update
			if err := rule.StateUpdate(ctx, mutableState, config, callbacks, event); err != nil {
				mutableState.Unlock()
				log.L(ctx).Errorf("error in StateUpdate for %s: %v", event.TypeString(), err)
				return err
			}
		}

		mutableState.Unlock()
	}

	// Step 3: Execute any actions defined for this event
	if err := sm.performActions(ctx, event, *eventHandler); err != nil {
		return err
	}

	// Step 4: Evaluate and execute transitions
	if err := sm.evaluateTransitions(ctx, event, *eventHandler); err != nil {
		return err
	}

	return nil
}

// evaluateEvent checks if the given event is relevant for the current state
func (sm *StateMachine[S, M, R, Cfg, Cb]) evaluateEvent(ctx context.Context, event common.Event) (*EventHandler[S, M, R, Cfg, Cb], error) {
	reader := sm.subject.GetStateReader()
	config := sm.subject.GetConfig()
	callbacks := sm.subject.GetCallbacks()
	currentState := reader.GetCurrentState()

	// Get the state definition for the current state
	stateDefinition, exists := sm.smConfig.Definitions[currentState]
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
		valid, err := eventHandler.Validator(ctx, reader, config, callbacks, event)
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
func (sm *StateMachine[S, M, R, Cfg, Cb]) performActions(ctx context.Context, event common.Event, eventHandler EventHandler[S, M, R, Cfg, Cb]) error {
	if len(eventHandler.Actions) == 0 {
		return nil
	}
	reader := sm.subject.GetStateReader()
	config := sm.subject.GetConfig()
	callbacks := sm.subject.GetCallbacks()

	reader.RLock()
	defer reader.RUnlock()
	for _, rule := range eventHandler.Actions {
		// Check event validator if present
		if rule.EventValidator != nil {
			valid, err := rule.EventValidator(ctx, reader, config, callbacks, event)
			if err != nil {
				log.L(ctx).Errorf("error in ActionRule EventValidator: %v", err)
				return err
			}
			if !valid {
				continue
			}
		}

		// Check guard condition if present
		if rule.If != nil && !rule.If(ctx, reader, config) {
			continue
		}

		// Execute the action
		if err := rule.Action(ctx, reader, config, callbacks, event); err != nil {
			log.L(ctx).Errorf("error executing action: %v", err)
			return err
		}
	}
	return nil
}

// evaluateTransitions checks each transition rule and executes the first matching one
func (sm *StateMachine[S, M, R, Cfg, Cb]) evaluateTransitions(
	ctx context.Context,
	event common.Event,
	eventHandler EventHandler[S, M, R, Cfg, Cb],
) error {
	reader := sm.subject.GetStateReader()
	mutableState := sm.subject.GetStateMutator()
	config := sm.subject.GetConfig()
	callbacks := sm.subject.GetCallbacks()

	mutableState.Lock() // TODO AM: I'm thinking this might be too coarse grained?
	defer mutableState.Unlock()

	for _, transition := range eventHandler.Transitions {
		// Check guard condition if present
		if transition.If != nil && !transition.If(ctx, reader, config) {
			continue
		}

		// Guard passed (or no guard) - execute this transition
		previousState := reader.GetCurrentState() // TODO AM: need to make sure this doesn't block
		newState := transition.To

		log.L(ctx).Debugf("state transition: %s -> %s (event: %s)",
			previousState.String(), newState.String(), event.TypeString())

		// Execute transition-specific state updates (with lock held)
		if len(transition.StateUpdates) > 0 {
			for _, stateUpdate := range transition.StateUpdates {
				if err := stateUpdate(ctx, mutableState, config, callbacks, event); err != nil {
					log.L(ctx).Errorf("error executing transition StateUpdate: %v", err)
					return err
				}
			}
		}

		// Execute transition-specific action (if defined)
		if transition.On != nil {
			if err := transition.On(ctx, reader, config, callbacks, event); err != nil {
				log.L(ctx).Errorf("error executing transition action: %v", err)
				return err
			}
		}

		// Update state machine state
		mutableState.SetCurrentState(newState)
		mutableState.SetLastStateChange(time.Now())
		mutableState.SetLatestEvent(event.TypeString())

		// Execute OnTransitionTo handler for the new state
		if newStateDefinition, exists := sm.smConfig.Definitions[newState]; exists {
			handler := newStateDefinition.OnTransitionTo

			// Execute state updates (with lock held)
			for _, rule := range handler.StateUpdates {
				// Check event validator if present
				if rule.EventValidator != nil {
					valid, err := rule.EventValidator(ctx, reader, config, callbacks, event)
					if err != nil {
						log.L(ctx).Errorf("error in OnTransitionTo StateUpdate EventValidator for state %s: %v", newState.String(), err)
						return err
					}
					if !valid {
						continue
					}
				}

				// Check guard condition if present
				if rule.If != nil && !rule.If(ctx, reader, config) {
					continue
				}

				// Execute the state update
				if err := rule.StateUpdate(ctx, mutableState, config, callbacks, event); err != nil {
					log.L(ctx).Errorf("error executing OnTransitionTo StateUpdate for state %s: %v", newState.String(), err)
					return err
				}
			}

			// Then execute actions
			for _, rule := range handler.Actions {
				// Check event validator if present
				if rule.EventValidator != nil {
					valid, err := rule.EventValidator(ctx, reader, config, callbacks, event)
					if err != nil {
						log.L(ctx).Errorf("error in OnTransitionTo Action EventValidator for state %s: %v", newState.String(), err)
						return err
					}
					if !valid {
						continue
					}
				}

				// Check guard condition if present
				if rule.If != nil && !rule.If(ctx, reader, config) {
					continue
				}

				if err := rule.Action(ctx, reader, config, callbacks, event); err != nil {
					log.L(ctx).Errorf("error executing OnTransitionTo Action for state %s: %v", newState.String(), err)
					return err
				}
			}
		}

		// Call the transition callback if configured
		if sm.smConfig.OnTransition != nil {
			// Re-fetch reader for callback
			sm.smConfig.OnTransition(ctx, reader, config, callbacks, previousState, newState, event) // TODO AM: what scope does this function have? I think it should maybe be removed
		}

		// Only the first matching transition is executed
		break
	}

	return nil
}

// TODO AM: I think this function goes if we get rid of OnTransition

// TimeSinceStateChange returns the duration since the last state change
func (sm *StateMachine[S, M, R, Cfg, Cb]) TimeSinceStateChange() time.Duration {
	// TODO AM: what's the locking model here if the function doesn't go?
	return time.Since(sm.subject.GetStateReader().GetLastStateChange())
}
