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
	"context"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
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

var testSchemaID = pldtypes.MustParseBytes32("0x" + strings.Repeat("bb", 32))

// endorsableState builds a wire state carrying the shared test schema, as every state produced by
// the resolve path does.
func endorsableState(id pldtypes.HexBytes) *prototk.EndorsableState {
	return &prototk.EndorsableState{Id: id.String(), SchemaId: testSchemaID.String()}
}

// resolvedState builds the StateWithLabels that state resolution would produce for a state,
// carrying the given string and int64 labels.
func resolvedState(id pldtypes.HexBytes, labels map[string]string, int64Labels map[string]int64) *components.StateWithLabels {
	state := &pldapi.State{StateBase: pldapi.StateBase{ID: id}}
	for l, v := range labels {
		state.Labels = append(state.Labels, &pldapi.StateLabel{State: id, Label: l, Value: v})
	}
	for l, v := range int64Labels {
		state.Int64Labels = append(state.Int64Labels, &pldapi.StateInt64Label{State: id, Label: l, Value: v})
	}
	return &components.StateWithLabels{State: state}
}

// outputSpec is a test-only bundle of the three per-state inputs to RecordAssemblyOutput: the wire
// state, its distribution list, and its resolved labels. record splits these into the three slices
// the real call takes.
type outputSpec struct {
	state            *prototk.EndorsableState
	distributionList []string
	resolved         *components.StateWithLabels
}

func output(state *prototk.EndorsableState, distributionList []string, resolved *components.StateWithLabels) outputSpec {
	return outputSpec{state: state, distributionList: distributionList, resolved: resolved}
}

// record calls RecordAssemblyOutput, building the three index-aligned slices from the specs.
// A nil resolved state leaves Labels nil (an unlabelled state, no ref).
func record(ctx context.Context, s *store, specs ...outputSpec) {
	states := make([]*prototk.EndorsableState, len(specs))
	labels := make([]*prototk.StateLabels, len(specs))
	distributionLists := make([][]string, len(specs))
	for i, sp := range specs {
		states[i] = sp.state
		if sp.resolved != nil {
			labels[i] = sp.resolved.ProtoLabels()
		}
		distributionLists[i] = sp.distributionList
	}
	s.RecordAssemblyOutput(ctx, states, labels, distributionLists)
}

// --- RecordAssemblyOutput ---

func TestRecordAssemblyOutput_DerivesNodesFromDistributionList(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("aa")
	schema := pldtypes.MustParseBytes32("0x" + strings.Repeat("bb", 32))

	state := &prototk.EndorsableState{Id: stateID.String(), SchemaId: schema.String(), StateDataJson: `{}`}
	record(ctx, s,
		output(state, []string{"alice@node1", "bob@node2"}, nil),
	)

	out, ok := s.statesByID[stateID.String()]
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"node1", "node2"}, out.AllowedNodes)
}

func TestRecordAssemblyOutput_BadLocator_StateStoredButInvisible(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("ab")

	record(ctx, s,
		output(endorsableState(stateID), []string{"not-a-valid-locator"}, nil),
	)

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

	record(ctx, s,
		output(endorsableState(stateID), nil, nil),
	)

	out, ok := s.statesByID[stateID.String()]
	require.True(t, ok)
	assert.Empty(t, out.AllowedNodes)
	assert.Empty(t, s.GetForNode("any-node"))
}

func TestRecordAssemblyOutput_StoresLabelsAndAdvertises(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("aa")

	state := &prototk.EndorsableState{Id: stateID.String(), SchemaId: testSchemaID.String(), StateDataJson: `{}`}
	record(ctx, s,
		output(state, []string{"alice@node1"}, resolvedState(stateID, map[string]string{"owner": "0xfeed"}, map[string]int64{"amount": 42})),
	)

	out, ok := s.statesByID[stateID.String()]
	require.True(t, ok)
	require.NotNil(t, out.GetLabels())
	require.Len(t, out.GetLabels().GetLabels(), 1)
	assert.Equal(t, "owner", out.GetLabels().GetLabels()[0].GetLabel())
	assert.Equal(t, "0xfeed", out.GetLabels().GetLabels()[0].GetValue())
	require.Len(t, out.GetLabels().GetInt64Labels(), 1)
	assert.Equal(t, "amount", out.GetLabels().GetInt64Labels()[0].GetLabel())
	assert.Equal(t, int64(42), out.GetLabels().GetInt64Labels()[0].GetValue())

	// A labelled state is advertised to its allowed node; projection into its query form is statemgr's job.
	require.Len(t, s.GetForNode("node1"), 1, "a labelled state must be advertised")
}

func TestRecordAssemblyOutput_StampsCreatedOnSnapshot(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("aa")

	state := &prototk.EndorsableState{Id: stateID.String(), SchemaId: testSchemaID.String(), StateDataJson: `{}`}
	record(ctx, s,
		output(state, []string{"alice@node1"}, resolvedState(stateID, map[string]string{"owner": "0xfeed"}, nil)),
	)

	out, ok := s.statesByID[stateID.String()]
	require.True(t, ok)
	assert.NotZero(t, out.GetCreated(), "store must stamp a coordinator-local created on the snapshot")
}

func TestRecordAssemblyOutput_MultipleOutputs_EachLabelledIndependently(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	s1 := testStateID("a1")
	s2 := testStateID("a2")

	record(ctx, s,
		output(endorsableState(s1), []string{"alice@node1"}, resolvedState(s1, map[string]string{"which": "one"}, nil)),
		output(endorsableState(s2), []string{"alice@node1"}, resolvedState(s2, map[string]string{"which": "two"}, nil)),
	)

	assert.Equal(t, "one", s.statesByID[s1.String()].GetLabels().GetLabels()[0].GetValue())
	assert.Equal(t, "two", s.statesByID[s2.String()].GetLabels().GetLabels()[0].GetValue())
}

func TestRecordAssemblyOutput_NoResolvedState_NotAdvertised(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("a4")

	record(ctx, s,
		output(endorsableState(stateID), []string{"alice@node1"}, nil),
	)

	// A state recorded without labels is internally inconsistent (assembly always resolves labels),
	// so it is stored but never advertised.
	out := s.statesByID[stateID.String()]
	assert.Nil(t, out.GetLabels())
	assert.Empty(t, s.GetForNode("node1"), "a label-less state must never be advertised")
}

func TestRecordAssemblyOutput_EmptyLabelSet_Advertised(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("a5")

	// A schema with no labelled fields resolves to a state with zero labels — distinct from
	// labels being unknown (nil), the state must still be advertised (queryable by its base labels).
	record(ctx, s,
		output(endorsableState(stateID), []string{"alice@node1"}, resolvedState(stateID, nil, nil)),
	)

	out := s.statesByID[stateID.String()]
	require.NotNil(t, out.GetLabels())
	assert.Empty(t, out.GetLabels().GetLabels())
	assert.Empty(t, out.GetLabels().GetInt64Labels())
	assert.Len(t, s.GetForNode("node1"), 1)
}

// --- GetForNode — enforcement of the default-deny posture ---

func TestGetForNode_OnlyAllowedNodeSeesState(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("af")

	record(ctx, s,
		output(endorsableState(stateID), []string{"alice@node1"}, resolvedState(stateID, map[string]string{"owner": "0xfeed"}, nil)),
	)

	assert.Len(t, s.GetForNode("node1"), 1, "node1 is in AllowedNodes and must receive the state")
	assert.Empty(t, s.GetForNode("node2"), "node2 is not in AllowedNodes and must not receive the state")
}

func TestGetForNode_NilAllowedNodes_DefaultDeny(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b0")
	s.statesByID[stateID.String()] = &prototk.SnapshotState{
		State:        &prototk.EndorsableState{Id: stateID.String()},
		AllowedNodes: nil,
	}

	assert.Empty(t, s.GetForNode("node1"), "nil AllowedNodes must be default-deny for every node")
}

func TestGetForNode_EmptyAllowedNodes_DefaultDeny(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b1")
	s.statesByID[stateID.String()] = &prototk.SnapshotState{
		State:        &prototk.EndorsableState{Id: stateID.String()},
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

	record(ctx, s,
		output(endorsableState(s1), []string{"alice@node1"}, resolvedState(s1, map[string]string{"owner": "0xfeed"}, nil)),
	)
	record(ctx, s,
		output(endorsableState(s2), []string{"bob@node2"}, resolvedState(s2, map[string]string{"owner": "0xbead"}, nil)),
	)
	// s3: nil AllowedNodes via direct insert
	s.statesByID[s3.String()] = &prototk.SnapshotState{State: &prototk.EndorsableState{Id: s3.String()}}

	node1States := s.GetForNode("node1")
	require.Len(t, node1States, 1)
	assert.Equal(t, s1.String(), node1States[0].GetState().GetId())

	node2States := s.GetForNode("node2")
	require.Len(t, node2States, 1)
	assert.Equal(t, s2.String(), node2States[0].GetState().GetId())
}

func TestGetForNode_FiltersByNodeAndLabelPresence(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	labelled := testStateID("b5")
	unlabelled := testStateID("b6")
	otherNode := testStateID("b7")

	record(ctx, s,
		output(endorsableState(labelled), []string{"alice@node1"}, resolvedState(labelled, map[string]string{"owner": "0xfeed"}, nil)),
		output(endorsableState(unlabelled), []string{"alice@node1"}, nil),
	)
	record(ctx, s,
		output(endorsableState(otherNode), []string{"bob@node2"}, resolvedState(otherNode, map[string]string{"owner": "0xbead"}, nil)),
	)

	states := s.GetForNode("node1")
	require.Len(t, states, 1, "only labelled states visible to node1 must be advertised")
	assert.Equal(t, labelled.String(), states[0].GetState().GetId())

	assert.Empty(t, s.GetForNode("node3"), "nodes with no visibility get nothing")
}

// --- RangeForNode — same node/label filter as GetForNode, without the intermediate slice ---

func TestRangeForNode_FiltersByNodeAndLabelPresence(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	labelled := testStateID("b8")
	unlabelled := testStateID("b9")
	otherNode := testStateID("ba")

	record(ctx, s,
		output(endorsableState(labelled), []string{"alice@node1"}, resolvedState(labelled, map[string]string{"owner": "0xfeed"}, nil)),
		output(endorsableState(unlabelled), []string{"alice@node1"}, nil),
	)
	record(ctx, s,
		output(endorsableState(otherNode), []string{"bob@node2"}, resolvedState(otherNode, map[string]string{"owner": "0xbead"}, nil)),
	)

	var seen []*prototk.SnapshotState
	s.RangeForNode("node1", func(state *prototk.SnapshotState) {
		seen = append(seen, state)
	})
	require.Len(t, seen, 1, "only labelled states visible to node1 must be ranged over")
	assert.Equal(t, labelled.String(), seen[0].GetState().GetId())

	s.RangeForNode("node3", func(state *prototk.SnapshotState) {
		t.Errorf("nodes with no visibility must get no callbacks, got state %s", state.GetState().GetId())
	})
}

// --- ImportIfAbsent — coordinator handover safety ---

func TestImportIfAbsent_StoresWhenAbsent(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b5")
	state := &prototk.SnapshotState{
		State:        &prototk.EndorsableState{Id: stateID.String()},
		AllowedNodes: []string{"node1"},
	}

	imported := s.ImportIfAbsent(stateID.String(), state)
	assert.True(t, imported)
	assert.Contains(t, s.statesByID, stateID.String())
}

func TestImportIfAbsent_WithLabels_Advertised(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b8")
	state := &prototk.SnapshotState{
		State:        endorsableState(stateID),
		AllowedNodes: []string{"node1"},
		Labels:       &prototk.StateLabels{Labels: []*prototk.StateLabel{{Label: "owner", Value: "0xfeed"}}},
	}

	require.True(t, s.ImportIfAbsent(stateID.String(), state))
	states := s.GetForNode("node1")
	require.Len(t, states, 1)
	assert.Equal(t, stateID.String(), states[0].GetState().GetId())
}

func TestImportIfAbsent_WithoutLabels_NotAdvertised(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b9")
	state := &prototk.SnapshotState{
		State:        endorsableState(stateID),
		AllowedNodes: []string{"node1"},
	}

	// A label-less state is stored (handover fidelity) but never advertised.
	require.True(t, s.ImportIfAbsent(stateID.String(), state))
	assert.Contains(t, s.statesByID, stateID.String())
	assert.Empty(t, s.GetForNode("node1"))
}

func TestImportIfAbsent_ExistingEntryTakesPrecedence(t *testing.T) {
	s := newTestStore()
	stateID := testStateID("b6")

	original := &prototk.SnapshotState{
		State:        &prototk.EndorsableState{Id: stateID.String()},
		AllowedNodes: []string{"node1"},
	}
	s.statesByID[stateID.String()] = original

	// Attempt to import different AllowedNodes for the same state.
	imported := s.ImportIfAbsent(stateID.String(), &prototk.SnapshotState{
		State:        &prototk.EndorsableState{Id: stateID.String()},
		AllowedNodes: []string{"node2"},
	})

	assert.False(t, imported, "ImportIfAbsent must not overwrite an existing entry")
	assert.Same(t, original, s.statesByID[stateID.String()], "existing entry must be unchanged")
	assert.Equal(t, []string{"node1"}, s.statesByID[stateID.String()].AllowedNodes, "AllowedNodes must not change")
}

// --- Delete ---

func TestDelete_StateNoLongerVisible(t *testing.T) {
	ctx := t.Context()
	s := newTestStore()
	stateID := testStateID("b9")

	record(ctx, s,
		output(endorsableState(stateID), []string{"alice@node1"}, resolvedState(stateID, map[string]string{"owner": "0xfeed"}, nil)),
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
