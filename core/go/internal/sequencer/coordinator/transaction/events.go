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
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type Event interface {
	common.Event
	GetTransactionID() uuid.UUID
}

type BaseCoordinatorEvent struct {
	common.BaseEvent
	TransactionID uuid.UUID
}

func (e *BaseCoordinatorEvent) GetTransactionID() uuid.UUID {
	return e.TransactionID
}

// TransactionDelegatedEvent is "emitted" when the coordinator receives a transaction.
// Feels slightly artificial to model this as an event because it happens every time we create a transaction object
// but rather than bury the logic in NewTransaction func, modeling this event allows us to define the initial state transition rules in the same declarative stateDefinitions structure as all other state transitions
type DelegatedEvent struct {
	BaseCoordinatorEvent
}

func (*DelegatedEvent) Type() EventType {
	return Event_Delegated
}

func (*DelegatedEvent) TypeString() string {
	return "Event_Delegated"
}

type SelectedEvent struct {
	BaseCoordinatorEvent
}

func (*SelectedEvent) Type() EventType {
	return Event_Selected
}

func (*SelectedEvent) TypeString() string {
	return "Event_Selected"
}

type AssembleCancelledEvent struct {
	BaseCoordinatorEvent
}

func (*AssembleCancelledEvent) Type() EventType {
	return Event_AssembleCancelled
}

func (*AssembleCancelledEvent) TypeString() string {
	return "Event_AssembleCancelled"
}

type AssembleRequestSentEvent struct {
	BaseCoordinatorEvent
}

func (*AssembleRequestSentEvent) Type() EventType {
	return Event_AssembleRequestSent
}

func (*AssembleRequestSentEvent) TypeString() string {
	return "Event_AssembleRequestSent"
}

type AssembleSuccessEvent struct {
	BaseCoordinatorEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (*AssembleSuccessEvent) Type() EventType {
	return Event_AssembleSuccess
}

func (*AssembleSuccessEvent) TypeString() string {
	return "Event_AssembleSuccess"
}

type AssembleRevertEvent struct {
	BaseCoordinatorEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (*AssembleRevertEvent) Type() EventType {
	return Event_AssembleRevert
}

func (*AssembleRevertEvent) TypeString() string {
	return "Event_AssembleRevert"
}

type AssembleErrorEvent struct {
	BaseCoordinatorEvent
	RequestID uuid.UUID
}

func (*AssembleErrorEvent) Type() EventType {
	return Event_AssembleError
}

func (*AssembleErrorEvent) TypeString() string {
	return "Event_AssembleError"
}

type EndorsedEvent struct {
	BaseCoordinatorEvent
	Endorsement *prototk.AttestationResult
	RequestID   uuid.UUID
}

func (*EndorsedEvent) Type() EventType {
	return Event_Endorsed
}

func (*EndorsedEvent) TypeString() string {
	return "Event_Endorsed"
}

type EndorseRevertEvent struct {
	BaseCoordinatorEvent
	Party                  string
	RevertReason           string
	AttestationRequestName string
	RequestID              uuid.UUID
}

func (*EndorseRevertEvent) Type() EventType {
	return Event_EndorseRevert
}

func (*EndorseRevertEvent) TypeString() string {
	return "Event_EndorseRevert"
}

// EndorseErrorEvent is queued when an endorser encountered an unexpected error processing the
// request (domain error, key resolution failure, etc.).
type EndorseErrorEvent struct {
	BaseCoordinatorEvent
	RequestID              uuid.UUID
	Party                  string
	AttestationRequestName string
}

func (*EndorseErrorEvent) Type() EventType {
	return Event_EndorseError
}

func (*EndorseErrorEvent) TypeString() string {
	return "Event_EndorseError"
}

// EndorseRequestRejectedEvent is queued when an endorser rejected the request before even attempting
// it — currently only block height tolerance.
type EndorseRequestRejectedEvent struct {
	BaseCoordinatorEvent
	Party                  string
	AttestationRequestName string
	RequestID              uuid.UUID
	RejectionReason        engineProto.RejectionReason
	CoordinatorBlockHeight int64
	EndorserBlockHeight    int64
	BlockHeightTolerance   int64
}

func (*EndorseRequestRejectedEvent) Type() EventType {
	return Event_EndorseRequestRejected
}

func (*EndorseRequestRejectedEvent) TypeString() string {
	return "Event_EndorseRequestRejected"
}

type AssembleRequestRejectedEvent struct {
	BaseCoordinatorEvent
	RequestID              uuid.UUID
	RejectionReason        engineProto.RejectionReason
	CoordinatorBlockHeight int64
	AssemblerBlockHeight   int64
	BlockHeightTolerance   int64
}

func (*AssembleRequestRejectedEvent) Type() EventType {
	return Event_AssembleRequestRejected
}

func (*AssembleRequestRejectedEvent) TypeString() string {
	return "Event_AssembleRequestRejected"
}

type DispatchRequestApprovedEvent struct {
	BaseCoordinatorEvent
	RequestID uuid.UUID
}

func (*DispatchRequestApprovedEvent) Type() EventType {
	return Event_DispatchRequestApproved
}

func (*DispatchRequestApprovedEvent) TypeString() string {
	return "Event_DispatchRequestApproved"
}

// Collected by the public transaction manager after being dispatched
type CollectedEvent struct {
	BaseCoordinatorEvent
	SignerAddress pldtypes.EthAddress
}

func (*CollectedEvent) Type() EventType {
	return Event_Collected
}

func (*CollectedEvent) TypeString() string {
	return "Event_Collected"
}

// Collected by the dispatcher thread and dispatched to the public transaction manager
type DispatchedEvent struct {
	BaseCoordinatorEvent
}

func (*DispatchedEvent) Type() EventType {
	return Event_Dispatched
}

func (*DispatchedEvent) TypeString() string {
	return "Event_Dispatched"
}

type NonceAllocatedEvent struct {
	BaseCoordinatorEvent
	Nonce uint64
}

func (*NonceAllocatedEvent) Type() EventType {
	return Event_NonceAllocated
}

func (*NonceAllocatedEvent) TypeString() string {
	return "Event_NonceAllocated"
}

type SubmittedEvent struct {
	BaseCoordinatorEvent
	SubmissionHash pldtypes.Bytes32
}

func (*SubmittedEvent) Type() EventType {
	return Event_Submitted
}

func (*SubmittedEvent) TypeString() string {
	return "Event_Submitted"
}

type ConfirmedSuccessEvent struct {
	BaseCoordinatorEvent
	Nonce   *pldtypes.HexUint64
	Hash    pldtypes.Bytes32
	OnChain pldtypes.OnChainLocation
}

func (*ConfirmedSuccessEvent) Type() EventType {
	return Event_ConfirmedSuccess
}

func (*ConfirmedSuccessEvent) TypeString() string {
	return "Event_ConfirmedSuccess"
}

type ConfirmedRevertedEvent struct {
	BaseCoordinatorEvent
	Nonce          *pldtypes.HexUint64
	Hash           pldtypes.Bytes32
	FailureMessage string
	RevertReason   pldtypes.HexBytes
	OnChain        pldtypes.OnChainLocation
}

func (*ConfirmedRevertedEvent) Type() EventType {
	return Event_ConfirmedReverted
}

func (*ConfirmedRevertedEvent) TypeString() string {
	return "Event_ConfirmedReverted"
}

type DependencySelectedForAssemblyEvent struct {
	BaseCoordinatorEvent
	SourceTransactionID uuid.UUID // The dependency that was selected
}

func (*DependencySelectedForAssemblyEvent) Type() EventType {
	return Event_DependencySelectedForAssemble
}

func (*DependencySelectedForAssemblyEvent) TypeString() string {
	return "Event_DependencySelectedForAssembly"
}

type DependencyResetEvent struct {
	BaseCoordinatorEvent
	SourceTransactionID uuid.UUID // The dependency that was reset
}

func (*DependencyResetEvent) Type() EventType {
	return Event_DependencyReset
}

func (*DependencyResetEvent) TypeString() string {
	return "Event_DependencyReset"
}

type DependencyConfirmedRevertedEvent struct {
	BaseCoordinatorEvent
	SourceTransactionID uuid.UUID // The dependency that was confirmed as reverted
}

func (*DependencyConfirmedRevertedEvent) Type() EventType {
	return Event_DependencyConfirmedReverted
}

func (*DependencyConfirmedRevertedEvent) TypeString() string {
	return "Event_DependencyConfirmedReverted"
}

type DependencyReadyEvent struct {
	BaseCoordinatorEvent
}

func (*DependencyReadyEvent) Type() EventType {
	return Event_DependencyReady
}

func (*DependencyReadyEvent) TypeString() string {
	return "Event_DependencyReady"
}

type RequestTimeoutIntervalEvent struct {
	BaseCoordinatorEvent
}

func (*RequestTimeoutIntervalEvent) Type() EventType {
	return Event_RequestTimeoutInterval
}

func (*RequestTimeoutIntervalEvent) TypeString() string {
	return "Event_RequestTimeoutInterval"
}

type StateTimeoutIntervalEvent struct {
	BaseCoordinatorEvent
}

func (*StateTimeoutIntervalEvent) Type() EventType {
	return Event_StateTimeoutInterval
}

func (*StateTimeoutIntervalEvent) TypeString() string {
	return "Event_StateTimeoutInterval"
}

// events emitted by the transaction state machine whenever a state transition occurs
type StateTransitionEvent struct {
	BaseCoordinatorEvent
	FromState State
	ToState   State
}

func (*StateTransitionEvent) Type() EventType {
	return Event_StateTransition
}

func (*StateTransitionEvent) TypeString() string {
	return "Event_StateTransition"
}

type PreDispatchRequestRejectedEvent struct {
	BaseCoordinatorEvent
	RequestID       uuid.UUID
	RejectionReason engineProto.RejectionReason
}

func (*PreDispatchRequestRejectedEvent) Type() EventType {
	return Event_PreDispatchRequestRejected
}

func (*PreDispatchRequestRejectedEvent) TypeString() string {
	return "Event_PreDispatchRequestRejected"
}

type ChainedDependencyFailedEvent struct {
	BaseCoordinatorEvent
	FailedTxID uuid.UUID
}

func (*ChainedDependencyFailedEvent) Type() EventType {
	return Event_ChainedDependencyFailed
}

func (*ChainedDependencyFailedEvent) TypeString() string {
	return "Event_ChainedDependencyFailed"
}

type ChainedDependencyEvictedEvent struct {
	BaseCoordinatorEvent
	EvictedTxID uuid.UUID
}

func (*ChainedDependencyEvictedEvent) Type() EventType {
	return Event_ChainedDependencyEvicted
}

func (*ChainedDependencyEvictedEvent) TypeString() string {
	return "Event_ChainedDependencyEvicted"
}

type PreAssembleDependencyTerminatedEvent struct {
	BaseCoordinatorEvent
}

func (*PreAssembleDependencyTerminatedEvent) Type() EventType {
	return Event_PreAssembleDependencyTerminated
}

func (*PreAssembleDependencyTerminatedEvent) TypeString() string {
	return "Event_PreAssembleDependencyTerminated"
}
