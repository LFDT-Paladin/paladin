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

package transaction

import (
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_buildDispatchBatch_ChainedPrivateBranch(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().PreparedPrivateTransaction(&pldapi.TransactionInput{})
	txn, mocks := builder.Build(ctx)

	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)

	batch, err := txn.buildDispatchBatch(ctx)
	require.NoError(t, err)
	require.NotNil(t, batch)
	assert.Len(t, batch.PrivateDispatches, 1)
	assert.Nil(t, batch.PublicDispatches)
	assert.Nil(t, batch.PreparedTransactions)
}

func Test_buildDispatchBatch_ChainedPrivateBranch_PrepareChainedReturnsError(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().PreparedPrivateTransaction(&pldapi.TransactionInput{})
	txn, mocks := builder.Build(ctx)

	prepareErr := errors.New("chained prepare failed")
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, prepareErr)

	batch, err := txn.buildDispatchBatch(ctx)
	require.Error(t, err)
	assert.Nil(t, batch)
	assert.Contains(t, err.Error(), "chained prepare failed")
}

func Test_buildDispatchBatch_PrepareTransactionBranch(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().Intent(prototk.TransactionSpecification_PREPARE_TRANSACTION).PreparedPrivateTransaction(&pldapi.TransactionInput{})
	txn, _ := builder.Build(ctx)

	batch, err := txn.buildDispatchBatch(ctx)
	require.NoError(t, err)
	require.NotNil(t, batch)
	assert.Len(t, batch.PreparedTransactions, 1)
	assert.Nil(t, batch.PublicDispatches)
	assert.Nil(t, batch.PrivateDispatches)
	assert.Equal(t, txn.pt.ID, batch.PreparedTransactions[0].ID)
}

func Test_buildDispatchBatch_PrepareTransactionBranch_PublicPrepared(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().Intent(prototk.TransactionSpecification_PREPARE_TRANSACTION).PreparedPublicTransaction(&pldapi.TransactionInput{})
	txn, _ := builder.Build(ctx)

	batch, err := txn.buildDispatchBatch(ctx)
	require.NoError(t, err)
	require.NotNil(t, batch)
	assert.Len(t, batch.PreparedTransactions, 1)
}

func Test_buildDispatchBatch_InvalidOutcome_SendWithNoPrepared(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build(ctx)

	batch, err := txn.buildDispatchBatch(ctx)
	require.Error(t, err)
	assert.Nil(t, batch)
	assert.Contains(t, err.Error(), "Prepare outcome unexpected")
}

func Test_buildDispatchBatch_InvalidOutcome_SendWithBothPrepared(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().PreparedPrivateTransaction(&pldapi.TransactionInput{}).PreparedPublicTransaction(&pldapi.TransactionInput{})
	txn, _ := builder.Build(ctx)

	batch, err := txn.buildDispatchBatch(ctx)
	require.Error(t, err)
	assert.Nil(t, batch)
	assert.Contains(t, err.Error(), "Prepare outcome unexpected")
}

func Test_buildDispatchBatch_PublicBranch(t *testing.T) {
	ctx := t.Context()
	gasVal := pldtypes.HexUint64(21000)
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().Signer(builder.GetOriginator().identityLocator).PreparedPublicTransaction(&pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Data:            pldtypes.RawJSON("[]"),
			PublicTxOptions: pldapi.PublicTxOptions{Gas: &gasVal},
		},
		ABI: abi.ABI{&abi.Entry{Type: abi.Function, Name: "test", Inputs: abi.ParameterArray{}}},
	})
	txn, mocks := builder.Build(ctx)

	mocks.KeyManager.On("ResolveEthAddressNewDatabaseTX", mock.Anything, mock.Anything).Return(pldtypes.RandAddress(), nil)
	mocks.PublicTxManager.On("ValidateTransactionNOTX", mock.Anything, mock.Anything).Return(nil)

	batch, err := txn.buildDispatchBatch(ctx)
	require.NoError(t, err)
	require.NotNil(t, batch)
	assert.Len(t, batch.PublicDispatches, 1)
	assert.Len(t, batch.PublicDispatches[0].PublicTxs, 1)
	assert.Nil(t, batch.PrivateDispatches)
	assert.Nil(t, batch.PreparedTransactions)
}

func Test_buildDispatchBatch_PublicBranch_BuildPublicTxSubmissionError(t *testing.T) {
	ctx := t.Context()
	gasVal := pldtypes.HexUint64(21000)
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().Signer(builder.GetOriginator().identityLocator).PreparedPublicTransaction(&pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Data:            pldtypes.RawJSON("[]"),
			PublicTxOptions: pldapi.PublicTxOptions{Gas: &gasVal},
		},
		ABI: abi.ABI{&abi.Entry{Type: abi.Function, Name: "test", Inputs: abi.ParameterArray{}}},
	})
	txn, mocks := builder.Build(ctx)
	mocks.KeyManager.On("ResolveEthAddressNewDatabaseTX", mock.Anything, mock.Anything).Return(nil, errors.New("resolve signer failed"))

	batch, err := txn.buildDispatchBatch(ctx)
	require.Error(t, err)
	assert.Nil(t, batch)
	assert.Contains(t, err.Error(), "resolve signer failed")
}

func Test_dispatch_PrepareTransactionReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build(ctx)
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything).Return(errors.New("prepare failed"))

	err := txn.dispatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prepare failed")
}

func Test_dispatch_BuildDispatchBatchReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build(ctx)
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything).Return(nil)

	err := txn.dispatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Prepare outcome unexpected")
}

func Test_dispatch_StateDistributionBuilderReturnsError(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().NoOutputStatesPotential()
	txn, mocks := builder.Build(ctx)
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)

	err := txn.dispatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state distribution")
}

func Test_dispatch_PersistDispatchBatchReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build(ctx)
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("persist failed"))

	err := txn.dispatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist failed")
}

func Test_dispatch_Success_ChainedPrivate(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build(ctx)
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.dispatch(ctx)
	require.NoError(t, err)
}

func Test_dispatch_Success_PrepareTransactionBranch(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().Intent(prototk.TransactionSpecification_PREPARE_TRANSACTION)
	txn, mocks := builder.Build(ctx)
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		pt := args.Get(1).(*components.PrivateTransaction)
		pt.PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.dispatch(ctx)
	require.NoError(t, err)
}

func Test_mapPreparedTransaction_PrivateTransaction(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().PreparedPrivateTransaction(&pldapi.TransactionInput{})
	txn, _ := builder.Build(ctx)

	refs := txn.mapPreparedTransaction()
	require.NotNil(t, refs)
	assert.Equal(t, txn.pt.ID, refs.ID)
	assert.Equal(t, txn.pt.Address, *refs.To)
	assert.Equal(t, &txn.pt.Address, refs.Transaction.To)
}

func Test_mapPreparedTransaction_PublicTransaction(t *testing.T) {
	ctx := t.Context()
	builder := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch)
	builder.PrivateTransactionBuilder().PreparedPublicTransaction(&pldapi.TransactionInput{})
	txn, _ := builder.Build(ctx)

	refs := txn.mapPreparedTransaction()
	require.NotNil(t, refs)
	assert.Equal(t, &txn.pt.Address, refs.Transaction.To)
}
