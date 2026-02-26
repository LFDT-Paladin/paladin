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

package transaction

import (
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type Event interface {
	common.Event
	GetTransactionID() uuid.UUID
}

type BaseEvent struct {
	common.BaseEvent
	TransactionID uuid.UUID
}

func (e *BaseEvent) GetTransactionID() uuid.UUID {
	return e.TransactionID
}

type ConfirmedSuccessEvent struct {
	BaseEvent
}

func (*ConfirmedSuccessEvent) Type() EventType {
	return Event_ConfirmedSuccess
}

func (*ConfirmedSuccessEvent) TypeString() string {
	return "Event_ConfirmedSuccess"
}

type ConfirmedRevertedEvent struct {
	BaseEvent
	RevertReason pldtypes.HexBytes
}

func (*ConfirmedRevertedEvent) Type() EventType {
	return Event_ConfirmedReverted
}

func (*ConfirmedRevertedEvent) TypeString() string {
	return "Event_ConfirmedReverted"
}

type CreatedEvent struct {
	BaseEvent
	PrivateTransaction *components.PrivateTransaction
}

func (*CreatedEvent) Type() EventType {
	return Event_Created
}

func (*CreatedEvent) TypeString() string {
	return "Event_Created"
}

type DelegatedEvent struct {
	BaseEvent
	Coordinator string
}

func (*DelegatedEvent) Type() EventType {
	return Event_Delegated
}

func (*DelegatedEvent) TypeString() string {
	return "Event_Delegated"
}

type AssembleRequestReceivedEvent struct {
	BaseEvent
	RequestID               uuid.UUID
	Coordinator             string
	CoordinatorsBlockHeight int64
	StateLocksJSON          []byte
	PreAssembly             []byte
}

func (*AssembleRequestReceivedEvent) Type() EventType {
	return Event_AssembleRequestReceived
}

func (*AssembleRequestReceivedEvent) TypeString() string {
	return "Event_AssembleRequestReceived"
}

type AssembleAndSignSuccessEvent struct {
	BaseEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (*AssembleAndSignSuccessEvent) Type() EventType {
	return Event_AssembleAndSignSuccess
}

func (*AssembleAndSignSuccessEvent) TypeString() string {
	return "Event_AssembleAndSignSuccess"
}

type AssembleRevertEvent struct {
	BaseEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (*AssembleRevertEvent) Type() EventType {
	return Event_AssembleRevert
}

func (*AssembleRevertEvent) TypeString() string {
	return "Event_AssembleRevert"
}

type AssembleParkEvent struct {
	BaseEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (*AssembleParkEvent) Type() EventType {
	return Event_AssemblePark
}

func (*AssembleParkEvent) TypeString() string {
	return "Event_AssemblePark"
}

type AssembleErrorEvent struct {
	BaseEvent
}

func (*AssembleErrorEvent) Type() EventType {
	return Event_AssembleError
}

func (*AssembleErrorEvent) TypeString() string {
	return "Event_AssembleError"
}

type PreDispatchRequestReceivedEvent struct {
	BaseEvent
	RequestID        uuid.UUID
	Coordinator      string
	PostAssemblyHash *pldtypes.Bytes32
}

func (*PreDispatchRequestReceivedEvent) Type() EventType {
	return Event_PreDispatchRequestReceived
}

func (*PreDispatchRequestReceivedEvent) TypeString() string {
	return "Event_PreDispatchRequestReceived"
}

type CoordinatorChangedEvent struct {
	BaseEvent
	Coordinator string
}

func (*CoordinatorChangedEvent) Type() EventType {
	return Event_CoordinatorChanged
}

func (*CoordinatorChangedEvent) TypeString() string {
	return "Event_CoordinatorChanged"
}

type DispatchedEvent struct {
	BaseEvent
	SignerAddress pldtypes.EthAddress
}

func (*DispatchedEvent) Type() EventType {
	return Event_Dispatched
}

func (*DispatchedEvent) TypeString() string {
	return "Event_Dispatched"
}

type NonceAssignedEvent struct {
	BaseEvent
	SignerAddress pldtypes.EthAddress // include the signer address in case we never actually saw a dispatch event
	Nonce         uint64
}

func (*NonceAssignedEvent) Type() EventType {
	return Event_NonceAssigned
}

func (*NonceAssignedEvent) TypeString() string {
	return "Event_NonceAssigned"
}

type SubmittedEvent struct {
	BaseEvent
	SignerAddress        pldtypes.EthAddress // include the signer address and nonce in case we never actually saw a dispatch event or nonce assigned event
	Nonce                uint64
	LatestSubmissionHash pldtypes.Bytes32
}

func (*SubmittedEvent) Type() EventType {
	return Event_Submitted
}

func (*SubmittedEvent) TypeString() string {
	return "Event_Submitted"
}

type ResumedEvent struct {
	BaseEvent
}

func (*ResumedEvent) Type() EventType {
	return Event_Resumed
}

func (*ResumedEvent) TypeString() string {
	return "Event_Resumed"
}
