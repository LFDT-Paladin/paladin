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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrapher_NewGrapher(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	assert.NotNil(t, g)
}

func TestAddMinter_Success(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
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

func TestAddMinter_RegistersSameGrapherTXInTransactionByIDAndTransactionByOutputState(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
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
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
	firstMinter := uuid.New()
	secondMinter := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("cc", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("dd", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, firstMinter))
	err := g.AddMinter(ctx, []*components.FullState{state}, secondMinter)
	require.Error(t, err)
	assert.ErrorContains(t, err, string(msgs.MsgSequencerGrapherAddMinterAlreadyExistsError))
}

func TestLockMintsOnSpend_DependsOnMinter(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	minterID := uuid.New()
	consumerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("ee", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("ff", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnSpend(ctx, []*components.FullState{state}, consumerID)

	assert.Equal(t, []uuid.UUID{minterID}, g.GetDependencies(ctx, consumerID))
}

func TestLockMintsOnRead_DependsOnMinter(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	minterID := uuid.New()
	readerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("11", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("22", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnRead(ctx, []*components.FullState{state}, readerID)

	assert.Equal(t, []uuid.UUID{minterID}, g.GetDependencies(ctx, readerID))
}

func TestGetDependencies_UnknownTransaction_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	assert.Nil(t, g.GetDependencies(ctx, uuid.New()))
}

func TestGetDependants_UnknownTransaction_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	assert.Nil(t, g.GetDependants(ctx, uuid.New()))
}

func TestGetDependants_ConsumerWithNoPrereqs_ReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	consumerID := uuid.New()
	unknown := pldtypes.MustParseHexBytes("0x" + strings.Repeat("b1", 32))
	g.LockMintsOnSpend(ctx, []*components.FullState{{ID: unknown}}, consumerID)

	deps := g.GetDependants(ctx, consumerID)
	require.NotNil(t, deps)
	assert.Empty(t, deps)
}

func TestGetDependants_ReturnsPrereqOf(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
	prereqID := uuid.New()
	dependentA := uuid.New()
	dependentB := uuid.New()

	g.mu.Lock()
	g.addConsumer(prereqID)
	g.addConsumer(dependentA)
	g.addConsumer(dependentB)
	prereqTX := g.transactionByID[prereqID]
	prereqTX.dependencies.PrereqOf = []uuid.UUID{dependentA, dependentB}
	g.mu.Unlock()

	assert.Equal(t, []uuid.UUID{dependentA, dependentB}, g.GetDependants(ctx, prereqID))
}

func TestLockMintsOnSpend_UnknownState_NoDependency(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	consumerID := uuid.New()
	unknown := pldtypes.MustParseHexBytes("0x" + strings.Repeat("33", 32))
	state := &components.FullState{ID: unknown}

	g.LockMintsOnSpend(ctx, []*components.FullState{state}, consumerID)
	assert.Empty(t, g.GetDependencies(ctx, consumerID))
}

func TestLockMintsOnSpend_MultipleStates_AppendsSpendLocks(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
	txID := uuid.New()
	s1 := pldtypes.MustParseHexBytes("0x" + strings.Repeat("de", 32))
	s2 := pldtypes.MustParseHexBytes("0x" + strings.Repeat("ef", 32))

	g.LockMintsOnSpend(ctx, []*components.FullState{{ID: s1}, {ID: s2}}, txID)

	locks := g.lockedStatesByTransaction[txID]
	require.Len(t, locks, 2)
	assert.True(t, locks[0].State.Equals(s1))
	assert.True(t, locks[1].State.Equals(s2))
	assert.Equal(t, pldapi.StateLockTypeSpend.Enum(), locks[0].Type)
}

func TestLockMintsOnCreate_LocksPotentialStates(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	txID := uuid.New()
	createdBy := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("44", 32))
	upserts := []*components.StateUpsert{
		{ID: stateID, CreatedBy: &createdBy},
	}
	states := []*components.FullState{{ID: stateID}}

	g.LockMintsOnCreate(ctx, upserts, states, txID)

	data, err := g.ExportMints(ctx)
	require.NoError(t, err)
	var exp exportableStates
	require.NoError(t, json.Unmarshal(data, &exp))
	require.Len(t, exp.LockedState, 1)
	assert.True(t, exp.LockedState[0].State.Equals(stateID))
	assert.Equal(t, txID, exp.LockedState[0].Transaction)
	assert.Equal(t, pldapi.StateLockTypeCreate.Enum(), exp.LockedState[0].Type)
}

func TestExportMints_OutputAndLocks(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	minterID := uuid.New()
	consumerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("55", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("66", 32)), Data: pldtypes.RawJSON(`{"x":1}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnRead(ctx, []*components.FullState{state}, consumerID)

	data, err := g.ExportMints(ctx)
	require.NoError(t, err)
	var exp exportableStates
	require.NoError(t, json.Unmarshal(data, &exp))
	require.Len(t, exp.OutputState, 1)
	assert.True(t, exp.OutputState[0].ID.Equals(stateID))
	require.Len(t, exp.LockedState, 1)
	assert.True(t, exp.LockedState[0].State.Equals(stateID))
}

func TestForget_ClearsPrereqOnMinterWhenConsumerForgotten(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
	minterID := uuid.New()
	consumerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("f0", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("f1", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnSpend(ctx, []*components.FullState{state}, consumerID)

	minterTX := g.transactionByID[minterID]
	require.Contains(t, minterTX.dependencies.PrereqOf, consumerID)

	require.NoError(t, g.Forget(consumerID))

	minterTX = g.transactionByID[minterID]
	require.NotContains(t, minterTX.dependencies.PrereqOf, consumerID)
}

func TestForget_ClearsMinterConsumerAndLocks(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
	minterID := uuid.New()
	consumerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("77", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("88", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnSpend(ctx, []*components.FullState{state}, consumerID)

	require.NoError(t, g.Forget(minterID))
	_, ok := g.transactionByOutputState[stateID.String()]
	assert.False(t, ok)
	_, ok = g.outputStatesByMinter[minterID]
	assert.False(t, ok)
}

func TestForget_UnknownTransaction_NoError(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	require.NoError(t, g.Forget(uuid.New()))
}

func TestForgetMints_RemovesOutputStatesOnly(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
	minterID := uuid.New()
	consumerID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("99", 32))
	state := &components.FullState{ID: stateID, Schema: pldtypes.MustParseBytes32("0x" + strings.Repeat("ab", 32)), Data: pldtypes.RawJSON(`{}`)}

	require.NoError(t, g.AddMinter(ctx, []*components.FullState{state}, minterID))
	g.LockMintsOnSpend(ctx, []*components.FullState{state}, consumerID)

	g.ForgetMints(minterID)
	_, ok := g.transactionByOutputState[stateID.String()]
	assert.False(t, ok)
	// Consumer entry remains until Forget
	_, ok = g.transactionByID[consumerID]
	assert.True(t, ok)
}

func TestForgetLocks(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
	txID := uuid.New()
	stateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("cd", 32))
	g.LockMintsOnRead(ctx, []*components.FullState{{ID: stateID}}, txID)

	g.ForgetLocks(txID)
	_, ok := g.lockedStatesByTransaction[txID]
	assert.False(t, ok)
}

func TestRemoveAllDependencyLinks_ClearsDependsOnForPrereqOfEntries(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
	prereqID := uuid.New()
	dependentID := uuid.New()

	g.mu.Lock()
	g.addConsumer(prereqID)
	g.addConsumer(dependentID)
	prereqTX := g.transactionByID[prereqID]
	dependentTX := g.transactionByID[dependentID]
	prereqTX.dependencies.PrereqOf = []uuid.UUID{dependentID}
	dependentTX.dependencies.DependsOn = []uuid.UUID{prereqID}
	g.mu.Unlock()

	g.RemoveAllDependencyLinks(prereqID)
	assert.Empty(t, dependentTX.dependencies.DependsOn)
}

func TestRemoveAllDependencyLinks_SkipsMissingLinkedTransactions(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx).(*grapher)
	prereqID := uuid.New()
	missingDependent := uuid.New()

	g.mu.Lock()
	g.addConsumer(prereqID)
	prereqTX := g.transactionByID[prereqID]
	prereqTX.dependencies.PrereqOf = []uuid.UUID{missingDependent}
	g.mu.Unlock()

	assert.NotPanics(t, func() {
		g.RemoveAllDependencyLinks(prereqID)
	})
}

func TestRemoveAllDependencyLinks_UnknownTransaction_NoOp(t *testing.T) {
	ctx := context.Background()
	g := NewGrapher(ctx)
	assert.NotPanics(t, func() {
		g.RemoveAllDependencyLinks(uuid.New())
	})
}

func Test_removeUUID(t *testing.T) {
	a := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	b := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	c := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	assert.Equal(t, []uuid.UUID{b, c}, removeUUID([]uuid.UUID{a, b, a, c, a}, a))
	assert.Empty(t, removeUUID([]uuid.UUID{a, a}, a))
	assert.Empty(t, removeUUID([]uuid.UUID{a}, a))
}
