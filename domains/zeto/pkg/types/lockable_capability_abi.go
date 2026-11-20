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

import "github.com/hyperledger-firefly/signer/pkg/abi"

// LockableCapabilityCreateLockABI is ILockableCapability / ZetoLockable.createLock on the deployed pool
// (bytes createArgs, bytes32 spendCommitment, bytes32 cancelCommitment, bytes data) — not IZetoFungible_V1,
// which is only the Paladin domain RPC surface.
var LockableCapabilityCreateLockABI = &abi.Entry{
	Type:            abi.Function,
	Name:            METHOD_CREATE_LOCK,
	StateMutability: abi.NonPayable,
	Inputs: abi.ParameterArray{
		{Name: "createArgs", Type: "bytes"},
		{Name: "spendCommitment", Type: "bytes32"},
		{Name: "cancelCommitment", Type: "bytes32"},
		{Name: "data", Type: "bytes"},
	},
	Outputs: abi.ParameterArray{
		{Name: "lockId", Type: "bytes32"},
	},
}

// LockableCapabilitySpendLockABI is ILockableCapability / ZetoLockable.spendLock on the deployed pool.
var LockableCapabilitySpendLockABI = &abi.Entry{
	Type:            abi.Function,
	Name:            METHOD_SPEND_LOCK,
	StateMutability: abi.NonPayable,
	Inputs: abi.ParameterArray{
		{Name: "lockId", Type: "bytes32"},
		{Name: "spendArgs", Type: "bytes"},
		{Name: "data", Type: "bytes"},
	},
}

// LockableCapabilityCancelLockABI is ILockableCapability / ZetoLockable.cancelLock on the deployed pool.
var LockableCapabilityCancelLockABI = &abi.Entry{
	Type:            abi.Function,
	Name:            METHOD_CANCEL_LOCK,
	StateMutability: abi.NonPayable,
	Inputs: abi.ParameterArray{
		{Name: "lockId", Type: "bytes32"},
		{Name: "cancelArgs", Type: "bytes"},
		{Name: "data", Type: "bytes"},
	},
}
