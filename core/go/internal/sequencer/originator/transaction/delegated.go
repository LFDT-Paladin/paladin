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

func action_Delegated(ctx context.Context, t *Transaction, event common.Event) error {
	e := event.(*DelegatedEvent)
	if e.Coordinator == "" {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "transaction delegate cannot be set to an empty node identity")
	}
	t.currentDelegate = e.Coordinator
	t.updateLastDelegatedTime()
	return nil
}

func (t *Transaction) updateLastDelegatedTime() {
	t.lastDelegatedTime = ptrTo(common.RealClock().Now())
}

func action_SendPreDispatchResponse(ctx context.Context, txn *Transaction, _ common.Event) error {
	// MRW TODO - sending a dispatch response should be based on some sanity check that we are OK for the coordinator
	// to proceed to dispatch. Not sure if that belongs here, or somewhere else, but at the moment we always reply OK/proceed.
	return txn.transportWriter.SendPreDispatchResponse(ctx, txn.currentDelegate, txn.latestPreDispatchRequestID, txn.pt.PreAssembly.TransactionSpecification)
}

// Validate that the assemble request matches the current delegate
func validator_AssembleRequestMatches(ctx context.Context, txn *Transaction, event common.Event) (bool, error) {
	assembleRequestEvent, ok := event.(*AssembleRequestReceivedEvent)
	if !ok {
		log.L(ctx).Errorf("expected event type *AssembleRequestReceivedEvent, got %T", event)
		return false, nil
	}

	log.L(ctx).Debugf("originator transaction validating assemble request - event coordinator %s, TX current delegate = %s", assembleRequestEvent.Coordinator, txn.currentDelegate)
	return assembleRequestEvent.Coordinator == txn.currentDelegate, nil

}

func validator_PreDispatchRequestMatchesAssembledDelegation(ctx context.Context, txn *Transaction, event common.Event) (bool, error) {
	preDispatchRequestEvent, ok := event.(*PreDispatchRequestReceivedEvent)
	if !ok {
		log.L(ctx).Errorf("expected event type *PreDispatchRequestReceivedEvent, got %T", event)
		return false, nil
	}
	txnHash, err := txn.hashInternal(ctx)
	if err != nil {
		log.L(ctx).Errorf("error hashing transaction: %s", err)
		return false, err
	}
	if preDispatchRequestEvent.Coordinator != txn.currentDelegate {
		log.L(ctx).Debugf("DispatchConfirmationRequest invalid for transaction %s.  Expected coordinator %s, got %s", txn.pt.ID.String(), txn.currentDelegate, preDispatchRequestEvent.Coordinator)
		return false, nil
	}
	if !txnHash.Equals(preDispatchRequestEvent.PostAssemblyHash) {
		log.L(ctx).Debugf("DispatchConfirmationRequest invalid for transaction %s.  Transaction hash does not match.", txn.pt.ID.String())
		return false, nil
	}

	return true, nil
}
