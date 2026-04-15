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
	"errors"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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

func Test_action_NotifyDependantsOfRevertedConfirmation_AlwaysResetsLocks(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Initial).
		ConfirmedLockRetentionGracePeriod(2).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{},
			},
		}).
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
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{},
			},
		}).
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
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{},
			},
		}).
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

func Test_action_FinalizeNonRetryableRevert_OnCommitCallback(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		RevertCount(2).
		RevertReason(pldtypes.MustParseHexBytes("0xdeadbeef")).
		Build()

	onCommitCalled := false
	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything,
		mock.MatchedBy(func(req *syncpoints.TransactionFinalizeRequest) bool {
			return req.Domain == txn.pt.Domain &&
				req.Originator == txn.originator &&
				req.TransactionID == txn.pt.ID
		}),
		mock.Anything, mock.Anything,
	).Run(func(_ context.Context, _ *syncpoints.TransactionFinalizeRequest, onCommit func(context.Context), _ func(context.Context, error)) {
		onCommit(ctx)
		onCommitCalled = true
	}).Return()

	err := action_FinalizeNonRetryableRevert(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, onCommitCalled, "onCommit callback should have been invoked")
}

func Test_action_FinalizeNonRetryableRevert_OnRollbackCallback(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		RevertCount(2).
		RevertReason(pldtypes.MustParseHexBytes("0xdeadbeef")).
		Build()

	rollbackErr := errors.New("finalize failed")
	onRollbackCalled := false
	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything,
		mock.MatchedBy(func(req *syncpoints.TransactionFinalizeRequest) bool {
			return req.Domain == txn.pt.Domain &&
				req.Originator == txn.originator &&
				req.TransactionID == txn.pt.ID
		}),
		mock.Anything, mock.Anything,
	).Run(func(_ context.Context, _ *syncpoints.TransactionFinalizeRequest, _ func(context.Context), onRollback func(context.Context, error)) {
		onRollback(ctx, rollbackErr)
		onRollbackCalled = true
	}).Return()

	err := action_FinalizeNonRetryableRevert(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, onRollbackCalled, "onRollback callback should have been invoked")
}

func Test_action_NotifyDependantsOfRevertedConfirmation_SendsRevertedEvent(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)
	tx2ID := uuid.New()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(grapher).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{tx2ID},
			},
		}).
		Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.pt.ID).Return()

	_, _ = NewTransactionBuilderForTesting(t, State_Pooled).
		TransactionID(tx2ID).
		Grapher(grapher).
		Build()

	err := action_NotifyDependantsOfRevertedConfirmation(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_notifyDependentsOfRevertedConfirmation_NoDependents(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Build()

	err := txn.notifyDependentsOfRevertedConfirmation(ctx)
	require.NoError(t, err)
}

func Test_notifyDependentsOfRevertedConfirmation_DependentNotInMemory(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{uuid.New()},
			},
		}).
		Build()

	err := txn.notifyDependentsOfRevertedConfirmation(ctx)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "PD012645"))
}

func Test_notifyDependentsOfRevertedConfirmation_HandleEventReturnsError(t *testing.T) {
	ctx := context.Background()
	mockGrapher := NewMockGrapher(t)
	dependentID := uuid.New()
	privateTxnID := uuid.New()

	// Set up mock expectations for grapher operations used during transaction creation
	mockGrapher.EXPECT().Add(mock.Anything, mock.Anything).Return().Maybe()
	mockGrapher.EXPECT().ForgetMints(mock.Anything).Return().Maybe()

	// Create a mock dependent transaction that returns an error from HandleEvent
	mockDependentTxn := NewMockCoordinatorTransaction(t)
	expectedError := errors.New("handle event error")
	mockDependentTxn.EXPECT().GetPrivateTransaction().Return(&components.PrivateTransaction{ID: privateTxnID})
	mockDependentTxn.EXPECT().HandleEvent(ctx, mock.AnythingOfType("*transaction.DependencyConfirmedRevertedEvent")).Return(expectedError)

	// Configure mock grapher to return the mock dependent transaction
	mockGrapher.EXPECT().TransactionByID(ctx, dependentID).Return(mockDependentTxn)

	// Create main transaction with the mock grapher and a dependent
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(mockGrapher).
		Dependencies(&TransactionDependencies{
			PostAssemble: PostAssembleDependencies{
				PrereqOf: []uuid.UUID{dependentID},
			},
		}).
		Build()

	// Call notifyDependentsOfRevertedConfirmation - should return the error from HandleEvent
	err := txn.notifyDependentsOfRevertedConfirmation(ctx)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
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

func TestDependsOn_CascadeFailure_SendsEventToDependentWhichFinalizesItself(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	revertedTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).
		Build()

	dependentTx, depMocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	revertedTx.dependencies.Chained.PrereqOf = []uuid.UUID{dependentTx.pt.ID}

	depMocks.SyncPoints.On("QueueTransactionFinalize",
		mock.Anything,
		mock.MatchedBy(func(req *syncpoints.TransactionFinalizeRequest) bool {
			return req.TransactionID == dependentTx.pt.ID &&
				req.FailureMessage != ""
		}),
		mock.Anything,
		mock.Anything,
	).Return()
	depMocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, dependentTx.pt.ID).Return().Once()

	err := action_CascadeChainedDependencyFailure(ctx, revertedTx, nil)
	require.NoError(t, err)

	assert.Equal(t, State_Reverted, dependentTx.stateMachine.CurrentState)
}

func TestDependsOn_FinalizeOnChainedDependencyFailure(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).Build()
	dependencyID := uuid.New()
	failureMsg := i18n.NewError(ctx, msgs.MsgTxMgrDependencyFailed, dependencyID).Error()

	mocks.SyncPoints.On("QueueTransactionFinalize",
		mock.Anything,
		mock.MatchedBy(func(req *syncpoints.TransactionFinalizeRequest) bool {
			return req.TransactionID == txn.pt.ID &&
				req.FailureMessage == failureMsg
		}),
		mock.Anything,
		mock.Anything,
	).Return()

	event := &ChainedDependencyFailedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		FailedTxID:           dependencyID,
	}
	err := action_FinalizeOnChainedDependencyFailure(ctx, txn, event)
	require.NoError(t, err)
}

func TestDependsOn_CascadeFailure_ErrorsOnMissingDependent(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	revertedTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).
		Build()

	unknownID := uuid.New()
	revertedTx.dependencies.Chained.PrereqOf = []uuid.UUID{unknownID}

	err := action_CascadeChainedDependencyFailure(ctx, revertedTx, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012645")
}

func TestDependsOn_CascadeEviction_SendsEventToDependentWhichEvictsItself(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	evictedTx, _ := NewTransactionBuilderForTesting(t, State_Evicted).
		Grapher(grapher).
		Build()

	dependentTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	evictedTx.dependencies.Chained.PrereqOf = []uuid.UUID{dependentTx.pt.ID}

	err := action_CascadeChainedDependencyEviction(ctx, evictedTx, nil)
	require.NoError(t, err)

	assert.Equal(t, State_Evicted, dependentTx.stateMachine.CurrentState)
}

func TestDependsOn_CascadeEviction_ErrorsOnMissingDependent(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	evictedTx, _ := NewTransactionBuilderForTesting(t, State_Evicted).
		Grapher(grapher).
		Build()

	unknownID := uuid.New()
	evictedTx.dependencies.Chained.PrereqOf = []uuid.UUID{unknownID}

	err := action_CascadeChainedDependencyEviction(ctx, evictedTx, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012645")
}

func TestDependsOn_ParentRecognition_ChainedDependencyRevert(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		FailureMessage: "PD012256: Transaction dependency abc12345 failed",
	}

	err := action_RecordConfirmation(context.Background(), txn, event)
	require.NoError(t, err)

	assert.Equal(t, 0, txn.revertCount)
	assert.True(t, txn.lastCanRetryRevert)
	assert.Equal(t, "PD012256: Transaction dependency abc12345 failed", txn.decodedRevertReason)
}

func TestDependsOn_ParentRecognition_RegularOffChainRevert(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		FailureMessage: "Some other error",
	}

	err := action_RecordConfirmation(context.Background(), txn, event)
	require.NoError(t, err)

	assert.Equal(t, 1, txn.revertCount)
	assert.False(t, txn.lastCanRetryRevert)
}

func TestDependsOn_ParentRecognition_OnChainRevertNotAffected(t *testing.T) {
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		Build()

	mocks.DomainAPI.On("IsBaseLedgerRevertRetryable", mock.Anything, mock.Anything).
		Return(true, "decoded reason", nil)

	event := &ConfirmedRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		RevertReason: pldtypes.HexBytes{0x01, 0x02},
		OnChain:      pldtypes.OnChainLocation{Type: pldtypes.OnChainTransaction},
		Hash:         pldtypes.Bytes32(pldtypes.RandBytes(32)),
	}

	err := action_RecordConfirmation(context.Background(), txn, event)
	require.NoError(t, err)

	assert.Equal(t, 1, txn.revertCount)
	assert.True(t, txn.lastCanRetryRevert)
}

func Test_action_NotifyPreAssembleDependentOfTermination_NilPrereqOf(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Reverted).Build()
	txn.dependencies.PreAssemble.PrereqOf = nil

	err := action_NotifyPreAssembleDependentOfTermination(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NotifyPreAssembleDependentOfTermination_DependentNotInGrapher(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	txn, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).
		Build()

	dependentID := uuid.New()
	txn.dependencies.PreAssemble.PrereqOf = &dependentID

	err := action_NotifyPreAssembleDependentOfTermination(ctx, txn, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012645")
}

func Test_action_NotifyPreAssembleDependentOfTermination_SendsEventToDependent(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	prereqTx, _ := NewTransactionBuilderForTesting(t, State_Reverted).
		Grapher(grapher).
		Build()

	dependentTx, mocks := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).
		Build()
	dependentTx.dependencies.PreAssemble.DependsOn = &prereqTx.pt.ID

	prereqTx.dependencies.PreAssemble.PrereqOf = &dependentTx.pt.ID

	// The dependent has no other unassembled deps, so it should transition to State_Pooled
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, dependentTx.pt.ID).Return()

	err := action_NotifyPreAssembleDependentOfTermination(ctx, prereqTx, nil)
	require.NoError(t, err)

	assert.Nil(t, dependentTx.dependencies.PreAssemble.DependsOn)
	assert.Equal(t, State_Pooled, dependentTx.GetCurrentState())
}

func Test_action_NotifyPreAssembleDependentOfTermination_StaysBlockedWithChainedDeps(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	prereqTx, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(grapher).
		Build()

	chainedDepTx, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Build()

	dependentTx, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Grapher(grapher).
		Build()
	dependentTx.dependencies.PreAssemble.DependsOn = &prereqTx.pt.ID
	dependentTx.dependencies.Chained.DependsOn = []uuid.UUID{chainedDepTx.pt.ID}
	dependentTx.dependencies.Chained.Unassembled = map[uuid.UUID]struct{}{chainedDepTx.pt.ID: {}}

	prereqTx.dependencies.PreAssemble.PrereqOf = &dependentTx.pt.ID

	err := action_NotifyPreAssembleDependentOfTermination(ctx, prereqTx, nil)
	require.NoError(t, err)

	assert.Nil(t, dependentTx.dependencies.PreAssemble.DependsOn)
	assert.Equal(t, State_PreAssembly_Blocked, dependentTx.GetCurrentState())
}
