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
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

func (t *coordinatorTransaction) revertTransactionFailedAssembly(ctx context.Context, revertReason string) {
	var tryFinalize func()
	tryFinalize = func() {
		t.syncPoints.QueueTransactionFinalize(ctx, &syncpoints.TransactionFinalizeRequest{
			Domain:          t.pt.Domain,
			ContractAddress: pldtypes.EthAddress{},
			Originator:      t.originator,
			TransactionID:   t.pt.ID,
			FailureMessage:  revertReason,
		},
			func(ctx context.Context) {
				log.L(ctx).Debugf("finalized deployment transaction: %s", t.pt.ID)
			},
			func(ctx context.Context, err error) {
				log.L(ctx).Errorf("error finalizing deployment: %s", err)
				tryFinalize()
			})
	}
	tryFinalize()
}

func (t *coordinatorTransaction) applyPostAssembly(ctx context.Context, postAssembly *components.TransactionPostAssembly, requestID uuid.UUID) error {
	t.pt.PostAssembly = postAssembly

	t.clearTimeoutSchedules()

	if t.pt.PostAssembly.AssemblyResult == prototk.AssembleTransactionResponse_REVERT {
		t.revertTransactionFailedAssembly(ctx, *postAssembly.RevertReason)
		return nil
	}
	if t.pt.PostAssembly.AssemblyResult == prototk.AssembleTransactionResponse_PARK {
		log.L(ctx).Debugf("assembly resulted in transaction %s parked", t.pt.ID.String())
		return nil
	}

	err := t.writeLockStates(ctx)
	if err != nil {
		// Internal error. Only option is to revert the transaction
		seqRevertEvent := &AssembleRevertResponseEvent{}
		seqRevertEvent.RequestID = requestID // Must match what the state machine thinks the current assemble request ID is
		seqRevertEvent.TransactionID = t.pt.ID
		t.queueEventForCoordinator(ctx, seqRevertEvent)
		t.revertTransactionFailedAssembly(ctx, i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgSequencerInternalError), err))
		// Return the original error
		return err
	}

	// Once we've written the lock states we have output states which must be added to the grapher
	for _, state := range postAssembly.OutputStates {
		err := t.grapher.AddMinter(ctx, state.ID, t)
		if err != nil {
			errMsg := i18n.NewError(ctx, msgs.MsgSequencerAddMinterError, t.pt.ID.String(), state.ID.String(), err)
			log.L(ctx).Error(errMsg)
			return errMsg
		}
	}
	return t.calculatePostAssembleDependencies(ctx)
}

func (t *coordinatorTransaction) sendAssembleRequest(ctx context.Context) error {
	// assemble requests have a short and long timeout
	// the short timeout is for toleration of unreliable networks whereby the action is to retry the request with the same idempotency key
	// the long timeout is to prevent an unavailable transaction originator/assemble from holding up the entire contract / privacy group given that the assemble step is single threaded
	// the action for the long timeout is to return the transaction to the mempool and let another transaction be selected

	// When we first send the request, we start a ticker to emit a requestTimeout event for each tick
	// and nudge the request every requestTimeout event to implement the short retry.
	// The state machine will deal with the longer state timeout via timeout guards.
	t.pendingAssembleRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		stateLocks, err := t.engineIntegration.GetStateLocks(ctx)
		if err != nil {
			log.L(ctx).Errorf("failed to get engine state locks: %s", err)
			return err
		}
		blockHeight, err := t.engineIntegration.GetBlockHeight(ctx)
		if err != nil {
			log.L(ctx).Errorf("failed to get engine block height: %s", err)
			return err
		}

		return t.transportWriter.SendAssembleRequest(ctx, t.originatorNode, t.pt.ID, idempotencyKey, t.pt.PreAssembly, stateLocks, blockHeight)
	})

	t.scheduleRequestTimeout(ctx)
	return t.pendingAssembleRequest.Nudge(ctx)
}

func (t *coordinatorTransaction) nudgeAssembleRequest(ctx context.Context) error {
	if t.pendingAssembleRequest == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "nudgeAssembleRequest called with no pending request")
	}
	return t.pendingAssembleRequest.Nudge(ctx)
}

func action_NotifyPreAssembleDependentOfSelection(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.notifyPreAssembleDependentOfSelection(ctx)
}

func (t *coordinatorTransaction) notifyPreAssembleDependentOfSelection(ctx context.Context) error {
	if t.preAssemblePrereqOf == nil {
		return nil
	}
	dependent := t.grapher.TransactionByID(ctx, *t.preAssemblePrereqOf)
	if dependent == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerGrapherDependencyNotFound, *t.preAssemblePrereqOf)
	}
	return dependent.HandleEvent(ctx, &DependencySelectedForAssemblyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: t.pt.ID,
		},
	})
}

func (t *coordinatorTransaction) calculatePostAssembleDependencies(ctx context.Context) error {
	// Dependencies can arise because  we have been assembled to spend states that were produced by other transactions
	// or because there are other transactions from the same originator that have not been dispatched yet or because the user has declared explicit dependencies
	// this function calculates the dependencies relating to states and sets up the reverse association
	// it is assumed that the other dependencies have already been set up when the transaction was first received by the coordinator TODO correct this comment line with more accurate description of when we expect the static dependencies to have been calculated.  Or make it more vague.
	if t.pt.PostAssembly == nil {
		msg := fmt.Sprintf("cannot calculate dependencies for transaction %s without a PostAssembly", t.pt.ID)
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}

	found := make(map[uuid.UUID]bool)
	t.dependencies = &pldapi.TransactionDependencies{
		DependsOn: make([]uuid.UUID, 0, len(t.pt.PostAssembly.InputStates)+len(t.pt.PostAssembly.ReadStates)),
		PrereqOf:  make([]uuid.UUID, 0, len(t.pt.PostAssembly.InputStates)+len(t.pt.PostAssembly.ReadStates)),
	}
	for _, state := range append(t.pt.PostAssembly.InputStates, t.pt.PostAssembly.ReadStates...) {
		dependency, err := t.grapher.LookupMinter(ctx, state.ID)
		if err != nil {
			errMsg := fmt.Sprintf("error looking up dependency for state %s: %s", state.ID, err)
			log.L(ctx).Error(errMsg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, errMsg)
		}
		if dependency == nil {
			log.L(ctx).Infof("no minter found for state %s", state.ID)
			//assume the state was produced by a confirmed transaction
			//TODO should we validate this by checking the domain context? If not, explain why this is safe in the architecture doc
			continue
		}
		if found[dependency.pt.ID] {
			continue
		}
		found[dependency.pt.ID] = true

		t.dependencies.DependsOn = append(t.dependencies.DependsOn, dependency.pt.ID)
		//also set up the reverse association
		dependency.dependencies.PrereqOf = append(dependency.dependencies.PrereqOf, t.pt.ID)
	}
	return nil
}

func (t *coordinatorTransaction) writeLockStates(ctx context.Context) error {
	return t.engineIntegration.WriteLockStatesForTransaction(ctx, t.pt)
}

func validator_MatchesPendingAssembleRequest(ctx context.Context, txn *coordinatorTransaction, event common.Event) (bool, error) {
	switch event := event.(type) {
	// TODO: Snapshot freshness
	// We currently accept an assemble response if the request ID matches the pending request.
	// Add a snapshot version to assemble requests/responses and reject responses
	// that were built from a snapshot older than the transaction's current assembly version.
	// This is intended to guard against stale in-flight assemble responses after repool/unwind.
	case *AssembleSuccessEvent:
		return txn.pendingAssembleRequest != nil && txn.pendingAssembleRequest.IdempotencyKey() == event.RequestID, nil
	case *AssembleRevertResponseEvent:
		return txn.pendingAssembleRequest != nil && txn.pendingAssembleRequest.IdempotencyKey() == event.RequestID, nil
	}
	return false, nil
}

func action_AssembleSuccess(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*AssembleSuccessEvent)
	err := t.applyPostAssembly(ctx, e.PostAssembly, e.RequestID)
	if err == nil {
		// Assembling resolves the required verifiers which will need passing on for the endorse step
		t.pt.PreAssembly.Verifiers = e.PreAssembly.Verifiers
	}
	return err
}

func action_AssembleRevertResponse(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*AssembleRevertResponseEvent)
	return t.applyPostAssembly(ctx, e.PostAssembly, e.RequestID)
}

func action_SendAssembleRequest(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.sendAssembleRequest(ctx)
}

func action_NudgeAssembleRequest(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("Nudging assemble request for transaction %s", txn.pt.ID.String())
	return txn.nudgeAssembleRequest(ctx)
}
