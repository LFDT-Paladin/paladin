/*
 * Copyright © 2025 Kaleido, Inc.
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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_String_Idle(t *testing.T) {
	result := State_Idle.String()
	assert.Equal(t, "Idle", result, "State.String() should return the correct string representation")
}

func TestState_String_Observing(t *testing.T) {
	result := State_Observing.String()
	assert.Equal(t, "Observing", result, "State.String() should return the correct string representation")
}

func TestState_String_Elect(t *testing.T) {
	result := State_Elect.String()
	assert.Equal(t, "Elect", result, "State.String() should return the correct string representation")
}

func TestState_String_Standby(t *testing.T) {
	result := State_Standby.String()
	assert.Equal(t, "Standby", result, "State.String() should return the correct string representation")
}

func TestState_String_Prepared(t *testing.T) {
	result := State_Prepared.String()
	assert.Equal(t, "Prepared", result, "State.String() should return the correct string representation")
}

func TestState_String_Active(t *testing.T) {
	result := State_Active.String()
	assert.Equal(t, "Active", result, "State.String() should return the correct string representation")
}

func TestState_String_Flush(t *testing.T) {
	result := State_Flush.String()
	assert.Equal(t, "Flush", result, "State.String() should return the correct string representation")
}

func TestState_String_Closing(t *testing.T) {
	result := State_Closing.String()
	assert.Equal(t, "Closing", result, "State.String() should return the correct string representation")
}

func TestState_String_InvalidState(t *testing.T) {
	result := State(999).String()
	assert.Equal(t, "Unknown", result, "State.String() should return the correct string representation")
}

func TestState_String_NegativeState(t *testing.T) {
	result := State(-1).String()
	assert.Equal(t, "Unknown", result, "State.String() should return the correct string representation")
}

func Test_queueEventInternal_QueuesPriorityEvent(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	c, _ := builder.Build(ctx)

	syncEvent := statemachine.NewSyncEvent()
	c.queueEventInternal(ctx, syncEvent)
	<-syncEvent.Done
	require.False(t, c.stateMachineEventLoop.IsStopped(), "event loop should still be running")
}
