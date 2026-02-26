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

package publictxmgr

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/pkg/ethclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestInFlightStatusString_Pending(t *testing.T) {
	status := InFlightStatusPending
	assert.Equal(t, "pending", status.String())
}

func TestInFlightStatusString_ConfirmReceived(t *testing.T) {
	status := InFlightStatusConfirmReceived
	assert.Equal(t, "confirm_received", status.String())
}

func TestProcessSubmittingStageOutput_PersistenceError(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}

	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing: &pldapi.PublicTxGasPricing{
				MaxFeePerGas:         pldtypes.Uint64ToUint256(10),
				MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1),
			},
		},
	})

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	// Set up submitting stage
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	txHash := confutil.P(pldtypes.Bytes32Keccak([]byte("0x000031")))
	currentGeneration.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage:   []byte("signedMessage"),
		TransactionHash: txHash,
	})

	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)

	// Add submit output first
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	submissionTime := confutil.P(pldtypes.TimestampNow())
	it.stateManager.GetCurrentGeneration(ctx).AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeSubmittedNew, ethclient.ErrorReason(""), nil)
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	// Now test persistence error case
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	rsc.StageOutput = &StageOutput{
		SubmitOutput: &SubmitOutputs{
			SubmissionOutcome: SubmissionOutcomeSubmittedNew,
			TxHash:            txHash,
			SubmissionTime:    submissionTime,
		},
	}
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	// Add persistence output with error
	it.stateManager.GetCurrentGeneration(ctx).AddPersistenceOutput(ctx, InFlightTxStageSubmitting, time.Now(), fmt.Errorf("persistence error"))
	err := it.processSubmittingStageOutput(ctx, currentGeneration, rsc, &StageOutput{
		PersistenceOutput: &PersistenceOutput{
			PersistenceError: fmt.Errorf("persistence error"),
		},
	})
	assert.NoError(t, err)
	// Should not clear the running stage context when there's a persistence error
	assert.NotNil(t, currentGeneration.GetRunningStageContext(ctx))
}

func TestProcessSubmittingStageOutput_FixedGasPriceUnderpriced(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	// Create transaction with fixed gas price
	fixedGasPrice := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10),
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1),
	}
	it, mTS := newInflightTransaction(o, 1, func(tx *DBPublicTxn) {
		tx.FixedGasPricing = pldtypes.JSONString(*fixedGasPrice)
	})
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}

	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing: fixedGasPrice,
		},
	})

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	// Set up submitting stage
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	txHash := confutil.P(pldtypes.Bytes32Keccak([]byte("0x000031")))
	currentGeneration.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage:   []byte("signedMessage"),
		TransactionHash: txHash,
	})

	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)

	// Test underpriced error with fixed gas price
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	err := it.processSubmittingStageOutput(ctx, currentGeneration, rsc, &StageOutput{
		SubmitOutput: &SubmitOutputs{
			Err:               fmt.Errorf("transaction underpriced"),
			ErrorReason:       string(ethclient.ErrorReasonTransactionUnderpriced),
			SubmissionOutcome: SubmissionOutcomeFailedRequiresRetry,
		},
	})
	assert.NoError(t, err)
	// With fixed gas price, should not reset gas pricing
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.False(t, rsc.StageOutputsToBePersisted.TxUpdates.ResetValues.GasPricing)
}

func TestStartNewStage_NewStatusNil(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing: &pldapi.PublicTxGasPricing{
				MaxFeePerGas:         pldtypes.Uint64ToUint256(10),
				MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1),
			},
		},
	})

	// Set orchestrator context so CanSubmit works
	it.stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{
		AvailableToSpend:         big.NewInt(100000),
		PreviousNonceCostUnknown: false,
	})

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	it.newStatus = nil
	it.startNewStage(ctx, big.NewInt(10000))
	// Should proceed to signing stage since no status update needed
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	assert.NotNil(t, rsc)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)
}

func TestStartNewStage_NewStatusMatchesCurrent(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	currentStatus := InFlightStatusPending
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing: &pldapi.PublicTxGasPricing{
				MaxFeePerGas:         pldtypes.Uint64ToUint256(10),
				MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1),
			},
			InFlightStatus: &currentStatus,
		},
	})

	// Set orchestrator context so CanSubmit works
	it.stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{
		AvailableToSpend:         big.NewInt(100000),
		PreviousNonceCostUnknown: false,
	})

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	it.newStatus = &currentStatus // same as current
	it.startNewStage(ctx, big.NewInt(10000))
	// Should proceed to signing stage since status already matches
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	assert.NotNil(t, rsc)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)
}

func TestStartNewStage_CannotSubmit(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing: &pldapi.PublicTxGasPricing{
				MaxFeePerGas:         pldtypes.Uint64ToUint256(10),
				MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1),
			},
		},
	})

	// Set orchestrator context with insufficient funds
	it.stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{
		AvailableToSpend:         big.NewInt(100), // less than cost
		PreviousNonceCostUnknown: false,
	})

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	it.newStatus = nil
	// Use a cost higher than available funds
	it.startNewStage(ctx, big.NewInt(10000))
	// Should not start a new stage when cannot submit
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	// Should be nil since CanSubmit returns false
	assert.Nil(t, rsc)
}

func TestStartNewStage_TrackingStage(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	txHash := confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001")))
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing: &pldapi.PublicTxGasPricing{
				MaxFeePerGas:         pldtypes.Uint64ToUint256(10),
				MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1),
			},
			TransactionHash: txHash,
			LastSubmit:      confutil.P(pldtypes.TimestampNow()), // recent submit
		},
	})

	// Set a long resubmit interval so we don't exceed it
	it.resubmitInterval = 1 * time.Hour

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	it.newStatus = nil
	it.startNewStage(ctx, big.NewInt(10000))
	// Should enter tracking stage (no new stage context created)
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	// Tracking stage doesn't create a running stage context, so it should be nil
	// The function just logs and returns
	assert.Nil(t, rsc)
}

func TestStartNewStage_LastSubmitTimeNil(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	txHash := confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001")))
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing: &pldapi.PublicTxGasPricing{
				MaxFeePerGas:         pldtypes.Uint64ToUint256(10),
				MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1),
			},
			TransactionHash: txHash,
			// LastSubmit is nil
		},
	})

	it.resubmitInterval = 1 * time.Minute

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	it.newStatus = nil
	it.startNewStage(ctx, big.NewInt(10000))
	// Should enter tracking stage when lastSubmitTime is nil
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	assert.Nil(t, rsc)
}

func TestStartNewStage_ResubmitIntervalExceeded(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	txHash := confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001")))
	// Set last submit time to be in the past, exceeding resubmit interval
	oldSubmitTime := confutil.P(pldtypes.TimestampFromUnix(time.Now().Add(-2 * time.Hour).Unix()))
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing: &pldapi.PublicTxGasPricing{
				MaxFeePerGas:         pldtypes.Uint64ToUint256(10),
				MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1),
			},
			TransactionHash: txHash,
			LastSubmit:      oldSubmitTime,
		},
	})

	// Set a short resubmit interval so it's exceeded
	it.resubmitInterval = 1 * time.Minute

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	it.newStatus = nil
	it.startNewStage(ctx, big.NewInt(10000))
	// Should trigger retrieve gas price stage when resubmit interval is exceeded
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	assert.NotNil(t, rsc)
	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)
}

func TestNotifyStatusUpdate_Resume(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)

	// Set status to suspending (IsReadyToExit returns true when status != Pending)
	suspending := InFlightStatusSuspending
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			InFlightStatus: &suspending,
		},
	})

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	// Try to resume (change from suspending to pending)
	pending := InFlightStatusPending
	updateRequired, err := it.NotifyStatusUpdate(ctx, pending)
	assert.NoError(t, err)
	assert.True(t, updateRequired)
	assert.NotNil(t, it.newStatus)
	assert.Equal(t, pending, *it.newStatus)
}

func TestNotifyStatusUpdate_ReadyToExitError(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)

	// Set status to confirm received (IsReadyToExit returns true when status != Pending)
	confirmReceived := InFlightStatusConfirmReceived
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			InFlightStatus: &confirmReceived,
		},
	})

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}

	// Try to change status when ready to exit (not a resume case - not suspending->pending)
	suspending := InFlightStatusSuspending
	updateRequired, err := it.NotifyStatusUpdate(ctx, suspending)
	assert.Error(t, err)
	assert.False(t, updateRequired)
	// Should not set newStatus when error occurs
	assert.Nil(t, it.newStatus)
}
