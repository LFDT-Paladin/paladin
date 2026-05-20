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
package transaction

import (
	"context"
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/dependencytracker"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type CoordinatorTransaction interface {
	HandleEvent(ctx context.Context, event common.Event) error
	GetID() uuid.UUID
	GetCurrentState() State
	HasDispatchedPublicTransaction() bool
	GetSnapshot(ctx context.Context) (*common.SnapshotPooledTransaction, *common.SnapshotDispatchedTransaction, *common.SnapshotConfirmedTransaction)
	GetPrivateTransaction() *components.PrivateTransaction
}

// coordinatorTransaction represents a transaction that is being coordinated by a contract sequencer agent in Coordinator state.
// It implements statemachine.Lockable; the state machine holds this lock for the duration of each ProcessEvent call.
// pt holds the private transaction; it is not embedded so that all modifications go through this package.
type coordinatorTransaction struct {
	sync.RWMutex

	pt           *components.PrivateTransaction
	stateMachine *StateMachine

	// immutable properties of the transaction
	originator                 string // The fully qualified identity of the originator e.g. "member1@node1"
	originatorNode             string // The node the originator is running on e.g. "node1"
	nodeName                   string // The local node coordinating this transaction
	domainSigningIdentity      string // Used if an endorsement constraint doesn't stipulate a specific endorser must submit
	coordinatorSigningIdentity string
	submitterSelection         prototk.ContractConfig_SubmitterSelection // The selection of submitter for the transaction

	// mutable fields that state machine actions will change
	signerAddress                      *pldtypes.EthAddress
	latestSubmissionHash               *pldtypes.Bytes32
	nonce                              *uint64
	revertReason                       pldtypes.HexBytes
	decodedRevertReason                string
	revertOnChain                      *pldtypes.OnChainLocation
	revertCount                        int
	lastCanRetryRevert                 bool
	assembleErrorCount                 int
	confirmedLocksReleased             bool
	heartbeatIntervalsSinceStateChange int
	stateEntryTime                     time.Time

	pendingAssembleRequest       *common.IdempotentRequest
	cancelRequestTimeoutSchedule func()                                          // Short timeout for retry e.g. network blip
	cancelStateTimeoutSchedule   func()                                          // Timeout for state completion before repooling
	pendingEndorsementRequests   map[string]map[string]*common.IdempotentRequest //map of attestationRequest names to a map of parties to a struct containing information about the active pending request
	pendingPreDispatchRequest    *common.IdempotentRequest

	//Configuration
	requestTimeout                    time.Duration
	stateTimeout                      time.Duration
	finalizingGracePeriod             int // number of heartbeat intervals that the transaction will remain in one of the terminal states ( Reverted or Confirmed) before it is removed from memory and no longer reported in heartbeats
	confirmedLockRetentionGracePeriod int // number of heartbeat intervals after confirmation before we clear in-memory state locks
	baseLedgerRevertRetryThreshold    int
	assembleErrorRetryThreshhold      int // this is for rare errors (not assembly reverts, but assemble outright failed at the originator)

	// Dependencies
	clock                             common.Clock
	transportWriter                   transport.TransportWriter
	grapher                           grapher.Grapher
	dependencyTracker                 dependencytracker.DependencyTracker
	engineIntegration                 common.EngineIntegration
	syncPoints                        syncpoints.SyncPoints
	components                        components.AllComponents
	domainAPI                         components.DomainSmartContract
	dCtx                              components.DomainContext
	queueEventForCoordinator          func(context.Context, common.Event)
	coordinatorTransactionHandleEvent func(context.Context, uuid.UUID, common.Event) error
	getCoordinatorTransactionState    func(context.Context, uuid.UUID) (State, bool)
	metrics                           metrics.DistributedSequencerMetrics
}

func NewTransaction(ctx context.Context,
	originator string,
	originatorNode string,
	nodeName string,
	pt *components.PrivateTransaction,
	coordinatorSigningIdentity string,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	queueEventForCoordinator func(context.Context, common.Event),
	coordinatorTransactionHandleEvent func(context.Context, uuid.UUID, common.Event) error,
	getCoordinatorTransactionState func(context.Context, uuid.UUID) (State, bool),
	engineIntegration common.EngineIntegration,
	syncPoints syncpoints.SyncPoints,
	allComponents components.AllComponents,
	domainAPI components.DomainSmartContract,
	dCtx components.DomainContext,
	requestTimeout,
	stateTimeout time.Duration,
	finalizingGracePeriod int,
	confirmedLockRetentionGracePeriod int,
	baseLedgerRevertRetryThreshold int,
	assembleErrorRetryThreshhold int,
	grapher grapher.Grapher,
	dependencyTracker dependencytracker.DependencyTracker,
	metrics metrics.DistributedSequencerMetrics,
) CoordinatorTransaction {
	return newTransaction(
		ctx,
		originator,
		originatorNode,
		nodeName,
		pt,
		coordinatorSigningIdentity,
		transportWriter,
		clock,
		queueEventForCoordinator,
		coordinatorTransactionHandleEvent,
		getCoordinatorTransactionState,
		engineIntegration,
		syncPoints,
		allComponents,
		domainAPI,
		dCtx,
		requestTimeout,
		stateTimeout,
		finalizingGracePeriod,
		confirmedLockRetentionGracePeriod,
		baseLedgerRevertRetryThreshold,
		assembleErrorRetryThreshhold,
		grapher,
		dependencyTracker,
		metrics,
	)
}

func newTransaction(
	ctx context.Context,
	originator string,
	originatorNode string,
	nodeName string,
	pt *components.PrivateTransaction,
	coordinatorSigningIdentity string,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	queueEventForCoordinator func(context.Context, common.Event),
	coordinatorTransactionHandleEvent func(context.Context, uuid.UUID, common.Event) error,
	getCoordinatorTransactionState func(context.Context, uuid.UUID) (State, bool),
	engineIntegration common.EngineIntegration,
	syncPoints syncpoints.SyncPoints,
	allComponents components.AllComponents,
	domainAPI components.DomainSmartContract,
	dCtx components.DomainContext,
	requestTimeout,
	stateTimeout time.Duration,
	finalizingGracePeriod int,
	confirmedLockRetentionGracePeriod int,
	baseLedgerRevertRetryThreshold int,
	assembleErrorRetryThreshhold int,
	grapher grapher.Grapher,
	dependencyTracker dependencytracker.DependencyTracker,
	metrics metrics.DistributedSequencerMetrics,
) *coordinatorTransaction {
	txCtx := log.WithLogField(ctx, "txID", pt.ID.String())

	txn := &coordinatorTransaction{
		originator:                        originator,
		originatorNode:                    originatorNode,
		nodeName:                          nodeName,
		pt:                                pt,
		transportWriter:                   transportWriter,
		clock:                             clock,
		queueEventForCoordinator:          queueEventForCoordinator,
		coordinatorTransactionHandleEvent: coordinatorTransactionHandleEvent,
		getCoordinatorTransactionState:    getCoordinatorTransactionState,
		engineIntegration:                 engineIntegration,
		syncPoints:                        syncPoints,
		components:                        allComponents,
		domainAPI:                         domainAPI,
		dCtx:                              dCtx,
		domainSigningIdentity:             domainAPI.Domain().FixedSigningIdentity(),
		coordinatorSigningIdentity:        coordinatorSigningIdentity,
		submitterSelection:                domainAPI.ContractConfig().GetSubmitterSelection(),
		requestTimeout:                    requestTimeout,
		stateTimeout:                      stateTimeout,
		finalizingGracePeriod:             finalizingGracePeriod,
		confirmedLockRetentionGracePeriod: confirmedLockRetentionGracePeriod,
		baseLedgerRevertRetryThreshold:    baseLedgerRevertRetryThreshold,
		assembleErrorRetryThreshhold:      assembleErrorRetryThreshhold,
		grapher:                           grapher,
		dependencyTracker:                 dependencyTracker,
		metrics:                           metrics,
	}

	// Set up chained dependencies carried from the parent coordinator's grapher.
	// Only retain dependencies that are still known in the grapher; unknown = assumed finalized.
	if pt.PreAssembly != nil && len(pt.PreAssembly.ChainedDependsOn) > 0 {
		for _, depID := range pt.PreAssembly.ChainedDependsOn {
			state, ok := txn.getCoordinatorTransactionState(txCtx, depID)
			if !ok {
				// It is possible for a chained transaction to be created referencing dependencies that the original
				// grapher knew about at creation time, but for the chained transactions of those dependencies to have
				// already been finalized and removed from memory, by the time the chained transaction begins to be sequenced.
				// We don't have anyway of knowing whether the transaction was finalized as a success or failure at this point;
				// however, failing chained transactions who's dependencies have failed is an optimisation to allow their
				// reassembly in the original coordinator to occur as quickly as possible when we know that failure for this
				// transaction is inevitable, even if it hasn't occured yet. So if we submit this transaction to the base ledger
				// and it fails because the prereq transaction was not confirmed on chain, it will just take a little longer
				// for that failure to get back to the original coordinator.
				// Log at warning level because it is helpful to be able to identity this condition.
				log.L(txCtx).Warnf("Dependency %s not found in grapher for TX %s, assuming finalized", depID, pt.ID)
				continue
			}
			txn.dependencyTracker.GetChainedDeps().AddPrerequisites(txCtx, pt.ID, depID)
			if state == State_Initial || state == State_PreAssembly_Blocked || state == State_Pooled {
				txn.dependencyTracker.GetChainedDeps().AddUnassembledDependencies(txCtx, pt.ID, depID)
			}
		}
	}

	txn.initializeStateMachine(State_Initial)

	return txn
}

func (t *coordinatorTransaction) GetCurrentState() State {
	// the state machine has its own lock for current state so we don't need to take the whole transaction lock
	return t.stateMachine.GetCurrentState()
}

// These functions are all called externally and return data that can change so always take
// a read lock. A consumer could also take a read lock if they wanted to be certain that a group of
// read functions are atomic

func (t *coordinatorTransaction) GetID() uuid.UUID {
	t.RLock()
	defer t.RUnlock()
	return t.pt.ID
}

func (t *coordinatorTransaction) HasDispatchedPublicTransaction() bool {
	t.RLock()
	defer t.RUnlock()
	return t.pt.PreparedPublicTransaction != nil &&
		t.pt.PreAssembly.TransactionSpecification.Intent == prototk.TransactionSpecification_SEND_TRANSACTION
}

func (t *coordinatorTransaction) GetPrivateTransaction() *components.PrivateTransaction {
	t.RLock()
	defer t.RUnlock()
	return t.pt
}
