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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

func action_UpdateSigningIdentity(_ context.Context, txn *CoordinatorTransaction, _ common.Event) error {
	txn.updateSigningIdentity()
	return nil
}

func guard_HasDynamicSigningIdentity(_ context.Context, txn *CoordinatorTransaction) bool {
	return txn.dynamicSigningIdentity
}

// The type of signing identity affects the safety of dispatching transactions in parallel. Every endorsement
// may stipulate a constraint that allows us to assume dispatching transactions in parallel will be safe knowing
// the signing identity nonce will provide ordering guarantees.
func (t *CoordinatorTransaction) updateSigningIdentity() {
	if t.pt.PostAssembly != nil && t.submitterSelection == prototk.ContractConfig_SUBMITTER_COORDINATOR {
		for _, endorsement := range t.pt.PostAssembly.Endorsements {
			for _, constraint := range endorsement.Constraints {
				if constraint == prototk.AttestationResult_ENDORSER_MUST_SUBMIT {
					t.pt.Signer = endorsement.Verifier.Lookup
					t.dynamicSigningIdentity = false
					log.L(context.Background()).Debugf("Setting transaction %s signer %s based on endorsement constraint", t.pt.ID.String(), t.pt.Signer)
					return
				}
			}
		}
	}
}

func (t *CoordinatorTransaction) isNotReady() bool {
	// test against the list of states that we consider to be past the point of ready as there is more chance of us noticing
	// a failing test if we add new states in the future and forget to update this list

	// "ready" is based on the transaction's dependencies and the signing identity. If the signer is fixed (i.e. the signer of this TX is
	// the same as the signer of the dependency) we can rely on the base ledger nonce ensuring ordering of the on-chain transactions. However,
	// if the signer is dynamic the base ledger TX for this Paladin transaction could be mined ahead of the dependency transaction, resulting
	// in public TX revert & reassembly (with no guarantee of every getting them on to the base ledger in the correct order). In such cases
	// we are forced to wait for the dependency to be confirmed, not just dispatched.

	if !t.dynamicSigningIdentity {
		log.L(context.Background()).Tracef("Checking if TX %s has progressed to dispatch state and unblocks it dependents", t.pt.ID.String())
		// Fixed signing address - safe to dispatch as soon as the dependency TX is dispatched
		notReady := t.stateMachine.CurrentState != State_Confirmed &&
			t.stateMachine.CurrentState != State_Submitted &&
			t.stateMachine.CurrentState != State_Dispatched &&
			t.stateMachine.CurrentState != State_Ready_For_Dispatch
		if notReady {
			log.L(context.Background()).Tracef("TX %s not dispatched, dependents remain blocked", t.pt.ID.String())
		}
		return notReady
	}

	log.L(context.Background()).Tracef("Checking if TX %s has progressed to confirmed state and unblocks it dependents", t.pt.ID.String())
	// Dynamic signing address - we must want for the dependency to be confirmed before we can dispatch
	notReady := t.stateMachine.CurrentState != State_Confirmed
	if notReady {
		log.L(context.Background()).Tracef("TX %s not confirmed, dependents remain blocked", t.pt.ID.String())
	}
	return notReady
}

func guard_HasDependenciesNotReady(ctx context.Context, txn *CoordinatorTransaction) bool {
	return txn.hasDependenciesNotReady(ctx)
}

// Function hasDependenciesNotReady checks if the transaction has any dependencies that themselves are not ready for dispatch
func (t *CoordinatorTransaction) hasDependenciesNotReady(ctx context.Context) bool {
	// We already calculated the dependencies when we got assembled and there is no way we could have picked up new dependencies without a re-assemble
	// some of them might have been confirmed and removed from our list to avoid a memory leak so this is not necessarily the complete list of dependencies
	// but it should contain all the ones that are not ready for dispatch

	dependencies := t.dependencies.DependsOn
	if t.pt.PreAssembly != nil && t.pt.PreAssembly.Dependencies != nil && t.pt.PreAssembly.Dependencies.DependsOn != nil {
		dependencies = append(dependencies, t.pt.PreAssembly.Dependencies.DependsOn...)
	}

	for _, dependencyID := range dependencies {
		dependency := t.grapher.TransactionByID(ctx, dependencyID)
		if dependency == nil {
			log.L(ctx).Warnf("TX %s has a dependency (%s) that's missing from memory", t.pt.ID.String(), dependencyID.String())
			// assume the dependency has been confirmed and is no longer in memory since we can't wait for a non-existent transaction
			// to be in the right state. Rely on TX re-assembly if the base ledger TX fails
			continue
		}

		if dependency.isNotReady() {
			return true
		}
	}

	return false
}

func (t *CoordinatorTransaction) traceDispatch(ctx context.Context) {
	// Log transaction signatures
	for _, signature := range t.pt.PostAssembly.Signatures {
		log.L(ctx).Tracef("Transaction %s has signature %+v", t.pt.ID.String(), signature)
	}

	// Log transaction endorsements
	for _, endorsement := range t.pt.PostAssembly.Endorsements {
		log.L(ctx).Tracef("Transaction %s has endorsement %+v", t.pt.ID.String(), endorsement)
	}
}

func (t *CoordinatorTransaction) notifyDependentsOfReadiness(ctx context.Context) error {
	if log.IsTraceEnabled() {
		t.traceDispatch(ctx)
	}

	//this function is called when the transaction enters the ready for dispatch state
	// and we have a duty to inform all the transactions that are dependent on us that we are ready in case they are otherwise ready and are blocked waiting for us
	for _, dependentId := range t.dependencies.PrereqOf {
		dependent := t.grapher.TransactionByID(ctx, dependentId)
		if dependent == nil {
			log.L(ctx).Warnf("TX %s has a dependency (%s) that's missing from memory", t.pt.ID.String(), dependentId.String())
		} else {
			err := dependent.HandleEvent(ctx, &DependencyReadyEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: dependent.pt.ID,
				},
			})

			if err != nil {
				log.L(ctx).Errorf("error notifying dependent transaction %s of readiness of transaction %s: %s", dependent.pt.ID, t.pt.ID, err)
				return err
			}
		}
	}
	return nil
}

func (t *CoordinatorTransaction) allocateSigningIdentity(ctx context.Context) {

	// Generate a dynamic signing identity unless Paladin config asserts something specific to use
	if t.domainSigningIdentity != "" {
		log.L(ctx).Debugf("Domain has a fixed signing identity for TX %s - using that", t.pt.ID.String())
		t.pt.Signer = t.domainSigningIdentity
		t.dynamicSigningIdentity = false
		return
	}

	log.L(ctx).Debugf("No fixed or endorsement-specific signing identity for TX %s - allocating a dynamic signing identity", t.pt.ID.String())
	t.pt.Signer = fmt.Sprintf("domains.%s.submit.%s", t.pt.Address.String(), uuid.New())
}

func action_NotifyDependentsOfReadiness(ctx context.Context, txn *CoordinatorTransaction, _ common.Event) error {
	// Make sure we have a signer identity allocated if no endorsement constraint has defined one
	if txn.pt.Signer == "" {
		txn.allocateSigningIdentity(ctx)
	}

	return txn.notifyDependentsOfReadiness(ctx)
}

// Function HasDependenciesNotIn checks if the transaction has any that are not in the provided ignoreList array.
func (t *CoordinatorTransaction) hasDependenciesNotIn(ctx context.Context, ignoreList []*CoordinatorTransaction) bool {

	var ignore = func(t *CoordinatorTransaction) bool {
		for _, ignoreTxn := range ignoreList {
			if ignoreTxn.pt.ID == t.pt.ID {
				return true
			}
		}
		return false
	}

	// Dependencies calculated at the time of assembly based on the state(s) being spent
	dependencies := t.dependencies

	//augment with the dependencies explicitly declared in the pre-assembly

	if t.pt.PreAssembly.Dependencies != nil && t.pt.PreAssembly.Dependencies.DependsOn != nil {
		dependencies.DependsOn = append(dependencies.DependsOn, t.pt.PreAssembly.Dependencies.DependsOn...)
	}

	for _, dependencyID := range dependencies.DependsOn {
		dependency := t.grapher.TransactionByID(ctx, dependencyID)
		if dependency == nil {
			//assume the dependency has been confirmed and no longer in memory
			//hasUnknownDependencies guard will be used to explicitly ensure the correct thing happens
			continue
		}

		if !ignore(dependency) {
			return true
		}
	}

	return false
}
