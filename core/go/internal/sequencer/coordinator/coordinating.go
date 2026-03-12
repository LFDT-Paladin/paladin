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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
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
	var previousTransaction transaction.CoordinatorTransaction

	for i, txn := range transactions {
		// store the transaction if it already exists, so if the next transaction is new we can establish the dependency via an event
		if c.transactionsByID[txn.ID] != nil {
			previousTransaction = c.transactionsByID[txn.ID]
			log.L(ctx).Debugf("transaction %s already being coordinated", txn.ID.String())
			continue
		}

		if len(c.transactionsByID) >= c.maxInflightTransactions {
			// We'll rely on the fact that originators retry incomplete transactions periodically
			return i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, c.maxInflightTransactions)
		}

		// We do all the validation needed to create a transaction upfront so that we can be certain when we tell the previous
		// transaction that it has a new dependency, that the creation of that dependency will definititely succeed.
		_, originatorNode, err := pldtypes.PrivateIdentityLocator(originator).Validate(ctx, "", false)
		if err != nil {
			log.L(ctx).Errorf("error validating originator %s: %s", originator, err)
			return err
		}
		hasChainedTransaction, err := c.components.TxManager().HasChainedTransaction(ctx, txn.ID)
		if err != nil {
			log.L(ctx).Errorf("error checking for chained transaction %s: %v", txn.ID, err)
			return err
		}

		// We use the order in which transaction are delegated to establish preassembly dependencies, which
		// is what allows us to ensure FIFO ordering within an originator up until first assembly.
		//
		// An originator sends all the transactions that it believes have not yet been assembled with ever delegation request.
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

		var previousTransactionID *uuid.UUID
		if previousTransaction != nil {
			switch previousTransaction.GetCurrentState() {
			case transaction.State_Initial, transaction.State_PreAssembly_Blocked, transaction.State_Pooled:
				// There is an incredibly slim possibility that the transaction has actually been repooled, so we are past first assembly,
				// but since we have no way of checking this it causes no issues to establish the dependency, since the already pooled transaction
				// will be selected for assembly ahead of this new transaction anyway.
				//
				// This would only be possible if
				// - the coordinator has been rejecting delegated transaction after reaching its max inflight limit
				// - the originator has missed the assembly request for the previous transaction, causing it to be repooled
				err := previousTransaction.HandleEvent(ctx, &transaction.NewPreAssembleDependencyEvent{
					BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
						TransactionID: previousTransaction.GetID(),
					},
					PrereqTransactionID: transactions[i-1].ID,
				})
				if err != nil {
					return err
				}
			}
		}

		newTransaction := transaction.NewTransaction(
			ctx,
			originator,
			originatorNode,
			hasChainedTransaction,
			c.nodeName,
			txn,
			c.signingIdentity,
			previousTransactionID,
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
			c.grapher,
			c.metrics,
		)

		c.transactionsByID[txn.ID] = newTransaction
		c.metrics.IncCoordinatingTransactions()

		receivedEvent := &transaction.DelegatedEvent{}
		receivedEvent.TransactionID = txn.ID

		err = newTransaction.HandleEvent(ctx, receivedEvent)
		if err != nil {
			log.L(ctx).Errorf("error handling ReceivedEvent for transaction %s: %v", txn.ID.String(), err)
			return err
		}
		previousTransaction = newTransaction
	}
	return nil
}

func action_SelectTransaction(ctx context.Context, c *coordinator, _ common.Event) error {
	// Take the opportunity to inform the sequencer lifecycle manager that we have become active so it can decide if that has
	// casued us to reach the node's limit on active coordinators.
	if c.activeCoordinatorNode != c.nodeName {
		c.activeCoordinatorNode = c.nodeName
		c.coordinatorActive(c.contractAddress, c.nodeName)
	}

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
