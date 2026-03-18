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
