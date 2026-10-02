/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

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
