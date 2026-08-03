// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package originator

import (
	"context"
	"fmt"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
)

// startDelegationLoop creates the notification channels and starts the batching goroutine. Called from the
// State_Sending entry hook on the event-loop goroutine. No-op if the originator has not started yet
// or the loop is already running (nil-guarded like the coordinator dispatch loop).
func (o *originator) startDelegationLoop() {
	if o.ctx == nil || o.delegationLoopCancel != nil {
		return
	}
	o.notifyFullDelegation = make(chan struct{}, 1)
	o.notifyPartialDelegation = make(chan struct{}, 1)
	loopCtx, cancel := context.WithCancel(o.ctx)
	done := make(chan struct{})
	o.delegationLoopCancel = cancel
	o.delegationLoopDone = done
	// Capture the channels as locals so stopDelegationLoop nil-ing the struct fields never races the goroutine.
	full, partial, interval := o.notifyFullDelegation, o.notifyPartialDelegation, o.delegationBatchInterval
	go func() {
		defer close(done)
		o.delegationLoop(loopCtx, interval, full, partial)
	}()
}

// stopDelegationLoop cancels the batching goroutine and waits for it to exit. Called from the
// State_Sending exit hook on the event-loop goroutine. cancel() is called before the join so a
// goroutine blocked queueing a flush event is released via the queue's ctx.Done() branch.
func (o *originator) stopDelegationLoop() {
	if o.delegationLoopCancel == nil {
		return
	}
	o.delegationLoopCancel()
	<-o.delegationLoopDone
	o.delegationLoopCancel = nil
	o.delegationLoopDone = nil
	o.notifyFullDelegation = nil
	o.notifyPartialDelegation = nil
}

// delegationLoop coalesces delegation requests. On each batch tick it drains both dirty-flag
// channels and, if anything was requested, queues a single DelegateSendBatchEvent onto the event loop.
// full takes priority over partial (a full resend is a superset of a partial one). Both channels are
// drained every tick so a stale partial notification cannot linger behind a full one.
func (o *originator) delegationLoop(ctx context.Context, interval time.Duration, full, partial chan struct{}) {
	log.L(ctx).Debugf("delegation batching loop started for %s (interval %s)", o.contractAddress, interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			var partialSend, fullSend bool
			select {
			case <-partial:
				partialSend = true
			default:
			}
			select {
			case <-full:
				fullSend = true
			default:
			}
			if partialSend || fullSend {
				o.queueEventInternal(ctx, &DelegateSendBatchEvent{Full: fullSend})
			}

		case <-ctx.Done():
			log.L(ctx).Debugf("delegation batching loop stopped for %s", o.contractAddress)
			return
		}
	}
}

// sendDelegationRequest delegates transactions to the current active coordinator in the order they
// were created on the originating node.
//
// When full is true, every resolved transaction which isn't yet confirmed is (re)delegated. This is
// used by the recovery paths (entry to Sending, dropped transactions, silence/failover, and
// delegation-rejected redirect) where the coordinator may be missing state, so we bias toward
// over-sending.
//
// When full is false only transactions whose state is State_Pending or State_Delegated are included,
// i.e. the minimal set required to ensure FIFO ordering until first assembly
func sendDelegationRequest(ctx context.Context, o *originator, full bool) error {
	// Delegate resolved transactions in the order they were created on the originating node.
	// A still-resolving transaction blocks all later transactions so the coordinator
	// receives them in creation order and never sees a transaction whose verifiers are unresolved.
	inFlight := 0
	transactionsToDelegate := make([]*components.PrivateTransaction, 0)
	for _, txn := range o.transactionsOrdered {
		// A transaction has advanced past verifier resolution, and so is eligible for delegation, only
		// once it has left State_Initial and State_Resolving.
		state := txn.GetCurrentState()
		if state == transaction.State_Initial || state == transaction.State_Resolving {
			break
		}
		inFlight++
		// On the golden path skip any transaction the coordinator already knows about (it has been
		// assembled at least once, so its ordering is locked in). We skip both the DelegatedEvent and
		// the protobuf entry.
		if !full && state != transaction.State_Pending && state != transaction.State_Delegated {
			continue
		}
		transactionsToDelegate = append(transactionsToDelegate, txn.GetPrivateTransaction())
		err := txn.HandleEvent(ctx, &transaction.DelegatedEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: txn.GetID(),
			},
			Coordinator: o.currentActiveCoordinator,
		})
		if err != nil {
			msg := fmt.Errorf("error handling delegated event for transaction %s: %v", txn.GetID(), err)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}

	if len(transactionsToDelegate) == 0 {
		log.L(ctx).Debugf("no resolved transactions to delegate")
		return nil
	}

	log.L(ctx).Debugf("sending delegation request for %d of %d in-flight transactions (full=%t)",
		len(transactionsToDelegate), inFlight, full)

	delegations := make([]*engineProto.PrivateTransactionDelegation, 0, len(transactionsToDelegate))
	for _, tx := range transactionsToDelegate {
		delegations = append(delegations, &engineProto.PrivateTransactionDelegation{
			Id:          tx.ID.String(),
			Domain:      tx.Domain,
			Intent:      tx.Intent,
			PreAssembly: tx.PreAssembly,
		})
	}
	return o.transportWriter.SendDelegationRequest(ctx, o.currentActiveCoordinator, &engineProto.DelegationRequest{
		DelegateNodeId:        o.currentActiveCoordinator,
		OriginatorBlockHeight: int64(o.currentBlockHeight),
		ContractAddress:       o.contractAddress.HexString(),
		Transactions:          delegations,
	})
}

// action_NotifyPartialDelegation indicates to the delegation batching loop that a partial delegation
// (only transactions State_Pending / State_Delegated) will be required on its next tick. Sending all
// transactions in these states is required to preserve FIFO ordering from this originator until first
// assembly. o.notifyPartialDelegation has length 1, so that multiple notfications result in a single
// delegation request.
func action_NotifyPartialDelegation(_ context.Context, o *originator, _ common.Event) error {
	select {
	case o.notifyPartialDelegation <- struct{}{}:
	default:
	}
	return nil
}

// action_NotifyFullDelegation indicates to the delegation batching loop that a full delegation
// (all resolved but unconfirmed transactions) will be required on its next tick. o.notifyFullDelegation
// has length 1, so that multiple notfications result in a single delegation request.
func action_NotifyFullDelegation(_ context.Context, o *originator, _ common.Event) error {
	select {
	case o.notifyFullDelegation <- struct{}{}:
	default:
	}
	return nil
}

// action_SendDelegation is the sole handler of Event_DelegateSendBatch, queued by the batching goroutine.
// It refreshes the block height once per flush (rather than once per trigger) and sends the coalesced
// delegation request.
func action_SendDelegation(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*DelegateSendBatchEvent)
	o.refreshBlockHeight(ctx)
	return sendDelegationRequest(ctx, o, e.Full)
}

// action_StartDelegationLoop starts the delegation batching goroutine on entry to State_Sending.
func action_StartDelegationLoop(_ context.Context, o *originator, _ common.Event) error {
	o.startDelegationLoop()
	return nil
}

// action_StopDelegationLoop stops the delegation batching goroutine on exit from State_Sending.
func action_StopDelegationLoop(_ context.Context, o *originator, _ common.Event) error {
	o.stopDelegationLoop()
	return nil
}

func validator_IsDelegationBlockHeightRejection(_ context.Context, _ *originator, event common.Event) (bool, error) {
	return event.(*DelegationRequestRejectedEvent).RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, nil
}

func validator_IsDelegationNotActiveCoordinatorRejection(_ context.Context, _ *originator, event common.Event) (bool, error) {
	return event.(*DelegationRequestRejectedEvent).RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE, nil
}

func action_LogDelegationBlockHeightRejection(ctx context.Context, _ *originator, event common.Event) error {
	e := event.(*DelegationRequestRejectedEvent)
	log.L(ctx).Warnf("delegation rejected due to block height tolerance exceeded: originator block height=%d, coordinator block height=%d, coordinator tolerance=%d",
		e.OriginatorBlockHeight, e.CoordinatorBlockHeight, e.BlockHeightTolerance)
	return nil
}

// action_HandleDelegationRejected processes a rejection from a coordinator. If the rejection names
// a coordinator that has higher priority than our current one, we redirect to it
func action_HandleDelegationRejected(_ context.Context, o *originator, event common.Event) error {
	e := event.(*DelegationRequestRejectedEvent)
	if e.ActiveCoordinator == "" {
		return nil
	}
	if common.IsHigherPriority(o.coordinatorPriorityList, e.ActiveCoordinator, o.currentActiveCoordinator) {
		o.currentActiveCoordinator = e.ActiveCoordinator
		o.resetFailoverIndex()
	}
	return nil
}
