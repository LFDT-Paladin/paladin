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

type notoFailableHooksSuite struct {
	ctx                 context.Context
	runner              Runner
	notoContractAddress *pldtypes.EthAddress
	failableAddress     *pldtypes.EthAddress
	failableABI         abi.ABI
	notary              string
	sub                 rpcclient.Subscription
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

	// --- Step 1: Deploy FailableTarget on the base ledger ---
	log.Info("Loading FailableTarget contract...")
	failableTarget, err := contracts.LoadFailableTargetContract()
	if err != nil {
		return err
	}
	s.failableABI = failableTarget.ABI

	log.Info("Deploying FailableTarget as public transaction...")
	failableDeployTxID, err := nodes[0].HTTPClient.PTX().SendTransaction(s.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "deploy",
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
					"salt":    group.GenesisSalt.String(),
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

	// --- Step 5: Set FailableTarget to fail ---
	log.Info("Setting FailableTarget to fail...")
	abiRef, err := nodes[0].HTTPClient.PTX().StoreABI(s.ctx, s.failableABI)
	if err != nil {
		return fmt.Errorf("failed to store FailableTarget ABI: %w", err)
	}

	setFailTxID, err := nodes[0].HTTPClient.PTX().SendTransaction(s.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:         pldapi.TransactionTypePublic.Enum(),
			Function:     "setFail",
			To:           s.failableAddress,
			From:         "deploy",
			Data:         pldtypes.RawJSON(`[true]`),
			ABIReference: &abiRef,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send setFail transaction: %w", err)
	}

	log.Info("Waiting for setFail receipt...")
	setFailReceipt, err := util.WaitForTransactionReceipt(s.ctx, nodes[0].HTTPClient, *setFailTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get setFail receipt: %w", err)
	}
	if !setFailReceipt.Success {
		return fmt.Errorf("setFail transaction failed: %s", setFailReceipt.FailureMessage)
	}
	log.Info("FailableTarget configured to fail")

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
		notoContractAddress: s.notoContractAddress,
		notary:              s.notary,
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

	totalReceipts := 0
	failedWithRevertData := 0
	failedWithoutRevertData := 0
	succeededUnexpectedly := 0

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
			receipt, err := nodes[0].HTTPClient.PTX().GetTransactionReceipt(s.ctx, *tx.ID)
			if err != nil || receipt == nil {
				continue
			}
			totalReceipts++
			if receipt.Success {
				succeededUnexpectedly++
			} else if receipt.FailureMessage != "" {
				failedWithRevertData++
			} else {
				failedWithoutRevertData++
			}
		}

		createdCursor = txs[len(txs)-1].Created
		if len(txs) < notoFailableHooksPostRunPageSize {
			break
		}
	}

	log.Infof(
		"Post-run analysis complete: %d total receipts, %d failed with revert data, %d failed without revert data, %d succeeded unexpectedly",
		totalReceipts, failedWithRevertData, failedWithoutRevertData, succeededUnexpectedly,
	)

	if succeededUnexpectedly > 0 {
		return fmt.Errorf("%d transactions succeeded unexpectedly (expected all to fail)", succeededUnexpectedly)
	}
	if failedWithoutRevertData > 0 {
		return fmt.Errorf("%d transactions failed without revert data", failedWithoutRevertData)
	}
	if totalReceipts == 0 {
		return fmt.Errorf("no transaction receipts found for post-run analysis")
	}

	log.Infof("All %d receipts contain failure messages as expected", failedWithRevertData)
	return nil
}

type notoFailableHooksWorker struct {
	testBase
	notoContractAddress *pldtypes.EthAddress
	notary              string
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

	mintParams := &nototypes.MintParams{
		To:     tc.notary,
		Amount: pldtypes.Int64ToInt256(1000),
	}
	mintJSON, err := json.Marshal(mintParams)
	if err != nil {
		return "", fmt.Errorf("failed to marshal mint params: %w", err)
	}

	txID, err := nodes[0].HTTPClient.PTX().SendTransaction(tc.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "noto",
			Function:       "mint",
			To:             tc.notoContractAddress,
			From:           "member",
			Data:           pldtypes.RawJSON(mintJSON),
			IdempotencyKey: util.GetIdempotencyKey(tc.startTime, tc.workerID, iterationCount),
		},
		ABI: nototypes.NotoABI,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send noto mint transaction: %w", err)
	}

	return txID.String(), nil
}
