/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the License); you may not use this file except in compliance with
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

import "github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"

// Paladin Zeto uses three independent version axes. They can advance separately:
//
//  (1) ZetoFungibleABIVersion — Paladin private / JSON ABI used to validate fungible transaction signatures against
//      what the domain plugin expects (domains/zeto/pkg/types/abis/IZetoFungible.json vs IZetoFungible_V1.json).
//      This is what DomainInstanceConfig.ZetoVariant stores on-chain for v1 configs (historical field name "zetoVariant").
//
//  (2) ZetoTargetContractABIVersion — Target Zeto token core interface generation from upstream zeto-contracts
//      (domains/zeto/internal/zeto/abis/IZeto.json vs IZeto_V1.json). Example mapping today: v0 → zeto-contracts ~v0.2.x,
//      v1 → ~v0.5.x. Used for event ABI merging and other core-token-shaped logic; not necessarily in lockstep with (1).
//
//  (3) ZetoPaladinFactoryVersion — Paladin Solidity domain factory wrapper (solidity/contracts/domains/zeto/ZetoFactory.sol
//      vs ZetoFactoryV1.sol) and thus which underlying ZetoTokenFactory* is invoked. Recorded as DomainInstanceConfig.FactoryVersion.

// ZetoFungibleABIVersion selects pkg/types/abis/IZetoFungible*.json for fungible handler ABI validation.
type ZetoFungibleABIVersion = pldtypes.HexUint64

const (
	// ZetoFungibleV0ABI selects IZetoFungible.json.
	ZetoFungibleV0ABI ZetoFungibleABIVersion = 0
	// ZetoFungibleV1ABI selects IZetoFungible_V1.json.
	ZetoFungibleV1ABI ZetoFungibleABIVersion = 1
)

// UseZetoOnchainPackedProofCalldata is true when the target Zeto fungible token expects `transfer` / `deposit` / `withdraw`
// calldata with a single ABI-encoded `bytes proof` blob (Groth16 struct, optionally prefixed by root or encryption metadata),
// as implemented in upstream zeto solidity (~v0.5.x). V0 Paladin configs keep discrete proof tuple + public fields in Prepare.
func UseZetoOnchainPackedProofCalldata(zetoVariant ZetoFungibleABIVersion) bool {
	return zetoVariant != ZetoFungibleV0ABI
}

// ZetoTargetContractABIVersion selects internal/zeto/abis/IZeto*.json for core token interface shape (events, etc.).
type ZetoTargetContractABIVersion uint8

const (
	// ZetoTargetContractABI_V0 selects IZeto.json (zeto-contracts v0.2.x-style core interface).
	ZetoTargetContractABI_V0 ZetoTargetContractABIVersion = 0
	// ZetoTargetContractABI_V1 selects IZeto_V1.json (zeto-contracts v0.5.x-style core interface).
	ZetoTargetContractABI_V1 ZetoTargetContractABIVersion = 1
)

// Every merged target-core generation registered for domain event dispatch must be listed here when adding a new IZeto*.json generation.
var SupportedZetoTargetContractABIVersions = []ZetoTargetContractABIVersion{
	ZetoTargetContractABI_V0,
	ZetoTargetContractABI_V1,
}

// ZetoPaladinFactoryVersion selects PrepareDeploy factory ABI / Solidity wrapper (ZetoFactory vs ZetoFactoryV1).
type ZetoPaladinFactoryVersion int64

const (
	// ZetoPaladinFactoryV0 is ZetoFactory.sol (factoryVersion 0 on DomainFactoryConfig / DomainInstanceConfig).
	ZetoPaladinFactoryV0 ZetoPaladinFactoryVersion = 0
	// ZetoPaladinFactoryV1 is ZetoFactoryV1.sol (factoryVersion 1).
	ZetoPaladinFactoryV1 ZetoPaladinFactoryVersion = 1
)
