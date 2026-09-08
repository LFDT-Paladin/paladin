// Copyright © 2026 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package statemgr

import (
	"context"
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

func (ss *stateManager) TransferState(ctx context.Context, dbTX persistence.DBTX, domain string, stateID pldtypes.HexBytes, recipient pldtypes.PrivateIdentityLocator) (uuid.UUID, error) {
	ctx = log.WithComponent(ctx, "statemanager")

	identity, node, err := recipient.Validate(ctx, ss.transportManager.LocalNodeName(), false)
	if err != nil {
		return uuid.Nil, err
	}
	recipient = pldtypes.PrivateIdentityLocator(fmt.Sprintf("%s@%s", identity, node))

	if node == ss.transportManager.LocalNodeName() {
		log.L(ctx).Debugf("State transfer to local recipient %s is a no-op", recipient)
		return uuid.Nil, nil
	}

	states, err := ss.GetStatesByID(ctx, dbTX, domain, nil, []pldtypes.HexBytes{stateID}, true, false)
	if err != nil {
		return uuid.Nil, err
	}
	state := states[0]

	sd := &components.StateDistribution{
		StateID:         state.ID.String(),
		IdentityLocator: recipient.String(),
		Domain:          domain,
		ContractAddress: state.ContractAddress.String(),
		SchemaID:        state.Schema.String(),
	}

	msg := &pldapi.ReliableMessage{
		Node:        node,
		MessageType: pldapi.RMTState.Enum(),
		Metadata:    pldtypes.JSONString(sd),
	}
	if err := ss.transportManager.SendReliable(ctx, dbTX, msg); err != nil {
		return uuid.Nil, err
	}

	log.L(ctx).Debugf("Queued state transfer for state %s to recipient %s on node %s (message=%s)", state.ID, recipient, node, msg.ID)
	return msg.ID, nil
}
