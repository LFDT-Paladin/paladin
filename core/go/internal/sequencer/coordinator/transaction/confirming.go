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

func guard_HasRevertReason(ctx context.Context, txn *CoordinatorTransaction) bool {
	return txn.revertReason.String() != ""
}

func action_Confirmed(ctx context.Context, t *CoordinatorTransaction, event common.Event) error {
	e := event.(*ConfirmedEvent)
	t.revertReason = e.RevertReason
	return t.transportWriter.SendTransactionConfirmed(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce, e.RevertReason)
}

func action_NotifyDependantsOfConfirmation(ctx context.Context, txn *CoordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("action_NotifyOfConfirmation - notifying dependents of confirmation for transaction %s", txn.pt.ID.String())
	txn.engineIntegration.ResetTransactions(ctx, txn.pt.ID)
	return txn.notifyDependentsOfConfirmation(ctx)
}

func (t *CoordinatorTransaction) notifyDependentsOfConfirmation(ctx context.Context) error {
	if log.IsTraceEnabled() {
		t.traceDispatch(ctx)
	}

	// this function is called when the transaction enters the confirmed state
	// and we have a duty to inform all the transactions that are dependent on us that we are ready in case they are otherwise ready and are blocked waiting for us
	for _, dependentId := range t.dependencies.PrereqOf {
		dependent := t.grapher.TransactionByID(ctx, dependentId)
		if dependent == nil {
			return i18n.NewError(ctx, msgs.MsgSequencerGrapherDependencyNotFound)
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
