/*
 * Copyright © 2025 Kaleido, Inc.
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

package originator

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_State_String_Idle(t *testing.T) {
	assert.Equal(t, "Idle", State_Idle.String())
}

func Test_State_String_Observing(t *testing.T) {
	assert.Equal(t, "Observing", State_Observing.String())
}

func Test_State_String_Sending(t *testing.T) {
	assert.Equal(t, "Sending", State_Sending.String())
}

func Test_State_String_Unknown(t *testing.T) {
	var s State = 999
	assert.Equal(t, "Unknown", s.String())
}

func Test_GetTxStatus_KnownTransactionReturnsStatus(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pending).Build()
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode").Transactions(txn)
	o, _ := builder.Build(ctx)
	defer o.Stop()

	status, err := o.GetTxStatus(ctx, txn.GetID())
	require.NoError(t, err)
	assert.Equal(t, txn.GetID().String(), status.TxID)
	assert.NotEmpty(t, status.Status)
}

func Test_GetTxStatus_UnknownTransactionReturnsUnknown(t *testing.T) {
	ctx := context.Background()
	builder := NewOriginatorBuilderForTesting(State_Observing).CommitteeMembers("sender@senderNode", "coordinator@coordinatorNode")
	o, _ := builder.Build(ctx)
	defer o.Stop()

	unknownID := uuid.New()
	status, err := o.GetTxStatus(ctx, unknownID)
	require.NoError(t, err)
	assert.Equal(t, unknownID.String(), status.TxID)
	assert.Equal(t, "unknown", status.Status)
}
