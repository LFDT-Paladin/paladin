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
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/google/uuid"
)

type dispatchLoop struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	stopped          chan struct{}
	dispatchQueue    chan *transaction.Transaction
	queueEvent       func(ctx context.Context, event common.Event)
	readyForDispatch func(context.Context, *transaction.Transaction)

	contractAddress  string
	inFlightMutex    *sync.Cond // TODO AM: I think this will need some rewriting but it overlaps with the transaction
	maxDispatchAhead int
	inFlightTxns     map[uuid.UUID]*transaction.Transaction
}

func newDispatchLoop(contractAddress string, maxDispatchAhead int, maxInflightTransactions int, queueEvent func(ctx context.Context, event common.Event), readyForDispatch func(context.Context, *transaction.Transaction)) *dispatchLoop {
	return &dispatchLoop{
		contractAddress:  contractAddress,
		dispatchQueue:    make(chan *transaction.Transaction, maxInflightTransactions),
		inFlightMutex:    sync.NewCond(&sync.Mutex{}),
		maxDispatchAhead: maxDispatchAhead,
		inFlightTxns:     make(map[uuid.UUID]*transaction.Transaction, maxDispatchAhead),
		queueEvent:       queueEvent,
		readyForDispatch: readyForDispatch,
	}
}

func (d *dispatchLoop) start(ctx context.Context) {
	if d.ctx == nil {
		d.ctx, d.cancelCtx = context.WithCancel(ctx)
		go d.run()
	}
}

func (d *dispatchLoop) stop() {
	if d.cancelCtx != nil {
		d.cancelCtx()
	}
}

func (d *dispatchLoop) addToQueue(tx *transaction.Transaction) {
	d.dispatchQueue <- tx
}

func (d *dispatchLoop) run() {
	defer close(d.stopped)
	dispatchedAhead := 0 // Number of transactions we've dispatched without confirming they are in the state machine's in-flight list
	log.L(d.ctx).Debugf("coordinator dispatch loop started for contract %s", d.contractAddress)

	for {
		select {
		case tx := <-d.dispatchQueue: // TODO AM: not sure this is immutable state - and probably doesn't need to be lowered
			log.L(d.ctx).Debugf("coordinator pulled transaction %s from the dispatch queue. In-flight count: %d, dispatched ahead: %d, max dispatch ahead: %d", tx.ID.String(), len(d.inFlightTxns), dispatchedAhead, d.maxDispatchAhead)

			d.inFlightMutex.L.Lock()

			// Too many in flight - wait for some to be confirmed
			for len(d.inFlightTxns)+dispatchedAhead >= d.maxDispatchAhead {
				d.inFlightMutex.Wait()
			}

			// Dispatch and then asynchronously update the state machine to State_Dispatched
			log.L(d.ctx).Debugf("submitting transaction %s for dispatch", tx.ID.String())
			d.readyForDispatch(d.ctx, tx)

			// Dispatched transactions that result in a chained private transaction don't count towards max dispatch ahead
			if tx.PreparedPrivateTransaction == nil {
				dispatchedAhead++
			}

			// Update the TX state machine
			d.queueEvent(d.ctx, &transaction.DispatchedEvent{
				BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
					TransactionID: tx.ID,
				},
			})

			// We almost never need to wait for the state machine's event loop to process the update to State_Dispatched
			// but if we hit the max dispatch ahead limit after dispatching this transaction we do, because we can't be sure
			// in-flight will be accurate on the next loop round
			if len(d.inFlightTxns)+dispatchedAhead >= d.maxDispatchAhead {
				for d.inFlightTxns[tx.ID] == nil {
					d.inFlightMutex.Wait()
				}
				dispatchedAhead = 0
			}
			d.inFlightMutex.L.Unlock()
		case <-d.ctx.Done():
			log.L(d.ctx).Debugf("coordinator dispatch loop for contract %s stopped", d.contractAddress)
			return
		}
	}
}
