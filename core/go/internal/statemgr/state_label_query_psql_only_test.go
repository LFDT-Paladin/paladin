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

//go:build testdbpostgres
// +build testdbpostgres

package statemgr

import (
	"context"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestLabelDrivenQueryExplain asserts the plan shape a label-driven query exists to produce: the
// query is served by the driving label's availability partial index with the scan bounded to one
// (label, value) range, narrowed by any .created bounds, and the index provides the .created order
// so no Sort node appears. Sequential and bitmap scans are disabled for the check because the
// planner would not pick an index over a seq scan at unit test row counts; what is being verified
// is that the query shape is index-serviceable, not the planner's costing at this size.
func TestLabelDrivenQueryExplain(t *testing.T) {
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
	makeWidgets(t, ctx, ss, "domain1", contractAddress, widgetSchema.ID(), []string{
		`{"size": 1, "color": "red", "price": 100}`,
	})
	makeWidgets(t, ctx, ss, "domain1", contractAddress, gadgetSchema.ID(), []string{
		`{"owner": "alice", "size": 10}`,
	})

	explain := func(schemaID pldtypes.Bytes32, jq *query.QueryJSON, excludedIDs []pldtypes.HexBytes) string {
		schema, err := ss.getSchemaByID(ctx, ss.p.NOTX(), "domain1", schemaID, true)
		require.NoError(t, err)
		driver := labelIndexDriver(jq, ss.labelSetFor(schema).labels)
		require.NotNil(t, driver, "expected the query to be label-driven")

		sqlStr := ss.p.NOTX().DB(ctx).ToSQL(func(tx *gorm.DB) *gorm.DB {
			return ss.buildStatesQuery(ctx, tx, "domain1", contractAddress, schema, jq, driver,
				statusScope(ctx, ss.p.NOTX(), pldapi.StateStatusConfirmed, excludedIDs).apply).
				Find(&[]*persistedState{})
		})

		var planLines []string
		err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			gdb := dbTX.DB(ctx)
			if err := gdb.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
				return err
			}
			if err := gdb.Exec("SET LOCAL enable_bitmapscan = off").Error; err != nil {
				return err
			}
			rows, err := gdb.Raw("EXPLAIN " + sqlStr).Rows()
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					return err
				}
				planLines = append(planLines, line)
			}
			return rows.Err()
		})
		require.NoError(t, err)
		plan := strings.Join(planLines, "\n")
		t.Logf("plan:\n%s", plan)
		return plan
	}

	t.Run("string label", func(t *testing.T) {
		plan := explain(widgetSchema.ID(), query.NewQueryBuilder().Sort(".created").Equal("color", "red").Limit(10).Query(), nil)
		assert.Regexp(t, `Index Scan using state_labels_available`, plan)
		assert.Regexp(t, `Index Cond:.*value.*= 'red'`, plan)
		assert.NotContains(t, plan, "Sort")
	})

	t.Run("string label descending", func(t *testing.T) {
		plan := explain(widgetSchema.ID(), query.NewQueryBuilder().Sort("-.created").Equal("color", "red").Limit(10).Query(), nil)
		assert.Regexp(t, `Index Scan Backward using state_labels_available`, plan)
		assert.NotContains(t, plan, "Sort")
	})

	t.Run("int64 label", func(t *testing.T) {
		plan := explain(gadgetSchema.ID(), query.NewQueryBuilder().Sort(".created").Equal("size", 10).Limit(10).Query(), nil)
		assert.Regexp(t, `Index Scan using state_int64_labels_available`, plan)
		assert.Regexp(t, `Index Cond:.*value.*= 10`, plan)
		assert.NotContains(t, plan, "Sort")
	})

	t.Run("first equality drives, further predicates checked per row", func(t *testing.T) {
		plan := explain(widgetSchema.ID(), query.NewQueryBuilder().Sort(".created").Equal("color", "red").GreaterThan("price", 100).Limit(10).Query(), nil)
		assert.Regexp(t, `Index Scan using state_labels_available on state_labels l\d`, plan)
		assert.Regexp(t, `Index Cond:.*value.*= 'red'`, plan)
		assert.Regexp(t, `Nested Loop`, plan)
		assert.NotContains(t, plan, "Sort")
	})

	t.Run("int64 equality drives a string further equality", func(t *testing.T) {
		plan := explain(gadgetSchema.ID(), query.NewQueryBuilder().Sort(".created").Equal("size", 10).Equal("owner", "alice").Limit(10).Query(), nil)
		assert.Regexp(t, `Index Scan using state_int64_labels_available on state_int64_labels l\d`, plan)
		assert.Regexp(t, `Index Cond:.*value.*= 10`, plan)
		assert.NotContains(t, plan, "Sort")
	})

	t.Run("created bound narrows the driving range", func(t *testing.T) {
		plan := explain(widgetSchema.ID(), query.NewQueryBuilder().Sort(".created").Equal("color", "red").GreaterThan(".created", 1000).Limit(10).Query(), nil)
		assert.Regexp(t, `Index Scan using state_labels_available`, plan)
		assert.Regexp(t, `Index Cond:.*value.*= 'red'.*created > `, plan)
		assert.NotContains(t, plan, "Sort")
	})

	t.Run("exclusions stay off the driving scan", func(t *testing.T) {
		excluded := []pldtypes.HexBytes{pldtypes.RandBytes(32), pldtypes.RandBytes(32)}
		plan := explain(widgetSchema.ID(), query.NewQueryBuilder().Sort(".created").Equal("color", "red").Limit(10).Query(), excluded)
		assert.Regexp(t, `Index Scan using state_labels_available`, plan)
		assert.NotContains(t, plan, "Sort")
		assert.Regexp(t, `Filter: \(id <> ALL \('\{`, plan)
	})
}
