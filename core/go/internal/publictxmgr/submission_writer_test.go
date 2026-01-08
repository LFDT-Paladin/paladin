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

package publictxmgr

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

func newTestSubmissionWriter(t *testing.T) (context.Context, *submissionWriter, *componentsmocks.SequencerManager, *mockMetrics, persistence.Persistence, sqlmock.Sqlmock, func()) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	sequencerManager := componentsmocks.NewSequencerManager(t)
	txManager := componentsmocks.NewTXManager(t)
	mockMetrics := &mockMetrics{}

	conf := &pldconf.PublicTxManagerConfig{
		Manager: pldconf.PublicTxManagerManagerConfig{
			SubmissionWriter: pldconf.FlushWriterConfig{
				WorkerCount: confutil.P(1),
			},
		},
	}

	sw := newSubmissionWriter(ctx, "test-node", mp.P, conf, mockMetrics, sequencerManager, txManager)

	return ctx, sw, sequencerManager, mockMetrics, mp.P, mp.Mock, func() {
		require.NoError(t, mp.Mock.ExpectationsWereMet())
	}
}

type mockMetrics struct {
	incDBSubmittedTransactionsByNCalled bool
	incDBSubmittedTransactionsByNValue   uint64
}

func (m *mockMetrics) IncDBSubmittedTransactions() {
	m.incDBSubmittedTransactionsByNCalled = true
	m.incDBSubmittedTransactionsByNValue = 1
}

func (m *mockMetrics) IncDBSubmittedTransactionsByN(numberOfTransactions uint64) {
	m.incDBSubmittedTransactionsByNCalled = true
	m.incDBSubmittedTransactionsByNValue = numberOfTransactions
}

func (m *mockMetrics) IncCompletedTransactions() {}

func (m *mockMetrics) IncCompletedTransactionsByN(numberOfTransactions uint64) {}

func (m *mockMetrics) RecordOperationMetrics(ctx context.Context, operationName string, operationResult string, durationInSeconds float64) {}

func (m *mockMetrics) RecordStageChangeMetrics(ctx context.Context, stage string, durationInSeconds float64) {}

func (m *mockMetrics) RecordInFlightTxQueueMetrics(ctx context.Context, usedCountPerStage map[string]int, freeCount int) {}

func (m *mockMetrics) RecordCompletedTransactionCountMetrics(ctx context.Context, processStatus string) {}

func (m *mockMetrics) RecordInFlightOrchestratorPoolMetrics(ctx context.Context, usedCountPerState map[string]int, freeCount int) {}

func createTestDBPubTxnSubmission(t *testing.T, withOriginator bool, withBinding bool, withGasPricing bool) *DBPubTxnSubmission {
	submission := &DBPubTxnSubmission{
		from:            "0x1234567890123456789012345678901234567890",
		PublicTxnID:     1,
		Created:         pldtypes.Timestamp(time.Now().UnixNano()),
		TransactionHash: pldtypes.RandBytes32(),
		TransactionType: pldtypes.Enum[pldapi.TransactionType](pldapi.TransactionTypePrivate),
		PrivateTXID:     uuid.New(),
	}

	if withOriginator {
		submission.PrivateTXOriginator = "originator-node"
	}

	if withBinding {
		nonce := pldtypes.HexUint64(42)
		to := pldtypes.MustEthAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")
		gas := pldtypes.HexUint64(21000)
		value := pldtypes.MustParseHexUint256("0x1000")
		maxPriorityFeePerGas := pldtypes.MustParseHexUint256("0x10")
		maxFeePerGas := pldtypes.MustParseHexUint256("0x100")

		submission.Binding = &pldapi.PublicTx{
			Nonce:   &nonce,
			To:      to,
			Created: pldtypes.Timestamp(time.Now().UnixNano()),
			Data:    pldtypes.HexBytes("0x1234"),
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas:   &gas,
				Value: value,
				PublicTxGasPricing: pldapi.PublicTxGasPricing{
					MaxPriorityFeePerGas: maxPriorityFeePerGas,
					MaxFeePerGas:         maxFeePerGas,
				},
			},
		}
	}

	if withGasPricing {
		gasPricing := pldapi.PublicTxGasPricing{
			MaxPriorityFeePerGas: pldtypes.MustParseHexUint256("0x20"),
			MaxFeePerGas:         pldtypes.MustParseHexUint256("0x200"),
		}
		gasPricingJSON, err := json.Marshal(gasPricing)
		require.NoError(t, err)
		submission.GasPricing = pldtypes.RawJSON(gasPricingJSON)
	}

	return submission
}

func TestRunBatch_WithOriginatorAndBinding(t *testing.T) {
	ctx, sw, sequencerManager, mockMetrics, p, mdb, done := newTestSubmissionWriter(t)
	defer done()

	submission := createTestDBPubTxnSubmission(t, true, true, true)

	// Mock database insert
	mdb.ExpectBegin()
	mdb.ExpectExec(`INSERT INTO "public_submissions"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mdb.ExpectCommit()

	// Mock HandlePublicTXSubmission
	sequencerManager.On("HandlePublicTXSubmission", mock.Anything, mock.Anything, "originator-node", submission.PrivateTXID, mock.MatchedBy(func(txSubmission *pldapi.PublicTxWithBinding) bool {
		// Verify the PublicTxWithBinding structure
		require.NotNil(t, txSubmission)
		require.NotNil(t, txSubmission.PublicTx)
		require.NotNil(t, txSubmission.TransactionHash)
		require.Equal(t, submission.TransactionHash, *txSubmission.TransactionHash)
		require.Equal(t, submission.PrivateTXID, txSubmission.Transaction)
		require.Equal(t, submission.TransactionType, txSubmission.TransactionType)

		// Verify PublicTx fields
		publicTX := txSubmission.PublicTx
		require.Equal(t, "test-node", publicTX.Dispatcher)
		require.Equal(t, submission.Binding.To, publicTX.To)
		require.Equal(t, submission.Binding.Data, publicTX.Data)
		require.Equal(t, submission.Binding.Gas, publicTX.PublicTxOptions.Gas)
		require.Equal(t, submission.Binding.Value, publicTX.PublicTxOptions.Value)
		require.Equal(t, submission.Binding.Created, publicTX.Created)
		require.NotNil(t, publicTX.Nonce)
		require.Equal(t, uint64(42), uint64(*publicTX.Nonce))
		require.Equal(t, submission.Binding.MaxPriorityFeePerGas, publicTX.PublicTxOptions.MaxPriorityFeePerGas)
		require.Equal(t, submission.Binding.MaxFeePerGas, publicTX.PublicTxOptions.MaxFeePerGas)

		// Verify From address
		expectedFrom := pldtypes.MustEthAddress(submission.from)
		require.Equal(t, *expectedFrom, publicTX.From)

		// Verify Submissions array
		require.Len(t, publicTX.Submissions, 1)
		submissionData := publicTX.Submissions[0]
		require.Equal(t, submission.Created, submissionData.Time)
		require.Equal(t, submission.TransactionHash, submissionData.TransactionHash)
		// Verify gas pricing from GasPricing field (not from Binding)
		expectedMaxPriorityFeePerGas := pldtypes.MustParseHexUint256("0x20")
		expectedMaxFeePerGas := pldtypes.MustParseHexUint256("0x200")
		require.Equal(t, expectedMaxPriorityFeePerGas, submissionData.PublicTxGasPricing.MaxPriorityFeePerGas)
		require.Equal(t, expectedMaxFeePerGas, submissionData.PublicTxGasPricing.MaxFeePerGas)

		return true
	})).Return(nil).Once()

	// Execute runBatch
	err := p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		results, err := sw.runBatch(ctx, dbTX, []*DBPubTxnSubmission{submission})
		require.NoError(t, err)
		require.Len(t, results, 1)
		return nil
	})
	require.NoError(t, err)

	// Verify metrics were called
	assert.True(t, mockMetrics.incDBSubmittedTransactionsByNCalled)
	assert.Equal(t, uint64(1), mockMetrics.incDBSubmittedTransactionsByNValue)

	sequencerManager.AssertExpectations(t)
}

func TestRunBatch_WithOriginatorAndBinding_WithoutGasPricing(t *testing.T) {
	ctx, sw, sequencerManager, mockMetrics, p, mdb, done := newTestSubmissionWriter(t)
	defer done()

	submission := createTestDBPubTxnSubmission(t, true, true, false)

	// Mock database insert - when GasPricing is nil, GORM uses (NULL) which doesn't require a parameter
	mdb.ExpectBegin()
	mdb.ExpectExec(`INSERT INTO "public_submissions"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mdb.ExpectCommit()

	// Mock HandlePublicTXSubmission
	sequencerManager.On("HandlePublicTXSubmission", mock.Anything, mock.Anything, "originator-node", submission.PrivateTXID, mock.MatchedBy(func(txSubmission *pldapi.PublicTxWithBinding) bool {
		require.NotNil(t, txSubmission)
		require.NotNil(t, txSubmission.PublicTx)
		require.Len(t, txSubmission.PublicTx.Submissions, 1)

		// Verify that when GasPricing is nil, the submission data has zero/empty gas pricing
		submissionData := txSubmission.PublicTx.Submissions[0]
		require.Nil(t, submissionData.PublicTxGasPricing.MaxPriorityFeePerGas)
		require.Nil(t, submissionData.PublicTxGasPricing.MaxFeePerGas)

		return true
	})).Return(nil).Once()

	// Execute runBatch
	err := p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		results, err := sw.runBatch(ctx, dbTX, []*DBPubTxnSubmission{submission})
		require.NoError(t, err)
		require.Len(t, results, 1)
		return nil
	})
	require.NoError(t, err)

	// Verify metrics were called
	assert.True(t, mockMetrics.incDBSubmittedTransactionsByNCalled)
	assert.Equal(t, uint64(1), mockMetrics.incDBSubmittedTransactionsByNValue)

	sequencerManager.AssertExpectations(t)
}

func TestRunBatch_WithoutOriginator(t *testing.T) {
	ctx, sw, sequencerManager, mockMetrics, p, mdb, done := newTestSubmissionWriter(t)
	defer done()

	submission := createTestDBPubTxnSubmission(t, false, true, true)

	// Mock database insert
	mdb.ExpectBegin()
	mdb.ExpectExec(`INSERT INTO "public_submissions"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mdb.ExpectCommit()

	// HandlePublicTXSubmission should NOT be called when PrivateTXOriginator is empty
	sequencerManager.AssertNotCalled(t, "HandlePublicTXSubmission")

	// Execute runBatch
	err := p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		results, err := sw.runBatch(ctx, dbTX, []*DBPubTxnSubmission{submission})
		require.NoError(t, err)
		require.Len(t, results, 1)
		return nil
	})
	require.NoError(t, err)

	// Verify metrics were called
	assert.True(t, mockMetrics.incDBSubmittedTransactionsByNCalled)
	assert.Equal(t, uint64(1), mockMetrics.incDBSubmittedTransactionsByNValue)
}

func TestRunBatch_WithoutBinding(t *testing.T) {
	ctx, sw, sequencerManager, mockMetrics, p, mdb, done := newTestSubmissionWriter(t)
	defer done()

	submission := createTestDBPubTxnSubmission(t, true, false, true)

	// Mock database insert
	mdb.ExpectBegin()
	mdb.ExpectExec(`INSERT INTO "public_submissions"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mdb.ExpectCommit()

	// HandlePublicTXSubmission should NOT be called when Binding is nil
	sequencerManager.AssertNotCalled(t, "HandlePublicTXSubmission")

	// Execute runBatch
	err := p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		results, err := sw.runBatch(ctx, dbTX, []*DBPubTxnSubmission{submission})
		require.NoError(t, err)
		require.Len(t, results, 1)
		return nil
	})
	require.NoError(t, err)

	// Verify metrics were called
	assert.True(t, mockMetrics.incDBSubmittedTransactionsByNCalled)
	assert.Equal(t, uint64(1), mockMetrics.incDBSubmittedTransactionsByNValue)
}

func TestRunBatch_HandlePublicTXSubmissionError(t *testing.T) {
	ctx, sw, sequencerManager, mockMetrics, p, mdb, done := newTestSubmissionWriter(t)
	defer done()

	submission := createTestDBPubTxnSubmission(t, true, true, true)

	// Mock database insert
	mdb.ExpectBegin()
	mdb.ExpectExec(`INSERT INTO "public_submissions"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mdb.ExpectRollback()

	// Mock HandlePublicTXSubmission to return an error
	expectedError := errors.New("sequencer error")
	sequencerManager.On("HandlePublicTXSubmission", mock.Anything, mock.Anything, "originator-node", submission.PrivateTXID, mock.Anything).
		Return(expectedError).Once()

	// Execute runBatch - should return error
	err := p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		results, err := sw.runBatch(ctx, dbTX, []*DBPubTxnSubmission{submission})
		require.Error(t, err)
		require.Equal(t, expectedError, err)
		require.Nil(t, results)
		return err
	})
	require.Error(t, err)

	// Metrics should NOT be called when there's an error
	assert.False(t, mockMetrics.incDBSubmittedTransactionsByNCalled)

	sequencerManager.AssertExpectations(t)
}

func TestRunBatch_MultipleSubmissions(t *testing.T) {
	ctx, sw, sequencerManager, mockMetrics, p, mdb, done := newTestSubmissionWriter(t)
	defer done()

	submission1 := createTestDBPubTxnSubmission(t, true, true, true)
	submission2 := createTestDBPubTxnSubmission(t, true, true, false)
	submission3 := createTestDBPubTxnSubmission(t, false, true, true) // no originator, should be skipped

	// Mock database insert
	mdb.ExpectBegin()
	mdb.ExpectExec(`INSERT INTO "public_submissions"`).
		WillReturnResult(sqlmock.NewResult(3, 3))
	mdb.ExpectCommit()

	// Mock HandlePublicTXSubmission for submission1 and submission2 (submission3 should be skipped)
	sequencerManager.On("HandlePublicTXSubmission", mock.Anything, mock.Anything, "originator-node", submission1.PrivateTXID, mock.Anything).
		Return(nil).Once()
	sequencerManager.On("HandlePublicTXSubmission", mock.Anything, mock.Anything, "originator-node", submission2.PrivateTXID, mock.Anything).
		Return(nil).Once()

	// Execute runBatch
	err := p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		results, err := sw.runBatch(ctx, dbTX, []*DBPubTxnSubmission{submission1, submission2, submission3})
		require.NoError(t, err)
		require.Len(t, results, 3)
		return nil
	})
	require.NoError(t, err)

	// Verify metrics were called with correct count
	assert.True(t, mockMetrics.incDBSubmittedTransactionsByNCalled)
	assert.Equal(t, uint64(3), mockMetrics.incDBSubmittedTransactionsByNValue)

	sequencerManager.AssertExpectations(t)
}

func TestRunBatch_NonceConversion(t *testing.T) {
	ctx, sw, sequencerManager, _, p, mdb, done := newTestSubmissionWriter(t)
	defer done()

	submission := createTestDBPubTxnSubmission(t, true, true, false)
	// Set a specific nonce value
	nonce := pldtypes.HexUint64(12345)
	submission.Binding.Nonce = &nonce

	// Mock database insert
	mdb.ExpectBegin()
	mdb.ExpectExec(`INSERT INTO "public_submissions"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mdb.ExpectCommit()

	// Mock HandlePublicTXSubmission and verify nonce conversion
	sequencerManager.On("HandlePublicTXSubmission", mock.Anything, mock.Anything, "originator-node", submission.PrivateTXID, mock.MatchedBy(func(txSubmission *pldapi.PublicTxWithBinding) bool {
		require.NotNil(t, txSubmission.PublicTx.Nonce)
		require.Equal(t, uint64(12345), uint64(*txSubmission.PublicTx.Nonce))
		return true
	})).Return(nil).Once()

	// Execute runBatch
	err := p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		results, err := sw.runBatch(ctx, dbTX, []*DBPubTxnSubmission{submission})
		require.NoError(t, err)
		require.Len(t, results, 1)
		return nil
	})
	require.NoError(t, err)

	sequencerManager.AssertExpectations(t)
}

func TestRunBatch_FromAddressConversion(t *testing.T) {
	ctx, sw, sequencerManager, _, p, mdb, done := newTestSubmissionWriter(t)
	defer done()

	submission := createTestDBPubTxnSubmission(t, true, true, false)
	// Set a specific from address
	testFromAddress := "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	submission.from = testFromAddress

	// Mock database insert
	mdb.ExpectBegin()
	mdb.ExpectExec(`INSERT INTO "public_submissions"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mdb.ExpectCommit()

	// Mock HandlePublicTXSubmission and verify from address conversion
	sequencerManager.On("HandlePublicTXSubmission", mock.Anything, mock.Anything, "originator-node", submission.PrivateTXID, mock.MatchedBy(func(txSubmission *pldapi.PublicTxWithBinding) bool {
		expectedFrom := pldtypes.MustEthAddress(testFromAddress)
		require.Equal(t, *expectedFrom, txSubmission.PublicTx.From)
		return true
	})).Return(nil).Once()

	// Execute runBatch
	err := p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		results, err := sw.runBatch(ctx, dbTX, []*DBPubTxnSubmission{submission})
		require.NoError(t, err)
		require.Len(t, results, 1)
		return nil
	})
	require.NoError(t, err)

	sequencerManager.AssertExpectations(t)
}

