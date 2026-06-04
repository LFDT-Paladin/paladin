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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_action_CalculateCoordinatorPriorities_SingleNodeInPool_SetsPriorityList(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1").
		CoordinatorSelectionBlockRange(100).
		CurrentBlockHeight(1000).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	require.NoError(t, action_CalculateCoordinatorPriorities(ctx, c, nil))
	assert.Equal(t, []string{"node1"}, c.coordinatorPriorityList)
}

func Test_action_CalculateCoordinatorPriorities_MultipleNodesInPool_SetsNonEmptyPriorityList(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionBlockRange(100).
		CurrentBlockHeight(1000).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	require.NoError(t, action_CalculateCoordinatorPriorities(ctx, c, nil))
	assert.NotEmpty(t, c.coordinatorPriorityList)
	assert.Contains(t, []string{"node1", "node2", "node3"}, c.coordinatorPriorityList[0])
}

func Test_action_CalculateCoordinatorPriorities_BlockHeightRounding_SameRangeSamePriorityList(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionBlockRange(100).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	require.NoError(t, action_UpdateBlockHeight(ctx, c, &common.NewBlockEvent{BlockHeight: 1000}))
	require.NoError(t, action_CalculateCoordinatorPriorities(ctx, c, nil))
	list1 := c.coordinatorPriorityList[0]

	require.NoError(t, action_UpdateBlockHeight(ctx, c, &common.NewBlockEvent{BlockHeight: 1001}))
	require.NoError(t, action_CalculateCoordinatorPriorities(ctx, c, nil))
	list2 := c.coordinatorPriorityList[0]

	require.NoError(t, action_UpdateBlockHeight(ctx, c, &common.NewBlockEvent{BlockHeight: 1099}))
	require.NoError(t, action_CalculateCoordinatorPriorities(ctx, c, nil))
	list3 := c.coordinatorPriorityList[0]

	assert.Equal(t, list1, list2)
	assert.Equal(t, list2, list3)

	require.NoError(t, action_UpdateBlockHeight(ctx, c, &common.NewBlockEvent{BlockHeight: 1100}))
	require.NoError(t, action_CalculateCoordinatorPriorities(ctx, c, nil))
	assert.Contains(t, []string{"node1", "node2", "node3"}, c.coordinatorPriorityList[0])
}

func Test_action_CalculateCoordinatorPriorities_DifferentBlockRanges_CanSelectDifferentPriorityHeads(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionBlockRange(50).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	require.NoError(t, action_UpdateBlockHeight(ctx, c, &common.NewBlockEvent{BlockHeight: 100}))
	require.NoError(t, action_CalculateCoordinatorPriorities(ctx, c, nil))
	head1 := c.coordinatorPriorityList[0]

	require.NoError(t, action_UpdateBlockHeight(ctx, c, &common.NewBlockEvent{BlockHeight: 150}))
	require.NoError(t, action_CalculateCoordinatorPriorities(ctx, c, nil))
	head2 := c.coordinatorPriorityList[0]

	assert.Contains(t, []string{"node1", "node2"}, head1)
	assert.Contains(t, []string{"node1", "node2"}, head2)
}

func Test_action_CalculateCoordinatorPriorities_SenderMode_NoOp(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("node1").
		Build()

	require.NoError(t, action_CalculateCoordinatorPriorities(ctx, c, nil))
	assert.Empty(t, c.coordinatorPriorityList, "SENDER mode must not compute a priority list")
}

func Test_action_UpdateBlockHeight_SetsCurrentBlockHeight(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).Build()

	err := action_UpdateBlockHeight(ctx, c, &common.NewBlockEvent{BlockHeight: 1000})
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), c.currentBlockHeight)
}
