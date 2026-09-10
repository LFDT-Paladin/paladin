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

package noto

import (
	"context"
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type burnFromHandler struct {
	burnCommon
}

func (h *burnFromHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (any, NotoDomainError) {
	var burnFromParams types.BurnFromParams
	if err := json.Unmarshal([]byte(params), &burnFromParams); err != nil {
		return nil, validationErr(err)
	}
	if err := h.validateBurnParams(ctx, burnFromParams.Amount); err != nil {
		return &burnFromParams, err
	}
	if burnFromParams.From == "" {
		return &burnFromParams, validationErr(i18n.NewError(ctx, msgs.MsgParameterRequired, "from"))
	}
	return &burnFromParams, nil
}

func (h *burnFromHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.BurnFromParams)
	if tx.DomainConfig.NotaryMode == types.NotaryModeBasic.Enum() {
		return nil, i18n.NewError(ctx, msgs.MsgBurnFromNotAllowed)
	}
	return h.initBurn(ctx, tx, params.From)
}

func (h *burnFromHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, NotoDomainError) {
	params := tx.Params.(*types.BurnFromParams)
	return h.assembleBurn(ctx, tx, req, params.From, params.Amount, params.Data)
}

func (h *burnFromHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, NotoDomainError) {
	params := tx.Params.(*types.BurnFromParams)
	return h.endorseBurn(ctx, tx, req, params.From, params.Amount, params.Data)
}

func (h *burnFromHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	params := tx.Params.(*types.BurnFromParams)
	return h.prepareBurn(ctx, tx, req, params.From, params.Amount, params.Data)
}
