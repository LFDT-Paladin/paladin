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
	"github.com/LFDT-Paladin/paladin/core/mocks/graphermocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_NudgeEndorsementRequests_CallsSendEndorsementRequests(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{}).
		Build()

	err := action_NudgeEndorsementRequests(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NudgeEndorsementRequests_WithUnfulfilledRequirements_InitializesPendingRequests(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PreAssembly(&prototk.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				ContractInfo: &prototk.ContractInfo{},
			},
		}).
		PostAssembly(&components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				AttestationPlan:   []*prototk.AttestationRequest{{Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1@node1"}}},
				Endorsements:      []*prototk.AttestationResult{},
				ResolvedVerifiers: []*prototk.ResolvedVerifier{{Lookup: "v1"}},
			},
		}).
		UseMockTransportWriter().
		WithCurrentBlockHeight(100).
		Build()

	mocks.TransportWriter.EXPECT().
		SendEndorsementRequest(
			mock.Anything, "node1", mock.MatchedBy(func(msg *engineProto.EndorsementRequest) bool {
				return msg.TransactionId == txn.pt.ID.String() && msg.Party == "party1@node1"
			}),
		).Return(nil)

	err := action_NudgeEndorsementRequests(ctx, txn, nil)
	require.NoError(t, err)
	// Assert state: pending endorsement requests were initialized (sendEndorsementRequests path)
	assert.NotNil(t, txn.pendingEndorsementRequests)
}

func Test_sendEndorsementRequests_SendEndorsementRequestReturnsError_LogsAndContinues(t *testing.T) {
	ctx := t.Context()
	sendErr := errors.New("transport send failed")
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PreAssembly(&prototk.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				ContractInfo: &prototk.ContractInfo{},
			},
		}).
		PostAssembly(&components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				AttestationPlan:   []*prototk.AttestationRequest{{Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1@node1"}}},
				Endorsements:      []*prototk.AttestationResult{},
				ResolvedVerifiers: []*prototk.ResolvedVerifier{{Lookup: "v1"}},
			},
		}).
		UseMockTransportWriter().
		WithCurrentBlockHeight(100).
		Build()

	mocks.TransportWriter.EXPECT().
		SendEndorsementRequest(
			mock.Anything, "node1", mock.MatchedBy(func(msg *engineProto.EndorsementRequest) bool {
				return msg.TransactionId == txn.pt.ID.String() && msg.Party == "party1@node1"
			}),
		).Return(sendErr)

	err := txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)
	assert.NotNil(t, txn.pendingEndorsementRequests)
}

func Test_sendEndorsementRequests_WhenPendingNil_SchedulesTimerAndQueueEventOnFire(t *testing.T) {
	ctx := t.Context()
	var timeoutEventReceived bool
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		UseMockClock().
		UseMockTransportWriter().
		NumberOfRequiredEndorsers(0).
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
	mocks.TransportWriter.EXPECT().SendEndorsementRequest(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	err := txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)

	assert.True(t, timeoutEventReceived, "queueEventForCoordinator should have been called with RequestTimeoutIntervalEvent")
}

func Test_sendEndorsementRequests_TwoAttestationNames_CreatesMapPerName(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PreAssembly(&prototk.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				ContractInfo: &prototk.ContractInfo{},
			},
		}).
		PostAssembly(&components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				AttestationPlan: []*prototk.AttestationRequest{{Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1@node1"}}, {Name: "att2", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party2@node2"}}},
				Endorsements:    []*prototk.AttestationResult{},
			},
		}).
		UseMockTransportWriter().
		WithCurrentBlockHeight(100).
		Build()

	mocks.TransportWriter.EXPECT().
		SendEndorsementRequest(
			mock.Anything, "node1", mock.MatchedBy(func(msg *engineProto.EndorsementRequest) bool {
				return msg.TransactionId == txn.pt.ID.String() && msg.Party == "party1@node1"
			}),
		).Return(nil)
	mocks.TransportWriter.EXPECT().
		SendEndorsementRequest(
			mock.Anything, "node2", mock.MatchedBy(func(msg *engineProto.EndorsementRequest) bool {
				return msg.TransactionId == txn.pt.ID.String() && msg.Party == "party2@node2"
			}),
		).Return(nil)

	err := txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)
	assert.Contains(t, txn.pendingEndorsementRequests, "att1")
	assert.Contains(t, txn.pendingEndorsementRequests, "att2")
}

func Test_sendEndorsementRequests_PermanentlyFailedParty_IsSkipped(t *testing.T) {
	// Covers the "nil sentinel" branch in sendEndorsementRequests: if a party was marked as
	// permanently failed in a previous call (nil value in the pending map), subsequent calls
	// must skip that party entirely and not send another endorsement request.
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				AttestationPlan: []*prototk.AttestationRequest{{
					Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1@node1"},
				}},
				Endorsements: []*prototk.AttestationResult{},
			},
		}).
		UseMockTransportWriter().
		WithCurrentBlockHeight(100).
		Build()

	// Pre-populate pendingEndorsementRequests so the nil check in the loop is reachable.
	// The outer map key exists, and the party entry is nil — the permanently-failed sentinel.
	txn.pendingEndorsementRequests = map[string]map[string]*common.IdempotentRequest{
		"att1": {"party1@node1": nil},
	}

	// No SendEndorsementRequest expectation registered: any call to it would fail the test.
	_ = mocks

	err := txn.sendEndorsementRequests(ctx)
	require.NoError(t, err)
	// The nil sentinel must be preserved — it was not overwritten.
	assert.Nil(t, txn.pendingEndorsementRequests["att1"]["party1@node1"])
}

func Test_action_RecordEndorseFailure_UnknownEventType_WarnsAndReturnsNil(t *testing.T) {
	// Covers the defensive guard in action_RecordEndorseFailure that fires when the event type
	// is not one of the three recognised failure event types.
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		AddPendingEndorsementRequest().
		Build()

	// Pass an unrecognised event type — reqName and party remain empty, triggering the warning path.
	err := action_RecordEndorseFailure(ctx, txn, &RequestTimeoutIntervalEvent{})
	require.NoError(t, err)
}

func Test_applyEndorsement_CollectsEndorsementAndClearsPendingRequest(t *testing.T) {
	// applyEndorsement runs only for an endorsement that validator_MatchesPendingEndorsementRequest
	// has matched, so it collects unconditionally and clears the request it answers.
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		PostAssembly(&components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{},
		}).
		AddPendingEndorsementRequest().
		Build()

	party := "endorser-0@node-0"
	endorsement := &prototk.AttestationResult{
		Name:     "endorse-0",
		Verifier: &prototk.ResolvedVerifier{Lookup: party},
	}

	err := txn.applyEndorsement(ctx, endorsement)
	require.NoError(t, err)
	assert.Equal(t, []*prototk.AttestationResult{endorsement}, txn.pt.PostAssembly.CollectedEndorsements)
	assert.NotContains(t, txn.pendingEndorsementRequests["endorse-0"], party, "the answered request is no longer outstanding")
}

func Test_resetEndorsementRequests_WhenPendingNotNull_CancelsAndClears(t *testing.T) {
	ctx := t.Context()
	cancelCalled := false
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).
		AddPendingEndorsementRequest().
		CancelRequestTimeoutSchedule(func() { cancelCalled = true }).
		Build()

	txn.resetEndorsementRequests(ctx)

	assert.True(t, cancelCalled)
	assert.Nil(t, txn.pendingEndorsementRequests)
}

func Test_EndorsementCompletion_ResetsRequests_OnTransitionToConfirmingDispatch(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(1).
		AddPendingEndorsementRequest()
	txn, _ := builder.Build()

	require.NotNil(t, txn.pendingEndorsementRequests)

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(0))
	require.NoError(t, err)
	assert.Equal(t, State_Confirming_Dispatchable, txn.stateMachine.GetCurrentState())
	assert.Nil(t, txn.pendingEndorsementRequests)
}

func Test_EndorsementCompletion_ResetsRequests_OnTransitionToBlocked(t *testing.T) {
	ctx := t.Context()
	grapher := graphermocks.NewGrapher(t)

	blockingTXID := uuid.New()
	_, _ = NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		TransactionID(blockingTXID).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		Build()

	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(1).
		AddPendingEndorsementRequest()
	txn, _ := builder.Build()

	grapher.EXPECT().GetDependencies(mock.Anything, txn.pt.ID).Return([]uuid.UUID{blockingTXID})

	require.NotNil(t, txn.pendingEndorsementRequests)

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(0))
	require.NoError(t, err)
	require.Equal(t, State_Blocked, txn.stateMachine.GetCurrentState())
	assert.Nil(t, txn.pendingEndorsementRequests)
}

// ── Endorsement Threshold Tests ───────────────────────────────────────────────

// threshold=0 (unset): all parties must endorse — preserves existing behaviour.
func Test_unfulfilledEndorsementRequirements_NilPostAssembly_ReturnsEmpty(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	txn.pt.PostAssembly = nil
	unfulfilled := txn.unfulfilledEndorsementRequirements(ctx)
	assert.Empty(t, unfulfilled)
}

func Test_extractEndorserNodes_NilPostAssembly_ReturnsEmpty(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).Build()
	txn.pt.PostAssembly = nil
	nodes := txn.extractEndorserNodes(ctx)
	assert.Empty(t, nodes)
}

func Test_extractEndorserNodes_InvalidPartyLocator_SkipsInvalidEntry(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).Build()
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{{
				AttestationType: prototk.AttestationType_ENDORSE,
				// "a@b@c" has too many @ signs and is an invalid locator; "valid@node1" is valid.
				Parties: []string{"a@b@c", "valid@node1"},
			}},
		},
	}
	nodes := txn.extractEndorserNodes(ctx)
	// The invalid locator is skipped; only the valid one is returned.
	assert.Equal(t, []string{"node1"}, nodes)
}

func Test_requestEndorsement_InvalidPartyLocator_ReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).Build()

	// "a@b@c" has too many @ signs and fails locator parsing.
	err := txn.requestEndorsement(ctx, uuid.New(), "a@b@c", &prototk.AttestationRequest{})
	require.Error(t, err)
}

func Test_validator_MatchesPendingEndorsementRequest(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).AddPendingEndorsementRequest()
	txn, _ := builder.Build()
	party := "endorser-0@node-0"
	pendingRequestID := txn.pendingEndorsementRequests["endorse-0"][party].IdempotencyKey().String()
	// A party already recorded as failed this round is left in the map as a nil sentinel
	failedParty := "endorser-1@node-1"
	txn.pendingEndorsementRequests["endorse-0"][failedParty] = nil

	tests := []struct {
		name     string
		event    common.Event
		expected bool
	}{
		{
			name: "matches the outstanding request",
			event: &EndorseErrorEvent{
				AttestationRequestName: "endorse-0",
				Party:                  party,
				RequestID:              pendingRequestID,
			},
			expected: true,
		},
		{
			name: "request ID from an earlier round",
			event: &EndorseRevertEvent{
				AttestationRequestName: "endorse-0",
				Party:                  party,
				RequestID:              uuid.NewString(),
			},
			expected: false,
		},
		{
			name: "response carrying no idempotency key",
			event: &EndorseErrorEvent{
				AttestationRequestName: "endorse-0",
				Party:                  party,
			},
			expected: false,
		},
		{
			name: "no outstanding request for the attestation requirement",
			event: &EndorseRequestRejectedEvent{
				AttestationRequestName: "endorse-unknown",
				Party:                  party,
				RequestID:              pendingRequestID,
			},
			expected: false,
		},
		{
			name: "no outstanding request for the party",
			event: &EndorseErrorEvent{
				AttestationRequestName: "endorse-0",
				Party:                  "other@node-1",
				RequestID:              pendingRequestID,
			},
			expected: false,
		},
		{
			name: "matches the outstanding request with an endorsement",
			event: &EndorsedEvent{
				RequestID: pendingRequestID,
				Endorsement: &prototk.AttestationResult{
					Name:     "endorse-0",
					Verifier: &prototk.ResolvedVerifier{Lookup: party},
				},
			},
			expected: true,
		},
		{
			name: "endorsement from a party already recorded as failed",
			event: &EndorsedEvent{
				RequestID: pendingRequestID,
				Endorsement: &prototk.AttestationResult{
					Name:     "endorse-0",
					Verifier: &prototk.ResolvedVerifier{Lookup: failedParty},
				},
			},
			expected: false,
		},
		{
			name: "failure response from a party already recorded as failed",
			event: &EndorseErrorEvent{
				AttestationRequestName: "endorse-0",
				Party:                  failedParty,
				RequestID:              pendingRequestID,
			},
			expected: false,
		},
		{
			name: "endorsement naming no verifier",
			event: &EndorsedEvent{
				RequestID:   pendingRequestID,
				Endorsement: &prototk.AttestationResult{Name: "endorse-0"},
			},
			expected: false,
		},
		{
			name:     "endorsement carrying no attestation result",
			event:    &EndorsedEvent{RequestID: pendingRequestID},
			expected: false,
		},
		{
			name:     "event type that carries no endorsement request",
			event:    &RequestTimeoutIntervalEvent{},
			expected: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matches, err := validator_MatchesPendingEndorsementRequest(ctx, txn, test.event)
			require.NoError(t, err)
			assert.Equal(t, test.expected, matches)
		})
	}
}

func Test_action_RecordEndorseFailure_EndorserIsActiveCoordinator_LogsWarning(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		AddPendingEndorsementRequest().
		Build()

	event := &EndorseRequestRejectedEvent{
		AttestationRequestName: "endorse-0",
		Party:                  "party1@node2",
		RejectionReason:        engineProto.RejectionReason_ENDORSER_IS_ACTIVE_COORDINATOR,
	}
	err := action_RecordEndorseFailure(ctx, txn, event)
	require.NoError(t, err)
}

func Test_action_RecordEndorseFailure_BlockHeightTolerance_LogsWarning(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		AddPendingEndorsementRequest().
		Build()

	event := &EndorseRequestRejectedEvent{
		AttestationRequestName: "endorse-0",
		Party:                  "party1@node2",
		RejectionReason:        engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
		CoordinatorBlockHeight: 100,
		EndorserBlockHeight:    200,
		BlockHeightTolerance:   10,
	}
	err := action_RecordEndorseFailure(ctx, txn, event)
	require.NoError(t, err)
}

func Test_unfulfilledEndorsementRequirements_ThresholdUnset_AllPartiesRequired(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).Build()
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{{
				Name:            "group-endorse",
				AttestationType: prototk.AttestationType_ENDORSE,
				VerifierType:    "ETH_ADDRESS",
				Parties:         []string{"p1@n1", "p2@n2", "p3@n3"},
				// Threshold unset → nil → 0 → all parties required
			}},
		},
		CollectedEndorsements: []*prototk.AttestationResult{
			{
				Name:            "group-endorse",
				AttestationType: prototk.AttestationType_ENDORSE,
				Verifier:        &prototk.ResolvedVerifier{Lookup: "p1@n1", VerifierType: "ETH_ADDRESS"},
			},
		},
	}

	unfulfilled := txn.unfulfilledEndorsementRequirements(ctx)

	require.Len(t, unfulfilled, 2, "with threshold unset, all 3 parties are required; 2 are still pending")
	parties := []string{unfulfilled[0].party, unfulfilled[1].party}
	assert.Contains(t, parties, "p2@n2")
	assert.Contains(t, parties, "p3@n3")
}

// threshold=1 of 3: one endorsement fulfils the requirement.
func Test_unfulfilledEndorsementRequirements_Threshold1of3_FulfilledAfterOneEndorsement(t *testing.T) {
	ctx := t.Context()
	threshold := int32(1)
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).Build()
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{{
				Name:            "group-endorse",
				AttestationType: prototk.AttestationType_ENDORSE,
				VerifierType:    "ETH_ADDRESS",
				Parties:         []string{"p1@n1", "p2@n2", "p3@n3"},
				Threshold:       &threshold,
			}},
		},
		CollectedEndorsements: []*prototk.AttestationResult{
			{
				Name:            "group-endorse",
				AttestationType: prototk.AttestationType_ENDORSE,
				Verifier:        &prototk.ResolvedVerifier{Lookup: "p1@n1", VerifierType: "ETH_ADDRESS"},
			},
		},
	}

	unfulfilled := txn.unfulfilledEndorsementRequirements(ctx)

	assert.Empty(t, unfulfilled, "threshold=1 met by one endorsement — plan is fulfilled")
}

// threshold=2 of 3: one endorsement is not enough; all remaining un-responded parties are nudged.
func Test_unfulfilledEndorsementRequirements_Threshold2of3_NotFulfilledAfterOne(t *testing.T) {
	ctx := t.Context()
	threshold := int32(2)
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).Build()
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{{
				Name:            "group-endorse",
				AttestationType: prototk.AttestationType_ENDORSE,
				VerifierType:    "ETH_ADDRESS",
				Parties:         []string{"p1@n1", "p2@n2", "p3@n3"},
				Threshold:       &threshold,
			}},
		},
		CollectedEndorsements: []*prototk.AttestationResult{
			{
				Name:            "group-endorse",
				AttestationType: prototk.AttestationType_ENDORSE,
				Verifier:        &prototk.ResolvedVerifier{Lookup: "p1@n1", VerifierType: "ETH_ADDRESS"},
			},
		},
	}

	unfulfilled := txn.unfulfilledEndorsementRequirements(ctx)

	// threshold=2, received=1 → not fulfilled; both remaining un-responded parties are nudged
	require.Len(t, unfulfilled, 2, "all non-responded parties should be nudged until threshold is met")
	parties := []string{unfulfilled[0].party, unfulfilled[1].party}
	assert.Contains(t, parties, "p2@n2")
	assert.Contains(t, parties, "p3@n3")
}
