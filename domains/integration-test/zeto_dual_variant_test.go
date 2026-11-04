// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

// TestZetoDualVariantSuite runs Zeto_AnonNullifier at V0 and V1 on one domain, whose factory holds both variants'
// implementations.
func TestZetoDualVariantSuite(t *testing.T) {
	for _, v := range []types.ZetoVariant{types.ZetoVariant_V0, types.ZetoVariant_V1} {
		if !helpers.ZetoZKArtifactsPresent(v) {
			t.Skipf("ZKP artifacts missing for %s", v)
		}
	}
	suite.Run(t, &zetoDualVariantTestSuite{})
}

type zetoDualVariantTestSuite struct {
	suite.Suite
	hdWalletSeed      *testbed.UTInitFunction
	tb                testbed.Testbed
	rpc               rpcclient.Client
	done              func()
	domainName        string
	domain            zeto.Zeto
	deployedContracts map[types.ZetoVariant]*helpers.ZetoDomainContracts
}

func (s *zetoDualVariantTestSuite) SetupSuite() {
	log.SetLevel("debug")
	s.hdWalletSeed = testbed.HDWalletSeedScopedToTest()

	s.domainName = "zeto_" + pldtypes.RandHex(8)
	s.deployedContracts = helpers.DeployZetoVariantContracts(s.T(), s.hdWalletSeed, zetoDualDeployConfigFile, controllerName)
	config := helpers.PrepareZetoVariantConfig(s.T(), types.ZetoVariant_V1, s.deployedContracts)
	waitForZeto, tbd := newZetoDomain(s.T(), config, s.deployedContracts[types.ZetoVariant_V1].FactoryAddress)

	done, _, tb, rpc, _ := newTestbed(s.T(), s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: tbd,
	})
	s.done = done
	s.tb = tb
	s.rpc = rpc
	s.domain = <-waitForZeto
}

func (s *zetoDualVariantTestSuite) TearDownSuite() {
	s.done()
}

func (s *zetoDualVariantTestSuite) Test_AnonNullifier_V0() {
	s.testAnonNullifierSmoke(types.ZetoVariant_V0, helpers.DeployZetoFungibleAtVariant)
}

func (s *zetoDualVariantTestSuite) Test_AnonNullifier_V1() {
	s.testAnonNullifierSmoke(types.ZetoVariant_V1, helpers.DeployZetoFungible)
}

type zetoFungibleDeployFunc func(context.Context, *testing.T, rpcclient.Client, string, string, string, types.ZetoVariant) *helpers.ZetoHelperFungible

func (s *zetoDualVariantTestSuite) testAnonNullifierSmoke(variant types.ZetoVariant, deploy zetoFungibleDeployFunc) {
	ctx := context.Background()
	t := s.T()
	tokenName := constants.TOKEN_ANON_NULLIFIER

	s.storeTokenABI(t, ctx, variant, tokenName)

	zeto := deploy(ctx, t, s.rpc, s.domainName, controllerName, tokenName, variant)
	zetoAddress := zeto.Address
	log.L(ctx).Infof("[%s] Zeto_AnonNullifier deployed to %s", variant, zetoAddress)

	var controllerAddr pldtypes.Bytes32
	rpcerr := s.rpc.CallRPC(ctx, &controllerAddr, "ptx_resolveVerifier", controllerName,
		zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	log.L(ctx).Infof("[%s] Mint two UTXOs (10, 20)", variant)
	zeto.Mint(ctx, controllerName, []uint64{10, 20}).SignAndSend(controllerName, true).Wait()

	jq := query.NewQueryBuilder().Limit(100).Equal("locked", false).Query()
	methodName := zetoUnlockedCoinQueryMethod(true)

	coins := findAvailableCoins(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
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
	log.L(ctx).Infof("[%s] Transfer %d to recipient1", variant, amount)
	zeto.Transfer(ctx, []string{recipient1Name}, []uint64{amount}).SignAndSend(controllerName, true).Wait()

	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "5", balanceOfResult["totalBalance"].(string))

	coins = findAvailableCoins(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		return len(coins) >= 2
	})
	require.Len(t, coins, 2)
	assert.Equal(t, int64(25), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, int64(5), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())
}

func (s *zetoDualVariantTestSuite) storeTokenABI(t *testing.T, ctx context.Context, variant types.ZetoVariant, tokenName string) {
	var result pldtypes.HexBytes
	contractAbi, ok := s.deployedContracts[variant].DeployedContractAbis[tokenName]
	require.True(t, ok, "Missing ABI for contract %s", tokenName)
	rpcerr := s.rpc.CallRPC(ctx, &result, "ptx_storeABI", contractAbi)
	if rpcerr != nil {
		require.NoError(t, rpcerr.RPCError())
	}
}
