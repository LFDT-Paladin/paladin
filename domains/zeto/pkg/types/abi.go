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

package types

import (
	_ "embed"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

//go:embed abis/IZetoFungible.json
var zetoFungibleJSON []byte

//go:embed abis/IZetoFungibleV1.json
var zetoFungibleV1JSON []byte

//go:embed abis/IZetoNonFungible.json
var zetoNonFungibleJSON []byte

var ZetoFungibleABI = solutils.MustParseBuildABI(zetoFungibleJSON)

// ZetoFungibleABI_V1 embeds the generation-1 fungible interface ABI (starts as a copy of V0; diverge when on-chain IZeto changes).
var ZetoFungibleABI_V1 = solutils.MustParseBuildABI(zetoFungibleV1JSON)

var ZetoNonFungibleABI = solutils.MustParseBuildABI(zetoNonFungibleJSON)

// ZetoFungibleFunctionForVariant resolves the function entry using ZetoFungibleABIVersion (axis 1; IZetoFungible*.json).
func ZetoFungibleFunctionForVariant(zetoVariant pldtypes.HexUint64, name string) *abi.Entry {
	build := ZetoFungibleABI
	if zetoVariant != ZetoFungibleV0ABI {
		build = ZetoFungibleABI_V1
	}
	return build.Functions()[name]
}

const (
	METHOD_MINT            = "mint"
	METHOD_TRANSFER        = "transfer"
	METHOD_TRANSFER_LOCKED = "transferLocked"
	METHOD_LOCK            = "lock"
	METHOD_DEPOSIT         = "deposit"
	METHOD_WITHDRAW        = "withdraw"
	METHOD_BALANCE_OF      = "balanceOf"
)

type InitializerParams struct {
	Name      string `json:"name"`
	Symbol    string `json:"symbol"`
	TokenName string `json:"tokenName"`
	// InitialOwner string `json:"initialOwner"` // TODO: allow the initial owner to be specified by the deploy request

	// DomainConfigSchema "v1" opts into prefixed on-chain config encoding (see types.DecodeDomainInstanceConfig).
	// Empty or "v0" keeps legacy encoding for registration bytes.
	DomainConfigSchema string `json:"domainConfigSchema,omitempty"`
	// ZetoVariant is ZetoFungibleABIVersion persisted for v1 configs (see versions.go).
	ZetoVariant uint64 `json:"zetoVariant,omitempty"`
	// FactoryVersion is ZetoPaladinFactoryVersion (see versions.go).
	FactoryVersion int64 `json:"factoryVersion,omitempty"`
	// CircuitBundleId selects DomainContract.bundleId for circuit resolution when non-empty.
	CircuitBundleId string `json:"circuitBundleId,omitempty"`
}

type DeployParams struct {
	TransactionID string            `json:"transactionId"`
	Data          pldtypes.HexBytes `json:"data"`
	TokenName     string            `json:"tokenName"`
	Name          string            `json:"name"`
	Symbol        string            `json:"symbol"`
	InitialOwner  string            `json:"initialOwner"`
	IsNonFungible bool              `json:"isNonFungible"`
}

type NonFungibleMintParams struct {
	Mints []*NonFungibleTransferParamEntry `json:"mints"`
}

type FungibleMintParams struct {
	Mints []*FungibleTransferParamEntry `json:"mints"`
}

type FungibleTransferParams struct {
	Transfers []*FungibleTransferParamEntry `json:"transfers"`
}

type FungibleTransferParamEntry struct {
	To     string               `json:"to"`
	Amount *pldtypes.HexUint256 `json:"amount"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type FungibleTransferLockedParams struct {
	LockedInputs []*pldtypes.HexUint256        `json:"lockedInputs"`
	Delegate     string                        `json:"delegate"`
	Transfers    []*FungibleTransferParamEntry `json:"transfers"`
}

type NonFungibleTransferParams struct {
	Transfers []*NonFungibleTransferParamEntry `json:"transfers"`
}

type NonFungibleTransferParamEntry struct {
	To      string               `json:"to"`
	URI     string               `json:"uri,omitempty"`
	TokenID *pldtypes.HexUint256 `json:"tokenID"`
}

type LockParams struct {
	Amount   *pldtypes.HexUint256 `json:"amount"`
	Delegate *pldtypes.EthAddress `json:"delegate"`
}

type DepositParams struct {
	Amount *pldtypes.HexUint256 `json:"amount"`
}

type WithdrawParams struct {
	Amount *pldtypes.HexUint256 `json:"amount"`
}

type FungibleBalanceOfParam struct {
	Account string `json:"account"`
}

type BalanceOfResult struct {
	TotalBalance *pldtypes.HexUint256 `json:"totalBalance"`
	TotalStates  *pldtypes.HexUint256 `json:"totalStates"`
	Overflow     bool                 `json:"overflow"`
}
