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
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSnapshot_PooledStates(t *testing.T) {
	ctx := context.Background()
	originator := "sender@node1"
	pooledStates := []State{
		State_Blocked,
		State_Confirming_Dispatchable,
		State_Endorsement_Gathering,
		State_PreAssembly_Blocked,
		State_Assembling,
		State_Pooled,
	}

	for _, state := range pooledStates {
		t.Run(state.String(), func(t *testing.T) {
			txn, _ := NewTransactionBuilderForTesting(t, state).
				Originator(originator).
				Build()

			pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
			require.NotNil(t, pooledSnapshot)
			assert.Nil(t, dispatchedSnapshot)
			assert.Nil(t, confirmedSnapshot)
			assert.Equal(t, txn.pt.ID, pooledSnapshot.ID)
			assert.Equal(t, "", pooledSnapshot.Originator)
		})
	}
}

func TestGetSnapshot_DispatchedStates_WithSigner(t *testing.T) {
	ctx := context.Background()
	originator := "sender@node1"
	nonce := uint64(42)
	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	signer := pldtypes.RandAddress()

	dispatchedStates := []State{
		State_Ready_For_Dispatch,
		State_Dispatched,
	}
	for _, state := range dispatchedStates {
		t.Run(state.String(), func(t *testing.T) {
			txn, _ := NewTransactionBuilderForTesting(t, state).
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
		})
	}
}

func TestGetSnapshot_DispatchedState_WithoutSigner(t *testing.T) {
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirmed).
		SignerAddress(nil).
		Build()

	pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
	assert.Nil(t, pooledSnapshot)
	assert.Nil(t, dispatchedSnapshot)
	require.NotNil(t, confirmedSnapshot)
	assert.Equal(t, pldtypes.EthAddress{}, confirmedSnapshot.Signer)
}

func TestGetSnapshot_ExcludedStates(t *testing.T) {
	ctx := context.Background()
	excludedStates := []State{
		State_Initial,
		State_Reverted,
		State_Final,
	}

	for _, state := range excludedStates {
		t.Run(state.String(), func(t *testing.T) {
			txn, _ := NewTransactionBuilderForTesting(t, state).Build()
			pooledSnapshot, dispatchedSnapshot, confirmedSnapshot := txn.GetSnapshot(ctx)
			assert.Nil(t, pooledSnapshot)
			assert.Nil(t, dispatchedSnapshot)
			assert.Nil(t, confirmedSnapshot)
		})
	}
}
