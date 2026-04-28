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
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/prometheus/client_golang/prometheus"
)

type SentMessageRecorder struct {
	transaction.SentMessageRecorder
	hasSentHandoverRequest bool
	sentHeartbeatCount     int
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{
		SentMessageRecorder: *transaction.NewSentMessageRecorder(),
	}
}

func (r *SentMessageRecorder) Reset(ctx context.Context) {
	r.hasSentHandoverRequest = false
	r.sentHeartbeatCount = 0
	r.SentMessageRecorder.Reset(ctx)
}

func (r *SentMessageRecorder) SendHeartbeat(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress, coordinatorSnapshot *common.CoordinatorSnapshot) error {
	r.sentHeartbeatCount++
	return nil
}

func (r *SentMessageRecorder) SendDelegationRequest(ctx context.Context, coordinatorLocator string, transactions []*components.PrivateTransaction, blockHeight uint64) error {
	return nil
}

func (r *SentMessageRecorder) SentHeartbeatCount() int {
	return r.sentHeartbeatCount
}

func (r *SentMessageRecorder) HasSentHeartbeat() bool {
	return r.sentHeartbeatCount > 0
}

type CoordinatorBuilderForTesting struct {
	t                                        *testing.T
	state                                    State
	domainAPI                                *componentsmocks.DomainSmartContract
	txManager                                *componentsmocks.TXManager
	sequencerManager                         *componentsmocks.SequencerManager
	contractAddress                          *pldtypes.EthAddress
	currentBlockHeight                       *uint64
	activeCoordinator                        *string
	transactions                             []transaction.CoordinatorTransaction
	heartbeatsUntilClosingGracePeriodExpires *int
	metrics                                  metrics.DistributedSequencerMetrics
	sequencerConfig                          *pldconf.SequencerConfig
}

type CoordinatorDependencyMocks struct {
	SentMessageRecorder *SentMessageRecorder
	EngineIntegration   *common.FakeEngineIntegrationForTesting
	SyncPoints          syncpoints.SyncPoints
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

	domainAPI := componentsmocks.NewDomainSmartContract(t)
	txManager := componentsmocks.NewTXManager(t)
	sequencerManager := componentsmocks.NewSequencerManager(t)
	return &CoordinatorBuilderForTesting{
		t:                t,
		state:            state,
		domainAPI:        domainAPI,
		txManager:        txManager,
		sequencerManager: sequencerManager,
		metrics:          metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
		sequencerConfig:  copySequencerDefaultsForTest(),
	}
}

func (b *CoordinatorBuilderForTesting) ContractAddress(contractAddress *pldtypes.EthAddress) *CoordinatorBuilderForTesting {
	b.contractAddress = contractAddress
	return b
}

func (b *CoordinatorBuilderForTesting) GetContractAddress() pldtypes.EthAddress {
	return *b.contractAddress
}

func (b *CoordinatorBuilderForTesting) CurrentBlockHeight(currentBlockHeight uint64) *CoordinatorBuilderForTesting {
	b.currentBlockHeight = &currentBlockHeight
	return b
}

func (b *CoordinatorBuilderForTesting) Transactions(transactions ...transaction.CoordinatorTransaction) *CoordinatorBuilderForTesting {
	b.transactions = transactions
	return b
}

func (b *CoordinatorBuilderForTesting) GetDomainAPI() *componentsmocks.DomainSmartContract {
	return b.domainAPI
}

func (b *CoordinatorBuilderForTesting) GetTXManager() *componentsmocks.TXManager {
	return b.txManager
}

func (b *CoordinatorBuilderForTesting) GetSequencerManager() *componentsmocks.SequencerManager {
	return b.sequencerManager
}

func (b *CoordinatorBuilderForTesting) GetSequencerConfig() *pldconf.SequencerConfig {
	return b.sequencerConfig
}

func (b *CoordinatorBuilderForTesting) OverrideSequencerConfig(config *pldconf.SequencerConfig) *CoordinatorBuilderForTesting {
	b.sequencerConfig = config
	return b
}

func (b *CoordinatorBuilderForTesting) Build(ctx context.Context) (*coordinator, *CoordinatorDependencyMocks, func()) {
	// TODO: This is a bit of a hack, but all this code gets substantial rework in the restructing PRs so
	// it makes sense to clean this up as part of that merge
	hasContractConfigExpectation := false
	for _, call := range b.domainAPI.ExpectedCalls {
		if call.Method == "ContractConfig" {
			hasContractConfigExpectation = true
			break
		}
	}
	if !hasContractConfigExpectation {
		b.domainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
			CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
			SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
		})
	}

	if b.contractAddress == nil {
		b.contractAddress = pldtypes.RandAddress()
	}
	mocks := &CoordinatorDependencyMocks{
		SentMessageRecorder: NewSentMessageRecorder(),
		EngineIntegration:   &common.FakeEngineIntegrationForTesting{},
		SyncPoints:          &syncpoints.MockSyncPoints{},
	}

	b.domainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	}).Maybe()
	buildCtx, cancel := context.WithCancel(ctx)

	allComponents := componentsmocks.NewAllComponents(b.t)
	mp, err := mockpersistence.NewSQLMockProvider()
	if err != nil {
		panic(err)
	}

	transportManager := componentsmocks.NewTransportManager(b.t)
	transportManager.On("LocalNodeName").Return("node1").Maybe()
	allComponents.On("TransportManager").Return(transportManager).Maybe()
	allComponents.On("TxManager").Return(b.txManager).Maybe()
	allComponents.On("SequencerManager").Return(b.sequencerManager).Maybe()
	allComponents.On("Persistence").Return(mp.P).Maybe()

	coordinator, err := NewCoordinator(
		buildCtx,
		b.contractAddress, // Contract address,
		b.domainAPI,
		nil,
		allComponents,
		nil,
		nil,
		mocks.SentMessageRecorder,
		common.RealClock(),
		mocks.EngineIntegration,
		mocks.SyncPoints,
		b.sequencerConfig,
		"node1",
		b.metrics,
	)
	if err != nil {
		panic(err)
	}

	// Drain startup events (CoordinatorCreatedEvent) before overriding the test state.
	syncEv := statemachine.NewSyncEvent()
	coordinator.QueueEvent(buildCtx, syncEv)
	<-syncEv.Done

	for _, tx := range b.transactions {
		coordinator.transactionsByID[tx.GetID()] = tx
	}

	// Reset activeCoordinatorNode which action_SelectActiveCoordinator may have set during startup.
	// TODO AM: this can have a big rethink with the new state machine
	coordinator.activeCoordinatorNode = ""
	coordinator.stateMachineEventLoop.StateMachine().SetCurrentState(b.state)
	switch b.state {
	case State_Observing:
		fallthrough
	case State_Elect:
		if b.currentBlockHeight == nil {
			b.currentBlockHeight = ptrTo(uint64(0))
		}

		if b.activeCoordinator == nil {
			b.activeCoordinator = ptrTo("activeCoordinator")
		}

		coordinator.currentBlockHeight = *b.currentBlockHeight
		coordinator.activeCoordinatorNode = *b.activeCoordinator
	case State_Closing:
		if b.heartbeatsUntilClosingGracePeriodExpires == nil {
			b.heartbeatsUntilClosingGracePeriodExpires = ptrTo(5)
		}
		coordinator.heartbeatIntervalsSinceStateChange = 5 - *b.heartbeatsUntilClosingGracePeriodExpires

	}

	done := func() {
		cancel()
		// With maxDispatchAhead=-1 the dispatch loop enters sync.Cond.Wait on its
		// first read. The context.AfterFunc broadcast can race with the goroutine
		// entering Wait (both dispatchQueue and ctx.Done are ready in the outer
		// select; Go picks randomly). Keep broadcasting until the loop exits.
		go func() {
			for {
				select {
				case <-coordinator.dispatchLoopStopped:
					return
				default:
					coordinator.inFlightMutex.L.Lock()
					coordinator.inFlightMutex.Broadcast()
					coordinator.inFlightMutex.L.Unlock()
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
		coordinator.WaitForDone(context.Background())
	}
	return coordinator, mocks, done
}
