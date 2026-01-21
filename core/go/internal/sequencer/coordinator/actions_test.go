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

import (
	"context"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCoordinator_SendHandoverRequest_SuccessfullySendsHandoverRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build(ctx)
	activeCoordinatorNode := "activeCoordinatorNode"

	// Set the active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode

	// Call sendHandoverRequest
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	assert.True(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "handover request should have been sent")
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestWithCorrectActiveCoordinatorNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "testCoordinatorNode"

	// Set the active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestWithCorrectContractAddress(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	contractAddress := pldtypes.RandAddress()
	builder.ContractAddress(contractAddress)
	c, _ := builder.Build(ctx)
	activeCoordinatorNode := "activeCoordinatorNode"

	// Set the active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_HandlesErrorFromSendHandoverRequestGracefully(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "activeCoordinatorNode"
	expectedError := fmt.Errorf("transport error")

	// Set the active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode
	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(expectedError)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest - should not panic even when error occurs
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_HandlesEmptyActiveCoordinatorNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := ""

	// Set empty active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_node1(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "node1"

	// Set the active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_node2ExampleCom(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "node2@example.com"

	// Set the active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_coordinatorNode123(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "coordinator-node-123"

	// Set the active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_WithCoordinatorNode_VeryLongCoordinatorNodeName(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)
	contractAddress := builder.GetContractAddress()
	activeCoordinatorNode := "very-long-coordinator-node-name-with-special-chars-123"

	// Set the active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode

	mockTransport := transport.NewMockTransportWriter(t)
	mockTransport.On("SendHandoverRequest", ctx, activeCoordinatorNode, &contractAddress).Return(nil)
	c.transportWriter = mockTransport

	// Call sendHandoverRequest
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	mockTransport.AssertExpectations(t)
}

func TestCoordinator_SendHandoverRequest_SendsHandoverRequestMultipleTimes(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, mocks := builder.Build(ctx)
	activeCoordinatorNode := "activeCoordinatorNode"

	// Set the active coordinator node
	c.state.activeCoordinatorNode = activeCoordinatorNode

	// Call sendHandoverRequest multiple times
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)
	action_SendHandoverRequest(ctx, c.state, c.smConfig, c.smCallbacks, nil)

	assert.True(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "handover request should have been sent")
}

// TODO AM: debug
// func TestCoordinator_SendHandoverRequest_HandlesContextCancellation(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)
// 	contractAddress := builder.GetContractAddress()
// 	activeCoordinatorNode := "activeCoordinatorNode"

// 	// Set the active coordinator node
// 	c.state.activeCoordinatorNode = activeCoordinatorNode

// 	// Create a cancelled context
// 	cancelledCtx, cancel := context.WithCancel(ctx)
// 	cancel()

// 	mockTransport := transport.NewMockTransportWriter(t)
// 	mockTransport.On("SendHandoverRequest", cancelledCtx, activeCoordinatorNode, &contractAddress).Return(nil)
// 	c.smContext.transportWriter = mockTransport

// 	// Call sendHandoverRequest with cancelled context
// 	action_SendHandoverRequest(ctx, c.state, c.config, c.callbacks, nil)

// 	mockTransport.AssertExpectations(t)
// }

func TestCoordinator_PropagateHeartbeatIntervalToAllTransactions_ReturnsNilWhenNoTransactionsExist(t *testing.T) {
	state := &coordinatorState{
		// Ensure transactionsByID is empty
		transactionsByID: make(map[uuid.UUID]*transaction.Transaction),
	}
	err := action_PropagateEventToAllTransactions(t.Context(), state, nil, nil, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err, "should return nil when no transactions exist")
}

func TestCoordinator_PropagateHeartbeatIntervalToAllTransactions_SuccessfullyPropagatesEventToSingleTransaction(t *testing.T) {
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	state := &coordinatorState{
		transactionsByID: map[uuid.UUID]*transaction.Transaction{
			txn.ID: txn,
		},
	}
	err := action_PropagateEventToAllTransactions(t.Context(), state, nil, nil, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err, "should successfully propagate event to single transaction")
}

func TestCoordinator_PropagateHeartbeatIntervalToAllTransactions_SuccessfullyPropagatesEventToMultipleTransactions(t *testing.T) {
	txBuilder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn1 := txBuilder1.Build()

	txBuilder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn2 := txBuilder2.Build()

	txBuilder3 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched)
	txn3 := txBuilder3.Build()

	state := &coordinatorState{
		transactionsByID: map[uuid.UUID]*transaction.Transaction{
			txn1.ID: txn1,
			txn2.ID: txn2,
			txn3.ID: txn3,
		},
	}

	err := action_PropagateEventToAllTransactions(t.Context(), state, nil, nil, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err, "should successfully propagate event to all transactions")
}

func TestCoordinator_PropagateHeartbeatIntervalToAllTransactions_HandlesEventPropagationWithManyTransactions(t *testing.T) {
	numTransactions := 10
	state := &coordinatorState{
		transactionsByID: make(map[uuid.UUID]*transaction.Transaction, numTransactions),
	}

	// Create many transactions
	for i := 0; i < numTransactions; i++ {
		txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
		txn := txBuilder.Build()
		state.transactionsByID[txn.ID] = txn
	}

	// Verify we have the expected number of transactions
	assert.Equal(t, numTransactions, len(state.transactionsByID), "should have correct number of transactions")

	err := action_PropagateEventToAllTransactions(t.Context(), state, nil, nil, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err, "should successfully propagate event to all transactions")
}

func TestCoordinator_PropagateHeartbeatIntervalToAllTransactions_HandlesContextCancellationGracefully(t *testing.T) {
	// Create a transaction
	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled)
	txn := txBuilder.Build()

	state := &coordinatorState{
		transactionsByID: map[uuid.UUID]*transaction.Transaction{
			txn.ID: txn,
		},
	}
	// Create a cancelled context
	cancelledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	// Propagate event with cancelled context
	_ = action_PropagateEventToAllTransactions(cancelledCtx, state, nil, nil, &common.HeartbeatIntervalEvent{})

	// Just verify it doesn't panic
}

// TODO AM: these are tests a level higher than just the action- see how we're looking as we progress
// func TestCoordinator_PropagateHeartbeatIntervalToAllTransactions_IncrementsHeartbeatCounterForConfirmedTransaction(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)

// 	// Create a transaction in State_Confirmed with 4 heartbeat intervals
// 	// (grace period is 5, so after one more heartbeat it should transition to State_Final)
// 	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).
// 		HeartbeatIntervalsSinceStateChange(4)
// 	txn := txBuilder.Build()

// 	// Add transaction to coordinator
// 	c.state.transactionsByID[txn.ID] = txn
// 	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "transaction should start in State_Confirmed")

// 	// Propagate heartbeat event
// 	event := &common.HeartbeatIntervalEvent{}
// 	err := c.propagateEventToAllTransactions(ctx, event)
// 	assert.NoError(t, err)

// 	// Transaction should have transitioned to State_Final (counter went from 4 to 5, which >= grace period of 5)
// 	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "transaction should have transitioned to State_Final after heartbeat")
// }

// func TestCoordinator_PropagateHeartbeatIntervalToAllTransactions_IncrementsHeartbeatCounterForRevertedTransaction(t *testing.T) {
// 	ctx := context.Background()
// 	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
// 	c, _ := builder.Build(ctx)

// 	// Create a transaction in State_Reverted with 4 heartbeat intervals
// 	txBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Reverted).
// 		HeartbeatIntervalsSinceStateChange(4)
// 	txn := txBuilder.Build()

// 	// Add transaction to coordinator
// 	c.state.transactionsByID[txn.ID] = txn
// 	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "transaction should start in State_Reverted")

// 	// Propagate heartbeat event
// 	event := &common.HeartbeatIntervalEvent{}
// 	err := c.propagateEventToAllTransactions(ctx, event)
// 	assert.NoError(t, err)

// 	// Transaction should have transitioned to State_Final
// 	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "transaction should have transitioned to State_Final after heartbeat")
// }
