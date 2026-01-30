/*
 * Copyright © 2025 Kaleido, Inc.
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
	"fmt"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
)

type State int

const (
	State_Initial                 State = iota // Initial state before anything is calculated
	State_Pooled                               // waiting in the pool to be assembled - TODO should rename to "Selectable" or "Selectable_Pooled".  Related to potential rename of `State_PreAssembly_Blocked`
	State_PreAssembly_Blocked                  // has not been assembled yet and cannot be assembled because a dependency never got assembled successfully - i.e. it was either Parked or Reverted is also blocked
	State_Assembling                           // an assemble request has been sent but we are waiting for the response
	State_Reverted                             // the transaction has been reverted by the assembler/originator
	State_Endorsement_Gathering                // assembled and waiting for endorsement
	State_Blocked                              // is fully endorsed but cannot proceed due to dependencies not being ready for dispatch
	State_Confirming_Dispatchable              // endorsed and waiting for confirmation that were are OK to dispatch. The originator can still request not to proceed at this point.
	State_Ready_For_Dispatch                   // dispatch confirmation received and waiting to be collected by the dispatcher thread.Going into this state is the point of no return
	State_Dispatched                           // collected by the dispatcher thread but not yet processed by the public TX manager
	State_SubmissionPrepared                   // collected by the public TX manager but not yet submitted
	State_Submitted                            // at least one submission has been made to the blockchain
	State_Confirmed                            // "recently" confirmed on the base ledger.  NOTE: confirmed transactions are not held in memory for ever so getting a list of confirmed transactions will only return those confirmed recently
	State_Final                                // final state for the transaction. Transactions are removed from memory as soon as they enter this state
)

type EventType = common.EventType

const (
	Event_Received                       EventType = iota + common.Event_HeartbeatInterval + 1 // Transaction initially received by the coordinator.  Might seem redundant explicitly modeling this as an event rather than putting this logic into the constructor, but it is useful to make the initial state transition rules explicit in the state machine definitions
	Event_Selected                                                                             // selected from the pool as the next transaction to be assembled
	Event_AssembleRequestSent                                                                  // assemble request sent to the assembler
	Event_Assemble_Success                                                                     // assemble response received from the originator
	Event_Assemble_Revert_Response                                                             // assemble response received from the originator with a revert reason
	Event_Endorsed                                                                             // endorsement received from one endorser
	Event_EndorsedRejected                                                                     // endorsement received from one endorser with a revert reason
	Event_DependencyReady                                                                      // another transaction, for which this transaction has a dependency on, has become ready for dispatch
	Event_DependencyAssembled                                                                  // another transaction, for which this transaction has a dependency on, has been assembled
	Event_DependencyReverted                                                                   // another transaction, for which this transaction has a dependency on, has been reverted
	Event_DispatchRequestApproved                                                              // dispatch confirmation received from the originator
	Event_DispatchRequestRejected                                                              // dispatch confirmation response received from the originator with a rejection
	Event_Dispatched                                                                           // dispatched to the public TX manager
	Event_Collected                                                                            // collected by the public TX manager
	Event_NonceAllocated                                                                       // nonce allocated by the dispatcher thread
	Event_Submitted                                                                            // submission made to the blockchain.  Each time this event is received, the submission hash is updated
	Event_Confirmed                                                                            // confirmation received from the blockchain of either a successful or reverted transaction
	Event_RequestTimeoutInterval                                                               // event emitted by the state machine on a regular period while we have pending requests
	Event_StateTransition                                                                      // event emitted by the state machine when a state transition occurs.  TODO should this be a separate enum?
	Event_AssembleTimeout                                                                      // the assemble timeout period has passed since we sent the first assemble request
	Event_TransactionUnknownByOriginator                                                       // originator has reported that it doesn't recognize this transaction
)

// Type aliases for the generic statemachine types, specialized for Transaction
type (
	Action           = statemachine.Action[*Transaction]
	Guard            = statemachine.Guard[*Transaction]
	ActionRule       = statemachine.ActionRule[*Transaction]
	Transition       = statemachine.Transition[State, *Transaction]
	Validator        = statemachine.Validator[*Transaction]
	EventHandler     = statemachine.EventHandler[State, *Transaction]
	StateDefinition  = statemachine.StateDefinition[State, *Transaction]
	StateDefinitions = statemachine.StateDefinitions[State, *Transaction]
	StateMachine     = statemachine.StateMachine[State, *Transaction]
)

var stateDefinitionsMap = StateDefinitions{
	State_Initial: {
		Events: map[EventType]EventHandler{
			Event_Received: { //TODO rename this event type because it is the first one we see in this struct and it seems like we are saying this is a definition related to receiving an event (at one level that is correct but it is not what is meant by Event_Received)
				Transitions: []Transition{
					{
						To: State_Submitted,
						If: guard_HasChainedTxInProgress,
					},
					{
						To: State_Pooled,
						If: statemachine.And(statemachine.Not(guard_HasUnassembledDependencies), statemachine.Not(guard_HasUnknownDependencies)),
					},
					{
						To: State_PreAssembly_Blocked,
						If: statemachine.Or(guard_HasUnassembledDependencies, guard_HasUnknownDependencies),
					},
				},
			},
		},
	},
	State_PreAssembly_Blocked: {
		Events: map[EventType]EventHandler{
			Event_DependencyAssembled: {
				Transitions: []Transition{{
					To: State_Pooled,
					If: statemachine.Not(guard_HasUnassembledDependencies),
				}},
			},
		},
	},
	State_Pooled: {
		OnTransitionTo: action_initializeDependencies,
		Events: map[EventType]EventHandler{
			Event_Selected: {
				Transitions: []Transition{
					{
						To: State_Assembling,
					}},
			},
			Event_DependencyReverted: {
				Transitions: []Transition{{
					To: State_PreAssembly_Blocked,
				}},
			},
		},
	},
	State_Assembling: {
		OnTransitionTo: action_SendAssembleRequest,
		Events: map[EventType]EventHandler{
			Event_Assemble_Success: {
				Validator: validator_MatchesPendingAssembleRequest,
				Actions:   []ActionRule{{Action: action_AssembleSuccess}},
				Transitions: []Transition{
					{
						To:     State_Endorsement_Gathering,
						Action: action_NotifyDependentsOfAssembled,
						If:     statemachine.Not(guard_AttestationPlanFulfilled),
					},
					{
						To: State_Confirming_Dispatchable,
						If: statemachine.And(guard_AttestationPlanFulfilled, statemachine.Not(guard_HasDependenciesNotReady)),
					}},
			},
			Event_RequestTimeoutInterval: {
				Actions: []ActionRule{{
					Action: action_NudgeAssembleRequest,
					If:     statemachine.Not(guard_AssembleTimeoutExceeded),
				}},
				Transitions: []Transition{{
					To:     State_Pooled,
					If:     guard_AssembleTimeoutExceeded,
					Action: action_IncrementAssembleErrors,
				}},
			},
			Event_Assemble_Revert_Response: {
				Validator: validator_MatchesPendingAssembleRequest,
				Actions:   []ActionRule{{Action: action_AssembleRevertResponse}},
				Transitions: []Transition{{
					To: State_Reverted,
				}},
			},
			// Handle response from originator indicating it doesn't recognize this transaction.
			// The most likely cause is that the transaction reached a terminal state (e.g., reverted
			// during assembly) but the response was lost, and the transaction has since been removed
			// from memory on the originator after cleanup. The coordinator should clean up this transaction.
			Event_TransactionUnknownByOriginator: {
				Transitions: []Transition{{
					To:     State_Final,
					Action: action_FinalizeAsUnknownByOriginator,
				}},
			},
		},
	},
	State_Endorsement_Gathering: {
		OnTransitionTo: action_SendEndorsementRequests,
		Events: map[EventType]EventHandler{
			Event_Endorsed: {
				Actions: []ActionRule{{Action: action_Endorsed}},
				Transitions: []Transition{
					{
						To: State_Confirming_Dispatchable,
						If: statemachine.And(guard_AttestationPlanFulfilled, statemachine.Not(guard_HasDependenciesNotReady)),
					},
					{
						To: State_Blocked,
						If: statemachine.And(guard_AttestationPlanFulfilled, guard_HasDependenciesNotReady),
					},
				},
			},
			Event_EndorsedRejected: {
				Actions: []ActionRule{{Action: action_EndorsedRejected}},
				Transitions: []Transition{
					{
						To:     State_Pooled,
						Action: action_IncrementAssembleErrors,
					},
				},
			},
			Event_RequestTimeoutInterval: {
				Actions: []ActionRule{{
					Action: action_NudgeEndorsementRequests,
				}},
			},
			Event_DependencyReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Blocked: {
		Events: map[EventType]EventHandler{
			Event_DependencyReady: {
				Transitions: []Transition{{
					To: State_Confirming_Dispatchable,
					If: statemachine.And(guard_AttestationPlanFulfilled, statemachine.Not(guard_HasDependenciesNotReady)),
				}},
			},
			Event_DependencyReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Confirming_Dispatchable: {
		OnTransitionTo: action_SendPreDispatchRequest,
		Events: map[EventType]EventHandler{
			Event_DispatchRequestApproved: {
				Validator: validator_MatchesPendingPreDispatchRequest,
				Actions:   []ActionRule{{Action: action_DispatchRequestApproved}},
				Transitions: []Transition{
					{
						To: State_Ready_For_Dispatch,
					}},
			},
			Event_RequestTimeoutInterval: {
				Actions: []ActionRule{{
					Action: action_NudgePreDispatchRequest,
				}},
			},
			Event_DependencyReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Ready_For_Dispatch: {
		OnTransitionTo: action_NotifyDependentsOfReadiness,
		Events: map[EventType]EventHandler{
			Event_Dispatched: {
				Transitions: []Transition{
					{
						To: State_Dispatched,
					}},
			},
			Event_DependencyReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Dispatched: {
		Events: map[EventType]EventHandler{
			Event_Collected: {
				Actions: []ActionRule{{Action: action_Collected}},
				Transitions: []Transition{
					{
						To: State_SubmissionPrepared,
					}},
			},
			Event_DependencyReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_SubmissionPrepared: {
		Events: map[EventType]EventHandler{
			Event_Submitted: {
				Actions: []ActionRule{{Action: action_Submitted}},
				Transitions: []Transition{
					{
						To: State_Submitted,
					}},
			},
			Event_NonceAllocated: {
				Actions: []ActionRule{{Action: action_NonceAllocated}},
			},
			Event_DependencyReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Submitted: {
		Events: map[EventType]EventHandler{
			Event_Confirmed: {
				Actions: []ActionRule{{Action: action_Confirmed}},
				Transitions: []Transition{
					{
						If: statemachine.Not(guard_HasRevertReason),
						To: State_Confirmed,
					},
					{
						Action: action_recordRevert,
						If:     guard_HasRevertReason,
						To:     State_Pooled,
					},
				},
			},
			Event_DependencyReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Reverted: {
		OnTransitionTo: action_NotifyDependentsOfRevert,
		Events: map[EventType]EventHandler{
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{{Action: action_IncrementHeartbeatIntervalsSinceStateChange}},
				Transitions: []Transition{
					{
						If: guard_HasGracePeriodPassedSinceStateChange,
						To: State_Final,
					}},
			},
		},
	},
	State_Confirmed: {
		OnTransitionTo: action_NotifyOfConfirmation,
		Events: map[EventType]EventHandler{
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{{Action: action_IncrementHeartbeatIntervalsSinceStateChange}},
				Transitions: []Transition{
					{
						If: guard_HasGracePeriodPassedSinceStateChange,
						To: State_Final,
					}},
			},
		},
	},
	State_Final: {
		// Cleanup is handled by the coordinator in response to the state transition event
	},
}

func (t *Transaction) initializeStateMachine(initialState State) {
	t.stateMachine = statemachine.NewStateMachine(initialState, stateDefinitionsMap,
		statemachine.WithTransitionCallback(func(ctx context.Context, t *Transaction, from, to State, event common.Event) {
			// Reset heartbeat counter on state change
			t.heartbeatIntervalsSinceStateChange = 0

			// Log the state transition
			log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf(
				"coord-tx | %s   | %s | %T | %s -> %s",
				t.pt.Address.String()[0:8], t.pt.ID.String()[0:8], event, from.String(), to.String())

			// Record metrics
			t.metrics.ObserveSequencerTXStateChange("Coord_"+to.String(), time.Duration(event.GetEventTime().Sub(t.stateMachine.GetLastStateChange()).Milliseconds()))

			// Queue state transition event for the coordinator
			if t.queueEventForCoordinator != nil {
				t.queueEventForCoordinator(ctx, &common.TransactionStateTransitionEvent[State]{
					BaseEvent:     common.BaseEvent{EventTime: time.Now()},
					TransactionID: t.pt.ID,
					From:          from,
					To:            to,
				})
			}
		}),
	)
}

func (t *Transaction) HandleEvent(ctx context.Context, event common.Event) error {
	log.L(ctx).Infof("transaction state machine handling new event (TX ID %s, TX originator %s, TX address %+v)", t.pt.ID.String(), t.originator, t.pt.Address.HexString())
	return t.stateMachine.ProcessEvent(ctx, t, event)
}

func action_Collected(_ context.Context, t *Transaction, event common.Event) error {
	e := event.(*CollectedEvent)
	t.signerAddress = &e.SignerAddress
	return nil
}

func action_NonceAllocated(ctx context.Context, t *Transaction, event common.Event) error {
	e := event.(*NonceAllocatedEvent)
	t.nonce = &e.Nonce
	return t.transportWriter.SendNonceAssigned(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce)
}

func action_Submitted(ctx context.Context, t *Transaction, event common.Event) error {
	e := event.(*SubmittedEvent)
	log.L(ctx).Infof("coordinator transaction applying SubmittedEvent for transaction %s submitted with hash %s", t.pt.ID.String(), e.SubmissionHash.HexString())
	t.latestSubmissionHash = &e.SubmissionHash
	return t.transportWriter.SendTransactionSubmitted(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, &e.SubmissionHash)
}

func action_Confirmed(ctx context.Context, t *Transaction, event common.Event) error {
	e := event.(*ConfirmedEvent)
	t.revertReason = e.RevertReason
	return t.transportWriter.SendTransactionConfirmed(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce, e.RevertReason)
}

func action_IncrementHeartbeatIntervalsSinceStateChange(ctx context.Context, t *Transaction, _ common.Event) error {
	log.L(ctx).Tracef("coordinator transaction %s (%s) increasing heartbeatIntervalsSinceStateChange to %d", t.pt.ID.String(), t.stateMachine.CurrentState.String(), t.heartbeatIntervalsSinceStateChange+1)
	t.heartbeatIntervalsSinceStateChange++
	return nil
}

func (s State) String() string {
	switch s {
	case State_Initial:
		return "State_Initial"
	case State_Pooled:
		return "State_Pooled"
	case State_PreAssembly_Blocked:
		return "State_PreAssembly_Blocked"
	case State_Assembling:
		return "State_Assembling"
	case State_Reverted:
		return "State_Reverted"
	case State_Endorsement_Gathering:
		return "State_Endorsement_Gathering"
	case State_Blocked:
		return "State_Blocked"
	case State_Confirming_Dispatchable:
		return "State_Confirming_Dispatchable"
	case State_Ready_For_Dispatch:
		return "State_Ready_For_Dispatch"
	case State_Dispatched:
		return "State_Dispatched"
	case State_SubmissionPrepared:
		return "State_SubmissionPrepared"
	case State_Submitted:
		return "State_Submitted"
	case State_Confirmed:
		return "State_Confirmed"
	case State_Final:
		return "State_Final"
	}
	return fmt.Sprintf("Unknown (%d)", s)
}
