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
	"github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
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
		t.revertCount++
		if len(e.RevertReason) == 0 {
			// Off-chain revert (for example a chained assembly failure propagated from another TX).
			// This is not a base-ledger revert, so do not route through domain retryability logic.
			t.revertOnChain = nil
			t.decodedRevertReason = e.FailureMessage
			t.lastCanRetryRevert = false
		} else {
			t.revertOnChain = &e.OnChain
			retryable, decodedReason, err := t.domainAPI.IsBaseLedgerRevertRetryable(ctx, t.revertReason)
			if err != nil {
				log.L(ctx).Errorf("error checking if revert is retryable for transaction %s, treating as non-retryable: %s", t.pt.ID.String(), err)
				retryable = false
			}
			if decodedReason != "" {
				// Keep coordinator-originated decode text aligned with tx manager failure formatting.
				// This could be perceived as a misuse of another components error code, but the alternatives are
				// - have a separate error code here- but why should the user care that we did the decoding in a different place
				// - stop decoding the revert reason in the domain so we don't have two decode place- but this might result in us
				//   decoding fewer reverts, since transaction manager doesn't necessarily have each domain's ABI
				t.decodedRevertReason = i18n.NewError(ctx, msgs.MsgTxMgrRevertedDecodedData, decodedReason).Error()
			} else {
				t.decodedRevertReason = ""
			}
			if decodedReason == "" && e.FailureMessage != "" {
				// Chained transaction outcomes can carry a decoded failure string from the child domain.
				// Use it when this coordinator cannot decode revert bytes.
				t.decodedRevertReason = e.FailureMessage
			}
			t.lastCanRetryRevert = retryable && t.revertCount <= t.baseLedgerRevertRetryThreshold
			log.L(ctx).Debugf("transaction %s base ledger reverted with \"%s\" (%s) (count=%d, retryable=%t, threshold=%d, canRetry=%t)",
				t.pt.ID.String(), t.decodedRevertReason, t.revertReason.String(), t.revertCount, retryable, t.baseLedgerRevertRetryThreshold, t.lastCanRetryRevert)
		}
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
	return t.transportWriter.SendTransactionConfirmed(
		ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce,
		engine.TransactionConfirmed_OUTCOME_SUCCESS, nil, "", false,
	)
}

func action_NotifyOriginatorOfRetryableRevert(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*ConfirmedRevertedEvent)
	return t.transportWriter.SendTransactionConfirmed(
		ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce,
		engine.TransactionConfirmed_OUTCOME_REVERTED, t.revertReason, t.decodedRevertReason, true,
	)
}

func action_NotifyOriginatorOfNonRetryableRevert(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*ConfirmedRevertedEvent)
	return t.transportWriter.SendTransactionConfirmed(
		ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce,
		engine.TransactionConfirmed_OUTCOME_REVERTED, t.revertReason, t.decodedRevertReason, false,
	)
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

func action_NotifyDependentsOfSuccessfulConfirmation(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("notifying dependents of successful confirmation for transaction %s", txn.pt.ID.String())
	if txn.confirmedLockRetentionGracePeriod == 0 {
		if err := action_ResetConfirmedTransactionLocksOnce(ctx, txn, nil); err != nil {
			return err
		}
	}
	return txn.notifyDependentsOfConfirmation(ctx)
}

func action_NotifyDependentsOfRevertedConfirmation(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
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
	for _, dependentId := range t.grapher.GetDependents(ctx, t.pt.ID) {
		dependent := t.getCoordinatorTransaction(ctx, dependentId)
		if dependent == nil {
			return i18n.NewError(ctx, msgs.MsgSequencerTransactionNotFound, dependentId)
		} else {
			err := dependent.HandleEvent(ctx, &DependencyReadyEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: dependent.GetPrivateTransaction().ID,
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error notifying dependent transaction %s of readiness of transaction %s: %s", dependent.GetPrivateTransaction().ID, t.pt.ID, err)
				return err
			}
		}
	}
	return nil
}

func (t *coordinatorTransaction) notifyDependentsOfRevertedConfirmation(ctx context.Context) error {
	log.L(ctx).Debugf("notifying dependents of reverted confirmation for transaction %s (dependents will repool)", t.pt.ID.String())
	for _, dependentId := range t.grapher.GetDependents(ctx, t.pt.ID) {
		dependent := t.getCoordinatorTransaction(ctx, dependentId)
		if dependent == nil {
			return i18n.NewError(ctx, msgs.MsgSequencerTransactionNotFound, dependentId)
		} else {
			err := dependent.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: dependent.GetPrivateTransaction().ID,
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error notifying dependent transaction %s of revert of transaction %s: %s", dependent.GetPrivateTransaction().ID, t.pt.ID, err)
				return err
			}
		}
	}
	return nil
}
