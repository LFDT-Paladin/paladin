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

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_action_UpdateBlockHeight_SetsCurrentBlockHeight(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _, done := builder.Build(ctx)
	defer done()

	err := action_UpdateBlockHeight(ctx, c, &NewBlockEvent{BlockHeight: 1000})
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), c.currentBlockHeight)
}

func Test_action_HeartbeatReceived_SetsActiveCoordinatorState(t *testing.T) {
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
	assert.Contains(t, c.originatorNodePool, "node1")
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
	c.heartbeatGracePeriod = 10
	c.heartbeatIntervalsSinceLastReceive = 5

	assert.False(t, guard_HeartbeatThresholdExceeded(ctx, c))
}

func Test_guard_ObservingIdleThresholdExceeded_ExactlyMet(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _, done := builder.Build(ctx)
	defer done()
	c.heartbeatGracePeriod = 10
	c.heartbeatIntervalsSinceLastReceive = 10

	assert.True(t, guard_HeartbeatThresholdExceeded(ctx, c))
}

func Test_guard_ObservingIdleThresholdExceeded_Exceeded(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _, done := builder.Build(ctx)
	defer done()
	c.heartbeatGracePeriod = 10
	c.heartbeatIntervalsSinceLastReceive = 15

	assert.True(t, guard_HeartbeatThresholdExceeded(ctx, c))
}
