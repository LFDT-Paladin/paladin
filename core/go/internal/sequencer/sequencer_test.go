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

package sequencer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	coordinatorTx "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/mocks/blockindexermocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/blockindexer"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newValidatedInvokeTx(contractAddr *pldtypes.EthAddress, domain string, submitMode pldapi.SubmitMode) *components.ValidatedTransaction {
	txID := uuid.New()
	return &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: submitMode.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: domain,
					To:     contractAddr,
					From:   "alice@test-node",
				},
			},
			Function: &components.ResolvedFunction{
				Definition: &abi.Entry{Name: "transfer", Type: abi.Function},
			},
		},
	}
}

func newDeployValidatedTx() *components.ValidatedTransaction {
	txID := uuid.New()
	return &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "alice@test-node",
					To:     nil,
					Data:   pldtypes.RawJSON(`{}`),
				},
			},
		},
	}
}

func goodDeployTxForEvaluate() *components.PrivateContractDeploy {
	to := pldtypes.RandAddress()
	entry := &abi.Entry{Name: "set", Type: abi.Function, Inputs: abi.ParameterArray{{Type: "uint256", Name: "value"}}}
	inputs, _ := entry.Inputs.ParseJSON([]byte(`{"value":0}`))
	return &components.PrivateContractDeploy{
		ID:     uuid.New(),
		Domain: "test-domain",
		Signer: "signer@test-node",
		InvokeTransaction: &components.EthTransaction{
			FunctionABI: entry,
			To:          *to,
			Inputs:      inputs,
		},
	}
}

func TestSequencerManager_handleDeployTx_NoDomain(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	err := sm.handleDeployTx(ctx, &components.PrivateContractDeploy{})
	require.Error(t, err)
}

func TestSequencerManager_handleDeployTx_DomainNotFound(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mocks.domainManager.EXPECT().GetDomainByName(ctx, "missing").Return(nil, errors.New("not found")).Once()

	err := sm.handleDeployTx(ctx, &components.PrivateContractDeploy{Domain: "missing"})
	require.Error(t, err)
}

func TestSequencerManager_handleDeployTx_InitDeployError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainManager.EXPECT().GetDomainByName(ctx, "test-domain").Return(mockDomain, nil).Once()
	mockDomain.EXPECT().InitDeploy(ctx, mock.Anything).Return(errors.New("init failed")).Once()

	err := sm.handleDeployTx(ctx, &components.PrivateContractDeploy{Domain: "test-domain"})
	require.Error(t, err)
}

func TestSequencerManager_handleDeployTx_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainManager.EXPECT().GetDomainByName(ctx, "test-domain").Return(mockDomain, nil).Once()
	mockDomain.EXPECT().InitDeploy(ctx, mock.Anything).Return(nil).Once()
	done := make(chan struct{})
	mockDomain.EXPECT().PrepareDeploy(mock.Anything, mock.Anything).Return(errors.New("stop background loop")).Once()
	mocks.metrics.EXPECT().IncDispatchedTransactions().Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, req *syncpoints.TransactionFinalizeRequest, onCommit func(context.Context), onRollback func(context.Context, error)) {
		close(done)
	}).Once()

	err := sm.handleDeployTx(ctx, &components.PrivateContractDeploy{Domain: "test-domain"})
	require.NoError(t, err)
	<-done
}

func TestSequencerManager_deploymentLoop_ResolveVerifierError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	identityResolver := componentsmocks.NewIdentityResolver(t)

	mocks.components.EXPECT().IdentityResolver().Return(identityResolver).Once()
	identityResolver.EXPECT().ResolveVerifier(ctx, "lookup1", "alg", "type").Return("", errors.New("resolve failed")).Once()

	mockDomain := componentsmocks.NewDomain(t)
	tx := &components.PrivateContractDeploy{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{Lookup: "lookup1", Algorithm: "alg", VerifierType: "type"},
		},
	}
	sm.deploymentLoop(ctx, mockDomain, tx)
}

func TestSequencerManager_deploymentLoop_EvaluateDeploymentError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	identityResolver := componentsmocks.NewIdentityResolver(t)

	mocks.components.EXPECT().IdentityResolver().Return(identityResolver).Once()
	identityResolver.EXPECT().ResolveVerifier(ctx, "lookup1", "alg", "type").Return("0x1234", nil).Once()

	mockDomain := componentsmocks.NewDomain(t)
	mockDomain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(errors.New("prepare failed")).Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(ctx, mock.Anything, mock.Anything, mock.Anything).Once()

	tx := &components.PrivateContractDeploy{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{Lookup: "lookup1", Algorithm: "alg", VerifierType: "type"},
		},
	}
	sm.deploymentLoop(ctx, mockDomain, tx)
	require.Len(t, tx.Verifiers, 1)
}

func TestSequencerManager_deploymentLoop_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	identityResolver := componentsmocks.NewIdentityResolver(t)

	mocks.components.EXPECT().IdentityResolver().Return(identityResolver).Once()
	identityResolver.EXPECT().ResolveVerifier(ctx, "lookup1", "alg", "type").Return("0x1234", nil).Once()

	mockDomain := componentsmocks.NewDomain(t)
	deployTx := goodDeployTxForEvaluate()
	mockDomain.EXPECT().PrepareDeploy(ctx, mock.Anything).Run(func(_ context.Context, tx *components.PrivateContractDeploy) {
		tx.Signer = deployTx.Signer
		tx.InvokeTransaction = deployTx.InvokeTransaction
	}).Return(nil)

	from := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().ResolveEthAddressBatchNewDatabaseTX(ctx, []string{"signer"}).Return([]*pldtypes.EthAddress{from}, nil).Once()
	mocks.publicTxManager.EXPECT().ValidateTransaction(ctx, nil, mock.Anything).Return(nil).Once()
	mocks.syncPoints.EXPECT().PersistDeployDispatchBatch(ctx, mock.Anything, mock.Anything).Return(nil).Once()

	tx := &components.PrivateContractDeploy{
		ID: uuid.New(),
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{Lookup: "lookup1", Algorithm: "alg", VerifierType: "type"},
		},
	}
	sm.deploymentLoop(ctx, mockDomain, tx)
}

func TestSequencerManager_evaluateDeployment_PrepareDeployError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	tx := &components.PrivateContractDeploy{ID: uuid.New()}
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(errors.New("prepare failed")).Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(ctx, mock.Anything, mock.Anything, mock.Anything).Once()

	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.Error(t, err)
}

func TestSequencerManager_evaluateDeployment_NonLocalSigner(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	tx := &components.PrivateContractDeploy{ID: uuid.New(), Signer: "signer@other-node"}
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(nil).Once()

	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.Error(t, err)
}

func TestSequencerManager_evaluateDeployment_ResolveAddressError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	tx := goodDeployTxForEvaluate()
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(nil).Once()
	mocks.keyManager.EXPECT().ResolveEthAddressBatchNewDatabaseTX(ctx, []string{"signer"}).Return(nil, errors.New("key error")).Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(ctx, mock.Anything, mock.Anything, mock.Anything).Once()

	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.Error(t, err)
}

func TestSequencerManager_evaluateDeployment_DeployTransactionNotImplemented(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	tx := &components.PrivateContractDeploy{
		ID:     uuid.New(),
		Signer: "signer@test-node",
		DeployTransaction: &components.EthDeployTransaction{
			ConstructorABI: &abi.Entry{Name: "constructor", Type: abi.Constructor},
		},
	}
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(nil).Once()
	from := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().ResolveEthAddressBatchNewDatabaseTX(ctx, []string{"signer"}).Return([]*pldtypes.EthAddress{from}, nil).Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(ctx, mock.Anything, mock.Anything, mock.Anything).Once()

	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.Error(t, err)
}

func TestSequencerManager_evaluateDeployment_NeitherInvokeNorDeploy(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	tx := &components.PrivateContractDeploy{ID: uuid.New(), Signer: "signer@test-node"}
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(nil).Once()
	from := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().ResolveEthAddressBatchNewDatabaseTX(ctx, []string{"signer"}).Return([]*pldtypes.EthAddress{from}, nil).Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(ctx, mock.Anything, mock.Anything, mock.Anything).Once()

	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.Error(t, err)
}

func TestSequencerManager_evaluateDeployment_ValidateTransactionError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	tx := goodDeployTxForEvaluate()
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(nil).Once()
	from := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().ResolveEthAddressBatchNewDatabaseTX(ctx, []string{"signer"}).Return([]*pldtypes.EthAddress{from}, nil).Once()
	mocks.publicTxManager.EXPECT().ValidateTransaction(ctx, nil, mock.Anything).Return(errors.New("validate failed")).Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(ctx, mock.Anything, mock.Anything, mock.Anything).Once()

	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.Error(t, err)
}

func TestSequencerManager_evaluateDeployment_PersistError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	tx := goodDeployTxForEvaluate()
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(nil).Once()
	from := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().ResolveEthAddressBatchNewDatabaseTX(ctx, []string{"signer"}).Return([]*pldtypes.EthAddress{from}, nil).Once()
	mocks.publicTxManager.EXPECT().ValidateTransaction(ctx, nil, mock.Anything).Return(nil).Once()
	mocks.syncPoints.EXPECT().PersistDeployDispatchBatch(ctx, mock.Anything, mock.Anything).Return(errors.New("persist failed")).Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(ctx, mock.Anything, mock.Anything, mock.Anything).Once()

	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.Error(t, err)
}

func TestSequencerManager_evaluateDeployment_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mockDomain := componentsmocks.NewDomain(t)
	tx := goodDeployTxForEvaluate()
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(nil).Once()
	from := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().ResolveEthAddressBatchNewDatabaseTX(ctx, []string{"signer"}).Return([]*pldtypes.EthAddress{from}, nil).Once()
	mocks.publicTxManager.EXPECT().ValidateTransaction(ctx, nil, mock.Anything).Return(nil).Once()
	mocks.syncPoints.EXPECT().PersistDeployDispatchBatch(ctx, mock.Anything, mock.Anything).Return(nil).Once()

	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.NoError(t, err)
}

func TestSequencerManager_revertDeploy_FinalizeRetry(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	tx := &components.PrivateContractDeploy{ID: uuid.New(), Domain: "test-domain", From: "alice@test-node"}
	callCount := 0
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(ctx, mock.Anything, mock.Anything, mock.Anything).Run(
		func(_ context.Context, _ *syncpoints.TransactionFinalizeRequest, onCommit func(context.Context), onRollback func(context.Context, error)) {
			callCount++
			if callCount == 1 {
				onRollback(ctx, errors.New("finalize failed"))
			} else {
				onCommit(ctx)
			}
		},
	).Twice()

	err := sm.revertDeploy(ctx, tx, errors.New("root cause"))
	require.Error(t, err)
	assert.Equal(t, 2, callCount)
}

func TestSequencerManager_HandleNewTx_BlockedByDependencies(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	txi := newValidatedInvokeTx(pldtypes.RandAddress(), "test-domain", pldapi.SubmitModeAuto)
	mocks.txManager.EXPECT().BlockedByDependencies(ctx, dbTX, txi).Return(true, nil).Once()

	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.NoError(t, err)
}

func TestSequencerManager_HandleNewTx_DeployNotSupported(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	txi := newDeployValidatedTx()
	txi.Transaction.SubmitMode = pldapi.SubmitModeExternal.Enum()
	mocks.txManager.EXPECT().BlockedByDependencies(ctx, dbTX, txi).Return(false, nil).Once()

	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.Error(t, err)
}

func TestSequencerManager_HandleNewTx_DeployAuto(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	txi := newDeployValidatedTx()
	mocks.txManager.EXPECT().BlockedByDependencies(ctx, dbTX, txi).Return(false, nil).Once()

	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainManager.EXPECT().GetDomainByName(mock.Anything, "test-domain").Return(mockDomain, nil).Once()
	mockDomain.EXPECT().InitDeploy(mock.Anything, mock.Anything).Return(nil).Once()
	done := make(chan struct{})
	mockDomain.EXPECT().PrepareDeploy(mock.Anything, mock.Anything).Return(errors.New("stop")).Once()
	mocks.metrics.EXPECT().IncDispatchedTransactions().Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, req *syncpoints.TransactionFinalizeRequest, onCommit func(context.Context), onRollback func(context.Context, error)) {
		close(done)
	}).Once()

	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.NoError(t, err)
	<-done
}

func TestSequencerManager_HandleNewTx_FunctionNotProvided(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	txi := newValidatedInvokeTx(pldtypes.RandAddress(), "test-domain", pldapi.SubmitModeAuto)
	txi.Function = nil
	mocks.txManager.EXPECT().BlockedByDependencies(ctx, dbTX, txi).Return(false, nil).Once()

	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.Error(t, err)
}

func TestSequencerManager_HandleNewTx_InvokeSuccess(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	txi := newValidatedInvokeTx(contractAddr, "test-domain", pldapi.SubmitModeAuto)
	mocks.txManager.EXPECT().BlockedByDependencies(ctx, dbTX, txi).Return(false, nil).Once()

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("test-domain").Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitTransaction(ctx, mock.Anything, mock.Anything).Run(func(_ context.Context, ptx *components.PrivateTransaction, _ *components.ResolvedTransaction) {
		ptx.PreAssembly = &prototk.TransactionPreAssembly{}
	}).Return(nil).Once()

	done := make(chan struct{})
	dbTX.EXPECT().AddPostCommit(mock.Anything).Run(func(fn func(context.Context)) {
		fn(ctx)
		close(done)
	}).Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.Anything).Once()
	mocks.metrics.EXPECT().IncAcceptedTransactions().Once()

	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.NoError(t, err)
	<-done
}

func TestSequencerManager_HandleTxResume_BlockedByDependencies(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	txi := newValidatedInvokeTx(pldtypes.RandAddress(), "test-domain", pldapi.SubmitModeAuto)
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, dbTX)
		},
	).Once()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, dbTX, txi).Return(true, nil).Once()

	err := sm.HandleTxResume(ctx, txi)
	require.NoError(t, err)
}

func TestSequencerManager_HandleTxResume_DeployAuto(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	txi := newDeployValidatedTx()
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, dbTX)
		},
	).Once()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, dbTX, txi).Return(false, nil).Once()

	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainManager.EXPECT().GetDomainByName(mock.Anything, "test-domain").Return(mockDomain, nil).Once()
	mockDomain.EXPECT().InitDeploy(mock.Anything, mock.Anything).Return(nil).Once()
	done := make(chan struct{})
	mockDomain.EXPECT().PrepareDeploy(mock.Anything, mock.Anything).Return(errors.New("stop")).Once()
	mocks.metrics.EXPECT().IncDispatchedTransactions().Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, req *syncpoints.TransactionFinalizeRequest, onCommit func(context.Context), onRollback func(context.Context, error)) {
		close(done)
	}).Once()

	err := sm.HandleTxResume(ctx, txi)
	require.NoError(t, err)
	<-done
}

func TestSequencerManager_HandleTxResume_FunctionNotProvided(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	txi := newValidatedInvokeTx(pldtypes.RandAddress(), "test-domain", pldapi.SubmitModeAuto)
	txi.Function = nil
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, dbTX)
		},
	).Once()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, dbTX, txi).Return(false, nil).Once()

	err := sm.HandleTxResume(ctx, txi)
	require.Error(t, err)
}

func TestSequencerManager_HandleTxResume_InvokeResume(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	txi := newValidatedInvokeTx(contractAddr, "test-domain", pldapi.SubmitModeExternal)
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, dbTX)
		},
	).Once()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, dbTX, txi).Return(false, nil).Once()

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("test-domain").Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(mock.Anything, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitTransaction(mock.Anything, mock.Anything, mock.Anything).Run(func(_ context.Context, ptx *components.PrivateTransaction, _ *components.ResolvedTransaction) {
		ptx.PreAssembly = &prototk.TransactionPreAssembly{}
	}).Return(nil).Once()
	mocks.originator.EXPECT().QueueEvent(mock.Anything, mock.Anything).Once()

	err := sm.HandleTxResume(ctx, txi)
	require.NoError(t, err)
}

func TestSequencerManager_handleTx_EmptyContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	empty := pldtypes.EthAddress{}
	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: &empty}},
	}
	err := sm.handleTx(ctx, dbTX, &components.PrivateTransaction{}, localTx, false)
	require.Error(t, err)
}

func TestSequencerManager_handleTx_DomainMismatch(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr, Domain: "wrong-domain"}},
	}
	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("right-domain").Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()

	err := sm.handleTx(ctx, dbTX, &components.PrivateTransaction{Address: *contractAddr}, localTx, false)
	require.Error(t, err)
}

func TestSequencerManager_handleTx_NilPreAssembly(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr, Domain: "test-domain"}},
	}
	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("test-domain").Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitTransaction(ctx, mock.Anything, localTx).Return(nil).Once()

	err := sm.handleTx(ctx, dbTX, &components.PrivateTransaction{Address: *contractAddr}, localTx, false)
	require.Error(t, err)
}

func TestSequencerManager_HandleTransactionCollected_Loaded(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	signer := pldtypes.RandAddress()
	txID := uuid.New()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().TryQueueEvent(ctx, mock.Anything).Return(true).Once()

	err := sm.HandleTransactionCollected(ctx, signer.String(), contractAddr.String(), txID)
	require.NoError(t, err)
}

func TestSequencerManager_HandleTransactionCollected_NotLoaded(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	signer := pldtypes.RandAddress()
	txID := uuid.New()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	err := sm.HandleTransactionCollected(ctx, signer.String(), contractAddr.String(), txID)
	require.NoError(t, err)
}

func TestSequencerManager_HandleNonceAssigned_Loaded(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().TryQueueEvent(ctx, mock.Anything).Return(true).Once()

	err := sm.HandleNonceAssigned(ctx, 42, contractAddr.String(), txID)
	require.NoError(t, err)
}

func TestSequencerManager_HandlePublicTXSubmission_Deploy(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	err := sm.HandlePublicTXSubmission(ctx, dbTX, uuid.New(), &pldapi.PublicTxWithBinding{
		PublicTx: &pldapi.PublicTx{To: nil},
	})
	require.NoError(t, err)
}

func TestSequencerManager_HandlePublicTXSubmission_LocalSender(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txHash := pldtypes.RandBytes32()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	txID := uuid.New()

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().TryQueueEvent(ctx, mock.Anything).Return(true).Once()

	err := sm.HandlePublicTXSubmission(ctx, dbTX, txID, &pldapi.PublicTxWithBinding{
		PublicTx: &pldapi.PublicTx{
			To:              contractAddr,
			TransactionHash: &txHash,
		},
		PublicTxBinding: pldapi.PublicTxBinding{
			TransactionContractAddress: contractAddr.String(),
			TransactionSender:          "alice@test-node",
		},
	})
	require.NoError(t, err)
}

func TestSequencerManager_HandlePublicTXSubmission_RemoteSender(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txHash := pldtypes.RandBytes32()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	txID := uuid.New()

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().TryQueueEvent(ctx, mock.Anything).Return(true).Once()
	mocks.transportManager.EXPECT().SendReliable(ctx, dbTX, mock.Anything).Return(nil).Once()

	err := sm.HandlePublicTXSubmission(ctx, dbTX, txID, &pldapi.PublicTxWithBinding{
		PublicTx: &pldapi.PublicTx{
			To:              contractAddr,
			TransactionHash: &txHash,
		},
		PublicTxBinding: pldapi.PublicTxBinding{
			TransactionContractAddress: contractAddr.String(),
			TransactionSender:          "alice@other-node",
		},
	})
	require.NoError(t, err)
}

func TestSequencerManager_HandlePublicTXSubmission_RemoteSenderError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txHash := pldtypes.RandBytes32()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	txID := uuid.New()

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().TryQueueEvent(ctx, mock.Anything).Return(true).Once()
	mocks.transportManager.EXPECT().SendReliable(ctx, dbTX, mock.Anything).Return(errors.New("send failed")).Once()

	err := sm.HandlePublicTXSubmission(ctx, dbTX, txID, &pldapi.PublicTxWithBinding{
		PublicTx: &pldapi.PublicTx{
			To:              contractAddr,
			TransactionHash: &txHash,
		},
		PublicTxBinding: pldapi.PublicTxBinding{
			TransactionContractAddress: contractAddr.String(),
			TransactionSender:          "alice@other-node",
		},
	})
	require.Error(t, err)
}

func TestSequencerManager_handleTransactionConfirmedSuccess_DeploySkip(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	contractAddr := pldtypes.RandAddress()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	completion := &components.TxCompletion{
		ReceiptInput: components.ReceiptInput{
			TransactionID:   uuid.New(),
			ContractAddress: contractAddr,
		},
	}

	sm.handleTransactionConfirmedSuccess(ctx, completion, nil)
}

func TestSequencerManager_HandleChainedTransactionOutcome_UnknownReceiptType(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	sm.HandleChainedTransactionOutcome(ctx, *contractAddr, txID, components.ReceiptType(999), "", nil, pldtypes.OnChainLocation{})
}

func TestSequencerManager_HandleDirectTransactionRevert_NilFrom(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	mocks.metrics.EXPECT().IncRevertedTransactions().Once()
	err := sm.HandleDirectTransactionRevert(ctx, dbTX, []*components.PublicTxMatch{{
		PaladinTXReference: components.PaladinTXReference{TransactionID: txID},
		IndexedTransactionNotify: &blockindexer.IndexedTransactionNotify{
			IndexedTransaction: pldapi.IndexedTransaction{
				From: nil,
				To:   contractAddr,
			},
		},
	}})
	require.Error(t, err)
}

func TestSequencerManager_HandleDirectTransactionRevert_NoSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	from := pldtypes.RandAddress()
	txID := uuid.New()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	mocks.metrics.EXPECT().IncRevertedTransactions().Once()
	err := sm.HandleDirectTransactionRevert(ctx, dbTX, []*components.PublicTxMatch{{
		PaladinTXReference: components.PaladinTXReference{TransactionID: txID},
		IndexedTransactionNotify: &blockindexer.IndexedTransactionNotify{
			IndexedTransaction: pldapi.IndexedTransaction{
				From: from,
				To:   contractAddr,
			},
		},
	}})
	require.NoError(t, err)
}

func TestSequencerManager_BuildNullifiers_SkipsMissingNullifierFields(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, nil)
		},
	).Once()

	nullifiers, err := sm.BuildNullifiers(ctx, []*components.StateDistributionWithData{
		{StateDistribution: components.StateDistribution{StateID: "0x1234", IdentityLocator: "alice@test-node"}},
	})
	require.NoError(t, err)
	assert.Empty(t, nullifiers)
}

func TestSequencerManager_BuildNullifiers_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	algo := "ECDSA"
	vType := "eth_address"
	pType := "raw"
	stateData := pldtypes.RawJSON(`{"k":"v"}`)

	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, dbTX)
		},
	).Once()
	kr := componentsmocks.NewKeyResolver(t)
	mocks.keyManager.EXPECT().KeyResolverForDBTX(dbTX).Return(kr).Once()
	kr.EXPECT().ResolveKey(mock.Anything, "alice", algo, vType).Return(&pldapi.KeyMappingAndVerifier{}, nil).Once()
	mocks.keyManager.EXPECT().Sign(mock.Anything, mock.Anything, pType, stateData.Bytes()).Return([]byte{1, 2, 3}, nil).Once()

	nullifiers, err := sm.BuildNullifiers(ctx, []*components.StateDistributionWithData{{
		StateDistribution: components.StateDistribution{
			StateID:               "0x0102",
			IdentityLocator:       "alice@test-node",
			NullifierAlgorithm:    &algo,
			NullifierVerifierType: &vType,
			NullifierPayloadType:  &pType,
		},
		StateData: stateData,
	}})
	require.NoError(t, err)
	require.Len(t, nullifiers, 1)
}

func TestSequencerManager_BuildNullifier_NotLocalIdentity(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	algo := "ECDSA"
	vType := "eth_address"
	pType := "raw"
	_, err := sm.BuildNullifier(ctx, nil, &components.StateDistributionWithData{
		StateDistribution: components.StateDistribution{
			StateID:               "0x0102",
			IdentityLocator:       "alice@other-node",
			NullifierAlgorithm:    &algo,
			NullifierVerifierType: &vType,
			NullifierPayloadType:  &pType,
		},
		StateData: pldtypes.RawJSON(`{}`),
	})
	require.Error(t, err)
}

func TestSequencerManager_BuildNullifier_SignError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	algo := "ECDSA"
	vType := "eth_address"
	pType := "raw"
	kr := componentsmocks.NewKeyResolver(t)
	kr.EXPECT().ResolveKey(mock.Anything, "alice", algo, vType).Return(nil, errors.New("resolve failed")).Once()

	_, err := sm.BuildNullifier(ctx, kr, &components.StateDistributionWithData{
		StateDistribution: components.StateDistribution{
			StateID:               "0x0102",
			IdentityLocator:       "alice@test-node",
			NullifierAlgorithm:    &algo,
			NullifierVerifierType: &vType,
			NullifierPayloadType:  &pType,
		},
		StateData: pldtypes.RawJSON(`{}`),
	})
	require.Error(t, err)
}

func TestSequencerManager_CallPrivateSmartContract_Success(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	call := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				To:     contractAddr,
				Domain: "test-domain",
			},
		},
		Function: &components.ResolvedFunction{
			Definition: &abi.Entry{Name: "get", Type: abi.Function},
		},
	}

	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Twice()
	mockDomain.EXPECT().Name().Return("test-domain").Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitCall(ctx, call).Return([]*prototk.ResolveVerifierRequest{
		{Lookup: "alice", Algorithm: "ECDSA", VerifierType: "eth_address"},
	}, nil).Once()

	identityResolver := componentsmocks.NewIdentityResolver(t)
	mocks.components.EXPECT().IdentityResolver().Return(identityResolver).Once()
	identityResolver.EXPECT().ResolveVerifier(ctx, "alice", "ECDSA", "eth_address").Return("0xabc", nil).Once()

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	mocks.stateManager.EXPECT().NewDomainQueryContext(ctx, mockDomain, *contractAddr).Return(mockDqc).Once()
	mockDqc.EXPECT().Close(mock.Anything).Once()
	mocks.domainAPI.EXPECT().ExecCall(mock.Anything, mockDqc, nil, call, mock.Anything).Return(&abi.ComponentValue{}, nil).Once()

	result, err := sm.CallPrivateSmartContract(ctx, call)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestSequencerManager_CallPrivateSmartContract_DomainMismatch(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	call := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr, Domain: "wrong"}},
	}
	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("right").Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()

	_, err := sm.CallPrivateSmartContract(ctx, call)
	require.Error(t, err)
}

func TestSequencerManager_BuildStateDistributions(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))
	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &prototk.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{From: "alice@test-node"},
		},
		PostAssembly: &components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				OutputStatesPotential: []*prototk.NewState{{DistributionList: []string{"alice@test-node"}}},
				InfoStatesPotential:   []*prototk.NewState{},
			},
			OutputStates: []*prototk.EndorsableState{{Id: stateID.String(), SchemaId: schemaID.String(), StateDataJson: `{}`}},
			InfoStates:   []*prototk.EndorsableState{},
		},
	}

	result, err := sm.BuildStateDistributions(ctx, tx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Local, 1)
}

func TestSequencerManager_PrivateTransactionsConfirmed_QueryPublicTxError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	txID := uuid.New()
	mocks.publicTxManager.EXPECT().QueryPublicTxForTransactions(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("query failed")).Once()

	sm.PrivateTransactionsConfirmed(ctx, []*components.TxCompletion{{
		ReceiptInput: components.ReceiptInput{TransactionID: txID},
		PSC:          mocks.domainAPI,
	}})
}

func TestSequencerManager_PrivateTransactionsConfirmed_MatchingPublicTxSuccessError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	txID := uuid.New()
	txHash := pldtypes.RandBytes32()
	mocks.publicTxManager.EXPECT().QueryPublicTxForTransactions(ctx, mock.Anything, mock.Anything, mock.Anything).Return(map[uuid.UUID][]*pldapi.PublicTx{
		txID: {{TransactionHash: &txHash}},
	}, nil).Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()

	sm.PrivateTransactionsConfirmed(ctx, []*components.TxCompletion{{
		ReceiptInput: components.ReceiptInput{
			TransactionID: txID,
			OnChain:       pldtypes.OnChainLocation{TransactionHash: txHash},
		},
		PSC: mocks.domainAPI,
	}})
}

func TestSequencerManager_PrivateTransactionsConfirmed_ChainedWithContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	txID := uuid.New()
	mocks.publicTxManager.EXPECT().QueryPublicTxForTransactions(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()

	sm.PrivateTransactionsConfirmed(ctx, []*components.TxCompletion{{
		ReceiptInput: components.ReceiptInput{
			TransactionID:   txID,
			ContractAddress: pldtypes.RandAddress(),
			OnChain:         pldtypes.OnChainLocation{TransactionHash: pldtypes.RandBytes32()},
		},
		PSC: mocks.domainAPI,
	}})
}

func TestSequencerManager_PrivateTransactionsConfirmed_ChainedCountError(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer func() { require.NoError(t, mp.Mock.ExpectationsWereMet()) }()
	mocks := newSequencerLifecycleTestMocksWithPersistence(t, mp.P)
	sm := newSequencerManagerForTesting(t, mocks)

	txID := uuid.New()
	mp.Mock.ExpectQuery("SELECT count\\(\\*\\).*chained_dispatches").WithArgs(txID).WillReturnError(errors.New("db error"))
	mocks.publicTxManager.EXPECT().QueryPublicTxForTransactions(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()

	sm.PrivateTransactionsConfirmed(ctx, []*components.TxCompletion{{
		ReceiptInput: components.ReceiptInput{TransactionID: txID},
		PSC:          mocks.domainAPI,
	}})
}

func TestSequencerManager_PrivateTransactionsConfirmed_ChainedCountZero(t *testing.T) {
	ctx := context.Background()
	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	defer func() { require.NoError(t, mp.Mock.ExpectationsWereMet()) }()
	mocks := newSequencerLifecycleTestMocksWithPersistence(t, mp.P)
	sm := newSequencerManagerForTesting(t, mocks)

	txID := uuid.New()
	mp.Mock.ExpectQuery("SELECT count\\(\\*\\).*chained_dispatches").WithArgs(txID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mocks.publicTxManager.EXPECT().QueryPublicTxForTransactions(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()

	sm.PrivateTransactionsConfirmed(ctx, []*components.TxCompletion{{
		ReceiptInput: components.ReceiptInput{TransactionID: txID},
		PSC:          mocks.domainAPI,
	}})
}

func TestSequencerManager_PrivateTransactionsConfirmed_MatchingPublicTx(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	txID := uuid.New()
	txHash := pldtypes.RandBytes32()
	nonce := pldtypes.HexUint64(5)
	mocks.publicTxManager.EXPECT().QueryPublicTxForTransactions(ctx, mock.Anything, mock.Anything, mock.Anything).Return(map[uuid.UUID][]*pldapi.PublicTx{
		txID: {{TransactionHash: &txHash, Nonce: &nonce}},
	}, nil).Once()
	mocks.metrics.EXPECT().IncConfirmedTransactions().Once()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinatorTx.ConfirmedSuccessEvent)
		return ok && event.TransactionID == txID && event.Nonce != nil
	})).Once()

	sm.PrivateTransactionsConfirmed(ctx, []*components.TxCompletion{{
		ReceiptInput: components.ReceiptInput{
			TransactionID: txID,
			OnChain:       pldtypes.OnChainLocation{TransactionHash: txHash},
		},
		PSC: mocks.domainAPI,
	}})
}

func TestSequencerManager_resumeIncompleteTransactions_MaxTransactionsOverride(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	sm.config.TransactionResumeMaxTransactions = confutil.P(2)
	sm.config.TransactionResumePageSize = confutil.P(1)

	callCount := 0
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, persistencemocks.NewDBTX(t))
		},
	).Once()
	mocks.txManager.EXPECT().QueryTransactionsResolved(mock.Anything, mock.Anything, nil, true).RunAndReturn(
		func(_ context.Context, _ *query.QueryJSON, _ persistence.DBTX, _ bool) ([]*components.ResolvedTransaction, error) {
			callCount++
			if callCount == 1 {
				return []*components.ResolvedTransaction{
					{Transaction: &pldapi.Transaction{
						ID:      confutil.P(uuid.New()),
						Created: pldtypes.Timestamp(time.Now().UnixNano()),
						TransactionBase: pldapi.TransactionBase{
							To: pldtypes.RandAddress(),
						},
					}},
				}, nil
			}
			return nil, nil
		},
	).Twice()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()

	sm.resumeIncompleteTransactions(ctx)
	assert.Equal(t, 2, callCount)
}

func TestSequencerManager_resumeIncompleteTransactions_Pagination(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	sm.config.TransactionResumePageSize = confutil.P(1)

	callCount := 0
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, persistencemocks.NewDBTX(t))
		},
	).Once()
	mocks.txManager.EXPECT().QueryTransactionsResolved(mock.Anything, mock.Anything, nil, true).RunAndReturn(
		func(_ context.Context, _ *query.QueryJSON, _ persistence.DBTX, _ bool) ([]*components.ResolvedTransaction, error) {
			callCount++
			if callCount == 1 {
				return []*components.ResolvedTransaction{
					{Transaction: &pldapi.Transaction{
						ID:      confutil.P(uuid.New()),
						Created: pldtypes.Timestamp(time.Now().UnixNano()),
						TransactionBase: pldapi.TransactionBase{
							To: pldtypes.RandAddress(),
						},
					}},
				}, nil
			}
			return nil, nil
		},
	).Twice()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()

	sm.resumeIncompleteTransactions(ctx)
	assert.Equal(t, 2, callCount)
}

func TestSequencerManager_resumeIncompleteTransactions_QueryError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mocks.txManager.EXPECT().QueryTransactionsResolved(mock.Anything, mock.Anything, nil, true).Return(nil, errors.New("query failed")).Once()

	sm.resumeIncompleteTransactions(ctx)
}

func TestSequencerManager_pollForIncompleteTransactions_ContextCancelDuringRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	blockIndexer := blockindexermocks.NewBlockIndexer(t)

	called := make(chan struct{})
	mocks.components.EXPECT().BlockIndexer().Return(blockIndexer).Once()
	blockIndexer.EXPECT().GetConfirmedBlockHeight(mock.Anything).RunAndReturn(func(context.Context) (pldtypes.HexUint64, error) {
		close(called)
		return 0, errors.New("not ready")
	}).Once()

	sm.pollForIncompleteTransactions(ctx, time.Second)
	<-called
}

func TestSequencerManager_pollForIncompleteTransactions_ContextCancelDuringTicker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	blockIndexer := blockindexermocks.NewBlockIndexer(t)

	mocks.components.EXPECT().BlockIndexer().Return(blockIndexer).Once()
	blockIndexer.EXPECT().GetConfirmedBlockHeight(mock.Anything).Return(pldtypes.HexUint64(100), nil).Once()

	// Each resumeIncompleteTransactions call invokes QueryTransactionsResolved once.
	// Reading the channel twice confirms both the initial resume call and at least one tick.
	calls := make(chan struct{}, 100)
	mocks.txManager.EXPECT().QueryTransactionsResolved(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, jq *query.QueryJSON, dbTX persistence.DBTX, pending bool) ([]*components.ResolvedTransaction, error) {
			calls <- struct{}{}
			return nil, nil
		},
	).Maybe()

	sm.pollForIncompleteTransactions(ctx, 20*time.Millisecond)
	<-calls // initial resume
	<-calls // first tick
}

func TestSequencerManager_pollForIncompleteTransactions_Disabled(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	sm.pollForIncompleteTransactions(ctx, 0)
}

func TestSequencerManager_handleTx_GetSmartContractError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr, Domain: "test-domain"}},
	}
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(nil, errors.New("not found")).Once()

	err := sm.handleTx(ctx, dbTX, &components.PrivateTransaction{Address: *contractAddr}, localTx, false)
	require.Error(t, err)
}

func TestSequencerManager_CallPrivateSmartContract_InitCallError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	call := &components.ResolvedTransaction{Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr}}}
	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("d").Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitCall(ctx, call).Return(nil, errors.New("init failed")).Once()
	_, err := sm.CallPrivateSmartContract(ctx, call)
	require.Error(t, err)
}

func TestSequencerManager_CallPrivateSmartContract_ResolveVerifierError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	call := &components.ResolvedTransaction{Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr}}}
	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("d").Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitCall(ctx, call).Return([]*prototk.ResolveVerifierRequest{{Lookup: "a"}}, nil).Once()
	identityResolver := componentsmocks.NewIdentityResolver(t)
	mocks.components.EXPECT().IdentityResolver().Return(identityResolver).Once()
	identityResolver.EXPECT().ResolveVerifier(ctx, "a", "", "").Return("", errors.New("resolve failed")).Once()
	_, err := sm.CallPrivateSmartContract(ctx, call)
	require.Error(t, err)
}

func TestSequencerManager_resumeIncompleteTransactions_HandleTxResumeError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).Return(errors.New("tx wrapper failed")).Once()
	mocks.txManager.EXPECT().QueryTransactionsResolved(mock.Anything, mock.Anything, nil, true).Return([]*components.ResolvedTransaction{{
		Transaction: &pldapi.Transaction{ID: confutil.P(uuid.New()), Created: pldtypes.Timestamp(time.Now().UnixNano()), TransactionBase: pldapi.TransactionBase{To: pldtypes.RandAddress()}},
	}}, nil).Once()
	sm.resumeIncompleteTransactions(ctx)
}

func TestSequencerManager_evaluateDeployment_EncodeCallDataError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	mockDomain := componentsmocks.NewDomain(t)
	to := pldtypes.RandAddress()
	tx := &components.PrivateContractDeploy{
		ID: uuid.New(), Signer: "signer@test-node",
		InvokeTransaction: &components.EthTransaction{
			FunctionABI: &abi.Entry{Name: "set", Type: abi.Function, Inputs: abi.ParameterArray{{Type: "uint256", Name: "value"}}},
			To:          *to, Inputs: nil,
		},
	}
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(nil).Once()
	from := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().ResolveEthAddressBatchNewDatabaseTX(ctx, []string{"signer"}).Return([]*pldtypes.EthAddress{from}, nil).Once()
	mocks.syncPoints.EXPECT().QueueTransactionFinalize(ctx, mock.Anything, mock.Anything, mock.Anything).Once()
	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.Error(t, err)
}

func TestSequencerManager_pollForIncompleteTransactions_BlockIndexerRetryTimer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	blockIndexer := blockindexermocks.NewBlockIndexer(t)

	mocks.components.EXPECT().BlockIndexer().Return(blockIndexer).Twice()
	// First call: block indexer not yet ready
	blockIndexer.EXPECT().GetConfirmedBlockHeight(mock.Anything).Return(pldtypes.HexUint64(0), errors.New("not ready")).Once()
	// Second call: ready — retry timer fired
	blockIndexer.EXPECT().GetConfirmedBlockHeight(mock.Anything).Return(pldtypes.HexUint64(100), nil).Once()

	resumed := make(chan struct{})
	mocks.txManager.EXPECT().QueryTransactionsResolved(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ *query.QueryJSON, _ persistence.DBTX, _ bool) ([]*components.ResolvedTransaction, error) {
			close(resumed)
			return nil, nil
		}).Once()

	go sm.pollForIncompleteTransactions(ctx, time.Hour)
	<-resumed
}

func TestSequencerManager_resumeIncompleteTransactions_LimitTrim(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	sm.config.TransactionResumeMaxTransactions = confutil.P(3)
	sm.config.TransactionResumePageSize = confutil.P(2)

	queryCount := 0
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, persistencemocks.NewDBTX(t))
		},
	).Twice()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Twice()
	mocks.txManager.EXPECT().QueryTransactionsResolved(mock.Anything, mock.Anything, nil, true).RunAndReturn(
		func(_ context.Context, q *query.QueryJSON, _ persistence.DBTX, _ bool) ([]*components.ResolvedTransaction, error) {
			queryCount++
			if queryCount == 1 {
				return []*components.ResolvedTransaction{
					{Transaction: &pldapi.Transaction{
						ID:              confutil.P(uuid.New()),
						Created:         pldtypes.Timestamp(time.Now().UnixNano()),
						TransactionBase: pldapi.TransactionBase{To: pldtypes.RandAddress()},
					}},
					{Transaction: &pldapi.Transaction{
						ID:              confutil.P(uuid.New()),
						Created:         pldtypes.Timestamp(time.Now().UnixNano() + 1),
						TransactionBase: pldapi.TransactionBase{To: pldtypes.RandAddress()},
					}},
				}, nil
			}
			require.NotNil(t, q.Limit)
			require.Equal(t, 1, *q.Limit)
			return nil, nil
		},
	).Times(2)

	sm.resumeIncompleteTransactions(ctx)
	assert.Equal(t, 2, queryCount)
}

func TestSequencerManager_evaluateDeployment_InvalidSignerValidateError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	mockDomain := componentsmocks.NewDomain(t)
	tx := goodDeployTxForEvaluate()
	tx.Signer = ""
	mockDomain.EXPECT().PrepareDeploy(ctx, tx).Return(nil).Once()
	err := sm.evaluateDeployment(ctx, mockDomain, tx)
	require.Error(t, err)
}

func TestSequencerManager_HandleNewTx_BlockedByDependenciesError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	txi := newValidatedInvokeTx(pldtypes.RandAddress(), "test-domain", pldapi.SubmitModeAuto)
	mocks.txManager.EXPECT().BlockedByDependencies(ctx, dbTX, txi).Return(false, errors.New("deps error")).Once()
	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.Error(t, err)
}

func TestSequencerManager_HandleNewTx_SubmitModeExternal(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	txi := newValidatedInvokeTx(contractAddr, "test-domain", pldapi.SubmitModeExternal)

	mocks.txManager.EXPECT().BlockedByDependencies(ctx, dbTX, txi).Return(false, nil).Once()
	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("test-domain").Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitTransaction(ctx, mock.MatchedBy(func(ptx *components.PrivateTransaction) bool {
		return ptx.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION
	}), mock.Anything).Run(func(_ context.Context, ptx *components.PrivateTransaction, _ *components.ResolvedTransaction) {
		ptx.PreAssembly = &prototk.TransactionPreAssembly{}
	}).Return(nil).Once()
	done := make(chan struct{})
	dbTX.EXPECT().AddPostCommit(mock.Anything).Run(func(fn func(context.Context)) { fn(ctx); close(done) }).Once()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.Anything).Once()
	mocks.metrics.EXPECT().IncAcceptedTransactions().Once()

	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.NoError(t, err)
	<-done
}

func TestSequencerManager_HandleTxResume_BlockedByDependenciesError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	txi := newValidatedInvokeTx(pldtypes.RandAddress(), "test-domain", pldapi.SubmitModeAuto)
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, dbTX)
		},
	).Once()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, dbTX, txi).Return(false, errors.New("deps error")).Once()
	err := sm.HandleTxResume(ctx, txi)
	require.Error(t, err)
}

func TestSequencerManager_HandleTxResume_DeployNotSupported(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	txi := newDeployValidatedTx()
	txi.Transaction.SubmitMode = pldapi.SubmitModeExternal.Enum()
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, dbTX)
		},
	).Once()
	mocks.txManager.EXPECT().BlockedByDependencies(mock.Anything, dbTX, txi).Return(false, nil).Once()
	err := sm.HandleTxResume(ctx, txi)
	require.Error(t, err)
}

func TestSequencerManager_handleTx_InitTransactionError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr, Domain: "test-domain"}},
		Function:    &components.ResolvedFunction{Definition: &abi.Entry{Name: "f", Type: abi.Function}},
	}
	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("test-domain").Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitTransaction(ctx, mock.Anything, localTx).Return(errors.New("init failed")).Once()
	err := sm.handleTx(ctx, dbTX, &components.PrivateTransaction{Address: *contractAddr}, localTx, false)
	require.Error(t, err)
}

func TestSequencerManager_handleTx_LoadSequencerError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr, Domain: "test-domain"}},
		Function:    &components.ResolvedFunction{Definition: &abi.Entry{Name: "f", Type: abi.Function}},
	}
	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("test-domain").Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitTransaction(ctx, mock.Anything, localTx).Run(func(_ context.Context, ptx *components.PrivateTransaction, _ *components.ResolvedTransaction) {
		ptx.PreAssembly = &prototk.TransactionPreAssembly{}
	}).Return(nil).Once()
	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(nil, errors.New("create failed")).Once()
	err := sm.handleTx(ctx, dbTX, &components.PrivateTransaction{Address: *contractAddr}, localTx, false)
	require.Error(t, err)
}

func TestSequencerManager_handleTx_ChainedDependsOn(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	chainedID := uuid.New()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	localTx := &components.ResolvedTransaction{
		Transaction:      &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr, Domain: "test-domain"}},
		Function:         &components.ResolvedFunction{Definition: &abi.Entry{Name: "f", Type: abi.Function}},
		ChainedDependsOn: []uuid.UUID{chainedID},
	}
	mockDomain := componentsmocks.NewDomain(t)
	mocks.domainAPI.EXPECT().Domain().Return(mockDomain).Once()
	mockDomain.EXPECT().Name().Return("test-domain").Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, dbTX, *contractAddr).Return(mocks.domainAPI, nil).Once()
	mocks.domainAPI.EXPECT().InitTransaction(ctx, mock.Anything, localTx).Run(func(_ context.Context, ptx *components.PrivateTransaction, _ *components.ResolvedTransaction) {
		ptx.PreAssembly = &prototk.TransactionPreAssembly{}
	}).Return(nil).Once()
	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(nil, errors.New("load failed")).Once()
	err := sm.handleTx(ctx, dbTX, &components.PrivateTransaction{Address: *contractAddr}, localTx, false)
	require.Error(t, err)
}

func TestSequencerManager_HandlePublicTXSubmission_InvalidSender(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	txHash := pldtypes.RandBytes32()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	seq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	mocks.coordinator.EXPECT().TryQueueEvent(ctx, mock.Anything).Return(true).Once()
	err := sm.HandlePublicTXSubmission(ctx, dbTX, uuid.New(), &pldapi.PublicTxWithBinding{
		PublicTx: &pldapi.PublicTx{To: contractAddr, TransactionHash: &txHash},
		PublicTxBinding: pldapi.PublicTxBinding{
			TransactionContractAddress: contractAddr.String(),
			TransactionSender:          "not-a-valid-locator",
		},
	})
	require.Error(t, err)
}

func TestSequencerManager_BuildNullifiers_ResolveKeyError(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	dbTX := persistencemocks.NewDBTX(t)
	algo := "ECDSA"
	vType := "eth_address"
	pType := "raw"
	mocks.persistence.EXPECT().Transaction(mock.Anything, mock.Anything).RunAndReturn(
		func(txCtx context.Context, fn func(context.Context, persistence.DBTX) error) error {
			return fn(txCtx, dbTX)
		},
	).Once()
	kr := componentsmocks.NewKeyResolver(t)
	mocks.keyManager.EXPECT().KeyResolverForDBTX(dbTX).Return(kr).Once()
	kr.EXPECT().ResolveKey(mock.Anything, "alice", algo, vType).Return(nil, errors.New("resolve failed")).Once()
	_, err := sm.BuildNullifiers(ctx, []*components.StateDistributionWithData{{
		StateDistribution: components.StateDistribution{
			StateID:               "0x0102",
			IdentityLocator:       "alice@test-node",
			NullifierAlgorithm:    &algo,
			NullifierVerifierType: &vType,
			NullifierPayloadType:  &pType,
		},
		StateData: pldtypes.RawJSON(`{}`),
	}})
	require.Error(t, err)
}

func TestSequencerManager_CallPrivateSmartContract_GetSmartContractError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)
	call := &components.ResolvedTransaction{Transaction: &pldapi.Transaction{TransactionBase: pldapi.TransactionBase{To: contractAddr}}}
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(nil, errors.New("not found")).Once()
	_, err := sm.CallPrivateSmartContract(ctx, call)
	require.Error(t, err)
}
