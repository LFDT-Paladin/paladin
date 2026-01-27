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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type TransactionState string

const (
	TransactionState_Pooled                TransactionState = "TransactionState_Pooled"
	TransactionState_Assembled             TransactionState = "TransactionState_Assembled"
	TransactionState_ConfirmingForDispatch TransactionState = "TransactionState_ConfirmingForDispatch"
	TransactionState_Dispatched            TransactionState = "TransactionState_Dispatched"
	TransactionState_Submitted             TransactionState = "TransactionState_Submitted"
	TransactionState_Rejected              TransactionState = "TransactionState_Rejected"
	TransactionState_ConfirmedSuccess      TransactionState = "TransactionState_ConfirmedSuccess"
	TransactionState_ConfirmedReverted     TransactionState = "TransactionState_ConfirmedReverted"
)

// Transaction represents a transaction that is being coordinated by a contract sequencer agent in Coordinator state.
// It implements statemachine.Lockable; the processor holds this lock for the duration of each ProcessEvent call.
// pt holds the private transaction; it is not embedded so that all modifications go through this package.
type Transaction struct {
	sync.RWMutex

	pt *components.PrivateTransaction

	stateMachine *StateMachine

	originator           string // The fully qualified identity of the originator e.g. "member1@node1"
	originatorNode       string // The node the originator is running on e.g. "node1"
	signerAddress        *pldtypes.EthAddress
	latestSubmissionHash *pldtypes.Bytes32
	nonce                *uint64
	revertReason         pldtypes.HexBytes
	revertTime           *pldtypes.Timestamp

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
	clock              common.Clock
	transportWriter    transport.TransportWriter
	grapher            Grapher
	engineIntegration  common.EngineIntegration
	syncPoints         syncpoints.SyncPoints
	notifyOfTransition OnStateTransition
	eventHandler       func(context.Context, common.Event)
	metrics            metrics.DistributedSequencerMetrics
}

// TODO think about naming of this compared to the OnTransitionTo func in the state machine
type OnStateTransition func(ctx context.Context, transactionID uuid.UUID, to, from State) // function to be invoked when transitioning into this state.  Called after transitioning event has been applied and any actions have fired

func NewTransaction(
	ctx context.Context,
	originator string,
	pt *components.PrivateTransaction,
	hasChainedTransaction bool,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	eventHandler func(context.Context, common.Event),
	engineIntegration common.EngineIntegration,
	syncPoints syncpoints.SyncPoints,
	requestTimeout,
	assembleTimeout common.Duration,
	finalizingGracePeriod int,
	grapher Grapher,
	metrics metrics.DistributedSequencerMetrics,
	onStateTransition OnStateTransition,
) (*Transaction, error) {
	_, originatorNode, err := pldtypes.PrivateIdentityLocator(originator).Validate(ctx, "", false)
	if err != nil {
		log.L(ctx).Errorf("error validating originator %s: %s", originator, err)
		return nil, err
	}
	txn := &Transaction{
		originator:                 originator,
		originatorNode:             originatorNode,
		pt:                         pt,
		transportWriter:            transportWriter,
		clock:                      clock,
		eventHandler:               eventHandler,
		engineIntegration:          engineIntegration,
		syncPoints:                 syncPoints,
		requestTimeout:             requestTimeout,
		assembleTimeout:            assembleTimeout,
		finalizingGracePeriod:      finalizingGracePeriod,
		dependencies:               &pldapi.TransactionDependencies{},
		grapher:                    grapher,
		metrics:                    metrics,
		notifyOfTransition:         onStateTransition,
		chainedTxAlreadyDispatched: hasChainedTransaction,
	}
	txn.initializeStateMachine(State_Initial)
	grapher.Add(context.Background(), txn)
	return txn, nil
}

// This function is external but doesn't not need a lock as ints are atomic
func (t *Transaction) GetCurrentState() State {
	return t.stateMachine.CurrentState
}

// These functions are all called externally and return data that can change so always take
// a read lock. A consumer could also take a read lock if they wanted to be certain that a group of
// read functions are atomic

func (t *Transaction) GetSignerAddress() *pldtypes.EthAddress {
	t.RLock()
	defer t.RUnlock()
	return t.signerAddress
}

func (t *Transaction) GetNonce() *uint64 {
	t.RLock()
	defer t.RUnlock()
	return t.nonce
}

func (t *Transaction) GetLatestSubmissionHash() *pldtypes.Bytes32 {
	t.RLock()
	defer t.RUnlock()
	return t.latestSubmissionHash
}

func (t *Transaction) GetRevertReason() pldtypes.HexBytes {
	t.RLock()
	defer t.RUnlock()
	return t.revertReason
}

func (t *Transaction) Originator() string {
	t.RLock()
	defer t.RUnlock()
	return t.originator
}

func (t *Transaction) GetErrorCount() int {
	t.RLock()
	defer t.RUnlock()
	return t.errorCount
}

// GetPrivateTransaction returns the private transaction for code where we really cannot do without the whole struct.
// Where possible, consumers should use the getters for individual values which then become immutable outside of this struct as
// returning the pointer to the whole struct opens to the door to the possibility of modifications outside of the state machine.
// TODO: Ideally there would be an interface around *components.PrivateTransaction to allow consumers more complete read only
// access.
func (t *Transaction) GetPrivateTransaction() *components.PrivateTransaction {
	t.RLock()
	defer t.RUnlock()
	return t.pt
}

func (t *Transaction) GetID() uuid.UUID {
	t.RLock()
	defer t.RUnlock()
	return t.pt.ID
}

func (t *Transaction) GetDomain() string {
	t.RLock()
	defer t.RUnlock()
	return t.pt.Domain
}

func (t *Transaction) GetContractAddress() pldtypes.EthAddress {
	t.RLock()
	defer t.RUnlock()
	return t.pt.Address
}

func (t *Transaction) GetTransactionSpecification() *prototk.TransactionSpecification {
	t.RLock()
	defer t.RUnlock()
	return t.pt.PreAssembly.TransactionSpecification
}

func (t *Transaction) GetOriginalSender() string {
	t.RLock()
	defer t.RUnlock()
	return t.pt.PreAssembly.TransactionSpecification.From
}

func (t *Transaction) GetOutputStateIDs() []pldtypes.HexBytes {
	t.RLock()
	defer t.RUnlock()
	//We use the output states here not the OutputStatesPotential because it is not possible for another transaction
	// to spend a state unless it has been written to the state store and at that point we have the state ID
	outputStateIDs := make([]pldtypes.HexBytes, len(t.pt.PostAssembly.OutputStates))
	for i, outputState := range t.pt.PostAssembly.OutputStates {
		outputStateIDs[i] = outputState.ID
	}
	return outputStateIDs
}

func (t *Transaction) HasPreparedPrivateTransaction() bool {
	t.RLock()
	defer t.RUnlock()
	return t.pt.PreparedPrivateTransaction != nil
}

func (t *Transaction) HasPreparedPublicTransaction() bool {
	t.RLock()
	defer t.RUnlock()
	return t.pt.PreparedPublicTransaction != nil
}

func (t *Transaction) GetSigner() string {
	t.RLock()
	defer t.RUnlock()
	return t.pt.Signer
}

// SetSigner sets the private transaction's Signer. Used by the sequencer when preparing dispatch.
// TODO: THIS SHOULD NOT MODIFY OUTSIDE OF THE STATE MACHINE
func (t *Transaction) SetSigner(s string) {
	t.Lock()
	defer t.Unlock()
	t.pt.Signer = s
}

// TODO AM: I think this goes when the signer setting moves into the state machine
func (t *Transaction) GetPostAssembly() *components.TransactionPostAssembly {
	t.RLock()
	defer t.RUnlock()
	return t.pt.PostAssembly
}
