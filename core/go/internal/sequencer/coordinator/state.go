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

package coordinator

import (
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/google/uuid"
)

// coordinatorState holds the mutable state that can only be modified by StateUpdate functions
// This struct is protected by the state mutex
type coordinatorState struct {
	statemachine.StateMachineState[State]

	// Active coordinator tracking
	activeCoordinatorNode        string
	activeCoordinatorBlockHeight uint64

	// Heartbeat tracking
	heartbeatIntervalsSinceStateChange int

	// Transaction tracking
	transactionsByID map[uuid.UUID]*transaction.Transaction

	// Block tracking
	currentBlockHeight uint64

	// Flush points from active coordinator (for handover)
	activeCoordinatorsFlushPointsBySignerNonce map[string]*common.FlushPoint

	// Originator node pool
	originatorNodePool []string

	// A coordinator can have at most one assembling transaction at a time
	assemblingTransaction *transaction.Transaction

	// Pool of transactions that are waiting to be assembled - currently ordered FIFO
	pooledTransactions []*transaction.Transaction
}

// newCoordinatorState creates a new coordinator state with initialized maps
func newCoordinatorState() *coordinatorState {
	return &coordinatorState{
		transactionsByID: make(map[uuid.UUID]*transaction.Transaction),
		activeCoordinatorsFlushPointsBySignerNonce: make(map[string]*common.FlushPoint),
		originatorNodePool:                         make([]string, 0),
	}
}

// coordinatorStateReader provides read-only access to coordinator state for guards and actions
type coordinatorStateReader interface {
	statemachine.StateReader[State]

	// Active coordinator
	GetActiveCoordinatorNode() string
	GetActiveCoordinatorBlockHeight() uint64

	// Heartbeat tracking
	GetHeartbeatIntervalsSinceStateChange() int

	// Transaction access
	GetTransactionByID(id uuid.UUID) *transaction.Transaction
	GetAllTransactions() map[uuid.UUID]*transaction.Transaction
	GetTransactionCount() int

	// Block tracking
	GetCurrentBlockHeight() uint64

	// Flush points
	GetFlushPointBySignerNonce(signerNonce string) *common.FlushPoint
	GetAllFlushPoints() map[string]*common.FlushPoint

	// Originator node pool
	GetOriginatorNodePool() []string

	// Assembling transaction
	GetAssemblingTransaction() *transaction.Transaction

	// Read locks for the state
	RLock()
	RUnlock()
}

// Implement coordinatorStateReader for coordinatorState
// TODO: consider making all these return copies of the data structures to avoid unintentionally modifying the state

func (s *coordinatorState) GetActiveCoordinatorNode() string {
	return s.activeCoordinatorNode
}

func (s *coordinatorState) GetActiveCoordinatorBlockHeight() uint64 {
	return s.activeCoordinatorBlockHeight
}

func (s *coordinatorState) GetHeartbeatIntervalsSinceStateChange() int {
	return s.heartbeatIntervalsSinceStateChange
}

func (s *coordinatorState) GetTransactionByID(id uuid.UUID) *transaction.Transaction {
	return s.transactionsByID[id]
}

func (s *coordinatorState) GetAllTransactions() map[uuid.UUID]*transaction.Transaction {
	return s.transactionsByID
}

func (s *coordinatorState) GetTransactionCount() int {
	return len(s.transactionsByID)
}

func (s *coordinatorState) GetCurrentBlockHeight() uint64 {
	return s.currentBlockHeight
}

func (s *coordinatorState) GetFlushPointBySignerNonce(signerNonce string) *common.FlushPoint {
	return s.activeCoordinatorsFlushPointsBySignerNonce[signerNonce]
}

func (s *coordinatorState) GetAllFlushPoints() map[string]*common.FlushPoint {
	return s.activeCoordinatorsFlushPointsBySignerNonce
}

func (s *coordinatorState) GetOriginatorNodePool() []string {
	return s.originatorNodePool
}

func (s *coordinatorState) GetAssemblingTransaction() *transaction.Transaction {
	return s.assemblingTransaction
}

// Pooled transactions
func (s *coordinatorState) GetPooledTransactions() []*transaction.Transaction {
	return s.pooledTransactions
}

// Setters for StateUpdate functions only

func (s *coordinatorState) SetActiveCoordinatorNode(node string) {
	s.activeCoordinatorNode = node
}

func (s *coordinatorState) SetActiveCoordinatorBlockHeight(height uint64) {
	s.activeCoordinatorBlockHeight = height
}

func (s *coordinatorState) SetHeartbeatIntervalsSinceStateChange(count int) {
	s.heartbeatIntervalsSinceStateChange = count
}

func (s *coordinatorState) IncrementHeartbeatIntervalsSinceStateChange() {
	s.heartbeatIntervalsSinceStateChange++
}

func (s *coordinatorState) SetCurrentBlockHeight(height uint64) {
	s.currentBlockHeight = height
}

func (s *coordinatorState) AddTransaction(id uuid.UUID, txn *transaction.Transaction) {
	s.transactionsByID[id] = txn
}

func (s *coordinatorState) DeleteTransaction(id uuid.UUID) {
	delete(s.transactionsByID, id)
}

func (s *coordinatorState) SetFlushPoint(signerNonce string, flushPoint *common.FlushPoint) {
	s.activeCoordinatorsFlushPointsBySignerNonce[signerNonce] = flushPoint
}

func (s *coordinatorState) SetOriginatorNodePool(pool []string) {
	s.originatorNodePool = pool
}

func (s *coordinatorState) SetAssemblingTransaction(txn *transaction.Transaction) {
	s.assemblingTransaction = txn
}

func (s *coordinatorState) ClearAssemblingTransaction() {
	s.assemblingTransaction = nil
}

func (s *coordinatorState) AddPooledTransaction(txn *transaction.Transaction) {
	// check if the transaction is already in the pool
	for _, t := range s.pooledTransactions {
		if t.ID == txn.ID {
			return
		}
	}
	s.pooledTransactions = append(s.pooledTransactions, txn)
}

func (s *coordinatorState) PopNextPooledTransaction() *transaction.Transaction {
	if len(s.pooledTransactions) == 0 {
		return nil
	}
	nextPooledTx := s.pooledTransactions[0]
	s.pooledTransactions = s.pooledTransactions[1:]
	return nextPooledTx
}
