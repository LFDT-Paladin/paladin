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

package domainmgr

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRPCQueryTransactions_EmptyDomains(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{},
	})
	defer done()

	handler := dm.rpcQueryTransactions()
	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"1"`),
		Method:  "domain_listDomains",
		Params:  []pldtypes.RawJSON{},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error, "Expected no error")

	var result []string
	err := json.Unmarshal(resp.Result.Bytes(), &result)
	require.NoError(t, err)

	// Check that all expected domains are present (order may vary)
	assert.ElementsMatch(t, []string{}, result)
}

func TestRPCQueryTransactions_SingleDomain(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	handler := dm.rpcQueryTransactions()
	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"1"`),
		Method:  "domain_listDomains",
		Params:  []pldtypes.RawJSON{},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error, "Expected no error")

	var result []string
	err := json.Unmarshal(resp.Result.Bytes(), &result)
	require.NoError(t, err)

	// Check that all expected domains are present (order may vary)
	assert.ElementsMatch(t, []string{"domain1"}, result)
}

func TestRPCQueryTransactions_MultipleDomains(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
			"domain2": {
				RegistryAddress: pldtypes.RandHex(20),
			},
			"domain3": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	handler := dm.rpcQueryTransactions()
	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"1"`),
		Method:  "domain_listDomains",
		Params:  []pldtypes.RawJSON{},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error, "Expected no error")

	var result []string
	err := json.Unmarshal(resp.Result.Bytes(), &result)
	require.NoError(t, err)

	// Check that all expected domains are present (order may vary)
	assert.ElementsMatch(t, []string{"domain1", "domain2", "domain3"}, result)
}

func TestRPCGetDomain_Success(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetDomain()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"1"`),
		Method:  "domain_getDomain",
		Params: []pldtypes.RawJSON{
			pldtypes.JSONString("test1"),
		},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error, "Expected no error")

	var result pldapi.Domain
	err := json.Unmarshal(resp.Result.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "test1", result.Name)
	assert.NotNil(t, result.RegistryAddress)
}

func TestRPCGetDomain_WithSigningAlgorithms(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Create a domain config with SigningAlgorithms
	domainConfig := goodDomainConf()
	domainConfig.SigningAlgorithms = map[string]int32{
		"domain:test1:algo1": 32,
		"domain:test1:algo2": 64,
	}

	// Register the domain
	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			return &prototk.ConfigureDomainResponse{
				DomainConfig: domainConfig,
			}, nil
		},
		InitDomain: func(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
			return &prototk.InitDomainResponse{}, nil
		},
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetDomain()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"1"`),
		Method:  "domain_getDomain",
		Params: []pldtypes.RawJSON{
			pldtypes.JSONString("test1"),
		},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error, "Expected no error")

	var result pldapi.Domain
	err := json.Unmarshal(resp.Result.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "test1", result.Name)
	assert.NotNil(t, result.RegistryAddress)
	require.NotNil(t, result.Config, "Config should be populated")
	assert.Equal(t, domainConfig.SigningAlgorithms, result.Config.SigningAlgorithms, "SigningAlgorithms should match")
}

func TestRPCGetDomain_DomainNotFound(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetDomain()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"2"`),
		Method:  "domain_getDomain",
		Params: []pldtypes.RawJSON{
			pldtypes.JSONString("nonexistent"),
		},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Error, "Expected error for nonexistent domain")
	assert.Regexp(t, "PD011600", resp.Error.Message)
}

func TestRPCGetDomain_InvalidParamCount(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetDomain()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"3"`),
		Method:  "domain_getDomain",
		Params:  []pldtypes.RawJSON{},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Error, "Expected error for invalid param count")
}

func TestRPCGetDomainByAddress_Success(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetDomainByAddress()
	domainAddr := *tp.d.RegistryAddress()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"1"`),
		Method:  "domain_getDomainByAddress",
		Params: []pldtypes.RawJSON{
			pldtypes.JSONString(domainAddr.String()),
		},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error, "Expected no error")

	var result pldapi.Domain
	err := json.Unmarshal(resp.Result.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "test1", result.Name)
	assert.Equal(t, domainAddr, *result.RegistryAddress)
}

func TestRPCGetDomainByAddress_DomainNotFound(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetDomainByAddress()

	unknownAddr := pldtypes.RandAddress()
	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"2"`),
		Method:  "domain_getDomainByAddress",
		Params: []pldtypes.RawJSON{
			pldtypes.JSONString(unknownAddr.String()),
		},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Error, "Expected error for nonexistent domain")
	assert.Regexp(t, "PD011600", resp.Error.Message)
}

func TestRPCGetDomainByAddress_InvalidParamCount(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	}
	mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetDomainByAddress()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"3"`),
		Method:  "domain_getDomainByAddress",
		Params:  []pldtypes.RawJSON{},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Error, "Expected error for invalid param count")
}

func TestRPCQuerySmartContracts_SuccessWithResults(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcQuerySmartContracts()

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

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"1"`),
		Method:  "domain_querySmartContracts",
		Params: []pldtypes.RawJSON{
			pldtypes.JSONString(jq),
		},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error, "Expected no error")

	var result []*pldapi.DomainSmartContract
	err := json.Unmarshal(resp.Result.Bytes(), &result)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, *contractAddr, result[0].Address)
	assert.Equal(t, domainAddr, *result[0].DomainAddress)
	assert.Equal(t, "test1", result[0].DomainName)
}

func TestRPCQuerySmartContracts_SuccessWithEmptyResults(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcQuerySmartContracts()

	limit := 10
	jq := &query.QueryJSON{
		Limit: &limit,
	}

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(
		sqlmock.NewRows([]string{"deploy_tx", "domain_address", "address", "config_bytes"}),
	)

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"2"`),
		Method:  "domain_querySmartContracts",
		Params: []pldtypes.RawJSON{
			pldtypes.JSONString(jq),
		},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error, "Expected no error")

	var result []*pldapi.DomainSmartContract
	err := json.Unmarshal(resp.Result.Bytes(), &result)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestRPCQuerySmartContracts_InvalidParamCount(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcQuerySmartContracts()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"3"`),
		Method:  "domain_querySmartContracts",
		Params:  []pldtypes.RawJSON{},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Error, "Expected error for invalid param count")
}

func TestRPCGetSmartContractByAddress_Success(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetSmartContractByAddress()

	contractAddr := pldtypes.RandAddress()
	domainAddr := *tp.d.RegistryAddress()

	// Mock the database transaction and query
	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(
		sqlmock.NewRows([]string{"deploy_tx", "domain_address", "address", "config_bytes"}).
			AddRow(uuid.New(), domainAddr.String(), contractAddr.String(), []byte{}),
	)
	mc.db.ExpectCommit()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"1"`),
		Method:  "domain_getSmartContractByAddress",
		Params: []pldtypes.RawJSON{
			pldtypes.JSONString(contractAddr.String()),
		},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error, "Expected no error")

	var result pldapi.DomainSmartContract
	err := json.Unmarshal(resp.Result.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, *contractAddr, result.Address)
	assert.Equal(t, domainAddr, *result.DomainAddress)
	assert.Equal(t, "test1", result.DomainName)
}

func TestRPCGetSmartContractByAddress_SmartContractNotFound(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetSmartContractByAddress()

	contractAddr := pldtypes.RandAddress()

	// Mock the database transaction and query returning no rows
	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(
		sqlmock.NewRows([]string{"deploy_tx", "domain_address", "address", "config_bytes"}),
	)
	mc.db.ExpectRollback()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"2"`),
		Method:  "domain_getSmartContractByAddress",
		Params: []pldtypes.RawJSON{
			pldtypes.JSONString(contractAddr.String()),
		},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Error, "Expected error for nonexistent contract")
	assert.Regexp(t, "PD011609", resp.Error.Message)
}

func TestRPCGetSmartContractByAddress_InvalidParamCount(t *testing.T) {
	ctx, dm, mc, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	// Register the domain
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
	mc.db.ExpectBegin()
	mc.db.ExpectCommit()
	registerTestDomain(t, dm, tp)

	handler := dm.rpcGetSmartContractByAddress()

	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"3"`),
		Method:  "domain_getSmartContractByAddress",
		Params:  []pldtypes.RawJSON{},
	}

	resp := handler.Handle(ctx, req)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Error, "Expected error for invalid param count")
}
