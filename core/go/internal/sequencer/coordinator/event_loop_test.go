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

// Event loop integration tests for coordinator state machine through the public QueueEvent interface.
// These tests verify the coordinator behavior through the full event loop path.
// For unit tests that test state machine logic in isolation, see state_transitions_test.go

import (
	"context"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	eventLoopTestTimeout  = 100 * time.Millisecond
	eventLoopTestInterval = 1 * time.Millisecond
)

// SyncEvent is a test-only event that signals when it has been dequeued by the event loop.
// This enables deterministic testing of "state stays the same" scenarios without time.Sleep.
type SyncEvent struct {
	common.BaseEvent
	processed chan struct{}
}

func NewSyncEvent() *SyncEvent {
	return &SyncEvent{processed: make(chan struct{})}
}

func (e *SyncEvent) Type() common.EventType { return -1 } // Test-only, not a real event type
func (e *SyncEvent) TypeString() string     { return "SyncEvent" }
func (e *SyncEvent) NotifyProcessed()       { close(e.processed) }
func (e *SyncEvent) Wait()                  { <-e.processed }

// queueAndSync queues an event and waits for it to be processed by the event loop.
// This is useful for testing that state does NOT change after an event.
func queueAndSync(ctx context.Context, c SeqCoordinator, event common.Event) {
	c.QueueEvent(ctx, event)
	sync := NewSyncEvent()
	c.QueueEvent(ctx, sync)
	sync.Wait()
}

// assertEventualStateEventLoop waits for the coordinator to reach the expected state
func assertEventualStateEventLoop(t *testing.T, c SeqCoordinator, expected State, msgAndArgs ...interface{}) {
	assert.Eventually(t, func() bool {
		return c.GetCurrentState() == expected
	}, eventLoopTestTimeout, eventLoopTestInterval, msgAndArgs...)
}

func TestEventLoop_Coordinator_InitializeOK(t *testing.T) {
	ctx := context.Background()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer c.Stop()

	assert.Equal(t, State_Idle, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
}

func TestEventLoop_Coordinator_Idle_ToActive_OnTransactionsDelegated(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.OriginatorIdentityPool(originator)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	// Use mock.Anything for context since event loop uses internal cancelCtx
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	require.Equal(t, State_Idle, c.GetCurrentState())

	// Queue event through public interface
	c.QueueEvent(ctx, &TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})

	assertEventualStateEventLoop(t, c, State_Active,
		"expected transition from Idle to Active")
}

func TestEventLoop_Coordinator_Idle_ToObserving_OnHeartbeatReceived(t *testing.T) {
	ctx := context.Background()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build(ctx)
	defer c.Stop()

	require.Equal(t, State_Idle, c.GetCurrentState())

	c.QueueEvent(ctx, &HeartbeatReceivedEvent{})

	assertEventualStateEventLoop(t, c, State_Observing,
		"expected transition from Idle to Observing")
}

func TestEventLoop_Coordinator_Observing_ToStandby_OnDelegated_IfBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := NewCoordinatorBuilderForTesting(t, State_Observing).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(194) // default tolerance is 5 so this is behind
	// Use mock.Anything for context since event loop uses internal cancelCtx
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})

	assertEventualStateEventLoop(t, c, State_Standby,
		"expected transition from Observing to Standby when behind")
}

func TestEventLoop_Coordinator_Observing_ToElect_OnDelegated_IfNotBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := NewCoordinatorBuilderForTesting(t, State_Observing).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(195) // default tolerance is 5 so this is not behind
	// Use mock.Anything for context since event loop uses internal cancelCtx
	builder.GetTXManager().On("HasChainedTransaction", mock.Anything, mock.Anything).Return(false, nil)
	c, mocks := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})

	assertEventualStateEventLoop(t, c, State_Elect,
		"expected transition from Observing to Elect when not behind")

	// Verify side effect: handover request was sent
	assert.Eventually(t, func() bool {
		return mocks.SentMessageRecorder.HasSentHandoverRequest()
	}, eventLoopTestTimeout, eventLoopTestInterval,
		"expected handover request to be sent")
}

func TestEventLoop_Coordinator_Standby_ToElect_OnNewBlock_IfNotBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := NewCoordinatorBuilderForTesting(t, State_Standby).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(194)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &NewBlockEvent{
		BlockHeight: 195, // default tolerance is 5 so we are not behind
	})

	assertEventualStateEventLoop(t, c, State_Elect,
		"expected transition from Standby to Elect when caught up")
}

func TestEventLoop_Coordinator_Standby_StaysInStandby_OnNewBlock_IfStillBehind(t *testing.T) {
	ctx := context.Background()

	builder := NewCoordinatorBuilderForTesting(t, State_Standby).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(193)
	c, mocks := builder.Build(ctx)
	defer c.Stop()

	queueAndSync(ctx, c, &NewBlockEvent{
		BlockHeight: 194, // still behind
	})

	assert.Equal(t, State_Standby, c.GetCurrentState(),
		"expected to stay in Standby when still behind")
	assert.False(t, mocks.SentMessageRecorder.HasSentHandoverRequest(),
		"handover request should not be sent")
}

func TestEventLoop_Coordinator_Elect_ToPrepared_OnHandover(t *testing.T) {
	ctx := context.Background()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Elect).Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &HandoverReceivedEvent{})

	assertEventualStateEventLoop(t, c, State_Prepared,
		"expected transition from Elect to Prepared")
}

func TestEventLoop_Coordinator_Prepared_ToActive_OnTransactionConfirmed_IfFlushCompleted(t *testing.T) {
	ctx := context.Background()

	builder := NewCoordinatorBuilderForTesting(t, State_Prepared)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	domainAPI := builder.GetDomainAPI()
	domainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})

	c.QueueEvent(ctx, &TransactionConfirmedEvent{
		From:  builder.GetFlushPointSignerAddress(),
		Nonce: builder.GetFlushPointNonce(),
		Hash:  builder.GetFlushPointHash(),
	})

	assertEventualStateEventLoop(t, c, State_Active,
		"expected transition from Prepared to Active when flush completed")
}

func TestEventLoop_Coordinator_Prepared_StaysInPrepared_OnTransactionConfirmed_IfNotFlushCompleted(t *testing.T) {
	ctx := context.Background()

	builder := NewCoordinatorBuilderForTesting(t, State_Prepared)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	otherHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	otherNonce := builder.GetFlushPointNonce() - 1

	queueAndSync(ctx, c, &TransactionConfirmedEvent{
		From:  builder.GetFlushPointSignerAddress(),
		Nonce: otherNonce,
		Hash:  otherHash,
	})

	assert.Equal(t, State_Prepared, c.GetCurrentState(),
		"expected to stay in Prepared when flush not completed")
}

func TestEventLoop_Coordinator_Active_ToIdle_NoTransactionsInFlight(t *testing.T) {
	ctx := context.Background()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})

	assertEventualStateEventLoop(t, c, State_Idle,
		"expected transition from Active to Idle when no transactions in flight")
}

func TestEventLoop_Coordinator_Active_StaysActive_OnTransactionConfirmed_IfTransactionsRemain(t *testing.T) {
	ctx := context.Background()

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	queueAndSync(ctx, c, &TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assert.Equal(t, State_Active, c.GetCurrentState(),
		"expected to stay Active when transactions remain")
}

func TestEventLoop_Coordinator_Active_ToFlush_OnHandoverRequest(t *testing.T) {
	ctx := context.Background()

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &HandoverRequestEvent{
		Requester: "newCoordinator",
	})

	assertEventualStateEventLoop(t, c, State_Flush,
		"expected transition from Active to Flush on handover request")
}

func TestEventLoop_Coordinator_Flush_ToClosing_OnTransactionConfirmed_IfFlushComplete(t *testing.T) {
	ctx := context.Background()

	// One transaction past point of no return, one not
	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Flush).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assertEventualStateEventLoop(t, c, State_Closing,
		"expected transition from Flush to Closing when flush complete")
}

func TestEventLoop_Coordinator_Flush_StaysInFlush_OnTransactionConfirmed_IfNotFlushComplete(t *testing.T) {
	ctx := context.Background()

	// Both transactions past point of no return
	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := NewCoordinatorBuilderForTesting(t, State_Flush).
		Transactions(delegation1, delegation2).
		Build(ctx)
	defer c.Stop()

	queueAndSync(ctx, c, &TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assert.Equal(t, State_Flush, c.GetCurrentState(),
		"expected to stay in Flush when flush not complete")
}

func TestEventLoop_Coordinator_Closing_ToIdle_OnHeartbeatInterval_IfClosingGracePeriodExpired(t *testing.T) {
	ctx := context.Background()

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	builder := NewCoordinatorBuilderForTesting(t, State_Closing).
		HeartbeatsUntilClosingGracePeriodExpires(1).
		Transactions(d)

	config := builder.GetSequencerConfig()
	config.ClosingGracePeriod = confutil.P(5)
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	defer c.Stop()

	c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})

	assertEventualStateEventLoop(t, c, State_Idle,
		"expected transition from Closing to Idle when grace period expired")
}

func TestEventLoop_Coordinator_Closing_StaysInClosing_OnHeartbeatInterval_IfGracePeriodNotExpired(t *testing.T) {
	ctx := context.Background()

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	builder := NewCoordinatorBuilderForTesting(t, State_Closing).
		HeartbeatsUntilClosingGracePeriodExpires(2).
		Transactions(d)
	config := builder.GetSequencerConfig()
	config.ClosingGracePeriod = confutil.P(5)
	builder.OverrideSequencerConfig(config)

	c, _ := builder.Build(ctx)
	defer c.Stop()

	queueAndSync(ctx, c, &common.HeartbeatIntervalEvent{})

	assert.Equal(t, State_Closing, c.GetCurrentState(),
		"expected to stay in Closing when grace period not expired")
}

func TestEventLoop_Coordinator_GracefulShutdownSequence(t *testing.T) {
	// Tests the complete shutdown sequence: Active -> Flush -> Closing -> Idle
	ctx := context.Background()

	delegation := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	builder := NewCoordinatorBuilderForTesting(t, State_Active).
		Transactions(delegation)
	config := builder.GetSequencerConfig()
	config.ClosingGracePeriod = confutil.P(1) // Short grace period for test
	builder.OverrideSequencerConfig(config)

	c, _ := builder.Build(ctx)
	defer c.Stop()

	// Step 1: Handover request -> Active to Flush
	c.QueueEvent(ctx, &HandoverRequestEvent{
		Requester: "newCoordinator",
	})

	assertEventualStateEventLoop(t, c, State_Flush,
		"Step 1: expected transition to Flush")

	// Step 2: Transaction confirmed -> Flush to Closing
	c.QueueEvent(ctx, &TransactionConfirmedEvent{
		From:  delegation.GetSignerAddress(),
		Nonce: *delegation.GetNonce(),
		Hash:  *delegation.GetLatestSubmissionHash(),
	})

	assertEventualStateEventLoop(t, c, State_Closing,
		"Step 2: expected transition to Closing")

	// Step 3: Grace period expires -> Closing to Idle
	c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})

	assertEventualStateEventLoop(t, c, State_Idle,
		"Step 3: expected transition to Idle")
}
