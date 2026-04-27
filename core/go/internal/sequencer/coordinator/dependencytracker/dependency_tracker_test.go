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
	a, b := uuid.New(), uuid.New()
	pre.AddPrerequisites(a, b)
	post.AddPrerequisites(a, b)
	ch.AddPrerequisites(a, b)
	assert.ElementsMatch(t, []uuid.UUID{b}, pre.GetPrerequisites(a))
	assert.ElementsMatch(t, []uuid.UUID{b}, post.GetPrerequisites(a))
	assert.ElementsMatch(t, []uuid.UUID{b}, ch.GetPrerequisites(a))
}

func TestAddPrerequisites_PostAssembly_MultiplePrerequisites(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	d.AddPrerequisites(a, b, c)
	assert.ElementsMatch(t, []uuid.UUID{b, c}, d.GetPrerequisites(a))
	assert.Contains(t, d.GetDependents(b), a)
	assert.Contains(t, d.GetDependents(c), a)
}

func TestAddPrerequisites_PostAssembly_SelfReferenceSkipped(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a := uuid.New()
	d.AddPrerequisites(a, a)
	assert.Empty(t, d.GetPrerequisites(a))
	assert.Empty(t, d.GetDependents(a))
}

func TestAddPrerequisites_PostAssembly_Deduplicates(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisites(a, b, b)
	require.Len(t, d.GetPrerequisites(a), 1)
	assert.Equal(t, b, d.GetPrerequisites(a)[0])
}

func TestAddPrerequisites_PreAssembly_SinglePrerequisiteOK(t *testing.T) {
	d := NewDependencyTracker().GetPreassemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisites(a, b)
	assert.Equal(t, []uuid.UUID{b}, d.GetPrerequisites(a))
}

func TestAddPrerequisites_PreAssembly_PanicsOnMultiplePrerequisites(t *testing.T) {
	d := NewDependencyTracker().GetPreassemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	require.Panics(t, func() {
		d.AddPrerequisites(a, b, c)
	})
}

func TestAddPrerequisites_Chained_SinglePrerequisiteOK(t *testing.T) {
	ch := NewDependencyTracker().GetChainedDeps()
	a, b := uuid.New(), uuid.New()
	ch.AddPrerequisites(a, b)
	assert.Equal(t, []uuid.UUID{b}, ch.GetPrerequisites(a))
}

func TestAddPrerequisites_Chained_PanicsOnMultiplePrerequisites(t *testing.T) {
	ch := NewDependencyTracker().GetChainedDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	require.Panics(t, func() {
		ch.AddPrerequisites(a, b, c)
	})
}

func TestClearPrerequisites_NoOpWhenUnknown(t *testing.T) {
	NewDependencyTracker().GetPostAssemblyDeps().ClearPrerequisites(uuid.New())
}

func TestClearPrerequisites_ClearsDependsOnAndBackLinks(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	d.AddPrerequisites(a, b, c)
	d.ClearPrerequisites(a)
	assert.Empty(t, d.GetPrerequisites(a))
	assert.NotContains(t, d.GetDependents(b), a)
	assert.NotContains(t, d.GetDependents(c), a)
}

func TestClearPrerequisites_SkipsMissingPrerequisiteNode(t *testing.T) {
	d := newDependencyChain(false)
	tx := uuid.New()
	ghost := uuid.New()
	d.nodes[tx] = &nodeLinks{dependsOn: []uuid.UUID{ghost}, prereqOf: nil}
	d.ClearPrerequisites(tx)
	require.NotNil(t, d.nodes[tx])
	assert.Empty(t, d.nodes[tx].dependsOn)
}

func TestClearDependents_NoOpWhenUnknown(t *testing.T) {
	NewDependencyTracker().GetPostAssemblyDeps().ClearDependents(uuid.New())
}

func TestClearDependents_ClearsPrereqOfAndBackLinks(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	d.AddPrerequisites(b, a)
	d.AddPrerequisites(c, a)
	assert.Contains(t, d.GetDependents(a), b)
	assert.Contains(t, d.GetDependents(a), c)
	d.ClearDependents(a)
	assert.Empty(t, d.GetDependents(a))
	assert.NotContains(t, d.GetPrerequisites(b), a)
	assert.NotContains(t, d.GetPrerequisites(c), a)
}

func TestClearDependents_SkipsMissingDependentNode(t *testing.T) {
	d := newDependencyChain(false)
	tx := uuid.New()
	ghost := uuid.New()
	d.nodes[tx] = &nodeLinks{dependsOn: nil, prereqOf: []uuid.UUID{ghost}}
	d.ClearDependents(tx)
	require.NotNil(t, d.nodes[tx])
	assert.Empty(t, d.nodes[tx].prereqOf)
}

func TestDelete_NoOpWhenUnknown(t *testing.T) {
	NewDependencyTracker().GetPostAssemblyDeps().Delete(uuid.New())
}

func TestDelete_RemovesNodeAndUpdatesPeers(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	d.AddPrerequisites(a, b)
	d.AddPrerequisites(c, a)
	d.Delete(a)
	assert.Empty(t, d.GetPrerequisites(a))
	assert.Empty(t, d.GetDependents(a))
	assert.Empty(t, d.GetDependents(b))
	assert.NotContains(t, d.GetPrerequisites(c), a)
}

func TestDelete_SkipsMissingDependentNode(t *testing.T) {
	d := newDependencyChain(false)
	tx := uuid.New()
	ghost := uuid.New()
	d.nodes[tx] = &nodeLinks{prereqOf: []uuid.UUID{ghost}, dependsOn: nil}
	d.Delete(tx)
	_, still := d.nodes[tx]
	assert.False(t, still)
}

func TestDelete_SkipsMissingPrerequisiteNode(t *testing.T) {
	d := newDependencyChain(false)
	tx := uuid.New()
	ghost := uuid.New()
	d.nodes[tx] = &nodeLinks{dependsOn: []uuid.UUID{ghost}, prereqOf: nil}
	d.Delete(tx)
	_, still := d.nodes[tx]
	assert.False(t, still)
}

func TestGetPrerequisites_GetDependents_NilWhenUnknown(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	unknown := uuid.New()
	assert.Nil(t, d.GetPrerequisites(unknown))
	assert.Nil(t, d.GetDependents(unknown))
}

func TestGetPrerequisites_GetDependents_AfterEdges(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisites(a, b)
	assert.Equal(t, []uuid.UUID{b}, d.GetPrerequisites(a))
	assert.Equal(t, []uuid.UUID{a}, d.GetDependents(b))
}

func TestGetDependents_ReturnsCopy(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisites(a, b)
	got := d.GetDependents(b)
	require.Len(t, got, 1)
	got[0] = uuid.Nil
	assert.Equal(t, a, d.GetDependents(b)[0])
}

func TestGetPrerequisites_ReturnsCopy(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	a, b := uuid.New(), uuid.New()
	d.AddPrerequisites(a, b)
	got := d.GetPrerequisites(a)
	require.Len(t, got, 1)
	got[0] = uuid.Nil
	assert.Equal(t, b, d.GetPrerequisites(a)[0])
}

func TestChained_UnassembledDependencies(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	tx := uuid.New()
	dep1, dep2 := uuid.New(), uuid.New()
	assert.Nil(t, cd.GetUnassembledDependencies(tx))

	cd.AddUnassembledDependencies(tx, dep1)
	require.NotNil(t, cd.GetUnassembledDependencies(tx))
	assert.Contains(t, cd.GetUnassembledDependencies(tx), dep1)

	cd.AddUnassembledDependencies(tx, dep2)
	assert.Contains(t, cd.GetUnassembledDependencies(tx), dep1)
	assert.Contains(t, cd.GetUnassembledDependencies(tx), dep2)

	cd.DeleteUnassembledDependencies(tx, dep1)
	assert.NotContains(t, cd.GetUnassembledDependencies(tx), dep1)
	assert.Contains(t, cd.GetUnassembledDependencies(tx), dep2)

	cd.DeleteUnassembledDependencies(tx, dep2)
	assert.NotContains(t, cd.GetUnassembledDependencies(tx), dep2)
}

func TestChained_DeleteUnassembledDependencies_UnknownTx(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	cd.DeleteUnassembledDependencies(uuid.New(), uuid.New())
}

func TestChained_AddUnassembledDependencies_SecondTx(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	tx1, tx2 := uuid.New(), uuid.New()
	d1, d2 := uuid.New(), uuid.New()
	cd.AddUnassembledDependencies(tx1, d1)
	cd.AddUnassembledDependencies(tx2, d2)
	assert.Contains(t, cd.GetUnassembledDependencies(tx1), d1)
	assert.Contains(t, cd.GetUnassembledDependencies(tx2), d2)
	assert.Len(t, cd.GetUnassembledDependencies(tx1), 1)
}

func TestChained_HasUnassembledDependencies(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	tx := uuid.New()
	dep := uuid.New()

	assert.False(t, cd.HasUnassembledDependencies(tx))
	cd.AddUnassembledDependencies(tx, dep)
	assert.True(t, cd.HasUnassembledDependencies(tx))
	cd.DeleteUnassembledDependencies(tx, dep)
	assert.False(t, cd.HasUnassembledDependencies(tx))
}

func TestChained_GetChainedChild_UnknownParent(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	got, ok := cd.GetChainedChild(uuid.New())
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)
}

func TestChained_GetChainedChild_NilWhenChildUnsetOrZero(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	parent := uuid.New()
	cd.SetChainedChild(parent, uuid.Nil)
	got, ok := cd.GetChainedChild(parent)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)
}

func TestChained_SetGetForgetChainedChild(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	parent, child := uuid.New(), uuid.New()
	got, ok := cd.GetChainedChild(parent)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)

	cd.SetChainedChild(parent, child)
	got, ok = cd.GetChainedChild(parent)
	require.True(t, ok)
	assert.Equal(t, child, got)

	cd.ForgetChainedChild(parent)
	got, ok = cd.GetChainedChild(parent)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)
}

func TestChained_ForgetChainedChild_NoOpWhenUnknownParent(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	cd.ForgetChainedChild(uuid.New())
}

func TestChained_Delete_RemovesMetadataAndDependencyLinks(t *testing.T) {
	cd := NewDependencyTracker().GetChainedDeps()
	parent := uuid.New()
	prereq := uuid.New()
	dependent := uuid.New()
	child := uuid.New()
	unassembled := uuid.New()

	cd.AddPrerequisites(parent, prereq)
	cd.AddPrerequisites(dependent, parent)
	cd.SetChainedChild(parent, child)
	cd.AddUnassembledDependencies(parent, unassembled)

	cd.Delete(parent)

	gotChild, ok := cd.GetChainedChild(parent)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, gotChild)
	assert.Nil(t, cd.GetUnassembledDependencies(parent))
	assert.Empty(t, cd.GetPrerequisites(dependent))
	assert.Empty(t, cd.GetDependents(prereq))
}

func TestDependencyTracker_Delete_RemovesAcrossAllChains(t *testing.T) {
	dt := NewDependencyTracker().(*dependencyTracker)
	txID := uuid.New()
	preReq := uuid.New()
	postReq := uuid.New()
	chainedReq := uuid.New()
	chainedChild := uuid.New()
	unassembled := uuid.New()

	dt.preAssembly.AddPrerequisites(txID, preReq)
	dt.postAssembly.AddPrerequisites(txID, postReq)
	dt.chained.AddPrerequisites(txID, chainedReq)
	dt.chained.SetChainedChild(txID, chainedChild)
	dt.chained.AddUnassembledDependencies(txID, unassembled)

	dt.Delete(txID)

	assert.Empty(t, dt.preAssembly.GetPrerequisites(txID))
	assert.Empty(t, dt.preAssembly.GetDependents(preReq))
	assert.Empty(t, dt.postAssembly.GetPrerequisites(txID))
	assert.Empty(t, dt.postAssembly.GetDependents(postReq))
	assert.Empty(t, dt.chained.GetPrerequisites(txID))
	assert.Empty(t, dt.chained.GetDependents(chainedReq))
	gotChild, ok := dt.chained.GetChainedChild(txID)
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, gotChild)
	assert.Nil(t, dt.chained.GetUnassembledDependencies(txID))
}

func TestHasPrerequisitesAndHasDependents(t *testing.T) {
	d := NewDependencyTracker().GetPostAssemblyDeps()
	tx := uuid.New()
	prereq := uuid.New()
	unknown := uuid.New()

	assert.False(t, d.HasPrerequisites(tx))
	assert.False(t, d.HasDependents(prereq))
	assert.False(t, d.HasPrerequisites(unknown))
	assert.False(t, d.HasDependents(unknown))

	d.AddPrerequisites(tx, prereq)
	assert.True(t, d.HasPrerequisites(tx))
	assert.True(t, d.HasDependents(prereq))

	d.ClearPrerequisites(tx)
	assert.False(t, d.HasPrerequisites(tx))
	assert.False(t, d.HasDependents(prereq))
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
			d.AddPrerequisites(leaf, center)
		}()
	}
	wg.Wait()
	assert.Len(t, d.GetDependents(center), 50)
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
