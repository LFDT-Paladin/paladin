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

type State = common.CoordinatorTransactionState

const (
	State_Initial                 = common.CoordinatorTransactionState_Initial                 // Initial state before anything is calculated
	State_Pooled                  = common.CoordinatorTransactionState_Pooled                  // Waiting in the pool to be assembled
	State_PreAssembly_Blocked     = common.CoordinatorTransactionState_PreAssembly_Blocked     // Cannot be assembled; a dependency was Parked or Reverted
	State_Assembling              = common.CoordinatorTransactionState_Assembling              // Assemble request sent to the originator; waiting for response
	State_Reverted                = common.CoordinatorTransactionState_Reverted                // Reverted by the assembler/originator or on the base ledger
	State_Endorsement_Gathering   = common.CoordinatorTransactionState_Endorsement_Gathering   // Assembled and waiting for endorsement
	State_Blocked                 = common.CoordinatorTransactionState_Blocked                 // Fully endorsed but dependencies not ready for dispatch
	State_Confirming_Dispatchable = common.CoordinatorTransactionState_Confirming_Dispatchable // Endorsed; waiting for originator dispatch confirmation; originator can still decline
	State_Ready_For_Dispatch      = common.CoordinatorTransactionState_Ready_For_Dispatch      // Dispatch confirmation received; point of no return; waiting to be collected by the dispatcher
	State_Dispatched              = common.CoordinatorTransactionState_Dispatched              // Collected by the dispatcher/public TX manager; in-flight on the base ledger
	State_Confirmed               = common.CoordinatorTransactionState_Confirmed               // Recently confirmed on the base ledger; not held in memory indefinitely
	State_Final                   = common.CoordinatorTransactionState_Final                   // Final state; transactions are removed from memory on entry
	State_Evicted                 = common.CoordinatorTransactionState_Evicted                 // Evicted for memory/in-flight slot management; distinct from Final
)

type EventType = common.EventType

const (
	Event_Delegated                       EventType = iota + common.Event_HeartbeatInterval + 1 // Transaction initially received by the coordinator.  Might seem redundant explicitly modeling this as an event rather than putting this logic into the constructor, but it is useful to make the initial state transition rules explicit in the state machine definitions
	Event_DependencySelectedForAssemble                                                         // the transaction delegated immediately before the transaction from the same originator has been selected for assembly
	Event_Selected                                                                              // selected from the pool as the next transaction to be assembled
	Event_AssembleRequestSent                                                                   // assemble request sent to the assembler
	Event_Assemble_Success                                                                      // assemble response received from the originator
	Event_Assemble_Revert_Response                                                              // assemble response received from the originator with a revert reason
	Event_Assemble_Error_Response                                                               // assemble response received from the originator with an error
	Event_Assemble_Cancelled                                                                    // the assemble attempt has been cancelled
	Event_Endorsed                                                                              // endorsement received from one endorser
	Event_EndorsedRejected                                                                      // endorsement received from one endorser with a revert reason
	Event_DependencyReady                                                                       // another transaction, for which this transaction has a dependency on, has become ready for dispatch
	Event_DependencyReset                                                                       // another transaction, for which this transaction has a dependency on, has been reset
	Event_DependencyConfirmedReverted                                                           // another transaction, for which this transaction has a dependency on, has been confirmed as reverted
	Event_DispatchRequestApproved                                                               // dispatch confirmation received from the originator
	Event_DispatchRequestRejected                                                               // dispatch confirmation response received from the originator with a rejection
	Event_Dispatched                                                                            // dispatched to the public TX manager
	Event_Collected                                                                             // collected by the public TX manager
	Event_NonceAllocated                                                                        // nonce allocated by the dispatcher thread
	Event_Submitted                                                                             // submission made to the blockchain.  Each time this event is received, the submission hash is updated
	Event_ConfirmedSuccess                                                                      // confirmation received from the blockchain of a successful transaction
	Event_ConfirmedReverted                                                                     // confirmation received from the blockchain of a reverted transaction
	Event_RequestTimeoutInterval                                                                // event emitted by the state machine on a regular period while we have pending requests
	Event_StateTimeoutInterval                                                                  // event emitted when a state has exceeded its maximum allowed duration
	Event_StateTransition                                                                       // event emitted by the state machine when a state transition occurs.  TODO should this be a separate enum?
	Event_TransactionUnknownByOriginator                                                        // originator has reported that it doesn't recognize this transaction
	Event_ChainedDependencyFailed                                                               // a chained (same-coordinator) dependency has been permanently finalized as failed
	Event_ChainedDependencyEvicted                                                              // a chained (same-coordinator) dependency has been evicted (e.g. assembly failure threshold exceeded)
	Event_PreAssembleDependencyTerminated                                                       // the pre-assemble (FIFO ordering) predecessor has reached a terminal state
	Event_NotActiveCoordinator                                                                  // originator has reported that this node is not the active coordinator for the transaction
)

// Type aliases for the generic statemachine types, specialized for Transaction
type (
	Action           = statemachine.Action[*coordinatorTransaction]
	Guard            = statemachine.Guard[*coordinatorTransaction]
	ActionRule       = statemachine.ActionRule[*coordinatorTransaction]
	Transition       = statemachine.Transition[State, *coordinatorTransaction]
	Validator        = statemachine.Validator[*coordinatorTransaction]
	EventHandler     = statemachine.EventHandler[State, *coordinatorTransaction]
	EventHandlers    = statemachine.EventHandlers[State, *coordinatorTransaction]
	StateDefinition  = statemachine.StateDefinition[State, *coordinatorTransaction]
	StateDefinitions = statemachine.StateDefinitions[State, *coordinatorTransaction]
	StateMachine     = statemachine.StateMachine[State, *coordinatorTransaction]
)

var stateDefinitionsMap = StateDefinitions{
	State_Initial: {
		Events: map[EventType]EventHandlers{
			// State_Initial only needs to handle Event_Delegated. The transaction is created and
			// immediately delegated on the coordinator event loop, so no other events can arrive
			// before Event_Delegated is processed.
			Event_Delegated: {Handlers: []EventHandler{{
				Transitions: []Transition{
					{
						To:      State_Reverted,
						If:      guard_HasRevertedChainedDependency,
						Actions: []ActionRule{{Action: action_FinalizeOnRevertedChainedDependencyAtCreation}},
					},
					{
						To: State_Evicted,
						If: guard_HasEvictedChainedDependency,
					},
					{
						To: State_PreAssembly_Blocked,
						If: guard_HasUnassembledDependencies,
					},
					{
						To: State_Pooled,
						If: statemachine.GuardNot(guard_HasUnassembledDependencies), // No-op check (opposite of guard_HasUnassembledDependencies above) but including to be explicit when we should go to pooled
					},
				},
			}}},
		},
	},
	State_PreAssembly_Blocked: {
		OnTransitionTo: []ActionRule{
			// this transition action is duplicated when the transaction moves to State_Pooled,
			// but the dupliction is safe, and including it on both avoids extra complexity in
			// all the places where a transaction may go to either State_PreAssembly_Blocked or
			// State_Pooled depending on its dependencies.
			{Action: action_InitializeForNewAssembly},
		},

		Events: map[EventType]EventHandlers{
			// Waiting for this event before we move to pooled ensures FIFO ordering for first assembly within an originator
			// and preservers chained dependency ordering
			Event_DependencySelectedForAssemble: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Validator: validator_IsPreAssembleDependency,
					Actions:   []ActionRule{{Action: action_RemovePreAssembleDependency}},
				}, {
					Validator: validator_IsChainedDependency,
					Actions:   []ActionRule{{Action: action_MarkChainedDependencyAssembled}},
				}, {
					Transitions: []Transition{
						{
							To: State_Pooled,
							If: statemachine.GuardNot(guard_HasUnassembledDependencies),
						},
					},
				}}},
			// The pre-assemble predecessor reached a terminal state — sever the FIFO link
			// so this transaction is not stuck waiting forever
			Event_PreAssembleDependencyTerminated: {Handlers: []EventHandler{{
				Actions: []ActionRule{{Action: action_RemovePreAssembleDependency}},
				Transitions: []Transition{
					{
						To: State_Pooled,
						If: statemachine.GuardNot(guard_HasUnassembledDependencies),
					},
				},
			}}},
			Event_ChainedDependencyFailed: {Handlers: []EventHandler{{
				Actions:     []ActionRule{{Action: action_FinalizeOnChainedDependencyFailure}},
				Transitions: []Transition{{To: State_Reverted}},
			}}},
			// Event_ChainedDependencyEvicted is only handled in pre-assembly states (Initial,
			// PreAssembly_Blocked, Pooled) because eviction only happens as a result of errored
			// assembly. Once past assembly, only ChainedDependencyFailed (terminal revert) is relevant.
			Event_ChainedDependencyEvicted: {Handlers: []EventHandler{{
				Transitions: []Transition{{To: State_Evicted}},
			}}},
		},
	},
	State_Pooled: {
		OnTransitionTo: []ActionRule{
			{Action: action_InitializeForNewAssembly},
		},
		Events: map[EventType]EventHandlers{
			Event_Selected: {Handlers: []EventHandler{{
				Actions: []ActionRule{
					// We notify dependents at the point of selection, since the outcome of assembly is irrelevant
					// to ensuring ordering for first assembly. It is relevant for chained dependencies but the
					{Action: action_NotifyDependentsOfSelection},
					{Action: action_RemovePreAssemblePrereqOf},
				},
				Transitions: []Transition{
					{
						To: State_Assembling,
					}},
			}}},
			// Dependency reset and revert events when we're in pooled state are always from chained dependencies.
			// Preassembly dependencies have been cleared by the time we get to pooled state.
			// Postassembly are not established until after we've assembled.
			Event_DependencyReset: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{
					{
						To: State_PreAssembly_Blocked,
					},
				},
			}}},
			Event_DependencyConfirmedReverted: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions:   []ActionRule{{Action: action_MarkChainedDependencyUnassembled}},
				Transitions: []Transition{
					{
						To: State_PreAssembly_Blocked,
					},
				},
			}}},
			Event_ChainedDependencyFailed: {Handlers: []EventHandler{{
				Actions:     []ActionRule{{Action: action_FinalizeOnChainedDependencyFailure}},
				Transitions: []Transition{{To: State_Reverted}},
			}}},
			Event_ChainedDependencyEvicted: {Handlers: []EventHandler{{
				Transitions: []Transition{{To: State_Evicted}},
			}}},
		},
	},
	State_Assembling: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_SendAssembleRequest},
		},
		Events: map[EventType]EventHandlers{
			Event_Assemble_Success: {Handlers: []EventHandler{{
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
					},
					{
						To: State_Blocked,
						If: statemachine.GuardAnd(guard_AttestationPlanFulfilled, guard_HasDependenciesNotReady),
					},
				},
			}}},
			Event_RequestTimeoutInterval: {Handlers: []EventHandler{{
				Actions: []ActionRule{{
					Action: action_NudgeAssembleRequest,
				}},
			}}},
			Event_StateTimeoutInterval: {Handlers: []EventHandler{{
				Transitions: []Transition{
					{
						To:      State_Pooled,
						Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
					},
				},
			}}},
			Event_Assemble_Cancelled: {Handlers: []EventHandler{{
				Transitions: []Transition{
					{
						To:      State_Pooled,
						Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
					},
				},
			}}},
			// Originator has informed us that no longer thinks we are the active coordinator for this transaction.
			// Evict the transaction so it is cleaned up; State_Evicted.OnTransitionTo handles
			// cascading eviction of dependents via action_CascadeChainedDependencyEviction.
			Event_NotActiveCoordinator: {Handlers: []EventHandler{{
				Transitions: []Transition{{
					To: State_Evicted,
				}},
			}}},
			Event_Assemble_Revert_Response: {Handlers: []EventHandler{{
				Validator: validator_MatchesPendingAssembleRequest,
				Actions:   []ActionRule{{Action: action_AssembleRevertResponse}},
				Transitions: []Transition{{
					To: State_Reverted,
				}},
			}}},
			Event_Assemble_Error_Response: {Handlers: []EventHandler{{
				Validator: validator_MatchesPendingAssembleRequest,
				Actions:   []ActionRule{{Action: action_AssembleError}},
				Transitions: []Transition{
					{
						If:      guard_CanRetryErroredAssemble,
						To:      State_Pooled,
						Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
					},
					{
						If: statemachine.GuardNot(guard_CanRetryErroredAssemble),
						To: State_Evicted,
					},
				},
			}}},
			// Handle response from originator indicating it doesn't recognize this transaction.
			// The most likely cause is that the transaction reached a terminal state (e.g., reverted
			// during assembly) but the response was lost, and the transaction has since been removed
			// from memory on the originator after cleanup. The coordinator should clean up this transaction.
			Event_TransactionUnknownByOriginator: {Handlers: []EventHandler{{
				Transitions: []Transition{{
					To:      State_Final,
					Actions: []ActionRule{{Action: action_FinalizeAsUnknownByOriginator}},
				}},
			}}},
			// A dependency resetting or reverting while we are assembling must be a chained dependency
			// (post-assembly dependencies don't exist yet).
			// The dependency is now unassembled, so we always go to PreAssembly_Blocked.
			Event_DependencyReset: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_DependencyConfirmedReverted: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_ChainedDependencyFailed: {Handlers: []EventHandler{{
				Actions:     []ActionRule{{Action: action_FinalizeOnChainedDependencyFailure}},
				Transitions: []Transition{{To: State_Reverted}},
			}}},
			Event_ChainedDependencyEvicted: {Handlers: []EventHandler{{
				Transitions: []Transition{{To: State_Evicted}},
			}}},
		},
	},
	State_Endorsement_Gathering: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_SendEndorsementRequests},
		},
		Events: map[EventType]EventHandlers{
			Event_Endorsed: {Handlers: []EventHandler{{
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
			}}},
			Event_EndorsedRejected: {Handlers: []EventHandler{{
				Transitions: []Transition{
					{
						To:      State_Pooled,
						Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
					},
				},
			}}},
			Event_RequestTimeoutInterval: {Handlers: []EventHandler{{
				Actions: []ActionRule{{
					Action: action_NudgeEndorsementRequests,
				}},
			}}},
			Event_StateTimeoutInterval: {Handlers: []EventHandler{{
				Transitions: []Transition{
					{
						To:      State_Pooled,
						Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
					},
				},
			}}},
			// When we see a dependency reset or revert while in Endorsement_Gathering:
			// - A transaction with a chained dependency will always go to PreAssembly_Blocked as its
			// chained dependency is now unassembled.
			// - A trasanction without a chained dependency will always go to Pooled as its post assembly
			// dependencies are now cleared, and it too far along to have preassembly dependencies.
			Event_DependencyReset: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}, {
				Validator: statemachine.ValidatorNot(validator_IsChainedDependency),
				Transitions: []Transition{{
					To:      State_Pooled,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_DependencyConfirmedReverted: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}, {
				Validator: statemachine.ValidatorNot(validator_IsChainedDependency),
				Transitions: []Transition{{
					To:      State_Pooled,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_ChainedDependencyFailed: {Handlers: []EventHandler{{
				Actions:     []ActionRule{{Action: action_FinalizeOnChainedDependencyFailure}},
				Transitions: []Transition{{To: State_Reverted}},
			}}},
		},
	},
	State_Blocked: {
		Events: map[EventType]EventHandlers{
			Event_DependencyReady: {Handlers: []EventHandler{{
				Actions: []ActionRule{
					{
						Action: action_UpdateSigningIdentity,
						If:     statemachine.GuardNot(guard_HasSigner),
					}},
				Transitions: []Transition{{
					To: State_Confirming_Dispatchable,
					If: statemachine.GuardNot(guard_HasDependenciesNotReady),
				}},
			}}},
			// When we see a dependency reset or revert while in Blocked:
			// - A transaction with a chained dependency will always go to PreAssembly_Blocked as its
			// chained dependency is now unassembled.
			// - A trasanction without a chained dependency will always go to Pooled as its post assembly
			// dependencies are now cleared, and it too far along to have preassembly dependencies.
			Event_DependencyReset: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}, {
				Validator: statemachine.ValidatorNot(validator_IsChainedDependency),
				Transitions: []Transition{{
					To:      State_Pooled,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_DependencyConfirmedReverted: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}, {
				Validator: statemachine.ValidatorNot(validator_IsChainedDependency),
				Transitions: []Transition{{
					To:      State_Pooled,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_ChainedDependencyFailed: {Handlers: []EventHandler{{
				Actions:     []ActionRule{{Action: action_FinalizeOnChainedDependencyFailure}},
				Transitions: []Transition{{To: State_Reverted}},
			}}},
		},
	},
	State_Confirming_Dispatchable: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_SendPreDispatchRequest},
		},
		Events: map[EventType]EventHandlers{
			Event_DispatchRequestApproved: {Handlers: []EventHandler{{
				Validator: validator_MatchesPendingPreDispatchRequest,
				Actions:   []ActionRule{{Action: action_DispatchRequestApproved}},
				Transitions: []Transition{
					{
						To: State_Ready_For_Dispatch,
					}},
			}}},
			Event_DispatchRequestRejected: {Handlers: []EventHandler{{
				Validator: validator_MatchesPendingPreDispatchRequest,
				Actions:   []ActionRule{{Action: action_DispatchRequestRejected}},
				Transitions: []Transition{
					{
						To:      State_Pooled,
						Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
					},
				},
			}}},
			// Originator has informed us that no longer thinks we are the active coordinator for this transaction.
			// Evict the transaction so it is cleaned up; State_Evicted.OnTransitionTo handles
			// cascading eviction of dependents via action_CascadeChainedDependencyEviction.
			Event_NotActiveCoordinator: {Handlers: []EventHandler{{
				Transitions: []Transition{{
					To: State_Evicted,
				}},
			}}},
			Event_RequestTimeoutInterval: {Handlers: []EventHandler{{
				Actions: []ActionRule{{
					Action: action_NudgePreDispatchRequest,
				}},
			}}},
			Event_StateTimeoutInterval: {Handlers: []EventHandler{{
				Transitions: []Transition{
					{
						To: State_Pooled,
						Actions: []ActionRule{
							{Action: action_DispatchRequestRejected},
							{Action: action_NotifyDependentsOfReset},
						},
					},
				},
			}}},
			// When we see a dependency reset or revert while in Confirming_Dispatchable:
			// - A transaction with a chained dependency will always go to PreAssembly_Blocked as its
			// chained dependency is now unassembled.
			// - A trasanction without a chained dependency will always go to Pooled as its post assembly
			// dependencies are now cleared, and it too far along to have preassembly dependencies.
			Event_DependencyReset: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}, {
				Validator: statemachine.ValidatorNot(validator_IsChainedDependency),
				Transitions: []Transition{{
					To:      State_Pooled,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_DependencyConfirmedReverted: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}, {
				Validator: statemachine.ValidatorNot(validator_IsChainedDependency),
				Transitions: []Transition{{
					To:      State_Pooled,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_ChainedDependencyFailed: {Handlers: []EventHandler{{
				Actions:     []ActionRule{{Action: action_FinalizeOnChainedDependencyFailure}},
				Transitions: []Transition{{To: State_Reverted}},
			}}},
		},
	},
	State_Ready_For_Dispatch: {
		OnTransitionTo: []ActionRule{
			{Action: action_NotifyDependentsOfReadiness},
		},
		Events: map[EventType]EventHandlers{
			Event_Dispatched: {Handlers: []EventHandler{{
				Actions: []ActionRule{
					{Action: action_AllocateSigningIdentity},
					{Action: action_Dispatch},
				},
				Transitions: []Transition{
					{
						To: State_Dispatched,
					},
				},
			}}},
			// When we see a dependency reset or revert while in Ready_For_Dispatch:
			// - A transaction with a chained dependency will always go to PreAssembly_Blocked as its
			// chained dependency is now unassembled.
			// - A trasanction without a chained dependency will always go to Pooled as its post assembly
			// dependencies are now cleared, and it too far along to have preassembly dependencies.
			Event_DependencyReset: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}, {
				Validator: statemachine.ValidatorNot(validator_IsChainedDependency),
				Transitions: []Transition{{
					To:      State_Pooled,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_DependencyConfirmedReverted: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions: []ActionRule{{
					Action: action_MarkChainedDependencyUnassembled,
				}},
				Transitions: []Transition{{
					To:      State_PreAssembly_Blocked,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}, {
				Validator: statemachine.ValidatorNot(validator_IsChainedDependency),
				Transitions: []Transition{{
					To:      State_Pooled,
					Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
				}},
			}}},
			Event_ChainedDependencyFailed: {Handlers: []EventHandler{{
				Actions:     []ActionRule{{Action: action_FinalizeOnChainedDependencyFailure}},
				Transitions: []Transition{{To: State_Reverted}},
			}}},
		},
	},
	State_Dispatched: {
		OnTransitionTo: []ActionRule{
			{Action: action_NotifyDispatched},
			{Action: action_CleanUpAssemblyPayload},
		},
		Events: map[EventType]EventHandlers{
			Event_Collected: {Handlers: []EventHandler{{
				Actions: []ActionRule{{Action: action_NotifyCollected}},
			}}},
			Event_NonceAllocated: {Handlers: []EventHandler{{
				Actions: []ActionRule{{Action: action_NotifyNonceAllocated}},
			}}},
			Event_Submitted: {Handlers: []EventHandler{{
				Actions: []ActionRule{{Action: action_NotifySubmitted}},
			}}},
			Event_ConfirmedSuccess: {Handlers: []EventHandler{{
				Actions: []ActionRule{
					{Action: action_RecordConfirmationSuccess},
					{Action: action_NotifyOriginatorOfConfirmation},
				},
				Transitions: []Transition{{To: State_Confirmed}},
			}}},
			Event_ConfirmedReverted: {Handlers: []EventHandler{{
				Actions: []ActionRule{
					{
						Action: action_RecordConfirmationRevert,
					},
				},
				Transitions: []Transition{
					{
						If: statemachine.GuardAnd(guard_CanRetryRevert, guard_HasUnassembledDependencies),
						To: State_PreAssembly_Blocked,
						Actions: []ActionRule{
							{Action: action_NotifyOriginatorOfRetryableRevert},
							{Action: action_NotifyDependentsOfReset},
						},
					},
					{
						If: statemachine.GuardAnd(guard_CanRetryRevert, statemachine.GuardNot(guard_HasUnassembledDependencies)),
						To: State_Pooled,
						Actions: []ActionRule{
							{Action: action_NotifyOriginatorOfRetryableRevert},
							{Action: action_NotifyDependentsOfReset},
						},
					},
					{
						If: statemachine.GuardNot(guard_CanRetryRevert),
						To: State_Reverted,
						Actions: []ActionRule{
							{Action: action_RevertTransactionInGrapher},
							{Action: action_NotifyOriginatorOfNonRetryableRevert},
							{Action: action_NotifyDependentsOfRevertedConfirmation},
							{Action: action_FinalizeNonRetryableRevert},
						},
					},
				},
			}}},
			Event_DependencyReset: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Validator: validator_IsChainedDependency,
					Actions:   []ActionRule{{Action: action_MarkChainedDependencyUnassembled}},
				}, {
					Actions: []ActionRule{
						{Action: action_ResetTransactionLocks},
						{Action: action_NotifyDependentsOfReset},
					},
				}},
			},
			// This event will be received if a chained dependency has reverted, we are still waiting for
			// our revert event, and the chained dependency is reassembled in that time.
			Event_DependencySelectedForAssemble: {Handlers: []EventHandler{{
				Validator: validator_IsChainedDependency,
				Actions:   []ActionRule{{Action: action_MarkChainedDependencyAssembled}},
			}}},
			Event_DependencyConfirmedReverted: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Validator: validator_IsChainedDependency,
					Actions:   []ActionRule{{Action: action_MarkChainedDependencyUnassembled}},
				}, {
					Actions: []ActionRule{
						{Action: action_ResetTransactionLocks},
						{Action: action_NotifyDependentsOfReset},
					},
				}},
			},
			Event_ChainedDependencyFailed: {Handlers: []EventHandler{{
				Actions:     []ActionRule{{Action: action_FinalizeOnChainedDependencyFailure}},
				Transitions: []Transition{{To: State_Reverted}},
			}}},
		},
	},
	State_Reverted: {
		OnTransitionTo: []ActionRule{
			{Action: action_ResetTransactionLocks},
			{Action: action_CascadeChainedDependencyFailure},
			{Action: action_NotifyPreAssembleDependentOfTermination},
		},
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatInterval: {Handlers: []EventHandler{{
				Actions: []ActionRule{{Action: action_IncrementHeartbeatIntervalsSinceStateChange}},
				Transitions: []Transition{
					{
						If: guard_HasFinalizingGracePeriodPassedSinceStateChange,
						To: State_Final,
					}},
			}}},
		},
	},
	State_Confirmed: {
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatInterval: {Handlers: []EventHandler{{
				Actions: []ActionRule{
					{
						Action: action_IncrementHeartbeatIntervalsSinceStateChange,
					},
				},
				Transitions: []Transition{
					{
						If: guard_HasFinalizingGracePeriodPassedSinceStateChange,
						To: State_Final,
					}},
			}}},
		},
	},
	State_Final: {
		// Cleanup is handled by the coordinator in response to the state transition event
	},
	State_Evicted: {
		OnTransitionTo: []ActionRule{
			{Action: action_CascadeChainedDependencyEviction},
			{Action: action_NotifyPreAssembleDependentOfTermination},
		},
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
					FromState:     from,
					ToState:       to,
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
	log.L(ctx).Tracef("coordinator transaction %s (%s) increasing heartbeatIntervalsSinceStateChange to %d", t.pt.ID.String(), t.stateMachine.GetCurrentState().String(), t.heartbeatIntervalsSinceStateChange+1)
	t.heartbeatIntervalsSinceStateChange++
	return nil
}
