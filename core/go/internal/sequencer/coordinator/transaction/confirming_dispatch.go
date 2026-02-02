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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
)

func (t *CoordinatorTransaction) applyDispatchConfirmation(_ context.Context, requestID uuid.UUID) error {
	t.pendingPreDispatchRequest = nil
	return nil
}

func (t *CoordinatorTransaction) sendPreDispatchRequest(ctx context.Context) error {

	if t.pendingPreDispatchRequest == nil {
		hash, err := t.hash(ctx)
		if err != nil {
			log.L(ctx).Debugf("error hashing transaction for dispatch confirmation request: %s", err)
			return err
		}
		t.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
			return t.transportWriter.SendPreDispatchRequest(
				ctx,
				t.originatorNode,
				idempotencyKey,
				t.pt.PreAssembly.TransactionSpecification,
				hash,
			)
		})
		t.cancelDispatchConfirmationRequestTimeoutSchedule = t.clock.ScheduleTimer(ctx, t.requestTimeout, func() {
			t.queueEventForCoordinator(ctx, &RequestTimeoutIntervalEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: t.pt.ID,
				},
			})
		})
	}

	sendErr := t.pendingPreDispatchRequest.Nudge(ctx)

	return sendErr

}

// Hash method of Transaction
func (t *CoordinatorTransaction) hash(ctx context.Context) (*pldtypes.Bytes32, error) {
	if t.pt == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot hash transaction without PrivateTransaction")
	}
	if t.pt.PostAssembly == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot hash transaction without PostAssembly")
	}

	// MRW TODO - MUST DO - this was relying on only signatures being present, but Pente contracts reject transactions that have both signatures and endorsements.
	// if len(t.pt.PostAssembly.Signatures) == 0 {
	// 	return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot hash transaction without at least one Signature")
	// }

	hash := sha3.NewLegacyKeccak256()

	if len(t.pt.PostAssembly.Signatures) != 0 {
		for _, signature := range t.pt.PostAssembly.Signatures {
			hash.Write(signature.Payload)
		}
	}

	var h32 pldtypes.Bytes32
	_ = hash.Sum(h32[0:0])
	return &h32, nil

}

func (t *CoordinatorTransaction) nudgePreDispatchRequest(ctx context.Context) error {
	if t.pendingPreDispatchRequest == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "nudgePreDispatchRequest called with no pending request")
	}

	return t.pendingPreDispatchRequest.Nudge(ctx)
}

func validator_MatchesPendingPreDispatchRequest(ctx context.Context, txn *CoordinatorTransaction, event common.Event) (bool, error) {
	switch event := event.(type) {
	case *DispatchRequestApprovedEvent:
		return txn.pendingPreDispatchRequest != nil && txn.pendingPreDispatchRequest.IdempotencyKey() == event.RequestID, nil
	}
	return false, nil
}

func action_DispatchRequestApproved(ctx context.Context, t *CoordinatorTransaction, event common.Event) error {
	e := event.(*DispatchRequestApprovedEvent)
	return t.applyDispatchConfirmation(ctx, e.RequestID)
}

func action_SendPreDispatchRequest(ctx context.Context, txn *CoordinatorTransaction, _ common.Event) error {
	return txn.sendPreDispatchRequest(ctx)
}

func action_NudgePreDispatchRequest(ctx context.Context, txn *CoordinatorTransaction, _ common.Event) error {
	return txn.nudgePreDispatchRequest(ctx)
}
