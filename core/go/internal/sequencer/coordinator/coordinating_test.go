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
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
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
	err := c.addToDelegatedTransactions(ctx, invalidOriginator, []*components.PrivateTransaction{txn}, "")

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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "")

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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "")

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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "")
	require.NoError(t, err, "should not return error on first add")
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	firstCoordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, firstCoordinatedTxn, "transaction should exist in transactionsByID")

	err = c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "")
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

	mockGrapher := transaction.NewMockGrapher(t)
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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn1}, "")
	require.NoError(t, err)
	require.Len(t, c.transactionsByID, 1)

	err = c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn2}, "")
	require.Error(t, err, "should return error when max inflight reached")
	assert.Len(t, c.transactionsByID, 1, "second transaction should not be added")
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
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	txn.PreAssembly = nil // Triggers error in action_InitializeForNewAssembly when transitioning to Pooled

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "delegation-1")

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
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "delegation-1")

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

func Test_action_SelectTransaction_WhenNotSender_StartsHeartbeatLoop(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Active)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0), nil)
	c.transactionsByID[txn.GetID()] = txn
	c.pooledTransactions = []transaction.CoordinatorTransaction{txn}

	err := action_SelectTransaction(ctx, c, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return c.heartbeatCtx != nil }, 100*time.Millisecond, 5*time.Millisecond, "heartbeat loop should start when not SENDER")
	// Cleanup so test doesn't leak
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
	}
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
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.GetID()).Return()
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
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{existingTxn, newTxn}, "")

	require.NoError(t, err)
}

func Test_addToDelegatedTransactions_PreviousTransactionHandleEventReturnsError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{existingTxn, newTxn}, "")

	require.Error(t, err)
	assert.Equal(t, expectedError, err)
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
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{existingTxn, newTxn}, "")

	require.NoError(t, err)
}

func Test_addToDelegatedTransactions_HandleEventError_CapturesFirstError(t *testing.T) {
	// Tests lines 167-171: when HandleEvent returns an error, it's captured in newTxnError
	// and the function eventually returns it
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	// Return true for hasChainedTransaction - this causes transition to State_Dispatched
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(true, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	// Set up mock transport writer that returns an error from SendDispatched
	// (called by action_NotifyDispatched when transitioning to State_Dispatched)
	mockTransport := transport.NewMockTransportWriter(t)
	expectedError := fmt.Errorf("send dispatched error")
	mockTransport.On("SendDispatched", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(expectedError)
	mockTransport.On("SendDelegationRequestAcknowledgment", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockTransport.On("WaitForDone", mock.Anything).Return().Maybe()
	c.transportWriter = mockTransport

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn}, "delegation-1")

	// The error from HandleEvent should be returned
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func Test_addToDelegatedTransactions_HandleEventError_CapturesOnlyFirstError(t *testing.T) {
	// Tests that when multiple transactions fail HandleEvent, only the first error is captured
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.On("FixedSigningIdentity").Return("")
	builder.GetDomainAPI().On("Domain").Return(mockDomain)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	// Return true for hasChainedTransaction - this causes transition to State_Dispatched
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(true, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	// Set up mock transport writer that returns different errors for each call
	mockTransport := transport.NewMockTransportWriter(t)
	firstError := fmt.Errorf("first error")
	secondError := fmt.Errorf("second error")
	mockTransport.On("SendDispatched", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(firstError).Once()
	mockTransport.On("SendDispatched", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(secondError).Once()
	mockTransport.On("SendDelegationRequestAcknowledgment", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockTransport.On("WaitForDone", mock.Anything).Return().Maybe()
	c.transportWriter = mockTransport

	txn1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()
	txn2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1).BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn1, txn2}, "delegation-1")

	// Only the first error should be returned (lines 168-169 check if newTxnError == nil)
	require.Error(t, err)
	assert.Equal(t, firstError, err)
}
