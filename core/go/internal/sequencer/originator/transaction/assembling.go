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
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

func action_AssembleRequestReceived(ctx context.Context, t *originatorTransaction, event common.Event) error {
	e := event.(*AssembleRequestReceivedEvent)
	log.L(ctx).Debugf("Received assemble request from coordinator %s. (Coordinator block height: %d, local block height: %d)",
		e.Coordinator, e.CoordinatorBlockHeight, t.getBlockHeight())
	t.currentDelegate = e.Coordinator
	t.latestAssembleRequest = &assembleRequestFromCoordinator{
		coordinatorsBlockHeight: e.CoordinatorBlockHeight,
		stateSnapshot:           e.StateSnapshot,
		requestID:               e.RequestID,
		expiry:                  e.Expiry,
	}
	return nil
}

func action_AssembleAndSignSuccess(_ context.Context, t *originatorTransaction, event common.Event) error {
	e := event.(*AssembleAndSignSuccessEvent)
	t.pt.PostAssembly = e.PostAssembly
	t.latestFulfilledAssembleRequestID = e.RequestID
	return nil
}

func action_AssembleRevert(ctx context.Context, t *originatorTransaction, event common.Event) error {
	e := event.(*AssembleRevertEvent)
	t.pt.PostAssembly = e.PostAssembly
	t.latestFulfilledAssembleRequestID = e.RequestID
	return nil
}

func action_AssemblePark(ctx context.Context, t *originatorTransaction, event common.Event) error {
	e := event.(*AssembleParkEvent)
	t.pt.PostAssembly = e.PostAssembly
	t.latestFulfilledAssembleRequestID = e.RequestID
	return nil
}

func action_AssembleError(ctx context.Context, t *originatorTransaction, event common.Event) error {
	e := event.(*AssembleErrorEvent)
	t.latestFulfilledAssembleRequestID = e.RequestID
	return nil
}

func action_CancelCurrentAssembly(_ context.Context, txn *originatorTransaction, _ common.Event) error {
	if txn.cancelCurrentAssembly != nil {
		txn.cancelCurrentAssembly()
		txn.cancelCurrentAssembly = nil
	}
	return nil
}

// action_AssembleAndSign spawns a background goroutine to perform the domain-level
// assembly work and queue the result event back to the originator. This keeps the
// transaction event loop unblocked while allowing the potentially slow AssembleAndSign
// call to run concurrently.
//
// If a previous assembly goroutine is still in flight (from a superseded request), its
// context is cancelled before the new goroutine is started.
//
// handleAssembleAndSign does not modify the private transaction or the latest assembly
// request, making it safe to call in a separate goroutine. This is enforced via unit tests
// in the engine integration component.
func action_AssembleAndSign(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	if txn.latestAssembleRequest == nil {
		//This should never happen unless there is a bug in the state machine logic
		log.L(ctx).Errorf("no assemble request found")
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "No assemble request found")
	}

	// Cancel any previously in-flight assembly goroutine for a superseded request
	if txn.cancelCurrentAssembly != nil {
		txn.cancelCurrentAssembly()
	}

	req := *txn.latestAssembleRequest
	preAssembly := txn.pt.PreAssembly
	resolvedVerifiers := txn.pt.ResolvedVerifiers
	txID := txn.pt.ID

	// Always create a cancellable context so a superseding request can abort this goroutine
	assembleCtx, cancel := context.WithCancel(ctx)
	if !req.expiry.IsZero() {
		assembleCtx, cancel = context.WithDeadline(assembleCtx, req.expiry)
	}
	txn.cancelCurrentAssembly = cancel
	txn.currentAssemblyRequestID = req.requestID

	go func() {
		defer cancel()
		txn.handleAssembleAndSign(assembleCtx, txID, req, preAssembly, resolvedVerifiers)
	}()
	return nil
}

func (txn *originatorTransaction) handleAssembleAndSign(ctx context.Context, txID uuid.UUID, req assembleRequestFromCoordinator, preAssembly *prototk.TransactionPreAssembly, resolvedVerifiers []*prototk.ResolvedVerifier) {
	assembleResponse, err := txn.engineIntegration.AssembleAndSign(ctx, txID, preAssembly, resolvedVerifiers, req.stateSnapshot, req.coordinatorsBlockHeight)
	if err != nil {
		if ctx.Err() != nil {
			log.L(ctx).Debugf("abandoning assembly for transaction %s: request expired", txID)
			return
		}
		log.L(ctx).Errorf("failed to assemble and sign transaction: %s", err)
		//This should never happen but if it does, the most likely cause of failure is an error in the local domain code. We should
		// tell the coordinator so it can park or discard the transaction
		txn.queueEventForOriginator(ctx, &AssembleErrorEvent{
			BaseEvent: BaseEvent{
				TransactionID: txID,
			},
			RequestID: req.requestID,
		})
		return
	}

	postAssembly := &components.TransactionPostAssembly{AssembleResponse: assembleResponse}
	switch assembleResponse.GetAssemblyResult() {
	case prototk.AssembleTransactionResponse_OK:
		log.L(ctx).Debugf("emitting AssembleAndSignSuccessEvent: %s", txID.String())
		txn.queueEventForOriginator(ctx, &AssembleAndSignSuccessEvent{
			BaseEvent: BaseEvent{
				TransactionID: txID,
			},
			RequestID:    req.requestID,
			PostAssembly: postAssembly,
		})
	case prototk.AssembleTransactionResponse_REVERT:
		log.L(ctx).Debugf("emitting AssembleRevertEvent: %s", txID.String())
		txn.queueEventForOriginator(ctx, &AssembleRevertEvent{
			BaseEvent: BaseEvent{
				TransactionID: txID,
			},
			RequestID:    req.requestID,
			PostAssembly: postAssembly,
		})
	case prototk.AssembleTransactionResponse_PARK:
		log.L(ctx).Debugf("emitting AssembleParkEvent: %s", txID.String())
		txn.queueEventForOriginator(ctx, &AssembleParkEvent{
			BaseEvent: BaseEvent{
				TransactionID: txID,
			},
			RequestID:    req.requestID,
			PostAssembly: postAssembly,
		})
	}
}

func buildAssembleResponse(txn *originatorTransaction) *engineProto.AssembleResponse {
	return &engineProto.AssembleResponse{
		TransactionId:     txn.pt.ID.String(),
		AssembleRequestId: txn.latestFulfilledAssembleRequestID.String(),
		ContractAddress:   txn.pt.Address.HexString(),
		PostAssembly:      txn.pt.PostAssembly.AssembleResponse,
	}
}

func action_SendAssembleRevertResponse(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	return txn.transportWriter.SendAssembleResponse(ctx, txn.currentDelegate, buildAssembleResponse(txn))
}

func action_SendAssembleParkResponse(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	return txn.transportWriter.SendAssembleResponse(ctx, txn.currentDelegate, buildAssembleResponse(txn))
}

func action_SendAssembleSuccessResponse(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	return txn.transportWriter.SendAssembleResponse(ctx, txn.currentDelegate, buildAssembleResponse(txn))
}

func action_SendAssembleError(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	return txn.transportWriter.SendAssembleError(ctx, txn.currentDelegate, &engineProto.AssembleError{
		TransactionId:     txn.pt.ID.String(),
		AssembleRequestId: txn.latestFulfilledAssembleRequestID.String(),
		ContractAddress:   txn.pt.Address.HexString(),
	})
}

func action_RefreshBlockHeight(ctx context.Context, t *originatorTransaction, _ common.Event) error {
	t.refreshBlockHeight(ctx)
	return nil
}

// validator_IsPrivateStateDataPendingForAssembly returns true when private state data is pending
// arrival at this node up to the coordinator's low watermark (coordinatorsBlockHeight - blockHeightTolerance).
func validator_IsPrivateStateDataPendingForAssembly(ctx context.Context, t *originatorTransaction, event common.Event) (bool, error) {
	e := event.(*AssembleRequestReceivedEvent)
	complete, err := t.engineIntegration.CheckPendingPrivateStateData(ctx, e.CoordinatorBlockHeight-e.BlockHeightTolerance)
	return !complete, err
}

// action_RejectAssemblyPrivateStateDataPending sends an AssembleRejection with reason
// PrivateStateDataPending to the coordinator. The originator stays in its current state;
// the coordinator will retry once the pending private state data has arrived.
func action_RejectAssemblyPrivateStateDataPending(ctx context.Context, t *originatorTransaction, event common.Event) error {
	e := event.(*AssembleRequestReceivedEvent)
	receiverBlockHeight := t.getBlockHeight()
	log.L(ctx).Warnf("rejecting assemble request from coordinator due to pending private state data (coordinator=%d, assembler=%d, tolerance=%d)",
		e.CoordinatorBlockHeight, receiverBlockHeight, e.BlockHeightTolerance)
	return t.transportWriter.SendAssembleRejection(ctx, e.Coordinator, &engineProto.AssembleRejection{
		TransactionId:          t.pt.ID.String(),
		AssembleRequestId:      e.RequestID.String(),
		ContractAddress:        t.pt.Address.HexString(),
		RejectionReason:        engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING,
		CoordinatorBlockHeight: e.CoordinatorBlockHeight,
		AssemblerBlockHeight:   receiverBlockHeight,
	})
}

// validator_AssembleBlockHeightToleranceExceeded returns true when the absolute difference between
// the coordinator's block height (from the assemble request) and this originator's block height
// exceeds the tolerance carried on the event.
func validator_AssembleBlockHeightToleranceExceeded(_ context.Context, t *originatorTransaction, event common.Event) (bool, error) {
	e := event.(*AssembleRequestReceivedEvent)
	receiverBH := uint64(t.getBlockHeight())
	coordinatorBH := uint64(e.CoordinatorBlockHeight)
	diff := max(receiverBH, coordinatorBH) - min(receiverBH, coordinatorBH)
	return diff > uint64(e.BlockHeightTolerance), nil
}

// action_SendAssembleBlockHeightRejection sends an AssembleRejection indicating the block height
// difference between coordinator and originator exceeds the tolerance carried on the event.
func action_SendAssembleBlockHeightRejection(ctx context.Context, t *originatorTransaction, event common.Event) error {
	e := event.(*AssembleRequestReceivedEvent)
	receiverBlockHeight := t.getBlockHeight()
	log.L(ctx).Warnf("rejecting assemble request from coordinator due to block height tolerance (coordinator=%d, assembler=%d, tolerance=%d)",
		e.CoordinatorBlockHeight, receiverBlockHeight, e.BlockHeightTolerance)
	return t.transportWriter.SendAssembleRejection(ctx, e.Coordinator, &engineProto.AssembleRejection{
		TransactionId:          t.pt.ID.String(),
		AssembleRequestId:      e.RequestID.String(),
		ContractAddress:        t.pt.Address.HexString(),
		RejectionReason:        engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
		CoordinatorBlockHeight: e.CoordinatorBlockHeight,
		AssemblerBlockHeight:   receiverBlockHeight,
	})
}

func validator_AssembleAndSignSuccessMatchesCurrentRequest(_ context.Context, t *originatorTransaction, event common.Event) (bool, error) {
	e := event.(*AssembleAndSignSuccessEvent)
	if t.latestAssembleRequest == nil {
		return false, nil
	}
	return t.latestAssembleRequest.requestID == e.RequestID, nil
}

// guard_AssembleRequestMatchesInProgressAssembly returns true when the most recent assemble request
// has the same idempotency key as the assembly goroutine currently in flight.
// This detects a coordinator nudge arriving while the originator is still assembling the original
// request: the nudge carries the same idempotency key, so there is no need to cancel and restart.
func guard_AssembleRequestMatchesInProgressAssembly(_ context.Context, txn *originatorTransaction) bool {
	return txn.cancelCurrentAssembly != nil && txn.latestAssembleRequest.requestID == txn.currentAssemblyRequestID
}

// action_SendAssembleRejectionNotCurrentDelegate sends an AssembleRejection indicating the
// originator does not recognise the sender as its current active coordinator.
func action_SendAssembleRejectionNotCurrentDelegate(ctx context.Context, txn *originatorTransaction, event common.Event) error {
	assembleRequestEvent := event.(*AssembleRequestReceivedEvent)
	log.L(ctx).Debugf("rejecting assemble request from %s: not current delegate (current=%s)", assembleRequestEvent.Coordinator, txn.currentDelegate)
	if err := txn.transportWriter.SendAssembleRejection(ctx, assembleRequestEvent.Coordinator, &engineProto.AssembleRejection{
		TransactionId:     txn.pt.ID.String(),
		AssembleRequestId: assembleRequestEvent.RequestID.String(),
		ContractAddress:   txn.pt.Address.HexString(),
		RejectionReason:   engineProto.RejectionReason_NOT_CURRENT_DELEGATE,
	}); err != nil {
		log.L(ctx).Warnf("failed to send assemble rejection (not-current-delegate) to %s: %s", assembleRequestEvent.Coordinator, err)
	}
	return nil
}
