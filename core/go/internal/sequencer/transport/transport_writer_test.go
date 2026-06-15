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
	"slices"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencertransportmocks"
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, txID, originatorNode, contractAddress, txHash)
	require.NoError(t, err)
}

func TestSendTransactionSubmitted_Loopback(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
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

}

func TestSendTransactionSubmitted_NilContractAddress(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, txID, originatorNode, contractAddress, txHash)
	require.NoError(t, err)
}

func TestSendTransactionSubmitted_VerifyProtoFields(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	var capturedPayload []byte
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		capturedPayload = msg.Payload
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	var capturedRequest engineProto.DelegationRequest
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_DelegationRequest {
			return false
		}
		if msg.Node != coordinatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		return proto.Unmarshal(msg.Payload, &capturedRequest) == nil
	})).Return(nil).Times(1)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRequest(ctx, coordinatorNode, transactions, blockHeight)
	require.NoError(t, err)

	assert.Equal(t, coordinatorNode, capturedRequest.DelegateNodeId)
	assert.Equal(t, int64(blockHeight), capturedRequest.OriginatorBlockHeight)
	require.Len(t, capturedRequest.PrivateTransactions, 2)

	var tx1 components.PrivateTransaction
	require.NoError(t, json.Unmarshal(capturedRequest.PrivateTransactions[0], &tx1))
	assert.Equal(t, tx1ID, tx1.ID)

	var tx2 components.PrivateTransaction
	require.NoError(t, json.Unmarshal(capturedRequest.PrivateTransactions[1], &tx2))
	assert.Equal(t, tx2ID, tx2.ID)
}

func TestSendDelegationRequest_EmptyTransactions(t *testing.T) {
	ctx := context.Background()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := uint64(100)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	var capturedRequest engineProto.DelegationRequest
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		return msg.MessageType == MessageType_DelegationRequest &&
			proto.Unmarshal(msg.Payload, &capturedRequest) == nil
	})).Return(nil).Times(1)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRequest(ctx, coordinatorNode, []*components.PrivateTransaction{}, blockHeight)
	require.NoError(t, err)
	assert.Empty(t, capturedRequest.PrivateTransactions)
}

// ===== SendDelegationResponse Tests =====

func TestSendDelegationResponse_Success(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	transactionIDs := make([]string, 0)
	transactionIDs = append(transactionIDs, uuid.New().String())
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_DelegationResponse {
			return false
		}
		if msg.Node != delegatingNodeName {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var ack engineProto.DelegationResponse
		err := proto.Unmarshal(msg.Payload, &ack)
		if err != nil {
			return false
		}
		if ack.DelegationId != delegationId {
			return false
		}
		for _, transactionID := range ack.TransactionIds {
			if !slices.Contains(transactionIDs, transactionID) {
				return false
			}
		}
		if ack.DelegateNodeId != delegatingNodeName {
			return false
		}
		if ack.ContractAddress != contractAddress.String() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationResponse(ctx, delegatingNodeName, delegationId, transactionIDs, []int64{0}, uint64(100))
	require.NoError(t, err)
}

func TestSendDelegationResponse_SendError(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	transactionID := uuid.New().String()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationResponse(ctx, delegatingNodeName, delegationId, []string{transactionID}, []int64{0}, uint64(100))
	require.NoError(t, err)
}

// ===== SendDelegationRejection Tests =====

func TestSendDelegationRejection_Success(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	blockHeight := uint64(100)
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_DelegationRejection {
			return false
		}
		if msg.Node != delegatingNodeName {
			return false
		}
		var ack engineProto.DelegationRejection
		if err := proto.Unmarshal(msg.Payload, &ack); err != nil {
			return false
		}
		return ack.DelegationId == delegationId &&
			ack.DelegateNodeId == delegatingNodeName &&
			ack.ContractAddress == contractAddress.String()
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRejection(ctx, delegatingNodeName, delegationId, engineProto.RejectionReason_NOT_CURRENT_DELEGATE, "active-coordinator", int64(blockHeight), int64(90), int64(0))
	require.NoError(t, err)
}

func TestSendDelegationRejection_SendError(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRejection(ctx, delegatingNodeName, delegationId, engineProto.RejectionReason_NOT_CURRENT_DELEGATE, "node1", int64(100), int64(90), int64(0))
	require.NoError(t, err)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendEndorsementRequest(ctx, txID, idempotencyKey, party, attRequest, transactionSpecification, verifiers, signatures, inputStates, readStates, outputStates, infoStates, time.Time{}, 0, 0)
	require.NoError(t, err)
}

func TestSendEndorsementRequest_SerialisesExpiryTimeIntoProto(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	idempotencyKey := uuid.New()
	party := "party1@node1"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
		ContractInfo:  &prototk.ContractInfo{ContractAddress: contractAddress.HexString()},
	}
	expiry := time.Now().Truncate(time.Millisecond) // truncate to ms precision to match UnixMilli roundtrip

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	var capturedExpiry int64
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var req engineProto.EndorsementRequest
		if err := proto.Unmarshal(msg.Payload, &req); err != nil {
			return false
		}
		capturedExpiry = req.ExpiryTimeUnixMs
		return msg.MessageType == MessageType_EndorsementRequest
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}
	err := tw.SendEndorsementRequest(ctx, txID, idempotencyKey, party, nil, transactionSpecification, nil, nil, nil, nil, nil, nil, expiry, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, expiry.UnixMilli(), capturedExpiry, "ExpiryTimeUnixMs should match expiry.UnixMilli()")
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendEndorsementRequest(ctx, txID, idempotencyKey, party, attRequest, transactionSpecification, nil, nil, nil, nil, nil, nil, time.Time{}, 0, 0)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementResponse(ctx, transactionId, idempotencyKey, contractAddress, attResult, endorsementResult, "", endorsementName, party, node)
	require.NoError(t, err)
}

// ===== SendEndorsementRejection Tests =====

func TestSendEndorsementRejection_Success(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	endorsementName := "att1"
	party := "party1@node2"
	node := "node2"
	coordinatorBlockHeight := int64(100)
	endorserBlockHeight := int64(95)
	blockHeightTolerance := int64(10)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_EndorsementRejection {
			return false
		}
		if msg.Node != node {
			return false
		}
		var r engineProto.EndorsementRejection
		if err := proto.Unmarshal(msg.Payload, &r); err != nil {
			return false
		}
		return r.TransactionId == transactionId &&
			r.IdempotencyKey == idempotencyKey &&
			r.ContractAddress == contractAddress &&
			r.AttestationRequestName == endorsementName &&
			r.Party == party &&
			r.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE && // reason passed explicitly
			r.CoordinatorBlockHeight == coordinatorBlockHeight &&
			r.EndorserBlockHeight == endorserBlockHeight &&
			r.BlockHeightTolerance == blockHeightTolerance
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementRejection(ctx, transactionId, idempotencyKey, contractAddress, endorsementName, party, node, engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, coordinatorBlockHeight, endorserBlockHeight, blockHeightTolerance)
	require.NoError(t, err)
}

func TestSendEndorsementRejection_SendError(t *testing.T) {
	ctx := context.Background()
	contractAddress := "0x1234567890123456789012345678901234567890"

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementRejection(ctx, uuid.New().String(), uuid.New().String(), contractAddress, "att1", "party1@node2", "node2", engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, 100, 95, 10)
	require.NoError(t, err)
}

// ===== SendEndorsementError Tests =====

func TestSendEndorsementError_Success(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	errorMessage := "domain returned unexpected error"
	party := "party1@node2"
	attestationRequestName := "att1"
	node := "node2"

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_EndorsementError {
			return false
		}
		if msg.Node != node {
			return false
		}
		var e engineProto.EndorsementError
		if err := proto.Unmarshal(msg.Payload, &e); err != nil {
			return false
		}
		return e.TransactionId == transactionId &&
			e.IdempotencyKey == idempotencyKey &&
			e.ContractAddress == contractAddress &&
			e.ErrorMessage == errorMessage &&
			e.Party == party &&
			e.AttestationRequestName == attestationRequestName
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementError(ctx, transactionId, idempotencyKey, contractAddress, errorMessage, party, attestationRequestName, node)
	require.NoError(t, err)
}

func TestSendEndorsementError_SendError(t *testing.T) {
	ctx := context.Background()
	contractAddress := "0x1234567890123456789012345678901234567890"

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementError(ctx, uuid.New().String(), uuid.New().String(), contractAddress, "some error", "party1@node2", "att1", "node2")
	require.NoError(t, err)
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

	stateLocks := grapher.ExportableStates{LockedState: []*grapher.StateLock{}}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
		if assembleRequest.CoordinatorBlockHeight != blockHeight {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRequest(ctx, assemblingNode, txID, idempotencyId, preAssembly, stateLocks, blockHeight, time.Time{}, 0)
	require.NoError(t, err)
}

func TestSendAssembleRequest_SerialisesExpiryTimeIntoProto(t *testing.T) {
	ctx := context.Background()
	assemblingNode := "assembling-node"
	txID := uuid.New()
	idempotencyId := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := int64(100)
	preAssembly := &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{TransactionId: txID.String()},
	}
	stateLocks := grapher.ExportableStates{LockedState: []*grapher.StateLock{}}
	expiry := time.Now().Truncate(time.Millisecond) // truncate to ms precision to match UnixMilli roundtrip

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	var capturedExpiry int64
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var req engineProto.AssembleRequest
		if err := proto.Unmarshal(msg.Payload, &req); err != nil {
			return false
		}
		capturedExpiry = req.ExpiryTimeUnixMs
		return msg.MessageType == MessageType_AssembleRequest
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}
	err := tw.SendAssembleRequest(ctx, assemblingNode, txID, idempotencyId, preAssembly, stateLocks, blockHeight, expiry, 0)
	require.NoError(t, err)
	assert.Equal(t, expiry.UnixMilli(), capturedExpiry, "ExpiryTimeUnixMs should match expiry.UnixMilli()")
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

	stateLocks := grapher.ExportableStates{LockedState: []*grapher.StateLock{}}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRequest(ctx, assemblingNode, txID, idempotencyId, preAssembly, stateLocks, blockHeight, time.Time{}, 0)
	require.NoError(t, err)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleResponse(ctx, txID, assembleRequestId, postAssembly, preAssembly, recipient)
	require.NoError(t, err)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleResponse(ctx, txID, assembleRequestId, postAssembly, preAssembly, recipient)
	require.NoError(t, err)
}

// ===== SendAssembleError Tests =====

func TestSendAssembleError_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_AssembleError {
			return false
		}
		if msg.Node != recipient {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var assembleError engineProto.AssembleError
		err := proto.Unmarshal(msg.Payload, &assembleError)
		if err != nil {
			return false
		}
		if assembleError.TransactionId != txID.String() {
			return false
		}
		if assembleError.AssembleRequestId != assembleRequestId.String() {
			return false
		}
		if assembleError.ContractAddress != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleError(ctx, txID, assembleRequestId, recipient)
	require.NoError(t, err)
}

func TestSendAssembleError_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleError(ctx, txID, assembleRequestId, recipient)
	require.NoError(t, err)
}

func TestSendAssembleError_Loopback(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleError(ctx, txID, assembleRequestId, recipient)
	require.NoError(t, err)

	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_AssembleError, msg.MessageType)
		assert.Equal(t, recipient, msg.Node)
		var assembleError engineProto.AssembleError
		err := proto.Unmarshal(msg.Payload, &assembleError)
		require.NoError(t, err)
		assert.Equal(t, txID.String(), assembleError.TransactionId)
		assert.Equal(t, assembleRequestId.String(), assembleError.AssembleRequestId)
		assert.Equal(t, contractAddress.HexString(), assembleError.ContractAddress)
	default:
		t.Fatal("Expected message in loopback queue")
	}

}

// ===== SendAssembleRejection Tests =====

func TestSendAssembleRejection_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorBlockHeight := int64(100)
	assemblerBlockHeight := int64(95)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_AssembleRejection {
			return false
		}
		if msg.Node != recipient {
			return false
		}
		var r engineProto.AssembleRejection
		if err := proto.Unmarshal(msg.Payload, &r); err != nil {
			return false
		}
		return r.TransactionId == txID.String() &&
			r.AssembleRequestId == assembleRequestId.String() &&
			r.ContractAddress == contractAddress.HexString() &&
			r.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
			r.CoordinatorBlockHeight == coordinatorBlockHeight &&
			r.AssemblerBlockHeight == assemblerBlockHeight
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRejection(ctx, txID, assembleRequestId, recipient, engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, coordinatorBlockHeight, assemblerBlockHeight)
	require.NoError(t, err)
}

func TestSendAssembleRejection_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRejection(ctx, txID, assembleRequestId, recipient, engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, 100, 95)
	require.NoError(t, err)
}

// ===== SendNonceAssigned Tests =====

func TestSendNonceAssigned_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
}

func TestSendNonceAssigned_NilContractAddress(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendNonceAssigned(ctx, txID, originatorNode, contractAddress, nonce)
	require.NoError(t, err)
}

func TestSendNonceAssigned_VerifyGeneratedId(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false)
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_WithoutNonce(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nil, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false)
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_NilContractAddress(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   nil,
	}

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, nil, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false)
	require.NoError(t, err)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", true)
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
		assert.True(t, txConfirmed.WillRetry)
	default:
		t.Fatal("Expected message in loopback queue")
	}

}

func TestSendTransactionConfirmed_WillRetryTrue(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		if txConfirmed.TransactionId != txID.String() {
			return false
		}
		if !txConfirmed.WillRetry {
			return false
		}
		if string(txConfirmed.RevertReason) != string(revertReason) {
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

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", true)
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_WillRetryFalse(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		if txConfirmed.WillRetry {
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

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false)
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_NilRevertReason(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		if txConfirmed.TransactionId != txID.String() {
			return false
		}
		if len(txConfirmed.RevertReason) != 0 {
			return false
		}
		if txConfirmed.WillRetry {
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

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_SUCCESS, nil, "", false)
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_Loopback_WillRetryFalse(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "local-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, txID, originatorNode, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false)
	require.NoError(t, err)

	select {
	case msg := <-loopbackQueue:
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		require.NoError(t, err)
		assert.Equal(t, txID.String(), txConfirmed.TransactionId)
		assert.Equal(t, revertReason, pldtypes.HexBytes(txConfirmed.RevertReason))
		assert.False(t, txConfirmed.WillRetry)
	default:
		t.Fatal("Expected message in loopback queue")
	}
}

// ===== SendHeartbeat Tests =====

func TestSendHeartbeat_Success(t *testing.T) {
	ctx := context.Background()
	targetNode := "target-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorSnapshot := &common.CoordinatorSnapshot{
		CoordinatorState:       common.CoordinatorState_Idle,
		BlockHeight:            100,
		PooledTransactions:     []*common.SnapshotPooledTransaction{},
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{},
		ConfirmedTransactions:  []*common.SnapshotConfirmedTransaction{},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
}

func TestSendHeartbeat_SendError(t *testing.T) {
	ctx := context.Background()
	targetNode := "target-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorSnapshot := &common.CoordinatorSnapshot{
		CoordinatorState: common.CoordinatorState_Idle,
		BlockHeight:      100,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendHeartbeat(ctx, targetNode, contractAddress, coordinatorSnapshot)
	require.NoError(t, err)
}

func TestSendHeartbeat_Loopback(t *testing.T) {
	ctx := context.Background()
	targetNode := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorSnapshot := &common.CoordinatorSnapshot{
		CoordinatorState: common.CoordinatorState_Idle,
		BlockHeight:      100,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRequest(ctx, originatorNode, idempotencyKey, transactionSpecification, hash)
	require.NoError(t, err)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchResponse(ctx, transactionOriginatorNode, idempotencyKey, transactionSpecification)
	require.NoError(t, err)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, transactionOriginator, idempotencyKey, transactionSpecification)
	require.NoError(t, err)
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
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
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

}

// ===== SendPreDispatchRejection Tests =====

func TestSendPreDispatchRejection_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	requestID := uuid.New()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_PreDispatchRejection {
			return false
		}
		if msg.Node != coordinatorNode {
			return false
		}
		var r engineProto.PreDispatchRejection
		if err := proto.Unmarshal(msg.Payload, &r); err != nil {
			return false
		}
		return r.TransactionId == txID.String() &&
			r.RequestId == requestID.String() &&
			r.ContractAddress == contractAddress.HexString() &&
			r.RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRejection(ctx, txID, requestID, coordinatorNode, engineProto.RejectionReason_NOT_CURRENT_DELEGATE)
	require.NoError(t, err)
}

func TestSendPreDispatchRejection_NilContractAddress(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	requestID := uuid.New()
	coordinatorNode := "coordinator-node"

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   nil,
	}

	err := tw.SendPreDispatchRejection(ctx, txID, requestID, coordinatorNode, engineProto.RejectionReason_NOT_CURRENT_DELEGATE)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attempt to send pre-dispatch rejection without specifying contract address")
}

func TestSendPreDispatchRejection_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	requestID := uuid.New()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("send failed"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRejection(ctx, txID, requestID, coordinatorNode, engineProto.RejectionReason_TRANSACTION_UNKNOWN)
	require.NoError(t, err)
}
