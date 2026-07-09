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
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
)

type State = common.CoordinatorTransactionState

const (
	State_Initial                 = common.CoordinatorTransactionState_Initial                 // Transaction state machine has been created
	State_Pooled                  = common.CoordinatorTransactionState_Pooled                  // The transaction is waiting in the pool to be selected and sent for assembly to the its originator
	State_PreAssembly_Blocked     = common.CoordinatorTransactionState_PreAssembly_Blocked     // The transaction cannot yet be put in the pool to be selected for assembly because a dependency must be assembled first
	State_Assembling              = common.CoordinatorTransactionState_Assembling              // An assemble request has been sent to the originator and we are waiting for the response
	State_Signing                 = common.CoordinatorTransactionState_Signing                 // The transaction has been assembled with a SIGN requirement; we are passively waiting for the originator to push the signature(s)
	State_Reverted                = common.CoordinatorTransactionState_Reverted                // The transaction has been reverted, either at assembly time by the originator or on the base ledger
	State_Endorsement_Gathering   = common.CoordinatorTransactionState_Endorsement_Gathering   // The transaction has been successfully assembled and endorsement requests have been sent
	State_Blocked                 = common.CoordinatorTransactionState_Blocked                 // All endorsements have been received but the transaction cannot proceed due to dependencies not being ready for dispatch
	State_Confirming_Dispatchable = common.CoordinatorTransactionState_Confirming_Dispatchable // The transaction has been endorsed. Confirmation from the originator is required before the transaction can be dispatched. The originator may still request not to proceed at this point.
	State_Ready_For_Dispatch      = common.CoordinatorTransactionState_Ready_For_Dispatch      // Dispatch confirmation has been received from the originator and the transaction is waiting to be collected by the dispatch goroutine
	State_Dispatched              = common.CoordinatorTransactionState_Dispatched              // Collected by the dispatcher thread and submitted by the public TX manager to the base ledger
	State_Confirmed               = common.CoordinatorTransactionState_Confirmed               // The transaction has been confirmed on the base ledger. It will remain in this state for a number heartbeat intervals before moving to State_Final to removed from memory.
	State_Final                   = common.CoordinatorTransactionState_Final                   // The transaction will be removed from memory upon entry to this state
	State_Evicted                 = common.CoordinatorTransactionState_Evicted                 // A problematic transaction is being evicted. Transactions are removed from memory upon entry to this this state. Distinct from State_Final because it might just be used for memory or in-flight slot management
)

type EventType = common.EventType

const (
	Event_Delegated                       EventType = iota + common.Event_HeartbeatInterval + 1 // Transaction initially received by the coordinator.  Might seem redundant explicitly modeling this as an event rather than putting this logic into the constructor, but it is useful to make the initial state transition rules explicit in the state machine definitions
	Event_DependencySelectedForAssemble                                                         // the transaction delegated immediately before the transaction from the same originator has been selected for assembly
	Event_Selected                                                                              // selected from the pool as the next transaction to be assembled
	Event_AssembleRequestSent                                                                   // assemble request sent to the assembler
	Event_AssembleSuccess                                                                       // assembler returned a successful assembly
	Event_AssembleRevert                                                                        // assembler returned a revert (domain said assembly is invalid)
	Event_AssembleError                                                                         // assembler returned an unexpected error
	Event_AssembleRequestRejected                                                               // originator rejected the assemble request (e.g. block height tolerance exceeded)
	Event_AssembleCancelled                                                                     // the assemble attempt has been cancelled
	Event_Signed                                                                                // a signature was pushed by the originator for a SIGN attestation
	Event_SignError                                                                             // the originator failed to sign a SIGN attestation and pushed an error
	Event_Endorsed                                                                              // endorsement received from one endorser
	Event_EndorseRevert                                                                         // endorser responded that the assembly is invalid (domain REVERT)
	Event_EndorseError                                                                          // endorser encountered an unexpected error processing the request
	Event_EndorseRequestRejected                                                                // endorser rejected the request before processing (e.g. block height tolerance)
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
	Event_PreDispatchRequestRejected                                                            // originator has rejected the pre-dispatch request (NOT_CURRENT_DELEGATE or TRANSACTION_UNKNOWN)
	Event_ChainedDependencyFailed                                                               // a chained (same-coordinator) dependency has been permanently finalized as failed
	Event_ChainedDependencyEvicted                                                              // a chained (same-coordinator) dependency has been evicted (e.g. assembly failure threshold exceeded)
	Event_PreAssembleDependencyTerminated                                                       // the pre-assemble (FIFO ordering) predecessor has reached a terminal state
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
			Event_Delegated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{
						{
							To: State_Reverted,
							If: guard_HasRevertedChainedDependency,
							Actions: []ActionRule{
								{Action: action_FinalizeOnRevertedChainedDependencyAtCreation},
								{Action: action_NotifyOriginatorOfChainedDependencyFailureAtCreation},
							},
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
				}},
			},
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
				}},
			},
			// A chained dependency has reset or been confirmed reverted while this transaction
			// is still waiting to be assembled
			Event_DependencyReset: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsChainedDependency,
					Actions:   []ActionRule{{Action: action_MarkChainedDependencyUnassembled}},
					// No state transition — already in the correct waiting state.
				}},
			},
			Event_DependencyConfirmedReverted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsChainedDependency,
					Actions:   []ActionRule{{Action: action_MarkChainedDependencyUnassembled}},
					// No state transition — ChainedDependencyFailed will follow for non-retryable
					// reverts and handle the cascade finalization via the existing handler below.
				}},
			},
			// The pre-assemble predecessor reached a terminal state — sever the FIFO link
			// so this transaction is not stuck waiting forever
			Event_PreAssembleDependencyTerminated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_RemovePreAssembleDependency}},
					Transitions: []Transition{
						{
							To: State_Pooled,
							If: statemachine.GuardNot(guard_HasUnassembledDependencies),
						},
					},
				}},
			},
			Event_ChainedDependencyFailed: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_FinalizeOnChainedDependencyFailure},
						{Action: action_NotifyOriginatorOfChainedDependencyFailure},
					},
					Transitions: []Transition{{To: State_Reverted}},
				}},
			},
			// Event_ChainedDependencyEvicted is only handled in pre-assembly states (Initial,
			// PreAssembly_Blocked, Pooled) because eviction only happens as a result of errored
			// assembly. Once past assembly, only ChainedDependencyFailed (terminal revert) is relevant.
			Event_ChainedDependencyEvicted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{To: State_Evicted}},
				}},
			},
		},
	},
	State_Pooled: {
		OnTransitionTo: []ActionRule{
			{Action: action_InitializeForNewAssembly},
		},
		Events: map[EventType]EventHandlers{
			Event_Selected: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
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
				}},
			},
			// Dependency reset and revert events when we're in pooled state are always from chained dependencies.
			// Preassembly dependencies have been cleared by the time we get to pooled state.
			// Postassembly are not established until after we've assembled.
			Event_DependencyReset: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsChainedDependency,
					Actions: []ActionRule{{
						Action: action_MarkChainedDependencyUnassembled,
					}},
					Transitions: []Transition{
						{
							To: State_PreAssembly_Blocked,
						},
					},
				}},
			},
			Event_DependencyConfirmedReverted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsChainedDependency,
					Actions:   []ActionRule{{Action: action_MarkChainedDependencyUnassembled}},
					Transitions: []Transition{
						{
							To: State_PreAssembly_Blocked,
						},
					},
				}},
			},
			Event_ChainedDependencyFailed: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_FinalizeOnChainedDependencyFailure},
						{Action: action_NotifyOriginatorOfChainedDependencyFailure},
					},
					Transitions: []Transition{{To: State_Reverted}},
				}},
			},
			Event_ChainedDependencyEvicted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{To: State_Evicted}},
				}},
			},
		},
	},
	State_Assembling: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_RefreshBlockHeight},
			{Action: action_SendAssembleRequest},
		},
		Events: map[EventType]EventHandlers{
			Event_AssembleSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_MatchesPendingAssembleRequest,
					Actions:   []ActionRule{{Action: action_AssembleSuccess}},
					Transitions: []Transition{
						{
							// The plan has an unfulfilled SIGN requirement on the originating node: wait passively in
							// State_Signing for the originator to push the signature(s) before proceeding.
							To: State_Signing,
							If: statemachine.GuardNot(guard_SignRequirementsFulfilled),
						},
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
				}},
			},
			// A SignResponse can win the race against its preceding AssembleResponse and arrive while the
			// transaction is still assembling. The SignResponse carries the assembled plan, so apply it and
			// the signature together, then route with the same guards as Event_AssembleSuccess. A later
			// AssembleSuccess for the same request lands in a state with no handler and is dropped harmlessly.
			Event_Signed: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: statemachine.ValidatorAnd(validator_MatchesPendingAssembleRequest, validator_SignedCarriesAssembly),
					Actions:   []ActionRule{{Action: action_AssembleAndSign}},
					Transitions: []Transition{
						{
							To: State_Signing,
							If: statemachine.GuardNot(guard_SignRequirementsFulfilled),
						},
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
				}},
			},
			// A SignError can likewise win the race. Abandon the in-flight assembly and repool or evict
			// against the shared assemble retry budget, mirroring the State_Signing Event_SignError handler.
			Event_SignError: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_MatchesPendingAssembleRequest,
					Actions:   []ActionRule{{Action: action_SignError}},
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
				}},
			},
			Event_RequestTimeoutInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_NudgeAssembleRequest,
					}},
				}},
			},
			Event_StateTimeoutInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{
						{
							To:      State_Pooled,
							Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
						},
					},
				}},
			},
			Event_AssembleCancelled: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{
						{
							To:      State_Pooled,
							Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
						},
					},
				}},
			},
			Event_AssembleRevert: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_MatchesPendingAssembleRequest,
					Actions:   []ActionRule{{Action: action_AssembleRevertResponse}},
					Transitions: []Transition{{
						To: State_Reverted,
					}},
				}},
			},
			Event_AssembleError: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
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
				}},
			},
			Event_AssembleRequestRejected: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// nil Validator — always fires first; logs the reason before any transition
					Actions: []ActionRule{{Action: action_LogAssembleRejection}},
				}, {
					// BLOCK_HEIGHT_TOLERANCE and PRIVATE_STATE_DATA_PENDING are both transient;
					// reset to Pooled so the system retries once the pending state data has arrived
					Validator: validator_IsAssembleRejection(
						engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
						engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING,
					),
					Transitions: []Transition{{
						To:      State_Pooled,
						Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
					}},
				}, {
					// Originator does not recognise this node as the active coordinator; evict.
					Validator: validator_IsAssembleRejection(engineProto.RejectionReason_NOT_CURRENT_DELEGATE),
					Transitions: []Transition{{
						To: State_Evicted,
					}},
				}, {
					// Originator no longer holds this transaction in memory; finalize.
					Validator: validator_IsAssembleRejection(engineProto.RejectionReason_TRANSACTION_UNKNOWN),
					Transitions: []Transition{{
						To:      State_Final,
						Actions: []ActionRule{{Action: action_FinalizeAsUnknownByOriginator}},
					}},
				}},
			},
			// A dependency resetting or reverting while we are assembling must be a chained dependency
			// (post-assembly dependencies don't exist yet).
			// The dependency is now unassembled, so we always go to PreAssembly_Blocked.
			Event_DependencyReset: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsChainedDependency,
					Actions: []ActionRule{{
						Action: action_MarkChainedDependencyUnassembled,
					}},
					Transitions: []Transition{{
						To:      State_PreAssembly_Blocked,
						Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
					}},
				}},
			},
			Event_DependencyConfirmedReverted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsChainedDependency,
					Actions: []ActionRule{{
						Action: action_MarkChainedDependencyUnassembled,
					}},
					Transitions: []Transition{{
						To:      State_PreAssembly_Blocked,
						Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
					}},
				}},
			},
			Event_ChainedDependencyFailed: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_FinalizeOnChainedDependencyFailure},
						{Action: action_NotifyOriginatorOfChainedDependencyFailure},
					},
					Transitions: []Transition{{To: State_Reverted}},
				}},
			},
			Event_ChainedDependencyEvicted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{To: State_Evicted}},
				}},
			},
		},
	},
	State_Signing: {
		// Passive: entry only arms the state timeout. The coordinator sends nothing; it waits for the
		// originator to push the signature(s) it produced for its own assembled plan.
		OnTransitionTo:   []ActionRule{{Action: action_ScheduleStateTimeout}},
		OnTransitionFrom: []ActionRule{{Action: action_ClearStateTimeout}},
		Events: map[EventType]EventHandlers{
			Event_Signed: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_MatchesPendingAssembleRequest,
					Actions: []ActionRule{
						{
							Action: action_Signed,
						},
					},
					// Only leave once every SIGN requirement is fulfilled; a partial signature keeps us here.
					// The onward targets reuse the exact guards from the assemble-success transition.
					Transitions: []Transition{
						{
							To: State_Endorsement_Gathering,
							If: statemachine.GuardAnd(
								guard_SignRequirementsFulfilled,
								statemachine.GuardNot(guard_AttestationPlanFulfilled),
							),
						},
						{
							To: State_Confirming_Dispatchable,
							If: statemachine.GuardAnd(
								guard_SignRequirementsFulfilled,
								guard_AttestationPlanFulfilled,
								statemachine.GuardNot(guard_HasDependenciesNotReady),
							),
						},
						{
							To: State_Blocked,
							If: statemachine.GuardAnd(
								guard_SignRequirementsFulfilled,
								guard_AttestationPlanFulfilled,
								guard_HasDependenciesNotReady,
							),
						},
					},
				}},
			},
			Event_SignError: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_MatchesPendingAssembleRequest,
					Actions:   []ActionRule{{Action: action_SignError}},
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
				}},
			},
			Event_StateTimeoutInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{
						{
							To:      State_Pooled,
							Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
						},
					},
				}},
			},
			// When we see a dependency reset or revert while in Signing:
			// - A transaction with a chained dependency will always go to PreAssembly_Blocked as its
			// chained dependency is now unassembled.
			// - A trasanction without a chained dependency will always go to Pooled as its post assembly
			// dependencies are now cleared, and it too far along to have preassembly dependencies.
			Event_DependencyReset:             dependencyResetHandler,
			Event_DependencyConfirmedReverted: dependencyRevertedHandler,
			Event_ChainedDependencyFailed:     chainedDependencyFailedHandler,
		},
	},
	State_Endorsement_Gathering: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_ComputeEndorseTolerances},
			{Action: action_RefreshBlockHeight},
			{Action: action_SendEndorsementRequests},
		},
		Events: map[EventType]EventHandlers{
			Event_Endorsed: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
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
				}},
			},
			// Domain returned REVERT: endorser rejected the assembly as invalid. Record the
			// failed party (stops nudging them) and check whether remaining non-failed parties
			// can still fulfill the plan. If tolerance exceeded → repool with full request reset;
			// otherwise stay put — the remaining parties may still provide enough endorsements.
			Event_EndorseRevert: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_RecordEndorseFailure}},
					Transitions: []Transition{{
						If: guard_EndorseFailureExceedsTolerance,
						To: State_Pooled,
						Actions: []ActionRule{
							{Action: action_NotifyDependentsOfReset},
							{Action: action_ResetEndorsementRequests},
						},
					}},
				}},
			},
			// Unexpected endorser error. Record the failed party (stops nudging them),
			// then check whether remaining non-failed parties can still fulfill the plan.
			// If tolerance exceeded → repool with full request reset; otherwise stay put
			// and let the retry/nudge mechanism continue with the remaining parties.
			Event_EndorseError: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_RecordEndorseFailure}},
					Transitions: []Transition{{
						If: guard_EndorseFailureExceedsTolerance,
						To: State_Pooled,
						Actions: []ActionRule{
							{Action: action_NotifyDependentsOfReset},
							{Action: action_ResetEndorsementRequests},
						},
					}},
				}},
			},
			// Service-level rejection (block height tolerance). Same as EndorseError:
			// record the failed party, check tolerance, repool+reset only if tolerance exceeded.
			Event_EndorseRequestRejected: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_RecordEndorseFailure}},
					Transitions: []Transition{{
						If: guard_EndorseFailureExceedsTolerance,
						To: State_Pooled,
						Actions: []ActionRule{
							{Action: action_NotifyDependentsOfReset},
							{Action: action_ResetEndorsementRequests},
						},
					}},
				}},
			},
			Event_RequestTimeoutInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_NudgeEndorsementRequests,
					}},
				}},
			},
			Event_StateTimeoutInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{
						{
							To:      State_Pooled,
							Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
						},
					},
				}},
			},
			// When we see a dependency reset or revert while in Endorsement_Gathering:
			// - A transaction with a chained dependency will always go to PreAssembly_Blocked as its
			// chained dependency is now unassembled.
			// - A trasanction without a chained dependency will always go to Pooled as its post assembly
			// dependencies are now cleared, and it too far along to have preassembly dependencies.
			Event_DependencyReset:             dependencyResetHandler,
			Event_DependencyConfirmedReverted: dependencyRevertedHandler,
			Event_ChainedDependencyFailed:     chainedDependencyFailedHandler,
		},
	},
	State_Blocked: {
		Events: map[EventType]EventHandlers{
			Event_DependencyReady: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{
							Action: action_UpdateSigningIdentity,
							If:     statemachine.GuardNot(guard_HasSigner),
						}},
					Transitions: []Transition{{
						To: State_Confirming_Dispatchable,
						If: statemachine.GuardNot(guard_HasDependenciesNotReady),
					}},
				}},
			},
			// When we see a dependency reset or revert while in Blocked:
			// - A transaction with a chained dependency will always go to PreAssembly_Blocked as its
			// chained dependency is now unassembled.
			// - A trasanction without a chained dependency will always go to Pooled as its post assembly
			// dependencies are now cleared, and it too far along to have preassembly dependencies.
			Event_DependencyReset:             dependencyResetHandler,
			Event_DependencyConfirmedReverted: dependencyRevertedHandler,
			Event_ChainedDependencyFailed:     chainedDependencyFailedHandler,
		},
	},
	State_Confirming_Dispatchable: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_SendPreDispatchRequest},
		},
		Events: map[EventType]EventHandlers{
			Event_DispatchRequestApproved: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_MatchesPendingPreDispatchRequest,
					Actions:   []ActionRule{{Action: action_DispatchRequestApproved}},
					Transitions: []Transition{
						{
							To: State_Ready_For_Dispatch,
						}},
				}},
			},
			Event_DispatchRequestRejected: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_MatchesPendingPreDispatchRequest,
					Actions:   []ActionRule{{Action: action_DispatchRequestRejected}},
					Transitions: []Transition{
						{
							To:      State_Pooled,
							Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
						},
					},
				}},
			},
			// Originator has rejected the pre-dispatch request.
			// NOT_CURRENT_DELEGATE: originator no longer tracks us as coordinator; evict.
			// TRANSACTION_UNKNOWN:  originator has cleaned up the transaction; finalize.
			Event_PreDispatchRequestRejected: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsPreDispatchNotCurrentDelegateRejection,
					Transitions: []Transition{{
						To: State_Evicted,
					}},
				}, {
					Validator: validator_IsPreDispatchTransactionUnknownRejection,
					Transitions: []Transition{{
						To:      State_Final,
						Actions: []ActionRule{{Action: action_FinalizeAsUnknownByOriginator}},
					}},
				}},
			},
			Event_RequestTimeoutInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_NudgePreDispatchRequest,
					}},
				}},
			},
			Event_StateTimeoutInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{
						{
							To: State_Pooled,
							Actions: []ActionRule{
								{Action: action_DispatchRequestRejected},
								{Action: action_NotifyDependentsOfReset},
							},
						},
					},
				}},
			},
			// When we see a dependency reset or revert while in Confirming_Dispatchable:
			// - A transaction with a chained dependency will always go to PreAssembly_Blocked as its
			// chained dependency is now unassembled.
			// - A trasanction without a chained dependency will always go to Pooled as its post assembly
			// dependencies are now cleared, and it too far along to have preassembly dependencies.
			Event_DependencyReset:             dependencyResetHandler,
			Event_DependencyConfirmedReverted: dependencyRevertedHandler,
			Event_ChainedDependencyFailed:     chainedDependencyFailedHandler,
		},
	},
	State_Ready_For_Dispatch: {
		OnTransitionTo: []ActionRule{
			{Action: action_NotifyDependentsOfReadiness},
		},
		Events: map[EventType]EventHandlers{
			Event_Dispatched: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_AllocateSigningIdentity},
						{Action: action_Dispatch},
					},
					Transitions: []Transition{
						{
							To: State_Dispatched,
						},
					},
				}},
			},
			// When we see a dependency reset or revert while in Ready_For_Dispatch:
			// - A transaction with a chained dependency will always go to PreAssembly_Blocked as its
			// chained dependency is now unassembled.
			// - A trasanction without a chained dependency will always go to Pooled as its post assembly
			// dependencies are now cleared, and it too far along to have preassembly dependencies.
			Event_DependencyReset:             dependencyResetHandler,
			Event_DependencyConfirmedReverted: dependencyRevertedHandler,
			Event_ChainedDependencyFailed:     chainedDependencyFailedHandler,
		},
	},
	State_Dispatched: {
		OnTransitionTo: []ActionRule{
			{Action: action_NotifyDispatched},
			{Action: action_CleanUpAssemblyPayload},
		},
		Events: map[EventType]EventHandlers{
			Event_Collected: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_NotifyCollected}},
				}},
			},
			Event_NonceAllocated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_NotifyNonceAllocated}},
				}},
			},
			Event_Submitted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_NotifySubmitted}},
				}},
			},
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_RecordConfirmationSuccess},
						{Action: action_NotifyOriginatorOfConfirmation},
					},
					Transitions: []Transition{{To: State_Confirmed}},
				}},
			},
			Event_ConfirmedReverted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
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
				}},
			},
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
			Event_DependencySelectedForAssemble: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsChainedDependency,
					Actions:   []ActionRule{{Action: action_MarkChainedDependencyAssembled}},
				}},
			},
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
			Event_ChainedDependencyFailed: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_FinalizeOnChainedDependencyFailure},
						{Action: action_NotifyOriginatorOfChainedDependencyFailure},
					},
					Transitions: []Transition{{To: State_Reverted}},
				}},
			},
		},
	},
	State_Reverted: {
		OnTransitionTo: []ActionRule{
			{Action: action_ResetTransactionLocks},
			{Action: action_CascadeChainedDependencyFailure},
			{Action: action_NotifyPreAssembleDependentOfTermination},
		},
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_IncrementHeartbeatIntervalsSinceStateChange}},
					Transitions: []Transition{
						{
							If: guard_HasFinalizingGracePeriodPassedSinceStateChange,
							To: State_Final,
						}},
				}},
			},
		},
	},
	State_Confirmed: {
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
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
				}},
			},
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
