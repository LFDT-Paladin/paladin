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
	"time"

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

// SeqOriginator is the interface that consumers use to interact with the originator.
// GetCurrentCoordinator uses its own mutex (not the event-loop lock) so it can be called without deadlocking with the event loop.
// GetTxStatus is thread-safe via RLock.
// TODO: SetActiveCoordinator modifies state outside the state machine and is an exception to the pattern
// which needs to be resolved
type SeqOriginator interface {
	// Asynchronously update the state machine by queueing an event to be processed. Most
	// callers should use this interface.
	QueueEvent(ctx context.Context, event common.Event)
	Stop()

	GetCurrentCoordinator() string
	GetTxStatus(ctx context.Context, txID uuid.UUID) (status components.PrivateTxStatus, err error)

	// DANGEROUS- THIS FUNCTION MODIFIES STATE OUTSIDE OF THE STATE MACHINE
	SetActiveCoordinator(ctx context.Context, coordinator string) error
}

type originator struct {
	sync.RWMutex // Implements statemachine.Lockable for thread-safe event processing

	/* State machine - using generic statemachine.ProcessorEventLoop */
	processorEventLoop *statemachine.ProcessorEventLoop[State, *originator]
	// activeCoordinatorNode is protected independently of the statemachine loop since there
	// is a possible deadlock when a node is both originator and coordinator where processing a message
	// from the loopback writer queue needs to call GetCurrentCoordinator, but the event loop is holding the
	// main mutex while waiting to put a new message onto the loopback writer queue.
	// TODO: can refactoring how coordinators/sequencers are loaded resolve this deadlock?
	activeCoordinatorMutex      sync.RWMutex
	activeCoordinatorNode       string
	timeOfMostRecentHeartbeat   common.Time
	transactionsByID            map[uuid.UUID]*transaction.Transaction
	submittedTransactionsByHash map[pldtypes.Bytes32]*uuid.UUID
	transactionsOrdered         []*uuid.UUID
	currentBlockHeight          uint64
	latestCoordinatorSnapshot   *common.CoordinatorSnapshot

	/* Config */
	nodeName             string
	blockRangeSize       uint64
	contractAddress      *pldtypes.EthAddress
	heartbeatThresholdMs common.Duration
	delegateTimeout      common.Duration

	/* Dependencies */
	transportWriter   transport.TransportWriter
	clock             common.Clock
	engineIntegration common.EngineIntegration
	metrics           metrics.DistributedSequencerMetrics

	/* Delegate loop */
	stopDelegateLoop    chan struct{}
	delegateLoopStopped chan struct{}
}

func NewOriginator(
	ctx context.Context,
	nodeName string,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	engineIntegration common.EngineIntegration,
	contractAddress *pldtypes.EthAddress,
	configuration *pldconf.SequencerConfig,
	heartbeatPeriodMs int,
	heartbeatThresholdIntervals int,
	metrics metrics.DistributedSequencerMetrics,
) (*originator, error) {
	o := &originator{
		nodeName:                    nodeName,
		transactionsByID:            make(map[uuid.UUID]*transaction.Transaction),
		submittedTransactionsByHash: make(map[pldtypes.Bytes32]*uuid.UUID),
		transportWriter:             transportWriter,
		blockRangeSize:              confutil.Uint64Min(configuration.BlockRange, pldconf.SequencerMinimum.BlockRange, *pldconf.SequencerDefaults.BlockRange),
		contractAddress:             contractAddress,
		clock:                       clock,
		engineIntegration:           engineIntegration,
		heartbeatThresholdMs:        clock.Duration(heartbeatPeriodMs * heartbeatThresholdIntervals),
		delegateTimeout:             confutil.DurationMin(configuration.DelegateTimeout, pldconf.SequencerMinimum.DelegateTimeout, *pldconf.SequencerDefaults.DelegateTimeout),
		metrics:                     metrics,
		stopDelegateLoop:            make(chan struct{}),
		delegateLoopStopped:         make(chan struct{}),
	}
	o.initializeProcessorEventLoop(State_Idle)

	go o.processorEventLoop.Start(ctx)
	go o.delegateLoop(ctx)

	return o, nil
}

// A sequencer can be asked to page itself out at any time to make space for other sequencers.
// This hook point provides a place to perform any tidy up actions needed in the originator
func (o *originator) Stop() {
	log.L(context.Background()).Infof("Stopping originator for contract %s", o.contractAddress.String())

	// Make Stop() idempotent - make sure we've not already been stopped
	if o.processorEventLoop.IsStopped() {
		return
	}

	o.stopDelegateLoop <- struct{}{}
	<-o.delegateLoopStopped

	o.processorEventLoop.Stop()
}

func (o *originator) GetCurrentState() State {
	return o.processorEventLoop.GetCurrentState()
}

func (o *originator) QueueEvent(ctx context.Context, event common.Event) {
	log.L(ctx).Tracef("Pushing originator event onto event queue: %s", event.TypeString())
	o.processorEventLoop.QueueEvent(ctx, event)
	log.L(ctx).Tracef("Pushed originator event onto event queue: %s", event.TypeString())
}

func (o *originator) delegateLoop(ctx context.Context) {
	defer close(o.delegateLoopStopped)
	log.L(ctx).Debugf("delegate loop started for contract %s", o.contractAddress.String())

	// Check for transactions still waiting to be delegated
	ticker := time.NewTicker(o.delegateTimeout.(time.Duration))
	defer func() {
		log.L(ctx).Debugf("delegate loop stopping for contract %s", o.contractAddress.String())
		ticker.Stop()
	}()
	for {
		select {
		case <-ticker.C:
			log.L(ctx).Debugf("delegate loop fired for contract %s", o.contractAddress.String())
			delegateTimeoutEvent := &DelegateTimeoutEvent{}
			delegateTimeoutEvent.BaseEvent = common.BaseEvent{}
			delegateTimeoutEvent.EventTime = time.Now()
			o.processorEventLoop.TryQueueEvent(ctx, delegateTimeoutEvent)
		case <-o.stopDelegateLoop:
			log.L(ctx).Debugf("delegate loop stopped for contract %s", o.contractAddress.String())
			return
		}
	}
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

func (o *originator) createTransaction(ctx context.Context, txn *components.PrivateTransaction) error {
	newTxn, err := transaction.NewTransaction(ctx,
		txn,
		o.transportWriter,
		o.QueueEvent,
		o.engineIntegration,
		o.metrics)
	if err != nil {
		log.L(ctx).Errorf("error creating transaction: %v", err)
		return err
	}
	o.transactionsByID[txn.ID] = newTxn
	o.transactionsOrdered = append(o.transactionsOrdered, &txn.ID)
	createdEvent := &transaction.CreatedEvent{}
	createdEvent.TransactionID = txn.ID
	err = newTxn.HandleEvent(ctx, createdEvent)
	if err != nil {
		log.L(ctx).Errorf("error handling CreatedEvent for transaction %s: %v", txn.ID.String(), err)
		return err
	}
	return nil
}

func (o *originator) removeTransaction(ctx context.Context, txnID uuid.UUID) {
	log.L(ctx).Debugf("removing transaction %s from originator", txnID.String())

	// Remove from transactionsByID
	delete(o.transactionsByID, txnID)

	// Remove from transactionsOrdered
	for i, id := range o.transactionsOrdered {
		if *id == txnID {
			o.transactionsOrdered = append(o.transactionsOrdered[:i], o.transactionsOrdered[i+1:]...)
			break
		}
	}

	// Note: submittedTransactionsByHash cleanup is handled separately in confirmTransaction
}

func (o *originator) transactionsOrderedByCreatedTime(ctx context.Context) ([]*transaction.Transaction, error) {
	//TODO AM are we actually saving anything by transactionsOrdered being an array of IDs rather than an array of *transaction.Transaction
	ordered := make([]*transaction.Transaction, len(o.transactionsOrdered))
	for i, id := range o.transactionsOrdered {
		ordered[i] = o.transactionsByID[*id]
	}
	return ordered, nil
}

func (o *originator) getTransactionsInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}
	matchingTxns := make([]*transaction.Transaction, 0, len(o.transactionsByID))
	for _, txn := range o.transactionsByID {
		if matchingStates[txn.GetCurrentState()] {
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}

func (o *originator) getTransactionsNotInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	nonMatchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		nonMatchingStates[state] = true
	}
	matchingTxns := make([]*transaction.Transaction, 0, len(o.transactionsByID))
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
