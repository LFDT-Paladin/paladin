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

package originator

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/google/uuid"
)

type State int
type EventType = common.EventType

const (
	State_Idle      State = iota //Not acting as a originator and not aware of any active coordinators
	State_Observing              //Not acting as a originator but aware of a node (which may be the same node) acting as a coordinator
	State_Sending                //Has some transactions that have been sent to a coordinator but not yet confirmed TODO should this be named State_Monitoring or State_Delegated or even State_Sent.  Sending sounds like it is in the process of sending the request message.
)

const (
	Event_HeartbeatInterval                EventType = iota + 300 // the heartbeat interval has passed since the last time a heartbeat was received or the last time this event was received
	Event_HeartbeatReceived                                       // a heartbeat message was received from the current active coordinator
	Event_TransactionCreated                                      // a new transaction has been created and is ready to be sent to the coordinator TODO maybe name something like Intent created?
	Event_TransactionConfirmed                                    // a transaction, that was send by this originator, has been confirmed on the base ledger
	Event_NewBlock                                                // a new block has been mined on the base ledger
	Event_Base_Ledger_Transaction_Reverted                        // A transaction has moved from the dispatched to pending state because it was reverted on the base ledger
	Event_Delegate_Timeout                                        // a regular interval to re-delegate transactions that have been delegated but not yet confirmed
)

// Type aliases for the generic state machine types
type (
	Action              = statemachine.Action[*originator]
	Guard               = statemachine.Guard[*originator]
	Validator           = statemachine.Validator[*originator]
	StateUpdate         = statemachine.StateUpdate[*originator]
	Transition          = statemachine.Transition[State, *originator]
	ActionRule          = statemachine.ActionRule[*originator]
	EventHandler        = statemachine.EventHandler[State, *originator]
	StateDefinition     = statemachine.StateDefinition[State, *originator]
	TransitionToHandler = statemachine.TransitionToHandler[*originator]
)

// buildStateDefinitions returns the state machine configuration for the originator
func buildStateDefinitions() statemachine.StateMachineConfig[State, *originator] {
	return statemachine.StateMachineConfig[State, *originator]{
		Definitions: map[State]StateDefinition{
			State_Idle: {
				Events: map[EventType]EventHandler{
					Event_HeartbeatReceived: {
						StateUpdates: []StateUpdate{stateupdate_HeartbeatReceived},
						Transitions: []Transition{{
							To: State_Observing,
						}},
					},
					Event_TransactionCreated: {
						StateUpdates: []StateUpdate{stateupdate_TransactionCreated},
						Validator:    validator_TransactionDoesNotExist,
						Transitions: []Transition{{
							To: State_Sending,
							On: action_SendDelegationRequest,
						}},
					},
				},
			},
			State_Observing: {
				Events: map[EventType]EventHandler{
					Event_HeartbeatInterval: {
						Transitions: []Transition{{
							To: State_Idle,
							If: guard_HeartbeatThresholdExceeded,
						}},
					},
					Event_TransactionCreated: {
						StateUpdates: []StateUpdate{stateupdate_TransactionCreated},
						Validator:    validator_TransactionDoesNotExist,
						Transitions: []Transition{{
							To: State_Sending,
							On: action_SendDelegationRequest,
						}},
					},
					Event_NewBlock: {},
					Event_HeartbeatReceived: {
						StateUpdates: []StateUpdate{stateupdate_HeartbeatReceived},
					},
				},
			},
			State_Sending: {
				Events: map[EventType]EventHandler{
					Event_TransactionConfirmed: {
						StateUpdates: []StateUpdate{stateupdate_TransactionConfirmed},
						Transitions: []Transition{{
							To: State_Observing,
							If: statemachine.Not(guard_HasUnconfirmedTransactions),
						}},
					},
					Event_TransactionCreated: {
						StateUpdates: []StateUpdate{stateupdate_TransactionCreated},
						Validator:    validator_TransactionDoesNotExist,
						Actions: []ActionRule{{
							Action: action_SendDelegationRequest,
						}},
					},
					Event_NewBlock: {},
					Event_HeartbeatReceived: {
						StateUpdates: []StateUpdate{stateupdate_HeartbeatReceived},
						Actions: []ActionRule{{
							If:     guard_HasDroppedTransactions,
							Action: action_SendDroppedTXDelegationRequest,
						}},
					},
					Event_Base_Ledger_Transaction_Reverted: {
						Actions: []ActionRule{{
							Action: action_SendDelegationRequest, //TODO Is this redundant?, coordinator should retry this unless it has dropped the transaction and we already handle the dropped case
						}},
					},
					Event_Delegate_Timeout: {
						Actions: []ActionRule{{
							Action: action_ResendTimedOutDelegationRequest, // Periodically re-delegate transactions that have reached their delegate timeout
						}},
					},
				},
			},
		},
		OnTransition: func(ctx context.Context, o *originator, from, to State, event common.Event) {
			log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf("orig     | %s   | %T | %s -> %s", o.contractAddress.String()[0:8], event, from.String(), to.String())
		},
	}
}

func (o *originator) InitializeStateMachine(ctx context.Context, initialState State) {
	smConfig := buildStateDefinitions()

	elConfig := statemachine.EventLoopConfig[State, *originator]{
		BufferSize: 50, // TODO >1 only required for sqlite coarse-grained locks. Should this be DB-dependent?
		OnEventReceived: func(ctx context.Context, o *originator, event common.Event) (bool, error) {
			log.L(ctx).Debugf("originator handling new event %s (contract address %s, node name %s)", event.TypeString(), o.contractAddress, o.nodeName)

			// Transaction events are propagated to the transaction state machines
			if transactionEvent, ok := event.(transaction.Event); ok {
				log.L(ctx).Debugf("originator propagating transaction event %s to transaction: %s", transactionEvent.TypeString(), transactionEvent.GetTransactionID().String())
				return true, o.propagateEventToTransaction(ctx, transactionEvent)
			}

			// Return false to let the state machine process originator-level events
			return false, nil
		},
	}

	name := "originator[" + o.contractAddress.String()[0:8] + "]"
	o.stateMachine = statemachine.NewEventLoopStateMachine(ctx, name, smConfig, elConfig, initialState, o)
}

// QueueEvent queues a state machine event for the event loop to process.
// Should be called by most Paladin components to ensure memory integrity of
// sequencer state machine and transactions.
func (o *originator) QueueEvent(ctx context.Context, event common.Event) {
	o.stateMachine.QueueEvent(ctx, event)
}

func (o *originator) SetActiveCoordinator(ctx context.Context, coordinator string) error {
	if coordinator == "" {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot set active coordinator to an empty string")
	}
	o.activeCoordinatorNode = coordinator
	log.L(ctx).Debugf("initial active coordinator set to %s", o.activeCoordinatorNode)
	return nil
}

func (o *originator) GetCurrentCoordinator() string {
	return o.activeCoordinatorNode
}

func (o *originator) GetTxStatus(ctx context.Context, txID uuid.UUID) (status components.PrivateTxStatus, err error) {
	// MRW TODO - this needs to use a thread safe mechanism similar to coordinator query queue
	if txn, ok := o.transactionsByID[txID]; !ok {
		endorsements := txn.GetEndorsementStatus(ctx)
		return components.PrivateTxStatus{
			TxID:         txID.String(),
			Status:       txn.GetCurrentState().String(),
			LatestEvent:  txn.GetLatestEvent(),
			Endorsements: endorsements,
			Transaction:  txn.PrivateTransaction,
		}, nil
	}
	return components.PrivateTxStatus{
		TxID:   txID.String(),
		Status: "unknown",
	}, nil
}

func (s State) String() string {
	switch s {
	case State_Idle:
		return "Idle"
	case State_Observing:
		return "Observing"
	case State_Sending:
		return "Sending"
	}
	return "Unknown"
}
