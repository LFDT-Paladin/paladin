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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// qualifierSQL returns the WHERE clause whereClauseForQual builds for a qualifier, as SQL.
func qualifierSQL(t *testing.T, q pldapi.StateStatusQualifier, spentColumn string) string {
	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	ctx := context.Background()
	return p.P.DB(ctx).ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := tx.Table("states").Where(whereClauseForQual(p.P.DB(ctx), q, spentColumn)).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
}

func TestWhereClauseForQual(t *testing.T) {
	// Available requires a confirm record and no spend record
	require.Equal(t,
		`SELECT count(*) FROM "states" WHERE "Spent"."transaction" IS NULL AND "Confirmed"."transaction" IS NOT NULL`,
		qualifierSQL(t, pldapi.StateStatusAvailable, "Spent"))

	// Confirmed is a synonym of available, selecting via the identical clause
	require.Equal(t,
		qualifierSQL(t, pldapi.StateStatusAvailable, "Spent"),
		qualifierSQL(t, pldapi.StateStatusConfirmed, "Spent"))

	// Unconfirmed requires no confirm record, and says nothing about spending
	require.Equal(t,
		`SELECT count(*) FROM "states" WHERE "Confirmed"."transaction" IS NULL`,
		qualifierSQL(t, pldapi.StateStatusUnconfirmed, "Spent"))

	// Spent requires a spend record, and says nothing about confirmation
	require.Equal(t,
		`SELECT count(*) FROM "states" WHERE "Spent"."transaction" IS NOT NULL`,
		qualifierSQL(t, pldapi.StateStatusSpent, "Spent"))

	// All scopes nothing
	require.Equal(t,
		`SELECT count(*) FROM "states" WHERE TRUE`,
		qualifierSQL(t, pldapi.StateStatusAll, "Spent"))

	// The spend column is a parameter, so nullifier queries scope on the nullifier's spend record
	require.Equal(t,
		`SELECT count(*) FROM "states" WHERE "Nullifier__Spent"."transaction" IS NULL AND "Confirmed"."transaction" IS NOT NULL`,
		qualifierSQL(t, pldapi.StateStatusAvailable, "Nullifier__Spent"))

	// An empty qualifier means all, matching the default the reader applies before calling here
	require.Equal(t,
		`SELECT count(*) FROM "states" WHERE TRUE`,
		qualifierSQL(t, pldapi.StateStatusQualifier(""), "Spent"))
}

// TestFindStatesUnsetQualifier proves the reader treats an unset qualifier as all, scoping the
// query on TRUE rather than on any confirm/spend predicate.
func TestFindStatesUnsetQualifier(t *testing.T) {
	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	mockGetSchemaOK(mdb)
	mdb.ExpectQuery(`SELECT.*FROM "states".*WHERE.*TRUE`).WillReturnError(fmt.Errorf("called"))

	_, err := ss.FindStates(ctx, ss.p.NOTX(), "domain1", pldtypes.RandBytes32(),
		query.NewQueryBuilder().Query(), &components.StateQueryOptions{})
	assert.Regexp(t, "called", err)
}
