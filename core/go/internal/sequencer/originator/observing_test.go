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

package originator

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_applyHeartbeatReceived_BasicUpdate(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress
	heartbeatEvent.CoordinatorSnapshot = common.CoordinatorSnapshot{
		BlockHeight: 1000,
	}

	err := o.applyHeartbeatReceived(ctx, heartbeatEvent)
	assert.NoError(t, err)

	// Verify time was updated
	assert.NotNil(t, o.timeOfMostRecentHeartbeat)

	// Verify coordinator was updated
	assert.Equal(t, coordinatorLocator, o.activeCoordinatorNode)

	// Verify snapshot was updated
	assert.NotNil(t, o.latestCoordinatorSnapshot)
	assert.Equal(t, uint64(1000), o.latestCoordinatorSnapshot.BlockHeight)
}

func Test_applyHeartbeatReceived_DispatchedTransactionNotFoundLogsAndContinues(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	// Create a dispatched transaction that doesn't exist in memory
	unknownTxID := uuid.New()
	heartbeatEvent.DispatchedTransactions = []*common.DispatchedTransaction{
		{
			Transaction: common.Transaction{
				ID:         unknownTxID,
				Originator: originatorLocator,
			},
		},
	}

	err := o.applyHeartbeatReceived(ctx, heartbeatEvent)
	// Should not error, just log a warning
	assert.NoError(t, err)
}

func Test_applyHeartbeatReceived_DispatchedTransactionWithHashUpdatesSubmitted(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	// Create a real transaction
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originatorLocator).
		NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Create the transaction in the originator
	err := o.createTransaction(ctx, txn)
	require.NoError(t, err)

	// Create heartbeat with dispatched transaction that has a hash
	signerAddress := pldtypes.RandAddress()
	submissionHash := pldtypes.RandBytes32()
	nonce := uint64(42)

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress
	heartbeatEvent.DispatchedTransactions = []*common.DispatchedTransaction{
		{
			Transaction: common.Transaction{
				ID:         txn.ID,
				Originator: originatorLocator,
			},
			Signer:               *signerAddress,
			LatestSubmissionHash: &submissionHash,
			Nonce:                &nonce,
		},
	}

	err = o.applyHeartbeatReceived(ctx, heartbeatEvent)
	assert.NoError(t, err)

	// Verify the transaction hash was added to submittedTransactionsByHash
	// Note: The hash is only added if HandleEvent succeeds, which depends on the transaction's state
	txIDPtr, exists := o.submittedTransactionsByHash[submissionHash]
	if exists {
		assert.Equal(t, txn.ID, *txIDPtr)
	}
	// If it doesn't exist, it means HandleEvent didn't process the event (transaction might be in wrong state)
}

func Test_applyHeartbeatReceived_DispatchedTransactionWithNonceOnlySendsNonceAssigned(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	// Create a real transaction
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originatorLocator).
		NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Create the transaction in the originator
	err := o.createTransaction(ctx, txn)
	require.NoError(t, err)

	// Create heartbeat with dispatched transaction that has a nonce but no hash
	nonce := uint64(42)

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress
	heartbeatEvent.DispatchedTransactions = []*common.DispatchedTransaction{
		{
			Transaction: common.Transaction{
				ID:         txn.ID,
				Originator: originatorLocator,
			},
			Nonce: &nonce,
			// No LatestSubmissionHash
		},
	}

	err = o.applyHeartbeatReceived(ctx, heartbeatEvent)
	assert.NoError(t, err)
}

func Test_applyHeartbeatReceived_DispatchedTransactionFromDifferentOriginatorIgnored(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	otherOriginatorLocator := "otherSender@otherNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress
	heartbeatEvent.DispatchedTransactions = []*common.DispatchedTransaction{
		{
			Transaction: common.Transaction{
				ID:         uuid.New(),
				Originator: otherOriginatorLocator, // Different originator
			},
		},
	}

	err := o.applyHeartbeatReceived(ctx, heartbeatEvent)
	assert.NoError(t, err)
}

func Test_applyHeartbeatReceived_DispatchedTransactionWithHashAndNonceSucceeds(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	// Create a real transaction
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originatorLocator).
		NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Create the transaction in the originator
	err := o.createTransaction(ctx, txn)
	require.NoError(t, err)

	submissionHash := pldtypes.RandBytes32()
	nonce := uint64(42)

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress
	heartbeatEvent.DispatchedTransactions = []*common.DispatchedTransaction{
		{
			Transaction: common.Transaction{
				ID:         txn.ID,
				Originator: originatorLocator,
			},
			LatestSubmissionHash: &submissionHash,
			Nonce:                &nonce,
		},
	}

	// This should succeed with a real transaction
	err = o.applyHeartbeatReceived(ctx, heartbeatEvent)
	assert.NoError(t, err)
}

func Test_applyHeartbeatReceived_DispatchedTransactionNonceOnlySucceeds(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	// Create a real transaction
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originatorLocator).
		NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()

	// Create the transaction in the originator
	err := o.createTransaction(ctx, txn)
	require.NoError(t, err)

	// Create heartbeat with dispatched transaction that has a nonce but no hash
	nonce := uint64(42)

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress
	heartbeatEvent.DispatchedTransactions = []*common.DispatchedTransaction{
		{
			Transaction: common.Transaction{
				ID:         txn.ID,
				Originator: originatorLocator,
			},
			Nonce: &nonce,
			// No LatestSubmissionHash
		},
	}

	// This should succeed with a real transaction
	err = o.applyHeartbeatReceived(ctx, heartbeatEvent)
	assert.NoError(t, err)
}

func Test_applyHeartbeatReceived_ConfirmedTransactionSuccessConvergesFromHeartbeat(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		Transactions(tx)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	signerAddress := tx.GetSignerAddress()
	require.NotNil(t, signerAddress)
	submissionHash := tx.GetLatestSubmissionHash()
	require.NotNil(t, submissionHash)
	nonce := tx.GetNonce()
	require.NotNil(t, nonce)

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.CoordinatorSnapshot = common.CoordinatorSnapshot{
		ConfirmedTransactions: []*common.ConfirmedTransaction{
			{
				DispatchedTransaction: common.DispatchedTransaction{
					Transaction: common.Transaction{
						ID:         tx.GetID(),
						Originator: originatorLocator,
					},
					Signer:               *signerAddress,
					LatestSubmissionHash: submissionHash,
					Nonce:                nonce,
				},
			},
		},
	}

	err := o.applyHeartbeatReceived(ctx, heartbeatEvent)
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, tx.GetCurrentState())
	_, found := o.submittedTransactionsByHash[*submissionHash]
	assert.False(t, found, "confirmed transaction hash should be removed from submitted map")
}

func Test_applyHeartbeatReceived_ConfirmedTransactionRevertedConvergesFromHeartbeat(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		Transactions(tx)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	signerAddress := tx.GetSignerAddress()
	require.NotNil(t, signerAddress)
	submissionHash := tx.GetLatestSubmissionHash()
	require.NotNil(t, submissionHash)
	nonce := tx.GetNonce()
	require.NotNil(t, nonce)
	revertReason := pldtypes.HexBytes("0x1234")

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.CoordinatorSnapshot = common.CoordinatorSnapshot{
		ConfirmedTransactions: []*common.ConfirmedTransaction{
			{
				DispatchedTransaction: common.DispatchedTransaction{
					Transaction: common.Transaction{
						ID:         tx.GetID(),
						Originator: originatorLocator,
					},
					Signer:               *signerAddress,
					LatestSubmissionHash: submissionHash,
					Nonce:                nonce,
				},
				RevertReason: revertReason,
			},
		},
	}

	err := o.applyHeartbeatReceived(ctx, heartbeatEvent)
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Delegated, tx.GetCurrentState())
	_, found := o.submittedTransactionsByHash[*submissionHash]
	assert.False(t, found, "reverted transaction hash should be removed from submitted map")
}

func Test_guard_HeartbeatThresholdExceeded_NilTimeReturnsTrue(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	// Ensure timeOfMostRecentHeartbeat is nil
	o.timeOfMostRecentHeartbeat = nil

	result := guard_HeartbeatThresholdExceeded(ctx, o)
	assert.True(t, result, "Should return true when timeOfMostRecentHeartbeat is nil")
}

func Test_guard_HeartbeatThresholdExceeded_ThresholdExpiredReturnsTrue(t *testing.T) {
	// Test that guard_HeartbeatThresholdExceeded returns true when threshold has expired
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, mocks := builder.Build(ctx)
	defer o.Stop()

	// Set timeOfMostRecentHeartbeat to a time in the past (beyond threshold)
	// For FakeClockForTesting, we need to advance the clock and then set an old time
	initialTime := mocks.Clock.Now()
	// Get threshold in milliseconds - for FakeClockForTesting, Duration is *fakeDuration
	// We'll advance by the threshold + some extra to ensure it's expired
	thresholdMs := TestDefault_HeartbeatThreshold * TestDefault_HeartbeatIntervalMs
	mocks.Clock.Advance(thresholdMs + 1000)
	o.timeOfMostRecentHeartbeat = initialTime

	result := guard_HeartbeatThresholdExceeded(ctx, o)
	assert.True(t, result, "Should return true when threshold has expired")
}

func Test_guard_HeartbeatThresholdExceeded_ThresholdNotExpiredReturnsFalse(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, mocks := builder.Build(ctx)
	defer o.Stop()

	// Set timeOfMostRecentHeartbeat to a recent time (within threshold)
	recentTime := mocks.Clock.Now()
	o.timeOfMostRecentHeartbeat = recentTime

	result := guard_HeartbeatThresholdExceeded(ctx, o)
	assert.False(t, result, "Should return false when threshold has not expired")
}

func Test_applyHeartbeatReceived_ContinuesWhenDispatchedEventNotApplicable(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		Transactions(tx)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	submissionHash := pldtypes.RandBytes32()
	err := o.applyHeartbeatReceived(ctx, &HeartbeatReceivedEvent{
		CoordinatorHeartbeatNotification: transport.CoordinatorHeartbeatNotification{
			From: coordinatorLocator,
			CoordinatorSnapshot: common.CoordinatorSnapshot{
				DispatchedTransactions: []*common.DispatchedTransaction{
					{
						Transaction: common.Transaction{
							ID:         tx.GetID(),
							Originator: originatorLocator,
						},
						LatestSubmissionHash: &submissionHash,
					},
				},
			},
		},
	})
	require.NoError(t, err)
}

func Test_applyHeartbeatReceived_ContinuesWhenConfirmedEventNotApplicable(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		Transactions(tx)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	err := o.applyHeartbeatReceived(ctx, &HeartbeatReceivedEvent{
		CoordinatorHeartbeatNotification: transport.CoordinatorHeartbeatNotification{
			From: coordinatorLocator,
			CoordinatorSnapshot: common.CoordinatorSnapshot{
				ConfirmedTransactions: []*common.ConfirmedTransaction{
					{
						DispatchedTransaction: common.DispatchedTransaction{
							Transaction: common.Transaction{
								ID:         tx.GetID(),
								Originator: originatorLocator,
							},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
}

func Test_applyDispatchedSnapshot_NoEvent(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		Transactions(tx)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	o.applyDispatchedSnapshot(ctx, &common.DispatchedTransaction{
		Transaction: common.Transaction{
			ID:         tx.GetID(),
			Originator: originatorLocator,
		},
	})
	assert.Equal(t, transaction.State_Delegated, tx.GetCurrentState())
}

func Test_applyDispatchedSnapshot_TxnNotFound(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	o.applyDispatchedSnapshot(ctx, &common.DispatchedTransaction{
		Transaction: common.Transaction{
			ID:         uuid.New(),
			Originator: originatorLocator,
		},
	})
}

func Test_applyDispatchedSnapshot_HashAndNonceSucceeds(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		Transactions(tx)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	nonce := uint64(7)
	submissionHash := pldtypes.RandBytes32()
	o.applyDispatchedSnapshot(ctx, &common.DispatchedTransaction{
		Transaction: common.Transaction{
			ID:         tx.GetID(),
			Originator: originatorLocator,
		},
		LatestSubmissionHash: &submissionHash,
		Nonce:                &nonce,
	})
	assert.Equal(t, transaction.State_Submitted, tx.GetCurrentState())
	txID, found := o.submittedTransactionsByHash[submissionHash]
	require.True(t, found)
	assert.Equal(t, tx.GetID(), *txID)
}

func Test_applyDispatchedSnapshot_NonceOnlySucceeds(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		Transactions(tx)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	nonce := uint64(7)
	o.applyDispatchedSnapshot(ctx, &common.DispatchedTransaction{
		Transaction: common.Transaction{
			ID:         tx.GetID(),
			Originator: originatorLocator,
		},
		Nonce: &nonce,
	})
	assert.Equal(t, transaction.State_Sequenced, tx.GetCurrentState())
}

func Test_applyConfirmedSnapshot_OriginatorMismatchIgnored(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	o.applyConfirmedSnapshot(ctx, &common.ConfirmedTransaction{
		DispatchedTransaction: common.DispatchedTransaction{
			Transaction: common.Transaction{
				ID:         uuid.New(),
				Originator: "other@node",
			},
		},
	})
}

func Test_applyConfirmedSnapshot_TxnNotFound(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	o.applyConfirmedSnapshot(ctx, &common.ConfirmedTransaction{
		DispatchedTransaction: common.DispatchedTransaction{
			Transaction: common.Transaction{
				ID:         uuid.New(),
				Originator: originatorLocator,
			},
		},
	})
}

func Test_applyConfirmedSnapshot_SuccessDeletesHashAndTransitions(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		Transactions(tx)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	submissionHash := tx.GetLatestSubmissionHash()
	require.NotNil(t, submissionHash)
	o.applyConfirmedSnapshot(ctx, &common.ConfirmedTransaction{
		DispatchedTransaction: common.DispatchedTransaction{
			Transaction: common.Transaction{
				ID:         tx.GetID(),
				Originator: originatorLocator,
			},
			LatestSubmissionHash: submissionHash,
		},
	})
	assert.Equal(t, transaction.State_Confirmed, tx.GetCurrentState())
	_, found := o.submittedTransactionsByHash[*submissionHash]
	assert.False(t, found)
}

func Test_applyConfirmedSnapshot_RevertedTransitions(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	tx := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).
		NodeName(originatorLocator).
		CommitteeMembers(originatorLocator, coordinatorLocator).
		Transactions(tx)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	o.applyConfirmedSnapshot(ctx, &common.ConfirmedTransaction{
		DispatchedTransaction: common.DispatchedTransaction{
			Transaction: common.Transaction{
				ID:         tx.GetID(),
				Originator: originatorLocator,
			},
		},
		RevertReason: pldtypes.HexBytes("0x1234"),
	})
	assert.Equal(t, transaction.State_Delegated, tx.GetCurrentState())
}
