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
package common

import (
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/grapher"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/statevisibilitytracker"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type CoordinatorSnapshot struct {
	DispatchedTransactions []*SnapshotDispatchedTransaction `json:"dispatchedTransactions"`
	PooledTransactions     []*SnapshotPooledTransaction     `json:"pooledTransactions"`
	ConfirmedTransactions  []*SnapshotConfirmedTransaction  `json:"confirmedTransactions"`
	CoordinatorState       CoordinatorState                 `json:"coordinatorState"`
	BlockHeight            uint64                           `json:"blockHeight"`
	// Locks and OutputStates are only populated in Flush and Closing heartbeats, for coordinator handover.
	// Locks contain only on-chain metadata (state IDs, types, block numbers) — no privacy protection needed,
	// as this data ends up on the base ledger. All locks are sent to every recipient node.
	// OutputStates carry private state data and are filtered per node: each recipient only receives
	// the OutputStates where it appears in AllowedNodes. A receiving coordinator can cross-reference the
	// create locks against its OutputStates to validate it has all the private data it needs.
	Locks        []*grapher.StateLock          `json:"locks,omitempty"`
	OutputStates []*statevisibilitytracker.OutputState `json:"outputStates,omitempty"`
}

type SnapshotPooledTransaction struct {
	ID         uuid.UUID
	Originator string
}

func (t *SnapshotPooledTransaction) GetID() string {
	return t.ID.String()
}

type SnapshotDispatchedTransaction struct {
	SnapshotPooledTransaction
	Signer               pldtypes.EthAddress
	LatestSubmissionHash *pldtypes.Bytes32
	Nonce                *uint64
}

type SnapshotConfirmedTransaction struct {
	SnapshotDispatchedTransaction
	Hash         pldtypes.Bytes32
	RevertReason pldtypes.HexBytes
}
