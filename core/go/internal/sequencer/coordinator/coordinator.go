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
	"hash/fnv"
	"slices"
	"strconv"
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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// Coordinaor is the interface that consumers should use to interact with the coordinator.
// It should only expose the following types of function
// - lifecycle functions (currently just stop)
// - event queueing: this should be the only way consumers are able to modify the state of the coordinator
// - getter functions: these can return the internal state of the coordinator but must be thread safe
//
// GetActiveCoordinatorNode and UpdateOriginatorNodePool are currently exceptions to this pattern and need to be removed
type Coordinator interface {
	// Asynchronously update the state machine by queueing an event to be processed
	// This is the only interface by which consumers should update the state of the coordinator
	QueueEvent(ctx context.Context, event common.Event)

	// Query the state of the coordinator
	GetCurrentState() State

	// DANGEROUS- THIS FUNCTIONS MODIFIES STATE OUTSIDE OF THE STATE MACHINE WHEN initIfNoActiveCoordinator IS TRUE
	GetActiveCoordinatorNode(ctx context.Context, initIfNoActiveCoordinator bool) string
	// DANGEROUS- THIS FUNCTIONS MODIFIES STATE OUTSIDE OF THE STATE MACHINE
	UpdateOriginatorNodePool(ctx context.Context, originatorNode string)

	// Lifecycle
	Stop()
}

type coordinator struct {
	sync.RWMutex // Mutex for thread-safe event processing (implements statemachine.Lockable)

	ctx       context.Context
	cancelCtx context.CancelFunc

	/* State machine - using generic statemachine.ProcessorEventLoop */
	processorEventLoop                         *statemachine.ProcessorEventLoop[State, *coordinator]
	activeCoordinatorNode                      string
	activeCoordinatorBlockHeight               uint64
	heartbeatIntervalsSinceStateChange         int
	heartbeatInterval                          common.Duration
	transactionsByID                           map[uuid.UUID]*transaction.Transaction
	pooledTransactions                         []*transaction.Transaction
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
	domainAPI         components.DomainSmartContract
	transportWriter   transport.TransportWriter
	clock             common.Clock
	engineIntegration common.EngineIntegration
	txManager         components.TXManager
	syncPoints        syncpoints.SyncPoints
	readyForDispatch  func(context.Context, *transaction.Transaction)
	coordinatorActive func(contractAddress *pldtypes.EthAddress, coordinatorNode string)
	coordinatorIdle   func(contractAddress *pldtypes.EthAddress)
	heartbeatCtx      context.Context
	heartbeatCancel   context.CancelFunc
	metrics           metrics.DistributedSequencerMetrics

	/* Dispatch loop */
	dispatchQueue       chan *transaction.Transaction
	stopDispatchLoop    chan struct{}
	dispatchLoopStopped chan struct{}
	inFlightTxns        map[uuid.UUID]*transaction.Transaction
	inFlightMutex       *sync.Cond
}

func NewCoordinator(
	ctx context.Context,
	cancelCtx context.CancelFunc,
	contractAddress *pldtypes.EthAddress,
	domainAPI components.DomainSmartContract,
	txManager components.TXManager,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	engineIntegration common.EngineIntegration,
	syncPoints syncpoints.SyncPoints,
	configuration *pldconf.SequencerConfig,
	nodeName string,
	metrics metrics.DistributedSequencerMetrics,
	readyForDispatch func(context.Context, *transaction.Transaction),
	coordinatorActive func(contractAddress *pldtypes.EthAddress, coordinatorNode string),
	coordinatorIdle func(contractAddress *pldtypes.EthAddress),
) (*coordinator, error) {
	maxInflightTransactions := confutil.IntMin(configuration.MaxInflightTransactions, pldconf.SequencerMinimum.MaxInflightTransactions, *pldconf.SequencerDefaults.MaxInflightTransactions)
	c := &coordinator{
		ctx:                                ctx,
		cancelCtx:                          cancelCtx,
		heartbeatIntervalsSinceStateChange: 0,
		transactionsByID:                   make(map[uuid.UUID]*transaction.Transaction),
		pooledTransactions:                 make([]*transaction.Transaction, 0, maxInflightTransactions),
		domainAPI:                          domainAPI,
		txManager:                          txManager,
		transportWriter:                    transportWriter,
		contractAddress:                    contractAddress,
		maxInflightTransactions:            maxInflightTransactions,
		grapher:                            transaction.NewGrapher(ctx),
		clock:                              clock,
		engineIntegration:                  engineIntegration,
		syncPoints:                         syncPoints,
		readyForDispatch:                   readyForDispatch,
		coordinatorActive:                  coordinatorActive,
		coordinatorIdle:                    coordinatorIdle,
		nodeName:                           nodeName,
		metrics:                            metrics,
		stopDispatchLoop:                   make(chan struct{}),
		dispatchLoopStopped:                make(chan struct{}),
	}
	c.originatorNodePool = make([]string, 0)

	// Initialize the processor event loop (state machine + event loop combined)
	c.initializeProcessorEventLoop(State_Idle)

	c.maxDispatchAhead = confutil.IntMin(configuration.MaxDispatchAhead, pldconf.SequencerMinimum.MaxDispatchAhead, *pldconf.SequencerDefaults.MaxDispatchAhead)
	c.inFlightMutex = sync.NewCond(&sync.Mutex{})
	c.inFlightTxns = make(map[uuid.UUID]*transaction.Transaction, c.maxDispatchAhead)
	c.dispatchQueue = make(chan *transaction.Transaction, maxInflightTransactions)

	// Configuration
	c.requestTimeout = confutil.DurationMin(configuration.RequestTimeout, pldconf.SequencerMinimum.RequestTimeout, *pldconf.SequencerDefaults.RequestTimeout)
	c.assembleTimeout = confutil.DurationMin(configuration.AssembleTimeout, pldconf.SequencerMinimum.AssembleTimeout, *pldconf.SequencerDefaults.AssembleTimeout)
	c.blockHeightTolerance = confutil.Uint64Min(configuration.BlockHeightTolerance, pldconf.SequencerMinimum.BlockHeightTolerance, *pldconf.SequencerDefaults.BlockHeightTolerance)
	c.closingGracePeriod = confutil.IntMin(configuration.ClosingGracePeriod, pldconf.SequencerMinimum.ClosingGracePeriod, *pldconf.SequencerDefaults.ClosingGracePeriod)
	c.maxInflightTransactions = confutil.IntMin(configuration.MaxInflightTransactions, pldconf.SequencerMinimum.MaxInflightTransactions, *pldconf.SequencerDefaults.MaxInflightTransactions)
	c.heartbeatInterval = confutil.DurationMin(configuration.HeartbeatInterval, pldconf.SequencerMinimum.HeartbeatInterval, *pldconf.SequencerDefaults.HeartbeatInterval)
	c.coordinatorSelectionBlockRange = confutil.Uint64Min(configuration.BlockRange, pldconf.SequencerMinimum.BlockRange, *pldconf.SequencerDefaults.BlockRange)

	// Start the processor event loop
	go c.processorEventLoop.Start(ctx)

	// Start dispatch queue loop
	go c.dispatchLoop(ctx)

	// Handle loopback messages to the same node in FIFO order without blocking the event loop
	transportWriter.StartLoopbackWriter(ctx)

	return c, nil
}

// GetCurrentState returns the current state of the coordinator.
// This method does NOT acquire a lock because:
//  1. Reading a single int (State enum) is atomic on all modern architectures
//  2. This method may be called from callbacks during event processing when the
//     coordinator's mutex is already held, which would cause a deadlock with RLock()
func (c *coordinator) GetCurrentState() State {
	return c.processorEventLoop.GetCurrentState()
}

func (c *coordinator) GetActiveCoordinatorNode(ctx context.Context, initIfNoActiveCoordinator bool) string {
	if initIfNoActiveCoordinator && c.activeCoordinatorNode == "" {
		// If we don't yet have an active coordinator, select one based on the appropriate algorithm for the contract type
		activeCoordinator, err := c.selectActiveCoordinatorNode(ctx)
		if err != nil {
			log.L(ctx).Errorf("error selecting next active coordinator: %v", err)
			return ""
		}
		c.activeCoordinatorNode = activeCoordinator
	}
	return c.activeCoordinatorNode
}

// The name of this function gives the impression that it is modifying state but it is actually
// using the existing state as the input to the algorithm that chooses the current active coordinator.
// It returns the name of this node, but doesn't make it the active coordinator
func (c *coordinator) selectActiveCoordinatorNode(ctx context.Context) (string, error) {
	coordinatorNode := ""
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_STATIC {
		// E.g. Noto
		if c.domainAPI.ContractConfig().GetStaticCoordinator() == "" {
			return "", i18n.NewError(ctx, "static coordinator mode is configured but static coordinator node is not set")
		}
		log.L(ctx).Debugf("coordinator %s selected as next active coordinator in static coordinator mode", c.domainAPI.ContractConfig().GetStaticCoordinator())
		// If the static coordinator returns a fully qualified identity extract just the node name

		coordinator, err := pldtypes.PrivateIdentityLocator(c.domainAPI.ContractConfig().GetStaticCoordinator()).Node(ctx, false)
		if err != nil {
			log.L(ctx).Errorf("error getting static coordinator node id for %s: %s", c.domainAPI.ContractConfig().GetStaticCoordinator(), err)
			return "", err
		}
		coordinatorNode = coordinator
	} else if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_ENDORSER {
		// E.g. Pente
		// Make a fair choice about the next coordinator
		if len(c.originatorNodePool) == 0 {
			log.L(ctx).Warnf("no pool to select a coordinator from yet")
			return "", nil
		} else {
			c.RLock()
			defer c.RUnlock()
			// Round block number down to the nearest block range (e.g. block 1012, 1013, 1014 etc. all become 1000 for hashing)
			effectiveBlockNumber := c.currentBlockHeight - (c.currentBlockHeight % c.coordinatorSelectionBlockRange)

			// Take a numeric hash of the identities using the current block range
			h := fnv.New32a()
			h.Write([]byte(strconv.FormatUint(effectiveBlockNumber, 10)))
			coordinatorNode = c.originatorNodePool[int(h.Sum32())%len(c.originatorNodePool)]
			log.L(ctx).Debugf("coordinator %s selected based on hash modulus of the originator pool %+v", coordinatorNode, c.originatorNodePool)
		}
	} else if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_SENDER {
		// E.g. Zeto
		log.L(ctx).Debugf("coordinator %s selected as next active coordinator in originator coordinator mode", c.nodeName)
		coordinatorNode = c.nodeName
	}

	log.L(ctx).Debugf("selected active coordinator for contract %s: %s", c.contractAddress.String(), coordinatorNode)

	return coordinatorNode, nil
}

// The originator node pool is the list of all parties who should receive heartbeats, and who are
// eligible to be chosen as the coordinator for ContractConfig_COORDINATOR_ENDORSER domains such as Pente
func (c *coordinator) UpdateOriginatorNodePool(ctx context.Context, originatorNode string) {
	log.L(ctx).Debugf("updating originator node pool for contract %s with node %s", c.contractAddress.String(), originatorNode)
	c.Lock()
	defer c.Unlock()
	c.updateOriginatorNodePoolInternal(originatorNode)
}

// updateOriginatorNodePoolInternal is the internal version called from within event processing
// where the lock is already held. It must not acquire the lock.
func (c *coordinator) updateOriginatorNodePoolInternal(originatorNode string) {
	if !slices.Contains(c.originatorNodePool, originatorNode) {
		c.originatorNodePool = append(c.originatorNodePool, originatorNode)
	}
	if !slices.Contains(c.originatorNodePool, c.nodeName) {
		// As coordinator we should always be in the pool as it's used to select the next coordinator when necessary
		c.originatorNodePool = append(c.originatorNodePool, c.nodeName)
	}
	slices.Sort(c.originatorNodePool)
}

// A coordinator may be required to stop if this node has reached its capacity. The node may still need to
// have an active sequencer for the contract address since it may be the only originator that can honour dispatch
// requests from another coordinator, but this node is no longer acting as the coordinator.
func (c *coordinator) Stop() {
	log.L(context.Background()).Infof("stopping coordinator for contract %s", c.contractAddress.String())

	// Make Stop() idempotent - check if already stopped
	if c.processorEventLoop.IsStopped() {
		return
	}

	// Stop the processor event loop (will process final CoordinatorClosedEvent)
	c.processorEventLoop.Stop()

	// Stop the dispatch loop
	c.stopDispatchLoop <- struct{}{}
	<-c.dispatchLoopStopped

	// Stop the loopback goroutine
	c.transportWriter.StopLoopbackWriter()

	// Cancel this coordinator's context which will cancel any timers started
	c.cancelCtx()
}

func (c *coordinator) dispatchLoop(ctx context.Context) {
	defer close(c.dispatchLoopStopped)
	dispatchedAhead := 0 // Number of transactions we've dispatched without confirming they are in the state machine's in-flight list
	log.L(ctx).Debugf("coordinator dispatch loop started for contract %s", c.contractAddress.String())

	for {
		select {
		case tx := <-c.dispatchQueue:
			log.L(ctx).Debugf("coordinator pulled transaction %s from the dispatch queue. In-flight count: %d, dispatched ahead: %d, max dispatch ahead: %d", tx.GetID().String(), len(c.inFlightTxns), dispatchedAhead, c.maxDispatchAhead)

			c.inFlightMutex.L.Lock()

			// Too many in flight - wait for some to be confirmed
			for len(c.inFlightTxns)+dispatchedAhead >= c.maxDispatchAhead {
				c.inFlightMutex.Wait()
			}

			// Dispatch and then asynchronously update the state machine to State_Dispatched
			log.L(ctx).Debugf("submitting transaction %s for dispatch", tx.GetID().String())
			c.readyForDispatch(ctx, tx)

			// Dispatched transactions that result in a chained private transaction don't count towards max dispatch ahead
			if !tx.HasPreparedPrivateTransaction() {
				dispatchedAhead++
			}

			// Update the TX state machine
			c.QueueEvent(ctx, &transaction.DispatchedEvent{
				BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
					TransactionID: tx.GetID(),
				},
			})

			// We almost never need to wait for the state machine's event loop to process the update to State_Dispatched
			// but if we hit the max dispatch ahead limit after dispatching this transaction we do, because we can't be sure
			// in-flight will be accurate on the next loop round
			if len(c.inFlightTxns)+dispatchedAhead >= c.maxDispatchAhead {
				for c.inFlightTxns[tx.GetID()] == nil {
					c.inFlightMutex.Wait()
				}
				dispatchedAhead = 0
			}
			c.inFlightMutex.L.Unlock()
		case <-c.stopDispatchLoop:
			log.L(ctx).Debugf("coordinator dispatch loop for contract %s stopped", c.contractAddress.String())
			return
		}
	}
}

func (c *coordinator) sendHandoverRequest(ctx context.Context) {
	err := c.transportWriter.SendHandoverRequest(ctx, c.activeCoordinatorNode, c.contractAddress)
	if err != nil {
		log.L(ctx).Errorf("error sending handover request: %v", err)
	}
}

// Originators send only the delegated transactions that they believe the coordinator needs to know/be reminded about. Which transactions are
// included in this list depends on whether it is an intitial attempt or a scheduled retry, and whether individual delegation timeouts have
// been exceeded. This means that the coordinator cannot infer any dependency or ordering between transactions based on the list of transactions
// in the request.
//
// originator must be a fully qualified identity locator otherwise an error will be returned
func (c *coordinator) addToDelegatedTransactions(ctx context.Context, originator string, transactions []*components.PrivateTransaction) error {
	for _, txn := range transactions {

		if c.transactionsByID[txn.ID] != nil {
			log.L(ctx).Debugf("transaction %s already being coordinated", txn.ID.String())
			continue
		}

		if len(c.transactionsByID) >= c.maxInflightTransactions {
			// We'll rely on the fact that originators retry incomplete transactions periodically
			return i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, c.maxInflightTransactions)
		}

		// The newly delegated TX might be after the restart of an originator, for which we've already
		// instantiated a chained TX
		hasChainedTransaction, err := c.txManager.HasChainedTransaction(ctx, txn.ID)
		if err != nil {
			log.L(ctx).Errorf("error checking for chained transaction: %v", err)
			return err
		}
		if hasChainedTransaction {
			log.L(ctx).Debugf("chained transaction %s found", txn.ID.String())
		}

		newTransaction, err := transaction.NewTransaction(
			ctx,
			originator,
			txn,
			hasChainedTransaction,
			c.transportWriter,
			c.clock,
			c.QueueEvent,
			c.engineIntegration,
			c.syncPoints,
			c.requestTimeout,
			c.assembleTimeout,
			c.closingGracePeriod,
			c.grapher,
			c.metrics,
		)
		if err != nil {
			log.L(ctx).Errorf("error creating transaction: %v", err)
			return err
		}

		c.transactionsByID[txn.ID] = newTransaction
		c.metrics.IncCoordinatingTransactions()

		receivedEvent := &transaction.ReceivedEvent{}
		receivedEvent.TransactionID = txn.ID

		err = c.transactionsByID[txn.ID].HandleEvent(ctx, receivedEvent)
		if err != nil {
			log.L(ctx).Errorf("error handling ReceivedEvent for transaction %s: %v", txn.ID.String(), err)
			return err
		}
	}
	return nil
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

func (c *coordinator) getTransactionsInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	log.L(ctx).Debugf("getting transactions in states: %+v", states)
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}

	log.L(ctx).Tracef("checking %d transactions for those in states: %+v", len(c.transactionsByID), states)
	matchingTxns := make([]*transaction.Transaction, 0, len(c.transactionsByID))
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
