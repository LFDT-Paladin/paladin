// Copyright © 2026 Kaleido, Inc.
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

package contracts

import (
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

// NotaryMode is the operating mode of the Noto notary.
type NotaryMode string

const (
	NotaryModeBasic NotaryMode = "basic"
	NotaryModeHooks NotaryMode = "hooks"
)

// MintParams are the parameters for a Noto mint transaction.
type MintParams struct {
	To     string               `json:"to"`
	Amount *pldtypes.HexUint256 `json:"amount"`
	Data   pldtypes.HexBytes    `json:"data"`
}

// TransferParams are the parameters for a Noto transfer transaction.
type TransferParams struct {
	To     string               `json:"to"`
	Amount *pldtypes.HexUint256 `json:"amount"`
	Data   pldtypes.HexBytes    `json:"data"`
}

// NotoABI contains the subset of the Noto private contract ABI used by the test suites.
var NotoABI = abi.ABI{
	{
		Type: abi.Function,
		Name: "mint",
		Inputs: abi.ParameterArray{
			{Name: "to", Type: "string"},
			{Name: "amount", Type: "uint256"},
			{Name: "data", Type: "bytes"},
		},
	},
	{
		Type: abi.Function,
		Name: "transfer",
		Inputs: abi.ParameterArray{
			{Name: "to", Type: "string"},
			{Name: "amount", Type: "uint256"},
			{Name: "data", Type: "bytes"},
		},
	},
}

// NotoConstructorABI describes the constructor inputs accepted by the Paladin
// Noto domain handler. This is a domain-level interface, not a Solidity ABI.
var NotoConstructorABI = abi.ABI{
	{Type: abi.Constructor, Inputs: abi.ParameterArray{
		{Name: "notary", Type: "string"},
		{Name: "notaryMode", Type: "string"},
		{Name: "options", Type: "tuple", Components: abi.ParameterArray{
			{Name: "hooks", Type: "tuple", Components: abi.ParameterArray{
				{Name: "publicAddress", Type: "string"},
				{Name: "privateAddress", Type: "string"},
				{Name: "privateGroup", Type: "tuple", Components: abi.ParameterArray{
					{Name: "salt", Type: "bytes32"},
					{Name: "members", Type: "string[]"},
				}},
			}},
		}},
	}},
}
