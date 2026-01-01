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
	"encoding/json"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateMintLock(t *testing.T) {
	ctx, mockCallbacks, n := newNotoFullSchemaSet(t)
	fn := types.NotoABI.Functions()["createMintLock"]

	receiverAddress := "0x2000000000000000000000000000000000000000"
	notaryKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "notary@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    contractAddress,
			ContractConfigJson: mustParseJSON(notoBasicConfigV1),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: `{
			"recipients": [{
				"to": "receiver@node2",
				"amount": 100
			}],
			"data": "0x1234"
		}`,
	}

	initRes, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.NoError(t, err)
	require.Len(t, initRes.RequiredVerifiers, 2)
	assert.Equal(t, "notary@node1", initRes.RequiredVerifiers[0].Lookup)
	assert.Equal(t, "receiver@node2", initRes.RequiredVerifiers[1].Lookup)

	verifiers := []*prototk.ResolvedVerifier{
		{
			Lookup:       "notary@node1",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     notaryKey.Address.String(),
		},
		{
			Lookup:       "receiver@node2",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     receiverAddress,
		},
	}

	assembleRes, err := n.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, assembleRes.AssemblyResult)
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 1) // lock
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 3) // manifest + data + spend-coin

	manifestState := assembleRes.AssembledTransaction.InfoStates[0]
	dataState := assembleRes.AssembledTransaction.InfoStates[1]
	spendCoinState := assembleRes.AssembledTransaction.InfoStates[2]
	newLockInfoState := assembleRes.AssembledTransaction.OutputStates[0]

	spendCoin, err := n.unmarshalCoin(spendCoinState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, receiverAddress, spendCoin.Owner.String())
	assert.Equal(t, "100", spendCoin.Amount.Int().String())
	outputInfo, err := n.unmarshalInfo(dataState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", outputInfo.Data.String())

	lockInfo, err := n.unmarshalLockV1(newLockInfoState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, notaryKey.Address.String(), lockInfo.Owner.String())
	lockID, err := n.computeLockId(ctx, pldtypes.MustEthAddress(contractAddress), pldtypes.MustEthAddress(notaryKey.Address.String()), tx.TransactionId)
	require.NoError(t, err)

	assert.Equal(t, lockID, lockInfo.LockID)
	require.Len(t, lockInfo.SpendOutputs, 1)
	require.Len(t, lockInfo.CancelOutputs, 0)
	require.NotEmpty(t, lockInfo.SpendData)
	require.Equal(t, lockInfo.SpendData, lockInfo.CancelData) // same data for both currently

	encodedUnlock, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoLockedCoin{}, []*types.NotoLockedCoin{}, []*types.NotoCoin{spendCoin})
	require.NoError(t, err)
	signature, err := notaryKey.SignDirect(encodedUnlock)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	inputStates := []*prototk.EndorsableState{}
	outputStates := []*prototk.EndorsableState{
		{
			SchemaId:      n.lockInfoSchemaV1.Id,
			Id:            *newLockInfoState.Id,
			StateDataJson: newLockInfoState.StateDataJson,
		},
	}
	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      n.dataSchemaV1.Id,
			Id:            *dataState.Id,
			StateDataJson: dataState.StateDataJson,
		},
		{
			SchemaId:      n.coinSchema.Id,
			Id:            *spendCoinState.Id,
			StateDataJson: spendCoinState.StateDataJson,
		},
	}

	endorseRes, err := n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		Reads:             nil,
		Info:              infoStates,
		Inputs:            inputStates,
		Outputs:           outputStates,
		EndorsementRequest: &prototk.AttestationRequest{
			Name: "notary",
		},
		Signatures: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: notaryKey.Address.String()},
				Payload:  signatureBytes,
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_ENDORSER_SUBMIT, endorseRes.EndorsementResult)

	// Prepare once to test base invoke
	prepareRes, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		ReadStates:        nil,
		InfoStates:        infoStates,
		InputStates:       inputStates,
		OutputStates:      outputStates,
		AttestationResult: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: notaryKey.Address.String()},
				Payload:  signatureBytes,
			},
			{
				Name:     "notary",
				Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"},
			},
		},
	})
	require.NoError(t, err)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Extract the options from the response to get the generated SpendTxId
	createLockABI := interfaceV1Build.ABI.Functions()["createLock"]
	expectedFunction := mustParseJSON(createLockABI)
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Decode the function parameters
	fnParams := decodeFnParams[CreateLockParams](t, createLockABI, prepareRes.Transaction.ParamsJson)
	data, err := n.decodeTransactionDataV1(ctx, fnParams.Data) // this is the transaction data for the prepare (not the prepared transaction)
	require.NoError(t, err)
	require.Equal(t, &types.NotoTransactionData_V1{
		InfoStates: []pldtypes.Bytes32{
			pldtypes.MustParseBytes32(*dataState.Id),
			pldtypes.MustParseBytes32(*spendCoinState.Id),
		},
	}, data)

	// Decode the options we store into the lockInfo
	emptyTxData, err := n.encodeTransactionDataV1(ctx, []*prototk.EndorsableState{})
	require.NoError(t, err)
	notoOptions := decodeSingleABITuple[types.NotoLockOptions](t, types.NotoLockOptionsABI, fnParams.Params.Options)
	expectedSpendHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, notoOptions.SpendTxId.HexString(), []string{}, endorsableStateIDs(infoStates[1:2]), emptyTxData)
	require.NoError(t, err)
	require.Equal(t, expectedSpendHash, fnParams.Params.SpendHash)
	expectedCancelHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, notoOptions.SpendTxId.HexString(), []string{}, []string{}, emptyTxData)
	require.NoError(t, err)
	require.Equal(t, expectedCancelHash, fnParams.Params.CancelHash)

	// Validate the encoded noto parameters passed in
	notoParams := decodeSingleABITuple[types.NotoLockOperation](t, types.NotoLockOperationABI, fnParams.CreateInputs)
	require.Equal(t, &types.NotoLockOperation{
		TxId:          "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		Inputs:        []string{},
		Outputs:       []string{*newLockInfoState.Id},
		LockedOutputs: []string{},
		Proof:         signatureBytes,
	}, notoParams)

	// Prepare again to test hook invoke
	hookAddress := "0x515fba7fe1d8b9181be074bd4c7119544426837c"
	tx.ContractInfo.ContractConfigJson = mustParseJSON(&types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
		NotaryMode:   types.NotaryModeHooks.Enum(),
		Variant:      types.NotoVariantDefault,
		Options: types.NotoOptions{
			Hooks: &types.NotoHooksOptions{
				PublicAddress:     pldtypes.MustEthAddress(hookAddress),
				DevUsePublicHooks: true,
			},
		},
	})
	prepareRes, err = n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		ReadStates:        nil,
		InfoStates:        infoStates,
		InputStates:       inputStates,
		OutputStates:      outputStates,
		AttestationResult: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: notaryKey.Address.String()},
				Payload:  signatureBytes,
			},
			{
				Name:     "notary",
				Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"},
			},
		},
	})
	require.NoError(t, err)
	expectedFunctionABI := hooksBuild.ABI.Functions()["onCreateMintLock"]
	assert.JSONEq(t, mustParseJSON(expectedFunctionABI), prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)
	_, err = expectedFunctionABI.EncodeCallDataJSON([]byte(prepareRes.Transaction.ParamsJson))
	require.NoError(t, err)

	// Verify hook invoke params
	var hookParams CreateMintLockHookParams
	err = json.Unmarshal([]byte(prepareRes.Transaction.ParamsJson), &hookParams)
	require.NoError(t, err)
	require.NotNil(t, hookParams.Sender)
	assert.Equal(t, notaryKey.Address.String(), hookParams.Sender.String())
	assert.Equal(t, lockID, hookParams.LockID)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x1234"), hookParams.Data)

	// Verify recipients
	require.Len(t, hookParams.Recipients, 1)
	require.NotNil(t, hookParams.Recipients[0].To)
	assert.Equal(t, pldtypes.MustEthAddress("0x2000000000000000000000000000000000000000").String(), hookParams.Recipients[0].To.String())
	require.NotNil(t, hookParams.Recipients[0].Amount)
	assert.Equal(t, pldtypes.Int64ToInt256(100).String(), hookParams.Recipients[0].Amount.String())

	// Verify prepared transaction
	assert.Equal(t, pldtypes.MustEthAddress(contractAddress), hookParams.Prepared.ContractAddress)
	assert.NotEmpty(t, hookParams.Prepared.EncodedCall)

	manifestState.Id = confutil.P(pldtypes.RandBytes32().String()) // manifest is odd one out that  doesn't get ID allocated during assemble
	mt := newManifestTester(t, ctx, n, mockCallbacks, tx.TransactionId, assembleRes.AssembledTransaction)
	mt.withMissingStates( /* no missing states */ ).
		completeForIdentity(notaryKey.Address.String()).
		completeForIdentity(receiverAddress)
	mt.withMissingNewStates(manifestState, dataState).
		incompleteForIdentity(notaryKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(dataState).
		incompleteForIdentity(notaryKey.Address.String()).
		completeForIdentity(receiverAddress) // receivers don't get the data
	mt.withMissingNewStates(newLockInfoState).
		incompleteForIdentity(notaryKey.Address.String()).
		completeForIdentity(receiverAddress) // receivers don't get the lock
	mt.withMissingNewStates(spendCoinState).
		incompleteForIdentity(notaryKey.Address.String()).
		incompleteForIdentity(receiverAddress)

	receipt := testGetDomainReceipt(t, n, &prototk.BuildReceiptRequest{
		TransactionId:     tx.TransactionId,
		UnavailableStates: false,
		InputStates:       inputStates,
		OutputStates:      outputStates,
		InfoStates:        infoStates,
	})
	require.Equal(t, lockInfo.LockID, receipt.LockInfo.LockID)
	require.Equal(t, "spendLock", receipt.LockInfo.UnlockFunction)
	require.NotNil(t, receipt.LockInfo.UnlockParams)
}
