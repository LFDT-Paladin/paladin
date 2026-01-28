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
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// State represents the coordinator's state
type State int

// EventType is an alias for common.EventType
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
	State_Idle: {
		OnTransitionTo: action_Idle,
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
		},
	},
	State_Elect: {
		OnTransitionTo: action_SendHandoverRequest,
		Events: map[EventType]EventHandler{
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
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
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
			},
			Event_HeartbeatReceived: {
				Actions: []ActionRule{{Action: action_HeartbeatReceived}},
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
				Actions: []ActionRule{
					{Action: action_IncrementHeartbeatIntervalsSinceStateChange},
					{Action: action_SendHeartbeat},
				},
				Transitions: []Transition{{
					To: State_Idle,
					If: guard_Not(guard_HasTransactionsInflight),
				}},
			},
			Event_TransactionsDelegated: {
				Actions: []ActionRule{{Action: action_TransactionsDelegated}},
				// if this is the first transaction we have received, it needs to move into pooled state before
				// it can be selected and there is a separate event for that
			},
			Event_TransactionConfirmed: {
				Actions: []ActionRule{{Action: action_TransactionConfirmed}},
			},
			Event_HandoverRequestReceived: { // MRW TODO - what if N nodes all startup in active mode simultaneously? None of them can request handover because that only happens from State_Observing
				Transitions: []Transition{{
					To: State_Flush,
				}},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{
					{Action: action_TransactionStateTransition},
					{Action: action_NudgeDispatchLoop},
					{Action: action_SelectTransaction, If: guard_Not(guard_HasTransactionAssembling)},
				},
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
				},
			},
			Event_TransactionConfirmed: {
				Actions: []ActionRule{{Action: action_TransactionConfirmed}},
				Transitions: []Transition{{
					To: State_Closing,
					If: guard_FlushComplete,
				}},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{{Action: action_TransactionStateTransition}},
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
				},
				Transitions: []Transition{{
					To: State_Idle,
					If: guard_ClosingGracePeriodExpired,
				}},
			},
			common.Event_TransactionStateTransition: {
				Actions: []ActionRule{{Action: action_TransactionStateTransition}},
			},
		},
	},
}

func (c *coordinator) initializeStateMachineEventLoop(initialState State) {
	c.stateMachineEventLoop = statemachine.NewStateMachineEventLoop(statemachine.StateMachineEventLoopConfig[State, *coordinator]{
		InitialState:        initialState,
		Definitions:         stateDefinitionsMap,
		Entity:              c,
		EventLoopBufferSize: 50, // TODO >1 only required for sqlite coarse-grained locks. Should this be DB-dependent?
		Name:                fmt.Sprintf("coordinator-%s", c.contractAddress.String()[0:8]),
		OnStop: func(ctx context.Context) common.Event {
			// Return the final event to process when stopping
			return &CoordinatorClosedEvent{}
		},
		TransitionCallback: c.onStateTransition,
		PreProcess:         c.preProcessEvent,
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
	// TODO AM: this is a strong candidate for common logging/metrics
	log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf(
		"coord    | %s   | %T | %s -> %s", c.contractAddress.String()[0:8], event, from.String(), to.String())
	c.heartbeatIntervalsSinceStateChange = 0
}

// QueueEvent asynchronously queues a state machine event for processing.
// Should be called by most Paladin components to ensure memory integrity of
// sequencer state machine and transactions.
func (c *coordinator) QueueEvent(ctx context.Context, event common.Event) {
	c.stateMachineEventLoop.QueueEvent(ctx, event)
}

// Action functions - first actions often apply event-specific data to the coordinator's internal state
func action_TransactionConfirmed(ctx context.Context, c *coordinator, event common.Event) error {
	// An earlier version of this code had handling for receiving a confirmation event and using it to monitor
	// transactions that another coordinator is coordinating, so that flush points could be updated and checked
	// in the case of a handover, rather than relying solely on heartbeats. But that same code version only queued
	// the event to a coordinator if it was the active coordinator and knew about the transaction, which meant the
	// monitoring path was never taken.
	//
	// This version of the code brings all the logic about whether a trasaction confirmed event should be acted on
	// into the coordinator state machine. The event is only handled in states where the coordinator is the active
	// coordinator, and then only acted on if the transaction is known. It is functionally equivalent, but without
	// the unused code, and decision making is contained within the state machine.
	e := event.(*TransactionConfirmedEvent)

	if _, ok := c.transactionsByID[e.TxID]; !ok {
		log.L(ctx).Warnf("action_TransactionConfirmed: Coordinator not tracking transaction ID %s", e.TxID)
		return nil
	}

	log.L(ctx).Debugf("we currently have %d transactions to handle, confirming that dispatched TX %s is in our list", len(c.transactionsByID), e.TxID.String())

	dispatchedTransaction, ok := c.transactionsByID[e.TxID]

	if !ok {
		log.L(ctx).Debugf("action_TransactionConfirmed: Coordinator not tracking transaction ID %s", e.TxID)
		return nil
	}

	if dispatchedTransaction.GetLatestSubmissionHash() == nil {
		// The transaction created a chained private transaction so there is no hash to compare
		log.L(ctx).Debugf("transaction %s confirmed with nil dispatch hash (confirmed hash of chained TX %s)", dispatchedTransaction.GetID().String(), e.Hash.String())
	} else if *(dispatchedTransaction.GetLatestSubmissionHash()) != e.Hash {
		// Is this not the transaction that we are looking for?
		// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
		// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
		log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected. Dispatch hash %s, confirmed hash %s", dispatchedTransaction.GetID().String(), dispatchedTransaction.GetLatestSubmissionHash(), e.Hash.String())
	}
	txEvent := &transaction.ConfirmedEvent{
		Hash:         e.Hash,
		RevertReason: e.RevertReason,
		Nonce:        e.Nonce,
	}
	txEvent.TransactionID = e.TxID
	txEvent.EventTime = time.Now()

	log.L(ctx).Debugf("Confirming dispatched TX %s", e.TxID.String())
	err := dispatchedTransaction.HandleEvent(ctx, txEvent)
	if err != nil {
		log.L(ctx).Errorf("error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.GetID().String(), err)
		return err
	}
	return nil
}

func action_NewBlock(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*NewBlockEvent)
	c.currentBlockHeight = e.BlockHeight
	return nil
}

func action_EndorsementRequested(_ context.Context, c *coordinator, event common.Event) error {
	e := event.(*EndorsementRequestedEvent)
	c.activeCoordinatorNode = e.From
	c.coordinatorActive(c.contractAddress, e.From)
	c.updateOriginatorNodePoolInternal(e.From) // In case we ever take over as coordinator we need to send heartbeats to potential originators
	return nil
}

func action_HeartbeatReceived(_ context.Context, c *coordinator, event common.Event) error {
	e := event.(*HeartbeatReceivedEvent)
	c.activeCoordinatorNode = e.From
	c.activeCoordinatorBlockHeight = e.BlockHeight
	c.coordinatorActive(c.contractAddress, e.From)
	c.updateOriginatorNodePoolInternal(e.From) // In case we ever take over as coordinator we need to send heartbeats to potential originators
	for _, flushPoint := range e.FlushPoints {
		c.activeCoordinatorsFlushPointsBySignerNonce[flushPoint.GetSignerNonce()] = flushPoint
	}
	return nil
}

func action_IncrementHeartbeatIntervalsSinceStateChange(ctx context.Context, c *coordinator, event common.Event) error {
	c.heartbeatIntervalsSinceStateChange++
	return nil
}

func action_TransactionStateTransition(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])

	// If a transaction has transitioned to Pooled, add it to the pool queue
	// For pooled transactions, when we are pooling (or re-pooling) we push the transaction
	// to the back of the queue to give best-effort FIFO assembly as transactions arrive at the
	// node. If a transaction needs re-assembly after a revert, it will be processed after
	// a new transaction that hasn't ever been assembled.
	if e.To == transaction.State_Pooled {
		txn := c.transactionsByID[e.TransactionID]
		if txn != nil {
			c.AddTransactionToBackOfPool(ctx, txn)
		}
	}

	// If a transaction has transitioned to Ready_For_Dispatch, queue it for dispatch
	if e.To == transaction.State_Ready_For_Dispatch {
		txn := c.transactionsByID[e.TransactionID]
		if txn != nil {
			c.dispatchQueue <- txn
		}
	}

	// If a transaction has reached its final state, clean it up from the coordinator
	if e.To == transaction.State_Final {
		delete(c.transactionsByID, e.TransactionID)
		c.metrics.DecCoordinatingTransactions()
		err := c.grapher.Forget(e.TransactionID)
		if err != nil {
			log.L(ctx).Errorf("error forgetting transaction %s: %v", e.TransactionID.String(), err)
		}
		log.L(ctx).Debugf("transaction %s cleaned up", e.TransactionID.String())
	}

	return nil
}

func action_SendHandoverRequest(ctx context.Context, c *coordinator, _ common.Event) error {
	c.sendHandoverRequest(ctx)
	return nil
}

func action_SelectTransaction(ctx context.Context, c *coordinator, _ common.Event) error {
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
	return c.selectNextTransactionToAssemble(ctx)
}

func action_Idle(ctx context.Context, c *coordinator, _ common.Event) error {
	c.coordinatorIdle(c.contractAddress)
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
	}
	return nil
}

func action_NudgeDispatchLoop(ctx context.Context, c *coordinator, _ common.Event) error {
	// Prod the dispatch loop with an updated in-flight count. This may release new transactions for dispatch
	c.inFlightMutex.L.Lock()
	defer c.inFlightMutex.L.Unlock()
	clear(c.inFlightTxns)
	dispatchingTransactions := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Dispatched, transaction.State_Submitted, transaction.State_SubmissionPrepared})
	for _, txn := range dispatchingTransactions {
		if !txn.HasPreparedPrivateTransaction() {
			// We don't count transactions that result in new private transactions
			c.inFlightTxns[txn.GetID()] = txn
		}
	}
	log.L(ctx).Debugf("coordinator has %d dispatching transactions", len(c.inFlightTxns))
	c.inFlightMutex.Signal()
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
			// This is currently unreachable because the heartbeat interval event only causes a transaction
			// to transition to State_Final, which has no event handler (the state transition is handled by the coordinator)
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
