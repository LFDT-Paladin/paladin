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
	"context"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

// TestSyncHook is an interface for test-only events that need to signal when they've been
// dequeued by the event loop. This enables deterministic testing without time.Sleep.
// Events implementing this interface are automatically handled by EventLoopStateMachine
// and are NOT passed to OnEventReceived or ProcessEvent.
type TestSyncHook interface {
	NotifyProcessed()
}

// SyncEvent is a test-only event for synchronizing with the event loop.
// Queue it after another event and call Wait() to block until both have been dequeued.
type SyncEvent struct {
	processed chan struct{}
}

// NewSyncEvent creates a new sync event for test synchronization
func NewSyncEvent() *SyncEvent {
	return &SyncEvent{processed: make(chan struct{})}
}

func (e *SyncEvent) Type() common.EventType  { return -1 } // Not a real event type
func (e *SyncEvent) TypeString() string      { return "SyncEvent" }
func (e *SyncEvent) GetEventTime() time.Time { return time.Time{} }
func (e *SyncEvent) NotifyProcessed()        { close(e.processed) }

// Wait blocks until the sync event has been processed by the event loop
func (e *SyncEvent) Wait() { <-e.processed }

// EventLoopConfig holds configuration for the event loop
type EventLoopConfig[S State, T any] struct {
	// BufferSize for the event channel (default: 50)
	BufferSize int

	// OnEventReceived is called for every event before ProcessEvent.
	// Return (true, nil) to skip processing by this state machine.
	// Return (true, err) if routing/handling failed.
	// Return (false, nil) to continue with normal ProcessEvent.
	OnEventReceived func(ctx context.Context, subject T, event common.Event) (handled bool, err error)

	// OnError is called when event processing returns an error.
	OnError func(ctx context.Context, subject T, event common.Event, err error)

	// OnStop is called when the event loop is stopping (before the done channel closes).
	// This allows synchronous cleanup like processing a "closed" event.
	OnStop func(ctx context.Context, subject T) error
}

// EventLoopStateMachine wraps a StateMachine with a managed event loop
type EventLoopStateMachine[S State, T any] struct {
	*StateMachine[S, T]

	ctx     context.Context
	subject T
	config  EventLoopConfig[S, T]
	name    string // for logging

	events   chan common.Event
	stopLoop chan struct{}
	done     chan struct{}
}

// NewEventLoopStateMachine creates a new state machine with event loop support
func NewEventLoopStateMachine[S State, T any](
	ctx context.Context,
	name string,
	smConfig StateMachineConfig[S, T],
	elConfig EventLoopConfig[S, T],
	initialState S,
	subject T,
) *EventLoopStateMachine[S, T] {
	bufSize := elConfig.BufferSize
	if bufSize <= 0 {
		bufSize = 50
	}
	return &EventLoopStateMachine[S, T]{
		StateMachine: NewStateMachine(smConfig, initialState),
		ctx:          ctx,
		subject:      subject,
		config:       elConfig,
		name:         name,
		events:       make(chan common.Event, bufSize),
		stopLoop:     make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Start begins the event loop in a goroutine.
// Returns a channel that closes when the event loop has stopped.
func (sm *EventLoopStateMachine[S, T]) Start() <-chan struct{} {
	go sm.eventLoop()
	return sm.done
}

// QueueEvent adds an event to be processed by the event loop
func (sm *EventLoopStateMachine[S, T]) QueueEvent(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("%s: pushing event onto queue: %s", sm.name, event.TypeString())
	sm.events <- event
	log.L(ctx).Tracef("%s: pushed event onto queue: %s", sm.name, event.TypeString())
}

// Stop signals the event loop to stop and waits for completion.
// Safe to call multiple times.
func (sm *EventLoopStateMachine[S, T]) Stop() {
	select {
	case <-sm.done:
		return // already stopped
	default:
	}
	sm.stopLoop <- struct{}{}
	<-sm.done
}

// Done returns a channel that closes when the event loop has stopped
func (sm *EventLoopStateMachine[S, T]) Done() <-chan struct{} {
	return sm.done
}

func (sm *EventLoopStateMachine[S, T]) eventLoop() {
	defer close(sm.done)
	log.L(sm.ctx).Debugf("%s: event loop started", sm.name)

	for {
		select {
		case event := <-sm.events:
			// Test sync hooks are handled automatically - never passed to callbacks
			if syncHook, ok := event.(TestSyncHook); ok {
				syncHook.NotifyProcessed()
				continue
			}

			log.L(sm.ctx).Debugf("%s: pulled event from queue: %s", sm.name, event.TypeString())

			// OnEventReceived for routing/interception (e.g., routing to child state machines)
			if sm.config.OnEventReceived != nil {
				handled, err := sm.config.OnEventReceived(sm.ctx, sm.subject, event)
				if err != nil {
					log.L(sm.ctx).Errorf("%s: error in OnEventReceived for event %s: %v", sm.name, event.TypeString(), err)
					if sm.config.OnError != nil {
						sm.config.OnError(sm.ctx, sm.subject, event, err)
					}
				}
				if handled {
					continue
				}
			}

			// Process through own state machine
			err := sm.ProcessEvent(sm.ctx, sm.subject, event)
			if err != nil {
				if sm.config.OnError != nil {
					sm.config.OnError(sm.ctx, sm.subject, event, err)
				} else {
					log.L(sm.ctx).Errorf("%s: error processing event %s: %v", sm.name, event.TypeString(), err)
				}
			}

		case <-sm.stopLoop:
			// Allow synchronous cleanup on stop
			if sm.config.OnStop != nil {
				if err := sm.config.OnStop(sm.ctx, sm.subject); err != nil {
					log.L(sm.ctx).Errorf("%s: error in OnStop: %v", sm.name, err)
				}
			}
			log.L(sm.ctx).Debugf("%s: event loop stopped", sm.name)
			return
		}
	}
}
