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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/require"
)

// Allows individual tests to focus on the different state availability, without huge amounts of boilerplate.
type manifestTester struct {
	t             *testing.T
	ctx           context.Context
	n             *Noto
	mockCallbacks *domain.MockDomainCallbacks
	txID          string
	inputStates   []*prototk.EndorsableState
	outputStates  []*prototk.EndorsableState
	readStates    []*prototk.EndorsableState
	infoStates    []*prototk.EndorsableState
}

type manifestTesterAvailabilityScenario struct {
	*manifestTester
	unavailable []*prototk.EndorsableState
}

func newManifestTester(t *testing.T,
	ctx context.Context,
	n *Noto,
	mockCallbacks *domain.MockDomainCallbacks,
	txID string,
	assembled *prototk.AssembledTransaction,
) *manifestTester {
	return &manifestTester{
		t:             t,
		ctx:           ctx,
		n:             n,
		mockCallbacks: mockCallbacks,
		txID:          txID,
		inputStates:   stateRefToEndorsableState(assembled.InputStates),
		outputStates:  newStateToEndorsableState(assembled.OutputStates),
		readStates:    stateRefToEndorsableState(assembled.ReadStates),
		infoStates:    newStateToEndorsableState(assembled.InfoStates),
	}
}

func stateRefToEndorsableState(newStates []*prototk.StateRef) []*prototk.EndorsableState {
	endorsableStates := make([]*prototk.EndorsableState, len(newStates))
	for i, stateRef := range newStates {
		endorsableStates[i] = &prototk.EndorsableState{
			Id:            stateRef.Id,
			SchemaId:      stateRef.SchemaId,
			StateDataJson: `{}`, // we don't have a case where we need this data in manifest processing
		}
	}
	return endorsableStates
}

func newStateToEndorsableState(newStates []*prototk.NewState) []*prototk.EndorsableState {
	endorsableStates := make([]*prototk.EndorsableState, len(newStates))
	for i, newState := range newStates {
		endorsableStates[i] = &prototk.EndorsableState{
			Id:            *newState.Id, // must have been allocated via Paladin state validation
			SchemaId:      newState.SchemaId,
			StateDataJson: newState.StateDataJson,
		}
	}
	return endorsableStates
}

func (mt *manifestTester) separateUnavailable(states []*prototk.EndorsableState, unavailable ...*prototk.EndorsableState) ([]*prototk.EndorsableState, []string) {
	trimmed := make([]*prototk.EndorsableState, 0, len(states))
	unavailableIDs := make([]string, 0, len(unavailable))
skipState:
	for _, s := range states {
		for _, us := range unavailable {
			if s.Id == us.Id {
				unavailableIDs = append(unavailableIDs, us.Id)
				continue skipState
			}
		}
		trimmed = append(trimmed, s)
	}
	return trimmed, unavailableIDs
}

func (mt *manifestTester) runCheckStateCompletionWithIdentity(localAddress string, unavailable ...*prototk.EndorsableState) (*prototk.CheckStateCompletionResponse, error) {

	mt.mockCallbacks.MockReverseKeyLookup = func(ctx context.Context, req *prototk.ReverseKeyLookupRequest) (*prototk.ReverseKeyLookupResponse, error) {
		res := &prototk.ReverseKeyLookupResponse{
			Results: make([]*prototk.ReverseKeyLookupResult, len(req.Lookups)),
		}
		for i, l := range req.Lookups {
			if l.Verifier == localAddress {
				res.Results[i] = &prototk.ReverseKeyLookupResult{
					Verifier:      l.Verifier,
					Found:         true,
					KeyIdentifier: confutil.P("key1"),
				}
			} else {
				res.Results[i] = &prototk.ReverseKeyLookupResult{
					Verifier: l.Verifier,
					Found:    false,
				}
			}
		}
		return res, nil
	}

	req := &prototk.CheckStateCompletionRequest{TransactionId: mt.txID, UnavailableStates: &prototk.UnavailableStates{}}
	if len(unavailable) > 0 {
		req.UnavailableStates.FirstUnavailableId = &unavailable[0].Id
	}
	req.InputStates, req.UnavailableStates.InputStateIds = mt.separateUnavailable(mt.inputStates, unavailable...)
	req.OutputStates, req.UnavailableStates.OutputStateIds = mt.separateUnavailable(mt.outputStates, unavailable...)
	req.ReadStates, req.UnavailableStates.ReadStateIds = mt.separateUnavailable(mt.readStates, unavailable...)
	req.InfoStates, req.UnavailableStates.InfoStateIds = mt.separateUnavailable(mt.infoStates, unavailable...)
	return mt.n.CheckStateCompletion(mt.ctx, req)
}

func (mt *manifestTester) withMissingStates(unavailable ...*prototk.StateRef) *manifestTesterAvailabilityScenario {
	return &manifestTesterAvailabilityScenario{
		manifestTester: mt,
		unavailable:    stateRefToEndorsableState(unavailable),
	}
}

func (mt *manifestTester) withMissingNewStates(unavailable ...*prototk.NewState) *manifestTesterAvailabilityScenario {
	return &manifestTesterAvailabilityScenario{
		manifestTester: mt,
		unavailable:    newStateToEndorsableState(unavailable),
	}
}

func (mts *manifestTesterAvailabilityScenario) completeForIdentity(localAddress string) *manifestTesterAvailabilityScenario {
	res, err := mts.runCheckStateCompletionWithIdentity(localAddress, mts.unavailable...)
	require.NoError(mts.t, err)
	require.Nil(mts.t, res.NextMissingStateId)
	return mts
}

func (mts *manifestTesterAvailabilityScenario) incompleteForIdentity(localAddress string) *manifestTesterAvailabilityScenario {
	res, err := mts.runCheckStateCompletionWithIdentity(localAddress, mts.unavailable...)
	require.NoError(mts.t, err)
	require.NotNil(mts.t, res.NextMissingStateId)
	return mts
}

func TestBuildManifestFailValidateStates(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	mockCallbacks.MockValidateStates = func(ctx context.Context, req *prototk.ValidateStatesRequest) (*prototk.ValidateStatesResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := n.newManifestBuilder().
		addInfoStates(
			identityList{
				{
					identifier: "key1",
					address:    pldtypes.RandAddress(),
				},
			},
			&prototk.NewState{
				SchemaId:      pldtypes.RandBytes32().String(),
				StateDataJson: `{}`,
			},
		).
		buildManifest(ctx, "state-query-context")
	require.Regexp(t, "pop", err)
}
