/*
 * Copyright © 2024 Kaleido, Inc.
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

package transport

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLoopbackTransportWriter(t *testing.T) {
	handlerCalled := false
	var receivedMsg *components.ReceivedMessage

	handler := func(ctx context.Context, message *components.ReceivedMessage) {
		handlerCalled = true
		receivedMsg = message
	}

	writer := NewLoopbackTransportWriter(handler)

	require.NotNil(t, writer)
	assert.NotNil(t, writer.LoopbackQueue())
	
	// Verify the queue is initialized with buffer size 1
	queue := writer.LoopbackQueue()
	assert.NotNil(t, queue)
	
	// Verify handler is set by calling Send
	ctx := context.Background()
	send := &components.FireAndForgetMessageSend{
		Node:        "test-node",
		MessageType: "test-message",
		Payload:     []byte("test-payload"),
	}
	
	err := writer.Send(ctx, send)
	require.NoError(t, err)
	assert.True(t, handlerCalled)
	assert.NotNil(t, receivedMsg)
}

func TestLoopbackTransportWriter_Send_WithAllFields(t *testing.T) {
	var receivedMsg *components.ReceivedMessage
	messageID := uuid.New()
	correlationID := uuid.New()

	handler := func(ctx context.Context, message *components.ReceivedMessage) {
		receivedMsg = message
	}

	writer := NewLoopbackTransportWriter(handler)
	ctx := context.Background()

	send := &components.FireAndForgetMessageSend{
		Node:          "test-node",
		MessageType:   "test-message-type",
		Payload:       []byte("test-payload-data"),
		MessageID:     &messageID,
		CorrelationID: &correlationID,
	}

	err := writer.Send(ctx, send)
	require.NoError(t, err)
	require.NotNil(t, receivedMsg)

	assert.Equal(t, "test-node", receivedMsg.FromNode)
	assert.Equal(t, "test-message-type", receivedMsg.MessageType)
	assert.Equal(t, []byte("test-payload-data"), receivedMsg.Payload)
	assert.Equal(t, messageID, receivedMsg.MessageID)
	assert.Equal(t, &correlationID, receivedMsg.CorrelationID)
}

func TestLoopbackTransportWriter_Send_WithNilMessageID(t *testing.T) {
	var receivedMsg *components.ReceivedMessage
	correlationID := uuid.New()

	handler := func(ctx context.Context, message *components.ReceivedMessage) {
		receivedMsg = message
	}

	writer := NewLoopbackTransportWriter(handler)
	ctx := context.Background()

	send := &components.FireAndForgetMessageSend{
		Node:          "test-node",
		MessageType:   "test-message-type",
		Payload:       []byte("test-payload"),
		MessageID:     nil,
		CorrelationID: &correlationID,
	}

	err := writer.Send(ctx, send)
	require.NoError(t, err)
	require.NotNil(t, receivedMsg)

	assert.Equal(t, "test-node", receivedMsg.FromNode)
	assert.Equal(t, "test-message-type", receivedMsg.MessageType)
	assert.Equal(t, []byte("test-payload"), receivedMsg.Payload)
	assert.Equal(t, uuid.Nil, receivedMsg.MessageID) // Should be zero value when nil
	assert.Equal(t, &correlationID, receivedMsg.CorrelationID)
}

func TestLoopbackTransportWriter_Send_WithNilCorrelationID(t *testing.T) {
	var receivedMsg *components.ReceivedMessage
	messageID := uuid.New()

	handler := func(ctx context.Context, message *components.ReceivedMessage) {
		receivedMsg = message
	}

	writer := NewLoopbackTransportWriter(handler)
	ctx := context.Background()

	send := &components.FireAndForgetMessageSend{
		Node:          "test-node",
		MessageType:   "test-message-type",
		Payload:       []byte("test-payload"),
		MessageID:     &messageID,
		CorrelationID: nil,
	}

	err := writer.Send(ctx, send)
	require.NoError(t, err)
	require.NotNil(t, receivedMsg)

	assert.Equal(t, "test-node", receivedMsg.FromNode)
	assert.Equal(t, "test-message-type", receivedMsg.MessageType)
	assert.Equal(t, []byte("test-payload"), receivedMsg.Payload)
	assert.Equal(t, messageID, receivedMsg.MessageID)
	assert.Nil(t, receivedMsg.CorrelationID)
}

func TestLoopbackTransportWriter_Send_WithNilIDs(t *testing.T) {
	var receivedMsg *components.ReceivedMessage

	handler := func(ctx context.Context, message *components.ReceivedMessage) {
		receivedMsg = message
	}

	writer := NewLoopbackTransportWriter(handler)
	ctx := context.Background()

	send := &components.FireAndForgetMessageSend{
		Node:          "test-node",
		MessageType:   "test-message-type",
		Payload:       []byte("test-payload"),
		MessageID:     nil,
		CorrelationID: nil,
	}

	err := writer.Send(ctx, send)
	require.NoError(t, err)
	require.NotNil(t, receivedMsg)

	assert.Equal(t, "test-node", receivedMsg.FromNode)
	assert.Equal(t, "test-message-type", receivedMsg.MessageType)
	assert.Equal(t, []byte("test-payload"), receivedMsg.Payload)
	assert.Equal(t, uuid.Nil, receivedMsg.MessageID)
	assert.Nil(t, receivedMsg.CorrelationID)
}

func TestLoopbackTransportWriter_Send_ContextPropagation(t *testing.T) {
	var receivedCtx context.Context

	handler := func(ctx context.Context, message *components.ReceivedMessage) {
		receivedCtx = ctx
	}

	writer := NewLoopbackTransportWriter(handler)
	
	ctx := context.WithValue(context.Background(), "test-key", "test-value")
	send := &components.FireAndForgetMessageSend{
		Node:        "test-node",
		MessageType: "test-message",
		Payload:     []byte("test-payload"),
	}

	err := writer.Send(ctx, send)
	require.NoError(t, err)
	assert.Equal(t, ctx, receivedCtx)
	assert.Equal(t, "test-value", receivedCtx.Value("test-key"))
}

func TestLoopbackTransportWriter_LoopbackQueue(t *testing.T) {
	handler := func(ctx context.Context, message *components.ReceivedMessage) {}

	writer := NewLoopbackTransportWriter(handler)

	queue1 := writer.LoopbackQueue()
	queue2 := writer.LoopbackQueue()

	// Should return the same channel instance
	assert.Equal(t, queue1, queue2)
	assert.NotNil(t, queue1)

	// Verify the queue can accept messages
	send := &components.FireAndForgetMessageSend{
		Node:        "test-node",
		MessageType: "test-message",
		Payload:     []byte("test-payload"),
	}

	select {
	case queue1 <- send:
		// Successfully sent
	default:
		t.Fatal("Queue should be able to accept at least one message")
	}

	// Verify we can receive from the queue
	select {
	case received := <-queue1:
		assert.Equal(t, send, received)
	default:
		t.Fatal("Should be able to receive the message we just sent")
	}
}

func TestLoopbackTransportWriter_Send_MultipleCalls(t *testing.T) {
	var callCount int
	var receivedMessages []*components.ReceivedMessage

	handler := func(ctx context.Context, message *components.ReceivedMessage) {
		callCount++
		receivedMessages = append(receivedMessages, message)
	}

	writer := NewLoopbackTransportWriter(handler)
	ctx := context.Background()

	send1 := &components.FireAndForgetMessageSend{
		Node:        "node-1",
		MessageType: "message-1",
		Payload:     []byte("payload-1"),
	}

	send2 := &components.FireAndForgetMessageSend{
		Node:        "node-2",
		MessageType: "message-2",
		Payload:     []byte("payload-2"),
	}

	err1 := writer.Send(ctx, send1)
	require.NoError(t, err1)

	err2 := writer.Send(ctx, send2)
	require.NoError(t, err2)

	assert.Equal(t, 2, callCount)
	assert.Len(t, receivedMessages, 2)
	assert.Equal(t, "node-1", receivedMessages[0].FromNode)
	assert.Equal(t, "node-2", receivedMessages[1].FromNode)
}

