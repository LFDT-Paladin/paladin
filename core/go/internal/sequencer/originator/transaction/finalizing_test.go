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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinalizeEvent_Type_ReturnsEvent_Finalize(t *testing.T) {
	e := &FinalizeEvent{TransactionID: uuid.New()}
	assert.Equal(t, Event_Finalize, e.Type())
}

func TestFinalizeEvent_TypeString_ReturnsExpected(t *testing.T) {
	e := &FinalizeEvent{TransactionID: uuid.New()}
	assert.Equal(t, "Event_Finalize", e.TypeString())
}

func TestFinalizeEvent_GetTransactionID_ReturnsID(t *testing.T) {
	id := uuid.New()
	e := &FinalizeEvent{TransactionID: id}
	assert.Equal(t, id, e.GetTransactionID())
}

func Test_action_NonceAssigned_SetsSignerAndNonce(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()
	addr := *pldtypes.RandAddress()
	nonce := uint64(42)
	event := &NonceAssignedEvent{
		BaseEvent:     BaseEvent{TransactionID: txn.pt.ID},
		SignerAddress: addr,
		Nonce:         nonce,
	}
	err := action_NonceAssigned(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, &addr, txn.signerAddress)
	assert.Equal(t, &nonce, txn.nonce)
}

func Test_action_Submitted_SetsSignerNonceAndHash(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()
	addr := *pldtypes.RandAddress()
	nonce := uint64(42)
	hash := pldtypes.RandBytes32()
	event := &SubmittedEvent{
		BaseEvent:             BaseEvent{TransactionID: txn.pt.ID},
		SignerAddress:         addr,
		Nonce:                 nonce,
		LatestSubmissionHash:  hash,
	}
	err := action_Submitted(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, &addr, txn.signerAddress)
	assert.Equal(t, &nonce, txn.nonce)
	assert.Equal(t, &hash, txn.latestSubmissionHash)
}

func Test_action_QueueFinalizeEvent_QueuesFinalizeEvent(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Confirmed)
	txn, mocks := builder.BuildWithMocks()
	err := action_QueueFinalizeEvent(ctx, txn, nil)
	require.NoError(t, err)
	events := mocks.GetEmittedEvents()
	require.Len(t, events, 1)
	finalizeEv, ok := events[0].(*FinalizeEvent)
	require.True(t, ok)
	assert.Equal(t, txn.pt.ID, finalizeEv.TransactionID)
}
