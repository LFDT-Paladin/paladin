//go:build !generate_mocks

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
	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/dependencytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/statevisibilitytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencercommonmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencertransportmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/syncpointsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// pendingEndorsementRequestAddition is used by the builder to add one pending endorsement request (builder creates IdempotentRequest from clock/requestTimeout).
type pendingEndorsementRequestAddition struct {
	attName string
	party   string
	sendFn  func(context.Context, uuid.UUID) error
}

type TransactionBuilderForTesting struct {
	t                                  *testing.T
	privateTransactionBuilder          *testutil.PrivateTransactionBuilderForTesting
	coordinatorTransactions            map[uuid.UUID]CoordinatorTransaction
	originator                         string
	originatorNode                     string
	queueEventForCoordinator           func(context.Context, common.Event)
	setDispatchedInFlight              func(txID uuid.UUID, inFlight bool)
	domainSigningIdentity              string
	coordinatorSigningIdentity         string
	signerAddress                      *pldtypes.EthAddress
	latestSubmissionHash               *pldtypes.Bytes32
	nonce                              *uint64
	revertReason                       pldtypes.HexBytes
	errorCount                         int
	state                              State
	useMockTransportWriter             bool
	useMockClock                       bool
	grapher                            grapher.Grapher
	stateVisibilityTracker             statevisibilitytracker.StateVisibilityStore
	dependencyTracker                  dependencytracker.DependencyTracker
	txn                                *coordinatorTransaction
	requestTimeout                     int
	stateTimeout                       int
	finalizingGracePeriod              int
	heartbeatIntervalsSinceStateChange int
	cancelRequestTimeoutSchedule       func()
	cancelStateTimeoutSchedule         func()
	pendingAssembleRequestSend         func(context.Context, uuid.UUID) error // if set, builder builds IdempotentRequest from clock/requestTimeout
	pendingEndorsementRequestAdditions []pendingEndorsementRequestAddition
	pendingPreDispatchRequestSend      func(context.Context, uuid.UUID) error // if set, builder builds IdempotentRequest from clock/requestTimeout
	submitterSelection                 prototk.ContractConfig_SubmitterSelection
	nodeName                           string
	baseLedgerRevertRetryThreshold     int
	assembleErrorCount                 int
	assembleErrorRetryThreshhold       int
	signErrorCount                     int
	signErrorRetryThreshhold           int
	endorseToleranceByRequirement      map[string]int
	revertCount                        int
	currentBlockHeight                 int64
	blockHeightTolerance               uint64
}

// Function NewTransactionBuilderForTesting creates a TransactionBuilderForTesting with random values for all fields
// use the builder methods to set specific values for fields before calling Build to create a new Transaction
func NewTransactionBuilderForTesting(t *testing.T, state State) *TransactionBuilderForTesting {
	builder := &TransactionBuilderForTesting{
		t:                         t,
		originator:                "sender@node1",
		originatorNode:            "node1",
		queueEventForCoordinator:  func(context.Context, common.Event) {},
		setDispatchedInFlight:     func(uuid.UUID, bool) {},
		signerAddress:             nil,
		latestSubmissionHash:      nil,
		state:                     state,
		stateTimeout:              5000,
		requestTimeout:            1000,
		finalizingGracePeriod:     5,
		privateTransactionBuilder: testutil.NewPrivateTransactionBuilderForTesting(),
		submitterSelection:        prototk.ContractConfig_SUBMITTER_COORDINATOR,
		nodeName:                  "node1",
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

func (b *TransactionBuilderForTesting) WithCurrentBlockHeight(blockHeight int64) *TransactionBuilderForTesting {
	b.currentBlockHeight = blockHeight
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

func (b *TransactionBuilderForTesting) CoordinatorTransactions(coordinatorTransactions map[uuid.UUID]CoordinatorTransaction) *TransactionBuilderForTesting {
	b.coordinatorTransactions = coordinatorTransactions
	return b
}

// coordinatorTransactionStateLookup builds getCoordinatorTransactionState from transactions indexed by ID.
func coordinatorTransactionStateLookup(byID map[uuid.UUID]CoordinatorTransaction) func(context.Context, uuid.UUID) (State, bool) {
	return func(_ context.Context, id uuid.UUID) (State, bool) {
		if byID == nil {
			return State(0), false
		}
		ct := byID[id]
		if ct == nil {
			return State(0), false
		}
		return ct.GetCurrentState(), true
	}
}

func coordinatorTransactionHandleEvent(byID map[uuid.UUID]CoordinatorTransaction) func(context.Context, uuid.UUID, common.Event) error {
	return func(ctx context.Context, id uuid.UUID, event common.Event) error {
		if byID == nil {
			return i18n.NewError(ctx, msgs.MsgSequencerTransactionNotFound, id)
		}
		ct := byID[id]
		if ct == nil {
			return i18n.NewError(ctx, msgs.MsgSequencerTransactionNotFound, id)
		}
		return ct.HandleEvent(ctx, event)
	}
}

func (b *TransactionBuilderForTesting) NumberOfEndorsements(num int) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.NumberOfEndorsements(num)
	return b
}

func (b *TransactionBuilderForTesting) NumberOfOutputStates(num int) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.NumberOfOutputStates(num)
	return b
}

func (b *TransactionBuilderForTesting) InputStateIDs(stateIDs ...string) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.InputStateIDs(stateIDs...)
	return b
}

func (b *TransactionBuilderForTesting) ReadStateIDs(stateIDs ...string) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.ReadStateIDs(stateIDs...)
	return b
}

func (b *TransactionBuilderForTesting) ChainedDependencies(transactionIDs ...uuid.UUID) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.ChainedDependencies(transactionIDs...)
	return b
}

func (b *TransactionBuilderForTesting) Reverts(revertReason string) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.Reverts(revertReason)
	return b
}

func (b *TransactionBuilderForTesting) Grapher(grapher grapher.Grapher) *TransactionBuilderForTesting {
	b.grapher = grapher
	return b
}

func (b *TransactionBuilderForTesting) DependencyTracker(dependencyTracker dependencytracker.DependencyTracker) *TransactionBuilderForTesting {
	b.dependencyTracker = dependencyTracker
	return b
}

func (b *TransactionBuilderForTesting) StateVisibility(stateVisibilityTracker statevisibilitytracker.StateVisibilityStore) *TransactionBuilderForTesting {
	b.stateVisibilityTracker = stateVisibilityTracker
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

func (b *TransactionBuilderForTesting) SetDispatchedInFlight(fn func(txID uuid.UUID, inFlight bool)) *TransactionBuilderForTesting {
	b.setDispatchedInFlight = fn
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

func (b *TransactionBuilderForTesting) AddPendingEndorsementRequest() *TransactionBuilderForTesting {
	return b.AddPendingEndorsementRequestWithCallback(0, func(ctx context.Context, idempotencyKey uuid.UUID) error { return nil })
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

func (b *TransactionBuilderForTesting) BaseLedgerRevertRetryThreshold(threshold int) *TransactionBuilderForTesting {
	b.baseLedgerRevertRetryThreshold = threshold
	return b
}

func (b *TransactionBuilderForTesting) AssembleErrorCount(count int) *TransactionBuilderForTesting {
	b.assembleErrorCount = count
	return b
}

func (b *TransactionBuilderForTesting) AssembleErrorRetryThreshold(threshold int) *TransactionBuilderForTesting {
	b.assembleErrorRetryThreshhold = threshold
	return b
}

func (b *TransactionBuilderForTesting) SignErrorCount(count int) *TransactionBuilderForTesting {
	b.signErrorCount = count
	return b
}

func (b *TransactionBuilderForTesting) SignErrorRetryThreshold(threshold int) *TransactionBuilderForTesting {
	b.signErrorRetryThreshhold = threshold
	return b
}

func (b *TransactionBuilderForTesting) EndorseTolerance(tolerance int) *TransactionBuilderForTesting {
	b.endorseToleranceByRequirement = map[string]int{"endorse-0": tolerance}
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

func (b *TransactionBuilderForTesting) PreAssembly(preAssembly *prototk.TransactionPreAssembly) *TransactionBuilderForTesting {
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
	TransportWriter     *sequencertransportmocks.TransportWriter
	Clock               *sequencercommonmocks.Clock
	EngineIntegration   *sequencercommonmocks.EngineIntegration
	SentMessageRecorder *testutil.SentMessageRecorder
	SyncPoints          *syncpointsmocks.SyncPoints
	AllComponents       *componentsmocks.AllComponents
	DomainAPI           *componentsmocks.DomainSmartContract
	Domain              *componentsmocks.Domain
	DomainStateWriter   *componentsmocks.DomainStateWriter
	StateManager        *componentsmocks.StateManager
	DomainQueryContext  *componentsmocks.DomainQueryContext
	KeyManager          *componentsmocks.KeyManager
	PublicTxManager     *componentsmocks.PublicTxManager
	TXManager           *componentsmocks.TXManager
	SequenceManager     *componentsmocks.SequencerManager
	DB                  sqlmock.Sqlmock
}

func (b *TransactionBuilderForTesting) Build() (*coordinatorTransaction, *transactionDependencyMocks) {
	ctx := b.t.Context()
	if b.dependencyTracker == nil {
		b.dependencyTracker = dependencytracker.NewDependencyTracker()
	}
	if b.stateVisibilityTracker == nil {
		b.stateVisibilityTracker = statevisibilitytracker.NewStore()
	}
	if b.grapher == nil {
		b.grapher = grapher.NewGrapher(b.dependencyTracker, b.stateVisibilityTracker, 5)
	}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(b.t, err)

	mocks := &transactionDependencyMocks{
		TransportWriter:     sequencertransportmocks.NewTransportWriter(b.t),
		Clock:               sequencercommonmocks.NewClock(b.t),
		EngineIntegration:   sequencercommonmocks.NewEngineIntegration(b.t),
		SentMessageRecorder: testutil.NewSentMessageRecorder(),
		SyncPoints:          syncpointsmocks.NewSyncPoints(b.t),
		AllComponents:       componentsmocks.NewAllComponents(b.t),
		KeyManager:          componentsmocks.NewKeyManager(b.t),
		PublicTxManager:     componentsmocks.NewPublicTxManager(b.t),
		TXManager:           componentsmocks.NewTXManager(b.t),
		SequenceManager:     componentsmocks.NewSequencerManager(b.t),
		StateManager:        componentsmocks.NewStateManager(b.t),
		DomainQueryContext:  componentsmocks.NewDomainQueryContext(b.t),
		DomainAPI:           componentsmocks.NewDomainSmartContract(b.t),
		Domain:              componentsmocks.NewDomain(b.t),
		DomainStateWriter:   componentsmocks.NewDomainStateWriter(b.t),
		DB:                  mp.Mock,
	}

	// link the mocks which return other mocks
	mocks.AllComponents.On("KeyManager").Return(mocks.KeyManager).Maybe()
	mocks.AllComponents.On("PublicTxManager").Return(mocks.PublicTxManager).Maybe()
	mocks.AllComponents.On("TxManager").Return(mocks.TXManager).Maybe()
	mocks.AllComponents.On("SequencerManager").Return(mocks.SequenceManager).Maybe()
	mocks.AllComponents.On("Persistence").Return(mp.P).Maybe()
	mocks.AllComponents.On("StateManager").Return(mocks.StateManager).Maybe()
	mocks.DomainQueryContext.On("Close", mock.Anything).Return().Maybe()
	mocks.StateManager.On("NewDomainQueryContext", mock.Anything, mock.Anything, mock.Anything).Return(mocks.DomainQueryContext).Maybe()
	mocks.DomainAPI.On("Domain").Return(mocks.Domain).Maybe()
	mocks.DomainAPI.On("Address").Return(*pldtypes.RandAddress()).Maybe()

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

	txn := newTransaction(
		ctx,
		b.originator,
		b.originatorNode,
		b.nodeName,
		privateTransaction,
		func() string { return b.coordinatorSigningIdentity },
		transportWriter,
		clock,
		b.queueEventForCoordinator,
		b.setDispatchedInFlight,
		coordinatorTransactionHandleEvent(b.coordinatorTransactions),
		coordinatorTransactionStateLookup(b.coordinatorTransactions),
		func(context.Context, ...string) {}, // notifyEndorserCandidates
		mocks.EngineIntegration,
		func(_ context.Context) {},
		func() int64 { return b.currentBlockHeight },
		b.blockHeightTolerance,
		mocks.SyncPoints,
		mocks.AllComponents,
		mocks.DomainAPI,
		mocks.DomainStateWriter,
		time.Duration(b.requestTimeout),
		time.Duration(b.stateTimeout),
		b.finalizingGracePeriod,
		b.baseLedgerRevertRetryThreshold,
		b.assembleErrorRetryThreshhold,
		b.signErrorRetryThreshhold,
		b.grapher,
		b.stateVisibilityTracker,
		b.dependencyTracker,
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
	txn.stateMachine.SetCurrentState(b.state)
	txn.revertReason = b.revertReason
	txn.revertCount = b.revertCount
	txn.assembleErrorCount = b.assembleErrorCount
	txn.signErrorCount = b.signErrorCount
	if b.endorseToleranceByRequirement != nil {
		txn.endorseToleranceByRequirement = b.endorseToleranceByRequirement
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
		for _, state := range privateTransaction.PostAssembly.OutputStates {
			err := b.grapher.AddMinter(ctx, []*prototk.EndorsableState{state}, txn.pt.ID)
			require.NoError(b.t, err)
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
		PostAssembly: b.BuildPostAssembly().AssembleResponse,
		RequestID:    b.txn.pendingAssembleRequest.IdempotencyKey(),
	}
}

func (b *TransactionBuilderForTesting) BuildAssembleRevertEvent() *AssembleRevertEvent {
	return &AssembleRevertEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: b.txn.pt.ID,
		},
		PostAssembly: b.BuildPostAssembly().AssembleResponse,
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

func (b *TransactionBuilderForTesting) BuildEndorseRevertEvent() *EndorseRevertEvent {
	party := b.privateTransactionBuilder.GetEndorserIdentityLocator(0)
	return &EndorseRevertEvent{
		BaseCoordinatorEvent:   BaseCoordinatorEvent{TransactionID: b.txn.pt.ID},
		Party:                  party,
		RevertReason:           "some reason for revert",
		AttestationRequestName: "endorse-0",
		RequestID:              b.txn.pendingEndorsementRequests["endorse-0"][party].IdempotencyKey(),
	}
}

func (b *TransactionBuilderForTesting) BuildEndorseRequestRejectedEvent() *EndorseRequestRejectedEvent {
	party := b.privateTransactionBuilder.GetEndorserIdentityLocator(0)
	return &EndorseRequestRejectedEvent{
		BaseCoordinatorEvent:   BaseCoordinatorEvent{TransactionID: b.txn.pt.ID},
		Party:                  party,
		AttestationRequestName: "endorse-0",
		RequestID:              b.txn.pendingEndorsementRequests["endorse-0"][party].IdempotencyKey(),
		CoordinatorBlockHeight: 100,
		EndorserBlockHeight:    200,
	}
}

func (b *TransactionBuilderForTesting) BuildEndorseErrorEvent() *EndorseErrorEvent {
	party := b.privateTransactionBuilder.GetEndorserIdentityLocator(0)
	return &EndorseErrorEvent{
		BaseCoordinatorEvent:   BaseCoordinatorEvent{TransactionID: b.txn.pt.ID},
		Party:                  party,
		AttestationRequestName: "endorse-0",
		RequestID:              b.txn.pendingEndorsementRequests["endorse-0"][party].IdempotencyKey(),
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

func (b *TransactionBuilderForTesting) BuildPreAssembly() *prototk.TransactionPreAssembly {
	return b.privateTransactionBuilder.BuildPreAssembly()
}
