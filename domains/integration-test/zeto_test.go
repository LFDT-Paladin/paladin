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

package integrationtest

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/pkg/testbed"
	"github.com/LFDT-Paladin/paladin/domains/integration-test/helpers"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zeto"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/stretchr/testify/suite"
)

var (
	controllerName = "controller"
)

// This is the path to the contracts file
// it should be set by the test runner
var contractsFile string

type zetoDomainTestSuite struct {
	suite.Suite
	hdWalletSeed      *testbed.UTInitFunction
	deployedContracts *helpers.ZetoDomainContracts
	domainName        string
	domain            zeto.Zeto
	rpc               rpcclient.Client
	tb                testbed.Testbed
	done              func()
	// zkpArtifactRoot is domains/zeto/zkp/<tag> (e.g. v0.2.2). Empty uses helpers.EffectiveZetoZKArtifactRoot().
	zkpArtifactRoot string
}

func (s *zetoDomainTestSuite) SetupSuite() {
	log.SetLevel("debug")
	s.hdWalletSeed = testbed.HDWalletSeedScopedToTest()
	ctx := context.Background()
	domainName := "zeto_" + pldtypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)
	zkpRoot := s.zkpArtifactRoot
	if zkpRoot == "" {
		zkpRoot = helpers.EffectiveZetoZKArtifactRoot()
	}
	log.L(ctx).Infof("Zeto ZKP artifact root = %s (%s)", zkpRoot, helpers.ZetoZKArtifactsDir(zkpRoot))
	domainContracts := helpers.DeployZetoContracts(s.T(), s.hdWalletSeed, contractsFile, controllerName, zkpRoot)
	s.deployedContracts = domainContracts
	config := helpers.PrepareZetoConfig(s.T(), s.deployedContracts, helpers.ZetoZKArtifactsDir(zkpRoot))
	waitForZeto, zetoTestbed := newZetoDomain(s.T(), config, domainContracts.FactoryAddress)
	done, _, tb, rpc, _ := newTestbed(s.T(), s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		domainName: zetoTestbed,
	})
	s.domainName = domainName
	s.domain = <-waitForZeto
	s.rpc = rpc
	s.tb = tb
	s.done = done
}

func (s *zetoDomainTestSuite) TearDownSuite() {
	s.done()
}
