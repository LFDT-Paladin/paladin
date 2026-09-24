/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeZetoSpendCommitment_EmptySpendPath(t *testing.T) {
	ctx := context.Background()
	lockedInputs := []string{"0x00000000000000000000000000000000000000000000000000000000000000aa"}
	spendData := []byte(`[{"to":"alice@node","amount":"1"}]`)

	hash1, err := ComputeZetoSpendCommitment(ctx, lockedInputs, []string{}, []string{"0xbb"}, spendData)
	require.NoError(t, err)
	hash2, err := ComputeZetoSpendCommitment(ctx, lockedInputs, nil, []string{"0xbb"}, spendData)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)
	assert.False(t, hash1.IsZero())

	cancelHash, err := ComputeZetoCancelCommitment(ctx, lockedInputs, []string{}, []string{"0xcc"}, spendData)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, cancelHash, "spend and cancel domains must differ")
}

func TestComputeZetoUnlockCommitment_DomainSeparation(t *testing.T) {
	ctx := context.Background()
	lockedInputs := []string{"0x01"}
	outputs := []string{"0x02"}
	data := []byte{0xab}

	spend, err := ComputeZetoUnlockCommitment(ctx, ZetoSpendCommitmentDomain, lockedInputs, []string{}, outputs, data)
	require.NoError(t, err)
	cancel, err := ComputeZetoUnlockCommitment(ctx, ZetoCancelCommitmentDomain, lockedInputs, []string{}, outputs, data)
	require.NoError(t, err)
	assert.NotEqual(t, spend, cancel)
}
