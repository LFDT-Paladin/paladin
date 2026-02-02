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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_action_NewBlock_SetsCurrentBlockHeight(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Standby)
	c, _ := builder.Build(ctx)

	err := action_NewBlock(ctx, c, &NewBlockEvent{BlockHeight: 1000})
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), c.currentBlockHeight)
}

func Test_action_EndorsementRequested_SetsActiveCoordinatorAndUpdatesPool(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Initial)
	c, _ := builder.Build(ctx)

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
	c, _ := builder.Build(ctx)

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
	c, _ := builder.Build(ctx)
	event := &HeartbeatReceivedEvent{}
	event.From = "node1"
	event.ContractAddress = &contractAddress
	event.BlockHeight = 2000
	signerAddr := pldtypes.RandAddress()
	event.FlushPoints = []*common.FlushPoint{
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
	c, mocks := builder.Build(ctx)
	c.activeCoordinatorNode = "otherNode"

	err := action_SendHandoverRequest(ctx, c, nil)
	require.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "SendHandoverRequest should be called")
}

func Test_action_Idle_CallsCoordinatorIdle(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _ := builder.Build(ctx)
	err := action_Idle(ctx, c, nil)
	require.NoError(t, err)
}

func Test_action_Idle_CancelsHeartbeatWhenSet(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Observing)
	c, _ := builder.Build(ctx)
	heartbeatCtx, cancel := context.WithCancel(ctx)
	c.heartbeatCtx = heartbeatCtx
	c.heartbeatCancel = cancel

	err := action_Idle(ctx, c, nil)
	require.NoError(t, err)
	select {
	case <-heartbeatCtx.Done():
		// heartbeatCancel was called and context is cancelled
	default:
		t.Fatal("heartbeatCancel should have been called, context should be cancelled")
	}
}
