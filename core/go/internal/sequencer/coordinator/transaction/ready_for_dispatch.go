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
)

func (t *Transaction) isNotReady() bool {
	//test against the list of states that we consider to be past the point of ready as there is more chance of us noticing
	// a failing test if we add new states in the future and forget to update this list
	return t.stateMachine.CurrentState != State_Confirmed &&
		t.stateMachine.CurrentState != State_Submitted &&
		t.stateMachine.CurrentState != State_Dispatched &&
		t.stateMachine.CurrentState != State_Ready_For_Dispatch
}

// Function hasDependenciesNotReady checks if the transaction has any dependencies that themselves are not ready for dispatch
func (t *Transaction) hasDependenciesNotReady(ctx context.Context) bool {

	//We already calculated the dependencies when we got assembled and there is no way we could have picked up new
	// dependencies without a re-assemble
	// some of them might have been confirmed and removed from our list to avoid a memory leak so this is not necessarily the complete list of dependencies
	// but it should contain all the ones that are not ready for dispatch

	dependencies := t.dependencies.DependsOn
	if t.pt.PreAssembly != nil && t.pt.PreAssembly.Dependencies != nil && t.pt.PreAssembly.Dependencies.DependsOn != nil {
		dependencies = append(dependencies, t.pt.PreAssembly.Dependencies.DependsOn...)
	}

	for _, dependencyID := range dependencies {
		dependency := t.grapher.TransactionByID(ctx, dependencyID)
		if dependency == nil {
			//assume the dependency has been confirmed and no longer in memory
			//hasUnknownDependencies guard will be used to explicitly ensure the correct thing happens
			continue
		}

		if dependency.isNotReady() {
			return true
		}
	}

	return false
}

func (t *Transaction) traceDispatch(ctx context.Context) {
	// Log transaction signatures
	for _, signature := range t.pt.PostAssembly.Signatures {
		log.L(ctx).Tracef("Transaction %s has signature %+v", t.pt.ID.String(), signature)
	}

	// Log transaction endorsements
	for _, endorsement := range t.pt.PostAssembly.Endorsements {
		log.L(ctx).Tracef("Transaction %s has endorsement %+v", t.pt.ID.String(), endorsement)
	}
}

func (t *Transaction) notifyDependentsOfReadiness(ctx context.Context) error {

	if log.IsTraceEnabled() {
		t.traceDispatch(ctx)
	}

	//this function is called when the transaction enters the ready for dispatch state
	// and we have a duty to inform all the transactions that are dependent on us that we are ready in case they are otherwise ready and are blocked waiting for us
	for _, dependentId := range t.dependencies.PrereqOf {
		dependent := t.grapher.TransactionByID(ctx, dependentId)
		if dependent == nil {
			msg := fmt.Sprintf("notifyDependentsOfReadiness: Dependent transaction %s not found in memory", dependentId)
			log.L(ctx).Error(msg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
		err := dependent.HandleEvent(ctx, &DependencyReadyEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: dependent.pt.ID,
			},
			DependencyID: t.pt.ID,
		})
		if err != nil {
			log.L(ctx).Errorf("error notifying dependent transaction %s of readiness of transaction %s: %s", dependent.pt.ID, t.pt.ID, err)
			return err
		}
	}
	return nil
}

func action_NotifyDependentsOfReadiness(ctx context.Context, txn *Transaction) error {
	return txn.notifyDependentsOfReadiness(ctx)
}

// Function HasDependenciesNotIn checks if the transaction has any that are not in the provided ignoreList array.
func (t *Transaction) hasDependenciesNotIn(ctx context.Context, ignoreList []*Transaction) bool {

	var ignore = func(t *Transaction) bool {
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
