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
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
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
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

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
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	// Create a dispatched transaction that doesn't exist in memory
	unknownTxID := uuid.New()
	heartbeatEvent.DispatchedTransactions = []*common.SnapshotDispatchedTransaction{
		{
			SnapshotPooledTransaction: common.SnapshotPooledTransaction{
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
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

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
	heartbeatEvent.DispatchedTransactions = []*common.SnapshotDispatchedTransaction{
		{
			SnapshotPooledTransaction: common.SnapshotPooledTransaction{
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
}

func Test_applyHeartbeatReceived_DispatchedTransactionWithNonceOnlySendsNonceAssigned(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

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
	heartbeatEvent.DispatchedTransactions = []*common.SnapshotDispatchedTransaction{
		{
			SnapshotPooledTransaction: common.SnapshotPooledTransaction{
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
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress
	heartbeatEvent.DispatchedTransactions = []*common.SnapshotDispatchedTransaction{
		{
			SnapshotPooledTransaction: common.SnapshotPooledTransaction{
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
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

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
	heartbeatEvent.DispatchedTransactions = []*common.SnapshotDispatchedTransaction{
		{
			SnapshotPooledTransaction: common.SnapshotPooledTransaction{
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
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

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
	heartbeatEvent.DispatchedTransactions = []*common.SnapshotDispatchedTransaction{
		{
			SnapshotPooledTransaction: common.SnapshotPooledTransaction{
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

func Test_guard_HeartbeatThresholdExceeded_NilTimeReturnsTrue(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	// Ensure timeOfMostRecentHeartbeat is nil
	o.timeOfMostRecentHeartbeat = nil

	result := guard_IdleThresholdExceeded(ctx, o)
	assert.True(t, result, "Should return true when timeOfMostRecentHeartbeat is nil")
}

func Test_guard_IdleThresholdExceeded_ThresholdExpiredReturnsTrue(t *testing.T) {
	// Test that guard_HeartbeatThresholdExceeded returns true when threshold has expired
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"

	clock := common.NewMockClock(t)
	initialTime := time.Now()
	clock.On("HasExpired", initialTime, time.Duration(10000*10*time.Millisecond)).Return(true).Once()

	builder := NewOriginatorBuilderForTesting(State_Observing).
		Clock(clock).
		CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	o.timeOfMostRecentHeartbeat = &initialTime

	result := guard_IdleThresholdExceeded(ctx, o)
	assert.True(t, result, "Should return true when threshold has expired")
}

func Test_guard_HeartbeatThresholdExceeded_ThresholdNotExpiredReturnsFalse(t *testing.T) {
	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"

	clock := common.NewMockClock(t)
	initialTime := time.Now()
	clock.On("HasExpired", initialTime, time.Duration(10000*10*time.Millisecond)).Return(false).Once()

	builder := NewOriginatorBuilderForTesting(State_Observing).
		Clock(clock).
		CommitteeMembers(originatorLocator, coordinatorLocator)
	o, _, cleanup := builder.Build(ctx)
	defer cleanup()

	o.timeOfMostRecentHeartbeat = &initialTime

	result := guard_IdleThresholdExceeded(ctx, o)
	assert.False(t, result, "Should return false when threshold has not expired")
}
