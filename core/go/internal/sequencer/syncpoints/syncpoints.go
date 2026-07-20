// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package syncpoints

import (
	"context"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/flushwriter"

	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PublicTransactionsSubmit func(tx *gorm.DB) (publicTxID []string, err error)

// SyncPoints is the interface for all private transaction manager's integration with persistent resources
// this includes writing to the database tables that private transaction manager owns as well as
// calling syncpoint APIs on other components like the TxManager and PublicTxManager
// All of the persistence here is offloaded to worker threads for performance reasons
type SyncPoints interface {
	Start()

	// PersistDispatchBatch commits every dispatch in the batch atomically in a single DB transaction and
	// blocks until it has committed. The whole batch is one flush-writer operation, so it can never be split
	// across two transactions. The write happens on the flush writer worker (runBatch -> writeDispatchOperations).
	PersistDispatchBatch(ctx context.Context, batch *DispatchBatch) error

	// Deploy is a special case of dispatch, where there are no private states, so no domain state writer is required
	PersistDeployTransactionDispatch(ctx context.Context, transactionID uuid.UUID, dispatch *TransactionDispatch) error

	// QueueTransactionFinalize integrates with TxManager to mark a transaction as finalized.
	// For off-chain failures, req.FailureMessage is set. For on-chain failures, req.OnChain and req.RevertData are set.
	// This is an async operation so it can safely be called from the sequencer event loop thread.
	// The onCommit and onRollback callbacks are called on a separate goroutine when the transaction is committed or rolled back.
	QueueTransactionFinalize(ctx context.Context, req *TransactionFinalizeRequest, onCommit func(context.Context), onRollback func(context.Context, error))

	// This is a recursive callback between syncpoints when flushing receipts, and FinalizeTransactions on txMgr
	WriteOrDistributeReceipts(ctx context.Context, dbTX persistence.DBTX, receipts []*components.ReceiptInputWithOriginator) error

	Close()
}

type syncPoints struct {
	started bool
	// bgCtx is the long-lived context that owns the flush writer. It is only cancelled on
	// coordinator/sequencer shutdown, never for control-flow reasons (e.g. an epoch-boundary
	// dispatch-loop stop). Durable point-of-no-return persists queue and wait on this context
	// so a caller cancellation can neither drop the operation before it is enqueued nor abort
	// the wait after the batch has committed.
	bgCtx        context.Context
	writer       flushwriter.Writer[*syncPointOperation, *noResult]
	txMgr        components.TXManager
	pubTxMgr     components.PublicTxManager
	transportMgr components.TransportManager
}

func NewSyncPoints(ctx context.Context, conf *pldconf.FlushWriterConfig, p persistence.Persistence, txMgr components.TXManager, pubTxMgr components.PublicTxManager, transportMgr components.TransportManager) SyncPoints {
	s := &syncPoints{
		bgCtx:        ctx,
		txMgr:        txMgr,
		pubTxMgr:     pubTxMgr,
		transportMgr: transportMgr,
	}
	s.writer = flushwriter.NewWriter(ctx, s.runBatch, p, conf, &pldconf.SequencerDefaults.Writer)
	return s
}

func (s *syncPoints) Start() {
	if !s.started {
		s.writer.Start()
	}
}

func (s *syncPoints) Close() {
	s.writer.Shutdown()
}
