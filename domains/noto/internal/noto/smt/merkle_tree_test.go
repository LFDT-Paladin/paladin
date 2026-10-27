/*
 * Copyright © 2026 Kaleido, Inc.
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

package smt

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/core"
	utxocore "github.com/LFDT-Paladin/smt/pkg/utxo/core"
	"github.com/stretchr/testify/assert"
)

func TestMerkleTreeName(t *testing.T) {
	address := "0xe12c416382988005ace9b2e2f9a8a904d8be961c"
	assert.Equal(t, "smt_noto_0xe12c416382988005ace9b2e2f9a8a904d8be961c", MerkleTreeName(address))
}

func TestGetHasher(t *testing.T) {
	hasher := GetHasher()
	assert.NotNil(t, hasher)
}

func TestNewMerkleTreeWithMockStorage(t *testing.T) {
	ctx := context.Background()
	mockStorage := &MockStatesStorage{
		getRootNodeRefFunc: func() (core.NodeRef, error) {
			return nil, nil // Simulate no existing root
		},
		getNewStatesFunc: func() ([]*prototk.NewConfirmedState, error) {
			return []*prototk.NewConfirmedState{}, nil // Simulate no new states
		},
	}

	mt, err := NewSmt(ctx, mockStorage, SMT_HEIGHT_UTXO)
	assert.NoError(t, err)
	assert.NotNil(t, mt, "NewSmt should return a valid Merkle tree instance")
}

// MockStatesStorage is a minimal implementation of smt.StatesStorage for testing NewSmt
// This mock provides the core.Storage interface methods needed to test the NewSmt wrapper
type MockStatesStorage struct {
	getRootNodeRefFunc func() (core.NodeRef, error)
	getNewStatesFunc   func() ([]*prototk.NewConfirmedState, error)
}

func (m *MockStatesStorage) GetRootNodeRef(ctx context.Context) (core.NodeRef, error) {
	if m.getRootNodeRefFunc != nil {
		return m.getRootNodeRefFunc()
	}
	return nil, nil
}

func (m *MockStatesStorage) UpsertRootNodeRef(ctx context.Context, ref core.NodeRef) error {
	return nil
}

func (m *MockStatesStorage) GetNode(ctx context.Context, ref core.NodeRef) (core.Node, error) {
	return nil, nil
}

func (m *MockStatesStorage) InsertNode(ctx context.Context, node core.Node) error {
	return nil
}

func (m *MockStatesStorage) BeginTx(ctx context.Context) (core.Transaction, error) {
	return nil, nil
}

func (m *MockStatesStorage) Commit(ctx context.Context) error {
	return nil
}

func (m *MockStatesStorage) Rollback(ctx context.Context) error {
	return nil
}

func (m *MockStatesStorage) Close() {
	// No-op for mock
}

func (m *MockStatesStorage) GetHasher() utxocore.Hasher {
	return nil
}

// GetNewStates implements the smt.StatesStorage interface
func (m *MockStatesStorage) GetNewStates(ctx context.Context) ([]*prototk.NewConfirmedState, error) {
	if m.getNewStatesFunc != nil {
		return m.getNewStatesFunc()
	}
	return nil, nil
}

// SetTransactionId implements the smt.StatesStorage interface
func (m *MockStatesStorage) SetTransactionId(txId string) {
	// No-op for mock
}
