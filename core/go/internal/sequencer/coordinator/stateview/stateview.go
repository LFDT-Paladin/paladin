// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package stateview answers an assembling originator's on-demand reads of the coordinator's
// ahead-of-chain view. Instead of shipping that view on the assemble request, the coordinator
// opens a view when the request is sent, capturing a point in time: the candidate
// states visible to the originator plus the IDs of the states already spend-locked. Two request
// kinds are answered from that captured view — queries against the available created states, and
// fetching the spent state ID list. Requests, responses and errors are routed directly between the
// transport handler and here — entirely off both event loops. The originator side of the exchange
// lives in sequencer/originator/stateview.
package stateview

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// Provider serves state view requests against the view captured for an assemble, keyed by the
// assemble request ID. A view is open for exactly the window in which the originator is entitled to
// query: from the assemble request being sent until the transaction leaves State_Assembling.
// Visibility is enforced against the transport-authenticated sender, which must match the node the
// view was captured for.
type Provider interface {
	// OpenView captures a point in time — the states currently available to node plus the IDs of
	// the states currently spend-locked — and holds it under assembleRequestID.
	// node is the only node permitted to query against it.
	OpenView(ctx context.Context, assembleRequestID string, node string)

	// CloseView discards the captured view. No-op if absent.
	CloseView(ctx context.Context, assembleRequestID string)

	// HandleQueryAvailableStates serves a state query request against the view captured for the assemble
	// it names, replying with a QueryAvailableStatesResponse carrying the matching states (with data), or a
	// single StateViewError for the whole request. fromNode is the transport-authenticated sender and must
	// be the node the view was captured for.
	HandleQueryAvailableStates(ctx context.Context, fromNode string, req *engineProto.QueryAvailableStatesRequest)

	// HandleGetSpentStateIDs serves the captured spent state ID list, replying with a
	// GetSpentStateIDsResponse or a StateViewError. The same sender rules apply as
	// HandleQueryAvailableStates.
	HandleGetSpentStateIDs(ctx context.Context, fromNode string, req *engineProto.GetSpentStateIDsRequest)
}

type capturedView struct {
	node          string
	candidates    []*prototk.SnapshotState
	spentStateIDs []pldtypes.HexBytes
}

type provider struct {
	domainName      string
	contractAddress string
	transportWriter transport.TransportWriter
	grapher         grapher.Grapher
	stateManager    components.StateManager

	mu    sync.Mutex
	views map[string]*capturedView
}

func NewProvider(domainName string, contractAddress string, transportWriter transport.TransportWriter, g grapher.Grapher, stateManager components.StateManager) Provider {
	return &provider{
		domainName:      domainName,
		contractAddress: contractAddress,
		transportWriter: transportWriter,
		grapher:         g,
		stateManager:    stateManager,
		views:           make(map[string]*capturedView),
	}
}

func (p *provider) OpenView(ctx context.Context, assembleRequestID string, node string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.views[assembleRequestID]; exists {
		log.L(ctx).Debugf("stateview provider: view for assemble request %s already open, keeping existing view", assembleRequestID)
		return
	}
	candidates, spentStateIDs := p.grapher.SnapshotView(ctx, node)
	log.L(ctx).Debugf("stateview provider: open view for assemble request %s for node %s with %d candidate states and %d spent state IDs", assembleRequestID, node, len(candidates), len(spentStateIDs))
	p.views[assembleRequestID] = &capturedView{node: node, candidates: candidates, spentStateIDs: spentStateIDs}
}

func (p *provider) CloseView(ctx context.Context, assembleRequestID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	log.L(ctx).Debugf("stateview provider: close view for assemble request %s", assembleRequestID)
	delete(p.views, assembleRequestID)
}

// lookupView validates the request against the open views, returning the captured view or an
// error when the request must be rejected. p.mu is held only for the lookup so evaluation and the
// response send run outside the lock.
func (p *provider) lookupView(ctx context.Context, fromNode string, assembleRequestID string) (*capturedView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	view, found := p.views[assembleRequestID]
	if !found {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerStateViewUnknownAssemble, assembleRequestID)
	}
	if view.node != fromNode {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerStateViewWrongNode, assembleRequestID, fromNode)
	}
	return view, nil
}

func (p *provider) HandleQueryAvailableStates(ctx context.Context, fromNode string, req *engineProto.QueryAvailableStatesRequest) {
	view, err := p.lookupView(ctx, fromNode, req.GetAssembleRequestId())
	if err != nil {
		p.sendError(ctx, fromNode, req.GetRequestId(), err)
		return
	}
	schemaID, err := pldtypes.ParseBytes32Ctx(ctx, req.GetSchemaId())
	if err != nil {
		p.sendError(ctx, fromNode, req.GetRequestId(), i18n.NewError(ctx, msgs.MsgSequencerStateViewInvalid, req.GetRequestId(), err))
		return
	}
	var jq query.QueryJSON
	if err := json.Unmarshal([]byte(req.GetQueryJson()), &jq); err != nil {
		p.sendError(ctx, fromNode, req.GetRequestId(), i18n.NewError(ctx, msgs.MsgSequencerStateViewInvalid, req.GetRequestId(), err))
		return
	}

	// The match/sort/limit runs on the captured candidate view, so the originator's view is
	// consistent for the whole in-flight assemble regardless of concurrent grapher changes.
	candidates := view.candidates
	states, err := p.stateManager.FindMatchingInMemoryStates(ctx, p.domainName, schemaID, &jq, candidates)
	if err != nil {
		p.sendError(ctx, fromNode, req.GetRequestId(), err)
		return
	}

	log.L(ctx).Debugf("stateview provider: serving %d states (of %d candidates) for request %s from %s", len(states), len(candidates), req.GetRequestId(), fromNode)
	if err := p.transportWriter.SendQueryAvailableStatesResponse(ctx, fromNode, &engineProto.QueryAvailableStatesResponse{
		ContractAddress: p.contractAddress,
		RequestId:       req.GetRequestId(),
		States:          states,
	}); err != nil {
		// Fire-and-forget: the reader re-sends the same request ID until it gets a response.
		log.L(ctx).Warnf("stateview provider: failed to send query available states response to %s: %s", fromNode, err)
	}
}

func (p *provider) HandleGetSpentStateIDs(ctx context.Context, fromNode string, req *engineProto.GetSpentStateIDsRequest) {
	view, err := p.lookupView(ctx, fromNode, req.GetAssembleRequestId())
	if err != nil {
		p.sendError(ctx, fromNode, req.GetRequestId(), err)
		return
	}

	spentStateIDs := make([][]byte, len(view.spentStateIDs))
	for i, id := range view.spentStateIDs {
		spentStateIDs[i] = id
	}

	log.L(ctx).Debugf("stateview provider: serving %d spent state IDs for request %s from %s", len(spentStateIDs), req.GetRequestId(), fromNode)
	if err := p.transportWriter.SendGetSpentStateIDsResponse(ctx, fromNode, &engineProto.GetSpentStateIDsResponse{
		ContractAddress: p.contractAddress,
		RequestId:       req.GetRequestId(),
		SpentStateIds:   spentStateIDs,
	}); err != nil {
		// Fire-and-forget: the reader re-sends the same request ID until it gets a response.
		log.L(ctx).Warnf("stateview provider: failed to send get spent state IDs response to %s: %s", fromNode, err)
	}
}

func (p *provider) sendError(ctx context.Context, node string, requestID string, cause error) {
	log.L(ctx).Warnf("stateview provider: rejecting state view request %s from %s: %s", requestID, node, cause)
	if err := p.transportWriter.SendStateViewError(ctx, node, &engineProto.StateViewError{
		ContractAddress: p.contractAddress,
		RequestId:       requestID,
		ErrorMessage:    cause.Error(),
	}); err != nil {
		log.L(ctx).Warnf("stateview provider: failed to send state view error to %s: %s", node, err)
	}
}
