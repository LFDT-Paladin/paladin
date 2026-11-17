// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package statemgr

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetStatesConfirmedUpdateFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec("UPDATE.*states").WillReturnError(fmt.Errorf("pop"))

	err := setStatesConfirmed(ctx, ss.p.NOTX(), []*pldapi.StateConfirmRecord{
		{DomainName: "domain1", State: pldtypes.HexBytes(pldtypes.RandBytes(32))},
	})
	assert.Regexp(t, "pop", err)
}

func TestSetStatesSpentUpdateFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec("UPDATE.*states").WillReturnError(fmt.Errorf("pop"))

	err := setStatesSpent(ctx, ss.p.NOTX(), []*pldapi.StateSpendRecord{
		{DomainName: "domain1", State: pldtypes.HexBytes(pldtypes.RandBytes(32))},
	})
	assert.Regexp(t, "pop", err)
}

func TestSetStateAvailableFromSpendConfirmRecordsNoStates(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	wrapper := &lockControlledPersistence{Persistence: ss.p}
	ss.p = wrapper

	err := ss.setStateAvailableFromSpendConfirmRecords(ctx, ss.p.NOTX(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, wrapper.lockCalls)
}

func TestSetStateAvailableFromSpendConfirmRecordsLockFail(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	wrapper := &lockControlledPersistence{Persistence: ss.p, lockErr: fmt.Errorf("lock error")}
	ss.p = wrapper

	err := ss.setStateAvailableFromSpendConfirmRecords(ctx, ss.p.NOTX(), []*pldapi.State{
		{StateBase: pldapi.StateBase{DomainName: "domain1", ID: pldtypes.HexBytes(pldtypes.RandBytes(32))}},
	})
	require.ErrorContains(t, err, "lock error")
	assert.Equal(t, 1, wrapper.lockCalls)
}

func TestSetStateAvailableFromSpendConfirmRecordsConfirmedUpdateFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec("UPDATE.*states").WillReturnError(fmt.Errorf("pop"))

	err := ss.setStateAvailableFromSpendConfirmRecords(ctx, ss.p.NOTX(), []*pldapi.State{
		{StateBase: pldapi.StateBase{DomainName: "domain1", ID: pldtypes.HexBytes(pldtypes.RandBytes(32))}},
	})
	assert.Regexp(t, "pop", err)
}

func TestSetStateAvailableFromSpendConfirmRecordsSpentUpdateFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	// confirmed flag, on states and both label tables
	db.ExpectExec(`UPDATE "states"`).WillReturnResult(driver.ResultNoRows)
	db.ExpectExec(`UPDATE "state_labels"`).WillReturnResult(driver.ResultNoRows)
	db.ExpectExec(`UPDATE "state_int64_labels"`).WillReturnResult(driver.ResultNoRows)
	// spent flag
	db.ExpectExec(`UPDATE "states"`).WillReturnError(fmt.Errorf("pop"))

	err := ss.setStateAvailableFromSpendConfirmRecords(ctx, ss.p.NOTX(), []*pldapi.State{
		{StateBase: pldapi.StateBase{DomainName: "domain1", ID: pldtypes.HexBytes(pldtypes.RandBytes(32))}},
	})
	assert.Regexp(t, "pop", err)
}

func TestSetStatesConfirmedLabelUpdateFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec(`UPDATE "states"`).WillReturnResult(driver.ResultNoRows)
	db.ExpectExec(`UPDATE "state_labels"`).WillReturnError(fmt.Errorf("pop"))

	err := setStatesConfirmed(ctx, ss.p.NOTX(), []*pldapi.StateConfirmRecord{
		{DomainName: "domain1", State: pldtypes.HexBytes(pldtypes.RandBytes(32))},
	})
	assert.Regexp(t, "pop", err)
}

func TestSetStateAvailableFromSpendConfirmRecordsLabelUpdateFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec(`UPDATE "states"`).WillReturnResult(driver.ResultNoRows)
	db.ExpectExec(`UPDATE "state_labels"`).WillReturnResult(driver.ResultNoRows)
	db.ExpectExec(`UPDATE "state_int64_labels"`).WillReturnError(fmt.Errorf("pop"))

	err := ss.setStateAvailableFromSpendConfirmRecords(ctx, ss.p.NOTX(), []*pldapi.State{
		{StateBase: pldapi.StateBase{DomainName: "domain1", ID: pldtypes.HexBytes(pldtypes.RandBytes(32))}},
	})
	assert.Regexp(t, "pop", err)
}

// checkLabelRowsMatchStates checks every label row has the same schema, contract address, created
// time and confirmed/spent flags as its state. A label row with no schema or created time counts as
// a mismatch, so rows that were never filled in cannot pass.
func checkLabelRowsMatchStates(t *testing.T, ctx context.Context, ss *stateManager) {
	for _, table := range []string{"state_labels", "state_int64_labels"} {
		var mismatched int64
		err := ss.p.NOTX().DB(ctx).Raw(fmt.Sprintf(`
			SELECT COUNT(*) FROM %[1]s l
			  JOIN states s ON s."domain_name" = l."domain_name" AND s."id" = l."state"
			 WHERE l."schema" IS NULL
			    OR l."created" IS NULL
			    OR l."schema" != s."schema"
			    OR l."created" != s."created"
			    OR COALESCE(l."contract_address", '') != COALESCE(s."contract_address", '')
			    OR l."confirmed" != s."confirmed"
			    OR l."spent" != s."spent"`, table)).
			Scan(&mismatched).Error
		require.NoError(t, err)
		assert.Zero(t, mismatched, "%s rows disagreeing with their states row", table)
	}
}

func TestWriteStateFinalizationsLockFail(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	wrapper := &lockControlledPersistence{Persistence: ss.p, lockErr: fmt.Errorf("lock error")}
	ss.p = wrapper

	err := ss.WriteStateFinalizations(ctx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{{DomainName: "domain1", State: pldtypes.HexBytes(pldtypes.RandBytes(32)), Transaction: uuid.New()}},
		nil, nil, nil)
	require.ErrorContains(t, err, "lock error")
	assert.Equal(t, 1, wrapper.lockCalls)
}
