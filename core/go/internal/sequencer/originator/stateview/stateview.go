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

// Package stateview gives an assembling transaction on-demand read access to the coordinator's
// ahead-of-chain view, over the view the coordinator captures when it sends the assemble request.
// Requests are correlated to responses purely by request ID, and each carries the assemble request
// ID so the coordinator answers from the view captured for that assemble. The coordinator side of
// the exchange lives in sequencer/coordinator/stateview.
package stateview

import (
	"context"
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// Reader is the originator-side dispatcher of coordinator state view requests. Requests are
// correlated to responses purely by request ID. Each request carries the assemble request ID so
// the coordinator answers it from the view captured for that assemble. All methods are
// thread-safe: the Handle* methods run on transport goroutines, requests on assembly goroutines —
// none of them touch the originator event loop.
type Reader interface {
	// ForCoordinator returns a RemoteStateView bound to
	// a) coordinatorNode — the node requests are sent to and the only node whose responses are accepted for them.
	// b) assembleRequestID — the assemble whose captured view the coordinator answers from. One view is captured per assembly.
	ForCoordinator(coordinatorNode string, assembleRequestID string) components.RemoteStateView

	// HandleQueryAvailableStatesResponse delivers a state query response to the request that is
	// waiting for it. Responses for unknown request IDs (stale, duplicate) and responses from any
	// node other than the one the request was sent to are dropped.
	HandleQueryAvailableStatesResponse(ctx context.Context, fromNode string, resp *engineProto.QueryAvailableStatesResponse)

	// HandleGetSpentStateIDsResponse delivers a spent state IDs response to the request that is
	// waiting for it. The same drop rules apply as HandleQueryAvailableStatesResponse.
	HandleGetSpentStateIDsResponse(ctx context.Context, fromNode string, resp *engineProto.GetSpentStateIDsResponse)

	// HandleError delivers a state view error, failing the request that is waiting for it.
	// The same drop rules apply as HandleQueryAvailableStatesResponse.
	HandleError(ctx context.Context, fromNode string, errMsg *engineProto.StateViewError)
}

type requestResult struct {
	states        []*prototk.QueriedState
	spentStateIDs []pldtypes.HexBytes
	err           error
}

type pendingRequest struct {
	coordinatorNode string
	resultCh        chan *requestResult
}

type reader struct {
	mu              sync.Mutex
	pending         map[string]*pendingRequest
	transportWriter transport.TransportWriter
	contractAddress string
	requestTimeout  time.Duration
	clock           common.Clock
}

func NewReader(contractAddress string, transportWriter transport.TransportWriter, requestTimeout time.Duration, clock common.Clock) Reader {
	return &reader{
		pending:         make(map[string]*pendingRequest),
		transportWriter: transportWriter,
		contractAddress: contractAddress,
		requestTimeout:  requestTimeout,
		clock:           clock,
	}
}

type boundView struct {
	reader            *reader
	coordinatorNode   string
	assembleRequestID string

	// The spend exclusion set is fixed for the captured view, so the first GetSpentStateIDs fetches it and
	// every later call on this view returns the cached slice. A failed fetch is not cached, so a
	// transient error is retried on the next call. spentMu guards the cache and is held across the
	// round-trip, so concurrent callers make a single fetch.
	spentMu       sync.Mutex
	spentFetched  bool
	spentStateIDs []pldtypes.HexBytes
}

func (r *reader) ForCoordinator(coordinatorNode string, assembleRequestID string) components.RemoteStateView {
	return &boundView{reader: r, coordinatorNode: coordinatorNode, assembleRequestID: assembleRequestID}
}

// QueryAvailableStates blocks until the response arrives or ctx expires (the assembly deadline).
// The returned states are NOT validated here — this is the responsibility of the caller.
func (b *boundView) QueryAvailableStates(ctx context.Context, schemaID string, queryJSON string) ([]*prototk.QueriedState, error) {
	result, err := b.reader.roundTrip(ctx, b.coordinatorNode, func(requestID string) error {
		return b.reader.transportWriter.SendQueryAvailableStatesRequest(ctx, b.coordinatorNode, &engineProto.QueryAvailableStatesRequest{
			ContractAddress:   b.reader.contractAddress,
			RequestId:         requestID,
			SchemaId:          schemaID,
			QueryJson:         queryJSON,
			AssembleRequestId: b.assembleRequestID,
		})
	})
	if err != nil {
		return nil, err
	}
	return result.states, result.err
}

// GetSpentStateIDs blocks until the response arrives or ctx expires (the assembly deadline).
// The result is cached since repeat calls should never return a different set of IDs.
func (b *boundView) GetSpentStateIDs(ctx context.Context) ([]pldtypes.HexBytes, error) {
	b.spentMu.Lock()
	defer b.spentMu.Unlock()
	if b.spentFetched {
		return b.spentStateIDs, nil
	}
	result, err := b.reader.roundTrip(ctx, b.coordinatorNode, func(requestID string) error {
		return b.reader.transportWriter.SendGetSpentStateIDsRequest(ctx, b.coordinatorNode, &engineProto.GetSpentStateIDsRequest{
			ContractAddress:   b.reader.contractAddress,
			RequestId:         requestID,
			AssembleRequestId: b.assembleRequestID,
		})
	})
	if err != nil {
		return nil, err
	}
	if result.err != nil {
		return nil, result.err
	}
	b.spentStateIDs = result.spentStateIDs
	b.spentFetched = true
	return b.spentStateIDs, nil
}

// roundTrip sends one idempotent request to the coordinator and does not return until a success
// or error result is delivered or ctx expires (the assembly deadline). Unanswered requests are
// retried on a configurable interval (requestTimeout).
func (r *reader) roundTrip(ctx context.Context, coordinatorNode string, send func(requestID string) error) (*requestResult, error) {
	requestID := uuid.New().String()
	pr := &pendingRequest{
		coordinatorNode: coordinatorNode,
		resultCh:        make(chan *requestResult, 1),
	}

	r.mu.Lock()
	r.pending[requestID] = pr
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.pending, requestID)
		r.mu.Unlock()
	}()

	for {
		if err := send(requestID); err != nil {
			log.L(ctx).Warnf("stateview reader: failed to send state view request %s: %s", requestID, err)
		}
		retryCh := make(chan struct{}, 1)
		cancelTimer := r.clock.ScheduleTimer(ctx, r.requestTimeout, func() {
			retryCh <- struct{}{}
		})
		select {
		case result := <-pr.resultCh:
			cancelTimer()
			return result, nil
		case <-ctx.Done():
			cancelTimer()
			return nil, ctx.Err()
		case <-retryCh:
			log.L(ctx).Debugf("stateview reader: retrying state view request %s", requestID)
		}
	}
}

func (r *reader) HandleQueryAvailableStatesResponse(ctx context.Context, fromNode string, resp *engineProto.QueryAvailableStatesResponse) {
	r.deliver(ctx, fromNode, resp.GetRequestId(), &requestResult{states: resp.GetStates()})
}

func (r *reader) HandleGetSpentStateIDsResponse(ctx context.Context, fromNode string, resp *engineProto.GetSpentStateIDsResponse) {
	raw := resp.GetSpentStateIds()
	spentStateIDs := make([]pldtypes.HexBytes, len(raw))
	for i, id := range raw {
		spentStateIDs[i] = id
	}
	r.deliver(ctx, fromNode, resp.GetRequestId(), &requestResult{spentStateIDs: spentStateIDs})
}

// HandleError delivers a StateViewError. The coordinator replies with an error instead of a
// response when the request is invalid (bad schema id / query JSON) or evaluation fails.
func (r *reader) HandleError(ctx context.Context, fromNode string, errMsg *engineProto.StateViewError) {
	r.deliver(ctx, fromNode, errMsg.GetRequestId(), &requestResult{
		err: i18n.NewError(ctx, msgs.MsgSequencerStateViewFailed, errMsg.GetRequestId(), errMsg.GetErrorMessage()),
	})
}

// deliver hands a result to the request waiting on requestID. Unknown request IDs (stale retries,
// duplicates) and results from the wrong node are dropped; the channel has capacity 1 and only the
// first result is kept.
func (r *reader) deliver(ctx context.Context, fromNode string, requestID string, result *requestResult) {
	r.mu.Lock()
	pr := r.pending[requestID]
	r.mu.Unlock()
	if pr == nil {
		log.L(ctx).Debugf("stateview reader: dropping state view result for unknown request %s", requestID)
		return
	}
	if fromNode != pr.coordinatorNode {
		log.L(ctx).Warnf("stateview reader: dropping state view result for request %s from %s: request was sent to a different node", requestID, fromNode)
		return
	}
	select {
	case pr.resultCh <- result:
	default:
		log.L(ctx).Debugf("stateview reader: dropping duplicate state view result for request %s", requestID)
	}
}
