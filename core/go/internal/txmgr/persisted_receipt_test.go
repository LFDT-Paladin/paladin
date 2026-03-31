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
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
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
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectCommit()
		},
	)
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: txID, ReceiptType: components.RT_FailedOnChainWithRevertData,
				FailureMessage: "domain decoded error"},
		})
	})
	assert.NoError(t, err)

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

func TestFinalizeTransactionsRedactFailureOverSuccessInBatch(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			/* no failed query here confirms we redacted the failure */
			mc.db.ExpectQuery("INSERT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{
				"transaction", "success",
			}).AddRow(txID, false))
			mc.db.ExpectExec("DELETE.*transaction_receipts").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("INSERT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))
			// Mock the transaction_deps query used by dependency notification pre-commit (no dependents)
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: txID, ReceiptType: components.RT_FailedWithMessage,
				FailureMessage: "something went wrong before"},
			{TransactionID: txID, ReceiptType: components.RT_Success /* this wins */},
			{TransactionID: txID, ReceiptType: components.RT_FailedWithMessage,
				FailureMessage: "something went wrong after"},
		})
	})
	assert.NoError(t, err)

}

func TestFinalizeTransactionsDoNotOverrideSuccessWithFailure(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			/* no failed query here confirms we redacted the failure */
			mc.db.ExpectQuery("INSERT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{
				"transaction", "success",
			}).AddRow(txID, true /* do not override */))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))
			// Mock the transaction_deps query used by dependency notification pre-commit (no dependents)
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: txID, ReceiptType: components.RT_FailedWithMessage,
				FailureMessage: "something went wrong"},
		})
	})
	assert.NoError(t, err)

}

func TestFinalizeTransactionsRedactFailureOverSuccessPersistedDoesNotSkip(t *testing.T) {

	txID1 := uuid.New()
	txID2 := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(txID1))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))
			// Mock the transaction_deps query used by dependency notification pre-commit (no dependents)
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: txID1, ReceiptType: components.RT_FailedWithMessage,
				FailureMessage: "something went wrong"},
			{TransactionID: txID2, ReceiptType: components.RT_FailedWithMessage,
				FailureMessage: "this is skipped"},
		})
	})
	assert.NoError(t, err)

}

func TestFinalizeTransactionsOverwriteFailureWithSuccessRealDB(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true)
	defer done()

	txA := uuid.New()
	txB := uuid.New()
	txC := uuid.New()

	// Seed an existing failure receipt.
	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID:  txB,
				ReceiptType:    components.RT_FailedWithMessage,
				FailureMessage: "seed failure",
			},
		})
	})
	require.NoError(t, err)

	initialReceipt, err := txm.GetTransactionReceiptByID(ctx, txB)
	require.NoError(t, err)
	require.False(t, initialReceipt.Success)
	require.NotZero(t, initialReceipt.Sequence)

	// Trigger the duplicate handling path with a partial conflict pattern (A inserted, B conflicts, C inserted).
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: txA,
				ReceiptType:   components.RT_Success,
			},
			{
				TransactionID: txB, // conflicts with seeded failure and must be replaced with success
				ReceiptType:   components.RT_Success,
			},
			{
				TransactionID: txC,
				ReceiptType:   components.RT_Success,
			},
		})
	})
	require.NoError(t, err)

	updatedReceipt, err := txm.GetTransactionReceiptByID(ctx, txB)
	require.NoError(t, err)
	require.True(t, updatedReceipt.Success)
	require.Empty(t, updatedReceipt.FailureMessage)
	require.Greater(t, updatedReceipt.Sequence, initialReceipt.Sequence, "replacement must allocate a new sequence")

	receiptA, err := txm.GetTransactionReceiptByID(ctx, txA)
	require.NoError(t, err)
	require.True(t, receiptA.Success)

	receiptC, err := txm.GetTransactionReceiptByID(ctx, txC)
	require.NoError(t, err)
	require.True(t, receiptC.Success)
}

func TestFinalizeTransactionsChainedLookupFail(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(txID))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID:  txID,
				Domain:         "domain1",
				ReceiptType:    components.RT_FailedWithMessage,
				FailureMessage: "something went wrong",
			},
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

func TestFinalizeTransactionsInsertOkChained(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, true, mockDomainContractResolve(t, "domain1"), func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.sequencerMgr.On("HandleChainedTransactionOutcome", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

		mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, mock.Anything).Return(
			&pldapi.TransactionStates{None: true}, nil,
		)

		md := componentsmocks.NewDomain(t)
		mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
		md.On("BuildDomainReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("not available"))
	})
	defer done()

	var chainedTx *components.ValidatedTransaction
	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		contractAddressDomain1 := pldtypes.RandAddress()
		chainedTx, err = txm.resolveNewTransaction(ctx, dbTX, &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				From:           "me",
				IdempotencyKey: "parent_txn",
				Type:           pldapi.TransactionTypePrivate.Enum(),
				Domain:         "domain1",
				To:             contractAddressDomain1,
				Function:       "doThing1",
			},
			ABI: abi.ABI{{Type: abi.Function, Name: "doThing1"}},
		}, pldapi.SubmitModeAuto)
		require.NoError(t, err)
		chainedTx.Transaction.ABIReference = chainedTx.Function.ABIReference

		return txm.ChainPrivateTransactions(ctx, dbTX, []*components.ChainedPrivateTransaction{
			{
				OriginalSenderLocator:   "sender@remote.node",
				OriginalTransaction:     uuid.New(),
				OriginalDomain:          "domain1",
				OriginalContractAddress: pldtypes.RandAddress().String(),
				NewTransaction:          chainedTx,
			},
		})
	})
	require.NoError(t, err)

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: *chainedTx.Transaction.ID,
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

	receipt, err := txm.GetTransactionReceiptByIDFull(ctx, *chainedTx.Transaction.ID)
	require.NoError(t, err)

	require.NotNil(t, receipt)
	require.JSONEq(t, fmt.Sprintf(`{
		"id":"%s",
		"sequence":%d,
		"domain": "domain1",
		"blockNumber":12345, 
		"logIndex":5,
		"public":null,
	 	"source":"0x3f9f796ff55589dd2358c458f185bbed357c0b6e",
	  	"success":true, 
	  	"transactionHash":"0xd0561b310b77e47bc16fb3c40d48b72255b1748efeecf7452373dfce8045af30", 
		"transactionIndex":10,
		"states": {"none": true},
		"domainReceiptError": "not available"
	}`, chainedTx.Transaction.ID, receipt.Sequence), pldtypes.JSONString(receipt).Pretty())

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

func TestGetFullReceiptSharedReadInfoStates(t *testing.T) {

	txID1 := uuid.New()
	txID2 := uuid.New()
	sharedReadState := &pldapi.StateBase{ID: pldtypes.HexBytes(pldtypes.RandBytes(32))}
	sharedInfoState := &pldapi.StateBase{ID: pldtypes.HexBytes(pldtypes.RandBytes(32))}

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			// Expectations are consumed in order by sqlmock. Each GetTransactionReceiptByIDFull
			// call produces: receipt query -> dispatches -> chained -> public txns.

			// --- receipt 1 ---
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "sequence", "indexed", "domain", "success"}).
					AddRow(txID1, 1, "2024-01-01T00:00:00Z", "domain1", true),
			)
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))

			// --- receipt 2 ---
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "sequence", "indexed", "domain", "success"}).
					AddRow(txID2, 2, "2024-01-01T00:00:00Z", "domain1", true),
			)
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))

			// Both transactions return the same read and info states
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID1).Return(
				&pldapi.TransactionStates{
					Read: []*pldapi.StateBase{sharedReadState},
					Info: []*pldapi.StateBase{sharedInfoState},
				}, nil,
			)
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID2).Return(
				&pldapi.TransactionStates{
					Read: []*pldapi.StateBase{sharedReadState},
					Info: []*pldapi.StateBase{sharedInfoState},
				}, nil,
			)

			md := componentsmocks.NewDomain(t)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
			md.On("BuildDomainReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

			mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(make(map[uuid.UUID][]*pldapi.PublicTx), nil)
		})
	defer done()

	receipt1, err := txm.GetTransactionReceiptByIDFull(ctx, txID1)
	require.NoError(t, err)
	require.NotNil(t, receipt1)
	require.NotNil(t, receipt1.States)
	require.Len(t, receipt1.States.Read, 1)
	assert.Equal(t, sharedReadState.ID, receipt1.States.Read[0].ID)
	require.Len(t, receipt1.States.Info, 1)
	assert.Equal(t, sharedInfoState.ID, receipt1.States.Info[0].ID)

	receipt2, err := txm.GetTransactionReceiptByIDFull(ctx, txID2)
	require.NoError(t, err)
	require.NotNil(t, receipt2)
	require.NotNil(t, receipt2.States)
	require.Len(t, receipt2.States.Read, 1)
	assert.Equal(t, sharedReadState.ID, receipt2.States.Read[0].ID)
	require.Len(t, receipt2.States.Info, 1)
	assert.Equal(t, sharedInfoState.ID, receipt2.States.Info[0].ID)
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

func TestBuildFullReceiptFailAddStateReceipt(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			// Mock GetTransactionStates to fail
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
				Return(nil, fmt.Errorf("state retrieval failed"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	// Create a receipt with a domain
	receipt := &pldapi.TransactionReceipt{
		ID: txID,
		TransactionReceiptData: pldapi.TransactionReceiptData{
			Domain:          "domain1",
			Sequence:        1000,
			Success:         true,
			ContractAddress: pldtypes.RandAddress(),
		},
	}

	// This should fail when trying to add state receipt
	_, err = txm.buildFullReceipt(ctx, receipt, false)
	assert.Regexp(t, "state retrieval failed", err)
	close(l.done)
}

func TestBuildFullReceiptFailDomainFindFail(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))

			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).Return(&pldapi.TransactionStates{}, nil)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(nil, fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	// Create a receipt with a domain
	receipt := &pldapi.TransactionReceipt{
		ID: txID,
		TransactionReceiptData: pldapi.TransactionReceiptData{
			Domain:          "domain1",
			Sequence:        1000,
			Success:         true,
			ContractAddress: pldtypes.RandAddress(),
		},
	}

	// This should fail when trying to add state receipt
	fr, err := txm.buildFullReceipt(ctx, receipt, true)
	assert.NoError(t, err)
	require.Regexp(t, "pop", fr.DomainReceiptError)
	close(l.done)
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
			// Mock the transaction_deps query used by dependency notification pre-commit (no dependents)
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			// HandleChainedTransactionOutcome is called post-commit for A's coordinator notification
			mc.sequencerMgr.On("HandleChainedTransactionOutcome", mock.Anything, mock.MatchedBy(func(addr pldtypes.EthAddress) bool {
				return addr == *pldtypes.MustEthAddress(contractAddress)
			}), originalTxID, components.RT_Success, mock.Anything, mock.Anything, mock.Anything).Return()
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
	mc.AssertNotCalled(t, "WriteOrDistributeChainedTransactionReceipts", mock.Anything, mock.Anything, mock.Anything)
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
			// Mock the transaction_deps query used by dependency notification pre-commit (no dependents)
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			// WriteOrDistributeChainedTransactionReceipts should NOT be called since receiptsToWrite is empty
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
			// Mock the transaction_deps query used by dependency notification pre-commit (no dependents)
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))

			// WriteOrDistributeChainedTransactionReceipts should NOT be called
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

func TestFinalizeTransactionsChainedOnChainRevertNotifiesCoordinator(t *testing.T) {
	chainedTxID := uuid.New()
	originalTxID := uuid.New()
	contractAddress := "0x1234567890123456789012345678901234567890"
	revertData := pldtypes.HexBytes{0xde, 0xad}

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			mc.db.ExpectQuery(`SELECT.*chained_private_txns.*WHERE.*chained_transaction.*(IN|ANY)`).
				WithArgs(sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address"}).
					AddRow(chainedTxID, originalTxID, "sender1", "domain1", contractAddress))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			// On-chain revert: coordinator is notified but receipt is NOT propagated
			mc.sequencerMgr.On("HandleChainedTransactionOutcome", mock.Anything, mock.MatchedBy(func(addr pldtypes.EthAddress) bool {
				return addr == *pldtypes.MustEthAddress(contractAddress)
			}), originalTxID, components.RT_FailedOnChainWithRevertData, mock.Anything, mock.MatchedBy(func(rd pldtypes.HexBytes) bool {
				return len(rd) == 2 && rd[0] == 0xde
			}), mock.Anything).Return()
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID: chainedTxID,
				Domain:        "chainedDomain",
				ReceiptType:   components.RT_FailedOnChainWithRevertData,
				RevertData:    revertData,
				OnChain: pldtypes.OnChainLocation{
					Type:            pldtypes.OnChainTransaction,
					TransactionHash: pldtypes.RandBytes32(),
					BlockNumber:     500,
				},
			},
		})
	})
	require.NoError(t, err)
	mc := txm.sequencerMgr.(*componentsmocks.SequencerManager)
	mc.AssertExpectations(t)
}

func TestFinalizeTransactionsChainedOffChainRevertNotifiesCoordinator(t *testing.T) {
	chainedTxID := uuid.New()
	originalTxID := uuid.New()
	contractAddress := "0x1234567890123456789012345678901234567890"

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			mc.db.ExpectQuery(`SELECT.*chained_private_txns.*WHERE.*chained_transaction.*(IN|ANY)`).
				WithArgs(sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address"}).
					AddRow(chainedTxID, originalTxID, "sender1", "domain1", contractAddress))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			// Off-chain revert: coordinator notified
			mc.sequencerMgr.On("HandleChainedTransactionOutcome", mock.Anything, mock.MatchedBy(func(addr pldtypes.EthAddress) bool {
				return addr == *pldtypes.MustEthAddress(contractAddress)
			}), originalTxID, components.RT_FailedWithMessage, mock.Anything, mock.Anything, mock.Anything).Return()
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				TransactionID:  chainedTxID,
				Domain:         "chainedDomain",
				ReceiptType:    components.RT_FailedWithMessage,
				FailureMessage: "assembly failed",
			},
		})
	})
	require.NoError(t, err)
	mc := txm.sequencerMgr.(*componentsmocks.SequencerManager)
	mc.AssertExpectations(t)
}
