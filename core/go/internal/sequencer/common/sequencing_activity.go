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

package common

import (
	"context"

	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type DBSequencingActivity struct {
	LocalID        *uint64            `gorm:"column:id"`
	SubjectID      string             `gorm:"column:subject_id"`
	Timestamp      pldtypes.Timestamp `gorm:"column:timestamp"`
	TransactionID  uuid.UUID          `gorm:"column:transaction_id"`
	ActivityType   string             `gorm:"column:activity_type"`
	SequencingNode string             `gorm:"column:submitting_node"`
}

func (DBSequencingActivity) TableName() string {
	return "sequencer_activities"
}

func WriteSequencingActivities(ctx context.Context, dbTX persistence.DBTX, sequencingActivities []*pldapi.SequencerActivity) error {
	dbActivities := make([]*DBSequencingActivity, 0, len(sequencingActivities))
	for _, sequencingActivity := range sequencingActivities {
		dbActivities = append(dbActivities, &DBSequencingActivity{
			SubjectID:      sequencingActivity.SubjectID,
			Timestamp:      sequencingActivity.Timestamp,
			TransactionID:  sequencingActivity.TransactionID,
			ActivityType:   sequencingActivity.ActivityType,
			SequencingNode: sequencingActivity.SequencingNode,
		})
	}

	if len(dbActivities) == 0 {
		return nil
	}

	return dbTX.DB().
		WithContext(ctx).
		Table("sequencer_activities").
		Create(dbActivities).
		Error
}
