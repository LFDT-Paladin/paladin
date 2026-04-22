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

package coordinator

import (
	"context"
	"hash/fnv"
	"slices"
	"strconv"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

func action_SelectActiveCoordinator(ctx context.Context, c *coordinator, _ common.Event) error {
	selectedCoordinator, err := c.selectActiveCoordinatorNode(ctx)
	if err != nil {
		log.L(ctx).Errorf("error selecting active coordinator: %v", err)
		return nil
	}
	if selectedCoordinator == "" {
		return nil
	}
	if c.activeCoordinatorNode != selectedCoordinator {
		c.activeCoordinatorNode = selectedCoordinator
		c.coordinatorActive(c.contractAddress, selectedCoordinator)
	}
	return nil
}

func (c *coordinator) selectActiveCoordinatorNode(ctx context.Context) (string, error) {
	coordinatorNode := ""
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_STATIC {
		// E.g. Noto
		if c.domainAPI.ContractConfig().GetStaticCoordinator() == "" {
			return "", i18n.NewError(ctx, "static coordinator mode is configured but static coordinator node is not set")
		}
		log.L(ctx).Debugf("coordinator %s selected as next active coordinator in static coordinator mode", c.domainAPI.ContractConfig().GetStaticCoordinator())
		// If the static coordinator returns a fully qualified identity extract just the node name

		coordinator, err := pldtypes.PrivateIdentityLocator(c.domainAPI.ContractConfig().GetStaticCoordinator()).Node(ctx, false)
		if err != nil {
			log.L(ctx).Errorf("error getting static coordinator node id for %s: %s", c.domainAPI.ContractConfig().GetStaticCoordinator(), err)
			return "", err
		}
		coordinatorNode = coordinator
	} else if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_ENDORSER {
		// E.g. Pente
		// Make a fair choice about the next coordinator
		if len(c.originatorNodePool) == 0 {
			log.L(ctx).Warnf("no pool to select a coordinator from yet")
			return "", nil
		}

		// Round block number down to the nearest block range (e.g. block 1012, 1013, 1014 etc. all become 1000 for hashing)
		effectiveBlockNumber := c.currentBlockHeight - (c.currentBlockHeight % c.coordinatorSelectionBlockRange)

		// Take a numeric hash of the identities using the current block range
		h := fnv.New32a()
		h.Write([]byte(strconv.FormatUint(effectiveBlockNumber, 10)))
		primaryIdx := int(h.Sum32()) % len(c.originatorNodePool)
		selectedIdx := (primaryIdx + c.coordinatorFailoverIndex) % len(c.originatorNodePool)
		coordinatorNode = c.originatorNodePool[selectedIdx]
		log.L(ctx).Debugf("coordinator %s selected (primaryCandidate=%s, failoverIndex=%d) based on effective block %d, pool %+v",
			coordinatorNode, c.originatorNodePool[primaryIdx], c.coordinatorFailoverIndex, effectiveBlockNumber, c.originatorNodePool)
	} else if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_SENDER {
		// E.g. Zeto
		log.L(ctx).Debugf("coordinator %s selected as next active coordinator in originator coordinator mode", c.nodeName)
		coordinatorNode = c.nodeName
	}

	log.L(ctx).Debugf("selected active coordinator for contract %s: %s", c.contractAddress.String(), coordinatorNode)

	return coordinatorNode, nil
}

func action_UpdateOriginatorNodePoolFromEvent(_ context.Context, c *coordinator, event common.Event) error {
	e := event.(*OriginatorNodePoolUpdateRequestedEvent)
	for _, node := range e.Nodes {
		c.updateOriginatorNodePool(node)
	}
	return nil
}

func (c *coordinator) updateOriginatorNodePool(originatorNode string) {
	if !slices.Contains(c.originatorNodePool, originatorNode) {
		c.originatorNodePool = append(c.originatorNodePool, originatorNode)
	}
	if !slices.Contains(c.originatorNodePool, c.nodeName) {
		// As coordinator we should always be in the pool as it's used to select the next coordinator when necessary
		c.originatorNodePool = append(c.originatorNodePool, c.nodeName)
	}
	slices.Sort(c.originatorNodePool)
}

// action_AttemptCoordinatorFailover is triggered when the currently observed coordinator has not
// sent a heartbeat for long enough to exceed the inactiveToIdleGracePeriod. It advances
// coordinatorFailoverIndex by one and selects the next deterministic candidate from the
// originatorNodePool. All peers independently compute the same candidate because they share
// the same sorted pool, block range, and failover counter progression.
//
// If the new candidate responds with a heartbeat, coordinatorFailoverIndex is reset to 0
// (in action_HeartbeatReceived) and normal operation resumes.
//
// If every node in the pool has been tried without response, coordinatorFailoverIndex wraps
// past the pool size: the index is reset to 0, activeCoordinatorNode is cleared, and the
// coordinator transitions to State_Idle.
//
// Only COORDINATOR_ENDORSER mode supports multi-node failover. For COORDINATOR_STATIC and
// COORDINATOR_SENDER modes, activeCoordinatorNode is cleared so the state machine can
// transition directly to State_Idle.
func action_AttemptCoordinatorFailover(ctx context.Context, c *coordinator, _ common.Event) error {
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() != prototk.ContractConfig_COORDINATOR_ENDORSER {
		// Static and sender modes have no fallback; go idle.
		c.activeCoordinatorNode = ""
		return nil
	}
	if len(c.originatorNodePool) <= 1 {
		log.L(ctx).Warnf("coordinator failover: pool has ≤1 node, going idle")
		c.activeCoordinatorNode = ""
		return nil
	}

	c.coordinatorFailoverIndex++
	if c.coordinatorFailoverIndex >= len(c.originatorNodePool) {
		// All candidates have been tried without a heartbeat response.
		log.L(ctx).Warnf("coordinator failover: all %d candidates in pool tried without response, going idle", len(c.originatorNodePool))
		c.coordinatorFailoverIndex = 0
		c.activeCoordinatorNode = ""
		return nil
	}

	newCoordinator, err := c.selectActiveCoordinatorNode(ctx)
	if err != nil || newCoordinator == "" {
		log.L(ctx).Warnf("coordinator failover: selection returned empty/error at failoverIndex=%d, going idle: %v", c.coordinatorFailoverIndex, err)
		c.activeCoordinatorNode = ""
		return nil
	}

	log.L(ctx).Infof("coordinator failover attempt %d: switching from %s to next candidate %s",
		c.coordinatorFailoverIndex, c.activeCoordinatorNode, newCoordinator)
	c.activeCoordinatorNode = newCoordinator
	c.coordinatorActive(c.contractAddress, newCoordinator)
	// Give the new candidate a fresh window to respond.
	c.heartbeatIntervalsSinceLastReceive = 0
	return nil
}

// action_ReevaluateCoordinatorOnNewBlock is called when a new block is received while in
// State_Observing. If the effective block range has changed (causing action_NewBlock to
// have reset coordinatorFailoverIndex to 0), this action re-runs coordinator selection so
// that the primary candidate for the new block range is tried first.
func action_ReevaluateCoordinatorOnNewBlock(ctx context.Context, c *coordinator, _ common.Event) error {
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() != prototk.ContractConfig_COORDINATOR_ENDORSER {
		return nil
	}
	if len(c.originatorNodePool) == 0 {
		return nil
	}
	newCoordinator, err := c.selectActiveCoordinatorNode(ctx)
	if err != nil || newCoordinator == "" {
		return nil
	}
	if newCoordinator != c.activeCoordinatorNode {
		log.L(ctx).Infof("coordinator selection changed after new block: %s → %s (failoverIndex=%d)",
			c.activeCoordinatorNode, newCoordinator, c.coordinatorFailoverIndex)
		c.activeCoordinatorNode = newCoordinator
		c.coordinatorActive(c.contractAddress, newCoordinator)
		c.heartbeatIntervalsSinceLastReceive = 0
	}
	return nil
}
