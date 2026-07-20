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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"google.golang.org/protobuf/proto"
)

// protoMarshalFn is a package-level variable so tests can inject errors.
var protoMarshalFn = proto.MarshalOptions{}.Marshal

// Where a request is sent, there are three possible types of message that may be sent back:
// - response: the result of actioning the request, which may include expected errors (e.g. assembly reverted)
// - error: an unexpected error occurred while actioning the request
// - rejection: the request was not actioned, a rejection reason must be included
type TransportWriter interface {
	StartLoopbackWriter()
	WaitForDone(ctx context.Context)
	SendDelegationRequest(ctx context.Context, node string, msg *engineProto.DelegationRequest) error
	SendDelegationResponse(ctx context.Context, node string, msg *engineProto.DelegationResponse) error
	SendDelegationRejection(ctx context.Context, node string, msg *engineProto.DelegationRejection) error
	SendHandoverRequest(ctx context.Context, node string, msg *engineProto.CoordinatorHandoverRequest) error
	SendEndorsementRequest(ctx context.Context, node string, msg *engineProto.EndorsementRequest) error
	SendEndorsementResponse(ctx context.Context, node string, msg *engineProto.EndorsementResponse) error
	SendEndorsementError(ctx context.Context, node string, msg *engineProto.EndorsementError) error
	SendEndorsementRejection(ctx context.Context, node string, msg *engineProto.EndorsementRejection) error
	SendAssembleRequest(ctx context.Context, node string, msg *engineProto.AssembleRequest) error
	SendAssembleResponse(ctx context.Context, node string, msg *engineProto.AssembleResponse) error
	SendAssembleError(ctx context.Context, node string, msg *engineProto.AssembleError) error
	SendAssembleRejection(ctx context.Context, node string, msg *engineProto.AssembleRejection) error
	SendNonceAssigned(ctx context.Context, node string, msg *engineProto.NonceAssigned) error
	SendTransactionSubmitted(ctx context.Context, node string, msg *engineProto.TransactionSubmitted) error
	SendTransactionConfirmed(ctx context.Context, node string, msg *engineProto.TransactionConfirmed) error
	SendHeartbeat(ctx context.Context, node string, msg *engineProto.CoordinatorHeartbeatNotification) error
	SendPreDispatchRequest(ctx context.Context, node string, msg *engineProto.PreDispatchRequest) error
	SendPreDispatchResponse(ctx context.Context, node string, msg *engineProto.PreDispatchResponse) error
	SendPreDispatchRejection(ctx context.Context, node string, msg *engineProto.PreDispatchRejection) error
	SendDispatched(ctx context.Context, node string, msg *engineProto.TransactionDispatched) error
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

func (tw *transportWriter) marshalAndSend(ctx context.Context, node string, msgType string, msg proto.Message) error {
	msgBytes, err := protoMarshalFn(msg)
	if err != nil {
		log.L(ctx).Errorf("error marshalling %s: %s", msgType, err)
		return err
	}
	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: msgType,
		Payload:     msgBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        node,
	}); err != nil {
		log.L(ctx).Warnf("error sending %s to %s: %s", msgType, node, err)
	}
	return nil
}

func (tw *transportWriter) SendDelegationRequest(ctx context.Context, node string, msg *engineProto.DelegationRequest) error {
	return tw.marshalAndSend(ctx, node, MessageType_DelegationRequest, msg)
}

func (tw *transportWriter) SendDelegationResponse(ctx context.Context, node string, msg *engineProto.DelegationResponse) error {
	return tw.marshalAndSend(ctx, node, MessageType_DelegationResponse, msg)
}

func (tw *transportWriter) SendDelegationRejection(ctx context.Context, node string, msg *engineProto.DelegationRejection) error {
	return tw.marshalAndSend(ctx, node, MessageType_DelegationRejection, msg)
}

func (tw *transportWriter) SendHandoverRequest(ctx context.Context, node string, msg *engineProto.CoordinatorHandoverRequest) error {
	return tw.marshalAndSend(ctx, node, MessageType_HandoverRequest, msg)
}

func (tw *transportWriter) SendEndorsementRequest(ctx context.Context, node string, msg *engineProto.EndorsementRequest) error {
	return tw.marshalAndSend(ctx, node, MessageType_EndorsementRequest, msg)
}

func (tw *transportWriter) SendEndorsementResponse(ctx context.Context, node string, msg *engineProto.EndorsementResponse) error {
	return tw.marshalAndSend(ctx, node, MessageType_EndorsementResponse, msg)
}

func (tw *transportWriter) SendEndorsementError(ctx context.Context, node string, msg *engineProto.EndorsementError) error {
	return tw.marshalAndSend(ctx, node, MessageType_EndorsementError, msg)
}

func (tw *transportWriter) SendEndorsementRejection(ctx context.Context, node string, msg *engineProto.EndorsementRejection) error {
	return tw.marshalAndSend(ctx, node, MessageType_EndorsementRejection, msg)
}

func (tw *transportWriter) SendAssembleRequest(ctx context.Context, node string, msg *engineProto.AssembleRequest) error {
	return tw.marshalAndSend(ctx, node, MessageType_AssembleRequest, msg)
}

func (tw *transportWriter) SendAssembleResponse(ctx context.Context, node string, msg *engineProto.AssembleResponse) error {
	return tw.marshalAndSend(ctx, node, MessageType_AssembleResponse, msg)
}

func (tw *transportWriter) SendAssembleError(ctx context.Context, node string, msg *engineProto.AssembleError) error {
	return tw.marshalAndSend(ctx, node, MessageType_AssembleError, msg)
}

func (tw *transportWriter) SendAssembleRejection(ctx context.Context, node string, msg *engineProto.AssembleRejection) error {
	return tw.marshalAndSend(ctx, node, MessageType_AssembleRejection, msg)
}

func (tw *transportWriter) SendNonceAssigned(ctx context.Context, node string, msg *engineProto.NonceAssigned) error {
	return tw.marshalAndSend(ctx, node, MessageType_NonceAssigned, msg)
}

func (tw *transportWriter) SendTransactionSubmitted(ctx context.Context, node string, msg *engineProto.TransactionSubmitted) error {
	return tw.marshalAndSend(ctx, node, MessageType_TransactionSubmitted, msg)
}

func (tw *transportWriter) SendTransactionConfirmed(ctx context.Context, node string, msg *engineProto.TransactionConfirmed) error {
	return tw.marshalAndSend(ctx, node, MessageType_TransactionConfirmed, msg)
}

func (tw *transportWriter) SendHeartbeat(ctx context.Context, node string, msg *engineProto.CoordinatorHeartbeatNotification) error {
	return tw.marshalAndSend(ctx, node, MessageType_CoordinatorHeartbeatNotification, msg)
}

func (tw *transportWriter) SendPreDispatchRequest(ctx context.Context, node string, msg *engineProto.PreDispatchRequest) error {
	return tw.marshalAndSend(ctx, node, MessageType_PreDispatchRequest, msg)
}

func (tw *transportWriter) SendPreDispatchResponse(ctx context.Context, node string, msg *engineProto.PreDispatchResponse) error {
	return tw.marshalAndSend(ctx, node, MessageType_PreDispatchResponse, msg)
}

func (tw *transportWriter) SendPreDispatchRejection(ctx context.Context, node string, msg *engineProto.PreDispatchRejection) error {
	return tw.marshalAndSend(ctx, node, MessageType_PreDispatchRejection, msg)
}

func (tw *transportWriter) SendDispatched(ctx context.Context, node string, msg *engineProto.TransactionDispatched) error {
	return tw.marshalAndSend(ctx, node, MessageType_Dispatched, msg)
}

func (tw *transportWriter) send(ctx context.Context, payload *components.FireAndForgetMessageSend) error {
	if payload.Node == "" {
		err := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "attempt to send message without specifying destination node name")
		return err
	}

	log.L(ctx).Debugf("%s sent to %s", payload.MessageType, payload.Node)
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
