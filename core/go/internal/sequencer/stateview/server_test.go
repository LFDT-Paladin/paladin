// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package stateview

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/dependencytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/statevisibilitytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	testStateID   = pldtypes.MustParseHexBytes("0x" + strings.Repeat("aa", 32))
	testSchemaID  = pldtypes.MustParseBytes32("0x" + strings.Repeat("bb", 32))
	testSessionID = "session-1"
)

// serverTestSetup builds a server over a real grapher + visibility tracker holding one labelled,
// CREATE-locked state visible to node1, with a mocked state manager for the match evaluation. A
// session (testSessionID) is opened for node1, freezing that state as its candidate snapshot.
func serverTestSetup(t *testing.T) (Server, *testutil.SentMessageRecorder, *componentsmocks.StateManager) {
	recorder := testutil.NewSentMessageRecorder()
	s, stateManager := serverTestSetupWithWriter(t, recorder)
	return s, recorder, stateManager
}

// serverTestSetupWithWriter is serverTestSetup with a caller-supplied transport writer, for tests
// that exercise the fire-and-forget send-failure paths.
func serverTestSetupWithWriter(t *testing.T, writer transport.TransportWriter) (Server, *componentsmocks.StateManager) {
	ctx := t.Context()
	tracker := statevisibilitytracker.NewStore()
	g := grapher.NewGrapher(dependencytracker.NewDependencyTracker(), tracker, 10)

	states := []*prototk.EndorsableState{{Id: testStateID.String(), SchemaId: testSchemaID.String(), StateDataJson: `{"some":"data"}`}}
	tracker.RecordAssemblyOutput(ctx, states, []*prototk.StateLabels{{}}, [][]string{{"alice@node1"}})
	g.LockMintsOnCreate(ctx, states, uuid.New())

	stateManager := componentsmocks.NewStateManager(t)
	s := NewServer("test-domain", testContractAddress, writer, g, stateManager)
	s.OpenSession(ctx, testSessionID, "node1")
	return s, stateManager
}

// failingWriter wraps the SentMessageRecorder, failing selected sends so the server's
// fire-and-forget warn-only paths can be exercised.
type failingWriter struct {
	*testutil.SentMessageRecorder
	failQueryResponse  bool
	failSpentResponse  bool
	failStateViewError bool
}

func (w *failingWriter) SendQueryAvailableStatesResponse(ctx context.Context, node string, msg *engineProto.QueryAvailableStatesResponse) error {
	if w.failQueryResponse {
		return errors.New("pop")
	}
	return w.SentMessageRecorder.SendQueryAvailableStatesResponse(ctx, node, msg)
}

func (w *failingWriter) SendGetSpentStateIDsResponse(ctx context.Context, node string, msg *engineProto.GetSpentStateIDsResponse) error {
	if w.failSpentResponse {
		return errors.New("pop")
	}
	return w.SentMessageRecorder.SendGetSpentStateIDsResponse(ctx, node, msg)
}

func (w *failingWriter) SendStateViewError(ctx context.Context, node string, msg *engineProto.StateViewError) error {
	if w.failStateViewError {
		return errors.New("pop")
	}
	return w.SentMessageRecorder.SendStateViewError(ctx, node, msg)
}

func TestServer_HandleQueryAvailableStates_ServesMatchingStates(t *testing.T) {
	ctx := t.Context()
	s, recorder, stateManager := serverTestSetup(t)

	// The matcher receives the visible candidates and returns the winners; the response carries
	// their full data plus created.
	stateManager.EXPECT().FindMatchingInMemoryStates(mock.Anything, "test-domain", testSchemaID, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ pldtypes.Bytes32, _ *query.QueryJSON, candidates []*prototk.SnapshotState) ([]*prototk.QueriedState, error) {
			require.Len(t, candidates, 1)
			assert.Equal(t, testStateID.String(), candidates[0].GetState().GetId())
			return []*prototk.QueriedState{{State: candidates[0].GetState(), Created: candidates[0].GetCreated()}}, nil
		}).Once()

	s.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SchemaId:        testSchemaID.String(),
		QueryJson:       `{}`,
		SessionId:       testSessionID,
	})

	require.Empty(t, recorder.SentStateViewErrors())
	responses := recorder.SentQueryAvailableStatesResponses()
	require.Len(t, responses, 1)
	assert.Equal(t, "req1", responses[0].GetRequestId())
	assert.Equal(t, testContractAddress, responses[0].GetContractAddress())
	require.Len(t, responses[0].GetStates(), 1)
	assert.Equal(t, testStateID.String(), responses[0].GetStates()[0].GetState().GetId())
	assert.Equal(t, `{"some":"data"}`, responses[0].GetStates()[0].GetState().GetStateDataJson())
	assert.NotZero(t, responses[0].GetStates()[0].GetCreated())
}

func TestServer_HandleQueryAvailableStates_UnentitledNodeGetsNoCandidates(t *testing.T) {
	ctx := t.Context()
	s, recorder, stateManager := serverTestSetup(t)

	// node2 has no visibility (default-deny): its session snapshot captures zero candidates — an
	// empty response, never an error, and never a data leak.
	s.OpenSession(ctx, "session-2", "node2")
	stateManager.EXPECT().FindMatchingInMemoryStates(mock.Anything, "test-domain", testSchemaID, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ pldtypes.Bytes32, _ *query.QueryJSON, candidates []*prototk.SnapshotState) ([]*prototk.QueriedState, error) {
			assert.Empty(t, candidates)
			return nil, nil
		}).Once()

	s.HandleQueryAvailableStates(ctx, "node2", &engineProto.QueryAvailableStatesRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SchemaId:        testSchemaID.String(),
		QueryJson:       `{}`,
		SessionId:       "session-2",
	})

	require.Empty(t, recorder.SentStateViewErrors())
	responses := recorder.SentQueryAvailableStatesResponses()
	require.Len(t, responses, 1)
	assert.Empty(t, responses[0].GetStates())
}

func TestServer_HandleQueryAvailableStates_BadSchemaID(t *testing.T) {
	ctx := t.Context()
	s, recorder, _ := serverTestSetup(t)

	s.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SchemaId:        "not-a-schema",
		QueryJson:       `{}`,
		SessionId:       testSessionID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Equal(t, "req1", errs[0].GetRequestId())
	assert.Regexp(t, "PD012656", errs[0].GetErrorMessage())
}

func TestServer_HandleQueryAvailableStates_BadQueryJSON(t *testing.T) {
	ctx := t.Context()
	s, recorder, _ := serverTestSetup(t)

	s.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SchemaId:        testSchemaID.String(),
		QueryJson:       `!!!not json`,
		SessionId:       testSessionID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012656", errs[0].GetErrorMessage())
}

func TestServer_HandleQueryAvailableStates_EvaluationError(t *testing.T) {
	ctx := t.Context()
	s, recorder, stateManager := serverTestSetup(t)

	stateManager.EXPECT().FindMatchingInMemoryStates(mock.Anything, "test-domain", testSchemaID, mock.Anything, mock.Anything).
		Return(nil, errors.New("pop")).Once()

	s.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SchemaId:        testSchemaID.String(),
		QueryJson:       `{}`,
		SessionId:       testSessionID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Equal(t, "req1", errs[0].GetRequestId())
	assert.Regexp(t, "pop", errs[0].GetErrorMessage())
}

func TestServer_HandleQueryAvailableStates_UnknownSession(t *testing.T) {
	ctx := t.Context()
	s, recorder, _ := serverTestSetup(t)

	s.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SchemaId:        testSchemaID.String(),
		QueryJson:       `{}`,
		SessionId:       "no-such-session",
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012657", errs[0].GetErrorMessage())
}

func TestServer_HandleQueryAvailableStates_WrongNode(t *testing.T) {
	ctx := t.Context()
	s, recorder, _ := serverTestSetup(t)

	// The session was opened for node1; node2 must not be able to query it.
	s.HandleQueryAvailableStates(ctx, "node2", &engineProto.QueryAvailableStatesRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SchemaId:        testSchemaID.String(),
		QueryJson:       `{}`,
		SessionId:       testSessionID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012658", errs[0].GetErrorMessage())
}

func TestServer_CloseSession_MakesSubsequentQueriesUnknown(t *testing.T) {
	ctx := t.Context()
	s, recorder, _ := serverTestSetup(t)

	s.CloseSession(ctx, testSessionID)

	s.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SchemaId:        testSchemaID.String(),
		QueryJson:       `{}`,
		SessionId:       testSessionID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012657", errs[0].GetErrorMessage())
}

func TestServer_HandleGetSpentStateIDs_ServesFrozenSpentSet(t *testing.T) {
	ctx := t.Context()
	recorder := testutil.NewSentMessageRecorder()
	tracker := statevisibilitytracker.NewStore()
	g := grapher.NewGrapher(dependencytracker.NewDependencyTracker(), tracker, 10)

	spentStateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("cc", 32))
	g.LockMintsOnReadAndSpend(ctx, nil, []*prototk.EndorsableState{{Id: spentStateID.String()}}, uuid.New())

	s := NewServer("test-domain", testContractAddress, recorder, g, componentsmocks.NewStateManager(t))
	s.OpenSession(ctx, testSessionID, "node1")

	// A state spend-locked after the session opened must not appear: the view froze at open.
	lateSpentStateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("dd", 32))
	g.LockMintsOnReadAndSpend(ctx, nil, []*prototk.EndorsableState{{Id: lateSpentStateID.String()}}, uuid.New())

	s.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SessionId:       testSessionID,
	})

	require.Empty(t, recorder.SentStateViewErrors())
	responses := recorder.SentGetSpentStateIDsResponses()
	require.Len(t, responses, 1)
	assert.Equal(t, "req1", responses[0].GetRequestId())
	assert.Equal(t, testContractAddress, responses[0].GetContractAddress())
	require.Len(t, responses[0].GetSpentStateIds(), 1)
	assert.Equal(t, []byte(spentStateID), responses[0].GetSpentStateIds()[0])
}

func TestServer_HandleGetSpentStateIDs_UnknownSession(t *testing.T) {
	ctx := t.Context()
	s, recorder, _ := serverTestSetup(t)

	s.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SessionId:       "unknown-session",
	})

	require.Empty(t, recorder.SentGetSpentStateIDsResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Equal(t, "req1", errs[0].GetRequestId())
	assert.Regexp(t, "PD012657", errs[0].GetErrorMessage())
}

func TestServer_HandleGetSpentStateIDs_WrongNode(t *testing.T) {
	ctx := t.Context()
	s, recorder, _ := serverTestSetup(t)

	s.HandleGetSpentStateIDs(ctx, "node2", &engineProto.GetSpentStateIDsRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SessionId:       testSessionID,
	})

	require.Empty(t, recorder.SentGetSpentStateIDsResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012658", errs[0].GetErrorMessage())
}

func TestServer_OpenSession_ExistingSessionKept(t *testing.T) {
	ctx := t.Context()
	s, recorder, _ := serverTestSetup(t)

	// Re-opening the same session ID for a different node keeps the existing view: the session
	// still belongs to node1, so node2 is rejected.
	s.OpenSession(ctx, testSessionID, "node2")

	s.HandleGetSpentStateIDs(ctx, "node2", &engineProto.GetSpentStateIDsRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SessionId:       testSessionID,
	})
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012658", errs[0].GetErrorMessage())

	// The original owner can still query the session.
	s.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req2",
		SessionId:       testSessionID,
	})
	responses := recorder.SentGetSpentStateIDsResponses()
	require.Len(t, responses, 1)
	assert.Equal(t, "req2", responses[0].GetRequestId())
}

func TestServer_HandleQueryAvailableStates_ResponseSendFailureIsWarnOnly(t *testing.T) {
	ctx := t.Context()
	writer := &failingWriter{SentMessageRecorder: testutil.NewSentMessageRecorder(), failQueryResponse: true}
	s, stateManager := serverTestSetupWithWriter(t, writer)

	stateManager.EXPECT().FindMatchingInMemoryStates(mock.Anything, "test-domain", testSchemaID, mock.Anything, mock.Anything).
		Return([]*prototk.QueriedState{}, nil).Once()

	// The response send fails: fire-and-forget, the server only logs (the client will retry).
	s.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SchemaId:        testSchemaID.String(),
		QueryJson:       `{}`,
		SessionId:       testSessionID,
	})

	require.Empty(t, writer.SentStateViewErrors())
	require.Empty(t, writer.SentQueryAvailableStatesResponses())
}

func TestServer_HandleGetSpentStateIDs_ResponseSendFailureIsWarnOnly(t *testing.T) {
	ctx := t.Context()
	writer := &failingWriter{SentMessageRecorder: testutil.NewSentMessageRecorder(), failSpentResponse: true}
	s, _ := serverTestSetupWithWriter(t, writer)

	// The response send fails: fire-and-forget, the server only logs (the client will retry).
	s.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SessionId:       testSessionID,
	})

	require.Empty(t, writer.SentStateViewErrors())
	require.Empty(t, writer.SentGetSpentStateIDsResponses())
}

func TestServer_SendError_SendFailureIsWarnOnly(t *testing.T) {
	ctx := t.Context()
	writer := &failingWriter{SentMessageRecorder: testutil.NewSentMessageRecorder(), failStateViewError: true}
	s, _ := serverTestSetupWithWriter(t, writer)

	// An unknown session triggers sendError; the error send itself fails and is only logged.
	s.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress: testContractAddress,
		RequestId:       "req1",
		SessionId:       "unknown-session",
	})

	require.Empty(t, writer.SentGetSpentStateIDsResponses())
	require.Empty(t, writer.SentStateViewErrors())
}
