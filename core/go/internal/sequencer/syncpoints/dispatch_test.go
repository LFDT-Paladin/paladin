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
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
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

	dCtx := componentsmocks.NewDomainContext(t)
	dCtx.On("Ctx").Return(ctx).Maybe()
	dCtxID := uuid.New()
	dCtx.On("Info").Return(components.DomainContextInfo{ID: dCtxID}).Maybe()
	dCtx.On("Flush", mock.Anything).Return(nil).Maybe()

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	contractAddr := pldtypes.RandAddress()
	dispatchBatch := &DispatchBatch{
		PublicDispatches:     []*PublicDispatch{},
		PrivateDispatches:    []*components.ChainedPrivateTransaction{},
		PreparedTransactions: []*components.PreparedTransactionWithRefs{},
	}

	err = sp.PersistDispatchBatch(dCtx, *contractAddr, dispatchBatch, []*components.StateDistribution{}, []*components.PreparedTransactionWithRefs{})
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

	dCtx := componentsmocks.NewDomainContext(t)
	dCtx.On("Ctx").Return(ctx).Maybe()
	dCtxID := uuid.New()
	dCtx.On("Info").Return(components.DomainContextInfo{ID: dCtxID}).Maybe()
	dCtx.On("Flush", mock.Anything).Return(nil).Maybe()

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

	err = sp.PersistDispatchBatch(dCtx, *contractAddr, dispatchBatch, []*components.StateDistribution{}, []*components.PreparedTransactionWithRefs{preparedTxn})
	require.NoError(t, err)
	require.NoError(t, mp.Mock.ExpectationsWereMet())
	txMgr.AssertExpectations(t)
}

func TestPersistDeployDispatchBatch_EmptyBatch(t *testing.T) {
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

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	dispatchBatch := &DispatchBatch{
		PublicDispatches:     []*PublicDispatch{},
		PrivateDispatches:    []*components.ChainedPrivateTransaction{},
		PreparedTransactions: []*components.PreparedTransactionWithRefs{},
	}

	err = sp.PersistDeployDispatchBatch(ctx, dispatchBatch)
	require.NoError(t, err)
	require.NoError(t, mp.Mock.ExpectationsWereMet())
}

func TestPersistDeployDispatchBatch_WithEmptyPublicDispatches(t *testing.T) {
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

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	dispatchBatch := &DispatchBatch{
		PublicDispatches: []*PublicDispatch{
			{
				PublicTxs:                    []*components.PublicTxSubmission{},
				PrivateTransactionDispatches: []*DispatchPersisted{},
			},
		},
		PrivateDispatches:    []*components.ChainedPrivateTransaction{},
		PreparedTransactions: []*components.PreparedTransactionWithRefs{},
	}

	err = sp.PersistDeployDispatchBatch(ctx, dispatchBatch)
	require.NoError(t, err)
	require.NoError(t, mp.Mock.ExpectationsWereMet())
}

func TestPersistDispatchBatch_WithStateDistributions(t *testing.T) {
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

	dCtx := componentsmocks.NewDomainContext(t)
	dCtx.On("Ctx").Return(ctx).Maybe()
	dCtxID := uuid.New()
	dCtx.On("Info").Return(components.DomainContextInfo{ID: dCtxID}).Maybe()
	dCtx.On("Flush", mock.Anything).Return(nil).Maybe()

	stateDist := &components.StateDistribution{
		IdentityLocator: "identity@node2",
	}
	transportMgr.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(msgs []*pldapi.ReliableMessage) bool {
		if len(msgs) != 1 {
			return false
		}
		return msgs[0].Node == "node2" && msgs[0].MessageType == pldapi.RMTState.Enum()
	})).Return(nil)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	contractAddr := pldtypes.RandAddress()
	dispatchBatch := &DispatchBatch{
		PublicDispatches:     []*PublicDispatch{},
		PrivateDispatches:    []*components.ChainedPrivateTransaction{},
		PreparedTransactions: []*components.PreparedTransactionWithRefs{},
	}

	err = sp.PersistDispatchBatch(dCtx, *contractAddr, dispatchBatch, []*components.StateDistribution{stateDist}, []*components.PreparedTransactionWithRefs{})
	require.NoError(t, err)
	require.NoError(t, mp.Mock.ExpectationsWereMet())
	transportMgr.AssertExpectations(t)
}

func TestPersistDispatchBatch_WithPreparedTxnDistributions_RemoteNode(t *testing.T) {
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

	dCtx := componentsmocks.NewDomainContext(t)
	dCtx.On("Ctx").Return(ctx).Maybe()
	dCtxID := uuid.New()
	dCtx.On("Info").Return(components.DomainContextInfo{ID: dCtxID}).Maybe()
	dCtx.On("Flush", mock.Anything).Return(nil).Maybe()

	preparedTxn := &components.PreparedTransactionWithRefs{
		PreparedTransactionBase: &pldapi.PreparedTransactionBase{
			Transaction: pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					From: "identity@node2",
				},
			},
		},
	}
	transportMgr.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(msgs []*pldapi.ReliableMessage) bool {
		if len(msgs) != 1 {
			return false
		}
		return msgs[0].Node == "node2" && msgs[0].MessageType == pldapi.RMTPreparedTransaction.Enum()
	})).Return(nil)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	contractAddr := pldtypes.RandAddress()
	dispatchBatch := &DispatchBatch{
		PublicDispatches:     []*PublicDispatch{},
		PrivateDispatches:    []*components.ChainedPrivateTransaction{},
		PreparedTransactions: []*components.PreparedTransactionWithRefs{},
	}

	err = sp.PersistDispatchBatch(dCtx, *contractAddr, dispatchBatch, []*components.StateDistribution{}, []*components.PreparedTransactionWithRefs{preparedTxn})
	require.NoError(t, err)
	require.NoError(t, mp.Mock.ExpectationsWereMet())
	transportMgr.AssertExpectations(t)
}

func TestPersistDispatchBatch_WithPrivateDispatches_RemoteNode(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	conf := &pldconf.FlushWriterConfig{
		WorkerCount:  confutil.P(1),
		BatchTimeout: confutil.P("100ms"),
		BatchMaxSize: confutil.P(10),
	}

	txMgr := componentsmocks.NewTXManager(t)
	txMgr.On("ChainPrivateTransactions", mock.Anything, mock.Anything, mock.MatchedBy(func(txns []*components.ChainedPrivateTransaction) bool {
		return len(txns) == 1 && txns[0].OriginalSenderLocator == "identity@node2"
	})).Return(nil)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)
	transportMgr.On("LocalNodeName").Return("node1")
	transportMgr.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(msgs []*pldapi.ReliableMessage) bool {
		if len(msgs) != 1 {
			return false
		}
		return msgs[0].Node == "node2" && msgs[0].MessageType == pldapi.RMTSequencingActivity.Enum()
	})).Return(nil)

	sp := NewSyncPoints(ctx, conf, mp.P, txMgr, pubTxMgr, transportMgr).(*syncPoints)
	sp.Start()
	defer sp.Close()

	dCtx := componentsmocks.NewDomainContext(t)
	dCtx.On("Ctx").Return(ctx).Maybe()
	dCtxID := uuid.New()
	dCtx.On("Info").Return(components.DomainContextInfo{ID: dCtxID}).Maybe()
	dCtx.On("Flush", mock.Anything).Return(nil).Maybe()

	origTxID := uuid.New()
	privateDispatch := &components.ChainedPrivateTransaction{
		OriginalSenderLocator: "identity@node2",
		OriginalTransaction:   origTxID,
	}

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	contractAddr := pldtypes.RandAddress()
	dispatchBatch := &DispatchBatch{
		PublicDispatches: []*PublicDispatch{},
		PrivateDispatches: []*components.ChainedPrivateTransaction{
			privateDispatch,
		},
		PreparedTransactions: []*components.PreparedTransactionWithRefs{},
	}

	err = sp.PersistDispatchBatch(dCtx, *contractAddr, dispatchBatch, []*components.StateDistribution{}, []*components.PreparedTransactionWithRefs{})
	require.NoError(t, err)
	require.NoError(t, mp.Mock.ExpectationsWereMet())
	txMgr.AssertExpectations(t)
	transportMgr.AssertExpectations(t)
}

func TestPersistDispatchBatch_WithPublicDispatchBindings_RemoteNode(t *testing.T) {
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
	fromAddr := pldtypes.RandAddress()
	localID := uint64(1)
	pubTxMgr.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]*pldapi.PublicTx{
		{From: *fromAddr, LocalID: &localID},
	}, nil)
	transportMgr := componentsmocks.NewTransportManager(t)
	transportMgr.On("LocalNodeName").Return("node1")
	transportMgr.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(msgs []*pldapi.ReliableMessage) bool {
		if len(msgs) != 1 {
			return false
		}
		return msgs[0].Node == "node2" && msgs[0].MessageType == pldapi.RMTSequencingActivity.Enum()
	})).Return(nil)

	sp := NewSyncPoints(ctx, conf, mp.P, txMgr, pubTxMgr, transportMgr).(*syncPoints)
	sp.Start()
	defer sp.Close()

	dCtx := componentsmocks.NewDomainContext(t)
	dCtx.On("Ctx").Return(ctx).Maybe()
	dCtxID := uuid.New()
	dCtx.On("Info").Return(components.DomainContextInfo{ID: dCtxID}).Maybe()
	dCtx.On("Flush", mock.Anything).Return(nil).Maybe()

	privateTxID := uuid.New().String()
	dispatchBatch := &DispatchBatch{
		PublicDispatches: []*PublicDispatch{
			{
				PublicTxs: []*components.PublicTxSubmission{
					{
						Bindings: []*components.PaladinTXReference{
							{
								TransactionID:     uuid.MustParse(privateTxID),
								TransactionSender: "identity@node2",
							},
						},
					},
				},
				PrivateTransactionDispatches: []*DispatchPersisted{
					{PrivateTransactionID: privateTxID},
				},
			},
		},
		PrivateDispatches:    []*components.ChainedPrivateTransaction{},
		PreparedTransactions: []*components.PreparedTransactionWithRefs{},
	}

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectExec("INSERT INTO .*dispatches.*").WillReturnResult(sqlmock.NewResult(1, 1))
	mp.Mock.ExpectCommit()

	contractAddr := pldtypes.RandAddress()
	err = sp.PersistDispatchBatch(dCtx, *contractAddr, dispatchBatch, []*components.StateDistribution{}, []*components.PreparedTransactionWithRefs{})
	require.NoError(t, err)
	require.NoError(t, mp.Mock.ExpectationsWereMet())
	transportMgr.AssertExpectations(t)
}
