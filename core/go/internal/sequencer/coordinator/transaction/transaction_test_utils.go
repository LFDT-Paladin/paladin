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

package transaction

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
)

type identityForTesting struct {
	identity        string
	identityLocator string
	verifier        string
	keyHandle       string
}

type SentMessageRecorder struct {
	hasSentAssembleRequest                        bool
	sentAssembleRequestIdempotencyKey             uuid.UUID
	numberOfSentAssembleRequests                  int
	hasSentDispatchConfirmationRequest            bool
	numberOfSentEndorsementRequests               int
	sentEndorsementRequestsForPartyIdempotencyKey map[string]uuid.UUID
	numberOfEndorsementRequestsForParty           map[string]int
	sentDispatchConfirmationRequestIdempotencyKey uuid.UUID
	numberOfSentDispatchConfirmationRequests      int
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
}

func (r *SentMessageRecorder) StartLoopbackWriter(ctx context.Context) {
}

func (r *SentMessageRecorder) StopLoopbackWriter() {
}

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

func (r *SentMessageRecorder) SendAssembleRequest(
	ctx context.Context,
	assemblingNode string,
	transactionID uuid.UUID,
	idempotencyKey uuid.UUID,
	transactionPreassembly *components.TransactionPreAssembly,
	stateLocksJSON []byte,
	blockHeight int64,
) error {
	r.hasSentAssembleRequest = true
	r.sentAssembleRequestIdempotencyKey = idempotencyKey
	r.numberOfSentAssembleRequests++
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
	return nil
}

func (r *SentMessageRecorder) SendAssembleResponse(ctx context.Context, txID uuid.UUID, requestID uuid.UUID, postAssembly *components.TransactionPostAssembly, preAssembly *components.TransactionPreAssembly, recipient string) error {
	return nil
}

func (r *SentMessageRecorder) SendPreDispatchResponse(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {
	return nil
}

func (r *SentMessageRecorder) SendHandoverRequest(ctx context.Context, activeCoordinator string, contractAddress *pldtypes.EthAddress) error {
	return nil
}

func (r *SentMessageRecorder) SendNonceAssigned(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, nonce uint64) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionSubmitted(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, txHash *pldtypes.Bytes32) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionConfirmed(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, nonce *pldtypes.HexUint64, revertReason pldtypes.HexBytes) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionUnknown(ctx context.Context, coordinatorNode string, txID uuid.UUID) error {
	return nil
}

func (r *SentMessageRecorder) SendDelegationRequest(ctx context.Context, coordinatorLocator string, transactions []*components.PrivateTransaction, blockHeight uint64) error {
	return nil
}

func (r *SentMessageRecorder) SendDelegationRequestAcknowledgment(ctx context.Context, delegatingNodeName string, delegationId string, delegateNodeName string, transactionID string) error {
	return nil
}

func (r *SentMessageRecorder) SendHeartbeat(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress, coordinatorSnapshot *common.CoordinatorSnapshot) error {
	return nil
}

func (r *SentMessageRecorder) SendDispatched(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {
	return nil
}

func (r *SentMessageRecorder) SendEndorsementResponse(ctx context.Context, transactionId, idempotencyKey, contractAddress string, attResult *prototk.AttestationResult, endorsementResult *components.EndorsementResult, revertReason, endorsementName, party, node string) error {
	return nil
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{
		sentEndorsementRequestsForPartyIdempotencyKey: make(map[string]uuid.UUID),
		numberOfEndorsementRequestsForParty:           make(map[string]int),
	}
}

type TransactionBuilderForTesting struct {
	t                                  *testing.T
	privateTransactionBuilder          *testutil.PrivateTransactionBuilderForTesting
	originator                         *identityForTesting
	signerAddress                      *pldtypes.EthAddress
	latestSubmissionHash               *pldtypes.Bytes32
	nonce                              *uint64
	state                              State
	grapher                            Grapher
	txn                                *CoordinatorTransaction
	requestTimeout                     int
	assembleTimeout                    int
	heartbeatIntervalsSinceStateChange int
	pendingAssemblyRequest             *common.IdempotentRequest
	pendingEndorsementRequests         map[string]map[string]*common.IdempotentRequest
	pendingPreDispatchRequest          *common.IdempotentRequest
	transportWriter                    *transport.MockTransportWriter
	originatorNode                     string
	preAssembly                        *components.TransactionPreAssembly
	domain                             string
	dependencies                       *pldapi.TransactionDependencies
	postAssembly                       *components.TransactionPostAssembly
	postAssemblySet                    bool                                 // true if PostAssembly() was called (allows setting nil)
	domainAPI                          *componentsmocks.DomainSmartContract // optional; if set, used instead of creating a new mock
}

// Function NewTransactionBuilderForTesting creates a TransactionBuilderForTesting with random values for all fields
// use the builder methods to set specific values for fields before calling Build to create a new Transaction
func NewTransactionBuilderForTesting(t *testing.T, state State) *TransactionBuilderForTesting {
	originatorName := "sender"
	originatorNode := "senderNode"
	builder := &TransactionBuilderForTesting{
		t: t,
		originator: &identityForTesting{
			identityLocator: fmt.Sprintf("%s@%s", originatorName, originatorNode),
			identity:        originatorName,
			verifier:        pldtypes.RandAddress().String(),
			keyHandle:       originatorName + "_KeyHandle",
		},
		state:                     state,
		assembleTimeout:           5000,
		requestTimeout:            100,
		privateTransactionBuilder: testutil.NewPrivateTransactionBuilderForTesting(),
	}

	switch state {
	case State_Submitted:
		nonce := rand.Uint64()
		builder.nonce = &nonce
		builder.signerAddress = pldtypes.RandAddress()
		latestSubmissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
		builder.latestSubmissionHash = &latestSubmissionHash
	case State_Endorsement_Gathering:
		//fine grained detail in this state needed to emulate what has already happened wrt endorsement requests and responses so far
	case State_SubmissionPrepared:
		// Emulates having received CollectedEvent; signerAddress is required when processing SubmittedEvent (which sends to originator)
		builder.signerAddress = pldtypes.RandAddress()
		fallthrough
	case State_Blocked:
		fallthrough
	case State_Confirming_Dispatchable:
		fallthrough
	case State_Ready_For_Dispatch:
		fallthrough
	case State_Dispatched:
		fallthrough
	case State_Confirmed:
		//we are emulating a transaction that has been passed State_Endorsement_Gathering so default to complete attestation plan
		builder.privateTransactionBuilder.EndorsementComplete()
	}
	return builder
}

func (b *TransactionBuilderForTesting) NumberOfRequiredEndorsers(num int) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.NumberOfRequiredEndorsers(num)
	return b
}

func (b *TransactionBuilderForTesting) NumberOfEndorsements(num int) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.NumberOfEndorsements(num)
	return b
}

func (b *TransactionBuilderForTesting) NumberOfOutputStates(num int) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.NumberOfOutputStates(num)
	return b
}

func (b *TransactionBuilderForTesting) InputStateIDs(stateIDs ...pldtypes.HexBytes) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.InputStateIDs(stateIDs...)
	return b
}

func (b *TransactionBuilderForTesting) ReadStateIDs(stateIDs ...pldtypes.HexBytes) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.ReadStateIDs(stateIDs...)
	return b
}

func (b *TransactionBuilderForTesting) PredefinedDependencies(transactionIDs ...uuid.UUID) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.PredefinedDependencies(transactionIDs...)
	return b
}

func (b *TransactionBuilderForTesting) Reverts(revertReason string) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.Reverts(revertReason)
	return b
}

func (b *TransactionBuilderForTesting) Grapher(grapher Grapher) *TransactionBuilderForTesting {
	b.grapher = grapher
	return b
}

// TransportWriter injects a mock transport writer for tests that need to set expectations on SendAssembleRequest etc.
func (b *TransactionBuilderForTesting) TransportWriter(mock *transport.MockTransportWriter) *TransactionBuilderForTesting {
	b.transportWriter = mock
	return b
}

// OriginatorNode sets the transaction's originator node (e.g. for sendAssembleRequest tests).
func (b *TransactionBuilderForTesting) OriginatorNode(node string) *TransactionBuilderForTesting {
	b.originatorNode = node
	return b
}

// PreAssembly sets the transaction's PreAssembly on the private transaction.
func (b *TransactionBuilderForTesting) PreAssembly(pre *components.TransactionPreAssembly) *TransactionBuilderForTesting {
	b.preAssembly = pre
	return b
}

// PrivateTransactionBuilder returns the private transaction builder so tests can configure PreAssembly, prepared transaction results, etc. before Build().
func (b *TransactionBuilderForTesting) PrivateTransactionBuilder() *testutil.PrivateTransactionBuilderForTesting {
	return b.privateTransactionBuilder
}

// Domain sets the transaction's domain on the private transaction.
func (b *TransactionBuilderForTesting) Domain(domain string) *TransactionBuilderForTesting {
	b.domain = domain
	return b
}

// Dependencies sets the transaction's dependencies.
func (b *TransactionBuilderForTesting) Dependencies(deps *pldapi.TransactionDependencies) *TransactionBuilderForTesting {
	b.dependencies = deps
	return b
}

// PostAssembly sets the transaction's PostAssembly on the private transaction. Pass nil to clear.
func (b *TransactionBuilderForTesting) PostAssembly(pa *components.TransactionPostAssembly) *TransactionBuilderForTesting {
	b.postAssembly = pa
	b.postAssemblySet = true
	return b
}

func (b *TransactionBuilderForTesting) Originator(originator *identityForTesting) *TransactionBuilderForTesting {
	b.originator = originator
	return b
}

func (b *TransactionBuilderForTesting) HeartbeatIntervalsSinceStateChange(heartbeatIntervalsSinceStateChange int) *TransactionBuilderForTesting {
	b.heartbeatIntervalsSinceStateChange = heartbeatIntervalsSinceStateChange
	return b
}

// SubmissionHash sets the transaction's latest submission hash (e.g. for State_Submitted/State_Dispatched). Overrides any default.
func (b *TransactionBuilderForTesting) SubmissionHash(hash pldtypes.Bytes32) *TransactionBuilderForTesting {
	b.latestSubmissionHash = &hash
	return b
}

func (b *TransactionBuilderForTesting) AddPendingAssemblyRequest() *TransactionBuilderForTesting {
	return b.AddPendingAssemblyRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error { return nil })
}

func (b *TransactionBuilderForTesting) AddPendingAssemblyRequestWithCallback(onSend func(ctx context.Context, idempotencyKey uuid.UUID) error) *TransactionBuilderForTesting {
	b.pendingAssemblyRequest = common.NewIdempotentRequest(b.t.Context(), &common.FakeClockForTesting{}, b.requestTimeout, onSend)
	return b
}

func (b *TransactionBuilderForTesting) AddPendingEndorsementRequest(endorsementName string, endorserIdentityLocator string) *TransactionBuilderForTesting {
	return b.AddPendingEndorsementRequestWithCallback(endorsementName, endorserIdentityLocator, func(ctx context.Context, idempotencyKey uuid.UUID) error { return nil })
}

func (b *TransactionBuilderForTesting) AddPendingEndorsementRequestWithCallback(endorsementName string, endorserIdentityLocator string, onSend func(ctx context.Context, idempotencyKey uuid.UUID) error) *TransactionBuilderForTesting {
	if b.pendingEndorsementRequests == nil {
		b.pendingEndorsementRequests = make(map[string]map[string]*common.IdempotentRequest)
	}
	if _, ok := b.pendingEndorsementRequests[endorsementName]; !ok {
		b.pendingEndorsementRequests[endorsementName] = make(map[string]*common.IdempotentRequest)
	}
	b.pendingEndorsementRequests[endorsementName][endorserIdentityLocator] = common.NewIdempotentRequest(b.t.Context(), &common.FakeClockForTesting{}, b.requestTimeout, onSend)
	return b
}

func (b *TransactionBuilderForTesting) AddPendingPreDispatchRequestWithCallback(onSend func(ctx context.Context, idempotencyKey uuid.UUID) error) *TransactionBuilderForTesting {
	b.pendingPreDispatchRequest = common.NewIdempotentRequest(b.t.Context(), &common.FakeClockForTesting{}, b.requestTimeout, onSend)
	return b
}

func (b *TransactionBuilderForTesting) AddPendingPreDispatchRequest() *TransactionBuilderForTesting {
	return b.AddPendingPreDispatchRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error { return nil })
}

func (b *TransactionBuilderForTesting) GetOriginator() *identityForTesting {
	return b.originator
}

func (b *TransactionBuilderForTesting) GetAssembleTimeout() int {
	return b.assembleTimeout
}

func (b *TransactionBuilderForTesting) GetRequestTimeout() int {
	return b.requestTimeout
}

func (b *TransactionBuilderForTesting) RequestTimeout(ms int) *TransactionBuilderForTesting {
	b.requestTimeout = ms
	return b
}

func (b *TransactionBuilderForTesting) AssembleTimeout(ms int) *TransactionBuilderForTesting {
	b.assembleTimeout = ms
	return b
}

func (b *TransactionBuilderForTesting) GetEndorsers() []string {
	endorsers := make([]string, b.privateTransactionBuilder.GetNumberOfEndorsers())
	for i := range endorsers {
		endorsers[i] = b.privateTransactionBuilder.GetEndorserIdentityLocator(i)
	}
	return endorsers
}

func (b *TransactionBuilderForTesting) GetEndorsementRequest(endorsementName string, endorserIdentityLocator string) *common.IdempotentRequest {
	if b.pendingEndorsementRequests == nil {
		return nil
	}
	if _, ok := b.pendingEndorsementRequests[endorsementName]; !ok {
		return nil
	}
	return b.pendingEndorsementRequests[endorsementName][endorserIdentityLocator]
}

func (b *TransactionBuilderForTesting) GetPendingAssemblyRequest() *common.IdempotentRequest {
	return b.pendingAssemblyRequest
}

func (b *TransactionBuilderForTesting) GetPendingPreDispatchRequest() *common.IdempotentRequest {
	return b.pendingPreDispatchRequest
}

type transactionDependencyFakes struct {
	SentMessageRecorder *SentMessageRecorder
	TransportWriter     *transport.MockTransportWriter
	Clock               *common.FakeClockForTesting
	EngineIntegration   *common.MockEngineIntegration
	SyncPoints          *syncpoints.MockSyncPoints
	DomainAPI           *componentsmocks.DomainSmartContract
	Domain              *componentsmocks.Domain
	TXManager           *componentsmocks.TXManager
	KeyManager          *componentsmocks.KeyManager
	PublicTxManager     *componentsmocks.PublicTxManager
	DomainContext       components.DomainContext
}

func (b *TransactionBuilderForTesting) Build(ctx context.Context) (*CoordinatorTransaction, *transactionDependencyFakes) {
	if b.grapher == nil {
		b.grapher = NewGrapher(ctx)
	}
	metrics := metrics.InitMetrics(ctx, prometheus.NewRegistry())

	privateTransaction := b.privateTransactionBuilder.Build()

	var transportWriter transport.TransportWriter
	fakeClock := &common.FakeClockForTesting{}
	mocks := &transactionDependencyFakes{
		Clock:             fakeClock,
		EngineIntegration: common.NewMockEngineIntegration(b.t),
		SyncPoints:        &syncpoints.MockSyncPoints{},
		Domain:            componentsmocks.NewDomain(b.t),
		TXManager:         componentsmocks.NewTXManager(b.t),
		KeyManager:        componentsmocks.NewKeyManager(b.t),
		PublicTxManager:   componentsmocks.NewPublicTxManager(b.t),
		DomainContext:     componentsmocks.NewDomainContext(b.t),
	}
	if b.domainAPI != nil {
		mocks.DomainAPI = b.domainAPI
	} else {
		mocks.DomainAPI = componentsmocks.NewDomainSmartContract(b.t)
		mocks.DomainAPI.EXPECT().Domain().Return(mocks.Domain).Maybe()
	}
	if b.transportWriter != nil {
		mocks.TransportWriter = b.transportWriter
		transportWriter = b.transportWriter
	} else {
		mocks.SentMessageRecorder = NewSentMessageRecorder()
		transportWriter = mocks.SentMessageRecorder
	}
	if b.domainAPI == nil {
		mocks.DomainAPI.EXPECT().Domain().Return(mocks.Domain).Maybe()
	}

	txn, err := NewTransaction(
		ctx,
		b.originator.identityLocator,
		privateTransaction,
		false, // hasChainedTransaction
		"",    // nodeName
		transportWriter,
		fakeClock,
		func(ctx context.Context, event common.Event) {
			// No-op event handler for tests
		},
		mocks.EngineIntegration,
		mocks.SyncPoints,
		fakeClock.Duration(b.requestTimeout),
		fakeClock.Duration(b.assembleTimeout),
		5,
		b.grapher,
		metrics,
		mocks.KeyManager,
		mocks.PublicTxManager,
		mocks.TXManager,
		mocks.DomainAPI,
		mocks.DomainContext,
		func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error) {
			// No-op nullifier builder for tests
			return nil, nil
		},
		nil,
	)
	if err != nil {
		panic(fmt.Sprintf("Error from NewTransaction: %v", err))
	}
	b.txn = txn

	//Update the private transaction struct to the accumulation that resulted from what ever events that we expect to have happened leading up to the current state
	// We don't attempt to emulate any other history of those past events but rather assert that the state machine's behavior is determined purely by its current finite state
	// and the contents of the PrivateTransaction struct

	if b.state == State_Endorsement_Gathering ||
		b.state == State_Blocked ||
		b.state == State_Confirming_Dispatchable ||
		b.state == State_Ready_For_Dispatch {

		mocks.EngineIntegration.EXPECT().WriteLockStatesForTransaction(ctx, mock.Anything).Return(nil)
		err := b.txn.applyPostAssembly(ctx, b.BuildPostAssembly(), uuid.New())
		if err != nil {
			panic("error from applyPostAssembly")
		}
	}

	if b.state == State_Confirmed ||
		b.state == State_Reverted {
		b.txn.heartbeatIntervalsSinceStateChange = b.heartbeatIntervalsSinceStateChange
	}

	b.txn.signerAddress = b.signerAddress
	b.txn.latestSubmissionHash = b.latestSubmissionHash
	b.txn.nonce = b.nonce
	b.txn.stateMachine.CurrentState = b.state
	b.txn.dynamicSigningIdentity = false
	b.txn.pendingAssembleRequest = b.pendingAssemblyRequest
	b.txn.pendingPreDispatchRequest = b.pendingPreDispatchRequest
	if b.pendingEndorsementRequests != nil {
		b.txn.pendingEndorsementRequests = b.pendingEndorsementRequests
		b.txn.cancelEndorsementRequestTimeoutSchedule = func() {}
	}
	if b.originatorNode != "" {
		b.txn.originatorNode = b.originatorNode
	}
	if b.preAssembly != nil {
		b.txn.pt.PreAssembly = b.preAssembly
	}
	if b.domain != "" {
		b.txn.pt.Domain = b.domain
	}
	if b.dependencies != nil {
		b.txn.dependencies = b.dependencies
	}
	if b.postAssemblySet {
		b.txn.pt.PostAssembly = b.postAssembly
	}
	return b.txn, mocks

}

func (b *TransactionBuilderForTesting) BuildEndorsedEvent(endorserIndex int) *EndorsedEvent {

	return &EndorsedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: b.txn.GetID(),
		},
		RequestID:   b.txn.pendingEndorsementRequests[b.privateTransactionBuilder.GetEndorsementName(endorserIndex)][b.privateTransactionBuilder.GetEndorserIdentityLocator(endorserIndex)].IdempotencyKey(),
		Endorsement: b.privateTransactionBuilder.BuildEndorsement(endorserIndex),
	}

}

func (b *TransactionBuilderForTesting) BuildEndorseRejectedEvent(endorserIndex int) *EndorsedRejectedEvent {

	attReqName := fmt.Sprintf("endorse-%d", endorserIndex)
	return &EndorsedRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: b.txn.GetID(),
		},
		RevertReason:           "some reason for rejection",
		AttestationRequestName: attReqName,
		RequestID:              b.txn.pendingEndorsementRequests[attReqName][b.privateTransactionBuilder.GetEndorserIdentityLocator(endorserIndex)].IdempotencyKey(),
		Party:                  b.privateTransactionBuilder.GetEndorserIdentityLocator(endorserIndex),
	}

}

func (b *TransactionBuilderForTesting) BuildPostAssembly() *components.TransactionPostAssembly {
	return b.privateTransactionBuilder.BuildPostAssembly()
}

func (b *TransactionBuilderForTesting) BuildPreAssembly() *components.TransactionPreAssembly {
	return b.privateTransactionBuilder.BuildPreAssembly()
}
