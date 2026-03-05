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
package transaction

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

func guard_HasRevertReason(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.revertReason.String() != ""
}

func action_RecordConfirmationDetails(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*ConfirmedEvent)
	if t.latestSubmissionHash == nil {
		// The transaction created a chained private transaction so there is no hash to compare
		log.L(ctx).Debugf("transaction %s confirmed with nil dispatch hash (confirmed hash of chained TX %s)", t.pt.ID.String(), e.Hash.String())
	} else if *t.latestSubmissionHash != e.Hash {
		// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
		// It is interesting so we log it but either way, this must be the transaction that we are looking for because the block indexer correlates with transaction IDs
		log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected. Dispatch hash %s, confirmed hash %s", t.pt.ID.String(), t.latestSubmissionHash, e.Hash.String())
	}

	t.revertReason = e.RevertReason
	return nil
}

func action_NotifyConfirmed(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*ConfirmedEvent)

	return t.transportWriter.SendTransactionConfirmed(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce, e.RevertReason)
}

func action_NotifyDependantsOfConfirmation(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("action_NotifyOfConfirmation - notifying dependents of confirmation for transaction %s", txn.pt.ID.String())
	if txn.confirmedLockRetentionGracePeriod == 0 {
		if err := action_ResetConfirmedTransactionLocksOnce(ctx, txn, nil); err != nil {
			return err
		}
	}
	return txn.notifyDependentsOfConfirmation(ctx)
}

func (t *coordinatorTransaction) notifyDependentsOfConfirmation(ctx context.Context) error {
	if log.IsTraceEnabled() {
		t.traceDispatch(ctx)
	}

	// this function is called when the transaction enters the confirmed state
	// and we have a duty to inform all the transactions that are dependent on us that we are ready in case they are otherwise ready and are blocked waiting for us
	for _, dependentId := range t.dependencies.PrereqOf {
		dependent := t.grapher.TransactionByID(ctx, dependentId)
		if dependent == nil {
			return i18n.NewError(ctx, msgs.MsgSequencerGrapherDependencyNotFound, dependentId)
		} else {
			err := dependent.HandleEvent(ctx, &DependencyReadyEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: dependent.pt.ID,
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error notifying dependent transaction %s of readiness of transaction %s: %s", dependent.pt.ID, t.pt.ID, err)
				return err
			}
		}
	}
	return nil
}
