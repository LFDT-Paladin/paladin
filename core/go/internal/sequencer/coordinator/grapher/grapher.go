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
	"encoding/json"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// The Grapher package provides 3 core functions to Paladin:
// 1. It allows transactions to link to each other in a bi-directional dependency graph, based entirely on post-assembly outputs. This ensures
//    base-ledger state changes are correctly ordered, crucial to transaction success.
// 2. It records ahead-of-chain state changes, such as inputs being locked, to allow successful ahead-of-chain assembly for new transactions.
// 3. It provdes an interface to export the current ahead-of-chain state changes to give to originators to base new assembly requests on.

// An instance of the grapher is owned by the coordinator for a given sequencer. Transactions can query the grapher in thread-safe manner to
// understand their relationships to other transactions. For example:
//  - Did it create a state that another TX now depends on?
//  - Did it consume/lock a state that another TX created?

// The grapher is updated when base-ledger transactions are successful or revert. For example:
//   - A base-ledger revert has occurred so locked states should be unlocked as they are available again for re-assembly
//   - A base-ledger confirmation has occurred so consumed states should be removed
type Grapher interface {
	AddMinter(ctx context.Context, state []*components.FullState, txID uuid.UUID) error
	ExportMints(ctx context.Context) ([]byte, error)
	Forget(transactionID uuid.UUID)
	GetDependencies(ctx context.Context, transactionID uuid.UUID) []uuid.UUID
	GetDependents(ctx context.Context, transactionID uuid.UUID) []uuid.UUID
	LockMintsOnCreate(ctx context.Context, upserts []*components.StateUpsert, states []*components.FullState, transactionID uuid.UUID)
	LockMintsOnReadAndSpend(ctx context.Context, readStates []*components.FullState, spendStates []*components.FullState, transactionID uuid.UUID)
}

type grapher struct {
	mu sync.RWMutex

	transactionByOutputState  map[string]*grapherTX
	transactionByID           map[uuid.UUID]*grapherTX
	outputStatesByMinter      map[uuid.UUID][]*components.StateUpsert // used for reverse lookup to cleanup transactionByOutputState
	lockedStatesByTransaction map[uuid.UUID][]*stateLock              // states locked by a given tranasction
}

type grapherTX struct {
	ID           uuid.UUID
	dependencies *pldapi.TransactionDependencies
}

func NewGrapher(ctx context.Context) Grapher {
	return &grapher{
		transactionByOutputState:  make(map[string]*grapherTX),
		transactionByID:           make(map[uuid.UUID]*grapherTX),
		outputStatesByMinter:      make(map[uuid.UUID][]*components.StateUpsert),
		lockedStatesByTransaction: make(map[uuid.UUID][]*stateLock),
	}
}

// pldapi.StateLocks do not include the stateID in the serialized JSON so we need to define a new struct to include it
type stateLock struct {
	State       pldtypes.HexBytes                   `json:"stateId"`
	Transaction uuid.UUID                           `json:"transaction"`
	Type        pldtypes.Enum[pldapi.StateLockType] `json:"type"`
}

type exportableStates struct {
	OutputState []*components.StateUpsert `json:"states"`
	LockedState []*stateLock              `json:"locks"`
}

// Record (idempotently) the existence of a transaction that consumes at least one state.
// Caller must hold g.mu write lock.
func (g *grapher) addConsumer(transactionID uuid.UUID) {
	if _, ok := g.transactionByID[transactionID]; !ok {
		g.transactionByID[transactionID] = &grapherTX{
			ID: transactionID,
			dependencies: &pldapi.TransactionDependencies{
				DependsOn: make([]uuid.UUID, 0),
				PrereqOf:  make([]uuid.UUID, 0),
			},
		}
	}
}

// Record that a set of states has been minted by the specified transaction. Adds the transaction to the grapher if it doesn't exist already.
func (g *grapher) AddMinter(ctx context.Context, states []*components.FullState, transactionID uuid.UUID) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.transactionByID[transactionID] = &grapherTX{
		ID: transactionID,
		dependencies: &pldapi.TransactionDependencies{
			DependsOn: make([]uuid.UUID, 0),
			PrereqOf:  make([]uuid.UUID, 0),
		},
	}
	for _, state := range states {
		if txn, ok := g.transactionByOutputState[state.ID.String()]; ok {
			return i18n.NewError(ctx, msgs.MsgSequencerGrapherAddMinterAlreadyExistsError, transactionID.String(), state.ID.String(), txn.ID.String())
		}
		g.transactionByOutputState[state.ID.String()] = g.transactionByID[transactionID]

		if g.outputStatesByMinter[transactionID] == nil {
			g.outputStatesByMinter[transactionID] = make([]*components.StateUpsert, 0)
		}
		g.outputStatesByMinter[transactionID] = append(g.outputStatesByMinter[transactionID], &components.StateUpsert{
			ID:     state.ID,
			Schema: state.Schema,
			Data:   state.Data,
		})
	}

	return nil
}

// Forget about a transaction from the grapher, including any states it produced, any locks it held, and any dependency chain it is part of
func (g *grapher) Forget(transactionID uuid.UUID) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Anything that used to depend on this transaction no longer does.
	// Anything that this transaction used to depend on, it no longer does
	g.removeAllDependencyLinks(transactionID)
	g.forgetMints(transactionID)
	g.forgetLocks(transactionID)
	delete(g.transactionByID, transactionID)
}

func (g *grapher) removeAllDependencyLinks(transactionID uuid.UUID) {
	tx := g.transactionByID[transactionID]
	if tx == nil {
		return
	}

	// Find all transactions that this TX is a pre-req of
	dependentIDs := make(map[uuid.UUID]struct{})
	for _, dependentID := range tx.dependencies.PrereqOf {
		dependentIDs[dependentID] = struct{}{}
	}
	// Then remove the depends-on chain from those transactions to this one
	for dependentID := range dependentIDs {
		tx := g.transactionByID[dependentID]
		if tx == nil {
			continue
		}
		tx.dependencies.DependsOn = removeUUID(tx.dependencies.DependsOn, transactionID)
	}

	// Find all transactions that this TX depends on
	prereqIDs := make(map[uuid.UUID]struct{})
	for _, prereqID := range tx.dependencies.DependsOn {
		prereqIDs[prereqID] = struct{}{}
	}
	// Then remove this TX from each prerequisite's pre-req list
	for prereqID := range prereqIDs {
		prereqTX := g.transactionByID[prereqID]
		if prereqTX == nil {
			continue
		}
		prereqTX.dependencies.PrereqOf = removeUUID(prereqTX.dependencies.PrereqOf, transactionID)
	}
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

// Caller must hold g.mu write lock
func (g *grapher) forgetMints(transactionID uuid.UUID) {
	if outputStates, ok := g.outputStatesByMinter[transactionID]; ok {
		for _, state := range outputStates {
			delete(g.transactionByOutputState, state.ID.String())
		}
		delete(g.outputStatesByMinter, transactionID)
	}
}

// Caller must hold g.mu write lock
func (g *grapher) forgetLocks(transactionID uuid.UUID) {
	delete(g.lockedStatesByTransaction, transactionID)
}

// Get transactions we are dependent on
func (g *grapher) GetDependencies(ctx context.Context, transactionID uuid.UUID) []uuid.UUID {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if tx, ok := g.transactionByID[transactionID]; ok {
		out := make([]uuid.UUID, len(tx.dependencies.DependsOn))
		copy(out, tx.dependencies.DependsOn)
		return out
	}
	return nil
}

// Get transactions we are a pre-req of
func (g *grapher) GetDependents(ctx context.Context, transactionID uuid.UUID) []uuid.UUID {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if tx, ok := g.transactionByID[transactionID]; ok {
		out := make([]uuid.UUID, len(tx.dependencies.PrereqOf))
		copy(out, tx.dependencies.PrereqOf)
		return out
	}
	return nil
}

// Caller must hold write lock
func (g *grapher) lockMints(states []*components.FullState, transactionID uuid.UUID, lockType pldapi.StateLockType) {
	g.addConsumer(transactionID)
	if g.lockedStatesByTransaction == nil {
		g.lockedStatesByTransaction = make(map[uuid.UUID][]*stateLock)
	}
	for _, state := range states {
		g.lockedStatesByTransaction[transactionID] = append(g.lockedStatesByTransaction[transactionID],
			&stateLock{
				State:       state.ID,
				Transaction: transactionID,
				Type:        lockType.Enum(),
			})
	}
}

func (g *grapher) LockMintsOnCreate(ctx context.Context, upserts []*components.StateUpsert, states []*components.FullState, transactionID uuid.UUID) {
	g.mu.Lock()
	defer g.mu.Unlock()

	createLocks := make([]*components.FullState, 0, len(states))
	for i, ps := range upserts {
		if ps.CreatedBy != nil {
			log.L(ctx).Debugf("LockMintsOnCreate: creating lock for potential state %s, it's full state ID is %s", ps.ID.String(), states[i].ID.String())
			createLocks = append(createLocks, &components.FullState{
				ID: states[i].ID,
			})
		}
	}
	g.lockMints(createLocks, transactionID, pldapi.StateLockTypeCreate)
}

func (g *grapher) LockMintsOnReadAndSpend(ctx context.Context, readStates []*components.FullState, spendStates []*components.FullState, transactionID uuid.UUID) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.lockMints(readStates, transactionID, pldapi.StateLockTypeRead)
	for _, state := range readStates {
		log.L(ctx).Debugf("LockMintsOnReadAndSpend: TX %s taking read lock on state %s", transactionID.String(), state.ID.String())
		mintedBy := g.transactionByOutputState[state.ID.String()]

		// We can spend something the grapher isn't aware of. If the grapher doesn't recognise this state this TX has no dependecies.
		if mintedBy != nil {
			// Add depends-on chain
			g.transactionByID[transactionID].dependencies.DependsOn = append(g.transactionByID[transactionID].dependencies.DependsOn, mintedBy.ID)
			// Add pre-req chain
			mintedBy.dependencies.PrereqOf = append(mintedBy.dependencies.PrereqOf, transactionID)
		}
	}

	g.lockMints(spendStates, transactionID, pldapi.StateLockTypeSpend)
	for _, state := range spendStates {
		log.L(ctx).Debugf("LockMintsOnReadAndSpend: TX %s taking spend lock on state %s", transactionID.String(), state.ID.String())
		mintedBy := g.transactionByOutputState[state.ID.String()]

		// We can spend something the grapher isn't aware of. If the grapher doesn't recognise this state this TX has no dependecies.
		if mintedBy != nil {
			// Add depends-on chain
			g.transactionByID[transactionID].dependencies.DependsOn = append(g.transactionByID[transactionID].dependencies.DependsOn, mintedBy.ID)
			// Add pre-req chain
			mintedBy.dependencies.PrereqOf = append(mintedBy.dependencies.PrereqOf, transactionID)
		}
	}
}

func (g *grapher) ExportMints(ctx context.Context) ([]byte, error) {
	g.mu.RLock()
	exportableStates := exportableStates{}
	exportableStates.OutputState = make([]*components.StateUpsert, 0, len(g.outputStatesByMinter))
	for _, states := range g.outputStatesByMinter {
		exportableStates.OutputState = append(exportableStates.OutputState, states...)
	}
	exportableStates.LockedState = make([]*stateLock, 0, len(g.lockedStatesByTransaction))
	for _, locks := range g.lockedStatesByTransaction {
		exportableStates.LockedState = append(exportableStates.LockedState, locks...)
	}
	g.mu.RUnlock()
	jsonStr, err := json.Marshal(exportableStates)
	return jsonStr, err
}
