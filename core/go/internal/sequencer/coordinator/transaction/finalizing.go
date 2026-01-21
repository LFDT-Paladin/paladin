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
func stateupdate_HeartbeatInterval(ctx context.Context, state *Transaction, _ *Transaction, _ *Transaction, _ common.Event) error {
	log.L(ctx).Tracef("coordinator transaction %s (%s) increasing heartbeatIntervalsSinceStateChange to %d", state.ID.String(), state.GetCurrentState().String(), state.heartbeatIntervalsSinceStateChange+1)
	state.heartbeatIntervalsSinceStateChange++
	return nil
}

// stateupdate_ResetHeartbeatCounter resets the heartbeat counter on state transitions
func stateupdate_ResetHeartbeatCounter(_ context.Context, state *Transaction, _ *Transaction, _ *Transaction, _ common.Event) error {
	state.heartbeatIntervalsSinceStateChange = 0
	return nil
}

func guard_HasGracePeriodPassedSinceStateChange(_ context.Context, reader *Transaction, _ *Transaction) bool {
	// Has this transaction been in the same state for longer than the finalizing grace period?
	// most useful to know this once we have reached one of the terminal states - Reverted or Committed
	// TODO AM: this is not using a proper Reader interface
	return reader.heartbeatIntervalsSinceStateChange >= reader.finalizingGracePeriod
}

// action_FinalizeAsUnknownByOriginator is called when the originator reports that it doesn't recognize
// a transaction. The most likely cause is that the transaction reached a terminal state (e.g. reverted
// during assembly) but the response was lost, and the transaction has since been removed from memory
// on the originator after cleanup. The coordinator should clean up this transaction.
func action_FinalizeAsUnknownByOriginator(ctx context.Context, reader *Transaction, _ *Transaction, _ *Transaction, _ common.Event) error {
	log.L(ctx).Warnf("action_FinalizeAsUnknownByOriginator - transaction %s reported as unknown by originator", reader.ID)
	// TODO AM: understand whether this is really an action or a state update
	return reader.finalizeAsUnknownByOriginator(ctx)
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
