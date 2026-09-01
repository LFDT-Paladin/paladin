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
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
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
// May carry a remote view (spend exclusions + on-demand state queries) for FindAvailableStates queries.
type domainQueryContext struct {
	ss                 *stateManager
	domainName         string
	customHashFunction bool
	contractAddress    pldtypes.EthAddress
	stateLock          sync.Mutex
	id                 uuid.UUID
	closed             bool
	remoteStateView    components.RemoteStateView
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
	}
	ss.domainContexts[id] = dqc
	return dqc
}

// NewDomainQueryContextWithRemoteView creates a domain query context whose FindAvailableStates
// queries merge a remote view with the local DB. This view is fixed for the life of the
// context.
func (ss *stateManager) NewDomainQueryContextWithRemoteView(ctx context.Context, domain components.Domain, contractAddress pldtypes.EthAddress, remoteStateView components.RemoteStateView) components.DomainQueryContext {
	ctx = createLogContext(ctx, domain.Name(), contractAddress, nil)

	id := uuid.New()
	log.L(ctx).Debugf("Assembly domain context %s for domain %s contract %s created", id, domain.Name(), contractAddress)

	ss.domainContextLock.Lock()
	defer ss.domainContextLock.Unlock()

	dqc := &domainQueryContext{
		ss:                 ss,
		domainName:         domain.Name(),
		customHashFunction: domain.CustomHashFunction(),
		contractAddress:    contractAddress,
		id:                 id,
		remoteStateView:    remoteStateView,
	}
	ss.domainContexts[id] = dqc
	return dqc
}

// getSpentStateIDs returns the remote view's spend exclusion set. Returns nil for local-only contexts.
func (dqc *domainQueryContext) getSpentStateIDs(ctx context.Context) ([]pldtypes.HexBytes, error) {
	if dqc.remoteStateView == nil {
		return nil, nil
	}
	spendStateIDs, err := dqc.remoteStateView.GetSpentStateIDs(ctx)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateViewSpentIDsFailed)
	}
	log.L(ctx).Debugf("Domain context %s applying %d spend exclusions", dqc.id, len(spendStateIDs))
	return spendStateIDs, nil
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

// ensureOpen fails if the context has been closed.
func (dqc *domainQueryContext) ensureOpen(ctx context.Context) error {
	dqc.stateLock.Lock()
	defer dqc.stateLock.Unlock()
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

// labelPreloadModifier returns a query modifier that preloads the persisted label rows, but only
// when this context has a remote view — the sole case where mergeSortLimit runs against DB
// states and needs their label values to sort them alongside view-returned states. This is
// an optimization: RecoverLabels falls back to re-parsing the state data when the rows are absent,
// so returning nil on the common path (no extra DB round-trips) stays correct.
//
// TODO: Under sustained load this preload fires on essentially every query and its cost
// (two extra SELECTs, on state_labels and state_int64labels) is paid per query. A further
// optimization is possible: findStatesCommon already INNER-JOINs the label tables for the fields
// referenced by the query's filter/sort, and the recovered values are consumed only by the
// in-memory sort in mergeSortLimit (which needs only the sort-key labels). Selecting those
// already-joined columns into the result would supply the sort values with zero extra round-trips
// and no re-parse, superseding both this preload and the RecoverLabels fallback — at the cost of a
// custom projection/scan, since GORM will not map arbitrary selected columns onto pldapi.State.
func (dqc *domainQueryContext) labelPreloadModifier() func(persistence.DBTX, *gorm.DB) *gorm.DB {
	// TODO: this isn't the same as the previous creating refs check- it means we're going to
	// always do this on assembly because we can't see if we're going to need to merge or not.
	// The optimisation which addresses the TODO is coming imminently
	if dqc.remoteStateView == nil {
		return nil
	}
	return func(_ persistence.DBTX, q *gorm.DB) *gorm.DB {
		return q.Preload("Labels").Preload("Int64Labels")
	}
}

// fetchRemoteViewStates sends the pre-marshaled query to the remote in-memory view and returns the
// raw matches. Network only — no DB access — so it can run concurrently with the local DB read.
// Runs unlocked — the remote query blocks on a network round-trip, and the dqc
// fields read here are immutable after construction.
func (dqc *domainQueryContext) fetchRemoteViewStates(ctx context.Context, schemaID pldtypes.Bytes32, queryJSON string) ([]*prototk.QueriedState, error) {
	queried, err := dqc.remoteStateView.QueryAvailableStates(ctx, schemaID.String(), queryJSON)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateViewQueryFailed)
	}
	log.L(ctx).Debugf("fetchRemoteViewStates: remote view returned %d states", len(queried))
	return queried, nil
}

// startRemoteViewFetch launches the remote view query (when a remote view is attached) concurrently
// with the caller's local DB read, and returns a wait function that blocks until the fetch completes
// and returns its results. All failures — including a query that cannot be re-marshaled for the
// round-trip — are reported through the wait function, so callers have a single error path. With no
// remote view attached the wait function is an immediate no-op.
func (dqc *domainQueryContext) startRemoteViewFetch(ctx context.Context, schemaID pldtypes.Bytes32, q *query.QueryJSON) func() ([]*prototk.QueriedState, error) {
	if dqc.remoteStateView == nil {
		return func() ([]*prototk.QueriedState, error) { return nil, nil }
	}
	queryJSON, err := json.Marshal(q)
	if err != nil {
		return func() ([]*prototk.QueriedState, error) { return nil, err }
	}
	var remoteStates []*prototk.QueriedState
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var fetchErr error
		remoteStates, fetchErr = dqc.fetchRemoteViewStates(gctx, schemaID, string(queryJSON))
		return fetchErr
	})
	return func() ([]*prototk.QueriedState, error) {
		if err := g.Wait(); err != nil {
			return nil, err
		}
		return remoteStates, nil
	}
}

// mergeRemoteViewStates merges the view-returned matches with the DB results. The remote view
// answers with full state data; because the source is not implicitely trusted, every returned state is
// validated. Runs unlocked — the dqc fields read here are immutable after construction.
func (dqc *domainQueryContext) mergeRemoteViewStates(ctx context.Context, dbTX persistence.DBTX, schema components.Schema, dbStates []*pldapi.State, remoteStates []*prototk.QueriedState, q *query.QueryJSON) ([]*pldapi.State, error) {
	if len(remoteStates) == 0 {
		return dbStates, nil
	}

	labelSet := dqc.ss.labelSetFor(schema)
	validated, err := dqc.validateQueriedStates(ctx, dbTX, schema, remoteStates, q, dbStates, labelSet)
	if err != nil {
		return nil, err
	}
	if len(validated) == 0 {
		return dbStates, nil
	}

	return dqc.mergeSortLimit(ctx, schema, dbStates, validated, q, labelSet)
}

// queriedStateEntry pairs a view-returned state with its hex-parsed ID, so the ID is
// parsed once across the dedup / hash-verification / cache-lookup passes.
type queriedStateEntry struct {
	qs *prototk.QueriedState
	id pldtypes.HexBytes
}

// validateQueriedStates validates view-returned states:
//   - schema must be exactly the queried schema;
//   - states already present in the DB results are dropped (the local, already-trusted copy wins);
//   - the id is a hash of the state's content, so recomputing the hash over the received
//     bytes and requiring it to equal the id proves the sender did not alter the data
//     (custom-hash domains verify the whole batch through the domain). A state whose content this
//     node previously validated is served from validatedStateCache.
//   - created is stamped from the response — the only value taken from the sender, as it drives
//     ordering and is not derivable from content.
//   - the query is re-evaluated against the recomputed labels. Matching ran on the sender's
//     copy of the labels, so a returned state whose validated labels do not satisfy the query
//     means the selection cannot be trusted and the whole operation fails.
//
// Runs unlocked — all dqc fields read here are immutable after construction.
func (dqc *domainQueryContext) validateQueriedStates(ctx context.Context, dbTX persistence.DBTX, schema components.Schema, queried []*prototk.QueriedState, q *query.QueryJSON, dbStates []*pldapi.State, labelSet *trackingLabelSet) ([]*components.StateWithLabels, error) {
	schemaID := schema.ID()

	dbStateIDs := make(map[string]struct{}, len(dbStates))
	for _, dbState := range dbStates {
		dbStateIDs[dbState.ID.String()] = struct{}{}
	}

	entries := make([]*queriedStateEntry, 0, len(queried))
	for _, qs := range queried {
		es := qs.GetState()
		esSchemaID, err := pldtypes.ParseBytes32Ctx(ctx, es.GetSchemaId())
		if err != nil {
			return nil, err
		}
		if !esSchemaID.Equals(&schemaID) {
			return nil, i18n.NewError(ctx, msgs.MsgStateQueriedStateSchemaMismatch, es.GetId(), es.GetSchemaId(), schemaID)
		}
		claimedID, err := pldtypes.ParseHexBytes(ctx, es.GetId())
		if err != nil {
			return nil, err
		}
		if _, dup := dbStateIDs[claimedID.String()]; dup {
			log.L(ctx).Tracef("Dropping queried state %s already present in DB results", claimedID)
			continue
		}
		entries = append(entries, &queriedStateEntry{qs: qs, id: claimedID})
	}
	if len(entries) == 0 {
		return nil, nil
	}

	if dqc.customHashFunction {
		d, err := dqc.ss.domainManager.GetDomainByName(ctx, dqc.domainName)
		if err != nil {
			return nil, err
		}
		esList := make([]*prototk.EndorsableState, len(entries))
		for i, e := range entries {
			esList[i] = e.qs.GetState()
		}
		verifiedIDs, err := d.ValidateStateHashes(ctx, esList)
		if err != nil {
			return nil, err
		}
		for i, e := range entries {
			if !e.id.Equals(verifiedIDs[i]) {
				return nil, i18n.NewError(ctx, msgs.MsgStateHashMismatch, e.id, verifiedIDs[i])
			}
		}
	}

	validated := make([]*components.StateWithLabels, 0, len(entries))
	for _, e := range entries {
		vs, err := dqc.ss.validateStateWithLabels(ctx, dqc.domainName, dqc.contractAddress, dqc.customHashFunction, dbTX, e.qs.GetState())
		if err != nil {
			return nil, err
		}

		// created comes from the response, not the state content, so we take it from there and add
		// the ".created" label that the content-derived labels from ProcessState do not include.
		vs.Created = pldtypes.Timestamp(e.qs.GetCreated())

		// Build the label set into a fresh map rather than mutating vs.LabelValues: on a cache hit
		// that map is shared with the cached entry.
		existing, _ := vs.LabelValues.(filters.PassthroughValueSet)
		labelValues := make(filters.PassthroughValueSet, len(existing)+1)
		maps.Copy(labelValues, existing)
		vs.LabelValues = addStateBaseLabels(labelValues, vs.ID, vs.Created)

		match, err := filters.EvalQuery(ctx, q, labelSet, vs.LabelValues)
		if err != nil {
			return nil, err
		}
		if !match {
			return nil, i18n.NewError(ctx, msgs.MsgStateQueriedStateNoMatch, vs.ID)
		}
		validated = append(validated, vs)
	}
	return validated, nil
}

// mergeSortLimit merges the DB states with the validated view-returned states, sorts the combined
// list on the query's sort instructions, and applies the query limit. Runs unlocked — inputs are
// owned by the caller.
func (dqc *domainQueryContext) mergeSortLimit(ctx context.Context, schema components.Schema, dbStates []*pldapi.State, remoteViewStates []*components.StateWithLabels, q *query.QueryJSON, labelSet *trackingLabelSet) ([]*pldapi.State, error) {
	fullList := make([]*components.StateWithLabels, 0, len(dbStates)+len(remoteViewStates))
	for _, s := range dbStates {
		withLabels, err := schema.RecoverLabels(ctx, s)
		if err != nil {
			return nil, err
		}
		fullList = append(fullList, withLabels)
	}
	fullList = append(fullList, remoteViewStates...)

	if err := filters.SortValueSetInPlace(ctx, labelSet, fullList, q.Sort...); err != nil {
		return nil, err
	}

	if q.Limit != nil && len(fullList) > *q.Limit {
		fullList = fullList[:*q.Limit]
	}
	retList := make([]*pldapi.State, len(fullList))
	for i, e := range fullList {
		retList[i] = e.State
	}
	return retList, nil
}

// FindAvailableStates queries available states, merging the remote view's matches.
func (dqc *domainQueryContext) FindAvailableStates(ctx context.Context, dbTX persistence.DBTX, schemaID pldtypes.Bytes32, q *query.QueryJSON) (components.Schema, []*pldapi.State, error) {
	ctx = createLogContext(ctx, dqc.domainName, dqc.contractAddress, &schemaID)
	log.L(ctx).Debugf("FindAvailableStates query=%s", q)

	if err := dqc.ensureOpen(ctx); err != nil {
		return nil, nil, err
	}

	spentStateIDs, err := dqc.getSpentStateIDs(ctx)
	if err != nil {
		return nil, nil, err
	}
	if log.IsTraceEnabled() {
		log.L(ctx).Tracef("Remote view spend exclusions: %d", len(spentStateIDs))
		for _, s := range spentStateIDs {
			log.L(ctx).Tracef("Remote view spend exclusion: %s", s.String())
		}
	}

	waitRemote := dqc.startRemoteViewFetch(ctx, schemaID, q)
	schema, dbStates, dbErr := dqc.ss.findStates(ctx, dbTX, dqc.domainName, &dqc.contractAddress, schemaID, q, &components.StateQueryOptions{
		StatusQualifier: pldapi.StateStatusAvailable,
		ExcludedIDs:     spentStateIDs,
		QueryModifier:   dqc.labelPreloadModifier(),
	})
	remoteStates, fetchErr := waitRemote()
	if fetchErr != nil {
		return nil, nil, fetchErr
	}
	if dbErr != nil {
		return nil, nil, dbErr
	}
	log.L(ctx).Tracef("FindAvailableStates read %d states from DB", len(dbStates))

	dbStates, err = dqc.mergeRemoteViewStates(ctx, dbTX, schema, dbStates, remoteStates, q)
	if log.IsTraceEnabled() {
		for _, s := range dbStates {
			log.L(ctx).Tracef("returning available state %s", s.ID)
		}
	}
	log.L(ctx).Debugf("FindAvailableStates read+merged %d states: %s", len(dbStates), logStateSummary(dbStates))

	return schema, dbStates, err
}

// FindAvailableNullifiers queries available nullifier-based states. The remote in-memory view
// carries no nullifiers, so nullifier queries answer from the DB only — the view's exclusion
// set still applies.
func (dqc *domainQueryContext) FindAvailableNullifiers(ctx context.Context, dbTX persistence.DBTX, schemaID pldtypes.Bytes32, q *query.QueryJSON) (components.Schema, []*pldapi.State, error) {
	ctx = createLogContext(ctx, dqc.domainName, dqc.contractAddress, &schemaID)
	log.L(ctx).Debugf("FindAvailableNullifiers query=%s", q)

	if err := dqc.ensureOpen(ctx); err != nil {
		return nil, nil, err
	}

	spentStateIDs, err := dqc.getSpentStateIDs(ctx)
	if err != nil {
		return nil, nil, err
	}
	return dqc.ss.findNullifiers(ctx, dbTX, dqc.domainName, &dqc.contractAddress, schemaID, q, &components.StateQueryOptions{
		StatusQualifier: pldapi.StateStatusAvailable,
		ExcludedIDs:     spentStateIDs,
	})
}

// GetStatesByID retrieves states by ID regardless of confirmation/spend status,
// including unconfirmed states served by the remote view.
func (dqc *domainQueryContext) GetStatesByID(ctx context.Context, dbTX persistence.DBTX, schemaID pldtypes.Bytes32, ids []string) (components.Schema, []*pldapi.State, error) {
	ctx = createLogContext(ctx, dqc.domainName, dqc.contractAddress, &schemaID)
	idsAny := make([]any, len(ids))
	for i, id := range ids {
		idsAny[i] = id
	}
	q := query.NewQueryBuilder().In(".id", idsAny).Sort(".created").Query()

	if err := dqc.ensureOpen(ctx); err != nil {
		return nil, nil, err
	}

	waitRemote := dqc.startRemoteViewFetch(ctx, schemaID, q)
	schema, dbStates, dbErr := dqc.ss.findStates(ctx, dbTX, dqc.domainName, &dqc.contractAddress, schemaID, q, &components.StateQueryOptions{
		StatusQualifier: pldapi.StateStatusAll,
		QueryModifier:   dqc.labelPreloadModifier(),
	})
	remoteStates, fetchErr := waitRemote()
	if fetchErr != nil {
		return nil, nil, fetchErr
	}
	if dbErr != nil {
		return nil, nil, dbErr
	}
	matches, err := dqc.mergeRemoteViewStates(ctx, dbTX, schema, dbStates, remoteStates, q)
	if err != nil {
		return nil, nil, err
	}
	return schema, matches, nil
}
