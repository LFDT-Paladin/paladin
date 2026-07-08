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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func coordSignPlan(parties ...string) *components.TransactionPostAssembly {
	return &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "sig",
					AttestationType: prototk.AttestationType_SIGN,
					Parties:         parties,
				},
			},
		},
	}
}

func TestApplySignature_Append(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("alice@node1")).Build()
	sig := &prototk.AttestationResult{Name: "sig", Verifier: &prototk.ResolvedVerifier{Lookup: "alice@node1"}}
	txn.applySignature(context.Background(), sig)
	assert.Len(t, txn.pt.PostAssembly.AssembleResponse.GetSignatures(), 1)
}

func TestApplySignature_IgnoresDuplicate(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("alice@node1")).Build()
	sig := &prototk.AttestationResult{Name: "sig", Verifier: &prototk.ResolvedVerifier{Lookup: "alice@node1"}}
	txn.applySignature(context.Background(), sig)
	txn.applySignature(context.Background(), sig)
	assert.Len(t, txn.pt.PostAssembly.AssembleResponse.GetSignatures(), 1)
}

func TestApplySignature_IgnoresNil(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("alice@node1")).Build()
	txn.applySignature(context.Background(), nil)
	assert.Empty(t, txn.pt.PostAssembly.AssembleResponse.GetSignatures())
}

func TestVerifierLookup_NilVerifier(t *testing.T) {
	assert.Equal(t, "", verifierLookup(&prototk.AttestationResult{}))
	assert.Equal(t, "alice", verifierLookup(&prototk.AttestationResult{Verifier: &prototk.ResolvedVerifier{Lookup: "alice"}}))
}

func TestUnfulfilledSignRequirements_LocalUnfulfilled(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("alice@node1")).Build()
	req := txn.unfulfilledSignRequirements(context.Background())
	require.Len(t, req, 1)
	assert.False(t, txn.signRequirementsFulfilled(context.Background()))
	assert.False(t, guard_SignRequirementsFulfilled(context.Background(), txn))
}

func TestUnfulfilledSignRequirements_Fulfilled(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("alice@node1")).Build()
	txn.pt.PostAssembly.AssembleResponse.Signatures = []*prototk.AttestationResult{{Name: "sig"}}
	assert.Empty(t, txn.unfulfilledSignRequirements(context.Background()))
	assert.True(t, txn.signRequirementsFulfilled(context.Background()))
}

func TestUnfulfilledSignRequirements_RemotePartySkipped(t *testing.T) {
	// A SIGN party on a non-originator node is fundamentally unsupported and must be skipped (not counted).
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("bob@node2")).Build()
	assert.Empty(t, txn.unfulfilledSignRequirements(context.Background()))
}

func TestUnfulfilledSignRequirements_InvalidLocatorSkipped(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("me@node1@extra")).Build()
	assert.Empty(t, txn.unfulfilledSignRequirements(context.Background()))
}

func TestUnfulfilledSignRequirements_NonSignSkipped(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(&components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{Name: "e", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"notary@node1"}},
			},
		},
	}).Build()
	assert.Empty(t, txn.unfulfilledSignRequirements(context.Background()))
}

func TestUnfulfilledSignRequirements_NilPostAssembly(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).Build()
	txn.pt.PostAssembly = nil
	assert.Empty(t, txn.unfulfilledSignRequirements(context.Background()))
}

func TestAction_Signed(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("alice@node1")).Build()
	err := action_Signed(context.Background(), txn, &SignedEvent{
		AttestationResult: &prototk.AttestationResult{Name: "sig", Verifier: &prototk.ResolvedVerifier{Lookup: "alice@node1"}},
	})
	require.NoError(t, err)
	assert.Len(t, txn.pt.PostAssembly.AssembleResponse.GetSignatures(), 1)
}

func TestAction_SignError_IncrementsCount(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("alice@node1")).Build()
	before := txn.assembleErrorCount
	err := action_SignError(context.Background(), txn, &SignErrorEvent{})
	require.NoError(t, err)
	assert.Equal(t, before+1, txn.assembleErrorCount)
}

func TestValidator_MatchesPendingAssembleRequest_SignEvents(t *testing.T) {
	b := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("alice@node1")).AddPendingAssembleRequest()
	txn, _ := b.Build()
	reqID := txn.pendingAssembleRequest.IdempotencyKey()

	match, err := validator_MatchesPendingAssembleRequest(context.Background(), txn, &SignedEvent{RequestID: reqID})
	require.NoError(t, err)
	assert.True(t, match)

	mismatch, err := validator_MatchesPendingAssembleRequest(context.Background(), txn, &SignErrorEvent{RequestID: uuid.New()})
	require.NoError(t, err)
	assert.False(t, mismatch)

	matchErr, err := validator_MatchesPendingAssembleRequest(context.Background(), txn, &SignErrorEvent{RequestID: reqID})
	require.NoError(t, err)
	assert.True(t, matchErr)
}

// ─── State machine transitions from State_Signing ────────────────────────

func TestCoordinator_Signing_StaysOnPartialSigned(t *testing.T) {
	ctx := context.Background()
	// Two SIGN attestations; only one gets signed → still unfulfilled → stay in State_Signing.
	plan := &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AttestationPlan: []*prototk.AttestationRequest{
				{Name: "sig1", AttestationType: prototk.AttestationType_SIGN, Parties: []string{"alice@node1"}},
				{Name: "sig2", AttestationType: prototk.AttestationType_SIGN, Parties: []string{"alice@node1"}},
			},
		},
	}
	b := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(plan).AddPendingAssembleRequest()
	txn, _ := b.Build()
	reqID := txn.pendingAssembleRequest.IdempotencyKey()

	err := txn.HandleEvent(ctx, &SignedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            reqID,
		AttestationResult:    &prototk.AttestationResult{Name: "sig1", Verifier: &prototk.ResolvedVerifier{Lookup: "alice@node1"}},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Signing, txn.GetCurrentState())
}

func TestCoordinator_Signing_SignError_RepoolUnderThreshold(t *testing.T) {
	ctx := context.Background()
	b := NewTransactionBuilderForTesting(t, State_Signing).
		PostAssembly(coordSignPlan("alice@node1")).
		AddPendingAssembleRequest().
		AssembleErrorRetryThreshold(3)
	txn, _ := b.Build()
	reqID := txn.pendingAssembleRequest.IdempotencyKey()

	err := txn.HandleEvent(ctx, &SignErrorEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            reqID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func TestCoordinator_Signing_SignError_EvictOverThreshold(t *testing.T) {
	ctx := context.Background()
	b := NewTransactionBuilderForTesting(t, State_Signing).
		PostAssembly(coordSignPlan("alice@node1")).
		AddPendingAssembleRequest().
		AssembleErrorRetryThreshold(0)
	txn, _ := b.Build()
	txn.assembleErrorCount = 1 // already over threshold before this error increments further
	reqID := txn.pendingAssembleRequest.IdempotencyKey()

	err := txn.HandleEvent(ctx, &SignErrorEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            reqID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Evicted, txn.GetCurrentState())
}

func TestCoordinator_Signing_StateTimeout_Repools(t *testing.T) {
	ctx := context.Background()
	b := NewTransactionBuilderForTesting(t, State_Signing).PostAssembly(coordSignPlan("alice@node1"))
	txn, _ := b.Build()

	err := txn.HandleEvent(ctx, &StateTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.GetCurrentState())
}

func TestCoordinatorSignEvents_TypeStrings(t *testing.T) {
	assert.Equal(t, Event_Signed, (&SignedEvent{}).Type())
	assert.Equal(t, "Event_Signed", (&SignedEvent{}).TypeString())
	assert.Equal(t, Event_SignError, (&SignErrorEvent{}).Type())
	assert.Equal(t, "Event_SignError", (&SignErrorEvent{}).TypeString())
}
