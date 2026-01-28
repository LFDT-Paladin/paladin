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

package common

import (
	"time"

	"github.com/google/uuid"
)

type EventType int

// function that can be used to emit events from the internals of the sequencer to feed back into the state machine
type EmitEvent func(event Event)

const (
	Event_HeartbeatInterval          EventType = iota // emitted on a regular basis, interval defined by the sequencer config a
	Event_TransactionStateTransition                  // transaction state machine transition; originator/coordinator handle cleanup and side effects
)

type BaseEvent struct {
	EventTime time.Time
}

func (e *BaseEvent) GetEventTime() time.Time {
	return e.EventTime
}

type Event interface {
	Type() EventType
	TypeString() string
	GetEventTime() time.Time
}

type HeartbeatIntervalEvent struct {
	BaseEvent
}

func (*HeartbeatIntervalEvent) Type() EventType {
	return Event_HeartbeatInterval
}

func (*HeartbeatIntervalEvent) TypeString() string {
	return "Event_HeartbeatInterval"
}

// TransactionStateTransitionEvent is queued by a transaction state machine when it transitions state.
// S is the state type (comparable, typically int-based enum); each state machine uses its own State type parameter.
type TransactionStateTransitionEvent[S comparable] struct {
	BaseEvent
	TransactionID uuid.UUID
	From          S
	To            S
}

func (*TransactionStateTransitionEvent[S]) Type() EventType {
	return Event_TransactionStateTransition
}

func (*TransactionStateTransitionEvent[S]) TypeString() string {
	return "Event_TransactionStateTransition"
}
