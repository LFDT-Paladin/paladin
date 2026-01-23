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
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTransactionByIDFullFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFullPublicFail(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_history").WillReturnRows(sqlmock.NewRows([]string{"id", "tx_id"}).AddRow(uuid.New(), txID))
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*sequencer_activities").WillReturnRows(sqlmock.NewRows([]string{}))
		}, mockQueryPublicTxForTransactions(func(ids []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error) {
			return nil, fmt.Errorf("pop")
		}))
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, txID)
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFullPublicHistoryFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_history").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFullSequencerActivityFail(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_history").WillReturnRows(sqlmock.NewRows([]string{"id", "tx_id"}).AddRow(uuid.New(), txID))
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*sequencer_activities").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, txID)
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFullDispatchesFail(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_history").WillReturnRows(sqlmock.NewRows([]string{"id", "tx_id"}).AddRow(uuid.New(), txID))
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, txID)
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFullChainedTransactionsFail(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_history").WillReturnRows(sqlmock.NewRows([]string{"id", "tx_id"}).AddRow(uuid.New(), txID))
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnError(fmt.Errorf("chained transactions query error"))
		})
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, txID)
	assert.Regexp(t, "chained transactions query error", err)
}

func TestGetTransactionByIDFullPublicHistory(t *testing.T) {
	txID := uuid.New()
	to1 := pldtypes.RandAddress()
	to2 := pldtypes.RandAddress()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			rows := sqlmock.NewRows([]string{"id", "tx_id", "to"}).
				AddRow(uuid.New(), txID, to1).
				AddRow(uuid.New(), txID, to2)
			mc.db.ExpectQuery("SELECT.*transaction_history").WillReturnRows(rows)
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*sequencer_activities").WillReturnRows(sqlmock.NewRows([]string{}))
		}, mockQueryPublicTxForTransactions(func(ids []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error) {
			pubTX := map[uuid.UUID][]*pldapi.PublicTx{
				txID: {{
					To: to2,
				}},
			}
			return pubTX, nil
		}))
	defer done()

	tx, err := txm.GetTransactionByIDFull(ctx, txID)
	require.NoError(t, err)
	require.Equal(t, 2, len(tx.History))
	assert.Equal(t, to1, tx.History[0].To)
	assert.Equal(t, to2, tx.History[1].To)
}

func TestGetTransactionByIDFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionByID(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionDependenciesFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionDependencies(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetResolvedTransactionByIDFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetResolvedTransactionByID(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestResolveABIReferencesAndCacheFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abis").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.resolveABIReferencesAndCache(ctx, txm.p.NOTX(), []*components.ResolvedTransaction{
		{Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				ABIReference: confutil.P((pldtypes.Bytes32)(pldtypes.RandBytes(32))),
			},
		}},
	})
	assert.Regexp(t, "pop", err)
}

func TestResolveABIReferencesAndCacheBadFunc(t *testing.T) {
	var abiHash = (pldtypes.Bytes32)(pldtypes.RandBytes(32))
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abis").WillReturnRows(mc.db.NewRows([]string{"hash", "abi"}).AddRow(
				abiHash.String(), `[]`,
			))
		})
	defer done()

	_, err := txm.resolveABIReferencesAndCache(ctx, txm.p.NOTX(), []*components.ResolvedTransaction{
		{Transaction: &pldapi.Transaction{
			ID: confutil.P(uuid.New()),
			TransactionBase: pldapi.TransactionBase{
				Function:     "doStuff()",
				To:           pldtypes.RandAddress(),
				ABIReference: confutil.P(abiHash),
			},
		}},
	})
	assert.Regexp(t, "PD012206", err)
}

func TestMapPersistedTXSequencingActivity(t *testing.T) {
	_, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners)
	defer done()

	localID := uint64(12345)
	remoteID := "remote-activity-id-123"
	timestamp := pldtypes.Timestamp(time.Now().UnixNano())
	activityType := "dispatched"
	submittingNode := "node-abc"
	transactionID := uuid.New()

	psa := &sequencer.DBSequencingActivity{
		LocalID:        &localID,
		RemoteID:       remoteID,
		Timestamp:      timestamp,
		ActivityType:   activityType,
		SubmittingNode: submittingNode,
		TransactionID:  transactionID,
	}

	result := txm.mapPersistedTXSequencingActivity(psa)

	require.NotNil(t, result)
	assert.Equal(t, &localID, result.LocalID)
	assert.Equal(t, remoteID, result.RemoteID)
	assert.Equal(t, timestamp, result.Timestamp)
	assert.Equal(t, activityType, result.ActivityType)
	assert.Equal(t, submittingNode, result.SubmittingNode)
	assert.Equal(t, transactionID, result.TransactionID)
}

func TestMapPersistedTXSequencingActivityWithNilLocalID(t *testing.T) {
	_, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners)
	defer done()

	remoteID := "remote-activity-id-456"
	timestamp := pldtypes.Timestamp(time.Now().UnixNano())
	activityType := "dispatched"
	submittingNode := "node-xyz"
	transactionID := uuid.New()

	psa := &sequencer.DBSequencingActivity{
		LocalID:        nil,
		RemoteID:       remoteID,
		Timestamp:      timestamp,
		ActivityType:   activityType,
		SubmittingNode: submittingNode,
		TransactionID:  transactionID,
	}

	result := txm.mapPersistedTXSequencingActivity(psa)

	require.NotNil(t, result)
	assert.Nil(t, result.LocalID)
	assert.Equal(t, remoteID, result.RemoteID)
	assert.Equal(t, timestamp, result.Timestamp)
	assert.Equal(t, activityType, result.ActivityType)
	assert.Equal(t, submittingNode, result.SubmittingNode)
	assert.Equal(t, transactionID, result.TransactionID)
}

func TestAddSequencerActivity_WithActivities(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()
	localID1 := uint64(100)
	localID2 := uint64(200)
	remoteID1 := "remote-1"
	remoteID2 := "remote-2"
	remoteID3 := "remote-3"
	timestamp1 := pldtypes.Timestamp(time.Now().UnixNano())
	timestamp2 := pldtypes.Timestamp(time.Now().UnixNano() + 1000)
	timestamp3 := pldtypes.Timestamp(time.Now().UnixNano() + 2000)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"id", "remote_id", "timestamp", "transaction_id", "activity_type", "submitting_node"}).
				AddRow(localID1, remoteID1, timestamp1, txID1, "dispatched", "node1").
				AddRow(localID2, remoteID2, timestamp2, txID1, "confirmed", "node1").
				AddRow(nil, remoteID3, timestamp3, txID2, "dispatched", "node2")
			mc.db.ExpectQuery("SELECT.*sequencer_activities").WillReturnRows(rows)
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID1}},
		{Transaction: &pldapi.Transaction{ID: &txID2}},
	}

	result, err := txm.AddSequencerActivity(ctx, txm.p.NOTX(), []uuid.UUID{txID1, txID2}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 2, len(result))

	// First transaction should have 2 sequencer activities
	require.NotNil(t, result[0].SequencerActivity)
	require.Equal(t, 2, len(result[0].SequencerActivity))
	assert.Equal(t, &localID1, result[0].SequencerActivity[0].LocalID)
	assert.Equal(t, remoteID1, result[0].SequencerActivity[0].RemoteID)
	assert.Equal(t, timestamp1, result[0].SequencerActivity[0].Timestamp)
	assert.Equal(t, "dispatched", result[0].SequencerActivity[0].ActivityType)
	assert.Equal(t, "node1", result[0].SequencerActivity[0].SubmittingNode)
	assert.Equal(t, txID1, result[0].SequencerActivity[0].TransactionID)

	assert.Equal(t, &localID2, result[0].SequencerActivity[1].LocalID)
	assert.Equal(t, remoteID2, result[0].SequencerActivity[1].RemoteID)
	assert.Equal(t, timestamp2, result[0].SequencerActivity[1].Timestamp)
	assert.Equal(t, "confirmed", result[0].SequencerActivity[1].ActivityType)
	assert.Equal(t, "node1", result[0].SequencerActivity[1].SubmittingNode)
	assert.Equal(t, txID1, result[0].SequencerActivity[1].TransactionID)

	// Second transaction should have 1 sequencer activity
	require.NotNil(t, result[1].SequencerActivity)
	require.Equal(t, 1, len(result[1].SequencerActivity))
	assert.Nil(t, result[1].SequencerActivity[0].LocalID)
	assert.Equal(t, remoteID3, result[1].SequencerActivity[0].RemoteID)
	assert.Equal(t, timestamp3, result[1].SequencerActivity[0].Timestamp)
	assert.Equal(t, "dispatched", result[1].SequencerActivity[0].ActivityType)
	assert.Equal(t, "node2", result[1].SequencerActivity[0].SubmittingNode)
	assert.Equal(t, txID2, result[1].SequencerActivity[0].TransactionID)
}

func TestAddSequencerActivity_WithoutActivities(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*sequencer_activities").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID1}},
		{Transaction: &pldapi.Transaction{ID: &txID2}},
	}

	result, err := txm.AddSequencerActivity(ctx, txm.p.NOTX(), []uuid.UUID{txID1, txID2}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 2, len(result))

	// Transactions should not have SequencerActivity set (nil or empty)
	assert.Nil(t, result[0].SequencerActivity)
	assert.Nil(t, result[1].SequencerActivity)
}

func TestAddSequencerActivity_PartialActivities(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()
	txID3 := uuid.New()
	localID1 := uint64(100)
	remoteID1 := "remote-1"
	timestamp1 := pldtypes.Timestamp(time.Now().UnixNano())

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"id", "remote_id", "timestamp", "transaction_id", "activity_type", "submitting_node"}).
				AddRow(localID1, remoteID1, timestamp1, txID1, "dispatched", "node1")
			mc.db.ExpectQuery("SELECT.*sequencer_activities").WillReturnRows(rows)
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID1}},
		{Transaction: &pldapi.Transaction{ID: &txID2}},
		{Transaction: &pldapi.Transaction{ID: &txID3}},
	}

	result, err := txm.AddSequencerActivity(ctx, txm.p.NOTX(), []uuid.UUID{txID1, txID2, txID3}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 3, len(result))

	// First transaction should have sequencer activity
	require.NotNil(t, result[0].SequencerActivity)
	require.Equal(t, 1, len(result[0].SequencerActivity))
	assert.Equal(t, &localID1, result[0].SequencerActivity[0].LocalID)
	assert.Equal(t, remoteID1, result[0].SequencerActivity[0].RemoteID)
	assert.Equal(t, txID1, result[0].SequencerActivity[0].TransactionID)

	// Second and third transactions should not have sequencer activities
	assert.Nil(t, result[1].SequencerActivity)
	assert.Nil(t, result[2].SequencerActivity)
}

func TestAddSequencerActivity_MultipleActivitiesForSameTransaction(t *testing.T) {
	txID := uuid.New()
	localID1 := uint64(100)
	localID2 := uint64(200)
	localID3 := uint64(300)
	remoteID1 := "remote-1"
	remoteID2 := "remote-2"
	remoteID3 := "remote-3"
	timestamp1 := pldtypes.Timestamp(time.Now().UnixNano())
	timestamp2 := pldtypes.Timestamp(time.Now().UnixNano() + 1000)
	timestamp3 := pldtypes.Timestamp(time.Now().UnixNano() + 2000)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"id", "remote_id", "timestamp", "transaction_id", "activity_type", "submitting_node"}).
				AddRow(localID1, remoteID1, timestamp1, txID, "dispatched", "node1").
				AddRow(localID2, remoteID2, timestamp2, txID, "confirmed", "node1").
				AddRow(localID3, remoteID3, timestamp3, txID, "finalized", "node1")
			mc.db.ExpectQuery("SELECT.*sequencer_activities").WillReturnRows(rows)
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID}},
	}

	result, err := txm.AddSequencerActivity(ctx, txm.p.NOTX(), []uuid.UUID{txID}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 1, len(result))

	// Transaction should have all 3 sequencer activities
	require.NotNil(t, result[0].SequencerActivity)
	require.Equal(t, 3, len(result[0].SequencerActivity))

	// Verify all activities are mapped correctly
	assert.Equal(t, &localID1, result[0].SequencerActivity[0].LocalID)
	assert.Equal(t, remoteID1, result[0].SequencerActivity[0].RemoteID)
	assert.Equal(t, "dispatched", result[0].SequencerActivity[0].ActivityType)

	assert.Equal(t, &localID2, result[0].SequencerActivity[1].LocalID)
	assert.Equal(t, remoteID2, result[0].SequencerActivity[1].RemoteID)
	assert.Equal(t, "confirmed", result[0].SequencerActivity[1].ActivityType)

	assert.Equal(t, &localID3, result[0].SequencerActivity[2].LocalID)
	assert.Equal(t, remoteID3, result[0].SequencerActivity[2].RemoteID)
	assert.Equal(t, "finalized", result[0].SequencerActivity[2].ActivityType)
}

func TestAddDispatches_WithDispatches(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()
	dispatchID1 := "dispatch-1"
	dispatchID2 := "dispatch-2"
	dispatchID3 := "dispatch-3"
	publicTxAddr1 := pldtypes.RandAddress()
	publicTxAddr2 := pldtypes.RandAddress()
	publicTxAddr3 := pldtypes.RandAddress()
	publicTxID1 := uint64(100)
	publicTxID2 := uint64(200)
	publicTxID3 := uint64(300)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"id", "private_transaction_id", "public_transaction_address", "public_transaction_id"}).
				AddRow(dispatchID1, txID1.String(), publicTxAddr1, publicTxID1).
				AddRow(dispatchID2, txID1.String(), publicTxAddr2, publicTxID2).
				AddRow(dispatchID3, txID2.String(), publicTxAddr3, publicTxID3)
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(rows)
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID1}},
		{Transaction: &pldapi.Transaction{ID: &txID2}},
	}

	result, err := txm.AddDispatches(ctx, txm.p.NOTX(), []uuid.UUID{txID1, txID2}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 2, len(result))

	// First transaction should have 2 dispatches
	require.NotNil(t, result[0].Dispatches)
	require.Equal(t, 2, len(result[0].Dispatches))
	assert.Equal(t, dispatchID1, result[0].Dispatches[0].ID)
	assert.Equal(t, txID1.String(), result[0].Dispatches[0].PrivateTransactionID)
	assert.Equal(t, publicTxAddr1.String(), result[0].Dispatches[0].PublicTransactionAddress.String())
	assert.Equal(t, publicTxID1, result[0].Dispatches[0].PublicTransactionID)

	assert.Equal(t, dispatchID2, result[0].Dispatches[1].ID)
	assert.Equal(t, txID1.String(), result[0].Dispatches[1].PrivateTransactionID)
	assert.Equal(t, publicTxAddr2.String(), result[0].Dispatches[1].PublicTransactionAddress.String())
	assert.Equal(t, publicTxID2, result[0].Dispatches[1].PublicTransactionID)

	// Second transaction should have 1 dispatch
	require.NotNil(t, result[1].Dispatches)
	require.Equal(t, 1, len(result[1].Dispatches))
	assert.Equal(t, dispatchID3, result[1].Dispatches[0].ID)
	assert.Equal(t, txID2.String(), result[1].Dispatches[0].PrivateTransactionID)
	assert.Equal(t, publicTxAddr3.String(), result[1].Dispatches[0].PublicTransactionAddress.String())
	assert.Equal(t, publicTxID3, result[1].Dispatches[0].PublicTransactionID)
}

func TestAddDispatches_WithoutDispatches(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID1}},
		{Transaction: &pldapi.Transaction{ID: &txID2}},
	}

	result, err := txm.AddDispatches(ctx, txm.p.NOTX(), []uuid.UUID{txID1, txID2}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 2, len(result))

	// Transactions should not have Dispatches set (nil or empty)
	assert.Nil(t, result[0].Dispatches)
	assert.Nil(t, result[1].Dispatches)
}

func TestAddDispatches_PartialDispatches(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()
	txID3 := uuid.New()
	dispatchID1 := "dispatch-1"
	publicTxAddr1 := pldtypes.RandAddress()
	publicTxID1 := uint64(100)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"id", "private_transaction_id", "public_transaction_address", "public_transaction_id"}).
				AddRow(dispatchID1, txID1.String(), publicTxAddr1, publicTxID1)
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(rows)
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID1}},
		{Transaction: &pldapi.Transaction{ID: &txID2}},
		{Transaction: &pldapi.Transaction{ID: &txID3}},
	}

	result, err := txm.AddDispatches(ctx, txm.p.NOTX(), []uuid.UUID{txID1, txID2, txID3}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 3, len(result))

	// First transaction should have dispatch
	require.NotNil(t, result[0].Dispatches)
	require.Equal(t, 1, len(result[0].Dispatches))
	assert.Equal(t, dispatchID1, result[0].Dispatches[0].ID)
	assert.Equal(t, txID1.String(), result[0].Dispatches[0].PrivateTransactionID)
	assert.Equal(t, publicTxAddr1.String(), result[0].Dispatches[0].PublicTransactionAddress.String())
	assert.Equal(t, publicTxID1, result[0].Dispatches[0].PublicTransactionID)

	// Second and third transactions should not have dispatches
	assert.Nil(t, result[1].Dispatches)
	assert.Nil(t, result[2].Dispatches)
}

func TestAddDispatches_MultipleDispatchesForSameTransaction(t *testing.T) {
	txID := uuid.New()
	dispatchID1 := "dispatch-1"
	dispatchID2 := "dispatch-2"
	dispatchID3 := "dispatch-3"
	publicTxAddr1 := pldtypes.RandAddress()
	publicTxAddr2 := pldtypes.RandAddress()
	publicTxAddr3 := pldtypes.RandAddress()
	publicTxID1 := uint64(100)
	publicTxID2 := uint64(200)
	publicTxID3 := uint64(300)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"id", "private_transaction_id", "public_transaction_address", "public_transaction_id"}).
				AddRow(dispatchID1, txID.String(), publicTxAddr1, publicTxID1).
				AddRow(dispatchID2, txID.String(), publicTxAddr2, publicTxID2).
				AddRow(dispatchID3, txID.String(), publicTxAddr3, publicTxID3)
			mc.db.ExpectQuery("SELECT.*dispatches").WillReturnRows(rows)
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID}},
	}

	result, err := txm.AddDispatches(ctx, txm.p.NOTX(), []uuid.UUID{txID}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 1, len(result))

	// Transaction should have all 3 dispatches
	require.NotNil(t, result[0].Dispatches)
	require.Equal(t, 3, len(result[0].Dispatches))

	// Verify all dispatches are mapped correctly
	assert.Equal(t, dispatchID1, result[0].Dispatches[0].ID)
	assert.Equal(t, txID.String(), result[0].Dispatches[0].PrivateTransactionID)
	assert.Equal(t, publicTxAddr1.String(), result[0].Dispatches[0].PublicTransactionAddress.String())
	assert.Equal(t, publicTxID1, result[0].Dispatches[0].PublicTransactionID)

	assert.Equal(t, dispatchID2, result[0].Dispatches[1].ID)
	assert.Equal(t, txID.String(), result[0].Dispatches[1].PrivateTransactionID)
	assert.Equal(t, publicTxAddr2.String(), result[0].Dispatches[1].PublicTransactionAddress.String())
	assert.Equal(t, publicTxID2, result[0].Dispatches[1].PublicTransactionID)

	assert.Equal(t, dispatchID3, result[0].Dispatches[2].ID)
	assert.Equal(t, txID.String(), result[0].Dispatches[2].PrivateTransactionID)
	assert.Equal(t, publicTxAddr3.String(), result[0].Dispatches[2].PublicTransactionAddress.String())
	assert.Equal(t, publicTxID3, result[0].Dispatches[2].PublicTransactionID)
}

func TestMapPersistedChainedTransaction(t *testing.T) {
	_, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners)
	defer done()

	chainedTxID := uuid.New()
	transactionID := uuid.New()
	localID := uuid.New()

	pd := &persistedChainedPrivateTxn{
		ChainedTransaction: chainedTxID,
		Transaction:        transactionID,
		ID:                 localID,
		Sender:             "sender-123",
		Domain:             "domain1",
		ContractAddress:    "0x1234567890123456789012345678901234567890",
	}

	result := txm.mapPersistedChainedTransaction(pd)

	require.NotNil(t, result)
	assert.Equal(t, chainedTxID.String(), result.ChainedTransactionID)
	assert.Equal(t, transactionID.String(), result.TransactionID)
	assert.Equal(t, localID.String(), result.LocalID)
}

func TestAddChainedTranasctions_WithChainedTransactions(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()
	chainedTxID1 := uuid.New()
	chainedTxID2 := uuid.New()
	chainedTxID3 := uuid.New()
	localID1 := uuid.New()
	localID2 := uuid.New()
	localID3 := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address", "id"}).
				AddRow(chainedTxID1, txID1, "sender1", "domain1", "0x1111", localID1).
				AddRow(chainedTxID2, txID1, "sender2", "domain2", "0x2222", localID2).
				AddRow(chainedTxID3, txID2, "sender3", "domain3", "0x3333", localID3)
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(rows)
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID1}},
		{Transaction: &pldapi.Transaction{ID: &txID2}},
	}

	result, err := txm.AddChainedTranasctions(ctx, txm.p.NOTX(), []uuid.UUID{txID1, txID2}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 2, len(result))

	// First transaction should have 2 chained transactions
	require.NotNil(t, result[0].ChainedPrivateTransactions)
	require.Equal(t, 2, len(result[0].ChainedPrivateTransactions))
	assert.Equal(t, chainedTxID1.String(), result[0].ChainedPrivateTransactions[0].ChainedTransactionID)
	assert.Equal(t, txID1.String(), result[0].ChainedPrivateTransactions[0].TransactionID)
	assert.Equal(t, localID1.String(), result[0].ChainedPrivateTransactions[0].LocalID)

	assert.Equal(t, chainedTxID2.String(), result[0].ChainedPrivateTransactions[1].ChainedTransactionID)
	assert.Equal(t, txID1.String(), result[0].ChainedPrivateTransactions[1].TransactionID)
	assert.Equal(t, localID2.String(), result[0].ChainedPrivateTransactions[1].LocalID)

	// Second transaction should have 1 chained transaction
	require.NotNil(t, result[1].ChainedPrivateTransactions)
	require.Equal(t, 1, len(result[1].ChainedPrivateTransactions))
	assert.Equal(t, chainedTxID3.String(), result[1].ChainedPrivateTransactions[0].ChainedTransactionID)
	assert.Equal(t, txID2.String(), result[1].ChainedPrivateTransactions[0].TransactionID)
	assert.Equal(t, localID3.String(), result[1].ChainedPrivateTransactions[0].LocalID)
}

func TestAddChainedTranasctions_WithoutChainedTransactions(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID1}},
		{Transaction: &pldapi.Transaction{ID: &txID2}},
	}

	result, err := txm.AddChainedTranasctions(ctx, txm.p.NOTX(), []uuid.UUID{txID1, txID2}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 2, len(result))

	// Transactions should not have ChainedPrivateTransactions set (nil or empty)
	assert.Nil(t, result[0].ChainedPrivateTransactions)
	assert.Nil(t, result[1].ChainedPrivateTransactions)
}

func TestAddChainedTranasctions_PartialChainedTransactions(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()
	txID3 := uuid.New()
	chainedTxID1 := uuid.New()
	localID1 := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address", "id"}).
				AddRow(chainedTxID1, txID1, "sender1", "domain1", "0x1111", localID1)
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(rows)
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID1}},
		{Transaction: &pldapi.Transaction{ID: &txID2}},
		{Transaction: &pldapi.Transaction{ID: &txID3}},
	}

	result, err := txm.AddChainedTranasctions(ctx, txm.p.NOTX(), []uuid.UUID{txID1, txID2, txID3}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 3, len(result))

	// First transaction should have chained transaction
	require.NotNil(t, result[0].ChainedPrivateTransactions)
	require.Equal(t, 1, len(result[0].ChainedPrivateTransactions))
	assert.Equal(t, chainedTxID1.String(), result[0].ChainedPrivateTransactions[0].ChainedTransactionID)
	assert.Equal(t, txID1.String(), result[0].ChainedPrivateTransactions[0].TransactionID)
	assert.Equal(t, localID1.String(), result[0].ChainedPrivateTransactions[0].LocalID)

	// Second and third transactions should not have chained transactions
	assert.Nil(t, result[1].ChainedPrivateTransactions)
	assert.Nil(t, result[2].ChainedPrivateTransactions)
}

func TestAddChainedTranasctions_MultipleChainedTransactionsForSameTransaction(t *testing.T) {
	txID := uuid.New()
	chainedTxID1 := uuid.New()
	chainedTxID2 := uuid.New()
	chainedTxID3 := uuid.New()
	localID1 := uuid.New()
	localID2 := uuid.New()
	localID3 := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"chained_transaction", "transaction", "sender", "domain", "contract_address", "id"}).
				AddRow(chainedTxID1, txID, "sender1", "domain1", "0x1111", localID1).
				AddRow(chainedTxID2, txID, "sender2", "domain2", "0x2222", localID2).
				AddRow(chainedTxID3, txID, "sender3", "domain3", "0x3333", localID3)
			mc.db.ExpectQuery("SELECT.*chained_private_txns").WillReturnRows(rows)
		})
	defer done()

	ptxs := []*pldapi.TransactionFull{
		{Transaction: &pldapi.Transaction{ID: &txID}},
	}

	result, err := txm.AddChainedTranasctions(ctx, txm.p.NOTX(), []uuid.UUID{txID}, ptxs)
	require.NoError(t, err)
	require.Equal(t, 1, len(result))

	// Transaction should have all 3 chained transactions
	require.NotNil(t, result[0].ChainedPrivateTransactions)
	require.Equal(t, 3, len(result[0].ChainedPrivateTransactions))

	// Verify all chained transactions are mapped correctly
	assert.Equal(t, chainedTxID1.String(), result[0].ChainedPrivateTransactions[0].ChainedTransactionID)
	assert.Equal(t, txID.String(), result[0].ChainedPrivateTransactions[0].TransactionID)
	assert.Equal(t, localID1.String(), result[0].ChainedPrivateTransactions[0].LocalID)

	assert.Equal(t, chainedTxID2.String(), result[0].ChainedPrivateTransactions[1].ChainedTransactionID)
	assert.Equal(t, txID.String(), result[0].ChainedPrivateTransactions[1].TransactionID)
	assert.Equal(t, localID2.String(), result[0].ChainedPrivateTransactions[1].LocalID)

	assert.Equal(t, chainedTxID3.String(), result[0].ChainedPrivateTransactions[2].ChainedTransactionID)
	assert.Equal(t, txID.String(), result[0].ChainedPrivateTransactions[2].TransactionID)
	assert.Equal(t, localID3.String(), result[0].ChainedPrivateTransactions[2].LocalID)
}
