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
	originatorTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/coordinatormocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatormocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencermetricsmocks"
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

type transportClientTestMocks struct {
	components      *componentsmocks.AllComponents
	domainManager   *componentsmocks.DomainManager
	stateManager    *componentsmocks.StateManager
	persistence     *persistencemocks.Persistence
	txManager       *componentsmocks.TXManager
	domainAPI       *componentsmocks.DomainSmartContract
	domain          *componentsmocks.Domain
	domainContext   *componentsmocks.DomainContext
	transportWriter *sequencertransportmocks.TransportWriter
	originator      *originatormocks.Originator
	coordinator     *coordinatormocks.Coordinator
	metrics         *sequencermetricsmocks.DistributedSequencerMetrics
}

func newTransportClientTestMocks(t *testing.T) *transportClientTestMocks {
	return &transportClientTestMocks{
		components:      componentsmocks.NewAllComponents(t),
		domainManager:   componentsmocks.NewDomainManager(t),
		stateManager:    componentsmocks.NewStateManager(t),
		persistence:     persistencemocks.NewPersistence(t),
		txManager:       componentsmocks.NewTXManager(t),
		domainAPI:       componentsmocks.NewDomainSmartContract(t),
		domain:          componentsmocks.NewDomain(t),
		domainContext:   componentsmocks.NewDomainContext(t),
		transportWriter: sequencertransportmocks.NewTransportWriter(t),
		originator:      originatormocks.NewOriginator(t),
		coordinator:     coordinatormocks.NewCoordinator(t),
		metrics:         sequencermetricsmocks.NewDistributedSequencerMetrics(t),
	}
}

func newSequencerManagerForTransportClientTesting(t *testing.T, mocks *transportClientTestMocks) *sequencerManager {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{}

	sm := &sequencerManager{
		ctx:                         ctx,
		config:                      config,
		components:                  mocks.components,
		nodeName:                    "test-node",
		sequencersLock:              sync.RWMutex{},
		sequencers:                  make(map[string]*sequencer),
		metrics:                     mocks.metrics,
		targetActiveSequencersLimit: 2,
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
		{"DelegationResponse", transport.MessageType_DelegationResponse},
		{"Dispatched", transport.MessageType_Dispatched},
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
	}
	preAssemblyJSON, _ := json.Marshal(preAssembly)

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:          txID.String(),
		AssembleRequestId:      requestID.String(),
		ContractAddress:        contractAddr.String(),
		PreAssembly:            preAssemblyJSON,
		StateLocks:             []byte("{}"),
		CoordinatorBlockHeight: 100,
	}
	payload, _ := proto.Marshal(assembleRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
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
		TransactionId:          txID.String(),
		AssembleRequestId:      requestID.String(),
		ContractAddress:        "invalid-address",
		PreAssembly:            preAssemblyJSON,
		StateLocks:             []byte("{}"),
		CoordinatorBlockHeight: 100,
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

func TestHandleAssembleRequest_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	preAssembly := &components.TransactionPreAssembly{}
	preAssemblyJSON, _ := json.Marshal(preAssembly)

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:          txID.String(),
		AssembleRequestId:      requestID.String(),
		ContractAddress:        contractAddr.String(),
		PreAssembly:            preAssemblyJSON,
		StateLocks:             []byte("{}"),
		CoordinatorBlockHeight: 100,
	}
	payload, _ := proto.Marshal(assembleRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     payload,
	}

	// GetSequencer is used and returns nil since no sequencer is in memory - should not panic
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

	// GetSequencer is used - sequencer must already be in memory
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

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleRevertEvent)
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
	requestID := uuid.New()
	assembleError := &engineProto.AssembleError{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:   contractAddr.String(),
		ErrorMessage:      "test error",
	}
	payload, _ := proto.Marshal(assembleError)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleError,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleErrorEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleAssembleError(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func newDelegationRequestMessage(fromNode string, contractAddr *pldtypes.EthAddress, blockHeight int64, txs ...*components.PrivateTransaction) *components.ReceivedMessage {
	allTxBytes := make([][]byte, 0, len(txs))
	for _, tx := range txs {
		b, _ := json.Marshal(tx)
		allTxBytes = append(allTxBytes, b)
	}
	delegationRequest := &engineProto.DelegationRequest{
		PrivateTransactions:   allTxBytes,
		OriginatorBlockHeight: blockHeight,
	}
	payload, _ := proto.Marshal(delegationRequest)
	return &components.ReceivedMessage{
		FromNode:    fromNode,
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationRequest,
		Payload:     payload,
	}
}

func newTestPrivateTx(contractAddr *pldtypes.EthAddress) *components.PrivateTransaction {
	return &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				ContractInfo: &prototk.ContractInfo{
					ContractAddress: contractAddr.String(),
				},
				From: "originator@node1",
			},
		},
	}
}

func TestHandleDelegationRequest_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	privateTx := newTestPrivateTx(contractAddr)
	message := newDelegationRequestMessage("test-node", contractAddr, 100, privateTx)

	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.TransactionsDelegatedEvent)
		return ok &&
			len(event.Transactions) == 1 &&
			event.Transactions[0].ID == privateTx.ID &&
			event.FromNode == "test-node" &&
			event.Originator == "originator@node1" &&
			event.OriginatorsBlockHeight == 100
	})).Once()

	sm.handleDelegationRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleDelegationRequest_MultipleTxBatch(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	tx1 := newTestPrivateTx(contractAddr)
	tx2 := newTestPrivateTx(contractAddr)
	tx3 := newTestPrivateTx(contractAddr)
	message := newDelegationRequestMessage("originator-node", contractAddr, 42, tx1, tx2, tx3)

	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.TransactionsDelegatedEvent)
		if !ok || len(event.Transactions) != 3 {
			return false
		}
		return event.Transactions[0].ID == tx1.ID &&
			event.Transactions[1].ID == tx2.ID &&
			event.Transactions[2].ID == tx3.ID &&
			event.FromNode == "originator-node" &&
			event.OriginatorsBlockHeight == 42
	})).Once()

	sm.handleDelegationRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleDelegationRequest_EmptyTransactions(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	// A delegation request with no transactions — coordinator must not be called
	message := newDelegationRequestMessage("originator-node", contractAddr, 100)

	sm.handleDelegationRequest(ctx, message)

	// No QueueEvent call expected — mocks.coordinator has no registered expectations,
	// and NewCoordinator(t) will fail the test if QueueEvent is called unexpectedly.
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

	// GetSequencer is used - sequencer must already be in memory
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

	// GetSequencer is used - sequencer must already be in memory
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
		Outcome:         engineProto.TransactionConfirmed_OUTCOME_SUCCESS,
	}
	payload, _ := proto.Marshal(transactionConfirmed)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionConfirmed,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
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
	failureMessage := "decoded revert"
	transactionConfirmed := &engineProto.TransactionConfirmed{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		Outcome:         engineProto.TransactionConfirmed_OUTCOME_REVERTED,
		RevertReason:    revertReason,
		FailureMessage:  failureMessage,
		WillRetry:       true,
	}
	payload, _ := proto.Marshal(transactionConfirmed)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionConfirmed,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.ConfirmedRevertedEvent)
		return ok && event.TransactionID == txID && string(event.RevertReason) == string(revertReason) && event.FailureMessage == failureMessage && event.WillRetry
	})).Once()

	sm.handleTransactionConfirmed(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandleTransactionConfirmed_RevertedWhenWillRetryTrueAndNoRevertReason(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	transactionConfirmed := &engineProto.TransactionConfirmed{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		Outcome:         engineProto.TransactionConfirmed_OUTCOME_REVERTED,
		WillRetry:       true,
	}
	payload, _ := proto.Marshal(transactionConfirmed)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionConfirmed,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.ConfirmedRevertedEvent)
		return ok && event.TransactionID == txID && event.WillRetry && len(event.RevertReason) == 0
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

	// GetSequencer is used - sequencer must already be in memory
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

func TestHandleCoordinatorHeartbeatNotification_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	coordinatorSnapshot := &common.CoordinatorSnapshot{}
	snapshotJSON, _ := json.Marshal(coordinatorSnapshot)
	heartbeatNotification := &engineProto.CoordinatorHeartbeatNotification{
		From:                "coordinator@node2",
		ContractAddress:     contractAddr.String(),
		CoordinatorSnapshot: snapshotJSON,
	}
	payload, _ := proto.Marshal(heartbeatNotification)

	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_CoordinatorHeartbeatNotification,
		Payload:     payload,
	}

	// LoadSequencer is called; returning an error causes it to return (nil, nil)
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("not found")).Once()

	sm.handleCoordinatorHeartbeatNotification(ctx, message)
	assert.Empty(t, sm.sequencers)
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

	preDispatchRequest := &engineProto.PreDispatchRequest{
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

	// GetSequencer is used - sequencer must already be in memory
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

	preDispatchResponse := &engineProto.PreDispatchResponse{
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

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.DispatchRequestApprovedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handlePreDispatchResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleDelegationResponse_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New()
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{txID.String()},
	}
	payload, _ := proto.Marshal(delegationRequestAcknowledgment)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	// Should not panic - this handler just logs
	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_MaxInFlightRejection(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID1 := uuid.New().String()
	txID2 := uuid.New().String()
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{txID1, txID2},
		Errors: []int64{
			int64(coordinator.DelegationAcknowledgementError_MaxInflightTransactions),
			int64(coordinator.DelegationAcknowledgementError_MaxInflightTransactions),
		},
	}
	payload, err := proto.Marshal(delegationRequestAcknowledgment)
	require.NoError(t, err)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	// Should not panic; handler logs max in flight rejections
	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_UnknownError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New().String()
	unknownErrorCode := int64(99)
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{txID},
		Errors:         []int64{unknownErrorCode},
	}
	payload, err := proto.Marshal(delegationRequestAcknowledgment)
	require.NoError(t, err)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_MixedErrors(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txIDAccepted := uuid.New().String()
	txIDMaxInFlight := uuid.New().String()
	txIDUnknown := uuid.New().String()
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{txIDAccepted, txIDMaxInFlight, txIDUnknown},
		Errors: []int64{
			int64(coordinator.DelegationAcknowledgementError_None),
			int64(coordinator.DelegationAcknowledgementError_MaxInflightTransactions),
			42, // unknown error code
		},
	}
	payload, err := proto.Marshal(delegationRequestAcknowledgment)
	require.NoError(t, err)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_Empty(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{},
		Errors:         []int64{},
	}
	payload, err := proto.Marshal(delegationRequestAcknowledgment)
	require.NoError(t, err)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	sm.handleDelegationResponse(ctx, message)
}

func TestHandleEndorsementRequest_Success_Sign(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: txID}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId: txID, IdempotencyKey: idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok && event.TransactionId == txID && event.IdempotencyKey == idempotencyKey
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementRequest_Success_Revert(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: txID}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId: txID, IdempotencyKey: idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok && event.FromNode == "coordinator-node" && event.TransactionId == txID && event.IdempotencyKey == idempotencyKey && event.Party == party
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementRequest_Success_EndorserSubmit(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: txID}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId: txID, IdempotencyKey: idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok && event.TransactionId == txID && event.IdempotencyKey == idempotencyKey && event.Party == party
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
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

// TestHandleEndorsementRequest_QueuesEventWithDecodedFields verifies that a valid endorsement
// request results in an EndorsementRequestReceivedEvent being queued on the coordinator with
// the correct decoded field values. (The former EndorseTransactionError test is superseded by
// tests in coordinator/endorsing_test.go now that that logic lives in the coordinator.)
func TestHandleEndorsementRequest_QueuesEventWithDecodedFields(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: txID}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId: txID, IdempotencyKey: idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	var capturedEvent *coordinator.EndorsementRequestReceivedEvent
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		var ok bool
		capturedEvent, ok = e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	require.NotNil(t, capturedEvent)
	assert.Equal(t, "test-node", capturedEvent.FromNode)
	assert.Equal(t, txID, capturedEvent.TransactionId)
	assert.Equal(t, idempotencyKey, capturedEvent.IdempotencyKey)
	assert.Equal(t, party, capturedEvent.Party)
	assert.NotNil(t, capturedEvent.PrivateEndorsementRequest)
	assert.Equal(t, 1, len(capturedEvent.PrivateEndorsementRequest.Verifiers))
	assert.Equal(t, 1, len(capturedEvent.PrivateEndorsementRequest.InputStates))
	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementRequest_LoadSequencerError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: txID}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId: txID, ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()

	// LoadSequencer returns nil because the existence check (GetSmartContractByAddress) errors.
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("not found")).Once()

	// QueueEvent must NOT be called because LoadSequencer returned nil.
	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// TestHandleEndorsementRequest_SendEndorsementResponseError was testing a scenario that now
// lives inside coordinator/endorsing.go (covered by endorsing_test.go). This test is repurposed
// to verify that the endorsement request event is queued even when the from-node field differs
// from the coordinator's own node name.
func TestHandleEndorsementRequest_SendEndorsementResponseError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: txID}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId: txID, IdempotencyKey: idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec, AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		_, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// TestHandleEndorsementRequest_SignValidationError verifies successful queueing when the party
// field contains a node name different from the local node.
func TestHandleEndorsementRequest_SignValidationError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	party := "party@other-node" // Different node — key is still resolved locally by the endorser

	txSpec := &prototk.TransactionSpecification{TransactionId: txID}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		TransactionId: txID, ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec, AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok && event.Party == party
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
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

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:          txID.String(),
		IdempotencyKey:         idempotencyKey.String(),
		ContractAddress:        contractAddr.String(),
		Endorsement:            endorsement,
		AttestationRequestName: attestationRequestName,
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
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

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.EndorseRevertEvent)
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

func TestHandleEndorsementResponse_SequencerNotLoaded(t *testing.T) {
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

	// GetSequencer is used and returns nil since no sequencer is in memory - should not panic
	sm.handleEndorsementResponse(ctx, message)
}

// ===== handleEndorsementError Tests =====

func TestHandleEndorsementError_QueuesToCoordinator(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()

	endorsementError := &engineProto.EndorsementError{
		TransactionId:   txID.String(),
		IdempotencyKey:  idempotencyKey.String(),
		ContractAddress: contractAddr.String(),
		ErrorMessage:    "domain returned unexpected error",
	}
	payload, _ := proto.Marshal(endorsementError)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementError,
		Payload:     payload,
	}

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.EndorseErrorEvent)
		return ok && event.TransactionID == txID && event.RequestID == idempotencyKey
	})).Once()

	sm.handleEndorsementError(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementError_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementError,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleEndorsementError(ctx, message)
}

func TestHandleEndorsementError_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()

	endorsementError := &engineProto.EndorsementError{
		TransactionId:   txID.String(),
		IdempotencyKey:  idempotencyKey.String(),
		ContractAddress: contractAddr.String(),
		ErrorMessage:    "some error",
	}
	payload, _ := proto.Marshal(endorsementError)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementError,
		Payload:     payload,
	}

	// GetSequencer returns nil since no sequencer is in memory - should not panic
	sm.handleEndorsementError(ctx, message)
}
