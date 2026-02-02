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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_action_UpdateSigningIdentity_CallsUpdateSigningIdentity(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		Endorsements: []*prototk.AttestationResult{
			{
				Verifier:    &prototk.ResolvedVerifier{Lookup: "signer1"},
				Constraints: []prototk.AttestationResult_AttestationConstraint{prototk.AttestationResult_ENDORSER_MUST_SUBMIT},
			},
		},
	}
	txn.submitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
	txn.pt.Signer = ""
	txn.dynamicSigningIdentity = true

	err := action_UpdateSigningIdentity(ctx, txn, nil)

	require.NoError(t, err)
	assert.Equal(t, "signer1", txn.pt.Signer)
	assert.False(t, txn.dynamicSigningIdentity)
}

func Test_updateSigningIdentity_NoPostAssembly(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = nil
	txn.submitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
	txn.pt.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Empty(t, txn.pt.Signer)
	assert.True(t, txn.dynamicSigningIdentity)
}

func Test_updateSigningIdentity_NoEndorsements(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		Endorsements: []*prototk.AttestationResult{},
	}
	txn.submitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
	txn.pt.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Empty(t, txn.pt.Signer)
	assert.True(t, txn.dynamicSigningIdentity)
}

func Test_updateSigningIdentity_EndorsementWithConstraint(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	verifierLookup := "verifier1"
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
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
	}
	txn.submitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
	txn.pt.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Equal(t, verifierLookup, txn.pt.Signer)
	assert.False(t, txn.dynamicSigningIdentity)
}

func Test_updateSigningIdentity_EndorsementWithoutConstraint(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		Endorsements: []*prototk.AttestationResult{
			{
				Verifier: &prototk.ResolvedVerifier{
					Lookup: "verifier1",
				},
				Constraints: []prototk.AttestationResult_AttestationConstraint{},
			},
		},
	}
	txn.submitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
	txn.pt.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Empty(t, txn.pt.Signer)
	assert.True(t, txn.dynamicSigningIdentity)
}

func Test_updateSigningIdentity_NonCoordinatorSubmitter(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
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
	}
	// Use a different submitter selection value (0 is COORDINATOR, so use 1 or higher)
	txn.submitterSelection = 999 // Invalid value to test the condition
	txn.pt.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Empty(t, txn.pt.Signer)
	assert.True(t, txn.dynamicSigningIdentity)
}

func Test_isNotReady_FixedSigningIdentity_Confirmed(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.CurrentState = State_Confirmed

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_FixedSigningIdentity_Submitted(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.CurrentState = State_Submitted

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_FixedSigningIdentity_Dispatched(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.CurrentState = State_Dispatched

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_FixedSigningIdentity_ReadyForDispatch(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.CurrentState = State_Ready_For_Dispatch

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_FixedSigningIdentity_NotReady(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.CurrentState = State_Assembling

	assert.True(t, txn.isNotReady())
}

func Test_isNotReady_DynamicSigningIdentity_Confirmed(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = true
	txn.stateMachine.CurrentState = State_Confirmed

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_DynamicSigningIdentity_NotReady(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = true
	txn.stateMachine.CurrentState = State_Ready_For_Dispatch

	assert.True(t, txn.isNotReady())
}

func Test_hasDependenciesNotReady_NoDependencies(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dependencies = &pldapi.TransactionDependencies{}
	txn.pt.PreAssembly = nil

	assert.False(t, txn.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DependencyNotInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)
	missingID := uuid.New()
	txn.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{missingID},
	}
	txn.pt.PreAssembly = nil

	assert.False(t, txn.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DependencyNotReady(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.CurrentState = State_Assembling
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.pt.ID},
	}
	txn2.pt.PreAssembly = nil

	assert.True(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DependencyReady(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.CurrentState = State_Confirmed
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.pt.ID},
	}
	txn2.pt.PreAssembly = nil

	assert.False(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_PreAssemblyDependencies(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.CurrentState = State_Assembling
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{}
	txn2.pt.PreAssembly = &components.TransactionPreAssembly{
		Dependencies: &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.pt.ID},
		},
	}

	assert.True(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_BothDependenciesAndPreAssemblyDependencies(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.CurrentState = State_Confirmed
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.stateMachine.CurrentState = State_Assembling
	txn2.dynamicSigningIdentity = false

	txn3, _ := newTransactionForUnitTesting(t, grapher)
	txn3.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.pt.ID},
	}
	txn3.pt.PreAssembly = &components.TransactionPreAssembly{
		Dependencies: &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn2.pt.ID},
		},
	}

	assert.True(t, txn3.hasDependenciesNotReady(ctx))
}

func Test_traceDispatch_WithPostAssembly(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
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
	}

	// Should not panic
	txn.traceDispatch(ctx)
}

func Test_notifyDependentsOfReadiness_NoDependents(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}

	err := txn.notifyDependentsOfReadiness(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfReadiness_DependentNotInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)
	missingID := uuid.New()
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{missingID},
	}

	err := txn.notifyDependentsOfReadiness(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfReadiness_DependentInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	// txn1 is the notifier: it enters Ready_For_Dispatch and notifies its dependents (txn2)
	txn1 := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Grapher(grapher).Build()
	// txn2 is the dependent: it must be in State_Blocked so that Event_DependencyReady causes a transition to State_Confirming_Dispatchable
	txn2 := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher).
		PredefinedDependencies(txn1.pt.ID).
		Build()
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.pt.ID},
	}
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{txn2.pt.ID},
	}

	err := txn1.notifyDependentsOfReadiness(ctx)
	assert.NoError(t, err)
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

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.pt.PostAssembly = &components.TransactionPostAssembly{
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
	}
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}

	err := txn1.notifyDependentsOfReadiness(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfReadiness_DependentHandleEventError(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	// Create the main transaction that will notify dependents
	txn1, _ := newTransactionForUnitTesting(t, grapher)

	// Create a dependent transaction in State_Blocked that will fail when handling DependencyReadyEvent
	// This happens when transitioning to State_Confirming_Dispatchable triggers action_SendPreDispatchRequest
	// which calls Hash(), which fails if PostAssembly is nil
	dependentTxnBuilder := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher)
	dependentTxn := dependentTxnBuilder.Build()
	dependentID := dependentTxn.pt.ID

	// Remove PostAssembly to cause Hash() to fail when transitioning to State_Confirming_Dispatchable
	// Note: guard_AttestationPlanFulfilled returns true when PostAssembly is nil (no unfulfilled requirements)
	// so the transition will be attempted, but action_SendPreDispatchRequest will fail
	dependentTxn.pt.PostAssembly = nil

	// Ensure the dependent transaction can transition (no dependencies not ready)
	// The guard requires: guard_And(guard_AttestationPlanFulfilled, guard_Not(guard_HasDependenciesNotReady))
	dependentTxn.dependencies = &pldapi.TransactionDependencies{}
	if dependentTxn.pt.PreAssembly == nil {
		dependentTxn.pt.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Set up the main transaction to have the dependent as a PrereqOf
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{dependentID},
	}

	// Call notifyDependentsOfReadinessAndQueueForDispatch - should return error
	err := txn1.notifyDependentsOfReadiness(ctx)
	assert.Error(t, err)
}

func Test_allocateSigningIdentity_WithDomainSigningIdentity(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.domainSigningIdentity = "domain-signer"
	txn.pt.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.allocateSigningIdentity(ctx)

	assert.Equal(t, "domain-signer", txn.pt.Signer)
	assert.False(t, txn.dynamicSigningIdentity)
}

func Test_allocateSigningIdentity_WithoutDomainSigningIdentity(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.domainSigningIdentity = ""
	txn.pt.Signer = ""
	addr := pldtypes.RandAddress()
	txn.pt.Address = *addr

	txn.allocateSigningIdentity(ctx)

	assert.NotEmpty(t, txn.pt.Signer)
	assert.Contains(t, txn.pt.Signer, txn.pt.Address.String())
	assert.Contains(t, txn.pt.Signer, "domains.")
	assert.Contains(t, txn.pt.Signer, ".submit.")
}

func Test_action_NotifyDependentsOfReadiness_WithExistingSigner(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.Signer = "existing-signer"
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}

	err := action_NotifyDependentsOfReadiness(ctx, txn, nil)
	assert.NoError(t, err)
	assert.Equal(t, "existing-signer", txn.pt.Signer)
}

func Test_action_NotifyDependentsOfReadiness_WithoutSigner(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.Signer = ""
	txn.domainSigningIdentity = ""
	addr := pldtypes.RandAddress()
	txn.pt.Address = *addr
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}

	err := action_NotifyDependentsOfReadiness(ctx, txn, nil)
	assert.NoError(t, err)
	assert.NotEmpty(t, txn.pt.Signer)
}

func Test_hasDependenciesNotIn_NoDependencies(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dependencies = &pldapi.TransactionDependencies{}
	txn.pt.PreAssembly = &components.TransactionPreAssembly{}

	assert.False(t, txn.hasDependenciesNotIn(ctx, []*CoordinatorTransaction{}))
}

func Test_hasDependenciesNotIn_DependencyNotInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)
	missingID := uuid.New()
	txn.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{missingID},
	}
	txn.pt.PreAssembly = &components.TransactionPreAssembly{}

	assert.False(t, txn.hasDependenciesNotIn(ctx, []*CoordinatorTransaction{}))
}

func Test_hasDependenciesNotIn_DependencyNotInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.pt.ID},
	}
	txn2.pt.PreAssembly = &components.TransactionPreAssembly{}

	assert.True(t, txn2.hasDependenciesNotIn(ctx, []*CoordinatorTransaction{}))
}

func Test_hasDependenciesNotIn_DependencyInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.pt.ID},
	}
	txn2.pt.PreAssembly = &components.TransactionPreAssembly{}

	assert.False(t, txn2.hasDependenciesNotIn(ctx, []*CoordinatorTransaction{txn1}))
}

func Test_hasDependenciesNotIn_PreAssemblyDependencyNotInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{}
	txn2.pt.PreAssembly = &components.TransactionPreAssembly{
		Dependencies: &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.pt.ID},
		},
	}

	assert.True(t, txn2.hasDependenciesNotIn(ctx, []*CoordinatorTransaction{}))
}

func Test_hasDependenciesNotIn_PreAssemblyDependencyInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{}
	txn2.pt.PreAssembly = &components.TransactionPreAssembly{
		Dependencies: &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.pt.ID},
		},
	}

	assert.False(t, txn2.hasDependenciesNotIn(ctx, []*CoordinatorTransaction{txn1}))
}

func Test_hasDependenciesNotIn_MultipleDependencies_OneInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn3, _ := newTransactionForUnitTesting(t, grapher)
	txn3.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.pt.ID, txn2.pt.ID},
	}
	txn3.pt.PreAssembly = &components.TransactionPreAssembly{}

	assert.True(t, txn3.hasDependenciesNotIn(ctx, []*CoordinatorTransaction{txn1}))
}

func Test_hasDependenciesNotIn_MultipleDependencies_AllInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn3, _ := newTransactionForUnitTesting(t, grapher)
	txn3.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.pt.ID, txn2.pt.ID},
	}
	txn3.pt.PreAssembly = &components.TransactionPreAssembly{}

	assert.False(t, txn3.hasDependenciesNotIn(ctx, []*CoordinatorTransaction{txn1, txn2}))
}
