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

package noto

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type preparedExecTestFixture struct {
	n              *Noto
	ctx            context.Context
	lockID         pldtypes.Bytes32
	spendTxId      pldtypes.Bytes32
	ownerKey       *secp256k1.KeyPair
	notaryAddress  string
	receiverAddr   string
	lockedCoin     *prototk.StoredState
	lockInfoState  *prototk.StoredState
	spendOutputID  pldtypes.Bytes32
	cancelOutputID pldtypes.Bytes32
	verifiers      []*prototk.ResolvedVerifier
	contractAddr   string
}

func newPreparedExecFixture(t *testing.T, delegated bool, prepared bool) *preparedExecTestFixture {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       testSchema("coin"),
		lockedCoinSchema: testSchema("lockedCoin"),
		lockInfoSchemaV0: testSchema("lockInfo"),
		lockInfoSchemaV1: testSchema("lockInfo_v1"),
		dataSchemaV0:     testSchema("data"),
		dataSchemaV1:     testSchema("data_v1"),
		dataSchemaV2:     testSchema("data_v2"),
		manifestSchema:   testSchema("manifest"),
	}
	ctx := t.Context()

	notaryAddress := "0x1000000000000000000000000000000000000000"
	receiverAddress := "0x2000000000000000000000000000000000000000"
	ownerKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	lockID := pldtypes.RandBytes32()
	spendTxId := pldtypes.RandBytes32()
	spendOutputID := pldtypes.RandBytes32()
	cancelOutputID := pldtypes.RandBytes32()

	lockedCoin := &prototk.StoredState{
		Id:       pldtypes.RandBytes32().String(),
		SchemaId: hashName("lockedCoin"),
		DataJson: mustParseJSON(types.NotoLockedCoin{
			Salt:   pldtypes.RandBytes32(),
			LockID: lockID,
			Owner:  (*pldtypes.EthAddress)(&ownerKey.Address),
			Amount: pldtypes.Int64ToInt256(100),
		}),
		CreatedAt: 1,
	}

	spender := ownerKey.Address.String()
	if delegated {
		spender = "0x9999999999999999999999999999999999999999"
	}
	spendTxIdStr := spendTxId.String()
	if !prepared {
		spendTxIdStr = pldtypes.Bytes32{}.String()
	}
	lockInfoState := &prototk.StoredState{
		Id:       pldtypes.RandBytes32().String(),
		SchemaId: hashName("lockInfo_v1"),
		DataJson: fmt.Sprintf(`{
			"lockId": "%s",
			"salt": "%s",
			"owner": "%s",
			"spender": "%s",
			"spendTxId": "%s",
			"spendOutputs": ["%s"],
			"spendData": "0xaabb",
			"cancelOutputs": ["%s"],
			"cancelData": "0xccdd"
		}`, lockID, pldtypes.RandBytes32(), ownerKey.Address, spender, spendTxIdStr, spendOutputID, cancelOutputID),
	}

	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		switch req.SchemaId {
		case hashName("lockInfo_v1"):
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{lockInfoState}}, nil
		case hashName("lockedCoin"):
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{lockedCoin}}, nil
		}
		return nil, fmt.Errorf("unmocked query for schema %s", req.SchemaId)
	}

	mockCallbacks.MockGetStatesByID = func(ctx context.Context, req *prototk.GetStatesByIDRequest) (*prototk.GetStatesByIDResponse, error) {
		states := []*prototk.StoredState{}
		for _, id := range req.StateIds {
			switch id {
			case spendOutputID.String():
				states = append(states, &prototk.StoredState{
					Id:       spendOutputID.String(),
					SchemaId: hashName("coin"),
					DataJson: mustParseJSON(types.NotoCoin{
						Salt:   pldtypes.RandBytes32(),
						Owner:  pldtypes.MustEthAddress(receiverAddress),
						Amount: pldtypes.Int64ToInt256(100),
					}),
				})
			case cancelOutputID.String():
				states = append(states, &prototk.StoredState{
					Id:       cancelOutputID.String(),
					SchemaId: hashName("coin"),
					DataJson: mustParseJSON(types.NotoCoin{
						Salt:   pldtypes.RandBytes32(),
						Owner:  (*pldtypes.EthAddress)(&ownerKey.Address),
						Amount: pldtypes.Int64ToInt256(100),
					}),
				})
			}
		}
		return &prototk.GetStatesByIDResponse{States: states}, nil
	}

	verifiers := []*prototk.ResolvedVerifier{
		{Lookup: "notary@node1", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS, Verifier: notaryAddress},
		{Lookup: "sender@node1", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS, Verifier: ownerKey.Address.String()},
	}

	return &preparedExecTestFixture{
		n:              n,
		ctx:            ctx,
		lockID:         lockID,
		spendTxId:      spendTxId,
		ownerKey:       ownerKey,
		notaryAddress:  notaryAddress,
		receiverAddr:   receiverAddress,
		lockedCoin:     lockedCoin,
		lockInfoState:  lockInfoState,
		spendOutputID:  spendOutputID,
		cancelOutputID: cancelOutputID,
		verifiers:      verifiers,
		contractAddr:   "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3",
	}
}

func (f *preparedExecTestFixture) tx(function string, hooks bool) *prototk.TransactionSpecification {
	fn := types.NotoABI.Functions()[function]
	config := &types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
		Variant:      types.NotoVariantV2,
	}
	if hooks {
		config.NotaryMode = types.NotaryModeHooks.Enum()
		config.Options = types.NotoOptions{
			Hooks: &types.NotoHooksOptions{
				PublicAddress:     pldtypes.MustEthAddress("0x515fba7fe1d8b9181be074bd4c7119544426837c"),
				DevUsePublicHooks: true,
			},
		}
	}
	return &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    f.contractAddr,
			ContractConfigJson: mustParseJSON(config),
		},
		FunctionAbiJson:    mustParseJSON(fn),
		FunctionSignature:  fn.SolString(),
		FunctionParamsJson: fmt.Sprintf(`{"lockId": "%s", "data": "0x1234"}`, f.lockID),
	}
}

// lockTransitionStates returns the input states the Endorse/Prepare steps receive for a LOCK_SPEND.
func (f *preparedExecTestFixture) inputStates() []*prototk.EndorsableState {
	return []*prototk.EndorsableState{
		{SchemaId: hashName("lockedCoin"), Id: f.lockedCoin.Id, StateDataJson: f.lockedCoin.DataJson},
		{SchemaId: hashName("lockInfo_v1"), Id: f.lockInfoState.Id, StateDataJson: f.lockInfoState.DataJson},
	}
}

func TestSpendLockPrepared(t *testing.T) {
	f := newPreparedExecFixture(t, false /* not delegated */, true /* prepared */)
	tx := f.tx("spendLock", false)

	initRes, err := f.n.InitTransaction(f.ctx, &prototk.InitTransactionRequest{Transaction: tx})
	require.NoError(t, err)
	require.Len(t, initRes.RequiredVerifiers, 2)

	assembleRes, err := f.n.AssembleTransaction(f.ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: f.verifiers,
	})
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, assembleRes.AssemblyResult)
	// Locked coin + lock state are consumed; no fresh outputs (confirmed by event)
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 2)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 0)
	// Only the notary endorses - no sender signature
	require.Len(t, assembleRes.AttestationPlan, 1)
	require.Equal(t, "notary", assembleRes.AttestationPlan[0].Name)
	require.Equal(t, prototk.AttestationType_ENDORSE, assembleRes.AttestationPlan[0].AttestationType)

	endorseRes, err := f.n.EndorseTransaction(f.ctx, &prototk.EndorseTransactionRequest{
		Transaction:        tx,
		ResolvedVerifiers:  f.verifiers,
		Inputs:             f.inputStates(),
		EndorsementRequest: &prototk.AttestationRequest{Name: "notary"},
	})
	require.NoError(t, err)
	require.Equal(t, prototk.EndorseTransactionResponse_ENDORSER_SUBMIT, endorseRes.EndorsementResult)

	prepareRes, err := f.n.PrepareTransaction(f.ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: f.verifiers,
		InputStates:       f.inputStates(),
		AttestationResult: []*prototk.AttestationResult{
			{Name: "notary", Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"}},
		},
	})
	require.NoError(t, err)

	spendLockABI := interfaceV2Build.ABI.Functions()["spendLock"]
	assert.JSONEq(t, mustParseJSON(spendLockABI), prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	params := decodeFnParams[SpendLockParams](t, spendLockABI, prepareRes.Transaction.ParamsJson)
	require.Equal(t, f.lockID, params.LockID)
	require.Equal(t, pldtypes.MustParseHexBytes("0x1234"), params.Data) // outer data passed through

	notoParams := decodeSingleABITuple[types.NotoSpendLockArgs](t, types.NotoSpendLockArgsABI, params.SpendArgs)
	require.Equal(t, f.spendTxId.String(), notoParams.TxId) // uses the prearranged spendTxId
	require.Equal(t, []string{f.lockedCoin.Id}, notoParams.Inputs)
	require.Equal(t, []string{f.spendOutputID.String()}, notoParams.Outputs) // prearranged spend outputs
	require.Equal(t, pldtypes.MustParseHexBytes("0xaabb"), notoParams.Data)  // prearranged spend data
	require.Empty(t, notoParams.Proof)
}

func TestSpendLockPreparedWithHooks(t *testing.T) {
	f := newPreparedExecFixture(t, false, true)
	tx := f.tx("spendLock", true /* hooks */)

	prepareRes, err := f.n.PrepareTransaction(f.ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: f.verifiers,
		InputStates:       f.inputStates(),
		AttestationResult: []*prototk.AttestationResult{
			{Name: "notary", Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"}},
		},
	})
	require.NoError(t, err)

	expectedFunctionABI := hooksV2Build.ABI.Functions()["onSpendLock"]
	assert.JSONEq(t, mustParseJSON(expectedFunctionABI), prepareRes.Transaction.FunctionAbiJson)

	var hookParams UnlockHookParams
	require.NoError(t, json.Unmarshal([]byte(prepareRes.Transaction.ParamsJson), &hookParams))
	assert.Equal(t, f.ownerKey.Address.String(), hookParams.Sender.String())
	assert.Equal(t, f.lockID, hookParams.LockID)
	require.Len(t, hookParams.Recipients, 1)
	assert.Equal(t, f.receiverAddr, hookParams.Recipients[0].To.String())
	assert.Equal(t, "100", hookParams.Recipients[0].Amount.Int().String())
}

func TestCancelLockPrepared(t *testing.T) {
	f := newPreparedExecFixture(t, false, true)
	tx := f.tx("cancelLock", false)

	assembleRes, err := f.n.AssembleTransaction(f.ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: f.verifiers,
	})
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, assembleRes.AssemblyResult)

	prepareRes, err := f.n.PrepareTransaction(f.ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: f.verifiers,
		InputStates:       f.inputStates(),
		AttestationResult: []*prototk.AttestationResult{
			{Name: "notary", Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"}},
		},
	})
	require.NoError(t, err)

	cancelLockABI := interfaceV2Build.ABI.Functions()["cancelLock"]
	assert.JSONEq(t, mustParseJSON(cancelLockABI), prepareRes.Transaction.FunctionAbiJson)

	params := decodeFnParams[CancelLockParams](t, cancelLockABI, prepareRes.Transaction.ParamsJson)
	require.Equal(t, f.lockID, params.LockID)

	notoParams := decodeSingleABITuple[types.NotoSpendLockArgs](t, types.NotoSpendLockArgsABI, params.CancelArgs)
	require.Equal(t, f.spendTxId.String(), notoParams.TxId)
	require.Equal(t, []string{f.cancelOutputID.String()}, notoParams.Outputs) // prearranged cancel outputs
	require.Equal(t, pldtypes.MustParseHexBytes("0xccdd"), notoParams.Data)   // prearranged cancel data
}

func TestCancelLockPreparedWithHooks(t *testing.T) {
	f := newPreparedExecFixture(t, false, true)
	tx := f.tx("cancelLock", true)

	prepareRes, err := f.n.PrepareTransaction(f.ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: f.verifiers,
		InputStates:       f.inputStates(),
		AttestationResult: []*prototk.AttestationResult{
			{Name: "notary", Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"}},
		},
	})
	require.NoError(t, err)

	expectedFunctionABI := hooksV2Build.ABI.Functions()["onCancelLock"]
	assert.JSONEq(t, mustParseJSON(expectedFunctionABI), prepareRes.Transaction.FunctionAbiJson)

	var hookParams CancelLockHookParams
	require.NoError(t, json.Unmarshal([]byte(prepareRes.Transaction.ParamsJson), &hookParams))
	assert.Equal(t, f.ownerKey.Address.String(), hookParams.Sender.String())
	assert.Equal(t, f.lockID, hookParams.LockID)
}

func TestSpendLockRejectsUnpreparedLock(t *testing.T) {
	f := newPreparedExecFixture(t, false, false /* not prepared */)
	tx := f.tx("spendLock", false)

	assembleRes, err := f.n.AssembleTransaction(f.ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: f.verifiers,
	})
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, assembleRes.AssemblyResult)
	require.Contains(t, *assembleRes.RevertReason, "PD200043")
}

func TestSpendLockRejectsDelegatedLock(t *testing.T) {
	f := newPreparedExecFixture(t, true /* delegated */, true)
	tx := f.tx("spendLock", false)

	assembleRes, err := f.n.AssembleTransaction(f.ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: f.verifiers,
	})
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, assembleRes.AssemblyResult)
	require.Contains(t, *assembleRes.RevertReason, "PD200044")
}

func TestSpendLockRejectsV0(t *testing.T) {
	f := newPreparedExecFixture(t, false, true)
	config := &types.NotoParsedConfig{NotaryLookup: "notary@node1", Variant: types.NotoVariantV0}
	fn := types.NotoABI.Functions()["spendLock"]

	// V0 is rejected during params validation (InitTransaction validates params first)
	_, err := f.n.InitTransaction(f.ctx, &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
			From:          "sender@node1",
			ContractInfo: &prototk.ContractInfo{
				ContractAddress:    f.contractAddr,
				ContractConfigJson: mustParseJSON(config),
			},
			FunctionAbiJson:    mustParseJSON(fn),
			FunctionSignature:  fn.SolString(),
			FunctionParamsJson: fmt.Sprintf(`{"lockId": "%s", "data": "0x"}`, f.lockID),
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "PD200014")
}

func TestSpendLockHooksRequiresV2(t *testing.T) {
	f := newPreparedExecFixture(t, false, true)
	tx := f.tx("spendLock", true)
	// Override to V1 with hooks
	config := &types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
		Variant:      types.NotoVariantV1,
		NotaryMode:   types.NotaryModeHooks.Enum(),
		Options: types.NotoOptions{
			Hooks: &types.NotoHooksOptions{
				PublicAddress:     pldtypes.MustEthAddress("0x515fba7fe1d8b9181be074bd4c7119544426837c"),
				DevUsePublicHooks: true,
			},
		},
	}
	tx.ContractInfo.ContractConfigJson = mustParseJSON(config)

	assembleRes, err := f.n.AssembleTransaction(f.ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: f.verifiers,
	})
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, assembleRes.AssemblyResult)
	require.Contains(t, *assembleRes.RevertReason, "PD200045")
}
