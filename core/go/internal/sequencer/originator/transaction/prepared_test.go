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

func TestAction_ResendPreDispatchResponse_Success(t *testing.T) {
	// Test that action_ResendPreDispatchResponse calls action_SendPreDispatchResponse with correct parameters
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
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

	// Reset the recorder to ensure clean state
	mocks.SentMessageRecorder.Reset(ctx)

	// Execute the action
	err := action_ResendPreDispatchResponse(ctx, txn)

	// Verify no error
	assert.NoError(t, err)

	// Verify that SendPreDispatchResponse was called
	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "SendPreDispatchResponse should have been called")
}

func TestAction_ResendPreDispatchResponse_TransportError(t *testing.T) {
	// Test that action_ResendPreDispatchResponse returns error when transport fails
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
	txn, _ := builder.BuildWithMocks()

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
	err := action_ResendPreDispatchResponse(ctx, txn)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Restore original transport
	txn.transportWriter = originalTransport
}

