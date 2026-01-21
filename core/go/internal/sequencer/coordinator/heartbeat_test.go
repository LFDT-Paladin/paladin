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

package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
)

func TestHeartbeatLoop_LoopLifecycle(t *testing.T) {
	// Verify that initial and periodic heartbeats are sent and the loop exits when the context is cancelled
	var eventCount int
	heartbeatLoop := &heatbeatLoop{
		contractAddress: pldtypes.RandAddress().String(),
		interval:        10 * time.Millisecond,
		queueEvent: func(_ context.Context, _ common.Event) {
			eventCount++
		},
	}

	// Start heartbeat loop in a goroutine
	heartbeatLoop.start(t.Context())

	assert.Eventually(t, func() bool {
		return eventCount >= 2
	}, 100*time.Millisecond, 10*time.Millisecond)

	// Cancel to stop the loop
	heartbeatLoop.stop()

	// Verify cleanup
	assert.Eventually(t, func() bool {
		return heartbeatLoop.ctx == nil
	}, 50*time.Millisecond, 1*time.Millisecond, "heartbeatCtx should be nil after loop ends")
	assert.Nil(t, heartbeatLoop.cancelCtx, "heartbeatCancel should be nil after loop ends")
}

func TestHeartbeatLoop_DoesNotStartIfHeartbeatCtxAlreadySet(t *testing.T) {
	var eventCount int
	heartbeatLoop := &heatbeatLoop{
		contractAddress: pldtypes.RandAddress().String(),
		interval:        10 * time.Millisecond,
		queueEvent: func(_ context.Context, _ common.Event) {
			eventCount++
		},
	}
	heartbeatLoop.ctx, heartbeatLoop.cancelCtx = context.WithCancel(context.Background())

	heartbeatLoop.start(t.Context())

	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, eventCount, "heartbeat should not be sent if loop already running")
}

func TestHeartbeatLoop_CanBeRestartedAfterCancellation(t *testing.T) {
	var eventCount int
	heartbeatLoop := &heatbeatLoop{
		contractAddress: pldtypes.RandAddress().String(),
		interval:        10 * time.Millisecond,
		queueEvent: func(_ context.Context, _ common.Event) {
			eventCount++
		},
	}

	// Start first loop and wait for event
	heartbeatLoop.start(t.Context())
	assert.Eventually(t, func() bool {
		return eventCount > 0
	}, 50*time.Millisecond, 1*time.Millisecond)

	// stop the loop
	heartbeatLoop.stop()
	assert.Eventually(t, func() bool {
		return heartbeatLoop.ctx == nil
	}, 50*time.Millisecond, 1*time.Millisecond)

	// Reset event count
	eventCount = 0

	// Start second loop and wait for event
	heartbeatLoop.start(t.Context())
	assert.Eventually(t, func() bool {
		return eventCount > 0
	}, 50*time.Millisecond, 1*time.Millisecond)

	// stop the loop
	heartbeatLoop.stop()
	assert.Eventually(t, func() bool {
		return heartbeatLoop.ctx == nil
	}, 50*time.Millisecond, 1*time.Millisecond)
}
