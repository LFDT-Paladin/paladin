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

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCoordinator_InitializeOK(t *testing.T) {
	ctx := context.Background()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Idle).Build(ctx)
	defer c.Stop()

	assert.Equal(t, coordinator.State_Idle, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Idle_ToActive_OnTransactionsDelegated(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Idle)
	builder.OriginatorIdentityPool(originator)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	assert.Equal(t, coordinator.State_Idle, c.GetCurrentState())

	c.QueueEvent(ctx, &coordinator.TransactionsDelegatedEvent{
		FromNode:     "testNode",
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Active
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Idle_ToObserving_OnHeartbeatReceived(t *testing.T) {
	ctx := context.Background()
	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Idle).Build(ctx)
	defer c.Stop()

	assert.Equal(t, coordinator.State_Idle, c.GetCurrentState())

	c.QueueEvent(ctx, &coordinator.HeartbeatReceivedEvent{})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Observing
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Observing_ToStandby_OnDelegated_IfBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Observing).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(194) // default tolerance is 5 so this is behind
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &coordinator.TransactionsDelegatedEvent{
		FromNode:     "testNode",
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Standby
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Observing_ToElect_OnDelegated_IfNotBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Observing).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(195) // default tolerance is 5 so this is not behind
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, mocks := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &coordinator.TransactionsDelegatedEvent{
		FromNode:     "testNode",
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Elect
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentHandoverRequest()
	}, 100*time.Millisecond, 1*time.Millisecond, "expected handover request to be sent")

}

func TestCoordinator_Standby_ToElect_OnNewBlock_IfNotBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Standby).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(194)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &coordinator.NewBlockEvent{
		BlockHeight: 195, // default tolerance is 5 in the test setup so we are not behind
	})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Elect
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Standby_NoTransition_OnNewBlock_IfStillBehind(t *testing.T) {
	ctx := context.Background()

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Standby).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(193)
	c, mocks := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &coordinator.NewBlockEvent{
		BlockHeight: 194, // default tolerance is 5 in the test setup so this is still behind
	})

	// Queue a sync event to ensure the previous event has been processed
	sync := statemachine.NewSyncEvent()
	c.QueueEvent(ctx, sync)
	<-sync.Done

	assert.Equal(t, coordinator.State_Standby, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
	assert.False(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "handover request not expected to be sent")
}

func TestCoordinator_Elect_ToPrepared_OnHandover(t *testing.T) {
	ctx := context.Background()
	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Elect).Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &coordinator.HandoverReceivedEvent{})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Prepared
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Prepared_ToActive_OnTransactionConfirmed_IfFlushCompleted(t *testing.T) {
	ctx := context.Background()
	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Prepared)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	domainAPI := builder.GetDomainAPI()
	domainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})

	flushNonce := pldtypes.HexUint64(builder.GetFlushPointNonce())
	c.QueueEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  builder.GetFlushPointSignerAddress(),
		Nonce: &flushNonce,
		Hash:  builder.GetFlushPointHash(),
	})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Active
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())

	//TODO should have other test cases where there are multiple flush points across multiple signers ( and across multiple coordinators?)
	//TODO test case where the nonce and signer match but hash does not.  This should still trigger the transition because there will never be another confirmed transaction for that nonce and signer

}

func TestCoordinator_PreparedNoTransition_OnTransactionConfirmed_IfNotFlushCompleted(t *testing.T) {
	ctx := context.Background()

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Prepared)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	otherHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	otherNonce := pldtypes.HexUint64(builder.GetFlushPointNonce() - 1)

	c.QueueEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  builder.GetFlushPointSignerAddress(),
		Nonce: &otherNonce,
		Hash:  otherHash,
	})

	// Queue a sync event to ensure the previous event has been processed
	sync := statemachine.NewSyncEvent()
	c.QueueEvent(ctx, sync)
	<-sync.Done

	assert.Equal(t, coordinator.State_Prepared, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Active_ToIdle_NoTransactionsInFlight(t *testing.T) {
	ctx := context.Background()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Active).
		Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Idle
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())
}

func TestCoordinator_ActiveNoTransition_OnTransactionConfirmed_IfNotTransactionsEmpty(t *testing.T) {
	ctx := context.Background()

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Active).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	delegation1Nonce := pldtypes.HexUint64(*delegation1.GetNonce())
	c.QueueEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: &delegation1Nonce,
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	// Queue a sync event to ensure the previous event has been processed
	sync := statemachine.NewSyncEvent()
	c.QueueEvent(ctx, sync)
	<-sync.Done

	assert.Equal(t, coordinator.State_Active, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Active_ToFlush_OnHandoverRequest(t *testing.T) {
	ctx := context.Background()

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Active).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &coordinator.HandoverRequestEvent{
		Requester: "newCoordinator",
	})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Flush
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Flush_ToClosing_OnTransactionConfirmed_IfFlushComplete(t *testing.T) {
	ctx := context.Background()

	//We have 2 transactions in flight but only one of them has passed the point of no return so we
	// should consider the flush complete when that one is confirmed
	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Flush).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	delegation1Nonce := pldtypes.HexUint64(*delegation1.GetNonce())
	c.QueueEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: &delegation1Nonce,
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Closing
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())

}

func TestCoordinator_FlushNoTransition_OnTransactionConfirmed_IfNotFlushComplete(t *testing.T) {
	ctx := context.Background()

	//We have 2 transactions in flight and passed the point of no return but only one of them will be confirmed so we should not
	// consider the flush complete

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Flush).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	delegation1Nonce := pldtypes.HexUint64(*delegation1.GetNonce())
	c.QueueEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: &delegation1Nonce,
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	// Queue a sync event to ensure the previous event has been processed
	sync := statemachine.NewSyncEvent()
	c.QueueEvent(ctx, sync)
	<-sync.Done

	assert.Equal(t, coordinator.State_Flush, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Closing_ToIdle_OnHeartbeatInterval_IfClosingGracePeriodExpired(t *testing.T) {
	ctx := context.Background()

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Closing).
		HeartbeatsUntilClosingGracePeriodExpires(1).
		Transactions(d)

	config := builder.GetSequencerConfig()
	config.ClosingGracePeriod = confutil.P(5)
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})

	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == coordinator.State_Idle
	}, 100*time.Millisecond, 1*time.Millisecond, "current state is %s", c.GetCurrentState())

}

func TestCoordinator_ClosingNoTransition_OnHeartbeatInterval_IfNotClosingGracePeriodExpired(t *testing.T) {
	ctx := context.Background()

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Closing).
		HeartbeatsUntilClosingGracePeriodExpires(2).
		Transactions(d)
	config := builder.GetSequencerConfig()
	config.ClosingGracePeriod = confutil.P(5)
	builder.OverrideSequencerConfig(config)

	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})

	// Queue a sync event to ensure the previous event has been processed
	sync := statemachine.NewSyncEvent()
	c.QueueEvent(ctx, sync)
	<-sync.Done

	assert.Equal(t, coordinator.State_Closing, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}
