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
	defer close(c.dispatchLoopStopped)
	dispatchedAhead := 0 // Number of transactions we've dispatched without confirming they are in the state machine's in-flight list
	log.L(ctx).Debugf("coordinator dispatch loop started for contract %s", c.contractAddress.String())

	for {
		select {
		case tx := <-c.dispatchQueue:
			log.L(ctx).Debugf("coordinator pulled transaction %s from the dispatch queue. In-flight count: %d, dispatched ahead: %d, max dispatch ahead: %d", tx.GetID().String(), len(c.inFlightTxns), dispatchedAhead, c.maxDispatchAhead)

			c.inFlightMutex.L.Lock()

			// Too many in flight - wait for some to be confirmed and re-check cancellation after each wake.
			for len(c.inFlightTxns)+dispatchedAhead >= c.maxDispatchAhead {
				c.inFlightMutex.Wait()
				select {
				case <-ctx.Done():
					c.inFlightMutex.L.Unlock()
					log.L(ctx).Debugf("coordinator dispatch loop for contract %s stopped", c.contractAddress.String())
					return
				default:
				}
			}

			// Ask the transaction state machine to handle dispatch.
			log.L(ctx).Debugf("submitting transaction %s for dispatch", tx.GetID().String())
			err := tx.HandleEvent(ctx, &transaction.DispatchedEvent{
				BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
					TransactionID: tx.GetID(),
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error dispatching transaction %s: %v", tx.GetID().String(), err)
				c.inFlightMutex.L.Unlock()
				continue
			}

			// Only dispatched transactions that result in a sent public transaction count towards max dispatch ahead
			if tx.HasDispatchedPublicTransaction() {
				dispatchedAhead++
			}

			// We almost never need to wait for the state machine's event loop to process the update to State_Dispatched
			// but if we hit the max dispatch ahead limit after dispatching this transaction we do, because we can't be sure
			// in-flight will be accurate on the next loop round
			if len(c.inFlightTxns)+dispatchedAhead >= c.maxDispatchAhead {
				for c.inFlightTxns[tx.GetID()] == nil {
					c.inFlightMutex.Wait()
					select {
					case <-ctx.Done():
						c.inFlightMutex.L.Unlock()
						log.L(ctx).Debugf("coordinator dispatch loop for contract %s stopped", c.contractAddress.String())
						return
					default:
					}
				}
				dispatchedAhead = 0
			}
			c.inFlightMutex.L.Unlock()
		case <-ctx.Done():
			log.L(ctx).Debugf("coordinator dispatch loop for contract %s stopped", c.contractAddress.String())
			return
		}
	}
}

func action_NudgeDispatchLoop(ctx context.Context, c *coordinator, _ common.Event) error {
	// Prod the dispatch loop with an updated in-flight count. This may release new transactions for dispatch
	c.inFlightMutex.L.Lock()
	defer c.inFlightMutex.L.Unlock()
	clear(c.inFlightTxns)
	dispatchingTransactions := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Dispatched})
	for _, txn := range dispatchingTransactions {
		if txn.HasDispatchedPublicTransaction() {
			// We don't count transactions that result in new private transactions or prepared transactions
			c.inFlightTxns[txn.GetID()] = txn
		}
	}
	log.L(ctx).Debugf("coordinator has %d dispatching transactions", len(c.inFlightTxns))
	c.inFlightMutex.Signal()
	return nil
}
