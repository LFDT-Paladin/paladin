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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
)

func (c *coordinator) selectNextTransactionToAssemble(ctx context.Context) error {
	log.L(ctx).Trace("selecting next transaction to assemble")
	txn := c.PopNextPooledTransaction(ctx)
	if txn == nil {
		log.L(ctx).Info("no transaction found to process")
		return nil
	}

	transactionSelectedEvent := &transaction.SelectedEvent{}
	transactionSelectedEvent.TransactionID = txn.GetID()
	err := txn.HandleEvent(ctx, transactionSelectedEvent)
	return err

}

func (c *coordinator) AddTransactionToBackOfPool(ctx context.Context, txn *transaction.Transaction) {
	// Check if transaction is already in the pool
	// This makes the function safe to call multiple times, albeit not strictly idempotently
	for _, pooledTxn := range c.pooledTransactions {
		if pooledTxn.GetID() == txn.GetID() {
			return
		}
	}
	c.pooledTransactions = append(c.pooledTransactions, txn)
}

func (c *coordinator) PopNextPooledTransaction(ctx context.Context) *transaction.Transaction {
	if len(c.pooledTransactions) == 0 {
		return nil
	}
	nextPooledTx := c.pooledTransactions[0]
	c.pooledTransactions = c.pooledTransactions[1:]
	return nextPooledTx
}
