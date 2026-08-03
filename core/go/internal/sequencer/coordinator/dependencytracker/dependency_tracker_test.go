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

package dependencytracker

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDependencyTracker_GettersReturnDistinctChains(t *testing.T) {
	tr := NewDependencyTracker()
	pre := tr.GetPreassemblyDeps()
	post := tr.GetPostAssemblyDeps()
	ch := tr.GetChainedDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	pre.AddPrerequisite(t.Context(), a, b)
	post.AddPrerequisites(t.Context(), a, b, c)
	ch.AddPrerequisites(t.Context(), a, b, c)
	preReq, ok := pre.GetPrerequisite(t.Context(), a)
	require.True(t, ok)
	assert.Equal(t, b, preReq)
	assert.ElementsMatch(t, []uuid.UUID{b, c}, post.GetPrerequisites(t.Context(), a))
	assert.ElementsMatch(t, []uuid.UUID{b, c}, ch.GetPrerequisites(t.Context(), a))
}

func TestAddPrerequisites_PostAssembly_MultiplePrerequisites(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	d.AddPrerequisites(t.Context(), a, b, c)
	assert.ElementsMatch(t, []uuid.UUID{b, c}, d.GetPrerequisites(t.Context(), a))
	assert.Contains(t, d.GetDependents(t.Context(), b), a)
	assert.Contains(t, d.GetDependents(t.Context(), c), a)
}

func TestAddPrerequisites_PostAssembly_SelfReferenceSkipped(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a := uuid.New()
	d.AddPrerequisites(t.Context(), a, a)
	assert.Empty(t, d.GetPrerequisites(t.Context(), a))
	assert.Empty(t, d.GetDependents(t.Context(), a))
}

func TestAddPrerequisites_PostAssembly_Deduplicates(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisites(t.Context(), a, b, b)
	require.Len(t, d.GetPrerequisites(t.Context(), a), 1)
	assert.Equal(t, b, d.GetPrerequisites(t.Context(), a)[0])
}

func TestAddPrerequisites_PreAssembly_SinglePrerequisiteOK(t *testing.T) {
	d := NewDependencyTracker().GetPreassemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisite(t.Context(), a, b)
	prereq, ok := d.GetPrerequisite(t.Context(), a)
	require.True(t, ok)
	assert.Equal(t, b, prereq)
}

func TestAddPrerequisites_PreAssembly_ReplacesExistingPrerequisite(t *testing.T) {
	d := NewDependencyTracker().GetPreassemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	d.AddPrerequisite(t.Context(), a, b)
	d.AddPrerequisite(t.Context(), a, c)
	prereq, ok := d.GetPrerequisite(t.Context(), a)
	require.True(t, ok)
	assert.Equal(t, c, prereq)
	dependent, ok := d.GetDependent(t.Context(), c)
	require.True(t, ok)
	assert.Equal(t, a, dependent)
}

func TestAddPrerequisites_PreAssembly_SelfReferenceSkipped(t *testing.T) {
	d := NewDependencyTracker().GetPreassemblyDeps()
	a := uuid.New()

	d.AddPrerequisite(t.Context(), a, a)

	_, ok := d.GetPrerequisite(t.Context(), a)
	assert.False(t, ok)
	assert.False(t, d.HasPrerequisite(t.Context(), a))
	assert.False(t, d.HasDependent(t.Context(), a))
}

func TestSingleDependencyChain_WrapperMethods(t *testing.T) {
	d := NewDependencyTracker().GetPreassemblyDeps()
	txID, prereqID := uuid.New(), uuid.New()

	d.AddPrerequisite(t.Context(), txID, prereqID)
	assert.True(t, d.HasPrerequisite(t.Context(), txID))
	assert.True(t, d.HasDependent(t.Context(), prereqID))

	d.ClearPrerequisite(t.Context(), txID)
	assert.False(t, d.HasPrerequisite(t.Context(), txID))
	assert.False(t, d.HasDependent(t.Context(), prereqID))

	d.AddPrerequisite(t.Context(), txID, prereqID)
	d.ClearDependent(t.Context(), prereqID)
	assert.False(t, d.HasPrerequisite(t.Context(), txID))
	assert.False(t, d.HasDependent(t.Context(), prereqID))
}

func TestSingleDependencyChain_Getters_NotFound(t *testing.T) {
	d := NewDependencyTracker().GetPreassemblyDeps()
	unknown := uuid.New()

	_, ok := d.GetPrerequisite(t.Context(), unknown)
	assert.False(t, ok)
	_, ok = d.GetDependent(t.Context(), unknown)
	assert.False(t, ok)
}

func TestAddPrerequisites_PreAssembly_ReplacesExistingDependentOnPrereq(t *testing.T) {
	d := NewDependencyTracker().GetPreassemblyDeps()
	tx1, tx2, prereq := uuid.New(), uuid.New(), uuid.New()

	d.AddPrerequisite(t.Context(), tx1, prereq)
	d.AddPrerequisite(t.Context(), tx2, prereq)

	dependent, ok := d.GetDependent(t.Context(), prereq)
	require.True(t, ok)
	assert.Equal(t, tx2, dependent)
}

func TestAddPrerequisites_Chained_SinglePrerequisiteOK(t *testing.T) {
	ch := NewDependencyTracker().GetChainedDeps()
	a, b := uuid.New(), uuid.New()
	ch.AddPrerequisites(t.Context(), a, b)
	assert.Equal(t, []uuid.UUID{b}, ch.GetPrerequisites(t.Context(), a))
}

func TestAddPrerequisites_Chained_AllowsMultiplePrerequisites(t *testing.T) {
	ch := NewDependencyTracker().GetChainedDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	ch.AddPrerequisites(t.Context(), a, b, c)
	assert.ElementsMatch(t, []uuid.UUID{b, c}, ch.GetPrerequisites(t.Context(), a))
}

func TestClearPrerequisites_NoOpWhenUnknown(t *testing.T) {
	NewDependencyTracker().GetPostAssemblyDeps().ClearPrerequisites(t.Context(), uuid.New())
}

func TestClearPrerequisites_ClearsDependsOnAndBackLinks(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	d.AddPrerequisites(t.Context(), a, b, c)
	d.ClearPrerequisites(t.Context(), a)
	assert.Empty(t, d.GetPrerequisites(t.Context(), a))
	assert.NotContains(t, d.GetDependents(t.Context(), b), a)
	assert.NotContains(t, d.GetDependents(t.Context(), c), a)
}

func TestClearPrerequisites_SkipsMissingPrerequisiteNode(t *testing.T) {
	d := newDependencyChain()
	tx := uuid.New()
	ghost := uuid.New()
	d.nodes[tx] = &nodeLinks{dependsOn: []uuid.UUID{ghost}, prereqOf: nil}
	d.ClearPrerequisites(t.Context(), tx)
	require.NotNil(t, d.nodes[tx])
	assert.Empty(t, d.nodes[tx].dependsOn)
}

func TestClearDependents_NoOpWhenUnknown(t *testing.T) {
	NewDependencyTracker().GetPostAssemblyDeps().ClearDependents(t.Context(), uuid.New())
}

func TestClearDependents_ClearsPrereqOfAndBackLinks(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	d.AddPrerequisites(t.Context(), b, a)
	d.AddPrerequisites(t.Context(), c, a)
	assert.Contains(t, d.GetDependents(t.Context(), a), b)
	assert.Contains(t, d.GetDependents(t.Context(), a), c)
	d.ClearDependents(t.Context(), a)
	assert.Empty(t, d.GetDependents(t.Context(), a))
	assert.NotContains(t, d.GetPrerequisites(t.Context(), b), a)
	assert.NotContains(t, d.GetPrerequisites(t.Context(), c), a)
}

func TestClearDependents_SkipsMissingDependentNode(t *testing.T) {
	d := newDependencyChain()
	tx := uuid.New()
	ghost := uuid.New()
	d.nodes[tx] = &nodeLinks{dependsOn: nil, prereqOf: []uuid.UUID{ghost}}
	d.ClearDependents(t.Context(), tx)
	require.NotNil(t, d.nodes[tx])
	assert.Empty(t, d.nodes[tx].prereqOf)
}

func TestDelete_NoOpWhenUnknown(t *testing.T) {
	NewDependencyTracker().GetPostAssemblyDeps().Delete(t.Context(), uuid.New())
}

func TestDelete_RemovesNodeAndUpdatesPeers(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	d.AddPrerequisites(t.Context(), a, b)
	d.AddPrerequisites(t.Context(), c, a)
	d.Delete(t.Context(), a)
	assert.Empty(t, d.GetPrerequisites(t.Context(), a))
	assert.Empty(t, d.GetDependents(t.Context(), a))
	assert.Empty(t, d.GetDependents(t.Context(), b))
	assert.NotContains(t, d.GetPrerequisites(t.Context(), c), a)
}

func TestDelete_SkipsMissingDependentNode(t *testing.T) {
	d := newDependencyChain()
	tx := uuid.New()
	ghost := uuid.New()
	d.nodes[tx] = &nodeLinks{prereqOf: []uuid.UUID{ghost}, dependsOn: nil}
	d.Delete(t.Context(), tx)
	_, still := d.nodes[tx]
	assert.False(t, still)
}

func TestDelete_SkipsMissingPrerequisiteNode(t *testing.T) {
	d := newDependencyChain()
	tx := uuid.New()
	ghost := uuid.New()
	d.nodes[tx] = &nodeLinks{dependsOn: []uuid.UUID{ghost}, prereqOf: nil}
	d.Delete(t.Context(), tx)
	_, still := d.nodes[tx]
	assert.False(t, still)
}

func TestGetPrerequisites_GetDependents_NilWhenUnknown(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	unknown := uuid.New()
	assert.Nil(t, d.GetPrerequisites(t.Context(), unknown))
	assert.Nil(t, d.GetDependents(t.Context(), unknown))
}

func TestGetPrerequisites_GetDependents_AfterEdges(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisites(t.Context(), a, b)
	assert.Equal(t, []uuid.UUID{b}, d.GetPrerequisites(t.Context(), a))
	assert.Equal(t, []uuid.UUID{a}, d.GetDependents(t.Context(), b))
}

func TestGetDependents_ReturnsCopy(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisites(t.Context(), a, b)
	got := d.GetDependents(t.Context(), b)
	require.Len(t, got, 1)
	got[0] = uuid.Nil
	assert.Equal(t, a, d.GetDependents(t.Context(), b)[0])
}

func TestGetPrerequisites_ReturnsCopy(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisites(t.Context(), a, b)
	got := d.GetPrerequisites(t.Context(), a)
	require.Len(t, got, 1)
	got[0] = uuid.Nil
	assert.Equal(t, b, d.GetPrerequisites(t.Context(), a)[0])
}

func TestChained_UnassembledDependencies(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	tx := uuid.New()
	dep1, dep2 := uuid.New(), uuid.New()
	assert.Nil(t, cd.GetUnassembledDependencies(t.Context(), tx))

	cd.AddUnassembledDependencies(t.Context(), tx, dep1)
	require.NotNil(t, cd.GetUnassembledDependencies(t.Context(), tx))
	assert.Contains(t, cd.GetUnassembledDependencies(t.Context(), tx), dep1)

	cd.AddUnassembledDependencies(t.Context(), tx, dep2)
	assert.Contains(t, cd.GetUnassembledDependencies(t.Context(), tx), dep1)
	assert.Contains(t, cd.GetUnassembledDependencies(t.Context(), tx), dep2)

	cd.DeleteUnassembledDependencies(t.Context(), tx, dep1)
	assert.NotContains(t, cd.GetUnassembledDependencies(t.Context(), tx), dep1)
	assert.Contains(t, cd.GetUnassembledDependencies(t.Context(), tx), dep2)

	cd.DeleteUnassembledDependencies(t.Context(), tx, dep2)
	assert.NotContains(t, cd.GetUnassembledDependencies(t.Context(), tx), dep2)
}

func TestChained_DeleteUnassembledDependencies_UnknownTx(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	cd.DeleteUnassembledDependencies(t.Context(), uuid.New(), uuid.New())
}

func TestChained_AddUnassembledDependencies_SecondTx(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	tx1, tx2 := uuid.New(), uuid.New()
	d1, d2 := uuid.New(), uuid.New()
	cd.AddUnassembledDependencies(t.Context(), tx1, d1)
	cd.AddUnassembledDependencies(t.Context(), tx2, d2)
	assert.Contains(t, cd.GetUnassembledDependencies(t.Context(), tx1), d1)
	assert.Contains(t, cd.GetUnassembledDependencies(t.Context(), tx2), d2)
	assert.Len(t, cd.GetUnassembledDependencies(t.Context(), tx1), 1)
}

func TestChained_HasUnassembledDependencies(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	tx := uuid.New()
	dep := uuid.New()

	assert.False(t, cd.HasUnassembledDependencies(t.Context(), tx))
	cd.AddUnassembledDependencies(t.Context(), tx, dep)
	assert.True(t, cd.HasUnassembledDependencies(t.Context(), tx))
	cd.DeleteUnassembledDependencies(t.Context(), tx, dep)
	assert.False(t, cd.HasUnassembledDependencies(t.Context(), tx))
}

func TestChained_GetChainedChild_UnknownParent(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	got, ok := cd.GetChainedChild(t.Context(), uuid.New())
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)
}

func TestChained_GetChainedChild_NilWhenChildUnsetOrZero(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	parent := uuid.New()
	cd.SetChainedChild(t.Context(), parent, uuid.Nil)
	got, ok := cd.GetChainedChild(t.Context(), parent)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)
}

func TestChained_SetGetForgetChainedChild(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	parent, child := uuid.New(), uuid.New()
	got, ok := cd.GetChainedChild(t.Context(), parent)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)

	cd.SetChainedChild(t.Context(), parent, child)
	got, ok = cd.GetChainedChild(t.Context(), parent)
	require.True(t, ok)
	assert.Equal(t, child, got)

	cd.ForgetChainedChild(t.Context(), parent)
	got, ok = cd.GetChainedChild(t.Context(), parent)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)
}

func TestChained_ForgetChainedChild_NoOpWhenUnknownParent(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	cd.ForgetChainedChild(t.Context(), uuid.New())
}

func TestChained_Delete_RemovesMetadataAndDependencyLinks(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	parent := uuid.New()
	prereq := uuid.New()
	dependent := uuid.New()
	child := uuid.New()
	unassembled := uuid.New()

	cd.AddPrerequisites(t.Context(), parent, prereq)
	cd.AddPrerequisites(t.Context(), dependent, parent)
	cd.SetChainedChild(t.Context(), parent, child)
	cd.AddUnassembledDependencies(t.Context(), parent, unassembled)

	cd.Delete(t.Context(), parent)

	gotChild, ok := cd.GetChainedChild(t.Context(), parent)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, gotChild)
	assert.Nil(t, cd.GetUnassembledDependencies(t.Context(), parent))
	assert.Empty(t, cd.GetPrerequisites(t.Context(), dependent))
	assert.Empty(t, cd.GetDependents(t.Context(), prereq))
}

func TestDependencyTracker_Delete_RemovesAcrossAllChains(t *testing.T) {
	dt := NewDependencyTracker().(*dependencyTracker)
	txID := uuid.New()
	preReq := uuid.New()
	postReq := uuid.New()
	chainedReq := uuid.New()
	chainedChild := uuid.New()
	unassembled := uuid.New()

	dt.preAssembly.AddPrerequisite(t.Context(), txID, preReq)
	dt.postAssembly.AddPrerequisites(t.Context(), txID, postReq)
	dt.chained.AddPrerequisites(t.Context(), txID, chainedReq)
	dt.chained.SetChainedChild(t.Context(), txID, chainedChild)
	dt.chained.AddUnassembledDependencies(t.Context(), txID, unassembled)

	dt.Delete(t.Context(), txID)

	assert.Empty(t, dt.preAssembly.GetPrerequisites(t.Context(), txID))
	assert.Empty(t, dt.preAssembly.GetDependents(t.Context(), preReq))
	assert.Empty(t, dt.postAssembly.GetPrerequisites(t.Context(), txID))
	assert.Empty(t, dt.postAssembly.GetDependents(t.Context(), postReq))
	assert.Empty(t, dt.chained.GetPrerequisites(t.Context(), txID))
	assert.Empty(t, dt.chained.GetDependents(t.Context(), chainedReq))
	gotChild, ok := dt.chained.GetChainedChild(t.Context(), txID)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, gotChild)
	assert.Nil(t, dt.chained.GetUnassembledDependencies(t.Context(), txID))
}

func TestHasPrerequisitesAndHasDependents(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	tx := uuid.New()
	prereq := uuid.New()
	unknown := uuid.New()

	assert.False(t, d.HasPrerequisites(t.Context(), tx))
	assert.False(t, d.HasDependents(t.Context(), prereq))
	assert.False(t, d.HasPrerequisites(t.Context(), unknown))
	assert.False(t, d.HasDependents(t.Context(), unknown))

	d.AddPrerequisites(t.Context(), tx, prereq)
	assert.True(t, d.HasPrerequisites(t.Context(), tx))
	assert.True(t, d.HasDependents(t.Context(), prereq))

	d.ClearPrerequisites(t.Context(), tx)
	assert.False(t, d.HasPrerequisites(t.Context(), tx))
	assert.False(t, d.HasDependents(t.Context(), prereq))
}

func TestConcurrentAddPrerequisites(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	center := uuid.New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			leaf := uuid.New()
			d.AddPrerequisites(t.Context(), leaf, center)
		}()
	}
	wg.Wait()
	assert.Len(t, d.GetDependents(t.Context(), center), 50)
}

func TestAppendUnique_Direct(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	assert.Equal(t, []uuid.UUID{a}, appendUnique([]uuid.UUID{a}, a))
	assert.Equal(t, []uuid.UUID{a, b}, appendUnique([]uuid.UUID{a}, b))
}

func TestRemoveUUIDHelper(t *testing.T) {
	a := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	b := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	c := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	assert.Equal(t, []uuid.UUID{b, c}, removeUUID([]uuid.UUID{a, b, a, c, a}, a))
}
