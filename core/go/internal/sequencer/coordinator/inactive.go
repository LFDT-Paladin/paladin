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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

func action_RejectDelegatedTransactions(ctx context.Context, c *coordinator, event common.Event) error {
	// TODO AM: implement accordinging to spec
	// Is there a nuance between not active and waiting for flush?
	return nil
}

// TODO AM: this needs to go somewhere more generic now
func action_UpdateBlockHeight(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*NewBlockEvent)
	newHeight := e.BlockHeight
	blockRange := c.coordinatorSelectionBlockRange

	// integer division tells us which block range epoch we're in and allows us to compare old with new
	c.newBlockRangeEpoch = newHeight/blockRange == c.currentBlockHeight/blockRange
	c.currentBlockHeight = newHeight
	return nil
}

func guard_IsNewBlockRangeEpoch(_ context.Context, c *coordinator) bool {
	// This is set when we update the block height, and remains valid until the next block height update
	return c.newBlockRangeEpoch
}

func validator_IsHeartbeatFromActiveCoordinator(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*HeartbeatReceivedEvent)
	return c.activeCoordinatorNode == e.From, nil
}

func action_HeartbeatReceived(_ context.Context, c *coordinator, event common.Event) error {
	e := event.(*HeartbeatReceivedEvent)
	c.activeCoordinatorState = State(e.CoordinatorState)
	return nil
}

// TODO AM: move everything about heartbeats to a new file
func action_ResetHeartbeatIntervalsSinceLastReceive(_ context.Context, c *coordinator, _ common.Event) error {
	c.heartbeatIntervalsSinceLastReceive = 0
	return nil
}

func action_IncrementHeartbeatIntervalsSinceLastReceive(_ context.Context, c *coordinator, _ common.Event) error {
	c.heartbeatIntervalsSinceLastReceive++
	return nil
}

func guard_HeartbeatThresholdExceeded(_ context.Context, c *coordinator) bool {
	return c.heartbeatIntervalsSinceLastReceive >= c.heartbeatGracePeriod
}
