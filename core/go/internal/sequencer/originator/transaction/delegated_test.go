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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_Delegated_EmptyCoordinator_ReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, _ := builder.BuildWithMocks()
	event := &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: "",
	}
	err := action_Delegated(ctx, txn, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction delegate cannot be set to an empty node identity")
}

func Test_action_Delegated_SetsDelegateAndUpdatesTime(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, _ := builder.BuildWithMocks()
	coordinator := "coord@node1"
	event := &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: coordinator,
	}
	err := action_Delegated(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, coordinator, txn.currentDelegate)
	assert.NotNil(t, txn.lastDelegatedTime)
}

func TestAction_SendPreDispatchResponse_Success(t *testing.T) {
	// Test that action_SendPreDispatchResponse calls SendPreDispatchResponse with correct parameters
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestPreDispatchRequestID = requestID

	// Ensure PreAssembly has TransactionSpecification
	transactionSpec := &prototk.TransactionSpecification{
		TransactionId: txn.GetID().String(),
		From:          "originator@node1",
	}
	if txn.pt.PreAssembly == nil {
		txn.pt.PreAssembly = &components.TransactionPreAssembly{}
	}
	txn.pt.PreAssembly.TransactionSpecification = transactionSpec

	// Execute the action
	err := action_SendPreDispatchResponse(ctx, txn, nil)

	// Verify no error
	assert.NoError(t, err)

	// Verify that SendPreDispatchResponse was called
	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "SendPreDispatchResponse should have been called")
}

func TestAction_SendPreDispatchResponse_TransportError(t *testing.T) {
	// Test that action_SendPreDispatchResponse returns error when transport fails
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	// Set up required fields first
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestPreDispatchRequestID = requestID

	// Ensure PreAssembly has TransactionSpecification
	transactionSpec := &prototk.TransactionSpecification{
		TransactionId: txn.GetID().String(),
		From:          "originator@node1",
	}
	if txn.pt.PreAssembly == nil {
		txn.pt.PreAssembly = &components.TransactionPreAssembly{}
	}
	txn.pt.PreAssembly.TransactionSpecification = transactionSpec

	// Create a mock transport writer that returns an error
	mockTransport := transport.NewMockTransportWriter(t)
	expectedError := errors.New("transport error")
	mockTransport.EXPECT().SendPreDispatchResponse(
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(expectedError)

	// Replace transport writer with mock
	originalTransport := txn.transportWriter
	txn.transportWriter = mockTransport

	// Execute the action
	err := action_SendPreDispatchResponse(ctx, txn, nil)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Restore original transport
	txn.transportWriter = originalTransport
}

func TestValidator_AssembleRequestMatches_Matches(t *testing.T) {
	// Test that validator_AssembleRequestMatches returns true when coordinator matches
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator

	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: coordinator,
		RequestID:   uuid.New(),
	}

	matches, err := validator_AssembleRequestMatches(ctx, txn, event)

	assert.NoError(t, err)
	assert.True(t, matches, "Should return true when coordinator matches")
}

func TestValidator_AssembleRequestMatches_DoesNotMatch(t *testing.T) {
	// Test that validator_AssembleRequestMatches returns false when coordinator does not match
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	differentCoordinator := "coordinator@node2"
	txn.currentDelegate = coordinator

	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: differentCoordinator,
		RequestID:   uuid.New(),
	}

	matches, err := validator_AssembleRequestMatches(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when coordinator does not match")
}

func TestValidator_AssembleRequestMatches_WrongEventType(t *testing.T) {
	// Test that validator_AssembleRequestMatches returns false when event type is wrong
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator

	// Use a different event type
	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: coordinator,
	}

	matches, err := validator_AssembleRequestMatches(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when event type is wrong")
}

func TestValidator_PreDispatchRequestMatchesAssembledDelegation_Success(t *testing.T) {
	// Test that validator_PreDispatchRequestMatchesAssembledDelegation returns true when coordinator and hash match
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	// Set up transaction with PostAssembly so Hash() works
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		Signatures: []*prototk.AttestationResult{
			{
				Payload: []byte("test signature"),
			},
		},
	}

	// Get the transaction hash
	txnHash, err := txn.GetHash(ctx)
	require.NoError(t, err)
	require.NotNil(t, txnHash)

	requestID := uuid.New()
	event := &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator:      coordinator,
		PostAssemblyHash: txnHash,
		RequestID:        requestID,
	}

	matches, err := validator_PreDispatchRequestMatchesAssembledDelegation(ctx, txn, event)

	assert.NoError(t, err)
	assert.True(t, matches, "Should return true when coordinator and hash match")
	// Note: request ID is stored by action_PreDispatchRequestReceived (first action) when the event is processed via HandleEvent
}

func TestValidator_PreDispatchRequestMatchesAssembledDelegation_WrongCoordinator(t *testing.T) {
	// Test that validator_PreDispatchRequestMatchesAssembledDelegation returns false when coordinator does not match
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	differentCoordinator := "coordinator@node2"
	txn.currentDelegate = coordinator
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		Signatures: []*prototk.AttestationResult{
			{
				Payload: []byte("test signature"),
			},
		},
	}

	// Get the transaction hash
	txnHash, err := txn.GetHash(ctx)
	require.NoError(t, err)
	require.NotNil(t, txnHash)

	event := &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator:      differentCoordinator,
		PostAssemblyHash: txnHash,
		RequestID:        uuid.New(),
	}

	matches, err := validator_PreDispatchRequestMatchesAssembledDelegation(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when coordinator does not match")
	assert.Equal(t, uuid.Nil, txn.latestPreDispatchRequestID, "Should not store request ID when validation fails")
}

func TestValidator_PreDispatchRequestMatchesAssembledDelegation_WrongHash(t *testing.T) {
	// Test that validator_PreDispatchRequestMatchesAssembledDelegation returns false when hash does not match
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		Signatures: []*prototk.AttestationResult{
			{
				Payload: []byte("test signature"),
			},
		},
	}

	// Get the transaction hash
	txnHash, err := txn.GetHash(ctx)
	require.NoError(t, err)
	require.NotNil(t, txnHash)

	// Create a different hash
	differentHash := ptrTo(pldtypes.RandBytes32())
	// Ensure it's different
	for differentHash.Equals(txnHash) {
		differentHash = ptrTo(pldtypes.RandBytes32())
	}

	event := &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator:      coordinator,
		PostAssemblyHash: differentHash,
		RequestID:        uuid.New(),
	}

	matches, err := validator_PreDispatchRequestMatchesAssembledDelegation(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when hash does not match")
	assert.Equal(t, uuid.Nil, txn.latestPreDispatchRequestID, "Should not store request ID when validation fails")
}

func TestValidator_PreDispatchRequestMatchesAssembledDelegation_WrongEventType(t *testing.T) {
	// Test that validator_PreDispatchRequestMatchesAssembledDelegation returns false when event type is wrong
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator

	// Use a different event type
	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: coordinator,
	}

	matches, err := validator_PreDispatchRequestMatchesAssembledDelegation(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when event type is wrong")
}

func TestValidator_PreDispatchRequestMatchesAssembledDelegation_HashError(t *testing.T) {
	// Test that validator_PreDispatchRequestMatchesAssembledDelegation returns error when Hash() fails
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator

	// Set PostAssembly to nil to cause Hash() to fail
	txn.pt.PostAssembly = nil

	event := &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator:      coordinator,
		PostAssemblyHash: ptrTo(pldtypes.RandBytes32()),
		RequestID:        uuid.New(),
	}

	matches, err := validator_PreDispatchRequestMatchesAssembledDelegation(ctx, txn, event)

	assert.Error(t, err, "Should return error when Hash() fails")
	assert.False(t, matches, "Should return false when there's an error")
	assert.Contains(t, err.Error(), "cannot hash transaction without PostAssembly", "Error should indicate missing PostAssembly")
}
