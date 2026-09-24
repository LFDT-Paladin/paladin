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
	"context"
	"testing"

	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// A dispatched transaction can be in Ready_For_Dispatch before the dispatcher thread has assigned a
// signer, so the snapshot carries an empty signer. This must parse to the zero address rather than
// failing the whole snapshot with "bad address - must be 20 bytes (len=0)".
func TestCoordinatorSnapshotFromProto_emptyDispatchedSigner(t *testing.T) {
	txID := uuid.New()
	s, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		DispatchedTransactions: []*engineProto.SnapshotDispatchedTransaction{
			{Id: txID.String()},
		},
	})
	require.NoError(t, err)
	require.Len(t, s.DispatchedTransactions, 1)
	assert.Equal(t, txID, s.DispatchedTransactions[0].ID)
	assert.Equal(t, pldtypes.EthAddress{}, s.DispatchedTransactions[0].Signer)
}

func TestCoordinatorSnapshotFromProto_populatedDispatchedSigner(t *testing.T) {
	txID := uuid.New()
	signer := pldtypes.RandAddress()
	s, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		DispatchedTransactions: []*engineProto.SnapshotDispatchedTransaction{
			{Id: txID.String(), Signer: signer.String()},
		},
	})
	require.NoError(t, err)
	require.Len(t, s.DispatchedTransactions, 1)
	assert.Equal(t, *signer, s.DispatchedTransactions[0].Signer)
}

func TestCoordinatorSnapshotFromProto_nil(t *testing.T) {
	s, err := CoordinatorSnapshotFromProto(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestCoordinatorSnapshotFromProto_allTransactionTypes(t *testing.T) {
	pooledID := uuid.New()
	dispatchedID := uuid.New()
	confirmedID := uuid.New()
	revertedID := uuid.New()
	signer := pldtypes.RandAddress()
	subHash := pldtypes.RandBytes32().String()
	confHash := pldtypes.RandBytes32()
	nonce := uint64(42)

	s, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		CoordinatorState:   3,
		BlockHeight:        100,
		EndorserCandidates: []string{"node1", "node2"},
		PooledTransactions: []*engineProto.SnapshotPooledTransaction{
			{Id: pooledID.String()},
		},
		DispatchedTransactions: []*engineProto.SnapshotDispatchedTransaction{
			{Id: dispatchedID.String(), Signer: signer.String(), LatestSubmissionHash: &subHash, Nonce: &nonce},
		},
		ConfirmedTransactions: []*engineProto.SnapshotConfirmedTransaction{
			{Id: confirmedID.String(), Signer: signer.String(), Hash: confHash.String()},
		},
		RevertedTransactions: []*engineProto.SnapshotRevertedTransaction{
			{Id: revertedID.String(), RevertReason: "0xdeadbeef"},
		},
	})
	require.NoError(t, err)
	require.Len(t, s.PooledTransactions, 1)
	assert.Equal(t, pooledID, s.PooledTransactions[0].ID)
	require.Len(t, s.DispatchedTransactions, 1)
	assert.Equal(t, dispatchedID, s.DispatchedTransactions[0].ID)
	assert.Equal(t, nonce, *s.DispatchedTransactions[0].Nonce)
	require.NotNil(t, s.DispatchedTransactions[0].LatestSubmissionHash)
	require.Len(t, s.ConfirmedTransactions, 1)
	assert.Equal(t, confHash, s.ConfirmedTransactions[0].Hash)
	require.Len(t, s.RevertedTransactions, 1)
	assert.Equal(t, revertedID, s.RevertedTransactions[0].ID)
}

func TestCoordinatorSnapshotFromProto_pooledBadUUID(t *testing.T) {
	_, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		PooledTransactions: []*engineProto.SnapshotPooledTransaction{{Id: "not-a-uuid"}},
	})
	require.Error(t, err)
}

func TestCoordinatorSnapshotFromProto_dispatchedBadUUID(t *testing.T) {
	_, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		DispatchedTransactions: []*engineProto.SnapshotDispatchedTransaction{{Id: "not-a-uuid"}},
	})
	require.Error(t, err)
}

func TestCoordinatorSnapshotFromProto_dispatchedBadSigner(t *testing.T) {
	_, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		DispatchedTransactions: []*engineProto.SnapshotDispatchedTransaction{
			{Id: uuid.New().String(), Signer: "bad-address"},
		},
	})
	require.Error(t, err)
}

func TestCoordinatorSnapshotFromProto_dispatchedBadSubmissionHash(t *testing.T) {
	badHash := "not-a-hash"
	_, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		DispatchedTransactions: []*engineProto.SnapshotDispatchedTransaction{
			{Id: uuid.New().String(), LatestSubmissionHash: &badHash},
		},
	})
	require.Error(t, err)
}

func TestCoordinatorSnapshotFromProto_confirmedBadUUID(t *testing.T) {
	_, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		ConfirmedTransactions: []*engineProto.SnapshotConfirmedTransaction{{Id: "not-a-uuid"}},
	})
	require.Error(t, err)
}

func TestCoordinatorSnapshotFromProto_confirmedBadHash(t *testing.T) {
	_, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		ConfirmedTransactions: []*engineProto.SnapshotConfirmedTransaction{
			{Id: uuid.New().String(), Hash: "not-a-hash"},
		},
	})
	require.Error(t, err)
}

func TestCoordinatorSnapshotFromProto_revertedBadUUID(t *testing.T) {
	_, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		RevertedTransactions: []*engineProto.SnapshotRevertedTransaction{{Id: "not-a-uuid"}},
	})
	require.Error(t, err)
}

func TestCoordinatorSnapshotFromProto_revertedBadReason(t *testing.T) {
	_, err := CoordinatorSnapshotFromProto(context.Background(), &engineProto.CoordinatorSnapshot{
		RevertedTransactions: []*engineProto.SnapshotRevertedTransaction{
			{Id: uuid.New().String(), RevertReason: "not-hex"},
		},
	})
	require.Error(t, err)
}
