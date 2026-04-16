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
	"github.com/google/uuid"
)

// Enum for delegation acknowledgement errors
type DelegationAcknowledgementError int64

const (
	DelegationAcknowledgementError_None DelegationAcknowledgementError = iota
	DelegationAcknowledgementError_MaxInflightTransactions
	DelegationAcknowledgementError_CoordinatorError
	DelegationAcknowledgementError_PreviousTransactionError
)

// Originators send only the delegated transactions that they believe the coordinator needs to know/be reminded about. Which transactions are
// included in this list depends on whether it is an intitial attempt or a scheduled retry, and whether individual delegation timeouts have
// been exceeded. This means that the coordinator cannot infer any dependency or ordering between transactions based on the list of transactions
// in the request.
func action_TransactionsDelegated(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*TransactionsDelegatedEvent)
	c.updateOriginatorNodePool(e.FromNode)
	return c.addToDelegatedTransactions(ctx, e.Originator, e.Transactions, e.DelegationID, c.newCoordinatorTransaction)
}

func (c *coordinator) newCoordinatorTransaction(ctx context.Context, originator string, originatorNode string, nodeName string, pt *components.PrivateTransaction, coordinatorSigningIdentity string, preAssembleDependsOn *uuid.UUID) transaction.CoordinatorTransaction {
	return transaction.NewTransaction(ctx, originator, originatorNode, nodeName, pt, coordinatorSigningIdentity, preAssembleDependsOn, c.transportWriter, c.clock, c.queueEventInternal, c.engineIntegration, c.syncPoints, c.components, c.domainAPI, c.dCtx, c.requestTimeout, c.stateTimeout, c.closingGracePeriod, c.confirmedLockRetentionGracePeriod, c.baseLedgerRevertRetryThreshold, c.assembleErrorRetryThreshhold, c.grapher, c.chainedChildStore, c.metrics)
}

// originator must be a fully qualified identity locator otherwise an error will be returned
func (c *coordinator) addToDelegatedTransactions(
	ctx context.Context,
	originator string,
	transactions []*components.PrivateTransaction,
	delegationID string,
	createTransaction func(
		ctx context.Context,
		originator string,
		originatorNode string,
		nodeName string,
		pt *components.PrivateTransaction,
		coordinatorSigningIdentity string,
		preAssembleDependsOn *uuid.UUID) transaction.CoordinatorTransaction) error {

	var previousTransaction transaction.CoordinatorTransaction

	delegateAcknowledgementIDs := make([]string, 0, len(transactions))
	delegateAcknowledgementErrors := make([]int64, len(transactions))
	rejectedMaxInFlight := 0
	acceptedTransactions := 0
	inProgressTransactions := 0
	var txnHandlingError error

	_, originatorNode, err := pldtypes.PrivateIdentityLocator(originator).Validate(ctx, "", false)
	if err != nil {
		log.L(ctx).Errorf("error validating originator %s: %s", originator, err)
		// This is likely a code bug that the originator can't do anything about, and since we can't parse the node to send an acknowledgement back to,
		// no point sending a delegation acknowledgement here.
		return err
	}

	for i, txn := range transactions {
		// Acknowledge every delegation
		delegateAcknowledgementIDs = append(delegateAcknowledgementIDs, txn.ID.String())

		if txnHandlingError != nil {
			// Any previous errors, don't handle this or subsequent TXNs to maintain FIFO ordering
			delegateAcknowledgementErrors[i] = int64(DelegationAcknowledgementError_PreviousTransactionError)
			continue
		}

		// store the transaction if it already exists, so if the next transaction is new we can establish the dependency via an event
		if c.transactionsByID[txn.ID] != nil {
			inProgressTransactions++
			previousTransaction = c.transactionsByID[txn.ID]
			log.L(ctx).Debugf("transaction %s already being coordinated", txn.ID.String())
			continue
		}

		if len(c.transactionsByID) >= c.maxInflightTransactions {
			rejectedMaxInFlight++
			log.L(ctx).Tracef("transaction %s being rejected - reached max in-flight limit", txn.ID.String())
			delegateAcknowledgementErrors[i] = int64(DelegationAcknowledgementError_MaxInflightTransactions)
			// Since all subsequent transactions will be rejected for the same reason, go through them all recording
			// an error for the delegation acknowledgement response. The originator may choose to do nothing but log
			// and retry later.
			continue
		}

		// We use the order in which transaction are delegated to establish preassembly dependencies, which is what allows us to
		// ensure FIFO ordering within an originator up until first assembly.
		//
		// An originator sends all of its known transactions (i.e. all that have not yet been confirmed) with every delegation request.
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
				txID := previousTransaction.GetID()
				// There is an incredibly slim possibility that the transaction has actually been repooled, so we are past first assembly,
				// but since we have no way of checking this it causes no issues to establish the dependency, since the already pooled transaction
				// will be selected for assembly ahead of this new transaction anyway.
				//
				// This would only be possible if
				// - the coordinator has been rejecting delegated transaction after reaching its max inflight limit
				// - the originator has missed the assembly request for the previous transaction, causing it to be repooled
				err := previousTransaction.HandleEvent(ctx, &transaction.NewPreAssembleDependencyEvent{
					BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
						TransactionID: txID,
					},
					PrereqTransactionID: txn.ID,
				})
				if err != nil {
					txnHandlingError = err
					delegateAcknowledgementErrors[i] = int64(DelegationAcknowledgementError_CoordinatorError)
					continue
				}
				previousTransactionID = &txID
			}
		}

		newTransaction := createTransaction(ctx, originator, originatorNode, c.nodeName, txn, c.signingIdentity, previousTransactionID)

		c.transactionsByID[txn.ID] = newTransaction
		c.metrics.IncCoordinatingTransactions()
		acceptedTransactions++

		receivedEvent := &transaction.DelegatedEvent{}
		receivedEvent.TransactionID = txn.ID

		err = newTransaction.HandleEvent(ctx, receivedEvent)
		if err != nil {
			delete(c.transactionsByID, txn.ID)
			c.metrics.DecCoordinatingTransactions()
			acceptedTransactions--
			txnHandlingError = err
			delegateAcknowledgementErrors[i] = int64(DelegationAcknowledgementError_CoordinatorError)
			// All subsequent transactions will be skipped
			continue
		}
		previousTransaction = newTransaction
	}

	// Acknowledge the delegate request. Optionally errors can be returned which the originator may use to base re-delegate decisions on
	err = c.transportWriter.SendDelegationRequestAcknowledgment(ctx, originatorNode, delegationID, delegateAcknowledgementIDs, delegateAcknowledgementErrors)
	if err != nil {
		return err
	}

	if rejectedMaxInFlight > 0 {
		err := i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, c.maxInflightTransactions, originatorNode, len(transactions), acceptedTransactions, inProgressTransactions, rejectedMaxInFlight)
		return err
	}

	if txnHandlingError != nil {
		// Return the first TX creation or handling error
		return txnHandlingError
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
	c.pooledTransactions[0] = nil // clear reference so the backing array doesn't pin the transaction from GC
	c.pooledTransactions = c.pooledTransactions[1:]
	return nextPooledTx
}

func (c *coordinator) removeTransactionFromPool(id uuid.UUID) {
	for i, txn := range c.pooledTransactions {
		if txn.GetID() == id {
			c.pooledTransactions[i] = nil
			c.pooledTransactions = append(c.pooledTransactions[:i], c.pooledTransactions[i+1:]...)
			return
		}
	}
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
	// this is a no-op if the transaction is not in the pool
	c.removeTransactionFromPool(e.TransactionID)
	c.metrics.DecCoordinatingTransactions()
	err := c.grapher.Forget(ctx, e.TransactionID)
	if err != nil {
		log.L(ctx).Errorf("error forgetting transaction %s: %v", e.TransactionID.String(), err)
	}
	c.chainedChildStore.ForgetChainedChild(e.TransactionID)
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
