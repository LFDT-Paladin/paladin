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

	// This should create state IDs when mapping from output potential states to output states. However, the IDs are lost below.
	err := t.writeStates(ctx)

	if err != nil {
		// Internal error. Only option is to revert the transaction
		revertReason := i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgSequencerInternalError), err)
		seqRevertEvent := &AssembleRevertResponseEvent{
			PostAssembly: &components.TransactionPostAssembly{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   &revertReason,
			},
		}
		seqRevertEvent.RequestID = requestID // Must match what the state machine thinks the current assemble request ID is
		seqRevertEvent.TransactionID = t.pt.ID
		t.queueEventForCoordinator(ctx, seqRevertEvent)
		t.revertTransactionFailedAssembly(ctx, revertReason)
		// Return the original error
		return err
	}

	// Add output states to the grapher for other transactions to use
	err = t.grapher.AddMinter(ctx, postAssembly.OutputStates, t.pt.ID)
	if err != nil {
		return err
	}

	// Add a lock for every output we create
	createLocks, err := t.engineIntegration.MapPotentialStates(ctx, postAssembly.OutputStatesPotential, t.pt)
	if err != nil {
		return err
	}

	// Add a lock for every output we create
	t.grapher.LockMintsOnCreate(ctx, createLocks, postAssembly.OutputStates, t.pt.ID)

	// Add a lock for every read state and spent state to prevent other transactions using them
	t.grapher.LockMintsOnReadAndSpend(ctx, postAssembly.ReadStates, postAssembly.InputStates, t.pt.ID)

	return nil
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
		grapherStatesAndLocks, err := t.grapher.ExportStatesAndLocks(ctx)
		if err != nil {
			log.L(ctx).Errorf("failed to export grapher state locks: %s", err)
			return err
		}

		blockHeight, err := t.engineIntegration.GetBlockHeight(ctx)
		if err != nil {
			log.L(ctx).Errorf("failed to get engine block height: %s", err)
			return err
		}

		return t.transportWriter.SendAssembleRequest(ctx, t.originatorNode, t.pt.ID, idempotencyKey, t.pt.PreAssembly, grapherStatesAndLocks, blockHeight)
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

// We notify a transactions dependents at the point it is selected for assembly. If this is the last unassembled prereq,
// the dependency can move to State_Pooled upon receviing this notification, since the outcome of assembly is irrelevant
// to ensuring that as a minimum the first assembly attempt is performed in order.
//
// For dependency types where the transactions must be assembled in the correct order, regardless of how many resets have
// occured, a dependency reset event will move the dependent transaction back to State_PreAssembly_Blocked if assembly of
// this transaction fails.
func action_NotifyDependentsOfSelection(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.notifyDependentsOfSelection(ctx)
}

func (t *coordinatorTransaction) notifyDependentsOfSelection(ctx context.Context) error {
	// Get transactions this is a pre-req of, including chained dependencies
	dependentIDs := t.dependencyTracker.GetChainedDeps().GetDependents(ctx, t.pt.ID)

	preAssembleDependent, hasPreAssembleDependent := t.dependencyTracker.GetPreassemblyDeps().GetDependent(ctx, t.pt.ID)

	if hasPreAssembleDependent {
		dependentIDs = append(dependentIDs, preAssembleDependent)
	}

	for _, dependentID := range dependentIDs {
		err := t.coordinatorTransactionHandleEvent(ctx, dependentID, &DependencySelectedForAssemblyEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: dependentID,
			},
			SourceTransactionID: t.pt.ID,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *coordinatorTransaction) writeStates(ctx context.Context) error {
	return t.engineIntegration.WriteStatesForTransaction(ctx, t.pt)
}

func validator_MatchesPendingAssembleRequest(ctx context.Context, txn *coordinatorTransaction, event common.Event) (bool, error) {
	switch event := event.(type) {
	case *AssembleSuccessEvent:
		return txn.pendingAssembleRequest != nil && txn.pendingAssembleRequest.IdempotencyKey() == event.RequestID, nil
	case *AssembleRevertResponseEvent:
		return txn.pendingAssembleRequest != nil && txn.pendingAssembleRequest.IdempotencyKey() == event.RequestID, nil
	case *AssembleErrorResponseEvent:
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

func guard_CanRetryErroredAssemble(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.assembleErrorCount <= txn.assembleErrorRetryThreshhold
}

func action_AssembleError(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	t.assembleErrorCount++
	return nil
}

func action_SendAssembleRequest(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.sendAssembleRequest(ctx)
}

func action_NudgeAssembleRequest(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("Nudging assemble request for transaction %s", txn.pt.ID.String())
	return txn.nudgeAssembleRequest(ctx)
}
