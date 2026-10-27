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
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/pkg/testbed"
	"github.com/LFDT-Paladin/paladin/domains/integration-test/helpers"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zeto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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

// zetoReceipts collects the domain receipts published by a receipt listener, so that a test can wait
// for the receipt of one specific transaction. Receipts for other transactions are held rather than
// discarded, so a test only has to name the transactions it makes assertions about, and cannot end up
// asserting against the receipt of some earlier transaction it chose not to check.
type zetoReceipts struct {
	sub      rpcclient.Subscription
	received chan zetoReceiptWithTXID
	pending  map[uuid.UUID]*types.ZetoDomainReceipt
}

func (r *zetoReceipts) waitFor(t *testing.T, txID uuid.UUID) *types.ZetoDomainReceipt {
	if receipt, ok := r.pending[txID]; ok {
		delete(r.pending, txID)
		return receipt
	}
	for {
		select {
		case received := <-r.received:
			if received.txID == txID {
				return &received.ZetoDomainReceipt
			}
			r.pending[received.txID] = &received.ZetoDomainReceipt
		case <-time.After(60 * time.Second):
			require.FailNowf(t, "No domain receipt", "no domain receipt received for transaction %s", txID)
			return nil
		}
	}
}

// invokeAndWait sends a private transaction and returns the domain receipt the listener published for
// it, so the assertions are always against the right transaction.
func (r *zetoReceipts) invokeAndWait(t *testing.T, tx *helpers.DomainTransactionHelper, signer string) *types.ZetoDomainReceipt {
	result := tx.SignAndSend(signer, true).Wait()
	txIDStr, ok := result["id"].(string)
	require.Truef(t, ok, "no transaction id in testbed_invoke result: %+v", result)
	txID, err := uuid.Parse(txIDStr)
	require.NoError(t, err)
	return r.waitFor(t, txID)
}

func (r *zetoReceipts) close(t *testing.T) {
	r.sub.Unsubscribe(t.Context())
}

// resolveZetoKey returns the Baby Jubjub public key a Zeto receipt identifies an owner by
func (s *zetoDomainTestSuite) resolveZetoKey(t *testing.T, ctx context.Context, name string) pldtypes.Bytes32 {
	var key pldtypes.Bytes32
	rpcerr := s.rpc.CallRPC(ctx, &key, "ptx_resolveVerifier", name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)
	return key
}

// requireTransferTo finds the transfer to a given party, so the assertion does not depend on the order
// the recipients happen to appear in the receipt
func requireTransferTo(t *testing.T, transfers []*types.ReceiptTransfer, from, to pldtypes.Bytes32, amount int64) {
	for _, transfer := range transfers {
		if transfer.To.String() == to.String() {
			assert.Equal(t, from.String(), transfer.From.String())
			require.NotNil(t, transfer.Amount)
			assert.Equal(t, amount, transfer.Amount.Int().Int64())
			return
		}
	}
	require.FailNowf(t, "Transfer not found", "no transfer to %s in %+v", to, transfers)
}

func (s *zetoDomainTestSuite) BeforeTest(suiteName, testName string) {
	ctx := s.T().Context()
	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Beginning test %s.%s", suiteName, testName)
	log.L(ctx).Info("*************************************")
}

func (s *zetoDomainTestSuite) AfterTest(suiteName, testName string) {
	ctx := s.T().Context()
	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Completed test %s.%s", suiteName, testName)
	log.L(ctx).Info("*************************************")
}

func subscribeToZetoReceipts(t *testing.T, wsClient pldclient.PaladinWSClient, domainName string) *zetoReceipts {
	ctx := t.Context()
	receipts := make(chan zetoReceiptWithTXID)

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
						} else {
							log.L(ctx).Errorf("Failed to unmarshal Zeto receipt in TX %s: %s", r.ID.String(), err.Error())
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
	return &zetoReceipts{
		sub:      sub,
		received: receipts,
		pending:  make(map[uuid.UUID]*types.ZetoDomainReceipt),
	}
}
