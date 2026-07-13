/*
 * Copyright © 2026 Kaleido, Inc.
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

package sequencer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	coordinatorTx "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	"github.com/LFDT-Paladin/paladin/core/mocks/blockindexermocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/coordinatormocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/metricsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatormocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencermetricsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencertransportmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/syncpointsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/blockindexer"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Test utilities and mocks for sequencer lifecycle testing
type sequencerLifecycleTestMocks struct {
	components       *componentsmocks.AllComponents
	domainManager    *componentsmocks.DomainManager
	stateManager     *componentsmocks.StateManager
	transportManager *componentsmocks.TransportManager
	persistence      *persistencemocks.Persistence
	txManager        *componentsmocks.TXManager
	publicTxManager  *componentsmocks.PublicTxManager
	keyManager       *componentsmocks.KeyManager
	domainAPI        *componentsmocks.DomainSmartContract
	transportWriter  *sequencertransportmocks.TransportWriter
	originator       *originatormocks.Originator
	coordinator      *coordinatormocks.Coordinator
	syncPoints       *syncpointsmocks.SyncPoints
	metrics          *sequencermetricsmocks.DistributedSequencerMetrics
}

func newSequencerLifecycleTestMocks(t *testing.T) *sequencerLifecycleTestMocks {
	return newSequencerLifecycleTestMocksWithPersistence(t, nil)
}

// newSequencerLifecycleTestMocksWithPersistence creates mocks with a custom persistence
// provider - use this when the test needs SQL-mock-backed persistence (e.g. mockpersistence.NewSQLMockProvider()).
func newSequencerLifecycleTestMocksWithPersistence(t *testing.T, sqlPersistence persistence.Persistence) *sequencerLifecycleTestMocks {
	m := &sequencerLifecycleTestMocks{
		components:       componentsmocks.NewAllComponents(t),
		domainManager:    componentsmocks.NewDomainManager(t),
		stateManager:     componentsmocks.NewStateManager(t),
		transportManager: componentsmocks.NewTransportManager(t),
		persistence:      persistencemocks.NewPersistence(t),
		txManager:        componentsmocks.NewTXManager(t),
		publicTxManager:  componentsmocks.NewPublicTxManager(t),
		keyManager:       componentsmocks.NewKeyManager(t),
		domainAPI:        componentsmocks.NewDomainSmartContract(t),
		transportWriter:  sequencertransportmocks.NewTransportWriter(t),
		originator:       originatormocks.NewOriginator(t),
		coordinator:      coordinatormocks.NewCoordinator(t),
		syncPoints:       syncpointsmocks.NewSyncPoints(t),
		metrics:          sequencermetricsmocks.NewDistributedSequencerMetrics(t),
	}

	// Component accessors are plumbing - register once so tests only need to
	// assert on what the retrieved components actually do.
	m.components.EXPECT().DomainManager().Return(m.domainManager).Maybe()
	m.components.EXPECT().StateManager().Return(m.stateManager).Maybe()
	m.components.EXPECT().TransportManager().Return(m.transportManager).Maybe()
	m.components.EXPECT().TxManager().Return(m.txManager).Maybe()
	m.components.EXPECT().PublicTxManager().Return(m.publicTxManager).Maybe()
	m.components.EXPECT().KeyManager().Return(m.keyManager).Maybe()
	if sqlPersistence != nil {
		m.components.EXPECT().Persistence().Return(sqlPersistence).Maybe()
	} else {
		m.components.EXPECT().Persistence().Return(m.persistence).Maybe()
		m.persistence.EXPECT().NOTX().Return(nil).Maybe()
	}

	return m
}

func (m *sequencerLifecycleTestMocks) setupDefaultExpectations(ctx context.Context, contractAddr *pldtypes.EthAddress) {
	m.transportManager.EXPECT().LocalNodeName().Return("test-node").Maybe()
	m.metrics.EXPECT().SetActiveCoordinators(mock.Anything).Maybe()
}

type mockContractConfig struct{}

func (m *mockContractConfig) GetCoordinatorSelection() interface{} {
	return nil
}

func (m *mockContractConfig) GetSubmitterSelection() interface{} {
	return nil
}

func newSequencerManagerForTesting(t *testing.T, mocks *sequencerLifecycleTestMocks) *sequencerManager {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{}

	sm := &sequencerManager{
		ctx:                         ctx,
		config:                      config,
		components:                  mocks.components,
		nodeName:                    "test-node",
		sequencersLock:              sync.RWMutex{},
		sequencers:                  make(map[string]*sequencer),
		metrics:                     mocks.metrics,
		syncPoints:                  mocks.syncPoints,
		heartbeatInterval:           10 * time.Second,
		targetActiveSequencersLimit: 2,
	}

	return sm
}

func newSequencerForTesting(contractAddr *pldtypes.EthAddress, mocks *sequencerLifecycleTestMocks) *sequencer {
	return &sequencer{
		contractAddress: contractAddr.String(),
		originator:      mocks.originator,
		coordinator:     mocks.coordinator,
		transportWriter: mocks.transportWriter,
		cancelCtx:       func() {},
		lastTXTime:      time.Now(),
	}
}

// Test sequencer interface methods
func TestSequencer_GetCoordinator(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	seq := newSequencerForTesting(contractAddr, mocks)

	coordinator := seq.GetCoordinator()
	assert.Equal(t, mocks.coordinator, coordinator)
}

func TestSequencer_GetOriginator(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	seq := newSequencerForTesting(contractAddr, mocks)

	originator := seq.GetOriginator()
	assert.Equal(t, mocks.originator, originator)
}

func TestSequencer_GetTransportWriter(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	seq := newSequencerForTesting(contractAddr, mocks)

	transportWriter := seq.GetTransportWriter()
	assert.Equal(t, mocks.transportWriter, transportWriter)
}

func TestSequencerManager_HandleTxResume_DeployShapedConstructorStringBlockedByDeps(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	txID := uuid.New()
	dbTX := persistencemocks.NewDBTX(t)
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID: &txID,
				TransactionBase: pldapi.TransactionBase{
					Function: "constructor()",
					To:       nil,
				},
			},
		},
	}

	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, dbTX)
		},
	).Once()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, dbTX, txi).Return(true, nil).Once()

	err := sm.HandleTxResume(ctx, txi)
	require.NoError(t, err)
}

func TestSequencerManager_LoadSequencer_NewSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for new sequencer creation
	mocks.setupDefaultExpectations(ctx, contractAddr)
	mockDomainSmartContract := componentsmocks.NewDomainSmartContract(t)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.EXPECT().GetBlockHeight().Return(int64(0))
	mockDomain.EXPECT().Name().Return("testDomain").Maybe()
	mockDomainSmartContract.EXPECT().Domain().Return(mockDomain)
	mockDomainSmartContract.EXPECT().ContractConfig().Return(&prototk.ContractConfig{StaticCoordinator: proto.String("test-identity@test-coordinator")}).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mockDomainSmartContract, nil)
	mocks.stateManager.EXPECT().NewDomainStateWriter(mock.Anything, mockDomain, *contractAddr).Return(componentsmocks.NewDomainStateWriter(t)).Once()

	// Setup transport writer creation
	mocks.transportWriter.EXPECT().SendDispatched(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Setup originator creation expectations

	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()

	// Create a mock private transaction
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "verifier1@node1"},
			},
		},
	}

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, mockDomainSmartContract, tx)
	require.NoError(t, err)
	// Verify results

	assert.NotNil(t, result)
	assert.NotNil(t, result.GetCoordinator())
	assert.NotNil(t, result.GetOriginator())
	assert.NotNil(t, result.GetTransportWriter())

	// Verify sequencer was stored
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Contains(t, sm.sequencers, contractAddr.String())
	mocks.metrics.AssertExpectations(t)

	sm.sequencers[contractAddr.String()].cancelCtx()
	result.GetCoordinator().WaitForDone(ctx)
	result.GetOriginator().WaitForDone(ctx)
}

func TestSequencerManager_LoadSequencer_ExistingSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create and store an existing sequencer
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = existingSeq

	// Setup expectations for existing sequencer
	mocks.setupDefaultExpectations(ctx, contractAddr)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	// Create a mock private transaction
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "verifier1@node1"},
			},
		},
	}

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, tx)
	require.NoError(t, err)
	// Verify results

	assert.NotNil(t, result)
	assert.Equal(t, existingSeq, result)

	// Verify lastTXTime was updated
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	seq := sm.sequencers[contractAddr.String()]
	assert.True(t, seq.lastTXTime.After(time.Now().Add(-time.Second)))
}

func TestSequencerManager_LoadSequencer_ExistingSequencer_NoCoordinator_Success(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create and store an existing sequencer
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = existingSeq

	// Setup expectations for existing sequencer
	mocks.setupDefaultExpectations(ctx, contractAddr)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	// Create a mock private transaction with required verifiers
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "verifier1@node1"},
			},
		},
	}

	// this should not error for existing sequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, tx)
	require.NoError(t, err)
	// Verify results

	assert.NotNil(t, result)
	assert.Equal(t, existingSeq, result)

	// Verify lastTXTime was updated
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	seq := sm.sequencers[contractAddr.String()]
	assert.True(t, seq.lastTXTime.After(time.Now().Add(-time.Second)))
}

func TestSequencerManager_LoadSequencer_NoDomainAPI(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for domain manager returning nil
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("contract not found")).Once()

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)
	require.NoError(t, err)
	// Verify results

	assert.Nil(t, result)
}

func TestSequencerManager_LoadSequencer_DomainManagerError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for domain manager error
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("database error")).Once()

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)
	require.NoError(t, err)
	// Verify results
	// This should not error, just return nil
	assert.Nil(t, result)
}

func TestSequencerManager_LoadSequencer_NoDomainProvided(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations
	mocks.setupDefaultExpectations(ctx, contractAddr)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil)
	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)

	// Verify results
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "No domain provided to create sequencer")
	mocks.metrics.AssertExpectations(t)
}

func TestSequencerManager_GetSequencer_NotLoaded(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := sm.GetSequencer(ctx, *contractAddr)
	assert.Nil(t, seq)
}

func TestSequencerManager_GetSequencer_Loaded(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	existing := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existing
	sm.sequencersLock.Unlock()

	seq := sm.GetSequencer(ctx, *contractAddr)
	assert.Equal(t, existing, seq)
}

func TestSequencerManager_handleTransactionConfirmedSuccess_NotLoaded(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()

	txID := uuid.New()
	nonce := pldtypes.HexUint64(7)
	completion := &components.TxCompletion{
		ReceiptInput: components.ReceiptInput{
			TransactionID: txID,
			OnChain: pldtypes.OnChainLocation{
				TransactionHash: pldtypes.RandBytes32(),
			},
		},
		PSC: mocks.domainAPI,
	}

	sm.handleTransactionConfirmedSuccess(ctx, completion, &nonce)

}

func TestSequencerManager_handleTransactionConfirmedSuccess_LoadedQueuesCoordinator_NoNonce(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()

	txID := uuid.New()
	txHash := pldtypes.RandBytes32()
	completion := &components.TxCompletion{
		ReceiptInput: components.ReceiptInput{
			TransactionID: txID,
			OnChain: pldtypes.OnChainLocation{
				TransactionHash: txHash,
			},
		},
		PSC: mocks.domainAPI,
	}

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinatorTx.ConfirmedSuccessEvent)
		return ok && event.TransactionID == txID && event.Hash == txHash
	})).Once()

	sm.handleTransactionConfirmedSuccess(ctx, completion, nil)

}

func TestSequencerManager_handleTransactionConfirmedSuccess_LoadedQueuesCoordinator_WithNonce(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()

	txID := uuid.New()
	txHash := pldtypes.RandBytes32()
	nonce := pldtypes.HexUint64(42)
	completion := &components.TxCompletion{
		ReceiptInput: components.ReceiptInput{
			TransactionID: txID,
			OnChain: pldtypes.OnChainLocation{
				TransactionHash: txHash,
			},
		},
		PSC: mocks.domainAPI,
	}

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinatorTx.ConfirmedSuccessEvent)
		return ok &&
			event.TransactionID == txID &&
			event.Hash == txHash &&
			event.Nonce != nil &&
			*event.Nonce == nonce
	})).Once()

	sm.handleTransactionConfirmedSuccess(ctx, completion, &nonce)

}

func TestSequencerManager_HandleDirectTransactionRevert_LoadedQueuesCoordinatorAndOriginator(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	from := pldtypes.RandAddress()
	txID := uuid.New()
	txHash := pldtypes.RandBytes32()
	revertReason := pldtypes.HexBytes("0x1234")
	nonce := uint64(77)
	dbTX := persistencemocks.NewDBTX(t)

	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	mocks.metrics.EXPECT().IncRevertedTransactions().Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinatorTx.ConfirmedRevertedEvent)
		if !ok {
			return false
		}
		return event.TransactionID == txID &&
			event.Hash == txHash &&
			event.RevertReason.Equals(revertReason) &&
			event.Nonce != nil && uint64(*event.Nonce) == nonce
	})).Once()
	err := sm.HandleDirectTransactionRevert(ctx, dbTX, []*components.PublicTxMatch{
		{
			PaladinTXReference: components.PaladinTXReference{
				TransactionID:              txID,
				TransactionSender:          "sender@node1",
				TransactionContractAddress: contractAddr.String(),
			},
			IndexedTransactionNotify: &blockindexer.IndexedTransactionNotify{
				IndexedTransaction: pldapi.IndexedTransaction{
					Hash:             txHash,
					BlockNumber:      12345,
					TransactionIndex: 7,
					From:             from,
					To:               contractAddr,
					Nonce:            nonce,
				},
				RevertReason: revertReason,
			},
		},
	})
	require.NoError(t, err)
}

func TestSequencerManager_HandleChainedTransactionOutcome_Success_LoadedQueuesCoordinator(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinatorTx.ConfirmedSuccessEvent)
		return ok && event.TransactionID == txID
	})).Once()

	sm.HandleChainedTransactionOutcome(ctx, *contractAddr, txID, components.RT_Success, "", nil, pldtypes.OnChainLocation{})
}

func TestSequencerManager_HandleChainedTransactionOutcome_OnChainRevert_LoadedQueuesCoordinator(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	revertData := pldtypes.HexBytes("0xdeadbeef")
	onChain := pldtypes.OnChainLocation{
		Type:            pldtypes.OnChainTransaction,
		TransactionHash: pldtypes.RandBytes32(),
		BlockNumber:     100,
	}

	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinatorTx.ConfirmedRevertedEvent)
		return ok &&
			event.TransactionID == txID &&
			event.RevertReason.Equals(revertData) &&
			event.OnChain.BlockNumber == 100
	})).Once()

	sm.HandleChainedTransactionOutcome(ctx, *contractAddr, txID, components.RT_FailedOnChainWithRevertData, "", revertData, onChain)
}

func TestSequencerManager_HandleChainedTransactionOutcome_OffChainRevert_LoadedQueuesCoordinator(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	failureMessage := "assembly revert"

	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinatorTx.ConfirmedRevertedEvent)
		return ok && event.TransactionID == txID && len(event.RevertReason) == 0 && event.FailureMessage == failureMessage
	})).Once()

	sm.HandleChainedTransactionOutcome(ctx, *contractAddr, txID, components.RT_FailedWithMessage, failureMessage, nil, pldtypes.OnChainLocation{})
}

func TestSequencerManager_HandleChainedTransactionOutcome_NotLoaded_NoOp(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// No sequencer loaded - should be a no-op (no panic, no coordinator call)
	sm.HandleChainedTransactionOutcome(ctx, *contractAddr, txID, components.RT_Success, "", nil, pldtypes.OnChainLocation{})
}

func TestSequencerManager_stopLowestPrioritySequencer_NoSequencers(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Call stopLowestPrioritySequencer
	sm.stopLowestPrioritySequencer(ctx)

	// Should not panic or error
	assert.Empty(t, sm.sequencers)
}

func TestSequencerManager_stopLowestPrioritySequencer_SequencerAlreadyClosing(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer that's already closing
	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Closing_Flush)

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	// Call stopLowestPrioritySequencer
	sm.stopLowestPrioritySequencer(ctx)

	// Verify sequencer is still in the map (not stopped)
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Contains(t, sm.sequencers, contractAddr.String())
}

func TestSequencerManager_stopLowestPrioritySequencer_IdleSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create an idle sequencer
	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Idle)
	mocks.originator.EXPECT().GetCurrentState().Return(originator.State_Idle)
	mocks.originator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks.coordinator.EXPECT().WaitForDone(mock.Anything).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	// Call stopLowestPrioritySequencer
	sm.stopLowestPrioritySequencer(ctx)

	// Verify sequencer was removed
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.NotContains(t, sm.sequencers, contractAddr.String())
}

func TestSequencerManager_stopLowestPrioritySequencer_IdleCoordinatorBusyOriginator(t *testing.T) {
	ctx := context.Background()
	contractAddr1 := pldtypes.RandAddress()
	contractAddr2 := pldtypes.RandAddress()
	mocks1 := newSequencerLifecycleTestMocks(t)
	mocks2 := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks1)

	seq1 := newSequencerForTesting(contractAddr1, mocks1)
	seq1.lastTXTime = time.Now().Add(-1 * time.Hour) // Newer
	seq2 := newSequencerForTesting(contractAddr2, mocks2)
	seq2.lastTXTime = time.Now().Add(-2 * time.Hour) // Older

	// seq1 has an idle coordinator but a busy originator, so it should not be immediately paged out.
	mocks1.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Idle)
	mocks1.originator.EXPECT().GetCurrentState().Return(originator.State_Sending)
	mocks2.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks2.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks2.originator.EXPECT().WaitForDone(mock.Anything).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr1.String()] = seq1
	sm.sequencers[contractAddr2.String()] = seq2
	sm.sequencersLock.Unlock()

	sm.stopLowestPrioritySequencer(ctx)

	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Contains(t, sm.sequencers, contractAddr1.String())
	assert.NotContains(t, sm.sequencers, contractAddr2.String())
}

func TestSequencerManager_stopLowestPrioritySequencer_LowestPriority(t *testing.T) {
	ctx := context.Background()
	contractAddr1 := pldtypes.RandAddress()
	contractAddr2 := pldtypes.RandAddress()
	mocks1 := newSequencerLifecycleTestMocks(t)
	mocks2 := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks1)

	// Create two sequencers with different lastTXTime
	seq1 := newSequencerForTesting(contractAddr1, mocks1)
	seq1.lastTXTime = time.Now().Add(-2 * time.Hour) // Older

	seq2 := newSequencerForTesting(contractAddr2, mocks2)
	seq2.lastTXTime = time.Now().Add(-1 * time.Hour) // Newer

	// Setup expectations - both are active, seq1 should be stopped
	mocks1.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks2.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks1.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks1.originator.EXPECT().WaitForDone(mock.Anything).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr1.String()] = seq1
	sm.sequencers[contractAddr2.String()] = seq2
	sm.sequencersLock.Unlock()

	// Call stopLowestPrioritySequencer
	sm.stopLowestPrioritySequencer(ctx)

	// Verify only seq1 was removed (lowest priority)
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.NotContains(t, sm.sequencers, contractAddr1.String())
	assert.Contains(t, sm.sequencers, contractAddr2.String())
}

func TestSequencerManager_StopAllSequencers_NoSequencers(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Call StopAllSequencers with empty sequencers map
	sm.StopAllSequencers(ctx)

	assert.Empty(t, sm.sequencers)
}

func TestSequencerManager_StopAllSequencers_SingleSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create and store a sequencer
	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	// Setup expectations for shutdown waits
	mocks.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks.originator.EXPECT().WaitForDone(mock.Anything).Once()

	// Call StopAllSequencers
	sm.StopAllSequencers(ctx)

	// Sequencers map should still contain the sequencer (it's not deleted, just stopped)
	assert.Contains(t, sm.sequencers, contractAddr.String())

	mocks.coordinator.AssertExpectations(t)
	mocks.originator.AssertExpectations(t)
}

func TestSequencerManager_StopAllSequencers_MultipleSequencers(t *testing.T) {
	ctx := context.Background()
	contractAddr1 := pldtypes.RandAddress()
	contractAddr2 := pldtypes.RandAddress()
	contractAddr3 := pldtypes.RandAddress()
	mocks1 := newSequencerLifecycleTestMocks(t)
	mocks2 := newSequencerLifecycleTestMocks(t)
	mocks3 := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks1)

	// Create and store multiple sequencers
	seq1 := newSequencerForTesting(contractAddr1, mocks1)
	seq2 := newSequencerForTesting(contractAddr2, mocks2)
	seq3 := newSequencerForTesting(contractAddr3, mocks3)

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr1.String()] = seq1
	sm.sequencers[contractAddr2.String()] = seq2
	sm.sequencers[contractAddr3.String()] = seq3
	sm.sequencersLock.Unlock()

	// Setup expectations for shutdown waits on all sequencers
	mocks1.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks1.originator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks2.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks2.originator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks3.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks3.originator.EXPECT().WaitForDone(mock.Anything).Once()

	// Verify shutdown is initially false
	sm.sequencersLock.RLock()
	initialCount := len(sm.sequencers)
	sm.sequencersLock.RUnlock()
	assert.Equal(t, 3, initialCount)

	// Call StopAllSequencers
	sm.StopAllSequencers(ctx)

	// Verify shutdown flag is set to true
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	// All sequencers should still be in the map (they're not deleted, just stopped)
	assert.Contains(t, sm.sequencers, contractAddr1.String())
	assert.Contains(t, sm.sequencers, contractAddr2.String())
	assert.Contains(t, sm.sequencers, contractAddr3.String())
	assert.Equal(t, 3, len(sm.sequencers))

	mocks1.coordinator.AssertExpectations(t)
	mocks1.originator.AssertExpectations(t)
	mocks2.coordinator.AssertExpectations(t)
	mocks2.originator.AssertExpectations(t)
	mocks3.coordinator.AssertExpectations(t)
	mocks3.originator.AssertExpectations(t)
}

// Tests for PreInit, PostInit, Start, Stop, and NewDistributedSequencerManager

func TestSequencerManager_PreInit_Success(t *testing.T) {
	config := &pldconf.SequencerConfig{}
	sMgr := NewDistributedSequencerManager(t.Context(), config).(*sequencerManager)

	// Create mocks
	preInitComponents := componentsmocks.NewPreInitComponents(t)
	metricsManager := metricsmocks.NewMetrics(t)
	registry := prometheus.NewRegistry()

	// Setup expectations
	preInitComponents.EXPECT().MetricsManager().Return(metricsManager).Once()
	metricsManager.EXPECT().Registry().Return(registry).Once()

	// Call PreInit
	result, err := sMgr.PreInit(preInitComponents)
	require.NoError(t, err)
	// Verify results

	assert.NotNil(t, result)
	assert.Nil(t, result.PreCommitHandler)
	assert.NotNil(t, sMgr.metrics)
}

func TestSequencerManager_PostInit_Success(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		Writer: pldconf.FlushWriterConfig{},
	}
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Create mocks
	allComponents := componentsmocks.NewAllComponents(t)
	transportManager := componentsmocks.NewTransportManager(t)
	persistence := persistencemocks.NewPersistence(t)
	txManager := componentsmocks.NewTXManager(t)
	publicTxManager := componentsmocks.NewPublicTxManager(t)

	// Setup expectations
	allComponents.EXPECT().TransportManager().Return(transportManager).Twice() // Called once for nodeName, once for NewSyncPoints
	transportManager.EXPECT().LocalNodeName().Return("test-node").Once()
	allComponents.EXPECT().Persistence().Return(persistence).Once()
	allComponents.EXPECT().TxManager().Return(txManager).Once()
	allComponents.EXPECT().PublicTxManager().Return(publicTxManager).Once()

	// Call PostInit
	err := sMgr.PostInit(allComponents)
	require.NoError(t, err)
	// Verify results

	assert.Equal(t, allComponents, sMgr.components)
	assert.Equal(t, "test-node", sMgr.nodeName)
	assert.NotNil(t, sMgr.syncPoints)
}

func TestSequencerManager_Start_Success(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		TransactionResumePollInterval: confutil.P("1s"),
		Writer:                        pldconf.FlushWriterConfig{},
	}
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Create mocks
	allComponents := componentsmocks.NewAllComponents(t)
	transportManager := componentsmocks.NewTransportManager(t)
	persistence := persistencemocks.NewPersistence(t)
	txManager := componentsmocks.NewTXManager(t)
	publicTxManager := componentsmocks.NewPublicTxManager(t)
	blockIndexer := blockindexermocks.NewBlockIndexer(t)

	// Setup PostInit first
	allComponents.EXPECT().TransportManager().Return(transportManager).Twice() // Called once for nodeName, once for NewSyncPoints
	transportManager.EXPECT().LocalNodeName().Return("test-node").Once()
	allComponents.EXPECT().Persistence().Return(persistence).Once()
	allComponents.EXPECT().TxManager().Return(txManager).Once()
	allComponents.EXPECT().PublicTxManager().Return(publicTxManager).Once()

	err := sMgr.PostInit(allComponents)
	require.NoError(t, err)

	// Setup expectations for pollForIncompleteTransactions
	allComponents.EXPECT().BlockIndexer().Return(blockIndexer).Maybe()
	blockIndexer.EXPECT().GetConfirmedBlockHeight(mock.Anything).Return(pldtypes.HexUint64(100), nil).Maybe()
	allComponents.EXPECT().TxManager().Return(txManager).Maybe()
	allComponents.EXPECT().Persistence().Return(persistence).Maybe()
	persistence.EXPECT().NOTX().Return(nil).Maybe()
	txManager.EXPECT().QueryTransactionsResolved(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*components.ResolvedTransaction{}, nil).Maybe()

	// Call Start
	err = sMgr.Start()

	// Verify results

	// Stop to clean up
	sMgr.Stop()
}

func TestSequencerManager_Start_ZeroPollInterval(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		TransactionResumePollInterval: confutil.P("-1s"), // Disabled (negative value disables polling)
		Writer:                        pldconf.FlushWriterConfig{},
	}
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Create mocks
	allComponents := componentsmocks.NewAllComponents(t)
	transportManager := componentsmocks.NewTransportManager(t)
	persistence := persistencemocks.NewPersistence(t)
	txManager := componentsmocks.NewTXManager(t)
	publicTxManager := componentsmocks.NewPublicTxManager(t)

	// Setup PostInit first
	allComponents.EXPECT().TransportManager().Return(transportManager).Times(2) // Called twice: once for nodeName, once for NewSyncPoints
	transportManager.EXPECT().LocalNodeName().Return("test-node").Once()
	allComponents.EXPECT().Persistence().Return(persistence).Once()
	allComponents.EXPECT().TxManager().Return(txManager).Once()
	allComponents.EXPECT().PublicTxManager().Return(publicTxManager).Once()

	err := sMgr.PostInit(allComponents)
	require.NoError(t, err)

	// Call Start - should not poll when interval is 0
	err = sMgr.Start()
	require.NoError(t, err)

	// Stop to clean up
	sMgr.Stop()
}

func TestSequencerManager_Stop_Success(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		Writer: pldconf.FlushWriterConfig{},
	}
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Create mocks
	allComponents := componentsmocks.NewAllComponents(t)
	transportManager := componentsmocks.NewTransportManager(t)
	persistence := persistencemocks.NewPersistence(t)
	txManager := componentsmocks.NewTXManager(t)
	publicTxManager := componentsmocks.NewPublicTxManager(t)

	// Setup PostInit
	allComponents.EXPECT().TransportManager().Return(transportManager).Twice()
	transportManager.EXPECT().LocalNodeName().Return("test-node").Once()
	allComponents.EXPECT().Persistence().Return(persistence).Once()
	allComponents.EXPECT().TxManager().Return(txManager).Once()
	allComponents.EXPECT().PublicTxManager().Return(publicTxManager).Once()

	err := sMgr.PostInit(allComponents)
	require.NoError(t, err)

	// Add a sequencer to test StopAllSequencers
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	seq := newSequencerForTesting(contractAddr, mocks)
	sMgr.sequencersLock.Lock()
	sMgr.sequencers[contractAddr.String()] = seq
	sMgr.sequencersLock.Unlock()

	// Setup expectations for Stop
	mocks.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks.originator.EXPECT().WaitForDone(mock.Anything).Once()

	// Call Stop
	sMgr.Stop()

	// Verify context is cancelled (we can't easily test this, but Stop should complete)
	// The syncPoints.Close() is called, and cancelCtx() is called
	// We verify that Stop completes without error
}

func TestSequencerManager_Stop_NoSequencers(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		Writer: pldconf.FlushWriterConfig{},
	}
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Create mocks
	allComponents := componentsmocks.NewAllComponents(t)
	transportManager := componentsmocks.NewTransportManager(t)
	persistence := persistencemocks.NewPersistence(t)
	txManager := componentsmocks.NewTXManager(t)
	publicTxManager := componentsmocks.NewPublicTxManager(t)

	// Setup PostInit
	allComponents.EXPECT().TransportManager().Return(transportManager).Twice()
	transportManager.EXPECT().LocalNodeName().Return("test-node").Once()
	allComponents.EXPECT().Persistence().Return(persistence).Once()
	allComponents.EXPECT().TxManager().Return(txManager).Once()
	allComponents.EXPECT().PublicTxManager().Return(publicTxManager).Once()

	err := sMgr.PostInit(allComponents)
	require.NoError(t, err)

	// Call Stop with no sequencers - should not panic
	sMgr.Stop()

	// Verify Stop completes successfully
	assert.Empty(t, sMgr.sequencers)
}

func TestNewDistributedSequencerManager_Success(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		TargetActiveSequencers: confutil.P(20),
	}

	// Call constructor
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Verify initial state
	assert.NotNil(t, sMgr.ctx)
	assert.NotNil(t, sMgr.cancelCtx)
	assert.Equal(t, config, sMgr.config)
	assert.NotNil(t, sMgr.sequencers)
	assert.Equal(t, 0, len(sMgr.sequencers))
	assert.Equal(t, 20, sMgr.targetActiveSequencersLimit)
}

func TestNewDistributedSequencerManager_DefaultLimits(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		// No limits specified - should use defaults
	}

	// Call constructor
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Verify default limits are applied
	assert.Greater(t, sMgr.targetActiveSequencersLimit, 0)
}

func TestNewDistributedSequencerManager_MinimumLimits(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		TargetActiveSequencers: confutil.P(0), // Below minimum
	}

	// Call constructor
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Verify minimum limits are applied
	assert.GreaterOrEqual(t, sMgr.targetActiveSequencersLimit, pldconf.SequencerMinimum.TargetActiveSequencers)
}

func TestSequencerManager_GetNodeName(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sMgr := newSequencerManagerForTesting(t, mocks)

	// Test that GetNodeName returns the expected node name
	expectedNodeName := "test-node"
	assert.Equal(t, expectedNodeName, sMgr.GetNodeName())

	// Test with a different node name
	sMgr2 := &sequencerManager{
		ctx:      ctx,
		nodeName: "another-node",
	}
	assert.Equal(t, "another-node", sMgr2.GetNodeName())
}

func TestSequencerManager_GetTxStatus_Success(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sMgr := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer and add it to the manager (so LoadSequencer will find it)
	seq := newSequencerForTesting(contractAddr, mocks)
	sMgr.sequencers[contractAddr.String()] = seq

	// Setup expectations for LoadSequencer when sequencer already exists
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	// Setup expectations for GetTxStatus
	txID := uuid.New()
	expectedStatus := components.PrivateTxStatus{
		TxID:   txID.String(),
		Status: "pending",
	}
	mocks.originator.EXPECT().GetTxStatus(ctx, txID).Return(expectedStatus, nil).Once()

	// Call GetTxStatus
	status, err := sMgr.GetTxStatus(ctx, contractAddr.String(), txID)
	require.NoError(t, err)
	// Verify results

	assert.Equal(t, expectedStatus, status)
	assert.Equal(t, txID.String(), status.TxID)
	assert.Equal(t, "pending", status.Status)
}

func TestSequencerManager_GetTxStatus_LoadSequencerError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sMgr := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for LoadSequencer to return an error
	// GetSmartContractByAddress expects a value type, not a pointer
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("domain not found")).Once()

	txID := uuid.New()

	// Call GetTxStatus
	status, err := sMgr.GetTxStatus(ctx, contractAddr.String(), txID)

	// Verify that error is returned and status is "unknown"
	// Note: LoadSequencer returns nil, nil when GetSmartContractByAddress returns an error (treats as deploy case)
	assert.Equal(t, "unknown", status.Status)
	assert.Equal(t, txID.String(), status.TxID)
	assert.NoError(t, err) // LoadSequencer returns nil, nil in this case, not an error
}

func TestSequencerManager_GetTxStatus_NilSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sMgr := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for LoadSequencer to return nil sequencer (no error, but sequencer is nil)
	// GetSmartContractByAddress expects a value type, not a pointer
	// When it returns an error, LoadSequencer returns nil, nil (treats as deploy case)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("domain not found")).Once()
	mocks.metrics.EXPECT().SetActiveSequencers(0).Maybe()

	txID := uuid.New()

	// Call GetTxStatus
	status, err := sMgr.GetTxStatus(ctx, contractAddr.String(), txID)

	// Verify that status is "unknown" and no error is returned (LoadSequencer returns nil, nil for deploy case)
	// Note: LoadSequencer returns nil, nil when GetSmartContractByAddress returns an error (treats as deploy case)
	assert.Equal(t, "unknown", status.Status)
	assert.Equal(t, txID.String(), status.TxID)
	assert.NoError(t, err) // LoadSequencer returns nil, nil in this case, not an error
}

func TestSequencerManager_GetTxStatus_OriginatorError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sMgr := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer and add it to the manager (so LoadSequencer will find it)
	seq := newSequencerForTesting(contractAddr, mocks)
	sMgr.sequencers[contractAddr.String()] = seq

	// Setup expectations for LoadSequencer when sequencer already exists
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	// Setup expectations for GetTxStatus to return an error
	txID := uuid.New()
	expectedError := errors.New("transaction not found")
	mocks.originator.EXPECT().GetTxStatus(ctx, txID).Return(components.PrivateTxStatus{}, expectedError).Once()

	// Call GetTxStatus
	status, err := sMgr.GetTxStatus(ctx, contractAddr.String(), txID)

	// Verify that error is returned
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Equal(t, "", status.TxID) // Empty status when error occurs
}

func TestSequencerManager_PrivateTransactionsConfirmed_PreservesOrder(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer func() { require.NoError(t, mp.Mock.ExpectationsWereMet()) }()
	mocks := newSequencerLifecycleTestMocksWithPersistence(t, mp.P)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	txID1 := uuid.New()
	txID2 := uuid.New()
	txID3 := uuid.New()
	txHash1 := pldtypes.RandBytes32()
	txHash2 := pldtypes.RandBytes32()
	txHash3 := pldtypes.RandBytes32()

	completions := []*components.TxCompletion{
		{
			ReceiptInput: components.ReceiptInput{
				TransactionID: txID1,
				OnChain:       pldtypes.OnChainLocation{TransactionHash: txHash1, BlockNumber: 100, TransactionIndex: 0},
			},
			PSC: mocks.domainAPI,
		},
		{
			ReceiptInput: components.ReceiptInput{
				TransactionID: txID2,
				OnChain:       pldtypes.OnChainLocation{TransactionHash: txHash2, BlockNumber: 100, TransactionIndex: 1},
			},
			PSC: mocks.domainAPI,
		},
		{
			ReceiptInput: components.ReceiptInput{
				TransactionID: txID3,
				OnChain:       pldtypes.OnChainLocation{TransactionHash: txHash3, BlockNumber: 100, TransactionIndex: 2},
			},
			PSC: mocks.domainAPI,
		},
	}

	mp.Mock.ExpectQuery("SELECT count\\(\\*\\).*chained_dispatches").WithArgs(txID1).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mp.Mock.ExpectQuery("SELECT count\\(\\*\\).*chained_dispatches").WithArgs(txID2).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mp.Mock.ExpectQuery("SELECT count\\(\\*\\).*chained_dispatches").WithArgs(txID3).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mocks.publicTxManager.EXPECT().QueryPublicTxForTransactions(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Times(3)

	mocks.metrics.EXPECT().IncConfirmedTransactions().Times(3)
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Times(3)

	var confirmedOrder []uuid.UUID
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinatorTx.ConfirmedSuccessEvent)
		if ok {
			confirmedOrder = append(confirmedOrder, event.TransactionID)
		}
		return ok
	})).Times(3)

	sm.PrivateTransactionsConfirmed(ctx, completions)

	require.Len(t, confirmedOrder, 3)
	assert.Equal(t, txID1, confirmedOrder[0], "first confirmation should be txID1")
	assert.Equal(t, txID2, confirmedOrder[1], "second confirmation should be txID2")
	assert.Equal(t, txID3, confirmedOrder[2], "third confirmation should be txID3")
}

func TestSequencerManager_PrivateTransactionsConfirmed_SkipsDeploys(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer func() { require.NoError(t, mp.Mock.ExpectationsWereMet()) }()
	mocks := newSequencerLifecycleTestMocksWithPersistence(t, mp.P)
	sm := newSequencerManagerForTesting(t, mocks)

	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	completions := []*components.TxCompletion{
		{
			ReceiptInput: components.ReceiptInput{
				TransactionID:   txID,
				ContractAddress: contractAddr,
				OnChain:         pldtypes.OnChainLocation{TransactionHash: pldtypes.RandBytes32(), BlockNumber: 100},
			},
			PSC: mocks.domainAPI,
		},
	}

	mocks.publicTxManager.EXPECT().QueryPublicTxForTransactions(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()

	sm.PrivateTransactionsConfirmed(ctx, completions)
}

func TestSequencerManager_PrivateTransactionsConfirmed_SynchronousProcessing(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer func() { require.NoError(t, mp.Mock.ExpectationsWereMet()) }()
	mocks := newSequencerLifecycleTestMocksWithPersistence(t, mp.P)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	txID := uuid.New()
	txHash := pldtypes.RandBytes32()
	completions := []*components.TxCompletion{
		{
			ReceiptInput: components.ReceiptInput{
				TransactionID: txID,
				OnChain:       pldtypes.OnChainLocation{TransactionHash: txHash, BlockNumber: 100},
			},
			PSC: mocks.domainAPI,
		},
	}

	mp.Mock.ExpectQuery("SELECT count\\(\\*\\).*chained_dispatches").WithArgs(txID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mocks.publicTxManager.EXPECT().QueryPublicTxForTransactions(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinatorTx.ConfirmedSuccessEvent)
		return ok && event.TransactionID == txID && event.Hash == txHash
	})).Once()

	sm.PrivateTransactionsConfirmed(ctx, completions)
}

func TestSequencerManager_removeIdleSequencers_BothIdle(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Idle)
	mocks.originator.EXPECT().GetCurrentState().Return(originator.State_Idle)
	mocks.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks.originator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	sm.removeIdleSequencers(ctx)

	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.NotContains(t, sm.sequencers, contractAddr.String())
}

func TestSequencerManager_removeIdleSequencers_CoordinatorActive(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks.originator.EXPECT().GetCurrentState().Return(originator.State_Idle)
	mocks.metrics.EXPECT().SetActiveSequencers(1).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	sm.removeIdleSequencers(ctx)

	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Contains(t, sm.sequencers, contractAddr.String())
}

func TestSequencerManager_removeIdleSequencers_OriginatorSending(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Idle)
	mocks.originator.EXPECT().GetCurrentState().Return(originator.State_Sending)
	mocks.metrics.EXPECT().SetActiveSequencers(1).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	sm.removeIdleSequencers(ctx)

	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Contains(t, sm.sequencers, contractAddr.String())
}

func TestSequencerManager_removeIdleSequencers_ObservingNotRemoved(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Observing)
	mocks.originator.EXPECT().GetCurrentState().Return(originator.State_Observing)
	mocks.metrics.EXPECT().SetActiveSequencers(1).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	sm.removeIdleSequencers(ctx)

	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Contains(t, sm.sequencers, contractAddr.String())
}

func TestSequencerManager_removeIdleSequencers_MixedStates(t *testing.T) {
	ctx := context.Background()
	idleAddr := pldtypes.RandAddress()
	activeAddr := pldtypes.RandAddress()
	idleMocks := newSequencerLifecycleTestMocks(t)
	activeMocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, idleMocks)

	idleSeq := newSequencerForTesting(idleAddr, idleMocks)
	idleMocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Idle)
	idleMocks.originator.EXPECT().GetCurrentState().Return(originator.State_Idle)
	idleMocks.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	idleMocks.originator.EXPECT().WaitForDone(mock.Anything).Once()

	activeSeq := newSequencerForTesting(activeAddr, activeMocks)
	activeMocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	activeMocks.originator.EXPECT().GetCurrentState().Return(originator.State_Sending)

	idleMocks.metrics.EXPECT().SetActiveSequencers(1).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[idleAddr.String()] = idleSeq
	sm.sequencers[activeAddr.String()] = activeSeq
	sm.sequencersLock.Unlock()

	sm.removeIdleSequencers(ctx)

	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.NotContains(t, sm.sequencers, idleAddr.String())
	assert.Contains(t, sm.sequencers, activeAddr.String())
}

func TestSequencerManager_removeIdleSequencers_Empty(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()

	sm.removeIdleSequencers(ctx)

	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Empty(t, sm.sequencers)
}

func TestHeartbeatLoop_SendsPeriodicHeartbeats(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)

	coordReceived := make(chan struct{}, 10)
	origReceived := make(chan struct{}, 10)
	mocks.coordinator.EXPECT().QueueEvent(mock.Anything, mock.AnythingOfType("*common.HeartbeatIntervalEvent")).Run(
		func(_ context.Context, _ common.Event) { coordReceived <- struct{}{} },
	).Return()
	mocks.originator.EXPECT().QueueEvent(mock.Anything, mock.AnythingOfType("*common.HeartbeatIntervalEvent")).Run(
		func(_ context.Context, _ common.Event) { origReceived <- struct{}{} },
	).Return()

	seq := &sequencer{
		contractAddress: contractAddr.String(),
		coordinator:     mocks.coordinator,
		originator:      mocks.originator,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go seq.heartbeatLoop(ctx, 50*time.Millisecond)

	// The initial immediate event should arrive quickly
	<-coordReceived
	<-origReceived

	// then wait for the period events
	<-coordReceived
	<-origReceived
}

func TestHeartbeatLoop_StopsWhenContextCancelled(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)

	mocks.coordinator.EXPECT().QueueEvent(mock.Anything, mock.AnythingOfType("*common.HeartbeatIntervalEvent")).Return()
	mocks.originator.EXPECT().QueueEvent(mock.Anything, mock.AnythingOfType("*common.HeartbeatIntervalEvent")).Return()

	seq := &sequencer{
		contractAddress: contractAddr.String(),
		coordinator:     mocks.coordinator,
		originator:      mocks.originator,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		seq.heartbeatLoop(ctx, 10*time.Second)
		close(done)
	}()

	cancel()
	<-done
}

func TestSequencer_shutdown_NilCancelCtx(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)

	seq := &sequencer{
		contractAddress: contractAddr.String(),
		originator:      mocks.originator,
		coordinator:     mocks.coordinator,
		cancelCtx:       nil,
	}
	seq.shutdown(ctx)
}

func TestSequencerManager_cleanupIdleSequencers_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	ticked := make(chan struct{})
	var once sync.Once
	mocks.metrics.EXPECT().SetActiveSequencers(0).Run(func(n int) {
		once.Do(func() { close(ticked) })
	}).Maybe()

	sm.cleanupIdleSequencers(ctx, 20*time.Millisecond)
	<-ticked
}

func TestSequencerManager_LoadSequencer_WithProvidedDomainAPI(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomainSmartContract := componentsmocks.NewDomainSmartContract(t)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.EXPECT().GetBlockHeight().Return(int64(0))
	mockDomain.EXPECT().Name().Return("testDomain").Maybe()
	mockDomainSmartContract.EXPECT().Domain().Return(mockDomain).Maybe()
	mockDomainSmartContract.EXPECT().ContractConfig().Return(&prototk.ContractConfig{StaticCoordinator: proto.String("test-identity@test-coordinator")}).Maybe()
	mocks.components.EXPECT().TransportManager().Return(mocks.transportManager).Maybe()
	mocks.transportManager.EXPECT().LocalNodeName().Return("test-node").Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mockDomainSmartContract, nil).Once()
	mocks.stateManager.EXPECT().NewDomainStateWriter(mock.Anything, mockDomain, *contractAddr).Return(componentsmocks.NewDomainStateWriter(t)).Once()
	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()

	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, mockDomainSmartContract, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	sm.sequencers[contractAddr.String()].cancelCtx()
	result.GetCoordinator().WaitForDone(ctx)
	result.GetOriginator().WaitForDone(ctx)
}

func TestSequencerManager_LoadSequencer_CreationGetSmartContractError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(nil, errors.New("db error")).Once()

	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestSequencerManager_LoadSequencer_InvalidSelectionConfig(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomainSmartContract := componentsmocks.NewDomainSmartContract(t)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.EXPECT().Name().Return("testDomain").Maybe()
	mockDomainSmartContract.EXPECT().Domain().Return(mockDomain).Maybe()
	mockDomainSmartContract.EXPECT().ContractConfig().Return(&prototk.ContractConfig{StaticCoordinator: proto.String("not-a-valid-locator")}).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mockDomainSmartContract, nil).Once()
	mocks.stateManager.EXPECT().NewDomainStateWriter(mock.Anything, mockDomain, *contractAddr).Return(componentsmocks.NewDomainStateWriter(t)).Once()
	mocks.transportManager.EXPECT().LocalNodeName().Return("test-node").Maybe()

	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestSequencerManager_LoadSequencer_ReachesTargetLimit(t *testing.T) {
	ctx := context.Background()
	contractAddr1 := pldtypes.RandAddress()
	contractAddr2 := pldtypes.RandAddress()
	contractAddr3 := pldtypes.RandAddress()
	mocks1 := newSequencerLifecycleTestMocks(t)
	mocks2 := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks1)
	sm.targetActiveSequencersLimit = 2

	seq1 := newSequencerForTesting(contractAddr1, mocks1)
	seq1.lastTXTime = time.Now().Add(-2 * time.Hour)
	seq2 := newSequencerForTesting(contractAddr2, mocks2)
	seq2.lastTXTime = time.Now().Add(-1 * time.Hour)
	sm.sequencers[contractAddr1.String()] = seq1
	sm.sequencers[contractAddr2.String()] = seq2

	mocks1.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks2.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks1.coordinator.EXPECT().WaitForDone(mock.Anything).Once()
	mocks1.originator.EXPECT().WaitForDone(mock.Anything).Once()

	mockDomainSmartContract := componentsmocks.NewDomainSmartContract(t)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.EXPECT().GetBlockHeight().Return(int64(0))
	mockDomain.EXPECT().Name().Return("testDomain").Maybe()
	mockDomainSmartContract.EXPECT().Domain().Return(mockDomain).Maybe()
	mockDomainSmartContract.EXPECT().ContractConfig().Return(&prototk.ContractConfig{StaticCoordinator: proto.String("test-identity@test-coordinator")}).Maybe()
	mocks1.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr3).Return(mocks1.domainAPI, nil).Once()
	mocks1.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr3).Return(mockDomainSmartContract, nil).Once()
	mocks1.stateManager.EXPECT().NewDomainStateWriter(mock.Anything, mockDomain, *contractAddr3).Return(componentsmocks.NewDomainStateWriter(t)).Once()
	mocks1.transportManager.EXPECT().LocalNodeName().Return("test-node").Maybe()
	mocks1.metrics.EXPECT().SetActiveSequencers(1).Once()

	result, err := sm.LoadSequencer(ctx, nil, *contractAddr3, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotContains(t, sm.sequencers, contractAddr1.String())

	sm.sequencers[contractAddr3.String()].cancelCtx()
	result.GetCoordinator().WaitForDone(ctx)
	result.GetOriginator().WaitForDone(ctx)
}
