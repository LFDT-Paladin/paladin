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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
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
		txn.pt.Domain,
		pldtypes.EthAddress{},
		txn.originator,
		txn.pt.ID,
		revertReason,
		mock.Anything, // onCommit callback
		mock.Anything, // onRollback callback
	).Return()

	// Call the function - it should queue a finalize
	// Note: This function uses a recursive retry mechanism in onRollback,
	// but we're just testing the initial call path for coverage
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
		txn.pt.Domain,
		pldtypes.EthAddress{},
		txn.originator,
		txn.pt.ID,
		revertReason,
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
		txn.pt.Domain,
		pldtypes.EthAddress{},
		txn.originator,
		txn.pt.ID,
		revertReason,
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
		txn.pt.Domain,
		pldtypes.EthAddress{},
		txn.originator,
		txn.pt.ID,
		mock.Anything,
		mock.Anything, // onCommit callback
		mock.Anything, // onRollback callback
	).Return()

	// Mock engine integration to return error so we hit the writeLockStates error path
	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.pt).Return(errors.New("write lock error"))

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
	mockGrapher.EXPECT().Add(ctx, mock.Anything).Return()
	mockGrapher.EXPECT().AddMinter(ctx, stateID, mock.Anything).Return(errors.New("add minter error"))

	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Grapher(mockGrapher).Build()
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		OutputStates: []*components.FullState{
			{ID: stateID},
		},
	}

	// Mock engine integration to succeed
	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, mock.Anything).Return(nil)

	err := txn.applyPostAssembly(ctx, postAssembly, uuid.New())
	assert.Error(t, err)
}

func Test_applyPostAssembly_Success_CalculateDependenciesError(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(uuid.New().String())
	mockGrapher := NewMockGrapher(t)
	mockGrapher.EXPECT().Add(ctx, mock.Anything).Return()
	// calculatePostAssembleDependencies looks up minters for InputStates; return error to trigger failure path
	mockGrapher.EXPECT().LookupMinter(ctx, stateID).Return(nil, errors.New("lookup minter error"))

	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(mockGrapher).
		Build()

	// Mock engine integration to succeed
	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, mock.Anything).Return(nil)

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
	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, mock.Anything).Return(nil)

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
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)

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
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return(nil, errors.New("state locks error"))

	err := txn.sendAssembleRequest(ctx)
	require.Error(t, err)
}

func Test_sendAssembleRequest_GetBlockHeightError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	// Mock engine integration
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(0), errors.New("block height error"))

	err := txn.sendAssembleRequest(ctx)
	assert.Error(t, err)
}

func Test_sendAssembleRequest_SendAssembleRequestError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).UseMockTransportWriter().Build()

	// Mock engine integration
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)

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
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	// Now nudge it - should succeed since request exists
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err = txn.nudgeAssembleRequest(ctx)
	assert.NoError(t, err)
}

func Test_isNotAssembled_NotAssembledStates(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	states := []State{
		State_Assembling,
		State_Pooled,
		State_Assembling,
		State_Reverted,
	}

	for _, state := range states {
		txn.stateMachine.CurrentState = state
		assert.True(t, txn.isNotAssembled(), "state %s should be not assembled", state)
	}
}

func Test_isNotAssembled_AssembledStates(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	states := []State{
		State_Endorsement_Gathering,
		State_Confirming_Dispatchable,
		State_Ready_For_Dispatch,
		State_Dispatched,
		State_Confirmed,
	}

	for _, state := range states {
		txn.stateMachine.CurrentState = state
		assert.False(t, txn.isNotAssembled(), "state %s should be assembled", state)
	}
}

func Test_notifyDependentsOfAssembled_NoDependents(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	err := txn.notifyDependentsOfAssembled(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfAssembled_WithDependent_HandleEventError(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)
	txn2ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{txn2ID}}).
		Build()
	txn2, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).
		TransactionID(txn2ID).
		Dependencies(&pldapi.TransactionDependencies{}).
		Build()

	// Mock HandleEvent by setting up txn2 to fail when processing DependencyAssembledEvent
	// When DependencyAssembledEvent is processed in State_PreAssembly_Blocked, it transitions to State_Pooled
	// which calls action_initializeDependencies. We can make that fail by removing PreAssembly.
	txn2.pt.PreAssembly = nil // This will cause action_initializeDependencies to fail

	// Call notifyDependentsOfAssembled - it should return the error from HandleEvent
	err := txn1.notifyDependentsOfAssembled(ctx)
	require.Error(t, err)
}

func Test_notifyDependentsOfAssembled_WithDependents(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)
	txn2ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{txn2ID}}).
		Build()
	_, _ = NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		TransactionID(txn2ID).
		Dependencies(&pldapi.TransactionDependencies{}).
		Build() // txn2 registered in grapher so txn1.notifyDependentsOfAssembled can find it

	err := txn1.notifyDependentsOfAssembled(ctx)
	require.NoError(t, err)
}

func Test_notifyDependentsOfAssembled_DependentNotFound(t *testing.T) {
	ctx := context.Background()
	missingID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{missingID}}).
		Build()

	err := txn1.notifyDependentsOfAssembled(ctx)
	require.Error(t, err)
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
		Dependencies(&pldapi.TransactionDependencies{}).
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
		Dependencies(&pldapi.TransactionDependencies{}).
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
		Dependencies(&pldapi.TransactionDependencies{}).
		Build()
	stateID := minterTxn.pt.PostAssembly.OutputStates[0].ID

	// Create dependent transaction
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		PostAssembly(&components.TransactionPostAssembly{
			InputStates: []*components.FullState{{ID: stateID}},
			ReadStates:  []*components.FullState{},
		}).
		Dependencies(&pldapi.TransactionDependencies{}).
		Build()

	err := txn.calculatePostAssembleDependencies(ctx)
	require.NoError(t, err)
	assert.Contains(t, txn.dependencies.DependsOn, minterTxn.pt.ID)
	assert.Contains(t, minterTxn.dependencies.PrereqOf, txn.pt.ID)
}

func Test_calculatePostAssembleDependencies_LookupMinterError(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(uuid.New().String())

	// Use a mock grapher that returns an error
	mockGrapher := NewMockGrapher(t)
	mockGrapher.EXPECT().Add(ctx, mock.Anything).Return()
	mockGrapher.EXPECT().LookupMinter(ctx, stateID).Return(nil, errors.New("lookup error"))

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Grapher(mockGrapher).
		PostAssembly(&components.TransactionPostAssembly{
			InputStates: []*components.FullState{{ID: stateID}},
			ReadStates:  []*components.FullState{},
		}).
		Dependencies(&pldapi.TransactionDependencies{}).
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
		Dependencies(&pldapi.TransactionDependencies{}).
		Build()

	err := txn.calculatePostAssembleDependencies(ctx)
	require.NoError(t, err)
	// Should only have one dependency entry
	require.Len(t, txn.dependencies.DependsOn, 1)
	assert.Equal(t, minterTxn.pt.ID, txn.dependencies.DependsOn[0])
}

func Test_writeLockStates_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.pt).Return(nil)

	err := txn.writeLockStates(ctx)
	require.NoError(t, err)
}

func Test_writeLockStates_Error(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, txn.pt).Return(errors.New("write error"))

	err := txn.writeLockStates(ctx)
	require.Error(t, err)
}

func Test_incrementErrors_IncrementsErrorCount(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	err := txn.incrementErrors()
	require.NoError(t, err)
	assert.Equal(t, 1, txn.errorCount)
}

func Test_validator_MatchesPendingAssembleRequest_AssembleSuccessEvent_Match(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		Build()

	// Create a pending request
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
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
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
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
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
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

	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
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
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	// Now nudge it
	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, txn.pendingAssembleRequest.IdempotencyKey(), txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err = action_NudgeAssembleRequest(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NotifyDependentsOfAssembled_Success(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	err := action_NotifyDependentsOfAssembled(ctx, txn, nil)
	require.NoError(t, err)
	// State: no dependents, so no HandleEvent calls; dependencies unchanged
	assert.Len(t, txn.dependencies.PrereqOf, 0)
}

func Test_action_IncrementAssembleErrors_Success(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()

	err := action_IncrementErrors(ctx, txn, nil)
	assert.NoError(t, err)
	assert.Equal(t, 1, txn.errorCount)
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
		txn.pt.Domain,
		pldtypes.EthAddress{},
		txn.originator,
		txn.pt.ID,
		revertReason,
		mock.Anything,
		mock.Anything,
	).Run(func(args mock.Arguments) {
		onCommit := args[6].(func(context.Context))
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
		txn.pt.Domain,
		pldtypes.EthAddress{},
		txn.originator,
		txn.pt.ID,
		revertReason,
		mock.Anything,
		mock.Anything,
	).Run(func(args mock.Arguments) {
		callCount++
		if callCount < maxCalls {
			onRollback := args[7].(func(context.Context, error))
			onRollback(ctx, errors.New("rollback error"))
		} else {
			onCommit := args[6].(func(context.Context))
			onCommit(ctx)
		}
	}).Return()

	txn.revertTransactionFailedAssembly(ctx, revertReason)

	assert.Equal(t, maxCalls, callCount)
}

func Test_sendAssembleRequest_RequestTimeoutCallback(t *testing.T) {
	ctx := context.Background()
	timeoutEventReceived := false
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		QueueEventForCoordinator(func(ctx context.Context, event common.Event) {
			if _, ok := event.(*RequestTimeoutIntervalEvent); ok {
				timeoutEventReceived = true
			}
		}).
		RequestTimeout(1).
		Build()

	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err := txn.sendAssembleRequest(ctx)
	require.NoError(t, err)

	mocks.Clock.Advance(1)
	assert.True(t, timeoutEventReceived)
}

func Test_onTransitionToAssembling_StateTimeoutCallback(t *testing.T) {
	ctx := context.Background()
	timeoutEventReceived := false
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		UseMockTransportWriter().
		QueueEventForCoordinator(func(ctx context.Context, event common.Event) {
			if _, ok := event.(*StateTimeoutIntervalEvent); ok {
				timeoutEventReceived = true
			}
		}).
		StateTimeout(1).
		Build()

	mocks.EngineIntegration.EXPECT().GetStateLocks(ctx).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(ctx).Return(int64(100), nil)
	mocks.TransportWriter.EXPECT().SendAssembleRequest(
		ctx, txn.originatorNode, txn.pt.ID, mock.Anything, txn.pt.PreAssembly, []byte("{}"), int64(100),
	).Return(nil)

	err := action_ScheduleStateTimeout(ctx, txn, nil)
	require.NoError(t, err)
	err = action_SendAssembleRequest(ctx, txn, nil)
	require.NoError(t, err)

	// Wait for timeout to fire
	mocks.Clock.Advance(1)

	assert.True(t, timeoutEventReceived)
}
