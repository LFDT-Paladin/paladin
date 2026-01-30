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
)

func Test_updateSigningIdentity_NoPostAssembly(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.PostAssembly = nil
	txn.submitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
	txn.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Empty(t, txn.Signer)
	assert.True(t, txn.dynamicSigningIdentity)
}

func Test_updateSigningIdentity_NoEndorsements(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.PostAssembly = &components.TransactionPostAssembly{
		Endorsements: []*prototk.AttestationResult{},
	}
	txn.submitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
	txn.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Empty(t, txn.Signer)
	assert.True(t, txn.dynamicSigningIdentity)
}

func Test_updateSigningIdentity_EndorsementWithConstraint(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	verifierLookup := "verifier1"
	txn.PostAssembly = &components.TransactionPostAssembly{
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
	txn.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Equal(t, verifierLookup, txn.Signer)
	assert.False(t, txn.dynamicSigningIdentity)
}

func Test_updateSigningIdentity_EndorsementWithoutConstraint(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.PostAssembly = &components.TransactionPostAssembly{
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
	txn.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Empty(t, txn.Signer)
	assert.True(t, txn.dynamicSigningIdentity)
}

func Test_updateSigningIdentity_NonCoordinatorSubmitter(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.PostAssembly = &components.TransactionPostAssembly{
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
	txn.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.updateSigningIdentity()

	assert.Empty(t, txn.Signer)
	assert.True(t, txn.dynamicSigningIdentity)
}

func Test_isNotReady_FixedSigningIdentity_Confirmed(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.currentState = State_Confirmed

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_FixedSigningIdentity_Submitted(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.currentState = State_Submitted

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_FixedSigningIdentity_Dispatched(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.currentState = State_Dispatched

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_FixedSigningIdentity_ReadyForDispatch(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.currentState = State_Ready_For_Dispatch

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_FixedSigningIdentity_NotReady(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = false
	txn.stateMachine.currentState = State_Assembling

	assert.True(t, txn.isNotReady())
}

func Test_isNotReady_DynamicSigningIdentity_Confirmed(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = true
	txn.stateMachine.currentState = State_Confirmed

	assert.False(t, txn.isNotReady())
}

func Test_isNotReady_DynamicSigningIdentity_NotReady(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dynamicSigningIdentity = true
	txn.stateMachine.currentState = State_Ready_For_Dispatch

	assert.True(t, txn.isNotReady())
}

func Test_hasDependenciesNotReady_NoDependencies(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dependencies = &pldapi.TransactionDependencies{}
	txn.previousTransaction = nil
	txn.PreAssembly = nil

	assert.False(t, txn.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DynamicSigningIdentity_UpdatesSigner(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)
	txn.dynamicSigningIdentity = true
	txn.PostAssembly = &components.TransactionPostAssembly{
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
	txn.submitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
	txn.dependencies = &pldapi.TransactionDependencies{}
	txn.previousTransaction = nil
	txn.PreAssembly = nil

	txn.hasDependenciesNotReady(ctx)

	assert.Equal(t, "verifier1", txn.Signer)
	assert.False(t, txn.dynamicSigningIdentity)
}

func Test_hasDependenciesNotReady_PreviousTransactionNotReady(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.currentState = State_Assembling
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.previousTransaction = txn1
	txn2.dependencies = &pldapi.TransactionDependencies{}

	assert.True(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_PreviousTransactionReady(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.currentState = State_Confirmed
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.previousTransaction = txn1
	txn2.dependencies = &pldapi.TransactionDependencies{}

	assert.False(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DependencyNotInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)
	missingID := uuid.New()
	txn.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{missingID},
	}
	txn.previousTransaction = nil
	txn.PreAssembly = nil

	assert.False(t, txn.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DependencyNotReady(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.currentState = State_Assembling
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.ID},
	}
	txn2.previousTransaction = nil
	txn2.PreAssembly = nil

	assert.True(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_DependencyReady(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.currentState = State_Confirmed
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.ID},
	}
	txn2.previousTransaction = nil
	txn2.PreAssembly = nil

	assert.False(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_PreAssemblyDependencies(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.currentState = State_Assembling
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{}
	txn2.PreAssembly = &components.TransactionPreAssembly{
		Dependencies: &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.ID},
		},
	}
	txn2.previousTransaction = nil

	assert.True(t, txn2.hasDependenciesNotReady(ctx))
}

func Test_hasDependenciesNotReady_BothDependenciesAndPreAssemblyDependencies(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.stateMachine.currentState = State_Confirmed
	txn1.dynamicSigningIdentity = false

	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.stateMachine.currentState = State_Assembling
	txn2.dynamicSigningIdentity = false

	txn3, _ := newTransactionForUnitTesting(t, grapher)
	txn3.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.ID},
	}
	txn3.PreAssembly = &components.TransactionPreAssembly{
		Dependencies: &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn2.ID},
		},
	}
	txn3.previousTransaction = nil

	assert.True(t, txn3.hasDependenciesNotReady(ctx))
}

func Test_traceDispatch_WithPostAssembly(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.PostAssembly = &components.TransactionPostAssembly{
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

func Test_notifyDependentsOfReadinessAndQueueForDispatch_NoDependents(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}
	onReadyCalled := false
	txn.onReadyForDispatch = func(ctx context.Context, t *Transaction) {
		onReadyCalled = true
	}

	err := txn.notifyDependentsOfReadinessAndQueueForDispatch(ctx)
	assert.NoError(t, err)
	assert.True(t, onReadyCalled)
}

func Test_notifyDependentsOfReadinessAndQueueForDispatch_DependentNotInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)
	missingID := uuid.New()
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{missingID},
	}
	onReadyCalled := false
	txn.onReadyForDispatch = func(ctx context.Context, t *Transaction) {
		onReadyCalled = true
	}

	err := txn.notifyDependentsOfReadinessAndQueueForDispatch(ctx)
	assert.NoError(t, err)
	assert.True(t, onReadyCalled)
}

func Test_notifyDependentsOfReadinessAndQueueForDispatch_DependentInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{txn2.ID},
	}
	onReadyCalled := false
	txn1.onReadyForDispatch = func(ctx context.Context, t *Transaction) {
		onReadyCalled = true
	}

	err := txn1.notifyDependentsOfReadinessAndQueueForDispatch(ctx)
	assert.NoError(t, err)
	assert.True(t, onReadyCalled)
}

func Test_notifyDependentsOfReadinessAndQueueForDispatch_WithTraceEnabled(t *testing.T) {
	ctx := context.Background()

	// Enable trace logging to cover the traceDispatch path
	log.EnsureInit()
	originalLevel := log.GetLevel()
	log.SetLevel("trace")
	defer log.SetLevel(originalLevel)

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.PostAssembly = &components.TransactionPostAssembly{
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
	onReadyCalled := false
	txn1.onReadyForDispatch = func(ctx context.Context, t *Transaction) {
		onReadyCalled = true
	}

	err := txn1.notifyDependentsOfReadinessAndQueueForDispatch(ctx)
	assert.NoError(t, err)
	assert.True(t, onReadyCalled)
}

func Test_notifyDependentsOfReadinessAndQueueForDispatch_DependentHandleEventError(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}
	onReadyCalled := false
	txn1.onReadyForDispatch = func(ctx context.Context, t *Transaction) {
		onReadyCalled = true
	}

	// The function should complete without error in normal cases
	// If we need to test error handling, we'd need to mock HandleEvent to return an error
	err := txn1.notifyDependentsOfReadinessAndQueueForDispatch(ctx)
	assert.NoError(t, err)
	assert.True(t, onReadyCalled)
}

func Test_notifyDependentsOfReadinessAndQueueForDispatch_DependentHandleEventError_ErrorPath(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	// Create the main transaction that will notify dependents
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	onReadyCalled := false
	txn1.onReadyForDispatch = func(ctx context.Context, t *Transaction) {
		onReadyCalled = true
	}

	// Create a dependent transaction in State_Blocked that will fail when handling DependencyReadyEvent
	// This happens when transitioning to State_Confirming_Dispatchable triggers action_SendPreDispatchRequest
	// which calls Hash(), which fails if PostAssembly is nil
	dependentTxnBuilder := NewTransactionBuilderForTesting(t, State_Blocked).
		Grapher(grapher)
	dependentTxn := dependentTxnBuilder.Build()
	dependentID := dependentTxn.ID

	// Remove PostAssembly to cause Hash() to fail when transitioning to State_Confirming_Dispatchable
	// Note: guard_AttestationPlanFulfilled returns true when PostAssembly is nil (no unfulfilled requirements)
	// so the transition will be attempted, but action_SendPreDispatchRequest will fail
	dependentTxn.PostAssembly = nil

	// Ensure the dependent transaction can transition (no dependencies not ready)
	// The guard requires: guard_And(guard_AttestationPlanFulfilled, guard_Not(guard_HasDependenciesNotReady))
	dependentTxn.dependencies = &pldapi.TransactionDependencies{}
	dependentTxn.previousTransaction = nil
	if dependentTxn.PreAssembly == nil {
		dependentTxn.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Set up the main transaction to have the dependent as a PrereqOf
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{dependentID},
	}

	// Call notifyDependentsOfReadinessAndQueueForDispatch - should return error
	err := txn1.notifyDependentsOfReadinessAndQueueForDispatch(ctx)
	assert.Error(t, err)
	assert.True(t, onReadyCalled) // onReadyForDispatch should still be called before the error
}

func Test_notifyDependentsOfConfirmationAndQueueForDispatch_NoDependents(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}

	err := txn.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfConfirmationAndQueueForDispatch_DependentNotInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)
	missingID := uuid.New()
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{missingID},
	}

	err := txn.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfConfirmationAndQueueForDispatch_DependentInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{txn2.ID},
	}

	err := txn1.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfConfirmationAndQueueForDispatch_WithTraceEnabled(t *testing.T) {
	ctx := context.Background()

	// Enable trace logging to cover the traceDispatch path
	log.EnsureInit()
	originalLevel := log.GetLevel()
	log.SetLevel("trace")
	defer log.SetLevel(originalLevel)

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn1.PostAssembly = &components.TransactionPostAssembly{
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

	err := txn1.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfConfirmationAndQueueForDispatch_DependentHandleEventError(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{txn2.ID},
	}

	// The function should complete without error in normal cases
	err := txn1.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfConfirmationAndQueueForDispatch_DependentHandleEventError_ErrorPath(t *testing.T) {
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
	dependentID := dependentTxn.ID

	// Remove PostAssembly to cause Hash() to fail when transitioning to State_Confirming_Dispatchable
	// Note: guard_AttestationPlanFulfilled returns true when PostAssembly is nil (no unfulfilled requirements)
	// so the transition will be attempted, but action_SendPreDispatchRequest will fail
	dependentTxn.PostAssembly = nil

	// Ensure the dependent transaction can transition (no dependencies not ready)
	// The guard requires: guard_And(guard_AttestationPlanFulfilled, guard_Not(guard_HasDependenciesNotReady))
	dependentTxn.dependencies = &pldapi.TransactionDependencies{}
	dependentTxn.previousTransaction = nil
	if dependentTxn.PreAssembly == nil {
		dependentTxn.PreAssembly = &components.TransactionPreAssembly{}
	}

	// Set up the main transaction to have the dependent as a PrereqOf
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{dependentID},
	}

	// Call notifyDependentsOfConfirmationAndQueueForDispatch - should return error
	err := txn1.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
	assert.Error(t, err)
}

func Test_allocateSigningIdentity_WithDomainSigningIdentity(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.domainSigningIdentity = "domain-signer"
	txn.Signer = ""
	txn.dynamicSigningIdentity = true

	txn.allocateSigningIdentity(ctx)

	assert.Equal(t, "domain-signer", txn.Signer)
	assert.False(t, txn.dynamicSigningIdentity)
}

func Test_allocateSigningIdentity_WithoutDomainSigningIdentity(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.domainSigningIdentity = ""
	txn.Signer = ""
	addr := pldtypes.RandAddress()
	txn.Address = *addr

	txn.allocateSigningIdentity(ctx)

	assert.NotEmpty(t, txn.Signer)
	assert.Contains(t, txn.Signer, txn.Address.String())
	assert.Contains(t, txn.Signer, "domains.")
	assert.Contains(t, txn.Signer, ".submit.")
}

func Test_action_NotifyDependentsOfReadiness_WithExistingSigner(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.Signer = "existing-signer"
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}
	onReadyCalled := false
	txn.onReadyForDispatch = func(ctx context.Context, t *Transaction) {
		onReadyCalled = true
	}

	err := action_NotifyDependentsOfReadiness(ctx, txn)
	assert.NoError(t, err)
	assert.Equal(t, "existing-signer", txn.Signer)
	assert.True(t, onReadyCalled)
}

func Test_action_NotifyDependentsOfReadiness_WithoutSigner(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.Signer = ""
	txn.domainSigningIdentity = ""
	addr := pldtypes.RandAddress()
	txn.Address = *addr
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}
	onReadyCalled := false
	txn.onReadyForDispatch = func(ctx context.Context, t *Transaction) {
		onReadyCalled = true
	}

	err := action_NotifyDependentsOfReadiness(ctx, txn)
	assert.NoError(t, err)
	assert.NotEmpty(t, txn.Signer)
	assert.True(t, onReadyCalled)
}

func Test_hasDependenciesNotIn_NoDependencies(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.previousTransaction = nil
	txn.dependencies = &pldapi.TransactionDependencies{}
	txn.PreAssembly = &components.TransactionPreAssembly{}

	assert.False(t, txn.hasDependenciesNotIn(ctx, []*Transaction{}))
}

func Test_hasDependenciesNotIn_PreviousTransactionNotInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.previousTransaction = txn1
	txn2.dependencies = &pldapi.TransactionDependencies{}
	txn2.PreAssembly = &components.TransactionPreAssembly{}

	assert.True(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{}))
}

func Test_hasDependenciesNotIn_PreviousTransactionInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.previousTransaction = txn1
	txn2.dependencies = &pldapi.TransactionDependencies{}
	txn2.PreAssembly = &components.TransactionPreAssembly{}

	assert.False(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{txn1}))
}

func Test_hasDependenciesNotIn_DependencyNotInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)
	missingID := uuid.New()
	txn.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{missingID},
	}
	txn.previousTransaction = nil
	txn.PreAssembly = &components.TransactionPreAssembly{}

	assert.False(t, txn.hasDependenciesNotIn(ctx, []*Transaction{}))
}

func Test_hasDependenciesNotIn_DependencyNotInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.ID},
	}
	txn2.previousTransaction = nil
	txn2.PreAssembly = &components.TransactionPreAssembly{}

	assert.True(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{}))
}

func Test_hasDependenciesNotIn_DependencyInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.ID},
	}
	txn2.previousTransaction = nil
	txn2.PreAssembly = &components.TransactionPreAssembly{}

	assert.False(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{txn1}))
}

func Test_hasDependenciesNotIn_PreAssemblyDependencyNotInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{}
	txn2.PreAssembly = &components.TransactionPreAssembly{
		Dependencies: &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.ID},
		},
	}
	txn2.previousTransaction = nil

	assert.True(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{}))
}

func Test_hasDependenciesNotIn_PreAssemblyDependencyInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn2.dependencies = &pldapi.TransactionDependencies{}
	txn2.PreAssembly = &components.TransactionPreAssembly{
		Dependencies: &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.ID},
		},
	}
	txn2.previousTransaction = nil

	assert.False(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{txn1}))
}

func Test_hasDependenciesNotIn_MultipleDependencies_OneInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn3, _ := newTransactionForUnitTesting(t, grapher)
	txn3.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.ID, txn2.ID},
	}
	txn3.previousTransaction = nil
	txn3.PreAssembly = &components.TransactionPreAssembly{}

	assert.True(t, txn3.hasDependenciesNotIn(ctx, []*Transaction{txn1}))
}

func Test_hasDependenciesNotIn_MultipleDependencies_AllInIgnoreList(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn3, _ := newTransactionForUnitTesting(t, grapher)
	txn3.dependencies = &pldapi.TransactionDependencies{
		DependsOn: []uuid.UUID{txn1.ID, txn2.ID},
	}
	txn3.previousTransaction = nil
	txn3.PreAssembly = &components.TransactionPreAssembly{}

	assert.False(t, txn3.hasDependenciesNotIn(ctx, []*Transaction{txn1, txn2}))
}
