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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/hyperledger-firefly/signer/pkg/abi"
	"github.com/hyperledger-firefly/signer/pkg/ethtypes"
)

// DomainFactoryConfig is the configuration for a Zeto domain
// to provision new domain instances based on a factory contract
// and avalable implementation contracts
//
// Implementations are given either per variant in Variants, or in DomainContracts, which is read as the single variant
// V0 with its proving artifacts directly under the snarkProver directories rather than in a V0/ subdirectory.
type DomainFactoryConfig struct {
	DomainContracts *DomainConfigContracts          `json:"domainContracts,omitempty"`
	Variants        map[string]*VariantConfig       `json:"variants,omitempty"`
	DefaultVariant  string                          `json:"defaultVariant,omitempty"`
	SnarkProver     zetosignerapi.SnarkProverConfig `json:"snarkProver"`
}

type DomainConfigContracts struct {
	Implementations []*DomainContract `yaml:"implementations"`
}

type VariantConfig struct {
	Implementations []*DomainContract `json:"implementations"`
}

type DomainContract struct {
	Name string `yaml:"name"`
	// RegisteredName is the name the implementation is registered under on the factory. It defaults to Name, and only
	// needs setting when one factory holds the same token type at more than one variant.
	RegisteredName string                  `yaml:"registeredName,omitempty" json:"registeredName,omitempty"`
	Circuits       *zetosignerapi.Circuits `yaml:"circuits"`
}

// DomainVariants is the validated form of DomainFactoryConfig's implementations, whichever format they were given in.
type DomainVariants struct {
	Implementations map[ZetoVariant][]*DomainContract
	// DefaultVariant is nil when several variants are configured and none is the default
	DefaultVariant *ZetoVariant
	// ArtifactsPerVariant is false for the DomainContracts format, whose artifacts sit directly under the snarkProver
	// directories.
	ArtifactsPerVariant bool
}

func (d *DomainFactoryConfig) ResolveVariants(ctx context.Context) (*DomainVariants, error) {
	if d.Variants == nil {
		var impls []*DomainContract
		if d.DomainContracts != nil {
			impls = d.DomainContracts.Implementations
		}
		v0 := ZetoVariant_V0
		return newDomainVariants(ctx, map[ZetoVariant][]*DomainContract{ZetoVariant_V0: impls}, &v0, false)
	}
	if d.DomainContracts != nil {
		return nil, i18n.NewError(ctx, msgs.MsgZetoConfigFormatConflict)
	}
	impls := make(map[ZetoVariant][]*DomainContract, len(d.Variants))
	for key, vc := range d.Variants {
		v, err := ParseZetoVariantKey(ctx, key)
		if err != nil {
			return nil, err
		}
		var contracts []*DomainContract
		if vc != nil {
			contracts = vc.Implementations
		}
		impls[v] = contracts
	}
	var defaultVariant *ZetoVariant
	switch {
	case d.DefaultVariant != "":
		v, err := ParseZetoVariantKey(ctx, d.DefaultVariant)
		if err != nil {
			return nil, err
		}
		if _, ok := impls[v]; !ok {
			return nil, i18n.NewError(ctx, msgs.MsgZetoVariantNotConfigured, v)
		}
		defaultVariant = &v
	case len(impls) == 1:
		for v := range impls {
			defaultVariant = &v
		}
	}
	return newDomainVariants(ctx, impls, defaultVariant, true)
}

func newDomainVariants(ctx context.Context, impls map[ZetoVariant][]*DomainContract, defaultVariant *ZetoVariant, artifactsPerVariant bool) (*DomainVariants, error) {
	registeredNames := map[string]bool{}
	for v, contracts := range impls {
		names := map[string]bool{}
		for _, c := range contracts {
			if names[c.Name] {
				return nil, i18n.NewError(ctx, msgs.MsgDuplicateZetoImplementation, c.Name, v)
			}
			names[c.Name] = true
			// TODO: accept non-fungible implementations at V1 once the non-fungible handlers build 0.5.x calldata
			if v != ZetoVariant_V0 && (c.Name == constants.TOKEN_NF_ANON || c.Name == constants.TOKEN_NF_ANON_NULLIFIER) {
				return nil, i18n.NewError(ctx, msgs.MsgZetoNonFungibleVariantUnsupported, c.Name)
			}
			if c.RegisteredName == "" {
				c.RegisteredName = c.Name
			}
			if registeredNames[c.RegisteredName] {
				return nil, i18n.NewError(ctx, msgs.MsgDuplicateZetoRegisteredName, c.RegisteredName)
			}
			registeredNames[c.RegisteredName] = true
			if c.Circuits != nil {
				c.Circuits.Init()
			}
		}
	}
	return &DomainVariants{
		Implementations:     impls,
		DefaultVariant:      defaultVariant,
		ArtifactsPerVariant: artifactsPerVariant,
	}, nil
}

func (dv *DomainVariants) GetImplementation(ctx context.Context, variant ZetoVariant, tokenName string) (*DomainContract, error) {
	contracts, ok := dv.Implementations[variant]
	if !ok {
		return nil, i18n.NewError(ctx, msgs.MsgZetoVariantNotConfigured, variant)
	}
	for _, c := range contracts {
		if c.Name == tokenName {
			return c, nil
		}
	}
	return nil, i18n.NewError(ctx, msgs.MsgZetoImplementationNotFound, tokenName, variant)
}

// DomainInstanceConfig is the domain instance config, which are
// sent to the domain contract deployment request to be published
// on-chain. This must include sufficient information for a Paladin
// node to fully initialize the domain instance, based on only
// on-chain information.
type DomainInstanceConfig struct {
	TokenName string                  `json:"tokenName"`
	Circuits  *zetosignerapi.Circuits `json:"circuits"`
	// Variant is the pool's ZetoVariant, recorded at deploy; see versions.go. It selects the private ABI, the target-core
	// event ABIs and the proving-artifact tree.
	Variant ZetoVariant `json:"variant"`
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

// ZetoDomainConfigID_V1 prefixes versioned on-chain domain instance configuration bytes. Config without it is
// DomainInstanceConfigABI, and the pool is variant V0.
var ZetoDomainConfigID_V1 = pldtypes.MustParseHexBytes("0x00020001")

// DomainInstanceConfigV1ABI encodes the payload after ZetoDomainConfigID_V1.
var DomainInstanceConfigV1ABI = &abi.ParameterArray{
	{Name: "tokenName", Type: "string"},
	{Name: "variant", Type: "uint256"},
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
