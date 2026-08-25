/*
 * Copyright © 2025 Kaleido, Inc.
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
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/dependencytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/statevisibilitytracker"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// The Grapher package provides 3 core functions to Paladin:
// 1. It allows transactions to link to each other in a bi-directional dependency graph, based entirely on post-assembly outputs. This ensures
//    base-ledger state changes are correctly ordered, crucial to transaction success.
// 2. It records ahead-of-chain state changes, such as inputs being locked, to allow successful ahead-of-chain assembly for new transactions.
// 3. It provides an interface to export the current ahead-of-chain state changes to give to originators to base new assembly requests on.

// An instance of the grapher is owned by the coordinator for a given sequencer. Transactions can query the grapher in thread-safe manner to
// understand their relationships to other transactions. For example:
//   - Did it create a state that another TX now depends on?
//   - Did it consume/lock a state that another TX created?

// The grapher is updated when base-ledger transactions are successful or revert. For example:
//   - A base-ledger revert has occurred so locked states should be unlocked as they are available again for re-assembly
//   - A base-ledger confirmation has occurred so consumed states should be removed once the block height tolerance window passes

// Lock lifecycle:
//   - Transaction-owned (Transaction != nil): managed by the transaction state machine via ForgetTransactionAndLocks and ForgetTransaction.
//   - No-transaction locks (Transaction == nil, ConfirmedAtBlock set): created when ForgetTransaction clears the
//     transaction reference, or imported directly via ImportStatesAndLocks on coordinator handover.
//     Cleaned up by ForgetLocks when currentBlockHeight >= ConfirmedAtBlock + blockHeightTolerance.
type Grapher interface {
	// AddMinter records that a set of states has been minted by the specified transaction.
	// Private state visibility (AllowedNodes) is managed separately by the statevisibilitytracker package.
	AddMinter(ctx context.Context, states []*prototk.EndorsableState, txID uuid.UUID) error
	// ExportStatesAndLocks returns the current ahead-of-chain state for a specific node.
	// OutputStates are filtered via the statevisibilitytracker store — only states where node appears in
	// AllowedNodes are included. All locks are returned unfiltered — lock data (state IDs, types,
	// block numbers) is on-chain metadata and does not need privacy protection.
	ExportStatesAndLocks(ctx context.Context, node string) (*prototk.StateSnapshot, error)
	// ForgetTransactionAndLocks fully removes a transaction and all its locks. Used for failure/reset paths (revert, repool, eviction).
	// No-op if the transaction is not known (e.g. already confirmed).
	ForgetTransactionAndLocks(ctx context.Context, transactionID uuid.UUID)
	// ForgetTransaction removes the transaction from the grapher's dependency tracking and minter index,
	// and stamps confirmedAtBlock on its locks, clearing the transaction reference.
	ForgetTransaction(ctx context.Context, transactionID uuid.UUID, confirmedAtBlock uint64)
	// ForgetLocks removes locks with no transaction whose block height tolerance window has passed,
	// meaning the persisted state records should have caught up and the lock is no longer needed.
	// Should be called on every NewBlock event.
	ForgetLocks(ctx context.Context, currentBlockHeight uint64)
	// ImportStatesAndLocks imports states and locks from a previous coordinator on handover.
	// OutputStates carry private state data filtered for this node; locks are imported to maintain
	// the ahead-of-chain view. Existing entries are never overwritten — the current coordinator's
	// own knowledge always takes precedence.
	ImportStatesAndLocks(ctx context.Context, snapshot *prototk.StateSnapshot)
	GetDependencies(ctx context.Context, transactionID uuid.UUID) []uuid.UUID
	GetDependents(ctx context.Context, transactionID uuid.UUID) []uuid.UUID
	LockMintsOnCreate(ctx context.Context, states []*prototk.EndorsableState, transactionID uuid.UUID)
	LockMintsOnReadAndSpend(ctx context.Context, readStates []*prototk.EndorsableState, spendStates []*prototk.EndorsableState, transactionID uuid.UUID)
}

type grapher struct {
	mu sync.RWMutex

	blockHeightTolerance   uint64
	stateVisibilityTracker statevisibilitytracker.StateVisibilityStore

	dependencyChain          dependencytracker.DependencyChain
	transactionByID          map[uuid.UUID]*grapherTX
	transactionByOutputState map[string]*grapherTX
	outputStatesByMinter     map[uuid.UUID][]string     // reverse lookup by transactions ID for building dependency chains and transaction-driven cleanup
	createLocksByStateID     map[string]*prototk.SnapshotStateLock      // create locks keyed by state ID (at most one per state, from its minter)
	spendLocksByStateID      map[string]*prototk.SnapshotStateLock      // spend locks keyed by state ID (at most one per state)
	readLocksByStateID       map[string]*prototk.SnapshotStateLock      // read locks keyed by state ID (at most one per state)
	locksByTransaction       map[uuid.UUID][]*prototk.SnapshotStateLock // secondary index for O(1) transaction-driven cleanup
}

type grapherTX struct {
	ID uuid.UUID
}

func NewGrapher(dependencyTracker dependencytracker.DependencyTracker, stateVisibilityTracker statevisibilitytracker.StateVisibilityStore, blockHeightTolerance uint64) Grapher {
	return &grapher{
		blockHeightTolerance:     blockHeightTolerance,
		stateVisibilityTracker:   stateVisibilityTracker,
		dependencyChain:          dependencyTracker.GetPostAssemblyDeps(),
		transactionByOutputState: make(map[string]*grapherTX),
		transactionByID:          make(map[uuid.UUID]*grapherTX),
		outputStatesByMinter:     make(map[uuid.UUID][]string),
		createLocksByStateID:     make(map[string]*prototk.SnapshotStateLock),
		spendLocksByStateID:      make(map[string]*prototk.SnapshotStateLock),
		readLocksByStateID:       make(map[string]*prototk.SnapshotStateLock),
		locksByTransaction:       make(map[uuid.UUID][]*prototk.SnapshotStateLock),
	}
}

// Record (idempotently) the existence of a transaction that consumes at least one state.
// Caller must hold g.mu write lock.
func (g *grapher) addConsumer(transactionID uuid.UUID) {
	if _, ok := g.transactionByID[transactionID]; !ok {
		g.transactionByID[transactionID] = &grapherTX{
			ID: transactionID,
		}
	}
}

// Record that a set of states has been minted by the specified transaction. Adds the transaction to the grapher if it doesn't exist already.
// Private state visibility is managed by the statevisibilitytracker store.
func (g *grapher) AddMinter(ctx context.Context, states []*prototk.EndorsableState, transactionID uuid.UUID) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.transactionByID[transactionID] = &grapherTX{
		ID: transactionID,
	}
	for _, state := range states {
		if txn, ok := g.transactionByOutputState[state.GetId()]; ok {
			return i18n.NewError(ctx, msgs.MsgSequencerGrapherAddMinterAlreadyExistsError, transactionID.String(), state.GetId(), txn.ID.String())
		}
		g.transactionByOutputState[state.GetId()] = g.transactionByID[transactionID]
		g.outputStatesByMinter[transactionID] = append(g.outputStatesByMinter[transactionID], state.GetId())
	}

	return nil
}

// ForgetTransactionAndLocks fully removes a transaction and all its locks from the grapher.
// Used for failure/reset paths: revert, repool, eviction.
// No-op if the transaction is not known (e.g. already confirmed).
func (g *grapher) ForgetTransactionAndLocks(ctx context.Context, transactionID uuid.UUID) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.transactionByID[transactionID]; !ok {
		return
	}

	g.dependencyChain.Delete(ctx, transactionID)
	g.forgetTxMints(transactionID)
	g.forgetLocks(transactionID)
	delete(g.transactionByID, transactionID)
}

// ForgetTransaction removes the transaction from all in-flight tracking (dependency chain,
// transactionByOutputState, outputStatesByMinter, transactionByID), stamps confirmedAtBlock on
// its locks, and clears the transaction reference on those locks.
// The statevisibilitytracker store is NOT cleared — private state data persists until the lock expires in
// ForgetLocks, so coordinator handover heartbeats include it within the block tolerance window.
// No-op if the transaction is not known.
func (g *grapher) ForgetTransaction(ctx context.Context, transactionID uuid.UUID, confirmedAtBlock uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.transactionByID[transactionID]; !ok {
		return
	}

	g.dependencyChain.Delete(ctx, transactionID)
	g.forgetTxMints(transactionID)

	// Stamp confirmedAtBlock on the transaction's locks and clear the transaction reference.
	// Each stateLock object is shared by pointer between locksByTransaction and the type-segregated
	// maps (createLocksByStateID / spendLocksByStateID / readLocksByStateID). Mutating the fields
	// here propagates to whichever of those maps holds the same pointer, so after this loop every
	// type-segregated entry for this transaction reflects the confirmed state — even though
	// locksByTransaction is deleted immediately below. Each lock carries the confirmedAtBlock of
	// its own transaction, independently of any other lock on the same state.
	txLocks := g.locksByTransaction[transactionID]
	for _, lock := range txLocks {
		lock.Transaction = nil
		lock.ConfirmedAtBlock = &confirmedAtBlock
	}
	delete(g.locksByTransaction, transactionID)
	delete(g.transactionByID, transactionID)

	log.L(ctx).Debugf("ForgetTransaction: confirmed %d locks for tx %s at block %d", len(txLocks), transactionID, confirmedAtBlock)
}

// ForgetLocks removes locks with no transaction whose block height tolerance window has passed,
// meaning the persisted state should have caught up and the lock is no longer needed.
// Removing a create lock cascades to delete the corresponding private state data from the
// statevisibilitytracker store — the create lock is the source of truth for how long private
// state data is held.
// Should be called on every NewBlock event.
func (g *grapher) ForgetLocks(ctx context.Context, currentBlockHeight uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for stateID, lock := range g.createLocksByStateID {
		if lock.Transaction == nil && lock.ConfirmedAtBlock != nil {
			if currentBlockHeight >= *lock.ConfirmedAtBlock+g.blockHeightTolerance {
				log.L(ctx).Debugf("ForgetLocks: removing create lock on state %s (confirmedAtBlock=%d, tolerance=%d, currentBlock=%d)",
					stateID, *lock.ConfirmedAtBlock, g.blockHeightTolerance, currentBlockHeight)
				delete(g.createLocksByStateID, stateID)
				g.stateVisibilityTracker.Delete(stateID)
			}
		}
	}
	for stateID, lock := range g.spendLocksByStateID {
		if lock.Transaction == nil && lock.ConfirmedAtBlock != nil {
			if currentBlockHeight >= *lock.ConfirmedAtBlock+g.blockHeightTolerance {
				log.L(ctx).Debugf("ForgetLocks: removing spend lock on state %s (confirmedAtBlock=%d, tolerance=%d, currentBlock=%d)",
					stateID, *lock.ConfirmedAtBlock, g.blockHeightTolerance, currentBlockHeight)
				delete(g.spendLocksByStateID, stateID)
			}
		}
	}
	for stateID, lock := range g.readLocksByStateID {
		if lock.Transaction == nil && lock.ConfirmedAtBlock != nil {
			if currentBlockHeight >= *lock.ConfirmedAtBlock+g.blockHeightTolerance {
				log.L(ctx).Debugf("ForgetLocks: removing read lock on state %s (confirmedAtBlock=%d, tolerance=%d, currentBlock=%d)",
					stateID, *lock.ConfirmedAtBlock, g.blockHeightTolerance, currentBlockHeight)
				delete(g.readLocksByStateID, stateID)
			}
		}
	}
}

// ImportStatesAndLocks imports confirmed locks and their associated private state data
// from a previous coordinator on handover. This is the for the golden path handover case where a new
// coordinator takes over only once the previous has flushed its transactions through to confirmation.
// Handing over the locks and private state data allows the new coordinator to maintain block height tolerance.
func (g *grapher) ImportStatesAndLocks(ctx context.Context, snapshot *prototk.StateSnapshot) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Build a map of confirmed locks only. In-flight locks are skipped.
	confirmedLockByStateID := make(map[string]*prototk.SnapshotStateLock, len(snapshot.GetLocks()))
	for _, lock := range snapshot.GetLocks() {
		stateID := lock.GetStateId()
		if lock.Transaction != nil {
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping in-flight lock on state %s — in-flight transactions are re-processed by the new coordinator", stateID)
			continue
		}
		if lock.ConfirmedAtBlock == nil {
			log.L(ctx).Warnf("ImportStatesAndLocks: skipping lock on state %s — no transaction and no ConfirmedAtBlock", stateID)
			continue
		}
		confirmedLockByStateID[stateID] = lock
	}

	// Import output states that have a corresponding confirmed lock.
	for _, state := range snapshot.GetStates() {
		stateID := state.GetState().GetId()
		if _, ok := confirmedLockByStateID[stateID]; !ok {
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping output state %s — no corresponding confirmed lock found", stateID)
			continue
		}
		if g.stateVisibilityTracker.ImportIfAbsent(stateID, state) {
			log.L(ctx).Debugf("ImportStatesAndLocks: imported output state %s", stateID)
		} else {
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping output state %s — existing entry takes precedence", stateID)
		}
	}

	// Import confirmed locks, preserving existing entries.
	// Route each lock into the appropriate type-segregated map.
	for stateID, lock := range confirmedLockByStateID {
		var targetMap map[string]*prototk.SnapshotStateLock
		switch lock.GetType() {
		case prototk.SnapshotStateLock_CREATE:
			targetMap = g.createLocksByStateID
		case prototk.SnapshotStateLock_SPEND:
			targetMap = g.spendLocksByStateID
		default:
			targetMap = g.readLocksByStateID
		}
		if _, exists := targetMap[stateID]; exists {
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping lock on state %s — existing lock takes precedence", stateID)
			continue
		}
		targetMap[stateID] = lock
		log.L(ctx).Debugf("ImportStatesAndLocks: imported confirmed lock on state %s", stateID)
	}
}

// forgetTxMints removes a transaction's in-flight tracking entries (transactionByOutputState
// and outputStatesByMinter) without touching the statevisibilitytracker store.
// Called from both the success path (ForgetTransaction) and the failure path (ForgetTransactionAndLocks) so
// that the transaction-indexed maps stay in sync with transactionByID.
// Caller must hold g.mu write lock.
func (g *grapher) forgetTxMints(transactionID uuid.UUID) {
	if stateIDs, ok := g.outputStatesByMinter[transactionID]; ok {
		for _, stateID := range stateIDs {
			delete(g.transactionByOutputState, stateID)
		}
		delete(g.outputStatesByMinter, transactionID)
	}
}

// forgetLocks removes all locks owned by a transaction from all lock indexes.
// Removing a create lock cascades to delete the corresponding private state data
// from the statevisibilitytracker store — the create lock governs the state's lifetime in the grapher.
// Caller must hold g.mu write lock.
func (g *grapher) forgetLocks(transactionID uuid.UUID) {
	for _, lock := range g.locksByTransaction[transactionID] {
		stateID := lock.GetStateId()
		switch lock.GetType() {
		case prototk.SnapshotStateLock_CREATE:
			delete(g.createLocksByStateID, stateID)
			g.stateVisibilityTracker.Delete(stateID)
		case prototk.SnapshotStateLock_SPEND:
			delete(g.spendLocksByStateID, stateID)
		default:
			delete(g.readLocksByStateID, stateID)
		}
	}
	delete(g.locksByTransaction, transactionID)
}

// Get transactions we are dependent on
func (g *grapher) GetDependencies(ctx context.Context, transactionID uuid.UUID) []uuid.UUID {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if _, ok := g.transactionByID[transactionID]; ok {
		return g.dependencyChain.GetPrerequisites(ctx, transactionID)
	}
	return nil
}

// Get transactions we are a pre-req of
func (g *grapher) GetDependents(ctx context.Context, transactionID uuid.UUID) []uuid.UUID {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if _, ok := g.transactionByID[transactionID]; ok {
		return g.dependencyChain.GetDependents(ctx, transactionID)
	}
	return nil
}

// Caller must hold write lock.
func (g *grapher) LockMintsOnCreate(ctx context.Context, states []*prototk.EndorsableState, transactionID uuid.UUID) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.addConsumer(transactionID)
	txnStr := transactionID.String()
	for _, state := range states {
		stateID := state.GetId()
		log.L(ctx).Debugf("LockMintsOnCreate: creating lock for state %s", stateID)
		lock := &prototk.SnapshotStateLock{StateId: stateID, Transaction: &txnStr, Type: prototk.SnapshotStateLock_CREATE}
		g.createLocksByStateID[stateID] = lock
		g.locksByTransaction[transactionID] = append(g.locksByTransaction[transactionID], lock)
	}
}

func (g *grapher) LockMintsOnReadAndSpend(ctx context.Context, readStates []*prototk.EndorsableState, spendStates []*prototk.EndorsableState, transactionID uuid.UUID) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.addConsumer(transactionID)
	txnStr := transactionID.String()
	for _, state := range readStates {
		stateID := state.GetId()
		log.L(ctx).Debugf("LockMintsOnReadAndSpend: TX %s taking read lock on state %s", transactionID.String(), stateID)
		lock := &prototk.SnapshotStateLock{StateId: stateID, Transaction: &txnStr, Type: prototk.SnapshotStateLock_READ}
		g.readLocksByStateID[stateID] = lock
		g.locksByTransaction[transactionID] = append(g.locksByTransaction[transactionID], lock)
		if mintedBy := g.transactionByOutputState[stateID]; mintedBy != nil {
			g.dependencyChain.AddPrerequisites(ctx, transactionID, mintedBy.ID)
		}
	}

	for _, state := range spendStates {
		stateID := state.GetId()
		log.L(ctx).Debugf("LockMintsOnReadAndSpend: TX %s taking spend lock on state %s", transactionID.String(), stateID)
		lock := &prototk.SnapshotStateLock{StateId: stateID, Transaction: &txnStr, Type: prototk.SnapshotStateLock_SPEND}
		g.spendLocksByStateID[stateID] = lock
		g.locksByTransaction[transactionID] = append(g.locksByTransaction[transactionID], lock)
		if mintedBy := g.transactionByOutputState[stateID]; mintedBy != nil {
			g.dependencyChain.AddPrerequisites(ctx, transactionID, mintedBy.ID)
		}
	}
}

func (g *grapher) ExportStatesAndLocks(ctx context.Context, node string) (*prototk.StateSnapshot, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := &prototk.StateSnapshot{}

	// Exclude private state data for states that also have a spend lock — those states are
	// already consumed ahead-of-chain by another transaction. Assemblers and incoming coordinators
	// have no use for the private data since the state cannot be respent. States are already held as
	// proto by the visibility tracker, so no per-state conversion is needed here.
	allStates := g.stateVisibilityTracker.GetForNode(node)
	result.States = make([]*prototk.SnapshotState, 0, len(allStates))
	for _, state := range allStates {
		if _, hasSpendLock := g.spendLocksByStateID[state.GetState().GetId()]; !hasSpendLock {
			result.States = append(result.States, state)
		}
	}

	// All locks are returned unfiltered — lock data is on-chain metadata and needs no privacy protection.
	result.Locks = make([]*prototk.SnapshotStateLock, 0, len(g.createLocksByStateID)+len(g.spendLocksByStateID)+len(g.readLocksByStateID))
	for _, lock := range g.createLocksByStateID {
		result.Locks = append(result.Locks, lock)
	}
	for _, lock := range g.spendLocksByStateID {
		result.Locks = append(result.Locks, lock)
	}
	for _, lock := range g.readLocksByStateID {
		result.Locks = append(result.Locks, lock)
	}
	log.L(ctx).Debugf("ExportStatesAndLocks: %d output states (filtered from %d), %d locks (node=%q)", len(result.States), len(allStates), len(result.Locks), node)
	return result, nil
}
