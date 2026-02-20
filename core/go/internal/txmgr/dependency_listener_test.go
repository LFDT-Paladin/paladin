/*
 * Copyright © 2026 Kaleido, Inc.
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
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBlockedByDependencies_NoDependencies_ReturnsFalse(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners)
	defer done()

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{ID: ptr(uuid.New())},
			DependsOn:   nil,
		},
	}

	blocked, err := txm.BlockedByDependencies(ctx, tx)
	require.NoError(t, err)
	assert.False(t, blocked)
}

func TestBlockedByDependencies_DependencyWithNoReceipt_ReturnsTrue(t *testing.T) {
	txID := uuid.New()
	depID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{ID: &txID},
			DependsOn:   []uuid.UUID{depID},
		},
	}

	blocked, err := txm.BlockedByDependencies(ctx, tx)
	require.NoError(t, err)
	assert.True(t, blocked)
}

func TestBlockedByDependencies_DependencyWithFailedReceipt_ReturnsTrue(t *testing.T) {
	txID := uuid.New()
	depID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "sequence", "indexed", "domain", "success", "tx_hash", "block_number", "tx_index", "log_index", "source", "failure_message", "revert_data", "contract_address"}).
					AddRow(depID, 1, "2024-01-01T00:00:00Z", "", false, nil, nil, nil, nil, nil, "failed", nil, nil),
			)
			// failDependentTransaction will attempt a new DB transaction to propagate the failure
			mc.db.ExpectBegin().WillReturnError(fmt.Errorf("mock: propagation not under test"))
		})
	defer done()

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{ID: &txID},
			DependsOn:   []uuid.UUID{depID},
		},
	}

	blocked, err := txm.BlockedByDependencies(ctx, tx)
	require.NoError(t, err)
	assert.True(t, blocked)
}

func TestBlockedByDependencies_DependencyWithError_ReturnsTrueAndError(t *testing.T) {
	txID := uuid.New()
	depID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnError(fmt.Errorf("db error"))
		})
	defer done()

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{ID: &txID},
			DependsOn:   []uuid.UUID{depID},
		},
	}

	blocked, err := txm.BlockedByDependencies(ctx, tx)
	assert.True(t, blocked)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestBlockedByDependencies_AllDependenciesSatisfied_ReturnsFalse(t *testing.T) {
	txID := uuid.New()
	depID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "sequence", "indexed", "domain", "success", "tx_hash", "block_number", "tx_index", "log_index", "source", "failure_message", "revert_data", "contract_address"}).
					AddRow(depID, 1, "2024-01-01T00:00:00Z", "", true, nil, nil, nil, nil, nil, nil, nil, nil),
			)
		})
	defer done()

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{ID: &txID},
			DependsOn:   []uuid.UUID{depID},
		},
	}

	blocked, err := txm.BlockedByDependencies(ctx, tx)
	require.NoError(t, err)
	assert.False(t, blocked)
}

func TestNotifyDependentTransactions_GetTransactionDependenciesWithTXError(t *testing.T) {
	prereqID := uuid.New()
	depsErr := fmt.Errorf("GetTransactionDependenciesWithTX failed")
	ctx, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			mc.db.ExpectQuery(`SELECT.*chained_private_txns`).WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnError(depsErr)
			mc.db.ExpectRollback()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: prereqID, ReceiptType: components.RT_Success},
		})
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetTransactionDependenciesWithTX failed")
}

func TestNotifyDependentTransactions_GetDependenciesError(t *testing.T) {
	prereqID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			mc.db.ExpectQuery(`SELECT.*chained_private_txns`).WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnError(fmt.Errorf("deps query failed"))
			mc.db.ExpectRollback()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: prereqID, ReceiptType: components.RT_Success},
		})
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deps query failed")
}

func TestNotifyDependentTransactions_GetResolvedTransactionByIDError(t *testing.T) {
	prereqID := uuid.New()
	depID := uuid.New()
	getResolvedErr := fmt.Errorf("GetResolvedTransactionByID failed")
	ctx, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts.*RETURNING").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(1))
			mc.db.ExpectQuery(`SELECT.*chained_private_txns`).WillReturnRows(sqlmock.NewRows([]string{}))
			// Return one dependent: depID depends on prereqID, so PrereqOf = [depID]
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(
				sqlmock.NewRows([]string{"transaction", "depends_on"}).AddRow(depID, prereqID),
			)
			// GetResolvedTransactionByID(depID) uses NOTX() and runs a transactions query; make it fail
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(getResolvedErr)
			mc.db.ExpectRollback()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: prereqID, ReceiptType: components.RT_Success},
		})
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetResolvedTransactionByID failed")
}

func TestNotifyDependentTransactions_SuccessWithDependent_CallsHandleTxResume(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.sequencerMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mc.sequencerMgr.On("HandleTxResume", mock.Anything, mock.MatchedBy(func(v *components.ValidatedTransaction) bool {
				return v.Transaction != nil && v.Transaction.ID != nil
			})).Return(nil).Once()
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	prereqID, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
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

	var depID uuid.UUID
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		ids, err := txm.PrepareTransactions(ctx, dbTX, &pldapi.TransactionInput{
			DependsOn: []uuid.UUID{*prereqID},
			TransactionBase: pldapi.TransactionBase{
				From:     "me",
				Type:     pldapi.TransactionTypePrivate.Enum(),
				Function: "doIt",
				To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
				Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
			},
			ABI: exampleABI,
		})
		if err != nil {
			return err
		}
		depID = ids[0]
		return nil
	})
	require.NoError(t, err)

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: *prereqID, ReceiptType: components.RT_Success},
		})
	})
	require.NoError(t, err)

	txm.sequencerMgr.(*componentsmocks.SequencerManager).AssertExpectations(t)
	_ = depID
}

func TestNotifyDependentTransactions_FailurePropagatesToDependent(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.sequencerMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	prereqID, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
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

	var depID uuid.UUID
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		ids, err := txm.PrepareTransactions(ctx, dbTX, &pldapi.TransactionInput{
			DependsOn: []uuid.UUID{*prereqID},
			TransactionBase: pldapi.TransactionBase{
				From:     "me",
				Type:     pldapi.TransactionTypePrivate.Enum(),
				Function: "doIt",
				To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
				Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
			},
			ABI: exampleABI,
		})
		if err != nil {
			return err
		}
		depID = ids[0]
		return nil
	})
	require.NoError(t, err)

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: *prereqID, ReceiptType: components.RT_FailedWithMessage, FailureMessage: "prereq failed"},
		})
	})
	require.NoError(t, err)

	receipt, err := txm.GetTransactionReceiptByID(ctx, depID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.False(t, receipt.Success)
	assert.Contains(t, receipt.FailureMessage, "Transaction dependency failed")
}

func TestNotifyDependentTransactions_HandleTxResumeError_LoggedAndCommitSucceeds(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.sequencerMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mc.sequencerMgr.On("HandleTxResume", mock.Anything, mock.Anything).Return(fmt.Errorf("resume failed")).Once()
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	prereqID, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
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

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := txm.PrepareTransactions(ctx, dbTX, &pldapi.TransactionInput{
			DependsOn: []uuid.UUID{*prereqID},
			TransactionBase: pldapi.TransactionBase{
				From:     "me",
				Type:     pldapi.TransactionTypePrivate.Enum(),
				Function: "doIt",
				To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
				Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
			},
			ABI: exampleABI,
		})
		return err
	})
	require.NoError(t, err)

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{TransactionID: *prereqID, ReceiptType: components.RT_Success},
		})
	})
	require.NoError(t, err)

	txm.sequencerMgr.(*componentsmocks.SequencerManager).AssertExpectations(t)
}

func ptr(u uuid.UUID) *uuid.UUID {
	return &u
}
