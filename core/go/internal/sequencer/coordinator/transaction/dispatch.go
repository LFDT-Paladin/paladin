/*
 * Copyright © 2025 Kaleido, Inc.
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

package transaction

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// action_Dispatch runs the full dispatch flow when handling Event_RequestDispatch in State_Ready_For_Dispatch.
func action_Dispatch(ctx context.Context, t *CoordinatorTransaction, _ common.Event) error {
	return t.dispatch(ctx)
}

// Dispatch runs the full dispatch flow: prepare, build batch, state distributions, nullifiers, persist, chained transactions.
func (t *CoordinatorTransaction) dispatch(ctx context.Context) error {
	if err := t.domainAPI.PrepareTransaction(t.dCtx, t.pt); err != nil {
		log.L(ctx).Errorf("error preparing transaction %s: %s", t.pt.ID, err)
		return err
	}

	dispatchBatch, err := t.buildDispatchBatch(ctx)
	if err != nil {
		return err
	}

	stateDistributionSet, err := common.NewStateDistributionBuilder(t.nodeName, t.pt).Build(ctx)
	if err != nil {
		log.L(ctx).Errorf("error getting state distributions: %s", err)
		return err
	}
	remoteStateDistributions := make([]*components.StateDistribution, 0, len(stateDistributionSet.Remote))
	for _, sd := range stateDistributionSet.Remote {
		log.L(ctx).Debugf("Adding remote state distribution %+v", sd.StateDistribution)
		remoteStateDistributions = append(remoteStateDistributions, &sd.StateDistribution)
	}

	localNullifiers, err := t.buildNullifiers(ctx, stateDistributionSet.Local)
	if err == nil && len(localNullifiers) > 0 {
		err = t.dCtx.UpsertNullifiers(localNullifiers...)
	}
	if err != nil {
		log.L(ctx).Errorf("error building nullifiers: %s", err)
		return err
	}

	log.L(ctx).Debugf("Persisting & deploying batch. %d public transactions, %d private transactions, %d prepared transactions", len(dispatchBatch.PublicDispatches), len(dispatchBatch.PrivateDispatches), len(dispatchBatch.PreparedTransactions))
	if t.dCtx != nil {
		if err := t.syncPoints.PersistDispatchBatch(t.dCtx, t.pt.Address, dispatchBatch, remoteStateDistributions, dispatchBatch.PreparedTransactions); err != nil {
			log.L(ctx).Errorf("error persisting batch: %s", err)
			return err
		}
	}

	if len(dispatchBatch.PrivateDispatches) > 0 && t.newPrivateTransaction != nil {
		txis := make([]*components.ValidatedTransaction, 0, len(dispatchBatch.PrivateDispatches))
		for _, chained := range dispatchBatch.PrivateDispatches {
			txis = append(txis, chained.NewTransaction)
		}
		if err := t.newPrivateTransaction(ctx, txis); err != nil {
			log.L(ctx).Errorf("error handling new transactions: %v", err)
			return err
		}
	}
	return nil
}

// buildDispatchBatch builds the dispatch batch for a transaction which has already been prepared via the domain
func (t *CoordinatorTransaction) buildDispatchBatch(ctx context.Context) (*syncpoints.DispatchBatch, error) {
	hasPublicTransaction := t.pt.PreparedPublicTransaction != nil
	hasPrivateTransaction := t.pt.PreparedPrivateTransaction != nil
	intent := t.pt.PreAssembly.TransactionSpecification.Intent

	if intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPublicTransaction && !hasPrivateTransaction {
		log.L(ctx).Debugf("Result of transaction %s is a public transaction (gas=%d)", t.pt.ID, *t.pt.PreparedPublicTransaction.Gas)
		publicTxSubmission, err := t.buildPublicTxSubmission(ctx)
		if err != nil {
			return nil, err
		}
		return &syncpoints.DispatchBatch{
			PublicDispatches: []*syncpoints.PublicDispatch{{
				PrivateTransactionDispatches: []*syncpoints.DispatchPersisted{
					{PrivateTransactionID: t.pt.ID.String()},
				},
				PublicTxs: []*components.PublicTxSubmission{publicTxSubmission},
			}},
		}, nil
	}

	if intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPrivateTransaction && !hasPublicTransaction {
		log.L(ctx).Debugf("Result of transaction %s is a chained private transaction", t.pt.ID)
		validatedPrivateTx, err := t.txManager.PrepareChainedPrivateTransaction(ctx, t.pt.PreAssembly.TransactionSpecification.From, t.pt.ID, t.pt.Domain, &t.pt.Address, t.pt.PreparedPrivateTransaction, pldapi.SubmitModeAuto)
		if err != nil {
			log.L(ctx).Errorf("error preparing chained transaction %s: %s", t.pt.ID, err)
			return nil, err
		}
		return &syncpoints.DispatchBatch{
			PrivateDispatches: []*components.ChainedPrivateTransaction{validatedPrivateTx},
		}, nil
	}

	if intent == prototk.TransactionSpecification_PREPARE_TRANSACTION && (hasPublicTransaction || hasPrivateTransaction) {
		log.L(ctx).Debugf("Result of transaction %s is a prepared transaction public=%t private=%t", t.pt.ID, hasPublicTransaction, hasPrivateTransaction)
		preparedTransactionWithRefs := t.mapPreparedTransaction()
		return &syncpoints.DispatchBatch{
			PreparedTransactions: []*components.PreparedTransactionWithRefs{preparedTransactionWithRefs},
		}, nil
	}

	err := i18n.NewError(ctx, msgs.MsgSequencerInvalidPrepareOutcome, t.pt.ID, intent, hasPublicTransaction, hasPrivateTransaction)
	log.L(ctx).Errorf("error preparing transaction %s: %s", t.pt.ID, err)
	return nil, err
}

func (t *CoordinatorTransaction) buildPublicTxSubmission(ctx context.Context) (*components.PublicTxSubmission, error) {
	unqualifiedSigner, err := pldtypes.PrivateIdentityLocator(t.pt.Signer).Identity(ctx)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgSequencerInternalError, err)
	}
	resolvedAddr, err := t.keyManager.ResolveEthAddressNewDatabaseTX(ctx, unqualifiedSigner)
	if err != nil {
		log.L(ctx).Errorf("failed to resolve signers for public transactions: %s", err)
		return nil, err
	}
	log.L(ctx).Debugf("DispatchTransactions: creating PublicTxSubmission from %s", t.pt.Signer)
	publicTx := t.pt.PreparedPublicTransaction
	publicTxSubmission := &components.PublicTxSubmission{
		Bindings: []*components.PaladinTXReference{{
			TransactionID:              t.pt.ID,
			TransactionType:            pldapi.TransactionTypePrivate.Enum(),
			TransactionSender:          t.pt.PreAssembly.TransactionSpecification.From,
			TransactionContractAddress: t.pt.Address.String(),
		}},
		PublicTxInput: pldapi.PublicTxInput{
			From:            resolvedAddr,
			To:              &t.pt.Address,
			PublicTxOptions: publicTx.PublicTxOptions,
		},
	}
	data, err := publicTx.ABI[0].EncodeCallDataJSONCtx(ctx, publicTx.Data)
	if err != nil {
		log.L(ctx).Errorf("failed to encode call data for public transaction %s: %s", t.pt.ID, err)
		return nil, err
	}
	publicTxSubmission.Data = pldtypes.HexBytes(data)
	log.L(ctx).Tracef("Validating public transaction %s", t.pt.ID.String())
	if err := t.publicTxManager.ValidateTransactionNOTX(ctx, publicTxSubmission); err != nil {
		log.L(ctx).Errorf("failed to validate public transaction %s: %s", t.pt.ID, err)
		return nil, err
	}
	return publicTxSubmission, nil
}

// mapPreparedTransaction returns prepared transaction refs for distribution
func (t *CoordinatorTransaction) mapPreparedTransaction() *components.PreparedTransactionWithRefs {
	tx := t.pt
	preparedTransaction := &components.PreparedTransactionWithRefs{
		PreparedTransactionBase: &pldapi.PreparedTransactionBase{
			ID:       tx.ID,
			Domain:   tx.Domain,
			To:       &tx.Address,
			Metadata: tx.PreparedMetadata,
		},
	}
	for _, s := range tx.PostAssembly.InputStates {
		preparedTransaction.StateRefs.Spent = append(preparedTransaction.StateRefs.Spent, s.ID)
	}
	for _, s := range tx.PostAssembly.ReadStates {
		preparedTransaction.StateRefs.Read = append(preparedTransaction.StateRefs.Read, s.ID)
	}
	for _, s := range tx.PostAssembly.OutputStates {
		preparedTransaction.StateRefs.Confirmed = append(preparedTransaction.StateRefs.Confirmed, s.ID)
	}
	for _, s := range tx.PostAssembly.InfoStates {
		preparedTransaction.StateRefs.Info = append(preparedTransaction.StateRefs.Info, s.ID)
	}
	if tx.PreparedPublicTransaction != nil {
		preparedTransaction.Transaction = *tx.PreparedPublicTransaction
	} else {
		preparedTransaction.Transaction = *tx.PreparedPrivateTransaction
	}
	return preparedTransaction
}
