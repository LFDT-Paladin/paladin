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
	"bytes"
	"context"
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger-firefly/signer/pkg/abi"
)

// DecodeDomainInstanceConfig decodes Paladin Zeto registration/config bytes.
// When the payload starts with ZetoDomainConfigID_V1, the remainder is ABI-decoded as DomainInstanceConfigV1ABI.
// Otherwise the full slice is decoded as legacy DomainInstanceConfigABI (tokenName + circuits only).
func DecodeDomainInstanceConfig(ctx context.Context, domainConfig []byte) (*DomainInstanceConfig, error) {
	var cfg DomainInstanceConfig
	if bytes.HasPrefix(domainConfig, ZetoDomainConfigID_V1) {
		if err := decodeABIConfig(ctx, DomainInstanceConfigV1ABI, domainConfig[len(ZetoDomainConfigID_V1):], &cfg); err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorAbiDecodeDomainInstanceConfig, err)
		}
	} else {
		if err := decodeABIConfig(ctx, DomainInstanceConfigABI, domainConfig, &cfg); err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgZetoLegacyDomainConfigDecode, ZetoDomainConfigID_V1.String(), err.Error())
		}
		// Legacy configs predate the variant, so they are V0 by definition.
		cfg.Variant = ZetoVariant_V0
	}
	cfg.stampCircuitVariant()
	return &cfg, nil
}

// stampCircuitVariant propagates the pool's variant onto its circuits, so the prover resolves .wasm and .zkey from that
// variant's artifact tree.
func (c *DomainInstanceConfig) stampCircuitVariant() {
	if c.Circuits != nil {
		c.Circuits.StampVariant(uint64(c.Variant))
	}
}

func decodeABIConfig(ctx context.Context, configABI *abi.ParameterArray, data []byte, cfg *DomainInstanceConfig) error {
	configValues, err := configABI.DecodeABIDataCtx(ctx, data, 0)
	if err != nil {
		return err
	}
	configJSON, err := pldtypes.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return err
	}
	return json.Unmarshal(configJSON, cfg)
}

// EncodeDomainInstanceConfig writes a V0 pool in the unprefixed encoding, so nodes that predate the variant can still
// read it; any other variant is written as ZetoDomainConfigID_V1 || abi.encode(DomainInstanceConfigV1ABI).
func EncodeDomainInstanceConfig(ctx context.Context, cfg *DomainInstanceConfig) (pldtypes.HexBytes, error) {
	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Variant == ZetoVariant_V0 {
		return DomainInstanceConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
	}
	encodedBody, err := DomainInstanceConfigV1ABI.EncodeABIDataJSONCtx(ctx, configJSON)
	if err != nil {
		return nil, err
	}
	return append(bytes.Clone(ZetoDomainConfigID_V1), encodedBody...), nil
}
