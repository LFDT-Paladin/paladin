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

package fungible

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMerkleNullifierCallbacks_CustomOwner(t *testing.T) {
	altOwner := "0x8cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"
	cb := newMerkleNullifierCallbacks("coin", altOwner)
	require.NotNil(t, cb.MockFindAvailableStates)
	resp, err := cb.MockFindAvailableStates(context.Background(), &prototk.FindAvailableStatesRequest{SchemaId: "coin"})
	require.NoError(t, err)
	require.NotEmpty(t, resp.States)
	assert.Contains(t, resp.States[0].DataJson, altOwner)
}

func TestNewMerkleNullifierKycCallbacks_ReturnsFindStates(t *testing.T) {
	cb := newMerkleNullifierKycCallbacks("coin")
	require.NotNil(t, cb.MockFindAvailableStates)
}

func TestMerkleNullifierKycFindAvailableStates_CyclesAllBranches(t *testing.T) {
	fn := merkleNullifierKycFindAvailableStates("coin", []*prototk.StoredState{{
		Id: "coin-in-merkle-1", DataJson: merkleUTXOCoinJSON,
	}})
	ctx := context.Background()
	resp, err := fn(ctx, &prototk.FindAvailableStatesRequest{SchemaId: "coin"})
	require.NoError(t, err)
	require.Len(t, resp.States, 1)
	for i := 0; i < 12; i++ {
		_, err := fn(ctx, &prototk.FindAvailableStatesRequest{SchemaId: "merkle_tree_root"})
		require.NoError(t, err)
	}
}
