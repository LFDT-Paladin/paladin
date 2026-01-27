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

	log.L(ctx).Infof("Deploying Noto factory")
	contractSource := map[string][]byte{
		"factory": helpers.NotoFactoryJSON,
	}
	contracts := deployContracts(ctx, s.T(), s.hdWalletSeed, notaryName, contractSource)
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

func (s *notoTestSuite) TestNoto() {
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
	noto := helpers.DeployNoto(ctx, t, paladinClient, s.domainName, notary, nil)
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

func (s *notoTestSuite) TestNotoLock() {
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
	noto := helpers.DeployNoto(ctx, t, paladinClient, s.domainName, notary, nil)
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
	require.NotEmpty(t, prepareUnlockReceipt.LockInfo.UnlockParams.TxId)

	log.L(ctx).Infof("Delegate lock to recipient2")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "delegateLock",
			Data: toJSON(t, &types.DelegateLockParams{
				LockID:   prepareUnlockReceipt.LockInfo.LockID,
				Unlock:   prepareUnlockReceipt.LockInfo.UnlockParams,
				Delegate: pldtypes.MustEthAddress(recipient2Key),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	delegateLockReceipt := <-notoReceipts
	require.NotEmpty(t, delegateLockReceipt.LockInfo)
	assert.Equal(t, prepareUnlockReceipt.LockInfo.LockID, delegateLockReceipt.LockInfo.LockID)
	assert.Equal(t, recipient2Key, delegateLockReceipt.LockInfo.Delegate.String())

	log.L(ctx).Infof("Unlock from recipient2")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
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
	require.Equal(t, prepareUnlockReceipt.LockInfo.UnlockParams.TxId, pldtypes.Bytes32UUIDFirst16(unlockReceipt.txID).String())
	require.Len(t, unlockReceipt.Transfers, 1)
	assert.Equal(t, int64(50), unlockReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient2Key, unlockReceipt.Transfers[0].To.String())

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 100")

	findAvailableCoins(t, ctx, paladinClient, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoLockedCoinState) bool {
		return len(coins) == 0
	})
	coins = findAvailableCoins(t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoCoinState) bool {
		return len(coins) == 2
	})
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
