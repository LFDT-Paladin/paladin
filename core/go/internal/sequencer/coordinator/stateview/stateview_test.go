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

const testContractAddress = "0x1234567890123456789012345678901234567890"

var (
	testStateID           = pldtypes.MustParseHexBytes("0x" + strings.Repeat("aa", 32))
	testSchemaID          = pldtypes.MustParseBytes32("0x" + strings.Repeat("bb", 32))
	testAssembleRequestID = "assemble-1"
)

// providerTestSetup builds a provider over a real grapher + visibility tracker holding one labelled,
// CREATE-locked state visible to node1, with a mocked state manager for the match evaluation. A
// view is captured for node1 under testAssembleRequestID, freezing that state as its candidate snapshot.
func providerTestSetup(t *testing.T) (Provider, *testutil.SentMessageRecorder, *componentsmocks.StateManager) {
	recorder := testutil.NewSentMessageRecorder()
	p, stateManager := providerTestSetupWithWriter(t, recorder)
	return p, recorder, stateManager
}

// providerTestSetupWithWriter is providerTestSetup with a caller-supplied transport writer, for tests
// that exercise the fire-and-forget send-failure paths.
func providerTestSetupWithWriter(t *testing.T, writer transport.TransportWriter) (Provider, *componentsmocks.StateManager) {
	ctx := t.Context()
	tracker := statevisibilitytracker.NewStore()
	g := grapher.NewGrapher(dependencytracker.NewDependencyTracker(), tracker, 10)

	states := []*prototk.EndorsableState{{Id: testStateID.String(), SchemaId: testSchemaID.String(), StateDataJson: `{"some":"data"}`}}
	tracker.RecordAssemblyOutput(ctx, states, []*prototk.StateLabels{{}}, [][]string{{"alice@node1"}})
	g.LockMintsOnCreate(ctx, states, uuid.New())

	stateManager := componentsmocks.NewStateManager(t)
	p := NewProvider("test-domain", testContractAddress, writer, g, stateManager)
	p.OpenView(ctx, testAssembleRequestID, "node1")
	return p, stateManager
}

// failingWriter wraps the SentMessageRecorder, failing selected sends so the provider's
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

func TestProvider_HandleQueryAvailableStates_ServesMatchingStates(t *testing.T) {
	ctx := t.Context()
	p, recorder, stateManager := providerTestSetup(t)

	// The matcher receives the visible candidates and returns the winners; the response carries
	// their full data plus created.
	stateManager.EXPECT().FindMatchingInMemoryStates(mock.Anything, "test-domain", testSchemaID, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ pldtypes.Bytes32, _ *query.QueryJSON, candidates []*prototk.SnapshotState) ([]*prototk.QueriedState, error) {
			require.Len(t, candidates, 1)
			assert.Equal(t, testStateID.String(), candidates[0].GetState().GetId())
			return []*prototk.QueriedState{{State: candidates[0].GetState(), Created: candidates[0].GetCreated()}}, nil
		}).Once()

	p.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		SchemaId:          testSchemaID.String(),
		QueryJson:         `{}`,
		AssembleRequestId: testAssembleRequestID,
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

func TestProvider_HandleQueryAvailableStates_UnentitledNodeGetsNoCandidates(t *testing.T) {
	ctx := t.Context()
	p, recorder, stateManager := providerTestSetup(t)

	// node2 has no visibility (default-deny): its snapshot captures zero candidates — an
	// empty response, never an error, and never a data leak.
	p.OpenView(ctx, "assemble-2", "node2")
	stateManager.EXPECT().FindMatchingInMemoryStates(mock.Anything, "test-domain", testSchemaID, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ pldtypes.Bytes32, _ *query.QueryJSON, candidates []*prototk.SnapshotState) ([]*prototk.QueriedState, error) {
			assert.Empty(t, candidates)
			return nil, nil
		}).Once()

	p.HandleQueryAvailableStates(ctx, "node2", &engineProto.QueryAvailableStatesRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		SchemaId:          testSchemaID.String(),
		QueryJson:         `{}`,
		AssembleRequestId: "assemble-2",
	})

	require.Empty(t, recorder.SentStateViewErrors())
	responses := recorder.SentQueryAvailableStatesResponses()
	require.Len(t, responses, 1)
	assert.Empty(t, responses[0].GetStates())
}

func TestProvider_HandleQueryAvailableStates_BadSchemaID(t *testing.T) {
	ctx := t.Context()
	p, recorder, _ := providerTestSetup(t)

	p.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		SchemaId:          "not-a-schema",
		QueryJson:         `{}`,
		AssembleRequestId: testAssembleRequestID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Equal(t, "req1", errs[0].GetRequestId())
	assert.Regexp(t, "PD012650", errs[0].GetErrorMessage())
}

func TestProvider_HandleQueryAvailableStates_BadQueryJSON(t *testing.T) {
	ctx := t.Context()
	p, recorder, _ := providerTestSetup(t)

	p.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		SchemaId:          testSchemaID.String(),
		QueryJson:         `!!!not json`,
		AssembleRequestId: testAssembleRequestID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012650", errs[0].GetErrorMessage())
}

func TestProvider_HandleQueryAvailableStates_EvaluationError(t *testing.T) {
	ctx := t.Context()
	p, recorder, stateManager := providerTestSetup(t)

	stateManager.EXPECT().FindMatchingInMemoryStates(mock.Anything, "test-domain", testSchemaID, mock.Anything, mock.Anything).
		Return(nil, errors.New("pop")).Once()

	p.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		SchemaId:          testSchemaID.String(),
		QueryJson:         `{}`,
		AssembleRequestId: testAssembleRequestID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Equal(t, "req1", errs[0].GetRequestId())
	assert.Regexp(t, "pop", errs[0].GetErrorMessage())
}

func TestProvider_HandleQueryAvailableStates_UnknownAssemble(t *testing.T) {
	ctx := t.Context()
	p, recorder, _ := providerTestSetup(t)

	p.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		SchemaId:          testSchemaID.String(),
		QueryJson:         `{}`,
		AssembleRequestId: "no-such-assemble",
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012651", errs[0].GetErrorMessage())
}

func TestProvider_HandleQueryAvailableStates_WrongNode(t *testing.T) {
	ctx := t.Context()
	p, recorder, _ := providerTestSetup(t)

	// The view was captured for node1; node2 must not be able to query it.
	p.HandleQueryAvailableStates(ctx, "node2", &engineProto.QueryAvailableStatesRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		SchemaId:          testSchemaID.String(),
		QueryJson:         `{}`,
		AssembleRequestId: testAssembleRequestID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012652", errs[0].GetErrorMessage())
}

func TestProvider_CloseView_MakesSubsequentQueriesUnknown(t *testing.T) {
	ctx := t.Context()
	p, recorder, _ := providerTestSetup(t)

	p.CloseView(ctx, testAssembleRequestID)

	p.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		SchemaId:          testSchemaID.String(),
		QueryJson:         `{}`,
		AssembleRequestId: testAssembleRequestID,
	})

	require.Empty(t, recorder.SentQueryAvailableStatesResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012651", errs[0].GetErrorMessage())
}

func TestProvider_HandleGetSpentStateIDs_ServesFrozenSpentSet(t *testing.T) {
	ctx := t.Context()
	recorder := testutil.NewSentMessageRecorder()
	tracker := statevisibilitytracker.NewStore()
	g := grapher.NewGrapher(dependencytracker.NewDependencyTracker(), tracker, 10)

	spentStateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("cc", 32))
	g.LockMintsOnReadAndSpend(ctx, nil, []*prototk.EndorsableState{{Id: spentStateID.String()}}, uuid.New())

	p := NewProvider("test-domain", testContractAddress, recorder, g, componentsmocks.NewStateManager(t))
	p.OpenView(ctx, testAssembleRequestID, "node1")

	// A state spend-locked after the view was captured must not appear: the view froze at capture.
	lateSpentStateID := pldtypes.MustParseHexBytes("0x" + strings.Repeat("dd", 32))
	g.LockMintsOnReadAndSpend(ctx, nil, []*prototk.EndorsableState{{Id: lateSpentStateID.String()}}, uuid.New())

	p.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		AssembleRequestId: testAssembleRequestID,
	})

	require.Empty(t, recorder.SentStateViewErrors())
	responses := recorder.SentGetSpentStateIDsResponses()
	require.Len(t, responses, 1)
	assert.Equal(t, "req1", responses[0].GetRequestId())
	assert.Equal(t, testContractAddress, responses[0].GetContractAddress())
	require.Len(t, responses[0].GetSpentStateIds(), 1)
	assert.Equal(t, []byte(spentStateID), responses[0].GetSpentStateIds()[0])
}

func TestProvider_HandleGetSpentStateIDs_UnknownAssemble(t *testing.T) {
	ctx := t.Context()
	p, recorder, _ := providerTestSetup(t)

	p.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		AssembleRequestId: "unknown-assemble",
	})

	require.Empty(t, recorder.SentGetSpentStateIDsResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Equal(t, "req1", errs[0].GetRequestId())
	assert.Regexp(t, "PD012651", errs[0].GetErrorMessage())
}

func TestProvider_HandleGetSpentStateIDs_WrongNode(t *testing.T) {
	ctx := t.Context()
	p, recorder, _ := providerTestSetup(t)

	p.HandleGetSpentStateIDs(ctx, "node2", &engineProto.GetSpentStateIDsRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		AssembleRequestId: testAssembleRequestID,
	})

	require.Empty(t, recorder.SentGetSpentStateIDsResponses())
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012652", errs[0].GetErrorMessage())
}

func TestProvider_OpenView_ExistingViewKept(t *testing.T) {
	ctx := t.Context()
	p, recorder, _ := providerTestSetup(t)

	// Re-opening the same assemble request ID for a different node keeps the existing view: it
	// still belongs to node1, so node2 is rejected.
	p.OpenView(ctx, testAssembleRequestID, "node2")

	p.HandleGetSpentStateIDs(ctx, "node2", &engineProto.GetSpentStateIDsRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		AssembleRequestId: testAssembleRequestID,
	})
	errs := recorder.SentStateViewErrors()
	require.Len(t, errs, 1)
	assert.Regexp(t, "PD012652", errs[0].GetErrorMessage())

	// The original owner can still query the view.
	p.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req2",
		AssembleRequestId: testAssembleRequestID,
	})
	responses := recorder.SentGetSpentStateIDsResponses()
	require.Len(t, responses, 1)
	assert.Equal(t, "req2", responses[0].GetRequestId())
}

func TestProvider_HandleQueryAvailableStates_ResponseSendFailureIsWarnOnly(t *testing.T) {
	ctx := t.Context()
	writer := &failingWriter{SentMessageRecorder: testutil.NewSentMessageRecorder(), failQueryResponse: true}
	p, stateManager := providerTestSetupWithWriter(t, writer)

	stateManager.EXPECT().FindMatchingInMemoryStates(mock.Anything, "test-domain", testSchemaID, mock.Anything, mock.Anything).
		Return([]*prototk.QueriedState{}, nil).Once()

	// The response send fails: fire-and-forget, the provider only logs (the reader will retry).
	p.HandleQueryAvailableStates(ctx, "node1", &engineProto.QueryAvailableStatesRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		SchemaId:          testSchemaID.String(),
		QueryJson:         `{}`,
		AssembleRequestId: testAssembleRequestID,
	})

	require.Empty(t, writer.SentStateViewErrors())
	require.Empty(t, writer.SentQueryAvailableStatesResponses())
}

func TestProvider_HandleGetSpentStateIDs_ResponseSendFailureIsWarnOnly(t *testing.T) {
	ctx := t.Context()
	writer := &failingWriter{SentMessageRecorder: testutil.NewSentMessageRecorder(), failSpentResponse: true}
	p, _ := providerTestSetupWithWriter(t, writer)

	// The response send fails: fire-and-forget, the provider only logs (the reader will retry).
	p.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		AssembleRequestId: testAssembleRequestID,
	})

	require.Empty(t, writer.SentStateViewErrors())
	require.Empty(t, writer.SentGetSpentStateIDsResponses())
}

func TestProvider_SendError_SendFailureIsWarnOnly(t *testing.T) {
	ctx := t.Context()
	writer := &failingWriter{SentMessageRecorder: testutil.NewSentMessageRecorder(), failStateViewError: true}
	p, _ := providerTestSetupWithWriter(t, writer)

	// An unknown assemble triggers sendError; the error send itself fails and is only logged.
	p.HandleGetSpentStateIDs(ctx, "node1", &engineProto.GetSpentStateIDsRequest{
		ContractAddress:   testContractAddress,
		RequestId:         "req1",
		AssembleRequestId: "unknown-assemble",
	})

	require.Empty(t, writer.SentGetSpentStateIDsResponses())
	require.Empty(t, writer.SentStateViewErrors())
}
