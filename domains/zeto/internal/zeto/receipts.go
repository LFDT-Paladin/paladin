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
	log.L(ctx).Debugf("Building receipt for Zeto transaction %s", req.TransactionId)
	receipt := &types.ZetoDomainReceipt{}

	// The transaction data is carried on info states. Handlers write one per transfer entry, so a
	// multi-recipient transfer has several - we report the last, as Noto does for prepareUnlock.
	infoStates := filterSchema(req.InfoStates, []string{z.dataSchema.Id})
	if len(infoStates) > 0 {
		info, err := unmarshalInfo(infoStates[len(infoStates)-1].StateDataJson)
		if err != nil {
			return nil, err
		}
		receipt.Data = info.Data
	}

	// Nullifier tokens record their sparse merkle tree updates against the transaction as well, so
	// narrow the lists down to the coin and token states before interpreting them.
	coinSchemas := []string{z.coinSchema.Id, z.nftSchema.Id}
	inputs, err := z.parseCoinList(ctx, "inputs", filterSchema(req.InputStates, coinSchemas))
	if err != nil {
		return nil, err
	}
	outputs, err := z.parseCoinList(ctx, "outputs", filterSchema(req.OutputStates, coinSchemas))
	if err != nil {
		return nil, err
	}

	receipt.States.Inputs = inputs.states
	receipt.States.LockedInputs = inputs.lockedStates
	receipt.States.Outputs = outputs.states
	receipt.States.LockedOutputs = outputs.lockedStates
	receipt.Transfers = append(buildFungibleTransfers(ctx, inputs, outputs), buildNonFungibleTransfers(ctx, inputs, outputs)...)

	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}

	log.L(ctx).Debugf("Built receipt for Zeto transaction %s", req.TransactionId)
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

func receiptState(ctx context.Context, state *prototk.EndorsableState) (*types.ReceiptState, error) {
	id, err := pldtypes.ParseHexBytes(ctx, state.Id)
	if err != nil {
		return nil, err
	}
	schemaID, err := pldtypes.ParseBytes32Ctx(ctx, state.SchemaId)
	if err != nil {
		return nil, err
	}
	return &types.ReceiptState{
		ID:     id,
		Schema: schemaID,
		Data:   pldtypes.RawJSON(state.StateDataJson),
	}, nil
}

// buildFungibleTransfers reduces the coins a transaction spent and created down to the value that
// moved between owners. A Zeto transaction only ever spends coins belonging to a single owner, so
// anything else means we cannot describe it as a set of transfers and we report none.
//
// Locked coins take part in the arithmetic alongside unlocked ones, so that locking value (which
// replaces unlocked coins with locked coins of the same total, owned by the same party) correctly
// reports no transfer at all.
func buildFungibleTransfers(ctx context.Context, inputs, outputs *parsedCoins) []*types.ReceiptTransfer {
	var from pldtypes.HexBytes
	fromAmount := new(big.Int)

	// Recipients are keyed by the hex of their public key, as HexBytes is a slice and so cannot be
	// a map key itself. The insertion order is tracked so the receipt is deterministic.
	toAmounts := make(map[string]*big.Int)
	toOwners := make(map[string]pldtypes.HexBytes)
	var recipients []string

	for _, coin := range slices.Concat(inputs.coins, inputs.lockedCoins) {
		if from == nil {
			from = coin.Owner
		} else if !coin.Owner.Equals(from) {
			log.L(ctx).Warnf("Unable to build transfers: transaction spends coins owned by more than one party")
			return nil
		}
		fromAmount.Add(fromAmount, coinAmount(coin))
	}

	for _, coin := range slices.Concat(outputs.coins, outputs.lockedCoins) {
		amount := coinAmount(coin)
		if coin.Owner.Equals(from) {
			// Value returned to the sender - change, or the locked half of a lock
			fromAmount.Sub(fromAmount, amount)
			continue
		}
		key := coin.Owner.String()
		if existing, ok := toAmounts[key]; ok {
			existing.Add(existing, amount)
			continue
		}
		toAmounts[key] = new(big.Int).Set(amount)
		toOwners[key] = coin.Owner
		recipients = append(recipients, key)
	}

	if len(recipients) == 0 {
		if from != nil && fromAmount.Sign() > 0 {
			// Burn or withdraw - value left the token with no recipient
			return []*types.ReceiptTransfer{{
				From:   from,
				Amount: (*pldtypes.HexUint256)(fromAmount),
			}}
		}
		return nil
	}

	transfers := make([]*types.ReceiptTransfer, 0, len(recipients))
	for _, key := range recipients {
		amount := toAmounts[key]
		if amount.Sign() > 0 {
			transfers = append(transfers, &types.ReceiptTransfer{
				From:   from,
				To:     toOwners[key],
				Amount: (*pldtypes.HexUint256)(amount),
			})
		}
	}
	return transfers
}

// buildNonFungibleTransfers matches each token the transaction created back to the token of the same
// ID that it spent. A token with no matching input was minted, an input with no matching output was
// burned, and a token whose owner did not change did not move and so is not reported.
func buildNonFungibleTransfers(ctx context.Context, inputs, outputs *parsedCoins) []*types.ReceiptTransfer {
	var from pldtypes.HexBytes
	for _, nft := range inputs.nfts {
		if from == nil {
			from = nft.Owner
		} else if !nft.Owner.Equals(from) {
			log.L(ctx).Warnf("Unable to build transfers: transaction spends tokens owned by more than one party")
			return nil
		}
	}

	newOwners := make(map[string]pldtypes.HexBytes, len(outputs.nfts))
	for _, nft := range outputs.nfts {
		newOwners[tokenKey(nft)] = nft.Owner
	}
	spent := make(map[string]bool, len(inputs.nfts))
	for _, nft := range inputs.nfts {
		spent[tokenKey(nft)] = true
	}

	transfers := make([]*types.ReceiptTransfer, 0, len(inputs.nfts)+len(outputs.nfts))
	for _, nft := range inputs.nfts {
		to := newOwners[tokenKey(nft)]
		if to.Equals(nft.Owner) {
			continue
		}
		transfers = append(transfers, &types.ReceiptTransfer{
			From:    nft.Owner,
			To:      to,
			TokenID: nft.TokenID,
		})
	}
	for _, nft := range outputs.nfts {
		if !spent[tokenKey(nft)] {
			transfers = append(transfers, &types.ReceiptTransfer{
				To:      nft.Owner,
				TokenID: nft.TokenID,
			})
		}
	}
	return transfers
}

// The receipt is built from state data read back from storage, so tolerate a coin arriving without
// the fields the domain always writes rather than failing (or panicking) on the whole receipt.
func coinAmount(coin *types.ZetoCoin) *big.Int {
	if coin.Amount == nil {
		return new(big.Int)
	}
	return coin.Amount.Int()
}

func tokenKey(nft *types.ZetoNFToken) string {
	if nft.TokenID == nil {
		return ""
	}
	return nft.TokenID.String()
}

type parsedCoins struct {
	coins       []*types.ZetoCoin
	lockedCoins []*types.ZetoCoin
	nfts        []*types.ZetoNFToken
	// states holds the unlocked coins and the non-fungible tokens (which have no locked form),
	// lockedStates the locked coins - in the order they appeared in the request.
	states       []*types.ReceiptState
	lockedStates []*types.ReceiptState
}

func (z *Zeto) parseCoinList(ctx context.Context, label string, states []*prototk.EndorsableState) (*parsedCoins, error) {
	statesUsed := make(map[string]bool)
	result := &parsedCoins{}
	for i, state := range states {
		if statesUsed[state.Id] {
			return nil, i18n.NewError(ctx, msgs.MsgDuplicateStateInList, label, i, state.Id)
		}
		statesUsed[state.Id] = true

		rState, err := receiptState(ctx, state)
		if err != nil {
			return nil, err
		}

		switch state.SchemaId {
		case z.coinSchema.Id:
			coin, err := unmarshalCoin(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgInvalidListInput, label, i, state.Id, err)
			}
			if coin.Locked {
				result.lockedCoins = append(result.lockedCoins, coin)
				result.lockedStates = append(result.lockedStates, rState)
			} else {
				result.coins = append(result.coins, coin)
				result.states = append(result.states, rState)
			}

		case z.nftSchema.Id:
			nft, err := unmarshalNFT(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgInvalidListInput, label, i, state.Id, err)
			}
			result.nfts = append(result.nfts, nft)
			result.states = append(result.states, rState)

		default:
			return nil, i18n.NewError(ctx, msgs.MsgUnexpectedSchema, state.SchemaId)
		}
	}
	return result, nil
}
