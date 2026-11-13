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

package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZetoVariantArtifactDirName(t *testing.T) {
	assert.Equal(t, "V0", ZetoVariant_V0.ArtifactDirName())
	assert.Equal(t, "V1", ZetoVariant_V1.ArtifactDirName())

	// Directory names must be unique, or two variants would share a proving-artifact tree.
	seen := map[string]bool{}
	for _, v := range SupportedZetoVariants {
		require.False(t, seen[v.ArtifactDirName()], "duplicate artifact dir %s", v.ArtifactDirName())
		seen[v.ArtifactDirName()] = true
	}
}

func TestValidateZetoVariant(t *testing.T) {
	ctx := t.Context()
	for _, v := range SupportedZetoVariants {
		require.NoError(t, ValidateZetoVariant(ctx, v))
	}
	err := ValidateZetoVariant(ctx, ZetoVariant(99))
	require.Error(t, err)
	assert.Regexp(t, "PD210143", err)
}

func TestParseZetoVariantKey(t *testing.T) {
	ctx := t.Context()
	v, err := ParseZetoVariantKey(ctx, "V0")
	require.NoError(t, err)
	assert.Equal(t, ZetoVariant_V0, v)
	v, err = ParseZetoVariantKey(ctx, "V1")
	require.NoError(t, err)
	assert.Equal(t, ZetoVariant_V1, v)

	for _, key := range []string{"v1", "V01", "1", "V", "V-1", "V1 ", ""} {
		_, err := ParseZetoVariantKey(ctx, key)
		assert.Regexp(t, "PD210144", err, key)
	}
	_, err = ParseZetoVariantKey(ctx, "V99")
	assert.Regexp(t, "PD210143", err)
}

func TestZetoFungibleABIForVariant(t *testing.T) {
	// every supported variant resolves to a distinct, non-empty ABI
	v0 := ZetoFungibleABIForVariant(ZetoVariant_V0)
	v1 := ZetoFungibleABIForVariant(ZetoVariant_V1)
	require.NotEmpty(t, v0)
	require.NotEmpty(t, v1)
	// V1 added the lock lifecycle methods; V0 has the legacy lock()
	assert.NotNil(t, v1.Functions()["createLock"])
	assert.Nil(t, v0.Functions()["createLock"])
	assert.NotNil(t, v0.Functions()["lock"])

	// an unknown variant has no ABI, so none of its functions resolve
	assert.Nil(t, ZetoFungibleABIForVariant(ZetoVariant(99)))
	assert.Nil(t, ZetoFungibleFunctionForVariant(ZetoVariant(99), "transfer"))
}

func TestZetoVariantString(t *testing.T) {
	assert.Equal(t, "V1", ZetoVariant_V1.String())
	assert.Equal(t, "V0", ZetoVariant_V0.String())
}

// The deploy constructor and the persisted domain config declare the variant as uint256, and the ABI serializer renders
// uint256 as a decimal string — so a deploy arriving through the ABI path presents "1", not 1. Both forms must decode,
// or the operator's zeto deploy fails with "cannot unmarshal string into Go struct field".
func TestZetoVariantDecodeNumberOrString(t *testing.T) {
	for _, encoded := range []string{`1`, `"1"`, `"0x1"`} {
		t.Run(encoded, func(t *testing.T) {
			var v ZetoVariant
			require.NoError(t, json.Unmarshal([]byte(encoded), &v))
			assert.Equal(t, ZetoVariant_V1, v)
		})
	}
}

func TestZetoVariantRejectNonNumeric(t *testing.T) {
	var v ZetoVariant
	assert.Error(t, json.Unmarshal([]byte(`"not-a-number"`), &v))
	assert.Error(t, json.Unmarshal([]byte(`{}`), &v))
	assert.Error(t, json.Unmarshal([]byte(`"v1"`), &v))
	assert.Error(t, json.Unmarshal([]byte(`[]`), &v))
}

// A deploy's InitializerParams must survive the exact JSON the ABI path produces.
func TestInitializerParamsDecodeFromABIEncodedDeploy(t *testing.T) {
	var p InitializerParams
	require.NoError(t, json.Unmarshal([]byte(`{"tokenName":"Zeto_Anon","variant":"1"}`), &p))
	assert.Equal(t, "Zeto_Anon", p.TokenName)
	require.NotNil(t, p.Variant)
	assert.Equal(t, ZetoVariant_V1, *p.Variant)

	p = InitializerParams{}
	require.NoError(t, json.Unmarshal([]byte(`{"tokenName":"Zeto_Anon"}`), &p))
	assert.Nil(t, p.Variant)
}

// Round-tripping must stay numeric so Go-side configs (domain YAML, testbed deploys) read naturally.
func TestZetoVariantMarshalsAsNumber(t *testing.T) {
	b, err := json.Marshal(ZetoVariant_V1)
	require.NoError(t, err)
	assert.Equal(t, "1", string(b))
}
