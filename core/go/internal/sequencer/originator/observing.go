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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
)

func action_HeartbeatReceived(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*HeartbeatReceivedEvent)
	return o.applyHeartbeatReceived(ctx, e)
}

func (o *originator) applyHeartbeatReceived(ctx context.Context, event *HeartbeatReceivedEvent) error {
	o.timeOfMostRecentHeartbeat = o.clock.Now()
	o.activeCoordinatorNode = event.From
	o.latestCoordinatorSnapshot = &event.CoordinatorSnapshot
	for _, dispatchedTransaction := range event.DispatchedTransactions {
		o.applyDispatchedSnapshot(ctx, dispatchedTransaction)
	}

	for _, confirmedTransaction := range event.ConfirmedTransactions {
		o.applyConfirmedSnapshot(ctx, confirmedTransaction)
	}

	// Note: sending dropped transaction re-delegations (i.e. those we are tracking but which the heartbeat doesn't mention)
	// is handled by state machine guards

	return nil
}

func (o *originator) applyDispatchedSnapshot(ctx context.Context, dispatchedTransaction *common.DispatchedTransaction) error {
	//if any of the dispatched transactions were sent by this originator, ensure that we have an up to date view of its state
	if dispatchedTransaction.Originator != o.nodeName {
		return nil
	}
	txn := o.transactionsByID[dispatchedTransaction.ID]
	if txn == nil {
		//unexpected situation to be in.  We trust our memory of transactions over the coordinator's, so we ignore this transaction
		log.L(ctx).Warnf("received heartbeat from %s with dispatched transaction %s but no transaction found in memory", o.activeCoordinatorNode, dispatchedTransaction.ID)
		return nil
	}

	if dispatchedTransaction.LatestSubmissionHash != nil {
		o.submittedTransactionsByHash[*dispatchedTransaction.LatestSubmissionHash] = &dispatchedTransaction.ID
	}

	event := o.buildDispatchedSnapshotEvent(dispatchedTransaction)
	if event != nil {
		// the events we can be handling here never return errors, but even if they did there's
		// no recovery action we can take when applying a heartbeat. If errors do occur they will
		// be logged by the common state machine code.
		_ = txn.HandleEvent(ctx, event)
	}
	return nil
}

func (o *originator) buildDispatchedSnapshotEvent(dispatchedTransaction *common.DispatchedTransaction) transaction.Event {
	if dispatchedTransaction.LatestSubmissionHash != nil {
		//if the dispatched transaction has a hash, then we can update our view of the transaction
		txnSubmittedEvent := &transaction.SubmittedEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: dispatchedTransaction.ID,
			},
			SignerAddress:        dispatchedTransaction.Signer,
			LatestSubmissionHash: *dispatchedTransaction.LatestSubmissionHash,
		}
		if dispatchedTransaction.Nonce != nil {
			txnSubmittedEvent.Nonce = *dispatchedTransaction.Nonce
		}
		return txnSubmittedEvent
	}

	if dispatchedTransaction.Nonce != nil {
		//if the dispatched transaction has a nonce but no hash, then it is sequenced
		return &transaction.NonceAssignedEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: dispatchedTransaction.ID,
			},
			SignerAddress: dispatchedTransaction.Signer,
			Nonce:         *dispatchedTransaction.Nonce,
		}
	}

	return nil
}

func (o *originator) applyConfirmedSnapshot(ctx context.Context, confirmedTransaction *common.ConfirmedTransaction) error {
	if confirmedTransaction.Originator != o.nodeName {
		return nil
	}
	txn := o.transactionsByID[confirmedTransaction.ID]
	if txn == nil {
		// we expect this to happen since we remove the transaction from memory as soon as it reaches a final state but the
		// coordinator will include it for a number of heartbeats after it reaches a final state.
		return nil
	}

	if confirmedTransaction.LatestSubmissionHash != nil {
		delete(o.submittedTransactionsByHash, *confirmedTransaction.LatestSubmissionHash)
	}

	event := buildConfirmedSnapshotEvent(confirmedTransaction)
	// the events we can be handling here never return errors, but even if they did there's
	// no recovery action we can take when applying a heartbeat. If errors do occur they will
	// be logged by the common state machine code.
	_ = txn.HandleEvent(ctx, event)
	return nil
}

func buildConfirmedSnapshotEvent(confirmedTransaction *common.ConfirmedTransaction) transaction.Event {
	if len(confirmedTransaction.RevertReason) == 0 {
		return &transaction.ConfirmedSuccessEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: confirmedTransaction.ID,
			},
		}
	}

	return &transaction.ConfirmedRevertedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: confirmedTransaction.ID,
		},
		RevertReason: confirmedTransaction.RevertReason,
	}
}

func guard_HeartbeatThresholdExceeded(ctx context.Context, o *originator) bool {
	if o.timeOfMostRecentHeartbeat == nil {
		//we have never seen a heartbeat so that was a really long time ago, certainly longer than any threshold
		return true
	}
	if o.clock.HasExpired(o.timeOfMostRecentHeartbeat, o.heartbeatThresholdMs) {
		return true
	}
	return false
}
