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
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/google/uuid"
)

func action_TransactionCreated(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*TransactionCreatedEvent)
	return o.addToTransactions(ctx, e.Transaction, o.newOriginatorTransaction)
}

func (o *originator) newOriginatorTransaction(ctx context.Context, pt *components.PrivateTransaction) (transaction.OriginatorTransaction, error) {
	return transaction.NewTransaction(ctx, pt, o.transportWriter, o.queueEventInternal, o.engineIntegration, o.metrics)
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
	if o.activeCoordinatorNode == "" {
		// the delegation timeout loop ensures that this request will be retried when we have an active coordinator
		log.L(ctx).Debugf("no active coordinator set yet; deferring delegation for contract %s", o.contractAddress.String())
		return nil
	}

	// Re-delegate all transactions. Every delegation request must include all transaction, sent in the order they were created
	// on the originating node.

	// Note: we could track assemble errors here (not reverts, but domain bugs that prevent successful assembly) and penalise
	// transactions who assemble at error time by putting them to the back of the list. The coordinator already gives a limited
	// number of retries in such scenarios so the current worst case is a slightly delay before we give up on the failing TX anyway
	// so for now we just re-delegate all transactions in their original order.

	// Update internal TX state machines before sending delegation requests to avoid race condition
	transactionsToDelegate := make([]*components.PrivateTransaction, 0)
	for _, txn := range o.transactionsOrdered {
		transactionsToDelegate = append(transactionsToDelegate, txn.GetPrivateTransaction())
		err := txn.HandleEvent(ctx, &transaction.DelegatedEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: txn.GetID(),
			},
			Coordinator: o.activeCoordinatorNode,
		})
		if err != nil {
			msg := fmt.Errorf("error handling delegated event for transaction %s: %v", txn.GetID(), err)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}

	log.L(ctx).Debugf("sending delegation request for %d transactions", len(o.transactionsOrdered))

	// Don't send delegation request before internal TX state machine has been updated
	return o.transportWriter.SendDelegationRequest(ctx, o.activeCoordinatorNode, transactionsToDelegate, o.currentBlockHeight)
}

func action_SendDelegationRequest(ctx context.Context, o *originator, _ common.Event) error {
	return sendDelegationRequest(ctx, o)
}

func guard_HasDroppedTransactions(ctx context.Context, o *originator) bool {
	// Are there any transactions that the current active coordinator seems to have dropped (as per its latest heartbeat)?
	// NOTE: "dropped" is not a state in the transaction state machine, but rather a description of the originator's view of the world
	// based on the heartbeats it receives from coordinators.
	for _, txn := range o.getTransactionsNotInStates([]transaction.State{transaction.State_Final, transaction.State_Confirmed, transaction.State_Reverted}) {
		// If any one of the transactions has been dropped, re-delegate everything
		if !transactionFoundInHeartbeat(o, txn) {
			log.L(ctx).Debugf("transaction %s is in Delegated state but not found in latest coordinator snapshot, assuming dropped", txn.GetID())
			return true
		}
	}
	return false
}

func transactionFoundInHeartbeat(o *originator, txn transaction.OriginatorTransaction) bool {
	for _, dispatchedTransaction := range o.latestCoordinatorSnapshot.DispatchedTransactions {
		if dispatchedTransaction.ID == txn.GetID() {
			return true
		}
	}
	for _, dispatchedTransaction := range o.latestCoordinatorSnapshot.PooledTransactions {
		if dispatchedTransaction.ID == txn.GetID() {
			return true
		}
	}
	for _, dispatchedTransaction := range o.latestCoordinatorSnapshot.ConfirmedTransactions {
		if dispatchedTransaction.ID == txn.GetID() {
			return true
		}
	}
	return false
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
	return e.To == transaction.State_Final, nil
}

func action_CleanUpTransaction(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	o.removeTransaction(ctx, e.TransactionID)
	return nil
}

func validator_OriginatorTransactionStateTransitionToConfirmed(ctx context.Context, _ *originator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.To == transaction.State_Confirmed, nil
}

func validator_OriginatorTransactionStateTransitionToReverted(ctx context.Context, _ *originator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.To == transaction.State_Reverted, nil
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

func action_ActiveCoordinatorUpdated(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*ActiveCoordinatorUpdatedEvent)
	if e.Coordinator == "" {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot set active coordinator to an empty string")
	}
	o.activeCoordinatorNode = e.Coordinator
	log.L(ctx).Debugf("active coordinator updated to %s", e.Coordinator)
	return nil
}

func guard_RedelegateThresholdExceeded(_ context.Context, o *originator) bool {
	if o.timeOfMostRecentHeartbeat == nil {
		//we have never seen a heartbeat so that was a really long time ago, certainly longer than any threshold
		return true
	}
	return o.clock.HasExpired(*o.timeOfMostRecentHeartbeat, time.Duration(o.redelegateThreshold)*o.heartbeatInterval)
}
