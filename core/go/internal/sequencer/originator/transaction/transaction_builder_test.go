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
	"math/rand/v2"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencercommonmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencertransportmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
)

type TransactionBuilderForTesting struct {
	t                         *testing.T
	privateTransactionBuilder *testutil.PrivateTransactionBuilderForTesting
	state                     State
	currentDelegate           string
	txn                       *originatorTransaction
	sentMessageRecorder       *testutil.SentMessageRecorder
	useMockTransportWriter    bool
	mockTransportWriter       *sequencertransportmocks.TransportWriter
	fakeEngineIntegration     *sequencercommonmocks.EngineIntegration
	queueEventForOriginator   func(ctx context.Context, event common.Event)

	/* Assembling State*/
	assembleRequestID uuid.UUID

	/* Post Assembling States (e.g. endorsing, reverted, parked)*/
	latestFulfilledAssembleRequestID uuid.UUID

	latestSubmissionHash *pldtypes.Bytes32
	signerAddress        *pldtypes.EthAddress
	nonce                *uint64

	metrics            metrics.DistributedSequencerMetrics
	currentBlockHeight int64

	checkStateComplete bool
	checkStateErr      error
}

// Function NewTransactionBuilderForTesting creates a TransactionBuilderForTesting with random values for all fields.
// Use the builder methods to set specific values for fields before calling Build to create a new Transaction
func NewTransactionBuilderForTesting(t *testing.T, state State) *TransactionBuilderForTesting {
	builder := &TransactionBuilderForTesting{
		t:                         t,
		state:                     state,
		currentDelegate:           uuid.New().String(),
		privateTransactionBuilder: testutil.NewPrivateTransactionBuilderForTesting(),
		fakeEngineIntegration:     sequencercommonmocks.NewEngineIntegration(t),
		sentMessageRecorder:       testutil.NewSentMessageRecorder(),
		metrics:                   metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
		checkStateComplete:        true, // default: state is complete
	}

	switch state {
	case State_Delegated:

	}
	return builder
}

func (b *TransactionBuilderForTesting) GetCoordinator() string {
	return b.currentDelegate
}

func (b *TransactionBuilderForTesting) GetLatestFulfilledAssembleRequestID() uuid.UUID {
	return b.latestFulfilledAssembleRequestID
}

func (b *TransactionBuilderForTesting) GetSignerAddress() pldtypes.EthAddress {
	if b.signerAddress == nil {
		b.signerAddress = pldtypes.RandAddress()
	}
	return *b.signerAddress
}

func (b *TransactionBuilderForTesting) GetNonce() uint64 {
	if b.nonce == nil {
		b.nonce = ptrTo(rand.Uint64())
	}
	return *b.nonce
}

func (b *TransactionBuilderForTesting) GetLatestSubmissionHash() pldtypes.Bytes32 {
	if b.latestSubmissionHash == nil {
		b.latestSubmissionHash = ptrTo(pldtypes.RandBytes32())
	}
	return *b.latestSubmissionHash
}

func (b *TransactionBuilderForTesting) QueueEventsTo(emit func(ctx context.Context, event common.Event)) *TransactionBuilderForTesting {
	b.queueEventForOriginator = emit
	return b
}

func (b *TransactionBuilderForTesting) GetBuiltTransaction() *originatorTransaction {
	return b.txn
}

func (b *TransactionBuilderForTesting) WithMockTransportWriter() *TransactionBuilderForTesting {
	b.useMockTransportWriter = true
	return b
}

func (b *TransactionBuilderForTesting) CurrentBlockHeight(blockHeight int64) *TransactionBuilderForTesting {
	b.currentBlockHeight = blockHeight
	return b
}

func (b *TransactionBuilderForTesting) WithCheckPendingPrivateStateData(complete bool) *TransactionBuilderForTesting {
	b.checkStateComplete = complete
	return b
}

func (b *TransactionBuilderForTesting) WithCheckPendingPrivateStateDataError(err error) *TransactionBuilderForTesting {
	b.checkStateComplete = false
	b.checkStateErr = err
	return b
}

type TransactionDependencyFakes struct {
	SentMessageRecorder *testutil.SentMessageRecorder
	TransportWriter     *sequencertransportmocks.TransportWriter
	EngineIntegration   *sequencercommonmocks.EngineIntegration
	transactionBuilder  *TransactionBuilderForTesting
	Events              chan common.Event
}

func (b *TransactionBuilderForTesting) BuildWithMocks() (*originatorTransaction, *TransactionDependencyFakes) {
	b.fakeEngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, mock.Anything).
		Return(b.checkStateComplete, b.checkStateErr).Maybe()

	mocks := &TransactionDependencyFakes{
		SentMessageRecorder: b.sentMessageRecorder,
		EngineIntegration:   b.fakeEngineIntegration,
		transactionBuilder:  b,
		Events:              make(chan common.Event, 16),
	}
	b.queueEventForOriginator = func(ctx context.Context, event common.Event) {
		mocks.Events <- event
	}
	if b.useMockTransportWriter {
		b.mockTransportWriter = sequencertransportmocks.NewTransportWriter(b.t)
		mocks.TransportWriter = b.mockTransportWriter
	}
	return b.Build(), mocks
}

func (b *TransactionBuilderForTesting) Build() *originatorTransaction {

	privateTransaction := b.privateTransactionBuilder.Build()
	if b.queueEventForOriginator == nil {
		b.queueEventForOriginator = func(ctx context.Context, event common.Event) {}
	}

	var transportWriter transport.TransportWriter = b.sentMessageRecorder
	if b.mockTransportWriter != nil {
		transportWriter = b.mockTransportWriter
	}

	txn := newTransaction(privateTransaction,
		b.fakeEngineIntegration,
		transportWriter,
		b.queueEventForOriginator,
		b.metrics,
		func(_ context.Context) {},
		func() int64 { return b.currentBlockHeight })

	txn.stateMachine.SetCurrentState(b.state)

	// Update the private transaction struct to the accumulation that resulted from what ever events that we expect to have happened leading up to the current state
	// We don't attempt to emulate any other history of those past events but rather assert that the state machine's behavior is determined purely by its current finite state
	// and the contents of the PrivateTransaction struct

	switch b.state {
	case State_Delegated:
		txn.currentDelegate = b.currentDelegate
	case State_Assembling:
		txn.currentDelegate = b.currentDelegate
		b.assembleRequestID = uuid.New()
		txn.latestAssembleRequest = &assembleRequestFromCoordinator{
			requestID: b.assembleRequestID,
		}
	case State_Endorsement_Gathering:
		txn.currentDelegate = b.currentDelegate
		b.latestFulfilledAssembleRequestID = uuid.New()
		txn.latestFulfilledAssembleRequestID = b.latestFulfilledAssembleRequestID
	case State_Reverted:
		txn.currentDelegate = b.currentDelegate
		b.latestFulfilledAssembleRequestID = uuid.New()
		txn.latestFulfilledAssembleRequestID = b.latestFulfilledAssembleRequestID

		txn.pt.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   ptrTo("test revert reason"),
		}
	case State_Parked:
		txn.currentDelegate = b.currentDelegate
		b.latestFulfilledAssembleRequestID = uuid.New()
		txn.latestFulfilledAssembleRequestID = b.latestFulfilledAssembleRequestID

		txn.pt.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_PARK,
		}
	case State_Prepared:
		txn.currentDelegate = b.currentDelegate

	case State_Submitted:
		txn.latestSubmissionHash = ptrTo(b.GetLatestSubmissionHash())
		fallthrough
	case State_Sequenced:
		txn.nonce = ptrTo(b.GetNonce())
		fallthrough
	case State_Dispatched:
		txn.currentDelegate = b.currentDelegate
		txn.signerAddress = ptrTo(b.GetSignerAddress())

	}

	b.txn = txn

	b.txn.stateMachine.SetCurrentState(b.state)
	return b.txn

}

func (m *TransactionDependencyFakes) MockForAssembleAndSignRequestOK() *mock.Call {

	return m.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, //ctx context.Contex
		m.transactionBuilder.txn.pt.ID,
		mock.Anything, //preAssembly *components.TransactionPreAssembly
		mock.Anything, //stateLocksJSON []byte
		mock.Anything, //blockHeight int64
	).Return(&components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}, nil)
}

func (m *TransactionDependencyFakes) MockForAssembleAndSignRequestRevert() *mock.Call {

	return m.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, //ctx context.Contex
		m.transactionBuilder.txn.pt.ID,
		mock.Anything, //preAssembly *components.TransactionPreAssembly
		mock.Anything, //stateLocksJSON []byte
		mock.Anything, //blockHeight int64
	).Return(&components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   ptrTo("test revert reason"),
	}, nil)
}

func (m *TransactionDependencyFakes) MockForAssembleAndSignRequestPark() *mock.Call {

	return m.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, //ctx context.Contex
		m.transactionBuilder.txn.pt.ID,
		mock.Anything, //preAssembly *components.TransactionPreAssembly
		mock.Anything, //stateLocksJSON []byte
		mock.Anything, //blockHeight int64
	).Return(&components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_PARK,
		RevertReason:   ptrTo("test revert reason"),
	}, nil)
}
