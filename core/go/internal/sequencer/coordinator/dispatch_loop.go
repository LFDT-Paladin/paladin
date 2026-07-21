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
)

func (c *coordinator) dispatchLoop(ctx context.Context) {
	log.L(ctx).Debugf("coordinator dispatch loop started for contract %s", c.contractAddress.String())

	for {
		select {
		case tx := <-c.dispatchQueue:
			log.L(ctx).Debugf("coordinator pulled transaction %s from the dispatch queue. max dispatch ahead: %d", tx.GetID().String(), c.maxDispatchAhead)

			c.inFlightMutex.L.Lock()

			// Too many in flight - wait for some to be confirmed and re-check cancellation after each wake.
			for len(c.inFlightTxns) >= c.maxDispatchAhead {
				c.inFlightMutex.Wait()
				select {
				case <-ctx.Done():
					c.inFlightMutex.L.Unlock()
					log.L(ctx).Debugf("coordinator dispatch loop for contract %s stopped", c.contractAddress.String())
					return
				default:
				}
			}
			// Release before the dispatch flow: HandleEvent performs synchronous DB/state persistence.
			// Holding inFlightMutex across it would block the coordinator event loop (which grabs the same
			// lock via setDispatchedInFlight) behind the DB write.
			c.inFlightMutex.L.Unlock()

			// Ask the transaction state machine to handle dispatch. The transition into State_Dispatched
			// synchronously adds this transaction to inFlightTxns (via setDispatchedInFlight) when it results
			// in a public transaction, so len(inFlightTxns) is accurate before the next loop iteration.
			log.L(ctx).Debugf("submitting transaction %s for dispatch", tx.GetID().String())
			err := tx.HandleEvent(ctx, &transaction.DispatchedEvent{
				BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
					TransactionID: tx.GetID(),
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error dispatching transaction %s: %v", tx.GetID().String(), err)
				continue
			}
			// Persist the batch off the transaction lock so the DB commit doesn't block the coordinator event
			// loop behind the tx lock. The transition into State_Dispatched (in HandleEvent above) has already
			// added this transaction to inFlightTxns via setDispatchedInFlight when it sent a public transaction,
			// so len(inFlightTxns) is accurate before the next loop iteration.
			if err := tx.PersistDispatch(ctx); err != nil {
				log.L(ctx).Errorf("error persisting dispatch for transaction %s: %v", tx.GetID().String(), err)
				continue
			}
		case <-ctx.Done():
			log.L(ctx).Debugf("coordinator dispatch loop for contract %s stopped", c.contractAddress.String())
			return
		}
	}
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
