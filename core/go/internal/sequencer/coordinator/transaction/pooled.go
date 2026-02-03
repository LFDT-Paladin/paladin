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
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

func (t *Transaction) SetPreviousTransaction(ctx context.Context, previousTransaction *Transaction) {
	//TODO consider moving this to the PreAssembly part of PrivateTransaction and specifying a responsibility of the originator to set this.
	// this is probably part of the decision on whether we expect the originator to include all current inflight transactions in every delegation request.
	t.previousTransaction = previousTransaction
}

func (t *Transaction) SetNextTransaction(ctx context.Context, nextTransaction *Transaction) {
	//TODO consider moving this to the PreAssembly part of PrivateTransaction and specifying a responsibility of the originator to set this.
	// this is probably part of the decision on whether we expect the originator to include all current inflight transactions in every delegation request.
	t.nextTransaction = nextTransaction
}

// Function hasDependenciesNotAssembled checks if the transaction has any dependencies that have not been assembled yet
func (t *Transaction) hasDependenciesNotAssembled(ctx context.Context) bool {

	// we cannot have unassembled dependencies other than those that were provided to us in the PreAssemble or the one we determined as previousTransaction when we initially received an ordered list of delegated transactions.
	if t.previousTransaction != nil && t.previousTransaction.isNotAssembled() {
		return true
	}

	if t.PreAssembly != nil && t.PreAssembly.Dependencies != nil {
		for _, dependencyID := range t.PreAssembly.Dependencies.DependsOn {
			dependency := t.grapher.TransactionByID(ctx, dependencyID)
			if dependency == nil {
				//assume the dependency has been confirmed and no longer in memory
				//hasUnknownDependencies guard will be used to explicitly ensure the correct thing happens
				continue
			}
			if dependency.isNotAssembled() {
				return true
			}
		}
	}

	return false
}

// Function hasUnknownDependencies checks if the transaction has any dependencies the coordinator does not have in memory.  These might be long gone confirmed to base ledger or maybe the delegation request for them hasn't reached us yet. At this point, we don't know
func (t *Transaction) hasUnknownDependencies(ctx context.Context) bool {

	dependencies := t.dependencies.DependsOn
	if t.PreAssembly != nil && t.PreAssembly.Dependencies != nil {
		dependencies = append(dependencies, t.PreAssembly.Dependencies.DependsOn...)
	}

	for _, dependencyID := range dependencies {
		dependency := t.grapher.TransactionByID(ctx, dependencyID)
		if dependency == nil {

			return true
		}

	}

	//if there are are any dependencies declared, they are all known to the current in memory context ( grapher)
	return false
}

// Initializes (or re-initializes) the transaction as it arrives in the pool
func (t *Transaction) initializeForNewAssemply(ctx context.Context) error {
	if t.PreAssembly == nil {
		msg := fmt.Sprintf("cannot calculate dependencies for transaction %s without a PreAssembly", t.ID)
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}

	if t.PreAssembly.Dependencies != nil {
		for _, dependencyID := range t.PreAssembly.Dependencies.DependsOn {
			dependencyTxn := t.grapher.TransactionByID(ctx, dependencyID)

			if nil == dependencyTxn {
				//either the dependency has been confirmed and no longer in memory or there was a overtake on the network and we have not received the delegation request for the dependency yet
				// in either case, the guards will stop this transaction from being assembled but will appear in the heartbeat messages so that the originator can take appropriate action (remove the dependency if it is confirmed, resend the dependency delegation request if it is an inflight transaction)

				//This should be relatively rare so worth logging as an info
				log.L(ctx).Infof("dependency %s not found in memory for transaction %s", dependencyID, t.ID)
				continue
			}

			//TODO this should be idempotent.
			if dependencyTxn.PreAssembly.Dependencies == nil {
				dependencyTxn.PreAssembly.Dependencies = &pldapi.TransactionDependencies{}
			}
			dependencyTxn.PreAssembly.Dependencies.PrereqOf = append(dependencyTxn.PreAssembly.Dependencies.PrereqOf, t.ID)
		}
	}

	// Reset anything that might have been updated during an initial attempt to assembly, endorse and dispatch this TX. This is a no-op if this is the first
	// and only time we pool & assemble this transaction but if we're re-pooling for any reason we must clear the post-assembly and any post-assembly
	// dependencies from a previous version of the grapher.
	t.PostAssembly = nil
	t.dependencies = &pldapi.TransactionDependencies{}
	t.grapher.ForgetMints(t.ID)
	t.cancelAssembleTimeoutSchedules()
	t.resetEndorsementRequests(ctx)

	return nil
}

func (t *Transaction) rePoolDependents(ctx context.Context) error {
	var rePoolError error
	// Raise a DependencyRevertedEvent for every TX that has this one as a pre-req. This will re-pool them
	for _, dependencyID := range t.dependencies.PrereqOf {
		dependencyTxn := t.grapher.TransactionByID(ctx, dependencyID)
		if dependencyTxn != nil {
			err := dependencyTxn.HandleEvent(ctx, &DependencyRevertedEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: dependencyID,
				},
			})
			if err != nil {
				errMsg := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "error notifying dependent transaction of revert", err)
				log.L(ctx).Error(errMsg)
				// Return the first error
				if rePoolError == nil {
					rePoolError = err
				}
			}
		}
	}

	return rePoolError
}

func action_recordRevert(ctx context.Context, txn *Transaction) error {
	err := txn.rePoolDependents(ctx)
	if err != nil {
		// log error but continue
		errMsg := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "error re-pooling dependents of transaction")
		log.L(ctx).Error(errMsg)
	}
	now := pldtypes.TimestampNow()
	txn.revertTime = &now
	return nil
}

func action_initializeDependencies(ctx context.Context, txn *Transaction) error {
	return txn.initializeDependencies(ctx)
}

func guard_HasUnassembledDependencies(ctx context.Context, txn *Transaction) bool {
	return txn.hasDependenciesNotAssembled(ctx)
}

func guard_HasUnknownDependencies(ctx context.Context, txn *Transaction) bool {
	return txn.hasUnknownDependencies(ctx)
}

func guard_HasDependenciesNotReady(ctx context.Context, txn *Transaction) bool {
	return txn.hasDependenciesNotReady(ctx)
}

func guard_HasChainedTxInProgress(ctx context.Context, txn *Transaction) bool {
	return txn.chainedTxAlreadyDispatched
}
