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

package privatetxnmgr

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Mock timer that can fire on demand
type testCoordinatorTimer struct {
	ch chan time.Time
}

func newTestTimer() *testCoordinatorTimer {
	return &testCoordinatorTimer{ch: make(chan time.Time, 1)}
}

func (t *testCoordinatorTimer) C() <-chan time.Time {
	return t.ch
}

func (t *testCoordinatorTimer) Stop() bool {
	select {
	case <-t.ch:
		return false
	default:
	}
	return true
}

func (t *testCoordinatorTimer) Fire() {
	t.ch <- time.Now()
}

func newAssembleCoordinatorForTest(timeout time.Duration) *assembleCoordinator {
	return &assembleCoordinator{
		ctx:            context.Background(),
		nodeName:       "node1",
		requests:       make(chan *assembleRequest, 10),
		stopProcess:    make(chan struct{}),
		inflight:       make(map[string]chan struct{}),
		requestTimeout: timeout,
		timerFactory:   newDefaultTimer,
	}
}

func inflightLen(ac *assembleCoordinator) int {
	ac.inflightLock.Lock()
	defer ac.inflightLock.Unlock()
	return len(ac.inflight)
}

func hasInflight(ac *assembleCoordinator, requestID string) bool {
	ac.inflightLock.Lock()
	defer ac.inflightLock.Unlock()
	_, exists := ac.inflight[requestID]
	return exists
}

func TestAssembleCoordinatorCompleteUnknownIDIsNonBlocking(t *testing.T) {
	ac := newAssembleCoordinatorForTest(20 * time.Minute)
	ac.Complete("missing-request")
}

func TestAssembleCoordinatorCompleteOnlyUnblocksMatchingRequest(t *testing.T) {
	ac := newAssembleCoordinatorForTest(20 * time.Minute)

	doneA := ac.registerInflight("request-a")
	_ = ac.registerInflight("request-b")

	ac.Complete("request-b")
	require.True(t, hasInflight(ac, "request-a"))

	ac.Complete("request-a")
	ac.waitForDone("request-a", doneA)
	require.Equal(t, 0, inflightLen(ac))
}

func TestAssembleCoordinatorLateCompleteDoesNotInterfereWithNextRequest(t *testing.T) {
	ac := newAssembleCoordinatorForTest(20 * time.Minute)
	ac.registerInflight("request-old")
	ac.removeInflight("request-old") // emulate late completion after request is already gone

	doneCurrent := ac.registerInflight("request-current")

	ac.Complete("request-old")
	require.True(t, hasInflight(ac, "request-current"), "late completion should not affect current request")

	ac.Complete("request-current")
	ac.waitForDone("request-current", doneCurrent)
	require.Equal(t, 0, inflightLen(ac))
}

func TestAssembleCoordinatorStopInterruptsWaitForDone(t *testing.T) {
	ac := newAssembleCoordinatorForTest(20 * time.Minute)
	doneCh := ac.registerInflight("request-stop")
	ac.Stop()
	ac.waitForDone("request-stop", doneCh)
	require.Equal(t, 0, inflightLen(ac))
}

func TestAssembleCoordinatorWaitForDoneCleansInflightOnSuccessAndTimeout(t *testing.T) {
	ac := newAssembleCoordinatorForTest(20 * time.Minute)

	timeoutTimer := newTestTimer()
	ac.timerFactory = func(_ time.Duration) coordinatorTimer { return timeoutTimer }

	successCh := ac.registerInflight("request-success")
	ac.Complete("request-success")
	ac.waitForDone("request-success", successCh)
	require.Equal(t, 0, inflightLen(ac))

	timeoutCh := ac.registerInflight("request-timeout")
	timeoutTimer.Fire()
	ac.waitForDone("request-timeout", timeoutCh)
	require.Equal(t, 0, inflightLen(ac))
}
