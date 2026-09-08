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

//go:build !testdbpostgres
// +build !testdbpostgres

package persistence

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateUpDown(t *testing.T) {

	ctx := context.Background()

	// Up runs as part of the init
	p, done, err := NewUnitTestPersistence(ctx, "persistence")
	require.NoError(t, err)
	assert.NotNil(t, p.DB(ctx))
	defer done()

	// Get the migration drive directly using the internal function, to run Down()
	err = p.(*provider).runMigration(ctx, func(m *migrate.Migrate) error { return m.Down() })
	require.NoError(t, err)

}
