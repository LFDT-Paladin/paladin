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
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/hyperledger-firefly/signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCircuits(t *testing.T) *zetosignerapi.Circuits {
	t.Helper()
	c := zetosignerapi.Circuits{
		"deposit":        {Name: "d", Type: "deposit"},
		"withdraw":       {Name: "w", Type: "withdraw"},
		"transfer":       {Name: "tr", Type: "transfer"},
		"transferLocked": {Name: "tl", Type: "transferLocked"},
	}
	return &c
}

// encodeLegacy builds headerless config bytes, as written by nodes that predate config generations.
func encodeLegacy(t *testing.T, tokenName string) []byte {
	t.Helper()
	configJSON, err := json.Marshal(&DomainInstanceConfig{TokenName: tokenName, Circuits: testCircuits(t)})
	require.NoError(t, err)
	encoded, err := DomainInstanceConfigABI.EncodeABIDataJSONCtx(t.Context(), configJSON)
	require.NoError(t, err)
	return encoded
}

// The prover selects its artifact tree from the circuit's variant, so decoding must stamp the pool's variant onto every
// circuit. Without this a V1 pool would silently prove against V0 artifacts.
func TestEncodeDecodeDomainInstanceConfig(t *testing.T) {
	ctx := t.Context()
	for _, variant := range []ZetoVariant{ZetoVariant_V0, ZetoVariant_V1, ZetoDomainConfigMaxVariant} {
		encoded, err := EncodeDomainInstanceConfig(ctx, &DomainInstanceConfig{
			TokenName: "anon_nullifier",
			Circuits:  testCircuits(t),
			Variant:   variant,
		})
		require.NoError(t, err)

		decoded, err := DecodeDomainInstanceConfig(ctx, encoded)
		require.NoError(t, err)
		assert.Equal(t, "anon_nullifier", decoded.TokenName)
		assert.Equal(t, variant, decoded.Variant)
		require.NotNil(t, decoded.Circuits)
		assert.Equal(t, "tr", (*decoded.Circuits)["transfer"].Name)
		for name, circuit := range *decoded.Circuits {
			assert.Equal(t, uint64(variant), circuit.Variant, "circuit %s", name)
		}
	}
}

// Every new pool, V0 included, is written as 0x0002 || uint16 variant || the legacy payload, byte for byte.
func TestEncodeDomainInstanceConfigLayout(t *testing.T) {
	ctx := t.Context()
	legacy := encodeLegacy(t, "anon")
	for variant, header := range map[ZetoVariant]string{
		ZetoVariant_V0: "00020000",
		ZetoVariant_V1: "00020001",
		0x0102:         "00020102",
	} {
		encoded, err := EncodeDomainInstanceConfig(ctx, &DomainInstanceConfig{TokenName: "anon", Circuits: testCircuits(t), Variant: variant})
		require.NoError(t, err)
		assert.Equal(t, header, hex.EncodeToString(encoded[:ZetoDomainConfigHeaderLen]), "variant %s", variant)
		assert.Equal(t, legacy, []byte(encoded[ZetoDomainConfigHeaderLen:]), "variant %s", variant)
	}
}

func TestEncodeDomainInstanceConfigVariantTooLarge(t *testing.T) {
	_, err := EncodeDomainInstanceConfig(t.Context(), &DomainInstanceConfig{TokenName: "anon", Circuits: testCircuits(t), Variant: ZetoDomainConfigMaxVariant + 1})
	assert.Regexp(t, "PD210165", err)
}

// Legacy (headerless) registration bytes predate the variant, so they decode as V0 and their circuits are stamped
// accordingly rather than left at an ambiguous zero-with-no-meaning. Their first four bytes are always zero.
func TestDecodeLegacyDomainInstanceConfigIsV0(t *testing.T) {
	legacy := encodeLegacy(t, "anon")
	assert.Equal(t, legacyDomainConfigHeader, legacy[:ZetoDomainConfigHeaderLen])

	decoded, err := DecodeDomainInstanceConfig(t.Context(), legacy)
	require.NoError(t, err)
	assert.Equal(t, "anon", decoded.TokenName)
	assert.Equal(t, ZetoVariant_V0, decoded.Variant)
	require.NotNil(t, decoded.Circuits)
	assert.Equal(t, "d", (*decoded.Circuits)["deposit"].Name)
	for _, circuit := range *decoded.Circuits {
		assert.Equal(t, uint64(ZetoVariant_V0), circuit.Variant)
	}
}

func TestDecodeDomainInstanceConfigUnknownGeneration(t *testing.T) {
	for _, header := range []string{"00010000", "00030001", "ffff0000"} {
		data, err := hex.DecodeString(header)
		require.NoError(t, err)
		_, err = DecodeDomainInstanceConfig(t.Context(), append(data, encodeLegacy(t, "anon")...))
		assert.Regexp(t, "PD210164", err, "header %s", header)
	}
}

// Configs written by zeto-v1 builds before generations reuse the 0x00020001 header over a different payload
// (tokenName, zetoVariant, factoryVersion, circuitBundleId, circuits). They must fail to decode, not decode wrongly.
func TestDecodePreGenerationV1PayloadFails(t *testing.T) {
	oldABI := &abi.ParameterArray{
		{Name: "tokenName", Type: "string"},
		{Name: "zetoVariant", Type: "uint256"},
		{Name: "factoryVersion", Type: "uint256"},
		{Name: "circuitBundleId", Type: "string"},
		{Name: "circuits", Type: "tuple", Components: zetoDomainCircuitsComponents},
	}
	configJSON, err := json.Marshal(map[string]any{
		"tokenName": "anon", "zetoVariant": 1, "factoryVersion": 1, "circuitBundleId": "b", "circuits": testCircuits(t),
	})
	require.NoError(t, err)
	payload, err := oldABI.EncodeABIDataJSONCtx(t.Context(), configJSON)
	require.NoError(t, err)
	_, err = DecodeDomainInstanceConfig(t.Context(), append([]byte{0x00, 0x02, 0x00, 0x01}, payload...))
	assert.Regexp(t, "PD210148", err)
}

func TestDecodeDomainInstanceConfigFail(t *testing.T) {
	ctx := t.Context()
	_, err := DecodeDomainInstanceConfig(ctx, []byte{0x01, 0x02})
	assert.Regexp(t, "PD210148.*legacy", err)
	_, err = DecodeDomainInstanceConfig(ctx, append(make([]byte, 4), 0x01))
	assert.Regexp(t, "PD210148.*legacy", err)
	_, err = DecodeDomainInstanceConfig(ctx, []byte{0x00, 0x02, 0x00, 0x01, 0x01})
	assert.Regexp(t, "PD210148.*0x0002", err)
}
