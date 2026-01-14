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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
)

type State int

const (
	State_Initial State = iota // Initial state before anything is calculated
	State_Pending              // Intent for the transaction has been created in the database and has been assigned a unique ID but is not currently known to be being processed by a coordinator
	//TODO currently Pending doesn't really make sense as a state because it is an instantaneous state.  It is the state of the transaction when it is first created and then immediately transitions to Delegated.
	// States only really make sense when the transaction is waiting for something to happen.  We could remove this state and just have the transaction start in the Delegated state
	// However, there may be a need for the originator to only delegate a subset of the transactions ( e.g. maybe there is an absolute ordering requirement and the only way to achieve that is by holding back until the dependency is confirmed)
	// It is also slightly complicated by the fact that the delegation request is sent by an action of the originator state machine because it sends the delegation request for multiple transactions at once.
	// Need a decision point on whether that is done by a) transaction emitting and event that triggers the originator to send the delegation request or b) the transaction state machine action makes a syncronous call to the originator to include that transaction in a new delegation request.
	// NOTE: initially there was a thought that we needed a pending state in case there is no current active coordinator so we can't go straight to delegated.  However, the current model is that we don't actually wait for any response from the coordinator.  We simply send the delegation request and assume that it is delegated at that point.
	// We only resend the request if we don't see the heartbeat.
	// Might need to rethink this and allow for some ack and shorter retry interval to tolerate less reliable networks,
	// Need to make a decision and document it in a README
	State_Delegated             // the transaction has been sent to the current active coordinator
	State_Assembling            // the coordinator has sent an assemble request that we have not replied to yet
	State_Endorsement_Gathering //we have responded to an assemble request and are waiting the coordinator to gather endorsements and send us a dispatch confirmation request
	State_Signing               // we have assembled the transaction and are waiting for the signing module to sign it before we respond to the coordinator with the signed assembled transaction
	State_Prepared              // we know that the coordinator has got as far as preparing a public transaction and we have sent a positive response to a coordinator's dispatch confirmation request but have not yet received a heartbeat that notifies us that the coordinator has dispatched the transaction to a public transaction manager for submission
	State_Dispatched            // the active coordinator that this transaction was delegated to has dispatched the transaction to a public transaction manager for submission
	State_Sequenced             // the transaction has been assigned a nonce by the public transaction manager
	State_Submitted             // the transaction has been submitted to the blockchain
	State_Confirmed             // the public transaction has been confirmed by the blockchain as successful
	State_Reverted              // upon attempting to assemble the transaction, the domain code has determined that the intent is not valid and the transaction is finalized as reverted
	State_Parked                // upon attempting to assemble the transaction, the domain code has determined that the transaction is not ready to be assembled and it is parked for later processing.  All remaining transactions for the current originator can continue - unless they have an explicit dependency on this transaction
	State_Final                 // final state for the transaction. Transactions are removed from memory as soon as they enter this state

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
	Event_CoordinatorChanged                          // the coordinator has changed
	Event_Finalize                                    // internal event to trigger transition from terminal states (Confirmed/Reverted) to State_Final for cleanup
)

// Type aliases for the generic state machine types
type (
	Action          = statemachine.Action[*Transaction]
	Guard           = statemachine.Guard[*Transaction]
	Validator       = statemachine.Validator[*Transaction]
	Transition      = statemachine.Transition[State, *Transaction]
	ActionRule      = statemachine.ActionRule[*Transaction]
	EventHandler    = statemachine.EventHandler[State, *Transaction]
	StateDefinition = statemachine.StateDefinition[State, *Transaction]
)

// buildStateDefinitions returns the state machine configuration for originator transactions
func buildStateDefinitions() statemachine.StateMachineConfig[State, *Transaction] {
	return statemachine.StateMachineConfig[State, *Transaction]{
		Definitions: map[State]StateDefinition{
			State_Initial: {
				Events: map[EventType]EventHandler{
					Event_Created: {
						Transitions: []Transition{
							{
								To: State_Pending,
							},
						},
					},
				},
			},
			State_Pending: {
				Events: map[EventType]EventHandler{
					Event_Delegated: {
						Transitions: []Transition{
							{
								To: State_Delegated,
							},
						},
					},
				},
			},
			State_Delegated: {
				Events: map[EventType]EventHandler{
					Event_AssembleRequestReceived: {
						Validator: validator_AssembleRequestMatches,
						Transitions: []Transition{
							{
								To: State_Assembling,
							},
						},
					},
					Event_CoordinatorChanged: {},
					// If we previously delegated i.e. before a node restart, and the result was a chained transaction, the coordinator doesn't need
					// to go through re-assembly and endorsement if it knows the result is a chained TX. We jump straight back to where we would be
					// if the chained TX had just been created and we had received an Event_Dispatched from the coordinator.
					Event_Dispatched: {
						Transitions: []Transition{
							{
								To: State_Dispatched,
							},
						},
					},
				},
			},
			State_Assembling: {
				OnTransitionTo: action_AssembleAndSign,
				Events: map[EventType]EventHandler{
					Event_AssembleAndSignSuccess: {
						Transitions: []Transition{
							{
								To: State_Endorsement_Gathering,
								On: action_SendAssembleSuccessResponse,
							},
						},
					},
					Event_AssembleRevert: {
						Transitions: []Transition{
							{
								To: State_Reverted,
								On: action_SendAssembleRevertResponse,
							},
						},
					},
					Event_AssemblePark: {
						Transitions: []Transition{
							{
								To: State_Parked,
								On: action_SendAssembleParkResponse,
							},
						},
					},
					Event_CoordinatorChanged: {
						//would be very strange to have missed a bunch of heartbeats and switched coordinators if we recently received an assemble request but it is possible so we need to handle it
						Transitions: []Transition{
							{
								To: State_Delegated,
							},
						},
					},
					Event_AssembleRequestReceived: {
						// For some reason we've been asked to assemble again. We must not have moved to endorsement gathering,
						// reverted, or parked. This could be because of a temporary issue preventing assembly (e.g. we couldn't
						// resolve a remote verifier while it was offline). Assuming this is a new request, action it.
						Validator: validator_AssembleRequestMatches,
						Actions: []ActionRule{
							{
								If:     statemachine.Not(guard_AssembleRequestMatchesPreviousResponse),
								Action: action_AssembleAndSign,
							},
							{
								If:     guard_AssembleRequestMatchesPreviousResponse,
								Action: action_ResendAssembleSuccessResponse,
							},
						},
						// No transition - we're still assembling
					},
				},
			},
			State_Endorsement_Gathering: {
				Events: map[EventType]EventHandler{
					Event_AssembleRequestReceived: {
						Validator: validator_AssembleRequestMatches,
						Actions: []ActionRule{
							{
								//We thought we had got as far as endorsement but it seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
								If:     guard_AssembleRequestMatchesPreviousResponse,
								Action: action_ResendAssembleSuccessResponse,
							}},
						Transitions: []Transition{{
							//This is different from the previous request. The coordinator must have decided that it was necessary to re-assemble with different available states so we go back to assembling state for a do-over
							If: statemachine.Not(guard_AssembleRequestMatchesPreviousResponse),
							To: State_Assembling,
						}},
					},
					Event_CoordinatorChanged: {
						Transitions: []Transition{
							{
								To: State_Delegated,
							},
						},
					},
					Event_PreDispatchRequestReceived: {
						Validator: validator_PreDispatchRequestMatchesAssembledDelegation,
						Transitions: []Transition{
							{
								To: State_Prepared,
								On: action_SendPreDispatchResponse,
							},
						},
					},
				},
			},
			State_Prepared: {
				Events: map[EventType]EventHandler{
					Event_Dispatched: {
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
					},
					Event_PreDispatchRequestReceived: {
						Validator: validator_PreDispatchRequestMatchesAssembledDelegation,
						// This means that we have already sent a dispatch confirmation response and we get another one.
						// 3 possibilities, 1) the response got lost and the same coordinator is retrying -> compare the request idempotency key and or validator_PreDispatchRequestMatchesAssembledDelegation
						//                  2) There is a coordinator that we previously delegated to, and assembled for, but since assumed had become unavailable and changed to another coordinator, but the first coordinator is somehow limping along and has got as far as endorsing that previously assembled transaction. But we have already chosen our new horse for this transaction so reject.
						//                  3) There is a bug somewhere.  Don't attempt to distinguish between 2 and 3.  Just reject the request and let the coordinator deal with it.
						Actions: []ActionRule{
							{
								Action: action_ResendPreDispatchResponse,
							},
						},
					},
					Event_CoordinatorChanged: {
						Transitions: []Transition{
							{
								To: State_Delegated,
							},
						},
						// this is a particularly interesting case because the coordinator has been changed ( most likely because the previous coordinator has stopped sending heartbeats)
						// just as we are about at the point of no return.  We have already sent a dispatch confirmation response and are waiting for the dispatch heartbeat
						// only option is to go with the new coordinator.  Assuming the old coordinator didn't receive the confirmation response, or has went offline before dispatching the transactions to a public transaction manager
						// worst case scenario, it has already dispatched the transaction and the base ledger double intent protection will cause one of the transactions to fail
					},
				},
			},
			State_Dispatched: {
				//TODO this is modelled as a state that is discrete to sequenced and submitted but it may be more elegant to model those as sub states of dispatch
				// because there is a set of rules that apply to all of them given that it is possible that it all happens so quickly from dispatch -> sequenced -> submitted -> confirmed
				// that we don't have time to see the heartbeat for those intermediate states so all of those states do actually behave like substates
				// the difference between each one is whether we have the signer address, or also the nonce or also the submission hash
				// for now, we simply copy some event handler rules across dispatched , sequenced and submitted
				Events: map[EventType]EventHandler{
					Event_ConfirmedSuccess: {
						Transitions: []Transition{{
							To: State_Confirmed,
						}},
					},
					Event_ConfirmedReverted: {
						Transitions: []Transition{{
							To: State_Delegated, //trust coordinator to retry
						}},
					},
					Event_CoordinatorChanged: {
						// coordinator has changed after we have seen the transaction dispatched.
						// we will either see the dispatched transaction confirmed or reverted by the blockchain but that might not be for a long long time
						// the fact that the coordinator has been changed on us means that we have lost contact with the original coordinator.
						// The original coordinator may or may not have lost contact with the base ledger.
						// If so, it may be days or months before it reconnects and managed to submit the transaction.
						// Rather than waiting in hope, we carry on with the new coordinator.  The double intent protection in the base ledger will ensure that only one of the coordinators manages to get the transaction through
						// and the other one will revert.  We just need to make sure that we don't overreact when we see a revert.
						// We _could_ introduce a new state that we transition here to give some time, after realizing the coordinator has gone AWOL in case the transaction has made it to that coordinator's
						// EVM node which is actively trying to get it into a block and we just don't get heartbeats for that.
						// However, by waiting, we would need to delaying other transactions from being delegated and assembled or risk things happening out of order
						// and the only downside of not waiting is that we plough ahead with a new assembly of things that will never get to the base ledger because the txn at the front will cause a double intent
						// so we need to redo them all - which isn't much worse than waiting and then redoing them all. On the other hand, if we plough ahead, there is a chance that new assembly does get to the base ledger
						// and there would have been no point waiting
						Transitions: []Transition{
							{
								To: State_Delegated,
							},
						},
					},
					Event_NonceAssigned: {
						Transitions: []Transition{
							{
								To: State_Sequenced,
							},
						},
					},
					Event_Submitted: {
						//we can skip past sequenced and go straight to submitted.
						Transitions: []Transition{
							{
								To: State_Submitted,
							},
						},
					},
				},
			},
			State_Sequenced: {
				Events: map[EventType]EventHandler{
					Event_ConfirmedSuccess: {
						Transitions: []Transition{{
							To: State_Confirmed,
						}},
					},
					Event_ConfirmedReverted: {
						Transitions: []Transition{{
							To: State_Delegated, //trust coordinator to retry
						}},
					},
					Event_CoordinatorChanged: {
						Transitions: []Transition{
							{
								To: State_Delegated,
							},
						},
					},
					Event_Submitted: {
						Transitions: []Transition{
							{
								To: State_Submitted,
							},
						},
					},
				},
			},
			State_Submitted: {
				Events: map[EventType]EventHandler{
					Event_Submitted: {}, // continue to handle submitted events in this state in case the submission hash changes
					Event_ConfirmedSuccess: {
						Transitions: []Transition{{
							To: State_Confirmed,
						}},
					},
					Event_ConfirmedReverted: {
						Transitions: []Transition{{
							To: State_Delegated, //trust coordinator to retry
						}},
					},
					Event_CoordinatorChanged: {
						Transitions: []Transition{
							{
								To: State_Delegated,
							},
						},
					},
					// After submission there's a race for us or the coordinator to find out that the base ledger transaction
					// reverted. We need to accomodate the coordinator getting there first and sending a new assemble request
					// before we receive the revert and moved back to delegated.
					Event_AssembleRequestReceived: {
						Validator: validator_AssembleRequestMatches,
						Transitions: []Transition{
							{
								To: State_Assembling,
							},
						},
					},
				},
			},

			State_Parked: {
				Events: map[EventType]EventHandler{
					Event_AssembleRequestReceived: {
						Actions: []ActionRule{
							{
								//it seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
								If:     guard_AssembleRequestMatchesPreviousResponse,
								Action: action_ResendAssembleParkResponse,
							}},
					},
					Event_Resumed: {
						Transitions: []Transition{{
							To: State_Pending,
						}},
					},
				},
			},
			State_Confirmed: {
				OnTransitionTo: action_QueueFinalizeEvent,
				Events: map[EventType]EventHandler{
					Event_Finalize: {
						Transitions: []Transition{{
							To: State_Final,
						}},
					},
				},
			},
			State_Reverted: {
				OnTransitionTo: action_QueueFinalizeEvent,
				Events: map[EventType]EventHandler{
					Event_Finalize: {
						Transitions: []Transition{{
							To: State_Final,
						}},
					},
					Event_AssembleRequestReceived: {
						Actions: []ActionRule{
							{
								// It seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
								// There is only a narrow window of time that this can occur before the transaction is cleaned up from memory. If this request is received again,
								// the coordinator will receive a transaction unknown response which will tell it that it can remove the transaction from its memory also.
								If:     guard_AssembleRequestMatchesPreviousResponse,
								Action: action_ResendAssembleRevertResponse,
							}},
					},
				},
			},
			State_Final: {
				OnTransitionTo: action_Cleanup,
			},
		},
		OnTransition: func(ctx context.Context, t *Transaction, from, to State, event common.Event) {
			// Log the transition
			log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf("orig-tx  | %s   | %s | %T | %s -> %s", t.Address.String()[0:8], t.ID.String()[0:8], event, from.String(), to.String())
			// Record metrics
			t.metrics.ObserveSequencerTXStateChange("Originator_"+to.String(), t.stateMachine.TimeSinceStateChange())
		},
	}
}

func (t *Transaction) InitializeStateMachine(initialState State) {
	t.stateMachine = statemachine.NewStateMachine(buildStateDefinitions(), initialState)
}

func (t *Transaction) ProcessEvent(ctx context.Context, event common.Event) error {
	return t.stateMachine.ProcessEvent(ctx, t, event)
}

// applyEvent updates the internal state of the Transaction with information from the event
// This is called before the state machine is evaluated for transitions that may be triggered by the event
// so that any guards on the transition rules can take into account the new internal state of the Transaction
func (t *Transaction) applyEvent(ctx context.Context, _ *Transaction, event common.Event) error {
	var err error
	switch event := event.(type) {
	case Event:
		err = event.ApplyToTransaction(ctx, t)

	default:
		//other events may trigger actions and/or state transitions but not require any internal state to be updated
		log.L(ctx).Debugf("no internal state to apply for event type %T", event)
	}
	return err
}

func (s State) String() string {
	switch s {
	case State_Initial:
		return "State_Initial"
	case State_Pending:
		return "State_Pending"
	case State_Delegated:
		return "State_Delegated"
	case State_Assembling:
		return "State_Assembling"
	case State_Endorsement_Gathering:
		return "State_Endorsement_Gathering"
	case State_Signing:
		return "State_Signing"
	case State_Prepared:
		return "State_Prepared"
	case State_Dispatched:
		return "State_Dispatched"
	case State_Sequenced:
		return "State_Sequenced"
	case State_Submitted:
		return "State_Submitted"
	case State_Confirmed:
		return "State_Confirmed"
	case State_Reverted:
		return "State_Reverted"
	case State_Parked:
		return "State_Parked"
	case State_Final:
		return "State_Final"
	}
	return "Unknown"
}
