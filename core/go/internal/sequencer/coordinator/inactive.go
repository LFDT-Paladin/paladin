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

func action_NewBlock(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*NewBlockEvent)
	c.currentBlockHeight = e.BlockHeight
	return nil
}

func action_EndorsementRequested(_ context.Context, c *coordinator, event common.Event) error {
	e := event.(*EndorsementRequestedEvent)
	if c.activeCoordinatorNode != e.From {
		c.activeCoordinatorNode = e.From
		c.coordinatorActive(c.contractAddress, e.From)
		c.updateOriginatorNodePool(e.From) // In case we ever take over as coordinator we need to send heartbeats to potential originators
	}
	return nil
}

func action_HeartbeatReceived(_ context.Context, c *coordinator, event common.Event) error {
	e := event.(*HeartbeatReceivedEvent)
	if c.activeCoordinatorNode != e.From {
		c.activeCoordinatorNode = e.From
		c.coordinatorActive(c.contractAddress, e.From)
		c.updateOriginatorNodePool(e.From) // In case we ever take over as coordinator we need to send heartbeats to potential originators
	}
	c.activeCoordinatorBlockHeight = e.BlockHeight
	for _, flushPoint := range e.FlushPoints {
		c.activeCoordinatorsFlushPointsBySignerNonce[flushPoint.GetSignerNonce()] = flushPoint
	}
	return nil
}

func action_SendHandoverRequest(ctx context.Context, c *coordinator, _ common.Event) error {
	c.sendHandoverRequest(ctx)
	return nil
}

func action_Idle(_ context.Context, c *coordinator, _ common.Event) error {
	c.coordinatorIdle(c.contractAddress)
	return nil
}

func action_ResetHeartbeatIntervalsSinceLastReceive(_ context.Context, c *coordinator, _ common.Event) error {
	c.heartbeatIntervalsSinceLastReceive = 0
	return nil
}

func action_IncrementHeartbeatIntervalsSinceLastReceive(_ context.Context, c *coordinator, _ common.Event) error {
	c.heartbeatIntervalsSinceLastReceive++
	return nil
}

func guard_ObservingIdleThresholdExceeded(_ context.Context, c *coordinator) bool {
	return c.heartbeatIntervalsSinceLastReceive >= c.inactiveToIdleGracePeriod
}
