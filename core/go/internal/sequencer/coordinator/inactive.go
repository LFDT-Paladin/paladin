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
	c.currentBlockHeight = e.BlockHeight
	// TODO AM: have we entered a new block range?
	return nil
}

func guard_IsNewBlockRange(_ context.Context, c *coordinator) bool {
	// TODO AM: implement
	return false
}

func validator_IsHeartbeatFromActiveCoordinator(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*HeartbeatReceivedEvent)
	return c.activeCoordinatorNode == e.From, nil
}

func action_HeartbeatReceived(_ context.Context, c *coordinator, event common.Event) error {
	// TODO AM: fix the typing
	// e := event.(*HeartbeatReceivedEvent)
	// c.activeCoordinatorState = e.CoordinatorState
	return nil
}

// TODO AM: make the name more descriptive - is it even needed if we're not tracking active coordinators?
func action_Idle(_ context.Context, c *coordinator, _ common.Event) error {
	c.coordinatorIdle(c.contractAddress)
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
