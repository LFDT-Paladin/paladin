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
	"context"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/coordinatortransactionmocks"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newMockCoordinatorTransactionWithOriginatorNode creates a mock CoordinatorTransaction that
// returns the given originatorNode from GetOriginatorNode(). GetID() returns a random UUID.
// Other interface methods are not stubbed; add expectations as needed in specific tests.
func newMockCoordinatorTransactionWithOriginatorNode(t *testing.T, originatorNode string) *coordinatortransactionmocks.CoordinatorTransaction {
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.On("GetID").Return(uuid.New()).Maybe()
	txn.On("GetOriginatorNode").Return(originatorNode).Maybe()
	txn.On("GetCurrentState").Return(transaction.State_Pooled).Maybe()
	return txn
}

func Test_addToDelegatedTransactions_NewTransactionError_ReturnsError(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build()
	validOriginator := "sender@senderNode"
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(validOriginator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	invalidOriginator := "sender@node1@node2"
	err := c.addToDelegatedTransactions(ctx, invalidOriginator, []*components.PrivateTransaction{txn}, "", c.newCoordinatorTransaction)

	require.Error(t, err, "should return error when NewTransaction fails")
	assert.Equal(t, 0, len(c.transactionsByID), "transaction should not be added when NewTransaction fails")
}

func Test_addToDelegatedTransactions_AddsTransactionInPreDispatchFlowState(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "", c.newCoordinatorTransaction)

	require.NoError(t, err, "should add delegated transaction")
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	coordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")
	assert.Contains(t, []transaction.State{
		transaction.State_Pooled,
		transaction.State_PreAssembly_Blocked,
		transaction.State_Assembling,
	}, coordinatedTxn.GetCurrentState(), "transaction should start in pre-dispatch flow states")
}

func Test_addToDelegatedTransactions_AddsTransactionInPooledFlowState(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "", c.newCoordinatorTransaction)

	require.NoError(t, err, "should add delegated transaction")
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	coordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")
	assert.NotEqual(t, transaction.State_Dispatched, coordinatedTxn.GetCurrentState(), "transaction should not start in State_Dispatched")
	assert.Contains(t, []transaction.State{transaction.State_Pooled, transaction.State_PreAssembly_Blocked, transaction.State_Assembling}, coordinatedTxn.GetCurrentState(), "transaction should be in pooled flow states")
}

func Test_addToDelegatedTransactions_DuplicateTransaction_SkipsAndReturnsNoError(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "", c.newCoordinatorTransaction)
	require.NoError(t, err, "should not return error on first add")
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	firstCoordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, firstCoordinatedTxn, "transaction should exist in transactionsByID")

	err = c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "", c.newCoordinatorTransaction)
	require.NoError(t, err, "should not return error when adding duplicate transaction")
	assert.Equal(t, 1, len(c.transactionsByID), "duplicate transaction should be skipped, count should remain 1")
	secondCoordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, secondCoordinatedTxn, "transaction should still exist in transactionsByID")
	assert.Equal(t, firstCoordinatedTxn, secondCoordinatedTxn, "duplicate transaction should not replace existing transaction")
}

func Test_coordinatorTransactionHandleEvent_TxnNotFound_ReturnsError(t *testing.T) {
	ctx := t.Context()
	txID := uuid.New()
	c := &coordinator{
		transactionsByID: map[uuid.UUID]transaction.CoordinatorTransaction{},
	}

	err := c.coordinatorTransactionHandleEvent(ctx, txID, &transaction.SelectedEvent{})
	require.Error(t, err)
	assert.ErrorContains(t, err, txID.String())
}

func Test_coordinatorTransactionHandleEvent_DelegatesToTransaction(t *testing.T) {
	ctx := t.Context()
	txID := uuid.New()
	handleErr := fmt.Errorf("handle failed")
	mockTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	mockTxn.EXPECT().HandleEvent(ctx, mock.Anything).Return(handleErr).Once()
	c := &coordinator{
		transactionsByID: map[uuid.UUID]transaction.CoordinatorTransaction{
			txID: mockTxn,
		},
	}

	err := c.coordinatorTransactionHandleEvent(ctx, txID, &transaction.SelectedEvent{})
	require.Error(t, err)
	assert.ErrorIs(t, err, handleErr)
}

func Test_getCoordinatorTransactionState_TxnNotFound_ReturnsFalse(t *testing.T) {
	ctx := t.Context()
	c := &coordinator{
		transactionsByID: map[uuid.UUID]transaction.CoordinatorTransaction{},
	}

	state, ok := c.getCoordinatorTransactionState(ctx, uuid.New())
	assert.False(t, ok)
	assert.Equal(t, transaction.State(0), state)
}

func Test_getCoordinatorTransactionState_TxnFound_ReturnsState(t *testing.T) {
	ctx := t.Context()
	txID := uuid.New()
	mockTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	mockTxn.EXPECT().GetCurrentState().Return(transaction.State_Confirming_Dispatchable).Once()
	c := &coordinator{
		transactionsByID: map[uuid.UUID]transaction.CoordinatorTransaction{
			txID: mockTxn,
		},
	}

	state, ok := c.getCoordinatorTransactionState(ctx, txID)
	assert.True(t, ok)
	assert.Equal(t, transaction.State_Confirming_Dispatchable, state)
}

func Test_addTransactionToBackOfPool_WhenNotInPool_Appends(t *testing.T) {
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)

	c.addTransactionToBackOfPool(txn)

	require.Len(t, c.pooledTransactions, 1, "pool should contain one transaction")
	assert.Equal(t, txn, c.pooledTransactions[0])
}

func Test_addTransactionToBackOfPool_WhenAlreadyInPool_DoesNotDuplicate(t *testing.T) {
	txID := uuid.New()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.EXPECT().GetID().Return(txID)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txn).Build()

	c.addTransactionToBackOfPool(txn)
	c.addTransactionToBackOfPool(txn)

	require.Len(t, c.pooledTransactions, 1, "pool should not duplicate transaction")
	assert.Equal(t, txn, c.pooledTransactions[0])
}

func Test_action_PoolTransaction(t *testing.T) {
	txID := uuid.New()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.EXPECT().GetID().Return(txID)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txn).Build()

	err := action_PoolTransaction(t.Context(), c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Pooled,
	})
	require.NoError(t, err)
	require.Len(t, c.pooledTransactions, 1, "transaction should be added to pool")
	assert.Equal(t, txn, c.pooledTransactions[0])
}

func Test_action_QueueTransactionForDispatch(t *testing.T) {
	txID := uuid.New()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.EXPECT().GetID().Return(txID)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txn).Build()
	err := action_QueueTransactionForDispatch(t.Context(), c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Ready_For_Dispatch,
	})
	require.NoError(t, err)
}

func Test_action_CleanUpTransaction_RemovesFromMap(t *testing.T) {
	txID := uuid.New()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.EXPECT().GetID().Return(txID)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txn).Build()
	err := action_CleanUpTransaction(t.Context(), c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Final,
	})
	require.NoError(t, err)
	_, ok := c.transactionsByID[txID]
	assert.False(t, ok, "transaction should be removed from map")
}

func Test_action_CleanUpTransaction_RemovesFromPool(t *testing.T) {
	txID := uuid.New()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.EXPECT().GetID().Return(txID)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txn).Build()
	c.addTransactionToBackOfPool(txn)
	require.Len(t, c.pooledTransactions, 1)
	err := action_CleanUpTransaction(t.Context(), c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Evicted,
	})
	require.NoError(t, err)
	assert.Empty(t, c.pooledTransactions, "transaction should be removed from pool on cleanup")
}

func Test_removeTransactionFromPool_RemovesMatchingTransaction(t *testing.T) {
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()

	txn1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID1 := uuid.New()
	txn1.EXPECT().GetID().Return(txID1)
	txn2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID2 := uuid.New()
	txn2.EXPECT().GetID().Return(txID2)

	c.addTransactionToBackOfPool(txn1)
	c.addTransactionToBackOfPool(txn2)
	require.Len(t, c.pooledTransactions, 2)

	c.removeTransactionFromPool(txID1)

	require.Len(t, c.pooledTransactions, 1)
	assert.Equal(t, txID2, c.pooledTransactions[0].GetID())
}

func Test_removeTransactionFromPool_NoOpIfNotInPool(t *testing.T) {
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txID := uuid.New()
	txn.EXPECT().GetID().Return(txID)
	c.addTransactionToBackOfPool(txn)
	require.Len(t, c.pooledTransactions, 1)

	c.removeTransactionFromPool(uuid.New())

	require.Len(t, c.pooledTransactions, 1, "pool should be unchanged when removing non-existent ID")
}

func Test_action_CleanUpTransaction_GrapherForgetError_LogsButReturnsNil(t *testing.T) {
	txID := uuid.New()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.EXPECT().GetID().Return(txID)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txn).Build()
	err := action_CleanUpTransaction(t.Context(), c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Final,
	})
	require.NoError(t, err)
	_, ok := c.transactionsByID[txID]
	assert.False(t, ok, "transaction should still be removed from map despite grapher error")
}

func Test_validator_TransactionStateTransitionFrom(t *testing.T) {
	ctx := t.Context()
	testCases := []struct {
		name       string
		states     []transaction.State
		eventFrom  transaction.State
		eventTo    transaction.State
		wantResult bool
	}{
		{
			name:       "matches single from state",
			states:     []transaction.State{transaction.State_Dispatched},
			eventFrom:  transaction.State_Dispatched,
			eventTo:    transaction.State_Pooled,
			wantResult: true,
		},
		{
			name:       "matches any from state in list",
			states:     []transaction.State{transaction.State_Dispatched, transaction.State_Assembling},
			eventFrom:  transaction.State_Assembling,
			eventTo:    transaction.State_Reverted,
			wantResult: true,
		},
		{
			name:       "does not match when from state not in list",
			states:     []transaction.State{transaction.State_Dispatched, transaction.State_Pooled},
			eventFrom:  transaction.State_Initial,
			eventTo:    transaction.State_Pooled,
			wantResult: false,
		},
		{
			name:       "returns false when no states provided",
			states:     nil,
			eventFrom:  transaction.State_Dispatched,
			eventTo:    transaction.State_Pooled,
			wantResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := validator_TransactionStateTransitionFrom(tc.states...)
			valid, err := validator(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{
				FromState: tc.eventFrom,
				ToState:   tc.eventTo,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, valid)
		})
	}
}

func Test_validator_TransactionStateTransitionTo(t *testing.T) {
	ctx := t.Context()
	testCases := []struct {
		name       string
		states     []transaction.State
		eventFrom  transaction.State
		eventTo    transaction.State
		wantResult bool
	}{
		{
			name:       "matches single to state",
			states:     []transaction.State{transaction.State_Pooled},
			eventFrom:  transaction.State_Dispatched,
			eventTo:    transaction.State_Pooled,
			wantResult: true,
		},
		{
			name:       "matches any to state in list",
			states:     []transaction.State{transaction.State_Pooled, transaction.State_Reverted},
			eventFrom:  transaction.State_Dispatched,
			eventTo:    transaction.State_Reverted,
			wantResult: true,
		},
		{
			name:       "does not match when to state not in list",
			states:     []transaction.State{transaction.State_Pooled, transaction.State_Ready_For_Dispatch},
			eventFrom:  transaction.State_Dispatched,
			eventTo:    transaction.State_Final,
			wantResult: false,
		},
		{
			name:       "returns false when no states provided",
			states:     nil,
			eventFrom:  transaction.State_Dispatched,
			eventTo:    transaction.State_Pooled,
			wantResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := validator_TransactionStateTransitionTo(tc.states...)
			valid, err := validator(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{
				FromState: tc.eventFrom,
				ToState:   tc.eventTo,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, valid)
		})
	}
}

func Test_addToDelegatedTransactions_WhenMaxInflightReached_ReturnsError(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxInflightTransactions = confutil.P(1)
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	txn1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	txn2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn1}, "", c.newCoordinatorTransaction)
	require.NoError(t, err)
	require.Len(t, c.transactionsByID, 1)

	err = c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn2}, "", c.newCoordinatorTransaction)
	require.Error(t, err, "should return error when max inflight reached")
	assert.Len(t, c.transactionsByID, 1, "second transaction should not be added")
}

func assertTransactionsByIDWindow(t *testing.T, c *coordinator, txns []*components.PrivateTransaction, start, wantLen int) {
	t.Helper()
	require.Len(t, c.transactionsByID, wantLen, "in-flight count should match max window")
	idxByID := make(map[uuid.UUID]int, len(txns))
	for i, p := range txns {
		idxByID[p.ID] = i
	}
	for i := start; i < start+wantLen; i++ {
		require.NotNil(t, c.transactionsByID[txns[i].ID], "txns[%d] (1-based txn %d) should be coordinated", i, i+1)
	}
	for id := range c.transactionsByID {
		idx := idxByID[id]
		assert.GreaterOrEqual(t, idx, start)
		assert.Less(t, idx, start+wantLen, "no unexpected transaction should remain in-flight")
	}
}

func assertPooledTransactionsIncreasingByDelegationIndex(t *testing.T, c *coordinator, txns []*components.PrivateTransaction, start, wantLen int) {
	t.Helper()
	idxByID := make(map[uuid.UUID]int, len(txns))
	for i, p := range txns {
		idxByID[p.ID] = i
	}
	windowEnd := start + wantLen
	lastIdx := -1
	for _, coord := range c.pooledTransactions {
		idx, ok := idxByID[coord.GetID()]
		if !ok || idx < start || idx >= windowEnd {
			continue
		}
		assert.Greater(t, idx, lastIdx, "pooledTransactions should list this window in increasing delegation index (FIFO)")
		lastIdx = idx
	}
}

// Exercises maxInflightTransactions=3 with ten private transactions: each round delegates the suffix
// [txn k .. txn 10] after simulating completion of txn k by deleting it from transactionsByID.
// Rounds 0-6 return max-inflight error (tail txns rejected); rounds 7-9 fit entirely under the cap.
// After each addToDelegatedTransactions, the in-flight set is exactly txns[round:round+wantLen] (e.g. round 1 => txns 2,3,4).
func Test_addToDelegatedTransactions_MaxInflightThree_SlidingWindowKeepsOrder(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxInflightTransactions = confutil.P(3)
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	txns := make([]*components.PrivateTransaction, 10)
	for i := range txns {
		txns[i] = testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	}

	for round := 0; round < 10; round++ {
		wantLen := min(3, 10-round)
		delegationID := fmt.Sprintf("delegation-round-%d", round)

		err := c.addToDelegatedTransactions(ctx, originator, txns[round:], delegationID, c.newCoordinatorTransaction)

		if round <= 6 {
			require.Error(t, err, "round %d: should return max inflight error when tail transactions are rejected", round)
		} else {
			require.NoError(t, err, "round %d: should succeed when no max-inflight rejections occur", round)
		}

		assertTransactionsByIDWindow(t, c, txns, round, wantLen)
		assertPooledTransactionsIncreasingByDelegationIndex(t, c, txns, round, wantLen)

		if round < 9 {
			delete(c.transactionsByID, txns[round].ID)
		}
	}

	delete(c.transactionsByID, txns[9].ID)
	require.Empty(t, c.transactionsByID, "after removing the last coordinated txn, map should be empty")
}

func Test_addToDelegatedTransactions_HandleEventError_ContinuesAndReturnsNoError(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	txn.PreAssembly = nil // Triggers error in action_InitializeForNewAssembly when transitioning to Pooled

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "delegation-1", c.newCoordinatorTransaction)

	require.NoError(t, err)
	require.Len(t, c.transactionsByID, 1)
}

func Test_addToDelegatedTransactions_SendDelegationResponseError_ReturnsError(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.WithMockTransportWriter().Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	mocks.TransportWriter.On("SendDelegationResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("send ack failed"))

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "delegation-1", c.newCoordinatorTransaction)

	require.Error(t, err)
	assert.Equal(t, "send ack failed", err.Error())
}

func Test_action_SelectTransaction_WhenNoPooledTransaction_ReturnsNil(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()

	err := action_SelectTransaction(ctx, c, nil)
	require.NoError(t, err)
}

func Test_action_cancelCurrentlyAssemblingTransaction_NoAssemblingTransaction_ReturnsNil(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build()
	err := action_cancelCurrentlyAssemblingTransaction(ctx, c, nil)
	require.NoError(t, err)
}

func Test_action_cancelCurrentlyAssemblingTransaction_WithAssemblingTransaction_CancelsIt(t *testing.T) {
	txID := uuid.New()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.EXPECT().GetID().Return(txID)
	txn.EXPECT().GetCurrentState().Return(transaction.State_Assembling)
	// Transaction should receive AssembleCancelledEvent
	txn.EXPECT().HandleEvent(mock.Anything, mock.AnythingOfType("*transaction.AssembleCancelledEvent")).Return(nil)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txn).Build()

	err := action_cancelCurrentlyAssemblingTransaction(t.Context(), c, nil)
	require.NoError(t, err)
}

func Test_action_PoolTransaction_WhenTxnNotInMap_NoOp(t *testing.T) {
	ctx := t.Context()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build()
	err := action_PoolTransaction(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: uuid.New(),
		ToState:       transaction.State_Pooled,
	})
	require.NoError(t, err)
	assert.Empty(t, c.pooledTransactions)
}

func Test_action_QueueTransactionForDispatch_WhenContextDone_DoesNotBlock(t *testing.T) {
	txID := uuid.New()
	txn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	txn.EXPECT().GetID().Return(txID)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txn).Build()

	ctxCancelled, cancel := context.WithCancel(t.Context())
	cancel()

	err := action_QueueTransactionForDispatch(ctxCancelled, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txID,
		ToState:       transaction.State_Ready_For_Dispatch,
	})
	require.NoError(t, err)
}

func Test_addToDelegatedTransactions_PreviousTransactionInPreAssemblyState_EstablishesDependency(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	// Create a mock previous transaction in State_Pooled
	mockPreviousTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	previousTxnID := uuid.New()
	mockPreviousTxn.EXPECT().GetCurrentState().Return(transaction.State_Pooled)
	mockPreviousTxn.EXPECT().GetID().Return(previousTxnID)

	// Add mock previous transaction to coordinator
	c.transactionsByID[previousTxnID] = mockPreviousTxn

	// Create transactions list: [existingTxn, newTxn]
	// The existingTxn will become previousTransaction, and newTxn will trigger the dependency logic
	existingTxn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	existingTxn.ID = previousTxnID // Use the same ID as the mock transaction

	newTxn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{existingTxn, newTxn}, "", c.newCoordinatorTransaction)

	require.NoError(t, err)
}

func Test_addToDelegatedTransactions_PreviousTransactionInPreAssemblyState_DoesNotRequireHandleEvent(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	// Create a mock previous transaction in State_Pooled.
	mockPreviousTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	previousTxnID := uuid.New()
	mockPreviousTxn.EXPECT().GetCurrentState().Return(transaction.State_Pooled)
	mockPreviousTxn.EXPECT().GetID().Return(previousTxnID)

	// Add mock previous transaction to coordinator
	c.transactionsByID[previousTxnID] = mockPreviousTxn

	// Create transactions list: [existingTxn, newTxn]
	existingTxn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	existingTxn.ID = previousTxnID

	newTxn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{existingTxn, newTxn}, "", c.newCoordinatorTransaction)

	require.NoError(t, err)
}

func Test_addToDelegatedTransactions_MockTransactionHandleEventReturnsError(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build()
	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	expectedError := fmt.Errorf("handle delegated event failed")
	mockTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	mockTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(expectedError)

	createTransaction := func(
		_ context.Context,
		_ string, _ string, _ string,
		_ *components.PrivateTransaction,
	) transaction.CoordinatorTransaction {
		return mockTxn
	}

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "delegation-1", createTransaction)

	require.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Len(t, c.transactionsByID, 0, "transaction should be removed from map when HandleEvent fails")
	_, ok := c.transactionsByID[txn.ID]
	assert.False(t, ok)
}

func Test_addToDelegatedTransactions_SubsequentTransactionGetsPreviousTransactionError(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)

	firstTxnError := fmt.Errorf("first transaction handle event failed")
	mockTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	mockTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(firstTxnError)

	createTransaction := func(
		_ context.Context,
		_ string, _ string, _ string,
		_ *components.PrivateTransaction,
	) transaction.CoordinatorTransaction {
		return mockTxn
	}

	var capturedErrors []int64
	c, mocks := builder.WithMockTransportWriter().Build()
	mocks.TransportWriter.On("SendDelegationResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(errors []int64) bool {
		capturedErrors = errors
		return true
	}), mock.Anything).Return(nil)

	txn1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	txn2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn1, txn2}, "delegation-1", createTransaction)

	require.Error(t, err)
	assert.Equal(t, firstTxnError, err)
	assert.Len(t, c.transactionsByID, 0, "first transaction removed after HandleEvent failure; second was skipped due to previous error")
	// Second transaction should get PreviousTransactionError in the acknowledgement (covers lines 94-95)
	require.Len(t, capturedErrors, 2, "ack should have one entry per transaction")
	assert.Equal(t, int64(DelegationAcknowledgementError_CoordinatorError), capturedErrors[0], "first txn gets CoordinatorError from HandleEvent failure")
	assert.Equal(t, int64(DelegationAcknowledgementError_PreviousTransactionError), capturedErrors[1], "second txn gets PreviousTransactionError when a previous txn failed")
}

func Test_addToDelegatedTransactions_ErrorStopsSubsequentTransactionsBeingAccepted(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)

	fifthErr := fmt.Errorf("fifth transaction HandleEvent failed")
	c, mocks := builder.WithMockTransportWriter().Build()
	mocks.TransportWriter.On("SendDelegationResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Delegate 10 transactions. TX 5 fails at HandleEvent time. 5-10 should not be in the TX list for the coordinator
	txns := make([]*components.PrivateTransaction, 10)
	for i := range txns {
		txns[i] = testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	}

	createTransaction := func(
		_ context.Context,
		_ string, _ string, _ string,
		pt *components.PrivateTransaction,
	) transaction.CoordinatorTransaction {
		idx := -1
		for i, priv := range txns {
			if priv.ID == pt.ID {
				idx = i
				break
			}
		}
		require.GreaterOrEqual(t, idx, 0)
		require.Less(t, idx, 5, "only the first five delegated transactions are created before the batch stops")

		m := coordinatortransactionmocks.NewCoordinatorTransaction(t)
		m.EXPECT().GetID().Return(pt.ID).Maybe()
		m.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()

		switch {
		case idx < 4:
			m.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()
		case idx == 4:
			m.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(fifthErr).Once()
		}
		return m
	}

	err := c.addToDelegatedTransactions(ctx, originator, txns, "delegation-10", createTransaction)

	require.Error(t, err)
	assert.Equal(t, fifthErr, err)
	require.Len(t, c.transactionsByID, 4, "transactions 1-4 accepted; 5 failed HandleEvent; 6-10 never created")
	for i := 0; i < 4; i++ {
		assert.NotNil(t, c.transactionsByID[txns[i].ID], "transaction %d should be in transactionsByID", i+1)
	}
	for i := 4; i < 10; i++ {
		assert.Nil(t, c.transactionsByID[txns[i].ID], "transaction %d should not be in transactionsByID", i+1)
	}
}

func Test_addToDelegatedTransactions_FifthFailsThenFullRetry_PreservesFirstFourAndAcceptsRestInOrder(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)

	fifthErr := fmt.Errorf("fifth transaction HandleEvent failed")

	c, mocks := builder.WithMockTransportWriter().Build()
	mocks.TransportWriter.On("SendDelegationResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

	txns := make([]*components.PrivateTransaction, 10)
	for i := range txns {
		txns[i] = testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	}

	// Coordinators for transactions 1-4: pass 1 (Delegated) and pass 2 (reused).
	m0 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	m0.EXPECT().GetID().Return(txns[0].ID).Maybe()
	m0.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	m0.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()

	m1 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	m1.EXPECT().GetID().Return(txns[1].ID).Maybe()
	m1.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	m1.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()

	m2 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	m2.EXPECT().GetID().Return(txns[2].ID).Maybe()
	m2.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	m2.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()

	m3 := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	m3.EXPECT().GetID().Return(txns[3].ID).Maybe()
	m3.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	m3.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()

	mFail := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	mFail.EXPECT().GetID().Return(txns[4].ID).Maybe()
	mFail.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	mFail.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(fifthErr).Once()

	firstFourByID := map[uuid.UUID]transaction.CoordinatorTransaction{
		txns[0].ID: m0,
		txns[1].ID: m1,
		txns[2].ID: m2,
		txns[3].ID: m3,
	}

	// New coordinators created on the successful retry for transactions 5-10 (indices 4-9).
	pass2Mocks := make([]*coordinatortransactionmocks.CoordinatorTransaction, 6)
	for i := range pass2Mocks {
		idx := 4 + i
		pt := txns[idx]
		m := coordinatortransactionmocks.NewCoordinatorTransaction(t)
		m.EXPECT().GetID().Return(pt.ID).Maybe()
		m.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
		m.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()
		pass2Mocks[i] = m
	}

	idxOf := func(pt *components.PrivateTransaction) int {
		for i, priv := range txns {
			if priv.ID == pt.ID {
				return i
			}
		}
		return -1
	}

	var secondPassNewOrder []uuid.UUID
	attempt := 0
	createTransaction := func(
		_ context.Context,
		_ string, _ string, _ string,
		pt *components.PrivateTransaction,
	) transaction.CoordinatorTransaction {
		idx := idxOf(pt)
		require.GreaterOrEqual(t, idx, 0)
		switch attempt {
		case 0:
			require.LessOrEqual(t, idx, 4)
			if idx < 4 {
				return firstFourByID[pt.ID]
			}
			return mFail
		case 1:
			if idx < 4 {
				return firstFourByID[pt.ID]
			}
			require.Less(t, idx, 10)
			secondPassNewOrder = append(secondPassNewOrder, pt.ID)
			return pass2Mocks[idx-4]
		default:
			t.Fatalf("unexpected attempt %d", attempt)
			return nil
		}
	}

	err := c.addToDelegatedTransactions(ctx, originator, txns, "delegation-pass1", createTransaction)
	require.Error(t, err)
	assert.Equal(t, fifthErr, err)
	require.Len(t, c.transactionsByID, 4)

	saved := [4]transaction.CoordinatorTransaction{
		c.transactionsByID[txns[0].ID],
		c.transactionsByID[txns[1].ID],
		c.transactionsByID[txns[2].ID],
		c.transactionsByID[txns[3].ID],
	}

	attempt = 1
	err = c.addToDelegatedTransactions(ctx, originator, txns, "delegation-pass2", createTransaction)
	require.NoError(t, err)

	require.Len(t, c.transactionsByID, 10)
	for i := range 4 {
		assert.Same(t, saved[i], c.transactionsByID[txns[i].ID], "coordinator for transaction %d should be unchanged after retry", i+1)
	}
	for i := range 10 {
		assert.NotNil(t, c.transactionsByID[txns[i].ID], "transaction %d should be coordinated", i+1)
	}
	assert.NotSame(t, mFail, c.transactionsByID[txns[4].ID], "transaction 5 should use a new coordinator after the failed first attempt")

	require.Len(t, secondPassNewOrder, 6)
	for i := range 6 {
		assert.Equal(t, txns[4+i].ID, secondPassNewOrder[i], "retry should create new coordinators for transactions 5-10 in delegation order")
	}
}

func Test_addToDelegatedTransactions_PreviousTransactionNotInPreAssemblyState_NoDependencyEstablished(t *testing.T) {
	ctx := t.Context()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build()
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	// Create a mock previous transaction in State_Assembling (not a pre-assembly state)
	mockPreviousTxn := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	previousTxnID := uuid.New()
	mockPreviousTxn.EXPECT().GetCurrentState().Return(transaction.State_Assembling)
	// NOTE: HandleEvent should NOT be called - no expectation set for it

	// Add mock previous transaction to coordinator
	c.transactionsByID[previousTxnID] = mockPreviousTxn

	// Create transactions list: [existingTxn, newTxn]
	existingTxn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	existingTxn.ID = previousTxnID

	newTxn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{existingTxn, newTxn}, "", c.newCoordinatorTransaction)

	require.NoError(t, err)
}

func Test_action_CleanUpTransactionsNotYetDispatched_DrainsPendingDispatchQueueItems(t *testing.T) {
	ctx := context.Background()

	txPooled := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	idPooled := uuid.New()
	txPooled.EXPECT().GetID().Return(idPooled)
	txPooled.EXPECT().GetCurrentState().Return(transaction.State_Pooled)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txPooled).Build()

	// Pre-populate the dispatch queue with a transaction reference to exercise the drain path.
	c.dispatchQueue <- txPooled

	err := action_CleanUpTransactionsNotYetDispatched(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(c.dispatchQueue), "dispatch queue must be drained")
}

func Test_action_CleanUpTransactionsNotYetDispatched_RemovesNonDispatchedTransactions(t *testing.T) {
	ctx := context.Background()

	txPooled := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	idPooled := uuid.New()
	txPooled.EXPECT().GetID().Return(idPooled)
	txPooled.EXPECT().GetCurrentState().Return(transaction.State_Pooled)

	txAssembling := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	idAssembling := uuid.New()
	txAssembling.EXPECT().GetID().Return(idAssembling)
	txAssembling.EXPECT().GetCurrentState().Return(transaction.State_Assembling)

	txConfirmed := coordinatortransactionmocks.NewCoordinatorTransaction(t)
	idConfirmed := uuid.New()
	txConfirmed.EXPECT().GetID().Return(idConfirmed)
	txConfirmed.EXPECT().GetCurrentState().Return(transaction.State_Confirmed)

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Transactions(txPooled, txAssembling, txConfirmed).Build()
	// cleanUpTransaction is called once for each non-dispatched, non-confirmed transaction (Pooled + Assembling).

	err := action_CleanUpTransactionsNotYetDispatched(ctx, c, nil)
	require.NoError(t, err)

	assert.NotContains(t, c.transactionsByID, idPooled, "Pooled transaction should be cleaned up")
	assert.NotContains(t, c.transactionsByID, idAssembling, "Assembling transaction should be cleaned up")
	assert.Contains(t, c.transactionsByID, idConfirmed, "Confirmed transaction should remain")
}

func Test_updateEndorserCandidates_AddsNodeToEmptyPool(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.updateEndorserCandidates(ctx, "node2")

	assert.Equal(t, 2, len(c.endorserCandidates), "pool should contain 2 nodes")
	assert.Contains(t, c.endorserCandidates, "node2", "pool should contain node2")
	assert.Contains(t, c.endorserCandidates, "node1", "pool should contain coordinator's own node")
}

func Test_updateEndorserCandidates_AddsNodeToNonEmptyPool(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node3").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.updateEndorserCandidates(ctx, "node2")

	assert.Equal(t, 3, len(c.endorserCandidates))
	assert.Contains(t, c.endorserCandidates, "node1")
	assert.Contains(t, c.endorserCandidates, "node2")
	assert.Contains(t, c.endorserCandidates, "node3")
}

func Test_updateEndorserCandidates_DoesNotAddDuplicateNode(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.updateEndorserCandidates(ctx, "node2")

	assert.Equal(t, 2, len(c.endorserCandidates), "duplicate should not be added")
}

func Test_updateEndorserCandidates_EnsuresCoordinatorsOwnNodeIsAlwaysInPool(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.updateEndorserCandidates(ctx, "node2")

	assert.Contains(t, c.endorserCandidates, "node1", "pool should contain coordinator's own node")
	assert.Equal(t, 2, len(c.endorserCandidates))
}

func Test_updateEndorserCandidates_DoesNotDuplicateCoordinatorsOwnNode(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.updateEndorserCandidates(ctx, "node1")

	assert.Equal(t, 2, len(c.endorserCandidates), "pool should still contain 2 nodes")
}

func Test_updateEndorserCandidates_HandlesMultipleSequentialUpdates(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.updateEndorserCandidates(ctx, "node2")
	c.updateEndorserCandidates(ctx, "node3")
	c.updateEndorserCandidates(ctx, "node4")

	assert.Equal(t, 4, len(c.endorserCandidates))
	assert.Contains(t, c.endorserCandidates, "node1")
	assert.Contains(t, c.endorserCandidates, "node2")
	assert.Contains(t, c.endorserCandidates, "node3")
	assert.Contains(t, c.endorserCandidates, "node4")
}

func Test_updateEndorserCandidates_NewNodeTriggersRecalculationAndNotifiesOriginator(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	var received common.Event
	c.notifyOriginator = func(_ context.Context, event common.Event) {
		received = event
	}

	c.updateEndorserCandidates(ctx, "node2")

	require.NotNil(t, received, "originator must be notified when pool grows")
	discoveredEvent, ok := received.(*common.EndorserNodesDiscoveredEvent)
	require.True(t, ok)
	assert.Contains(t, discoveredEvent.Nodes, "node1")
	assert.Contains(t, discoveredEvent.Nodes, "node2")
}

func Test_updateEndorserCandidates_NoNewNodeDoesNotNotifyOriginator(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	notified := false
	c.notifyOriginator = func(_ context.Context, _ common.Event) { notified = true }

	c.updateEndorserCandidates(ctx, "node2")

	assert.False(t, notified, "originator must not be notified when pool does not grow")
}

func Test_updateEndorserCandidates_NoOpInSenderMode(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build() // default = SENDER mode

	c.updateEndorserCandidates(ctx, "node2")

	assert.Empty(t, c.endorserCandidates, "endorserCandidates must not grow in SENDER mode")
}

func Test_recordOriginatorActivity_SetsCountToZero(t *testing.T) {
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()

	c.recordOriginatorActivity("node2")

	assert.Equal(t, 0, c.originatorActivity["node2"])
}

func Test_recordOriginatorActivity_ResetsExistingCount(t *testing.T) {
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorActivity(map[string]int{"node2": 3}).
		Build()

	c.recordOriginatorActivity("node2")

	assert.Equal(t, 0, c.originatorActivity["node2"])
}

func Test_recordOriginatorActivity_NoOpInEndorserMode(t *testing.T) {
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.recordOriginatorActivity("node2")

	assert.Empty(t, c.originatorActivity, "originatorActivity must not be populated in ENDORSER mode")
}

func Test_updateOriginatorActivity_ResetsCountForActiveNode(t *testing.T) {
	ctx := context.Background()
	txn := newMockCoordinatorTransactionWithOriginatorNode(t, "node2")
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		OriginatorActivity(map[string]int{"node2": 3}).
		InactiveGracePeriod(10).
		Build()

	c.updateOriginatorActivity(ctx)

	assert.Equal(t, 0, c.originatorActivity["node2"], "active node counter must be reset to 0")
}

func Test_updateOriginatorActivity_IncrementsCountForInactiveNode(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorActivity(map[string]int{"node2": 2}).
		InactiveGracePeriod(10).
		Build()

	c.updateOriginatorActivity(ctx)

	assert.Equal(t, 3, c.originatorActivity["node2"])
}

func Test_updateOriginatorActivity_PrunesNodeAtGracePeriod(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorActivity(map[string]int{"node2": 5}).
		InactiveGracePeriod(5).
		Build()

	c.updateOriginatorActivity(ctx)

	assert.NotContains(t, c.originatorActivity, "node2", "node must be pruned when count reaches inactiveGracePeriod")
}

func Test_updateOriginatorActivity_DoesNotPruneActiveNode(t *testing.T) {
	ctx := context.Background()
	txn := newMockCoordinatorTransactionWithOriginatorNode(t, "node2")
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		OriginatorActivity(map[string]int{"node2": 5}).
		InactiveGracePeriod(5).
		Build()

	c.updateOriginatorActivity(ctx)

	assert.Contains(t, c.originatorActivity, "node2", "active node must not be pruned")
	assert.Equal(t, 0, c.originatorActivity["node2"])
}

func Test_calculateCoordinatorPriorities_UpdatesPriorityList(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.calculateCoordinatorPriorities(ctx)
	assert.NotEmpty(t, c.coordinatorPriorityList)
	assert.Contains(t, []string{"node1", "node2"}, c.coordinatorPriorityList[0])
}

// scheduleRequestTimeout cancels an existing timer before arming a new one, and its timer callback
// queues a RequestTimeoutIntervalEvent onto the coordinator event loop.
func Test_scheduleRequestTimeout_ReplacesExistingTimer(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).Build()

	oldCancelCalled := false
	c.cancelRequestTimeout = func() { oldCancelCalled = true }

	c.scheduleRequestTimeout(ctx)

	assert.True(t, oldCancelCalled, "scheduleRequestTimeout must cancel the existing timer before arming a new one")
	assert.NotNil(t, c.cancelRequestTimeout, "scheduleRequestTimeout must arm a new timer")
	c.cancelRequestTimeout() // clean up the newly-armed timer
}

// scheduleRequestTimeout_TimerCallback verifies that when the timer fires it queues a
// RequestTimeoutIntervalEvent onto the coordinator's event loop.
func Test_scheduleRequestTimeout_TimerCallback_QueuesRequestTimeoutEvent(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).UseMockClock().Build()

	// When ScheduleTimer is called, invoke the callback synchronously so the closure body is covered.
	// The callback calls QueueEvent which sends to the buffered event channel.
	mocks.Clock.On("ScheduleTimer", mock.Anything, mock.Anything, mock.Anything).
		Return(func() {}).
		Run(func(args mock.Arguments) {
			args.Get(2).(func())()
		})

	c.scheduleRequestTimeout(ctx)

	assert.NotNil(t, c.cancelRequestTimeout, "scheduleRequestTimeout must set a cancel func")
}

// scheduleStateTimeout cancels an existing timer before arming a new one.
func Test_scheduleStateTimeout_ReplacesExistingTimer(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).Build()

	oldCancelCalled := false
	c.cancelStateTimeout = func() { oldCancelCalled = true }

	c.scheduleStateTimeout(ctx)

	assert.True(t, oldCancelCalled, "scheduleStateTimeout must cancel the existing timer before arming a new one")
	assert.NotNil(t, c.cancelStateTimeout, "scheduleStateTimeout must arm a new timer")
	c.cancelStateTimeout() // clean up the newly-armed timer
}

// scheduleStateTimeout_TimerCallback verifies that when the timer fires it queues a
// StateTimeoutIntervalEvent onto the coordinator's event loop.
func Test_scheduleStateTimeout_TimerCallback_QueuesStateTimeoutEvent(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).UseMockClock().Build()

	mocks.Clock.On("ScheduleTimer", mock.Anything, mock.Anything, mock.Anything).
		Return(func() {}).
		Run(func(args mock.Arguments) {
			args.Get(2).(func())()
		})

	c.scheduleStateTimeout(ctx)

	assert.NotNil(t, c.cancelStateTimeout, "scheduleStateTimeout must set a cancel func")
}

// action_ProcessConfirmedTransactionsFromSnapshot returns immediately when there is no snapshot
// (e.g. a plain Observing heartbeat carries no coordinator state).
func Test_action_ProcessConfirmedTransactionsFromSnapshot_NilSnapshot_ReturnsNil(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Prepared).Build()

	err := action_ProcessConfirmedTransactionsFromSnapshot(ctx, c, &common.HeartbeatReceivedEvent{
		FromNode:            "node2",
		CoordinatorSnapshot: nil, // no snapshot — guard must return nil immediately
	})
	require.NoError(t, err)
}

// clearTimeoutSchedules calls and nils both cancel functions when they are set, and also
// clears pendingHandoverRequest.
func Test_clearTimeoutSchedules_WithBothSet_CancelsBothAndNilsOut(t *testing.T) {
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).Build()

	requestCancelled := false
	stateCancelled := false
	c.cancelRequestTimeout = func() { requestCancelled = true }
	c.cancelStateTimeout = func() { stateCancelled = true }
	c.pendingHandoverRequest = nil // value doesn't matter; just ensure it is cleared

	c.clearTimeoutSchedules()

	assert.True(t, requestCancelled, "cancelRequestTimeout must be called")
	assert.True(t, stateCancelled, "cancelStateTimeout must be called")
	assert.Nil(t, c.cancelRequestTimeout, "cancelRequestTimeout must be nilled")
	assert.Nil(t, c.cancelStateTimeout, "cancelStateTimeout must be nilled")
	assert.Nil(t, c.pendingHandoverRequest, "pendingHandoverRequest must be cleared")
}

func Test_nudgeHandoverRequest_NoPendingRequest_ReturnsError(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).Build()

	err := c.nudgeHandoverRequest(ctx)

	require.Error(t, err, "nudgeHandoverRequest must error when no pending request exists")
	assert.ErrorContains(t, err, "nudgeHandoverRequest called with no pending request")
}

func Test_nudgeHandoverRequest_WithPendingRequest_CallsNudge(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Elect).
		NodeName("node1").
		CurrentActiveCoordinator("node2").
		WithMockTransportWriter().
		Build()
	mocks.TransportWriter.EXPECT().SendHandoverRequest(mock.Anything, "node2", mock.Anything).Return(nil).Once()
	// A freshly created IdempotentRequest (requestTime == nil) always sends on first Nudge.
	c.pendingHandoverRequest = common.NewIdempotentRequest(ctx, c.clock, c.requestTimeout, func(ctx context.Context, _ uuid.UUID) error {
		return c.transportWriter.SendHandoverRequest(ctx, c.currentActiveCoordinator, c.contractAddress)
	})

	err := c.nudgeHandoverRequest(ctx)

	require.NoError(t, err)
}
