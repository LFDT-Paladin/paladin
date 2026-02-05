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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// Originators send only the delegated transactions that they believe the coordinator needs to know/be reminded about. Which transactions are
// included in this list depends on whether it is an intitial attempt or a scheduled retry, and whether individual delegation timeouts have
// been exceeded. This means that the coordinator cannot infer any dependency or ordering between transactions based on the list of transactions
// in the request.
func action_TransactionsDelegated(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*TransactionsDelegatedEvent)
	c.updateOriginatorNodePool(e.FromNode)
	return c.addToDelegatedTransactions(ctx, e.Originator, e.Transactions)
}

// originator must be a fully qualified identity locator otherwise an error will be returned
func (c *coordinator) addToDelegatedTransactions(ctx context.Context, originator string, transactions []*components.PrivateTransaction) error {
	for _, txn := range transactions {

		if c.transactionsByID[txn.ID] != nil {
			log.L(ctx).Debugf("transaction %s already being coordinated", txn.ID.String())
			continue
		}

		if len(c.transactionsByID) >= c.maxInflightTransactions {
			// We'll rely on the fact that originators retry incomplete transactions periodically
			return i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, c.maxInflightTransactions)
		}

		// The newly delegated TX might be after the restart of an originator, for which we've already
		// instantiated a chained TX
		hasChainedTransaction, err := c.txManager.HasChainedTransaction(ctx, txn.ID)
		if err != nil {
			log.L(ctx).Errorf("error checking for chained transaction: %v", err)
			return err
		}
		if hasChainedTransaction {
			log.L(ctx).Debugf("chained transaction %s found", txn.ID.String())
		}

		newTransaction, err := transaction.NewTransaction(
			ctx,
			originator,
			txn,
			hasChainedTransaction,
			c.nodeName,
			c.transportWriter,
			c.clock,
			c.queueEventInternal,
			c.engineIntegration,
			c.syncPoints,
			c.requestTimeout,
			c.assembleTimeout,
			c.closingGracePeriod,
			c.grapher,
			c.metrics,
			c.keyManager,
			c.publicTxManager,
			c.txManager,
			c.domainAPI,
			c.dCtx,
			c.buildNullifiers,
			c.newPrivateTransaction,
		)
		if err != nil {
			log.L(ctx).Errorf("error creating transaction: %v", err)
			return err
		}

		c.transactionsByID[txn.ID] = newTransaction
		c.metrics.IncCoordinatingTransactions()

		receivedEvent := &transaction.ReceivedEvent{}
		receivedEvent.TransactionID = txn.ID

		err = c.transactionsByID[txn.ID].HandleEvent(ctx, receivedEvent)
		if err != nil {
			log.L(ctx).Errorf("error handling ReceivedEvent for transaction %s: %v", txn.ID.String(), err)
			return err
		}
	}
	return nil
}

func action_SelectTransaction(ctx context.Context, c *coordinator, _ common.Event) error {
	// Take the opportunity to inform the sequencer lifecycle manager that we have become active so it can decide if that has
	// casued us to reach the node's limit on active coordinators.
	c.activeCoordinatorNode = c.nodeName
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

func (c *coordinator) selectNextTransactionToAssemble(ctx context.Context) error {
	log.L(ctx).Trace("selecting next transaction to assemble")
	txn := c.popNextPooledTransaction(ctx)
	if txn == nil {
		log.L(ctx).Info("no transaction found to process")
		return nil
	}

	transactionSelectedEvent := &transaction.SelectedEvent{}
	transactionSelectedEvent.TransactionID = txn.GetID()
	err := txn.HandleEvent(ctx, transactionSelectedEvent)
	return err

}

func (c *coordinator) addTransactionToBackOfPool(txn *transaction.CoordinatorTransaction) {
	// Check if transaction is already in the pool
	// This makes the function safe to call multiple times, albeit not strictly idempotently
	for _, pooledTxn := range c.pooledTransactions {
		if pooledTxn.GetID() == txn.GetID() {
			return
		}
	}
	c.pooledTransactions = append(c.pooledTransactions, txn)
}

func (c *coordinator) popNextPooledTransaction(ctx context.Context) *transaction.CoordinatorTransaction {
	if len(c.pooledTransactions) == 0 {
		return nil
	}
	nextPooledTx := c.pooledTransactions[0]
	c.pooledTransactions = c.pooledTransactions[1:]
	return nextPooledTx
}

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
			c.addTransactionToBackOfPool(txn)
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
