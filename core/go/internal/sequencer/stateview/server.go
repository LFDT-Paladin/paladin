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

// Package stateview gives an assembling originator on-demand read access to the coordinator's
// ahead-of-chain view. Instead of shipping that view on the assemble request, the coordinator
// opens a per-assemble session when the request is sent, capturing a point-in-time view: the candidate
// states visible to the originator plus the IDs of the states already spend-locked. Two request
// kinds are answered from that point-in-time view — queries against the available created states, and
// fetching the spent state ID list. Requests, responses and errors are routed directly between the
// transport handler and the server/client — entirely off both event loops.
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

// Server serves state view requests against a per-assemble session. A session is open for exactly
// the window in which the originator is entitled to query: from the assemble request being sent
// until the transaction leaves State_Assembling. Visibility is enforced against the transport-authenticated
// sender, which must match the node the session was opened for.
type Server interface {
	// OpenSession captures a point-in-time view — the states currently available to node plus the IDs of
	// the states currently spend-locked — and holds it under sessionID.
	// node is the only node permitted to query on this session.
	OpenSession(ctx context.Context, sessionID string, node string)

	// CloseSession removes the session. No-op if absent.
	CloseSession(ctx context.Context, sessionID string)

	// HandleQueryAvailableStates serves a state query request against the request's session, replying with a
	// QueryAvailableStatesResponse carrying the matching states (with data), or a single StateViewError for
	// the whole request. fromNode is the transport-authenticated sender and must own the session.
	HandleQueryAvailableStates(ctx context.Context, fromNode string, req *engineProto.QueryAvailableStatesRequest)

	// HandleGetSpentStateIDs serves the session's captured spent state ID list, replying with a
	// GetSpentStateIDsResponse or a StateViewError. The same sender rules apply as
	// HandleQueryAvailableStates.
	HandleGetSpentStateIDs(ctx context.Context, fromNode string, req *engineProto.GetSpentStateIDsRequest)
}

type serverSession struct {
	node          string
	candidates    []*prototk.SnapshotState
	spentStateIDs []pldtypes.HexBytes
}

type server struct {
	domainName      string
	contractAddress string
	transportWriter transport.TransportWriter
	grapher         grapher.Grapher
	stateManager    components.StateManager

	mu       sync.Mutex
	sessions map[string]*serverSession
}

func NewServer(domainName string, contractAddress string, transportWriter transport.TransportWriter, g grapher.Grapher, stateManager components.StateManager) Server {
	return &server{
		domainName:      domainName,
		contractAddress: contractAddress,
		transportWriter: transportWriter,
		grapher:         g,
		stateManager:    stateManager,
		sessions:        make(map[string]*serverSession),
	}
}

func (s *server) OpenSession(ctx context.Context, sessionID string, node string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[sessionID]; exists {
		log.L(ctx).Debugf("stateview server: session %s already open, keeping existing view", sessionID)
		return
	}
	candidates, spentStateIDs := s.grapher.SnapshotView(ctx, node)
	log.L(ctx).Debugf("stateview server: open session %s for node %s with %d candidate states and %d spent state IDs", sessionID, node, len(candidates), len(spentStateIDs))
	s.sessions[sessionID] = &serverSession{node: node, candidates: candidates, spentStateIDs: spentStateIDs}
}

func (s *server) CloseSession(ctx context.Context, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log.L(ctx).Debugf("stateview server: close session %s", sessionID)
	delete(s.sessions, sessionID)
}

// lookupSession validates the request against the session registry, returning the session or an
// error when the request must be rejected. s.mu is held only for the lookup so evaluation and the
// response send run outside the lock.
func (s *server) lookupSession(ctx context.Context, fromNode string, sessionID string) (*serverSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, found := s.sessions[sessionID]
	if !found {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerStateViewUnknownSession, sessionID)
	}
	if session.node != fromNode {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerStateViewWrongNode, sessionID)
	}
	return session, nil
}

func (s *server) HandleQueryAvailableStates(ctx context.Context, fromNode string, req *engineProto.QueryAvailableStatesRequest) {
	session, err := s.lookupSession(ctx, fromNode, req.GetSessionId())
	if err != nil {
		s.sendError(ctx, fromNode, req.GetRequestId(), err)
		return
	}
	schemaID, err := pldtypes.ParseBytes32Ctx(ctx, req.GetSchemaId())
	if err != nil {
		s.sendError(ctx, fromNode, req.GetRequestId(), i18n.NewError(ctx, msgs.MsgSequencerStateViewInvalid, req.GetRequestId(), err))
		return
	}
	var jq query.QueryJSON
	if err := json.Unmarshal([]byte(req.GetQueryJson()), &jq); err != nil {
		s.sendError(ctx, fromNode, req.GetRequestId(), i18n.NewError(ctx, msgs.MsgSequencerStateViewInvalid, req.GetRequestId(), err))
		return
	}

	// The match/sort/limit runs on the session's point-in-time candidate view, so the originator's
	// view is consistent for the whole in-flight assemble regardless of concurrent grapher changes.
	candidates := session.candidates
	states, err := s.stateManager.FindMatchingInMemoryStates(ctx, s.domainName, schemaID, &jq, candidates)
	if err != nil {
		s.sendError(ctx, fromNode, req.GetRequestId(), err)
		return
	}

	log.L(ctx).Debugf("stateview server: serving %d states (of %d candidates) for request %s from %s", len(states), len(candidates), req.GetRequestId(), fromNode)
	if err := s.transportWriter.SendQueryAvailableStatesResponse(ctx, fromNode, &engineProto.QueryAvailableStatesResponse{
		ContractAddress: s.contractAddress,
		RequestId:       req.GetRequestId(),
		States:          states,
	}); err != nil {
		// Fire-and-forget: the client re-sends the same request ID until it gets a response.
		log.L(ctx).Warnf("stateview server: failed to send query available states response to %s: %s", fromNode, err)
	}
}

func (s *server) HandleGetSpentStateIDs(ctx context.Context, fromNode string, req *engineProto.GetSpentStateIDsRequest) {
	session, err := s.lookupSession(ctx, fromNode, req.GetSessionId())
	if err != nil {
		s.sendError(ctx, fromNode, req.GetRequestId(), err)
		return
	}

	spentStateIDs := make([][]byte, len(session.spentStateIDs))
	for i, id := range session.spentStateIDs {
		spentStateIDs[i] = id
	}

	log.L(ctx).Debugf("stateview server: serving %d spent state IDs for request %s from %s", len(spentStateIDs), req.GetRequestId(), fromNode)
	if err := s.transportWriter.SendGetSpentStateIDsResponse(ctx, fromNode, &engineProto.GetSpentStateIDsResponse{
		ContractAddress: s.contractAddress,
		RequestId:       req.GetRequestId(),
		SpentStateIds:   spentStateIDs,
	}); err != nil {
		// Fire-and-forget: the client re-sends the same request ID until it gets a response.
		log.L(ctx).Warnf("stateview server: failed to send get spent state IDs response to %s: %s", fromNode, err)
	}
}

func (s *server) sendError(ctx context.Context, node string, requestID string, cause error) {
	log.L(ctx).Warnf("stateview server: rejecting state view request %s from %s: %s", requestID, node, cause)
	if err := s.transportWriter.SendStateViewError(ctx, node, &engineProto.StateViewError{
		ContractAddress: s.contractAddress,
		RequestId:       requestID,
		ErrorMessage:    cause.Error(),
	}); err != nil {
		log.L(ctx).Warnf("stateview server: failed to send state view error to %s: %s", node, err)
	}
}
