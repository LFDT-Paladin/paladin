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
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
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

func TestNewTransaction_Success_ReturnsTransaction(t *testing.T) {
	ctx := context.Background()
	pt := &components.PrivateTransaction{ID: uuid.New()}
	allComponents := componentsmocks.NewAllComponents(t)
	domainAPI := componentsmocks.NewDomainSmartContract(t)
	domain := componentsmocks.NewDomain(t)
	clock := common.NewMockClock(t)

	domainAPI.EXPECT().Domain().Return(domain)
	domain.EXPECT().FixedSigningIdentity().Return("domain-signer")
	domainAPI.EXPECT().ContractConfig().Return(&prototk.ContractConfig{
		SubmitterSelection: prototk.ContractConfig_SUBMITTER_COORDINATOR,
	})
	clock.EXPECT().Now().Return(time.Now())

	txn, err := newTransaction(
		ctx,
		"sender@node1",
		"node1",
		pt,
		"coordinator-signer",
		nil,
		transport.NewMockTransportWriter(t),
		clock,
		func(ctx context.Context, event common.Event) {},
		common.NewMockEngineIntegration(t),
		&syncpoints.MockSyncPoints{},
		allComponents,
		domainAPI,
		nil,
		time.Duration(1000),
		time.Duration(5000),
		5,
		0,
		3,
		3,
		NewGrapher(ctx),
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, txn)
	assert.Equal(t, pt.ID, txn.GetID())
	assert.Equal(t, State_Initial, txn.GetCurrentState())
}

func TestNewTransaction_PublicAPI_ReturnsTransaction(t *testing.T) {
	ctx := context.Background()
	pt := &components.PrivateTransaction{ID: uuid.New()}
	allComponents := componentsmocks.NewAllComponents(t)
	domainAPI := componentsmocks.NewDomainSmartContract(t)
	domain := componentsmocks.NewDomain(t)
	clock := common.NewMockClock(t)

	domainAPI.EXPECT().Domain().Return(domain)
	domain.EXPECT().FixedSigningIdentity().Return("domain-signer")
	domainAPI.EXPECT().ContractConfig().Return(&prototk.ContractConfig{
		SubmitterSelection: prototk.ContractConfig_SUBMITTER_COORDINATOR,
	})
	clock.EXPECT().Now().Return(time.Now())

	txn, err := NewTransaction(
		ctx,
		"sender@node1",
		"node1",
		pt,
		"coordinator-signer",
		nil,
		transport.NewMockTransportWriter(t),
		clock,
		func(ctx context.Context, event common.Event) {},
		common.NewMockEngineIntegration(t),
		&syncpoints.MockSyncPoints{},
		allComponents,
		domainAPI,
		nil,
		time.Duration(1000),
		time.Duration(5000),
		5,
		0,
		3,
		3,
		NewGrapher(ctx),
		metrics.InitMetrics(ctx, prometheus.NewRegistry()),
	)
	require.NoError(t, err)
	require.NotNil(t, txn)
	assert.Equal(t, pt.ID, txn.GetID())
	assert.Equal(t, State_Initial, txn.GetCurrentState())
}

func TestNewTransaction_InvalidOriginator_ReturnsError(t *testing.T) {
	ctx := context.Background()
	pt := &components.PrivateTransaction{ID: uuid.New()}
	allComponents := componentsmocks.NewAllComponents(t)
	domainAPI := componentsmocks.NewDomainSmartContract(t)
	clock := common.NewMockClock(t)

	// "sender" has no @ so with allowEmptyNode false, Validate returns an error
	txn, err := newTransaction(
		ctx,
		"sender",
		"node1",
		pt,
		"coordinator-signer",
		nil,
		transport.NewMockTransportWriter(t),
		clock,
		func(ctx context.Context, event common.Event) {},
		common.NewMockEngineIntegration(t),
		&syncpoints.MockSyncPoints{},
		allComponents,
		domainAPI,
		nil,
		time.Duration(1000),
		time.Duration(5000),
		5,
		0,
		3,
		3,
		NewGrapher(ctx),
		nil,
	)
	require.Error(t, err)
	require.Nil(t, txn)
}

func TestTransaction_GetID_ReturnsPrivateTransactionID(t *testing.T) {
	id := uuid.New()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).TransactionID(id).Build()

	assert.Equal(t, id, txn.GetID())
}

func TestTransaction_GetCurrentState_ReturnsState(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	assert.Equal(t, State_Initial, txn.GetCurrentState())
}

func TestTransaction_HasDispatchedPublicTransaction_TrueWhenSetAndIntentIsSend(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		Build()
	assert.True(t, txn.HasDispatchedPublicTransaction())
}

func TestTransaction_HasDispatchedPublicTransaction_FalseWhenSetAndIntentIsNotSend(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_PREPARE_TRANSACTION,
			},
		}).
		Build()
	assert.False(t, txn.HasDispatchedPublicTransaction())
}

func TestTransaction_HasDispatchedPublicTransaction_FalseWhenNil(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		Build()

	assert.False(t, txn.HasDispatchedPublicTransaction())
}
