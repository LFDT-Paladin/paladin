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

// Package dependencytracker implements thread-safe bidirectional dependency graphs between
// transactions (depends-on / prereq-of), organized as DependencyChains under a DependencyTracker.
package dependencytracker

import (
	"slices"
	"sync"

	"github.com/google/uuid"
)

// DependencyTracker holds three independent dependency chains: pre-assembly, post-assembly, and chained.
type DependencyTracker interface {
	GetPreassemblyDeps() DependencyChain
	GetPostAssemblyDeps() DependencyChain
	GetChainedDeps() ChainedTransactionDependencyChain
}

// DependencyChain records, for each transaction ID, which other transactions it depends on
// and which transactions depend on it. The interface (currently) has one method for adding dependencies,
// via AddPrerequisites() which updates all the pre-req and depends-on lists. There are getters for the
// individual lists, and functions to remove from just one of the lists.
type DependencyChain interface {
	AddPrerequisites(txID uuid.UUID, prereq ...uuid.UUID) // Updates both the pre-req-of and depends-on lists
	ClearPrerequisites(txID uuid.UUID)                    // Remove all depends-on entries this transaction for the transaction, updating their pre-req-of chains as well
	ClearDependents(txID uuid.UUID)                       // Remove all pre-req of entries for this transaction, updating their depends-on chains as well
	Delete(transactionID uuid.UUID)                       // Remove the TX entirely, so it has no pre-reqs or dependents and any it did have their respective lists updated
	GetPrerequisites(txID uuid.UUID) []uuid.UUID
	GetDependents(txID uuid.UUID) []uuid.UUID
}

type ChainedTransactionDependencyChain interface {
	DependencyChain
	AddUnassembledDependencies(txID uuid.UUID, unassembledDependencyIDs ...uuid.UUID)
	DeleteUnassembledDependencies(txID uuid.UUID, unassembledDependencyIDs ...uuid.UUID)
	GetUnassembledDependencies(txID uuid.UUID) map[uuid.UUID]struct{}
}

type dependencyChain struct {
	mu              sync.RWMutex
	nodes           map[uuid.UUID]*nodeLinks
	singleChainOnly bool // Specifies if the chain supports multiple pre-reqs and dependents or not. Allows for stricter runtime checks based on the chain type
}

type nodeLinks struct {
	dependsOn []uuid.UUID
	prereqOf  []uuid.UUID
}

type chainedDependencies struct {
	*dependencyChain
	unassembledDependencies map[uuid.UUID]map[uuid.UUID]struct{}
}

type dependencyTracker struct {
	preAssembly  *dependencyChain
	postAssembly *dependencyChain
	chained      *chainedDependencies
}

func newDependencyChain(singleChainOnly bool) *dependencyChain {
	return &dependencyChain{
		nodes:           make(map[uuid.UUID]*nodeLinks),
		singleChainOnly: singleChainOnly,
	}
}

func NewDependencyTracker() DependencyTracker {
	return &dependencyTracker{
		preAssembly:  newDependencyChain(true),
		postAssembly: newDependencyChain(false),
		chained: &chainedDependencies{
			dependencyChain:         newDependencyChain(true),
			unassembledDependencies: make(map[uuid.UUID]map[uuid.UUID]struct{}),
		},
	}
}

func (dt *dependencyTracker) GetPreassemblyDeps() DependencyChain {
	return dt.preAssembly
}

func (dt *dependencyTracker) GetPostAssemblyDeps() DependencyChain {
	return dt.postAssembly
}

func (dt *dependencyTracker) GetChainedDeps() ChainedTransactionDependencyChain {
	return dt.chained
}

func (cd *chainedDependencies) AddUnassembledDependencies(txID uuid.UUID, unassembledDependencyIDs ...uuid.UUID) {
	for _, unassembledDependencyID := range unassembledDependencyIDs {
		if cd.unassembledDependencies[txID] == nil {
			cd.unassembledDependencies[txID] = make(map[uuid.UUID]struct{})
		}
		cd.unassembledDependencies[txID][unassembledDependencyID] = struct{}{}
	}
}

func (cd *chainedDependencies) DeleteUnassembledDependencies(txID uuid.UUID, unassembledDependencyIDs ...uuid.UUID) {
	for _, unassembledDependencyID := range unassembledDependencyIDs {
		delete(cd.unassembledDependencies[txID], unassembledDependencyID)
	}
}

func (cd *chainedDependencies) GetUnassembledDependencies(txID uuid.UUID) map[uuid.UUID]struct{} {
	return cd.unassembledDependencies[txID]
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

// AddPrerequisites records that txID depends on each prerequisite (prerequisite IDs must
// complete before txID). Updates both sides of each edge.
func (d *dependencyChain) AddPrerequisites(txID uuid.UUID, prereq ...uuid.UUID) {
	if d.singleChainOnly {
		if len(prereq) > 1 {
			panic("singleChainOnly dependency chain does not support multiple prerequisites")
		}
	}
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

func (d *dependencyChain) ClearPrerequisites(txID uuid.UUID) {
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

func (d *dependencyChain) ClearDependents(txID uuid.UUID) {
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
func (d *dependencyChain) Delete(transactionID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
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
func (d *dependencyChain) GetDependents(txID uuid.UUID) []uuid.UUID {
	d.mu.RLock()
	defer d.mu.RUnlock()
	tx := d.nodes[txID]
	if tx == nil {
		return nil
	}
	return slices.Clone(tx.prereqOf)
}

// Return a copy of the prerequisites for the given transaction ID
func (d *dependencyChain) GetPrerequisites(txID uuid.UUID) []uuid.UUID {
	d.mu.RLock()
	defer d.mu.RUnlock()
	tx := d.nodes[txID]
	if tx == nil {
		return nil
	}
	return slices.Clone(tx.dependsOn)
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
