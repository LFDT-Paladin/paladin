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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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

	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, txn.pt).Return(errors.New("write lock error"))

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
	// Mock grapher to return error when adding minter
	mockGrapher := NewMockGrapher(t)
	mockGrapher.EXPECT().Add(mock.Anything, mock.Anything).Return()
	mockGrapher.EXPECT().AddMinter(mock.Anything, stateID, mock.Anything).Return(errors.New("add minter error"))

	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Grapher(mockGrapher).Build()
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		OutputStates: []*components.FullState{
			{ID: stateID},
		},
	}

	// Mock engine integration to succeed
	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	assert.Error(t, err)
}

func Test_applyPostAssembly_Success_CalculateDependenciesError(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(uuid.New().String())
	mockGrapher := NewMockGrapher(t)
	mockGrapher.EXPECT().Add(mock.Anything, mock.Anything).Return()
	// calculatePostAssembleDependencies looks up minters for InputStates; return error to trigger failure path
	mockGrapher.EXPECT().LookupMinter(ctx, stateID).Return(nil, errors.New("lookup minter error"))

	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		Build()

	// Mock engine integration to succeed
	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	// The function will try to look up minters for InputStates and ReadStates
	// Since we have empty arrays, it won't call LookupMinter, so we need to add a state
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		OutputStates:   []*components.FullState{},
		InputStates:    []*components.FullState{{ID: stateID}},
	}

	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	// Fails in calculatePostAssembleDependencies when LookupMinter returns error
	require.ErrorContains(t, err, "lookup minter error")
}

func Test_applyPostAssembly_Success_Complete(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	// Mock engine integration to succeed
	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

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
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)

	// Mock transport writer - use mock.Anything for idempotency key since it's generated dynamically
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)
	assert.NotNil(t, txn.pendingAssembleRequest)
	assert.NotNil(t, txn.cancelRequestTimeoutSchedule)
}

func Test_sendAssembleRequest_GetStateLocksError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	// Mock engine integration to return error
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return(nil, errors.New("state locks error"))

	err := txn.sendAssembleRequest(ctx)
	require.Error(t, err)
}

func Test_sendAssembleRequest_GetBlockHeightError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	// Mock engine integration
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(0), errors.New("block height error"))

	err := txn.sendAssembleRequest(ctx)
	assert.Error(t, err)
}

func Test_sendAssembleRequest_SendAssembleRequestError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).UseMockTransportWriter().Build()

	// Mock engine integration
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)

	// Mock transport writer to return error - use mock.Anything for idempotency key
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
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
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	// Now nudge it - should succeed since request exists
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err = txn.nudgeAssembleRequest(ctx)
	assert.NoError(t, err)
}

func Test_calculatePostAssembleDependencies_NilPostAssembly(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	txn.pt.PostAssembly = nil

	err := txn.calculatePostAssembleDependencies(ctx)
	require.Error(t, err)
}

func Test_calculatePostAssembleDependencies_NoInputOrReadStates(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		PostAssembly(&components.TransactionPostAssembly{
			InputStates: []*components.FullState{},
			ReadStates:  []*components.FullState{},
		}).
		Build()

	err := txn.calculatePostAssembleDependencies(ctx)
	require.NoError(t, err)
}

func Test_calculatePostAssembleDependencies_StateWithNoMinter(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(uuid.New().String())
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		PostAssembly(&components.TransactionPostAssembly{
			InputStates: []*components.FullState{{ID: stateID}},
			ReadStates:  []*components.FullState{},
		}).
		Build()

	err := txn.calculatePostAssembleDependencies(ctx)
	require.NoError(t, err)
}

func Test_calculatePostAssembleDependencies_StateWithMinter(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)

	// Create a minter transaction
	minterTxn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build()
	stateID := minterTxn.pt.PostAssembly.OutputStates[0].ID

	// Create dependent transaction
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		PostAssembly(&components.TransactionPostAssembly{
			InputStates: []*components.FullState{{ID: stateID}},
			ReadStates:  []*components.FullState{},
		}).
		Build()

	err := txn.calculatePostAssembleDependencies(ctx)
	require.NoError(t, err)
	assert.Contains(t, txn.dependencies.PostAssemble.DependsOn, minterTxn.pt.ID)
	assert.Contains(t, minterTxn.dependencies.PostAssemble.PrereqOf, txn.pt.ID)
}

func Test_calculatePostAssembleDependencies_LookupMinterError(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(uuid.New().String())

	// Use a mock grapher that returns an error
	mockGrapher := NewMockGrapher(t)
	mockGrapher.EXPECT().Add(mock.Anything, mock.Anything).Return()
	mockGrapher.EXPECT().LookupMinter(ctx, stateID).Return(nil, errors.New("lookup error"))

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Grapher(mockGrapher).
		PostAssembly(&components.TransactionPostAssembly{
			InputStates: []*components.FullState{{ID: stateID}},
			ReadStates:  []*components.FullState{},
		}).
		Build()

	err := txn.calculatePostAssembleDependencies(ctx)
	require.Error(t, err)
}

func Test_calculatePostAssembleDependencies_DuplicateDependency(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)
	// Create a minter transaction
	minterTxn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfOutputStates(2).
		Grapher(grapher).
		Build()
	stateID1 := minterTxn.pt.PostAssembly.OutputStates[0].ID
	stateID2 := minterTxn.pt.PostAssembly.OutputStates[1].ID

	// Create dependent transaction with both states
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		PostAssembly(&components.TransactionPostAssembly{
			InputStates: []*components.FullState{{ID: stateID1}, {ID: stateID2}},
			ReadStates:  []*components.FullState{},
		}).
		Build()

	err := txn.calculatePostAssembleDependencies(ctx)
	require.NoError(t, err)
	// Should only have one dependency entry
	require.Len(t, txn.dependencies.PostAssemble.DependsOn, 1)
	assert.Equal(t, minterTxn.pt.ID, txn.dependencies.PostAssemble.DependsOn[0])
}

func Test_writeLockStates_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, txn.pt).Return(nil)

	err := txn.writeLockStates(ctx)
	require.NoError(t, err)
}

func Test_writeLockStates_Error(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, txn.pt).Return(errors.New("write error"))

	err := txn.writeLockStates(ctx)
	require.Error(t, err)
}

func Test_validator_MatchesPendingAssembleRequest_AssembleSuccessEvent_Match(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	// Create a pending request
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
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
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
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
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
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
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
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
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
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

	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
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
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	// Now nudge it
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, txn.pendingAssembleRequest.IdempotencyKey(), txn.pt.PreAssembly, []byte("{}"), int64(100),
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

	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
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

	mockGrapher := NewMockGrapher(t)
	mockGrapher.EXPECT().Add(mock.Anything, mock.Anything).Return()
	mockGrapher.EXPECT().TransactionByID(ctx, dependentID).Return(nil)

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Grapher(mockGrapher).Build()
	txn.dependencies.PreAssemble.PrereqOf = &dependentID

	err := txn.notifyDependentsOfSelection(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), dependentID.String())
}

func Test_notifyDependentsOfSelection_PreAssembleDependent(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()
	txn.dependencies.PreAssemble.PrereqOf = &dependentTxn.pt.ID

	err := txn.notifyDependentsOfSelection(ctx)
	require.NoError(t, err)
}

func Test_notifyDependentsOfSelection_ChainedDependent(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	dependentTxn, depMocks := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).
		Build()
	depMocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, dependentTxn.pt.ID).Return().Maybe()

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.PrereqOf = []uuid.UUID{dependentTxn.pt.ID}

	err := txn.notifyDependentsOfSelection(ctx)
	require.NoError(t, err)
}

func Test_AssembleSuccess_TransitionsToBlocked_WhenAttestationFulfilledButDepsNotReady(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	dependency, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		AddPendingAssembleRequest().
		NumberOfRequiredEndorsers(0).
		InputStateIDs(dependency.pt.PostAssembly.OutputStates[0].ID)

	txn, mocks := txnBuilder.Build()

	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, txnBuilder.BuildAssembleSuccessEvent())
	require.NoError(t, err)
	assert.Equal(t, State_Blocked, txn.GetCurrentState())
}

func Test_Assembling_DependencyReset_TransitionsToPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
	assert.Contains(t, txn.dependencies.Chained.Unassembled, depTx.pt.ID)
}

func Test_Assembling_DependencyConfirmedReverted_TransitionsToPreAssemblyBlocked(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	txn.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{}

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		SourceTransactionID:  depTx.pt.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState())
	assert.Contains(t, txn.dependencies.Chained.Unassembled, depTx.pt.ID)
}

func Test_Assembling_ChainedDependencyFailed_TransitionsToReverted(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depID := uuid.New()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()

	mocks.SyncPoints.On("QueueTransactionFinalize",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &ChainedDependencyFailedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		FailedTxID:           depID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Reverted, txn.GetCurrentState())
}

func Test_Assembling_ChainedDependencyEvicted_TransitionsToEvicted(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

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
	grapher := NewGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.PrereqOf = []uuid.UUID{uuid.New()}

	err := txn.notifyDependentsOfSelection(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012645")
}
