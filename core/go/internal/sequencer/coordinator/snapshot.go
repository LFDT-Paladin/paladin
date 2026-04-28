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

package coordinator

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

func action_SendHeartbeat(ctx context.Context, c *coordinator, _ common.Event) error {
	return c.sendHeartbeat(ctx, c.contractAddress)
}

func (c *coordinator) sendHeartbeat(ctx context.Context, contractAddress *pldtypes.EthAddress) error {
	snapshot := c.getSnapshot(ctx)
	log.L(ctx).Debugf("sending heartbeats for sequencer %s", contractAddress.String())
	var err error
	for _, node := range c.originatorNodePool {
		if node != c.nodeName {
			log.L(ctx).Debugf("sending heartbeat to %s", node)
			err = c.transportWriter.SendHeartbeat(ctx, node, contractAddress, snapshot)
			if err != nil {
				log.L(ctx).Errorf("error sending heartbeat to %s: %v", node, err)
			}
		}
	}
	return err
}

func (c *coordinator) getSnapshot(ctx context.Context) *common.CoordinatorSnapshot {
	log.L(ctx).Debugf("creating snapshot for sequencer %s", c.contractAddress.String())
	// This function is called from the sequencer loop so is safe to read internal state
	pooledTransactions := make([]*common.SnapshotPooledTransaction, 0, len(c.transactionsByID))
	dispatchedTransactions := make([]*common.SnapshotDispatchedTransaction, 0, len(c.transactionsByID))
	confirmedTransactions := make([]*common.SnapshotConfirmedTransaction, 0, len(c.transactionsByID))

	//Snapshot contains a coarse grained view of transactions state.
	// All known transactions fall into one of 3 categories
	// 1. Pooled transactions - these are transactions that have been delegated but not yet dispatched
	// 2. Dispatched transactions - these are transactions that are past the point of no return, the precise status (ready for collection, dispatched, nonce assigned, submitted to a blockchain node) is dependent on parallel processing from this point onward
	// 3. Confirmed transactions - these are transactions that have been confirmed by the network
	for _, txn := range c.transactionsByID {
		pooledTransaction, dispatchedTransaction, confirmedTransaction := txn.GetSnapshot(ctx)
		if pooledTransaction != nil {
			pooledTransactions = append(pooledTransactions, pooledTransaction)
		}
		if dispatchedTransaction != nil {
			dispatchedTransactions = append(dispatchedTransactions, dispatchedTransaction)
		}
		if confirmedTransaction != nil {
			confirmedTransactions = append(confirmedTransactions, confirmedTransaction)
		}
	}

	log.L(ctx).Debugf("created snapshot for sequencer %s with %d transactions (%d pooled transactions, %d dispatched transactions, , %d confirmed transactions)",
		c.contractAddress.String(), len(pooledTransactions)+len(dispatchedTransactions)+len(confirmedTransactions),
		len(pooledTransactions), len(dispatchedTransactions), len(confirmedTransactions))
	return &common.CoordinatorSnapshot{
		DispatchedTransactions: dispatchedTransactions,
		PooledTransactions:     pooledTransactions,
		ConfirmedTransactions:  confirmedTransactions,
		CoordinatorState:       c.stateMachineEventLoop.GetCurrentState().String(),
		BlockHeight:            c.currentBlockHeight,
	}
}

func action_IncrementHeartbeatIntervalsSinceStateChange(ctx context.Context, c *coordinator, event common.Event) error {
	c.heartbeatIntervalsSinceStateChange++
	return nil
}

func action_PropagateHeartbeatIntervalToTransactions(ctx context.Context, c *coordinator, _ common.Event) error {
	return c.propagateEventToAllTransactions(ctx, &common.HeartbeatIntervalEvent{})
}
