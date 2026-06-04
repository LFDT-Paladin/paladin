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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// SentMessageRecorder implements TransportWriter for use in tests.
// TODO: add test coverage- or consider moving to its own package which we exclude from coverage
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
	hasSentAssembleErrorResponse   bool
	hasSentTransactionUnknown      bool
	transactionUnknownTxID         uuid.UUID
	transactionUnknownCoordinator  string
	hasSentNotActiveCoordinator    bool
	hasSentDelegationRequest       bool
	delegatedTransactions          []*components.PrivateTransaction
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
	r.hasSentAssembleErrorResponse = false
	r.hasSentTransactionUnknown = false
	r.transactionUnknownTxID = uuid.UUID{}
	r.transactionUnknownCoordinator = ""
	r.hasSentNotActiveCoordinator = false
	r.hasSentDelegationRequest = false
	r.delegatedTransactions = nil
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

func (r *SentMessageRecorder) SendAssembleRequest(
	ctx context.Context,
	assemblingNode string,
	transactionID uuid.UUID,
	idempotencyKey uuid.UUID,
	transactionPreassembly *components.TransactionPreAssembly,
	stateLocks grapher.ExportableStates,
	blockHeight int64,
) error {
	r.hasSentAssembleRequest = true
	r.sentAssembleRequestIdempotencyKey = idempotencyKey
	r.numberOfSentAssembleRequests++
	r.assembleKeyByTxID[transactionID] = idempotencyKey
	return nil
}

func (r *SentMessageRecorder) SendEndorsementRequest(
	ctx context.Context,
	txID uuid.UUID,
	idempotencyKey uuid.UUID,
	party string,
	attRequest *prototk.AttestationRequest,
	transactionSpecification *prototk.TransactionSpecification,
	verifiers []*prototk.ResolvedVerifier,
	signatures []*prototk.AttestationResult,
	inputStates []*prototk.EndorsableState,
	readStates []*prototk.EndorsableState,
	outputStates []*prototk.EndorsableState,
	infoStates []*prototk.EndorsableState,
) error {
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

func (r *SentMessageRecorder) SendPreDispatchRequest(
	ctx context.Context,
	transactionOriginator string,
	idempotencyKey uuid.UUID,
	transactionSpecification *prototk.TransactionSpecification,
	hash *pldtypes.Bytes32,
) error {
	r.hasSentDispatchConfirmationRequest = true
	r.sentDispatchConfirmationRequestIdempotencyKey = idempotencyKey
	r.numberOfSentDispatchConfirmationRequests++
	if transactionSpecification != nil {
		if txID, err := uuid.Parse(transactionSpecification.TransactionId); err == nil {
			r.dispatchConfirmKeyByTxID[txID] = idempotencyKey
		}
	}
	return nil
}

func (r *SentMessageRecorder) SendHeartbeat(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress, coordinatorSnapshot *common.CoordinatorSnapshot) error {
	r.sentHeartbeatCount++
	return nil
}

func (r *SentMessageRecorder) SendAssembleResponse(ctx context.Context, txID uuid.UUID, requestID uuid.UUID, postAssembly *components.TransactionPostAssembly, preAssembly *components.TransactionPreAssembly, recipient string) error {
	switch postAssembly.AssemblyResult {
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

func (r *SentMessageRecorder) SendAssembleErrorResponse(ctx context.Context, txID uuid.UUID, requestID uuid.UUID, recipient string) error {
	r.hasSentAssembleErrorResponse = true
	return nil
}

func (r *SentMessageRecorder) HasSentAssembleErrorResponse() bool {
	return r.hasSentAssembleErrorResponse
}

func (r *SentMessageRecorder) SendPreDispatchResponse(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {
	r.hasSentConfirmationResponse = true
	return nil
}

func (r *SentMessageRecorder) HasSentPreDispatchResponse() bool {
	return r.hasSentConfirmationResponse
}

func (r *SentMessageRecorder) SendNonceAssigned(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, nonce uint64) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionSubmitted(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, txHash *pldtypes.Bytes32) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionConfirmed(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, nonce *pldtypes.HexUint64, outcome engineProto.TransactionConfirmed_Outcome, revertReason pldtypes.HexBytes, failureMessage string, willRetry bool) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionUnknown(ctx context.Context, coordinatorNode string, txID uuid.UUID) error {
	r.hasSentTransactionUnknown = true
	r.transactionUnknownTxID = txID
	r.transactionUnknownCoordinator = coordinatorNode
	return nil
}

func (r *SentMessageRecorder) HasSentTransactionUnknown() bool {
	return r.hasSentTransactionUnknown
}

func (r *SentMessageRecorder) GetTransactionUnknownDetails() (txID uuid.UUID, coordinator string) {
	return r.transactionUnknownTxID, r.transactionUnknownCoordinator
}

func (r *SentMessageRecorder) SendNotActiveCoordinator(ctx context.Context, coordinatorNode string, txID uuid.UUID) error {
	r.hasSentNotActiveCoordinator = true
	return nil
}

func (r *SentMessageRecorder) HasSentNotActiveCoordinator() bool {
	return r.hasSentNotActiveCoordinator
}

func (r *SentMessageRecorder) SendDelegationRequest(ctx context.Context, coordinatorLocator string, transactions []*components.PrivateTransaction, blockHeight uint64) error {
	r.hasSentDelegationRequest = true
	r.delegatedTransactions = transactions
	return nil
}

func (r *SentMessageRecorder) HasSentDelegationRequest() bool {
	return r.hasSentDelegationRequest
}

func (r *SentMessageRecorder) HasDelegatedTransaction(txid uuid.UUID) bool {
	for _, tx := range r.delegatedTransactions {
		if tx.ID == txid {
			return true
		}
	}
	return false
}

func (r *SentMessageRecorder) GetDelegatedTransactions() []*components.PrivateTransaction {
	return r.delegatedTransactions
}

func (r *SentMessageRecorder) SendDelegationRequestAcknowledgment(ctx context.Context, delegatingNodeName string, delegationId string, transactionIDs []string, errors []int64, blockHeight uint64) error {
	return nil
}

func (r *SentMessageRecorder) SendDelegationRequestRejection(ctx context.Context, delegatingNodeName string, delegationId string, blockHeight uint64, activeCoordinator string) error {
	return nil
}

func (r *SentMessageRecorder) SendHandoverRequest(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress) error {
	r.hasSentHandoverRequest = true
	return nil
}

func (r *SentMessageRecorder) HasSentHandoverRequest() bool {
	return r.hasSentHandoverRequest
}

func (r *SentMessageRecorder) SendDispatched(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {
	return nil
}

func (r *SentMessageRecorder) SendEndorsementResponse(ctx context.Context, transactionId, idempotencyKey, contractAddress string, attResult *prototk.AttestationResult, endorsementResult *components.EndorsementResult, revertReason, endorsementName, party, node string) error {
	return nil
}
