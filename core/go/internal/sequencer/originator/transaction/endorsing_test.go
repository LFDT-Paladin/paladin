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
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
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
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			Signatures: []*prototk.AttestationResult{
				{
					Payload: []byte("test signature"),
				},
			},
		},
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &prototk.TransactionPreAssembly{
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
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).WithMockTransportWriter()
	txn, mocks := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	// Set up PostAssembly with OK result
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
		},
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &prototk.TransactionPreAssembly{}

	expectedError := errors.New("transport error")
	mocks.TransportWriter.EXPECT().SendAssembleResponse(
		mock.Anything,
		coordinator,
		mock.MatchedBy(func(msg *engineProto.AssembleResponse) bool {
			return msg.TransactionId == txn.GetID().String() &&
				msg.AssembleRequestId == requestID.String()
		}),
	).Return(expectedError)

	// Execute the action
	err := action_ResendAssembleSuccessResponse(ctx, txn, nil)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
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
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   ptrTo("test revert reason"),
		},
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &prototk.TransactionPreAssembly{
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
	builder := NewTransactionBuilderForTesting(t, State_Reverted).WithMockTransportWriter()
	txn, mocks := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	// Set up PostAssembly with REVERT result
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   ptrTo("test revert reason"),
		},
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &prototk.TransactionPreAssembly{}

	expectedError := errors.New("transport error")
	mocks.TransportWriter.EXPECT().SendAssembleResponse(
		mock.Anything,
		coordinator,
		mock.MatchedBy(func(msg *engineProto.AssembleResponse) bool {
			return msg.TransactionId == txn.GetID().String() &&
				msg.AssembleRequestId == requestID.String()
		}),
	).Return(expectedError)

	// Execute the action
	err := action_ResendAssembleRevertResponse(ctx, txn, nil)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
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
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_PARK,
		},
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &prototk.TransactionPreAssembly{
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
	builder := NewTransactionBuilderForTesting(t, State_Parked).WithMockTransportWriter()
	txn, mocks := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	// Set up PostAssembly with PARK result
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_PARK,
		},
	}

	// Set up PreAssembly
	txn.pt.PreAssembly = &prototk.TransactionPreAssembly{}

	expectedError := errors.New("transport error")
	mocks.TransportWriter.EXPECT().SendAssembleResponse(
		mock.Anything,
		coordinator,
		mock.MatchedBy(func(msg *engineProto.AssembleResponse) bool {
			return msg.TransactionId == txn.GetID().String() &&
				msg.AssembleRequestId == requestID.String()
		}),
	).Return(expectedError)

	// Execute the action
	err := action_ResendAssembleParkResponse(ctx, txn, nil)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestValidator_AssembleRequestMatchesPreviousResponse_Matches(t *testing.T) {
	// Returns true when the event's request ID matches the most recently fulfilled request
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	requestID := uuid.New()
	txn.latestFulfilledAssembleRequestID = requestID

	matches, err := validator_AssembleRequestMatchesPreviousResponse(ctx, txn, &AssembleRequestReceivedEvent{RequestID: requestID})

	assert.NoError(t, err)
	assert.True(t, matches, "Should return true when request IDs match")
}

func TestValidator_AssembleRequestMatchesPreviousResponse_DoesNotMatch(t *testing.T) {
	// Returns false when the event's request ID differs from the most recently fulfilled request
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	fulfilledRequestID := uuid.New()
	newRequestID := uuid.New()
	for newRequestID == fulfilledRequestID {
		newRequestID = uuid.New()
	}

	txn.latestFulfilledAssembleRequestID = fulfilledRequestID

	matches, err := validator_AssembleRequestMatchesPreviousResponse(ctx, txn, &AssembleRequestReceivedEvent{RequestID: newRequestID})

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when request IDs do not match")
}

func TestValidator_AssembleRequestMatchesPreviousResponse_NilUUID(t *testing.T) {
	// Handles nil UUID (uuid.Nil) correctly
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	txn.latestFulfilledAssembleRequestID = uuid.Nil

	matches, err := validator_AssembleRequestMatchesPreviousResponse(ctx, txn, &AssembleRequestReceivedEvent{RequestID: uuid.Nil})

	assert.NoError(t, err)
	assert.True(t, matches, "Should return true when both request IDs are uuid.Nil")
}

func TestValidator_AssembleRequestMatchesPreviousResponse_OneNilUUID(t *testing.T) {
	// Returns false when one is nil and the other is not
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	txn.latestFulfilledAssembleRequestID = uuid.Nil

	matches, err := validator_AssembleRequestMatchesPreviousResponse(ctx, txn, &AssembleRequestReceivedEvent{RequestID: uuid.New()})

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when one request ID is nil and the other is not")
}
