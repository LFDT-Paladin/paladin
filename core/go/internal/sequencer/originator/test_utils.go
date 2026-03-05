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
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	uuid "github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

type SentMessageRecorder struct {
	transaction.SentMessageRecorder
	hasSentDelegationRequest bool
	delegatedTransactions    []*components.PrivateTransaction
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{}
}

func (r *SentMessageRecorder) SendDelegationRequest(ctx context.Context, coordinator string, transactions []*components.PrivateTransaction, originatorsBlockHeight uint64) error {
	r.delegatedTransactions = transactions
	r.hasSentDelegationRequest = true
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

func (r *SentMessageRecorder) Reset(ctx context.Context) {
	r.SentMessageRecorder.Reset(ctx)
	r.hasSentDelegationRequest = false
	r.delegatedTransactions = nil
}

type OriginatorBuilderForTesting struct {
	state               State
	nodeName            *string
	committeeMembers    []string
	contractAddress     *pldtypes.EthAddress
	transactionBuilders []*transaction.TransactionBuilderForTesting
	metrics             metrics.DistributedSequencerMetrics
	sequencerConfig     *pldconf.SequencerConfig
	clock               common.Clock
	heartbeatInterval   *time.Duration
}

type OriginatorDependencyMocks struct {
	SentMessageRecorder *SentMessageRecorder
	EngineIntegration   *common.FakeEngineIntegrationForTesting
}

func NewOriginatorBuilderForTesting(state State) *OriginatorBuilderForTesting {
	defaults := pldconf.SequencerDefaults
	return &OriginatorBuilderForTesting{
		state:           state,
		metrics:         metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
		sequencerConfig: &defaults,
		clock:           common.RealClock(),
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

func (b *OriginatorBuilderForTesting) TransactionBuilders(builders ...*transaction.TransactionBuilderForTesting) *OriginatorBuilderForTesting {
	b.transactionBuilders = builders
	return b
}

func (b *OriginatorBuilderForTesting) Clock(clock common.Clock) *OriginatorBuilderForTesting {
	b.clock = clock
	return b
}

func (b *OriginatorBuilderForTesting) HeartbeatInterval(interval time.Duration) *OriginatorBuilderForTesting {
	b.heartbeatInterval = &interval
	return b
}

func (b *OriginatorBuilderForTesting) GetContractAddress() pldtypes.EthAddress {
	return *b.contractAddress
}

func (b *OriginatorBuilderForTesting) GetSequencerConfig() *pldconf.SequencerConfig {
	return b.sequencerConfig
}

func (b *OriginatorBuilderForTesting) OverrideSequencerConfig(config *pldconf.SequencerConfig) {
	b.sequencerConfig = config
}

func (b *OriginatorBuilderForTesting) Build(ctx context.Context) (*originator, *OriginatorDependencyMocks, func()) {

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
		SentMessageRecorder: NewSentMessageRecorder(),
		EngineIntegration:   &common.FakeEngineIntegrationForTesting{},
	}

	var originator *originator

	var err error
	buildCtx, cancel := context.WithCancel(ctx)
	originator, err = NewOriginator(
		buildCtx,
		*b.nodeName,
		mocks.SentMessageRecorder,
		b.clock,
		mocks.EngineIntegration,
		b.contractAddress,
		b.sequencerConfig,
		b.metrics,
	)
	if b.heartbeatInterval != nil {
		originator.heartbeatInterval = *b.heartbeatInterval
	}

	for _, txBuilder := range b.transactionBuilders {
		tx := txBuilder.QueueEventsTo(originator.queueEventInternal).Build()
		txID := tx.GetID()
		originator.transactionsByID[txID] = tx
		originator.transactionsOrdered = append(originator.transactionsOrdered, tx)
	}

	if err != nil {
		panic(err)
	}

	originator.stateMachineEventLoop.StateMachine().CurrentState = b.state
	switch b.state {
	// Any state specific setup can be done here
	}

	originator.activeCoordinatorNode = "coordinator"

	done := func() {
		cancel()
		originator.WaitForDone(context.Background())
	}
	return originator, mocks, done
}
