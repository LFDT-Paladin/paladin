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

package sequencer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	coordTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	originatorTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func (sMgr *sequencerManager) HandlePaladinMsg(ctx context.Context, message *components.ReceivedMessage) {
	//TODO this need to become an ultra low latency, non blocking, handover to the event loop thread.
	// need some thought on how to handle errors, retries, buffering, swapping idle sequencers in and out of memory etc...

	log.L(ctx).Debugf("%+v received from %s", message.MessageType, message.FromNode)

	//Send the event to the sequencer handler
	switch message.MessageType {
	case transport.MessageType_AssembleRequest:
		go sMgr.handleAssembleRequest(sMgr.ctx, message)
	case transport.MessageType_AssembleResponse:
		go sMgr.handleAssembleResponse(sMgr.ctx, message)
	case transport.MessageType_AssembleError:
		go sMgr.handleAssembleError(sMgr.ctx, message)
	case transport.MessageType_AssembleRejection:
		go sMgr.handleAssembleRejection(sMgr.ctx, message)
	case transport.MessageType_CoordinatorHeartbeatNotification:
		go sMgr.handleCoordinatorHeartbeatNotification(sMgr.ctx, message)
	case transport.MessageType_DelegationRequest:
		go sMgr.handleDelegationRequest(sMgr.ctx, message)
	case transport.MessageType_DelegationResponse:
		go sMgr.handleDelegationResponse(sMgr.ctx, message)
	case transport.MessageType_DelegationRejection:
		go sMgr.handleDelegationRejection(sMgr.ctx, message)
	case transport.MessageType_Dispatched:
		go sMgr.handleDispatchedEvent(sMgr.ctx, message)
	case transport.MessageType_PreDispatchRequest:
		go sMgr.handlePreDispatchRequest(sMgr.ctx, message)
	case transport.MessageType_PreDispatchResponse:
		go sMgr.handlePreDispatchResponse(sMgr.ctx, message)
	case transport.MessageType_EndorsementRequest:
		go sMgr.handleEndorsementRequest(sMgr.ctx, message)
	case transport.MessageType_EndorsementResponse:
		go sMgr.handleEndorsementResponse(sMgr.ctx, message)
	case transport.MessageType_EndorsementError:
		go sMgr.handleEndorsementError(sMgr.ctx, message)
	case transport.MessageType_EndorsementRejection:
		go sMgr.handleEndorsementRejection(sMgr.ctx, message)
	case transport.MessageType_NonceAssigned:
		go sMgr.handleNonceAssigned(sMgr.ctx, message)
	case transport.MessageType_TransactionSubmitted:
		go sMgr.handleTransactionSubmitted(sMgr.ctx, message)
	case transport.MessageType_TransactionConfirmed:
		go sMgr.handleTransactionConfirmed(sMgr.ctx, message)
	case transport.MessageType_PreDispatchRejection:
		go sMgr.handlePreDispatchRejection(sMgr.ctx, message)
	case transport.MessageType_HandoverRequest:
		go sMgr.handleHandoverRequest(sMgr.ctx, message)
	default:
		log.L(ctx).Errorf("Unknown message type: %s", message.MessageType)
	}
}

func (sMgr *sequencerManager) logPaladinMessageUnmarshalError(ctx context.Context, message *components.ReceivedMessage, err error) {
	log.L(ctx).Errorf("<< ERROR unmarshalling proto message%s from %s: %s", message.MessageType, message.FromNode, err)
}

func (sMgr *sequencerManager) logPaladinMessageFieldMissingError(ctx context.Context, message *components.ReceivedMessage, field string) {
	log.L(ctx).Errorf("<< field %s missing from proto message %s received from %s", field, message.MessageType, message.FromNode)
}

func (sMgr *sequencerManager) logPaladinMessageJsonUnmarshalError(ctx context.Context, jsonObject string, message *components.ReceivedMessage, err error) {
	log.L(ctx).Errorf("<< ERROR unmarshalling JSON object %s from proto message %s (received from %s): %s", jsonObject, message.MessageType, message.FromNode, err)
}

func (sMgr *sequencerManager) parseContractAddressString(ctx context.Context, contractAddressString string, message *components.ReceivedMessage) *pldtypes.EthAddress {
	contractAddress, err := pldtypes.ParseEthAddress(contractAddressString)
	if err != nil {
		log.L(ctx).Errorf("<< ERROR unmarshalling contract address from proto message %s (received from %s): %s", message.MessageType, message.FromNode, err)
		return nil
	}
	return contractAddress
}

func (sMgr *sequencerManager) handleAssembleRequest(ctx context.Context, message *components.ReceivedMessage) {

	assembleRequest := &engineProto.AssembleRequest{}
	err := proto.Unmarshal(message.Payload, assembleRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	preAssembly := &components.TransactionPreAssembly{}
	err = json.Unmarshal(assembleRequest.PreAssembly, preAssembly)
	if err != nil {
		sMgr.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPreAssembly", message, err)
		return
	}
	log.L(ctx).Infof("handling assemble request with %d required verifiers", len(preAssembly.RequiredVerifiers))

	contractAddress := sMgr.parseContractAddressString(ctx, assembleRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the assemble request
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: assemble request for transaction %s cannot be processed unless already in memory",
			contractAddress, assembleRequest.TransactionId)
		return
	}

	assembleRequestEvent := &originatorTransaction.AssembleRequestReceivedEvent{}
	assembleRequestEvent.TransactionID = uuid.MustParse(assembleRequest.TransactionId)
	assembleRequestEvent.RequestID = uuid.MustParse(assembleRequest.AssembleRequestId)
	assembleRequestEvent.Coordinator = message.FromNode
	assembleRequestEvent.CoordinatorBlockHeight = assembleRequest.CoordinatorBlockHeight
	assembleRequestEvent.BlockHeightTolerance = assembleRequest.BlockHeightTolerance
	assembleRequestEvent.StateLocksJSON = assembleRequest.StateLocks
	assembleRequestEvent.PreAssembly = assembleRequest.PreAssembly
	assembleRequestEvent.EventTime = time.Now()
	if assembleRequest.ExpiryTimeUnixMs != 0 {
		assembleRequestEvent.Expiry = time.UnixMilli(assembleRequest.ExpiryTimeUnixMs)
	}

	seq.GetOriginator().QueueEvent(ctx, assembleRequestEvent)
}

func (sMgr *sequencerManager) handleAssembleResponse(ctx context.Context, message *components.ReceivedMessage) {
	assembleResponse := &engineProto.AssembleResponse{}

	err := proto.Unmarshal(message.Payload, assembleResponse)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, assembleResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	postAssembly := &components.TransactionPostAssembly{}
	err = json.Unmarshal(assembleResponse.PostAssembly, postAssembly)
	if err != nil {
		sMgr.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPostAssembly", message, err)
		return
	}

	preAssembly := &components.TransactionPreAssembly{}
	err = json.Unmarshal(assembleResponse.PreAssembly, preAssembly)
	if err != nil {
		sMgr.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPreAssembly", message, err)
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the assembly response
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: assemble response for transaction %s cannot be processed unless already in memory",
			contractAddress, assembleResponse.TransactionId)
		return
	}

	switch postAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
		assembleResponseEvent := &coordTransaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = uuid.MustParse(assembleResponse.TransactionId)
		assembleResponseEvent.RequestID = uuid.MustParse(assembleResponse.AssembleRequestId)
		assembleResponseEvent.PostAssembly = postAssembly
		assembleResponseEvent.EventTime = time.Now()
		seq.GetCoordinator().QueueEvent(ctx, assembleResponseEvent)
	case prototk.AssembleTransactionResponse_PARK:
		log.L(ctx).Errorf("coordinator state machine cannot move from Assembling to Parked")
	case prototk.AssembleTransactionResponse_REVERT:
		assembleResponseEvent := &coordTransaction.AssembleRevertEvent{}
		assembleResponseEvent.TransactionID = uuid.MustParse(assembleResponse.TransactionId)
		assembleResponseEvent.RequestID = uuid.MustParse(assembleResponse.AssembleRequestId)
		assembleResponseEvent.PostAssembly = postAssembly
		assembleResponseEvent.EventTime = time.Now()
		seq.GetCoordinator().QueueEvent(ctx, assembleResponseEvent)
	default:
		log.L(ctx).Errorf("received unexpected assemble response type %s", postAssembly.AssemblyResult)
	}
}

func (sMgr *sequencerManager) handleAssembleError(ctx context.Context, message *components.ReceivedMessage) {
	assembleError := &engineProto.AssembleError{}

	err := proto.Unmarshal(message.Payload, assembleError)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, assembleError.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: assemble error for transaction %s cannot be processed unless already in memory",
			contractAddress, assembleError.TransactionId)
		return
	}

	assembleErrorEvent := &coordTransaction.AssembleErrorEvent{}
	assembleErrorEvent.RequestID = uuid.MustParse(assembleError.AssembleRequestId)
	assembleErrorEvent.TransactionID = uuid.MustParse(assembleError.TransactionId)
	assembleErrorEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, assembleErrorEvent)
}

func (sMgr *sequencerManager) handleAssembleRejection(ctx context.Context, message *components.ReceivedMessage) {
	assembleRejection := &engineProto.AssembleRejection{}

	err := proto.Unmarshal(message.Payload, assembleRejection)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, assembleRejection.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: assemble rejection for transaction %s cannot be processed unless already in memory",
			contractAddress, assembleRejection.TransactionId)
		return
	}

	assembleRejectedEvent := &coordTransaction.AssembleRequestRejectedEvent{}
	assembleRejectedEvent.RequestID = uuid.MustParse(assembleRejection.AssembleRequestId)
	assembleRejectedEvent.TransactionID = uuid.MustParse(assembleRejection.TransactionId)
	assembleRejectedEvent.EventTime = time.Now()
	assembleRejectedEvent.RejectionReason = assembleRejection.RejectionReason
	assembleRejectedEvent.CoordinatorBlockHeight = assembleRejection.CoordinatorBlockHeight
	assembleRejectedEvent.AssemblerBlockHeight = assembleRejection.AssemblerBlockHeight
	seq.GetCoordinator().QueueEvent(ctx, assembleRejectedEvent)
}

func (sMgr *sequencerManager) handleCoordinatorHeartbeatNotification(ctx context.Context, message *components.ReceivedMessage) {
	heartbeatNotification := &engineProto.CoordinatorHeartbeatNotification{}
	err := proto.Unmarshal(message.Payload, heartbeatNotification)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	from := heartbeatNotification.From
	if from == "" {
		sMgr.logPaladinMessageFieldMissingError(ctx, message, "From")
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, heartbeatNotification.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	coordinatorSnapshot := &common.CoordinatorSnapshot{}
	err = json.Unmarshal(heartbeatNotification.CoordinatorSnapshot, coordinatorSnapshot)
	if err != nil {
		sMgr.logPaladinMessageJsonUnmarshalError(ctx, "CoordinatorSnapshot", message, err)
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer for contract %s to pass heartbeat event: %v", contractAddress, err)
		return
	}

	heartbeatEvent := &common.HeartbeatReceivedEvent{}
	heartbeatEvent.FromNode = from
	heartbeatEvent.ContractAddress = contractAddress
	heartbeatEvent.CoordinatorSnapshot = coordinatorSnapshot
	heartbeatEvent.EventTime = time.Now()
	seq.GetOriginator().QueueEvent(ctx, heartbeatEvent)
	seq.GetCoordinator().QueueEvent(ctx, heartbeatEvent)
}

func (sMgr *sequencerManager) handlePreDispatchRequest(ctx context.Context, message *components.ReceivedMessage) {
	preDispatchRequest := &engineProto.TransactionDispatched{}

	err := proto.Unmarshal(message.Payload, preDispatchRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, preDispatchRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the predispatch request
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: predispatch request for transaction %s cannot be processed unless already in memory",
			contractAddress, preDispatchRequest.TransactionId)
		return
	}

	postAssemblyHash := pldtypes.NewBytes32FromSlice(preDispatchRequest.PostAssembleHash)

	preDispatchRequestReceivedEvent := &originatorTransaction.PreDispatchRequestReceivedEvent{
		RequestID:        uuid.MustParse(preDispatchRequest.Id),
		Coordinator:      message.FromNode,
		PostAssemblyHash: &postAssemblyHash,
	}
	preDispatchRequestReceivedEvent.TransactionID = uuid.MustParse(preDispatchRequest.TransactionId[2:34])
	preDispatchRequestReceivedEvent.EventTime = time.Now()

	// TODO - not sure where we should make the decision as to whether or not to approve dispatch.
	// For now we just proceed and send an approval response. It's possible that the check belongs in the state machine
	// validator function for PreDispatchRequestReceivedEvent?

	seq.GetOriginator().QueueEvent(ctx, preDispatchRequestReceivedEvent)
}

func (sMgr *sequencerManager) handlePreDispatchResponse(ctx context.Context, message *components.ReceivedMessage) {
	preDispatchResponse := &engineProto.TransactionDispatched{}

	err := proto.Unmarshal(message.Payload, preDispatchResponse)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, preDispatchResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the pre dispatch response
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: predispatch response for transaction %s cannot be processed unless already in memory",
			contractAddress, preDispatchResponse.TransactionId)
		return
	}

	// TODO - we don't yet return anything other than approved.

	dispatchRequestApprovedEvent := &coordTransaction.DispatchRequestApprovedEvent{
		RequestID: uuid.MustParse(preDispatchResponse.Id),
	}
	dispatchRequestApprovedEvent.TransactionID = uuid.MustParse(preDispatchResponse.TransactionId[2:34])
	dispatchRequestApprovedEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, dispatchRequestApprovedEvent)
}

func (sMgr *sequencerManager) handleDispatchedEvent(ctx context.Context, message *components.ReceivedMessage) {
	dispatchedEvent := &engineProto.TransactionDispatched{}

	err := proto.Unmarshal(message.Payload, dispatchedEvent)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, dispatchedEvent.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the dispatched event
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: dispatched event for transaction %s cannot be processed unless already in memory",
			contractAddress, dispatchedEvent.TransactionId)
		return
	}

	dispatchConfirmedEvent := &originatorTransaction.DispatchedEvent{}
	dispatchConfirmedEvent.TransactionID = uuid.MustParse(dispatchedEvent.TransactionId[2:34])
	dispatchConfirmedEvent.Coordinator = message.FromNode
	dispatchConfirmedEvent.EventTime = time.Now()

	seq.GetOriginator().QueueEvent(ctx, dispatchConfirmedEvent)
}

func (sMgr *sequencerManager) handleDelegationRequest(ctx context.Context, message *components.ReceivedMessage) {
	delegationRequest := &engineProto.DelegationRequest{}
	err := proto.Unmarshal(message.Payload, delegationRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	transactionDelegatedEvent := &coordinator.TransactionsDelegatedEvent{}
	transactionDelegatedEvent.FromNode = message.FromNode
	transactionDelegatedEvent.OriginatorsBlockHeight = uint64(delegationRequest.OriginatorBlockHeight)
	transactionDelegatedEvent.DelegationID = delegationRequest.DelegationId
	transactionDelegatedEvent.EventTime = time.Now()

	var contractAddress *pldtypes.EthAddress
	for _, txBytes := range delegationRequest.PrivateTransactions {
		privateTransaction := &components.PrivateTransaction{}
		if err = json.Unmarshal(txBytes, privateTransaction); err != nil {
			sMgr.logPaladinMessageJsonUnmarshalError(ctx, "PrivateTransaction", message, err)
			return
		}
		if contractAddress == nil {
			contractAddress = sMgr.parseContractAddressString(ctx, privateTransaction.PreAssembly.TransactionSpecification.ContractInfo.ContractAddress, message)
			if contractAddress == nil {
				return
			}
		}
		if transactionDelegatedEvent.Originator == "" {
			transactionDelegatedEvent.Originator = privateTransaction.PreAssembly.TransactionSpecification.From
		}
		transactionDelegatedEvent.Transactions = append(transactionDelegatedEvent.Transactions, privateTransaction)
	}

	if contractAddress == nil {
		log.L(ctx).Warnf("delegation request from %s contained no transactions", message.FromNode)
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to handle delegation request event %v:", err)
		return
	}

	seq.GetCoordinator().QueueEvent(ctx, transactionDelegatedEvent)
}

func (sMgr *sequencerManager) handleDelegationResponse(ctx context.Context, message *components.ReceivedMessage) {
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{}
	err := proto.Unmarshal(message.Payload, delegationRequestAcknowledgment)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, delegationRequestAcknowledgment.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	rejectedDelegationIDs := make([]string, 0, len(delegationRequestAcknowledgment.TransactionIds))
	rejectedDelegationMaxInFlight := 0
	rejectedDelegationCoordinatorError := 0

	// Currently we don't act on specific errors, but we have the option in the future to treat a specific delegate rejection
	// differently to just relying on re-delegate on the next heartbeat/timeout. For now log explicit rejections from the coordinator.
	for i, errorCode := range delegationRequestAcknowledgment.Errors {
		switch coordinator.DelegationAcknowledgementError(errorCode) {
		case coordinator.DelegationAcknowledgementError_MaxInflightTransactions:
			rejectedDelegationIDs = append(rejectedDelegationIDs, delegationRequestAcknowledgment.TransactionIds[i])
			rejectedDelegationMaxInFlight++
		case coordinator.DelegationAcknowledgementError_CoordinatorError, coordinator.DelegationAcknowledgementError_PreviousTransactionError:
			rejectedDelegationCoordinatorError++
		}
	}

	if rejectedDelegationMaxInFlight > 0 {
		log.L(ctx).Debugf("coordinator rejected %d delegations with max in flight limit", rejectedDelegationMaxInFlight)
		log.L(ctx).Tracef("rejected delegations: %+v", rejectedDelegationIDs)
	}
	if rejectedDelegationCoordinatorError > 0 {
		log.L(ctx).Warnf("coordinator error processing %d delegations", rejectedDelegationCoordinatorError)
	}
}

func (sMgr *sequencerManager) handleDelegationRejection(ctx context.Context, message *components.ReceivedMessage) {
	delegationRejection := &engineProto.DelegationRejection{}
	err := proto.Unmarshal(message.Payload, delegationRejection)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, delegationRejection.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: delegation rejection cannot be processed unless already in memory", contractAddress)
		return
	}

	rejectedEvent := &originator.DelegationRequestRejectedEvent{}
	rejectedEvent.ActiveCoordinator = delegationRejection.ActiveCoordinator
	rejectedEvent.RejectionReason = delegationRejection.RejectionReason
	rejectedEvent.OriginatorBlockHeight = delegationRejection.OriginatorBlockHeight
	rejectedEvent.CoordinatorBlockHeight = delegationRejection.CoordinatorBlockHeight
	rejectedEvent.BlockHeightTolerance = delegationRejection.BlockHeightTolerance
	rejectedEvent.EventTime = time.Now()
	seq.GetOriginator().QueueEvent(ctx, rejectedEvent)
}

func (sMgr *sequencerManager) handleHandoverRequest(ctx context.Context, message *components.ReceivedMessage) {
	handoverRequest := &engineProto.CoordinatorHandoverRequest{}
	if err := proto.Unmarshal(message.Payload, handoverRequest); err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, handoverRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the coordinator in memory to process the handover request
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: handover request cannot be processed unless already in memory", contractAddress)
		return
	}

	handoverEvent := &coordinator.HandoverRequestEvent{}
	handoverEvent.FromNode = handoverRequest.FromNode
	handoverEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, handoverEvent)
}

func (sMgr *sequencerManager) handleEndorsementRequest(ctx context.Context, message *components.ReceivedMessage) {
	endorsementRequest := &engineProto.EndorsementRequest{}
	err := proto.Unmarshal(message.Payload, endorsementRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, endorsementRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	privateEndorsementRequest := &components.PrivateTransactionEndorseRequest{
		TransactionSpecification: endorsementRequest.TransactionSpecification,
		Verifiers:                endorsementRequest.Verifiers,
		Signatures:               endorsementRequest.Signatures,
		InputStates:              endorsementRequest.InputStates,
		ReadStates:               endorsementRequest.ReadStates,
		OutputStates:             endorsementRequest.OutputStates,
		InfoStates:               endorsementRequest.InfoStates,
		Endorsement:              endorsementRequest.AttestationRequest,
		// Endorser is resolved by the coordinator goroutine via KeyManager.
	}

	for _, state := range privateEndorsementRequest.InfoStates {
		log.L(ctx).Debugf("private endorsement info state: %+v", state)
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("handleEndorsementRequest failed to obtain sequencer: %v", err)
		return
	}

	endorsementRequestReceivedEvent := &coordinator.EndorsementRequestReceivedEvent{
		FromNode:                  message.FromNode,
		TransactionId:             endorsementRequest.TransactionId,
		IdempotencyKey:            endorsementRequest.IdempotencyKey,
		Party:                     endorsementRequest.Party,
		PrivateEndorsementRequest: privateEndorsementRequest,
		AttestationRequest:        endorsementRequest.AttestationRequest,
		CoordinatorBlockHeight:    endorsementRequest.CoordinatorBlockHeight,
		BlockHeightTolerance:      endorsementRequest.BlockHeightTolerance,
	}
	if endorsementRequest.ExpiryTimeUnixMs != 0 {
		endorsementRequestReceivedEvent.Expiry = time.UnixMilli(endorsementRequest.ExpiryTimeUnixMs)
	}
	seq.GetCoordinator().QueueEvent(ctx, endorsementRequestReceivedEvent)
}

func (sMgr *sequencerManager) handleEndorsementResponse(ctx context.Context, message *components.ReceivedMessage) {
	endorsementResponse := &engineProto.EndorsementResponse{}
	err := proto.Unmarshal(message.Payload, endorsementResponse)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, endorsementResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the endorsement response
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: endorsement response for transaction %s cannot be processed unless already in memory",
			contractAddress, endorsementResponse.TransactionId)
		return
	}

	// Endorsement reverted
	if endorsementResponse.GetRevertReason() != "" {
		endorseRevertEvent := &coordTransaction.EndorseRevertEvent{}
		endorseRevertEvent.TransactionID = uuid.MustParse(endorsementResponse.TransactionId)
		endorseRevertEvent.RequestID = uuid.MustParse(endorsementResponse.IdempotencyKey)
		endorseRevertEvent.EventTime = time.Now()
		endorseRevertEvent.Party = endorsementResponse.Party
		endorseRevertEvent.RevertReason = endorsementResponse.GetRevertReason()
		endorseRevertEvent.AttestationRequestName = endorsementResponse.AttestationRequestName
		seq.GetCoordinator().QueueEvent(ctx, endorseRevertEvent)
		return
	}

	// Endorsement succeeded
	endorsement := endorsementResponse.Endorsement

	endorsementResponseEvent := &coordTransaction.EndorsedEvent{}
	endorsementResponseEvent.TransactionID = uuid.MustParse(endorsementResponse.TransactionId)
	endorsementResponseEvent.RequestID = uuid.MustParse(endorsementResponse.IdempotencyKey)
	endorsementResponseEvent.Endorsement = endorsement
	endorsementResponseEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, endorsementResponseEvent)
}

func (sMgr *sequencerManager) handleEndorsementRejection(ctx context.Context, message *components.ReceivedMessage) {
	endorsementRejection := &engineProto.EndorsementRejection{}
	err := proto.Unmarshal(message.Payload, endorsementRejection)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, endorsementRejection.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: endorsement rejection for transaction %s cannot be processed unless already in memory",
			contractAddress, endorsementRejection.TransactionId)
		return
	}

	endorseRejectedEvent := &coordTransaction.EndorseRequestRejectedEvent{}
	endorseRejectedEvent.TransactionID = uuid.MustParse(endorsementRejection.TransactionId)
	endorseRejectedEvent.RequestID = uuid.MustParse(endorsementRejection.IdempotencyKey)
	endorseRejectedEvent.EventTime = time.Now()
	endorseRejectedEvent.Party = endorsementRejection.Party
	endorseRejectedEvent.AttestationRequestName = endorsementRejection.AttestationRequestName
	endorseRejectedEvent.RejectionReason = endorsementRejection.RejectionReason
	endorseRejectedEvent.CoordinatorBlockHeight = endorsementRejection.CoordinatorBlockHeight
	endorseRejectedEvent.EndorserBlockHeight = endorsementRejection.EndorserBlockHeight
	endorseRejectedEvent.BlockHeightTolerance = endorsementRejection.BlockHeightTolerance
	seq.GetCoordinator().QueueEvent(ctx, endorseRejectedEvent)
}

func (sMgr *sequencerManager) handleEndorsementError(ctx context.Context, message *components.ReceivedMessage) {
	endorsementError := &engineProto.EndorsementError{}
	err := proto.Unmarshal(message.Payload, endorsementError)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, endorsementError.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: endorsement error for transaction %s cannot be processed unless already in memory",
			contractAddress, endorsementError.TransactionId)
		return
	}

	endorseErrorEvent := &coordTransaction.EndorseErrorEvent{}
	endorseErrorEvent.TransactionID = uuid.MustParse(endorsementError.TransactionId)
	endorseErrorEvent.RequestID = uuid.MustParse(endorsementError.IdempotencyKey)
	endorseErrorEvent.EventTime = time.Now()
	endorseErrorEvent.Party = endorsementError.Party
	endorseErrorEvent.AttestationRequestName = endorsementError.AttestationRequestName
	seq.GetCoordinator().QueueEvent(ctx, endorseErrorEvent)
}

func (sMgr *sequencerManager) handleNonceAssigned(ctx context.Context, message *components.ReceivedMessage) {
	nonceAssigned := &engineProto.NonceAssigned{}
	err := proto.Unmarshal(message.Payload, nonceAssigned)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, nonceAssigned.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the nonce assigned event
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: nonce assigned event for transaction %s cannot be processed unless already in memory",
			contractAddress, nonceAssigned.TransactionId)
		return
	}

	nonceAssignedEvent := &originatorTransaction.NonceAssignedEvent{}
	nonceAssignedEvent.TransactionID = uuid.MustParse(nonceAssigned.TransactionId)
	nonceAssignedEvent.Nonce = uint64(nonceAssigned.Nonce)
	nonceAssignedEvent.Coordinator = message.FromNode
	nonceAssignedEvent.EventTime = time.Now()

	seq.GetOriginator().QueueEvent(ctx, nonceAssignedEvent)
}

func (sMgr *sequencerManager) handleTransactionSubmitted(ctx context.Context, message *components.ReceivedMessage) {
	transactionSubmitted := &engineProto.TransactionSubmitted{}
	err := proto.Unmarshal(message.Payload, transactionSubmitted)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, transactionSubmitted.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the transaction submitted event
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: transaction submitted event for transaction %s cannot be processed unless already in memory",
			contractAddress, transactionSubmitted.TransactionId)
		return
	}

	transactionSubmittedEvent := &originatorTransaction.SubmittedEvent{}
	transactionSubmittedEvent.TransactionID = uuid.MustParse(transactionSubmitted.TransactionId)
	transactionSubmittedEvent.LatestSubmissionHash = pldtypes.Bytes32(transactionSubmitted.Hash)
	transactionSubmittedEvent.Coordinator = message.FromNode
	transactionSubmittedEvent.EventTime = time.Now()

	seq.GetOriginator().QueueEvent(ctx, transactionSubmittedEvent)
}

func (sMgr *sequencerManager) handleTransactionConfirmed(ctx context.Context, message *components.ReceivedMessage) {
	transactionConfirmed := &engineProto.TransactionConfirmed{}
	err := proto.Unmarshal(message.Payload, transactionConfirmed)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, transactionConfirmed.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the transaction confirmed event
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: transaction confirmed event for transaction %s cannot be processed unless already in memory",
			contractAddress, transactionConfirmed.TransactionId)
		return
	}

	if transactionConfirmed.GetOutcome() == engineProto.TransactionConfirmed_OUTCOME_REVERTED {
		transactionSubmittedEvent := &originatorTransaction.ConfirmedRevertedEvent{}
		transactionSubmittedEvent.TransactionID = uuid.MustParse(transactionConfirmed.TransactionId)
		transactionSubmittedEvent.RevertReason = transactionConfirmed.RevertReason
		transactionSubmittedEvent.FailureMessage = transactionConfirmed.GetFailureMessage()
		transactionSubmittedEvent.WillRetry = transactionConfirmed.WillRetry
		transactionSubmittedEvent.EventTime = time.Now()
		seq.GetOriginator().QueueEvent(ctx, transactionSubmittedEvent)
	} else {
		transactionSubmittedEvent := &originatorTransaction.ConfirmedSuccessEvent{}
		transactionSubmittedEvent.TransactionID = uuid.MustParse(transactionConfirmed.TransactionId)
		transactionSubmittedEvent.EventTime = time.Now()
		seq.GetOriginator().QueueEvent(ctx, transactionSubmittedEvent)
	}
}

func (sMgr *sequencerManager) handlePreDispatchRejection(ctx context.Context, message *components.ReceivedMessage) {
	rejection := &engineProto.PreDispatchRejection{}
	if err := proto.Unmarshal(message.Payload, rejection); err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, rejection.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	txID, err := uuid.Parse(rejection.TransactionId)
	if err != nil {
		log.L(ctx).Errorf("handlePreDispatchRejection: invalid transaction ID %q: %v", rejection.TransactionId, err)
		return
	}

	requestID, err := uuid.Parse(rejection.RequestId)
	if err != nil {
		log.L(ctx).Errorf("handlePreDispatchRejection: invalid request ID %q: %v", rejection.RequestId, err)
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: pre-dispatch rejection for transaction %s cannot be processed unless already in memory",
			contractAddress, rejection.TransactionId)
		return
	}

	log.L(ctx).Debugf("received pre-dispatch rejection for tx %s from originator %s (reason=%d)", txID, message.FromNode, rejection.RejectionReason)

	rejectedEvent := &coordTransaction.PreDispatchRequestRejectedEvent{
		BaseCoordinatorEvent: coordTransaction.BaseCoordinatorEvent{
			TransactionID: txID,
		},
		RequestID:       requestID,
		RejectionReason: rejection.RejectionReason,
	}
	rejectedEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, rejectedEvent)
}
