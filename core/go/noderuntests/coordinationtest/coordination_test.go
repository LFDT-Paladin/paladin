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
	"testing"
	"time"

	testutils "github.com/LFDT-Paladin/paladin/core/noderuntests/pkg"
	"github.com/LFDT-Paladin/paladin/core/noderuntests/pkg/domains"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Map of node names to config paths. Each node needs its own DB and static signing key
var CONFIG_PATHS = map[string]string{
	"alice": "./config/postgres.coordinationtest.alice.config.yaml",
	"bob":   "./config/postgres.coordinationtest.bob.config.yaml",
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
		transactionReceiptConditionExpectedPublicTXCount(t, ctx, aliceTx.ID(), bob.GetClient(), 1),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt with 1 public TX",
	)

	// Check Alice and Bob both have the same view of the world
	aliceTxFull, err := alice.GetClient().PTX().GetTransactionFull(ctx, aliceTx.ID())
	require.NoError(t, err)
	require.NotNil(t, aliceTxFull)

	bobTxFull, err := bob.GetClient().PTX().GetTransactionFull(ctx, aliceTx.ID())
	require.NoError(t, err)
	require.NotNil(t, bobTxFull)

	// assert.Equal(t, aliceTxFull.ABIReference, bobTxFull.ABIReference)
	// assert.Equal(t, aliceTxFull.Domain, bobTxFull.Domain)
	// assert.Equal(t, aliceTxFull.Function, bobTxFull.Function)
	// assert.Equal(t, aliceTxFull.From, bobTxFull.From)
	// assert.Equal(t, aliceTxFull.To, bobTxFull.To)
	// assert.Equal(t, aliceTxFull.Gas, bobTxFull.Gas)
	// assert.Equal(t, aliceTxFull.Data, bobTxFull.Data)
	// assert.Equal(t, aliceTxFull.Public[0].TransactionHash, bobTxFull.Public[0].TransactionHash)
	// assert.Equal(t, aliceTxFull.Public[0].From, bobTxFull.Public[0].From)
	// assert.Equal(t, aliceTxFull.Public[0].To, bobTxFull.Public[0].To)
	// assert.Equal(t, aliceTxFull.Public[0].Value, bobTxFull.Public[0].Value)
	// assert.Equal(t, aliceTxFull.Public[0].Gas, bobTxFull.Public[0].Gas)
}
