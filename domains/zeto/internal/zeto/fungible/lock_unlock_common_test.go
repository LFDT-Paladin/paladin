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
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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

// overflowCommitment is one bit too wide to be a 32 byte state id
var overflowCommitment = "0x1" + strings.Repeat("0", 64)

func testCommitments(ids ...string) []pldtypes.HexUint256 {
	out := make([]pldtypes.HexUint256, len(ids))
	for i, id := range ids {
		out[i] = *pldtypes.MustParseHexUint256(id)
	}
	return out
}

func testCoinStateID(t *testing.T, coin *types.ZetoCoin) string {
	t.Helper()
	hash, err := coin.Hash(t.Context())
	require.NoError(t, err)
	id, err := hash.Bytes32(t.Context())
	require.NoError(t, err)
	return id.String()
}

func testStateIDs(ids ...string) []pldtypes.Bytes32 {
	out := make([]pldtypes.Bytes32, len(ids))
	for i, id := range ids {
		out[i] = pldtypes.MustParseBytes32(id)
	}
	return out
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
	prepared, revert, err := loadLockedCoins(ctx, cb, &prototk.StateSchema{Id: "coin"}, testCommitments(lockedID), false, "ctx")
	require.NoError(t, err)
	assert.False(t, revert)
	require.Len(t, prepared.coins, 1)

	_, _, err = loadLockedCoins(ctx, cb, &prototk.StateSchema{Id: "coin"}, testCommitments(overflowCommitment), false, "ctx")
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
	recipients := []*types.FungibleTransferParamEntry{
		{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
	}
	out, err := pinnedOutputStatesForNullifiers(ctx, &prototk.StateSchema{Id: "coin"}, "zeto", []*types.ZetoCoin{coin}, recipients)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.NotNil(t, out[0].Id)
	assert.Equal(t, testCoinStateID(t, coin), *out[0].Id)

	_, err = pinnedOutputStatesForNullifiers(ctx, &prototk.StateSchema{Id: "coin"}, "zeto", []*types.ZetoCoin{}, recipients)
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
	li.CancelOutputs = testCommitments(cancelID)

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

	li.SpendOutputs = testCommitments(overflowCommitment)
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

func TestZetoSpendLockArgsTupleABI_RoundTrip(t *testing.T) {
	ctx := t.Context()
	wire := zetoSpendLockArgsWireJSON{
		TxID:          "0x" + strings.Repeat("aa", 32),
		LockedOutputs: []string{"0x01"},
		Outputs:       []string{"0x02", "0x03"},
		Proof:         "0xabcd",
		Data:          "0xbeef",
	}
	j, err := json.Marshal([]any{wire})
	require.NoError(t, err)
	enc, err := zetoSpendLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, j)
	require.NoError(t, err)
	require.NotEmpty(t, enc)
	decoded, err := zetoSpendLockArgsTupleABI.DecodeABIDataCtx(ctx, enc, 0)
	require.NoError(t, err)
	require.Len(t, decoded.Children, 1)
	inner := decoded.Children[0]
	require.NotNil(t, inner)
	require.Len(t, inner.Children, 5)
	out, err := inner.JSON()
	require.NoError(t, err)
	js := string(out)
	assert.Contains(t, js, `"txId"`)
	assert.Contains(t, js, `"lockedOutputs"`)
	assert.Contains(t, js, `"outputs"`)
	assert.Contains(t, js, `"proof"`)
	assert.Contains(t, js, `"data"`)
}

func TestZetoSpendLockArgsTupleABI_FirstWordMatchesEthers(t *testing.T) {
	ctx := t.Context()
	wire := zetoSpendLockArgsWireJSON{
		TxID:          "0x" + strings.Repeat("11", 32),
		LockedOutputs: []string{"0x01"},
		Outputs:       []string{"0x02"},
		Proof:         "0xabcd",
		Data:          "0x",
	}
	j, err := json.Marshal([]any{wire})
	require.NoError(t, err)
	enc, err := zetoSpendLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, j)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(enc), 32)
	got := hex.EncodeToString(enc[:32])
	const ethersFirstWord = "0000000000000000000000000000000000000000000000000000000000000020"
	assert.Equal(t, ethersFirstWord, got, "must match ethers tuple encode; mismatch breaks Solidity abi.decode(spendArgs, (IZetoLockableCapability.ZetoSpendLockArgs))")
}

var (
	testUnlockLockID   = pldtypes.MustParseBytes32("0x" + strings.Repeat("5a", 32))
	testUnlockLockedID = "0x" + strings.Repeat("aa", 32)
	testUnlockSpendID  = "0x" + strings.Repeat("bb", 32)
	testUnlockCancelID = "0x" + strings.Repeat("cc", 32)
)

type testUnlockOp struct {
	name             string
	op               *unlockOperation
	outputID         string
	recipient        string
	setOutputs       func(li *types.ZetoLockInfoState, outputs []pldtypes.HexUint256)
	setData          func(li *types.ZetoLockInfoState, data pldtypes.HexBytes)
	emptyOutputsCode string
	missingDataCode  string
}

var testUnlockOps = []testUnlockOp{
	{
		name:             "spend",
		op:               spendLockOperation,
		outputID:         testUnlockSpendID,
		recipient:        "recipient@node",
		setOutputs:       func(li *types.ZetoLockInfoState, outputs []pldtypes.HexUint256) { li.SpendOutputs = outputs },
		setData:          func(li *types.ZetoLockInfoState, data pldtypes.HexBytes) { li.SpendData = data },
		emptyOutputsCode: "PD210152",
		missingDataCode:  "PD210151",
	},
	{
		name:             "cancel",
		op:               cancelLockOperation,
		outputID:         testUnlockCancelID,
		recipient:        "spender@node",
		setOutputs:       func(li *types.ZetoLockInfoState, outputs []pldtypes.HexUint256) { li.CancelOutputs = outputs },
		setData:          func(li *types.ZetoLockInfoState, data pldtypes.HexBytes) { li.CancelData = data },
		emptyOutputsCode: "PD210155",
		missingDataCode:  "PD210154",
	},
}

func testUnlockLockInfo(t *testing.T) *types.ZetoLockInfoState {
	li := testLockInfoForAssemble(t, testUnlockLockID, testUnlockLockedID, testUnlockSpendID, 1)
	li.CancelOutputs = testCommitments(testUnlockCancelID)
	return li
}

func testUnlockCoins() map[string]string {
	return map[string]string{
		testUnlockLockedID: testCoinJSONAmount(true, "0x01"),
		testUnlockSpendID:  testCoinJSONAmount(false, "0x01"),
		// distinct salt so the cancel coin hash (its state id) differs from the spend coin
		testUnlockCancelID: `{"salt":"0x03","owner":"` + testCoinOwnerBJJ + `","amount":"0x01","locked":false}`,
	}
}

// testUnlockPinnedStateID is the state id assembled for a pinned output coin - its commitment hash
func testUnlockPinnedStateID(t *testing.T, outputID string) string {
	coin, err := makeCoin(testUnlockCoins()[outputID])
	require.NoError(t, err)
	return testCoinStateID(t, coin)
}

func newTestUnlockCommon(t *testing.T, li *types.ZetoLockInfoState, coinsByID map[string]string) *unlockCommon {
	lockInfoJSON := ""
	if li != nil {
		raw, err := json.Marshal(li)
		require.NoError(t, err)
		lockInfoJSON = string(raw)
	}
	return &unlockCommon{
		baseHandler: baseHandler{name: "zeto", stateSchemas: testAssembleSchemas()},
		callbacks:   mockLockAssembleCallbacks(lockInfoJSON, coinsByID),
	}
}

func testUnlockAssembleTx(h *unlockCommon, tokenName string) (*types.ParsedTransaction, *prototk.AssembleTransactionRequest) {
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{
			From:         "spender@node",
			ContractInfo: &prototk.ContractInfo{ContractAddress: "0x1234567890123456789012345678901234567890"},
		},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: tokenName,
			Circuits: &zetosignerapi.Circuits{
				// v0.5 Zeto_AnonNullifier: lock spend uses the plain "anon" circuit + Groth16Verifier_Anon (see zeto ignition zeto_anon_nullifier).
				types.METHOD_TRANSFER_LOCKED: {Name: "anon", UsesNullifiers: false},
			},
		},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: tx.Transaction,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{{
			Lookup: "spender@node", Verifier: testCoinOwnerBJJ,
			Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		}},
	}
	return tx, req
}

func TestUnlockAssemble_NullifierTokenReissuesOperationOutputs(t *testing.T) {
	for _, tc := range testUnlockOps {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestUnlockCommon(t, testUnlockLockInfo(t), testUnlockCoins())
			tx, req := testUnlockAssembleTx(h, constants.TOKEN_ANON_NULLIFIER)
			res, err := h.assemble(t.Context(), tx, req, tc.op, testUnlockLockID)
			require.NoError(t, err)
			require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
			require.Len(t, res.AssembledTransaction.InputStates, 1)
			assert.Equal(t, testUnlockLockedID, res.AssembledTransaction.InputStates[0].Id)
			require.Len(t, res.AssembledTransaction.OutputStates, 1)
			assert.Equal(t, testUnlockPinnedStateID(t, tc.outputID), *res.AssembledTransaction.OutputStates[0].Id)
			assert.Equal(t, []string{tc.recipient}, res.AssembledTransaction.OutputStates[0].DistributionList)
			require.Len(t, res.AttestationPlan, 1)
			assert.Equal(t, []string{"spender@node"}, res.AttestationPlan[0].Parties)
		})
	}
}

func TestUnlockAssemble_NoOutputStatesWithoutNullifiers(t *testing.T) {
	h := newTestUnlockCommon(t, testUnlockLockInfo(t), testUnlockCoins())
	tx, req := testUnlockAssembleTx(h, constants.TOKEN_ANON)
	res, err := h.assemble(t.Context(), tx, req, spendLockOperation, testUnlockLockID)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	assert.Empty(t, res.AssembledTransaction.OutputStates)
	assert.NotEmpty(t, res.AttestationPlan)
}

func TestUnlockAssemble_Errors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		lockInfo   func(li *types.ZetoLockInfoState) *types.ZetoLockInfoState
		setup      func(h *unlockCommon, req *prototk.AssembleTransactionRequest)
		errContent string
	}{
		{
			name:       "sender verifier not resolved",
			setup:      func(h *unlockCommon, req *prototk.AssembleTransactionRequest) { req.ResolvedVerifiers = nil },
			errContent: "PD210036",
		},
		{
			name:       "lock info schema missing",
			setup:      func(h *unlockCommon, req *prototk.AssembleTransactionRequest) { h.stateSchemas.LockInfoSchema = nil },
			errContent: "PD210126",
		},
		{
			name:       "lock not found",
			lockInfo:   func(li *types.ZetoLockInfoState) *types.ZetoLockInfoState { return nil },
			errContent: "PD210149",
		},
		{
			name:       "empty locked outputs",
			lockInfo:   func(li *types.ZetoLockInfoState) *types.ZetoLockInfoState { li.LockedOutputs = nil; return li },
			errContent: "PD210150",
		},
		{
			name:       "missing spender",
			lockInfo:   func(li *types.ZetoLockInfoState) *types.ZetoLockInfoState { li.Spender = nil; return li },
			errContent: "missing spender",
		},
		{
			name: "invalid locked output id",
			lockInfo: func(li *types.ZetoLockInfoState) *types.ZetoLockInfoState {
				li.LockedOutputs = testCommitments(overflowCommitment)
				return li
			},
			errContent: "PD020027",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			li := testUnlockLockInfo(t)
			if tc.lockInfo != nil {
				li = tc.lockInfo(li)
			}
			h := newTestUnlockCommon(t, li, testUnlockCoins())
			tx, req := testUnlockAssembleTx(h, constants.TOKEN_ANON)
			if tc.setup != nil {
				tc.setup(h, req)
			}
			_, err := h.assemble(t.Context(), tx, req, spendLockOperation, testUnlockLockID)
			require.ErrorContains(t, err, tc.errContent)
		})
	}
}

func TestUnlockAssemble_OperationErrors(t *testing.T) {
	for _, tc := range testUnlockOps {
		t.Run(tc.name, func(t *testing.T) {
			for _, ec := range []struct {
				name       string
				mutate     func(li *types.ZetoLockInfoState)
				errContent string
			}{
				{"invalid output id", func(li *types.ZetoLockInfoState) { tc.setOutputs(li, testCommitments(overflowCommitment)) }, "PD020027"},
				{"empty outputs", func(li *types.ZetoLockInfoState) { tc.setOutputs(li, nil) }, tc.emptyOutputsCode},
				{"missing data", func(li *types.ZetoLockInfoState) { tc.setData(li, nil) }, tc.missingDataCode},
			} {
				t.Run(ec.name, func(t *testing.T) {
					li := testUnlockLockInfo(t)
					ec.mutate(li)
					h := newTestUnlockCommon(t, li, testUnlockCoins())
					tx, req := testUnlockAssembleTx(h, constants.TOKEN_ANON)
					_, err := h.assemble(t.Context(), tx, req, tc.op, testUnlockLockID)
					require.ErrorContains(t, err, ec.errContent)
				})
			}
		})
	}
}

func TestUnlockAssemble_Reverts(t *testing.T) {
	for _, tc := range []struct {
		name          string
		op            *unlockOperation
		amount        uint64
		coins         func(coins map[string]string)
		revertContent string
	}{
		{
			name:          "locked coins missing",
			op:            spendLockOperation,
			amount:        1,
			coins:         func(coins map[string]string) { delete(coins, testUnlockLockedID) },
			revertContent: "PD210134",
		},
		{
			name:          "pinned spend output missing",
			op:            spendLockOperation,
			amount:        1,
			coins:         func(coins map[string]string) { delete(coins, testUnlockSpendID) },
			revertContent: "PD210134",
		},
		{
			name:          "pinned cancel output missing",
			op:            cancelLockOperation,
			amount:        1,
			coins:         func(coins map[string]string) { delete(coins, testUnlockCancelID) },
			revertContent: "PD210134",
		},
		{
			name:          "locked inputs less than recipients",
			op:            spendLockOperation,
			amount:        100,
			revertContent: "PD210129",
		},
		{
			name:   "pinned outputs differ from recipients",
			op:     spendLockOperation,
			amount: 5,
			coins: func(coins map[string]string) {
				coins[testUnlockLockedID] = testCoinJSONAmount(true, "0x05")
			},
			revertContent: "PD210129",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			li := testLockInfoForAssemble(t, testUnlockLockID, testUnlockLockedID, testUnlockSpendID, tc.amount)
			li.CancelOutputs = testCommitments(testUnlockCancelID)
			coins := testUnlockCoins()
			if tc.coins != nil {
				tc.coins(coins)
			}
			h := newTestUnlockCommon(t, li, coins)
			tx, req := testUnlockAssembleTx(h, constants.TOKEN_ANON)
			res, err := h.assemble(t.Context(), tx, req, tc.op, testUnlockLockID)
			require.NoError(t, err)
			require.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
			require.NotNil(t, res.RevertReason)
			assert.Contains(t, *res.RevertReason, tc.revertContent)
		})
	}
}

func testUnlockPrepareTx(tokenName string, variant pldtypes.HexUint64) *types.ParsedTransaction {
	return &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "spender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: tokenName, ZetoVariant: variant},
	}
}

func testUnlockPrepareReq(t *testing.T, publicInputs map[string]string) *prototk.PrepareTransactionRequest {
	proofRes := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x01", "0x02"},
			B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
			C: []string{"0x07", "0x08"},
		},
		PublicInputs: publicInputs,
	}
	payload, err := proto.Marshal(&proofRes)
	require.NoError(t, err)
	return &prototk.PrepareTransactionRequest{
		Transaction:       &prototk.TransactionSpecification{From: "spender@node", TransactionId: "0x" + strings.Repeat("33", 32)},
		StateQueryContext: "ctx",
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
}

type abiJSONFields struct {
	Name   string `json:"name"`
	Inputs []struct {
		Name string `json:"name"`
	} `json:"inputs"`
}

func TestUnlockPrepare_V1EncodesOperationArgs(t *testing.T) {
	v1 := pldtypes.HexUint64(types.ZetoFungibleABI_V1)
	for _, tc := range testUnlockOps {
		t.Run(tc.name, func(t *testing.T) {
			li := testUnlockLockInfo(t)
			h := newTestUnlockCommon(t, li, testUnlockCoins())
			res, err := h.prepare(t.Context(), testUnlockPrepareTx(constants.TOKEN_ANON_NULLIFIER, v1), testUnlockPrepareReq(t, nil), tc.op, testUnlockLockID)
			require.NoError(t, err)

			var fn abiJSONFields
			require.NoError(t, json.Unmarshal([]byte(res.Transaction.FunctionAbiJson), &fn))
			assert.Equal(t, tc.op.functionABI.Name, fn.Name)
			require.Len(t, fn.Inputs, 3)
			assert.Equal(t, "spender@node", *res.Transaction.RequiredSigner)

			var params map[string]any
			require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
			assert.Len(t, params, 3)
			assert.Equal(t, testUnlockLockID.HexString0xPrefix(), params["lockId"])
			assert.Contains(t, params, "data")
			argsHex, _ := params[tc.op.argsParam].(string)
			require.NotEmpty(t, argsHex)

			decoded, err := zetoSpendLockArgsTupleABI.DecodeABIData(pldtypes.MustParseHexBytes(argsHex), 0)
			require.NoError(t, err)
			require.Len(t, decoded.Children, 1)
			require.Len(t, decoded.Children[0].Children, 5)
			j, err := decoded.Children[0].JSON()
			require.NoError(t, err)
			assert.Contains(t, string(j), `"data":"`+hex.EncodeToString(tc.op.data(li))+`"`)
		})
	}
}

func TestUnlockPrepare_V0DiscreteProofParams(t *testing.T) {
	h := newTestUnlockCommon(t, testUnlockLockInfo(t), testUnlockCoins())
	req := testUnlockPrepareReq(t, nil)
	req.InputStates = []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(true)}}
	req.OutputStates = []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(false)}}
	res, err := h.prepare(t.Context(), testUnlockPrepareTx(constants.TOKEN_ANON, pldtypes.HexUint64(types.ZetoFungibleABI_V0)), req, spendLockOperation, testUnlockLockID)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	assert.NotContains(t, params, "spendArgs")
	assert.Equal(t, testUnlockLockID.HexString0xPrefix(), params["lockId"])
	assert.NotNil(t, params["inputs"])
	assert.NotNil(t, params["outputs"])
	assert.NotNil(t, params["proof"])
}

func TestUnlockPrepare_V0NullifierPublicInputs(t *testing.T) {
	h := newTestUnlockCommon(t, testUnlockLockInfo(t), testUnlockCoins())
	req := testUnlockPrepareReq(t, map[string]string{"nullifiers": "0x09,0x0a", "root": "0x0b"})
	req.InputStates = []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(true)}}
	res, err := h.prepare(t.Context(), testUnlockPrepareTx(constants.TOKEN_ANON_NULLIFIER, pldtypes.HexUint64(types.ZetoFungibleABI_V0)), req, spendLockOperation, testUnlockLockID)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	assert.NotContains(t, params, "inputs")
	assert.Equal(t, []any{"0x09", "0x0a"}, params["nullifiers"])
	assert.Equal(t, "0x0b", params["root"])
}

func TestUnlockPrepare_Errors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		lockInfo   func(li *types.ZetoLockInfoState) *types.ZetoLockInfoState
		coins      func(coins map[string]string)
		req        func(req *prototk.PrepareTransactionRequest)
		errContent string
	}{
		{
			name:       "missing sender attestation",
			req:        func(req *prototk.PrepareTransactionRequest) { req.AttestationResult = nil },
			errContent: "PD210043",
		},
		{
			name: "invalid proof",
			req: func(req *prototk.PrepareTransactionRequest) {
				req.AttestationResult[0].Payload = []byte("not-protobuf")
			},
			errContent: "PD210044",
		},
		{
			name:       "lock not found",
			lockInfo:   func(li *types.ZetoLockInfoState) *types.ZetoLockInfoState { return nil },
			errContent: "PD210149",
		},
		{
			name:       "invalid transaction id",
			req:        func(req *prototk.PrepareTransactionRequest) { req.Transaction.TransactionId = "bad-tx-id" },
			errContent: "PD210028",
		},
		{
			name: "invalid pinned output id",
			lockInfo: func(li *types.ZetoLockInfoState) *types.ZetoLockInfoState {
				li.SpendOutputs = testCommitments(overflowCommitment)
				return li
			},
			errContent: "PD020027",
		},
		{
			name:       "pinned output missing",
			coins:      func(coins map[string]string) { delete(coins, testUnlockSpendID) },
			errContent: "PD210134",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			li := testUnlockLockInfo(t)
			if tc.lockInfo != nil {
				li = tc.lockInfo(li)
			}
			coins := testUnlockCoins()
			if tc.coins != nil {
				tc.coins(coins)
			}
			h := newTestUnlockCommon(t, li, coins)
			req := testUnlockPrepareReq(t, nil)
			if tc.req != nil {
				tc.req(req)
			}
			_, err := h.prepare(t.Context(), testUnlockPrepareTx(constants.TOKEN_ANON, pldtypes.HexUint64(types.ZetoFungibleABI_V1)), req, spendLockOperation, testUnlockLockID)
			require.ErrorContains(t, err, tc.errContent)
		})
	}
}

func testCoinJSONAmount(locked bool, amountHex string) string {
	if locked {
		return `{"salt":"0x02","owner":"` + testCoinOwnerBJJ + `","amount":"` + amountHex + `","locked":true}`
	}
	return `{"salt":"0x01","owner":"` + testCoinOwnerBJJ + `","amount":"` + amountHex + `","locked":false}`
}

type lockAssembleCallbacks struct {
	lockInfoJSON string
	coinsByID    map[string]string
}

func (c *lockAssembleCallbacks) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	if req.SchemaId == "lock_info" && c.lockInfoJSON != "" {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{{DataJson: c.lockInfoJSON}},
		}, nil
	}
	if req.SchemaId == "coin" {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{Id: "coin-in-1", DataJson: testCoinJSONAmount(false, "0x05")},
			},
		}, nil
	}
	return &prototk.FindAvailableStatesResponse{}, nil
}

func (c *lockAssembleCallbacks) GetStatesByID(ctx context.Context, req *prototk.GetStatesByIDRequest) (*prototk.GetStatesByIDResponse, error) {
	states := make([]*prototk.StoredState, 0, len(req.StateIds))
	for _, id := range req.StateIds {
		if dj, ok := c.coinsByID[id]; ok {
			states = append(states, &prototk.StoredState{Id: id, SchemaId: "coin", DataJson: dj})
		}
	}
	return &prototk.GetStatesByIDResponse{States: states}, nil
}

func (c *lockAssembleCallbacks) EncodeData(context.Context, *prototk.EncodeDataRequest) (*prototk.EncodeDataResponse, error) {
	return nil, nil
}

func (c *lockAssembleCallbacks) RecoverSigner(context.Context, *prototk.RecoverSignerRequest) (*prototk.RecoverSignerResponse, error) {
	return nil, nil
}

func (c *lockAssembleCallbacks) DecodeData(context.Context, *prototk.DecodeDataRequest) (*prototk.DecodeDataResponse, error) {
	return nil, nil
}

func (c *lockAssembleCallbacks) LocalNodeName(context.Context, *prototk.LocalNodeNameRequest) (*prototk.LocalNodeNameResponse, error) {
	return nil, nil
}

func (c *lockAssembleCallbacks) SendTransaction(context.Context, *prototk.SendTransactionRequest) (*prototk.SendTransactionResponse, error) {
	return nil, nil
}

func (c *lockAssembleCallbacks) ReverseKeyLookup(context.Context, *prototk.ReverseKeyLookupRequest) (*prototk.ReverseKeyLookupResponse, error) {
	return nil, nil
}

func (c *lockAssembleCallbacks) ValidateStates(context.Context, *prototk.ValidateStatesRequest) (*prototk.ValidateStatesResponse, error) {
	return nil, nil
}

func mockLockAssembleCallbacks(lockInfoJSON string, coinsByID map[string]string) *lockAssembleCallbacks {
	if coinsByID == nil {
		coinsByID = map[string]string{}
	}
	return &lockAssembleCallbacks{lockInfoJSON: lockInfoJSON, coinsByID: coinsByID}
}

func testLockInfoForAssemble(t *testing.T, lockID pldtypes.Bytes32, lockedID, spendID string, amount uint64) *types.ZetoLockInfoState {
	t.Helper()
	spendData, err := recipientsForLockInfoJSON(&types.CreateLockParams{
		From: "spender@node",
		Recipients: []*types.FungibleTransferParamEntry{
			{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(amount)},
		},
	}, "spender@node")
	require.NoError(t, err)
	cancelData, err := cancelRecipientsForLockInfoJSON("spender@node", pldtypes.Uint64ToUint256(amount))
	require.NoError(t, err)
	return &types.ZetoLockInfoState{
		LockID:        lockID,
		Spender:       pldtypes.MustEthAddress("0x74e71b05854ee819cb9397be01c82570a178d019"),
		LockedOutputs: testCommitments(lockedID),
		SpendOutputs:  testCommitments(spendID),
		SpendData:     spendData,
		CancelData:    cancelData,
		CancelOutputs: testCommitments(spendID),
	}
}

func testAssembleSchemas() *common.StateSchemas {
	return &common.StateSchemas{
		CoinSchema:           &prototk.StateSchema{Id: "coin"},
		MerkleTreeRootSchema: &prototk.StateSchema{Id: "merkle_tree_root"},
		MerkleTreeNodeSchema: &prototk.StateSchema{Id: "merkle_tree_node"},
		LockInfoSchema:       &prototk.StateSchema{Id: "lock_info"},
	}
}
