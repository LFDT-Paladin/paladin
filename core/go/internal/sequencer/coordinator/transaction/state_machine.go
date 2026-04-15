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
	State_Dispatched                           // collected by the dispatcher/public TX manager and in-flight on base ledger
	State_Confirmed                            // "recently" confirmed on the base ledger.  NOTE: confirmed transactions are not held in memory for ever so getting a list of confirmed transactions will only return those confirmed recently
	State_Final                                // final state for the transaction. Transactions are removed from memory as soon as they enter this state
	State_Evicted                              // evicted state for a problematic transaction. Transactions are removed from memory as soon as they enter this state. Distinct from State_Final because it might just used for memory or in-flight slot management
)

type EventType = common.EventType

const (
	Event_Delegated                      EventType = iota + common.Event_HeartbeatInterval + 1 // Transaction initially received by the coordinator.  Might seem redundant explicitly modeling this as an event rather than putting this logic into the constructor, but it is useful to make the initial state transition rules explicit in the state machine definitions
	Event_DependencySelectedForAssemble                                                        // the transaction delegated immediately before the transaction from the same originator has been selected for assembly
	Event_NewPreAssembleDependency                                                             // a new preassemble dependency has been established
	Event_Selected                                                                             // selected from the pool as the next transaction to be assembled
	Event_AssembleRequestSent                                                                  // assemble request sent to the assembler
	Event_Assemble_Success                                                                     // assemble response received from the originator
	Event_Assemble_Revert_Response                                                             // assemble response received from the originator with a revert reason
	Event_Assemble_Error_Response                                                              // assemble response received from the originator with an error
	Event_Assemble_Cancelled                                                                   // the assemble attempt has been cancelled
	Event_Endorsed                                                                             // endorsement received from one endorser
	Event_EndorsedRejected                                                                     // endorsement received from one endorser with a revert reason
	Event_DependencyReady                                                                      // another transaction, for which this transaction has a dependency on, has become ready for dispatch
	Event_DependencyAssembled                                                                  // another transaction, for which this transaction has a dependency on, has been assembled
	Event_DependencyReverted                                                                   // another transaction, for which this transaction has a dependency on, has been reverted
	Event_DependencyReset                                                                      // another transaction, for which this transaction has a dependency on, has been reset
	Event_DependencyConfirmedReverted                                                          // another transaction, for which this transaction has a dependency on, has been confirmed as reverted
	Event_DispatchRequestApproved                                                              // dispatch confirmation received from the originator
	Event_DispatchRequestRejected                                                              // dispatch confirmation response received from the originator with a rejection
	Event_Dispatched                                                                           // dispatched to the public TX manager
	Event_Collected                                                                            // collected by the public TX manager
	Event_NonceAllocated                                                                       // nonce allocated by the dispatcher thread
	Event_Submitted                                                                            // submission made to the blockchain.  Each time this event is received, the submission hash is updated
	Event_ConfirmedSuccess                                                                     // confirmation received from the blockchain of a successful transaction
	Event_ConfirmedReverted                                                                    // confirmation received from the blockchain of a reverted transaction
	Event_RequestTimeoutInterval                                                               // event emitted by the state machine on a regular period while we have pending requests
	Event_StateTimeoutInterval                                                                 // event emitted when a state has exceeded its maximum allowed duration
	Event_StateTransition                                                                      // event emitted by the state machine when a state transition occurs.  TODO should this be a separate enum?
	Event_TransactionUnknownByOriginator                                                       // originator has reported that it doesn't recognize this transaction
)

// Type aliases for the generic statemachine types, specialized for Transaction
type (
	Action           = statemachine.Action[*coordinatorTransaction]
	Guard            = statemachine.Guard[*coordinatorTransaction]
	ActionRule       = statemachine.ActionRule[*coordinatorTransaction]
	Transition       = statemachine.Transition[State, *coordinatorTransaction]
	Validator        = statemachine.Validator[*coordinatorTransaction]
	EventHandler     = statemachine.EventHandler[State, *coordinatorTransaction]
	StateDefinition  = statemachine.StateDefinition[State, *coordinatorTransaction]
	StateDefinitions = statemachine.StateDefinitions[State, *coordinatorTransaction]
	StateMachine     = statemachine.StateMachine[State, *coordinatorTransaction]
)

var stateDefinitionsMap = StateDefinitions{
	State_Initial: {
		Events: map[EventType]EventHandler{
			Event_Delegated: {
				Transitions: []Transition{
					{
						To: State_PreAssembly_Blocked,
						If: guard_HasUnassembledDependencies,
					},
					{
						To: State_Pooled,
						If: statemachine.GuardNot(guard_HasUnassembledDependencies), // No-op check (opposite of guard_HasUnassembledDependencies above) but including to be explicit when we should go to pooled
					},
				},
			},
			Event_NewPreAssembleDependency: {
				Actions: []ActionRule{{Action: action_AddPreAssemblePrereqOf}},
			},
		},
	},
	State_PreAssembly_Blocked: {
		Events: map[EventType]EventHandler{
			Event_NewPreAssembleDependency: {
				Actions: []ActionRule{{Action: action_AddPreAssemblePrereqOf}},
			},
			// Waiting for this event before we moved to pooled ensures FIFO ordering for first assembly within an originator.
			Event_DependencySelectedForAssemble: {
				Actions: []ActionRule{{Action: action_RemovePreAssembleDependency}},
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Pooled: {
		OnTransitionTo: []ActionRule{
			{Action: action_NotifyDependentsOfReset},
			{Action: action_InitializeForNewAssembly},
		},
		Events: map[EventType]EventHandler{
			Event_NewPreAssembleDependency: {
				Actions: []ActionRule{{Action: action_AddPreAssemblePrereqOf}},
			},
			Event_Selected: {
				Actions: []ActionRule{
					// We notify the preassemble dependent at the point of selection, since the outcome of assembly is irrelevant
					// to ensuring FIFO for first assembly within an originator.
					{Action: action_NotifyPreAssembleDependentOfSelection},
					{Action: action_RemovePreAssemblePrereqOf},
				},
				Transitions: []Transition{
					{
						To: State_Assembling,
					}},
			},
		},
	},
	State_Assembling: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_SendAssembleRequest},
		},
		Events: map[EventType]EventHandler{
			Event_Assemble_Success: {
				Validator: validator_MatchesPendingAssembleRequest,
				Actions: []ActionRule{
					{
						Action: action_AssembleSuccess,
					},
					{
						Action: action_UpdateSigningIdentity,
						If:     statemachine.GuardAnd(guard_AttestationPlanFulfilled, statemachine.GuardNot(guard_HasSigner)),
					},
				},
				Transitions: []Transition{
					{
						To: State_Endorsement_Gathering,
						If: statemachine.GuardNot(guard_AttestationPlanFulfilled),
					},
					{
						To: State_Confirming_Dispatchable,
						If: statemachine.GuardAnd(guard_AttestationPlanFulfilled, statemachine.GuardNot(guard_HasDependenciesNotReady)),
					}},
			},
			Event_RequestTimeoutInterval: {
				Actions: []ActionRule{{
					Action: action_NudgeAssembleRequest,
				}},
			},
			Event_StateTimeoutInterval: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
			Event_Assemble_Cancelled: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
			Event_Assemble_Revert_Response: {
				Validator: validator_MatchesPendingAssembleRequest,
				Actions:   []ActionRule{{Action: action_AssembleRevertResponse}},
				Transitions: []Transition{{
					To: State_Reverted,
				}},
			},
			Event_Assemble_Error_Response: {
				Validator: validator_MatchesPendingAssembleRequest,
				Actions:   []ActionRule{{Action: action_AssembleError}},
				Transitions: []Transition{
					{
						If: guard_CanRetryErroredAssemble,
						To: State_Pooled,
					},
					{
						If: statemachine.GuardNot(guard_CanRetryErroredAssemble),
						To: State_Evicted,
					},
				},
			},
			// Handle response from originator indicating it doesn't recognize this transaction.
			// The most likely cause is that the transaction reached a terminal state (e.g., reverted
			// during assembly) but the response was lost, and the transaction has since been removed
			// from memory on the originator after cleanup. The coordinator should clean up this transaction.
			Event_TransactionUnknownByOriginator: {
				Transitions: []Transition{{
					To:      State_Final,
					Actions: []ActionRule{{Action: action_FinalizeAsUnknownByOriginator}},
				}},
			},
		},
	},
	State_Endorsement_Gathering: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_SendEndorsementRequests},
		},
		Events: map[EventType]EventHandler{
			Event_Endorsed: {
				Actions: []ActionRule{
					{
						Action: action_Endorsed,
					},
					{
						Action: action_ResetEndorsementRequests,
						If:     guard_AttestationPlanFulfilled,
					},
					{
						Action: action_UpdateSigningIdentity,
						If:     statemachine.GuardAnd(guard_AttestationPlanFulfilled, statemachine.GuardNot(guard_HasSigner)),
					}},
				Transitions: []Transition{
					{
						To: State_Confirming_Dispatchable,
						If: statemachine.GuardAnd(guard_AttestationPlanFulfilled, statemachine.GuardNot(guard_HasDependenciesNotReady)),
					},
					{
						To: State_Blocked,
						If: statemachine.GuardAnd(guard_AttestationPlanFulfilled, guard_HasDependenciesNotReady),
					},
				},
			},
			Event_EndorsedRejected: {
				Transitions: []Transition{
					{
						To: State_Pooled,
					},
				},
			},
			Event_RequestTimeoutInterval: {
				Actions: []ActionRule{{
					Action: action_NudgeEndorsementRequests,
				}},
			},
			Event_StateTimeoutInterval: {
				Transitions: []Transition{
					{
						To: State_Pooled,
					},
				},
			},
			Event_DependencyReset: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
			Event_DependencyConfirmedReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Blocked: {
		Events: map[EventType]EventHandler{
			Event_DependencyReady: {
				Actions: []ActionRule{
					{
						Action: action_UpdateSigningIdentity,
						If:     statemachine.GuardAnd(guard_AttestationPlanFulfilled, statemachine.GuardNot(guard_HasSigner)),
					}},
				Transitions: []Transition{{
					To: State_Confirming_Dispatchable,
					If: statemachine.GuardAnd(guard_AttestationPlanFulfilled, statemachine.GuardNot(guard_HasDependenciesNotReady)),
				}},
			},
			Event_DependencyReset: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
			Event_DependencyConfirmedReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Confirming_Dispatchable: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_SendPreDispatchRequest},
		},
		Events: map[EventType]EventHandler{
			Event_DispatchRequestApproved: {
				Validator: validator_MatchesPendingPreDispatchRequest,
				Actions:   []ActionRule{{Action: action_DispatchRequestApproved}},
				Transitions: []Transition{
					{
						To: State_Ready_For_Dispatch,
					}},
			},
			Event_DispatchRequestRejected: {
				Validator: validator_MatchesPendingPreDispatchRequest,
				Actions:   []ActionRule{{Action: action_DispatchRequestRejected}},
				Transitions: []Transition{
					{
						To: State_Pooled,
					},
				},
			},
			Event_RequestTimeoutInterval: {
				Actions: []ActionRule{{
					Action: action_NudgePreDispatchRequest,
				}},
			},
			Event_StateTimeoutInterval: {
				Transitions: []Transition{
					{
						To:      State_Pooled,
						Actions: []ActionRule{{Action: action_DispatchRequestRejected}},
					},
				},
			},
			Event_DependencyReset: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
			Event_DependencyConfirmedReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Ready_For_Dispatch: {
		OnTransitionTo: []ActionRule{
			{Action: action_AllocateSigningIdentity},
			{Action: action_NotifyDependentsOfReadiness},
		},
		Events: map[EventType]EventHandler{
			Event_Dispatched: {
				Actions: []ActionRule{
					{
						Action: action_Dispatch,
					},
				},
				Transitions: []Transition{
					{
						To: State_Dispatched,
					},
				},
			},
			Event_DependencyReset: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
			Event_DependencyConfirmedReverted: {
				Transitions: []Transition{{
					To: State_Pooled,
				}},
			},
		},
	},
	State_Dispatched: {
		OnTransitionTo: []ActionRule{
			{Action: action_NotifyDispatched},
			{Action: action_CleanUpAssemblyPayload},
		},
		Events: map[EventType]EventHandler{
			Event_Collected: {
				Actions: []ActionRule{{Action: action_NotifyCollected}},
			},
			Event_NonceAllocated: {
				Actions: []ActionRule{{Action: action_NotifyNonceAllocated}},
			},
			Event_Submitted: {
				Actions: []ActionRule{{Action: action_NotifySubmitted}},
			},
			Event_ConfirmedSuccess: {
				Actions: []ActionRule{
					{Action: action_RecordConfirmation},
					{Action: action_NotifyOriginatorOfConfirmation},
					{Action: action_NotifyDependantsOfSuccessfulConfirmation},
				},
				Transitions: []Transition{{To: State_Confirmed}},
			},
			Event_ConfirmedReverted: {
				Actions: []ActionRule{
					{
						Action: action_RecordConfirmation,
					},
				},
				Transitions: []Transition{
					{
						If: guard_CanRetryRevert,
						To: State_Pooled,
						Actions: []ActionRule{
							{Action: action_NotifyOriginatorOfRetryableRevert},
						},
					},
					{
						If: statemachine.GuardNot(guard_CanRetryRevert),
						To: State_Reverted,
						Actions: []ActionRule{
							{Action: action_NotifyOriginatorOfNonRetryableRevert},
							{Action: action_NotifyDependantsOfRevertedConfirmation},
							{Action: action_FinalizeNonRetryableRevert},
						},
					},
				},
			},
			Event_DependencyReset: {
				Actions: []ActionRule{
					{Action: action_ResetTransactionLocks},
					{Action: action_NotifyDependentsOfReset},
				},
			},
			Event_DependencyConfirmedReverted: {
				Actions: []ActionRule{
					{Action: action_ResetTransactionLocks},
					{Action: action_NotifyDependentsOfReset},
				},
			},
		},
	},
	State_Reverted: {
		OnTransitionTo: []ActionRule{
			{Action: action_ResetTransactionLocks},
		},
		Events: map[EventType]EventHandler{
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{{Action: action_IncrementHeartbeatIntervalsSinceStateChange}},
				Transitions: []Transition{
					{
						If: guard_HasFinalizingGracePeriodPassedSinceStateChange,
						To: State_Final,
					}},
			},
		},
	},
	State_Confirmed: {
		Events: map[EventType]EventHandler{
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{
						Action: action_IncrementHeartbeatIntervalsSinceStateChange,
					},
					{
						// TODO: this could be handled in a more sophisticated way using block height, either
						// by resetting a number of blocks after confirmation, or by removing this grace period
						// by only allowing originators to assemble if they are at the same block height as the
						// coordinator
						Action: action_ResetConfirmedTransactionLocksOnce,
						If:     guard_HasConfirmedLockRetentionGracePeriodPassedSinceStateChange,
					},
				},
				Transitions: []Transition{
					{
						If: guard_HasFinalizingGracePeriodPassedSinceStateChange,
						To: State_Final,
					}},
			},
		},
	},
	State_Final: {
		// Cleanup is handled by the coordinator in response to the state transition event
	},
	State_Evicted: {
		// Cleanup is handled by the coordinator in response to the state transition event
	},
}

func (t *coordinatorTransaction) initializeStateMachine(initialState State) {
	t.stateMachine = statemachine.NewStateMachine(initialState, stateDefinitionsMap,
		fmt.Sprintf("coord-tx-%s", t.pt.ID.String()[0:8]),
		statemachine.WithTransitionCallback(func(ctx context.Context, t *coordinatorTransaction, from, to State, event common.Event) {
			// Reset heartbeat counter on state change
			t.heartbeatIntervalsSinceStateChange = 0
			t.stateEntryTime = t.clock.Now()

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
	t.stateEntryTime = t.clock.Now()
}

func (t *coordinatorTransaction) HandleEvent(ctx context.Context, event common.Event) error {
	// Adding the log field here means every function called by the transaction state machine will have the txID field
	// in addition to the fields of the parent context
	txCtx := log.WithLogField(ctx, "txID", t.pt.ID.String())
	return t.stateMachine.ProcessEvent(txCtx, t, event)
}

func action_IncrementHeartbeatIntervalsSinceStateChange(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
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
	case State_Confirmed:
		return "State_Confirmed"
	case State_Final:
		return "State_Final"
	case State_Evicted:
		return "State_Evicted"
	}
	return fmt.Sprintf("Unknown (%d)", s)
}
