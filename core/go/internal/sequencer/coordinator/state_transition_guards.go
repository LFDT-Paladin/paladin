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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
)

// Guard type is defined as a type alias in state_machine.go using statemachine.Guard[*coordinator]

func guard_HasUnconfirmedDispatchedTransactions(ctx context.Context, c *coordinator) bool {
	return len(
		c.getTransactionsInStates(ctx, []transaction.State{
			transaction.State_Dispatched,
		}),
	) > 0
}

// Function noTransactionsInflight returns true if all transactions that have been delegated to this coordinator have been confirmed/reverted
// and since removed from memory
func guard_HasTransactionsInflight(_ context.Context, c *coordinator) bool {
	return len(c.transactionsByID) > 0
}

func guard_ClosingGracePeriodExpired(_ context.Context, c *coordinator) bool {
	// measure number of complete heartbeat interval periods - e.g. count of 2 means
	// 1 full heartbeat interval has elapsed, hence use of > not >=
	return c.heartbeatIntervalsSinceStateChange > c.closingGracePeriod
}

func guard_HasTransactionAssembling(_ context.Context, c *coordinator) bool {
	return c.assemblyInFlight
}

// guard_InactiveGracePeriodExceeded returns true when no heartbeat has been received for at least
// inactiveGracePeriod heartbeat intervals.
func guard_InactiveGracePeriodExceeded(_ context.Context, c *coordinator) bool {
	// measure number of complete heartbeat interval periods - e.g. count of 2 means
	// 1 full heartbeat interval has elapsed, hence use of > not >=
	return c.heartbeatIntervalsSinceLastReceive > c.inactiveGracePeriod
}

// guard_IsHigherPriorityThanCurrentActive returns true when this node has a strictly higher
// priority (lower index) than the current active coordinator in the coordinator priority list.
// Used in Observing and Closing when deciding whether to initiate a handover.
func guard_IsHigherPriorityThanCurrentActive(_ context.Context, c *coordinator) bool {
	return common.IsHigherPriority(c.coordinatorPriorityList, c.nodeName, c.currentActiveCoordinator)
}

// Guards are typically combined using the statemachine combination functions so that the state machine definition
// can read like a spec; however, this particular check self describes better by being combined
// into a new single guard function.
func guard_MustFlushToRotateSigningIdentity(ctx context.Context, c *coordinator) bool {
	return c.signingIdentity.used && guard_HasUnconfirmedDispatchedTransactions(ctx, c)
}
