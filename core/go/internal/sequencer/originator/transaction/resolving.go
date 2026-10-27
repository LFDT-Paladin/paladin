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
package transaction

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// guard_HasRequiredVerifiers reports whether the transaction has any verifiers that need resolving
// before it can be delegated. A transaction with none skips State_Resolving entirely.
func guard_HasRequiredVerifiers(_ context.Context, t *originatorTransaction) bool {
	return len(t.pt.PreAssembly.GetRequiredVerifiers()) > 0
}

// action_ResolveVerifiers spawns a background goroutine that resolves the required verifiers and queues
// the result back to the originator. Resolving before delegation keeps verifier resolution off the
// coordinator's serialized assembly window: assembly reads the stored results with zero resolution work.
//
// The goroutine must not mutate txn.pt — it only reads the required verifiers captured here and queues an
// event carrying the resolved slice. The write to txn.pt.ResolvedVerifiers happens on the state-machine
// goroutine in action_VerifiersResolved.
func action_ResolveVerifiers(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	txID := txn.pt.ID
	requiredVerifiers := txn.pt.PreAssembly.GetRequiredVerifiers()
	go txn.handleResolveVerifiers(ctx, txID, requiredVerifiers)
	return nil
}

func (txn *originatorTransaction) handleResolveVerifiers(ctx context.Context, txID uuid.UUID, requiredVerifiers []*prototk.ResolveVerifierRequest) {
	resolvedVerifiers, err := txn.engineIntegration.ResolveVerifiers(ctx, requiredVerifiers)
	if err != nil {
		log.L(ctx).Errorf("failed to resolve verifiers for transaction %s: %s", txID, err)
		txn.queueEventForOriginator(ctx, &VerifierResolutionFailedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		})
		return
	}
	log.L(ctx).Debugf("resolved %d verifiers for transaction %s", len(resolvedVerifiers), txID)
	txn.queueEventForOriginator(ctx, &VerifiersResolvedEvent{
		BaseEvent:         BaseEvent{TransactionID: txID},
		ResolvedVerifiers: resolvedVerifiers,
	})
}

// action_VerifiersResolved stores the resolved verifiers on the local transaction. This is the only place
// the originator writes txn.pt.ResolvedVerifiers; assembly consumes them from here. The field is in-memory
// only and never leaves the node — no inbound coordinator message ever writes it.
func action_VerifiersResolved(_ context.Context, txn *originatorTransaction, event common.Event) error {
	e := event.(*VerifiersResolvedEvent)
	txn.pt.ResolvedVerifiers = e.ResolvedVerifiers
	return nil
}

// action_ScheduleResolveRetry arms a delayed retry of verifier resolution. Used when resolution fails
// (e.g. a remote verifier's node is offline) so the transaction retries on a backoff timer before
// delegation rather than looping tightly or burning a coordinator assembly slot.
func action_ScheduleResolveRetry(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	txn.scheduleResolveRetry(ctx)
	return nil
}

// action_CancelResolveRetry cancels any pending resolution retry timer. Run when leaving State_Resolving
// so a retry can never fire after the transaction has advanced or finalized. No-op if none is scheduled.
func action_CancelResolveRetry(_ context.Context, txn *originatorTransaction, _ common.Event) error {
	txn.clearResolveRetrySchedule()
	return nil
}

func (txn *originatorTransaction) scheduleResolveRetry(ctx context.Context) {
	txn.clearResolveRetrySchedule()
	txID := txn.pt.ID
	txn.cancelResolveRetry = txn.clock.ScheduleTimer(ctx, txn.resolveRetryBackoff, func() {
		txn.queueEventForOriginator(ctx, &VerifierResolutionRetryEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		})
	})
}

func (txn *originatorTransaction) clearResolveRetrySchedule() {
	if txn.cancelResolveRetry != nil {
		txn.cancelResolveRetry()
		txn.cancelResolveRetry = nil
	}
}
