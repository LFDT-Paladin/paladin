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

package publictxmgr

import (
	"context"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/flushwriter"
	"github.com/LFDT-Paladin/paladin/core/internal/publictxmgr/metrics"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"

	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"

	"gorm.io/gorm/clause"
)

type noResult struct{}

type submissionWriter struct {
	flushwriter.Writer[*DBPubTxnSubmission, *noResult]
	metrics          metrics.PublicTransactionManagerMetrics
	sequencerManager components.SequencerManager
	rootTxMgr        components.TXManager
}

func newSubmissionWriter(bgCtx context.Context, p persistence.Persistence, conf *pldconf.PublicTxManagerConfig, metrics metrics.PublicTransactionManagerMetrics, sequencerManager components.SequencerManager, rootTxMgr components.TXManager) *submissionWriter {
	sw := &submissionWriter{}
	sw.metrics = metrics
	sw.sequencerManager = sequencerManager
	sw.rootTxMgr = rootTxMgr
	sw.Writer = flushwriter.NewWriter(bgCtx, sw.runBatch, p, &conf.Manager.SubmissionWriter, &pldconf.PublicTxManagerDefaults.Manager.SubmissionWriter)
	return sw
}

func (sw *submissionWriter) runBatch(ctx context.Context, tx persistence.DBTX, values []*DBPubTxnSubmission) ([]flushwriter.Result[*noResult], error) {
	err := tx.DB().
		Table("public_submissions").
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tx_hash"}},
			DoNothing: true, // immutable
		}).
		Create(values).
		Error
	if err != nil {
		return nil, err
	}

	// Once we have persisted a TX binding with the originating node the sequencer needs to distribute the public submission back to that node. We submit a reliable message
	// under the same DBTX as the local persist
	for _, value := range values {

		if value.PrivateTXOriginator != "" {
			publicTXSubmission := &pldapi.PublicTxWithBinding{}
			nonce := pldtypes.HexUint64(*value.Nonce)
			publicTX := &pldapi.PublicTx{
				From:    value.From,
				To:      value.To,
				Data:    value.Data,
				Nonce:   &nonce,
				Created: value.Created,
			}
			publicTXSubmission.PublicTx = publicTX

			// Do we need this?
			// if value.Completed != nil {
			// 	publicTX.CompletedAt = &value.Completed.Created
			// 	publicTX.Success = &value.Completed.Success
			// 	publicTX.RevertData = value.Completed.RevertData
			// }
			publicTX.Submissions = []*pldapi.PublicTxSubmissionData{
				{
					Time:               value.Submissions[0].Created,
					TransactionHash:    value.Submissions[0].TransactionHash,
					PublicTxGasPricing: pldapi.PublicTxGasPricing{},
				},
			}
			err = sw.sequencerManager.HandlePublicTXSubmission(ctx,
				tx,
				value.PrivateTXOriginator,
				value.PrivateTXID,
				publicTXSubmission,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	sw.metrics.IncDBSubmittedTransactionsByN(uint64(len(values)))
	// We don't actually provide any result, so just build an array of nil results
	return make([]flushwriter.Result[*noResult], len(values)), err
}
