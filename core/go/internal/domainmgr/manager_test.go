/*
 * Copyright © 2024 Kaleido, Inc.
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

package domainmgr

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/statemgr"
	"github.com/LFDT-Paladin/paladin/core/mocks/blockindexermocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/ethclientmocks"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	db               sqlmock.Sqlmock
	c                *componentsmocks.AllComponents
	ethClient        *ethclientmocks.EthClient
	ethClientFactory *ethclientmocks.EthClientFactory
	stateStore       *componentsmocks.StateManager
	blockIndexer     *blockindexermocks.BlockIndexer
	keyManager       *componentsmocks.KeyManager
	txManager        *componentsmocks.TXManager
	sequencerManager *componentsmocks.SequencerManager
	transportMgr     *componentsmocks.TransportManager
	publicTxManager  *componentsmocks.PublicTxManager
}

func newTestDomainManager(t *testing.T, realDB bool, conf *pldconf.DomainManagerInlineConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *domainManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	allComponents := componentsmocks.NewAllComponents(t)
	mc := &mockComponents{
		c:                allComponents,
		blockIndexer:     blockindexermocks.NewBlockIndexer(t),
		stateStore:       componentsmocks.NewStateManager(t),
		ethClientFactory: ethclientmocks.NewEthClientFactory(t),
		keyManager:       componentsmocks.NewKeyManager(t),
		txManager:        componentsmocks.NewTXManager(t),
		sequencerManager: componentsmocks.NewSequencerManager(t),
		transportMgr:     componentsmocks.NewTransportManager(t),
		publicTxManager:  componentsmocks.NewPublicTxManager(t),
	}

	// Blockchain stuff is always mocked
	allComponents.On("EthClientFactory").Return(mc.ethClientFactory)
	mc.ethClientFactory.On("ChainID").Return(int64(12345)).Maybe()
	mc.ethClientFactory.On("HTTPClient").Return(mc.ethClient).Maybe()
	mc.ethClientFactory.On("WSClient").Return(mc.ethClient).Maybe()
	allComponents.On("BlockIndexer").Return(mc.blockIndexer)
	mc.keyManager.On("AddInMemorySigner", "domain", mock.Anything).Return().Maybe()
	allComponents.On("KeyManager").Return(mc.keyManager)
	allComponents.On("TxManager").Return(mc.txManager)
	allComponents.On("SequencerManager").Return(mc.sequencerManager)
	allComponents.On("TransportManager").Return(mc.transportMgr)
	allComponents.On("PublicTxManager").Return(mc.publicTxManager)
	mc.transportMgr.On("LocalNodeName").Return("node1").Maybe()

	var p persistence.Persistence
	var err error
	var realStateManager components.StateManager
	var pDone func()
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx, "domainmgr")
		require.NoError(t, err)
		realStateManager = statemgr.NewStateManager(ctx, &pldconf.StateStoreConfig{}, p)
		allComponents.On("StateManager").Return(realStateManager)
		_, _ = realStateManager.PreInit(allComponents)
	} else {
		mp, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		p = mp.P
		mc.db = mp.Mock
		pDone = func() {
			require.NoError(t, mp.Mock.ExpectationsWereMet())
		}
		allComponents.On("StateManager").Return(mc.stateStore)
	}
	allComponents.On("Persistence").Return(p)

	for _, fn := range extraSetup {
		fn(mc)
	}

	dm := NewDomainManager(ctx, conf)

	_, err = dm.PreInit(allComponents)
	require.NoError(t, err)
	err = dm.PostInit(allComponents)
	require.NoError(t, err)

	if realDB {
		allComponents.On("DomainManager").Return(dm)
		_ = realStateManager.PostInit(allComponents)
		_ = realStateManager.Start()
	}

	err = dm.Start()
	require.NoError(t, err)

	return ctx, dm.(*domainManager), mc, func() {
		cancelCtx()
		pDone()
		dm.Stop()
	}
}

func TestConfiguredDomains(t *testing.T) {
	_, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				Plugin: pldconf.PluginConfig{
					Type:    string(pldtypes.LibraryTypeCShared),
					Library: "some/where",
				},
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	assert.Equal(t, map[string]*pldconf.PluginConfig{
		"test1": {
			Type:    string(pldtypes.LibraryTypeCShared),
			Library: "some/where",
		},
	}, dm.ConfiguredDomains())
}

func TestDomainRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	_, err := dm.DomainRegistered("unknown", nil)
	assert.Regexp(t, "PD011600", err)
}

func TestDomainMissingRegistryAddress(t *testing.T) {
	config := &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				Plugin: pldconf.PluginConfig{
					Type:    string(pldtypes.LibraryTypeCShared),
					Library: "some/where",
				},
			},
		},
	}

	mc := &mockComponents{
		blockIndexer:     blockindexermocks.NewBlockIndexer(t),
		stateStore:       componentsmocks.NewStateManager(t),
		ethClientFactory: ethclientmocks.NewEthClientFactory(t),
		keyManager:       componentsmocks.NewKeyManager(t),
		txManager:        componentsmocks.NewTXManager(t),
		sequencerManager: componentsmocks.NewSequencerManager(t),
		transportMgr:     componentsmocks.NewTransportManager(t),
	}
	componentsmocks := componentsmocks.NewAllComponents(t)
	componentsmocks.On("EthClientFactory").Return(mc.ethClientFactory)
	mc.ethClientFactory.On("ChainID").Return(int64(12345)).Maybe()
	mc.ethClientFactory.On("HTTPClient").Return(mc.ethClient).Maybe()
	mc.ethClientFactory.On("WSClient").Return(mc.ethClient).Maybe()
	componentsmocks.On("BlockIndexer").Return(mc.blockIndexer)
	mc.keyManager.On("AddInMemorySigner", "domain", mock.Anything).Return().Maybe()
	componentsmocks.On("KeyManager").Return(mc.keyManager)
	componentsmocks.On("TxManager").Return(mc.txManager)
	componentsmocks.On("SequencerManager").Return(mc.sequencerManager)
	componentsmocks.On("TransportManager").Return(mc.transportMgr)
	componentsmocks.On("PublicTxManager").Return(mc.publicTxManager)

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	componentsmocks.On("StateManager").Return(mc.stateStore)
	componentsmocks.On("Persistence").Return(mp.P)
	dm := NewDomainManager(context.Background(), config)
	_, err = dm.PreInit(componentsmocks)
	require.NoError(t, err)
	err = dm.PostInit(componentsmocks)
	assert.Regexp(t, "PD011606", err)
}

func TestGetDomainNotFound(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	_, err := dm.GetDomainByName(ctx, "wrong")
	assert.Regexp(t, "PD011600", err)

	_, err = dm.getDomainByAddress(ctx, pldtypes.MustEthAddress(pldtypes.RandHex(20)))
	assert.Regexp(t, "PD011600", err)

	dc := dm.getDomainByAddressOrNil(pldtypes.MustEthAddress(pldtypes.RandHex(20)))
	assert.Nil(t, dc)
}

func TestGetDomainNotInit(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{
		AbiStateSchemasJson: []string{`{!!! bad`},
	})
	defer done()

	_, err := td.dm.GetDomainByName(td.ctx, td.d.name)
	assert.Regexp(t, "PD011601", err)
}

func TestMustParseLoaders(t *testing.T) {
	assert.Panics(t, func() {
		_ = mustParseEmbeddedBuildABI([]byte(`{!wrong`))
	})
	assert.Panics(t, func() {
		_ = mustParseEventSoliditySignature(abi.ABI{}, "nope")
	})
	assert.Panics(t, func() {
		_ = mustParseEventSignatureHash(abi.ABI{}, "nope")
	})
	badEvent := &abi.Entry{
		Type:   abi.Event,
		Name:   "broken",
		Inputs: abi.ParameterArray{{Type: "wrong"}},
	}
	assert.Panics(t, func() {
		_ = mustParseEventSoliditySignature(abi.ABI{badEvent}, "broken")
	})
	assert.Panics(t, func() {
		_ = mustParseEventSignatureHash(abi.ABI{badEvent}, "broken")
	})
}

func TestWaitForDeployDomainNotFound(t *testing.T) {
	reqID := uuid.New()
	domainAddr := pldtypes.RandAddress()
	contractAddr := pldtypes.RandAddress()

	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	}, func(mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows([]string{
			"deploy_tx", "domain_address", "address", "config_bytes",
		}).AddRow(
			reqID, domainAddr, contractAddr, "",
		))
	})
	defer done()

	received := make(chan struct{})
	go func() {
		_, err := dm.ExecDeployAndWait(ctx, reqID, func() error {
			// We simulate this on the main test routine below
			return nil
		})
		assert.Regexp(t, "PD011654", err)
		close(received)
	}()

	for dm.privateTxWaiter.InFlightCount() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	dm.privateTxWaiter.GetInflight(reqID).Complete(&components.ReceiptInput{
		ContractAddress: contractAddr,
	})

	<-received

}

func TestWaitForDeployNotADeploy(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	reqID := uuid.New()

	received := make(chan struct{})
	go func() {
		_, err := dm.ExecDeployAndWait(ctx, reqID, func() error {
			// We simulate this on the main test routine below
			return nil
		})
		assert.Regexp(t, "PD011648", err)
		close(received)
	}()

	for dm.privateTxWaiter.InFlightCount() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	dm.privateTxWaiter.GetInflight(reqID).Complete(&components.ReceiptInput{
		ContractAddress: nil, // we complete without a contract address
	})

	<-received

}

func TestWaitForDeployTimeout(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := dm.waitForDeploy(cancelled, dm.privateTxWaiter.AddInflight(cancelled, uuid.New()))
	assert.Regexp(t, "PD020100", err)
}

func TestWaitForTransactionTimeout(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	err := dm.ExecAndWaitTransaction(cancelled, uuid.New(), func() error { return nil })
	assert.Regexp(t, "PD020100", err)
}

func TestQuerySmartContractsLimitNotSet(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	jq := &query.QueryJSON{}
	_, err := dm.querySmartContracts(ctx, jq)
	assert.Regexp(t, "PD010721", err)
}

func TestQuerySmartContractsDatabaseError(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	limit := 10
	jq := &query.QueryJSON{
		Limit: &limit,
	}

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnError(fmt.Errorf("database error"))

	_, err := dm.querySmartContracts(ctx, jq)
	assert.Regexp(t, "database error", err)
}

func TestQuerySmartContractsEmptyResults(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	limit := 10
	jq := &query.QueryJSON{
		Limit: &limit,
	}

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(
		sqlmock.NewRows([]string{"deploy_tx", "domain_address", "address", "config_bytes"}),
	)

	results, err := dm.querySmartContracts(ctx, jq)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestQuerySmartContractsDomainNotFound(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	limit := 10
	jq := &query.QueryJSON{
		Limit: &limit,
	}

	contractAddr := pldtypes.RandAddress()
	// Use a different domain address that's not configured
	unknownDomainAddr := pldtypes.RandAddress()

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(
		sqlmock.NewRows([]string{"deploy_tx", "domain_address", "address", "config_bytes"}).
			AddRow(uuid.New(), unknownDomainAddr.String(), contractAddr.String(), []byte{}),
	)

	results, err := dm.querySmartContracts(ctx, jq)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, *contractAddr, results[0].Address)
	require.NotNil(t, results[0].DomainAddress, "DomainAddress should not be nil")
	assert.Equal(t, *unknownDomainAddr, *results[0].DomainAddress)
	assert.Empty(t, results[0].DomainName) // Domain not found, so DomainName should be empty
}

func TestQuerySmartContractsDomainFound(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain manually
	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			return &prototk.ConfigureDomainResponse{
				DomainConfig: goodDomainConf(),
			}, nil
		},
		InitDomain: func(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
			return &prototk.InitDomainResponse{}, nil
		},
		InitContract: func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
			return &prototk.InitContractResponse{
				Valid: true,
				ContractConfig: &prototk.ContractConfig{
					ContractConfigJson:   `{}`,
					CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
					SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
				},
			}, nil
		},
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	// Set up database transaction expectations for domain initialization
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	limit := 10
	jq := &query.QueryJSON{
		Limit: &limit,
	}

	domainAddr := *tp.d.RegistryAddress()
	contractAddr := pldtypes.RandAddress()

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(
		sqlmock.NewRows([]string{"deploy_tx", "domain_address", "address", "config_bytes"}).
			AddRow(uuid.New(), domainAddr.String(), contractAddr.String(), []byte{0xfe, 0xed, 0xbe, 0xef}),
	)

	results, err := dm.querySmartContracts(ctx, jq)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, *contractAddr, results[0].Address)
	assert.Equal(t, domainAddr, *results[0].DomainAddress)
	assert.Equal(t, "test1", results[0].DomainName) // Domain found, so DomainName should be set
}

func TestQuerySmartContractsMultipleResults(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain manually
	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			return &prototk.ConfigureDomainResponse{
				DomainConfig: goodDomainConf(),
			}, nil
		},
		InitDomain: func(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
			return &prototk.InitDomainResponse{}, nil
		},
		InitContract: func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
			return &prototk.InitContractResponse{
				Valid: true,
				ContractConfig: &prototk.ContractConfig{
					ContractConfigJson:   `{}`,
					CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
					SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
				},
			}, nil
		},
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	// Set up database transaction expectations for domain initialization
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	limit := 10
	jq := &query.QueryJSON{
		Limit: &limit,
	}

	domainAddr := *tp.d.RegistryAddress()
	contractAddr1 := pldtypes.RandAddress()
	contractAddr2 := pldtypes.RandAddress()
	unknownDomainAddr := pldtypes.RandAddress()

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(
		sqlmock.NewRows([]string{"deploy_tx", "domain_address", "address", "config_bytes"}).
			AddRow(uuid.New(), domainAddr.String(), contractAddr1.String(), []byte{0xfe, 0xed, 0xbe, 0xef}).
			AddRow(uuid.New(), unknownDomainAddr.String(), contractAddr2.String(), []byte{0xde, 0xad, 0xbe, 0xef}),
	)

	results, err := dm.querySmartContracts(ctx, jq)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// First result: domain found
	assert.Equal(t, *contractAddr1, results[0].Address)
	assert.Equal(t, domainAddr, *results[0].DomainAddress)
	assert.Equal(t, "test1", results[0].DomainName)

	// Second result: domain not found
	assert.Equal(t, *contractAddr2, results[1].Address)
	assert.Equal(t, *unknownDomainAddr, *results[1].DomainAddress)
	assert.Empty(t, results[1].DomainName)
}

func TestQuerySmartContractsEnrichError(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain manually
	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			return &prototk.ConfigureDomainResponse{
				DomainConfig: goodDomainConf(),
			}, nil
		},
		InitDomain: func(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
			return &prototk.InitDomainResponse{}, nil
		},
		InitContract: func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
			return nil, fmt.Errorf("init contract error")
		},
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	// Set up database transaction expectations for domain initialization
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	limit := 10
	jq := &query.QueryJSON{
		Limit: &limit,
	}

	domainAddr := *tp.d.RegistryAddress()
	contractAddr := pldtypes.RandAddress()

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(
		sqlmock.NewRows([]string{"deploy_tx", "domain_address", "address", "config_bytes"}).
			AddRow(uuid.New(), domainAddr.String(), contractAddr.String(), []byte{0xfe, 0xed, 0xbe, 0xef}),
	)

	_, err := dm.querySmartContracts(ctx, jq)
	assert.Regexp(t, "init contract error", err)
}
