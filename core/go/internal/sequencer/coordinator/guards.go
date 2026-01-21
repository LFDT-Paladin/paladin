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

package coordinator

import (
	"context"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// Guard type is defined in state_machine.go as a type alias to statemachine.Guard[coordinatorStateReader, *coordinatorContext]

func guard_Behind(_ context.Context, reader coordinatorStateReader, config *stateMachineConfig) bool {
	//Return true if the current block height that our indexer has reached is behind the current coordinator
	// there is a configured tolerance so if we are within this tolerance we are not considered behind
	return reader.GetCurrentBlockHeight() < reader.GetActiveCoordinatorBlockHeight()-config.blockHeightTolerance
}

func guard_ActiveCoordinatorFlushComplete(_ context.Context, reader coordinatorStateReader, _ *stateMachineConfig) bool {
	for _, flushPoint := range reader.GetAllFlushPoints() {
		if !flushPoint.Confirmed {
			return false
		}
	}
	return true
}

// Function flushComplete returns true if there are no transactions past the point of no return that haven't been confirmed yet
// TODO: does considering the flush complete while there might be transactions in terminal states (State_Confirmed/State reverted)
// waiting for the grace period to expire before being cleaned result in a memory leak? N.B. There is currently no heartbeat handling in State_Flush
func guard_FlushComplete(_ context.Context, reader coordinatorStateReader, _ *stateMachineConfig) bool {
	return len(
		getTransactionsInStatesFromReader(reader, []transaction.State{
			transaction.State_Ready_For_Dispatch,
			transaction.State_Dispatched,
			transaction.State_Submitted,
		}),
	) == 0
}

// Function noTransactionsInflight returns true if all transactions that have been delegated to this coordinator have been confirmed/reverted
// and since removed from memory
// TODO AM: this is the one that needs renaming
func guard_HasTransactionsInflight(_ context.Context, reader coordinatorStateReader, _ *stateMachineConfig) bool {
	return reader.GetTransactionCount() > 0
}

func guard_ClosingGracePeriodExpired(_ context.Context, reader coordinatorStateReader, config *stateMachineConfig) bool {
	return reader.GetHeartbeatIntervalsSinceStateChange() >= config.closingGracePeriod
}

func guard_HasTransactionAssembling(_ context.Context, reader coordinatorStateReader, _ *stateMachineConfig) bool {
	return reader.GetAssemblingTransaction() != nil
}

func guard_ShouldHeartbeat(_ context.Context, _ coordinatorStateReader, config *stateMachineConfig) bool {
	// For domain types that can coordinate other nodes' transactions (e.g. Noto or Pente), start heartbeating
	// Domains such as Zeto that are always coordinated on the originating node, heartbeats aren't required
	// because other nodes cannot take over coordination.
	return config.coordinatorSelection != prototk.ContractConfig_COORDINATOR_SENDER
}

// Helper function for guards that need to get transactions in states
func getTransactionsInStatesFromReader(reader coordinatorStateReader, states []transaction.State) []*transaction.Transaction {
	matchingStates := make(map[transaction.State]bool)
	for _, s := range states {
		matchingStates[s] = true
	}

	allTxns := reader.GetAllTransactions()
	matchingTxns := make([]*transaction.Transaction, 0, len(allTxns))
	for _, txn := range allTxns {
		if matchingStates[txn.GetState()] {
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}
