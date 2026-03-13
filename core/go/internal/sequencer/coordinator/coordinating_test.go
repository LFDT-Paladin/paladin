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
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
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
	err := c.addToDelegatedTransactions(ctx, invalidOriginator, []*components.PrivateTransaction{txn})

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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})
	require.NoError(t, err, "should not return error on first add")
	require.Equal(t, 1, len(c.transactionsByID), "transaction should be added to transactionsByID")
	firstCoordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, firstCoordinatedTxn, "transaction should exist in transactionsByID")

	err = c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})
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

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn1})
	require.NoError(t, err)
	require.Len(t, c.transactionsByID, 1)

	err = c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn2})
	require.Error(t, err, "should return error when max inflight reached")
	assert.Len(t, c.transactionsByID, 1, "second transaction should not be added")
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
