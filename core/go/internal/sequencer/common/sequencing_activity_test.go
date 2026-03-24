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

func TestWriteSequencingActivities_EmptyList(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer mp.Mock.ExpectationsWereMet()

	err = WriteSequencingActivities(ctx, mp.P.NOTX(), []*pldapi.SequencerActivity{})
	require.NoError(t, err)
}

func TestWriteSequencingActivities_SingleActivity(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer mp.Mock.ExpectationsWereMet()

	txID := uuid.New()
	sequencingActivity := &pldapi.SequencerActivity{
		SubjectID:      "subject-123",
		Timestamp:      pldtypes.Timestamp(time.Now().UnixNano()),
		TransactionID:  txID,
		ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
		SequencingNode: "node1",
	}

	mp.Mock.ExpectQuery(`INSERT INTO "sequencer_activities"`).
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err = WriteSequencingActivities(ctx, mp.P.NOTX(), []*pldapi.SequencerActivity{sequencingActivity})
	require.NoError(t, err)
}

func TestWriteSequencingActivities_MultipleActivities(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer mp.Mock.ExpectationsWereMet()

	activities := []*pldapi.SequencerActivity{
		{
			SubjectID:      "subject-1",
			Timestamp:      pldtypes.Timestamp(time.Now().UnixNano()),
			TransactionID:  uuid.New(),
			ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
			SequencingNode: "node1",
		},
		{
			SubjectID:      "subject-2",
			Timestamp:      pldtypes.Timestamp(time.Now().UnixNano()),
			TransactionID:  uuid.New(),
			ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
			SequencingNode: "node2",
		},
		{
			SubjectID:      "subject-3",
			Timestamp:      pldtypes.Timestamp(time.Now().UnixNano()),
			TransactionID:  uuid.New(),
			ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
			SequencingNode: "node3",
		},
	}

	mp.Mock.ExpectQuery(`INSERT INTO "sequencer_activities"`).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2).AddRow(3))

	err = WriteSequencingActivities(ctx, mp.P.NOTX(), activities)
	require.NoError(t, err)
}

func TestWriteSequencingActivities_DatabaseError(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer mp.Mock.ExpectationsWereMet()

	dbError := errors.New("database connection error")
	sequencingActivity := &pldapi.SequencerActivity{
		SubjectID:      "subject-123",
		Timestamp:      pldtypes.Timestamp(time.Now().UnixNano()),
		TransactionID:  uuid.New(),
		ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
		SequencingNode: "node1",
	}

	mp.Mock.ExpectQuery(`INSERT INTO "sequencer_activities"`).
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnError(dbError)

	err = WriteSequencingActivities(ctx, mp.P.NOTX(), []*pldapi.SequencerActivity{sequencingActivity})
	assert.Error(t, err)
	assert.Equal(t, dbError, err)
}
