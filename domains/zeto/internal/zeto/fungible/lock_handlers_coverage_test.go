/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func anonLockAssembleTx(lockID pldtypes.Bytes32, cancel bool) (*types.ParsedTransaction, *prototk.AssembleTransactionRequest) {
	h := NewSpendLockHandler("zeto", nil, testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{
			From: "spender@node",
			ContractInfo: &prototk.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
		},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "Zeto_Anon",
			Circuits: &zetosignerapi.Circuits{
				types.METHOD_TRANSFER_LOCKED: {Name: "anon", UsesNullifiers: false},
			},
		},
	}
	if cancel {
		tx.Params = &types.CancelLockParams{LockId: lockID, From: "spender@node"}
	} else {
		tx.Params = &types.SpendLockParams{LockId: lockID, From: "spender@node"}
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

func marshalLockInfo(t *testing.T, li *types.ZetoLockInfoState) string {
	t.Helper()
	raw, err := json.Marshal(li)
	require.NoError(t, err)
	return string(raw)
}

func TestCancelLockAssemble_LockNotFound(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("01", 32))
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks("", nil),
		testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockAssemble_EmptyLockedOutputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("02", 32))
	li := &types.ZetoLockInfoState{LockID: lockID, Spender: "0x74e71b05854ee819cb9397be01c82570a178d019", CancelOutputs: []string{"0x" + strings.Repeat("dd", 32)}}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockAssemble_EmptySpender(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("03", 32))
	li := &types.ZetoLockInfoState{
		LockID: lockID, LockedOutputs: []string{"0x" + strings.Repeat("aa", 32)},
		CancelOutputs: []string{"0x" + strings.Repeat("dd", 32)},
	}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockAssemble_DelegateNotResolved(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("04", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 1)
	li.Spender = "delegate@node"
	li.CancelOutputs = []string{cancelID}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockAssemble_InvalidLockedOutputID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("05", 32))
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, "not-a-valid-id", cancelID, 1)
	li.CancelOutputs = []string{cancelID}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockAssemble_InvalidCancelOutputID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("06", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, "0x"+strings.Repeat("bb", 32), 1)
	li.CancelOutputs = []string{"bad-cancel-id"}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockAssemble_EmptyCancelOutputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("07", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, "0x"+strings.Repeat("bb", 32), 1)
	li.CancelOutputs = nil
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockAssemble_EmptyCancelData(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("08", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := &types.ZetoLockInfoState{
		LockID: lockID, Spender: "0x74e71b05854ee819cb9397be01c82570a178d019",
		LockedOutputs: []string{lockedID}, CancelOutputs: []string{cancelID},
	}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockAssemble_RevertLockedCoinsMissing(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("09", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 1)
	li.CancelOutputs = []string{cancelID}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
}

func TestCancelLockAssemble_RevertCancelPinnedMissing(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("0a", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 1)
	li.CancelOutputs = []string{cancelID}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
}

func TestSpendLockAssemble_LockNotFound(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("21", 32))
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks("", nil),
		testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockAssemble_EmptyLockedOutputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32))
	li := &types.ZetoLockInfoState{LockID: lockID, Spender: "0x74e71b05854ee819cb9397be01c82570a178d019", SpendOutputs: []string{"0x" + strings.Repeat("bb", 32)}}
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockAssemble_EmptySpender(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("23", 32))
	li := &types.ZetoLockInfoState{
		LockID: lockID, LockedOutputs: []string{"0x" + strings.Repeat("aa", 32)},
		SpendOutputs: []string{"0x" + strings.Repeat("bb", 32)},
	}
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockAssemble_DelegateNotResolved(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("24", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 1)
	li.Spender = "delegate@node"
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		spendID:  testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockAssemble_InvalidLockedOutputID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("25", 32))
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, "invalid", spendID, 1)
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockAssemble_InvalidSpendOutputID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("26", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, "0x"+strings.Repeat("bb", 32), 1)
	li.SpendOutputs = []string{"not-valid"}
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockAssemble_EmptySpendOutputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("27", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, "0x"+strings.Repeat("bb", 32), 1)
	li.SpendOutputs = nil
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockAssemble_EmptySpendData(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("28", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := &types.ZetoLockInfoState{
		LockID: lockID, Spender: "0x74e71b05854ee819cb9397be01c82570a178d019",
		LockedOutputs: []string{lockedID}, SpendOutputs: []string{spendID},
	}
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		spendID:  testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockAssemble_RevertLockedCoinsMissing(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("29", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 1)
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		spendID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
}

func TestSpendLockAssemble_RevertSpendPinnedMissing(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("2a", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 1)
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
}

func TestSpendLockAssemble_RevertOutputTotalMismatch(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("2b", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 5)
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x05"),
		spendID:  testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
}

func TestSpendLockProofOutputs_FromOutputStatesSkipsLockedAndSchema(t *testing.T) {
	ctx := context.Background()
	h := &spendLockHandler{
		baseHandler: baseHandler{
			stateSchemas: &common.StateSchemas{CoinSchema: &prototk.StateSchema{Id: "coin"}},
		},
	}
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("2c", 32))
	req := &prototk.PrepareTransactionRequest{
		OutputStates: []*prototk.EndorsableState{
			{SchemaId: "other", StateDataJson: testCoinStateJSON(false)},
			{StateDataJson: testCoinStateJSON(true)},
			{StateDataJson: testCoinStateJSON(false)},
		},
	}
	outs, err := h.spendLockProofOutputs(ctx, lockID, req, false, common.GetInputSize(2))
	require.NoError(t, err)
	require.NotEmpty(t, outs)
}

func TestSpendLockProofOutputs_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	h := &spendLockHandler{
		baseHandler: baseHandler{
			stateSchemas: &common.StateSchemas{CoinSchema: &prototk.StateSchema{Id: "coin"}},
		},
	}
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("2d", 32))
	req := &prototk.PrepareTransactionRequest{
		OutputStates: []*prototk.EndorsableState{{StateDataJson: "not json"}},
	}
	_, err := h.spendLockProofOutputs(ctx, lockID, req, false, 2)
	require.Error(t, err)
}

func TestSpendLockProofOutputs_PinnedSpendOutputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("2e", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 1)
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := &spendLockHandler{
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
	outs, err := h.spendLockProofOutputs(ctx, lockID, req, false, common.GetInputSize(1))
	require.NoError(t, err)
	require.NotEmpty(t, outs)
}

func TestCancelLockPrepare_MissingAttestation(t *testing.T) {
	ctx := context.Background()
	h := NewCancelLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, nil)
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("30", 32))
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon"},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	_, err := h.Prepare(ctx, tx, &prototk.PrepareTransactionRequest{Transaction: tx.Transaction})
	require.Error(t, err)
}

func TestSpendLockPrepare_MissingAttestation(t *testing.T) {
	ctx := context.Background()
	h := NewSpendLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, nil)
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("31", 32))
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon"},
		Params:       &types.SpendLockParams{LockId: lockID, From: "sender@node"},
	}
	_, err := h.Prepare(ctx, tx, &prototk.PrepareTransactionRequest{Transaction: tx.Transaction})
	require.Error(t, err)
}

func TestCancelLockAssemble_ETHDelegateDirectPath(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("32", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 1)
	li.CancelOutputs = []string{cancelID}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
}

func TestSpendLockAssemble_ETHDelegateDirectPath(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("33", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 1)
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		spendID:  testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, false)
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
}

func TestCancelLockPrepare_V0WithPinnedCancelOutputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("35", 32))
	cancelID := "0x" + strings.Repeat("dd", 32)
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{
		LockID: lockID, CancelData: cancelData, CancelOutputs: []string{cancelID},
	}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})

	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x01", "0x02"},
			B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
			C: []string{"0x07", "0x08"},
		},
	}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)

	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV0ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("36", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	res, err := h.Prepare(ctx, tx, req)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	assert.NotNil(t, params["outputs"])
}

func TestCancelLockPrepare_BadProofUnmarshal(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("42", 32))
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{LockID: lockID, CancelData: cancelData}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		&prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("43", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: []byte("not-protobuf"),
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockPrepare_InvalidCancelOutputID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("44", 32))
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{
		LockID: lockID, CancelData: cancelData, CancelOutputs: []string{"not-valid-id"},
	}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		&prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("45", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockPrepare_V0NullifierPublicInputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("46", 32))
	cancelID := "0x" + strings.Repeat("dd", 32)
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{
		LockID: lockID, CancelData: cancelData, CancelOutputs: []string{cancelID},
	}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x01", "0x02"},
			B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
			C: []string{"0x07", "0x08"},
		},
		PublicInputs: map[string]string{"nullifiers": "0x09,0x0a", "root": "0x0b"},
	}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: constants.TOKEN_ANON_NULLIFIER, ZetoVariant: types.ZetoFungibleV0ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("47", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	res, err := h.Prepare(ctx, tx, req)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	_, hasInputs := params["inputs"]
	assert.False(t, hasInputs)
	assert.Equal(t, "0x0b", params["root"])
}

func TestSpendLockPrepare_BadProofUnmarshal(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("48", 32))
	h := NewSpendLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, nil)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.SpendLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("49", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: []byte("bad"),
		}},
	}
	_, err := h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockPrepare_LockNotFound(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("4a", 32))
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks("", nil),
		&prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.SpendLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("4b", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockPrepare_V1WithOutputStatesInRequest(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("42", 32))
	spendData, err := recipientsForLockInfoJSON(&types.CreateLockParams{
		From: "spender@node",
		Recipients: []*types.FungibleTransferParamEntry{
			{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
		},
	}, "spender@node")
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{LockID: lockID, SpendData: spendData}
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		&prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "spender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.SpendLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("43", 32)},
		OutputStates: []*prototk.EndorsableState{
			{StateDataJson: testCoinStateJSON(false)},
			{StateDataJson: testCoinStateJSON(true)},
		},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.NoError(t, err)
}

func TestSpendLockPrepare_BadTxID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("45", 32))
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, "0x"+strings.Repeat("aa", 32), spendID, 1)
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		spendID: testCoinJSONAmount(false, "0x01"),
	}), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "spender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.SpendLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "bad-tx-id"},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockPrepare_V1NullifierEncodesSpendArgs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("4c", 32))
	spendID := "0x" + strings.Repeat("bb", 32)
	spendData, err := recipientsForLockInfoJSON(&types.CreateLockParams{
		From: "spender@node",
		Recipients: []*types.FungibleTransferParamEntry{
			{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
		},
	}, "spender@node")
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{LockID: lockID, SpendData: spendData, SpendOutputs: []string{spendID}}
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		spendID: testCoinJSONAmount(false, "0x01"),
	}), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x01", "0x02"},
			B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
			C: []string{"0x07", "0x08"},
		},
		PublicInputs: map[string]string{"nullifiers": "0x09,0x0a", "root": "0x0b"},
	}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "spender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: constants.TOKEN_ANON_NULLIFIER, ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.SpendLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("4d", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	res, err := h.Prepare(ctx, tx, req)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	assert.NotEmpty(t, params["spendArgs"])
}

func TestCancelLockPrepare_V1NullifierEncodesCancelArgs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("48", 32))
	cancelID := "0x" + strings.Repeat("dd", 32)
	cancelData, err := cancelRecipientsForLockInfoJSON("sender@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{
		LockID: lockID, CancelData: cancelData, CancelOutputs: []string{cancelID},
	}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x01", "0x02"},
			B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
			C: []string{"0x07", "0x08"},
		},
		PublicInputs: map[string]string{"nullifiers": "0x09,0x0a", "root": "0x0b"},
	}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: constants.TOKEN_ANON_NULLIFIER, ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("49", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	res, err := h.Prepare(ctx, tx, req)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	assert.NotEmpty(t, params["cancelArgs"])
}

func TestCancelLockPrepare_InvalidCancelOutputIDInLockInfo(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("4a", 32))
	li := &types.ZetoLockInfoState{LockID: lockID, CancelOutputs: []string{"bad-output-id"}}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		&prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("4b", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockPrepare_BadTxID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("46", 32))
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, "0x"+strings.Repeat("aa", 32), cancelID, 1)
	li.CancelOutputs = []string{cancelID}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "bad-tx-id"},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockPrepare_V0NullifierPublicInputs(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("37", 32))
	spendID := "0x" + strings.Repeat("bb", 32)
	spendData, err := recipientsForLockInfoJSON(&types.CreateLockParams{
		From: "spender@node",
		Recipients: []*types.FungibleTransferParamEntry{
			{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
		},
	}, "spender@node")
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{LockID: lockID, SpendData: spendData, SpendOutputs: []string{spendID}}
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		spendID: testCoinJSONAmount(false, "0x01"),
	}), &prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})

	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x01", "0x02"},
			B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
			C: []string{"0x07", "0x08"},
		},
		PublicInputs: map[string]string{"nullifiers": "0x09,0x0a", "root": "0x0b"},
	}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)

	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "spender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_AnonNullifier", ZetoVariant: types.ZetoFungibleV0ABI},
		Params:       &types.SpendLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("38", 32)},
		InputStates: []*prototk.EndorsableState{{StateDataJson: testCoinStateJSON(true)}},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	res, err := h.Prepare(ctx, tx, req)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Transaction.ParamsJson), &params))
	_, hasInputs := params["inputs"]
	assert.False(t, hasInputs)
	assert.Equal(t, "0x0b", params["root"])
}

func TestCancelLockPrepare_LockNotFound(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("40", 32))
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks("", nil),
		&prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("41", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockPrepare_PinnedCoinsRevert(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("39", 32))
	cancelID := "0x" + strings.Repeat("dd", 32)
	cancelData, err := cancelRecipientsForLockInfoJSON("controller@node", pldtypes.Uint64ToUint256(1))
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{
		LockID: lockID, CancelData: cancelData, CancelOutputs: []string{cancelID},
	}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		&prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.CancelLockParams{LockId: lockID, From: "sender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("3a", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestSpendLockPrepare_SpendPinnedCoinsRevert(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("3b", 32))
	spendID := "0x" + strings.Repeat("bb", 32)
	spendData, err := recipientsForLockInfoJSON(&types.CreateLockParams{
		From: "spender@node",
		Recipients: []*types.FungibleTransferParamEntry{
			{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
		},
	}, "spender@node")
	require.NoError(t, err)
	li := &types.ZetoLockInfoState{LockID: lockID, SpendData: spendData, SpendOutputs: []string{spendID}}
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), nil),
		&prototk.StateSchema{Id: "coin"}, nil, nil, nil, &prototk.StateSchema{Id: "lock_info"})
	proofReq := corepb.ProvingResponse{Proof: &corepb.SnarkProof{
		A: []string{"0x01", "0x02"},
		B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}},
		C: []string{"0x07", "0x08"},
	}}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "spender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.SpendLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("3c", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.Error(t, err)
}

func TestCancelLockValidateParams_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	h := NewCancelLockHandler("zeto", nil, nil, nil, nil, nil, nil)
	_, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, "{bad")
	require.Error(t, err)
}

func TestSpendLockValidateParams_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	h := NewSpendLockHandler("zeto", nil, nil, nil, nil, nil, nil)
	_, err := h.ValidateParams(ctx, &types.DomainInstanceConfig{}, "{bad")
	require.Error(t, err)
}

func TestSpendRecipientsFromLockInfo_Errors(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("3d", 32))
	_, err := spendRecipientsFromLockInfo(ctx, &types.ZetoLockInfoState{LockID: lockID})
	require.Error(t, err)
	_, err = spendRecipientsFromLockInfo(ctx, &types.ZetoLockInfoState{LockID: lockID, SpendData: []byte("not-json")})
	require.Error(t, err)
	_, err = spendRecipientsFromLockInfo(ctx, &types.ZetoLockInfoState{LockID: lockID, SpendData: []byte("[]")})
	require.Error(t, err)
}

func TestCreateLockAssemble_MissingRecipientVerifier(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", mockLockAssembleCallbacks("", nil),
		testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, testAssembleSchemas().LockInfoSchema)
	txSpec := &prototk.TransactionSpecification{
		From:          "controller@node",
		TransactionId: "0x" + strings.Repeat("58", 32),
		ContractInfo:  &prototk.ContractInfo{ContractAddress: "0x1234567890123456789012345678901234567890"},
	}
	tx := &types.ParsedTransaction{
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "Zeto_Anon",
			Circuits:  &zetosignerapi.Circuits{types.METHOD_TRANSFER: {Name: "anon", UsesNullifiers: false}},
		},
		Params: &types.CreateLockParams{
			From: "controller@node",
			Recipients: []*types.FungibleTransferParamEntry{
				{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
			},
		},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: txSpec,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{Lookup: "controller@node", Verifier: testCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
			{Lookup: "controller@node", Verifier: "0x74e71b05854ee819cb9397be01c82570a178d019", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
		},
	}
	_, err := h.Assemble(ctx, tx, req)
	require.Error(t, err)
}

func TestCreateLockAssemble_NullifierWithRemainder(t *testing.T) {
	ctx := context.Background()
	cb := newMerkleNullifierCallbacks("coin", merkleUTXOCoinOwnerBJJ)
	h := NewCreateLockHandler("zeto", cb,
		testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema,
		testAssembleSchemas().MerkleTreeNodeSchema, testAssembleSchemas().LockInfoSchema)
	txSpec := &prototk.TransactionSpecification{
		From:          "controller@node",
		TransactionId: "0x" + strings.Repeat("5b", 32),
		ContractInfo:  &prototk.ContractInfo{ContractAddress: "0x1234567890123456789012345678901234567890"},
	}
	tx := &types.ParsedTransaction{
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: constants.TOKEN_ANON_NULLIFIER,
			Circuits:  &zetosignerapi.Circuits{types.METHOD_TRANSFER: {Name: "anon", UsesNullifiers: true}},
		},
		Params: &types.CreateLockParams{
			From: "controller@node",
			Recipients: []*types.FungibleTransferParamEntry{
				{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
			},
		},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: txSpec,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{Lookup: "controller@node", Verifier: merkleUTXOCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
			{Lookup: "controller@node", Verifier: "0x74e71b05854ee819cb9397be01c82570a178d019", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
			{Lookup: "recipient@node", Verifier: merkleUTXOCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
		},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	assert.Greater(t, len(res.AssembledTransaction.OutputStates), 2)
}

func TestCreateLockAssemble_NullifierTokenSuccess(t *testing.T) {
	ctx := context.Background()
	cb := newMerkleNullifierCallbacks("coin", merkleUTXOCoinOwnerBJJ)
	h := NewCreateLockHandler("zeto", cb,
		testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema,
		testAssembleSchemas().MerkleTreeNodeSchema, testAssembleSchemas().LockInfoSchema)
	txSpec := &prototk.TransactionSpecification{
		From:          "controller@node",
		TransactionId: "0x" + strings.Repeat("5a", 32),
		ContractInfo:  &prototk.ContractInfo{ContractAddress: "0x1234567890123456789012345678901234567890"},
	}
	tx := &types.ParsedTransaction{
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: constants.TOKEN_ANON_NULLIFIER,
			Circuits:  &zetosignerapi.Circuits{types.METHOD_TRANSFER: {Name: "anon", UsesNullifiers: true}},
		},
		Params: &types.CreateLockParams{
			From: "controller@node",
			Recipients: []*types.FungibleTransferParamEntry{
				{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)},
			},
		},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: txSpec,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{Lookup: "controller@node", Verifier: merkleUTXOCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
			{Lookup: "controller@node", Verifier: "0x74e71b05854ee819cb9397be01c82570a178d019", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
			{Lookup: "recipient@node", Verifier: merkleUTXOCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
		},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	assert.NotEmpty(t, res.AttestationPlan)
}

func TestCreateLockAssemble_RevertInsufficientInputs(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", mockLockAssembleCallbacks("", map[string]string{
		"coin-in-1": testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, testAssembleSchemas().LockInfoSchema)
	txSpec := &prototk.TransactionSpecification{
		From:          "controller@node",
		TransactionId: "0x" + strings.Repeat("3f", 32),
		ContractInfo:  &prototk.ContractInfo{ContractAddress: "0x1234567890123456789012345678901234567890"},
	}
	tx := &types.ParsedTransaction{
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "Zeto_Anon",
			Circuits:  &zetosignerapi.Circuits{types.METHOD_TRANSFER: {Name: "anon", UsesNullifiers: false}},
		},
		Params: &types.CreateLockParams{
			From: "controller@node",
			Recipients: []*types.FungibleTransferParamEntry{
				nil,
				{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(100)},
			},
		},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: txSpec,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{Lookup: "controller@node", Verifier: testCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
			{Lookup: "controller@node", Verifier: "0x74e71b05854ee819cb9397be01c82570a178d019", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
			{Lookup: "recipient@node", Verifier: testCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
		},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
}

func TestCancelLockAssemble_IdentityDelegateWithEthVerifier(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("34", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 1)
	li.Spender = "spender@node"
	li.CancelOutputs = []string{cancelID}
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(marshalLockInfo(t, li), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)
	tx, req := anonLockAssembleTx(lockID, true)
	req.ResolvedVerifiers = append(req.ResolvedVerifiers, &prototk.ResolvedVerifier{
		Lookup: "spender@node", Verifier: "0x74e71b05854ee819cb9397be01c82570a178d019",
		Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS,
	})
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
}

func TestSpendPinnedOutputStatesForNullifiers_Success(t *testing.T) {
	ctx := context.Background()
	spendID := "0x" + strings.Repeat("ef", 32)
	coin, err := makeCoin(testCoinJSONAmount(false, "0x01"))
	require.NoError(t, err)
	recipients := []*types.FungibleTransferParamEntry{{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)}}
	states, err := spendPinnedOutputStatesForNullifiers(ctx, &prototk.StateSchema{Id: "coin"}, "zeto",
		[]string{spendID}, []*types.ZetoCoin{coin}, recipients)
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.Equal(t, spendID, *states[0].Id)
}

func TestSpendPinnedOutputStatesForNullifiers_LengthMismatch(t *testing.T) {
	ctx := context.Background()
	coin, err := makeCoin(testCoinJSONAmount(false, "0x01"))
	require.NoError(t, err)
	_, err = spendPinnedOutputStatesForNullifiers(ctx, &prototk.StateSchema{Id: "coin"}, "zeto",
		[]string{"0x" + strings.Repeat("ef", 32), "0x" + strings.Repeat("ee", 32)},
		[]*types.ZetoCoin{coin},
		[]*types.FungibleTransferParamEntry{{To: "recipient@node", Amount: pldtypes.Uint64ToUint256(1)}})
	require.Error(t, err)
}

func TestSpendLockProofOutputs_BadOutputStateJSON(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("44", 32))
	h := NewSpendLockHandler("zeto", nil, &prototk.StateSchema{Id: "coin"}, nil, nil, nil, nil)
	req := &prototk.PrepareTransactionRequest{
		OutputStates: []*prototk.EndorsableState{{StateDataJson: "bad-json", SchemaId: "coin"}},
	}
	_, err := h.spendLockProofOutputs(ctx, lockID, req, false, 2)
	require.Error(t, err)
}
