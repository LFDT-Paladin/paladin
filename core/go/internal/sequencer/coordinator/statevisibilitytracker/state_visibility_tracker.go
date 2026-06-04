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

// Package statevisibilitytracker is the single control point for private state visibility in the sequencer.
//
// It tracks which nodes are permitted to hold each private state's data, derived from the assembly
// response DistributionList. The default posture is deny: a state whose AllowedNodes is nil or empty
// is invisible to every node. This is the correct fail-safe for unknown distributions — no state data
// is leaked by default.
//
// The store is internally thread-safe; callers do not need external synchronisation.
package statevisibilitytracker

import (
	"context"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// OutputState wraps a StateUpsert with AllowedNodes for coordinator handover and assembly requests.
// AllowedNodes is the set of nodes permitted to hold this state's private data, derived from the
// assembly response DistributionList. A nil or empty AllowedNodes means the distribution is unknown
// and the state is excluded from all exports — this is the default-deny posture.
type OutputState struct {
	components.StateUpsert
	AllowedNodes []string `json:"allowedNodes,omitempty"`
}

// StateVisibilityStore is the single interface for all private state visibility operations.
// All methods are thread-safe.
type StateVisibilityStore interface {
	// RecordAssemblyOutput is the only path by which newly minted state visibility is written.
	// It derives AllowedNodes for each output state from the DistributionList in the assembly
	// response and stores the result.
	RecordAssemblyOutput(ctx context.Context, states []*components.FullState, potentials []*prototk.NewState)

	// GetForNode returns all states that node is explicitly listed in AllowedNodes for.
	// States with nil or empty AllowedNodes are always excluded (default-deny).
	GetForNode(node string) []*OutputState

	// ImportIfAbsent records state only if no entry already exists for stateID.
	// Existing entries always take precedence — a coordinator's own knowledge must never be
	// overwritten by a handover import. Returns true if the state was stored.
	ImportIfAbsent(stateID string, state *OutputState) bool

	// Delete removes stateID. No-op if absent.
	Delete(stateID string)
}

type store struct {
	mu         sync.RWMutex
	statesByID map[string]*OutputState
}

// NewStore returns a new, empty StateVisibilityStore.
func NewStore() StateVisibilityStore {
	return &store{
		statesByID: make(map[string]*OutputState),
	}
}

func (s *store) RecordAssemblyOutput(ctx context.Context, states []*components.FullState, potentials []*prototk.NewState) {
	// Derive AllowedNodes before acquiring the lock — no shared state is read here.
	allowedNodes := allowedNodesFromDistributionList(ctx, states, potentials)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, state := range states {
		stateID := state.ID.String()
		s.statesByID[stateID] = &OutputState{
			StateUpsert: components.StateUpsert{
				ID:     state.ID,
				Schema: state.Schema,
				Data:   state.Data,
			},
			AllowedNodes: allowedNodes[stateID],
		}
	}
}

func (s *store) GetForNode(node string) []*OutputState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*OutputState, 0, len(s.statesByID))
	for _, state := range s.statesByID {
		if nodeInAllowedList(state.AllowedNodes, node) {
			result = append(result, state)
		}
	}
	return result
}

func (s *store) ImportIfAbsent(stateID string, state *OutputState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.statesByID[stateID]; exists {
		return false
	}
	s.statesByID[stateID] = state
	return true
}

func (s *store) Delete(stateID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.statesByID, stateID)
}

// allowedNodesFromDistributionList builds the stateID → node names map for output states by
// reading the DistributionList that the domain included in its assembly response. This is the
// authoritative source for which nodes are permitted to hold each state's private data.
// If a locator cannot be parsed, a warning is logged and that recipient is skipped — the state
// is still stored but will be invisible to the unparseable node (default-deny).
func allowedNodesFromDistributionList(ctx context.Context, states []*components.FullState, potentials []*prototk.NewState) map[string][]string {
	allowedNodes := make(map[string][]string)
	for i, state := range states {
		if i >= len(potentials) {
			break
		}
		stateID := state.ID.String()
		for _, recipient := range potentials[i].DistributionList {
			node, err := pldtypes.PrivateIdentityLocator(recipient).Node(ctx, false)
			if err != nil {
				log.L(ctx).Warnf("statevisibilitytracker: could not extract node from locator %q: %s", recipient, err)
				continue
			}
			allowedNodes[stateID] = append(allowedNodes[stateID], node)
		}
	}
	return allowedNodes
}

// nodeInAllowedList reports whether node appears in the allowed list.
// A nil or empty allowed list means unknown distribution — the state is excluded (default-deny).
func nodeInAllowedList(allowed []string, node string) bool {
	for _, n := range allowed {
		if n == node {
			return true
		}
	}
	return false
}
