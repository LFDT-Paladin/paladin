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

package coordinator

import (
	"context"
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
)

// State represents the coordinator's state
type State int

// EventType is an alias for common.EventType
type EventType = common.EventType

const (
	State_Initial   State = iota // Coordinator created but not yet selected an active coordinator
	State_Idle                   // Not acting as a coordinator and not aware of any other active coordinators
	State_Observing              // Not acting as a coordinator but aware of another node acting as a coordinator
	State_Elect                  // Selected to take over from another coordinator but waiting to see the previous coordinator's flush point
	State_Active                 // Have seen the flush point or have reason to believe the old coordinator has become unavailable and am now assembling transactions based on available knowledge of the state of the base ledger and submitting transactions to the base ledger.
	State_Flush                  // Stopped assembling and dispatching transactions but continue to submit transactions that are already dispatched
	State_Closing                // Have flushed and are continuing to sent closing status for `x` heartbeats.
	State_Error                  // Unrecoverable error state
)

const (
	Event_CoordinatorCreated EventType = iota + common.Event_TransactionStateTransition + 1
	Event_TransactionsDelegated
	Event_HeartbeatReceived
	Event_NewBlock
)

// Type aliases for the generic statemachine types, specialized for coordinator
type (
	Action           = statemachine.Action[*coordinator]
	Guard            = statemachine.Guard[*coordinator]
	Validator        = statemachine.Validator[*coordinator]
	ActionRule       = statemachine.ActionRule[*coordinator]
	Transition       = statemachine.Transition[State, *coordinator]
	EventHandler     = statemachine.EventHandler[State, *coordinator]
	StateDefinition  = statemachine.StateDefinition[State, *coordinator]
	StateDefinitions = statemachine.StateDefinitions[State, *coordinator]
)

func guard_IsActiveCoordinator(ctx context.Context, c *coordinator) bool {
	return c.nodeName == c.activeCoordinatorNode
}

// TODO AM: something to think about
// does a node who is endorsing in a privacy group but not submitting need a way to understand that the preferred active
// coordinator has become unavailable?
// I think the simplest way is to endorse everything that comes in? Are there knock on implications for this?

var stateDefinitionsMap = StateDefinitions{
	State_Initial: {
		Events: map[EventType]EventHandler{
			Event_CoordinatorCreated: {
				Actions: []ActionRule{{Action: action_SelectActiveCoordinator}},
				Transitions: []Transition{
					{To: State_Idle, If: guard_HasActiveCoordinator},
					// TODO AM: add this state to garbage collection and priority for removal
					{To: State_Error, If: statemachine.GuardNot(guard_HasActiveCoordinator)},
				},
			},
		},
	},
	State_Idle: {
		OnTransitionTo: []ActionRule{{Action: action_Idle}},
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{
					If:     guard_IsActiveCoordinator,
					Action: action_ProcessDelegatedTransactions,
				}, {
					If:     statemachine.GuardNot(guard_IsActiveCoordinator),
					Action: action_RejectDelegatedTransactions,
				}},
				Transitions: []Transition{{
					If: guard_IsActiveCoordinator,
					To: State_Active,
				}},
			},
			Event_HeartbeatReceived: {
				Validator: validator_IsHeartbeatFromActiveCoordinator,
				Actions:   []ActionRule{{Action: action_HeartbeatReceived}},
				Transitions: []Transition{{
					To: State_Observing,
				}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{
					{
						Action: action_UpdateBlockHeight,
					},
					{
						// If we have entered a new block range and we are the new coordinator, we stay in State_Idle
						// since as far as we can see there is no active work for this domain instance. If we receive
						// delegated transactions, we will now move to State_Active and process rather than reject them.
						If:     guard_IsNewBlockRange,
						Action: action_SelectActiveCoordinator,
					},
				},
			},
		},
	},
	State_Observing: {
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_RejectDelegatedTransactions}},
			},
			Event_HeartbeatReceived: {
				Validator: validator_IsHeartbeatFromActiveCoordinator,
				Actions: []ActionRule{
					{Action: action_HeartbeatReceived},
					{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
				},
			},
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceLastReceive},
				},
				Transitions: []Transition{{
					To: State_Idle,
					If: guard_HeartbeatThresholdExceeded,
				}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{
					{
						Action: action_UpdateBlockHeight,
					},
					{
						If:     guard_IsNewBlockRange,
						Action: action_SelectActiveCoordinator,
					},
				},
				Transitions: []Transition{{
					To: State_Elect,
					If: guard_IsActiveCoordinator,
				}},
			},
		},
	},
	State_Elect: {
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_RejectDelegatedTransactions}},
			},
			Event_HeartbeatReceived: {
				Validator: validator_IsHeartbeatFromActiveCoordinator,
				Actions: []ActionRule{
					{Action: action_HeartbeatReceived},
					{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
				},
				Transitions: []Transition{{
					To: State_Active,
					If: guard_ActiveCoordinatorFlushComplete,
				}},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{
					{
						// There is a small chance we have come here from State_Closing and still have transactions in terminal
						// states from the previous block range that we haven't cleaned up from memory yet, so we handle that here.
						// We need to keep heartbeating if we have these transactions in memory.
						Validator: validator_TransactionStateTransitionTo(transaction.State_Final),
						Action:    action_CleanUpTransaction,
					},
				},
			},
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceLastReceive},
					{
						// Sending a heartbeat only if we have in memory transactions from a previous block range which hadn't yet
						// been cleaned up when we moved from State_Closing to State_Selected. This is very unlikely to occur, but
						// it is technically possible if we are configured with a small block range but a long closing grace period.
						Action: action_SendHeartbeat,
						If:     guard_HasTransactionsInflight,
					},
				},
				// If we stop receiving heartbeats from the previously active coordinator, we move to State_Active
				// without confirming that the flush is complete. There is a risk that the previous coordinator is actually
				// still submitting transactions and it is network issues causing us to not receive heartbeats. In this case
				// we will have state contention on the base ledger, but this should eventually be resolved with retries.
				Transitions: []Transition{{
					To: State_Active,
					If: guard_HeartbeatThresholdExceeded,
				}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{
					{
						Action: action_UpdateBlockHeight,
					},
					{
						If:     guard_IsNewBlockRange,
						Action: action_SelectActiveCoordinator,
					},
				},
				// This transition should only happen if we have either a very short block range, or the previous active
				// coordinator is taking a very long time to flush (e.g. dispatched transactions but base ledger is unavailble).
				Transitions: []Transition{{
					To: State_Observing,
					If: statemachine.GuardNot(guard_IsActiveCoordinator),
				}},
			},
		},
	},
	State_Active: {
		OnTransitionTo: []ActionRule{
			{Action: action_NewSigningIdentity},
			{Action: action_NotifyCoordinatorActive},
			{Action: action_SelectTransaction},
		},
		Events: map[EventType]EventHandler{
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceStateChange},
					{Action: action_PropagateHeartbeatIntervalToTransactions},
					{Action: action_SendHeartbeat},
				},
				Transitions: []Transition{{
					To: State_Idle,
					If: statemachine.GuardNot(guard_HasTransactionsInflight),
				}},
			},
			Event_TransactionsDelegated: {
				// don't select a transaction here since events must to move into pooled state before
				// they can be selected and there is a separate event for that
				Actions: []ActionRule{{Action: action_ProcessDelegatedTransactions}},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{
					{
						// This TX is leaving dispatched after being reverted on chain, cancel any transaction being assembled
						// This could be more nuanced if we could capture the set of potential states that are being removed
						// as part of unwinding the dependency chain, and only repool the transaction once assembly is complete if
						// it is using one of these potential outputs as an input.
						Validator: statemachine.ValidatorAnd(
							validator_TransactionStateTransitionFrom(transaction.State_Dispatched),
							validator_TransactionStateTransitionTo(transaction.State_Pooled, transaction.State_Reverted),
						),
						If:     guard_HasTransactionAssembling,
						Action: action_cancelCurrentlyAssemblingTransaction,
					},
					{
						Validator: validator_TransactionStateTransitionTo(transaction.State_Pooled),
						Action:    action_PoolTransaction,
					},
					{
						Validator: validator_TransactionStateTransitionTo(transaction.State_Ready_For_Dispatch),
						Action:    action_QueueTransactionForDispatch,
					},
					{
						Validator: validator_TransactionStateTransitionTo(transaction.State_Final),
						Action:    action_CleanUpTransaction,
					},
					{
						Validator: validator_TransactionStateTransitionTo(transaction.State_Evicted),
						Action:    action_CleanUpTransaction,
					},
					{
						Action: action_NudgeDispatchLoop,
					},
					{
						Action: action_SelectTransaction,
						If:     statemachine.GuardNot(guard_HasTransactionAssembling),
					},
				},
			},
			Event_NewBlock: {
				Actions: []ActionRule{
					{Action: action_UpdateBlockHeight},
					{Action: action_SelectActiveCoordinator},
				},
				// We have entered a new block range. Regardless of whether or not we are the preferred active coordinator for
				// this block range, we still need to flush all the dispatched transactions from the previous block range.
				Transitions: []Transition{{
					To: State_Flush,
					If: guard_IsNewBlockRange,
				}},
			},
		},
	},
	State_Flush: {
		OnTransitionTo: []ActionRule{{
			// Clean up transactions that have not reached point of no return. They will disappear from heartbeats which means that the
			// the originator will start trying to redelegate to the new active coordinator. The new coordinator should be rejecting
			// these delegations while it waits for the flush to complete. If it doesn't, the base ledger still protects against duplicate
			// submissions and double spends; we tolerate the inefficiency this causes in the protocol.
			Action: action_CleanUpTransactionsNotYetDispatched,
		}},
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_RejectDelegatedTransactions}},
			},
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceStateChange},
					{Action: action_SendHeartbeat},
					{Action: action_PropagateHeartbeatIntervalToTransactions},
				},
			},
			Event_NewBlock: {
				Actions: []ActionRule{
					{
						Action: action_UpdateBlockHeight,
					},
					{
						If:     guard_IsNewBlockRange,
						Action: action_SelectActiveCoordinator,
					},
				},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{
					{
						Validator: validator_TransactionStateTransitionTo(transaction.State_Final),
						Action:    action_CleanUpTransaction,
					}, {
						// A transaction was dispatched and has been reverted on chain but the revert reason has been considered
						// retryable. The next coordinator will be able to assemble and submit this transaction again. We just
						// need to clean it up from memory.
						Validator: statemachine.ValidatorAnd(
							validator_TransactionStateTransitionFrom(transaction.State_Dispatched),
							statemachine.ValidatorNot(
								validator_TransactionStateTransitionTo(transaction.State_Confirmed, transaction.State_Reverted),
							),
						),
						Action: action_CleanUpTransaction,
					},
				},
				Transitions: []Transition{{
					// There is a new preferred active coordinator for the new block range. We move to State_Closing so we can
					// continue to send heartbeats with our confirmed transactions until they have all reached the end of their
					// grace period.
					To: State_Closing,
					If: statemachine.GuardAnd(guard_FlushComplete, statemachine.GuardNot(guard_IsActiveCoordinator)),
				}, {
					// We are still the preferred active coordinator for the new block range. We move back to State_Active
					// so we can start accepting delegated transactions again, and submitting them using our new signing identity.
					To: State_Active,
					If: statemachine.GuardAnd(guard_FlushComplete, guard_IsActiveCoordinator),
				}},
			},
		},
	},
	State_Closing: {
		// TODO AM: send a heartbeat here so that the new coordinator can see we're in closing immediately. This heartbeat
		// should include our state locks so the new coordinator can start assembling immediately even if they haven't yet
		// indexed the transaction(s) that have just been confirmed.
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_RejectDelegatedTransactions}},
			},
			Event_HeartbeatReceived: {
				Validator: validator_IsHeartbeatFromActiveCoordinator,
				Actions: []ActionRule{
					{Action: action_HeartbeatReceived},
					{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
				},
			},
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceStateChange},
					{Action: action_SendHeartbeat},
					{Action: action_PropagateHeartbeatIntervalToTransactions},
				},
				Transitions: []Transition{{
					To: State_Idle,
					If: statemachine.GuardAnd(
						statemachine.GuardNot(guard_HasTransactionsInflight),
						guard_HeartbeatThresholdExceeded,
					),
				}, {
					To: State_Observing,
					If: statemachine.GuardAnd(
						statemachine.GuardNot(guard_HasTransactionsInflight),
						statemachine.GuardNot(guard_HeartbeatThresholdExceeded),
					),
				}},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{
					{
						Validator: validator_TransactionStateTransitionTo(transaction.State_Final),
						Action:    action_CleanUpTransaction,
					},
				},
			},
			Event_NewBlock: {
				Actions: []ActionRule{
					{
						Action: action_UpdateBlockHeight,
					},
					{
						If:     guard_IsNewBlockRange,
						Action: action_SelectActiveCoordinator,
					},
				},
				Transitions: []Transition{{
					// If we're now the preferred active coordinator again but we haven't been receiving heartbeats
					// then we don't need to make a transition here, we can just wait until we move to State_Idle
					// once we have no transactions in memory.
					To: State_Elect,
					If: statemachine.GuardAnd(
						guard_IsActiveCoordinator,
						statemachine.GuardNot(guard_HeartbeatThresholdExceeded),
					),
				}},
			},
		},
	},
	State_Error: {
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_RejectDelegatedTransactions}},
			},
		},
	},
}

func (c *coordinator) initializeStateMachineEventLoop(initialState State, eventQueueSize int, priorityEventQueueSize int) {
	c.stateMachineEventLoop = statemachine.NewStateMachineEventLoop(statemachine.StateMachineEventLoopConfig[State, *coordinator]{
		InitialState:           initialState,
		Definitions:            stateDefinitionsMap,
		Entity:                 c,
		EventQueueSize:         eventQueueSize,
		PriorityEventQueueSize: priorityEventQueueSize,
		Name:                   fmt.Sprintf("coordinator-%s", c.contractAddress.String()[0:8]),
		TransitionCallback:     c.onStateTransition,
		PreProcess:             c.preProcessEvent,
	})
}

// preProcessEvent handles events that should be processed before the state machine.
// Returns (true, nil) if the event was fully handled and should not be passed to the state machine.
func (c *coordinator) preProcessEvent(ctx context.Context, entity *coordinator, event common.Event) (bool, error) {
	// Transaction events are propagated to the transaction state machine, not the coordinator state machine
	if transactionEvent, ok := event.(transaction.Event); ok {
		log.L(ctx).Debugf("coordinator propagating event %s to transactions: %s", event.TypeString(), transactionEvent.TypeString())
		return true, c.propagateEventToTransaction(ctx, transactionEvent)
	}
	return false, nil
}

// onStateTransition is called when the state machine transitions to a new state
func (c *coordinator) onStateTransition(ctx context.Context, entity *coordinator, from State, to State, event common.Event) {
	c.heartbeatIntervalsSinceStateChange = 0
}

// QueueEvent asynchronously queues a state machine event for processing.
// Should be called by most Paladin components to ensure memory integrity of
// sequencer state machine and transactions.
func (c *coordinator) QueueEvent(ctx context.Context, event common.Event) {
	c.stateMachineEventLoop.QueueEvent(ctx, event)
}

func (c *coordinator) TryQueueEvent(ctx context.Context, event common.Event) bool {
	return c.stateMachineEventLoop.TryQueueEvent(ctx, event)
}

// Queue a state machine event generated internally for the sequencer loop to process. Is prioritized above
// external event sources
func (c *coordinator) queueEventInternal(ctx context.Context, event common.Event) {
	c.stateMachineEventLoop.QueuePriorityEvent(ctx, event)
}

func (s State) String() string {
	switch s {
	case State_Initial:
		return "Initial"
	case State_Idle:
		return "Idle"
	case State_Observing:
		return "Observing"
	case State_Elect:
		return "Elect"
	case State_Active:
		return "Active"
	case State_Flush:
		return "Flush"
	case State_Closing:
		return "Closing"
	case State_Error:
		return "Error"
	}
	return "Unknown"
}
