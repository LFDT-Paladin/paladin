// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
