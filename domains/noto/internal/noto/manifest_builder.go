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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type coinLogicalOwner int

const (
	FROM coinLogicalOwner = iota
	TO
)

type manifestBuilder struct {
	n                 *Noto
	tx                *types.ParsedTransaction
	stateQueryContext string
	resolvedVerifiers []*prototk.ResolvedVerifier
	notary            string
	notaryAddress     *pldtypes.EthAddress
	sender            string
	senderAddress     *pldtypes.EthAddress
	from              string
	fromAddress       *pldtypes.EthAddress
	to                string
	toAddress         *pldtypes.EthAddress
	manifest          types.NotoManifest
	inputs            preparedInputs
	outputs           preparedOutputs
	infoStates        []*prototk.NewState
}

func (n *Noto) newManifestBuilder(ctx context.Context, tx *types.ParsedTransaction, stateQueryContext string, resolvedVerifiers []*prototk.ResolvedVerifier, from, to string) (mb *manifestBuilder, err error) {
	mb = &manifestBuilder{
		n:                 n,
		tx:                tx,
		stateQueryContext: stateQueryContext,
		resolvedVerifiers: resolvedVerifiers,
		manifest: types.NotoManifest{
			Salt: pldtypes.RandBytes32(),
		},
	}
	mb.notaryAddress, err = n.findEthAddressVerifier(ctx, "notary", tx.DomainConfig.NotaryLookup, mb.resolvedVerifiers)
	if err == nil {
		mb.senderAddress, err = n.findEthAddressVerifier(ctx, "sender", from, mb.resolvedVerifiers)
	}
	if err == nil {
		if mb.from == "" {
			// If the from is not specified, then the sender is the from
			mb.from = mb.sender
			mb.fromAddress = mb.senderAddress
		} else {
			mb.fromAddress, err = n.findEthAddressVerifier(ctx, "from", from, mb.resolvedVerifiers)
		}
	}
	if err == nil && to != "" {
		mb.toAddress, err = n.findEthAddressVerifier(ctx, "to", to, mb.resolvedVerifiers)
	}
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (mb *manifestBuilder) prepareOutputCoin(recipient coinLogicalOwner, amount *pldtypes.HexUint256) error {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	var ownerAddress *pldtypes.EthAddress
	var paladinDistributionList []string
	switch recipient {
	case TO:
		ownerAddress = mb.toAddress
		paladinDistributionList = []string{mb.notary, mb.to, mb.sender}
	case FROM:
		fallthrough
	default:
		ownerAddress = mb.fromAddress
		paladinDistributionList = []string{mb.notary, mb.sender}
		if mb.from != mb.sender {
			paladinDistributionList = append(paladinDistributionList, mb.from)
		}
	}
	newCoin := &types.NotoCoin{
		Salt:   pldtypes.RandBytes32(),
		Owner:  ownerAddress,
		Amount: amount,
	}
	newState, err := mb.n.makeNewCoinState(newCoin, paladinDistributionList)
	if err == nil {
		mb.outputs.recipients = append(mb.outputs.recipients, recipient)
		mb.outputs.coins = append(mb.outputs.coins, newCoin)
		mb.outputs.states = append(mb.outputs.states, newState)
	}
	return err
}

func (mb *manifestBuilder) selectAndPrepareInputCoins(ctx context.Context, amount *pldtypes.HexUint256) (revert bool, err error) {
	var lastStateTimestamp int64
	stateRefs := []*prototk.StateRef{}
	coins := []*types.NotoCoin{}
	for {
		// TODO: make this configurable
		queryBuilder := query.NewQueryBuilder().
			Limit(10).
			Sort(".created").
			Equal("owner", mb.fromAddress.String())

		if lastStateTimestamp > 0 {
			queryBuilder.GreaterThan(".created", lastStateTimestamp)
		}

		log.L(ctx).Debugf("State query: %s", queryBuilder.Query())
		states, err := mb.n.findAvailableStates(ctx, mb.stateQueryContext, mb.n.coinSchema.Id, queryBuilder.Query().String())
		if err != nil {
			return false, err
		}
		if len(states) == 0 {
			return true, i18n.NewError(ctx, msgs.MsgInsufficientFunds, mb.inputs.total.Text(10))
		}
		for _, state := range states {
			lastStateTimestamp = state.CreatedAt
			coin, err := mb.n.unmarshalCoin(state.DataJson)
			if err != nil {
				return false, i18n.NewError(ctx, msgs.MsgInvalidStateData, state.Id, err)
			}
			mb.inputs.total = mb.inputs.total.Add(mb.inputs.total, coin.Amount.Int())
			inputState := &prototk.StateRef{
				SchemaId: state.SchemaId,
				Id:       state.Id,
			}
			stateRefs = append(stateRefs, inputState)
			coins = append(coins, coin)
			mb.inputs.coins = append(mb.inputs.coins, coin)
			mb.inputs.states = append(mb.inputs.states, inputState)
			log.L(ctx).Debugf("Selecting coin %s value=%s total=%s required=%s)", state.Id, coin.Amount.Int().Text(10), mb.inputs.total.Text(10), amount.Int().Text(10))
			if mb.inputs.total.Cmp(amount.Int()) >= 0 {
				return false, nil
			}
		}
	}
}

func (mb *manifestBuilder) addStateToManifest(ctx context.Context, stateIDStr string, availability []*pldtypes.EthAddress) error {
	stateID, err := pldtypes.ParseBytes32Ctx(ctx, stateIDStr)
	if err != nil {
		// Very much unexpected as we should have validated the state ID by this point
		log.L(ctx).Errorf("State '%s' could not be added to manifest due to invalid stateId")
		return err
	}
	mb.manifest.States = append(mb.manifest.States, &types.NotoManifestStateEntry{
		ID:           pldtypes.Bytes32(stateID),
		Participants: availability,
	})
	return nil
}

func (mb *manifestBuilder) prepareTransferInfoStates(ctx context.Context, tx *types.ParsedTransaction, data pldtypes.HexBytes) error {

	infoDistributionList := []string{mb.notary, mb.sender}
	if mb.to != "" {
		infoDistributionList = append(infoDistributionList, mb.to)
	}
	if mb.from != mb.sender {
		infoDistributionList = append(infoDistributionList, mb.from)
	}
	txData, err := mb.n.prepareTransactionDataInfo(data, tx.DomainConfig.Variant, infoDistributionList)
	if err != nil {
		return err
	}

	if !tx.DomainConfig.IsV0() {

		// We now need to validate (and calculate the hash) of all the output and info states - apart from the manifest.
		allOutputStates := make([]*prototk.NewState, 0, 1+len(mb.outputs.states))
		allOutputStates = append(allOutputStates, txData)
		allOutputStates = append(allOutputStates, mb.outputs.states...)
		validatedOutputStates, err := mb.n.Callbacks.ValidateStates(ctx, &prototk.ValidateStatesRequest{
			StateQueryContext: mb.stateQueryContext,
			States:            allOutputStates,
		})
		if err != nil {
			return err
		}
		for i, s := range allOutputStates {
			// We supply the hash back to Paladin on all our states, which allows Paladin to double
			// check when we return from assemble that the hashes are correct.
			preCalculatedHash := validatedOutputStates.States[i].Id
			s.Id = &preCalculatedHash
		}

		availableToAll := []*pldtypes.EthAddress{mb.notaryAddress, mb.senderAddress}
		if mb.to != "" {
			availableToAll = append(availableToAll, mb.toAddress)
		}
		availableToSender := []*pldtypes.EthAddress{mb.notaryAddress, mb.senderAddress}
		if mb.from != mb.sender {
			availableToAll = append(availableToAll, mb.fromAddress)
			availableToSender = append(availableToSender, mb.fromAddress)
		}
		err = mb.addStateToManifest(ctx, *txData.Id /* set above */, availableToAll)
		for _, state := range mb.inputs.states {
			if err == nil {
				err = mb.addStateToManifest(ctx, state.Id, availableToSender)
			}
		}
		for i, state := range mb.outputs.states {
			if err == nil {
				switch mb.outputs.recipients[i] {
				case TO:
					err = mb.addStateToManifest(ctx, *state.Id, availableToAll)
				case FROM:
					fallthrough
				default:
					err = mb.addStateToManifest(ctx, *state.Id, availableToSender)
				}
			}
		}
		if err != nil {
			return err
		}
		// Now we can seal the manifest
		manifestState, err := mb.n.makeNewManifestInfoState(&mb.manifest, infoDistributionList)
		if err != nil {
			return err
		}
		// ... and add it to the first entry in the info-states (ahead of the txData)
		mb.infoStates = append(mb.infoStates, manifestState)
	}
	mb.infoStates = append(mb.infoStates, txData)
	return nil

}
