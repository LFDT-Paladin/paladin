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

package originator

import (
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
)

type Event interface {
	common.Event
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

type TransactionCreatedEvent struct {
	common.BaseEvent
	Transaction *components.PrivateTransaction
}

func (*TransactionCreatedEvent) Type() EventType {
	return Event_TransactionCreated
}

func (*TransactionCreatedEvent) TypeString() string {
	return "Event_TransactionCreated"
}

type ActiveCoordinatorUpdatedEvent struct {
	common.BaseEvent
	Coordinator string
}

func (*ActiveCoordinatorUpdatedEvent) Type() EventType {
	return Event_ActiveCoordinatorUpdated
}

func (*ActiveCoordinatorUpdatedEvent) TypeString() string {
	return "Event_ActiveCoordinatorUpdated"
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
