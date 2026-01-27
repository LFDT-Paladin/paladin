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

	go el.Start(ctx)

	el.QueueEvent(ctx, newTestEvent(Event_Start))
	el.QueueEvent(ctx, newTestEvent(Event_Process))
	el.QueueEvent(ctx, newTestEvent(Event_Complete))
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done // blocks until all three events have been processed

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
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

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
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

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
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

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
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

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
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

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
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	el.StopAsync()
	el.WaitForStop()

	assert.True(t, el.IsStopped())
}

func TestEventLoopStopAsyncWhenAlreadyStopped(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done
	el.Stop()

	// StopAsync when already stopped hits "case <-el.loopStopped: return"
	el.StopAsync()
	el.StopAsync()

	assert.True(t, el.IsStopped())
}

func TestEventLoopTryQueueEvent(t *testing.T) {
	config := EventLoopConfig{BufferSize: 2}

	processorPickedUp := make(chan struct{})
	releaseProcessor := make(chan struct{})
	var once sync.Once
	processor := func(ctx context.Context, event common.Event) error {
		once.Do(func() { close(processorPickedUp) })
		<-releaseProcessor
		return nil
	}

	el := NewEventLoop(processor, config)
	ctx := context.Background()

	go el.Start(ctx)

	// One event will be picked up by the loop and block in processor
	assert.True(t, el.TryQueueEvent(ctx, newTestEvent(Event_Start)))
	<-processorPickedUp

	// Processor is blocking; fill the buffer (size 2: one slot may be taken by in-flight event)
	assert.True(t, el.TryQueueEvent(ctx, newTestEvent(Event_Process)))
	assert.True(t, el.TryQueueEvent(ctx, newTestEvent(Event_Complete)))

	// Buffer should now be full
	assert.False(t, el.TryQueueEvent(ctx, newTestEvent(Event_Fail)))

	close(releaseProcessor)
	el.Stop()
}

func TestEventLoopContextCancellation(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx, cancel := context.WithCancel(context.Background())

	go el.Start(ctx)
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	cancel()

	<-el.StopChannel()
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
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	assert.True(t, el.IsRunning())

	el.Stop()

	assert.False(t, el.IsRunning())
}

func TestEventLoopFIFOOrdering(t *testing.T) {
	var processedOrder []int
	var mu sync.Mutex
	allProcessed := make(chan struct{})

	processor := func(ctx context.Context, event common.Event) error {
		mu.Lock()
		processedOrder = append(processedOrder, int(event.Type()))
		n := len(processedOrder)
		mu.Unlock()
		if n == 10 {
			close(allProcessed)
		}
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)

	for i := 0; i < 10; i++ {
		el.QueueEvent(ctx, newTestEvent(common.EventType(1000+i)))
	}

	<-allProcessed
	el.Stop()

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, 10, len(processedOrder))
	for i := 0; i < 10; i++ {
		assert.Equal(t, 1000+i, processedOrder[i])
	}
}

func TestEventLoopConcurrentQueueing(t *testing.T) {
	var count atomic.Int32
	totalExpected := int32(10 * 100)
	allProcessed := make(chan struct{})

	processor := func(ctx context.Context, event common.Event) error {
		n := count.Add(1)
		if n == totalExpected {
			close(allProcessed)
		}
		return nil
	}

	el := NewEventLoop(processor, EventLoopConfig{BufferSize: 1000})
	ctx := context.Background()

	go el.Start(ctx)

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
	<-allProcessed
	el.Stop()

	assert.Equal(t, totalExpected, count.Load())
}

func TestEventLoopDefaultConfig(t *testing.T) {
	config := DefaultEventLoopConfig()
	assert.Equal(t, 50, config.BufferSize)
}

func TestEventLoopZeroBufferSize(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	el := NewEventLoop(processor, EventLoopConfig{BufferSize: 0})
	ctx := context.Background()

	go el.Start(ctx)
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	el.QueueEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	el.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	el.Stop()
}

func TestEventLoopEventChannel(t *testing.T) {
	processed := make(chan struct{})

	processor := func(ctx context.Context, event common.Event) error {
		close(processed)
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)

	el.EventChannel() <- newTestEvent(Event_Start)

	<-processed
	el.Stop()
}

func TestEventLoopStopChannel(t *testing.T) {
	processor := func(ctx context.Context, event common.Event) error {
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	stopCh := el.StopChannel()
	require.NotNil(t, stopCh)

	go el.Start(ctx)
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	el.StopAsync()
	<-stopCh
	assert.True(t, el.IsStopped())
}

func TestEventLoopStopAsyncAlreadySignaled(t *testing.T) {
	// Block processor until we've called StopAsync twice, so the loop never
	// reads from stopLoop before the second StopAsync hits the "default" branch.
	releaseProcessor := make(chan struct{})
	processorBlocked := make(chan struct{})
	processor := func(ctx context.Context, event common.Event) error {
		close(processorBlocked) // signal that we're in the processor
		<-releaseProcessor      // block until test has called StopAsync twice
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)
	el.QueueEvent(ctx, newTestEvent(Event_Start))
	<-processorBlocked // wait until loop is stuck in processor

	// First StopAsync sends to stopLoop (buffer size 1)
	el.StopAsync()
	// Second StopAsync hits "already signaled" default (stopLoop buffer full)
	el.StopAsync()

	close(releaseProcessor)
	el.WaitForStop()
	assert.True(t, el.IsStopped())
}

func TestEventLoopStopAsyncConcurrentDoubleSignal(t *testing.T) {
	// Multiple goroutines call StopAsync; one will send, the rest hit "already signaled" default.
	releaseProcessor := make(chan struct{})
	processorBlocked := make(chan struct{})
	processor := func(ctx context.Context, event common.Event) error {
		close(processorBlocked)
		<-releaseProcessor
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)
	el.QueueEvent(ctx, newTestEvent(Event_Start))
	<-processorBlocked

	// Many goroutines call StopAsync; first send succeeds, rest hit default
	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() { el.StopAsync(); wg.Done() }()
	}
	wg.Wait()

	close(releaseProcessor)
	el.WaitForStop()
	assert.True(t, el.IsStopped())
}

func TestEventLoopOnStopFinalEventError(t *testing.T) {
	finalEventErr := errors.New("final event failed")
	processedFinal := make(chan struct{})

	processor := func(ctx context.Context, event common.Event) error {
		if event.Type() == Event_Reset {
			close(processedFinal)
			return finalEventErr
		}
		return nil
	}

	onStop := func(ctx context.Context) common.Event {
		return newTestEvent(Event_Reset)
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig(), WithOnStop(onStop))
	ctx := context.Background()

	go el.Start(ctx)
	el.QueueEvent(ctx, newTestEvent(Event_Start))
	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	el.Stop()

	<-processedFinal
	assert.True(t, el.IsStopped())
}

func TestEventLoopSyncEvent(t *testing.T) {
	var processed int
	var mu sync.Mutex

	processor := func(ctx context.Context, event common.Event) error {
		mu.Lock()
		processed++
		mu.Unlock()
		return nil
	}

	el := NewEventLoop(processor, DefaultEventLoopConfig())
	ctx := context.Background()

	go el.Start(ctx)

	syncEv := NewSyncEvent()
	el.QueueEvent(ctx, syncEv)
	<-syncEv.Done

	el.QueueEvent(ctx, newTestEvent(Event_Start))
	syncEv2 := NewSyncEvent()
	el.QueueEvent(ctx, syncEv2)
	<-syncEv2.Done

	el.Stop()

	mu.Lock()
	n := processed
	mu.Unlock()
	assert.Equal(t, 1, n) // SyncEvents are not counted (handled specially)
}
