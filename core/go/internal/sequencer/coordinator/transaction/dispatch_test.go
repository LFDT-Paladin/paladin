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

func Test_action_Dispatch(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("prepare failed"))

	err := action_Dispatch(ctx, txn, nil)
	require.ErrorContains(t, err, "prepare failed")
}

func Test_buildDispatchBatch_ChainedPrivateBranch(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		Build()

	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		Build()

	prepareErr := errors.New("chained prepare failed")
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, prepareErr)

	batch, err := txn.buildDispatchBatch(ctx)
	require.Error(t, err)
	assert.Nil(t, batch)
	assert.Contains(t, err.Error(), "chained prepare failed")
}

func Test_buildDispatchBatch_PrepareTransactionBranch(t *testing.T) {
	ctx := t.Context()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_PREPARE_TRANSACTION,
			},
		}).
		Build()

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

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_PREPARE_TRANSACTION,
			},
		}).
		Build()

	batch, err := txn.buildDispatchBatch(ctx)
	require.NoError(t, err)
	require.NotNil(t, batch)
	assert.Len(t, batch.PreparedTransactions, 1)
}

func Test_buildDispatchBatch_InvalidOutcome_SendWithNoPrepared(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{},
		}).
		Build()

	batch, err := txn.buildDispatchBatch(ctx)
	require.ErrorContains(t, err, "Prepare outcome unexpected")
	assert.Nil(t, batch)
}

func Test_buildDispatchBatch_InvalidOutcome_SendWithBothPrepared(t *testing.T) {
	ctx := t.Context()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		PreparedPublicTransaction(&pldapi.TransactionInput{}).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		Build()

	batch, err := txn.buildDispatchBatch(ctx)
	require.ErrorContains(t, err, "Prepare outcome unexpected")
	assert.Nil(t, batch)
}

func Test_buildDispatchBatch_PublicBranch(t *testing.T) {
	ctx := t.Context()
	gasVal := pldtypes.HexUint64(21000)
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Signer("signer@node1").
		NodeName("node1").
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		PreparedPublicTransaction(&pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Data:            pldtypes.RawJSON("[]"),
				PublicTxOptions: pldapi.PublicTxOptions{Gas: &gasVal},
			},
			ABI: abi.ABI{&abi.Entry{Type: abi.Function, Name: "test", Inputs: abi.ParameterArray{}}},
		}).
		Build()

	mocks.KeyManager.On("ResolveEthAddressNewDatabaseTX", mock.Anything, "signer").Return(pldtypes.RandAddress(), nil)
	mocks.PublicTxManager.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil)

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
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Signer("signer@node1").
		NodeName("node1").
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		PreparedPublicTransaction(&pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Data:            pldtypes.RawJSON("[]"),
				PublicTxOptions: pldapi.PublicTxOptions{Gas: &gasVal},
			},
			ABI: abi.ABI{&abi.Entry{Type: abi.Function, Name: "test", Inputs: abi.ParameterArray{}}},
		}).
		Build()

	mocks.KeyManager.On("ResolveEthAddressNewDatabaseTX", mock.Anything, "signer").Return(nil, errors.New("resolve signer failed"))

	batch, err := txn.buildDispatchBatch(ctx)
	require.ErrorContains(t, err, "resolve signer failed")
	require.Nil(t, batch)
}

func Test_buildDispatchBatch_PublicBranch_EncodeCallDataError(t *testing.T) {
	ctx := t.Context()
	gasVal := pldtypes.HexUint64(21000)
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Signer("signer@node1").
		NodeName("node1").
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		PreparedPublicTransaction(&pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Data:            pldtypes.RawJSON("[]"),
				PublicTxOptions: pldapi.PublicTxOptions{Gas: &gasVal},
			},
			ABI: abi.ABI{&abi.Entry{
				Type:   abi.Function,
				Name:   "test",
				Inputs: abi.ParameterArray{{Name: "a", Type: "uint256"}},
			}},
		}).
		Build()

	mocks.KeyManager.On("ResolveEthAddressNewDatabaseTX", mock.Anything, "signer").Return(pldtypes.RandAddress(), nil)

	batch, err := txn.buildDispatchBatch(ctx)
	require.Error(t, err)
	require.Nil(t, batch)
}

func Test_buildDispatchBatch_PublicBranch_InvalidSignerIdentity(t *testing.T) {
	ctx := t.Context()
	gasVal := pldtypes.HexUint64(21000)
	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Signer("bad%signer@node1").
		NodeName("node1").
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		PreparedPublicTransaction(&pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Data:            pldtypes.RawJSON("[]"),
				PublicTxOptions: pldapi.PublicTxOptions{Gas: &gasVal},
			},
			ABI: abi.ABI{&abi.Entry{Type: abi.Function, Name: "test", Inputs: abi.ParameterArray{}}},
		}).
		Build()

	batch, err := txn.buildDispatchBatch(ctx)
	require.Error(t, err)
	require.Nil(t, batch)
}

func Test_buildDispatchBatch_PublicBranch_ValidateTransactionError(t *testing.T) {
	ctx := t.Context()
	gasVal := pldtypes.HexUint64(21000)
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Signer("signer@node1").
		NodeName("node1").
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
			},
		}).
		PreparedPublicTransaction(&pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Data:            pldtypes.RawJSON("[]"),
				PublicTxOptions: pldapi.PublicTxOptions{Gas: &gasVal},
			},
			ABI: abi.ABI{&abi.Entry{Type: abi.Function, Name: "test", Inputs: abi.ParameterArray{}}},
		}).
		Build()

	mocks.KeyManager.On("ResolveEthAddressNewDatabaseTX", mock.Anything, "signer").Return(pldtypes.RandAddress(), nil)
	mocks.PublicTxManager.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("validate failed"))

	batch, err := txn.buildDispatchBatch(ctx)
	require.ErrorContains(t, err, "validate failed")
	require.Nil(t, batch)
}

func Test_dispatch_PrepareTransactionReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("prepare failed"))

	err := txn.dispatch(ctx)
	require.ErrorContains(t, err, "prepare failed")
}

func Test_dispatch_BuildDispatchBatchReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{},
		}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.dispatch(ctx)
	require.ErrorContains(t, err, "Prepare outcome unexpected")
}

func Test_dispatch_StateDistributionBuilderReturnsError(t *testing.T) {
	ctx := t.Context()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
				// not setting a PreAssembly.TransactionSpecification.From causes an error in NewStateDistributionBuilder.Build
			},
		}).
		Build()

	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(2).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)

	err := txn.dispatch(ctx)
	require.ErrorContains(t, err, "state distribution")
}

func Test_dispatch_BuildNullifiersReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(2).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, errors.New("build nullifiers failed"))

	err := txn.dispatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build nullifiers failed")
}

func Test_dispatch_UpsertNullifiersReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(2).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return([]*components.NullifierUpsert{{}}, nil)
	mocks.DomainContext.On("UpsertNullifiers", mock.Anything).Return(errors.New("upsert nullifiers failed"))

	err := txn.dispatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert nullifiers failed")
}

func Test_dispatch_Success_WithNullifiers(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(2).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).
		Return([]*components.NullifierUpsert{{ID: pldtypes.HexBytes(pldtypes.RandBytes(32))}}, nil)
	mocks.DomainContext.On("UpsertNullifiers", mock.Anything).Return(nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mocks.SequenceManager.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.dispatch(ctx)
	require.NoError(t, err)
}

func Test_dispatch_PersistDispatchBatchReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(2).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("persist failed"))

	err := txn.dispatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist failed")
}

func Test_dispatch_PersistDispatchBatch_WithRemoteStateDistributions(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		PostAssembly(&components.TransactionPostAssembly{
			OutputStates: []*components.FullState{
				{
					ID:     pldtypes.HexBytes(pldtypes.RandBytes(32)),
					Schema: pldtypes.Bytes32(pldtypes.RandBytes(32)),
					Data:   pldtypes.JSONString("{\"data\":\"hello\"}"),
				},
			},
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"receiver@node2"},
				},
			},
		}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(2).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			sd := args.Get(3).([]*components.StateDistribution)
			require.Len(t, sd, 1)
			assert.Equal(t, "receiver@node2", sd[0].IdentityLocator)
		}).
		Return(nil)
	mocks.SequenceManager.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.dispatch(ctx)
	require.NoError(t, err)
}

func Test_dispatch_HandleNewTransactionsReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(2).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mocks.SequenceManager.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("handle new transactions failed"))

	err := txn.dispatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handle new transactions failed")
}

func Test_dispatch_Success_PrepareTransactionBranch_DoesNotHandleNewTransactions(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_PREPARE_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		Build()

	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		pt := args.Get(2).(*components.PrivateTransaction)
		pt.PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.dispatch(ctx)
	require.NoError(t, err)
	mocks.SequenceManager.AssertNotCalled(t, "HandleNewTransactions", mock.Anything, mock.Anything)
}

func Test_dispatch_Success_ChainedPrivate(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_SEND_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		Build()
	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(2).(*components.PrivateTransaction).PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.TXManager.On("PrepareChainedPrivateTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&components.ChainedPrivateTransaction{NewTransaction: &components.ValidatedTransaction{}}, nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mocks.SequenceManager.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.dispatch(ctx)
	require.NoError(t, err)
}

func Test_dispatch_Success_PrepareTransactionBranch(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				Intent: prototk.TransactionSpecification_PREPARE_TRANSACTION,
				From:   "sender@node1",
			},
		}).
		Build()

	mocks.DomainAPI.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		pt := args.Get(2).(*components.PrivateTransaction)
		pt.PreparedPrivateTransaction = &pldapi.TransactionInput{}
	}).Return(nil)
	mocks.SequenceManager.On("BuildNullifiers", mock.Anything, mock.Anything).Return(nil, nil)
	mocks.SyncPoints.On("PersistDispatchBatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.dispatch(ctx)
	require.NoError(t, err)
}

func Test_mapPreparedTransaction_PrivateTransaction(t *testing.T) {
	addr := pldtypes.RandAddress()
	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Address(*addr).
		PreparedPrivateTransaction(&pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				To: addr,
			},
		}).
		Build()

	refs := txn.mapPreparedTransaction()
	require.NotNil(t, refs)
	assert.Equal(t, txn.pt.ID, refs.ID)
	assert.Equal(t, txn.pt.Address, *refs.To)
	assert.Equal(t, txn.pt.Address, *refs.Transaction.To)
}

func Test_mapPreparedTransaction_PublicTransaction(t *testing.T) {
	addr := pldtypes.RandAddress()
	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Address(*addr).
		PreparedPublicTransaction(&pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				To: addr,
			},
		}).
		Build()

	refs := txn.mapPreparedTransaction()
	require.NotNil(t, refs)
	assert.Equal(t, &txn.pt.Address, refs.Transaction.To)
}

func Test_mapPreparedTransaction_StateRefs(t *testing.T) {
	addr := pldtypes.RandAddress()
	inputID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	readID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	outputID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	infoID := pldtypes.HexBytes(pldtypes.RandBytes(32))

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Address(*addr).
		PostAssembly(&components.TransactionPostAssembly{
			InputStates:  []*components.FullState{{ID: inputID}},
			ReadStates:   []*components.FullState{{ID: readID}},
			OutputStates: []*components.FullState{{ID: outputID}},
			InfoStates:   []*components.FullState{{ID: infoID}},
		}).
		PreparedPrivateTransaction(&pldapi.TransactionInput{}).
		Build()

	refs := txn.mapPreparedTransaction()
	require.NotNil(t, refs)
	assert.Equal(t, []pldtypes.HexBytes{inputID}, refs.StateRefs.Spent)
	assert.Equal(t, []pldtypes.HexBytes{readID}, refs.StateRefs.Read)
	assert.Equal(t, []pldtypes.HexBytes{outputID}, refs.StateRefs.Confirmed)
	assert.Equal(t, []pldtypes.HexBytes{infoID}, refs.StateRefs.Info)
}
