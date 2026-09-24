/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCoinOwnerBJJ = "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922"

func testCoinStateJSON(locked bool) string {
	if locked {
		return `{"salt":"0x02","owner":"` + testCoinOwnerBJJ + `","amount":"0x05","locked":true}`
	}
	return `{"salt":"0x01","owner":"` + testCoinOwnerBJJ + `","amount":"0x0a","locked":false}`
}

func mockLockInfoCallbacks(lockInfoJSON string) *domain.MockDomainCallbacks {
	return &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			if req.SchemaId == "lock_info" {
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{{DataJson: lockInfoJSON}},
				}, nil
			}
			return &prototk.FindAvailableStatesResponse{}, nil
		},
	}
}

func TestValidateLockUnlockParams(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("11", 32))
	require.Error(t, validateLockUnlockParams(ctx, lockID, ""))
	require.Error(t, validateLockUnlockParams(ctx, pldtypes.Bytes32{}, "alice@node"))
	require.NoError(t, validateLockUnlockParams(ctx, lockID, "alice@node"))
}

func TestLoadLockInfoByLockID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32))
	li := &types.ZetoLockInfoState{LockID: lockID, SpendData: []byte(`[]`)}
	raw, err := json.Marshal(li)
	require.NoError(t, err)

	got, err := loadLockInfoByLockID(ctx, mockLockInfoCallbacks(string(raw)), &prototk.StateSchema{Id: "lock_info"}, lockID, "ctx")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, lockID, got.LockID)

	got, err = loadLockInfoByLockID(ctx, mockLockInfoCallbacks(string(raw)), nil, lockID, "ctx")
	require.NoError(t, err)
	assert.Nil(t, got)

	_, err = loadLockInfoByLockID(ctx, mockLockInfoCallbacks("not-json"), &prototk.StateSchema{Id: "lock_info"}, lockID, "ctx")
	require.Error(t, err)

	_, err = loadLockInfoByLockID(ctx, &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("query failed")
		},
	}, &prototk.StateSchema{Id: "lock_info"}, lockID, "ctx")
	require.Error(t, err)

	got, err = loadLockInfoByLockID(ctx, &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{}, nil
		},
	}, &prototk.StateSchema{Id: "lock_info"}, lockID, "ctx")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestLoadLockedCoins(t *testing.T) {
	ctx := context.Background()
	lockedID := "0x" + strings.Repeat("aa", 32)
	cb := mockLockAssembleCallbacks("", map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
	})
	prepared, revert, err := loadLockedCoins(ctx, cb, &prototk.StateSchema{Id: "coin"}, []string{lockedID}, false, "ctx")
	require.NoError(t, err)
	assert.False(t, revert)
	require.Len(t, prepared.coins, 1)

	_, _, err = loadLockedCoins(ctx, cb, &prototk.StateSchema{Id: "coin"}, []string{"not-valid-id"}, false, "ctx")
	require.Error(t, err)
}

func TestLockUnlockProofOutputs_FromOutputStates(t *testing.T) {
	ctx := context.Background()
	h := &lockUnlockHandler{
		baseHandler: baseHandler{
			stateSchemas: &common.StateSchemas{CoinSchema: &prototk.StateSchema{Id: "coin"}},
		},
	}
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("33", 32))
	req := &prototk.PrepareTransactionRequest{
		OutputStates: []*prototk.EndorsableState{
			{StateDataJson: testCoinStateJSON(false)},
			{StateDataJson: testCoinStateJSON(true)},
		},
	}
	outs, err := lockUnlockProofOutputs(ctx, h, lockID, nil, req, false, common.GetInputSize(2))
	require.NoError(t, err)
	require.NotEmpty(t, outs)
}

func TestValidateSpendLockParams(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("44", 32))
	require.Error(t, validateSpendLockParams(ctx, &types.SpendLockParams{LockId: lockID}))
	require.Error(t, validateSpendLockParams(ctx, &types.SpendLockParams{From: "alice@node"}))
	require.NoError(t, validateSpendLockParams(ctx, &types.SpendLockParams{LockId: lockID, From: "alice@node"}))
}

func TestPinnedOutputStatesForNullifiers(t *testing.T) {
	ctx := context.Background()
	coin, err := makeCoin(testCoinStateJSON(false))
	require.NoError(t, err)
	spendID := "0x" + strings.Repeat("cd", 32)
	recipients := []*types.FungibleTransferParamEntry{
		{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
	}
	out, err := pinnedOutputStatesForNullifiers(ctx, &prototk.StateSchema{Id: "coin"}, "zeto", []string{spendID}, []*types.ZetoCoin{coin}, recipients)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.NotNil(t, out[0].Id)
	assert.Equal(t, spendID, *out[0].Id)

	_, err = pinnedOutputStatesForNullifiers(ctx, &prototk.StateSchema{Id: "coin"}, "zeto", []string{spendID}, []*types.ZetoCoin{}, recipients)
	require.Error(t, err)
}

func TestSpendPinnedOutputStatesForNullifiers(t *testing.T) {
	ctx := context.Background()
	coin, err := makeCoin(testCoinStateJSON(false))
	require.NoError(t, err)
	spendID := "0x" + strings.Repeat("ef", 32)
	recipients := []*types.FungibleTransferParamEntry{
		{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
	}
	out, err := spendPinnedOutputStatesForNullifiers(ctx, &prototk.StateSchema{Id: "coin"}, "zeto", []string{spendID}, []*types.ZetoCoin{coin}, recipients)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, spendID, *out[0].Id)
}

func TestLockUnlockProofOutputs_PinnedCoinsPath(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("55", 32))
	spendID := "0x" + strings.Repeat("cd", 32)
	li := testLockInfoForAssemble(t, lockID, "0x"+strings.Repeat("aa", 32), spendID, 1)
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := &lockUnlockHandler{
		baseHandler: baseHandler{
			stateSchemas: &common.StateSchemas{
				CoinSchema:     &prototk.StateSchema{Id: "coin"},
				LockInfoSchema: &prototk.StateSchema{Id: "lock_info"},
			},
		},
		callbacks: mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
			spendID: testCoinJSONAmount(false, "0x01"),
		}),
	}
	req := &prototk.PrepareTransactionRequest{StateQueryContext: "ctx"}
	outs, err := lockUnlockProofOutputs(ctx, h, lockID, li.SpendOutputs, req, false, common.GetInputSize(1))
	require.NoError(t, err)
	require.NotEmpty(t, outs)
}

func TestPinnedCoinsForPrepare(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("66", 32))
	spendID := "0x" + strings.Repeat("cd", 32)
	li := testLockInfoForAssemble(t, lockID, "0x"+strings.Repeat("aa", 32), spendID, 1)
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := &lockUnlockHandler{
		baseHandler: baseHandler{
			stateSchemas: &common.StateSchemas{CoinSchema: &prototk.StateSchema{Id: "coin"}},
		},
		callbacks: mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
			spendID: testCoinJSONAmount(false, "0x01"),
		}),
	}
	coins, err := h.pinnedCoinsForPrepare(ctx, lockID, []string{spendID}, "ctx", false)
	require.NoError(t, err)
	require.Len(t, coins, 1)

	missingID := "0x" + strings.Repeat("ff", 32)
	_, err = h.pinnedCoinsForPrepare(ctx, lockID, []string{missingID}, "ctx", false)
	require.Error(t, err)
}

func TestLockUnlockProofOutputs_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	h := &lockUnlockHandler{
		baseHandler: baseHandler{
			stateSchemas: &common.StateSchemas{CoinSchema: &prototk.StateSchema{Id: "coin"}},
		},
	}
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("77", 32))
	req := &prototk.PrepareTransactionRequest{
		OutputStates: []*prototk.EndorsableState{{StateDataJson: "bad"}},
	}
	_, err := lockUnlockProofOutputs(ctx, h, lockID, nil, req, false, 2)
	require.Error(t, err)
}
