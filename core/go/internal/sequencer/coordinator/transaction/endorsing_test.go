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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_EndorsedRejected_CompletesWithoutError(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	event := &EndorsedRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		RevertReason:           "rejected by endorser",
		Party:                  "party1",
		AttestationRequestName: "att1",
		RequestID:              uuid.New(),
	}

	err := action_EndorsedRejected(ctx, txn, event)
	require.NoError(t, err)
	// applyEndorsementRejection is a no-op (returns nil); assert action completed
}

func Test_action_NudgeEndorsementRequests_CallsSendEndorsementRequests(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	// No unfulfilled endorsement requirements: PostAssembly nil so unfulfilledEndorsementRequirements returns empty.
	// PreAssembly must be non-nil because sendEndorsementRequests reads t.pt.PreAssembly.Verifiers.
	txn.pt.PostAssembly = nil
	txn.pt.PreAssembly = &components.TransactionPreAssembly{Verifiers: []*prototk.ResolvedVerifier{}}

	err := action_NudgeEndorsementRequests(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NudgeEndorsementRequests_WithUnfulfilledRequirements_InitializesPendingRequests(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AttestationPlan: []*prototk.AttestationRequest{
			{
				Name:            "att1",
				AttestationType: prototk.AttestationType_ENDORSE,
				Parties:         []string{"party1"},
			},
		},
		Endorsements: []*prototk.AttestationResult{},
		InputStates:  []*components.FullState{},
		ReadStates:   []*components.FullState{},
		OutputStates: []*components.FullState{},
		InfoStates:   []*components.FullState{},
	}
	txn.pt.PreAssembly = &components.TransactionPreAssembly{
		Verifiers:                []*prototk.ResolvedVerifier{{Lookup: "v1"}},
		TransactionSpecification: nil,
	}

	mocks.transportWriter.EXPECT().
		SendEndorsementRequest(
			ctx, txn.pt.ID, mock.Anything, "party1", mock.Anything,
			(*prototk.TransactionSpecification)(nil), mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil)

	err := action_NudgeEndorsementRequests(ctx, txn, nil)
	require.NoError(t, err)
	// Assert state: pending endorsement requests were initialized (sendEndorsementRequests path)
	assert.NotNil(t, txn.pendingEndorsementRequests)
	mocks.transportWriter.AssertExpectations(t)
}

func Test_sendEndorsementRequests_WhenPendingNil_SchedulesTimerAndQueueEventOnFire(t *testing.T) {
	ctx := context.Background()
	realClock := common.RealClock()
	mockTransportWriter := transport.NewMockTransportWriter(t)
	mockEngineIntegration := common.NewMockEngineIntegration(t)
	grapher := NewGrapher(ctx)
	txn, err := NewTransaction(
		ctx,
		fmt.Sprintf("%s@%s", uuid.NewString(), uuid.NewString()),
		&components.PrivateTransaction{
			ID:           uuid.New(),
			Domain:       "test-domain",
			PreAssembly:  &components.TransactionPreAssembly{Verifiers: []*prototk.ResolvedVerifier{}},
			PostAssembly: nil, // no unfulfilled requirements so we only hit the nil branch and schedule timer
		},
		false,
		mockTransportWriter,
		realClock,
		func(ctx context.Context, event common.Event) {},
		mockEngineIntegration,
		&syncpoints.MockSyncPoints{},
		realClock.Duration(1),
		realClock.Duration(5000),
		5,
		"",
		prototk.ContractConfig_SUBMITTER_COORDINATOR,
		grapher,
		nil,
	)
	require.NoError(t, err)

	var timeoutEventReceived bool
	var mu sync.Mutex
	txn.queueEventForCoordinator = func(ctx context.Context, event common.Event) {
		if _, ok := event.(*RequestTimeoutIntervalEvent); ok {
			mu.Lock()
			timeoutEventReceived = true
			mu.Unlock()
		}
	}

	err = txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)
	time.Sleep(15 * time.Millisecond)

	mu.Lock()
	assert.True(t, timeoutEventReceived, "queueEventForCoordinator should have been called with RequestTimeoutIntervalEvent")
	mu.Unlock()
}

func Test_sendEndorsementRequests_TwoAttestationNames_CreatesMapPerName(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AttestationPlan: []*prototk.AttestationRequest{
			{Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1"}},
			{Name: "att2", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party2"}},
		},
		Endorsements: []*prototk.AttestationResult{},
		InputStates:  []*components.FullState{},
		ReadStates:   []*components.FullState{},
		OutputStates: []*components.FullState{},
		InfoStates:   []*components.FullState{},
	}
	txn.pt.PreAssembly = &components.TransactionPreAssembly{Verifiers: []*prototk.ResolvedVerifier{}}

	mocks.transportWriter.EXPECT().
		SendEndorsementRequest(
			ctx, txn.pt.ID, mock.Anything, "party1", mock.Anything,
			(*prototk.TransactionSpecification)(nil), mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil)
	mocks.transportWriter.EXPECT().
		SendEndorsementRequest(
			ctx, txn.pt.ID, mock.Anything, "party2", mock.Anything,
			(*prototk.TransactionSpecification)(nil), mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil)

	err := txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)
	assert.Contains(t, txn.pendingEndorsementRequests, "att1")
	assert.Contains(t, txn.pendingEndorsementRequests, "att2")
	mocks.transportWriter.AssertExpectations(t)
}

func Test_applyEndorsement_NoPendingRequestForAttestationName_IgnoresAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{Endorsements: []*prototk.AttestationResult{}}
	txn.pendingEndorsementRequests = make(map[string]map[string]*common.IdempotentRequest)
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
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{Endorsements: []*prototk.AttestationResult{}}
	pr := common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, k uuid.UUID) error { return nil })
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"att1": {"party1": pr},
	}

	endorsement := &prototk.AttestationResult{
		Name:     "att1",
		Verifier: &prototk.ResolvedVerifier{Lookup: "party1"},
	}
	wrongRequestID := uuid.New() // different from pr.IdempotencyKey()

	err := txn.applyEndorsement(ctx, endorsement, wrongRequestID)
	require.NoError(t, err)
	assert.Empty(t, txn.pt.PostAssembly.Endorsements)
}

func Test_applyEndorsement_NoPendingRequestForParty_IgnoresAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{Endorsements: []*prototk.AttestationResult{}}
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"att1": {
			"otherParty": common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, k uuid.UUID) error { return nil }),
		},
	}

	endorsement := &prototk.AttestationResult{
		Name:     "att1",
		Verifier: &prototk.ResolvedVerifier{Lookup: "party1"},
	}
	requestID := uuid.New()

	err := txn.applyEndorsement(ctx, endorsement, requestID)
	require.NoError(t, err)
	assert.Empty(t, txn.pt.PostAssembly.Endorsements)
}

func Test_sendEndorsementRequests_NudgeReturnsError_SetsLatestError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AttestationPlan: []*prototk.AttestationRequest{
			{Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1"}},
		},
		Endorsements: []*prototk.AttestationResult{},
		InputStates:  []*components.FullState{},
		ReadStates:   []*components.FullState{},
		OutputStates: []*components.FullState{},
		InfoStates:   []*components.FullState{},
	}
	txn.pt.PreAssembly = &components.TransactionPreAssembly{Verifiers: []*prototk.ResolvedVerifier{}}

	mocks.transportWriter.EXPECT().
		SendEndorsementRequest(
			ctx, txn.pt.ID, mock.Anything, "party1", mock.Anything,
			(*prototk.TransactionSpecification)(nil), mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(assert.AnError)

	err := txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, txn.latestError)
}

func Test_resetEndorsementRequests_WhenPendingNotNull_CancelsAndClears(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	cancelCalled := false
	txn.cancelRequestTimeoutSchedule = func() { cancelCalled = true }
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"att1": {},
	}

	txn.resetEndorsementRequests(ctx)

	assert.True(t, cancelCalled)
	assert.Nil(t, txn.pendingEndorsementRequests)
}

func Test_endorsementStateTimeoutExceeded_NoPendingRequests_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.pendingEndorsementRequests = nil

	assert.False(t, txn.endorsementStateTimeoutExceeded(ctx))
	assert.False(t, guard_EndorsementStateTimeoutExceeded(ctx, txn))
}

func Test_endorsementStateTimeoutExceeded_WithPendingRequest_NotExpired(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	req := common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})
	require.NoError(t, req.Nudge(ctx))
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"att1": {"party1": req},
	}

	mocks.clock.Advance(1000) // less than default state timeout (5000)
	assert.False(t, txn.endorsementStateTimeoutExceeded(ctx))
	assert.False(t, guard_EndorsementStateTimeoutExceeded(ctx, txn))
}

func Test_endorsementStateTimeoutExceeded_WithPendingRequest_Expired(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	req := common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})
	require.NoError(t, req.Nudge(ctx))
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"att1": {"party1": req},
	}

	mocks.clock.Advance(6000) // greater than default state timeout (5000)
	assert.True(t, txn.endorsementStateTimeoutExceeded(ctx))
	assert.True(t, guard_EndorsementStateTimeoutExceeded(ctx, txn))
}

func Test_endorsementStateTimeoutExceeded_WithPendingRequest_NilFirstRequestTime_UsesStateEntryTime(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	req := common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return errors.New("send failed")
	})
	_ = req.Nudge(ctx) // Keep FirstRequestTime nil
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"att1": {"party1": req},
	}

	assert.False(t, txn.endorsementStateTimeoutExceeded(ctx))
	assert.False(t, guard_EndorsementStateTimeoutExceeded(ctx, txn))

	mocks.clock.Advance(6000) // greater than default state timeout (5000)
	assert.True(t, txn.endorsementStateTimeoutExceeded(ctx))
	assert.True(t, guard_EndorsementStateTimeoutExceeded(ctx, txn))
}

func Test_EndorsementCompletion_ResetsRequests_OnTransitionToConfirmingDispatch(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	txn, _ := builder.BuildWithMocks()

	require.NotNil(t, txn.pendingEndorsementRequests)
	require.NotNil(t, txn.cancelRequestTimeoutSchedule)
	require.NotNil(t, txn.cancelStateTimeoutSchedule)

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(2))
	require.NoError(t, err)
	assert.Equal(t, State_Confirming_Dispatchable, txn.GetCurrentState())
	assert.Nil(t, txn.pendingEndorsementRequests)
}

func Test_EndorsementCompletion_ResetsRequests_OnTransitionToBlocked(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	blockingDependency := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		InputStateIDs(blockingDependency.pt.PostAssembly.OutputStates[0].ID)
	txn, _ := builder.BuildWithMocks()

	require.NotNil(t, txn.pendingEndorsementRequests)
	require.NotNil(t, txn.cancelRequestTimeoutSchedule)
	require.NotNil(t, txn.cancelStateTimeoutSchedule)

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(2))
	require.NoError(t, err)
	assert.Equal(t, State_Blocked, txn.GetCurrentState())
	assert.Nil(t, txn.pendingEndorsementRequests)
	assert.Nil(t, txn.cancelRequestTimeoutSchedule)
	assert.Nil(t, txn.cancelStateTimeoutSchedule)
}

func Test_requestEndorsement_TransportError_SetsLatestErrorAndReturnsError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)
	txn.pt.PreAssembly = &components.TransactionPreAssembly{
		Verifiers:                []*prototk.ResolvedVerifier{},
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		Signatures:   []*prototk.AttestationResult{},
		InputStates:  []*components.FullState{},
		ReadStates:   []*components.FullState{},
		OutputStates: []*components.FullState{},
		InfoStates:   []*components.FullState{},
	}

	mocks.transportWriter.EXPECT().
		SendEndorsementRequest(
			ctx, txn.pt.ID, mock.Anything, "party1", mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(assert.AnError)

	err := txn.requestEndorsement(ctx, uuid.New(), "party1", &prototk.AttestationRequest{Name: "att1"})
	require.Error(t, err)
	assert.NotEmpty(t, txn.latestError)
}
