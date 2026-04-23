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

package grapher

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/dependencytracker"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func testGrapher(t *testing.T) Grapher {
	t.Helper()
	return NewGrapher(testCtx(t), dependencytracker.NewDependencyTracker())
}

func testGrapherUnlocked(t *testing.T) *grapher {
	t.Helper()
	return NewGrapher(testCtx(t), dependencytracker.NewDependencyTracker()).(*grapher)
}

func TestGrapher_NewGrapher(t *testing.T) {
	g := testGrapher(t)
	assert.NotNil(t, g)
}

func TestAddMinter_Success(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	minterID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("aa", 32))

	err := g.AddMinter(ctx, []*components.FullState{
		{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("bb", 32)), Data: pldtypes.RawJSON(`{}`)},
	}, minterID)
	require.NoError(t, err)

	assert.Equal(t, minterID, g.transactionByOutputState[stateID.String()].ID)
	require.Contains(t, g.outputStatesByMinter, minterID)
	require.Len(t, g.outputStatesByMinter[minterID], 1)
	assert.True(t, g.outputStatesByMinter[minterID][0].ID.Equals(stateID))
}

func TestAddMinter_MultipleStates_AppendsToOutputStatesByMinter(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	minterID := uuid.New()
	s1 := pldtypes.MustParseHexBytes("0x" + strings.Repeat("01", 32))
	s2 := pldtypes.MustParseHexBytes("0x" + strings.Repeat("02", 32))
	states := []*components.FullState{
		{ID: s1, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("03", 32)), Data: pldtypes.RawJSON(`{}`)},
		{ID: s2, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("04", 32)), Data: pldtypes.RawJSON(`{}`)},
	}
	require.NoError(t, g.AddMinter(ctx, states, minterID))
	require.Len(t, g.outputStatesByMinter[minterID], 2)
	assert.True(t, g.outputStatesByMinter[minterID][0].ID.Equals(s1))
	assert.True(t, g.outputStatesByMinter[minterID][1].ID.Equals(s2))
}

func TestAddMinter_RegistersSameGrapherTXInTransactionByIDAndTransactionByOutputState(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	minterID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("c0", 32))
	states := []*components.FullState{
		{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("c1", 32)), Data: pldtypes.RawJSON(`{}`)},
	}

	require.NoError(t, g.AddMinter(ctx, states, minterID))

	txByID, ok := g.transactionByID[minterID]
	require.True(t, ok, "AddMinter should register the minter in transactionByID")
	assert.Equal(t, minterID, txByID.ID)

	txByOutput, ok := g.transactionByOutputState[stateID.String()]
	require.True(t, ok, "AddMinter should register each minted state in transactionByOutputState")
	assert.Same(t, txByID, txByOutput, "both indexes should reference the same grapherTX")
}

func TestAddMinter_AlreadyExists(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	firstMinter := uuid.New()
	secondMinter := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("cc", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("dd", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, firstMinter))
	err := g.AddMinter(ctx, []*components.FullState{state}, secondMinter)
	require.Error(t, err)
	assert.ErrorContains(t, err, string(msgs.MsgSequencerGrapherAddMinterAlreadyExistsError))
}

func TestAddConsumer_Idempotent(t *testing.T) {
	g := testGrapherUnlocked(t)
	txID := uuid.New()
	g.mu.Lock()
	g.addConsumer(txID)
	g.addConsumer(txID)
	g.mu.Unlock()
	require.Contains(t, g.transactionByID, txID)
}

func TestLockMintsOnSpend_DependsOnMinter(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	minterID := uuid.New()
	consumerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("ee", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("ff", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{}, []*components.FullState{state}, consumerID)

	assert.Equal(t, []uuid.UUID{minterID}, g.GetDependencies(ctx, consumerID))
}

func TestLockMintsOnRead_DependsOnMinter(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	minterID := uuid.New()
	readerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("11", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{state}, []*components.FullState{}, readerID)

	assert.Equal(t, []uuid.UUID{minterID}, g.GetDependencies(ctx, readerID))
}

func TestGetDependencies_UnknownTransaction_ReturnsNil(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	assert.Nil(t, g.GetDependencies(ctx, uuid.New()))
}

func TestGetDependents_UnknownTransaction_ReturnsNil(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	assert.Nil(t, g.GetDependents(ctx, uuid.New()))
}

func TestGetDependents_ConsumerWithNoReadPrereqs_ReturnsEmptySlice(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	consumerID := uuid.New()
	unknown := pldtypes.MustParseHexBytes("0x" + strings.Repeat("b1", 32))
	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{{ID: unknown}}, []*components.FullState{}, consumerID)

	assert.Empty(t, g.GetDependents(ctx, consumerID))
}

func TestGetDependents_ConsumerWithNoSpendPrereqs_ReturnsEmptySlice(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	consumerID := uuid.New()
	unknown := pldtypes.MustParseHexBytes("0x" + strings.Repeat("b1", 32))
	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{}, []*components.FullState{{ID: unknown}}, consumerID)

	assert.Empty(t, g.GetDependents(ctx, consumerID))
}

func TestGetDependents_ReturnsDependentsViaPrerequisiteEdges(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	prereqID := uuid.New()
	dependentA := uuid.New()
	dependentB := uuid.New()

	g.mu.Lock()
	g.addConsumer(prereqID)
	g.addConsumer(dependentA)
	g.addConsumer(dependentB)
	g.dependencyChain.AddPrerequisites(dependentA, prereqID)
	g.dependencyChain.AddPrerequisites(dependentB, prereqID)
	g.mu.Unlock()

	assert.ElementsMatch(t, []uuid.UUID{dependentA, dependentB}, g.GetDependents(ctx, prereqID))
}

func TestLockMintsOnSpend_UnknownReadState_NoDependency(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	consumerID := uuid.New()
	unknown := pldtypes.MustParseHexBytes("0x" + strings.Repeat("33", 32))
	state := &components.FullState{ID: unknown}

	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{state}, []*components.FullState{}, consumerID)
	assert.Empty(t, g.GetDependencies(ctx, consumerID))
}

func TestLockMintsOnSpend_UnknownSpendState_NoDependency(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	consumerID := uuid.New()
	unknown := pldtypes.MustParseHexBytes("0x" + strings.Repeat("33", 32))
	state := &components.FullState{ID: unknown}

	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{}, []*components.FullState{state}, consumerID)
	assert.Empty(t, g.GetDependencies(ctx, consumerID))
}

func TestLockMintsOnSpend_MultipleStates_AppendsSpendLocks(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	txID := uuid.New()
	s1 := pldtypes.MustParseHexBytes("0x" + strings.Repeat("de", 32))
	s2 := pldtypes.MustParseHexBytes("0x" + strings.Repeat("ef", 32))

	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{}, []*components.FullState{{ID: s1}, {ID: s2}}, txID)

	locks := g.lockedStatesByTransaction[txID]
	require.Len(t, locks, 2)
	assert.True(t, locks[0].State.Equals(s1))
	assert.True(t, locks[1].State.Equals(s2))
	assert.Equal(t, pldapi.StateLockTypeSpend.Enum(), locks[0].Type)
}

func TestLockMintsOnCreate_LocksPotentialStates(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	txID := uuid.New()
	createdBy := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("44", 32))
	upserts := []*components.StateUpsert{
		{ID: stateID, CreatedBy: &createdBy},
	}
	states := []*components.FullState{{ID: stateID}}

	g.LockMintsOnCreate(ctx, upserts, states, txID)

	data, err := g.ExportStatesAndLocks(ctx)
	require.NoError(t, err)
	var exp exportableStates
	require.NoError(t, json.Unmarshal(data, &exp))
	require.Len(t, exp.LockedState, 1)
	assert.True(t, exp.LockedState[0].State.Equals(stateID))
	assert.Equal(t, txID, exp.LockedState[0].Transaction)
	assert.Equal(t, pldapi.StateLockTypeCreate.Enum(), exp.LockedState[0].Type)
}

func TestLockMintsOnCreate_NoCreatedBy_NoLocks(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	txID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("90", 32))
	upserts := []*components.StateUpsert{{ID: stateID, CreatedBy: nil}}
	states := []*components.FullState{{ID: stateID}}

	g.LockMintsOnCreate(ctx, upserts, states, txID)

	data, err := g.ExportStatesAndLocks(ctx)
	require.NoError(t, err)
	var exp exportableStates
	require.NoError(t, json.Unmarshal(data, &exp))
	assert.Empty(t, exp.LockedState)
}

func TestLockMintsOnCreate_MixedCreatedBy_AppendsOnlyPotential(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	txID := uuid.New()
	createdBy := uuid.New()
	s1 := pldtypes.MustParseHexBytes("0x" + strings.Repeat("91", 32))
	s2 := pldtypes.MustParseHexBytes("0x" + strings.Repeat("92", 32))
	upserts := []*components.StateUpsert{
		{ID: s1, CreatedBy: nil},
		{ID: s2, CreatedBy: &createdBy},
	}
	states := []*components.FullState{{ID: s1}, {ID: s2}}

	g.LockMintsOnCreate(ctx, upserts, states, txID)

	data, err := g.ExportStatesAndLocks(ctx)
	require.NoError(t, err)
	var exp exportableStates
	require.NoError(t, json.Unmarshal(data, &exp))
	require.Len(t, exp.LockedState, 1)
	assert.True(t, exp.LockedState[0].State.Equals(s2))
}

func TestExportStatesAndLocks_OutputAndLocks(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	minterID := uuid.New()
	consumerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("55", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("66", 32)), Data: pldtypes.RawJSON(`{"x":1}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{state}, []*components.FullState{}, consumerID)

	data, err := g.ExportStatesAndLocks(ctx)
	require.NoError(t, err)
	var exp exportableStates
	require.NoError(t, json.Unmarshal(data, &exp))
	require.Len(t, exp.OutputState, 1)
	assert.True(t, exp.OutputState[0].ID.Equals(stateID))
	require.Len(t, exp.LockedState, 1)
	assert.True(t, exp.LockedState[0].State.Equals(stateID))
}

func TestExportStatesAndLocks_EmptyGrapher(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	data, err := g.ExportStatesAndLocks(ctx)
	require.NoError(t, err)
	var exp exportableStates
	require.NoError(t, json.Unmarshal(data, &exp))
	assert.Empty(t, exp.OutputState)
	assert.Empty(t, exp.LockedState)
}

func TestForget_UnknownTransaction_RemoveAllDependencyLinksEarlyReturn(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapher(t)
	unknown := uuid.New()
	g.Forget(unknown)
	assert.Nil(t, g.GetDependencies(ctx, unknown))
}

func TestForget_RemoveAllDependencyLinks_SkipsMissingDependent(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	ghostDependent := uuid.New()
	realDependent := uuid.New()
	minterID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("a0", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("a1", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{}, []*components.FullState{state}, realDependent)

	g.mu.Lock()
	g.addConsumer(ghostDependent)
	g.dependencyChain.AddPrerequisites(ghostDependent, minterID)
	g.mu.Unlock()

	g.Forget(minterID)

	assert.Empty(t, g.GetDependencies(ctx, realDependent))
}

func TestForget_RemoveAllDependencyLinks_SkipsMissingPrerequisite(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	txID := uuid.New()
	ghostPrereq := uuid.New()
	minterID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("b0", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("b1", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))

	g.mu.Lock()
	g.addConsumer(ghostPrereq)
	g.addConsumer(txID)
	g.dependencyChain.AddPrerequisites(txID, minterID, ghostPrereq)
	g.mu.Unlock()

	g.Forget(txID)

	assert.NotContains(t, g.GetDependents(ctx, minterID), txID)
}

func TestForget_ClearsPrereqOnMinterWhenConsumerForgotten(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	minterID := uuid.New()
	consumerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("f0", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("f1", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{}, []*components.FullState{state}, consumerID)

	require.Contains(t, g.GetDependents(ctx, minterID), consumerID)

	g.Forget(consumerID)

	assert.NotContains(t, g.GetDependents(ctx, minterID), consumerID)
}

func TestForget_ClearsMinterConsumerAndLocks(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	minterID := uuid.New()
	consumerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("77", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("88", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{}, []*components.FullState{state}, consumerID)
	g.Forget(minterID)
	_, ok := g.transactionByOutputState[stateID.String()]
	assert.False(t, ok)
	_, ok = g.outputStatesByMinter[minterID]
	assert.False(t, ok)
}

func TestForget_ClearsLocksForTransaction(t *testing.T) {
	ctx := testCtx(t)
	g := testGrapherUnlocked(t)
	txID := uuid.New()
	s := pldtypes.MustParseHexBytes("0x" + strings.Repeat("ab", 32))
	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{{ID: s}}, []*components.FullState{}, txID)
	require.Contains(t, g.lockedStatesByTransaction, txID)
	g.Forget(txID)
	_, ok := g.lockedStatesByTransaction[txID]
	assert.False(t, ok)
}

func TestLockMints_InitializesNilLockedStatesMap(t *testing.T) {
	ctx := testCtx(t)
	deps := dependencytracker.NewDependencyTracker()
	g := &grapher{
		dependencyChain:           deps.GetPostAssemblyDeps(),
		transactionByOutputState:  make(map[string]*grapherTX),
		transactionByID:           make(map[uuid.UUID]*grapherTX),
		outputStatesByMinter:      make(map[uuid.UUID][]*components.StateUpsert),
		lockedStatesByTransaction: nil,
	}
	txID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("e1", 32))

	g.LockMintsOnReadAndSpend(ctx, []*components.FullState{{ID: stateID}}, []*components.FullState{}, txID)

	require.NotNil(t, g.lockedStatesByTransaction)
	locks := g.lockedStatesByTransaction[txID]
	require.Len(t, locks, 1)
	assert.True(t, locks[0].State.Equals(stateID))
	assert.Equal(t, pldapi.StateLockTypeRead.Enum(), locks[0].Type)
}
