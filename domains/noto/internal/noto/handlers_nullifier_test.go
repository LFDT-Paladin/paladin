/*
 * Copyright © 2026 Kaleido, Inc.
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
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var notoNullifierConfig = &types.NotoParsedConfig{
	NotaryMode:   types.NotaryModeBasic.Enum(),
	NotaryLookup: "notary@node1",
	Variant:      types.NotoVariantV2Nullifiers,
	Options: types.NotoOptions{
		Basic: &types.NotoBasicOptions{
			RestrictMint: &pTrue,
			AllowBurn:    &pTrue,
			AllowLock:    &pTrue,
		},
	},
}

// Two coins that share a nullifier can never both be spent, so a sender must not be able
// to build a transaction containing a collision. This covers both halves of that:
//
//   - The historic attack - two outputs differing only by owner, sharing a salt and amount -
//     no longer collides at all, because the nullifier now binds the owner. Such a transfer
//     is endorsed, and the two coins have distinct nullifiers.
//   - A collision now requires a genuine duplicate coin. The base ledger already rejects
//     that (it will not re-add an existing commitment to the tree), but the notary rejects
//     it first, with an error that says what is wrong.
func TestEndorseRejectsCollidingOutputNullifiers(t *testing.T) {
	n := &Noto{
		Callbacks:      newMockCallbacks(),
		coinSchema:     testSchema("coin"),
		dataSchemaV2:   testSchema("data_v2"),
		manifestSchema: testSchema("manifest"),
	}
	ctx := t.Context()
	fn := types.NotoABI.Functions()["transfer"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	receiverAddress := pldtypes.MustEthAddress("0x2000000000000000000000000000000000000000")
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)
	senderAddress := (*pldtypes.EthAddress)(&senderKey.Address)

	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    contractAddress,
			ContractConfigJson: mustParseJSON(notoNullifierConfig),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: `{
			"to": "receiver@node2",
			"amount": 75,
			"data": "0x1234"
		}`,
	}

	resolvedVerifiers := []*prototk.ResolvedVerifier{
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
			Verifier:     senderAddress.String(),
		},
		{
			Lookup:       "receiver@node2",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     receiverAddress.String(),
		},
	}

	inputCoin := &types.NotoCoin{
		Salt:   pldtypes.RandBytes32(),
		Owner:  senderAddress,
		Amount: pldtypes.Int64ToInt256(150),
	}
	inputStates := []*prototk.EndorsableState{
		{
			SchemaId:      hashName("coin"),
			Id:            pldtypes.RandBytes32().String(),
			StateDataJson: mustParseJSON(inputCoin),
		},
	}

	// endorse builds an endorsement request for the supplied outputs, with a valid sender
	// signature over the unmasked coins, so that the only thing under test is the outputs
	endorse := func(outputCoins []*types.NotoCoin) (*prototk.EndorseTransactionResponse, error) {
		outputStates := make([]*prototk.EndorsableState, len(outputCoins))
		for i, coin := range outputCoins {
			outputStates[i] = &prototk.EndorsableState{
				SchemaId:      hashName("coin"),
				Id:            pldtypes.RandBytes32().String(),
				StateDataJson: mustParseJSON(coin),
			}
		}
		encodedTransfer, err := n.encodeTransferUnmasked(ctx, ethtypes.MustNewAddress(contractAddress),
			[]*types.NotoCoin{inputCoin}, outputCoins)
		require.NoError(t, err)
		signature, err := senderKey.SignDirect(encodedTransfer)
		require.NoError(t, err)

		return n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
			Transaction:       tx,
			ResolvedVerifiers: resolvedVerifiers,
			Inputs:            inputStates,
			Outputs:           outputStates,
			EndorsementRequest: &prototk.AttestationRequest{
				Name: "notary",
			},
			Signatures: []*prototk.AttestationResult{
				{
					Name:     "sender",
					Verifier: &prototk.ResolvedVerifier{Verifier: senderAddress.String()},
					Payload:  pldtypes.HexBytes(signature.CompactRSV()),
				},
			},
		})
	}

	sharedSalt := pldtypes.RandBytes32()
	amount := pldtypes.Int64ToInt256(75)

	// The historic attack shape: one output to the recipient and one to the sender, sharing
	// a salt and amount. This is harmless now - the two coins have distinct nullifiers - so
	// it is endorsed rather than rejected
	toRecipient := &types.NotoCoin{Salt: sharedSalt, Owner: receiverAddress, Amount: amount}
	toSelf := &types.NotoCoin{Salt: sharedSalt, Owner: senderAddress, Amount: amount}

	recipientNullifier, err := calculateNullifier(ctx, toRecipient)
	require.NoError(t, err)
	selfNullifier, err := calculateNullifier(ctx, toSelf)
	require.NoError(t, err)
	require.NotEqual(t, recipientNullifier.String(), selfNullifier.String())

	endorseRes, err := endorse([]*types.NotoCoin{toRecipient, toSelf})
	require.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_ENDORSER_SUBMIT, endorseRes.EndorsementResult)

	// A collision now requires a duplicate coin, which the notary rejects
	_, err = endorse([]*types.NotoCoin{toRecipient, toRecipient})
	assert.Regexp(t, "PD200045", err)

	// The guard only applies to the nullifier variants - other variants spend by state ID
	tx.ContractInfo.ContractConfigJson = mustParseJSON(notoBasicConfigV1)
	endorseRes, err = endorse([]*types.NotoCoin{toRecipient, toRecipient})
	require.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_ENDORSER_SUBMIT, endorseRes.EndorsementResult)
}

func TestValidateDistinctNullifiers(t *testing.T) {
	ctx := t.Context()
	n := testNullifierNoto()
	owner := pldtypes.MustEthAddress("0x1111111111111111111111111111111111111111")
	other := pldtypes.MustEthAddress("0x2222222222222222222222222222222222222222")
	salt := pldtypes.RandBytes32()
	amount := pldtypes.Uint64ToUint256(100)

	coinA := testCoinState("0x01", &types.NotoCoin{Salt: salt, Owner: owner, Amount: amount})
	coinB := testCoinState("0x02", &types.NotoCoin{Salt: pldtypes.RandBytes32(), Owner: owner, Amount: amount})
	// A duplicate coin under a different state ID - the only way to collide now that the
	// nullifier covers every field of the coin
	coinCollidingWithA := testCoinState("0x03", &types.NotoCoin{Salt: salt, Owner: owner, Amount: amount})

	// Distinct coins are fine, including coins that differ only by owner
	require.NoError(t, n.validateDistinctNullifiers(ctx,
		[]*prototk.EndorsableState{coinA},
		[]*prototk.EndorsableState{
			coinB,
			testCoinState("0x07", &types.NotoCoin{Salt: salt, Owner: other, Amount: amount}),
		},
	))

	// An output colliding with an input is caught, as it is nullified by the very
	// transaction that creates it
	err := n.validateDistinctNullifiers(ctx,
		[]*prototk.EndorsableState{coinA},
		[]*prototk.EndorsableState{coinCollidingWithA},
	)
	assert.Regexp(t, "PD200045", err)
	assert.Regexp(t, "0x01", err)
	assert.Regexp(t, "0x03", err)

	// Collisions within a single list are caught
	err = n.validateDistinctNullifiers(ctx, []*prototk.EndorsableState{coinA, coinCollidingWithA})
	assert.Regexp(t, "PD200045", err)

	// The same state appearing in more than one list is not a collision with itself
	require.NoError(t, n.validateDistinctNullifiers(ctx,
		[]*prototk.EndorsableState{coinA},
		[]*prototk.EndorsableState{coinA, coinB},
	))

	// Locked coins are spent by ID, so they are not part of the nullifier check - two
	// identical locked coins are rejected as duplicate states elsewhere, not here
	lockID := pldtypes.RandBytes32()
	lockedCoin := &types.NotoLockedCoin{Salt: salt, LockID: lockID, Owner: owner, Amount: amount}
	lockedData, err := json.Marshal(lockedCoin)
	require.NoError(t, err)
	require.NoError(t, n.validateDistinctNullifiers(ctx, []*prototk.EndorsableState{
		{Id: "0x08", SchemaId: "lockedCoin", StateDataJson: string(lockedData)},
		{Id: "0x09", SchemaId: "lockedCoin", StateDataJson: string(lockedData)},
	}))

	// States that are not coins have no nullifier
	lockInfo := &prototk.EndorsableState{
		Id:            "0x04",
		SchemaId:      "lockInfo",
		StateDataJson: `{"salt": "0x00", "lockId": "0x01", "owner": "0x1111111111111111111111111111111111111111"}`,
	}
	otherLockInfo := &prototk.EndorsableState{
		Id:            "0x05",
		SchemaId:      "lockInfo",
		StateDataJson: `{"salt": "0x00", "lockId": "0x01", "owner": "0x1111111111111111111111111111111111111111"}`,
	}
	require.NoError(t, n.validateDistinctNullifiers(ctx, []*prototk.EndorsableState{lockInfo, otherLockInfo}))

	// A coin that cannot be nullified is an error, not a pass
	err = n.validateDistinctNullifiers(ctx, []*prototk.EndorsableState{
		testCoinState("0x06", &types.NotoCoin{Salt: salt, Owner: owner}), // no amount
	})
	assert.Regexp(t, "PD200044", err)
}

// smtRootIndex is the tree root returned by the mocked merkle tree state below, so tests can
// assert that a proof carries the current commitment tree root
const smtRootIndex = "0x9bc7adede8e6ef3f5a6a3a466a9d9f115d040e8891f77023ebc4825196b55726"

// notoWithMockedCommitmentTree builds a Noto with the schemas a nullifier variant needs, and a state
// query mock that answers the commitment tree lookups as well as the supplied coin queries
func notoWithMockedCommitmentTree(t *testing.T, coinStates map[string][]*prototk.StoredState) *Noto {
	mockCallbacks := newMockCallbacks()
	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		switch req.SchemaId {
		case hashName("merkle_tree_root"):
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{DataJson: fmt.Sprintf(`{"rootIndex":"%s","smtName":"smt_noto"}`, smtRootIndex)},
				},
			}, nil
		case hashName("merkle_tree_node"):
			return &prototk.FindAvailableStatesResponse{}, nil
		}
		if states, found := coinStates[req.SchemaId]; found {
			return &prototk.FindAvailableStatesResponse{States: states}, nil
		}
		return nil, fmt.Errorf("unmocked query for schema %s", req.SchemaId)
	}
	return &Noto{
		Callbacks:            mockCallbacks,
		coinSchema:           testSchema("coin"),
		lockedCoinSchema:     testSchema("lockedCoin"),
		lockInfoSchemaV1:     testSchema("lockInfo_v1"),
		dataSchemaV0:         testSchema("data"),
		dataSchemaV1:         testSchema("data_v1"),
		dataSchemaV2:         testSchema("data_v2"),
		manifestSchema:       testSchema("manifest"),
		merkleTreeRootSchema: testSchema("merkle_tree_root"),
		merkleTreeNodeSchema: testSchema("merkle_tree_node"),
	}
}

// decodeRootAndSignature unpacks the (root, signature) proof that the nullifier variants
// require, which is what NotoNullifiers._createLock / _updateLock decode on-chain
func decodeRootAndSignature(t *testing.T, proof pldtypes.HexBytes) (*pldtypes.HexUint256, pldtypes.HexBytes) {
	paramTypes := abi.ParameterArray{
		{Name: "root", Type: "uint256"},
		{Name: "signature", Type: "bytes"},
	}
	cv, err := paramTypes.DecodeABIData(proof, 0)
	require.NoError(t, err)
	decoded, err := cv.JSON()
	require.NoError(t, err)
	var rootAndSignature struct {
		Root      *pldtypes.HexUint256 `json:"root"`
		Signature pldtypes.HexBytes    `json:"signature"`
	}
	require.NoError(t, json.Unmarshal(decoded, &rootAndSignature))
	return rootAndSignature.Root, rootAndSignature.Signature
}

// A lock in a nullifier variant must consume its unlocked inputs by nullifier, register the
// locked contents by ID, and carry the commitment tree root in the proof - NotoNullifiers
// ._createLock decodes the proof and rejects an unknown root, so a bare signature reverts.
func TestLockNullifierVariantParams(t *testing.T) {
	ctx := t.Context()
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)
	notaryAddress := "0x1000000000000000000000000000000000000000"

	inputCoin := &types.NotoCoinState{
		ID: pldtypes.RandBytes32(),
		Data: types.NotoCoin{
			Salt:   pldtypes.RandBytes32(),
			Owner:  (*pldtypes.EthAddress)(&senderKey.Address),
			Amount: pldtypes.Int64ToInt256(100),
		},
	}
	n := notoWithMockedCommitmentTree(t, map[string][]*prototk.StoredState{
		hashName("coin"): {
			{
				Id:       inputCoin.ID.String(),
				SchemaId: hashName("coin"),
				DataJson: mustParseJSON(inputCoin.Data),
			},
		},
	})

	fn := types.NotoABI.Functions()["lock"]
	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    contractAddress,
			ContractConfigJson: mustParseJSON(notoNullifierConfig),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		// Lock 60 of the 100 available, so there is an unlocked remainder output too
		FunctionParamsJson: `{
			"amount": 60,
			"data": "0x1234"
		}`,
	}

	resolvedVerifiers := []*prototk.ResolvedVerifier{
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
		ResolvedVerifiers: resolvedVerifiers,
	})
	require.NoError(t, err)
	require.Equal(t, prototk.AssembleTransactionResponse_OK, assembleRes.AssemblyResult)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 3) // locked coin + remainder + lock info

	lockedCoinState := assembleRes.AssembledTransaction.OutputStates[0]
	remainderState := assembleRes.AssembledTransaction.OutputStates[1]
	lockState := assembleRes.AssembledTransaction.OutputStates[2]

	// Locked outputs are spent by ID so they carry no nullifier spec; the unlocked remainder does
	require.Empty(t, lockedCoinState.NullifierSpecs)
	require.Len(t, remainderState.NullifierSpecs, 1)
	assert.Equal(t, types.PAYLOAD_DOMAIN_NOTO_NULLIFIER, remainderState.NullifierSpecs[0].PayloadType)

	lockedCoin, err := n.unmarshalLockedCoin(lockedCoinState.StateDataJson)
	require.NoError(t, err)
	remainderCoin, err := n.unmarshalCoin(remainderState.StateDataJson)
	require.NoError(t, err)

	encodedLock, err := n.encodeLock(ctx, ethtypes.MustNewAddress(contractAddress),
		[]*types.NotoCoin{&inputCoin.Data}, []*types.NotoCoin{remainderCoin}, []*types.NotoLockedCoin{lockedCoin})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedLock)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	inputStates := []*prototk.EndorsableState{
		{
			SchemaId:      hashName("coin"),
			Id:            inputCoin.ID.String(),
			StateDataJson: mustParseJSON(inputCoin.Data),
		},
	}
	outputStates := make([]*prototk.EndorsableState, 3)
	for i, s := range assembleRes.AssembledTransaction.OutputStates {
		outputStates[i] = &prototk.EndorsableState{SchemaId: s.SchemaId, Id: *s.Id, StateDataJson: s.StateDataJson}
	}
	dataState := assembleRes.AssembledTransaction.InfoStates[1]
	infoStates := []*prototk.EndorsableState{
		{SchemaId: dataState.SchemaId, Id: *dataState.Id, StateDataJson: dataState.StateDataJson},
	}
	senderAttestation := &prototk.AttestationResult{
		Name:     "sender",
		Verifier: &prototk.ResolvedVerifier{Verifier: senderKey.Address.String()},
		Payload:  signatureBytes,
	}

	endorseRes, err := n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:        tx,
		ResolvedVerifiers:  resolvedVerifiers,
		Inputs:             inputStates,
		Outputs:            outputStates,
		Info:               infoStates,
		EndorsementRequest: &prototk.AttestationRequest{Name: "notary"},
		Signatures:         []*prototk.AttestationResult{senderAttestation},
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_ENDORSER_SUBMIT, endorseRes.EndorsementResult)

	prepareRes, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: resolvedVerifiers,
		InputStates:       inputStates,
		OutputStates:      outputStates,
		InfoStates:        infoStates,
		AttestationResult: []*prototk.AttestationResult{
			senderAttestation,
			{Name: "notary", Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"}},
		},
	})
	require.NoError(t, err)

	createLockABI := interfaceV2Build.ABI.Functions()["createLock"]
	assert.JSONEq(t, mustParseJSON(createLockABI), prepareRes.Transaction.FunctionAbiJson)
	params := decodeFnParams[CreateLockParams](t, createLockABI, prepareRes.Transaction.ParamsJson)
	notoParams := decodeSingleABITuple[types.NotoCreateLockArgs](t, types.NotoCreateLockArgsABI, params.CreateArgs)

	// Unlocked inputs are consumed by nullifier
	inputNullifier, err := calculateNullifier(ctx, &inputCoin.Data)
	require.NoError(t, err)
	assert.Equal(t, []string{inputNullifier.String()}, notoParams.Inputs)

	// Locked contents and the unlocked remainder output are identified by ID
	assert.Equal(t, []string{*lockedCoinState.Id}, notoParams.Contents)
	assert.Equal(t, []string{*remainderState.Id}, notoParams.Outputs)
	assert.Equal(t, pldtypes.MustParseBytes32(*lockState.Id), notoParams.NewLockState)

	// The proof carries the commitment tree root, not just the signature, encoded exactly as
	// the transfer/mint/burn paths encode it
	expectedProof, err := n.encodeRootAndSignature(ctx, contractAddress, "", signatureBytes)
	require.NoError(t, err)
	assert.Equal(t, pldtypes.HexBytes(expectedProof).String(), notoParams.Proof.String())

	root, proofSignature := decodeRootAndSignature(t, notoParams.Proof)
	assert.False(t, root.NilOrZero(), "proof must carry the current commitment tree root")
	assert.Equal(t, signatureBytes.String(), proofSignature.String())
}

// prepareUnlock issues updateLock, which NotoNullifiers._updateLock guards with the same root
// check as createLock - and the locked contents it carries must stay identified by ID, so that
// the spend commitment matches the one computed when the lock was created.
func TestPrepareUnlockNullifierVariantParams(t *testing.T) {
	ctx := t.Context()
	senderAddress := pldtypes.RandAddress()
	lockID := pldtypes.RandBytes32()
	signature := pldtypes.HexBytes("a-signature")

	n := notoWithMockedCommitmentTree(t, nil)
	h := &lockCommon{noto: n}

	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &types.ParsedTransaction{
		ContractAddress: ethtypes.MustNewAddress(contractAddress),
		DomainConfig:    notoNullifierConfig,
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
			From:          "sender@node1",
		},
	}

	lockedCoin := &types.NotoLockedCoin{
		Salt:   pldtypes.RandBytes32(),
		LockID: lockID,
		Owner:  senderAddress,
		Amount: pldtypes.Int64ToInt256(100),
	}
	lockedInputs := []*prototk.EndorsableState{
		{
			SchemaId:      hashName("lockedCoin"),
			Id:            pldtypes.RandBytes32().String(),
			StateDataJson: mustParseJSON(lockedCoin),
		},
	}
	spendOutputs := []*prototk.EndorsableState{
		{
			SchemaId:      hashName("coin"),
			Id:            pldtypes.RandBytes32().String(),
			StateDataJson: mustParseJSON(&types.NotoCoin{Salt: pldtypes.RandBytes32(), Owner: pldtypes.RandAddress(), Amount: pldtypes.Int64ToInt256(100)}),
		},
	}

	lockInfo := types.NotoLockInfo_V1{
		Salt:      pldtypes.RandBytes32(),
		LockID:    lockID,
		Owner:     senderAddress,
		Spender:   senderAddress,
		SpendTxId: pldtypes.RandBytes32(),
	}
	lt := &lockTransition{
		noto:            n,
		prevLockStateID: pldtypes.RandBytes32(),
		prevLockInfo:    lockInfo,
		newLockStateID:  pldtypes.RandBytes32(),
		newLockInfo:     lockInfo,
	}

	paramsJSON, err := h.buildPrepareUnlockParams(ctx, tx, "state-query-context", lt, signature, lockedInputs, spendOutputs, nil, nil)
	require.NoError(t, err)

	updateLockABI := interfaceV2Build.ABI.Functions()["updateLock"]
	params := decodeFnParams[UpdateLockParams](t, updateLockABI, string(paramsJSON))
	notoParams := decodeSingleABITuple[types.NotoUpdateLockArgs](t, types.NotoUpdateLockArgsABI, params.UpdateArgs)

	// Locked contents stay identified by ID
	assert.Equal(t, []string{lockedInputs[0].Id}, notoParams.Contents)

	// The proof carries the commitment tree root
	expectedProof, err := n.encodeRootAndSignature(ctx, contractAddress, "state-query-context", signature)
	require.NoError(t, err)
	assert.Equal(t, pldtypes.HexBytes(expectedProof).String(), notoParams.Proof.String())
	root, proofSignature := decodeRootAndSignature(t, notoParams.Proof)
	assert.False(t, root.NilOrZero())
	assert.Equal(t, signature.String(), proofSignature.String())

	// The spend commitment covers the locked inputs by ID, so it matches what createLock
	// computed over the same locked states
	expectedSpendCommitment, err := n.unlockHashFromIDs_V1(ctx, tx.ContractAddress, lockID, lockInfo.SpendTxId.String(),
		[]string{lockedInputs[0].Id}, []string{spendOutputs[0].Id}, lockInfo.SpendData)
	require.NoError(t, err)
	assert.Equal(t, expectedSpendCommitment, params.SpendCommitment)

	// Other variants get the bare signature - only the nullifier variants check a root
	tx.DomainConfig = notoBasicConfigV1
	paramsJSON, err = h.buildPrepareUnlockParams(ctx, tx, "state-query-context", lt, signature, lockedInputs, spendOutputs, nil, nil)
	require.NoError(t, err)
	params = decodeFnParams[UpdateLockParams](t, updateLockABI, string(paramsJSON))
	notoParams = decodeSingleABITuple[types.NotoUpdateLockArgs](t, types.NotoUpdateLockArgsABI, params.UpdateArgs)
	assert.Equal(t, signature.String(), notoParams.Proof.String())
}

// spendLock needs no root: Noto._spendLock consumes the locked states by ID and never decodes
// the proof, and NotoNullifiers does not override it. So unlock must pass the bare signature
// and identify its locked inputs by ID, matching what the lock registered.
func TestUnlockNullifierVariantParams(t *testing.T) {
	ctx := t.Context()
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)
	senderAddress := (*pldtypes.EthAddress)(&senderKey.Address)
	lockID := pldtypes.RandBytes32()

	n := notoWithMockedCommitmentTree(t, nil)
	fn := types.NotoABI.Functions()["unlock"]

	lockedCoinState := &prototk.EndorsableState{
		SchemaId: hashName("lockedCoin"),
		Id:       pldtypes.RandBytes32().String(),
		StateDataJson: mustParseJSON(&types.NotoLockedCoin{
			Salt:   pldtypes.RandBytes32(),
			LockID: lockID,
			Owner:  senderAddress,
			Amount: pldtypes.Int64ToInt256(100),
		}),
	}
	lockInfoState := &prototk.EndorsableState{
		SchemaId: hashName("lockInfo_v1"),
		Id:       pldtypes.RandBytes32().String(),
		StateDataJson: mustParseJSON(&types.NotoLockInfo_V1{
			Salt:    pldtypes.RandBytes32(),
			LockID:  lockID,
			Owner:   senderAddress,
			Spender: senderAddress,
		}),
	}
	outputState := &prototk.EndorsableState{
		SchemaId: hashName("coin"),
		Id:       pldtypes.RandBytes32().String(),
		StateDataJson: mustParseJSON(&types.NotoCoin{
			Salt:   pldtypes.RandBytes32(),
			Owner:  pldtypes.RandAddress(),
			Amount: pldtypes.Int64ToInt256(100),
		}),
	}
	dataState := &prototk.EndorsableState{
		SchemaId:      hashName("data_v2"),
		Id:            pldtypes.RandBytes32().String(),
		StateDataJson: `{"salt":"0x1b0d6be69d1d5bd7ff9b1b8b7d3b1de4b23e6ba95d8b6c8e4f0eb9c0f6a9f36e","data":"0x1234"}`,
	}

	signatureBytes := pldtypes.HexBytes("a-signature")
	prepareRes, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
			From:          "sender@node1",
			ContractInfo: &prototk.ContractInfo{
				ContractAddress:    "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3",
				ContractConfigJson: mustParseJSON(notoNullifierConfig),
			},
			FunctionAbiJson:   mustParseJSON(fn),
			FunctionSignature: fn.SolString(),
			FunctionParamsJson: fmt.Sprintf(`{
				"lockId": "%s",
				"from": "sender@node1",
				"recipients": [{"to": "receiver@node2", "amount": 100}],
				"data": "0x1234"
			}`, lockID),
		},
		InputStates:  []*prototk.EndorsableState{lockedCoinState, lockInfoState},
		OutputStates: []*prototk.EndorsableState{outputState},
		InfoStates:   []*prototk.EndorsableState{dataState},
		AttestationResult: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: senderAddress.String()},
				Payload:  signatureBytes,
			},
			{Name: "notary", Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"}},
		},
	})
	require.NoError(t, err)

	spendLockABI := interfaceV2Build.ABI.Functions()["spendLock"]
	assert.JSONEq(t, mustParseJSON(spendLockABI), prepareRes.Transaction.FunctionAbiJson)
	params := decodeFnParams[SpendLockParams](t, spendLockABI, prepareRes.Transaction.ParamsJson)
	assert.Equal(t, lockID, params.LockID)
	notoParams := decodeSingleABITuple[types.NotoSpendLockArgs](t, types.NotoSpendLockArgsABI, params.SpendArgs)

	// Locked inputs are identified by ID, never by nullifier
	assert.Equal(t, lockedCoinState.Id, notoParams.Inputs[0])
	// Note this handler also appends the lock info state ID to the inputs, which Noto._spendLock
	// does not expect - it consumes the lock state separately from storage. That is a separate
	// (variant-independent) issue, so it is deliberately not asserted as correct here.

	// The new unlocked outputs are commitments, added to the tree on-chain
	assert.Equal(t, []string{outputState.Id}, notoParams.Outputs)

	// No root: the proof is the bare signature
	assert.Equal(t, signatureBytes.String(), notoParams.Proof.String())
}
