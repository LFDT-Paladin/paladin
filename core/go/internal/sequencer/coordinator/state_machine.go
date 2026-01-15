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
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
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
	Event_Activated EventType = iota + common.Event_HeartbeatInterval + 1
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
	Event_TransactionStateTransition
	Event_EndorsementRequested // Only used to update the state machine with updated information about the active coordinator, out of band of the heartbeats
)

// Type aliases for cleaner code - these reference the generic statemachine types specialized for coordinator
type (
	Action          = statemachine.Action[*coordinator]
	Guard           = statemachine.Guard[*coordinator]
	StateUpdate     = statemachine.StateUpdate[*coordinator]
	ActionRule      = statemachine.ActionRule[*coordinator]
	Transition      = statemachine.Transition[State, *coordinator]
	EventHandler    = statemachine.EventHandler[State, *coordinator]
	StateDefinition = statemachine.StateDefinition[State, *coordinator]
)

// buildStateDefinitions creates the state machine configuration for the coordinator
func buildStateDefinitions() map[State]StateDefinition {
	return map[State]StateDefinition{
		State_Idle: {
			OnTransitionTo: action_Idle,
			Events: map[EventType]EventHandler{
				Event_TransactionsDelegated: {
					OnHandleEvent: stateupdate_TransactionsDelegated,
					Transitions: []Transition{{
						To: State_Active,
					}},
				},
				Event_HeartbeatReceived: {
					OnHandleEvent: stateupdate_HeartbeatReceived,
					Transitions: []Transition{{
						To: State_Observing,
					}},
				},
				Event_EndorsementRequested: { // We can assert that someone else is actively coordinating if we're receiving these
					OnHandleEvent: stateupdate_EndorsementRequested,
					Transitions: []Transition{{
						To: State_Observing,
					}},
				},
			},
		},
		State_Observing: {
			Events: map[EventType]EventHandler{
				Event_TransactionsDelegated: {
					OnHandleEvent: stateupdate_TransactionsDelegated,
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
					OnHandleEvent: stateupdate_TransactionsDelegated,
				},
				Event_NewBlock: {
					OnHandleEvent: stateupdate_NewBlock,
					Transitions: []Transition{{
						To: State_Elect,
						If: statemachine.Not(guard_Behind),
					}},
				},
			},
		},
		State_Elect: {
			OnTransitionTo: action_SendHandoverRequest,
			Events: map[EventType]EventHandler{
				Event_TransactionsDelegated: {
					OnHandleEvent: stateupdate_TransactionsDelegated,
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
					OnHandleEvent: stateupdate_TransactionsDelegated,
				},
				Event_TransactionConfirmed: {
					OnHandleEvent: stateupdate_TransactionConfirmed,
					Transitions: []Transition{{
						To: State_Active,
						If: guard_ActiveCoordinatorFlushComplete,
					}},
				},
			},
		},
		State_Active: {
			OnTransitionTo: action_SelectTransaction,
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					OnHandleEvent: stateupdate_HeartbeatInterval,
					Actions: []ActionRule{{
						Action: action_SendHeartbeat,
					}},
					Transitions: []Transition{{
						To: State_Idle,
						If: statemachine.Not(guard_HasTransactionsInflight),
					}},
				},
				Event_TransactionsDelegated: {
					OnHandleEvent: stateupdate_TransactionsDelegated,
					Actions: []ActionRule{{
						Action: action_SelectTransaction,
						If:     statemachine.Not(guard_HasTransactionAssembling),
					}},
				},
				Event_TransactionConfirmed: {
					OnHandleEvent: stateupdate_TransactionConfirmed,
				},
				Event_HandoverRequestReceived: { // MRW TODO - what if N nodes all startup in active mode simultaneously? None of them can request handover because that only happens from State_Observing
					Transitions: []Transition{{
						To: State_Flush,
					}},
				},
			},
		},
		State_Flush: {
			//TODO should we move to active if we get delegated transactions while in flush?
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					OnHandleEvent: stateupdate_HeartbeatInterval,
					Actions: []ActionRule{{
						Action: action_SendHeartbeat,
					}},
				},
				Event_TransactionConfirmed: {
					OnHandleEvent: stateupdate_TransactionConfirmed,
					Transitions: []Transition{{
						To: State_Closing,
						If: guard_FlushComplete,
					}},
				},
			},
		},
		State_Closing: {
			//TODO should we move to active if we get delegated transactions while in closing?
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					OnHandleEvent: stateupdate_HeartbeatInterval,
					Actions: []ActionRule{{
						Action: action_SendHeartbeat,
					}},
					Transitions: []Transition{{
						To: State_Idle,
						If: guard_ClosingGracePeriodExpired,
					}},
				},
			},
		},
	}
}

func (c *coordinator) InitializeStateMachine(ctx context.Context, initialState State) {
	smConfig := statemachine.StateMachineConfig[State, *coordinator]{
		Definitions: buildStateDefinitions(),
		OnTransition: func(ctx context.Context, c *coordinator, from, to State, event common.Event) {
			log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf("coord    | %s   | %T | %s -> %s", c.contractAddress.String()[0:8], event, from.String(), to.String())
			c.heartbeatIntervalsSinceStateChange = 0
		},
	}

	elConfig := statemachine.EventLoopConfig[State, *coordinator]{
		BufferSize: 50, // TODO >1 only required for sqlite coarse-grained locks. Should this be DB-dependent?
		OnEventReceived: func(ctx context.Context, c *coordinator, event common.Event) (bool, error) {
			log.L(ctx).Debugf("coordinator handling new event %s (contract address %s, active coordinator %s, current originator pool %+v)", event.TypeString(), c.contractAddress, c.activeCoordinatorNode, c.originatorNodePool)

			// Transaction events are propagated to the transaction state machines
			if transactionEvent, ok := event.(transaction.Event); ok {
				log.L(ctx).Debugf("coordinator propagating event %s to transactions: %s", event.TypeString(), transactionEvent.TypeString())
				return true, c.propagateEventToTransaction(ctx, transactionEvent)
			}

			// Return false to let the state machine process coordinator-level events
			return false, nil
		},
		OnStop: func(ctx context.Context, c *coordinator) error {
			// Synchronously move the state machine to closed
			return c.stateMachine.ProcessEvent(ctx, c, &CoordinatorClosedEvent{})
		},
	}

	name := "coordinator[" + c.contractAddress.String()[0:8] + "]"
	c.stateMachine = statemachine.NewEventLoopStateMachine(ctx, name, smConfig, elConfig, initialState, c)
}

// QueueEvent queues a state machine event for the event loop to process.
// Should be called by most Paladin components to ensure memory integrity of
// sequencer state machine and transactions.
func (c *coordinator) QueueEvent(ctx context.Context, event common.Event) {
	c.stateMachine.QueueEvent(ctx, event)
}

func action_SendHandoverRequest(ctx context.Context, c *coordinator) error {
	c.sendHandoverRequest(ctx)
	return nil
}

func action_SelectTransaction(ctx context.Context, c *coordinator) error {
	// Take the opportunity to inform the sequencer lifecycle manager that we have become active so it can decide if that has
	// casued us to reach the node's limit on active coordinators.
	c.coordinatorActive(c.contractAddress, c.nodeName)

	// For domain types that can coordinate other nodes' transactions (e.g. Noto or Pente), start heartbeating
	// Domains such as Zeto that are always coordinated on the originating node, heartbeats aren't required
	// because other nodes cannot take over coordination.
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() != prototk.ContractConfig_COORDINATOR_SENDER {
		go c.heartbeatLoop(ctx)
	}

	// Select our next transaction. May return nothing if a different transaction is currently being assembled.
	return c.selectNextTransactionToAssemble(ctx, nil)
}

func action_Idle(ctx context.Context, c *coordinator) error {
	c.coordinatorIdle(c.contractAddress)
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
	}
	return nil
}

func (c *coordinator) heartbeatLoop(ctx context.Context) {
	if c.heartbeatCtx == nil {
		c.heartbeatCtx, c.heartbeatCancel = context.WithCancel(ctx)
		defer c.heartbeatCancel()

		log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf("coord    | %s   | Starting heartbeat loop", c.contractAddress.String()[0:8])

		// Send an initial heartbeat interval event to be handled immediately
		c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})
		err := c.propagateEventToAllTransactions(ctx, &common.HeartbeatIntervalEvent{})
		if err != nil {
			log.L(ctx).Errorf("error propagating heartbeat interval event to all transactions: %v", err)
		}

		// Then every N seconds
		ticker := time.NewTicker(c.heartbeatInterval.(time.Duration))
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})
				err := c.propagateEventToAllTransactions(ctx, &common.HeartbeatIntervalEvent{})
				if err != nil {
					log.L(ctx).Errorf("error propagating heartbeat interval event to all transactions: %v", err)
				}
			case <-c.heartbeatCtx.Done():
				log.L(ctx).Infof("Ending heartbeat loop for %s", c.contractAddress.String())
				c.heartbeatCtx = nil
				c.heartbeatCancel = nil
				return
			}
		}
	}
}
