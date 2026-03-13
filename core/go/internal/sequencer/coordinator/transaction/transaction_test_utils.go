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
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// pendingEndorsementRequestAddition is used by the builder to add one pending endorsement request (builder creates IdempotentRequest from clock/requestTimeout).
type pendingEndorsementRequestAddition struct {
	attName string
	party   string
	sendFn  func(context.Context, uuid.UUID) error
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

func (r *SentMessageRecorder) StartLoopbackWriter() {
}

func (r *SentMessageRecorder) WaitForDone(ctx context.Context) {
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

func (r *SentMessageRecorder) SendTransactionConfirmed(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, nonce *pldtypes.HexUint64, revertReason pldtypes.HexBytes, willRetry bool) error {
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
	originator                         string
	queueEventForCoordinator           func(context.Context, common.Event)
	domainSigningIdentity              string
	coordinatorSigningIdentity         string
	signerAddress                      *pldtypes.EthAddress
	latestSubmissionHash               *pldtypes.Bytes32
	nonce                              *uint64
	revertReason                       pldtypes.HexBytes
	errorCount                         int
	dependencies                       *pldapi.TransactionDependencies
	state                              State
	useMockTransportWriter             bool
	useMockClock                       bool
	grapher                            Grapher
	txn                                *coordinatorTransaction
	requestTimeout                     int
	stateTimeout                       int
	finalizingGracePeriod              int
	confirmedLockRetentionGracePeriod  int
	heartbeatIntervalsSinceStateChange int
	cancelRequestTimeoutSchedule       func()
	cancelStateTimeoutSchedule         func()
	pendingAssembleRequestSend         func(context.Context, uuid.UUID) error // if set, builder builds IdempotentRequest from clock/requestTimeout
	pendingEndorsementRequestAdditions []pendingEndorsementRequestAddition
	pendingPreDispatchRequestSend      func(context.Context, uuid.UUID) error // if set, builder builds IdempotentRequest from clock/requestTimeout
	confirmedLocksReleased             bool
	submitterSelection                 prototk.ContractConfig_SubmitterSelection
	nodeName                           string
	baseLedgerRevertRetryThreshold     int
	revertCount                        int
}

// Function NewTransactionBuilderForTesting creates a TransactionBuilderForTesting with random values for all fields
// use the builder methods to set specific values for fields before calling Build to create a new Transaction
func NewTransactionBuilderForTesting(t *testing.T, state State) *TransactionBuilderForTesting {
	builder := &TransactionBuilderForTesting{
		t:                                 t,
		originator:                        "sender@node1",
		queueEventForCoordinator:          func(context.Context, common.Event) {},
		signerAddress:                     nil,
		latestSubmissionHash:              nil,
		state:                             state,
		stateTimeout:                      5000,
		requestTimeout:                    1000,
		finalizingGracePeriod:             5,
		confirmedLockRetentionGracePeriod: 1,
		privateTransactionBuilder:         testutil.NewPrivateTransactionBuilderForTesting(),
		submitterSelection:                prototk.ContractConfig_SUBMITTER_COORDINATOR,
		nodeName:                          "node1",
	}

	switch state {
	case State_Dispatched:
		nonce := rand.Uint64()
		builder.nonce = &nonce
		builder.signerAddress = pldtypes.RandAddress()
		latestSubmissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
		builder.latestSubmissionHash = &latestSubmissionHash
		builder.privateTransactionBuilder.EndorsementComplete()
	case State_Endorsement_Gathering:
		//fine grained detail in this state needed to emulate what has already happened wrt endorsement requests and responses so far
	case State_Blocked, State_Confirming_Dispatchable, State_Ready_For_Dispatch, State_Confirmed:
		//we are emulating a transaction that has been passed State_Endorsement_Gathering so default to complete attestation plan
		builder.privateTransactionBuilder.EndorsementComplete()
	}
	return builder
}

func (b *TransactionBuilderForTesting) UseMockTransportWriter() *TransactionBuilderForTesting {
	b.useMockTransportWriter = true
	return b
}

func (b *TransactionBuilderForTesting) UseMockClock() *TransactionBuilderForTesting {
	b.useMockClock = true
	return b
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

func (b *TransactionBuilderForTesting) Originator(originator string) *TransactionBuilderForTesting {
	b.originator = originator
	return b
}

func (b *TransactionBuilderForTesting) NodeName(nodeName string) *TransactionBuilderForTesting {
	b.nodeName = nodeName
	return b
}

func (b *TransactionBuilderForTesting) HeartbeatIntervalsSinceStateChange(heartbeatIntervalsSinceStateChange int) *TransactionBuilderForTesting {
	b.heartbeatIntervalsSinceStateChange = heartbeatIntervalsSinceStateChange
	return b
}

func (b *TransactionBuilderForTesting) DomainSigningIdentity(domainSigningIdentity string) *TransactionBuilderForTesting {
	b.domainSigningIdentity = domainSigningIdentity
	return b
}

func (b *TransactionBuilderForTesting) CoordinatorSigningIdentity(identity string) *TransactionBuilderForTesting {
	b.coordinatorSigningIdentity = identity
	return b
}

func (b *TransactionBuilderForTesting) QueueEventForCoordinator(queueFn func(context.Context, common.Event)) *TransactionBuilderForTesting {
	b.queueEventForCoordinator = queueFn
	return b
}

func (b *TransactionBuilderForTesting) RequestTimeout(requestTimeout int) *TransactionBuilderForTesting {
	b.requestTimeout = requestTimeout
	return b
}

func (b *TransactionBuilderForTesting) StateTimeout(stateTimeout int) *TransactionBuilderForTesting {
	b.stateTimeout = stateTimeout
	return b
}

func (b *TransactionBuilderForTesting) FinalizingGracePeriod(finalizingGracePeriod int) *TransactionBuilderForTesting {
	b.finalizingGracePeriod = finalizingGracePeriod
	return b
}

func (b *TransactionBuilderForTesting) ConfirmedLockRetentionGracePeriod(gracePeriod int) *TransactionBuilderForTesting {
	b.confirmedLockRetentionGracePeriod = gracePeriod
	return b
}

func (b *TransactionBuilderForTesting) CurrentState(state State) *TransactionBuilderForTesting {
	b.state = state
	return b
}

func (b *TransactionBuilderForTesting) SignerAddress(address *pldtypes.EthAddress) *TransactionBuilderForTesting {
	b.signerAddress = address
	return b
}

func (b *TransactionBuilderForTesting) Nonce(nonce *uint64) *TransactionBuilderForTesting {
	b.nonce = nonce
	return b
}

// SubmissionHash sets the transaction's latest submission hash (e.g. for State_Dispatched). Overrides any default.
func (b *TransactionBuilderForTesting) SubmissionHash(hash pldtypes.Bytes32) *TransactionBuilderForTesting {
	b.latestSubmissionHash = &hash
	return b
}

func (b *TransactionBuilderForTesting) LatestSubmissionHash(hash *pldtypes.Bytes32) *TransactionBuilderForTesting {
	b.latestSubmissionHash = hash
	return b
}

func (b *TransactionBuilderForTesting) RevertReason(revertReason pldtypes.HexBytes) *TransactionBuilderForTesting {
	b.revertReason = revertReason
	return b
}

func (b *TransactionBuilderForTesting) ErrorCount(errorCount int) *TransactionBuilderForTesting {
	b.errorCount = errorCount
	return b
}

func (b *TransactionBuilderForTesting) Dependencies(dependencies *pldapi.TransactionDependencies) *TransactionBuilderForTesting {
	b.dependencies = dependencies
	return b
}

// AddPendingAssembleRequest adds a pending assemble request with a no-op send callback; the builder creates the
// IdempotentRequest in Build() using its clock and requestTimeout.
func (b *TransactionBuilderForTesting) AddPendingAssembleRequest() *TransactionBuilderForTesting {
	return b.AddPendingAssembleRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error { return nil })
}

// AddPendingAssembleRequestWithCallback sets the send function for the assemble request; the builder will create the
// IdempotentRequest using its clock and requestTimeout in Build(), so tests need not construct the request.
func (b *TransactionBuilderForTesting) AddPendingAssembleRequestWithCallback(sendFn func(context.Context, uuid.UUID) error) *TransactionBuilderForTesting {
	b.pendingAssembleRequestSend = sendFn
	return b
}

func (b *TransactionBuilderForTesting) AddPendingEndorsementRequest(index int) *TransactionBuilderForTesting {
	return b.AddPendingEndorsementRequestWithCallback(index, func(ctx context.Context, idempotencyKey uuid.UUID) error { return nil })
}

// AddPendingEndorsementRequestWithCallback adds one pending endorsement request; the builder will create the IdempotentRequest
// using its clock and requestTimeout in Build(). The map is always initialised on the transaction (empty if no additions).
func (b *TransactionBuilderForTesting) AddPendingEndorsementRequestWithCallback(index int, sendFn func(context.Context, uuid.UUID) error) *TransactionBuilderForTesting {
	b.pendingEndorsementRequestAdditions = append(b.pendingEndorsementRequestAdditions, pendingEndorsementRequestAddition{
		attName: fmt.Sprintf("endorse-%d", index),
		party:   fmt.Sprintf("endorser-%d@node-%d", index, index),
		sendFn:  sendFn,
	})
	return b
}

func (b *TransactionBuilderForTesting) AddPendingPreDispatchRequest() *TransactionBuilderForTesting {
	return b.AddPendingPreDispatchRequestWithCallback(func(ctx context.Context, idempotencyKey uuid.UUID) error { return nil })
}

// AddPendingPreDispatchRequestWithCallback sets the send function for the pre-dispatch request; the builder will create the
// IdempotentRequest using its clock and requestTimeout in Build(), so tests need not construct the request.
func (b *TransactionBuilderForTesting) AddPendingPreDispatchRequestWithCallback(sendFn func(context.Context, uuid.UUID) error) *TransactionBuilderForTesting {
	b.pendingPreDispatchRequestSend = sendFn
	return b
}

func (b *TransactionBuilderForTesting) CancelRequestTimeoutSchedule(cancel func()) *TransactionBuilderForTesting {
	b.cancelRequestTimeoutSchedule = cancel
	return b
}

func (b *TransactionBuilderForTesting) CancelStateTimeoutSchedule(cancel func()) *TransactionBuilderForTesting {
	b.cancelStateTimeoutSchedule = cancel
	return b
}

func (b *TransactionBuilderForTesting) ConfirmedLocksReleased(released bool) *TransactionBuilderForTesting {
	b.confirmedLocksReleased = released
	return b
}

func (b *TransactionBuilderForTesting) BaseLedgerRevertRetryThreshold(threshold int) *TransactionBuilderForTesting {
	b.baseLedgerRevertRetryThreshold = threshold
	return b
}

func (b *TransactionBuilderForTesting) RevertCount(count int) *TransactionBuilderForTesting {
	b.revertCount = count
	return b
}

func (b *TransactionBuilderForTesting) SubmitterSelection(selection prototk.ContractConfig_SubmitterSelection) *TransactionBuilderForTesting {
	b.submitterSelection = selection
	return b
}

func (b *TransactionBuilderForTesting) TransactionID(id uuid.UUID) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.ID(id)
	return b
}

func (b *TransactionBuilderForTesting) Domain(domain string) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.Domain(domain)
	return b
}

func (b *TransactionBuilderForTesting) Address(address pldtypes.EthAddress) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.Address(address)
	return b
}

func (b *TransactionBuilderForTesting) PreAssembly(preAssembly *components.TransactionPreAssembly) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.PreAssembly(preAssembly)
	return b
}

func (b *TransactionBuilderForTesting) PostAssembly(postAssembly *components.TransactionPostAssembly) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.PostAssembly(postAssembly)
	return b
}

func (b *TransactionBuilderForTesting) PreparedPrivateTransaction(tx *pldapi.TransactionInput) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.PreparedPrivateTransaction(tx)
	return b
}

func (b *TransactionBuilderForTesting) PreparedPublicTransaction(tx *pldapi.TransactionInput) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.PreparedPublicTransaction(tx)
	return b
}

func (b *TransactionBuilderForTesting) Signer(signer string) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.Signer(signer)
	return b
}

func (b *TransactionBuilderForTesting) GetStateTimeout() int {
	return b.stateTimeout
}

func (b *TransactionBuilderForTesting) GetRequestTimeout() int {
	return b.requestTimeout
}

func (b *TransactionBuilderForTesting) GetEndorsers() []string {
	endorsers := make([]string, b.privateTransactionBuilder.GetNumberOfEndorsers())
	for i := range endorsers {
		endorsers[i] = b.privateTransactionBuilder.GetEndorserIdentityLocator(i)
	}
	return endorsers
}

type transactionDependencyMocks struct {
	TransportWriter     *transport.MockTransportWriter
	Clock               *common.MockClock
	EngineIntegration   *common.MockEngineIntegration
	SentMessageRecorder *SentMessageRecorder
	SyncPoints          *syncpoints.MockSyncPoints
	AllComponents       *componentsmocks.AllComponents
	DomainAPI           *componentsmocks.DomainSmartContract
	Domain              *componentsmocks.Domain
	DomainContext       *componentsmocks.DomainContext
	KeyManager          *componentsmocks.KeyManager
	PublicTxManager     *componentsmocks.PublicTxManager
	TXManager           *componentsmocks.TXManager
	SequenceManager     *componentsmocks.SequencerManager
	DB                  sqlmock.Sqlmock
}

func (b *TransactionBuilderForTesting) Build() (*coordinatorTransaction, *transactionDependencyMocks) {
	ctx := context.Background()
	if b.grapher == nil {
		b.grapher = NewGrapher(ctx)
	}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(b.t, err)

	mocks := &transactionDependencyMocks{
		TransportWriter:     transport.NewMockTransportWriter(b.t),
		Clock:               common.NewMockClock(b.t),
		EngineIntegration:   common.NewMockEngineIntegration(b.t),
		SentMessageRecorder: NewSentMessageRecorder(),
		SyncPoints:          syncpoints.NewMockSyncPoints(b.t),
		AllComponents:       componentsmocks.NewAllComponents(b.t),
		KeyManager:          componentsmocks.NewKeyManager(b.t),
		PublicTxManager:     componentsmocks.NewPublicTxManager(b.t),
		TXManager:           componentsmocks.NewTXManager(b.t),
		SequenceManager:     componentsmocks.NewSequencerManager(b.t),
		DomainAPI:           componentsmocks.NewDomainSmartContract(b.t),
		Domain:              componentsmocks.NewDomain(b.t),
		DomainContext:       componentsmocks.NewDomainContext(b.t),
		DB:                  mp.Mock,
	}

	// link the mocks which return other mocks
	mocks.AllComponents.On("KeyManager").Return(mocks.KeyManager).Maybe()
	mocks.AllComponents.On("PublicTxManager").Return(mocks.PublicTxManager).Maybe()
	mocks.AllComponents.On("TxManager").Return(mocks.TXManager).Maybe()
	mocks.AllComponents.On("SequencerManager").Return(mocks.SequenceManager).Maybe()
	mocks.AllComponents.On("Persistence").Return(mp.P).Maybe()
	mocks.DomainAPI.On("Domain").Return(mocks.Domain).Maybe()

	// create the mocks needed for the NewTransaction call below
	// the return values of these can be set by builder methods if needed
	mocks.Domain.On("FixedSigningIdentity").Return("")
	mocks.DomainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		SubmitterSelection: b.submitterSelection,
	})

	privateTransaction := b.privateTransactionBuilder.Build()

	var transportWriter transport.TransportWriter
	if b.useMockTransportWriter {
		transportWriter = mocks.TransportWriter
	} else {
		transportWriter = mocks.SentMessageRecorder
	}

	var clock common.Clock
	if b.useMockClock {
		clock = mocks.Clock
		// newTransaction results in a call to clock.Now(), so we need to mock that once
		mocks.Clock.On("Now").Return(time.Now()).Once()
	} else {
		clock = common.RealClock()
	}

	txn, err := newTransaction(
		ctx,
		b.originator,
		b.nodeName,
		privateTransaction,
		b.coordinatorSigningIdentity,
		transportWriter,
		clock,
		b.queueEventForCoordinator,
		mocks.EngineIntegration,
		mocks.SyncPoints,
		mocks.AllComponents,
		mocks.DomainAPI,
		mocks.DomainContext,
		time.Duration(b.requestTimeout),
		time.Duration(b.stateTimeout),
		b.finalizingGracePeriod,
		b.confirmedLockRetentionGracePeriod,
		b.baseLedgerRevertRetryThreshold,
		b.grapher,
		metrics.InitMetrics(ctx, prometheus.NewRegistry()),
	)
	require.NoError(b.t, err)

	txn.signerAddress = b.signerAddress
	txn.domainSigningIdentity = b.domainSigningIdentity
	txn.latestSubmissionHash = b.latestSubmissionHash
	txn.nonce = b.nonce
	txn.heartbeatIntervalsSinceStateChange = b.heartbeatIntervalsSinceStateChange
	txn.cancelRequestTimeoutSchedule = b.cancelRequestTimeoutSchedule
	txn.cancelStateTimeoutSchedule = b.cancelStateTimeoutSchedule
	txn.confirmedLocksReleased = b.confirmedLocksReleased
	txn.stateMachine.CurrentState = b.state
	txn.revertReason = b.revertReason
	txn.revertCount = b.revertCount

	if b.dependencies != nil {
		txn.dependencies = b.dependencies
	}
	if b.pendingAssembleRequestSend != nil {
		txn.pendingAssembleRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, b.pendingAssembleRequestSend)
	}

	if len(b.pendingEndorsementRequestAdditions) > 0 {
		txn.pendingEndorsementRequests = make(map[string]map[string]*common.IdempotentRequest)
		for _, add := range b.pendingEndorsementRequestAdditions {
			if txn.pendingEndorsementRequests[add.attName] == nil {
				txn.pendingEndorsementRequests[add.attName] = make(map[string]*common.IdempotentRequest)
			}
			txn.pendingEndorsementRequests[add.attName][add.party] = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, add.sendFn)
		}
	}

	if b.pendingPreDispatchRequestSend != nil {
		txn.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, b.pendingPreDispatchRequestSend)
	}

	if privateTransaction.PostAssembly != nil {
		if _, isRealGrapher := b.grapher.(*grapher); isRealGrapher {
			for _, state := range privateTransaction.PostAssembly.OutputStates {
				err := b.grapher.AddMinter(ctx, state.ID, txn)
				require.NoError(b.t, err)
			}
		}
	}

	b.txn = txn
	return b.txn, mocks
}

// BuildAssembleSuccessEvent returns an AssembleSuccessEvent for the transaction built by this builder.
// The builder must have added a pending assemble request (e.g. AddPendingAssembleRequest()) before Build().
func (b *TransactionBuilderForTesting) BuildAssembleSuccessEvent() *AssembleSuccessEvent {
	return &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: b.txn.pt.ID,
		},
		PostAssembly: b.BuildPostAssembly(),
		PreAssembly:  b.BuildPreAssembly(),
		RequestID:    b.txn.pendingAssembleRequest.IdempotencyKey(),
	}
}

func (b *TransactionBuilderForTesting) BuildAssembleRevertEvent() *AssembleRevertResponseEvent {
	return &AssembleRevertResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: b.txn.pt.ID,
		},
		PostAssembly: b.BuildPostAssembly(),
		RequestID:    b.txn.pendingAssembleRequest.IdempotencyKey(),
	}
}

func (b *TransactionBuilderForTesting) BuildEndorsedEvent(endorserIndex int) *EndorsedEvent {

	return &EndorsedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: b.txn.pt.ID,
		},
		RequestID:   b.txn.pendingEndorsementRequests[b.privateTransactionBuilder.GetEndorsementName(endorserIndex)][b.privateTransactionBuilder.GetEndorserIdentityLocator(endorserIndex)].IdempotencyKey(),
		Endorsement: b.privateTransactionBuilder.BuildEndorsement(endorserIndex),
	}

}

func (b *TransactionBuilderForTesting) BuildEndorseRejectedEvent(endorserIndex int) *EndorsedRejectedEvent {

	attReqName := fmt.Sprintf("endorse-%d", endorserIndex)
	return &EndorsedRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: b.txn.pt.ID,
		},
		RevertReason:           "some reason for rejection",
		AttestationRequestName: attReqName,
		RequestID:              b.txn.pendingEndorsementRequests[attReqName][b.privateTransactionBuilder.GetEndorserIdentityLocator(endorserIndex)].IdempotencyKey(),
		Party:                  b.privateTransactionBuilder.GetEndorserIdentityLocator(endorserIndex),
	}

}

func (b *TransactionBuilderForTesting) BuildDispatchRequestApprovedEvent() *DispatchRequestApprovedEvent {
	return &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: b.txn.pt.ID,
		},
		RequestID: b.txn.pendingPreDispatchRequest.IdempotencyKey(),
	}
}

func (b *TransactionBuilderForTesting) BuildPostAssembly() *components.TransactionPostAssembly {
	return b.privateTransactionBuilder.BuildPostAssembly()
}

func (b *TransactionBuilderForTesting) BuildPreAssembly() *components.TransactionPreAssembly {
	return b.privateTransactionBuilder.BuildPreAssembly()
}
