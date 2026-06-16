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

package statemgr

import (
	"context"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"gorm.io/gorm/clause"
)

// pendingPrivateStateData tracks confirmed states for opted-in domains that are still
// waiting for private data. One row exists per state ID; rows are deleted when
// the corresponding state data arrives.
type pendingPrivateStateData struct {
	StateID     string `gorm:"column:state_id;primaryKey"`
	Contract    string `gorm:"column:contract"`
	BlockNumber int64  `gorm:"column:block_number"`
}

func (pendingPrivateStateData) TableName() string {
	return "pending_private_state_data"
}

// WritePendingPrivateStateDataBatch is called from the event indexer within the event-stream DB
// transaction after the entire event batch has been processed for a domain that supports the
// completion index. It makes a single getStateIDsMissingPrivateData query for all state IDs
// in the batch, then writes one row per missing state.
func (ss *stateManager) WritePendingPrivateStateDataBatch(ctx context.Context, dbTX persistence.DBTX, domainName string, states []components.PendingPrivateStateDataEntry) error {
	if len(states) == 0 {
		return nil
	}

	allStateIDs := make([]pldtypes.HexBytes, len(states))
	for i, s := range states {
		allStateIDs[i] = s.StateID
	}

	missingIDs, err := ss.getStateIDsMissingPrivateData(ctx, dbTX, domainName, allStateIDs)
	if err != nil {
		return err
	}
	if len(missingIDs) == 0 {
		return nil
	}

	stateIndex := make(map[string]*components.PendingPrivateStateDataEntry, len(states))
	for i := range states {
		stateIndex[states[i].StateID.String()] = &states[i]
	}

	rows := make([]*pendingPrivateStateData, 0, len(missingIDs))
	for _, id := range missingIDs {
		entry := stateIndex[id.String()]
		if entry == nil {
			continue
		}
		rows = append(rows, &pendingPrivateStateData{
			StateID:     id.String(),
			Contract:    entry.Contract.String(),
			BlockNumber: entry.BlockNumber,
		})
	}

	return dbTX.DB().
		WithContext(ctx).
		Clauses(clause.OnConflict{
			DoNothing: true,
		}).
		Create(rows).
		Error
}

// updatePendingPrivateStateData is called internally from writeStates after new states are written.
// It deletes rows whose state_id matches one of the arrived states.
func (ss *stateManager) updatePendingPrivateStateData(ctx context.Context, dbTX persistence.DBTX, arrivedStateIDs []pldtypes.HexBytes) error {
	if len(arrivedStateIDs) == 0 {
		return nil
	}

	arrivedStrs := make([]string, len(arrivedStateIDs))
	for i, id := range arrivedStateIDs {
		arrivedStrs[i] = id.String()
	}

	return dbTX.DB().
		WithContext(ctx).
		Where("state_id IN ?", arrivedStrs).
		Delete(&pendingPrivateStateData{}).
		Error
}

// CheckPendingPrivateStateDataForContract returns true if there are no outstanding rows for
// the given contract at or below the given block number.
func (ss *stateManager) CheckPendingPrivateStateDataForContract(ctx context.Context, dbTX persistence.DBTX, contract string, block int64) (bool, error) {
	var count int64
	err := dbTX.DB().
		WithContext(ctx).
		Model(&pendingPrivateStateData{}).
		Where("contract = ? AND block_number <= ?", contract, block).
		Count(&count).
		Error
	if err != nil {
		return false, err
	}
	return count == 0, nil
}
