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

type manifestBuilder struct {
	n                  *Noto
	tx                 *types.ParsedTransaction
	stateQueryContext  string
	resolvedVerifiers  []*prototk.ResolvedVerifier
	notary             identityPair
	sender             identityPair
	manifest           types.NotoManifest
	inputs             preparedInputs
	outputs            preparedOutputs
	lockedOutputs      preparedLockedOutputs
	commonDistribution identityList
	infoDistribution   identityList
	infoStates         []*prototk.NewState
}

func (n *Noto) newManifestBuilder(ctx context.Context, tx *types.ParsedTransaction, stateQueryContext string, resolvedVerifiers []*prototk.ResolvedVerifier, from string, recipients ...string) (mb *manifestBuilder, err error) {
	mb = &manifestBuilder{
		n:                 n,
		tx:                tx,
		stateQueryContext: stateQueryContext,
		resolvedVerifiers: resolvedVerifiers,
		manifest: types.NotoManifest{
			Salt: pldtypes.RandBytes32(),
		},
	}
	err = mb.initCommonParticipant(ctx, "notary", tx.DomainConfig.NotaryLookup)
	if err == nil {
		err = mb.initCommonParticipant(ctx, "sender", tx.Transaction.From)
	}
	if err != nil {
		return nil, err
	}
	return mb, nil
}

// Called during construction with the participants (notary/sender) that receive every state
func (mb *manifestBuilder) initCommonParticipant(ctx context.Context, description string, identifier string) (err error) {
	id := &identityPair{identifier: identifier}
	id.address, err = mb.n.findEthAddressVerifier(ctx, description, identifier, mb.resolvedVerifiers)
	if err == nil {
		mb.commonDistribution = append(mb.commonDistribution, id)
		mb.infoDistribution = append(mb.infoDistribution, id)
	}
	return err
}

// Called by the code using the build to set up a participant to use to build states.
func (mb *manifestBuilder) addParticipant(ctx context.Context, description string, identifier string) (participant *stateOwner, err error) {
	participant = &stateOwner{identityPair: identityPair{identifier: identifier}}
	participant.address, err = mb.n.findEthAddressVerifier(ctx, description, identifier, mb.resolvedVerifiers)
	if err == nil {
		duplicateCommon := false
		for _, existing := range mb.commonDistribution {
			if existing.identifier == identifier {
				// For example if the sender and from/to are the same
				duplicateCommon = true
				break
			}
		}
		if !duplicateCommon {
			mb.infoDistribution = append(mb.infoDistribution, &participant.identityPair)
			participant.distribution = append([]*identityPair{&participant.identityPair}, mb.commonDistribution...)
		}
	}
	return participant, err
}

func (mb *manifestBuilder) prepareOutputCoin(recipient *stateOwner, amount *pldtypes.HexUint256) error {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	newCoin := &types.NotoCoin{
		Salt:   pldtypes.RandBytes32(),
		Owner:  recipient.address,
		Amount: amount,
	}
	newState, err := mb.n.makeNewCoinState(newCoin, recipient.distribution.identities())
	if err == nil {
		mb.outputs.recipients = append(mb.outputs.recipients, recipient)
		mb.outputs.coins = append(mb.outputs.coins, newCoin)
		mb.outputs.states = append(mb.outputs.states, newState)
	}
	return err
}

func (mb *manifestBuilder) prepareLockedOutputCoin(id pldtypes.Bytes32, recipient *stateOwner, amount *pldtypes.HexUint256) error {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	newCoin := &types.NotoLockedCoin{
		Salt:   pldtypes.RandBytes32(),
		LockID: id,
		Owner:  recipient.address,
		Amount: amount,
	}
	newState, err := mb.n.makeNewLockedCoinState(newCoin, recipient.distribution.identities())
	if err == nil {
		mb.lockedOutputs.recipients = append(mb.lockedOutputs.recipients, recipient)
		mb.lockedOutputs.coins = append(mb.lockedOutputs.coins, newCoin)
		mb.lockedOutputs.states = append(mb.lockedOutputs.states, newState)
	}
	return err
}

func (mb *manifestBuilder) selectAndPrepareInputCoins(ctx context.Context, owner *pldtypes.EthAddress, amount *pldtypes.HexUint256) (revert bool, err error) {
	var lastStateTimestamp int64
	stateRefs := []*prototk.StateRef{}
	coins := []*types.NotoCoin{}
	for {
		// TODO: make this configurable
		queryBuilder := query.NewQueryBuilder().
			Limit(10).
			Sort(".created").
			Equal("owner", owner.String())

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

	txData, err := mb.n.prepareTransactionDataInfo(data, tx.DomainConfig.Variant, mb.infoDistribution.identities())
	if err != nil {
		return err
	}

	if !tx.DomainConfig.IsV0() {

		// We now need to validate (and calculate the hash) of all the output and info states - apart from the manifest.
		allOutputStates := make([]*prototk.NewState, 0, 1+len(mb.outputs.states))
		allOutputStates = append(allOutputStates, txData)
		allOutputStates = append(allOutputStates, mb.outputs.states...)
		allOutputStates = append(allOutputStates, mb.lockedOutputs.states...)
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

		err = mb.addStateToManifest(ctx, *txData.Id /* set above */, mb.infoDistribution.addresses())
		for _, state := range mb.inputs.states {
			if err == nil {
				// We only expect the notary and the originator to require the input states
				err = mb.addStateToManifest(ctx, state.Id, mb.commonDistribution.addresses())
			}
		}
		for i, state := range mb.outputs.states {
			if err == nil {
				err = mb.addStateToManifest(ctx, *state.Id, mb.outputs.recipients[i].distribution.addresses())
			}
		}
		for i, state := range mb.lockedOutputs.states {
			if err == nil {
				err = mb.addStateToManifest(ctx, *state.Id, mb.lockedOutputs.recipients[i].distribution.addresses())
			}
		}
		if err != nil {
			return err
		}
		// Now we can seal the manifest
		manifestState, err := mb.n.makeNewManifestInfoState(&mb.manifest, mb.infoDistribution.identities())
		if err != nil {
			return err
		}
		// ... and add it to the first entry in the info-states (ahead of the txData)
		mb.infoStates = append(mb.infoStates, manifestState)
	}
	mb.infoStates = append(mb.infoStates, txData)
	return nil

}
