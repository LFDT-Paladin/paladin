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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/rpcserver"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRPCModule(t *testing.T) {

	domainConf := goodDomainConf()
	td, done := newTestDomain(t, true, domainConf)
	defer done()
	dm := td.dm
	ctx := td.ctx
	rpc, rpcDone := newTestRPCServer(t, ctx, dm)
	defer rpcDone()

	_, contractAddr := registerTestSmartContract(t, td)

	var domainNames []string
	err := rpc.CallRPC(ctx, &domainNames, "domain_listDomains")
	require.NoError(t, err)
	assert.Equal(t, []string{"test1"}, domainNames)

	var domain1 *pldapi.Domain
	err = rpc.CallRPC(ctx, &domain1, "domain_getDomain", "test1")
	require.NoError(t, err)
	assert.Equal(t, "test1", domain1.Name)

	err = rpc.CallRPC(ctx, &domain1, "domain_getDomain", "test2")
	require.Regexp(t, "PD011600", err)

	err = rpc.CallRPC(ctx, &domain1, "domain_getDomainByAddress", td.d.registryAddress.String())
	require.NoError(t, err)
	assert.Equal(t, "test1", domain1.Name)

	err = rpc.CallRPC(ctx, &domain1, "domain_getDomainByAddress", pldtypes.RandAddress().String())
	require.Regexp(t, "PD011600", err)

	var dscs []*pldapi.DomainSmartContract
	err = rpc.CallRPC(ctx, &dscs, "domain_querySmartContracts", query.NewQueryBuilder().Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, dscs, 1)

	var dsc *pldapi.DomainSmartContract
	err = rpc.CallRPC(ctx, &dsc, "domain_getSmartContractByAddress", contractAddr.String())
	require.NoError(t, err)
	require.Equal(t, dscs[0], dsc)

	err = rpc.CallRPC(ctx, &dsc, "domain_getSmartContractByAddress", pldtypes.RandAddress().String())
	require.Regexp(t, "PD011609", err)
}

func newTestRPCServer(t *testing.T, ctx context.Context, dm *domainManager) (rpcclient.Client, func()) {

	s, err := rpcserver.NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			HTTPServerConfig: pldconf.HTTPServerConfig{Address: confutil.P("127.0.0.1"), Port: confutil.P(0)},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)

	s.Register(dm.RPCModule())

	c := rpcclient.WrapRestyClient(resty.New().SetBaseURL(fmt.Sprintf("http://%s", s.HTTPAddr())))

	return c, s.Stop

}
