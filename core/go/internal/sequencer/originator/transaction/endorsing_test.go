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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAction_ResendAssembleSuccessResponse_Success(t *testing.T) {
	// Test that action_ResendAssembleSuccessResponse calls action_SendAssembleSuccessResponse with correct parameters
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	// Set up PostAssembly with OK result
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		Signatures: []*prototk.AttestationResult{
			{
				Payload: []byte("test signature"),
			},
		},
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txn.GetID().String(),
		},
	}

	// Reset the recorder to ensure clean state
	mocks.SentMessageRecorder.Reset(ctx)

	// Execute the action
	err := action_ResendAssembleSuccessResponse(ctx, txn, nil)

	// Verify no error
	assert.NoError(t, err)

	// Verify that SendAssembleResponse was called with success result
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "SendAssembleResponse should have been called with OK result")
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "Should not have sent revert response")
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "Should not have sent park response")
}

func TestAction_ResendAssembleSuccessResponse_TransportError(t *testing.T) {
	// Test that action_ResendAssembleSuccessResponse returns error when transport fails
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	// Set up PostAssembly with OK result
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &components.TransactionPreAssembly{}

	// Create a mock transport writer that returns an error
	mockTransport := transport.NewMockTransportWriter(t)
	expectedError := errors.New("transport error")
	mockTransport.EXPECT().SendAssembleResponse(
		mock.Anything,
		txn.GetID(),
		requestID,
		txn.pt.PostAssembly,
		txn.pt.PreAssembly,
		coordinator,
	).Return(expectedError)

	// Replace transport writer with mock
	originalTransport := txn.transportWriter
	txn.transportWriter = mockTransport

	// Execute the action
	err := action_ResendAssembleSuccessResponse(ctx, txn, nil)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Restore original transport
	txn.transportWriter = originalTransport
}

func TestAction_ResendAssembleRevertResponse_Success(t *testing.T) {
	// Test that action_ResendAssembleRevertResponse calls action_SendAssembleRevertResponse with correct parameters
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Reverted)
	txn, mocks := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	// Set up PostAssembly with REVERT result
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   ptrTo("test revert reason"),
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txn.GetID().String(),
		},
	}

	// Reset the recorder to ensure clean state
	mocks.SentMessageRecorder.Reset(ctx)

	// Execute the action
	err := action_ResendAssembleRevertResponse(ctx, txn, nil)

	// Verify no error
	assert.NoError(t, err)

	// Verify that SendAssembleResponse was called with revert result
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "SendAssembleResponse should have been called with REVERT result")
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "Should not have sent success response")
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "Should not have sent park response")
}

func TestAction_ResendAssembleRevertResponse_TransportError(t *testing.T) {
	// Test that action_ResendAssembleRevertResponse returns error when transport fails
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Reverted)
	txn, _ := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	// Set up PostAssembly with REVERT result
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   ptrTo("test revert reason"),
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &components.TransactionPreAssembly{}

	// Create a mock transport writer that returns an error
	mockTransport := transport.NewMockTransportWriter(t)
	expectedError := errors.New("transport error")
	mockTransport.EXPECT().SendAssembleResponse(
		mock.Anything,
		txn.GetID(),
		requestID,
		txn.pt.PostAssembly,
		txn.pt.PreAssembly,
		coordinator,
	).Return(expectedError)

	// Replace transport writer with mock
	originalTransport := txn.transportWriter
	txn.transportWriter = mockTransport

	// Execute the action
	err := action_ResendAssembleRevertResponse(ctx, txn, nil)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Restore original transport
	txn.transportWriter = originalTransport
}

func TestAction_ResendAssembleParkResponse_Success(t *testing.T) {
	// Test that action_ResendAssembleParkResponse calls action_SendAssembleParkResponse with correct parameters
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Parked)
	txn, mocks := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	// Set up PostAssembly with PARK result
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_PARK,
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txn.GetID().String(),
		},
	}

	// Reset the recorder to ensure clean state
	mocks.SentMessageRecorder.Reset(ctx)

	// Execute the action
	err := action_ResendAssembleParkResponse(ctx, txn, nil)

	// Verify no error
	assert.NoError(t, err)

	// Verify that SendAssembleResponse was called with park result
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "SendAssembleResponse should have been called with PARK result")
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "Should not have sent success response")
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "Should not have sent revert response")
}

func TestAction_ResendAssembleParkResponse_TransportError(t *testing.T) {
	// Test that action_ResendAssembleParkResponse returns error when transport fails
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Parked)
	txn, _ := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	// Set up PostAssembly with PARK result
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_PARK,
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &components.TransactionPreAssembly{}

	// Create a mock transport writer that returns an error
	mockTransport := transport.NewMockTransportWriter(t)
	expectedError := errors.New("transport error")
	mockTransport.EXPECT().SendAssembleResponse(
		mock.Anything,
		txn.GetID(),
		requestID,
		txn.pt.PostAssembly,
		txn.pt.PreAssembly,
		coordinator,
	).Return(expectedError)

	// Replace transport writer with mock
	originalTransport := txn.transportWriter
	txn.transportWriter = mockTransport

	// Execute the action
	err := action_ResendAssembleParkResponse(ctx, txn, nil)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Restore original transport
	txn.transportWriter = originalTransport
}

func TestGuard_AssembleRequestMatchesPreviousResponse_Matches(t *testing.T) {
	// Test that guard_AssembleRequestMatchesPreviousResponse returns true when request IDs match
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	// Set up matching request IDs
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{
		requestID: requestID,
	}

	matches := guard_AssembleRequestMatchesPreviousResponse(ctx, txn)

	assert.True(t, matches, "Should return true when request IDs match")
}

func TestGuard_AssembleRequestMatchesPreviousResponse_DoesNotMatch(t *testing.T) {
	// Test that guard_AssembleRequestMatchesPreviousResponse returns false when request IDs do not match
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	// Set up different request IDs
	fulfilledRequestID := uuid.New()
	newRequestID := uuid.New()
	// Ensure they're different
	for newRequestID == fulfilledRequestID {
		newRequestID = uuid.New()
	}

	txn.latestFulfilledAssembleRequestID = fulfilledRequestID
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{
		requestID: newRequestID,
	}

	matches := guard_AssembleRequestMatchesPreviousResponse(ctx, txn)

	assert.False(t, matches, "Should return false when request IDs do not match")
}

func TestGuard_AssembleRequestMatchesPreviousResponse_NilUUID(t *testing.T) {
	// Test that guard_AssembleRequestMatchesPreviousResponse handles nil UUID (uuid.Nil) correctly
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	// Set up both as nil UUID
	txn.latestFulfilledAssembleRequestID = uuid.Nil
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{
		requestID: uuid.Nil,
	}

	matches := guard_AssembleRequestMatchesPreviousResponse(ctx, txn)

	assert.True(t, matches, "Should return true when both request IDs are uuid.Nil")
}

func TestGuard_AssembleRequestMatchesPreviousResponse_OneNilUUID(t *testing.T) {
	// Test that guard_AssembleRequestMatchesPreviousResponse returns false when one is nil and other is not
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	// Set up one as nil UUID and one as a real UUID
	txn.latestFulfilledAssembleRequestID = uuid.Nil
	txn.latestAssembleRequest = &assembleRequestFromCoordinator{
		requestID: uuid.New(),
	}

	matches := guard_AssembleRequestMatchesPreviousResponse(ctx, txn)

	assert.False(t, matches, "Should return false when one request ID is nil and the other is not")
}

