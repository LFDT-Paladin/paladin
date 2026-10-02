/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleSnarkProof() *corepb.SnarkProof {
	return &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{
			{Items: []string{"0x03", "0x04"}},
			{Items: []string{"0x05", "0x06"}},
		},
		C: []string{"0x07", "0x08"},
	}
}

func TestEncodeZetoOnchainDepositProofBytes(t *testing.T) {
	ctx := context.Background()
	b, err := EncodeZetoOnchainDepositProofBytes(ctx, sampleSnarkProof())
	require.NoError(t, err)
	assert.NotEmpty(t, b)
}

func TestEncodeZetoOnchainWithdrawProofBytes(t *testing.T) {
	ctx := context.Background()
	proof := sampleSnarkProof()
	pub := map[string]string{"root": "0x0b"}

	b, err := EncodeZetoOnchainWithdrawProofBytes(ctx, constants.TOKEN_ANON, proof, pub)
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	b, err = EncodeZetoOnchainWithdrawProofBytes(ctx, constants.TOKEN_ANON_NULLIFIER, proof, pub)
	require.NoError(t, err)
	assert.NotEmpty(t, b)
}

func TestEncodeZetoOnchainTransferProofBytes(t *testing.T) {
	ctx := context.Background()
	proof := sampleSnarkProof()
	pub := map[string]string{
		"root":              "0x0b",
		"encryptionNonce":   "0x01",
		"encryptedValues":   "0x02,0x03",
		"ecdhPublicKey":     "0x04,0x05",
	}

	b, err := EncodeZetoOnchainTransferProofBytes(ctx, constants.TOKEN_ANON, proof, pub, false)
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	b, err = EncodeZetoOnchainTransferProofBytes(ctx, constants.TOKEN_ANON_NULLIFIER, proof, pub, false)
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	b, err = EncodeZetoOnchainTransferProofBytes(ctx, constants.TOKEN_ANON_NULLIFIER, proof, pub, true)
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	// lockedInputs on nullifier transfer uses proof tuple only (no root prefix).
	b, err = EncodeZetoOnchainTransferProofBytes(ctx, constants.TOKEN_ANON_NULLIFIER_KYC, proof, nil, true)
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	b, err = EncodeZetoOnchainTransferProofBytes(ctx, constants.TOKEN_ANON_ENC, proof, pub, false)
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	pubEmptyEcdh := map[string]string{
		"encryptionNonce": "0x01",
		"encryptedValues": "0x02,0x03",
		"ecdhPublicKey":   "",
	}
	b, err = EncodeZetoOnchainTransferProofBytes(ctx, constants.TOKEN_ANON_ENC, proof, pubEmptyEcdh, false)
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	pubLongEcdh := map[string]string{
		"encryptionNonce": "0x01",
		"encryptedValues": "0x02",
		"ecdhPublicKey":   "0x04,0x05,0x06,0x07",
	}
	b, err = EncodeZetoOnchainTransferProofBytes(ctx, constants.TOKEN_ANON_ENC, proof, pubLongEcdh, false)
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	pubSparseEcdh := map[string]string{
		"encryptionNonce": "0x01",
		"encryptedValues": "0x02,0x03",
		"ecdhPublicKey":   " , 0x05 ",
	}
	b, err = EncodeZetoOnchainTransferProofBytes(ctx, constants.TOKEN_ANON_ENC, proof, pubSparseEcdh, false)
	require.NoError(t, err)
	assert.NotEmpty(t, b)
}

