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
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSnapshot_PooledStates_StateBlocked(t *testing.T) {
	ctx := t.Context()
	originator := "sender@node1"
	txn, _ := NewTransactionBuilderForTesting(t, State_Blocked).
		Originator(originator).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	require.NotNil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
	assert.Equal(t, txn.pt.ID, pooledSnapshot.ID)
	assert.Equal(t, "", pooledSnapshot.Originator)
}

func TestGetSnapshot_PooledStates_StateConfirmingDispatchable(t *testing.T) {
	ctx := t.Context()
	originator := "sender@node1"
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		Originator(originator).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	require.NotNil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
	assert.Equal(t, txn.pt.ID, pooledSnapshot.ID)
	assert.Equal(t, "", pooledSnapshot.Originator)
}

func TestGetSnapshot_PooledStates_StateEndorsementGathering(t *testing.T) {
	ctx := t.Context()
	originator := "sender@node1"
	txn, _ := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Originator(originator).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	require.NotNil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
	assert.Equal(t, txn.pt.ID, pooledSnapshot.ID)
	assert.Equal(t, "", pooledSnapshot.Originator)
}

func TestGetSnapshot_PooledStates_StatePreAssemblyBlocked(t *testing.T) {
	ctx := t.Context()
	originator := "sender@node1"
	txn, _ := NewTransactionBuilderForTesting(t, State_PreAssembly_Blocked).
		Originator(originator).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	require.NotNil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
	assert.Equal(t, txn.pt.ID, pooledSnapshot.ID)
	assert.Equal(t, "", pooledSnapshot.Originator)
}

func TestGetSnapshot_PooledStates_StateAssembling(t *testing.T) {
	ctx := t.Context()
	originator := "sender@node1"
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		Originator(originator).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	require.NotNil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
	assert.Equal(t, txn.pt.ID, pooledSnapshot.ID)
	assert.Equal(t, "", pooledSnapshot.Originator)
}

func TestGetSnapshot_PooledStates_StatePooled(t *testing.T) {
	ctx := t.Context()
	originator := "sender@node1"
	txn, _ := NewTransactionBuilderForTesting(t, State_Pooled).
		Originator(originator).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	require.NotNil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
	assert.Equal(t, txn.pt.ID, pooledSnapshot.ID)
	assert.Equal(t, "", pooledSnapshot.Originator)
}

func TestGetSnapshot_DispatchedStates_WithSigner_StateReadyForDispatch(t *testing.T) {
	ctx := t.Context()
	originator := "sender@node1"
	nonce := uint64(42)
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	signer := pldtypes.RandAddress()

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		Originator(originator).
		SignerAddress(signer).
		Nonce(&nonce).
		LatestSubmissionHash(&submissionHash).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	assert.Nil(t, pooledSnapshot)
	require.NotNil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
	assert.Equal(t, txn.pt.ID, dispatchedSnapshot.ID)
	assert.Equal(t, originator, dispatchedSnapshot.Originator)
	assert.Equal(t, *signer, dispatchedSnapshot.Signer)
	assert.Equal(t, &nonce, dispatchedSnapshot.Nonce)
	assert.Equal(t, &submissionHash, dispatchedSnapshot.LatestSubmissionHash)
}

func TestGetSnapshot_DispatchedStates_WithSigner_StateDispatched(t *testing.T) {
	ctx := t.Context()
	originator := "sender@node1"
	nonce := uint64(42)
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	signer := pldtypes.RandAddress()

	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).
		Originator(originator).
		SignerAddress(signer).
		Nonce(&nonce).
		LatestSubmissionHash(&submissionHash).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	assert.Nil(t, pooledSnapshot)
	require.NotNil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
	assert.Equal(t, txn.pt.ID, dispatchedSnapshot.ID)
	assert.Equal(t, originator, dispatchedSnapshot.Originator)
	assert.Equal(t, *signer, dispatchedSnapshot.Signer)
	assert.Equal(t, &nonce, dispatchedSnapshot.Nonce)
	assert.Equal(t, &submissionHash, dispatchedSnapshot.LatestSubmissionHash)
}

func TestGetSnapshot_DispatchedState_WithoutSigner(t *testing.T) {
	ctx := t.Context()
	nonce := uint64(99)
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))

	txn, _ := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		SignerAddress(nil).
		Nonce(&nonce).
		LatestSubmissionHash(&submissionHash).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	assert.Nil(t, pooledSnapshot)
	require.NotNil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
	assert.Nil(t, dispatchedSnapshot.Nonce)
	assert.Nil(t, dispatchedSnapshot.LatestSubmissionHash)
}

func TestGetSnapshot_Confirmed_WithSigner(t *testing.T) {
	ctx := t.Context()
	originator := "sender@node1"
	nonce := uint64(11)
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	signer := pldtypes.RandAddress()
	revertReason := pldtypes.MustParseHexBytes("0x1234")

	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		Originator(originator).
		SignerAddress(signer).
		Nonce(&nonce).
		LatestSubmissionHash(&submissionHash).
		RevertReason(revertReason).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	assert.Nil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	require.NotNil(t, confirmedSnapshot)
	assert.Equal(t, txn.pt.ID, confirmedSnapshot.ID)
	assert.Equal(t, *signer, confirmedSnapshot.Signer)
	assert.Equal(t, &nonce, confirmedSnapshot.Nonce)
	assert.Equal(t, &submissionHash, confirmedSnapshot.LatestSubmissionHash)
	assert.Equal(t, revertReason, confirmedSnapshot.RevertReason)
}

func TestGetSnapshot_Confirmed_WithoutSigner(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		SignerAddress(nil).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	assert.Nil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	require.NotNil(t, confirmedSnapshot)
	assert.Equal(t, pldtypes.EthAddress{}, confirmedSnapshot.Signer)
}

func TestGetSnapshot_ExcludedStates_StateInitial(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()
	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	assert.Nil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
}

func TestGetSnapshot_ExcludedStates_StateReverted(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Reverted).Build()
	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	assert.Nil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
}

func TestGetSnapshot_ExcludedStates_StateFinal(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Final).Build()
	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	assert.Nil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	assert.Nil(t, confirmedSnapshot)
}
