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
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"

	"google.golang.org/protobuf/proto"
)

// Where a request is sent, there are three possible types of message that may be sent back:
// - response: the result of actioning the request, which may include expected errors (e.g. assembly reverted)
// - error: an unexpected error occurred while actioning the request
// - rejection: the request was not actioned, a rejection reason must be included
type TransportWriter interface {
	StartLoopbackWriter()
	WaitForDone(ctx context.Context)
	SendDelegationRequest(ctx context.Context, coordinatorNode string, transactions []*components.PrivateTransaction, blockHeight uint64) error
	SendDelegationResponse(ctx context.Context, delegatingNodeName string, delegationId string, transactionIDs []string, errors []int64, blockHeight uint64) error
	SendDelegationRejection(ctx context.Context, delegatingNodeName string, delegationId string, rejectionReason engineProto.RejectionReason, activeCoordinator string, originatorBlockHeight, coordinatorBlockHeight, blockHeightTolerance int64) error
	SendHandoverRequest(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress) error
	SendEndorsementRequest(ctx context.Context, txID uuid.UUID, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*prototk.EndorsableState, readStates []*prototk.EndorsableState, outputStates []*prototk.EndorsableState, infoStates []*prototk.EndorsableState, expiryTime time.Time, coordinatorBlockHeight int64, blockHeightTolerance int64) error
	SendEndorsementResponse(ctx context.Context, transactionId, idempotencyKey, contractAddress string, attResult *prototk.AttestationResult, endorsementResult *components.EndorsementResult, revertReason, endorsementName, party, node string) error
	SendEndorsementError(ctx context.Context, transactionId, idempotencyKey, contractAddress, errorMessage, party, attestationRequestName, node string) error
	SendEndorsementRejection(ctx context.Context, transactionId, idempotencyKey, contractAddress, endorsementName, party, node string, reason engineProto.RejectionReason, coordinatorBlockHeight, endorserBlockHeight, blockHeightTolerance int64) error
	SendAssembleRequest(ctx context.Context, assemblingNode string, txID uuid.UUID, idempotencyId uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocks grapher.ExportableStates, coordinatorBlockHeight int64, expiryTime time.Time, blockHeightTolerance int64) error
	SendAssembleResponse(ctx context.Context, txID uuid.UUID, assembleRequestId uuid.UUID, postAssembly *components.TransactionPostAssembly, preAssembly *components.TransactionPreAssembly, recipient string) error
	SendAssembleError(ctx context.Context, txID uuid.UUID, assembleRequestId uuid.UUID, recipient string) error
	SendAssembleRejection(ctx context.Context, txID uuid.UUID, assembleRequestId uuid.UUID, recipient string, reason engineProto.RejectionReason, coordinatorBlockHeight, assemblerBlockHeight int64) error
	SendNonceAssigned(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, nonce uint64) error
	SendTransactionSubmitted(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, txHash *pldtypes.Bytes32) error
	SendTransactionConfirmed(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, nonce *pldtypes.HexUint64, outcome engineProto.TransactionConfirmed_Outcome, revertReason pldtypes.HexBytes, failureMessage string, willRetry bool) error
	SendHeartbeat(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress, coordinatorSnapshot *common.CoordinatorSnapshot) error
	SendPreDispatchRequest(ctx context.Context, originatorNode string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification, hash *pldtypes.Bytes32) error
	SendPreDispatchResponse(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error
	SendPreDispatchRejection(ctx context.Context, txID uuid.UUID, requestID uuid.UUID, coordinatorNode string, reason engineProto.RejectionReason) error
	SendDispatched(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error
}

func NewTransportWriter(ctx context.Context, contractAddress *pldtypes.EthAddress, nodeID string, transportManager components.TransportManager, loopbackHandler func(ctx context.Context, message *components.ReceivedMessage)) TransportWriter {
	loopbackTransport := NewLoopbackTransportWriter(loopbackHandler)
	return &transportWriter{
		ctx:                   ctx,
		nodeID:                nodeID,
		transportManager:      transportManager,
		loopbackTransport:     loopbackTransport,
		contractAddress:       contractAddress,
		loopbackSenderStopped: make(chan struct{}),
	}
}

type transportWriter struct {
	ctx                   context.Context
	nodeID                string
	transportManager      components.TransportManager
	loopbackTransport     LoopbackTransportManager
	contractAddress       *pldtypes.EthAddress
	loopbackSenderStopped chan struct{}
}

func (tw *transportWriter) StartLoopbackWriter() {
	// We use a separate goroutine to send loopback messages to free up the event loops.
	go tw.loopbackSender()
}

func (tw *transportWriter) WaitForDone(ctx context.Context) {
	select {
	case <-tw.loopbackSenderStopped:
	case <-ctx.Done():
	}
}

func (tw *transportWriter) SendDelegationRequest(
	ctx context.Context,
	coordinatorNode string,
	transactions []*components.PrivateTransaction,
	blockHeight uint64,
) error {
	allTxBytes := make([][]byte, 0, len(transactions))
	for _, transaction := range transactions {
		transactionBytes, err := json.Marshal(transaction)
		if err != nil {
			log.L(ctx).Errorf("error marshalling transaction for delegation request: %v", err)
			return err
		}
		allTxBytes = append(allTxBytes, transactionBytes)
	}

	delegationRequest := &engineProto.DelegationRequest{
		DelegateNodeId:        coordinatorNode,
		OriginatorBlockHeight: int64(blockHeight),
		PrivateTransactions:   allTxBytes,
	}
	delegationRequestBytes, err := proto.Marshal(delegationRequest)
	if err != nil {
		log.L(ctx).Errorf("error marshalling delegationRequest message: %s", err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_DelegationRequest,
		Payload:     delegationRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        coordinatorNode,
	}); err != nil {
		log.L(ctx).Warnf("error sending delegationRequest message: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendDelegationResponse(
	ctx context.Context,
	delegatingNodeName string,
	delegationId string,
	transactionIDs []string,
	errors []int64,
	blockHeight uint64,
) error {
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		DelegationId:    delegationId,
		TransactionIds:  transactionIDs,
		DelegateNodeId:  delegatingNodeName,
		ContractAddress: tw.contractAddress.String(),
		Errors:          errors,
	}
	delegationRequestAcknowledgmentBytes, err := proto.Marshal(delegationRequestAcknowledgment)
	if err != nil {
		log.L(ctx).Errorf("error marshalling delegationRequestAcknowledgment  message: %s", err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_DelegationResponse,
		Payload:     delegationRequestAcknowledgmentBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        delegatingNodeName,
	}); err != nil {
		log.L(ctx).Warnf("error sending delegationResponse message: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendDelegationRejection(
	ctx context.Context,
	delegatingNodeName string,
	delegationId string,
	rejectionReason engineProto.RejectionReason,
	activeCoordinator string,
	originatorBlockHeight, coordinatorBlockHeight, blockHeightTolerance int64,
) error {
	rejection := &engineProto.DelegationRejection{
		DelegationId:           delegationId,
		DelegateNodeId:         delegatingNodeName,
		ContractAddress:        tw.contractAddress.String(),
		ActiveCoordinator:      activeCoordinator,
		RejectionReason:        rejectionReason,
		OriginatorBlockHeight:  originatorBlockHeight,
		CoordinatorBlockHeight: coordinatorBlockHeight,
		BlockHeightTolerance:   blockHeightTolerance,
	}
	rejectionBytes, err := proto.Marshal(rejection)
	if err != nil {
		log.L(ctx).Errorf("error marshalling delegation rejection message: %s", err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_DelegationRejection,
		Payload:     rejectionBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        delegatingNodeName,
	}); err != nil {
		log.L(ctx).Warnf("error sending delegationRejection message: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendHandoverRequest(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress) error {
	log.L(ctx).Debugf("transport writer sending handover request to %s for contract %s", targetNode, contractAddress.String())

	handoverRequest := &engineProto.CoordinatorHandoverRequest{
		FromNode:        tw.nodeID,
		ContractAddress: contractAddress.String(),
	}
	handoverRequestBytes, err := proto.Marshal(handoverRequest)
	if err != nil {
		log.L(ctx).Errorf("error marshalling handover request: %s", err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_HandoverRequest,
		Payload:     handoverRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        targetNode,
	}); err != nil {
		log.L(ctx).Warnf("error sending handover request: %s", err)
	}
	return nil
}

// TODO do we have duplication here?  contractAddress and transactionID are in the transactionSpecification
func (tw *transportWriter) SendEndorsementRequest(ctx context.Context, txID uuid.UUID, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*prototk.EndorsableState, readStates []*prototk.EndorsableState, outputStates []*prototk.EndorsableState, infoStates []*prototk.EndorsableState, expiryTime time.Time, coordinatorBlockHeight int64, blockHeightTolerance int64) error {
	log.L(ctx).Debugf("sending endorse request with TX ID %+v", transactionSpecification.TransactionId)
	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey:           idempotencyKey.String(),
		ContractAddress:          transactionSpecification.ContractInfo.ContractAddress,
		TransactionId:            txID.String(),
		AttestationRequest:       attRequest,
		Party:                    party,
		TransactionSpecification: transactionSpecification,
		Verifiers:                verifiers,
		Signatures:               signatures,
		InputStates:              inputStates,
		ReadStates:               readStates,
		OutputStates:             outputStates,
		InfoStates:               infoStates,
		ExpiryTimeUnixMs:         expiryTime.UnixMilli(),
		CoordinatorBlockHeight:   coordinatorBlockHeight,
		BlockHeightTolerance:     blockHeightTolerance,
	}

	endorsementRequestBytes, err := proto.Marshal(endorsementRequest)
	if err != nil {
		log.L(ctx).Error("error marshalling endorsement request", err)
		return err
	}

	partyNode, err := pldtypes.PrivateIdentityLocator(party).Node(ctx, false)
	if err != nil {
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_EndorsementRequest,
		Node:        partyNode,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     endorsementRequestBytes,
	}); err != nil {
		log.L(ctx).Warnf("error sending endorsement request: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendEndorsementResponse(ctx context.Context, transactionId, idempotencyKey, contractAddress string, attResult *prototk.AttestationResult, endorsementResult *components.EndorsementResult, revertReason, endorsementName, party, node string) error {

	endorsementResponse := &engineProto.EndorsementResponse{
		Endorsement:            attResult,
		TransactionId:          transactionId,
		IdempotencyKey:         idempotencyKey,
		AttestationRequestName: endorsementName,
		Party:                  party,
		ContractAddress:        contractAddress,
	}

	if revertReason != "" {
		endorsementResponse.RevertReason = &revertReason
	}

	endorsementResponseBytes, err := proto.Marshal(endorsementResponse)
	if err != nil {
		log.L(ctx).Error("error marshalling endorsement response", err)
	}

	payload := &components.FireAndForgetMessageSend{
		MessageType: MessageType_EndorsementResponse,
		Node:        node,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     endorsementResponseBytes,
	}

	if err = tw.send(ctx, payload); err != nil {
		log.L(ctx).Warnf("error sending endorsement response: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendEndorsementError(ctx context.Context, transactionId, idempotencyKey, contractAddress, errorMessage, party, attestationRequestName, node string) error {
	endorsementError := &engineProto.EndorsementError{
		TransactionId:          transactionId,
		IdempotencyKey:         idempotencyKey,
		ContractAddress:        contractAddress,
		ErrorMessage:           errorMessage,
		Party:                  party,
		AttestationRequestName: attestationRequestName,
	}
	endorsementErrorBytes, err := proto.Marshal(endorsementError)
	if err != nil {
		log.L(ctx).Errorf("error marshalling endorsement error message: %s", err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_EndorsementError,
		Node:        node,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     endorsementErrorBytes,
	}); err != nil {
		log.L(ctx).Warnf("error sending endorsement error message: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendEndorsementRejection(ctx context.Context, transactionId, idempotencyKey, contractAddress, endorsementName, party, node string, reason engineProto.RejectionReason, coordinatorBlockHeight, endorserBlockHeight, blockHeightTolerance int64) error {
	rejection := &engineProto.EndorsementRejection{
		TransactionId:          transactionId,
		IdempotencyKey:         idempotencyKey,
		ContractAddress:        contractAddress,
		AttestationRequestName: endorsementName,
		Party:                  party,
		RejectionReason:        reason,
		CoordinatorBlockHeight: coordinatorBlockHeight,
		EndorserBlockHeight:    endorserBlockHeight,
		BlockHeightTolerance:   blockHeightTolerance,
	}
	rejectionBytes, err := proto.Marshal(rejection)
	if err != nil {
		log.L(ctx).Errorf("error marshalling endorsement rejection message: %s", err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_EndorsementRejection,
		Node:        node,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     rejectionBytes,
	}); err != nil {
		log.L(ctx).Warnf("error sending endorsement rejection: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendAssembleRequest(ctx context.Context, assemblingNode string, txID uuid.UUID, idempotencyId uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocks grapher.ExportableStates, coordinatorBlockHeight int64, expiryTime time.Time, blockHeightTolerance int64) error {

	log.L(ctx).Tracef("transport writer attempting to send assemble request to assembling node %s", assemblingNode)

	preAssemblyBytes, err := json.Marshal(preAssembly)
	if err != nil {
		log.L(ctx).Error("error marshalling preassembly", err)
		return err
	}
	stateLocksJSON, err := json.Marshal(stateLocks)
	if err != nil {
		log.L(ctx).Error("error marshalling state locks", err)
		return err
	}
	log.L(ctx).Debugf("assemble request state locks for tx %s: %s", txID, string(stateLocksJSON))

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:          txID.String(),
		AssembleRequestId:      idempotencyId.String(),
		ContractAddress:        tw.contractAddress.HexString(),
		PreAssembly:            preAssemblyBytes,
		StateLocks:             stateLocksJSON,
		CoordinatorBlockHeight: coordinatorBlockHeight,
		ExpiryTimeUnixMs:       expiryTime.UnixMilli(),
		BlockHeightTolerance:   blockHeightTolerance,
	}

	assembleRequestBytes, err := proto.Marshal(assembleRequest)
	if err != nil {
		log.L(ctx).Error("error marshalling assemble request", err)
		return err
	}

	payload := &components.FireAndForgetMessageSend{
		MessageType: MessageType_AssembleRequest,
		Node:        assemblingNode,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     assembleRequestBytes,
	}

	if err = tw.send(ctx, payload); err != nil {
		log.L(ctx).Warnf("error sending assemble request: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendAssembleError(ctx context.Context, txID uuid.UUID, assembleRequestId uuid.UUID, recipient string) error {

	log.L(ctx).Tracef("transport writer attempting to send assemble error response to node %s", recipient)

	assembleError := &engineProto.AssembleError{
		TransactionId:     txID.String(),
		AssembleRequestId: assembleRequestId.String(),
		ContractAddress:   tw.contractAddress.HexString(),
	}
	assembleErrorBytes, err := proto.Marshal(assembleError)
	if err != nil {
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_AssembleError,
		Node:        recipient,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     assembleErrorBytes,
	}); err != nil {
		log.L(ctx).Warnf("error sending assemble error response to %s: %s", recipient, err)
	}
	return nil
}

func (tw *transportWriter) SendAssembleRejection(ctx context.Context, txID uuid.UUID, assembleRequestId uuid.UUID, recipient string, reason engineProto.RejectionReason, coordinatorBlockHeight, assemblerBlockHeight int64) error {

	log.L(ctx).Tracef("transport writer attempting to send assemble rejection to node %s (reason=%d)", recipient, reason)

	rejection := &engineProto.AssembleRejection{
		TransactionId:          txID.String(),
		AssembleRequestId:      assembleRequestId.String(),
		ContractAddress:        tw.contractAddress.HexString(),
		RejectionReason:        reason,
		CoordinatorBlockHeight: coordinatorBlockHeight,
		AssemblerBlockHeight:   assemblerBlockHeight,
	}
	rejectionBytes, err := proto.Marshal(rejection)
	if err != nil {
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_AssembleRejection,
		Node:        recipient,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     rejectionBytes,
	}); err != nil {
		log.L(ctx).Warnf("error sending assemble rejection to %s: %s", recipient, err)
	}
	return nil
}

func (tw *transportWriter) SendPreDispatchRejection(ctx context.Context, txID uuid.UUID, requestID uuid.UUID, coordinatorNode string, reason engineProto.RejectionReason) error {
	log.L(ctx).Debugf("transport writer sending pre-dispatch rejection for tx %s to coordinator %s (reason=%d)", txID, coordinatorNode, reason)

	if tw.contractAddress == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "attempt to send pre-dispatch rejection without specifying contract address")
	}

	msgBytes, err := proto.Marshal(&engineProto.PreDispatchRejection{
		TransactionId:   txID.String(),
		RequestId:       requestID.String(),
		ContractAddress: tw.contractAddress.HexString(),
		RejectionReason: reason,
	})
	if err != nil {
		log.L(ctx).Errorf("error marshalling pre-dispatch rejection: %s", err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_PreDispatchRejection,
		Payload:     msgBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        coordinatorNode,
	}); err != nil {
		log.L(ctx).Warnf("error sending pre-dispatch rejection to %s: %s", coordinatorNode, err)
	}
	return nil
}

func (tw *transportWriter) SendAssembleResponse(ctx context.Context, txID uuid.UUID, assembleRequestId uuid.UUID, postAssembly *components.TransactionPostAssembly, preAssembly *components.TransactionPreAssembly, recipient string) error {

	log.L(ctx).Tracef("transport writer attempting to send assemble response to node %s", recipient)

	postAssemblyBytes, err := json.Marshal(postAssembly)
	if err != nil {
		return err
	}

	preAssemblyBytes, err := json.Marshal(preAssembly)
	if err != nil {
		return err
	}

	assembleResponse := &engineProto.AssembleResponse{
		TransactionId:     txID.String(),
		AssembleRequestId: assembleRequestId.String(),
		ContractAddress:   tw.contractAddress.HexString(),
		PostAssembly:      postAssemblyBytes,
		PreAssembly:       preAssemblyBytes,
	}
	assembleResponseBytes, err := proto.Marshal(assembleResponse)
	if err != nil {
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_AssembleResponse,
		Node:        recipient,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     assembleResponseBytes,
	}); err != nil {
		log.L(ctx).Warnf("error sending assemble response to %s: %s", recipient, err)
	}
	return nil
}

func (tw *transportWriter) SendNonceAssigned(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, nonce uint64) error {

	log.L(ctx).Tracef("transport writer attempting to send nonce assigned message to node %s", originatorNode)

	if contractAddress == nil {
		err := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "attempt to send nonce assigned event request without specifying contract address")
		return err
	}
	nonceAssigned := &engineProto.NonceAssigned{
		Id:              uuid.New().String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddress.HexString(),
		Nonce:           int64(nonce),
	}
	nonceAssignedBytes, err := proto.Marshal(nonceAssigned)
	if err != nil {
		log.L(ctx).Errorf("error marshalling nonce assigned event: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_NonceAssigned,
		Payload:     nonceAssignedBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        originatorNode,
	}); err != nil {
		log.L(ctx).Warnf("error sending nonce assigned event: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendTransactionSubmitted(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, txHash *pldtypes.Bytes32) error {

	log.L(ctx).Tracef("transport writer attempting to send transaction submitted message to node %s", originatorNode)

	if contractAddress == nil {
		err := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "attempt to send TX submitted event without specifying contract address")
		return err
	}
	txSubmitted := &engineProto.TransactionSubmitted{
		Id:              uuid.New().String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddress.HexString(),
		Hash:            txHash.Bytes(),
	}
	txSubmittedBytes, err := proto.Marshal(txSubmitted)
	if err != nil {
		log.L(ctx).Errorf("error marshalling TX submitted event: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_TransactionSubmitted,
		Payload:     txSubmittedBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        originatorNode,
	}); err != nil {
		log.L(ctx).Warnf("error sending transaction submitted event: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendTransactionConfirmed(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, nonce *pldtypes.HexUint64, outcome engineProto.TransactionConfirmed_Outcome, revertReason pldtypes.HexBytes, failureMessage string, willRetry bool) error {

	log.L(ctx).Tracef("transport writer attempting to send transaction confirmed message to node %s", originatorNode)

	if contractAddress == nil {
		err := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "attempt to send TX submitted event without specifying contract address")
		return err
	}

	txConfirmed := &engineProto.TransactionConfirmed{
		Id:              uuid.New().String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddress.HexString(),
		Outcome:         outcome,
		RevertReason:    revertReason,
		FailureMessage:  failureMessage,
		WillRetry:       willRetry,
	}
	if nonce != nil {
		txConfirmed.Nonce = int64(*nonce)
	}
	txConfirmedBytes, err := proto.Marshal(txConfirmed)
	if err != nil {
		log.L(ctx).Errorf("error marshalling TX confirmed event: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_TransactionConfirmed,
		Payload:     txConfirmedBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        originatorNode,
	}); err != nil {
		log.L(ctx).Warnf("error sending transaction confirmed event: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendHeartbeat(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress, coordinatorSnapshot *common.CoordinatorSnapshot) error {

	log.L(ctx).Tracef("transport writer attempting to send haertbeat to node %s", targetNode)

	coordinatorSnapshotBytes, err := json.Marshal(coordinatorSnapshot)
	if err != nil {
		log.L(ctx).Errorf("error marshalling heartbeat: %s", err)
		return err
	}

	heartbeatRequest := &engineProto.CoordinatorHeartbeatNotification{
		From:                tw.transportManager.LocalNodeName(),
		ContractAddress:     contractAddress.HexString(),
		CoordinatorSnapshot: coordinatorSnapshotBytes,
	}
	log.L(ctx).Debugf("sending heartbeat: From 	%s, Contract Address %s", tw.transportManager.LocalNodeName(), contractAddress.HexString())
	heartbeatRequestBytes, err := proto.Marshal(heartbeatRequest)
	if err != nil {
		log.L(ctx).Errorf("error marshalling heartbeat request message: %s", err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_CoordinatorHeartbeatNotification,
		Payload:     heartbeatRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        targetNode,
	}); err != nil {
		// Transport failures sending a heartbeat are transient and best-effort — the send layer
		// itself is fire-and-forget and message loss is expected. Log a warning but don't
		// propagate the error so callers can continue with subsequent actions.
		log.L(ctx).Warnf("error sending heartbeat request message: %s", err)
	}

	return nil
}

func (tw *transportWriter) SendPreDispatchRequest(ctx context.Context, originatorNode string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification, hash *pldtypes.Bytes32) error {

	log.L(ctx).Tracef("transport writer attempting to send pre-dispatch request to node %s", originatorNode)

	dispatchConfirmationRequest := &engineProto.PreDispatchRequest{
		Id:              idempotencyKey.String(),
		TransactionId:   transactionSpecification.TransactionId,
		ContractAddress: tw.contractAddress.HexString(),
		PostAssembleHash: hash.Bytes(),
	}

	dispatchConfirmationRequestBytes, err := proto.Marshal(dispatchConfirmationRequest)
	if err != nil {
		log.L(ctx).Errorf("error marshalling pre-dispatch request message: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_PreDispatchRequest,
		Payload:     dispatchConfirmationRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        originatorNode,
	}); err != nil {
		log.L(ctx).Warnf("error sending pre-dispatch request message: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendPreDispatchResponse(ctx context.Context, transactionOriginatorNode string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {

	log.L(ctx).Tracef("transport writer attempting to send pre-dispatch response to node %s", transactionOriginatorNode)

	dispatchResponseEvent := &engineProto.PreDispatchResponse{
		Id:              idempotencyKey.String(),
		TransactionId:   transactionSpecification.TransactionId,
		ContractAddress: tw.contractAddress.HexString(),
	}

	dispatchResponseEventBytes, err := proto.Marshal(dispatchResponseEvent)
	if err != nil {
		log.L(ctx).Errorf("error marshalling pre-dispatch response message: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_PreDispatchResponse,
		Payload:     dispatchResponseEventBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        transactionOriginatorNode,
	}); err != nil {
		log.L(ctx).Warnf("error sending pre-dispatch response message: %s", err)
	}
	return nil
}

func (tw *transportWriter) SendDispatched(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {

	log.L(ctx).Tracef("transport writer attempting to send dispatched message to node %s", transactionOriginator)

	dispatchedEvent := &engineProto.TransactionDispatched{
		Id:              idempotencyKey.String(),
		TransactionId:   transactionSpecification.TransactionId,
		ContractAddress: tw.contractAddress.HexString(),
		Signer:          transactionOriginator,
	}

	dispatchedEventBytes, err := proto.Marshal(dispatchedEvent)
	if err != nil {
		log.L(ctx).Errorf("error marshalling dispatch confirmation request  message: %s", err)
	}

	node, err := pldtypes.PrivateIdentityLocator(transactionOriginator).Node(ctx, false)
	if err != nil {
		log.L(ctx).Errorf("error getting transaction dispatched originator node id for %s: %s", transactionOriginator, err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_Dispatched,
		Payload:     dispatchedEventBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        node,
	}); err != nil {
		log.L(ctx).Warnf("error sending dispatched event: %s", err)
	}
	return nil
}

func (tw *transportWriter) send(ctx context.Context, payload *components.FireAndForgetMessageSend) error {
	if payload.Node == "" {
		err := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "attempt to send message without specifying destination node name")
		return err
	}

	log.L(ctx).Debugf("%+v sent to %s", payload.MessageType, payload.Node)
	if payload.Node == "" || payload.Node == tw.transportManager.LocalNodeName() {
		// "Localhost" loopback
		log.L(ctx).Debugf("sending %s to loopback interface", payload.MessageType)
		select {
		case tw.loopbackTransport.LoopbackQueue() <- payload:
		case <-ctx.Done():
			return ctx.Err()
		case <-tw.ctx.Done():
			return tw.ctx.Err()
		}

		return nil
	}
	log.L(ctx).Debugf("sending %s to node: %s", payload.MessageType, payload.Node)
	err := tw.transportManager.Send(ctx, payload)
	return err
}

// Run the loopback transport in a goroutine to avoid blocking the event loop. This is important for the
// channel-based event queue to ensure the queue consumer is not blocked when we happen to be sending
// to ourselves.
func (tw *transportWriter) loopbackSender() {
	defer close(tw.loopbackSenderStopped)
	for {
		select {
		case queuedPayload, ok := <-tw.loopbackTransport.LoopbackQueue():
			if !ok {
				log.L(tw.ctx).Infof("shutting down loopback sender for contract %s", tw.contractAddress.String())
				return
			}

			err := tw.loopbackTransport.Send(tw.ctx, queuedPayload)
			if err != nil {
				log.L(tw.ctx).Errorf("error sending %s to loopback interface for contract %s: %s", queuedPayload.MessageType, tw.contractAddress.String(), err)
			}
		case <-tw.ctx.Done():
			log.L(tw.ctx).Infof("shutting down loopback sender for contract %s", tw.contractAddress.String())
			return
		}
	}
}
