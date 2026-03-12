//go:build !besu_paid_gas
// +build !besu_paid_gas

package componenttest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	testutils "github.com/LFDT-Paladin/paladin/core/noderuntests/pkg"
	"github.com/LFDT-Paladin/paladin/core/noderuntests/pkg/domains"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateTransactionsSequencerLimitSequentialContracts(t *testing.T) {
	ctx := t.Context()

	sequencerConfig := pldconf.SequencerDefaults
	sequencerConfig.TargetActiveSequencers = confutil.P(10)

	alice := newSingleNodePartyForComponentTestingWithSequencerConfig(t, "node1", &sequencerConfig)
	client := alice.GetClient()
	endorsementSet := []string{alice.GetIdentityLocator()}

	sendDeploy := func(flowNum int) pldclient.SentTransaction {
		deploy := client.ForABI(ctx, *domains.SimpleTokenConstructorABI(domains.PrivacyGroupEndorsement)).
			Private().
			Domain("domain1").
			IdempotencyKey(fmt.Sprintf("deploy-seq-limit-%d", flowNum)).
			From(alice.GetIdentity()).
			Inputs(pldtypes.RawJSON(fmt.Sprintf(`{
                    "from": "wallets.org1.node1",
                    "endorsementSet": ["%s"],
                    "name": "SequencerLimitToken%d",
                    "symbol": "SLT%d",
					"endorsementMode": "%s",
					"hookAddress": "",
					"amountVisible": false
                }`, endorsementSet[0], flowNum, flowNum, domains.PrivacyGroupEndorsement))).
			Send()
		require.NoError(t, deploy.Error(), "deploy flow %d should submit", flowNum)
		return deploy
	}

	waitForDeploy := func(flowNum int, tx pldclient.SentTransaction) pldclient.TransactionResult {
		result := tx.Wait(transactionLatencyThreshold(t))
		require.NoError(t, result.Error(), "deploy number %d should complete", flowNum)
		require.NotNil(t, result.Receipt(), "deploy number %d should produce a receipt", flowNum)
		return result
	}

	// Kick off 10 deploys first, then wait for all receipts.
	deployTxs := make([]pldclient.SentTransaction, 0, 10)
	for i := 1; i <= 10; i++ {
		deployTxs = append(deployTxs, sendDeploy(i))
	}

	for i := 1; i <= 10; i++ {
		waitForDeploy(i, deployTxs[i-1])
	}

	// Deploy 11 will cause a sequencer to be stopped and replaced by the new one
	deploy11 := sendDeploy(11).Wait(transactionLatencyThreshold(t))
	require.NoError(t, deploy11.Error(), "deploy number 11 should complete; failure indicates sequencer limit regression")
	require.NotNil(t, deploy11.Receipt(), "deploy number 11 should produce a receipt")
}

func newTwoDomainParty(t *testing.T, nodeName string) testutils.Party {
	registry1 := deployDomainRegistry(t, nodeName)
	registry2 := deployDomainRegistry(t, nodeName)
	party := testutils.NewPartyForTestingWithNodeName(t, nodeName, nodeName, registry1)
	domainConfig := &domains.SimpleDomainPairConfig{
		SubmitMode:             domains.ENDORSER_SUBMISSION,
		Domain1RegistryAddress: registry1.String(),
		Domain2RegistryAddress: registry2.String(),
	}
	startNode(t, party, domainConfig)
	return party
}

func deploySimpleTokenInDomain(t *testing.T, ctx context.Context, client pldclient.PaladinClient, domainName string, identity string, params *domains.ConstructorParameters) *pldtypes.EthAddress {
	dplyTx := client.ForABI(ctx, *domains.SimpleTokenConstructorABI(params.EndorsementMode)).
		Private().
		Domain(domainName).
		From(identity).
		Inputs(pldtypes.JSONString(params)).
		Send().Wait(transactionLatencyThreshold(t) + 5*time.Second)
	require.NoError(t, dplyTx.Error())
	return dplyTx.Receipt().ContractAddress
}

func TestChainedTransactionSuccess(t *testing.T) {
	ctx := t.Context()
	party := newTwoDomainParty(t, "node1")
	client := party.GetClient()

	hookTargetAddr := deploySimpleTokenInDomain(t, ctx, client, "domain2", party.GetIdentity(), &domains.ConstructorParameters{
		From:            party.GetIdentity(),
		Name:            "HookTarget",
		Symbol:          "HT",
		EndorsementMode: domains.SelfEndorsement,
	})

	originAddr := deploySimpleTokenInDomain(t, ctx, client, "domain1", party.GetIdentity(), &domains.ConstructorParameters{
		From:            party.GetIdentity(),
		Name:            "Origin",
		Symbol:          "OR",
		EndorsementMode: domains.SelfEndorsement,
		HookAddress:     hookTargetAddr.String(),
	})

	tx := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		From(party.GetIdentity()).
		To(originAddr).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + party.GetIdentity() + `",
			"amount": "100"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, tx.Error())
	require.NotNil(t, tx.Receipt())
	assert.True(t, tx.Receipt().Success)

	chainedTxIdempotencyKey := fmt.Sprintf("%s_transfer", tx.ID().String())
	receiptLimit := 1
	chainedTxns, err := client.PTX().QueryTransactionsFull(ctx, &query.QueryJSON{
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
	require.Len(t, chainedTxns, 1)
	assert.True(t, chainedTxns[0].Receipt.Success)
}

func TestChainedTransactionRetryableRevertThenSucceeds(t *testing.T) {
	ctx := t.Context()
	party := newTwoDomainParty(t, "node1")
	client := party.GetClient()

	hookTargetAddr := deploySimpleTokenInDomain(t, ctx, client, "domain2", party.GetIdentity(), &domains.ConstructorParameters{
		From:            party.GetIdentity(),
		Name:            "HookTarget",
		Symbol:          "HT",
		EndorsementMode: domains.SelfEndorsement,
	})

	originAddr := deploySimpleTokenInDomain(t, ctx, client, "domain1", party.GetIdentity(), &domains.ConstructorParameters{
		From:            party.GetIdentity(),
		Name:            "Origin",
		Symbol:          "OR",
		EndorsementMode: domains.SelfEndorsement,
		HookAddress:     hookTargetAddr.String(),
	})

	// Amount 1003: retryable error on first attempt, succeeds on retry
	tx := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		From(party.GetIdentity()).
		To(originAddr).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + party.GetIdentity() + `",
			"amount": "1003"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.NoError(t, tx.Error())
	require.NotNil(t, tx.Receipt())
	assert.True(t, tx.Receipt().Success)

	chainedTxIdempotencyKey := fmt.Sprintf("%s_transfer", tx.ID().String())
	receiptLimit := 1
	chainedTxns, err := client.PTX().QueryTransactionsFull(ctx, &query.QueryJSON{
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
	require.Len(t, chainedTxns, 1)
	assert.True(t, chainedTxns[0].Receipt.Success)
	// Should have more than 1 public transaction due to the retry
	assert.Greater(t, len(chainedTxns[0].Public), 1)
}

func TestChainedTransactionAssemblyFailure(t *testing.T) {
	ctx := t.Context()
	party := newTwoDomainParty(t, "node1")
	client := party.GetClient()

	// Deploy the hook target contract in domain2 (no hook)
	hookTargetAddr := deploySimpleTokenInDomain(t, ctx, client, "domain2", party.GetIdentity(), &domains.ConstructorParameters{
		From:            party.GetIdentity(),
		Name:            "HookTarget",
		Symbol:          "HT",
		EndorsementMode: domains.SelfEndorsement,
	})

	// Deploy the originating contract in domain1 with a hook pointing to domain2's contract
	originAddr := deploySimpleTokenInDomain(t, ctx, client, "domain1", party.GetIdentity(), &domains.ConstructorParameters{
		From:            party.GetIdentity(),
		Name:            "Origin",
		Symbol:          "OR",
		EndorsementMode: domains.SelfEndorsement,
		HookAddress:     hookTargetAddr.String(),
	})

	// Amount 1001 causes an assembly revert in the chained TX (domain2, no hook, triggers revert)
	tx := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		From(party.GetIdentity()).
		To(originAddr).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + party.GetIdentity() + `",
			"amount": "1001"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.Error(t, tx.Error())
	require.NotNil(t, tx.Receipt())
	assert.False(t, tx.Receipt().Success)

	// The chained TX should also have a failure receipt, queryable by idempotency key
	chainedTxIdempotencyKey := fmt.Sprintf("%s_transfer", tx.ID().String())
	receiptLimit := 1
	chainedTxns, err := client.PTX().QueryTransactionsFull(ctx, &query.QueryJSON{
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
	require.Len(t, chainedTxns, 1)
	assert.False(t, chainedTxns[0].Receipt.Success)
}

func TestChainedTransactionBaseLedgerRevertFailure(t *testing.T) {
	ctx := t.Context()
	party := newTwoDomainParty(t, "node1")
	client := party.GetClient()

	// Deploy the hook target contract in domain2 (no hook)
	hookTargetAddr := deploySimpleTokenInDomain(t, ctx, client, "domain2", party.GetIdentity(), &domains.ConstructorParameters{
		From:            party.GetIdentity(),
		Name:            "HookTarget",
		Symbol:          "HT",
		EndorsementMode: domains.SelfEndorsement,
	})

	// Deploy the originating contract in domain1 with a hook pointing to domain2's contract
	originAddr := deploySimpleTokenInDomain(t, ctx, client, "domain1", party.GetIdentity(), &domains.ConstructorParameters{
		From:            party.GetIdentity(),
		Name:            "Origin",
		Symbol:          "OR",
		EndorsementMode: domains.SelfEndorsement,
		HookAddress:     hookTargetAddr.String(),
	})

	// Amount 1005 causes a non-retryable base-ledger revert in the chained TX
	tx := client.ForABI(ctx, *domains.SimpleTokenTransferABI()).
		Private().
		Domain("domain1").
		From(party.GetIdentity()).
		To(originAddr).
		Function("transfer").
		Inputs(pldtypes.RawJSON(`{
			"from": "",
			"to": "` + party.GetIdentity() + `",
			"amount": "1005"
		}`)).
		Send().Wait(transactionLatencyThreshold(t))
	require.Error(t, tx.Error())
	require.NotNil(t, tx.Receipt())
	assert.False(t, tx.Receipt().Success)
	assert.Contains(t, tx.Receipt().FailureMessage, "SimpleTokenNonRetryableError")

	// The chained TX should also have a failure receipt
	chainedTxIdempotencyKey := fmt.Sprintf("%s_transfer", tx.ID().String())
	receiptLimit := 1
	chainedTxns, err := client.PTX().QueryTransactionsFull(ctx, &query.QueryJSON{
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
	require.Len(t, chainedTxns, 1)
	assert.False(t, chainedTxns[0].Receipt.Success)
	assert.Contains(t, chainedTxns[0].Receipt.FailureMessage, "SimpleTokenNonRetryableError")
	assert.NotNil(t, chainedTxns[0].Receipt.TransactionReceiptDataOnchain)
	assert.NotNil(t, chainedTxns[0].Receipt.TransactionHash)
	assert.Greater(t, chainedTxns[0].Receipt.BlockNumber, int64(0))
}
