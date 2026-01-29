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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_Action_TransactionConfirmed_TransactionNotTracked_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// TxID not in coordinator: action returns nil (no error), logs warning
	nonExistentTxID := uuid.New()
	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: nonExistentTxID,
		From: pldtypes.RandAddress(),
	})

	require.NoError(t, err)
	assert.Empty(t, c.transactionsByID)
}

func TestCoordinator_Action_TransactionConfirmed_TransactionTracked_HandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Transaction in coordinator, looked up by TxID only; HandleEvent succeeds → action returns nil, txn moves to State_Confirmed
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn := txBuilder.Build()
	c.transactionsByID[txn.GetID()] = txn

	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: hash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState())
}

func TestCoordinator_Action_TransactionConfirmed_TransactionTracked_NilSubmissionHash_HandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Transaction has no submission hash (chained private tx); action still dispatches ConfirmedEvent, HandleEvent succeeds
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID[txn.GetID()] = txn

	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: hash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState())
}

func TestCoordinator_Action_TransactionConfirmed_TransactionTracked_MatchingHash_HandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Transaction has submission hash (builder default for State_Submitted); event hash matches; HandleEvent succeeds
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID[txn.GetID()] = txn
	submissionHash := txn.GetLatestSubmissionHash()
	require.NotNil(t, submissionHash, "builder sets submission hash for State_Submitted")

	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: *submissionHash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState())
}

func TestCoordinator_Action_TransactionConfirmed_TransactionTracked_DifferentHash_HandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Transaction has submission hash (builder default); event hash differs (action still confirms, logs debug); HandleEvent succeeds
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID[txn.GetID()] = txn
	differentHash := pldtypes.Bytes32(pldtypes.RandBytes(32))

	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: differentHash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState())
}

func TestCoordinator_Action_TransactionConfirmed_TransactionTracked_EventNotHandledByTransaction_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	// Transaction in State_Pooled does not handle ConfirmedEvent; statemachine returns nil (event not applicable); action returns nil, state unchanged
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()
	c.transactionsByID[txn.GetID()] = txn

	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn.GetID(),
		Hash: hash,
	})

	require.NoError(t, err)
	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState())
}

func TestCoordinator_Action_TransactionConfirmed_MultipleTransactions_EachHandleEventSucceeds(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn1 := txBuilder1.Build()
	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn2 := txBuilder2.Build()
	c.transactionsByID[txn1.GetID()] = txn1
	c.transactionsByID[txn2.GetID()] = txn2

	// Confirm first by TxID; HandleEvent succeeds
	err := action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID: txn1.GetID(),
		Hash: pldtypes.Bytes32(pldtypes.RandBytes(32)),
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn1.GetCurrentState())

	// Confirm second by TxID; HandleEvent succeeds
	err = action_TransactionConfirmed(ctx, c, &TransactionConfirmedEvent{
		TxID:  txn2.GetID(),
		From:  nil,
		Nonce: nil,
		Hash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn2.GetCurrentState())
}
