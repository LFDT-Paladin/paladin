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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// Enum for delegation acknowledgement errors
type DelegationAcknowledgementError int64

const (
	DelegationAcknowledgementError_None DelegationAcknowledgementError = iota
	DelegationAcknowledgementError_MaxInflightTransactions
)

// Originators send only the delegated transactions that they believe the coordinator needs to know/be reminded about. Which transactions are
// included in this list depends on whether it is an intitial attempt or a scheduled retry, and whether individual delegation timeouts have
// been exceeded. This means that the coordinator cannot infer any dependency or ordering between transactions based on the list of transactions
// in the request.
func action_TransactionsDelegated(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*TransactionsDelegatedEvent)
	c.updateOriginatorNodePool(e.FromNode)
	return c.addToDelegatedTransactions(ctx, e.Originator, e.Transactions, e.DelegationID)
}

// originator must be a fully qualified identity locator otherwise an error will be returned
func (c *coordinator) addToDelegatedTransactions(ctx context.Context, originator string, transactions []*components.PrivateTransaction, delegationID string) error {
	delegateAcknowledgementIDs := make([]string, 0, len(transactions))
	delegateAcknowledgementErrors := make([]int64, len(transactions))
	rejectedMaxInFlight := 0
	acceptedTransactions := 0
	inProgressTransactions := 0
	var newTxnError error
	for i, txn := range transactions {

		// Acknowledge every delegation
		delegateAcknowledgementIDs = append(delegateAcknowledgementIDs, txn.ID.String())

		if c.transactionsByID[txn.ID] != nil {
			inProgressTransactions++
			log.L(ctx).Tracef("transaction %s already being coordinated", txn.ID.String())
			continue
		}

		if len(c.transactionsByID) >= c.maxInflightTransactions {
			// We'll rely on the fact that originators retry incomplete transactions periodically
			rejectedMaxInFlight++
			log.L(ctx).Tracef("transaction %s being rejected - reached max in-flight limit", txn.ID.String())
			delegateAcknowledgementErrors[i] = int64(DelegationAcknowledgementError_MaxInflightTransactions)
			continue
		}

		newTransaction, err := transaction.NewTransaction(
			ctx,
			originator,
			c.nodeName,
			txn,
			c.signingIdentity,
			c.transportWriter,
			c.clock,
			c.queueEventInternal,
			c.engineIntegration,
			c.syncPoints,
			c.components,
			c.domainAPI,
			c.dCtx,
			c.requestTimeout,
			c.stateTimeout,
			c.closingGracePeriod,
			c.confirmedLockRetentionGracePeriod,
			c.baseLedgerRevertRetryThreshold,
			c.assembleErrorRetryThreshhold,
			c.grapher,
			c.metrics,
		)
		if err != nil {
			if newTxnError == nil {
				newTxnError = err
			}
			continue
		}

		acceptedTransactions++
		c.transactionsByID[txn.ID] = newTransaction
		c.metrics.IncCoordinatingTransactions()

		receivedEvent := &transaction.DelegatedEvent{}
		receivedEvent.TransactionID = txn.ID

		err = c.transactionsByID[txn.ID].HandleEvent(ctx, receivedEvent)
		if err != nil {
			continue
		}
	}

	// Acknowledge the delegate request. Optionally errors can be returned which the originator may use to base re-delegate decisions on
	err := c.transportWriter.SendDelegationRequestAcknowledgment(ctx, c.nodeName, delegationID, delegateAcknowledgementIDs, delegateAcknowledgementErrors)
	if err != nil {
		return err
	}

	if rejectedMaxInFlight > 0 {
		err := i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, c.maxInflightTransactions, len(transactions), acceptedTransactions, rejectedMaxInFlight)
		return err
	}

	if newTxnError != nil {
		// Return the first TX create error
		return newTxnError
	}

	return nil
}

func action_SelectTransaction(ctx context.Context, c *coordinator, _ common.Event) error {
	log.L(ctx).Debugf("selecting transaction for coordinator %s", c.nodeName)
	// Take the opportunity to inform the sequencer lifecycle manager that we have become active so it can decide if that has
	// casued us to reach the node's limit on active coordinators.
	c.activeCoordinatorNode = c.nodeName
	c.coordinatorActive(c.contractAddress, c.nodeName)

	// For domain types that can coordinate other nodes' transactions (e.g. Noto or Pente), start heartbeating
	// Domains such as Zeto that are always coordinated on the originating node, heartbeats aren't required
	// because other nodes cannot take over coordination.
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() != prototk.ContractConfig_COORDINATOR_SENDER {
		log.L(ctx).Debugf("starting heartbeat loop for coordinator %s", c.nodeName)
		go c.heartbeatLoop(ctx)
	} else {
		log.L(ctx).Debugf("not starting heartbeat loop for coordinator %s", c.nodeName)
	}

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
	c.pooledTransactions = c.pooledTransactions[1:]
	return nextPooledTx
}

func validator_TransactionStateTransitionToPooled(ctx context.Context, _ *coordinator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.To == transaction.State_Pooled, nil
}

func validator_TransactionStateTransitionDispatchedToPooled(ctx context.Context, _ *coordinator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.From == transaction.State_Dispatched && e.To == transaction.State_Pooled, nil
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

func validator_TransactionStateTransitionToReadyForDispatch(ctx context.Context, _ *coordinator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.To == transaction.State_Ready_For_Dispatch, nil
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

func validator_TransactionStateTransitionToFinal(ctx context.Context, _ *coordinator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.To == transaction.State_Final, nil
}

func validator_TransactionStateTransitionToEvicted(ctx context.Context, _ *coordinator, event common.Event) (bool, error) {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	return e.To == transaction.State_Evicted, nil
}

func action_CleanUpTransaction(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*common.TransactionStateTransitionEvent[transaction.State])
	delete(c.transactionsByID, e.TransactionID)
	c.metrics.DecCoordinatingTransactions()
	err := c.grapher.Forget(e.TransactionID)
	if err != nil {
		log.L(ctx).Errorf("error forgetting transaction %s: %v", e.TransactionID.String(), err)
	}
	log.L(ctx).Debugf("transaction %s cleaned up", e.TransactionID.String())
	return nil
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
