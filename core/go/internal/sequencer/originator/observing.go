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
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
)

// action_ProcessConfirmedTransactions notifies originator transactions of any confirmations
// included in the heartbeat snapshot. This runs unconditionally for every heartbeat regardless
// of the sender's state or identity.
func action_ProcessConfirmedTransactions(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*common.HeartbeatReceivedEvent)
	return o.processConfirmedTransactions(ctx, e)
}

// action_ProcessCurrentCoordinatorHeartbeat handles a live heartbeat from the currently tracked
// coordinator. It resets the liveness timer and propagates dispatch state updates to local
// transaction state machines. Dropped-transaction detection is handled separately in
// the heartbeat action chain, so the two concerns remain independently observable.
func action_ProcessCurrentCoordinatorHeartbeat(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*common.HeartbeatReceivedEvent)
	o.heartbeatIntervalsSinceLastReceive = 0
	return o.processDispatchedTransactions(ctx, e)
}

func action_SwitchActiveCoordinator(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*common.HeartbeatReceivedEvent)
	log.L(ctx).Debugf("switching active coordinator from %s to %s", o.currentActiveCoordinator, e.FromNode)
	o.currentActiveCoordinator = e.FromNode
	o.resetFailoverIndex()
	o.heartbeatIntervalsSinceLastReceive = 0
	return nil
}

// processDispatchedTransactions propagates dispatch state updates (submission hash, nonce)
// from the active coordinator's heartbeat to our local transaction state machines.
func (o *originator) processDispatchedTransactions(ctx context.Context, event *common.HeartbeatReceivedEvent) error {
	for _, dispatchedTransaction := range event.CoordinatorSnapshot.DispatchedTransactions {
		if dispatchedTransaction.Originator != o.nodeName {
			continue
		}
		txn := o.transactionsByID[dispatchedTransaction.ID]
		if txn == nil {
			log.L(ctx).Warnf("received heartbeat from %s with dispatched transaction %s but no transaction found in memory",
				o.currentActiveCoordinator, dispatchedTransaction.ID)
			continue
		}
		if dispatchedTransaction.LatestSubmissionHash != nil {
			txnSubmittedEvent := &transaction.SubmittedEvent{}
			txnSubmittedEvent.TransactionID = dispatchedTransaction.ID
			txnSubmittedEvent.SignerAddress = dispatchedTransaction.Signer
			txnSubmittedEvent.LatestSubmissionHash = *dispatchedTransaction.LatestSubmissionHash
			txnSubmittedEvent.Coordinator = event.FromNode
			if dispatchedTransaction.Nonce != nil {
				txnSubmittedEvent.Nonce = *dispatchedTransaction.Nonce
			}
			if err := txn.HandleEvent(ctx, txnSubmittedEvent); err != nil {
				msg := fmt.Errorf("error handling transaction submitted event for transaction %s: %v", txn.GetID(), err)
				return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
			}
		} else if dispatchedTransaction.Nonce != nil {
			err := txn.HandleEvent(ctx, &transaction.NonceAssignedEvent{
				BaseEvent:   transaction.BaseEvent{TransactionID: dispatchedTransaction.ID},
				Nonce:       *dispatchedTransaction.Nonce,
				Coordinator: event.FromNode,
			})
			if err != nil {
				msg := fmt.Errorf("error handling nonce assigned event for transaction %s: %v", txn.GetID(), err)
				return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
			}
		}
	}
	return nil
}

// processConfirmedTransactions notifies originator transactions of any confirmations
// included in the heartbeat snapshot, regardless of sender or state.
func (o *originator) processConfirmedTransactions(ctx context.Context, event *common.HeartbeatReceivedEvent) error {
	for _, confirmedTransaction := range event.CoordinatorSnapshot.ConfirmedTransactions {
		if confirmedTransaction.Originator != o.nodeName {
			continue
		}
		txn := o.transactionsByID[confirmedTransaction.ID]
		if txn == nil {
			log.L(ctx).Debugf("received confirmed transaction %s in heartbeat from %s but no transaction found in memory",
				confirmedTransaction.ID, event.FromNode)
			continue
		}
		if len(confirmedTransaction.RevertReason) > 0 {
			err := txn.HandleEvent(ctx, &transaction.ConfirmedRevertedEvent{
				BaseEvent:    transaction.BaseEvent{TransactionID: confirmedTransaction.ID},
				RevertReason: confirmedTransaction.RevertReason,
				WillRetry:    false,
			})
			if err != nil {
				msg := fmt.Errorf("error handling confirmed reverted event for transaction %s: %v", txn.GetID(), err)
				return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
			}
		} else {
			err := txn.HandleEvent(ctx, &transaction.ConfirmedSuccessEvent{
				BaseEvent: transaction.BaseEvent{TransactionID: confirmedTransaction.ID},
			})
			if err != nil {
				msg := fmt.Errorf("error handling confirmed success event for transaction %s: %v", txn.GetID(), err)
				return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
			}
		}
	}
	return nil
}

// hasDroppedTransactions returns true if any non-final transaction is absent from the coordinator's
// snapshot, implying the coordinator has dropped it and we need to redelegate everything.
func (o *originator) hasDroppedTransactions(ctx context.Context, snapshot *common.CoordinatorSnapshot) bool {
	for _, txn := range o.getTransactionsNotInStates([]transaction.State{transaction.State_Final, transaction.State_Confirmed, transaction.State_Reverted}) {
		if !transactionFoundInSnapshot(snapshot, txn) {
			log.L(ctx).Debugf("transaction %s not found in latest coordinator snapshot, assuming dropped", txn.GetID())
			return true
		}
	}
	return false
}

func transactionFoundInSnapshot(snapshot *common.CoordinatorSnapshot, txn transaction.OriginatorTransaction) bool {
	for _, t := range snapshot.DispatchedTransactions {
		if t.ID == txn.GetID() {
			return true
		}
	}
	for _, t := range snapshot.PooledTransactions {
		if t.ID == txn.GetID() {
			return true
		}
	}
	for _, t := range snapshot.ConfirmedTransactions {
		if t.ID == txn.GetID() {
			return true
		}
	}
	return false
}

func action_IncrementHeartbeatIntervalCounts(_ context.Context, o *originator, _ common.Event) error {
	o.heartbeatIntervalsSinceLastReceive++
	return nil
}
