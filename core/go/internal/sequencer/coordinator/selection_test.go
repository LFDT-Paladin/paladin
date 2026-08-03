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
	"github.com/stretchr/testify/mock"
)

func Test_calculateCoordinatorPriorities_SingleNodeInPool_SetsPriorityList(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1").
		CoordinatorSelectionBlockRange(100).
		CurrentBlockHeight(1000).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.calculateCoordinatorPriorities(ctx)
	assert.Equal(t, []string{"node1"}, c.coordinatorPriorityList)
}

func Test_calculateCoordinatorPriorities_MultipleNodesInPool_SetsNonEmptyPriorityList(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionBlockRange(100).
		CurrentBlockHeight(1000).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.calculateCoordinatorPriorities(ctx)
	assert.NotEmpty(t, c.coordinatorPriorityList)
	assert.Contains(t, []string{"node1", "node2", "node3"}, c.coordinatorPriorityList[0])
}

func Test_calculateCoordinatorPriorities_BlockHeightRounding_SameRangeSamePriorityList(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2", "node3").
		CoordinatorSelectionBlockRange(100).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.effectiveBlockHeight = common.ComputeEffectiveBlockHeight(1000, 100)
	c.calculateCoordinatorPriorities(ctx)
	list1 := c.coordinatorPriorityList[0]

	c.effectiveBlockHeight = common.ComputeEffectiveBlockHeight(1001, 100)
	c.calculateCoordinatorPriorities(ctx)
	list2 := c.coordinatorPriorityList[0]

	c.effectiveBlockHeight = common.ComputeEffectiveBlockHeight(1099, 100)
	c.calculateCoordinatorPriorities(ctx)
	list3 := c.coordinatorPriorityList[0]

	assert.Equal(t, list1, list2)
	assert.Equal(t, list2, list3)

	c.effectiveBlockHeight = common.ComputeEffectiveBlockHeight(1100, 100)
	c.calculateCoordinatorPriorities(ctx)
	assert.Contains(t, []string{"node1", "node2", "node3"}, c.coordinatorPriorityList[0])
}

func Test_calculateCoordinatorPriorities_DifferentBlockRanges_CanSelectDifferentPriorityHeads(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1", "node2").
		CoordinatorSelectionBlockRange(50).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	c.effectiveBlockHeight = common.ComputeEffectiveBlockHeight(100, 50)
	c.calculateCoordinatorPriorities(ctx)
	head1 := c.coordinatorPriorityList[0]

	c.effectiveBlockHeight = common.ComputeEffectiveBlockHeight(150, 50)
	c.calculateCoordinatorPriorities(ctx)
	head2 := c.coordinatorPriorityList[0]

	assert.Contains(t, []string{"node1", "node2"}, head1)
	assert.Contains(t, []string{"node1", "node2"}, head2)
}

func Test_calculateCoordinatorPriorities_SenderMode_NoOp(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		CurrentActiveCoordinator("node1").
		Build()

	c.calculateCoordinatorPriorities(ctx)
	assert.Empty(t, c.coordinatorPriorityList, "SENDER mode must not compute a priority list")
}


func Test_refreshBlockHeight_SetsEffectiveBlockHeight(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		EndorserCandidates("node1").
		CoordinatorSelectionBlockRange(100).
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(1000))

	c.refreshBlockHeight(ctx)
	assert.Equal(t, int64(1000), c.currentBlockHeight)
	assert.Equal(t, uint64(1000), c.effectiveBlockHeight)
}
