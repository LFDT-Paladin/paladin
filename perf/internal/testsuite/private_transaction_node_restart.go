// Copyright © 2025 Kaleido, Inc.
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
	"math/rand"
	"strings"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	"github.com/LFDT-Paladin/paladin/perf/internal/contracts"
	"github.com/LFDT-Paladin/paladin/perf/internal/util"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/retry"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	log "github.com/sirupsen/logrus"
)

type privateTransactionNodeRestartSuite struct {
	ctx             context.Context
	runner          Runner
	privacyGroupID  *pldtypes.HexBytes
	contractAddress *pldtypes.EthAddress
	sub             rpcclient.Subscription
}

const privateTxPostRunPageSize = 500
const privateTxRetryAttempts = 5
const txMgrIdempotencyKeyClashErrorCode = "PD012220"

// NewPrivateTransactionNodeRestartSuite creates a new private transaction node restart test suite with the given context and runner.
func NewPrivateTransactionNodeRestartSuite(ctx context.Context, runner Runner) *privateTransactionNodeRestartSuite {
	return &privateTransactionNodeRestartSuite{ctx: ctx, runner: runner}
}

func (s *privateTransactionNodeRestartSuite) Setup() error {
	log.Infof("Running private transaction node restart test")
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes configured")
	}

	simpleStorage, err := contracts.LoadSimpleStorageContract()
	if err != nil {
		return err
	}

	members := make([]string, len(nodes))
	for i, node := range nodes {
		members[i] = fmt.Sprintf("member@%s", node.Config.Name)
	}

	log.Info("Creating privacy group for pente test...")
	group, err := nodes[0].HTTPClient.PrivacyGroups().CreateGroup(s.ctx, &pldapi.PrivacyGroupInput{
		Domain:  "pente",
		Members: members,
		Name:    "perf-test-privacy-group",
		Configuration: map[string]string{
			"evmVersion":           "shanghai",
			"externalCallsEnabled": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create privacy group: %w", err)
	}

	s.privacyGroupID = &group.ID
	log.Infof("Privacy group created with ID: %s", group.ID)

	log.Info("Waiting for privacy group creation receipt...")
	receipt, err := util.WaitForTransactionReceipt(s.ctx, nodes[0].HTTPClient, group.GenesisTransaction, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get privacy group creation receipt: %w", err)
	}
	if !receipt.Success {
		return fmt.Errorf("privacy group creation transaction failed")
	}
	log.Info("Privacy group creation confirmed")

	var function *abi.Entry
	for _, entry := range simpleStorage.ABI {
		if entry.Type == abi.Constructor {
			function = entry
			break
		}
	}

	log.Info("Deploying contract to privacy group...")
	deployTxID, err := nodes[0].HTTPClient.PrivacyGroups().SendTransaction(s.ctx, &pldapi.PrivacyGroupEVMTXInput{
		Domain: "pente",
		Group:  *s.privacyGroupID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "member",
			Bytecode: simpleStorage.Bytecode,
			Function: function,
			Input:    pldtypes.RawJSON(fmt.Sprintf("[%d]", 0)),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to deploy contract: %w", err)
	}

	log.Info("Waiting for contract deployment receipt...")
	deployReceipt, err := util.WaitForTransactionReceiptFull(s.ctx, nodes[0].HTTPClient, deployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get contract deployment receipt: %w", err)
	}
	if !deployReceipt.Success {
		return fmt.Errorf("contract deployment transaction failed")
	}

	if len(nodes) > 1 {
		log.Info("Waiting for contract deployment domain receipt to index on peer nodes...")
		for i := 1; i < len(nodes); i++ {
			node := nodes[i]
			log.Infof("Waiting for deployment domain receipt on node %s...", node.Config.Name)
			_, err = util.WaitForDomainReceipt(s.ctx, node.HTTPClient, "pente", deployTxID, 60*time.Second)
			if err != nil {
				return fmt.Errorf("failed waiting for deployment domain receipt on node %s: %w", node.Config.Name, err)
			}
			log.Infof("Deployment domain receipt indexed on node %s", node.Config.Name)
		}
	}

	var addr *pldtypes.EthAddress
	if deployReceipt.DomainReceipt != nil {
		var domainReceipt map[string]interface{}
		if err := json.Unmarshal(deployReceipt.DomainReceipt, &domainReceipt); err == nil {
			if receiptData, ok := domainReceipt["receipt"].(map[string]interface{}); ok {
				if addrStr, ok := receiptData["contractAddress"].(string); ok {
					addr = pldtypes.MustEthAddress(addrStr)
					log.Infof("Contract deployed at address: %s", *addr)
				}
			}
		}
	}

	if addr == nil {
		return fmt.Errorf("contract address not found in deployment receipt")
	}
	s.contractAddress = addr

	// Create receipt listener (stays in Setup per plan)
	var latestSequence *uint64
	qb := query.NewQueryBuilder().Equal("domain", "pente").Sort("-sequence").Limit(1)
	receipts, err := nodes[0].HTTPClient.PTX().QueryTransactionReceipts(s.ctx, qb.Query())
	if err == nil && len(receipts) > 0 {
		seq := receipts[0].Sequence
		latestSequence = &seq
		log.Infof("Found latest sequence: %d, will start listener from sequence above this", seq)
	} else {
		log.Info("No existing receipts found, starting listener from beginning")
	}

	_, _ = nodes[0].HTTPClient.PTX().DeleteReceiptListener(s.ctx, "penteperflistener")

	txType := pldapi.TransactionTypePrivate.Enum()
	_, err = nodes[0].HTTPClient.PTX().CreateReceiptListener(s.ctx, &pldapi.TransactionReceiptListener{
		Name: "penteperflistener",
		Filters: pldapi.TransactionReceiptFilters{
			Type:          &txType,
			Domain:        "pente",
			SequenceAbove: latestSequence,
		},
		Options: pldapi.TransactionReceiptListenerOptions{
			DomainReceipts: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create receipt listener: %w", err)
	}

	return nil
}

func (s *privateTransactionNodeRestartSuite) Subscribe() (rpcclient.Subscription, error) {
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes configured")
	}
	sub, err := nodes[0].WSClient.PTX().SubscribeReceipts(s.ctx, "penteperflistener")
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to pente receipts: %w", err)
	}
	s.sub = sub
	return sub, nil
}

func (s *privateTransactionNodeRestartSuite) Unsubscribe() {
	if s.sub != nil {
		if err := s.sub.Unsubscribe(s.ctx); err != nil {
			log.Errorf("Error unsubscribing from subscription: %s", err.Error())
		} else {
			log.Info("Successfully unsubscribed")
		}
		s.sub = nil
	}
}

func (s *privateTransactionNodeRestartSuite) Cleanup() {
	nodes := s.runner.GetNodes()
	if len(nodes) > 0 {
		_, err := nodes[0].HTTPClient.PTX().DeleteReceiptListener(s.ctx, "penteperflistener")
		if err != nil {
			log.Debugf("Failed to delete receipt listener penteperflistener: %v", err)
		} else {
			log.Infof("Successfully deleted receipt listener: penteperflistener")
		}
	}
}

func (s *privateTransactionNodeRestartSuite) NewWorker(startTime int64, workerID int) TestCase {
	return newPrivateTransactionNodeRestartTestWorker(s.ctx, startTime, workerID, s.privacyGroupID, s.contractAddress, s.runner)
}

func (s *privateTransactionNodeRestartSuite) PostRun() error {
	if s.contractAddress == nil {
		return fmt.Errorf("contract address not set for post-run analysis")
	}
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes configured")
	}

	log.Infof("Running post-run private transaction analysis for contract %s", *s.contractAddress)

	var createdCursor pldtypes.Timestamp
	totalTransactions := 0
	multiPublicTransactions := 0
	multiPublicTransactionIDs := make([]string, 0)
	pageCount := 0

	for {
		qb := query.NewQueryBuilder().
			Equal("domain", "pente").
			Equal("to", *s.contractAddress).
			Sort("-created").
			Limit(privateTxPostRunPageSize)
		if createdCursor != 0 {
			qb = qb.LessThan("created", createdCursor)
		}

		txs, err := nodes[0].HTTPClient.PTX().QueryTransactionsFull(s.ctx, qb.Query())
		if err != nil {
			return fmt.Errorf("post-run queryTransactionsFull failed for contract %s with created cursor %s: %w", *s.contractAddress, createdCursor.String(), err)
		}
		if len(txs) == 0 {
			break
		}

		pageCount++
		totalTransactions += len(txs)

		for _, tx := range txs {
			if tx != nil && len(tx.Public) > 1 {
				multiPublicTransactions++
				if tx.ID != nil {
					multiPublicTransactionIDs = append(multiPublicTransactionIDs, tx.ID.String())
				}
			}
		}

		createdCursor = txs[len(txs)-1].Created
		if len(txs) < privateTxPostRunPageSize {
			break
		}
	}

	log.Infof(
		"Private transaction post-run analysis complete for contract %s: scanned %d transactions across %d pages; %d had more than one public transaction",
		*s.contractAddress,
		totalTransactions,
		pageCount,
		multiPublicTransactions,
	)
	if len(multiPublicTransactionIDs) > 0 {
		log.Infof(
			"Transaction IDs with more than one public submission (%d): %v",
			len(multiPublicTransactionIDs),
			multiPublicTransactionIDs,
		)
	}
	return nil
}

type privateTransactionNodeRestart struct {
	testBase
	privacyGroupID  *pldtypes.HexBytes
	contractAddress *pldtypes.EthAddress
	runner          Runner
	random          *rand.Rand
}

func newPrivateTransactionNodeRestartTestWorker(ctx context.Context, startTime int64, workerID int, privacyGroupID *pldtypes.HexBytes, contractAddress *pldtypes.EthAddress, runner Runner) TestCase {
	return &privateTransactionNodeRestart{
		testBase: testBase{
			ctx:       ctx,
			startTime: startTime,
			workerID:  workerID,
		},
		privacyGroupID:  privacyGroupID,
		contractAddress: contractAddress,
		runner:          runner,
		random:          rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID))),
	}
}

func (tc *privateTransactionNodeRestart) Name() conf.TestName {
	return conf.PerfTestPrivateTransactionNodeRestart
}

func (tc *privateTransactionNodeRestart) RunOnce(iterationCount int) (string, error) {
	nodes := tc.runner.GetNodes()
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes configured")
	}
	nodeIndex := tc.random.Intn(len(nodes))
	client := nodes[nodeIndex].HTTPClient

	setFunctionABI := &abi.Entry{
		Name: "set",
		Type: "function",
		Inputs: abi.ParameterArray{
			{
				Name: "newValue",
				Type: "uint256",
			},
		},
	}

	inputData := map[string]interface{}{
		"newValue": tc.workerID,
	}
	inputJSON, err := json.Marshal(inputData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal input data: %w", err)
	}

	idempotencyKey := util.GetIdempotencyKey(tc.startTime, tc.workerID, iterationCount)
	txInput := &pldapi.PrivacyGroupEVMTXInput{
		Domain: "pente",
		Group:  *tc.privacyGroupID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "member",
			To:       tc.contractAddress,
			Function: setFunctionABI,
			Input:    pldtypes.RawJSON(inputJSON),
		},
		IdempotencyKey: idempotencyKey,
	}

	retryPolicy := retry.NewRetryLimited(&pldconf.RetryConfigWithMax{
		RetryConfig: pldconf.RetryConfig{
			InitialDelay: confutil.P("250ms"),
			MaxDelay:     confutil.P("2s"),
			Factor:       confutil.P(2.0),
		},
		MaxAttempts: confutil.P(privateTxRetryAttempts),
	})
	var txID string
	err = retryPolicy.Do(tc.ctx, func(attempt int) (bool, error) {
		sentTxID, sendErr := client.PrivacyGroups().SendTransaction(tc.ctx, txInput)
		if sendErr != nil {
			// Don't retry idempotency key clashes, because this indicates the server rejected deduplication semantics.
			if strings.Contains(sendErr.Error(), txMgrIdempotencyKeyClashErrorCode) {
				return false, sendErr
			}
			log.Warnf("Worker %d send attempt %d/%d failed on node %d: %v", tc.workerID, attempt, privateTxRetryAttempts, nodeIndex, sendErr)
			return true, sendErr
		}
		txID = sentTxID.String()
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to send pente transaction to node %d after %d attempts: %w", nodeIndex, privateTxRetryAttempts, err)
	}

	log.Debugf("Worker %d sent pente transaction %s to node %d", tc.workerID, txID, nodeIndex)
	return txID, nil
}
