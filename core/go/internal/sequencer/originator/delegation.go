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
	"github.com/google/uuid"
)

// inFlightDelegation tracks a delegation request that has been sent but whose acknowledgement has not
// yet arrived. It lives in the full or partial in-flight slot, and its one-shot timer signals a request
// timeout: when it fires it notifies for a new delegaation so the batching loop re-delegates the
// still-outstanding transactions from live state under a fresh delegation ID. The transactions it carried
// are not recorded here because that rebuild reads live state rather than replaying this request.
type inFlightDelegation struct {
	delegationID string
	cancelTimer  func()
}

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
	o.delegationLoopCtx = loopCtx
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
// Any in-flight delegation requests are discarded along with their timers: outside State_Sending
// there is nothing left to delegate.
func (o *originator) stopDelegationLoop() {
	if o.delegationLoopCancel == nil {
		return
	}
	o.delegationLoopCancel()
	<-o.delegationLoopDone
	o.cancelAllInFlightDelegations()
	o.delegationLoopCtx = nil
	o.delegationLoopCancel = nil
	o.delegationLoopDone = nil
	o.notifyFullDelegation = nil
	o.notifyPartialDelegation = nil
}

// delegationLoop coalesces delegation requests. On each batch tick it drains both dirty-flag
// channels and, if either was set, queues a single DelegateSendBatchEvent onto the event loop. full
// takes priority over partial (a full send is a superset of a partial one). Both channels are drained
// every tick so a stale partial notification cannot linger behind a full one.
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
// When full is false only transactions whose state is State_Pending — sent but not yet acknowledged
// by the coordinator, or not yet sent at all — are included. FIFO ordering with transactions the
// coordinator has already acknowledged is preserved by naming the most recent State_Delegated
// transaction in the request's last_delegated_transaction_id rather than resending it.
func sendDelegationRequest(ctx context.Context, o *originator, full bool) error {
	// Delegate resolved transactions in the order they were created on the originating node.
	// A still-resolving transaction blocks all later transactions so the coordinator
	// receives them in creation order and never sees a transaction whose verifiers are unresolved.
	inFlight := 0
	transactionsToDelegate := make([]*components.PrivateTransaction, 0)
	var lastDelegatedTransactionID string
	var lastDelegatedTxn transaction.OriginatorTransaction
	for _, txn := range o.transactionsOrdered {
		// A transaction has advanced past verifier resolution, and so is eligible for delegation, only
		// once it has left State_Initial and State_Resolving.
		state := txn.GetCurrentState()
		if state == transaction.State_Initial || state == transaction.State_Resolving {
			break
		}
		inFlight++
		if !full {
			// Acknowledged (Delegated) transactions are not resent; the most recent one seen before the
			// first included transaction is named as the predecessor the coordinator orders this
			// request after. Acceptance is prefix-ordered, so Delegated transactions precede Pending ones.
			if state == transaction.State_Delegated && len(transactionsToDelegate) == 0 {
				lastDelegatedTxn = txn
			}
			if state != transaction.State_Pending {
				continue
			}
		}
		if lastDelegatedTxn != nil {
			lastDelegatedTransactionID = lastDelegatedTxn.GetID().String()
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

	// Record the request before sending so that even a request which fails to send times out and is
	// re-delegated. recordInFlightDelegation mints the delegation ID, supersedes the requests this send
	// replaces, and arms a fresh one-shot timer; the request below is sent under ifd.delegationID.
	ifd := o.recordInFlightDelegation(full)

	return o.transportWriter.SendDelegationRequest(ctx, o.currentActiveCoordinator, &engineProto.DelegationRequest{
		DelegationId:               ifd.delegationID,
		DelegateNodeId:             o.currentActiveCoordinator,
		OriginatorBlockHeight:      int64(o.currentBlockHeight),
		ContractAddress:            o.contractAddress.HexString(),
		Transactions:               delegations,
		LastDelegatedTransactionId: lastDelegatedTransactionID,
	})
}

// recordInFlightDelegation mints a fresh delegation ID, supersedes the in-flight requests this send
// replaces, then records the new request in its slot and arms its timer, returning the entry so the caller
// sends under its delegation ID. A full send supersedes every in-flight request (it re-delegates a superset);
// a partial send supersedes only a prior partial — a full carries transactions a partial would not resend, so
// its recovery timer is left running. This keeps at most one full and one partial in flight at once, each to
// the current coordinator.
func (o *originator) recordInFlightDelegation(full bool) *inFlightDelegation {
	ifd := &inFlightDelegation{delegationID: uuid.New().String()}
	if full {
		o.cancelAllInFlightDelegations()
		o.fullInFlight = ifd
		o.startInFlightDelegationTimer(ifd, o.notifyFullDelegation)
	} else {
		stopInFlightDelegationTimer(o.partialInFlight)
		o.partialInFlight = ifd
		o.startInFlightDelegationTimer(ifd, o.notifyPartialDelegation)
	}
	return ifd
}

func (o *originator) startInFlightDelegationTimer(ifd *inFlightDelegation, notify chan struct{}) {
	loopCtx := o.delegationLoopCtx
	if loopCtx == nil {
		return
	}
	ifd.cancelTimer = o.clock.ScheduleTimer(loopCtx, o.requestTimeout, func() {
		select {
		case notify <- struct{}{}:
		default:
		}
	})
}

// stopInFlightDelegationTimer stops an in-flight request's one-shot timer, if it has one. Safe on a nil entry.
func stopInFlightDelegationTimer(ifd *inFlightDelegation) {
	if ifd != nil && ifd.cancelTimer != nil {
		ifd.cancelTimer()
	}
}

// cancelInFlightByID stops and clears whichever in-flight slot holds the given delegation ID. Called when
// its acknowledgement arrives (whatever the per-transaction outcomes were: a response means the request
// landed). A response for a request already superseded matches neither slot and is a no-op; its
// transactions are still applied by the caller from the response itself.
func (o *originator) cancelInFlightByID(delegationID string) {
	switch {
	case o.fullInFlight != nil && o.fullInFlight.delegationID == delegationID:
		stopInFlightDelegationTimer(o.fullInFlight)
		o.fullInFlight = nil
	case o.partialInFlight != nil && o.partialInFlight.delegationID == delegationID:
		stopInFlightDelegationTimer(o.partialInFlight)
		o.partialInFlight = nil
	}
}

// cancelAllInFlightDelegations discards both in-flight delegation requests. Called when the active
// coordinator changes — the full delegation to the new coordinator supersedes them — and when the
// delegation loop stops.
func (o *originator) cancelAllInFlightDelegations() {
	stopInFlightDelegationTimer(o.fullInFlight)
	stopInFlightDelegationTimer(o.partialInFlight)
	o.fullInFlight = nil
	o.partialInFlight = nil
}

// action_NotifyPartialDelegation indicates to the delegation batching loop that a partial delegation
// (only State_Pending transactions, i.e. those not yet acknowledged by the coordinator) will be
// required on its next tick. Every partial request carries all Pending transactions, so a lost
// request or acknowledgement is repaired by the next partial send. o.notifyPartialDelegation has
// length 1, so that multiple notfications result in a single delegation request.
func action_NotifyPartialDelegation(_ context.Context, o *originator, _ common.Event) error {
	o.partialDelegationNotification()
	return nil
}

func (o *originator) partialDelegationNotification() {
	select {
	case o.notifyPartialDelegation <- struct{}{}:
	default:
	}
}

// action_NotifyFullDelegation indicates to the delegation batching loop that a full delegation
// (all resolved but unconfirmed transactions) will be required on its next tick. o.notifyFullDelegation
// has length 1, so that multiple notfications result in a single delegation request.
func action_NotifyFullDelegation(_ context.Context, o *originator, _ common.Event) error {
	o.fullDelegationNotification()
	return nil
}

func (o *originator) fullDelegationNotification() {
	select {
	case o.notifyFullDelegation <- struct{}{}:
	default:
	}
}

// action_SendDelegation is the sole handler of Event_DelegateSendBatch, queued by the batching goroutine.
// It refreshes the block height once per flush (rather than once per trigger) and sends the coalesced
// delegation request. The trigger is the same whether a fresh notification or an in-flight request's
// timeout set the flag: sendDelegationRequest rebuilds from live state, re-delegating any still-Pending
// transactions under a fresh delegation ID.
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
		o.cancelAllInFlightDelegations()
	}
	return nil
}

// validator_IsDelegationAckFromCurrentCoordinator returns true when the acknowledgement's sender is
// the currently tracked active coordinator. An acknowledgement from any other node is stale (e.g.
// sent by a coordinator we have since failed away from) and is dropped.
func validator_IsDelegationAckFromCurrentCoordinator(_ context.Context, o *originator, event common.Event) (bool, error) {
	return event.(*DelegationRequestAcknowledgedEvent).FromNode == o.currentActiveCoordinator, nil
}

// action_HandleDelegationAcknowledged processes a DelegationResponse from the current active
// coordinator. The response means the request landed, so its in-flight request timer is stopped
// regardless of the per-transaction outcomes. The coordinator acknowledges transactions in the order
// they were sent, accepting each one until it reaches one it could not take on. Each accepted
// transaction is moved Pending → Delegated; the first transaction the coordinator did not accept, and
// every transaction after it, stays Pending and is re-delegated. That re-delegation is full when the
// coordinator did not recognise our last delegated transaction (it cannot then guarantee FIFO
// ordering), and partial otherwise.
func action_HandleDelegationAcknowledged(ctx context.Context, o *originator, event common.Event) error {
	e := event.(*DelegationRequestAcknowledgedEvent)
	o.cancelInFlightByID(e.DelegationID)

	for i, transactionIDString := range e.TransactionIDs {
		if i >= len(e.Results) {
			// A response with fewer acknowledgements than transactions is malformed; treat this
			// transaction and everything after it as un-acknowledged and re-delegate them.
			log.L(ctx).Warnf("coordinator %s returned fewer acknowledgements than transactions; requesting partial delegation", e.FromNode)
			o.partialDelegationNotification()
			return nil
		}
		if e.Results[i] == engineProto.DelegationAcknowledgementResult_UNKNOWN_LAST_DELEGATED_TRANSACTION {
			// The coordinator does not recognise our last delegated transaction and so cannot guarantee
			// FIFO ordering. Re-delegate everything with a full delegation.
			log.L(ctx).Warnf("coordinator %s does not recognise our last delegated transaction; requesting full delegation", e.FromNode)
			o.fullDelegationNotification()
			return nil
		}
		if e.Results[i] != engineProto.DelegationAcknowledgementResult_DELEGATION_ACCEPTED {
			// The coordinator did not accept this transaction, so it and every transaction after it stays
			// Pending. Re-delegate them with a partial delegation.
			log.L(ctx).Debugf("coordinator %s did not accept transaction %s; requesting partial delegation", e.FromNode, transactionIDString)
			o.partialDelegationNotification()
			return nil
		}

		// The coordinator accepted this transaction: move it Pending → Delegated.
		transactionID, err := uuid.Parse(transactionIDString)
		if err != nil {
			log.L(ctx).Warnf("delegation acknowledgement from %s contains invalid transaction ID %s: %v", e.FromNode, transactionIDString, err)
			continue
		}
		txn := o.transactionsByID[transactionID]
		if txn == nil {
			// The transaction completed and was cleaned up while the acknowledgement was in flight.
			continue
		}
		err = txn.HandleEvent(ctx, &transaction.DelegationAcknowledgedEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: transactionID,
			},
			Coordinator: e.FromNode,
		})
		if err != nil {
			msg := fmt.Errorf("error handling delegation acknowledged event for transaction %s: %v", transactionIDString, err)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}
	return nil
}
