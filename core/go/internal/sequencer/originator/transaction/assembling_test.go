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

package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAction_AssembleAndSign_NoAssembleRequest(t *testing.T) {
	// Test that action_AssembleAndSign returns error when latestAssembleRequest is nil
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()

	// Explicitly set latestAssembleRequest to nil
	txn.latestAssembleRequest = nil

	// Execute the action
	err := action_AssembleAndSign(ctx, txn)

	// Verify error is returned
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "No assemble request found")
}

func TestAction_AssembleAndSign_EngineIntegrationError(t *testing.T) {
	// Test that action_AssembleAndSign returns error when AssembleAndSign fails
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set (should be set by builder for State_Assembling)
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")

	// Set up PreAssembly
	if txn.PreAssembly == nil {
		txn.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Mock AssembleAndSign to return an error
	expectedError := errors.New("assembly failed")
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.ID,
		txn.PreAssembly,
		mock.Anything,
		mock.Anything,
	).Return(nil, expectedError)

	// Execute the action
	err := action_AssembleAndSign(ctx, txn)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Verify no events were emitted
	events := mocks.GetEmittedEvents()
	assert.Empty(t, events, "No events should be emitted when AssembleAndSign fails")
}

func TestAction_AssembleAndSign_Success_OK(t *testing.T) {
	// Test that action_AssembleAndSign emits AssembleAndSignSuccessEvent when AssembleAndSign returns OK
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")
	requestID := txn.latestAssembleRequest.requestID

	// Set up PreAssembly
	if txn.PreAssembly == nil {
		txn.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Create expected post assembly with OK result
	expectedPostAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	// Mock AssembleAndSign to return OK
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.ID,
		txn.PreAssembly,
		mock.Anything,
		mock.Anything,
	).Return(expectedPostAssembly, nil)

	// Execute the action
	err := action_AssembleAndSign(ctx, txn)

	// Verify no error
	assert.NoError(t, err)

	// Verify AssembleAndSignSuccessEvent was emitted
	events := mocks.GetEmittedEvents()
	require.Len(t, events, 1, "Should emit exactly one event")

	successEvent, ok := events[0].(*AssembleAndSignSuccessEvent)
	require.True(t, ok, "Event should be AssembleAndSignSuccessEvent")
	assert.Equal(t, txn.ID, successEvent.TransactionID)
	assert.Equal(t, requestID, successEvent.RequestID)
	assert.Equal(t, expectedPostAssembly, successEvent.PostAssembly)
}

func TestAction_AssembleAndSign_Success_REVERT(t *testing.T) {
	// Test that action_AssembleAndSign emits AssembleRevertEvent when AssembleAndSign returns REVERT
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")
	requestID := txn.latestAssembleRequest.requestID

	// Set up PreAssembly
	if txn.PreAssembly == nil {
		txn.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Create expected post assembly with REVERT result
	revertReason := "transaction reverted"
	expectedPostAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   &revertReason,
	}

	// Mock AssembleAndSign to return REVERT
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.ID,
		txn.PreAssembly,
		mock.Anything,
		mock.Anything,
	).Return(expectedPostAssembly, nil)

	// Execute the action
	err := action_AssembleAndSign(ctx, txn)

	// Verify no error
	assert.NoError(t, err)

	// Verify AssembleRevertEvent was emitted
	events := mocks.GetEmittedEvents()
	require.Len(t, events, 1, "Should emit exactly one event")

	revertEvent, ok := events[0].(*AssembleRevertEvent)
	require.True(t, ok, "Event should be AssembleRevertEvent")
	assert.Equal(t, txn.ID, revertEvent.TransactionID)
	assert.Equal(t, requestID, revertEvent.RequestID)
	assert.Equal(t, expectedPostAssembly, revertEvent.PostAssembly)
}

func TestAction_AssembleAndSign_Success_PARK(t *testing.T) {
	// Test that action_AssembleAndSign emits AssembleParkEvent when AssembleAndSign returns PARK
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")
	requestID := txn.latestAssembleRequest.requestID

	// Set up PreAssembly
	if txn.PreAssembly == nil {
		txn.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Create expected post assembly with PARK result
	expectedPostAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_PARK,
	}

	// Mock AssembleAndSign to return PARK
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.ID,
		txn.PreAssembly,
		mock.Anything,
		mock.Anything,
	).Return(expectedPostAssembly, nil)

	// Execute the action
	err := action_AssembleAndSign(ctx, txn)

	// Verify no error
	assert.NoError(t, err)

	// Verify AssembleParkEvent was emitted
	events := mocks.GetEmittedEvents()
	require.Len(t, events, 1, "Should emit exactly one event")

	parkEvent, ok := events[0].(*AssembleParkEvent)
	require.True(t, ok, "Event should be AssembleParkEvent")
	assert.Equal(t, txn.ID, parkEvent.TransactionID)
	assert.Equal(t, requestID, parkEvent.RequestID)
	assert.Equal(t, expectedPostAssembly, parkEvent.PostAssembly)
}

func TestAction_AssembleAndSign_EventHandlerError_OK(t *testing.T) {
	// Test that action_AssembleAndSign returns error when eventHandler fails for OK result
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")

	// Set up PreAssembly
	if txn.PreAssembly == nil {
		txn.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Create expected post assembly with OK result
	expectedPostAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	// Mock AssembleAndSign to return OK
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.ID,
		txn.PreAssembly,
		mock.Anything,
		mock.Anything,
	).Return(expectedPostAssembly, nil)

	// Set up eventHandler to return an error
	expectedError := errors.New("event handler error")
	txn.eventHandler = func(ctx context.Context, event common.Event) error {
		return expectedError
	}

	// Execute the action
	err := action_AssembleAndSign(ctx, txn)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestAction_AssembleAndSign_EventHandlerError_REVERT(t *testing.T) {
	// Test that action_AssembleAndSign returns error when eventHandler fails for REVERT result
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")

	// Set up PreAssembly
	if txn.PreAssembly == nil {
		txn.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Create expected post assembly with REVERT result
	revertReason := "transaction reverted"
	expectedPostAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   &revertReason,
	}

	// Mock AssembleAndSign to return REVERT
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.ID,
		txn.PreAssembly,
		mock.Anything,
		mock.Anything,
	).Return(expectedPostAssembly, nil)

	// Set up eventHandler to return an error
	expectedError := errors.New("event handler error")
	txn.eventHandler = func(ctx context.Context, event common.Event) error {
		return expectedError
	}

	// Execute the action
	err := action_AssembleAndSign(ctx, txn)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestAction_AssembleAndSign_EventHandlerError_PARK(t *testing.T) {
	// Test that action_AssembleAndSign returns error when eventHandler fails for PARK result
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")

	// Set up PreAssembly
	if txn.PreAssembly == nil {
		txn.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Create expected post assembly with PARK result
	expectedPostAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_PARK,
	}

	// Mock AssembleAndSign to return PARK
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		txn.ID,
		txn.PreAssembly,
		mock.Anything,
		mock.Anything,
	).Return(expectedPostAssembly, nil)

	// Set up eventHandler to return an error
	expectedError := errors.New("event handler error")
	txn.eventHandler = func(ctx context.Context, event common.Event) error {
		return expectedError
	}

	// Execute the action
	err := action_AssembleAndSign(ctx, txn)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestAction_AssembleAndSign_AssembleAndSignCalledWithCorrectParameters(t *testing.T) {
	// Test that AssembleAndSign is called with correct parameters
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Ensure latestAssembleRequest is set
	require.NotNil(t, txn.latestAssembleRequest, "latestAssembleRequest should be set for State_Assembling")
	requestID := txn.latestAssembleRequest.requestID
	stateLocksJSON := txn.latestAssembleRequest.stateLocksJSON
	coordinatorsBlockHeight := txn.latestAssembleRequest.coordinatorsBlockHeight

	// Set up PreAssembly
	preAssembly := &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txn.ID.String(),
		},
	}
	txn.PreAssembly = preAssembly

	// Create expected post assembly
	expectedPostAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	// Mock AssembleAndSign with specific parameter expectations
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		ctx,
		txn.ID,
		preAssembly,
		stateLocksJSON,
		coordinatorsBlockHeight,
	).Return(expectedPostAssembly, nil)

	// Execute the action
	err := action_AssembleAndSign(ctx, txn)

	// Verify no error
	assert.NoError(t, err)

	// Verify AssembleAndSign was called with correct parameters
	mocks.EngineIntegration.AssertExpectations(t)

	// Verify event was emitted with correct request ID
	events := mocks.GetEmittedEvents()
	require.Len(t, events, 1, "Should emit exactly one event")
	successEvent, ok := events[0].(*AssembleAndSignSuccessEvent)
	require.True(t, ok, "Event should be AssembleAndSignSuccessEvent")
	assert.Equal(t, requestID, successEvent.RequestID)
}
