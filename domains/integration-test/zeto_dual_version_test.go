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
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/pkg/testbed"
	"github.com/LFDT-Paladin/paladin/domains/integration-test/helpers"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zeto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const zetoDualDeployConfigFile = "./zeto/config-for-deploy-anon-nullifier-dual.yaml"

// TestZetoDualVersionSuite runs Zeto_AnonNullifier on two domain instances (v0.2.2 + fungible V1) in one testbed.
func TestZetoDualVersionSuite(t *testing.T) {
	if !helpers.ZetoZKArtifactsRootPresent(helpers.ZetoZKArtifactRootLatest) {
		t.Skipf("ZKP artifacts missing for %s", helpers.ZetoZKArtifactRootLatest)
	}
	if !helpers.ZetoZKArtifactsRootPresent(helpers.ZetoZKArtifactRootV051) {
		t.Skipf("ZKP artifacts missing for %s", helpers.ZetoZKArtifactRootV051)
	}
	suite.Run(t, &zetoDualVersionTestSuite{})
}

type zetoDualVersionEndpoint struct {
	zkpRoot           string
	domainName        string
	domain            zeto.Zeto
	deployedContracts *helpers.ZetoDomainContracts
}

type zetoDualVersionTestSuite struct {
	suite.Suite
	hdWalletSeed *testbed.UTInitFunction
	tb           testbed.Testbed
	rpc          rpcclient.Client
	done         func()

	v0 zetoDualVersionEndpoint
	v1 zetoDualVersionEndpoint
}

func (s *zetoDualVersionTestSuite) SetupSuite() {
	log.SetLevel("debug")
	s.hdWalletSeed = testbed.HDWalletSeedScopedToTest()
	ctx := context.Background()

	s.v0.zkpRoot = helpers.ZetoZKArtifactRootLatest
	s.v1.zkpRoot = helpers.ZetoZKArtifactRootV051
	s.v0.domainName = "zeto_v0_" + pldtypes.RandHex(8)
	s.v1.domainName = "zeto_v1_" + pldtypes.RandHex(8)

	log.L(ctx).Infof("Deploying AnonNullifier contracts (dual config) for %s", s.v0.zkpRoot)
	s.v0.deployedContracts = helpers.DeployZetoContractsDualVariant(
		s.T(), s.hdWalletSeed, zetoDualDeployConfigFile, helpers.ZetoDualDeployVariantV0, controllerName, s.v0.zkpRoot,
	)
	log.L(ctx).Infof("Deploying AnonNullifier contracts (dual config) for %s / fungible V1", s.v1.zkpRoot)
	s.v1.deployedContracts = helpers.DeployZetoContractsDualVariant(
		s.T(), s.hdWalletSeed, zetoDualDeployConfigFile, helpers.ZetoDualDeployVariantV1, controllerName, s.v1.zkpRoot,
	)

	v0Config := helpers.PrepareZetoConfig(s.T(), s.v0.deployedContracts, helpers.ZetoZKArtifactsDir(s.v0.zkpRoot))
	v1Config := helpers.PrepareZetoConfig(s.T(), s.v1.deployedContracts, helpers.ZetoZKArtifactsDir(s.v1.zkpRoot))

	waitV0, tbdV0 := newZetoDomain(s.T(), v0Config, s.v0.deployedContracts.FactoryAddress)
	waitV1, tbdV1 := newZetoDomain(s.T(), v1Config, s.v1.deployedContracts.FactoryAddress)

	done, _, tb, rpc, _ := newTestbed(s.T(), s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.v0.domainName: tbdV0,
		s.v1.domainName: tbdV1,
	})
	s.done = done
	s.tb = tb
	s.rpc = rpc
	s.v0.domain = <-waitV0
	s.v1.domain = <-waitV1

	log.L(ctx).Infof("Dual-version testbed: domain %s (zkp %s), domain %s (zkp %s)",
		s.v0.domainName, s.v0.zkpRoot, s.v1.domainName, s.v1.zkpRoot)
}

func (s *zetoDualVersionTestSuite) TearDownSuite() {
	s.done()
}

func (s *zetoDualVersionTestSuite) Test_AnonNullifier_V0() {
	s.testAnonNullifierSmoke(s.v0, helpers.DeployZetoFungible)
}

func (s *zetoDualVersionTestSuite) Test_AnonNullifier_V1() {
	s.testAnonNullifierSmoke(s.v1, helpers.DeployZetoFungibleV1)
}

type zetoFungibleDeployFunc func(context.Context, *testing.T, rpcclient.Client, string, string, string, string) *helpers.ZetoHelperFungible

func (s *zetoDualVersionTestSuite) testAnonNullifierSmoke(ep zetoDualVersionEndpoint, deploy zetoFungibleDeployFunc) {
	ctx := context.Background()
	t := s.T()
	tokenName := constants.TOKEN_ANON_NULLIFIER

	s.storeTokenABI(t, ctx, ep, tokenName)

	zeto := deploy(ctx, t, s.rpc, ep.domainName, controllerName, tokenName, ep.zkpRoot)
	zetoAddress := zeto.Address
	log.L(ctx).Infof("[%s] Zeto_AnonNullifier deployed to %s", ep.domainName, zetoAddress)

	var controllerAddr pldtypes.Bytes32
	rpcerr := s.rpc.CallRPC(ctx, &controllerAddr, "ptx_resolveVerifier", controllerName,
		zetosignerapi.AlgoDomainZetoSnarkBJJ(ep.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	log.L(ctx).Infof("[%s] Mint two UTXOs (10, 20)", ep.domainName)
	zeto.Mint(ctx, controllerName, []uint64{10, 20}).SignAndSend(controllerName, true).Wait()

	jq := query.NewQueryBuilder().Limit(100).Equal("locked", false).Query()
	methodName := zetoUnlockedCoinQueryMethod(true)

	coins := findAvailableCoins(t, ctx, s.rpc, ep.domain.Name(), ep.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		return len(coins) >= 2
	})
	require.Len(t, coins, 2)
	assert.Equal(t, int64(10), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[0].Data.Owner.String())
	assert.Equal(t, int64(20), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())

	balanceOfResult := zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "30", balanceOfResult["totalBalance"].(string))

	amount := uint64(25)
	log.L(ctx).Infof("[%s] Transfer %d to recipient1", ep.domainName, amount)
	zeto.Transfer(ctx, []string{recipient1Name}, []uint64{amount}).SignAndSend(controllerName, true).Wait()

	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "5", balanceOfResult["totalBalance"].(string))

	coins = findAvailableCoins(t, ctx, s.rpc, ep.domain.Name(), ep.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		return len(coins) >= 2
	})
	require.Len(t, coins, 2)
	assert.Equal(t, int64(25), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, int64(5), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())
}

func (s *zetoDualVersionTestSuite) storeTokenABI(t *testing.T, ctx context.Context, ep zetoDualVersionEndpoint, tokenName string) {
	var result pldtypes.HexBytes
	contractAbi, ok := ep.deployedContracts.DeployedContractAbis[tokenName]
	require.True(t, ok, "Missing ABI for contract %s", tokenName)
	rpcerr := s.rpc.CallRPC(ctx, &result, "ptx_storeABI", contractAbi)
	if rpcerr != nil {
		require.NoError(t, rpcerr.RPCError())
	}
}
