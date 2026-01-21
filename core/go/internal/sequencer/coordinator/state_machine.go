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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
)

type State int
type EventType = common.EventType

const (
	State_Idle      State = iota // Not acting as a coordinator and not aware of any other active coordinators
	State_Observing              // Not acting as a coordinator but aware of another node acting as a coordinator
	State_Elect                  // Elected to take over from another coordinator and waiting for handover information
	State_Standby                // Going to be coordinator on the next block range but local indexer is not at that block yet.
	State_Prepared               // Have received the handover response but haven't seen the flush point confirmed
	State_Active                 // Have seen the flush point or have reason to believe the old coordinator has become unavailable and am now assembling transactions based on available knowledge of the state of the base ledger and submitting transactions to the base ledger.
	State_Flush                  // Stopped assembling and dispatching transactions but continue to submit transactions that are already dispatched
	State_Closing                // Have flushed and am continuing to sent closing status for `x` heartbeats.
)

func (s State) String() string {
	switch s {
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

const (
	Event_Activated EventType = iota + common.Event_TransactionStateTransition + 1
	Event_Nominated
	Event_Flushed
	Event_Closed
	Event_TransactionsDelegated
	Event_TransactionConfirmed
	Event_TransactionDispatchConfirmed
	Event_HeartbeatReceived
	Event_NewBlock
	Event_HandoverRequestReceived
	Event_HandoverReceived
	Event_EndorsementRequested // Only used to update the state machine with updated information about the active coordinator, out of band of the heartbeats
)

// Type aliases for cleaner code - these reference the generic statemachine types specialized for coordinator
// M = *coordinatorState (mutable)
// R = coordinatorStateReader (read-only)
// Cfg = *config (config)
// Cb = *callbacks (callbacks)
type (
	Action              = statemachine.Action[State, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks]
	Guard               = statemachine.Guard[State, coordinatorStateReader, *stateMachineConfig]
	Validator           = statemachine.Validator[State, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks]
	StateUpdate         = statemachine.StateUpdate[State, *coordinatorState, *stateMachineConfig, *stateMachineCallbacks]
	StateUpdateRule     = statemachine.StateUpdateRule[State, *coordinatorState, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks]
	ActionRule          = statemachine.ActionRule[State, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks]
	Transition          = statemachine.Transition[State, *coordinatorState, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks]
	EventHandler        = statemachine.EventHandler[State, *coordinatorState, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks]
	StateDefinition     = statemachine.StateDefinition[State, *coordinatorState, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks]
	TransitionToHandler = statemachine.TransitionToHandler[State, *coordinatorState, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks]
	StateMachineConfig  = statemachine.StateMachineConfig[State, *coordinatorState, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks]
	EventLoopConfig     = statemachine.EventLoopConfig[State, *coordinatorState, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks, *coordinator]
)

// buildStateDefinitions creates the state machine configuration for the coordinator
func buildStateDefinitions() map[State]StateDefinition {
	return map[State]StateDefinition{
		State_Idle: {
			OnTransitionTo: TransitionToHandler{
				Actions: []ActionRule{{Action: action_Idle}},
			},
			Events: map[EventType]EventHandler{
				Event_TransactionsDelegated: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_TransactionsDelegated}},
					Transitions: []Transition{{
						To: State_Active,
					}},
				},
				Event_HeartbeatReceived: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_HeartbeatReceived}},
					Actions:      []ActionRule{{Action: action_NotifyCoordinatorActive}},
					Transitions: []Transition{{
						To: State_Observing,
					}},
				},
				Event_EndorsementRequested: { // We can assert that someone else is actively coordinating if we're receiving these
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_EndorsementRequested}},
					Actions:      []ActionRule{{Action: action_NotifyCoordinatorActive}},
					Transitions: []Transition{{
						To: State_Observing,
					}},
				},
			},
		},
		State_Observing: {
			Events: map[EventType]EventHandler{
				Event_TransactionsDelegated: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_TransactionsDelegated}},
					Transitions: []Transition{
						{
							To: State_Standby,
							If: guard_Behind,
						},
						{
							To: State_Elect,
							If: statemachine.Not(guard_Behind),
						},
					},
				},
			},
		},
		State_Standby: {
			Events: map[EventType]EventHandler{
				Event_TransactionsDelegated: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_TransactionsDelegated}},
				},
				Event_NewBlock: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_NewBlock}},
					Transitions: []Transition{{
						To: State_Elect,
						If: statemachine.Not(guard_Behind),
					}},
				},
			},
		},
		State_Elect: {
			OnTransitionTo: TransitionToHandler{
				Actions: []ActionRule{{Action: action_SendHandoverRequest}},
			},
			Events: map[EventType]EventHandler{
				Event_TransactionsDelegated: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_TransactionsDelegated}},
				},
				Event_HandoverReceived: {
					Transitions: []Transition{{
						To: State_Prepared,
					}},
				},
			},
		},
		State_Prepared: {
			Events: map[EventType]EventHandler{
				Event_TransactionsDelegated: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_TransactionsDelegated}},
				},
				Event_TransactionConfirmed: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_TransactionConfirmed}},
					Transitions: []Transition{{
						To: State_Active,
						If: guard_ActiveCoordinatorFlushComplete,
					}},
				},
			},
		},
		State_Active: {
			OnTransitionTo: TransitionToHandler{
				StateUpdates: []StateUpdateRule{
					{StateUpdate: stateupdate_ResetHeartbeatIntervalsSinceStateChange},
					{StateUpdate: stateupdate_SelectTransaction},
				},
				Actions: []ActionRule{{
					Action: action_CoordinatorActive,
				}, {
					Action: action_StartHeartbeatLoop,
					If:     guard_ShouldHeartbeat,
				}, {
					Action: action_NotifiyTransactionSelected,
					If:     guard_HasTransactionAssembling,
				}},
			},
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_HeartbeatInterval}},
					Actions: []ActionRule{
						{Action: action_PropagateEventToAllTransactions},
						{Action: action_SendHeartbeat},
					},
					Transitions: []Transition{{
						To: State_Idle,
						If: statemachine.Not(guard_HasTransactionsInflight),
					}},
				},
				Event_TransactionsDelegated: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_TransactionsDelegated}},
					// We don't look to see if we should select a new transaction here
					// because the new transaction won't be added to the pool until we
					// have processed it's state transition from initial to pooled.
				},
				Event_TransactionConfirmed: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_TransactionConfirmed}},
				},
				Event_HandoverRequestReceived: { // MRW TODO - what if N nodes all startup in active mode simultaneously? None of them can request handover because that only happens from State_Observing
					Transitions: []Transition{{
						To: State_Flush,
					}},
				},
				common.Event_TransactionStateTransition: {
					StateUpdates: []StateUpdateRule{
						{
							StateUpdate: stateupdate_CalculateInflightTransactions,
						}, {
							EventValidator: validator_IsTransitionToPooled,
							StateUpdate:    stateupdate_AddTransactionToPool,
						}, {
							EventValidator: validator_IsTransitionToFinal,
							StateUpdate:    stateupdate_CleanupFinalTransaction,
						}, {
							EventValidator: validator_IsTransitionFromAssembling,
							StateUpdate:    stateupdate_ClearAssemblingTransaction,
						},
						{
							If:          statemachine.Not(guard_HasTransactionAssembling),
							StateUpdate: stateupdate_SelectTransaction,
						},
					},
					Actions: []ActionRule{{
						EventValidator: validator_IsTransitionToReadyToDispatch,
						Action:         action_QueueTransactionForDispatch,
					}, {
						EventValidator: validator_IsTransitionFromAssembling,
						If:             guard_HasTransactionAssembling,
						Action:         action_NotifiyTransactionSelected,
					}},
				},
			},
		},
		State_Flush: {
			OnTransitionTo: TransitionToHandler{
				StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_ResetHeartbeatIntervalsSinceStateChange}},
			},
			//TODO should we move to active if we get delegated transactions while in flush?
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_HeartbeatInterval}},
					Actions: []ActionRule{
						{Action: action_PropagateEventToAllTransactions},
						{Action: action_SendHeartbeat},
					},
				},
				Event_TransactionConfirmed: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_TransactionConfirmed}},
					Transitions: []Transition{{
						To: State_Closing,
						If: guard_FlushComplete,
					}},
				},
				common.Event_TransactionStateTransition: {
					StateUpdates: []StateUpdateRule{{
						EventValidator: validator_IsTransitionToFinal,
						StateUpdate:    stateupdate_CleanupFinalTransaction,
					}},
				},
			},
		},
		State_Closing: {
			OnTransitionTo: TransitionToHandler{
				StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_ResetHeartbeatIntervalsSinceStateChange}},
			},
			//TODO should we move to active if we get delegated transactions while in closing?
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					StateUpdates: []StateUpdateRule{{StateUpdate: stateupdate_HeartbeatInterval}},
					Actions: []ActionRule{
						{Action: action_PropagateEventToAllTransactions},
						{Action: action_SendHeartbeat},
					},
					Transitions: []Transition{{
						To: State_Idle,
						If: guard_ClosingGracePeriodExpired,
					}},
				},
				common.Event_TransactionStateTransition: {
					StateUpdates: []StateUpdateRule{{
						EventValidator: validator_IsTransitionToFinal,
						StateUpdate:    stateupdate_CleanupFinalTransaction,
					}},
				},
			},
		},
	}
}

func (c *coordinator) InitializeStateMachine(ctx context.Context, initialState State) {
	smConfig := StateMachineConfig{
		Definitions: buildStateDefinitions(),
		OnTransition: func(ctx context.Context, reader coordinatorStateReader, config *stateMachineConfig, _ *stateMachineCallbacks, from, to State, event common.Event) {
			log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf("coord    | %s   | %T | %s -> %s", config.contractAddress.String()[0:8], event, from.String(), to.String())
		},
	}

	elConfig := EventLoopConfig{
		BufferSize: 50, // TODO >1 only required for sqlite coarse-grained locks. Should this be DB-dependent?
		OnEventReceived: func(ctx context.Context, reader coordinatorStateReader, config *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) (bool, error) {
			log.L(ctx).Debugf("coordinator handling new event %s (contract address %s, active coordinator %s, current originator pool %+v)", event.TypeString(), config.contractAddress, reader.GetActiveCoordinatorNode(), reader.GetOriginatorNodePool())
			// Transaction events are propagated to the transaction state machines
			if transactionEvent, ok := event.(transaction.Event); ok {
				log.L(ctx).Debugf("coordinator propagating event %s to transactions: %s", event.TypeString(), transactionEvent.TypeString())
				if txn := reader.GetTransactionByID(transactionEvent.GetTransactionID()); txn != nil {
					return true, txn.ProcessEvent(ctx, transactionEvent)
				} else {
					log.L(ctx).Debugf("ignoring event because transaction not known to this coordinator %s", transactionEvent.GetTransactionID().String())
				}
				return true, nil
			}
			// Return false to let the state machine process coordinator-level events
			return false, nil
		},
		OnStop: func(ctx context.Context, c *coordinator) error {
			// Synchronously move the state machine to closed
			return c.stateMachine.ProcessEvent(ctx, &CoordinatorClosedEvent{})
		},
	}

	name := "coordinator[" + c.smConfig.contractAddress.String()[0:8] + "]"
	c.stateMachine = statemachine.NewEventLoopStateMachine(ctx, name, smConfig, elConfig, initialState, c)
}

// QueueEvent queues a state machine event for the event loop to process.
// Should be called by most Paladin components to ensure memory integrity of
// sequencer state machine and transactions.
func (c *coordinator) QueueEvent(ctx context.Context, event common.Event) {
	c.stateMachine.QueueEvent(ctx, event)
}

// func (c *coordinator) heartbeatLoop(ctx context.Context) {
// 	if c.heartbeatCtx == nil {
// 		c.heartbeatCtx, c.heartbeatCancel = context.WithCancel(ctx)
// 		defer c.heartbeatCancel()

// 		log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf("coord    | %s   | Starting heartbeat loop", c.contractAddress.String()[0:8])

// 		// Send an initial heartbeat interval event to be handled immediately
// 		c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})
// 		err := c.propagateEventToAllTransactions(ctx, &common.HeartbeatIntervalEvent{})
// 		if err != nil {
// 			log.L(ctx).Errorf("error propagating heartbeat interval event to all transactions: %v", err)
// 		}

// 		// Then every N seconds
// 		ticker := time.NewTicker(c.heartbeatInterval.(time.Duration))
// 		defer ticker.Stop()
// 		for {
// 			select {
// 			case <-ticker.C:
// 				c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})
// 				err := c.propagateEventToAllTransactions(ctx, &common.HeartbeatIntervalEvent{})
// 				if err != nil {
// 					log.L(ctx).Errorf("error propagating heartbeat interval event to all transactions: %v", err)
// 				}
// 			case <-c.heartbeatCtx.Done():
// 				log.L(ctx).Infof("Ending heartbeat loop for %s", c.contractAddress.String())
// 				c.heartbeatCtx = nil
// 				c.heartbeatCancel = nil
// 				return
// 			}
// 		}
// 	}
// }
