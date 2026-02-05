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
 * specific language governing permissions and limitations under this License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/require"
)

// TestDispatchLoop_StopWhileWaitingForInFlightSlot covers the path where the dispatch loop
// is blocked in the first Wait() (too many in flight) and exits when Stop() sends to
// stopDispatchLoop and Signals. We pre-populate inFlightTxns so that when the loop
// pulls the queued tx it sees len(inFlightTxns)+dispatchedAhead >= maxDispatchAhead and enters Wait().
func TestDispatchLoop_StopWhileWaitingForInFlightSlot(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(1)
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	c.Start()
	defer c.Stop()

	// Pre-populate inFlightTxns so the dispatch loop will enter the first Wait() when it pulls the tx
	dummyTxn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build(ctx)
	c.inFlightMutex.L.Lock()
	c.inFlightTxns[dummyTxn.GetID()] = dummyTxn
	c.inFlightMutex.L.Unlock()

	// Queue one tx: transition to Ready_For_Dispatch so it gets sent to dispatchQueue.
	// buildDispatchBatchAndPreparedTransactionDistributions requires PreAssembly.TransactionSpecification.
	txn, _ := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		PreAssembly(&components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{Intent: prototk.TransactionSpecification_SEND_TRANSACTION},
		}).Build(ctx)
	c.transactionsByID[txn.GetID()] = txn
	err := action_TransactionStateTransition(ctx, c, &common.TransactionStateTransitionEvent[transaction.State]{
		TransactionID: txn.GetID(),
		To:            transaction.State_Ready_For_Dispatch,
	})
	require.NoError(t, err)

	// Give the dispatch loop time to pull the tx and enter the first Wait() (too many in flight).
	time.Sleep(50 * time.Millisecond)
	// Stop() sends to stopDispatchLoop (buffered) then Signals; loop wakes, receives from stopDispatchLoop, returns.
	c.Stop()
}

// TestDispatchLoop_StopAtSelect covers the path where the dispatch loop is in the top-level
// select and receives from stopDispatchLoop (loop not in Wait()).
func TestDispatchLoop_StopAtSelect(t *testing.T) {
	ctx := context.Background()
	builder := NewCoordinatorBuilderForTesting(t, State_Idle)
	config := builder.GetSequencerConfig()
	config.MaxDispatchAhead = confutil.P(-1) // no dispatch progress, so loop stays in select
	builder.OverrideSequencerConfig(config)
	c, _ := builder.Build(ctx)
	c.Start()
	// Stop without ever queueing a tx; loop is blocked on select between dispatchQueue and stopDispatchLoop
	c.Stop()
}
