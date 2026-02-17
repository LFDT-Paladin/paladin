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

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/blockindexermocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/metricsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/blockindexer"
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
	transportWriter  *transport.MockTransportWriter
	originator       *originator.MockSeqOriginator
	coordinator      *coordinator.MockSeqCoordinator
	syncPoints       *syncpoints.MockSyncPoints
	metrics          *metrics.MockDistributedSequencerMetrics
}

func newSequencerLifecycleTestMocks(t *testing.T) *sequencerLifecycleTestMocks {

	return &sequencerLifecycleTestMocks{
		components:       componentsmocks.NewAllComponents(t),
		domainManager:    componentsmocks.NewDomainManager(t),
		stateManager:     componentsmocks.NewStateManager(t),
		transportManager: componentsmocks.NewTransportManager(t),
		persistence:      persistencemocks.NewPersistence(t),
		txManager:        componentsmocks.NewTXManager(t),
		publicTxManager:  componentsmocks.NewPublicTxManager(t),
		keyManager:       componentsmocks.NewKeyManager(t),
		domainAPI:        componentsmocks.NewDomainSmartContract(t),
		transportWriter:  transport.NewMockTransportWriter(t),
		originator:       originator.NewMockSeqOriginator(t),
		coordinator:      coordinator.NewMockSeqCoordinator(t),
		syncPoints:       syncpoints.NewMockSyncPoints(t),
		metrics:          metrics.NewMockDistributedSequencerMetrics(t),
	}
}

func (m *sequencerLifecycleTestMocks) setupDefaultExpectations(ctx context.Context, contractAddr *pldtypes.EthAddress) {
	// Setup default component expectations
	m.components.EXPECT().DomainManager().Return(m.domainManager).Maybe()
	m.components.EXPECT().StateManager().Return(m.stateManager).Maybe()
	m.components.EXPECT().TransportManager().Return(m.transportManager).Maybe()
	m.components.EXPECT().Persistence().Return(m.persistence).Maybe()
	m.components.EXPECT().TxManager().Return(m.txManager).Maybe()
	m.components.EXPECT().PublicTxManager().Return(m.publicTxManager).Maybe()
	m.components.EXPECT().KeyManager().Return(m.keyManager).Maybe()
	// m.components.EXPECT().MetricsManager().Return(m.metricsManager).Maybe()

	// Setup persistence expectations
	m.persistence.EXPECT().NOTX().Return(nil).Maybe()

	// Setup transport manager expectations
	m.transportManager.EXPECT().LocalNodeName().Return("test-node").Maybe()

	// Setup domain manager expectations
	m.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, contractAddr).Return(m.domainAPI, nil).Maybe()

	// // Setup domain API expectations
	// m.domainAPI.EXPECT().Domain().Return("test-domain").Maybe()
	// m.domainAPI.EXPECT().ContractConfig().Return(&mockContractConfig{}).Maybe()
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
		ctx:                           ctx,
		config:                        config,
		components:                    mocks.components,
		nodeName:                      "test-node",
		sequencersLock:                sync.RWMutex{},
		sequencers:                    make(map[string]*sequencer),
		metrics:                       mocks.metrics,
		targetActiveCoordinatorsLimit: 2,
		targetActiveSequencersLimit:   2,
	}

	return sm
}

func newSequencerForTesting(contractAddr *pldtypes.EthAddress, mocks *sequencerLifecycleTestMocks) *sequencer {
	return &sequencer{
		contractAddress: contractAddr.String(),
		originator:      mocks.originator,
		coordinator:     mocks.coordinator,
		transportWriter: mocks.transportWriter,
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

func TestSequencerManager_LoadSequencer_NewSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for new sequencer creation
	mocks.setupDefaultExpectations(ctx, contractAddr)
	mockDomainSmartContract := componentsmocks.NewDomainSmartContract(t)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomainSmartContract.EXPECT().Domain().Return(mockDomain).Twice()
	mockDomainSmartContract.EXPECT().ContractConfig().Return(&prototk.ContractConfig{StaticCoordinator: proto.String("test-identity@test-coordinator")}).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mockDomainSmartContract, nil)
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mockDomain, *contractAddr).Return(componentsmocks.NewDomainContext(t)).Twice()

	// Setup transport writer creation
	mocks.transportWriter.EXPECT().SendDispatched(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Setup originator creation expectations
	mocks.originator.EXPECT().SetActiveCoordinator(ctx, mock.Anything).Maybe()

	// Setup coordinator creation expectations
	mocks.coordinator.EXPECT().UpdateOriginatorNodePool(ctx, mock.Anything).Maybe()
	mocks.coordinator.EXPECT().GetActiveCoordinatorNode(ctx, true).Return("test-coordinator").Maybe()

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

	// Verify results
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.GetCoordinator())
	assert.NotNil(t, result.GetOriginator())
	assert.NotNil(t, result.GetTransportWriter())

	// Verify sequencer was stored
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Contains(t, sm.sequencers, contractAddr.String())
	mocks.metrics.AssertExpectations(t)
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

	// Setup coordinator expectations for existing sequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()

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

	// Verify results
	require.NoError(t, err)
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

	// Setup coordinator expectations - GetCurrentCoordinator returns empty string (no coordinator set)
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("").Once()

	// Create a mock private transaction with required verifiers
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "verifier1@node1"},
			},
		},
	}

	// Setup expectations for setInitialCoordinator to succeed
	mocks.coordinator.EXPECT().UpdateOriginatorNodePool(ctx, "node1").Once()
	mocks.coordinator.EXPECT().GetActiveCoordinatorNode(ctx, true).Return("test-coordinator").Once()
	mocks.originator.EXPECT().SetActiveCoordinator(ctx, "test-coordinator").Return(nil).Once()

	//  this should call setInitialCoordinator
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, tx)

	// Verify results
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, existingSeq, result)

	// Verify lastTXTime was updated
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	seq := sm.sequencers[contractAddr.String()]
	assert.True(t, seq.lastTXTime.After(time.Now().Add(-time.Second)))
}

func TestSequencerManager_LoadSequencer_ExistingSequencer_NoCoordinator_Error(t *testing.T) {
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

	// Setup coordinator expectations - GetCurrentCoordinator returns empty string (no coordinator set)
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("").Once()

	// Create a mock private transaction with required verifiers
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "verifier1@node1"},
			},
		},
	}

	// Setup expectations for setInitialCoordinator to fail
	mocks.coordinator.EXPECT().UpdateOriginatorNodePool(ctx, "node1").Once()
	mocks.coordinator.EXPECT().GetActiveCoordinatorNode(ctx, true).Return("test-coordinator").Once()
	expectedError := errors.New("failed to set active coordinator")
	mocks.originator.EXPECT().SetActiveCoordinator(ctx, "test-coordinator").Return(expectedError).Once()

	// this should call setInitialCoordinator
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, tx)

	// Verify that the error from setInitialCoordinator is returned
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
}

func TestSequencerManager_LoadSequencer_NoDomainAPI(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for domain manager returning nil
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("contract not found")).Once()

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)

	// Verify results
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestSequencerManager_LoadSequencer_DomainManagerError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for domain manager error
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("database error")).Once()

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)

	// Verify results
	require.NoError(t, err) // This should not error, just return nil
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
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Flush)

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
	mocks.originator.EXPECT().Stop().Once()
	mocks.coordinator.EXPECT().Stop().Once()

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
	mocks1.coordinator.EXPECT().Stop().Once()
	mocks1.originator.EXPECT().Stop().Once()

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

func TestSequencerManager_updateActiveCoordinators_NoActiveCoordinators(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer with inactive coordinator
	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Idle)
	mocks.metrics.EXPECT().SetActiveCoordinators(0).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	// Call updateActiveCoordinators
	sm.updateActiveCoordinators(ctx)

	mocks.metrics.AssertExpectations(t)
}

func TestSequencerManager_updateActiveCoordinators_ActiveCoordinators(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer with active coordinator
	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks.metrics.EXPECT().SetActiveCoordinators(1).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	// Call updateActiveCoordinators
	sm.updateActiveCoordinators(ctx)

	mocks.metrics.AssertExpectations(t)
}

func TestSequencerManager_updateActiveCoordinators_ExceedsLimit(t *testing.T) {
	ctx := context.Background()
	contractAddr1 := pldtypes.RandAddress()
	contractAddr2 := pldtypes.RandAddress()
	contractAddr3 := pldtypes.RandAddress()
	mocks1 := newSequencerLifecycleTestMocks(t)
	mocks2 := newSequencerLifecycleTestMocks(t)
	mocks3 := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks1)

	// Set limit to 2
	sm.targetActiveCoordinatorsLimit = 2

	// Create three sequencers with active coordinators
	seq1 := newSequencerForTesting(contractAddr1, mocks1)
	seq1.lastTXTime = time.Now().Add(-3 * time.Hour) // Oldest

	seq2 := newSequencerForTesting(contractAddr2, mocks2)
	seq2.lastTXTime = time.Now().Add(-2 * time.Hour) // Middle

	seq3 := newSequencerForTesting(contractAddr3, mocks3)
	seq3.lastTXTime = time.Now().Add(-1 * time.Hour) // Newest

	// Setup expectations - all are active, seq1 should be stopped
	mocks1.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks2.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks3.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks1.coordinator.EXPECT().Stop().Once()
	mocks1.originator.EXPECT().Stop().Once()
	mocks1.metrics.EXPECT().SetActiveCoordinators(3).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr1.String()] = seq1
	sm.sequencers[contractAddr2.String()] = seq2
	sm.sequencers[contractAddr3.String()] = seq3
	sm.sequencersLock.Unlock()

	// Call updateActiveCoordinators
	sm.updateActiveCoordinators(ctx)

	// Verify only seq1 was removed (lowest priority)
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.NotContains(t, sm.sequencers, contractAddr1.String())
	assert.Contains(t, sm.sequencers, contractAddr2.String())
	assert.Contains(t, sm.sequencers, contractAddr3.String())

	mocks1.metrics.AssertExpectations(t)
}

func TestSequencerManager_setInitialCoordinator_InvalidVerifierLookup(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer
	seq := newSequencerForTesting(contractAddr, mocks)

	// Create a transaction with an invalid verifier lookup (too many @ symbols)
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "invalid@format@too@many"}, // Invalid format - too many @ symbols
			},
		},
	}

	// Call setInitialCoordinator
	err := sm.setInitialCoordinator(ctx, tx, seq)

	// Verify that an error is returned
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD020006") // Error code for invalid private identity locator format
}

func TestSequencerManager_setInitialCoordinator_SetActiveCoordinatorError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer
	seq := newSequencerForTesting(contractAddr, mocks)

	// Create a transaction with valid required verifiers
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "verifier1@node1"},
			},
		},
	}

	// Setup expectations for successful verifier validation
	mocks.coordinator.EXPECT().UpdateOriginatorNodePool(ctx, "node1").Once()

	// Setup expectation for GetActiveCoordinatorNode to return a coordinator
	mocks.coordinator.EXPECT().GetActiveCoordinatorNode(ctx, true).Return("test-coordinator").Once()

	// Setup expectation for SetActiveCoordinator to return an error (this is what we're testing)
	expectedError := errors.New("failed to set active coordinator")
	mocks.originator.EXPECT().SetActiveCoordinator(ctx, "test-coordinator").Return(expectedError).Once()

	// Call setInitialCoordinator
	err := sm.setInitialCoordinator(ctx, tx, seq)

	// Verify that the error from SetActiveCoordinator is returned
	require.Error(t, err)
	assert.Equal(t, expectedError, err)
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

	// Setup expectations for Stop() calls
	mocks.coordinator.EXPECT().Stop().Once()
	mocks.originator.EXPECT().Stop().Once()

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

	// Setup expectations for Stop() calls on all sequencers
	mocks1.coordinator.EXPECT().Stop().Once()
	mocks1.originator.EXPECT().Stop().Once()
	mocks2.coordinator.EXPECT().Stop().Once()
	mocks2.originator.EXPECT().Stop().Once()
	mocks3.coordinator.EXPECT().Stop().Once()
	mocks3.originator.EXPECT().Stop().Once()

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
	ctx := context.Background()
	config := &pldconf.SequencerConfig{}
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Create mocks
	preInitComponents := componentsmocks.NewPreInitComponents(t)
	metricsManager := metricsmocks.NewMetrics(t)
	registry := prometheus.NewRegistry()

	// Setup expectations
	preInitComponents.EXPECT().MetricsManager().Return(metricsManager).Once()
	metricsManager.EXPECT().Registry().Return(registry).Once()

	// Call PreInit
	result, err := sMgr.PreInit(preInitComponents)

	// Verify results
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.PreCommitHandler)
	assert.NotNil(t, sMgr.metrics)

	// Verify PreCommitHandler can be called
	testBlocks := []*pldapi.IndexedBlock{
		{Number: 100},
		{Number: 101},
	}
	testTransactions := []*blockindexer.IndexedTransactionNotify{}
	mockDBTX := persistencemocks.NewDBTX(t)
	mockDBTX.EXPECT().AddPostCommit(mock.Anything).Once()

	err = result.PreCommitHandler(ctx, mockDBTX, testBlocks, testTransactions)
	require.NoError(t, err)
}

func TestSequencerManager_PreInit_PreCommitHandler_EmptyBlocks(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{}
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

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

	// Test PreCommitHandler exists
	// In practice, blocks should never be empty, but we test that the handler exists
	assert.NotNil(t, result.PreCommitHandler)
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

	// Verify results
	require.NoError(t, err)
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
	require.NoError(t, err)

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
	mocks.coordinator.EXPECT().Stop().Once()
	mocks.originator.EXPECT().Stop().Once()

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
		TargetActiveCoordinators: confutil.P(10),
		TargetActiveSequencers:   confutil.P(20),
	}

	// Call constructor
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Verify initial state
	assert.NotNil(t, sMgr.ctx)
	assert.NotNil(t, sMgr.cancelCtx)
	assert.Equal(t, config, sMgr.config)
	assert.NotNil(t, sMgr.sequencers)
	assert.Equal(t, 0, len(sMgr.sequencers))
	assert.Equal(t, 10, sMgr.targetActiveCoordinatorsLimit)
	assert.Equal(t, 20, sMgr.targetActiveSequencersLimit)
	assert.Equal(t, int64(0), sMgr.blockHeight)
}

func TestNewDistributedSequencerManager_DefaultLimits(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		// No limits specified - should use defaults
	}

	// Call constructor
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Verify default limits are applied
	assert.Greater(t, sMgr.targetActiveCoordinatorsLimit, 0)
	assert.Greater(t, sMgr.targetActiveSequencersLimit, 0)
}

func TestNewDistributedSequencerManager_MinimumLimits(t *testing.T) {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{
		TargetActiveCoordinators: confutil.P(0), // Below minimum
		TargetActiveSequencers:   confutil.P(0), // Below minimum
	}

	// Call constructor
	sMgr := NewDistributedSequencerManager(ctx, config).(*sequencerManager)

	// Verify minimum limits are applied
	assert.GreaterOrEqual(t, sMgr.targetActiveCoordinatorsLimit, pldconf.SequencerMinimum.TargetActiveCoordinators)
	assert.GreaterOrEqual(t, sMgr.targetActiveSequencersLimit, pldconf.SequencerMinimum.TargetActiveSequencers)
}

func TestSequencerManager_OnNewBlockHeight(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sMgr := newSequencerManagerForTesting(t, mocks)

	// Test initial block height is 0
	assert.Equal(t, int64(0), sMgr.GetBlockHeight())

	// Test setting block height
	testHeight := int64(100)
	sMgr.OnNewBlockHeight(ctx, testHeight)
	assert.Equal(t, testHeight, sMgr.GetBlockHeight())

	// Test updating block height
	newHeight := int64(200)
	sMgr.OnNewBlockHeight(ctx, newHeight)
	assert.Equal(t, newHeight, sMgr.GetBlockHeight())

	// Test concurrent updates to ensure thread safety
	var wg sync.WaitGroup
	numGoroutines := 10
	expectedHeight := int64(1000)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(height int64) {
			defer wg.Done()
			sMgr.OnNewBlockHeight(ctx, height)
		}(expectedHeight + int64(i))
	}

	wg.Wait()
	// After concurrent updates, the height should be one of the values we set
	// (the exact value depends on which goroutine finished last)
	finalHeight := sMgr.GetBlockHeight()
	assert.GreaterOrEqual(t, finalHeight, expectedHeight)
	assert.Less(t, finalHeight, expectedHeight+int64(numGoroutines))
}

func TestSequencerManager_GetBlockHeight(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sMgr := newSequencerManagerForTesting(t, mocks)

	// Test initial block height
	assert.Equal(t, int64(0), sMgr.GetBlockHeight())

	// Test after setting block height
	testHeight := int64(42)
	sMgr.OnNewBlockHeight(ctx, testHeight)
	assert.Equal(t, testHeight, sMgr.GetBlockHeight())

	// Test concurrent reads to ensure thread safety
	var wg sync.WaitGroup
	numReaders := 20
	sMgr.OnNewBlockHeight(ctx, int64(500))

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			height := sMgr.GetBlockHeight()
			assert.Equal(t, int64(500), height)
		}()
	}

	wg.Wait()
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
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()

	// Setup expectations for GetTxStatus
	txID := uuid.New()
	expectedStatus := components.PrivateTxStatus{
		TxID:   txID.String(),
		Status: "pending",
	}
	mocks.originator.EXPECT().GetTxStatus(ctx, txID).Return(expectedStatus, nil).Once()

	// Call GetTxStatus
	status, err := sMgr.GetTxStatus(ctx, contractAddr.String(), txID)

	// Verify results
	require.NoError(t, err)
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
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
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
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
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
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()

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
