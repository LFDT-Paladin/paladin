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
	"github.com/google/uuid"
)

// FinalizeEvent is an internal event that triggers cleanup of a transaction
// that has reached a terminal state (Confirmed or Reverted).
type FinalizeEvent struct {
	common.BaseEvent
	TransactionID uuid.UUID
}

func (e *FinalizeEvent) Type() EventType {
	return Event_Finalize
}

func (e *FinalizeEvent) TypeString() string {
	return "Event_Finalize"
}

func (e *FinalizeEvent) GetTransactionID() uuid.UUID {
	return e.TransactionID
}

func action_NonceAssigned(ctx context.Context, t *OriginatorTransaction, event common.Event) error {
	e := event.(*NonceAssignedEvent)
	t.signerAddress = &e.SignerAddress //TODO should we throw an error if the signer address is already set to something else? Or remove these fields from this event?

	t.nonce = &e.Nonce
	return nil
}

func action_Submitted(ctx context.Context, t *OriginatorTransaction, event common.Event) error {
	e := event.(*SubmittedEvent)
	t.signerAddress = &e.SignerAddress //TODO should we throw an error if the signer address is already set to something else? Or remove these fields from this event?

	t.nonce = &e.Nonce
	t.latestSubmissionHash = &e.LatestSubmissionHash
	return nil
}

// action_QueueFinalizeEvent queues a FinalizeEvent to the originator; the originator routes it back to this transaction.
// This is called when entering State_Confirmed or State_Reverted.
func action_QueueFinalizeEvent(ctx context.Context, txn *OriginatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("action_QueueFinalizeEvent - queueing finalize event for transaction %s", txn.pt.ID.String())
	event := &FinalizeEvent{
		TransactionID: txn.pt.ID,
	}
	txn.queueEventForOriginator(ctx, event)
	return nil
}
