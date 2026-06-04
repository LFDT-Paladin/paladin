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
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCoordinatorCreatedEvent_Type(t *testing.T) {
	event := &CoordinatorCreatedEvent{}
	assert.Equal(t, Event_CoordinatorCreated, event.Type())
}

func TestTransactionsDelegatedEvent_Type(t *testing.T) {
	event := &TransactionsDelegatedEvent{}
	assert.Equal(t, Event_TransactionsDelegated, event.Type())
}

func TestTransactionsDelegatedEvent_TypeString(t *testing.T) {
	event := &TransactionsDelegatedEvent{}
	assert.Equal(t, "Event_TransactionsDelegated", event.TypeString())
}

func TestTransactionsDelegatedEvent_GetEventTime(t *testing.T) {
	event := &TransactionsDelegatedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestTransactionsDelegatedEvent_Fields(t *testing.T) {
	fromNode := "testNode"
	originator := "test@testNode"
	blockHeight := uint64(100)
	txID := uuid.New()
	transactions := []*components.PrivateTransaction{
		{ID: txID},
	}

	event := &TransactionsDelegatedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		FromNode:               fromNode,
		Originator:             originator,
		Transactions:           transactions,
		OriginatorsBlockHeight: blockHeight,
	}

	assert.Equal(t, fromNode, event.FromNode)
	assert.Equal(t, originator, event.Originator)
	assert.Equal(t, transactions, event.Transactions)
	assert.Equal(t, blockHeight, event.OriginatorsBlockHeight)
	assert.Equal(t, txID, event.Transactions[0].ID)
}

func TestNewBlockEvent_Type(t *testing.T) {
	event := &common.NewBlockEvent{}
	assert.Equal(t, common.Event_NewBlock, event.Type())
}

func TestNewBlockEvent_TypeString(t *testing.T) {
	event := &common.NewBlockEvent{}
	assert.Equal(t, "Event_NewBlock", event.TypeString())
}

func TestNewBlockEvent_GetEventTime(t *testing.T) {
	event := &common.NewBlockEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestNewBlockEvent_Fields(t *testing.T) {
	blockHeight := uint64(200)
	event := &common.NewBlockEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		BlockHeight: blockHeight,
	}
	assert.Equal(t, blockHeight, event.BlockHeight)
}

func TestEvent_InterfaceCompliance(t *testing.T) {
	// Test that all events with BaseEvent implement the Event interface
	events := []Event{
		&CoordinatorCreatedEvent{},
		&TransactionsDelegatedEvent{},
	}

	for _, event := range events {
		// Verify that Type() returns a valid EventType
		eventType := event.Type()
		assert.NotNil(t, eventType)

		// Verify that TypeString() returns a non-empty string
		typeString := event.TypeString()
		assert.NotEmpty(t, typeString)

		// Verify that GetEventTime() is callable
		_ = event.GetEventTime()
	}
}

func TestCoordinatorCreatedEvent_TypeAndTypeString(t *testing.T) {
	event := &CoordinatorCreatedEvent{}
	assert.Equal(t, Event_CoordinatorCreated, event.Type())
	assert.Equal(t, "Event_CoordinatorCreated", event.TypeString())
}
