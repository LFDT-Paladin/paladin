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
package common

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestTransaction_GetID_newRandomUUID(t *testing.T) {
	id := uuid.New()
	tx := &SnapshotPooledTransaction{
		ID: id,
	}
	result := tx.GetID()

	// For random UUIDs, verify it's a valid UUID string format
	parsed, err := uuid.Parse(result)
	assert.NoError(t, err)
	assert.Equal(t, id, parsed)
}

func TestTransaction_GetID_specificUUID(t *testing.T) {
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	tx := &SnapshotPooledTransaction{
		ID: id,
	}
	result := tx.GetID()
	assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000", result)
}

func TestTransaction_GetID_nilUUID(t *testing.T) {
	tx := &SnapshotPooledTransaction{
		ID: uuid.Nil,
	}
	result := tx.GetID()
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", result)
}
