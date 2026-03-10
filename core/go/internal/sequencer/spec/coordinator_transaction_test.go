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

package spec

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorTransaction_Initial_ToPooled_OnReceived_IfNoInflightDependencies(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()

	err := txn.HandleEvent(ctx, &transaction.DelegatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Initial_ToPreAssemblyBlocked_OnReceived_IfDependencyNotAssembled(t *testing.T) {

	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	//transaction2 depends on transaction 1 and transaction 1 gets reverted
	txn1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).
		Grapher(grapher).
		Build()

	txn2, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Initial).
		Grapher(grapher).
		PredefinedDependencies(txn1.GetID()).
		Build()

	err := txn2.HandleEvent(ctx, &transaction.DelegatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn2.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_PreAssembly_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())
}

func TestCoordinatorTransaction_Initial_ToPreAssemblyBlocked_OnReceived_IfDependencyUnknown(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Initial).
		PredefinedDependencies(uuid.New()).
		Build()

	err := txn.HandleEvent(ctx, &transaction.DelegatedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_PreAssembly_Blocked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Pooled_ToAssembling_OnSelected(t *testing.T) {
	ctx := context.Background()

	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()
	mocks.EngineIntegration.EXPECT().GetStateLocks(mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.EXPECT().GetBlockHeight(mock.Anything).Return(int64(100), nil)

	err := txn.HandleEvent(ctx, &transaction.SelectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)

	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, true, mocks.SentMessageRecorder.HasSentAssembleRequest())
}

func TestCoordinatorTransaction_Assembling_ToEndorsing_OnAssembleResponse(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		NumberOfOutputStates(1).
		AddPendingAssembleRequest()
	txn, mocks := txnBuilder.Build()

	successEvent := txnBuilder.BuildAssembleSuccessEvent()
	outputState := successEvent.PostAssembly.OutputStates[0]
	mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Run(func(ctx context.Context, txn *components.PrivateTransaction) {
		assert.Equal(t, outputState.ID, txn.PostAssembly.OutputStates[0].ID)
	}).Return(nil)

	err := txn.HandleEvent(ctx, successEvent)
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 3, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())
}

func TestCoordinatorTransaction_Assembling_NoTransition_OnAssembleResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn, _ := txnBuilder.Build()

	err := txn.HandleEvent(ctx, &transaction.AssembleSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    uuid.New(), //generate a new random request ID so that it won't match the pending request
	})
	assert.NoError(t, err)
	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_NoTransition_OnRequestTimeout(t *testing.T) {
	ctx := context.Background()
	hasNudged := false
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		RequestTimeout(1).
		AddPendingAssembleRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error {
			hasNudged = true
			return nil
		}).
		Build()

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.True(t, hasNudged)
	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToPooled_OnStateTimeout_IfStateTimeoutExpired(t *testing.T) {
	ctx := context.Background()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		StateTimeout(1).
		Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.GetID()).Return()

	err := txn.HandleEvent(ctx, &transaction.StateTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToReverted_OnAssembleRevertResponse(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		AddPendingAssembleRequest().
		Reverts("some revert reason")

	txn, mocks := txnBuilder.Build()

	mocks.SyncPoints.On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, txnBuilder.BuildAssembleRevertEvent())
	require.NoError(t, err)

	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_NoTransition_OnAssembleRevertResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		AddPendingAssembleRequest().
		Reverts("some revert reason")

	txn, _ := txnBuilder.Build()

	err := txn.HandleEvent(ctx, &transaction.AssembleRevertResponseEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    uuid.New(), //generate a new random request ID so that it won't match the pending request,
	})
	require.NoError(t, err)

	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests(t *testing.T) {
	ctx := context.Background()
	var requestCount int
	incrementCount := func(ctx context.Context, idempotencyKey uuid.UUID) error {
		requestCount++
		return nil
	}
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		AddPendingEndorsementRequestWithCallback(0, incrementCount).
		AddPendingEndorsementRequestWithCallback(1, incrementCount).
		AddPendingEndorsementRequestWithCallback(2, incrementCount).
		Build()

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, requestCount)
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests_Partial(t *testing.T) {
	//emulate the case where only a subset of the endorsement requests have timed out
	ctx := context.Background()
	var requestCount int
	incrementCount := func(ctx context.Context, idempotencyKey uuid.UUID) error {
		requestCount++
		return nil
	}
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(4).
		AddPendingEndorsementRequestWithCallback(0, incrementCount).
		AddPendingEndorsementRequestWithCallback(1, incrementCount).
		AddPendingEndorsementRequestWithCallback(2, incrementCount).
		AddPendingEndorsementRequestWithCallback(3, incrementCount)

	txn, _ := builder.Build()

	//2 endorsements come back in a timely manner
	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(0))
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, builder.BuildEndorsedEvent(1))
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, requestCount)
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Endorsement_Gathering_ToConfirmingDispatch_OnEndorsed_IfAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		AddPendingEndorsementRequest(2)

	txn, mocks := builder.Build()
	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(2))
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.True(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "expected a dispatch confirmation request to be sent, but none were sent")

}

func TestCoordinatorTransaction_Endorsement_GatheringNoTransition_IfNotAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(1). //only 1 existing endorsement so the next one does not complete the attestation plan
		AddPendingEndorsementRequest(1).
		AddPendingEndorsementRequest(2)

	txn, mocks := builder.Build()

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(1))
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.False(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "did not expected a dispatch confirmation request to be sent, but one was sent")

}

func TestCoordinatorTransaction_Endorsement_Gathering_ToBlocked_OnEndorsed_IfAttestationPlanCompleteAndHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	txn1, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	builder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		AddPendingEndorsementRequest(2).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.GetID()},
		})
	txn2, _ := builder2.Build()

	err := txn2.HandleEvent(ctx, builder2.BuildEndorsedEvent(2))
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_ToPooled_OnEndorseRejected(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		AddPendingEndorsementRequest(2)

	txn, mocks := builder.Build()
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.GetID()).Return()

	err := txn.HandleEvent(ctx, builder.BuildEndorseRejectedEvent(2))
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_NudgeRequest_OnRequestTimeout(t *testing.T) {
	ctx := context.Background()
	var nudged bool
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		AddPendingPreDispatchRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error {
			nudged = true
			return nil
		}).
		Build()

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.True(t, nudged)
	assert.Equal(t, transaction.State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_ToReadyForDispatch_OnDispatchConfirmed(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		AddPendingPreDispatchRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error {
			return nil
		})
	txn, _ := builder.Build()

	err := txn.HandleEvent(ctx, builder.BuildDispatchRequestApprovedEvent())
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Ready_For_Dispatch, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_NoTransition_OnDispatchConfirmed_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build()

	err := txn.HandleEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RequestID: uuid.New(),
	})
	require.NoError(t, err)

	assert.Equal(t, transaction.State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Blocked_ToConfirmingDispatch_OnDependencyReady_IfNotHasDependenciesNotReady(t *testing.T) {
	//TODO rethink naming of this test and/or the guard function because we end up with a double negative
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  One of which (B) is ready for dispatch and the other (C) becomes ready for dispatch,
	// triggering a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	txAID := uuid.New()
	txBID := uuid.New()
	txCID := uuid.New()

	_, _ = transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).
		Grapher(grapher).
		TransactionID(txBID).
		Build()

	builderC := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		Grapher(grapher).
		TransactionID(txCID).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{txAID},
		}).
		AddPendingPreDispatchRequest()
	txnC, _ := builderC.Build()

	builderA := transaction.NewTransactionBuilderForTesting(t, transaction.State_Blocked).
		Grapher(grapher).
		TransactionID(txAID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txBID, txCID},
		})
	txnA, _ := builderA.Build()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing C to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnC.HandleEvent(ctx, builderC.BuildDispatchRequestApprovedEvent())
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirming_Dispatchable, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())
}

func TestCoordinatorTransaction_BlockedNoTransition_OnDependencyReady_IfHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  Neither of which a ready for dispatch. One of them (B) becomes ready for dispatch, but the other is still not ready
	// thus gating the triggering of a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)
	txAID := uuid.New()
	txBID := uuid.New()
	txCID := uuid.New()

	builderB := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		Grapher(grapher).
		TransactionID(txBID).
		AddPendingPreDispatchRequest().
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{txAID},
		})
	txnB, _ := builderB.Build()

	_, _ = transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		Grapher(grapher).
		TransactionID(txCID).
		AddPendingPreDispatchRequest().
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{txAID},
		}).Build()

	txnA, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Blocked).
		Grapher(grapher).
		TransactionID(txAID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txBID, txCID},
		}).Build()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing B to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnB.HandleEvent(ctx, builderB.BuildDispatchRequestApprovedEvent())
	require.NoError(t, err)

	assert.Equal(t, transaction.State_Blocked, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())

}

func TestCoordinatorTransaction_ReadyForDispatch_ToDispatched_OnDispatched(t *testing.T) {
	ctx := context.Background()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_PREPARE_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		PostAssembly(&components.TransactionPostAssembly{}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)
		tx.PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, &transaction.DispatchedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_NoTransition_OnCollected(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_NoTransition_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToPooled_OnConfirmedRevert_IfRetryable(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.HexBytes("0x01020304")
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).
		BaseLedgerRevertRetryThreshold(3).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "", nil)
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.GetID()).Return()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedRevertedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RevertReason: revertReason,
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToConfirmed_OnConfirmedRevert_IfNonRetryable(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.HexBytes("0x01020304")
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(false, "decoded error", nil)
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.GetID()).Return()
	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedRevertedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RevertReason: revertReason,
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToConfirmed_OnConfirmedRevert_IfThresholdExceeded(t *testing.T) {
	ctx := context.Background()
	revertReason := pldtypes.HexBytes("0x01020304")
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).
		BaseLedgerRevertRetryThreshold(1).
		RevertCount(1).
		Dependencies(&pldapi.TransactionDependencies{PrereqOf: []uuid.UUID{}}).
		Build()
	mocks.DomainAPI.EXPECT().IsBaseLedgerRevertRetryable(mock.Anything, []byte(revertReason)).Return(true, "", nil)
	mocks.EngineIntegration.EXPECT().ResetTransactions(mock.Anything, txn.GetID()).Return()
	mocks.SyncPoints.EXPECT().QueueTransactionFinalize(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedRevertedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RevertReason: revertReason,
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToConfirmed_OnConfirmedSuccess(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Confirmed_ToFinal_OnHeartbeatInterval_IfHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).
		ConfirmedLocksReleased(true).
		HeartbeatIntervalsSinceStateChange(4).
		Build()

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Confirmed_NoTransition_OnHeartbeatInterval_IfNotHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).
		ConfirmedLocksReleased(true).
		HeartbeatIntervalsSinceStateChange(3).
		Build()

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToFinal_OnTransactionUnknownByOriginator(t *testing.T) {
	// Test that when an originator reports a transaction as unknown (most likely because
	// it reverted during assembly but the response was lost and the transaction has since
	// been cleaned up on the originator), the coordinator transitions to State_Final
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).Build()

	err := txn.HandleEvent(ctx, &transaction.TransactionUnknownByOriginatorEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}
