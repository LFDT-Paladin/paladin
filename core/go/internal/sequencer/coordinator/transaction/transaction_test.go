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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransaction_HasDependenciesNotReady_FalseIfNoDependencies(t *testing.T) {
	ctx := context.Background()
	transaction, _ := newTransactionForUnitTesting(t, nil)
	assert.False(t, transaction.hasDependenciesNotReady(ctx))
}

func TestTransaction_HasDependenciesNotReady_TrueOK(t *testing.T) {
	grapher := NewGrapher(context.Background())

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1 := transaction1Builder.Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(transaction1.pt.PostAssembly.OutputStates[0].ID)
	transaction2 := transaction2Builder.Build()

	err := transaction2.HandleEvent(context.Background(), &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction2.pt.ID,
		},
		PostAssembly: transaction2Builder.BuildPostAssembly(),
		PreAssembly:  transaction2Builder.BuildPreAssembly(),
		RequestID:    transaction2.pendingAssembleRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.True(t, transaction2.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_HasDependenciesNotReady_TrueWhenStatesAreReadOnly(t *testing.T) {
	grapher := NewGrapher(context.Background())

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1 := transaction1Builder.Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		ReadStateIDs(transaction1.pt.PostAssembly.OutputStates[0].ID)
	transaction2 := transaction2Builder.Build()

	err := transaction2.HandleEvent(context.Background(), &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction2.pt.ID,
		},
		PostAssembly: transaction2Builder.BuildPostAssembly(),
		PreAssembly:  transaction2Builder.BuildPreAssembly(),
		RequestID:    transaction2.pendingAssembleRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.True(t, transaction2.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_HasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1 := transaction1Builder.Build()
	transaction1.dynamicSigningIdentity = false

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction2 := transaction2Builder.Build()
	transaction2.dynamicSigningIdentity = false

	transaction3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(transaction1.pt.PostAssembly.OutputStates[0].ID, transaction2.pt.PostAssembly.OutputStates[0].ID)
	transaction3 := transaction3Builder.Build()

	err := transaction3.HandleEvent(context.Background(), &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: transaction3.pt.ID,
		},
		PostAssembly: transaction3Builder.BuildPostAssembly(),
		PreAssembly:  transaction3Builder.BuildPreAssembly(),
		RequestID:    transaction3.pendingAssembleRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

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
	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

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
	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

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
	assert.False(t, transaction3.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_HasDependenciesNotReady_FalseIfHasNoDependencies(t *testing.T) {

	transaction1 := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Build()

	assert.False(t, transaction1.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_AddsItselfToGrapher(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	transaction, _ := newTransactionForUnitTesting(t, grapher)

	txn := grapher.TransactionByID(ctx, transaction.pt.ID)

	assert.NotNil(t, txn)
}

type transactionDependencyMocks struct {
	transportWriter   *transport.MockTransportWriter
	clock             *common.FakeClockForTesting
	engineIntegration *common.MockEngineIntegration
	syncPoints        syncpoints.SyncPoints
}

func newTransactionForUnitTesting(t *testing.T, grapher Grapher) (*Transaction, *transactionDependencyMocks) {
	if grapher == nil {
		grapher = NewGrapher(context.Background())
	}
	mocks := &transactionDependencyMocks{
		transportWriter:   transport.NewMockTransportWriter(t),
		clock:             &common.FakeClockForTesting{},
		engineIntegration: common.NewMockEngineIntegration(t),
		syncPoints:        &syncpoints.MockSyncPoints{},
	}
	txn, err := NewTransaction(
		context.Background(),
		fmt.Sprintf("%s@%s", uuid.NewString(), uuid.NewString()),
		&components.PrivateTransaction{
			ID: uuid.New(),
		},
		false,
		mocks.transportWriter,
		mocks.clock,
		func(ctx context.Context, event common.Event) {
			// No-op event handler for tests
		},
		mocks.engineIntegration,
		mocks.syncPoints,
		mocks.clock.Duration(1000),
		mocks.clock.Duration(5000),
		5,
		"",
		prototk.ContractConfig_SUBMITTER_COORDINATOR,
		grapher,
		nil,
	)
	require.NoError(t, err)

	return txn, mocks

}

func TestNewTransaction_InvalidOriginator_ReturnsError(t *testing.T) {
	ctx := context.Background()
	transportWriter := transport.NewMockTransportWriter(t)
	clock := &common.FakeClockForTesting{}
	engineIntegration := common.NewMockEngineIntegration(t)
	syncPoints := &syncpoints.MockSyncPoints{}

	_, err := NewTransaction(
		ctx,
		"", // invalid: empty originator
		&components.PrivateTransaction{ID: uuid.New()},
		false,
		transportWriter,
		clock,
		func(ctx context.Context, event common.Event) {},
		engineIntegration,
		syncPoints,
		clock.Duration(1000),
		clock.Duration(5000),
		5,
		"",
		prototk.ContractConfig_SUBMITTER_COORDINATOR,
		NewGrapher(ctx),
		nil,
	)
	require.Error(t, err)
}

func TestTransaction_GetSignerAddress_ReturnsSetValue(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	addr := pldtypes.RandAddress()
	txn.signerAddress = addr

	assert.Equal(t, addr, txn.GetSignerAddress())
}

func TestTransaction_GetNonce_ReturnsSetValue(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	nonce := uint64(42)
	txn.nonce = &nonce

	assert.Equal(t, &nonce, txn.GetNonce())
}

func TestTransaction_GetLatestSubmissionHash_ReturnsSetValue(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	txn.latestSubmissionHash = &hash

	assert.Equal(t, &hash, txn.GetLatestSubmissionHash())
}

func TestTransaction_GetRevertReason_ReturnsSetValue(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	reason := pldtypes.MustParseHexBytes("0x1234")
	txn.revertReason = reason

	assert.Equal(t, reason, txn.GetRevertReason())
}

func TestTransaction_Originator_ReturnsSetValue(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.originator = "sender@node1"

	assert.Equal(t, "sender@node1", txn.Originator())
}

func TestTransaction_GetErrorCount_ReturnsSetValue(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.errorCount = 3

	assert.Equal(t, 3, txn.GetErrorCount())
}

func TestTransaction_GetID_ReturnsPrivateTransactionID(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	id := txn.pt.ID

	assert.Equal(t, id, txn.GetID())
}

func TestTransaction_GetDomain_ReturnsPrivateTransactionDomain(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.Domain = "test-domain"

	assert.Equal(t, "test-domain", txn.GetDomain())
}

func TestTransaction_GetCurrentState_ReturnsState(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)

	assert.Equal(t, State_Initial, txn.GetCurrentState())
}

func TestTransaction_GetPrivateTransaction_ReturnsPt(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	pt := txn.pt

	assert.Same(t, pt, txn.GetPrivateTransaction())
}

func TestTransaction_GetContractAddress_ReturnsAddress(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	addr := *pldtypes.RandAddress()
	txn.pt.Address = addr

	assert.Equal(t, addr, txn.GetContractAddress())
}

func TestTransaction_GetTransactionSpecification_ReturnsPreAssemblySpec(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	spec := &prototk.TransactionSpecification{}
	txn.pt.PreAssembly = &components.TransactionPreAssembly{TransactionSpecification: spec}

	assert.Same(t, spec, txn.GetTransactionSpecification())
}

func TestTransaction_GetOriginalSender_ReturnsFrom(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{From: "0xSender"},
	}

	assert.Equal(t, "0xSender", txn.GetOriginalSender())
}

func TestTransaction_GetOutputStateIDs_ReturnsOutputStateIDs(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	id1 := pldtypes.HexBytes("0x01")
	id2 := pldtypes.HexBytes("0x02")
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		OutputStates: []*components.FullState{
			{ID: id1},
			{ID: id2},
		},
	}

	ids := txn.GetOutputStateIDs()
	require.Len(t, ids, 2)
	assert.Equal(t, id1, ids[0])
	assert.Equal(t, id2, ids[1])
}

func TestTransaction_HasPreparedPrivateTransaction_TrueWhenSet(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PreparedPrivateTransaction = &pldapi.TransactionInput{}

	assert.True(t, txn.HasPreparedPrivateTransaction())
}

func TestTransaction_HasPreparedPrivateTransaction_FalseWhenNil(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PreparedPrivateTransaction = nil

	assert.False(t, txn.HasPreparedPrivateTransaction())
}

func TestTransaction_HasPreparedPublicTransaction_TrueWhenSet(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PreparedPublicTransaction = &pldapi.TransactionInput{}

	assert.True(t, txn.HasPreparedPublicTransaction())
}

func TestTransaction_HasPreparedPublicTransaction_FalseWhenNil(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PreparedPublicTransaction = nil

	assert.False(t, txn.HasPreparedPublicTransaction())
}

func TestTransaction_GetSigner_ReturnsSigner(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.Signer = "signer-identity"

	assert.Equal(t, "signer-identity", txn.GetSigner())
}

//TODO add unit test for the guards and various different combinations of dependency not ready scenarios ( e.g. pre-assemble dependencies vs post-assemble dependencies) and for those dependencies being in various different states ( the state machine test only test for "not assembled" or "not ready" but each of these "not" states actually correspond to several possible finite states.)

//TODO add unit tests to assert that if a dependency arrives after its dependent, then the dependency is correctly updated with a reference to the dependent so that we can notify the dependent when the dependency state changes ( e.g. is dispatched, is assembled)
// . - or think about whether this should this be a state machine test?

//TODO add unit test for notification function being called
// - it should be able to cause a sequencer abend if it hits an error
