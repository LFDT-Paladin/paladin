/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import (
	"context"
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

// Domain tags match ZetoLockable._SPEND_HASH_DOMAIN / _CANCEL_HASH_DOMAIN in zeto_lockable.sol.
var (
	ZetoSpendCommitmentDomain = pldtypes.Bytes32Keccak([]byte("Zeto.spendCommitment.v1"))
	ZetoCancelCommitmentDomain = pldtypes.Bytes32Keccak([]byte("Zeto.cancelCommitment.v1"))
)

var (
	zetoUint256ArrayABI = abi.ParameterArray{{Type: "uint256[]"}}
	zetoUnlockHashTupleABI = abi.ParameterArray{
		{Type: "bytes32"},
		{Type: "bytes32"},
		{Type: "bytes32"},
		{Type: "bytes32"},
		{Type: "bytes32"},
	}
)

// ComputeZetoUnlockCommitment implements ZetoLockable._buildUnlockHash / IZetoLockableCapability.computeSpendHash
// and computeCancelHash (domain selects spend vs cancel).
func ComputeZetoUnlockCommitment(ctx context.Context, domain pldtypes.Bytes32, lockedInputs, lockedOutputs, outputs []string, data []byte) (pldtypes.Bytes32, error) {
	encLockedInputs, err := encodeZetoUint256Array(ctx, lockedInputs)
	if err != nil {
		return pldtypes.Bytes32{}, err
	}
	encLockedOutputs, err := encodeZetoUint256Array(ctx, lockedOutputs)
	if err != nil {
		return pldtypes.Bytes32{}, err
	}
	encOutputs, err := encodeZetoUint256Array(ctx, outputs)
	if err != nil {
		return pldtypes.Bytes32{}, err
	}
	tupleJSON, err := json.Marshal([]any{
		domain.HexString0xPrefix(),
		pldtypes.Bytes32Keccak(encLockedInputs).HexString0xPrefix(),
		pldtypes.Bytes32Keccak(encLockedOutputs).HexString0xPrefix(),
		pldtypes.Bytes32Keccak(encOutputs).HexString0xPrefix(),
		pldtypes.Bytes32Keccak(data).HexString0xPrefix(),
	})
	if err != nil {
		return pldtypes.Bytes32{}, err
	}
	encoded, err := zetoUnlockHashTupleABI.EncodeABIDataJSONCtx(ctx, tupleJSON)
	if err != nil {
		return pldtypes.Bytes32{}, err
	}
	return pldtypes.Bytes32Keccak(encoded), nil
}

// ComputeZetoSpendCommitment hashes the spendLock preimage pinned at createLock.
func ComputeZetoSpendCommitment(ctx context.Context, lockedInputs, lockedOutputs, outputs []string, data []byte) (pldtypes.Bytes32, error) {
	return ComputeZetoUnlockCommitment(ctx, ZetoSpendCommitmentDomain, lockedInputs, lockedOutputs, outputs, data)
}

// ComputeZetoCancelCommitment hashes the cancelLock preimage pinned at createLock.
func ComputeZetoCancelCommitment(ctx context.Context, lockedInputs, lockedOutputs, outputs []string, data []byte) (pldtypes.Bytes32, error) {
	return ComputeZetoUnlockCommitment(ctx, ZetoCancelCommitmentDomain, lockedInputs, lockedOutputs, outputs, data)
}

func encodeZetoUint256Array(ctx context.Context, values []string) ([]byte, error) {
	if values == nil {
		values = []string{}
	}
	jsonValues, err := json.Marshal([]any{values})
	if err != nil {
		return nil, err
	}
	return zetoUint256ArrayABI.EncodeABIDataJSONCtx(ctx, jsonValues)
}
