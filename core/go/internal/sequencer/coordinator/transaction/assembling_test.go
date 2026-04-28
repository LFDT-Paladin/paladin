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

package transaction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/dependencytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// assemblingSharedGrapher returns a grapher backed by a single dependency tracker so multiple
// test transactions share consistent pre-/chained-assembly edges.
func assemblingSharedGrapher(ctx context.Context) (grapher.Grapher, dependencytracker.DependencyTracker) {
	dt := newTestDependencyTracker(ctx)
	return grapher.NewGrapher(ctx, dt), dt
}

func Test_revertTransactionFailedAssembly_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Domain("test-domain").Build()

	revertReason := "test revert reason"
	mocks.SyncPoints.On("QueueTransactionFinalize",
		ctx,
		mock.MatchedBy(func(req *syncpoints.TransactionFinalizeRequest) bool {
			return req.FailureMessage == revertReason
		}),
		mock.Anything, // onCommit callback
		mock.Anything, // onRollback callback
	).Return()

	txn.revertTransactionFailedAssembly(ctx, revertReason)
}

func Test_applyPostAssembly_RevertResult(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Domain("test-domain").Build()

	revertReason := "test revert"
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   &revertReason,
	}
	requestID := uuid.New()

	mocks.SyncPoints.On("QueueTransactionFinalize",
		ctx,
		mock.MatchedBy(func(req *syncpoints.TransactionFinalizeRequest) bool {
			return req.FailureMessage == revertReason
		}),
		mock.Anything, // onCommit callback
		mock.Anything, // onRollback callback
	).Return()

	err := txn.applyPostAssembly(ctx, postAssembly, requestID)
	require.NoError(t, err)
	assert.Equal(t, postAssembly, txn.pt.PostAssembly)
}

func Test_action_AssembleRevertResponse_SetsPostAssemblyAndFinalizes(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Domain("test-domain").Build()
	revertReason := "assembler reverted"
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   &revertReason,
	}

	mocks.SyncPoints.On("QueueTransactionFinalize",
		ctx,
		mock.MatchedBy(func(req *syncpoints.TransactionFinalizeRequest) bool {
			return req.FailureMessage == revertReason
		}),
		mock.Anything, // onCommit callback
		mock.Anything, // onRollback callback
	).Return()

	event := &AssembleRevertResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		PostAssembly:         postAssembly,
		RequestID:            uuid.New(),
	}

	err := action_AssembleRevertResponse(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, postAssembly, txn.pt.PostAssembly)
}

func Test_applyPostAssembly_ParkResult(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_PARK,
	}

	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, postAssembly, txn.pt.PostAssembly)
}

func Test_applyPostAssembly_Success_WriteLockStatesError(t *testing.T) {
	ctx := context.Background()
	var capturedEvent common.Event
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		Domain("test-domain").
		QueueEventForCoordinator(func(ctx context.Context, event common.Event) {
			capturedEvent = event
		}).
		Build()

	mocks.SyncPoints.On("QueueTransactionFinalize",
		ctx,
		mock.Anything, mock.Anything, mock.Anything,
	).Return()

	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, txn.pt).Return(errors.New("write lock error"))

	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}
	requestID := uuid.New()

	err := txn.applyPostAssembly(ctx, postAssembly, requestID)

	require.ErrorContains(t, err, "write lock error")
	// Assert state: revert event was queued so state machine can transition
	require.NotNil(t, capturedEvent)
	revertEv, ok := capturedEvent.(*AssembleRevertResponseEvent)
	require.True(t, ok)
	assert.Equal(t, requestID, revertEv.RequestID)
	assert.Equal(t, txn.pt.ID, revertEv.TransactionID)
}

func Test_applyPostAssembly_Success_AddMinterError(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(uuid.New().String())
	mockGrapher := grapher.NewMockGrapher(t)
	mockGrapher.EXPECT().AddMinter(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("add minter error"))

	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Grapher(mockGrapher).Build()
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		OutputStates: []*components.FullState{
			{ID: stateID},
		},
	}

	// Mock engine integration to succeed
	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	assert.Error(t, err)
}

func Test_applyPostAssembly_Success_MapPotentialStatesError(t *testing.T) {
	ctx := context.Background()
	mockGrapher := grapher.NewMockGrapher(t)
	mockGrapher.EXPECT().AddMinter(mock.Anything, mock.Anything, mock.Anything).Return(nil)

	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		Build()

	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)
	mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, txn.pt).Return(nil, errors.New("map potential states error"))

	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		OutputStates:   []*components.FullState{},
	}

	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.ErrorContains(t, err, "map potential states error")
}

func Test_applyPostAssembly_Success_Complete(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	// Mock engine integration to succeed
	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)
	mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, txn.pt).Return(nil, nil)

	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		OutputStates:   []*components.FullState{},
	}

	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, postAssembly, txn.pt.PostAssembly)
}

func Test_sendAssembleRequest_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	// Mock engine integration
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)

	// Mock transport writer - use mock.Anything for idempotency key since it's generated dynamically
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)
	assert.NotNil(t, txn.pendingAssembleRequest)
	assert.NotNil(t, txn.cancelRequestTimeoutSchedule)
}

func Test_sendAssembleRequest_GetBlockHeightError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	// Mock engine integration
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0), errors.New("block height error"))

	err := txn.sendAssembleRequest(ctx)
	assert.Error(t, err)
}

func Test_sendAssembleRequest_ExportStatesAndLocksError(t *testing.T) {
	ctx := context.Background()
	mockGrapher := grapher.NewMockGrapher(t)
	mockGrapher.EXPECT().ExportStatesAndLocks(mock.Anything).Return(grapher.ExportableStates{}, errors.New("export states and locks failed"))

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		Build()

	err := txn.sendAssembleRequest(ctx)
	require.ErrorContains(t, err, "export states and locks failed")
}

func Test_sendAssembleRequest_SendAssembleRequestError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).UseMockTransportWriter().Build()

	// Mock engine integration
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)

	// Mock transport writer to return error - use mock.Anything for idempotency key
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(errors.New("send error"))

	err := txn.sendAssembleRequest(ctx)
	assert.Error(t, err)
}

func Test_nudgeAssembleRequest_NilPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	err := txn.nudgeAssembleRequest(ctx)
	assert.Error(t, err)
}

func Test_nudgeAssembleRequest_WithPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	// Create a pending request first
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	// Now nudge it - should succeed since request exists
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err = txn.nudgeAssembleRequest(ctx)
	assert.NoError(t, err)
}

func Test_writeLockStates_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, txn.pt).Return(nil)

	err := txn.writeStates(ctx)
	require.NoError(t, err)
}

func Test_writeLockStates_Error(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, txn.pt).Return(errors.New("write error"))

	err := txn.writeStates(ctx)
	require.Error(t, err)
}

func Test_validator_MatchesPendingAssembleRequest_AssembleSuccessEvent_Match(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	// Create a pending request
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	requestID := txn.pendingAssembleRequest.IdempotencyKey()
	event := &AssembleSuccessEvent{
		RequestID: requestID,
	}

	result, err := validator_MatchesPendingAssembleRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.True(t, result)
}

func Test_validator_MatchesPendingAssembleRequest_AssembleSuccessEvent_NoMatch(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	// Create a pending request
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	event := &AssembleSuccessEvent{
		RequestID: uuid.New(), // Different ID
	}

	result, err := validator_MatchesPendingAssembleRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, result)
}

func Test_validator_MatchesPendingAssembleRequest_AssembleSuccessEvent_NilPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	event := &AssembleSuccessEvent{
		RequestID: uuid.New(),
	}

	result, err := validator_MatchesPendingAssembleRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, result)
}

func Test_validator_MatchesPendingAssembleRequest_AssembleRevertResponseEvent_Match(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	// Create a pending request
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	requestID := txn.pendingAssembleRequest.IdempotencyKey()
	event := &AssembleRevertResponseEvent{
		RequestID: requestID,
	}

	result, err := validator_MatchesPendingAssembleRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.True(t, result)
}

func Test_validator_MatchesPendingAssembleRequest_AssembleErrorResponseEvent_Match(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	// Create a pending request
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	requestID := txn.pendingAssembleRequest.IdempotencyKey()
	event := &AssembleErrorResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            requestID,
	}

	result, err := validator_MatchesPendingAssembleRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.True(t, result)
}

func Test_validator_MatchesPendingAssembleRequest_AssembleErrorResponseEvent_NoMatch(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	// Create a pending request
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	event := &AssembleErrorResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(), // Different ID
	}

	result, err := validator_MatchesPendingAssembleRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, result)
}

func Test_validator_MatchesPendingAssembleRequest_AssembleErrorResponseEvent_NilPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	event := &AssembleErrorResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(),
	}

	result, err := validator_MatchesPendingAssembleRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, result)
}

func Test_validator_MatchesPendingAssembleRequest_OtherEventType(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	event := &SelectedEvent{}

	result, err := validator_MatchesPendingAssembleRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, result)
}

func Test_action_SendAssembleRequest_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := action_SendAssembleRequest(ctx, txn, nil)
	require.NoError(t, err)
	// Assert state: pending request and timer schedules were set
	assert.NotNil(t, txn.pendingAssembleRequest)
	assert.NotNil(t, txn.cancelRequestTimeoutSchedule)
}

func Test_action_NudgeAssembleRequest_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	// Create a pending request first
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	// Now nudge it
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, txn.pendingAssembleRequest.IdempotencyKey(), txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err = action_NudgeAssembleRequest(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_revertTransactionFailedAssembly_OnCommitCallback(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		Domain("test-domain").
		Build()
	revertReason := "test revert reason"

	onCommitCalled := false
	mocks.SyncPoints.On("QueueTransactionFinalize",
		ctx,
		mock.Anything, mock.Anything, mock.Anything,
	).Run(func(args mock.Arguments) {
		onCommit := args.Get(2).(func(context.Context))
		onCommit(ctx)
		onCommitCalled = true
	}).Return()

	txn.revertTransactionFailedAssembly(ctx, revertReason)

	assert.True(t, onCommitCalled)
}

func Test_revertTransactionFailedAssembly_OnRollbackRetry(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Domain("test-domain").Build()
	revertReason := "test revert reason"

	callCount := 0
	maxCalls := 2
	mocks.SyncPoints.On("QueueTransactionFinalize",
		ctx,
		mock.Anything, mock.Anything, mock.Anything,
	).Run(func(args mock.Arguments) {
		callCount++
		if callCount < maxCalls {
			onRollback := args.Get(3).(func(context.Context, error))
			onRollback(ctx, errors.New("rollback error"))
		} else {
			onCommit := args.Get(2).(func(context.Context))
			onCommit(ctx)
		}
	}).Return()

	txn.revertTransactionFailedAssembly(ctx, revertReason)

	assert.Equal(t, maxCalls, callCount)
}

func Test_sendAssembleRequest_schedulesTimer(t *testing.T) {
	ctx := context.Background()
	timeoutEventReceived := false
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		UseMockClock().
		QueueEventForCoordinator(func(ctx context.Context, event common.Event) {
			if _, ok := event.(*RequestTimeoutIntervalEvent); ok {
				timeoutEventReceived = true
			}
		}).
		RequestTimeout(1).
		Build()

	mocks.Clock.On("Now").Return(time.Now()).Once()
	mocks.Clock.On("ScheduleTimer", mock.Anything, time.Duration(1), mock.Anything).Return(func() {}).Run(func(args mock.Arguments) {
		callback := args.Get(2).(func())
		callback()
	})

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, mock.Anything, int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)
	assert.True(t, timeoutEventReceived)
}

func Test_guard_CanRetryErroredAssemble_WhenBelowThreshold(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		AssembleErrorCount(0).
		AssembleErrorRetryThreshold(3).
		Build()

	assert.True(t, guard_CanRetryErroredAssemble(ctx, txn))
}

func Test_guard_CanRetryErroredAssemble_WhenAtThreshold(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		AssembleErrorCount(4). // 4 errors, 3 retries allowed
		AssembleErrorRetryThreshold(3).
		Build()

	assert.False(t, guard_CanRetryErroredAssemble(ctx, txn))
}

func Test_guard_CanRetryErroredAssemble_WhenAboveThreshold(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		AssembleErrorCount(5).
		AssembleErrorRetryThreshold(3).
		Build()

	assert.False(t, guard_CanRetryErroredAssemble(ctx, txn))
}

func Test_action_AssembleError_IncrementsCountAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	event := &AssembleErrorResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(),
	}

	err := action_AssembleError(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, 1, txn.assembleErrorCount)
}

func Test_action_AssembleError_MultipleCallsIncrementCount(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	event := &AssembleErrorResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(),
	}

	for i := 1; i <= 3; i++ {
		err := action_AssembleError(ctx, txn, event)
		require.NoError(t, err)
		assert.Equal(t, i, txn.assembleErrorCount)
	}
}

func Test_notifyDependentsOfSelection_NoDependents(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	err := txn.notifyDependentsOfSelection(ctx)
	require.NoError(t, err)
}

func Test_notifyDependentsOfSelection_PreAssembleDependentNotFound(t *testing.T) {
	ctx := context.Background()
	dependentID := uuid.New()
	depTracker := dependencytracker.NewDependencyTracker()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).DependencyTracker(depTracker).Build()
	depTracker.GetPreassemblyDeps().AddPrerequisites(dependentID, txn.pt.ID)

	err := txn.notifyDependentsOfSelection(ctx)
	require.Error(t, err)
}

func Test_notifyDependentsOfSelection_PreAssembleDependent(t *testing.T) {
	ctx := context.Background()
	g, dt := assemblingSharedGrapher(ctx)

	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(g).DependencyTracker(dt).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(g).DependencyTracker(dt).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			dependentTxn.GetPrivateTransaction().ID: dependentTxn,
		}).
		Build()

	dt.GetPreassemblyDeps().AddPrerequisites(dependentTxn.pt.ID, txn.pt.ID)

	err := txn.notifyDependentsOfSelection(ctx)
	require.NoError(t, err)
}

func Test_notifyDependentsOfSelection_ChainedDependent(t *testing.T) {
	ctx := context.Background()
	g, dt := assemblingSharedGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(g).DependencyTracker(dt).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(g).DependencyTracker(dt).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			depTx.GetPrivateTransaction().ID: depTx,
		}).
		Build()

	dt.GetChainedDeps().AddPrerequisites(depTx.pt.ID, txn.pt.ID)

	err := txn.notifyDependentsOfSelection(ctx)
	require.NoError(t, err)
}

func Test_AssembleSuccess_TransitionsToBlocked_WhenAttestationFulfilledButDepsNotReady(t *testing.T) {
	ctx := context.Background()
	g, _ := assemblingSharedGrapher(ctx)

	dependency, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(g).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(g).
		AddPendingAssembleRequest().
		NumberOfRequiredEndorsers(0).
		InputStateIDs(dependency.pt.PostAssembly.OutputStates[0].ID)

	txn, mocks := txnBuilder.Build()
	mocks.EngineIntegration.EXPECT().MapPotentialStates(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mocks.EngineIntegration.EXPECT().WriteStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, txnBuilder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.Equal(t, State_Blocked, txn.GetCurrentState())
}

func Test_Assembling_DependencyReset_TransitionsToPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	g, dt := assemblingSharedGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(g).DependencyTracker(dt).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(g).DependencyTracker(dt).
		Build()
	dt.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
	_, marked := txn.dependencyTracker.GetChainedDeps().GetUnassembledDependencies(txn.pt.ID)[depTx.pt.ID]
	assert.True(t, marked)
}

func Test_Assembling_DependencyConfirmedReverted_TransitionsToPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	g, dt := assemblingSharedGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(g).DependencyTracker(dt).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(g).DependencyTracker(dt).
		Build()
	dt.GetChainedDeps().AddPrerequisites(txn.pt.ID, depTx.pt.ID)

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
	_, marked := txn.dependencyTracker.GetChainedDeps().GetUnassembledDependencies(txn.pt.ID)[depTx.pt.ID]
	assert.True(t, marked)
}

func Test_Assembling_ChainedDependencyFailed_TransitionsToReverted(t *testing.T) {
	ctx := context.Background()
	grapher := newTestGrapher(ctx)

	depID := uuid.New()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()

	mocks.SyncPoints.On("QueueTransactionFinalize",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()
	// mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &ChainedDependencyFailedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		FailedTxID:           depID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Reverted, txn.GetCurrentState())
}

func Test_Assembling_ChainedDependencyEvicted_TransitionsToEvicted(t *testing.T) {
	ctx := context.Background()
	grapher := newTestGrapher(ctx)

	depID := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()

	err := txn.HandleEvent(ctx, &ChainedDependencyEvictedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		EvictedTxID:          depID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Evicted, txn.GetCurrentState())
}

func Test_notifyDependentsOfSelection_ChainedDependentNotFound(t *testing.T) {
	ctx := context.Background()
	g, dt := assemblingSharedGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(g).DependencyTracker(dt).
		Build()
	missingDependentID := uuid.New()
	dt.GetChainedDeps().AddPrerequisites(missingDependentID, txn.pt.ID)

	err := txn.notifyDependentsOfSelection(ctx)
	require.Error(t, err)
}

func Test_action_NotifyPreAssembleDependentOfSelection_Success(t *testing.T) {
	ctx := context.Background()
	g, dt := assemblingSharedGrapher(ctx)

	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(g).DependencyTracker(dt).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(g).DependencyTracker(dt).
		CoordinatorTransactions(map[uuid.UUID]CoordinatorTransaction{
			dependentTxn.GetPrivateTransaction().ID: dependentTxn,
		}).
		Build()

	dt.GetPreassemblyDeps().AddPrerequisites(dependentTxn.pt.ID, txn.pt.ID)

	err := action_NotifyDependentsOfSelection(ctx, txn, nil)
	require.NoError(t, err)
}
