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

package common

import (
	"context"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTransactionIDBytes32(t *testing.T) {
	ctx := context.Background()

	_, err := ParseTransactionIDBytes32(ctx, "")
	require.Error(t, err)

	hexID := "0x" + strings.Repeat("aa", 32)
	got, err := ParseTransactionIDBytes32(ctx, hexID)
	require.NoError(t, err)
	assert.Equal(t, pldtypes.MustParseBytes32(hexID), got)

	u := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	got, err = ParseTransactionIDBytes32(ctx, u.String())
	require.NoError(t, err)
	assert.False(t, got.IsZero())

	_, err = ParseTransactionIDBytes32(ctx, "not-a-valid-id")
	require.Error(t, err)
}
