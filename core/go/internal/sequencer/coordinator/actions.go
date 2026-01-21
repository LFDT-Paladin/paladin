/*
 * Copyright © 2026 Kaleido, Inc.
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

package coordinator

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
)

func action_SendHandoverRequest(ctx context.Context, reader coordinatorStateReader, config *stateMachineConfig, callbacks *stateMachineCallbacks, _ common.Event) error {
	err := callbacks.transportWriter.SendHandoverRequest(ctx, reader.GetActiveCoordinatorNode(), config.contractAddress)
	if err != nil {
		log.L(ctx).Errorf("error sending handover request: %v", err)
	}
	return err
}

// action_PropagateEventToAllTransactions propagates HeartbeatIntervalEvent to all transactions
func action_PropagateEventToAllTransactions(ctx context.Context, reader coordinatorStateReader, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) error {
	for _, txn := range reader.GetAllTransactions() {
		err := txn.ProcessEvent(ctx, event)
		if err != nil {
			log.L(ctx).Errorf("error handling event %v for transaction %s: %v", event.Type(), txn.ID.String(), err)
			return err
		}
	}
	return nil
}

func action_CoordinatorActive(ctx context.Context, reader coordinatorStateReader, config *stateMachineConfig, callbacks *stateMachineCallbacks, _ common.Event) error {
	callbacks.coordinatorActive(config.contractAddress, config.nodeName)
	return nil
}

// action_NotifyCoordinatorActive notifies the sequencer lifecycle manager that a coordinator is active
// TODO AM: need a clearer name between this and action_CoordinatorActive
func action_NotifyCoordinatorActive(_ context.Context, _ coordinatorStateReader, config *stateMachineConfig, callbacks *stateMachineCallbacks, event common.Event) error {
	var from string
	switch e := event.(type) {
	case *EndorsementRequestedEvent:
		from = e.From
	case *HeartbeatReceivedEvent:
		from = e.From
	default:
		return nil
	}
	callbacks.coordinatorActive(config.contractAddress, from)
	return nil
}

func action_StartHeartbeatLoop(ctx context.Context, reader coordinatorStateReader, _ *stateMachineConfig, callbacks *stateMachineCallbacks, _ common.Event) error {
	callbacks.startHeartbeatLoop(ctx)
	return nil
}

func action_NotifiyTransactionSelected(ctx context.Context, reader coordinatorStateReader, _ *stateMachineConfig, _ *stateMachineCallbacks, event common.Event) error {
	transactionSelectedEvent := event.(*transaction.SelectedEvent)
	txn := reader.GetTransactionByID(transactionSelectedEvent.TransactionID)
	if txn == nil {
		log.L(ctx).Warnf("Transaction %s not found when attempting to notify", transactionSelectedEvent.TransactionID.String())
		return nil
	}
	return txn.ProcessEvent(ctx, transactionSelectedEvent)
}

func action_Idle(_ context.Context, _ coordinatorStateReader, config *stateMachineConfig, callbacks *stateMachineCallbacks, _ common.Event) error {
	callbacks.coordinatorIdle(config.contractAddress)
	callbacks.stopHeartbeatLoop()
	return nil
}

func action_SendHeartbeat(ctx context.Context, reader coordinatorStateReader, config *stateMachineConfig, callbacks *stateMachineCallbacks, _ common.Event) error {
	snapshot := getSnapshot(ctx, reader, config)
	log.L(ctx).Debugf("sending heartbeats for sequencer %s", config.contractAddress.String())
	var err error
	for _, node := range reader.GetOriginatorNodePool() {
		if node != config.nodeName {
			log.L(ctx).Debugf("sending heartbeat to %s", node)
			err = callbacks.transportWriter.SendHeartbeat(ctx, node, config.contractAddress, snapshot)
			if err != nil {
				log.L(ctx).Errorf("error sending heartbeat to %s: %v", node, err)
			}
		}
	}
	return err
}

func action_QueueTransactionForDispatch(ctx context.Context, reader coordinatorStateReader, _ *stateMachineConfig, callbacks *stateMachineCallbacks, event common.Event) error {
	transactionStateTransitionEvent := event.(*common.TransactionStateTransitionEvent)

	txID := transactionStateTransitionEvent.TxID
	toState := transaction.State(transactionStateTransitionEvent.To)
	fromState := transaction.State(transactionStateTransitionEvent.From)

	log.L(ctx).Debugf("Handling transaction %s state transition from %s to %s", txID.String(), fromState.String(), toState.String())

	txn := reader.GetTransactionByID(transactionStateTransitionEvent.TxID)
	if txn == nil {
		log.L(ctx).Warnf("Transaction %s not found when attempting to transition", transactionStateTransitionEvent.TxID.String())
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "transaction not found")
	}

	// the guard should have already checked this but we'll check again to be sure
	if toState == transaction.State_Ready_For_Dispatch {
		callbacks.queueForDispatch(txn)

	}
	return nil
}
