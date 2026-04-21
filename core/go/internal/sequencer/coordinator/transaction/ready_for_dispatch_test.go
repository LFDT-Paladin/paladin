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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_UpdateSigningIdentity_CallsUpdateSigningIdentity(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		PostAssembly(&components.TransactionPostAssembly{
			Endorsements: []*prototk.AttestationResult{
				{
					Verifier:    &prototk.ResolvedVerifier{Lookup: "signer1"},
					Constraints: []prototk.AttestationResult_AttestationConstraint{prototk.AttestationResult_ENDORSER_MUST_SUBMIT},
				},
			},
		}).
		SubmitterSelection(prototk.ContractConfig_SUBMITTER_COORDINATOR).
		Build()

	err := action_UpdateSigningIdentity(ctx, txn, nil)
	assert.NoError(t, err)
	assert.Equal(t, "signer1", txn.pt.Signer)
}

func Test_updateSigningIdentity_NoPostAssembly(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		SubmitterSelection(prototk.ContractConfig_SUBMITTER_COORDINATOR).
		Build()
	txn.pt.PostAssembly = nil

	txn.updateSigningIdentity()

	assert.Empty(t, txn.pt.Signer)
}

func Test_updateSigningIdentity_NoEndorsements(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{
			Endorsements: []*prototk.AttestationResult{},
		}).
		SubmitterSelection(prototk.ContractConfig_SUBMITTER_COORDINATOR).
		Build()

	txn.updateSigningIdentity()

	assert.Empty(t, txn.pt.Signer)
}

func Test_updateSigningIdentity_EndorsementWithConstraint(t *testing.T) {
	verifierLookup := "verifier1"

	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{
			Endorsements: []*prototk.AttestationResult{
				{
					Verifier: &prototk.ResolvedVerifier{
						Lookup: verifierLookup,
					},
					Constraints: []prototk.AttestationResult_AttestationConstraint{
						prototk.AttestationResult_ENDORSER_MUST_SUBMIT,
					},
				},
			},
		}).
		SubmitterSelection(prototk.ContractConfig_SUBMITTER_COORDINATOR).
		Build()

	txn.updateSigningIdentity()

	assert.Equal(t, verifierLookup, txn.pt.Signer)
}

func Test_updateSigningIdentity_EndorsementWithoutConstraint(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{
			Endorsements: []*prototk.AttestationResult{
				{
					Verifier:    &prototk.ResolvedVerifier{Lookup: "verifier1"},
					Constraints: []prototk.AttestationResult_AttestationConstraint{},
				},
			},
		}).
		SubmitterSelection(prototk.ContractConfig_SUBMITTER_COORDINATOR).
		Build()

	txn.updateSigningIdentity()

	assert.Empty(t, txn.pt.Signer)
}

func Test_updateSigningIdentity_NonCoordinatorSubmitter(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{
			Endorsements: []*prototk.AttestationResult{
				{
					Verifier: &prototk.ResolvedVerifier{
						Lookup: "verifier1",
					},
					Constraints: []prototk.AttestationResult_AttestationConstraint{
						prototk.AttestationResult_ENDORSER_MUST_SUBMIT,
					},
				},
			},
		}).
		SubmitterSelection(999). // Invalid value to test the condition
		Build()

	txn.updateSigningIdentity()

	assert.Empty(t, txn.pt.Signer)
}

func Test_hasDependenciesNotReady_NoDependencies(t *testing.T) {
	ctx := context.Background()

	mockGrapher := grapher.NewMockGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Dependencies(&pldapi.TransactionDependencies{}).
		Build()

	mockGrapher.EXPECT().GetDependencies(mock.Anything, txn.pt.ID).Return(nil)

	assert.False(t, txn.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DependencyNotInMemory(t *testing.T) {
	ctx := context.Background()

	missingID := uuid.New()
	mockGrapher := grapher.NewMockGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{missingID},
		}).
		Build()

	mockGrapher.EXPECT().GetDependencies(mock.Anything, txn.pt.ID).Return([]uuid.UUID{missingID})

	// Coordinator has no transaction for missingID — treated as blocking / error path
	assert.True(t, txn.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DependencyNotReady(t *testing.T) {
	ctx := context.Background()

	mockGrapher := grapher.NewMockGrapher(t)
	txn1, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(mockGrapher).Build()
	txn1.stateMachine.CurrentState = State_Assembling

	txn2, _ := NewTransactionBuilderForTesting(t, State_Initial).Grapher(grapher).Build()
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.pt.ID},
	}
	txn2.pt.PreAssembly = nil

	assert.True(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DependencyReady(t *testing.T) {
	ctx := context.Background()

	mockGrapher := grapher.NewMockGrapher(t)
	txn1, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Grapher(mockGrapher).
		Build()

	txn2, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.pt.ID},
		}).
		Build()

	mockGrapher.EXPECT().GetDependencies(mock.Anything, txn2.pt.ID).Return([]uuid.UUID{txn1.pt.ID})

	txByID := map[uuid.UUID]CoordinatorTransaction{
		txn1.pt.ID: txn1,
		txn2.pt.ID: txn2,
	}
	lookup := func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
		return txByID[id]
	}
	txn1.getCoordinatorTransaction = lookup
	txn2.getCoordinatorTransaction = lookup

	assert.False(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_traceDispatch_WithPostAssembly(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
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

	// Should not panic
	txn.traceDispatch(ctx)
}

func Test_notifyDependentsOfReadiness_NoDependents(t *testing.T) {
	ctx := context.Background()

	mockGrapher := grapher.NewMockGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{},
		}).
		Build()

	mockGrapher.EXPECT().GetDependents(mock.Anything, txn.pt.ID).Return(nil)

	err := txn.notifyDependentsOfReadiness(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfReadiness_DependentNotInMemory(t *testing.T) {
	ctx := context.Background()
	missingID := uuid.New()
	mockGrapher := grapher.NewMockGrapher(t)
	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{missingID},
		}).
		Build()

	mockGrapher.EXPECT().GetDependents(mock.Anything, txn.pt.ID).Return([]uuid.UUID{missingID})

	err := txn.notifyDependentsOfReadiness(ctx)
	require.ErrorContains(t, err, "PD012645")
}

func Test_notifyDependentsOfReadiness_DependentInMemory(t *testing.T) {
	ctx := context.Background()

	mockGrapher := grapher.NewMockGrapher(t)
	tx1ID := uuid.New()
	tx2ID := uuid.New()
	// txn1 is the notifier: it enters Ready_For_Dispatch and notifies its dependents (txn2)
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		TransactionID(tx1ID).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{tx2ID},
		}).
		Build()
	// txn2 is the dependent: it must be in State_Blocked so that Event_DependencyReady causes a transition to State_Confirming_Dispatchable
	txn2, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		TransactionID(tx2ID).
		Grapher(grapher).
		PredefinedDependencies(tx1ID).
		Dependencies(&pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{tx1ID},
		}).
		Build()

	mockGrapher.EXPECT().GetDependents(mock.Anything, txn1.pt.ID).Return([]uuid.UUID{txn2.pt.ID}).Once()
	mockGrapher.EXPECT().GetDependencies(mock.Anything, txn2.pt.ID).Return(nil).Once()

	txByID := map[uuid.UUID]CoordinatorTransaction{
		txn1.pt.ID: txn1,
		txn2.pt.ID: txn2,
	}
	lookup := func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
		return txByID[id]
	}
	txn1.getCoordinatorTransaction = lookup
	txn2.getCoordinatorTransaction = lookup

	err := txn1.notifyDependentsOfReadiness(ctx)
	require.NoError(t, err)
	assert.Equal(t, State_Confirming_Dispatchable, txn2.stateMachine.CurrentState,
		"DependencyReadyEvent should transition txn2 from State_Blocked to State_Confirming_Dispatchable")
}

func Test_notifyDependentsOfReadiness_WithTraceEnabled(t *testing.T) {
	ctx := context.Background()

	// Enable trace logging to cover the traceDispatch path
	log.EnsureInit()
	originalLevel := log.GetLevel()
	log.SetLevel("trace")
	defer log.SetLevel(originalLevel)

	mockGrapher := grapher.NewMockGrapher(t)
	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(mockGrapher).
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

	mockGrapher.EXPECT().GetDependents(mock.Anything, txn1.pt.ID).Return(nil)

	err := txn1.notifyDependentsOfReadiness(ctx)
	require.NoError(t, err)
}

func Test_notifyDependentsOfReadiness_DependentHandleEventError(t *testing.T) {
	ctx := context.Background()

	mockGrapher := grapher.NewMockGrapher(t)

	// Dependent in State_Blocked: DependencyReady transitions to Confirming_Dispatchable and runs sendPreDispatchRequest,
	// which fails to hash when PostAssembly is nil (attestation guard still allows the transition).
	dependentID := uuid.New()
	mainTxnID := uuid.New()

	dependentTxn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(mockGrapher).
		TransactionID(dependentID).
		Build()

	dependentTxn.pt.PostAssembly = nil

	txn1, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		Dependencies(&pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{dependentID},
		}).
		PreAssembly(&components.TransactionPreAssembly{}).
		Build()

	mockGrapher.EXPECT().GetDependents(mock.Anything, txn1.pt.ID).Return([]uuid.UUID{dependentTxn.pt.ID}).Once()
	mockGrapher.EXPECT().GetDependencies(mock.Anything, dependentTxn.pt.ID).Return(nil).Once()

	txByID := map[uuid.UUID]CoordinatorTransaction{
		txn1.pt.ID:         txn1,
		dependentTxn.pt.ID: dependentTxn,
	}
	lookup := func(_ context.Context, id uuid.UUID) CoordinatorTransaction {
		return txByID[id]
	}
	txn1.getCoordinatorTransaction = lookup
	dependentTxn.getCoordinatorTransaction = lookup

	err := txn1.notifyDependentsOfReadiness(ctx)
	assert.Error(t, err)
}

func Test_allocateSigningIdentity_WithDomainSigningIdentity(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		DomainSigningIdentity("domain-signer").
		Build()

	txn.allocateSigningIdentity(ctx)

	assert.Equal(t, "domain-signer", txn.pt.Signer)
}

func Test_allocateSigningIdentity_WithoutDomainSigningIdentity(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		CoordinatorSigningIdentity("coordinator-signer").
		Build()

	txn.allocateSigningIdentity(ctx)

	assert.Equal(t, "coordinator-signer", txn.pt.Signer)
}

func Test_action_AllocateSigningIdentity_WithExistingSigner(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Signer("existing-signer").
		Build()

	err := action_AllocateSigningIdentity(ctx, txn, nil)
	require.NoError(t, err)
	assert.Equal(t, "existing-signer", txn.pt.Signer)
}

func Test_action_AllocateSigningIdentity_WithoutSigner(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		CoordinatorSigningIdentity("coordinator-signer").
		Build()

	err := action_AllocateSigningIdentity(ctx, txn, nil)
	require.NoError(t, err)
	assert.Equal(t, "coordinator-signer", txn.pt.Signer)
}

func TestDependsOn_HasDependenciesNotReady_BlockedByDep(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()

	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	depTx.dependencies.Chained.PrereqOf = []uuid.UUID{txn.pt.ID}

	assert.True(t, txn.hasDependenciesNotReady(ctx))
}

func TestDependsOn_HasDependenciesNotReady_UnblockedWhenDepDispatched(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Grapher(grapher).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()

	txn.dependencies.Chained.DependsOn = []uuid.UUID{depTx.pt.ID}
	depTx.dependencies.Chained.PrereqOf = []uuid.UUID{txn.pt.ID}

	assert.False(t, txn.hasDependenciesNotReady(ctx))
}

func TestDependsOn_HasDependenciesNotReady_UnknownDepBlocksDispatch(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	unknownID := uuid.New()

	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		Build()
	txn.dependencies.Chained.DependsOn = []uuid.UUID{unknownID}

	assert.True(t, txn.hasDependenciesNotReady(ctx))
}

func Test_Blocked_DependencyReady_TransitionsToConfirmingDispatchable(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).
		Build()
	txn.dependencies.PostAssemble.DependsOn = []uuid.UUID{depTx.pt.ID}

	err := txn.HandleEvent(ctx, &DependencyReadyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState())
}

func Test_Blocked_DependencyReady_StaysBlocked_WhenDepsNotReady(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		Build()

	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).
		Build()
	txn.dependencies.PostAssemble.DependsOn = []uuid.UUID{depTx.pt.ID}

	err := txn.HandleEvent(ctx, &DependencyReadyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Blocked, txn.GetCurrentState())
}

func TestDependsOn_NotifyDependentsOfReadiness(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	depTx, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Grapher(grapher).
		Build()

	dependentTx, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).
		Build()

	depTx.dependencies.Chained.PrereqOf = []uuid.UUID{dependentTx.pt.ID}

	err := depTx.notifyDependentsOfReadiness(ctx)
	require.NoError(t, err)
}
