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
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/domains/integration-test/helpers"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TestFungibleZetoV1Suite mirrors zeto_fungible_test.go for the fungible V1 axis (ZetoFactoryV1, IZetoFungible_V1,
// createLock instead of legacy lock). Locking runs for every fungible token implementation. The v0.2.2 suite keeps
// transferLocked/delegateLock coverage on TOKEN_ANON only; V1 pools do not expose those legacy entrypoints on-chain.
//
// Subtest label "v0.5.0" matches TestFungibleZetoSuite: t.Run(zkpArtifactRoot) where root is domains/zeto/zkp/<tag>
// (see helpers.ZetoZKArtifactRootV050 and ZetoFungibleV1ZKArtifactRootsForTestRun in helpers/zeto_zkp_versions.go).
func TestFungibleZetoV1Suite(t *testing.T) {
	for _, root := range helpers.ZetoFungibleV1ZKArtifactRootsForTestRun() {
		t.Run(root, func(t *testing.T) {
			if !helpers.ZetoZKArtifactsRootPresent(root) {
				t.Skipf("ZKP artifacts missing for %s (extract with Gradle :domains:zeto:extractZetoZkpVariants)", root)
			}
			suite.Run(t, &fungibleV1TestSuiteHelper{
				zetoDomainTestSuite: zetoDomainTestSuite{
					zkpArtifactRoot: root,
					contractsFile:   "./zeto/config-for-deploy-fungible-v1.yaml",
				},
			})
		})
	}
}

type fungibleV1TestSuiteHelper struct {
	zetoDomainTestSuite
}

func (s *fungibleV1TestSuiteHelper) TestV1_Zeto_Anon() {
	s.testZetoV1(s.T(), constants.TOKEN_ANON, false, false)
}

func (s *fungibleV1TestSuiteHelper) TestV1_Zeto_AnonBatch() {
	s.testZetoV1(s.T(), constants.TOKEN_ANON, true, false)
}

func (s *fungibleV1TestSuiteHelper) TestV1_Zeto_AnonEnc() {
	s.testZetoV1(s.T(), constants.TOKEN_ANON_ENC, false, false)
}

func (s *fungibleV1TestSuiteHelper) TestV1_Zeto_AnonEncBatch() {
	s.testZetoV1(s.T(), constants.TOKEN_ANON_ENC, true, false)
}

func (s *fungibleV1TestSuiteHelper) TestV1_Zeto_AnonNullifier() {
	s.testZetoV1(s.T(), constants.TOKEN_ANON_NULLIFIER, false, true)
}

func (s *fungibleV1TestSuiteHelper) TestV1_Zeto_AnonNullifierBatch() {
	s.testZetoV1(s.T(), constants.TOKEN_ANON_NULLIFIER, true, true)
}

func (s *fungibleV1TestSuiteHelper) TestV1_Zeto_AnonNullifierKyc() {
	s.testZetoV1(s.T(), constants.TOKEN_ANON_NULLIFIER_KYC, false, true, true)
}

func (s *fungibleV1TestSuiteHelper) TestV1_Zeto_AnonNullifierKycBatch() {
	s.testZetoV1(s.T(), constants.TOKEN_ANON_NULLIFIER_KYC, true, true, true)
}

func (s *fungibleV1TestSuiteHelper) testZetoV1(t *testing.T, tokenName string, useBatch bool, isNullifiersToken bool, isKycToken ...bool) {
	ctx := context.Background()
	zkpRoot := s.zkpArtifactRoot
	if zkpRoot == "" {
		zkpRoot = helpers.EffectiveZetoZKArtifactRoot()
	}
	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("fungible V1 (zkp %s) — deploying an instance of the %s token", zkpRoot, tokenName)
	log.L(ctx).Info("*************************************")
	s.setupContractsAbi(t, ctx, tokenName)

	zeto := helpers.DeployZetoFungibleV1(ctx, t, s.rpc, s.domainName, controllerName, tokenName, zkpRoot)
	zetoAddress := zeto.Address
	log.L(ctx).Infof("Zeto instance deployed to %s", zetoAddress.String())

	var controllerEthAddr string
	rpcerr := s.rpc.CallRPC(ctx, &controllerEthAddr, "ptx_resolveVerifier", controllerName, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)

	log.L(ctx).Infof("Deploying the sample ERC20 with initialOwner %s", controllerEthAddr)
	erc20Address, err := helpers.DeployERC20(ctx, s.rpc, controllerName, controllerEthAddr)
	require.NoError(t, err)

	log.L(ctx).Infof("Setting the ERC20 contract (%s) to the Zeto instance", erc20Address)
	zeto.SetERC20(ctx, s.tb, controllerName, erc20Address)

	var controllerAddr pldtypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &controllerAddr, "ptx_resolveVerifier", controllerName, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	if len(isKycToken) > 0 && isKycToken[0] {
		log.L(ctx).Infof("Registering participant %s in the KYC registry (pubKey=%s)", controllerName, controllerAddr.String())
		pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(controllerAddr.HexString())
		require.NoError(t, err)
		zeto.Register(ctx, s.tb, controllerName, []*big.Int{pubKey.X, pubKey.Y})

		var recipientAddr pldtypes.Bytes32
		rpcerr = s.rpc.CallRPC(ctx, &recipientAddr, "ptx_resolveVerifier", recipient1Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
		require.Nil(t, rpcerr)
		log.L(ctx).Infof("Registering participant %s in the KYC registry", recipient1Name)
		pubKey, err = zetosigner.DecodeBabyJubJubPublicKey(recipientAddr.HexString())
		require.NoError(t, err)
		zeto.Register(ctx, s.tb, controllerName, []*big.Int{pubKey.X, pubKey.Y})

		rpcerr = s.rpc.CallRPC(ctx, &recipientAddr, "ptx_resolveVerifier", recipient2Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
		require.Nil(t, rpcerr)
		log.L(ctx).Infof("Registering participant %s in the KYC registry", recipient2Name)
		pubKey, err = zetosigner.DecodeBabyJubJubPublicKey(recipientAddr.HexString())
		require.NoError(t, err)
		zeto.Register(ctx, s.tb, controllerName, []*big.Int{pubKey.X, pubKey.Y})

		rpcerr = s.rpc.CallRPC(ctx, &recipientAddr, "ptx_resolveVerifier", recipient3Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
		require.Nil(t, rpcerr)
		log.L(ctx).Infof("Registering participant %s in the KYC registry", recipient3Name)
		pubKey, err = zetosigner.DecodeBabyJubJubPublicKey(recipientAddr.HexString())
		require.NoError(t, err)
		zeto.Register(ctx, s.tb, controllerName, []*big.Int{pubKey.X, pubKey.Y})

		time.Sleep(5 * time.Second) // wait for the KYC registry to be updated
	}

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Mint two UTXOs (10, 20) from controller to controller")
	log.L(ctx).Info("*************************************")
	zeto.Mint(ctx, controllerName, []uint64{10, 20}).SignAndSend(controllerName, true).Wait()

	jq := query.NewQueryBuilder().Limit(100).Equal("locked", false).Query()
	methodName := zetoUnlockedCoinQueryMethod(isNullifiersToken)

	coins := findAvailableCoins(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		return len(coins) >= 2
	})
	require.Len(t, coins, 2)
	assert.Equal(t, int64(10), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[0].Data.Owner.String())
	assert.Equal(t, int64(20), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())

	balanceOfResult := zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "30", balanceOfResult["totalBalance"].(string), "Balance of controller should be 30")
	if useBatch {
		log.L(ctx).Info("*************************************")
		log.L(ctx).Infof("Mint 30 from controller to controller")
		log.L(ctx).Info("*************************************")
		zeto.Mint(ctx, controllerName, []uint64{30}).SignAndSend(controllerName, true).Wait()
		balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
		assert.Equal(t, "60", balanceOfResult["totalBalance"].(string), "Balance of controller should be 60")
	}

	if useBatch {
		amount1 := 15
		amount2 := 40
		log.L(ctx).Info("*************************************")
		log.L(ctx).Infof("Transfer %d from controller to recipient1 (%d) and recipient2 (%d)", amount1+amount2, amount1, amount2)
		log.L(ctx).Info("*************************************")
		zeto.Transfer(ctx, []string{recipient1Name, recipient2Name}, []uint64{uint64(amount1), uint64(amount2)}).SignAndSend(controllerName, true).Wait()
	} else {
		amount := 25
		log.L(ctx).Info("*************************************")
		log.L(ctx).Infof("Transfer %d from controller to recipient1", amount)
		log.L(ctx).Info("*************************************")
		zeto.Transfer(ctx, []string{recipient1Name}, []uint64{uint64(amount)}).SignAndSend(controllerName, true).Wait()
	}
	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "5", balanceOfResult["totalBalance"].(string), "Balance of controller should be 5")

	expectedCoins := 2
	if useBatch {
		expectedCoins = 3
	}
	coins = findAvailableCoins(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		if len(coins) >= expectedCoins {
			if useBatch {
				return coins[0].Data.Amount.Int().Text(10) == "15" && coins[1].Data.Amount.Int().Text(10) == "40" && coins[2].Data.Amount.Int().Text(10) == "5"
			}
			return coins[0].Data.Amount.Int().Text(10) == "25" && coins[1].Data.Amount.Int().Text(10) == "5"
		}
		return false
	})
	if len(coins) != expectedCoins {
		for i, coin := range coins {
			fmt.Printf("==> Coin %d: %+v\n", i, coin)
		}
	}
	require.Len(t, coins, expectedCoins)

	if useBatch {
		assert.Equal(t, int64(15), coins[0].Data.Amount.Int().Int64())
		assert.Equal(t, int64(40), coins[1].Data.Amount.Int().Int64())
		assert.Equal(t, int64(5), coins[2].Data.Amount.Int().Int64())
		assert.Equal(t, controllerAddr.String(), coins[2].Data.Owner.String())
	} else {
		assert.Equal(t, int64(25), coins[0].Data.Amount.Int().Int64())
		assert.Equal(t, int64(5), coins[1].Data.Amount.Int().Int64())
		assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())
	}

	log.L(ctx).Infof("Mint 100 in ERC20 to controller")
	zeto.MintERC20(ctx, s.tb, *erc20Address, 100, controllerName, controllerEthAddr)

	log.L(ctx).Infof("Approve Zeto (%s) to spend from the controller account (%s)", zetoAddress.String(), controllerEthAddr)
	zeto.ApproveERC20(ctx, s.tb, *erc20Address, 100, controllerName)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Deposit from ERC20 balance to Zeto")
	log.L(ctx).Info("*************************************")
	zeto.Deposit(ctx, 100).SignAndSend(controllerName, true).Wait()

	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "105", balanceOfResult["totalBalance"].(string), "Balance of controller should be 105")

	expectedCoins += 2
	coins = findAvailableCoins(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		return len(coins) >= expectedCoins
	})
	require.Len(t, coins, expectedCoins)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Withdraw back to ERC20 balance from Zeto")
	log.L(ctx).Info("*************************************")
	zeto.Withdraw(ctx, 100).SignAndSend(controllerName, true).Wait()

	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "5", balanceOfResult["totalBalance"].(string), "Balance of controller should be 5")

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("createLock (IZetoFungible_V1): lock value 1 twice with delegate recipient1")
	log.L(ctx).Info("*************************************")
	// Batch mode transfers 15 to recipient1 and 40 to recipient2; non-batch sends 25 to recipient1 only.
	var recipient1TransferAmount int64 = 25
	expectedCoinsAfterLock1 := 3
	expectedCoinsAfterLock2 := 4
	expectedCoinsAfterSpendLock := 4
	if useBatch {
		recipient1TransferAmount = 15
		expectedCoinsAfterLock1 = 4     // recipient1: 15; recipient2: 40; controller: 4 + locked 1
		expectedCoinsAfterLock2 = 5     // recipient1: 15; recipient2: 40; controller: 3 + locked 1 + locked 1
		expectedCoinsAfterSpendLock = 5 // recipient1: 15, 1; recipient2: 40; controller: 3 + locked 1 (then 3 only after lock1 spent)
	}
	var recipient1EthAddrStr string
	rpcerr = s.rpc.CallRPC(ctx, &recipient1EthAddrStr, "ptx_resolveVerifier", recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)

	waitLock1 := zeto.CreateLock(ctx, controllerName, recipient1Name, 1, nil).SignAndSend(controllerName, true).Wait()
	tr1 := helpers.DecodeTransactionInvokeResult(t, waitLock1)
	require.NotNil(t, tr1.PreparedTransaction)
	require.NotEmpty(t, tr1.PreparedTransaction.From)
	chainLockID1, calculatedLockID1 := helpers.ZetoLockIDsFromCreateLockReceipt(ctx, t, s.rpc, s.tb, &zeto.ZetoHelper, tr1)
	assert.Equal(t, calculatedLockID1, chainLockID1, "createLock #1: on-chain lockId must match keccak256(pool, publicCreateLockMsgSender, txId)")
	lockID1 := chainLockID1

	var recipient1Addr pldtypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &recipient1Addr, "ptx_resolveVerifier", recipient1Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	// inspect all the coins after lock1 createLock
	// non-batch: recipient1 {25}; controller {4}, {1 locked}
	// batch: recipient1 {15}; recipient2 {40}; controller {4}, {1 locked}
	count := 0
	var recipient1Amounts1 []int64
	var recipient1Locked1 []bool
	var controllerAmounts1 []int64
	var controllerLocked1 []bool
	coinsAfterLock1 := findAvailableZetoCoinsUnlockedAndLocked(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), isNullifiersToken, zetoAddress, 100, func(coins []*types.ZetoCoinState) bool {
		count++
		time.Sleep(1 * time.Second)
		log.L(ctx).Infof("\n==> Count for lock1 createLock: %d\n", len(coins))
		for i, coin := range coins {
			log.L(ctx).Infof("==> Coin %d: %+v\n", i, coin)
		}
		recipient1Amounts1 = []int64{}
		recipient1Locked1 = []bool{}
		controllerAmounts1 = []int64{}
		controllerLocked1 = []bool{}
		for _, coin := range coins {
			owner := coin.Data.Owner.String()
			amount := coin.Data.Amount.Int().Int64()
			switch owner {
			case recipient1Addr.String():
				recipient1Amounts1 = append(recipient1Amounts1, amount)
				recipient1Locked1 = append(recipient1Locked1, coin.Data.Locked)
			case controllerAddr.String():
				controllerAmounts1 = append(controllerAmounts1, amount)
				controllerLocked1 = append(controllerLocked1, coin.Data.Locked)
			}
		}
		return (len(coins) == expectedCoinsAfterLock1 && len(recipient1Amounts1) == 1 && len(controllerAmounts1) == 2) || count >= 5
	})
	require.Len(t, coinsAfterLock1, expectedCoinsAfterLock1)
	assert.ElementsMatch(t, []int64{recipient1TransferAmount}, recipient1Amounts1, "recipient1 coins after createLock #1")
	assert.ElementsMatch(t, []bool{false}, recipient1Locked1, "recipient1 coins after createLock #1")
	assert.ElementsMatch(t, []int64{4, 1}, controllerAmounts1, "controller coins after createLock #1 (remainder 4 + locked 1)")
	assert.ElementsMatch(t, []bool{false, true}, controllerLocked1, "controller coins after createLock #1 (remainder 4 + locked 1)")

	waitLock2 := zeto.CreateLock(ctx, controllerName, recipient1Name, 1, nil).SignAndSend(controllerName, true).Wait()
	tr2 := helpers.DecodeTransactionInvokeResult(t, waitLock2)
	require.NotNil(t, tr2.PreparedTransaction)
	require.NotEmpty(t, tr2.PreparedTransaction.From)
	chainLockID2, calculatedLockID2 := helpers.ZetoLockIDsFromCreateLockReceipt(ctx, t, s.rpc, s.tb, &zeto.ZetoHelper, tr2)
	assert.Equal(t, calculatedLockID2, chainLockID2, "createLock #2: on-chain lockId must match keccak256(pool, publicCreateLockMsgSender, txId)")
	lockID2 := chainLockID2
	// inspect all the coins after lock2 createLock
	// non-batch: recipient1 {25}; controller {3}, {1 locked}, {1 locked}
	// batch: recipient1 {15}; recipient2 {40}; controller {3}, {1 locked}, {1 locked}
	var recipient1Amounts2 []int64
	var recipient1Locked2 []bool
	var controllerAmounts2 []int64
	var controllerLocked2 []bool
	count = 0
	coinsAfterLock2 := findAvailableZetoCoinsUnlockedAndLocked(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), isNullifiersToken, zetoAddress, 100, func(coins []*types.ZetoCoinState) bool {
		count++
		time.Sleep(1 * time.Second)
		log.L(ctx).Infof("\n==> Count for lock2 createLock: %d\n", len(coins))
		for i, coin := range coins {
			log.L(ctx).Infof("==> Coin %d: %+v\n", i, coin)
		}
		recipient1Amounts2 = []int64{}
		recipient1Locked2 = []bool{}
		controllerAmounts2 = []int64{}
		controllerLocked2 = []bool{}
		for _, coin := range coins {
			owner := coin.Data.Owner.String()
			amount := coin.Data.Amount.Int().Int64()
			switch owner {
			case recipient1Addr.String():
				recipient1Amounts2 = append(recipient1Amounts2, amount)
				recipient1Locked2 = append(recipient1Locked2, coin.Data.Locked)
			case controllerAddr.String():
				controllerAmounts2 = append(controllerAmounts2, amount)
				controllerLocked2 = append(controllerLocked2, coin.Data.Locked)
			}
		}
		return (len(coins) == expectedCoinsAfterLock2 && len(recipient1Amounts2) == 1 && len(controllerAmounts2) == 3) || count >= 5
	})
	require.Len(t, coinsAfterLock2, expectedCoinsAfterLock2)
	assert.ElementsMatch(t, []int64{recipient1TransferAmount}, recipient1Amounts2, "recipient1 coins after createLock #2")
	assert.ElementsMatch(t, []bool{false}, recipient1Locked2, "recipient1 coins after createLock #2")
	assert.ElementsMatch(t, []int64{3, 1, 1}, controllerAmounts2, "controller coins after createLock #2 (remainder 3 + locked 1 + locked 1)")
	assert.ElementsMatch(t, []bool{false, true, true}, controllerLocked2, "controller coins after createLock #2 (remainder 3 + locked 1 + locked 1)")

	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "3", balanceOfResult["totalBalance"].(string), "Balance of controller should be 3")

	pld := helpers.NewPaladinClient(t, ctx, s.tb)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("spendLock (IZetoFungible_V1): unlock second locked position (public spendLock from lock delegate)")
	log.L(ctx).Info("*************************************")
	// ZetoLockable.createLock sets lock.spender = msg.sender of the public createLock (tr2.PreparedTransaction.From),
	// not the Paladin lock delegate (recipient1). Public spendLock must use that same submitter identity.
	unlockTxResult := zeto.PrepareSpendLock(ctx, s.tb, pld, &helpers.SpendLockRequest{
		From: controllerName, LockId: lockID2,
	}).SignAndSend(controllerName).Wait(5 * time.Second)
	require.NoError(t, unlockTxResult.Error())
	require.NotNil(t, unlockTxResult.Receipt())

	// inspect all the coins after first spendLock (lock2 spent)
	// non-batch: recipient1 {25, 1}; controller {3}, {1 locked}
	// batch: recipient1 {15, 1}; recipient2 {40}; controller {3}, {1 locked}
	count = 0
	var controllerAmounts3 []int64
	var controllerLocked3 []bool
	var recipient1Amounts3 []int64
	var recipient1Locked3 []bool
	coinsAfterSpendUnlock := findAvailableZetoCoinsUnlockedAndLocked(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), isNullifiersToken, zetoAddress, 100, func(coins []*types.ZetoCoinState) bool {
		// sleep for 1 second
		time.Sleep(1 * time.Second)
		count++
		log.L(ctx).Infof("\n==> Count after first spendLock: %d\n", len(coins))
		for i, coin := range coins {
			log.L(ctx).Infof("\t==> Coin %d: %+v\n", i, coin)
		}
		recipient1Amounts3 = []int64{}
		recipient1Locked3 = []bool{}
		controllerAmounts3 = []int64{}
		controllerLocked3 = []bool{}
		for _, coin := range coins {
			owner := coin.Data.Owner.String()
			amount := coin.Data.Amount.Int().Int64()
			switch owner {
			case recipient1Addr.String():
				recipient1Amounts3 = append(recipient1Amounts3, amount)
				recipient1Locked3 = append(recipient1Locked3, coin.Data.Locked)
			case controllerAddr.String():
				controllerAmounts3 = append(controllerAmounts3, amount)
				controllerLocked3 = append(controllerLocked3, coin.Data.Locked)
			}
		}
		return (len(coins) == expectedCoinsAfterSpendLock && len(recipient1Amounts3) == 2 && len(controllerAmounts3) == 2) || count >= 5
	})
	require.Len(t, coinsAfterSpendUnlock, expectedCoinsAfterSpendLock)
	assert.ElementsMatch(t, []int64{3, 1}, controllerAmounts3, "controller coins after first spendLock (remainder 3 + locked 1)")
	assert.ElementsMatch(t, []bool{false, true}, controllerLocked3, "controller coins after first spendLock (remainder 3 + locked 1)")
	assert.ElementsMatch(t, []int64{recipient1TransferAmount, 1}, recipient1Amounts3, "recipient1 coins after first spendLock")
	assert.ElementsMatch(t, []bool{false, false}, recipient1Locked3, "recipient1 coins after first spendLock")

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("delegateLock then spendLock (ILockableCapability): delegate first lock to recipient2, spend using recipients pinned at createLock")
	log.L(ctx).Info("*************************************")
	var recipient2EthAddrStr string
	rpcerr = s.rpc.CallRPC(ctx, &recipient2EthAddrStr, "ptx_resolveVerifier", recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)
	recipient2EthAddr := pldtypes.MustEthAddress(recipient2EthAddrStr)
	// delegateLock requires msg.sender == current lock spender (same public submitter as createLock for lock1).
	zeto.DelegateLockV1(ctx, s.tb, lockID1, recipient2EthAddr, tr1.PreparedTransaction.From)

	spendTxResult := zeto.PrepareSpendLock(ctx, s.tb, pld, &helpers.SpendLockRequest{
		From: controllerName, LockId: lockID1,
	}).SignAndSend(recipient2Name).Wait(5 * time.Second)
	require.NoError(t, spendTxResult.Error())
	require.NotNil(t, spendTxResult.Receipt())

	// inspect all the coins after second spendLock (lock1 spent)
	// non-batch: recipient1 {25, 1, 1}; controller {3}
	// batch: recipient1 {15, 1, 1}; recipient2 {40}; controller {3}
	count = 0
	var controllerAmounts4 []int64
	var controllerLocked4 []bool
	var recipient1Amounts4 []int64
	var recipient1Locked4 []bool
	coinsAfterSpendToRecipient := findAvailableZetoCoinsUnlockedAndLocked(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), isNullifiersToken, zetoAddress, 100, func(coins []*types.ZetoCoinState) bool {
		time.Sleep(1 * time.Second)
		count++
		log.L(ctx).Infof("\n==> Count after second spendLock: %d\n", len(coins))
		for i, coin := range coins {
			log.L(ctx).Infof("\t==> Coin %d: %+v\n", i, coin)
		}
		controllerAmounts4 = []int64{}
		controllerLocked4 = []bool{}
		recipient1Amounts4 = []int64{}
		recipient1Locked4 = []bool{}
		for _, coin := range coins {
			owner := coin.Data.Owner.String()
			amount := coin.Data.Amount.Int().Int64()
			switch owner {
			case controllerAddr.String():
				controllerAmounts4 = append(controllerAmounts4, amount)
				controllerLocked4 = append(controllerLocked4, coin.Data.Locked)
			case recipient1Addr.String():
				recipient1Amounts4 = append(recipient1Amounts4, amount)
				recipient1Locked4 = append(recipient1Locked4, coin.Data.Locked)
			}
		}
		return (len(coins) == expectedCoinsAfterSpendLock && len(controllerAmounts4) == 1 && len(recipient1Amounts4) == 3) || count >= 5
	})
	require.Len(t, coinsAfterSpendToRecipient, expectedCoinsAfterSpendLock)
	assert.ElementsMatch(t, []int64{3}, controllerAmounts4, "controller coins after second spendLock (remainder 3)")
	assert.ElementsMatch(t, []bool{false}, controllerLocked4, "controller coins after second spendLock (remainder 3)")
	assert.ElementsMatch(t, []int64{recipient1TransferAmount, 1, 1}, recipient1Amounts4, "recipient1 coins after second spendLock")
	assert.ElementsMatch(t, []bool{false, false, false}, recipient1Locked4, "recipient1 coins after second spendLock")
}
