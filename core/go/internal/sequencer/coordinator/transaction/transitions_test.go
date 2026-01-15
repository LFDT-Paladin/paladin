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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTransaction_Initial_ToPooled_OnReceived_IfNoInflightDependencies(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Pooled).Build()

	err := txn.ProcessEvent(ctx, &ReceivedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Initial_ToPreAssemblyBlocked_OnReceived_IfDependencyNotAssembled(t *testing.T) {

	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := NewGrapher(ctx)

	//transaction2 depends on transaction 1 and transaction 1 gets reverted
	builder1 := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher)

	txn1 := builder1.Build()

	builder2 := NewTransactionBuilderForTesting(t, State_Initial).
		Grapher(grapher).
		Originator(builder1.GetOriginator()).
		PredefinedDependencies(txn1.ID)
	txn2 := builder2.Build()

	err := txn2.ProcessEvent(ctx, &ReceivedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn2.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_PreAssembly_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestTransaction_Initial_ToPreAssemblyBlocked_OnReceived_IfDependencyUnknown(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial).
		PredefinedDependencies(uuid.New())
	txn := builder.Build()

	err := txn.ProcessEvent(ctx, &ReceivedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_PreAssembly_Blocked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestTransaction_Pooled_ToAssembling_OnSelected(t *testing.T) {
	ctx := context.Background()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Pooled).BuildWithMocks()

	err := txn.ProcessEvent(ctx, &SelectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, true, mocks.SentMessageRecorder.HasSentAssembleRequest())
}

func TestTransaction_Assembling_ToEndorsing_OnAssembleResponse(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := txnBuilder.BuildWithMocks()

	err := txn.ProcessEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		PreAssembly:  txnBuilder.BuildPreAssembly(),
		RequestID:    mocks.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 3, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())
	//TODO some assertions that WriteLockAndDistributeStatesForTransaction was called with the expected states

}

func TestTransaction_Assembling_NoTransition_OnAssembleResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn := txnBuilder.Build()

	err := txn.ProcessEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    uuid.New(), //generate a new random request ID so that it won't match the pending request
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Assembling_NoTransition_OnRequestTimeout_IfNotAssembleTimeoutExpired(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := txnBuilder.BuildWithMocks()

	mocks.Clock.Advance(txnBuilder.GetAssembleTimeout() - 1)

	assert.Equal(t, 1, mocks.SentMessageRecorder.NumberOfSentAssembleRequests())
	err := txn.ProcessEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, mocks.SentMessageRecorder.NumberOfSentAssembleRequests())

	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Assembling_ToPooled_OnRequestTimeout_IfAssembleTimeoutExpired(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := txnBuilder.BuildWithMocks()

	mocks.Clock.Advance(txnBuilder.GetAssembleTimeout() + 1)

	err := txn.ProcessEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 1, txn.GetErrorCount(), "expected error count to be 1, but it was %d", txn.GetErrorCount())
}

func TestTransaction_Assembling_ToReverted_OnAssembleRevertResponse(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Reverts("some revert reason")

	txn, mocks := txnBuilder.BuildWithMocks()

	mocks.SyncPoints.(*syncpoints.MockSyncPoints).On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.ProcessEvent(ctx, &AssembleRevertResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    mocks.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Assembling_NoTransition_OnAssembleRevertResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling).
		Reverts("some revert reason")

	txn := txnBuilder.Build()

	err := txn.ProcessEvent(ctx, &AssembleRevertResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    uuid.New(), //generate a new random request ID so that it won't match the pending request,
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Pooled_ToPreAssemblyBlocked_OnDependencyReverted(t *testing.T) {
	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := NewGrapher(ctx)

	//transaction2 depends on transaction 1 and transaction 1 gets reverted
	builder1 := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Reverts("some revert reason")
	txn1, mocks1 := builder1.BuildWithMocks()

	mocks1.SyncPoints.(*syncpoints.MockSyncPoints).On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	builder2 := NewTransactionBuilderForTesting(t, State_Pooled).
		Grapher(grapher).
		Originator(builder1.GetOriginator()).
		PredefinedDependencies(txn1.ID)
	txn2 := builder2.Build()

	err := txn1.ProcessEvent(ctx, &AssembleRevertResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn1.ID,
		},
		PostAssembly: builder1.BuildPostAssembly(),
		RequestID:    mocks1.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_PreAssembly_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3)

	txn, mocks := builder.BuildWithMocks()
	assert.Equal(t, 3, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())

	mocks.Clock.Advance(builder.GetRequestTimeout() + 1)

	err := txn.ProcessEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 6, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())

	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests_Partial(t *testing.T) {
	//emulate the case where only a subset of the endorsement requests have timed out
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(4)

	txn, mocks := builder.BuildWithMocks()
	assert.Equal(t, 4, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())

	//2 endorsements come back in a timely manner
	err := txn.ProcessEvent(ctx, builder.BuildEndorsedEvent(0))
	assert.NoError(t, err)

	err = txn.ProcessEvent(ctx, builder.BuildEndorsedEvent(1))
	assert.NoError(t, err)

	mocks.Clock.Advance(builder.GetRequestTimeout() + 1)

	err = txn.ProcessEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 6, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests()) // the 4 original requests plus 2 nudge requests
	assert.Equal(t, 1, mocks.SentMessageRecorder.NumberOfEndorsementRequestsForParty(builder.GetEndorsers()[0]))
	assert.Equal(t, 1, mocks.SentMessageRecorder.NumberOfEndorsementRequestsForParty(builder.GetEndorsers()[1]))
	assert.Equal(t, 2, mocks.SentMessageRecorder.NumberOfEndorsementRequestsForParty(builder.GetEndorsers()[2]))
	assert.Equal(t, 2, mocks.SentMessageRecorder.NumberOfEndorsementRequestsForParty(builder.GetEndorsers()[3]))

	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestTransaction_Endorsement_Gathering_ToConfirmingDispatch_OnEndorsed_IfAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)

	txn, mocks := builder.BuildWithMocks()
	err := txn.ProcessEvent(ctx, builder.BuildEndorsedEvent(2))
	assert.NoError(t, err)

	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.True(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "expected a dispatch confirmation request to be sent, but none were sent")

}

func TestTransaction_Endorsement_GatheringNoTransition_IfNotAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(1) //only 1 existing endorsement so the next one does not complete the attestation plan

	txn, mocks := builder.BuildWithMocks()

	err := txn.ProcessEvent(ctx, builder.BuildEndorsedEvent(1))
	assert.NoError(t, err)

	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.False(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "did not expected a dispatch confirmation request to be sent, but one was sent")

}

func TestTransaction_Endorsement_Gathering_ToBlocked_OnEndorsed_IfAttestationPlanCompleteAndHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := NewGrapher(ctx)

	builder1 := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	txn1 := builder1.Build()

	builder2 := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		InputStateIDs(txn1.PostAssembly.OutputStates[0].ID)
	txn2 := builder2.Build()

	err := txn2.ProcessEvent(ctx, builder2.BuildEndorsedEvent(2))
	assert.NoError(t, err)

	assert.Equal(t, State_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestTransaction_Endorsement_Gathering_ToPooled_OnEndorseRejected(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)

	txn := builder.Build()
	err := txn.ProcessEvent(ctx, builder.BuildEndorseRejectedEvent(2))
	assert.NoError(t, err)

	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 1, txn.GetErrorCount())

}

func TestTransaction_ConfirmingDispatch_NudgeRequest_OnRequestTimeout(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable)
	txn, mocks := builder.BuildWithMocks()
	assert.Equal(t, 1, mocks.SentMessageRecorder.NumberOfSentDispatchConfirmationRequests())

	mocks.Clock.Advance(builder.GetRequestTimeout() + 1)

	err := txn.ProcessEvent(ctx, &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, 2, mocks.SentMessageRecorder.NumberOfSentDispatchConfirmationRequests())
	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_ConfirmingDispatch_ToReadyForDispatch_OnDispatchConfirmed(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).BuildWithMocks()

	err := txn.ProcessEvent(ctx, &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: mocks.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Ready_For_Dispatch, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_ConfirmingDispatch_NoTransition_OnDispatchConfirmed_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build()

	err := txn.ProcessEvent(ctx, &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: uuid.New(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Blocked_ToConfirmingDispatch_OnDependencyReady_IfNotHasDependenciesNotReady(t *testing.T) {
	//TODO rethink naming of this test and/or the guard function because we end up with a double negative
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  One of which (B) is ready for dispatch and the other (C) becomes ready for dispatch,
	// triggering a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	grapher := NewGrapher(ctx)

	builderB := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher)
	txnB := builderB.Build()

	builderC := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		Grapher(grapher)
	txnC, mocksC := builderC.BuildWithMocks()

	builderA := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).
		InputStateIDs(
			txnB.PostAssembly.OutputStates[0].ID,
			txnC.PostAssembly.OutputStates[0].ID,
		)
	txnA := builderA.Build()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing C to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnC.ProcessEvent(ctx, &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txnC.ID,
		},
		RequestID: mocksC.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Confirming_Dispatchable, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())

}

func TestTransaction_BlockedNoTransition_OnDependencyReady_IfHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  Neither of which a ready for dispatch. One of them (B) becomes ready for dispatch, but the other is still not ready
	// thus gating the triggering of a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	grapher := NewGrapher(ctx)

	builderB := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		Grapher(grapher)
	txnB, mocksB := builderB.BuildWithMocks()

	builderC := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		Grapher(grapher)
	txnC := builderC.Build()

	builderA := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).
		InputStateIDs(
			txnB.PostAssembly.OutputStates[0].ID,
			txnC.PostAssembly.OutputStates[0].ID,
		)
	txnA := builderA.Build()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing B to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnB.ProcessEvent(ctx, &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txnB.ID,
		},
		RequestID: mocksB.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Blocked, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())

}

func TestTransaction_ReadyForDispatch_ToDispatched_OnDispatched(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()

	err := txn.ProcessEvent(ctx, &DispatchedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Dispatched_ToSubmissionPrepared_OnCollected(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	err := txn.ProcessEvent(ctx, &CollectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_SubmissionPrepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_SubmissionPrepared_ToSubmitted_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_SubmissionPrepared).Build()

	err := txn.ProcessEvent(ctx, &SubmittedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Submitted_ToPooled_OnConfirmed_IfRevert(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Submitted).Build()

	err := txn.ProcessEvent(ctx, &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RevertReason: pldtypes.HexBytes("0x01020304"),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Submitted_ToConfirmed_IfNoRevert(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Submitted).Build()

	err := txn.ProcessEvent(ctx, &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Confirmed_ToFinal_OnHeartbeatInterval_IfHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Confirmed).HeartbeatIntervalsSinceStateChange(4).Build()

	err := txn.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err)

	assert.Equal(t, State_Final, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Confirmed_NoTransition_OnHeartbeatInterval_IfNotHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Confirmed).HeartbeatIntervalsSinceStateChange(3).Build()

	err := txn.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err)

	assert.Equal(t, State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestTransaction_Assembling_ToFinal_OnTransactionUnknownByOriginator(t *testing.T) {
	// Test that when an originator reports a transaction as unknown (most likely because
	// it reverted during assembly but the response was lost and the transaction has since
	// been cleaned up on the originator), the coordinator transitions to State_Final
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling)

	txn, mocks := txnBuilder.BuildWithMocks()

	mocks.SyncPoints.(*syncpoints.MockSyncPoints).On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.ProcessEvent(ctx, &TransactionUnknownByOriginatorEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Final, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}
