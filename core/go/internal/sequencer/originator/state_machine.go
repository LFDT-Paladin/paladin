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
	"fmt"

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

// Type aliases for the generic statemachine types, specialized for originator
type (
	Action           = statemachine.Action[*originator]
	EventAction      = statemachine.EventAction[*originator]
	Guard            = statemachine.Guard[*originator]
	ActionRule       = statemachine.ActionRule[*originator]
	Transition       = statemachine.Transition[State, *originator]
	EventHandler     = statemachine.EventHandler[State, *originator]
	StateDefinition  = statemachine.StateDefinition[State, *originator]
	StateDefinitions = statemachine.StateDefinitions[State, *originator]
)

var stateDefinitionsMap = StateDefinitions{
	State_Idle: {
		Events: map[EventType]EventHandler{
			Event_HeartbeatReceived: {
				OnEvent:     eventAction_HeartbeatReceived,
				Transitions: []Transition{{To: State_Observing}},
			},
			Event_TransactionCreated: {
				Validator:   validator_TransactionDoesNotExist,
				OnEvent:     eventAction_TransactionCreated,
				Transitions: []Transition{{To: State_Sending, On: action_SendDelegationRequest}},
			},
		},
	},
	State_Observing: {
		Events: map[EventType]EventHandler{
			Event_HeartbeatInterval: {
				Transitions: []Transition{{To: State_Idle, If: guard_HeartbeatThresholdExceeded}},
			},
			Event_TransactionCreated: {
				Validator:   validator_TransactionDoesNotExist,
				OnEvent:     eventAction_TransactionCreated,
				Transitions: []Transition{{To: State_Sending, On: action_SendDelegationRequest}},
			},
			Event_NewBlock:          {},
			Event_HeartbeatReceived: {OnEvent: eventAction_HeartbeatReceived},
			common.Event_TransactionStateTransition: {
				OnEvent: eventAction_OriginatorTransactionStateTransition,
			},
		},
	},
	State_Sending: {
		Events: map[EventType]EventHandler{
			Event_TransactionConfirmed: {
				OnEvent:     eventAction_TransactionConfirmed,
				Transitions: []Transition{{To: State_Observing, If: guard_Not(guard_HasUnconfirmedTransactions)}},
			},
			Event_TransactionCreated: {
				Validator: validator_TransactionDoesNotExist,
				OnEvent:   eventAction_TransactionCreated,
				Actions:   []ActionRule{{Action: action_SendDelegationRequest}},
			},
			Event_NewBlock: {},
			Event_HeartbeatReceived: {
				OnEvent: eventAction_HeartbeatReceived,
				Actions: []ActionRule{{If: guard_HasDroppedTransactions, Action: action_SendDroppedTXDelegationRequest}},
			},
			Event_Base_Ledger_Transaction_Reverted: {
				Actions: []ActionRule{{Action: action_SendDelegationRequest}},
			},
			Event_Delegate_Timeout: {
				Actions: []ActionRule{{Action: action_ResendTimedOutDelegationRequest}},
			},
			common.Event_TransactionStateTransition: {
				OnEvent: eventAction_OriginatorTransactionStateTransition,
			},
		},
	},
}

func (o *originator) initializeStateMachineEventLoop(initialState State) {
	o.stateMachineEventLoop = statemachine.NewStateMachineEventLoop(statemachine.StateMachineEventLoopConfig[State, *originator]{
		InitialState:        initialState,
		Definitions:         stateDefinitionsMap,
		Entity:              o,
		EventLoopBufferSize: 50,
		Name:                fmt.Sprintf("originator-%s", o.contractAddress.String()[0:8]),
		TransitionCallback:  o.onStateTransition,
		PreProcess:          o.preProcessEvent,
	})
}

func (o *originator) preProcessEvent(ctx context.Context, entity *originator, event common.Event) (bool, error) {
	if transactionEvent, ok := event.(transaction.Event); ok {
		log.L(ctx).Debugf("Originator propagating transaction event %s to transaction: %s", event.TypeString(), transactionEvent.GetTransactionID().String())
		return true, o.propagateEventToTransaction(ctx, transactionEvent)
	}
	return false, nil
}

func (o *originator) onStateTransition(ctx context.Context, entity *originator, from State, to State, event common.Event) {
	log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf(
		"orig     | %s   | %T | %s -> %s", o.contractAddress.String()[0:8], event, from.String(), to.String())
}

// Event action functions - apply event-specific data to the originator's internal state
func eventAction_HeartbeatReceived(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*HeartbeatReceivedEvent)
	return o.applyHeartbeatReceived(ctx, e)
}

func eventAction_TransactionCreated(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*TransactionCreatedEvent)
	return o.createTransaction(ctx, e.Transaction)
}

func eventAction_TransactionConfirmed(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*TransactionConfirmedEvent)
	return o.confirmTransaction(ctx, e.From, e.Nonce, e.Hash, e.RevertReason)
}

func eventAction_OriginatorTransactionStateTransition(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	switch e.To {
	case transaction.State_Final:
		o.removeTransaction(ctx, e.TransactionID)
	case transaction.State_Confirmed, transaction.State_Reverted:
		o.QueueEvent(ctx, &transaction.FinalizeEvent{
			BaseEvent:     common.BaseEvent{EventTime: e.GetEventTime()},
			TransactionID: e.TransactionID,
		})
	}
	return nil
}

func (o *originator) SetActiveCoordinator(ctx context.Context, coordinator string) error {
	if coordinator == "" {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot set active coordinator to an empty string")
	}
	o.activeCoordinatorMutex.Lock()
	o.activeCoordinatorNode = coordinator
	o.activeCoordinatorMutex.Unlock()
	log.L(ctx).Debugf("initial active coordinator set to %s", coordinator)
	return nil
}

// GetCurrentCoordinator returns the current coordinator using only activeCoordinatorMu (not the main originator lock),
// so it can be called from LoadSequencer while the event loop holds the lock (avoids deadlock).
func (o *originator) GetCurrentCoordinator() string {
	o.activeCoordinatorMutex.RLock()
	defer o.activeCoordinatorMutex.RUnlock()
	return o.activeCoordinatorNode
}

func (o *originator) GetTxStatus(ctx context.Context, txID uuid.UUID) (status components.PrivateTxStatus, err error) {
	o.RLock()
	defer o.RUnlock()
	if txn, ok := o.transactionsByID[txID]; ok {
		return txn.GetStatus(ctx), nil
	}
	return components.PrivateTxStatus{
		TxID:   txID.String(),
		Status: "unknown",
	}, nil
}

func guard_Not(guard Guard) Guard {
	return func(ctx context.Context, o *originator) bool {
		return !guard(ctx, o)
	}
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
