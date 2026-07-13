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
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/dependencytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/statevisibilitytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// signingIdentityState groups the coordinator's current signing key with a flag that tracks
// whether any transaction has consumed it since the last key rotation.
type signingIdentityState struct {
	value string
	used  bool // set when a transaction first retrieves the signing identity; cleared on key rotation
}

// Coordinator is the interface that consumers should use to interact with the coordinator.
type Coordinator interface {
	// Start initializes the coordinator from the contract config and begins the event loop and
	// dispatch goroutines. It must be called once after construction before any events are processed.
	Start(ctx context.Context)

	// Asynchronously update the state machine by queueing an event to be processed
	// These are the only interfaces by which consumers should update the state of the coordinator
	QueueEvent(ctx context.Context, event common.Event)
	TryQueueEvent(ctx context.Context, event common.Event) bool

	// Query the state of the coordinator
	GetCurrentState() State

	// WaitForDone blocks until the coordinator has stopped after context cancellation.
	WaitForDone(ctx context.Context)
}

type coordinator struct {
	// Mutex for thread-safe event processing (implements statemachine.Lockable)
	// Any functions passed to the state machine do not need to take the lock themselves
	// since the state machine takes the lock for the duration of the event processing.
	// Any functions that expose non atomic state outside of the coordinator must
	// take the read lock when called.
	sync.RWMutex

	started bool
	ctx     context.Context

	/* State machine - using generic statemachine.StateMachineEventLoop */
	stateMachineEventLoop              *statemachine.StateMachineEventLoop[State, *coordinator]
	currentActiveCoordinator           string
	heartbeatIntervalsSinceStateChange int
	heartbeatIntervalsSinceLastReceive int
	transactionsByID                   map[uuid.UUID]transaction.CoordinatorTransaction
	pooledTransactions                 []transaction.CoordinatorTransaction
	currentBlockHeight                 int64
	effectiveBlockHeight               uint64
	dependencyTracker                  dependencytracker.DependencyTracker
	grapher                            grapher.Grapher
	stateVisibilityTracker             statevisibilitytracker.StateVisibilityStore
	endorserCandidates                 []string       // ENDORSER mode only: candidate nodes for coordinator priority list and heartbeat fan-out
	originatorActivity                 map[string]int // STATIC/SENDER only: heartbeat-intervals since last delegation activity per originator node
	coordinatorPriorityList            []string       // priority-ordered list; index 0 is current active coordinator
	signingIdentity                    signingIdentityState

	// Handover request tracking
	pendingHandoverRequest *common.IdempotentRequest // idempotent request in flight while in State_Elect

	// Request/state timeout timers
	cancelRequestTimeout func() // cancels the pending request-nudge timer; armed once on Elect entry
	cancelStateTimeout   func() // cancels the pending give-up timer

	/* Config */
	contractAddress                *pldtypes.EthAddress
	blockHeightTolerance           uint64
	closingGracePeriod             int // expressed as a multiple of heartbeat intervals
	inactiveGracePeriod            int // expressed as a multiple of heartbeat intervals
	baseLedgerRevertRetryThreshold int
	assembleErrorRetryThreshhold   int
	requestTimeout                 time.Duration
	stateTimeout                   time.Duration
	nodeName                       string
	coordinatorSelectionBlockRange uint64
	maxInflightTransactions        int
	maxDispatchAhead               int
	coordinatorSelection           prototk.ContractConfig_CoordinatorSelection

	/* Dependencies */
	domainAPI             components.DomainSmartContract
	dsw                   components.DomainStateWriter
	components            components.AllComponents
	transportWriter       transport.TransportWriter
	clock                 common.Clock
	engineIntegration     common.EngineIntegration
	buildNullifiers       func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error)
	newPrivateTransaction func(context.Context, []*components.ValidatedTransaction) error
	syncPoints            syncpoints.SyncPoints
	metrics               metrics.DistributedSequencerMetrics
	notifyOriginator      func(ctx context.Context, event common.Event) // optional callback to push events to the co-located originator

	/* Dispatch loop */
	dispatchQueue      chan transaction.CoordinatorTransaction
	dispatchLoopCancel context.CancelFunc // non-nil iff this coordinator owns a running loop
	dispatchLoopDone   chan struct{}      // per-run done channel; nil = never started / already stopped+waited
	inFlightTxns       map[uuid.UUID]transaction.CoordinatorTransaction
	inFlightMutex      *sync.Cond
}

func NewCoordinator(
	contractAddress *pldtypes.EthAddress,
	domainAPI components.DomainSmartContract,
	dsw components.DomainStateWriter,
	allComponents components.AllComponents,
	buildNullifiers func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error),
	newPrivateTransaction func(context.Context, []*components.ValidatedTransaction) error,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	engineIntegration common.EngineIntegration,
	syncPoints syncpoints.SyncPoints,
	configuration *pldconf.SequencerConfig,
	nodeName string,
	metrics metrics.DistributedSequencerMetrics,
	notifyOriginator func(ctx context.Context, event common.Event),
	selectionConfig *common.CoordinatorSelectionConfig,
) *coordinator {
	dependencyTracker := dependencytracker.NewDependencyTracker()
	stateVisibilityTracker := statevisibilitytracker.NewStore()
	c := &coordinator{
		heartbeatIntervalsSinceStateChange: 0,
		transactionsByID:                   make(map[uuid.UUID]transaction.CoordinatorTransaction),
		domainAPI:                          domainAPI,
		dsw:                                dsw,
		components:                         allComponents,
		buildNullifiers:                    buildNullifiers,
		newPrivateTransaction:              newPrivateTransaction,
		transportWriter:                    transportWriter,
		contractAddress:                    contractAddress,
		dependencyTracker:                  dependencyTracker,
		stateVisibilityTracker:             stateVisibilityTracker,
		grapher:                            grapher.NewGrapher(dependencyTracker, stateVisibilityTracker, confutil.Uint64Min(configuration.BlockHeightTolerance, pldconf.SequencerMinimum.BlockHeightTolerance, *pldconf.SequencerDefaults.BlockHeightTolerance)),
		clock:                              clock,
		engineIntegration:                  engineIntegration,
		syncPoints:                         syncPoints,
		nodeName:                           nodeName,
		metrics:                            metrics,
		notifyOriginator:                   notifyOriginator,
	}

	// Configuration
	coordinatorEventQueueSize := confutil.IntMin(configuration.CoordinatorEventQueueSize, pldconf.SequencerMinimum.CoordinatorEventQueueSize, *pldconf.SequencerDefaults.CoordinatorEventQueueSize)
	coordinatorPriorityEventQueueSize := confutil.IntMin(configuration.CoordinatorPriorityEventQueueSize, pldconf.SequencerMinimum.CoordinatorPriorityEventQueueSize, *pldconf.SequencerDefaults.CoordinatorPriorityEventQueueSize)
	c.maxInflightTransactions = confutil.IntMin(configuration.MaxInflightTransactions, pldconf.SequencerMinimum.MaxInflightTransactions, *pldconf.SequencerDefaults.MaxInflightTransactions)
	c.maxDispatchAhead = confutil.IntMinIfPositive(configuration.MaxDispatchAhead, pldconf.SequencerMinimum.MaxDispatchAhead, *pldconf.SequencerDefaults.MaxDispatchAhead)
	c.requestTimeout = confutil.DurationMin(configuration.RequestTimeout, pldconf.SequencerMinimum.RequestTimeout, *pldconf.SequencerDefaults.RequestTimeout)
	c.stateTimeout = confutil.DurationMin(configuration.StateTimeout, pldconf.SequencerMinimum.StateTimeout, *pldconf.SequencerDefaults.StateTimeout)
	c.blockHeightTolerance = confutil.Uint64Min(configuration.BlockHeightTolerance, pldconf.SequencerMinimum.BlockHeightTolerance, *pldconf.SequencerDefaults.BlockHeightTolerance)
	c.closingGracePeriod = confutil.IntMin(configuration.ClosingGracePeriod, pldconf.SequencerMinimum.ClosingGracePeriod, *pldconf.SequencerDefaults.ClosingGracePeriod)
	c.inactiveGracePeriod = confutil.IntMin(configuration.InactiveGracePeriod, pldconf.SequencerMinimum.InactiveGracePeriod, *pldconf.SequencerDefaults.InactiveGracePeriod)
	c.baseLedgerRevertRetryThreshold = confutil.IntMin(configuration.BaseLedgerRevertRetryThreshold, pldconf.SequencerMinimum.BaseLedgerRevertRetryThreshold, *pldconf.SequencerDefaults.BaseLedgerRevertRetryThreshold)
	c.assembleErrorRetryThreshhold = confutil.IntMin(configuration.AssembleErrorRetryThreshold, pldconf.SequencerMinimum.AssembleErrorRetryThreshold, *pldconf.SequencerDefaults.AssembleErrorRetryThreshold)
	c.maxInflightTransactions = confutil.IntMin(configuration.MaxInflightTransactions, pldconf.SequencerMinimum.MaxInflightTransactions, *pldconf.SequencerDefaults.MaxInflightTransactions)
	c.coordinatorSelectionBlockRange = confutil.Uint64Min(configuration.BlockRange, pldconf.SequencerMinimum.BlockRange, *pldconf.SequencerDefaults.BlockRange)

	// Initialize coordinator selection state from pre-resolved config.
	c.coordinatorSelection = selectionConfig.Mode
	switch selectionConfig.Mode {
	case prototk.ContractConfig_COORDINATOR_STATIC:
		c.currentActiveCoordinator = selectionConfig.StaticCoordinator
	case prototk.ContractConfig_COORDINATOR_SENDER:
		c.currentActiveCoordinator = nodeName
	case prototk.ContractConfig_COORDINATOR_ENDORSER:
		c.endorserCandidates = selectionConfig.Endorsers
	}

	// Initialize the state machine event loop (state machine + event loop combined)
	c.initializeStateMachineEventLoop(State_Initial, coordinatorEventQueueSize, coordinatorPriorityEventQueueSize)

	c.originatorActivity = make(map[string]int)
	c.inFlightMutex = sync.NewCond(&sync.Mutex{})
	c.inFlightTxns = make(map[uuid.UUID]transaction.CoordinatorTransaction, c.maxDispatchAhead)
	c.pooledTransactions = make([]transaction.CoordinatorTransaction, 0, c.maxInflightTransactions)
	c.dispatchQueue = make(chan transaction.CoordinatorTransaction, c.maxInflightTransactions)

	return c
}

func (c *coordinator) Start(ctx context.Context) {
	if c.started {
		return
	}
	coordCtx := log.WithLogField(ctx, "role", "coordinator")
	c.ctx = coordCtx

	blockHeight := c.engineIntegration.GetBlockHeight(ctx)
	c.currentBlockHeight = blockHeight
	c.effectiveBlockHeight = common.ComputeEffectiveBlockHeight(uint64(blockHeight), c.coordinatorSelectionBlockRange)

	c.started = true

	context.AfterFunc(ctx, func() {
		// the dispatch loop may be waiting on this mutex when the context is cancelled - this wakes
		// it up so it may exit
		c.inFlightMutex.L.Lock()
		c.inFlightMutex.Broadcast()
		c.inFlightMutex.L.Unlock()
	})

	// Start the state machine event loop
	go c.stateMachineEventLoop.Start(coordCtx)

	// Handle loopback messages to the same node without blocking the event loop
	c.transportWriter.StartLoopbackWriter()

	// Trigger the initial transition out of State_Initial
	c.QueueEvent(coordCtx, &CoordinatorCreatedEvent{})
}

// GetCurrentState returns the current state of the coordinator.
// The state machine has its own mutex for protecting the current state variable.
func (c *coordinator) GetCurrentState() State {
	return c.stateMachineEventLoop.GetCurrentState()
}

func (c *coordinator) WaitForDone(ctx context.Context) {
	if !c.started {
		return
	}
	c.RLock()
	done := c.dispatchLoopDone
	c.RUnlock()

	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return
		}
	}
	c.stateMachineEventLoop.WaitForDone(ctx)
	c.transportWriter.WaitForDone(ctx)
}

func (c *coordinator) propagateEventToTransaction(ctx context.Context, event transaction.Event) error {
	if txn := c.transactionsByID[event.GetTransactionID()]; txn != nil {
		return txn.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("ignoring event %s because transaction %s not known to this coordinator", event.TypeString(), event.GetTransactionID().String())
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

func (c *coordinator) getTransactionsInStates(ctx context.Context, states []transaction.State) []transaction.CoordinatorTransaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	log.L(ctx).Debugf("getting transactions in states: %+v", states)
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}

	log.L(ctx).Tracef("checking %d transactions for those in states: %+v", len(c.transactionsByID), states)
	matchingTxns := make([]transaction.CoordinatorTransaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if matchingStates[txn.GetCurrentState()] {
			log.L(ctx).Debugf("found transaction %s in state %s", txn.GetID().String(), txn.GetCurrentState())
			matchingTxns = append(matchingTxns, txn)
		}
	}
	log.L(ctx).Tracef("%d transactions in states: %+v", len(matchingTxns), states)
	return matchingTxns
}

func (c *coordinator) getTransactionsNotInStates(ctx context.Context, states []transaction.State) []transaction.CoordinatorTransaction {
	log.L(ctx).Debugf("getting transactions not in states: %+v", states)
	excludedStates := make(map[transaction.State]bool)
	for _, state := range states {
		excludedStates[state] = true
	}

	log.L(ctx).Tracef("checking %d transactions for those not in states: %+v", len(c.transactionsByID), states)
	matchingTxns := make([]transaction.CoordinatorTransaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if !excludedStates[txn.GetCurrentState()] {
			log.L(ctx).Debugf("found transaction %s in state %s", txn.GetID().String(), txn.GetCurrentState())
			matchingTxns = append(matchingTxns, txn)
		}
	}
	log.L(ctx).Tracef("%d transactions not in states: %+v", len(matchingTxns), states)
	return matchingTxns
}
