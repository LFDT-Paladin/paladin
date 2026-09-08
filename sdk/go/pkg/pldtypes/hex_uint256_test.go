// Copyright © 2024 Kaleido, Inc.
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

package pldtypes

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHexUint256(t *testing.T) {

	assert.Equal(t, int64(10), Uint64ToUint256(10).Int().Int64())

	v := MustParseHexUint256("9223372036854775807")
	assert.Equal(t, uint64(9223372036854775807), v.Int().Uint64())
	dbv, err := v.Value()
	require.NoError(t, err)
	assert.Equal(t, "0x7fffffffffffffff", v.String())
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, uint64(9223372036854775807), v.Int().Uint64())

	v = MustParseHexUint256("1152921504606846975")
	assert.Equal(t, uint64(1152921504606846975), v.Int().Uint64())
	dbv, err = v.Value()
	require.NoError(t, err)
	assert.Equal(t, "0x0fffffffffffffff", v.String())
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, uint64(1152921504606846975), v.Int().Uint64())

	v = MustParseHexUint256("0x8000000000000000")
	assert.Equal(t, uint64(9223372036854775808), v.Int().Uint64())
	dbv, err = v.Value()
	require.NoError(t, err)
	assert.Equal(t, "0000000000000000000000000000000000000000000000008000000000000000", dbv)
	assert.Equal(t, "0x8000000000000000", v.String())
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, uint64(0x8000000000000000), v.Int().Uint64())

	v = MustParseHexUint256("0x8000000000000000")
	assert.Equal(t, uint64(0x8000000000000000), v.Int().Uint64())

	// The largest value the type holds is 256 bits, and round-trips through the DB
	v = MustParseHexUint256("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	assert.Equal(t, 256, v.Int().BitLen())
	dbv, err = v.Value()
	require.NoError(t, err)
	assert.Equal(t, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", dbv)
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, 256, v.Int().BitLen())

	assert.Panics(t, func() {
		_ = MustParseHexUint256("wrong")
	})

	assert.Panics(t, func() {
		_ = MustParseHexUint256("-1")
	})

	assert.Panics(t, func() {
		_ = MustParseHexUint256("115792089237316195423570985008687907853269984665640564039457584007913129639936")
	})

	_, err = ParseHexUint256(t.Context(), "wrong")
	require.Regexp(t, "PD020009", err)

	// The type is unsigned, so a negative is not a valid value for it
	_, err = ParseHexUint256(t.Context(), "-1")
	assert.Regexp(t, "PD020027", err)
	_, err = ParseHexUint256(t.Context(), "-0x2a")
	assert.Regexp(t, "PD020027", err)
	_, err = ParseHexUint256(t.Context(), "-255")
	assert.Regexp(t, "PD020027", err)

	// Nor is a value of more than 256 bits
	_, err = ParseHexUint256(t.Context(), "115792089237316195423570985008687907853269984665640564039457584007913129639936")
	assert.Regexp(t, "PD020028", err)
	_, err = ParseHexUint256(t.Context(), "0x10000000000000000000000000000000000000000000000000000000000000000")
	assert.Regexp(t, "PD020028", err)

	type testStruct struct {
		F1 *HexUint256 `json:"f1"`
	}
	var ts testStruct
	err = json.Unmarshal([]byte(`{
		"f1": 1000000000000000000000001
	}`), &ts)
	require.NoError(t, err)
	require.Equal(t, "1000000000000000000000001", ts.F1.Int().Text(10))
	err = json.Unmarshal([]byte(`{
		"f1": "0x7fffffffffffffff"
	}`), &ts)
	require.NoError(t, err)
	assert.Equal(t, "7fffffffffffffff", ts.F1.HexString())
	err = json.Unmarshal([]byte(`{
		"f1": "wrong"
	}`), &ts)
	assert.Regexp(t, "PD020009", err)
	err = json.Unmarshal([]byte(`{
		"f1": false
	}`), &ts)
	assert.Regexp(t, "PD020002", err)
	err = json.Unmarshal([]byte(`{
		"f1": "-0x2a"
	}`), &ts)
	assert.Regexp(t, "PD020027", err)
	err = json.Unmarshal([]byte(`{
		"f1": -42
	}`), &ts)
	assert.Regexp(t, "PD020027", err)
	err = json.Unmarshal([]byte(`{
		"f1": 115792089237316195423570985008687907853269984665640564039457584007913129639936
	}`), &ts)
	assert.Regexp(t, "PD020028", err)

	err = ts.F1.Scan(int64(12345))
	require.NoError(t, err)
	assert.Equal(t, uint64(12345), ts.F1.Int().Uint64())

	err = ts.F1.Scan(false)
	assert.Regexp(t, "PD020002.*bool", err)

	b, err := json.Marshal(ts)
	require.NoError(t, err)
	assert.Equal(t, `{"f1":"0x3039"}`, string(b))

	err = ts.F1.Scan("0x12346")
	assert.Regexp(t, "PD020013", err)

	err = ts.F1.Scan(int64(-1))
	assert.Regexp(t, "PD020027", err)

	err = v.Scan("wrong000000000000000000000000000000000000000000007fffffffffffffff")
	assert.Regexp(t, "PD020013", err)

	// A value can also be handed to the type by conversion, bypassing the constructors.
	// The two hex accessors must agree on what it is, and neither may misreport its sign
	neg := (*HexUint256)(big.NewInt(-42))
	assert.Equal(t, "-0x2a", neg.HexString0xPrefix())
	assert.Equal(t, "-2a", neg.HexString())

	// Such a value cannot be represented in the DB, so it is rejected rather than
	// serialized as something else, and the caller's value is left as it was
	_, err = neg.Value()
	assert.Regexp(t, "PD020027", err)
	assert.Equal(t, int64(-42), neg.Int().Int64())

	over := (*HexUint256)(new(big.Int).Lsh(big.NewInt(1), 256))
	_, err = over.Value()
	assert.Regexp(t, "PD020028", err)
	assert.Equal(t, 257, over.Int().BitLen())

	bi := big.NewInt(-99)
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000063", string(PadHexBigUint(bi, make([]byte, 64))))
	assert.Equal(t, int64(-99), bi.Int64())

	assert.True(t, ((*HexUint256)(nil)).NilOrZero())

	dbv, err = ((*HexUint256)(nil)).Value()
	assert.NoError(t, err)
	assert.Nil(t, dbv)

}
