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
	"hash/fnv"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

type SeqCoordinator interface {
	// Asynchronously update the state machine by queueing an event to be processed. Most
	// callers should use this interface.
	QueueEvent(ctx context.Context, event common.Event)

	// Synchronously update the state machine by processing this event. Primarily used for testing the state machine.
	ProcessEvent(ctx context.Context, event common.Event) error

	// Manage the state of the coordinator
	GetActiveCoordinatorNode(ctx context.Context, initIfNoActiveCoordinator bool) string
	SelectActiveCoordinatorNode(ctx context.Context) (string, error)
	GetCurrentState() State
	UpdateOriginatorNodePool(ctx context.Context, originatorNode string)

	// Transactions being sequencer (here or elsewhere)
	GetTransactionsReadyToDispatch(ctx context.Context) ([]*components.PrivateTransaction, error)
	GetTransactionByID(ctx context.Context, txID uuid.UUID) *transaction.Transaction

	// Lifecycle
	Stop()
}

type coordinator struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	/* State */
	stateMachine                               *StateMachine
	activeCoordinatorNode                      string
	activeCoordinatorBlockHeight               uint64
	heartbeatIntervalsSinceStateChange         int
	heartbeatInterval                          common.Duration
	transactionsByID                           map[uuid.UUID]*transaction.Transaction
	pooledTransactionsMutex                    sync.RWMutex
	pooledTransactions                         []*transaction.Transaction
	currentBlockHeight                         uint64
	activeCoordinatorsFlushPointsBySignerNonce map[string]*common.FlushPoint
	grapher                                    transaction.Grapher

	/* Config */
	contractAddress                *pldtypes.EthAddress
	blockHeightTolerance           uint64
	closingGracePeriod             int // expressed as a multiple of heartbeat intervals
	requestTimeout                 common.Duration
	assembleTimeout                common.Duration
	originatorNodePool             []string // The (possibly changing) list of originator nodes
	originatorNodePoolMutex        sync.RWMutex
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

	/*Algorithms*/
	transactionSelector TransactionSelector

	/* Event loop */
	coordinatorEvents chan common.Event
	stopEventLoop     chan struct{}
	eventLoopStopped  chan struct{}
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
		coordinatorEvents:                  make(chan common.Event, 50), // TODO >1 only required for sqlite coarse-grained locks. Should this be DB-dependent?
		stopEventLoop:                      make(chan struct{}),
		eventLoopStopped:                   make(chan struct{}),
		stopDispatchLoop:                   make(chan struct{}),
		dispatchLoopStopped:                make(chan struct{}),
	}
	c.originatorNodePool = make([]string, 0)
	c.InitializeStateMachine(State_Idle)
	c.transactionSelector = NewTransactionSelector(ctx, c)
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

	// Start event processing loop
	go c.eventLoop(ctx)

	// Start dispatch queue loop
	go c.dispatchLoop(ctx)

	// Handle loopback messages to the same node in FIFO order without blocking the event loop
	transportWriter.StartLoopbackWriter(ctx)

	return c, nil
}

func (c *coordinator) eventLoop(ctx context.Context) {
	defer close(c.eventLoopStopped)
	log.L(ctx).Debugf("coordinator event loop started for contract %s", c.contractAddress.String())
	for {
		select {
		case event := <-c.coordinatorEvents:
			log.L(ctx).Debugf("coordinator for contract %s pulled event from the queue: %s", c.contractAddress.String(), event.TypeString())
			err := c.ProcessEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("error processing event: %v", err)
			}
		case <-c.stopEventLoop:
			// Synchronously move the state machine to closed
			err := c.ProcessEvent(ctx, &CoordinatorClosedEvent{})
			if err != nil {
				// Log internal error
				err := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "error processing coordinator closed event for contract %s: %v", c.contractAddress.String(), err)
				log.L(ctx).Error(err)
			}
			log.L(ctx).Debugf("coordinator event loop stopped for contract %s", c.contractAddress.String())
			return
		}
	}
}

func (c *coordinator) dispatchLoop(ctx context.Context) {
	defer close(c.dispatchLoopStopped)
	dispatchedAhead := 0 // Number of transactions we've dispatched without confirming they are in the state machine's in-flight list
	log.L(ctx).Debugf("coordinator dispatch loop started for contract %s", c.contractAddress.String())

	for {
		select {
		case tx := <-c.dispatchQueue:
			log.L(ctx).Debugf("coordinator pulled transaction %s from the dispatch queue. In-flight count: %d, dispatched ahead: %d, max dispatch ahead: %d", tx.ID.String(), len(c.inFlightTxns), dispatchedAhead, c.maxDispatchAhead)

			c.inFlightMutex.L.Lock()

			// Too many in flight - wait for some to be confirmed
			for len(c.inFlightTxns)+dispatchedAhead >= c.maxDispatchAhead {
				c.inFlightMutex.Wait()
			}

			// Dispatch and then asynchronously update the state machine to State_Dispatched
			log.L(ctx).Debugf("submitting transaction %s for dispatch", tx.ID.String())
			c.readyForDispatch(ctx, tx)

			// Dispatched transactions that result in a chained private transaction don't count towards max dispatch ahead
			if tx.PreparedPrivateTransaction == nil {
				dispatchedAhead++
			}

			// Update the TX state machine
			c.QueueEvent(ctx, &transaction.DispatchedEvent{
				BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
					TransactionID: tx.ID,
				},
			})

			// We almost never need to wait for the state machine's event loop to process the update to State_Dispatched
			// but if we hit the max dispatch ahead limit after dispatching this transaction we do, because we can't be sure
			// in-flight will be accurate on the next loop round
			if len(c.inFlightTxns)+dispatchedAhead >= c.maxDispatchAhead {
				for c.inFlightTxns[tx.ID] == nil {
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

func (c *coordinator) GetActiveCoordinatorNode(ctx context.Context, initIfNoActiveCoordinator bool) string {
	if initIfNoActiveCoordinator && c.activeCoordinatorNode == "" {
		// If we don't yet have an active coordinator, select one based on the appropriate algorithm for the contract type
		activeCoordinator, err := c.SelectActiveCoordinatorNode(ctx)
		if err != nil {
			log.L(ctx).Errorf("error selecting next active coordinator: %v", err)
			return ""
		}
		c.activeCoordinatorNode = activeCoordinator
	}
	return c.activeCoordinatorNode
}

func (c *coordinator) SelectActiveCoordinatorNode(ctx context.Context) (string, error) {
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
			c.originatorNodePoolMutex.RLock()
			defer c.originatorNodePoolMutex.RUnlock()
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
	c.originatorNodePoolMutex.Lock()
	defer c.originatorNodePoolMutex.Unlock()
	if !slices.Contains(c.originatorNodePool, originatorNode) {
		c.originatorNodePool = append(c.originatorNodePool, originatorNode)
	}
	if !slices.Contains(c.originatorNodePool, c.nodeName) {
		// As coordinator we should always be in the pool as it's used to select the next coordinator when necessary
		c.originatorNodePool = append(c.originatorNodePool, c.nodeName)
	}
	slices.Sort(c.originatorNodePool)
}

// TODO consider renaming to setDelegatedTransactionsForOriginator to make it clear that we expect originators to include all inflight transactions in every delegation request and therefore this is
// a replace, not an add.  Need to finalize the decision about whether we expect the originator to include all inflight delegated transactions in every delegation request. Currently the code assumes we do so need to make the spec clear on that point and
// record a decision record to explain why.  Every  time we come back to this point, we will be tempted to reverse that decision so we need to make sure we have a record of the known consequences.
// originator must be a fully qualified identity locator otherwise an error will be returned
func (c *coordinator) addToDelegatedTransactions(ctx context.Context, originator string, transactions []*components.PrivateTransaction) error {
	var previousTransaction *transaction.Transaction
	for _, txn := range transactions {

		if len(c.transactionsByID) >= c.maxInflightTransactions {
			// We'll rely on the fact that originators retry incomplete transactions periodically
			return i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, c.maxInflightTransactions)
		}

		newTransaction, err := transaction.NewTransaction(
			ctx,
			originator,
			txn,
			c.transportWriter,
			c.clock,
			c.ProcessEvent,
			c.engineIntegration,
			c.syncPoints,
			c.requestTimeout,
			c.assembleTimeout,
			c.closingGracePeriod,
			c.grapher,
			c.metrics,
			c.AddTransactionToBackOfPool,
			c.queueForDispatch,
			func(ctx context.Context, t *transaction.Transaction, to, from transaction.State) {
				// TX state changed, check if we need to be selecting the next transaction for this sequencer
				//TODO the following logic should be moved to the state machine so that all the rules are in one place
				if c.stateMachine.currentState == State_Active {
					if from == transaction.State_Assembling || to == transaction.State_Pooled {
						err := c.selectNextTransactionToAssemble(ctx, &TransactionStateTransitionEvent{
							TransactionID: t.ID,
							From:          from,
							To:            to,
						})
						if err != nil {
							log.L(ctx).Errorf("error selecting next transaction after transaction %s moved from %s to %s: %v", t.ID.String(), from.String(), to.String(), err)
							//TODO figure out how to get this to the abend handler
						}
					}
				}

				// Prod the dispatch loop with an updated in-flight count. This may release new transactions for dispatch
				c.inFlightMutex.L.Lock()
				defer c.inFlightMutex.L.Unlock()
				clear(c.inFlightTxns)
				dispatchingTransactions := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Dispatched, transaction.State_Submitted, transaction.State_SubmissionPrepared})
				for _, txn := range dispatchingTransactions {
					if txn.PreparedPrivateTransaction == nil {
						// We don't count transactions the result in new private transactions
						c.inFlightTxns[txn.ID] = txn
					}
				}
				log.L(ctx).Debugf("coordinator has %d dispatching transactions", len(c.inFlightTxns))
				c.inFlightMutex.Signal()
			},
			func(ctx context.Context) {
				// TX cleaned up after confirmation & sufficient heartbeats
				delete(c.transactionsByID, txn.ID)
				c.metrics.DecCoordinatingTransactions()
				err := c.grapher.Forget(txn.ID)

				if err != nil {
					log.L(ctx).Errorf("error forgetting transaction %s: %v", txn.ID.String(), err)
				}
				log.L(ctx).Debugf("transaction %s cleaned up", txn.ID.String())
			},
		)
		if err != nil {
			log.L(ctx).Errorf("error creating transaction: %v", err)
			return err
		}

		if previousTransaction != nil {
			newTransaction.SetPreviousTransaction(ctx, previousTransaction)
			previousTransaction.SetNextTransaction(ctx, newTransaction)
		}
		c.transactionsByID[txn.ID] = newTransaction
		c.metrics.IncCoordinatingTransactions()
		previousTransaction = newTransaction

		receivedEvent := &transaction.ReceivedEvent{}
		receivedEvent.TransactionID = txn.ID

		// The newly delegated TX might be after the restart of an originator, for which we've already
		// instantiated a chained TX
		hasChainedTransaction, err := c.txManager.HasChainedTransaction(ctx, txn.ID)
		if err != nil {
			log.L(ctx).Errorf("error checking for chained transaction: %v", err)
			return err
		}
		if hasChainedTransaction {
			log.L(ctx).Debugf("chained transaction %s found", txn.ID.String())
			newTransaction.SetChainedTxInProgress()
		}
		err = c.transactionsByID[txn.ID].HandleEvent(ctx, receivedEvent)
		if err != nil {
			log.L(ctx).Errorf("error handling ReceivedEvent for transaction %s: %v", txn.ID.String(), err)
			return err
		}
	}
	return nil
}

func (c *coordinator) queueForDispatch(ctx context.Context, txn *transaction.Transaction) {
	c.dispatchQueue <- txn
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
			log.L(ctx).Errorf("error handling event %v for transaction %s: %v", event.Type(), txn.ID.String(), err)
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
		if matchingStates[txn.GetState()] {
			log.L(ctx).Debugf("found transaction %s in state %s", txn.ID.String(), txn.GetState())
			matchingTxns = append(matchingTxns, txn)
		}
	}
	log.L(ctx).Tracef("%d transactions in states: %+v", len(matchingTxns), states)
	return matchingTxns
}

// MRW TODO - is there a reason we need to find by nonce and not by TX ID?
func (c *coordinator) findTransactionBySignerNonce(ctx context.Context, signer *pldtypes.EthAddress, nonce uint64) *transaction.Transaction {
	//TODO this would be more efficient by maintaining a separate index but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	for _, txn := range c.transactionsByID {
		if txn != nil {
			log.L(ctx).Tracef("Tracked TX ID %s", txn.ID.String())
		}
		if txn != nil && txn.GetSignerAddress() != nil {
			log.L(ctx).Tracef("Tracked TX ID %s signer address '%s'", txn.ID.String(), txn.GetSignerAddress().String())
		}
		if txn.GetSignerAddress() != nil && *txn.GetSignerAddress() == *signer && txn.GetNonce() != nil && *(txn.GetNonce()) == nonce {
			return txn
		}
	}
	return nil
}

func (c *coordinator) confirmDispatchedTransaction(ctx context.Context, txId uuid.UUID, from *pldtypes.EthAddress, nonce uint64, hash pldtypes.Bytes32, revertReason pldtypes.HexBytes) (bool, error) {
	log.L(ctx).Debugf("we currently have %d transactions to handle, confirming that dispatched TX %s is in our list", len(c.transactionsByID), txId.String())

	// Confirming a transaction via its chained transaction, we won't hav a from address
	if from != nil {
		// First check whether it is one that we have been coordinating
		if dispatchedTransaction := c.findTransactionBySignerNonce(ctx, from, nonce); dispatchedTransaction != nil {
			if dispatchedTransaction.GetLatestSubmissionHash() == nil || *(dispatchedTransaction.GetLatestSubmissionHash()) != hash {
				// Is this not the transaction that we are looking for?
				// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
				// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
				log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected", dispatchedTransaction.ID.String())
			}
			event := &transaction.ConfirmedEvent{
				Hash:         hash,
				RevertReason: revertReason,
				Nonce:        nonce,
			}
			event.TransactionID = dispatchedTransaction.ID
			event.EventTime = time.Now()
			err := dispatchedTransaction.HandleEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.ID.String(), err)
				return false, err
			}
			return true, nil
		}
	}

	for _, dispatchedTransaction := range c.transactionsByID {
		if dispatchedTransaction.ID == txId {
			if dispatchedTransaction.GetLatestSubmissionHash() == nil {
				// The transaction created a chained private transaction so there is no hash to compare
				log.L(ctx).Debugf("transaction %s confirmed with nil dispatch hash (confirmed hash of chained TX %s)", dispatchedTransaction.ID.String(), hash.String())
			} else if *(dispatchedTransaction.GetLatestSubmissionHash()) != hash {
				// Is this not the transaction that we are looking for?
				// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
				// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
				log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected. Dispatch hash %s, confirmed hash %s", dispatchedTransaction.ID.String(), dispatchedTransaction.GetLatestSubmissionHash(), hash.String())
			}
			event := &transaction.ConfirmedEvent{
				Hash:         hash,
				RevertReason: revertReason,
				Nonce:        nonce,
			}
			event.TransactionID = txId
			event.EventTime = time.Now()

			log.L(ctx).Debugf("Confirming dispatched TX %s", txId.String())
			err := dispatchedTransaction.HandleEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.ID.String(), err)
				return false, err
			}
			return true, nil
		}
	}
	log.L(ctx).Infof("failed to find a transaction submitted by signer %s", from.String())
	return false, nil

}

func (c *coordinator) confirmMonitoredTransaction(_ context.Context, from *pldtypes.EthAddress, nonce uint64) {
	if flushPoint := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", from.String(), nonce)]; flushPoint != nil {
		//We do not remove the flushPoint from the list because there is a chance that the coordinator hasn't seen this confirmation themselves and
		// when they send us the next heartbeat, it will contain this FlushPoint so it would get added back into the list and we would not see the confirmation again
		flushPoint.Confirmed = true
	}
}

func ptrTo[T any](v T) *T {
	return &v
}

// A coordinator may be required to stop if this node has reached its capacity. The node may still need to
// have an active sequencer for the contract address since it may be the only originator that can honour dispatch
// requests from another coordinator, but this node is no longer acting as the coordinator.
func (c *coordinator) Stop() {
	log.L(context.Background()).Infof("stopping coordinator for contract %s", c.contractAddress.String())

	// Make Stop() idempotent - make sure we've not already been stopped
	select {
	case <-c.eventLoopStopped:
		return
	default:
	}

	// Stop the event and dispatch loops
	c.stopEventLoop <- struct{}{}
	c.stopDispatchLoop <- struct{}{}
	<-c.eventLoopStopped
	<-c.dispatchLoopStopped

	// Stop the loopback goroutine
	c.transportWriter.StopLoopbackWriter()

	// Cancel this coordinator's context which will cancel any timers started
	c.cancelCtx()
}

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (c *coordinator) GetCurrentState() State {
	return c.stateMachine.currentState
}
