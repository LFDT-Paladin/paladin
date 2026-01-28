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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
)

type assembleRequestFromCoordinator struct {
	coordinatorsBlockHeight int64
	stateLocksJSON          []byte
	requestID               uuid.UUID
	preAssembly             []byte
}

// Transaction tracks the state of a transaction that is being sent by the local node in originator state.
// It implements statemachine.Lockable; the state machine holds this lock for the duration of each ProcessEvent call.
// pt holds the private transaction; it is not embedded so that all modifications must go through this package.
type Transaction struct {
	sync.RWMutex
	stateMachine                     *StateMachine
	pt                               *components.PrivateTransaction
	engineIntegration                common.EngineIntegration
	transportWriter                  transport.TransportWriter
	queueEventForOriginator          func(context.Context, common.Event)
	currentDelegate                  string
	lastDelegatedTime                *common.Time
	latestAssembleRequest            *assembleRequestFromCoordinator
	latestFulfilledAssembleRequestID uuid.UUID
	latestPreDispatchRequestID       uuid.UUID
	signerAddress                    *pldtypes.EthAddress
	latestSubmissionHash             *pldtypes.Bytes32
	nonce                            *uint64
	metrics                          metrics.DistributedSequencerMetrics
}

func NewTransaction(
	ctx context.Context,
	pt *components.PrivateTransaction,
	transportWriter transport.TransportWriter,
	queueEventForOriginator func(context.Context, common.Event),
	engineIntegration common.EngineIntegration,
	metrics metrics.DistributedSequencerMetrics,
) (*Transaction, error) {
	if pt == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "cannot create transaction without private tx")
	}
	txn := &Transaction{
		pt:                      pt,
		engineIntegration:       engineIntegration,
		transportWriter:         transportWriter,
		queueEventForOriginator: queueEventForOriginator,
		metrics:                 metrics,
	}

	txn.initializeStateMachine(State_Initial)

	return txn, nil
}

func (t *Transaction) GetID() uuid.UUID {
	t.RLock()
	defer t.RUnlock()
	return t.pt.ID
}

// GetPrivateTransaction returns the private transaction for code where we really cannot do without the whole struct.
func (t *Transaction) GetPrivateTransaction() *components.PrivateTransaction {
	t.RLock()
	defer t.RUnlock()
	return t.pt
}

// GetStatus returns the transaction status for external use. Caller may call from any goroutine.
func (t *Transaction) GetStatus(ctx context.Context) components.PrivateTxStatus {
	t.RLock()
	defer t.RUnlock()
	if t.pt == nil {
		return components.PrivateTxStatus{TxID: "", Status: "unknown"}
	}
	return components.PrivateTxStatus{
		TxID:         t.pt.ID.String(),
		Status:       t.stateMachine.GetCurrentState().String(),
		LatestEvent:  t.stateMachine.GetLatestEvent(),
		Endorsements: t.getEndorsementStatus(ctx),
		Transaction:  t.pt,
	}
}

func (t *Transaction) GetHash(ctx context.Context) (*pldtypes.Bytes32, error) {
	t.RLock()
	defer t.RUnlock()
	return t.hashInternal(ctx)
}

// hashInternal contains the hashing logic; used internally and by the public Hash.
func (t *Transaction) hashInternal(ctx context.Context) (*pldtypes.Bytes32, error) {
	if t.pt == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "cannot hash transaction without PrivateTransaction")
	}
	if t.pt.PostAssembly == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "cannot hash transaction without PostAssembly")
	}

	log.L(ctx).Debugf("hashing transaction %s with %d signatures and %d endorsements", t.pt.ID.String(), len(t.pt.PostAssembly.Signatures), len(t.pt.PostAssembly.Endorsements))

	// MRW TODO MUST DO - it's not clear is a originator transaction hash if valid without any signatures or endorsements.
	// After assemble a Pente TX can have just the assembler's endorsement (not everyone else's), so comparing hashes with > 1 endorsements will fail
	// if len(t.pt.PostAssembly.Signatures) == 0 {
	// 	return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, " cannot hash transaction without at least one Signature")
	// }

	hash := sha3.NewLegacyKeccak256()
	for _, signature := range t.pt.PostAssembly.Signatures {
		hash.Write(signature.Payload)
	}
	var h32 pldtypes.Bytes32
	_ = hash.Sum(h32[0:0])
	return &h32, nil
}

func (t *Transaction) getEndorsementStatus(ctx context.Context) []components.PrivateTxEndorsementStatus {
	if t.pt == nil || t.pt.PostAssembly == nil {
		return nil
	}
	endorsementRequestStates := make([]components.PrivateTxEndorsementStatus, len(t.pt.PostAssembly.AttestationPlan))
	for i, attRequest := range t.pt.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				found := false
				endorsementRequestState := &components.PrivateTxEndorsementStatus{Party: party, EndorsementReceived: false}
				for _, endorsement := range t.pt.PostAssembly.Endorsements {
					log.L(ctx).Debugf("existing endorsement from party %s", endorsement.Verifier.Lookup)
					found = endorsement.Name == attRequest.Name &&
						party == endorsement.Verifier.Lookup &&
						attRequest.VerifierType == endorsement.Verifier.VerifierType
					if found {
						endorsementRequestState.EndorsementReceived = true
						break
					}
				}
				endorsementRequestStates[i] = *endorsementRequestState
			}
		}
	}
	return endorsementRequestStates
}

func ptrTo[T any](v T) *T {
	return &v
}

func (t *Transaction) GetCurrentState() State {
	t.RLock()
	defer t.RUnlock()
	return t.stateMachine.GetCurrentState()
}

func (t *Transaction) GetLatestEvent() string {
	t.RLock()
	defer t.RUnlock()
	return t.stateMachine.GetLatestEvent()
}

func (t *Transaction) GetSignerAddress() *pldtypes.EthAddress {
	t.RLock()
	defer t.RUnlock()
	return t.signerAddress
}

func (t *Transaction) GetLatestSubmissionHash() *pldtypes.Bytes32 {
	t.RLock()
	defer t.RUnlock()
	return t.latestSubmissionHash
}

func (t *Transaction) GetNonce() *uint64 {
	t.RLock()
	defer t.RUnlock()
	return t.nonce
}

func (t *Transaction) GetLastDelegatedTime() *common.Time {
	t.RLock()
	defer t.RUnlock()
	return t.lastDelegatedTime
}
