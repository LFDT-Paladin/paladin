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

	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_NotifyCollected_SetsSignerAddress(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	signerAddr := pldtypes.RandAddress()
	event := &CollectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		SignerAddress: *signerAddr,
	}

	err := action_NotifyCollected(ctx, txn, event)
	require.NoError(t, err)

	// Assert state: signerAddress was set from the event
	require.NotNil(t, txn.signerAddress)
	assert.Equal(t, signerAddr.String(), txn.signerAddress.String())
}

func Test_action_NotifyNonceAllocated_SetsNonceAndSends(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		UseMockTransportWriter().
		Build()

	nonce := uint64(123)
	event := &NonceAllocatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce: nonce,
	}

	mocks.TransportWriter.EXPECT().
		SendNonceAssigned(mock.Anything, txn.originatorNode, mock.MatchedBy(func(msg *engineProto.NonceAssigned) bool {
			return msg.TransactionId == txn.pt.ID.String() && msg.Nonce == int64(nonce)
		})).
		Return(nil)

	err := action_NotifyNonceAllocated(ctx, txn, event)
	require.NoError(t, err)

	// Assert state: nonce was set
	require.NotNil(t, txn.nonce)
	assert.Equal(t, nonce, *txn.nonce)
}

func Test_action_NotifyNonceAllocated_PropagatesSendError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		UseMockTransportWriter().
		Build()

	event := &NonceAllocatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		Nonce: 1,
	}

	mocks.TransportWriter.EXPECT().
		SendNonceAssigned(mock.Anything, txn.originatorNode, mock.MatchedBy(func(msg *engineProto.NonceAssigned) bool {
			return msg.TransactionId == txn.pt.ID.String() && msg.Nonce == int64(1)
		})).
		Return(assert.AnError)

	err := action_NotifyNonceAllocated(ctx, txn, event)
	require.Error(t, err)

	// State still updated even when send fails
	require.NotNil(t, txn.nonce)
	assert.Equal(t, uint64(1), *txn.nonce)
}

func Test_action_NotifySubmitted_SetsSubmissionHashAndSends(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		UseMockTransportWriter().
		Build()

	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	event := &SubmittedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		SubmissionHash: submissionHash,
	}

	mocks.TransportWriter.EXPECT().
		SendTransactionSubmitted(mock.Anything, txn.originatorNode, mock.MatchedBy(func(msg *engineProto.TransactionSubmitted) bool {
			return msg.TransactionId == txn.pt.ID.String()
		})).
		Return(nil)

	err := action_NotifySubmitted(ctx, txn, event)
	require.NoError(t, err)

	// Assert state: latestSubmissionHash was set
	require.NotNil(t, txn.latestSubmissionHash)
	assert.Equal(t, submissionHash, *txn.latestSubmissionHash)
}

func Test_action_NotifySubmitted_PropagatesSendError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		UseMockTransportWriter().
		Build()

	submissionHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	event := &SubmittedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
		SubmissionHash: submissionHash,
	}

	mocks.TransportWriter.EXPECT().
		SendTransactionSubmitted(mock.Anything, txn.originatorNode, mock.MatchedBy(func(msg *engineProto.TransactionSubmitted) bool {
			return msg.TransactionId == txn.pt.ID.String()
		})).
		Return(assert.AnError)

	err := action_NotifySubmitted(ctx, txn, event)
	require.Error(t, err)

	// State still updated
	require.NotNil(t, txn.latestSubmissionHash)
	assert.Equal(t, submissionHash, *txn.latestSubmissionHash)
}

func Test_action_NotifyDispatched_UsesTransactionSpec(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		UseMockTransportWriter().
		Build()

	mocks.TransportWriter.EXPECT().
		SendDispatched(mock.Anything, txn.originatorNode, mock.MatchedBy(func(msg *engineProto.TransactionDispatched) bool {
			return msg.TransactionId == txn.pt.ID.String() && msg.Signer == txn.originator
		})).
		Return(nil)

	err := action_NotifyDispatched(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NotifyDispatched_AllowsNilTransactionSpec(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		UseMockTransportWriter().
		Build()
	txn.pt.PreAssembly = nil

	mocks.TransportWriter.EXPECT().
		SendDispatched(mock.Anything, txn.originatorNode, mock.MatchedBy(func(msg *engineProto.TransactionDispatched) bool {
			return msg.TransactionId == txn.pt.ID.String()
		})).
		Return(nil)

	err := action_NotifyDispatched(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NotifyDispatched_PropagatesSendError(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		UseMockTransportWriter().
		Build()

	mocks.TransportWriter.EXPECT().
		SendDispatched(mock.Anything, txn.originatorNode, mock.MatchedBy(func(msg *engineProto.TransactionDispatched) bool {
			return msg.TransactionId == txn.pt.ID.String()
		})).
		Return(assert.AnError)

	err := action_NotifyDispatched(ctx, txn, nil)
	require.Error(t, err)
}
