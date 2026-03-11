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

package transaction

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// Interface Grapher allows transactions to link to each other in a bi-directional dependency graph. It is owned by the coordinator for a given sequencer.
// Transactions can query the grapher to understand their relationships to previous and subsequent transactions. For example:
//  - Did it create a state that another TX now depends on?
//  - Did it consume/lock a state that another TX created?
//  - A base-ledger revert has occurred, who else will need to be re-pooled and re-assembled? Which states should be unlocked and which ones removed?
//  - A base-ledger confirmation has occurred, which states and locks can be removed?

// There are 3 ordering guarantees that Paladin enforces:
//   - Explicit transaction dependencies. These are specified by the application and tracked at the originator. The grapher is not related to explicit dependencies.
//   - Implicit transaction dependencies. These come about as a result of assembling transactions and are tracked through this grapher.
//   - First-assemble dependencies. A transaction arriving at an originator from an application is guaranteed to have its FIRST assemble request take place
//     before any transaction that arrived at the originator after it. Re-assembly order (e.g. if a base ledger TX reverts) is not guaranteed.
type Grapher interface {
	Add(context.Context, *CoordinatorTransaction)
	AddMinter(ctx context.Context, state []*components.FullState, transaction *CoordinatorTransaction) error
	ExportMints(ctx context.Context) ([]byte, error)
	Forget(transactionID uuid.UUID) error
	ForgetMints(transactionID uuid.UUID)
	ForgetLocks(transactionID uuid.UUID)
	LookupMinter(ctx context.Context, stateID pldtypes.HexBytes) (*CoordinatorTransaction, error)
	LockMintsOnCreate(ctx context.Context, upserts []*components.StateUpsert, states []*components.FullState, transactionID uuid.UUID)
	LockMintsOnSpend(ctx context.Context, states []*components.FullState, transactionID uuid.UUID)
	LockMintsOnRead(ctx context.Context, states []*components.FullState, transactionID uuid.UUID)
	TransactionByID(ctx context.Context, transactionID uuid.UUID) *CoordinatorTransaction
}

type grapher struct {
	transactionByOutputState  map[string]*CoordinatorTransaction
	transactionByID           map[uuid.UUID]*CoordinatorTransaction
	outputStatesByMinter      map[uuid.UUID][]*components.StateUpsert // used for reverse lookup to cleanup transactionByOutputState
	lockedStatesByTransaction map[uuid.UUID][]*stateLock              // states locked by a given tranasction
}

// The grapher is designed to be called on a single-threaded sequencer event loop and is not thread safe.
// It must only be called from the state machine loop to ensure assembly of a TX is based on completion of
// any updates made by a previous change in the state machine (e.g. removing states from a previously
// reverted transaction)
func NewGrapher(ctx context.Context) Grapher {
	return &grapher{
		transactionByOutputState:  make(map[string]*CoordinatorTransaction),
		transactionByID:           make(map[uuid.UUID]*CoordinatorTransaction),
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

func (s *grapher) Add(ctx context.Context, txn CoordinatorTransaction) {
	s.transactionByID[txn.GetID()] = txn
}

// Define states as having been produced by the specified transaction.
func (s *grapher) AddMinter(ctx context.Context, states []*components.FullState, transaction *CoordinatorTransaction) error {
	for _, state := range states {
		if txn, ok := s.transactionByOutputState[state.ID.String()]; ok {
			return i18n.NewError(ctx, msgs.MsgSequencerAddMinterAlreadyExistsError, transaction.pt.ID.String(), state.ID.String(), txn.pt.ID.String())
		}
		s.transactionByOutputState[state.ID.String()] = transaction

		if s.outputStatesByMinter[transaction.pt.ID] == nil {
			s.outputStatesByMinter[transaction.pt.ID] = []*components.StateUpsert{
				&components.StateUpsert{
					ID:     state.ID,
					Schema: state.Schema,
					Data:   state.Data,
				},
			}
		} else {
			s.outputStatesByMinter[transaction.pt.ID] = append(s.outputStatesByMinter[transaction.pt.ID], &components.StateUpsert{
				ID:     state.ID,
				Schema: state.Schema,
				Data:   state.Data,
			})
		}
	}

	return nil
}

// Forget about a transaction from the grapher, including any states it produced and any locks it held.
func (s *grapher) Forget(transactionID uuid.UUID) error {
	txn := s.transactionByID[transactionID]
	if txn != nil {
		s.pruneDependencyLinks(txn)
	}
	s.ForgetMints(transactionID)
	s.ForgetLocks(transactionID)
	delete(s.transactionByID, transactionID)
	return nil
}

// Remove stale dependency links in both directions:
//   - forward links on dependents that point to this tx (dependent.DependsOn)
//   - reverse links on prerequisites that include this tx as a dependent (prereq.PrereqOf)
//
// Note - this mutates transaction dependency metadata; it does not mutate grapher indexes directly.
func (s *grapher) pruneDependencyLinks(txn CoordinatorTransaction) {
	// Remove this TX from all dependent forward links (dependent.DependsOn).
	dependentIDs := make(map[uuid.UUID]struct{})
	if txn.GetDependencies() != nil {
		for _, dependentID := range txn.GetDependencies().PrereqOf {
			dependentIDs[dependentID] = struct{}{}
		}
	}
	for dependentID := range dependentIDs {
		dependent := s.transactionByID[dependentID]
		if dependent == nil {
			continue
		}
		if dependent.GetDependencies() != nil {
			dependent.GetDependencies().DependsOn = removeUUID(dependent.GetDependencies().DependsOn, txn.GetID())
		}
	}

	// Remove this TX from all prerequisite reverse links (prereq.PrereqOf).
	// If transactions are being dispatched as chained transactions, there is no guarantee that the
	// chained transactions will be finalised in the order they were dispatched in, which means a dependent
	// may be cleaned up before the prerequisite.
	prereqIDs := make(map[uuid.UUID]struct{})
	if txn.GetDependencies() != nil {
		for _, prereqID := range txn.GetDependencies().DependsOn {
			prereqIDs[prereqID] = struct{}{}
		}
	}
	for prereqID := range prereqIDs {
		prereq := s.transactionByID[prereqID]
		if prereq == nil {
			continue
		}
		if prereq.GetDependencies() != nil {
			prereq.GetDependencies().PrereqOf = removeUUID(prereq.GetDependencies().PrereqOf, txn.GetID())
		}
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

func (s *grapher) ForgetMints(transactionID uuid.UUID) {
	if outputStates, ok := s.outputStatesByMinter[transactionID]; ok {
		for _, state := range outputStates {
			delete(s.transactionByOutputState, state.ID.String())
		}
		delete(s.outputStatesByMinter, transactionID)
	}
	// Note we specifically don't delete the transaction (i.e. the minter) here. Use Forget() to do both.
}

func (s *grapher) ForgetLocks(transactionID uuid.UUID) {
	if locks, ok := s.lockedStatesByTransaction[transactionID]; ok {
		for _, lock := range locks {
			delete(s.lockedStatesByTransaction, lock.Transaction)
		}
		delete(s.lockedStatesByTransaction, transactionID)
	}
	// Note we specifically don't delete the transaction (i.e. the minter) here. Use Forget() to do both.
}

func (s *grapher) ForgetLocks(transactionID uuid.UUID) {
	if locks, ok := s.lockedStatesByTransaction[transactionID]; ok {
		for _, lock := range locks {
			delete(s.lockedStatesByTransaction, lock.Transaction)
		}
		delete(s.lockedStatesByTransaction, transactionID)
	}
	// Note we specifically don't delete the transaction (i.e. the minter) here. Use Forget() to do both.
}

func (s *grapher) TransactionByID(ctx context.Context, transactionID uuid.UUID) CoordinatorTransaction {
	return s.transactionByID[transactionID]
}

func (s *grapher) lockMints(ctx context.Context, states []*components.FullState, transactionID uuid.UUID, lockType pldapi.StateLockType) {
	for _, state := range states {
		if s.lockedStatesByTransaction[transactionID] == nil {
			s.lockedStatesByTransaction[transactionID] = []*stateLock{
				&stateLock{
					State:       state.ID,
					Transaction: transactionID,
					Type:        lockType.Enum(),
				},
			}
		} else {
			s.lockedStatesByTransaction[transactionID] = append(s.lockedStatesByTransaction[transactionID], &stateLock{
				State:       state.ID,
				Transaction: transactionID,
				Type:        lockType.Enum(),
			})
		}
	}
}

func (s *grapher) LockMintsOnCreate(ctx context.Context, upserts []*components.StateUpsert, states []*components.FullState, transactionID uuid.UUID) {
	createLocks := make([]*components.FullState, 0, len(states))
	for i, ps := range upserts {
		if ps.CreatedBy != nil {
			log.L(ctx).Debugf("LockMintsOnCreate: creating lock for potential state %s, it's full state ID is %s", ps.ID.String(), states[i].ID.String())
			createLocks = append(createLocks, &components.FullState{
				ID: states[i].ID,
			})
		}
	}
	s.lockMints(ctx, createLocks, transactionID, pldapi.StateLockTypeCreate)
}

func (s *grapher) LockMintsOnRead(ctx context.Context, states []*components.FullState, transactionID uuid.UUID) {
	s.lockMints(ctx, states, transactionID, pldapi.StateLockTypeRead)
}

func (s *grapher) LockMintsOnSpend(ctx context.Context, states []*components.FullState, transactionID uuid.UUID) {
	s.lockMints(ctx, states, transactionID, pldapi.StateLockTypeSpend)
}

func (s *grapher) LookupMinter(ctx context.Context, stateID pldtypes.HexBytes) (*CoordinatorTransaction, error) {
	return s.transactionByOutputState[stateID.String()], nil
}

func (s *grapher) ExportMints(ctx context.Context) ([]byte, error) {
	log.L(ctx).Debugf("ExportMints: exporting %d output states and %d locked states", len(s.outputStatesByMinter), len(s.lockedStatesByTransaction))
	exportableStates := exportableStates{}
	exportableStates.OutputState = make([]*components.StateUpsert, 0, len(s.outputStatesByMinter))
	for _, states := range s.outputStatesByMinter {
		log.L(ctx).Debugf("ExportMints: exporting %d output states", len(states))
		exportableStates.OutputState = append(exportableStates.OutputState, states...)
	}
	exportableStates.LockedState = make([]*stateLock, 0, len(s.lockedStatesByTransaction))
	for _, locks := range s.lockedStatesByTransaction {
		log.L(ctx).Debugf("ExportMints: exporting %d locked states", len(locks))
		exportableStates.LockedState = append(exportableStates.LockedState, locks...)
	}
	jsonStr, err := json.Marshal(exportableStates)
	if err != nil {
		return nil, err
	}

	fmt.Printf("ExportMints: JSON string: %s\n", jsonStr)
	return jsonStr, nil
}
