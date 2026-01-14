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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// stateupdate_HeartbeatInterval increments the heartbeat counter
func stateupdate_HeartbeatInterval(ctx context.Context, txn *Transaction, _ common.Event) error {
	log.L(ctx).Tracef("coordinator transaction %s (%s) increasing heartbeatIntervalsSinceStateChange to %d", txn.ID.String(), txn.GetCurrentState().String(), txn.heartbeatIntervalsSinceStateChange+1)
	txn.heartbeatIntervalsSinceStateChange++
	return nil
}

func guard_HasGracePeriodPassedSinceStateChange(ctx context.Context, txn *Transaction) bool {
	// Has this transaction been in the same state for longer than the finalizing grace period?
	// most useful to know this once we have reached one of the terminal states - Reverted or Committed
	return txn.heartbeatIntervalsSinceStateChange >= txn.finalizingGracePeriod
}

func action_Cleanup(ctx context.Context, txn *Transaction) error {
	log.L(ctx).Infof("action_Cleanup - cleaning up transaction %s", txn.ID.String())
	return txn.cleanup(ctx)
}

// action_FinalizeAsUnknownByOriginator is called when the originator reports that it doesn't recognize
// a transaction. The most likely cause is that the transaction reached a terminal state (e.g. reverted
// during assembly) but the response was lost, and the transaction has since been removed from memory
// on the originator after cleanup. The coordinator should clean up this transaction.
func action_FinalizeAsUnknownByOriginator(ctx context.Context, txn *Transaction) error {
	log.L(ctx).Warnf("action_FinalizeAsUnknownByOriginator - transaction %s reported as unknown by originator", txn.ID)
	return txn.finalizeAsUnknownByOriginator(ctx)
}

func (t *Transaction) finalizeAsUnknownByOriginator(ctx context.Context) error {
	t.cancelAssembleTimeoutSchedules()

	var tryFinalize func()
	tryFinalize = func() {
		t.syncPoints.QueueTransactionFinalize(ctx, t.Domain, pldtypes.EthAddress{}, t.originator, t.ID,
			"originator reported transaction as unknown",
			func(ctx context.Context) {
				log.L(ctx).Debugf("finalized transaction %s after unknown response from originator", t.ID)
			},
			func(ctx context.Context, err error) {
				log.L(ctx).Errorf("error finalizing transaction %s: %s", t.ID, err)
				tryFinalize()
			})
	}
	tryFinalize()
	return nil
}
