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
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// Coordinator is the interface that consumers should use to interact with the coordinator.
type Coordinator interface {
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

	ctx context.Context

	signingIdentity string

	/* State machine - using generic statemachine.StateMachineEventLoop */
	stateMachineEventLoop                      *statemachine.StateMachineEventLoop[State, *coordinator]
	activeCoordinatorNode                      string
	activeCoordinatorBlockHeight               uint64
	heartbeatIntervalsSinceStateChange         int
	heartbeatIntervalsSinceLastReceive         int
	transactionsByID                           map[uuid.UUID]transaction.CoordinatorTransaction
	pooledTransactions                         []transaction.CoordinatorTransaction
	currentBlockHeight                         uint64
	activeCoordinatorsFlushPointsBySignerNonce map[string]*common.SnapshotFlushPoint
	grapher                                    transaction.Grapher
	originatorNodePool                         []string // The (possibly changing) list of originator nodes

	/* Config */
	contractAddress                   *pldtypes.EthAddress
	blockHeightTolerance              uint64
	closingGracePeriod                int // expressed as a multiple of heartbeat intervals
	inactiveToIdleGracePeriod         int // expressed as a multiple of heartbeat intervals
	confirmedLockRetentionGracePeriod int // expressed as a multiple of heartbeat intervals
	baseLedgerRevertRetryThreshold    int
	assembleErrorRetryThreshhold      int
	requestTimeout                    time.Duration
	stateTimeout                      time.Duration
	nodeName                          string
	coordinatorSelectionBlockRange    uint64
	coordinatorFailoverIndex          int
	maxInflightTransactions           int
	maxDispatchAhead                  int

	/* Dependencies */
	domainAPI             components.DomainSmartContract
	dCtx                  components.DomainContext
	components            components.AllComponents
	transportWriter       transport.TransportWriter
	clock                 common.Clock
	engineIntegration     common.EngineIntegration
	buildNullifiers       func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error)
	newPrivateTransaction func(context.Context, []*components.ValidatedTransaction) error
	syncPoints            syncpoints.SyncPoints
	coordinatorActive     func(contractAddress *pldtypes.EthAddress, coordinatorNode string)
	coordinatorIdle       func(contractAddress *pldtypes.EthAddress)
	metrics               metrics.DistributedSequencerMetrics

	/* Dispatch loop */
	dispatchQueue       chan transaction.CoordinatorTransaction
	dispatchLoopStopped chan struct{}
	inFlightTxns        map[uuid.UUID]transaction.CoordinatorTransaction
	inFlightMutex       *sync.Cond

	/* Chained child tracking — maps parent transaction ID to the chained child transaction ID.
	   Protected by its own mutex so the dispatch loop can read/write without acquiring
	   individual transaction RWMutex locks (avoiding deadlocks with the event loop). */
	chainedChildStore transaction.ChainedChildStore
}

func NewCoordinator(
	ctx context.Context,
	contractAddress *pldtypes.EthAddress,
	domainAPI components.DomainSmartContract,
	dCtx components.DomainContext,
	allComponents components.AllComponents,
	buildNullifiers func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error),
	newPrivateTransaction func(context.Context, []*components.ValidatedTransaction) error,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	engineIntegration common.EngineIntegration,
	syncPoints syncpoints.SyncPoints,
	initialOriginatorNodePool []string,
	configuration *pldconf.SequencerConfig,
	nodeName string,
	metrics metrics.DistributedSequencerMetrics,
	coordinatorActive func(contractAddress *pldtypes.EthAddress, coordinatorNode string),
	coordinatorIdle func(contractAddress *pldtypes.EthAddress),
) (*coordinator, error) {
	coordCtx := log.WithLogField(ctx, "role", "coordinator")
	c := &coordinator{
		ctx:                                coordCtx,
		heartbeatIntervalsSinceStateChange: 0,
		transactionsByID:                   make(map[uuid.UUID]transaction.CoordinatorTransaction),
		domainAPI:                          domainAPI,
		dCtx:                               dCtx,
		components:                         allComponents,
		buildNullifiers:                    buildNullifiers,
		newPrivateTransaction:              newPrivateTransaction,
		transportWriter:                    transportWriter,
		contractAddress:                    contractAddress,
		grapher:                            transaction.NewGrapher(coordCtx),
		clock:                              clock,
		engineIntegration:                  engineIntegration,
		syncPoints:                         syncPoints,
		coordinatorActive:                  coordinatorActive,
		coordinatorIdle:                    coordinatorIdle,
		nodeName:                           nodeName,
		metrics:                            metrics,
		dispatchLoopStopped:                make(chan struct{}),
		chainedChildStore:                  transaction.NewChainedChildStore(),
	}
	c.originatorNodePool = make([]string, 0, len(initialOriginatorNodePool))
	for _, node := range initialOriginatorNodePool {
		c.updateOriginatorNodePool(node)
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
	c.inactiveToIdleGracePeriod = confutil.IntMin(configuration.InactiveToIdleGracePeriod, pldconf.SequencerMinimum.InactiveToIdleGracePeriod, *pldconf.SequencerDefaults.InactiveToIdleGracePeriod)
	c.confirmedLockRetentionGracePeriod = confutil.IntMin(configuration.ConfirmedLockRetentionGracePeriod, pldconf.SequencerMinimum.ConfirmedLockRetentionGracePeriod, *pldconf.SequencerDefaults.ConfirmedLockRetentionGracePeriod)
	c.baseLedgerRevertRetryThreshold = confutil.IntMin(configuration.BaseLedgerRevertRetryThreshold, pldconf.SequencerMinimum.BaseLedgerRevertRetryThreshold, *pldconf.SequencerDefaults.BaseLedgerRevertRetryThreshold)
	c.assembleErrorRetryThreshhold = confutil.IntMin(configuration.AssembleErrorRetryThreshold, pldconf.SequencerMinimum.AssembleErrorRetryThreshold, *pldconf.SequencerDefaults.AssembleErrorRetryThreshold)
	c.maxInflightTransactions = confutil.IntMin(configuration.MaxInflightTransactions, pldconf.SequencerMinimum.MaxInflightTransactions, *pldconf.SequencerDefaults.MaxInflightTransactions)
	c.coordinatorSelectionBlockRange = confutil.Uint64Min(configuration.BlockRange, pldconf.SequencerMinimum.BlockRange, *pldconf.SequencerDefaults.BlockRange)

	c.signingIdentity = fmt.Sprintf("domains.%s.submit.%s", c.contractAddress.String(), uuid.New())

	// Initialize the state machine event loop (state machine + event loop combined)
	c.initializeStateMachineEventLoop(State_Initial, coordinatorEventQueueSize, coordinatorPriorityEventQueueSize)

	c.inFlightMutex = sync.NewCond(&sync.Mutex{})
	c.inFlightTxns = make(map[uuid.UUID]transaction.CoordinatorTransaction, c.maxDispatchAhead)
	c.pooledTransactions = make([]transaction.CoordinatorTransaction, 0, c.maxInflightTransactions)
	c.dispatchQueue = make(chan transaction.CoordinatorTransaction, c.maxInflightTransactions)
	context.AfterFunc(ctx, func() {
		// the disptach loop may be waiting on this mutex when the context is cancelled- this wakes
		// it up so it may exit
		c.inFlightMutex.L.Lock()
		c.inFlightMutex.Broadcast()
		c.inFlightMutex.L.Unlock()
	})

	if err := c.initializeOriginatorNodePoolFromContractConfig(coordCtx); err != nil {
		return nil, err
	}

	// Start the state machine event loop
	go c.stateMachineEventLoop.Start(coordCtx)

	// Start dispatch queue loop
	go c.dispatchLoop(coordCtx)

	// Handle loopback messages to the same node without blocking the event loop
	transportWriter.StartLoopbackWriter()

	// Trigger the initial transition out of State_Initial
	c.QueueEvent(coordCtx, &CoordinatorCreatedEvent{})

	return c, nil
}

// GetCurrentState returns the current state of the coordinator.
// TODO This method cannot acquire a lock because this method may be called from callbacks during event processing when the
// coordinator's mutex is already held, which would cause a deadlock with RLock(). Currently it is always called in a thread
// safe way, but as it is not guaranteed to be in the future this needs more refactoring.
func (c *coordinator) GetCurrentState() State {
	return c.stateMachineEventLoop.GetCurrentState()
}

func (c *coordinator) WaitForDone(ctx context.Context) {
	select {
	case <-c.dispatchLoopStopped:
	case <-ctx.Done():
		return
	}
	c.stateMachineEventLoop.WaitForDone(ctx)
	c.transportWriter.WaitForDone(ctx)
}

func (c *coordinator) sendHandoverRequest(ctx context.Context) {
	err := c.transportWriter.SendHandoverRequest(ctx, c.activeCoordinatorNode, c.contractAddress)
	if err != nil {
		log.L(ctx).Errorf("error sending handover request: %v", err)
	}
}

func (c *coordinator) initializeOriginatorNodePoolFromContractConfig(ctx context.Context) error {
	contractConfig := c.domainAPI.ContractConfig()
	if contractConfig.GetCoordinatorSelection() != prototk.ContractConfig_COORDINATOR_ENDORSER {
		return nil
	}
	candidates := contractConfig.GetCoordinatorEndorserCandidates()
	if len(candidates) == 0 {
		log.L(ctx).Warnf("endorser coordinator mode for contract %s has no configured candidates; runtime originator updates will populate the pool", c.contractAddress.String())
		return nil
	}

	c.originatorNodePool = make([]string, 0, len(candidates))
	for _, locator := range candidates {
		_, node, err := pldtypes.PrivateIdentityLocator(locator).Validate(ctx, "", false)
		if err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgSequencerInvalidEndorserCandidate, locator)
		}
		c.originatorNodePool = append(c.originatorNodePool, node)
	}
	slices.Sort(c.originatorNodePool)
	log.L(ctx).Debugf("initialized originator node pool from coordinator endorser candidates: %+v", c.originatorNodePool)
	return nil
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

func ptrTo[T any](v T) *T {
	return &v
}
