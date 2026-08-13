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

package transaction

import (
	"context"
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// action_StartPrepare runs on the transition into State_Preparing. It spawns a goroutine that runs the
// whole prepare-and-build sequence under a bounded retry with backoff, so slow or transiently failing
// calls (domain prepare, key resolution, chained transaction preparation) never block the coordinator
// event loop. The goroutine only reads from the private transaction and writes nothing back to it:
// everything it builds is carried on the resulting Event_PrepareSucceeded as the pending dispatch,
// ready to be queued for the dispatch loop. Each spawn is identified by a fresh prepare ID recorded
// on the transaction; a result from a goroutine spawned before the transaction left and re-entered
// State_Preparing carries a stale ID and is dropped.
func action_StartPrepare(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	prepareID := uuid.New()
	t.inFlightPrepareID = prepareID
	pt := t.pt
	revertCount := t.revertCount
	prepareCtx, cancel := context.WithCancel(ctx)
	t.cancelPrepare = cancel
	go func() {
		defer cancel()
		var pendingDispatch *syncpoints.PendingDispatch
		err := t.prepareRetry.Do(prepareCtx, func(_ int) (bool, error) {
			var err error
			pendingDispatch, err = t.prepareAndBuildDispatch(prepareCtx, pt, revertCount)
			return true, err
		})
		if err != nil {
			log.L(ctx).Errorf("prepare failed for transaction %s: %s", pt.ID, err)
			t.queueEventForCoordinator(ctx, &PrepareFailedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: pt.ID},
				PrepareID:            prepareID,
			})
			return
		}
		t.queueEventForCoordinator(ctx, &PrepareSucceededEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: pt.ID},
			PrepareID:            prepareID,
			PendingDispatch:      pendingDispatch,
		})
	}()
	return nil
}

// action_CancelPrepare runs on the transition out of State_Preparing. Cancellation aborts the in-flight
// prepare goroutine's retry backoff so it exits promptly; on the success path the goroutine has already
// finished and this just releases the context.
func action_CancelPrepare(_ context.Context, t *coordinatorTransaction, _ common.Event) error {
	if t.cancelPrepare != nil {
		t.cancelPrepare()
		t.cancelPrepare = nil
	}
	t.inFlightPrepareID = uuid.Nil
	return nil
}

// validator_MatchesInFlightPrepareID drops prepare results from a goroutine spawned before the
// transaction left and re-entered State_Preparing (e.g. a repool raced the in-flight prepare).
func validator_MatchesInFlightPrepareID(_ context.Context, t *coordinatorTransaction, event common.Event) (bool, error) {
	switch e := event.(type) {
	case *PrepareSucceededEvent:
		return e.PrepareID == t.inFlightPrepareID, nil
	case *PrepareFailedEvent:
		return e.PrepareID == t.inFlightPrepareID, nil
	}
	return false, nil
}

// action_QueuePreparedDispatch applies a successful prepare on the transition into
// State_Ready_For_Dispatch: it registers any chained child produced by the build, places the built
// pending dispatch onto the coordinator's dispatch queue, and releases the heavy post-assembly and
// prepared-dispatch payload data. PreAssembly is preserved because it holds the
// TransactionSpecification and RequiredVerifiers needed if the transaction reverts and must be
// re-assembled.
func action_QueuePreparedDispatch(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*PrepareSucceededEvent)
	for _, privateDispatch := range e.PendingDispatch.Dispatch.PrivateDispatches {
		if privateDispatch.NewTransaction != nil &&
			privateDispatch.NewTransaction.Transaction != nil &&
			privateDispatch.NewTransaction.Transaction.ID != nil {
			t.dependencyTracker.GetChainedDeps().SetChainedChild(ctx, t.pt.ID, *privateDispatch.NewTransaction.Transaction.ID)
		}
	}
	t.enqueueForDispatch(ctx, t, e.PendingDispatch)
	t.pt.CleanUpPostAssemblyData()
	return nil
}

// prepareAndBuildDispatch runs off the coordinator event loop. It prepares the transaction via the
// domain, builds the transaction dispatch, resolves state distributions, and builds the nullifier
// records validated against and linked to the states to be written. The private transaction is only
// ever read: the prepare outputs are returned by the domain and everything built from them goes into
// the returned pending dispatch, so nothing is written back to shared transaction state from this
// goroutine.
func (t *coordinatorTransaction) prepareAndBuildDispatch(ctx context.Context, pt *components.PrivateTransaction, revertCount int) (*syncpoints.PendingDispatch, error) {
	// TODO: should this domain query context be populated with a snapshot of the domain's states at the point the transaction
	// finished assembling? Doing this would require storing a grapher snapshot for every transaction.
	// In a previous iteration of this code where state/nullifer writing and domain query context capabilities coexisted
	// in a single domain context, the context was effectively loaded with the entire coordinator ahead of chain view,
	// including transactions assembled after this one. This was arguably too far ahead, as the domain shouldn't be able
	// to query the future when preparing the transaction. At this stage the domain should not really need to be querying
	// states, as the prepare request contains all the states the transaction consumes/spends/creates, hence the decision
	// to not import a snapshot at the time of writing this comment, but it is something to be aware of.
	dqc := t.components.StateManager().NewDomainQueryContext(ctx, t.domainAPI.Domain(), t.domainAPI.Address())
	defer dqc.Close(ctx)
	prep, err := t.domainAPI.PrepareTransaction(ctx, dqc, t.components.Persistence().NOTX(), pt)
	if err != nil {
		log.L(ctx).Errorf("error preparing transaction %s: %s", pt.ID, err)
		return nil, err
	}

	dispatch, err := t.buildTransactionDispatch(ctx, pt, prep, revertCount)
	if err != nil {
		return nil, err
	}

	stateDistributionSet, err := common.NewStateDistributionBuilder(t.nodeName, pt).Build(ctx)
	if err != nil {
		log.L(ctx).Errorf("error getting state distributions: %s", err)
		return nil, err
	}
	remoteStateDistributions := make([]*components.StateDistribution, 0, len(stateDistributionSet.Remote))
	for _, sd := range stateDistributionSet.Remote {
		log.L(ctx).Debugf("Adding remote state distribution %+v", sd.StateDistribution)
		remoteStateDistributions = append(remoteStateDistributions, &sd.StateDistribution)
	}

	nullifiers, err := t.components.SequencerManager().BuildNullifiers(ctx, stateDistributionSet.Local)
	if err != nil {
		log.L(ctx).Errorf("error building nullifiers: %s", err)
		return nil, err
	}

	// The output/info states (resolved at assembly) and their nullifiers are carried on the pending
	// dispatch; they are written to the DB in the same transaction that persists the dispatch.
	return &syncpoints.PendingDispatch{
		TransactionID:      pt.ID,
		Dispatch:           dispatch,
		StateDistributions: remoteStateDistributions,
		StatesToWrite:      append(pt.PostAssembly.OutputStatesWithLabels, pt.PostAssembly.InfoStatesWithLabels...),
		Nullifiers:         nullifiers,
	}, nil
}

// buildTransactionDispatch builds the dispatch from the outputs of preparing a transaction via the
// domain. It runs on the prepare goroutine, reading the private transaction and the prepare result;
// any chained child it produces is registered in the dependency tracker on the event loop when the
// result is applied, not here.
func (t *coordinatorTransaction) buildTransactionDispatch(ctx context.Context, pt *components.PrivateTransaction, prep *components.PrepareTransactionResult, revertCount int) (*syncpoints.TransactionDispatch, error) {
	hasPublicTransaction := prep.PreparedPublicTransaction != nil
	hasPrivateTransaction := prep.PreparedPrivateTransaction != nil
	intent := pt.PreAssembly.TransactionSpecification.Intent

	if intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPublicTransaction && !hasPrivateTransaction {
		log.L(ctx).Debugf("Result of transaction %s is a public transaction (gas=%d)", pt.ID, *prep.PreparedPublicTransaction.Gas)
		publicTxSubmission, err := t.buildPublicTxSubmission(ctx, pt, prep)
		if err != nil {
			return nil, err
		}
		return &syncpoints.TransactionDispatch{
			PublicDispatches: []*syncpoints.PublicDispatch{{
				PrivateTransactionDispatches: []*syncpoints.DispatchPersisted{
					{TransactionID: pt.ID.String()},
				},
				PublicTxs: []*components.PublicTxSubmission{publicTxSubmission},
			}},
		}, nil
	}

	if intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPrivateTransaction && !hasPublicTransaction {
		log.L(ctx).Debugf("Result of transaction %s is a chained private transaction", pt.ID)
		preparedPrivateTransaction := *prep.PreparedPrivateTransaction
		if preparedPrivateTransaction.IdempotencyKey != "" {
			// We can't rely on just the idempotency key from the domain is it will be the same if we retry a private dispatch.
			// The domain needs to have its own way of detecting duplicate transactions beyond the idempotency key in paladin, as
			// a single private transaction with a unqiue idempotency key can still result in multiple base ledger submissions.
			preparedPrivateTransaction.IdempotencyKey = fmt.Sprintf("%s_%d_%d", preparedPrivateTransaction.IdempotencyKey, t.clock.Now().UnixNano(), revertCount)
		}
		validatedPrivateTx, err := t.components.TxManager().PrepareChainedPrivateTransaction(ctx, t.components.Persistence().NOTX(), pt.PreAssembly.TransactionSpecification.From, pt.ID, pt.Domain, &pt.Address, &preparedPrivateTransaction, pldapi.SubmitModeAuto)
		if err != nil {
			log.L(ctx).Errorf("error preparing chained transaction %s: %s", pt.ID, err)
			return nil, err
		}
		if validatedPrivateTx.NewTransaction != nil &&
			validatedPrivateTx.NewTransaction.Transaction != nil &&
			validatedPrivateTx.NewTransaction.Transaction.ID != nil {

			childID := *validatedPrivateTx.NewTransaction.Transaction.ID

			// Propagate ordering knowledge: for each dependency of this transaction,
			// if that dependency also produced a chained child, the new child must depend on it.
			// These go into ChainedDependsOn so the receiving sequencer passes them into the coordinator
			// for in-memory ordering rather than blocking on confirmation. Every dependency reached
			// State_Ready_For_Dispatch before this transaction entered State_Preparing, so any chained
			// child a dependency produced is already registered in the tracker.
			// This assumes that if a domain instance is dispatching private transactions, it is doing so consistently
			// for every transaction, and to the same domain instance. This is a valid assumption at the point of writing,
			// but could change in the future. While we generally don't want to make assumptions about specific domain
			// behaviour in the sequencer, this is a case where accounting for every possible permutation of
			// dispatches would result in unncessarily complex code.
			seen := make(map[uuid.UUID]bool)
			var chainedDeps []uuid.UUID
			for _, depID := range append(t.dependencyTracker.GetPostAssemblyDeps().GetPrerequisites(ctx, pt.ID), t.dependencyTracker.GetChainedDeps().GetPrerequisites(ctx, pt.ID)...) {
				if depChildID, ok := t.dependencyTracker.GetChainedDeps().GetChainedChild(ctx, depID); ok && !seen[depChildID] {
					seen[depChildID] = true
					chainedDeps = append(chainedDeps, depChildID)
				}
			}
			if len(chainedDeps) > 0 {
				validatedPrivateTx.NewTransaction.ChainedDependsOn = append(validatedPrivateTx.NewTransaction.ChainedDependsOn, chainedDeps...)
				log.L(ctx).Debugf("Chained TX %s has %d dependencies from parent grapher: %v", childID, len(chainedDeps), chainedDeps)
			}
		}
		return &syncpoints.TransactionDispatch{
			PrivateDispatches: []*components.ChainedPrivateTransaction{validatedPrivateTx},
		}, nil
	}

	if intent == prototk.TransactionSpecification_PREPARE_TRANSACTION && (hasPublicTransaction || hasPrivateTransaction) {
		log.L(ctx).Debugf("Result of transaction %s is a prepared transaction public=%t private=%t", pt.ID, hasPublicTransaction, hasPrivateTransaction)
		preparedTransactionWithRefs := mapPreparedTransaction(pt, prep)
		return &syncpoints.TransactionDispatch{
			PreparedTransactions: []*components.PreparedTransactionWithRefs{preparedTransactionWithRefs},
		}, nil
	}

	err := i18n.NewError(ctx, msgs.MsgSequencerInvalidPrepareOutcome, pt.ID, intent, hasPublicTransaction, hasPrivateTransaction)
	log.L(ctx).Errorf("error preparing transaction %s: %s", pt.ID, err)
	return nil, err
}

func (t *coordinatorTransaction) buildPublicTxSubmission(ctx context.Context, pt *components.PrivateTransaction, prep *components.PrepareTransactionResult) (*components.PublicTxSubmission, error) {
	unqualifiedSigner, err := pldtypes.PrivateIdentityLocator(prep.Signer).Identity(ctx)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgSequencerInternalError, err)
	}
	resolvedAddr, err := t.components.KeyManager().ResolveEthAddressNewDatabaseTX(ctx, unqualifiedSigner)
	if err != nil {
		log.L(ctx).Errorf("failed to resolve signers for public transactions: %s", err)
		return nil, err
	}
	log.L(ctx).Debugf("DispatchTransactions: creating PublicTxSubmission from %s", prep.Signer)
	publicTx := prep.PreparedPublicTransaction
	publicTxSubmission := &components.PublicTxSubmission{
		Bindings: []*components.PaladinTXReference{{
			TransactionID:              pt.ID,
			TransactionType:            pldapi.TransactionTypePrivate.Enum(),
			TransactionSender:          pt.PreAssembly.TransactionSpecification.From,
			TransactionContractAddress: pt.Address.String(),
		}},
		PublicTxInput: pldapi.PublicTxInput{
			From:            resolvedAddr,
			To:              &pt.Address,
			PublicTxOptions: publicTx.PublicTxOptions,
		},
	}
	data, err := publicTx.ABI[0].EncodeCallDataJSONCtx(ctx, publicTx.Data)
	if err != nil {
		log.L(ctx).Errorf("failed to encode call data for public transaction %s: %s", pt.ID, err)
		return nil, err
	}
	publicTxSubmission.Data = pldtypes.HexBytes(data)
	log.L(ctx).Tracef("Validating public transaction %s", pt.ID.String())
	if err := t.components.PublicTxManager().ValidateTransaction(ctx, t.components.Persistence().NOTX(), publicTxSubmission); err != nil {
		log.L(ctx).Errorf("failed to validate public transaction %s: %s", pt.ID, err)
		return nil, err
	}
	return publicTxSubmission, nil
}

// mapPreparedTransaction returns prepared transaction refs for distribution
func mapPreparedTransaction(tx *components.PrivateTransaction, prep *components.PrepareTransactionResult) *components.PreparedTransactionWithRefs {
	preparedTransaction := &components.PreparedTransactionWithRefs{
		PreparedTransactionBase: &pldapi.PreparedTransactionBase{
			ID:       tx.ID,
			Domain:   tx.Domain,
			To:       &tx.Address,
			Metadata: prep.PreparedMetadata,
		},
	}
	for _, s := range tx.PostAssembly.AssembleResponse.GetInputStates() {
		preparedTransaction.StateRefs.Spent = append(preparedTransaction.StateRefs.Spent, pldtypes.MustParseHexBytes(s.GetId()))
	}
	for _, s := range tx.PostAssembly.AssembleResponse.GetReadStates() {
		preparedTransaction.StateRefs.Read = append(preparedTransaction.StateRefs.Read, pldtypes.MustParseHexBytes(s.GetId()))
	}
	for _, s := range tx.PostAssembly.OutputStates {
		preparedTransaction.StateRefs.Confirmed = append(preparedTransaction.StateRefs.Confirmed, pldtypes.MustParseHexBytes(s.GetId()))
	}
	for _, s := range tx.PostAssembly.InfoStates {
		preparedTransaction.StateRefs.Info = append(preparedTransaction.StateRefs.Info, pldtypes.MustParseHexBytes(s.GetId()))
	}
	if prep.PreparedPublicTransaction != nil {
		preparedTransaction.Transaction = *prep.PreparedPublicTransaction
	} else {
		preparedTransaction.Transaction = *prep.PreparedPrivateTransaction
	}
	return preparedTransaction
}
