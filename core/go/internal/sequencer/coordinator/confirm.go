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
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
)

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
