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
	seqcommon "github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
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
			mc.db.ExpectQuery("SELECT.*sequencer_activities").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, txID)
	assert.Regexp(t, "pop", err)
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

func TestResolveABIReferencesAndCacheConstructorSignatureWithNilTo(t *testing.T) {
	var abiHash = (pldtypes.Bytes32)(pldtypes.RandBytes(32))
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abis").WillReturnRows(mc.db.NewRows([]string{"hash", "abi"}).AddRow(
				abiHash.String(), `[{
					"type":"constructor",
					"inputs":[
						{"name":"group","type":"tuple","components":[{"name":"salt","type":"bytes32"},{"name":"members","type":"string[]"}]},
						{"name":"endorsementType","type":"string"},
						{"name":"evmVersion","type":"string"},
						{"name":"externalCallsEnabled","type":"bool"}
					]
				}]`,
			))
		})
	defer done()

	txID := uuid.New()
	txs, err := txm.resolveABIReferencesAndCache(ctx, txm.p.NOTX(), []*components.ResolvedTransaction{
		{Transaction: &pldapi.Transaction{
			ID: &txID,
			TransactionBase: pldapi.TransactionBase{
				Function:     "((bytes32,string[]),string,string,bool)",
				To:           nil,
				ABIReference: confutil.P(abiHash),
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, txs, 1)
	require.NotNil(t, txs[0].Function)
	assert.Equal(t, "((bytes32,string[]),string,string,bool)", txs[0].Function.Signature)
}

func TestResolveABIReferencesAndCacheFunctionWithNilToStillFails(t *testing.T) {
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
				To:           nil,
				ABIReference: confutil.P(abiHash),
			},
		}},
	})
	assert.Regexp(t, "PD012204", err)
}

func TestMapPersistedTXSequencingActivity(t *testing.T) {
	_, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners)
	defer done()

	localID := uint64(12345)
	subjectID := "subject-activity-id-123"
	timestamp := pldtypes.Timestamp(time.Now().UnixNano())
	activityType := "dispatch"
	sequencingNode := "node-abc"
	transactionID := uuid.New()

	psa := &seqcommon.DBSequencingActivity{
		LocalID:        &localID,
		SubjectID:      subjectID,
		Timestamp:      timestamp,
		ActivityType:   activityType,
		SequencingNode: sequencingNode,
		TransactionID:  transactionID,
	}

	result := txm.mapPersistedTXSequencingActivity(psa)

	require.NotNil(t, result)
	assert.Equal(t, &localID, result.LocalID)
	assert.Equal(t, subjectID, result.SubjectID)
	assert.Equal(t, timestamp, result.Timestamp)
	assert.Equal(t, activityType, result.ActivityType)
	assert.Equal(t, sequencingNode, result.SequencingNode)
	assert.Equal(t, transactionID, result.TransactionID)
}

func TestMapPersistedTXSequencingActivityWithNilLocalID(t *testing.T) {
	_, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners)
	defer done()

	subjectID := "subject-activity-id-456"
	timestamp := pldtypes.Timestamp(time.Now().UnixNano())
	activityType := "dispatch"
	sequencingNode := "node-xyz"
	transactionID := uuid.New()

	psa := &seqcommon.DBSequencingActivity{
		LocalID:        nil,
		SubjectID:      subjectID,
		Timestamp:      timestamp,
		ActivityType:   activityType,
		SequencingNode: sequencingNode,
		TransactionID:  transactionID,
	}

	result := txm.mapPersistedTXSequencingActivity(psa)

	require.NotNil(t, result)
	assert.Nil(t, result.LocalID)
	assert.Equal(t, subjectID, result.SubjectID)
	assert.Equal(t, timestamp, result.Timestamp)
	assert.Equal(t, activityType, result.ActivityType)
	assert.Equal(t, sequencingNode, result.SequencingNode)
	assert.Equal(t, transactionID, result.TransactionID)
}

func TestAddSequencerActivity_WithActivities(t *testing.T) {
	txID1 := uuid.New()
	txID2 := uuid.New()
	localID1 := uint64(100)
	localID2 := uint64(200)
	subjectID1 := "subject-1"
	subjectID2 := "subject-2"
	subjectID3 := "subject-3"
	timestamp1 := pldtypes.Timestamp(time.Now().UnixNano())
	timestamp2 := pldtypes.Timestamp(time.Now().UnixNano() + 1000)
	timestamp3 := pldtypes.Timestamp(time.Now().UnixNano() + 2000)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"id", "subject_id", "timestamp", "transaction_id", "activity_type", "submitting_node"}).
				AddRow(localID1, subjectID1, timestamp1, txID1, "dispatch", "node1").
				AddRow(localID2, subjectID2, timestamp2, txID1, "chained_dispatch", "node1").
				AddRow(nil, subjectID3, timestamp3, txID2, "dispatch", "node2")
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
	assert.Equal(t, subjectID1, result[0].SequencerActivity[0].SubjectID)
	assert.Equal(t, timestamp1, result[0].SequencerActivity[0].Timestamp)
	assert.Equal(t, "dispatch", result[0].SequencerActivity[0].ActivityType)
	assert.Equal(t, "node1", result[0].SequencerActivity[0].SequencingNode)
	assert.Equal(t, txID1, result[0].SequencerActivity[0].TransactionID)

	assert.Equal(t, &localID2, result[0].SequencerActivity[1].LocalID)
	assert.Equal(t, subjectID2, result[0].SequencerActivity[1].SubjectID)
	assert.Equal(t, timestamp2, result[0].SequencerActivity[1].Timestamp)
	assert.Equal(t, "chained_dispatch", result[0].SequencerActivity[1].ActivityType)
	assert.Equal(t, "node1", result[0].SequencerActivity[1].SequencingNode)
	assert.Equal(t, txID1, result[0].SequencerActivity[1].TransactionID)

	// Second transaction should have 1 sequencer activity
	require.NotNil(t, result[1].SequencerActivity)
	require.Equal(t, 1, len(result[1].SequencerActivity))
	assert.Nil(t, result[1].SequencerActivity[0].LocalID)
	assert.Equal(t, subjectID3, result[1].SequencerActivity[0].SubjectID)
	assert.Equal(t, timestamp3, result[1].SequencerActivity[0].Timestamp)
	assert.Equal(t, "dispatch", result[1].SequencerActivity[0].ActivityType)
	assert.Equal(t, "node2", result[1].SequencerActivity[0].SequencingNode)
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
	subjectID1 := "subject-1"
	timestamp1 := pldtypes.Timestamp(time.Now().UnixNano())

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"id", "subject_id", "timestamp", "transaction_id", "activity_type", "submitting_node"}).
				AddRow(localID1, subjectID1, timestamp1, txID1, "dispatch", "node1")
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
	assert.Equal(t, subjectID1, result[0].SequencerActivity[0].SubjectID)
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
	subjectID1 := "subject-1"
	subjectID2 := "subject-2"
	subjectID3 := "subject-3"
	timestamp1 := pldtypes.Timestamp(time.Now().UnixNano())
	timestamp2 := pldtypes.Timestamp(time.Now().UnixNano() + 1000)
	timestamp3 := pldtypes.Timestamp(time.Now().UnixNano() + 2000)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			rows := sqlmock.NewRows([]string{"id", "subject_id", "timestamp", "transaction_id", "activity_type", "submitting_node"}).
				AddRow(localID1, subjectID1, timestamp1, txID, "dispatch", "node1").
				AddRow(localID2, subjectID2, timestamp2, txID, "chained_dispatch", "node1").
				AddRow(localID3, subjectID3, timestamp3, txID, "dispatch", "node1")
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
	assert.Equal(t, subjectID1, result[0].SequencerActivity[0].SubjectID)
	assert.Equal(t, "dispatch", result[0].SequencerActivity[0].ActivityType)

	assert.Equal(t, &localID2, result[0].SequencerActivity[1].LocalID)
	assert.Equal(t, subjectID2, result[0].SequencerActivity[1].SubjectID)
	assert.Equal(t, "chained_dispatch", result[0].SequencerActivity[1].ActivityType)

	assert.Equal(t, &localID3, result[0].SequencerActivity[2].LocalID)
	assert.Equal(t, subjectID3, result[0].SequencerActivity[2].SubjectID)
	assert.Equal(t, "dispatch", result[0].SequencerActivity[2].ActivityType)
}

func TestMapPersistedChainedDispatch(t *testing.T) {
	_, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners)
	defer done()

	chainedTxID := uuid.New()
	transactionID := uuid.New()
	id := uuid.New()

	pd := &persistedChainedDispatch{
		ChainedTransaction: chainedTxID,
		Transaction:        transactionID,
		ID:                 id,
		Sender:             "sender-123",
		Domain:             "domain1",
		ContractAddress:    "0x1234567890123456789012345678901234567890",
	}

	result := txm.mapPersistedChainedDispatch(pd)

	require.NotNil(t, result)
	assert.Equal(t, chainedTxID.String(), result.ChainedTransactionID)
	assert.Equal(t, transactionID.String(), result.TransactionID)
	assert.Equal(t, id.String(), result.ID)
}
