/*
 * Copyright © 2026 Kaleido, Inc.
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

package syncpoints

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/flushwriter"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSyncPointOperation_WriteKey(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	op := &syncPointOperation{
		contractAddress: *contractAddr,
	}

	key := op.WriteKey()
	assert.Equal(t, contractAddr.String(), key)
}

func TestRunBatch_EmptyBatch(t *testing.T) {
	ctx := context.Background()
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	values := []*syncPointOperation{}
	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 0, len(results))
}

func TestRunBatch_OnlyDomainStateWriters(t *testing.T) {
	ctx := context.Background()
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	// Create mock domain contexts
	dc1 := componentsmocks.NewDomainStateWriter(t)
	dc1.On("Flush", mock.Anything, dbTX).Return(nil)

	dc2 := componentsmocks.NewDomainStateWriter(t)
	dc2.On("Flush", mock.Anything, dbTX).Return(nil)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress:   *contractAddr,
			domainStateWriter: dc1,
		},
		{
			contractAddress:   *contractAddr,
			domainStateWriter: dc2,
		},
		{
			contractAddress:   *contractAddr,
			domainStateWriter: dc1, // Duplicate - should be deduplicated
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 3, len(results))
	dc1.AssertExpectations(t)
	dc2.AssertExpectations(t)
}

func TestRunBatch_DomainStateWriterFlushError(t *testing.T) {
	ctx := context.Background()
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	dc := componentsmocks.NewDomainStateWriter(t)
	flushErr := errors.New("flush error")
	dc.On("Flush", mock.Anything, dbTX).Return(flushErr)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress:   *contractAddr,
			domainStateWriter: dc,
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	assert.Error(t, err)
	assert.Equal(t, flushErr, err)
	assert.Nil(t, results)
	dc.AssertExpectations(t)
}

func TestRunBatch_OnlyFinalizeOperations(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)

	// Mock WriteOrDistributeReceipts behavior
	// Since originators are on the same node as LocalNodeName, receipts go to FinalizeTransactions
	mockTXMgr.On("FinalizeTransactions", ctx, dbTX, mock.MatchedBy(func(receipts []*components.ReceiptInput) bool {
		return len(receipts) == 2
	})).Return(nil)
	mockTransportMgr.On("LocalNodeName").Return("node1")

	contractAddr := pldtypes.RandAddress()
	txID1 := uuid.New()
	txID2 := uuid.New()

	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			finalizeOperation: &finalizeOperation{
				TransactionFinalizeRequest: TransactionFinalizeRequest{
					Domain:         "domain1",
					TransactionID:  txID1,
					FailureMessage: "error1",
					Originator:     "originator1@node1",
				},
			},
		},
		{
			contractAddress: *contractAddr,
			finalizeOperation: &finalizeOperation{
				TransactionFinalizeRequest: TransactionFinalizeRequest{
					Domain:         "domain2",
					TransactionID:  txID2,
					FailureMessage: "error2",
					Originator:     "originator2@node1",
				},
			},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	mockTXMgr.AssertExpectations(t)
	mockTransportMgr.AssertExpectations(t)
}

func TestRunBatch_OnlyDispatchOperations(t *testing.T) {
	ctx := context.Background()
	mockPubTxMgr := componentsmocks.NewPublicTxManager(t)
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     mockPubTxMgr,
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)

	// Mock writeDispatchOperations - when there are no PrivateTransactionDispatches, it should skip
	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperations: []*dispatchOperation{{
				publicDispatches: []*PublicDispatch{
					{
						PublicTxs:                    []*components.PublicTxSubmission{},
						PrivateTransactionDispatches: []*DispatchPersisted{}, // Empty - will be skipped
					},
				},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
}

func TestRunBatch_MixedOperations(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockPubTxMgr := componentsmocks.NewPublicTxManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     mockPubTxMgr,
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)

	// Setup domain context
	dc := componentsmocks.NewDomainStateWriter(t)
	dc.On("Flush", mock.Anything, dbTX).Return(nil)

	// Setup finalize operation mocks
	// Since originator is on the same node as LocalNodeName, receipt goes to FinalizeTransactions
	mockTXMgr.On("FinalizeTransactions", ctx, dbTX, mock.MatchedBy(func(receipts []*components.ReceiptInput) bool {
		return len(receipts) == 1
	})).Return(nil)
	mockTransportMgr.On("LocalNodeName").Return("node1")

	// Setup dispatch operation - use empty dispatches to avoid complex GORM mocking

	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	values := []*syncPointOperation{
		{
			contractAddress:   *contractAddr,
			domainStateWriter: dc,
			finalizeOperation: &finalizeOperation{
				TransactionFinalizeRequest: TransactionFinalizeRequest{
					Domain:         "domain1",
					TransactionID:  txID,
					FailureMessage: "error1",
					Originator:     "originator1@node1",
				},
			},
		},
		{
			contractAddress:   *contractAddr,
			domainStateWriter: dc,
			dispatchOperations: []*dispatchOperation{{
				publicDispatches: []*PublicDispatch{
					{
						PublicTxs:                    []*components.PublicTxSubmission{},
						PrivateTransactionDispatches: []*DispatchPersisted{}, // Empty - will be skipped
					},
				},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	dc.AssertExpectations(t)
	mockTXMgr.AssertExpectations(t)
}

func TestRunBatch_FinalizeOperationError(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)

	finalizeErr := errors.New("finalize error")
	mockTXMgr.On("FinalizeTransactions", ctx, dbTX, mock.MatchedBy(func(receipts []*components.ReceiptInput) bool {
		return len(receipts) == 1
	})).Return(finalizeErr)
	mockTransportMgr.On("LocalNodeName").Return("node1")

	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			finalizeOperation: &finalizeOperation{
				TransactionFinalizeRequest: TransactionFinalizeRequest{
					Domain:         "domain1",
					TransactionID:  txID,
					FailureMessage: "error1",
					Originator:     "originator1@node1",
				},
			},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	assert.Error(t, err)
	assert.Equal(t, finalizeErr, err)
	assert.Nil(t, results)
	mockTXMgr.AssertExpectations(t)
}

func TestRunBatch_DispatchOperationError(t *testing.T) {
	ctx := context.Background()
	mockPubTxMgr := componentsmocks.NewPublicTxManager(t)
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     mockPubTxMgr,
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	dispatchErr := errors.New("dispatch error")
	mockPubTxMgr.On("WriteNewTransactions", mock.Anything, dbTX, mock.Anything).Return(nil, dispatchErr)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperations: []*dispatchOperation{{
				publicDispatches: []*PublicDispatch{
					{
						PublicTxs: []*components.PublicTxSubmission{
							{},
						},
						PrivateTransactionDispatches: []*DispatchPersisted{
							{},
						},
					},
				},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	assert.Error(t, err)
	assert.Equal(t, dispatchErr, err)
	assert.Nil(t, results)
	mockPubTxMgr.AssertExpectations(t)
}

func TestRunBatch_NoOperations(t *testing.T) {
	ctx := context.Background()
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			// No operations set
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
	assert.IsType(t, flushwriter.Result[*noResult]{}, results[0])
}

func TestRunBatch_MultipleDomainStateWritersDedup(t *testing.T) {
	ctx := context.Background()
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	// Deduplication is by pointer identity (interface value), so two different DomainStateWriter
	// instances are two different map keys. Both will be flushed.
	dc1 := componentsmocks.NewDomainStateWriter(t)
	dc1.On("Flush", mock.Anything, dbTX).Return(nil)

	dc2 := componentsmocks.NewDomainStateWriter(t)
	dc2.On("Flush", mock.Anything, dbTX).Return(nil)

	// Adding the same writer twice deduplicates it to one flush.
	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{contractAddress: *contractAddr, domainStateWriter: dc1},
		{contractAddress: *contractAddr, domainStateWriter: dc2},
		{contractAddress: *contractAddr, domainStateWriter: dc1}, // duplicate of dc1
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 3, len(results))
	// dc1 and dc2 each flushed exactly once despite dc1 appearing twice
	dc1.AssertNumberOfCalls(t, "Flush", 1)
	dc2.AssertNumberOfCalls(t, "Flush", 1)
}

func TestRunBatch_FinalizeOperationsWithEmptyFailureMessage(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)

	// When FailureMessage is empty, no receipts should be distributed
	// WriteOrDistributeReceipts will be called with empty receipts list
	mockTransportMgr.On("LocalNodeName").Return("node1").Maybe()

	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			finalizeOperation: &finalizeOperation{
				TransactionFinalizeRequest: TransactionFinalizeRequest{
					Domain:         "domain1",
					TransactionID:  txID,
					FailureMessage: "",
					Originator:     "originator1@node1",
				},
			},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
	// FinalizeTransactions should not be called when there are no failure messages
	mockTXMgr.AssertNotCalled(t, "FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything)
	mockTransportMgr.AssertNotCalled(t, "SendReliable", mock.Anything, mock.Anything, mock.Anything)
}

func TestRunBatch_FinalizeOperationsWithOnChainRevert(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)

	txID := uuid.New()
	revertData := pldtypes.MustParseHexBytes("0xdeadbeef")
	onChain := pldtypes.OnChainLocation{
		Type:             pldtypes.OnChainTransaction,
		TransactionHash:  pldtypes.NewBytes32FromSlice([]byte{0x01}),
		BlockNumber:      42,
		TransactionIndex: 7,
	}

	mockTXMgr.On("FinalizeTransactions", ctx, dbTX, mock.MatchedBy(func(receipts []*components.ReceiptInput) bool {
		if len(receipts) != 1 {
			return false
		}
		r := receipts[0]
		return r.ReceiptType == components.RT_FailedOnChainWithRevertData &&
			r.TransactionID == txID &&
			r.OnChain.BlockNumber == 42 &&
			r.OnChain.TransactionIndex == 7 &&
			r.RevertData.String() == revertData.String()
	})).Return(nil)
	mockTransportMgr.On("LocalNodeName").Return("node1")

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			finalizeOperation: &finalizeOperation{
				TransactionFinalizeRequest: TransactionFinalizeRequest{
					Domain:        "domain1",
					TransactionID: txID,
					Originator:    "originator1@node1",
					RevertData:    revertData,
					OnChain:       &onChain,
				},
			},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
	mockTXMgr.AssertExpectations(t)
}

func TestRunBatch_FinalizeOperations_ZeroValueOnChainPointerUsesOffChainReceiptType(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)

	txID := uuid.New()
	failureMessage := "assembly failed upstream"
	zeroOnChain := pldtypes.OnChainLocation{Type: pldtypes.NotOnChain}

	mockTXMgr.On("FinalizeTransactions", ctx, dbTX, mock.MatchedBy(func(receipts []*components.ReceiptInput) bool {
		if len(receipts) != 1 {
			return false
		}
		r := receipts[0]
		return r.ReceiptType == components.RT_FailedWithMessage &&
			r.TransactionID == txID &&
			r.FailureMessage == failureMessage &&
			len(r.RevertData) == 0 &&
			r.OnChain.Type == pldtypes.NotOnChain
	})).Return(nil)
	mockTransportMgr.On("LocalNodeName").Return("node1")

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			finalizeOperation: &finalizeOperation{
				TransactionFinalizeRequest: TransactionFinalizeRequest{
					Domain:         "domain1",
					TransactionID:  txID,
					Originator:     "originator1@node1",
					FailureMessage: failureMessage,
					OnChain:        &zeroOnChain,
				},
			},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
	mockTXMgr.AssertExpectations(t)
}

func TestWriteOrDistributeReceipts_LocalSuccessIsFinalized(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)
	txID := uuid.New()

	mockTransportMgr.On("LocalNodeName").Return("node1")
	mockTXMgr.On("FinalizeTransactions", ctx, dbTX, mock.MatchedBy(func(receipts []*components.ReceiptInput) bool {
		return len(receipts) == 1 &&
			receipts[0].ReceiptType == components.RT_Success &&
			receipts[0].TransactionID == txID
	})).Return(nil).Once()

	err := s.WriteOrDistributeReceipts(ctx, dbTX, []*components.ReceiptInputWithOriginator{{
		Originator:            "wallets.org1.alice@node1",
		DomainContractAddress: "0xabc",
		ReceiptInput: components.ReceiptInput{
			ReceiptType:   components.RT_Success,
			TransactionID: txID,
		},
	}})
	require.NoError(t, err)
	mockTXMgr.AssertExpectations(t)
	mockTransportMgr.AssertExpectations(t)
}

func TestWriteOrDistributeReceipts_RemoteSuccessIsSentReliably(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)
	txID := uuid.New()

	mockTransportMgr.On("LocalNodeName").Return("node1")
	mockTransportMgr.On("SendReliable", ctx, dbTX, mock.MatchedBy(func(msgs []*pldapi.ReliableMessage) bool {
		return len(msgs) == 1 &&
			msgs[0].Node == "node2" &&
			msgs[0].MessageType.V() == pldapi.RMTReceipt
	})).Return(nil).Once()

	err := s.WriteOrDistributeReceipts(ctx, dbTX, []*components.ReceiptInputWithOriginator{{
		Originator:            "wallets.org1.alice@node2",
		DomainContractAddress: "0xabc",
		ReceiptInput: components.ReceiptInput{
			ReceiptType:   components.RT_Success,
			TransactionID: txID,
		},
	}})
	require.NoError(t, err)
	mockTXMgr.AssertNotCalled(t, "FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything)
	mockTransportMgr.AssertExpectations(t)
}

func TestWriteOrDistributeReceipts_RemoteSendError_LoggedAndIgnored(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)
	txID := uuid.New()

	mockTransportMgr.On("LocalNodeName").Return("node1")
	mockTransportMgr.On("SendReliable", ctx, dbTX, mock.Anything).Return(errors.New("peer unreachable")).Once()

	err := s.WriteOrDistributeReceipts(ctx, dbTX, []*components.ReceiptInputWithOriginator{{
		Originator:            "wallets.org1.alice@node2",
		DomainContractAddress: "0xabc",
		ReceiptInput: components.ReceiptInput{
			ReceiptType:    components.RT_FailedWithMessage,
			TransactionID:  txID,
			FailureMessage: "upstream failure",
		},
	}})
	// SendReliable error is logged but not returned
	require.NoError(t, err)
}

func TestQueueTransactionFinalize_OnCommit(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	conf := &pldconf.FlushWriterConfig{
		WorkerCount:  confutil.P(1),
		BatchTimeout: confutil.P("100ms"),
		BatchMaxSize: confutil.P(10),
	}

	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)
	transportMgr.On("LocalNodeName").Return("node1").Maybe()

	sp := NewSyncPoints(ctx, conf, mp.P, txMgr, pubTxMgr, transportMgr).(*syncPoints)
	sp.Start()
	defer sp.Close()

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectCommit()

	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	commitCh := make(chan struct{})

	sp.QueueTransactionFinalize(ctx, &TransactionFinalizeRequest{
		Domain:          "domain1",
		ContractAddress: *contractAddr,
		TransactionID:   txID,
		Originator:      "originator@node1",
		// No failure message → empty receipts → commit with no DB writes
	}, func(ctx context.Context) {
		close(commitCh)
	}, func(ctx context.Context, err error) {
		t.Errorf("unexpected rollback: %v", err)
	})

	<-commitCh
}

func TestQueueTransactionFinalize_OnRollback(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	conf := &pldconf.FlushWriterConfig{
		WorkerCount:  confutil.P(1),
		BatchTimeout: confutil.P("100ms"),
		BatchMaxSize: confutil.P(10),
	}

	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)
	transportMgr.On("LocalNodeName").Return("node1").Maybe()

	sp := NewSyncPoints(ctx, conf, mp.P, txMgr, pubTxMgr, transportMgr).(*syncPoints)
	sp.Start()
	defer sp.Close()

	finalizeErr := errors.New("finalize failed")
	txMgr.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).Return(finalizeErr)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectRollback()

	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	rollbackCh := make(chan error, 1)

	sp.QueueTransactionFinalize(ctx, &TransactionFinalizeRequest{
		Domain:          "domain1",
		ContractAddress: *contractAddr,
		TransactionID:   txID,
		Originator:      "originator@node1",
		FailureMessage:  "assembly reverted",
	}, func(ctx context.Context) {
		t.Error("unexpected commit")
	}, func(ctx context.Context, err error) {
		rollbackCh <- err
	})

	rbErr := <-rollbackCh
	require.Error(t, rbErr)
}

func TestRunBatch_WithPrivateDispatches(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	chainedTx := &components.ChainedPrivateTransaction{
		ID:                    uuid.New(),
		OriginalTransaction:   uuid.New(),
		OriginalSenderLocator: "identity@node1",
	}
	mockTXMgr.On("ChainPrivateTransactions", mock.Anything, dbTX, mock.MatchedBy(func(txs []*components.ChainedPrivateTransaction) bool {
		return len(txs) == 1 && txs[0] == chainedTx
	})).Return(nil)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperations: []*dispatchOperation{{
				privateDispatches: []*components.ChainedPrivateTransaction{chainedTx},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
}

func TestRunBatch_WithPrivateDispatchesError(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	chainErr := errors.New("chain error")
	mockTXMgr.On("ChainPrivateTransactions", mock.Anything, dbTX, mock.Anything).Return(chainErr)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperations: []*dispatchOperation{{
				privateDispatches: []*components.ChainedPrivateTransaction{
					{ID: uuid.New()},
				},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)
	assert.Error(t, err)
	assert.Equal(t, chainErr, err)
	assert.Nil(t, results)
}

func TestRunBatch_WithPreparedReliableMsgs(t *testing.T) {
	ctx := context.Background()
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)

	mockTransportMgr.On("SendReliable", mock.Anything, dbTX, mock.Anything).Return(nil)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperations: []*dispatchOperation{{
				preparedReliableMsgs: []*pldapi.ReliableMessage{
					{Node: "node2", MessageType: pldapi.RMTState.Enum()},
				},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
}

func TestRunBatch_WithPreparedReliableMsgsError(t *testing.T) {
	ctx := context.Background()
	mockTransportMgr := componentsmocks.NewTransportManager(t)
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: mockTransportMgr,
	}
	dbTX := persistencemocks.NewDBTX(t)

	sendErr := errors.New("send error")
	mockTransportMgr.On("SendReliable", mock.Anything, dbTX, mock.Anything).Return(sendErr)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperations: []*dispatchOperation{{
				preparedReliableMsgs: []*pldapi.ReliableMessage{
					{Node: "node2", MessageType: pldapi.RMTState.Enum()},
				},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)
	assert.Error(t, err)
	assert.Equal(t, sendErr, err)
	assert.Nil(t, results)
}

func TestRunBatch_WithLocalPreparedTxnsError(t *testing.T) {
	ctx := context.Background()
	mockTXMgr := componentsmocks.NewTXManager(t)
	s := &syncPoints{
		txMgr:        mockTXMgr,
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	writeErr := errors.New("write prepared txns error")
	mockTXMgr.On("WritePreparedTransactions", mock.Anything, dbTX, mock.Anything).Return(writeErr)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperations: []*dispatchOperation{{
				localPreparedTxns: []*components.PreparedTransactionWithRefs{
					{},
				},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)
	assert.Error(t, err)
	assert.Equal(t, writeErr, err)
	assert.Nil(t, results)
}

func TestRunBatch_WithLocalSequencerActivities(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}

	mp.Mock.ExpectQuery("INSERT.*sequencer_activities").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	dbTX := mp.P.NOTX()
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperations: []*dispatchOperation{{
				localSequencerActivites: []*components.SequencingActivity{
					{
						SubjectID:      uuid.New().String(),
						TransactionID:  txID,
						ActivityType:   "dispatch",
						SequencingNode: "node1",
					},
				},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results))
}

func TestRunBatch_WithLocalSequencerActivitiesError(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}

	dbErr := errors.New("db insert error")
	mp.Mock.ExpectQuery("INSERT.*sequencer_activities").WillReturnError(dbErr)

	dbTX := mp.P.NOTX()
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperations: []*dispatchOperation{{
				localSequencerActivites: []*components.SequencingActivity{
					{
						SubjectID:      uuid.New().String(),
						TransactionID:  txID,
						ActivityType:   "dispatch",
						SequencingNode: "node1",
					},
				},
			}},
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)
	assert.Error(t, err)
	assert.Nil(t, results)
}
