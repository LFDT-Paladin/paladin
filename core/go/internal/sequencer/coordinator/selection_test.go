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
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		Build(ctx)

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
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity@node1"),
	})

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func Test_selectActiveCoordinatorNode_StaticMode_StaticCoordinatorWithIdentityOnly_ReturnsError(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity"),
	})

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.Error(t, err)
	assert.Empty(t, coordinatorNode)
}

func Test_selectActiveCoordinatorNode_StaticMode_EmptyStaticCoordinator_ReturnsError(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String(""),
	})

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.Error(t, err)
	assert.Empty(t, coordinatorNode)
	assert.Contains(t, err.Error(), "static coordinator mode is configured but static coordinator node is not set")
}

func Test_selectActiveCoordinatorNode_EndorserMode_EmptyPool_ReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool().
		CurrentBlockHeight(1000)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Empty(t, coordinatorNode)
}

func Test_selectActiveCoordinatorNode_EndorserMode_SingleNodeInPool_ReturnsNode(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool("node1").
		CurrentBlockHeight(1000)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func Test_selectActiveCoordinatorNode_EndorserMode_MultipleNodesInPool_ReturnsOneOfPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool("node1", "node2", "node3").
		CurrentBlockHeight(1000)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Contains(t, []string{"node1", "node2", "node3"}, coordinatorNode)
}

func Test_selectActiveCoordinatorNode_EndorserMode_BlockHeightRounding_SameRangeSameCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool("node1", "node2", "node3").
		CurrentBlockHeight(1000)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

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
	builder := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool("node1", "node2").
		CurrentBlockHeight(100)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(50))
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
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
	c, mocks := builder.Build(ctx)
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	assert.Equal(t, "node1", c.nodeName)

	coordinatorNode, err := c.selectActiveCoordinatorNode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "node1", coordinatorNode)
}

func Test_updateOriginatorNodePool_AddsNodeToEmptyPool(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool().
		Build(ctx)

	c.updateOriginatorNodePool("node2")

	assert.Equal(t, 2, len(c.originatorNodePool), "pool should contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
}

func Test_updateOriginatorNodePool_AddsNodeToNonEmptyPool(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool("node1", "node3").
		Build(ctx)

	c.updateOriginatorNodePool("node2")

	assert.Equal(t, 3, len(c.originatorNodePool), "pool should contain 3 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
	assert.Contains(t, c.originatorNodePool, "node3", "pool should contain node3")
}

func Test_updateOriginatorNodePool_DoesNotAddDuplicateNode(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool("node1", "node2").
		Build(ctx)

	c.updateOriginatorNodePool("node2")

	assert.Equal(t, 2, len(c.originatorNodePool), "pool should still contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
}

func Test_updateOriginatorNodePool_EnsuresCoordinatorsOwnNodeIsAlwaysInPool(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool().
		Build(ctx)

	c.updateOriginatorNodePool("node2")

	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
	assert.Equal(t, 2, len(c.originatorNodePool), "pool should contain 2 nodes")
}

func Test_updateOriginatorNodePool_EnsuresCoordinatorsOwnNodeIsAddedEvenWhenPoolAlreadyHasOtherNodes(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool("node2", "node3").
		Build(ctx)

	c.updateOriginatorNodePool("node4")

	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
	assert.Equal(t, 4, len(c.originatorNodePool), "pool should contain 4 nodes")
}

func Test_updateOriginatorNodePool_DoesNotDuplicateCoordinatorsOwnNode(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool("node1", "node2").
		Build(ctx)

	c.updateOriginatorNodePool("node1")

	assert.Equal(t, 2, len(c.originatorNodePool), "pool should still contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain node1")
	assert.Contains(t, c.originatorNodePool, "node2", "pool should contain node2")
}

func Test_updateOriginatorNodePool_HandlesMultipleSequentialUpdates(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool().
		Build(ctx)

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
	c, _ := NewCoordinatorBuilderForTesting(t, State_Idle).
		OriginatorIdentityPool().
		Build(ctx)

	c.updateOriginatorNodePool("")

	assert.Equal(t, 2, len(c.originatorNodePool), "pool should contain 2 nodes")
	assert.Contains(t, c.originatorNodePool, "", "pool should contain empty string")
	assert.Contains(t, c.originatorNodePool, "node1", "pool should contain coordinator's own node")
}

func Test_action_SelectActiveCoordinator_StaticModeSelectsCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial)
	c, mocks := builder.Build(ctx)
	c.Start()
	defer c.Stop()
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity@node1"),
	})

	c.QueueEvent(ctx, &CoordinatorCreatedEvent{})

	require.Eventually(t, func() bool {
		return c.GetCurrentState() == State_Idle && c.activeCoordinatorNode == "node1"
	}, time.Second, 10*time.Millisecond)
}

func Test_action_SelectActiveCoordinator_SenderModeSelectsSelf(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial)
	c, mocks := builder.Build(ctx)
	c.Start()
	defer c.Stop()
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})

	c.QueueEvent(ctx, &CoordinatorCreatedEvent{})

	require.Eventually(t, func() bool {
		return c.activeCoordinatorNode == "node1" && c.GetCurrentState() == State_Idle
	}, time.Second, 10*time.Millisecond)
}

func Test_action_SelectActiveCoordinator_EndorserModeSelectsFromPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial).
		OriginatorIdentityPool("node1", "node2", "node3").
		CurrentBlockHeight(1000)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	c.Start()
	defer c.Stop()
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

	c.QueueEvent(ctx, &CoordinatorCreatedEvent{})

	require.Eventually(t, func() bool {
		return c.GetCurrentState() == State_Idle
	}, time.Second*1000000000, 10*time.Millisecond)
	assert.Contains(t, []string{"node1", "node2", "node3"}, c.activeCoordinatorNode)
}

func Test_action_SelectActiveCoordinator_EmptyPoolLeavesSelectingState(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial).
		OriginatorIdentityPool().
		CurrentBlockHeight(1000)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, mocks := builder.Build(ctx)
	c.Start()
	defer c.Stop()
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

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
	c, mocks := builder.Build(ctx)
	c.Start()
	defer c.Stop()
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    proto.String("identity"),
	})

	err := action_SelectActiveCoordinator(ctx, c, nil)
	require.NoError(t, err)
	assert.Empty(t, c.activeCoordinatorNode, "activeCoordinatorNode should remain empty when select returns error")
}
