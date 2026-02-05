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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCoordinatorTransaction_Initial_ToPooled_OnReceived_IfNoInflightDependencies(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)

	err := txn.HandleEvent(ctx, &transaction.ReceivedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Initial_ToPreAssemblyBlocked_OnReceived_IfDependencyNotAssembled(t *testing.T) {

	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	//transaction2 depends on transaction 1 and transaction 1 gets reverted
	builder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).
		Grapher(grapher)

	txn1, _ := builder1.Build(ctx)

	builder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Initial).
		Grapher(grapher).
		Originator(builder1.GetOriginator()).
		PredefinedDependencies(txn1.GetID())
	txn2, _ := builder2.Build(ctx)

	err := txn2.HandleEvent(ctx, &transaction.ReceivedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn2.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_PreAssembly_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestCoordinatorTransaction_Initial_ToPreAssemblyBlocked_OnReceived_IfDependencyUnknown(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Initial).
		PredefinedDependencies(uuid.New())
	txn, _ := builder.Build(ctx)

	err := txn.HandleEvent(ctx, &transaction.ReceivedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_PreAssembly_Blocked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestCoordinatorTransaction_Pooled_ToAssembling_OnSelected(t *testing.T) {
	ctx := context.Background()

	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build(ctx)
	mocks.EngineIntegration.On("GetStateLocks", mock.Anything).Return([]byte("{}"), nil)
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(100), nil)

	err := txn.HandleEvent(ctx, &transaction.SelectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, true, mocks.SentMessageRecorder.HasSentAssembleRequest())
}

func TestCoordinatorTransaction_Assembling_ToEndorsing_OnAssembleResponse(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		AddPendingAssemblyRequest()
	txn, mocks := txnBuilder.Build(ctx)
	mocks.EngineIntegration.On("WriteLockStatesForTransaction", mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, &transaction.AssembleSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		PreAssembly:  txnBuilder.BuildPreAssembly(),
		RequestID:    txnBuilder.GetPendingAssemblyRequest().IdempotencyKey(),
	})
	assert.NoError(t, err)
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 3, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())
	//TODO some assertions that WriteLockAndDistributeStatesForTransaction was called with the expected states

}

func TestCoordinatorTransaction_Assembling_NoTransition_OnAssembleResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn, _ := txnBuilder.Build(ctx)

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

func TestCoordinatorTransaction_Assembling_NoTransition_OnRequestTimeout_IfNotAssembleTimeoutExpired(t *testing.T) {
	ctx := context.Background()
	var requestsSent int
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		RequestTimeout(1).
		AssembleTimeout(10).
		AddPendingAssemblyRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error { requestsSent++; return nil })
	txn, mocks := txnBuilder.Build(ctx)

	mocks.Clock.Advance(9)

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, requestsSent)

	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToPooled_OnRequestTimeout_IfAssembleTimeoutExpired(t *testing.T) {
	ctx := context.Background()
	var requestsSent int
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		RequestTimeout(1).
		AssembleTimeout(10).
		AddPendingAssemblyRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error { requestsSent++; return nil })
	txn, mocks := txnBuilder.Build(ctx)

	mocks.Clock.Advance(11)

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 1, txn.GetErrorCount(), "expected error count to be 1, but it was %d", txn.GetErrorCount())
}

func TestCoordinatorTransaction_Assembling_ToReverted_OnAssembleRevertResponse(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		AddPendingAssemblyRequest().
		Reverts("some revert reason")

	txn, mocks := txnBuilder.Build(ctx)

	mocks.SyncPoints.On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, &transaction.AssembleRevertResponseEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    txnBuilder.GetPendingAssemblyRequest().IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_NoTransition_OnAssembleRevertResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		Reverts("some revert reason")

	txn, _ := txnBuilder.Build(ctx)

	err := txn.HandleEvent(ctx, &transaction.AssembleRevertResponseEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    uuid.New(), //generate a new random request ID so that it won't match the pending request,
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Pooled_ToPreAssemblyBlocked_OnDependencyReverted(t *testing.T) {
	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)
	//transaction2 depends on transaction 1 and transaction 1 gets reverted
	txn1Preassembly := &components.TransactionPreAssembly{
		Dependencies: &pldapi.TransactionDependencies{},
	}
	builder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		Grapher(grapher).
		PreAssembly(txn1Preassembly).
		AddPendingAssemblyRequest().
		Reverts("some revert reason")
	txn1, mocks1 := builder1.Build(ctx)

	mocks1.SyncPoints.On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	builder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).
		Grapher(grapher).
		Originator(builder1.GetOriginator()).
		PredefinedDependencies(txn1.GetID())
	txn2, _ := builder2.Build(ctx)
	txn1Preassembly.Dependencies.PrereqOf = append(txn1Preassembly.Dependencies.PrereqOf, txn2.GetID())

	err := txn1.HandleEvent(ctx, &transaction.AssembleRevertResponseEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn1.GetID(),
		},
		PostAssembly: builder1.BuildPostAssembly(),
		RequestID:    builder1.GetPendingAssemblyRequest().IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_PreAssembly_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3)

	txn, mocks := builder.Build(ctx)

	mocks.Clock.Advance(builder.GetRequestTimeout() + 1)

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())

	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests_Partial(t *testing.T) {
	//emulate the case where only a subset of the endorsement requests have timed out
	ctx := context.Background()
	sentEndorsementRequests := map[uuid.UUID]int{}
	totalRequests := 0
	onSend := func(ctx context.Context, idempotencyKey uuid.UUID) error {
		totalRequests++
		sentEndorsementRequests[idempotencyKey]++
		return nil
	}
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		RequestTimeout(1).
		AddPendingEndorsementRequestWithCallback("endorse-0", "endorser-0@node-0", onSend).
		AddPendingEndorsementRequestWithCallback("endorse-1", "endorser-1@node-1", onSend).
		AddPendingEndorsementRequestWithCallback("endorse-2", "endorser-2@node-2", onSend).
		AddPendingEndorsementRequestWithCallback("endorse-3", "endorser-3@node-3", onSend).
		NumberOfRequiredEndorsers(4)

	txn, mocks := builder.Build(ctx)

	//2 endorsements come back in a timely manner
	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(0))
	assert.NoError(t, err)

	err = txn.HandleEvent(ctx, builder.BuildEndorsedEvent(1))
	assert.NoError(t, err)

	mocks.Clock.Advance(2)

	err = txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, totalRequests)
	assert.Equal(t, 1, sentEndorsementRequests[builder.GetEndorsementRequest("endorse-2", "endorser-2@node-2").IdempotencyKey()])
	assert.Equal(t, 1, sentEndorsementRequests[builder.GetEndorsementRequest("endorse-3", "endorser-3@node-3").IdempotencyKey()])

	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_ToConfirmingDispatch_OnEndorsed_IfAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		AddPendingEndorsementRequest("endorse-2", "endorser-2@node-2")

	txn, mocks := builder.Build(ctx)
	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(2))
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.True(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "expected a dispatch confirmation request to be sent, but none were sent")

}

func TestCoordinatorTransaction_Endorsement_GatheringNoTransition_IfNotAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(1). //only 1 existing endorsement so the next one does not complete the attestation plan
		AddPendingEndorsementRequest("endorse-1", "endorser-1@node-1")
	txn, mocks := builder.Build(ctx)

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(1))
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.False(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "did not expected a dispatch confirmation request to be sent, but one was sent")

}

func TestCoordinatorTransaction_Endorsement_Gathering_ToBlocked_OnEndorsed_IfAttestationPlanCompleteAndHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	builder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	txn1, _ := builder1.Build(ctx)

	builder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		InputStateIDs(txn1.GetOutputStateIDs()[0]).
		AddPendingEndorsementRequest("endorse-2", "endorser-2@node-2")
	txn2, _ := builder2.Build(ctx)

	err := txn2.HandleEvent(ctx, builder2.BuildEndorsedEvent(2))
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_ToPooled_OnEndorseRejected(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		AddPendingEndorsementRequest("endorse-2", "endorser-2@node-2")
	txn, _ := builder.Build(ctx)
	err := txn.HandleEvent(ctx, builder.BuildEndorseRejectedEvent(2))
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 1, txn.GetErrorCount())

}

func TestCoordinatorTransaction_ConfirmingDispatch_NudgeRequest_OnRequestTimeout(t *testing.T) {
	ctx := context.Background()
	var nudged bool
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		AddPendingPreDispatchRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error { nudged = true; return nil })
	txn, mocks := builder.Build(ctx)

	mocks.Clock.Advance(builder.GetRequestTimeout() + 1)

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.True(t, nudged)
	assert.Equal(t, transaction.State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_ToReadyForDispatch_OnDispatchApproved(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest()
	txn, mocks := builder.Build(ctx)
	mocks.Domain.On("FixedSigningIdentity").Return("")

	err := txn.HandleEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RequestID: builder.GetPendingPreDispatchRequest().IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Ready_For_Dispatch, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_NoTransition_OnDispatchConfirmed_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build(ctx)

	err := txn.HandleEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RequestID: uuid.New(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Blocked_ToConfirmingDispatch_OnDependencyReady_IfNotHasDependenciesNotReady(t *testing.T) {
	//TODO rethink naming of this test and/or the guard function because we end up with a double negative
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  One of which (B) is ready for dispatch and the other (C) becomes ready for dispatch,
	// triggering a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	builderB := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).
		Grapher(grapher)
	txnB, _ := builderB.Build(ctx)

	builderC := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		Grapher(grapher).
		AddPendingPreDispatchRequest()
	txnC, mocksC := builderC.Build(ctx)
	mocksC.Domain.On("FixedSigningIdentity").Return("")

	builderA := transaction.NewTransactionBuilderForTesting(t, transaction.State_Blocked).
		Grapher(grapher).
		InputStateIDs(
			txnB.GetOutputStateIDs()[0],
			txnC.GetOutputStateIDs()[0],
		)
	txnA, _ := builderA.Build(ctx)

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing C to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnC.HandleEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txnC.GetID(),
		},
		RequestID: builderC.GetPendingPreDispatchRequest().IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirming_Dispatchable, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())

}

func TestCoordinatorTransaction_BlockedNoTransition_OnDependencyReady_IfHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  Neither of which a ready for dispatch. One of them (B) becomes ready for dispatch, but the other is still not ready
	// thus gating the triggering of a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	builderB := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		Grapher(grapher).
		AddPendingPreDispatchRequest()
	txnB, mocksB := builderB.Build(ctx)
	mocksB.Domain.On("FixedSigningIdentity").Return("")

	builderC := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		Grapher(grapher)
	txnC, _ := builderC.Build(ctx)

	builderA := transaction.NewTransactionBuilderForTesting(t, transaction.State_Blocked).
		Grapher(grapher).
		InputStateIDs(
			txnB.GetOutputStateIDs()[0],
			txnC.GetOutputStateIDs()[0],
		)
	txnA, _ := builderA.Build(ctx)

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing B to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnB.HandleEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txnB.GetID(),
		},
		RequestID: builderB.GetPendingPreDispatchRequest().IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Blocked, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())

}

func TestCoordinatorTransaction_ReadyForDispatch_ToDispatched_OnDispatched(t *testing.T) {
	ctx := context.Background()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).
		Build(ctx)
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.ChainedPrivateTransaction{
		NewTransaction: &components.ValidatedTransaction{},
	}, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, &transaction.DispatchEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToSubmissionPrepared_OnCollected(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build(ctx)

	err := txn.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_SubmissionPrepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_SubmissionPrepared_ToSubmitted_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_SubmissionPrepared).Build(ctx)

	err := txn.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Submitted_ToPooled_OnConfirmed_IfRevert(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build(ctx)

	err := txn.HandleEvent(ctx, &transaction.ConfirmedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
		RevertReason: pldtypes.HexBytes("0x01020304"),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Submitted_ToConfirmed_IfNoRevert(t *testing.T) {
	ctx := context.Background()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build(ctx)
	mocks.EngineIntegration.On("ResetTransactions", mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, &transaction.ConfirmedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Confirmed_ToFinal_OnHeartbeatInterval_IfHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).HeartbeatIntervalsSinceStateChange(4).Build(ctx)

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Confirmed_NoTransition_OnHeartbeatInterval_IfNotHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).HeartbeatIntervalsSinceStateChange(3).Build(ctx)

	err := txn.HandleEvent(ctx, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToFinal_OnTransactionUnknownByOriginator(t *testing.T) {
	// Test that when an originator reports a transaction as unknown (most likely because
	// it reverted during assembly but the response was lost and the transaction has since
	// been cleaned up on the originator), the coordinator transitions to State_Final
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)

	txn, mocks := txnBuilder.Build(ctx)

	mocks.SyncPoints.On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, &transaction.TransactionUnknownByOriginatorEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}
