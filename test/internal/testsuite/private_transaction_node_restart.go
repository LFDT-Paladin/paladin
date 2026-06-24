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
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/retry"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/test/internal/conf"
	"github.com/LFDT-Paladin/paladin/test/internal/contracts"
	"github.com/LFDT-Paladin/paladin/test/internal/util"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	log "github.com/sirupsen/logrus"
)

// mergedSubscription fans notifications from multiple per-node subscriptions into a
// single channel. Successful pente transactions arrive from all N nodes (the block
// indexer on every node sees the on-chain event), while reverted transactions produce
// a receipt only on the originator node. By subscribing to all nodes we see both
// cases. Duplicates for successful transactions are handled naturally by the
// workerIDMap.LoadAndDelete call in the perf runner's batchEventLoop.
type mergedSubscription struct {
	id            uuid.UUID
	subs          []rpcclient.Subscription
	notifications chan rpcclient.RPCSubscriptionNotification
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

func newMergedSubscription(ctx context.Context, subs []rpcclient.Subscription) *mergedSubscription {
	mergedCtx, cancel := context.WithCancel(ctx)
	m := &mergedSubscription{
		id:            uuid.New(),
		subs:          subs,
		notifications: make(chan rpcclient.RPCSubscriptionNotification),
		ctx:           mergedCtx,
		cancel:        cancel,
	}
	for _, s := range subs {
		m.wg.Add(1)
		go m.forward(s)
	}
	// Close the merged channel once every forwarder has exited.
	go func() {
		m.wg.Wait()
		close(m.notifications)
	}()
	return m
}

func (m *mergedSubscription) forward(s rpcclient.Subscription) {
	defer m.wg.Done()
	for {
		select {
		case n, ok := <-s.Notifications():
			if !ok {
				return
			}
			select {
			case m.notifications <- n:
			case <-m.ctx.Done():
				return
			}
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *mergedSubscription) LocalID() uuid.UUID { return m.id }

func (m *mergedSubscription) Notifications() chan rpcclient.RPCSubscriptionNotification {
	return m.notifications
}

func (m *mergedSubscription) Unsubscribe(ctx context.Context) rpcclient.ErrorRPC {
	m.cancel() // stop forwarder goroutines
	var lastErr rpcclient.ErrorRPC
	for _, s := range m.subs {
		if err := s.Unsubscribe(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// nodeAndTxID carries a completed txID together with the index of the node it was submitted to.
type nodeAndTxID struct {
	txID      string
	nodeIndex int
}

type privateTransactionNodeRestartSuite struct {
	ctx             context.Context
	runner          Runner
	privacyGroupID  *pldtypes.HexBytes
	contractAddress *pldtypes.EthAddress
	sub             rpcclient.Subscription
	// submittedTxIDs holds txID→struct{} per node. Entries are deleted when
	// OnReceiptBatch processes them; remaining entries at PostRun time are
	// transactions that never received a completion (e.g. in-flight at test end).
	submittedTxIDs []*sync.Map // one entry per node, indexed by position in GetNodes()
	// txIDToNode provides O(1) reverse lookup: txID → nodeIndex.
	txIDToNode sync.Map

	resultsMu sync.Mutex
	failures  []string
}

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

	s.submittedTxIDs = make([]*sync.Map, len(nodes))
	for i := range nodes {
		s.submittedTxIDs[i] = &sync.Map{}
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

	// Create a receipt listener on every node so we capture both successful receipts
	// (written by all nodes via the block-indexer event stream) and reverted receipts
	// (written only by the originator node). The per-node sequenceAbove values are
	// sampled independently because each node has its own sequence counter.
	txType := pldapi.TransactionTypePrivate.Enum()
	qb := query.NewQueryBuilder().Equal("domain", "pente").Sort("-sequence").Limit(1)

	for _, node := range nodes {
		var latestSequence *uint64
		receipts, qErr := node.HTTPClient.PTX().QueryTransactionReceipts(s.ctx, qb.Query())
		if qErr == nil && len(receipts) > 0 {
			seq := receipts[0].Sequence
			latestSequence = &seq
			log.Infof("Node %s: found latest pente sequence %d, starting listener above this", node.Config.Name, seq)
		} else {
			log.Infof("Node %s: no existing pente receipts found, starting listener from beginning", node.Config.Name)
		}

		_, _ = node.HTTPClient.PTX().DeleteReceiptListener(s.ctx, "penteperflistener")

		_, err = node.HTTPClient.PTX().CreateReceiptListener(s.ctx, &pldapi.TransactionReceiptListener{
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
			return fmt.Errorf("failed to create receipt listener on node %s: %w", node.Config.Name, err)
		}
	}

	return nil
}

func (s *privateTransactionNodeRestartSuite) Subscribe() (rpcclient.Subscription, error) {
	nodes := s.runner.GetNodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes configured")
	}

	subs := make([]rpcclient.Subscription, 0, len(nodes))
	for _, node := range nodes {
		sub, err := node.WSClient.PTX().SubscribeReceipts(s.ctx, "penteperflistener")
		if err != nil {
			// Unsubscribe any already-opened subscriptions before returning the error.
			for _, opened := range subs {
				_ = opened.Unsubscribe(s.ctx)
			}
			return nil, fmt.Errorf("failed to subscribe to pente receipts on node %s: %w", node.Config.Name, err)
		}
		log.Infof("Subscribed to penteperflistener on node %s", node.Config.Name)
		subs = append(subs, sub)
	}

	merged := newMergedSubscription(s.ctx, subs)
	s.sub = merged
	return merged, nil
}

// OnReceiptBatch is called by the runner every N completions. It resolves the originating
// node for each txID, validates the receipt, and records failures. Concurrent calls are
// safe via resultsMu and sync.Map.
func (s *privateTransactionNodeRestartSuite) OnReceiptBatch(txIDs []string) {
	entries := make([]nodeAndTxID, 0, len(txIDs))
	for _, id := range txIDs {
		v, ok := s.txIDToNode.LoadAndDelete(id)
		if !ok {
			continue
		}
		nodeIndex := v.(int)
		s.submittedTxIDs[nodeIndex].Delete(id)
		entries = append(entries, nodeAndTxID{txID: id, nodeIndex: nodeIndex})
	}
	if len(entries) > 0 {
		s.checkBatch(entries)
	}
}

func (s *privateTransactionNodeRestartSuite) checkBatch(entries []nodeAndTxID) {
	nodes := s.runner.GetNodes()
	var batchFailures []string
	for _, entry := range entries {
		if entry.nodeIndex < 0 || entry.nodeIndex >= len(nodes) {
			batchFailures = append(batchFailures, fmt.Sprintf("tx %s: invalid node index %d", entry.txID, entry.nodeIndex))
			continue
		}
		txID := uuid.MustParse(entry.txID)
		node := nodes[entry.nodeIndex]
		r, err := node.HTTPClient.PTX().GetTransactionReceipt(s.ctx, txID)
		if err != nil {
			batchFailures = append(batchFailures, fmt.Sprintf("tx %s: error querying receipt on node %s: %v",
				entry.txID, node.Config.Name, err))
			continue
		}
		if r == nil {
			batchFailures = append(batchFailures, fmt.Sprintf("tx %s: no receipt found on node %s",
				entry.txID, node.Config.Name))
			continue
		}
		if !r.Success {
			batchFailures = append(batchFailures, fmt.Sprintf("tx %s failed on node %s: %s",
				entry.txID, node.Config.Name, r.FailureMessage))
		}
	}

	log.Infof("privateTransactionNodeRestartSuite: rolling check batch=%d failures=%d", len(entries), len(batchFailures))

	if len(batchFailures) > 0 {
		for _, f := range batchFailures {
			log.Errorf("privateTransactionNodeRestartSuite: rolling check failure: %s", f)
		}
		s.resultsMu.Lock()
		s.failures = append(s.failures, batchFailures...)
		s.resultsMu.Unlock()
	}
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
	for _, node := range s.runner.GetNodes() {
		_, err := node.HTTPClient.PTX().DeleteReceiptListener(s.ctx, "penteperflistener")
		if err != nil {
			log.Debugf("Node %s: failed to delete receipt listener penteperflistener: %v", node.Config.Name, err)
		} else {
			log.Infof("Node %s: successfully deleted receipt listener penteperflistener", node.Config.Name)
		}
	}
}

func (s *privateTransactionNodeRestartSuite) NewWorker(startTime int64, workerID int) TestCase {
	return newPrivateTransactionNodeRestartTestWorker(s.ctx, startTime, workerID, s.privacyGroupID, s.contractAddress, s.runner, s.submittedTxIDs, &s.txIDToNode)
}

func (s *privateTransactionNodeRestartSuite) PostRun() error {
	// Check any txIDs that were submitted but never received a completion notification —
	// e.g. transactions still in-flight when the node kill/restart test ended.
	nodes := s.runner.GetNodes()
	for i, nodeMap := range s.submittedTxIDs {
		if i >= len(nodes) {
			break
		}
		node := nodes[i]
		nodeMap.Range(func(key, _ any) bool {
			txID := uuid.MustParse(key.(string))
			r, err := node.HTTPClient.PTX().GetTransactionReceipt(s.ctx, txID)
			if err != nil {
				s.resultsMu.Lock()
				s.failures = append(s.failures, fmt.Sprintf("tx %s: error querying receipt on node %s: %v",
					txID, node.Config.Name, err))
				s.resultsMu.Unlock()
				return true
			}
			if r == nil {
				s.resultsMu.Lock()
				s.failures = append(s.failures, fmt.Sprintf("tx %s: no receipt found on node %s",
					txID, node.Config.Name))
				s.resultsMu.Unlock()
				return true
			}
			if !r.Success {
				s.resultsMu.Lock()
				s.failures = append(s.failures, fmt.Sprintf("tx %s failed on node %s: %s",
					txID, node.Config.Name, r.FailureMessage))
				s.resultsMu.Unlock()
			}
			return true
		})
	}

	s.resultsMu.Lock()
	defer s.resultsMu.Unlock()

	log.Infof("privateTransactionNodeRestartSuite: post-run complete, total failures=%d", len(s.failures))

	if len(s.failures) > 0 {
		return fmt.Errorf("%d transaction(s) failed:\n%s", len(s.failures), strings.Join(s.failures, "\n"))
	}
	return nil
}

type privateTransactionNodeRestart struct {
	testBase
	privacyGroupID  *pldtypes.HexBytes
	contractAddress *pldtypes.EthAddress
	runner          Runner
	random          *rand.Rand
	submittedTxIDs  []*sync.Map // indexed by node position, shared with suite
	txIDToNode      *sync.Map   // shared with suite for O(1) reverse lookup
}

func newPrivateTransactionNodeRestartTestWorker(ctx context.Context, startTime int64, workerID int, privacyGroupID *pldtypes.HexBytes, contractAddress *pldtypes.EthAddress, runner Runner, submittedTxIDs []*sync.Map, txIDToNode *sync.Map) TestCase {
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
		submittedTxIDs:  submittedTxIDs,
		txIDToNode:      txIDToNode,
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

	tc.submittedTxIDs[nodeIndex].Store(txID, struct{}{})
	tc.txIDToNode.Store(txID, nodeIndex)
	return txID, nil
}
