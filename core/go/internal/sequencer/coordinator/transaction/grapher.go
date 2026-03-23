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
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// Interface Grapher allows transactions to link to each other in a dependency graph
// Transactions may know about their dependencies either explicitly via transactions IDs specified on the pre-assembly spec
// or implicitly via the post assembly input and read state IDs .
// In the former case, the Grapher helps to resolve a transaction ID to a pointer to the in-memory state machine for that transaction object
// In the latter case the Grapher helps to resolve a state ID to a pointer to the in-memory state machine for the transaction object that produced that state
// Transactions register themselves with the Grapher and can use the Grapher to look up each other
// The Grapher is not a graph data structure, but a simple index of transactions by ID and by state ID
// the actual graph is the emergent data structure of the transactions maintaining links to each other
type Grapher interface {
	Add(context.Context, CoordinatorTransaction)
	TransactionByID(ctx context.Context, transactionID uuid.UUID) CoordinatorTransaction
	LookupMinter(ctx context.Context, stateID pldtypes.HexBytes) (*coordinatorTransaction, error)
	AddMinter(ctx context.Context, stateID pldtypes.HexBytes, transaction *coordinatorTransaction) error
	Forget(transactionID uuid.UUID) error
	ForgetMints(transactionID uuid.UUID)
}

type grapher struct {
	transactionByOutputState map[string]*coordinatorTransaction
	transactionByID          map[uuid.UUID]CoordinatorTransaction
	outputStatesByMinter     map[uuid.UUID][]string //used for reverse lookup to cleanup transactionByOutputState
}

// The grapher is designed to be called on a single-threaded sequencer event loop and is not thread safe.
// It must only be called from the state machine loop to ensure assembly of a TX is based on completion of
// any updates made by a previous change in the state machine (e.g. removing states from a previously
// reverted transaction)
func NewGrapher(ctx context.Context) Grapher {
	return &grapher{
		transactionByOutputState: make(map[string]*coordinatorTransaction),
		transactionByID:          make(map[uuid.UUID]CoordinatorTransaction),
		outputStatesByMinter:     make(map[uuid.UUID][]string),
	}
}

func (s *grapher) Add(ctx context.Context, txn CoordinatorTransaction) {
	s.transactionByID[txn.GetID()] = txn
}

func (s *grapher) LookupMinter(ctx context.Context, stateID pldtypes.HexBytes) (*coordinatorTransaction, error) {
	return s.transactionByOutputState[stateID.String()], nil
}

func (s *grapher) AddMinter(ctx context.Context, stateID pldtypes.HexBytes, transaction *coordinatorTransaction) error {
	if txn, ok := s.transactionByOutputState[stateID.String()]; ok {
		msg := fmt.Sprintf("Duplicate minter. stateID %s already indexed as minted by %s but attempted to add minter %s", stateID.String(), txn.pt.ID.String(), transaction.pt.ID.String())
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}
	s.transactionByOutputState[stateID.String()] = transaction
	s.outputStatesByMinter[transaction.pt.ID] = append(s.outputStatesByMinter[transaction.pt.ID], stateID.String())
	return nil
}

func (s *grapher) Forget(transactionID uuid.UUID) error {
	txn := s.transactionByID[transactionID]
	if txn != nil {
		s.pruneDependencyLinks(txn)
	}
	s.ForgetMints(transactionID)
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
		for _, stateID := range outputStates {
			delete(s.transactionByOutputState, stateID)
		}
		delete(s.outputStatesByMinter, transactionID)
	}
	// Note we specifically don't delete the transaction (i.e. the minter) here. Use Forget() to do both.
}

func (s *grapher) TransactionByID(ctx context.Context, transactionID uuid.UUID) CoordinatorTransaction {
	return s.transactionByID[transactionID]
}
