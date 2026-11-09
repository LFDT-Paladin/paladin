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
	ctx := t.Context()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("11", 32))
	require.Error(t, validateLockUnlockParams(ctx, lockID, ""))
	require.Error(t, validateLockUnlockParams(ctx, pldtypes.Bytes32{}, "alice@node"))
	require.NoError(t, validateLockUnlockParams(ctx, lockID, "alice@node"))
}

func TestFindLockInfo(t *testing.T) {
	ctx := t.Context()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32))
	li := &types.ZetoLockInfoState{LockID: lockID, SpendData: []byte(`[]`)}
	raw, err := json.Marshal(li)
	require.NoError(t, err)
	schema := &prototk.StateSchema{Id: "lock_info"}

	found := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{{Id: "lock-info-state", DataJson: string(raw)}},
			}, nil
		},
	}
	got, stateID, err := FindLockInfo(ctx, found, schema, "ctx", lockID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, lockID, got.LockID)
	assert.Equal(t, "lock-info-state", stateID)

	_, _, err = FindLockInfo(ctx, mockLockInfoCallbacks("not-json"), schema, "ctx", lockID)
	require.ErrorContains(t, err, "PD210087")

	_, _, err = FindLockInfo(ctx, &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("query failed")
		},
	}, schema, "ctx", lockID)
	require.ErrorContains(t, err, "query failed")

	got, _, err = FindLockInfo(ctx, &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{}, nil
		},
	}, schema, "ctx", lockID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUnlockCommonLoadLockInfo(t *testing.T) {
	ctx := t.Context()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("23", 32))
	raw, err := json.Marshal(&types.ZetoLockInfoState{LockID: lockID})
	require.NoError(t, err)

	h := &unlockCommon{
		baseHandler: baseHandler{stateSchemas: &common.StateSchemas{}},
		callbacks:   mockLockInfoCallbacks(string(raw)),
	}
	_, err = h.loadLockInfo(ctx, lockID, "ctx")
	require.ErrorContains(t, err, "PD210126")

	h.stateSchemas.LockInfoSchema = &prototk.StateSchema{Id: "lock_info"}
	got, err := h.loadLockInfo(ctx, lockID, "ctx")
	require.NoError(t, err)
	assert.Equal(t, lockID, got.LockID)

	h.callbacks = &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{}, nil
		},
	}
	_, err = h.loadLockInfo(ctx, lockID, "ctx")
	require.ErrorContains(t, err, "PD210149")
}

func TestLoadLockedCoins(t *testing.T) {
	ctx := t.Context()
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

func TestRecipientsFromLockInfo(t *testing.T) {
	ctx := t.Context()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("24", 32))
	spendData, err := json.Marshal([]*types.FungibleTransferParamEntry{{To: "bob@node", Amount: pldtypes.Uint64ToUint256(3)}})
	require.NoError(t, err)
	cancelData, err := json.Marshal([]*types.FungibleTransferParamEntry{{To: "alice@node", Amount: pldtypes.Uint64ToUint256(3)}})
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{LockID: lockID, SpendData: spendData, CancelData: cancelData}

	got, err := recipientsFromLockInfo(ctx, li, spendLockOperation)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "bob@node", got[0].To)

	got, err = recipientsFromLockInfo(ctx, li, cancelLockOperation)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "alice@node", got[0].To)

	for _, tc := range []struct {
		op         *unlockOperation
		li         *types.ZetoLockInfoState
		errContent string
	}{
		{spendLockOperation, &types.ZetoLockInfoState{LockID: lockID}, "PD210151"},
		{spendLockOperation, &types.ZetoLockInfoState{LockID: lockID, SpendData: []byte("[]")}, "PD210151"},
		{spendLockOperation, &types.ZetoLockInfoState{LockID: lockID, SpendData: []byte("not-json")}, "invalid spendData"},
		{cancelLockOperation, &types.ZetoLockInfoState{LockID: lockID}, "PD210154"},
		{cancelLockOperation, &types.ZetoLockInfoState{LockID: lockID, CancelData: []byte("[]")}, "PD210154"},
		{cancelLockOperation, &types.ZetoLockInfoState{LockID: lockID, CancelData: []byte("not-json")}, "invalid cancelData"},
	} {
		_, err := recipientsFromLockInfo(ctx, tc.li, tc.op)
		require.ErrorContains(t, err, tc.errContent)
	}
}

func TestPinnedOutputStatesForNullifiers(t *testing.T) {
	ctx := t.Context()
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

func TestProofOutputs_FromOutputStates(t *testing.T) {
	ctx := t.Context()
	h := &unlockCommon{
		baseHandler: baseHandler{
			stateSchemas: &common.StateSchemas{CoinSchema: &prototk.StateSchema{Id: "coin"}},
		},
	}
	req := &prototk.PrepareTransactionRequest{
		OutputStates: []*prototk.EndorsableState{
			{SchemaId: "other", StateDataJson: testCoinStateJSON(false)},
			{StateDataJson: testCoinStateJSON(false)},
			{StateDataJson: testCoinStateJSON(true)},
		},
	}
	outs, err := h.proofOutputs(ctx, req, spendLockOperation, &types.ZetoLockInfoState{}, false, common.GetInputSize(2))
	require.NoError(t, err)
	assert.Len(t, outs, 1)
}

func TestProofOutputs_PinnedOutputsForOperation(t *testing.T) {
	ctx := t.Context()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("55", 32))
	spendID := "0x" + strings.Repeat("cd", 32)
	cancelID := "0x" + strings.Repeat("ce", 32)
	li := testLockInfoForAssemble(t, lockID, "0x"+strings.Repeat("aa", 32), spendID, 1)
	li.CancelOutputs = []string{cancelID}

	h := &unlockCommon{
		baseHandler: baseHandler{
			stateSchemas: &common.StateSchemas{CoinSchema: &prototk.StateSchema{Id: "coin"}},
		},
		callbacks: mockLockAssembleCallbacks("", map[string]string{
			cancelID: testCoinJSONAmount(false, "0x01"),
		}),
	}
	req := &prototk.PrepareTransactionRequest{StateQueryContext: "ctx"}
	outs, err := h.proofOutputs(ctx, req, cancelLockOperation, li, false, common.GetInputSize(1))
	require.NoError(t, err)
	assert.Len(t, outs, 1)

	_, err = h.proofOutputs(ctx, req, spendLockOperation, li, false, common.GetInputSize(1))
	require.Error(t, err)

	li.SpendOutputs = []string{"not-a-valid-coin-id"}
	_, err = h.proofOutputs(ctx, req, spendLockOperation, li, false, common.GetInputSize(1))
	require.Error(t, err)
}

func TestProofOutputs_UnmarshalError(t *testing.T) {
	ctx := t.Context()
	h := &unlockCommon{
		baseHandler: baseHandler{
			stateSchemas: &common.StateSchemas{CoinSchema: &prototk.StateSchema{Id: "coin"}},
		},
	}
	req := &prototk.PrepareTransactionRequest{
		OutputStates: []*prototk.EndorsableState{{StateDataJson: "bad"}},
	}
	_, err := h.proofOutputs(ctx, req, spendLockOperation, &types.ZetoLockInfoState{}, false, 2)
	require.ErrorContains(t, err, "PD210087")
}
