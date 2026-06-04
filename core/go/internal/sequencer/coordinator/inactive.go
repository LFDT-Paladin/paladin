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

// validator_IsHeartbeatSenderLive returns true when the heartbeat sender reports being in one of
// the liveness-proving states: Elect, Prepared, Active, or Active_Flush.
func validator_IsHeartbeatSenderLive(_ context.Context, _ *coordinator, event common.Event) (bool, error) {
	e := event.(*common.HeartbeatReceivedEvent)
	state := e.CoordinatorSnapshot.CoordinatorState
	return state == common.CoordinatorState_Elect ||
		state == common.CoordinatorState_Prepared ||
		state == common.CoordinatorState_Active ||
		state == common.CoordinatorState_Active_Flush, nil
}

// validator_IsHeartbeatFromHigherPriorityCoordinator returns true when a heartbeat is froma node that is higher-priority than this node
func validator_IsHeartbeatFromHigherPriorityCoordinator(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*common.HeartbeatReceivedEvent)
	return common.IsHigherPriority(c.coordinatorPriorityList, e.FromNode, c.currentActiveCoordinator), nil

}

// validator_IsHandoverRequestFromHigherPriorityCoordinator returns true when a HandoverRequest is from
// a node that has strictly higher priority (lower index) than this node.
func validator_IsHandoverRequestFromHigherPriorityCoordinator(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*HandoverRequestEvent)
	return common.IsHigherPriority(c.coordinatorPriorityList, e.FromNode, c.nodeName), nil
}

func action_RejectDelegationRequest(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*TransactionsDelegatedEvent)
	c.recordOriginatorActivity(e.FromNode)
	return c.transportWriter.SendDelegationRequestRejection(ctx, e.FromNode, e.DelegationID, c.currentBlockHeight, c.currentActiveCoordinator)
}

func action_SetSelfAsActiveCoordinator(_ context.Context, c *coordinator, _ common.Event) error {
	c.currentActiveCoordinator = c.nodeName
	return nil
}

// action_UpdateActiveCoordinator updates the current active coordinator from either a received
// heartbeat or a handover request. Both events carry the sender identity in different fields.
func action_UpdateActiveCoordinator(_ context.Context, c *coordinator, event common.Event) error {
	switch e := event.(type) {
	case *common.HeartbeatReceivedEvent:
		c.currentActiveCoordinator = e.FromNode
	case *HandoverRequestEvent:
		c.currentActiveCoordinator = e.FromNode
	}
	return nil
}

func action_ResetHeartbeatIntervalsSinceLastReceive(_ context.Context, c *coordinator, _ common.Event) error {
	c.heartbeatIntervalsSinceLastReceive = 0
	return nil
}

func action_IncrementHeartbeatIntervalCounts(_ context.Context, c *coordinator, _ common.Event) error {
	c.heartbeatIntervalsSinceLastReceive++
	c.heartbeatIntervalsSinceStateChange++
	return nil
}

// validator_IsHeartbeatFromCurrentActiveCoordinator checks that the heartbeat is from the node
// we believe is the current active coordinator
func validator_IsHeartbeatFromCurrentActiveCoordinator(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*common.HeartbeatReceivedEvent)
	return c.currentActiveCoordinator == e.FromNode, nil
}
