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
	"github.com/google/uuid"
)

func (tm *txManager) notifyDependentTransactions(ctx context.Context, dbTX persistence.DBTX, receipts []*transactionReceipt) error {
	dependentFailureReceipts := make([]*components.ReceiptInput, 0)
	for _, receipt := range receipts {
		deps, err := tm.getTransactionDependenciesWithinTX(ctx, receipt.TransactionID, dbTX)
		if err != nil {
			return err
		}
		for _, dep := range deps.PrereqOf {
			if receipt.Success {
				resolvedTx, err := tm.getResolvedTransactionByIDWithinTX(ctx, dep, dbTX)
				if err != nil {
					return err
				}
				dbTX.AddPostCommit(func(ctx context.Context) {
					log.L(ctx).Debugf("Dependency %s successful, resuming TX %s", receipt.TransactionID, dep)
					if resumeErr := tm.sequencerMgr.HandleTxResume(ctx, &components.ValidatedTransaction{
						ResolvedTransaction: *resolvedTx,
					}); resumeErr != nil {
						log.L(ctx).Error(i18n.WrapError(ctx, resumeErr, msgs.MsgTxMgrResumeTXFailed, resolvedTx.Transaction.ID))
					}
				})
			} else {
				// Check if the dependent already has a receipt to prevent unbounded recursion with circular dependencies
				existingReceipt, err := tm.GetTransactionReceiptByID(ctx, dep)
				if err != nil {
					log.L(ctx).Errorf("Failed to check existing receipt for dependent TX %s: %s", dep, err)
					continue
				}
				if existingReceipt != nil {
					log.L(ctx).Debugf("Dependent TX %s already has a receipt, skipping failure propagation", dep)
					continue
				}
				log.L(ctx).Debugf("TX %s failed, inserting failure receipt for dependent TX %s", receipt.TransactionID, dep)
				dependentFailureReceipts = append(dependentFailureReceipts, &components.ReceiptInput{
					TransactionID:  dep,
					ReceiptType:    components.RT_FailedWithMessage,
					FailureMessage: i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgTxMgrDependencyFailed), receipt.TransactionID),
				})
			}
		}
	}

	if len(dependentFailureReceipts) > 0 {
		return tm.FinalizeTransactions(ctx, dbTX, dependentFailureReceipts)
	}
	return nil
}

func (tm *txManager) BlockedByDependencies(ctx context.Context, tx *components.ValidatedTransaction) (bool, error) {
	for _, dep := range tx.DependsOn {
		depTXReceipt, err := tm.GetTransactionReceiptByID(ctx, dep)
		if err != nil {
			return true, err
		}
		if depTXReceipt == nil {
			log.L(ctx).Debugf("Transaction %s has outstanding dependency %s", tx.Transaction.ID, dep)
			return true, nil
		}
		if !depTXReceipt.Success {
			log.L(ctx).Infof("Transaction %s dependency %s has failed - propagating failure", tx.Transaction.ID, dep)
			if finalizeErr := tm.failDependentTransaction(ctx, tx, dep); finalizeErr != nil {
				log.L(ctx).Errorf(i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgTxMgrFailDependentTXFailed), tx.Transaction.ID, finalizeErr))
			}
			return true, nil
		}
	}
	return false, nil
}

func (tm *txManager) failDependentTransaction(ctx context.Context, tx *components.ValidatedTransaction, failedDep uuid.UUID) error {
	return tm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return tm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID:  *tx.Transaction.ID,
				ReceiptType:    components.RT_FailedWithMessage,
				FailureMessage: i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgTxMgrDependencyFailed), failedDep),
			},
		})
	})
}
