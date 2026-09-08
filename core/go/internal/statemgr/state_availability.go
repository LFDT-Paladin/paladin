// Copyright contributors to Paladin, an LFDT project
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

	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// The confirmed/spent columns on the states table are denormalized from state_confirm_records/state_spend_records
// so the "available" query can use the states_available partial index instead of anti-joining the
// ever-growing record tables.
//
// Private state data are distibuted off chain via reliable messaging, whereas confirm/spend records
// are created as result of indexing the base ledger. This means that:
//
// - confirm/spend records may arrive before the private state data
// - confirm/spend records may arrive after the private state data
// - confirm/spend records may arrive for states for which this node will never receive the private state data
// - confirm/spend records may arrive concurrently with the private state data (different goroutines are responsible
//   for persisting to each of the tables)
//
// As a result, to ensure consistency we
// - check for confirm/spend records when persisting a newly received private state and set the
//   confirmed/spent columns accordingly
// - check for matching private states when indexing a confirm/spend record and again set the
//   confirmed/spent columns accordingly
// - use a named DB lock to synchronise between goroutines writing private state data and confirm/spend locks
//
// Auto vacuum will keep the size of partial index small over time, compared to the full set of spent states.
// There is a cost to auto vacuum but this should be outweighed by the benefit of avoiding the full antijoin.
// The partial index should stay proportional to usage. It will only grow if a large number of states are
// confirmed but never spent.

const (
	availabilityFlagConfirmed = "confirmed"
	availabilityFlagSpent     = "spent"
)

const availabilityFlagLock = "state_availability_flags"

func setStatesConfirmed(ctx context.Context, dbTX persistence.DBTX, confirms []*pldapi.StateConfirmRecord) error {
	idsByDomain := make(map[string][]pldtypes.HexBytes, 1)
	for _, c := range confirms {
		idsByDomain[c.DomainName] = append(idsByDomain[c.DomainName], c.State)
	}
	for domainName, ids := range idsByDomain {
		if err := dbTX.DB(ctx).
			Table("states").
			Where("domain_name = ?", domainName).
			Where("id IN ?", ids).
			Where(`"confirmed" = FALSE`).
			Update(availabilityFlagConfirmed, true).
			Error; err != nil {
			return err
		}
	}
	return nil
}

func setStatesSpent(ctx context.Context, dbTX persistence.DBTX, spends []*pldapi.StateSpendRecord) error {
	idsByDomain := make(map[string][]pldtypes.HexBytes, 1)
	for _, s := range spends {
		idsByDomain[s.DomainName] = append(idsByDomain[s.DomainName], s.State)
	}
	for domainName, ids := range idsByDomain {
		if err := dbTX.DB(ctx).
			Table("states").
			Where("domain_name = ?", domainName).
			Where("id IN ?", ids).
			Where(`"spent" = FALSE`).
			Update(availabilityFlagSpent, true).
			Error; err != nil {
			return err
		}
	}
	return nil
}

func (ss *stateManager) setStateAvailableFromSpendConfirmRecords(ctx context.Context, dbTX persistence.DBTX, states []*pldapi.State) error {
	if len(states) == 0 {
		return nil
	}
	if err := ss.p.TakeNamedLock(ctx, dbTX, availabilityFlagLock); err != nil {
		return err
	}

	idsByDomain := make(map[string][]pldtypes.HexBytes, 1)
	for _, s := range states {
		idsByDomain[s.DomainName] = append(idsByDomain[s.DomainName], s.ID)
	}

	for domainName, ids := range idsByDomain {
		if err := reconcileAvailabilityFlag(ctx, dbTX, domainName, ids, availabilityFlagConfirmed, "state_confirm_records"); err != nil {
			return err
		}
		if err := reconcileAvailabilityFlag(ctx, dbTX, domainName, ids, availabilityFlagSpent, "state_spend_records"); err != nil {
			return err
		}
	}
	return nil
}

func reconcileAvailabilityFlag(ctx context.Context, dbTX persistence.DBTX, domainName string, ids []pldtypes.HexBytes, column, recordTable string) error {
	return dbTX.DB(ctx).
		Table("states").
		Where("domain_name = ?", domainName).
		Where("id IN ?", ids).
		Where(fmt.Sprintf(`"%s" = FALSE`, column)).
		Where(fmt.Sprintf(`EXISTS (SELECT 1 FROM %s r WHERE r.domain_name = states.domain_name AND r.state = states.id)`, recordTable)).
		Update(column, true).
		Error
}
