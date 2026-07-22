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

// action_ProcessRevertedTransactions notifies originator transactions of any reverts
// included in the heartbeat snapshot. Only runs for heartbeats from the current coordinator.
func action_ProcessRevertedTransactions(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*common.HeartbeatReceivedEvent)
	return o.processRevertedTransactions(ctx, e)
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
// Transactions not found in our in-memory map belong to other originators and are silently skipped.
func (o *originator) processDispatchedTransactions(ctx context.Context, event *common.HeartbeatReceivedEvent) error {
	for _, dispatchedTransaction := range event.CoordinatorSnapshot.DispatchedTransactions {
		txn := o.transactionsByID[dispatchedTransaction.ID]
		if txn == nil {
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

// processConfirmedTransactions notifies originator transactions of any on-chain successes
// included in the heartbeat snapshot, regardless of coordinator state.
// Coordinator State_Confirmed is only reached via success; revert reason is never present here.
// Transactions not found in our in-memory map belong to other originators and are silently skipped.
func (o *originator) processConfirmedTransactions(ctx context.Context, event *common.HeartbeatReceivedEvent) error {
	for _, confirmedTransaction := range event.CoordinatorSnapshot.ConfirmedTransactions {
		txn := o.transactionsByID[confirmedTransaction.ID]
		if txn == nil {
			continue
		}
		err := txn.HandleEvent(ctx, &transaction.ConfirmedSuccessEvent{
			BaseEvent: transaction.BaseEvent{TransactionID: confirmedTransaction.ID},
		})
		if err != nil {
			msg := fmt.Errorf("error handling confirmed success event for transaction %s: %v", txn.GetID(), err)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}
	return nil
}

// processRevertedTransactions notifies originator transactions of any reverts included in the
// heartbeat snapshot from the current coordinator. Only the raw on-chain revert bytes are
// propagated; the coordinator does not send failure message as a transaction reverted at assembly time
// may have private data in its failure message.
func (o *originator) processRevertedTransactions(ctx context.Context, event *common.HeartbeatReceivedEvent) error {
	for _, revertedTransaction := range event.CoordinatorSnapshot.RevertedTransactions {
		txn := o.transactionsByID[revertedTransaction.ID]
		if txn == nil {
			continue
		}
		err := txn.HandleEvent(ctx, &transaction.ConfirmedRevertedEvent{
			BaseEvent:    transaction.BaseEvent{TransactionID: revertedTransaction.ID},
			RevertReason: revertedTransaction.RevertReason,
			WillRetry:    false,
		})
		if err != nil {
			msg := fmt.Errorf("error handling confirmed reverted event for transaction %s: %v", txn.GetID(), err)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}
	return nil
}

// hasDroppedTransactions returns true if any transaction that should be in-flight on the coordinator
// is absent from its snapshot, implying the coordinator has dropped it and we need to redelegate
// everything.
func (o *originator) hasDroppedTransactions(ctx context.Context, snapshot *common.CoordinatorSnapshot) bool {
	// Two groups of transactions are excluded from the dropped check:
	//   - pre-delegation states (Initial, Resolving, Pending): never sent to a coordinator, so their
	//     absence from a snapshot is expected and does not indicate a drop.
	//   - terminal states (Confirmed, Reverted, Final): although a coordinator does report these in its
	//     snapshot, they have already been finalized so there is no purpose in redelegating them.
	excludedStates := []transaction.State{
		transaction.State_Initial,
		transaction.State_Resolving,
		transaction.State_Pending,
		transaction.State_Confirmed,
		transaction.State_Reverted,
		transaction.State_Final,
	}
	for _, txn := range o.getTransactionsNotInStates(excludedStates) {
		// A freshly-delegated transaction races the coordinator's snapshot: the snapshot we are checking
		// against may have been generated before the delegation reached the coordinator, so its absence is
		// expected rather than a drop. Only consider a transaction dropped once at least one full heartbeat
		// interval has elapsed since it was first delegated to the current coordinator, by which point that
		// coordinator has had the chance to include it in a snapshot. The first-delegation time is reset if
		// the transaction is redirected to a different coordinator, so the grace restarts on each handover
		// but is not extended by the partial FIFO resend to the same coordinator.
		firstDelegated := txn.GetFirstDelegatedTime()
		if firstDelegated == nil || o.clock.Now().Sub(*firstDelegated) < o.heartbeatInterval {
			continue
		}
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
	for _, t := range snapshot.RevertedTransactions {
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
