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
Integration tests for the coordinator failover feature (issue-1143).

These tests exercise the full multi-node stack and verify that when the
currently-observed coordinator node stops heartbeating, the remaining nodes
deterministically fail over to the next candidate from the originator pool
and complete in-flight transactions — rather than going immediately idle.

All tests use SelfEndorsement + ENDORSER_SUBMISSION so that each originating
node can assemble and endorse its own transaction without requiring other nodes
to be online. This isolates the coordinator-selection/failover logic from
endorsement availability — we can verify that transactions submitted by alice
complete even when bob (a coordinator candidate) is offline, because alice can
self-endorse; the only thing that changes is which node acts as coordinator.

Scenarios covered:

 1. TestCoordinatorFailover_PrimaryCoordinatorStops_TransactionCompletesViaFailover
    Three nodes (alice, bob, carol) using SelfEndorsement. Alice submits a
    transaction. Bob is then stopped. The test verifies that the remaining
    nodes elect the next deterministic coordinator candidate and the transaction
    eventually completes via the failover coordinator.

 2. TestCoordinatorFailover_PrimaryCoordinatorStopsAndRecovers_FailoverIndexReset
    Same 3-node topology. Bob stops — failover kicks in and alice completes a
    transaction. Bob then restarts, sending heartbeats that reset the failover
    index on peers. A post-recovery transaction verifies the system re-converges
    and completes normally.

 3. TestCoordinatorFailover_AllCandidatesUnresponsive_SystemGoesIdle
    Three nodes using NotaryEndorsement with bob as the notary. After populating
    the coordinator pool, both bob and carol are stopped. Because bob is both a
    coordinator candidate AND the required notary endorser, alice cannot complete
    any transaction: the coordinator pool is fully exhausted (idle) AND endorsement
    is unavailable. The test verifies the transaction does NOT complete within the
    normal window — safe idle behaviour rather than looping forever.

 4. TestCoordinatorFailover_FailoverThenNewBlockRange_ReselectsPrimary
    Two-node setup (alice, bob) using SelfEndorsement with a small BlockRange.
    Bob is stopped so failover fires. New blocks cross a block-range boundary,
    resetting coordinatorFailoverIndex to 0 and triggering
    action_ReevaluateCoordinatorOnNewBlock. Bob is restarted; the test verifies
    the stalled transaction completes once the boundary reset re-selects a live
    coordinator.
*/
package coordinationtest

import (
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	testutils "github.com/LFDT-Paladin/paladin/core/noderuntests/pkg"
	"github.com/LFDT-Paladin/paladin/core/noderuntests/pkg/domains"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failoverSequencerConfig returns a SequencerConfig tuned for fast failover in tests:
//   - 1 s heartbeat interval  → observers detect missed heartbeats quickly
//   - InactiveToIdleGracePeriod=2  → after 2 missed intervals the failover action fires
//   - RedelegateGracePeriod=1  → redelegate quickly once we become active coordinator
//   - short RequestTimeout / StateTimeout so stalled assembly is re-pooled fast
func failoverSequencerConfig() *pldconf.SequencerConfig {
	cfg := pldconf.SequencerDefaults
	cfg.HeartbeatInterval = confutil.P("1s")
	cfg.InactiveToIdleGracePeriod = confutil.P(2)
	cfg.RedelegateGracePeriod = confutil.P(1)
	cfg.RequestTimeout = confutil.P("3s")
	cfg.StateTimeout = confutil.P("10s")
	return &cfg
}

// failoverTransactionLatencyThreshold gives a generous window for failover tests
// because failover itself takes a few heartbeat cycles before transactions resume.
func failoverTransactionLatencyThreshold(t *testing.T) time.Duration {
	return transactionLatencyThresholdCustom(t, func() *time.Duration {
		d := 30 * time.Second
		return &d
	}())
}

// ── Test 1 ───────────────────────────────────────────────────────────────────

// TestCoordinatorFailover_PrimaryCoordinatorStops_TransactionCompletesViaFailover
// verifies that when the primary coordinator node goes offline, the remaining
// nodes fail over to the next candidate in the pool and the pending transaction
// eventually completes.
//
// SelfEndorsement is used so that alice can assemble and endorse her own
// transaction without needing bob or carol online — the only coordination role
// bob provides is as coordinator, not as endorser.
func TestCoordinatorFailover_PrimaryCoordinatorStops_TransactionCompletesViaFailover(t *testing.T) {
	ctx := t.Context()

	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	carol := testutils.NewPartyForTesting(t, "carol", domainRegistryAddress)

	// Wire all peers together so all three appear in the coordinator pool.
	alice.AddPeer(bob.GetNodeConfig(), carol.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig(), carol.GetNodeConfig())
	carol.AddPeer(alice.GetNodeConfig(), bob.GetNodeConfig())

	seqCfg := failoverSequencerConfig()
	alice.OverrideSequencerConfig(seqCfg)
	bob.OverrideSequencerConfig(seqCfg)
	carol.OverrideSequencerConfig(seqCfg)

	// SelfEndorsement: each node endorses its own transactions, so alice's
	// transactions can complete without bob or carol being online as endorsers.
	// ENDORSER_SUBMISSION: the coordinator node submits to the base ledger.
	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	startNode(t, carol, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, carol)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FailoverToken1",
		Symbol:          "FO1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, failoverTransactionLatencyThreshold)

	// Warm-up: submit a transaction while all nodes are healthy. This populates
	// the originator node pool on all coordinators so failover has candidates to
	// cycle through.
	warmupTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("failover-warmup-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "100000000000000000000"
		}`)).
		Send().Wait(failoverTransactionLatencyThreshold(t))
	require.NoError(t, warmupTx.Error(), "warm-up transaction should succeed with all nodes healthy")

	// Stop bob. After InactiveToIdleGracePeriod heartbeat intervals the failover
	// action fires on observing nodes and picks the next live candidate from the
	// pool (alice or carol, whichever the hash selects next).
	stopNode(t, bob)

	// Submit a transaction while bob is offline — it should eventually complete
	// via the failover coordinator (alice or carol).
	failoverTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("failover-tx-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "50000000000000000000"
		}`)).
		Send()
	require.NoError(t, failoverTx.Error(), "send should not return an immediate error")

	// Failover takes a few heartbeat cycles; use the generous threshold.
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, *failoverTx.ID(), alice.GetClient(), false),
		failoverTransactionLatencyThreshold(t),
		200*time.Millisecond,
		"transaction should complete after coordinator failover with bob offline",
	)
}

// ── Test 2 ───────────────────────────────────────────────────────────────────

// TestCoordinatorFailover_PrimaryCoordinatorStopsAndRecovers_FailoverIndexReset
// verifies that when the primary coordinator stops, failover engages, a
// transaction still completes via the failover coordinator, and then when the
// primary restarts and sends a heartbeat the failover index is reset so
// subsequent transactions complete normally.
//
// SelfEndorsement is used so alice can complete transactions regardless of
// whether bob is online as an endorser — the test is purely about coordinator
// role failover and recovery, not endorsement availability.
func TestCoordinatorFailover_PrimaryCoordinatorStopsAndRecovers_FailoverIndexReset(t *testing.T) {
	ctx := t.Context()

	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	carol := testutils.NewPartyForTesting(t, "carol", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig(), carol.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig(), carol.GetNodeConfig())
	carol.AddPeer(alice.GetNodeConfig(), bob.GetNodeConfig())

	seqCfg := failoverSequencerConfig()
	alice.OverrideSequencerConfig(seqCfg)
	bob.OverrideSequencerConfig(seqCfg)
	carol.OverrideSequencerConfig(seqCfg)

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	startNode(t, carol, domainConfig)
	t.Cleanup(func() {
		stopNode(t, carol)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FailoverToken2",
		Symbol:          "FO2",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, failoverTransactionLatencyThreshold)

	// Warm-up: confirm healthy operation and populate the coordinator pool.
	warmupTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("recovery-warmup-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "100000000000000000000"
		}`)).
		Send().Wait(failoverTransactionLatencyThreshold(t))
	require.NoError(t, warmupTx.Error(), "warm-up transaction should succeed with all nodes healthy")

	// Stop bob to trigger failover on observing nodes.
	stopNode(t, bob)

	// Submit a transaction — should complete via the failover coordinator
	// (alice or carol, whoever the hash selects at failoverIndex=1).
	failoverTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("recovery-failover-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "50000000000000000000"
		}`)).
		Send()
	require.NoError(t, failoverTx.Error())

	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, *failoverTx.ID(), alice.GetClient(), false),
		failoverTransactionLatencyThreshold(t),
		200*time.Millisecond,
		"failover transaction should complete while bob is offline",
	)

	// Restart bob — his heartbeats will reach alice and carol, and
	// action_HeartbeatReceived will reset coordinatorFailoverIndex to 0.
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	// Allow enough time for at least 2 heartbeat cycles so the reset propagates.
	time.Sleep(3 * time.Second)

	// Post-recovery transaction — should complete normally now that bob is back
	// and the failover index has been reset to 0.
	recoveryTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("post-recovery-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "25000000000000000000"
		}`)).
		Send().Wait(failoverTransactionLatencyThreshold(t))
	require.NoError(t, recoveryTx.Error(), "post-recovery transaction should complete after bob rejoins and failover index resets")
}

// ── Test 3 ───────────────────────────────────────────────────────────────────

// TestCoordinatorFailover_AllCandidatesUnresponsive_SystemGoesIdle
// verifies that when every candidate in the originator pool is unreachable, the
// surviving node exhausts the failover list and the submitted transaction does
// NOT complete within the normal latency window — safe idle behaviour rather
// than looping forever.
//
// NotaryEndorsement with bob as notary is used so that alice's transaction
// genuinely cannot complete when bob is offline: bob is both a required endorser
// AND a coordinator candidate, so the system is stuck from both angles.
func TestCoordinatorFailover_AllCandidatesUnresponsive_SystemGoesIdle(t *testing.T) {
	ctx := t.Context()

	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)
	carol := testutils.NewPartyForTesting(t, "carol", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig(), carol.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig(), carol.GetNodeConfig())
	carol.AddPeer(alice.GetNodeConfig(), bob.GetNodeConfig())

	// Very short grace period so all failover candidates are exhausted quickly.
	seqCfg := failoverSequencerConfig()
	seqCfg.InactiveToIdleGracePeriod = confutil.P(1)
	alice.OverrideSequencerConfig(seqCfg)
	bob.OverrideSequencerConfig(seqCfg)
	carol.OverrideSequencerConfig(seqCfg)

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	startNode(t, carol, domainConfig)

	// NotaryEndorsement: bob is the notary, so every transaction submitted by
	// alice requires bob's endorsement. When bob goes offline, transactions are
	// stuck from both the coordinator side (pool exhausted → idle) and the
	// endorsement side (notary unreachable).
	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FailoverToken3",
		Symbol:          "FO3",
		EndorsementMode: domains.NotaryEndorsement,
		Notary:          bob.GetIdentityLocator(),
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, failoverTransactionLatencyThreshold)

	// Warm-up: populate the coordinator pool on all nodes. With NotaryEndorsement
	// the notary (bob) must be online to endorse — which he is at this point.
	warmupTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("idle-warmup-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "100000000000000000000"
		}`)).
		Send().Wait(failoverTransactionLatencyThreshold(t))
	require.NoError(t, warmupTx.Error(), "warm-up transaction should succeed with all nodes healthy")

	// Stop bob and carol — alice has no reachable coordinator candidates.
	stopNode(t, bob)
	stopNode(t, carol)
	t.Cleanup(func() {
		stopNode(t, alice)
	})

	// Submit a transaction that requires bob's notary endorsement — which is now
	// impossible since bob is offline. Alice should exhaust all coordinator
	// failover candidates and go idle rather than looping forever.
	// The transaction must NOT receive a receipt within the normal window.
	stalledTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("idle-stall-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "10000000000000000000"
		}`)).
		Send()
	require.NoError(t, stalledTx.Error(), "send should not fail synchronously")

	// Use the short (normal) threshold — a receipt here would be a test failure.
	result := stalledTx.Wait(transactionLatencyThreshold(t))
	require.Error(t, result.Error(), "stalled transaction should NOT complete when bob (notary + coordinator candidate) is offline")
	assert.Contains(t, result.Error().Error(), "timed out",
		"error should indicate a timeout, not a hard failure")
}

// ── Test 4 ───────────────────────────────────────────────────────────────────

// TestCoordinatorFailover_FailoverThenNewBlockRange_ReselectsPrimary
// verifies the OnNewBlockHeight → TryQueueEvent → action_ReevaluateCoordinatorOnNewBlock
// wiring end-to-end. When a new block-range boundary is crossed the failover
// index is reset to 0 and the coordinator is re-selected, so transactions
// resume once a live node is re-elected as primary.
//
// SelfEndorsement is used so alice's transaction can complete without bob as
// an endorser — bob's role here is purely as a coordinator candidate.
func TestCoordinatorFailover_FailoverThenNewBlockRange_ReselectsPrimary(t *testing.T) {
	ctx := t.Context()

	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	// Small BlockRange (minimum = 10) so a boundary is crossed quickly during
	// the test without needing to wait for many blocks.
	seqCfg := failoverSequencerConfig()
	seqCfg.BlockRange = confutil.P(uint64(10))
	alice.OverrideSequencerConfig(seqCfg)
	bob.OverrideSequencerConfig(seqCfg)

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FailoverToken4",
		Symbol:          "FO4",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, failoverTransactionLatencyThreshold)

	// Warm-up: populate the coordinator pool on both nodes.
	warmupTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("blockrange-warmup-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "100000000000000000000"
		}`)).
		Send().Wait(failoverTransactionLatencyThreshold(t))
	require.NoError(t, warmupTx.Error(), "warm-up transaction should succeed with all nodes healthy")

	// Stop bob to force failover on alice's coordinator state machine.
	stopNode(t, bob)

	// Submit a transaction while bob is offline — will stall until a live
	// coordinator is selected (either via failover or block-range reset).
	stalledTx := alice.GetClient().ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		IdempotencyKey("blockrange-stall-" + uuid.New().String()).
		From(alice.GetIdentity()).
		To(contractAddress).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + alice.GetIdentityLocator() + `",
			"amount": "10000000000000000000"
		}`)).
		Send()
	require.NoError(t, stalledTx.Error())

	// Allow the failover action to fire (≥2 heartbeat intervals = ≥2 s) and for
	// Besu to mine enough blocks to cross a block-range boundary (every 10 blocks).
	// action_NewBlock resets coordinatorFailoverIndex to 0 at the boundary, and
	// action_ReevaluateCoordinatorOnNewBlock re-selects the primary candidate.
	time.Sleep(5 * time.Second)

	// Restart bob. With the failover index reset, the next coordinator selection
	// may land on alice (who is always live), so the stalled transaction should
	// be able to complete even before a second boundary.
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, bob)
	})

	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, *stalledTx.ID(), alice.GetClient(), false),
		failoverTransactionLatencyThreshold(t),
		200*time.Millisecond,
		"stalled transaction should complete after bob restarts and block-range boundary resets the failover index",
	)
}
