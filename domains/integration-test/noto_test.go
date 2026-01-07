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
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/pkg/testbed"
	"github.com/LFDT-Paladin/paladin/domains/integration-test/helpers"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	notaryName = "notary@node1"
)

func TestNotoSuite(t *testing.T) {
	suite.Run(t, new(notoTestSuite))
}

type notoTestSuite struct {
	suite.Suite
	hdWalletSeed   *testbed.UTInitFunction
	domainName     string
	factoryAddress string
}

func (s *notoTestSuite) SetupSuite() {
	ctx := context.Background()
	s.domainName = "noto_" + pldtypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", s.domainName)

	s.hdWalletSeed = testbed.HDWalletSeedScopedToTest()

	log.L(ctx).Infof("Deploying Noto contracts")
	// Deploy: noto (default impl), noto_v0, factory_impl
	contractSource := map[string][]byte{
		"noto":         helpers.NotoJSON,
		"noto_v0":      helpers.NotoV0JSON,
		"factory_impl": helpers.NotoFactoryJSON,
	}

	// After deploying implementations, deploy proxy and register v0
	deployProxyAndConfigure := func(deployed map[string]string, rpc rpcclient.Client) {
		// Deploy factory proxy with initialize(notoImplAddress)
		proxyAddr := deployFactoryProxy(ctx, s.T(), rpc, notaryName,
			deployed["factory_impl"], helpers.NotoFactoryJSON,
			fmt.Sprintf(`["%s"]`, deployed["noto"]))
		deployed["factory"] = proxyAddr
		log.L(ctx).Infof("Factory proxy deployed to %s", proxyAddr)

		// Register noto_v0 implementation on the factory (via proxy)
		result := pldclient.Wrap(rpc).ReceiptPollingInterval(200*time.Millisecond).
			ForABI(ctx, helpers.NotoFactoryABI).
			Public().
			From(notaryName).
			To(pldtypes.MustEthAddress(proxyAddr)).
			Function("registerImplementation").
			Inputs(pldtypes.RawJSON(fmt.Sprintf(`["noto_v0", "%s"]`, deployed["noto_v0"]))).
			BuildTX().
			Send().
			Wait(5 * time.Second)
		require.NoError(s.T(), result.Error())
	}
	contracts := deployContracts(ctx, s.T(), s.hdWalletSeed, notaryName, contractSource, deployProxyAndConfigure)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}
	s.factoryAddress = contracts["factory"]
}

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	require.NoError(t, err)
	return result
}

func (s *notoTestSuite) TestNotoV1() {
	s.testNoto("v1")
}

func (s *notoTestSuite) TestNotoV0() {
	s.testNoto("v0")
}

func (s *notoTestSuite) testNoto(version string) {
	t := s.T()
	ctx := t.Context()
	log.L(ctx).Infof("TestNoto")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, _, _, paladinClient := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()

	notoDomain := <-waitForNoto

	notoReceipts := make(chan notoReceiptWithTXID)
	subscribeAndSendNotoReceiptsToChannel(t, paladinClient, notoDomain.Name(), notoReceipts)

	notaryKey, err := paladinClient.PTX().ResolveVerifier(ctx, notaryName, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient1Key, err := paladinClient.PTX().ResolveVerifier(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := paladinClient.PTX().ResolveVerifier(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	var noto *helpers.NotoHelper
	if version == "v1" {
		noto = helpers.DeployNoto(ctx, t, paladinClient, s.domainName, notary, nil)
	} else {
		noto = helpers.DeployNotoImplementation(ctx, t, paladinClient, s.domainName, "noto_v0", notary, nil)
	}
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Mint 100 from notary to notary")
	rpcerr := paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     notaryName,
				Amount: pldtypes.Int64ToInt256(100),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	mintReceipt := <-notoReceipts
	require.Len(t, mintReceipt.Transfers, 1)
	assert.Equal(t, int64(100), mintReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, notaryKey, mintReceipt.Transfers[0].To.String())

	coins := findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey, coins[0].Data.Owner.String())

	// check balance
	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: notaryName}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of notary should be 100")

	log.L(ctx).Infof("Attempt mint from non-notary (should fail)")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(100),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NotNil(t, rpcerr)
	assert.ErrorContains(t, rpcerr, "PD200009")

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)

	log.L(ctx).Infof("Transfer 150 from notary (should fail)")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(150),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NotNil(t, false)
	assert.ErrorContains(t, rpcerr, "assemble result was REVERT")

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)

	log.L(ctx).Infof("Transfer 50 from notary to recipient1")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	transferReceipt := <-notoReceipts
	require.Len(t, transferReceipt.Transfers, 1)
	assert.Equal(t, int64(50), transferReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient1Key, transferReceipt.Transfers[0].To.String())

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: notaryName}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of notary should be 50")

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 50")

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey, coins[1].Data.Owner.String())

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient2Name,
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	transferReceipt = <-notoReceipts
	require.Len(t, transferReceipt.Transfers, 1)
	assert.Equal(t, int64(50), transferReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient2Key, transferReceipt.Transfers[0].To.String())

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[1].Data.Owner.String())

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 0")

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient2 should be 50")

	log.L(ctx).Infof("Burn 25 from recipient2")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient2Name,
			To:       noto.Address,
			Function: "burn",
			Data: toJSON(t, &types.BurnParams{
				Amount: pldtypes.Int64ToInt256(25),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	burnReceipt := <-notoReceipts
	require.Len(t, burnReceipt.Transfers, 1)
	assert.Equal(t, int64(25), burnReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient2Key, burnReceipt.Transfers[0].From.String())
	assert.Nil(t, burnReceipt.Transfers[0].To)

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "25", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 25")

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey, coins[0].Data.Owner.String())
	assert.Equal(t, int64(25), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[1].Data.Owner.String())
}

func (s *notoTestSuite) TestNotoLockV1() {
	s.testNotoLock("v1")
}

func (s *notoTestSuite) TestNotoLockV0() {
	s.testNotoLock("v0")
}

func (s *notoTestSuite) testNotoLock(version string) {
	t := s.T()
	ctx := t.Context()
	log.L(ctx).Infof("TestNotoLock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, _, _, paladinClient := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()

	notoDomain := <-waitForNoto

	notoReceipts := make(chan notoReceiptWithTXID)
	subscribeAndSendNotoReceiptsToChannel(t, paladinClient, notoDomain.Name(), notoReceipts)

	recipient1Key, err := paladinClient.PTX().ResolveVerifier(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := paladinClient.PTX().ResolveVerifier(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	var noto *helpers.NotoHelper
	if version == "v1" {
		noto = helpers.DeployNoto(ctx, t, paladinClient, s.domainName, notary, nil)
	} else {
		noto = helpers.DeployNotoImplementation(ctx, t, paladinClient, s.domainName, "noto_v0", notary, nil)
	}
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Mint 100 from notary to recipient1")
	rpcerr := paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(100),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	mintReceipt := <-notoReceipts
	require.Len(t, mintReceipt.Transfers, 1)
	assert.Equal(t, int64(100), mintReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient1Key, mintReceipt.Transfers[0].To.String())

	coins := findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Lock 50 from recipient1")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "lock",
			Data: toJSON(t, &types.LockParams{
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	lockReceipt := <-notoReceipts
	require.NotNil(t, lockReceipt.LockInfo)
	require.NotEmpty(t, lockReceipt.LockInfo.LockID)

	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 50")

	lockedCoins := findAvailableCoins[types.NotoLockedCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 1)
	assert.Equal(t, int64(50), lockedCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, lockedCoins[0].Data.Owner.String())
	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2 (succeeds but does not use locked state)")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient2Name,
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	transferReceipt := <-notoReceipts
	require.Len(t, transferReceipt.Transfers, 1)
	assert.Equal(t, int64(50), transferReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient2Key, transferReceipt.Transfers[0].To.String())

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 0")

	lockedCoins = findAvailableCoins[types.NotoLockedCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 1)
	assert.Equal(t, int64(50), lockedCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, lockedCoins[0].Data.Owner.String())
	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Prepare unlock that will send all 50 to recipient2")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "prepareUnlock",
			Data: toJSON(t, &types.UnlockParams{
				LockID: lockReceipt.LockInfo.LockID,
				From:   recipient1Name,
				Recipients: []*types.UnlockRecipient{{
					To:     recipient2Name,
					Amount: pldtypes.Int64ToInt256(50),
				}},
				Data: pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	prepareUnlockReceipt := <-notoReceipts

	require.NotEmpty(t, prepareUnlockReceipt.LockInfo)
	require.NotEmpty(t, prepareUnlockReceipt.LockInfo.UnlockParams)
	require.NotEmpty(t, prepareUnlockReceipt.LockInfo.UnlockParams["txId"])
	require.NotEmpty(t, prepareUnlockReceipt.LockInfo.UnlockTxId)
	assert.Equal(t, prepareUnlockReceipt.LockInfo.UnlockParams["txId"], prepareUnlockReceipt.LockInfo.UnlockTxId.HexString0xPrefix())
	unlockTXID := pldtypes.MustParseBytes32(prepareUnlockReceipt.LockInfo.UnlockTxId.HexString0xPrefix()).UUIDFirst16()

	log.L(ctx).Infof("Delegate lock to recipient2")
	delegateLockParams := &types.DelegateLockParams{
		LockID:   prepareUnlockReceipt.LockInfo.LockID,
		Delegate: pldtypes.MustEthAddress(recipient2Key),
	}
	delegateLockABI := types.NotoABI
	if version == "v0" {
		var unlockParams types.UnlockPublicParams
		err = json.Unmarshal(toJSON(t, prepareUnlockReceipt.LockInfo.UnlockParams), &unlockParams)
		require.NoError(t, err)
		delegateLockParams.Unlock = &unlockParams
		delegateLockABI = types.NotoV0ABI
	}
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "delegateLock",
			Data:     toJSON(t, delegateLockParams),
		},
		ABI: delegateLockABI,
	}, false)
	require.NoError(t, rpcerr)

	delegateLockReceipt := <-notoReceipts
	require.NotEmpty(t, delegateLockReceipt.LockInfo)
	assert.Equal(t, prepareUnlockReceipt.LockInfo.LockID, delegateLockReceipt.LockInfo.LockID)
	assert.Equal(t, recipient2Key, delegateLockReceipt.LockInfo.Delegate.String())

	log.L(ctx).Infof("Unlock from recipient2")
	var notoBuild *solutils.SolidityBuild
	if version == "v1" {
		notoBuild = solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	} else {
		notoBuild = solutils.MustLoadBuild(helpers.NotoV0InterfaceJSON)
	}
	tx := paladinClient.ForABI(ctx, notoBuild.ABI).
		Public().
		From(recipient2Name).
		To(noto.Address).
		Function("unlock").
		Inputs(prepareUnlockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	unlockReceipt := <-notoReceipts
	require.Equal(t, unlockReceipt.txID, unlockTXID)
	require.Len(t, unlockReceipt.Transfers, 1)
	assert.Equal(t, int64(50), unlockReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient2Key, unlockReceipt.Transfers[0].To.String())

	// Wait for locked coins to be consumed and new coins to be created
	findAvailableCoins(t, ctx, paladinClient, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoLockedCoinState) bool {
		return len(coins) == 0
	})
	coins = findAvailableCoins(t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoCoinState) bool {
		return len(coins) == 2
	})

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 100")
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[1].Data.Owner.String())
}

type notoReceiptWithTXID struct {
	types.NotoDomainReceipt
	txID uuid.UUID
}

func subscribeAndSendNotoReceiptsToChannel(t *testing.T, wsClient pldclient.PaladinWSClient, domainName string, receipts chan notoReceiptWithTXID) {
	ctx := t.Context()

	privateType := pldtypes.Enum[pldapi.TransactionType](pldapi.TransactionTypePrivate)
	listenerName := fmt.Sprintf("listener-%s", domainName)
	_, err := wsClient.PTX().CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: listenerName,
		Filters: pldapi.TransactionReceiptFilters{
			Type:   &privateType,
			Domain: domainName,
		},
		Options: pldapi.TransactionReceiptListenerOptions{
			DomainReceipts: true,
		},
	})
	require.NoError(t, err)

	sub, err := wsClient.PTX().SubscribeReceipts(ctx, listenerName)
	require.NoError(t, err)
	go func() {
		// No test assertions in this routine, if there's an error, no receipts are sent and the test will fail
		for {
			select {
			case subNotification, ok := <-sub.Notifications():
				if ok {
					notoReceipts := make([]notoReceiptWithTXID, 0)
					var batch pldapi.TransactionReceiptBatch
					_ = json.Unmarshal(subNotification.GetResult(), &batch)
					for _, r := range batch.Receipts {
						if r.DomainReceipt == nil {
							continue
						}
						var notoReceipt types.NotoDomainReceipt
						err = json.Unmarshal(r.DomainReceipt, &notoReceipt)
						if err == nil {
							notoReceipts = append(notoReceipts, notoReceiptWithTXID{
								NotoDomainReceipt: notoReceipt,
								txID:              r.ID,
							})
						}
					}
					_ = subNotification.Ack(ctx)
					// send after the ack otherwise the main test can complete when it receives the last values and the websocket is closed before the ack
					// can be sent
					for _, n := range notoReceipts {
						receipts <- n
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// TODO: move the new tests to use websockets with assertions on domain receipts
func (s *notoTestSuite) TestNotoCreateMintLock() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoCreateMintLock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, tb, rpc, _ := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	notoDomain := <-waitForNoto

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, notary, nil)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Create mint lock with 100 to recipient1 and 50 to recipient2")
	var invokeResult testbed.TransactionResult
	rpcerr := rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "createMintLock",
			Data: toJSON(t, &types.CreateMintLockParams{
				Recipients: []*types.UnlockRecipient{
					{
						To:     recipient1Name,
						Amount: pldtypes.Int64ToInt256(100),
					},
					{
						To:     recipient2Name,
						Amount: pldtypes.Int64ToInt256(50),
					},
				},
				Data: pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var mintLockReceipt types.NotoDomainReceipt
	err := json.Unmarshal(invokeResult.DomainReceipt, &mintLockReceipt)
	require.NoError(t, err)
	require.NotNil(t, mintLockReceipt.LockInfo)
	require.NotEmpty(t, mintLockReceipt.LockInfo.LockID)

	// Verify no coins exist yet
	coins := findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 0)
	lockedCoins := findAvailableCoins[types.NotoLockedCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 0)

	// Verify balances are 0
	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 0 before unlock")
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient2 should be 0 before unlock")

	// Verify unlock params structure
	require.NotNil(t, mintLockReceipt.LockInfo.UnlockParams)
	unlockParamsMap := mintLockReceipt.LockInfo.UnlockParams
	paramsMap, ok := unlockParamsMap["params"].(map[string]any)
	require.True(t, ok, "params should be a map")
	lockedInputs, ok := paramsMap["lockedInputs"].([]any)
	require.True(t, ok, "lockedInputs should be an array")
	assert.Equal(t, 0, len(lockedInputs), "createMintLock should have no lockedInputs")
	outputs, ok := paramsMap["outputs"].([]any)
	require.True(t, ok, "outputs should be an array")
	assert.Equal(t, 2, len(outputs), "createMintLock should have 2 outputs")

	// Perform the unlock using the prepared unlock params
	log.L(ctx).Infof("Unlock the mint lock")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	tx := pld.ForABI(ctx, notoBuild.ABI).
		Public().
		From(notaryName).
		To(noto.Address).
		Function("unlock").
		Inputs(mintLockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	// Wait for coins to be created
	coins = findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoCoinState) bool {
		return len(coins) == 2
	})
	require.Len(t, coins, 2, "Should have 2 coins after unlock")

	// Verify coin amounts and owners
	var recipient1Coin, recipient2Coin *types.NotoCoinState
	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := tb.ResolveKey(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	for _, coin := range coins {
		if coin.Data.Owner.String() == recipient1Key.Verifier.Verifier {
			recipient1Coin = coin
		} else if coin.Data.Owner.String() == recipient2Key.Verifier.Verifier {
			recipient2Coin = coin
		}
	}
	require.NotNil(t, recipient1Coin, "Should have coin for recipient1")
	require.NotNil(t, recipient2Coin, "Should have coin for recipient2")
	assert.Equal(t, int64(100), recipient1Coin.Data.Amount.Int().Int64(), "Recipient1 should have 100")
	assert.Equal(t, int64(50), recipient2Coin.Data.Amount.Int().Int64(), "Recipient2 should have 50")

	// Verify final balances
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 100 after unlock")
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient2 should be 50 after unlock")
}

func (s *notoTestSuite) TestNotoPrepareBurnUnlock() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoPrepareBurnUnlock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, tb, rpc, _ := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	notoDomain := <-waitForNoto

	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, notary, nil)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Mint 100 from notary to recipient1")
	var invokeResult testbed.TransactionResult
	rpcerr := rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(100),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	coins := findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())

	log.L(ctx).Infof("Lock 50 from recipient1")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "lock",
			Data: toJSON(t, &types.LockParams{
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var lockReceipt types.NotoDomainReceipt
	err = json.Unmarshal(invokeResult.DomainReceipt, &lockReceipt)
	require.NoError(t, err)
	require.NotNil(t, lockReceipt.LockInfo)
	require.NotEmpty(t, lockReceipt.LockInfo.LockID)

	lockedCoins := findAvailableCoins[types.NotoLockedCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 1)
	assert.Equal(t, int64(50), lockedCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key.Verifier.Verifier, lockedCoins[0].Data.Owner.String())

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())

	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 50")

	log.L(ctx).Infof("Prepare burn unlock for the locked 50")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "prepareBurnUnlock",
			Data: toJSON(t, &types.PrepareBurnUnlockParams{
				LockID: lockReceipt.LockInfo.LockID,
				From:   recipient1Name,
				Amount: pldtypes.Int64ToInt256(50),
				Data:   pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var burnUnlockReceipt types.NotoDomainReceipt
	err = json.Unmarshal(invokeResult.DomainReceipt, &burnUnlockReceipt)
	require.NoError(t, err)
	require.NotNil(t, burnUnlockReceipt.LockInfo)
	require.NotNil(t, burnUnlockReceipt.LockInfo.UnlockParams)

	// Verify unlock params structure
	unlockParamsMap := burnUnlockReceipt.LockInfo.UnlockParams
	paramsMap, ok := unlockParamsMap["params"].(map[string]any)
	require.True(t, ok, "params should be a map")
	outputs, ok := paramsMap["outputs"].([]any)
	require.True(t, ok, "outputs should be an array")
	assert.Equal(t, 0, len(outputs), "prepareBurnUnlock should have no outputs")

	// Perform the unlock using the prepared unlock params (this should burn the locked value)
	log.L(ctx).Infof("Unlock the burn lock (should burn the locked value)")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	tx := pld.ForABI(ctx, notoBuild.ABI).
		Public().
		From(notaryName).
		To(noto.Address).
		Function("unlock").
		Inputs(burnUnlockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	// Wait for locked coins to be consumed (burned)
	lockedCoins = findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoLockedCoinState) bool {
		return len(coins) == 0
	})
	require.Len(t, lockedCoins, 0, "Should have no locked coins after burn unlock")

	// Verify no new coins were created (the locked value was burned)
	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1, "Should still have only 1 coin (the original 50)")
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64(), "The remaining coin should still be 50")

	// Verify final balance (should still be 50, as the locked 50 was burned)
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should still be 50 after burn")
}
