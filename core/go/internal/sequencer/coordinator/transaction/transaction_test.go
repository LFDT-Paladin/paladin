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
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTransaction_HasDependenciesNotReady_FalseIfNoDependencies(t *testing.T) {
	ctx := context.Background()
	transaction, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	assert.False(t, transaction.hasDependenciesNotReady(ctx))
}

func TestTransaction_HasDependenciesNotReady_TrueOK(t *testing.T) {
	grapher := NewGrapher(t.Context())

	transaction1, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		AddPendingAssembleRequest().
		InputStateIDs(transaction1.pt.PostAssembly.OutputStates[0].ID)

	transaction2, transaction2Mocks := transaction2Builder.Build()

	transaction2Mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := transaction2.HandleEvent(context.Background(), &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction2.pt.ID,
		},
		PostAssembly: transaction2Builder.BuildPostAssembly(),
		PreAssembly:  transaction2Builder.BuildPreAssembly(),
		RequestID:    transaction2.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)
	assert.True(t, transaction2.hasDependenciesNotReady(context.Background()))
}

func TestTransaction_HasDependenciesNotReady_TrueWhenStatesAreReadOnly(t *testing.T) {
	grapher := NewGrapher(context.Background())

	transaction1, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		AddPendingAssembleRequest().
		ReadStateIDs(transaction1.pt.PostAssembly.OutputStates[0].ID)
	transaction2, transaction2Mocks := transaction2Builder.Build()

	transaction2Mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := transaction2.HandleEvent(context.Background(), &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction2.pt.ID,
		},
		PostAssembly: transaction2Builder.BuildPostAssembly(),
		PreAssembly:  transaction2Builder.BuildPreAssembly(),
		RequestID:    transaction2.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)
	assert.True(t, transaction2.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_HasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		AddPendingEndorsementRequest(2).
		AddPendingPreDispatchRequest()
	transaction1, _ := transaction1Builder.Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		AddPendingEndorsementRequest(2).
		AddPendingPreDispatchRequest()

	transaction2, _ := transaction2Builder.Build()

	transaction3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		AddPendingAssembleRequest().
		InputStateIDs(transaction1.pt.PostAssembly.OutputStates[0].ID, transaction2.pt.PostAssembly.OutputStates[0].ID)
	transaction3, transaction3Mocks := transaction3Builder.Build()

	transaction3Mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(mock.Anything, mock.Anything).Return(nil)

	err := transaction3.HandleEvent(context.Background(), &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction3.pt.ID,
		},
		PostAssembly: transaction3Builder.BuildPostAssembly(),
		PreAssembly:  transaction3Builder.BuildPreAssembly(),
		RequestID:    transaction3.pendingAssembleRequest.IdempotencyKey(),
	})
	require.NoError(t, err)

	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

	assert.Equal(t, State_Endorsement_Gathering, transaction1.stateMachine.CurrentState)
	assert.Equal(t, State_Endorsement_Gathering, transaction2.stateMachine.CurrentState)

	//move both dependencies forward
	err = transaction1.HandleEvent(ctx, transaction1Builder.BuildEndorsedEvent(2))
	require.NoError(t, err)
	err = transaction2.HandleEvent(ctx, transaction2Builder.BuildEndorsedEvent(2))
	require.NoError(t, err)

	//Should still be blocked because dependencies have not been confirmed for dispatch yet
	assert.Equal(t, State_Confirming_Dispatchable, transaction1.stateMachine.CurrentState)
	assert.Equal(t, State_Confirming_Dispatchable, transaction2.stateMachine.CurrentState)
	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

	//move one dependency to ready to dispatch
	err = transaction1.HandleEvent(ctx, &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction1.pt.ID,
		},
		RequestID: transaction1.pendingPreDispatchRequest.IdempotencyKey(),
	})
	require.NoError(t, err)

	//Should still be blocked because not all dependencies have been confirmed for dispatch yet
	assert.Equal(t, State_Ready_For_Dispatch, transaction1.stateMachine.CurrentState)
	assert.Equal(t, State_Confirming_Dispatchable, transaction2.stateMachine.CurrentState)
	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

	//finally move the last dependency to ready to dispatch
	err = transaction2.HandleEvent(ctx, &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction2.pt.ID,
		},
		RequestID: transaction2.pendingPreDispatchRequest.IdempotencyKey(),
	})
	require.NoError(t, err)

	//Should still be blocked because not all dependencies have been confirmed for dispatch yet
	assert.Equal(t, State_Ready_For_Dispatch, transaction1.stateMachine.CurrentState)
	assert.Equal(t, State_Ready_For_Dispatch, transaction2.stateMachine.CurrentState)
	assert.False(t, transaction3.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_HasDependenciesNotReady_FalseIfHasNoDependencies(t *testing.T) {
	transaction1, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Build()

	assert.False(t, transaction1.hasDependenciesNotReady(context.Background()))
}

func TestTransaction_AddsItselfToGrapher(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	transaction, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build()

	txn := grapher.TransactionByID(ctx, transaction.pt.ID)
	assert.NotNil(t, txn)
}

func TestNewTransaction_InvalidOriginator_ReturnsError(t *testing.T) {
	ctx := context.Background()
	clock := &common.FakeClockForTesting{}

	_, err := NewTransaction(
		ctx,
		"", // invalid: empty originator
		"node1",
		&components.PrivateTransaction{ID: uuid.New()},
		"coordinator-signer",
		transport.NewMockTransportWriter(t),
		clock,
		func(ctx context.Context, event common.Event) {},
		common.NewMockEngineIntegration(t),
		&syncpoints.MockSyncPoints{},
		componentsmocks.NewAllComponents(t),
		componentsmocks.NewDomainSmartContract(t),
		nil,
		clock.Duration(1000),
		clock.Duration(5000),
		5,
		0,
		NewGrapher(ctx),
		nil,
	)
	require.Error(t, err)
}

func TestTransaction_GetSignerAddress_ReturnsSetValue(t *testing.T) {
	addr := pldtypes.RandAddress()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).SignerAddress(addr).Build()

	assert.Equal(t, addr, txn.GetSignerAddress())
}

func TestTransaction_GetNonce_ReturnsSetValue(t *testing.T) {
	nonce := uint64(42)

	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Nonce(&nonce).Build()
	assert.Equal(t, &nonce, txn.GetNonce())
}

func TestTransaction_GetLatestSubmissionHash_ReturnsSetValue(t *testing.T) {
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).SubmissionHash(hash).Build()

	assert.Equal(t, &hash, txn.GetLatestSubmissionHash())
}

func TestTransaction_GetRevertReason_ReturnsSetValue(t *testing.T) {
	reason := pldtypes.MustParseHexBytes("0x1234")
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).RevertReason(reason).Build()

	assert.Equal(t, reason, txn.GetRevertReason())
}

func TestTransaction_Originator_ReturnsSetValue(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Originator("sender@node1").Build()

	assert.Equal(t, "sender@node1", txn.Originator())
}

func TestTransaction_GetErrorCount_ReturnsSetValue(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).ErrorCount(3).Build()

	assert.Equal(t, 3, txn.GetErrorCount())
}

func TestTransaction_GetID_ReturnsPrivateTransactionID(t *testing.T) {
	id := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).TransactionID(id).Build()

	assert.Equal(t, id, txn.GetID())
}

func TestTransaction_GetDomain_ReturnsPrivateTransactionDomain(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Domain("test-domain").Build()

	assert.Equal(t, "test-domain", txn.GetDomain())
}

func TestTransaction_GetCurrentState_ReturnsState(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	assert.Equal(t, State_Initial, txn.GetCurrentState())
}

func TestTransaction_GetPrivateTransaction_ReturnsPt(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	pt := txn.pt

	assert.Same(t, pt, txn.GetPrivateTransaction())
}

func TestTransaction_GetContractAddress_ReturnsAddress(t *testing.T) {
	addr := *pldtypes.RandAddress()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Address(addr).Build()

	assert.Equal(t, addr, txn.GetContractAddress())
}

func TestTransaction_GetTransactionSpecification_ReturnsPreAssemblySpec(t *testing.T) {
	spec := &prototk.TransactionSpecification{}
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PreAssembly(&components.TransactionPreAssembly{TransactionSpecification: spec}).
		Build()

	assert.Same(t, spec, txn.GetTransactionSpecification())
}

func TestTransaction_GetOriginalSender_ReturnsFrom(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PreAssembly(&components.TransactionPreAssembly{TransactionSpecification: &prototk.TransactionSpecification{From: "0xSender"}}).
		Build()
	assert.Equal(t, "0xSender", txn.GetOriginalSender())
}

func TestTransaction_GetOutputStateIDs_ReturnsOutputStateIDs(t *testing.T) {
	id1 := pldtypes.HexBytes("0x01")
	id2 := pldtypes.HexBytes("0x02")
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PostAssembly(&components.TransactionPostAssembly{OutputStates: []*components.FullState{
			{ID: id1},
			{ID: id2},
		}}).
		Build()

	ids := txn.GetOutputStateIDs()
	require.Len(t, ids, 2)
	assert.Equal(t, id1, ids[0])
	assert.Equal(t, id2, ids[1])
}

func TestTransaction_HasPreparedPrivateTransaction_TrueWhenSet(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		Build()
	assert.True(t, txn.HasPreparedPrivateTransaction())
}

func TestTransaction_HasPreparedPrivateTransaction_FalseWhenNil(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	assert.False(t, txn.HasPreparedPrivateTransaction())
}

func TestTransaction_HasPreparedPublicTransaction_TrueWhenSet(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		Build()
	assert.True(t, txn.HasPreparedPublicTransaction())
}

func TestTransaction_HasPreparedPublicTransaction_FalseWhenNil(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	assert.False(t, txn.HasPreparedPublicTransaction())
}

func TestTransaction_GetSigner_ReturnsSigner(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Signer("signer-identity").Build()
	assert.Equal(t, "signer-identity", txn.GetSigner())
}
