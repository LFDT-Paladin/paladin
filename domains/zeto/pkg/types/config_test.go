/*
 * Copyright contributors to Paladin, an LFDT project
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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resolveVariantsJSON(t *testing.T, configJSON string) (*DomainVariants, error) {
	var config DomainFactoryConfig
	require.NoError(t, json.Unmarshal([]byte(configJSON), &config))
	return config.ResolveVariants(t.Context())
}

func TestResolveVariantsDomainContractsFormat(t *testing.T) {
	dv, err := resolveVariantsJSON(t, `{
		"domainContracts": {"implementations": [{"name": "Zeto_Anon", "circuits": {"deposit": {"name": "deposit"}}}]},
		"snarkProver": {"circuitsDir": "/zkp"}
	}`)
	require.NoError(t, err)
	assert.Equal(t, ZetoVariant_V0, *dv.DefaultVariant)
	assert.False(t, dv.ArtifactsPerVariant)
	impl, err := dv.GetImplementation(t.Context(), ZetoVariant_V0, "Zeto_Anon")
	require.NoError(t, err)
	assert.Equal(t, "Zeto_Anon", impl.RegisteredName)
	assert.Equal(t, "deposit", (*impl.Circuits)["deposit"].Name)
	assert.Equal(t, "deposit", string((*impl.Circuits)["deposit"].Type))
}

func TestResolveVariantsNoImplementations(t *testing.T) {
	dv, err := resolveVariantsJSON(t, `{"snarkProver": {"circuitsDir": "/zkp"}}`)
	require.NoError(t, err)
	assert.Equal(t, ZetoVariant_V0, *dv.DefaultVariant)
	assert.False(t, dv.ArtifactsPerVariant)
	_, err = dv.GetImplementation(t.Context(), ZetoVariant_V0, "Zeto_Anon")
	assert.Regexp(t, "PD210146", err)
}

func TestResolveVariantsSingleVariant(t *testing.T) {
	dv, err := resolveVariantsJSON(t, `{
		"variants": {"V1": {"implementations": [{"name": "Zeto_Anon"}]}}
	}`)
	require.NoError(t, err)
	assert.Equal(t, ZetoVariant_V1, *dv.DefaultVariant)
	assert.True(t, dv.ArtifactsPerVariant)
	_, err = dv.GetImplementation(t.Context(), ZetoVariant_V0, "Zeto_Anon")
	assert.Regexp(t, "PD210145.*V0", err)
}

func TestResolveVariantsMultipleVariants(t *testing.T) {
	dv, err := resolveVariantsJSON(t, `{
		"variants": {
			"V0": {"implementations": [{"name": "Zeto_Anon"}, {"name": "Zeto_NfAnon"}]},
			"V1": {"implementations": [{"name": "Zeto_Anon", "registeredName": "Zeto_Anon_V1"}]}
		},
		"defaultVariant": "V1"
	}`)
	require.NoError(t, err)
	assert.Equal(t, ZetoVariant_V1, *dv.DefaultVariant)
	impl, err := dv.GetImplementation(t.Context(), ZetoVariant_V0, "Zeto_Anon")
	require.NoError(t, err)
	assert.Equal(t, "Zeto_Anon", impl.RegisteredName)
	impl, err = dv.GetImplementation(t.Context(), ZetoVariant_V1, "Zeto_Anon")
	require.NoError(t, err)
	assert.Equal(t, "Zeto_Anon_V1", impl.RegisteredName)
	_, err = dv.GetImplementation(t.Context(), ZetoVariant_V1, "Zeto_NfAnon")
	assert.Regexp(t, "PD210146.*Zeto_NfAnon.*V1", err)
}

func TestResolveVariantsNoDefault(t *testing.T) {
	dv, err := resolveVariantsJSON(t, `{"variants": {"V0": {}, "V1": {}}}`)
	require.NoError(t, err)
	assert.Nil(t, dv.DefaultVariant)
}

func TestResolveVariantsErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		config string
		err    string
	}{
		"both formats": {
			config: `{"domainContracts": {"implementations": []}, "variants": {"V0": {}}}`,
			err:    "PD210161",
		},
		"invalid variant key": {
			config: `{"variants": {"v1": {}}}`,
			err:    "PD210144",
		},
		"unsupported variant": {
			config: `{"variants": {"V9": {}}}`,
			err:    "PD210143",
		},
		"invalid default variant": {
			config: `{"variants": {"V1": {}}, "defaultVariant": "1"}`,
			err:    "PD210144",
		},
		"default variant not configured": {
			config: `{"variants": {"V1": {}}, "defaultVariant": "V0"}`,
			err:    "PD210145",
		},
		"duplicate implementation": {
			config: `{"variants": {"V1": {"implementations": [{"name": "Zeto_Anon"}, {"name": "Zeto_Anon", "registeredName": "other"}]}}}`,
			err:    "PD210147",
		},
		"duplicate registered name": {
			config: `{"variants": {"V0": {"implementations": [{"name": "Zeto_Anon"}]}, "V1": {"implementations": [{"name": "Zeto_Anon"}]}}, "defaultVariant": "V1"}`,
			err:    "PD210159",
		},
		"non-fungible at V1": {
			config: `{"variants": {"V1": {"implementations": [{"name": "Zeto_NfAnonNullifier"}]}}}`,
			err:    "PD210162",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveVariantsJSON(t, tc.config)
			assert.Regexp(t, tc.err, err)
		})
	}
}
