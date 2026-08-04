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
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// testRemoteView is an in-test components.RemoteStateView standing in for the
// coordinator side of the per-select state query: it holds the ahead-of-chain candidates (already
// filtered to CREATE-locked, not-spend-locked, visible states, as the grapher does) and answers
// each query through the same FindMatchingInMemoryStates evaluation the real coordinator-side
// server uses. It records each queried schema ID.
type testRemoteView struct {
	ss            *stateManager
	domainName    string
	candidates    []*prototk.SnapshotState
	spentStateIDs []pldtypes.HexBytes
	calls         []string
	spentCalls    int
	err           error
	spentErr      error
}

func (q *testRemoteView) GetSpentStateIDs(_ context.Context) ([]pldtypes.HexBytes, error) {
	q.spentCalls++
	if q.spentErr != nil {
		return nil, q.spentErr
	}
	return q.spentStateIDs, nil
}

func (q *testRemoteView) QueryAvailableStates(ctx context.Context, schemaID string, queryJSON string) ([]*prototk.QueriedState, error) {
	q.calls = append(q.calls, schemaID)
	if q.err != nil {
		return nil, q.err
	}
	parsedSchemaID, err := pldtypes.ParseBytes32(schemaID)
	if err != nil {
		return nil, err
	}
	var jq query.QueryJSON
	if err := json.Unmarshal([]byte(queryJSON), &jq); err != nil {
		return nil, err
	}
	return q.ss.FindMatchingInMemoryStates(ctx, q.domainName, parsedSchemaID, &jq, q.candidates)
}

// staticView returns a fixed response regardless of the query — for trust-boundary tests where
// the "coordinator" answers with states it should not (tampered IDs, wrong schema, non-matches).
type staticView struct {
	states []*prototk.QueriedState
	calls  int
}

func (q *staticView) QueryAvailableStates(_ context.Context, _ string, _ string) ([]*prototk.QueriedState, error) {
	q.calls++
	return q.states, nil
}

func (q *staticView) GetSpentStateIDs(_ context.Context) ([]pldtypes.HexBytes, error) {
	return nil, nil
}

// snapshotStateOf builds a remote-view candidate (prototk.SnapshotState) for a resolved state,
// exactly as the visibility tracker holds it: full state data, its proto labels, and the ordering
// created time.
// FindMatchingInMemoryStates projects it into the query-evaluation form.
func snapshotStateOf(s *components.StateWithLabels, created int64) *prototk.SnapshotState {
	return &prototk.SnapshotState{
		State:   endorsableStateOf(s),
		Labels:  s.ProtoLabels(),
		Created: created,
	}
}

func endorsableStateOf(s *components.StateWithLabels) *prototk.EndorsableState {
	return &prototk.EndorsableState{
		Id:            s.ID.String(),
		SchemaId:      s.Schema.String(),
		StateDataJson: string(s.Data),
	}
}

func queriedStatesOf(created int64, states ...*components.StateWithLabels) []*prototk.QueriedState {
	out := make([]*prototk.QueriedState, len(states))
	for i, s := range states {
		out[i] = &prototk.QueriedState{State: endorsableStateOf(s), Created: created}
	}
	return out
}

// coinTestSetup persists the fakeCoin schema on a real DB state manager and returns a random
// contract address to build the coordinator view against — the assembly context is opened per-test.
func coinTestSetup(t *testing.T) (ctx context.Context, ss *stateManager, m *mockComponents, schema1 *abiSchema, contractAddress *pldtypes.EthAddress, done func()) {
	ctx, ss, m, dbDone := newDBTestStateManager(t)

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema1.Schema})
	require.NoError(t, err)

	return ctx, ss, m, schema1, pldtypes.RandAddress(), dbDone
}

func makeFakeCoin(t *testing.T, ctx context.Context, schema *abiSchema, contractAddress *pldtypes.EthAddress, customHash bool, amount int) *components.StateWithLabels {
	s, err := schema.ProcessStateWithLabels(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": %d, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		amount, pldtypes.RandHex(32))), nil, customHash)
	require.NoError(t, err)
	return s
}

// newTestRemoteViewContext opens an assembly context for domain1 whose view evaluates queries
// against the given states as ahead-of-chain candidates. The caller owns Close.
func newTestRemoteViewContext(t *testing.T, ctx context.Context, ss *stateManager, contractAddress *pldtypes.EthAddress, states ...*components.StateWithLabels) (*testRemoteView, *domainQueryContext) {
	candidates := make([]*prototk.SnapshotState, len(states))
	for i, s := range states {
		candidates[i] = snapshotStateOf(s, 0)
	}
	view := &testRemoteView{ss: ss, domainName: "domain1", candidates: candidates}
	return view, newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
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

	_, dc2 := newTestDomainContext(t, ctx, ss, "domain1", false)
	dc2.Close(ctx)
	_, _, err := dc2.FindAvailableStates(ctx, ss.p.NOTX(), pldtypes.Bytes32(pldtypes.RandBytes(32)), nil)
	assert.Regexp(t, "PD010122", err) // closed

}

func TestDomainQueryContextContractAddress(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	assert.Equal(t, *contractAddress, dqc.ContractAddress())

}

// TestDCMergeRemoteViewDedup proves a remote-view-returned state already present in the DB results
// is dropped without validation — the local, already-trusted copy wins.
func TestDCMergeRemoteViewDedup(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress := pldtypes.RandAddress()
	s1 := makeFakeCoin(t, ctx, schema, contractAddress, false, 10)

	view := &staticView{states: queriedStatesOf(0, s1)}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
	defer dqc.Close(ctx)

	// Simulate the DB having returned s1 already — the coordinator copy is dropped, and since
	// nothing else was returned the DB result is passed through untouched.
	q := &query.QueryJSON{Sort: []string{".created"}}
	qJSON, err := json.Marshal(q)
	require.NoError(t, err)
	queried, err := dqc.fetchRemoteViewStates(ctx, schema.ID(), string(qJSON))
	require.NoError(t, err)
	states, err := dqc.mergeRemoteViewStates(ctx, ss.p.NOTX(), schema, []*pldapi.State{
		s1.State,
	}, queried, q)
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, s1.ID, states[0].ID)

	// Dedup happens before validation, so the cache was never touched.
	hits, misses := cacheCounts(ss)
	assert.Zero(t, hits)
	assert.Zero(t, misses)
}

func TestDCMMergeAndSortStatesSortFail(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	s1, err := schema.ProcessStateWithLabels(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dqc.customHashFunction)
	require.NoError(t, err)

	_, err = dqc.mergeSortLimit(ctx, schema, []*pldapi.State{
		s1.State,
	}, nil, query.NewQueryBuilder().Sort("wrong").Query(), ss.labelSetFor(schema))
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
	_, err = dqc.mergeSortLimit(ctx, schema, []*pldapi.State{
		{StateBase: pldapi.StateBase{ID: pldtypes.HexBytes(pldtypes.RandBytes(32)), Data: pldtypes.RawJSON(`!!! bad`)}},
	}, nil, query.NewQueryBuilder().Query(), ss.labelSetFor(schema))
	require.Error(t, err)
}

func TestDCMergeCoordinatorStatesResultMergeFail(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress := pldtypes.RandAddress()

	// A matching coordinator state makes the merge run.
	s1 := makeFakeCoin(t, ctx, schema, contractAddress, false, 20)
	view := &staticView{states: queriedStatesOf(0, s1)}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
	defer dqc.Close(ctx)

	// The DB state has unparseable data, so the merge fails recovering its labels.
	q := query.NewQueryBuilder().Query()
	qJSON, err := json.Marshal(q)
	require.NoError(t, err)
	queried, err := dqc.fetchRemoteViewStates(ctx, schema.ID(), string(qJSON))
	require.NoError(t, err)
	_, err = dqc.mergeRemoteViewStates(ctx, ss.p.NOTX(), schema, []*pldapi.State{
		{StateBase: pldapi.StateBase{ID: pldtypes.HexBytes(pldtypes.RandBytes(32)), Data: pldtypes.RawJSON(`!!! bad`)}},
	}, queried, q)
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
// when the combined DB+coordinator result set exceeds it.
func TestMergeInMemoryMatchesLimit(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dqc.Close(ctx)

	mkState := func(amount int) *components.StateWithLabels {
		s, e := schema.ProcessStateWithLabels(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
			`{"amount": %d, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
			amount, pldtypes.RandHex(32))), nil, dqc.customHashFunction)
		require.NoError(t, e)
		return s
	}

	dbStates := []*pldapi.State{mkState(10).State, mkState(20).State, mkState(30).State}
	limit := 2
	result, err := dqc.mergeSortLimit(ctx, schema, dbStates, nil, query.NewQueryBuilder().Limit(limit).Sort(".created").Query(), ss.labelSetFor(schema))
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// TestMergeCoordinatorClosedContext verifies that a closed assembly DomainQueryContext fails at
// the query entry points, before the DB read or the remote fetch is launched.
func TestMergeCoordinatorClosedContext(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress := pldtypes.RandAddress()
	view := &staticView{}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
	dqc.Close(ctx)

	_, _, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema.ID(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD010122", err)

	_, _, err = dqc.GetStatesByID(ctx, ss.p.NOTX(), schema.ID(), []string{pldtypes.RandHex(32)})
	assert.Regexp(t, "PD010122", err)

	assert.Zero(t, view.calls)
}

// blockingDBTX holds every DB access blocked until release is closed, signalling the first
// access — so a test can prove other work proceeds while the DB read is in flight.
type blockingDBTX struct {
	persistence.DBTX
	dbAccessed chan struct{}
	release    chan struct{}
	accessOnce sync.Once
}

func (b *blockingDBTX) DB(ctx context.Context) *gorm.DB {
	b.accessOnce.Do(func() { close(b.dbAccessed) })
	<-b.release
	return b.DBTX.DB(ctx)
}

// fetchSignallingView signals when the remote view query starts.
type fetchSignallingView struct {
	components.RemoteStateView
	fetchStarted chan struct{}
	fetchOnce    sync.Once
}

func (v *fetchSignallingView) QueryAvailableStates(ctx context.Context, schemaID string, queryJSON string) ([]*prototk.QueriedState, error) {
	v.fetchOnce.Do(func() { close(v.fetchStarted) })
	return v.RemoteStateView.QueryAvailableStates(ctx, schemaID, queryJSON)
}

// TestFindAvailableStatesOverlapsRemoteFetch proves the remote view query runs concurrently with
// the DB read: the DB read is held blocked, and the remote fetch must still start before it is
// released. A regression to serial ordering deadlocks here (caught by the suite timeout).
func TestFindAvailableStatesOverlapsRemoteFetch(t *testing.T) {
	ctx, ss, _, schema, contractAddress, done := coinTestSetup(t)
	defer done()

	s1 := makeFakeCoin(t, ctx, schema, contractAddress, false, 10)
	view := &fetchSignallingView{
		RemoteStateView: &testRemoteView{ss: ss, domainName: "domain1", candidates: []*prototk.SnapshotState{snapshotStateOf(s1, 0)}},
		fetchStarted:    make(chan struct{}),
	}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
	defer dqc.Close(ctx)

	dbTX := &blockingDBTX{DBTX: ss.p.NOTX(), dbAccessed: make(chan struct{}), release: make(chan struct{})}

	var states []*pldapi.State
	found := make(chan error, 1)
	go func() {
		var err error
		_, states, err = dqc.FindAvailableStates(ctx, dbTX, schema.ID(), query.NewQueryBuilder().Sort(".created").Query())
		found <- err
	}()

	<-dbTX.dbAccessed   // the DB read is in flight and held blocked
	<-view.fetchStarted // the remote fetch started anyway — it did not wait for the DB read

	close(dbTX.release)
	require.NoError(t, <-found)
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

// TestFindAvailableStatesWithRemoteView is the end-to-end shape of an assembly select: the
// remote view answers from the coordinator's ahead-of-chain candidates, the returned states
// validate, and repeat selections are served from the validated-state cache instead of
// re-validating.
func TestFindAvailableStatesWithRemoteView(t *testing.T) {

	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	s2 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)

	view, dqc := newTestRemoteViewContext(t, ctx, ss, contractAddress, s1, s2)
	defer dqc.Close(ctx)

	_, states, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.ElementsMatch(t, []pldtypes.HexBytes{s1.ID, s2.ID}, []pldtypes.HexBytes{states[0].ID, states[1].ID})
	require.Len(t, view.calls, 1)
	assert.Equal(t, schema1.ID().String(), view.calls[0])

	// The first query validated and cached both states (content-only), so a second identical query
	// still round-trips to the coordinator but re-validation is served from validatedStateCache.
	_, misses := cacheCounts(ss)
	require.Equal(t, 2, misses)
	_, states, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 2)
	hits, misses := cacheCounts(ss)
	assert.Equal(t, 2, hits, "second query must be served from the cache")
	assert.Equal(t, 2, misses)
}

// TestFindAvailableStatesQueryEvaluatedOnCoordinator proves the label filter, sort and limit are
// applied to the coordinator's candidates.
func TestFindAvailableStatesQueryEvaluatedOnCoordinator(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	s10 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	s20 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)
	s30 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 30)

	_, dqc := newTestRemoteViewContext(t, ctx, ss, contractAddress, s10, s20, s30)
	defer dqc.Close(ctx)

	// Label equality
	_, states, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(),
		query.NewQueryBuilder().Equal("amount", 20).Query())
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, s20.ID, states[0].ID)

	// Sort descending on the amount label with a limit
	_, states, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(),
		query.NewQueryBuilder().Limit(2).Sort("-amount").Query())
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.Equal(t, s30.ID, states[0].ID)
	assert.Equal(t, s20.ID, states[1].ID)
}

// TestFindAvailableStatesSchemaFilteredOnCoordinator proves a query for a different schema matches
// none of the coordinator's candidates.
func TestFindAvailableStatesSchemaFilteredOnCoordinator(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	schema2, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI2))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema2.Schema})
	require.NoError(t, err)

	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	view, dqc := newTestRemoteViewContext(t, ctx, ss, contractAddress, s1)
	defer dqc.Close(ctx)

	_, states, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema2.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	assert.Empty(t, states)
	assert.Len(t, view.calls, 1)
}

// TestNullifierQueriesSkipRemoteView proves nullifier queries never round-trip to the coordinator —
// its ahead-of-chain view carries no nullifiers.
func TestNullifierQueriesSkipRemoteView(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	view, dqc := newTestRemoteViewContext(t, ctx, ss, contractAddress, s1)
	defer dqc.Close(ctx)

	_, states, err := dqc.FindAvailableNullifiers(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	assert.Empty(t, states)
	assert.Empty(t, view.calls)
}

// TestRemoteViewQueryError proves a failed remote view round-trip fails the whole select.
func TestRemoteViewQueryError(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	view := &testRemoteView{err: errors.New("pop")}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
	defer dqc.Close(ctx)

	_, _, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD010134.*pop", err)
}

// --- validation of coordinator-returned states (the coordinator is not trusted) ---

// TestQueriedStateBadIDsRejected proves unparseable IDs on returned states fail the select.
func TestQueriedStateBadIDsRejected(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	// Bad state ID
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, &staticView{
		states: []*prototk.QueriedState{{State: &prototk.EndorsableState{Id: "not-hex", SchemaId: schema1.ID().String()}}},
	})
	_, _, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.Error(t, err)
	dqc.Close(ctx)

	// Bad schema ID
	dqc2 := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, &staticView{
		states: []*prototk.QueriedState{{State: &prototk.EndorsableState{Id: pldtypes.RandHex(32), SchemaId: "not-a-schema"}}},
	})
	defer dqc2.Close(ctx)
	_, _, err = dqc2.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.Error(t, err)
}

// TestQueriedStateSchemaMismatch proves a returned state carrying a schema other than the queried
// one fails the whole select.
func TestQueriedStateSchemaMismatch(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	schema2, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI2))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema2.Schema})
	require.NoError(t, err)

	// The coordinator answers a schema2 query with a state that validates under schema1.
	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, &staticView{
		states: queriedStatesOf(0, s1),
	})
	defer dqc.Close(ctx)

	_, _, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema2.ID(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD010136", err)
}

// TestQueriedStateNoMatchRejected proves the query is re-evaluated against the authoritative
// recomputed labels: a returned state that does not really match the query fails the whole select —
// on both the fresh-validation path and the cache-hit path.
func TestQueriedStateNoMatchRejected(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	// The coordinator returns the amount=10 state for a query demanding amount=9999.
	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, &staticView{
		states: queriedStatesOf(0, s1),
	})
	defer dqc.Close(ctx)

	jq := query.NewQueryBuilder().Equal("amount", 9999).Query()
	_, _, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), jq)
	assert.Regexp(t, "PD010135", err)

	// The failed select still cached the (validly hashed) content — the re-evaluation must reject
	// again on a cache hit.
	_, misses := cacheCounts(ss)
	require.Equal(t, 1, misses)
	_, _, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), jq)
	assert.Regexp(t, "PD010135", err)
	hits, _ := cacheCounts(ss)
	assert.Equal(t, 1, hits)
}

// TestQueriedStatePopulatesCache proves the queried-state path is a content-only cache participant:
// a miss validates + stores a content-only entry (Created == 0) under the state's content id, the
// returned state carries the coordinator's created (not the zeroed cache value), and a second query
// for the same state is served from the cache.
func TestQueriedStatePopulatesCache(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)

	const coordinatorCreated int64 = 1_700_000_000_000_000_000
	view := &testRemoteView{ss: ss, domainName: "domain1", candidates: []*prototk.SnapshotState{snapshotStateOf(s1, coordinatorCreated)}}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
	defer dqc.Close(ctx)

	_, states, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, s1.ID, states[0].ID)
	assert.Equal(t, pldtypes.Timestamp(coordinatorCreated), states[0].Created, "returned state must carry the coordinator's created")

	// The miss validated and stored a content-only entry.
	hits, misses := cacheCounts(ss)
	assert.Equal(t, 0, hits)
	assert.Equal(t, 1, misses, "the first select is a cache miss that validates and populates the cache")
	cached, ok := peekCache(ss, validatedStateCacheKey("domain1", *contractAddress, s1.ID))
	require.True(t, ok, "queried-state validation must populate the shared cache")
	assert.Equal(t, pldtypes.Timestamp(0), cached.Created, "cached entry must be content-only (Created == 0)")

	// A second query for the same state hits the cache — and still carries the coordinator's
	// created rather than the zeroed cache value.
	_, states, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, pldtypes.Timestamp(coordinatorCreated), states[0].Created)
	hits, _ = cacheCounts(ss)
	assert.Equal(t, 1, hits)
}

// TestQueriedStateCustomHashBypass proves customHashFunction states are never cached, and that the
// validation path verifies their hashes through the domain on every select.
func TestQueriedStateCustomHashBypass(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema1.Schema})
	require.NoError(t, err)

	contractAddress := pldtypes.RandAddress()

	id := pldtypes.HexBytes(pldtypes.RandBytes(32))
	s1, err := schema1.ProcessStateWithLabels(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), id, true)
	require.NoError(t, err)

	// Every select must verify the whole batch through the domain for custom-hash domains.
	md := componentsmocks.NewDomain(t)
	m.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
	md.On("ValidateStateHashes", mock.Anything, mock.Anything).Return([]pldtypes.HexBytes{s1.ID}, nil).Twice()

	view := &testRemoteView{ss: ss, domainName: "domain1", candidates: []*prototk.SnapshotState{snapshotStateOf(s1, 0)}}

	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", true, contractAddress, view)
	defer dqc.Close(ctx)
	_, states, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 1)
	_, states, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 1)

	hits, misses := cacheCounts(ss)
	assert.Equal(t, 0, hits)
	assert.Equal(t, 0, misses, "customHashFunction states must not touch the cache")
	_, ok := peekCache(ss, validatedStateCacheKey("domain1", *contractAddress, s1.ID))
	assert.False(t, ok)
}

// TestQueriedStateCustomHashDomainErrors covers the custom-hash verification failure modes.
func TestQueriedStateCustomHashDomainErrors(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	schemaC, err := newABISchema(ctx, "customdomain", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schemaC.Schema})
	require.NoError(t, err)

	contractAddress := pldtypes.RandAddress()

	id := pldtypes.HexBytes(pldtypes.RandBytes(32))
	s1, err := schemaC.ProcessStateWithLabels(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 10, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), id, true)
	require.NoError(t, err)

	dqc := newTestAssemblyContext(t, ctx, ss, "customdomain", true, contractAddress, &staticView{
		states: queriedStatesOf(0, s1),
	})
	defer dqc.Close(ctx)

	// The domain lookup fails: the whole select fails.
	m.domainManager.On("GetDomainByName", mock.Anything, "customdomain").Return(nil, errors.New("no such domain")).Once()
	_, _, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schemaC.ID(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "no such domain", err)

	// The domain rejects the batch hashes: the whole select fails.
	md := componentsmocks.NewDomain(t)
	m.domainManager.On("GetDomainByName", mock.Anything, "customdomain").Return(md, nil)
	md.On("ValidateStateHashes", mock.Anything, mock.Anything).Return(nil, errors.New("bad hashes")).Once()
	_, _, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schemaC.ID(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "bad hashes", err)

	// The domain-verified hash differs from the claimed ID: the whole select fails.
	otherID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	md.On("ValidateStateHashes", mock.Anything, mock.Anything).Return([]pldtypes.HexBytes{otherID}, nil).Once()
	_, _, err = dqc.FindAvailableStates(ctx, ss.p.NOTX(), schemaC.ID(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD010129", err)
}

// TestQueriedStateCachePoisoning proves returned data whose claimed ID does not match its content
// fails hash verification and is never inserted into the cache.
func TestQueriedStateCachePoisoning(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)

	// Serve s1's real content claiming a tampered ID.
	badID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, &staticView{
		states: []*prototk.QueriedState{{State: &prototk.EndorsableState{
			Id:            badID.String(),
			SchemaId:      schema1.ID().String(),
			StateDataJson: string(s1.Data),
		}}},
	})
	defer dqc.Close(ctx)

	_, _, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD010129", err) // state hash mismatch

	// The miss goes through the caching validation path, but ProcessState rejects the content/id
	// mismatch before anything is stored, so the tampered id never poisons the cache.
	hits, misses := cacheCounts(ss)
	assert.Equal(t, 0, hits)
	assert.Equal(t, 1, misses)
	_, ok := peekCache(ss, validatedStateCacheKey("domain1", *contractAddress, badID))
	assert.False(t, ok, "hash-mismatched data must never be cached")
}

// TestQueriedCreatedOrdering proves coordinator states sort by the created carried on the query
// response, merged against DB states on the shared ".created" axis.
func TestQueriedCreatedOrdering(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	early := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	late := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)

	view := &testRemoteView{ss: ss, domainName: "domain1", candidates: []*prototk.SnapshotState{
		snapshotStateOf(late, 2000),
		snapshotStateOf(early, 1000),
	}}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
	defer dqc.Close(ctx)

	_, states, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Sort(".created").Query())
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.Equal(t, early.ID, states[0].ID, "created=1000 sorts first")
	assert.Equal(t, late.ID, states[1].ID, "created=2000 sorts last")
	assert.Equal(t, pldtypes.Timestamp(1000), states[0].Created)
	assert.Equal(t, pldtypes.Timestamp(2000), states[1].Created)
}

// TestValidatedStateCacheContentOnly proves ValidateStatesWithLabels returns content-only states (Created == 0
// — created is the caller's to stamp), caches content-only, and hands each caller an isolated copy so
// a caller can stamp its own created without mutating the shared entry or another caller's state.
func TestValidatedStateCacheContentOnly(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	require.NoError(t, ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema1.Schema}))
	contractAddress := pldtypes.RandAddress()

	s := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	es := &prototk.EndorsableState{Id: s.ID.String(), SchemaId: schema1.ID().String(), StateDataJson: string(s.Data)}
	key := validatedStateCacheKey("domain1", *contractAddress, s.ID)

	md := componentsmocks.NewDomain(t)
	md.On("Name").Return("domain1")
	md.On("CustomHashFunction").Return(false)

	// First call: miss → validates and caches content-only; the returned state carries no created.
	out1, err := ss.ValidateStatesWithLabels(ctx, ss.p.NOTX(), md, *contractAddress, es)
	require.NoError(t, err)
	require.Len(t, out1, 1)
	assert.Equal(t, pldtypes.Timestamp(0), out1[0].Created, "created is the caller's to stamp, not set here")

	cached, ok := peekCache(ss, key)
	require.True(t, ok)
	assert.Equal(t, pldtypes.Timestamp(0), cached.Created, "cached entry must be content-only")

	// Second call: cache hit, still content-only.
	out2, err := ss.ValidateStatesWithLabels(ctx, ss.p.NOTX(), md, *contractAddress, es)
	require.NoError(t, err)
	assert.Equal(t, pldtypes.Timestamp(0), out2[0].Created)

	// Copy isolation: distinct State pointers, so a caller stamping created on one leaves the cache and
	// the other caller untouched.
	assert.NotSame(t, out1[0].State, out2[0].State)
	assert.NotSame(t, cached.State, out1[0].State)
	out1[0].Created = 12345
	cachedAfter, _ := peekCache(ss, key)
	assert.Equal(t, pldtypes.Timestamp(0), cachedAfter.Created, "stamping a caller's copy must not touch the cache")
	assert.Equal(t, pldtypes.Timestamp(0), out2[0].Created, "nor another caller's copy")

	hits, misses := cacheCounts(ss)
	assert.Equal(t, 1, hits)
	assert.Equal(t, 1, misses)
}

// TestQueriedStatesMixedHitMiss proves a select returning one already-cached state and one uncached
// state validates only the miss, while the hit is served from the cache.
func TestQueriedStatesMixedHitMiss(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	cachedState := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	freshState := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)

	// Seed the cache with cachedState only, via a select that returns just it.
	_, dqc1 := newTestRemoteViewContext(t, ctx, ss, contractAddress, cachedState)
	_, _, err := dqc1.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	dqc1.Close(ctx)

	hits, misses := cacheCounts(ss)
	require.Equal(t, 0, hits)
	require.Equal(t, 1, misses)

	_, dqc2 := newTestRemoteViewContext(t, ctx, ss, contractAddress, cachedState, freshState)
	defer dqc2.Close(ctx)
	_, states, err := dqc2.FindAvailableStates(ctx, ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Sort(".created").Query())
	require.NoError(t, err)
	require.Len(t, states, 2)

	hits, misses = cacheCounts(ss)
	assert.Equal(t, 1, hits, "the previously-validated state must hit")
	assert.Equal(t, 2, misses, "only the fresh state re-validates")
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

	_, sw, resolve := newTestDomainStateWriter(t, ctx, ss, "domain1", false)
	sw.contractAddress = *contractAddress

	// Write two states each with a nullifier
	nullID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	nullID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	tx1 := uuid.New()
	states1, err := resolve(
		genWidget(t, schema1.ID(), fmt.Sprintf(`{"amount": 11, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))),
		genWidget(t, schema1.ID(), fmt.Sprintf(`{"amount": 22, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))),
	)
	require.NoError(t, err)
	require.Len(t, states1, 2)

	nullifiers1 := []*pldapi.StateNullifier{
		{DomainName: "domain1", State: states1[0].ID, ID: nullID1},
		{DomainName: "domain1", State: states1[1].ID, ID: nullID2},
	}
	err = sw.StageWrites(ctx, states1, nullifiers1...)
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

// TestQueriedStateEvalQueryError proves a query that cannot be evaluated against the schema's
// label set fails the validation of view-returned states.
func TestQueriedStateEvalQueryError(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress := pldtypes.RandAddress()
	s1 := makeFakeCoin(t, ctx, schema, contractAddress, false, 10)

	view := &staticView{states: queriedStatesOf(0, s1)}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
	defer dqc.Close(ctx)

	// The returned state validates, but the query references an unknown field, so the
	// re-evaluation against the recomputed labels fails.
	q := query.NewQueryBuilder().Equal("wrong", "any").Query()
	queried, err := dqc.fetchRemoteViewStates(ctx, schema.ID(), "{}")
	require.NoError(t, err)
	_, err = dqc.mergeRemoteViewStates(ctx, ss.p.NOTX(), schema, nil, queried, q)
	assert.Regexp(t, "PD010700", err)
}

// TestFindAvailableStatesSpentIDsFetchError proves a failure fetching the remote view's spend
// exclusion set fails FindAvailableStates before any DB read or remote fetch.
func TestFindAvailableStatesSpentIDsFetchError(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	view := &testRemoteView{spentErr: errors.New("pop")}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, pldtypes.RandAddress(), view)
	defer dqc.Close(ctx)

	_, _, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), pldtypes.RandBytes32(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD010137.*pop", err)
	assert.Empty(t, view.calls)
}

// TestFindAvailableNullifiersSpentIDsFetchError proves the same spend exclusion fetch failure
// fails FindAvailableNullifiers.
func TestFindAvailableNullifiersSpentIDsFetchError(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	view := &testRemoteView{spentErr: errors.New("pop")}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, pldtypes.RandAddress(), view)
	defer dqc.Close(ctx)

	_, _, err := dqc.FindAvailableNullifiers(ctx, ss.p.NOTX(), pldtypes.RandBytes32(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD010137.*pop", err)
	assert.Empty(t, view.calls)
}

// TestFindAvailableStatesUnmarshalableQuery proves a query that cannot be re-marshaled for the
// remote round-trip fails the select without the remote fetch ever being launched.
func TestFindAvailableStatesUnmarshalableQuery(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	view := &staticView{}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, pldtypes.RandAddress(), view)
	defer dqc.Close(ctx)

	// A raw JSON value that is not valid JSON fails json.Marshal of the whole query.
	q := &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				Equal: []*query.OpSingleVal{
					{Op: query.Op{Field: "amount"}, Value: pldtypes.RawJSON(`!not-json`)},
				},
			},
		},
	}
	_, _, err := dqc.FindAvailableStates(ctx, ss.p.NOTX(), pldtypes.RandBytes32(), q)
	assert.Regexp(t, "invalid character", err)
	assert.Zero(t, view.calls)
}

// TestGetStatesByIDWithRemoteView proves GetStatesByID merges remote-view-served states with
// DB states: one requested state lives only in the DB, the other only in the remote view, and
// both come back.
func TestGetStatesByIDWithRemoteView(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	// sDB is written (unconfirmed) to the DB; sRemote is known only to the remote view.
	sDB := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	sRemote := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)

	_, sw, resolve := newTestDomainStateWriter(t, ctx, ss, "domain1", false)
	sw.contractAddress = *contractAddress
	dbStates, err := resolve(endorsableStateOf(sDB))
	require.NoError(t, err)
	require.NoError(t, sw.StageWrites(ctx, dbStates))
	syncFlushWriter(t, ctx, sw)

	view, dqc := newTestRemoteViewContext(t, ctx, ss, contractAddress, sRemote)
	defer dqc.Close(ctx)

	schema, states, err := dqc.GetStatesByID(ctx, ss.p.NOTX(), schema1.ID(), []string{sDB.ID.String(), sRemote.ID.String()})
	require.NoError(t, err)
	require.NotNil(t, schema)
	require.Len(t, states, 2)
	assert.ElementsMatch(t, []pldtypes.HexBytes{sDB.ID, sRemote.ID}, []pldtypes.HexBytes{states[0].ID, states[1].ID})
	require.Len(t, view.calls, 1)
	assert.Equal(t, schema1.ID().String(), view.calls[0])
}

// TestGetStatesByIDRemoteFetchError proves a failed remote view round-trip fails GetStatesByID
// even though the DB read succeeds.
func TestGetStatesByIDRemoteFetchError(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	view := &testRemoteView{err: errors.New("pop")}
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, view)
	defer dqc.Close(ctx)

	_, _, err := dqc.GetStatesByID(ctx, ss.p.NOTX(), schema1.ID(), []string{pldtypes.RandHex(32)})
	assert.Regexp(t, "PD010134.*pop", err)
}

// TestGetStatesByIDMergeError proves a remote-view answer that does not satisfy the ID query —
// the view returned a state whose ID was never asked for — fails the whole lookup.
func TestGetStatesByIDMergeError(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	dqc := newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress, &staticView{
		states: queriedStatesOf(0, s1),
	})
	defer dqc.Close(ctx)

	_, _, err := dqc.GetStatesByID(ctx, ss.p.NOTX(), schema1.ID(), []string{pldtypes.RandHex(32)})
	assert.Regexp(t, "PD010135", err)
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

func TestNewDomainQueryContextWithRemoteView_DelegatesSpentIDsToView(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	spent := []pldtypes.HexBytes{pldtypes.RandBytes(32), pldtypes.RandBytes(32)}
	view := &testRemoteView{spentStateIDs: spent}

	md := componentsmocks.NewDomain(t)
	md.On("Name").Return("domain1")
	md.On("CustomHashFunction").Return(false)

	dqc := ss.NewDomainQueryContextWithRemoteView(ctx, md, *pldtypes.RandAddress(), view)
	defer dqc.Close(ctx)

	// Construction does not fetch the exclusion set.
	assert.Zero(t, view.spentCalls)

	// getSpentStateIDs delegates straight to the view, so each call reaches it.
	got, err := dqc.(*domainQueryContext).getSpentStateIDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, spent, got)
	assert.Equal(t, 1, view.spentCalls)

	got, err = dqc.(*domainQueryContext).getSpentStateIDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, spent, got)
	assert.Equal(t, 2, view.spentCalls)
}

func TestNewDomainQueryContextWithRemoteView_SpentIDsFetchError(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	view := &testRemoteView{spentErr: errors.New("pop")}

	md := componentsmocks.NewDomain(t)
	md.On("Name").Return("domain1")
	md.On("CustomHashFunction").Return(false)

	dqc := ss.NewDomainQueryContextWithRemoteView(ctx, md, *pldtypes.RandAddress(), view)
	defer dqc.Close(ctx)

	// The fetch is lazy, so the error surfaces from the query rather than construction, and a
	// failed fetch is not cached — the next call retries.
	_, err := dqc.(*domainQueryContext).getSpentStateIDs(ctx)
	assert.Regexp(t, "PD010137", err)
	assert.Regexp(t, "pop", err)
	assert.Equal(t, 1, view.spentCalls)

	_, err = dqc.(*domainQueryContext).getSpentStateIDs(ctx)
	assert.Regexp(t, "PD010137", err)
	assert.Equal(t, 2, view.spentCalls)
}
