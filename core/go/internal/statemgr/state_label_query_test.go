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
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gadgetABI gives an int64 indexed label (size), which widgetABI does not have, so the
// state_int64_labels side of label-driven queries gets covered.
const gadgetABI = `{
	"type": "tuple",
	"internalType": "struct Gadget",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "owner",
			"type": "string",
			"indexed": true
		},
		{
			"name": "size",
			"type": "int64",
			"indexed": true
		}
	]
}`

// TestLabelDrivenQueryParity checks a label-driven available-state query returns exactly what the
// states-driven query returns, over a corpus covering both label tables, confirmed/spent/unconfirmed
// states, every operator alongside the driving equality, .created and .id predicates, limits, sorts
// and exclusion lists, and that queries with nothing to drive them stay states-driven.
func TestLabelDrivenQueryParity(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)
	mockStateCallback(m)

	widgetSchema, err := newABISchema(ctx, "domain1", testABIParam(t, widgetABI))
	require.NoError(t, err)
	gadgetSchema, err := newABISchema(ctx, "domain1", testABIParam(t, gadgetABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{widgetSchema.Schema, gadgetSchema.Schema})
	require.NoError(t, err)

	contractAddress := pldtypes.RandAddress()

	confirm := func(states ...*pldapi.State) {
		for _, s := range states {
			err := ss.WriteStateFinalizations(ctx, ss.p.NOTX(), nil, nil,
				[]*pldapi.StateConfirmRecord{{DomainName: "domain1", State: s.ID, Transaction: uuid.New()}}, nil)
			require.NoError(t, err)
		}
	}
	spend := func(states ...*pldapi.State) {
		for _, s := range states {
			err := ss.WriteStateFinalizations(ctx, ss.p.NOTX(),
				[]*pldapi.StateSpendRecord{{DomainName: "domain1", State: s.ID, Transaction: uuid.New()}}, nil, nil, nil)
			require.NoError(t, err)
		}
	}

	// makeWidgets writes each state in its own transaction, so created timestamps are distinct
	// and both query shapes order deterministically.
	widgets := makeWidgets(t, ctx, ss, "domain1", contractAddress, widgetSchema.ID(), []string{
		`{"size": 1, "color": "red",  "price": 100}`,
		`{"size": 2, "color": "red",  "price": 150}`,
		`{"size": 3, "color": "blue", "price": 199}`,
		`{"size": 4, "color": "red",  "price": 300}`,
		`{"size": 5, "color": "pink", "price": 500}`,
		`{"size": 6, "color": "red",  "price": 100}`,
		`{"size": 7, "color": "blue", "price": 150}`,
		`{"size": 8, "color": "red",  "price": 199}`,
		`{"size": 9, "color": "blue", "price": 300}`,
		`{"size": 10, "color": "red", "price": 500}`,
	})
	confirm(widgets[:9]...) // widgets[9] stays unconfirmed
	spend(widgets[1], widgets[6])
	// available reds in created order: 0, 3, 5, 7 — 1 is spent, 9 unconfirmed

	gadgets := makeWidgets(t, ctx, ss, "domain1", contractAddress, gadgetSchema.ID(), []string{
		`{"owner": "alice", "size": 10}`,
		`{"owner": "alice", "size": 20}`,
		`{"owner": "bob",   "size": 10}`,
		`{"owner": "alice", "size": 10}`,
	})
	confirm(gadgets...)
	spend(gadgets[3])
	// available size=10 gadgets in created order: 0, 2

	checkLabelRowsMatchStates(t, ctx, ss)

	idsOf := func(states []*pldapi.State) []string {
		ids := make([]string, len(states))
		for i, s := range states {
			ids[i] = s.ID.String()
		}
		return ids
	}

	// reference runs the states-driven query with the availability scope, the path a label-driven
	// query replaces.
	reference := func(contractAddress *pldtypes.EthAddress, schemaID pldtypes.Bytes32, jq *query.QueryJSON, excludedIDs []pldtypes.HexBytes) []*pldapi.State {
		scope := statusScope(ctx, ss.p.NOTX(), pldapi.StateStatusConfirmed, excludedIDs)
		scope.availableOnly = false
		_, states, err := ss.findStatesCommon(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, jq, scope)
		require.NoError(t, err)
		return states
	}

	driverFor := func(schemaID pldtypes.Bytes32, jq *query.QueryJSON) *schemaLabelInfo {
		schema, err := ss.getSchemaByID(ctx, ss.p.NOTX(), "domain1", schemaID, true)
		require.NoError(t, err)
		return labelIndexDriver(jq, ss.labelSetFor(schema).labels)
	}

	// checkDriven asserts the query is driven from the expected label and returns exactly the
	// expected states, in order, matching the states-driven reference.
	checkDriven := func(name string, schemaID pldtypes.Bytes32, jq *query.QueryJSON, excludedIDs []pldtypes.HexBytes, driverLabel string, expected ...*pldapi.State) {
		t.Run(name, func(t *testing.T) {
			driver := driverFor(schemaID, jq)
			require.NotNil(t, driver, "expected the query to be label-driven")
			assert.Equal(t, driverLabel, driver.label)

			_, states, err := ss.findStates(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, jq, pldapi.StateStatusConfirmed, excludedIDs)
			require.NoError(t, err)
			assert.Equal(t, idsOf(expected), idsOf(states))
			assert.Equal(t, idsOf(reference(contractAddress, schemaID, jq, excludedIDs)), idsOf(states))
		})
	}

	// checkStatesDriven asserts the query has no label equality to drive it, and is answered
	// identically to the reference.
	checkStatesDriven := func(name string, schemaID pldtypes.Bytes32, jq *query.QueryJSON) {
		t.Run(name, func(t *testing.T) {
			require.Nil(t, driverFor(schemaID, jq), "expected the query to be states-driven")

			_, states, err := ss.findStates(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, jq, pldapi.StateStatusConfirmed, nil)
			require.NoError(t, err)
			assert.Equal(t, idsOf(reference(contractAddress, schemaID, jq, nil)), idsOf(states))
		})
	}

	widgetID := widgetSchema.ID()
	gadgetID := gadgetSchema.ID()
	eq := func(field, value string, mods ...func(*query.OpSingleVal)) *query.OpSingleVal {
		op := &query.OpSingleVal{Op: query.Op{Field: field}, Value: pldtypes.RawJSON(value)}
		for _, m := range mods {
			m(op)
		}
		return op
	}
	not := func(op *query.OpSingleVal) { op.Not = true }
	caseInsensitive := func(op *query.OpSingleVal) { op.CaseInsensitive = true }
	created := func(s *pldapi.State) int64 { return int64(s.Created) }
	createdJSON := func(s *pldapi.State) string { return fmt.Sprintf("%d", s.Created) }

	// Single equality
	checkDriven("string label eq", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Query(), nil, "color",
		widgets[0], widgets[3], widgets[5], widgets[7])
	checkDriven("limit binds", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Limit(2).Query(), nil, "color",
		widgets[0], widgets[3])
	checkDriven("descending prefix", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Sort("-.created").Query(), nil, "color",
		widgets[7], widgets[5], widgets[3], widgets[0])
	checkDriven("descending suffix", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Sort(".created DESC").Limit(3).Query(), nil, "color",
		widgets[7], widgets[5], widgets[3])
	checkDriven("explicit ascending", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Sort(".created asc").Query(), nil, "color",
		widgets[0], widgets[3], widgets[5], widgets[7])
	checkDriven("exclusions", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Query(),
		[]pldtypes.HexBytes{widgets[3].ID, widgets[8].ID}, "color",
		widgets[0], widgets[5], widgets[7])
	checkDriven("uint256 label eq", widgetID,
		query.NewQueryBuilder().Equal("price", 199).Query(), nil, "price",
		widgets[2], widgets[7])
	checkDriven("int64 label eq", gadgetID,
		query.NewQueryBuilder().Equal("size", 10).Query(), nil, "size",
		gadgets[0], gadgets[2])
	checkDriven("no matches", widgetID,
		query.NewQueryBuilder().Equal("color", "chartreuse").Query(), nil, "color")
	checkDriven("long-form equal", widgetID, &query.QueryJSON{
		Statements: query.Statements{Ops: query.Ops{Equal: []*query.OpSingleVal{eq("color", `"red"`)}}},
	}, nil, "color",
		widgets[0], widgets[3], widgets[5], widgets[7])

	// Choosing the driver
	checkDriven("first equality drives", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Equal("price", 100).Query(), nil, "color",
		widgets[0], widgets[5])
	checkDriven("int64 equality drives a string further equality", gadgetID,
		query.NewQueryBuilder().Equal("size", 10).Equal("owner", "alice").Query(), nil, "size",
		gadgets[0])
	checkDriven("string equality drives an int64 further equality", gadgetID,
		query.NewQueryBuilder().Equal("owner", "alice").Equal("size", 10).Query(), nil, "owner",
		gadgets[0])
	checkDriven("equal before eq", widgetID, &query.QueryJSON{
		Statements: query.Statements{Ops: query.Ops{
			Equal: []*query.OpSingleVal{eq("price", `199`)},
			Eq:    []*query.OpSingleVal{eq("color", `"red"`)},
		}},
	}, nil, "price",
		widgets[7])
	checkDriven("base field equality is passed over", widgetID,
		query.NewQueryBuilder().Equal(".id", widgets[3].ID.String()).Equal("color", "red").Query(), nil, "color",
		widgets[3])
	checkDriven("negated equality is passed over", widgetID, &query.QueryJSON{
		Statements: query.Statements{Ops: query.Ops{Eq: []*query.OpSingleVal{eq("color", `"red"`, not), eq("price", `199`)}}},
	}, nil, "price",
		widgets[2])
	checkDriven("case-insensitive equality is passed over", widgetID, &query.QueryJSON{
		Statements: query.Statements{Ops: query.Ops{Eq: []*query.OpSingleVal{eq("color", `"RED"`, caseInsensitive), eq("price", `199`)}}},
	}, nil, "price",
		widgets[7])
	checkDriven("same label twice with different values", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Equal("color", "blue").Query(), nil, "color")

	// Other operators alongside the driving equality
	checkDriven("label range", widgetID,
		query.NewQueryBuilder().Equal("color", "red").GreaterThan("price", 100).Query(), nil, "color",
		widgets[3], widgets[7])
	checkDriven("label negated equality", widgetID,
		query.NewQueryBuilder().Equal("color", "red").NotEqual("price", 100).Query(), nil, "color",
		widgets[3], widgets[7])
	checkDriven("label in", widgetID,
		query.NewQueryBuilder().Equal("color", "red").In("price", []any{100, 199}).Query(), nil, "color",
		widgets[0], widgets[5], widgets[7])
	checkDriven("label like", gadgetID,
		query.NewQueryBuilder().Equal("size", 10).Like("owner", "ali%").Query(), nil, "size",
		gadgets[0])
	checkDriven("label case-insensitive equality", gadgetID, &query.QueryJSON{
		Statements: query.Statements{Ops: query.Ops{Eq: []*query.OpSingleVal{eq("size", `10`), eq("owner", `"ALICE"`, caseInsensitive)}}},
	}, nil, "size",
		gadgets[0])
	checkDriven("or branches beside a top-level equality", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Or(
			query.NewQueryBuilder().Equal("price", 100),
			query.NewQueryBuilder().Equal("price", 300),
		).Query(), nil, "color",
		widgets[0], widgets[3], widgets[5])
	checkDriven("same label in the driver and a further predicate", widgetID,
		query.NewQueryBuilder().Equal("color", "red").NotEqual("color", "blue").Query(), nil, "color",
		widgets[0], widgets[3], widgets[5], widgets[7])

	// The state's own fields, served from the driving row
	checkDriven("id equality", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Equal(".id", widgets[5].ID.String()).Query(), nil, "color",
		widgets[5])
	checkDriven("contract address equality", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Equal("contractAddress", contractAddress.String()).Query(), nil, "color",
		widgets[0], widgets[3], widgets[5], widgets[7])
	checkDriven("created lower bound", widgetID,
		query.NewQueryBuilder().Equal("color", "red").GreaterThan(".created", created(widgets[3])).Query(), nil, "color",
		widgets[5], widgets[7])
	checkDriven("created inclusive lower bound", widgetID,
		query.NewQueryBuilder().Equal("color", "red").GreaterThanOrEqual(".created", created(widgets[3])).Query(), nil, "color",
		widgets[3], widgets[5], widgets[7])
	checkDriven("created upper bound", widgetID,
		query.NewQueryBuilder().Equal("color", "red").LessThan(".created", created(widgets[5])).Query(), nil, "color",
		widgets[0], widgets[3])
	checkDriven("created window descending", widgetID,
		query.NewQueryBuilder().Equal("color", "red").
			GreaterThan(".created", created(widgets[0])).
			LessThanOrEqual(".created", created(widgets[7])).
			Sort("-.created").Limit(2).Query(), nil, "color",
		widgets[7], widgets[5])
	checkDriven("created bounds in long form", widgetID, &query.QueryJSON{
		Statements: query.Statements{Ops: query.Ops{
			Eq:                 []*query.OpSingleVal{eq("color", `"red"`)},
			GreaterThanOrEqual: []*query.OpSingleVal{eq(".created", createdJSON(widgets[3]))},
			LessThan:           []*query.OpSingleVal{eq(".created", createdJSON(widgets[7]))},
		}},
	}, nil, "color",
		widgets[3], widgets[5])
	// The shape of a later page of coin selection: owner equality, resuming after the last
	// created timestamp seen, with exclusions.
	checkDriven("paged with exclusions", gadgetID,
		query.NewQueryBuilder().Equal("owner", "alice").GreaterThan(".created", created(gadgets[0])).Sort(".created").Limit(10).Query(),
		[]pldtypes.HexBytes{gadgets[0].ID}, "owner",
		gadgets[1])

	// Sorts
	redsByID := []*pldapi.State{widgets[0], widgets[3], widgets[5], widgets[7]}
	slices.SortFunc(redsByID, func(a, b *pldapi.State) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	checkDriven("sort on id", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Sort(".id").Query(), nil, "color",
		redsByID...)
	checkDriven("sort on created then id", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Sort(".created", ".id").Query(), nil, "color",
		widgets[0], widgets[3], widgets[5], widgets[7])
	checkDriven("sort on a label then created", widgetID,
		query.NewQueryBuilder().Equal("color", "red").Sort("price", ".created").Query(), nil, "color",
		widgets[0], widgets[5], widgets[7], widgets[3])

	// Nothing to drive from
	checkStatesDriven("negated equality only", widgetID,
		query.NewQueryBuilder().NotEqual("color", "red").Query())
	checkStatesDriven("in only", widgetID,
		query.NewQueryBuilder().In("color", []any{"red", "blue"}).Query())
	checkStatesDriven("or branches only", widgetID,
		query.NewQueryBuilder().Or(
			query.NewQueryBuilder().Equal("color", "red"),
			query.NewQueryBuilder().Equal("color", "blue"),
		).Query())
	checkStatesDriven("case-insensitive equality only", widgetID, &query.QueryJSON{
		Statements: query.Statements{Ops: query.Ops{Eq: []*query.OpSingleVal{eq("color", `"RED"`, caseInsensitive)}}},
	})
	checkStatesDriven("id equality only", widgetID,
		query.NewQueryBuilder().Equal(".id", widgets[0].ID.String()).Query())
	checkStatesDriven("created bound only", widgetID,
		query.NewQueryBuilder().GreaterThan(".created", created(widgets[3])).Query())
	checkStatesDriven("no predicate at all", widgetID,
		query.NewQueryBuilder().Query())

	t.Run("not scoped to a contract", func(t *testing.T) {
		jq := query.NewQueryBuilder().Equal("color", "red").Query()
		_, states, err := ss.findStates(ctx, ss.p.NOTX(), "domain1", nil, widgetID, jq, pldapi.StateStatusConfirmed, nil)
		require.NoError(t, err)
		assert.Equal(t, idsOf(reference(nil, widgetID, jq, nil)), idsOf(states))
		assert.Equal(t, idsOf([]*pldapi.State{widgets[0], widgets[3], widgets[5], widgets[7]}), idsOf(states))
	})

	for _, tc := range []struct {
		name string
		jq   *query.QueryJSON
	}{
		{"unresolvable value", query.NewQueryBuilder().Equal("price", "not-a-number").Query()},
		{"unresolvable further value", query.NewQueryBuilder().Equal("color", "red").Equal("price", "not-a-number").Query()},
		{"unresolvable created bound", query.NewQueryBuilder().Equal("color", "red").GreaterThan(".created", "not-a-time").Query()},
		{"negated created bound", &query.QueryJSON{
			Statements: query.Statements{Ops: query.Ops{
				Eq: []*query.OpSingleVal{eq("color", `"red"`)},
				GT: []*query.OpSingleVal{eq(".created", createdJSON(widgets[3]), not)},
			}},
		}},
	} {
		t.Run(tc.name+" is reported", func(t *testing.T) {
			_, _, err := ss.findStates(ctx, ss.p.NOTX(), "domain1", contractAddress, widgetID, tc.jq, pldapi.StateStatusConfirmed, nil)
			require.Error(t, err)
		})
	}

	// A non-availability qualifier is never label-driven: all reds regardless of status.
	t.Run("status all is states-driven", func(t *testing.T) {
		jq := query.NewQueryBuilder().Equal("color", "red").Query()
		_, states, err := ss.findStates(ctx, ss.p.NOTX(), "domain1", contractAddress, widgetID, jq, pldapi.StateStatusAll, nil)
		require.NoError(t, err)
		assert.Equal(t, idsOf([]*pldapi.State{widgets[0], widgets[1], widgets[3], widgets[5], widgets[7], widgets[9]}), idsOf(states))
	})
}

func TestLabelRowFieldSetCoversBaseStateFields(t *testing.T) {
	fs := labelRowFieldSet{trackingLabelSet: &trackingLabelSet{labels: map[string]*schemaLabelInfo{}, used: map[string]*schemaLabelInfo{}}, row: "l0"}
	for fieldName := range baseStateFields {
		resolver := fs.ResolverFor(fieldName)
		require.NotNil(t, resolver, fieldName)
		assert.True(t, strings.HasPrefix(resolver.SQLColumn(), `l0.`), "%s resolves to %s", fieldName, resolver.SQLColumn())
	}
}

func TestLabelIndexFastPathQueryError(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, widgetABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	db.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("pop"))

	_, _, err = ss.findStates(ctx, ss.p.NOTX(), "domain1", pldtypes.RandAddress(), schema.ID(),
		query.NewQueryBuilder().Equal("color", "red").Query(), pldapi.StateStatusConfirmed, nil)
	assert.Regexp(t, "pop", err)
}
