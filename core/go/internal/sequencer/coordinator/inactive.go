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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
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

// validator_IsHandoverRequestFromHigherPriorityCoordinator returns true when a HandoverRequest is from
// a node that has strictly higher priority (lower index) than this node.
func validator_IsHandoverRequestFromHigherPriorityCoordinator(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*HandoverRequestEvent)
	return common.IsHigherPriority(c.coordinatorPriorityList, e.FromNode, c.nodeName), nil
}

// validator_IsDelegationBlockHeightToleranceExceeded returns true when the absolute difference
// between this coordinator's stored block height (refreshed by action_RefreshBlockHeight)
// and the originator's block height exceeds the configured block height tolerance.
func validator_IsDelegationBlockHeightToleranceExceeded(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*TransactionsDelegatedEvent)
	ch := uint64(c.currentBlockHeight)
	oh := e.OriginatorsBlockHeight
	diff := max(ch, oh) - min(ch, oh)
	return diff > c.blockHeightTolerance, nil
}

// action_RejectDelegationRequestBlockHeight sends a DelegationRejection indicating the block
// height difference exceeds the configured tolerance.
func action_RejectDelegationRequestBlockHeight(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*TransactionsDelegatedEvent)
	c.recordOriginatorActivity(e.FromNode)
	log.L(ctx).Warnf("rejecting delegation from %s due to block height tolerance (originator=%d, coordinator=%d, tolerance=%d)",
		e.FromNode, e.OriginatorsBlockHeight, c.currentBlockHeight, c.blockHeightTolerance)
	return c.transportWriter.SendDelegationRejection(ctx, e.FromNode, &engineProto.DelegationRejection{
		DelegationId:           e.DelegationID,
		DelegateNodeId:         e.FromNode,
		ContractAddress:        c.contractAddress.HexString(),
		RejectionReason:        engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
		OriginatorBlockHeight:  int64(e.OriginatorsBlockHeight),
		CoordinatorBlockHeight: c.currentBlockHeight,
		BlockHeightTolerance:   int64(c.blockHeightTolerance),
	})
}

func action_RejectDelegationRequest(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*TransactionsDelegatedEvent)
	c.recordOriginatorActivity(e.FromNode)
	return c.transportWriter.SendDelegationRejection(ctx, e.FromNode, &engineProto.DelegationRejection{
		DelegationId:           e.DelegationID,
		DelegateNodeId:         e.FromNode,
		ContractAddress:        c.contractAddress.HexString(),
		ActiveCoordinator:      c.currentActiveCoordinator,
		RejectionReason:        engineProto.RejectionReason_NOT_CURRENT_DELEGATE,
		OriginatorBlockHeight:  int64(e.OriginatorsBlockHeight),
		CoordinatorBlockHeight: c.currentBlockHeight,
	})
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

func action_AddEndorsersFromSnapshot(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*common.HeartbeatReceivedEvent)
	c.updateEndorserCandidates(ctx, e.CoordinatorSnapshot.EndorserCandidates...)
	return nil
}
