/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package helpers

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/pkg/testbed"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zetoDelegateLockArgsTupleABI matches IZetoLockableCapability.ZetoDelegateLockArgs — ABI-encoded delegateArgs
// for delegateLock(bytes32,bytes,address,bytes) on zeto-contracts ~v0.5.x.
var zetoDelegateLockArgsTupleABI = abi.ParameterArray{
	{
		Type:         "tuple",
		InternalType: "struct ZetoDelegateLockArgs",
		Components: abi.ParameterArray{
			{Name: "txId", Type: "bytes32"},
		},
	},
}

//go:embed abis/SampleERC20.json
var erc20ABI []byte

type ZetoHelper struct {
	t       *testing.T
	rpc     rpcclient.Client
	Address *pldtypes.EthAddress
	// implABI is the on-chain pool implementation ABI for public transactions (fungible helpers).
	implABI abi.ABI
}

// ZetoVersionLatest is the default primary zkp tree (see ZetoZKArtifactRootLatest).
const ZetoVersionLatest = ZetoZKArtifactRootLatest

// ZetoZKArtifactsDir returns the circuits/proving-keys root for integration tests (cwd = domains/integration-test).
func ZetoZKArtifactsDir(version string) string {
	if version == "" || strings.EqualFold(strings.TrimSpace(version), "latest") {
		version = ZetoZKArtifactRootLatest
	}
	return filepath.Join("..", "zeto", "zkp", version)
}

// =============================================================================
//
//	Fungible
//
// =============================================================================
type ZetoHelperFungible struct {
	ZetoHelper
}

func DeployZetoFungible(ctx context.Context, t *testing.T, rpc rpcclient.Client, domainName, controllerName, tokenName, zkpArtifactRoot string) *ZetoHelperFungible {
	zkpRoot := strings.TrimSpace(zkpArtifactRoot)
	if zkpRoot == "" {
		zkpRoot = EffectiveZetoZKArtifactRoot()
	}
	abiPath := ResolveZetoImplementationAbiPath(filepath.Join(".", "helpers", "abis", tokenName+".json"), zkpRoot)
	raw, err := os.ReadFile(abiPath)
	require.NoError(t, err)
	// ABI only — MustLoadBuild would require Poseidon (and other) library addresses from linkReferences.
	implABI := solutils.MustParseBuildABI(raw)

	var addr pldtypes.EthAddress
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy", domainName, controllerName, &types.InitializerParams{
		TokenName: tokenName,
		Name:      "Test Zeto",
		Symbol:    "ZETO",
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr)
	}
	return &ZetoHelperFungible{
		ZetoHelper: ZetoHelper{
			t:       t,
			rpc:     rpc,
			Address: &addr,
			implABI: implABI,
		},
	}
}

// DeployZetoFungibleV1 deploys a fungible V1 pool (ZetoFactoryV1 + on-chain domain config schema v1 + IZetoFungible_V1 handler axis).
// zkpArtifactRoot should be helpers.ZetoZKArtifactRootV050 (e.g. "v0.5.0") so implementation ABIs resolve under helpers/abis/zkp/<root>/.
func DeployZetoFungibleV1(ctx context.Context, t *testing.T, rpc rpcclient.Client, domainName, controllerName, tokenName, zkpArtifactRoot string) *ZetoHelperFungible {
	zkpRoot := strings.TrimSpace(zkpArtifactRoot)
	if zkpRoot == "" {
		zkpRoot = EffectiveZetoZKArtifactRoot()
	}
	abiPath := ResolveZetoImplementationAbiPath(filepath.Join(".", "helpers", "abis", tokenName+".json"), zkpRoot)
	raw, err := os.ReadFile(abiPath)
	require.NoError(t, err)
	implABI := solutils.MustParseBuildABI(raw)

	var addr pldtypes.EthAddress
	init := &types.InitializerParams{
		TokenName:          tokenName,
		Name:               "Test Zeto",
		Symbol:             "ZETO",
		DomainConfigSchema: types.DomainConfigSchemaV1,
		ZetoVariant:        uint64(types.ZetoFungibleV1ABI),
		FactoryVersion:     int64(types.ZetoPaladinFactoryV1),
	}
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy", domainName, controllerName, init)
	if rpcerr != nil {
		assert.NoError(t, rpcerr)
	}
	return &ZetoHelperFungible{
		ZetoHelper: ZetoHelper{
			t:       t,
			rpc:     rpc,
			Address: &addr,
			implABI: implABI,
		},
	}
}

// DecodeTransactionInvokeResult unmarshals the map returned by SentDomainTransaction.Wait() after testbed_invoke
// into a TransactionResult (same wire shape as testbed_prepare).
func DecodeTransactionInvokeResult(t *testing.T, invokeWait map[string]any) *testbed.TransactionResult {
	t.Helper()
	b, err := json.Marshal(invokeWait)
	require.NoError(t, err)
	var tr testbed.TransactionResult
	require.NoError(t, json.Unmarshal(b, &tr))
	return &tr
}

// ZetoLockIDFromCreateLockInvoke returns the lockId from the ZetoLockInfoState output produced by createLock.
// spendLock loads lock info by this id; prefer this over recomputing keccak(pool, msg.sender, txId) when the public
// submitter identity is not known at test time.
func ZetoLockIDFromCreateLockInvoke(t *testing.T, tr *testbed.TransactionResult) pldtypes.Bytes32 {
	t.Helper()
	li := ZetoLockInfoFromCreateLockInvoke(t, tr)
	return li.LockID
}

// ZetoLockInfoFromCreateLockInvoke returns the persisted lock-info state from a completed createLock invoke.
func ZetoLockInfoFromCreateLockInvoke(t *testing.T, tr *testbed.TransactionResult) *types.ZetoLockInfoState {
	t.Helper()
	require.NotNil(t, tr)
	for _, st := range append(tr.InfoStates, tr.OutputStates...) {
		if st == nil || len(st.Data) == 0 {
			continue
		}
		var li types.ZetoLockInfoState
		if err := json.Unmarshal(st.Data, &li); err != nil {
			continue
		}
		if li.LockID.IsZero() {
			continue
		}
		return &li
	}
	require.Fail(t, "no ZetoLockInfoState found in createLock info/output states")
	return nil
}

// ZetoLockCreatedEvent is the non-indexed payload for ILockableCapability.ZetoLockCreated (v0.5.x).
type ZetoLockCreatedEvent struct {
	LockId pldtypes.Bytes32 `json:"lockId"`
}

// createLockReceiptPublicTxHash returns the base-ledger transaction hash that emitted ZetoLockCreated for a createLock invoke.
func createLockReceiptPublicTxHash(ctx context.Context, t *testing.T, rpc rpcclient.Client, tr *testbed.TransactionResult) *pldtypes.Bytes32 {
	t.Helper()
	require.NotNil(t, tr)
	var receipt pldapi.TransactionReceipt
	rpcerr := rpc.CallRPC(ctx, &receipt, "ptx_getTransactionReceipt", tr.ID)
	require.Nil(t, rpcerr)
	require.True(t, receipt.Success, "createLock receipt not successful")
	if receipt.TransactionHash != nil && !receipt.TransactionHash.IsZero() {
		return receipt.TransactionHash
	}
	var receiptFull pldapi.TransactionReceiptFull
	rpcerr = rpc.CallRPC(ctx, &receiptFull, "ptx_getTransactionReceiptFull", tr.ID)
	require.Nil(t, rpcerr)
	for _, pub := range receiptFull.Public {
		if pub != nil && pub.TransactionHash != nil && !pub.TransactionHash.IsZero() {
			return pub.TransactionHash
		}
	}
	require.Fail(t, "no public transaction hash on createLock receipt %s", tr.ID)
	return nil
}

// ZetoLockIDsFromCreateLockReceipt returns the lockId from ZetoLockCreated and the value computed as
// keccak256(abi.encode(pool, publicCreateLockMsgSender, txId)) using the Paladin createLock transaction id.
func ZetoLockIDsFromCreateLockReceipt(ctx context.Context, t *testing.T, rpc rpcclient.Client, tb testbed.Testbed, z *ZetoHelper, tr *testbed.TransactionResult) (chainLockID, calculatedLockID pldtypes.Bytes32) {
	t.Helper()
	require.NotNil(t, tr)
	require.NotNil(t, z)
	require.NotNil(t, tr.PreparedTransaction)
	require.NotEmpty(t, tr.PreparedTransaction.From)
	publicTxHash := createLockReceiptPublicTxHash(ctx, t, rpc, tr)
	chainLockID = ZetoLockIDFromZetoLockCreatedEvent(ctx, t, tb, z.implABI, publicTxHash)
	publicEth := ResolveVerifierEthAddress(ctx, t, rpc, tr.PreparedTransaction.From)
	calculatedLockID = ComputeZetoFungibleV1LockID(z.Address, publicEth, tr.ID)
	return chainLockID, calculatedLockID
}

// AssertCreateLockChainLockIDMatchesCalculated decodes ZetoLockCreated.lockId and requires it to match the
// keccak256(pool, publicCreateLockMsgSender, paladinTxId) formula. Returns the on-chain lockId for spendLock.
func AssertCreateLockChainLockIDMatchesCalculated(ctx context.Context, t *testing.T, rpc rpcclient.Client, tb testbed.Testbed, z *ZetoHelper, tr *testbed.TransactionResult) pldtypes.Bytes32 {
	t.Helper()
	chainLockID, calculatedLockID := ZetoLockIDsFromCreateLockReceipt(ctx, t, rpc, tb, z, tr)
	persistedLockID := ZetoLockIDFromCreateLockInvoke(t, tr)
	assert.Equal(t, calculatedLockID.HexString0xPrefix(), chainLockID.HexString0xPrefix(),
		"ZetoLockCreated.lockId must match keccak256(abi.encode(pool, publicCreateLockMsgSender, txId)); pool=%s publicSubmitter=%s paladinTxId=%s",
		z.Address, tr.PreparedTransaction.From, tr.ID)
	assert.Equal(t, chainLockID.HexString0xPrefix(), persistedLockID.HexString0xPrefix(),
		"persisted ZetoLockInfoState.lockId must match on-chain ZetoLockCreated.lockId")
	return chainLockID
}

// ZetoLockIDFromCreateLockConfirmed returns the on-chain ZetoLockCreated.lockId after a completed createLock invoke.
func ZetoLockIDFromCreateLockConfirmed(ctx context.Context, t *testing.T, rpc rpcclient.Client, tb testbed.Testbed, z *ZetoHelper, tr *testbed.TransactionResult) pldtypes.Bytes32 {
	t.Helper()
	return AssertCreateLockChainLockIDMatchesCalculated(ctx, t, rpc, tb, z, tr)
}

// ZetoLockIDFromZetoLockCreatedEvent decodes ZetoLockCreated from a confirmed public createLock transaction.
func ZetoLockIDFromZetoLockCreatedEvent(ctx context.Context, t *testing.T, tb testbed.Testbed, implABI abi.ABI, publicTxHash *pldtypes.Bytes32) pldtypes.Bytes32 {
	t.Helper()
	th := NewTransactionHelper(ctx, t, tb, nil)
	var ev ZetoLockCreatedEvent
	found := th.FindEvent(publicTxHash, implABI, "ZetoLockCreated", &ev)
	require.NotNil(t, found, "ZetoLockCreated event not found in tx %s", publicTxHash)
	require.False(t, ev.LockId.IsZero())
	return ev.LockId
}

// ZetoLockedCoinIDFromCreateLockInvoke returns the UTXO commitment id of the locked coin from createLock output states.
// spendLock resolves coins via spendLockedOutputs in lock info (same commitment strings).
func ZetoLockedCoinIDFromCreateLockInvoke(t *testing.T, tr *testbed.TransactionResult) *pldtypes.HexUint256 {
	t.Helper()
	require.NotNil(t, tr)
	var locked *pldtypes.HexUint256
	for _, st := range tr.OutputStates {
		if st == nil || len(st.Data) == 0 {
			continue
		}
		var coin types.ZetoCoin
		require.NoError(t, json.Unmarshal(st.Data, &coin))
		if !coin.Locked {
			continue
		}
		require.Nil(t, locked, "expected exactly one locked coin output from createLock")
		// Use Poseidon commitment from coin fields (same as domain utxosFromOutputStates / spendLock proof),
		// not raw st.ID bytes, so spendLock spendLockedOutputs match the proof's input commitments.
		h, err := coin.Hash(context.Background())
		require.NoError(t, err)
		v := *h
		locked = &v
	}
	require.NotNil(t, locked, "no locked coin found in createLock output states")
	return locked
}

// ResolveVerifierEthAddress resolves a Paladin identity (e.g. controller@node1 or testbed.onetime.*) to an ETH address.
func ResolveVerifierEthAddress(ctx context.Context, t *testing.T, rpc rpcclient.Client, verifierLookup string) pldtypes.EthAddress {
	t.Helper()
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "ptx_resolveVerifier", verifierLookup, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)
	return *pldtypes.MustEthAddress(addr)
}

// ComputeZetoFungibleV1LockID returns keccak256(abi.encode(pool, publicCreateLockMsgSender, txId)) for ZetoLockable v0.5.x
// (see zeto_lockable.sol _computeLockId). publicCreateLockMsgSender must be the ETH address that submitted the on-chain
// createLock (PreparedPublicTransaction.From), not necessarily the Paladin BJJ sender. txId is Bytes32UUIDFirst16(createLockTxID).
func ComputeZetoFungibleV1LockID(pool *pldtypes.EthAddress, publicCreateLockMsgSender pldtypes.EthAddress, createLockTxID uuid.UUID) pldtypes.Bytes32 {
	txID := pldtypes.Bytes32UUIDFirst16(createLockTxID)
	return types.ComputeZetoLockIDV1(*pool, publicCreateLockMsgSender, txID)
}

// CreateLock builds a domain tx for IZetoFungible_V1.createLock(from, recipients, unlockData, data).
// from is the Paladin identity that signs the private leg; to is the spend recipient pinned in lock info (Recipients[].To).
// unlockData is application-specific opaque bytes stored on lock info and passed through to on-chain outer data.
func (z *ZetoHelperFungible) CreateLock(ctx context.Context, from, to string, amount int, unlockData pldtypes.HexBytes) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI_V1.Functions()[types.METHOD_CREATE_LOCK]
	emptyData := pldtypes.HexBytes{}
	if unlockData == nil {
		unlockData = emptyData
	}
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.CreateLockParams{
		From: from,
		Recipients: []*types.FungibleTransferParamEntry{
			{To: to, Amount: pldtypes.Uint64ToUint256(uint64(amount)), Data: emptyData},
		},
		UnlockData: unlockData,
		Data:       emptyData,
	}))
}

// SpendLockRequest groups arguments for PrepareSpendLock, analogous to swap_helper.StateData for SwapHelper.Prepare.
type SpendLockRequest struct {
	// From is the Paladin identity for SpendLockParams.from and should match DomainTransactionHelper.Prepare(signer).
	From string
	// LockId is the v0.5 on-chain lock id (keccak(pool, createLock msg.sender, txId)).
	LockId pldtypes.Bytes32
}

// PrepareSpendLock runs testbed_prepare for IZetoFungible_V1.spendLock (lockId, from, data).
// then wraps the result for a public submission path like swap_helper.SwapHelper.Prepare → *TransactionHelper.
// Use SpendLockRequest.From for the domain prepare signer; call SignAndSend(onChainLockSpender) for msg.sender.
//
// Delegate and spend recipients are taken from persisted ZetoLockInfoState (unlockData / spendData), not from this request.
func (z *ZetoHelperFungible) PrepareSpendLock(ctx context.Context, tb testbed.Testbed, pld pldclient.PaladinClient, req *SpendLockRequest) *TransactionHelper {
	require.NotNil(z.t, req)
	require.NotEmpty(z.t, req.From, "SpendLockRequest.From is required for testbed_prepare")
	prepared := z.spendLockDomainTransaction(ctx, req).Prepare(req.From)
	require.NotNil(z.t, prepared.PreparedTransaction, "spendLock prepare should return a public transaction")
	// Prepared tx uses on-chain spendLock(bytes32,bytes,bytes); domain prepare attaches IZetoFungible_V1 ABI (string from).
	// pldclient resolves Function against tx.ABI — swap to pool implementation ABI so SignAndSend can encode/send.
	builder := pld.TxBuilder(ctx).Wrap(prepared.PreparedTransaction).Public().ABI(z.implABI)
	return NewTransactionHelper(ctx, z.t, tb, builder)
}

func (z *ZetoHelperFungible) spendLockDomainTransaction(ctx context.Context, req *SpendLockRequest) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI_V1.Functions()[types.METHOD_SPEND_LOCK]
	emptyData := pldtypes.HexBytes{}
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.SpendLockParams{
		LockId: req.LockId,
		From:   req.From,
		Data:   emptyData,
	}))
}

func DeployERC20(ctx context.Context, rpc rpcclient.Client, deployer, initialOwnerAddr string) (*pldtypes.EthAddress, error) {
	build := solutils.MustLoadBuild(erc20ABI)
	params := fmt.Sprintf(`{"initialOwner":"%s"}`, initialOwnerAddr)
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode", deployer, build.ABI, build.Bytecode.String(), pldtypes.RawJSON(params))
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return pldtypes.MustEthAddress(addr), nil
}

func (z *ZetoHelperFungible) SetERC20(ctx context.Context, tb testbed.Testbed, sender string, erc20Address *pldtypes.EthAddress) {
	paramsJson, _ := json.Marshal(&map[string]string{"erc20": erc20Address.String()})
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     sender,
			To:       z.Address,
			Function: "setERC20",
			Data:     paramsJson,
		},
		ABI: z.implABI,
	})
	assert.NoError(z.t, err)
}

func (z *ZetoHelperFungible) Mint(ctx context.Context, to string, amounts []uint64) *DomainTransactionHelper {
	entries := make([]*types.FungibleTransferParamEntry, len(amounts))
	for i, amount := range amounts {
		entries[i] = &types.FungibleTransferParamEntry{
			To:     to,
			Amount: pldtypes.Uint64ToUint256(amount),
		}
	}
	fn := types.ZetoFungibleABI.Functions()["mint"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.FungibleMintParams{
		Mints: entries,
	}))
}

func (z *ZetoHelperFungible) Transfer(ctx context.Context, to []string, amounts []uint64) *DomainTransactionHelper {
	entries := make([]*types.FungibleTransferParamEntry, len(amounts))
	for i, amount := range amounts {
		entries[i] = &types.FungibleTransferParamEntry{
			To:     to[i],
			Amount: pldtypes.Uint64ToUint256(amount),
		}
	}
	fn := types.ZetoFungibleABI.Functions()["transfer"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.FungibleTransferParams{
		Transfers: entries,
	}))
}

func (z *ZetoHelperFungible) BalanceOf(ctx context.Context, account string) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["balanceOf"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.FungibleBalanceOfParam{
		Account: account,
	}))
}

func (z *ZetoHelper) TransferLocked(ctx context.Context, lockedUtxo *pldtypes.HexUint256, delegate, to string, amount uint64) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["transferLocked"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.FungibleTransferLockedParams{
		LockedInputs: []*pldtypes.HexUint256{lockedUtxo},
		Delegate:     delegate,
		Transfers: []*types.FungibleTransferParamEntry{
			{
				To:     to,
				Amount: pldtypes.Uint64ToUint256(amount),
			},
		},
	}))
}

func (z *ZetoHelper) SendTransferLocked(ctx context.Context, tb testbed.Testbed, sender string, result *testbed.TransactionResult) {
	require.NotNil(z.t, result.PreparedTransaction)
	tx := *result.PreparedTransaction
	tx.From = sender
	_, err := tb.ExecTransactionSync(ctx, &tx)
	assert.NoError(z.t, err)
}

// SendSpendLock submits the public spendLock leg for a prepared IZetoFungible_V1 spendLock via testbed.ExecTransactionSync.
// Prefer PrepareSpendLock(ctx, tb, pld, req).SignAndSend(sender) for the same pattern as swap_helper.Prepare.
//
// sender must be the on-chain lock spender (ZetoLockable: same identity as the public createLock submitter).
// Use the prepared tx's Function (full signature) and ABI ({single entry}) from the domain — rebuilding with a
// short function name and the full pool ABI can serialize the wrong overload and produce undecodable calldata
// (Solidity reverts with no reason on ABI decode failure).
func (z *ZetoHelper) SendSpendLock(ctx context.Context, tb testbed.Testbed, sender string, result *testbed.TransactionResult) (*pldapi.TransactionReceipt, error) {
	require.NotNil(z.t, result.PreparedTransaction)
	tx := *result.PreparedTransaction
	tx.From = sender
	receipt, err := tb.ExecTransactionSync(ctx, &tx)
	if err != nil {
		return nil, err
	}
	assert.NoError(z.t, err)
	require.NotNil(z.t, receipt)
	return receipt, nil
}

func (z *ZetoHelper) Lock(ctx context.Context, delegate *pldtypes.EthAddress, amount int) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["lock"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.LockParams{
		Delegate: delegate,
		Amount:   pldtypes.Uint64ToUint256(uint64(amount)),
	}))
}

func (z *ZetoHelper) DelegateLock(ctx context.Context, tb testbed.Testbed, lockedUtxo *pldtypes.HexUint256, delegate *pldtypes.EthAddress, sender string) {
	txInput := map[string]any{
		"utxos":    []string{lockedUtxo.String()},
		"delegate": delegate.String(),
		"data":     "0x",
	}
	txInputJson, _ := json.Marshal(txInput)
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     sender,
			To:       z.Address,
			Function: "delegateLock",
			Data:     txInputJson,
		},
		ABI: z.implABI,
	})
	assert.NoError(z.t, err)
}

// DelegateLockV1 submits ILockableCapability.delegateLock on fungible V1 (~v0.5.x upstream) pools:
// delegateLock(bytes32 lockId, bytes delegateArgs, address newSpender, bytes data).
// sender must be the current on-chain lock spender (the public createLock submitter ETH identity).
func (z *ZetoHelper) DelegateLockV1(ctx context.Context, tb testbed.Testbed, lockID pldtypes.Bytes32, newSpender *pldtypes.EthAddress, sender string) {
	txID := pldtypes.Bytes32UUIDFirst16(uuid.New())
	delegateArgsJSON, err := json.Marshal([]any{map[string]any{
		"txId": txID.HexString0xPrefix(),
	}})
	assert.NoError(z.t, err)
	delegateArgsBytes, err := zetoDelegateLockArgsTupleABI.EncodeABIDataJSONCtx(ctx, delegateArgsJSON)
	assert.NoError(z.t, err)
	txInput := map[string]any{
		"lockId":       lockID.HexString0xPrefix(),
		"delegateArgs": pldtypes.HexBytes(delegateArgsBytes).HexString0xPrefix(),
		"newSpender":   newSpender.String(),
		"data":         "0x",
	}
	txInputJSON, err := json.Marshal(txInput)
	assert.NoError(z.t, err)
	_, err = tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     sender,
			To:       z.Address,
			Function: "delegateLock",
			Data:     txInputJSON,
		},
		ABI: z.implABI,
	})
	assert.NoError(z.t, err)
}

func (z *ZetoHelper) MintERC20(ctx context.Context, tb testbed.Testbed, erc20Address pldtypes.EthAddress, amount int64, from, to string) {
	paramsJson, _ := json.Marshal(&map[string]any{"amount": amount, "to": to})
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     from,
			To:       &erc20Address,
			Function: "mint",
			Data:     paramsJson,
		},
		ABI: solutils.MustLoadBuild(erc20ABI).ABI,
	})
	assert.NoError(z.t, err)
}

func (z *ZetoHelper) ApproveERC20(ctx context.Context, tb testbed.Testbed, erc20Address pldtypes.EthAddress, amount int64, from string) {
	paramsJson, _ := json.Marshal(&map[string]any{"spender": z.Address.String(), "value": amount})
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     from,
			To:       &erc20Address,
			Function: "approve",
			Data:     paramsJson,
		},
		ABI: solutils.MustLoadBuild(erc20ABI).ABI,
	})
	assert.NoError(z.t, err)
}

func (z *ZetoHelper) Deposit(ctx context.Context, amount int64) *DomainTransactionHelper {
	params := &types.DepositParams{
		Amount: pldtypes.Int64ToInt256(amount),
	}
	fn := types.ZetoFungibleABI.Functions()["deposit"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &params))
}

func (z *ZetoHelper) Withdraw(ctx context.Context, amount int64) *DomainTransactionHelper {
	params := &types.WithdrawParams{
		Amount: pldtypes.Int64ToInt256(amount),
	}
	fn := types.ZetoFungibleABI.Functions()["withdraw"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &params))
}

func (z *ZetoHelper) Register(ctx context.Context, tb testbed.Testbed, sender string, publicKey []*big.Int) {
	abi := abi.ABI{
		&abi.Entry{
			Type: abi.Function,
			Name: "register",
			Inputs: abi.ParameterArray{
				{Name: "publicKey", Type: "uint256[2]"},
				{Name: "data", Type: "bytes"},
			},
		},
	}
	paramsJson, _ := json.Marshal(&map[string]any{"publicKey": publicKey, "data": "0x"})
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     sender,
			To:       z.Address,
			Function: "register",
			Data:     paramsJson,
		},
		ABI: abi,
	})
	assert.NoError(z.t, err)
}

// =============================================================================
//
//	NonFungible
//
// =============================================================================
type ZetoHelperNonFungible struct {
	ZetoHelper
}

func DeployZetoNonFungible(ctx context.Context, t *testing.T, rpc rpcclient.Client, domainName, controllerName, tokenName string) *ZetoHelperNonFungible {
	var addr pldtypes.EthAddress
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy", domainName, controllerName, &types.InitializerParams{
		TokenName: tokenName,
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr)
	}
	return &ZetoHelperNonFungible{
		ZetoHelper: ZetoHelper{
			t:       t,
			rpc:     rpc,
			Address: &addr,
		},
	}
}

func (n *ZetoHelperNonFungible) Mint(ctx context.Context, to, uri []string) *DomainTransactionHelper {
	entries := make([]*types.NonFungibleTransferParamEntry, len(uri))
	for i, u := range uri {
		entries[i] = &types.NonFungibleTransferParamEntry{
			To:  to[i],
			URI: u,
		}
	}
	fn := types.ZetoNonFungibleABI.Functions()["mint"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.NonFungibleMintParams{
		Mints: entries,
	}))
}

func (n *ZetoHelperNonFungible) Transfer(ctx context.Context, to string, tokenID *pldtypes.HexUint256) *DomainTransactionHelper {
	fn := types.ZetoNonFungibleABI.Functions()["transfer"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.NonFungibleTransferParams{
		Transfers: []*types.NonFungibleTransferParamEntry{
			{
				To:      to,
				TokenID: tokenID,
			},
		},
	}))
}
