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

package statemgr

import (
	"context"

	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// inMemoryState is a candidate held in the form ready for query evaluation: the full proto state plus
// its prebuilt label values, parsed schema ID and created ordering time.
type inMemoryState struct {
	state       *prototk.EndorsableState
	schemaID    pldtypes.Bytes32
	labelValues filters.PassthroughValueSet
	created     int64
}

// Implementing ValueSet lets inMemoryStates be ordered by filters.SortValueSetInPlace
func (s *inMemoryState) ValueSet() filters.ValueSet {
	return s.labelValues
}

// FindMatchingInMemoryStates evaluates a domain state query against caller-supplied snapshot
// candidates: each is projected into its query-evaluation form (parsed schema ID + label values),
// then filtered by schema, matched, sorted and limited. The projected label values are the sole
// match/sort input, so beyond the (cached) schema resolution no DB read occurs. A candidate whose
// IDs cannot be parsed is skipped rather than failing the whole query. Results are returned as
// *prototk.QueriedState — the state plus its coordinator created ordering time, all any caller needs.
func (ss *stateManager) FindMatchingInMemoryStates(ctx context.Context, domainName string, schemaID pldtypes.Bytes32, jq *query.QueryJSON, candidates []*prototk.SnapshotState) ([]*prototk.QueriedState, error) {
	schema, err := ss.getSchemaByID(ctx, ss.p.NOTX(), domainName, schemaID, true)
	if err != nil {
		return nil, err
	}
	labelSet := ss.labelSetFor(schema)

	// TODO: candidates are a frozen point-in-time session snapshot, but this re-projects every one
	// into an inMemoryState (parsing both IDs and rebuilding the label value map) on every query. A
	// single assembly can issue multiple queries against the same session, so the projection could be
	// computed once when the session opens and reused.
	matches := make([]*inMemoryState, 0, len(candidates))
	for _, snapshot := range candidates {
		candidate, err := newInMemoryState(ctx, snapshot.GetState(), snapshot.GetLabels(), snapshot.GetCreated())
		if err != nil {
			continue
		}
		if !candidate.schemaID.Equals(&schemaID) {
			continue
		}
		match, err := filters.EvalQuery(ctx, jq, labelSet, candidate.labelValues)
		if err != nil {
			return nil, err
		}
		if match {
			matches = append(matches, candidate)
		}
	}

	// Default to created timestamp order, matching the DB implementation, so the receiver
	// merges consistently ordered inputs.
	sortInstructions := jq.Sort
	if len(sortInstructions) == 0 {
		sortInstructions = []string{".created"}
	}
	if err := filters.SortValueSetInPlace(ctx, labelSet, matches, sortInstructions...); err != nil {
		return nil, err
	}

	if jq.Limit != nil && len(matches) > *jq.Limit {
		matches = matches[:*jq.Limit]
	}

	out := make([]*prototk.QueriedState, len(matches))
	for i, m := range matches {
		out[i] = &prototk.QueriedState{State: m.state, Created: m.created}
	}
	return out, nil
}

// newInMemoryState projects a snapshot into its query-evaluation form: it parses the state and schema
// IDs, converts the proto labels into a value set, and applies the base labels. It returns an error
// if either ID cannot be parsed, so the caller can skip an internally-inconsistent state rather than
// advertise it to queries.
func newInMemoryState(ctx context.Context, state *prototk.EndorsableState, protoLabels *prototk.StateLabels, created int64) (*inMemoryState, error) {
	id, err := pldtypes.ParseHexBytes(ctx, state.GetId())
	if err != nil {
		return nil, err
	}
	schemaID, err := pldtypes.ParseBytes32(state.GetSchemaId())
	if err != nil {
		return nil, err
	}
	labels := protoLabels.GetLabels()
	int64Labels := protoLabels.GetInt64Labels()
	labelValues := make(filters.PassthroughValueSet, len(labels)+len(int64Labels)+2)
	for _, l := range labels {
		labelValues[l.GetLabel()] = l.GetValue()
	}
	for _, l := range int64Labels {
		labelValues[l.GetLabel()] = l.GetValue()
	}
	return &inMemoryState{
		state:       state,
		schemaID:    schemaID,
		labelValues: addStateBaseLabels(labelValues, id, pldtypes.Timestamp(created)),
		created:     created,
	}, nil
}
