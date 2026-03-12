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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestBaseEvent_GetEventTime(t *testing.T) {
	now := time.Now()
	event := &BaseEvent{
		EventTime: now,
	}

	result := event.GetEventTime()
	assert.Equal(t, now, result)
}

func TestBaseEvent_GetEventTime_ZeroTime(t *testing.T) {
	event := &BaseEvent{
		EventTime: time.Time{},
	}

	result := event.GetEventTime()
	assert.True(t, result.IsZero())
}

func TestHeartbeatIntervalEvent_Type(t *testing.T) {
	event := &HeartbeatIntervalEvent{}

	result := event.Type()
	assert.Equal(t, Event_HeartbeatInterval, result)
}

func TestHeartbeatIntervalEvent_TypeString(t *testing.T) {
	event := &HeartbeatIntervalEvent{}

	result := event.TypeString()
	assert.Equal(t, "Event_HeartbeatInterval", result)
}

func TestHeartbeatIntervalEvent_GetEventTime(t *testing.T) {
	now := time.Now()
	event := &HeartbeatIntervalEvent{
		BaseEvent: BaseEvent{
			EventTime: now,
		},
	}

	result := event.GetEventTime()
	assert.Equal(t, now, result)
}

func TestHeartbeatIntervalEvent_ImplementsEventInterface(t *testing.T) {
	// This test verifies that HeartbeatIntervalEvent implements the Event interface
	var _ Event = (*HeartbeatIntervalEvent)(nil)

	now := time.Now()
	event := &HeartbeatIntervalEvent{
		BaseEvent: BaseEvent{
			EventTime: now,
		},
	}

	// Test all interface methods
	assert.Equal(t, Event_HeartbeatInterval, event.Type())
	assert.Equal(t, "Event_HeartbeatInterval", event.TypeString())
	assert.Equal(t, now, event.GetEventTime())
}

func TestEventType_HeartbeatInterval(t *testing.T) {
	// Verify that Event_HeartbeatInterval has the expected value (0 for iota)
	assert.Equal(t, EventType(0), Event_HeartbeatInterval)
}

func TestTransactionStateTransitionEvent_Type(t *testing.T) {
	event := &TransactionStateTransitionEvent[int]{}

	result := event.Type()
	assert.Equal(t, Event_TransactionStateTransition, result)
}

func TestTransactionStateTransitionEvent_TypeString(t *testing.T) {
	event := &TransactionStateTransitionEvent[int]{}

	result := event.TypeString()
	assert.Equal(t, "Event_TransactionStateTransition", result)
}

func TestTransactionStateTransitionEvent_GetEventTime(t *testing.T) {
	now := time.Now()
	event := &TransactionStateTransitionEvent[int]{
		BaseEvent:     BaseEvent{EventTime: now},
		TransactionID: uuid.New(),
		From:          0,
		To:            1,
	}

	result := event.GetEventTime()
	assert.Equal(t, now, result)
}

func TestTransactionStateTransitionEvent_ImplementsEventInterface(t *testing.T) {
	// This test verifies that TransactionStateTransitionEvent implements the Event interface
	var _ Event = (*TransactionStateTransitionEvent[int])(nil)

	now := time.Now()
	txID := uuid.New()
	event := &TransactionStateTransitionEvent[int]{
		BaseEvent:     BaseEvent{EventTime: now},
		TransactionID: txID,
		From:          0,
		To:            1,
	}

	assert.Equal(t, Event_TransactionStateTransition, event.Type())
	assert.Equal(t, "Event_TransactionStateTransition", event.TypeString())
	assert.Equal(t, now, event.GetEventTime())
	assert.Equal(t, txID, event.TransactionID)
	assert.Equal(t, 0, event.From)
	assert.Equal(t, 1, event.To)
}
