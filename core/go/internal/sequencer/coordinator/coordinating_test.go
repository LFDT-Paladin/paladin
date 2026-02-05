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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_TransactionConfirmed_TransactionNotTracked_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Build(ctx)

	nonExistentTxID := uuid.New()
	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: nonExistentTxID,
		From: pldtypes.RandAddress(),
	})

	require.NoError(t, err)
	assert.Empty(t, c.transactionsByID)
}

func Test_action_TransactionConfirmed_TransactionTracked_HandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build(ctx)
	mocks.EngineIntegration.On("ResetTransactions", ctx, txn.GetID()).Return(nil)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: hash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState())
}

func Test_action_TransactionConfirmed_TransactionTracked_NilSubmissionHash_HandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build(ctx)
	mocks.EngineIntegration.On("ResetTransactions", ctx, txn.GetID()).Return(nil)
	builder := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn)
	c, _ := builder.Build(ctx)

	require.NoError(t, action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: pldtypes.Bytes32(pldtypes.RandBytes(32)),
	}))
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState())
}

func Test_action_TransactionConfirmed_TransactionTracked_MatchingHash_HandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build(ctx)
	mocks.EngineIntegration.On("ResetTransactions", ctx, txn.GetID()).Return(nil)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	submissionHash := txn.GetLatestSubmissionHash()
	require.NotNil(t, submissionHash, "builder sets submission hash for State_Submitted")

	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: *submissionHash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState())
}

func Test_action_TransactionConfirmed_TransactionTracked_DifferentHash_HandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build(ctx)
	mocks.EngineIntegration.On("ResetTransactions", ctx, txn.GetID()).Return(nil)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	differentHash := pldtypes.Bytes32(pldtypes.RandBytes(32))

	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: differentHash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState())
}

func Test_action_TransactionConfirmed_TransactionTracked_EventNotHandledByTransaction_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: hash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState())
}

func Test_action_TransactionConfirmed_MultipleTransactions_EachHandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	txn1, mocks1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build(ctx)
	mocks1.EngineIntegration.On("ResetTransactions", ctx, txn1.GetID()).Return(nil)
	txn2, mocks2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build(ctx)
	mocks2.EngineIntegration.On("ResetTransactions", ctx, txn2.GetID()).Return(nil)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn1, txn2).
		Build(ctx)

	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn1.GetID(),
		Hash: pldtypes.Bytes32(pldtypes.RandBytes(32)),
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn1.GetCurrentState())

	err = action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn2.GetID(),
		From:  nil,
		Nonce: nil,
		Hash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn2.GetCurrentState())
}

func Test_addToDelegatedTransactions_NewTransactionError_ReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build(ctx)
	mocks.TXManager.On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)

	validOriginator := "sender@senderNode"
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(validOriginator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	invalidOriginator := "sender@node1@node2"
	err := c.addToDelegatedTransactions(ctx, invalidOriginator, []*components.PrivateTransaction{txn})

	require.Error(t, err, "should return error when NewTransaction fails")
	assert.Equal(t, 0, len(c.transactionsByID), "transaction should not be added when NewTransaction fails")
}

func Test_addToDelegatedTransactions_HasChainedTransactionError_ReturnsError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build(ctx)
	expectedError := fmt.Errorf("database error checking chained transaction")

	mocks.TXManager.On("HasChainedTransaction", ctx, mock.Anything).Return(false, expectedError)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

	require.Error(t, err, "should return error when HasChainedTransaction fails")
	assert.Equal(t, expectedError, err, "should return the same error from HasChainedTransaction")
	assert.Len(t, c.transactionsByID, 0, "when HasChainedTransaction fails, the transaction is not added to the map")
}

func Test_addToDelegatedTransactions_WithChainedTransaction_AddsTransactionInSubmittedState(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.TXManager.On("HasChainedTransaction", ctx, mock.Anything).Return(true, nil)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

	require.NoError(t, err, "should not return error when HasChainedTransaction returns true")
	require.Len(t, c.transactionsByID, 1, "transaction should be added to transactionsByID")
	coordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")
	assert.Equal(t, transaction.State_Submitted, coordinatedTxn.GetCurrentState(), "transaction should be in State_Submitted when chained transaction is found")
}

func Test_addToDelegatedTransactions_WithoutChainedTransaction_AddsTransactionInPooledState(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.TXManager.On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})

	require.NoError(t, err, "should not return error when HasChainedTransaction returns false")
	require.Len(t, c.transactionsByID, 1, "transaction should be added to transactionsByID")
	coordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")
	assert.NotEqual(t, transaction.State_Submitted, coordinatedTxn.GetCurrentState(), "transaction should NOT be in State_Submitted when chained transaction is not found")
	assert.Contains(t, []transaction.State{transaction.State_Pooled, transaction.State_PreAssembly_Blocked, transaction.State_Assembling}, coordinatedTxn.GetCurrentState(), "transaction should be in Pooled, PreAssembly_Blocked, or Assembling state when chained transaction is not found")
}

func Test_addToDelegatedTransactions_DuplicateTransaction_SkipsAndReturnsNoError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.TXManager.On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	err := c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})
	require.NoError(t, err, "should not return error on first add")
	require.Len(t, c.transactionsByID, 1, "transaction should be added to transactionsByID")
	firstCoordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, firstCoordinatedTxn, "transaction should exist in transactionsByID")

	err = c.addToDelegatedTransactions(ctx, originator, []*components.PrivateTransaction{txn})
	require.NoError(t, err, "should not return error when adding duplicate transaction")
	assert.Len(t, c.transactionsByID, 1, "duplicate transaction should be skipped, count should remain 1")
	secondCoordinatedTxn := c.transactionsByID[txn.ID]
	require.NotNil(t, secondCoordinatedTxn, "transaction should still exist in transactionsByID")
	assert.Equal(t, firstCoordinatedTxn, secondCoordinatedTxn, "duplicate transaction should not replace existing transaction")
}

func Test_addTransactionToBackOfPool_WhenNotInPool_Appends(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	c.addTransactionToBackOfPool(txn)

	require.Len(t, c.pooledTransactions, 1, "pool should contain one transaction")
	assert.Equal(t, txn, c.pooledTransactions[0])
}

func Test_addTransactionToBackOfPool_WhenAlreadyInPool_DoesNotDuplicate(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	c.addTransactionToBackOfPool(txn)
	c.addTransactionToBackOfPool(txn)

	require.Len(t, c.pooledTransactions, 1, "pool should not duplicate transaction")
	assert.Equal(t, txn, c.pooledTransactions[0])
}

func Test_action_TransactionStateTransition_ToPooled_AddsToBackOfPool(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	err := action_TransactionStateTransition(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txn.GetID(),
		To:            transaction.State_Pooled,
	})
	require.NoError(t, err)
	require.Len(t, c.pooledTransactions, 1, "transaction should be added to pool")
	assert.Equal(t, txn, c.pooledTransactions[0])
}

func Test_action_TransactionStateTransition_ToReadyForDispatch_QueuesForDispatch(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	err := action_TransactionStateTransition(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txn.GetID(),
		To:            transaction.State_Ready_For_Dispatch,
	})
	require.NoError(t, err)
}

func Test_action_TransactionStateTransition_ToFinal_RemovesFromMap(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build(ctx)
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Transactions(txn).
		Build(ctx)

	err := action_TransactionStateTransition(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txn.GetID(),
		To:            transaction.State_Final,
	})
	require.NoError(t, err)
	_, ok := c.transactionsByID[txn.GetID()]
	assert.False(t, ok, "transaction should be removed from map")
}

func Test_addToDelegatedTransactions_WhenMaxInflightReached_ReturnsError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxInflightTransactions = confutil.P(1)
	config.MaxDispatchAhead = confutil.P(-1)
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.TXManager.On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)

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
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	}).Maybe()

	err := action_SelectTransaction(ctx, c, nil)
	require.NoError(t, err)
}

func Test_action_SelectTransaction_WhenNotSender_StartsHeartbeatLoop(t *testing.T) {
	ctx := context.Background()
	txn, txnMocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	txnMocks.EngineIntegration.On("GetStateLocks", mock.Anything).Return(nil, nil)
	txnMocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(100), nil)
	builder := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(txn)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	defer func() {
		if c.heartbeatCancel != nil {
			c.heartbeatCancel()
		}
	}()

	err := action_SelectTransaction(ctx, c, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return c.heartbeatCtx != nil }, 100*time.Millisecond, 5*time.Millisecond, "heartbeat loop should start when not SENDER")
}
