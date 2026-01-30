/*
 * Copyright © 2024 Kaleido, Inc.
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

package sequencer

import (
	"context"
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	coordinatorTx "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/blockindexer"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newSequencerManagerForHandlerTesting returns a manager with syncPoints set so handlers that call WriteOrDistributeReceipts work.
func newSequencerManagerForHandlerTesting(t *testing.T, mocks *sequencerLifecycleTestMocks) *sequencerManager {
	sm := newSequencerManagerForTesting(t, mocks)
	sm.syncPoints = mocks.syncPoints
	return sm
}

// ---- HandleTransactionCollected ----

func TestHandleTransactionCollected_NoSequencer_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	// No domain at address -> LoadSequencer returns (nil, nil)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(nil, errors.New("not found")).Once()

	err := sm.HandleTransactionCollected(ctx, contractAddr.String(), contractAddr.String(), uuid.New())
	require.NoError(t, err)
}

func TestHandleTransactionCollected_SequencerPresent_QueuesCollectedEvent(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(ev interface{}) bool {
		collected, ok := ev.(*coordinatorTx.CollectedEvent)
		return ok && collected.TransactionID == txID
	})).Once()

	err := sm.HandleTransactionCollected(ctx, contractAddr.String(), contractAddr.String(), txID)
	require.NoError(t, err)
	mocks.coordinator.AssertExpectations(t)
}

// ---- HandleNonceAssigned ----

func TestHandleNonceAssigned_NoSequencer_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(nil, errors.New("not found")).Once()

	err := sm.HandleNonceAssigned(ctx, 1, contractAddr.String(), uuid.New())
	require.NoError(t, err)
}

func TestHandleNonceAssigned_GetTransactionByIDReturnsNil_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()

	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.Anything).Once()
	mocks.coordinator.EXPECT().GetTransactionByID(ctx, txID).Return(nil).Once()

	err := sm.HandleNonceAssigned(ctx, 1, contractAddr.String(), txID)
	require.Error(t, err)
	assert.Regexp(t, "transaction.*not found|internal", err.Error())
}

func TestHandleNonceAssigned_Success_QueuesEventAndSendsNonceAssigned(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	coordTx := coordinatorTx.NewMinimalCoordinatorTransactionForTest("origin-node", txID)

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(ev interface{}) bool {
		_, ok := ev.(*coordinatorTx.NonceAllocatedEvent)
		return ok
	})).Once()
	mocks.coordinator.EXPECT().GetTransactionByID(ctx, txID).Return(coordTx).Once()
	mocks.transportWriter.EXPECT().SendNonceAssigned(ctx, txID, "origin-node", mock.Anything, uint64(1)).Return(nil).Once()

	err := sm.HandleNonceAssigned(ctx, 1, contractAddr.String(), txID)
	require.NoError(t, err)
}

// ---- HandlePublicTXSubmission ----

func TestHandlePublicTXSubmission_Deploy_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	txID := uuid.New()
	tx := &pldapi.PublicTxWithBinding{PublicTx: &pldapi.PublicTx{To: nil}} // deploy

	err := sm.HandlePublicTXSubmission(ctx, dbTX, txID, tx)
	require.NoError(t, err)
}

// TestHandlePublicTXSubmission_NoSequencer_SameNode_ReturnsNil: when no sequencer exists (GetSmartContractByAddress returns error so LoadSequencer returns nil)
// and sender is same node, we don't call SendReliable and return nil.
func TestHandlePublicTXSubmission_NoSequencer_SameNode_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	dbTX := persistencemocks.NewDBTX(t)
	txID := uuid.New()
	hash := pldtypes.RandBytes32()
	tx := &pldapi.PublicTxWithBinding{
		PublicTx: &pldapi.PublicTx{
			To:              contractAddr,
			TransactionHash: &hash,
		},
		PublicTxBinding: pldapi.PublicTxBinding{
			TransactionSender:          "sender@test-node", // same node -> no SendReliable
			TransactionContractAddress: contractAddr.String(),
		},
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(nil, errors.New("not found")).Once()

	err := sm.HandlePublicTXSubmission(ctx, dbTX, txID, tx)
	require.NoError(t, err)
}

func TestHandlePublicTXSubmission_SequencerPresent_QueuesSubmittedAndSendsToOriginator(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	hash := pldtypes.RandBytes32()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	dbTX := persistencemocks.NewDBTX(t)
	tx := &pldapi.PublicTxWithBinding{
		PublicTx: &pldapi.PublicTx{
			To:              contractAddr,
			TransactionHash: &hash,
		},
		PublicTxBinding: pldapi.PublicTxBinding{
			TransactionSender:          "sender@test-node", // same node -> no SendReliable
			TransactionContractAddress: contractAddr.String(),
		},
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()

	coordTx := coordinatorTx.NewMinimalCoordinatorTransactionForTest("origin-node", txID)
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(ev interface{}) bool {
		sub, ok := ev.(*coordinatorTx.SubmittedEvent)
		return ok && sub.TransactionID == txID
	})).Once()
	mocks.coordinator.EXPECT().GetTransactionByID(ctx, txID).Return(coordTx).Once()
	mocks.transportWriter.EXPECT().SendTransactionSubmitted(ctx, txID, "origin-node", contractAddr, &hash).Return(nil).Once()

	err := sm.HandlePublicTXSubmission(ctx, dbTX, txID, tx)
	require.NoError(t, err)
}

// ---- HandleTransactionConfirmed ----

func TestHandleTransactionConfirmed_Deploy_SelectsActiveCoordinator(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	confirmed := &components.TxCompletion{
		ReceiptInput: components.ReceiptInput{
			TransactionID:   uuid.New(),
			ContractAddress: contractAddr,
		},
		PSC: nil,
	}

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.coordinator.EXPECT().SelectActiveCoordinatorNode(ctx).Return("", nil).Maybe()

	from := pldtypes.RandAddress()
	err := sm.HandleTransactionConfirmed(ctx, confirmed, from, nil)
	require.NoError(t, err)
}

func TestHandleTransactionConfirmed_Invoke_NotCoordinator_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	domainAPI := componentsmocks.NewDomainSmartContract(t)
	domainAPI.EXPECT().Address().Return(*contractAddr).Maybe()
	confirmed := &components.TxCompletion{
		ReceiptInput: components.ReceiptInput{TransactionID: uuid.New()},
		PSC:          domainAPI,
	}
	from := pldtypes.RandAddress()

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.coordinator.EXPECT().GetActiveCoordinatorNode(ctx, false).Return("other-node").Once()

	err := sm.HandleTransactionConfirmed(ctx, confirmed, from, nil)
	require.NoError(t, err)
}

func TestHandleTransactionConfirmed_Invoke_Coordinator_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	from := pldtypes.RandAddress()
	hash := pldtypes.RandBytes32()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	domainAPI := componentsmocks.NewDomainSmartContract(t)
	domainAPI.EXPECT().Address().Return(*contractAddr).Maybe()
	confirmed := &components.TxCompletion{
		ReceiptInput: components.ReceiptInput{
			TransactionID: txID,
			OnChain:       pldtypes.OnChainLocation{TransactionHash: hash},
		},
		PSC: domainAPI,
	}
	coordTx := coordinatorTx.NewMinimalCoordinatorTransactionForTest("origin-node", txID)
	nonce := pldtypes.HexUint64(2)

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.coordinator.EXPECT().GetActiveCoordinatorNode(ctx, false).Return("test-node").Once()
	mocks.coordinator.EXPECT().GetTransactionByID(ctx, txID).Return(coordTx).Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(ev interface{}) bool {
		e, ok := ev.(*coordinator.TransactionConfirmedEvent)
		return ok && e.TxID == txID
	})).Once()
	mocks.transportWriter.EXPECT().SendTransactionConfirmed(ctx, txID, "origin-node", contractAddr, &nonce, mock.Anything).Return(nil).Once()

	err := sm.HandleTransactionConfirmed(ctx, confirmed, from, &nonce)
	require.NoError(t, err)
}

// ---- HandleTransactionConfirmedByChainedTransaction ----

func TestHandleTransactionConfirmedByChainedTransaction_Deploy_SelectsActiveCoordinator(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	confirmed := &components.TxCompletion{
		ReceiptInput: components.ReceiptInput{
			TransactionID:   uuid.New(),
			ContractAddress: contractAddr,
		},
		PSC: nil,
	}

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.coordinator.EXPECT().SelectActiveCoordinatorNode(ctx).Return("", nil).Maybe()

	err := sm.HandleTransactionConfirmedByChainedTransaction(ctx, confirmed)
	require.NoError(t, err)
}

func TestHandleTransactionConfirmedByChainedTransaction_Invoke_Coordinator_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	hash := pldtypes.RandBytes32()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	domainAPI := componentsmocks.NewDomainSmartContract(t)
	domainAPI.EXPECT().Address().Return(*contractAddr).Maybe()
	confirmed := &components.TxCompletion{
		ReceiptInput: components.ReceiptInput{
			TransactionID: txID,
			OnChain:       pldtypes.OnChainLocation{TransactionHash: hash},
		},
		PSC: domainAPI,
	}
	coordTx := coordinatorTx.NewMinimalCoordinatorTransactionForTest("origin-node", txID)

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.coordinator.EXPECT().GetActiveCoordinatorNode(ctx, false).Return("test-node").Once()
	mocks.coordinator.EXPECT().GetTransactionByID(ctx, txID).Return(coordTx).Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(ev interface{}) bool {
		e, ok := ev.(*coordinator.TransactionConfirmedEvent)
		return ok && e.TxID == txID && e.Nonce == 0
	})).Once()
	mocks.transportWriter.EXPECT().SendTransactionConfirmed(ctx, txID, "origin-node", contractAddr, mock.MatchedBy(func(n *pldtypes.HexUint64) bool { return n == nil }), mock.Anything).Return(nil).Once()

	err := sm.HandleTransactionConfirmedByChainedTransaction(ctx, confirmed)
	require.NoError(t, err)
}

// ---- HandleTransactionFailed ----

// When no sequencer exists for the contract (GetSmartContractByAddress returns error), LoadSequencer returns (nil, nil).
// HandleTransactionFailed still distributes the built failure receipts via WriteOrDistributeReceiptsPostSubmit.
func TestHandleTransactionFailed_NoSequencer_DistributesReceipts(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForHandlerTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	dbTX := persistencemocks.NewDBTX(t)
	from := pldtypes.RandAddress()
	failures := []*components.PublicTxMatch{
		{
			PaladinTXReference: components.PaladinTXReference{
				TransactionID:              txID,
				TransactionSender:          "sender@node",
				TransactionContractAddress: contractAddr.String(),
			},
			IndexedTransactionNotify: &blockindexer.IndexedTransactionNotify{
				IndexedTransaction: pldapi.IndexedTransaction{
					Hash:        pldtypes.RandBytes32(),
					BlockNumber: 1,
					From:        from,
					To:          contractAddr,
					Nonce:       0,
				},
			},
		},
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(nil, errors.New("domain error")).Once()
	mocks.metrics.EXPECT().IncRevertedTransactions().Once()
	// WriteOrDistributeReceiptsPostSubmit filters to assembly reverts only (OnChain.Type == 0); these receipts have OnChainTransaction so they are filtered out
	mocks.syncPoints.EXPECT().WriteOrDistributeReceipts(ctx, dbTX, mock.MatchedBy(func(receipts []*components.ReceiptInputWithOriginator) bool {
		return len(receipts) == 0
	})).Return(nil).Once()

	err := sm.HandleTransactionFailed(ctx, dbTX, failures)
	require.NoError(t, err)
}

func TestHandleTransactionFailed_CoordinatorNotTracking_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForHandlerTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	dbTX := persistencemocks.NewDBTX(t)
	from := pldtypes.RandAddress()
	failures := []*components.PublicTxMatch{
		{
			PaladinTXReference: components.PaladinTXReference{
				TransactionID:              txID,
				TransactionSender:          "sender@node",
				TransactionContractAddress: contractAddr.String(),
			},
			IndexedTransactionNotify: &blockindexer.IndexedTransactionNotify{
				IndexedTransaction: pldapi.IndexedTransaction{
					Hash:        pldtypes.RandBytes32(),
					BlockNumber: 1,
					From:        from,
					To:          contractAddr,
					Nonce:       0,
				},
			},
		},
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.metrics.EXPECT().IncRevertedTransactions().Once()
	mocks.coordinator.EXPECT().GetTransactionByID(ctx, txID).Return(nil).Once()

	err := sm.HandleTransactionFailed(ctx, dbTX, failures)
	require.NoError(t, err)
}

func TestHandleTransactionFailed_FromNil_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForHandlerTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	dbTX := persistencemocks.NewDBTX(t)
	coordTx := coordinatorTx.NewMinimalCoordinatorTransactionForTest("origin-node", txID)
	failures := []*components.PublicTxMatch{
		{
			PaladinTXReference: components.PaladinTXReference{
				TransactionID:              txID,
				TransactionSender:          "sender@node",
				TransactionContractAddress: contractAddr.String(),
			},
			IndexedTransactionNotify: &blockindexer.IndexedTransactionNotify{
				IndexedTransaction: pldapi.IndexedTransaction{
					Hash:        pldtypes.RandBytes32(),
					BlockNumber: 1,
					From:        nil, // nil From -> error
					To:          contractAddr,
					Nonce:       0,
				},
			},
		},
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.metrics.EXPECT().IncRevertedTransactions().Once()
	mocks.coordinator.EXPECT().GetTransactionByID(ctx, txID).Return(coordTx).Once()
	// From is nil so we return error before QueueEvent or SendTransactionConfirmed

	err := sm.HandleTransactionFailed(ctx, dbTX, failures)
	require.Error(t, err)
	assert.Regexp(t, "From|nil", err.Error())
}

func TestHandleTransactionFailed_Success_CallsWriteOrDistributeReceipts(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForHandlerTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	dbTX := persistencemocks.NewDBTX(t)
	from := pldtypes.RandAddress()
	nonce := uint64(1)
	hash := pldtypes.RandBytes32()
	failures := []*components.PublicTxMatch{
		{
			PaladinTXReference: components.PaladinTXReference{
				TransactionID:              txID,
				TransactionSender:          "sender@node",
				TransactionContractAddress: contractAddr.String(),
			},
			IndexedTransactionNotify: &blockindexer.IndexedTransactionNotify{
				IndexedTransaction: pldapi.IndexedTransaction{
					Hash:        hash,
					BlockNumber: 1,
					From:        from,
					To:          contractAddr,
					Nonce:       nonce,
				},
				RevertReason: []byte("revert"),
			},
		},
	}

	coordTx := coordinatorTx.NewMinimalCoordinatorTransactionForTest("origin-node", txID)
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()
	mocks.metrics.EXPECT().IncRevertedTransactions().Once()
	mocks.coordinator.EXPECT().GetTransactionByID(ctx, txID).Return(coordTx).Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.Anything).Once()
	mocks.transportWriter.EXPECT().SendTransactionConfirmed(ctx, txID, "origin-node", contractAddr, mock.Anything, mock.Anything).Return(nil).Once()
	// WriteOrDistributeReceiptsPostSubmit filters to only assembly reverts (OnChain.Type == 0); on-chain failures are filtered out, so we get empty slice
	mocks.syncPoints.EXPECT().WriteOrDistributeReceipts(ctx, dbTX, mock.MatchedBy(func(receipts []*components.ReceiptInputWithOriginator) bool {
		return len(receipts) == 0
	})).Return(nil).Once()

	err := sm.HandleTransactionFailed(ctx, dbTX, failures)
	require.NoError(t, err)
}

// ---- BuildNullifiers ----

func TestBuildNullifiers_EmptyList_ReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().Transaction(ctx, mock.Anything).Run(func(_ context.Context, fn func(context.Context, persistence.DBTX) error) {
		_ = fn(ctx, nil)
	}).Return(nil).Once()

	nullifiers, err := sm.BuildNullifiers(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, nullifiers)
}

func TestBuildNullifiers_SkipsEntriesWithNilNullifierFields(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	stateID := "state1"
	distributions := []*components.StateDistributionWithData{
		{
			StateDistribution: components.StateDistribution{
				StateID:               stateID,
				IdentityLocator:       "id@test-node",
				NullifierAlgorithm:    nil, // skip
				NullifierVerifierType: nil,
				NullifierPayloadType:  nil,
			},
		},
	}

	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().Transaction(ctx, mock.Anything).Run(func(_ context.Context, fn func(context.Context, persistence.DBTX) error) {
		_ = fn(ctx, nil)
	}).Return(nil).Once()

	nullifiers, err := sm.BuildNullifiers(ctx, distributions)
	require.NoError(t, err)
	assert.Empty(t, nullifiers)
}

// ---- BuildNullifier ----

func TestBuildNullifier_NotLocalIdentity_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	algo := "algo"
	verifierType := "verifier"
	payloadType := "payload"
	s := &components.StateDistributionWithData{
		StateDistribution: components.StateDistribution{
			StateID:               "state1",
			IdentityLocator:       "other@other-node", // not local
			NullifierAlgorithm:    &algo,
			NullifierVerifierType: &verifierType,
			NullifierPayloadType:  &payloadType,
		},
		StateData: []byte("{}"),
	}

	kr := componentsmocks.NewKeyResolver(t)
	nullifier, err := sm.BuildNullifier(ctx, kr, s)
	require.Error(t, err)
	assert.Nil(t, nullifier)
	assert.Regexp(t, "local|nullifier", err.Error())
}

// ---- CallPrivateSmartContract ----

func TestCallPrivateSmartContract_GetSmartContractByAddressError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	call := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: "test-domain",
				To:     contractAddr,
			},
		},
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(nil, errors.New("not found")).Once()

	result, err := sm.CallPrivateSmartContract(ctx, call)
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestCallPrivateSmartContract_DomainMismatch_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	call := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: "wrong-domain",
				To:     contractAddr,
			},
		},
	}

	domainAPI := componentsmocks.NewDomainSmartContract(t)
	domainMock := componentsmocks.NewDomain(t)
	domainMock.EXPECT().Name().Return("actual-domain").Once()
	domainAPI.EXPECT().Domain().Return(domainMock).Once()
	domainAPI.EXPECT().Address().Return(*contractAddr).Maybe()

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(domainAPI, nil).Once()

	result, err := sm.CallPrivateSmartContract(ctx, call)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Regexp(t, "domain|mismatch", err.Error())
}

func TestCallPrivateSmartContract_InitCallError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	call := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: "test-domain",
				To:     contractAddr,
			},
		},
	}

	domainAPI := componentsmocks.NewDomainSmartContract(t)
	domainMock := componentsmocks.NewDomain(t)
	domainMock.EXPECT().Name().Return("test-domain").Maybe()
	domainAPI.EXPECT().Domain().Return(domainMock).Maybe()
	domainAPI.EXPECT().Address().Return(*contractAddr).Maybe()
	domainAPI.EXPECT().InitCall(ctx, call).Return(nil, errors.New("init call failed")).Once()

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(domainAPI, nil).Once()

	result, err := sm.CallPrivateSmartContract(ctx, call)
	require.Error(t, err)
	assert.Nil(t, result)
}

// ---- WriteOrDistributeReceiptsPostSubmit ----

func TestWriteOrDistributeReceiptsPostSubmit_FiltersAssemblyRevertsAndCallsSyncPoints(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForHandlerTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	txID := uuid.New()
	receipts := []*components.ReceiptInputWithOriginator{
		{
			Originator: "orig@node",
			ReceiptInput: components.ReceiptInput{
				TransactionID: txID,
				ReceiptType:   components.RT_FailedWithMessage,
				OnChain:       pldtypes.OnChainLocation{}, // Type == 0 -> assembly revert
			},
		},
		{
			Originator: "orig2@node",
			ReceiptInput: components.ReceiptInput{
				TransactionID: uuid.New(),
				ReceiptType:   components.RT_FailedOnChainWithRevertData,
				OnChain:       pldtypes.OnChainLocation{Type: pldtypes.OnChainTransaction}, // not assembly
			},
		},
	}

	mocks.syncPoints.EXPECT().WriteOrDistributeReceipts(ctx, dbTX, mock.MatchedBy(func(r []*components.ReceiptInputWithOriginator) bool {
		return len(r) == 1 && r[0].OnChain.Type == 0
	})).Return(nil).Once()

	err := sm.WriteOrDistributeReceiptsPostSubmit(ctx, dbTX, receipts)
	require.NoError(t, err)
}

// ---- mapPreparedTransaction ----

func TestMapPreparedTransaction_MapsPrivateTxToPreparedWithRefs(t *testing.T) {
	txID := uuid.New()
	domain := "test-domain"
	addr := pldtypes.RandAddress()
	inputID := pldtypes.MustParseHexBytes("0x01")
	readID := pldtypes.MustParseHexBytes("0x02")
	outputID := pldtypes.MustParseHexBytes("0x03")
	infoID := pldtypes.MustParseHexBytes("0x04")

	tx := &components.PrivateTransaction{
		ID:               txID,
		Domain:           domain,
		Address:          *addr,
		PreparedMetadata: []byte("{}"),
		PostAssembly: &components.TransactionPostAssembly{
			InputStates:  []*components.FullState{{ID: inputID}},
			ReadStates:   []*components.FullState{{ID: readID}},
			OutputStates: []*components.FullState{{ID: outputID}},
			InfoStates:   []*components.FullState{{ID: infoID}},
		},
		PreparedPublicTransaction: &pldapi.TransactionInput{},
	}

	pt := mapPreparedTransaction(tx)

	require.NotNil(t, pt)
	assert.Equal(t, txID, pt.ID)
	assert.Equal(t, domain, pt.Domain)
	assert.Equal(t, *addr, *pt.To)
	assert.Equal(t, []pldtypes.HexBytes{inputID}, pt.StateRefs.Spent)
	assert.Equal(t, []pldtypes.HexBytes{readID}, pt.StateRefs.Read)
	assert.Equal(t, []pldtypes.HexBytes{outputID}, pt.StateRefs.Confirmed)
	assert.Equal(t, []pldtypes.HexBytes{infoID}, pt.StateRefs.Info)
	assert.NotNil(t, pt.Transaction)
}
