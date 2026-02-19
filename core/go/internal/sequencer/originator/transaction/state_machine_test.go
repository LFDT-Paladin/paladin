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
package transaction

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_action_CoordinatorChanged_SetsCurrentDelegate(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()
	coordinator := "newcoord@node1"
	event := &CoordinatorChangedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: coordinator,
	}
	err := action_CoordinatorChanged(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, coordinator, txn.currentDelegate)
}

func Test_State_String_AllStates(t *testing.T) {
	states := []struct {
		s    State
		want string
	}{
		{State_Initial, "State_Initial"},
		{State_Pending, "State_Pending"},
		{State_Delegated, "State_Delegated"},
		{State_Assembling, "State_Assembling"},
		{State_Endorsement_Gathering, "State_Endorsement_Gathering"},
		{State_Signing, "State_Signing"},
		{State_Prepared, "State_Prepared"},
		{State_Dispatched, "State_Dispatched"},
		{State_Sequenced, "State_Sequenced"},
		{State_Submitted, "State_Submitted"},
		{State_Confirmed, "State_Confirmed"},
		{State_Reverted, "State_Reverted"},
		{State_Parked, "State_Parked"},
		{State_Final, "State_Final"},
	}
	for _, tc := range states {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.s.String())
		})
	}
}

func Test_State_String_Unknown(t *testing.T) {
	s := State(999)
	assert.Equal(t, "Unknown", s.String())
}

func Test_HandleEvent_ProcessesEvent(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, mocks := builder.BuildWithMocks()
	coordinator := "coord@node1"
	event := &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: coordinator,
	}
	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, coordinator, txn.currentDelegate)
	// Transition callback should have been invoked
	require.Len(t, mocks.GetEmittedEvents(), 1)
}

func Test_initializeStateMachine_InvokesTransitionCallback(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, mocks := builder.BuildWithMocks()
	// Drive a transition so the callback runs (Created -> Pending)
	event := &CreatedEvent{
		BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
	}
	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	// Should have emitted a state transition event
	events := mocks.GetEmittedEvents()
	require.Len(t, events, 1)
	_, ok := events[0].(*common.TransactionStateTransitionEvent[State])
	require.True(t, ok)
}

func Test_HandleEvent_DelegatedAcceptsSubmittedFastForward(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()
	signer := builder.GetSignerAddress()
	nonce := builder.GetNonce()
	hash := builder.GetLatestSubmissionHash()

	err := txn.HandleEvent(ctx, &SubmittedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		SignerAddress:        signer,
		Nonce:                nonce,
		LatestSubmissionHash: hash,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Submitted, txn.GetCurrentState())
	require.NotNil(t, txn.GetNonce())
	assert.Equal(t, nonce, *txn.GetNonce())
	require.NotNil(t, txn.GetLatestSubmissionHash())
	assert.Equal(t, hash, *txn.GetLatestSubmissionHash())
}

func Test_HandleEvent_AssemblingAcceptsSubmittedFastForward(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()
	signer := builder.GetSignerAddress()
	nonce := builder.GetNonce()
	hash := builder.GetLatestSubmissionHash()

	err := txn.HandleEvent(ctx, &SubmittedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		SignerAddress:        signer,
		Nonce:                nonce,
		LatestSubmissionHash: hash,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Submitted, txn.GetCurrentState())
}

func Test_HandleEvent_EndorsementGatheringAcceptsConfirmedSuccessFastForward(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &ConfirmedSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Confirmed, txn.GetCurrentState())
}

func Test_HandleEvent_PreparedAcceptsConfirmedRevertedFastForward(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
	txn, _ := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RevertReason: pldtypes.HexBytes("0x1234"),
	})
	require.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
}
