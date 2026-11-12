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

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
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

// The prover selects its artifact tree from the circuit's variant, so decoding must stamp the pool's variant onto every
// circuit. Without this a V1 pool would silently prove against V0 artifacts.
func TestEncodeDecodeDomainInstanceConfig(t *testing.T) {
	ctx := t.Context()
	encoded, err := EncodeDomainInstanceConfig(ctx, &DomainInstanceConfig{
		TokenName: "anon_nullifier",
		Circuits:  testCircuits(t),
		Variant:   ZetoVariant_V1,
	})
	require.NoError(t, err)
	assert.Equal(t, []byte(ZetoDomainConfigID_V1), []byte(encoded[:len(ZetoDomainConfigID_V1)]))

	decoded, err := DecodeDomainInstanceConfig(ctx, encoded)
	require.NoError(t, err)
	assert.Equal(t, "anon_nullifier", decoded.TokenName)
	assert.Equal(t, ZetoVariant_V1, decoded.Variant)
	require.NotNil(t, decoded.Circuits)
	assert.Equal(t, "tr", (*decoded.Circuits)["transfer"].Name)
	for name, circuit := range *decoded.Circuits {
		assert.Equal(t, uint64(ZetoVariant_V1), circuit.Variant, "circuit %s", name)
	}
}

// Legacy (unprefixed) registration bytes predate the variant, so they decode as V0 and their circuits are stamped
// accordingly rather than left at an ambiguous zero-with-no-meaning.
func TestDecodeUnprefixedDomainInstanceConfigIsV0(t *testing.T) {
	ctx := t.Context()
	configJSON, err := json.Marshal(&DomainInstanceConfig{TokenName: "anon", Circuits: testCircuits(t)})
	require.NoError(t, err)
	encoded, err := DomainInstanceConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
	require.NoError(t, err)

	decoded, err := DecodeDomainInstanceConfig(ctx, encoded)
	require.NoError(t, err)
	assert.Equal(t, "anon", decoded.TokenName)
	assert.Equal(t, ZetoVariant_V0, decoded.Variant)
	require.NotNil(t, decoded.Circuits)
	assert.Equal(t, "d", (*decoded.Circuits)["deposit"].Name)
	for _, circuit := range *decoded.Circuits {
		assert.Equal(t, uint64(ZetoVariant_V0), circuit.Variant)
	}
}

// A V0 pool must be byte-identical to the unprefixed encoding, so nodes that predate the variant can still read it.
func TestEncodeV0DomainInstanceConfigIsUnprefixed(t *testing.T) {
	ctx := t.Context()
	cfg := &DomainInstanceConfig{TokenName: "anon", Circuits: testCircuits(t), Variant: ZetoVariant_V0}
	encoded, err := EncodeDomainInstanceConfig(ctx, cfg)
	require.NoError(t, err)

	configJSON, err := json.Marshal(&DomainInstanceConfig{TokenName: "anon", Circuits: testCircuits(t)})
	require.NoError(t, err)
	expected, err := DomainInstanceConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
	require.NoError(t, err)
	assert.Equal(t, []byte(expected), []byte(encoded))

	decoded, err := DecodeDomainInstanceConfig(ctx, encoded)
	require.NoError(t, err)
	assert.Equal(t, ZetoVariant_V0, decoded.Variant)
}

func TestDecodeDomainInstanceConfigFail(t *testing.T) {
	ctx := t.Context()
	_, err := DecodeDomainInstanceConfig(ctx, []byte{0x01, 0x02})
	assert.Regexp(t, "PD210148", err)
	_, err = DecodeDomainInstanceConfig(ctx, append([]byte(ZetoDomainConfigID_V1), 0x01))
	assert.Regexp(t, "PD210018", err)
}
