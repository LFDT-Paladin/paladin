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
type TestSyncHook interface {
	NotifyProcessed()
}

// SyncEvent is a test-only event for synchronizing with the event loop.
type SyncEvent struct {
	processed chan struct{}
}

// NewSyncEvent creates a new sync event for test synchronization
func NewSyncEvent() *SyncEvent {
	return &SyncEvent{processed: make(chan struct{})}
}

func (e *SyncEvent) Type() common.EventType  { return -1 }
func (e *SyncEvent) TypeString() string      { return "SyncEvent" }
func (e *SyncEvent) GetEventTime() time.Time { return time.Time{} }
func (e *SyncEvent) NotifyProcessed()        { close(e.processed) }

// Wait blocks until the sync event has been processed by the event loop
func (e *SyncEvent) Wait() { <-e.processed }

// EventLoopConfig holds configuration for the event loop
// T is the subject type that implements Subject[S, M, R, Cfg, Cb]
// TODO AM: why is this getting the subject in addition to the mutator and reader? - I suspect that if designed properly
// the event loop should not need the mutator
type EventLoopConfig[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any, T Subject[S, M, R, Cfg, Cb]] struct {
	// BufferSize for the event channel (default: 50)
	BufferSize int

	// OnEventReceived is called for every event before ProcessEvent.
	// Return (true, nil) to skip processing by this state machine.
	// Return (true, err) if routing/handling failed.
	// Return (false, nil) to continue with normal ProcessEvent.
	// OnEventReceived does not receive the state mutator as it must not modify state outside the state machine.
	OnEventReceived func(ctx context.Context, reader R, config Cfg, callbacks Cb, event common.Event) (handled bool, err error)

	// TODO AM: if these functions stay they must not be passed the full subject necessarily
	// OnError is called when event processing returns an error.
	OnError func(ctx context.Context, subject T, event common.Event, err error)

	// OnStop is called when the event loop is stopping.
	OnStop func(ctx context.Context, subject T) error
}

// EventLoopStateMachine wraps a StateMachine with a managed event loop
// T is the subject type that implements Subject[S, M, R, Cfg, Cb]
type EventLoopStateMachine[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any, T Subject[S, M, R, Cfg, Cb]] struct {
	*StateMachine[S, M, R, Cfg, Cb]

	ctx      context.Context
	subject  T
	elConfig EventLoopConfig[S, M, R, Cfg, Cb, T]
	name     string

	events   chan common.Event
	stopLoop chan struct{}
	done     chan struct{}
	started  bool
}

// NewEventLoopStateMachine creates a new state machine with event loop support
func NewEventLoopStateMachine[S State, M StateMutator[S], R StateReader[S], Cfg any, Cb any, T Subject[S, M, R, Cfg, Cb]](
	ctx context.Context,
	name string,
	smConfig StateMachineConfig[S, M, R, Cfg, Cb],
	elConfig EventLoopConfig[S, M, R, Cfg, Cb, T],
	initialState S,
	subject T,
) *EventLoopStateMachine[S, M, R, Cfg, Cb, T] {
	bufSize := elConfig.BufferSize
	if bufSize <= 0 {
		bufSize = 50
	}
	return &EventLoopStateMachine[S, M, R, Cfg, Cb, T]{
		StateMachine: NewStateMachine(subject, smConfig, initialState),
		ctx:          ctx,
		subject:      subject,
		elConfig:     elConfig,
		name:         name,
		events:       make(chan common.Event, bufSize),
		stopLoop:     make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Start begins the event loop in a goroutine.
// Multiple calls to Start are ignored; only the first call starts the loop.
// TODO AM: this doesn't need to return done - it's accessible through a function
func (sm *EventLoopStateMachine[S, M, R, Cfg, Cb, T]) Start() <-chan struct{} {
	if sm.started {
		return sm.done
	}
	sm.started = true
	go sm.eventLoop()
	return sm.done
}

// QueueEvent adds an event to be processed by the event loop
func (sm *EventLoopStateMachine[S, M, R, Cfg, Cb, T]) QueueEvent(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("%s: pushing event onto queue: %s", sm.name, event.TypeString())
	sm.events <- event
	log.L(ctx).Tracef("%s: pushed event onto queue: %s", sm.name, event.TypeString())
}

// Stop signals the event loop to stop and waits for completion.
func (sm *EventLoopStateMachine[S, M, R, Cfg, Cb, T]) Stop() {
	select {
	case <-sm.done:
		return
	default:
	}
	sm.stopLoop <- struct{}{}
	<-sm.done
}

// Done returns a channel that closes when the event loop has stopped
func (sm *EventLoopStateMachine[S, M, R, Cfg, Cb, T]) Done() <-chan struct{} {
	return sm.done
}

func (sm *EventLoopStateMachine[S, M, R, Cfg, Cb, T]) eventLoop() {
	defer close(sm.done)
	log.L(sm.ctx).Debugf("%s: event loop started", sm.name)

	for {
		select {
		case event := <-sm.events:
			// Test sync hooks are handled automatically
			if syncHook, ok := event.(TestSyncHook); ok {
				syncHook.NotifyProcessed()
				continue
			}

			log.L(sm.ctx).Debugf("%s: pulled event from queue: %s", sm.name, event.TypeString())

			// OnEventReceived for routing/interception
			if sm.elConfig.OnEventReceived != nil {
				handled, err := sm.elConfig.OnEventReceived(sm.ctx, sm.subject.GetStateReader(), sm.subject.GetConfig(), sm.subject.GetCallbacks(), event)
				if err != nil {
					log.L(sm.ctx).Errorf("%s: error in OnEventReceived for event %s: %v", sm.name, event.TypeString(), err)
					if sm.elConfig.OnError != nil {
						sm.elConfig.OnError(sm.ctx, sm.subject, event, err)
					}
				}
				if handled {
					continue
				}
			}

			// Process through state machine - subject implements Subject[S, M, R, Cfg, Cb]
			err := sm.ProcessEvent(sm.ctx, event)
			if err != nil {
				if sm.elConfig.OnError != nil {
					sm.elConfig.OnError(sm.ctx, sm.subject, event, err)
				} else {
					log.L(sm.ctx).Errorf("%s: error processing event %s: %v", sm.name, event.TypeString(), err)
				}
			}

		case <-sm.stopLoop:
			// Allow synchronous cleanup on stop
			if sm.elConfig.OnStop != nil {
				if err := sm.elConfig.OnStop(sm.ctx, sm.subject); err != nil {
					log.L(sm.ctx).Errorf("%s: error in OnStop: %v", sm.name, err)
				}
			}
			log.L(sm.ctx).Debugf("%s: event loop stopped", sm.name)
			return
		}
	}
}
