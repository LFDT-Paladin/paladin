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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGuard_HasRevertReason_FalseWhenEmpty(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Initially revertReason should be nil (zero value for HexBytes)
	// When nil, String() returns "", so guard returns false
	assert.False(t, guard_HasRevertReason(ctx, txn))

	// Note: An empty slice HexBytes{} would return "0x" from String(),
	// which is not empty, so the guard would return true. Only nil returns false.
}

func TestGuard_HasRevertReason_TrueWhenSet(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set revertReason to a non-empty value
	txn.revertReason = pldtypes.MustParseHexBytes("0x1234567890abcdef")
	assert.True(t, guard_HasRevertReason(ctx, txn))

	// Test with another value
	txn.revertReason = pldtypes.MustParseHexBytes("0xdeadbeef")
	assert.True(t, guard_HasRevertReason(ctx, txn))
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
		PrereqOf: []uuid.UUID{txn2.pt.ID},
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

	err := txn1.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfConfirmationAndQueueForDispatch_DependentHandleEventError(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{txn2.pt.ID},
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

	// Call notifyDependentsOfConfirmationAndQueueForDispatch - should return error
	err := txn1.notifyDependentsOfConfirmationAndQueueForDispatch(ctx)
	assert.Error(t, err)
}
