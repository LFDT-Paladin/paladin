// Copyright © 2024 Kaleido, Inc.
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
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPersistStateMissingSchema(t *testing.T) {
	ctx, ss, db, m, done := newDBMockStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))
	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	upserts := []*components.StateUpsertOutsideContext{
		{
			ContractAddress: pldtypes.RandAddress(),
			SchemaID:        pldtypes.Bytes32Keccak(([]byte)("test")),
		},
	}

	_, err := ss.WritePreVerifiedStates(ctx, ss.p.NOTX(), "domain1", upserts)
	assert.Regexp(t, "PD010106", err)

	_, err = ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", upserts)
	assert.Regexp(t, "PD010106", err)
}

func TestPersistStateInvalidState(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)

	schemaID := pldtypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	upserts := []*components.StateUpsertOutsideContext{
		{
			ContractAddress: pldtypes.RandAddress(),
			SchemaID:        schemaID,
		},
	}

	_, err := ss.WritePreVerifiedStates(ctx, ss.p.NOTX(), "domain1", upserts)
	assert.Regexp(t, "PD010116", err)

	_, err = ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", upserts)
	assert.Regexp(t, "PD010116", err)
}

func TestGetStateMissing(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	stateID := pldtypes.Bytes32Keccak(([]byte)("state1")).Bytes()
	_, err := ss.GetStatesByID(ctx, ss.p.NOTX(), "domain1", nil, []pldtypes.HexBytes{stateID}, true, false)
	assert.Regexp(t, "PD010112", err)
}

func TestFindStatesMissingSchema(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	contractAddress := pldtypes.RandAddress()
	_, _, err := ss.findStates(ctx, ss.p.NOTX(), "domain1", contractAddress, pldtypes.Bytes32Keccak(([]byte)("schema1")), &query.QueryJSON{}, "all")
	assert.Regexp(t, "PD010106", err)
}

func TestFindStatesBadQuery(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schemaID := pldtypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	contractAddress := pldtypes.RandAddress()
	_, _, err := ss.findStates(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				Equal: []*query.OpSingleVal{
					{Op: query.Op{Field: "wrong"}},
				},
			},
		},
	}, "all")
	assert.Regexp(t, "PD010700.*wrong", err)

}

func TestFindStatesFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	schemaID := pldtypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		Schema:     &pldapi.Schema{ID: schemaID},
		definition: &abi.Parameter{},
	})

	db.ExpectQuery("SELECT.*created").WillReturnError(fmt.Errorf("pop"))

	contractAddress := pldtypes.RandAddress()
	_, _, err := ss.findStates(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				GreaterThan: []*query.OpSingleVal{
					{Op: query.Op{
						Field: ".created",
					}, Value: pldtypes.RawJSON(fmt.Sprintf("%d", time.Now().UnixNano()))},
				},
			},
		},
	}, "all")
	assert.Regexp(t, "pop", err)

}

func TestWritePreVerifiedStateInvalidDomain(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	m.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(nil, fmt.Errorf("not found"))

	_, err := ss.WritePreVerifiedStates(ctx, ss.p.NOTX(), "domain1", []*components.StateUpsertOutsideContext{})
	assert.Regexp(t, "not found", err)

	_, err = ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", []*components.StateUpsertOutsideContext{})
	assert.Regexp(t, "not found", err)

}

func TestWriteReceivedStatesValidateHashFail(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	md := mockDomain(t, m, "domain1", true)
	md.On("ValidateStateHashes", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	_, err := ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", []*components.StateUpsertOutsideContext{
		{ID: pldtypes.RandBytes(32), SchemaID: pldtypes.RandBytes32(),
			Data: pldtypes.RawJSON(fmt.Sprintf(
				`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
				pldtypes.RandHex(32)))},
	})
	assert.Regexp(t, "pop", err)

}
func TestWriteReceivedStatesValidateHashOkInsertFail(t *testing.T) {
	ctx, ss, db, m, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec("INSERT.*states").WillReturnError(fmt.Errorf("pop"))

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema1.ID()), schema1)

	md := mockDomain(t, m, "domain1", true)
	stateID1 := pldtypes.RandBytes(32)
	md.On("ValidateStateHashes", mock.Anything, mock.Anything).Return([]pldtypes.HexBytes{stateID1}, nil)

	_, err = ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", []*components.StateUpsertOutsideContext{
		{SchemaID: schema1.ID(), Data: pldtypes.RawJSON(fmt.Sprintf(
			`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
			pldtypes.RandHex(32)))},
	})
	assert.Regexp(t, "pop", err)

}

func TestWriteNullifiersForReceivedStatesOkRealDB(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	md := componentsmocks.NewDomain(t)
	m.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)

	err := ss.WriteNullifiersForReceivedStates(ctx, ss.p.NOTX(), "domain1", []*pldapi.StateNullifier{
		{
			DomainName: "domain1",
			ID:         pldtypes.HexBytes(pldtypes.RandHex(32)),
			State:      pldtypes.HexBytes(pldtypes.RandHex(32)),
		},
		{
			DomainName: "domain1",
			ID:         pldtypes.HexBytes(pldtypes.RandHex(32)),
			State:      pldtypes.HexBytes(pldtypes.RandHex(32)),
		},
	})
	require.NoError(t, err)

}

func TestWriteNullifiersForReceivedStatesBadDomain(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	m.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(nil, fmt.Errorf("not found"))

	err := ss.WriteNullifiersForReceivedStates(ctx, ss.p.NOTX(), "domain1", []*pldapi.StateNullifier{
		{
			DomainName: "domain1",
			ID:         pldtypes.HexBytes(pldtypes.RandHex(32)),
			State:      pldtypes.HexBytes(pldtypes.RandHex(32)),
		},
	})
	assert.Regexp(t, "not found", err)

}

func TestWritePreVerifiedStates_ClearsCompletionRows(t *testing.T) {
	// Writing states should delete any outstanding completion rows for those state IDs.
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema.Schema})
	require.NoError(t, err)

	_ = mockDomain(t, m, "domain1", false)
	m.txManager.On("NotifyStatesDBChanged", mock.Anything).Return()

	contractAddr := pldtypes.RandAddress()

	// We need the real state IDs to pre-insert completion rows, so write the states first.
	var states []*pldapi.State
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		states, err = ss.WritePreVerifiedStates(ctx, dbTX, "domain1", []*components.StateUpsertOutsideContext{
			{
				SchemaID:        schema.ID(),
				Data:            pldtypes.RawJSON(fmt.Sprintf(`{"amount":10,"owner":"0x615dD09124271D8008225054d85Ffe720E7a447A","salt":"%s"}`, pldtypes.RandHex(32))),
				ContractAddress: contractAddr,
			},
			{
				SchemaID:        schema.ID(),
				Data:            pldtypes.RawJSON(fmt.Sprintf(`{"amount":20,"owner":"0x615dD09124271D8008225054d85Ffe720E7a447A","salt":"%s"}`, pldtypes.RandHex(32))),
				ContractAddress: contractAddr,
			},
		})
		return err
	})
	require.NoError(t, err)
	require.Len(t, states, 2)

	// Seed pending rows for both state IDs.
	for _, s := range states {
		err = ss.p.DB(ctx).Create(&pendingPrivateStateData{
			StateID:     s.ID.String(),
			Contract:    contractAddr.String(),
			BlockNumber: 1,
		}).Error
		require.NoError(t, err)
	}

	// Now write the states again (idempotent upsert) — this must clear the pending rows.
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err = ss.WritePreVerifiedStates(ctx, dbTX, "domain1", []*components.StateUpsertOutsideContext{
			{ID: states[0].ID, SchemaID: schema.ID(), Data: states[0].Data, ContractAddress: contractAddr},
			{ID: states[1].ID, SchemaID: schema.ID(), Data: states[1].Data, ContractAddress: contractAddr},
		})
		return err
	})
	require.NoError(t, err)

	var remaining []pendingPrivateStateData
	err = ss.p.DB(ctx).Find(&remaining).Error
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

// validationDomain is a minimal domain for the state validation calls, which read only the domain
// name and whether the domain calculates its own state hashes.
func validationDomain(t *testing.T, name string, customHashFunction bool) components.Domain {
	md := componentsmocks.NewDomain(t)
	md.On("Name").Return(name).Maybe()
	md.On("CustomHashFunction").Return(customHashFunction).Maybe()
	return md
}

func TestValidateStates(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	require.Len(t, schemas, 1)
	schemaID := schemas[0].ID()
	fakeHash1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	fakeHash2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	contractAddress := *pldtypes.RandAddress()

	state1 := &prototk.EndorsableState{
		Id:            fakeHash1.String(),
		SchemaId:      schemaID.String(),
		StateDataJson: fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, pldtypes.RandHex(32)),
	}
	states, err := ss.ValidateStates(ctx, ss.p.NOTX(), validationDomain(t, "domain1", true), contractAddress,
		state1,
		&prototk.EndorsableState{
			Id:            fakeHash2.String(),
			SchemaId:      schemaID.String(),
			StateDataJson: fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, pldtypes.RandHex(32)),
		},
	)
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.NotEmpty(t, states[0].ID)
	assert.Equal(t, fakeHash2, states[1].ID)

	// Empty call is a no-op
	states, err = ss.ValidateStates(ctx, ss.p.NOTX(), validationDomain(t, "domain1", true), contractAddress)
	require.NoError(t, err)
	require.Empty(t, states)

}

func TestValidateStatesBadSchema(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddress := *pldtypes.RandAddress()
	_, err := ss.ValidateStates(ctx, ss.p.NOTX(), validationDomain(t, "domain1", false), contractAddress, &prototk.EndorsableState{
		SchemaId:      pldtypes.RandBytes32().String(),
		StateDataJson: `{}`,
	})
	assert.Regexp(t, "PD010106", err) // unknown schema

	_, err = ss.ValidateStatesWithLabels(ctx, ss.p.NOTX(), validationDomain(t, "domain1", false), contractAddress, &prototk.EndorsableState{
		SchemaId:      pldtypes.RandBytes32().String(),
		StateDataJson: `{}`,
	})
	assert.Regexp(t, "PD010106", err) // unknown schema

}

func TestValidateStatesBadData(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	require.Len(t, schemas, 1)

	contractAddress := *pldtypes.RandAddress()
	_, err = ss.ValidateStates(ctx, ss.p.NOTX(), validationDomain(t, "domain1", false), contractAddress, &prototk.EndorsableState{
		SchemaId:      schemas[0].ID().String(),
		StateDataJson: `{!!! wrong`,
	})
	assert.Regexp(t, "PD010116", err)

}

// TestValidateStatesCacheMissAndHit proves ValidateStates participates in the validated-state
// cache: a content-addressed state with a verified ID misses and re-validates on first sight
// (without seeding the cache itself), and once the caching path (ValidateStatesWithLabels) has
// stored it, re-validation is served from the cache with the label rows stripped.
func TestValidateStatesCacheMissAndHit(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	require.NoError(t, ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema1.Schema}))

	contractAddress := *pldtypes.RandAddress()
	s := makeFakeCoin(t, ctx, schema1, &contractAddress, false, 10)
	es := &prototk.EndorsableState{Id: s.ID.String(), SchemaId: schema1.ID().String(), StateDataJson: string(s.Data)}
	domain := validationDomain(t, "domain1", false)
	cacheKey := validatedStateCacheKey("domain1", contractAddress, s.ID)

	// First call: cache miss — the state re-validates from content, and ValidateStates itself
	// does not seed the cache.
	out1, err := ss.ValidateStates(ctx, ss.p.NOTX(), domain, contractAddress, es)
	require.NoError(t, err)
	require.Len(t, out1, 1)
	assert.Equal(t, s.ID, out1[0].ID)
	hits, misses := cacheCounts(ss)
	assert.Equal(t, 0, hits)
	assert.Equal(t, 1, misses)
	_, ok := peekCache(ss, cacheKey)
	assert.False(t, ok, "ValidateStates must not seed the cache")

	// Seed the cache through the labels path, then re-validate: served from the cache, with the
	// label rows nil-ed off the returned copy.
	_, err = ss.ValidateStatesWithLabels(ctx, ss.p.NOTX(), domain, contractAddress, es)
	require.NoError(t, err)
	out2, err := ss.ValidateStates(ctx, ss.p.NOTX(), domain, contractAddress, es)
	require.NoError(t, err)
	require.Len(t, out2, 1)
	assert.Equal(t, s.ID, out2[0].ID)
	assert.Nil(t, out2[0].Labels)
	assert.Nil(t, out2[0].Int64Labels)
	hits, misses = cacheCounts(ss)
	assert.Equal(t, 1, hits, "the second ValidateStates must be served from the cache")
	assert.Equal(t, 2, misses)

	// The hit returns an isolated shallow copy — stamping it must not touch the cache entry.
	cached, ok := peekCache(ss, cacheKey)
	require.True(t, ok)
	assert.NotSame(t, cached.State, out2[0])
	out2[0].Created = 12345
	cachedAfter, _ := peekCache(ss, cacheKey)
	assert.Equal(t, pldtypes.Timestamp(0), cachedAfter.Created)

}

func TestValidateStatesUnparseableSchemaID(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	contractAddress := *pldtypes.RandAddress()

	// The schema ID parse failure surfaces from both validation paths.
	_, err := ss.ValidateStates(ctx, ss.p.NOTX(), validationDomain(t, "domain1", false), contractAddress, &prototk.EndorsableState{
		SchemaId:      "not-a-schema",
		StateDataJson: `{}`,
	})
	require.Error(t, err)

	_, err = ss.ValidateStatesWithLabels(ctx, ss.p.NOTX(), validationDomain(t, "domain1", false), contractAddress, &prototk.EndorsableState{
		SchemaId:      "not-a-schema",
		StateDataJson: `{}`,
	})
	require.Error(t, err)

}

func TestValidateStatesBadStateID(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	contractAddress := *pldtypes.RandAddress()
	_, err := ss.ValidateStates(ctx, ss.p.NOTX(), validationDomain(t, "domain1", false), contractAddress, &prototk.EndorsableState{
		Id:            "not-valid-hex",
		SchemaId:      pldtypes.RandBytes32().String(),
		StateDataJson: `{}`,
	})
	require.Error(t, err)

}
