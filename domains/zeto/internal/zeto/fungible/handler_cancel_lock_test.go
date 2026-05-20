/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCancelLockValidateParams(t *testing.T) {
	ctx := context.Background()
	h := NewCancelLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32))
	paramsJSON := `{"lockId":"` + lockID.HexString0xPrefix() + `","from":"alice@node","data":"0x"}`
	parsed, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, paramsJSON)
	require.NoError(t, err)
	cp, ok := parsed.(*types.CancelLockParams)
	require.True(t, ok)
	assert.Equal(t, lockID, cp.LockId)
	assert.Equal(t, "alice@node", cp.From)
}

func TestGetCancelLockABI_V1UsesLockableCapability(t *testing.T) {
	abi := types.LockableCapabilityCancelLockABI
	require.Equal(t, types.METHOD_CANCEL_LOCK, abi.Name)
	require.Len(t, abi.Inputs, 3)
	assert.Equal(t, "lockId", abi.Inputs[0].Name)
	assert.Equal(t, "cancelArgs", abi.Inputs[1].Name)
	assert.Equal(t, "data", abi.Inputs[2].Name)
}
