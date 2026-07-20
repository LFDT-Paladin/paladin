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

	"slices"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/google/uuid"
)

func validator_IsDelegationBlockHeightRejection(_ context.Context, _ *originator, event common.Event) (bool, error) {
	return event.(*DelegationRequestRejectedEvent).RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, nil
}

func validator_IsDelegationNotActiveCoordinatorRejection(_ context.Context, _ *originator, event common.Event) (bool, error) {
	return event.(*DelegationRequestRejectedEvent).RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE, nil
}

func action_LogDelegationBlockHeightRejection(ctx context.Context, _ *originator, event common.Event) error {
	e := event.(*DelegationRequestRejectedEvent)
	log.L(ctx).Warnf("delegation rejected due to block height tolerance exceeded: originator block height=%d, coordinator block height=%d, coordinator tolerance=%d",
		e.OriginatorBlockHeight, e.CoordinatorBlockHeight, e.BlockHeightTolerance)
	return nil
}

func action_TransactionCreated(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*TransactionCreatedEvent)
	return o.addToTransactions(ctx, e.Transaction, o.newOriginatorTransaction)
}

// refreshBlockHeight queries the live block height, updates currentBlockHeight, and updates
// effectiveBlockHeight (recalculating coordinator priorities) when the epoch changes.
func (o *originator) refreshBlockHeight(ctx context.Context) {
	liveHeight := o.engineIntegration.GetBlockHeight(ctx)
	o.currentBlockHeight = liveHeight
	newEffective := common.ComputeEffectiveBlockHeight(uint64(liveHeight), o.blockRange)
	if newEffective != o.effectiveBlockHeight {
		o.effectiveBlockHeight = newEffective
		o.calculateCoordinatorPriorities(ctx)
	}
}

func (o *originator) newOriginatorTransaction(ctx context.Context, pt *components.PrivateTransaction) (transaction.OriginatorTransaction, error) {
	return transaction.NewTransaction(
		ctx,
		pt,
		o.transportWriter,
		o.queueEventInternal,
		o.engineIntegration,
		o.metrics,
		func(ctx context.Context) { o.refreshBlockHeight(ctx) },
		func() int64 { return o.currentBlockHeight },
	)
}

func (o *originator) addToTransactions(
	ctx context.Context,
	txn *components.PrivateTransaction,
	createTransaction func(
		ctx context.Context,
		pt *components.PrivateTransaction) (transaction.OriginatorTransaction, error)) error {
	newTxn, err := createTransaction(ctx, txn)
	if err != nil {
		log.L(ctx).Errorf("error creating transaction: %v", err)
		return err
	}
	o.transactionsByID[txn.ID] = newTxn
	o.transactionsOrdered = append(o.transactionsOrdered, newTxn)
	createdEvent := &transaction.CreatedEvent{}
	createdEvent.TransactionID = txn.ID
	err = newTxn.HandleEvent(ctx, createdEvent)
	if err != nil {
		log.L(ctx).Errorf("error handling CreatedEvent for transaction %s: %v", txn.ID.String(), err)
		return err
	}
	return nil
}

func sendDelegationRequest(ctx context.Context, o *originator) error {
	// Re-delegate all transactions in the order they were created on the originating node.
	transactionsToDelegate := make([]*components.PrivateTransaction, 0)
	for _, txn := range o.transactionsOrdered {
		transactionsToDelegate = append(transactionsToDelegate, txn.GetPrivateTransaction())
		err := txn.HandleEvent(ctx, &transaction.DelegatedEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: txn.GetID(),
			},
			Coordinator: o.currentActiveCoordinator,
		})
		if err != nil {
			msg := fmt.Errorf("error handling delegated event for transaction %s: %v", txn.GetID(), err)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}

	log.L(ctx).Debugf("sending delegation request for %d transactions", len(o.transactionsOrdered))

	delegations := make([]*engineProto.PrivateTransactionDelegation, 0, len(transactionsToDelegate))
	for _, tx := range transactionsToDelegate {
		delegations = append(delegations, &engineProto.PrivateTransactionDelegation{
			Id:          tx.ID.String(),
			Domain:      tx.Domain,
			Intent:      tx.Intent,
			PreAssembly: tx.PreAssembly,
		})
	}
	return o.transportWriter.SendDelegationRequest(ctx, o.currentActiveCoordinator, &engineProto.DelegationRequest{
		DelegateNodeId:        o.currentActiveCoordinator,
		OriginatorBlockHeight: int64(o.currentBlockHeight),
		ContractAddress:       o.contractAddress.HexString(),
		Transactions:          delegations,
	})
}

func action_SendDelegationRequest(ctx context.Context, o *originator, _ common.Event) error {
	return sendDelegationRequest(ctx, o)
}

// action_RefreshBlockHeight queries the live block height and updates effectiveBlockHeight and the
// priority list if the epoch has changed.
func action_RefreshBlockHeight(ctx context.Context, o *originator, _ common.Event) error {
	o.refreshBlockHeight(ctx)
	return nil
}

// resetFailoverIndex sets failoverIndex so the next failover walk step targets the
// highest-priority candidate that is not the current active coordinator. Called whenever
// currentActiveCoordinator changes via an external signal (heartbeat switch, rejection redirect,
// priority recalculation). Must NOT be called from action_FailoverToNextCoordinator.
//
// No-op unless the priority list has more than one entry (STATIC/SENDER modes and single-node
// ENDORSER pools cannot failover to a different coordinator).
func (o *originator) resetFailoverIndex() {
	if len(o.coordinatorPriorityList) <= 1 {
		return
	}
	if o.currentActiveCoordinator == o.coordinatorPriorityList[0] {
		o.failoverIndex = 1
	} else {
		o.failoverIndex = 0
	}
}

// action_FailoverToNextCoordinator advances currentActiveCoordinator to the next candidate in
// the priority list, increments failoverIndex (wrapping), resets the liveness counter, then
// delegates. If there is no alternative coordinator to failover to (single endorser or STATE/SENDER mode)
// this becomes the equivalent of a redelegate.
func action_FailoverToNextCoordinator(ctx context.Context, o *originator, _ common.Event) error {
	if len(o.coordinatorPriorityList) > 1 {
		prev := o.currentActiveCoordinator
		o.currentActiveCoordinator = o.coordinatorPriorityList[o.failoverIndex]
		o.failoverIndex = (o.failoverIndex + 1) % len(o.coordinatorPriorityList)
		o.heartbeatIntervalsSinceLastReceive = 0
		log.L(ctx).Debugf("originator failing over from %s to %s (failoverIndex now %d)",
			prev, o.currentActiveCoordinator, o.failoverIndex)
	}
	return sendDelegationRequest(ctx, o)
}

// action_ResetToTopPriorityCoordinator sets currentActiveCoordinator to the highest-priority
// candidate and recalibrates failoverIndex. Used when entering Idle and on epoch boundaries
// while Idle to ensure the next Sending entry starts from a fresh, highest-priority delegation
// target rather than a potentially stale one.
//
// No-op unless the priority list has more than one entry.
func action_ResetToTopPriorityCoordinator(ctx context.Context, o *originator, _ common.Event) error {
	if len(o.coordinatorPriorityList) <= 1 {
		return nil
	}
	prev := o.currentActiveCoordinator
	o.currentActiveCoordinator = o.coordinatorPriorityList[0]
	o.failoverIndex = 1
	if prev != o.currentActiveCoordinator {
		log.L(ctx).Debugf("originator reset active coordinator from %s to top priority %s",
			prev, o.currentActiveCoordinator)
	}
	return nil
}

func guard_InactiveGracePeriodExceeded(_ context.Context, o *originator) bool {
	// measure number of complete heartbeat interval periods - e.g. count of 2 means
	// 1 full heartbeat interval has elapsed, hence use of > not >=
	return o.heartbeatIntervalsSinceLastReceive > o.inactiveGracePeriod
}

// validator_IsFromCurrentCoordinator returns true when the heartbeat sender is the currently
// tracked active coordinator. Does not check liveness; used where identity alone is sufficient.
func validator_IsFromCurrentCoordinator(_ context.Context, o *originator, event common.Event) (bool, error) {
	e := event.(*common.HeartbeatReceivedEvent)
	return e.FromNode == o.currentActiveCoordinator, nil
}

// validator_IsSenderHigherPriorityThanCurrentCoordinator returns true when the heartbeat sender
// has a lower priority index (higher priority) than the currently tracked active coordinator.
func validator_IsSenderHigherPriorityThanCurrentCoordinator(_ context.Context, o *originator, event common.Event) (bool, error) {
	e := event.(*common.HeartbeatReceivedEvent)
	return common.IsHigherPriority(o.coordinatorPriorityList, e.FromNode, o.currentActiveCoordinator), nil
}

// validator_HasDroppedTransactions returns true when the heartbeat snapshot is missing at least one
// transaction that we believe is still in-flight, indicating the coordinator has dropped it.
func validator_HasDroppedTransactions(ctx context.Context, o *originator, event common.Event) (bool, error) {
	e := event.(*common.HeartbeatReceivedEvent)
	return o.hasDroppedTransactions(ctx, e.CoordinatorSnapshot), nil
}

func action_ResetHeartbeatIntervalsSinceLastReceive(_ context.Context, o *originator, _ common.Event) error {
	o.heartbeatIntervalsSinceLastReceive = 0
	return nil
}

// Validate that the transaction doesn't already exist. When we resume transactions from the DB, e.g. after a restart or a timeout, we may already be processing
// the transaction and possibly taking a long time to complete them so we shouldn't restart the state machine from scratch for such in-progress transactions
func validator_TransactionDoesNotExist(ctx context.Context, o *originator, event common.Event) (bool, error) {
	transactionCreatedEvent, ok := event.(*TransactionCreatedEvent)
	if !ok {
		log.L(ctx).Errorf("expected event type *TransactionCreatedEvent, got %T", event)
		return false, nil
	}
	if transactionCreatedEvent.Transaction == nil {
		// If transaction is nil, let createTransaction handle the error
		return true, nil
	}
	if o.transactionsByID[transactionCreatedEvent.Transaction.ID] != nil {
		log.L(ctx).Debugf("transaction %s already in progress, not resuming", transactionCreatedEvent.Transaction.ID.String())
		return false, nil
	}

	return true, nil
}

func validator_OriginatorTransactionStateTransitionToFinal(ctx context.Context, _ *originator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.ToState == transaction.State_Final, nil
}

func action_CleanUpTransaction(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	o.removeTransaction(ctx, e.TransactionID)
	return nil
}

func validator_OriginatorTransactionStateTransitionToConfirmed(ctx context.Context, _ *originator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.ToState == transaction.State_Confirmed, nil
}

func validator_OriginatorTransactionStateTransitionToReverted(ctx context.Context, _ *originator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.ToState == transaction.State_Reverted, nil
}

func action_FinalizeTransaction(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	o.queueEventInternal(ctx, &transaction.FinalizeEvent{
		BaseEvent:     common.BaseEvent{EventTime: e.GetEventTime()},
		TransactionID: e.TransactionID,
	})
	return nil
}

func (o *originator) removeTransaction(ctx context.Context, txnID uuid.UUID) {
	log.L(ctx).Debugf("removing transaction %s from originator", txnID.String())

	// Remove from transactionsByID
	delete(o.transactionsByID, txnID)

	// Remove from transactionsOrdered
	for i, txn := range o.transactionsOrdered {
		if txn.GetID() == txnID {
			o.transactionsOrdered = append(o.transactionsOrdered[:i], o.transactionsOrdered[i+1:]...)
			break
		}
	}
}

func action_CalculateCoordinatorPriorities(ctx context.Context, o *originator, _ common.Event) error {
	o.calculateCoordinatorPriorities(ctx)
	return nil
}

// calculateCoordinatorPriorities recomputes coordinatorPriorityList from the current
// endorserCandidates and effectiveBlockHeight. No-op when endorserCandidates is empty (STATIC/SENDER modes).
func (o *originator) calculateCoordinatorPriorities(ctx context.Context) {
	if len(o.endorserCandidates) == 0 {
		return
	}
	o.coordinatorPriorityList = common.ComputeCoordinatorPriorityList(
		ctx,
		o.endorserCandidates,
		o.effectiveBlockHeight,
	)
	if o.currentActiveCoordinator == "" {
		// this should really only run on start up - after that we never unset the current active coordinator
		o.currentActiveCoordinator = o.coordinatorPriorityList[0]
	}
	// whenever we have a new coordinator priority list we want to make sure that a failover walk through the
	// list, if required, starts from the beginning
	o.resetFailoverIndex()
}

// action_UpdateEndorserCandidates replaces the endorser candidates with the full sorted+deduped
// list sent by the coordinator. The coordinator copies the slice before sending so the originator
// can assign it directly without aliasing the coordinator's internal state.
func action_UpdateEndorserCandidates(_ context.Context, o *originator, event common.Event) error {
	e := event.(*common.EndorserNodesDiscoveredEvent)
	o.endorserCandidates = e.Nodes
	return nil
}

// action_UpdateEndorserCandidatesFromHeartbeat merges the heartbeat sender's known endorser pool
// into the local candidate pool, then recomputes the priority list. This runs as handler 0 for
// all HeartbeatReceived events so that subsequent handlers (e.g.
// validator_IsSenderHigherPriorityThanCurrentCoordinator) see an up-to-date list — important
// because the heartbeat is queued to the originator before the coordinator's
// EndorserNodesDiscoveredEvent notification arrives.
func action_UpdateEndorserCandidatesFromHeartbeat(ctx context.Context, o *originator, event common.Event) error {
	if len(o.endorserCandidates) == 0 {
		return nil // STATIC/SENDER mode: endorserCandidates is empty
	}
	e := event.(*common.HeartbeatReceivedEvent)
	candidates := []string{e.FromNode}
	if e.CoordinatorSnapshot != nil && len(e.CoordinatorSnapshot.EndorserCandidates) > 0 {
		candidates = e.CoordinatorSnapshot.EndorserCandidates
	}
	changed := false
	for _, node := range candidates {
		if !slices.Contains(o.endorserCandidates, node) {
			o.endorserCandidates = append(o.endorserCandidates, node)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	slices.Sort(o.endorserCandidates)
	o.coordinatorPriorityList = common.ComputeCoordinatorPriorityList(
		ctx,
		o.endorserCandidates,
		o.effectiveBlockHeight,
	)
	o.resetFailoverIndex()
	return nil
}

// action_UpdateActiveCoordinatorFromHeartbeat records the heartbeat sender as the current active
// coordinator. Called only when the sender is in Active or Elect state.
func action_UpdateActiveCoordinatorFromHeartbeat(_ context.Context, o *originator, event common.Event) error {
	e := event.(*common.HeartbeatReceivedEvent)
	o.currentActiveCoordinator = e.FromNode
	o.resetFailoverIndex()
	return nil
}

// action_HandleDelegationRejected processes a rejection from a coordinator. If the rejection names
// a coordinator that has higher priority than our current one, we redirect to it
func action_HandleDelegationRejected(_ context.Context, o *originator, event common.Event) error {
	e := event.(*DelegationRequestRejectedEvent)
	if e.ActiveCoordinator == "" {
		return nil
	}
	if common.IsHigherPriority(o.coordinatorPriorityList, e.ActiveCoordinator, o.currentActiveCoordinator) {
		o.currentActiveCoordinator = e.ActiveCoordinator
		o.resetFailoverIndex()
	}
	return nil
}

// validator_IsHeartbeatSenderLive returns true when the heartbeat sender reports being in one of
// the liveness-proving states: Elect, Prepared, Active, or Active_Flush.
func validator_IsHeartbeatSenderLive(_ context.Context, _ *originator, event common.Event) (bool, error) {
	e := event.(*common.HeartbeatReceivedEvent)
	s := e.CoordinatorSnapshot.CoordinatorState
	return s == common.CoordinatorState_Elect ||
		s == common.CoordinatorState_Prepared ||
		s == common.CoordinatorState_Active ||
		s == common.CoordinatorState_Active_Flush, nil
}

func guard_HasTransactions(ctx context.Context, o *originator) bool {
	return len(o.transactionsByID) > 0
}
