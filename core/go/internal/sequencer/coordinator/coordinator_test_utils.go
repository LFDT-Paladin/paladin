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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
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

func (r *SentMessageRecorder) SendHandoverRequest(ctx context.Context, activeCoordinator string, contractAddress *pldtypes.EthAddress) error {
	r.hasSentHandoverRequest = true
	return nil
}

func (r *SentMessageRecorder) HasSentHandoverRequest() bool {
	return r.hasSentHandoverRequest
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

// noopBuildNullifiers is used in unit tests when the dispatch path is not exercised or uses mocks.
var noopBuildNullifiers = func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error) {
	return nil, nil
}

// noopNewPrivateTransactions is used in unit tests when the dispatch path is not exercised or uses mocks.
var noopNewPrivateTransactions = func(context.Context, []*components.ValidatedTransaction) error {
	return nil
}

type CoordinatorBuilderForTesting struct {
	t                                        *testing.T
	state                                    State
	originatorIdentityPool                   []string
	contractAddress                          *pldtypes.EthAddress
	currentBlockHeight                       *uint64
	activeCoordinatorBlockHeight             *uint64
	activeCoordinator                        *string
	flushPointTransactionID                  *uuid.UUID
	flushPointHash                           *pldtypes.Bytes32
	flushPointNonce                          *uint64
	flushPointSignerAddress                  *pldtypes.EthAddress
	emitFunction                             func(event common.Event)
	transactions                             []*transaction.CoordinatorTransaction
	heartbeatsUntilClosingGracePeriodExpires *int
	metrics                                  metrics.DistributedSequencerMetrics
	sequencerConfig                          *pldconf.SequencerConfig
}

type CoordinatorDependencyMocks struct {
	SentMessageRecorder *SentMessageRecorder
	Clock               *common.FakeClockForTesting
	EngineIntegration   *common.FakeEngineIntegrationForTesting
	SyncPoints          *syncpoints.MockSyncPoints
	emittedEvents       []common.Event
	DomainAPI           *componentsmocks.DomainSmartContract
	Domain              *componentsmocks.Domain
	TXManager           *componentsmocks.TXManager
	KeyManager          *componentsmocks.KeyManager
	PublicTxManager     *componentsmocks.PublicTxManager
	DomainContext       components.DomainContext
}

// copySequencerDefaultsForTest returns a deep copy of SequencerDefaults so tests that mutate
// config (e.g. GetSequencerConfig().MaxDispatchAhead = ...) do not affect later tests.
func copySequencerDefaultsForTest() *pldconf.SequencerConfig {
	def := &pldconf.SequencerDefaults
	copy := &pldconf.SequencerConfig{
		Writer: pldconf.FlushWriterConfig{},
	}
	if def.AssembleTimeout != nil {
		v := *def.AssembleTimeout
		copy.AssembleTimeout = &v
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
	if def.ClosingGracePeriod != nil {
		v := *def.ClosingGracePeriod
		copy.ClosingGracePeriod = &v
	}
	if def.DelegateTimeout != nil {
		v := *def.DelegateTimeout
		copy.DelegateTimeout = &v
	}
	if def.HeartbeatInterval != nil {
		v := *def.HeartbeatInterval
		copy.HeartbeatInterval = &v
	}
	if def.HeartbeatThreshold != nil {
		v := *def.HeartbeatThreshold
		copy.HeartbeatThreshold = &v
	}
	if def.MaxInflightTransactions != nil {
		v := *def.MaxInflightTransactions
		copy.MaxInflightTransactions = &v
	}
	if def.MaxDispatchAhead != nil {
		v := *def.MaxDispatchAhead
		copy.MaxDispatchAhead = &v
	}
	if def.TargetActiveCoordinators != nil {
		v := *def.TargetActiveCoordinators
		copy.TargetActiveCoordinators = &v
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

func (b *CoordinatorBuilderForTesting) ActiveCoordinator(activeCoordinator string) *CoordinatorBuilderForTesting {
	b.activeCoordinator = &activeCoordinator
	return b
}

func (b *CoordinatorBuilderForTesting) OriginatorIdentityPool(originatorIdentityPool ...string) *CoordinatorBuilderForTesting {
	b.originatorIdentityPool = originatorIdentityPool
	return b
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

func (b *CoordinatorBuilderForTesting) ActiveCoordinatorBlockHeight(activeCoordinatorBlockHeight uint64) *CoordinatorBuilderForTesting {
	b.activeCoordinatorBlockHeight = &activeCoordinatorBlockHeight
	return b
}

func (b *CoordinatorBuilderForTesting) Transactions(transactions ...*transaction.CoordinatorTransaction) *CoordinatorBuilderForTesting {
	b.transactions = transactions
	return b
}

func (b *CoordinatorBuilderForTesting) HeartbeatsUntilClosingGracePeriodExpires(heartbeatsUntilClosingGracePeriodExpires int) *CoordinatorBuilderForTesting {
	b.heartbeatsUntilClosingGracePeriodExpires = &heartbeatsUntilClosingGracePeriodExpires
	return b
}

func (b *CoordinatorBuilderForTesting) GetFlushPointNonce() uint64 {
	return *b.flushPointNonce
}

func (b *CoordinatorBuilderForTesting) GetFlushPointSignerAddress() *pldtypes.EthAddress {
	return b.flushPointSignerAddress
}

func (b *CoordinatorBuilderForTesting) GetFlushPointHash() pldtypes.Bytes32 {
	return *b.flushPointHash
}

func (b *CoordinatorBuilderForTesting) GetSequencerConfig() *pldconf.SequencerConfig {
	return b.sequencerConfig
}

func (b *CoordinatorBuilderForTesting) OverrideSequencerConfig(config *pldconf.SequencerConfig) *CoordinatorBuilderForTesting {
	b.sequencerConfig = config
	return b
}

func (b *CoordinatorBuilderForTesting) Build(ctx context.Context) (*coordinator, *CoordinatorDependencyMocks) {
	if b.contractAddress == nil {
		b.contractAddress = pldtypes.RandAddress()
	}
	mocks := &CoordinatorDependencyMocks{
		SentMessageRecorder: NewSentMessageRecorder(),
		Clock:               &common.FakeClockForTesting{},
		EngineIntegration:   &common.FakeEngineIntegrationForTesting{},
		SyncPoints:          &syncpoints.MockSyncPoints{},
		DomainAPI:           componentsmocks.NewDomainSmartContract(b.t),
		Domain:              componentsmocks.NewDomain(b.t),
		TXManager:           componentsmocks.NewTXManager(b.t),
		KeyManager:          componentsmocks.NewKeyManager(b.t),
		PublicTxManager:     componentsmocks.NewPublicTxManager(b.t),
		DomainContext:       componentsmocks.NewDomainContext(b.t),
	}
	mocks.DomainAPI.EXPECT().Domain().Return(mocks.Domain).Maybe()

	b.emitFunction = func(event common.Event) {
		mocks.emittedEvents = append(mocks.emittedEvents, event)
	}

	coordinator := NewCoordinator(
		ctx,
		b.contractAddress, // Contract address,
		mocks.DomainAPI,
		mocks.TXManager,
		mocks.KeyManager,
		mocks.PublicTxManager,
		mocks.SentMessageRecorder,
		mocks.Clock,
		mocks.EngineIntegration,
		mocks.SyncPoints,
		b.originatorIdentityPool,
		b.sequencerConfig,
		"node1",
		b.metrics,
		mocks.DomainContext,
		noopBuildNullifiers,
		noopNewPrivateTransactions,
		func(contractAddress *pldtypes.EthAddress, coordinatorNode string) {}, // coordinatorStarted function, not used in tests
		func(contractAddress *pldtypes.EthAddress) {},                         // coordinatorIdle function, not used in tests
	)

	for _, tx := range b.transactions {
		coordinator.transactionsByID[tx.GetID()] = tx
		coordinator.pooledTransactions = append(coordinator.pooledTransactions, tx)
	}

	coordinator.stateMachineEventLoop.StateMachine().CurrentState = b.state
	switch b.state {
	case State_Initial:
		fallthrough
	case State_Idle:
		if b.currentBlockHeight == nil {
			b.currentBlockHeight = ptrTo(uint64(0))
		}
		coordinator.currentBlockHeight = *b.currentBlockHeight
	case State_Observing:
		fallthrough
	case State_Standby:
		fallthrough
	case State_Elect:
		if b.currentBlockHeight == nil {
			b.currentBlockHeight = ptrTo(uint64(0))
		}
		if b.activeCoordinatorBlockHeight == nil {
			b.activeCoordinatorBlockHeight = ptrTo(uint64(0))
		}

		if b.activeCoordinator == nil {
			b.activeCoordinator = ptrTo("activeCoordinator")
		}

		coordinator.currentBlockHeight = *b.currentBlockHeight
		coordinator.activeCoordinatorBlockHeight = *b.activeCoordinatorBlockHeight
		coordinator.activeCoordinatorNode = *b.activeCoordinator
	case State_Prepared:
		if b.flushPointTransactionID == nil {
			b.flushPointTransactionID = ptrTo(uuid.New())
		}
		if b.flushPointHash == nil {
			b.flushPointHash = ptrTo(pldtypes.Bytes32(pldtypes.RandBytes(32)))
		}
		if b.flushPointNonce == nil {
			b.flushPointNonce = ptrTo(uint64(42))
		}
		if b.flushPointSignerAddress == nil {
			b.flushPointSignerAddress = pldtypes.RandAddress()
		}

		coordinator.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
			fmt.Sprintf("%s:%d", b.flushPointSignerAddress.String(), *b.flushPointNonce): {
				TransactionID: *b.flushPointTransactionID,
				Hash:          *b.flushPointHash,
				Nonce:         *b.flushPointNonce,
				From:          *b.flushPointSignerAddress,
			},
		}
	case State_Closing:
		if b.heartbeatsUntilClosingGracePeriodExpires == nil {
			b.heartbeatsUntilClosingGracePeriodExpires = ptrTo(5)
		}
		coordinator.heartbeatIntervalsSinceStateChange = 5 - *b.heartbeatsUntilClosingGracePeriodExpires

	}

	// Actions like action_HeartbeatReceived write to this map; ensure it is never nil
	if coordinator.activeCoordinatorsFlushPointsBySignerNonce == nil {
		coordinator.activeCoordinatorsFlushPointsBySignerNonce = make(map[string]*common.FlushPoint)
	}

	return coordinator, mocks
}
