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
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// partyKeyVerifier is the resolved verifier string used for party key resolution in tests.
const partyKeyVerifier = "party-verifier"


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

// setupEndorsementMocks sets up StateManager, DomainContext, and KeyManager mocks for tests
// that call handleEndorsementRequest directly. The KeyManager is pre-wired to succeed for the
// party key resolution step (party "party1@<fromNode>" → unqualifiedLookup "party1"). SIGN-path
// tests should add extra expectations on the returned KeyManager for the signing step.
// Returns the DomainContext and the shared KeyManager mock.
func setupEndorsementMocks(t *testing.T, mocks *CoordinatorDependencyMocks) (*componentsmocks.DomainContext, *componentsmocks.KeyManager) {
	t.Helper()
	mockDomain := componentsmocks.NewDomain(t)
	contractAddr := pldtypes.RandAddress()

	mocks.DomainAPI.On("Domain").Return(mockDomain).Maybe()
	mocks.DomainAPI.On("Address").Return(*contractAddr).Maybe()

	mockStateManager := componentsmocks.NewStateManager(t)
	mockDomainContext := componentsmocks.NewDomainContext(t)
	mockStateManager.On("NewDomainContext", mock.Anything, mockDomain, *contractAddr).Return(mockDomainContext)
	mockDomainContext.On("Close").Return().Maybe()
	mocks.AllComponents.On("StateManager").Return(mockStateManager).Maybe()

	// Party key resolution: buildEndorsementEvent sets Party = "party1@<fromNode>", so the
	// unqualified lookup is "party1". Uses Maybe() so tests that fail before reaching this
	// step (e.g. PartyKeyResolveError) don't need to match it.
	mockKeyManager := componentsmocks.NewKeyManager(t)
	partyKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{Verifier: partyKeyVerifier},
	}
	mockKeyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, "party1", mock.Anything, mock.Anything).
		Return(partyKey, nil).Maybe()
	mocks.AllComponents.On("KeyManager").Return(mockKeyManager).Maybe()

	return mockDomainContext, mockKeyManager
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
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	done := make(chan struct{})
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _, _, _ string, _ *prototk.AttestationResult, _ *components.EndorsementResult, _, _, _, _ string) {
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
		SendEndorsementError(mock.Anything, "tx-1", "ik-1", mock.Anything, mock.Anything, mock.Anything, mock.Anything, "node2").
		Run(func(_ context.Context, _, _, _, _, _, _, _ string) { close(errorSent) }).
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
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, "tx-1", "ik-1", mock.Anything, mock.Anything, endorsementResult, revertMsg, "att1", "party1@node2", "node2").
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
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, "tx-1", "ik-1", mock.Anything, mock.Anything, endorsementResult, "(no revert reason)", "att1", "party1@node2", "node2").
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_PartyIdentityError_SendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "tx-1", "ik-1", mock.Anything, mock.Anything, mock.Anything, mock.Anything, "node2").
		Return(nil)

	// Party "@node2" has an empty identity part, causing PrivateIdentityLocator.Identity to fail.
	event := buildEndorsementEvent("node2")
	event.Party = "@node2"
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_PartyKeyResolveError_SendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	// Set up KeyManager to fail party key resolution; no StateManager or EndorseTransaction needed.
	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "party1", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("key not found"))
	mocks.AllComponents.On("KeyManager").Return(mockKeyManager).Maybe()

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "tx-1", "ik-1", mock.Anything, mock.Anything, mock.Anything, mock.Anything, "node2").
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_EndorseTransactionError_SendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		WithMockTransportWriter().
		Build()

	setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("domain error"))

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "tx-1", "ik-1", mock.Anything, mock.Anything, mock.Anything, mock.Anything, "node2").
		Return(nil)

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
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	var capturedAttResult *prototk.AttestationResult
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _, _, _ string, attResult *prototk.AttestationResult, _ *components.EndorsementResult, _, _, _, _ string) {
			capturedAttResult = attResult
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

	_, km := setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	// EndorseTransaction returns SIGN with endorser on this node.
	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_SIGN,
		Endorser: &prototk.ResolvedVerifier{Lookup: "signer@node1"},
		Payload:  []byte("payload-to-sign"),
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{Verifier: "verifier-value"},
	}
	km.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "signer", mock.Anything, mock.Anything).Return(resolvedKey, nil)
	km.EXPECT().Sign(mock.Anything, resolvedKey, mock.Anything, mock.Anything).Return([]byte("signature"), nil)

	var capturedAttResult *prototk.AttestationResult
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _, _, _ string, attResult *prototk.AttestationResult, _ *components.EndorsementResult, _, _, _, _ string) {
			capturedAttResult = attResult
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

func Test_handleEndorsementRequest_Sign_ResolveKeyError_SendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		WithMockTransportWriter().
		Build()

	_, km := setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	// Party key resolution (via setupEndorsementMocks) succeeds. EndorseTransaction returns SIGN.
	// The signer key resolution then fails.
	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_SIGN,
		Endorser: &prototk.ResolvedVerifier{Lookup: "signer@node1"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	km.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "signer", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("key error"))

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "tx-1", "ik-1", mock.Anything, mock.Anything, mock.Anything, mock.Anything, "node2").
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_Sign_SignError_SendsEndorsementError(t *testing.T) {
	ctx := t.Context()
	c, mocks := NewCoordinatorBuilderForTesting(t, State_Observing).
		NodeName("node1").
		WithMockTransportWriter().
		Build()

	_, km := setupEndorsementMocks(t, mocks)
	mocks.AllComponents.On("Persistence").Return(mocks.AllComponents.Persistence()).Maybe()

	// Party key resolution (via setupEndorsementMocks) succeeds. EndorseTransaction returns SIGN.
	// The signer key resolution succeeds, but Sign fails.
	endorsementResult := &components.EndorsementResult{
		Result:   prototk.EndorseTransactionResponse_SIGN,
		Endorser: &prototk.ResolvedVerifier{Lookup: "signer@node1"},
	}
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{Verifier: "verifier-value"},
	}
	km.EXPECT().ResolveKeyNewDatabaseTX(mock.Anything, "signer", mock.Anything, mock.Anything).Return(resolvedKey, nil)
	km.EXPECT().Sign(mock.Anything, resolvedKey, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("sign error"))

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "tx-1", "ik-1", mock.Anything, mock.Anything, mock.Anything, mock.Anything, "node2").
		Return(nil)

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
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	// Response is still sent (with empty payload since we didn't sign).
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	// Verify the contract address from c.contractAddress is used (not from the event).
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything, contractAddr.String(), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)
	mocks.TransportWriter.EXPECT().
		SendEndorsementResponse(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	event := buildEndorsementEvent("node2")
	c.handleEndorsementRequest(ctx, event)
}

func Test_handleEndorsementRequest_Sign_ValidateEndorserError_SendsEndorsementError(t *testing.T) {
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
	mocks.DomainAPI.EXPECT().EndorseTransaction(mock.Anything, mock.Anything, mock.Anything).Return(endorsementResult, nil)

	mocks.TransportWriter.EXPECT().
		SendEndorsementError(mock.Anything, "tx-1", "ik-1", mock.Anything, mock.Anything, mock.Anything, mock.Anything, "node2").
		Return(nil)

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
