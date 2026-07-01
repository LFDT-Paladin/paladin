// Copyright © 2024 Kaleido, Inc.
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
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
)

type logStateSummary []*pldapi.State

func (lr logStateSummary) String() string {
	summary := make([]string, len(lr))
	for i, s := range lr {
		summary[i] = fmt.Sprintf("schema=%s/id=%s/contract=%s", s.Schema, s.ID, s.ContractAddress)
	}
	return strings.Join(summary, ",")
}

// createLogContext enriches a context with domain/contract/schema log fields.
func createLogContext(ctx context.Context, domainName string, contractAddress pldtypes.EthAddress, schemaID *pldtypes.Bytes32) context.Context {
	ctx = log.WithComponent(ctx, log.Component(fmt.Sprintf("domain-ctx-%s", domainName)))
	ctx = log.WithLogField(ctx, "domain", domainName)
	ctx = log.WithLogField(ctx, "contract", contractAddress.String())
	if schemaID != nil {
		ctx = log.WithLogField(ctx, "schema", schemaID.String())
	}
	return ctx
}

// Short-lived, registered in the state manager. Always closed by the caller via defer dqc.Close(ctx).
// May import a coordinator snapshot (creatingStates + txLocks) for FindAvailableStates queries.
type domainQueryContext struct {
	ss                 *stateManager
	domainName         string
	customHashFunction bool
	contractAddress    pldtypes.EthAddress
	stateLock          sync.Mutex
	id                 uuid.UUID
	closed             bool
	// creatingStates and txLocks are populated via ImportSnapshot for assembly queries.
	creatingStates map[string]*components.StateWithLabels
	// State locks are an in memory structure only, recording a set of locks associated with each transaction.
	// These are held only in memory, and used during DB queries to create a view on top of the database
	// that can make both additional states available, and remove visibility to states.
	txLocks []*pldapi.StateLock
}

// Very important that callers Close domain query contexts they open.
func (ss *stateManager) NewDomainQueryContext(ctx context.Context, domain components.Domain, contractAddress pldtypes.EthAddress) components.DomainQueryContext {
	id := uuid.New()
	log.L(ctx).Debugf("Domain context %s for domain %s contract %s created", id, domain.Name(), contractAddress)

	ss.domainContextLock.Lock()
	defer ss.domainContextLock.Unlock()

	dqc := &domainQueryContext{
		ss:                 ss,
		domainName:         domain.Name(),
		customHashFunction: domain.CustomHashFunction(),
		contractAddress:    contractAddress,
		id:                 id,
		creatingStates:     make(map[string]*components.StateWithLabels),
	}
	ss.domainContexts[id] = dqc
	return dqc
}

// nil if not found
func (ss *stateManager) GetDomainQueryContext(ctx context.Context, id uuid.UUID) components.DomainQueryContext {
	ss.domainContextLock.Lock()
	defer ss.domainContextLock.Unlock()

	ret, found := ss.domainContexts[id]
	if found {
		return ret
	}
	return nil // means an actual nil value to the interface
}

// MUST hold the stateLock to call this function.
func (dqc *domainQueryContext) checkClosed(ctx context.Context) error {
	if dqc.closed {
		return i18n.NewError(ctx, msgs.MsgStateDomainContextClosed)
	}
	return nil
}

// ID returns the UUID that identifies this context in the state manager registry.
func (dqc *domainQueryContext) ID() uuid.UUID {
	return dqc.id
}

// ContractAddress returns the contract address this context was opened for.
func (dqc *domainQueryContext) ContractAddress() pldtypes.EthAddress {
	return dqc.contractAddress
}

// Close deregisters the context from the state manager.
func (dqc *domainQueryContext) Close(ctx context.Context) {
	dqc.stateLock.Lock()
	dqc.closed = true
	dqc.stateLock.Unlock()

	log.L(ctx).Debugf("Domain query context %s for domain %s contract %s closed", dqc.id, dqc.domainName, dqc.contractAddress)

	dqc.ss.domainContextLock.Lock()
	defer dqc.ss.domainContextLock.Unlock()
	delete(dqc.ss.domainContexts, dqc.id)
}

type exportSnapshot struct {
	States []*components.StateUpsert `json:"states"`
	Locks  []*exportableStateLock    `json:"locks"`
}

// pldapi.StateLocks do not include the stateID in the serialized JSON so we need a separate struct.
type exportableStateLock struct {
	State       pldtypes.HexBytes                   `json:"stateId"`
	Transaction uuid.UUID                           `json:"transaction"`
	Type        pldtypes.Enum[pldapi.StateLockType] `json:"type"`
}

// ImportSnapshot hydrates this context from a coordinator grapher export (JSON).
// Populates creatingStates and txLocks for assembly queries
func (dqc *domainQueryContext) ImportSnapshot(ctx context.Context, stateLocksJSON []byte) error {
	ctx = createLogContext(ctx, dqc.domainName, dqc.contractAddress, nil)
	dqc.stateLock.Lock()
	defer dqc.stateLock.Unlock()
	if err := dqc.checkClosed(ctx); err != nil {
		return err
	}

	var snapshot exportSnapshot
	err := json.Unmarshal(stateLocksJSON, &snapshot)
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainContextImportInvalidJSON)
	}

	// Validate and process the snapshot states without appending to any DB write buffer.
	vss, err := dqc.ss.validateStateSet(ctx, dqc.domainName, dqc.contractAddress, dqc.customHashFunction, dqc.ss.p.NOTX(), snapshot.States...)
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainContextImportBadStates)
	}

	processedStates := make(map[string]*components.StateWithLabels, len(vss.withValues))
	for _, vs := range vss.withValues {
		processedStates[vs.ID.String()] = vs
	}

	dqc.creatingStates = make(map[string]*components.StateWithLabels)
	dqc.txLocks = make([]*pldapi.StateLock, 0, len(snapshot.Locks))
	for _, l := range snapshot.Locks {
		dqc.txLocks = append(dqc.txLocks, &pldapi.StateLock{
			DomainName:  dqc.domainName,
			StateID:     l.State,
			Transaction: l.Transaction,
			Type:        l.Type,
		})
		if l.Type == pldapi.StateLockTypeCreate.Enum() {
			if state, found := processedStates[l.State.String()]; found {
				dqc.creatingStates[state.ID.String()] = state
			} else {
				// The state distribution message may not have arrived yet, or we are not on the list.
				// The domain can Park the transaction if it suspects the former.
				log.L(ctx).Infof("ImportSnapshot: state %s not found in snapshot states", l.State)
			}
		}
	}

	return nil
}

func (dqc *domainQueryContext) applyLocks(ctx context.Context, states []*pldapi.State) []*pldapi.State {
	for _, s := range states {
		s.Locks = []*pldapi.StateLock{}
		for _, l := range dqc.txLocks {
			if l.StateID.Equals(s.ID) {
				log.L(ctx).Tracef("state %s is locked by %s", s.ID, l.Transaction)
				s.Locks = append(s.Locks, l)
			}
		}
	}
	return states
}

func (dqc *domainQueryContext) mergeSnapshotStates(ctx context.Context, schema components.Schema, dbStates []*pldapi.State, q *query.QueryJSON, excludeSpent, requireNullifier bool) (_ []*components.StateWithLabels, err error) {
	matches := make([]*components.StateWithLabels, 0, len(dqc.creatingStates))
	schemaId := schema.Persisted().ID
	for _, state := range dqc.creatingStates {
		log.L(ctx).Tracef("State %s is a creating state", state.ID)
		if !state.Schema.Equals(&schemaId) {
			continue
		}
		if excludeSpent {
			spent := false
			for _, lock := range dqc.txLocks {
				if lock.StateID.Equals(state.ID) && lock.Type.V() == pldapi.StateLockTypeSpend {
					log.L(ctx).Tracef("State %s is spent by transaction %s - not including in the response", state.ID, lock.Transaction)
					spent = true
					break
				}
			}
			if spent {
				continue
			}
		}

		if requireNullifier && state.Nullifier == nil {
			continue
		}

		labelSet := dqc.ss.labelSetFor(schema)
		match, err := filters.EvalQuery(ctx, q, labelSet, state.LabelValues)
		if err != nil {
			return nil, err
		}
		if match {
			dup := false
			for _, dbState := range dbStates {
				if dbState.ID.Equals(state.ID) {
					dup = true
					break
				}
			}
			if !dup {
				log.L(ctx).Tracef("Matched state %s from snapshot", &state.ID)
				shallowCopy := *state
				matches = append(matches, &shallowCopy)
			}
		}
	}

	if log.IsTraceEnabled() {
		log.L(ctx).Tracef("mergeSnapshotStates: found %d matches", len(matches))
		for _, m := range matches {
			log.L(ctx).Tracef("Matched state: %s", m.ID)
		}
	}

	return matches, nil
}

func (dqc *domainQueryContext) mergeInMemoryMatches(ctx context.Context, schema components.Schema, states []*pldapi.State, extras []*components.StateWithLabels, q *query.QueryJSON) (_ []*pldapi.State, err error) {
	fullList := make([]*components.StateWithLabels, len(states), len(states)+len(extras))
	persistedStateIDs := make(map[string]bool)
	for i, s := range states {
		if fullList[i], err = schema.RecoverLabels(ctx, s); err != nil {
			return nil, err
		}
		persistedStateIDs[s.ID.String()] = true
	}

	for _, s := range extras {
		if !persistedStateIDs[s.ID.String()] {
			fullList = append(fullList, s)
		}
	}

	sortInstructions := q.Sort
	if err = filters.SortValueSetInPlace(ctx, dqc.ss.labelSetFor(schema), fullList, sortInstructions...); err != nil {
		return nil, err
	}

	listLen := len(fullList)
	if q.Limit != nil && listLen > *q.Limit {
		listLen = *q.Limit
	}
	retList := make([]*pldapi.State, listLen)
	for i := 0; i < listLen; i++ {
		retList[i] = fullList[i].State
	}
	return retList, nil
}

// mergeSnapshotApplyLocks merges snapshot creatingStates with DB results and applies locks.
func (dqc *domainQueryContext) mergeSnapshotApplyLocks(ctx context.Context, schema components.Schema, dbStates []*pldapi.State, q *query.QueryJSON, excludeSpent, requireNullifier bool) (_ []*pldapi.State, err error) {
	log.L(ctx).Debugf("domainQueryContext:mergeSnapshotApplyLocks txLocks=%d creatingStates=%d", len(dqc.txLocks), len(dqc.creatingStates))
	dqc.stateLock.Lock()
	defer dqc.stateLock.Unlock()
	if err := dqc.checkClosed(ctx); err != nil {
		return nil, err
	}

	retStates := dbStates
	matches, err := dqc.mergeSnapshotStates(ctx, schema, dbStates, q, excludeSpent, requireNullifier)
	if err != nil {
		return nil, err
	}
	if len(matches) > 0 {
		if retStates, err = dqc.mergeInMemoryMatches(ctx, schema, dbStates, matches, q); err != nil {
			return nil, err
		}
	}

	return dqc.applyLocks(ctx, retStates), nil
}

// getSnapshotSpends returns spend locks from the snapshot-loaded txLocks.
func (dqc *domainQueryContext) getSnapshotSpends(ctx context.Context) (spending []pldtypes.HexBytes, err error) {
	dqc.stateLock.Lock()
	defer dqc.stateLock.Unlock()
	if err = dqc.checkClosed(ctx); err != nil {
		return nil, err
	}

	for _, l := range dqc.txLocks {
		if l.Type.V() == pldapi.StateLockTypeSpend {
			spending = append(spending, l.StateID)
		}
	}
	return spending, nil
}

// FindAvailableStates queries available states, merging snapshot creatingStates and applying locks.
func (dqc *domainQueryContext) FindAvailableStates(ctx context.Context, dbTX persistence.DBTX, schemaID pldtypes.Bytes32, q *query.QueryJSON) (components.Schema, []*pldapi.State, error) {
	ctx = createLogContext(ctx, dqc.domainName, dqc.contractAddress, &schemaID)
	log.L(ctx).Debugf("FindAvailableStates query=%s", q)

	spending, err := dqc.getSnapshotSpends(ctx)
	if err != nil {
		return nil, nil, err
	}

	if log.IsTraceEnabled() {
		log.L(ctx).Tracef("Snapshot spend locks: %d", len(spending))
		for _, s := range spending {
			log.L(ctx).Tracef("Snapshot spend: %s", s.String())
		}
	}

	schema, states, err := dqc.ss.findStates(ctx, dbTX, dqc.domainName, &dqc.contractAddress, schemaID, q, &components.StateQueryOptions{
		StatusQualifier: pldapi.StateStatusAvailable,
		ExcludedIDs:     spending,
	})
	if err != nil {
		return nil, nil, err
	}
	log.L(ctx).Tracef("FindAvailableStates read %d states from DB", len(states))

	states, err = dqc.mergeSnapshotApplyLocks(ctx, schema, states, q, true /* exclude spent */, false)
	if log.IsTraceEnabled() {
		for _, s := range states {
			log.L(ctx).Tracef("returning available state %s", s.ID)
		}
	}
	log.L(ctx).Debugf("FindAvailableStates read+merged %d states: %s", len(states), logStateSummary(states))

	return schema, states, err
}

// FindAvailableNullifiers queries available nullifier-based states, merging snapshot state.
func (dqc *domainQueryContext) FindAvailableNullifiers(ctx context.Context, dbTX persistence.DBTX, schemaID pldtypes.Bytes32, q *query.QueryJSON) (components.Schema, []*pldapi.State, error) {
	ctx = createLogContext(ctx, dqc.domainName, dqc.contractAddress, &schemaID)
	log.L(ctx).Debugf("FindAvailableNullifiers query=%s", q)

	spending, err := dqc.getSnapshotSpends(ctx)
	if err != nil {
		return nil, nil, err
	}

	// For snapshot-loaded contexts, nullifiers are on creatingStates entries; no unFlushed buffer.
	// Pass empty nullifierIDs — committed nullifiers are queryable via the DB directly.
	schema, states, err := dqc.ss.findNullifiers(ctx, dbTX, dqc.domainName, &dqc.contractAddress, schemaID, q, pldapi.StateStatusAvailable, spending, nil)
	if err != nil {
		return nil, nil, err
	}

	states, err = dqc.mergeSnapshotApplyLocks(ctx, schema, states, q, true /* exclude spent */, true)
	return schema, states, err
}

// GetStatesByID retrieves states by ID regardless of confirmation/spend status,
// including states pending in memory from a snapshot.
func (dqc *domainQueryContext) GetStatesByID(ctx context.Context, dbTX persistence.DBTX, schemaID pldtypes.Bytes32, ids []string) (components.Schema, []*pldapi.State, error) {
	ctx = createLogContext(ctx, dqc.domainName, dqc.contractAddress, &schemaID)
	idsAny := make([]any, len(ids))
	for i, id := range ids {
		idsAny[i] = id
	}
	q := query.NewQueryBuilder().In(".id", idsAny).Sort(".created").Query()
	schema, matches, err := dqc.ss.findStates(ctx, dbTX, dqc.domainName, &dqc.contractAddress, schemaID, q, &components.StateQueryOptions{
		StatusQualifier: pldapi.StateStatusAll,
	})
	if err == nil {
		var memMatches []*components.StateWithLabels
		memMatches, err = dqc.mergeSnapshotStates(ctx, schema, matches, q, false /* locked states are fine */, false /* nullifiers not required */)
		if err == nil && len(memMatches) > 0 {
			matches, err = dqc.mergeInMemoryMatches(ctx, schema, matches, memMatches, q)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	return schema, matches, err
}

