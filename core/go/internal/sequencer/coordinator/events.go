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
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
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

type HeartbeatReceivedEvent struct {
	common.BaseEvent
	transport.CoordinatorHeartbeatNotification
}

func (*HeartbeatReceivedEvent) Type() EventType {
	return Event_HeartbeatReceived
}

func (*HeartbeatReceivedEvent) TypeString() string {
	return "Event_HeartbeatReceived"
}

type NewBlockEvent struct {
	common.BaseEvent
	BlockHeight uint64
}

func (*NewBlockEvent) Type() EventType {
	return Event_NewBlock
}

func (*NewBlockEvent) TypeString() string {
	return "Event_NewBlock"
}
