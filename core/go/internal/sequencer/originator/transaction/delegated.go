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
	"github.com/google/uuid"
)

func action_Delegated(ctx context.Context, t *originatorTransaction, event common.Event) error {
	e := event.(*DelegatedEvent)
	if e.Coordinator == "" {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "transaction delegate cannot be set to an empty node identity")
	}
	t.currentDelegate = e.Coordinator
	t.updateLastDelegatedTime()
	return nil
}

// action_ResetDelegationState clears all assembly and dispatch state accumulated for the previous
// coordinator. Called alongside action_Delegated when a transaction is re-delegated to a new coordinator.
func action_ResetDelegationState(_ context.Context, t *originatorTransaction, _ common.Event) error {
	t.latestAssembleRequest = nil
	t.latestFulfilledAssembleRequestID = uuid.Nil
	t.latestPreDispatchRequestID = uuid.Nil
	// TODO: do we want to clear these? Clearing them doesn't stop the transaction from being confirmed
	// which is the most likely outcome as the old coordinator continues to flush, but it does lose potentially
	// useful information about the last submission
	t.signerAddress = nil
	t.latestSubmissionHash = nil
	t.nonce = nil
	return nil
}

func validator_CoordinatorIsCurrentDelegate(ctx context.Context, t *originatorTransaction, event common.Event) (bool, error) {
	if e, ok := event.(EventWithCoordinator); ok && e.GetCoordinator() == t.currentDelegate {
		return true, nil
	}
	return false, nil
}

func (t *originatorTransaction) updateLastDelegatedTime() {
	t.lastDelegatedTime = ptrTo(common.RealClock().Now())
}

func action_SendPreDispatchResponse(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	// MRW TODO - sending a dispatch response should be based on some sanity check that we are OK for the coordinator
	// to proceed to dispatch. Not sure if that belongs here, or somewhere else, but at the moment we always reply OK/proceed.
	return txn.transportWriter.SendPreDispatchResponse(ctx, txn.currentDelegate, txn.latestPreDispatchRequestID, txn.pt.PreAssembly.TransactionSpecification)
}

func validator_AssembleRequestMatches(ctx context.Context, txn *originatorTransaction, event common.Event) (bool, error) {
	assembleRequestEvent, ok := event.(*AssembleRequestReceivedEvent)
	if !ok {
		log.L(ctx).Errorf("expected event type *AssembleRequestReceivedEvent, got %T", event)
		return false, nil
	}
	if assembleRequestEvent.Coordinator != txn.currentDelegate {
		log.L(ctx).Debugf("originator transaction rejecting assemble request - event coordinator %s, TX current delegate = %s", assembleRequestEvent.Coordinator, txn.currentDelegate)
		return false, nil
	}
	return true, nil
}

// action_SendNotActiveCoordinatorForAssembleRequest proactively notifies a non-active coordinator
// that it should evict this transaction rather than waiting indefinitely for a response.
func action_SendNotActiveCoordinatorForAssembleRequest(ctx context.Context, txn *originatorTransaction, event common.Event) error {
	assembleRequestEvent := event.(*AssembleRequestReceivedEvent)
	if err := txn.transportWriter.SendNotActiveCoordinator(ctx, assembleRequestEvent.Coordinator, txn.pt.ID); err != nil {
		log.L(ctx).Warnf("failed to send not-active-coordinator rejection to %s: %s", assembleRequestEvent.Coordinator, err)
	}
	return nil
}

func validator_PreDispatchRequestMatchesAssembledDelegation(ctx context.Context, txn *originatorTransaction, event common.Event) (bool, error) {
	preDispatchRequestEvent, ok := event.(*PreDispatchRequestReceivedEvent)
	if !ok {
		log.L(ctx).Errorf("expected event type *PreDispatchRequestReceivedEvent, got %T", event)
		return false, nil
	}
	if preDispatchRequestEvent.Coordinator != txn.currentDelegate {
		log.L(ctx).Debugf("DispatchConfirmationRequest invalid for transaction %s. Expected coordinator %s, got %s", txn.pt.ID.String(), txn.currentDelegate, preDispatchRequestEvent.Coordinator)
		if err := txn.transportWriter.SendNotActiveCoordinator(ctx, preDispatchRequestEvent.Coordinator, txn.pt.ID); err != nil {
			log.L(ctx).Warnf("failed to send not-active-coordinator rejection to %s: %s", preDispatchRequestEvent.Coordinator, err)
		}
		return false, nil
	}
	txnHash, err := txn.hashInternal(ctx)
	if err != nil {
		log.L(ctx).Errorf("error hashing transaction: %s", err)
		return false, err
	}
	if !txnHash.Equals(preDispatchRequestEvent.PostAssemblyHash) {
		// TODO: Should be be rejecting the dispatch here rather than leaving it to time out on the coordinator?
		log.L(ctx).Debugf("DispatchConfirmationRequest invalid for transaction %s. Transaction hash does not match.", txn.pt.ID.String())
		return false, nil
	}
	return true, nil
}
