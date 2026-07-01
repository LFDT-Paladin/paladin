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

package syncpoints

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPersistDispatchBatch_EmptyBatch(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	conf := &pldconf.FlushWriterConfig{
		WorkerCount:  confutil.P(1),
		BatchTimeout: confutil.P("100ms"),
		BatchMaxSize: confutil.P(10),
	}

	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)
	transportMgr.On("LocalNodeName").Return("node1").Maybe()

	sp := NewSyncPoints(ctx, conf, mp.P, txMgr, pubTxMgr, transportMgr).(*syncPoints)
	sp.Start()
	defer sp.Close()

	dsw := componentsmocks.NewDomainStateWriter(t)
	dsw.On("Flush", mock.Anything, mock.Anything).Return(nil).Maybe()

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	contractAddr := pldtypes.RandAddress()
	dispatchBatch := &DispatchBatch{
		PublicDispatches:     []*PublicDispatch{},
		PrivateDispatches:    []*components.ChainedPrivateTransaction{},
		PreparedTransactions: []*components.PreparedTransactionWithRefs{},
	}

	err = sp.PersistDispatchBatch(ctx, dsw, *contractAddr, uuid.New(), dispatchBatch, []*components.StateDistribution{}, []*components.PreparedTransactionWithRefs{})
	require.NoError(t, err)
	require.NoError(t, mp.Mock.ExpectationsWereMet())
}

func TestPersistDispatchBatch_WithPreparedTxnDistributions_LocalNode(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	conf := &pldconf.FlushWriterConfig{
		WorkerCount:  confutil.P(1),
		BatchTimeout: confutil.P("100ms"),
		BatchMaxSize: confutil.P(10),
	}

	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)
	transportMgr.On("LocalNodeName").Return("node1")

	sp := NewSyncPoints(ctx, conf, mp.P, txMgr, pubTxMgr, transportMgr).(*syncPoints)
	sp.Start()
	defer sp.Close()

	dsw := componentsmocks.NewDomainStateWriter(t)
	dsw.On("Flush", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Create a prepared transaction distribution for local node
	preparedTxn := &components.PreparedTransactionWithRefs{
		PreparedTransactionBase: &pldapi.PreparedTransactionBase{
			Transaction: pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					From: "identity@node1", // Local node
				},
			},
		},
	}

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	contractAddr := pldtypes.RandAddress()
	dispatchBatch := &DispatchBatch{
		PublicDispatches:     []*PublicDispatch{},
		PrivateDispatches:    []*components.ChainedPrivateTransaction{},
		PreparedTransactions: []*components.PreparedTransactionWithRefs{},
	}

	txMgr.On("WritePreparedTransactions", mock.Anything, mock.Anything, mock.MatchedBy(func(txns []*components.PreparedTransactionWithRefs) bool {
		return len(txns) == 1 && txns[0] == preparedTxn
	})).Return(nil)

	err = sp.PersistDispatchBatch(ctx, dsw, *contractAddr, uuid.New(), dispatchBatch, []*components.StateDistribution{}, []*components.PreparedTransactionWithRefs{preparedTxn})
	require.NoError(t, err)
	require.NoError(t, mp.Mock.ExpectationsWereMet())
	txMgr.AssertExpectations(t)
}

func newTestSyncPoints(t *testing.T, localNode string) (*syncPoints, *mockpersistence.SQLMockProvider, *componentsmocks.TXManager, *componentsmocks.PublicTxManager, *componentsmocks.TransportManager) {
	t.Helper()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	conf := &pldconf.FlushWriterConfig{
		WorkerCount:  confutil.P(1),
		BatchTimeout: confutil.P("100ms"),
		BatchMaxSize: confutil.P(10),
	}

	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)
	transportMgr.On("LocalNodeName").Return(localNode).Maybe()

	sp := NewSyncPoints(context.Background(), conf, mp.P, txMgr, pubTxMgr, transportMgr).(*syncPoints)
	sp.Start()
	t.Cleanup(sp.Close)
	return sp, mp, txMgr, pubTxMgr, transportMgr
}

func newTestDomainStateWriter(t *testing.T) *componentsmocks.DomainStateWriter {
	t.Helper()
	dsw := componentsmocks.NewDomainStateWriter(t)
	dsw.On("Flush", mock.Anything, mock.Anything).Return(nil).Maybe()
	return dsw
}

func TestPersistDispatchBatch_WithRemotePreparedTxnDistribution(t *testing.T) {
	ctx := context.Background()
	sp, mp, _, _, transportMgr := newTestSyncPoints(t, "node1")
	dsw := newTestDomainStateWriter(t)

	remotePreparedTxn := &components.PreparedTransactionWithRefs{
		PreparedTransactionBase: &pldapi.PreparedTransactionBase{
			Transaction: pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					From: "identity@node2", // remote node → goes to preparedReliableMsgs
				},
			},
		},
	}
	transportMgr.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	err := sp.PersistDispatchBatch(ctx, dsw, *pldtypes.RandAddress(), uuid.New(), &DispatchBatch{}, []*components.StateDistribution{}, []*components.PreparedTransactionWithRefs{remotePreparedTxn})
	require.NoError(t, err)
}

func TestPersistDispatchBatch_WithStateDistributions(t *testing.T) {
	ctx := context.Background()
	sp, mp, _, _, transportMgr := newTestSyncPoints(t, "node1")
	dsw := newTestDomainStateWriter(t)

	stateDistribution := &components.StateDistribution{
		IdentityLocator: "identity@node2",
	}
	transportMgr.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	err := sp.PersistDispatchBatch(ctx, dsw, *pldtypes.RandAddress(), uuid.New(), &DispatchBatch{}, []*components.StateDistribution{stateDistribution}, []*components.PreparedTransactionWithRefs{})
	require.NoError(t, err)
}

func TestPersistDispatchBatch_WithPublicDispatch_LocalBinding(t *testing.T) {
	ctx := context.Background()
	sp, mp, _, pubTxMgr, _ := newTestSyncPoints(t, "node1")
	dsw := newTestDomainStateWriter(t)

	txID := uuid.New()
	localID := uint64(42)
	pubTxMgr.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]*pldapi.PublicTx{{LocalID: &localID}}, nil)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectExec("INSERT.*dispatches").WillReturnResult(sqlmock.NewResult(1, 1))
	mp.Mock.ExpectQuery("INSERT.*sequencer_activities").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mp.Mock.ExpectCommit()

	dispatchBatch := &DispatchBatch{
		PublicDispatches: []*PublicDispatch{
			{
				PublicTxs: []*components.PublicTxSubmission{{
					Bindings: []*components.PaladinTXReference{
						{
							// non-matching TransactionID → exercises the `continue` branch
							TransactionID:     uuid.New(),
							TransactionSender: "identity@node1",
						},
						{
							TransactionID:     txID,
							TransactionSender: "identity@node1", // local binding
						},
					},
				}},
				PrivateTransactionDispatches: []*DispatchPersisted{{
					TransactionID: txID.String(),
				}},
			},
		},
	}

	err := sp.PersistDispatchBatch(ctx, dsw, *pldtypes.RandAddress(), uuid.New(), dispatchBatch, nil, nil)
	require.NoError(t, err)
}

func TestPersistDispatchBatch_WithPublicDispatch_RemoteBinding(t *testing.T) {
	ctx := context.Background()
	sp, mp, _, pubTxMgr, transportMgr := newTestSyncPoints(t, "node1")
	dsw := newTestDomainStateWriter(t)

	txID := uuid.New()
	localID := uint64(43)
	pubTxMgr.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]*pldapi.PublicTx{{LocalID: &localID}}, nil)
	transportMgr.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectExec("INSERT.*dispatches").WillReturnResult(sqlmock.NewResult(1, 1))
	mp.Mock.ExpectCommit()

	dispatchBatch := &DispatchBatch{
		PublicDispatches: []*PublicDispatch{
			{
				PublicTxs: []*components.PublicTxSubmission{{
					Bindings: []*components.PaladinTXReference{{
						TransactionID:     txID,
						TransactionSender: "identity@node2", // remote binding → sequencer activity sent as ReliableMessage
					}},
				}},
				PrivateTransactionDispatches: []*DispatchPersisted{{
					TransactionID: txID.String(),
				}},
			},
		},
	}

	err := sp.PersistDispatchBatch(ctx, dsw, *pldtypes.RandAddress(), uuid.New(), dispatchBatch, nil, nil)
	require.NoError(t, err)
}

func TestPersistDispatchBatch_WithPrivateDispatch_Local(t *testing.T) {
	ctx := context.Background()
	sp, mp, txMgr, _, _ := newTestSyncPoints(t, "node1")
	dsw := newTestDomainStateWriter(t)

	originalTxID := uuid.New()
	txMgr.On("ChainPrivateTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("INSERT.*sequencer_activities").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mp.Mock.ExpectCommit()

	dispatchBatch := &DispatchBatch{
		PrivateDispatches: []*components.ChainedPrivateTransaction{{
			OriginalTransaction:   originalTxID,
			OriginalSenderLocator: "identity@node1", // local sender → goes to localSequencerActivities
		}},
	}

	err := sp.PersistDispatchBatch(ctx, dsw, *pldtypes.RandAddress(), uuid.New(), dispatchBatch, nil, nil)
	require.NoError(t, err)
}

func TestPersistDispatchBatch_WithPrivateDispatch_Remote(t *testing.T) {
	ctx := context.Background()
	sp, mp, txMgr, _, transportMgr := newTestSyncPoints(t, "node1")
	dsw := newTestDomainStateWriter(t)

	originalTxID := uuid.New()
	txMgr.On("ChainPrivateTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	transportMgr.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	dispatchBatch := &DispatchBatch{
		PrivateDispatches: []*components.ChainedPrivateTransaction{{
			OriginalTransaction:   originalTxID,
			OriginalSenderLocator: "identity@node2", // remote sender → sequencer activity sent as ReliableMessage
		}},
	}

	err := sp.PersistDispatchBatch(ctx, dsw, *pldtypes.RandAddress(), uuid.New(), dispatchBatch, nil, nil)
	require.NoError(t, err)
}

func TestPersistDeployDispatchBatch_EmptyBatch(t *testing.T) {
	ctx := context.Background()
	sp, mp, _, _, _ := newTestSyncPoints(t, "node1")

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	err := sp.PersistDeployDispatchBatch(ctx, uuid.New(), &DispatchBatch{})
	require.NoError(t, err)
}

func TestPersistDeployDispatchBatch_WithPublicDispatches(t *testing.T) {
	ctx := context.Background()
	sp, mp, _, pubTxMgr, _ := newTestSyncPoints(t, "node1")

	localID := uint64(99)
	pubTxMgr.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]*pldapi.PublicTx{{LocalID: &localID}}, nil)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectExec("INSERT.*dispatches").WillReturnResult(sqlmock.NewResult(1, 1))
	mp.Mock.ExpectCommit()

	// No pre-allocation of IDs → dispatch.ID == "" → covered branch in writeDispatchOperations
	dispatchBatch := &DispatchBatch{
		PublicDispatches: []*PublicDispatch{{
			PublicTxs: []*components.PublicTxSubmission{{
				Bindings: []*components.PaladinTXReference{},
			}},
			PrivateTransactionDispatches: []*DispatchPersisted{{
				TransactionID: uuid.New().String(),
				// ID intentionally empty to exercise the ID allocation branch
			}},
		}},
	}

	err := sp.PersistDeployDispatchBatch(ctx, uuid.New(), dispatchBatch)
	require.NoError(t, err)
}

func TestPersistDeployDispatchBatch_DBInsertError(t *testing.T) {
	ctx := context.Background()
	sp, mp, _, pubTxMgr, _ := newTestSyncPoints(t, "node1")

	localID := uint64(100)
	pubTxMgr.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]*pldapi.PublicTx{{LocalID: &localID}}, nil)

	dbErr := errors.New("db insert failed")
	mp.Mock.ExpectBegin()
	mp.Mock.ExpectExec("INSERT.*dispatches").WillReturnError(dbErr)
	mp.Mock.ExpectRollback()

	dispatchBatch := &DispatchBatch{
		PublicDispatches: []*PublicDispatch{{
			PublicTxs: []*components.PublicTxSubmission{{
				Bindings: []*components.PaladinTXReference{},
			}},
			PrivateTransactionDispatches: []*DispatchPersisted{{
				TransactionID: uuid.New().String(),
			}},
		}},
	}

	err := sp.PersistDeployDispatchBatch(ctx, uuid.New(), dispatchBatch)
	require.Error(t, err)
	assert.ErrorContains(t, err, "db insert failed")
}

func TestPersistDispatchBatch_SequencerActivitiesError(t *testing.T) {
	ctx := context.Background()
	sp, mp, _, pubTxMgr, _ := newTestSyncPoints(t, "node1")
	dsw := newTestDomainStateWriter(t)

	txID := uuid.New()
	localID := uint64(44)
	pubTxMgr.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]*pldapi.PublicTx{{LocalID: &localID}}, nil)

	dbErr := errors.New("sequencer_activities insert failed")
	mp.Mock.ExpectBegin()
	mp.Mock.ExpectExec("INSERT.*dispatches").WillReturnResult(sqlmock.NewResult(1, 1))
	mp.Mock.ExpectQuery("INSERT.*sequencer_activities").WillReturnError(dbErr)
	mp.Mock.ExpectRollback()

	dispatchBatch := &DispatchBatch{
		PublicDispatches: []*PublicDispatch{{
			PublicTxs: []*components.PublicTxSubmission{{
				Bindings: []*components.PaladinTXReference{{
					TransactionID:     txID,
					TransactionSender: "identity@node1",
				}},
			}},
			PrivateTransactionDispatches: []*DispatchPersisted{{
				TransactionID: txID.String(),
			}},
		}},
	}

	err := sp.PersistDispatchBatch(ctx, dsw, *pldtypes.RandAddress(), uuid.New(), dispatchBatch, nil, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "sequencer_activities insert failed")
}
