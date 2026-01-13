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
	"encoding/json"
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSendTransactionSubmitted_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		// Verify message type
		if msg.MessageType != MessageType_TransactionSubmitted {
			return false
		}
		// Verify node
		if msg.Node != originatorNode {
			return false
		}
		// Verify component
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		// Verify payload is valid proto
		var txSubmitted engineProto.TransactionSubmitted
		err := proto.Unmarshal(msg.Payload, &txSubmitted)
		if err != nil {
			return false
		}
		// Verify transaction ID
		if txSubmitted.TransactionId != txID.String() {
			return false
		}
		// Verify contract address
		if txSubmitted.ContractAddress != contractAddress.HexString() {
			return false
		}
		// Verify hash
		if len(txSubmitted.Hash) != 32 {
			return false
		}
		// Verify ID is set (should be a UUID)
		if txSubmitted.Id == "" {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, txID, originatorNode, contractAddress, txHash)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendTransactionSubmitted_Loopback(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, txID, originatorNode, contractAddress, txHash)
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_TransactionSubmitted, msg.MessageType)
		assert.Equal(t, originatorNode, msg.Node)
		// Verify payload
		var txSubmitted engineProto.TransactionSubmitted
		err := proto.Unmarshal(msg.Payload, &txSubmitted)
		require.NoError(t, err)
		assert.Equal(t, txID.String(), txSubmitted.TransactionId)
		assert.Equal(t, contractAddress.HexString(), txSubmitted.ContractAddress)
		assert.Equal(t, txHash.Bytes(), txSubmitted.Hash)
	default:
		t.Fatal("Expected message in loopback queue")
	}

	mockTransportManager.AssertExpectations(t)
	mockLoopbackTransport.AssertExpectations(t)
}

func TestSendTransactionSubmitted_NilContractAddress(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   nil,
	}

	err := tw.SendTransactionSubmitted(ctx, txID, originatorNode, nil, txHash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract address")

	// Verify no messages were sent
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendTransactionSubmitted_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, txID, originatorNode, contractAddress, txHash)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendTransactionSubmitted_VerifyProtoFields(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	var capturedPayload []byte
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		capturedPayload = msg.Payload
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, txID, originatorNode, contractAddress, txHash)
	require.NoError(t, err)

	// Unmarshal and verify all fields
	var txSubmitted engineProto.TransactionSubmitted
	err = proto.Unmarshal(capturedPayload, &txSubmitted)
	require.NoError(t, err)

	assert.Equal(t, txID.String(), txSubmitted.TransactionId)
	assert.Equal(t, contractAddress.HexString(), txSubmitted.ContractAddress)
	assert.Equal(t, txHash.Bytes(), txSubmitted.Hash)
	assert.NotEmpty(t, txSubmitted.Id) // Should have a generated UUID

	// Verify the ID is a valid UUID
	_, err = uuid.Parse(txSubmitted.Id)
	assert.NoError(t, err)

	mockTransportManager.AssertExpectations(t)
}

// ===== SendDelegationRequest Tests =====

func TestSendDelegationRequest_Success(t *testing.T) {
	ctx := context.Background()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := uint64(100)

	tx1ID := uuid.New()
	tx2ID := uuid.New()
	transactions := []*components.PrivateTransaction{
		{
			ID:      tx1ID,
			Domain:  "test-domain",
			Address: *contractAddress,
		},
		{
			ID:      tx2ID,
			Domain:  "test-domain",
			Address: *contractAddress,
		},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	callCount := 0
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		callCount++
		if msg.MessageType != MessageType_DelegationRequest {
			return false
		}
		if msg.Node != coordinatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var delegationRequest engineProto.DelegationRequest
		err := proto.Unmarshal(msg.Payload, &delegationRequest)
		if err != nil {
			return false
		}
		if callCount == 1 && delegationRequest.TransactionId != tx1ID.String() {
			return false
		}
		if callCount == 2 && delegationRequest.TransactionId != tx2ID.String() {
			return false
		}
		if delegationRequest.DelegateNodeId != coordinatorNode {
			return false
		}
		if delegationRequest.BlockHeight != int64(blockHeight) {
			return false
		}
		return true
	})).Return(nil).Times(2)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRequest(ctx, coordinatorNode, transactions, blockHeight)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendDelegationRequest_EmptyTransactions(t *testing.T) {
	ctx := context.Background()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := uint64(100)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRequest(ctx, coordinatorNode, []*components.PrivateTransaction{}, blockHeight)
	require.NoError(t, err)
	mockTransportManager.AssertNotCalled(t, "Send")
}

// ===== SendDelegationRequestAcknowledgment Tests =====

func TestSendDelegationRequestAcknowledgment_Success(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	delegateNodeName := "delegate-node"
	transactionID := uuid.New().String()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_DelegationRequestAcknowledgment {
			return false
		}
		if msg.Node != delegatingNodeName {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var ack engineProto.DelegationRequestAcknowledgment
		err := proto.Unmarshal(msg.Payload, &ack)
		if err != nil {
			return false
		}
		if ack.DelegationId != delegationId {
			return false
		}
		if ack.TransactionId != transactionID {
			return false
		}
		if ack.DelegateNodeId != delegateNodeName {
			return false
		}
		if ack.ContractAddress != contractAddress.String() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRequestAcknowledgment(ctx, delegatingNodeName, delegationId, delegateNodeName, transactionID)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendDelegationRequestAcknowledgment_SendError(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	delegateNodeName := "delegate-node"
	transactionID := uuid.New().String()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRequestAcknowledgment(ctx, delegatingNodeName, delegationId, delegateNodeName, transactionID)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

// ===== SendEndorsementRequest Tests =====

func TestSendEndorsementRequest_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	idempotencyKey := uuid.New()
	party := "party1@node1"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	attRequest := &prototk.AttestationRequest{
		Name: "test-attestation",
	}

	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddress.HexString(),
		},
	}

	verifiers := []*prototk.ResolvedVerifier{
		{Lookup: "verifier1"},
	}

	signatures := []*prototk.AttestationResult{
		{Name: "signature1"},
	}

	inputStates := []*prototk.EndorsableState{
		{Id: "input1"},
	}

	readStates := []*prototk.EndorsableState{
		{Id: "read1"},
	}

	outputStates := []*prototk.EndorsableState{
		{Id: "output1"},
	}

	infoStates := []*prototk.EndorsableState{
		{Id: "info1"},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_EndorsementRequest {
			return false
		}
		if msg.Node != "node1" {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var endorsementRequest engineProto.EndorsementRequest
		err := proto.Unmarshal(msg.Payload, &endorsementRequest)
		if err != nil {
			return false
		}
		if endorsementRequest.TransactionId != txID.String() {
			return false
		}
		if endorsementRequest.IdempotencyKey != idempotencyKey.String() {
			return false
		}
		if endorsementRequest.Party != party {
			return false
		}
		if endorsementRequest.ContractAddress != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendEndorsementRequest(ctx, txID, idempotencyKey, party, attRequest, transactionSpecification, verifiers, signatures, inputStates, readStates, outputStates, infoStates)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendEndorsementRequest_NodeLookupError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	idempotencyKey := uuid.New()
	party := "invalid-party" // No @node, will fail Node lookup
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	attRequest := &prototk.AttestationRequest{
		Name: "test-attestation",
	}

	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddress.HexString(),
		},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendEndorsementRequest(ctx, txID, idempotencyKey, party, attRequest, transactionSpecification, nil, nil, nil, nil, nil, nil)
	require.Error(t, err)
	mockTransportManager.AssertNotCalled(t, "Send")
}

// ===== SendEndorsementResponse Tests =====

func TestSendEndorsementResponse_Success(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	endorsementName := "test-endorsement"
	party := "party1"
	node := "target-node"
	revertReason := ""

	attResult := &prototk.AttestationResult{
		Name: "test-result",
	}

	endorsementResult := &components.EndorsementResult{
		Result: prototk.EndorseTransactionResponse_SIGN,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_EndorsementResponse {
			return false
		}
		if msg.Node != node {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var response engineProto.EndorsementResponse
		err := proto.Unmarshal(msg.Payload, &response)
		if err != nil {
			return false
		}
		if response.TransactionId != transactionId {
			return false
		}
		if response.IdempotencyKey != idempotencyKey {
			return false
		}
		if response.ContractAddress != contractAddress {
			return false
		}
		if response.AttestationRequestName != endorsementName {
			return false
		}
		if response.Party != party {
			return false
		}
		if response.RevertReason != nil {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementResponse(ctx, transactionId, idempotencyKey, contractAddress, attResult, endorsementResult, revertReason, endorsementName, party, node)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendEndorsementResponse_WithRevertReason(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	endorsementName := "test-endorsement"
	party := "party1"
	node := "target-node"
	revertReason := "transaction reverted"

	attResult := &prototk.AttestationResult{
		Name: "test-result",
	}

	endorsementResult := &components.EndorsementResult{
		Result: prototk.EndorseTransactionResponse_REVERT,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var response engineProto.EndorsementResponse
		err := proto.Unmarshal(msg.Payload, &response)
		if err != nil {
			return false
		}
		if response.RevertReason == nil || *response.RevertReason != revertReason {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementResponse(ctx, transactionId, idempotencyKey, contractAddress, attResult, endorsementResult, revertReason, endorsementName, party, node)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendEndorsementResponse_SendError(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	endorsementName := "test-endorsement"
	party := "party1"
	node := "target-node"

	attResult := &prototk.AttestationResult{
		Name: "test-result",
	}

	endorsementResult := &components.EndorsementResult{}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementResponse(ctx, transactionId, idempotencyKey, contractAddress, attResult, endorsementResult, "", endorsementName, party, node)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

// ===== SendAssembleRequest Tests =====

func TestSendAssembleRequest_Success(t *testing.T) {
	ctx := context.Background()
	assemblingNode := "assembling-node"
	txID := uuid.New()
	idempotencyId := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := int64(100)

	preAssembly := &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txID.String(),
		},
	}

	stateLocksJSON := []byte(`{"locks": []}`)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_AssembleRequest {
			return false
		}
		if msg.Node != assemblingNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var assembleRequest engineProto.AssembleRequest
		err := proto.Unmarshal(msg.Payload, &assembleRequest)
		if err != nil {
			return false
		}
		if assembleRequest.TransactionId != txID.String() {
			return false
		}
		if assembleRequest.AssembleRequestId != idempotencyId.String() {
			return false
		}
		if assembleRequest.ContractAddress != contractAddress.HexString() {
			return false
		}
		if assembleRequest.BlockHeight != blockHeight {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRequest(ctx, assemblingNode, txID, idempotencyId, preAssembly, stateLocksJSON, blockHeight)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendAssembleRequest_SendError(t *testing.T) {
	ctx := context.Background()
	assemblingNode := "assembling-node"
	txID := uuid.New()
	idempotencyId := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := int64(100)

	preAssembly := &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txID.String(),
		},
	}

	stateLocksJSON := []byte(`{"locks": []}`)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRequest(ctx, assemblingNode, txID, idempotencyId, preAssembly, stateLocksJSON, blockHeight)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

// ===== SendAssembleResponse Tests =====

func TestSendAssembleResponse_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	preAssembly := &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txID.String(),
		},
	}

	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_AssembleResponse {
			return false
		}
		if msg.Node != recipient {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var assembleResponse engineProto.AssembleResponse
		err := proto.Unmarshal(msg.Payload, &assembleResponse)
		if err != nil {
			return false
		}
		if assembleResponse.TransactionId != txID.String() {
			return false
		}
		if assembleResponse.AssembleRequestId != assembleRequestId.String() {
			return false
		}
		if assembleResponse.ContractAddress != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleResponse(ctx, txID, assembleRequestId, postAssembly, preAssembly, recipient)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendAssembleResponse_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	preAssembly := &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txID.String(),
		},
	}

	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleResponse(ctx, txID, assembleRequestId, postAssembly, preAssembly, recipient)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

// ===== SendHandoverRequest Tests =====

func TestSendHandoverRequest_Success(t *testing.T) {
	ctx := context.Background()
	activeCoordinator := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_HandoverRequest {
			return false
		}
		if msg.Node != activeCoordinator {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var handoverRequest HandoverRequest
		err := json.Unmarshal(msg.Payload, &handoverRequest)
		if err != nil {
			return false
		}
		if handoverRequest.ContractAddress == nil || handoverRequest.ContractAddress.HexString() != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendHandoverRequest(ctx, activeCoordinator, contractAddress)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendHandoverRequest_NilContractAddress(t *testing.T) {
	ctx := context.Background()
	activeCoordinator := "coordinator-node"

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   nil,
	}

	err := tw.SendHandoverRequest(ctx, activeCoordinator, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract address")
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendHandoverRequest_SendError(t *testing.T) {
	ctx := context.Background()
	activeCoordinator := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendHandoverRequest(ctx, activeCoordinator, contractAddress)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

// ===== SendNonceAssigned Tests =====

func TestSendNonceAssigned_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_NonceAssigned {
			return false
		}
		if msg.Node != originatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var nonceAssigned engineProto.NonceAssigned
		err := proto.Unmarshal(msg.Payload, &nonceAssigned)
		if err != nil {
			return false
		}
		if nonceAssigned.TransactionId != txID.String() {
			return false
		}
		if nonceAssigned.ContractAddress != contractAddress.HexString() {
			return false
		}
		if nonceAssigned.Nonce != int64(nonce) {
			return false
		}
		if nonceAssigned.Id == "" {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendNonceAssigned(ctx, txID, originatorNode, contractAddress, nonce)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendNonceAssigned_NilContractAddress(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   nil,
	}

	err := tw.SendNonceAssigned(ctx, txID, originatorNode, nil, nonce)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract address")
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendNonceAssigned_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendNonceAssigned(ctx, txID, originatorNode, contractAddress, nonce)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendNonceAssigned_VerifyGeneratedId(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	var capturedPayload []byte
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		capturedPayload = msg.Payload
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendNonceAssigned(ctx, txID, originatorNode, contractAddress, nonce)
	require.NoError(t, err)

	var nonceAssigned engineProto.NonceAssigned
	err = proto.Unmarshal(capturedPayload, &nonceAssigned)
	require.NoError(t, err)

	// Verify the ID is a valid UUID
	_, err = uuid.Parse(nonceAssigned.Id)
	assert.NoError(t, err)

	mockTransportManager.AssertExpectations(t)
}

// ===== SendTransactionConfirmed Tests =====

func TestSendTransactionConfirmed_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_TransactionConfirmed {
			return false
		}
		if msg.Node != originatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		if txConfirmed.TransactionId != txID.String() {
			return false
		}
		if txConfirmed.ContractAddress != contractAddress.HexString() {
			return false
		}
		if txConfirmed.Nonce != int64(*nonce) {
			return false
		}
		if string(txConfirmed.RevertReason) != string(revertReason) {
			return false
		}
		if txConfirmed.Id == "" {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, revertReason)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendTransactionConfirmed_WithoutNonce(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		// Nonce should be 0 when not provided
		if txConfirmed.Nonce != 0 {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nil, revertReason)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendTransactionConfirmed_NilContractAddress(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   nil,
	}

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, nil, nonce, revertReason)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract address")
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendTransactionConfirmed_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, revertReason)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendTransactionConfirmed_Loopback(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, revertReason)
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_TransactionConfirmed, msg.MessageType)
		assert.Equal(t, originatorNode, msg.Node)
		// Verify payload
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		require.NoError(t, err)
		assert.Equal(t, txID.String(), txConfirmed.TransactionId)
		assert.Equal(t, contractAddress.HexString(), txConfirmed.ContractAddress)
		assert.Equal(t, int64(*nonce), txConfirmed.Nonce)
		assert.Equal(t, revertReason, pldtypes.HexBytes(txConfirmed.RevertReason))
	default:
		t.Fatal("Expected message in loopback queue")
	}

	mockTransportManager.AssertExpectations(t)
	mockLoopbackTransport.AssertExpectations(t)
}

// ===== SendHeartbeat Tests =====

func TestSendHeartbeat_Success(t *testing.T) {
	ctx := context.Background()
	targetNode := "target-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorSnapshot := &common.CoordinatorSnapshot{
		CoordinatorState:       "Idle",
		BlockHeight:            100,
		FlushPoints:            []*common.FlushPoint{},
		PooledTransactions:     []*common.Transaction{},
		DispatchedTransactions: []*common.DispatchedTransaction{},
		ConfirmedTransactions:  []*common.ConfirmedTransaction{},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_CoordinatorHeartbeatNotification {
			return false
		}
		if msg.Node != targetNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var heartbeat engineProto.CoordinatorHeartbeatNotification
		err := proto.Unmarshal(msg.Payload, &heartbeat)
		if err != nil {
			return false
		}
		if heartbeat.From != "local-node" {
			return false
		}
		if heartbeat.ContractAddress != contractAddress.HexString() {
			return false
		}
		// Verify coordinator snapshot can be unmarshalled
		var snapshot common.CoordinatorSnapshot
		err = json.Unmarshal(heartbeat.CoordinatorSnapshot, &snapshot)
		if err != nil {
			return false
		}
		if snapshot.CoordinatorState != coordinatorSnapshot.CoordinatorState {
			return false
		}
		if snapshot.BlockHeight != coordinatorSnapshot.BlockHeight {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendHeartbeat(ctx, targetNode, contractAddress, coordinatorSnapshot)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendHeartbeat_SendError(t *testing.T) {
	ctx := context.Background()
	targetNode := "target-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorSnapshot := &common.CoordinatorSnapshot{
		CoordinatorState: "Idle",
		BlockHeight:      100,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendHeartbeat(ctx, targetNode, contractAddress, coordinatorSnapshot)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendHeartbeat_Loopback(t *testing.T) {
	ctx := context.Background()
	targetNode := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorSnapshot := &common.CoordinatorSnapshot{
		CoordinatorState: "Idle",
		BlockHeight:      100,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendHeartbeat(ctx, targetNode, contractAddress, coordinatorSnapshot)
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_CoordinatorHeartbeatNotification, msg.MessageType)
		assert.Equal(t, targetNode, msg.Node)
		// Verify payload
		var heartbeat engineProto.CoordinatorHeartbeatNotification
		err := proto.Unmarshal(msg.Payload, &heartbeat)
		require.NoError(t, err)
		assert.Equal(t, "local-node", heartbeat.From)
		assert.Equal(t, contractAddress.HexString(), heartbeat.ContractAddress)
	default:
		t.Fatal("Expected message in loopback queue")
	}

	mockTransportManager.AssertExpectations(t)
	mockLoopbackTransport.AssertExpectations(t)
}

// ===== SendPreDispatchRequest Tests =====

func TestSendPreDispatchRequest_Success(t *testing.T) {
	ctx := context.Background()
	originatorNode := "originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}
	hashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	hash := &hashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_PreDispatchRequest {
			return false
		}
		if msg.Node != originatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var dispatchRequest engineProto.TransactionDispatched
		err := proto.Unmarshal(msg.Payload, &dispatchRequest)
		if err != nil {
			return false
		}
		if dispatchRequest.Id != idempotencyKey.String() {
			return false
		}
		if dispatchRequest.TransactionId != transactionSpecification.TransactionId {
			return false
		}
		if dispatchRequest.ContractAddress != contractAddress.HexString() {
			return false
		}
		if len(dispatchRequest.PostAssembleHash) != 32 {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRequest(ctx, originatorNode, idempotencyKey, transactionSpecification, hash)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendPreDispatchRequest_SendError(t *testing.T) {
	ctx := context.Background()
	originatorNode := "originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}
	hashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	hash := &hashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRequest(ctx, originatorNode, idempotencyKey, transactionSpecification, hash)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendPreDispatchRequest_Loopback(t *testing.T) {
	ctx := context.Background()
	originatorNode := "local-node" // Same as local node name
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}
	hashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	hash := &hashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRequest(ctx, originatorNode, idempotencyKey, transactionSpecification, hash)
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_PreDispatchRequest, msg.MessageType)
		assert.Equal(t, originatorNode, msg.Node)
		// Verify payload
		var dispatchRequest engineProto.TransactionDispatched
		err := proto.Unmarshal(msg.Payload, &dispatchRequest)
		require.NoError(t, err)
		assert.Equal(t, idempotencyKey.String(), dispatchRequest.Id)
		assert.Equal(t, transactionSpecification.TransactionId, dispatchRequest.TransactionId)
		assert.Equal(t, contractAddress.HexString(), dispatchRequest.ContractAddress)
		assert.Equal(t, hash.Bytes(), dispatchRequest.PostAssembleHash)
	default:
		t.Fatal("Expected message in loopback queue")
	}

	mockTransportManager.AssertExpectations(t)
	mockLoopbackTransport.AssertExpectations(t)
}

// ===== SendPreDispatchResponse Tests =====

func TestSendPreDispatchResponse_Success(t *testing.T) {
	ctx := context.Background()
	transactionOriginatorNode := "originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_PreDispatchResponse {
			return false
		}
		if msg.Node != transactionOriginatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var dispatchResponse engineProto.TransactionDispatched
		err := proto.Unmarshal(msg.Payload, &dispatchResponse)
		if err != nil {
			return false
		}
		if dispatchResponse.Id != idempotencyKey.String() {
			return false
		}
		if dispatchResponse.TransactionId != transactionSpecification.TransactionId {
			return false
		}
		if dispatchResponse.ContractAddress != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchResponse(ctx, transactionOriginatorNode, idempotencyKey, transactionSpecification)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendPreDispatchResponse_SendError(t *testing.T) {
	ctx := context.Background()
	transactionOriginatorNode := "originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchResponse(ctx, transactionOriginatorNode, idempotencyKey, transactionSpecification)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendPreDispatchResponse_Loopback(t *testing.T) {
	ctx := context.Background()
	transactionOriginatorNode := "local-node" // Same as local node name
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchResponse(ctx, transactionOriginatorNode, idempotencyKey, transactionSpecification)
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_PreDispatchResponse, msg.MessageType)
		assert.Equal(t, transactionOriginatorNode, msg.Node)
		// Verify payload
		var dispatchResponse engineProto.TransactionDispatched
		err := proto.Unmarshal(msg.Payload, &dispatchResponse)
		require.NoError(t, err)
		assert.Equal(t, idempotencyKey.String(), dispatchResponse.Id)
		assert.Equal(t, transactionSpecification.TransactionId, dispatchResponse.TransactionId)
		assert.Equal(t, contractAddress.HexString(), dispatchResponse.ContractAddress)
	default:
		t.Fatal("Expected message in loopback queue")
	}

	mockTransportManager.AssertExpectations(t)
	mockLoopbackTransport.AssertExpectations(t)
}

// ===== SendDispatched Tests =====

func TestSendDispatched_Success(t *testing.T) {
	ctx := context.Background()
	transactionOriginator := "originator@originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_Dispatched {
			return false
		}
		if msg.Node != "originator-node" {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var dispatched engineProto.TransactionDispatched
		err := proto.Unmarshal(msg.Payload, &dispatched)
		if err != nil {
			return false
		}
		if dispatched.Id != idempotencyKey.String() {
			return false
		}
		if dispatched.TransactionId != transactionSpecification.TransactionId {
			return false
		}
		if dispatched.ContractAddress != contractAddress.HexString() {
			return false
		}
		if dispatched.Signer != transactionOriginator {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, transactionOriginator, idempotencyKey, transactionSpecification)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendDispatched_NodeLookupError(t *testing.T) {
	ctx := context.Background()
	transactionOriginator := "invalid-originator" // No @node, will fail Node lookup
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, transactionOriginator, idempotencyKey, transactionSpecification)
	require.Error(t, err)
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendDispatched_SendError(t *testing.T) {
	ctx := context.Background()
	transactionOriginator := "originator@originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	sendError := errors.New("transport send error")
	mockTransportManager.On("Send", ctx, mock.Anything).Return(sendError)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, transactionOriginator, idempotencyKey, transactionSpecification)
	require.Error(t, err)
	assert.Equal(t, sendError, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendDispatched_Loopback(t *testing.T) {
	ctx := context.Background()
	transactionOriginator := "originator@local-node" // Node is same as local node name
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, transactionOriginator, idempotencyKey, transactionSpecification)
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_Dispatched, msg.MessageType)
		assert.Equal(t, "local-node", msg.Node)
		// Verify payload
		var dispatched engineProto.TransactionDispatched
		err := proto.Unmarshal(msg.Payload, &dispatched)
		require.NoError(t, err)
		assert.Equal(t, idempotencyKey.String(), dispatched.Id)
		assert.Equal(t, transactionSpecification.TransactionId, dispatched.TransactionId)
		assert.Equal(t, contractAddress.HexString(), dispatched.ContractAddress)
		assert.Equal(t, transactionOriginator, dispatched.Signer)
	default:
		t.Fatal("Expected message in loopback queue")
	}

	mockTransportManager.AssertExpectations(t)
	mockLoopbackTransport.AssertExpectations(t)
}

func TestSendTransactionUnknown_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_TransactionUnknown {
			return false
		}
		if msg.Node != coordinatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var txUnknown engineProto.TransactionUnknown
		err := proto.Unmarshal(msg.Payload, &txUnknown)
		if err != nil {
			return false
		}
		if txUnknown.TransactionId != txID.String() {
			return false
		}
		if txUnknown.ContractAddress != contractAddress.HexString() {
			return false
		}
		if txUnknown.Id == "" {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionUnknown(ctx, coordinatorNode, txID)
	require.NoError(t, err)
	mockTransportManager.AssertExpectations(t)
}

func TestSendTransactionUnknown_NilContractAddress(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	coordinatorNode := "coordinator-node"

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   nil, // No contract address
	}

	err := tw.SendTransactionUnknown(ctx, coordinatorNode, txID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attempt to send transaction unknown without specifying contract address")
}

func TestSendTransactionUnknown_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("send failed"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionUnknown(ctx, coordinatorNode, txID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send failed")
	mockTransportManager.AssertExpectations(t)
}

func TestSendTransactionUnknown_Loopback(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	coordinatorNode := "local-node" // Same as local node
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := NewMockLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)

	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionUnknown(ctx, coordinatorNode, txID)
	require.NoError(t, err)

	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_TransactionUnknown, msg.MessageType)
		assert.Equal(t, "local-node", msg.Node)
		// Verify payload
		var txUnknown engineProto.TransactionUnknown
		err := proto.Unmarshal(msg.Payload, &txUnknown)
		require.NoError(t, err)
		assert.Equal(t, txID.String(), txUnknown.TransactionId)
		assert.Equal(t, contractAddress.HexString(), txUnknown.ContractAddress)
	default:
		t.Fatal("Expected message in loopback queue")
	}

	mockTransportManager.AssertExpectations(t)
	mockLoopbackTransport.AssertExpectations(t)
}
