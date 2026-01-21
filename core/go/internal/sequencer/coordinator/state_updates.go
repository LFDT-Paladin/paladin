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
	"fmt"
	"slices"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// stateupdate_TransactionsDelegated adds delegated transactions to tracking
// TODO: Need to finalize the decision about whether we expect the originator to include all inflight delegated transactions in every delegation request.
// Currently the code assumes we do so need to make the spec clear on that point and record a decision record to explain why. Every time we come back to
// this point, we will be tempted to reverse that decision so we need to make sure we have a record of the known consequences.
// originator must be a fully qualified identity locator otherwise an error will be returned
func stateupdate_TransactionsDelegated(ctx context.Context, state *coordinatorState, config *stateMachineConfig, callbacks *stateMachineCallbacks, event common.Event) error {
	delegatedEvent := event.(*TransactionsDelegatedEvent)
	originator := delegatedEvent.Originator
	transactions := delegatedEvent.Transactions

	var previousTransaction *transaction.Transaction
	for _, txn := range transactions {

		if state.GetTransactionCount() >= config.maxInflightTransactions {
			// We'll rely on the fact that originators retry incomplete transactions periodically
			return i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, config.maxInflightTransactions)
		}

		newTransaction, err := callbacks.newTransaction(ctx, originator, txn)
		if err != nil {
			log.L(ctx).Errorf("error creating transaction: %v", err)
			return err
		}

		// TODO AM: previous and next can probably be removed
		if previousTransaction != nil {
			newTransaction.SetPreviousTransaction(ctx, previousTransaction)
			previousTransaction.SetNextTransaction(ctx, newTransaction)
		}
		// TODO AM: there's a bunch of code to think about here in the model of the new state machine
		state.AddTransaction(txn.ID, newTransaction)
		callbacks.metrics.IncCoordinatingTransactions()
		previousTransaction = newTransaction

		receivedEvent := &transaction.ReceivedEvent{}
		receivedEvent.TransactionID = txn.ID

		// The newly delegated TX might be after the restart of an originator, for which we've already
		// instantiated a chained TX
		hasChainedTransaction, err := callbacks.txManager.HasChainedTransaction(ctx, txn.ID)
		if err != nil {
			log.L(ctx).Errorf("error checking for chained transaction: %v", err)
			return err
		}
		if hasChainedTransaction {
			log.L(ctx).Debugf("chained transaction %s found", txn.ID.String())
			// TODO AM: this needs to be an event
			newTransaction.SetChainedTxInProgress()
		}
		err = state.GetTransactionByID(txn.ID).ProcessEvent(ctx, receivedEvent)
		if err != nil {
			log.L(ctx).Errorf("error handling ReceivedEvent for transaction %s: %v", txn.ID.String(), err)
			return err
		}
	}
	return nil
}

// stateupdate_TransactionConfirmed confirms a dispatched transaction and updates tracking maps
func stateupdate_TransactionConfirmed(ctx context.Context, state *coordinatorState, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) error {
	confirmedEvent := event.(*TransactionConfirmedEvent)
	//This may be a confirmation of a transaction that we have have been coordinating or it may be one that another coordinator has been coordinating
	//if the latter, then we may or may not know about it depending on whether we have seen a heartbeat from that coordinator since last time
	// we were loaded into memory
	//TODO - we can't actually guarantee that we have all transactions we dispatched in memory.
	//Even assuming that the public txmgr is in the same process (may not be true forever)  and assuming that we haven't been swapped out ( likely not to be true very soon) there is still a chance that the transaction was submitted to the base ledger, then the process restarted then we get the confirmation.
	// //When the process starts, we need to make sure that the coordinator is pre loaded with knowledge of all transactions that it has dispatched
	// MRW TODO ^^
	isDispatchedTransaction, err := confirmDispatchedTransaction(ctx, state, confirmedEvent.TxID, confirmedEvent.From, confirmedEvent.Nonce, confirmedEvent.Hash, confirmedEvent.RevertReason)
	if err != nil {
		log.L(ctx).Errorf("error confirming transaction From: %s , Nonce: %d, Hash: %v: %v", confirmedEvent.From, confirmedEvent.Nonce, confirmedEvent.Hash, err)
		return err
	}
	if !isDispatchedTransaction {
		confirmMonitoredTransaction(ctx, state, confirmedEvent.From, confirmedEvent.Nonce)
	}
	return nil
}

func confirmDispatchedTransaction(ctx context.Context, state *coordinatorState, txId uuid.UUID, from *pldtypes.EthAddress, nonce uint64, hash pldtypes.Bytes32, revertReason pldtypes.HexBytes) (bool, error) {
	log.L(ctx).Debugf("we currently have %d transactions to handle, confirming that dispatched TX %s is in our list", state.GetTransactionCount(), txId.String())

	// Confirming a transaction via its chained transaction, we won't hav a from address
	if from != nil {
		// First check whether it is one that we have been coordinating
		if dispatchedTransaction := state.GetTransactionByID(txId); dispatchedTransaction != nil {
			// TODO AM: is this reading the transaction state? If we need to make sure that the coordinator state machine only has the transaction state reader
			// and that we use appropriate locks here (or document why no lock needed)
			if dispatchedTransaction.GetLatestSubmissionHash() == nil || *(dispatchedTransaction.GetLatestSubmissionHash()) != hash {
				// Is this not the transaction that we are looking for?
				// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
				// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
				log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected", dispatchedTransaction.ID.String())
			}
			event := &transaction.ConfirmedEvent{
				Hash:         hash,
				RevertReason: revertReason,
				Nonce:        nonce,
			}
			event.TransactionID = dispatchedTransaction.ID
			event.EventTime = time.Now()
			err := dispatchedTransaction.ProcessEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.ID.String(), err)
				return false, err
			}
			return true, nil
		}
	}

	for _, dispatchedTransaction := range state.GetAllTransactions() {
		if dispatchedTransaction.ID == txId {
			if dispatchedTransaction.GetLatestSubmissionHash() == nil {
				// The transaction created a chained private transaction so there is no hash to compare
				log.L(ctx).Debugf("transaction %s confirmed with nil dispatch hash (confirmed hash of chained TX %s)", dispatchedTransaction.ID.String(), hash.String())
			} else if *(dispatchedTransaction.GetLatestSubmissionHash()) != hash {
				// Is this not the transaction that we are looking for?
				// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
				// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
				log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected. Dispatch hash %s, confirmed hash %s", dispatchedTransaction.ID.String(), dispatchedTransaction.GetLatestSubmissionHash(), hash.String())
			}
			event := &transaction.ConfirmedEvent{
				Hash:         hash,
				RevertReason: revertReason,
				Nonce:        nonce,
			}
			event.TransactionID = txId
			event.EventTime = time.Now()

			log.L(ctx).Debugf("Confirming dispatched TX %s", txId.String())
			err := dispatchedTransaction.ProcessEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.ID.String(), err)
				return false, err
			}
			return true, nil
		}
	}
	log.L(ctx).Infof("failed to find a transaction submitted by signer %s", from.String())
	return false, nil

}

func confirmMonitoredTransaction(_ context.Context, state *coordinatorState, from *pldtypes.EthAddress, nonce uint64) {
	if flushPoint := state.GetFlushPointBySignerNonce(fmt.Sprintf("%s:%d", from.String(), nonce)); flushPoint != nil {
		//We do not remove the flushPoint from the list because there is a chance that the coordinator hasn't seen this confirmation themselves and
		// when they send us the next heartbeat, it will contain this FlushPoint so it would get added back into the list and we would not see the confirmation again
		flushPoint.Confirmed = true
	}
}

// stateupdate_NewBlock updates the current block height
func stateupdate_NewBlock(_ context.Context, state *coordinatorState, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) error {
	newBlockEvent := event.(*NewBlockEvent)
	state.SetCurrentBlockHeight(newBlockEvent.BlockHeight)
	return nil
}

// stateupdate_EndorsementRequested updates the active coordinator and originator pool
func stateupdate_EndorsementRequested(ctx context.Context, state *coordinatorState, config *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) error {
	endorsementRequestedEvent := event.(*EndorsementRequestedEvent)
	state.SetActiveCoordinatorNode(endorsementRequestedEvent.From)
	// In case we ever take over as coordinator we need to send heartbeats to potential originators
	updateOriginatorNodePoolInternal(ctx, state, config, endorsementRequestedEvent.From)
	return nil
}

// stateupdate_HeartbeatReceived updates the active coordinator, block height, and stores flush points
func stateupdate_HeartbeatReceived(ctx context.Context, state *coordinatorState, config *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) error {
	heartbeatEvent := event.(*HeartbeatReceivedEvent)
	state.SetActiveCoordinatorNode(heartbeatEvent.From)
	state.SetActiveCoordinatorBlockHeight(heartbeatEvent.BlockHeight)
	// In case we ever take over as coordinator we need to send heartbeats to potential originators
	updateOriginatorNodePoolInternal(ctx, state, config, heartbeatEvent.From)
	for _, flushPoint := range heartbeatEvent.FlushPoints {
		state.SetFlushPoint(flushPoint.GetSignerNonce(), flushPoint)
	}
	return nil
}

// Helper function for updating originator node pool
func updateOriginatorNodePoolInternal(ctx context.Context, state *coordinatorState, config *stateMachineConfig, originatorNode string) {
	log.L(ctx).Debugf("updating originator node pool for contract %s with node %s", config.contractAddress.String(), originatorNode)
	pool := state.GetOriginatorNodePool()
	if !slices.Contains(pool, originatorNode) {
		pool = append(pool, originatorNode)
	}
	if !slices.Contains(pool, config.nodeName) {
		pool = append(pool, config.nodeName)
	}
	slices.Sort(pool)
	state.SetOriginatorNodePool(pool)
}

// stateupdate_HeartbeatInterval increments the heartbeat intervals counter
func stateupdate_HeartbeatInterval(_ context.Context, state *coordinatorState, _ *stateMachineConfig, _ *stateMachineCallbacks, _ common.Event) error {
	state.IncrementHeartbeatIntervalsSinceStateChange()
	return nil
}

// stateupdate_ResetHeartbeatsSinceIntervalresets the heartbeat intervals counter on state transitions
func stateupdate_ResetHeartbeatIntervalsSinceStateChange(ctx context.Context, state *coordinatorState, _ *stateMachineConfig, _ *stateMachineCallbacks, _ common.Event) error {
	state.SetHeartbeatIntervalsSinceStateChange(0)
	return nil
}

// stateupdate_AddTransactionToPool adds the transaction to the pool
// Use with validator_IsTransitionToPooled
func stateupdate_AddTransactionToPool(ctx context.Context, state *coordinatorState, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) error {
	transactionStateTransitionEvent := event.(*common.TransactionStateTransitionEvent)
	txID := transactionStateTransitionEvent.TxID

	txn := state.GetTransactionByID(txID)
	if txn == nil {
		log.L(ctx).Warnf("Transaction %s not found when attempting to add to pool", txID.String())
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "transaction not found")
	}

	log.L(ctx).Debugf("Adding transaction %s to pooled transactions", txID.String())
	state.AddPooledTransaction(txn)
	return nil
}

// stateupdate_CleanupFinalTransaction removes the transaction from state and cleans up associated resources
// Use with validator_IsTransitionToFinal
func stateupdate_CleanupFinalTransaction(ctx context.Context, state *coordinatorState, _ *stateMachineConfig, callbacks *stateMachineCallbacks, event common.Event) error {
	transactionStateTransitionEvent := event.(*common.TransactionStateTransitionEvent)
	txID := transactionStateTransitionEvent.TxID

	log.L(ctx).Debugf("Cleaning up transaction %s", txID.String())
	state.DeleteTransaction(txID)
	// TODO AM: are metrics and grapher actions that follow the state update?
	callbacks.metrics.DecCoordinatingTransactions()
	if err := callbacks.grapher.Forget(txID); err != nil {
		log.L(ctx).Errorf("Error forgetting transaction %s: %v", txID.String(), err)
	}
	log.L(ctx).Debugf("Transaction %s cleaned up", txID.String())
	return nil
}

// stateupdate_ClearAssemblingTransaction clears the assembling transaction slot
// Use with validator_IsTransitionFromAssembling
func stateupdate_ClearAssemblingTransaction(ctx context.Context, state *coordinatorState, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) error {
	transactionStateTransitionEvent := event.(*common.TransactionStateTransitionEvent)
	txID := transactionStateTransitionEvent.TxID

	log.L(ctx).Debugf("Clearing assembling transaction %s", txID.String())
	state.ClearAssemblingTransaction()
	return nil
}

func stateupdate_SelectTransaction(ctx context.Context, state *coordinatorState, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) error {
	if state.GetAssemblingTransaction() != nil {
		return nil
	}

	if txn := state.PopNextPooledTransaction(); txn != nil {
		log.L(ctx).Debugf("Selected transaction %s to assemble", txn.ID.String())
		state.SetAssemblingTransaction(txn)
	}
	return nil
}

func stateupdate_CalculateInflightTransactions(ctx context.Context, state *coordinatorState, config *stateMachineConfig, _ *stateMachineCallbacks, _ common.Event) error {
	// TODO AM: this is using the existing method of communicating with the dispatch loop which we know needs some reworking
	// it may in fact be an action - really we're giving the dispatch loop a new number of inflight transactions rather than changing state as such
	// how about the getTransactionsInStates part moves to the reader interface and the dispatch loop is allowed to read this (under lock)
	// (which is a reminder that we need the two levels of reading- with and without locks)
	// I think that keeping the coordination mutex is a good idea- just some thought needed about scope and where it lives- is control of it a callback?
	config.inFlightMutex.L.Lock()
	defer config.inFlightMutex.L.Unlock()
	clear(config.inFlightTxns)
	dispatchingTransactions := getTransactionsInStates(ctx, state, []transaction.State{transaction.State_Dispatched, transaction.State_Submitted, transaction.State_SubmissionPrepared})
	for _, txn := range dispatchingTransactions {
		if txn.PreparedPrivateTransaction == nil {
			// We don't count transactions the result in new private transactions
			config.inFlightTxns[txn.ID] = txn
		}
	}
	log.L(ctx).Debugf("coordinator has %d dispatching transactions", len(config.inFlightTxns))
	config.inFlightMutex.Signal()
	return nil
}
