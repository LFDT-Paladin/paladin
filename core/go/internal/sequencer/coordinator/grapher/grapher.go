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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// The Grapher package provides 3 core functions to Paladin:
// 1. It allows transactions to link to each other in a bi-directional dependency graph, based entirely on post-assembly outputs and reads. This ensures
//    base-ledger state changes are correctly ordered, crucial to transaction success.
// 2. It records ahead-of-chain state changes, such as inputs being locked, to allow successful ahead-of-chain assembly for new transactions.
// 3. It provides interfaces onto the current ahead-of-chain state changes: query evaluation for assembling originators
//    (SnapshotView) and export for coordinator handover (ExportStatesAndLocks).

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
//     Cleaned up by ForgetConfirmedLocks when currentBlockHeight >= ConfirmedAtBlock + blockHeightTolerance.
type Grapher interface {
	// AddMinter records that a set of states has been minted by the specified transaction.
	// Private state visibility (AllowedNodes) is managed separately by the statevisibilitytracker package.
	AddMinter(ctx context.Context, states []*prototk.EndorsableState, txID uuid.UUID) error
	// ExportStatesAndLocks returns the current ahead-of-chain state for a specific node.
	// OutputStates are filtered via the statevisibilitytracker store — only states where node appears in
	// AllowedNodes are included. All locks are returned unfiltered — lock data (state IDs, types,
	// block numbers) is on-chain metadata and does not need privacy protection.
	ExportStatesAndLocks(ctx context.Context, node string) (*prototk.StateSnapshot, error)
	// SnapshotView returns the state view served to an assembling node: the states available to the node, the IDs
	// of the spend-locked states whose private data it may hold and that it must exclude from its own store, and the
	// first block whose available states the view supplies. acceptsFromBlock says whether the node restricts its own
	// store to the states confirmed before that block. Without optimistic spend, or when the node does not accept a
	// from block, the spent IDs cover every spend-locked state the node may hold.
	SnapshotView(ctx context.Context, node string, acceptsFromBlock bool) (candidates []*prototk.SnapshotState, spentStateIDs []pldtypes.HexBytes, fromBlock int64)
	// ForgetTransactionAndLocks fully removes a transaction and all its locks. Used for failure/reset paths (revert, repool, eviction).
	// No-op if the transaction is not known (e.g. already confirmed).
	ForgetTransactionAndLocks(ctx context.Context, transactionID uuid.UUID)
	// ForgetTransaction removes the transaction from the grapher's dependency tracking and minter index,
	// and stamps confirmedAtBlock on its locks, clearing the transaction reference.
	ForgetTransaction(ctx context.Context, transactionID uuid.UUID, confirmedAtBlock uint64)
	// ForgetConfirmedLocks removes locks with no transaction whose block height tolerance window has passed,
	// meaning the persisted state records should have caught up and the lock is no longer needed.
	// Should be called on every NewBlock event.
	ForgetConfirmedLocks(ctx context.Context, currentBlockHeight uint64)
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
	allowOptimisticSpend   bool
	stateVisibilityTracker statevisibilitytracker.StateVisibilityStore

	dependencyChain                dependencytracker.DependencyChain
	readDeps                       dependencytracker.DependencyChain
	transactionByID                map[uuid.UUID]*grapherTX
	transactionByOutputState       map[string]*grapherTX
	outputStatesByMinter           map[uuid.UUID][]string                     // reverse lookup by transactions ID for building dependency chains and transaction-driven cleanup
	createLocksByStateID           map[string]*prototk.SnapshotStateLock      // create locks keyed by state ID (at most one per state, from its minter)
	spendLocksByStateID            map[string]*prototk.SnapshotStateLock      // spend locks keyed by state ID (at most one per state)
	unspentCreatedStateIDs         map[string]struct{}                        // states with a create lock and no spend lock, so a snapshot never scans the spent majority
	spentStates                    map[string]*spentState                     // every spend-locked state, with its ID parsed once when the lock is taken
	spentStateIDsWithoutCreateLock spentStateIDSet                            // the spend exclusion set that an assembly which can optimistically spend states and is running a version which understands `fromBlock` will use
	allSpentStateIDs               spentStateIDSet                            // the spend exclusion set that an assembly which cannot optimistically spend, or is running on an older version which does not understand `fromBlock` will use
	confirmedLocksByBlock          map[uint64][]*prototk.SnapshotStateLock    // confirmed locks by confirmedAtBlock, so a sweep visits only the locks that are due
	readersByStateID               map[string]map[uuid.UUID]bool              // in-flight transactions reading each state
	readStatesByReader             map[uuid.UUID][]string                     // reverse lookup for transaction-driven cleanup
	locksByTransaction             map[uuid.UUID][]*prototk.SnapshotStateLock // secondary index for O(1) transaction-driven cleanup
	locksHeldFromBlock             uint64                                     // all create locks confirmed from this block onwards are still held: i.e. the block after the last one ForgetConfirmedLocks forgot
}

type grapherTX struct {
	ID uuid.UUID
}

type spentState struct {
	id    pldtypes.HexBytes
	nodes []string // nil when the state's distribution is unknown
}

type spentStateIDSet struct {
	anyNode map[string]pldtypes.HexBytes            // sent to every node: states with no known distribution, which any node may hold
	byNode  map[string]map[string]pldtypes.HexBytes // sent to each node their private data is distributed to
}

func NewGrapher(dependencyTracker dependencytracker.DependencyTracker, stateVisibilityTracker statevisibilitytracker.StateVisibilityStore, blockHeightTolerance uint64, allowOptimisticSpend bool) Grapher {
	return &grapher{
		blockHeightTolerance:           blockHeightTolerance,
		allowOptimisticSpend:           allowOptimisticSpend,
		stateVisibilityTracker:         stateVisibilityTracker,
		dependencyChain:                dependencyTracker.GetPostAssemblyDeps(),
		readDeps:                       dependencyTracker.GetReadDeps(),
		transactionByOutputState:       make(map[string]*grapherTX),
		transactionByID:                make(map[uuid.UUID]*grapherTX),
		outputStatesByMinter:           make(map[uuid.UUID][]string),
		createLocksByStateID:           make(map[string]*prototk.SnapshotStateLock),
		spendLocksByStateID:            make(map[string]*prototk.SnapshotStateLock),
		unspentCreatedStateIDs:         make(map[string]struct{}),
		allSpentStateIDs:               newSpentStateIDSet(),
		spentStateIDsWithoutCreateLock: newSpentStateIDSet(),
		spentStates:                    make(map[string]*spentState),
		confirmedLocksByBlock:          make(map[uint64][]*prototk.SnapshotStateLock),
		readersByStateID:               make(map[string]map[uuid.UUID]bool),
		readStatesByReader:             make(map[uuid.UUID][]string),
		locksByTransaction:             make(map[uuid.UUID][]*prototk.SnapshotStateLock),
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
	g.forgetTxReads(ctx, transactionID)
	g.forgetTxLocks(transactionID)
	delete(g.transactionByID, transactionID)
}

// ForgetTransaction removes the transaction from all in-flight tracking (dependency chain,
// transactionByOutputState, outputStatesByMinter, read tracking, transactionByID), stamps confirmedAtBlock on
// its locks, and clears the transaction reference on those locks.
// The statevisibilitytracker store is NOT cleared — private state data persists until the lock expires in
// ForgetConfirmedLocks, so coordinator handover heartbeats include it within the block tolerance window.
// No-op if the transaction is not known.
func (g *grapher) ForgetTransaction(ctx context.Context, transactionID uuid.UUID, confirmedAtBlock uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.transactionByID[transactionID]; !ok {
		return
	}

	g.dependencyChain.Delete(ctx, transactionID)
	g.forgetTxMints(transactionID)
	g.forgetTxReads(ctx, transactionID)

	// Stamp confirmedAtBlock on the transaction's locks and clear the transaction reference.
	// Each stateLock object is shared by pointer between locksByTransaction and the type-segregated
	// maps (createLocksByStateID / spendLocksByStateID). Mutating the fields
	// here propagates to whichever of those maps holds the same pointer, so after this loop every
	// type-segregated entry for this transaction reflects the confirmed state — even though
	// locksByTransaction is deleted immediately below. Each lock carries the confirmedAtBlock of
	// its own transaction, independently of any other lock on the same state.
	// A confirmation is applied before the coordinator's height passes its block, so a stamped create lock still
	// places its state in the view and the spent state ID sets need no change.
	txLocks := g.locksByTransaction[transactionID]
	for _, lock := range txLocks {
		lock.Transaction = nil
		lock.ConfirmedAtBlock = &confirmedAtBlock
	}
	g.confirmedLocksByBlock[confirmedAtBlock] = append(g.confirmedLocksByBlock[confirmedAtBlock], txLocks...)
	delete(g.locksByTransaction, transactionID)
	delete(g.transactionByID, transactionID)

	log.L(ctx).Debugf("ForgetTransaction: confirmed %d locks for tx %s at block %d", len(txLocks), transactionID, confirmedAtBlock)
}

// ForgetConfirmedLocks removes locks with no transaction whose block height tolerance window has passed,
// meaning the persisted state should have caught up and the lock is no longer needed.
// Removing a create lock cascades to delete the corresponding private state data from the
// statevisibilitytracker store — the create lock is the source of truth for how long private
// state data is held.
// Should be called on every NewBlock event.
func (g *grapher) ForgetConfirmedLocks(ctx context.Context, currentBlockHeight uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if currentBlockHeight < g.blockHeightTolerance {
		return // nothing is old enough to forget yet
	}
	forgetUpToBlock := currentBlockHeight - g.blockHeightTolerance
	g.forgetConfirmedLocks(ctx, forgetUpToBlock)
	g.locksHeldFromBlock = forgetUpToBlock + 1
}

// Caller must hold g.mu write lock.
func (g *grapher) forgetConfirmedLocks(ctx context.Context, forgetUpToBlock uint64) {
	for confirmedAtBlock, locks := range g.confirmedLocksByBlock {
		if confirmedAtBlock > forgetUpToBlock {
			continue
		}
		for _, lock := range locks {
			stateID := lock.GetStateId()
			switch lock.GetType() {
			case prototk.SnapshotStateLock_CREATE:
				if g.createLocksByStateID[stateID] == lock {
					log.L(ctx).Debugf("forgetConfirmedLocks: removing create lock on state %s (confirmedAtBlock=%d, forgetUpToBlock=%d)",
						stateID, confirmedAtBlock, forgetUpToBlock)
					g.deleteCreateLock(stateID)
					g.stateVisibilityTracker.Delete(stateID)
				}
			case prototk.SnapshotStateLock_SPEND:
				if g.spendLocksByStateID[stateID] == lock {
					log.L(ctx).Debugf("forgetConfirmedLocks: removing spend lock on state %s (confirmedAtBlock=%d, forgetUpToBlock=%d)",
						stateID, confirmedAtBlock, forgetUpToBlock)
					g.deleteSpendLock(stateID)
				}
			}
		}
		delete(g.confirmedLocksByBlock, confirmedAtBlock)
	}
}

// ImportStatesAndLocks imports confirmed locks and their associated private state data
// from a previous coordinator on handover. This is the for the golden path handover case where a new
// coordinator takes over only once the previous has flushed its transactions through to confirmation.
// Handing over the locks and private state data allows the new coordinator to maintain block height tolerance.
func (g *grapher) ImportStatesAndLocks(ctx context.Context, snapshot *prototk.StateSnapshot) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Import confirmed locks only, preserving existing entries. In-flight locks are skipped.
	// Route each lock into the appropriate type-segregated map, as one state can hold both a create and a spend lock.
	confirmedCreateLockStateIDs := make(map[string]bool, len(snapshot.GetLocks()))
	// Spend locks are set after the states are imported, so each is filed under the nodes its state is distributed to.
	var spendLocks []*prototk.SnapshotStateLock
	for _, lock := range snapshot.GetLocks() {
		stateID := lock.GetStateId()
		if lock.GetType() == prototk.SnapshotStateLock_READ {
			// A confirmed read places no constraint on later transactions.
			// Read locks are now removed from snapshots, but a node running an earlier version
			// might still send one.
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping read lock on state %s", stateID)
			continue
		}
		if lock.Transaction != nil {
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping in-flight lock on state %s — in-flight transactions are re-processed by the new coordinator", stateID)
			continue
		}
		if lock.ConfirmedAtBlock == nil {
			log.L(ctx).Warnf("ImportStatesAndLocks: skipping lock on state %s — no transaction and no ConfirmedAtBlock", stateID)
			continue
		}
		if *lock.ConfirmedAtBlock < g.locksHeldFromBlock {
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping lock on state %s — confirmed at block %d, which ForgetConfirmedLocks has already forgotten", stateID, *lock.ConfirmedAtBlock)
			continue
		}
		var targetMap map[string]*prototk.SnapshotStateLock
		switch lock.GetType() {
		case prototk.SnapshotStateLock_CREATE:
			targetMap = g.createLocksByStateID
			confirmedCreateLockStateIDs[stateID] = true
		default:
			targetMap = g.spendLocksByStateID
		}
		if _, exists := targetMap[stateID]; exists {
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping lock on state %s — existing lock takes precedence", stateID)
			continue
		}
		switch lock.GetType() {
		case prototk.SnapshotStateLock_CREATE:
			g.setCreateLock(stateID, lock)
		case prototk.SnapshotStateLock_SPEND:
			spendLocks = append(spendLocks, lock)
		}
		g.confirmedLocksByBlock[*lock.ConfirmedAtBlock] = append(g.confirmedLocksByBlock[*lock.ConfirmedAtBlock], lock)
		log.L(ctx).Debugf("ImportStatesAndLocks: imported confirmed lock on state %s", stateID)
	}

	// Import output states that have a corresponding confirmed create lock
	for _, state := range snapshot.GetStates() {
		stateID := state.GetState().GetId()
		if !confirmedCreateLockStateIDs[stateID] {
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping output state %s — no corresponding confirmed create lock found", stateID)
			continue
		}
		if g.stateVisibilityTracker.ImportIfAbsent(stateID, state) {
			log.L(ctx).Debugf("ImportStatesAndLocks: imported output state %s", stateID)
		} else {
			log.L(ctx).Debugf("ImportStatesAndLocks: skipping output state %s — existing entry takes precedence", stateID)
		}
	}

	for _, lock := range spendLocks {
		g.setSpendLock(ctx, lock.GetStateId(), lock)
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

// Caller must hold g.mu write lock.
func (g *grapher) forgetTxReads(ctx context.Context, transactionID uuid.UUID) {
	g.readDeps.Delete(ctx, transactionID)
	for _, stateID := range g.readStatesByReader[transactionID] {
		delete(g.readersByStateID[stateID], transactionID)
		if len(g.readersByStateID[stateID]) == 0 {
			delete(g.readersByStateID, stateID)
		}
	}
	delete(g.readStatesByReader, transactionID)
}

// forgetTxLocks removes all locks owned by a transaction from all lock indexes.
// Removing a create lock cascades to delete the corresponding private state data
// from the statevisibilitytracker store — the create lock governs the state's lifetime in the grapher.
// Caller must hold g.mu write lock.
func (g *grapher) forgetTxLocks(transactionID uuid.UUID) {
	for _, lock := range g.locksByTransaction[transactionID] {
		stateID := lock.GetStateId()
		switch lock.GetType() {
		case prototk.SnapshotStateLock_CREATE:
			g.deleteCreateLock(stateID)
			g.stateVisibilityTracker.Delete(stateID)
		case prototk.SnapshotStateLock_SPEND:
			g.deleteSpendLock(stateID)
		}
	}
	delete(g.locksByTransaction, transactionID)
}

// The create and spend lock maps should change only through these four functions, which keep
// unspentCreatedStateIDs and the spent state ID sets consistent with them. Caller must hold g.mu write lock.
func (g *grapher) setCreateLock(stateID string, lock *prototk.SnapshotStateLock) {
	g.createLocksByStateID[stateID] = lock
	if _, spent := g.spendLocksByStateID[stateID]; !spent {
		g.unspentCreatedStateIDs[stateID] = struct{}{}
	}
}

func (g *grapher) deleteCreateLock(stateID string) {
	delete(g.createLocksByStateID, stateID)
	delete(g.unspentCreatedStateIDs, stateID)
	if spent := g.spentStates[stateID]; spent != nil && g.allowOptimisticSpend {
		g.spentStateIDsWithoutCreateLock.add(stateID, spent)
	}
}

func (g *grapher) setSpendLock(ctx context.Context, stateID string, lock *prototk.SnapshotStateLock) {
	g.spendLocksByStateID[stateID] = lock
	delete(g.unspentCreatedStateIDs, stateID)
	id, err := pldtypes.ParseHexBytes(ctx, stateID)
	if err != nil {
		// Lock keys are produced from locally-resolved state IDs, so this cannot happen in
		// practice; leaving it out keeps the spent set a valid superset of the consumed states.
		log.L(ctx).Warnf("spend lock on unparseable state id %q is left out of the spent set: %s", stateID, err)
		return
	}
	// A node can only select a state it holds the private data for, so a spent state needs excluding
	// only on the nodes it is distributed to. A state's distribution never changes, so it is read once here.
	spent := &spentState{id: id, nodes: g.stateVisibilityTracker.AllowedNodes(stateID)}
	g.spentStates[stateID] = spent
	g.allSpentStateIDs.add(stateID, spent)
	// A node given a from block takes from its own store only the states confirmed before locksHeldFromBlock, and the
	// create lock of every state confirmed from then on is still held, so a state with one is never among them.
	if _, created := g.createLocksByStateID[stateID]; g.allowOptimisticSpend && !created {
		g.spentStateIDsWithoutCreateLock.add(stateID, spent)
	}
}

func (g *grapher) deleteSpendLock(stateID string) {
	delete(g.spendLocksByStateID, stateID)
	if spent := g.spentStates[stateID]; spent != nil {
		g.allSpentStateIDs.remove(stateID, spent)
		g.spentStateIDsWithoutCreateLock.remove(stateID, spent)
		delete(g.spentStates, stateID)
	}
	if _, created := g.createLocksByStateID[stateID]; created {
		g.unspentCreatedStateIDs[stateID] = struct{}{}
	}
}

func newSpentStateIDSet() spentStateIDSet {
	return spentStateIDSet{
		anyNode: make(map[string]pldtypes.HexBytes),
		byNode:  make(map[string]map[string]pldtypes.HexBytes),
	}
}

// add records a spent state's ID under the nodes it is sent to. Caller must hold g.mu write lock.
func (s spentStateIDSet) add(stateID string, spent *spentState) {
	if spent.nodes == nil {
		s.anyNode[stateID] = spent.id
		return
	}
	for _, node := range spent.nodes {
		byID := s.byNode[node]
		if byID == nil {
			byID = make(map[string]pldtypes.HexBytes)
			s.byNode[node] = byID
		}
		byID[stateID] = spent.id
	}
}

// Caller must hold g.mu write lock.
func (s spentStateIDSet) remove(stateID string, spent *spentState) {
	if spent.nodes == nil {
		delete(s.anyNode, stateID)
		return
	}
	for _, node := range spent.nodes {
		delete(s.byNode[node], stateID)
	}
}

// Caller must hold g.mu.
func (s spentStateIDSet) forNode(node string) []pldtypes.HexBytes {
	forNode := s.byNode[node]
	ids := make([]pldtypes.HexBytes, 0, len(s.anyNode)+len(forNode))
	for _, id := range s.anyNode {
		ids = append(ids, id)
	}
	for _, id := range forNode {
		ids = append(ids, id)
	}
	return ids
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
		g.setCreateLock(stateID, lock)
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
		log.L(ctx).Debugf("LockMintsOnReadAndSpend: TX %s reading state %s", transactionID.String(), stateID)
		if g.readersByStateID[stateID] == nil {
			g.readersByStateID[stateID] = make(map[uuid.UUID]bool)
		}
		g.readersByStateID[stateID][transactionID] = true
		g.readStatesByReader[transactionID] = append(g.readStatesByReader[transactionID], stateID)
		if mintedBy := g.transactionByOutputState[stateID]; mintedBy != nil {
			g.dependencyChain.AddPrerequisites(ctx, transactionID, mintedBy.ID)
		}
	}

	for _, state := range spendStates {
		stateID := state.GetId()
		log.L(ctx).Debugf("LockMintsOnReadAndSpend: TX %s taking spend lock on state %s", transactionID.String(), stateID)
		lock := &prototk.SnapshotStateLock{StateId: stateID, Transaction: &txnStr, Type: prototk.SnapshotStateLock_SPEND}
		g.setSpendLock(ctx, stateID, lock)
		g.locksByTransaction[transactionID] = append(g.locksByTransaction[transactionID], lock)
		if mintedBy := g.transactionByOutputState[stateID]; mintedBy != nil {
			g.dependencyChain.AddPrerequisites(ctx, transactionID, mintedBy.ID)
		}
		// A read state must still be unspent when its reader executes on chain.
		for readerID := range g.readersByStateID[stateID] {
			if readerID != transactionID {
				g.readDeps.AddPrerequisites(ctx, transactionID, readerID)
			}
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
	result.Locks = make([]*prototk.SnapshotStateLock, 0, len(g.createLocksByStateID)+len(g.spendLocksByStateID))
	for _, lock := range g.createLocksByStateID {
		result.Locks = append(result.Locks, lock)
	}
	for _, lock := range g.spendLocksByStateID {
		result.Locks = append(result.Locks, lock)
	}
	log.L(ctx).Debugf("ExportStatesAndLocks: %d output states (filtered from %d), %d locks (node=%q)", len(result.States), len(allStates), len(result.Locks), node)
	return result, nil
}

func (g *grapher) SnapshotView(ctx context.Context, node string, acceptsFromBlock bool) ([]*prototk.SnapshotState, []pldtypes.HexBytes, int64) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Only states with a create lock and no spend lock are selectable ahead-of-chain: a spend lock
	// means another transaction already consumed the state.
	var candidates []*prototk.SnapshotState
	for stateID := range g.unspentCreatedStateIDs {
		if state := g.stateVisibilityTracker.GetByIDForNode(stateID, node); state != nil {
			candidates = append(candidates, state)
		}
	}

	var spentStateIDs []pldtypes.HexBytes
	if g.allowOptimisticSpend && acceptsFromBlock {
		spentStateIDs = g.spentStateIDsWithoutCreateLock.forNode(node)
	} else {
		spentStateIDs = g.allSpentStateIDs.forNode(node)
	}
	fromBlock := int64(g.locksHeldFromBlock)
	log.L(ctx).Debugf("SnapshotView: %d candidate states, %d spent state IDs, from block %d (node=%q)", len(candidates), len(spentStateIDs), fromBlock, node)
	return candidates, spentStateIDs, fromBlock
}
