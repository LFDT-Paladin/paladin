/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/require"
)

func TestMakeNewLockInfoState_UsesLockIDAsStateID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0xccdd000000000000000000000000000000000000000000000000000000000002")
	li := &types.ZetoLockInfoState{
		LockID:  lockID,
		Spender: "0x74e71b05854ee819cb9397be01c82570a178d019",
	}
	ns, err := makeNewLockInfoState(ctx, &prototk.StateSchema{Id: "lock_info"}, li, []string{"sender@node1"})
	require.NoError(t, err)
	require.NotNil(t, ns.Id)
	require.Equal(t, lockID.HexString0xPrefix(), *ns.Id)
}
