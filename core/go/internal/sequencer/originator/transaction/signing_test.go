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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func signPlan(parties ...string) *components.TransactionPostAssembly {
	return &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "sig",
					AttestationType: prototk.AttestationType_SIGN,
					Algorithm:       "ecdsa",
					VerifierType:    "eth_address",
					Parties:         parties,
					Payload:         []byte("payload"),
					PayloadType:     "bytes",
				},
			},
		},
	}
}

func TestGuard_HasLocalSignRequirement_LocalParty(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")
	assert.True(t, guard_HasLocalSignRequirement(context.Background(), txn))
}

func TestGuard_HasLocalSignRequirement_RemoteParty(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("bob@node2")
	assert.False(t, guard_HasLocalSignRequirement(context.Background(), txn))
}

func TestGuard_HasLocalSignRequirement_InvalidLocatorSkipped(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("me@node1@extra")
	assert.False(t, guard_HasLocalSignRequirement(context.Background(), txn))
}

func TestGuard_HasLocalSignRequirement_NoSignAttestation(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"notary@node1"}},
			},
		},
	}
	assert.False(t, guard_HasLocalSignRequirement(context.Background(), txn))
}

func TestGuard_HasLocalSignRequirement_NilPostAssembly(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = nil
	assert.False(t, guard_HasLocalSignRequirement(context.Background(), txn))
}

func TestAction_FulfilSignAttestations_LocalPartySigns(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")

	result := &prototk.AttestationResult{Name: "sig", Payload: []byte("signature")}
	mocks.EngineIntegration.On("SignAttestation", mock.Anything, txn.pt.ID, mock.Anything, "alice@node1").
		Return(result, nil).Once()

	err := action_FulfilSignAttestations(ctx, txn, nil)
	require.NoError(t, err)
	require.NotNil(t, txn.cancelCurrentSign)

	event := <-mocks.Events
	successEvent, ok := event.(*SignSuccessEvent)
	require.True(t, ok, "expected SignSuccessEvent, got %T", event)
	assert.Equal(t, txn.latestFulfilledAssembleRequestID, successEvent.RequestID)
	require.Len(t, successEvent.Results, 1)
	assert.Equal(t, []byte("signature"), successEvent.Results[0].Payload)
}

func TestAction_FulfilSignAttestations_RemotePartyContributesNothing(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("bob@node2")

	// Remote party: SignAttestation returns (nil, nil).
	mocks.EngineIntegration.On("SignAttestation", mock.Anything, txn.pt.ID, mock.Anything, "bob@node2").
		Return(nil, nil).Once()

	err := action_FulfilSignAttestations(ctx, txn, nil)
	require.NoError(t, err)

	event := <-mocks.Events
	successEvent, ok := event.(*SignSuccessEvent)
	require.True(t, ok, "expected SignSuccessEvent, got %T", event)
	assert.Empty(t, successEvent.Results)
}

func TestAction_FulfilSignAttestations_SignErrorQueuesSignErrorEvent(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")

	mocks.EngineIntegration.On("SignAttestation", mock.Anything, txn.pt.ID, mock.Anything, "alice@node1").
		Return(nil, errors.New("sign boom")).Once()

	err := action_FulfilSignAttestations(ctx, txn, nil)
	require.NoError(t, err)

	event := <-mocks.Events
	errorEvent, ok := event.(*SignErrorEvent)
	require.True(t, ok, "expected SignErrorEvent, got %T", event)
	assert.Equal(t, txn.latestFulfilledAssembleRequestID, errorEvent.RequestID)
}

func TestAction_FulfilSignAttestations_CancelsPreviousInFlight(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")

	cancelled := false
	txn.cancelCurrentSign = func() { cancelled = true }

	mocks.EngineIntegration.On("SignAttestation", mock.Anything, txn.pt.ID, mock.Anything, "alice@node1").
		Return(&prototk.AttestationResult{Name: "sig"}, nil).Once()

	err := action_FulfilSignAttestations(ctx, txn, nil)
	require.NoError(t, err)
	assert.True(t, cancelled, "previous in-flight sign should have been cancelled")
	<-mocks.Events
}

func TestHandleSign_AbandonsOnCancelledContext(t *testing.T) {
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	mocks.EngineIntegration.On("SignAttestation", mock.Anything, txn.pt.ID, mock.Anything, "alice@node1").
		Return(nil, errors.New("cancelled")).Once()

	txn.handleSign(cancelledCtx, txn.pt.ID, txn.latestFulfilledAssembleRequestID, txn.pt.PostAssembly.AssembleResponse.GetAttestationPlan())

	// No event should be queued when the context is already cancelled.
	select {
	case event := <-mocks.Events:
		t.Fatalf("expected no event on cancelled context, got %T", event)
	default:
	}
}

func TestHandleSign_SkipsNonSignAttestations(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()

	plan := []*prototk.AttestationRequest{
		{AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"notary@node1"}},
		{Name: "sig", AttestationType: prototk.AttestationType_SIGN, Parties: []string{"alice@node1"}},
	}
	mocks.EngineIntegration.On("SignAttestation", mock.Anything, txn.pt.ID, mock.Anything, "alice@node1").
		Return(&prototk.AttestationResult{Name: "sig"}, nil).Once()

	txn.handleSign(ctx, txn.pt.ID, txn.latestFulfilledAssembleRequestID, plan)

	event := <-mocks.Events
	successEvent, ok := event.(*SignSuccessEvent)
	require.True(t, ok)
	require.Len(t, successEvent.Results, 1)
}

func TestAction_SendSignResponse_TransportError(t *testing.T) {
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).WithMockTransportWriter().BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")
	txn.pt.PostAssembly.AssembleResponse.Signatures = []*prototk.AttestationResult{{Name: "sig1"}}

	mocks.TransportWriter.On("SendSignResponse", mock.Anything, txn.currentDelegate, mock.Anything).
		Return(errors.New("transport down")).Once()

	err := action_SendSignResponse(context.Background(), txn, nil)
	require.ErrorContains(t, err, "transport down")
}

func TestAction_SignSuccess_RecordsSignatures(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")

	results := []*prototk.AttestationResult{{Name: "sig", Payload: []byte("sig-bytes")}}
	err := action_SignSuccess(context.Background(), txn, &SignSuccessEvent{Results: results})
	require.NoError(t, err)
	require.Len(t, txn.pt.PostAssembly.AssembleResponse.GetSignatures(), 1)
	assert.Equal(t, []byte("sig-bytes"), txn.pt.PostAssembly.AssembleResponse.GetSignatures()[0].Payload)
}

func TestAction_SendSignResponse_PushesOnePerSignature(t *testing.T) {
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")
	txn.pt.PostAssembly.AssembleResponse.Signatures = []*prototk.AttestationResult{
		{Name: "sig1"}, {Name: "sig2"},
	}

	err := action_SendSignResponse(context.Background(), txn, nil)
	require.NoError(t, err)
	sent := mocks.SentMessageRecorder.SentSignResponses()
	require.Len(t, sent, 2)
	assert.Equal(t, txn.pt.ID.String(), sent[0].TransactionId)
	assert.Equal(t, txn.latestFulfilledAssembleRequestID.String(), sent[0].AssembleRequestId)
	assert.Equal(t, "sig1", sent[0].AttestationResult.Name)
	assert.Equal(t, "sig2", sent[1].AttestationResult.Name)
}

func TestAction_SendSignError_Pushes(t *testing.T) {
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	err := action_SendSignError(context.Background(), txn, nil)
	require.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentSignError())
}

func TestAction_CancelCurrentSign(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()

	// No-op when no goroutine is running.
	require.NoError(t, action_CancelCurrentSign(context.Background(), txn, nil))

	cancelled := false
	txn.cancelCurrentSign = func() { cancelled = true }
	require.NoError(t, action_CancelCurrentSign(context.Background(), txn, nil))
	assert.True(t, cancelled)
	assert.Nil(t, txn.cancelCurrentSign)
}

func TestValidator_SignSuccessMatchesCurrentRequest(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()

	match, err := validator_SignSuccessMatchesCurrentRequest(context.Background(), txn, &SignSuccessEvent{RequestID: txn.latestFulfilledAssembleRequestID})
	require.NoError(t, err)
	assert.True(t, match)

	mismatch, err := validator_SignSuccessMatchesCurrentRequest(context.Background(), txn, &SignSuccessEvent{RequestID: uuid.New()})
	require.NoError(t, err)
	assert.False(t, mismatch)
}

// ─── State machine transitions ──────────────────────────────────────────

func TestOriginatorTransaction_Assembling_ToSigning_OnAssembleSuccess(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).BuildWithMocks()

	postAssembly := signPlan("alice@node1")
	mocks.EngineIntegration.On("SignAttestation", mock.Anything, txn.pt.ID, mock.Anything, "alice@node1").
		Return(&prototk.AttestationResult{Name: "sig", Payload: []byte("sig")}, nil).Maybe()

	err := txn.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseEvent:    BaseEvent{TransactionID: txn.pt.ID},
		RequestID:    txn.latestAssembleRequest.requestID,
		PostAssembly: postAssembly,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Signing, txn.GetCurrentState())
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse())
}

func TestOriginatorTransaction_Signing_ToEndorsement_OnSignSuccess(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")

	err := txn.HandleEvent(ctx, &SignSuccessEvent{
		BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
		RequestID: txn.latestFulfilledAssembleRequestID,
		Results:   []*prototk.AttestationResult{{Name: "sig", Payload: []byte("sig")}},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState())
	require.Len(t, mocks.SentMessageRecorder.SentSignResponses(), 1)
}

func TestOriginatorTransaction_Signing_StaleSignSuccessIgnored(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")

	err := txn.HandleEvent(ctx, &SignSuccessEvent{
		BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
		RequestID: uuid.New(), // does not match latestFulfilledAssembleRequestID
	})
	require.NoError(t, err)
	assert.Equal(t, State_Signing, txn.GetCurrentState())
	assert.Empty(t, mocks.SentMessageRecorder.SentSignResponses())
}

func TestOriginatorTransaction_Signing_ToDelegated_OnSignError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Signing).BuildWithMocks()
	txn.pt.PostAssembly = signPlan("alice@node1")

	err := txn.HandleEvent(ctx, &SignErrorEvent{
		BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
		RequestID: txn.latestFulfilledAssembleRequestID,
	})
	require.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
	assert.True(t, mocks.SentMessageRecorder.HasSentSignError())
}

func TestSignEvents_TypeStrings(t *testing.T) {
	assert.Equal(t, Event_SignSuccess, (&SignSuccessEvent{}).Type())
	assert.Equal(t, "Event_SignSuccess", (&SignSuccessEvent{}).TypeString())
	assert.Equal(t, Event_SignError, (&SignErrorEvent{}).Type())
	assert.Equal(t, "Event_SignError", (&SignErrorEvent{}).TypeString())
	assert.Equal(t, Event_AssembleSuccess, (&AssembleSuccessEvent{}).Type())
	assert.Equal(t, "Event_AssembleSuccess", (&AssembleSuccessEvent{}).TypeString())
}
