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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// snapshotStates builds the proto SnapshotState wire form ImportSnapshot consumes directly, without
// touching the DB — the import path reads only the snapshot protos.
func snapshotStates(states ...*components.StateWithLabels) []*prototk.SnapshotState {
	out := make([]*prototk.SnapshotState, len(states))
	for i, s := range states {
		out[i] = &prototk.SnapshotState{State: &prototk.EndorsableState{
			Id:            s.ID.String(),
			SchemaId:      s.Schema.String(),
			StateDataJson: string(s.Data),
		}}
	}
	return out
}

const fakeCoinABI = `{
	"type": "tuple",
	"internalType": "struct FakeCoin",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "owner",
			"type": "address",
			"indexed": true
		},
		{
			"name": "amount",
			"type": "uint256",
			"indexed": true
		}
	]
}`

const fakeCoinABI2 = `{
	"type": "tuple",
	"internalType": "struct FakeCoin2",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "owner",
			"type": "address",
			"indexed": true
		},
		{
			"name": "tokenUri",
			"type": "bytes32",
			"indexed": true
		}
	]
}`

func TestUpsertSchemaEmptyList(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{})
	require.NoError(t, err)
	require.Len(t, schemas, 0)

}

// TestDCClosedErrorPaths verifies that a closed DomainQueryContext returns the correct errors.
func TestDCClosedErrorPaths(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	dqc.Close(ctx)

	err := dqc.ImportSnapshot(ctx, &prototk.StateSnapshot{})
	assert.Regexp(t, "PD010122", err) // closed

	_, dc2 := newTestDomainContext(t, ctx, ss, "domain1", false)
	dc2.Close(ctx)
	_, _, err = dc2.FindAvailableStates(ctx, ss.p.NOTX(), pldtypes.Bytes32(pldtypes.RandBytes(32)), nil)
	assert.Regexp(t, "PD010122", err) // closed

}

func TestDomainQueryContextContractAddress(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	assert.Equal(t, *contractAddress, dqc.ContractAddress())

}

func TestDCMergeSnapshotRequireNullifier(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)
	dqc.creatingStates[s1.ID.String()] = s1

	// State visible without nullifier requirement
	states, err := dqc.mergeSnapshotStatesResult(ctx, schema, []*pldapi.State{}, &query.QueryJSON{
		Sort: []string{".created"},
	}, false, false)
	require.NoError(t, err)
	assert.Len(t, states, 1)

	// State NOT visible when nullifier is required and state has none
	states, err = dqc.mergeSnapshotStatesResult(ctx, schema, []*pldapi.State{}, &query.QueryJSON{
		Sort: []string{".created"},
	}, false, true)
	require.NoError(t, err)
	assert.Len(t, states, 0)

	// Attach a nullifier to the state in the snapshot
	s1.Nullifier = &pldapi.StateNullifier{ID: pldtypes.RandBytes(32), State: s1.ID}

	// Now visible when nullifier is required
	states, err = dqc.mergeSnapshotStatesResult(ctx, schema, []*pldapi.State{}, &query.QueryJSON{
		Sort: []string{".created"},
	}, false, true)
	require.NoError(t, err)
	assert.Len(t, states, 1)

}

func TestDCMergeSnapshotApplyLocksMultipleSchemas(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema1.ID()), schema1)

	schema2, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI2))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema2.ID()), schema2)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	s1, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)
	s2, err := schema2.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"tokenUri": "%s", "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32), pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	dqc.creatingStates[s1.ID.String()] = s1
	dqc.creatingStates[s2.ID.String()] = s2

	states, err := dqc.mergeSnapshotStatesResult(ctx, schema1, []*pldapi.State{},
		query.NewQueryBuilder().Sort(".created").Query(), true, false)
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, s1.State, states[0])

}

func TestDCMergeSnapshotDedupAndSpendExclusion(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// s1 is included in snapshot, s2 is included but spent
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 10, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s2, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	tx1 := uuid.New()
	tx1Str := tx1.String()
	dqc.creatingStates[s1.ID.String()] = s1
	dqc.creatingStates[s2.ID.String()] = s2
	dqc.txLocks = append(dqc.txLocks, &prototk.SnapshotStateLock{
		Type:        prototk.SnapshotStateLock_SPEND,
		StateId:     s2.ID.String(),
		Transaction: &tx1Str,
	})

	// Simulate the DB having returned s1 already — it should not be duplicated.
	// s2 is excluded because it has a spend lock. Result: 1 state (s1 from DB only).
	states, err := dqc.mergeSnapshotStatesResult(ctx, schema, []*pldapi.State{
		s1.State,
	}, &query.QueryJSON{
		Sort: []string{".created"},
	}, true, false)
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, s1.ID, states[0].ID)

}

func TestDCMergeSnapshotEvalError(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)
	dqc.creatingStates[s1.ID.String()] = s1

	_, err = dqc.mergeSnapshotStatesResult(ctx, schema, []*pldapi.State{},
		query.NewQueryBuilder().Equal("wrong", "any").Query(), true, false)
	assert.Regexp(t, "PD010700", err)

}

func TestDCMMergeAndSortStatesSortFail(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	_, err = dqc.mergeAndSortStates(ctx, schema, []*pldapi.State{
		s1.State,
	}, []*components.StateWithLabels{}, query.NewQueryBuilder().Sort("wrong").Query())
	assert.Regexp(t, "PD010700", err)
}

func TestDCMergeAndSortStatesRecoverLabelsFail(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// State with no preloaded labels and unparseable data forces the RecoverLabels re-parse to fail.
	_, err = dqc.mergeAndSortStates(ctx, schema, []*pldapi.State{
		{StateBase: pldapi.StateBase{ID: pldtypes.HexBytes(pldtypes.RandBytes(32)), Data: pldtypes.RawJSON(`!!! bad`)}},
	}, []*components.StateWithLabels{}, query.NewQueryBuilder().Query())
	require.Error(t, err)
}

func TestDCMergeSnapshotStatesResultMergeFail(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// A matching creating state ensures len(matches) > 0 so mergeInMemoryMatches runs.
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)
	dqc.creatingStates[s1.ID.String()] = s1

	// The DB state has unparseable data, so mergeInMemoryMatches fails recovering its labels.
	_, err = dqc.mergeSnapshotStatesResult(ctx, schema, []*pldapi.State{
		{StateBase: pldapi.StateBase{ID: pldtypes.HexBytes(pldtypes.RandBytes(32)), Data: pldtypes.RawJSON(`!!! bad`)}},
	}, query.NewQueryBuilder().Query(), false, false)
	require.Error(t, err)
}

func TestGetSnapshotSpendsBadStateID(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	dqc.txLocks = append(dqc.txLocks, &prototk.SnapshotStateLock{
		Type:    prototk.SnapshotStateLock_SPEND,
		StateId: "not-valid-hex",
	})

	_, err := dqc.getSnapshotSpends(ctx)
	require.Error(t, err)
}

func TestDCFindBadQuery(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema.Schema})
	require.NoError(t, err)

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	schemaID := schema.ID()
	assert.Equal(t, "type=FakeCoin(bytes32 salt,address owner,uint256 amount),labels=[owner,amount]", schema.Signature())

	_, _, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schemaID, query.NewQueryBuilder().Sort("wrong").Query())
	assert.Regexp(t, "PD010700", err)

	_, _, err = dqc.FindAvailableNullifiers(ctx, ss.p.NOTX(), schemaID, query.NewQueryBuilder().Sort("wrong").Query())
	assert.Regexp(t, "PD010700", err)

}

// TestMergeInMemoryMatchesLimit verifies that the Limit in the query is applied
// when the combined DB+snapshot result set exceeds it.
func TestMergeInMemoryMatchesLimit(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	mkState := func(amount int) *components.StateWithLabels {
		s, e := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
			`{"amount": %d, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
			amount, pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
		require.NoError(t, e)
		return s
	}

	extras := []*components.StateWithLabels{mkState(10), mkState(20), mkState(30)}
	limit := 2
	result, err := dqc.mergeAndSortStates(ctx, schema, []*pldapi.State{}, extras, query.NewQueryBuilder().Limit(limit).Sort(".created").Query())
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// TestMergeSnapshotApplyLocksClosedContext verifies that mergeSnapshotStatesResult
// returns a closed-context error when the DomainQueryContext has been closed.
func TestMergeSnapshotApplyLocksClosedContext(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	dqc.Close(ctx)

	_, err = dqc.mergeSnapshotStatesResult(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, false)
	assert.Regexp(t, "PD010122", err)
}

// TestGetStatesByIDWithSnapshotState verifies GetStatesByID returns states that
// exist only in the snapshot (not yet flushed to DB).
func TestGetStatesByIDWithSnapshotState(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema1.Schema})
	require.NoError(t, err)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	s1, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)
	dqc.creatingStates[s1.ID.String()] = s1

	_, states, err := dqc.GetStatesByID(ctx, ss.p.NOTX(), schema1.ID(), []string{s1.ID.String()})
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, s1.ID, states[0].ID)
}

// TestFindAvailableNullifiersClosedContext verifies FindAvailableNullifiers
// returns an error when the DomainQueryContext is closed.
func TestFindAvailableNullifiersClosedContext(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	dqc.Close(ctx)

	_, _, err := dqc.FindAvailableNullifiers(ctx, ss.p.NOTX(), pldtypes.Bytes32(pldtypes.RandBytes(32)), nil)
	assert.Regexp(t, "PD010122", err)
}

func TestBadSchema(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{{}})
	assert.Regexp(t, "PD010114", err)

}

func TestCheckEvalGTTimestamp(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	jq := query.NewQueryBuilder().GreaterThan(".created", 1726545933211347000).Limit(10).Sort(".created").Query()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	labelSet := dqc.ss.labelSetFor(schema)

	ls := filters.PassthroughValueSet{}

	stateID := pldtypes.MustParseHexBytes("2eaf4727b7c7e9b3728b1344ac38ea6d8698603dc3b41d9458d7c011c20ce672")

	// create time is equal - no match
	created := pldtypes.TimestampFromUnix(1726545933211347000)
	addStateBaseLabels(ls, stateID, created)
	match, err := filters.EvalQuery(ctx, jq, labelSet, ls)
	assert.NoError(t, err)
	assert.False(t, match)

	// create time is greater - match
	created = pldtypes.TimestampFromUnix(1726545933211347001)
	addStateBaseLabels(ls, stateID, created)
	match, err = filters.EvalQuery(ctx, jq, labelSet, ls)
	assert.NoError(t, err)
	assert.True(t, match)

	// create time is less - no match
	created = pldtypes.TimestampFromUnix(1726545933211346999)
	addStateBaseLabels(ls, stateID, created)
	match, err = filters.EvalQuery(ctx, jq, labelSet, ls)
	assert.NoError(t, err)
	assert.False(t, match)

}

func TestImportSnapshot(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema1.Schema})
	require.NoError(t, err)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Use a writer to pre-write states into the DB (simulating them being already confirmed)
	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", false)
	sw.contractAddress = *contractAddress

	s1, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s2, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s3ID := pldtypes.RandHex(32)

	s4, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s5, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	transactionID1 := uuid.New()
	transactionID2 := uuid.New()
	transactionID3 := uuid.New()

	stateUpserts := []*components.StateUpsert{
		{
			ID:     s1.ID,
			Schema: schema1.ID(),
			Data:   s1.Data,
		},
		{
			ID:     s2.ID,
			Schema: schema1.ID(),
			Data:   s2.Data,
		},
		{
			ID:     s4.ID,
			Schema: schema1.ID(),
			Data:   s4.Data,
		},
		{
			ID:     s5.ID,
			Schema: schema1.ID(),
			Data:   s5.Data,
		},
	}
	// Write them via the writer so they exist in DB
	tx0 := uuid.New()
	upsertWithCreate := make([]*components.StateUpsert, len(stateUpserts))
	for i, u := range stateUpserts {
		upsertWithCreate[i] = &components.StateUpsert{
			ID:        u.ID,
			Schema:    u.Schema,
			Data:      u.Data,
			CreatedBy: &tx0,
		}
	}
	_, err = sw.StageStateUpserts(ctx, ss.p.NOTX(), upsertWithCreate...)
	require.NoError(t, err)
	syncFlushWriter(t, ctx, sw)

	//imported locks include
	// - state1 created by transaction1 for which we have the data
	// - state2 created by transaction2 for which we have the data but has been spent by transaction3
	// - state3 created by transaction3 for which we do not have the data
	// - state4 created by transaction3 for which we do have the data
	// and does not include state5 even though we do have the data for that
	// so after all that, the only available states should be state1 and state 4
	lock := func(stateID, txnID string, lockType prototk.SnapshotStateLock_StateLockType) *prototk.SnapshotStateLock {
		tx := txnID
		return &prototk.SnapshotStateLock{StateId: stateID, Transaction: &tx, Type: lockType}
	}
	snapshot := &prototk.StateSnapshot{
		Locks: []*prototk.SnapshotStateLock{
			lock(s1.ID.String(), transactionID1.String(), prototk.SnapshotStateLock_CREATE),
			lock(s2.ID.String(), transactionID2.String(), prototk.SnapshotStateLock_CREATE),
			lock(s3ID, transactionID3.String(), prototk.SnapshotStateLock_CREATE),
			lock(s4.ID.String(), transactionID3.String(), prototk.SnapshotStateLock_CREATE),
			lock(s2.ID.String(), transactionID3.String(), prototk.SnapshotStateLock_SPEND),
		},
		States: snapshotStates(s1, s2, s4, s5),
	}

	err = dqc.ImportSnapshot(ctx, snapshot)
	require.NoError(t, err)
	_, states, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.Equal(t, s1.ID, states[0].ID)
	assert.Equal(t, s4.ID, states[1].ID)

}

// TestFindNullifiersSpendingExclusion exercises the spendingStates and
// spendingNullifiers filter branches inside findNullifiers.
func TestFindNullifiersSpendingExclusion(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema1.Schema})
	require.NoError(t, err)

	contractAddress, _ := newTestDomainContext(t, ctx, ss, "domain1", false)

	_, sw := newTestDomainStateWriter(t, ctx, ss, "domain1", false)
	sw.contractAddress = *contractAddress

	// Write two states each with a nullifier
	nullID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	nullID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	tx1 := uuid.New()
	states1, err := sw.StageStateUpserts(ctx, ss.p.NOTX(),
		genWidget(t, schema1.ID(), &tx1, fmt.Sprintf(`{"amount": 11, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))),
		genWidget(t, schema1.ID(), &tx1, fmt.Sprintf(`{"amount": 22, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))),
	)
	require.NoError(t, err)
	require.Len(t, states1, 2)

	err = sw.StageNullifierUpserts(ctx,
		&components.NullifierUpsert{State: states1[0].ID, ID: nullID1},
		&components.NullifierUpsert{State: states1[1].ID, ID: nullID2},
	)
	require.NoError(t, err)
	syncFlushWriter(t, ctx, sw)

	// Confirm both
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{}, []*pldapi.StateReadRecord{},
		[]*pldapi.StateConfirmRecord{
			{DomainName: "domain1", State: states1[0].ID, Transaction: tx1},
			{DomainName: "domain1", State: states1[1].ID, Transaction: tx1},
		}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	// Both are visible without exclusions
	found, err := ss.FindContractNullifiers(ctx, ss.p.NOTX(), "domain1", *contractAddress, schema1.ID(),
		query.NewQueryBuilder().Query(), pldapi.StateStatusAvailable)
	require.NoError(t, err)
	assert.Len(t, found, 2)

	// Exclude state[0] via spendingStates — only state[1] should appear
	_, found, err = ss.findNullifiers(ctx, ss.p.NOTX(), "domain1", contractAddress, schema1.ID(),
		query.NewQueryBuilder().Query(), &components.StateQueryOptions{
			StatusQualifier: pldapi.StateStatusAvailable,
			ExcludedIDs:     []pldtypes.HexBytes{states1[0].ID},
		})
	require.NoError(t, err)
	assert.Len(t, found, 1)
	assert.Equal(t, states1[1].ID, found[0].ID)

	// Exclude state[1]'s nullifier via spendingNullifiers — only state[0] should appear
	_, found, err = ss.findNullifiers(ctx, ss.p.NOTX(), "domain1", contractAddress, schema1.ID(),
		query.NewQueryBuilder().Query(), &components.StateQueryOptions{
			StatusQualifier:      pldapi.StateStatusAvailable,
			ExcludedNullifierIDs: []pldtypes.HexBytes{nullID2},
		})
	require.NoError(t, err)
	assert.Len(t, found, 1)
	assert.Equal(t, states1[0].ID, found[0].ID)

	// nil options defaults the status qualifier to "all" and returns both nullifier states
	_, found, err = ss.findNullifiers(ctx, ss.p.NOTX(), "domain1", contractAddress, schema1.ID(),
		query.NewQueryBuilder().Query(), nil)
	require.NoError(t, err)
	assert.Len(t, found, 2)

	// empty options also defaults the status qualifier, and a QueryModifier narrows to state[0]
	_, found, err = ss.findNullifiers(ctx, ss.p.NOTX(), "domain1", contractAddress, schema1.ID(),
		query.NewQueryBuilder().Query(), &components.StateQueryOptions{
			QueryModifier: func(_ persistence.DBTX, q *gorm.DB) *gorm.DB {
				return q.Where(`"states"."id" = ?`, states1[0].ID)
			},
		})
	require.NoError(t, err)
	assert.Len(t, found, 1)
	assert.Equal(t, states1[0].ID, found[0].ID)
}

func TestImportSnapshotBadStates(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	err := dqc.ImportSnapshot(ctx, &prototk.StateSnapshot{
		States: []*prototk.SnapshotState{
			{State: &prototk.EndorsableState{
				Id:            pldtypes.RandHex(32),
				SchemaId:      pldtypes.RandHex(32),
				StateDataJson: "{}",
			}},
		},
	})
	require.Regexp(t, "PD010133.*PD010106" /* unknown state schema */, err)

}

func TestImportSnapshotUnparseableStateID(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()
	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)
	// A malformed state schema ID must fail parsing at the wire boundary and be wrapped as bad states.
	err := dqc.ImportSnapshot(ctx, &prototk.StateSnapshot{
		States: []*prototk.SnapshotState{
			{State: &prototk.EndorsableState{Id: pldtypes.RandHex(32), SchemaId: "not-a-valid-hex-schema"}},
		},
	})
	assert.Error(t, err)
	assert.Regexp(t, "PD010133", err)

}

func TestImportSnapshotReplaces(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema1.Schema})
	require.NoError(t, err)

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	s1, err := schema1.ProcessState(ctx, &dqc.contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 10, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	txID := uuid.New()
	txStr := txID.String()
	snapshot := &prototk.StateSnapshot{
		Locks:  []*prototk.SnapshotStateLock{{StateId: s1.ID.String(), Transaction: &txStr, Type: prototk.SnapshotStateLock_CREATE}},
		States: snapshotStates(s1),
	}

	// Import the same snapshot multiple times
	for i := 0; i < 10; i++ {
		err = dqc.ImportSnapshot(ctx, snapshot)
		require.NoError(t, err)
	}

	// ImportSnapshot replaces the snapshot on each call; verify creatingStates doesn't accumulate.
	dqc.stateLock.Lock()
	creatingLen := len(dqc.creatingStates)
	dqc.stateLock.Unlock()
	assert.Equal(t, 1, creatingLen, "creatingStates should have exactly one entry after repeated imports")

	// Verify the import is still functional — querying should return the state
	_, states, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, s1.ID, states[0].ID)
}

func TestGetStatesByIDFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()
	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	db.ExpectQuery("SELECT.*schemas").WillReturnError(fmt.Errorf("pop"))

	_, _, err := dqc.GetStatesByID(ctx, dqc.ss.p.NOTX(), pldtypes.Bytes32(pldtypes.RandBytes(32)), []string{pldtypes.RandHex(32)})
	assert.Regexp(t, "pop", err)
}

func TestFindSnapshotMatchesBasic(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Create a state and add it to creatingStates
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)
	dqc.creatingStates[s1.ID.String()] = s1

	// Test basic matching - should return the state
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 1)
	assert.Equal(t, s1.ID, snapshotStates[0].ID)
}

func TestFindSnapshotMatchesSchemaFiltering(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema1.ID()), schema1)

	schema2, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI2))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema2.ID()), schema2)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Create states with different schemas
	s1, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s2, err := schema2.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"tokenUri": "%s", "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32), pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	dqc.creatingStates[s1.ID.String()] = s1
	dqc.creatingStates[s2.ID.String()] = s2

	// Query for schema1 - should only return s1
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema1, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 1)
	assert.Equal(t, s1.ID, snapshotStates[0].ID)

	// Query for schema2 - should only return s2
	snapshotStates, err = dqc.findSnapshotMatches(ctx, schema2, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 1)
	assert.Equal(t, s2.ID, snapshotStates[0].ID)
}

func TestFindSnapshotMatchesExcludeSpent(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Create two states
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s2, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 200, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	dqc.creatingStates[s1.ID.String()] = s1
	dqc.creatingStates[s2.ID.String()] = s2

	// Add a spend lock for s1
	txID := uuid.New()
	txIDStr := txID.String()
	dqc.txLocks = append(dqc.txLocks, &prototk.SnapshotStateLock{
		Type:        prototk.SnapshotStateLock_SPEND,
		StateId:     s1.ID.String(),
		Transaction: &txIDStr,
	})

	// With excludeSpent=true, s1 should be excluded
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), true, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 1)
	assert.Equal(t, s2.ID, snapshotStates[0].ID)

	// With excludeSpent=false, both should be included
	snapshotStates, err = dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 2)
}

func TestFindSnapshotMatchesRequireNullifier(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Create two states
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s2, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 200, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	// Add nullifier to s1 only
	nullifierID := pldtypes.RandBytes(32)
	s1.Nullifier = &pldapi.StateNullifier{
		ID:    nullifierID,
		State: s1.ID,
	}

	dqc.creatingStates[s1.ID.String()] = s1
	dqc.creatingStates[s2.ID.String()] = s2

	// With requireNullifier=true, only s1 should be returned
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, true)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 1)
	assert.Equal(t, s1.ID, snapshotStates[0].ID)

	// With requireNullifier=false, both should be returned
	snapshotStates, err = dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 2)
}

func TestFindSnapshotMatchesQueryFiltering(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Create states with different amounts
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s2, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 200, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	dqc.creatingStates[s1.ID.String()] = s1
	dqc.creatingStates[s2.ID.String()] = s2

	// Query for amount = 100 - should only return s1
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Equal("amount", int64(100)).Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 1)
	assert.Equal(t, s1.ID, snapshotStates[0].ID)

	// Query for amount = 200 - should only return s2
	snapshotStates, err = dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Equal("amount", int64(200)).Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 1)
	assert.Equal(t, s2.ID, snapshotStates[0].ID)
}

func TestFindSnapshotMatchesDuplicateDetection(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Create a state
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	dqc.creatingStates[s1.ID.String()] = s1

	// If the state is already in dbStates, it should not be returned
	dbStates := []*pldapi.State{s1.State}
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema, dbStates, query.NewQueryBuilder().Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 0)

	// If the state is not in dbStates, it should be returned
	snapshotStates, err = dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 1)
	assert.Equal(t, s1.ID, snapshotStates[0].ID)
}

func TestFindSnapshotMatchesQueryError(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Create a state
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	dqc.creatingStates[s1.ID.String()] = s1

	// Query with invalid field should return error
	_, err = dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Equal("invalidField", "value").Query(), false, false)
	assert.Error(t, err)
	assert.Regexp(t, "PD010700", err)
}

func TestFindSnapshotMatchesEmptyResults(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	_, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// No creating states - should return empty
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 0)
}

func TestFindSnapshotMatchesMultipleMatches(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Create multiple states that all match the query
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s2, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 200, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s3, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 300, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	dqc.creatingStates[s1.ID.String()] = s1
	dqc.creatingStates[s2.ID.String()] = s2
	dqc.creatingStates[s3.ID.String()] = s3

	// All states should match a query that matches all
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), false, false)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 3)

	// Verify all states are present
	matchIDs := make(map[string]bool)
	for _, m := range snapshotStates {
		matchIDs[m.ID.String()] = true
	}
	assert.True(t, matchIDs[s1.ID.String()])
	assert.True(t, matchIDs[s2.ID.String()])
	assert.True(t, matchIDs[s3.ID.String()])
}

func TestFindSnapshotMatchesCombinedFilters(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	// Create multiple states with different properties
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 100, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s2, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 200, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	s3, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 300, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction, true)
	require.NoError(t, err)

	// Add nullifier to s1 and s3
	nullifierID1 := pldtypes.RandBytes(32)
	s1.Nullifier = &pldapi.StateNullifier{
		ID:    nullifierID1,
		State: s1.ID,
	}
	nullifierID3 := pldtypes.RandBytes(32)
	s3.Nullifier = &pldapi.StateNullifier{
		ID:    nullifierID3,
		State: s3.ID,
	}

	// Add spend lock to s2
	txID := uuid.New()
	txIDStr := txID.String()
	dqc.txLocks = append(dqc.txLocks, &prototk.SnapshotStateLock{
		Type:        prototk.SnapshotStateLock_SPEND,
		StateId:     s2.ID.String(),
		Transaction: &txIDStr,
	})

	dqc.creatingStates[s1.ID.String()] = s1
	dqc.creatingStates[s2.ID.String()] = s2
	dqc.creatingStates[s3.ID.String()] = s3

	// Test with excludeSpent=true and requireNullifier=true
	// s1 and s3 have nullifiers and are not spent, s2 is spent
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema, []*pldapi.State{}, query.NewQueryBuilder().Query(), true, true)
	require.NoError(t, err)
	require.Len(t, snapshotStates, 2)
	matchIDs := make(map[string]bool)
	for _, m := range snapshotStates {
		matchIDs[m.ID.String()] = true
	}
	assert.True(t, matchIDs[s1.ID.String()])
	assert.True(t, matchIDs[s3.ID.String()])
	assert.False(t, matchIDs[s2.ID.String()])
}
