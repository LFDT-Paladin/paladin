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
)

func (t *coordinatorTransaction) GetSnapshot(ctx context.Context) (*common.SnapshotPooledTransaction, *common.SnapshotDispatchedTransaction, *common.SnapshotConfirmedTransaction) {
	t.RLock()
	defer t.RUnlock()

	currentState := t.stateMachine.GetCurrentState()
	log.L(ctx).Debugf("next transaction to assess current status of %s. Current state: %s", t.pt.ID.String(), currentState.String())

	switch currentState {
	// pooled transactions are those that have been delegated but not yet dispatched, this includes
	// the various states from being delegated up to being ready for dispatch
	case State_Blocked,
		State_Confirming_Dispatchable,
		State_Endorsement_Gathering,
		State_PreAssembly_Blocked,
		State_Assembling,
		State_Pooled:
		return &common.SnapshotPooledTransaction{
			ID: t.pt.ID,
		}, nil, nil

	// State_Ready_For_Dispatch is already past the point of no return. It is as good as dispatched, just waiting for
	// the dispatcher thread to collect it so we include it in the dispatched transactions of the snapshot
	case State_Ready_For_Dispatch,
		State_Dispatched:
		dispatchedTransaction := &common.SnapshotDispatchedTransaction{
			SnapshotPooledTransaction: common.SnapshotPooledTransaction{
				ID:         t.pt.ID,
				Originator: t.originator,
			},
		}
		if t.signerAddress != nil {
			dispatchedTransaction.Signer = *t.signerAddress
			dispatchedTransaction.Nonce = t.nonce
			dispatchedTransaction.LatestSubmissionHash = t.latestSubmissionHash
		}
		return nil, dispatchedTransaction, nil

	case State_Confirmed:
		log.L(ctx).Debugf("heartbeat snapshot building, transaction ID %s is in State_Confirmed, sending to heartbeat receipients", t.pt.ID.String())
		confirmedTransaction := &common.SnapshotConfirmedTransaction{
			SnapshotDispatchedTransaction: common.SnapshotDispatchedTransaction{
				SnapshotPooledTransaction: common.SnapshotPooledTransaction{
					ID: t.pt.ID,
				},
				Nonce:                t.nonce,
				LatestSubmissionHash: t.latestSubmissionHash,
			},
			RevertReason: t.revertReason,
		}
		if t.signerAddress != nil {
			confirmedTransaction.Signer = *t.signerAddress
		}
		return nil, nil, confirmedTransaction
	}

	// Reverted/final/other states are excluded from snapshots.
	return nil, nil, nil
}
