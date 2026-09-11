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
	"bytes"
	"context"
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

type domainInstanceConfigV1Wire struct {
	TokenName       string                  `json:"tokenName"`
	ZetoVariant     pldtypes.HexUint64      `json:"zetoVariant"`
	FactoryVersion  pldtypes.HexUint64      `json:"factoryVersion"`
	CircuitBundleId string                  `json:"circuitBundleId"`
	Circuits        *zetosignerapi.Circuits `json:"circuits"`
}

// DecodeDomainInstanceConfig decodes Paladin Zeto registration/config bytes.
// When the payload starts with ZetoDomainConfigID_V1, the remainder is ABI-decoded as DomainInstanceConfigV1ABI.
// Otherwise the full slice is decoded as legacy DomainInstanceConfigABI (tokenName + circuits only).
func DecodeDomainInstanceConfig(ctx context.Context, domainConfig []byte) (*DomainInstanceConfig, error) {
	if len(domainConfig) >= len(ZetoDomainConfigID_V1) {
		prefix := domainConfig[0:len(ZetoDomainConfigID_V1)]
		if bytes.Equal(prefix, ZetoDomainConfigID_V1) {
			return decodeDomainInstanceConfigV1(ctx, domainConfig[len(ZetoDomainConfigID_V1):])
		}
	}
	return decodeDomainInstanceConfigV0(ctx, domainConfig)
}

func decodeDomainInstanceConfigV0(ctx context.Context, domainConfig []byte) (*DomainInstanceConfig, error) {
	configValues, err := DomainInstanceConfigABI.DecodeABIDataCtx(ctx, domainConfig, 0)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgZetoLegacyDomainConfigDecode, ZetoDomainConfigID_V1.String(), err.Error())
	}
	configJSON, err := pldtypes.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var cfg DomainInstanceConfig
	if err = json.Unmarshal(configJSON, &cfg); err != nil {
		return nil, err
	}
	cfg.ConfigSchema = DomainConfigSchemaV0
	return &cfg, nil
}

func decodeDomainInstanceConfigV1(ctx context.Context, payload []byte) (*DomainInstanceConfig, error) {
	configValues, err := DomainInstanceConfigV1ABI.DecodeABIDataCtx(ctx, payload, 0)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorAbiDecodeDomainInstanceConfig, err)
	}
	configJSON, err := pldtypes.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var wire domainInstanceConfigV1Wire
	if err = json.Unmarshal(configJSON, &wire); err != nil {
		return nil, err
	}
	cfg := &DomainInstanceConfig{
		ConfigSchema:    DomainConfigSchemaV1,
		TokenName:       wire.TokenName,
		Circuits:        wire.Circuits,
		ZetoVariant:     wire.ZetoVariant,
		FactoryVersion:  int64(wire.FactoryVersion),
		CircuitBundleId: wire.CircuitBundleId,
	}
	return cfg, nil
}

// EncodeDomainInstanceConfigV0 encodes legacy registration bytes (no prefix).
func EncodeDomainInstanceConfigV0(ctx context.Context, cfg *DomainInstanceConfig) (pldtypes.HexBytes, error) {
	partial := struct {
		TokenName string                  `json:"tokenName"`
		Circuits  *zetosignerapi.Circuits `json:"circuits"`
	}{
		TokenName: cfg.TokenName,
		Circuits:  cfg.Circuits,
	}
	configJSON, err := json.Marshal(partial)
	if err != nil {
		return nil, err
	}
	return DomainInstanceConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
}

// EncodeDomainInstanceConfigV1 encodes ZetoDomainConfigID_V1 || abi.encode(DomainInstanceConfigV1ABI).
func EncodeDomainInstanceConfigV1(ctx context.Context, cfg *DomainInstanceConfig) (pldtypes.HexBytes, error) {
	wire := domainInstanceConfigV1Wire{
		TokenName:       cfg.TokenName,
		ZetoVariant:     cfg.ZetoVariant,
		FactoryVersion:  pldtypes.HexUint64(cfg.FactoryVersion),
		CircuitBundleId: cfg.CircuitBundleId,
		Circuits:        cfg.Circuits,
	}
	configJSON, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	encodedBody, err := DomainInstanceConfigV1ABI.EncodeABIDataJSONCtx(ctx, configJSON)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(ZetoDomainConfigID_V1)+len(encodedBody))
	out = append(out, ZetoDomainConfigID_V1...)
	out = append(out, encodedBody...)
	return out, nil
}
