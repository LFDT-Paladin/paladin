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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_MaxInflightTransactions(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxInflightTransactions = confutil.P(5)
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)
	c, _ := builder.Build(ctx)

	// Start by simulating the originator and delegate a transaction to the coordinator
	for i := range 100 {
		transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
		txn := transactionBuilder.BuildSparse()
		err := stateupdate_TransactionsDelegated(ctx, c.state, c.smConfig, c.smCallbacks, &TransactionsDelegatedEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			Originator:   originator,
			Transactions: []*components.PrivateTransaction{txn},
		})

		if i < 5 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			require.ErrorContains(t, err, "PD012642")
		}
	}
}

func TestCoordinator_AddToDelegatedTransactions_NewTransactionError(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Use a valid originator for the transaction builder (it validates immediately)
	validOriginator := "sender@senderNode"
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(validOriginator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Use an invalid originator identity that will cause NewTransaction to return an error
	invalidOriginator := "sender@node1@node2"
	err := stateupdate_TransactionsDelegated(ctx, c.state, c.smConfig, c.smCallbacks, &TransactionsDelegatedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		Originator:   invalidOriginator,
		Transactions: []*components.PrivateTransaction{txn},
	})

	require.Error(t, err, "should return error when NewTransaction fails")
	// Verify that the transaction was not added to transactionsByID
	assert.Equal(t, 0, len(c.state.transactionsByID), "transaction should not be added when NewTransaction fails")
}

func TestCoordinator_AddToDelegatedTransactions_HasChainedTransactionError(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	expectedError := fmt.Errorf("database error checking chained transaction")
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(false, expectedError)
	c, _ := builder.Build(ctx)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Call addToDelegatedTransactions - this should return an error when HasChainedTransaction fails
	err := stateupdate_TransactionsDelegated(ctx, c.state, c.smConfig, c.smCallbacks, &TransactionsDelegatedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		Originator:   originator,
		Transactions: []*components.PrivateTransaction{txn},
	})

	require.Error(t, err, "should return error when HasChainedTransaction fails")
	assert.Equal(t, expectedError, err, "should return the same error from HasChainedTransaction")
	assert.Equal(t, 1, len(c.state.transactionsByID), "transaction is added before HasChainedTransaction check, so it will be in the map even if check fails")
}

func TestCoordinator_AddToDelegatedTransactions_WithChainedTransaction(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(true, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(0) // Stop the dispatcher loop from progressing states
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Call addToDelegatedTransactions - this should call SetChainedTxInProgress() when hasChainedTransaction is true
	err := stateupdate_TransactionsDelegated(ctx, c.state, c.smConfig, c.smCallbacks, &TransactionsDelegatedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		Originator:   originator,
		Transactions: []*components.PrivateTransaction{txn},
	})

	// Verify that no error occurred
	require.NoError(t, err, "should not return error when HasChainedTransaction returns true")

	// Verify that the transaction was added to transactionsByID
	require.Equal(t, 1, len(c.state.transactionsByID), "transaction should be added to transactionsByID")
	coordinatedTxn := c.state.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")

	// Verify that SetChainedTxInProgress() was called by checking the transaction state
	assert.Equal(t, transaction.State_Submitted, coordinatedTxn.GetState(), "transaction should be in State_Submitted when chained transaction is found")
}

func TestCoordinator_AddToDelegatedTransactions_WithoutChainedTransaction(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetTXManager().On("HasChainedTransaction", ctx, mock.Anything).Return(false, nil)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(0) // Stop the dispatcher loop from progressing states
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)

	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Call addToDelegatedTransactions - this should NOT call SetChainedTxInProgress() when hasChainedTransaction is false
	err := stateupdate_TransactionsDelegated(ctx, c.state, c.smConfig, c.smCallbacks, &TransactionsDelegatedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		Originator:   originator,
		Transactions: []*components.PrivateTransaction{txn},
	})

	// Verify that no error occurred
	require.NoError(t, err, "should not return error when HasChainedTransaction returns false")

	// Verify that the transaction was added to transactionsByID
	require.Equal(t, 1, len(c.state.transactionsByID), "transaction should be added to transactionsByID")
	coordinatedTxn := c.state.transactionsByID[txn.ID]
	require.NotNil(t, coordinatedTxn, "transaction should exist in transactionsByID")

	assert.NotEqual(t, transaction.State_Submitted, coordinatedTxn.GetState(), "transaction should NOT be in State_Submitted when chained transaction is not found")
	// The transaction should be in a state that indicates it's ready for normal processing
	assert.Contains(t, []transaction.State{transaction.State_Pooled, transaction.State_PreAssembly_Blocked}, coordinatedTxn.GetState(), "transaction should be in Pooled or PreAssembly_Blocked state when chained transaction is not found")
}
