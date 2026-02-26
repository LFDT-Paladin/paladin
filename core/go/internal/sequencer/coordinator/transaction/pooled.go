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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// Function hasDependenciesNotAssembled checks if the transaction has any dependencies that have not been assembled yet
func (t *CoordinatorTransaction) hasDependenciesNotAssembled(ctx context.Context) bool {
	if t.pt.PreAssembly != nil && t.pt.PreAssembly.Dependencies != nil {
		for _, dependencyID := range t.pt.PreAssembly.Dependencies.DependsOn {
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
func (t *CoordinatorTransaction) hasUnknownDependencies(ctx context.Context) bool {

	dependencies := t.dependencies.DependsOn
	if t.pt.PreAssembly != nil && t.pt.PreAssembly.Dependencies != nil {
		dependencies = append(dependencies, t.pt.PreAssembly.Dependencies.DependsOn...)
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
func (t *CoordinatorTransaction) initializeForNewAssemply(ctx context.Context) error {
	if t.pt.PreAssembly == nil {
		msg := fmt.Sprintf("cannot calculate dependencies for transaction %s without a PreAssembly", t.pt.ID)
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}

	if t.pt.PreAssembly.Dependencies != nil {
		for _, dependencyID := range t.pt.PreAssembly.Dependencies.DependsOn {
			dependencyTxn := t.grapher.TransactionByID(ctx, dependencyID)

			if nil == dependencyTxn {
				//either the dependency has been confirmed and no longer in memory or there was a overtake on the network and we have not received the delegation request for the dependency yet
				// in either case, the guards will stop this transaction from being assembled but will appear in the heartbeat messages so that the originator can take appropriate action (remove the dependency if it is confirmed, resend the dependency delegation request if it is an inflight transaction)

				//This should be relatively rare so worth logging as an info
				log.L(ctx).Infof("dependency %s not found in memory for transaction %s", dependencyID, t.pt.ID)
				continue
			}

			//TODO this should be idempotent.
			if dependencyTxn.pt.PreAssembly.Dependencies == nil {
				dependencyTxn.pt.PreAssembly.Dependencies = &pldapi.TransactionDependencies{}
			}
			dependencyTxn.pt.PreAssembly.Dependencies.PrereqOf = append(dependencyTxn.pt.PreAssembly.Dependencies.PrereqOf, t.pt.ID)
		}
	}

	// Reset anything that might have been updated during an initial attempt to assembly, endorse and dispatch this TX. This is a no-op if this is the first
	// and only time we pool & assemble this transaction but if we're re-pooling for any reason we must clear the post-assembly and any post-assembly
	// dependencies from a previous version of the grapher.
	t.pt.PostAssembly = nil
	t.dependencies = &pldapi.TransactionDependencies{}
	t.grapher.ForgetMints(t.pt.ID)
	t.clearTimeoutSchedules()
	t.resetEndorsementRequests(ctx)
	t.engineIntegration.ResetTransactions(ctx, t.pt.ID)

	return nil
}

func action_recordRevert(ctx context.Context, txn *CoordinatorTransaction, _ common.Event) error {
	now := pldtypes.TimestampNow()
	txn.revertTime = &now
	return nil
}

func action_onTransitionToPooled(ctx context.Context, txn *CoordinatorTransaction, event common.Event) error {
	// We emit a DependencyRepooledEvent whenever we transition to pooled. For the initial transition
	// from State_Initial to State_Pooled and the transition from State_Assembling to State_Pooled
	// we do not expect any dependents yet, so this is a no-op.
	if err := txn.notifyDependentsOfRepool(ctx); err != nil {
		return err
	}
	return txn.initializeForNewAssemply(ctx)
}

func guard_HasUnassembledDependencies(ctx context.Context, txn *CoordinatorTransaction) bool {
	return txn.hasDependenciesNotAssembled(ctx)
}

func guard_HasUnknownDependencies(ctx context.Context, txn *CoordinatorTransaction) bool {
	return txn.hasUnknownDependencies(ctx)
}

func guard_HasChainedTxInProgress(ctx context.Context, txn *CoordinatorTransaction) bool {
	return txn.chainedTxAlreadyDispatched
}

func (t *CoordinatorTransaction) notifyDependentsOfRepool(ctx context.Context) error {
	for _, dependentID := range t.dependencies.PrereqOf {
		dependentTxn := t.grapher.TransactionByID(ctx, dependentID)
		if dependentTxn != nil {
			err := dependentTxn.HandleEvent(ctx, &DependencyRepooledEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: dependentID,
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error notifying dependent transaction %s of repool of transaction %s: %s", dependentID, t.pt.ID, err)
				return err
			}
		} else {
			// The only condition under which this branch should be reachable is if the dependent has failed on
			// assembly, which is a final state, and has been cleaned up from memory
			log.L(ctx).Warnf("notifyDependentsOfRepool: Dependent transaction %s not found in memory", dependentID)
		}
	}

	return nil
}
