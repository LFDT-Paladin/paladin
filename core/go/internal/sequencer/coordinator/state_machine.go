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
	State_Elect                  // Elected to take over from another coordinator and waiting for handover information
	State_Standby                // Going to be coordinator on the next block range but local indexer is not at that block yet.
	State_Prepared               // Have received the handover response but haven't seen the flush point confirmed
	State_Active                 // Have seen the flush point or have reason to believe the old coordinator has become unavailable and am now assembling transactions based on available knowledge of the state of the base ledger and submitting transactions to the base ledger.
	State_Flush                  // Stopped assembling and dispatching transactions but continue to submit transactions that are already dispatched
	State_Closing                // Have flushed and am continuing to sent closing status for `x` heartbeats.
)

const (
	Event_Nominated EventType = iota + common.Event_TransactionStateTransition + 1
	Event_Flushed
	Event_Closed
	Event_CoordinatorCreated
	Event_TransactionsDelegated
	Event_TransactionDispatchConfirmed
	Event_HeartbeatReceived
	Event_NewBlock
	Event_HandoverRequestReceived
	Event_HandoverReceived
	Event_EndorsementRequested // Only used to update the state machine with updated information about the active coordinator, out of band of the heartbeats
	Event_OriginatorNodePoolUpdateRequested
)

// Type aliases for the generic statemachine types, specialized for coordinator
type (
	Action           = statemachine.Action[*coordinator]
	Guard            = statemachine.Guard[*coordinator]
	ActionRule       = statemachine.ActionRule[*coordinator]
	Transition       = statemachine.Transition[State, *coordinator]
	EventHandler     = statemachine.EventHandler[State, *coordinator]
	StateDefinition  = statemachine.StateDefinition[State, *coordinator]
	StateDefinitions = statemachine.StateDefinitions[State, *coordinator]
)

var stateDefinitionsMap = StateDefinitions{
	State_Initial: {
		Events: map[EventType]EventHandler{
			Event_CoordinatorCreated: {
				Actions: []ActionRule{{Action: action_SelectActiveCoordinator}},
				Transitions: []Transition{
					{To: State_Idle, If: guard_HasActiveCoordinator},
				},
			},
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
				Transitions: []Transition{{
					To: State_Active,
				}},
			},
			Event_HeartbeatReceived: {
				Actions: []ActionRule{{Action: action_HeartbeatReceived}},
				Transitions: []Transition{{
					To: State_Observing,
				}},
			},
			Event_EndorsementRequested: { // We can assert that someone else is actively coordinating if we're receiving these
				Actions: []ActionRule{{Action: action_EndorsementRequested}},
				Transitions: []Transition{{
					To: State_Observing,
				}},
			},
			Event_OriginatorNodePoolUpdateRequested: {
				Actions: []ActionRule{
					{
						Action: action_UpdateOriginatorNodePoolFromEvent,
					},
					{
						Action: action_SelectActiveCoordinator,
					},
				},
				Transitions: []Transition{
					{
						To: State_Idle,
						If: guard_HasActiveCoordinator,
					},
				},
			},
			Event_NewBlock: {
				Actions: []ActionRule{{Action: action_NewBlock}},
			},
		},
	},
	State_Idle: {
		OnTransitionTo: []ActionRule{{Action: action_Idle}},
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
				Transitions: []Transition{{
					To: State_Active,
				}},
			},
			Event_HeartbeatReceived: {
				Actions: []ActionRule{{Action: action_HeartbeatReceived}},
				Transitions: []Transition{{
					To: State_Observing,
				}},
			},
			Event_EndorsementRequested: { // We can assert that someone else is actively coordinating if we're receiving these
				Actions: []ActionRule{{Action: action_EndorsementRequested}},
				Transitions: []Transition{{
					To: State_Observing,
				}},
			},
			Event_OriginatorNodePoolUpdateRequested: {
				Actions: []ActionRule{{Action: action_UpdateOriginatorNodePoolFromEvent}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{{Action: action_NewBlock}},
			},
		},
	},
	State_Observing: {
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
				Transitions: []Transition{
					{
						To: State_Standby,
						If: guard_Behind,
					},
					{
						To: State_Elect,
						If: guard_Not(guard_Behind),
					},
				},
			},
			Event_HeartbeatReceived: {
				Actions: []ActionRule{
					{Action: action_HeartbeatReceived},
					{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
				},
			},
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceLastReceive},
					// When the threshold is exceeded, try the next deterministic coordinator candidate
					// before committing to State_Idle. On a successful failover, action_AttemptCoordinatorFailover
					// resets heartbeatIntervalsSinceLastReceive to 0, which causes guard_ObservingIdleThresholdExceeded
					// to return false and prevents the transition below.
					{
						Action: action_AttemptCoordinatorFailover,
						If:     guard_ObservingIdleThresholdExceeded,
					},
				},
				Transitions: []Transition{{
					To: State_Idle,
					If: guard_ObservingIdleThresholdExceeded,
				}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{
					{Action: action_NewBlock},                        // updates currentBlockHeight, resets failoverIndex on range change
					{Action: action_ReevaluateCoordinatorOnNewBlock}, // re-selects coordinator if index or range changed
				},
			},
			Event_OriginatorNodePoolUpdateRequested: {
				Actions: []ActionRule{{Action: action_UpdateOriginatorNodePoolFromEvent}},
			},
		},
	},
	State_Standby: {
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{{Action: action_NewBlock}},
				Transitions: []Transition{{
					To: State_Elect,
					If: guard_Not(guard_Behind),
				}},
			},
			Event_OriginatorNodePoolUpdateRequested: {
				Actions: []ActionRule{{Action: action_UpdateOriginatorNodePoolFromEvent}},
			},
		},
	},
	State_Elect: {
		OnTransitionTo: []ActionRule{{Action: action_SendHandoverRequest}},
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
			},
			Event_HandoverReceived: {
				Transitions: []Transition{{
					To: State_Prepared,
				}},
			},
			Event_OriginatorNodePoolUpdateRequested: {
				Actions: []ActionRule{{Action: action_UpdateOriginatorNodePoolFromEvent}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{{Action: action_NewBlock}},
			},
		},
	},
	State_Prepared: {
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
			},
			Event_HeartbeatReceived: {
				Actions: []ActionRule{{Action: action_HeartbeatReceived}},
				Transitions: []Transition{{
					To: State_Active,
					If: guard_ActiveCoordinatorFlushComplete,
				}},
			},
			Event_OriginatorNodePoolUpdateRequested: {
				Actions: []ActionRule{{Action: action_UpdateOriginatorNodePoolFromEvent}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{{Action: action_NewBlock}},
			},
		},
	},
	State_Active: {
		OnTransitionTo: []ActionRule{{Action: action_SelectTransaction}},
		Events: map[EventType]EventHandler{
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceStateChange},
					{Action: action_SendHeartbeat},
					{Action: action_PropagateHeartbeatToTransactions},
				},
				Transitions: []Transition{{
					To: State_Idle,
					If: guard_Not(guard_HasTransactionsInflight),
				}},
			},
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
				// don't select a transaction here since events must to move into pooled state before
				// they can be selected and there is a separate event for that
			},
			Event_HandoverRequestReceived: { // MRW TODO - what if N nodes all startup in active mode simultaneously? None of them can request handover because that only happens from State_Observing
				Transitions: []Transition{{
					To: State_Flush,
				}},
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
						If:     guard_Not(guard_HasTransactionAssembling),
					},
				},
			},
			Event_OriginatorNodePoolUpdateRequested: {
				Actions: []ActionRule{{Action: action_UpdateOriginatorNodePoolFromEvent}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{{Action: action_NewBlock}},
			},
		},
	},
	State_Flush: {
		//TODO: should the dispatch loop stop dispatching transactions while in flush?
		//TODO should we move to active if we get delegated transactions while in flush?
		Events: map[EventType]EventHandler{
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceStateChange},
					{Action: action_SendHeartbeat},
					{Action: action_PropagateHeartbeatToTransactions},
				},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{
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
				},
				Transitions: []Transition{{
					To: State_Closing,
					If: guard_FlushComplete,
				}},
			},
			Event_OriginatorNodePoolUpdateRequested: {
				Actions: []ActionRule{{Action: action_UpdateOriginatorNodePoolFromEvent}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{{Action: action_NewBlock}},
			},
		},
	},
	State_Closing: {
		//TODO should we move to active if we get delegated transactions while in closing?
		Events: map[EventType]EventHandler{
			common.Event_HeartbeatInterval: {
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceStateChange},
					{Action: action_SendHeartbeat},
					{Action: action_PropagateHeartbeatToTransactions},
				},
				Transitions: []Transition{{
					To: State_Idle,
					If: guard_ClosingGracePeriodExpired,
				}},
			},
			common.Event_TransactionStateTransition: {
				// TODO: these actions probably shouldn't be necessary in Closing state
				// but this is closely related to many of the other TODO questions in this
				// state machine definition and they need to be addressed together
				Actions: []ActionRule{
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
				},
			},
			Event_OriginatorNodePoolUpdateRequested: {
				Actions: []ActionRule{{Action: action_UpdateOriginatorNodePoolFromEvent}},
			},
			Event_NewBlock: {
				Actions: []ActionRule{{Action: action_NewBlock}},
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
	case State_Standby:
		return "Standby"
	case State_Prepared:
		return "Prepared"
	case State_Active:
		return "Active"
	case State_Flush:
		return "Flush"
	case State_Closing:
		return "Closing"
	}
	return "Unknown"
}
