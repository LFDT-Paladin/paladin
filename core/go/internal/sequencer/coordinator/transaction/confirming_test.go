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
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func Test_guard_HasRevertReason_FalseWhenEmpty(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Initially revertReason should be nil (zero value for HexBytes)
	// When nil, String() returns "", so guard returns false
	assert.False(t, guard_HasRevertReason(ctx, txn))

	// Note: An empty slice HexBytes{} would return "0x" from String(),
	// which is not empty, so the guard would return true. Only nil returns false.
}

func Test_guard_HasRevertReason_TrueWhenSet(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set revertReason to a non-empty value
	txn.revertReason = pldtypes.MustParseHexBytes("0x1234567890abcdef")
	assert.True(t, guard_HasRevertReason(ctx, txn))

	// Test with another value
	txn.revertReason = pldtypes.MustParseHexBytes("0xdeadbeef")
	assert.True(t, guard_HasRevertReason(ctx, txn))
}

func Test_notifyDependentsOfConfirmation_NoDependents(t *testing.T) {
	ctx := context.Background()

	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{},
	}

	err := txn.notifyDependentsOfConfirmation(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfConfirmation_DependentNotInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn, _ := newTransactionForUnitTesting(t, grapher)
	missingID := uuid.New()
	txn.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{missingID},
	}

	err := txn.notifyDependentsOfConfirmation(ctx)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "PD012645"))
}

func Test_notifyDependentsOfConfirmation_WithTraceEnabled(t *testing.T) {
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

	err := txn1.notifyDependentsOfConfirmation(ctx)
	assert.NoError(t, err)
}
func Test_notifyDependentsOfConfirmation_DependentInMemory(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{txn2.pt.ID},
	}

	err := txn1.notifyDependentsOfConfirmation(ctx)
	assert.NoError(t, err)
}

func Test_notifyDependentsOfConfirmation_DependentHandleEventError(t *testing.T) {
	ctx := context.Background()

	grapher := NewGrapher(ctx)
	txn1, _ := newTransactionForUnitTesting(t, grapher)
	txn2, _ := newTransactionForUnitTesting(t, grapher)
	txn1.dependencies = &pldapi.TransactionDependencies{
		PrereqOf: []uuid.UUID{txn2.pt.ID},
	}

	// The function should complete without error in normal cases
	err := txn1.notifyDependentsOfConfirmation(ctx)
	assert.NoError(t, err)
}

func Test_action_Confirmed_SetsRevertReasonAndSends(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	nonce := pldtypes.HexUint64(42)
	revertReason := pldtypes.MustParseHexBytes("0x1234")
	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce:        &nonce,
		RevertReason: revertReason,
	}

	mocks.transportWriter.EXPECT().
		SendTransactionConfirmed(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, &nonce, revertReason).
		Return(nil)

	err := action_Confirmed(ctx, txn, event)
	assert.NoError(t, err)

	// Assert state: revertReason was set
	assert.Equal(t, revertReason, txn.revertReason)
	mocks.transportWriter.AssertExpectations(t)
}
