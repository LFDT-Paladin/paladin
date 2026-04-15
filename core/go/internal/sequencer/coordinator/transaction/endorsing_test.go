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
	"errors"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_NudgeEndorsementRequests_CallsSendEndorsementRequests(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PreAssembly(&components.TransactionPreAssembly{Verifiers: []*prototk.ResolvedVerifier{{Lookup: "v1"}}}).
		Build()
	// No unfulfilled endorsement requirements: PostAssembly nil so unfulfilledEndorsementRequirements returns empty.
	// PreAssembly must be non-nil because sendEndorsementRequests reads t.pt.PreAssembly.Verifiers.
	txn.pt.PostAssembly = nil

	err := action_NudgeEndorsementRequests(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NudgeEndorsementRequests_WithUnfulfilledRequirements_InitializesPendingRequests(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{{Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1"}}},
			Endorsements:    []*prototk.AttestationResult{},
		}).
		PreAssembly(&components.TransactionPreAssembly{Verifiers: []*prototk.ResolvedVerifier{{Lookup: "v1"}}}).
		UseMockTransportWriter().
		Build()

	mocks.TransportWriter.EXPECT().
		SendEndorsementRequest(
			ctx, txn.pt.ID, mock.Anything, "party1", mock.Anything,
			(*prototk.TransactionSpecification)(nil), mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil)

	err := action_NudgeEndorsementRequests(ctx, txn, nil)
	require.NoError(t, err)
	// Assert state: pending endorsement requests were initialized (sendEndorsementRequests path)
	assert.NotNil(t, txn.pendingEndorsementRequests)
}

func Test_sendEndorsementRequests_SendEndorsementRequestReturnsError_LogsAndContinues(t *testing.T) {
	ctx := context.Background()
	sendErr := errors.New("transport send failed")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{{Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1"}}},
			Endorsements:    []*prototk.AttestationResult{},
		}).
		PreAssembly(&components.TransactionPreAssembly{Verifiers: []*prototk.ResolvedVerifier{{Lookup: "v1"}}}).
		UseMockTransportWriter().
		Build()

	mocks.TransportWriter.EXPECT().
		SendEndorsementRequest(
			ctx, txn.pt.ID, mock.Anything, "party1", mock.Anything,
			(*prototk.TransactionSpecification)(nil), mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(sendErr)

	err := txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)
	assert.NotNil(t, txn.pendingEndorsementRequests)
}

func Test_sendEndorsementRequests_WhenPendingNil_SchedulesTimerAndQueueEventOnFire(t *testing.T) {
	ctx := context.Background()
	var timeoutEventReceived bool
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		UseMockClock().
		QueueEventForCoordinator(func(ctx context.Context, event common.Event) {
			if _, ok := event.(*RequestTimeoutIntervalEvent); ok {
				timeoutEventReceived = true
			}
		}).
		RequestTimeout(1).
		Build()

	mocks.Clock.On("Now").Return(time.Now())
	mocks.Clock.On("ScheduleTimer", mock.Anything, time.Duration(1), mock.Anything).Return(func() {}).Run(func(args mock.Arguments) {
		callback := args.Get(2).(func())
		callback()
	})

	err := txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)

	assert.True(t, timeoutEventReceived, "queueEventForCoordinator should have been called with RequestTimeoutIntervalEvent")
}

func Test_sendEndorsementRequests_TwoAttestationNames_CreatesMapPerName(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{{Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1"}}, {Name: "att2", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party2"}}},
			Endorsements:    []*prototk.AttestationResult{},
		}).
		UseMockTransportWriter().
		Build()

	mocks.TransportWriter.EXPECT().
		SendEndorsementRequest(
			ctx, txn.pt.ID, mock.Anything, "party1", mock.Anything,
			(*prototk.TransactionSpecification)(nil), mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil)
	mocks.TransportWriter.EXPECT().
		SendEndorsementRequest(
			ctx, txn.pt.ID, mock.Anything, "party2", mock.Anything,
			(*prototk.TransactionSpecification)(nil), mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil)

	err := txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)
	assert.Contains(t, txn.pendingEndorsementRequests, "att1")
	assert.Contains(t, txn.pendingEndorsementRequests, "att2")
}

func Test_applyEndorsement_NoPendingRequestForAttestationName_IgnoresAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{Endorsements: []*prototk.AttestationResult{}}).
		Build()
	// No entry for "att1" so applyEndorsement will hit the "no pending request found for attestation request name" path

	endorsement := &prototk.AttestationResult{
		Name:     "att1",
		Verifier: &prototk.ResolvedVerifier{Lookup: "party1"},
	}

	err := txn.applyEndorsement(ctx, endorsement, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, txn.pt.PostAssembly.Endorsements)
}

func Test_applyEndorsement_IdempotencyKeyMismatch_IgnoresAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{Endorsements: []*prototk.AttestationResult{}}).
		AddPendingEndorsementRequest(0).
		Build()

	endorsement := &prototk.AttestationResult{
		Name:     "endorse-0",
		Verifier: &prototk.ResolvedVerifier{Lookup: "party1"},
	}
	wrongRequestID := uuid.New() // different from pr.IdempotencyKey()

	err := txn.applyEndorsement(ctx, endorsement, wrongRequestID)
	require.NoError(t, err)
	assert.Empty(t, txn.pt.PostAssembly.Endorsements)
}

// Test_applyEndorsement_IdempotencyKeyMismatch_WithMatchingParty covers the branch that logs
// "ignoring endorsement response ... because idempotency key ... does not match expected".
// We use the same attestation name and party as the pending request so we find the request,
// then pass a requestID that does not match the pending request's IdempotencyKey.
func Test_applyEndorsement_IdempotencyKeyMismatch_WithMatchingParty_IgnoresAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{Endorsements: []*prototk.AttestationResult{}}).
		AddPendingEndorsementRequest(0).
		Build()

	// AddPendingEndorsementRequest(0) creates attName "endorse-0" and party "endorser-0@node-0"
	expectedKey := txn.pendingEndorsementRequests["endorse-0"]["endorser-0@node-0"].IdempotencyKey()
	wrongRequestID := uuid.New()
	require.NotEqual(t, expectedKey, wrongRequestID, "test must use a different request ID")

	endorsement := &prototk.AttestationResult{
		Name:     "endorse-0",
		Verifier: &prototk.ResolvedVerifier{Lookup: "endorser-0@node-0"},
	}

	err := txn.applyEndorsement(ctx, endorsement, wrongRequestID)
	require.NoError(t, err)
	assert.Empty(t, txn.pt.PostAssembly.Endorsements, "endorsement with mismatched idempotency key should be ignored")
}

func Test_applyEndorsement_NoPendingRequestForParty_IgnoresAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{Endorsements: []*prototk.AttestationResult{}}).
		AddPendingEndorsementRequest(0).
		Build()

	endorsement := &prototk.AttestationResult{
		Name:     "att1",
		Verifier: &prototk.ResolvedVerifier{Lookup: "wrong-lookup"},
	}
	requestID := uuid.New()

	err := txn.applyEndorsement(ctx, endorsement, requestID)
	require.NoError(t, err)
	assert.Empty(t, txn.pt.PostAssembly.Endorsements)
}

func Test_resetEndorsementRequests_WhenPendingNotNull_CancelsAndClears(t *testing.T) {
	ctx := context.Background()
	cancelCalled := false
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		AddPendingEndorsementRequest(0).
		CancelRequestTimeoutSchedule(func() { cancelCalled = true }).
		Build()

	txn.resetEndorsementRequests(ctx)

	assert.True(t, cancelCalled)
	assert.Nil(t, txn.pendingEndorsementRequests)
}

func Test_EndorsementCompletion_ResetsRequests_OnTransitionToConfirmingDispatch(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		AddPendingEndorsementRequest(2).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	txn, _ := builder.Build()

	require.NotNil(t, txn.pendingEndorsementRequests)

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(2))
	require.NoError(t, err)
	assert.Equal(t, State_Confirming_Dispatchable, txn.stateMachine.GetCurrentState())
	assert.Nil(t, txn.pendingEndorsementRequests)
}

func Test_EndorsementCompletion_ResetsRequests_OnTransitionToBlocked(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	blockingTXID := uuid.New()
	_, _ = NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		TransactionID(blockingTXID).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		AddPendingEndorsementRequest(2).NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Dependencies(&pldapi.TransactionDependencies{DependsOn: []uuid.UUID{blockingTXID}})
	txn, _ := builder.Build()

	require.NotNil(t, txn.pendingEndorsementRequests)

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(2))
	require.NoError(t, err)
	require.Equal(t, State_Blocked, txn.stateMachine.GetCurrentState())
	assert.Nil(t, txn.pendingEndorsementRequests)
}
