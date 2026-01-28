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

package txmgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFinalizeTransactionsNoOp(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.FinalizeTransactions(ctx, txm.p.NOTX(), nil)
	assert.NoError(t, err)

}

func TestFinalizeTransactionsSuccessWithFailure(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.FinalizeTransactions(ctx, txm.p.NOTX(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_Success,
			FailureMessage: "not empty",
		},
	})
	assert.Regexp(t, "PD012213", err)
}

func TestFinalizeTransactionsBadType(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.FinalizeTransactions(ctx, txm.p.NOTX(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.ReceiptType(42)}})
	assert.Regexp(t, "PD012213", err)

}

func TestFinalizeTransactionsFailedWithMessageNoMessage(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.FinalizeTransactions(ctx, txm.p.NOTX(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_FailedWithMessage}})
	assert.Regexp(t, "PD012213", err)

}

func TestFinalizeTransactionsFailedWithRevertDataWithMessage(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.FinalizeTransactions(ctx, txm.p.NOTX(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_FailedOnChainWithRevertData,
			FailureMessage: "not empty"}})
	assert.Regexp(t, "PD012213", err)

}

func TestFinalizeTransactionsInsertFail(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: txID, ReceiptType: components.RT_FailedWithMessage,
				FailureMessage: "something went wrong"},
		})
	})
	assert.Regexp(t, "pop", err)

}

func mockKeyResolver(t *testing.T, mc *mockComponents) *componentsmocks.KeyResolver {
	kr := componentsmocks.NewKeyResolver(t)
	mc.keyManager.On("KeyResolverForDBTX", mock.Anything).Return(kr)
	return kr
}

func mockDomainContractResolve(t *testing.T, domainName string, contractAddrs ...pldtypes.EthAddress) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mgsc := mc.domainManager.On("GetSmartContractByAddress", mock.Anything, mock.Anything, mock.MatchedBy(func(a pldtypes.EthAddress) bool {
			if len(contractAddrs) == 0 {
				return true
			}
			for _, contractAddr := range contractAddrs {
				if contractAddr == a {
					return true
				}
			}
			return false
		}))
		mgsc.Run(func(args mock.Arguments) {
			mpsc := componentsmocks.NewDomainSmartContract(t)
			mdmn := componentsmocks.NewDomain(t)
			mdmn.On("Name").Return(domainName)
			mpsc.On("Domain").Return(mdmn)
			mpsc.On("Address").Return(args[2].(pldtypes.EthAddress)).Maybe()
			mgsc.Return(mpsc, nil)
		})
	}
}

func TestFinalizeTransactionsInsertOkOffChain(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, true, mockDomainContractResolve(t, "domain1"), func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.sequencerMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	txID, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     "me",
			Type:     pldapi.TransactionTypePrivate.Enum(),
			Function: "doIt",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	require.NoError(t, err)

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: *txID,
				ReceiptType:   components.RT_FailedOnChainWithRevertData,
			},
		})
	})
	require.NoError(t, err)

	receipt, err := txm.GetTransactionReceiptByID(ctx, *txID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.JSONEq(t, fmt.Sprintf(`{
		"id":"%s",
		"sequence":%d,
		"failureMessage":"PD012214: Unable to decode revert data (no revert data available)"
	}`, txID, receipt.Sequence), string(pldtypes.JSONString(receipt)))

}

func TestFinalizeTransactionsInsertOkEvent(t *testing.T) {

	var txID *uuid.UUID
	var err error
	ctx, txm, done := newTestTransactionManager(t, true, mockDomainContractResolve(t, "domain1"), mockQueryPublicTxForTransactions(func(ids []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error) {
		pubTX := map[uuid.UUID][]*pldapi.PublicTx{
			*txID: {},
		}
		return pubTX, nil
	}), func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.sequencerMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, mock.Anything).Return(
			&pldapi.TransactionStates{None: true}, nil,
		)

		md := componentsmocks.NewDomain(t)
		mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
		md.On("BuildDomainReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("not available"))
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	txID, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     "me",
			Type:     pldapi.TransactionTypePrivate.Enum(),
			Domain:   "domain1",
			Function: "doIt",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: *txID,
				Domain:        "domain1",
				ReceiptType:   components.RT_Success,
				OnChain: pldtypes.OnChainLocation{
					Type:             pldtypes.OnChainEvent,
					TransactionHash:  pldtypes.MustParseBytes32("d0561b310b77e47bc16fb3c40d48b72255b1748efeecf7452373dfce8045af30"),
					BlockNumber:      12345,
					TransactionIndex: 10,
					LogIndex:         5,
					Source:           pldtypes.MustEthAddress("0x3f9f796ff55589dd2358c458f185bbed357c0b6e"),
				},
			},
		})
	})
	require.NoError(t, err)

	receipt, err := txm.GetTransactionReceiptByIDFull(ctx, *txID)
	require.NoError(t, err)

	require.NotNil(t, receipt)
	require.JSONEq(t, fmt.Sprintf(`{
		"id":"%s",
		"sequence":%d,
		"domain": "domain1",
		"blockNumber":12345, 
		"logIndex":5,
	 	"source":"0x3f9f796ff55589dd2358c458f185bbed357c0b6e",
	  	"success":true, 
	  	"transactionHash":"0xd0561b310b77e47bc16fb3c40d48b72255b1748efeecf7452373dfce8045af30", 
		"transactionIndex":10,
		"states": {"none": true},
		"domainReceiptError": "not available",
		"public": []
	}`, txID, receipt.Sequence), pldtypes.JSONString(receipt).Pretty())

}

func TestCalculateRevertErrorNoData(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.CalculateRevertError(ctx, nil, nil)
	assert.Regexp(t, "PD012214", err)

}

func TestCalculateRevertErrorQueryFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abi_entries").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	err := txm.CalculateRevertError(ctx, txm.p.NOTX(), []byte("any data"))
	assert.Regexp(t, "PD012221.*pop", err)

}

func TestCalculateRevertErrorDecodeFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abi_entries").WillReturnRows(sqlmock.NewRows([]string{"definition"}).AddRow(`{}`))
		})
	defer done()

	err := txm.CalculateRevertError(ctx, txm.p.NOTX(), []byte("any data"))
	assert.Regexp(t, "PD012221", err)

}

func TestGetTransactionReceiptNoResult(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	res, err := txm.GetTransactionReceiptByID(ctx, uuid.New())
	assert.NoError(t, err)
	assert.Nil(t, res)

}

func TestGetTransactionReceiptFullNoResult(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	res, err := txm.GetTransactionReceiptByIDFull(ctx, uuid.New())
	assert.NoError(t, err)
	assert.Nil(t, res)

}

func TestGetTransactionReceiptFullMergeDispatchesError(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			// Mock successful transaction_receipts query
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "sequence", "indexed", "domain", "success", "tx_hash", "block_number", "tx_index", "log_index", "source", "failure_message", "revert_data", "contract_address"}).
					AddRow(txID, 1, "2024-01-01T00:00:00Z", "domain1", true, nil, nil, nil, nil, nil, nil, nil, nil),
			)

			// Mock GetTransactionStates
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).Return(
				&pldapi.TransactionStates{None: true}, nil,
			)

			// Mock GetDomainByName and BuildDomainReceipt
			md := componentsmocks.NewDomain(t)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
			md.On("BuildDomainReceipt", mock.Anything, mock.Anything, txID, mock.Anything).Return(nil, nil)

			// Mock dispatches query to return an error - this will trigger line 444
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnError(fmt.Errorf("database error"))
		})
	defer done()

	res, err := txm.GetTransactionReceiptByIDFull(ctx, txID)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Regexp(t, "database error", err.Error())

}

func TestGetTransactionReceiptFullMergeChainedTransactionsError(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			// Mock successful transaction_receipts query
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "sequence", "indexed", "domain", "success", "tx_hash", "block_number", "tx_index", "log_index", "source", "failure_message", "revert_data", "contract_address"}).
					AddRow(txID, 1, "2024-01-01T00:00:00Z", "domain1", true, nil, nil, nil, nil, nil, nil, nil, nil),
			)

			// Mock GetTransactionStates
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).Return(
				&pldapi.TransactionStates{None: true}, nil,
			)

			// Mock GetDomainByName and BuildDomainReceipt
			md := componentsmocks.NewDomain(t)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
			md.On("BuildDomainReceipt", mock.Anything, mock.Anything, txID, mock.Anything).Return(nil, nil)

			// Mock successful dispatches query
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(
				sqlmock.NewRows([]string{"id", "private_transaction_id", "public_transaction_address", "public_transaction_id"}),
			)

			// Mock chained_private_txns query to return an error - this will trigger line 424
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnError(fmt.Errorf("chained transactions query error"))
		})
	defer done()

	res, err := txm.GetTransactionReceiptByIDFull(ctx, txID)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Regexp(t, "chained transactions query error", err.Error())

}

func TestGetTransactionReceiptFullMergePublicTransactionsError(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			// Mock successful transaction_receipts query
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "sequence", "indexed", "domain", "success", "tx_hash", "block_number", "tx_index", "log_index", "source", "failure_message", "revert_data", "contract_address"}).
					AddRow(txID, 1, "2024-01-01T00:00:00Z", "domain1", true, nil, nil, nil, nil, nil, nil, nil, nil),
			)

			// Mock GetTransactionStates
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).Return(
				&pldapi.TransactionStates{None: true}, nil,
			)

			// Mock GetDomainByName and BuildDomainReceipt
			md := componentsmocks.NewDomain(t)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
			md.On("BuildDomainReceipt", mock.Anything, mock.Anything, txID, mock.Anything).Return(nil, nil)

			// Mock successful dispatches query
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(
				sqlmock.NewRows([]string{"id", "private_transaction_id", "public_transaction_address", "public_transaction_id"}),
			)

			// Mock chained_private_txns query - called by mergeChainedTranasctions before mergeReceiptPublicTransactions
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(
				sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address"}),
			)

			// Mock QueryPublicTxForTransactions to return an error - this will trigger line 428
			mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil, fmt.Errorf("public tx query error"))
		})
	defer done()

	res, err := txm.GetTransactionReceiptByIDFull(ctx, txID)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Regexp(t, "public tx query error", err.Error())

}

func TestMergeDispatchesGroupsByPrivateTransactionID(t *testing.T) {
	// This test specifically exercises line 449 where dispatches are grouped by PrivateTransactionID
	txID := uuid.New()
	txIDStr := txID.String()
	contractAddr1 := pldtypes.MustEthAddress("0x1111111111111111111111111111111111111111")
	contractAddr2 := pldtypes.MustEthAddress("0x2222222222222222222222222222222222222222")

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			// Mock successful transaction_receipts query
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "sequence", "indexed", "domain", "success", "tx_hash", "block_number", "tx_index", "log_index", "source", "failure_message", "revert_data", "contract_address"}).
					AddRow(txID, 1, "2024-01-01T00:00:00Z", "domain1", true, nil, nil, nil, nil, nil, nil, nil, nil),
			)

			// Mock GetTransactionStates
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).Return(
				&pldapi.TransactionStates{None: true}, nil,
			)

			// Mock GetDomainByName and BuildDomainReceipt
			md := componentsmocks.NewDomain(t)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
			md.On("BuildDomainReceipt", mock.Anything, mock.Anything, txID, mock.Anything).Return(nil, nil)

			// Mock dispatches query to return multiple dispatches with the same PrivateTransactionID
			// This tests the grouping logic at line 449: txdpMap[txdp.PrivateTransactionID] = append(...)
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(
				sqlmock.NewRows([]string{"id", "private_transaction_id", "public_transaction_address", "public_transaction_id"}).
					AddRow("dispatch-1", txIDStr, contractAddr1.String(), uint64(100)).
					AddRow("dispatch-2", txIDStr, contractAddr2.String(), uint64(200)).
					AddRow("dispatch-3", txIDStr, contractAddr1.String(), uint64(300)),
			)

			// Mock chained_private_txns query - called by mergeChainedTranasctions after mergeDispatches
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(
				sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address"}),
			)

			// Mock QueryPublicTxForTransactions - called by mergeReceiptPublicTransactions after mergeDispatches
			mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(make(map[uuid.UUID][]*pldapi.PublicTx), nil)
		})
	defer done()

	res, err := txm.GetTransactionReceiptByIDFull(ctx, txID)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify that all three dispatches were grouped together and assigned to the transaction
	assert.Len(t, res.Dispatches, 3, "All dispatches with the same PrivateTransactionID should be grouped together")

	// Verify the dispatches are correctly mapped
	dispatchIDs := make(map[string]bool)
	for _, dispatch := range res.Dispatches {
		assert.Equal(t, txIDStr, dispatch.PrivateTransactionID, "All dispatches should have the same PrivateTransactionID")
		dispatchIDs[dispatch.ID] = true
	}
	assert.True(t, dispatchIDs["dispatch-1"], "dispatch-1 should be present")
	assert.True(t, dispatchIDs["dispatch-2"], "dispatch-2 should be present")
	assert.True(t, dispatchIDs["dispatch-3"], "dispatch-3 should be present")
}

func TestMergeChainedTranasctionsGroupsByTransactionID(t *testing.T) {
	// This test specifically exercises line 480 where chained transactions are grouped by Transaction ID
	txID := uuid.New()
	txIDStr := txID.String()
	chainedTxID1 := uuid.New()
	chainedTxID2 := uuid.New()
	chainedTxID3 := uuid.New()
	localID1 := uuid.New()
	localID2 := uuid.New()
	localID3 := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			// Mock successful transaction_receipts query
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "sequence", "indexed", "domain", "success", "tx_hash", "block_number", "tx_index", "log_index", "source", "failure_message", "revert_data", "contract_address"}).
					AddRow(txID, 1, "2024-01-01T00:00:00Z", "domain1", true, nil, nil, nil, nil, nil, nil, nil, nil),
			)

			// Mock GetTransactionStates
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).Return(
				&pldapi.TransactionStates{None: true}, nil,
			)

			// Mock GetDomainByName and BuildDomainReceipt
			md := componentsmocks.NewDomain(t)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
			md.On("BuildDomainReceipt", mock.Anything, mock.Anything, txID, mock.Anything).Return(nil, nil)

			// Mock successful dispatches query
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(
				sqlmock.NewRows([]string{"id", "private_transaction_id", "public_transaction_address", "public_transaction_id"}),
			)

			// Mock chained_private_txns query to return multiple chained transactions with the same Transaction ID
			// This tests the grouping logic at line 480: txdpMap[string(txdp.Transaction.String())] = append(...)
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(
				sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address", "id"}).
					AddRow(chainedTxID1, txID, "sender1", "domain1", "0x1111", localID1).
					AddRow(chainedTxID2, txID, "sender2", "domain2", "0x2222", localID2).
					AddRow(chainedTxID3, txID, "sender3", "domain3", "0x3333", localID3),
			)

			// Mock QueryPublicTxForTransactions - called by mergeReceiptPublicTransactions after mergeChainedTranasctions
			mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(make(map[uuid.UUID][]*pldapi.PublicTx), nil)
		})
	defer done()

	res, err := txm.GetTransactionReceiptByIDFull(ctx, txID)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify that all three chained transactions were grouped together and assigned to the transaction
	assert.Len(t, res.ChainedPrivateTransactions, 3, "All chained transactions with the same Transaction ID should be grouped together")

	// Verify the chained transactions are correctly mapped
	chainedTxIDs := make(map[string]bool)
	for _, chainedTx := range res.ChainedPrivateTransactions {
		assert.Equal(t, txIDStr, chainedTx.TransactionID, "All chained transactions should have the same Transaction ID")
		chainedTxIDs[chainedTx.ChainedTransactionID] = true
	}
	assert.True(t, chainedTxIDs[chainedTxID1.String()], "chainedTxID1 should be present")
	assert.True(t, chainedTxIDs[chainedTxID2.String()], "chainedTxID2 should be present")
	assert.True(t, chainedTxIDs[chainedTxID3.String()], "chainedTxID3 should be present")

	// Verify LocalIDs are correctly mapped
	localIDs := make(map[string]bool)
	for _, chainedTx := range res.ChainedPrivateTransactions {
		localIDs[chainedTx.LocalID] = true
	}
	assert.True(t, localIDs[localID1.String()], "localID1 should be present")
	assert.True(t, localIDs[localID2.String()], "localID2 should be present")
	assert.True(t, localIDs[localID3.String()], "localID3 should be present")
}

func TestGetDomainReceiptFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(nil, fmt.Errorf("not found"))
		})
	defer done()

	_, err := txm.GetDomainReceiptByID(ctx, "domain1", uuid.New())
	assert.Regexp(t, "not found", err)

}

func TestDecodeRevertErrorBadSerializer(t *testing.T) {
	revertReasonTooSmallHex := pldtypes.MustParseHexBytes("0x08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000001d5468652073746f7265642076616c756520697320746f6f20736d616c6c000000")

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abi_entries").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	_, err := txm.DecodeRevertError(ctx, txm.p.NOTX(), revertReasonTooSmallHex, "wrong")
	assert.Regexp(t, "PD020015", err)

}

func TestDecodeCall(t *testing.T) {

	sampleABI := abi.ABI{
		{Type: abi.Function, Name: "set", Inputs: abi.ParameterArray{
			{Type: "uint256", Name: "newValue"},
		}},
	}

	ctx, txm, done := newTestTransactionManager(t, true)
	defer done()

	_, err := txm.storeABINewDBTX(ctx, sampleABI)
	require.NoError(t, err)

	validCall, err := sampleABI.Functions()["set"].EncodeCallDataJSON([]byte(`[12345]`))
	require.NoError(t, err)

	decoded, err := txm.DecodeCall(ctx, txm.p.NOTX(), validCall, "")
	assert.NoError(t, err)
	require.JSONEq(t, `{"newValue": "12345"}`, string(decoded.Data))
	require.Equal(t, `set(uint256)`, string(decoded.Signature))

	invalidCall := append(sampleABI.Functions()["set"].FunctionSelectorBytes(), []byte{0x00}...)
	_, err = txm.DecodeCall(ctx, txm.p.NOTX(), pldtypes.HexBytes(invalidCall), "")
	assert.Regexp(t, "PD012227.*1 matched function selector", err)

	short := []byte{0xfe, 0xed}
	_, err = txm.DecodeCall(ctx, txm.p.NOTX(), pldtypes.HexBytes(short), "")
	assert.Regexp(t, "PD012226", err)

	_, err = txm.DecodeCall(ctx, txm.p.NOTX(), validCall, "wrong")
	assert.Regexp(t, "PD020015", err)

}

func TestDecodeEvent(t *testing.T) {

	sampleABI := abi.ABI{
		{Type: abi.Event, Name: "Updated", Inputs: abi.ParameterArray{
			{Type: "uint256", Name: "newValue", Indexed: true},
		}},
	}

	ctx, txm, done := newTestTransactionManager(t, true)
	defer done()

	_, err := txm.storeABINewDBTX(ctx, sampleABI)
	require.NoError(t, err)

	validTopic0 := pldtypes.Bytes32(sampleABI.Events()["Updated"].SignatureHashBytes())
	validTopic1, err := (&abi.ParameterArray{{Type: "uint256"}}).EncodeABIDataJSON([]byte(`["12345"]`))
	require.NoError(t, err)

	decoded, err := txm.DecodeEvent(ctx, txm.p.NOTX(), []pldtypes.Bytes32{validTopic0, pldtypes.Bytes32(validTopic1)}, []byte{}, "")
	assert.NoError(t, err)
	require.JSONEq(t, `{"newValue": "12345"}`, string(decoded.Data))
	require.Equal(t, `Updated(uint256)`, string(decoded.Signature))

	_, err = txm.DecodeEvent(ctx, txm.p.NOTX(), []pldtypes.Bytes32{validTopic0 /* missing 2nd topic*/}, []byte{}, "")
	assert.Regexp(t, "PD012229.*1 matched signature", err)

	_, err = txm.DecodeEvent(ctx, txm.p.NOTX(), []pldtypes.Bytes32{pldtypes.RandBytes32() /* unknown topic */}, []byte{}, "")
	assert.Regexp(t, "PD012229", err)

	_, err = txm.DecodeEvent(ctx, txm.p.NOTX(), []pldtypes.Bytes32{ /* no topics */ }, []byte{}, "")
	assert.Regexp(t, "PD012226", err)

	_, err = txm.DecodeEvent(ctx, txm.p.NOTX(), []pldtypes.Bytes32{validTopic0, pldtypes.Bytes32(validTopic1)}, []byte{}, "wrong")
	assert.Regexp(t, "PD020015", err)

}

func TestFinalizeTransactionsChainedReceiptPropagationSuccess(t *testing.T) {
	chainedTxID := uuid.New()
	originalTxID := uuid.New()
	originalSender := "sender1"
	originalDomain := "domain1"
	contractAddress := "0x1234567890123456789012345678901234567890"

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			// Mock the receipt insert (with RETURNING clause, so it's a Query)
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			// Mock the chaining records query - return a matching chaining record
			mc.db.ExpectQuery(`SELECT.*chained_private_txns.*WHERE.*chained_transaction.*(IN|ANY)`).
				WithArgs(sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address"}).
					AddRow(chainedTxID, originalTxID, originalSender, originalDomain, contractAddress))
			// Mock WriteOrDistributeReceiptsPostSubmit
			mc.sequencerMgr.On("WriteOrDistributeReceiptsPostSubmit", mock.Anything, mock.Anything, mock.MatchedBy(func(receipts []*components.ReceiptInputWithOriginator) bool {
				if len(receipts) != 1 {
					return false
				}
				r := receipts[0]
				// Check that the receipt was properly transformed: TransactionID changed to original, Domain set, and originator info added
				return r.TransactionID == originalTxID &&
					r.Domain == originalDomain &&
					r.Originator == originalSender &&
					r.DomainContractAddress == contractAddress
			})).Return(nil)
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: chainedTxID,
				Domain:        "chainedDomain",
				ReceiptType:   components.RT_Success,
			},
		})
	})
	require.NoError(t, err)
	mc := txm.sequencerMgr.(*componentsmocks.SequencerManager)
	mc.AssertExpectations(t)
}

func TestFinalizeTransactionsChainedReceiptPropagationNoMatch(t *testing.T) {
	chainedTxID := uuid.New()
	nonMatchingTxID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			// Mock the receipt insert (with RETURNING clause, so it's a Query)
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			// Mock the chaining records query - return a chaining record that doesn't match any receipt
			mc.db.ExpectQuery(`SELECT.*chained_private_txns.*WHERE.*chained_transaction.*(IN|ANY)`).
				WithArgs(sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address"}).
					AddRow(nonMatchingTxID, uuid.New(), "sender1", "domain1", "0x1234"))
			// WriteOrDistributeReceiptsPostSubmit should NOT be called since receiptsToWrite is empty
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: chainedTxID,
				Domain:        "chainedDomain",
				ReceiptType:   components.RT_Success,
			},
		})
	})
	require.NoError(t, err)
}

func TestFinalizeTransactionsChainedReceiptPropagationMultipleMatches(t *testing.T) {
	chainedTxID1 := uuid.New()
	chainedTxID2 := uuid.New()
	originalTxID1 := uuid.New()
	originalTxID2 := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			// Mock the receipt insert (with RETURNING clause, so it's a Query)
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1).AddRow(2))
			// Mock the chaining records query - return multiple chaining records
			// GORM uses ANY for multiple values in PostgreSQL
			mc.db.ExpectQuery(`SELECT.*chained_private_txns.*WHERE.*chained_transaction.*(IN|ANY)`).
				WithArgs(sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address"}).
					AddRow(chainedTxID1, originalTxID1, "sender1", "domain1", "0x1111").
					AddRow(chainedTxID2, originalTxID2, "sender2", "domain2", "0x2222"))
			// Mock WriteOrDistributeReceiptsPostSubmit with 2 receipts
			mc.sequencerMgr.On("WriteOrDistributeReceiptsPostSubmit", mock.Anything, mock.Anything, mock.MatchedBy(func(receipts []*components.ReceiptInputWithOriginator) bool {
				if len(receipts) != 2 {
					return false
				}
				// Check both receipts are present - match by original transaction IDs
				found1, found2 := false, false
				for _, r := range receipts {
					if r.TransactionID == originalTxID1 {
						found1 = true
					}
					if r.TransactionID == originalTxID2 {
						found2 = true
					}
				}
				return found1 && found2
			})).Return(nil)
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: chainedTxID1,
				Domain:        "chainedDomain1",
				ReceiptType:   components.RT_Success,
			},
			{
				TransactionID:  chainedTxID2,
				Domain:         "chainedDomain2",
				ReceiptType:    components.RT_FailedWithMessage,
				FailureMessage: "test failure",
			},
		})
	})
	require.NoError(t, err)
	mc := txm.sequencerMgr.(*componentsmocks.SequencerManager)
	mc.AssertExpectations(t)
}

func TestFinalizeTransactionsChainedReceiptPropagationQueryError(t *testing.T) {
	chainedTxID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			// Mock the receipt insert (with RETURNING clause, so it's a Query)
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			// Mock the chaining records query to return an error
			mc.db.ExpectQuery(`SELECT.*chained_private_txns.*WHERE.*chained_transaction.*(IN|ANY)`).
				WithArgs(sqlmock.AnyArg()).
				WillReturnError(fmt.Errorf("database query error"))
			mc.db.ExpectRollback()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: chainedTxID,
				Domain:        "chainedDomain",
				ReceiptType:   components.RT_Success,
			},
		})
	})
	require.Error(t, err)
	assert.Regexp(t, "database query error", err.Error())
}

func TestFinalizeTransactionsChainedReceiptPropagationWriteError(t *testing.T) {
	chainedTxID := uuid.New()
	originalTxID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			// Mock the receipt insert (with RETURNING clause, so it's a Query)
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			// Mock the chaining records query - return a matching chaining record
			mc.db.ExpectQuery(`SELECT.*chained_private_txns.*WHERE.*chained_transaction.*(IN|ANY)`).
				WithArgs(sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address"}).
					AddRow(chainedTxID, originalTxID, "sender1", "domain1", "0x1234"))
			// Mock WriteOrDistributeReceiptsPostSubmit to return an error
			mc.sequencerMgr.On("WriteOrDistributeReceiptsPostSubmit", mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("sequencer write error"))
			mc.db.ExpectRollback()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: chainedTxID,
				Domain:        "chainedDomain",
				ReceiptType:   components.RT_Success,
			},
		})
	})
	require.Error(t, err)
	assert.Regexp(t, "sequencer write error", err.Error())
}

func TestFinalizeTransactionsChainedReceiptPropagationNoChainingRecords(t *testing.T) {
	chainedTxID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			// Mock the receipt insert (with RETURNING clause, so it's a Query)
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			// Mock the chaining records query - return no records
			mc.db.ExpectQuery(`SELECT.*chained_private_txns.*WHERE.*chained_transaction.*(IN|ANY)`).
				WithArgs(sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address"}))
			// WriteOrDistributeReceiptsPostSubmit should NOT be called
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: chainedTxID,
				Domain:        "chainedDomain",
				ReceiptType:   components.RT_Success,
			},
		})
	})
	require.NoError(t, err)
}
