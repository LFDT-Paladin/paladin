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

package sequencer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBSequencingActivity_TableName(t *testing.T) {
	activity := DBSequencingActivity{}
	tableName := activity.TableName()
	assert.Equal(t, "sequencer_activities", tableName)
}

func TestWriteReceivedSequencingActivities_EmptyList(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer mp.Mock.ExpectationsWereMet()

	sm := &sequencerManager{}

	err = sm.WriteReceivedSequencingActivities(ctx, mp.P.NOTX(), []*pldapi.SequencerActivity{})
	require.NoError(t, err)
}

func TestWriteReceivedSequencingActivities_SingleActivity(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer mp.Mock.ExpectationsWereMet()

	sm := &sequencerManager{}

	txID := uuid.New()
	subjectID := "subject-123"
	activityType := string(pldapi.SequencerActivityType_Dispatch)
	sequencingNode := "node1"
	timestamp := pldtypes.Timestamp(time.Now().UnixNano())

	sequencingActivity := &pldapi.SequencerActivity{
		SubjectID:      subjectID,
		Timestamp:      timestamp,
		TransactionID:  txID,
		ActivityType:   activityType,
		SequencingNode: sequencingNode,
	}

	// Expect INSERT query for sequencer_activities table (GORM uses Query with RETURNING)
	mp.Mock.ExpectQuery(`INSERT INTO "sequencer_activities"`).
		WithArgs(
			sqlmock.AnyArg(), // subject_id
			sqlmock.AnyArg(), // timestamp
			sqlmock.AnyArg(), // transaction_id
			sqlmock.AnyArg(), // activity_type
			sqlmock.AnyArg(), // submitting_node
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err = sm.WriteReceivedSequencingActivities(ctx, mp.P.NOTX(), []*pldapi.SequencerActivity{sequencingActivity})
	require.NoError(t, err)
}

func TestWriteReceivedSequencingActivities_MultipleActivities(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer mp.Mock.ExpectationsWereMet()

	sm := &sequencerManager{}

	txID1 := uuid.New()
	txID2 := uuid.New()
	txID3 := uuid.New()

	activities := []*pldapi.SequencerActivity{
		{
			SubjectID:      "subject-1",
			Timestamp:      pldtypes.Timestamp(time.Now().UnixNano()),
			TransactionID:  txID1,
			ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
			SequencingNode: "node1",
		},
		{
			SubjectID:      "subject-2",
			Timestamp:      pldtypes.Timestamp(time.Now().UnixNano()),
			TransactionID:  txID2,
			ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
			SequencingNode: "node2",
		},
		{
			SubjectID:      "subject-3",
			Timestamp:      pldtypes.Timestamp(time.Now().UnixNano()),
			TransactionID:  txID3,
			ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
			SequencingNode: "node3",
		},
	}

	// Expect INSERT query for multiple activities (GORM uses Query with RETURNING)
	mp.Mock.ExpectQuery(`INSERT INTO "sequencer_activities"`).
		WithArgs(
			sqlmock.AnyArg(), // subject_id for activity 1
			sqlmock.AnyArg(), // timestamp for activity 1
			sqlmock.AnyArg(), // transaction_id for activity 1
			sqlmock.AnyArg(), // activity_type for activity 1
			sqlmock.AnyArg(), // submitting_node for activity 1
			sqlmock.AnyArg(), // subject_id for activity 2
			sqlmock.AnyArg(), // timestamp for activity 2
			sqlmock.AnyArg(), // transaction_id for activity 2
			sqlmock.AnyArg(), // activity_type for activity 2
			sqlmock.AnyArg(), // submitting_node for activity 2
			sqlmock.AnyArg(), // subject_id for activity 3
			sqlmock.AnyArg(), // timestamp for activity 3
			sqlmock.AnyArg(), // transaction_id for activity 3
			sqlmock.AnyArg(), // activity_type for activity 3
			sqlmock.AnyArg(), // submitting_node for activity 3
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2).AddRow(3))

	err = sm.WriteReceivedSequencingActivities(ctx, mp.P.NOTX(), activities)
	require.NoError(t, err)
}

func TestWriteReceivedSequencingActivities_DatabaseError(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer mp.Mock.ExpectationsWereMet()

	sm := &sequencerManager{}

	txID := uuid.New()
	sequencingActivity := &pldapi.SequencerActivity{
		SubjectID:      "subject-123",
		Timestamp:      pldtypes.Timestamp(time.Now().UnixNano()),
		TransactionID:  txID,
		ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
		SequencingNode: "node1",
	}

	dbError := errors.New("database connection error")
	// Expect INSERT query to fail (GORM uses Query with RETURNING)
	mp.Mock.ExpectQuery(`INSERT INTO "sequencer_activities"`).
		WithArgs(
			sqlmock.AnyArg(), // subject_id
			sqlmock.AnyArg(), // timestamp
			sqlmock.AnyArg(), // transaction_id
			sqlmock.AnyArg(), // activity_type
			sqlmock.AnyArg(), // submitting_node
		).
		WillReturnError(dbError)

	err = sm.WriteReceivedSequencingActivities(ctx, mp.P.NOTX(), []*pldapi.SequencerActivity{sequencingActivity})
	assert.Error(t, err)
	assert.Equal(t, dbError, err)
}

func TestWriteReceivedSequencingActivities_ActivityWithAllFields(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer mp.Mock.ExpectationsWereMet()

	sm := &sequencerManager{}

	txID := uuid.New()
	localID := uint64(42)
	subjectID := "subject-activity-456"
	activityType := "custom_activity"
	sequencingNode := "test-node"
	timestamp := pldtypes.Timestamp(time.Now().UnixNano())

	sequencingActivity := &pldapi.SequencerActivity{
		LocalID:        &localID,
		SubjectID:      subjectID,
		Timestamp:      timestamp,
		TransactionID:  txID,
		ActivityType:   activityType,
		SequencingNode: sequencingNode,
	}

	// Expect INSERT query (GORM uses Query with RETURNING)
	mp.Mock.ExpectQuery(`INSERT INTO "sequencer_activities"`).
		WithArgs(
			sqlmock.AnyArg(), // subject_id
			sqlmock.AnyArg(), // timestamp
			sqlmock.AnyArg(), // transaction_id
			sqlmock.AnyArg(), // activity_type
			sqlmock.AnyArg(), // submitting_node
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err = sm.WriteReceivedSequencingActivities(ctx, mp.P.NOTX(), []*pldapi.SequencerActivity{sequencingActivity})
	require.NoError(t, err)
}
