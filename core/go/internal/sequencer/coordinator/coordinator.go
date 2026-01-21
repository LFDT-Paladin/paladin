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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// TODO AM: this interface should end up with just QueueEvent, Stop and pure getters
type SeqCoordinator interface {
	// Asynchronously update the state machine by queueing an event to be processed.
	// This is the primary way to send events to the coordinator.
	QueueEvent(ctx context.Context, event common.Event)

	// Manage the state of the coordinator
	GetActiveCoordinatorNode(ctx context.Context, initIfNoActiveCoordinator bool) string
	SelectActiveCoordinatorNode(ctx context.Context) (string, error)
	UpdateOriginatorNodePool(ctx context.Context, originatorNode string)

	GetCurrentState() State
	GetTransactionByID(ctx context.Context, txID uuid.UUID) *transaction.Transaction

	// Lifecycle
	Stop()
}

// coordinator implements SeqCoordinator and statemachine.Subject
type coordinator struct {
	// Application context
	ctx       context.Context
	cancelCtx context.CancelFunc

	// State machine
	stateMachine *statemachine.EventLoopStateMachine[State, *coordinatorState, coordinatorStateReader, *stateMachineConfig, *stateMachineCallbacks, *coordinator]

	// Mutable state (protected by state.mu, modified only by StateUpdates)
	state *coordinatorState

	// State machine config and callbacks- to control what the state machine has access to
	smConfig    *stateMachineConfig
	smCallbacks *stateMachineCallbacks

	heartbeatLoop *heatbeatLoop
	dispatchLoop  *dispatchLoop

	domainAPI       components.DomainSmartContract
	transportWriter transport.TransportWriter

	coordinatorSelectionBlockRange uint64
	nodeName                       string
	contractAdress                 string
}

type stateMachineCallbacks struct {
	startHeartbeatLoop func(ctx context.Context)
	stopHeartbeatLoop  func()
	queueForDispatch   func(tx *transaction.Transaction)
	transportWriter    transport.TransportWriter // TWO functions- could be defined as a single function?
	coordinatorActive  func(contractAddress *pldtypes.EthAddress, coordinatorNode string)
	coordinatorIdle    func(contractAddress *pldtypes.EthAddress)
	grapher            transaction.Grapher
	metrics            metrics.DistributedSequencerMetrics
	txManager          components.TXManager
	newTransaction     func(ctx context.Context, originator string, txn *components.PrivateTransaction) (*transaction.Transaction, error)
}

type stateMachineConfig struct {
	contractAddress         *pldtypes.EthAddress
	blockHeightTolerance    uint64
	closingGracePeriod      int // expressed as a multiple of heartbeat intervals
	requestTimeout          common.Duration
	assembleTimeout         common.Duration
	nodeName                string
	maxInflightTransactions int

	coordinatorSelection prototk.ContractConfig_CoordinatorSelection

	// Dispatch loop control (has its own mutex) - TODO AM: this is what will need cleaning up
	inFlightTxns  map[uuid.UUID]*transaction.Transaction
	inFlightMutex *sync.Cond
}

// Implement statemachine.Subject interface

func (c *coordinator) GetStateMutator() *coordinatorState {
	return c.state
}

func (c *coordinator) GetStateReader() coordinatorStateReader {
	return c.state
}

func (c *coordinator) GetConfig() *stateMachineConfig {
	return c.smConfig
}

func (c *coordinator) GetCallbacks() *stateMachineCallbacks {
	return c.smCallbacks
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
	maxDispatchAhead := confutil.IntMin(configuration.MaxDispatchAhead, pldconf.SequencerMinimum.MaxDispatchAhead, *pldconf.SequencerDefaults.MaxDispatchAhead)

	// Create coordinator with separate state and context
	c := &coordinator{
		ctx:       ctx,
		cancelCtx: cancelCtx,

		domainAPI:       domainAPI,
		transportWriter: transportWriter,

		coordinatorSelectionBlockRange: confutil.Uint64Min(configuration.BlockRange, pldconf.SequencerMinimum.BlockRange, *pldconf.SequencerDefaults.BlockRange),
		nodeName:                       nodeName,
		contractAdress:                 contractAddress.String(),
	}

	c.heartbeatLoop = newHeartbeatLoop(
		contractAddress.String(),
		confutil.DurationMin(configuration.HeartbeatInterval, pldconf.SequencerMinimum.HeartbeatInterval, *pldconf.SequencerDefaults.HeartbeatInterval),
		c.QueueEvent,
	)

	c.dispatchLoop = newDispatchLoop(
		contractAddress.String(),
		maxDispatchAhead,
		maxInflightTransactions,
		c.QueueEvent,
		readyForDispatch,
	)

	c.state = newCoordinatorState()

	c.smConfig = &stateMachineConfig{
		contractAddress:         contractAddress,
		blockHeightTolerance:    confutil.Uint64Min(configuration.BlockHeightTolerance, pldconf.SequencerMinimum.BlockHeightTolerance, *pldconf.SequencerDefaults.BlockHeightTolerance),
		closingGracePeriod:      confutil.IntMin(configuration.ClosingGracePeriod, pldconf.SequencerMinimum.ClosingGracePeriod, *pldconf.SequencerDefaults.ClosingGracePeriod),
		requestTimeout:          confutil.DurationMin(configuration.RequestTimeout, pldconf.SequencerMinimum.RequestTimeout, *pldconf.SequencerDefaults.RequestTimeout),
		assembleTimeout:         confutil.DurationMin(configuration.AssembleTimeout, pldconf.SequencerMinimum.AssembleTimeout, *pldconf.SequencerDefaults.AssembleTimeout),
		nodeName:                nodeName,
		maxInflightTransactions: maxInflightTransactions,
		coordinatorSelection:    domainAPI.ContractConfig().GetCoordinatorSelection(),
		// Dispatch loop properties which aren't yet properly isolated TODO AM
		inFlightTxns:  c.dispatchLoop.inFlightTxns,
		inFlightMutex: c.dispatchLoop.inFlightMutex,
	}

	c.smCallbacks = &stateMachineCallbacks{
		startHeartbeatLoop: c.heartbeatLoop.start,
		stopHeartbeatLoop:  c.heartbeatLoop.stop,
		queueForDispatch:   c.dispatchLoop.addToQueue,
		transportWriter:    transportWriter,
		coordinatorActive:  coordinatorActive,
		coordinatorIdle:    coordinatorIdle,
		grapher:            transaction.NewGrapher(ctx), // TODO AM: double check that the grapher meets our expactations of the state machine model
		metrics:            metrics,
		txManager:          txManager,
	}

	c.smCallbacks.newTransaction = func(ctx context.Context, originator string, txn *components.PrivateTransaction) (*transaction.Transaction, error) {
		return transaction.NewTransaction(
			ctx,
			originator,
			txn,
			transportWriter,
			clock,
			c.QueueEvent,
			engineIntegration,
			syncPoints,
			c.smConfig.requestTimeout,
			c.smConfig.assembleTimeout,
			c.smConfig.closingGracePeriod,
			c.smCallbacks.grapher,
			metrics,
		)
	}

	// Initialize state machine (creates event loop)
	c.InitializeStateMachine(ctx, State_Idle)

	// Start event processing loop
	c.stateMachine.Start()

	// Start dispatch queue loop
	c.dispatchLoop.start(ctx)

	// Handle loopback messages to the same node in FIFO order without blocking the event loop
	transportWriter.StartLoopbackWriter(ctx)

	return c, nil
}

// TODO AM: undertsand this better and check it fits into the state machine model- it doesn't look like it does
func (c *coordinator) GetActiveCoordinatorNode(ctx context.Context, initIfNoActiveCoordinator bool) string {
	if initIfNoActiveCoordinator && c.state.GetActiveCoordinatorNode() == "" {
		// If we don't yet have an active coordinator, select one based on the appropriate algorithm for the contract type
		activeCoordinator, err := c.SelectActiveCoordinatorNode(ctx)
		if err != nil {
			log.L(ctx).Errorf("error selecting next active coordinator: %v", err)
			return ""
		}
		c.state.SetActiveCoordinatorNode(activeCoordinator)
	}
	return c.state.GetActiveCoordinatorNode()
}

// TODO AM: undertsand this better and check it fits into the state machine model- it doesn't look like it does
// this function is entirely read only- if it needs to be it can be an external function provided it takes the read lock
func (c *coordinator) SelectActiveCoordinatorNode(ctx context.Context) (string, error) {
	coordinatorNode := ""
	contractConfig := c.domainAPI.ContractConfig()

	switch contractConfig.GetCoordinatorSelection() {
	case prototk.ContractConfig_COORDINATOR_STATIC: // E.g. Noto
		if contractConfig.GetStaticCoordinator() == "" {
			return "", i18n.NewError(ctx, "static coordinator mode is configured but static coordinator node is not set")
		}
		log.L(ctx).Debugf("coordinator %s selected as next active coordinator in static coordinator mode", contractConfig.GetStaticCoordinator())
		// If the static coordinator returns a fully qualified identity extract just the node name
		coordinator, err := pldtypes.PrivateIdentityLocator(contractConfig.GetStaticCoordinator()).Node(ctx, false)
		if err != nil {
			log.L(ctx).Errorf("error getting static coordinator node id for %s: %s", contractConfig.GetStaticCoordinator(), err)
			return "", err
		}
		coordinatorNode = coordinator

	case prototk.ContractConfig_COORDINATOR_ENDORSER: // E.g. Pente
		// Make a fair choice about the next coordinator
		// c.smContext.originatorNodePoolMutex.RLock()
		// defer c.smContext.originatorNodePoolMutex.RUnlock()
		// if len(c.smContext.originatorNodePool) == 0 {
		// 	log.L(ctx).Warnf("no pool to select a coordinator from yet")
		// 	return "", nil
		// }
		// TODO AM: we are reading the state here (and below) inc without a lock- this must be fixed
		if len(c.state.GetOriginatorNodePool()) == 0 {
			log.L(ctx).Warnf("no pool to select a coordinator from yet")
			return "", nil
		}
		// Round block number down to the nearest block range (e.g. block 1012, 1013, 1014 etc. all become 1000 for hashing)
		effectiveBlockNumber := c.state.GetCurrentBlockHeight() - (c.state.GetCurrentBlockHeight() % c.coordinatorSelectionBlockRange)

		// Take a numeric hash of the identities using the current block range
		h := fnv.New32a()
		h.Write([]byte(strconv.FormatUint(effectiveBlockNumber, 10)))
		coordinatorNode = c.state.originatorNodePool[int(h.Sum32())%len(c.state.originatorNodePool)]
		log.L(ctx).Debugf("coordinator %s selected based on hash modulus of the originator pool %+v", coordinatorNode, c.state.originatorNodePool)

	case prototk.ContractConfig_COORDINATOR_SENDER: // E.g. Zeto
		log.L(ctx).Debugf("coordinator %s selected as next active coordinator in originator coordinator mode", c.smConfig.nodeName)
		coordinatorNode = c.nodeName
	}

	log.L(ctx).Debugf("selected active coordinator for contract %s: %s", c.smConfig.contractAddress.String(), coordinatorNode)
	return coordinatorNode, nil
}

// The originator node pool is the list of all parties who should receive heartbeats, and who are
// eligible to be chosen as the coordinator for ContractConfig_COORDINATOR_ENDORSER domains such as Pente
// TODO AM: again this is being called from outside the state machine but modifying state
// just calling the internal function for now but we will want to reimplement this as an event into the state machine to update this state
func (c *coordinator) UpdateOriginatorNodePool(ctx context.Context, originatorNode string) {
	updateOriginatorNodePoolInternal(ctx, c.state, c.smConfig, originatorNode)
}

func getTransactionsInStates(ctx context.Context, reader coordinatorStateReader, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	log.L(ctx).Debugf("getting transactions in states: %+v", states)
	matchingStates := make(map[transaction.State]bool)
	for _, s := range states {
		matchingStates[s] = true
	}

	allTxns := reader.GetAllTransactions()
	log.L(ctx).Tracef("checking %d transactions for those in states: %+v", len(allTxns), states)
	matchingTxns := make([]*transaction.Transaction, 0, len(allTxns))
	for _, txn := range allTxns {
		if matchingStates[txn.GetState()] {
			log.L(ctx).Debugf("found transaction %s in state %s", txn.ID.String(), txn.GetState())
			matchingTxns = append(matchingTxns, txn)
		}
	}
	log.L(ctx).Tracef("%d transactions in states: %+v", len(matchingTxns), states)
	return matchingTxns
}

// A coordinator may be required to stop if this node has reached its capacity. The node may still need to
// have an active sequencer for the contract address since it may be the only originator that can honour dispatch
// requests from another coordinator, but this node is no longer acting as the coordinator.
func (c *coordinator) Stop() {
	log.L(context.Background()).Infof("stopping coordinator for contract %s", c.contractAdress)

	// Make Stop() idempotent - make sure we've not already been stopped
	select {
	case <-c.stateMachine.Done():
		return
	default:
	}

	// Stop the event loop (this triggers OnStop which processes CoordinatorClosedEvent)
	c.stateMachine.Stop()

	// Stop the dispatch loop
	c.dispatchLoop.stop()
	<-c.dispatchLoop.stopped

	// Stop the loopback goroutine
	c.transportWriter.StopLoopbackWriter()

	// Cancel this coordinator's context which will cancel any timers started
	c.cancelCtx()
}

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (c *coordinator) GetCurrentState() State {
	return c.state.GetCurrentState()
}

// TODO AM: this is just not right
func (c *coordinator) GetTransactionByID(ctx context.Context, txID uuid.UUID) *transaction.Transaction {
	return c.state.GetTransactionByID(txID)
}
