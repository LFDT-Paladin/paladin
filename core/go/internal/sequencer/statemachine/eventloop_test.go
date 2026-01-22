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
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventLoopBasic(t *testing.T) {
	var processedEvents []common.EventType
	var mu sync.Mutex

	processor := func(ctx context.Context, event common.Event) error {
		mu.Lock()
		defer mu.Unlock()
		processedEvents = append(processedEvents, event.Type())
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	// Start event loop
	go el.Start(ctx)

	// Queue some events
	el.QueueEvent(ctx, newTestEvent(Event_Start))
	el.QueueEvent(ctx, newTestEvent(Event_Process))
	el.QueueEvent(ctx, newTestEvent(Event_Complete))

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Stop and wait
	el.Stop()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 3, len(processedEvents))
	assert.Equal(t, Event_Start, processedEvents[0])
	assert.Equal(t, Event_Process, processedEvents[1])
	assert.Equal(t, Event_Complete, processedEvents[2])
}

func TestEventLoopWithOnStop(t *testing.T) {
	var processedEvents []common.EventType
	var mu sync.Mutex

	processor := func(ctx context.Context, event common.Event) error {
		mu.Lock()
		defer mu.Unlock()
		processedEvents = append(processedEvents, event.Type())
		return nil
	}

	onStop := func(ctx context.Context) common.Event {
		return newTestEvent(Event_Reset) // Final event
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig(), WithOnStop(onStop))
	ctx := context.Background()

	go el.Start(ctx)

	el.QueueEvent(ctx, newTestEvent(Event_Start))
	time.Sleep(50 * time.Millisecond)

	el.Stop()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, len(processedEvents))
	assert.Equal(t, Event_Start, processedEvents[0])
	assert.Equal(t, Event_Reset, processedEvents[1]) // Final event from onStop
}

func TestEventLoopWithOnStopReturnsNil(t *testing.T) {
	var processedEvents []common.EventType
	var mu sync.Mutex

	processor := func(ctx context.Context, event common.Event) error {
		mu.Lock()
		defer mu.Unlock()
		processedEvents = append(processedEvents, event.Type())
		return nil
	}

	onStop := func(ctx context.Context) common.Event {
		return nil // No final event
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig(), WithOnStop(onStop))
	ctx := context.Background()

	go el.Start(ctx)

	el.QueueEvent(ctx, newTestEvent(Event_Start))
	time.Sleep(50 * time.Millisecond)

	el.Stop()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, len(processedEvents))
}

func TestEventLoopWithName(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig(),
		WithEventLoopName("test-loop"))
	ctx := context.Background()

	go el.Start(ctx)

	el.QueueEvent(ctx, newTestEvent(Event_Start))
	time.Sleep(50 * time.Millisecond)

	el.Stop()
}

func TestEventLoopProcessorError(t *testing.T) {
	expectedErr := errors.New("processing failed")

	processor := func(ctx context.Context, event common.Event) error {
		return expectedErr
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)

	el.QueueEvent(ctx, newTestEvent(Event_Start))
	time.Sleep(50 * time.Millisecond)

	el.Stop()
	// Error is logged via log.L(ctx) - no assertion needed
}

func TestEventLoopStopIdempotent(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	// Stop multiple times should not panic
	el.Stop()
	el.Stop()
	el.Stop()

	assert.True(t, el.IsStopped())
}

func TestEventLoopStopAsync(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	// Async stop
	el.StopAsync()

	// Wait separately
	el.WaitForStop()

	assert.True(t, el.IsStopped())
}

func TestEventLoopTryQueueEvent(t *testing.T) {
	// Create a small buffer
	config := EventLoopConfig{BufferSize: 2}

	// Processor that blocks
	blockCh := make(chan struct{})
	processor := func(ctx context.Context, event common.Event) error {
		<-blockCh // Block until signaled
		return nil
	}

	el := NewEventLoop(processor, config)
	ctx := context.Background()

	go el.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	// Queue events up to buffer limit (one will be picked up by the processor and block)
	assert.True(t, el.TryQueueEvent(ctx, newTestEvent(Event_Start)))
	time.Sleep(10 * time.Millisecond) // Let the first event be picked up

	// Now the processor is blocking, fill the buffer
	assert.True(t, el.TryQueueEvent(ctx, newTestEvent(Event_Process)))
	assert.True(t, el.TryQueueEvent(ctx, newTestEvent(Event_Complete)))

	// Buffer should now be full
	assert.False(t, el.TryQueueEvent(ctx, newTestEvent(Event_Fail)))

	// Unblock processor
	close(blockCh)

	el.Stop()
}

func TestEventLoopContextCancellation(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx, cancel := context.WithCancel(context.Background())

	go el.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	// Cancel context
	cancel()

	// Should stop
	time.Sleep(50 * time.Millisecond)
	assert.True(t, el.IsStopped())
}

func TestEventLoopIsRunning(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	assert.False(t, el.IsRunning())

	go el.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	assert.True(t, el.IsRunning())

	el.Stop()

	assert.False(t, el.IsRunning())
}

func TestEventLoopFIFOOrdering(t *testing.T) {
	var processedOrder []int
	var mu sync.Mutex

	processor := func(ctx context.Context, event common.Event) error {
		mu.Lock()
		defer mu.Unlock()
		// Use event type as order indicator
		processedOrder = append(processedOrder, int(event.Type()))
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)

	// Queue events with different "order" values encoded in event type
	for i := 0; i < 10; i++ {
		el.QueueEvent(ctx, newTestEvent(common.EventType(1000+i)))
	}

	time.Sleep(100 * time.Millisecond)
	el.Stop()

	mu.Lock()
	defer mu.Unlock()

	// Verify FIFO order
	require.Equal(t, 10, len(processedOrder))
	for i := 0; i < 10; i++ {
		assert.Equal(t, 1000+i, processedOrder[i])
	}
}

func TestEventLoopConcurrentQueueing(t *testing.T) {
	var count atomic.Int32

	processor := func(ctx context.Context, event common.Event) error {
		count.Add(1)
		return nil
	}

	el := NewEventLoop(processor, EventLoopConfig{BufferSize: 1000})
	ctx := context.Background()

	go el.Start(ctx)

	// Queue events from multiple goroutines
	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				el.QueueEvent(ctx, newTestEvent(Event_Start))
			}
		}()
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)
	el.Stop()

	assert.Equal(t, int32(numGoroutines*eventsPerGoroutine), count.Load())
}

func TestEventLoopDefaultConfig(t *testing.T) {
	config := DefaultEventLoopConfig()
	assert.Equal(t, 50, config.BufferSize)
}

func TestEventLoopZeroBufferSize(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	// Zero buffer size should use default
	el := NewEventLoop(processor, EventLoopConfig{BufferSize: 0})
	ctx := context.Background()

	go el.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	// Should still work
	el.QueueEvent(ctx, newTestEvent(Event_Start))
	time.Sleep(50 * time.Millisecond)

	el.Stop()
}

func TestEventLoopEventChannel(t *testing.T) {
	var processed bool

	processor := func(ctx context.Context, event common.Event) error {
		processed = true
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)

	// Use direct channel access
	el.EventChannel() <- newTestEvent(Event_Start)

	time.Sleep(50 * time.Millisecond)
	el.Stop()

	assert.True(t, processed)
}
