/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import (
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// ComputeZetoLockIDV1 returns keccak256(abi.encode(pool, publicCreateLockMsgSender, txId)) matching
// ZetoLockable._computeLockId(bytes32) in zeto-contracts ~v0.5.x (abi.encode pads each argument to 32 bytes).
func ComputeZetoLockIDV1(pool, publicCreateLockMsgSender pldtypes.EthAddress, txID pldtypes.Bytes32) pldtypes.Bytes32 {
	var enc [96]byte
	copy(enc[12:32], pool[:])
	copy(enc[44:64], publicCreateLockMsgSender[:])
	copy(enc[64:96], txID[:])
	return pldtypes.Bytes32Keccak(enc[:])
}
