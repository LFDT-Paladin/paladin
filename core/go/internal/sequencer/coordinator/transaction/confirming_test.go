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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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

func Test_action_NotifyOriginatorOfConfirmation_Success(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		UseMockTransportWriter().
		Build()

	nonce := pldtypes.HexUint64(42)
	event := &ConfirmedSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce: &nonce,
	}

	mocks.TransportWriter.EXPECT().
		SendTransactionConfirmed(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, &nonce, engine.TransactionConfirmed_OUTCOME_SUCCESS, pldtypes.HexBytes(nil), "", false).
		Return(nil)

	err := action_NotifyOriginatorOfConfirmation(ctx, txn, event)
	require.NoError(t, err)
}

func Test_action_NotifyOriginatorOfRetryableRevert(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		UseMockTransportWriter().
		Build()

	nonce := pldtypes.HexUint64(42)
	revertReason := pldtypes.MustParseHexBytes("0x1234")
	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce:        &nonce,
		RevertReason: revertReason,
	}
	txn.revertReason = revertReason

	mocks.TransportWriter.EXPECT().
		SendTransactionConfirmed(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, &nonce, engine.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "", true).
		Return(nil)

	err := action_NotifyOriginatorOfRetryableRevert(ctx, txn, event)
	require.NoError(t, err)
}

func Test_action_NotifyOriginatorOfNonRetryableRevert(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		UseMockTransportWriter().
		Build()

	nonce := pldtypes.HexUint64(42)
	revertReason := pldtypes.MustParseHexBytes("0x1234")
	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce:        &nonce,
		RevertReason: revertReason,
	}
	txn.revertReason = revertReason

	mocks.TransportWriter.EXPECT().
		SendTransactionConfirmed(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, &nonce, engine.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "", false).
		Return(nil)

	err := action_NotifyOriginatorOfNonRetryableRevert(ctx, txn, event)
	require.NoError(t, err)
}

func Test_action_RecordConfirmation_RevertSetsRevertReason(t *testing.T) {
	ctx := context.Background()
	hash := pldtypes.RandBytes32()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		LatestSubmissionHash(&hash).
		Build()
	revertReason := pldtypes.MustParseHexBytes("0x1234")
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(false, "", nil)
	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		RevertReason: revertReason,
	}

	err := action_RecordConfirmation(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, revertReason, txn.revertReason)
	assert.Equal(t, 1, txn.revertCount)
}

func Test_action_RecordConfirmation_RevertIncrementsRevertCount(t *testing.T) {
	ctx := context.Background()
	hash := pldtypes.RandBytes32()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		LatestSubmissionHash(&hash).
		RevertCount(2).
		Build()
	revertReason := pldtypes.MustParseHexBytes("0xabcd")
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(false, "", nil)
	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		RevertReason: revertReason,
	}

	err := action_RecordConfirmation(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, 3, txn.revertCount)
}

func Test_action_RecordConfirmation_SuccessNilHash(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Build()
	event := &ConfirmedSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
	}

	err := action_RecordConfirmation(ctx, txn, event)
	require.NoError(t, err)
}

func Test_action_RecordConfirmation_SuccessDifferentHash(t *testing.T) {
	ctx := context.Background()
	hash := pldtypes.RandBytes32()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		LatestSubmissionHash(&hash).
		Build()
	event := &ConfirmedSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Hash: pldtypes.RandBytes32(),
	}

	err := action_RecordConfirmation(ctx, txn, event)
	require.NoError(t, err)
}

func Test_action_NotifyDependantsOfSuccessfulConfirmation_NoDependents(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	err := action_NotifyDependantsOfSuccessfulConfirmation(ctx, txn, nil)
	require.NoError(t, err)
	assert.Len(t, txn.dependencies.PrereqOf, 0)
}

func Test_action_NotifyDependantsOfSuccessfulConfirmation_ResetLocksWhenRetentionNotConfigured(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		ConfirmedLockRetentionGracePeriod(0).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := action_NotifyDependantsOfSuccessfulConfirmation(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, txn.confirmedLocksReleased)
}

func Test_action_NotifyDependantsOfSuccessfulConfirmation_DoesNotResetLocksWhenRetentionConfigured(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		ConfirmedLockRetentionGracePeriod(2).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	err := action_NotifyDependantsOfSuccessfulConfirmation(ctx, txn, nil)
	require.NoError(t, err)
	assert.False(t, txn.confirmedLocksReleased)
}

func Test_action_NotifyDependantsOfRevertedConfirmation_AlwaysResetsLocks(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		ConfirmedLockRetentionGracePeriod(2).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return().Once()

	err := action_NotifyDependantsOfRevertedConfirmation(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, txn.confirmedLocksReleased)
}

func Test_ConfirmedSuccess_DispatchedStates_TransitionsToConfirmed(t *testing.T) {
	ctx := context.Background()
	dispatchedStates := []State{
		State_Dispatched,
	}

	for _, state := range dispatchedStates {
		t.Run(state.String(), func(t *testing.T) {
			txn, _ := NewTransactionBuilderForTesting(t, state).Build()
			nonce := pldtypes.HexUint64(77)
			event := &ConfirmedSuccessEvent{
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

func Test_ConfirmedRevert_StateDispatched_RetryableRevert_TransitionsToPooled(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xbeef")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "", nil)
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()
	nonce := pldtypes.HexUint64(88)
	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce:        &nonce,
		RevertReason: revertReason,
	}

	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.stateMachine.GetCurrentState())
}
func Test_ConfirmedRevert_StateDispatched_NonRetryable_TransitionsToReverted(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xdead")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(false, "decoded error", nil)
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()
	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()
	nonce := pldtypes.HexUint64(88)
	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce:        &nonce,
		RevertReason: revertReason,
	}

	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Reverted, txn.stateMachine.GetCurrentState())
}

func Test_ConfirmedRevert_StateDispatched_RetryableRevert_ExceedsThreshold_TransitionsToReverted(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xbeef")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(1).
		RevertCount(1).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "", nil)
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()
	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()
	nonce := pldtypes.HexUint64(88)
	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce:        &nonce,
		RevertReason: revertReason,
	}

	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, State_Reverted, txn.stateMachine.GetCurrentState())
}

func Test_action_RecordConfirmation_RevertRetryableAndUnderThreshold(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xbeef")
	hash := pldtypes.RandBytes32()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		LatestSubmissionHash(&hash).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "decoded", nil)

	err := action_RecordConfirmation(ctx, txn, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		Hash:                 hash,
		RevertReason:         revertReason,
	})
	require.NoError(t, err)
	assert.True(t, txn.lastCanRetryRevert)
	assert.Equal(t, "PD012216: Transaction reverted decoded", txn.decodedRevertReason)
	assert.Equal(t, 1, txn.revertCount)
}

func Test_action_RecordConfirmation_RevertRetryableAtThreshold(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xbeef")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		RevertCount(2).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "", nil)

	err := action_RecordConfirmation(ctx, txn, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RevertReason:         revertReason,
	})
	require.NoError(t, err)
	assert.True(t, txn.lastCanRetryRevert)
}

func Test_action_RecordConfirmation_RevertRetryableOverThreshold(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xbeef")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		RevertCount(3).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "", nil)

	err := action_RecordConfirmation(ctx, txn, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RevertReason:         revertReason,
	})
	require.NoError(t, err)
	assert.False(t, txn.lastCanRetryRevert)
}

func Test_action_RecordConfirmation_RevertNotRetryable(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xdead")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(false, "decoded error", nil)

	err := action_RecordConfirmation(ctx, txn, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RevertReason:         revertReason,
	})
	require.NoError(t, err)
	assert.False(t, txn.lastCanRetryRevert)
	assert.Equal(t, "PD012216: Transaction reverted decoded error", txn.decodedRevertReason)
}

func Test_action_RecordConfirmation_OffChainFailureMessageSkipsDomainRetryCheck(t *testing.T) {
	ctx := context.Background()
	failureMessage := "assembly failed upstream"
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		Build()

	err := action_RecordConfirmation(ctx, txn, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		FailureMessage:       failureMessage,
	})
	require.NoError(t, err)
	assert.False(t, txn.lastCanRetryRevert)
	assert.Equal(t, failureMessage, txn.decodedRevertReason)
	assert.Empty(t, txn.revertReason)
	assert.Nil(t, txn.revertOnChain)
}

func Test_action_RecordConfirmation_OnChainRevertWithFailureMessageStillUsesDomainRetryability(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xdead")
	failureMessage := "decoded by chained tx domain"
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(false, "decoded by coordinator domain", nil)

	err := action_RecordConfirmation(ctx, txn, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RevertReason:         revertReason,
		FailureMessage:       failureMessage,
	})
	require.NoError(t, err)
	assert.False(t, txn.lastCanRetryRevert)
	assert.Equal(t, "PD012216: Transaction reverted decoded by coordinator domain", txn.decodedRevertReason)
	assert.Equal(t, revertReason, txn.revertReason)
}

func Test_action_RecordConfirmation_OnChainRevertFallsBackToEventFailureMessageWhenDecodeEmpty(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xdead")
	failureMessage := "decoded by chained tx domain"
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(false, "", nil)

	err := action_RecordConfirmation(ctx, txn, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RevertReason:         revertReason,
		FailureMessage:       failureMessage,
	})
	require.NoError(t, err)
	assert.False(t, txn.lastCanRetryRevert)
	assert.Equal(t, failureMessage, txn.decodedRevertReason)
	assert.Equal(t, revertReason, txn.revertReason)
}

func Test_action_RecordConfirmation_RevertDomainAPIError_TreatedAsNonRetryable(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xdead")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(false, "", assert.AnError)

	err := action_RecordConfirmation(ctx, txn, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RevertReason:         revertReason,
	})
	require.NoError(t, err)
	assert.False(t, txn.lastCanRetryRevert)
}

func Test_action_RecordConfirmation_RevertThresholdZero(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.MustParseHexBytes("0xbeef")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(0).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "", nil)

	err := action_RecordConfirmation(ctx, txn, &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RevertReason:         revertReason,
	})
	require.NoError(t, err)
	assert.False(t, txn.lastCanRetryRevert)
}

func Test_action_RecordConfirmation_SuccessResetsCanRetry(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()
	txn.lastCanRetryRevert = true

	err := action_RecordConfirmation(ctx, txn, &ConfirmedSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.False(t, txn.lastCanRetryRevert)
}

func Test_guard_CanRetryRevert_ReadsStoredValue(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	txn.lastCanRetryRevert = true
	assert.True(t, guard_CanRetryRevert(ctx, txn))

	txn.lastCanRetryRevert = false
	assert.False(t, guard_CanRetryRevert(ctx, txn))
}

func Test_action_FinalizeNonRetryableRevert(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		RevertCount(2).
		RevertReason(pldtypes.MustParseHexBytes("0xdeadbeef")).
		Build()

	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything,
		mock.MatchedBy(func(req *syncpoints.TransactionFinalizeRequest) bool {
			return req.Domain == txn.pt.Domain &&
				req.Originator == txn.originator &&
				req.TransactionID == txn.pt.ID &&
				req.FailureMessage == "" &&
				req.RevertData.String() == txn.revertReason.String()
		}),
		mock.Anything, mock.Anything,
	).Return()

	err := action_FinalizeNonRetryableRevert(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NotifyDependantsOfRevertedConfirmation_SendsRevertedEvent(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)
	tx2ID := uuid.New()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{tx2ID}}).
		Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	_, _ = NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(tx2ID).
		Grapher(grapher).
		Build()

	err := action_NotifyDependantsOfRevertedConfirmation(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NotifyDependantsOfSuccessfulConfirmation_SendsReadyEvent(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)
	tx2ID := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{tx2ID}}).
		Build()

	_, _ = NewTransactionBuilderForTesting(t, State_Blocked).
		TransactionID(tx2ID).
		Grapher(grapher).
		Build()

	err := action_NotifyDependantsOfSuccessfulConfirmation(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_notifyDependentsOfRevertedConfirmation_NoDependents(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()

	err := txn.notifyDependentsOfRevertedConfirmation(ctx)
	require.NoError(t, err)
}

func Test_notifyDependentsOfRevertedConfirmation_DependentNotInMemory(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{uuid.New()}}).
		Build()

	err := txn.notifyDependentsOfRevertedConfirmation(ctx)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "PD012645"))
}

func Test_DependencyReset_Dispatched_StaysDispatched(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &DependencyResetEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Dispatched, txn.stateMachine.GetCurrentState())
}

func Test_DependencyConfirmedReverted_Dispatched_StaysDispatched(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Dispatched, txn.stateMachine.GetCurrentState())
}

func Test_DependencyReset_PreDispatchStates_TransitionsToPooled(t *testing.T) {
	ctx := context.Background()
	preDispatchStates := []State{
		State_Endorsement_Gathering,
		State_Blocked,
		State_Confirming_Dispatchable,
		State_Ready_For_Dispatch,
	}

	for _, state := range preDispatchStates {
		t.Run(state.String(), func(t *testing.T) {
			txn, mocks := NewTransactionBuilderForTesting(t, state).Build()
			mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

			err := txn.HandleEvent(ctx, &DependencyResetEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
			})
			require.NoError(t, err)
			assert.Equal(t, State_Pooled, txn.stateMachine.GetCurrentState())
		})
	}
}

func Test_DependencyConfirmedReverted_PreDispatchStates_TransitionsToPooled(t *testing.T) {
	ctx := context.Background()
	preDispatchStates := []State{
		State_Endorsement_Gathering,
		State_Blocked,
		State_Confirming_Dispatchable,
		State_Ready_For_Dispatch,
	}

	for _, state := range preDispatchStates {
		t.Run(state.String(), func(t *testing.T) {
			txn, mocks := NewTransactionBuilderForTesting(t, state).Build()
			mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

			err := txn.HandleEvent(ctx, &DependencyConfirmedRevertedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
			})
			require.NoError(t, err)
			assert.Equal(t, State_Pooled, txn.stateMachine.GetCurrentState())
		})
	}
}
