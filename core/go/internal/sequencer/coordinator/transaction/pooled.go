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
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/google/uuid"
)

func (t *coordinatorTransaction) hasDependenciesNotAssembled(ctx context.Context) bool {
	// PreAssemble.DependsOn can only be set when transactions have arrived in the same delegation request.
	// It is cleared when the dependent transaction is selected for assembly which means there is no way
	// that this can be cleared if a dependency has not yet been assembled.
	if t.dependencies.PreAssemble.DependsOn != nil {
		return true
	}
	return t.hasUnassembledChainedDependencies(ctx)
}

func (t *coordinatorTransaction) hasUnassembledChainedDependencies(_ context.Context) bool {
	return len(t.dependencies.Chained.Unassembled) > 0
}

func action_InitializeForNewAssembly(ctx context.Context, txn *coordinatorTransaction, event common.Event) error {
	return txn.initializeForNewAssembly(ctx)
}

// Initializes (or re-initializes) the transaction as it arrives in the pool
func (t *coordinatorTransaction) initializeForNewAssembly(ctx context.Context) error {
	// Reset anything that might have been updated during an initial attempt to assembly, endorse and dispatch this TX. This is a no-op if this is the first
	// and only time we pool & assemble this transaction but if we're re-pooling for any reason we must clear the post-assembly and any post-assembly
	// dependencies from a previous version of the grapher.
	t.pt.CleanUpPostAssemblyData()
	t.chainedChildStore.ForgetChainedChild(t.pt.ID)
	// Clear post-assembly dependencies. Chained dependencies are tracked separately and persist.
	t.dependencies.PostAssemble.DependsOn = nil
	t.dependencies.PostAssemble.PrereqOf = nil
	t.pendingPreDispatchRequest = nil
	t.grapher.Forget(t.pt.ID)
	t.clearTimeoutSchedules()
	t.resetEndorsementRequests(ctx)

	return nil
}

func action_ResetTransactionLocks(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("resetting transaction locks for %s", txn.pt.ID.String())
	// Clear minted-state index immediately when resetting in-memory transaction state to avoid
	// later assembles binding to stale minters that have already been reset/reverted.
	txn.grapher.Forget(txn.pt.ID)
	return nil
}

func guard_HasUnassembledDependencies(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.hasDependenciesNotAssembled(ctx)
}

func action_MarkChainedDependencyAssembled(ctx context.Context, txn *coordinatorTransaction, event common.Event) error {
	e := event.(*DependencySelectedForAssemblyEvent)
	log.L(ctx).Debugf("marking chained dependency %s as assembled for TX %s", e.SourceTransactionID, txn.pt.ID)
	delete(txn.dependencies.Chained.Unassembled, e.SourceTransactionID)
	return nil
}

func validator_IsChainedDependency(_ context.Context, txn *coordinatorTransaction, event common.Event) (bool, error) {
	var sourceID uuid.UUID
	switch e := event.(type) {
	case *DependencySelectedForAssemblyEvent:
		sourceID = e.SourceTransactionID
	case *DependencyResetEvent:
		sourceID = e.SourceTransactionID
	case *DependencyConfirmedRevertedEvent:
		sourceID = e.SourceTransactionID
	default:
		return false, nil
	}
	for _, depID := range txn.dependencies.Chained.DependsOn {
		if depID == sourceID {
			return true, nil
		}
	}
	return false, nil
}

func action_MarkChainedDependencyUnassembled(ctx context.Context, txn *coordinatorTransaction, event common.Event) error {
	var sourceID uuid.UUID
	switch e := event.(type) {
	case *DependencyResetEvent:
		sourceID = e.SourceTransactionID
	case *DependencyConfirmedRevertedEvent:
		sourceID = e.SourceTransactionID
	default:
		return nil
	}
	log.L(ctx).Debugf("marking chained dependency %s as unassembled for TX %s", sourceID, txn.pt.ID)
	txn.dependencies.Chained.Unassembled[sourceID] = struct{}{}
	return nil
}

func action_NotifyDependentsOfReset(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	// We emit a DependencyResetEvent for chained and post assembly dependencies whenever we transition to
	// State_Pooled or State_PreAssembly_Blocked.
	// For the initial transition from State_Initial and the transition from State_Assembling to State_Pooled
	// the only dependents we expect are chained dependencies
	if err := txn.notifyDependentsOfReset(ctx); err != nil {
		return err
	}
	// Once dependents have been notified of reset, clear tracked dependencies so repeated reset
	// events while dispatched are no-ops and stale dependency links are dropped.
	// MRW TODO - forget dependencies for grapher 2
	txn.dependencies = TransactionDependencies{}

	txn.grapher.Forget(txn.pt.ID)
	return nil
}

func (t *coordinatorTransaction) notifyDependentsOfReset(ctx context.Context) error {
	// MRW TODO - also get grapher 2 dependents
	for _, dependentID := range t.grapher.GetDependents(ctx, t.pt.ID) {
		dependentTxn := t.getCoordinatorTransaction(ctx, dependentID)
		if dependentTxn != nil {
			err := dependentTxn.HandleEvent(ctx, &DependencyResetEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: dependentID,
				},
				SourceTransactionID: t.pt.ID,
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

func (t *coordinatorTransaction) removeFromDependencyPrereqOf(ctx context.Context) {
	for _, depID := range t.dependencies.PostAssemble.DependsOn {
		dep, ok := t.getCoordinatorTransaction(ctx, depID).(*coordinatorTransaction)
		if !ok || dep == nil {
			continue
		}
		prereqOf := dep.dependencies.PostAssemble.PrereqOf
		for i, id := range prereqOf {
			if id == t.pt.ID {
				dep.dependencies.PostAssemble.PrereqOf = append(prereqOf[:i], prereqOf[i+1:]...)
				break
			}
		}
	}
}

// guard_HasRevertedChainedDependency returns true if any chained dependency is in State_Reverted.
// Used on Event_Delegated to short-circuit directly to State_Reverted when a dependency has already
// failed by the time this transaction is created.
func guard_HasRevertedChainedDependency(ctx context.Context, txn *coordinatorTransaction) bool {
	for _, depID := range txn.dependencies.Chained.DependsOn {
		dep := txn.getCoordinatorTransaction(ctx, depID)
		if dep != nil && dep.GetCurrentState() == State_Reverted {
			return true
		}
	}
	return false
}

// guard_HasEvictedChainedDependency returns true if any chained dependency is in State_Evicted.
// Used on Event_Delegated to short-circuit directly to State_Evicted when a dependency has already
// been evicted by the time this transaction is created.
func guard_HasEvictedChainedDependency(ctx context.Context, txn *coordinatorTransaction) bool {
	for _, depID := range txn.dependencies.Chained.DependsOn {
		dep := txn.getCoordinatorTransaction(ctx, depID)
		if dep != nil && dep.GetCurrentState() == State_Evicted {
			return true
		}
	}
	return false
}

// action_FinalizeOnRevertedChainedDependencyAtCreation scans the chained dependencies to find the
// reverted one and queues a finalization with the appropriate failure message. This handles the race
// where a chained dependency has already reverted by the time this transaction is delegated.
func action_FinalizeOnRevertedChainedDependencyAtCreation(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	for _, depID := range t.dependencies.Chained.DependsOn {
		dep := t.getCoordinatorTransaction(ctx, depID)
		if dep != nil && dep.GetCurrentState() == State_Reverted {
			log.L(ctx).Infof("finalizing TX %s at creation due to chained dependency %s already reverted", t.pt.ID, depID)
			t.syncPoints.QueueTransactionFinalize(ctx,
				&syncpoints.TransactionFinalizeRequest{
					Domain:          t.pt.Domain,
					ContractAddress: t.pt.Address,
					Originator:      t.originator,
					TransactionID:   t.pt.ID,
					FailureMessage:  i18n.NewError(ctx, msgs.MsgTxMgrDependencyFailed, depID).Error(),
				},
				func(ctx context.Context) {
					log.L(ctx).Debugf("finalized TX %s due to chained dependency failure at creation", t.pt.ID)
				},
				func(ctx context.Context, err error) {
					log.L(ctx).Errorf("error finalizing TX %s due to chained dependency failure at creation: %s", t.pt.ID, err)
				},
			)
			return nil
		}
	}
	return nil
}

func validator_IsPreAssembleDependency(_ context.Context, txn *coordinatorTransaction, event common.Event) (bool, error) {
	e := event.(*DependencySelectedForAssemblyEvent)
	return txn.dependencies.PreAssemble.DependsOn != nil && *txn.dependencies.PreAssemble.DependsOn == e.SourceTransactionID, nil
}

func action_RemovePreAssembleDependency(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	txn.dependencies.PreAssemble.DependsOn = nil
	return nil
}

func action_AddPreAssemblePrereqOf(ctx context.Context, txn *coordinatorTransaction, event common.Event) error {
	e := event.(*NewPreAssembleDependencyEvent)
	txn.dependencies.PreAssemble.PrereqOf = &e.PrereqTransactionID
	return nil
}

func action_RemovePreAssemblePrereqOf(_ context.Context, txn *coordinatorTransaction, _ common.Event) error {
	txn.dependencies.PreAssemble.PrereqOf = nil
	return nil
}
