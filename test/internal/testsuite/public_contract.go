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
	"fmt"
	"time"

	"github.com/LFDT-Paladin/paladin/test/internal/conf"
	"github.com/LFDT-Paladin/paladin/test/internal/contracts"
	"github.com/LFDT-Paladin/paladin/test/internal/util"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	log "github.com/sirupsen/logrus"
)

type publicContractSuite struct {
	ctx             context.Context
	runner          Runner
	contractAddress *pldtypes.EthAddress
	sub             rpcclient.Subscription
	abiRef          *pldtypes.Bytes32
}

// NewPublicContractSuite creates a new public contract test suite with the given context and runner.
func NewPublicContractSuite(ctx context.Context, runner Runner) *publicContractSuite {
	return &publicContractSuite{ctx: ctx, runner: runner}
}

func (s *publicContractSuite) Setup() error {
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes configured")
	}
	log.Infof("Running public contract test using first configured node: %s", nodes[0].Config.HTTPEndpoint)

	simpleStorage, err := contracts.LoadSimpleStorageContract()
	if err != nil {
		return err
	}

	// create ABI separately so we know its ref
	hash, err := nodes[0].HTTPClient.PTX().StoreABI(s.ctx, simpleStorage.ABI)
	if err != nil {
		return fmt.Errorf("failed to store ABI: %w", err)
	}

	s.abiRef = &hash

	log.Info("Deploying contract as public transaction...")
	deployTxID, err := nodes[0].HTTPClient.PTX().SendTransaction(s.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "deploy",
			Data: pldtypes.RawJSON(fmt.Sprintf("[%d]", 0)),
		},
		Bytecode: simpleStorage.Bytecode,
		ABI:      simpleStorage.ABI,
	})
	if err != nil {
		return fmt.Errorf("failed to deploy contract: %w", err)
	}

	log.Info("Waiting for contract deployment receipt...")
	deployReceipt, err := util.WaitForTransactionReceiptFull(s.ctx, nodes[0].HTTPClient, *deployTxID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get contract deployment receipt: %w", err)
	}
	if !deployReceipt.Success {
		return fmt.Errorf("contract deployment transaction failed")
	}

	if deployReceipt.ContractAddress != nil {
		s.contractAddress = deployReceipt.ContractAddress
		log.Infof("Contract deployed at address: %s", *deployReceipt.ContractAddress)
	} else {
		return fmt.Errorf("contract address not found in deployment receipt")
	}

	// Create receipt listener (stays in Setup per plan; listener is not deleted by Unsubscribe or node kill)
	const listenerName = "publiclistener"
	var latestSequence *uint64
	qb := query.NewQueryBuilder().Sort("-sequence").Limit(1)
	receipts, err := nodes[0].HTTPClient.PTX().QueryTransactionReceipts(s.ctx, qb.Query())
	if err == nil && len(receipts) > 0 {
		seq := receipts[0].Sequence
		latestSequence = &seq
		log.Infof("Found latest sequence: %d, will start listener from sequence above this", seq)
	} else {
		log.Info("No existing receipts found, starting listener from beginning")
	}

	_, _ = nodes[0].HTTPClient.PTX().DeleteReceiptListener(s.ctx, listenerName)

	txType := pldapi.TransactionTypePublic.Enum()
	_, err = nodes[0].HTTPClient.PTX().CreateReceiptListener(s.ctx, &pldapi.TransactionReceiptListener{
		Name: listenerName,
		Filters: pldapi.TransactionReceiptFilters{
			Type:          &txType,
			SequenceAbove: latestSequence,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create receipt listener: %w", err)
	}

	return nil
}

func (s *publicContractSuite) Subscribe() (rpcclient.Subscription, error) {
	const listenerName = "publiclistener"
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes configured")
	}
	sub, err := nodes[0].WSClient.PTX().SubscribeReceipts(s.ctx, listenerName)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to public receipts: %w", err)
	}
	s.sub = sub
	return sub, nil
}

func (s *publicContractSuite) Unsubscribe() {
	if s.sub != nil {
		if err := s.sub.Unsubscribe(s.ctx); err != nil {
			log.Errorf("Error unsubscribing from subscription: %s", err.Error())
		} else {
			log.Info("Successfully unsubscribed")
		}
		s.sub = nil
	}
}

func (s *publicContractSuite) Cleanup() {
	nodes := s.runner.GetNodes()
	if len(nodes) > 0 {
		_, err := nodes[0].HTTPClient.PTX().DeleteReceiptListener(s.ctx, "publiclistener")
		if err != nil {
			log.Debugf("Failed to delete receipt listener publiclistener: %v", err)
		} else {
			log.Infof("Successfully deleted receipt listener: publiclistener")
		}
	}
}

func (s *publicContractSuite) NewWorker(startTime int64, workerID int) TestCase {
	return &publicContract{
		testBase: testBase{
			ctx:       s.ctx,
			startTime: startTime,
			workerID:  workerID,
		},
		contractAddress: s.contractAddress,
		runner:          s.runner,
		abiRef:          s.abiRef,
	}
}

func (s *publicContractSuite) PostRun() error {
	return nil
}

type publicContract struct {
	testBase
	contractAddress *pldtypes.EthAddress
	runner          Runner
	abiRef          *pldtypes.Bytes32
}

func (tc *publicContract) Name() conf.TestName {
	return conf.PerfTestPublicContract
}

func (tc *publicContract) RunOnce(iterationCount int) (string, error) {
	if tc.contractAddress == nil {
		return "", fmt.Errorf("contract address not set - contract deployment may have failed")
	}
	nodes := tc.runner.GetNodes()
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes configured")
	}

	result, err := nodes[0].HTTPClient.PTX().SendTransaction(tc.ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:         pldapi.TransactionTypePublic.Enum(),
			Function:     "set",
			To:           tc.contractAddress,
			ABIReference: tc.abiRef,
			// This test is more valuable if it uses different signing keys, otherwise it only exercises
			// a single transaction orchestrator. This approach works when using the default paladin
			// wallet, but may require additional configuration if testing with an external wallet
			From:           fmt.Sprintf("test%d", tc.workerID),
			Data:           pldtypes.RawJSON(fmt.Sprintf("[%d]", tc.workerID)),
			IdempotencyKey: util.GetIdempotencyKey(tc.startTime, tc.workerID, iterationCount),
		},
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprint(result), nil
}
