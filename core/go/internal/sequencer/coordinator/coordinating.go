/*
 * Copyright © 2026 Kaleido, Inc.
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
	"slices"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// Enum for delegation acknowledgement errors
type DelegationAcknowledgementError int64

const (
	DelegationAcknowledgementError_None DelegationAcknowledgementError = iota
	DelegationAcknowledgementError_MaxInflightTransactions
	DelegationAcknowledgementError_CoordinatorError
	DelegationAcknowledgementError_PreviousTransactionError
)

// action_SendHandoverRequest sends a CoordinatorHandoverRequest to the current active coordinator,
// asking it to step down so this node can take over. Creates an IdempotentRequest on first call
// and arms the request-timeout timer. Mirrors the pattern of sendAssembleRequest.
func action_SendHandoverRequest(ctx context.Context, c *coordinator, _ common.Event) error {
	return c.sendHandoverRequest(ctx)
}

// action_NudgeHandoverRequest is called when the request-timeout fires and re-prompts the
// IdempotentRequest to send if enough time has elapsed since the last attempt.
func action_NudgeHandoverRequest(ctx context.Context, c *coordinator, _ common.Event) error {
	return c.nudgeHandoverRequest(ctx)
}

func (c *coordinator) sendHandoverRequest(ctx context.Context) error {
	if c.pendingHandoverRequest == nil {
		c.pendingHandoverRequest = common.NewIdempotentRequest(ctx, c.clock, c.requestTimeout, func(ctx context.Context, _ uuid.UUID) error {
			return c.transportWriter.SendHandoverRequest(ctx, c.currentActiveCoordinator, c.contractAddress)
		})
		c.scheduleRequestTimeout(ctx)
	}
	return c.pendingHandoverRequest.Nudge(ctx)
}

func (c *coordinator) nudgeHandoverRequest(ctx context.Context) error {
	if c.pendingHandoverRequest == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "nudgeHandoverRequest called with no pending request")
	}
	return c.pendingHandoverRequest.Nudge(ctx)
}

// action_ScheduleStateTimeout arms the state give-up timer. Called from OnTransitionTo of states
// that use the send-nudge-give-up pattern (currently State_Elect).
func action_ScheduleStateTimeout(ctx context.Context, c *coordinator, _ common.Event) error {
	c.scheduleStateTimeout(ctx)
	return nil
}

// action_ClearTimeoutSchedules cancels any pending request and state timers. Included as a
// transition action on every exit from a timed state so timers never fire after the state is left.
func action_ClearTimeoutSchedules(_ context.Context, c *coordinator, _ common.Event) error {
	c.clearTimeoutSchedules()
	return nil
}

func (c *coordinator) scheduleRequestTimeout(ctx context.Context) {
	if c.cancelRequestTimeout != nil {
		c.cancelRequestTimeout()
	}
	c.cancelRequestTimeout = c.clock.ScheduleTimer(ctx, c.requestTimeout, func() {
		c.QueueEvent(ctx, &RequestTimeoutIntervalEvent{})
	})
}

func (c *coordinator) scheduleStateTimeout(ctx context.Context) {
	if c.cancelStateTimeout != nil {
		c.cancelStateTimeout()
	}
	c.cancelStateTimeout = c.clock.ScheduleTimer(ctx, c.stateTimeout, func() {
		c.QueueEvent(ctx, &StateTimeoutIntervalEvent{})
	})
}

func (c *coordinator) clearTimeoutSchedules() {
	if c.cancelRequestTimeout != nil {
		c.cancelRequestTimeout()
		c.cancelRequestTimeout = nil
	}
	if c.cancelStateTimeout != nil {
		c.cancelStateTimeout()
		c.cancelStateTimeout = nil
	}
	c.pendingHandoverRequest = nil
}

// action_ProcessConfirmedTransactionsFromSnapshot cleans up any delegated transactions that the
// flushing/closing coordinator has already confirmed. Called in State_Prepared so that transactions
// confirmed by the outgoing coordinator are not redundantly re-submitted after becoming Active.
func action_ProcessConfirmedTransactionsFromSnapshot(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*common.HeartbeatReceivedEvent)
	if e.CoordinatorSnapshot == nil {
		return nil
	}
	for _, confirmed := range e.CoordinatorSnapshot.ConfirmedTransactions {
		c.cleanUpTransaction(ctx, confirmed.ID)
	}
	return nil
}

// action_ImportStatesAndLocks imports confirmed locks and private state data from the previous
// coordinator's closing heartbeat into the grapher. This covers states confirmed within the block
// height tolerance window.
// Triggered as a transition action on State_Prepared → State_Active when the closing heartbeat arrives.
func action_ImportStatesAndLocks(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*common.HeartbeatReceivedEvent)
	snapshot := e.CoordinatorSnapshot
	if len(snapshot.Locks) > 0 || len(snapshot.OutputStates) > 0 {
		log.L(ctx).Debugf("action_ImportStatesAndLocks: importing %d output states and %d locks from previous coordinator snapshot", len(snapshot.OutputStates), len(snapshot.Locks))
		c.grapher.ImportStatesAndLocks(ctx, snapshot.OutputStates, snapshot.Locks)
	}
	return nil
}

// Originators send only the delegated transactions that they believe the coordinator needs to know/be reminded about. Which transactions are
// included in this list depends on whether it is an intitial attempt or a scheduled retry, and whether individual delegation timeouts have
// been exceeded. This means that the coordinator cannot infer any dependency or ordering between transactions based on the list of transactions
// in the request.
func action_ProcessDelegatedTransactions(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*TransactionsDelegatedEvent)
	c.recordOriginatorActivity(e.FromNode)
	return c.addToDelegatedTransactions(ctx, e.Originator, e.Transactions, e.DelegationID, c.newCoordinatorTransaction)
}

// recordOriginatorActivity records that an originator node has sent a delegation request,
// resetting its inactivity counter to 0. No-op in ENDORSER mode.
func (c *coordinator) recordOriginatorActivity(node string) {
	if c.coordinatorSelection == prototk.ContractConfig_COORDINATOR_ENDORSER {
		return
	}
	c.originatorActivity[node] = 0
}

// guard_IsCoordinatorEndorserSelectionMode returns true when the coordinator is
// configured for COORDINATOR_ENDORSER mode, where all endorsers are also coordinator candidates.
func guard_IsCoordinatorEndorserSelectionMode(_ context.Context, c *coordinator) bool {
	return c.coordinatorSelection == prototk.ContractConfig_COORDINATOR_ENDORSER
}

// updateEndorserCandidates adds newly-discovered endorser nodes to the candidate pool.
// Only operates in ENDORSER mode; no-op in STATIC/SENDER. When new nodes are actually added
// both the coordinator's own priority list is recomputed and the co-located originator is
// notified with the updated candidates so it can recompute its own list independently.
func (c *coordinator) updateEndorserCandidates(ctx context.Context, nodes ...string) bool {
	if c.coordinatorSelection != prototk.ContractConfig_COORDINATOR_ENDORSER {
		return false
	}
	before := len(c.endorserCandidates)
	for _, node := range nodes {
		if !slices.Contains(c.endorserCandidates, node) {
			c.endorserCandidates = append(c.endorserCandidates, node)
		}
	}
	if !slices.Contains(c.endorserCandidates, c.nodeName) {
		c.endorserCandidates = append(c.endorserCandidates, c.nodeName)
	}
	if len(c.endorserCandidates) > before {
		slices.Sort(c.endorserCandidates)
		c.coordinatorPriorityList = common.ComputeCoordinatorPriorityList(
			ctx,
			c.endorserCandidates,
			c.effectiveBlockHeight,
		)
		c.notifyOriginator(ctx, &common.EndorserNodesDiscoveredEvent{
			// Put a copy of the candidates in the event so the originator can't modify the coordinator's internal state.
			Nodes: slices.Clone(c.endorserCandidates),
		})
		return true
	}
	return false
}

func (c *coordinator) coordinatorTransactionHandleEvent(ctx context.Context, txID uuid.UUID, event common.Event) error {
	txn := c.transactionsByID[txID]
	if txn == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerTransactionNotFound, txID)
	}
	return txn.HandleEvent(ctx, event)
}

func (c *coordinator) getCoordinatorTransactionState(ctx context.Context, id uuid.UUID) (transaction.State, bool) {
	txn := c.transactionsByID[id]
	if txn == nil {
		return transaction.State(0), false
	}
	return txn.GetCurrentState(), true
}

func (c *coordinator) getCoordinatorSigningIdentity() string {
	c.signingIdentity.used = true
	return c.signingIdentity.value
}

func action_RefreshBlockHeight(ctx context.Context, c *coordinator, _ common.Event) error {
	c.refreshBlockHeight(ctx)
	return nil
}

// refreshBlockHeight queries the live block height, caches it in c.currentBlockHeight,
// calls grapher.ForgetLocks, recomputes the priority list, and queues an internal
// EpochBoundaryReachedEvent when the effective block height advances to a new epoch.
func (c *coordinator) refreshBlockHeight(ctx context.Context) {
	liveHeight := c.engineIntegration.GetBlockHeight(ctx)
	c.currentBlockHeight = liveHeight
	c.grapher.ForgetLocks(ctx, uint64(liveHeight))
	c.calculateCoordinatorPriorities(ctx)
	newEffective := common.ComputeEffectiveBlockHeight(uint64(liveHeight), c.coordinatorSelectionBlockRange)
	if newEffective != c.effectiveBlockHeight {
		c.effectiveBlockHeight = newEffective
		c.queueEventInternal(ctx, &EpochBoundaryReachedEvent{})
	}
}

func (c *coordinator) newCoordinatorTransaction(ctx context.Context, originator string, originatorNode string, nodeName string, pt *components.PrivateTransaction) transaction.CoordinatorTransaction {
	return transaction.NewTransaction(
		ctx,
		originator,
		originatorNode,
		nodeName,
		pt,
		c.getCoordinatorSigningIdentity,
		c.transportWriter,
		c.clock,
		c.queueEventInternal,
		c.coordinatorTransactionHandleEvent,
		c.getCoordinatorTransactionState,
		func(ctx context.Context, nodes ...string) { c.updateEndorserCandidates(ctx, nodes...) },
		c.engineIntegration,
		c.refreshBlockHeight,
		func() int64 { return c.currentBlockHeight },
		c.blockHeightTolerance,
		c.syncPoints,
		c.components,
		c.domainAPI,
		c.dCtx,
		c.requestTimeout,
		c.stateTimeout,
		c.closingGracePeriod,
		c.baseLedgerRevertRetryThreshold,
		c.assembleErrorRetryThreshhold,
		c.grapher,
		c.stateVisibilityTracker,
		c.dependencyTracker,
		c.metrics,
	)
}

// originator must be a fully qualified identity locator otherwise an error will be returned
func (c *coordinator) addToDelegatedTransactions(
	ctx context.Context,
	originator string,
	transactions []*components.PrivateTransaction,
	delegationID string,
	createTransaction func(
		ctx context.Context,
		originator string,
		originatorNode string,
		nodeName string,
		pt *components.PrivateTransaction) transaction.CoordinatorTransaction) error {

	var previousTransaction transaction.CoordinatorTransaction

	delegateAcknowledgementIDs := make([]string, 0, len(transactions))
	delegateAcknowledgementErrors := make([]int64, len(transactions))
	rejectedMaxInFlight := 0
	acceptedTransactions := 0
	inProgressTransactions := 0
	var txnHandlingError error

	_, originatorNode, err := pldtypes.PrivateIdentityLocator(originator).Validate(ctx, "", false)
	if err != nil {
		log.L(ctx).Errorf("error validating originator %s: %s", originator, err)
		// This is likely a code bug that the originator can't do anything about, and since we can't parse the node to send an acknowledgement back to,
		// no point sending a delegation acknowledgement here.
		return err
	}

	for i, txn := range transactions {
		// Acknowledge every delegation
		delegateAcknowledgementIDs = append(delegateAcknowledgementIDs, txn.ID.String())

		if txnHandlingError != nil {
			// Any previous errors, don't handle this or subsequent TXNs to maintain FIFO ordering
			delegateAcknowledgementErrors[i] = int64(DelegationAcknowledgementError_PreviousTransactionError)
			continue
		}

		// store the transaction if it already exists, so if the next transaction is new we can establish the dependency via an event
		if c.transactionsByID[txn.ID] != nil {
			inProgressTransactions++
			previousTransaction = c.transactionsByID[txn.ID]
			log.L(ctx).Debugf("transaction %s already being coordinated", txn.ID.String())
			continue
		}

		if len(c.transactionsByID) >= c.maxInflightTransactions {
			rejectedMaxInFlight++
			log.L(ctx).Tracef("transaction %s being rejected - reached max in-flight limit", txn.ID.String())
			delegateAcknowledgementErrors[i] = int64(DelegationAcknowledgementError_MaxInflightTransactions)
			// Since all subsequent transactions will be rejected for the same reason, go through them all recording
			// an error for the delegation acknowledgement response. The originator may choose to do nothing but log
			// and retry later.
			continue
		}

		// We use the order in which transaction are delegated to establish preassembly dependencies, which is what allows us to
		// ensure FIFO ordering within an originator up until first assembly.
		//
		// An originator sends all of its known transactions (i.e. all that have not yet been confirmed) with every delegation request.
		// If an originator believes a transaction has been assembled for the first time, it definitely has been, so we can
		// trust that we have all the information we need in this request to ensure the ordering.
		// We cannot rely on an originator to know that that a transaction has never been assembled, so we need to check our
		// own records of transactions states, and only establish dependencies when we know a prereq transaction is definitely
		// going to be selected for assembly again.

		// Checking for prereq state here means that there is the potential for a race condition with the dispatch loop. The current
		// code is safe because:
		// - we check the prereq transaction state under lock
		// - if the prereq transaction is in a preassembly state then the only goroutine that can move it out of that state is this one
		//   so we can establish the dependency knowing that the new transaction will definitely receive the selection notitication

		if previousTransaction != nil {
			switch previousTransaction.GetCurrentState() {
			case transaction.State_Initial, transaction.State_PreAssembly_Blocked, transaction.State_Pooled:
				txID := previousTransaction.GetID()
				// New delegated transaction depends on the previous one while both are in pre-assembly flow.
				// There is an incredibly slim possibility that the transaction has actually been repooled, so we are past first assembly,
				// but since we have no way of checking this it causes no issues to establish the dependency, since the already pooled transaction
				// will be selected for assembly ahead of this new transaction anyway.
				//
				// This would only be possible if
				// - the coordinator has been rejecting delegated transaction after reaching its max inflight limit
				// - the originator has missed the assembly request for the previous transaction, causing it to be repooled
				c.dependencyTracker.GetPreassemblyDeps().AddPrerequisite(ctx, txn.ID, txID)
			}
		}

		newTransaction := createTransaction(ctx, originator, originatorNode, c.nodeName, txn)

		c.transactionsByID[txn.ID] = newTransaction
		c.metrics.IncCoordinatingTransactions()
		acceptedTransactions++

		receivedEvent := &transaction.DelegatedEvent{}
		receivedEvent.TransactionID = txn.ID

		err = newTransaction.HandleEvent(ctx, receivedEvent)
		if err != nil {
			delete(c.transactionsByID, txn.ID)
			c.metrics.DecCoordinatingTransactions()
			acceptedTransactions--
			txnHandlingError = err
			delegateAcknowledgementErrors[i] = int64(DelegationAcknowledgementError_CoordinatorError)
			// All subsequent transactions will be skipped
			continue
		}
		previousTransaction = newTransaction
	}

	// Acknowledge the delegate request. Optionally errors can be returned which the originator may use to base re-delegate decisions on
	err = c.transportWriter.SendDelegationResponse(ctx, originatorNode, delegationID, delegateAcknowledgementIDs, delegateAcknowledgementErrors, uint64(c.currentBlockHeight))
	if err != nil {
		return err
	}

	if rejectedMaxInFlight > 0 {
		err := i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, c.maxInflightTransactions, originatorNode, len(transactions), acceptedTransactions, inProgressTransactions, rejectedMaxInFlight)
		return err
	}

	if txnHandlingError != nil {
		// Return the first TX creation or handling error
		return txnHandlingError
	}
	return nil
}

func action_NewSigningIdentity(ctx context.Context, c *coordinator, _ common.Event) error {
	c.signingIdentity.value = fmt.Sprintf("domains.%s.submit.%s", c.contractAddress.String(), uuid.New())
	c.signingIdentity.used = false
	log.L(ctx).Debugf("new signing identity: %s", c.signingIdentity.value)
	return nil
}

func action_SelectTransaction(ctx context.Context, c *coordinator, _ common.Event) error {
	// Select our next transaction. May return nothing if a different transaction is currently being assembled.
	return c.selectNextTransactionToAssemble(ctx)
}

func (c *coordinator) selectNextTransactionToAssemble(ctx context.Context) error {
	log.L(ctx).Trace("selecting next transaction to assemble")
	txn := c.popNextPooledTransaction()
	if txn == nil {
		log.L(ctx).Info("no transaction found to process")
		return nil
	}

	transactionSelectedEvent := &transaction.SelectedEvent{}
	transactionSelectedEvent.TransactionID = txn.GetID()
	err := txn.HandleEvent(ctx, transactionSelectedEvent)
	return err

}

func (c *coordinator) addTransactionToBackOfPool(txn transaction.CoordinatorTransaction) {
	// Check if transaction is already in the pool
	// This makes the function safe to call multiple times, albeit not strictly idempotently
	for _, pooledTxn := range c.pooledTransactions {
		if pooledTxn.GetID() == txn.GetID() {
			return
		}
	}
	c.pooledTransactions = append(c.pooledTransactions, txn)
}

func (c *coordinator) popNextPooledTransaction() transaction.CoordinatorTransaction {
	if len(c.pooledTransactions) == 0 {
		return nil
	}
	nextPooledTx := c.pooledTransactions[0]
	c.pooledTransactions[0] = nil // clear reference so the backing array doesn't pin the transaction from GC
	c.pooledTransactions = c.pooledTransactions[1:]
	return nextPooledTx
}

func (c *coordinator) removeTransactionFromPool(id uuid.UUID) {
	for i, txn := range c.pooledTransactions {
		if txn.GetID() == id {
			c.pooledTransactions[i] = nil
			c.pooledTransactions = append(c.pooledTransactions[:i], c.pooledTransactions[i+1:]...)
			return
		}
	}
}

func validator_TransactionStateTransitionFrom(states ...transaction.State) statemachine.Validator[*coordinator] {
	return func(ctx context.Context, _ *coordinator, event common.Event) (bool, error) {
		e := event.(*common.TransactionStateTransitionEvent[transaction.State])
		for _, s := range states {
			if e.FromState == s {
				return true, nil
			}
		}
		return false, nil
	}
}

func validator_TransactionStateTransitionTo(states ...transaction.State) statemachine.Validator[*coordinator] {
	return func(ctx context.Context, _ *coordinator, event common.Event) (bool, error) {
		e := event.(*common.TransactionStateTransitionEvent[transaction.State])
		for _, s := range states {
			if e.ToState == s {
				return true, nil
			}
		}
		return false, nil
	}
}

func action_PoolTransaction(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	// For pooled transactions, when we are pooling (or re-pooling) we push the transaction
	// to the back of the queue to give best-effort FIFO assembly as transactions arrive at the
	// node. If a transaction needs re-assembly after a revert, it will be processed after
	// a new transaction that hasn't ever been assembled.
	txn := c.transactionsByID[e.TransactionID]
	if txn != nil {
		c.addTransactionToBackOfPool(txn)
	}
	return nil
}

func action_QueueTransactionForDispatch(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	txn := c.transactionsByID[e.TransactionID]
	if txn != nil {
		select {
		case c.dispatchQueue <- txn:
		case <-ctx.Done():
		}
	}
	return nil
}

func action_CleanUpTransactionsNotYetDispatched(ctx context.Context, c *coordinator, _ common.Event) error {
	txns := c.getTransactionsNotInStates(ctx, []transaction.State{
		transaction.State_Dispatched,
		transaction.State_Confirmed,
		transaction.State_Reverted,
	})
	for _, txn := range txns {
		c.cleanUpTransaction(ctx, txn.GetID())
	}
	// Drain any Ready_For_Dispatch items still sitting in the dispatch channel.
	// The dispatch loop is guaranteed to be stopped before this action runs (either by an
	// explicit action_StopDispatchLoop earlier in the same sequence, or by State_Active's
	// OnTransitionFrom no-op stop). Consuming these references here prevents a future
	// dispatch loop — started when this node is re-elected — from processing stale
	// CoordinatorTransaction objects whose IDs are no longer in transactionsByID.
	for {
		select {
		case <-c.dispatchQueue:
		default:
			return nil
		}
	}
}

func action_CleanUpTransaction(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	c.cleanUpTransaction(ctx, e.TransactionID)
	return nil
}

func (c *coordinator) cleanUpTransaction(ctx context.Context, txID uuid.UUID) {
	txn := c.transactionsByID[txID]
	if txn != nil {
		delete(c.transactionsByID, txID)
		// this is a no-op if the transaction is not in the pool
		c.removeTransactionFromPool(txID)
		c.metrics.DecCoordinatingTransactions()
		c.grapher.ForgetTransactionAndLocks(ctx, txID)
		c.dependencyTracker.Delete(ctx, txID)
		// TODO: this is a workaround until https://github.com/LFDT-Paladin/paladin/issues/1247 is resolved.
		// The domain context is still holding onto the transaction locks even though it is no longer
		// responsible for them when running in a long-lived mode just for flushing private state data.
		c.dCtx.ResetTransactions(txID)
		log.L(ctx).Debugf("transaction %s cleaned up", txID.String())
	}
}

func action_cancelCurrentlyAssemblingTransaction(ctx context.Context, c *coordinator, _ common.Event) error {
	log.L(ctx).Debug("cancelling any transaction currently being assembled")
	assemblingTransactions := c.getTransactionsInStates(ctx, []transaction.State{
		transaction.State_Assembling,
	})
	if len(assemblingTransactions) > 0 {
		log.L(ctx).Debugf("cancelling assembling transaction: %s", assemblingTransactions[0].GetID().String())
		err := assemblingTransactions[0].HandleEvent(ctx, &transaction.AssembleCancelledEvent{
			BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
				TransactionID: assemblingTransactions[0].GetID(),
			},
		})
		return err
	}
	return nil
}

func validator_HeartBeatState(state ...common.CoordinatorState) statemachine.Validator[*coordinator] {
	return func(_ context.Context, c *coordinator, event common.Event) (bool, error) {
		e := event.(*common.HeartbeatReceivedEvent)
		for _, s := range state {
			if e.CoordinatorSnapshot.CoordinatorState == s {
				return true, nil
			}
		}
		return false, nil
	}
}

// validator_IsHeartbeatFromHigherPriorityCoordinator returns true when a heartbeat is from a node
// that is higher-priority than this node in the coordinator priority list.
func validator_IsHeartbeatFromHigherPriorityCoordinator(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*common.HeartbeatReceivedEvent)
	return common.IsHigherPriority(c.coordinatorPriorityList, e.FromNode, c.nodeName), nil
}
