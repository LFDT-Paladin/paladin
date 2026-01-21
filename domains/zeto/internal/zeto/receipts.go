/*
 * Copyright © 2026 Kaleido, Inc.
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

package zeto

import (
	"context"
	"encoding/json"
	"math/big"
	"slices"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

func (z *Zeto) buildReceipt(ctx context.Context, req *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error) {
	log.L(ctx).Debugf("Building receipt for Zeto domain from request: %s", req.TransactionId)
	receipt := &types.ZetoDomainReceipt{}

	// filter out states for the SMT if any
	infoStates := filterSchema(req.InfoStates, []string{z.dataSchema.Id})
	if len(infoStates) == 1 {
		info, err := unmarshalInfo(infoStates[0].StateDataJson)
		if err != nil {
			return nil, err
		}
		receipt.Data = info.Data
	}

	var err error
	receipt.States.Inputs, err = receiptStates(ctx, filterSchema(req.InputStates, []string{z.coinSchema.Id, z.nftSchema.Id}))
	if err == nil {
		receipt.States.Outputs, err = receiptStates(ctx, filterSchema(req.OutputStates, []string{z.coinSchema.Id, z.nftSchema.Id}))
		if err != nil {
			return nil, err
		}
	}

	receipt.Transfers, err = z.receiptTransfers(ctx, req)
	if err != nil {
		return nil, err
	}

	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}

	log.L(ctx).Debugf("Built receipt for Zeto domain from request: %s", req.TransactionId)
	return &prototk.BuildReceiptResponse{
		ReceiptJson: string(receiptJSON),
	}, nil
}

func filterSchema(states []*prototk.EndorsableState, schemas []string) (filtered []*prototk.EndorsableState) {
	for _, state := range states {
		if slices.Contains(schemas, state.SchemaId) {
			filtered = append(filtered, state)
		}
	}
	return filtered
}

func unmarshalInfo(stateData string) (*types.TransactionData, error) {
	var info types.TransactionData
	err := json.Unmarshal([]byte(stateData), &info)
	return &info, err
}

func unmarshalCoin(stateData string) (*types.ZetoCoin, error) {
	var coin types.ZetoCoin
	err := json.Unmarshal([]byte(stateData), &coin)
	return &coin, err
}

func unmarshalNFT(stateData string) (*types.ZetoNFToken, error) {
	var nft types.ZetoNFToken
	err := json.Unmarshal([]byte(stateData), &nft)
	return &nft, err
}

func receiptStates(ctx context.Context, states []*prototk.EndorsableState) ([]*types.ReceiptState, error) {
	coins := make([]*types.ReceiptState, len(states))
	for i, state := range states {
		id, err := pldtypes.ParseHexBytes(ctx, state.Id)
		if err != nil {
			return nil, err
		}
		schemaID, err := pldtypes.ParseBytes32Ctx(ctx, state.SchemaId)
		if err != nil {
			return nil, err
		}
		coins[i] = &types.ReceiptState{
			ID:     id,
			Schema: schemaID,
			Data:   pldtypes.RawJSON(state.StateDataJson),
		}
	}
	return coins, nil
}

func (z *Zeto) receiptTransfers(ctx context.Context, req *prototk.BuildReceiptRequest) ([]*types.ReceiptTransfer, error) {
	inputCoins, err := z.parseCoinList(ctx, "inputs", filterSchema(req.InputStates, []string{z.coinSchema.Id, z.nftSchema.Id}))
	if err != nil {
		return nil, err
	}
	outputCoins, err := z.parseCoinList(ctx, "outputs", filterSchema(req.OutputStates, []string{z.coinSchema.Id, z.nftSchema.Id}))
	if err != nil {
		return nil, err
	}

	transfers, err := buildFungibleTransfers(inputCoins, outputCoins)
	if err != nil {
		return nil, err
	}
	nftTransfers, err := buildNonFungibleTransfers(inputCoins, outputCoins)
	if err != nil {
		return nil, err
	}
	transfers = append(transfers, nftTransfers...)
	return transfers, nil
}

func buildFungibleTransfers(inputCoins *parsedCoins, outputCoins *parsedCoins) ([]*types.ReceiptTransfer, error) {
	var from pldtypes.HexBytes
	fromAmount := big.NewInt(0)
	to := make(map[*pldtypes.HexBytes]*big.Int)

	parseInput := func(owner pldtypes.HexBytes, amount *big.Int) bool {
		if from == nil {
			from = owner
		} else if !owner.Equals(from) {
			return false
		}
		fromAmount.Add(fromAmount, amount)
		return true
	}

	parseOutput := func(owner pldtypes.HexBytes, amount *big.Int) bool {
		if owner.Equals(from) {
			fromAmount.Sub(fromAmount, amount)
		} else if toAmount, ok := to[&owner]; ok {
			toAmount.Add(toAmount, amount)
		} else {
			to[&owner] = amount
		}
		return true
	}

	for _, coin := range inputCoins.coins {
		if !parseInput(coin.Owner, coin.Amount.Int()) {
			return nil, nil
		}
	}
	for _, coin := range outputCoins.coins {
		if !parseOutput(coin.Owner, coin.Amount.Int()) {
			return nil, nil
		}
	}

	if len(to) == 0 && from != nil && fromAmount.BitLen() > 0 {
		// special case for burn (no recipients)
		return []*types.ReceiptTransfer{{
			From:   from,
			Amount: (*pldtypes.HexUint256)(fromAmount),
		}}, nil
	}

	transfers := make([]*types.ReceiptTransfer, 0, len(to))
	for owner, amount := range to {
		if amount.BitLen() > 0 {
			transfers = append(transfers, &types.ReceiptTransfer{
				From:   from,
				To:     *owner,
				Amount: (*pldtypes.HexUint256)(amount),
			})
		}
	}
	return transfers, nil
}

func buildNonFungibleTransfers(inputCoins *parsedCoins, outputCoins *parsedCoins) ([]*types.ReceiptTransfer, error) {
	var transfers []*types.ReceiptTransfer
	var owner pldtypes.HexBytes
	for _, nft := range inputCoins.nfts {
		if owner == nil {
			owner = nft.Owner
		} else if !nft.Owner.Equals(owner) {
			return nil, nil
		}
		transfers = append(transfers, &types.ReceiptTransfer{
			From:    nft.Owner,
			TokenId: nft.TokenID,
		})
	}
	for _, nft := range outputCoins.nfts {
		if owner.Equals(nft.Owner) {
			// skip self-transfers
			continue
		}
		// find the matching input NFT
		for _, transfer := range transfers {
			if transfer.TokenId.String() == nft.TokenID.String() {
				transfer.To = nft.Owner
				break
			}
		}
	}
	return transfers, nil
}

type parsedCoins struct {
	coins  []*types.ZetoCoin
	nfts   []*types.ZetoNFToken
	states []*prototk.StateRef
	total  *big.Int
}

func (z *Zeto) parseCoinList(ctx context.Context, label string, states []*prototk.EndorsableState) (*parsedCoins, error) {
	statesUsed := make(map[string]bool)
	result := &parsedCoins{
		total: new(big.Int),
	}
	for i, state := range states {
		if statesUsed[state.Id] {
			return nil, i18n.NewError(ctx, msgs.MsgDuplicateStateInList, label, i, state.Id)
		}
		statesUsed[state.Id] = true

		switch state.SchemaId {
		case z.coinSchema.Id:
			coin, err := unmarshalCoin(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgInvalidListInput, label, i, state.Id, err)
			}
			result.coins = append(result.coins, coin)
			result.total = result.total.Add(result.total, coin.Amount.Int())
			result.states = append(result.states, &prototk.StateRef{
				SchemaId: state.SchemaId,
				Id:       state.Id,
			})

		case z.nftSchema.Id:
			nft, err := unmarshalNFT(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgInvalidListInput, label, i, state.Id, err)
			}
			result.nfts = append(result.nfts, nft)
			result.states = append(result.states, &prototk.StateRef{
				SchemaId: state.SchemaId,
				Id:       state.Id,
			})

		default:
			return nil, i18n.NewError(ctx, msgs.MsgUnexpectedSchema, state.SchemaId)
		}
	}
	return result, nil
}
