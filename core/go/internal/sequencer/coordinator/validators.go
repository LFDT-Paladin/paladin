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

// validator_IsStateTransitionToReadyToDispatch is an event validator that checks if the event is a transition to Ready_For_Dispatch
func validator_IsTransitionToReadyToDispatch(_ context.Context, _ coordinatorStateReader, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) (bool, error) {
	transactionStateTransitionEvent := event.(*common.TransactionStateTransitionEvent)
	return transaction.State(transactionStateTransitionEvent.To) == transaction.State_Ready_For_Dispatch, nil
}

// validator_IsTransitionToPooled checks if the event is a transition to State_Pooled
func validator_IsTransitionToPooled(_ context.Context, _ coordinatorStateReader, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) (bool, error) {
	transactionStateTransitionEvent := event.(*common.TransactionStateTransitionEvent)
	return transaction.State(transactionStateTransitionEvent.To) == transaction.State_Pooled, nil
}

// validator_IsTransitionToFinal checks if the event is a transition to State_Final
func validator_IsTransitionToFinal(_ context.Context, _ coordinatorStateReader, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) (bool, error) {
	transactionStateTransitionEvent := event.(*common.TransactionStateTransitionEvent)
	return transaction.State(transactionStateTransitionEvent.To) == transaction.State_Final, nil
}

// validator_IsTransitionFromAssembling checks if the event is a transition from State_Assembling
func validator_IsTransitionFromAssembling(_ context.Context, _ coordinatorStateReader, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) (bool, error) {
	transactionStateTransitionEvent := event.(*common.TransactionStateTransitionEvent)
	return transaction.State(transactionStateTransitionEvent.From) == transaction.State_Assembling, nil
}
