// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// legacyDomainConfigHeader is what the first four bytes of a legacy (headerless) config always are: the high-order zero
// bytes of the 32-byte ABI offset of tokenName. See the header layout documented at ZetoDomainConfigGen2.
var legacyDomainConfigHeader = make([]byte, ZetoDomainConfigHeaderLen)

// DecodeDomainInstanceConfig decodes the on-chain domain config of a Zeto pool.
//   - First four bytes zero: a legacy config, decoded whole as DomainInstanceConfigABI, at variant V0.
//   - Generation 2 header: the variant from bytes 2-3, then the remainder decoded as DomainInstanceConfigABI.
//   - Any other header: an unknown generation, rejected.
//
// The variant is then stamped onto every circuit, so the prover reads that variant's artifact tree.
func DecodeDomainInstanceConfig(ctx context.Context, domainConfig []byte) (*DomainInstanceConfig, error) {
	var cfg DomainInstanceConfig
	switch {
	case len(domainConfig) < ZetoDomainConfigHeaderLen || bytes.Equal(domainConfig[:ZetoDomainConfigHeaderLen], legacyDomainConfigHeader):
		// Legacy configs predate the variant, so they are V0 by definition. Input too short to hold a header cannot be a
		// valid ABI payload either, and is reported by the legacy decode.
		if err := decodeConfigPayload(ctx, domainConfig, &cfg); err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgZetoDomainConfigDecode, "legacy", err.Error())
		}
		cfg.Variant = ZetoVariant_V0
	default:
		header := domainConfig[:ZetoDomainConfigHeaderLen]
		generation := binary.BigEndian.Uint16(header[0:2])
		if generation != ZetoDomainConfigGen2 {
			return nil, i18n.NewError(ctx, msgs.MsgZetoUnknownDomainConfigGeneration, fmt.Sprintf("0x%04x", generation), pldtypes.HexBytes(header).String())
		}
		if err := decodeConfigPayload(ctx, domainConfig[ZetoDomainConfigHeaderLen:], &cfg); err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgZetoDomainConfigDecode, fmt.Sprintf("0x%04x", generation), err.Error())
		}
		cfg.Variant = ZetoVariant(binary.BigEndian.Uint16(header[2:4]))
	}
	cfg.stampCircuitVariant()
	return &cfg, nil
}

// stampCircuitVariant propagates the pool's variant onto its circuits, so the prover resolves .wasm and .zkey from that
// variant's artifact tree. The on-chain circuits tuple has no variant field.
func (c *DomainInstanceConfig) stampCircuitVariant() {
	if c.Circuits != nil {
		c.Circuits.StampVariant(uint64(c.Variant))
	}
}

func decodeConfigPayload(ctx context.Context, data []byte, cfg *DomainInstanceConfig) error {
	configValues, err := DomainInstanceConfigABI.DecodeABIDataCtx(ctx, data, 0)
	if err != nil {
		return err
	}
	configJSON, err := pldtypes.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return err
	}
	return json.Unmarshal(configJSON, cfg)
}

// EncodeDomainInstanceConfig writes every pool, V0 included, as a generation 2 config:
// 0x0002 || uint16 variant || abi.encode(DomainInstanceConfigABI). Nodes that predate config generations cannot read it,
// so every node in a network must be upgraded before new pools are deployed.
func EncodeDomainInstanceConfig(ctx context.Context, cfg *DomainInstanceConfig) (pldtypes.HexBytes, error) {
	if cfg.Variant > ZetoDomainConfigMaxVariant {
		return nil, i18n.NewError(ctx, msgs.MsgZetoVariantExceedsConfigHeader, cfg.Variant, ZetoDomainConfigMaxVariant)
	}
	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	payload, err := DomainInstanceConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
	if err != nil {
		return nil, err
	}
	encoded := make([]byte, ZetoDomainConfigHeaderLen, ZetoDomainConfigHeaderLen+len(payload))
	binary.BigEndian.PutUint16(encoded[0:2], ZetoDomainConfigGen2)
	binary.BigEndian.PutUint16(encoded[2:4], uint16(cfg.Variant))
	return append(encoded, payload...), nil
}
