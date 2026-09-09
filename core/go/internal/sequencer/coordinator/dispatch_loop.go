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

package coordinator

import (
	"context"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
)

type queuedDispatch struct {
	txn        transaction.CoordinatorTransaction
	prepared   *syncpoints.PendingDispatch
	enqueuedAt time.Time // so the dispatch loop can report how long the dispatch waited before being dequeued.
}

// enqueueForDispatch is called on the coordinator event loop under the transaction lock, so transactions
// are enqueued strictly in the order they become ready for dispatch - the order that ultimately drives
// on-chain nonce order.
func (c *coordinator) enqueueForDispatch(ctx context.Context, txn transaction.CoordinatorTransaction, prepared *syncpoints.PendingDispatch) {
	select {
	case c.dispatchQueue <- queuedDispatch{txn: txn, prepared: prepared, enqueuedAt: c.clock.Now()}:
	case <-ctx.Done():
	}
}

func (c *coordinator) dispatchLoop(ctx context.Context) {
	log.L(ctx).Debugf("coordinator dispatch loop started for contract %s", c.contractAddress.String())

	for {
		select {
		case qd := <-c.dispatchQueue:
			c.metrics.ObserveDispatchQueueWait(c.clock.Now().Sub(qd.enqueuedAt))
			// Wait for dispatch-ahead capacity, then pull a batch (this tx plus any others already queued,
			// capped to the capacity) and commit them in a single flush.
			capacity := c.awaitDispatchAheadCapacity(ctx)
			if capacity <= 0 {
				log.L(ctx).Debugf("coordinator dispatch loop for contract %s stopped", c.contractAddress.String())
				return
			}
			if capacity > c.dispatchMaxBatchSize {
				capacity = c.dispatchMaxBatchSize
			}
			batch := c.pullDispatchBatch(qd, capacity)
			c.dispatchBatch(ctx, batch)
		case <-ctx.Done():
			log.L(ctx).Debugf("coordinator dispatch loop for contract %s stopped", c.contractAddress.String())
			return
		}
	}
}

// awaitDispatchAheadCapacity blocks until at least one dispatch-ahead slot is free, then returns the number of
// free slots (maxDispatchAhead - inFlight). Returns 0 if the context is cancelled while waiting. Only
// public transactions occupy a slot, so this is a safe upper bound on how many transactions to pull: some
// of the batch may not count towards inFlightTxns.
func (c *coordinator) awaitDispatchAheadCapacity(ctx context.Context) int {
	c.inFlightMutex.L.Lock()
	defer c.inFlightMutex.L.Unlock()
	waitStart := c.clock.Now()
	for len(c.inFlightTxns) >= c.maxDispatchAhead {
		c.inFlightMutex.Wait()
		select {
		case <-ctx.Done():
			return 0
		default:
		}
	}
	c.metrics.ObserveDispatchInflightWait(c.clock.Now().Sub(waitStart))
	return c.maxDispatchAhead - len(c.inFlightTxns)
}

// pullDispatchBatch returns first plus up to capacity-1 further queued dispatches already waiting on the
// queue, without blocking for more, in the order they came off the queue.
func (c *coordinator) pullDispatchBatch(first queuedDispatch, capacity int) []queuedDispatch {
	batch := make([]queuedDispatch, 1, capacity)
	batch[0] = first
	for len(batch) < capacity {
		select {
		case qd := <-c.dispatchQueue:
			c.metrics.ObserveDispatchQueueWait(c.clock.Now().Sub(qd.enqueuedAt))
			batch = append(batch, qd)
		default:
			return batch
		}
	}
	return batch
}

func (c *coordinator) dispatchBatch(ctx context.Context, batch []queuedDispatch) {
	// Append in pull order. This order is what makes the on-chain nonces follow the order transactions were
	// selected for dispatch:
	//  1. Every dispatch in the batch carries this coordinator's contract address as its flush-writer
	//     WriteKey, so the writer routes the whole batch to the SAME worker (it shards by WriteKey) and
	//     commits it in one DB transaction.
	//  2. That transaction inserts each public transaction row into public_txns in Append order, so the
	//     auto-increment pub_txn_id is monotonic with our Append order.
	//  3. Nonces are NOT assigned here. Later the per-signing-address public-tx orchestrator polls its
	//     unprocessed rows ORDER BY pub_txn_id and assigns gapless sequential nonces in that order.
	// So pull order -> Append order -> insert order -> pub_txn_id order -> nonce order.
	dispatchBatch := &syncpoints.DispatchBatch{
		ContractAddress: *c.contractAddress,
	}
	for _, qd := range batch {
		txID := qd.prepared.TransactionID
		log.L(ctx).Debugf("submitting transaction %s for dispatch", txID.String())
		// The point of no return is the transaction accepting this event, so we persist the dispatch only
		// if it takes effect. Accepting it also updates the coordinator's in-flight count synchronously, so
		// len(inFlightTxns) is accurate for the next capacity check. If a dependency reset or revert has
		// already moved the transaction on, the event is a no-op and the dispatch is dropped.
		if err := qd.txn.HandleEvent(ctx, &transaction.DispatchedEvent{
			BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
				TransactionID: txID,
			},
			PublicTransaction: len(qd.prepared.Dispatch.PublicDispatches) > 0,
		}); err != nil {
			log.L(ctx).Errorf("error handling dispatched event for transaction %s: %v", txID.String(), err)
		}
		if qd.txn.GetCurrentState() != transaction.State_Dispatched {
			continue
		}
		dispatchBatch.Append(qd.prepared)
	}

	// Record the composition of the batch about to be persisted; the low end of the histogram
	// reveals batches collapsing to a single entry.
	var public, private, prepared int
	for _, pd := range dispatchBatch.Dispatches() {
		public += len(pd.Dispatch.PublicDispatches)
		private += len(pd.Dispatch.PrivateDispatches)
		prepared += len(pd.Dispatch.PreparedTransactions)
	}
	c.metrics.ObserveDispatchBatchSize("public", public)
	c.metrics.ObserveDispatchBatchSize("private", private)
	c.metrics.ObserveDispatchBatchSize("prepared", prepared)

	// Commit the whole batch in a single DB transaction, including each dispatch's new states and
	// nullifiers, which the pending dispatches carry in memory. Persistence happens off the transaction
	// lock so the DB commit does not block the coordinator event loop behind a tx lock. A failed commit
	// (typically transient DB unavailability) rolls back the whole DB transaction and the next attempt
	// re-writes everything from the in-memory batch, retrying indefinitely with backoff. The batch's
	// transactions stay dispatched throughout and are persisted when a retry succeeds.
	err := c.dispatchCommitErrorRetry.Do(ctx, func(_ int) (bool, error) {
		return true, c.syncPoints.PersistDispatchBatch(ctx, dispatchBatch)
	})
	if err != nil {
		// The retry only returns an error when the context is cancelled, so the dispatch loop is shutting
		// down; the batch is left unpersisted and is re-driven from persisted state on restart.
		return
	}

	// The batch has committed atomically; hand off any chained child transactions in order.
	for _, pd := range dispatchBatch.Dispatches() {
		if err := c.handleChainedChildren(ctx, pd.Dispatch); err != nil {
			log.L(ctx).Errorf("error handling chained children after dispatch: %v", err)
		}
	}
}

// handleChainedChildren submits the chained child transactions produced by a dispatch into the sequencer
// for processing. It runs after the dispatch has committed; it does not persist the dispatch (that is done
// atomically in the flush) but injects each child as a new transaction so it gets assembled and dispatched
// in turn.
func (c *coordinator) handleChainedChildren(ctx context.Context, dispatch *syncpoints.TransactionDispatch) error {
	for _, chained := range dispatch.PrivateDispatches {
		err := c.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTx persistence.DBTX) error {
			return c.components.SequencerManager().HandleNewTx(ctx, dbTx, chained.NewTransaction)
		})
		if err != nil {
			log.L(ctx).Errorf("error handling new private transaction: %v", err)
			return err
		}
	}
	return nil
}

func (c *coordinator) startDispatchLoop() {
	if c.ctx == nil || c.dispatchLoopCancel != nil {
		return // coordinator not yet started, or loop already running
	}
	loopCtx, cancel := context.WithCancel(c.ctx)
	done := make(chan struct{})
	c.dispatchLoopCancel = cancel
	c.dispatchLoopDone = done
	go func() {
		defer close(done)
		c.dispatchLoop(loopCtx)
	}()
}

func (c *coordinator) stopDispatchLoop() {
	if c.dispatchLoopCancel == nil {
		return
	}
	c.dispatchLoopCancel()
	c.dispatchLoopCancel = nil

	// Wake the loop if it is blocked in a cond.Wait on inFlightMutex,
	// since context cancellation alone does not unblock that.
	c.inFlightMutex.L.Lock()
	c.inFlightMutex.Broadcast()
	c.inFlightMutex.L.Unlock()

	<-c.dispatchLoopDone
	c.dispatchLoopDone = nil
}

func action_StartDispatchLoop(_ context.Context, c *coordinator, _ common.Event) error {
	c.startDispatchLoop()
	return nil
}

// action_QueueRestartDispatchLoop defers the dispatch loop restart by queuing a RestartDispatchLoopEvent
// rather than calling startDispatchLoop directly. This gives the coordinator a chance to process any
// pending events (e.g. delegations) before the loop resumes.
func action_QueueRestartDispatchLoop(ctx context.Context, c *coordinator, _ common.Event) error {
	c.queueEventInternal(ctx, &RestartDispatchLoopEvent{})
	return nil
}

func action_StopDispatchLoop(_ context.Context, c *coordinator, _ common.Event) error {
	c.stopDispatchLoop()
	return nil
}
