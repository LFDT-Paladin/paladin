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

package coordinator

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// partyKeyVerifier is the resolved verifier string used for party key resolution in tests.
const partyKeyVerifier = "party-verifier"

// inFlightEndorsementKeys returns the idempotency keys of the endorsements currently in flight,
// taking the mutex that the endorsement goroutines take when they release their claim.
func inFlightEndorsementKeys(c *coordinator) []string {
	c.inFlightEndorsementsMutex.Lock()
	defer c.inFlightEndorsementsMutex.Unlock()
	keys := make([]string, 0, len(c.inFlightEndorsements))
	for key := range c.inFlightEndorsements {
		keys = append(keys, key)
	}
	return keys
}

// matchEndorsementErrorMsg returns a mock.MatchedBy matcher that inspects the EndorsementError
// proto struct to verify the transaction ID and idempotency key.
func matchEndorsementErrorMsg(txID, ik string) interface{} {
	return mock.MatchedBy(func(msg *engineProto.EndorsementError) bool {
		return msg.TransactionId == txID && msg.IdempotencyKey == ik
	})
}

// matchEndorsementRejectionMsg returns a mock.MatchedBy matcher that inspects the
// EndorsementRejection proto struct, verifying the rejection reason and optional block heights.
func matchEndorsementRejectionMsg(txID, ik string, reason engineProto.RejectionReason, coordBH, endorserBH, tolerance int64) interface{} {
	return mock.MatchedBy(func(msg *engineProto.EndorsementRejection) bool {
		return msg.TransactionId == txID &&
			msg.IdempotencyKey == ik &&
			msg.RejectionReason == reason &&
			msg.CoordinatorBlockHeight == coordBH &&
			msg.EndorserBlockHeight == endorserBH &&
			msg.BlockHeightTolerance == tolerance
	})
}

// matchEndorsementResponseMsg returns a mock.MatchedBy matcher that inspects the
// EndorsementResponse proto struct, verifying the key fields that were previously individual args.
func matchEndorsementResponseMsg(txID, ik, party, attName string, revertReason *string) interface{} {
	return mock.MatchedBy(func(msg *engineProto.EndorsementResponse) bool {
		if msg.TransactionId != txID || msg.IdempotencyKey != ik || msg.Party != party || msg.AttestationRequestName != attName {
			return false
		}
		if revertReason != nil {
			return msg.RevertReason != nil && *msg.RevertReason == *revertReason
		}
		return true
	})
}

// buildEndorsementEvent creates a minimal EndorsementRequestReceivedEvent for tests.
func buildEndorsementEvent(fromNode string) *EndorsementRequestReceivedEvent {
	return &EndorsementRequestReceivedEvent{
		FromNode:                  fromNode,
		TransactionId:             "tx-1",
		IdempotencyKey:            "ik-1",
		Party:                     "party1@" + fromNode,
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
		AttestationRequest: &prototk.AttestationRequest{
			Name:            "att1",
			AttestationType: prototk.AttestationType_ENDORSE,
		},
	}
}

// setupEndorsementMocks wires the KeyManager mock for tests that call handleEndorsementRequest
// directly. The KeyManager is pre-wired to succeed for the party key resolution
// step (party "party1@<fromNode>" → unqualifiedLookup "party1"). SIGN-path tests should add
// extra expectations on the returned KeyManager for the signing step.
func setupEndorsementMocks(t *testing.T, mocks *CoordinatorDependencyMocks) *componentsmocks.KeyManager {
	t.Helper()
	mockKeyManager := componentsmocks.NewKeyManager(t)
	partyKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{Verifier: partyKeyVerifier},
	}
	mockKeyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, "party1", mock.Anything, mock.Anything).
		Return(partyKey, nil).Maybe()
	mocks.AllComponents.On("KeyManager").Return(mockKeyManager).Maybe()
	return mockKeyManager
}

// --- validator_IsPrivateStateDataPendingForEndorsement tests ---

func Test_validator_IsPrivateStateDataPendingForEndorsement_Complete_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).Build()
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(true, nil) // lowWatermark = 100 - 10

	event := &EndorsementRequestReceivedEvent{
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10,
		AttestationRequest:     &prototk.AttestationRequest{},
	}
	result, err := validator_IsPrivateStateDataPendingForEndorsement(ctx, c, event)
	require.NoError(t, err)
	assert.False(t, result)
}

func Test_validator_IsPrivateStateDataPendingForEndorsement_Incomplete_ReturnsTrue(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).Build()
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, nil) // lowWatermark = 100 - 10

	event := &EndorsementRequestReceivedEvent{
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10,
		AttestationRequest:     &prototk.AttestationRequest{},
	}
	result, err := validator_IsPrivateStateDataPendingForEndorsement(ctx, c, event)
	require.NoError(t, err)
	assert.True(t, result)
}

func Test_validator_IsPrivateStateDataPendingForEndorsement_Error_Propagates(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).Build()
	dbErr := fmt.Errorf("db error")
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, int64(90)).Return(false, dbErr)

	event := &EndorsementRequestReceivedEvent{
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10,
		AttestationRequest:     &prototk.AttestationRequest{},
	}
	_, err := validator_IsPrivateStateDataPendingForEndorsement(ctx, c, event)
	assert.ErrorIs(t, err, dbErr)
}

func Test_action_RejectEndorsementPrivateStateDataPending_SendsRejection(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Idle).
		WithMockTransportWriter().
		Build()

	mocks.TransportWriter.EXPECT().SendEndorsementRejection(
		mock.Anything, "node2", matchEndorsementRejectionMsg("tx-1", "ik-1",
			engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING, int64(100), int64(0), int64(10)),
	).Return(nil)

	event := &EndorsementRequestReceivedEvent{
		TransactionId:          "tx-1",
		IdempotencyKey:         "ik-1",
		FromNode:               "node2",
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10,
		AttestationRequest:     &prototk.AttestationRequest{Name: "att1"},
		Party:                  "party1@node2",
	}
	err := action_RejectEndorsementPrivateStateDataPending(ctx, c, event)
	require.NoError(t, err)
}

// --- validator tests ---

func Test_validator_IsEndorsementRequestFromHigherPriorityCoordinator_HigherPriority_ReturnsTrue(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node2").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	event := &EndorsementRequestReceivedEvent{FromNode: "node1"}
	result, err := validator_IsEndorsementRequestFromHigherPriorityCoordinator(ctx, c, event)
	require.NoError(t, err)
	assert.True(t, result, "node1 (index 0) is higher priority than node2 (index 1)")
}

func Test_validator_IsEndorsementRequestFromHigherPriorityCoordinator_LowerPriority_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CoordinatorPriorityList("node1", "node2", "node3").
		Build()

	event := &EndorsementRequestReceivedEvent{FromNode: "node3"}
	result, err := validator_IsEndorsementRequestFromHigherPriorityCoordinator(ctx, c, event)
	require.NoError(t, err)
	assert.False(t, result, "node3 (index 2) is lower priority than node1 (index 0)")
}

func Test_validator_IsEndorsementRequestFromHigherPriorityCoordinator_SamePriority_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CoordinatorPriorityList("node1", "node2").
		Build()

	event := &EndorsementRequestReceivedEvent{FromNode: "node1"}
	result, err := validator_IsEndorsementRequestFromHigherPriorityCoordinator(ctx, c, event)
	require.NoError(t, err)
	assert.False(t, result, "same node is not higher priority than itself")
}

// --- validator_IsEndorsementRequestFromSelf tests ---

func Test_validator_IsEndorsementRequestFromSelf_SameNode_ReturnsTrue(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		Build()

	event := &EndorsementRequestReceivedEvent{FromNode: "node1"}
	result, err := validator_IsEndorsementRequestFromSelf(ctx, c, event)
	require.NoError(t, err)
	assert.True(t, result, "request from own node should match")
}

func Test_validator_IsEndorsementRequestFromSelf_DifferentNode_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		Build()

	event := &EndorsementRequestReceivedEvent{FromNode: "node2"}
	result, err := validator_IsEndorsementRequestFromSelf(ctx, c, event)
	require.NoError(t, err)
	assert.False(t, result, "request from a different node should not match")
}

func Test_handleEndorsementRequest_SendEndorsementErrorFails_LogsAndContinues(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	// Trigger sendErr via a party identity error (empty identity), and have SendEndorsementError itself fail.
	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "node2", matchEndorsementErrorMsg("tx-1", "ik-1")).
		Return(fmt.Errorf("transport failure"))

	event := buildEndorsementEvent("node2")
	event.Party = "@node2" // empty identity — triggers sendErr immediately
	c.handleEndorsementRequest(ctx, event)
	// Should not panic; the SendEndorsementError error is only logged.
}

// --- action_UpdateActiveCoordinatorFromEndorsementRequest tests ---

func Test_action_UpdateActiveCoordinatorFromEndorsementRequest_SetsFromNode(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Observing).
		CurrentActiveCoordinator("oldNode").
		Build()

	event := &EndorsementRequestReceivedEvent{FromNode: "newNode"}
	err := action_UpdateActiveCoordinatorFromEndorsementRequest(ctx, c, event)
	require.NoError(t, err)
	assert.Equal(t, "newNode", c.currentActiveCoordinator)
}

// --- action_HandleEndorsementRequest tests ---

func Test_action_HandleEndorsementRequest_SpawnsGoroutineThatCompletesEndorsement(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		Endorser: &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	done := make(chan struct{})
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ string, msg *engineProto.EndorsementResponse) {
			close(done)
		}).
		Return(nil)

	event := buildEndorsementEvent("node2")
	err := action_HandleEndorsementRequest(ctx, c, event)
	require.NoError(t, err)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for endorsement goroutine to complete")
	}
}

func Test_action_HandleEndorsementRequest_SendsEndorsementError_WhenExpiryAlreadyElapsed(t *testing.T) {
	// When the EndorsementRequestReceivedEvent carries an already-elapsed expiry,
	// action_HandleEndorsementRequest must spawn a goroutine whose context is already cancelled.
	// The goroutine should exit (via the key-resolution or domain-call error) and send an
	// EndorsementError back to the coordinator.
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		WithKeyManagerError(context.DeadlineExceeded).
		Build()

	errorSent := make(chan struct{})
	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "node2", matchEndorsementErrorMsg("tx-1", "ik-1")).
		Run(func(_ context.Context, _ string, _ *engineProto.EndorsementError) { close(errorSent) }).
		Return(nil)

	event := buildEndorsementEvent("node2")
	event.Expiry = time.Now().Add(-time.Second) // already expired

	err := action_HandleEndorsementRequest(ctx, c, event)
	require.NoError(t, err)

	<-errorSent
}

// --- handleEndorsementRequest goroutine tests ---

func Test_handleEndorsementRequest_Revert_SendsResponseWithRevertReason(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	revertMsg := "not allowed"
	endorsementResult := &components.EndorsementResult{
		Result:       prototk.EndorseTransactionResponse_REVERT,
		RevertReason: &revertMsg,
		Endorser:     &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, "node2", matchEndorsementResponseMsg("tx-1", "ik-1", "party1@node2", "att1", &revertMsg)).
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_Revert_NoRevertReason_UsesDefaultMessage(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:       prototk.EndorseTransactionResponse_REVERT,
		RevertReason: nil,
		Endorser:     &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	defaultMsg := "(no revert reason)"
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, "node2", matchEndorsementResponseMsg("tx-1", "ik-1", "party1@node2", "att1", &defaultMsg)).
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_NonRevertResultWithRevertReason_LogsWarningAndIgnoresReason(t *testing.T) {
	// A revert reason on a result that is not a REVERT is a domain bug - it is logged at WARN and
	// dropped, so the response carries no revert reason.
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	revertMsg := "ignored by the coordinator"
	endorsementResult := &components.EndorsementResult{
		Result:       prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		RevertReason: &revertMsg,
		Endorser:     &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	var capturedMsg *engineProto.EndorsementResponse
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, "node2", mock.Anything).
		Run(func(_ context.Context, _ string, msg *engineProto.EndorsementResponse) {
			capturedMsg = msg
		}).
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)

	require.NotNil(t, capturedMsg)
	assert.Nil(t, capturedMsg.RevertReason)
}

func Test_handleEndorsementRequest_PartyIdentityError_SendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "node2", matchEndorsementErrorMsg("tx-1", "ik-1")).
		Return(nil)

	// Party "@node2" has an empty identity part, causing PrivateIdentityLocator.Identity to fail.
	event := buildEndorsementEvent("node2")
	event.Party = "@node2"
	c.handleEndorsementRequest(ctx, event)
}

// Key resolution goes to the database, so a failure may be transient: it is retried, and the requester
// only hears about it once every attempt has failed.
func Test_handleEndorsementRequest_PartyKeyResolveError_RetriesThenSendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		EndorseErrorRetryMaxAttempts(2).
		Build()

	// Set up KeyManager to fail party key resolution; no StateManager or EndorseTransaction needed.
	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "party1", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("key not found")).Times(2)
	mocks.AllComponents.On("KeyManager").Return(mockKeyManager).Maybe()

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "node2", matchEndorsementErrorMsg("tx-1", "ik-1")).
		Return(nil).
		Once()

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

// An error from the domain may be transient, so the endorsement is attempted again before the requester
// is told: reporting an endorsement error costs the transaction a failure against this attestation
// requirement, and enough of those force it to be re-assembled.
func Test_handleEndorsementRequest_EndorseTransactionError_RetriesThenSendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		EndorseErrorRetryMaxAttempts(3).
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("domain error")).Times(3)

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "node2", matchEndorsementErrorMsg("tx-1", "ik-1")).
		Return(nil).
		Once()

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

// A domain error that clears on the next attempt is never reported: the requester sees a normal
// endorsement response and the transaction keeps its place in the round.
func Test_handleEndorsementRequest_TransientEndorseTransactionError_SucceedsOnRetry(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		Endorser: &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("domain unavailable")).Once()
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil).Once()

	// The strict transport writer mock has no SendEndorsementError expectation, so the test fails if the
	// first failure is reported to the requester.
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, "node2", matchEndorsementResponseMsg("tx-1", "ik-1", "party1@node2", "att1", nil)).
		Return(nil).
		Once()

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

// The retry covers the whole attempt, not just the call that failed: a signing failure re-runs the
// domain endorsement too, since the payload to sign comes from it.
func Test_handleEndorsementRequest_Sign_TransientSignError_SucceedsOnRetry(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		WithMockTransportWriter().
		Build()

	km := setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_SIGN,
		Endorser: &prototk.ResolvedVerifier{Lookup: "signer@node1"},
		Payload:  []byte("payload-to-sign"),
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil).Times(2)

	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{Verifier: "verifier-value"},
	}
	km.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "signer", mock.Anything, mock.Anything).Return(resolvedKey, nil).Times(2)
	km.EXPECT().Sign(mock.Anything, resolvedKey, mock.Anything, []byte("payload-to-sign")).Return(nil, fmt.Errorf("signing module unavailable")).Once()
	km.EXPECT().Sign(mock.Anything, resolvedKey, mock.Anything, []byte("payload-to-sign")).Return([]byte("signature"), nil).Once()

	var capturedAttResult *prototk.AttestationResult
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, "node2", mock.Anything).
		Run(func(_ context.Context, _ string, msg *engineProto.EndorsementResponse) {
			capturedAttResult = msg.Endorsement
		}).
		Return(nil).
		Once()

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)

	require.NotNil(t, capturedAttResult)
	assert.Equal(t, []byte("signature"), capturedAttResult.Payload)
}

// Once the request's expiry has passed the requester has stopped waiting for a reply, so the retry must
// abandon the endorsement rather than sleep between attempts.
func Test_handleEndorsementRequest_ExpiredContext_AbandonsRetryAfterOneAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		EndorseErrorRetryMaxAttempts(3).
		Build()

	// Key resolution fails with a retryable error, but the expired context stops the retry after the
	// first attempt: a second call to the strict mock would fail the test.
	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "party1", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("database unavailable")).Once()
	mocks.AllComponents.On("KeyManager").Return(mockKeyManager).Maybe()

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "node2", matchEndorsementErrorMsg("tx-1", "ik-1")).
		Return(nil).
		Once()

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_EndorserSubmit_SendsResponseWithConstraint(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		Endorser: &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	var capturedAttResult *prototk.AttestationResult
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ string, msg *engineProto.EndorsementResponse) {
			capturedAttResult = msg.Endorsement
		}).
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)

	require.NotNil(t, capturedAttResult)
	assert.Contains(t, capturedAttResult.Constraints, prototk.AttestationResult_ENDORSER_MUST_SUBMIT)
}

func Test_handleEndorsementRequest_Sign_ThisNode_SignsAndSendsResponse(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		WithMockTransportWriter().
		Build()

	km := setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	// EndorseTransaction returns SIGN with endorser on this node.
	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_SIGN,
		Endorser: &prototk.ResolvedVerifier{Lookup: "signer@node1"},
		Payload:  []byte("payload-to-sign"),
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{Verifier: "verifier-value"},
	}
	km.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "signer", mock.Anything, mock.Anything).Return(resolvedKey, nil)
	km.EXPECT().Sign(mock.Anything, resolvedKey, mock.Anything, mock.Anything).Return([]byte("signature"), nil)

	var capturedAttResult *prototk.AttestationResult
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ string, msg *engineProto.EndorsementResponse) {
			capturedAttResult = msg.Endorsement
		}).
		Return(nil)

	event := buildEndorsementEvent("node2")
	event.AttestationRequest = &prototk.AttestationRequest{
		Name:            "att1",
		AttestationType: prototk.AttestationType_ENDORSE,
		PayloadType:     "secp256k1",
	}
	c.handleEndorsementRequest(ctx, event)

	require.NotNil(t, capturedAttResult)
	assert.Equal(t, []byte("signature"), capturedAttResult.Payload)
}

func Test_handleEndorsementRequest_Sign_ResolveKeyError_RetriesThenSendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		WithMockTransportWriter().
		EndorseErrorRetryMaxAttempts(2).
		Build()

	km := setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	// Party key resolution (via setupEndorsementMocks) succeeds. EndorseTransaction returns SIGN.
	// The signer key resolution then fails.
	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_SIGN,
		Endorser: &prototk.ResolvedVerifier{Lookup: "signer@node1"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil).Times(2)
	km.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "signer", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("key error")).Times(2)

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "node2", matchEndorsementErrorMsg("tx-1", "ik-1")).
		Return(nil).
		Once()

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_Sign_SignError_RetriesThenSendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		WithMockTransportWriter().
		EndorseErrorRetryMaxAttempts(2).
		Build()

	km := setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	// Party key resolution (via setupEndorsementMocks) succeeds. EndorseTransaction returns SIGN.
	// The signer key resolution succeeds, but Sign fails.
	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_SIGN,
		Endorser: &prototk.ResolvedVerifier{Lookup: "signer@node1"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil).Times(2)

	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{Verifier: "verifier-value"},
	}
	km.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "signer", mock.Anything, mock.Anything).Return(resolvedKey, nil).Times(2)
	km.EXPECT().Sign(mock.Anything, resolvedKey, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("sign error")).Times(2)

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "node2", matchEndorsementErrorMsg("tx-1", "ik-1")).
		Return(nil).
		Once()

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_Sign_WrongNode_LogsErrorAndSendsResponseUnsigned(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	// Endorser is on node2, not node1 — the SIGN request is not for us.
	// The code logs an error but does not return early: it still calls SendEndorsementResponse
	// with an unsigned (nil Payload) attestation result.
	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_SIGN,
		Endorser: &prototk.ResolvedVerifier{Lookup: "signer@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	// Response is still sent (with empty payload since we didn't sign).
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_SendResponseError_LogsError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		Endorser: &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything).
		Return(fmt.Errorf("transport error"))

	event := buildEndorsementEvent("node2")
	// Should log the error but not panic.
	c.handleEndorsementRequest(ctx, event)
}

// --- common.IsHigherPriority edge cases via the validator ---

func Test_validator_IsEndorsementRequestFromHigherPriorityCoordinator_SenderNotInList_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node1").
		CoordinatorPriorityList("node1", "node2").
		Build()

	// "unknown-node" is not in the priority list; IsHigherPriority returns false.
	event := &EndorsementRequestReceivedEvent{FromNode: "unknown-node"}
	result, err := validator_IsEndorsementRequestFromHigherPriorityCoordinator(ctx, c, event)
	require.NoError(t, err)
	assert.False(t, result)
}

func Test_validator_IsEndorsementRequestFromHigherPriorityCoordinator_ThisNodeNotInList_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	// If this node is not in the priority list either, no one is higher priority.
	c, _ := NewCoordinatorBuilderForTesting(t, State_Active).
		NodeName("node-unknown").
		CoordinatorPriorityList("node1", "node2").
		Build()

	event := &EndorsementRequestReceivedEvent{FromNode: "node1"}
	result, err := validator_IsEndorsementRequestFromHigherPriorityCoordinator(ctx, c, event)
	require.NoError(t, err)
	// node1 is at index 0 and node-unknown is not in the list (treated as len = sentinel high).
	// IsHigherPriority(node1, node-unknown) = 0 < len → true.
	assert.True(t, result)
}

// Test that Persistence mock chaining works (since coordinator_builder uses mp.P for Persistence
// but tests need AllComponents.Persistence() to route through correctly).
func Test_handleEndorsementRequest_UsesContractAddressFromCoordinator(t *testing.T) {
	ctx := t.Context()
	contractAddr := pldtypes.RandAddress()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		ContractAddress(contractAddr).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		Endorser: &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	// Verify the contract address from c.contractAddress is used (not from the event).
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.MatchedBy(func(msg *engineProto.EndorsementResponse) bool {
			return msg.ContractAddress == contractAddr.HexString()
		})).
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

// Verify that IncEndorsedTransactions is called on success.
func Test_handleEndorsementRequest_IncEndorsedTransactionsOnSuccess(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		Endorser: &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

// The endorser named in the domain's response is part of that response, so a locator we cannot parse
// will come back identically on every attempt: it is reported without retrying.
func Test_handleEndorsementRequest_Sign_ValidateEndorserError_SendsEndorsementErrorWithoutRetrying(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	// Endorser.Lookup contains invalid characters — Validate will return an error because
	// PrivateIdentityLocator requires a valid identity@node format when requireNode=true and
	// the node name would be inferred from a locator with no "@" as local-node, so we need
	// something that will actually fail parsing. Using an empty lookup causes a validation error.
	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_SIGN,
		Endorser: &prototk.ResolvedVerifier{Lookup: "@"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil).Once()

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "node2", matchEndorsementErrorMsg("tx-1", "ik-1")).
		Return(nil).
		Once()

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_action_AddEndorsementRequestSenderToEndorserCandidates_AddsSenderNode(t *testing.T) {
	ctx := t.Context()
	c, _ := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		EndorserCandidates("node1").
		CoordinatorPriorityList("node1").
		CoordinatorSelectionMode(prototk.ContractConfig_COORDINATOR_ENDORSER).
		Build()

	event := &EndorsementRequestReceivedEvent{
		FromNode:                  "node2",
		PrivateEndorsementRequest: &components.PrivateTransactionEndorseRequest{},
	}

	require.NoError(t, action_AddEndorsementRequestSenderToEndorserCandidates(ctx, c, event))

	assert.ElementsMatch(t, []string{"node1", "node2"}, c.endorserCandidates)
	assert.Len(t, c.coordinatorPriorityList, 2)
}

func Test_action_HandleEndorsementRequest_IgnoresDuplicateRequestWhileEndorsementInFlight(t *testing.T) {
	// A coordinator nudges an outstanding endorsement request by resending it with the same
	// idempotency key. While the first attempt is still in the domain, the resend must not start a
	// second endorsement of the same transaction.
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		Endorser: &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	var endorseCalls atomic.Int32
	inDomain := make(chan struct{}, 2)
	release := make(chan struct{})
	mocks.DomainAPI.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ mock.Arguments) {
			endorseCalls.Add(1)
			inDomain <- struct{}{}
			<-release
		}).
		Return(endorsementResult, nil)

	var responsesSent atomic.Int32
	responded := make(chan struct{}, 2)
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ string, _ *engineProto.EndorsementResponse) {
			responsesSent.Add(1)
			responded <- struct{}{}
		}).
		Return(nil)

	require.NoError(t, action_HandleEndorsementRequest(ctx, c, buildEndorsementEvent("node2")))
	<-inDomain // the first attempt is now inside the domain call

	// The resend carries the same idempotency key, so it must be dropped without spawning a goroutine.
	// Checking the in-flight set rather than the call counts keeps this deterministic: the claim is
	// taken synchronously by the action, whereas a wrongly spawned goroutine would reach the domain
	// call at some later, unpredictable point.
	require.NoError(t, action_HandleEndorsementRequest(ctx, c, buildEndorsementEvent("node2")))
	assert.Equal(t, []string{"ik-1"}, inFlightEndorsementKeys(c))

	close(release)
	<-responded
	assert.Equal(t, int32(1), endorseCalls.Load())
	assert.Equal(t, int32(1), responsesSent.Load())
}

func Test_action_HandleEndorsementRequest_EndorsesAfreshAfterPreviousRequestCompletes(t *testing.T) {
	// Deduplication only covers endorsements that are still running: once one has finished, a request
	// carrying the same idempotency key is endorsed again, so a response lost in transit can still be
	// recovered by a nudge.
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		Endorser: &prototk.ResolvedVerifier{Lookup: "party1@node2"},
	}
	var endorseCalls atomic.Int32
	mocks.DomainAPI.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ mock.Arguments) { endorseCalls.Add(1) }).
		Return(endorsementResult, nil)

	responded := make(chan struct{}, 2)
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ string, _ *engineProto.EndorsementResponse) { responded <- struct{}{} }).
		Return(nil)

	require.NoError(t, action_HandleEndorsementRequest(ctx, c, buildEndorsementEvent("node2")))
	<-responded
	// The claim is released by the goroutine's defer, which runs after the response is sent
	require.Eventually(t, func() bool { return len(inFlightEndorsementKeys(c)) == 0 }, time.Second, time.Millisecond)

	require.NoError(t, action_HandleEndorsementRequest(ctx, c, buildEndorsementEvent("node2")))
	<-responded
	assert.Equal(t, int32(2), endorseCalls.Load())
}

func Test_action_HandleEndorsementRequest_IgnoresRequestWithNoIdempotencyKey(t *testing.T) {
	// Every endorsement reply echoes the request's idempotency key so the requester can match it, so a
	// request without one cannot be answered usefully and must not reach the domain. The strict mocks
	// fail the test if the domain is called or any reply is sent.
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	event := buildEndorsementEvent("node2")
	event.IdempotencyKey = ""
	require.NoError(t, action_HandleEndorsementRequest(ctx, c, event))

	assert.Empty(t, inFlightEndorsementKeys(c))
}
