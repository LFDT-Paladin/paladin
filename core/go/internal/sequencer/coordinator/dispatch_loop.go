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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
)

func (c *coordinator) dispatchLoop(ctx context.Context) {
	log.L(ctx).Debugf("coordinator dispatch loop started for contract %s", c.contractAddress.String())

	for {
		select {
		case tx := <-c.dispatchQueue:
			// Wait for dispatch-ahead capacity, then pull a batch (this tx plus any others already queued,
			// capped to the capacity) and prepare, stage and commit them in a single flush.
			capacity := c.awaitDispatchAheadCapacity(ctx)
			if capacity <= 0 {
				log.L(ctx).Debugf("coordinator dispatch loop for contract %s stopped", c.contractAddress.String())
				return
			}
			batch := c.pullDispatchBatch(tx, capacity)
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
	for len(c.inFlightTxns) >= c.maxDispatchAhead {
		c.inFlightMutex.Wait()
		select {
		case <-ctx.Done():
			return 0
		default:
		}
	}
	return c.maxDispatchAhead - len(c.inFlightTxns)
}

// pullDispatchBatch returns first plus up to capacity-1 further transactions already waiting on the queue,
// without blocking for more, in the order they came off the queue. This pull order is significant: it
// ultimately determines on-chain nonce order - see the staging loop in dispatchBatch for why.
func (c *coordinator) pullDispatchBatch(first transaction.CoordinatorTransaction, capacity int) []transaction.CoordinatorTransaction {
	batch := make([]transaction.CoordinatorTransaction, 1, capacity)
	batch[0] = first
	for len(batch) < capacity {
		select {
		case tx := <-c.dispatchQueue:
			batch = append(batch, tx)
		default:
			return batch
		}
	}
	return batch
}

// dispatchBatch prepares each transaction, detaches and takes ownership of each prepared dispatch, appends
// them all to a single DispatchBatch in pull order, commits the batch in one DB transaction, then hands off
// chained children.
func (c *coordinator) dispatchBatch(ctx context.Context, batch []transaction.CoordinatorTransaction) {
	// Append in pull order. This order is what makes the on-chain nonces follow the order transactions were
	// selected for dispatch:
	//  1. Every dispatch in the batch carries this coordinator's contract address as its flush-writer
	//     WriteKey, so the writer routes the whole batch to the SAME worker (it shards by WriteKey) and
	//     commits it in one DB transaction.
	//  2. That transaction inserts each public transaction row into public_txns in Append order, so the
	//     auto-increment pub_txn_id is monotonic with our Append order.
	//  3. Nonces are NOT assigned here. Later the per-signing-address public-tx orchestrator polls its
	//     unprocessed rows ORDER BY pub_txn_id and assigns gapless sequential nonces in that order.
	// So pull order -> Append order -> insert order -> pub_txn_id order -> nonce order. Appending in
	// prepare-completion order instead (e.g. if prepare were ever parallelised) would let nonces follow
	// completion order rather than selection order, so we must append strictly in pull order.
	var dispatchBatch *syncpoints.DispatchBatch
	for _, tx := range batch {
		log.L(ctx).Debugf("submitting transaction %s for dispatch", tx.GetID().String())
		// HandleEvent transitions the transaction into State_Dispatched under its lock, synchronously adding
		// it to inFlightTxns (via setDispatchedInFlight) when it sends a public transaction, so
		// len(inFlightTxns) is accurate for the next capacity check.
		if err := tx.HandleEvent(ctx, &transaction.DispatchedEvent{
			BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
				TransactionID: tx.GetID(),
			},
		}); err != nil {
			log.L(ctx).Errorf("error dispatching transaction %s: %v", tx.GetID().String(), err)
			continue
		}
		// Reading the pending dispatch after a successful HandleEvent is a point of no return: from here it
		// will be persisted regardless of any later state change to the transaction. A nil result means the
		// transaction was repooled before its DispatchedEvent was processed, so prepare never ran - skip it,
		// it produces no dispatch and no nonce. The batch is created lazily on the first real dispatch so a
		// batch of only-repooled transactions touches no syncPoints.
		if pd := tx.PendingDispatch(ctx); pd != nil {
			if dispatchBatch == nil {
				dispatchBatch = &syncpoints.DispatchBatch{
					DomainStateWriter: c.dsw,
					ContractAddress:   *c.contractAddress,
				}
			}
			dispatchBatch.Append(pd)
		}
	}
	if dispatchBatch == nil {
		return
	}

	// Commit the whole batch in a single DB transaction. Persistence happens off the transaction lock so the
	// DB commit does not block the coordinator event loop behind a tx lock.
	if err := c.syncPoints.PersistDispatchBatch(ctx, dispatchBatch); err != nil {
		log.L(ctx).Errorf("error persisting dispatch batch: %v", err)
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
