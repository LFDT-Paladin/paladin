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
	for _, txn := range transactions {

		if c.transactionsByID[txn.ID] != nil {
			log.L(ctx).Debugf("transaction %s already being coordinated", txn.ID.String())
			continue
		}

		if len(c.transactionsByID) >= c.maxInflightTransactions {
			// We'll rely on the fact that originators retry incomplete transactions periodically
			return i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, c.maxInflightTransactions)
		}

		// The newly delegated TX might be after the restart of an originator, for which we've already
		// instantiated a chained TX
		hasChainedTransaction, err := c.txManager.HasChainedTransaction(ctx, txn.ID)
		if err != nil {
			log.L(ctx).Errorf("error checking for chained transaction: %v", err)
			return err
		}
		if hasChainedTransaction {
			log.L(ctx).Debugf("chained transaction %s found", txn.ID.String())
		}

		newTransaction, err := transaction.NewTransaction(
			ctx,
			originator,
			txn,
			hasChainedTransaction,
			c.transportWriter,
			c.clock,
			c.QueueEvent,
			c.engineIntegration,
			c.syncPoints,
			c.requestTimeout,
			c.assembleTimeout,
			c.closingGracePeriod,
			c.domainAPI.Domain().FixedSigningIdentity(),
			c.domainAPI.ContractConfig().GetSubmitterSelection(),
			c.grapher,
			c.metrics,
		)
		if err != nil {
			log.L(ctx).Errorf("error creating transaction: %v", err)
			return err
		}

		c.transactionsByID[txn.ID] = newTransaction
		c.metrics.IncCoordinatingTransactions()

		receivedEvent := &transaction.ReceivedEvent{}
		receivedEvent.TransactionID = txn.ID

		err = c.transactionsByID[txn.ID].HandleEvent(ctx, receivedEvent)
		if err != nil {
			log.L(ctx).Errorf("error handling ReceivedEvent for transaction %s: %v", txn.ID.String(), err)
			return err
		}
	}
	return nil
}
