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

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSpendLockABI_V1UsesLockableCapabilityOnChainShape(t *testing.T) {
	abi := types.LockableCapabilitySpendLockABI
	require.Equal(t, types.METHOD_SPEND_LOCK, abi.Name)
	require.Len(t, abi.Inputs, 3)
	assert.Equal(t, "lockId", abi.Inputs[0].Name)
	assert.Equal(t, "spendArgs", abi.Inputs[1].Name)
	assert.Equal(t, "data", abi.Inputs[2].Name)
}

func TestSpendLockValidateParams_WireJSON(t *testing.T) {
	ctx := context.Background()
	h := spendLockHandler{unlockCommon: unlockCommon{baseHandler: baseHandler{name: "zeto"}}}
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32))
	emptyData := pldtypes.HexBytes{}
	params := types.SpendLockParams{
		LockId: lockID,
		From:   "alice@node",
		Data:   emptyData,
	}
	paramsJSON, err := json.Marshal(&params)
	require.NoError(t, err)
	parsed, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, string(paramsJSON))
	require.NoError(t, err)
	sp, ok := parsed.(*types.SpendLockParams)
	require.True(t, ok)
	assert.Equal(t, lockID, sp.LockId)
	assert.Equal(t, "alice@node", sp.From)
}

func strPtr(s string) *string { return &s }

func TestSpendLockInitAndEndorse(t *testing.T) {
	ctx := context.Background()
	h := NewSpendLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, nil)
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{From: "alice@node"},
		Params:      &types.SpendLockParams{From: "alice@node"},
	}
	res, err := h.Init(ctx, tx, &prototk.InitTransactionRequest{})
	require.NoError(t, err)
	require.Len(t, res.RequiredVerifiers, 1)
	endorseRes, err := h.Endorse(ctx, tx, &prototk.EndorseTransactionRequest{})
	require.NoError(t, err)
	assert.Nil(t, endorseRes)
}

func TestSpendLockValidateParams_Errors(t *testing.T) {
	ctx := context.Background()
	h := spendLockHandler{unlockCommon: unlockCommon{baseHandler: baseHandler{name: "zeto"}}}
	_, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, `{"lockId":"0x`+strings.Repeat("00", 32)+`"}`)
	require.Error(t, err)
}

func TestSpendLockValidateParams_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	h := NewSpendLockHandler("zeto", nil, nil, nil, nil, nil, nil)
	_, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, "{bad")
	require.Error(t, err)
}

func TestSpendLockAssemble_UsesSpendOperation(t *testing.T) {
	h := &spendLockHandler{unlockCommon: *newTestUnlockCommon(t, testUnlockLockInfo(t), testUnlockCoins())}
	tx, req := testUnlockAssembleTx(&h.unlockCommon, constants.TOKEN_ANON_NULLIFIER)
	tx.Params = &types.SpendLockParams{LockId: testUnlockLockID, From: "spender@node"}
	res, err := h.Assemble(t.Context(), tx, req)
	require.NoError(t, err)
	require.Len(t, res.AssembledTransaction.OutputStates, 1)
	assert.Equal(t, testUnlockPinnedStateID(t, testUnlockSpendID), *res.AssembledTransaction.OutputStates[0].Id)
}

func TestSpendLockPrepare_UsesSpendOperation(t *testing.T) {
	h := &spendLockHandler{unlockCommon: *newTestUnlockCommon(t, testUnlockLockInfo(t), testUnlockCoins())}
	tx := testUnlockPrepareTx(constants.TOKEN_ANON, pldtypes.HexUint64(types.ZetoFungibleABI_V1))
	tx.Params = &types.SpendLockParams{LockId: testUnlockLockID, From: "spender@node"}
	res, err := h.Prepare(t.Context(), tx, testUnlockPrepareReq(t, nil))
	require.NoError(t, err)
	var fn abiJSONFields
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.FunctionAbiJson), &fn))
	assert.Equal(t, "spendLock", fn.Name)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	assert.NotEmpty(t, params["spendArgs"])
}
