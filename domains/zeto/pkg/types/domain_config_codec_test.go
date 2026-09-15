/*
 * Copyright © 2024 Kaleido, Inc.
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

package types

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCircuits(t *testing.T) *zetosignerapi.Circuits {
	t.Helper()
	c := zetosignerapi.Circuits{
		"deposit":         {Name: "d", Type: "deposit"},
		"withdraw":        {Name: "w", Type: "withdraw"},
		"transfer":        {Name: "tr", Type: "transfer"},
		"transferLocked":  {Name: "tl", Type: "transferLocked"},
	}
	return &c
}

func TestEncodeDecodeDomainInstanceConfigV0(t *testing.T) {
	ctx := context.Background()
	cfg := &DomainInstanceConfig{
		TokenName: "anon",
		Circuits:  testCircuits(t),
	}
	encoded, err := EncodeDomainInstanceConfigV0(ctx, cfg)
	require.NoError(t, err)

	decoded, err := DecodeDomainInstanceConfig(ctx, encoded)
	require.NoError(t, err)
	assert.Equal(t, DomainConfigSchemaV0, decoded.ConfigSchema)
	assert.Equal(t, "anon", decoded.TokenName)
	assert.Equal(t, pldtypes.HexUint64(0), decoded.ZetoVariant)
	require.NotNil(t, decoded.Circuits)
	assert.Equal(t, "d", (*decoded.Circuits)["deposit"].Name)
}

func TestEncodeDecodeDomainInstanceConfigV1(t *testing.T) {
	ctx := context.Background()
	cfg := &DomainInstanceConfig{
		TokenName:       "anon_nullifier",
		Circuits:        testCircuits(t),
		ZetoVariant:     7,
		FactoryVersion:  2,
		CircuitBundleId: "bundle-a",
	}
	encoded, err := EncodeDomainInstanceConfigV1(ctx, cfg)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(encoded), len(ZetoDomainConfigID_V1))
	assert.Equal(t, []byte(ZetoDomainConfigID_V1), []byte(encoded[:len(ZetoDomainConfigID_V1)]))

	decoded, err := DecodeDomainInstanceConfig(ctx, encoded)
	require.NoError(t, err)
	assert.Equal(t, DomainConfigSchemaV1, decoded.ConfigSchema)
	assert.Equal(t, "anon_nullifier", decoded.TokenName)
	assert.Equal(t, pldtypes.HexUint64(7), decoded.ZetoVariant)
	assert.Equal(t, int64(2), decoded.FactoryVersion)
	assert.Equal(t, "bundle-a", decoded.CircuitBundleId)
	require.NotNil(t, decoded.Circuits)
	assert.Equal(t, "tr", (*decoded.Circuits)["transfer"].Name)
}

func TestGetCircuitsForDeploy_BundleID(t *testing.T) {
	ctx := context.Background()
	d := &DomainFactoryConfig{
		DomainContracts: DomainConfigContracts{
			Implementations: []*DomainContract{
				{Name: "anon", BundleID: "b1", Circuits: testCircuits(t)},
				{Name: "other", BundleID: "b2", Circuits: testCircuits(t)},
			},
		},
	}
	got, err := d.GetCircuitsForDeploy(ctx, "ignored", 0, "b2")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "tr", (*got)["transfer"].Name)
}

func TestGetCircuitsForDeploy_VariantFilter(t *testing.T) {
	ctx := context.Background()
	v1 := uint64(1)
	d := &DomainFactoryConfig{
		DomainContracts: DomainConfigContracts{
			Implementations: []*DomainContract{
				{Name: "anon", ForZetoVariant: &v1, Circuits: testCircuits(t)},
			},
		},
	}
	got, err := d.GetCircuitsForDeploy(ctx, "anon", 1, "")
	require.NoError(t, err)
	require.NotNil(t, got)

	_, err = d.GetCircuitsForDeploy(ctx, "anon", 99, "")
	require.Error(t, err)
}

func TestGetCircuitsForDeploy_DuplicateBundleID(t *testing.T) {
	ctx := context.Background()
	d := &DomainFactoryConfig{
		DomainContracts: DomainConfigContracts{
			Implementations: []*DomainContract{
				{Name: "a", BundleID: "dup", Circuits: testCircuits(t)},
				{Name: "b", BundleID: "dup", Circuits: testCircuits(t)},
			},
		},
	}
	_, err := d.GetCircuitsForDeploy(ctx, "", 0, "dup")
	require.Error(t, err)
}
