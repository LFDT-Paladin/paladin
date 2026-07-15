// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FakeCoin struct {
	Amount ethtypes.HexInteger       `json:"amount"`
	Salt   ethtypes.HexBytes0xPrefix `json:"salt"`
}

func parseFakeCoin(t *testing.T, s *pldapi.State) *FakeCoin {
	var c FakeCoin
	err := json.Unmarshal(s.Data, &c)
	require.NoError(t, err)
	return &c
}

func checkPostCommit(t *testing.T, ss *stateManager, txID uuid.UUID, expectedSpent, expectedRead, expectedConfirmed, expectedInfo []pldtypes.HexBytes) {

	txStates, err := ss.GetTransactionStates(ss.bgCtx, ss.p.NOTX(), txID)
	require.NoError(t, err)

	require.Nil(t, txStates.Unavailable)

	toMap := func(states []*pldapi.StateBase) map[string]bool {
		m := make(map[string]bool)
		for _, s := range states {
			m[s.ID.String()] = true
		}
		return m
	}

	spentIDs := toMap(txStates.Spent)
	require.Equal(t, len(expectedSpent), len(spentIDs), "unique spent ID counts match")
	for _, sID := range expectedSpent {
		require.Contains(t, spentIDs, sID.String())
	}

	readIDs := toMap(txStates.Read)
	require.Equal(t, len(expectedRead), len(readIDs), "unique read ID counts match")
	for _, sID := range expectedRead {
		require.Contains(t, readIDs, sID.String())
	}

	confirmedIDs := toMap(txStates.Confirmed)
	require.Equal(t, len(expectedConfirmed), len(confirmedIDs), "unique confirmed ID counts match")
	for _, sID := range expectedConfirmed {
		require.Contains(t, confirmedIDs, sID.String())
	}

	infoIDs := toMap(txStates.Info)
	require.Equal(t, len(expectedInfo), len(infoIDs), "unique confirmed ID counts match")
	for _, sID := range expectedInfo {
		require.Contains(t, infoIDs, sID.String())
	}

}

// TestDSWFlushNoWork verifies that flushing with no pending writes is a no-op.
func TestDSWFlushNoWork(t *testing.T) {

	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", false)

	mdb.ExpectBegin()
	mdb.ExpectCommit()

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return sw.Flush(ctx, dbTX)
	})
	require.NoError(t, err)

	// There was nothing to flush, so flushing was a no-op
	require.Nil(t, sw.flushing)

}

// TestDSWUpsertSchemaAndStates verifies StageStateUpserts on the DomainStateWriter.
func TestDSWUpsertSchemaAndStates(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	// Enable trace logging to exercise the trace log lines
	log.EnsureInit()
	originalLevel := log.GetLevel()
	log.SetLevel("trace")
	defer log.SetLevel(originalLevel)

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	require.Len(t, schemas, 1)
	schemaID := schemas[0].ID()
	fakeHash1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	fakeHash2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", true)

	upsert1 := &components.StateUpsert{
		ID:     fakeHash1,
		Schema: schemaID,
		Data:   pldtypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, pldtypes.RandHex(32))),
	}
	states, err := sw.StageStateUpserts(ctx, ss.p.NOTX(),
		upsert1,
		&components.StateUpsert{
			ID:     fakeHash2,
			Schema: schemaID,
			Data:   pldtypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, pldtypes.RandHex(32))),
		},
	)
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.NotEmpty(t, states[0].ID)
	assert.Equal(t, fakeHash2, states[1].ID)

	// Check the DB is happy with us double-writing states so we don't de-dup anything in the unFlushed list
	_, err = sw.StageStateUpserts(ctx, ss.p.NOTX(), upsert1)
	require.NoError(t, err)
	require.Len(t, sw.unFlushed.states, 3)

	syncFlushWriter(t, ctx, sw)
}

// TestDSWStateContextMintSpendMint is the primary integration test for the DomainStateWriter:
// write states, flush, write more states, verify DB queries at each stage.
func TestDSWStateContextMintSpendMint(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	// Enable trace logging to exercise the trace log lines
	log.EnsureInit()
	originalLevel := log.GetLevel()
	log.SetLevel("trace")
	defer log.SetLevel(originalLevel)

	transactionID1 := uuid.New()
	transactionID2 := uuid.New()
	transactionID3 := uuid.New()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{
		testABIParam(t, fakeCoinABI), // Pop in our widget ABI
		{Type: "tuple", InternalType: "struct TXInfo", Components: abi.ParameterArray{ // and an info state schema
			{Name: "info", Type: "string"},
			{Name: "salt", Type: "bytes32"},
		}},
	})
	require.NoError(t, err)
	assert.Len(t, schemas, 2)
	schemaID := schemas[0].ID()
	infoSchema := schemas[1].ID()

	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", false)

	// Batch 1: tx1 creates 3 coins + 1 info; tx3 creates 2 coins
	tx1states, err := sw.StageStateUpserts(ctx, ss.p.NOTX(),
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID1},
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 10,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID1},
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 75,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID1},
		&components.StateUpsert{Schema: infoSchema, Data: pldtypes.RawJSON(fmt.Sprintf(`{"info": "some info", "salt": "%s"}`, pldtypes.RandHex(32)))},
	)
	require.NoError(t, err)
	assert.Len(t, tx1states, 4)

	// tx3: spends tx1states[1]=10 and tx1states[2]=75, creates 35 and 50
	tx3states, err := sw.StageStateUpserts(ctx, ss.p.NOTX(),
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 35, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID3},
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 50, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID3},
	)
	require.NoError(t, err)
	assert.Len(t, tx3states, 2)
	// The DSW write buffer now holds all 6 coin/info states
	assert.Equal(t, 6, len(sw.unFlushed.states))

	// Flush to DB
	syncFlushWriter(t, ctx, sw)

	// Finalize: tx3 spends tx1states[1..2]; tx2 reads tx1states[1]; tx1 confirms its coins and info; tx3 confirms its coins
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(), []*pldapi.StateSpendRecord{
		{DomainName: "domain1", State: tx1states[1].ID, Transaction: transactionID3},
		{DomainName: "domain1", State: tx1states[2].ID, Transaction: transactionID3},
	}, []*pldapi.StateReadRecord{
		{DomainName: "domain1", State: tx1states[1].ID, Transaction: transactionID2},
	}, []*pldapi.StateConfirmRecord{
		{DomainName: "domain1", State: tx1states[0].ID, Transaction: transactionID1},
		{DomainName: "domain1", State: tx1states[2].ID, Transaction: transactionID1},
		{DomainName: "domain1", State: tx3states[0].ID, Transaction: transactionID3},
		{DomainName: "domain1", State: tx3states[1].ID, Transaction: transactionID3},
	}, []*pldapi.StateInfoRecord{
		{DomainName: "domain1", State: tx1states[3].ID, Transaction: transactionID1},
	})
	require.NoError(t, err)

	// DB query: available = confirmed, not spent → coin100 + coin35 + coin50
	dbStates, err := ss.FindContractStates(ctx, ss.p.NOTX(), "domain1", &sw.contractAddress, schemaID, query.NewQueryBuilder().Sort("owner", "amount").Query(), pldapi.StateStatusAvailable)
	require.NoError(t, err)
	assert.Len(t, dbStates, 3)
	assert.Equal(t, int64(50), parseFakeCoin(t, dbStates[0]).Amount.Int64())
	assert.Equal(t, int64(35), parseFakeCoin(t, dbStates[1]).Amount.Int64())
	assert.Equal(t, int64(100), parseFakeCoin(t, dbStates[2]).Amount.Int64())

	// Batch 2: tx4 creates coin20 and coin30 (it will spend coin50 and read coin100 at finalization)
	transactionID4 := uuid.New()
	tx4states, err := sw.StageStateUpserts(ctx, ss.p.NOTX(),
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID4},
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 30, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID4},
	)
	require.NoError(t, err)
	assert.Len(t, tx4states, 2)

	// GetStatesByID: tx3states[1]=50 is in DB; tx4states[0]=20 is still in DSW buffer → only tx3states[1] returned
	statesByID, err := sw.ss.GetStatesByID(ctx, ss.p.NOTX(), sw.domainName, &sw.contractAddress,
		[]pldtypes.HexBytes{tx3states[1].ID, tx4states[0].ID}, false, false)
	require.NoError(t, err)
	assert.Len(t, statesByID, 1)
	assert.Equal(t, int64(50), parseFakeCoin(t, statesByID[0]).Amount.Int64())

	// Second flush
	syncFlushWriter(t, ctx, sw)

	// Finalize: tx4 spends coin50, reads coin100, confirms coin20+coin30; tx5 spends coin20 externally
	transactionID5 := uuid.New()
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(), []*pldapi.StateSpendRecord{
		{DomainName: "domain1", State: tx3states[1].ID, Transaction: transactionID4}, // spend coin50
		{DomainName: "domain1", State: tx4states[0].ID, Transaction: transactionID5}, // external spend of coin20
	}, []*pldapi.StateReadRecord{
		{DomainName: "domain1", State: tx1states[0].ID, Transaction: transactionID4}, // read coin100
	}, []*pldapi.StateConfirmRecord{
		{DomainName: "domain1", State: tx4states[0].ID, Transaction: transactionID4},
		{DomainName: "domain1", State: tx4states[1].ID, Transaction: transactionID4},
	}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	// Available: coin100 + coin35 + coin30 (coin20 spent by tx5, coin50 spent by tx4)
	dbAvail, err := ss.FindContractStates(ctx, ss.p.NOTX(), "domain1", &sw.contractAddress, schemaID,
		query.NewQueryBuilder().Sort("owner", "amount").Query(), pldapi.StateStatusAvailable)
	require.NoError(t, err)
	assert.Len(t, dbAvail, 3)
	assert.Equal(t, int64(30), parseFakeCoin(t, dbAvail[0]).Amount.Int64())
	assert.Equal(t, int64(35), parseFakeCoin(t, dbAvail[1]).Amount.Int64())
	assert.Equal(t, int64(100), parseFakeCoin(t, dbAvail[2]).Amount.Int64())

	// Post-commit checks
	checkPostCommit(t, ss, transactionID1,
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{tx1states[0].ID, tx1states[2].ID},
		[]pldtypes.HexBytes{tx1states[3].ID},
	)
	checkPostCommit(t, ss, transactionID2,
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{tx1states[1].ID},
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{},
	)
	checkPostCommit(t, ss, transactionID3,
		[]pldtypes.HexBytes{tx1states[1].ID, tx1states[2].ID},
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{tx3states[0].ID, tx3states[1].ID},
		[]pldtypes.HexBytes{},
	)
	checkPostCommit(t, ss, transactionID4,
		[]pldtypes.HexBytes{tx3states[1].ID},
		[]pldtypes.HexBytes{tx1states[0].ID},
		[]pldtypes.HexBytes{tx4states[0].ID, tx4states[1].ID},
		[]pldtypes.HexBytes{},
	)

}

// TestDSWStateContextMintSpendWithNullifier tests the full mint/spend lifecycle with nullifiers.
func TestDSWStateContextMintSpendWithNullifier(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	transactionID1 := uuid.New()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	assert.Len(t, schemas, 1)
	schemaID := schemas[0].ID()
	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	nullifier1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	nullifier2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	data1 := pldtypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32)))
	data2 := pldtypes.RawJSON(fmt.Sprintf(`{"amount": 10,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32)))

	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", true)

	// Start with 2 states
	tx1states, err := sw.StageStateUpserts(ctx, ss.p.NOTX(),
		&components.StateUpsert{ID: stateID1, Schema: schemaID, Data: data1, CreatedBy: &transactionID1},
		&components.StateUpsert{ID: stateID2, Schema: schemaID, Data: data2, CreatedBy: &transactionID1},
	)
	require.NoError(t, err)
	assert.Len(t, tx1states, 2)

	// Attach a nullifier to the first state
	err = sw.StageNullifierUpserts(ctx,
		&components.NullifierUpsert{State: stateID1, ID: nullifier1},
	)
	require.NoError(t, err)

	// Cannot attach another nullifier without a reset
	err = sw.StageNullifierUpserts(ctx,
		&components.NullifierUpsert{State: stateID1, ID: nullifier2},
	)
	assert.Regexp(t, "PD010127", err)

	// Flush the states to the database
	syncFlushWriter(t, ctx, sw)

	// Confirm both states
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(), []*pldapi.StateSpendRecord{}, []*pldapi.StateReadRecord{},
		[]*pldapi.StateConfirmRecord{
			{DomainName: "domain1", State: stateID1, Transaction: transactionID1},
			{DomainName: "domain1", State: stateID2, Transaction: transactionID1},
		}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	contractAddress := sw.contractAddress

	// Confirm still 2 states and 1 nullifier in DB
	dbStates, err := ss.FindContractStates(ctx, ss.p.NOTX(), "domain1", &contractAddress, schemaID,
		query.NewQueryBuilder().Query(), pldapi.StateStatusAvailable)
	require.NoError(t, err)
	assert.Len(t, dbStates, 2)
	nullStates, err := ss.FindContractNullifiers(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID,
		query.NewQueryBuilder().Query(), pldapi.StateStatusAvailable)
	require.NoError(t, err)
	assert.Len(t, nullStates, 1)
	require.NotNil(t, nullStates[0].Nullifier)
	assert.Equal(t, nullifier1, nullStates[0].Nullifier.ID)

	// Flush a second time (empty flush)
	syncFlushWriter(t, ctx, sw)

	// Spend the nullifier state via finalization (simulates tx spending it)
	transactionID3 := uuid.New()
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{
			{DomainName: "domain1", State: nullifier1, Transaction: transactionID3},
		}, []*pldapi.StateReadRecord{}, []*pldapi.StateConfirmRecord{}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	// Reset the writer so we start fresh from the DB perspective
	sw.Reset()

	// Confirm no more nullifiers available in DB
	nullStates2, err := ss.FindContractNullifiers(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID,
		query.NewQueryBuilder().Query(), pldapi.StateStatusAvailable)
	require.NoError(t, err)
	assert.Len(t, nullStates2, 0)

	// StageNullifierUpserts fails when state is not in the write buffer
	err = sw.StageNullifierUpserts(ctx, &components.NullifierUpsert{State: stateID2, ID: nullifier2})
	assert.Regexp(t, "PD010126", err)
	// Re-stage the state then attach nullifier
	_, err = sw.StageStateUpserts(ctx, ss.p.NOTX(), &components.StateUpsert{ID: stateID2, Schema: schemaID, Data: data2, CreatedBy: &transactionID1})
	require.NoError(t, err)
	err = sw.StageNullifierUpserts(ctx, &components.NullifierUpsert{State: stateID2, ID: nullifier2})
	require.NoError(t, err)

}

// TestDSWStageNullifierUpsertsFromFlushingBuffer verifies that StageNullifierUpserts can attach
// a nullifier to a state that has already been moved into the flushing buffer by Flush.
func TestDSWStageNullifierUpsertsFromFlushingBuffer(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec("INSERT.*schemas").WillReturnResult(driver.ResultNoRows)

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schemas[0].ID()), schemas[0])

	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", false)

	tx1 := uuid.New()
	data1 := fmt.Sprintf(`{"amount": 50, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))
	states, err := sw.StageStateUpserts(ctx, ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	require.NoError(t, err)
	require.Len(t, states, 1)
	stateID := states[0].ID

	// Begin a flush — this moves unFlushed → flushing without committing yet
	db.ExpectBegin()
	db.ExpectExec("INSERT.*states").WillReturnResult(driver.ResultNoRows)
	db.ExpectExec("INSERT.*state_labels").WillReturnResult(driver.ResultNoRows)
	db.ExpectExec("DELETE.*pending_private_state_data").WillReturnResult(driver.ResultNoRows)
	db.ExpectCommit()
	var flushErr error
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		flushErr = sw.Flush(ctx, dbTX)
		// State is now in sw.flushing; attach a nullifier while it is still there
		nullID := pldtypes.HexBytes(pldtypes.RandBytes(32))
		nullErr := sw.StageNullifierUpserts(ctx, &components.NullifierUpsert{State: stateID, ID: nullID})
		require.NoError(t, nullErr)
		return nil
	})
	require.NoError(t, flushErr)
	require.NoError(t, err)
}

func TestDSWStageStateUpsertsFailSchemaLookup(t *testing.T) {

	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT.*schema").WillReturnError(fmt.Errorf("pop"))

	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", false)

	_, err := sw.StageStateUpserts(ctx, ss.p.NOTX(), &components.StateUpsert{
		ID:     pldtypes.RandBytes(32),
		Schema: pldtypes.Bytes32(pldtypes.RandBytes(32)),
	})
	assert.Regexp(t, "pop", err)

}

// TestDSWUpsertBadData verifies that StageStateUpserts rejects malformed state data.
func TestDSWUpsertBadData(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema.Schema})
	require.NoError(t, err)

	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", false)

	_, err = sw.StageStateUpserts(ctx, ss.p.NOTX(), &components.StateUpsert{
		Schema: schema.ID(), Data: pldtypes.RawJSON(`"wrong"`),
	})
	assert.Regexp(t, "FF22038", err)

}

// TestDSWFlushErrorCapture verifies that a flush error is captured and all subsequent
// StageStateUpserts/StageNullifierUpserts calls return that error until Reset() is called,
// and that a double-Flush within the same transaction is rejected.
func TestDSWFlushErrorCapture(t *testing.T) {

	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec("INSERT.*schemas").WillReturnResult(driver.ResultNoRows)
	db.ExpectBegin()
	db.ExpectExec("INSERT").WillReturnError(fmt.Errorf("pop"))

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)

	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schemas[0].ID()), schemas[0])

	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", false)

	data1 := fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))
	tx1 := uuid.New()
	_, err = sw.StageStateUpserts(ctx, ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	require.NoError(t, err)

	// Flush returns an error from the DB
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return sw.Flush(ctx, dbTX)
	})
	require.Regexp(t, "pop", err)

	// StageStateUpserts and StageNullifierUpserts return the captured error until Reset
	_, err = sw.StageStateUpserts(ctx, ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	assert.Regexp(t, "PD010119.*pop", err)

	err = sw.StageNullifierUpserts(ctx)
	assert.Regexp(t, "PD010119.*pop", err)

	// Flush also returns the captured error
	db.ExpectBegin()
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return sw.Flush(ctx, dbTX)
	})
	assert.Regexp(t, "pop", err)

	sw.Reset()

	err = sw.checkResetInitUnFlushed(ctx)
	require.NoError(t, err)

	// After Reset, upserts and a successful flush work again
	_, err = sw.StageStateUpserts(ctx, ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	require.NoError(t, err)
	_, err = sw.StageStateUpserts(ctx, ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	require.NoError(t, err)

	db.ExpectBegin()
	db.ExpectExec("INSERT.*states").WillReturnResult(driver.ResultNoRows)
	db.ExpectExec("INSERT.*state_labels").WillReturnResult(driver.ResultNoRows)
	db.ExpectExec("DELETE.*pending_private_state_data").WillReturnResult(driver.ResultNoRows)
	db.ExpectCommit()
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		err := sw.Flush(ctx, dbTX)
		require.NoError(t, err)
		err = sw.Flush(ctx, dbTX)
		assert.Regexp(t, "PD010131", err) // cannot flush again until callback
		return nil
	})
	require.NoError(t, err)

	// Simulate an async commit error via the finalizer
	sw.flushing = newPendingStateWrites(sw.ss)
	sw.finalizer(ctx, fmt.Errorf("crackle"))

	_, err = sw.StageStateUpserts(ctx, ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	assert.Regexp(t, "PD010119.*crackle", err)

}
