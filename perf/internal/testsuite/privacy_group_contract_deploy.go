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
	"fmt"
	"math/rand"
	"time"

	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	"github.com/LFDT-Paladin/paladin/perf/internal/contracts"
	"github.com/LFDT-Paladin/paladin/perf/internal/util"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	log "github.com/sirupsen/logrus"
)

const privacyGroupContractDeployListenerName = "penteprivacygroupdeploylistener"

type privacyGroupContractDeploySuite struct {
	ctx         context.Context
	runner      Runner
	sub         rpcclient.Subscription
	constructor *abi.Entry
	bytecode    pldtypes.HexBytes
}

// NewPrivacyGroupContractDeploySuite creates a new privacy group contract deploy test suite.
func NewPrivacyGroupContractDeploySuite(ctx context.Context, runner Runner) *privacyGroupContractDeploySuite {
	return &privacyGroupContractDeploySuite{ctx: ctx, runner: runner}
}

func (s *privacyGroupContractDeploySuite) Setup() error {
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes configured")
	}
	log.Infof("Running privacy group contract deploy test")

	simpleStorage, err := contracts.LoadSimpleStorageContract()
	if err != nil {
		return err
	}
	s.bytecode = simpleStorage.Bytecode
	for _, entry := range simpleStorage.ABI {
		if entry.Type == abi.Constructor {
			s.constructor = entry
			break
		}
	}

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

	_, _ = nodes[0].HTTPClient.PTX().DeleteReceiptListener(s.ctx, privacyGroupContractDeployListenerName)

	txType := pldapi.TransactionTypePrivate.Enum()
	_, err = nodes[0].HTTPClient.PTX().CreateReceiptListener(s.ctx, &pldapi.TransactionReceiptListener{
		Name: privacyGroupContractDeployListenerName,
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

func (s *privacyGroupContractDeploySuite) Subscribe() (rpcclient.Subscription, error) {
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes configured")
	}
	sub, err := nodes[0].WSClient.PTX().SubscribeReceipts(s.ctx, privacyGroupContractDeployListenerName)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to pente receipts: %w", err)
	}
	s.sub = sub
	return sub, nil
}

func (s *privacyGroupContractDeploySuite) Unsubscribe() {
	if s.sub != nil {
		if err := s.sub.Unsubscribe(s.ctx); err != nil {
			log.Errorf("Error unsubscribing from subscription: %s", err.Error())
		} else {
			log.Info("Successfully unsubscribed")
		}
		s.sub = nil
	}
}

func (s *privacyGroupContractDeploySuite) Cleanup() {
	nodes := s.runner.GetNodes()
	if len(nodes) > 0 {
		_, err := nodes[0].HTTPClient.PTX().DeleteReceiptListener(s.ctx, privacyGroupContractDeployListenerName)
		if err != nil {
			log.Debugf("Failed to delete receipt listener %s: %v", privacyGroupContractDeployListenerName, err)
		} else {
			log.Infof("Successfully deleted receipt listener: %s", privacyGroupContractDeployListenerName)
		}
	}
}

func (s *privacyGroupContractDeploySuite) NewWorker(startTime int64, workerID int) TestCase {
	return &privacyGroupContractDeploy{
		testBase: testBase{
			ctx:       s.ctx,
			startTime: startTime,
			workerID:  workerID,
		},
		runner:      s.runner,
		random:      rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID))),
		bytecode:    s.bytecode,
		constructor: s.constructor,
	}
}

func (s *privacyGroupContractDeploySuite) PostRun() error {
	return nil
}

type privacyGroupContractDeploy struct {
	testBase
	runner      Runner
	random      *rand.Rand
	bytecode    pldtypes.HexBytes
	constructor *abi.Entry
}

func (tc *privacyGroupContractDeploy) Name() conf.TestName {
	return conf.PerfTestPrivacyGroupContractDeploy
}

func (tc *privacyGroupContractDeploy) RunOnce(iterationCount int) (string, error) {
	nodes := tc.runner.GetNodes()
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes configured")
	}

	nodeIndex := tc.random.Intn(len(nodes))
	client := nodes[nodeIndex].HTTPClient

	members := make([]string, len(nodes))
	for i, node := range nodes {
		members[i] = fmt.Sprintf("member@%s", node.Config.Name)
	}

	groupName := fmt.Sprintf("perf-test-privacy-group-%d-%d-%d", tc.startTime, tc.workerID, iterationCount)
	group, err := client.PrivacyGroups().CreateGroup(tc.ctx, &pldapi.PrivacyGroupInput{
		Domain:  "pente",
		Members: members,
		Name:    groupName,
		Configuration: map[string]string{
			"evmVersion":           "shanghai",
			"externalCallsEnabled": "true",
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create privacy group on node %d: %w", nodeIndex, err)
	}

	groupReceipt, err := util.WaitForTransactionReceipt(tc.ctx, client, group.GenesisTransaction, 60*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to get privacy group creation receipt: %w", err)
	}
	if !groupReceipt.Success {
		return "", fmt.Errorf("privacy group creation transaction failed")
	}

	deployTxID, err := client.PrivacyGroups().SendTransaction(tc.ctx, &pldapi.PrivacyGroupEVMTXInput{
		Domain: "pente",
		Group:  group.ID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "member",
			Bytecode: tc.bytecode,
			Function: tc.constructor,
			Input:    pldtypes.RawJSON(fmt.Sprintf("[%d]", 0)),
		},
		IdempotencyKey: util.GetIdempotencyKey(tc.startTime, tc.workerID, iterationCount),
	})
	if err != nil {
		return "", fmt.Errorf("failed to deploy contract to privacy group on node %d: %w", nodeIndex, err)
	}

	return deployTxID.String(), nil
}
