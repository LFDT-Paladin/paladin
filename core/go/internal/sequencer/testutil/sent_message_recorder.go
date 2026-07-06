/*
 * Copyright © 2025 Kaleido, Inc.
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

package testutil

import (
	"context"

	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// SentMessageRecorder implements TransportWriter for use in tests.
// It records outgoing messages (both coordinator-side and originator-side) so tests can assert on what was sent.
type SentMessageRecorder struct {
	// Coordinator-side tracking
	hasSentAssembleRequest                        bool
	sentAssembleRequestIdempotencyKey             uuid.UUID
	numberOfSentAssembleRequests                  int
	hasSentDispatchConfirmationRequest            bool
	numberOfSentEndorsementRequests               int
	sentEndorsementRequestsForPartyIdempotencyKey map[string]uuid.UUID
	numberOfEndorsementRequestsForParty           map[string]int
	sentDispatchConfirmationRequestIdempotencyKey uuid.UUID
	numberOfSentDispatchConfirmationRequests      int

	assembleKeyByTxID        map[uuid.UUID]uuid.UUID
	endorseKeyByTxIDAndParty map[uuid.UUID]map[string]uuid.UUID
	dispatchConfirmKeyByTxID map[uuid.UUID]uuid.UUID

	hasSentHandoverRequest bool
	sentHeartbeatCount     int

	// Originator-side tracking
	hasSentConfirmationResponse    bool
	hasSentAssembleSuccessResponse bool
	hasSentAssembleRevertResponse  bool
	hasSentAssembleParkResponse    bool
	hasSentAssembleError           bool
	hasSentAssembleRejection       bool
	hasSentPreDispatchRejection    bool
	preDispatchRejectionReason     engineProto.RejectionReason
	hasSentDelegationRequest       bool
	delegatedTransactionIDs        []uuid.UUID
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{
		sentEndorsementRequestsForPartyIdempotencyKey: make(map[string]uuid.UUID),
		numberOfEndorsementRequestsForParty:           make(map[string]int),
		assembleKeyByTxID:                             make(map[uuid.UUID]uuid.UUID),
		endorseKeyByTxIDAndParty:                      make(map[uuid.UUID]map[string]uuid.UUID),
		dispatchConfirmKeyByTxID:                      make(map[uuid.UUID]uuid.UUID),
	}
}

func (r *SentMessageRecorder) Reset(ctx context.Context) {
	r.hasSentAssembleRequest = false
	r.sentAssembleRequestIdempotencyKey = uuid.UUID{}
	r.numberOfSentAssembleRequests = 0
	r.hasSentDispatchConfirmationRequest = false
	r.numberOfSentEndorsementRequests = 0
	r.sentEndorsementRequestsForPartyIdempotencyKey = make(map[string]uuid.UUID)
	r.numberOfEndorsementRequestsForParty = make(map[string]int)
	r.sentDispatchConfirmationRequestIdempotencyKey = uuid.UUID{}
	r.numberOfSentDispatchConfirmationRequests = 0
	r.hasSentHandoverRequest = false
	r.sentHeartbeatCount = 0
	r.hasSentConfirmationResponse = false
	r.hasSentAssembleSuccessResponse = false
	r.hasSentAssembleRevertResponse = false
	r.hasSentAssembleParkResponse = false
	r.hasSentAssembleError = false
	r.hasSentAssembleRejection = false
	r.hasSentPreDispatchRejection = false
	r.preDispatchRejectionReason = 0
	r.hasSentDelegationRequest = false
	r.delegatedTransactionIDs = nil
	// per-tx maps are NOT reset — they accumulate across the full test
}

func (r *SentMessageRecorder) StartLoopbackWriter() {}

func (r *SentMessageRecorder) WaitForDone(ctx context.Context) {}

func (r *SentMessageRecorder) HasSentAssembleRequest() bool {
	return r.hasSentAssembleRequest
}

func (r *SentMessageRecorder) HasSentDispatchConfirmationRequest() bool {
	return r.hasSentDispatchConfirmationRequest
}

func (r *SentMessageRecorder) NumberOfSentAssembleRequests() int {
	return r.numberOfSentAssembleRequests
}

func (r *SentMessageRecorder) NumberOfSentEndorsementRequests() int {
	return r.numberOfSentEndorsementRequests
}

func (r *SentMessageRecorder) SentEndorsementRequestsForPartyIdempotencyKey(party string) uuid.UUID {
	return r.sentEndorsementRequestsForPartyIdempotencyKey[party]
}

func (r *SentMessageRecorder) NumberOfEndorsementRequestsForParty(party string) int {
	return r.numberOfEndorsementRequestsForParty[party]
}

func (r *SentMessageRecorder) NumberOfSentDispatchConfirmationRequests() int {
	return r.numberOfSentDispatchConfirmationRequests
}

func (r *SentMessageRecorder) SentAssembleRequestIdempotencyKey() uuid.UUID {
	return r.sentAssembleRequestIdempotencyKey
}

func (r *SentMessageRecorder) SentDispatchConfirmationRequestIdempotencyKey() uuid.UUID {
	return r.sentDispatchConfirmationRequestIdempotencyKey
}

func (r *SentMessageRecorder) AssembleKeyForTx(txID uuid.UUID) uuid.UUID {
	return r.assembleKeyByTxID[txID]
}

func (r *SentMessageRecorder) EndorseKeyForTxAndParty(txID uuid.UUID, party string) uuid.UUID {
	if m, ok := r.endorseKeyByTxIDAndParty[txID]; ok {
		return m[party]
	}
	return uuid.UUID{}
}

func (r *SentMessageRecorder) DispatchConfirmKeyForTx(txID uuid.UUID) uuid.UUID {
	return r.dispatchConfirmKeyByTxID[txID]
}

func (r *SentMessageRecorder) SentHeartbeatCount() int {
	return r.sentHeartbeatCount
}

func (r *SentMessageRecorder) HasSentHeartbeat() bool {
	return r.sentHeartbeatCount > 0
}

func (r *SentMessageRecorder) SendAssembleRequest(ctx context.Context, node string, msg *engineProto.AssembleRequest) error {
	r.hasSentAssembleRequest = true
	idempotencyKey, _ := uuid.Parse(msg.AssembleRequestId)
	txID, _ := uuid.Parse(msg.TransactionId)
	r.sentAssembleRequestIdempotencyKey = idempotencyKey
	r.numberOfSentAssembleRequests++
	r.assembleKeyByTxID[txID] = idempotencyKey
	return nil
}

func (r *SentMessageRecorder) SendEndorsementRequest(ctx context.Context, node string, msg *engineProto.EndorsementRequest) error {
	party := msg.Party
	idempotencyKey, _ := uuid.Parse(msg.IdempotencyKey)
	txID, _ := uuid.Parse(msg.TransactionId)
	r.numberOfSentEndorsementRequests++
	if _, ok := r.numberOfEndorsementRequestsForParty[party]; ok {
		r.numberOfEndorsementRequestsForParty[party]++
	} else {
		r.numberOfEndorsementRequestsForParty[party] = 1
		r.sentEndorsementRequestsForPartyIdempotencyKey[party] = idempotencyKey
	}
	if r.endorseKeyByTxIDAndParty[txID] == nil {
		r.endorseKeyByTxIDAndParty[txID] = make(map[string]uuid.UUID)
	}
	r.endorseKeyByTxIDAndParty[txID][party] = idempotencyKey
	return nil
}

func (r *SentMessageRecorder) SendPreDispatchRequest(ctx context.Context, node string, msg *engineProto.PreDispatchRequest) error {
	r.hasSentDispatchConfirmationRequest = true
	idempotencyKey, _ := uuid.Parse(msg.Id)
	r.sentDispatchConfirmationRequestIdempotencyKey = idempotencyKey
	r.numberOfSentDispatchConfirmationRequests++
	if txID, err := uuid.Parse(msg.TransactionId); err == nil {
		r.dispatchConfirmKeyByTxID[txID] = idempotencyKey
	}
	return nil
}

func (r *SentMessageRecorder) SendHeartbeat(ctx context.Context, node string, msg *engineProto.CoordinatorHeartbeatNotification) error {
	r.sentHeartbeatCount++
	return nil
}

func (r *SentMessageRecorder) SendAssembleResponse(ctx context.Context, node string, msg *engineProto.AssembleResponse) error {
	switch msg.GetPostAssembly().GetAssemblyResult() {
	case prototk.AssembleTransactionResponse_OK:
		r.hasSentAssembleSuccessResponse = true
	case prototk.AssembleTransactionResponse_REVERT:
		r.hasSentAssembleRevertResponse = true
	case prototk.AssembleTransactionResponse_PARK:
		r.hasSentAssembleParkResponse = true
	}
	return nil
}

func (r *SentMessageRecorder) HasSentAssembleSuccessResponse() bool {
	return r.hasSentAssembleSuccessResponse
}

func (r *SentMessageRecorder) HasSentAssembleRevertResponse() bool {
	return r.hasSentAssembleRevertResponse
}

func (r *SentMessageRecorder) HasSentAssembleParkResponse() bool {
	return r.hasSentAssembleParkResponse
}

func (r *SentMessageRecorder) SendAssembleError(ctx context.Context, node string, msg *engineProto.AssembleError) error {
	r.hasSentAssembleError = true
	return nil
}

func (r *SentMessageRecorder) SendAssembleRejection(ctx context.Context, node string, msg *engineProto.AssembleRejection) error {
	r.hasSentAssembleRejection = true
	return nil
}

func (r *SentMessageRecorder) SendPreDispatchRejection(ctx context.Context, node string, msg *engineProto.PreDispatchRejection) error {
	r.hasSentPreDispatchRejection = true
	r.preDispatchRejectionReason = msg.RejectionReason
	return nil
}

func (r *SentMessageRecorder) HasSentPreDispatchRejection() bool {
	return r.hasSentPreDispatchRejection
}

func (r *SentMessageRecorder) PreDispatchRejectionReason() engineProto.RejectionReason {
	return r.preDispatchRejectionReason
}

func (r *SentMessageRecorder) HasSentAssembleRejection() bool {
	return r.hasSentAssembleRejection
}

func (r *SentMessageRecorder) HasSentAssembleError() bool {
	return r.hasSentAssembleError
}

func (r *SentMessageRecorder) SendPreDispatchResponse(ctx context.Context, node string, msg *engineProto.PreDispatchResponse) error {
	r.hasSentConfirmationResponse = true
	return nil
}

func (r *SentMessageRecorder) HasSentPreDispatchResponse() bool {
	return r.hasSentConfirmationResponse
}

func (r *SentMessageRecorder) SendNonceAssigned(ctx context.Context, node string, msg *engineProto.NonceAssigned) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionSubmitted(ctx context.Context, node string, msg *engineProto.TransactionSubmitted) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionConfirmed(ctx context.Context, node string, msg *engineProto.TransactionConfirmed) error {
	return nil
}

func (r *SentMessageRecorder) SendDelegationRequest(ctx context.Context, node string, msg *engineProto.DelegationRequest) error {
	r.hasSentDelegationRequest = true
	for _, del := range msg.Transactions {
		id, err := uuid.Parse(del.GetId())
		if err != nil {
			continue
		}
		r.delegatedTransactionIDs = append(r.delegatedTransactionIDs, id)
	}
	return nil
}

func (r *SentMessageRecorder) HasSentDelegationRequest() bool {
	return r.hasSentDelegationRequest
}

func (r *SentMessageRecorder) HasDelegatedTransaction(txid uuid.UUID) bool {
	for _, id := range r.delegatedTransactionIDs {
		if id == txid {
			return true
		}
	}
	return false
}

func (r *SentMessageRecorder) SendDelegationResponse(ctx context.Context, node string, msg *engineProto.DelegationResponse) error {
	return nil
}

func (r *SentMessageRecorder) SendDelegationRejection(ctx context.Context, node string, msg *engineProto.DelegationRejection) error {
	return nil
}

func (r *SentMessageRecorder) SendHandoverRequest(ctx context.Context, node string, msg *engineProto.CoordinatorHandoverRequest) error {
	r.hasSentHandoverRequest = true
	return nil
}

func (r *SentMessageRecorder) HasSentHandoverRequest() bool {
	return r.hasSentHandoverRequest
}

func (r *SentMessageRecorder) SendDispatched(ctx context.Context, node string, msg *engineProto.TransactionDispatched) error {
	return nil
}

func (r *SentMessageRecorder) SendEndorsementResponse(ctx context.Context, node string, msg *engineProto.EndorsementResponse) error {
	return nil
}

func (r *SentMessageRecorder) SendEndorsementError(ctx context.Context, node string, msg *engineProto.EndorsementError) error {
	return nil
}

func (r *SentMessageRecorder) SendEndorsementRejection(ctx context.Context, node string, msg *engineProto.EndorsementRejection) error {
	return nil
}
