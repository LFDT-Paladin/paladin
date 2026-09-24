/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"context"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTransactionIDBytes32(t *testing.T) {
	ctx := context.Background()

	_, err := ParseTransactionIDBytes32(ctx, "")
	require.Error(t, err)

	hexID := "0x" + strings.Repeat("aa", 32)
	got, err := ParseTransactionIDBytes32(ctx, hexID)
	require.NoError(t, err)
	assert.Equal(t, pldtypes.MustParseBytes32(hexID), got)

	u := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	got, err = ParseTransactionIDBytes32(ctx, u.String())
	require.NoError(t, err)
	assert.False(t, got.IsZero())

	_, err = ParseTransactionIDBytes32(ctx, "not-a-valid-id")
	require.Error(t, err)
}
