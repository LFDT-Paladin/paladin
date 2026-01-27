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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorStateEventActivated_Type(t *testing.T) {
	event := &CoordinatorStateEventActivated{}
	assert.Equal(t, Event_Activated, event.Type())
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

func TestCoordinatorClosedEvent_Type(t *testing.T) {
	event := &CoordinatorClosedEvent{}
	assert.Equal(t, Event_Closed, event.Type())
}

func TestCoordinatorClosedEvent_TypeString(t *testing.T) {
	event := &CoordinatorClosedEvent{}
	assert.Equal(t, "Event_Closed", event.TypeString())
}

func TestCoordinatorClosedEvent_GetEventTime(t *testing.T) {
	event := &CoordinatorClosedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestCoordinatorFlushedEvent_Type(t *testing.T) {
	event := &CoordinatorFlushedEvent{}
	assert.Equal(t, Event_Flushed, event.Type())
}

func TestCoordinatorFlushedEvent_TypeString(t *testing.T) {
	event := &CoordinatorFlushedEvent{}
	assert.Equal(t, "Event_Flushed", event.TypeString())
}

func TestTransactionConfirmedEvent_Type(t *testing.T) {
	event := &TransactionConfirmedEvent{}
	assert.Equal(t, Event_TransactionConfirmed, event.Type())
}

func TestTransactionConfirmedEvent_TypeString(t *testing.T) {
	event := &TransactionConfirmedEvent{}
	assert.Equal(t, "Event_TransactionConfirmed", event.TypeString())
}

func TestTransactionConfirmedEvent_GetEventTime(t *testing.T) {
	event := &TransactionConfirmedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestTransactionConfirmedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	from := pldtypes.RandAddress()
	nonce := pldtypes.HexUint64(42)
	hash := pldtypes.RandBytes32()
	revertReason := pldtypes.HexBytes{0x01, 0x02, 0x03}

	event := &TransactionConfirmedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		TxID:         txID,
		From:         from,
		Nonce:        &nonce,
		Hash:         hash,
		RevertReason: revertReason,
	}

	assert.Equal(t, txID, event.TxID)
	assert.Equal(t, from, event.From)
	require.NotNil(t, event.Nonce, "Nonce should be set")
	assert.Equal(t, uint64(42), event.Nonce.Uint64())
	assert.Equal(t, hash, event.Hash)
	assert.Equal(t, revertReason, event.RevertReason)
}

func TestTransactionDispatchConfirmedEvent_Type(t *testing.T) {
	event := &TransactionDispatchConfirmedEvent{}
	assert.Equal(t, Event_TransactionDispatchConfirmed, event.Type())
}

func TestTransactionDispatchConfirmedEvent_TypeString(t *testing.T) {
	event := &TransactionDispatchConfirmedEvent{}
	assert.Equal(t, "Event_TransactionDispatchConfirmed", event.TypeString())
}

func TestTransactionDispatchConfirmedEvent_GetEventTime(t *testing.T) {
	event := &TransactionDispatchConfirmedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestTransactionDispatchConfirmedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &TransactionDispatchConfirmedEvent{
		TransactionID: txID,
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestEndorsementRequestedEvent_Type(t *testing.T) {
	event := &EndorsementRequestedEvent{}
	assert.Equal(t, Event_EndorsementRequested, event.Type())
}

func TestEndorsementRequestedEvent_TypeString(t *testing.T) {
	event := &EndorsementRequestedEvent{}
	assert.Equal(t, "Event_EndorsementRequested", event.TypeString())
}

func TestEndorsementRequestedEvent_GetEventTime(t *testing.T) {
	event := &EndorsementRequestedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestEndorsementRequestedEvent_Fields(t *testing.T) {
	from := "test@testNode"
	event := &EndorsementRequestedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		From: from,
	}
	assert.Equal(t, from, event.From)
}

func TestHeartbeatReceivedEvent_Type(t *testing.T) {
	event := &HeartbeatReceivedEvent{}
	assert.Equal(t, Event_HeartbeatReceived, event.Type())
}

func TestHeartbeatReceivedEvent_TypeString(t *testing.T) {
	event := &HeartbeatReceivedEvent{}
	assert.Equal(t, "Event_HeartbeatReceived", event.TypeString())
}

func TestHeartbeatReceivedEvent_GetEventTime(t *testing.T) {
	event := &HeartbeatReceivedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestHeartbeatReceivedEvent_EmbeddedNotification(t *testing.T) {
	contractAddress := pldtypes.RandAddress()
	notification := transport.CoordinatorHeartbeatNotification{
		From:            "coordinator@node",
		ContractAddress: contractAddress,
	}
	notification.BlockHeight = 100
	event := &HeartbeatReceivedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		CoordinatorHeartbeatNotification: notification,
	}
	assert.Equal(t, notification.From, event.From)
	assert.Equal(t, notification.ContractAddress, event.ContractAddress)
	assert.Equal(t, uint64(100), event.BlockHeight)
}

func TestHandoverRequestEvent_Type(t *testing.T) {
	event := &HandoverRequestEvent{}
	assert.Equal(t, Event_HandoverRequestReceived, event.Type())
}

func TestHandoverRequestEvent_TypeString(t *testing.T) {
	event := &HandoverRequestEvent{}
	assert.Equal(t, "Event_HandoverRequestReceived", event.TypeString())
}

func TestHandoverRequestEvent_GetEventTime(t *testing.T) {
	event := &HandoverRequestEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestHandoverRequestEvent_Fields(t *testing.T) {
	requester := "requester@testNode"
	event := &HandoverRequestEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		Requester: requester,
	}
	assert.Equal(t, requester, event.Requester)
}

func TestNewBlockEvent_Type(t *testing.T) {
	event := &NewBlockEvent{}
	assert.Equal(t, Event_NewBlock, event.Type())
}

func TestNewBlockEvent_TypeString(t *testing.T) {
	event := &NewBlockEvent{}
	assert.Equal(t, "Event_NewBlock", event.TypeString())
}

func TestNewBlockEvent_GetEventTime(t *testing.T) {
	event := &NewBlockEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestNewBlockEvent_Fields(t *testing.T) {
	blockHeight := uint64(200)
	event := &NewBlockEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Now(),
		},
		BlockHeight: blockHeight,
	}
	assert.Equal(t, blockHeight, event.BlockHeight)
}

func TestHandoverReceivedEvent_Type(t *testing.T) {
	event := &HandoverReceivedEvent{}
	assert.Equal(t, Event_HandoverReceived, event.Type())
}

func TestHandoverReceivedEvent_TypeString(t *testing.T) {
	event := &HandoverReceivedEvent{}
	assert.Equal(t, "Event_HandoverReceived", event.TypeString())
}

func TestHandoverReceivedEvent_GetEventTime(t *testing.T) {
	event := &HandoverReceivedEvent{
		BaseEvent: common.BaseEvent{
			EventTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), event.GetEventTime())
}

func TestTransactionStateTransitionEvent_Type(t *testing.T) {
	event := &TransactionStateTransitionEvent{}
	assert.Equal(t, Event_TransactionStateTransition, event.Type())
}

func TestTransactionStateTransitionEvent_TypeString(t *testing.T) {
	event := &TransactionStateTransitionEvent{}
	assert.Equal(t, "Event_TransactionStateTransition", event.TypeString())
}

func TestTransactionStateTransitionEvent_Fields(t *testing.T) {
	txID := uuid.New()
	fromState := transaction.State_Pooled
	toState := transaction.State_Ready_For_Dispatch

	event := &TransactionStateTransitionEvent{
		TransactionID: txID,
		From:          fromState,
		To:            toState,
	}

	assert.Equal(t, txID, event.TransactionID)
	assert.Equal(t, fromState, event.From)
	assert.Equal(t, toState, event.To)
}

func TestEvent_InterfaceCompliance(t *testing.T) {
	// Test that all events with BaseEvent implement the Event interface
	events := []Event{
		&TransactionsDelegatedEvent{},
		&CoordinatorClosedEvent{},
		&TransactionConfirmedEvent{},
		&TransactionDispatchConfirmedEvent{},
		&EndorsementRequestedEvent{},
		&HeartbeatReceivedEvent{},
		&HandoverRequestEvent{},
		&NewBlockEvent{},
		&HandoverReceivedEvent{},
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

func TestCoordinatorStateEventActivated_TypeAndTypeString(t *testing.T) {
	event := &CoordinatorStateEventActivated{}
	assert.Equal(t, Event_Activated, event.Type())
	assert.Equal(t, "Event_Activated", event.TypeString())
}

func TestCoordinatorFlushedEvent_TypeAndTypeString(t *testing.T) {
	event := &CoordinatorFlushedEvent{}
	assert.Equal(t, Event_Flushed, event.Type())
	assert.Equal(t, "Event_Flushed", event.TypeString())
}

func TestTransactionStateTransitionEvent_TypeAndTypeString(t *testing.T) {
	event := &TransactionStateTransitionEvent{}
	assert.Equal(t, Event_TransactionStateTransition, event.Type())
	assert.Equal(t, "Event_TransactionStateTransition", event.TypeString())
}
