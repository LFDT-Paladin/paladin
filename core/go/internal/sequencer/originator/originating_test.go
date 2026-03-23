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
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_ActiveCoordinatorUpdated_EmptyCoordinatorReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	err := o.stateMachineEventLoop.ProcessEvent(ctx, &ActiveCoordinatorUpdatedEvent{Coordinator: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cannot set active coordinator to an empty string")
}

func Test_action_ActiveCoordinatorUpdated_SetsActiveCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	coordinator := "new-coordinator@node2"
	err := action_ActiveCoordinatorUpdated(ctx, o, &ActiveCoordinatorUpdatedEvent{Coordinator: coordinator})
	require.NoError(t, err)
	assert.Equal(t, coordinator, o.activeCoordinatorNode)
}

func Test_guard_HasDroppedTransactions_TrueWhenDelegatedTxnNotInSnapshot(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, coordinatorLocator).TransactionBuilders(txBuilder)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()
	txn := txBuilder.GetBuiltTransaction()
	require.NotNil(t, txn)

	o.latestCoordinatorSnapshot = &common.CoordinatorSnapshot{
		PooledTransactions: []*common.SnapshotPooledTransaction{},
	}

	assert.True(t, guard_HasDroppedTransactions(ctx, o))
}

func Test_guard_HasDroppedTransactions_FalseWhenDelegatedTxnInSnapshot(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, coordinatorLocator).TransactionBuilders(txBuilder)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()
	txn := txBuilder.GetBuiltTransaction()
	require.NotNil(t, txn)

	o.latestCoordinatorSnapshot = &common.CoordinatorSnapshot{
		PooledTransactions: []*common.SnapshotPooledTransaction{
			{ID: txn.GetID(), Originator: originatorLocator},
		},
	}

	assert.False(t, guard_HasDroppedTransactions(ctx, o))
}

func Test_transactionFoundInHeartbeat_TrueWhenInDispatchedTransactions(t *testing.T) {
	originatorLocator := "sender@senderNode"
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, "coordinator@coordinatorNode").TransactionBuilders(txBuilder)
	o, _, cleanup := builder.Build(context.Background())
	defer cleanup()
	txn := txBuilder.GetBuiltTransaction()
	require.NotNil(t, txn)

	o.latestCoordinatorSnapshot = &common.CoordinatorSnapshot{
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{
			{SnapshotPooledTransaction: common.SnapshotPooledTransaction{ID: txn.GetID(), Originator: originatorLocator}},
		},
		PooledTransactions:    []*common.SnapshotPooledTransaction{},
		ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{},
	}

	assert.True(t, transactionFoundInHeartbeat(o, txn))
}

func Test_transactionFoundInHeartbeat_TrueWhenInPooledTransactions(t *testing.T) {
	originatorLocator := "sender@senderNode"
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, "coordinator@coordinatorNode").TransactionBuilders(txBuilder)
	o, _, cleanup := builder.Build(context.Background())
	defer cleanup()
	txn := txBuilder.GetBuiltTransaction()
	require.NotNil(t, txn)

	o.latestCoordinatorSnapshot = &common.CoordinatorSnapshot{
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{},
		PooledTransactions: []*common.SnapshotPooledTransaction{
			{ID: txn.GetID(), Originator: originatorLocator},
		},
		ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{},
	}

	assert.True(t, transactionFoundInHeartbeat(o, txn))
}

func Test_transactionFoundInHeartbeat_TrueWhenInConfirmedTransactions(t *testing.T) {
	originatorLocator := "sender@senderNode"
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, "coordinator@coordinatorNode").TransactionBuilders(txBuilder)
	o, _, cleanup := builder.Build(context.Background())
	defer cleanup()
	txn := txBuilder.GetBuiltTransaction()
	require.NotNil(t, txn)

	o.latestCoordinatorSnapshot = &common.CoordinatorSnapshot{
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{},
		PooledTransactions:     []*common.SnapshotPooledTransaction{},
		ConfirmedTransactions: []*common.SnapshotConfirmedTransaction{
			{SnapshotDispatchedTransaction: common.SnapshotDispatchedTransaction{
				SnapshotPooledTransaction: common.SnapshotPooledTransaction{ID: txn.GetID(), Originator: originatorLocator},
			}},
		},
	}

	assert.True(t, transactionFoundInHeartbeat(o, txn))
}

func Test_transactionFoundInHeartbeat_FalseWhenNotInSnapshot(t *testing.T) {
	originatorLocator := "sender@senderNode"
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, "coordinator@coordinatorNode").TransactionBuilders(txBuilder)
	o, _, cleanup := builder.Build(context.Background())
	defer cleanup()
	txn := txBuilder.GetBuiltTransaction()
	require.NotNil(t, txn)

	o.latestCoordinatorSnapshot = &common.CoordinatorSnapshot{
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{},
		PooledTransactions:     []*common.SnapshotPooledTransaction{},
		ConfirmedTransactions:  []*common.SnapshotConfirmedTransaction{},
	}

	assert.False(t, transactionFoundInHeartbeat(o, txn))
}

func Test_transactionFoundInHeartbeat_FalseWhenOnlyOtherTxnsInSnapshot(t *testing.T) {
	originatorLocator := "sender@senderNode"
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers(originatorLocator, "coordinator@coordinatorNode").TransactionBuilders(txBuilder)
	o, _, cleanup := builder.Build(context.Background())
	defer cleanup()
	txn := txBuilder.GetBuiltTransaction()
	require.NotNil(t, txn)

	otherID := uuid.New()
	o.latestCoordinatorSnapshot = &common.CoordinatorSnapshot{
		PooledTransactions: []*common.SnapshotPooledTransaction{
			{ID: otherID, Originator: originatorLocator},
		},
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{},
		ConfirmedTransactions:  []*common.SnapshotConfirmedTransaction{},
	}

	assert.False(t, transactionFoundInHeartbeat(o, txn))
}

func Test_sendDelegationRequest_NoActiveCoordinatorDefersAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"

	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pending)
	builder := NewOriginatorBuilderForTesting(State_Sending).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		TransactionBuilders(txBuilder)
	o, mocks, cleanup := builder.Build(ctx)
	defer cleanup()

	o.activeCoordinatorNode = ""

	err := sendDelegationRequest(ctx, o)
	require.NoError(t, err)
	assert.False(t, mocks.SentMessageRecorder.HasSentDelegationRequest(),
		"delegation request should not be sent when there is no active coordinator")
}

type mockOriginatorTransactionForDelegation struct {
	id        uuid.UUID
	pt        *components.PrivateTransaction
	handleErr error
}

func (m *mockOriginatorTransactionForDelegation) HandleEvent(_ context.Context, _ common.Event) error {
	return m.handleErr
}
func (m *mockOriginatorTransactionForDelegation) GetID() uuid.UUID           { return m.id }
func (m *mockOriginatorTransactionForDelegation) GetAssembleErrorCount() int { return 0 }
func (m *mockOriginatorTransactionForDelegation) GetPrivateTransaction() *components.PrivateTransaction {
	return m.pt
}
func (m *mockOriginatorTransactionForDelegation) GetCurrentState() transaction.State {
	return transaction.State_Pending
}
func (m *mockOriginatorTransactionForDelegation) GetStatus(_ context.Context) components.PrivateTxStatus {
	return components.PrivateTxStatus{TxID: m.id.String(), Status: "pending"}
}

func Test_addToTransactions_HandleCreatedEventError_ReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	txnID := uuid.New()
	pt := &components.PrivateTransaction{ID: txnID}
	expectedErr := fmt.Errorf("created event handling failed")
	mockTxn := &mockOriginatorTransactionForDelegation{
		id:        txnID,
		pt:        pt,
		handleErr: expectedErr,
	}

	createTransaction := func(context.Context, *components.PrivateTransaction) (transaction.OriginatorTransaction, error) {
		return mockTxn, nil
	}

	err := o.addToTransactions(ctx, pt, createTransaction)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func Test_sendDelegationRequest_HandleEventError_ReturnsWrappedError(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	// Build with no transactions, then inject a mock that fails on HandleEvent
	txnID := uuid.New()
	expectedErr := fmt.Errorf("delegated event handling failed")
	mockTxn := &mockOriginatorTransactionForDelegation{
		id:        txnID,
		pt:        &components.PrivateTransaction{ID: txnID},
		handleErr: expectedErr,
	}
	o.transactionsOrdered = []transaction.OriginatorTransaction{mockTxn}
	o.transactionsByID[txnID] = mockTxn
	o.activeCoordinatorNode = "coordinator@coordinatorNode"

	err := sendDelegationRequest(ctx, o)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error handling delegated event for transaction")
	assert.Contains(t, err.Error(), txnID.String())
	assert.Contains(t, err.Error(), expectedErr.Error())
}

func Test_validator_TransactionDoesNotExist_InvalidEventTypeReturnsFalse(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	valid, err := validator_TransactionDoesNotExist(ctx, o, &HeartbeatReceivedEvent{})
	assert.NoError(t, err)
	assert.False(t, valid)
}

func Test_validator_TransactionDoesNotExist_NilTransactionReturnsTrue(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	valid, err := validator_TransactionDoesNotExist(ctx, o, &TransactionCreatedEvent{Transaction: nil})
	assert.NoError(t, err)
	assert.True(t, valid)
}

func Test_validator_TransactionDoesNotExist_TransactionAlreadyExistsReturnsFalse(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pending)
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator).TransactionBuilders(txBuilder)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()
	txn := txBuilder.GetBuiltTransaction()
	require.NotNil(t, txn)

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
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	transactionBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pending)
	pt := transactionBuilder.Build().GetPrivateTransaction()
	valid, err := validator_TransactionDoesNotExist(ctx, o, &TransactionCreatedEvent{Transaction: pt})
	assert.NoError(t, err)
	assert.True(t, valid)
}

func Test_validator_OriginatorTransactionStateTransitionToFinal(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_OriginatorTransactionStateTransitionToFinal(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Final})
	require.NoError(t, err)
	assert.True(t, valid)

	valid, err = validator_OriginatorTransactionStateTransitionToFinal(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Confirmed})
	require.NoError(t, err)
	assert.False(t, valid)
}

func Test_validator_OriginatorTransactionStateTransitionToConfirmed(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_OriginatorTransactionStateTransitionToConfirmed(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Confirmed})
	require.NoError(t, err)
	assert.True(t, valid)

	valid, err = validator_OriginatorTransactionStateTransitionToConfirmed(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Reverted})
	require.NoError(t, err)
	assert.False(t, valid)
}

func Test_validator_OriginatorTransactionStateTransitionToReverted(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_OriginatorTransactionStateTransitionToReverted(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Reverted})
	require.NoError(t, err)
	assert.True(t, valid)

	valid, err = validator_OriginatorTransactionStateTransitionToReverted(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Final})
	require.NoError(t, err)
	assert.False(t, valid)
}

func Test_guard_RedelegateThresholdExceeded_TrueWhenNoHeartbeatReceived(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Sending).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	o.timeOfMostRecentHeartbeat = nil

	assert.True(t, guard_RedelegateThresholdExceeded(ctx, o))
}

func Test_guard_RedelegateThresholdExceeded_TrueWhenClockExpired(t *testing.T) {
	ctx := context.Background()
	mockClock := common.NewMockClock(t)
	mockClock.EXPECT().HasExpired(mock.Anything, mock.Anything).Return(true)

	builder := NewOriginatorBuilderForTesting(State_Sending).
		CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode").
		Clock(mockClock)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	now := time.Now()
	o.timeOfMostRecentHeartbeat = &now

	assert.True(t, guard_RedelegateThresholdExceeded(ctx, o))
}

func Test_guard_RedelegateThresholdExceeded_FalseWhenClockNotExpired(t *testing.T) {
	ctx := context.Background()
	mockClock := common.NewMockClock(t)
	mockClock.EXPECT().HasExpired(mock.Anything, mock.Anything).Return(false)

	builder := NewOriginatorBuilderForTesting(State_Sending).
		CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode").
		Clock(mockClock)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	now := time.Now()
	o.timeOfMostRecentHeartbeat = &now

	assert.False(t, guard_RedelegateThresholdExceeded(ctx, o))
}
