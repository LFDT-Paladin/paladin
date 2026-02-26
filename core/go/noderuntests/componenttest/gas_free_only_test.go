//go:build !besu_paid_gas
// +build !besu_paid_gas

package componenttest

import (
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/noderuntests/pkg/domains"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
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
