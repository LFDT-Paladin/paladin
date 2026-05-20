/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math/big"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

// ZetoLockInfoState is persisted off-chain (Paladin state DB). The Paladin state row id equals lockId (unique per pool
// on-chain); JSON lockId matches for queries. Mirrors the intent of
// ZetoLockableStorage.ZetoLockInfo in upstream zeto_lockable.sol plus preimage fields required to:
//   - rebuild spend/cancel unlock hashes (IZetoLockableCapability.computeSpendHash / computeCancelHash), and
//   - assemble ZetoSpendLockArgs-shaped calldata (txId, lockedOutputs, outputs, proof, data) for spendLock/cancelLock.
//
// Upstream ZetoSpendLockArgs omits lockedInputs — the contract reads them from lock storage. Paladin persists the
// locked collateral commitment strings (uint256 hex, from lockedOutputCoins at createLock) in LockedOutputs. spendLock
// and cancelLock both use this same set as proof inputs. SpendOutputs / CancelOutputs are pre-pinned Paladin coin state
// ids (info states) for the proposed spend and cancel paths; SpendData / CancelData hold recipient JSON for assemble.
//
// Commitment preimages use the same tuple shape as zeto_lockable._buildUnlockHash (domain-separated spend vs cancel).
type ZetoLockInfoState struct {
	Salt   *pldtypes.HexUint256 `json:"salt"`
	LockID pldtypes.Bytes32     `json:"lockId"`

	Owner   string `json:"owner"`   // 0x-prefixed ETH address hex
	Spender string `json:"spender"` // 0x-prefixed ETH address hex

	// Locked collateral commitments (shared proof inputs for spendLock and cancelLock).
	LockedOutputs []string `json:"lockedOutputs"`

	// Preimage for spend commitment (see _buildUnlockHash with _SPEND_HASH_DOMAIN).
	SpendOutputs []string          `json:"spendOutputs"`
	SpendData    pldtypes.HexBytes `json:"spendData"`

	// Preimage for cancel commitment (see _buildUnlockHash with _CANCEL_HASH_DOMAIN).
	CancelOutputs []string          `json:"cancelOutputs"`
	CancelData    pldtypes.HexBytes `json:"cancelData"`

	// Opaque copy of IZetoFungible_V1.createLock unlockData from the createLock request.
	// No omitempty: ZetoLockInfoState AbiStateSchema validation requires this key (bytes may be empty "0x").
	UnlockData pldtypes.HexBytes `json:"unlockData"`

	hash *pldtypes.HexUint256 `json:"-"`
}

// ZetoLockInfoStateABI is registered as the 6th AbiStateSchema (see GetStateSchemas); query with Equal("lockId", lockID)
// (pass pldtypes.Bytes32; same as Noto lock queries — avoids query value shape drift).
var ZetoLockInfoStateABI = &abi.Parameter{
	Name:         "ZetoLockInfoState",
	Type:         "tuple",
	InternalType: "struct ZetoLockInfoState",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "uint256"},
		{Name: "lockId", Type: "bytes32", Indexed: true},
		{Name: "owner", Type: "address"},
		{Name: "spender", Type: "address"},
		{Name: "lockedOutputs", Type: "uint256[]"},
		{Name: "spendOutputs", Type: "uint256[]"},
		{Name: "spendData", Type: "bytes"},
		{Name: "cancelOutputs", Type: "uint256[]"},
		{Name: "cancelData", Type: "bytes"},
		{Name: "unlockData", Type: "bytes"},
	},
}

func (z *ZetoLockInfoState) Hash(ctx context.Context) (*pldtypes.HexUint256, error) {
	if z.hash != nil {
		return z.hash, nil
	}
	b, err := json.Marshal(z)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if z.Salt != nil {
		h.Write(z.Salt.Int().Bytes())
	}
	h.Write(b)
	sum := h.Sum(nil)
	hi := new(big.Int).SetBytes(sum)
	out := pldtypes.HexUint256(*hi)
	z.hash = &out
	return z.hash, nil
}
