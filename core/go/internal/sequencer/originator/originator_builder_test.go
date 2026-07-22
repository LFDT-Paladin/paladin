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

package originator

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencercommonmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencertransportmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
)

const (
	TestDefault_HeartbeatThreshold  int = 5
	TestDefault_HeartbeatIntervalMs int = 100
)

type OriginatorBuilderForTesting struct {
	t                                  *testing.T
	state                              State
	nodeName                           *string
	committeeMembers                   []string
	contractAddress                    *pldtypes.EthAddress
	metrics                            metrics.DistributedSequencerMetrics
	sequencerConfig                    *pldconf.SequencerConfig
	blockRange                         *uint64
	currentEffectiveBlockHeight        *uint64
	endorserCandidates                 []string
	coordinatorPriorityList            []string
	currentActiveCoordinator           *string
	heartbeatIntervalsSinceLastReceive *int
	inactiveGracePeriod                *int
	failoverIndex                      *int
	transactions                       []transaction.OriginatorTransaction
	useMockTransportWriter             bool
	checkStateComplete                 bool
	checkStateErr                      error
}

type OriginatorDependencyMocks struct {
	SentMessageRecorder *testutil.SentMessageRecorder
	EngineIntegration   *sequencercommonmocks.EngineIntegration
	TransportWriter     *sequencertransportmocks.TransportWriter
}

func NewOriginatorBuilderForTesting(t *testing.T, state State) *OriginatorBuilderForTesting {
	return &OriginatorBuilderForTesting{
		t:                  t,
		state:              state,
		metrics:            metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
		sequencerConfig:    &pldconf.SequencerDefaults,
		checkStateComplete: true, // default: state is complete
	}
}

func (b *OriginatorBuilderForTesting) ContractAddress(contractAddress *pldtypes.EthAddress) *OriginatorBuilderForTesting {
	b.contractAddress = contractAddress
	return b
}

func (b *OriginatorBuilderForTesting) NodeName(nodeName string) *OriginatorBuilderForTesting {
	b.nodeName = &nodeName
	return b
}

func (b *OriginatorBuilderForTesting) CommitteeMembers(committeeMembers ...string) *OriginatorBuilderForTesting {
	b.committeeMembers = committeeMembers
	return b
}

func (b *OriginatorBuilderForTesting) GetContractAddress() pldtypes.EthAddress {
	return *b.contractAddress
}

func (b *OriginatorBuilderForTesting) GetCoordinatorHeartbeatThresholdMs() int {
	return TestDefault_HeartbeatThreshold * TestDefault_HeartbeatIntervalMs
}

func (b *OriginatorBuilderForTesting) GetSequencerConfig() *pldconf.SequencerConfig {
	return b.sequencerConfig
}

func (b *OriginatorBuilderForTesting) OverrideSequencerConfig(config *pldconf.SequencerConfig) {
	b.sequencerConfig = config
}

func (b *OriginatorBuilderForTesting) BlockRange(n uint64) *OriginatorBuilderForTesting {
	b.blockRange = &n
	return b
}

func (b *OriginatorBuilderForTesting) CurrentBlockHeight(n uint64) *OriginatorBuilderForTesting {
	b.currentEffectiveBlockHeight = &n
	return b
}

func (b *OriginatorBuilderForTesting) WithEndorserCandidates(nodes ...string) *OriginatorBuilderForTesting {
	b.endorserCandidates = append([]string(nil), nodes...)
	return b
}

func (b *OriginatorBuilderForTesting) CoordinatorPriorityList(nodes ...string) *OriginatorBuilderForTesting {
	b.coordinatorPriorityList = append([]string(nil), nodes...)
	return b
}

func (b *OriginatorBuilderForTesting) CurrentActiveCoordinator(node string) *OriginatorBuilderForTesting {
	b.currentActiveCoordinator = &node
	return b
}

func (b *OriginatorBuilderForTesting) HeartbeatIntervalsSinceLastReceive(n int) *OriginatorBuilderForTesting {
	b.heartbeatIntervalsSinceLastReceive = &n
	return b
}

func (b *OriginatorBuilderForTesting) InactiveGracePeriod(n int) *OriginatorBuilderForTesting {
	b.inactiveGracePeriod = &n
	return b
}

func (b *OriginatorBuilderForTesting) FailoverIndex(n int) *OriginatorBuilderForTesting {
	b.failoverIndex = &n
	return b
}
func (b *OriginatorBuilderForTesting) Transactions(txns ...transaction.OriginatorTransaction) *OriginatorBuilderForTesting {
	b.transactions = txns
	return b
}

func (b *OriginatorBuilderForTesting) WithCheckPendingPrivateStateData(complete bool) *OriginatorBuilderForTesting {
	b.checkStateComplete = complete
	return b
}

func (b *OriginatorBuilderForTesting) WithCheckPendingPrivateStateDataError(err error) *OriginatorBuilderForTesting {
	b.checkStateComplete = false
	b.checkStateErr = err
	return b
}

// WithMockTransportWriter switches the transport writer from the default SentMessageRecorder to a
// full testify mock. This enables per-call assertions (e.g. assert the exact coordinator node that
// a delegation request was sent to). Requires a *testing.T for mock cleanup registration.
func (b *OriginatorBuilderForTesting) WithMockTransportWriter(t *testing.T) *OriginatorBuilderForTesting {
	b.t = t
	b.useMockTransportWriter = true
	return b
}

func ptrTo[T any](v T) *T {
	return &v
}

func (b *OriginatorBuilderForTesting) Build() (*originator, *OriginatorDependencyMocks) {

	if b.nodeName == nil {
		b.nodeName = ptrTo("member1@node1")
	}

	if b.committeeMembers == nil {
		b.committeeMembers = []string{*b.nodeName}
	}

	if b.contractAddress == nil {
		b.contractAddress = pldtypes.RandAddress()
	}
	mocks := &OriginatorDependencyMocks{
		SentMessageRecorder: testutil.NewSentMessageRecorder(),
		EngineIntegration:   sequencercommonmocks.NewEngineIntegration(b.t),
	}

	if b.useMockTransportWriter {
		mocks.TransportWriter = sequencertransportmocks.NewTransportWriter(b.t)
	}

	// Default no-op verifier resolution so transactions created via the normal flow (Initial → Resolving)
	// don't fail on an unexpected mock call. Tests that assert on resolution set their own expectation.
	mocks.EngineIntegration.On("ResolveVerifiers", mock.Anything, mock.Anything).
		Return([]*prototk.ResolvedVerifier{}, nil).Maybe()

	seqConfig := b.sequencerConfig
	if seqConfig == nil {
		seqConfig = &pldconf.SequencerDefaults
	}

	var tw transport.TransportWriter = mocks.SentMessageRecorder
	if mocks.TransportWriter != nil {
		tw = mocks.TransportWriter
	}

	originator := NewOriginator(
		*b.nodeName,
		tw,
		mocks.EngineIntegration,
		b.contractAddress,
		seqConfig,
		b.metrics,
		&common.CoordinatorSelectionConfig{},
	)

	for _, tx := range b.transactions {
		txID := tx.GetID()
		originator.transactionsByID[txID] = tx
		originator.transactionsOrdered = append(originator.transactionsOrdered, tx)
	}

	originator.stateMachineEventLoop.StateMachine().SetCurrentState(b.state)
	switch b.state {
	// Any state specific setup can be done here
	}

	if b.currentActiveCoordinator != nil {
		originator.currentActiveCoordinator = *b.currentActiveCoordinator
	} else if originator.currentActiveCoordinator == "" {
		originator.currentActiveCoordinator = "coordinator"
	}
	if b.blockRange != nil {
		originator.blockRange = *b.blockRange
	}
	if b.currentEffectiveBlockHeight != nil {
		originator.effectiveBlockHeight = *b.currentEffectiveBlockHeight
	}
	if b.endorserCandidates != nil {
		originator.endorserCandidates = b.endorserCandidates
	}
	if b.coordinatorPriorityList != nil {
		originator.coordinatorPriorityList = b.coordinatorPriorityList
	}
	if b.heartbeatIntervalsSinceLastReceive != nil {
		originator.heartbeatIntervalsSinceLastReceive = *b.heartbeatIntervalsSinceLastReceive
	}
	if b.inactiveGracePeriod != nil {
		originator.inactiveGracePeriod = *b.inactiveGracePeriod
	}
	if b.failoverIndex != nil {
		originator.failoverIndex = *b.failoverIndex
	}

	return originator, mocks
}
