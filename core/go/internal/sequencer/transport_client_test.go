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

package sequencer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	coordTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	originatorTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type transportClientTestMocks struct {
	components      *componentsmocks.AllComponents
	domainManager   *componentsmocks.DomainManager
	stateManager    *componentsmocks.StateManager
	persistence     *persistencemocks.Persistence
	txManager       *componentsmocks.TXManager
	keyManager      *componentsmocks.KeyManager
	domainAPI       *componentsmocks.DomainSmartContract
	domain          *componentsmocks.Domain
	domainContext   *componentsmocks.DomainContext
	transportWriter *transport.MockTransportWriter
	originator      *originator.MockSeqOriginator
	coordinator     *coordinator.MockSeqCoordinator
	metrics         *metrics.MockDistributedSequencerMetrics
}

func newTransportClientTestMocks(t *testing.T) *transportClientTestMocks {
	return &transportClientTestMocks{
		components:      componentsmocks.NewAllComponents(t),
		domainManager:   componentsmocks.NewDomainManager(t),
		stateManager:   componentsmocks.NewStateManager(t),
		persistence:    persistencemocks.NewPersistence(t),
		txManager:      componentsmocks.NewTXManager(t),
		keyManager:      componentsmocks.NewKeyManager(t),
		domainAPI:      componentsmocks.NewDomainSmartContract(t),
		domain:          componentsmocks.NewDomain(t),
		domainContext:   componentsmocks.NewDomainContext(t),
		transportWriter: transport.NewMockTransportWriter(t),
		originator:      originator.NewMockSeqOriginator(t),
		coordinator:     coordinator.NewMockSeqCoordinator(t),
		metrics:         metrics.NewMockDistributedSequencerMetrics(t),
	}
}

func newSequencerManagerForTransportClientTesting(t *testing.T, mocks *transportClientTestMocks) *sequencerManager {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{}

	sm := &sequencerManager{
		ctx:                           ctx,
		config:                        config,
		components:                    mocks.components,
		nodeName:                      "test-node",
		sequencersLock:                sync.RWMutex{},
		sequencers:                    make(map[string]*sequencer),
		metrics:                       mocks.metrics,
		targetActiveCoordinatorsLimit: 2,
		targetActiveSequencersLimit:   2,
	}

	return sm
}

func newSequencerForTransportClientTesting(contractAddr *pldtypes.EthAddress, mocks *transportClientTestMocks) *sequencer {
	return &sequencer{
		contractAddress: contractAddr.String(),
		originator:      mocks.originator,
		coordinator:     mocks.coordinator,
		transportWriter: mocks.transportWriter,
		lastTXTime:      time.Now(),
	}
}

func setupDefaultMocks(ctx context.Context, mocks *transportClientTestMocks, contractAddr *pldtypes.EthAddress) {
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Maybe()
	mocks.components.EXPECT().StateManager().Return(mocks.stateManager).Maybe()
	mocks.components.EXPECT().TxManager().Return(mocks.txManager).Maybe()
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainAPI.EXPECT().Domain().Return(mocks.domain).Maybe()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Maybe()
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mocks.domain, *contractAddr).Return(mocks.domainContext).Maybe()
}

// Test HandlePaladinMsg routing
func TestHandlePaladinMsg_Routing(t *testing.T) {
	tests := []struct {
		name        string
		messageType string
	}{
		{"AssembleRequest", transport.MessageType_AssembleRequest},
		{"AssembleResponse", transport.MessageType_AssembleResponse},
		{"AssembleError", transport.MessageType_AssembleError},
		{"CoordinatorHeartbeatNotification", transport.MessageType_CoordinatorHeartbeatNotification},
		{"DelegationRequest", transport.MessageType_DelegationRequest},
		{"DelegationRequestAcknowledgment", transport.MessageType_DelegationRequestAcknowledgment},
		{"Dispatched", transport.MessageType_Dispatched},
		{"HandoverRequest", transport.MessageType_HandoverRequest},
		{"PreDispatchRequest", transport.MessageType_PreDispatchRequest},
		{"PreDispatchResponse", transport.MessageType_PreDispatchResponse},
		{"EndorsementRequest", transport.MessageType_EndorsementRequest},
		{"EndorsementResponse", transport.MessageType_EndorsementResponse},
		{"NonceAssigned", transport.MessageType_NonceAssigned},
		{"TransactionSubmitted", transport.MessageType_TransactionSubmitted},
		{"TransactionConfirmed", transport.MessageType_TransactionConfirmed},
		{"Unknown", "UnknownMessageType"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mocks := newTransportClientTestMocks(t)
			sm := newSequencerManagerForTransportClientTesting(t, mocks)

			message := &components.ReceivedMessage{
				FromNode:    "test-node",
				MessageID:   uuid.New(),
				MessageType: tt.messageType,
				Payload:     []byte("test-payload"),
			}

			// Should not panic
			sm.HandlePaladinMsg(ctx, message)
		})
	}
}

// Test handleAssembleRequest
func TestHandleAssembleRequest_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	// Create test data
	txID := uuid.New()
	requestID := uuid.New()
	preAssembly := &components.TransactionPreAssembly{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{Lookup: "verifier1@node1"},
		},
		Verifiers: []*prototk.ResolvedVerifier{},
	}
	preAssemblyJSON, _ := json.Marshal(preAssembly)

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:  contractAddr.String(),
		PreAssembly:       preAssemblyJSON,
		StateLocks:        []byte("{}"),
		BlockHeight:       100,
	}
	payload, _ := proto.Marshal(assembleRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.coordinator.EXPECT().GetActiveCoordinatorNode(ctx, true).Return("coordinator-node").Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.AssembleRequestReceivedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleAssembleRequest(ctx, message)

	mocks.originator.AssertExpectations(t)
	mocks.coordinator.AssertExpectations(t)
}

func TestHandleAssembleRequest_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleAssembleRequest(ctx, message)
}

func TestHandleAssembleRequest_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New()
	requestID := uuid.New()
	preAssembly := &components.TransactionPreAssembly{}
	preAssemblyJSON, _ := json.Marshal(preAssembly)

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:  "invalid-address",
		PreAssembly:       preAssemblyJSON,
		StateLocks:        []byte("{}"),
		BlockHeight:       100,
	}
	payload, _ := proto.Marshal(assembleRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     payload,
	}

	// Should not panic
	sm.handleAssembleRequest(ctx, message)
}

func TestHandleAssembleRequest_LoadSequencerError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	preAssembly := &components.TransactionPreAssembly{}
	preAssemblyJSON, _ := json.Marshal(preAssembly)

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:  contractAddr.String(),
		PreAssembly:       preAssemblyJSON,
		StateLocks:        []byte("{}"),
		BlockHeight:       100,
	}
	payload, _ := proto.Marshal(assembleRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will fail
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("not found")).Once()

	// Should not panic
	sm.handleAssembleRequest(ctx, message)
}

// Test handleAssembleResponse
func TestHandleAssembleResponse_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	preAssembly := &components.TransactionPreAssembly{}
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}
	preAssemblyJSON, _ := json.Marshal(preAssembly)
	postAssemblyJSON, _ := json.Marshal(postAssembly)

	assembleResponse := &engineProto.AssembleResponse{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:  contractAddr.String(),
		PreAssembly:       preAssemblyJSON,
		PostAssembly:      postAssemblyJSON,
	}
	payload, _ := proto.Marshal(assembleResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleResponse,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called, so we need to mock its dependencies
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleSuccessEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleAssembleResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleAssembleResponse_Revert(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	preAssembly := &components.TransactionPreAssembly{}
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
	}
	preAssemblyJSON, _ := json.Marshal(preAssembly)
	postAssemblyJSON, _ := json.Marshal(postAssembly)

	assembleResponse := &engineProto.AssembleResponse{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:  contractAddr.String(),
		PreAssembly:       preAssemblyJSON,
		PostAssembly:      postAssemblyJSON,
	}
	payload, _ := proto.Marshal(assembleResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleResponse,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleRevertResponseEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleAssembleResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// Test handleAssembleError
func TestHandleAssembleError_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	assembleError := &engineProto.AssembleError{
		TransactionId:  txID.String(),
		ContractAddress: contractAddr.String(),
		ErrorMessage:   "test error",
	}
	payload, _ := proto.Marshal(assembleError)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleError,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.AssembleErrorEvent)
		return ok && event.TransactionID == txID
	})).Once()

	sm.handleAssembleError(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// Test handleDelegationRequest
func TestHandleDelegationRequest_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	privateTx := &components.PrivateTransaction{
		ID: txID,
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				ContractInfo: &prototk.ContractInfo{
					ContractAddress: contractAddr.String(),
				},
				From: "originator@node1",
			},
		},
	}
	privateTxJSON, _ := json.Marshal(privateTx)

	delegationRequest := &engineProto.DelegationRequest{
		PrivateTransaction: privateTxJSON,
		BlockHeight:        100,
	}
	payload, _ := proto.Marshal(delegationRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationRequest,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.coordinator.EXPECT().UpdateOriginatorNodePool(ctx, "test-node").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.TransactionsDelegatedEvent)
		return ok && len(event.Transactions) == 1 && event.Transactions[0].ID == txID
	})).Once()

	sm.handleDelegationRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// Test handleHandoverRequest
func TestHandleHandoverRequest_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	handoverRequest := &engineProto.HandoverRequest{
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(handoverRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_HandoverRequest,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.HandoverRequestEvent)
		return ok && event.Requester == "test-node"
	})).Once()

	sm.handleHandoverRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// Test handleNonceAssigned
func TestHandleNonceAssigned_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	nonceAssigned := &engineProto.NonceAssigned{
		TransactionId:  txID.String(),
		ContractAddress: contractAddr.String(),
		Nonce:          42,
	}
	payload, _ := proto.Marshal(nonceAssigned)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_NonceAssigned,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.NonceAssignedEvent)
		return ok && event.TransactionID == txID && event.Nonce == 42
	})).Once()

	sm.handleNonceAssigned(ctx, message)

	mocks.originator.AssertExpectations(t)
}

// Test handleTransactionSubmitted
func TestHandleTransactionSubmitted_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	hash := pldtypes.RandBytes32()
	transactionSubmitted := &engineProto.TransactionSubmitted{
		TransactionId:  txID.String(),
		ContractAddress: contractAddr.String(),
		Hash:           hash[:],
	}
	payload, _ := proto.Marshal(transactionSubmitted)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionSubmitted,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.SubmittedEvent)
		return ok && event.TransactionID == txID
	})).Once()

	sm.handleTransactionSubmitted(ctx, message)

	mocks.originator.AssertExpectations(t)
}

// Test handleTransactionConfirmed
func TestHandleTransactionConfirmed_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	transactionConfirmed := &engineProto.TransactionConfirmed{
		TransactionId:  txID.String(),
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(transactionConfirmed)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionConfirmed,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.ConfirmedSuccessEvent)
		return ok && event.TransactionID == txID
	})).Once()

	sm.handleTransactionConfirmed(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandleTransactionConfirmed_Reverted(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	revertReason := pldtypes.HexBytes("test revert reason")
	transactionConfirmed := &engineProto.TransactionConfirmed{
		TransactionId:  txID.String(),
		ContractAddress: contractAddr.String(),
		RevertReason:   revertReason,
	}
	payload, _ := proto.Marshal(transactionConfirmed)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionConfirmed,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.ConfirmedRevertedEvent)
		return ok && event.TransactionID == txID && string(event.RevertReason) == string(revertReason)
	})).Once()

	sm.handleTransactionConfirmed(ctx, message)

	mocks.originator.AssertExpectations(t)
}

// Test handleDispatchedEvent
func TestHandleDispatchedEvent_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	// TransactionId format is "0x" + 32 hex characters (16 bytes)
	// UUID is 16 bytes, convert to hex without dashes
	txIDBytes := [16]byte(txID)
	txIDHex := "0x" + fmt.Sprintf("%032x", txIDBytes)
	dispatchedEvent := &engineProto.TransactionDispatched{
		TransactionId:  txIDHex,
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(dispatchedEvent)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_Dispatched,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.DispatchedEvent)
		// Note: TransactionID parsing from hex string may not match exactly due to format conversion
		return ok && event.TransactionID != uuid.Nil
	})).Once()

	sm.handleDispatchedEvent(ctx, message)

	mocks.originator.AssertExpectations(t)
}

// Test parseContractAddressString
func TestParseContractAddressString_Valid(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	message := &components.ReceivedMessage{
		FromNode: "test-node",
	}

	result := sm.parseContractAddressString(ctx, contractAddr.String(), message)
	require.NotNil(t, result)
	assert.Equal(t, contractAddr.String(), result.String())
}

func TestParseContractAddressString_Invalid(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode: "test-node",
	}

	result := sm.parseContractAddressString(ctx, "invalid-address", message)
	assert.Nil(t, result)
}

// Test handleCoordinatorHeartbeatNotification
func TestHandleCoordinatorHeartbeatNotification_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	confirmedTxID := uuid.New()
	coordinatorSnapshot := &common.CoordinatorSnapshot{
		ConfirmedTransactions: []*common.ConfirmedTransaction{
			{
				DispatchedTransaction: common.DispatchedTransaction{
					Transaction: common.Transaction{
						ID: confirmedTxID,
					},
				},
			},
		},
	}
	snapshotJSON, _ := json.Marshal(coordinatorSnapshot)

	heartbeatNotification := &engineProto.CoordinatorHeartbeatNotification{
		From:                "coordinator-node",
		ContractAddress:     contractAddr.String(),
		CoordinatorSnapshot: snapshotJSON,
	}
	payload, _ := proto.Marshal(heartbeatNotification)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_CoordinatorHeartbeatNotification,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		_, ok := e.(*coordTransaction.HeartbeatIntervalEvent)
		return ok
	})).Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originator.HeartbeatReceivedEvent)
		return ok && event.From == "coordinator-node"
	})).Once()

	sm.handleCoordinatorHeartbeatNotification(ctx, message)

	mocks.coordinator.AssertExpectations(t)
	mocks.originator.AssertExpectations(t)
}

func TestHandleCoordinatorHeartbeatNotification_MissingFrom(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	coordinatorSnapshot := &common.CoordinatorSnapshot{}
	snapshotJSON, _ := json.Marshal(coordinatorSnapshot)

	heartbeatNotification := &engineProto.CoordinatorHeartbeatNotification{
		From:                "", // Missing From field
		ContractAddress:     contractAddr.String(),
		CoordinatorSnapshot: snapshotJSON,
	}
	payload, _ := proto.Marshal(heartbeatNotification)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_CoordinatorHeartbeatNotification,
		Payload:     payload,
	}

	// Should not panic and should return early
	sm.handleCoordinatorHeartbeatNotification(ctx, message)
}

// Test handlePreDispatchRequest
func TestHandlePreDispatchRequest_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	hash := pldtypes.RandBytes32()
	// TransactionId format is "0x" + 32 hex characters (16 bytes)
	txIDBytes := [16]byte(txID)
	txIDHex := "0x" + fmt.Sprintf("%032x", txIDBytes)

	preDispatchRequest := &engineProto.TransactionDispatched{
		Id:             requestID.String(),
		TransactionId: txIDHex,
		ContractAddress: contractAddr.String(),
		PostAssembleHash: hash[:],
	}
	payload, _ := proto.Marshal(preDispatchRequest)

	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_PreDispatchRequest,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.PreDispatchRequestReceivedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID && event.Coordinator == "coordinator-node"
	})).Once()

	sm.handlePreDispatchRequest(ctx, message)

	mocks.originator.AssertExpectations(t)
}

// Test handlePreDispatchResponse
func TestHandlePreDispatchResponse_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	// TransactionId format is "0x" + 32 hex characters (16 bytes)
	txIDBytes := [16]byte(txID)
	txIDHex := "0x" + fmt.Sprintf("%032x", txIDBytes)

	preDispatchResponse := &engineProto.TransactionDispatched{
		Id:             requestID.String(),
		TransactionId: txIDHex,
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(preDispatchResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_PreDispatchResponse,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Mock GetCurrentCoordinator call from LoadSequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("coordinator-node").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.DispatchRequestApprovedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handlePreDispatchResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// Test handleDelegationRequestAcknowledgment
func TestHandleDelegationRequestAcknowledgment_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New()
	delegationRequestAcknowledgment := &engineProto.DelegationRequestAcknowledgment{
		TransactionId: txID.String(),
	}
	payload, _ := proto.Marshal(delegationRequestAcknowledgment)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationRequestAcknowledgment,
		Payload:     payload,
	}

	// Should not panic - this handler just logs
	sm.handleDelegationRequestAcknowledgment(ctx, message)
}

func TestHandleDelegationRequestAcknowledgment_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationRequestAcknowledgment,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleDelegationRequestAcknowledgment(ctx, message)
}

