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

// Test_updateSigningIdentity tests the updateSigningIdentity function
func Test_updateSigningIdentity(t *testing.T) {
	t.Run("NoPostAssembly", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.PostAssembly = nil
		txn.submitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
		txn.Signer = ""
		txn.dynamicSigningIdentity = true

		txn.updateSigningIdentity()

		assert.Empty(t, txn.Signer)
		assert.True(t, txn.dynamicSigningIdentity)
	})

	t.Run("NoEndorsements", func(t *testing.T) {
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
	})

	t.Run("EndorsementWithConstraint", func(t *testing.T) {
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
	})

	t.Run("EndorsementWithoutConstraint", func(t *testing.T) {
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
	})

	t.Run("NonCoordinatorSubmitter", func(t *testing.T) {
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
	})
}

// Test_isNotReady tests the isNotReady function
func Test_isNotReady(t *testing.T) {
	t.Run("FixedSigningIdentity_Confirmed", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.dynamicSigningIdentity = false
		txn.stateMachine.currentState = State_Confirmed

		assert.False(t, txn.isNotReady())
	})

	t.Run("FixedSigningIdentity_Submitted", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.dynamicSigningIdentity = false
		txn.stateMachine.currentState = State_Submitted

		assert.False(t, txn.isNotReady())
	})

	t.Run("FixedSigningIdentity_Dispatched", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.dynamicSigningIdentity = false
		txn.stateMachine.currentState = State_Dispatched

		assert.False(t, txn.isNotReady())
	})

	t.Run("FixedSigningIdentity_ReadyForDispatch", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.dynamicSigningIdentity = false
		txn.stateMachine.currentState = State_Ready_For_Dispatch

		assert.False(t, txn.isNotReady())
	})

	t.Run("FixedSigningIdentity_NotReady", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.dynamicSigningIdentity = false
		txn.stateMachine.currentState = State_Assembling

		assert.True(t, txn.isNotReady())
	})

	t.Run("DynamicSigningIdentity_Confirmed", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.dynamicSigningIdentity = true
		txn.stateMachine.currentState = State_Confirmed

		assert.False(t, txn.isNotReady())
	})

	t.Run("DynamicSigningIdentity_NotReady", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.dynamicSigningIdentity = true
		txn.stateMachine.currentState = State_Ready_For_Dispatch

		assert.True(t, txn.isNotReady())
	})
}

// Test_hasDependenciesNotReady tests the hasDependenciesNotReady function
func Test_hasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	t.Run("NoDependencies", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.dependencies = &pldapi.TransactionDependencies{}
		txn.previousTransaction = nil
		txn.PreAssembly = nil

		assert.False(t, txn.hasDependenciesNotReady(ctx))
	})

	t.Run("DynamicSigningIdentity_UpdatesSigner", func(t *testing.T) {
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
	})

	t.Run("PreviousTransactionNotReady", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn1, _ := newTransactionForUnitTesting(t, grapher)
		txn1.stateMachine.currentState = State_Assembling
		txn1.dynamicSigningIdentity = false

		txn2, _ := newTransactionForUnitTesting(t, grapher)
		txn2.previousTransaction = txn1
		txn2.dependencies = &pldapi.TransactionDependencies{}

		assert.True(t, txn2.hasDependenciesNotReady(ctx))
	})

	t.Run("PreviousTransactionReady", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn1, _ := newTransactionForUnitTesting(t, grapher)
		txn1.stateMachine.currentState = State_Confirmed
		txn1.dynamicSigningIdentity = false

		txn2, _ := newTransactionForUnitTesting(t, grapher)
		txn2.previousTransaction = txn1
		txn2.dependencies = &pldapi.TransactionDependencies{}

		assert.False(t, txn2.hasDependenciesNotReady(ctx))
	})

	t.Run("DependencyNotInMemory", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn, _ := newTransactionForUnitTesting(t, grapher)
		missingID := uuid.New()
		txn.dependencies = &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{missingID},
		}
		txn.previousTransaction = nil
		txn.PreAssembly = nil

		assert.False(t, txn.hasDependenciesNotReady(ctx))
	})

	t.Run("DependencyNotReady", func(t *testing.T) {
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
	})

	t.Run("DependencyReady", func(t *testing.T) {
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
	})

	t.Run("PreAssemblyDependencies", func(t *testing.T) {
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
	})

	t.Run("BothDependenciesAndPreAssemblyDependencies", func(t *testing.T) {
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
	})
}

// Test_traceDispatch tests the traceDispatch function
func Test_traceDispatch(t *testing.T) {
	ctx := context.Background()

	t.Run("WithPostAssembly", func(t *testing.T) {
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
	})
}

// Test_notifyDependentsOfReadinessAndQueueForDispatch tests the notifyDependentsOfReadinessAndQueueForDispatch function
func Test_notifyDependentsOfReadinessAndQueueForDispatch(t *testing.T) {
	ctx := context.Background()

	t.Run("NoDependents", func(t *testing.T) {
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
	})

	t.Run("DependentNotInMemory", func(t *testing.T) {
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
	})

	t.Run("DependentInMemory", func(t *testing.T) {
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
	})

	t.Run("WithTraceEnabled", func(t *testing.T) {
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
	})

	t.Run("DependentHandleEventError", func(t *testing.T) {
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
	})

	t.Run("DependentHandleEventError_ErrorPath", func(t *testing.T) {
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
		// This tests the error handling in lines 149-150
		err := txn1.notifyDependentsOfReadinessAndQueueForDispatch(ctx)
		assert.Error(t, err)
		assert.True(t, onReadyCalled) // onReadyForDispatch should still be called before the error
	})
}

// Test_notifyDependentsOfConfirmationAndQueueForDispatch tests the notifyDependentsOfConfirmationAndQueueForDispatch function
func Test_notifyDependentsOfConfirmationAndQueueForDispatch(t *testing.T) {
	ctx := context.Background()

	t.Run("NoDependents", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.dependencies = &pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{},
		}

		err := txn.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
		assert.NoError(t, err)
	})

	t.Run("DependentNotInMemory", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn, _ := newTransactionForUnitTesting(t, grapher)
		missingID := uuid.New()
		txn.dependencies = &pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{missingID},
		}

		err := txn.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
		assert.NoError(t, err)
	})

	t.Run("DependentInMemory", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn1, _ := newTransactionForUnitTesting(t, grapher)
		txn2, _ := newTransactionForUnitTesting(t, grapher)
		txn1.dependencies = &pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{txn2.ID},
		}

		err := txn1.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
		assert.NoError(t, err)
	})

	t.Run("WithTraceEnabled", func(t *testing.T) {
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
	})

	t.Run("DependentHandleEventError", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn1, _ := newTransactionForUnitTesting(t, grapher)
		txn2, _ := newTransactionForUnitTesting(t, grapher)
		txn1.dependencies = &pldapi.TransactionDependencies{
			PrereqOf: []uuid.UUID{txn2.ID},
		}

		// The function should complete without error in normal cases
		err := txn1.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
		assert.NoError(t, err)
	})

	t.Run("DependentHandleEventError_ErrorPath", func(t *testing.T) {
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
		// This tests the error handling in lines 177-178
		err := txn1.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
		assert.Error(t, err)
	})
}

// Test_allocateSigningIdentity tests the allocateSigningIdentity function
func Test_allocateSigningIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("WithDomainSigningIdentity", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.domainSigningIdentity = "domain-signer"
		txn.Signer = ""
		txn.dynamicSigningIdentity = true

		txn.allocateSigningIdentity(ctx)

		assert.Equal(t, "domain-signer", txn.Signer)
		assert.False(t, txn.dynamicSigningIdentity)
	})

	t.Run("WithoutDomainSigningIdentity", func(t *testing.T) {
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
	})
}

// Test_action_NotifyDependentsOfReadiness tests the action_NotifyDependentsOfReadiness function
func Test_action_NotifyDependentsOfReadiness(t *testing.T) {
	ctx := context.Background()

	t.Run("WithExistingSigner", func(t *testing.T) {
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
	})

	t.Run("WithoutSigner", func(t *testing.T) {
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
	})
}

// Test_hasDependenciesNotIn tests the hasDependenciesNotIn function
func Test_hasDependenciesNotIn(t *testing.T) {
	ctx := context.Background()

	t.Run("NoDependencies", func(t *testing.T) {
		txn, _ := newTransactionForUnitTesting(t, nil)
		txn.previousTransaction = nil
		txn.dependencies = &pldapi.TransactionDependencies{}
		txn.PreAssembly = &components.TransactionPreAssembly{}

		assert.False(t, txn.hasDependenciesNotIn(ctx, []*Transaction{}))
	})

	t.Run("PreviousTransactionNotInIgnoreList", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn1, _ := newTransactionForUnitTesting(t, grapher)
		txn2, _ := newTransactionForUnitTesting(t, grapher)
		txn2.previousTransaction = txn1
		txn2.dependencies = &pldapi.TransactionDependencies{}
		txn2.PreAssembly = &components.TransactionPreAssembly{}

		assert.True(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{}))
	})

	t.Run("PreviousTransactionInIgnoreList", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn1, _ := newTransactionForUnitTesting(t, grapher)
		txn2, _ := newTransactionForUnitTesting(t, grapher)
		txn2.previousTransaction = txn1
		txn2.dependencies = &pldapi.TransactionDependencies{}
		txn2.PreAssembly = &components.TransactionPreAssembly{}

		assert.False(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{txn1}))
	})

	t.Run("DependencyNotInMemory", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn, _ := newTransactionForUnitTesting(t, grapher)
		missingID := uuid.New()
		txn.dependencies = &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{missingID},
		}
		txn.previousTransaction = nil
		txn.PreAssembly = &components.TransactionPreAssembly{}

		assert.False(t, txn.hasDependenciesNotIn(ctx, []*Transaction{}))
	})

	t.Run("DependencyNotInIgnoreList", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn1, _ := newTransactionForUnitTesting(t, grapher)
		txn2, _ := newTransactionForUnitTesting(t, grapher)
		txn2.dependencies = &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.ID},
		}
		txn2.previousTransaction = nil
		txn2.PreAssembly = &components.TransactionPreAssembly{}

		assert.True(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{}))
	})

	t.Run("DependencyInIgnoreList", func(t *testing.T) {
		grapher := NewGrapher(ctx)
		txn1, _ := newTransactionForUnitTesting(t, grapher)
		txn2, _ := newTransactionForUnitTesting(t, grapher)
		txn2.dependencies = &pldapi.TransactionDependencies{
			DependsOn: []uuid.UUID{txn1.ID},
		}
		txn2.previousTransaction = nil
		txn2.PreAssembly = &components.TransactionPreAssembly{}

		assert.False(t, txn2.hasDependenciesNotIn(ctx, []*Transaction{txn1}))
	})

	t.Run("PreAssemblyDependencyNotInIgnoreList", func(t *testing.T) {
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
	})

	t.Run("PreAssemblyDependencyInIgnoreList", func(t *testing.T) {
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
	})

	t.Run("MultipleDependencies_OneInIgnoreList", func(t *testing.T) {
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
	})

	t.Run("MultipleDependencies_AllInIgnoreList", func(t *testing.T) {
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
	})
}
