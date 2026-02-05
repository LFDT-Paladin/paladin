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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var transactionFilters = filters.FieldMap{
	"id":             filters.UUIDField("id"),
	"idempotencyKey": filters.StringField("idempotency_key"),
	"submitMode":     filters.StringField("submit_mode"),
	"created":        filters.TimestampField("created"),
	"abiReference":   filters.TimestampField("abi_ref"),
	"functionName":   filters.StringField("fn_name"),
	"domain":         filters.StringField(`"transactions"."domain"`),
	"from":           filters.StringField(`"from"`),
	"to":             filters.HexBytesField(`"to"`),
	"type":           filters.StringField(`"type"`),
}

func (tm *txManager) mapPersistedTXBase(pt *persistedTransaction) *pldapi.Transaction {
	res := &pldapi.Transaction{
		ID:         &pt.ID,
		Created:    pt.Created,
		SubmitMode: pt.SubmitMode,
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: stringOrEmpty(pt.IdempotencyKey),
			Type:           pt.Type,
			Domain:         stringOrEmpty(pt.Domain),
			Function:       stringOrEmpty(pt.Function),
			ABIReference:   pt.ABIReference,
			From:           pt.From,
			To:             pt.To,
			Data:           pt.Data,
		},
	}
	return res
}

func (tm *txManager) mapPersistedTXFull(pt *persistedTransaction) *pldapi.TransactionFull {
	res := &pldapi.TransactionFull{
		Transaction: tm.mapPersistedTXBase(pt),
	}
	receipt := pt.TransactionReceipt
	if receipt != nil {
		res.Receipt = mapPersistedReceipt(receipt)
	}
	for _, dep := range pt.TransactionDeps {
		res.DependsOn = append(res.DependsOn, dep.DependsOn)
	}
	return res
}

func (tm *txManager) mapPersistedTXHistory(pth *persistedTransactionHistory) *pldapi.TransactionHistory {
	return &pldapi.TransactionHistory{
		Created: pth.Created,
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: stringOrEmpty(pth.IdempotencyKey),
			Type:           pth.Type,
			Domain:         stringOrEmpty(pth.Domain),
			Function:       stringOrEmpty(pth.Function),
			ABIReference:   pth.ABIReference,
			From:           pth.From,
			To:             pth.To,
			Data:           pth.Data,
			PublicTxOptions: pldapi.PublicTxOptions{
				Value: pth.Value,
				Gas:   pth.Gas,
				PublicTxGasPricing: pldapi.PublicTxGasPricing{
					MaxFeePerGas:         pth.MaxFeePerGas,
					MaxPriorityFeePerGas: pth.MaxPriorityFeePerGas,
				},
			},
		},
	}
}

func (tm *txManager) mapPersistedTXSequencingActivity(psa *sequencer.DBSequencingActivity) *pldapi.SequencerActivity {
	return &pldapi.SequencerActivity{
		LocalID:        psa.LocalID,
		SubjectID:      psa.SubjectID,
		Timestamp:      psa.Timestamp,
		ActivityType:   psa.ActivityType,
		SequencingNode: psa.SequencingNode,
		TransactionID:  psa.TransactionID,
	}
}

func (tm *txManager) mapPersistedTXDispatch(pd *syncpoints.DispatchPersisted) *pldapi.Dispatch {
	return &pldapi.Dispatch{
		ID:                       pd.ID,
		PrivateTransactionID:     pd.PrivateTransactionID,
		PublicTransactionAddress: pd.PublicTransactionAddress,
		PublicTransactionID:      pd.PublicTransactionID,
	}
}

func (tm *txManager) mapPersistedChainedTransaction(pd *persistedChainedPrivateTxn) *pldapi.ChainedTransaction {
	return &pldapi.ChainedTransaction{
		ChainedTransactionID: pd.ChainedTransaction.String(),
		TransactionID:        pd.Transaction.String(),
		LocalID:              pd.ID.String(),
	}
}

func (tm *txManager) mapPersistedTXResolved(pt *persistedTransaction) *components.ResolvedTransaction {
	res := &components.ResolvedTransaction{
		Transaction: tm.mapPersistedTXBase(pt),
	}
	for _, dep := range pt.TransactionDeps {
		res.DependsOn = append(res.DependsOn, dep.DependsOn)
	}
	return res
}

func (tm *txManager) QueryTransactions(ctx context.Context, jq *query.QueryJSON, dbTX persistence.DBTX, pending bool) ([]*pldapi.Transaction, error) {
	ctx = log.WithComponent(ctx, "txmanager")
	qw := &filters.QueryWrapper[persistedTransaction, pldapi.Transaction]{
		P:           tm.p,
		Table:       "transactions",
		DefaultSort: "-created",
		Filters:     transactionFilters,
		Query:       jq,
		Finalize: func(q *gorm.DB) *gorm.DB {
			if pending {
				q = q.Joins("TransactionReceipt").
					Where(`"TransactionReceipt"."transaction" IS NULL`)
			}
			return q
		},
		MapResult: func(pt *persistedTransaction) (*pldapi.Transaction, error) {
			return tm.mapPersistedTXBase(pt), nil
		},
	}
	return qw.Run(ctx, dbTX)
}

func (tm *txManager) QueryTransactionsFull(ctx context.Context, jq *query.QueryJSON, dbTX persistence.DBTX, pending bool) (results []*pldapi.TransactionFull, err error) {
	return tm.QueryTransactionsFullTx(ctx, jq, dbTX, pending)
}

func (tm *txManager) QueryTransactionsResolved(ctx context.Context, jq *query.QueryJSON, dbTX persistence.DBTX, pending bool) ([]*components.ResolvedTransaction, error) {
	ctx = log.WithComponent(ctx, "txmanager")
	qw := &filters.QueryWrapper[persistedTransaction, components.ResolvedTransaction]{
		P:           tm.p,
		Table:       "transactions",
		DefaultSort: "-created",
		Filters:     transactionFilters,
		Query:       jq,
		Finalize: func(q *gorm.DB) *gorm.DB {
			q = q.
				Preload("TransactionDeps").
				Joins("TransactionReceipt")
			if pending {
				q = q.Joins("TransactionReceipt").
					Where(`"TransactionReceipt"."transaction" IS NULL`)
			}
			return q
		},
		MapResult: func(pt *persistedTransaction) (*components.ResolvedTransaction, error) {
			return tm.mapPersistedTXResolved(pt), nil
		},
	}
	ptxs, err := qw.Run(ctx, dbTX)
	if err != nil {
		return nil, err
	}
	return tm.resolveABIReferencesAndCache(ctx, dbTX, ptxs)
}

func (tm *txManager) QueryTransactionsFullTx(ctx context.Context, jq *query.QueryJSON, dbTX persistence.DBTX, pending bool) ([]*pldapi.TransactionFull, error) {
	ctx = log.WithComponent(ctx, "txmanager")
	qw := &filters.QueryWrapper[persistedTransaction, pldapi.TransactionFull]{
		P:           tm.p,
		Table:       "transactions",
		DefaultSort: "-created",
		Filters:     transactionFilters,
		Query:       jq,
		Finalize: func(q *gorm.DB) *gorm.DB {
			q = q.
				Preload("TransactionDeps").
				Joins("TransactionReceipt")

			if pending {
				q = q.Where(`"TransactionReceipt"."transaction" IS NULL`)
			}
			return q
		},
		MapResult: func(pt *persistedTransaction) (*pldapi.TransactionFull, error) {
			return tm.mapPersistedTXFull(pt), nil
		},
	}
	ptxs, err := qw.Run(ctx, dbTX)
	if err != nil {
		return nil, err
	}

	txIDs := make([]uuid.UUID, len(ptxs))
	for i, tx := range ptxs {
		txIDs[i] = *tx.ID
	}

	ptxs, err = tm.AddTransactionHistory(ctx, dbTX, txIDs, ptxs)
	if err != nil {
		return nil, err
	}

	ptxs, err = tm.AddDispatches(ctx, dbTX, txIDs, ptxs)
	if err != nil {
		return nil, err
	}

	ptxs, err = tm.AddChainedTranasctions(ctx, dbTX, txIDs, ptxs)
	if err != nil {
		return nil, err
	}

	ptxs, err = tm.AddSequencerActivity(ctx, dbTX, txIDs, ptxs)
	if err != nil {
		return nil, err
	}

	return tm.mergePublicTransactions(ctx, dbTX, txIDs, ptxs)
}

func (tm *txManager) AddTransactionHistory(ctx context.Context, dbTX persistence.DBTX, txIDs []uuid.UUID, ptxs []*pldapi.TransactionFull) ([]*pldapi.TransactionFull, error) {
	txhs := []*persistedTransactionHistory{}
	err := dbTX.DB().Table("transaction_history").
		WithContext(ctx).
		Order(`"created" DESC`).
		Where(`"tx_id" IN (?)`, txIDs).
		Find(&txhs).
		Error
	if err != nil {
		return nil, err
	}
	// group by txID
	txhMap := make(map[uuid.UUID][]*persistedTransactionHistory, len(txhs))
	for _, txh := range txhs {
		txhMap[txh.TXID] = append(txhMap[txh.TXID], txh)
	}
	// only add history if there are 2 or more entries
	for _, tx := range ptxs {
		if txhs, ok := txhMap[*tx.ID]; ok && len(txhs) > 1 {
			tx.History = make([]*pldapi.TransactionHistory, len(txhs))
			for i, txh := range txhs {
				tx.History[i] = tm.mapPersistedTXHistory(txh)
			}
		}
	}
	return ptxs, nil
}

func (tm *txManager) AddSequencerActivity(ctx context.Context, dbTX persistence.DBTX, txIDs []uuid.UUID, ptxs []*pldapi.TransactionFull) ([]*pldapi.TransactionFull, error) {
	txsas := []*sequencer.DBSequencingActivity{}
	err := dbTX.DB().Table("sequencer_activities").
		WithContext(ctx).
		Order(`"timestamp" DESC`).
		Where(`"transaction_id" IN (?)`, txIDs).
		Find(&txsas).
		Error
	if err != nil {
		return nil, err
	}
	// group by txID
	txsaMap := make(map[uuid.UUID][]*sequencer.DBSequencingActivity, len(txsas))
	for _, txsa := range txsas {
		txsaMap[txsa.TransactionID] = append(txsaMap[txsa.TransactionID], txsa)
	}
	for _, tx := range ptxs {
		if txsas, ok := txsaMap[*tx.ID]; ok {
			tx.SequencerActivity = make([]*pldapi.SequencerActivity, len(txsas))
			for i, txsa := range txsas {
				tx.SequencerActivity[i] = tm.mapPersistedTXSequencingActivity(txsa)
			}
		}
	}
	return ptxs, nil
}

func (tm *txManager) AddDispatches(ctx context.Context, dbTX persistence.DBTX, txIDs []uuid.UUID, ptxs []*pldapi.TransactionFull) ([]*pldapi.TransactionFull, error) {
	txdps := []*syncpoints.DispatchPersisted{}
	err := dbTX.DB().Table("dispatches").
		WithContext(ctx).
		Where(`"private_transaction_id" IN (?)`, txIDs).
		Find(&txdps).
		Error
	if err != nil {
		return nil, err
	}
	// group by txID
	txdpMap := make(map[string][]*syncpoints.DispatchPersisted, len(txdps))
	for _, txdp := range txdps {
		txdpMap[txdp.PrivateTransactionID] = append(txdpMap[txdp.PrivateTransactionID], txdp)
	}
	for _, tx := range ptxs {
		if txdps, ok := txdpMap[tx.ID.String()]; ok {
			tx.Dispatches = make([]*pldapi.Dispatch, len(txdps))
			for i, txdp := range txdps {
				tx.Dispatches[i] = tm.mapPersistedTXDispatch(txdp)
			}
		}
	}
	return ptxs, nil
}

func (tm *txManager) AddChainedTranasctions(ctx context.Context, dbTX persistence.DBTX, txIDs []uuid.UUID, ptxs []*pldapi.TransactionFull) ([]*pldapi.TransactionFull, error) {
	txdps := []*persistedChainedPrivateTxn{}
	err := dbTX.DB().Table("chained_private_txns").
		WithContext(ctx).
		Where(`"transaction" IN (?)`, txIDs).
		Find(&txdps).
		Error
	if err != nil {
		return nil, err
	}
	// group by txID
	txdpMap := make(map[string][]*persistedChainedPrivateTxn, len(txdps))
	for _, txdp := range txdps {
		txdpMap[txdp.Transaction.String()] = append(txdpMap[txdp.Transaction.String()], txdp)
	}
	for _, tx := range ptxs {
		if txdps, ok := txdpMap[tx.ID.String()]; ok {
			tx.ChainedPrivateTransactions = make([]*pldapi.ChainedTransaction, len(txdps))
			for i, txdp := range txdps {
				tx.ChainedPrivateTransactions[i] = tm.mapPersistedChainedTransaction(txdp)
			}
		}
	}
	return ptxs, nil
}

func (tm *txManager) mergePublicTransactions(ctx context.Context, dbTX persistence.DBTX, txIDs []uuid.UUID, txs []*pldapi.TransactionFull) ([]*pldapi.TransactionFull, error) {
	pubTxByTX, err := tm.publicTxMgr.QueryPublicTxForTransactions(ctx, dbTX, txIDs, nil)
	if err != nil {
		return nil, err
	}
	for _, tx := range txs {
		tx.Public = pubTxByTX[*tx.ID]
	}
	return txs, nil
}

func (tm *txManager) resolveABIReferencesAndCache(ctx context.Context, dbTX persistence.DBTX, txs []*components.ResolvedTransaction) (_ []*components.ResolvedTransaction, err error) {
	abis := make(map[pldtypes.Bytes32]*pldapi.StoredABI, len(txs))
	for _, tx := range txs {
		a := abis[*tx.Transaction.ABIReference]
		if a == nil {
			if a, err = tm.getABIByHash(ctx, dbTX, *tx.Transaction.ABIReference); a == nil || err != nil {
				return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrABIReferenceLookupFailed, tx.Transaction.ABIReference)
			}
		}
		resolvedFunction, err := tm.pickFunction(ctx, a, tx.Transaction.Function, tx.Transaction.To)
		if err != nil {
			return nil, err
		}
		tx.Function = resolvedFunction

		// We can cache this transaction for ID lookup at this point
		tm.txCache.Set(*tx.Transaction.ID, tx)
	}
	return txs, nil
}

func (tm *txManager) GetTransactionByIDFull(ctx context.Context, id uuid.UUID) (result *pldapi.TransactionFull, err error) {
	ctx = log.WithComponent(ctx, "txmanager")
	ptxs, err := tm.QueryTransactionsFull(ctx, query.NewQueryBuilder().Limit(1).Equal("id", id).Query(), tm.p.NOTX(), false)
	if len(ptxs) == 0 || err != nil {
		return nil, err
	}
	return ptxs[0], nil
}

func (tm *txManager) getResolvedTransactionByIDWithinTX(ctx context.Context, id uuid.UUID, dbTX persistence.DBTX) (*components.ResolvedTransaction, error) {
	ctx = log.WithComponent(ctx, "txmanager")
	// This is cache optimized, because domains rely on the sender node's transaction store as
	// the only place to read transaction data from for init and assembly.
	// So we maintain a cache to make that lookup efficient.
	rtx, _ := tm.txCache.Get(id)
	if rtx != nil {
		return rtx, nil
	}

	// Do the query - this function also does the caching (so individual TXs get cached from paginated queries)
	rtxs, err := tm.QueryTransactionsResolved(ctx, query.NewQueryBuilder().Limit(1).Equal("id", id).Query(), dbTX, false)
	if len(rtxs) == 0 || err != nil {
		return nil, err
	}
	return rtxs[0], nil
}

func (tm *txManager) GetResolvedTransactionByID(ctx context.Context, id uuid.UUID) (*components.ResolvedTransaction, error) {
	return tm.getResolvedTransactionByIDWithinTX(ctx, id, tm.p.NOTX())
}

func (tm *txManager) GetTransactionByID(ctx context.Context, id uuid.UUID) (*pldapi.Transaction, error) {
	ctx = log.WithComponent(ctx, "txmanager")
	ptxs, err := tm.QueryTransactions(ctx, query.NewQueryBuilder().Limit(1).Equal("id", id).Query(), tm.p.NOTX(), false)
	if len(ptxs) == 0 || err != nil {
		return nil, err
	}
	return ptxs[0], nil
}

func (tm *txManager) GetTransactionByIdempotencyKey(ctx context.Context, idempotencyKey string) (*pldapi.Transaction, error) {
	ptxs, err := tm.QueryTransactions(ctx, query.NewQueryBuilder().Limit(1).Equal("idempotencyKey", idempotencyKey).Query(), tm.p.NOTX(), false)
	if len(ptxs) == 0 || err != nil {
		return nil, err
	}
	return ptxs[0], nil
}

func (tm *txManager) getTransactionDependenciesWithinTX(ctx context.Context, id uuid.UUID, dbTX persistence.DBTX) (*pldapi.TransactionDependencies, error) {
	ctx = log.WithComponent(ctx, "txmanager")
	var persistedDeps []*transactionDep
	err := dbTX.DB().
		WithContext(ctx).
		Table(`transaction_deps`).
		Where(`"transaction" = ?`, id).
		Or("depends_on = ?", id).
		Find(&persistedDeps).
		Error
	if err != nil {
		return nil, err
	}
	res := &pldapi.TransactionDependencies{
		DependsOn: make([]uuid.UUID, 0, len(persistedDeps)),
		PrereqOf:  make([]uuid.UUID, 0, len(persistedDeps)),
	}
	for _, td := range persistedDeps {
		if td.Transaction == id {
			res.DependsOn = append(res.DependsOn, td.DependsOn)
		} else {
			res.PrereqOf = append(res.PrereqOf, td.Transaction)
		}
	}
	return res, nil
}

func (tm *txManager) GetTransactionDependencies(ctx context.Context, id uuid.UUID) (*pldapi.TransactionDependencies, error) {
	return tm.getTransactionDependenciesWithinTX(ctx, id, tm.p.NOTX())
}

func (tm *txManager) queryPublicTransactions(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.PublicTxWithBinding, error) {
	if err := filters.CheckLimitSet(ctx, jq); err != nil {
		return nil, err
	}
	return tm.publicTxMgr.QueryPublicTxWithBindings(ctx, tm.p.NOTX(), jq)
}

func (tm *txManager) GetPublicTransactionByNonce(ctx context.Context, from pldtypes.EthAddress, nonce pldtypes.HexUint64) (*pldapi.PublicTxWithBinding, error) {
	ctx = log.WithComponent(ctx, "txmanager")
	prs, err := tm.publicTxMgr.QueryPublicTxWithBindings(ctx, tm.p.NOTX(),
		query.NewQueryBuilder().Limit(1).
			Equal("from", from).
			Equal("nonce", nonce).
			Query())
	if len(prs) == 0 || err != nil {
		return nil, err
	}
	return prs[0], nil
}

func (tm *txManager) GetPublicTransactionByHash(ctx context.Context, hash pldtypes.Bytes32) (*pldapi.PublicTxWithBinding, error) {
	ctx = log.WithComponent(ctx, "txmanager")
	return tm.publicTxMgr.GetPublicTransactionForHash(ctx, tm.p.NOTX(), hash)
}
