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

package originator

import (
	"context"
	"sync"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// Originator is the interface that consumers use to interact with the originator.
type Originator interface {
	// Asynchronously update the state machine by queueing an event to be processed
	// This the only interface by which consumers should update the state of the originator
	QueueEvent(ctx context.Context, event common.Event)

	GetTxStatus(ctx context.Context, txID uuid.UUID) (status components.PrivateTxStatus, err error)
	GetCurrentState() State

	WaitForDone(ctx context.Context)
}

type originator struct {
	// Mutex for thread-safe event processing (implements statemachine.Lockable)
	// Any functions passed to the state machine do not need to take the lock themselves
	// since the state machine takes the lock for the duration of the event processing.
	// Any functions that expose non atomic state outside of the originator must
	// take the read lock when called.
	sync.RWMutex
	ctx context.Context

	/* State machine - using generic statemachine.StateMachineEventLoop */
	stateMachineEventLoop              *statemachine.StateMachineEventLoop[State, *originator]
	activeCoordinatorNode              string
	heartbeatIntervalsSinceLastReceive int
	transactionsByID                   map[uuid.UUID]transaction.OriginatorTransaction
	transactionsOrdered                []transaction.OriginatorTransaction
	currentBlockHeight                 uint64
	latestCoordinatorSnapshot          *common.CoordinatorSnapshot

	/* Config */
	nodeName            string
	blockRangeSize      uint64
	contractAddress     *pldtypes.EthAddress
	idleThreshold       int // expressed as a multiple of heartbeat intervals
	redelegateThreshold int // expressed as a multiple of heartbeat intervals

	/* Dependencies */
	transportWriter   transport.TransportWriter
	engineIntegration common.EngineIntegration
	metrics           metrics.DistributedSequencerMetrics
}

func NewOriginator(
	ctx context.Context,
	nodeName string,
	transportWriter transport.TransportWriter,
	engineIntegration common.EngineIntegration,
	contractAddress *pldtypes.EthAddress,
	configuration *pldconf.SequencerConfig,
	metrics metrics.DistributedSequencerMetrics,
) (*originator, error) {
	origCtx := log.WithLogField(ctx, "role", "originator")
	o := &originator{
		ctx:                 origCtx,
		nodeName:            nodeName,
		transactionsByID:    make(map[uuid.UUID]transaction.OriginatorTransaction),
		transportWriter:     transportWriter,
		blockRangeSize:      confutil.Uint64Min(configuration.BlockRange, pldconf.SequencerMinimum.BlockRange, *pldconf.SequencerDefaults.BlockRange),
		contractAddress:     contractAddress,
		engineIntegration:   engineIntegration,
		metrics:             metrics,
		idleThreshold:       confutil.IntMin(configuration.InactiveToIdleGracePeriod, pldconf.SequencerMinimum.InactiveToIdleGracePeriod, *pldconf.SequencerDefaults.InactiveToIdleGracePeriod),
		redelegateThreshold: confutil.IntMin(configuration.RedelegateGracePeriod, pldconf.SequencerMinimum.RedelegateGracePeriod, *pldconf.SequencerDefaults.RedelegateGracePeriod),
	}

	originatorEventQueueSize := confutil.IntMin(configuration.OriginatorEventQueueSize, pldconf.SequencerMinimum.OriginatorEventQueueSize, *pldconf.SequencerDefaults.OriginatorEventQueueSize)
	originatorPriorityEventQueueSize := confutil.IntMin(configuration.OriginatorPriorityEventQueueSize, pldconf.SequencerMinimum.OriginatorPriorityEventQueueSize, *pldconf.SequencerDefaults.OriginatorPriorityEventQueueSize)
	o.initializeStateMachineEventLoop(State_Idle, originatorEventQueueSize, originatorPriorityEventQueueSize)

	go o.stateMachineEventLoop.Start(origCtx)

	return o, nil
}

func (o *originator) WaitForDone(ctx context.Context) {
	o.stateMachineEventLoop.WaitForDone(ctx)
}

func (o *originator) GetCurrentState() State {
	o.RLock()
	defer o.RUnlock()
	return o.stateMachineEventLoop.GetCurrentState()
}

func (o *originator) QueueEvent(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("Pushing originator event onto event queue: %s", event.TypeString())
	o.stateMachineEventLoop.QueueEvent(ctx, event)
	log.L(ctx).Tracef("Pushed originator event onto event queue: %s", event.TypeString())
}

func (o *originator) queueEventInternal(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("Pushing internal originator event onto priority queue: %s", event.TypeString())
	o.stateMachineEventLoop.QueuePriorityEvent(ctx, event)
	log.L(ctx).Tracef("Pushed internal originator event onto priority queue: %s", event.TypeString())
}

func (o *originator) propagateEventToTransaction(ctx context.Context, event transaction.Event) error {
	if txn := o.transactionsByID[event.GetTransactionID()]; txn != nil {
		return txn.HandleEvent(ctx, event)
	}

	// Transaction not known to this originator.
	// The most likely cause is that the transaction reached a terminal state (e.g., reverted during assembly)
	// and has since been removed from memory after cleanup. We need to tell the coordinator so they can clean up.
	log.L(ctx).Debugf("transaction not known to this originator %s", event.GetTransactionID().String())

	// Extract coordinator from events that require a response
	var coordinator string

	switch e := event.(type) {
	case *transaction.AssembleRequestReceivedEvent:
		coordinator = e.Coordinator
	case *transaction.PreDispatchRequestReceivedEvent:
		coordinator = e.Coordinator
	default:
		// Other events can be safely ignored
		return nil
	}

	log.L(ctx).Warnf("received %s for unknown transaction %s, notifying coordinator %s",
		event.TypeString(), event.GetTransactionID(), coordinator)
	return o.transportWriter.SendTransactionUnknown(ctx, coordinator, event.GetTransactionID())
}

// getTransactionsInStates returns transactions in any of the given states.
//
//nolint:unused // retaining until we decide we don't have any reasons for retrieving transactions by state
func (o *originator) getTransactionsInStates(states []transaction.State) []transaction.OriginatorTransaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}
	matchingTxns := make([]transaction.OriginatorTransaction, 0, len(o.transactionsByID))
	for _, txn := range o.transactionsByID {
		if matchingStates[txn.GetCurrentState()] {
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}

func (o *originator) getTransactionsNotInStates(states []transaction.State) []transaction.OriginatorTransaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	nonMatchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		nonMatchingStates[state] = true
	}
	matchingTxns := make([]transaction.OriginatorTransaction, 0, len(o.transactionsByID))
	for _, txn := range o.transactionsByID {
		if !nonMatchingStates[txn.GetCurrentState()] {
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}

func ptrTo[T any](v T) *T {
	return &v
}
