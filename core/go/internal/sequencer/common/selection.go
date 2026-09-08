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

package common

import (
	"context"
	"hash/fnv"
	"slices"
	"strconv"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type CoordinatorSelectionConfig struct {
	Mode              prototk.ContractConfig_CoordinatorSelection
	StaticCoordinator string   // STATIC: validated node name extracted from the locator
	Endorsers         []string // ENDORSER: deduped+sorted candidate nodes + local node; always non-empty
}

// ResolveCoordinatorSelectionConfig validates the coordinator selection configuration from
// the domain ContractConfig and returns a CoordinatorSelectionConfig ready to be passed to
// NewOriginator and NewCoordinator. It must be called before creating either component.
func ResolveCoordinatorSelectionConfig(
	ctx context.Context,
	nodeName string,
	contractAddress *pldtypes.EthAddress,
	contractConfig *prototk.ContractConfig,
) (*CoordinatorSelectionConfig, error) {
	cfg := &CoordinatorSelectionConfig{
		Mode: contractConfig.GetCoordinatorSelection(),
	}

	switch cfg.Mode {
	case prototk.ContractConfig_COORDINATOR_STATIC:
		staticCoordinator := contractConfig.GetStaticCoordinator()
		if staticCoordinator == "" {
			return nil, i18n.NewError(ctx, msgs.MsgSequencerStaticCoordinatorNotSet, contractAddress.String())
		}
		node, err := pldtypes.PrivateIdentityLocator(staticCoordinator).Node(ctx, false)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgSequencerInvalidStaticCoordinator, contractAddress.String(), staticCoordinator)
		}
		cfg.StaticCoordinator = node
		log.L(ctx).Debugf("static coordinator node for contract %s validated and set: %s", contractAddress.String(), node)

	case prototk.ContractConfig_COORDINATOR_SENDER:
		log.L(ctx).Debugf("coordinator selection is SENDER mode for contract %s", contractAddress.String())

	case prototk.ContractConfig_COORDINATOR_ENDORSER:
		candidates := contractConfig.GetCoordinatorEndorserCandidates()
		if len(candidates) == 0 {
			log.L(ctx).Warnf("no coordinator endorser candidates configured for contract %s; defaulting to local node: %s", contractAddress.String(), nodeName)
			cfg.Endorsers = []string{nodeName}
			return cfg, nil
		}
		nodes := make([]string, 0, len(candidates)+1)
		for _, locator := range candidates {
			_, node, err := pldtypes.PrivateIdentityLocator(locator).Validate(ctx, "", false)
			if err != nil {
				return nil, i18n.WrapError(ctx, err, msgs.MsgSequencerInvalidEndorserCandidate, locator)
			}
			nodes = append(nodes, node)
		}
		// Always include the local node so the list is never empty.
		nodes = append(nodes, nodeName)
		cfg.Endorsers = DedupeSortedCoordinatorEndorserNodes(nodes)
		log.L(ctx).Debugf("resolved coordinator endorsers for contract %s: %+v", contractAddress.String(), cfg.Endorsers)
	}

	return cfg, nil
}

// DedupeSortedCoordinatorEndorserNodes sorts node names in place and removes duplicate entries
// (adjacent after sort). Use this when building the endorser pool so hash-modulus selection
// sees one slot per coordinator node.
func DedupeSortedCoordinatorEndorserNodes(nodes []string) []string {
	if len(nodes) == 0 {
		return nodes
	}
	slices.Sort(nodes)
	return slices.Compact(nodes)
}

// ComputeCoordinatorPriorityList returns a priority-ordered list of coordinator nodes for the
// given effective block height. The node at index 0 is the highest-priority (currently selected)
// coordinator; remaining nodes follow in sorted order. All nodes that call this function with the
// same pool and effective block height will independently reach the same result.
//
// For COORDINATOR_STATIC and COORDINATOR_SENDER modes, the coordinator field is set once at
// construction/Start time and this function is never invoked.
func ComputeCoordinatorPriorityList(
	ctx context.Context,
	nodePool []string,
	effectiveBlockHeight uint64,
) []string {
	n := len(nodePool)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return []string{nodePool[0]}
	}

	// Take a numeric hash of the effective block number
	h := fnv.New32a()
	h.Write([]byte(strconv.FormatUint(effectiveBlockHeight, 10)))
	p := int(h.Sum32()) % n
	selected := nodePool[p]
	log.L(ctx).Debugf("coordinator priority list: selected index %d (%q) from pool %+v", p, selected, nodePool)

	// Build priority list: walk the sorted pool starting at the selected index,
	// wrapping around so the ordering is e.g. [3,4,1,2] when p=2 and n=4.
	list := make([]string, n)
	for i := range n {
		list[i] = nodePool[(p+i)%n]
	}
	return list
}

// PriorityIndexOf returns the index of node in the priority list, or len(list) if absent.
// A lower index means higher priority (index 0 is the current active coordinator).
func PriorityIndexOf(list []string, node string) int {
	for i, n := range list {
		if n == node {
			return i
		}
	}
	return len(list)
}

func IsHigherPriority(list []string, node1 string, node2 string) bool {
	idx1 := PriorityIndexOf(list, node1)
	idx2 := PriorityIndexOf(list, node2)
	return idx1 < idx2
}

// ComputeEffectiveBlockHeight returns the epoch-aligned block height for a given raw block
// height and epoch width (blockRange). The result is constant within an epoch, changing only
// when the raw height crosses the next epoch boundary.
func ComputeEffectiveBlockHeight(height, blockRange uint64) uint64 {
	return height - (height % blockRange)
}
