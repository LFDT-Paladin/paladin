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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

func guard_HasFinalizingGracePeriodPassedSinceStateChange(ctx context.Context, txn *coordinatorTransaction) bool {
	// Has this transaction been in the same state for longer than the finalizing grace period?
	// most useful to know this once we have reached one of the terminal states - Reverted or Committed
	return txn.heartbeatIntervalsSinceStateChange >= txn.finalizingGracePeriod
}

func guard_HasConfirmedLockRetentionGracePeriodPassedSinceStateChange(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.heartbeatIntervalsSinceStateChange >= txn.confirmedLockRetentionGracePeriod
}

func action_ResetConfirmedTransactionLocksOnce(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	if txn.confirmedLocksReleased {
		return nil
	}
	log.L(ctx).Debugf("releasing confirmed transaction locks for %s", txn.pt.ID.String())
	txn.grapher.ForgetLocks(txn.pt.ID)
	txn.confirmedLocksReleased = true
	return nil
}

// action_FinalizeAsUnknownByOriginator is called when the originator reports that it doesn't recognize
// a transaction. The most likely cause is that the transaction reached a terminal state (e.g. reverted
// during assembly) but the response was lost, and the transaction has since been removed from memory
// on the originator after cleanup. The coordinator should clean up this transaction.
func action_FinalizeAsUnknownByOriginator(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Warnf("action_FinalizeAsUnknownByOriginator - transaction %s reported as unknown by originator", txn.pt.ID)
	return txn.finalizeAsUnknownByOriginator(ctx)
}

func (t *coordinatorTransaction) finalizeAsUnknownByOriginator(_ context.Context) error {
	t.clearTimeoutSchedules()
	// Note: ResetTransactions is not called here because Event_TransactionUnknownByOriginator
	// is only handled in State_Assembling, which is before WriteLockStatesForTransaction has
	// been called -- so no creatingStates or txLocks exist in the domain context for this
	// transaction's current assembly attempt. If the state machine is ever extended to handle
	// this event in post-assembly states, ResetTransactions must be added here.
	return nil
}
