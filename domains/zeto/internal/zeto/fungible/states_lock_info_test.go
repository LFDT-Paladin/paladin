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
	ctx := t.Context()
	lockID := pldtypes.MustParseBytes32("0xccdd000000000000000000000000000000000000000000000000000000000002")
	li := &types.ZetoLockInfoState{
		LockID:  lockID,
		Spender: pldtypes.MustEthAddress("0x74e71b05854ee819cb9397be01c82570a178d019"),
	}
	ns, err := makeNewLockInfoState(ctx, &prototk.StateSchema{Id: "lock_info"}, li, []string{"sender@node1"})
	require.NoError(t, err)
	require.NotNil(t, ns.Id)
	require.Equal(t, lockID.HexString0xPrefix(), *ns.Id)
}

func TestMakeNewLockInfoState_Errors(t *testing.T) {
	ctx := context.Background()
	schema := &prototk.StateSchema{Id: "lock_info"}
	_, err := makeNewLockInfoState(ctx, schema, nil, nil)
	require.Error(t, err)
	_, err = makeNewLockInfoState(ctx, schema, &types.ZetoLockInfoState{}, nil)
	require.Error(t, err)
}
