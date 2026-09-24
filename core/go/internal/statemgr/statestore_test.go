// Copyright © 2026 Kaleido, Inc.
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
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/metrics"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

// countingStateCache wraps the validated-state cache so tests can assert hit/miss behavior.
type countingStateCache struct {
	cache.Cache[string, *components.StateWithLabels]
	hits, misses int
}

func (c *countingStateCache) Get(key string) (*components.StateWithLabels, bool) {
	v, ok := c.Cache.Get(key)
	if ok {
		c.hits++
	} else {
		c.misses++
	}
	return v, ok
}

// cacheCounts returns the validated-state cache hit/miss tallies recorded so far.
func cacheCounts(ss *stateManager) (hits, misses int) {
	cc := ss.validatedStateCache.(*countingStateCache)
	return cc.hits, cc.misses
}

// peekCache inspects the validated-state cache without disturbing the hit/miss tallies.
func peekCache(ss *stateManager, key string) (*components.StateWithLabels, bool) {
	return ss.validatedStateCache.(*countingStateCache).Cache.Get(key)
}

type mockComponents struct {
	domainManager    *componentsmocks.DomainManager
	txManager        *componentsmocks.TXManager
	transportManager *componentsmocks.TransportManager
	allComponents    *componentsmocks.AllComponents
}

func newMockComponents(t *testing.T) *mockComponents {
	m := &mockComponents{}
	m.domainManager = componentsmocks.NewDomainManager(t)
	m.txManager = componentsmocks.NewTXManager(t)
	m.transportManager = componentsmocks.NewTransportManager(t)
	m.allComponents = componentsmocks.NewAllComponents(t)
	m.allComponents.On("DomainManager").Return(m.domainManager)
	m.allComponents.On("TxManager").Return(m.txManager)
	m.allComponents.On("TransportManager").Return(m.transportManager)
	m.allComponents.On("MetricsManager").Return(metrics.NewMetricsManager(context.Background())).Maybe()
	return m
}

func newDBTestStateManager(t *testing.T) (context.Context, *stateManager, *mockComponents, func()) {
	ctx := context.Background()

	logConf := pldconf.LogDefaults
	logConf.Level = confutil.P("trace")
	log.InitConfig(&logConf)

	p, pDone, err := persistence.NewUnitTestPersistence(ctx, "statemgr")
	require.NoError(t, err)
	ss := NewStateManager(ctx, &pldconf.StateStoreConfig{}, p)
	ssm := ss.(*stateManager)
	ssm.validatedStateCache = &countingStateCache{Cache: ssm.validatedStateCache}

	m := newMockComponents(t)

	ir, err := ss.PreInit(m.allComponents)
	require.NoError(t, err)
	require.NotNil(t, ir)
	require.NotEmpty(t, ir.RPCModules)

	err = ss.PostInit(m.allComponents)
	require.NoError(t, err)

	err = ss.Start()
	require.NoError(t, err)

	return ctx, ss.(*stateManager), m, func() {
		ss.Stop()
		pDone()
	}
}

func newDBMockStateManager(t *testing.T) (context.Context, *stateManager, sqlmock.Sqlmock, *mockComponents, func()) {
	ctx := context.Background()
	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	ss := NewStateManager(ctx, &pldconf.StateStoreConfig{}, p.P)
	ssm := ss.(*stateManager)
	ssm.validatedStateCache = &countingStateCache{Cache: ssm.validatedStateCache}

	m := newMockComponents(t)

	_, err = ss.PreInit(m.allComponents)
	require.NoError(t, err)

	err = ss.PostInit(m.allComponents)
	require.NoError(t, err)

	err = ss.Start()
	require.NoError(t, err)

	return ctx, ss.(*stateManager), p.Mock, m, func() {
		ss.Stop()
		require.NoError(t, p.Mock.ExpectationsWereMet())
	}
}

func TestGetTransactionStatesUnavailable(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	txID := uuid.New()
	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID3 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID4 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	err := ss.WriteStateFinalizations(ctx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{
			{DomainName: "domain1", State: stateID1, Transaction: txID},
		},
		[]*pldapi.StateReadRecord{
			{DomainName: "domain1", State: stateID2, Transaction: txID},
		},
		[]*pldapi.StateConfirmRecord{
			{DomainName: "domain1", State: stateID3, Transaction: txID},
		},
		[]*pldapi.StateInfoRecord{
			{DomainName: "domain1", State: stateID4, Transaction: txID},
		})
	require.NoError(t, err)

	txStates, err := ss.GetTransactionStates(ctx, ss.p.NOTX(), txID)
	require.NoError(t, err)
	require.Empty(t, txStates.Spent)
	require.Empty(t, txStates.Read)
	require.Empty(t, txStates.Confirmed)
	require.Equal(t, []pldtypes.HexBytes{stateID1}, txStates.Unavailable.Spent)
	require.Equal(t, []pldtypes.HexBytes{stateID2}, txStates.Unavailable.Read)
	require.Equal(t, []pldtypes.HexBytes{stateID3}, txStates.Unavailable.Confirmed)
	require.Equal(t, []pldtypes.HexBytes{stateID4}, txStates.Unavailable.Info)
}

func TestGetTransactionStatesAvailable(t *testing.T) {

	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)
	mockStateCallback(m)

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, widgetABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema.Schema})
	require.NoError(t, err)

	contractAddress := pldtypes.RandAddress()
	widgets := makeWidgets(t, ctx, ss, "domain1", contractAddress, schema.ID(), []string{
		`{"size": 11111, "color": "red",  "price": 100}`,
		`{"size": 22222, "color": "red",  "price": 150}`,
		`{"size": 33333, "color": "blue", "price": 199}`,
		`{"size": 44444, "color": "pink", "price": 199}`,
	})

	txID := uuid.New()
	err = ss.WriteStateFinalizations(ctx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{
			{DomainName: "domain1", State: widgets[0].ID, Transaction: txID},
		},
		[]*pldapi.StateReadRecord{
			{DomainName: "domain1", State: widgets[1].ID, Transaction: txID},
		},
		[]*pldapi.StateConfirmRecord{
			{DomainName: "domain1", State: widgets[2].ID, Transaction: txID},
		},
		[]*pldapi.StateInfoRecord{
			{DomainName: "domain1", State: widgets[3].ID, Transaction: txID},
		})
	require.NoError(t, err)

	txStates, err := ss.GetTransactionStates(ctx, ss.p.NOTX(), txID)
	require.NoError(t, err)
	require.Nil(t, txStates.Unavailable)
	require.Len(t, txStates.Spent, 1)
	assert.Equal(t, widgets[0].ID, txStates.Spent[0].ID)
	require.Len(t, txStates.Read, 1)
	assert.Equal(t, widgets[1].ID, txStates.Read[0].ID)
	require.Len(t, txStates.Confirmed, 1)
	assert.Equal(t, widgets[2].ID, txStates.Confirmed[0].ID)
	require.Len(t, txStates.Info, 1)
	assert.Equal(t, widgets[3].ID, txStates.Info[0].ID)
}

func TestGetTransactionStatesReadInfoMultiTx(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	txA := uuid.New()
	txB := uuid.New()
	readStateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	infoStateID := pldtypes.HexBytes(pldtypes.RandBytes(32))

	// Associate the same read and info state with transaction A
	err := ss.WriteStateFinalizations(ctx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{},
		[]*pldapi.StateReadRecord{
			{DomainName: "domain1", State: readStateID, Transaction: txA},
		},
		[]*pldapi.StateConfirmRecord{},
		[]*pldapi.StateInfoRecord{
			{DomainName: "domain1", State: infoStateID, Transaction: txA},
		})
	require.NoError(t, err)

	// Associate the same read and info state with transaction B
	err = ss.WriteStateFinalizations(ctx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{},
		[]*pldapi.StateReadRecord{
			{DomainName: "domain1", State: readStateID, Transaction: txB},
		},
		[]*pldapi.StateConfirmRecord{},
		[]*pldapi.StateInfoRecord{
			{DomainName: "domain1", State: infoStateID, Transaction: txB},
		})
	require.NoError(t, err)

	// Both transactions should see the read/info state (as unavailable since the
	// state rows themselves don't exist, only the record association rows)
	txAStates, err := ss.GetTransactionStates(ctx, ss.p.NOTX(), txA)
	require.NoError(t, err)
	require.Equal(t, []pldtypes.HexBytes{readStateID}, txAStates.Unavailable.Read)
	require.Equal(t, []pldtypes.HexBytes{infoStateID}, txAStates.Unavailable.Info)

	txBStates, err := ss.GetTransactionStates(ctx, ss.p.NOTX(), txB)
	require.NoError(t, err)
	require.Equal(t, []pldtypes.HexBytes{readStateID}, txBStates.Unavailable.Read)
	require.Equal(t, []pldtypes.HexBytes{infoStateID}, txBStates.Unavailable.Info)
}

func TestGetTransactionStatesFail(t *testing.T) {

	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT.*states").WillReturnError(fmt.Errorf("pop"))

	_, err := ss.GetTransactionStates(ctx, ss.p.NOTX(), uuid.New())
	assert.Regexp(t, "pop", err)
}

// ─── GetStateIDsMissingPrivateData ────────────────────────────────────────────

func insertTestState(t *testing.T, ss *stateManager, domainName string, id pldtypes.HexBytes) {
	t.Helper()
	schemaHash := pldtypes.Bytes32Keccak([]byte("test"))
	err := ss.p.DB(context.Background()).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&pldapi.Schema{
			ID:         schemaHash,
			DomainName: domainName,
			Type:       pldapi.SchemaTypeABI.Enum(),
		}).Error
	require.NoError(t, err)
	err = ss.p.DB(context.Background()).
		Table("states").
		Create(&pldapi.StateBase{
			ID:         id,
			DomainName: domainName,
			Schema:     schemaHash,
		}).Error
	require.NoError(t, err)
}

func TestGetStateIDsMissingPrivateData_EmptyInput(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	missing, err := ss.getStateIDsMissingPrivateData(ctx, ss.p.NOTX(), "domain1", nil)
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestGetStateIDsMissingPrivateData_AllPresent(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	id1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	id2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	insertTestState(t, ss, "domain1", id1)
	insertTestState(t, ss, "domain1", id2)

	missing, err := ss.getStateIDsMissingPrivateData(ctx, ss.p.NOTX(), "domain1", []pldtypes.HexBytes{id1, id2})
	require.NoError(t, err)
	assert.Empty(t, missing)
}

func TestGetStateIDsMissingPrivateData_NonePresent(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	id1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	id2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	missing, err := ss.getStateIDsMissingPrivateData(ctx, ss.p.NOTX(), "domain1", []pldtypes.HexBytes{id1, id2})
	require.NoError(t, err)
	require.Len(t, missing, 2)
	missingSet := map[string]bool{missing[0].String(): true, missing[1].String(): true}
	assert.True(t, missingSet[id1.String()])
	assert.True(t, missingSet[id2.String()])
}

func TestGetStateIDsMissingPrivateData_Mixed(t *testing.T) {
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	presentID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	missingID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	insertTestState(t, ss, "domain1", presentID)

	missing, err := ss.getStateIDsMissingPrivateData(ctx, ss.p.NOTX(), "domain1", []pldtypes.HexBytes{presentID, missingID})
	require.NoError(t, err)
	require.Len(t, missing, 1)
	assert.Equal(t, missingID.String(), missing[0].String())
}

func TestGetStateIDsMissingPrivateData_DomainScopedCorrectly(t *testing.T) {
	// A state present in domain2 must not count as present when querying domain1.
	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	id := pldtypes.HexBytes(pldtypes.RandBytes(32))
	insertTestState(t, ss, "domain2", id)

	missing, err := ss.getStateIDsMissingPrivateData(ctx, ss.p.NOTX(), "domain1", []pldtypes.HexBytes{id})
	require.NoError(t, err)
	require.Len(t, missing, 1)
	assert.Equal(t, id.String(), missing[0].String())
}

func TestGetStateIDsMissingPrivateData_DBError(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT.*id.*states").WillReturnError(fmt.Errorf("pop"))

	id := pldtypes.HexBytes(pldtypes.RandBytes(32))
	_, err := ss.getStateIDsMissingPrivateData(ctx, ss.p.NOTX(), "domain1", []pldtypes.HexBytes{id})
	assert.Regexp(t, "pop", err)
}
