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

package coordinator

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencercommonmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencertransportmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/syncpointsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
)

type CoordinatorBuilderForTesting struct {
	t                                        *testing.T
	state                                    State
	contractAddress                          *pldtypes.EthAddress
	coordinatorSelectionMode                 *prototk.ContractConfig_CoordinatorSelection
	currentEffectiveBlockHeight              *uint64
	transactions                             []transaction.CoordinatorTransaction
	pooledTransactions                       []transaction.CoordinatorTransaction
	assemblingTxID                           *uuid.UUID
	heartbeatsUntilClosingGracePeriodExpires *int
	metrics                                  metrics.DistributedSequencerMetrics
	sequencerConfig                          *pldconf.SequencerConfig
	endorserCandidates                       *[]string
	originatorActivity                       map[string]int
	currentActiveCoordinator                 *string
	localNodeName                            string
	heartbeatIntervalsSinceLastReceive       *int
	inactiveGracePeriod                      *int
	heartbeatIntervalsSinceStateChange       *int
	coordinatorPriorityList                  []string
	useMockTransportWriter                   bool
	useMockClock                             bool
	grapher                                  grapher.Grapher
	signingIdentityUsed                      *bool
	keyManagerResolveErr                     error
}

type CoordinatorDependencyMocks struct {
	SentMessageRecorder *testutil.SentMessageRecorder
	EngineIntegration   *sequencercommonmocks.EngineIntegration
	SyncPoints          *syncpointsmocks.SyncPoints
	TransportWriter     *sequencertransportmocks.TransportWriter
	Clock               *sequencercommonmocks.Clock
	AllComponents       *componentsmocks.AllComponents
	Domain              *componentsmocks.Domain
	DomainAPI           *componentsmocks.DomainSmartContract
	DomainStateWriter   *componentsmocks.DomainStateWriter
	StateManager        *componentsmocks.StateManager
	DomainQueryContext  *componentsmocks.DomainQueryContext
	TXManager           *componentsmocks.TXManager
	SequencerManager    *componentsmocks.SequencerManager
	Persistence         *mockpersistence.SQLMockProvider
}

// copySequencerDefaultsForTest returns a deep copy of SequencerDefaults so tests that mutate
// config (e.g. GetSequencerConfig().MaxDispatchAhead = ...) do not affect later tests.
func copySequencerDefaultsForTest() *pldconf.SequencerConfig {
	def := &pldconf.SequencerDefaults
	copy := &pldconf.SequencerConfig{
		Writer: pldconf.FlushWriterConfig{},
	}
	if def.StateTimeout != nil {
		v := *def.StateTimeout
		copy.StateTimeout = &v
	}
	if def.RequestTimeout != nil {
		v := *def.RequestTimeout
		copy.RequestTimeout = &v
	}
	if def.BlockHeightTolerance != nil {
		v := *def.BlockHeightTolerance
		copy.BlockHeightTolerance = &v
	}
	if def.BlockRange != nil {
		v := *def.BlockRange
		copy.BlockRange = &v
	}
	if def.CoordinatorEventQueueSize != nil {
		v := *def.CoordinatorEventQueueSize
		copy.CoordinatorEventQueueSize = &v
	}
	if def.CoordinatorPriorityEventQueueSize != nil {
		v := *def.CoordinatorPriorityEventQueueSize
		copy.CoordinatorPriorityEventQueueSize = &v
	}
	if def.OriginatorEventQueueSize != nil {
		v := *def.OriginatorEventQueueSize
		copy.OriginatorEventQueueSize = &v
	}
	if def.OriginatorPriorityEventQueueSize != nil {
		v := *def.OriginatorPriorityEventQueueSize
		copy.OriginatorPriorityEventQueueSize = &v
	}
	if def.ClosingGracePeriod != nil {
		v := *def.ClosingGracePeriod
		copy.ClosingGracePeriod = &v
	}
	if def.HeartbeatInterval != nil {
		v := *def.HeartbeatInterval
		copy.HeartbeatInterval = &v
	}
	if def.MaxInflightTransactions != nil {
		v := *def.MaxInflightTransactions
		copy.MaxInflightTransactions = &v
	}
	if def.MaxDispatchAhead != nil {
		v := *def.MaxDispatchAhead
		copy.MaxDispatchAhead = &v
	}
	if def.TargetActiveSequencers != nil {
		v := *def.TargetActiveSequencers
		copy.TargetActiveSequencers = &v
	}
	if def.TransactionResumePollInterval != nil {
		v := *def.TransactionResumePollInterval
		copy.TransactionResumePollInterval = &v
	}
	if def.Writer.WorkerCount != nil {
		v := *def.Writer.WorkerCount
		copy.Writer.WorkerCount = &v
	}
	if def.Writer.BatchTimeout != nil {
		v := *def.Writer.BatchTimeout
		copy.Writer.BatchTimeout = &v
	}
	if def.Writer.BatchMaxSize != nil {
		v := *def.Writer.BatchMaxSize
		copy.Writer.BatchMaxSize = &v
	}
	return copy
}

func NewCoordinatorBuilderForTesting(t *testing.T, state State) *CoordinatorBuilderForTesting {
	return &CoordinatorBuilderForTesting{
		t:               t,
		state:           state,
		metrics:         metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
		sequencerConfig: copySequencerDefaultsForTest(),
	}
}

func (b *CoordinatorBuilderForTesting) ContractAddress(contractAddress *pldtypes.EthAddress) *CoordinatorBuilderForTesting {
	b.contractAddress = contractAddress
	return b
}

func (b *CoordinatorBuilderForTesting) GetContractAddress() pldtypes.EthAddress {
	return *b.contractAddress
}

func (b *CoordinatorBuilderForTesting) CurrentBlockHeight(h uint64) *CoordinatorBuilderForTesting {
	b.currentEffectiveBlockHeight = &h
	return b
}

func (b *CoordinatorBuilderForTesting) Transactions(transactions ...transaction.CoordinatorTransaction) *CoordinatorBuilderForTesting {
	b.transactions = transactions
	return b
}

func (b *CoordinatorBuilderForTesting) PooledTransactions(transactions ...transaction.CoordinatorTransaction) *CoordinatorBuilderForTesting {
	b.pooledTransactions = transactions
	return b
}

// AssemblingTransaction marks the single assembly slot as occupied by the given transaction ID,
// mirroring the state the coordinator holds while a transaction is being assembled.
func (b *CoordinatorBuilderForTesting) AssemblingTransaction(txID uuid.UUID) *CoordinatorBuilderForTesting {
	b.assemblingTxID = &txID
	return b
}

func (b *CoordinatorBuilderForTesting) GetSequencerConfig() *pldconf.SequencerConfig {
	return b.sequencerConfig
}

func (b *CoordinatorBuilderForTesting) OverrideSequencerConfig(config *pldconf.SequencerConfig) *CoordinatorBuilderForTesting {
	b.sequencerConfig = config
	return b
}

// EndorserCandidates sets the endorser candidate pool for ENDORSER-mode coordinator tests.
func (b *CoordinatorBuilderForTesting) EndorserCandidates(nodes ...string) *CoordinatorBuilderForTesting {
	b.endorserCandidates = &nodes
	return b
}

// OriginatorActivity seeds the originator activity map for STATIC/SENDER-mode coordinator tests.
// The map keys are originator node names and the values are heartbeat intervals since last activity.
func (b *CoordinatorBuilderForTesting) OriginatorActivity(activity map[string]int) *CoordinatorBuilderForTesting {
	b.originatorActivity = activity
	return b
}

func (b *CoordinatorBuilderForTesting) CoordinatorSelectionMode(mode prototk.ContractConfig_CoordinatorSelection) *CoordinatorBuilderForTesting {
	b.coordinatorSelectionMode = &mode
	return b
}

func (b *CoordinatorBuilderForTesting) NodeName(name string) *CoordinatorBuilderForTesting {
	b.localNodeName = name
	return b
}

func (b *CoordinatorBuilderForTesting) CurrentActiveCoordinator(node string) *CoordinatorBuilderForTesting {
	b.currentActiveCoordinator = &node
	return b
}

func (b *CoordinatorBuilderForTesting) CoordinatorSelectionBlockRange(n uint64) *CoordinatorBuilderForTesting {
	b.sequencerConfig.BlockRange = confutil.P(n)
	return b
}

func (b *CoordinatorBuilderForTesting) CoordinatorPriorityList(nodes ...string) *CoordinatorBuilderForTesting {
	b.coordinatorPriorityList = nodes
	return b
}

func (b *CoordinatorBuilderForTesting) HeartbeatIntervalsSinceLastReceive(n int) *CoordinatorBuilderForTesting {
	b.heartbeatIntervalsSinceLastReceive = &n
	return b
}

func (b *CoordinatorBuilderForTesting) InactiveGracePeriod(n int) *CoordinatorBuilderForTesting {
	b.inactiveGracePeriod = &n
	return b
}

func (b *CoordinatorBuilderForTesting) HeartbeatIntervalsSinceStateChange(n int) *CoordinatorBuilderForTesting {
	b.heartbeatIntervalsSinceStateChange = &n
	return b
}

// WithKeyManagerError configures the AllComponents KeyManager mock so that
// ResolveKeyNewDatabaseTX returns the given error.  Use this when testing paths that
// should abandon work when key resolution fails (e.g. an already-expired deadline).
func (b *CoordinatorBuilderForTesting) WithKeyManagerError(err error) *CoordinatorBuilderForTesting {
	b.keyManagerResolveErr = err
	return b
}

func (b *CoordinatorBuilderForTesting) WithMockTransportWriter() *CoordinatorBuilderForTesting {
	b.useMockTransportWriter = true
	return b
}

func (b *CoordinatorBuilderForTesting) AssembleErrorRetryThreshold(n int) *CoordinatorBuilderForTesting {
	b.sequencerConfig.AssembleErrorRetryThreshold = confutil.P(n)
	return b
}

func (b *CoordinatorBuilderForTesting) BlockHeightTolerance(n uint64) *CoordinatorBuilderForTesting {
	b.sequencerConfig.BlockHeightTolerance = confutil.P(n)
	return b
}

func (b *CoordinatorBuilderForTesting) ClosingGracePeriod(n int) *CoordinatorBuilderForTesting {
	b.sequencerConfig.ClosingGracePeriod = confutil.P(n)
	return b
}

func (b *CoordinatorBuilderForTesting) Grapher(grapher grapher.Grapher) *CoordinatorBuilderForTesting {
	b.grapher = grapher
	return b
}

func (b *CoordinatorBuilderForTesting) SigningIdentityUsed(used bool) *CoordinatorBuilderForTesting {
	b.signingIdentityUsed = &used
	return b
}

func (b *CoordinatorBuilderForTesting) UseMockClock() *CoordinatorBuilderForTesting {
	b.useMockClock = true
	return b
}

func (b *CoordinatorBuilderForTesting) Build() (*coordinator, *CoordinatorDependencyMocks) {
	if b.contractAddress == nil {
		b.contractAddress = pldtypes.RandAddress()
	}
	mocks := &CoordinatorDependencyMocks{
		SentMessageRecorder: testutil.NewSentMessageRecorder(),
		EngineIntegration:   sequencercommonmocks.NewEngineIntegration(b.t),
		SyncPoints:          syncpointsmocks.NewSyncPoints(b.t),
		Clock:               sequencercommonmocks.NewClock(b.t),
		AllComponents:       componentsmocks.NewAllComponents(b.t),
		Domain:              componentsmocks.NewDomain(b.t),
		DomainAPI:           componentsmocks.NewDomainSmartContract(b.t),
		DomainStateWriter:   componentsmocks.NewDomainStateWriter(b.t),
		StateManager:        componentsmocks.NewStateManager(b.t),
		DomainQueryContext:  componentsmocks.NewDomainQueryContext(b.t),
		TXManager:           componentsmocks.NewTXManager(b.t),
		SequencerManager:    componentsmocks.NewSequencerManager(b.t),
	}

	mocks.DomainAPI.On("Domain").Return(mocks.Domain).Maybe()
	mocks.DomainAPI.On("Address").Return(*b.contractAddress).Maybe()
	mocks.DomainQueryContext.On("Close", mock.Anything).Return().Maybe()
	mocks.StateManager.On("NewDomainQueryContext", mock.Anything, mock.Anything, mock.Anything).Return(mocks.DomainQueryContext).Maybe()

	if b.useMockTransportWriter {
		mockTransportWriter := sequencertransportmocks.NewTransportWriter(b.t)
		mockTransportWriter.On("StartLoopbackWriter").Return().Maybe()
		mockTransportWriter.On("StopLoopbackWriter").Return().Maybe()
		mockTransportWriter.On("WaitForDone", mock.Anything).Return().Maybe()
		mocks.TransportWriter = mockTransportWriter
	}

	mp, err := mockpersistence.NewSQLMockProvider()
	if err != nil {
		panic(err)
	}
	mocks.Persistence = mp

	localNode := "node1"
	if b.localNodeName != "" {
		localNode = b.localNodeName
	}

	transportManager := componentsmocks.NewTransportManager(b.t)
	transportManager.On("LocalNodeName").Return(localNode).Maybe()
	mocks.AllComponents.On("SequencerManager").Return(mocks.SequencerManager).Maybe()
	mocks.AllComponents.On("StateManager").Return(mocks.StateManager).Maybe()
	mocks.AllComponents.On("Persistence").Return(mp.P).Maybe()

	if b.keyManagerResolveErr != nil {
		mockKeyManager := componentsmocks.NewKeyManager(b.t)
		mockKeyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, b.keyManagerResolveErr).Maybe()
		mocks.AllComponents.On("KeyManager").Return(mockKeyManager).Maybe()
	}

	var transportWriter transport.TransportWriter = mocks.SentMessageRecorder
	if mocks.TransportWriter != nil {
		transportWriter = mocks.TransportWriter
	}

	var clock common.Clock
	if b.useMockClock {
		clock = mocks.Clock
	} else {
		clock = common.RealClock()
	}

	coordinator := NewCoordinator(
		b.contractAddress,
		mocks.DomainAPI,
		mocks.DomainStateWriter,
		mocks.AllComponents,
		nil,
		nil,
		transportWriter,
		clock,
		mocks.EngineIntegration,
		mocks.SyncPoints,
		b.sequencerConfig,
		localNode,
		b.metrics,
		func(_ context.Context, _ common.Event) {}, // no-op notifyOriginator; tests that need it wire c.notifyOriginator directly
		&common.CoordinatorSelectionConfig{},
	)

	if b.coordinatorSelectionMode != nil {
		coordinator.coordinatorSelection = *b.coordinatorSelectionMode
	}

	// Loops are not started yet — set state and seed transactions directly.
	for _, tx := range b.transactions {
		coordinator.transactionsByID[tx.GetID()] = tx
	}
	if len(b.pooledTransactions) > 0 {
		for _, tx := range b.pooledTransactions {
			coordinator.transactionsByID[tx.GetID()] = tx
			coordinator.pooledTransactions = append(coordinator.pooledTransactions, tx)
		}
	}
	if b.assemblingTxID != nil {
		coordinator.assemblyInFlight = true
		coordinator.assemblingTxID = *b.assemblingTxID
	}
	coordinator.stateMachineEventLoop.StateMachine().SetCurrentState(b.state)

	if b.currentEffectiveBlockHeight != nil {
		coordinator.effectiveBlockHeight = *b.currentEffectiveBlockHeight
		coordinator.currentBlockHeight = int64(*b.currentEffectiveBlockHeight)
	}
	if b.endorserCandidates != nil {
		coordinator.endorserCandidates = *b.endorserCandidates
	}
	if b.originatorActivity != nil {
		coordinator.originatorActivity = b.originatorActivity
	}
	if b.currentActiveCoordinator != nil {
		coordinator.currentActiveCoordinator = *b.currentActiveCoordinator
	}
	if b.heartbeatIntervalsSinceLastReceive != nil {
		coordinator.heartbeatIntervalsSinceLastReceive = *b.heartbeatIntervalsSinceLastReceive
	}
	if b.inactiveGracePeriod != nil {
		coordinator.inactiveGracePeriod = *b.inactiveGracePeriod
	}
	if b.heartbeatIntervalsSinceStateChange != nil {
		coordinator.heartbeatIntervalsSinceStateChange = *b.heartbeatIntervalsSinceStateChange
	}
	if b.grapher != nil {
		coordinator.grapher = b.grapher
	}
	if b.signingIdentityUsed != nil {
		coordinator.signingIdentity.used = *b.signingIdentityUsed
	}
	if b.coordinatorPriorityList != nil {
		coordinator.coordinatorPriorityList = b.coordinatorPriorityList
	}
	return coordinator, mocks
}
