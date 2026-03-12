// Copyright © 2026 Kaleido, Inc.
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

package testsuite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	"github.com/LFDT-Paladin/paladin/perf/internal/contracts"
	"github.com/LFDT-Paladin/paladin/perf/internal/util"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"

	nototypes "github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	log "github.com/sirupsen/logrus"
)

const notoFailableHooksListenerName = "notofailablehookslistener"
const notoFailableHooksPostRunPageSize = 500

var notoConstructorABI = abi.ABI{
	{Type: abi.Constructor, Inputs: abi.ParameterArray{
		{Name: "notary", Type: "string"},
		{Name: "notaryMode", Type: "string"},
		{Name: "options", Type: "tuple", Components: abi.ParameterArray{
			{Name: "hooks", Type: "tuple", Components: abi.ParameterArray{
				{Name: "publicAddress", Type: "string"},
				{Name: "privateAddress", Type: "string"},
				{Name: "privateGroup", Type: "tuple", Components: abi.ParameterArray{
					{Name: "salt", Type: "bytes32"},
					{Name: "members", Type: "string[]"},
				}},
			}},
		}},
	}},
}

const notoFailableHooksFailEvery = 30

type notoFailableHooksSuite struct {
	ctx                 context.Context
	runner              Runner
	notoContractAddress *pldtypes.EthAddress
	failableAddress     *pldtypes.EthAddress
	notary              string
	recipient           string
	sub                 rpcclient.Subscription
	submissions         atomic.Int64
}

func NewNotoFailableHooksSuite(ctx context.Context, runner Runner) *notoFailableHooksSuite {
	return &notoFailableHooksSuite{ctx: ctx, runner: runner}
}

func (s *notoFailableHooksSuite) Setup() error {
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes configured")
	}

	s.notary = fmt.Sprintf("member@%s", nodes[0].Config.Name)
	s.recipient = fmt.Sprintf("recipient@%s", nodes[0].Config.Name)

	// --- Step 1: Deploy FailableTarget on the base ledger ---
	log.Info("Loading FailableTarget contract...")
	failableTarget, err := contracts.LoadFailableTargetContract()
	if err != nil {
		return err
	}
	log.Infof("Deploying FailableTarget (failEvery=%d) as public transaction...", notoFailableHooksFailEvery)
	failableDeployTxID, err := nodes[0].HTTPClient.PTX().SendTransaction(s.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "deploy",
			Data: pldtypes.RawJSON(fmt.Sprintf(`[%d]`, notoFailableHooksFailEvery)),
		},
		ABI:      failableTarget.ABI,
		Bytecode: failableTarget.Bytecode,
	})
	if err != nil {
		return fmt.Errorf("failed to deploy FailableTarget: %w", err)
	}

	log.Info("Waiting for FailableTarget deployment receipt...")
	failableReceipt, err := util.WaitForTransactionReceiptFull(s.ctx, nodes[0].HTTPClient, *failableDeployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get FailableTarget deployment receipt: %w", err)
	}
	if !failableReceipt.Success {
		return fmt.Errorf("FailableTarget deployment failed: %s", failableReceipt.FailureMessage)
	}
	if failableReceipt.ContractAddress == nil {
		return fmt.Errorf("FailableTarget contract address not found in deployment receipt")
	}
	s.failableAddress = failableReceipt.ContractAddress
	log.Infof("FailableTarget deployed at address: %s", *s.failableAddress)

	// --- Step 2: Create Pente privacy group ---
	members := make([]string, len(nodes))
	for i, node := range nodes {
		members[i] = fmt.Sprintf("member@%s", node.Config.Name)
	}

	log.Info("Creating Pente privacy group...")
	group, err := nodes[0].HTTPClient.PrivacyGroups().CreateGroup(s.ctx, &pldapi.PrivacyGroupInput{
		Domain:  "pente",
		Members: members,
		Name:    "noto-failable-hooks-test",
		Configuration: map[string]string{
			"evmVersion":           "shanghai",
			"externalCallsEnabled": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create privacy group: %w", err)
	}
	log.Infof("Privacy group created with ID: %s", group.ID)

	log.Info("Waiting for privacy group genesis receipt...")
	groupReceipt, err := util.WaitForTransactionReceipt(s.ctx, nodes[0].HTTPClient, group.GenesisTransaction, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get privacy group genesis receipt: %w", err)
	}
	if !groupReceipt.Success {
		return fmt.Errorf("privacy group creation failed: %s", groupReceipt.FailureMessage)
	}
	penteAddress := groupReceipt.ContractAddress
	log.Infof("Privacy group contract address: %s", penteAddress)

	// --- Step 3: Deploy NotoHooksFailable inside the privacy group ---
	log.Info("Loading NotoHooksFailable contract...")
	notoHooks, err := contracts.LoadNotoHooksFailableContract()
	if err != nil {
		return err
	}

	var hooksConstructor *abi.Entry
	for _, entry := range notoHooks.ABI {
		if entry.Type == abi.Constructor {
			hooksConstructor = entry
			break
		}
	}

	log.Info("Deploying NotoHooksFailable to privacy group...")
	hooksDeployTxID, err := nodes[0].HTTPClient.PrivacyGroups().SendTransaction(s.ctx, &pldapi.PrivacyGroupEVMTXInput{
		Domain: "pente",
		Group:  group.ID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "member",
			Bytecode: notoHooks.Bytecode,
			Function: hooksConstructor,
			Input:    pldtypes.RawJSON(fmt.Sprintf(`{"_failableTarget":"%s"}`, s.failableAddress)),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to deploy NotoHooksFailable: %w", err)
	}

	log.Info("Waiting for NotoHooksFailable deployment receipt...")
	hooksReceipt, err := util.WaitForTransactionReceiptFull(s.ctx, nodes[0].HTTPClient, hooksDeployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get NotoHooksFailable deployment receipt: %w", err)
	}
	if !hooksReceipt.Success {
		return fmt.Errorf("NotoHooksFailable deployment failed: %s", hooksReceipt.FailureMessage)
	}

	var hooksPrivateAddress *pldtypes.EthAddress
	if hooksReceipt.DomainReceipt != nil {
		var domainReceipt pldapi.PenteDomainReceipt
		if err := json.Unmarshal(hooksReceipt.DomainReceipt, &domainReceipt); err == nil {
			if domainReceipt.Receipt != nil && domainReceipt.Receipt.ContractAddress != nil {
				hooksPrivateAddress = domainReceipt.Receipt.ContractAddress
			}
		}
	}
	if hooksPrivateAddress == nil {
		return fmt.Errorf("NotoHooksFailable private contract address not found in domain receipt")
	}
	log.Infof("NotoHooksFailable deployed at private address: %s", *hooksPrivateAddress)

	// --- Step 4: Deploy Noto domain instance with hooks ---
	log.Info("Deploying Noto with failable hooks...")
	constructorParams := map[string]any{
		"notary":     s.notary,
		"notaryMode": string(nototypes.NotaryModeHooks),
		"options": map[string]any{
			"hooks": map[string]any{
				"publicAddress":  penteAddress.String(),
				"privateAddress": hooksPrivateAddress.String(),
				"privateGroup": map[string]any{
					"salt":    group.ID.String(),
					"members": members,
				},
			},
		},
	}

	notoDeployResult := nodes[0].HTTPClient.ForABI(s.ctx, notoConstructorABI).
		Private().
		Domain("noto").
		Constructor().
		From("member").
		Inputs(constructorParams).
		Send().
		Wait(60 * time.Second)
	if notoDeployResult.Error() != nil {
		return fmt.Errorf("failed to deploy Noto: %w", notoDeployResult.Error())
	}
	if notoDeployResult.Receipt().ContractAddress == nil {
		return fmt.Errorf("Noto contract address not found in deployment receipt")
	}
	s.notoContractAddress = notoDeployResult.Receipt().ContractAddress
	log.Infof("Noto deployed at address: %s", *s.notoContractAddress)

	// --- Step 5: Mint initial supply to the notary ---
	log.Info("Minting initial supply to notary...")
	mintParams := &nototypes.MintParams{
		To:     s.notary,
		Amount: pldtypes.Int64ToInt256(1000000),
	}
	mintJSON, err := json.Marshal(mintParams)
	if err != nil {
		return fmt.Errorf("failed to marshal initial mint params: %w", err)
	}
	mintTxID, err := nodes[0].HTTPClient.PTX().SendTransaction(s.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePrivate.Enum(),
			Domain:   "noto",
			Function: "mint",
			To:       s.notoContractAddress,
			From:     "member",
			Data:     pldtypes.RawJSON(mintJSON),
		},
		ABI: nototypes.NotoABI,
	})
	if err != nil {
		return fmt.Errorf("failed to send initial mint transaction: %w", err)
	}
	mintReceipt, err := util.WaitForTransactionReceipt(s.ctx, nodes[0].HTTPClient, *mintTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get initial mint receipt: %w", err)
	}
	if !mintReceipt.Success {
		return fmt.Errorf("initial mint failed: %s", mintReceipt.FailureMessage)
	}
	log.Infof("Initial supply of 1000000 minted to %s", s.notary)

	// --- Step 6: Create receipt listener for Noto ---
	var latestSequence *uint64
	qb := query.NewQueryBuilder().Equal("domain", "noto").Sort("-sequence").Limit(1)
	receipts, err := nodes[0].HTTPClient.PTX().QueryTransactionReceipts(s.ctx, qb.Query())
	if err == nil && len(receipts) > 0 {
		seq := receipts[0].Sequence
		latestSequence = &seq
		log.Infof("Found latest sequence: %d, will start listener from sequence above this", seq)
	} else {
		log.Info("No existing Noto receipts found, starting listener from beginning")
	}

	_, _ = nodes[0].HTTPClient.PTX().DeleteReceiptListener(s.ctx, notoFailableHooksListenerName)

	txType := pldapi.TransactionTypePrivate.Enum()
	_, err = nodes[0].HTTPClient.PTX().CreateReceiptListener(s.ctx, &pldapi.TransactionReceiptListener{
		Name: notoFailableHooksListenerName,
		Filters: pldapi.TransactionReceiptFilters{
			Type:          &txType,
			Domain:        "noto",
			SequenceAbove: latestSequence,
		},
		Options: pldapi.TransactionReceiptListenerOptions{
			DomainReceipts: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create receipt listener: %w", err)
	}

	log.Info("Noto failable hooks test setup complete")
	return nil
}

func (s *notoFailableHooksSuite) Subscribe() (rpcclient.Subscription, error) {
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes configured")
	}
	sub, err := nodes[0].WSClient.PTX().SubscribeReceipts(s.ctx, notoFailableHooksListenerName)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to noto receipts: %w", err)
	}
	s.sub = sub
	return sub, nil
}

func (s *notoFailableHooksSuite) Unsubscribe() {
	if s.sub != nil {
		if err := s.sub.Unsubscribe(s.ctx); err != nil {
			log.Errorf("Error unsubscribing from subscription: %s", err.Error())
		} else {
			log.Info("Successfully unsubscribed")
		}
		s.sub = nil
	}
}

func (s *notoFailableHooksSuite) Cleanup() {
	nodes := s.runner.GetNodes()
	if len(nodes) > 0 {
		_, err := nodes[0].HTTPClient.PTX().DeleteReceiptListener(s.ctx, notoFailableHooksListenerName)
		if err != nil {
			log.Debugf("Failed to delete receipt listener %s: %v", notoFailableHooksListenerName, err)
		} else {
			log.Infof("Successfully deleted receipt listener: %s", notoFailableHooksListenerName)
		}
	}
}

func (s *notoFailableHooksSuite) NewWorker(startTime int64, workerID int) TestCase {
	return &notoFailableHooksWorker{
		testBase: testBase{
			ctx:       s.ctx,
			startTime: startTime,
			workerID:  workerID,
		},
		suite:               s,
		notoContractAddress: s.notoContractAddress,
		notary:              s.notary,
		recipient:           s.recipient,
		runner:              s.runner,
	}
}

func (s *notoFailableHooksSuite) PostRun() error {
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes configured")
	}
	if s.notoContractAddress == nil {
		return fmt.Errorf("noto contract address not set for post-run analysis")
	}

	log.Infof("Running post-run analysis for noto failable hooks test (contract %s)", *s.notoContractAddress)

	// The failure message is the raw hex-encoded ABI revert data, so match
	// against the hex encoding of the expected reason string.
	const expectedRevertReason = "Configured to fail"
	expectedRevertReasonHex := fmt.Sprintf("%x", expectedRevertReason)

	// NotoInvalidInput selector: keccak256("NotoInvalidInput(bytes32)")[:4]
	const notoInvalidInputSelector = "8b8ff76e"

	totalSubmissions := int(s.submissions.Load())
	totalReceipts := 0
	succeeded := 0
	failedWithExpectedRevert := 0
	failedWithNotoInvalidInput := 0
	failedWithNoOnchainData := 0
	var unexpectedResults []string

	var createdCursor pldtypes.Timestamp
	for {
		qb := query.NewQueryBuilder().
			Equal("domain", "noto").
			Equal("to", *s.notoContractAddress).
			Sort("-created").
			Limit(notoFailableHooksPostRunPageSize)
		if createdCursor != 0 {
			qb = qb.LessThan("created", createdCursor)
		}

		txs, err := nodes[0].HTTPClient.PTX().QueryTransactionsFull(s.ctx, qb.Query())
		if err != nil {
			return fmt.Errorf("post-run queryTransactionsFull failed: %w", err)
		}
		if len(txs) == 0 {
			break
		}

		for _, tx := range txs {
			if tx == nil || tx.ID == nil {
				continue
			}
			if !strings.Contains(tx.Function, "transfer") {
				continue
			}
			receipt, err := nodes[0].HTTPClient.PTX().GetTransactionReceiptFull(s.ctx, *tx.ID)
			if err != nil || receipt == nil {
				continue
			}
			totalReceipts++

			hasOnchainData := receipt.TransactionReceiptDataOnchain != nil
			var blockNum int64
			if hasOnchainData {
				blockNum = receipt.TransactionReceiptDataOnchain.BlockNumber
			}

			if receipt.Success {
				succeeded++
				if !hasOnchainData {
					failedWithNoOnchainData++
					log.Warnf("tx %s: succeeded but has no on-chain data in receipt", tx.ID)
				} else {
					shouldHaveSucceeded := blockNum%int64(notoFailableHooksFailEvery) != 0
					if !shouldHaveSucceeded {
						unexpectedResults = append(unexpectedResults, fmt.Sprintf("tx %s: succeeded at block %d but expected failure (block %% %d == 0)", tx.ID, blockNum, notoFailableHooksFailEvery))
					}
				}
			} else if strings.Contains(receipt.FailureMessage, expectedRevertReasonHex) {
				failedWithExpectedRevert++
				if !hasOnchainData {
					failedWithNoOnchainData++
					log.Warnf("tx %s: reverted with %q but has no on-chain data in receipt", tx.ID, expectedRevertReason)
				} else {
					shouldHaveFailed := blockNum%int64(notoFailableHooksFailEvery) == 0
					if !shouldHaveFailed {
						unexpectedResults = append(unexpectedResults, fmt.Sprintf("tx %s: reverted at block %d but expected success (block %% %d != 0)", tx.ID, blockNum, notoFailableHooksFailEvery))
					}
				}
			} else if strings.Contains(receipt.FailureMessage, notoInvalidInputSelector) {
				failedWithNotoInvalidInput++
				log.Infof("tx %s: NotoInvalidInput (cascade from prior revert): %s", tx.ID, receipt.FailureMessage)
			} else {
				txFull, txErr := nodes[0].HTTPClient.PTX().GetTransactionFull(s.ctx, *tx.ID)
				if txErr == nil && txFull != nil {
					txJSON, _ := json.MarshalIndent(txFull, "", "  ")
					log.Errorf("Unexpected failure for tx %s (full transaction):\n%s", tx.ID, string(txJSON))
				}
				unexpectedResults = append(unexpectedResults, fmt.Sprintf("tx %s: unexpected failure: %s", tx.ID, receipt.FailureMessage))
			}
		}

		createdCursor = txs[len(txs)-1].Created
		if len(txs) < notoFailableHooksPostRunPageSize {
			break
		}
	}

	log.Infof(
		"Post-run analysis complete: %d submissions, %d receipts, %d succeeded, %d reverted with %q, %d NotoInvalidInput (cascade), %d missing on-chain data, %d unexpected",
		totalSubmissions, totalReceipts, succeeded, failedWithExpectedRevert, expectedRevertReason, failedWithNotoInvalidInput, failedWithNoOnchainData, len(unexpectedResults),
	)

	if totalReceipts == 0 {
		return fmt.Errorf("no transaction receipts found for post-run analysis")
	}
	if totalReceipts != totalSubmissions {
		return fmt.Errorf("receipt count %d does not match submission count %d", totalReceipts, totalSubmissions)
	}
	if failedWithNoOnchainData > 0 {
		log.Warnf("%d transactions had no on-chain data in their receipt", failedWithNoOnchainData)
	}
	if len(unexpectedResults) > 0 {
		for _, msg := range unexpectedResults {
			log.Errorf("Unexpected result: %s", msg)
		}
		return fmt.Errorf("%d/%d transactions had unexpected results (only expected success or revert with %q)", len(unexpectedResults), totalReceipts, expectedRevertReason)
	}
	if failedWithExpectedRevert == 0 {
		return fmt.Errorf("no transactions reverted - expected some failures with failEvery=%d", notoFailableHooksFailEvery)
	}
	if succeeded == 0 {
		return fmt.Errorf("no transactions succeeded - expected some successes with failEvery=%d", notoFailableHooksFailEvery)
	}

	log.Infof("All assertions passed: %d succeeded, %d reverted with %q, %d NotoInvalidInput (cascade), %d missing on-chain data",
		succeeded, failedWithExpectedRevert, expectedRevertReason, failedWithNotoInvalidInput, failedWithNoOnchainData)
	return nil
}

type notoFailableHooksWorker struct {
	testBase
	suite               *notoFailableHooksSuite
	notoContractAddress *pldtypes.EthAddress
	notary              string
	recipient           string
	runner              Runner
}

func (tc *notoFailableHooksWorker) Name() conf.TestName {
	return conf.PerfTestNotoFailableHooks
}

func (tc *notoFailableHooksWorker) RunOnce(iterationCount int) (string, error) {
	nodes := tc.runner.GetNodes()
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes configured")
	}

	transferParams := &nototypes.TransferParams{
		To:     tc.recipient,
		Amount: pldtypes.Int64ToInt256(1),
	}
	transferJSON, err := json.Marshal(transferParams)
	if err != nil {
		return "", fmt.Errorf("failed to marshal transfer params: %w", err)
	}

	txID, err := nodes[0].HTTPClient.PTX().SendTransaction(tc.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "noto",
			Function:       "transfer",
			To:             tc.notoContractAddress,
			From:           "member",
			Data:           pldtypes.RawJSON(transferJSON),
			IdempotencyKey: util.GetIdempotencyKey(tc.startTime, tc.workerID, iterationCount),
		},
		ABI: nototypes.NotoABI,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send noto transfer transaction: %w", err)
	}
	tc.suite.submissions.Add(1)

	return txID.String(), nil
}
