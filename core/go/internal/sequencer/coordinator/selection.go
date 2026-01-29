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
	c.activeCoordinatorNode = selectedCoordinator
	c.coordinatorActive(c.contractAddress, selectedCoordinator)
	return nil
}

// The name of this function gives the impression that it is modifying state but it is actually
// using the existing state as the input to the algorithm that chooses the current active coordinator.
// It returns the name of this node, but doesn't make it the active coordinator
// TODO AM: maybe it could select
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
		} else {
			// Round block number down to the nearest block range (e.g. block 1012, 1013, 1014 etc. all become 1000 for hashing)
			effectiveBlockNumber := c.currentBlockHeight - (c.currentBlockHeight % c.coordinatorSelectionBlockRange)

			// Take a numeric hash of the identities using the current block range
			h := fnv.New32a()
			h.Write([]byte(strconv.FormatUint(effectiveBlockNumber, 10)))
			coordinatorNode = c.originatorNodePool[int(h.Sum32())%len(c.originatorNodePool)]
			log.L(ctx).Debugf("coordinator %s selected based on hash modulus of the originator pool %+v", coordinatorNode, c.originatorNodePool)
		}
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
