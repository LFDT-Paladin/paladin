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

package spec

import (
	"context"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/stretchr/testify/assert"
)

func TestStateMachine_InitializeOK(t *testing.T) {
	ctx := context.Background()
	o, _ := originator.NewOriginatorBuilderForTesting(originator.State_Idle).Build(ctx)
	defer o.Stop()
	assert.Equal(t, originator.State_Idle, o.GetCurrentState(), "current state is %s", o.GetCurrentState().String())
}

func TestStateMachine_Idle_ToObserving_OnHeartbeatReceived(t *testing.T) {
	ctx := context.Background()
	builder := originator.NewOriginatorBuilderForTesting(originator.State_Idle)
	o, _ := builder.Build(ctx)
	defer o.Stop()
	assert.Equal(t, originator.State_Idle, o.GetCurrentState())

	heartbeatEvent := &originator.HeartbeatReceivedEvent{}
	heartbeatEvent.From = "coordinator"
	ca := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &ca
	o.QueueEvent(ctx, heartbeatEvent)
	assert.Eventually(t, func() bool { return o.GetCurrentState() == originator.State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", o.GetCurrentState().String())
}

func TestStateMachine_Idle_ToSending_OnTransactionCreated(t *testing.T) {
	ctx := context.Background()
	builder := originator.NewOriginatorBuilderForTesting(originator.State_Idle)
	o, mocks := builder.Build(ctx)
	defer o.Stop()
	assert.Equal(t, originator.State_Idle, o.GetCurrentState())

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator("sender@node1").Build()
	o.QueueEvent(ctx, &originator.TransactionCreatedEvent{Transaction: txn})
	assert.Eventually(t, func() bool { return o.GetCurrentState() == originator.State_Sending }, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", o.GetCurrentState().String())
	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "Delegation request should be sent")
}

func TestStateMachine_Observing_ToSending_OnTransactionCreated(t *testing.T) {
	ctx := context.Background()
	builder := originator.NewOriginatorBuilderForTesting(originator.State_Observing)
	o, mocks := builder.Build(ctx)
	defer o.Stop()

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator("sender@node1").Build()
	o.QueueEvent(ctx, &originator.TransactionCreatedEvent{Transaction: txn})
	assert.Eventually(t, func() bool { return o.GetCurrentState() == originator.State_Sending }, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", o.GetCurrentState().String())
	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "Delegation request should be sent")
}

func TestStateMachine_Sending_ToObserving_OnTransactionConfirmed_IfNoTransactionsInflight(t *testing.T) {
	ctx := context.Background()

	soleTransaction := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	o, _ := originator.NewOriginatorBuilderForTesting(originator.State_Sending).
		Transactions(soleTransaction).
		Build(ctx)
	defer o.Stop()

	o.QueueEvent(ctx, &originator.TransactionConfirmedEvent{
		From:  soleTransaction.GetSignerAddress(),
		Nonce: *soleTransaction.GetNonce(),
		Hash:  *soleTransaction.GetLatestSubmissionHash(),
	})
	assert.Eventually(t, func() bool { return o.GetCurrentState() == originator.State_Observing }, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", o.GetCurrentState().String())
}

func TestStateMachine_Sending_NoTransition_OnTransactionConfirmed_IfHasTransactionsInflight(t *testing.T) {
	ctx := context.Background()
	txn1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	txn2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	o, _ := originator.NewOriginatorBuilderForTesting(originator.State_Sending).
		Transactions(txn1, txn2).
		Build(ctx)
	defer o.Stop()

	o.QueueEvent(ctx, &originator.TransactionConfirmedEvent{
		From:  txn1.GetSignerAddress(),
		Nonce: *txn1.GetNonce(),
		Hash:  *txn1.GetLatestSubmissionHash(),
	})
	sync := statemachine.NewSyncEvent()
	o.QueueEvent(ctx, sync)
	<-sync.Done
	assert.Equal(t, originator.State_Sending, o.GetCurrentState(), "current state is %s", o.GetCurrentState().String())
}

func TestStateMachine_Observing_ToIdle_OnHeartbeatInterval_IfHeartbeatThresholdExpired(t *testing.T) {
	ctx := context.Background()
	builder := originator.NewOriginatorBuilderForTesting(originator.State_Observing)
	o, mocks := builder.Build(ctx)
	defer o.Stop()

	heartbeatEvent := &originator.HeartbeatReceivedEvent{}
	heartbeatEvent.From = "coordinator"
	ca := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &ca
	o.QueueEvent(ctx, heartbeatEvent)
	sync := statemachine.NewSyncEvent()
	o.QueueEvent(ctx, sync)
	<-sync.Done

	mocks.Clock.Advance(builder.GetCoordinatorHeartbeatThresholdMs() + 1)

	o.QueueEvent(ctx, &originator.HeartbeatIntervalEvent{})
	assert.Eventually(t, func() bool { return o.GetCurrentState() == originator.State_Idle }, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", o.GetCurrentState().String())
}

func TestStateMachine_Observing_NoTransition_OnHeartbeatInterval_IfHeartbeatThresholdNotExpired(t *testing.T) {
	ctx := context.Background()
	builder := originator.NewOriginatorBuilderForTesting(originator.State_Observing)
	o, mocks := builder.Build(ctx)
	defer o.Stop()

	heartbeatEvent := &originator.HeartbeatReceivedEvent{}
	heartbeatEvent.From = "coordinator"
	ca := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &ca
	o.QueueEvent(ctx, heartbeatEvent)
	sync := statemachine.NewSyncEvent()
	o.QueueEvent(ctx, sync)
	<-sync.Done

	mocks.Clock.Advance(builder.GetCoordinatorHeartbeatThresholdMs() - 1)

	o.QueueEvent(ctx, &originator.HeartbeatIntervalEvent{})
	sync2 := statemachine.NewSyncEvent()
	o.QueueEvent(ctx, sync2)
	<-sync2.Done
	assert.Equal(t, originator.State_Observing, o.GetCurrentState(), "current state is %s", o.GetCurrentState().String())
}

func TestStateMachine_Sending_DoDelegateTransactions_OnHeartbeatReceived_IfHasDroppedTransaction(t *testing.T) {
	ctx := context.Background()
	coordinatorLocator := "coordinator@node1"

	builder := originator.NewOriginatorBuilderForTesting(originator.State_Sending)
	o, mocks := builder.Build(ctx)
	defer o.Stop()
	txn1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator("sender@node1").Build()
	o.QueueEvent(ctx, &originator.TransactionCreatedEvent{Transaction: txn1})
	assert.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Delegation request should be sent")

	txn2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator("sender@node1").Build()
	o.QueueEvent(ctx, &originator.TransactionCreatedEvent{Transaction: txn2})
	assert.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Delegation request should be sent")

	mocks.SentMessageRecorder.Reset(ctx)

	// Only one of the delegated transactions are included in the heartbeat
	heartbeatEvent := &originator.HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	ca := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &ca
	heartbeatEvent.PooledTransactions = []*common.Transaction{
		{
			ID:         txn1.ID,
			Originator: "sender@node1",
		},
	}

	o.QueueEvent(ctx, heartbeatEvent)
	assert.Eventually(t, func() bool { return mocks.SentMessageRecorder.HasSentDelegationRequest() }, 100*time.Millisecond, 1*time.Millisecond, "Delegation request should be sent after heartbeat")
}
