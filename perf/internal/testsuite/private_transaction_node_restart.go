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
	"time"

	"github.com/LFDT-Paladin/paladin/perf/internal/contracts"
	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	"github.com/LFDT-Paladin/paladin/perf/internal/util"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	log "github.com/sirupsen/logrus"
)

type privateTransactionNodeRestartSuite struct {
	ctx             context.Context
	httpClients     []pldclient.PaladinClient
	wsClient        pldclient.PaladinWSClient
	nodes           []conf.NodeConfig
	privacyGroupID  *pldtypes.HexBytes
	contractAddress *pldtypes.EthAddress
	sub             rpcclient.Subscription
}

// NewPrivateTransactionNodeRestartSuite creates a new private transaction node restart test suite with the given context and clients.
func NewPrivateTransactionNodeRestartSuite(ctx context.Context, httpClients []pldclient.PaladinClient, wsClient pldclient.PaladinWSClient, nodes []conf.NodeConfig) *privateTransactionNodeRestartSuite {
	return &privateTransactionNodeRestartSuite{ctx: ctx, httpClients: httpClients, wsClient: wsClient, nodes: nodes}
}

func (s *privateTransactionNodeRestartSuite) Setup() error {
	log.Infof("Running private transaction node restart test")

	simpleStorage, err := contracts.LoadSimpleStorageContract()
	if err != nil {
		return err
	}

	members := make([]string, len(s.nodes))
	for i, node := range s.nodes {
		members[i] = fmt.Sprintf("member@%s", node.Name)
	}

	log.Info("Creating privacy group for pente test...")
	group, err := s.httpClients[0].PrivacyGroups().CreateGroup(s.ctx, &pldapi.PrivacyGroupInput{
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
	receipt, err := util.WaitForTransactionReceipt(s.ctx, s.httpClients[0], group.GenesisTransaction, 60*time.Second)
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
	deployTxID, err := s.httpClients[0].PrivacyGroups().SendTransaction(s.ctx, &pldapi.PrivacyGroupEVMTXInput{
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
	deployReceipt, err := util.WaitForTransactionReceiptFull(s.ctx, s.httpClients[0], deployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get contract deployment receipt: %w", err)
	}
	if !deployReceipt.Success {
		return fmt.Errorf("contract deployment transaction failed")
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

	return nil
}

func (s *privateTransactionNodeRestartSuite) Subscribe() (rpcclient.Subscription, error) {
	var latestSequence *uint64
	qb := query.NewQueryBuilder().Equal("domain", "pente").Sort("-sequence").Limit(1)
	receipts, err := s.httpClients[0].PTX().QueryTransactionReceipts(s.ctx, qb.Query())
	if err == nil && len(receipts) > 0 {
		seq := receipts[0].Sequence
		latestSequence = &seq
		log.Infof("Found latest sequence: %d, will start listener from sequence above this", seq)
	} else {
		log.Info("No existing receipts found, starting listener from beginning")
	}

	_, err = s.httpClients[0].PTX().DeleteReceiptListener(s.ctx, "penteperflistener")
	if err != nil {
		log.Debugf("No existing listener to delete (or delete failed): %v", err)
	}

	txType := pldapi.TransactionTypePrivate.Enum()
	_, err = s.httpClients[0].PTX().CreateReceiptListener(s.ctx, &pldapi.TransactionReceiptListener{
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
		return nil, fmt.Errorf("failed to create receipt listener: %w", err)
	}

	sub, err := s.wsClient.PTX().SubscribeReceipts(s.ctx, "penteperflistener")
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
	_, err := s.httpClients[0].PTX().DeleteReceiptListener(s.ctx, "penteperflistener")
	if err != nil {
		log.Debugf("Failed to delete receipt listener penteperflistener: %v", err)
	} else {
		log.Infof("Successfully deleted receipt listener: penteperflistener")
	}
}

func (s *privateTransactionNodeRestartSuite) NewWorker(startTime int64, workerID int) TestCase {
	return newPrivateTransactionNodeRestartTestWorker(s.ctx, startTime, workerID, s.privacyGroupID, s.contractAddress, s.httpClients)
}

type privateTransactionNodeRestart struct {
	testBase
	privacyGroupID  *pldtypes.HexBytes
	contractAddress *pldtypes.EthAddress
	httpClients     []pldclient.PaladinClient
	random          *rand.Rand
}

func newPrivateTransactionNodeRestartTestWorker(ctx context.Context, startTime int64, workerID int, privacyGroupID *pldtypes.HexBytes, contractAddress *pldtypes.EthAddress, httpClients []pldclient.PaladinClient) TestCase {
	return &privateTransactionNodeRestart{
		testBase: testBase{
			ctx:       ctx,
			startTime: startTime,
			workerID:  workerID,
		},
		privacyGroupID:  privacyGroupID,
		contractAddress: contractAddress,
		httpClients:     httpClients,
		random:          rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID))),
	}
}

func (tc *privateTransactionNodeRestart) Name() conf.TestName {
	return conf.PerfTestPrivateTransactionNodeRestart
}

func (tc *privateTransactionNodeRestart) RunOnce(iterationCount int) (string, error) {
	if len(tc.httpClients) == 0 {
		return "", fmt.Errorf("no HTTP clients available")
	}
	nodeIndex := tc.random.Intn(len(tc.httpClients))
	client := tc.httpClients[nodeIndex]

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

	txID, err := client.PrivacyGroups().SendTransaction(tc.ctx, &pldapi.PrivacyGroupEVMTXInput{
		Domain: "pente",
		Group:  *tc.privacyGroupID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "member",
			To:       tc.contractAddress,
			Function: setFunctionABI,
			Input:    pldtypes.RawJSON(inputJSON),
		},
		IdempotencyKey: util.GetIdempotencyKey(tc.startTime, tc.workerID, iterationCount),
	})
	if err != nil {
		return "", fmt.Errorf("failed to send pente transaction to node %d: %w", nodeIndex, err)
	}

	log.Debugf("Worker %d sent pente transaction %s to node %d", tc.workerID, txID, nodeIndex)
	return txID.String(), nil
}
