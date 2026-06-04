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

package statevisibilitytracker

import (
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore() *store {
	return NewStore().(*store)
}

// testStateID returns a 32-byte state ID where every byte is the given value.
// Only values 0x00–0x0f are valid (single hex digit per byte pair), so we use
// a two-hex-digit representation of the value directly.
func testStateID(hex2 string) pldtypes.HexBytes {
	return pldtypes.MustParseHexBytes("0x" + strings.Repeat(hex2, 32))
}

// --- RecordAssemblyOutput ---

func TestRecordAssemblyOutput_DerivesNodesFromDistributionList(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("aa")
	schema := pldtypes.MustParseBytes32("0x" + strings.Repeat("bb", 32))

	states := []*components.FullState{{ID: stateID, Schema: schema, Data: pldtypes.RawJSON(`{}`)}}
	potentials := []*prototk.NewState{{DistributionList: []string{"alice@node1", "bob@node2"}}}

	s.RecordAssemblyOutput(ctx, states, potentials)

	out, ok := s.statesByID[stateID.String()]
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"node1", "node2"}, out.AllowedNodes)
}

func TestRecordAssemblyOutput_BadLocator_StateStoredButInvisible(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("ab")

	states := []*components.FullState{{ID: stateID}}
	potentials := []*prototk.NewState{{DistributionList: []string{"not-a-valid-locator"}}}

	s.RecordAssemblyOutput(ctx, states, potentials)

	// State is stored (no data loss) but AllowedNodes is empty — default-deny means nobody sees it.
	out, ok := s.statesByID[stateID.String()]
	require.True(t, ok, "state must be stored even when locator parsing fails")
	assert.Empty(t, out.AllowedNodes, "unparseable locator must not produce an allowed node")
	assert.Empty(t, s.GetForNode("any-node"), "state with empty AllowedNodes must be invisible to every node")
}

func TestRecordAssemblyOutput_EmptyDistributionList_StateStoredButInvisible(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("ac")

	states := []*components.FullState{{ID: stateID}}
	potentials := []*prototk.NewState{{DistributionList: nil}}

	s.RecordAssemblyOutput(ctx, states, potentials)

	out, ok := s.statesByID[stateID.String()]
	require.True(t, ok)
	assert.Empty(t, out.AllowedNodes)
	assert.Empty(t, s.GetForNode("any-node"))
}

func TestRecordAssemblyOutput_MoreStatesThanPotentials_ExtraStateInvisible(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	s1 := testStateID("ad")
	s2 := testStateID("ae")

	states := []*components.FullState{{ID: s1}, {ID: s2}}
	potentials := []*prototk.NewState{{DistributionList: []string{"alice@node1"}}} // only one potential

	s.RecordAssemblyOutput(ctx, states, potentials)

	// s1 has a corresponding potential → AllowedNodes populated
	out1, ok := s.statesByID[s1.String()]
	require.True(t, ok)
	assert.Equal(t, []string{"node1"}, out1.AllowedNodes)

	// s2 has no corresponding potential → stored with nil AllowedNodes (default-deny, invisible to all)
	out2, ok := s.statesByID[s2.String()]
	require.True(t, ok, "state must be stored even without a corresponding potential")
	assert.Empty(t, out2.AllowedNodes, "state without a potential must have empty AllowedNodes (default-deny)")

	// node1 sees only s1 (has AllowedNodes=["node1"]); s2 is invisible (nil AllowedNodes)
	node1States := s.GetForNode("node1")
	require.Len(t, node1States, 1)
	assert.True(t, node1States[0].ID.Equals(s1))
}

// --- GetForNode — enforcement of the default-deny posture ---

func TestGetForNode_OnlyAllowedNodeSeesState(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("af")

	s.RecordAssemblyOutput(ctx,
		[]*components.FullState{{ID: stateID}},
		[]*prototk.NewState{{DistributionList: []string{"alice@node1"}}},
	)

	assert.Len(t, s.GetForNode("node1"), 1, "node1 is in AllowedNodes and must receive the state")
	assert.Empty(t, s.GetForNode("node2"), "node2 is not in AllowedNodes and must not receive the state")
}

func TestGetForNode_NilAllowedNodes_DefaultDeny(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b0")
	s.statesByID[stateID.String()] = &OutputState{
		StateUpsert:  components.StateUpsert{ID: stateID},
		AllowedNodes: nil,
	}

	assert.Empty(t, s.GetForNode("node1"), "nil AllowedNodes must be default-deny for every node")
}

func TestGetForNode_EmptyAllowedNodes_DefaultDeny(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b1")
	s.statesByID[stateID.String()] = &OutputState{
		StateUpsert:  components.StateUpsert{ID: stateID},
		AllowedNodes: []string{},
	}

	assert.Empty(t, s.GetForNode("node1"), "empty AllowedNodes must be default-deny for every node")
}

func TestGetForNode_MultipleStates_FiltersCorrectly(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	s1 := testStateID("b2")
	s2 := testStateID("b3")
	s3 := testStateID("b4")

	s.RecordAssemblyOutput(ctx,
		[]*components.FullState{{ID: s1}},
		[]*prototk.NewState{{DistributionList: []string{"alice@node1"}}},
	)
	s.RecordAssemblyOutput(ctx,
		[]*components.FullState{{ID: s2}},
		[]*prototk.NewState{{DistributionList: []string{"bob@node2"}}},
	)
	// s3: nil AllowedNodes via direct insert
	s.statesByID[s3.String()] = &OutputState{StateUpsert: components.StateUpsert{ID: s3}}

	node1States := s.GetForNode("node1")
	require.Len(t, node1States, 1)
	assert.True(t, node1States[0].ID.Equals(s1))

	node2States := s.GetForNode("node2")
	require.Len(t, node2States, 1)
	assert.True(t, node2States[0].ID.Equals(s2))
}

// --- ImportIfAbsent — coordinator handover safety ---

func TestImportIfAbsent_StoresWhenAbsent(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b5")
	state := &OutputState{
		StateUpsert:  components.StateUpsert{ID: stateID},
		AllowedNodes: []string{"node1"},
	}

	imported := s.ImportIfAbsent(stateID.String(), state)
	assert.True(t, imported)
	assert.Contains(t, s.statesByID, stateID.String())
}

func TestImportIfAbsent_ExistingEntryTakesPrecedence(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b6")

	original := &OutputState{
		StateUpsert:  components.StateUpsert{ID: stateID},
		AllowedNodes: []string{"node1"},
	}
	s.statesByID[stateID.String()] = original

	// Attempt to import different AllowedNodes for the same state.
	imported := s.ImportIfAbsent(stateID.String(), &OutputState{
		StateUpsert:  components.StateUpsert{ID: stateID},
		AllowedNodes: []string{"node2"},
	})

	assert.False(t, imported, "ImportIfAbsent must not overwrite an existing entry")
	assert.Equal(t, original, s.statesByID[stateID.String()], "existing entry must be unchanged")
	assert.Equal(t, []string{"node1"}, s.statesByID[stateID.String()].AllowedNodes, "AllowedNodes must not change")
}

// --- Delete ---

func TestDelete_StateNoLongerVisible(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("b9")

	s.RecordAssemblyOutput(ctx,
		[]*components.FullState{{ID: stateID}},
		[]*prototk.NewState{{DistributionList: []string{"alice@node1"}}},
	)
	require.Len(t, s.GetForNode("node1"), 1)

	s.Delete(stateID.String())

	assert.Empty(t, s.GetForNode("node1"), "deleted state must be invisible to all nodes")
	assert.NotContains(t, s.statesByID, stateID.String(), "deleted state must not be tracked")
}

func TestDelete_NoOp_WhenAbsent(t *testing.T) {
	s := newTestStore()
	// Must not panic
	s.Delete(testStateID("ba").String())
}
