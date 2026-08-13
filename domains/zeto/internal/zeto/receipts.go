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

	data, err := transferData(filterSchema(req.InfoStates, []string{z.dataSchema.Id}))
	if err != nil {
		return nil, err
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
	receipt.Transfers = append(buildFungibleTransfers(ctx, inputs, outputs, data), buildNonFungibleTransfers(ctx, inputs, outputs)...)

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

// transferData returns the data supplied on each transfer entry, in entry order.
//
// None of Zeto's methods take a top-level data parameter - data is supplied per transfer entry, and
// each entry gets its own info state. An info state's content is just a salt and the data, with
// nothing naming the entry it belongs to, so entries are identified by position: the handlers write
// the info states in entry order, and the ids are carried through the on-chain transaction data as
// an ordered list, so the order survives to here.
//
// A party only receives the info states for the entries it is party to, so a recipient sees just
// their own - which is also the only output coin they see, keeping the positions aligned.
func transferData(infoStates []*prototk.EndorsableState) ([]pldtypes.HexBytes, error) {
	data := make([]pldtypes.HexBytes, len(infoStates))
	for i, state := range infoStates {
		info, err := unmarshalInfo(state.StateDataJson)
		if err != nil {
			return nil, err
		}
		data[i] = info.Data
	}
	return data, nil
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

// buildFungibleTransfers reports the value each output coin moved.
//
// One transfer is reported per coin rather than per recipient. Data is supplied per transfer entry
// and each entry produces exactly one coin, so combining a recipient's coins into a single transfer
// would leave their entries sharing one transfer with no way to report their data separately.
//
// A Zeto transaction only ever spends coins belonging to a single owner, so anything else means we
// cannot describe it as a set of transfers and we report none.
//
// Coins that come back to the sender - change, or the locked half of a lock - are netted off rather
// than reported, so locking (which replaces unlocked coins with locked coins of the same total owned
// by the same party) correctly reports that nothing moved.
func buildFungibleTransfers(ctx context.Context, inputs, outputs *parsedCoins, entryData []pldtypes.HexBytes) []*types.ReceiptTransfer {
	var from pldtypes.HexBytes
	fromAmount := new(big.Int)

	for _, coin := range inputs.coins {
		if from == nil {
			from = coin.Owner
		} else if !coin.Owner.Equals(from) {
			log.L(ctx).Warnf("Unable to build transfers: transaction spends coins owned by more than one party")
			return nil
		}
		fromAmount.Add(fromAmount, coinAmount(coin))
	}

	transfers := make([]*types.ReceiptTransfer, 0, len(outputs.coins))
	for i, coin := range outputs.coins {
		amount := coinAmount(coin)
		fromAmount.Sub(fromAmount, amount)
		if coin.Owner.Equals(from) {
			// Returned to the sender - change, or the locked half of a lock
			continue
		}
		if amount.Sign() == 0 {
			// Handlers pad their outputs out to the width the circuit requires
			continue
		}
		transfers = append(transfers, &types.ReceiptTransfer{
			From:   from,
			To:     coin.Owner,
			Amount: (*pldtypes.HexUint256)(amount),
			Data:   dataForEntry(entryData, i),
		})
	}

	if len(transfers) == 0 && from != nil && fromAmount.Sign() > 0 {
		// Burn or withdraw - value left the token with no recipient
		return []*types.ReceiptTransfer{{
			From:   from,
			Amount: (*pldtypes.HexUint256)(fromAmount),
		}}
	}
	return transfers
}

// dataForEntry returns the data supplied on the transfer entry that produced the output coin at the
// given position.
//
// Handlers write one info state per entry and one output coin per entry, both in entry order, so the
// two line up by position. Coins beyond the entries - change, or the outputs of methods that take no
// data at all, such as deposit - have no entry and so no data.
func dataForEntry(entryData []pldtypes.HexBytes, i int) pldtypes.HexBytes {
	if i < len(entryData) {
		return entryData[i]
	}
	return nil
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
	// coins holds the fungible coins, locked and unlocked alike, in the order they appeared in the
	// request - which for outputs is the order of the transfer entries that produced them, so a coin's
	// position identifies its entry. Locked coins are told apart by their own Locked flag.
	coins []*types.ZetoCoin
	nfts  []*types.ZetoNFToken
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
			result.coins = append(result.coins, coin)
			if coin.Locked {
				result.lockedStates = append(result.lockedStates, rState)
			} else {
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
