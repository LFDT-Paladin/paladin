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
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
)

type Event interface {
	common.Event
}

type TransactionCreatedEvent struct {
	common.BaseEvent
	Transaction         *components.PrivateTransaction
	ResolvedTransaction *components.ResolvedTransaction
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

type DelegationRequestRejectedEvent struct {
	common.BaseEvent
	RejectionReason        engineProto.RejectionReason
	ActiveCoordinator      string
	OriginatorBlockHeight  int64
	CoordinatorBlockHeight int64
	BlockHeightTolerance   int64
}

func (*DelegationRequestRejectedEvent) Type() EventType {
	return Event_DelegationRequestRejected
}

func (*DelegationRequestRejectedEvent) TypeString() string {
	return "Event_DelegationRequestRejected"
}
