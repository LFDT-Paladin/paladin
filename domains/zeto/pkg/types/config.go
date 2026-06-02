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
	"context"
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

// DomainConfigSchema values describe how on-chain registration bytes were encoded (see domain_config_codec.go).
const (
	DomainConfigSchemaV0 = "v0"
	DomainConfigSchemaV1 = "v1"
)

// ZetoVariantV0 / ZetoVariantV1 are legacy names for the on-chain zetoVariant field; see ZetoFungibleABIVersion (axis 1 in versions.go).
var (
	ZetoVariantV0 = ZetoFungibleV0ABI
	ZetoVariantV1 = ZetoFungibleV1ABI
)

// DomainFactoryConfig is the configuration for a Zeto domain
// to provision new domain instances based on a factory contract
// and avalable implementation contracts
type DomainFactoryConfig struct {
	DomainContracts DomainConfigContracts           `json:"domainContracts"`
	SnarkProver     zetosignerapi.SnarkProverConfig `json:"snarkProver"`
	// FactoryVersion selects ZetoPaladinFactoryVersion (ZetoFactory.sol vs ZetoFactoryV1.sol); see versions.go.
	FactoryVersion int64 `json:"factoryVersion,omitempty"`
}

type DomainConfigContracts struct {
	Implementations []*DomainContract `yaml:"implementations"`
}

type DomainContract struct {
	Name     string                  `yaml:"name"`
	Circuits *zetosignerapi.Circuits `yaml:"circuits"`
	// BundleID, when set, identifies this implementation for v1 on-chain CircuitBundleId (Phase A multi-version).
	BundleID string `yaml:"bundleId,omitempty" json:"bundleId,omitempty"`
	// ForZetoVariant, when set, restricts this row to deploys that declare that zetoVariant (nil = wildcard).
	ForZetoVariant *uint64 `yaml:"forZetoVariant,omitempty" json:"forZetoVariant,omitempty"`
}

func (d *DomainFactoryConfig) GetCircuits(ctx context.Context, tokenName string) (*zetosignerapi.Circuits, error) {
	for _, contract := range d.DomainContracts.Implementations {
		if contract.Name == tokenName {
			return contract.Circuits, nil
		}
	}
	return nil, i18n.NewError(ctx, msgs.MsgContractNotFound, tokenName)
}

// GetCircuitsForDeploy resolves circuits for PrepareDeploy using optional v1 selectors.
// When circuitBundleId is non-empty, the implementation with matching BundleID is used.
// Otherwise implementations are filtered by token Name and optional ForZetoVariant (nil matches any variant).
// If no variant-specific row matches, GetCircuits(tokenName) legacy behavior is used.
func (d *DomainFactoryConfig) GetCircuitsForDeploy(ctx context.Context, tokenName string, zetoVariant uint64, circuitBundleId string) (*zetosignerapi.Circuits, error) {
	if circuitBundleId != "" {
		var match *DomainContract
		for _, impl := range d.DomainContracts.Implementations {
			if impl.BundleID != circuitBundleId {
				continue
			}
			if match != nil {
				return nil, i18n.NewError(ctx, msgs.MsgDuplicateZetoCircuitBundleId, circuitBundleId)
			}
			match = impl
		}
		if match == nil {
			return nil, i18n.NewError(ctx, msgs.MsgContractNotFound, circuitBundleId)
		}
		return match.Circuits, nil
	}

	var matches []*DomainContract
	for _, impl := range d.DomainContracts.Implementations {
		if impl.Name != tokenName {
			continue
		}
		if impl.ForZetoVariant != nil && *impl.ForZetoVariant != zetoVariant {
			continue
		}
		matches = append(matches, impl)
	}
	switch len(matches) {
	case 1:
		return matches[0].Circuits, nil
	case 0:
		for _, impl := range d.DomainContracts.Implementations {
			if impl.Name == tokenName && impl.ForZetoVariant != nil {
				return nil, i18n.NewError(ctx, msgs.MsgContractNotFound, fmt.Sprintf("%s@%d", tokenName, zetoVariant))
			}
		}
		return d.GetCircuits(ctx, tokenName)
	default:
		return nil, i18n.NewError(ctx, msgs.MsgAmbiguousZetoCircuitImplementation, tokenName)
	}
}

func (d *DomainFactoryConfig) GetCircuit(ctx context.Context, tokenName, method string) (*zetosignerapi.Circuit, error) {
	for _, contract := range d.DomainContracts.Implementations {
		if contract.Name == tokenName {
			return (*contract.Circuits)[method], nil
		}
	}
	return nil, i18n.NewError(ctx, msgs.MsgContractNotFound, tokenName)
}

// DomainInstanceConfig is the domain instance config, which are
// sent to the domain contract deployment request to be published
// on-chain. This must include sufficient information for a Paladin
// node to fully initialize the domain instance, based on only
// on-chain information.
type DomainInstanceConfig struct {
	TokenName string                  `json:"tokenName"`
	Circuits  *zetosignerapi.Circuits `json:"circuits"`
	// ConfigSchema is "v0" (legacy ABI-only bytes) or "v1" (prefixed encoding); set when decoding on-chain config.
	ConfigSchema string `json:"configSchema,omitempty"`
	// ZetoVariant is ZetoFungibleABIVersion (Paladin IZetoFungible*.json axis); see versions.go.
	ZetoVariant pldtypes.HexUint64 `json:"zetoVariant,omitempty"`
	// FactoryVersion is ZetoPaladinFactoryVersion at deploy; see versions.go.
	FactoryVersion int64 `json:"factoryVersion,omitempty"`
	// CircuitBundleId references DomainContract.BundleID when circuits are resolved by opaque id.
	CircuitBundleId string `json:"circuitBundleId,omitempty"`
}

// zetoDomainCircuitsComponents is shared by legacy and v1 domain instance config ABI tuples.
var zetoDomainCircuitsComponents = []*abi.Parameter{
	{Type: "tuple", Name: "deposit", Components: []*abi.Parameter{{Type: "string", Name: "name"}, {Type: "string", Name: "type"}, {Type: "bool", Name: "usesEncryption"}, {Type: "bool", Name: "usesNullifiers"}, {Type: "bool", Name: "usesKyc"}}},
	{Type: "tuple", Name: "withdraw", Components: []*abi.Parameter{{Type: "string", Name: "name"}, {Type: "string", Name: "type"}, {Type: "bool", Name: "usesEncryption"}, {Type: "bool", Name: "usesNullifiers"}, {Type: "bool", Name: "usesKyc"}}},
	{Type: "tuple", Name: "transfer", Components: []*abi.Parameter{{Type: "string", Name: "name"}, {Type: "string", Name: "type"}, {Type: "bool", Name: "usesEncryption"}, {Type: "bool", Name: "usesNullifiers"}, {Type: "bool", Name: "usesKyc"}}},
	{Type: "tuple", Name: "transferLocked", Components: []*abi.Parameter{{Type: "string", Name: "name"}, {Type: "string", Name: "type"}, {Type: "bool", Name: "usesEncryption"}, {Type: "bool", Name: "usesNullifiers"}, {Type: "bool", Name: "usesKyc"}}},
}

// DomainInstanceConfigABI is the ABI for the DomainInstanceConfig,
// used to encode and decode the on-chain data for the domain config
var DomainInstanceConfigABI = &abi.ParameterArray{
	{
		Type: "string",
		Name: "tokenName",
	},
	{
		Type:       "tuple",
		Name:       "circuits",
		Components: zetoDomainCircuitsComponents,
	},
}

// ZetoDomainConfigID_V1 prefixes versioned on-chain domain instance configuration bytes (Phase A).
var ZetoDomainConfigID_V1 = pldtypes.MustParseHexBytes("0x00020001")

// DomainInstanceConfigV1ABI encodes the payload after ZetoDomainConfigID_V1.
var DomainInstanceConfigV1ABI = &abi.ParameterArray{
	{Name: "tokenName", Type: "string"},
	{Name: "zetoVariant", Type: "uint256"},
	{Name: "factoryVersion", Type: "uint256"},
	{Name: "circuitBundleId", Type: "string"},
	{Name: "circuits", Type: "tuple", Components: zetoDomainCircuitsComponents},
}

// marks the version of the Zeto transaction data schema
var ZetoTransactionDataID_V0 = ethtypes.MustNewHexBytes0xPrefix("0x00010000")

type ZetoTransactionData_V0 struct {
	TransactionID pldtypes.Bytes32   `json:"transactionId"`
	InfoStates    []pldtypes.Bytes32 `json:"infoStates"`
}

var ZetoTransactionDataABI_V0 = &abi.ParameterArray{
	{Name: "transactionId", Type: "bytes32"},
	{Name: "infoStates", Type: "bytes32[]"},
}

type DomainHandler = domain.DomainHandler[DomainInstanceConfig]
type DomainCallHandler = domain.DomainCallHandler[DomainInstanceConfig]
type ParsedTransaction = domain.ParsedTransaction[DomainInstanceConfig]
