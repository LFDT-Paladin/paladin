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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDomain = "domain1"

// lockControlledPersistence wraps a real Persistence and allows tests to control the
// outcome of TakeNamedLock without touching the underlying DB connection.
type lockControlledPersistence struct {
	persistence.Persistence
	lockErr   error
	lockCalls int
}

func (p *lockControlledPersistence) TakeNamedLock(_ context.Context, _ persistence.DBTX, _ string) error {
	p.lockCalls++
	return p.lockErr
}

// queryAllPendingRows fetches every row from pending_private_state_data.
func queryAllPendingRows(t *testing.T, ss *stateManager) []pendingPrivateStateData {
	t.Helper()
	var rows []pendingPrivateStateData
	err := ss.p.DB().Find(&rows).Error
	require.NoError(t, err)
	return rows
}

// seedPendingRow inserts a row directly, bypassing business logic.
func seedPendingRow(t *testing.T, ss *stateManager, contract string, stateID pldtypes.HexBytes, blockNumber int64) {
	t.Helper()
	err := ss.p.DB().Create(&pendingPrivateStateData{
		StateID:     stateID.String(),
		Contract:    contract,
		BlockNumber: blockNumber,
	}).Error
	require.NoError(t, err)
}

// makePendingEntry builds a PendingPrivateStateDataEntry using the given contract address.
func makePendingEntry(stateID pldtypes.HexBytes, contract pldtypes.EthAddress, blockNumber int64) components.PendingPrivateStateDataEntry {
	return components.PendingPrivateStateDataEntry{
		StateID:     stateID,
		Contract:    contract,
		BlockNumber: blockNumber,
	}
}

// ─── WritePendingPrivateStateDataBatch ────────────────────────────────────────────

func TestWritePendingPrivateStateDataBatch_AllStatesPresent(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	id1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	id2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	// Pre-insert both states so getStateIDsMissingPrivateData returns empty.
	insertTestState(t, ss, testDomain, id1)
	insertTestState(t, ss, testDomain, id2)

	entries := []components.PendingPrivateStateDataEntry{
		makePendingEntry(id1, contractAddr, 100),
		makePendingEntry(id2, contractAddr, 100),
	}

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.WritePendingPrivateStateDataBatch(ctx, dbTX, testDomain, entries)
	})
	require.NoError(t, err)

	assert.Empty(t, queryAllPendingRows(t, ss))
}

func TestWritePendingPrivateStateDataBatch_OneMissingState(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	presentID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	missingID := pldtypes.HexBytes(pldtypes.RandBytes(32))

	insertTestState(t, ss, testDomain, presentID) // present; missingID is not inserted

	entries := []components.PendingPrivateStateDataEntry{
		makePendingEntry(presentID, contractAddr, 200),
		makePendingEntry(missingID, contractAddr, 200),
	}

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.WritePendingPrivateStateDataBatch(ctx, dbTX, testDomain, entries)
	})
	require.NoError(t, err)

	rows := queryAllPendingRows(t, ss)
	require.Len(t, rows, 1)
	assert.Equal(t, contractAddr.String(), rows[0].Contract)
	assert.Equal(t, missingID.String(), rows[0].StateID)
	assert.Equal(t, int64(200), rows[0].BlockNumber)
}

func TestWritePendingPrivateStateDataBatch_MultipleMissingStates(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	id1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	id2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	id3 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	entries := []components.PendingPrivateStateDataEntry{
		makePendingEntry(id1, contractAddr, 300),
		makePendingEntry(id2, contractAddr, 300),
		makePendingEntry(id3, contractAddr, 300),
	}

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.WritePendingPrivateStateDataBatch(ctx, dbTX, testDomain, entries)
	})
	require.NoError(t, err)

	rows := queryAllPendingRows(t, ss)
	require.Len(t, rows, 3)
	writtenIDs := make(map[string]bool)
	for _, r := range rows {
		writtenIDs[r.StateID] = true
		assert.Equal(t, contractAddr.String(), r.Contract)
		assert.Equal(t, int64(300), r.BlockNumber)
	}
	assert.True(t, writtenIDs[id1.String()])
	assert.True(t, writtenIDs[id2.String()])
	assert.True(t, writtenIDs[id3.String()])
}

func TestWritePendingPrivateStateDataBatch_Idempotent(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	missingID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	entries := []components.PendingPrivateStateDataEntry{makePendingEntry(missingID, contractAddr, 10)}

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		if err := ss.WritePendingPrivateStateDataBatch(ctx, dbTX, testDomain, entries); err != nil {
			return err
		}
		return ss.WritePendingPrivateStateDataBatch(ctx, dbTX, testDomain, entries)
	})
	require.NoError(t, err)

	rows := queryAllPendingRows(t, ss)
	require.Len(t, rows, 1)
	assert.Equal(t, missingID.String(), rows[0].StateID)
}

func TestWritePendingPrivateStateDataBatch_EmptyEntries(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.WritePendingPrivateStateDataBatch(ctx, dbTX, testDomain, nil)
	})
	require.NoError(t, err)
	assert.Empty(t, queryAllPendingRows(t, ss))
}

func TestWritePendingPrivateStateDataBatch_DBError(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT.*id.*states").WillReturnError(fmt.Errorf("db lookup error"))

	contractAddr := *pldtypes.RandAddress()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	entries := []components.PendingPrivateStateDataEntry{makePendingEntry(stateID, contractAddr, 10)}

	err := ss.WritePendingPrivateStateDataBatch(ctx, ss.p.NOTX(), testDomain, entries)
	require.ErrorContains(t, err, "db lookup error")
}

func TestWritePendingPrivateStateDataBatch_MultipleContracts(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contract1 := *pldtypes.RandAddress()
	contract2 := *pldtypes.RandAddress()
	id1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	id2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	entries := []components.PendingPrivateStateDataEntry{
		{StateID: id1, Contract: contract1, BlockNumber: 50},
		{StateID: id2, Contract: contract2, BlockNumber: 51},
	}

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.WritePendingPrivateStateDataBatch(ctx, dbTX, testDomain, entries)
	})
	require.NoError(t, err)

	rows := queryAllPendingRows(t, ss)
	require.Len(t, rows, 2)
	rowByID := make(map[string]pendingPrivateStateData)
	for _, r := range rows {
		rowByID[r.StateID] = r
	}
	assert.Equal(t, contract1.String(), rowByID[id1.String()].Contract)
	assert.Equal(t, int64(50), rowByID[id1.String()].BlockNumber)
	assert.Equal(t, contract2.String(), rowByID[id2.String()].Contract)
	assert.Equal(t, int64(51), rowByID[id2.String()].BlockNumber)
}

func TestWritePendingPrivateStateDataBatch_LockAcquisitionError(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	wrapper := &lockControlledPersistence{Persistence: ss.p, lockErr: fmt.Errorf("lock error")}
	ss.p = wrapper

	contractAddr := *pldtypes.RandAddress()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	entries := []components.PendingPrivateStateDataEntry{makePendingEntry(stateID, contractAddr, 10)}

	err := ss.WritePendingPrivateStateDataBatch(ctx, ss.p.NOTX(), testDomain, entries)
	require.ErrorContains(t, err, "lock error")
	assert.Equal(t, 1, wrapper.lockCalls)
	assert.Empty(t, queryAllPendingRows(t, ss))
}

func TestWritePendingPrivateStateDataBatch_EmptyBatch_SkipsLock(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	wrapper := &lockControlledPersistence{Persistence: ss.p}
	ss.p = wrapper

	err := ss.WritePendingPrivateStateDataBatch(ctx, ss.p.NOTX(), testDomain, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, wrapper.lockCalls)
}

// ─── updatePendingPrivateStateData ─────────────────────────────────────────────────────

func TestUpdatePendingPrivateStateData_EmptyList(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	seedPendingRow(t, ss, contractAddr.String(), stateID, 10)

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.updatePendingPrivateStateData(ctx, dbTX, nil)
	})
	require.NoError(t, err)

	rows := queryAllPendingRows(t, ss)
	require.Len(t, rows, 1)
}

func TestUpdatePendingPrivateStateData_NoMatchingRows(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	trackedID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	seedPendingRow(t, ss, contractAddr.String(), trackedID, 10)

	arrivedID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.updatePendingPrivateStateData(ctx, dbTX, []pldtypes.HexBytes{arrivedID})
	})
	require.NoError(t, err)

	rows := queryAllPendingRows(t, ss)
	require.Len(t, rows, 1)
	assert.Equal(t, trackedID.String(), rows[0].StateID)
}

func TestUpdatePendingPrivateStateData_SingleStateArrives(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	seedPendingRow(t, ss, contractAddr.String(), stateID, 10)

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.updatePendingPrivateStateData(ctx, dbTX, []pldtypes.HexBytes{stateID})
	})
	require.NoError(t, err)

	assert.Empty(t, queryAllPendingRows(t, ss))
}

func TestUpdatePendingPrivateStateData_MultipleStatesArrive(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	id1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	id2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	untrackedID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	seedPendingRow(t, ss, contractAddr.String(), id1, 10)
	seedPendingRow(t, ss, contractAddr.String(), id2, 11)

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.updatePendingPrivateStateData(ctx, dbTX, []pldtypes.HexBytes{id1, id2, untrackedID})
	})
	require.NoError(t, err)

	assert.Empty(t, queryAllPendingRows(t, ss))
}

func TestUpdatePendingPrivateStateData_PartialArrivals(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	id1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	id2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	seedPendingRow(t, ss, contractAddr.String(), id1, 10)
	seedPendingRow(t, ss, contractAddr.String(), id2, 10)

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return ss.updatePendingPrivateStateData(ctx, dbTX, []pldtypes.HexBytes{id1})
	})
	require.NoError(t, err)

	rows := queryAllPendingRows(t, ss)
	require.Len(t, rows, 1)
	assert.Equal(t, id2.String(), rows[0].StateID)
}

func TestUpdatePendingPrivateStateData_LockAcquisitionError(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	seedPendingRow(t, ss, contractAddr.String(), stateID, 10)

	wrapper := &lockControlledPersistence{Persistence: ss.p, lockErr: fmt.Errorf("lock error")}
	ss.p = wrapper

	err := ss.updatePendingPrivateStateData(ctx, ss.p.NOTX(), []pldtypes.HexBytes{stateID})
	require.ErrorContains(t, err, "lock error")
	assert.Equal(t, 1, wrapper.lockCalls)
	// Row must still be present — the delete did not run.
	assert.Len(t, queryAllPendingRows(t, ss), 1)
}

func TestUpdatePendingPrivateStateData_EmptyList_SkipsLock(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	wrapper := &lockControlledPersistence{Persistence: ss.p}
	ss.p = wrapper

	err := ss.updatePendingPrivateStateData(ctx, ss.p.NOTX(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, wrapper.lockCalls)
}

// ─── CheckPendingPrivateStateDataForContract ──────────────────────────────────────────

func TestCheckPendingPrivateStateDataForContract_NoRows_Complete(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	complete, err := ss.CheckPendingPrivateStateDataForContract(ctx, ss.p.NOTX(), contractAddr.String(), 1000)
	require.NoError(t, err)
	assert.True(t, complete)
}

func TestCheckPendingPrivateStateDataForContract_RowAtOrBelowBlock_Incomplete(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	seedPendingRow(t, ss, contractAddr.String(), pldtypes.RandBytes(32), 50)

	complete, err := ss.CheckPendingPrivateStateDataForContract(ctx, ss.p.NOTX(), contractAddr.String(), 50)
	require.NoError(t, err)
	assert.False(t, complete)
}

func TestCheckPendingPrivateStateDataForContract_RowAboveMaxBlock_Complete(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	seedPendingRow(t, ss, contractAddr.String(), pldtypes.RandBytes(32), 100)

	complete, err := ss.CheckPendingPrivateStateDataForContract(ctx, ss.p.NOTX(), contractAddr.String(), 50)
	require.NoError(t, err)
	assert.True(t, complete)
}

func TestCheckPendingPrivateStateDataForContract_DifferentContract_DoesNotInterfere(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	otherContract := pldtypes.RandAddress().String()
	seedPendingRow(t, ss, otherContract, pldtypes.RandBytes(32), 10)

	complete, err := ss.CheckPendingPrivateStateDataForContract(ctx, ss.p.NOTX(), contractAddr.String(), 100)
	require.NoError(t, err)
	assert.True(t, complete)
}

func TestCheckPendingPrivateStateDataForContract_MultipleRows_Incomplete(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddr := *pldtypes.RandAddress()
	seedPendingRow(t, ss, contractAddr.String(), pldtypes.RandBytes(32), 10)
	seedPendingRow(t, ss, contractAddr.String(), pldtypes.RandBytes(32), 20)

	complete, err := ss.CheckPendingPrivateStateDataForContract(ctx, ss.p.NOTX(), contractAddr.String(), 100)
	require.NoError(t, err)
	assert.False(t, complete)
}
