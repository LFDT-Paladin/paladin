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

package coordinator

import (
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type Event interface {
	common.Event
}

type CoordinatorCreatedEvent struct {
	common.BaseEvent
}

func (*CoordinatorCreatedEvent) Type() EventType {
	return Event_CoordinatorCreated
}

func (*CoordinatorCreatedEvent) TypeString() string {
	return "Event_CoordinatorCreated"
}

type TransactionsDelegatedEvent struct {
	common.BaseEvent
	FromNode               string // Node name that sent the delegation request
	Originator             string // Fully qualified identity locator for the originator
	Transactions           []*components.PrivateTransaction
	OriginatorsBlockHeight uint64
	DelegationID           string
}

func (*TransactionsDelegatedEvent) Type() EventType {
	return Event_TransactionsDelegated
}

func (*TransactionsDelegatedEvent) TypeString() string {
	return "Event_TransactionsDelegated"
}

// RequestTimeoutIntervalEvent is fired by the request-timeout timer to trigger a re-send of the
// pending outbound request for the current state (e.g. HandoverRequest while in State_Elect).
type RequestTimeoutIntervalEvent struct {
	common.BaseEvent
}

func (*RequestTimeoutIntervalEvent) Type() EventType { return Event_RequestTimeoutInterval }
func (*RequestTimeoutIntervalEvent) TypeString() string {
	return "Event_RequestTimeoutInterval"
}

// StateTimeoutIntervalEvent is fired by the state-timeout timer to signal that the current state
// has exceeded its maximum wait duration (e.g. Elect → Active when no handover acknowledgement arrives).
type StateTimeoutIntervalEvent struct {
	common.BaseEvent
}

func (*StateTimeoutIntervalEvent) Type() EventType { return Event_StateTimeoutInterval }
func (*StateTimeoutIntervalEvent) TypeString() string {
	return "Event_StateTimeoutInterval"
}

// HandoverRequestEvent is queued by the transport client to the coordinator when a
// CoordinatorHandoverRequest message is received from a higher-priority node. The coordinator
// in Active state handles it identically to a preemption heartbeat.
type HandoverRequestEvent struct {
	common.BaseEvent
	FromNode string
}

func (*HandoverRequestEvent) Type() EventType { return Event_HandoverRequest }
func (*HandoverRequestEvent) TypeString() string {
	return "Event_HandoverRequest"
}

// RestartDispatchLoopEvent is queued internally during an in-place key rotation so that the
// dispatch loop is restarted in a subsequent event-processing step, after any pending
// TransactionStateTransitionEvents have been processed and c.inFlightTxns is up to date.
type RestartDispatchLoopEvent struct {
	common.BaseEvent
}

func (*RestartDispatchLoopEvent) Type() EventType { return Event_RestartDispatchLoop }
func (*RestartDispatchLoopEvent) TypeString() string {
	return "Event_RestartDispatchLoop"
}

type EndorsementRequestReceivedEvent struct {
	common.BaseEvent
	FromNode                  string
	TransactionId             string
	IdempotencyKey            string
	Party                     string
	PrivateEndorsementRequest *components.PrivateTransactionEndorseRequest
	AttestationRequest        *prototk.AttestationRequest
	Expiry                    time.Time
	CoordinatorBlockHeight    int64
	BlockHeightTolerance      int64
}

func (*EndorsementRequestReceivedEvent) Type() EventType {
	return Event_EndorsementRequestReceived
}

func (*EndorsementRequestReceivedEvent) TypeString() string {
	return "Event_EndorsementRequestReceived"
}
