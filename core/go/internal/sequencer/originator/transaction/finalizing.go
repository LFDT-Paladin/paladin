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

func (e *FinalizeEvent) ApplyToTransaction(ctx context.Context, t *Transaction) error {
	// No internal state changes needed - this event just triggers the transition to State_Final
	return nil
}

// action_QueueFinalizeEvent queues a FinalizeEvent to trigger cleanup.
// This is called when entering State_Confirmed or State_Reverted.
func action_QueueFinalizeEvent(ctx context.Context, txn *Transaction) error {
	log.L(ctx).Debugf("action_QueueFinalizeEvent - queueing finalize event for transaction %s", txn.ID.String())
	event := &FinalizeEvent{
		TransactionID: txn.ID,
	}
	txn.queueEventForOriginator(ctx, event)
	return nil
}

// action_Cleanup removes the transaction from memory.
// This is called when entering State_Final.
func action_Cleanup(ctx context.Context, txn *Transaction) error {
	log.L(ctx).Infof("action_Cleanup - cleaning up originator transaction %s", txn.ID.String())
	if txn.onCleanup != nil {
		txn.onCleanup(ctx)
	}
	return nil
}
