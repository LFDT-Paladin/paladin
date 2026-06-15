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

type State = common.OriginatorState
type EventType = common.EventType

// Note: inline comments on State_* constants are used in auto-generated documentation.
// Keep them accurate and human-readable - see scripts/generate_state_machine_docs.py
const (
	State_Initial   = common.OriginatorState_Initial   // Waiting for initial coordinator selection
	State_Idle      = common.OriginatorState_Idle      // Not acting as an originator and not aware of any active coordinators
	State_Observing = common.OriginatorState_Observing // Not acting as an originator but aware of a node (which may be the same node) acting as a coordinator
	State_Sending   = common.OriginatorState_Sending   // Has some transactions that have been delegated to a coordinator but not yet confirmed
)

const (
	Event_OriginatorCreated         EventType = iota + 300 // fired once by Start to drive the initial coordinator selection
	Event_TransactionCreated                               // a new transaction has been created and is ready to be sent to the coordinator TODO maybe name something like Intent created?
	Event_DelegationRequestRejected                        // pushed by transport_client when a DelegationResponse arrives with Accepted == false
)

// Type aliases for the generic statemachine types, specialized for originator
type (
	Action           = statemachine.Action[*originator]
	Guard            = statemachine.Guard[*originator]
	Validator        = statemachine.Validator[*originator]
	ActionRule       = statemachine.ActionRule[*originator]
	Transition       = statemachine.Transition[State, *originator]
	EventHandler     = statemachine.EventHandler[State, *originator]
	EventHandlers    = statemachine.EventHandlers[State, *originator]
	StateDefinition  = statemachine.StateDefinition[State, *originator]
	StateDefinitions = statemachine.StateDefinitions[State, *originator]
)

var stateDefinitionsMap = StateDefinitions{
	State_Initial: {
		Events: map[EventType]EventHandlers{
			Event_OriginatorCreated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions:     []ActionRule{{Action: action_CalculateCoordinatorPriorities}},
					Transitions: []Transition{{To: State_Idle}},
				}},
			},
		},
	},
	State_Idle: {
		OnTransitionTo: []ActionRule{
			// When entering Idle the last known active coordinator has gone silent.
			// Reset to the highest-priority candidate so the next Sending entry starts fresh.
			{Action: action_ResetToTopPriorityCoordinator},
		},
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_UpdateEndorserCandidatesFromHeartbeat}},
				}, {
					Validator: validator_IsHeartbeatSenderLive,
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinatorFromHeartbeat},
						{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
					},
					Transitions: []Transition{{To: State_Observing}},
				}},
			},
			Event_TransactionCreated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_TransactionDoesNotExist,
					Actions: []ActionRule{
						{Action: action_TransactionCreated},
						// If we are idle we have not been receiving heartbeats from a coordinator so we
						// need to make sure our delegation goes to the current top priority coordinator
						{Action: action_RefreshBlockHeight},
						{Action: action_ResetToTopPriorityCoordinator},
					},
					Transitions: []Transition{{
						To: State_Sending,
					}},
				}},
			},
			common.Event_EndorserNodesDiscovered: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateEndorserCandidates},
						{Action: action_CalculateCoordinatorPriorities},
						{Action: action_ResetToTopPriorityCoordinator},
					},
				}},
			},
			common.Event_TransactionStateTransition: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_OriginatorTransactionStateTransitionToFinal,
					Actions:   []ActionRule{{Action: action_CleanUpTransaction}},
				}, {
					Validator: statemachine.ValidatorOr(
						validator_OriginatorTransactionStateTransitionToConfirmed,
						validator_OriginatorTransactionStateTransitionToReverted,
					),
					Actions: []ActionRule{{Action: action_FinalizeTransaction}},
				}},
			},
		},
	},
	State_Observing: {
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_UpdateEndorserCandidatesFromHeartbeat}},
				}, {
					Validator: validator_IsHeartbeatSenderLive,
					Actions: []ActionRule{
						{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
						{Action: action_UpdateActiveCoordinatorFromHeartbeat},
					},
				}},
			},
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions:     []ActionRule{{Action: action_IncrementHeartbeatIntervalCounts}},
					Transitions: []Transition{{To: State_Idle, If: guard_InactiveGracePeriodExceeded}},
				}},
			},
			Event_TransactionCreated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_TransactionDoesNotExist,
					Actions:   []ActionRule{{Action: action_TransactionCreated}},
					Transitions: []Transition{{
						To: State_Sending,
					}},
				}},
			},
			common.Event_EndorserNodesDiscovered: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateEndorserCandidates},
						{Action: action_CalculateCoordinatorPriorities},
					},
				}},
			},
			common.Event_TransactionStateTransition: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_OriginatorTransactionStateTransitionToFinal,
					Actions:   []ActionRule{{Action: action_CleanUpTransaction}},
				}, {
					Validator: statemachine.ValidatorOr(
						validator_OriginatorTransactionStateTransitionToConfirmed,
						validator_OriginatorTransactionStateTransitionToReverted,
					),
					Actions: []ActionRule{{Action: action_FinalizeTransaction}},
				}},
			},
		},
	},
	State_Sending: {
		OnTransitionTo: []ActionRule{
			// Delegate immediately to the current active coordinator on entering Sending.
			// If the coordinator is still in Elect or Prepared it will accept the delegation
			// and manage the handover itself.
			{Action: action_RefreshBlockHeight},
			{Action: action_SendDelegationRequest},
		},
		Events: map[EventType]EventHandlers{
			Event_TransactionCreated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_TransactionDoesNotExist,
					Actions: []ActionRule{
						{Action: action_TransactionCreated},
						{Action: action_RefreshBlockHeight},
						{Action: action_SendDelegationRequest},
					},
				}},
			},
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_UpdateEndorserCandidatesFromHeartbeat}},
				}, {
					// Process confirmed transactions from every heartbeat regardless of sender state or identity.
					Actions: []ActionRule{{Action: action_ProcessConfirmedTransactions}},
				}, {
					// Higher-priority coordinator announced; redirect and reset liveness timer.
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatSenderLive,
						validator_IsSenderHigherPriorityThanCurrentCoordinator,
					),
					Actions: []ActionRule{
						{Action: action_SwitchActiveCoordinator},
					},
				}, {
					// Any live non-current node becomes coordinator when ours has gone silent.
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatSenderLive,
						statemachine.ValidatorNot(validator_IsFromCurrentCoordinator),
					),
					Actions: []ActionRule{{If: guard_InactiveGracePeriodExceeded, Action: action_SwitchActiveCoordinator}},
				}, {
					// Heartbeat from the (possibly just-elected) current coordinator:
					Validator: validator_IsFromCurrentCoordinator,
					Actions:   []ActionRule{{Action: action_ProcessCurrentCoordinatorHeartbeat}},
				}, {
					// Our coordinator has dropped transactions (or our new coordinator from the earlier action
					// has never heard of our transactions); redelegate everything.
					// No liveness check: a closing coordinator that drops transactions also needs to trigger a redelegate.
					Validator: statemachine.ValidatorAnd(
						validator_IsFromCurrentCoordinator,
						validator_HasDroppedTransactions,
					),
					Actions: []ActionRule{
						{Action: action_RefreshBlockHeight},
						{Action: action_SendDelegationRequest},
					},
				}},
			},
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalCounts},
					// When the active coordinator has been silent too long, failover to the next
					// highest-priority candidate if one is available. Otherwise redelegate to the same node.
					{
						If:     guard_InactiveGracePeriodExceeded,
						Action: action_RefreshBlockHeight,
					},
					{
						If:     guard_InactiveGracePeriodExceeded,
						Action: action_FailoverToNextCoordinator,
					},
					},
				}},
			},
			Event_DelegationRequestRejected: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					// Delegation was rejected because the receiver's block height is too far from ours — log and wait.
					// We will redelegate when we next see a heartbeat not containing our transactions.
					Validator: validator_IsDelegationBlockHeightRejection,
					Actions:   []ActionRule{{Action: action_LogDelegationBlockHeightRejection}},
				}, {
					Validator: validator_IsDelegationNotActiveCoordinatorRejection,
					Actions: []ActionRule{
						{Action: action_HandleDelegationRejected},
						// We always redelegate immediately, regardless of whether the current active coordinator has changed
						{Action: action_RefreshBlockHeight},
						{Action: action_SendDelegationRequest},
					},
				}},
			},
			common.Event_TransactionStateTransition: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_OriginatorTransactionStateTransitionToFinal,
					Actions:   []ActionRule{{Action: action_CleanUpTransaction}},
					Transitions: []Transition{
						{To: State_Observing, If: statemachine.GuardNot(guard_HasTransactions)},
					},
				}, {
					Validator: statemachine.ValidatorOr(
						validator_OriginatorTransactionStateTransitionToConfirmed,
						validator_OriginatorTransactionStateTransitionToReverted,
					),
					Actions: []ActionRule{{Action: action_FinalizeTransaction}},
				}},
			},
			common.Event_EndorserNodesDiscovered: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateEndorserCandidates},
						{Action: action_CalculateCoordinatorPriorities},
					},
				}},
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
