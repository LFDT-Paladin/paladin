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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
)

func sendDelegationRequest(ctx context.Context, o *originator, includeAlreadyDelegated bool, ignoreDelegateTimeout bool) error {
	// Find pending transactions only and (optionally) already delegated transactions
	privateTransactions := make([]*components.PrivateTransaction, 0)
	transactionsToDelegate := make([]*transaction.Transaction, 0)
	for _, txn := range o.transactionsOrdered {
		if includeAlreadyDelegated && txn.GetCurrentState() == transaction.State_Delegated && (ignoreDelegateTimeout || (txn.GetLastDelegatedTime() != nil && common.RealClock().HasExpired(*txn.GetLastDelegatedTime(), o.delegateTimeout))) {
			// only re-delegate after the delegate timeout
			privateTransactions = append(privateTransactions, txn.GetPrivateTransaction())
			transactionsToDelegate = append(transactionsToDelegate, txn)
		}

		if txn.GetCurrentState() == transaction.State_Pending {
			privateTransactions = append(privateTransactions, txn.GetPrivateTransaction())
			transactionsToDelegate = append(transactionsToDelegate, txn)
		}
	}

	// Update internal TX state machines before sending delegation requests to avoid race condition
	for _, txn := range transactionsToDelegate {
		err := txn.HandleEvent(ctx, &transaction.DelegatedEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: txn.GetID(),
			},
			Coordinator: o.activeCoordinatorNode,
		})
		if err != nil {
			msg := fmt.Sprintf("error handling delegated event for transaction %s: %v", txn.GetID(), err)
			log.L(ctx).Error(msg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}

	// Don't send delegation request before internal TX state machine has been updated
	return o.transportWriter.SendDelegationRequest(ctx, o.activeCoordinatorNode, privateTransactions, o.currentBlockHeight)
}

func action_SendDroppedTXDelegationRequest(ctx context.Context, o *originator) error {
	return sendDelegationRequest(ctx, o, true, true)
}

func action_ResendTimedOutDelegationRequest(ctx context.Context, o *originator) error {
	return sendDelegationRequest(ctx, o, true, false)
}

func action_SendDelegationRequest(ctx context.Context, o *originator) error {
	return sendDelegationRequest(ctx, o, false, false)
}

func guard_HasDroppedTransactions(ctx context.Context, o *originator) bool {
	//are there any transactions that the current active coordinator seems to have dropped ( as per its latest heartbeat)
	//NOTE: "dropped" is not a state in the transaction state machine, but rather a state in the originator's view of the world.
	// Reason for this is that it is not really a state of the transaction, it is a property of the heartbeat event and as such,
	// is reconciled as part of handling that event so immediately, the transaction is in Delegated state again
	for _, txn := range o.getTransactionsInStates([]transaction.State{transaction.State_Delegated}) {
		dropped := true
		for _, dispatchedTransaction := range o.latestCoordinatorSnapshot.PooledTransactions {
			if dispatchedTransaction.ID == txn.GetID() {
				dropped = false
				break
			}
		}
		if dropped {
			log.L(ctx).Debugf("transaction %s is in Delegated state but not found in latest coordinator snapshot, assuming dropped", txn.GetID())
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
