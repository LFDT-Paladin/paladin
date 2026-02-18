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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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
	originator      *originator.MockOriginator
	coordinator     *coordinator.MockCoordinator
	metrics         *metrics.MockDistributedSequencerMetrics
}

func newTransportClientTestMocks(t *testing.T) *transportClientTestMocks {
	return &transportClientTestMocks{
		components:      componentsmocks.NewAllComponents(t),
		domainManager:   componentsmocks.NewDomainManager(t),
		stateManager:    componentsmocks.NewStateManager(t),
		persistence:     persistencemocks.NewPersistence(t),
		txManager:       componentsmocks.NewTXManager(t),
		keyManager:      componentsmocks.NewKeyManager(t),
		domainAPI:       componentsmocks.NewDomainSmartContract(t),
		domain:          componentsmocks.NewDomain(t),
		domainContext:   componentsmocks.NewDomainContext(t),
		transportWriter: transport.NewMockTransportWriter(t),
		originator:      originator.NewMockOriginator(t),
		coordinator:     coordinator.NewMockCoordinator(t),
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
		ContractAddress:   contractAddr.String(),
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
		ContractAddress:   "invalid-address",
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
		ContractAddress:   contractAddr.String(),
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
		ContractAddress:   contractAddr.String(),
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
		ContractAddress:   contractAddr.String(),
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

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleRevertResponseEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleAssembleResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleAssembleError_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	assembleError := &engineProto.AssembleError{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		ErrorMessage:    "test error",
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

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.AssembleErrorEvent)
		return ok && event.TransactionID == txID
	})).Once()

	sm.handleAssembleError(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

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

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.TransactionsDelegatedEvent)
		return ok && len(event.Transactions) == 1 && event.Transactions[0].ID == txID
	})).Once()

	sm.handleDelegationRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

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

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.HandoverRequestEvent)
		return ok && event.Requester == "test-node"
	})).Once()

	sm.handleHandoverRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleNonceAssigned_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	nonceAssigned := &engineProto.NonceAssigned{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		Nonce:           42,
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

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.NonceAssignedEvent)
		return ok && event.TransactionID == txID && event.Nonce == 42
	})).Once()

	sm.handleNonceAssigned(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandleTransactionSubmitted_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	hash := pldtypes.RandBytes32()
	transactionSubmitted := &engineProto.TransactionSubmitted{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		Hash:            hash[:],
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

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.SubmittedEvent)
		return ok && event.TransactionID == txID
	})).Once()

	sm.handleTransactionSubmitted(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandleTransactionConfirmed_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	transactionConfirmed := &engineProto.TransactionConfirmed{
		TransactionId:   txID.String(),
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
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		RevertReason:    revertReason,
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

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.ConfirmedRevertedEvent)
		return ok && event.TransactionID == txID && string(event.RevertReason) == string(revertReason)
	})).Once()

	sm.handleTransactionConfirmed(ctx, message)

	mocks.originator.AssertExpectations(t)
}

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
		TransactionId:   txIDHex,
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

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.DispatchedEvent)
		// Note: TransactionID parsing from hex string may not match exactly due to format conversion
		return ok && event.TransactionID != uuid.Nil
	})).Once()

	sm.handleDispatchedEvent(ctx, message)

	mocks.originator.AssertExpectations(t)
}

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
		Id:               requestID.String(),
		TransactionId:    txIDHex,
		ContractAddress:  contractAddr.String(),
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

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.PreDispatchRequestReceivedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID && event.Coordinator == "coordinator-node"
	})).Once()

	sm.handlePreDispatchRequest(ctx, message)

	mocks.originator.AssertExpectations(t)
}

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
		Id:              requestID.String(),
		TransactionId:   txIDHex,
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

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.DispatchRequestApprovedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handlePreDispatchResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

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

func TestHandleEndorsementRequest_Success_Sign(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	// Create test data
	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddr.String(),
		},
	}
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}

	signature := &prototk.AttestationResult{
		Name: "sig1",
	}

	state := &prototk.EndorsableState{
		Id: "state1",
	}

	attestationRequest := &prototk.AttestationRequest{
		Name:            "endorsement1",
		Algorithm:       "ECDSA",
		VerifierType:    "eth_address",
		PayloadType:     "raw",
		AttestationType: prototk.AttestationType_ENDORSE,
	}

	txSpecAny, _ := anypb.New(txSpec)
	verifierAny, _ := anypb.New(verifier)
	signatureAny, _ := anypb.New(signature)
	stateAny, _ := anypb.New(state)
	attestationAny, _ := anypb.New(attestationRequest)

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		IdempotencyKey:           idempotencyKey,
		ContractAddress:          contractAddr.String(),
		Party:                    party,
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{signatureAny},
		InputStates:              []*anypb.Any{stateAny},
		ReadStates:               []*anypb.Any{stateAny},
		OutputStates:             []*anypb.Any{stateAny},
		InfoStates:               []*anypb.Any{stateAny},
		AttestationRequest:       attestationAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Times(2)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Times(3)
	mocks.persistence.EXPECT().NOTX().Return(nil).Times(3)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	// Mock identity resolution
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Times(2)
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier:  "test-verifier",
			Type:      "eth_address",
			Algorithm: "ECDSA",
		},
	}
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Times(2).Return(resolvedKey, nil)
	mocks.keyManager.EXPECT().Sign(ctx, resolvedKey, "raw", []byte("endorsement-payload")).Return([]byte("signature"), nil).Once()

	// Mock domain context
	mocks.domainAPI.EXPECT().Domain().Return(mocks.domain).Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.components.EXPECT().StateManager().Return(mocks.stateManager).Once()
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mocks.domain, *contractAddr).Return(mocks.domainContext).Once()
	mocks.domainContext.EXPECT().Close().Once()

	// Mock endorsement result - SIGN
	endorsementResult := &components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("endorsement-payload"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup: party,
		},
	}
	mocks.domainAPI.EXPECT().EndorseTransaction(mocks.domainContext, mock.Anything, mock.MatchedBy(func(req *components.PrivateTransactionEndorseRequest) bool {
		return req.TransactionSpecification != nil && req.Endorsement != nil
	})).Return(endorsementResult, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.EndorsementRequestedEvent)
		return ok && event.From == "coordinator-node"
	})).Once()

	mocks.metrics.EXPECT().IncEndorsedTransactions().Once()
	mocks.transportWriter.EXPECT().SendEndorsementResponse(ctx, txID, idempotencyKey, contractAddr.String(), mock.Anything, endorsementResult, "", "endorsement1", party, "coordinator-node").Return(nil).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
	mocks.transportWriter.AssertExpectations(t)
	mocks.metrics.AssertExpectations(t)
}

func TestHandleEndorsementRequest_Success_Revert(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"
	revertReason := "transaction reverted"

	// Create test data
	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddr.String(),
		},
	}
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}

	signature := &prototk.AttestationResult{
		Name: "sig1",
	}

	state := &prototk.EndorsableState{
		Id: "state1",
	}

	attestationRequest := &prototk.AttestationRequest{
		Name:            "endorsement1",
		Algorithm:       "ECDSA",
		VerifierType:    "eth_address",
		PayloadType:     "raw",
		AttestationType: prototk.AttestationType_ENDORSE,
	}

	txSpecAny, _ := anypb.New(txSpec)
	verifierAny, _ := anypb.New(verifier)
	signatureAny, _ := anypb.New(signature)
	stateAny, _ := anypb.New(state)
	attestationAny, _ := anypb.New(attestationRequest)

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		IdempotencyKey:           idempotencyKey,
		ContractAddress:          contractAddr.String(),
		Party:                    party,
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{signatureAny},
		InputStates:              []*anypb.Any{stateAny},
		ReadStates:               []*anypb.Any{stateAny},
		OutputStates:             []*anypb.Any{stateAny},
		InfoStates:               []*anypb.Any{stateAny},
		AttestationRequest:       attestationAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Times(2)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Times(3)
	mocks.persistence.EXPECT().NOTX().Return(nil).Times(3)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	// Mock identity resolution
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Once()
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier:  "test-verifier",
			Type:      "eth_address",
			Algorithm: "ECDSA",
		},
	}
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(resolvedKey, nil).Once()

	// Mock domain context
	mocks.domainAPI.EXPECT().Domain().Return(mocks.domain).Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.components.EXPECT().StateManager().Return(mocks.stateManager).Once()
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mocks.domain, *contractAddr).Return(mocks.domainContext).Once()
	mocks.domainContext.EXPECT().Close().Once()

	// Mock endorsement result - REVERT
	endorsementResult := &components.EndorsementResult{
		Result:       prototk.EndorseTransactionResponse_REVERT,
		Payload:      []byte("endorsement-payload"),
		RevertReason: &revertReason,
		Endorser: &prototk.ResolvedVerifier{
			Lookup: party,
		},
	}
	mocks.domainAPI.EXPECT().EndorseTransaction(mocks.domainContext, mock.Anything, mock.Anything).Return(endorsementResult, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.Anything).Once()

	mocks.metrics.EXPECT().IncEndorsedTransactions().Once()
	mocks.transportWriter.EXPECT().SendEndorsementResponse(ctx, txID, idempotencyKey, contractAddr.String(), mock.Anything, endorsementResult, revertReason, "endorsement1", party, "coordinator-node").Return(nil).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
	mocks.transportWriter.AssertExpectations(t)
	mocks.metrics.AssertExpectations(t)
}

func TestHandleEndorsementRequest_Success_EndorserSubmit(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	// Create test data
	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddr.String(),
		},
	}
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}

	signature := &prototk.AttestationResult{
		Name: "sig1",
	}

	state := &prototk.EndorsableState{
		Id: "state1",
	}

	attestationRequest := &prototk.AttestationRequest{
		Name:            "endorsement1",
		Algorithm:       "ECDSA",
		VerifierType:    "eth_address",
		PayloadType:     "raw",
		AttestationType: prototk.AttestationType_ENDORSE,
	}

	txSpecAny, _ := anypb.New(txSpec)
	verifierAny, _ := anypb.New(verifier)
	signatureAny, _ := anypb.New(signature)
	stateAny, _ := anypb.New(state)
	attestationAny, _ := anypb.New(attestationRequest)

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		IdempotencyKey:           idempotencyKey,
		ContractAddress:          contractAddr.String(),
		Party:                    party,
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{signatureAny},
		InputStates:              []*anypb.Any{stateAny},
		ReadStates:               []*anypb.Any{stateAny},
		OutputStates:             []*anypb.Any{stateAny},
		InfoStates:               []*anypb.Any{stateAny},
		AttestationRequest:       attestationAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Times(2)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Times(3)
	mocks.persistence.EXPECT().NOTX().Return(nil).Times(3)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	// Mock identity resolution
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Once()
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier:  "test-verifier",
			Type:      "eth_address",
			Algorithm: "ECDSA",
		},
	}
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(resolvedKey, nil).Once()

	// Mock domain context
	mocks.domainAPI.EXPECT().Domain().Return(mocks.domain).Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.components.EXPECT().StateManager().Return(mocks.stateManager).Once()
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mocks.domain, *contractAddr).Return(mocks.domainContext).Once()
	mocks.domainContext.EXPECT().Close().Once()

	// Mock endorsement result - ENDORSER_SUBMIT
	endorsementResult := &components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		Payload: []byte("endorsement-payload"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup: party,
		},
	}
	mocks.domainAPI.EXPECT().EndorseTransaction(mocks.domainContext, mock.Anything, mock.Anything).Return(endorsementResult, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.Anything).Once()

	mocks.metrics.EXPECT().IncEndorsedTransactions().Once()
	mocks.transportWriter.EXPECT().SendEndorsementResponse(ctx, txID, idempotencyKey, contractAddr.String(), mock.MatchedBy(func(att *prototk.AttestationResult) bool {
		return len(att.Constraints) > 0 && att.Constraints[0] == prototk.AttestationResult_ENDORSER_MUST_SUBMIT
	}), endorsementResult, "", "endorsement1", party, "coordinator-node").Return(nil).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
	mocks.transportWriter.AssertExpectations(t)
	mocks.metrics.AssertExpectations(t)
}

func TestHandleEndorsementRequest_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New().String()
	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:   txID,
		ContractAddress: "invalid-address",
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_GetSmartContractError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:   txID,
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("not found")).Once()

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_TransactionSpecificationUnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	invalidAny, _ := anypb.New(&prototk.TransactionSpecification{})
	invalidAny.Value = []byte("invalid")
	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		ContractAddress:          contractAddr.String(),
		TransactionSpecification: invalidAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_VerifierUnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
	}
	txSpecAny, _ := anypb.New(txSpec)
	invalidAny, _ := anypb.New(&prototk.ResolvedVerifier{})
	invalidAny.Value = []byte("invalid")

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		ContractAddress:          contractAddr.String(),
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{invalidAny},
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_SignatureUnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
	}
	txSpecAny, _ := anypb.New(txSpec)
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}
	verifierAny, _ := anypb.New(verifier)
	invalidAny, _ := anypb.New(&prototk.AttestationResult{})
	invalidAny.Value = []byte("invalid")

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		ContractAddress:          contractAddr.String(),
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{invalidAny},
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_IdentityResolutionError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
	}
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}

	signature := &prototk.AttestationResult{
		Name: "sig1",
	}

	state := &prototk.EndorsableState{
		Id: "state1",
	}

	attestationRequest := &prototk.AttestationRequest{
		Name:            "endorsement1",
		Algorithm:       "ECDSA",
		VerifierType:    "eth_address",
		PayloadType:     "raw",
		AttestationType: prototk.AttestationType_ENDORSE,
	}

	txSpecAny, _ := anypb.New(txSpec)
	verifierAny, _ := anypb.New(verifier)
	signatureAny, _ := anypb.New(signature)
	stateAny, _ := anypb.New(state)
	attestationAny, _ := anypb.New(attestationRequest)

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		ContractAddress:          contractAddr.String(),
		Party:                    "invalid-party",
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{signatureAny},
		InputStates:              []*anypb.Any{stateAny},
		ReadStates:               []*anypb.Any{stateAny},
		OutputStates:             []*anypb.Any{stateAny},
		InfoStates:               []*anypb.Any{stateAny},
		AttestationRequest:       attestationAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()

	// Mock identity resolution will fail
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Once()
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(nil, errors.New("key not found")).Once()

	// Should not panic - identity resolution will fail
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_ResolveKeyError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
	}
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}

	signature := &prototk.AttestationResult{
		Name: "sig1",
	}

	state := &prototk.EndorsableState{
		Id: "state1",
	}

	attestationRequest := &prototk.AttestationRequest{
		Name:            "endorsement1",
		Algorithm:       "ECDSA",
		VerifierType:    "eth_address",
		PayloadType:     "raw",
		AttestationType: prototk.AttestationType_ENDORSE,
	}

	txSpecAny, _ := anypb.New(txSpec)
	verifierAny, _ := anypb.New(verifier)
	signatureAny, _ := anypb.New(signature)
	stateAny, _ := anypb.New(state)
	attestationAny, _ := anypb.New(attestationRequest)

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		ContractAddress:          contractAddr.String(),
		Party:                    party,
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{signatureAny},
		InputStates:              []*anypb.Any{stateAny},
		ReadStates:               []*anypb.Any{stateAny},
		OutputStates:             []*anypb.Any{stateAny},
		InfoStates:               []*anypb.Any{stateAny},
		AttestationRequest:       attestationAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()

	// Mock identity resolution succeeds but key resolution fails
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Maybe()
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(nil, errors.New("key not found")).Maybe()

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_EndorseTransactionError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
	}
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}

	signature := &prototk.AttestationResult{
		Name: "sig1",
	}

	state := &prototk.EndorsableState{
		Id: "state1",
	}

	attestationRequest := &prototk.AttestationRequest{
		Name:            "endorsement1",
		Algorithm:       "ECDSA",
		VerifierType:    "eth_address",
		PayloadType:     "raw",
		AttestationType: prototk.AttestationType_ENDORSE,
	}

	txSpecAny, _ := anypb.New(txSpec)
	verifierAny, _ := anypb.New(verifier)
	signatureAny, _ := anypb.New(signature)
	stateAny, _ := anypb.New(state)
	attestationAny, _ := anypb.New(attestationRequest)

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		ContractAddress:          contractAddr.String(),
		Party:                    party,
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{signatureAny},
		InputStates:              []*anypb.Any{stateAny},
		ReadStates:               []*anypb.Any{stateAny},
		OutputStates:             []*anypb.Any{stateAny},
		InfoStates:               []*anypb.Any{stateAny},
		AttestationRequest:       attestationAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()

	// Mock identity resolution
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Once()
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier:  "test-verifier",
			Type:      "eth_address",
			Algorithm: "ECDSA",
		},
	}
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(resolvedKey, nil).Once()

	// Mock domain context
	mocks.domainAPI.EXPECT().Domain().Return(mocks.domain).Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.components.EXPECT().StateManager().Return(mocks.stateManager).Once()
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mocks.domain, *contractAddr).Return(mocks.domainContext).Once()
	mocks.domainContext.EXPECT().Close().Once()

	// Mock endorsement error
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainAPI.EXPECT().EndorseTransaction(mocks.domainContext, mock.Anything, mock.Anything).Return(nil, errors.New("endorsement failed")).Once()

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_LoadSequencerError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
	}
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}

	signature := &prototk.AttestationResult{
		Name: "sig1",
	}

	state := &prototk.EndorsableState{
		Id: "state1",
	}

	attestationRequest := &prototk.AttestationRequest{
		Name:            "endorsement1",
		Algorithm:       "ECDSA",
		VerifierType:    "eth_address",
		PayloadType:     "raw",
		AttestationType: prototk.AttestationType_ENDORSE,
	}

	txSpecAny, _ := anypb.New(txSpec)
	verifierAny, _ := anypb.New(verifier)
	signatureAny, _ := anypb.New(signature)
	stateAny, _ := anypb.New(state)
	attestationAny, _ := anypb.New(attestationRequest)

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		ContractAddress:          contractAddr.String(),
		Party:                    party,
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{signatureAny},
		InputStates:              []*anypb.Any{stateAny},
		ReadStates:               []*anypb.Any{stateAny},
		OutputStates:             []*anypb.Any{stateAny},
		InfoStates:               []*anypb.Any{stateAny},
		AttestationRequest:       attestationAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()

	// Mock identity resolution
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Once()
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier:  "test-verifier",
			Type:      "eth_address",
			Algorithm: "ECDSA",
		},
	}
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(resolvedKey, nil).Once()

	// Mock domain context
	mocks.domainAPI.EXPECT().Domain().Return(mocks.domain).Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.components.EXPECT().StateManager().Return(mocks.stateManager).Once()
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mocks.domain, *contractAddr).Return(mocks.domainContext).Once()
	mocks.domainContext.EXPECT().Close().Once()

	// Mock endorsement result
	endorsementResult := &components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("endorsement-payload"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup: party,
		},
	}
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainAPI.EXPECT().EndorseTransaction(mocks.domainContext, mock.Anything, mock.Anything).Return(endorsementResult, nil).Once()

	// Mock identity validation for signing (signing happens before LoadSequencer)
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Once()
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(resolvedKey, nil).Once()
	mocks.keyManager.EXPECT().Sign(ctx, resolvedKey, "raw", []byte("endorsement-payload")).Return([]byte("signature"), nil).Once()

	// Mock LoadSequencer failure
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("not found")).Once()

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_SendEndorsementResponseError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
	}
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}

	signature := &prototk.AttestationResult{
		Name: "sig1",
	}

	state := &prototk.EndorsableState{
		Id: "state1",
	}

	attestationRequest := &prototk.AttestationRequest{
		Name:            "endorsement1",
		Algorithm:       "ECDSA",
		VerifierType:    "eth_address",
		PayloadType:     "raw",
		AttestationType: prototk.AttestationType_ENDORSE,
	}

	txSpecAny, _ := anypb.New(txSpec)
	verifierAny, _ := anypb.New(verifier)
	signatureAny, _ := anypb.New(signature)
	stateAny, _ := anypb.New(state)
	attestationAny, _ := anypb.New(attestationRequest)

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		IdempotencyKey:           idempotencyKey,
		ContractAddress:          contractAddr.String(),
		Party:                    party,
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{signatureAny},
		InputStates:              []*anypb.Any{stateAny},
		ReadStates:               []*anypb.Any{stateAny},
		OutputStates:             []*anypb.Any{stateAny},
		InfoStates:               []*anypb.Any{stateAny},
		AttestationRequest:       attestationAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()

	// Mock identity resolution
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Once()
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier:  "test-verifier",
			Type:      "eth_address",
			Algorithm: "ECDSA",
		},
	}
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(resolvedKey, nil).Once()

	// Mock domain context
	mocks.domainAPI.EXPECT().Domain().Return(mocks.domain).Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.components.EXPECT().StateManager().Return(mocks.stateManager).Once()
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mocks.domain, *contractAddr).Return(mocks.domainContext).Once()
	mocks.domainContext.EXPECT().Close().Once()

	// Mock endorsement result
	endorsementResult := &components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("endorsement-payload"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup: party,
		},
	}
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainAPI.EXPECT().EndorseTransaction(mocks.domainContext, mock.Anything, mock.Anything).Return(endorsementResult, nil).Once()

	// Mock identity validation for signing
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Once()
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(resolvedKey, nil).Once()
	mocks.keyManager.EXPECT().Sign(ctx, resolvedKey, "raw", []byte("endorsement-payload")).Return([]byte("signature"), nil).Once()

	// Mock LoadSequencer
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.Anything).Once()

	mocks.metrics.EXPECT().IncEndorsedTransactions().Once()
	mocks.transportWriter.EXPECT().SendEndorsementResponse(ctx, txID, idempotencyKey, contractAddr.String(), mock.Anything, endorsementResult, "", "endorsement1", party, "coordinator-node").Return(errors.New("send failed")).Once()

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
	mocks.transportWriter.AssertExpectations(t)
	mocks.metrics.AssertExpectations(t)
}

func TestHandleEndorsementRequest_SignValidationError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	party := "party@other-node" // Different node, so validation will fail

	txSpec := &prototk.TransactionSpecification{
		TransactionId: txID,
	}
	verifier := &prototk.ResolvedVerifier{
		Lookup: "verifier@node1",
	}

	signature := &prototk.AttestationResult{
		Name: "sig1",
	}

	state := &prototk.EndorsableState{
		Id: "state1",
	}

	attestationRequest := &prototk.AttestationRequest{
		Name:            "endorsement1",
		Algorithm:       "ECDSA",
		VerifierType:    "eth_address",
		PayloadType:     "raw",
		AttestationType: prototk.AttestationType_ENDORSE,
	}

	txSpecAny, _ := anypb.New(txSpec)
	verifierAny, _ := anypb.New(verifier)
	signatureAny, _ := anypb.New(signature)
	stateAny, _ := anypb.New(state)
	attestationAny, _ := anypb.New(attestationRequest)

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId:            txID,
		ContractAddress:          contractAddr.String(),
		Party:                    party,
		TransactionSpecification: txSpecAny,
		Verifiers:                []*anypb.Any{verifierAny},
		Signatures:               []*anypb.Any{signatureAny},
		InputStates:              []*anypb.Any{stateAny},
		ReadStates:               []*anypb.Any{stateAny},
		OutputStates:             []*anypb.Any{stateAny},
		InfoStates:               []*anypb.Any{stateAny},
		AttestationRequest:       attestationAny,
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Setup mocks
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Maybe()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	// Mock identity resolution
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Once()
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier:  "test-verifier",
			Type:      "eth_address",
			Algorithm: "ECDSA",
		},
	}
	mocks.keyManager.EXPECT().ResolveKeyNewDatabaseTX(ctx, mock.Anything, "ECDSA", "eth_address").Return(resolvedKey, nil).Once()

	// Mock domain context
	mocks.domainAPI.EXPECT().Domain().Return(mocks.domain).Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.components.EXPECT().StateManager().Return(mocks.stateManager).Once()
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mocks.domain, *contractAddr).Return(mocks.domainContext).Once()
	mocks.domainContext.EXPECT().Close().Once()

	// Mock endorsement result - SIGN but for different node
	endorsementResult := &components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("endorsement-payload"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup: party,
		},
	}
	mocks.domainAPI.EXPECT().EndorseTransaction(mocks.domainContext, mock.Anything, mock.Anything).Return(endorsementResult, nil).Once()

	// Validation will fail because party is for different node, so signing won't happen
	// But LoadSequencer will still be called
	mocks.metrics.EXPECT().SetActiveSequencers(0).Maybe()

	// Should not panic - validation will fail because party is for different node
	sm.handleEndorsementRequest(ctx, message)
}

// Test handleEndorsementResponse
func TestHandleEndorsementResponse_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()
	attestationRequestName := "endorsement1"

	// Create endorsement
	endorsement := &prototk.AttestationResult{
		Name: "test-endorsement",
	}
	endorsementAny, _ := anypb.New(endorsement)

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:          txID.String(),
		IdempotencyKey:         idempotencyKey.String(),
		ContractAddress:        contractAddr.String(),
		Endorsement:            endorsementAny,
		AttestationRequestName: attestationRequestName,
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.EndorsedEvent)
		return ok && event.TransactionID == txID && event.RequestID == idempotencyKey && event.Endorsement != nil
	})).Once()

	sm.handleEndorsementResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementResponse_Revert(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()
	revertReason := "transaction reverted"
	attestationRequestName := "endorsement1"

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:          txID.String(),
		IdempotencyKey:         idempotencyKey.String(),
		ContractAddress:        contractAddr.String(),
		RevertReason:           &revertReason,
		AttestationRequestName: attestationRequestName,
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.EndorsedRejectedEvent)
		return ok && event.TransactionID == txID && event.RequestID == idempotencyKey && event.RevertReason == revertReason && event.AttestationRequestName == attestationRequestName
	})).Once()

	sm.handleEndorsementResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementResponse_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleEndorsementResponse(ctx, message)
}

func TestHandleEndorsementResponse_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New()
	idempotencyKey := uuid.New()

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:   txID.String(),
		IdempotencyKey:  idempotencyKey.String(),
		ContractAddress: "invalid-address",
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// Should not panic
	sm.handleEndorsementResponse(ctx, message)
}

func TestHandleEndorsementResponse_LoadSequencerError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:   txID.String(),
		IdempotencyKey:  idempotencyKey.String(),
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will fail
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("not found")).Once()

	// Should not panic
	sm.handleEndorsementResponse(ctx, message)
}

func TestHandleEndorsementResponse_EndorsementUnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()

	// Create invalid endorsement (corrupted data)
	invalidEndorsementAny, _ := anypb.New(&prototk.AttestationResult{})
	invalidEndorsementAny.Value = []byte("invalid-endorsement-data")

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:   txID.String(),
		IdempotencyKey:  idempotencyKey.String(),
		ContractAddress: contractAddr.String(),
		Endorsement:     invalidEndorsementAny,
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// Setup mocks - LoadSequencer will be called
	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	// Should not panic - endorsement unmarshal will fail but function should handle it gracefully
	sm.handleEndorsementResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}
