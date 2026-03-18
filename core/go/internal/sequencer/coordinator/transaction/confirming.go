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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

func guard_CanRetryRevert(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.lastCanRetryRevert
}

func action_RecordConfirmation(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	var hash pldtypes.Bytes32
	switch e := event.(type) {
	case *ConfirmedSuccessEvent:
		hash = e.Hash
		t.revertReason = nil
		t.decodedRevertReason = ""
		t.revertOnChain = nil
		t.lastCanRetryRevert = false
	case *ConfirmedRevertedEvent:
		hash = e.Hash
		t.revertReason = e.RevertReason
		t.revertOnChain = &e.OnChain
		t.revertCount++

		retryable, decodedReason, err := t.domainAPI.IsBaseLedgerRevertRetryable(ctx, t.revertReason)
		if err != nil {
			log.L(ctx).Errorf("error checking if revert is retryable for transaction %s, treating as non-retryable: %s", t.pt.ID.String(), err)
			retryable = false
		}
		t.decodedRevertReason = decodedReason
		t.lastCanRetryRevert = retryable && t.revertCount <= t.baseLedgerRevertRetryThreshold
		log.L(ctx).Debugf("transaction %s base ledger reverted with \"%s\" (%s) (count=%d, retryable=%t, threshold=%d, canRetry=%t)",
			t.pt.ID.String(), t.decodedRevertReason, t.revertReason.String(), t.revertCount, retryable, t.baseLedgerRevertRetryThreshold, t.lastCanRetryRevert)
	}

	if t.latestSubmissionHash == nil {
		// The transaction created a chained private transaction so there is no hash to compare
		log.L(ctx).Debugf("transaction %s confirmed with nil dispatch hash (confirmed hash of chained TX %s)", t.pt.ID.String(), hash.String())
	} else if *t.latestSubmissionHash != hash {
		// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
		// It is interesting so we log it but either way, this must be the transaction that we are looking for because the block indexer correlates with transaction IDs
		log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected. Dispatch hash %s, confirmed hash %s", t.pt.ID.String(), t.latestSubmissionHash, hash.String())
	}

	return nil
}

func action_NotifyOriginatorOfConfirmation(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*ConfirmedSuccessEvent)
	return t.transportWriter.SendTransactionConfirmed(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce, nil, false)
}

func action_NotifyOriginatorOfRetryableRevert(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*ConfirmedRevertedEvent)
	return t.transportWriter.SendTransactionConfirmed(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce, e.RevertReason, true)
}

func action_NotifyOriginatorOfNonRetryableRevert(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*ConfirmedRevertedEvent)
	return t.transportWriter.SendTransactionConfirmed(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce, e.RevertReason, false)
}

func action_FinalizeNonRetryableRevert(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	failureMessage := t.decodedRevertReason
	log.L(ctx).Infof("finalizing transaction %s as reverted (revertCount=%d): %s", t.pt.ID.String(), t.revertCount, failureMessage)
	t.syncPoints.QueueTransactionFinalize(ctx,
		&syncpoints.TransactionFinalizeRequest{
			Domain:          t.pt.Domain,
			ContractAddress: pldtypes.EthAddress{},
			Originator:      t.originator,
			TransactionID:   t.pt.ID,
			FailureMessage:  failureMessage,
			RevertData:      t.revertReason,
			OnChain:         t.revertOnChain,
		},
		func(ctx context.Context) {
			log.L(ctx).Debugf("finalized non-retryable revert for transaction %s", t.pt.ID)
		},
		func(ctx context.Context, err error) {
			log.L(ctx).Errorf("error finalizing non-retryable revert for transaction %s: %s", t.pt.ID, err)
		},
	)
	return nil
}

func action_NotifyDependantsOfSuccessfulConfirmation(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("notifying dependents of successful confirmation for transaction %s", txn.pt.ID.String())
	if txn.confirmedLockRetentionGracePeriod == 0 {
		if err := action_ResetConfirmedTransactionLocksOnce(ctx, txn, nil); err != nil {
			return err
		}
	}
	return txn.notifyDependentsOfConfirmation(ctx)
}

func action_NotifyDependantsOfRevertedConfirmation(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("notifying dependents of reverted confirmation for transaction %s", txn.pt.ID.String())
	if err := action_ResetConfirmedTransactionLocksOnce(ctx, txn, nil); err != nil {
		return err
	}
	return txn.notifyDependentsOfRevertedConfirmation(ctx)
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
func (t *coordinatorTransaction) notifyDependentsOfRevertedConfirmation(ctx context.Context) error {
	log.L(ctx).Debugf("notifying dependents of reverted confirmation for transaction %s (dependents will repool)", t.pt.ID.String())
	for _, dependentId := range t.dependencies.PrereqOf {
		dependent := t.grapher.TransactionByID(ctx, dependentId)
		if dependent == nil {
			return i18n.NewError(ctx, msgs.MsgSequencerGrapherDependencyNotFound, dependentId)
		} else {
			err := dependent.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: dependent.pt.ID,
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error notifying dependent transaction %s of revert of transaction %s: %s", dependent.pt.ID, t.pt.ID, err)
				return err
			}
		}
	}
	return nil
}
