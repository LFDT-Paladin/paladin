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
	Now() time.Time
	HasExpired(time.Time, time.Duration) bool
	Duration(milliseconds int) time.Duration
	ScheduleTimer(context.Context, time.Duration, func()) (cancel func())
}

type realClock struct{}

func (c *realClock) Duration(milliseconds int) time.Duration {
	return time.Duration(milliseconds) * time.Millisecond
}
func (c *realClock) Now() time.Time {
	return time.Now()
}

func RealClock() Clock {
	return &realClock{}
}

func (c *realClock) HasExpired(start time.Time, duration time.Duration) bool {
	return !time.Now().Before(start.Add(duration))
}

func (c *realClock) ScheduleTimer(ctx context.Context, duration time.Duration, f func()) (cancel func()) {
	timerCtx, cancel := context.WithCancel(ctx)
	timer := time.NewTimer(duration)
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
