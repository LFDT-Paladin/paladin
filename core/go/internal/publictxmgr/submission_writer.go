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
	"encoding/json"

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
	nodeName         string
}

func newSubmissionWriter(bgCtx context.Context, nodeName string, p persistence.Persistence, conf *pldconf.PublicTxManagerConfig, metrics metrics.PublicTransactionManagerMetrics, sequencerManager components.SequencerManager, rootTxMgr components.TXManager) *submissionWriter {
	sw := &submissionWriter{}
	sw.metrics = metrics
	sw.sequencerManager = sequencerManager
	sw.rootTxMgr = rootTxMgr
	sw.nodeName = nodeName
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

	// Once we have persisted a TX binding with the originating node the sequencer needs to distribute the public submission back to that node.
	// We ask the sequencer to a) update its local state machine and b) send a reliable message to the originator under the same DBTX
	for _, value := range values {
		if value.PrivateTXOriginator != "" && value.Binding != nil {

			publicTXSubmission := &pldapi.PublicTxWithBinding{}
			nonce := pldtypes.HexUint64(*value.Binding.Nonce)
			from := pldtypes.MustEthAddress(value.from)
			publicTX := &pldapi.PublicTx{
				Dispatcher: sw.nodeName,
				From:       *from,
				To:         value.Binding.To,
				Data:       value.Binding.Data,
				Nonce:      &nonce,
				Created:    value.Binding.Created,
				PublicTxOptions: pldapi.PublicTxOptions{
					Gas:   value.Binding.Gas,
					Value: value.Binding.Value,
					PublicTxGasPricing: pldapi.PublicTxGasPricing{
						MaxPriorityFeePerGas: value.Binding.PublicTxOptions.PublicTxGasPricing.MaxPriorityFeePerGas,
						MaxFeePerGas:         value.Binding.PublicTxOptions.PublicTxGasPricing.MaxFeePerGas,
					},
				},
			}
			publicTXSubmission.PublicTx = publicTX
			publicTXSubmission.TransactionHash = &value.TransactionHash
			publicTXSubmission.Transaction = value.PrivateTXID
			publicTXSubmission.TransactionType = value.TransactionType

			var submissionGasPrice pldapi.PublicTxGasPricing
			if value.GasPricing != nil {
				_ = json.Unmarshal(value.GasPricing, &submissionGasPrice)
			}
			publicTX.Submissions = []*pldapi.PublicTxSubmissionData{
				{
					Time:            value.Created,
					TransactionHash: value.TransactionHash,
					PublicTxGasPricing: pldapi.PublicTxGasPricing{
						MaxPriorityFeePerGas: submissionGasPrice.MaxPriorityFeePerGas,
						MaxFeePerGas:         submissionGasPrice.MaxFeePerGas,
					},
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
