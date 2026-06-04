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
)

type Event interface {
	common.Event
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

type OriginatorCreatedEvent struct {
	common.BaseEvent
}

func (*OriginatorCreatedEvent) Type() EventType {
	return Event_OriginatorCreated
}

func (*OriginatorCreatedEvent) TypeString() string {
	return "Event_OriginatorCreated"
}

// DelegationRejectedEvent carries the name of the coordinator that the rejecting node believes
// is currently active so the originator can fast-redirect to a higher-priority coordinator.
type DelegationRejectedEvent struct {
	common.BaseEvent
	ActiveCoordinator string
}

func (*DelegationRejectedEvent) Type() EventType {
	return Event_DelegationRejected
}

func (*DelegationRejectedEvent) TypeString() string {
	return "Event_DelegationRejected"
}
