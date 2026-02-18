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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_confirmTransaction_HashNotFoundReturnsNil(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	hash := pldtypes.RandBytes32()
	revertReason := pldtypes.HexBytes("")

	err := o.confirmTransaction(ctx, hash, revertReason)
	require.NoError(t, err)
}

func Test_confirmTransaction_NilTransactionIDReturnsError(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	hash := pldtypes.RandBytes32()
	revertReason := pldtypes.HexBytes("")

	// Set up submittedTransactionsByHash with a nil transactionID
	o.submittedTransactionsByHash[hash] = nil

	err := o.confirmTransaction(ctx, hash, revertReason)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found in submitted transactions but nil transaction ID")
}

func Test_confirmTransaction_NilTransactionReturnsError(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	hash := pldtypes.RandBytes32()
	revertReason := pldtypes.HexBytes("")
	txID := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build().GetID()

	// Set up submittedTransactionsByHash with a valid transactionID
	o.submittedTransactionsByHash[hash] = &txID
	// But don't add the transaction to transactionsByID, so it will be nil/not found

	err := o.confirmTransaction(ctx, hash, revertReason)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found in submitted transactions but nil transaction")
}

func Test_confirmTransaction_TransactionNotFoundReturnsError(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	hash := pldtypes.RandBytes32()
	revertReason := pldtypes.HexBytes("")
	unknownTxID := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build().GetID()

	// Set up submittedTransactionsByHash with a transactionID that doesn't exist in transactionsByID
	o.submittedTransactionsByHash[hash] = &unknownTxID

	err := o.confirmTransaction(ctx, hash, revertReason)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found in submitted transactions but nil transaction")
}

func Test_confirmTransaction_Success_EmptyRevertReasonRemovesHash(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)

	// Create a transaction in Submitted state using the transaction builder
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn := txnBuilder.Build()

	// Add the transaction to the originator using the builder
	builder.Transactions(txn)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	// Get the submission hash from the transaction
	submissionHash := txn.GetLatestSubmissionHash()
	require.NotNil(t, submissionHash, "Transaction in Submitted state should have a submission hash")

	// Verify the transaction hash was added to submittedTransactionsByHash (done by builder)
	_, exists := o.submittedTransactionsByHash[*submissionHash]
	require.True(t, exists, "Hash should exist in submittedTransactionsByHash after setup")

	revertReason := pldtypes.HexBytes("")

	err := o.confirmTransaction(ctx, *submissionHash, revertReason)
	require.NoError(t, err)

	// Verify the hash was deleted after confirmation
	_, exists = o.submittedTransactionsByHash[*submissionHash]
	assert.False(t, exists, "Hash should be deleted from submittedTransactionsByHash after confirmation")
}

func Test_confirmTransaction_Success_WithRevertReasonRemovesHash(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)

	// Create a transaction in Submitted state using the transaction builder
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted)
	txn := txnBuilder.Build()

	// Add the transaction to the originator using the builder
	builder.Transactions(txn)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	// Get the submission hash from the transaction
	submissionHash := txn.GetLatestSubmissionHash()
	require.NotNil(t, submissionHash, "Transaction in Submitted state should have a submission hash")

	// Verify the transaction hash was added to submittedTransactionsByHash (done by builder)
	_, exists := o.submittedTransactionsByHash[*submissionHash]
	require.True(t, exists, "Hash should exist in submittedTransactionsByHash after setup")

	revertReason := pldtypes.HexBytes("0x123456")

	err := o.confirmTransaction(ctx, *submissionHash, revertReason)
	require.NoError(t, err)

	// Verify the hash was deleted after confirmation
	_, exists = o.submittedTransactionsByHash[*submissionHash]
	assert.False(t, exists, "Hash should be deleted from submittedTransactionsByHash after confirmation")
}
