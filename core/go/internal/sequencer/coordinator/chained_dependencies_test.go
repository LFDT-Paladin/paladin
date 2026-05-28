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
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func TestChainedDependencies(t *testing.T) {
	suite.Run(t, new(ChainedDependenciesSuite))
}

type ChainedDependenciesSuite struct {
	suite.Suite

	ctx        context.Context
	c          *coordinator
	mocks      *CoordinatorDependencyMocks
	syncPoints *syncpoints.MockSyncPoints
	builder    *CoordinatorBuilderForTesting
	originator string
	done       func()

	txBuilders map[uuid.UUID]*testutil.PrivateTransactionBuilderForTesting
	txns       map[uuid.UUID]*components.PrivateTransaction
}

func (s *ChainedDependenciesSuite) SetupTest() {
	s.ctx = context.Background()
	s.originator = "sender@senderNode"

	s.builder = NewCoordinatorBuilderForTesting(s.T(), State_Idle)
	mockDomain := componentsmocks.NewDomain(s.T())
	mockDomain.On("FixedSigningIdentity").Return("")
	s.builder.GetDomainAPI().On("Domain").Return(mockDomain)
	s.builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	s.builder.GetDomainAPI().On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)
		tx.PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil).Maybe()
	s.builder.GetSequencerManager().On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	config := s.builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1)
	s.builder.OverrideSequencerConfig(config)
	s.c, s.mocks, s.done = s.builder.Build(s.ctx)

	s.syncPoints = s.mocks.SyncPoints.(*syncpoints.MockSyncPoints)
	s.syncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	s.syncPoints.On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	s.txBuilders = make(map[uuid.UUID]*testutil.PrivateTransactionBuilderForTesting)
	s.txns = make(map[uuid.UUID]*components.PrivateTransaction)
}

func (s *ChainedDependenciesSuite) TearDownTest() {
	s.done()
}

// ---------- helpers ----------

func (s *ChainedDependenciesSuite) newTx(chainedDeps ...uuid.UUID) uuid.UUID {
	s.T().Helper()
	b := testutil.NewPrivateTransactionBuilderForTesting().
		Address(s.builder.GetContractAddress()).
		Originator(s.originator).
		NumberOfRequiredEndorsers(1)
	pa := &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			From:   s.originator,
			Intent: prototk.TransactionSpecification_PREPARE_TRANSACTION,
		},
	}
	if len(chainedDeps) > 0 {
		b.ChainedDependencies(chainedDeps...)
		pa.ChainedDependsOn = chainedDeps
	}
	b.PreAssembly(pa)
	txn := b.BuildSparse()
	txn.PreAssembly.TransactionSpecification.TransactionId = txn.ID.String()
	s.txBuilders[txn.ID] = b
	s.txns[txn.ID] = txn
	return txn.ID
}

func (s *ChainedDependenciesSuite) delegate(txIDs ...uuid.UUID) {
	s.T().Helper()
	pts := make([]*components.PrivateTransaction, len(txIDs))
	for i, id := range txIDs {
		pts[i] = s.txns[id]
	}
	s.c.QueueEvent(s.ctx, &TransactionsDelegatedEvent{
		FromNode: "testNode", Originator: s.originator, Transactions: pts,
	})
}

func (s *ChainedDependenciesSuite) progressToReadyForDispatch(txIDs ...uuid.UUID) {
	s.T().Helper()
	rec := &s.mocks.SentMessageRecorder.SentMessageRecorder
	for _, id := range txIDs {
		b := s.txBuilders[id]

		s.sync()
		s.c.QueueEvent(s.ctx, &transaction.AssembleSuccessEvent{
			BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{TransactionID: id},
			RequestID:            rec.AssembleKeyForTx(id),
			PostAssembly:         b.BuildPostAssembly(),
			PreAssembly:          b.BuildPreAssembly(),
		})

		endorser := b.GetEndorserIdentityLocator(0)
		s.sync()
		s.c.QueueEvent(s.ctx, &transaction.EndorsedEvent{
			BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{TransactionID: id},
			RequestID:            rec.EndorseKeyForTxAndParty(id, endorser),
			Endorsement:          b.BuildEndorsement(0),
		})

		s.sync()
		s.c.QueueEvent(s.ctx, &transaction.DispatchRequestApprovedEvent{
			BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{TransactionID: id},
			RequestID:            rec.DispatchConfirmKeyForTx(id),
		})

		s.assertInState(transaction.State_Ready_For_Dispatch, id)
	}
}

func (s *ChainedDependenciesSuite) dispatch(txIDs ...uuid.UUID) {
	s.T().Helper()
	for _, id := range txIDs {
		s.c.QueueEvent(s.ctx, &transaction.DispatchedEvent{
			BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{TransactionID: id},
		})
	}
	s.sync()
	dispatched := s.c.getTransactionsInStates(s.ctx, []transaction.State{transaction.State_Dispatched})
	s.Require().GreaterOrEqual(len(dispatched), len(txIDs), "all dispatched")
}

func (s *ChainedDependenciesSuite) injectRevert(txID uuid.UUID, retriable bool) {
	s.T().Helper()
	revertReason := pldtypes.MustParseHexBytes("0xdeadbeef")
	msg := "non-retriable"
	if retriable {
		msg = "retriable"
	}
	s.builder.GetDomainAPI().On("IsBaseLedgerRevertRetryable", mock.Anything, []byte(revertReason)).
		Return(retriable, msg, nil).Maybe()

	nonce := pldtypes.HexUint64(42)
	s.c.QueueEvent(s.ctx, &transaction.ConfirmedRevertedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{TransactionID: txID},
		Nonce:                &nonce,
		Hash:                 pldtypes.Bytes32(pldtypes.RandBytes(32)),
		RevertReason:         revertReason,
		FailureMessage:       msg,
	})
}

func (s *ChainedDependenciesSuite) assertInState(state transaction.State, txIDs ...uuid.UUID) {
	s.T().Helper()
	s.sync()
	for _, id := range txIDs {
		found := false
		for _, tx := range s.c.getTransactionsInStates(s.ctx, []transaction.State{state}) {
			if tx.GetID() == id {
				found = true
				break
			}
		}
		s.Require().True(found, "expected %s in %s", id, state)
	}
}

func (s *ChainedDependenciesSuite) sync() {
	ev := statemachine.NewSyncEvent()
	s.c.QueueEvent(s.ctx, ev)
	<-ev.Done
}

func (s *ChainedDependenciesSuite) assertEvicted(txIDs ...uuid.UUID) {
	s.T().Helper()
	s.sync()
	for _, id := range txIDs {
		s.Require().Nil(s.c.transactionsByID[id], "expected %s to be evicted and cleaned up", id)
	}
}

func (s *ChainedDependenciesSuite) getFinalizeRequestForTx(txID uuid.UUID) *syncpoints.TransactionFinalizeRequest {
	s.T().Helper()
	for _, call := range s.syncPoints.Calls {
		if call.Method == "QueueTransactionFinalize" {
			req := call.Arguments.Get(1).(*syncpoints.TransactionFinalizeRequest)
			if req.TransactionID == txID {
				return req
			}
		}
	}
	s.Require().Failf("no finalize request found", "expected QueueTransactionFinalize call for TX %s", txID)
	return nil
}

// non-retriable revert cascades A→B→C: A fails with its revert reason from on chain, B and C fail
// with a message that their chained dependency has failed
func (s *ChainedDependenciesSuite) TestNonRetriableRevertCascade() {
	a := s.newTx()
	b := s.newTx(a)
	c := s.newTx(b)

	s.delegate(a, b, c)
	s.progressToReadyForDispatch(a, b, c)
	s.dispatch(a, b, c)

	s.injectRevert(a, false)

	s.assertInState(transaction.State_Reverted, a, b, c)
	s.syncPoints.AssertNumberOfCalls(s.T(), "QueueTransactionFinalize", 3)

	reqA := s.getFinalizeRequestForTx(a)
	s.Require().Contains(reqA.FailureMessage, "non-retriable")
	s.Require().Equal([]byte(pldtypes.MustParseHexBytes("0xdeadbeef")), []byte(reqA.RevertData))

	reqB := s.getFinalizeRequestForTx(b)
	s.Require().Contains(reqB.FailureMessage, "PD012256")
	s.Require().Contains(reqB.FailureMessage, a.String(), "B's failure should reference dependency A")

	reqC := s.getFinalizeRequestForTx(c)
	s.Require().Contains(reqC.FailureMessage, "PD012256")
	s.Require().Contains(reqC.FailureMessage, b.String(), "C's failure should reference dependency B")
}

// mid-chain retriable revert re-pools B, C stays dispatched, A unaffected
func (s *ChainedDependenciesSuite) TestMidChainRetriableRevert() {
	a := s.newTx()
	b := s.newTx(a)
	c := s.newTx(b)

	s.delegate(a, b, c)
	s.progressToReadyForDispatch(a, b, c)
	s.dispatch(a, b, c)

	s.injectRevert(b, true)

	s.assertInState(transaction.State_Assembling, b)
	s.assertInState(transaction.State_Dispatched, a)
	s.assertInState(transaction.State_Dispatched, c)
}

// head-of-chain retriable revert re-pools A. B and C stay dispatched
func (s *ChainedDependenciesSuite) TestHeadRetriableRevertCascadesReset() {
	a := s.newTx()
	b := s.newTx(a)
	c := s.newTx(b)

	s.delegate(a, b, c)
	s.progressToReadyForDispatch(a, b, c)
	s.dispatch(a, b, c)

	s.injectRevert(a, true)

	s.assertInState(transaction.State_Assembling, a)
	s.assertInState(transaction.State_Dispatched, b, c)
}

// eviction cascades A→B
func (s *ChainedDependenciesSuite) TestEvictionCascade() {
	s.c.assembleErrorRetryThreshhold = 0

	a := s.newTx()
	b := s.newTx(a)

	s.delegate(a, b)
	s.assertInState(transaction.State_Assembling, a)

	s.c.QueueEvent(s.ctx, &transaction.AssembleErrorResponseEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{TransactionID: a},
		RequestID:            s.mocks.SentMessageRecorder.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
	})

	s.assertEvicted(a, b)
}

// late-arriving dependent finds reverted dep → immediate revert
func (s *ChainedDependenciesSuite) TestLateArrivalFindsRevertedDep() {
	a := s.newTx()
	s.delegate(a)
	s.progressToReadyForDispatch(a)
	s.dispatch(a)
	s.injectRevert(a, false)
	s.assertInState(transaction.State_Reverted, a)

	b := s.newTx(a)
	s.delegate(b)

	s.assertInState(transaction.State_Reverted, b)
}

// late-arriving dependent after evicted dep is cleaned up
// Unlike Reverted (which stays in the grapher for heartbeat-based cleanup), Evicted
// transactions are cleaned up immediately by the coordinator. When B arrives after
// A has been evicted and cleaned up, A is no longer in the grapher so B proceeds
// normally (the dependency is treated as finalized). This test documents that behaviour.
func (s *ChainedDependenciesSuite) TestLateArrivalAfterEvictedDepCleanedUp() {
	s.c.assembleErrorRetryThreshhold = 0

	a := s.newTx()
	s.delegate(a)
	s.assertInState(transaction.State_Assembling, a)

	s.c.QueueEvent(s.ctx, &transaction.AssembleErrorResponseEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{TransactionID: a},
		RequestID:            s.mocks.SentMessageRecorder.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
	})
	s.assertEvicted(a)

	b := s.newTx(a)
	s.delegate(b)
	s.assertInState(transaction.State_Assembling, b)
}
