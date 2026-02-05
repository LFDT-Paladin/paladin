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
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTransaction_HasDependenciesNotReady_FalseIfNoDependencies(t *testing.T) {
	ctx := t.Context()
	transaction, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(ctx)
	assert.False(t, transaction.hasDependenciesNotReady(ctx))
}

func TestTransaction_HasDependenciesNotReady_TrueOK(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1, _ := transaction1Builder.Build(ctx)

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(transaction1.pt.PostAssembly.OutputStates[0].ID).
		AddPendingAssemblyRequest()
	transaction2, mocks2 := transaction2Builder.Build(ctx)
	mocks2.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, mock.Anything).Return(nil)

	err := transaction2.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction2.pt.ID,
		},
		PostAssembly: transaction2Builder.BuildPostAssembly(),
		PreAssembly:  transaction2Builder.BuildPreAssembly(),
		RequestID:    transaction2.pendingAssembleRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.True(t, transaction2.hasDependenciesNotReady(ctx))
}

func TestTransaction_HasDependenciesNotReady_TrueWhenStatesAreReadOnly(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1, _ := transaction1Builder.Build(ctx)

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		ReadStateIDs(transaction1.pt.PostAssembly.OutputStates[0].ID).
		AddPendingAssemblyRequest()
	transaction2, mocks2 := transaction2Builder.Build(ctx)
	mocks2.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, mock.Anything).Return(nil)

	err := transaction2.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction2.pt.ID,
		},
		PostAssembly: transaction2Builder.BuildPostAssembly(),
		PreAssembly:  transaction2Builder.BuildPreAssembly(),
		RequestID:    transaction2.pendingAssembleRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.True(t, transaction2.hasDependenciesNotReady(ctx))
}

func TestTransaction_HasDependenciesNotReady(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfEndorsements(2).
		AddPendingEndorsementRequest("endorse-2", "endorser-2@node-2")

	transaction1, mocks1 := transaction1Builder.Build(ctx)
	mocks1.Domain.On("FixedSigningIdentity").Return("fixed")

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfEndorsements(2).
		AddPendingEndorsementRequest("endorse-2", "endorser-2@node-2")

	transaction2, mocks2 := transaction2Builder.Build(ctx)
	mocks2.Domain.On("FixedSigningIdentity").Return("fixed")

	transaction3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(transaction1.pt.PostAssembly.OutputStates[0].ID, transaction2.pt.PostAssembly.OutputStates[0].ID).
		AddPendingAssemblyRequest()
	transaction3, mocks3 := transaction3Builder.Build(ctx)
	mocks3.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, mock.Anything).Return(nil)

	err := transaction3.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction3.pt.ID,
		},
		PostAssembly: transaction3Builder.BuildPostAssembly(),
		PreAssembly:  transaction3Builder.BuildPreAssembly(),
		RequestID:    transaction3.pendingAssembleRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.True(t, transaction3.hasDependenciesNotReady(ctx))

	assert.Equal(t, State_Endorsement_Gathering, transaction1.stateMachine.CurrentState)
	assert.Equal(t, State_Endorsement_Gathering, transaction2.stateMachine.CurrentState)

	//move both dependencies forward
	err = transaction1.HandleEvent(ctx, transaction1Builder.BuildEndorsedEvent(2))
	assert.NoError(t, err)
	err = transaction2.HandleEvent(ctx, transaction2Builder.BuildEndorsedEvent(2))
	assert.NoError(t, err)

	//Should still be blocked because dependencies have not been confirmed for dispatch yet
	assert.Equal(t, State_Confirming_Dispatchable, transaction1.stateMachine.CurrentState)
	assert.Equal(t, State_Confirming_Dispatchable, transaction2.stateMachine.CurrentState)
	assert.True(t, transaction3.hasDependenciesNotReady(ctx))

	//move one dependency to ready to dispatch
	err = transaction1.HandleEvent(ctx, &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction1.pt.ID,
		},
		RequestID: transaction1.pendingPreDispatchRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	//Should still be blocked because not all dependencies have been confirmed for dispatch yet
	assert.Equal(t, State_Ready_For_Dispatch, transaction1.stateMachine.CurrentState)
	assert.Equal(t, State_Confirming_Dispatchable, transaction2.stateMachine.CurrentState)
	assert.True(t, transaction3.hasDependenciesNotReady(ctx))

	//finally move the last dependency to ready to dispatch
	err = transaction2.HandleEvent(ctx, &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction2.pt.ID,
		},
		RequestID: transaction2.pendingPreDispatchRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	//Should still be blocked because not all dependencies have been confirmed for dispatch yet
	assert.Equal(t, State_Ready_For_Dispatch, transaction1.stateMachine.CurrentState)
	assert.Equal(t, State_Ready_For_Dispatch, transaction2.stateMachine.CurrentState)
	assert.False(t, transaction3.hasDependenciesNotReady(ctx))

}

func TestTransaction_HasDependenciesNotReady_FalseIfHasNoDependencies(t *testing.T) {
	ctx := t.Context()
	transaction1, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Build(ctx)
	assert.False(t, transaction1.hasDependenciesNotReady(ctx))
}

func TestTransaction_AddsItselfToGrapher(t *testing.T) {
	ctx := t.Context()
	grapher := NewGrapher(ctx)

	transaction, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build(ctx)

	txn := grapher.TransactionByID(ctx, transaction.pt.ID)

	assert.NotNil(t, txn)
}

func TestNewTransaction_InvalidOriginator_ReturnsError(t *testing.T) {
	ctx := t.Context()
	transportWriter := transport.NewMockTransportWriter(t)
	clock := &common.FakeClockForTesting{}
	engineIntegration := common.NewMockEngineIntegration(t)
	syncPoints := &syncpoints.MockSyncPoints{}

	_, err := NewTransaction(
		ctx,
		"", // invalid: empty originator
		&components.PrivateTransaction{ID: uuid.New()},
		false,
		"", // nodeName
		transportWriter,
		clock,
		func(ctx context.Context, event common.Event) {},
		engineIntegration,
		syncPoints,
		clock.Duration(1000),
		clock.Duration(5000),
		5,
		NewGrapher(ctx),
		nil,
		nil, // keyManager
		nil, // publicTxManager
		nil, // txManager
		nil, // domainAPI
		nil, // dCtx
		nil, // buildNullifiers
		nil, // newPrivateTransaction
	)
	require.Error(t, err)
}

func TestTransaction_GetSignerAddress_ReturnsSetValue(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())
	addr := pldtypes.RandAddress()
	txn.signerAddress = addr

	assert.Equal(t, addr, txn.GetSignerAddress())
}

func TestTransaction_GetNonce_ReturnsSetValue(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())
	nonce := uint64(42)
	txn.nonce = &nonce

	assert.Equal(t, &nonce, txn.GetNonce())
}

func TestTransaction_GetLatestSubmissionHash_ReturnsSetValue(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	txn.latestSubmissionHash = &hash

	assert.Equal(t, &hash, txn.GetLatestSubmissionHash())
}

func TestTransaction_GetRevertReason_ReturnsSetValue(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())
	reason := pldtypes.MustParseHexBytes("0x1234")
	txn.revertReason = reason

	assert.Equal(t, reason, txn.GetRevertReason())
}

func TestTransaction_Originator_ReturnsSetValue(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())
	txn.originator = "sender@node1"

	assert.Equal(t, "sender@node1", txn.Originator())
}

func TestTransaction_GetErrorCount_ReturnsSetValue(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())
	txn.errorCount = 3

	assert.Equal(t, 3, txn.GetErrorCount())
}

func TestTransaction_GetID_ReturnsPrivateTransactionID(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())
	id := txn.pt.ID

	assert.Equal(t, id, txn.GetID())
}

func TestTransaction_GetCurrentState_ReturnsState(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())

	assert.Equal(t, State_Initial, txn.GetCurrentState())
}

func TestTransaction_GetOutputStateIDs_ReturnsOutputStateIDs(t *testing.T) {
	ctx := t.Context()
	id1 := pldtypes.HexBytes("0x01")
	id2 := pldtypes.HexBytes("0x02")
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PostAssembly(&components.TransactionPostAssembly{
			OutputStates: []*components.FullState{
				{ID: id1},
				{ID: id2},
			},
		}).
		Build(ctx)

	ids := txn.GetOutputStateIDs()
	require.Len(t, ids, 2)
	assert.Equal(t, id1, ids[0])
	assert.Equal(t, id2, ids[1])
}

func TestTransaction_HasPreparedPrivateTransaction_TrueWhenSet(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())
	txn.pt.PreparedPrivateTransaction = &pldapi.TransactionInput{}

	assert.True(t, txn.HasPreparedPrivateTransaction())
}

func TestTransaction_HasPreparedPrivateTransaction_FalseWhenNil(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build(t.Context())
	txn.pt.PreparedPrivateTransaction = nil

	assert.False(t, txn.HasPreparedPrivateTransaction())
}

//TODO add unit test for the guards and various different combinations of dependency not ready scenarios ( e.g. pre-assemble dependencies vs post-assemble dependencies) and for those dependencies being in various different states ( the state machine test only test for "not assembled" or "not ready" but each of these "not" states actually correspond to several possible finite states.)

//TODO add unit tests to assert that if a dependency arrives after its dependent, then the dependency is correctly updated with a reference to the dependent so that we can notify the dependent when the dependency state changes ( e.g. is dispatched, is assembled)
// . - or think about whether this should this be a state machine test?

//TODO add unit test for notification function being called
// - it should be able to cause a sequencer abend if it hits an error
