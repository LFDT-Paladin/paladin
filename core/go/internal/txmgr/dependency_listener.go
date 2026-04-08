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

package txmgr

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
)

func (tm *txManager) notifyDependentTransactions(ctx context.Context, dbTX persistence.DBTX, receipts []*transactionReceipt) error {
	log := log.L(tm.bgCtx)
	dependentFailureReceipts := make([]*components.ReceiptInput, 0)
	for _, receipt := range receipts {
		deps, err := tm.getTransactionDependenciesWithinTX(tm.bgCtx, receipt.TransactionID, dbTX)
		if err != nil {
			return err
		}
		for _, dep := range deps.PrereqOf {
			if receipt.Success {
				resolvedTx, err := tm.getResolvedTransactionByIDWithinTX(ctx, dep, dbTX)
				if err != nil {
					return err
				}
				// Add the necessary post-commits to tap the sequencer manager once the DB is updated
				dbTX.AddPostCommit(func(ctx context.Context) {
					log.Debugf("Dependency %s successful, resuming TX %s", dep, receipt.TransactionID)
					err = tm.sequencerMgr.HandleTxResume(ctx, &components.ValidatedTransaction{
						ResolvedTransaction: *resolvedTx,
					})
					if err != nil {
						// Log and continue
						log.Error(i18n.WrapError(ctx, err, msgs.MsgTxMgrResumeTXFailed, resolvedTx.Transaction.ID))
					}
				})
			} else {
				log.Debugf("TX %s failed, inserting failure receipt for dependent TX %s", receipt.TransactionID, dep)
				dependentFailureReceipts = append(dependentFailureReceipts, &components.ReceiptInput{
					TransactionID:  dep,
					ReceiptType:    components.RT_FailedWithMessage,
					FailureMessage: "Transaction dependency failed",
				})
			}
		}
	}

	if len(dependentFailureReceipts) > 0 {
		return tm.FinalizeTransactions(ctx, dbTX, dependentFailureReceipts)
	}
	return nil
}

func (tm *txManager) BlockedByDependencies(ctx context.Context, dbTX persistence.DBTX, tx *components.ValidatedTransaction) (bool, error) {
	for _, dep := range tx.DependsOn {
		depTXReceipt, err := tm.getTransactionReceiptByIDWithTX(ctx, dbTX, dep)
		if err != nil {
			// Fail safe - if we can't check the status of a dependency we should assume we're waiting for it.
			return true, err
		}
		if depTXReceipt == nil || !depTXReceipt.Success {
			// Dependency TX receipt isn't present or it failed. Either way the caller must not progress the TX in question
			log.L(ctx).Debugf("Transaction %s has outstanding dependency %s", tx.Transaction.ID, dep)
			return true, nil
		}
	}
	return false, nil
}
