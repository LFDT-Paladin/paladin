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

// The dependency tracker is a package that is designed to provide thread-safe tracking of inter-TX dependencies.
// Paladin has several mechanisms for tracking transactions that might depend on, or be a pre-requisite, of another:
//   - pre-assembly dependencies. These are determined the first time transactions are received, to maintain FIFO order
//     of delegated tranasctions
//   - post-assembly dependencies. These are determine by the grapher package. The grapher updates the PostAssembly chain in
//     the dependency tracker directly, based on calls to the grapher to mint/lock/clear states.
//   - chained dependencies. These are determined when transaction dispatches result in a new chained transactions, where each of
//     the chained transactions has a dependency link with the chained transactions of previous parent transactions.

// The dependency tracker also implements a discrete interface DependencyChain which each of the above types of dependency is
// represented by. Some dependencies are graph-like (e.g. TX3 pre-reqs TX1 and TX2). Some are linked-lists, and the DependencyChain
// constructor allows the caller to specify this, to allow for stricter checks at chain-manipulation time. The DependencyChain
// is thread-safe, so a call can retrieve the singleton-instance of the correct chain, then update it in a thread-safe manner.
package dependencytracker

import (
	"context"
	"slices"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/google/uuid"
)

// DependencyTracker holds three independent dependency chains: pre-assembly, post-assembly, and chained.
type DependencyTracker interface {
	GetPreassemblyDeps() SingleDependencyChain
	GetPostAssemblyDeps() DependencyChain
	GetChainedDeps() ChainedTransactionDependencyChain
	Delete(ctx context.Context, txID uuid.UUID)
}

// DependencyChain records, for each transaction ID, which other transactions it depends on
// and which transactions depend on it. The interface (currently) has one method for adding dependencies,
// via AddPrerequisites() which updates all the pre-req and depends-on lists. There are getters for the
// individual lists, and functions to remove from just one of the lists.
type DependencyChain interface {
	AddPrerequisites(ctx context.Context, txID uuid.UUID, prereq ...uuid.UUID) // Updates both the pre-req-of and depends-on lists
	ClearPrerequisites(ctx context.Context, txID uuid.UUID)                    // Remove all depends-on entries for this transaction for the transaction, updating their pre-req-of chains as well
	ClearDependents(ctx context.Context, txID uuid.UUID)                       // Remove all pre-req of entries for this transaction, updating their depends-on chains as well
	Delete(ctx context.Context, txID uuid.UUID)                                // Remove the TX entirely, so it has no pre-reqs or dependents and any it did have their respective lists updated
	GetPrerequisites(ctx context.Context, txID uuid.UUID) []uuid.UUID
	GetDependents(ctx context.Context, txID uuid.UUID) []uuid.UUID
	HasPrerequisites(ctx context.Context, txID uuid.UUID) bool
	HasDependents(ctx context.Context, txID uuid.UUID) bool
}

type SingleDependencyChain interface {
	ClearPrerequisite(ctx context.Context, txID uuid.UUID) // Remove all depends-on entries for this transaction for the transaction, updating their pre-req-of chains as well
	ClearDependent(ctx context.Context, txID uuid.UUID)    // Remove all pre-req of entries for this transaction, updating their depends-on chains as well
	Delete(ctx context.Context, txID uuid.UUID)            // Remove the TX entirely, so it has no pre-reqs or dependents and any it did have their respective lists updated
	GetPrerequisite(ctx context.Context, txID uuid.UUID) (uuid.UUID, bool)
	GetDependent(ctx context.Context, txID uuid.UUID) (uuid.UUID, bool)
	HasPrerequisite(ctx context.Context, txID uuid.UUID) bool
	HasDependent(ctx context.Context, txID uuid.UUID) bool
	AddPrerequisite(ctx context.Context, txID uuid.UUID, prereq uuid.UUID) // Updates both the pre-req-of and depends-on lists
}

type ChainedTransactionDependencyChain interface {
	DependencyChain
	AddUnassembledDependencies(ctx context.Context, txID uuid.UUID, unassembledDependencyIDs ...uuid.UUID)
	DeleteUnassembledDependencies(ctx context.Context, txID uuid.UUID, unassembledDependencyIDs ...uuid.UUID)
	GetUnassembledDependencies(ctx context.Context, txID uuid.UUID) map[uuid.UUID]struct{}
	HasUnassembledDependencies(ctx context.Context, txID uuid.UUID) bool
	SetChainedChild(ctx context.Context, parentID uuid.UUID, childID uuid.UUID)
	GetChainedChild(ctx context.Context, parentID uuid.UUID) (uuid.UUID, bool)
	ForgetChainedChild(ctx context.Context, parentID uuid.UUID)
}

type dependencyChain struct {
	mu    sync.RWMutex
	nodes map[uuid.UUID]*nodeLinks
}

type singleDependencyChain struct {
	*dependencyChain
}

type nodeLinks struct {
	dependsOn []uuid.UUID
	prereqOf  []uuid.UUID
}

type chainedDependencies struct {
	*dependencyChain
	unassembledDependencies map[uuid.UUID]map[uuid.UUID]struct{}
	children                map[uuid.UUID]uuid.UUID
}

type dependencyTracker struct {
	preAssembly  *singleDependencyChain
	postAssembly *dependencyChain
	chained      *chainedDependencies
}

func newDependencyChain() *dependencyChain {
	return &dependencyChain{
		nodes: make(map[uuid.UUID]*nodeLinks),
	}
}

func newSingleDependencyChain() *singleDependencyChain {
	return &singleDependencyChain{
		dependencyChain: newDependencyChain(),
	}
}

func NewDependencyTracker() DependencyTracker {
	return &dependencyTracker{
		preAssembly:  newSingleDependencyChain(),
		postAssembly: newDependencyChain(),
		chained: &chainedDependencies{
			dependencyChain:         newDependencyChain(),
			unassembledDependencies: make(map[uuid.UUID]map[uuid.UUID]struct{}),
			children:                make(map[uuid.UUID]uuid.UUID),
		},
	}
}

func (dt *dependencyTracker) GetPreassemblyDeps() SingleDependencyChain {
	return dt.preAssembly
}

func (dt *dependencyTracker) GetPostAssemblyDeps() DependencyChain {
	return dt.postAssembly
}

func (dt *dependencyTracker) GetChainedDeps() ChainedTransactionDependencyChain {
	return dt.chained
}

func (dt *dependencyTracker) Delete(ctx context.Context, txID uuid.UUID) {
	dt.chained.Delete(ctx, txID)
	dt.postAssembly.Delete(ctx, txID)
	dt.preAssembly.Delete(ctx, txID)
}

func (cd *chainedDependencies) AddUnassembledDependencies(_ context.Context, txID uuid.UUID, unassembledDependencyIDs ...uuid.UUID) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	for _, unassembledDependencyID := range unassembledDependencyIDs {
		if cd.unassembledDependencies[txID] == nil {
			cd.unassembledDependencies[txID] = make(map[uuid.UUID]struct{})
		}
		cd.unassembledDependencies[txID][unassembledDependencyID] = struct{}{}
	}
}

func (cd *chainedDependencies) DeleteUnassembledDependencies(_ context.Context, txID uuid.UUID, unassembledDependencyIDs ...uuid.UUID) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	for _, unassembledDependencyID := range unassembledDependencyIDs {
		delete(cd.unassembledDependencies[txID], unassembledDependencyID)
	}
}

func (cd *chainedDependencies) GetUnassembledDependencies(_ context.Context, txID uuid.UUID) map[uuid.UUID]struct{} {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	m := cd.unassembledDependencies[txID]
	if m == nil {
		return nil
	}
	out := make(map[uuid.UUID]struct{}, len(m))
	for k := range m {
		out[k] = struct{}{}
	}
	return out
}

func (cd *chainedDependencies) HasUnassembledDependencies(_ context.Context, txID uuid.UUID) bool {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	return len(cd.unassembledDependencies[txID]) > 0
}

// Functions that manage the (optional) child node for a given chained transaction
func (cd *chainedDependencies) SetChainedChild(_ context.Context, parentID uuid.UUID, childID uuid.UUID) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.children[parentID] = childID
}

func (cd *chainedDependencies) GetChainedChild(_ context.Context, parentID uuid.UUID) (uuid.UUID, bool) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	if childID, ok := cd.children[parentID]; ok && childID != uuid.Nil {
		return childID, true
	}
	return uuid.Nil, false
}

func (cd *chainedDependencies) ForgetChainedChild(_ context.Context, parentID uuid.UUID) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	delete(cd.children, parentID)
}

func (cd *chainedDependencies) Delete(_ context.Context, id uuid.UUID) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	delete(cd.children, id)
	delete(cd.unassembledDependencies, id)
	cd.delete(id)
}

func (d *dependencyChain) ensure(id uuid.UUID) *nodeLinks {
	n, ok := d.nodes[id]
	if !ok {
		n = &nodeLinks{
			dependsOn: make([]uuid.UUID, 0),
			prereqOf:  make([]uuid.UUID, 0),
		}
		d.nodes[id] = n
	}
	return n
}

// AddPrerequisite records that txID depends on prerequisite (single prerequisite only).
// Existing links for txID are replaced to preserve pre-assembly single-chain semantics.
func (d *singleDependencyChain) AddPrerequisite(ctx context.Context, txID uuid.UUID, prereq uuid.UUID) {
	if prereq == txID {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	tx := d.ensure(txID)
	prereqNode := d.ensure(prereq)
	// TODO: this mirrors the behaviour of preassembly dependencies before this new grapher
	// where a new dependency would overwrite the old one, if it existed. This doesn't quite
	// feel right as in principle why should there be an old one? I'm choosing to preserve
	// the old tested behaviour until we have time to think about this more.
	if len(tx.dependsOn) > 0 && tx.dependsOn[0] != prereq {
		log.L(ctx).Warnf("overwriting existing single prerequisite for TX %s from %s to %s", txID, tx.dependsOn[0], prereq)
	}
	if len(prereqNode.prereqOf) > 0 && prereqNode.prereqOf[0] != txID {
		log.L(ctx).Warnf("overwriting existing singledependent for TX %s from %s to %s", prereq, prereqNode.prereqOf[0], txID)
	}
	tx.dependsOn = []uuid.UUID{prereq}
	prereqNode.prereqOf = []uuid.UUID{txID}
}

func (d *singleDependencyChain) ClearPrerequisite(ctx context.Context, txID uuid.UUID) {
	d.ClearPrerequisites(ctx, txID)
}

func (d *singleDependencyChain) ClearDependent(ctx context.Context, txID uuid.UUID) {
	d.ClearDependents(ctx, txID)
}

func (d *singleDependencyChain) GetPrerequisite(ctx context.Context, txID uuid.UUID) (uuid.UUID, bool) {
	prereqs := d.GetPrerequisites(ctx, txID)
	if len(prereqs) == 0 {
		return uuid.Nil, false
	}
	return prereqs[0], true
}

func (d *singleDependencyChain) GetDependent(ctx context.Context, txID uuid.UUID) (uuid.UUID, bool) {
	dependents := d.GetDependents(ctx, txID)
	if len(dependents) == 0 {
		return uuid.Nil, false
	}
	return dependents[0], true
}

func (d *singleDependencyChain) HasPrerequisite(ctx context.Context, txID uuid.UUID) bool {
	return d.HasPrerequisites(ctx, txID)
}

func (d *singleDependencyChain) HasDependent(ctx context.Context, txID uuid.UUID) bool {
	return d.HasDependents(ctx, txID)
}

// AddPrerequisites records that txID depends on each prerequisite (prerequisite IDs must
// complete before txID). Updates both sides of each edge.
func (d *dependencyChain) AddPrerequisites(_ context.Context, txID uuid.UUID, prereq ...uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx := d.ensure(txID)
	for _, preReq := range prereq {
		if preReq == txID {
			continue
		}
		prereqNode := d.ensure(preReq)
		tx.dependsOn = appendUnique(tx.dependsOn, preReq)
		prereqNode.prereqOf = appendUnique(prereqNode.prereqOf, txID)
	}
}

func (d *dependencyChain) ClearPrerequisites(_ context.Context, txID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx := d.nodes[txID]
	if tx == nil {
		return
	}
	for _, prereqID := range tx.dependsOn {
		if prereqNode, ok := d.nodes[prereqID]; ok {
			prereqNode.prereqOf = removeUUID(prereqNode.prereqOf, txID)
		}
	}
	// Reset the depends-on list
	tx.dependsOn = make([]uuid.UUID, 0)
}

func (d *dependencyChain) ClearDependents(_ context.Context, txID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx := d.nodes[txID]
	if tx == nil {
		return
	}

	for _, dependentID := range tx.prereqOf {
		if depNode, ok := d.nodes[dependentID]; ok {
			depNode.dependsOn = removeUUID(depNode.dependsOn, txID)
		}
	}
	// Reset the pre-req-of list
	tx.prereqOf = make([]uuid.UUID, 0)
}

// Delete removes all bidirectional links for the transaction and deletes its node from the chain.
func (d *dependencyChain) Delete(_ context.Context, transactionID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.delete(transactionID)
}

func (d *dependencyChain) delete(transactionID uuid.UUID) {
	tx := d.nodes[transactionID]
	if tx == nil {
		return
	}

	for _, dependentID := range tx.prereqOf {
		if depNode, ok := d.nodes[dependentID]; ok {
			depNode.dependsOn = removeUUID(depNode.dependsOn, transactionID)
		}
	}
	for _, prereqID := range tx.dependsOn {
		if prereqNode, ok := d.nodes[prereqID]; ok {
			prereqNode.prereqOf = removeUUID(prereqNode.prereqOf, transactionID)
		}
	}
	delete(d.nodes, transactionID)
}

// Return a copy of the dependents for the given transaction ID
func (d *dependencyChain) GetDependents(_ context.Context, txID uuid.UUID) []uuid.UUID {
	d.mu.RLock()
	defer d.mu.RUnlock()
	tx := d.nodes[txID]
	if tx == nil {
		return nil
	}
	return slices.Clone(tx.prereqOf)
}

// Return a copy of the prerequisites for the given transaction ID
func (d *dependencyChain) GetPrerequisites(_ context.Context, txID uuid.UUID) []uuid.UUID {
	d.mu.RLock()
	defer d.mu.RUnlock()
	tx := d.nodes[txID]
	if tx == nil {
		return nil
	}
	return slices.Clone(tx.dependsOn)
}

func (d *dependencyChain) HasDependents(_ context.Context, txID uuid.UUID) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	tx := d.nodes[txID]
	if tx == nil {
		return false
	}
	return len(tx.prereqOf) > 0
}

func (d *dependencyChain) HasPrerequisites(_ context.Context, txID uuid.UUID) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	tx := d.nodes[txID]
	if tx == nil {
		return false
	}
	return len(tx.dependsOn) > 0
}

func appendUnique(ids []uuid.UUID, add uuid.UUID) []uuid.UUID {
	for _, id := range ids {
		if id == add {
			return ids
		}
	}
	return append(ids, add)
}

func removeUUID(ids []uuid.UUID, target uuid.UUID) []uuid.UUID {
	filtered := ids[:0]
	for _, id := range ids {
		if id != target {
			filtered = append(filtered, id)
		}
	}
	return filtered
}
