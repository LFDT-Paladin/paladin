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

package originator

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_action_ActiveCoordinatorUpdated_Success(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _ := builder.Build(ctx)
	defer o.Stop()

	err := o.stateMachineEventLoop.ProcessEvent(ctx, &ActiveCoordinatorUpdatedEvent{Coordinator: "node1"})
	require.NoError(t, err)
	assert.Equal(t, "node1", o.GetCurrentCoordinator())
}

func Test_action_ActiveCoordinatorUpdated_EmptyCoordinatorReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _ := builder.Build(ctx)
	defer o.Stop()

	err := o.stateMachineEventLoop.ProcessEvent(ctx, &ActiveCoordinatorUpdatedEvent{Coordinator: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cannot set active coordinator to an empty string")
}

func Test_guard_HasDroppedTransactions_TrueWhenDelegatedTxnNotInSnapshot(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated).Build()
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, coordinatorLocator).Transactions(txn)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	o.latestCoordinatorSnapshot = &common.CoordinatorSnapshot{
		PooledTransactions: []*common.Transaction{},
	}

	assert.True(t, guard_HasDroppedTransactions(ctx, o))
}

func Test_guard_HasDroppedTransactions_FalseWhenDelegatedTxnInSnapshot(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated).Build()
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, coordinatorLocator).Transactions(txn)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	o.latestCoordinatorSnapshot = &common.CoordinatorSnapshot{
		PooledTransactions: []*common.Transaction{
			{ID: txn.GetID(), Originator: originatorLocator},
		},
	}

	assert.False(t, guard_HasDroppedTransactions(ctx, o))
}

func Test_sendDelegationRequest_NoActiveCoordinatorReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _ := builder.Build(ctx)
	defer o.Stop()

	o.activeCoordinatorNode = ""
	err := o.stateMachineEventLoop.ProcessEvent(ctx, &DelegateTimeoutEvent{})
	require.NoError(t, err)
}

func Test_validator_TransactionDoesNotExist_InvalidEventTypeReturnsFalse(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _ := builder.Build(ctx)
	defer o.Stop()

	valid, err := validator_TransactionDoesNotExist(ctx, o, &HeartbeatReceivedEvent{})
	assert.NoError(t, err)
	assert.False(t, valid)
}

func Test_validator_TransactionDoesNotExist_NilTransactionReturnsTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _ := builder.Build(ctx)
	defer o.Stop()

	valid, err := validator_TransactionDoesNotExist(ctx, o, &TransactionCreatedEvent{Transaction: nil})
	assert.NoError(t, err)
	assert.True(t, valid)
}

func Test_validator_TransactionDoesNotExist_TransactionAlreadyExistsReturnsFalse(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pending).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator).Transactions(txn)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	require.NotNil(t, o.transactionsByID[txn.GetID()])

	valid, err := validator_TransactionDoesNotExist(ctx, o, &TransactionCreatedEvent{
		Transaction: txn.GetPrivateTransaction(),
	})
	assert.NoError(t, err)
	assert.False(t, valid)
}

func Test_validator_TransactionDoesNotExist_NewTransactionReturnsTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _ := builder.Build(ctx)
	defer o.Stop()

	transactionBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pending)
	pt := transactionBuilder.Build().GetPrivateTransaction()
	valid, err := validator_TransactionDoesNotExist(ctx, o, &TransactionCreatedEvent{Transaction: pt})
	assert.NoError(t, err)
	assert.True(t, valid)
}
