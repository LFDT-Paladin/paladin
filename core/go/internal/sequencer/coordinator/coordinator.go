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

package coordinator

import (
	"context"
	"sync"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// Coordinator is the interface that consumers should use to interact with the coordinator.
type Coordinator interface {
	// Asynchronously update the state machine by queueing an event to be processed
	// This is the only interface by which consumers should update the state of the coordinator
	QueueEvent(ctx context.Context, event common.Event)

	// Query the state of the coordinator
	GetCurrentState() State

	// Lifecycle
	Start()
	Stop()
}

type coordinator struct {
	// Mutex for thread-safe event processing (implements statemachine.Lockable)
	// Any functions passed to the state machine do not need to take the lock themselves
	// since the state machine takes the lock for the duration of the event processing.
	// Any functions that expose non atomic state outside of the coordinator must
	// take the read lock when called.
	sync.RWMutex

	ctx       context.Context
	cancelCtx context.CancelFunc
	started   bool

	/* State machine - using generic statemachine.StateMachineEventLoop */
	stateMachineEventLoop                      *statemachine.StateMachineEventLoop[State, *coordinator]
	activeCoordinatorNode                      string
	activeCoordinatorBlockHeight               uint64
	heartbeatIntervalsSinceStateChange         int
	heartbeatInterval                          common.Duration
	transactionsByID                           map[uuid.UUID]*transaction.CoordinatorTransaction
	pooledTransactions                         []*transaction.CoordinatorTransaction
	currentBlockHeight                         uint64
	activeCoordinatorsFlushPointsBySignerNonce map[string]*common.FlushPoint
	grapher                                    transaction.Grapher
	originatorNodePool                         []string // The (possibly changing) list of originator nodes

	/* Config */
	contractAddress                *pldtypes.EthAddress
	blockHeightTolerance           uint64
	closingGracePeriod             int // expressed as a multiple of heartbeat intervals
	requestTimeout                 common.Duration
	assembleTimeout                common.Duration
	nodeName                       string
	coordinatorSelectionBlockRange uint64
	maxInflightTransactions        int
	maxDispatchAhead               int

	/* Dependencies */
	domainAPI             components.DomainSmartContract
	transportWriter       transport.TransportWriter
	clock                 common.Clock
	engineIntegration     common.EngineIntegration
	txManager             components.TXManager
	keyManager            components.KeyManager
	publicTxManager       components.PublicTxManager
	syncPoints            syncpoints.SyncPoints
	dCtx                  components.DomainContext
	buildNullifiers       func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error)
	newPrivateTransaction func(context.Context, []*components.ValidatedTransaction) error
	coordinatorActive     func(contractAddress *pldtypes.EthAddress, coordinatorNode string)
	coordinatorIdle       func(contractAddress *pldtypes.EthAddress)
	heartbeatCtx          context.Context
	heartbeatCancel       context.CancelFunc
	metrics               metrics.DistributedSequencerMetrics

	/* Dispatch loop */
	dispatchQueue       chan *transaction.CoordinatorTransaction
	stopDispatchLoop    chan struct{}
	dispatchLoopStopped chan struct{}
	inFlightTxns        map[uuid.UUID]*transaction.CoordinatorTransaction
	inFlightMutex       *sync.Cond
}

func NewCoordinator(
	ctx context.Context,
	contractAddress *pldtypes.EthAddress,
	domainAPI components.DomainSmartContract,
	txManager components.TXManager,
	keyManager components.KeyManager,
	publicTxManager components.PublicTxManager,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	engineIntegration common.EngineIntegration,
	syncPoints syncpoints.SyncPoints,
	initialOriginatorNodePool []string,
	configuration *pldconf.SequencerConfig,
	nodeName string,
	metrics metrics.DistributedSequencerMetrics,
	dCtx components.DomainContext,
	buildNullifiers func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error),
	newPrivateTransaction func(context.Context, []*components.ValidatedTransaction) error,
	coordinatorActive func(contractAddress *pldtypes.EthAddress, coordinatorNode string),
	coordinatorIdle func(contractAddress *pldtypes.EthAddress),
) *coordinator {
	maxInflightTransactions := confutil.IntMin(configuration.MaxInflightTransactions, pldconf.SequencerMinimum.MaxInflightTransactions, *pldconf.SequencerDefaults.MaxInflightTransactions)
	coordinatorCtx, coordinatorCancelCtx := context.WithCancel(ctx)
	c := &coordinator{
		ctx:                                coordinatorCtx,
		cancelCtx:                          coordinatorCancelCtx,
		heartbeatIntervalsSinceStateChange: 0,
		transactionsByID:                   make(map[uuid.UUID]*transaction.CoordinatorTransaction),
		pooledTransactions:                 make([]*transaction.CoordinatorTransaction, 0, maxInflightTransactions),
		domainAPI:                          domainAPI,
		txManager:                          txManager,
		keyManager:                         keyManager,
		publicTxManager:                    publicTxManager,
		transportWriter:                    transportWriter,
		contractAddress:                    contractAddress,
		maxInflightTransactions:            maxInflightTransactions,
		grapher:                            transaction.NewGrapher(ctx),
		clock:                              clock,
		engineIntegration:                  engineIntegration,
		syncPoints:                         syncPoints,
		dCtx:                               dCtx,
		buildNullifiers:                    buildNullifiers,
		newPrivateTransaction:              newPrivateTransaction,
		coordinatorActive:                  coordinatorActive,
		coordinatorIdle:                    coordinatorIdle,
		nodeName:                           nodeName,
		metrics:                            metrics,
		stopDispatchLoop:                   make(chan struct{}, 1),
		dispatchLoopStopped:                make(chan struct{}),
	}
	c.originatorNodePool = make([]string, 0, len(initialOriginatorNodePool))
	for _, node := range initialOriginatorNodePool {
		c.updateOriginatorNodePool(node)
	}

	// Initialize the state machine event loop (state machine + event loop combined)
	c.initializeStateMachineEventLoop(State_Initial)

	c.maxDispatchAhead = confutil.IntMinIfPositive(configuration.MaxDispatchAhead, pldconf.SequencerMinimum.MaxDispatchAhead, *pldconf.SequencerDefaults.MaxDispatchAhead)
	c.inFlightMutex = sync.NewCond(&sync.Mutex{})
	c.inFlightTxns = make(map[uuid.UUID]*transaction.CoordinatorTransaction, c.maxDispatchAhead)
	c.dispatchQueue = make(chan *transaction.CoordinatorTransaction, maxInflightTransactions)

	// Configuration
	c.requestTimeout = confutil.DurationMin(configuration.RequestTimeout, pldconf.SequencerMinimum.RequestTimeout, *pldconf.SequencerDefaults.RequestTimeout)
	c.assembleTimeout = confutil.DurationMin(configuration.AssembleTimeout, pldconf.SequencerMinimum.AssembleTimeout, *pldconf.SequencerDefaults.AssembleTimeout)
	c.blockHeightTolerance = confutil.Uint64Min(configuration.BlockHeightTolerance, pldconf.SequencerMinimum.BlockHeightTolerance, *pldconf.SequencerDefaults.BlockHeightTolerance)
	c.closingGracePeriod = confutil.IntMin(configuration.ClosingGracePeriod, pldconf.SequencerMinimum.ClosingGracePeriod, *pldconf.SequencerDefaults.ClosingGracePeriod)
	c.maxInflightTransactions = confutil.IntMin(configuration.MaxInflightTransactions, pldconf.SequencerMinimum.MaxInflightTransactions, *pldconf.SequencerDefaults.MaxInflightTransactions)
	c.heartbeatInterval = confutil.DurationMin(configuration.HeartbeatInterval, pldconf.SequencerMinimum.HeartbeatInterval, *pldconf.SequencerDefaults.HeartbeatInterval)
	c.coordinatorSelectionBlockRange = confutil.Uint64Min(configuration.BlockRange, pldconf.SequencerMinimum.BlockRange, *pldconf.SequencerDefaults.BlockRange)

	return c
}

func (c *coordinator) Start() {
	// Start the state machine event loop
	go c.stateMachineEventLoop.Start(c.ctx)

	// Start dispatch queue loop
	go c.dispatchLoop(c.ctx)

	// Handle loopback messages to the same node in FIFO order without blocking the event loop
	c.transportWriter.StartLoopbackWriter(c.ctx)

	// Trigger the initial transition out of State_Initial
	c.QueueEvent(c.ctx, &CoordinatorCreatedEvent{})
	c.started = true
}

// GetCurrentState returns the current state of the coordinator.
// This method does NOT acquire a lock because:
//  1. Reading a single int (State enum) is atomic on all modern architectures
//  2. This method may be called from callbacks during event processing when the
//     coordinator's mutex is already held, which would cause a deadlock with RLock()
func (c *coordinator) GetCurrentState() State {
	return c.stateMachineEventLoop.GetCurrentState()
}

// A coordinator may be required to stop if this node has reached its capacity. The node may still need to
// have an active sequencer for the contract address since it may be the only originator that can honour dispatch
// requests from another coordinator, but this node is no longer acting as the coordinator.
func (c *coordinator) Stop() {
	if !c.started {
		return
	}
	log.L(context.Background()).Infof("stopping coordinator for contract %s", c.contractAddress.String())

	// Make Stop() idempotent - check if already stopped
	if c.stateMachineEventLoop.IsStopped() {
		return
	}

	// Stop the dispatch loop first so it is not blocked sending to the state machine's queue
	// (dispatch loop uses non-blocking queueing and checks stopDispatchLoop).
	c.stopDispatchLoop <- struct{}{}
	c.inFlightMutex.L.Lock()
	c.inFlightMutex.Signal()
	c.inFlightMutex.L.Unlock()
	<-c.dispatchLoopStopped

	// Stop the state machine event loop (will process final CoordinatorClosedEvent)
	c.stateMachineEventLoop.Stop()

	// Stop the loopback goroutine
	c.transportWriter.StopLoopbackWriter()

	// Cancel this coordinator's context which will cancel any timers started
	c.cancelCtx()
}

func (c *coordinator) sendHandoverRequest(ctx context.Context) {
	err := c.transportWriter.SendHandoverRequest(ctx, c.activeCoordinatorNode, c.contractAddress)
	if err != nil {
		log.L(ctx).Errorf("error sending handover request: %v", err)
	}
}

func (c *coordinator) propagateEventToTransaction(ctx context.Context, event transaction.Event) error {
	if txn := c.transactionsByID[event.GetTransactionID()]; txn != nil {
		return txn.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("ignoring event because transaction not known to this coordinator %s", event.GetTransactionID().String())
	}
	return nil
}

func (c *coordinator) propagateEventToAllTransactions(ctx context.Context, event common.Event) error {
	for _, txn := range c.transactionsByID {
		err := txn.HandleEvent(ctx, event)
		if err != nil {
			log.L(ctx).Errorf("error handling event %v for transaction %s: %v", event.Type(), txn.GetID().String(), err)
			return err
		}
	}
	return nil
}

func (c *coordinator) getTransactionsInStates(ctx context.Context, states []transaction.State) []*transaction.CoordinatorTransaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	log.L(ctx).Debugf("getting transactions in states: %+v", states)
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}

	log.L(ctx).Tracef("checking %d transactions for those in states: %+v", len(c.transactionsByID), states)
	matchingTxns := make([]*transaction.CoordinatorTransaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if matchingStates[txn.GetCurrentState()] {
			log.L(ctx).Debugf("found transaction %s in state %s", txn.GetID().String(), txn.GetCurrentState())
			matchingTxns = append(matchingTxns, txn)
		}
	}
	log.L(ctx).Tracef("%d transactions in states: %+v", len(matchingTxns), states)
	return matchingTxns
}

func ptrTo[T any](v T) *T {
	return &v
}
