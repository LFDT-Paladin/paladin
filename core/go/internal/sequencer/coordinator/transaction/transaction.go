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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// CoordinatorTransaction represents a transaction that is being coordinated by a contract sequencer agent in Coordinator state.
// It implements statemachine.Lockable; the state machine holds this lock for the duration of each ProcessEvent call.
// pt holds the private transaction; it is not embedded so that all modifications go through this package.
type CoordinatorTransaction struct {
	sync.RWMutex

	pt *components.PrivateTransaction

	stateMachine *StateMachine

	originator             string // The fully qualified identity of the originator e.g. "member1@node1"
	originatorNode         string // The node the originator is running on e.g. "node1"
	signerAddress          *pldtypes.EthAddress
	dynamicSigningIdentity bool // True if the signing identity isn't fixed by domain config or endorser constraints
	latestSubmissionHash   *pldtypes.Bytes32
	nonce                  *uint64
	revertReason           pldtypes.HexBytes
	revertTime             *pldtypes.Timestamp

	//TODO move the fields that are really just fine grained state info.  Move them into the stateMachine struct ( consider separate structs for each concrete state)
	heartbeatIntervalsSinceStateChange               int
	pendingAssembleRequest                           *common.IdempotentRequest
	cancelAssembleTimeoutSchedule                    func()                                          // Longer timeout for assembly to complete, before giving up and trying to assemble the next TX
	cancelAssembleRequestTimeoutSchedule             func()                                          // Short timeout for retry e.g. network blip
	cancelEndorsementRequestTimeoutSchedule          func()                                          // Short timeout for retry e.g. network blip
	cancelDispatchConfirmationRequestTimeoutSchedule func()                                          // Short timeout for retry e.g. network blip
	pendingEndorsementRequests                       map[string]map[string]*common.IdempotentRequest //map of attestationRequest names to a map of parties to a struct containing information about the active pending request
	pendingEndorsementsMutex                         sync.Mutex
	pendingPreDispatchRequest                        *common.IdempotentRequest
	chainedTxAlreadyDispatched                       bool
	latestError                                      string
	dependencies                                     *pldapi.TransactionDependencies

	//Configuration
	requestTimeout        common.Duration
	assembleTimeout       common.Duration
	errorCount            int
	finalizingGracePeriod int // number of heartbeat intervals that the transaction will remain in one of the terminal states ( Reverted or Confirmed) before it is removed from memory and no longer reported in heartbeats

	// Dependencies
	clock                    common.Clock
	transportWriter          transport.TransportWriter
	grapher                  Grapher
	engineIntegration        common.EngineIntegration
	syncPoints               syncpoints.SyncPoints
	queueEventForCoordinator func(context.Context, common.Event)
	metrics                  metrics.DistributedSequencerMetrics
	keyManager               components.KeyManager
	publicTxManager          components.PublicTxManager
	txManager                components.TXManager
	domainAPI                components.DomainSmartContract
	dCtx                     components.DomainContext
	nodeName                 string
	buildNullifiers          func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error)
	newPrivateTransaction    func(context.Context, []*components.ValidatedTransaction) error
}

func NewTransaction(
	ctx context.Context,
	originator string,
	pt *components.PrivateTransaction,
	hasChainedTransaction bool,
	nodeName string,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	queueEventForCoordinator func(context.Context, common.Event),
	engineIntegration common.EngineIntegration,
	syncPoints syncpoints.SyncPoints,
	requestTimeout,
	assembleTimeout common.Duration,
	finalizingGracePeriod int,
	grapher Grapher,
	metrics metrics.DistributedSequencerMetrics,
	keyManager components.KeyManager,
	publicTxManager components.PublicTxManager,
	txManager components.TXManager,
	domainAPI components.DomainSmartContract,
	dCtx components.DomainContext,
	buildNullifiers func(context.Context, []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error),
	newPrivateTransaction func(context.Context, []*components.ValidatedTransaction) error,
) (*CoordinatorTransaction, error) {
	_, originatorNode, err := pldtypes.PrivateIdentityLocator(originator).Validate(ctx, "", false)
	if err != nil {
		log.L(ctx).Errorf("error validating originator %s: %s", originator, err)
		return nil, err
	}
	txn := &CoordinatorTransaction{
		originator:                 originator,
		originatorNode:             originatorNode,
		pt:                         pt,
		transportWriter:            transportWriter,
		clock:                      clock,
		queueEventForCoordinator:   queueEventForCoordinator,
		engineIntegration:          engineIntegration,
		syncPoints:                 syncPoints,
		dynamicSigningIdentity:     true, // Assume no nonce protection for dispatch ordering until we determine otherwise
		requestTimeout:             requestTimeout,
		assembleTimeout:            assembleTimeout,
		finalizingGracePeriod:      finalizingGracePeriod,
		dependencies:               &pldapi.TransactionDependencies{},
		grapher:                    grapher,
		metrics:                    metrics,
		chainedTxAlreadyDispatched: hasChainedTransaction,
		keyManager:                 keyManager,
		publicTxManager:            publicTxManager,
		txManager:                  txManager,
		domainAPI:                  domainAPI,
		dCtx:                       dCtx,
		nodeName:                   nodeName,
		buildNullifiers:            buildNullifiers,
		newPrivateTransaction:      newPrivateTransaction,
	}
	txn.initializeStateMachine(State_Initial)
	grapher.Add(context.Background(), txn)
	return txn, nil
}

// This function is external but doesn't not need a lock as ints are atomic
func (t *CoordinatorTransaction) GetCurrentState() State {
	return t.stateMachine.GetCurrentState()
}

// These functions are all called externally and return data that can change so always take
// a read lock. A consumer could also take a read lock if they wanted to be certain that a group of
// read functions are atomic

func (t *CoordinatorTransaction) GetSignerAddress() *pldtypes.EthAddress {
	t.RLock()
	defer t.RUnlock()
	return t.signerAddress
}

func (t *CoordinatorTransaction) GetNonce() *uint64 {
	t.RLock()
	defer t.RUnlock()
	return t.nonce
}

func (t *CoordinatorTransaction) GetLatestSubmissionHash() *pldtypes.Bytes32 {
	t.RLock()
	defer t.RUnlock()
	return t.latestSubmissionHash
}

func (t *CoordinatorTransaction) GetRevertReason() pldtypes.HexBytes {
	t.RLock()
	defer t.RUnlock()
	return t.revertReason
}

func (t *CoordinatorTransaction) Originator() string {
	t.RLock()
	defer t.RUnlock()
	return t.originator
}

func (t *CoordinatorTransaction) GetErrorCount() int {
	t.RLock()
	defer t.RUnlock()
	return t.errorCount
}

func (t *CoordinatorTransaction) GetID() uuid.UUID {
	t.RLock()
	defer t.RUnlock()
	return t.pt.ID
}

func (t *CoordinatorTransaction) GetOutputStateIDs() []pldtypes.HexBytes {
	t.RLock()
	defer t.RUnlock()
	// We use the output states here not the OutputStatesPotential because it is not possible for another transaction
	// to spend a state unless it has been written to the state store and at that point we have the state ID
	outputStateIDs := make([]pldtypes.HexBytes, len(t.pt.PostAssembly.OutputStates))
	for i, outputState := range t.pt.PostAssembly.OutputStates {
		outputStateIDs[i] = outputState.ID
	}
	return outputStateIDs
}

func (t *CoordinatorTransaction) HasPreparedPrivateTransaction() bool {
	t.RLock()
	defer t.RUnlock()
	return t.pt.PreparedPrivateTransaction != nil
}
