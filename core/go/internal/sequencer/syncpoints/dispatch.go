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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	seqcommon "github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

type dispatchOperation struct {
	transactionID           uuid.UUID
	publicDispatches        []*PublicDispatch
	privateDispatches       []*components.ChainedPrivateTransaction
	localPreparedTxns       []*components.PreparedTransactionWithRefs
	preparedReliableMsgs    []*pldapi.ReliableMessage
	localSequencerActivites []*components.SequencingActivity
}

type DispatchPersisted struct {
	ID                  string `json:"id"`
	TransactionID       string `json:"transactionID"`
	PublicTransactionID uint64 `json:"publicTransactionID"`
}

// A dispatch sequence is a collection of private transactions that are submitted together for a given signing address in order
type PublicDispatch struct {
	PublicTxs                    []*components.PublicTxSubmission
	PrivateTransactionDispatches []*DispatchPersisted
}

// TransactionDispatch is the resolved outcome of dispatching one transaction: the public dispatch
// sequences, any chained private transactions, and any prepared transactions it produced. It is the
// content written for one transaction; the flush writer coalesces many of these into a single DB write.
type TransactionDispatch struct {
	PublicDispatches     []*PublicDispatch
	PrivateDispatches    []*components.ChainedPrivateTransaction
	PreparedTransactions []*components.PreparedTransactionWithRefs
}

// PendingDispatch is one transaction's resolved dispatch awaiting persistence: the content to write plus
// the remote state distributions to send. A DispatchBatch collects these; PersistDispatchBatch commits the
// whole batch in a single DB transaction.
type PendingDispatch struct {
	TransactionID      uuid.UUID
	Dispatch           *TransactionDispatch
	StateDistributions []*components.StateDistribution
}

// DispatchBatch accumulates the pending dispatches for one contract so they commit together in a single DB
// transaction. The domain state writer and contract address are batch-level, not per-dispatch: every
// dispatch in a batch belongs to the same coordinator (one contract), so they share the one state writer
// and the flush-writer WriteKey. Append preserves order, which is what preserves on-chain nonce order.
type DispatchBatch struct {
	DomainStateWriter components.DomainStateWriter
	ContractAddress   pldtypes.EthAddress
	dispatches        []*PendingDispatch
}

// Append adds a pending dispatch to the batch, preserving order.
func (b *DispatchBatch) Append(d *PendingDispatch) {
	b.dispatches = append(b.dispatches, d)
}

// Dispatches returns the pending dispatches in the batch, in Append order.
func (b *DispatchBatch) Dispatches() []*PendingDispatch {
	return b.dispatches
}

// PersistDispatchBatch commits every dispatch in the batch and blocks until it has committed. The whole
// batch is submitted as one flush-writer operation, so it commits atomically in a single DB transaction
// via runBatch -> writeDispatchOperations.
//
// Ordering: dispatches are written in Append order. Every dispatch carries the batch's contract address as
// its flush-writer WriteKey, so the writer routes the batch to a single worker that inserts each public
// transaction row into public_txns in Append order, giving a monotonic auto-increment pub_txn_id. Nonces
// are NOT assigned here; the per-signing-address public-tx orchestrator later assigns gapless sequential
// nonces ORDER BY pub_txn_id. So Append order -> insert order -> pub_txn_id order -> nonce order.
//
// It enqueues and waits on the long-lived s.bgCtx, never the caller's ctx. Dispatch persistence is a point
// of no return: a control-flow cancellation of the caller (e.g. an epoch-boundary dispatch-loop stop
// rotating the coordinator signing key) must not drop the operation before it is queued, nor return a
// context-cancelled error after the batch has already committed and leave transactions wedged in
// Ready_For_Dispatch.
func (s *syncPoints) PersistDispatchBatch(ctx context.Context, batch *DispatchBatch) error {
	dispatchOperations := make([]*dispatchOperation, 0, len(batch.dispatches))
	for _, pd := range batch.dispatches {
		dispatchOperations = append(dispatchOperations, s.buildDispatchOperation(ctx, pd))
	}

	op := s.writer.QueueWithFlush(s.bgCtx, &syncPointOperation{
		domainStateWriter:  batch.DomainStateWriter,
		contractAddress:    batch.ContractAddress,
		dispatchOperations: dispatchOperations,
	})
	_, err := op.WaitFlushed(s.bgCtx)
	return err
}

// buildDispatchOperation turns one pending dispatch into the flush-writer's dispatchOperation, allocating
// dispatch IDs and building the reliable messages (remote prepared transactions, state distributions and
// sequencer activity records) to send alongside the DB write.
func (s *syncPoints) buildDispatchOperation(ctx context.Context, pd *PendingDispatch) *dispatchOperation {
	dispatch := pd.Dispatch

	preparedReliableMsgs := make([]*pldapi.ReliableMessage, 0,
		len(dispatch.PreparedTransactions)+len(pd.StateDistributions))

	var localPreparedTxns []*components.PreparedTransactionWithRefs
	for _, preparedTxnDistribution := range dispatch.PreparedTransactions {
		node, _ := pldtypes.PrivateIdentityLocator(preparedTxnDistribution.Transaction.From).Node(ctx, false)
		if node != s.transportMgr.LocalNodeName() {
			preparedReliableMsgs = append(preparedReliableMsgs, &pldapi.ReliableMessage{
				Node:        node,
				MessageType: pldapi.RMTPreparedTransaction.Enum(),
				Metadata:    pldtypes.JSONString(preparedTxnDistribution),
			})
		} else {
			localPreparedTxns = append(localPreparedTxns, preparedTxnDistribution)
		}
	}

	for _, stateDistribution := range pd.StateDistributions {
		node, _ := pldtypes.PrivateIdentityLocator(stateDistribution.IdentityLocator).Node(ctx, false)
		preparedReliableMsgs = append(preparedReliableMsgs, &pldapi.ReliableMessage{
			Node:        node,
			MessageType: pldapi.RMTState.Enum(),
			Metadata:    pldtypes.JSONString(stateDistribution),
		})
	}

	// Allocate dispatch IDs early so we can distribute sequencer dispatch records with a remote ID that correlates to the dispatch ID
	for _, publicDispatch := range dispatch.PublicDispatches {
		for _, dispatches := range publicDispatch.PrivateTransactionDispatches {
			dispatches.ID = uuid.New().String()
		}
	}

	var localSequencerActivities []*components.SequencingActivity

	// Sequencer activity dispatch records for public transactions
	for _, publicDispatch := range dispatch.PublicDispatches {
		for i, privateTx := range publicDispatch.PrivateTransactionDispatches {
			sequencingProgress := &components.SequencingActivity{
				SubjectID:      privateTx.ID, // This is the dispatch ID (not the TX ID)
				Timestamp:      pldtypes.TimestampNow(),
				ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
				SequencingNode: s.transportMgr.LocalNodeName(), // Us
				TransactionID:  uuid.MustParse(privateTx.TransactionID),
			}

			localNodePersisted := false

			for _, binding := range publicDispatch.PublicTxs[i].Bindings {
				node, _ := pldtypes.PrivateIdentityLocator(binding.TransactionSender).Node(ctx, false)
				if binding.TransactionID.String() != privateTx.TransactionID {
					continue
				}
				if node == s.transportMgr.LocalNodeName() && !localNodePersisted {
					localSequencerActivities = append(localSequencerActivities, sequencingProgress)
					localNodePersisted = true
				}
				if node != s.transportMgr.LocalNodeName() {
					log.L(ctx).Tracef("Sending sequencer dispatch activity for TX %s to node %s", binding.TransactionID.String(), binding.TransactionSender)
					preparedReliableMsgs = append(preparedReliableMsgs, &pldapi.ReliableMessage{
						Node:        node,
						MessageType: pldapi.RMTSequencingActivity.Enum(),
						Metadata:    pldtypes.JSONString(sequencingProgress),
					})
				}
			}
		}
	}

	// Sequencer activity dispatch records for chained private transactions
	for _, privateDispatch := range dispatch.PrivateDispatches {
		privateDispatch.ID = uuid.New() // Allocate a local chained ID early (not the TX ID) to include in sequencer activity records
		sequencingProgress := &components.SequencingActivity{
			SubjectID:      privateDispatch.ID.String(), // This is the dispatch ID (not the TX ID)
			Timestamp:      pldtypes.TimestampNow(),
			ActivityType:   string(pldapi.SequencerActivityType_ChainedDispatch),
			SequencingNode: s.transportMgr.LocalNodeName(), // Us
			TransactionID:  privateDispatch.OriginalTransaction,
		}

		node, _ := pldtypes.PrivateIdentityLocator(privateDispatch.OriginalSenderLocator).Node(ctx, false)
		if node == s.transportMgr.LocalNodeName() {
			localSequencerActivities = append(localSequencerActivities, sequencingProgress)
		} else {
			log.L(ctx).Tracef("Sending sequencer chained-dispatch activity for TX %s to node %s", privateDispatch.OriginalTransaction, privateDispatch.OriginalSenderLocator)
			preparedReliableMsgs = append(preparedReliableMsgs, &pldapi.ReliableMessage{
				Node:        node,
				MessageType: pldapi.RMTSequencingActivity.Enum(),
				Metadata:    pldtypes.JSONString(sequencingProgress),
			})
		}
	}

	return &dispatchOperation{
		transactionID:           pd.TransactionID,
		publicDispatches:        dispatch.PublicDispatches,
		privateDispatches:       dispatch.PrivateDispatches,
		localPreparedTxns:       localPreparedTxns,
		preparedReliableMsgs:    preparedReliableMsgs,
		localSequencerActivites: localSequencerActivities,
	}
}

func (s *syncPoints) PersistDeployTransactionDispatch(ctx context.Context, transactionID uuid.UUID, dispatch *TransactionDispatch) error {

	// Send the write operation with all of the batch sequence operations to the flush worker.
	// Queue and wait on the long-lived s.bgCtx rather than the caller's ctx - deploy dispatch is
	// equally a point of no return, so a caller cancellation must not drop or orphan the persist.
	op := s.writer.Queue(s.bgCtx, &syncPointOperation{
		dispatchOperations: []*dispatchOperation{{
			transactionID:    transactionID,
			publicDispatches: dispatch.PublicDispatches,
		}},
	})

	//wait for the flush to complete
	_, err := op.WaitFlushed(s.bgCtx)
	return err
}

func (s *syncPoints) writeDispatchOperations(ctx context.Context, dbTX persistence.DBTX, dispatchOperations []*dispatchOperation) (err error) {
	log.L(ctx).Debugf("writeDispatchOperations writing %d dispatchOperations", len(dispatchOperations))

	// For each operation in the batch we hand the public transactions to the public transaction manager,
	// which inserts them in the same DB transaction that records the dispatch. The nonce itself is allocated
	// later by that manager's per-signing-address orchestrator, ordered by the pub_txn_id assigned on insert.

	// Build lists of things to insert (we are insert only)
	for _, op := range dispatchOperations {
		opCtx := log.WithLogField(ctx, "txID", op.transactionID.String())
		log.L(opCtx).Tracef("writeDispatchOperations op: %+v", *op)

		//for each batchSequence operation, hand the public transactions to the public transaction manager to
		//insert (nonce allocated later, ordered by pub_txn_id) and persist the dispatch records.
		for _, dispatchSequenceOp := range op.publicDispatches {
			if len(dispatchSequenceOp.PrivateTransactionDispatches) == 0 {
				continue
			}

			// Call the public transaction manager persist to the database under the current transaction
			publicTxns, err := s.pubTxMgr.WriteNewTransactions(opCtx, dbTX, dispatchSequenceOp.PublicTxs)
			if err != nil {
				log.L(opCtx).Errorf("Error submitting public transactions: %s", err)
				return err
			}

			//TODO this results in an `INSERT` for each dispatchSequence
			//Would it be more efficient to pass an array for the whole flush?
			// could get complicated on the public transaction manager side because
			// it needs to allocate a nonce for each dispatch and that is specific to signing key
			for dispatchIndex, dispatch := range dispatchSequenceOp.PrivateTransactionDispatches {

				//fill in the foreign key before persisting in our dispatch table
				dispatch.PublicTransactionID = *publicTxns[dispatchIndex].LocalID
				if dispatch.ID == "" {
					dispatch.ID = uuid.New().String()
				}
				// Dispatch ID populated early before queueing the dispatch operations so sequencer activity records can include them
			}

			log.L(opCtx).Debugf("Writing dispatch batch %d", len(dispatchSequenceOp.PrivateTransactionDispatches))

			err = dbTX.DB().
				Table("dispatches").
				Clauses(clause.OnConflict{
					Columns: []clause.Column{
						{Name: "transaction_id"},
						{Name: "public_transaction_id"},
					},
					DoNothing: true, // immutable
				}).
				Create(dispatchSequenceOp.PrivateTransactionDispatches).
				Error

			if err != nil {
				log.L(opCtx).Errorf("Error persisting dispatches: %s", err)
				return err
			}
		}

		if len(op.privateDispatches) > 0 {
			err := s.txMgr.ChainPrivateTransactions(opCtx, dbTX, op.privateDispatches)
			if err != nil {
				log.L(opCtx).Errorf("Error persisting private dispatches: %s", err)
				return err
			}
		}

		if len(op.localPreparedTxns) > 0 {
			log.L(opCtx).Debugf("Writing prepared transactions locally  %d", len(op.localPreparedTxns))

			err := s.txMgr.WritePreparedTransactions(opCtx, dbTX, op.localPreparedTxns)
			if err != nil {
				log.L(opCtx).Errorf("Error persisting prepared transactions: %s", err)
				return err
			}
		}

		if len(op.preparedReliableMsgs) == 0 {
			log.L(opCtx).Debug("No prepared reliable messages to persist")
		} else {

			log.L(opCtx).Debugf("Writing %d reliable messages", len(op.preparedReliableMsgs))
			err := s.transportMgr.SendReliable(opCtx, dbTX, op.preparedReliableMsgs...)
			if err != nil {
				log.L(opCtx).Errorf("Error persisting prepared reliable messages: %s", err)
				return err
			}
		}

		if len(op.localSequencerActivites) > 0 {
			log.L(ctx).Debugf("Persisting %d local sequencer activities", len(op.localSequencerActivites))
			if err := seqcommon.WriteSequencingActivities(ctx, dbTX, op.localSequencerActivites); err != nil {
				log.L(ctx).Errorf("Error persisting local sequencer activities: %s", err)
				return err
			}
		}

	}
	return nil
}
