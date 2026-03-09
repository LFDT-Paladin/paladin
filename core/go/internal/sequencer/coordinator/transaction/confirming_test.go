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
	"context"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_guard_HasRevertReason_FalseWhenEmpty(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	// Initially revertReason should be nil (zero value for HexBytes)
	// When nil, String() returns "", so guard returns false
	assert.False(t, guard_HasRevertReason(ctx, txn))

	// Note: An empty slice HexBytes{} would return "0x" from String(),
	// which is not empty, so the guard would return true. Only nil returns false.
}

func Test_guard_HasRevertReason_TrueWhenSet(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	// Set revertReason to a non-empty value
	txn.revertReason = pldtypes.MustParseHexBytes("0x1234567890abcdef")
	assert.True(t, guard_HasRevertReason(ctx, txn))

	// Test with another value
	txn.revertReason = pldtypes.MustParseHexBytes("0xdeadbeef")
	assert.True(t, guard_HasRevertReason(ctx, txn))
}

func Test_notifyDependentsOfConfirmation_NoDependents(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	err := txn.notifyDependentsOfConfirmation(ctx)
	require.NoError(t, err)
}

func Test_notifyDependentsOfConfirmation_DependentNotInMemory(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{uuid.New()}}).
		Build()

	err := txn.notifyDependentsOfConfirmation(ctx)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "PD012645"))
}

func Test_notifyDependentsOfConfirmation_WithTraceEnabled(t *testing.T) {
	ctx := context.Background()

	// Enable trace logging to cover the traceDispatch path
	log.EnsureInit()
	originalLevel := log.GetLevel()
	log.SetLevel("trace")
	defer log.SetLevel(originalLevel)

	txn1, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		PostAssembly(&components.TransactionPostAssembly{
			Signatures: []*prototk.AttestationResult{
				{
					Verifier: &prototk.ResolvedVerifier{
						Lookup: "verifier1",
					},
				},
			},
			Endorsements: []*prototk.AttestationResult{
				{
					Verifier: &prototk.ResolvedVerifier{
						Lookup: "verifier2",
					},
				},
			},
		}).
		Build()

	err := txn1.notifyDependentsOfConfirmation(ctx)
	require.NoError(t, err)
}
func Test_notifyDependentsOfConfirmation_DependentInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	tx2ID := uuid.New()
	txn1, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{tx2ID}}).
		Build()
	_, _ = NewTransactionBuilderForTesting(t, State_Initial).
		TransactionID(tx2ID).
		Grapher(grapher).
		TransactionID(tx2ID).
		Build()

	err := txn1.notifyDependentsOfConfirmation(ctx)
	require.NoError(t, err)
}

// TODO: this test can be implemented when there is a way to mock the dependent transaction
// func Test_notifyDependentsOfConfirmation_DependentHandleEventError(t *testing.T) {}

func Test_action_NotifyConfirmed_SendsToOrignator(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		UseMockTransportWriter().
		Build()

	nonce := pldtypes.HexUint64(42)
	revertReason := pldtypes.MustParseHexBytes("0x1234")
	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce:        &nonce,
		RevertReason: revertReason,
	}

	mocks.TransportWriter.EXPECT().
		SendTransactionConfirmed(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, &nonce, revertReason).
		Return(nil)

	err := action_NotifyConfirmed(ctx, txn, event)
	require.NoError(t, err)
}

func Test_action_RecordConfirmationDetails_SetsRevertReason(t *testing.T) {
	ctx := context.Background()
	hash := pldtypes.RandBytes32()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		LatestSubmissionHash(&hash).
		Build()
	revertReason := pldtypes.MustParseHexBytes("0x1234")
	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		RevertReason: revertReason,
	}

	err := action_RecordConfirmationDetails(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, revertReason, txn.revertReason)
}

func Test_action_RecordConfirmationDetails_NilHash(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Build()
	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
	}

	err := action_RecordConfirmationDetails(ctx, txn, event)
	require.NoError(t, err)
}

func Test_action_RecordConfirmationDetails_DifferentHash(t *testing.T) {
	ctx := context.Background()
	hash := pldtypes.RandBytes32()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		LatestSubmissionHash(&hash).
		Build()
	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Hash: pldtypes.RandBytes32(),
	}

	err := action_RecordConfirmationDetails(ctx, txn, event)
	require.NoError(t, err)
}

func Test_action_NotifyDependantsOfConfirmation_Success(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	err := action_NotifyDependantsOfConfirmation(ctx, txn, nil)
	require.NoError(t, err)
	assert.Len(t, txn.dependencies.PrereqOf, 0)
}

func Test_action_NotifyDependantsOfConfirmation_ResetLocksOnTransitionWhenRetentionNotConfigured(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		ConfirmedLockRetentionGracePeriod(0).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := action_NotifyDependantsOfConfirmation(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, txn.confirmedLocksReleased)
}

func Test_action_NotifyDependantsOfConfirmation_DoesNotResetLocksOnTransitionWhenRetentionConfigured(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		ConfirmedLockRetentionGracePeriod(2).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	err := action_NotifyDependantsOfConfirmation(ctx, txn, nil)
	require.NoError(t, err)
	assert.False(t, txn.confirmedLocksReleased)
}

func Test_action_NotifyDependantsOfConfirmation_ResetsLocksImmediatelyWhenRetentionDisabled(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		ConfirmedLockRetentionGracePeriod(0).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return().Once()

	err := action_NotifyDependantsOfConfirmation(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, txn.confirmedLocksReleased)
}

func Test_EventConfirmed_NonTerminalStates_TransitionsToConfirmed_WhenNoRevertReason(t *testing.T) {
	ctx := context.Background()
	nonTerminalStates := []State{
		State_Initial,
		State_PreAssembly_Blocked,
		State_Pooled,
		State_Assembling,
		State_Endorsement_Gathering,
		State_Blocked,
		State_Confirming_Dispatchable,
		State_Ready_For_Dispatch,
		State_Dispatched,
	}

	for _, state := range nonTerminalStates {
		t.Run(state.String(), func(t *testing.T) {
			txn, _ := NewTransactionBuilderForTesting(t, state).Build()
			nonce := pldtypes.HexUint64(77)
			event := &ConfirmedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: txn.pt.ID,
				},
				Nonce: &nonce,
			}

			err := txn.HandleEvent(ctx, event)
			require.NoError(t, err)
			assert.Equal(t, State_Confirmed, txn.stateMachine.GetCurrentState())
		})
	}
}

func Test_EventConfirmed_NonTerminalStates_RevertStaysInPlace_ForNewHandlers(t *testing.T) {
	ctx := context.Background()
	nonTerminalStates := []State{
		State_Initial,
		State_PreAssembly_Blocked,
		State_Pooled,
		State_Assembling,
		State_Endorsement_Gathering,
		State_Blocked,
		State_Confirming_Dispatchable,
		State_Ready_For_Dispatch,
	}

	for _, state := range nonTerminalStates {
		t.Run(state.String(), func(t *testing.T) {
			txn, _ := NewTransactionBuilderForTesting(t, state).Build()
			nonce := pldtypes.HexUint64(88)
			event := &ConfirmedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: txn.pt.ID,
				},
				Nonce:        &nonce,
				RevertReason: pldtypes.MustParseHexBytes("0xbeef"),
			}

			err := txn.HandleEvent(ctx, event)
			require.NoError(t, err)
			assert.Equal(t, state, txn.stateMachine.GetCurrentState())
		})
	}
}

func Test_EventConfirmed_StateDispatched_RevertTransitionsToPooled(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()
	nonce := pldtypes.HexUint64(88)
	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce:        &nonce,
		RevertReason: pldtypes.MustParseHexBytes("0xbeef"),
	}

	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.stateMachine.GetCurrentState())
}
