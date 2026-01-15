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

package statemachine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test subject for event loop tests
type testSubject struct {
	eventsProcessed []common.Event
	errorOnEvent    bool
}

// Test event types
type testEvent struct {
	common.BaseEvent
	value string
}

func (e *testEvent) Type() common.EventType { return 1 }
func (e *testEvent) TypeString() string     { return "TestEvent" }

type errorTriggerEvent struct {
	common.BaseEvent
}

func (e *errorTriggerEvent) Type() common.EventType { return 2 }
func (e *errorTriggerEvent) TypeString() string     { return "ErrorTriggerEvent" }

// Test states - must be int type to satisfy State constraint
type testState int

const (
	testState_Initial testState = iota
	testState_Running
)

func (s testState) String() string {
	switch s {
	case testState_Initial:
		return "Initial"
	case testState_Running:
		return "Running"
	default:
		return "Unknown"
	}
}

func newTestEventLoopStateMachine(ctx context.Context, subject *testSubject, errorOnEvent bool) *EventLoopStateMachine[testState, *testSubject] {
	subject.errorOnEvent = errorOnEvent

	smConfig := StateMachineConfig[testState, *testSubject]{
		Definitions: map[testState]StateDefinition[testState, *testSubject]{
			testState_Initial: {
				Events: map[EventType]EventHandler[testState, *testSubject]{
					1: {
						OnHandleEvent: func(ctx context.Context, s *testSubject, event common.Event) error {
							s.eventsProcessed = append(s.eventsProcessed, event)
							return nil
						},
						Transitions: []Transition[testState, *testSubject]{
							{To: testState_Running},
						},
					},
					2: {
						OnHandleEvent: func(ctx context.Context, s *testSubject, event common.Event) error {
							if s.errorOnEvent {
								return fmt.Errorf("simulated error")
							}
							s.eventsProcessed = append(s.eventsProcessed, event)
							return nil
						},
					},
				},
			},
			testState_Running: {
				Events: map[EventType]EventHandler[testState, *testSubject]{
					1: {
						OnHandleEvent: func(ctx context.Context, s *testSubject, event common.Event) error {
							s.eventsProcessed = append(s.eventsProcessed, event)
							return nil
						},
					},
					2: {
						OnHandleEvent: func(ctx context.Context, s *testSubject, event common.Event) error {
							if s.errorOnEvent {
								return fmt.Errorf("simulated error")
							}
							s.eventsProcessed = append(s.eventsProcessed, event)
							return nil
						},
					},
				},
			},
		},
	}

	elConfig := EventLoopConfig[testState, *testSubject]{
		BufferSize: 10,
	}

	return NewEventLoopStateMachine(ctx, "test", smConfig, elConfig, testState_Initial, subject)
}

func TestEventLoop_StartAndStop(t *testing.T) {
	ctx := context.Background()
	subject := &testSubject{}
	sm := newTestEventLoopStateMachine(ctx, subject, false)

	// Start the event loop
	sm.Start()

	// Verify the event loop is running by checking Done channel is not closed
	select {
	case <-sm.Done():
		t.Fatal("event loop should not be stopped initially")
	default:
		// Expected - loop is running
	}

	// Stop the event loop
	sm.Stop()

	// Verify the Done channel is closed
	select {
	case <-sm.Done():
		// Expected - loop stopped
	case <-time.After(100 * time.Millisecond):
		t.Fatal("event loop did not stop within timeout")
	}
}

func TestEventLoop_QueueEventProcessesEvents(t *testing.T) {
	ctx := context.Background()
	subject := &testSubject{}
	sm := newTestEventLoopStateMachine(ctx, subject, false)
	sm.Start()
	defer sm.Stop()

	// Queue an event
	event := &testEvent{value: "test1"}
	sm.QueueEvent(ctx, event)

	// Use SyncEvent to wait for processing
	sync := NewSyncEvent()
	sm.QueueEvent(ctx, sync)
	sync.Wait()

	// Verify the event was processed
	require.Equal(t, 1, len(subject.eventsProcessed))
	assert.Equal(t, event, subject.eventsProcessed[0])
}

func TestEventLoop_SyncEventWaitsForProcessing(t *testing.T) {
	ctx := context.Background()
	subject := &testSubject{}
	sm := newTestEventLoopStateMachine(ctx, subject, false)
	sm.Start()
	defer sm.Stop()

	// Queue multiple events followed by a sync
	sm.QueueEvent(ctx, &testEvent{value: "first"})
	sm.QueueEvent(ctx, &testEvent{value: "second"})
	sm.QueueEvent(ctx, &testEvent{value: "third"})

	sync := NewSyncEvent()
	sm.QueueEvent(ctx, sync)
	sync.Wait()

	// All events should be processed
	require.Equal(t, 3, len(subject.eventsProcessed))
}

func TestEventLoop_ErrorHandlingContinuesLoop(t *testing.T) {
	// Test that the event loop continues processing after an error

	ctx := context.Background()
	subject := &testSubject{}
	sm := newTestEventLoopStateMachine(ctx, subject, true) // Enable errors
	sm.Start()
	defer sm.Stop()

	// Queue an event that will trigger an error
	sm.QueueEvent(ctx, &errorTriggerEvent{})

	// Queue a valid event after the error
	validEvent := &testEvent{value: "after-error"}
	sm.QueueEvent(ctx, validEvent)

	// Wait for processing
	sync := NewSyncEvent()
	sm.QueueEvent(ctx, sync)
	sync.Wait()

	// The valid event should still be processed (error didn't stop the loop)
	require.Equal(t, 1, len(subject.eventsProcessed))
	assert.Equal(t, validEvent, subject.eventsProcessed[0])
}

func TestEventLoop_StopBlocksUntilComplete(t *testing.T) {
	ctx := context.Background()
	subject := &testSubject{}
	sm := newTestEventLoopStateMachine(ctx, subject, false)
	sm.Start()

	// Queue some events
	for i := 0; i < 5; i++ {
		sm.QueueEvent(ctx, &testEvent{value: fmt.Sprintf("event-%d", i)})
	}

	// Stop should block until the loop is complete
	sm.Stop()

	// After Stop returns, Done channel should be closed
	select {
	case <-sm.Done():
		// Expected
	default:
		t.Fatal("Done channel should be closed after Stop returns")
	}
}

func TestEventLoop_DoneChannelClosesOnStop(t *testing.T) {
	ctx := context.Background()
	subject := &testSubject{}
	sm := newTestEventLoopStateMachine(ctx, subject, false)
	sm.Start()

	done := sm.Done()

	// Verify channel is open initially
	select {
	case <-done:
		t.Fatal("Done channel should be open while running")
	default:
		// Expected
	}

	sm.Stop()

	// Verify channel is closed after stop
	select {
	case _, ok := <-done:
		assert.False(t, ok, "Done channel should be closed")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Done channel should close after Stop")
	}
}

func TestEventLoop_OnStopCallbackCalled(t *testing.T) {
	ctx := context.Background()
	subject := &testSubject{}

	onStopCalled := false

	smConfig := StateMachineConfig[testState, *testSubject]{
		Definitions: map[testState]StateDefinition[testState, *testSubject]{
			testState_Initial: {},
		},
	}

	elConfig := EventLoopConfig[testState, *testSubject]{
		BufferSize: 10,
		OnStop: func(ctx context.Context, s *testSubject) error {
			onStopCalled = true
			return nil
		},
	}

	sm := NewEventLoopStateMachine(ctx, "test", smConfig, elConfig, testState_Initial, subject)
	sm.Start()
	sm.Stop()

	assert.True(t, onStopCalled, "OnStop callback should be called when event loop stops")
}

func TestEventLoop_OnErrorCallbackCalled(t *testing.T) {
	ctx := context.Background()
	subject := &testSubject{}

	var capturedError error
	var capturedEvent common.Event

	smConfig := StateMachineConfig[testState, *testSubject]{
		Definitions: map[testState]StateDefinition[testState, *testSubject]{
			testState_Initial: {
				Events: map[EventType]EventHandler[testState, *testSubject]{
					2: {
						OnHandleEvent: func(ctx context.Context, s *testSubject, event common.Event) error {
							return fmt.Errorf("test error")
						},
					},
				},
			},
		},
	}

	elConfig := EventLoopConfig[testState, *testSubject]{
		BufferSize: 10,
		OnError: func(ctx context.Context, s *testSubject, event common.Event, err error) {
			capturedError = err
			capturedEvent = event
		},
	}

	sm := NewEventLoopStateMachine(ctx, "test", smConfig, elConfig, testState_Initial, subject)
	sm.Start()
	defer sm.Stop()

	// Queue an event that triggers an error
	errorEvent := &errorTriggerEvent{}
	sm.QueueEvent(ctx, errorEvent)

	sync := NewSyncEvent()
	sm.QueueEvent(ctx, sync)
	sync.Wait()

	assert.NotNil(t, capturedError, "OnError should capture the error")
	assert.Equal(t, "test error", capturedError.Error())
	assert.Equal(t, errorEvent, capturedEvent, "OnError should capture the event")
}

func TestEventLoop_MultipleStartsIgnored(t *testing.T) {
	ctx := context.Background()
	subject := &testSubject{}
	sm := newTestEventLoopStateMachine(ctx, subject, false)

	// Start multiple times - should not panic or cause issues
	sm.Start()
	sm.Start()
	sm.Start()

	// Verify still works
	sm.QueueEvent(ctx, &testEvent{value: "test"})
	sync := NewSyncEvent()
	sm.QueueEvent(ctx, sync)
	sync.Wait()

	require.Equal(t, 1, len(subject.eventsProcessed))

	sm.Stop()
}
