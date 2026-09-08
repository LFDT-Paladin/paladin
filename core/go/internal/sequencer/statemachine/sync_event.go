/*
 * Copyright © 2026 Kaleido, Inc.
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

package statemachine

import (
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

// SyncEvent is a test-only event used to synchronize with the event loop.
// When processed by the event loop, it signals the Done channel, indicating
// that all previously queued events have been processed.
type SyncEvent struct {
	common.BaseEvent
	Done chan struct{}
}

// syncEventType is a special event type used internally for sync events.
// It uses a negative value to avoid conflicts with user-defined event types.
const syncEventType common.EventType = -1

func (*SyncEvent) Type() common.EventType {
	return syncEventType
}

func (*SyncEvent) TypeString() string {
	return "SyncEvent"
}

// NewSyncEvent creates a new SyncEvent with an initialized Done channel.
func NewSyncEvent() *SyncEvent {
	return &SyncEvent{
		Done: make(chan struct{}),
	}
}

// isSyncEvent returns true if the event is a SyncEvent.
func isSyncEvent(event common.Event) (*SyncEvent, bool) {
	sync, ok := event.(*SyncEvent)
	return sync, ok
}
