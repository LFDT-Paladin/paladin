/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestZetoCreateLockArgsTupleABI_RoundTrip ensures createArgs bytes match Solidity abi.decode(createArgs, (ZetoCreateLockArgs)).
func TestZetoCreateLockArgsTupleABI_RoundTrip(t *testing.T) {
	ctx := context.Background()
	wire := zetoCreateLockArgsWireJSON{
		TxID:          "0x" + strings.Repeat("aa", 32),
		Inputs:        []string{"0x01", "0x02"},
		Outputs:       []string{"0x03"},
		LockedOutputs: []string{"0x04"},
		Proof:         "0xabcd",
	}
	j, err := json.Marshal([]any{wire})
	require.NoError(t, err)

	enc, err := zetoCreateLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, j)
	require.NoError(t, err)
	require.NotEmpty(t, enc)

	decoded, err := zetoCreateLockArgsTupleABI.DecodeABIDataCtx(ctx, enc, 0)
	require.NoError(t, err)
	require.Len(t, decoded.Children, 1)
	inner := decoded.Children[0]
	require.NotNil(t, inner)
	require.Len(t, inner.Children, 5)
	out, err := inner.JSON()
	require.NoError(t, err)
	js := string(out)
	assert.Contains(t, js, `"txId"`)
	assert.Contains(t, js, `"inputs"`)
	assert.Contains(t, js, `"outputs"`)
	assert.Contains(t, js, `"lockedOutputs"`)
	assert.Contains(t, js, `"proof"`)
}

// ethers v6: AbiCoder.encode(["tuple(bytes32,uint256[],uint256[],uint256[],bytes)"], [obj])
// prefixes the payload with a 0x20 offset word (see zeto_anon.ts encodeCreateArgs).
func TestZetoCreateLockArgsTupleABI_FirstWordMatchesEthers(t *testing.T) {
	ctx := context.Background()
	wire := zetoCreateLockArgsWireJSON{
		TxID:          "0x" + strings.Repeat("11", 32),
		Inputs:        []string{"0x01", "0x02"},
		Outputs:       []string{},
		LockedOutputs: []string{"0x03"},
		Proof:         "0xabcd",
	}
	j, err := json.Marshal([]any{wire})
	require.NoError(t, err)
	enc, err := zetoCreateLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, j)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(enc), 32)
	got := hex.EncodeToString(enc[:32])
	const ethersFirstWord = "0000000000000000000000000000000000000000000000000000000000000020"
	assert.Equal(t, ethersFirstWord, got, "must match ethers encodeCreateArgs / zeto_anon.ts; mismatch breaks Solidity abi.decode(createArgs, (ZetoCreateLockArgs))")
}
