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

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func Test_action_UpdateOriginatorNodePoolFromEvent_AddsNodesToPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{}

	err := action_UpdateOriginatorNodePoolFromEvent(ctx, c, &OriginatorNodePoolUpdateRequestedEvent{
		Nodes: []string{"node2", "node3"},
	})
	require.NoError(t, err)
	assert.Len(t, c.originatorNodePool, 3, "pool should contain event nodes plus coordinator's own node")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2 from event")
	assert.Contains(t, c.originatorNodePool, "node3", "pool should contain node3 from event")
}

func Test_selectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithFullyQualifiedIdentity_ReturnsNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity@node1"),
	})
	c, _, done := builder.Build(ctx)
	defer done()

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func Test_selectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithIdentityOnly_ReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity"),
	})
	c, _, done := builder.Build(ctx)
	defer done()

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.Error(t, err)
	assert.Empty(t, coordinatorNode)
}

func Test_selectActiveCoordinatorNode_StaticMode_EmptyStaticCoordinator_ReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String(""),
	})
	c, _, done := builder.Build(ctx)
	defer done()

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.Error(t, err)
	assert.Empty(t, coordinatorNode)
	assert.Contains(t, err.Error(), "static coordinator mode is configured but static coordinator node is not set")
}

func Test_selectActiveCoordinatorNode_EndorserMode_EmptyPool_ReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{}
	c.currentBlockHeight = 1000

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Empty(t, coordinatorNode)
}

func Test_selectActiveCoordinatorNode_EndorserMode_SingleNodeInPool_ReturnsNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1"}
	c.currentBlockHeight = 1000

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func Test_selectActiveCoordinatorNode_EndorserMode_MultipleNodesInPool_ReturnsOneOfPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Contains(t, []string{"node1", "node2", "node3"}, coordinatorNode)
}

func Test_selectActiveCoordinatorNode_EndorserMode_BlockHeightRounding_SameRangeSameCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2", "node3"}

	c.currentBlockHeight = 1000
	coordinatorNode1, err1 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err1)
	c.currentBlockHeight = 1001
	coordinatorNode2, err2 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err2)
	c.currentBlockHeight = 1099
	coordinatorNode3, err3 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err3)
	assert.Equal(t, coordinatorNode1, coordinatorNode2)
	assert.Equal(t, coordinatorNode2, coordinatorNode3)

	c.currentBlockHeight = 1100
	coordinatorNode4, err4 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err4)
	assert.Contains(t, []string{"node1", "node2", "node3"}, coordinatorNode4)
}

func Test_selectActiveCoordinatorNode_EndorserMode_DifferentBlockRanges_CanSelectDifferentCoordinators(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(50))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2"}

	c.currentBlockHeight = 100
	coordinatorNode1, err1 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err1)
	c.currentBlockHeight = 150
	coordinatorNode2, err2 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err2)
	assert.Contains(t, []string{"node1", "node2"}, coordinatorNode1)
	assert.Contains(t, []string{"node1", "node2"}, coordinatorNode2)
}

func Test_selectActiveCoordinatorNode_SenderMode_ReturnsCurrentNodeName(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _, done := builder.Build(ctx)
	defer done()
	assert.Equal(t, "node1", c.nodeName)

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func Test_updateOriginatorNodePool_AddsNodeToEmptyPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{}

	c.updateOriginatorNodePool("node2")

	assert.Equal(t, 2, len(c.originatorNodePool), "pool should contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
}

func Test_updateOriginatorNodePool_AddsNodeToNonEmptyPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node3"}

	c.updateOriginatorNodePool("node2")

	assert.Equal(t, 3, len(c.originatorNodePool), "pool should contain 3 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
	assert.Contains(t, c.originatorNodePool, "node3", "pool should contain node3")
}

func Test_updateOriginatorNodePool_DoesNotAddDuplicateNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2"}

	c.updateOriginatorNodePool("node2")

	assert.Equal(t, 2, len(c.originatorNodePool), "pool should still contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
}

func Test_updateOriginatorNodePool_EnsuresCoordinatorsOwnNodeIsAlwaysInPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{}

	c.updateOriginatorNodePool("node2")

	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
	assert.Equal(t, 2, len(c.originatorNodePool), "pool should contain 2 nodes")
}

func Test_updateOriginatorNodePool_EnsuresCoordinatorsOwnNodeIsAddedEvenWhenPoolAlreadyHasOtherNodes(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node2", "node3"}

	c.updateOriginatorNodePool("node4")

	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
	assert.Equal(t, 4, len(c.originatorNodePool), "pool should contain 4 nodes")
}

func Test_updateOriginatorNodePool_DoesNotDuplicateCoordinatorsOwnNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2"}

	c.updateOriginatorNodePool("node1")

	assert.Equal(t, 2, len(c.originatorNodePool), "pool should still contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
}

func Test_updateOriginatorNodePool_HandlesMultipleSequentialUpdates(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{}

	c.updateOriginatorNodePool("node2")
	c.updateOriginatorNodePool("node3")
	c.updateOriginatorNodePool("node4")

	assert.Equal(t, 4, len(c.originatorNodePool), "pool should contain 4 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
	assert.Contains(t, c.originatorNodePool, "node3", "pool should contain node3")
	assert.Contains(t, c.originatorNodePool, "node4", "pool should contain node4")
}

func Test_updateOriginatorNodePool_HandlesEmptyStringNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{}

	c.updateOriginatorNodePool("")

	assert.Equal(t, 2, len(c.originatorNodePool), "pool should contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "", "pool should contain empty string")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
}

func Test_action_SelectActiveCoordinator_StaticModeSelectsCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity@node1"),
	})
	c, _, done := builder.Build(ctx)
	defer done()
	c.activeCoordinatorNode = ""

	c.QueueEvent(ctx, &CoordinatorCreatedEvent{})

	require.Eventually(t, func() bool {
		return c.GetCurrentState() == State_Idle && c.activeCoordinatorNode == "node1"
	}, time.Second, 10*time.Millisecond)
}

func Test_action_SelectActiveCoordinator_SenderModeSelectsSelf(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _, done := builder.Build(ctx)
	defer done()
	c.activeCoordinatorNode = ""

	c.QueueEvent(ctx, &CoordinatorCreatedEvent{})

	require.Eventually(t, func() bool {
		return c.activeCoordinatorNode == "node1" && c.GetCurrentState() == State_Idle
	}, time.Second, 10*time.Millisecond)
}

func Test_action_SelectActiveCoordinator_EndorserModeSelectsFromPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.activeCoordinatorNode = ""
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000

	c.QueueEvent(ctx, &CoordinatorCreatedEvent{})

	require.Eventually(t, func() bool {
		return c.GetCurrentState() == State_Idle
	}, time.Second*1000000000, 10*time.Millisecond)
	assert.Contains(t, []string{"node1", "node2", "node3"}, c.activeCoordinatorNode)
}

func Test_action_SelectActiveCoordinator_EmptyPoolLeavesSelectingState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.activeCoordinatorNode = ""
	c.originatorNodePool = []string{}
	c.currentBlockHeight = 1000

	c.QueueEvent(ctx, &CoordinatorCreatedEvent{})

	syncEvent := statemachine.NewSyncEvent()
	c.QueueEvent(ctx, syncEvent)
	<-syncEvent.Done

	assert.Equal(t, State_Initial, c.GetCurrentState())
	assert.Empty(t, c.activeCoordinatorNode)
}

func Test_action_SelectActiveCoordinator_WhenSelectReturnsError_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity"),
	})
	c, _, done := builder.Build(ctx)
	defer done()
	c.activeCoordinatorNode = ""

	err := action_SelectActiveCoordinator(ctx, c, nil)
	require.NoError(t, err)
	assert.Empty(t, c.activeCoordinatorNode, "activeCoordinatorNode should remain empty when select returns error")
}

func Test_selectActiveCoordinatorNode_EndorserMode_FailoverIndex0_SameAsPrimarySelection(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000
	c.coordinatorFailoverIndex = 0

	coordinator0, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Contains(t, []string{"node1", "node2", "node3"}, coordinator0)
}

func Test_selectActiveCoordinatorNode_EndorserMode_FailoverIndex1_SelectsDifferentNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000

	c.coordinatorFailoverIndex = 0
	primary, err0 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err0)

	c.coordinatorFailoverIndex = 1
	failover1, err1 := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err1)

	assert.NotEqual(t, primary, failover1, "failover index 1 should select a different node than primary")
	assert.Contains(t, []string{"node1", "node2", "node3"}, failover1)
}

func Test_selectActiveCoordinatorNode_EndorserMode_FailoverIndex2_SelectsThirdOption(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000

	c.coordinatorFailoverIndex = 0
	p0, _ := c.selectActiveCoordinatorNode(ctx)
	c.coordinatorFailoverIndex = 1
	p1, _ := c.selectActiveCoordinatorNode(ctx)
	c.coordinatorFailoverIndex = 2
	p2, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)

	// All three failover slots should be distinct nodes covering the whole pool
	selected := []string{p0, p1, p2}
	assert.ElementsMatch(t, []string{"node1", "node2", "node3"}, selected,
		"the three failover indices should select each node in the pool exactly once")
}

func Test_selectActiveCoordinatorNode_EndorserMode_FailoverWrapsAround(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000

	c.coordinatorFailoverIndex = 0
	p0, _ := c.selectActiveCoordinatorNode(ctx)
	c.coordinatorFailoverIndex = 3 // wraps around to the same as index 0
	p3, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, p0, p3, "failoverIndex=3 should wrap around to the same node as failoverIndex=0 for a 3-node pool")
}

func Test_action_AttemptCoordinatorFailover_EndorserMode_SingleNodePool_GoesIdle(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1"}
	c.activeCoordinatorNode = "node1"

	err := action_AttemptCoordinatorFailover(ctx, c, nil)
	require.NoError(t, err)
	assert.Empty(t, c.activeCoordinatorNode, "single-node pool should result in idle (no other candidates)")
}

func Test_action_AttemptCoordinatorFailover_EndorserMode_MultiNodePool_SelectsNextCandidate(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000
	c.coordinatorFailoverIndex = 0

	// Record the primary coordinator
	primaryCoordinator, _ := c.selectActiveCoordinatorNode(ctx)
	c.activeCoordinatorNode = primaryCoordinator
	c.heartbeatIntervalsSinceLastReceive = 5 // simulates having waited

	err := action_AttemptCoordinatorFailover(ctx, c, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, c.coordinatorFailoverIndex, "failover index should be incremented to 1")
	assert.NotEqual(t, primaryCoordinator, c.activeCoordinatorNode, "failover should select a different node from the primary")
	assert.Contains(t, []string{"node1", "node2", "node3"}, c.activeCoordinatorNode)
	assert.Equal(t, 0, c.heartbeatIntervalsSinceLastReceive, "heartbeat counter should be reset so new candidate gets a fair window")
}

func Test_action_AttemptCoordinatorFailover_EndorserMode_AllNodesExhausted_GoesIdle(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2"}
	c.currentBlockHeight = 1000
	// Simulate already having tried node2 (index 1 was the last failover), so next increment
	// would bring us to 2 which equals len(pool)
	c.coordinatorFailoverIndex = 1
	c.activeCoordinatorNode = "node2"

	err := action_AttemptCoordinatorFailover(ctx, c, nil)
	require.NoError(t, err)

	assert.Empty(t, c.activeCoordinatorNode, "all candidates tried — should go idle")
	assert.Equal(t, 0, c.coordinatorFailoverIndex, "failover index should be reset after exhaustion")
}

func Test_action_AttemptCoordinatorFailover_StaticMode_GoesIdle(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity@node1"),
	})
	c, _, done := builder.Build(ctx)
	defer done()
	c.activeCoordinatorNode = "node1"

	err := action_AttemptCoordinatorFailover(ctx, c, nil)
	require.NoError(t, err)
	assert.Empty(t, c.activeCoordinatorNode, "static mode has no fallback; should go idle")
}

func Test_action_AttemptCoordinatorFailover_SenderMode_GoesIdle(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _, done := builder.Build(ctx)
	defer done()
	c.activeCoordinatorNode = "someNode"

	err := action_AttemptCoordinatorFailover(ctx, c, nil)
	require.NoError(t, err)
	assert.Empty(t, c.activeCoordinatorNode, "sender mode has no pool fallback; should go idle")
}

func Test_action_ReevaluateCoordinatorOnNewBlock_NonEndorserMode_IsNoop(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _, done := builder.Build(ctx)
	defer done()
	c.activeCoordinatorNode = "node1"
	original := c.activeCoordinatorNode

	err := action_ReevaluateCoordinatorOnNewBlock(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, original, c.activeCoordinatorNode, "non-endorser mode should be a no-op")
}

func Test_action_ReevaluateCoordinatorOnNewBlock_EmptyPool_IsNoop(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{}
	c.activeCoordinatorNode = ""

	err := action_ReevaluateCoordinatorOnNewBlock(ctx, c, nil)
	require.NoError(t, err)
	assert.Empty(t, c.activeCoordinatorNode, "empty pool should be a no-op")
}

func Test_action_ReevaluateCoordinatorOnNewBlock_SameSelection_NoUpdate(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000
	c.coordinatorFailoverIndex = 0

	selected, _ := c.selectActiveCoordinatorNode(ctx)
	c.activeCoordinatorNode = selected

	// Re-evaluate without changing block or failover index — same selection expected.
	err := action_ReevaluateCoordinatorOnNewBlock(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, selected, c.activeCoordinatorNode, "same block range and failover index should produce the same selection")
}

func Test_action_ReevaluateCoordinatorOnNewBlock_RangeChanged_UpdatesCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(50))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()
	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 100
	c.coordinatorFailoverIndex = 0

	firstSelected, _ := c.selectActiveCoordinatorNode(ctx)
	c.activeCoordinatorNode = firstSelected

	// Simulate crossing a block-range boundary (action_NewBlock would have updated currentBlockHeight
	// and reset coordinatorFailoverIndex if it was non-zero).
	c.currentBlockHeight = 150 // new range: 150 - (150 % 50) = 150

	// Now re-evaluate — may or may not change depending on the hash, but must not error.
	err := action_ReevaluateCoordinatorOnNewBlock(ctx, c, nil)
	require.NoError(t, err)
	assert.Contains(t, []string{"node1", "node2", "node3"}, c.activeCoordinatorNode)
}
