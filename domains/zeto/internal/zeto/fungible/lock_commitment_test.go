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
	"encoding/json"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMax(t *testing.T) {
	assert.Equal(t, 3, max(1, 3, 2))
	assert.Equal(t, 5, max(5, 1, 2))
}

func TestLockInfoFromOutputStates(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("55", 32))
	info := types.ZetoLockInfoState{
		LockID:           lockID,
		SpendCommitment:  pldtypes.RandBytes32(),
		CancelCommitment: pldtypes.RandBytes32(),
	}
	raw, err := json.Marshal(&info)
	require.NoError(t, err)

	got, err := lockInfoFromOutputStates(ctx, &prototk.StateSchema{Id: "lock_info"}, []*prototk.EndorsableState{
		{StateDataJson: testCoinStateJSON(false), SchemaId: "coin"},
		{StateDataJson: string(raw), SchemaId: "lock_info"},
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, lockID, got.LockID)

	_, err = lockInfoFromOutputStates(ctx, nil, nil)
	require.Error(t, err)

	_, err = lockInfoFromOutputStates(ctx, &prototk.StateSchema{Id: "lock_info"}, []*prototk.EndorsableState{
		{StateDataJson: testCoinStateJSON(false), SchemaId: "coin"},
	})
	require.Error(t, err)
}

func TestComputeCreateLockCommitments(t *testing.T) {
	ctx := context.Background()
	coin, err := makeCoin(testCoinStateJSON(false))
	require.NoError(t, err)
	spendData, err := recipientsForLockInfoJSON(&types.CreateLockParams{
		From: "controller@node",
		Recipients: []*types.FungibleTransferParamEntry{
			{To: "alice@node", Amount: pldtypes.Uint64ToUint256(1)},
		},
	}, "controller@node")
	require.NoError(t, err)
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)

	spendC, cancelC, err := computeCreateLockCommitments(ctx, []string{"0xaa"}, []*types.ZetoCoin{coin}, []*types.ZetoCoin{coin}, spendData, cancelData)
	require.NoError(t, err)
	assert.False(t, spendC.IsZero())
	assert.False(t, cancelC.IsZero())
	assert.NotEqual(t, spendC, cancelC)

	badCoin := &types.ZetoCoin{Owner: pldtypes.MustParseHexBytes("0x1234")}
	_, _, err = computeCreateLockCommitments(ctx, []string{"0xaa"}, []*types.ZetoCoin{badCoin}, []*types.ZetoCoin{coin}, spendData, cancelData)
	require.Error(t, err)

	_, _, err = computeCreateLockCommitments(ctx, []string{"0xaa"}, []*types.ZetoCoin{coin}, []*types.ZetoCoin{badCoin}, spendData, cancelData)
	require.Error(t, err)

	_, _, err = computeCreateLockCommitments(ctx, []string{"0", "0xaa"}, []*types.ZetoCoin{coin}, []*types.ZetoCoin{coin}, spendData, cancelData)
	require.NoError(t, err)
}
