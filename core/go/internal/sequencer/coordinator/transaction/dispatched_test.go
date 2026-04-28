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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
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
		SendNonceAssigned(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, nonce).
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
		SendNonceAssigned(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, uint64(1)).
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
		SendTransactionSubmitted(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, &submissionHash).
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
		SendTransactionSubmitted(ctx, txn.pt.ID, txn.originatorNode, &txn.pt.Address, &submissionHash).
		Return(assert.AnError)

	err := action_NotifySubmitted(ctx, txn, event)
	require.Error(t, err)

	// State still updated
	require.NotNil(t, txn.latestSubmissionHash)
	assert.Equal(t, submissionHash, *txn.latestSubmissionHash)
}

func Test_action_ReleaseAssemblyPayload_NilsHeavyFields(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		InputStates:  []*components.FullState{{Data: pldtypes.RawJSON(`{}`)}},
		OutputStates: []*components.FullState{{Data: pldtypes.RawJSON(`{}`)}},
		Endorsements: []*prototk.AttestationResult{{Payload: []byte("sig")}},
	}
	txn.pt.PreparedPublicTransaction = &pldapi.TransactionInput{}
	txn.pt.PreparedMetadata = pldtypes.RawJSON(`{"meta":true}`)
	txn.pt.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}

	savedID := txn.pt.ID
	savedDomain := txn.pt.Domain
	savedAddress := txn.pt.Address

	err := action_CleanUpAssemblyPayload(ctx, txn, nil)
	require.NoError(t, err)

	assert.Nil(t, txn.pt.PostAssembly)
	assert.NotNil(t, txn.pt.PreAssembly, "PreAssembly preserved for retryable reverts")
	assert.Nil(t, txn.pt.PreparedPublicTransaction)
	assert.Nil(t, txn.pt.PreparedPrivateTransaction)
	assert.Nil(t, txn.pt.PreparedMetadata)

	assert.Equal(t, savedID, txn.pt.ID)
	assert.Equal(t, savedDomain, txn.pt.Domain)
	assert.Equal(t, savedAddress, txn.pt.Address)
}

func Test_action_ReleaseAssemblyPayload_SafeWithNilFields(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	txn.pt.PostAssembly = nil
	txn.pt.PreAssembly = nil
	txn.pt.PreparedPublicTransaction = nil
	txn.pt.PreparedPrivateTransaction = nil
	txn.pt.PreparedMetadata = nil

	err := action_CleanUpAssemblyPayload(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_action_NotifyDispatched_UsesTransactionSpec(t *testing.T) {
	ctx := t.Context()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		UseMockTransportWriter().
		Build()

	spec := txn.pt.PreAssembly.TransactionSpecification
	mocks.TransportWriter.EXPECT().
		SendDispatched(ctx, txn.originator, mock.Anything, spec).
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
		SendDispatched(ctx, txn.originator, mock.Anything, (*prototk.TransactionSpecification)(nil)).
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
		SendDispatched(ctx, txn.originator, mock.Anything, txn.pt.PreAssembly.TransactionSpecification).
		Return(assert.AnError)

	err := action_NotifyDispatched(ctx, txn, nil)
	require.Error(t, err)
}
