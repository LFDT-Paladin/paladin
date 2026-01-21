/*
 * Copyright © 2024 Kaleido, Inc.
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

package integrationtest

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/pkg/testbed"
	"github.com/LFDT-Paladin/paladin/domains/integration-test/helpers"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zeto"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

//go:embed helpers/abis/Zeto_Anon.json
var zetoAnonAbi []byte

var (
	controllerName = "controller"
)

// This is the path to the contracts file
// it should be set by the test runner
var contractsFile string

type zetoDomainTestSuite struct {
	suite.Suite
	hdWalletSeed      *testbed.UTInitFunction
	deployedContracts *helpers.ZetoDomainContracts
	domainName        string
	domain            zeto.Zeto
	rpc               rpcclient.Client
	pldClient         pldclient.PaladinWSClient
	tb                testbed.Testbed
	receiptsSub       rpcclient.Subscription
	receiptsChan      chan zetoReceiptWithTXID
	done              func()
}

func (s *zetoDomainTestSuite) SetupSuite() {
	log.SetLevel("debug")
	s.hdWalletSeed = testbed.HDWalletSeedScopedToTest()
	domainContracts := helpers.DeployZetoContracts(s.T(), s.hdWalletSeed, contractsFile, controllerName)
	s.deployedContracts = domainContracts
	ctx := context.Background()
	domainName := "zeto_" + pldtypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)
	config := helpers.PrepareZetoConfig(s.T(), s.deployedContracts, "../zeto/zkp")
	waitForZeto, zetoTestbed := newZetoDomain(s.T(), config, domainContracts.FactoryAddress)
	done, _, tb, rpc, pldClient := newTestbed(s.T(), s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		domainName: zetoTestbed,
	})
	s.domainName = domainName
	s.domain = <-waitForZeto
	s.rpc = rpc
	s.pldClient = pldClient
	s.tb = tb
	s.done = done
}

func (s *zetoDomainTestSuite) TearDownSuite() {
	s.done()
}

type zetoReceiptWithTXID struct {
	types.ZetoDomainReceipt
	txID uuid.UUID
}

func (s *zetoDomainTestSuite) BeforeTest(suiteName, testName string) {
	ctx := s.T().Context()
	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Beginning test %s.%s", suiteName, testName)
	log.L(ctx).Info("*************************************")

	s.receiptsChan = make(chan zetoReceiptWithTXID)
	s.receiptsSub = subscribeAndSendZetoReceiptsToChannel(s.T(), s.pldClient, s.domainName, s.receiptsChan)
}

func (s *zetoDomainTestSuite) AfterTest(suiteName, testName string) {
	ctx := s.T().Context()
	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Completed test %s.%s", suiteName, testName)
	log.L(ctx).Info("*************************************")

	s.receiptsSub.Unsubscribe(s.T().Context())
	close(s.receiptsChan)
}

func subscribeAndSendZetoReceiptsToChannel(t *testing.T, wsClient pldclient.PaladinWSClient, domainName string, receipts chan zetoReceiptWithTXID) rpcclient.Subscription {
	ctx := t.Context()

	privateType := pldtypes.Enum[pldapi.TransactionType](pldapi.TransactionTypePrivate)
	listenerName := fmt.Sprintf("listener-%s-%s", domainName, pldtypes.RandHex(8))
	_, err := wsClient.PTX().CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: listenerName,
		Filters: pldapi.TransactionReceiptFilters{
			Type:   &privateType,
			Domain: domainName,
		},
		Options: pldapi.TransactionReceiptListenerOptions{
			DomainReceipts: true,
		},
	})
	require.NoError(t, err)

	sub, err := wsClient.PTX().SubscribeReceipts(ctx, listenerName)
	require.NoError(t, err)
	go func() {
		// No test assertions in this routine, if there's an error, no receipts are sent and the test will fail
		for {
			select {
			case subNotification, ok := <-sub.Notifications():
				if ok {
					zetoReceipts := make([]zetoReceiptWithTXID, 0)
					var batch pldapi.TransactionReceiptBatch
					_ = json.Unmarshal(subNotification.GetResult(), &batch)
					for _, r := range batch.Receipts {
						if r.DomainReceipt == nil {
							continue
						}
						var zetoReceipt types.ZetoDomainReceipt
						err = json.Unmarshal(r.DomainReceipt, &zetoReceipt)
						if err == nil {
							zetoReceipts = append(zetoReceipts, zetoReceiptWithTXID{
								ZetoDomainReceipt: zetoReceipt,
								txID:              r.ID,
							})
						}
					}
					_ = subNotification.Ack(ctx)
					// send after the ack otherwise the main test can complete when it receives the last values and the websocket is closed before the ack
					// can be sent
					for _, n := range zetoReceipts {
						receipts <- n
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return sub
}
