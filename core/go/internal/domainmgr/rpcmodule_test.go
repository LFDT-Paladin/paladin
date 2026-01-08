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
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
)

func TestRPCModuleBuild(t *testing.T) {
	_, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerInlineConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	assert.NotNil(t, dm.rpcModule)

	// Verify RPC handlers are registered
	assert.NotNil(t, dm.rpcQueryTransactions())
	assert.NotNil(t, dm.rpcGetDomain())
	assert.NotNil(t, dm.rpcGetDomainByAddress())
	assert.NotNil(t, dm.rpcQuerySmartContracts())
	assert.NotNil(t, dm.rpcGetSmartContractByAddress())
}
