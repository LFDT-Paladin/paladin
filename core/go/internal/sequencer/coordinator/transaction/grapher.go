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

// Interface Grapher allows transactions to link to each other in a dependency graph
// Transactions may know about their dependencies either explicitly via transactions IDs specified on the pre-assembly spec
// or implicitly via the post assembly input and read state IDs .
// In the former case, the Grapher helps to resolve a transaction ID to a pointer to the in-memory state machine for that transaction object
// In the latter case the Grapher helps to resolve a state ID to a pointer to the in-memory state machine for the transaction object that produced that state
// Transactions register themselves with the Grapher and can use the Grapher to look up each other
// The Grapher is not a graph data structure, but a simple index of transactions by ID and by state ID
// the actual graph is the emergent data structure of the transactions maintaining links to each other
type Grapher interface {
	Add(context.Context, *Transaction)
	AddMinter(ctx context.Context, state *components.FullState, transaction *Transaction) error
	ExportMints(ctx context.Context) ([]byte, error)
	Forget(transactionID uuid.UUID) error
	ForgetMints(transactionID uuid.UUID)
	ForgetLocks(transactionID uuid.UUID)
	LookupMinter(ctx context.Context, stateID pldtypes.HexBytes) (*Transaction, error)
	LockMintOnCreate(ctx context.Context, stateID pldtypes.HexBytes, transactionID uuid.UUID) error
	LockMintOnSpend(ctx context.Context, stateID pldtypes.HexBytes, transactionID uuid.UUID) error
	TransactionByID(ctx context.Context, transactionID uuid.UUID) *Transaction
}

type grapher struct {
	transactionByOutputState  map[string]*Transaction
	transactionByID           map[uuid.UUID]*Transaction
	outputStatesByMinter      map[uuid.UUID][]*components.StateUpsert // used for reverse lookup to cleanup transactionByOutputState
	lockedStatesByTransaction map[uuid.UUID][]*stateLock              // states locked by a given tranasction
}

// The grapher is designed to be called on a single-threaded sequencer event loop and is not thread safe.
// It must only be called from the state machine loop to ensure assembly of a TX is based on completion of
// any updates made by a previous change in the state machine (e.g. removing states from a previously
// reverted transaction)
func NewGrapher(ctx context.Context) Grapher {
	return &grapher{
		transactionByOutputState:  make(map[string]*Transaction),
		transactionByID:           make(map[uuid.UUID]*Transaction),
		outputStatesByMinter:      make(map[uuid.UUID][]*components.StateUpsert),
		lockedStatesByTransaction: make(map[uuid.UUID][]*stateLock),
	}
}

func (s *grapher) Add(ctx context.Context, txn *Transaction) {
	s.transactionByID[txn.ID] = txn
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

func (s *grapher) AddMinter(ctx context.Context, state *components.FullState, transaction *Transaction) error {
	if txn, ok := s.transactionByOutputState[state.ID.String()]; ok {
		msg := fmt.Sprintf("Duplicate minter. stateID %s already indexed as minted by %s but attempted to add minter %s", state.ID.String(), txn.ID.String(), transaction.ID.String())
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}
	s.transactionByOutputState[state.ID.String()] = transaction

	if s.outputStatesByMinter[transaction.ID] == nil {
		s.outputStatesByMinter[transaction.ID] = []*components.StateUpsert{
			&components.StateUpsert{
				ID:     state.ID,
				Schema: state.Schema,
				Data:   state.Data,
			},
		}
	} else {
		s.outputStatesByMinter[transaction.ID] = append(s.outputStatesByMinter[transaction.ID], &components.StateUpsert{
			ID:     state.ID,
			Schema: state.Schema,
			Data:   state.Data,
		})
	}

	return nil
}

func (s *grapher) ExportMints(ctx context.Context) ([]byte, error) {
	exportableStates := exportableStates{}
	exportableStates.OutputState = make([]*components.StateUpsert, 0, len(s.outputStatesByMinter))
	for _, states := range s.outputStatesByMinter {
		exportableStates.OutputState = append(exportableStates.OutputState, states...)
	}
	exportableStates.LockedState = make([]*stateLock, 0, len(s.lockedStatesByTransaction))
	for _, locks := range s.lockedStatesByTransaction {
		exportableStates.LockedState = append(exportableStates.LockedState, locks...)
	}
	jsonStr, err := json.Marshal(exportableStates)
	if err != nil {
		return nil, err
	}

	fmt.Printf("ExportMints: JSON string:   %s\n", jsonStr)
	return jsonStr, nil
}

func (s *grapher) Forget(transactionID uuid.UUID) error {
	fmt.Printf("Forget: Forgetting transaction ID %s\n", transactionID.String())
	s.ForgetMints(transactionID)
	s.ForgetLocks(transactionID)
	delete(s.transactionByID, transactionID)
	return nil
}

func (s *grapher) ForgetMints(transactionID uuid.UUID) {
	fmt.Printf("ForgetMints: Forgetting the mints of transaction ID %s\n", transactionID.String())
	if outputStates, ok := s.outputStatesByMinter[transactionID]; ok {
		for _, state := range outputStates {
			fmt.Printf("ForgetMints: Grapher forgetting output state ID %s\n", state.ID.String())
			delete(s.transactionByOutputState, state.ID.String())
		}
		delete(s.outputStatesByMinter, transactionID)
	}
	// Note we specifically don't delete the transaction (i.e. the minter) here. Use Forget() to do both.
}

func (s *grapher) ForgetLocks(transactionID uuid.UUID) {
	fmt.Printf("ForgetLocks: Forgetting the locks of transaction ID %s\n", transactionID.String())
	if locks, ok := s.lockedStatesByTransaction[transactionID]; ok {
		for _, lock := range locks {
			fmt.Printf("ForgetLocks: Grapher forgetting lock for state ID %s\n", lock.State.String())
			delete(s.lockedStatesByTransaction, lock.Transaction)
		}
		delete(s.lockedStatesByTransaction, transactionID)
	}
	// Note we specifically don't delete the transaction (i.e. the minter) here. Use Forget() to do both.
}

func (s *grapher) lockMint(ctx context.Context, stateID pldtypes.HexBytes, transactionID uuid.UUID, lockType pldapi.StateLockType) error {
	fmt.Printf("LockMint: Locking state ID %s for transaction ID %s\n", stateID.String(), transactionID.String())
	if s.lockedStatesByTransaction[transactionID] == nil {
		s.lockedStatesByTransaction[transactionID] = []*stateLock{
			&stateLock{
				State:       stateID,
				Transaction: transactionID,
				Type:        pldapi.StateLockTypeCreate.Enum(),
			},
		}
	} else {
		s.lockedStatesByTransaction[transactionID] = append(s.lockedStatesByTransaction[transactionID], &stateLock{
			State:       stateID,
			Transaction: transactionID,
			Type:        pldapi.StateLockTypeCreate.Enum(),
		})
	}

	return nil
}

func (s *grapher) LockMintOnCreate(ctx context.Context, stateID pldtypes.HexBytes, transactionID uuid.UUID) error {
	return s.lockMint(ctx, stateID, transactionID, pldapi.StateLockTypeCreate)
}

func (s *grapher) LockMintOnSpend(ctx context.Context, stateID pldtypes.HexBytes, transactionID uuid.UUID) error {
	return s.lockMint(ctx, stateID, transactionID, pldapi.StateLockTypeSpend)
}

func (s *grapher) LookupMinter(ctx context.Context, stateID pldtypes.HexBytes) (*Transaction, error) {
	return s.transactionByOutputState[stateID.String()], nil
}

func (s *grapher) TransactionByID(ctx context.Context, transactionID uuid.UUID) *Transaction {
	return s.transactionByID[transactionID]
}

// SortTransactions sorts the transactions based on their dependencies.
// It ensures that transactions are sequenced in such a way that all dependencies are resolved before the transaction itself is processed.
// It returns an error if a circular dependency is detected or if any transaction has dependencies that are not in the input list.
// This function is used to ensure that transactions are processed in the correct order, respecting their dependencies.
// It assumes that the transactions are provided in a state where they are ready to be sequenced.
func SortTransactions(ctx context.Context, transactions []*Transaction) ([]*Transaction, error) {
	sortedTransactions := make([]*Transaction, 0, len(transactions))
	// Ensure the returned array is sorted according to the dependency graph

	// continue to loop through all transactions picking off any who have no dependencies
	//  other than transactions that have already been sequenced
	for len(transactions) > 0 {

		//assume we don't find any transaction with no dependencies in this iteration
		found := false

		// Find the next transaction that has no dependencies
		for i, txn := range transactions {
			// If the transaction has no dependencies, we can sequence it
			if !txn.hasDependenciesNotIn(ctx, sortedTransactions) {
				// Add the transaction to the sequenced transactions
				sortedTransactions = append(sortedTransactions, txn)

				// Remove the transaction from the transactions array
				transactions = append(transactions[:i], transactions[i+1:]...)
				found = true
				break // Restart the loop to check for more transactions
			}
		}
		if !found {
			// If we didn't find any transaction with no dependencies, it means we have a circular dependency
			// or some transactions are not in the input list, which should not happen in normal usage
			msg := "Circular dependency detected or some transactions are not in the input list"
			log.L(ctx).Error(msg)
			return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}
	return sortedTransactions, nil
}
