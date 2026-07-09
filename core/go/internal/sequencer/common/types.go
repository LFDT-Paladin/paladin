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
	"context"

	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// The CoordinatorSnapshot proto is converted wholesale to native types (uuid.UUID / pldtypes.EthAddress / Bytes32 /
// HexBytes) once at the transport boundary, rather than per-field on demand: the state-machine
// consumers would otherwise repeat those conversions on every pass over the snapshot.

// CoordinatorSnapshot must only contain information about transactions which is (or will eventually be)
// known on the base ledger (e.g. hash, nonce, signer, revert reason). It must not contain information
// such as the transaction originator.
type CoordinatorSnapshot struct {
	DispatchedTransactions []*SnapshotDispatchedTransaction `json:"dispatchedTransactions"`
	PooledTransactions     []*SnapshotPooledTransaction     `json:"pooledTransactions"`
	ConfirmedTransactions  []*SnapshotConfirmedTransaction  `json:"confirmedTransactions"`
	RevertedTransactions   []*SnapshotRevertedTransaction   `json:"revertedTransactions"`
	CoordinatorState       CoordinatorState                 `json:"coordinatorState"`
	BlockHeight            uint64                           `json:"blockHeight"`
	EndorserCandidates     []string                         `json:"endorserCandidates,omitempty"` // (COORDINATOR_ENDORSER selection mode only)
}

type SnapshotPooledTransaction struct {
	ID uuid.UUID
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
	Hash pldtypes.Bytes32
}

type SnapshotRevertedTransaction struct {
	SnapshotPooledTransaction
	RevertReason pldtypes.HexBytes
}

func CoordinatorSnapshotFromProto(ctx context.Context, p *engineProto.CoordinatorSnapshot) (*CoordinatorSnapshot, error) {
	if p == nil {
		return nil, nil
	}
	s := &CoordinatorSnapshot{
		CoordinatorState:   CoordinatorState(int(p.GetCoordinatorState())),
		BlockHeight:        p.GetBlockHeight(),
		EndorserCandidates: p.GetEndorserCandidates(),
	}
	for _, pt := range p.GetPooledTransactions() {
		id, err := uuid.Parse(pt.GetId())
		if err != nil {
			return nil, err
		}
		s.PooledTransactions = append(s.PooledTransactions, &SnapshotPooledTransaction{ID: id})
	}
	for _, pt := range p.GetDispatchedTransactions() {
		dt, err := dispatchedFromProto(ctx, pt.GetId(), pt.GetSigner(), pt.LatestSubmissionHash, pt.Nonce)
		if err != nil {
			return nil, err
		}
		s.DispatchedTransactions = append(s.DispatchedTransactions, dt)
	}
	for _, pt := range p.GetConfirmedTransactions() {
		dt, err := dispatchedFromProto(ctx, pt.GetId(), pt.GetSigner(), pt.LatestSubmissionHash, pt.Nonce)
		if err != nil {
			return nil, err
		}
		var hash pldtypes.Bytes32
		if pt.GetHash() != "" {
			hash, err = pldtypes.ParseBytes32Ctx(ctx, pt.GetHash())
			if err != nil {
				return nil, err
			}
		}
		s.ConfirmedTransactions = append(s.ConfirmedTransactions, &SnapshotConfirmedTransaction{
			SnapshotDispatchedTransaction: *dt,
			Hash:                          hash,
		})
	}
	for _, pt := range p.GetRevertedTransactions() {
		id, err := uuid.Parse(pt.GetId())
		if err != nil {
			return nil, err
		}
		revertReason, err := pldtypes.ParseHexBytes(ctx, pt.GetRevertReason())
		if err != nil {
			return nil, err
		}
		s.RevertedTransactions = append(s.RevertedTransactions, &SnapshotRevertedTransaction{
			SnapshotPooledTransaction: SnapshotPooledTransaction{ID: id},
			RevertReason:              revertReason,
		})
	}
	return s, nil
}

func dispatchedFromProto(ctx context.Context, id, signer string, latestSubmissionHash *string, nonce *uint64) (*SnapshotDispatchedTransaction, error) {
	txID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	dt := &SnapshotDispatchedTransaction{
		SnapshotPooledTransaction: SnapshotPooledTransaction{ID: txID},
		Nonce:                     nonce,
	}
	// A transaction can be in Ready_For_Dispatch before the dispatcher thread has collected it and
	// assigned a signer, so the snapshot legitimately carries an empty signer. Leave the zero address
	// in that case rather than failing to parse the whole snapshot.
	if signer != "" {
		signerAddr, err := pldtypes.ParseEthAddress(signer)
		if err != nil {
			return nil, err
		}
		dt.Signer = *signerAddr
	}
	if latestSubmissionHash != nil {
		h, err := pldtypes.ParseBytes32Ctx(ctx, *latestSubmissionHash)
		if err != nil {
			return nil, err
		}
		dt.LatestSubmissionHash = &h
	}
	return dt, nil
}
