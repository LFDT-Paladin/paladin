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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_addToDelegatedTransactions_NewTransactionError_ReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	validOriginator := "sender@senderNode"
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(validOriginator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	invalidOriginator := "sender@node1@node2"
	err := c.addToDelegatedTransactions(ctx, invalidOriginator, []*components.PrivateTransaction{txn}, "", c.newCoordinatorTransaction)

	require.Error(t, err, "should return error when NewTransaction fails")
	assert.Equal(t, 0, len(c.transactionsByID), "transaction should not be added when NewTransaction fails")
}

func Test_addToDelegatedTransactions_AddsTransactionInPreDispatchFlowState(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

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
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

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
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

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

func Test_addTransactionToBackOfPool_WhenNotInPool_Appends(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()

	c.addTransactionToBackOfPool(txn)

	require.Len(t, c.pooledTransactions, 1, "pool should contain one transaction")
	assert.Equal(t, txn, c.pooledTransactions[0])
}

func Test_addTransactionToBackOfPool_WhenAlreadyInPool_DoesNotDuplicate(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()

	c.addTransactionToBackOfPool(txn)
	c.addTransactionToBackOfPool(txn)

	require.Len(t, c.pooledTransactions, 1, "pool should not duplicate transaction")
	assert.Equal(t, txn, c.pooledTransactions[0])
}

func Test_action_PoolTransaction(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	c.transactionsByID[txn.GetID()] = txn

	err := action_PoolTransaction(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txn.GetID(),
		To:            transaction.State_Pooled,
	})
	require.NoError(t, err)
	require.Len(t, c.pooledTransactions, 1, "transaction should be added to pool")
	assert.Equal(t, txn, c.pooledTransactions[0])
}

func Test_action_QueueTransactionForDispatch(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build()
	c.transactionsByID[txn.GetID()] = txn

	err := action_QueueTransactionForDispatch(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txn.GetID(),
		To:            transaction.State_Ready_For_Dispatch,
	})
	require.NoError(t, err)
}

func Test_action_CleanUpTransaction_RemovesFromMap(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID[txn.GetID()] = txn

	err := action_CleanUpTransaction(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txn.GetID(),
		To:            transaction.State_Final,
	})
	require.NoError(t, err)
	_, ok := c.transactionsByID[txn.GetID()]
	assert.False(t, ok, "transaction should be removed from map")
}

func Test_action_CleanUpTransaction_GrapherForgetError_LogsButReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	c.transactionsByID[txn.GetID()] = txn

	mockGrapher := grapher.NewMockGrapher(t)
	mockGrapher.EXPECT().Forget(txn.GetID()).Return(fmt.Errorf("forget failed"))
	c.grapher = mockGrapher

	err := action_CleanUpTransaction(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txn.GetID(),
		To:            transaction.State_Final,
	})
	require.NoError(t, err)
	_, ok := c.transactionsByID[txn.GetID()]
	assert.False(t, ok, "transaction should still be removed from map despite grapher error")
}

func Test_validator_TransactionStateTransitionToPooled(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_TransactionStateTransitionToPooled(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Pooled})
	require.NoError(t, err)
	assert.True(t, valid)

	valid, err = validator_TransactionStateTransitionToPooled(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Final})
	require.NoError(t, err)
	assert.False(t, valid)
}

func Test_validator_TransactionStateTransitionToReadyForDispatch(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_TransactionStateTransitionToReadyForDispatch(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Ready_For_Dispatch})
	require.NoError(t, err)
	assert.True(t, valid)

	valid, err = validator_TransactionStateTransitionToReadyForDispatch(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Pooled})
	require.NoError(t, err)
	assert.False(t, valid)
}

func Test_validator_TransactionStateTransitionToFinal(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_TransactionStateTransitionToFinal(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Final})
	require.NoError(t, err)
	assert.True(t, valid)

	valid, err = validator_TransactionStateTransitionToFinal(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{To: transaction.State_Pooled})
	require.NoError(t, err)
	assert.False(t, valid)
}

func Test_addToDelegatedTransactions_WhenMaxInflightReached_ReturnsError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxInflightTransactions = confutil.P(1)
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _, done := builder.Build(ctx)
	defer done()

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
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	config.MaxInflightTransactions = confutil.P(3)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

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
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	txn.PreAssembly = nil // Triggers error in action_InitializeForNewAssembly when transitioning to Pooled

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "delegation-1", c.newCoordinatorTransaction)

	require.NoError(t, err)
	require.Len(t, c.transactionsByID, 1)
}

func Test_addToDelegatedTransactions_SendDelegationRequestAcknowledgmentError_ReturnsError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendDelegationRequestAcknowledgment", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("send ack failed"))
	mockTransport.On("WaitForDone", mock.Anything).Return().Maybe()
	c.transportWriter = mockTransport

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "delegation-1", c.newCoordinatorTransaction)

	require.Error(t, err)
	assert.Equal(t, "send ack failed", err.Error())
	mockTransport.AssertExpectations(t)
}

func Test_action_SelectTransaction_WhenNoPooledTransaction_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.pooledTransactions = nil

	err := action_SelectTransaction(ctx, c, nil)
	require.NoError(t, err)
}

func Test_action_cancelCurrentlyAssemblingTransaction_NoAssemblingTransaction_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	err := action_cancelCurrentlyAssemblingTransaction(ctx, c, nil)
	require.NoError(t, err)
}

func Test_action_cancelCurrentlyAssemblingTransaction_WithAssemblingTransaction_CancelsIt(t *testing.T) {
	ctx := context.Background()
	mockGrapher := grapher.NewMockGrapher(t)
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Grapher(mockGrapher).Build()
	mockGrapher.EXPECT().ForgetLocks(txn.GetID())
	mockGrapher.EXPECT().GetDependents(mock.Anything, txn.GetID()).Return([]uuid.UUID{})
	mockGrapher.EXPECT().RemoveAllDependencyLinks(txn.GetID()) // Moving the TX back to pooled should remove all depends-on and prereq-of links
	mockGrapher.EXPECT().ForgetMints(txn.GetID())              // Moving to pooled should also reset its mints in the grapher
	c.transactionsByID[txn.GetID()] = txn

	err := action_cancelCurrentlyAssemblingTransaction(ctx, c, nil)
	require.NoError(t, err)
	// Transaction should transition from Assembling to Pooled when AssembleCancelledEvent is handled
	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState())
}

func Test_validator_TransactionStateTransitionDispatchedToPooled(t *testing.T) {
	ctx := context.Background()
	valid, err := validator_TransactionStateTransitionDispatchedToPooled(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{
		From: transaction.State_Dispatched,
		To:   transaction.State_Pooled,
	})
	require.NoError(t, err)
	assert.True(t, valid)

	valid, err = validator_TransactionStateTransitionDispatchedToPooled(ctx, nil, &common.TransactionStateTransitionEvent[transaction.State]{
		From: transaction.State_Assembling,
		To:   transaction.State_Pooled,
	})
	require.NoError(t, err)
	assert.False(t, valid)
}

func Test_action_PoolTransaction_WhenTxnNotInMap_NoOp(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	err := action_PoolTransaction(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: uuid.New(),
		To:            transaction.State_Pooled,
	})
	require.NoError(t, err)
	assert.Empty(t, c.pooledTransactions)
}

func Test_action_QueueTransactionForDispatch_WhenContextDone_DoesNotBlock(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build()
	c.transactionsByID[txn.GetID()] = txn

	ctxCancelled, cancel := context.WithCancel(ctx)
	cancel()

	err := action_QueueTransactionForDispatch(ctxCancelled, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txn.GetID(),
		To:            transaction.State_Ready_For_Dispatch,
	})
	require.NoError(t, err)
}

func Test_addToDelegatedTransactions_PreviousTransactionInPreAssemblyState_EstablishesDependency(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a mock previous transaction in State_Pooled
	mockPreviousTxn := transaction.NewMockCoordinatorTransaction(t)
	previousTxnID := uuid.New()
	mockPreviousTxn.EXPECT().GetCurrentState().Return(transaction.State_Pooled)
	mockPreviousTxn.EXPECT().GetID().Return(previousTxnID)
	mockPreviousTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NewPreAssembleDependencyEvent")).Return(nil)

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

func Test_addToDelegatedTransactions_PreviousTransactionHandleEventReturnsError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a mock previous transaction in State_Pooled that returns an error from HandleEvent
	mockPreviousTxn := transaction.NewMockCoordinatorTransaction(t)
	previousTxnID := uuid.New()
	expectedError := fmt.Errorf("handle event error")
	mockPreviousTxn.EXPECT().GetCurrentState().Return(transaction.State_Pooled)
	mockPreviousTxn.EXPECT().GetID().Return(previousTxnID)
	mockPreviousTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NewPreAssembleDependencyEvent")).Return(expectedError)

	// Add mock previous transaction to coordinator
	c.transactionsByID[previousTxnID] = mockPreviousTxn

	// Create transactions list: [existingTxn, newTxn]
	existingTxn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	existingTxn.ID = previousTxnID

	newTxn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{existingTxn, newTxn}, "", c.newCoordinatorTransaction)

	require.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func Test_addToDelegatedTransactions_MockTransactionHandleEventReturnsError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("").Maybe()
	builder.GetDomainAPI().On("Domain").Return(mockDomain).Maybe()
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	expectedError := fmt.Errorf("handle delegated event failed")
	mockTxn := transaction.NewMockCoordinatorTransaction(t)
	mockTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(expectedError)

	createTransaction := func(
		_ context.Context,
		_ string, _ string, _ string,
		pt *components.PrivateTransaction,
		_ string,
		_ *uuid.UUID,
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
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("").Maybe()
	builder.GetDomainAPI().On("Domain").Return(mockDomain).Maybe()
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	txn1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	txn2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	firstTxnError := fmt.Errorf("first transaction handle event failed")
	mockTxn := transaction.NewMockCoordinatorTransaction(t)
	mockTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(firstTxnError)

	createTransaction := func(
		_ context.Context,
		_ string, _ string, _ string,
		_ *components.PrivateTransaction,
		_ string,
		_ *uuid.UUID,
	) transaction.CoordinatorTransaction {
		return mockTxn
	}

	var capturedErrors []int64
	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendDelegationRequestAcknowledgment", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(errors []int64) bool {
		capturedErrors = errors
		return true
	})).Return(nil)
	mockTransport.On("WaitForDone", mock.Anything).Return().Maybe()
	c.transportWriter = mockTransport

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn1, txn2}, "delegation-1", createTransaction)

	require.Error(t, err)
	assert.Equal(t, firstTxnError, err)
	assert.Len(t, c.transactionsByID, 0, "first transaction removed after HandleEvent failure; second was skipped due to previous error")
	// Second transaction should get PreviousTransactionError in the acknowledgement (covers lines 94-95)
	require.Len(t, capturedErrors, 2, "ack should have one entry per transaction")
	assert.Equal(t, int64(DelegationAcknowledgementError_CoordinatorError), capturedErrors[0], "first txn gets CoordinatorError from HandleEvent failure")
	assert.Equal(t, int64(DelegationAcknowledgementError_PreviousTransactionError), capturedErrors[1], "second txn gets PreviousTransactionError when a previous txn failed")
	mockTransport.AssertExpectations(t)
}

func Test_addToDelegatedTransactions_ErrorStopsSubsequentTransactionsBeingAccepted(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("").Maybe()
	builder.GetDomainAPI().On("Domain").Return(mockDomain).Maybe()
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	// Delegate 10 transactions. TX 5 fails at HandleEvent time. 5-10 should not be in the TX list for the coordinator
	txns := make([]*components.PrivateTransaction, 10)
	for i := range txns {
		txns[i] = testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	}

	fifthErr := fmt.Errorf("fifth transaction HandleEvent failed")
	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendDelegationRequestAcknowledgment", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockTransport.On("WaitForDone", mock.Anything).Return().Maybe()
	c.transportWriter = mockTransport

	createTransaction := func(
		_ context.Context,
		_ string, _ string, _ string,
		pt *components.PrivateTransaction,
		_ string,
		_ *uuid.UUID,
	) transaction.CoordinatorTransaction {
		idx := -1
		for i, t := range txns {
			if t.ID == pt.ID {
				idx = i
				break
			}
		}
		require.GreaterOrEqual(t, idx, 0)
		require.Less(t, idx, 5, "only the first five delegated transactions are created before the batch stops")

		m := transaction.NewMockCoordinatorTransaction(t)
		m.EXPECT().GetID().Return(pt.ID).Maybe()
		m.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()

		switch {
		case idx < 4:
			m.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()
			m.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NewPreAssembleDependencyEvent")).Return(nil).Once()
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
	mockTransport.AssertExpectations(t)
}

func Test_addToDelegatedTransactions_FifthFailsThenFullRetry_PreservesFirstFourAndAcceptsRestInOrder(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("").Maybe()
	builder.GetDomainAPI().On("Domain").Return(mockDomain).Maybe()
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	txns := make([]*components.PrivateTransaction, 10)
	for i := range txns {
		txns[i] = testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	}

	fifthErr := fmt.Errorf("fifth transaction HandleEvent failed")

	// Coordinators for transactions 1-4: pass 1 (Delegated + NewPreAssemble each) and pass 2 (extra NewPreAssemble on txn 4's coordinator before txn 5 succeeds).
	m0 := transaction.NewMockCoordinatorTransaction(t)
	m0.EXPECT().GetID().Return(txns[0].ID).Maybe()
	m0.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	m0.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()
	m0.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NewPreAssembleDependencyEvent")).Return(nil).Once()

	m1 := transaction.NewMockCoordinatorTransaction(t)
	m1.EXPECT().GetID().Return(txns[1].ID).Maybe()
	m1.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	m1.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()
	m1.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NewPreAssembleDependencyEvent")).Return(nil).Once()

	m2 := transaction.NewMockCoordinatorTransaction(t)
	m2.EXPECT().GetID().Return(txns[2].ID).Maybe()
	m2.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	m2.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()
	m2.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NewPreAssembleDependencyEvent")).Return(nil).Once()

	m3 := transaction.NewMockCoordinatorTransaction(t)
	m3.EXPECT().GetID().Return(txns[3].ID).Maybe()
	m3.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
	m3.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()
	m3.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NewPreAssembleDependencyEvent")).Return(nil).Once()
	m3.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NewPreAssembleDependencyEvent")).Return(nil).Once()

	mFail := transaction.NewMockCoordinatorTransaction(t)
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
	pass2Mocks := make([]*transaction.MockCoordinatorTransaction, 6)
	for i := range pass2Mocks {
		idx := 4 + i
		pt := txns[idx]
		m := transaction.NewMockCoordinatorTransaction(t)
		m.EXPECT().GetID().Return(pt.ID).Maybe()
		m.EXPECT().GetCurrentState().Return(transaction.State_Pooled).Maybe()
		m.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DelegatedEvent")).Return(nil).Once()
		if idx < 9 {
			m.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.NewPreAssembleDependencyEvent")).Return(nil).Once()
		}
		pass2Mocks[i] = m
	}

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendDelegationRequestAcknowledgment", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()
	mockTransport.On("WaitForDone", mock.Anything).Return().Maybe()
	c.transportWriter = mockTransport

	idxOf := func(pt *components.PrivateTransaction) int {
		for i, t := range txns {
			if t.ID == pt.ID {
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
		_ string,
		_ *uuid.UUID,
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

	mockTransport.AssertExpectations(t)
}

func Test_addToDelegatedTransactions_PreviousTransactionNotInPreAssemblyState_NoDependencyEstablished(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	// Create a mock previous transaction in State_Assembling (not a pre-assembly state)
	mockPreviousTxn := transaction.NewMockCoordinatorTransaction(t)
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
