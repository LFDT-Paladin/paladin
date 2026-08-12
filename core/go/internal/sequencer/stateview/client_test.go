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
	"sync"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testContractAddress = "0x1234567890123456789012345678901234567890"

// fakeClock is a manually-driven common.Clock for deterministic retry tests.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []func()
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Now()}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) HasExpired(t time.Time, d time.Duration) bool {
	return c.Now().After(t.Add(d))
}

func (c *fakeClock) Duration(milliseconds int) time.Duration {
	return time.Duration(milliseconds) * time.Millisecond
}

func (c *fakeClock) ScheduleTimer(_ context.Context, _ time.Duration, fn func()) (cancel func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timers = append(c.timers, fn)
	return func() {}
}

// fireTimers runs (and clears) the scheduled timer callbacks, waiting for at least one to be
// scheduled first — the retry loop schedules its timer just after sending, so a test that
// synchronised on the send may get here before the timer registration.
func (c *fakeClock) fireTimers() {
	for {
		c.mu.Lock()
		timers := c.timers
		c.timers = nil
		c.mu.Unlock()
		if len(timers) > 0 {
			for _, fn := range timers {
				fn()
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// signallingWriter wraps the SentMessageRecorder to hand each state view request to the test on a
// channel, so tests can synchronise with the request goroutine (and optionally inject send errors).
type signallingWriter struct {
	*testutil.SentMessageRecorder
	sent      chan *engineProto.QueryAvailableStatesRequest
	sentSpent chan *engineProto.GetSpentStateIDsRequest
	sendErr   error
}

func newSignallingWriter() *signallingWriter {
	return &signallingWriter{
		SentMessageRecorder: testutil.NewSentMessageRecorder(),
		sent:                make(chan *engineProto.QueryAvailableStatesRequest, 16),
		sentSpent:           make(chan *engineProto.GetSpentStateIDsRequest, 16),
	}
}

func (w *signallingWriter) SendQueryAvailableStatesRequest(_ context.Context, _ string, msg *engineProto.QueryAvailableStatesRequest) error {
	w.sent <- msg
	return w.sendErr
}

func (w *signallingWriter) SendGetSpentStateIDsRequest(_ context.Context, _ string, msg *engineProto.GetSpentStateIDsRequest) error {
	w.sentSpent <- msg
	return w.sendErr
}

type queryOutcome struct {
	states []*prototk.QueriedState
	err    error
}

// startQuery runs QueryAvailableStates on a goroutine and returns the channel its outcome arrives on.
func startQuery(ctx context.Context, querier components.RemoteStateView, schemaID, queryJSON string) chan queryOutcome {
	done := make(chan queryOutcome, 1)
	go func() {
		states, err := querier.QueryAvailableStates(ctx, schemaID, queryJSON)
		done <- queryOutcome{states: states, err: err}
	}()
	return done
}

func newTestClient(t *testing.T) (Client, *signallingWriter, *fakeClock) {
	writer := newSignallingWriter()
	clock := newFakeClock()
	return NewClient(testContractAddress, writer, 3*time.Second, clock), writer, clock
}

func TestClient_QueryAvailableStates_Success(t *testing.T) {
	ctx := t.Context()
	c, writer, _ := newTestClient(t)
	querier := c.ForCoordinator("coordinator-node", "session-1")

	done := startQuery(ctx, querier, "0xschema", `{"eq":[{"field":"amount","value":10}]}`)
	req := <-writer.sent
	assert.Equal(t, testContractAddress, req.GetContractAddress())
	assert.Equal(t, "0xschema", req.GetSchemaId())
	assert.Equal(t, `{"eq":[{"field":"amount","value":10}]}`, req.GetQueryJson())
	assert.Equal(t, "session-1", req.GetSessionId())
	require.NotEmpty(t, req.GetRequestId())

	c.HandleQueryAvailableStatesResponse(ctx, "coordinator-node", &engineProto.QueryAvailableStatesResponse{
		RequestId: req.GetRequestId(),
		States: []*prototk.QueriedState{
			{State: &prototk.EndorsableState{Id: "0xaaaa"}, Created: 1000},
			{State: &prototk.EndorsableState{Id: "0xbbbb"}, Created: 2000},
		},
	})

	outcome := <-done
	require.NoError(t, outcome.err)
	require.Len(t, outcome.states, 2)
	assert.Equal(t, "0xaaaa", outcome.states[0].GetState().GetId())
	assert.Equal(t, int64(1000), outcome.states[0].GetCreated())
}

func TestClient_QueryAvailableStates_Error(t *testing.T) {
	ctx := t.Context()
	c, writer, _ := newTestClient(t)
	querier := c.ForCoordinator("coordinator-node", "session-1")

	done := startQuery(ctx, querier, "0xschema", `{}`)
	req := <-writer.sent

	c.HandleError(ctx, "coordinator-node", &engineProto.StateViewError{
		RequestId:    req.GetRequestId(),
		ErrorMessage: "no states for you",
	})

	outcome := <-done
	require.Error(t, outcome.err)
	assert.Regexp(t, "PD012655", outcome.err)
	assert.Regexp(t, "no states for you", outcome.err)
}

func TestClient_QueryAvailableStates_RetriesSameRequestID(t *testing.T) {
	ctx := t.Context()
	c, writer, clock := newTestClient(t)
	querier := c.ForCoordinator("coordinator-node", "session-1")

	done := startQuery(ctx, querier, "0xschema", `{}`)
	req1 := <-writer.sent

	// The retry timer fires: the SAME request ID is re-sent (idempotent request).
	clock.fireTimers()
	req2 := <-writer.sent
	assert.Equal(t, req1.GetRequestId(), req2.GetRequestId())
	assert.Equal(t, req1.GetQueryJson(), req2.GetQueryJson())

	c.HandleQueryAvailableStatesResponse(ctx, "coordinator-node", &engineProto.QueryAvailableStatesResponse{
		RequestId: req2.GetRequestId(),
		States:    []*prototk.QueriedState{{State: &prototk.EndorsableState{Id: "0xaaaa"}}},
	})
	outcome := <-done
	require.NoError(t, outcome.err)
	require.Len(t, outcome.states, 1)
}

func TestClient_QueryAvailableStates_SendErrorIsRetried(t *testing.T) {
	ctx := t.Context()
	c, writer, clock := newTestClient(t)
	writer.sendErr = assert.AnError // every send "fails" — the query must keep waiting and retrying
	querier := c.ForCoordinator("coordinator-node", "session-1")

	done := startQuery(ctx, querier, "0xschema", `{}`)
	req1 := <-writer.sent
	clock.fireTimers()
	req2 := <-writer.sent
	assert.Equal(t, req1.GetRequestId(), req2.GetRequestId())

	// A response still completes the query even though sends were erroring.
	c.HandleQueryAvailableStatesResponse(ctx, "coordinator-node", &engineProto.QueryAvailableStatesResponse{
		RequestId: req1.GetRequestId(),
	})
	outcome := <-done
	require.NoError(t, outcome.err)
	assert.Empty(t, outcome.states)
}

func TestClient_HandleQueryAvailableStatesResponse_WrongNodeDropped(t *testing.T) {
	ctx := t.Context()
	c, writer, _ := newTestClient(t)
	querier := c.ForCoordinator("coordinator-node", "session-1")

	done := startQuery(ctx, querier, "0xschema", `{}`)
	req := <-writer.sent

	// A response from a node other than the one queried must be dropped — the query stays pending.
	c.HandleQueryAvailableStatesResponse(ctx, "impostor-node", &engineProto.QueryAvailableStatesResponse{
		RequestId: req.GetRequestId(),
		States:    []*prototk.QueriedState{{State: &prototk.EndorsableState{Id: "0xevil"}}},
	})
	c.HandleError(ctx, "impostor-node", &engineProto.StateViewError{
		RequestId:    req.GetRequestId(),
		ErrorMessage: "boom",
	})
	select {
	case <-done:
		t.Fatal("query must not complete from a different node's response")
	case <-time.After(20 * time.Millisecond):
	}

	// The real coordinator's response completes it.
	c.HandleQueryAvailableStatesResponse(ctx, "coordinator-node", &engineProto.QueryAvailableStatesResponse{RequestId: req.GetRequestId()})
	outcome := <-done
	require.NoError(t, outcome.err)
}

func TestClient_HandleQueryAvailableStatesResponse_UnknownRequestDropped(t *testing.T) {
	ctx := t.Context()
	c, _, _ := newTestClient(t)
	// No pending query at all — must not panic.
	c.HandleQueryAvailableStatesResponse(ctx, "coordinator-node", &engineProto.QueryAvailableStatesResponse{RequestId: "unknown"})
	c.HandleError(ctx, "coordinator-node", &engineProto.StateViewError{RequestId: "unknown"})
}

func TestClient_HandleQueryAvailableStatesResponse_DuplicateDropped(t *testing.T) {
	ctx := t.Context()
	c, writer, _ := newTestClient(t)
	querier := c.ForCoordinator("coordinator-node", "session-1")

	done := startQuery(ctx, querier, "0xschema", `{}`)
	req := <-writer.sent

	// Two results for the same request: only the first is kept, the duplicate is dropped without
	// blocking the transport goroutine.
	resp := &engineProto.QueryAvailableStatesResponse{
		RequestId: req.GetRequestId(),
		States:    []*prototk.QueriedState{{State: &prototk.EndorsableState{Id: "0xaaaa"}}},
	}
	// Grab the pending entry so the duplicate delivery races against an undrained channel.
	cImpl := c.(*client)
	cImpl.mu.Lock()
	pq := cImpl.pending[req.GetRequestId()]
	cImpl.mu.Unlock()
	require.NotNil(t, pq)
	cImpl.deliver(ctx, "coordinator-node", req.GetRequestId(), &requestResult{states: resp.GetStates()})
	cImpl.deliver(ctx, "coordinator-node", req.GetRequestId(), &requestResult{states: nil})

	outcome := <-done
	require.NoError(t, outcome.err)
	require.Len(t, outcome.states, 1, "the first delivered result wins")
}

func TestClient_QueryAvailableStates_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	c, writer, _ := newTestClient(t)
	querier := c.ForCoordinator("coordinator-node", "session-1")

	done := startQuery(ctx, querier, "0xschema", `{}`)
	<-writer.sent
	cancel()

	outcome := <-done
	require.ErrorIs(t, outcome.err, context.Canceled)
}

func TestClient_Deliver_DuplicateWithFullBufferDropped(t *testing.T) {
	ctx := t.Context()
	c, _, _ := newTestClient(t)
	cImpl := c.(*client)

	// Register a pending request directly (no consumer goroutine), so the second delivery
	// deterministically finds the capacity-1 result channel still full.
	pr := &pendingRequest{coordinatorNode: "coordinator-node", resultCh: make(chan *requestResult, 1)}
	cImpl.mu.Lock()
	cImpl.pending["req1"] = pr
	cImpl.mu.Unlock()

	first := &requestResult{states: []*prototk.QueriedState{{State: &prototk.EndorsableState{Id: "0xaaaa"}}}}
	cImpl.deliver(ctx, "coordinator-node", "req1", first)
	// The buffer is full: the duplicate must be dropped without blocking the transport goroutine.
	cImpl.deliver(ctx, "coordinator-node", "req1", &requestResult{})

	result := <-pr.resultCh
	assert.Same(t, first, result, "the first delivered result wins")
	select {
	case <-pr.resultCh:
		t.Fatal("duplicate result must not be buffered")
	default:
	}
}

type spentIDsOutcome struct {
	ids []pldtypes.HexBytes
	err error
}

// startGetSpentStateIDs runs GetSpentStateIDs on a goroutine and returns the channel its outcome
// arrives on.
func startGetSpentStateIDs(ctx context.Context, view components.RemoteStateView) chan spentIDsOutcome {
	done := make(chan spentIDsOutcome, 1)
	go func() {
		ids, err := view.GetSpentStateIDs(ctx)
		done <- spentIDsOutcome{ids: ids, err: err}
	}()
	return done
}

func TestClient_GetSpentStateIDs_Success(t *testing.T) {
	ctx := t.Context()
	c, writer, _ := newTestClient(t)
	view := c.ForCoordinator("coordinator-node", "session-1")

	done := startGetSpentStateIDs(ctx, view)
	req := <-writer.sentSpent
	assert.Equal(t, testContractAddress, req.GetContractAddress())
	assert.Equal(t, "session-1", req.GetSessionId())

	spentA := []byte{0x01, 0x02}
	spentB := []byte{0x03, 0x04}
	c.HandleGetSpentStateIDsResponse(ctx, "coordinator-node", &engineProto.GetSpentStateIDsResponse{
		RequestId:     req.GetRequestId(),
		SpentStateIds: [][]byte{spentA, spentB},
	})

	outcome := <-done
	require.NoError(t, outcome.err)
	require.Len(t, outcome.ids, 2)
	assert.Equal(t, pldtypes.HexBytes(spentA), outcome.ids[0])
	assert.Equal(t, pldtypes.HexBytes(spentB), outcome.ids[1])
}

func TestClient_GetSpentStateIDs_CachedAfterFirstFetch(t *testing.T) {
	ctx := t.Context()
	c, writer, _ := newTestClient(t)
	view := c.ForCoordinator("coordinator-node", "session-1")

	done := startGetSpentStateIDs(ctx, view)
	req := <-writer.sentSpent

	spentA := []byte{0x01, 0x02}
	c.HandleGetSpentStateIDsResponse(ctx, "coordinator-node", &engineProto.GetSpentStateIDsResponse{
		RequestId:     req.GetRequestId(),
		SpentStateIds: [][]byte{spentA},
	})
	outcome := <-done
	require.NoError(t, outcome.err)
	require.Len(t, outcome.ids, 1)

	// A second call returns the cached set synchronously, without sending another request.
	ids, err := view.GetSpentStateIDs(ctx)
	require.NoError(t, err)
	require.Len(t, ids, 1)
	assert.Equal(t, pldtypes.HexBytes(spentA), ids[0])
	select {
	case <-writer.sentSpent:
		t.Fatal("second GetSpentStateIDs must not send another request")
	default:
	}
}

func TestClient_GetSpentStateIDs_ErrorFailsFast(t *testing.T) {
	ctx := t.Context()
	c, writer, _ := newTestClient(t)
	view := c.ForCoordinator("coordinator-node", "session-1")

	done := startGetSpentStateIDs(ctx, view)
	req := <-writer.sentSpent

	c.HandleError(ctx, "coordinator-node", &engineProto.StateViewError{
		RequestId:    req.GetRequestId(),
		ErrorMessage: "no such session",
	})

	outcome := <-done
	require.ErrorContains(t, outcome.err, "PD012655")
	require.ErrorContains(t, outcome.err, "no such session")
}

func TestClient_GetSpentStateIDs_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	c, writer, _ := newTestClient(t)
	view := c.ForCoordinator("coordinator-node", "session-1")

	done := startGetSpentStateIDs(ctx, view)
	<-writer.sentSpent
	cancel()

	outcome := <-done
	require.ErrorIs(t, outcome.err, context.Canceled)
	assert.Nil(t, outcome.ids)
}

func TestClient_GetSpentStateIDs_WrongNodeResponseDropped(t *testing.T) {
	ctx := t.Context()
	c, writer, _ := newTestClient(t)
	view := c.ForCoordinator("coordinator-node", "session-1")

	done := startGetSpentStateIDs(ctx, view)
	req := <-writer.sentSpent

	// A response from any node other than the one the request was sent to is dropped; the
	// request keeps waiting until the real coordinator answers.
	c.HandleGetSpentStateIDsResponse(ctx, "impostor-node", &engineProto.GetSpentStateIDsResponse{
		RequestId:     req.GetRequestId(),
		SpentStateIds: [][]byte{{0xff}},
	})
	select {
	case <-done:
		t.Fatal("response from the wrong node must not complete the request")
	default:
	}

	c.HandleGetSpentStateIDsResponse(ctx, "coordinator-node", &engineProto.GetSpentStateIDsResponse{
		RequestId: req.GetRequestId(),
	})
	outcome := <-done
	require.NoError(t, outcome.err)
	assert.Empty(t, outcome.ids)
}
