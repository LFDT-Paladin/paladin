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
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
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

func (t *coordinatorTransaction) applyPostAssembly(ctx context.Context, assemblyResponse *prototk.TransactionPostAssembly, requestID uuid.UUID) error {
	applyStart := t.clock.Now()
	defer func() { t.metrics.ObserveAssembleResponseApply(t.clock.Now().Sub(applyStart)) }()

	t.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse:      assemblyResponse,
		CollectedEndorsements: append([]*prototk.AttestationResult{}, assemblyResponse.GetEndorsements()...),
	}

	t.clearTimeoutSchedules()

	if assemblyResponse.GetAssemblyResult() == prototk.AssembleTransactionResponse_REVERT {
		t.revertTransactionFailedAssembly(ctx, i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgSequencerAssembleRevert), assemblyResponse.GetRevertReason()))
		return nil
	}
	if assemblyResponse.GetAssemblyResult() == prototk.AssembleTransactionResponse_PARK {
		log.L(ctx).Debugf("assembly resulted in transaction %s parked", t.pt.ID.String())
		return nil
	}

	err := t.engineIntegration.ResolveStatesForTransaction(ctx, t.pt)

	if err != nil {
		// Internal error. Only option is to revert the transaction
		revertReason := i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgSequencerInternalError), err)
		seqRevertEvent := &AssembleRevertEvent{
			PostAssembly: &prototk.TransactionPostAssembly{
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

	pa := t.pt.PostAssembly

	// Add output states to the grapher for other transactions to use
	err = t.grapher.AddMinter(ctx, pa.OutputStates, t.pt.ID)
	if err != nil {
		return err
	}

	// Record private state visibility after AddMinter succeeds.
	t.recordOutputStateVisibility(ctx)

	// Add a lock for every output we create.
	t.grapher.LockMintsOnCreate(ctx, pa.OutputStates, t.pt.ID)

	// Add a lock for every read state and spent state to prevent other transactions using them.
	t.grapher.LockMintsOnReadAndSpend(ctx, pa.AssembleResponse.GetReadStates(), pa.AssembleResponse.GetInputStates(), t.pt.ID)

	return nil
}

// recordOutputStateVisibility loads newly minted output-state visibility into the tracker. The three
// index-aligned slices are derived from the transaction's PostAssembly: each output state, its
// proto labels (OutputStatesWithLabels, in OutputStates order), and its distribution
// list from the matching potential output (the authoritative AllowedNodes source).
//
// All three of these sources are guaranteed 1:1 with OutputStates in the output of ResolvePotentialStates.
// This pattern of multiple aligned slices is fragile, but the fragility is kept to the single
// function in the state visibility tracker and this one call site. The alternative involves either
// composite types specifically for this function call, which come with the risk of being misused elsewhere,
// or reusing existing types with multiple back and forth conversions between different field types
// (e.g. pldtypes hex to proto string), which drives high CPU through allocs -> GC.
func (t *coordinatorTransaction) recordOutputStateVisibility(ctx context.Context) {
	pa := t.pt.PostAssembly
	outputs := pa.OutputStates
	potentials := pa.AssembleResponse.GetOutputStatesPotential()
	labels := make([]*prototk.StateLabels, len(outputs))
	distributionLists := make([][]string, len(outputs))
	for i := range outputs {
		labels[i] = pa.OutputStatesWithLabels[i].ProtoLabels()
		distributionLists[i] = potentials[i].GetDistributionList()
	}
	t.stateVisibilityTracker.RecordAssemblyOutput(ctx, outputs, labels, distributionLists)
}

func (t *coordinatorTransaction) sendAssembleRequest(ctx context.Context) error {
	// assemble requests have a short and long timeout
	// the short timeout is for toleration of unreliable networks whereby the action is to retry the request with the same idempotency key
	// the long timeout is to prevent an unavailable transaction originator/assemble from holding up the entire contract / privacy group given that the assemble step is single threaded
	// the action for the long timeout is to return the transaction to the mempool and let another transaction be selected

	// When we first send the request, we start a ticker to emit a requestTimeout event for each tick
	// and nudge the request every requestTimeout event to implement the short retry.
	// The state machine will deal with the longer state timeout via timeout guards.
	t.pendingAssembleRequest = common.NewIdempotentRequestWithKey(ctx, t.clock, t.requestTimeout, t.assembleRequestID, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return t.transportWriter.SendAssembleRequest(ctx, t.originatorNode, &engineProto.AssembleRequest{
			TransactionId:          t.pt.ID.String(),
			AssembleRequestId:      idempotencyKey.String(),
			ContractAddress:        t.pt.Address.HexString(),
			CoordinatorBlockHeight: t.getBlockHeight(),
			ExpiryTimeUnixMs:       t.clock.Now().Add(t.stateTimeout).UnixMilli(),
			BlockHeightTolerance:   int64(t.blockHeightTolerance),
		})
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

func validator_MatchesPendingAssembleRequest(ctx context.Context, txn *coordinatorTransaction, event common.Event) (bool, error) {
	if txn.pendingAssembleRequest == nil {
		return false, nil
	}
	var requestID uuid.UUID
	switch event := event.(type) {
	case *AssembleSuccessEvent:
		requestID = event.RequestID
	case *AssembleRevertEvent:
		requestID = event.RequestID
	case *AssembleErrorEvent:
		requestID = event.RequestID
	case *SignedEvent:
		requestID = event.RequestID
	case *SignErrorEvent:
		requestID = event.RequestID
	}
	return txn.pendingAssembleRequest.IdempotencyKey() == requestID, nil
}

// validator_SignedCarriesAssembly gates the State_Assembling Event_Signed fast-forward: the SignResponse
// must carry the piggybacked assembly to advance assembly. A payload-less SignResponse (e.g. from an older
// peer) is not matched, and is left for the separate AssembleSuccess to advance the transaction.
func validator_SignedCarriesAssembly(_ context.Context, _ *coordinatorTransaction, event common.Event) (bool, error) {
	return event.(*SignedEvent).PostAssembly != nil, nil
}

func action_AssembleSuccess(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*AssembleSuccessEvent)
	return t.applyPostAssembly(ctx, e.PostAssembly, e.RequestID)
}

func action_AssembleRevertResponse(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*AssembleRevertEvent)
	return t.applyPostAssembly(ctx, e.PostAssembly, e.RequestID)
}

func guard_CanRetryErroredAssemble(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.assembleErrorCount <= txn.assembleErrorRetryThreshhold
}

func guard_CanRetryErroredSign(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.signErrorCount <= txn.signErrorRetryThreshhold
}

func action_AssembleError(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	t.assembleErrorCount++
	return nil
}

func action_SendAssembleRequest(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.sendAssembleRequest(ctx)
}

// action_OpenStateView initialises the assembleRequestID for this attempt and captures the state view
// served to the originator: the states currently available to it plus the IDs of the states already
// spend-locked. Every state view request for this assemble is answered from that captured view, so the
// originator's view cannot shift while the assemble is in flight.
func action_OpenStateView(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	txn.assembleRequestID = uuid.New()
	txn.stateViewProvider.OpenView(ctx, txn.assembleRequestID.String(), txn.originatorNode)
	return nil
}

func action_CloseStateView(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	txn.stateViewProvider.CloseView(ctx, txn.assembleRequestID.String())
	return nil
}

func action_NudgeAssembleRequest(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("Nudging assemble request for transaction %s", txn.pt.ID.String())
	return txn.nudgeAssembleRequest(ctx)
}

func action_LogAssembleRejection(ctx context.Context, txn *coordinatorTransaction, event common.Event) error {
	e := event.(*AssembleRequestRejectedEvent)
	log.L(ctx).Warnf("assemble request rejected (reason=%s): coordinator block height=%d, assembler block height=%d, tolerance=%d",
		e.RejectionReason, e.CoordinatorBlockHeight, e.AssemblerBlockHeight, txn.blockHeightTolerance)
	return nil
}

func validator_IsAssembleRejection(reasons ...engineProto.RejectionReason) Validator {
	return func(_ context.Context, _ *coordinatorTransaction, event common.Event) (bool, error) {
		reason := event.(*AssembleRequestRejectedEvent).RejectionReason
		for _, r := range reasons {
			if reason == r {
				return true, nil
			}
		}
		return false, nil
	}
}
