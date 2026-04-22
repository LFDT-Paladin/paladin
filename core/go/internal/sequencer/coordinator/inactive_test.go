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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_action_NewBlock_SetsCurrentBlockHeight(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Standby)
	c, _, done := builder.Build(ctx)
	defer done()

	err := action_NewBlock(ctx, c, &NewBlockEvent{BlockHeight: 1000})
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), c.currentBlockHeight)
}

func Test_action_EndorsementRequested_SetsActiveCoordinatorAndUpdatesPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial)
	c, _, done := builder.Build(ctx)
	defer done()

	err := action_EndorsementRequested(ctx, c, &EndorsementRequestedEvent{From: "node1"})
	require.NoError(t, err)
	assert.Equal(t, "node1", c.activeCoordinatorNode)
	assert.Contains(t, c.originatorNodePool, "node1")
}

func Test_action_HeartbeatReceived_SetsActiveCoordinatorBlockHeightAndUpdatesPool(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial).ContractAddress(addr)
	contractAddress := builder.GetContractAddress()
	c, _, done := builder.Build(ctx)
	defer done()

	event := &HeartbeatReceivedEvent{}
	event.From = "node1"
	event.ContractAddress = &contractAddress
	event.BlockHeight = 2000

	err := action_HeartbeatReceived(ctx, c, event)
	require.NoError(t, err)
	assert.Equal(t, "node1", c.activeCoordinatorNode)
	assert.Equal(t, uint64(2000), c.activeCoordinatorBlockHeight)
	assert.Contains(t, c.originatorNodePool, "node1")
}

func Test_action_HeartbeatReceived_StoresFlushPoints(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial).ContractAddress(addr)
	contractAddress := builder.GetContractAddress()
	c, _, done := builder.Build(ctx)
	defer done()
	event := &HeartbeatReceivedEvent{}
	event.From = "node1"
	event.ContractAddress = &contractAddress
	event.BlockHeight = 2000
	signerAddr := pldtypes.RandAddress()
	event.FlushPoints = []*common.SnapshotFlushPoint{
		{From: *signerAddr, Nonce: 42, Hash: pldtypes.Bytes32{}},
	}

	err := action_HeartbeatReceived(ctx, c, event)
	require.NoError(t, err)
	key := event.FlushPoints[0].GetSignerNonce()
	assert.NotEmpty(t, key)
	assert.Equal(t, event.FlushPoints[0], c.activeCoordinatorsFlushPointsBySignerNonce[key])
}

func Test_action_SendHandoverRequest_CallsSendHandoverRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Elect)
	c, mocks, done := builder.Build(ctx)
	defer done()
	c.activeCoordinatorNode = "otherNode"

	err := action_SendHandoverRequest(ctx, c, nil)
	require.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "SendHandoverRequest should be called")
}

func Test_action_Idle_CallsCoordinatorIdle(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _, done := builder.Build(ctx)
	defer done()
	err := action_Idle(ctx, c, nil)
	require.NoError(t, err)
}

func Test_action_ResetHeartbeatIntervalsSinceLastReceive(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _, done := builder.Build(ctx)
	defer done()
	c.heartbeatIntervalsSinceLastReceive = 7

	err := action_ResetHeartbeatIntervalsSinceLastReceive(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, c.heartbeatIntervalsSinceLastReceive)
}

func Test_action_IncrementHeartbeatIntervalsSinceLastReceive(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _, done := builder.Build(ctx)
	defer done()
	c.heartbeatIntervalsSinceLastReceive = 3

	err := action_IncrementHeartbeatIntervalsSinceLastReceive(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, 4, c.heartbeatIntervalsSinceLastReceive)
}

func Test_guard_ObservingIdleThresholdExceeded_NotExceeded(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _, done := builder.Build(ctx)
	defer done()
	c.inactiveToIdleGracePeriod = 10
	c.heartbeatIntervalsSinceLastReceive = 5

	assert.False(t, guard_ObservingIdleThresholdExceeded(ctx, c))
}

func Test_guard_ObservingIdleThresholdExceeded_ExactlyMet(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _, done := builder.Build(ctx)
	defer done()
	c.inactiveToIdleGracePeriod = 10
	c.heartbeatIntervalsSinceLastReceive = 10

	assert.True(t, guard_ObservingIdleThresholdExceeded(ctx, c))
}

func Test_guard_ObservingIdleThresholdExceeded_Exceeded(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _, done := builder.Build(ctx)
	defer done()
	c.inactiveToIdleGracePeriod = 10
	c.heartbeatIntervalsSinceLastReceive = 15

	assert.True(t, guard_ObservingIdleThresholdExceeded(ctx, c))
}

func Test_action_NewBlock_ResetsFailoverIndex_OnBlockRangeBoundary(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Standby)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	c.currentBlockHeight = 1000 // effective range: 1000
	c.coordinatorFailoverIndex = 2

	// A block still within the same range should NOT reset the failover index
	err := action_NewBlock(ctx, c, &NewBlockEvent{BlockHeight: 1099})
	require.NoError(t, err)
	assert.Equal(t, 2, c.coordinatorFailoverIndex, "failover index should not reset within the same block range")

	// A block crossing the boundary SHOULD reset the failover index
	err = action_NewBlock(ctx, c, &NewBlockEvent{BlockHeight: 1100})
	require.NoError(t, err)
	assert.Equal(t, uint64(1100), c.currentBlockHeight)
	assert.Equal(t, 0, c.coordinatorFailoverIndex, "failover index should reset when block range boundary is crossed")
}

func Test_action_NewBlock_DoesNotResetFailoverIndex_WhenAlreadyZero(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Standby)
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	c.currentBlockHeight = 1000
	c.coordinatorFailoverIndex = 0 // already zero

	err := action_NewBlock(ctx, c, &NewBlockEvent{BlockHeight: 1100}) // range boundary
	require.NoError(t, err)
	assert.Equal(t, 0, c.coordinatorFailoverIndex, "failover index remains 0 when crossing boundary with index already at 0")
}

func Test_action_HeartbeatReceived_ResetsFailoverIndex(t *testing.T) {
	ctx := context.Background()
	addr := pldtypes.RandAddress()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing).ContractAddress(addr)
	contractAddress := builder.GetContractAddress()
	c, _, done := builder.Build(ctx)
	defer done()

	c.coordinatorFailoverIndex = 3 // some previous failover attempts

	event := &HeartbeatReceivedEvent{}
	event.From = "node1"
	event.ContractAddress = &contractAddress
	event.BlockHeight = 2000

	err := action_HeartbeatReceived(ctx, c, event)
	require.NoError(t, err)
	assert.Equal(t, 0, c.coordinatorFailoverIndex, "receiving a valid heartbeat should reset the failover index")
}

func Test_action_Idle_ResetsFailoverIndex(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _, done := builder.Build(ctx)
	defer done()
	c.coordinatorFailoverIndex = 2

	err := action_Idle(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, c.coordinatorFailoverIndex, "entering idle state should reset the failover index")
}

func Test_Observing_FailoverThenEventuallyIdle_IntegrationFlow(t *testing.T) {
	// Full state-machine integration test: 3 nodes, primary coordinator goes offline,
	// failover cycles through the pool, all unresponsive, ends up in State_Idle.
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	config := builder.GetSequencerConfig()
	config.BlockRange = confutil.P(uint64(100))
	config.InactiveToIdleGracePeriod = confutil.P(2) // short threshold for the test
	builder.OverrideSequencerConfig(config)
	c, _, done := builder.Build(ctx)
	defer done()

	c.originatorNodePool = []string{"node1", "node2", "node3"}
	c.currentBlockHeight = 1000
	c.coordinatorFailoverIndex = 0

	// Pick the primary coordinator and start observing it
	primaryCoordinator, _ := c.selectActiveCoordinatorNode(ctx)
	c.activeCoordinatorNode = primaryCoordinator

	// Queue enough heartbeat intervals to exhaust all 3 candidates (3 candidates × 2 threshold = 6 intervals)
	// After 2 intervals: primary candidate times out → failover to candidate 2
	// After 2 more intervals: candidate 2 times out → failover to candidate 3
	// After 2 more intervals: candidate 3 times out → all exhausted → State_Idle
	for i := 0; i < 6; i++ {
		c.QueueEvent(ctx, &common.HeartbeatIntervalEvent{})
	}

	require.Eventually(t, func() bool {
		return c.GetCurrentState() == State_Idle
	}, 2*time.Second, 10*time.Millisecond, "should eventually reach State_Idle after all coordinator candidates are exhausted")
	assert.Equal(t, 0, c.coordinatorFailoverIndex, "failover index should be reset after going idle")
}
