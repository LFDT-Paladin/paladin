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
	Forget(transactionID uuid.UUID) error
	ForgetMints(transactionID uuid.UUID)
	ForgetLocks(transactionID uuid.UUID)
	GetDependencies(ctx context.Context, transactionID uuid.UUID) []uuid.UUID
	GetDependants(ctx context.Context, transactionID uuid.UUID) []uuid.UUID
	// LookupMinter(ctx context.Context, stateID pldtypes.HexBytes) (*CoordinatorTransaction, error)
	LockMintsOnCreate(ctx context.Context, upserts []*components.StateUpsert, states []*components.FullState, transactionID uuid.UUID)
	LockMintsOnSpend(ctx context.Context, states []*components.FullState, transactionID uuid.UUID)
	LockMintsOnRead(ctx context.Context, states []*components.FullState, transactionID uuid.UUID)
	// TransactionByID(ctx context.Context, transactionID uuid.UUID) *CoordinatorTransaction
}

type grapher struct {
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

// Record (idempotently) the existence of a transaction that consumes at least one state
func (s *grapher) addConsumer(ctx context.Context, transactionID uuid.UUID) {
	if _, ok := s.transactionByID[transactionID]; !ok {
		s.transactionByID[transactionID] = &grapherTX{
			ID: transactionID,
			dependencies: &pldapi.TransactionDependencies{
				DependsOn: make([]uuid.UUID, 0),
				PrereqOf:  make([]uuid.UUID, 0),
			},
		}
	}
}

// Defines states as having been produced by the specified transaction.
// Adds the transaction to the grapher if it doesn't exist already.
func (s *grapher) AddMinter(ctx context.Context, states []*components.FullState, transactionID uuid.UUID) error {
	for _, state := range states {
		if txn, ok := s.transactionByOutputState[state.ID.String()]; ok {
			return i18n.NewError(ctx, msgs.MsgSequencerAddMinterAlreadyExistsError, transactionID.String(), state.ID.String(), txn.ID.String())
		}
		s.transactionByOutputState[state.ID.String()] = &grapherTX{
			ID: transactionID,
			dependencies: &pldapi.TransactionDependencies{
				DependsOn: make([]uuid.UUID, 0),
				PrereqOf:  make([]uuid.UUID, 0),
			},
		}

		if s.outputStatesByMinter[transactionID] == nil {
			s.outputStatesByMinter[transactionID] = []*components.StateUpsert{
				&components.StateUpsert{
					ID:     state.ID,
					Schema: state.Schema,
					Data:   state.Data,
				},
			}
		}
	}

	return nil
}

// Forget about a transaction from the grapher, including any states it produced and any locks it held.
func (s *grapher) Forget(transactionID uuid.UUID) error {
	// txn := s.transactionByID[transactionID]
	// if txn != nil {
	// 	s.pruneDependencyLinks(txn)
	// }
	s.ForgetMints(transactionID)
	s.ForgetLocks(transactionID)
	delete(s.transactionByID, transactionID)
	return nil
}

// Temporary approach that removes updates depends-on list for any transactions this is a pre-req of
// Note - this doesn't update the grapher itself
// func (s *grapher) pruneDependencyLinks(txn *CoordinatorTransaction) {
// 	// Remove this TX from all dependent forward links.
// 	dependentIDs := make(map[uuid.UUID]struct{})
// 	if txn.dependencies != nil {
// 		for _, dependentID := range txn.dependencies.PrereqOf {
// 			dependentIDs[dependentID] = struct{}{}
// 		}
// 	}
// 	for dependentID := range dependentIDs {
// 		dependent := s.transactionByID[dependentID]
// 		if dependent == nil {
// 			continue
// 		}
// 		if dependent.dependencies != nil {
// 			dependent.dependencies.DependsOn = removeUUID(dependent.dependencies.DependsOn, txn.pt.ID)
// 		}
// 	}
// }

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

// func (s *grapher) TransactionByID(ctx context.Context, transactionID uuid.UUID) *CoordinatorTransaction {
// 	return s.transactionByID[transactionID]
// }

// Get transactions we are dependent on
func (s *grapher) GetDependencies(ctx context.Context, transactionID uuid.UUID) []uuid.UUID {
	return s.transactionByID[transactionID].dependencies.DependsOn
}

// Get transactions we are a pre-req of
func (s *grapher) GetDependants(ctx context.Context, transactionID uuid.UUID) []uuid.UUID {
	return s.transactionByID[transactionID].dependencies.PrereqOf
}
func (s *grapher) lockMints(ctx context.Context, states []*components.FullState, transactionID uuid.UUID, lockType pldapi.StateLockType) {
	s.addConsumer(ctx, transactionID)
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
	for _, state := range states {
		mintedBy := s.transactionByOutputState[state.ID.String()]

		// We can spend something the grapher isn't aware of. If the grapher doesn't recognise this state this TX has no dependecies.
		if mintedBy != nil {
			s.transactionByID[transactionID].dependencies.DependsOn = append(s.transactionByID[transactionID].dependencies.DependsOn, mintedBy.ID)
		}
	}
}

func (s *grapher) LockMintsOnSpend(ctx context.Context, states []*components.FullState, transactionID uuid.UUID) {
	s.lockMints(ctx, states, transactionID, pldapi.StateLockTypeSpend)
	for _, state := range states {
		mintedBy := s.transactionByOutputState[state.ID.String()]

		// We can spend something the grapher isn't aware of. If the grapher doesn't recognise this state this TX has no dependecies.
		if mintedBy != nil {
			s.transactionByID[transactionID].dependencies.DependsOn = append(s.transactionByID[transactionID].dependencies.DependsOn, mintedBy.ID)
		}
	}
}

// func (s *grapher) LookupMinter(ctx context.Context, stateID pldtypes.HexBytes) (*CoordinatorTransaction, error) {
// 	return s.transactionByOutputState[stateID.String()], nil
// }

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
