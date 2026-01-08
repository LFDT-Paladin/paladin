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
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRealClock_HasExpired_NotExpired(t *testing.T) {
	clock := RealClock()
	start := time.Now()
	duration := 100 * time.Millisecond

	// Immediately after start, should not be expired
	expired := clock.HasExpired(start, duration)
	assert.False(t, expired, "should not be expired immediately after start")
}

func TestRealClock_HasExpired_Expired(t *testing.T) {
	clock := RealClock()
	start := time.Now().Add(-200 * time.Millisecond) // 200ms ago
	duration := 100 * time.Millisecond

	// Start + duration = 100ms ago, which is in the past
	expired := clock.HasExpired(start, duration)
	assert.True(t, expired, "should be expired when start + duration is in the past")
}

func TestRealClock_HasExpired_ExactlyAtExpiry(t *testing.T) {
	clock := RealClock()
	start := time.Now().Add(-100 * time.Millisecond) // 100ms ago
	duration := 100 * time.Millisecond

	// Start + duration = now (approximately), but After() returns true only if strictly after
	// So we need to wait a tiny bit to ensure it's expired
	time.Sleep(1 * time.Millisecond)
	expired := clock.HasExpired(start, duration)
	assert.True(t, expired, "should be expired when start + duration is exactly at or before now")
}

func TestRealClock_HasExpired_ZeroDuration(t *testing.T) {
	clock := RealClock()
	start := time.Now().Add(-1 * time.Millisecond) // 1ms ago
	duration := 0 * time.Millisecond

	// With zero duration, if start is in the past, it should be expired
	expired := clock.HasExpired(start, duration)
	assert.True(t, expired, "should be expired when start is in the past and duration is zero")
}

func TestRealClock_HasExpired_FutureStart(t *testing.T) {
	clock := RealClock()
	start := time.Now().Add(100 * time.Millisecond) // 100ms in the future
	duration := 50 * time.Millisecond

	// Even with a future start, start + duration is still in the future
	expired := clock.HasExpired(start, duration)
	assert.False(t, expired, "should not be expired when start + duration is in the future")
}

func TestRealClock_ScheduleTimer_FiresAfterDuration(t *testing.T) {
	clock := RealClock()
	ctx := context.Background()
	duration := 50 * time.Millisecond

	var fired bool
	var mu sync.Mutex

	f := func() {
		mu.Lock()
		defer mu.Unlock()
		fired = true
	}

	cancel := clock.ScheduleTimer(ctx, duration, f)

	// Wait for timer to fire
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.True(t, fired, "timer function should have been called")
	mu.Unlock()

	// Cancel should not panic
	cancel()
}

func TestRealClock_ScheduleTimer_CanBeCancelled(t *testing.T) {
	clock := RealClock()
	ctx := context.Background()
	duration := 200 * time.Millisecond

	var fired bool
	var mu sync.Mutex

	f := func() {
		mu.Lock()
		defer mu.Unlock()
		fired = true
	}

	cancel := clock.ScheduleTimer(ctx, duration, f)

	// Cancel immediately
	cancel()

	// Wait longer than the duration
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	assert.False(t, fired, "timer function should not have been called after cancellation")
	mu.Unlock()
}

func TestRealClock_ScheduleTimer_CancelledViaContext(t *testing.T) {
	clock := RealClock()
	ctx, cancelCtx := context.WithCancel(context.Background())
	duration := 200 * time.Millisecond

	var fired bool
	var mu sync.Mutex

	f := func() {
		mu.Lock()
		defer mu.Unlock()
		fired = true
	}

	cancel := clock.ScheduleTimer(ctx, duration, f)

	// Cancel the context
	cancelCtx()

	// Wait longer than the duration
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	assert.False(t, fired, "timer function should not have been called after context cancellation")
	mu.Unlock()

	// Cancel should not panic
	cancel()
}

func TestRealClock_ScheduleTimer_MultipleTimers(t *testing.T) {
	clock := RealClock()
	ctx := context.Background()

	var fired1, fired2 bool
	var mu sync.Mutex

	f1 := func() {
		mu.Lock()
		defer mu.Unlock()
		fired1 = true
	}

	f2 := func() {
		mu.Lock()
		defer mu.Unlock()
		fired2 = true
	}

	cancel1 := clock.ScheduleTimer(ctx, 50*time.Millisecond, f1)
	cancel2 := clock.ScheduleTimer(ctx, 100*time.Millisecond, f2)

	// Wait for both timers
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	assert.True(t, fired1, "first timer should have fired")
	assert.True(t, fired2, "second timer should have fired")
	mu.Unlock()

	cancel1()
	cancel2()
}

func TestRealClock_ScheduleTimer_ShortDuration(t *testing.T) {
	clock := RealClock()
	ctx := context.Background()
	duration := 10 * time.Millisecond

	var fired bool
	var mu sync.Mutex

	f := func() {
		mu.Lock()
		defer mu.Unlock()
		fired = true
	}

	cancel := clock.ScheduleTimer(ctx, duration, f)

	// Wait for timer to fire
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.True(t, fired, "timer function should have been called even with short duration")
	mu.Unlock()

	cancel()
}

func TestFakeClockForTesting_ScheduleTimer_ReturnsCancelFunction(t *testing.T) {
	clock := &FakeClockForTesting{}
	ctx := context.Background()
	duration := clock.Duration(100)
	var callbackCalled bool

	cancel := clock.ScheduleTimer(ctx, duration, func() {
		callbackCalled = true
	})

	// Verify cancel function is returned and can be called
	assert.NotNil(t, cancel, "ScheduleTimer should return a cancel function")
	
	// Call cancel - should not panic
	cancel()
	
	// Verify callback was never called (fake clock doesn't actually schedule)
	assert.False(t, callbackCalled, "callback should not be called in fake clock")
}

func TestFakeClockForTesting_ScheduleTimer_CallbackNeverFires(t *testing.T) {
	clock := &FakeClockForTesting{}
	ctx := context.Background()
	duration := clock.Duration(50)
	var callbackCalled bool
	var mu sync.Mutex

	f := func() {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
	}

	cancel := clock.ScheduleTimer(ctx, duration, f)

	// Wait a bit to ensure callback would have fired if it were real
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.False(t, callbackCalled, "callback should never fire in fake clock without manual advancement")
	mu.Unlock()

	cancel()
}

func TestFakeClockForTesting_ScheduleTimer_MultipleTimers(t *testing.T) {
	clock := &FakeClockForTesting{}
	ctx := context.Background()

	var callback1Called, callback2Called bool
	var mu sync.Mutex

	f1 := func() {
		mu.Lock()
		defer mu.Unlock()
		callback1Called = true
	}

	f2 := func() {
		mu.Lock()
		defer mu.Unlock()
		callback2Called = true
	}

	cancel1 := clock.ScheduleTimer(ctx, clock.Duration(50), f1)
	cancel2 := clock.ScheduleTimer(ctx, clock.Duration(100), f2)

	// Wait to ensure callbacks would have fired if real
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	assert.False(t, callback1Called, "first callback should not fire in fake clock")
	assert.False(t, callback2Called, "second callback should not fire in fake clock")
	mu.Unlock()

	cancel1()
	cancel2()
}

func TestFakeClockForTesting_ScheduleTimer_CancelCanBeCalledMultipleTimes(t *testing.T) {
	clock := &FakeClockForTesting{}
	ctx := context.Background()
	duration := clock.Duration(100)

	cancel := clock.ScheduleTimer(ctx, duration, func() {})

	// Call cancel multiple times - should not panic
	cancel()
	cancel()
	cancel()
}

func TestFakeClockForTesting_ScheduleTimer_WithNilCallback(t *testing.T) {
	clock := &FakeClockForTesting{}
	ctx := context.Background()
	duration := clock.Duration(100)

	// Should not panic even with nil callback
	cancel := clock.ScheduleTimer(ctx, duration, nil)
	assert.NotNil(t, cancel, "should return cancel function even with nil callback")
	
	cancel()
}

