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
package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStateMachine_InitializeOK(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(ctx)
	assert.NotNil(t, txn)
	assert.Equal(t, State_Initial, txn.stateMachine.CurrentState, "current state is %s", txn.stateMachine.CurrentState.String())
}

func Test_State_String_AllStates(t *testing.T) {
	tests := []struct {
		state  State
		expect string
	}{
		{State_Initial, "State_Initial"},
		{State_Pooled, "State_Pooled"},
		{State_PreAssembly_Blocked, "State_PreAssembly_Blocked"},
		{State_Assembling, "State_Assembling"},
		{State_Reverted, "State_Reverted"},
		{State_Endorsement_Gathering, "State_Endorsement_Gathering"},
		{State_Blocked, "State_Blocked"},
		{State_Confirming_Dispatchable, "State_Confirming_Dispatchable"},
		{State_Ready_For_Dispatch, "State_Ready_For_Dispatch"},
		{State_Dispatched, "State_Dispatched"},
		{State_SubmissionPrepared, "State_SubmissionPrepared"},
		{State_Submitted, "State_Submitted"},
		{State_Confirmed, "State_Confirmed"},
		{State_Final, "State_Final"},
	}
	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.state.String())
		})
	}
}

func Test_State_String_Unknown(t *testing.T) {
	// State value beyond defined constants
	s := State(99)
	assert.Contains(t, s.String(), "Unknown")
	assert.Contains(t, s.String(), "99")
}

func Test_action_IncrementHeartbeatIntervalsSinceStateChange_IncrementsCounter(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		HeartbeatIntervalsSinceStateChange(2).
		Build(ctx)

	err := action_IncrementHeartbeatIntervalsSinceStateChange(ctx, txn, nil)
	assert.NoError(t, err)
	assert.Equal(t, 3, txn.heartbeatIntervalsSinceStateChange)
}
