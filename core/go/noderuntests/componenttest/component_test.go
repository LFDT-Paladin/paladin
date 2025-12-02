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

/*
Test Kata component with no mocking of any internal units.
Starts the GRPC server and drives the internal functions via GRPC messages
*/
package componenttest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	testutils "github.com/LFDT-Paladin/paladin/core/noderuntests/pkg"
	"github.com/LFDT-Paladin/paladin/core/noderuntests/pkg/domains"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var CONFIG_PATHS = map[string]string{
	"node1": "./config/sqlite.node1.config.yaml",
	"node2": "./config/sqlite.node2.config.yaml",
	"node3": "./config/sqlite.node3.config.yaml",
}

func deployDomainRegistry(t *testing.T, nodeName string) *pldtypes.EthAddress {
	return testutils.DeployDomainRegistry(t, CONFIG_PATHS[nodeName])
}

func newInstanceForComponentTesting(t *testing.T, deployDomainAddress *pldtypes.EthAddress, enableWS bool, nodeName string) testutils.ComponentTestInstance {
	return testutils.NewInstanceForTesting(t, deployDomainAddress, nil, nil, nil, enableWS, CONFIG_PATHS[nodeName], false)
}

func newInstanceForComponentTestingWithDomainRegistry(t *testing.T) testutils.ComponentTestInstance {
	return newInstanceForComponentTesting(t, deployDomainRegistry(t, "node1"), true, "node1")
}

func startNode(t *testing.T, party testutils.Party, nodeName string, domainConfig interface{}) {
	party.Start(t, domainConfig, CONFIG_PATHS[nodeName], false)
}

func TestRunSimpleStorageEthTransaction(t *testing.T) {
	ctx := t.Context()

	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTestingWithDomainRegistry(t)
	c := instance.GetClient()

	build, err := solutils.LoadBuild(ctx, simpleStorageBuildJSON)
	require.NoError(t, err)

	simpleStorage := c.ForABI(ctx, build.ABI).Public().From("key1")

	res := simpleStorage.Clone().
		Constructor().
		Bytecode(build.Bytecode).
		Inputs(`{"x":11223344}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())
	contractAddr := res.Receipt().ContractAddress

	// set up the event listener
	success, err := c.PTX().CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "listener1",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     abi.ABI{build.ABI.Events()["Changed"]},
			Address: contractAddr,
		}},
	})
	require.NoError(t, err)
	require.True(t, success)

	wsClient, err := c.WebSocket(ctx, instance.GetWSConfig())
	require.NoError(t, err)

	eventData := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener1", eventData)

	success, err = c.PTX().StartBlockchainEventListener(ctx, "listener1")
	require.NoError(t, err)
	require.True(t, success)

	data := <-eventData
	assert.JSONEq(t, `{"x":"11223344"}`, data)

	var getX pldtypes.RawJSON
	err = simpleStorage.Clone().
		Function("get").
		To(contractAddr).
		Outputs(&getX).
		Call()
	require.NoError(t, err)
	assert.JSONEq(t, `{"x":"11223344"}`, getX.Pretty())

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":99887766}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	data = <-eventData
	assert.JSONEq(t, `{"x":"99887766"}`, data)

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":1234}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	data = <-eventData
	assert.JSONEq(t, `{"x":"1234"}`, data)
}

func TestBlockchainEventListeners(t *testing.T) {
	ctx := t.Context()

	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTestingWithDomainRegistry(t)
	c := instance.GetClient()

	build, err := solutils.LoadBuild(ctx, simpleStorageBuildJSON)
	require.NoError(t, err)

	simpleStorage := c.ForABI(ctx, build.ABI).Public().From("key1")

	res := simpleStorage.Clone().
		Constructor().
		Bytecode(build.Bytecode).
		Inputs(`{"x":1}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())
	contractAddr := res.Receipt().ContractAddress
	deployBlock := res.Receipt().BlockNumber

	// set up the event listener
	_, err = c.PTX().CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name:    "listener1",
		Started: confutil.P(false),
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     abi.ABI{build.ABI.Events()["Changed"]},
			Address: contractAddr,
		}},
	})
	require.NoError(t, err)

	status, err := c.PTX().GetBlockchainEventListenerStatus(ctx, "listener1")
	require.NoError(t, err)
	assert.Equal(t, int64(-1), status.Checkpoint.BlockNumber)

	wsClient, err := c.WebSocket(ctx, instance.GetWSConfig())
	require.NoError(t, err)

	listener1 := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener1", listener1)

	require.Never(t, func() bool {
		select {
		case <-listener1:
			return true
		default:
			return false
		}
	}, 100*time.Millisecond, 5*time.Millisecond, "unexpected event received on stopped listener")

	_, err = c.PTX().StartBlockchainEventListener(ctx, "listener1")
	require.NoError(t, err)

	assert.JSONEq(t, `{"x":"1"}`, <-listener1)

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":2}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	assert.JSONEq(t, `{"x":"2"}`, <-listener1)

	// making this check immediately after receiving the event results in a race condition where the ack might not have been processed
	// and the checkpoint updated, so check that it is either equal to the block number of the deploy or the block number of the invoke
	status, err = c.PTX().GetBlockchainEventListenerStatus(ctx, "listener1")
	require.NoError(t, err)
	assert.True(t, status.Checkpoint.BlockNumber == deployBlock || status.Checkpoint.BlockNumber == res.Receipt().BlockNumber)

	// stop the event listener
	_, err = c.PTX().StopBlockchainEventListener(ctx, "listener1")
	require.NoError(t, err)

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":3}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	// pause to make sure that if an event was going to be received, it would have been
	ticker2 := time.NewTicker(10 * time.Millisecond)
	defer ticker2.Stop()

	select {
	case <-listener1:
		t.FailNow()
	case <-ticker2.C:
	}

	_, err = c.PTX().StartBlockchainEventListener(ctx, "listener1")
	require.NoError(t, err)

	assert.JSONEq(t, `{"x":"3"}`, <-listener1)

	// create a second listener with default fromBlock settings, it should receive all the events
	_, err = c.PTX().CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "listener2",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     abi.ABI{build.ABI.Events()["Changed"]},
			Address: contractAddr,
		}},
	})
	require.NoError(t, err)

	listener2 := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener2", listener2)

	assert.JSONEq(t, `{"x":"1"}`, <-listener2)
	assert.JSONEq(t, `{"x":"2"}`, <-listener2)
	assert.JSONEq(t, `{"x":"3"}`, <-listener2)

	// create a third listener that listeners from latest
	_, err = c.PTX().CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "listener3",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     abi.ABI{build.ABI.Events()["Changed"]},
			Address: contractAddr,
		}},
		Options: pldapi.BlockchainEventListenerOptions{
			FromBlock: json.RawMessage(`"latest"`),
		},
	})
	require.NoError(t, err)

	listener3 := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener3", listener3)

	// submit another transaction- this should be the next event that all the listeners receive
	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":4}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	assert.JSONEq(t, `{"x":"4"}`, <-listener1)
	assert.JSONEq(t, `{"x":"4"}`, <-listener2)
	assert.JSONEq(t, `{"x":"4"}`, <-listener3)
}

func subscribeAndSendDataToChannel(ctx context.Context, t *testing.T, wsClient pldclient.PaladinWSClient, listenerName string, data chan string) {
	sub, err := wsClient.PTX().SubscribeBlockchainEvents(ctx, listenerName)
	require.NoError(t, err)
	go func() {
		for {
			select {
			case subNotification, ok := <-sub.Notifications():
				if ok {
					eventData := make([]string, 0)
					var batch pldapi.TransactionEventBatch
					_ = json.Unmarshal(subNotification.GetResult(), &batch)
					for _, e := range batch.Events {
						t.Logf("Received event on %s from %d/%d/%d : %s", listenerName, e.BlockNumber, e.TransactionIndex, e.LogIndex, e.Data.String())
						eventData = append(eventData, e.Data.String())
					}
					require.NoError(t, subNotification.Ack(ctx))
					// send after the ack otherwise the main test can complete when it receives the last values and the websocket is closed before the ack
					// can be sent
					for _, d := range eventData {
						data <- d
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func TestUpdatePublicTransaction(t *testing.T) {
	ctx := t.Context()
	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTestingWithDomainRegistry(t)
	c := instance.GetClient()

	// set up the receipt listener
	success, err := c.PTX().CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)
	require.True(t, success)

	wsClient, err := c.WebSocket(ctx, instance.GetWSConfig())
	require.NoError(t, err)

	sub, err := wsClient.PTX().SubscribeReceipts(ctx, "listener1")
	require.NoError(t, err)

	build, err := solutils.LoadBuild(ctx, simpleStorageBuildJSON)
	require.NoError(t, err)

	simpleStorage := c.ForABI(ctx, build.ABI).Public().From("key1")

	res := simpleStorage.Clone().
		Constructor().
		Bytecode(build.Bytecode).
		Inputs(`{"x":11223344}`).
		Send()
	require.NoError(t, res.Error())

	var deployReceipt *pldapi.TransactionReceiptFull

	for deployReceipt == nil {
		subNotification, ok := <-sub.Notifications()
		if ok {
			var batch pldapi.TransactionReceiptBatch
			_ = json.Unmarshal(subNotification.GetResult(), &batch)
			for _, r := range batch.Receipts {
				if *res.ID() == r.ID {
					deployReceipt = r
				}
			}
			err := subNotification.Ack(ctx)
			require.NoError(t, err)
		}
	}

	tx, err := c.PTX().GetTransactionFull(ctx, *res.ID())
	require.NoError(t, err)
	contractAddr := tx.Receipt.ContractAddress

	setRes := simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":99887766}`).
		PublicTxOptions(pldapi.PublicTxOptions{
			// gas is set below instrinsic limit
			Gas: confutil.P(pldtypes.HexUint64(1)),
		}).
		Send()
	require.NoError(t, setRes.Error())
	require.NotNil(t, setRes.ID())

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		tx, err = c.PTX().GetTransactionFull(ctx, *setRes.ID())
		require.NoError(ct, err)
		require.Len(ct, tx.Public, 1)
		require.NotNil(ct, tx.Public[0].Activity[0])
		assert.Regexp(ct, "ERROR.*Intrinsic", tx.Public[0].Activity[0])
	}, 10*time.Second, 100*time.Millisecond, "Transaction was not processed with error in time")

	_, err = c.PTX().UpdateTransaction(ctx, *setRes.ID(), &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:         "key1",
			Function:     "set",
			Data:         pldtypes.RawJSON(`{"_x":99887766}`),
			To:           contractAddr,
			ABIReference: tx.ABIReference,
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas: confutil.P(pldtypes.HexUint64(10000000)),
			},
		},
	})
	require.NoError(t, err)

	var setReceipt *pldapi.TransactionReceiptFull
	for setReceipt == nil {
		subNotification, ok := <-sub.Notifications()
		if ok {
			var batch pldapi.TransactionReceiptBatch
			_ = json.Unmarshal(subNotification.GetResult(), &batch)
			for _, r := range batch.Receipts {
				if *setRes.ID() == r.ID {
					setReceipt = r
				}
			}
			err := subNotification.Ack(ctx)
			require.NoError(t, err)
		}

	}

	tx, err = c.PTX().GetTransactionFull(ctx, *setRes.ID())
	require.NoError(t, err)
	require.NotNil(t, tx.Receipt)
	require.True(t, tx.Receipt.Success)
	require.Len(t, tx.Public, 1)
	assert.Equal(t, tx.Public[0].Submissions[0].TransactionHash.HexString(), setReceipt.TransactionHash.HexString())
	assert.Len(t, tx.History, 2)

	// try to update the transaction again- it should fail now it is complete
	_, err = c.PTX().UpdateTransaction(ctx, *setRes.ID(), &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:         "key1",
			Function:     "set",
			Data:         pldtypes.RawJSON(`{"_x":99887765}`),
			To:           contractAddr,
			ABIReference: tx.ABIReference,
		},
	})
	assert.ErrorContains(t, err, "PD011937")
}

func TestPrivateTransactionsDeployAndExecute(t *testing.T) {
	// Coarse grained black box test of the core component manager
	// no mocking although it does use a simple domain implementation that exists solely for testing
	// and is loaded directly through go function calls via the unit test plugin loader
	// (as opposed to compiling as a separate shared library)
	// Even though the domain is a fake, the test does deploy a real contract to the blockchain and the domain
	// manager does communicate with it via the grpc interface.
	// The bootstrap code that is the entry point to the java side is not tested here, we bootstrap the component manager by hand

	ctx := t.Context()
	instance := newInstanceForComponentTestingWithDomainRegistry(t)
	client := instance.GetClient()

	// Check there are no transactions before we start
	txns, err := client.PTX().QueryTransactionsFull(ctx, query.NewQueryBuilder().Limit(1).Query())
	require.NoError(t, err)
	assert.Len(t, txns, 0)

	deployTx := client.ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.SelfEndorsement)).
		Private().
		Domain("domain1").
		IdempotencyKey("deploy1").
		From("wallets.org1.aaaaaa").
		Inputs(pldtypes.RawJSON(`{
                    "from": "wallets.org1.aaaaaa",
                    "name": "FakeToken1",
                    "symbol": "FT1",
					"endorsementMode": "` + domains.SelfEndorsement + `",
					"hookAddress": ""
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, deployTx.Error())
	contractAddress := deployTx.Receipt().ContractAddress

	// Start a private transaction
	tx1 := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1").
		From("wallets.org1.aaaaaa").
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                "from": "",
                "to": "wallets.org1.aaaaaa",
                "amount": "123000000000000000000"
            }`)).
		Send().Wait(transactionLatencyThreshold(t))

	require.NoError(t, tx1.Error())

	txns, err = client.PTX().QueryTransactionsFull(ctx, query.NewQueryBuilder().Limit(2).Query())
	require.NoError(t, err)
	assert.Len(t, txns, 2)

	txFull, err := client.PTX().GetTransactionFull(ctx, tx1.ID())
	require.NoError(t, err)

	require.NotNil(t, txFull.Receipt)
	assert.True(t, txFull.Receipt.Success)
}

func TestPrivateTransactionsMintThenTransfer(t *testing.T) {
	// Invoke 2 transactions on the same contract where the second transaction relies on the state created by the first

	ctx := t.Context()
	instance := newInstanceForComponentTestingWithDomainRegistry(t)
	client := instance.GetClient()

	// Check there are no transactions before we start
	txns, err := client.PTX().QueryTransactionsFull(ctx, query.NewQueryBuilder().Limit(1).Query())
	require.NoError(t, err)
	assert.Len(t, txns, 0)
	deployTx := client.ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.SelfEndorsement)).
		Private().
		Domain("domain1").
		IdempotencyKey("deploy1").
		From("wallets.org1.aaaaaa").
		Inputs(pldtypes.RawJSON(`{
                    "from": "wallets.org1.aaaaaa",
                    "name": "FakeToken1",
                    "symbol": "FT1",
					"endorsementMode": "` + domains.SelfEndorsement + `",
					"hookAddress": ""
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, deployTx.Error())
	contractAddress := deployTx.Receipt().ContractAddress

	// Start a private transaction - Mint to alice
	tx1 := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1").
		From("wallets.org1.aaaaaa").
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                "from": "",
                "to": "wallets.org1.bbbbbb",
                "amount": "123000000000000000000"
            }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, tx1.Error())

	// Start a private transaction - Transfer from alice to bob
	tx2 := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx2").
		From("wallets.org1.bbbbbb").
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                "from": "wallets.org1.bbbbbb",
                "to": "wallets.org1.aaaaaa",
                "amount": "123000000000000000000"
            }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, tx2.Error())
}

func TestPrivateTransactionRevertedAssembleFailed(t *testing.T) {
	// Invoke a transaction that will fail to assemble
	// in this case, we use the simple token domain and attempt to transfer from a wallet that has no tokens
	ctx := t.Context()
	instance := newInstanceForComponentTestingWithDomainRegistry(t)
	client := instance.GetClient()

	deployTx := client.ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.SelfEndorsement)).
		Private().
		Domain("domain1").
		IdempotencyKey("deploy1").
		From("wallets.org1.aaaaaa").
		Inputs(pldtypes.RawJSON(`{
					"from": "wallets.org1.aaaaaa",
					"name": "FakeToken1",
					"symbol": "FT1",
					"endorsementMode": "` + domains.SelfEndorsement + `",
					"hookAddress": ""
				}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, deployTx.Error())
	contractAddress := deployTx.Receipt().ContractAddress

	// Start a private transaction - Transfer from alice to bob but we expect that alice can't afford this
	// however, that wont be known until the transaction is assembled which is asynchronous so the initial submission
	// should succeed
	tx1 := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx2").
		From("wallets.org1.bbbbbb").
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
				"from": "wallets.org1.bbbbbb",
				"to": "wallets.org1.aaaaaa",
				"amount": "123000000000000000000"
			}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.Error(t, tx1.Error())
	require.NotNil(t, tx1.Receipt())
	require.False(t, tx1.Receipt().Success)
	assert.Regexp(t, domains.SimpleDomainInsufficientFundsError, tx1.Receipt().FailureMessage)

	//Check that the domain is left in a healthy state and we can submit good transactions
	// Start a private transaction - Mint to alice
	goodTx := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("goodTx").
		From("wallets.org1.aaaaaa").
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                "from": "",
                "to": "wallets.org1.bbbbbb",
                "amount": "123000000000000000000"
            }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, goodTx.Error())
}

func TestDeployOnOneNodeInvokeOnAnother(t *testing.T) {
	// We use the simple token where there is no actual on chain checking of the notary
	// so either node can assemble a transaction with an attestation plan for a local notary
	// there is also no access control around minting so both nodes are able to mint tokens and we don't
	// need the complexity of cross node transfers in this test
	ctx := t.Context()

	domainRegistryAddress := deployDomainRegistry(t, "node1")

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, false, "node1")
	client1 := instance1.GetClient()
	aliceIdentity := "wallets.org1.alice"
	aliceAddress := instance1.ResolveEthereumAddress(aliceIdentity)
	t.Logf("Alice address: %s", aliceAddress)

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, false, "node2")
	client2 := instance2.GetClient()
	bobIdentity := "wallets.org2.bob"
	bobAddress := instance2.ResolveEthereumAddress(bobIdentity)
	t.Logf("Bob address: %s", bobAddress)

	//If this fails, it is most likely a bug in the test utils that configures each node with seed mnemonics
	assert.NotEqual(t, aliceAddress, bobAddress)

	// send JSON RPC message to node 1 to deploy a private contract, using alice's key
	deployTx := client1.ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.SelfEndorsement)).
		Private().
		Domain("domain1").
		IdempotencyKey("deploy1").
		From(aliceIdentity).
		Inputs(pldtypes.RawJSON(`{
			"from": "` + aliceIdentity + `",
			"name": "FakeToken1",
			"symbol": "FT1",
			"endorsementMode": "` + domains.SelfEndorsement + `",
			"hookAddress": ""
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, deployTx.Error())
	contractAddress := deployTx.Receipt().ContractAddress

	// Start a private transaction on alices node
	// this is a mint to alice
	aliceTx := client1.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice").
		From(aliceIdentity).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + aliceIdentity + `",
                    "amount": "123000000000000000000"
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, aliceTx.Error())

	// Start a private transaction on bobs node
	// This is a mint to bob
	bobTx1 := client2.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-bob").
		From(bobIdentity).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bobIdentity + `",
                    "amount": "123000000000000000000"
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, bobTx1.Error())
}

func TestResolveIdentityFromRemoteNode(t *testing.T) {
	// stand up 2 nodes, with different key managers
	// send an RPC request to one node to resolve the identity of a user@the-other-node
	// this forces both nodes to communicate with each other to resolve the identity

	ctx := t.Context()

	//TODO shouldn't need domain registry for this test
	domainRegistryAddress := deployDomainRegistry(t, "node1")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	startNode(t, alice, "node1", nil)
	startNode(t, bob, "node2", nil)

	client1 := alice.GetClient()
	aliceIdentity := alice.GetIdentityLocator()
	aliceAddress := alice.ResolveEthereumAddress(aliceIdentity)
	t.Logf("Alice address: %s", aliceAddress)

	client2 := bob.GetClient()
	bobIdentity := bob.GetIdentityLocator()
	bobUnqualifiedIdentity := bob.GetIdentity()
	bobAddress := bob.ResolveEthereumAddress(bobIdentity)
	t.Logf("Bob address: %s", bobAddress)

	// send JSON RPC message to node 1 to resolve a verifier on node 2
	verifierResult1, err := client1.PTX().ResolveVerifier(ctx, bobIdentity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	require.NotNil(t, verifierResult1)

	// resolve the same verifier on node 2 directly
	verifierResult2, err := client2.PTX().ResolveVerifier(ctx, bobIdentity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	require.NotNil(t, verifierResult2)

	// resolve the same verifier on node 2 directly using the unqualified identity
	verifierResult3, err := client2.PTX().ResolveVerifier(ctx, bobUnqualifiedIdentity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	require.NotNil(t, verifierResult3)

	// all 3 results should be the same
	assert.Equal(t, verifierResult1, verifierResult2)
	assert.Equal(t, verifierResult1, verifierResult3)

}

func TestCreateStateOnOneNodeSpendOnAnother(t *testing.T) {
	// We use the simple token in SelfEndorsement mode (similar to zeto so either node can assemble a transaction
	// however, in this test, Bob's transaction will only succeed if he can spend the coins that Alice transfers to him
	// so this tests that the state is shared between the nodes

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "node1")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, "node1", domainConfig)
	startNode(t, bob, "node2", domainConfig)

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alices node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bobs node
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice").
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "123000000000000000000"
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, aliceTx.Error())

	// Start a private transaction on bobs node
	// This is a transfer which relies on bobs node being aware of the state created by alice's mint to bob above
	bobTx1 := bob.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-bob").
		From(bob.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                    "from": "` + bob.GetIdentityLocator() + `",
                    "to": "` + alice.GetIdentityLocator() + `",
                    "amount": "123000000000000000000"
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, bobTx1.Error())
}

// This function helps work around a timing issue with sqlite in-memory DB. If a node attempts to connect to a peer
// while mid-transaction (e.g. issuing SendReliable during a runBatch) sqlite blocks the SELECT query it issues to
// look up the peer's connection details, which hangs the test.If the peers are already connected, this issue doesn't
// arise, and if using postgres it also doesn't arise. This util function resolves all identities between all clients
// to ensure peers are connected before running the test. This is over zealous when running the entire suite because
// the peers will have connected in the first test, but when running an individual test it allows the test to pass.
func ensurePeerConnections(t *testing.T, ctx context.Context, parties ...testutils.Party) {
	clients := make([]pldclient.PaladinClient, len(parties))
	identities := make([]string, len(parties))
	for i, party := range parties {
		clients[i] = party.GetClient()
		identities[i] = party.GetIdentityLocator()
	}
	// For every client, resolve every identity
	for _, client := range clients {
		for _, identity := range identities {
			var verifierResult string
			verifierResult, err := client.PTX().ResolveVerifier(ctx, identity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			require.NoError(t, err)
			require.NotEmpty(t, verifierResult)
		}
	}
}

func TestNotaryDelegated(t *testing.T) {
	//This is similar to the noto scenario
	// all transfers must be endorsed by the single notary and the notary must submit to the base ledger
	// it also happens to be the case in noto that only the notary can mint so we replicate that
	// constraint here too so this test serves as a reasonable contract test for the noto use case

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "node1")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	notary := testutils.NewPartyForTesting(t, "notary", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig(), notary.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig(), notary.GetNodeConfig())
	notary.AddPeer(alice.GetNodeConfig(), bob.GetNodeConfig())

	startNode(t, alice, "node1", nil)
	startNode(t, bob, "node2", nil)
	startNode(t, notary, "node3", nil)

	ensurePeerConnections(t, ctx, alice, bob, notary)

	// send JSON RPC message to node 3 ( notary) to deploy a private contract
	deployTx := notary.GetClient().ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.NotaryEndorsement)).
		Private().
		Domain("domain1").
		IdempotencyKey("deploy1").
		From(notary.GetIdentityLocator()).
		Inputs(pldtypes.RawJSON(`{
					"notary": "` + notary.GetIdentityLocator() + `",
					"name": "FakeToken1",
					"symbol": "FT1",
					"endorsementMode": "NotaryEndorsement",
					"hookAddress": ""
				}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, deployTx.Error())
	contractAddress := deployTx.Receipt().ContractAddress

	// As notary, mint some tokens to alice
	// Start a private transaction on notary node
	// this is a mint to alice so alice should later be able to do a transfer to bob
	mintTx := notary.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-mint").
		From(notary.GetIdentityLocator()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
					"from": "",
					"to": "` + alice.GetIdentityLocator() + `",
					"amount": "100"
				}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, mintTx.Error())

	// Start a private transaction on alices node to transfer to bob
	transferA2BTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("transferA2B1").
		From(alice.GetIdentityLocator()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
					"from": "` + alice.GetIdentityLocator() + `",
					"to": "` + bob.GetIdentityLocator() + `",
					"amount": "50"
				}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, transferA2BTx.Error())

	// Attempt a private transaction on alices node that will fail due to insufficent funds
	transferA2FailTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("transferFailA2B1").
		From(alice.GetIdentityLocator()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
					"from": "` + alice.GetIdentityLocator() + `",
					"to": "` + bob.GetIdentityLocator() + `",
					"amount": "5000000000000000000"
				}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.Error(t, transferA2FailTx.Error())
	require.NotNil(t, transferA2FailTx.Receipt())
	require.False(t, transferA2FailTx.Receipt().Success)
}

func TestNotaryDelegatedPrepare(t *testing.T) {
	//Similar to the TestNotaryDelegated test except in this case, the transaction is not submitted to the base ledger by the notary.
	//instead, the assembled and prepared transaction is returned to the originator node to submit to the base ledger whenever it is deemed appropriate
	// NOTE the use of ptx_prepareTransaction instead of ptx_sendTransaction on the transfer

	ctx := t.Context()

	domainRegistryAddress := deployDomainRegistry(t, "node1")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	notary := testutils.NewPartyForTesting(t, "notary", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig(), notary.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig(), notary.GetNodeConfig())
	notary.AddPeer(alice.GetNodeConfig(), bob.GetNodeConfig())

	startNode(t, alice, "node1", nil)
	startNode(t, bob, "node2", nil)
	startNode(t, notary, "node3", nil)

	ensurePeerConnections(t, ctx, alice, bob, notary)

	// send JSON RPC message to node 3 ( notary) to deploy a private contract
	deployTx := notary.GetClient().ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.NotaryEndorsement)).
		Private().
		Domain("domain1").
		IdempotencyKey("deploy1").
		From(notary.GetIdentityLocator()).
		Inputs(pldtypes.RawJSON(`{
					"notary": "` + notary.GetIdentityLocator() + `",
					"name": "FakeToken1",
					"symbol": "FT1",
					"endorsementMode": "NotaryEndorsement",
					"deleteSubmitToSender": true,
					"hookAddress": ""
				}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, deployTx.Error())
	contractAddress := deployTx.Receipt().ContractAddress

	// As notary, mint some tokens to alice
	// Start a private transaction on notary node
	// this is a mint to alice so alice should later be able to do a transfer to bob
	mintTx := notary.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-mint").
		From(notary.GetIdentityLocator()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
					"from": "",
					"to": "` + alice.GetIdentityLocator() + `",
					"amount": "100"
				}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, mintTx.Error())

	// Prepare a private transaction on alices node to transfer to bob
	transferA2BTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("transferA2B1").
		From(alice.GetIdentityLocator()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
					"from": "` + alice.GetIdentityLocator() + `",
					"to": "` + bob.GetIdentityLocator() + `",
					"amount": "25"
				}`)).
		Prepare()
	require.NoError(t, transferA2BTx.Error())
	transferA2BTxID := transferA2BTx.ID()
	require.NotNil(t, transferA2BTxID)

	_, err := alice.GetClient().PTX().GetTransactionFull(ctx, *transferA2BTx.ID())
	require.NoError(t, err)

	_, err = notary.GetClient().PTX().GetTransactionFull(ctx, *transferA2BTx.ID())
	require.NoError(t, err)

	assert.Eventually(t,
		func() bool {
			// The transaction is prepared with a from-address that is local to node3 - so only
			// node3 will be able to send it. So that's where it gets persisted.
			preparedTx, err := notary.GetClient().PTX().GetPreparedTransaction(ctx, *transferA2BTx.ID())
			require.NoError(t, err)

			if preparedTx == nil {
				return false
			}
			assert.Empty(t, preparedTx.Transaction.Domain)
			return preparedTx.ID == *transferA2BTx.ID() && len(preparedTx.States.Spent) == 1 && len(preparedTx.States.Confirmed) == 2

		},
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Prepared transaction not available on originator node",
	)

}

func TestSingleNodeSelfEndorseConcurrentSpends(t *testing.T) {
	// Invoke a bunch of transactions on the same contract on a single node, in self endorsement mode ( a la zeto )
	// where there is a reasonable possibility of contention between transactions

	//start by minting 5 coins then send 5 transactions to spend them
	// if there is no contention, each transfer should be able to spend a coin each

	ctx := t.Context()
	instance := newInstanceForComponentTestingWithDomainRegistry(t)
	client := instance.GetClient()

	deployTx := client.ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.SelfEndorsement)).
		Private().
		Domain("domain1").
		IdempotencyKey("deploy1").
		From("wallets.org1.aaaaaa").
		Inputs(pldtypes.RawJSON(`{
                    "from": "wallets.org1.aaaaaa",
                    "name": "FakeToken1",
                    "symbol": "FT1",
					"endorsementMode": "` + domains.SelfEndorsement + `",
					"hookAddress": ""
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, deployTx.Error())
	contractAddress := deployTx.Receipt().ContractAddress

	// Do the 5 mints
	mint := func() pldclient.SentTransaction {
		tx := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
			Private().
			Domain("domain1").
			IdempotencyKey(pldtypes.RandHex(8)).
			From("wallets.org1.aaaaaa").
			To(contractAddress).
			Function("transfer").
			Inputs(pldtypes.RawJSON(`{
					"from": "",
					"to": "wallets.org1.aaaaaa",
					"amount": "1"
				}`)).
			Send()
		require.NoError(t, tx.Error())
		return tx
	}
	waitForTransaction := func(tx pldclient.SentTransaction) {
		result := tx.Wait(transactionLatencyThreshold(t))
		require.NoError(t, result.Error())
	}

	mint1 := mint()
	mint2 := mint()
	mint3 := mint()
	mint4 := mint()
	mint5 := mint()

	waitForTransaction(mint1)
	waitForTransaction(mint2)
	waitForTransaction(mint3)
	waitForTransaction(mint4)
	waitForTransaction(mint5)

	// Now kick off the 5 transfers
	transfer := func() pldclient.SentTransaction {
		tx := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
			Private().
			Domain("domain1").
			IdempotencyKey(pldtypes.RandHex(8)).
			From("wallets.org1.aaaaaa").
			To(contractAddress).
			Function("transfer").
			Inputs(pldtypes.RawJSON(`{
					"from": "wallets.org1.aaaaaa",
					"to": "wallets.org1.bbbbbb",
					"amount": "1"
				}`)).
			Send()
		require.NoError(t, tx.Error())
		return tx
	}
	transfer1 := transfer()
	transfer2 := transfer()
	transfer3 := transfer()
	transfer4 := transfer()
	transfer5 := transfer()

	waitForTransaction(transfer1)
	waitForTransaction(transfer2)
	waitForTransaction(transfer3)
	waitForTransaction(transfer4)
	waitForTransaction(transfer5)

}

func TestSingleNodeSelfEndorseSeriesOfTransfers(t *testing.T) {
	// Invoke a series of transactions on the same contract on a single node, in self endorsement mode ( a la zeto )
	//where each transaction relies on the state created by the previous

	ctx := t.Context()
	instance := newInstanceForComponentTestingWithDomainRegistry(t)
	client := instance.GetClient()

	// Check there are no transactions before we start
	txns, err := client.PTX().QueryTransactionsFull(ctx, query.NewQueryBuilder().Limit(1).Query())
	require.NoError(t, err)
	assert.Len(t, txns, 0)
	deployTx := client.ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.SelfEndorsement)).
		Private().
		Domain("domain1").
		IdempotencyKey("deploy1").
		From("wallets.org1.aaaaaa").
		Inputs(pldtypes.RawJSON(`{
                    "from": "wallets.org1.aaaaaa",
                    "name": "FakeToken1",
                    "symbol": "FT1",
					"endorsementMode": "` + domains.SelfEndorsement + `",
					"hookAddress": ""
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, deployTx.Error())
	contractAddress := deployTx.Receipt().ContractAddress

	// Start a private transaction - Mint to alice
	tx1 := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1").
		From("wallets.org1.aaaaaa").
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                "from": "",
                "to": "wallets.org1.bbbbbb",
                "amount": "100"
            }`)).
		Send()
	require.NoError(t, tx1.Error())

	// Start a private transaction - Transfer from alice to bob
	tx2 := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx2").
		From("wallets.org1.bbbbbb").
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                "from": "wallets.org1.bbbbbb",
                "to": "wallets.org1.aaaaaa",
                "amount": "99"
            }`)).
		Send()
	require.NoError(t, tx2.Error())

	// Start a private transaction - Transfer from alice to bob
	tx3 := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx3").
		From("wallets.org1.aaaaaa").
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
                "from": "wallets.org1.aaaaaa",
                "to": "wallets.org1.bbbbbb",
                "amount": "98"
            }`)).
		Send()
	require.NoError(t, tx3.Error())

	tx1Result := tx1.Wait(transactionLatencyThreshold(t))
	require.NoError(t, tx1Result.Error())
	tx2Result := tx2.Wait(transactionLatencyThreshold(t))
	require.NoError(t, tx2Result.Error())
	tx3Result := tx3.Wait(transactionLatencyThreshold(t))
	require.NoError(t, tx3Result.Error())

}

func TestNotaryEndorseConcurrentSpends(t *testing.T) {
	// Invoke a bunch of transactions on the same contract in self endorsement mode ( a la noto )
	// perform the transfers from the same identity so that there is high likelihood of contention

	//start by minting 5 coins then send 5 transactions to spend them
	// if there is no contention, each transfer should be able to spend a coin each

	ctx := t.Context()

	domainRegistryAddress := deployDomainRegistry(t, "node1")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	notary := testutils.NewPartyForTesting(t, "notary", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig(), notary.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig(), notary.GetNodeConfig())
	notary.AddPeer(alice.GetNodeConfig(), bob.GetNodeConfig())

	startNode(t, alice, "node1", nil)
	startNode(t, bob, "node2", nil)
	startNode(t, notary, "node3", nil)

	ensurePeerConnections(t, ctx, alice, bob, notary)

	// send JSON RPC message to node 3 ( notary) to deploy a private contract
	deployTx := notary.GetClient().ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.NotaryEndorsement)).
		Private().
		Domain("domain1").
		IdempotencyKey("deploy1").
		From(notary.GetIdentityLocator()).
		Inputs(pldtypes.RawJSON(`{
					"notary": "` + notary.GetIdentityLocator() + `",
					"name": "FakeToken1",
					"symbol": "FT1",
					"endorsementMode": "NotaryEndorsement",
					"hookAddress": ""
				}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, deployTx.Error())
	contractAddress := deployTx.Receipt().ContractAddress

	// Start a private transaction on notary node
	// this is a mint to alice so alice should later be able to do a transfer to bob

	// Do the 5 mints
	mint := func() pldclient.SentTransaction {
		tx := notary.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
			Private().
			Domain("domain1").
			IdempotencyKey(pldtypes.RandHex(8)).
			From(notary.GetIdentityLocator()).
			To(contractAddress).
			Function("transfer").
			Inputs(pldtypes.RawJSON(`{
					"from": "",
					"to": "` + alice.GetIdentityLocator() + `",
					"amount": "100"
				}`)).
			Send()
		require.NoError(t, tx.Error())
		return tx
	}
	waitForTransaction := func(tx pldclient.SentTransaction) {
		result := tx.Wait(transactionLatencyThreshold(t))
		require.NoError(t, result.Error())
	}

	mint1 := mint()
	mint2 := mint()
	mint3 := mint()
	mint4 := mint()
	mint5 := mint()

	waitForTransaction(mint1)
	waitForTransaction(mint2)
	waitForTransaction(mint3)
	waitForTransaction(mint4)
	waitForTransaction(mint5)

	// Now kick off the 5 transfers
	transfer := func() pldclient.SentTransaction {
		tx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
			Private().
			Domain("domain1").
			IdempotencyKey(pldtypes.RandHex(8)).
			From(alice.GetIdentityLocator()).
			To(contractAddress).
			Function("transfer").
			Inputs(pldtypes.RawJSON(`{
						"from": "` + alice.GetIdentityLocator() + `",
						"to": "` + bob.GetIdentityLocator() + `",
						"amount": "100"
					}`)).
			Send()
		require.NoError(t, tx.Error())
		return tx
	}
	transfer1 := transfer()
	transfer2 := transfer()
	transfer3 := transfer()
	transfer4 := transfer()
	transfer5 := transfer()

	waitForTransaction(transfer1)
	waitForTransaction(transfer2)
	waitForTransaction(transfer3)
	waitForTransaction(transfer4)
	waitForTransaction(transfer5)

}

func TestPrivacyGroupEndorsement(t *testing.T) {
	// This test is intended to emulate the pente domain where all transactions must be endorsed by all parties in the predefined privacy group
	// in this case, we have 3 nodes, each representing a different party in the privacy group
	// and we expect that all transactions must be endorsed by all 3 nodes and that all output states are distributed to all 3 nodes
	// Unlike the coin based domains, this is a "world state" based domain so there is only ever one available state at any one time and each
	// transaction spends that state and creates a new one.  So there is contention between parties
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "node1")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	carol := testutils.NewPartyForTesting(t, "carol", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig(), carol.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig(), carol.GetNodeConfig())
	carol.AddPeer(alice.GetNodeConfig(), bob.GetNodeConfig())

	domainConfig := &domains.SimpleStorageDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}
	startNode(t, alice, "node1", domainConfig)
	startNode(t, bob, "node2", domainConfig)
	startNode(t, carol, "node3", domainConfig)

	endorsementSet := []string{alice.GetIdentityLocator(), bob.GetIdentityLocator(), carol.GetIdentityLocator()}

	constructorParameters := &domains.SimpleStorageConstructorParameters{
		EndorsementSet:  endorsementSet,
		Name:            "SimpleStorage1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
	}
	// send JSON RPC message to node 1 to deploy a private contract
	contractAddress := alice.DeploySimpleStorageDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this should require endorsement from bob and carol
	// Initialise a new map
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleStorageInitABI()).
		Private().
		Domain("simpleStorageDomain").
		IdempotencyKey("tx1-alice").
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("init").
		Inputs(pldtypes.RawJSON(`{
					"map":"map1"
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, aliceTx.Error())

	// Start a private transaction on bob's node
	// this should require endorsement from alice and carol
	bobTx := bob.GetClient().ForABI(ctx, *domains.SimpleStorageSetABI()).
		Private().
		Domain("simpleStorageDomain").
		IdempotencyKey("tx1-bob").
		From(bob.GetIdentity()).
		To(contractAddress).
		Function("set").
		Inputs(pldtypes.RawJSON(`{
					"map":"map1",
                    "key": "foo",
					"value": "quz"
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, bobTx.Error())

	bobSchemas, err := bob.GetClient().StateStore().ListSchemas(ctx, "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, bobSchemas, 1)

	bobStates, err := bob.GetClient().StateStore().QueryContractStates(ctx, "simpleStorageDomain", *contractAddress, bobSchemas[0].ID, &query.QueryJSON{}, "available")
	require.NoError(t, err)
	require.Len(t, bobStates, 1)
	stateData := make(map[string]string)
	storage := make(map[string]string)
	jsonErr := json.Unmarshal(bobStates[0].Data.Bytes(), &stateData)
	require.NoError(t, jsonErr)

	jsonErr = json.Unmarshal([]byte(stateData["records"]), &storage)
	require.NoError(t, jsonErr)

	assert.Equal(t, "quz", storage["foo"])

	// Alice should see the same latest state of the world as Bob
	aliceSchemas, err := alice.GetClient().StateStore().ListSchemas(ctx, "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, aliceSchemas, 1)
	assert.Equal(t, bobSchemas[0].ID, aliceSchemas[0].ID)

	aliceStates, err := alice.GetClient().StateStore().QueryContractStates(ctx, "simpleStorageDomain", *contractAddress, aliceSchemas[0].ID, &query.QueryJSON{}, "available")

	require.NoError(t, err)
	require.Len(t, aliceStates, 1)
	assert.Equal(t, bobStates[0].ID, aliceStates[0].ID)
	assert.Equal(t, bobStates[0].Data.Bytes(), aliceStates[0].Data.Bytes())

}

func TestPrivacyGroupEndorsementConcurrent(t *testing.T) {
	// This test is identical to TestPrivacyGroupEndorsement except that it sends the transactions concurrently
	// For manual exploratory testing of longevity , it is possible to increase the number of iterations and the test should still be valid
	// however, it is hard coded to a small number by default so that it can be run in CI
	NUM_ITERATIONS := 2
	NUM_TRANSACTIONS_PER_NODE_PER_ITERATION := 2
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "node1")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	carol := testutils.NewPartyForTesting(t, "carol", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig(), carol.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig(), carol.GetNodeConfig())
	carol.AddPeer(alice.GetNodeConfig(), bob.GetNodeConfig())

	domainConfig := &domains.SimpleStorageDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}
	startNode(t, alice, "node1", domainConfig)
	startNode(t, bob, "node2", domainConfig)
	startNode(t, carol, "node3", domainConfig)

	endorsementSet := []string{alice.GetIdentityLocator(), bob.GetIdentityLocator(), carol.GetIdentityLocator()}

	constructorParameters := &domains.SimpleStorageConstructorParameters{
		EndorsementSet:  endorsementSet,
		Name:            "SimpleStorage1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
	}
	// send JSON RPC message to node 1 to deploy a private contract
	contractAddress := alice.DeploySimpleStorageDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)
	initTx := alice.GetClient().ForABI(ctx, *domains.SimpleStorageInitABI()).
		Private().
		Domain("simpleStorageDomain").
		IdempotencyKey("init-tx").
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("init").
		Inputs(pldtypes.RawJSON(`{
                    "map":"TestPrivacyGroupEndorsementConcurrent"
                }`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, initTx.Error())

	//initialize a map that all parties should be able to access concurrently
	// we wait for the confirmation of this transaction to ensure that there is no race condition of someone trying to call `set` before the map is initialized
	// TODO - so long as we have the transaction id for the init transaction, we could declare a dependency on it for the set transactions
	for i := 0; i < NUM_ITERATIONS; i++ {
		// Start a number of private transaction on alice's node
		// this should require endorsement from bob and carol
		aliceTxs := make([]pldclient.SentTransaction, NUM_TRANSACTIONS_PER_NODE_PER_ITERATION)
		bobTxs := make([]pldclient.SentTransaction, NUM_TRANSACTIONS_PER_NODE_PER_ITERATION)
		carolTxs := make([]pldclient.SentTransaction, NUM_TRANSACTIONS_PER_NODE_PER_ITERATION)

		for j := 0; j < NUM_TRANSACTIONS_PER_NODE_PER_ITERATION; j++ {
			aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleStorageSetABI()).
				Private().
				Domain("simpleStorageDomain").
				IdempotencyKey(fmt.Sprintf("tx1-alice-%d-%d", i, j)).
				From(alice.GetIdentity()).
				To(contractAddress).
				Function("set").
				Inputs(pldtypes.RawJSON(fmt.Sprintf(`{
				 	"map":"TestPrivacyGroupEndorsementConcurrent",
                    "key": "alice_key_%d_%d",
					"value": "alice_value_%d_%d"
                }`, i, j, i, j))).
				Send()
			require.NoError(t, aliceTx.Error())
			aliceTxs[j] = aliceTx

			// Start a private transaction on bob's node
			// this should require endorsement from alice and carol
			bobTx := bob.GetClient().ForABI(ctx, *domains.SimpleStorageSetABI()).
				Private().
				Domain("simpleStorageDomain").
				IdempotencyKey(fmt.Sprintf("tx1-bob-%d-%d", i, j)).
				From(bob.GetIdentity()).
				To(contractAddress).
				Function("set").
				Inputs(pldtypes.RawJSON(fmt.Sprintf(`{
				 	"map":"TestPrivacyGroupEndorsementConcurrent",
                    "key": "bob_key_%d_%d",
					"value": "bob_value_%d_%d"
                }`, i, j, i, j))).
				Send()
			require.NoError(t, bobTx.Error())
			bobTxs[j] = bobTx

			carolTx := carol.GetClient().ForABI(ctx, *domains.SimpleStorageSetABI()).
				Private().
				Domain("simpleStorageDomain").
				IdempotencyKey(fmt.Sprintf("tx1-carol-%d-%d", i, j)).
				From(bob.GetIdentity()).
				To(contractAddress).
				Function("set").
				Inputs(pldtypes.RawJSON(fmt.Sprintf(`{
				 	"map":"TestPrivacyGroupEndorsementConcurrent",
                    "key": "carol_key_%d_%d",
					"value": "carol_value_%d_%d"
                }`, i, j, i, j))).
				Send()
			require.NoError(t, carolTx.Error())
			carolTxs[j] = carolTx
		}

		//once all transactions for this iteration are sent, wait for all of them to be confirmed before starting the next iteration
		for j := 0; j < NUM_TRANSACTIONS_PER_NODE_PER_ITERATION; j++ {
			aliceTxResult := aliceTxs[j].Wait(transactionLatencyThreshold(t))
			require.NoError(t, aliceTxResult.Error())
			bobTxResult := bobTxs[j].Wait(transactionLatencyThreshold(t))
			require.NoError(t, bobTxResult.Error())
			carolTxResult := carolTxs[j].Wait(transactionLatencyThreshold(t))
			require.NoError(t, carolTxResult.Error())
		}
	}

	var schemas []*pldapi.Schema
	var err error

	schemas, err = alice.GetClient().StateStore().ListSchemas(ctx, "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, schemas, 1)

	schemas, err = bob.GetClient().StateStore().ListSchemas(ctx, "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, schemas, 1)

	schemas, err = carol.GetClient().StateStore().ListSchemas(ctx, "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, schemas, 1)

	aliceStates, err := alice.GetClient().StateStore().QueryContractStates(ctx, "simpleStorageDomain", *contractAddress, schemas[0].ID, &query.QueryJSON{}, "available")
	require.NoError(t, err)
	require.Len(t, aliceStates, 1)

	bobStates, err := bob.GetClient().StateStore().QueryContractStates(ctx, "simpleStorageDomain", *contractAddress, schemas[0].ID, &query.QueryJSON{}, "available")
	require.NoError(t, err)
	require.Len(t, bobStates, 1)
	assert.Equal(t, aliceStates[0].Data, bobStates[0].Data)

	carolStates, err := carol.GetClient().StateStore().QueryContractStates(ctx, "simpleStorageDomain", *contractAddress, schemas[0].ID, &query.QueryJSON{}, "available")
	require.NoError(t, err)
	require.Len(t, carolStates, 1)
	assert.Equal(t, aliceStates[0].Data, carolStates[0].Data)

	stateData := make(map[string]string)
	storage := make(map[string]string)
	jsonErr := json.Unmarshal(aliceStates[0].Data.Bytes(), &stateData)
	require.NoError(t, jsonErr)

	jsonErr = json.Unmarshal([]byte(stateData["records"]), &storage)
	require.NoError(t, jsonErr)

	for i := 0; i < NUM_ITERATIONS; i++ {
		for j := 0; j < NUM_TRANSACTIONS_PER_NODE_PER_ITERATION; j++ {
			assert.Equal(t, fmt.Sprintf("alice_value_%d_%d", i, j), storage[fmt.Sprintf("alice_key_%d_%d", i, j)])
			assert.Equal(t, fmt.Sprintf("bob_value_%d_%d", i, j), storage[fmt.Sprintf("bob_key_%d_%d", i, j)])
			assert.Equal(t, fmt.Sprintf("carol_value_%d_%d", i, j), storage[fmt.Sprintf("carol_key_%d_%d", i, j)])
		}
	}
}
