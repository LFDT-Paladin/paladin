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
	"time"
)

type Clock interface {
	//wrapper of time.Now()
	//primarily to allow artificial clocks to be injected for testing
	Now() Time
	HasExpired(Time, Duration) bool
	Duration(milliseconds int) Duration
	ScheduleTimer(context.Context, Duration, func()) (cancel func())
}

type Duration interface {
}

type Time interface {
}

type realClock struct{}

func (c *realClock) Duration(milliseconds int) Duration {
	return time.Duration(milliseconds) * time.Millisecond
}
func (c *realClock) Now() Time {
	return time.Now()
}

func RealClock() Clock {
	return &realClock{}
}

func (c *realClock) HasExpired(start Time, duration Duration) bool {
	realStart := start.(time.Time)
	realDuration := duration.(time.Duration)
	return time.Now().After(realStart.Add(realDuration))
}

func (c *realClock) ScheduleTimer(ctx context.Context, duration Duration, f func()) (cancel func()) {
	timerCtx, cancel := context.WithCancel(ctx)
	realDuration := duration.(time.Duration)
	timer := time.NewTimer(realDuration)
	go func() {
		defer timer.Stop()
		select {
		case <-timer.C:
			f()
		case <-timerCtx.Done():
			return
		}
	}()
	return cancel
}

type FakeClockForTesting struct {
	currentTime   int
	pendingTimers []pendingTimer
}

type pendingTimer struct {
	fireTime  int
	callback  func()
	cancelled *bool
}

type fakeTime struct {
	milliseconds int
}

type fakeDuration struct {
	milliseconds int
}

// On the fake clock, time is just a number (milliseconds).
// Advance adds to currentTime and runs any scheduled timer callbacks whose fire time is now in the past.
func (c *FakeClockForTesting) Advance(advance int) {
	c.currentTime += advance
	var stillPending []pendingTimer
	for _, pt := range c.pendingTimers {
		if pt.cancelled != nil && *pt.cancelled {
			continue
		}
		if pt.fireTime <= c.currentTime && pt.callback != nil {
			pt.callback()
			continue
		}
		stillPending = append(stillPending, pt)
	}
	c.pendingTimers = stillPending
}

func (c *FakeClockForTesting) Now() Time {
	return &fakeTime{c.currentTime}
}

func (c *FakeClockForTesting) HasExpired(start Time, duration Duration) bool {
	startMillis := start.(*fakeTime).milliseconds
	durationMillis := duration.(*fakeDuration).milliseconds
	nowMillis := c.currentTime
	return nowMillis > startMillis+durationMillis

}

func (c *FakeClockForTesting) ScheduleTimer(_ context.Context, duration Duration, f func()) (cancel func()) {
	var durationMillis int
	if fake, ok := duration.(*fakeDuration); ok {
		durationMillis = fake.milliseconds
	} else {
		durationMillis = int(duration.(time.Duration).Milliseconds())
	}
	fireTime := c.currentTime + durationMillis
	cancelled := false
	c.pendingTimers = append(c.pendingTimers, pendingTimer{fireTime: fireTime, callback: f, cancelled: &cancelled})
	return func() {
		cancelled = true
	}
}

func (c *FakeClockForTesting) Duration(milliseconds int) Duration {
	return &fakeDuration{milliseconds}
}
