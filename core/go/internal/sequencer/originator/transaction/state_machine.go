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

type State = common.OriginatorTransactionState

// Note: inline comments on State_* constants are used in auto-generated documentation.
// Keep them accurate and human-readable - see scripts/generate_state_machine_docs.py
const (
	State_Initial               = common.OriginatorTransactionState_Initial               // Transaction state machine created
	State_Pending               = common.OriginatorTransactionState_Pending               // The transaction has not yet been delegated to a coordinator
	State_Delegated             = common.OriginatorTransactionState_Delegated             // The transaction has been sent to the current active coordinator
	State_Assembling            = common.OriginatorTransactionState_Assembling            // The coordinator has sent an assemble request to us and we have not yet sent the assembled transaction back to the coordinator
	State_Endorsement_Gathering = common.OriginatorTransactionState_Endorsement_Gathering // An assemble response has been sent to the active coordinator, who should now be gathering endorsements for the transaction. A dispatch confirmation request is expected in this state.
	State_Prepared              = common.OriginatorTransactionState_Prepared              // We know that the coordinator has got as far as preparing a public transaction for this transaction
	State_Dispatched            = common.OriginatorTransactionState_Dispatched            // The active coordinator that this transaction was delegated to has dispatched the transaction to a public transaction manager for submission to the base ledger
	State_Sequenced             = common.OriginatorTransactionState_Sequenced             // The public transaction manager at the coordinator has allocated a nonce for this transaction's base ledger transaction
	State_Submitted             = common.OriginatorTransactionState_Submitted             // The base ledger transaction has been submitted to the blockchain
	State_Confirmed             = common.OriginatorTransactionState_Confirmed             // The base ledger transaction has been confirmed by the blockchain as successful
	State_Reverted              = common.OriginatorTransactionState_Reverted              // Upon attempting to assemble the transaction, the domain code has determined that the intent is not valid and the transaction is finalized as reverted
	State_Parked                = common.OriginatorTransactionState_Parked                // Upon attempting to assemble the transaction, the domain code has determined that the transaction is not ready to be assembled and it is parked for later processing. Other transactions for the current originator can continue unless they have an explicit dependency on this transaction.
	State_Final                 = common.OriginatorTransactionState_Final                 // Final state for the transaction. Transactions are removed from memory as soon as they enter this state
)

type EventType = common.EventType

const (
	Event_Created                    EventType = iota // Transaction initially received by the originator or has been loaded from the database after a restart / swap-in
	Event_ConfirmedSuccess                            // confirmation received from the blockchain of base ledge transaction successful completion
	Event_ConfirmedReverted                           // confirmation received from the blockchain of base ledge transaction failure
	Event_Delegated                                   // transaction has been delegated to a coordinator
	Event_AssembleRequestReceived                     // coordinator has requested that we assemble the transaction
	Event_AssembleAndSignSuccess                      // we have successfully assembled the transaction and signing module has signed the assembled transaction
	Event_AssembleRevert                              // we have failed to assemble the transaction
	Event_AssemblePark                                // we have parked the transaction
	Event_AssembleError                               // an unexpected error occurred while trying to assemble the transaction
	Event_Dispatched                                  // coordinator has dispatched the transaction to a public transaction manager
	Event_PreDispatchRequestReceived                  // coordinator has requested confirmation that the transaction is OK to be dispatched
	Event_Resumed                                     // Received an RPC call to resume a parked transaction
	Event_NonceAssigned                               // the public transaction manager has assigned a nonce to the transaction
	Event_Submitted                                   // the transaction has been submitted to the blockchain
	Event_Finalize                                    // internal event to trigger transition from terminal states (Confirmed/Reverted) to State_Final for cleanup
)

// Type aliases for the generic statemachine types, specialized for Transaction
type (
	Action           = statemachine.Action[*originatorTransaction]
	Guard            = statemachine.Guard[*originatorTransaction]
	ActionRule       = statemachine.ActionRule[*originatorTransaction]
	Transition       = statemachine.Transition[State, *originatorTransaction]
	Validator        = statemachine.Validator[*originatorTransaction]
	EventHandler     = statemachine.EventHandler[State, *originatorTransaction]
	EventHandlers    = statemachine.EventHandlers[State, *originatorTransaction]
	StateDefinition  = statemachine.StateDefinition[State, *originatorTransaction]
	StateDefinitions = statemachine.StateDefinitions[State, *originatorTransaction]
	StateMachine     = statemachine.StateMachine[State, *originatorTransaction]
)

var stateDefinitionsMap = StateDefinitions{
	State_Initial: {
		Events: map[EventType]EventHandlers{
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_Created: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{
						{
							To: State_Pending,
						},
					},
				}},
			},
		},
	},
	State_Pending: {
		Events: map[EventType]EventHandlers{
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_Delegated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_Delegated}},
					Transitions: []Transition{
						{
							To: State_Delegated,
						},
					},
				}},
			},
		},
	},
	State_Delegated: {
		Events: map[EventType]EventHandlers{
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_Delegated: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Validator: statemachine.ValidatorNot(validator_CoordinatorIsCurrentDelegate),
					Actions:   []ActionRule{{Action: action_ResetDelegationState}},
				}, {
					Actions: []ActionRule{{Action: action_Delegated}},
				}},
			},
			Event_AssembleRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh the cached block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					// Assemble request is not from the current delegate; reject without entering the assembly flow.
					Validator: statemachine.ValidatorNot(validator_AssembleRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendAssembleRejectionNotCurrentDelegate}},
				}, {
					// Block height tolerance exceeded: reject without entering the assembly flow.
					Validator: validator_AssembleBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_SendAssembleBlockHeightRejection}},
				}, {
					// Private state incomplete: reject so the coordinator retries once states have arrived.
					Validator: validator_IsPrivateStateDataPendingForAssembly,
					Actions:   []ActionRule{{Action: action_RejectAssemblyPrivateStateDataPending}},
				}, {
					// All checks pass: assemble and transition.
					Validator: statemachine.ValidatorAnd(
						validator_AssembleRequestFromCurrentDelegate,
						statemachine.ValidatorNot(validator_AssembleBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForAssembly),
					),
					Actions: []ActionRule{{Action: action_AssembleRequestReceived}},
					Transitions: []Transition{
						{
							To: State_Assembling,
						},
					},
				}},
			},
			Event_Dispatched: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_CoordinatorIsCurrentDelegate,
					Actions:   []ActionRule{{Action: action_Dispatched}},
					Transitions: []Transition{
						{
							To: State_Dispatched,
						},
					},
				}},
			},
		},
	},
	State_Assembling: {
		OnTransitionTo: []ActionRule{{Action: action_AssembleAndSign}},
		Events: map[EventType]EventHandlers{
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_Delegated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: statemachine.ValidatorNot(validator_CoordinatorIsCurrentDelegate),
					Actions: []ActionRule{
						{Action: action_Delegated},
						{Action: action_ResetDelegationState},
					},
					Transitions: []Transition{{
						To: State_Delegated,
					}},
				}},
			},
			Event_AssembleAndSignSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_AssembleAndSignSuccessMatchesCurrentRequest,
					Actions:   []ActionRule{{Action: action_AssembleAndSignSuccess}},
					Transitions: []Transition{
						{
							To:      State_Endorsement_Gathering,
							Actions: []ActionRule{{Action: action_SendAssembleSuccessResponse}},
						},
					},
				}},
			},
			Event_AssembleRevert: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_AssembleRevert}},
					Transitions: []Transition{
						{
							To:      State_Reverted,
							Actions: []ActionRule{{Action: action_SendAssembleRevertResponse}},
						},
					},
				}},
			},
			Event_AssemblePark: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_AssemblePark}},
					Transitions: []Transition{
						{
							To:      State_Parked,
							Actions: []ActionRule{{Action: action_SendAssembleParkResponse}},
						},
					},
				}},
			},
			Event_AssembleError: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_AssembleError}},
					Transitions: []Transition{
						{
							// We've been given opportunities by the coordinator to assemble without error. In the future we might insert a failure receipt
							// for such cases, but for now we free up the state machine, allow other transactions to be delegated ahead, and will be allowed
							// to retry on the TX resume interval (i.e. when we re-read from the DB)
							To:      State_Delegated,
							Actions: []ActionRule{{Action: action_SendAssembleError}},
						},
					},
				}},
			},
			Event_AssembleRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh the cached block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					// Assemble request is not from the current delegate; reject without entering the assembly flow.
					Validator: statemachine.ValidatorNot(validator_AssembleRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendAssembleRejectionNotCurrentDelegate}},
				}, {
					// Block height tolerance exceeded: reject without entering the assembly flow.
					Validator: validator_AssembleBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_SendAssembleBlockHeightRejection}},
				}, {
					// Private state incomplete: reject without entering the assembly flow.
					Validator: validator_IsPrivateStateDataPendingForAssembly,
					Actions:   []ActionRule{{Action: action_RejectAssemblyPrivateStateDataPending}},
				}, {
					// All checks pass: assemble and proceed.
					Validator: statemachine.ValidatorAnd(
						validator_AssembleRequestFromCurrentDelegate,
						statemachine.ValidatorNot(validator_AssembleBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForAssembly),
					),
					// For some reason we've been asked to assemble again. We must not have moved to endorsement gathering,
					// reverted, or parked. This could be because of a temporary issue preventing assembly (e.g. we couldn't
					// resolve a remote verifier while it was offline). Assuming this is a new request, action it.
					Actions: []ActionRule{
						{Action: action_AssembleRequestReceived},
						{If: statemachine.GuardNot(guard_AssembleRequestMatchesPreviousResponse), Action: action_AssembleAndSign},
						{If: guard_AssembleRequestMatchesPreviousResponse, Action: action_ResendAssembleSuccessResponse},
					},
					// No transition - we're already in Assembling
				}},
			},
		},
	},
	State_Endorsement_Gathering: {
		Events: map[EventType]EventHandlers{
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_Delegated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: statemachine.ValidatorNot(validator_CoordinatorIsCurrentDelegate),
					Actions: []ActionRule{
						{Action: action_Delegated},
						{Action: action_ResetDelegationState},
					},
					Transitions: []Transition{{
						To: State_Delegated,
					}},
				}},
			},
			Event_AssembleRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh the cached block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					// Assemble request is not from the current delegate; reject without entering the assembly flow.
					Validator: statemachine.ValidatorNot(validator_AssembleRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendAssembleRejectionNotCurrentDelegate}},
				}, {
					// Block height tolerance exceeded: reject without entering the assembly flow.
					Validator: validator_AssembleBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_SendAssembleBlockHeightRejection}},
				}, {
					// Private state incomplete: reject without entering the assembly flow.
					Validator: validator_IsPrivateStateDataPendingForAssembly,
					Actions:   []ActionRule{{Action: action_RejectAssemblyPrivateStateDataPending}},
				}, {
					// All checks pass: assemble and proceed.
					Validator: statemachine.ValidatorAnd(
						validator_AssembleRequestFromCurrentDelegate,
						statemachine.ValidatorNot(validator_AssembleBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForAssembly),
					),
					Actions: []ActionRule{
						{Action: action_AssembleRequestReceived},
						//We thought we had got as far as endorsement but it seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
						{If: guard_AssembleRequestMatchesPreviousResponse, Action: action_ResendAssembleSuccessResponse},
					},
					Transitions: []Transition{{
						//This is different from the previous request. The coordinator must have decided that it was necessary to re-assemble with different available states so we go back to assembling state for a do-over
						If: statemachine.GuardNot(guard_AssembleRequestMatchesPreviousResponse),
						To: State_Assembling,
					}},
				}},
			},
			Event_PreDispatchRequestReceived: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_PreDispatchRequestFromCurrentDelegate,
					Actions:   []ActionRule{{Action: action_PreDispatchRequestReceived}},
					Transitions: []Transition{
						{
							To:      State_Prepared,
							Actions: []ActionRule{{Action: action_SendPreDispatchResponse}},
						},
					},
				}},
			},
		},
	},
	State_Prepared: {
		Events: map[EventType]EventHandlers{
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_Delegated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: statemachine.ValidatorNot(validator_CoordinatorIsCurrentDelegate),
					Actions: []ActionRule{
						{Action: action_Delegated},
						{Action: action_ResetDelegationState},
					},
					Transitions: []Transition{{
						To: State_Delegated,
					}},
				}},
			},
			Event_Dispatched: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_Dispatched}},
					//Note: no validator here although this event may or may not match the most recent dispatch confirmation response.
					// It is possible that we timed out  on Prepared state, delegated to another coordinator, got as far as prepared again and now just learning that
					// the original coordinator has dispatched the transaction.
					// We can't do anything to stop that, but it is interesting to apply the information from event to our state machine because we don't know which of
					// the many base ledger transactions will eventually be confirmed and we are actually not too fussy about which one does
					Transitions: []Transition{
						{
							To: State_Dispatched,
						},
					},
				}},
			},
			Event_AssembleRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh the cached block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					// Assemble request is not from the current delegate; reject without entering the assembly flow.
					Validator: statemachine.ValidatorNot(validator_AssembleRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendAssembleRejectionNotCurrentDelegate}},
				}, {
					// Block height tolerance exceeded: reject without entering the assembly flow.
					Validator: validator_AssembleBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_SendAssembleBlockHeightRejection}},
				}, {
					// Private state incomplete: reject without entering the assembly flow.
					Validator: validator_IsPrivateStateDataPendingForAssembly,
					Actions:   []ActionRule{{Action: action_RejectAssemblyPrivateStateDataPending}},
				}, {
					// All checks pass: assemble and proceed.
					Validator: statemachine.ValidatorAnd(
						validator_AssembleRequestFromCurrentDelegate,
						statemachine.ValidatorNot(validator_AssembleBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForAssembly),
					),
					Actions: []ActionRule{
						{Action: action_AssembleRequestReceived},
						//We thought we had got as far as prepared but it seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
						{If: guard_AssembleRequestMatchesPreviousResponse, Action: action_ResendAssembleSuccessResponse},
					},
					Transitions: []Transition{{
						//This is different from the previous request. The coordinator must have decided that it was necessary to re-assemble with different available states so we go back to assembling state for a do-over
						If: statemachine.GuardNot(guard_AssembleRequestMatchesPreviousResponse),
						To: State_Assembling,
					}},
				}},
			},
			Event_PreDispatchRequestReceived: {
				// This means that we have already sent a dispatch confirmation response and we get another one.
				// 3 possibilities, 1) the response got lost and the same coordinator is retrying -> compare the request idempotency key and or validator_PreDispatchRequestFromCurrentDelegate
				//                  2) There is a coordinator that we previously delegated to, and assembled for, but since assumed had become unavailable and changed to another coordinator, but the first coordinator is somehow limping along and has got as far as endorsing that previously assembled transaction. But we have already chosen our new horse for this transaction so reject.
				//                  3) There is a bug somewhere.  Don't attempt to distinguish between 2 and 3.  Just reject the request and let the coordinator deal with it.
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_PreDispatchRequestFromCurrentDelegate,
					Actions: []ActionRule{
						{Action: action_PreDispatchRequestReceived},
						{Action: action_ResendPreDispatchResponse},
					},
				}, {
					Validator: statemachine.ValidatorNot(validator_PreDispatchRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendPreDispatchRejectionNotCurrentDelegate}},
				}},
			},
		},
	},
	State_Dispatched: {
		//TODO this is modelled as a state that is discrete to sequenced and submitted but it may be more elegant to model those as sub states of dispatch
		// because there is a set of rules that apply to all of them given that it is possible that it all happens so quickly from dispatch -> sequenced -> submitted -> confirmed
		// that we don't have time to see the heartbeat for those intermediate states so all of those states do actually behave like substates
		// the difference between each one is whether we have the signer address, or also the nonce or also the submission hash
		// for now, we simply copy some event handler rules across dispatched , sequenced and submitted
		Events: map[EventType]EventHandlers{
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_ConfirmedReverted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_RecordWillRetry}},
					Transitions: []Transition{
						{
							If: guard_WillRetry,
							To: State_Delegated,
						},
						{
							If: statemachine.GuardNot(guard_WillRetry),
							To: State_Confirmed,
						},
					},
				}},
			},
			Event_Delegated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: statemachine.ValidatorNot(validator_CoordinatorIsCurrentDelegate),
					Actions: []ActionRule{
						{Action: action_Delegated},
						{Action: action_ResetDelegationState},
					},
					Transitions: []Transition{{
						To: State_Delegated,
					}},
				}},
			},
			Event_NonceAssigned: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_CoordinatorIsCurrentDelegate,
					Actions:   []ActionRule{{Action: action_NonceAssigned}},
					Transitions: []Transition{
						{
							To: State_Sequenced,
						},
					},
				}},
			},
			Event_Submitted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_CoordinatorIsCurrentDelegate,
					Actions:   []ActionRule{{Action: action_Submitted}},
					//we can skip past sequenced and go straight to submitted.
					Transitions: []Transition{
						{
							To: State_Submitted,
						},
					},
				}},
			},
			Event_AssembleRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh the cached block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					// Assemble request is not from the current delegate; reject without entering the assembly flow.
					Validator: statemachine.ValidatorNot(validator_AssembleRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendAssembleRejectionNotCurrentDelegate}},
				}, {
					// Block height tolerance exceeded: reject without entering the assembly flow.
					Validator: validator_AssembleBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_SendAssembleBlockHeightRejection}},
				}, {
					// Private state incomplete: reject without entering the assembly flow.
					Validator: validator_IsPrivateStateDataPendingForAssembly,
					Actions:   []ActionRule{{Action: action_RejectAssemblyPrivateStateDataPending}},
				}, {
					// All checks pass: assemble and proceed.
					Validator: statemachine.ValidatorAnd(
						validator_AssembleRequestFromCurrentDelegate,
						statemachine.ValidatorNot(validator_AssembleBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForAssembly),
					),
					// The coordinator must have decided that it was necessary to re-assemble with different available
					// states so we go back to assembling state for another attempt
					Actions: []ActionRule{{Action: action_AssembleRequestReceived}},
					Transitions: []Transition{{
						To: State_Assembling,
					}},
				}},
			},
		},
	},
	State_Sequenced: {
		Events: map[EventType]EventHandlers{
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_ConfirmedReverted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_RecordWillRetry}},
					Transitions: []Transition{
						{
							If: guard_WillRetry,
							To: State_Delegated,
						},
						{
							If: statemachine.GuardNot(guard_WillRetry),
							To: State_Confirmed,
						},
					},
				}},
			},
			Event_Delegated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: statemachine.ValidatorNot(validator_CoordinatorIsCurrentDelegate),
					Actions: []ActionRule{
						{Action: action_Delegated},
						{Action: action_ResetDelegationState},
					},
					Transitions: []Transition{{
						To: State_Delegated,
					}},
				}},
			},
			Event_Submitted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_CoordinatorIsCurrentDelegate,
					Actions:   []ActionRule{{Action: action_Submitted}},
					Transitions: []Transition{
						{
							To: State_Submitted,
						},
					},
				}},
			},
			Event_AssembleRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh the cached block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					// Assemble request is not from the current delegate; reject without entering the assembly flow.
					Validator: statemachine.ValidatorNot(validator_AssembleRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendAssembleRejectionNotCurrentDelegate}},
				}, {
					// Block height tolerance exceeded: reject without entering the assembly flow.
					Validator: validator_AssembleBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_SendAssembleBlockHeightRejection}},
				}, {
					// Private state incomplete: reject without entering the assembly flow.
					Validator: validator_IsPrivateStateDataPendingForAssembly,
					Actions:   []ActionRule{{Action: action_RejectAssemblyPrivateStateDataPending}},
				}, {
					// All checks pass: assemble and proceed.
					Validator: statemachine.ValidatorAnd(
						validator_AssembleRequestFromCurrentDelegate,
						statemachine.ValidatorNot(validator_AssembleBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForAssembly),
					),
					// The coordinator must have decided that it was necessary to re-assemble with different available
					// states so we go back to assembling state for another attempt
					Actions: []ActionRule{{Action: action_AssembleRequestReceived}},
					Transitions: []Transition{{
						To: State_Assembling,
					}},
				}},
			},
		},
	},
	State_Submitted: {
		Events: map[EventType]EventHandlers{
			Event_Submitted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_CoordinatorIsCurrentDelegate,
					Actions:   []ActionRule{{Action: action_Submitted}},
				}},
			}, // continue to handle submitted events in this state in case the submission hash changes
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_ConfirmedReverted: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_RecordWillRetry}},
					Transitions: []Transition{
						{
							If: guard_WillRetry,
							To: State_Delegated,
						},
						{
							If: statemachine.GuardNot(guard_WillRetry),
							To: State_Confirmed,
						},
					},
				}},
			},
			Event_Delegated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: statemachine.ValidatorNot(validator_CoordinatorIsCurrentDelegate),
					Actions: []ActionRule{
						{Action: action_Delegated},
						{Action: action_ResetDelegationState},
					},
					Transitions: []Transition{{
						To: State_Delegated,
					}},
				}},
			},
			// After submission there's a race for us or the coordinator to find out that the base ledger transaction
			// reverted. We need to accomodate the coordinator getting there first and sending a new assemble request
			// before we receive the revert and moved back to delegated.
			Event_AssembleRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh the cached block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					// Assemble request is not from the current delegate; reject without entering the assembly flow.
					Validator: statemachine.ValidatorNot(validator_AssembleRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendAssembleRejectionNotCurrentDelegate}},
				}, {
					// Block height tolerance exceeded: reject without entering the assembly flow.
					Validator: validator_AssembleBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_SendAssembleBlockHeightRejection}},
				}, {
					// Private state incomplete: reject without entering the assembly flow.
					Validator: validator_IsPrivateStateDataPendingForAssembly,
					Actions:   []ActionRule{{Action: action_RejectAssemblyPrivateStateDataPending}},
				}, {
					// Both checks pass: assemble and transition.
					Validator: statemachine.ValidatorAnd(
						validator_AssembleRequestFromCurrentDelegate,
						statemachine.ValidatorNot(validator_AssembleBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForAssembly),
					),
					Actions: []ActionRule{{Action: action_AssembleRequestReceived}},
					Transitions: []Transition{
						{
							To: State_Assembling,
						},
					},
				}},
			},
		},
	},

	State_Parked: {
		Events: map[EventType]EventHandlers{
			Event_ConfirmedSuccess: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				}},
			},
			Event_Delegated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: statemachine.ValidatorNot(validator_CoordinatorIsCurrentDelegate),
					Actions: []ActionRule{
						{Action: action_Delegated},
						{Action: action_ResetDelegationState},
					},
					Transitions: []Transition{{
						To: State_Delegated,
					}},
				}},
			},
			Event_AssembleRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh the cached block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					// Assemble request is not from the current delegate; reject without entering the assembly flow.
					Validator: statemachine.ValidatorNot(validator_AssembleRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendAssembleRejectionNotCurrentDelegate}},
				}, {
					// Block height tolerance exceeded: reject without entering the assembly flow.
					Validator: validator_AssembleBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_SendAssembleBlockHeightRejection}},
				}, {
					// Private state incomplete: reject without entering the assembly flow.
					Validator: validator_IsPrivateStateDataPendingForAssembly,
					Actions:   []ActionRule{{Action: action_RejectAssemblyPrivateStateDataPending}},
				}, {
					// All checks pass: assemble and proceed.
					Validator: statemachine.ValidatorAnd(
						validator_AssembleRequestFromCurrentDelegate,
						statemachine.ValidatorNot(validator_AssembleBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForAssembly),
					),
					Actions: []ActionRule{
						{
							Action: action_AssembleRequestReceived,
						},
						{
							//it seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
							If:     guard_AssembleRequestMatchesPreviousResponse,
							Action: action_ResendAssembleParkResponse,
						}},
				}},
			},
			Event_Resumed: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Pending,
					}},
				}},
			},
		},
	},
	State_Confirmed: {
		OnTransitionTo: []ActionRule{{Action: action_QueueFinalizeEvent}},
		Events: map[EventType]EventHandlers{
			Event_Finalize: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Final,
					}},
				}},
			},
		},
	},
	State_Reverted: {
		OnTransitionTo: []ActionRule{{Action: action_QueueFinalizeEvent}},
		Events: map[EventType]EventHandlers{
			Event_Finalize: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Transitions: []Transition{{
						To: State_Final,
					}},
				}},
			},
			Event_AssembleRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh the cached block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					// Assemble request is not from the current delegate; reject without entering the assembly flow.
					Validator: statemachine.ValidatorNot(validator_AssembleRequestFromCurrentDelegate),
					Actions:   []ActionRule{{Action: action_SendAssembleRejectionNotCurrentDelegate}},
				}, {
					// Block height tolerance exceeded: reject without entering the assembly flow.
					Validator: validator_AssembleBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_SendAssembleBlockHeightRejection}},
				}, {
					// Private state incomplete: reject without entering the assembly flow.
					Validator: validator_IsPrivateStateDataPendingForAssembly,
					Actions:   []ActionRule{{Action: action_RejectAssemblyPrivateStateDataPending}},
				}, {
					// All checks pass: assemble and proceed.
					Validator: statemachine.ValidatorAnd(
						validator_AssembleRequestFromCurrentDelegate,
						statemachine.ValidatorNot(validator_AssembleBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForAssembly),
					),
					Actions: []ActionRule{
						{Action: action_AssembleRequestReceived},
						{
							// It seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
							// There is only a narrow window of time that this can occur before the transaction is cleaned up from memory. If this request is received again,
							// the coordinator will receive a transaction unknown response which will tell it that it can remove the transaction from its memory also.
							If:     guard_AssembleRequestMatchesPreviousResponse,
							Action: action_ResendAssembleRevertResponse,
						}},
				}},
			},
		},
	},
	State_Final: {
		// Cleanup is driven by the originator when it receives common.TransactionStateTransitionEvent with To==State_Final
	},
}

func (t *originatorTransaction) initializeStateMachine(initialState State) {
	t.stateMachine = statemachine.NewStateMachine(initialState, stateDefinitionsMap,
		fmt.Sprintf("orig-tx-%s", t.pt.ID.String()[0:8]),
		statemachine.WithTransitionCallback(func(ctx context.Context, t *originatorTransaction, from, to State, event common.Event) {
			if t.queueEventForOriginator != nil {
				t.queueEventForOriginator(ctx, &common.TransactionStateTransitionEvent[State]{
					BaseEvent:     common.BaseEvent{EventTime: time.Now()},
					TransactionID: t.pt.ID,
					FromState:     from,
					ToState:       to,
				})
			}
		}),
	)
}

func (t *originatorTransaction) HandleEvent(ctx context.Context, event common.Event) error {
	// Adding the log field here means every function called by the transaction state machine will have the txID field
	// in addition to the fields of the parent context
	txCtx := log.WithLogField(ctx, "txID", t.pt.ID.String())
	return t.stateMachine.ProcessEvent(txCtx, t, event)
}
