/*
 * Copyright © 2024 Kaleido, Inc.
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

package privatetxnmgr

import (
	"context"
	"fmt"
	"hash/fnv"
	"slices"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// Coordinator selector policy is either
//  - coordinator node is statically configured in the contract
//  - deterministic and fair rotation between a predefined set of endorsers
//  - the sender of the transaction coordinates the transaction
//
// Submitter selection policy is either
// - Coordinator submits
// - Sender submits
// - 3rd party submission

// Currently only the following combinations are implemented
// 1+1 - core option set for Noto
// 2+1 - core option set for Pente
// 3+2 - core option set for Zeto

type CoordinatorSelectionMode int

const (
	BlockHeightRoundRobin CoordinatorSelectionMode = iota
	HashedSelection       CoordinatorSelectionMode = iota
)

// Override only intended for unit tests currently
var EndorsementCoordinatorSelectionMode CoordinatorSelectionMode = HashedSelection

func NewCoordinatorSelector(ctx context.Context, nodeName string, contractConfig *prototk.ContractConfig, sequencerConfig pldconf.PrivateTxManagerSequencerConfig) (ptmgrtypes.CoordinatorSelector, error) {
	if contractConfig.GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_SENDER {
		return &staticCoordinatorSelectorPolicy{
			nodeName: nodeName,
		}, nil
	}
	if contractConfig.GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_STATIC {
		staticCoordinator := contractConfig.GetStaticCoordinator()
		//staticCoordinator must be a fully qualified identity because it is also used to locate the signing key
		// but at this point, we only need the node name
		staticCoordinatorNode, err := pldtypes.PrivateIdentityLocator(staticCoordinator).Node(ctx, false)
		if err != nil {
			log.L(ctx).Errorf("Error resolving node for static coordinator %s: %s", staticCoordinator, err)
			return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, err)
		}

		return &staticCoordinatorSelectorPolicy{
			nodeName: staticCoordinatorNode,
		}, nil
	}
	if contractConfig.GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_ENDORSER {
		if EndorsementCoordinatorSelectionMode == BlockHeightRoundRobin {
			return &roundRobinCoordinatorSelectorPolicy{
				localNode: nodeName,
				rangeSize: confutil.Int(sequencerConfig.RoundRobinCoordinatorBlockRangeSize, *pldconf.PrivateTxManagerDefaults.Sequencer.RoundRobinCoordinatorBlockRangeSize),
			}, nil
		}
		// TODO: More work is required to perform leader election of an endorser, so right now a simple hash algorithm is used.
		return &endorsementSetHashSelection{
			localNode: nodeName,
		}, nil
	}
	return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidCoordinatorSelection, contractConfig.GetCoordinatorSelection())
}

func getEndorsementSet(ctx context.Context, localNode string, transaction *components.PrivateTransaction) (identities, uniqueNodes []string, err error) {
	var candidateParties []string
	if len(transaction.PreAssembly.EndorsementSet) > 0 {
		// During InitTransaction() the domain should return an endorsement set, as otherwise we wouldn't know
		// the right coordinator to use without speculative assembly of the transaction
		candidateParties = append(candidateParties, transaction.PreAssembly.EndorsementSet...)
	} else if transaction.PostAssembly != nil {
		// This code path is left in from before we added the endorsement_set to the InitTransaction() return.
		// The V1 stream will clean this up as we fully implement and test the move away from hash-based election,
		// to fully dynamic election of the coordinator.
		// For now this code path in V0 is left as a safety net.
		for _, attestationPlan := range transaction.PostAssembly.AttestationPlan {
			if attestationPlan.AttestationType == prototk.AttestationType_ENDORSE {
				candidateParties = append(candidateParties, attestationPlan.Parties...)
			}
		}
	}
	candidateNodesMap := make(map[string]struct{})
	for _, party := range candidateParties {
		identity, node, err := pldtypes.PrivateIdentityLocator(party).Validate(ctx, localNode, false)
		if err != nil {
			log.L(ctx).Errorf("SelectCoordinatorNode: Error resolving node for party %s: %s", party, err)
			return nil, nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, err)
		}
		candidateNodesMap[node] = struct{}{}
		identities = append(identities, fmt.Sprintf("%s@%s", identity, node))
	}

	uniqueNodes = make([]string, 0, len(candidateNodesMap))
	for candidateNode := range candidateNodesMap {
		uniqueNodes = append(uniqueNodes, candidateNode)
	}
	slices.Sort(uniqueNodes)
	slices.Sort(identities)

	return
}

type staticCoordinatorSelectorPolicy struct {
	nodeName string
}

type endorsementSetHashSelection struct {
	localNode  string
	chosenNode string
}

func (s *staticCoordinatorSelectorPolicy) SelectCoordinatorNode(ctx context.Context, _ *components.PrivateTransaction, environment ptmgrtypes.SequencerEnvironment) (int64, string, error) {
	log.L(ctx).Debugf("SelectCoordinatorNode: Selecting coordinator node %s", s.nodeName)
	return environment.GetBlockHeight(), s.nodeName, nil
}

func (s *endorsementSetHashSelection) SelectCoordinatorNode(ctx context.Context, transaction *components.PrivateTransaction, environment ptmgrtypes.SequencerEnvironment) (int64, string, error) {
	blockHeight := environment.GetBlockHeight()
	if s.chosenNode == "" {
		identities, uniqueNodes, err := getEndorsementSet(ctx, s.localNode, transaction)
		if err != nil {
			return -1, "", err
		}
		if len(uniqueNodes) == 0 {
			log.L(ctx).Warnf("SelectCoordinatorNode: No candidate nodes, assuming local node is the coordinator for %s", transaction.ID)
			return blockHeight, s.localNode, nil
		}
		// Take a simple numeric hash of the identities string
		h := fnv.New32a()
		for _, identity := range identities {
			h.Write([]byte(identity))
		}
		// Use that as an index into the chosen node set
		s.chosenNode = uniqueNodes[int(h.Sum32())%len(uniqueNodes)]
		log.L(ctx).Infof("SelectCoordinatorNode: Chosen node %s for transaction %s", s.chosenNode, transaction.ID)
	}

	return blockHeight, s.chosenNode, nil

}

type roundRobinCoordinatorSelectorPolicy struct {
	localNode string
	rangeSize int
}

func (s *roundRobinCoordinatorSelectorPolicy) SelectCoordinatorNode(ctx context.Context, transaction *components.PrivateTransaction, environment ptmgrtypes.SequencerEnvironment) (int64, string, error) {
	blockHeight := environment.GetBlockHeight()
	_, uniqueNodes, err := getEndorsementSet(ctx, s.localNode, transaction)
	if err != nil {
		return -1, "", err
	}
	if len(uniqueNodes) == 0 {
		//if we still don't have any candidate nodes, then we can't select a coordinator so just assume we are the coordinator
		log.L(ctx).Debug("SelectCoordinatorNode: No candidate nodes, assuming local node is the coordinator")
		return blockHeight, s.localNode, nil
	}

	rangeIndex := blockHeight / int64(s.rangeSize)

	coordinatorIndex := int(rangeIndex) % len(uniqueNodes)
	coordinatorNode := uniqueNodes[coordinatorIndex]
	log.L(ctx).Debugf("SelectCoordinatorNode: selected coordinator node %s using round robin algorithm for blockHeight: %d and rangeSize %d ", coordinatorNode, blockHeight, s.rangeSize)

	return blockHeight, coordinatorNode, nil

}
