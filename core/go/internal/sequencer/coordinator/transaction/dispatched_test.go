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

func Test_action_Collected_SetsSignerAddress(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	signerAddr := pldtypes.RandAddress()
	event := &CollectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		SignerAddress: *signerAddr,
	}

	err := action_Collected(ctx, txn, event)
	require.NoError(t, err)

	// Assert state: signerAddress was set from the event
	require.NotNil(t, txn.signerAddress)
	assert.Equal(t, signerAddr.String(), txn.signerAddress.String())
}

func Test_action_NonceAllocated_SetsNonceAndSends(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	nonce := uint64(123)
	event := &NonceAllocatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce: nonce,
	}

	mocks.transportWriter.EXPECT().
		SendNonceAssigned(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, nonce).
		Return(nil)

	err := action_NonceAllocated(ctx, txn, event)
	require.NoError(t, err)

	// Assert state: nonce was set
	require.NotNil(t, txn.nonce)
	assert.Equal(t, nonce, *txn.nonce)
	mocks.transportWriter.AssertExpectations(t)
}

func Test_action_NonceAllocated_PropagatesSendError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	event := &NonceAllocatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce: 1,
	}

	mocks.transportWriter.EXPECT().
		SendNonceAssigned(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, uint64(1)).
		Return(assert.AnError)

	err := action_NonceAllocated(ctx, txn, event)
	assert.Error(t, err)

	// State still updated even when send fails
	require.NotNil(t, txn.nonce)
	assert.Equal(t, uint64(1), *txn.nonce)
}

func Test_action_Submitted_SetsSubmissionHashAndSends(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	event := &SubmittedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		SubmissionHash: submissionHash,
	}

	mocks.transportWriter.EXPECT().
		SendTransactionSubmitted(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, &submissionHash).
		Return(nil)

	err := action_Submitted(ctx, txn, event)
	require.NoError(t, err)

	// Assert state: latestSubmissionHash was set
	require.NotNil(t, txn.latestSubmissionHash)
	assert.Equal(t, submissionHash, *txn.latestSubmissionHash)
	mocks.transportWriter.AssertExpectations(t)
}

func Test_action_Submitted_PropagatesSendError(t *testing.T) {
	ctx := context.Background()
	txn, mocks := newTransactionForUnitTesting(t, nil)

	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	event := &SubmittedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		SubmissionHash: submissionHash,
	}

	mocks.transportWriter.EXPECT().
		SendTransactionSubmitted(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, &submissionHash).
		Return(assert.AnError)

	err := action_Submitted(ctx, txn, event)
	assert.Error(t, err)

	// State still updated
	require.NotNil(t, txn.latestSubmissionHash)
	assert.Equal(t, submissionHash, *txn.latestSubmissionHash)
}
