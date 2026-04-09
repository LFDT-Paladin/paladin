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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

func action_UpdateSigningIdentity(_ context.Context, txn *coordinatorTransaction, _ common.Event) error {
	txn.updateSigningIdentity()
	return nil
}

func guard_HasSigner(_ context.Context, txn *coordinatorTransaction) bool {
	return txn.pt.Signer != ""
}

// The type of signing identity affects the safety of dispatching transactions in parallel. Every endorsement
// may stipulate a constraint that allows us to assume dispatching transactions in parallel will be safe knowing
// the signing identity nonce will provide ordering guarantees.
func (t *coordinatorTransaction) updateSigningIdentity() {
	if t.pt.PostAssembly != nil && t.submitterSelection == prototk.ContractConfig_SUBMITTER_COORDINATOR {
		for _, endorsement := range t.pt.PostAssembly.Endorsements {
			for _, constraint := range endorsement.Constraints {
				if constraint == prototk.AttestationResult_ENDORSER_MUST_SUBMIT {
					t.pt.Signer = endorsement.Verifier.Lookup
					log.L(context.Background()).Debugf("Setting transaction %s signer %s based on endorsement constraint", t.pt.ID.String(), t.pt.Signer)
					return
				}
			}
		}
	}
}

func (t *coordinatorTransaction) DependentsMustWait() bool {
	// The return value of this function is based on whether it has progress far enough that it is safe for its dependents to be dispatched.
	log.L(context.Background()).Tracef("Checking if TX %s has progressed to dispatch state and unblocks it dependents", t.pt.ID.String())
	// Safe to dispatch as soon as the dependency TX is dispatched
	notReady := t.stateMachine.CurrentState != State_Confirmed &&
		t.stateMachine.CurrentState != State_Dispatched &&
		t.stateMachine.CurrentState != State_Ready_For_Dispatch
	if notReady {
		log.L(context.Background()).Tracef("TX %s not dispatched, dependents remain blocked", t.pt.ID.String())
	}
	return notReady

}

func guard_HasDependenciesNotReady(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.hasDependenciesNotReady(ctx)
}

// Function hasDependenciesNotReady checks if the transaction has any dependencies that themselves are not ready for dispatch
func (t *coordinatorTransaction) hasDependenciesNotReady(ctx context.Context) bool {
	// We already calculated the dependencies when we got assembled and there is no way we could have picked up new dependencies without a re-assemble
	// some of them might have been confirmed and removed from our list to avoid a memory leak so this is not necessarily the complete list of dependencies
	// but it should contain all the ones that are not ready for dispatch

	dependencies := t.grapher.GetDependencies(ctx, t.pt.ID)
	if t.pt.PreAssembly != nil && t.pt.PreAssembly.Dependencies != nil && t.pt.PreAssembly.Dependencies.DependsOn != nil {
		dependencies = append(dependencies, t.pt.PreAssembly.Dependencies.DependsOn...)
	}

	for _, dependencyID := range dependencies {
		dependency := t.getCoordinatorTransaction(ctx, dependencyID)
		if dependency == nil {
			log.L(ctx).Error(i18n.NewError(ctx, msgs.MsgSequencerGrapherDependencyNotFound, dependencyID))
			return true
		}

		if dependency.DependentsMustWait() {
			return true
		}
	}

	return false
}

func (t *coordinatorTransaction) traceDispatch(ctx context.Context) {
	// Log transaction signatures
	for _, signature := range t.pt.PostAssembly.Signatures {
		log.L(ctx).Tracef("Transaction %s has signature %+v", t.pt.ID.String(), signature)
	}

	// Log transaction endorsements
	for _, endorsement := range t.pt.PostAssembly.Endorsements {
		log.L(ctx).Tracef("Transaction %s has endorsement %+v", t.pt.ID.String(), endorsement)
	}
}

func (t *coordinatorTransaction) notifyDependentsOfReadiness(ctx context.Context) error {
	if log.IsTraceEnabled() {
		t.traceDispatch(ctx)
	}

	//this function is called when the transaction enters the ready for dispatch state
	// and we have a duty to inform all the transactions that are dependent on us that we are ready in case they are otherwise ready and are blocked waiting for us
	for _, dependentID := range t.grapher.GetDependants(ctx, t.pt.ID) {
		t.handleEventForCoordinator(ctx, &DependencyReadyEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: dependentID,
			},
		})
	}
	return nil
}

func action_AllocateSigningIdentity(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	// Make sure we have a signer identity allocated if no endorsement constraint has defined one
	if txn.pt.Signer == "" {
		txn.allocateSigningIdentity(ctx)
	}
	return nil
}

func (t *coordinatorTransaction) allocateSigningIdentity(ctx context.Context) {
	// Use the coordinator signing identity unless Paladin config asserts something specific to use
	if t.domainSigningIdentity != "" {
		log.L(ctx).Debugf("Domain has a fixed signing identity for TX %s - using that", t.pt.ID.String())
		t.pt.Signer = t.domainSigningIdentity
		return
	}

	log.L(ctx).Debugf("No fixed or endorsement-specific signing identity for TX %s - allocating a dynamic signing identity", t.pt.ID.String())
	t.pt.Signer = t.coordinatorSigningIdentity
}

func action_NotifyDependentsOfReadiness(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.notifyDependentsOfReadiness(ctx)
}
