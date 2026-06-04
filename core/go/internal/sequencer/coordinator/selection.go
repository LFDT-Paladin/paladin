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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// action_CalculateCoordinatorPriorities recomputes the coordinator priority list for the current
// block height and epoch. No-op in STATIC and SENDER modes.
func action_CalculateCoordinatorPriorities(ctx context.Context, c *coordinator, _ common.Event) error {
	if c.coordinatorSelection != prototk.ContractConfig_COORDINATOR_ENDORSER {
		return nil
	}
	c.coordinatorPriorityList = common.ComputeCoordinatorPriorityList(
		ctx,
		c.endorserCandidates,
		c.currentBlockHeight,
		c.coordinatorSelectionBlockRange,
	)
	return nil
}

func validator_IsOnEpochBoundary(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	_, isOnBoundary := common.DecodeNewBlockHeight(c.currentBlockHeight, c.coordinatorSelectionBlockRange, event)
	return isOnBoundary, nil
}

func action_UpdateBlockHeight(ctx context.Context, c *coordinator, event common.Event) error {
	c.currentBlockHeight, _ = common.DecodeNewBlockHeight(c.currentBlockHeight, c.coordinatorSelectionBlockRange, event)
	c.grapher.ForgetLocks(ctx, c.currentBlockHeight)
	return nil
}
