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
		Spender:       "0x74e71b05854ee819cb9397be01c82570a178d019",
		LockedOutputs: []string{lockedID},
		SpendOutputs:  []string{spendID},
		SpendData:     spendData,
		CancelData:    cancelData,
		CancelOutputs: []string{spendID},
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

func nullifierLockAssembleTx(t *testing.T, lockID pldtypes.Bytes32, forCancel bool) (*types.ParsedTransaction, *prototk.AssembleTransactionRequest) {
	t.Helper()
	h := NewSpendLockHandler("zeto", nil, testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{
			From: "spender@node",
			ContractInfo: &prototk.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
		},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: constants.TOKEN_ANON_NULLIFIER,
			Circuits: &zetosignerapi.Circuits{
				types.METHOD_TRANSFER_LOCKED: {Name: "anon", UsesNullifiers: false},
			},
		},
	}
	if forCancel {
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

func TestSpendLockAssemble_NullifierTokenBuildsOutputStates(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("33", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 1)
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		spendID:  testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)

	tx, req := nullifierLockAssembleTx(t, lockID, false)
	req.Transaction = tx.Transaction

	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	require.NotEmpty(t, res.AssembledTransaction.OutputStates)
}

func TestCancelLockAssemble_NullifierTokenBuildsOutputStates(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("44", 32))
	lockedID := "0x" + strings.Repeat("cc", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 1)
	li.CancelOutputs = []string{cancelID}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)

	tx, req := nullifierLockAssembleTx(t, lockID, true)
	req.Transaction = tx.Transaction

	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	require.NotEmpty(t, res.AssembledTransaction.OutputStates)
}

func TestSpendLockAssemble_RevertInsufficientInput(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("77", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 100)
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		spendID:  testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)

	tx, req := nullifierLockAssembleTx(t, lockID, false)
	tx.DomainConfig.TokenName = "Zeto_Anon"
	req.Transaction = tx.Transaction

	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
	require.NotNil(t, res.RevertReason)
}

func TestSpendLockAssemble_DelegateIdentityResolvedToEth(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("88", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 1)
	li.Spender = "spender@node"
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		spendID:  testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)

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
		Params: &types.SpendLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: tx.Transaction,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup: "spender@node", Verifier: testCoinOwnerBJJ,
				Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
			{
				Lookup: "spender@node", Verifier: "0x74e71b05854ee819cb9397be01c82570a178d019",
				Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
}

func TestSpendLockAssemble_Errors(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("99", 32))
	h := NewSpendLockHandler("zeto", nil, testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{From: "spender@node"},
		Params:      &types.SpendLockParams{LockId: lockID, From: "spender@node"},
	}
	_, err := h.Assemble(ctx, tx, &prototk.AssembleTransactionRequest{Transaction: tx.Transaction})
	require.Error(t, err)

	tx.DomainConfig = &types.DomainInstanceConfig{TokenName: "Zeto_Anon"}
	h2 := NewSpendLockHandler("zeto", nil, testAssembleSchemas().CoinSchema, nil, nil, nil, nil)
	_, err = h2.Assemble(ctx, tx, &prototk.AssembleTransactionRequest{
		Transaction: tx.Transaction,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{{
			Lookup: "spender@node", Verifier: testCoinOwnerBJJ,
			Algorithm: h2.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		}},
	})
	require.Error(t, err)
}

func TestCancelLockAssemble_DelegateIdentityResolvedToEth(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("bb", 32))
	lockedID := "0x" + strings.Repeat("cc", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 1)
	li.Spender = "spender@node"
	li.CancelOutputs = []string{cancelID}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)

	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{
			From: "spender@node",
			ContractInfo: &prototk.ContractInfo{ContractAddress: "0x1234567890123456789012345678901234567890"},
		},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "Zeto_Anon",
			Circuits:  &zetosignerapi.Circuits{types.METHOD_TRANSFER_LOCKED: {Name: "anon", UsesNullifiers: false}},
		},
		Params: &types.CancelLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: tx.Transaction,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{Lookup: "spender@node", Verifier: testCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
			{Lookup: "spender@node", Verifier: "0x74e71b05854ee819cb9397be01c82570a178d019", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
		},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
}

func TestCancelLockAssemble_RevertOutputTotalMismatch(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("bc", 32))
	lockedID := "0x" + strings.Repeat("cc", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 5)
	li.CancelOutputs = []string{cancelID}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x05"),
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)

	tx, req := nullifierLockAssembleTx(t, lockID, true)
	tx.DomainConfig.TokenName = "Zeto_Anon"
	req.Transaction = tx.Transaction

	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
}

func TestCancelLockAssemble_RevertInsufficientInput(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("bb", 32))
	lockedID := "0x" + strings.Repeat("cc", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 50)
	li.CancelOutputs = []string{cancelID}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)

	tx, req := nullifierLockAssembleTx(t, lockID, true)
	tx.DomainConfig.TokenName = "Zeto_Anon"
	req.Transaction = tx.Transaction

	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
}

func TestCancelLockAssemble_Errors(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("aa", 32))
	h := NewCancelLockHandler("zeto", nil, testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{From: "spender@node"},
		Params:      &types.CancelLockParams{LockId: lockID, From: "spender@node"},
	}
	_, err := h.Assemble(ctx, tx, &prototk.AssembleTransactionRequest{Transaction: tx.Transaction})
	require.Error(t, err)
}

func TestSpendLockAssemble_Success(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("11", 32))
	lockedID := "0x" + strings.Repeat("aa", 32)
	spendID := "0x" + strings.Repeat("bb", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 1)
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		spendID:  testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)

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
		Params: &types.SpendLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: tx.Transaction,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{{
			Lookup: "spender@node", Verifier: testCoinOwnerBJJ,
			Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		}},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	require.NotEmpty(t, res.AttestationPlan)
}

func TestCancelLockAssemble_Success(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32))
	lockedID := "0x" + strings.Repeat("cc", 32)
	cancelID := "0x" + strings.Repeat("dd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, cancelID, 1)
	li.CancelOutputs = []string{cancelID}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		lockedID: testCoinJSONAmount(true, "0x01"),
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, nil, testAssembleSchemas().LockInfoSchema)

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
		Params: &types.CancelLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: tx.Transaction,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{{
			Lookup: "spender@node", Verifier: testCoinOwnerBJJ,
			Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		}},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
}

type createLockRemainderCallbacks struct {
	lockAssembleCallbacks
}

func (c *createLockRemainderCallbacks) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	if req.SchemaId == "coin" {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{{
				Id: "coin-in-1", DataJson: testCoinJSONAmount(false, "0x05"),
			}},
		}, nil
	}
	return &prototk.FindAvailableStatesResponse{}, nil
}

func TestCreateLockAssemble_WithRemainderOutput(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", &createLockRemainderCallbacks{},
		testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, testAssembleSchemas().LockInfoSchema)

	txSpec := &prototk.TransactionSpecification{
		From:          "controller@node",
		TransactionId: "0x" + strings.Repeat("99", 32),
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
		},
	}
	tx := &types.ParsedTransaction{
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "Zeto_Anon",
			Circuits: &zetosignerapi.Circuits{
				types.METHOD_TRANSFER: {Name: "anon", UsesNullifiers: false},
			},
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
			{Lookup: "recipient@node", Verifier: testCoinOwnerBJJ, Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
		},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	require.Greater(t, len(res.AssembledTransaction.OutputStates), 2)
}

func TestCreateLockAssemble_Success(t *testing.T) {
	ctx := context.Background()
	h := NewCreateLockHandler("zeto", mockLockAssembleCallbacks("", nil),
		testAssembleSchemas().CoinSchema, testAssembleSchemas().MerkleTreeRootSchema, testAssembleSchemas().MerkleTreeNodeSchema, testAssembleSchemas().LockInfoSchema)

	txSpec := &prototk.TransactionSpecification{
		From:          "controller@node",
		TransactionId: "0x" + strings.Repeat("88", 32),
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
		},
	}
	tx := &types.ParsedTransaction{
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "Zeto_Anon",
			Circuits: &zetosignerapi.Circuits{
				types.METHOD_TRANSFER: {Name: "anon", UsesNullifiers: false},
			},
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
			{
				Lookup: "controller@node", Verifier: testCoinOwnerBJJ,
				Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
			{
				Lookup: "controller@node", Verifier: "0x74e71b05854ee819cb9397be01c82570a178d019",
				Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS,
			},
			{
				Lookup: "recipient@node", Verifier: testCoinOwnerBJJ,
				Algorithm: h.getAlgoZetoSnarkBJJ(), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	assert.NotEmpty(t, res.AttestationPlan)
}

func TestLoadCoinStatesByIDs_RevertWhenMissing(t *testing.T) {
	ctx := context.Background()
	id := "0x" + strings.Repeat("ee", 32)
	cb := mockLockAssembleCallbacks("", map[string]string{})
	_, _, revert, err := loadCoinStatesByIDs(ctx, cb, &prototk.StateSchema{Id: "coin"}, false, "ctx", []string{id}, true)
	require.Error(t, err)
	assert.True(t, revert)
}

func TestLoadCoinStatesByIDs_SpendOutputLockedReverts(t *testing.T) {
	ctx := context.Background()
	id := "0x" + strings.Repeat("ee", 32)
	cb := mockLockAssembleCallbacks("", map[string]string{id: testCoinJSONAmount(true, "0x01")})
	_, _, revert, err := loadCoinStatesByIDs(ctx, cb, &prototk.StateSchema{Id: "coin"}, false, "ctx", []string{id}, false)
	require.Error(t, err)
	assert.True(t, revert)
}

func TestLoadCoinStatesByIDs_InputNotLockedReverts(t *testing.T) {
	ctx := context.Background()
	id := "0x" + strings.Repeat("ef", 32)
	cb := mockLockAssembleCallbacks("", map[string]string{id: testCoinJSONAmount(false, "0x01")})
	_, _, revert, err := loadCoinStatesByIDs(ctx, cb, &prototk.StateSchema{Id: "coin"}, false, "ctx", []string{id}, true)
	require.Error(t, err)
	assert.True(t, revert)
}

func TestLoadCoinStatesByIDs_UnmarshalErrorReverts(t *testing.T) {
	ctx := context.Background()
	id := "0x" + strings.Repeat("f0", 32)
	cb := mockLockAssembleCallbacks("", map[string]string{id: "not-json"})
	_, _, revert, err := loadCoinStatesByIDs(ctx, cb, &prototk.StateSchema{Id: "coin"}, false, "ctx", []string{id}, true)
	require.Error(t, err)
	assert.True(t, revert)
}

func TestLoadCoinStatesByIDs(t *testing.T) {
	ctx := context.Background()
	id := "0x" + strings.Repeat("ee", 32)
	cb := mockLockAssembleCallbacks("", map[string]string{id: testCoinJSONAmount(true, "0x01")})
	prepared, stored, revert, err := loadCoinStatesByIDs(ctx, cb, &prototk.StateSchema{Id: "coin"}, false, "ctx", []string{id}, true)
	require.NoError(t, err)
	assert.False(t, revert)
	require.Len(t, prepared.coins, 1)
	require.Len(t, stored, 1)
	assert.True(t, prepared.coins[0].Locked)
}

func TestSpendLockPrepare_PinnedOutputsPath(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("99", 32))
	lockedID := "0x" + strings.Repeat("ab", 32)
	spendID := "0x" + strings.Repeat("cd", 32)
	li := testLockInfoForAssemble(t, lockID, lockedID, spendID, 1)
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)

	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		spendID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)

	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{A: []string{"0x01", "0x02"}, B: []*corepb.B_Item{{Items: []string{"0x03", "0x04"}}, {Items: []string{"0x05", "0x06"}}}, C: []string{"0x07", "0x08"}},
	}
	payload, err := proto.Marshal(&proofReq)
	require.NoError(t, err)

	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "spender@node"},
		DomainConfig: &types.DomainInstanceConfig{TokenName: "Zeto_Anon", ZetoVariant: types.ZetoFungibleV1ABI},
		Params:       &types.SpendLockParams{LockId: lockID, From: "spender@node"},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "0x" + strings.Repeat("77", 32)},
		AttestationResult: []*prototk.AttestationResult{{
			Name: "sender", AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType: strPtr(zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK), Payload: payload,
		}},
	}
	_, err = h.Prepare(ctx, tx, req)
	require.NoError(t, err)
}

func TestSpendPinnedCoinsForPrepare_Success(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("99", 32))
	spendID := "0x" + strings.Repeat("cd", 32)
	li := testLockInfoForAssemble(t, lockID, "0x"+strings.Repeat("ab", 32), spendID, 1)
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		spendID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)

	coins, err := h.spendPinnedCoinsForPrepare(ctx, lockID, "ctx", false)
	require.NoError(t, err)
	require.Len(t, coins, 1)
}

func TestSpendPinnedCoinsForPrepare_InvalidSpendOutputID(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("98", 32))
	li := &types.ZetoLockInfoState{
		LockID:       lockID,
		SpendOutputs: []string{"not-a-valid-coin-id"},
	}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)
	h := NewSpendLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), nil),
		testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)

	_, err = h.spendPinnedCoinsForPrepare(ctx, lockID, "ctx", false)
	require.Error(t, err)
}

func TestCancelPinnedCoinsForPrepare_Success(t *testing.T) {
	ctx := context.Background()
	lockID := pldtypes.MustParseBytes32("0x" + strings.Repeat("97", 32))
	cancelID := "0x" + strings.Repeat("de", 32)
	li := testLockInfoForAssemble(t, lockID, "0x"+strings.Repeat("ab", 32), cancelID, 1)
	li.CancelOutputs = []string{cancelID}
	lockInfoJSON, err := json.Marshal(li)
	require.NoError(t, err)
	h := NewCancelLockHandler("zeto", mockLockAssembleCallbacks(string(lockInfoJSON), map[string]string{
		cancelID: testCoinJSONAmount(false, "0x01"),
	}), testAssembleSchemas().CoinSchema, nil, nil, nil, testAssembleSchemas().LockInfoSchema)

	coins, err := h.pinnedCoinsForPrepare(ctx, lockID, li.CancelOutputs, "ctx", false)
	require.NoError(t, err)
	require.Len(t, coins, 1)
}
