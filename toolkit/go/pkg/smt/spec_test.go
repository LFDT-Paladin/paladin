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

package smt

import (
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/smt/pkg/utxo"
	"github.com/stretchr/testify/assert"
)

func TestNewMerkleTreeSpec(t *testing.T) {
	stateQueryContext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()

	// Test successful creation
	callbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{}, nil
		},
	}

	spec, err := NewMerkleTreeSpec(
		t.Context(),
		"test-tree",
		StatesTree,
		64,
		hasher,
		true,
		callbacks,
		"root-schema",
		"node-schema",
		stateQueryContext,
	)
	assert.NoError(t, err)
	assert.NotNil(t, spec)
	assert.Equal(t, "test-tree", spec.Name)
	assert.Equal(t, 64, spec.Levels)
	assert.Equal(t, StatesTree, spec.Type)
	assert.NotNil(t, spec.Storage)
	assert.NotNil(t, spec.Tree)
}

func TestNewMerkleTreeSpecWithError(t *testing.T) {
	stateQueryContext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()

	// Test error handling when callbacks fail
	callbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("callback error")
		},
	}

	spec, err := NewMerkleTreeSpec(
		t.Context(),
		"test-tree",
		LockedStatesTree,
		32,
		hasher,
		true,
		callbacks,
		"root-schema",
		"node-schema",
		stateQueryContext,
	)
	assert.Error(t, err)
	assert.Nil(t, spec)
}

func TestMerkleTreeTypeConstants(t *testing.T) {
	assert.Equal(t, 0, int(StatesTree))
	assert.Equal(t, 1, int(LockedStatesTree))
	assert.Equal(t, 2, int(KycStatesTree))
}
