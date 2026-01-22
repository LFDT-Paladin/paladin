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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

// EventLoopConfig holds configuration for the event loop
type EventLoopConfig struct {
	// BufferSize is the size of the event channel buffer (default: 50)
	BufferSize int
}

// DefaultEventLoopConfig returns a configuration with sensible defaults
func DefaultEventLoopConfig() EventLoopConfig {
	return EventLoopConfig{
		BufferSize: 50,
	}
}

// EventProcessor is a function that processes events for an entity
type EventProcessor func(ctx context.Context, event common.Event) error

// OnStopCallback is called when the event loop receives a stop signal
// It can optionally return a final event to process before stopping
type OnStopCallback func(ctx context.Context) common.Event

// EventLoop manages an event processing loop for a state machine.
// It provides:
//   - Asynchronous event queueing via QueueEvent
//   - Synchronous event processing via ProcessEvent
//   - Graceful shutdown via Stop
type EventLoop struct {
	events      chan common.Event
	stopLoop    chan struct{}
	loopStopped chan struct{}
	processor   EventProcessor
	onStop      OnStopCallback
	name        string // For logging
	running     bool
}

// EventLoopOption is a functional option for configuring an EventLoop
type EventLoopOption func(*EventLoop)

// WithOnStop sets a callback that is invoked when the event loop is stopping.
// The callback can return an event to be processed as the final event before stopping.
func WithOnStop(callback OnStopCallback) EventLoopOption {
	return func(el *EventLoop) {
		el.onStop = callback
	}
}

// WithEventLoopName sets a name for the event loop (used in logging)
func WithEventLoopName(name string) EventLoopOption {
	return func(el *EventLoop) {
		el.name = name
	}
}

// NewEventLoop creates a new event loop.
// processor: function to call for each event
// config: event loop configuration
// opts: optional configuration options
func NewEventLoop(
	processor EventProcessor,
	config EventLoopConfig,
	opts ...EventLoopOption,
) *EventLoop {
	if config.BufferSize <= 0 {
		config.BufferSize = 50
	}

	el := &EventLoop{
		events:      make(chan common.Event, config.BufferSize),
		stopLoop:    make(chan struct{}, 1), // Buffered so Stop() never blocks on send
		loopStopped: make(chan struct{}),
		processor:   processor,
		name:        "eventloop",
	}

	for _, opt := range opts {
		opt(el)
	}

	return el
}

// Start begins the event processing loop. This should be called as a goroutine.
// The loop will run until Stop() is called.
func (el *EventLoop) Start(ctx context.Context) {
	defer close(el.loopStopped)
	el.running = true

	log.L(ctx).Debugf("%s: event loop started", el.name)

	for {
		select {
		case event := <-el.events:
			// Handle SyncEvent specially - just signal and continue
			if syncEvent, ok := IsSyncEvent(event); ok {
				log.L(ctx).Debugf("%s: sync event processed", el.name)
				close(syncEvent.Done)
				continue
			}

			log.L(ctx).Debugf("%s: processing event %s", el.name, event.TypeString())
			err := el.processor(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("%s: error processing event %s: %v", el.name, event.TypeString(), err)
			}
		case <-el.stopLoop:
			// Process final event if callback provided
			if el.onStop != nil {
				if finalEvent := el.onStop(ctx); finalEvent != nil {
					log.L(ctx).Debugf("%s: processing final event %s", el.name, finalEvent.TypeString())
					err := el.processor(ctx, finalEvent)
					if err != nil {
						log.L(ctx).Errorf("%s: error processing final event: %v", el.name, err)
					}
				}
			}
			log.L(ctx).Debugf("%s: event loop stopped", el.name)
			el.running = false
			return
		case <-ctx.Done():
			log.L(ctx).Debugf("%s: context cancelled, stopping event loop", el.name)
			el.running = false
			return
		}
	}
}

// QueueEvent asynchronously queues an event for processing.
// This is the recommended way for most components to submit events.
// The event will be processed in FIFO order on the event loop goroutine.
func (el *EventLoop) QueueEvent(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("%s: queueing event %s", el.name, event.TypeString())
	el.events <- event
}

// TryQueueEvent attempts to queue an event without blocking.
// Returns true if the event was queued, false if the buffer is full.
func (el *EventLoop) TryQueueEvent(ctx context.Context, event common.Event) bool {
	select {
	case el.events <- event:
		log.L(ctx).Tracef("%s: queued event %s", el.name, event.TypeString())
		return true
	default:
		log.L(ctx).Warnf("%s: event buffer full, dropping event %s", el.name, event.TypeString())
		return false
	}
}

// Stop signals the event loop to stop and waits for it to complete.
// This is idempotent - calling Stop multiple times is safe.
func (el *EventLoop) Stop() {
	// Check if already stopped
	select {
	case <-el.loopStopped:
		return
	default:
	}

	// Signal stop
	select {
	case el.stopLoop <- struct{}{}:
	default:
		// Already signaled
	}

	// Wait for completion
	<-el.loopStopped
}

// StopAsync signals the event loop to stop but does not wait for completion.
// Use WaitForStop to wait for the loop to finish.
func (el *EventLoop) StopAsync() {
	select {
	case <-el.loopStopped:
		return
	default:
	}

	select {
	case el.stopLoop <- struct{}{}:
	default:
	}
}

// WaitForStop waits for the event loop to complete after Stop or StopAsync was called.
func (el *EventLoop) WaitForStop() {
	<-el.loopStopped
}

// IsStopped returns true if the event loop has been stopped.
func (el *EventLoop) IsStopped() bool {
	select {
	case <-el.loopStopped:
		return true
	default:
		return false
	}
}

// IsRunning returns true if the event loop is currently running.
func (el *EventLoop) IsRunning() bool {
	return el.running && !el.IsStopped()
}

// EventChannel returns the underlying event channel for advanced use cases.
// Most callers should use QueueEvent instead.
func (el *EventLoop) EventChannel() chan<- common.Event {
	return el.events
}

// StopChannel returns the stop channel for advanced use cases.
func (el *EventLoop) StopChannel() <-chan struct{} {
	return el.loopStopped
}
