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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

func action_SendHeartbeat(ctx context.Context, c *coordinator, _ common.Event) error {
	return c.sendHeartbeat(ctx, false)
}

func action_SendHeartbeatWithLocks(ctx context.Context, c *coordinator, _ common.Event) error {
	return c.sendHeartbeat(ctx, true)
}

// sendHeartbeat builds the base snapshot once, then sends a per-node copy to each heartbeat
// recipient. In ENDORSER mode the recipients are the endorser candidates; in STATIC/SENDER modes
// the originator activity map is updated first and its surviving keys are used.
// In Closing_Flush/Closing states, the grapher is queried per-node:
// each node receives all locks (unfiltered) plus only the OutputStates it is permitted to hold
// (filtered by AllowedNodes).
func (c *coordinator) sendHeartbeat(ctx context.Context, includeLocks bool) error {
	baseSnapshot := c.getSnapshot(ctx)

	var nodes []string
	if c.coordinatorSelection == prototk.ContractConfig_COORDINATOR_ENDORSER {
		nodes = c.endorserCandidates
	} else {
		nodes = make([]string, 0, len(c.originatorActivity))
		for node := range c.originatorActivity {
			nodes = append(nodes, node)
		}
	}

	log.L(ctx).Debugf("sending heartbeats for sequencer %s to %d nodes (includeLocks=%v)", c.contractAddress.String(), len(nodes), includeLocks)
	var err error
	for _, node := range nodes {
		log.L(ctx).Debugf("sending heartbeat to %s", node)
		snapshot := baseSnapshot
		if includeLocks {
			statesAndLocks, exportErr := c.grapher.ExportStatesAndLocks(ctx, node)
			if exportErr != nil {
				log.L(ctx).Errorf("error exporting states and locks for node %s: %v", node, exportErr)
				err = exportErr
				continue
			}
			snapshot = &common.CoordinatorSnapshot{
				DispatchedTransactions: baseSnapshot.DispatchedTransactions,
				PooledTransactions:     baseSnapshot.PooledTransactions,
				ConfirmedTransactions:  baseSnapshot.ConfirmedTransactions,
				RevertedTransactions:   baseSnapshot.RevertedTransactions,
				CoordinatorState:       baseSnapshot.CoordinatorState,
				BlockHeight:            baseSnapshot.BlockHeight,
				Locks:                  statesAndLocks.LockedState,
				OutputStates:           statesAndLocks.OutputState,
			}
		}
		if sendErr := c.transportWriter.SendHeartbeat(ctx, node, c.contractAddress, snapshot); sendErr != nil {
			log.L(ctx).Errorf("error sending heartbeat to %s: %v", node, sendErr)
			err = sendErr
		}
	}
	return err
}

// updateOriginatorActivity refreshes the originator activity map for STATIC/SENDER modes.
// For each tracked node: if there is a transaction currently in memory for that node the
// counter is reset to 0; otherwise it is incremented. Nodes whose counter reaches the
// inactive grace period are pruned and will no longer receive heartbeats.
func (c *coordinator) updateOriginatorActivity(ctx context.Context) {
	activeNodes := make(map[string]bool, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		activeNodes[txn.GetOriginatorNode()] = true
	}

	for node := range c.originatorActivity {
		if activeNodes[node] {
			c.originatorActivity[node] = 0
		} else {
			c.originatorActivity[node]++
		}
	}

	for node, count := range c.originatorActivity {
		// measure number of complete heartbeat interval periods - e.g. count of 2 means
		// 1 full heartbeat interval has elapsed, hence use of > not >=
		if count > c.inactiveGracePeriod {
			log.L(ctx).Debugf("pruning originator %s from activity map after %d heartbeat intervals of inactivity", node, count)
			delete(c.originatorActivity, node)
		}
	}
}

// getSnapshot builds the coordinator snapshot (without per-node lock data).
// Locks are attached per-node in sendHeartbeat for Closing_Flush/Closing heartbeats.
func (c *coordinator) getSnapshot(ctx context.Context) *common.CoordinatorSnapshot {
	log.L(ctx).Debugf("creating snapshot for sequencer %s", c.contractAddress.String())
	pooledTransactions := make([]*common.SnapshotPooledTransaction, 0, len(c.transactionsByID))
	dispatchedTransactions := make([]*common.SnapshotDispatchedTransaction, 0, len(c.transactionsByID))
	confirmedTransactions := make([]*common.SnapshotConfirmedTransaction, 0, len(c.transactionsByID))
	revertedTransactions := make([]*common.SnapshotRevertedTransaction, 0, len(c.transactionsByID))

	for _, txn := range c.transactionsByID {
		pooledTransaction, dispatchedTransaction, confirmedTransaction, revertedTransaction := txn.GetSnapshot(ctx)
		if pooledTransaction != nil {
			pooledTransactions = append(pooledTransactions, pooledTransaction)
		}
		if dispatchedTransaction != nil {
			dispatchedTransactions = append(dispatchedTransactions, dispatchedTransaction)
		}
		if confirmedTransaction != nil {
			confirmedTransactions = append(confirmedTransactions, confirmedTransaction)
		}
		if revertedTransaction != nil {
			revertedTransactions = append(revertedTransactions, revertedTransaction)
		}
	}

	coordinatorState := c.stateMachineEventLoop.GetCurrentState()
	log.L(ctx).Debugf("created snapshot for sequencer %s with %d transactions (%d pooled, %d dispatched, %d confirmed, %d reverted)",
		c.contractAddress.String(), len(pooledTransactions)+len(dispatchedTransactions)+len(confirmedTransactions)+len(revertedTransactions),
		len(pooledTransactions), len(dispatchedTransactions), len(confirmedTransactions), len(revertedTransactions))

	return &common.CoordinatorSnapshot{
		DispatchedTransactions: dispatchedTransactions,
		PooledTransactions:     pooledTransactions,
		ConfirmedTransactions:  confirmedTransactions,
		RevertedTransactions:   revertedTransactions,
		CoordinatorState:       coordinatorState,
		BlockHeight:            uint64(c.currentBlockHeight),
		EndorserCandidates:     c.endorserCandidates,
	}
}

// action_UpdateOriginatorActivity advances the originator activity map for STATIC/SENDER modes.
func action_UpdateOriginatorActivity(ctx context.Context, c *coordinator, _ common.Event) error {
	if c.coordinatorSelection == prototk.ContractConfig_COORDINATOR_ENDORSER {
		return nil
	}
	c.updateOriginatorActivity(ctx)
	return nil
}

func action_IncrementHeartbeatIntervalsSinceStateChange(ctx context.Context, c *coordinator, event common.Event) error {
	c.heartbeatIntervalsSinceStateChange++
	return nil
}

func action_PropagateHeartbeatIntervalToTransactions(ctx context.Context, c *coordinator, _ common.Event) error {
	return c.propagateEventToAllTransactions(ctx, &common.HeartbeatIntervalEvent{})
}
