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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
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
	Event_HeartbeatReceived        EventType = iota + 300 // a heartbeat message was received from the current active coordinator
	Event_TransactionCreated                              // a new transaction has been created and is ready to be sent to the coordinator TODO maybe name something like Intent created?
	Event_NewBlock                                        // a new block has been mined on the base ledger
	Event_ActiveCoordinatorUpdated                        // a new active coordinator is available
)

// Type aliases for the generic statemachine types, specialized for originator
type (
	Action           = statemachine.Action[*originator]
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
			Event_ActiveCoordinatorUpdated: {
				Actions: []ActionRule{{Action: action_ActiveCoordinatorUpdated}},
			},
			Event_HeartbeatReceived: {
				Actions:     []ActionRule{{Action: action_HeartbeatReceived}},
				Transitions: []Transition{{To: State_Observing}},
			},
			Event_TransactionCreated: {
				Validator: validator_TransactionDoesNotExist,
				Actions:   []ActionRule{{Action: action_TransactionCreated}},
				Transitions: []Transition{{
					To: State_Sending,
				}},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{
					{
						Validator: validator_OriginatorTransactionStateTransitionToFinal,
						Action:    action_CleanUpTransaction,
					},
					{
						Validator: statemachine.ValidatorOr(
							validator_OriginatorTransactionStateTransitionToConfirmed,
							validator_OriginatorTransactionStateTransitionToReverted,
						),
						Action: action_FinalizeTransaction,
					},
				},
			},
		},
	},
	State_Observing: {
		Events: map[EventType]EventHandler{
			Event_ActiveCoordinatorUpdated: {
				Actions: []ActionRule{{Action: action_ActiveCoordinatorUpdated}},
			},
			common.Event_HeartbeatInterval: {
				Actions:     []ActionRule{{Action: action_IncrementHeartbeatIntervalsSinceLastReceive}},
				Transitions: []Transition{{To: State_Idle, If: guard_IdleThresholdExceeded}},
			},
			Event_TransactionCreated: {
				Validator: validator_TransactionDoesNotExist,
				Actions:   []ActionRule{{Action: action_TransactionCreated}},
				Transitions: []Transition{{
					To: State_Sending,
				}},
			},
			Event_HeartbeatReceived: {Actions: []ActionRule{{Action: action_HeartbeatReceived}}},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{
					{
						Validator: validator_OriginatorTransactionStateTransitionToFinal,
						Action:    action_CleanUpTransaction,
					},
					{
						Validator: statemachine.ValidatorOr(
							validator_OriginatorTransactionStateTransitionToConfirmed,
							validator_OriginatorTransactionStateTransitionToReverted,
						),
						Action: action_FinalizeTransaction,
					},
				},
			},
		},
	},
	State_Sending: {
		OnTransitionTo: []ActionRule{
			{Action: action_SendDelegationRequest},
		},
		Events: map[EventType]EventHandler{
			Event_ActiveCoordinatorUpdated: {
				Actions: []ActionRule{
					{Action: action_ActiveCoordinatorUpdated},
					{Action: action_SendDelegationRequest},
				},
			},
			Event_TransactionCreated: {
				Validator: validator_TransactionDoesNotExist,
				Actions: []ActionRule{
					{Action: action_TransactionCreated},
					{Action: action_SendDelegationRequest},
				},
			},
			Event_HeartbeatReceived: {
				Actions: []ActionRule{
					{Action: action_HeartbeatReceived},
					{Action: action_SendDelegationRequest, If: guard_HasDroppedTransactions},
				},
			},
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{
						Action: action_IncrementHeartbeatIntervalsSinceLastReceive,
					},
					{
						// Resend all the delegation requests if we have not seen a heartbeat in a while
						// It could be that no one thinks they are coordinating, so this will nudge the node who
						// we think should be the active coordinator.
						// If we have been seeing heartbeats, the handling for Event_HeartbeatReceived will ensure we
						// are resending delegation requests only if we have transactions that the active coordinator
						// does not know about.
						Action: action_SendDelegationRequest,
						If:     guard_RedelegateThresholdExceeded,
					},
				},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{
					{
						Validator: validator_OriginatorTransactionStateTransitionToFinal,
						Action:    action_CleanUpTransaction,
					},
					{
						Validator: statemachine.ValidatorOr(
							validator_OriginatorTransactionStateTransitionToConfirmed,
							validator_OriginatorTransactionStateTransitionToReverted,
						),
						Action: action_FinalizeTransaction,
					},
				},
				Transitions: []Transition{
					{To: State_Observing, If: statemachine.GuardNot(guard_HasUnconfirmedTransactions)},
				},
			},
		},
	},
}

func (o *originator) initializeStateMachineEventLoop(initialState State, eventQueueSize int, priorityEventQueueSize int) {
	o.stateMachineEventLoop = statemachine.NewStateMachineEventLoop(statemachine.StateMachineEventLoopConfig[State, *originator]{
		InitialState:           initialState,
		Definitions:            stateDefinitionsMap,
		Entity:                 o,
		EventQueueSize:         eventQueueSize,
		PriorityEventQueueSize: priorityEventQueueSize,
		Name:                   fmt.Sprintf("originator-%s", o.contractAddress.String()[0:8]),
		PreProcess:             o.preProcessEvent,
	})
}

func (o *originator) preProcessEvent(ctx context.Context, entity *originator, event common.Event) (bool, error) {
	if transactionEvent, ok := event.(transaction.Event); ok {
		log.L(ctx).Debugf("Originator propagating transaction event %s to transaction: %s", event.TypeString(), transactionEvent.GetTransactionID().String())
		return true, o.propagateEventToTransaction(ctx, transactionEvent)
	}
	return false, nil
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
