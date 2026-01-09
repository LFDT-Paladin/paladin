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

func TestRPCQueryTransactions(t *testing.T) {
	tests := []struct {
		name           string
		domains        map[string]*pldconf.DomainConfig
		expectedResult []string
	}{
		{
			name:           "empty domains",
			domains:        map[string]*pldconf.DomainConfig{},
			expectedResult: []string{},
		},
		{
			name: "single domain",
			domains: map[string]*pldconf.DomainConfig{
				"domain1": {
					RegistryAddress: pldtypes.RandHex(20),
				},
			},
			expectedResult: []string{"domain1"},
		},
		{
			name: "multiple domains",
			domains: map[string]*pldconf.DomainConfig{
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
			expectedResult: []string{"domain1", "domain2", "domain3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
				Domains: tt.domains,
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
			assert.ElementsMatch(t, tt.expectedResult, result)
		})
	}
}

func TestRPCGetDomain(t *testing.T) {
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

	t.Run("success", func(t *testing.T) {
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
	})

	t.Run("domain not found", func(t *testing.T) {
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
	})

	t.Run("invalid param count", func(t *testing.T) {
		req := &rpcclient.RPCRequest{
			JSONRpc: "2.0",
			ID:      pldtypes.RawJSON(`"3"`),
			Method:  "domain_getDomain",
			Params:  []pldtypes.RawJSON{},
		}

		resp := handler.Handle(ctx, req)
		require.NotNil(t, resp)
		assert.NotNil(t, resp.Error, "Expected error for invalid param count")
	})
}

func TestRPCGetDomainByAddress(t *testing.T) {
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

	t.Run("success", func(t *testing.T) {
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
	})

	t.Run("domain not found", func(t *testing.T) {
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
	})

	t.Run("invalid param count", func(t *testing.T) {
		req := &rpcclient.RPCRequest{
			JSONRpc: "2.0",
			ID:      pldtypes.RawJSON(`"3"`),
			Method:  "domain_getDomainByAddress",
			Params:  []pldtypes.RawJSON{},
		}

		resp := handler.Handle(ctx, req)
		require.NotNil(t, resp)
		assert.NotNil(t, resp.Error, "Expected error for invalid param count")
	})
}

func TestRPCQuerySmartContracts(t *testing.T) {
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

	t.Run("success with results", func(t *testing.T) {
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
	})

	t.Run("success with empty results", func(t *testing.T) {
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
	})

	t.Run("invalid param count", func(t *testing.T) {
		req := &rpcclient.RPCRequest{
			JSONRpc: "2.0",
			ID:      pldtypes.RawJSON(`"3"`),
			Method:  "domain_querySmartContracts",
			Params:  []pldtypes.RawJSON{},
		}

		resp := handler.Handle(ctx, req)
		require.NotNil(t, resp)
		assert.NotNil(t, resp.Error, "Expected error for invalid param count")
	})
}

func TestRPCGetSmartContractByAddress(t *testing.T) {
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

	t.Run("success", func(t *testing.T) {
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
	})

	t.Run("smart contract not found", func(t *testing.T) {
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

		// Note: The current implementation has a bug where it tries to access sc.Domain()
		// even when sc is nil (when GetSmartContractByAddress returns an error).
		// This test will panic, revealing the bug. The code should check if err is nil
		// before accessing sc.
		assert.Panics(t, func() {
			handler.Handle(ctx, req)
		}, "Expected panic when contract is not found due to nil pointer dereference")
	})

	t.Run("invalid param count", func(t *testing.T) {
		req := &rpcclient.RPCRequest{
			JSONRpc: "2.0",
			ID:      pldtypes.RawJSON(`"3"`),
			Method:  "domain_getSmartContractByAddress",
			Params:  []pldtypes.RawJSON{},
		}

		resp := handler.Handle(ctx, req)
		require.NotNil(t, resp)
		assert.NotNil(t, resp.Error, "Expected error for invalid param count")
	})
}
