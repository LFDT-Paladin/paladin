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

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelegateLock(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       &prototk.StateSchema{Id: "coin"},
		lockedCoinSchema: &prototk.StateSchema{Id: "lockedCoin"},
		lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfo_v1"},
		dataSchemaV1:     &prototk.StateSchema{Id: "data"},
		manifestSchema:   &prototk.StateSchema{Id: "manifest"},
	}
	ctx := context.Background()
	fn := types.NotoABI.Functions()["delegateLock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	delegateAddress := "0x2000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	lockID := pldtypes.RandBytes32()
	inputLockedCoin := &types.NotoLockedCoinState{
		ID: pldtypes.RandBytes32(),
		Data: types.NotoLockedCoin{
			LockID: lockID,
			Owner:  (*pldtypes.EthAddress)(&senderKey.Address),
			Amount: pldtypes.Int64ToInt256(100),
		},
	}
	inputLockState := &prototk.StoredState{
		Id:       "0xa7c7fa6677f6938bb90f9f0ccb3487707fe6a93c527d899f09af497ece2e603b",
		SchemaId: "lockInfo_v1",
		DataJson: fmt.Sprintf(`{"lockId":"%s"}`, lockID),
	}
	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		switch req.SchemaId {
		case "lockInfo_v1":
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{inputLockState},
			}, nil
		case "lockedCoin":
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						Id:       inputLockedCoin.ID.String(),
						SchemaId: "lockedCoin",
						DataJson: mustParseJSON(inputLockedCoin.Data),
					},
				},
			}, nil
		}
		return nil, fmt.Errorf("unmocked query")
	}

	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddress,
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				NotaryLookup: "notary@node1",
				Variant:      types.NotoVariantDefault,
			}),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: fmt.Sprintf(`{
			"lockId": "%s",
			"delegate": "%s",
			"data": "0x1234"
		}`, lockID, delegateAddress),
	}

	initRes, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.NoError(t, err)
	require.Len(t, initRes.RequiredVerifiers, 2)
	assert.Equal(t, "notary@node1", initRes.RequiredVerifiers[0].Lookup)
	assert.Equal(t, "sender@node1", initRes.RequiredVerifiers[1].Lookup)

	verifiers := []*prototk.ResolvedVerifier{
		{
			Lookup:       "notary@node1",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     notaryAddress,
		},
		{
			Lookup:       "sender@node1",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     senderKey.Address.String(),
		},
	}

	assembleRes, err := n.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, assembleRes.AssemblyResult)
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 1)  // old info
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 1) // new info
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 2) // manifest + txData
	assert.Equal(t, inputLockState.Id, assembleRes.AssembledTransaction.InputStates[0].Id)
	assert.Equal(t, "lockInfo_v1", assembleRes.AssembledTransaction.OutputStates[0].SchemaId)
	assert.Equal(t, inputLockedCoin.ID.String(), assembleRes.AssembledTransaction.ReadStates[0].Id)
	outputInfo, err := n.unmarshalInfo(assembleRes.AssembledTransaction.InfoStates[1].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", outputInfo.Data.String())

	encodedDelegate, err := n.encodeDelegateLock(ctx, ethtypes.MustNewAddress(contractAddress), lockID, pldtypes.MustEthAddress(delegateAddress), pldtypes.MustParseHexBytes("0x1234"))
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedDelegate)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	inputStates := []*prototk.EndorsableState{
		{
			SchemaId:      "lockInfo_v1",
			Id:            inputLockState.Id,
			StateDataJson: mustParseJSON(inputLockState.DataJson),
		},
	}
	readStates := []*prototk.EndorsableState{
		{
			SchemaId:      "lockedCoin",
			Id:            inputLockedCoin.ID.String(),
			StateDataJson: mustParseJSON(inputLockedCoin.Data),
		},
	}
	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      "manifest",
			Id:            "0x4cc7840e186de23c4127b4853c878708d2642f1942959692885e098f1944547d",
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[0].StateDataJson,
		},
		{
			SchemaId:      "data",
			Id:            "0x4cc7840e186de23c4127b4853c878708d2642f1942959692885e098f1944547d",
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[1].StateDataJson,
		},
	}
	outputStates := []*prototk.EndorsableState{
		{
			SchemaId:      "lockInfo_v1",
			Id:            "0x4da2191dc83d31196a735d8df477c1a588f1a5a15c084e7d66b7157ab539019f",
			StateDataJson: assembleRes.AssembledTransaction.OutputStates[0].StateDataJson,
		},
	}

	endorseRes, err := n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		Inputs:            inputStates,
		Reads:             readStates,
		Info:              infoStates,
		Outputs:           outputStates,
		EndorsementRequest: &prototk.AttestationRequest{
			Name: "notary",
		},
		Signatures: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: senderKey.Address.String()},
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
		ReadStates:        readStates,
		InfoStates:        infoStates,
		AttestationResult: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: senderKey.Address.String()},
				Payload:  signatureBytes,
			},
			{
				Name:     "notary",
				Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"},
			},
		},
	})
	require.NoError(t, err)

	// Decode the parameters
	delegateLockABI := interfaceBuild.ABI.Functions()["delegateLock"]
	expectedFunction := mustParseJSON(delegateLockABI)
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Validate the parameters
	params := decodeFnParams[DelegateLockParams](t, delegateLockABI, prepareRes.Transaction.ParamsJson)
	notoParams := decodeSingleABITuple[types.NotoDelegateOperation](t, types.NotoDelegateOperationABI, params.DelegateInputs)
	require.Equal(t, &types.NotoDelegateOperation{
		TxId:  "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		Proof: signatureBytes,
	}, notoParams)
	data, err := n.decodeTransactionDataV1(ctx, params.Data)
	require.NoError(t, err)
	require.Equal(t, &types.NotoTransactionData_V1{
		InfoStates: []pldtypes.Bytes32{
			pldtypes.MustParseBytes32("0x4cc7840e186de23c4127b4853c878708d2642f1942959692885e098f1944547d"),
			pldtypes.MustParseBytes32("0x4cc7840e186de23c4127b4853c878708d2642f1942959692885e098f1944547d"),
		},
	}, data)

	var invokeFn abi.Entry
	err = json.Unmarshal([]byte(prepareRes.Transaction.FunctionAbiJson), &invokeFn)
	require.NoError(t, err)
	encodedCall, err := invokeFn.EncodeCallDataJSONCtx(ctx, []byte(prepareRes.Transaction.ParamsJson))
	require.NoError(t, err)

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
		ReadStates:        readStates,
		InfoStates:        infoStates,
		AttestationResult: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: senderKey.Address.String()},
				Payload:  signatureBytes,
			},
			{
				Name:     "notary",
				Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"},
			},
		},
	})
	require.NoError(t, err)
	expectedFunctionABI := hooksBuild.ABI.Functions()["onDelegateLock"]
	assert.JSONEq(t, mustParseJSON(expectedFunctionABI), prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)
	_, err = expectedFunctionABI.EncodeCallDataJSON([]byte(prepareRes.Transaction.ParamsJson))
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{
		"sender": "%s",
		"lockId": "%s",
		"delegate": "%s",
		"data": "0x1234",
		"prepared": {
			"contractAddress": "%s",
			"encodedCall": "%s"
		}
	}`, senderKey.Address, lockID, delegateAddress, contractAddress, pldtypes.HexBytes(encodedCall)), prepareRes.Transaction.ParamsJson)

	// Verify manifest
	manifestState := assembleRes.AssembledTransaction.InfoStates[0]
	manifestState.Id = confutil.P(pldtypes.RandBytes32().String()) // manifest is odd one out that  doesn't get ID allocated during assemble
	dataState := assembleRes.AssembledTransaction.InfoStates[1]
	mt := newManifestTester(t, ctx, n, mockCallbacks, tx.TransactionId, assembleRes.AssembledTransaction)
	mt.withMissingStates( /* no missing states */ ).
		completeForIdentity(notaryAddress).
		completeForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(manifestState, dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())

}
