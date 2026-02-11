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

/*
Test Kata component with no mocking of any internal units.
Starts the GRPC server and drives the internal functions via GRPC messages
*/
package coordinationtest

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	testutils "github.com/LFDT-Paladin/paladin/core/noderuntests/pkg"
	"github.com/LFDT-Paladin/paladin/core/noderuntests/pkg/domains"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Map of node names to config paths. Each node needs its own DB and static signing key
var CONFIG_PATHS = map[string]string{
	"alice": "./config/postgres.coordinationtest.alice.config.yaml",
	"bob":   "./config/postgres.coordinationtest.bob.config.yaml",
	"carol": "./config/postgres.coordinationtest.carol.config.yaml",
}

func deployDomainRegistry(t *testing.T, nodeName string) *pldtypes.EthAddress {
	return testutils.DeployDomainRegistry(t, CONFIG_PATHS[nodeName])
}

func startNode(t *testing.T, party testutils.Party, domainConfig interface{}) {
	party.Start(t, domainConfig, CONFIG_PATHS[party.GetName()], true)
}

func stopNode(t *testing.T, party testutils.Party) {
	party.Stop(t)
}

func TestTransactionSuccessPrivacyGroupEndorsement(t *testing.T) {
	// Test a regular privacy group endorsement transaction
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator()},
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
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

	// Check alice has the TX including the public TX information
	assert.Eventually(t,
		transactionReceiptConditionExpectedPublicTXCount(t, ctx, aliceTx.ID(), alice.GetClient(), 1),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt with 1 public TX",
	)
	// Check bob has the public TX info as well
	assert.Eventually(t,
		transactionReceiptFullConditionExpectedPublicTXCount(t, ctx, aliceTx.ID(), bob.GetClient(), 1),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt with 1 public TX",
	)

	// Check Alice and Bob both have the same view of the world
	aliceTxFull, err := alice.GetClient().PTX().GetTransactionFull(ctx, aliceTx.ID())
	require.NoError(t, err)
	require.NotNil(t, aliceTxFull)

	bobTxFull, err := bob.GetClient().PTX().GetTransactionReceiptFull(ctx, aliceTx.ID())
	require.NoError(t, err)
	require.NotNil(t, bobTxFull)

	// Check the data both nodes have is consistent. We're comparing a transaction with a transaction receipt so domain is the only comparable field
	assert.Equal(t, aliceTxFull.Domain, bobTxFull.Domain)

	// Check the public transaction records are consistent
	assert.Equal(t, aliceTxFull.Public[0].Dispatcher, bobTxFull.Public[0].Dispatcher)
	assert.Equal(t, aliceTxFull.Public[0].TransactionHash, bobTxFull.Public[0].TransactionHash)
	assert.Equal(t, aliceTxFull.Public[0].From, bobTxFull.Public[0].From)
	assert.Equal(t, aliceTxFull.Public[0].To, bobTxFull.Public[0].To)
	assert.Equal(t, aliceTxFull.Public[0].Value, bobTxFull.Public[0].Value)
	assert.Equal(t, aliceTxFull.Public[0].Gas, bobTxFull.Public[0].Gas)
	assert.Equal(t, aliceTxFull.Public[0].Nonce, bobTxFull.Public[0].Nonce)
	assert.Equal(t, aliceTxFull.Public[0].Data, bobTxFull.Public[0].Data)
	assert.Equal(t, aliceTxFull.Public[0].Created, bobTxFull.Public[0].Created)
	assert.Equal(t, aliceTxFull.Public[0].PublicTxOptions, bobTxFull.Public[0].PublicTxOptions)

	// Check the public transaction submissions are consistent
	assert.True(t, len(aliceTxFull.Public[0].Submissions) == 1)
	assert.Equal(t, aliceTxFull.Public[0].Submissions[0].TransactionHash, bobTxFull.Public[0].Submissions[0].TransactionHash)
	assert.Equal(t, aliceTxFull.Public[0].Submissions[0].Time, bobTxFull.Public[0].Submissions[0].Time)
	assert.Equal(t, aliceTxFull.Public[0].Submissions[0].PublicTxGasPricing.MaxPriorityFeePerGas, bobTxFull.Public[0].Submissions[0].PublicTxGasPricing.MaxPriorityFeePerGas)
	assert.Equal(t, aliceTxFull.Public[0].Submissions[0].PublicTxGasPricing.MaxFeePerGas, bobTxFull.Public[0].Submissions[0].PublicTxGasPricing.MaxFeePerGas)

	// Check Alice has the sequencing activity Bob has distributed to her
	assert.True(t, len(aliceTxFull.SequencerActivity) == 1)
	assert.Equal(t, aliceTxFull.SequencerActivity[0].ActivityType, string(pldapi.SequencerActivityType_Dispatch)) // Only 1 activity type supported currently
	assert.Equal(t, aliceTxFull.SequencerActivity[0].SequencingNode, bob.GetName())
	assert.Equal(t, aliceTxFull.SequencerActivity[0].TransactionID, aliceTx.ID())

	// Check Bob has the dispatch
	assert.True(t, len(bobTxFull.Dispatches) == 1)
}

func TestTransactionSuccessAfterStartStopSingleNode(t *testing.T) {
	// We want to test that we can start some nodes, send a transaction, restart the nodes and send some more transactions

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
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

	// Start a private transaction on bob's node
	// This is a transfer which relies on bob's node being aware of the state created by alice's mint to bob above
	bobTx1 := bob.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-bob-" + uuid.New().String()).
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

	stopNode(t, alice)

	verifierResult, err := bob.GetClient().PTX().ResolveVerifier(ctx, bob.GetIdentityLocator(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	require.NotEmpty(t, verifierResult)

	_, err = alice.GetClient().PTX().ResolveVerifier(ctx, bob.GetIdentityLocator(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Error(t, err)

	startNode(t, alice, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
	})

	verifierResult, err = alice.GetClient().PTX().ResolveVerifier(ctx, alice.GetIdentityLocator(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	require.NotEmpty(t, verifierResult)

	verifierResult, err = alice.GetClient().PTX().ResolveVerifier(ctx, bob.GetIdentityLocator(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	require.NotEmpty(t, verifierResult)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	aliceTx = alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx2-alice-" + uuid.New().String()).
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
}

func TestTransactionSuccessIfOneNodeStoppedButNotARequiredVerifier(t *testing.T) {
	// Test that we can start 2 nodes, then submit a transaction while one of them is stopped.
	// The  node that is stopped is not a required verifier so the transaction should succeed
	// without restarting that node.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
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

	// Stop alice's node before submitting a transaction request to bob's node.
	stopNode(t, alice)

	// Start a private transaction on bob's node, TO bob's identifier. Alice isn't involved at all so isn't a required verifier
	bobTx1 := bob.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-bob-" + uuid.New().String()).
		From(bob.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "` + bob.GetIdentityLocator() + `",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	// Check that even though alice's node is stopped, since it is not a required verifier
	// the transaction should succeed.
	require.NoError(t, bobTx1.Error())
}

func TestTransactionSuccessIfOneRequiredVerifierStoppedDuringSubmission(t *testing.T) {
	// Test that we can start 2 nodes, stop one of them, then submit a transaction where both nodes
	// are required verifiers. While one node is offline we shouldn't get a receipt. After the node
	// is restarted the transaction should proceed to completion.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	sequencerConfig := pldconf.SequencerDefaults
	sequencerConfig.AssembleTimeout = confutil.P("60s") // In this test we don't want to hit this
	sequencerConfig.RequestTimeout = confutil.P("10s")  // Extend this enough to give the bob node enough time to restart
	alice.OverrideSequencerConfig(&sequencerConfig)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
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

	// Stop alice's node before submitting a transaction request to bob's node.
	stopNode(t, alice)

	// Start a private transaction on bob's node, TO alice's identifier. This can't proceed while her node is stopped.
	bobTx1 := bob.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-bob-" + uuid.New().String()).
		From(bob.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "` + bob.GetIdentityLocator() + `",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send()
	require.NoError(t, bobTx1.Error())

	// Check that we don't receive a receipt in the usual time while alice's node is offline
	result := bobTx1.Wait(transactionLatencyThreshold(t))
	require.ErrorContains(t, result.Error(), "timed out")

	startNode(t, alice, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
	})

	// Check that we did receive a receipt once alice's node was restarted
	customThreshold := 15 * time.Second
	result = bobTx1.Wait(transactionLatencyThresholdCustom(t, &customThreshold))
	require.NoError(t, result.Error())
}

func TestTransactionSuccessIfOneRequiredVerifierStoppedLongerThanRequestTimeout(t *testing.T) {
	// Test that we can start 2 nodes, stop one of them, then submit a transaction where both nodes
	// are required verifiers. While one node is offline we shouldn't get a receipt. After the node
	// is restarted the transaction should proceed to completion.

	// This test is identical to TestTransactionSuccessIfOneRequiredVerifierStoppedDuringSubmission but
	// intentionally waits longer than RequestTimeout before restarting the node. This exercises AssembleTimeout
	// separately.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	sequencerConfig := pldconf.SequencerDefaults
	sequencerConfig.RequestTimeout = confutil.P("1s")   // In this test we don't want to rely on request timeout so make sure it fires before the bob node is restarted
	sequencerConfig.AssembleTimeout = confutil.P("10s") // In this test we want to ensure assemble timeout causes the transaction to be re-pooled and re-assembled
	alice.OverrideSequencerConfig(&sequencerConfig)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
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

	// Stop alice's node before submitting a transaction request to bob's node.
	stopNode(t, alice)

	// Start a private transaction on bob's node, TO alice's identifier. This can't proceed while her node is stopped.
	bobTx1 := bob.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-bob-" + uuid.New().String()).
		From(bob.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "` + bob.GetIdentityLocator() + `",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send()
	require.NoError(t, bobTx1.Error())

	// Check that we don't receive a receipt in the usual time while alice's node is offline
	result := bobTx1.Wait(transactionLatencyThreshold(t))
	require.ErrorContains(t, result.Error(), "timed out")

	startNode(t, alice, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
	})

	// Check that we did receive a receipt once alice's node was restarted
	customThreshold := 15 * time.Second
	result = bobTx1.Wait(transactionLatencyThresholdCustom(t, &customThreshold))
	require.NoError(t, result.Error())
}

func TestTransactionResumesIfBothRequiredVerifiersAreStoppedBeforeCompletion(t *testing.T) {
	// Test that we can start 2 nodes, stop one of them, then submit a transaction where both nodes
	// are required verifiers. While one node is offline we shouldn't get a receipt. We then stop
	// the remaining node so there are no active nodes. On restarting both, one should resume coordination
	// and the transaction should be successful.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
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

	// Stop alice's node before submitting a transaction request to bob's node.
	stopNode(t, alice)

	// Start a private transaction on bob's node, TO alice's identifier. This can't proceed while her node is stopped.
	bobTx1 := bob.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-bob-" + uuid.New().String()).
		From(bob.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "` + bob.GetIdentityLocator() + `",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send()
	require.NoError(t, bobTx1.Error())

	// Check that we don't receive a receipt in the usual time while alice's node is offline
	result := bobTx1.Wait(transactionLatencyThreshold(t))
	require.ErrorContains(t, result.Error(), "timed out")

	// Now stop bob's node as well.
	stopNode(t, bob)

	// Restart both nodes
	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	// Check that we did receive a receipt once the nodes restarted
	// We can't use Wait as the client in the SentTransaction is for the previous instance of the running node
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, *bobTx1.ID(), bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionSuccessChainedTransaction(t *testing.T) {

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	// Deploy a token that will be call as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.SelfEndorsement,
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(chainedContractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, aliceTx.Error())

	// Bob's node has the receipt
	assert.Eventually(t,
		transactionReceiptConditionReceiptOnly(t, ctx, aliceTx.ID(), bob.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionSuccessChainedTransactionSelfEndorsementThenPrivacyGroupEndorsement(t *testing.T) {

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
	}

	// Deploy a token that will be called as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.SelfEndorsement,
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(chainedContractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, aliceTx.Error())

	// Alice's node should have the full transaction as well as the receipt that Wait checks for
	_, err := alice.GetClient().PTX().GetTransactionFull(ctx, aliceTx.ID())
	require.NoError(t, err)

	// Bob's node has the receipt, but not necesarily the original transaction
	assert.Eventually(t,
		transactionReceiptConditionReceiptOnly(t, ctx, aliceTx.ID(), bob.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Get the full transaction from Alice and check there is a chained transaction created on Alice's node
	aliceTxFull, err := alice.GetClient().PTX().GetTransactionFull(ctx, aliceTx.ID())
	require.NoError(t, err)
	require.NotNil(t, aliceTxFull)

	assert.True(t, len(aliceTxFull.ChainedPrivateTransactions) == 1)
	assert.Equal(t, aliceTxFull.ChainedPrivateTransactions[0].TransactionID, aliceTx.ID().String())

	// Now query the chained transaction on Alice's node, which should have sequencing activity sent from Bob, the coordinator
	aliceChainedTxFull, err := alice.GetClient().PTX().GetTransactionFull(ctx, uuid.MustParse(aliceTxFull.ChainedPrivateTransactions[0].ChainedTransactionID))
	require.NoError(t, err)
	require.NotNil(t, aliceChainedTxFull)

	assert.True(t, len(aliceChainedTxFull.SequencerActivity) == 1)
	assert.Equal(t, aliceChainedTxFull.SequencerActivity[0].SequencingNode, bob.GetName())
	assert.Equal(t, aliceChainedTxFull.SequencerActivity[0].TransactionID.String(), aliceTxFull.ChainedPrivateTransactions[0].ChainedTransactionID)
	assert.Equal(t, aliceChainedTxFull.SequencerActivity[0].ActivityType, string(pldapi.SequencerActivityType_Dispatch))

	// Finally check that Bob who coordinated the chained transaction has a receipt with dispatch information that correlates with Alice's sequencing activity
	bobChainedTxReceiptFull, err := bob.GetClient().PTX().GetTransactionReceiptFull(ctx, uuid.MustParse(aliceTxFull.ChainedPrivateTransactions[0].ChainedTransactionID))
	require.NoError(t, err)
	require.NotNil(t, bobChainedTxReceiptFull)

	assert.True(t, len(bobChainedTxReceiptFull.Dispatches) == 1)
	assert.Equal(t, bobChainedTxReceiptFull.Dispatches[0].ID, aliceChainedTxFull.SequencerActivity[0].SubjectID)
	assert.Equal(t, bobChainedTxReceiptFull.Dispatches[0].PrivateTransactionID, aliceTxFull.ChainedPrivateTransactions[0].ChainedTransactionID)
}

func TestTransactionSuccessChainedTransactionPrivacyGroupEndorsementThenSelfEndorsement(t *testing.T) {

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	// Deploy a token that will be call as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(chainedContractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, aliceTx.Error())

	// Alice's node should have the full transaction as well as the receipt that Wait checks for
	_, err := alice.GetClient().PTX().GetTransactionFull(ctx, aliceTx.ID())
	require.NoError(t, err)

	// Bob's node has the receipt only
	assert.Eventually(t,
		transactionReceiptConditionReceiptOnly(t, ctx, aliceTx.ID(), bob.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Now query the transaction in full and check that there is a sequencing activity record from bob who coordinated the original tranasction
	aliceTxFull, err := alice.GetClient().PTX().GetTransactionFull(ctx, aliceTx.ID())
	require.NoError(t, err)
	require.NotNil(t, aliceTxFull)

	assert.True(t, len(aliceTxFull.SequencerActivity) == 1)
	assert.Equal(t, aliceTxFull.SequencerActivity[0].SequencingNode, bob.GetName())
	assert.Equal(t, aliceTxFull.SequencerActivity[0].TransactionID.String(), aliceTx.ID().String())
	assert.Equal(t, aliceTxFull.SequencerActivity[0].ActivityType, string(pldapi.SequencerActivityType_ChainedDispatch)) // The coordination resulted in a chained transaction, not a public dispatch

	// Query the transaction receipt on bob's node, to get the chained transaction and check it correlates with Alice's sequencing activity
	bobTxReceiptFull, err := bob.GetClient().PTX().GetTransactionReceiptFull(ctx, aliceTx.ID())
	require.NoError(t, err)
	require.NotNil(t, bobTxReceiptFull)

	assert.True(t, len(bobTxReceiptFull.ChainedPrivateTransactions) == 1)
	assert.Equal(t, bobTxReceiptFull.ChainedPrivateTransactions[0].TransactionID, aliceTx.ID().String())
	assert.Equal(t, bobTxReceiptFull.ChainedPrivateTransactions[0].LocalID, aliceTxFull.SequencerActivity[0].SubjectID)

	// Finally query Bob for the full chained transaction. It is coordinated by Bob so should have public dispatch, but not sequencing activity
	bobChainedTxFull, err := bob.GetClient().PTX().GetTransactionFull(ctx, uuid.MustParse(bobTxReceiptFull.ChainedPrivateTransactions[0].ChainedTransactionID))
	require.NoError(t, err)
	require.NotNil(t, bobChainedTxFull)

	assert.True(t, len(bobChainedTxFull.Dispatches) == 1)
	assert.Equal(t, bobChainedTxFull.Dispatches[0].PrivateTransactionID, bobTxReceiptFull.ChainedPrivateTransactions[0].ChainedTransactionID)
}

func TestTransactionSuccessChainedTransactionPrivacyGroupEndorsementThenPrivacyGroupEndorsement(t *testing.T) {

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
	}

	// Deploy a token that will be call as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(chainedContractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, aliceTx.Error())

	// Alice's node should have the full transaction as well as the receipt that Wait checks for
	_, err := alice.GetClient().PTX().GetTransactionFull(ctx, aliceTx.ID())
	require.NoError(t, err)

	// Bob's node has the full transaction and receipt
	assert.Eventually(t,
		transactionReceiptConditionReceiptOnly(t, ctx, aliceTx.ID(), bob.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionRevertDuringAssembly(t *testing.T) {
	// Test that we can start 2 nodes, then submit a transaction while one of them is stopped.
	// The  node that is stopped is not a required verifier so the transaction should succeed
	// without restarting that node.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "1001"
		}`)). // Special value 1001 in the simple domain causes revert at assembly time
		Send().Wait(transactionLatencyThreshold(t))

	require.Error(t, aliceTx.Error())
	require.NotNil(t, aliceTx.Receipt())
	require.False(t, aliceTx.Receipt().Success)
}

func TestTransactionRevertDuringEndorsement(t *testing.T) {
	// Test that a transaction which reverts at endorsement time is still successful
	// due to the transaction being re-assembled and then successfully endorsed.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "1002"
		}`)). // Special value 1002 in the simple domain causes revert at endorsement time
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, aliceTx.Error())
}

func TestTransactionRevertOnBaseLedger(t *testing.T) {
	// Test that we can start 2 nodes, then submit a transaction while one of them is stopped.
	// The  node that is stopped is not a required verifier so the transaction should succeed
	// without restarting that node.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	customDuration := 5 * time.Second

	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "1003"
		}`)). // Special value 1003 in the simple domain causes revert once on the base ledger, then subsequently be successful
		Send().Wait(transactionLatencyThresholdCustom(t, &customDuration))
	require.NoError(t, aliceTx.Error())

	txFull, err := alice.GetClient().PTX().GetTransactionFull(ctx, aliceTx.ID())
	require.NoError(t, err)
	assert.Len(t, txFull.Public, 2)
}

func TestTransactionSuccessChainedTransactionStopNodesBeforeCompletion(t *testing.T) {

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	carol := testutils.NewPartyForTesting(t, "carol", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	alice.AddPeer(carol.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())
	bob.AddPeer(carol.GetNodeConfig())
	carol.AddPeer(alice.GetNodeConfig())
	carol.AddPeer(bob.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	// Re-delegation happens on an interval to catch the case where node A resumes a TX but the initial fire-and-forget delegate fails
	// because node B is still coming up. If nothing else happens on the contract there's nothing to nudge re-delegation except the delegate timeout.
	// Reduce it down a little here to speed up the test.
	sequencerConfig := pldconf.SequencerDefaults
	sequencerConfig.DelegateTimeout = confutil.P("2s")

	alice.OverrideSequencerConfig(&sequencerConfig)
	bob.OverrideSequencerConfig(&sequencerConfig)
	carol.OverrideSequencerConfig(&sequencerConfig)

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	startNode(t, carol, domainConfig)

	privacyGroupConstructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator(), carol.GetIdentityLocator()},
	}

	// Deploy a token that will be called as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, privacyGroupConstructorParameters, transactionLatencyThreshold)

	notaryConstructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		Notary:          bob.GetIdentityLocator(),
		EndorsementMode: domains.NotaryEndorsement,
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := bob.DeploySimpleDomainInstanceContract(t, notaryConstructorParameters, transactionLatencyThreshold)

	// Stop Carol's node. She is required in order to endorse the hook transaction, so we are forcing the original and the chained transactions to be
	// unable to complete initially.
	stopNode(t, carol)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(chainedContractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.ErrorContains(t, aliceTx.Error(), "timed out")

	// Now we want to stop the world. This exercises 2 code paths:
	// 1. The originator of the first transaction resuming their transaction
	// 2. The originator of the second (chained) transaction resuming their transaction
	stopNode(t, bob)
	stopNode(t, alice)

	// Wait a mo to ensure shutdown has finished
	time.Sleep(2 * time.Second)

	// Restart the nodes (order is important)
	// Starting bob ensures that when alice is restarted, she is successful in re-delegating to bob.
	// The other way round, typically what happens is alice attempts to delegate first but bob's gRPC
	// interface isn't ready so we don't actually get a delegation request on bob, which we are specifically
	// wanting to exercise.
	startNode(t, bob, domainConfig)
	startNode(t, alice, domainConfig)
	startNode(t, carol, domainConfig)

	t.Cleanup(func() {
		stopNode(t, carol)
		stopNode(t, bob)
		stopNode(t, alice)
	})

	customDuration := 10 * time.Second
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTx.ID(), alice.GetClient(), false),
		transactionLatencyThresholdCustom(t, &customDuration),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionFailureWhenChainedTransactionAssembleReverts(t *testing.T) {
	// Test that a chained transaction failure percolates back to the original transaction.

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	// Deploy a token that will be call as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.SelfEndorsement,
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(chainedContractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "1001"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.Error(t, aliceTx.Error())

	// Alices's node has the failure receipt for the original transaction
	assert.Eventually(t,
		transactionReceiptConditionFailureReceiptOnly(t, ctx, aliceTx.ID(), alice.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Alice's node also has the failure receipt for the chained transaction, which we can query by idempotency key
	chainedTxIdempotencyKey := fmt.Sprintf("%s_transfer", aliceTx.ID().String())
	receiptLimit := 1
	alicesChainedTransaction, err := alice.GetClient().PTX().QueryTransactionsFull(ctx, &query.QueryJSON{
		Limit: &receiptLimit,
		Statements: query.Statements{
			Ops: query.Ops{
				Equal: []*query.OpSingleVal{
					{
						Op: query.Op{
							Field: "idempotencyKey",
						},
						Value: pldtypes.JSONString(chainedTxIdempotencyKey),
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, alicesChainedTransaction, 1)
	require.True(t, alicesChainedTransaction[0].Receipt.Success == false)
}

func TestTransactionFailureChainedTransactionDifferentOriginators(t *testing.T) {
	// Test that a chained transaction failure percolates back to the original transaction.
	// Specifically, tests the case where the originator of the original TX is different from
	// the originator of the chained TX.

	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
	}

	// Deploy a token that will be called as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(chainedContractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "1001"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.Error(t, aliceTx.Error())

	// Alices's node has the failure receipt for the original transaction
	assert.Eventually(t,
		transactionReceiptConditionFailureReceiptOnly(t, ctx, aliceTx.ID(), alice.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Bob's node has the failure receipt for the chained transaction, which we can query by idempotency key
	bobsTXIdempotencyKey := fmt.Sprintf("%s_transfer", aliceTx.ID().String())
	receiptLimit := 1
	bobsChainedTransaction, err := bob.GetClient().PTX().QueryTransactionsFull(ctx, &query.QueryJSON{
		Limit: &receiptLimit,
		Statements: query.Statements{
			Ops: query.Ops{
				Equal: []*query.OpSingleVal{
					{
						Op: query.Op{
							Field: "idempotencyKey",
						},
						Value: pldtypes.JSONString(bobsTXIdempotencyKey),
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, bobsChainedTransaction, 1)
	require.True(t, bobsChainedTransaction[0].Receipt.Success == false)
}

func TestTransactionSuccessMultipleConcurrentPrivacyGroupEndorsement(t *testing.T) {
	// This test exercises the re-assembly and re-dispatch of transactions who's base ledger
	// transactions revert. The simple storage domain base ledger contract has an option to
	// ensure the value stored is prev+1. For out-of-sequence delivery where new signing addresses
	// are used for each Paladin public TX it is possible (likely) for the base ledger transactions to
	// revert. This test drops 30 transactions in and expects every one to be successful, knowing that
	// several of them are likely to have at least 1 base ledger revert before being successful.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")
	numberOfIterations := 30

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
		AmountVisible:   true,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionLatencyThreshold)

	// Submit a number of transactions that are likely to hit on-chain reverts but must all be eventually successful.
	aliceTxns := make([]*uuid.UUID, numberOfIterations)
	for i := 0; i < numberOfIterations; i++ {
		aliceTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
			Private().
			Domain("domain1").
			IdempotencyKey("tx1-alice-" + uuid.New().String()).
			From(alice.GetIdentity()).
			To(contractAddress).
			Function("transfer").
			Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "` + strconv.Itoa(i+1) + `"
		}`)).
			Send()
		require.NoError(t, aliceTx.Error())
		aliceTxns[i] = aliceTx.ID()

	}

	// Check all transactions are eventually successful
	for i := 0; i < numberOfIterations; i++ {
		assert.Eventually(t,
			transactionReceiptCondition(t, ctx, *aliceTxns[i], alice.GetClient(), false),
			transactionLatencyThreshold(t),
			100*time.Millisecond,
			"Transaction did not receive a receipt for Alice TX %s", aliceTxns[i])
	}
}

func TestTransactionWaitsUntilExplicitPrereqTransactionSuccessful(t *testing.T) {
	// Test that a transaction with an explicit dependency doesn't complete until the dependency has.
	// We test this with 2 contracts: one requires alice and bob to endorse, the other just requires alice.
	// We stop the bob node so TX 1 can't complete, then we submit TX 2. Even though TX 2 only requires
	// alice to endorse it shouldn't complete until we restart bob's node and TX 1 goes through.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
	})

	constructorParameters1 := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
	}

	constructorParameters2 := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	// Deploy 2 contracts, testing that a TX of one domain instance can have a dependency on a TX of the other domain instance
	contractAddress1 := alice.DeploySimpleDomainInstanceContract(t, constructorParameters1, transactionLatencyThreshold)
	contractAddress2 := alice.DeploySimpleDomainInstanceContract(t, constructorParameters2, transactionLatencyThreshold)

	// Stop bob's node
	stopNode(t, bob)

	// Start a private transaction on alice's node
	// This requires both nodes to be up because they are both endorsers of contract 1. Having stopped bob node already
	// this TX cannot currently proceed.
	aliceTx1 := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress1).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send()
	require.NoError(t, aliceTx1.Error())

	// Start a private transaction on alice's node
	// This only requires Alice's not to endorse, but it has an explicit dependency on TX1 so must not be successful yet
	aliceTx2 := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		DependsOn([]uuid.UUID{*aliceTx1.ID()}). // This TX depends on TX1 and must wait for it to complete before being processed
		IdempotencyKey("tx2-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress2).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send()
	require.NoError(t, aliceTx2.Error())

	// Check that we don't receive a receipt for either transaction
	result1 := aliceTx1.Wait(transactionLatencyThreshold(t))
	require.ErrorContains(t, result1.Error(), "timed out")
	result2 := aliceTx2.Wait(transactionLatencyThreshold(t))
	require.ErrorContains(t, result2.Error(), "timed out")

	// Restarting Bob's node should allow both transactions to go through
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, bob)
	})

	// Check that we then get a receipt for both
	customThreshold := 15 * time.Second
	result1 = aliceTx1.Wait(transactionLatencyThresholdCustom(t, &customThreshold))
	require.NoError(t, result1.Error())
	result2 = aliceTx2.Wait(transactionLatencyThresholdCustom(t, &customThreshold))
	require.NoError(t, result2.Error())
}

func TestTransactionWithExplicitPrereqSuccessfulAfterRestart(t *testing.T) {
	// Test that a transaction with an explicit dependency doesn't complete until the dependency has.
	// We test this with 2 contracts: one requires alice and bob to endorse, the other just requires alice.
	// We stop the bob node so TX 1 can't complete, then we submit TX 2. Even though TX 2 only requires
	// alice to endorse it shouldn't complete until we restart bob's node and TX 1 goes through.
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	carol := testutils.NewPartyForTesting(t, "carol", domainRegistryAddress)

	sequencerConfig := pldconf.SequencerDefaults
	sequencerConfig.AssembleTimeout = confutil.P("10s")
	sequencerConfig.RequestTimeout = confutil.P("3s")
	sequencerConfig.TransactionResumePollInterval = confutil.P("5s") // We're relying on sequencer TX resume to get TX2 through to completion
	alice.OverrideSequencerConfig(&sequencerConfig)

	alice.AddPeer(bob.GetNodeConfig())
	alice.AddPeer(carol.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())
	bob.AddPeer(carol.GetNodeConfig())
	carol.AddPeer(alice.GetNodeConfig())
	carol.AddPeer(bob.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	startNode(t, carol, domainConfig)

	constructorParameters1 := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
	}

	constructorParameters2 := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), carol.GetIdentityLocator()},
	}

	// Deploy 2 contracts, testing that a TX of one domain instance can have a dependency on a TX of the other domain instance
	contractAddress1 := alice.DeploySimpleDomainInstanceContract(t, constructorParameters1, transactionLatencyThreshold)
	contractAddress2 := alice.DeploySimpleDomainInstanceContract(t, constructorParameters2, transactionLatencyThreshold)

	// Stop carols's node
	stopNode(t, carol)

	// Start a private transaction on alice's node
	// This requires both nodes to be up because they are both endorsers of contract 1. Having stopped bob node already
	// this TX cannot currently proceed.
	aliceTx1 := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress1).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send()
	require.NoError(t, aliceTx1.Error())

	// Start a private transaction on alice's node
	// This only requires Alice's not to endorse, but it has an explicit dependency on TX1 so must not be successful yet
	aliceTx2 := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		DependsOn([]uuid.UUID{*aliceTx1.ID()}). // This TX depends on TX1 and must wait for it to complete before being processed
		IdempotencyKey("tx2-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress2).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send()
	require.NoError(t, aliceTx2.Error())

	// Check that we receive a receipt for TX1 but not TX2
	assert.Eventually(t,
		transactionReceiptConditionReceiptOnly(t, ctx, *aliceTx1.ID(), alice.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt or result was incorrect",
	)
	result2 := aliceTx2.Wait(transactionLatencyThreshold(t))
	require.ErrorContains(t, result2.Error(), "timed out")

	// Stop all remaining nodes
	stopNode(t, alice)
	stopNode(t, bob)

	// Wait a mo
	time.Sleep(1 * time.Second)

	// Restarting Alice and Carol's nodes should allow TX 2 to be successful, because TX 1 completed before we stopped the nodes
	// so there is no dependency blocking TX2
	startNode(t, carol, domainConfig)
	startNode(t, bob, domainConfig)
	startNode(t, alice, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
		stopNode(t, carol)
	})

	// Check that we then get a receipt for TX2
	customThreshold := 20 * time.Second
	assert.Eventually(t,
		transactionReceiptConditionReceiptOnly(t, ctx, *aliceTx2.ID(), alice.GetClient()),
		transactionLatencyThresholdCustom(t, &customThreshold),
		100*time.Millisecond,
		"Transaction did not receive a receipt or result was incorrect",
	)
}

func TestTransactionFailsIfExplicitPrereqTransactionFails(t *testing.T) {
	// Test that a transaction with an explicit dependency fails if that dependency reverts
	ctx := t.Context()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters2 := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	// Deploy a contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters2, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// This is designed to revert at assembly time. TX2 should also fail because it is dependent on this TX
	aliceTx1 := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("tx1-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "1001"
		}`)). // Special value 1001 in the simple domain causes revert at assembly time
		Send()
	require.NoError(t, aliceTx1.Error())

	// Start another private transaction on alice's node, dependent on TX1
	aliceTx2 := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		DependsOn([]uuid.UUID{*aliceTx1.ID()}). // This TX depends on TX1 so if TX1 fails, this TX fails
		IdempotencyKey("tx2-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send()
	require.NoError(t, aliceTx2.Error())

	// Start one last private transaction on alice's node, dependent on TX2
	aliceTx3 := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		DependsOn([]uuid.UUID{*aliceTx2.ID()}). // This TX depends on TX2 so if TX2 fails, this TX fails
		IdempotencyKey("tx3-alice-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + bob.GetIdentityLocator() + `",
			"amount": "123000000000000000000"
		}`)).
		Send()
	require.NoError(t, aliceTx3.Error())

	// Check that we then get a receipt for both, and that both were unsuccessful
	assert.Eventually(t,
		transactionReceiptConditionFailureReceiptOnly(t, ctx, *aliceTx1.ID(), alice.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt or result was incorrect",
	)
	assert.Eventually(t,
		transactionReceiptConditionFailureReceiptOnly(t, ctx, *aliceTx2.ID(), alice.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt or result was incorrect",
	)
	assert.Eventually(t,
		transactionReceiptConditionFailureReceiptOnly(t, ctx, *aliceTx3.ID(), alice.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt or result was incorrect",
	)
}
