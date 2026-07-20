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
	"strings"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
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
	txLocks []*prototk.SnapshotStateLock
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

// ImportSnapshot hydrates this context (typically from a coordinator grapher export)
// Populates creatingStates and txLocks for assembly queries.
func (dqc *domainQueryContext) ImportSnapshot(ctx context.Context, snapshot *prototk.StateSnapshot) error {
	ctx = createLogContext(ctx, dqc.domainName, dqc.contractAddress, nil)
	dqc.stateLock.Lock()
	defer dqc.stateLock.Unlock()
	if err := dqc.checkClosed(ctx); err != nil {
		return err
	}

	// Validate and process the snapshot states
	snapshotStates := snapshot.GetStates()
	processedStates := make(map[string]*components.StateWithLabels, len(snapshotStates))
	for _, snapshotState := range snapshotStates {
		vs, err := dqc.ss.validateAndConvertEndorsableState(ctx, dqc.domainName, dqc.contractAddress, dqc.customHashFunction, dqc.ss.p.NOTX(), snapshotState.GetState(), true)
		if err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgDomainContextImportBadStates)
		}
		processedStates[vs.ID.String()] = vs
	}
	dqc.creatingStates = make(map[string]*components.StateWithLabels)
	dqc.txLocks = snapshot.GetLocks()
	for _, l := range dqc.txLocks {
		if l.GetType() == prototk.SnapshotStateLock_CREATE {
			stateID := l.GetStateId()
			if state, found := processedStates[stateID]; found {
				dqc.creatingStates[state.ID.String()] = state
			}
			// A snapshot can contain create locks for states which already have a corresponding
			// spend lock, in which case the private state data is omitted. A snapshot may also
			// contain a create lock for a state which will not be distributed to this node, in
			// which case the private state data will again be omitted.
		}
	}

	return nil
}

// labelPreloadModifier returns a query modifier that preloads the persisted label rows, but only
// when this context has in-flight snapshot creates — the sole case where mergeInMemoryMatches runs
// against DB states and needs their label values. This is an optimization: RecoverLabels falls back
// to re-parsing the state data when the rows are absent, so returning nil on the common path (no
// extra DB round-trips) stays correct.
//
// TODO: Under sustained load creatingStates is non-empty on essentially every query, so this preload
// fires almost always and its cost (two extra SELECTs, on state_labels and state_int64labels) is
// paid per query. A further optimization is possible: findStatesCommon already INNER-JOINs the
// label tables for the fields referenced by the query's filter/sort, and the recovered values are
// consumed only by the in-memory sort in mergeInMemoryMatches (which needs only the sort-key
// labels). Selecting those already-joined columns into the result would supply the sort values with
// zero extra round-trips and no re-parse, superseding both this preload and the RecoverLabels
// fallback — at the cost of a custom projection/scan, since GORM will not map arbitrary selected
// columns onto pldapi.State.
func (dqc *domainQueryContext) labelPreloadModifier() func(persistence.DBTX, *gorm.DB) *gorm.DB {
	if len(dqc.creatingStates) == 0 {
		return nil
	}
	return func(_ persistence.DBTX, q *gorm.DB) *gorm.DB {
		return q.Preload("Labels").Preload("Int64Labels")
	}
}

func (dqc *domainQueryContext) findSnapshotMatches(ctx context.Context, schema components.Schema, dbStates []*pldapi.State, q *query.QueryJSON, excludeSpent, requireNullifier bool) (snapshotMatches []*components.StateWithLabels, err error) {
	snapshotMatches = make([]*components.StateWithLabels, 0, len(dqc.creatingStates))
	schemaId := schema.Persisted().ID
	for _, state := range dqc.creatingStates {
		log.L(ctx).Tracef("State %s is a creating state", state.ID)
		if !state.Schema.Equals(&schemaId) {
			continue
		}
		if excludeSpent {
			spent := false
			for _, lock := range dqc.txLocks {
				if lock.GetStateId() == state.ID.String() && lock.GetType() == prototk.SnapshotStateLock_SPEND {
					log.L(ctx).Tracef("State %s is spent by transaction %s - not including in the response", state.ID, lock.GetTransaction())
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
				snapshotMatches = append(snapshotMatches, &shallowCopy)
			}
		}
	}

	if log.IsTraceEnabled() {
		log.L(ctx).Tracef("findSnapshotMatches: found %d matches", len(snapshotMatches))
		for _, m := range snapshotMatches {
			log.L(ctx).Tracef("Matched state: %s", m.ID)
		}
	}

	return snapshotMatches, nil
}

func (dqc *domainQueryContext) mergeAndSortStates(ctx context.Context, schema components.Schema, states []*pldapi.State, extras []*components.StateWithLabels, q *query.QueryJSON) (_ []*pldapi.State, err error) {
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

// mergeSnapshotStatesResult merges snapshot creatingStates with DB results.
func (dqc *domainQueryContext) mergeSnapshotStatesResult(ctx context.Context, schema components.Schema, dbStates []*pldapi.State, q *query.QueryJSON, excludeSpent, requireNullifier bool) (_ []*pldapi.State, err error) {
	log.L(ctx).Debugf("domainQueryContext:mergeSnapshotStatesResult txLocks=%d creatingStates=%d", len(dqc.txLocks), len(dqc.creatingStates))
	dqc.stateLock.Lock()
	defer dqc.stateLock.Unlock()
	if err := dqc.checkClosed(ctx); err != nil {
		return nil, err
	}

	retStates := dbStates
	snapshotStates, err := dqc.findSnapshotMatches(ctx, schema, dbStates, q, excludeSpent, requireNullifier)
	if err != nil {
		return nil, err
	}
	if len(snapshotStates) > 0 {
		if retStates, err = dqc.mergeAndSortStates(ctx, schema, dbStates, snapshotStates, q); err != nil {
			return nil, err
		}
	}

	return retStates, nil
}

// getSnapshotSpends returns spend locks from the snapshot-loaded txLocks.
func (dqc *domainQueryContext) getSnapshotSpends(ctx context.Context) (spending []pldtypes.HexBytes, err error) {
	dqc.stateLock.Lock()
	defer dqc.stateLock.Unlock()
	if err = dqc.checkClosed(ctx); err != nil {
		return nil, err
	}

	for _, l := range dqc.txLocks {
		if l.GetType() == prototk.SnapshotStateLock_SPEND {
			stateID, err := pldtypes.ParseHexBytes(ctx, l.GetStateId())
			if err != nil {
				return nil, err
			}
			spending = append(spending, stateID)
		}
	}
	return spending, nil
}

// FindAvailableStates queries available states, merging snapshot creatingStates.
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
		QueryModifier:   dqc.labelPreloadModifier(),
	})
	if err != nil {
		return nil, nil, err
	}
	log.L(ctx).Tracef("FindAvailableStates read %d states from DB", len(states))

	states, err = dqc.mergeSnapshotStatesResult(ctx, schema, states, q, true /* exclude spent */, false)
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
	schema, states, err := dqc.ss.findNullifiers(ctx, dbTX, dqc.domainName, &dqc.contractAddress, schemaID, q, &components.StateQueryOptions{
		StatusQualifier: pldapi.StateStatusAvailable,
		ExcludedIDs:     spending,
		QueryModifier:   dqc.labelPreloadModifier(),
	})
	if err != nil {
		return nil, nil, err
	}

	states, err = dqc.mergeSnapshotStatesResult(ctx, schema, states, q, true /* exclude spent */, true)
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
		QueryModifier:   dqc.labelPreloadModifier(),
	})
	if err == nil {
		var snapshotStates []*components.StateWithLabels
		snapshotStates, err = dqc.findSnapshotMatches(ctx, schema, matches, q, false /* locked states are fine */, false /* nullifiers not required */)
		if err == nil && len(snapshotStates) > 0 {
			matches, err = dqc.mergeAndSortStates(ctx, schema, matches, snapshotStates, q)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	return schema, matches, err
}
