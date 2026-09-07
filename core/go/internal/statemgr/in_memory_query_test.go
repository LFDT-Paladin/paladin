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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindMatchingInMemoryStates_LabelFilter(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	s10 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	s20 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)
	candidates := []*prototk.SnapshotState{snapshotStateOf(s10, 1), snapshotStateOf(s20, 2)}

	matches, err := ss.FindMatchingInMemoryStates(ctx, "domain1", schema1.ID(),
		query.NewQueryBuilder().Equal("amount", 20).Query(), candidates)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, s20.ID.String(), matches[0].State.GetId())
}

func TestFindMatchingInMemoryStates_SchemaFilter(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	schema2, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI2))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema2.Schema})
	require.NoError(t, err)

	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	candidates := []*prototk.SnapshotState{snapshotStateOf(s1, 1)}

	// Querying a different schema matches nothing.
	matches, err := ss.FindMatchingInMemoryStates(ctx, "domain1", schema2.ID(),
		query.NewQueryBuilder().Query(), candidates)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestFindMatchingInMemoryStates_SortAndLimit(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	s10 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	s20 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)
	s30 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 30)
	candidates := []*prototk.SnapshotState{snapshotStateOf(s10, 1), snapshotStateOf(s20, 2), snapshotStateOf(s30, 3)}

	// Sort descending on the amount label, limit 2.
	matches, err := ss.FindMatchingInMemoryStates(ctx, "domain1", schema1.ID(),
		query.NewQueryBuilder().Limit(2).Sort("-amount").Query(), candidates)
	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.Equal(t, s30.ID.String(), matches[0].State.GetId())
	assert.Equal(t, s20.ID.String(), matches[1].State.GetId())
}

func TestFindMatchingInMemoryStates_DefaultCreatedSort(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	late := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	early := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)
	candidates := []*prototk.SnapshotState{snapshotStateOf(late, 2000), snapshotStateOf(early, 1000)}

	// No sort instruction defaults to ascending ".created", mirroring the DB query default.
	matches, err := ss.FindMatchingInMemoryStates(ctx, "domain1", schema1.ID(),
		query.NewQueryBuilder().Query(), candidates)
	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.Equal(t, early.ID.String(), matches[0].State.GetId())
	assert.Equal(t, late.ID.String(), matches[1].State.GetId())
}

func TestFindMatchingInMemoryStates_EvalError(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	s1 := makeFakeCoin(t, ctx, schema1, contractAddress, false, 10)
	candidates := []*prototk.SnapshotState{snapshotStateOf(s1, 1)}

	_, err := ss.FindMatchingInMemoryStates(ctx, "domain1", schema1.ID(),
		query.NewQueryBuilder().Equal("wrong", "any").Query(), candidates)
	assert.Regexp(t, "PD010700", err)
}

func TestFindMatchingInMemoryStates_UnparseableCandidateSkipped(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	good := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)
	// A candidate whose state ID cannot be parsed is skipped rather than failing the whole query.
	bad := snapshotStateOf(good, 1)
	bad.State = &prototk.EndorsableState{Id: "not-hex", SchemaId: schema1.ID().String()}

	matches, err := ss.FindMatchingInMemoryStates(ctx, "domain1", schema1.ID(),
		query.NewQueryBuilder().Query(), []*prototk.SnapshotState{bad, snapshotStateOf(good, 2)})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, good.ID.String(), matches[0].State.GetId())
}

func TestFindMatchingInMemoryStates_SortError(t *testing.T) {
	ctx, ss, _, schema1, _, done := coinTestSetup(t)
	defer done()

	// With no candidates the per-candidate query evaluation never runs, so the unknown sort
	// field is first resolved (and rejected) by the sort itself.
	_, err := ss.FindMatchingInMemoryStates(ctx, "domain1", schema1.ID(),
		query.NewQueryBuilder().Sort("wrong").Query(), nil)
	assert.Regexp(t, "PD010700", err)
}

func TestFindMatchingInMemoryStates_BadSchemaIDCandidateSkipped(t *testing.T) {
	ctx, ss, _, schema1, contractAddress, done := coinTestSetup(t)
	defer done()

	good := makeFakeCoin(t, ctx, schema1, contractAddress, false, 20)
	// A candidate whose state ID parses but whose schema ID cannot be parsed is skipped rather
	// than failing the whole query.
	bad := snapshotStateOf(good, 1)
	bad.State = &prototk.EndorsableState{Id: pldtypes.RandHex(32), SchemaId: "not-a-schema"}

	matches, err := ss.FindMatchingInMemoryStates(ctx, "domain1", schema1.ID(),
		query.NewQueryBuilder().Query(), []*prototk.SnapshotState{bad, snapshotStateOf(good, 2)})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, good.ID.String(), matches[0].State.GetId())
}

const fakeGadgetABI = `{
	"type": "tuple",
	"internalType": "struct FakeGadget",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "size",
			"type": "int64",
			"indexed": true
		}
	]
}`

// TestFindMatchingInMemoryStates_Int64Labels proves a snapshot's int64 labels (produced by
// indexed int64 fields) are projected into the candidate's queryable label values.
func TestFindMatchingInMemoryStates_Int64Labels(t *testing.T) {
	ctx, ss, _, _, _, done := coinTestSetup(t)
	defer done()

	gadgetSchema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeGadgetABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{gadgetSchema.Schema})
	require.NoError(t, err)

	contractAddress := pldtypes.RandAddress()
	mkGadget := func(size int) *components.StateWithLabels {
		s, err := gadgetSchema.ProcessStateWithLabels(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
			`{"size": %d, "salt": "%s"}`, size, pldtypes.RandHex(32))), nil, false)
		require.NoError(t, err)
		return s
	}
	s42 := mkGadget(42)
	s43 := mkGadget(43)
	require.NotEmpty(t, s42.Int64Labels, "the size field must produce an int64 label")

	matches, err := ss.FindMatchingInMemoryStates(ctx, "domain1", gadgetSchema.ID(),
		query.NewQueryBuilder().Equal("size", 42).Query(),
		[]*prototk.SnapshotState{snapshotStateOf(s42, 1), snapshotStateOf(s43, 2)})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, s42.ID.String(), matches[0].State.GetId())
}

func TestFindMatchingInMemoryStates_UnknownSchema(t *testing.T) {
	ctx, ss, _, _, _, done := coinTestSetup(t)
	defer done()

	_, err := ss.FindMatchingInMemoryStates(ctx, "domain1", pldtypes.Bytes32(pldtypes.RandBytes(32)),
		query.NewQueryBuilder().Query(), nil)
	require.Error(t, err)
}
