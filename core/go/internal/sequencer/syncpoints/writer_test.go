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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/flushwriter"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
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

func TestRunBatch_OnlyDomainContexts(t *testing.T) {
	ctx := context.Background()
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	// Create mock domain contexts
	dc1 := componentsmocks.NewDomainContext(t)
	dc1ID := uuid.New()
	dc1.On("Info").Return(components.DomainContextInfo{ID: dc1ID})
	dc1.On("Flush", dbTX).Return(nil)

	dc2 := componentsmocks.NewDomainContext(t)
	dc2ID := uuid.New()
	dc2.On("Info").Return(components.DomainContextInfo{ID: dc2ID})
	dc2.On("Flush", dbTX).Return(nil)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			domainContext:   dc1,
		},
		{
			contractAddress: *contractAddr,
			domainContext:   dc2,
		},
		{
			contractAddress: *contractAddr,
			domainContext:   dc1, // Duplicate - should be deduplicated
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 3, len(results))
	dc1.AssertExpectations(t)
	dc2.AssertExpectations(t)
}

func TestRunBatch_DomainContextFlushError(t *testing.T) {
	ctx := context.Background()
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	dc := componentsmocks.NewDomainContext(t)
	dcID := uuid.New()
	dc.On("Info").Return(components.DomainContextInfo{ID: dcID})
	flushErr := errors.New("flush error")
	dc.On("Flush", dbTX).Return(flushErr)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			domainContext:   dc,
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
				Domain:         "domain1",
				TransactionID:  txID1,
				FailureMessage: "error1",
				Originator:     "originator1@node1",
			},
		},
		{
			contractAddress: *contractAddr,
			finalizeOperation: &finalizeOperation{
				Domain:         "domain2",
				TransactionID:  txID2,
				FailureMessage: "error2",
				Originator:     "originator2@node1",
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
			dispatchOperation: &dispatchOperation{
				publicDispatches: []*PublicDispatch{
					{
						PublicTxs:                    []*components.PublicTxSubmission{},
						PrivateTransactionDispatches: []*DispatchPersisted{}, // Empty - will be skipped
					},
				},
			},
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
	dc := componentsmocks.NewDomainContext(t)
	dcID := uuid.New()
	dc.On("Info").Return(components.DomainContextInfo{ID: dcID})
	dc.On("Flush", dbTX).Return(nil)

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
			contractAddress: *contractAddr,
			domainContext:   dc,
			finalizeOperation: &finalizeOperation{
				Domain:         "domain1",
				TransactionID:  txID,
				FailureMessage: "error1",
				Originator:     "originator1@node1",
			},
		},
		{
			contractAddress: *contractAddr,
			domainContext:   dc,
			dispatchOperation: &dispatchOperation{
				publicDispatches: []*PublicDispatch{
					{
						PublicTxs:                    []*components.PublicTxSubmission{},
						PrivateTransactionDispatches: []*DispatchPersisted{}, // Empty - will be skipped
					},
				},
			},
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
				Domain:         "domain1",
				TransactionID:  txID,
				FailureMessage: "error1",
				Originator:     "originator1@node1",
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
	mockPubTxMgr.On("WriteNewTransactions", ctx, dbTX, mock.Anything).Return(nil, dispatchErr)

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			dispatchOperation: &dispatchOperation{
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
			},
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

func TestRunBatch_MultipleDomainContextsWithSameID(t *testing.T) {
	ctx := context.Background()
	s := &syncPoints{
		txMgr:        componentsmocks.NewTXManager(t),
		pubTxMgr:     componentsmocks.NewPublicTxManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}
	dbTX := persistencemocks.NewDBTX(t)

	// Create domain contexts with the same ID - should be deduplicated
	// The map key is the domain context ID, so the last one added overwrites the first
	// Only one flush should occur (for the last one added)
	dcID := uuid.New()
	dc1 := componentsmocks.NewDomainContext(t)
	dc1.On("Info").Return(components.DomainContextInfo{ID: dcID})
	// dc1 won't be flushed since dc2 overwrites it in the map

	dc2 := componentsmocks.NewDomainContext(t)
	dc2.On("Info").Return(components.DomainContextInfo{ID: dcID})
	dc2.On("Flush", dbTX).Return(nil)
	// dc2 will be flushed since it's the last one added to the map

	contractAddr := pldtypes.RandAddress()
	values := []*syncPointOperation{
		{
			contractAddress: *contractAddr,
			domainContext:   dc1,
		},
		{
			contractAddress: *contractAddr,
			domainContext:   dc2, // Same ID as dc1, overwrites dc1 in the map
		},
	}

	results, err := s.runBatch(ctx, dbTX, values)

	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	// Only dc2 should be flushed since it overwrote dc1 in the map
	dc1.AssertNotCalled(t, "Flush", mock.Anything)
	dc2.AssertExpectations(t)
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
				Domain:         "domain1",
				TransactionID:  txID,
				FailureMessage: "", // Empty failure message
				Originator:     "originator1@node1",
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
