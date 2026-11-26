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
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

func (ss *stateManager) WritePreVerifiedStates(ctx context.Context, dbTX persistence.DBTX, domainName string, states []*components.StateUpsertOutsideContext) ([]*pldapi.State, error) {
	ctx = log.WithComponent(ctx, "statemanager")
	d, err := ss.domainManager.GetDomainByName(ctx, domainName)
	if err != nil {
		return nil, err
	}

	return ss.processInsertStates(ctx, dbTX, d, states)
}

func (ss *stateManager) WriteReceivedStates(ctx context.Context, dbTX persistence.DBTX, domainName string, states []*components.StateUpsertOutsideContext) ([]*pldapi.State, error) {
	ctx = log.WithComponent(ctx, "statemanager")
	if log.IsDebugEnabled() {
		stateIDs := make([]string, len(states))
		for i, s := range states {
			stateIDs[i] = s.ID.String()
		}
		log.L(ctx).Debugf("WriteReceivedStates domain=%s count=%d stateIds=%v", domainName, len(states), stateIDs)
	}

	d, err := ss.domainManager.GetDomainByName(ctx, domainName)
	if err != nil {
		return nil, err
	}

	if d.CustomHashFunction() {
		dStates := make([]*prototk.EndorsableState, len(states))
		for i, s := range states {
			dStates[i] = &prototk.EndorsableState{
				Id:            s.ID.String(),
				SchemaId:      s.SchemaID.String(),
				StateDataJson: string(s.Data),
			}
		}
		ids, err := d.ValidateStateHashes(ctx, dStates)
		if err != nil {
			// Whole batch fails if any state in the batch is invalid
			return nil, err
		}
		for i, s := range states {
			// The domain is responsible for generating any missing IDs
			s.ID = ids[i]
		}
	}

	return ss.processInsertStates(ctx, dbTX, d, states)
}

func (ss *stateManager) WriteNullifiersForReceivedStates(ctx context.Context, dbTX persistence.DBTX, domainName string, nullifiers []*pldapi.StateNullifier) (err error) {
	ctx = log.WithComponent(ctx, "statemanager")
	_, err = ss.domainManager.GetDomainByName(ctx, domainName)
	if err != nil {
		return err
	}

	if len(nullifiers) > 0 {
		err = dbTX.DB(ctx).
			Table("state_nullifiers").
			Clauses(clause.OnConflict{
				DoNothing: true, // immutable
			}).
			Create(newPersistedStateNullifiers(nullifiers)).
			Error
	}

	return err
}

func (ss *stateManager) processInsertStates(ctx context.Context, dbTX persistence.DBTX, d components.Domain, inStates []*components.StateUpsertOutsideContext) (processedStates []*pldapi.State, err error) {

	processedStates = make([]*pldapi.State, len(inStates))
	withLabels := make([]*components.StateWithLabels, len(inStates))
	for i, inState := range inStates {
		schema, err := ss.getSchemaByID(ctx, dbTX, d.Name(), inState.SchemaID, true)
		if err != nil {
			return nil, err
		}

		s, err := schema.ProcessStateWithLabels(ctx, inState.ContractAddress, inState.Data, inState.ID, d.CustomHashFunction())
		if err != nil {
			return nil, err
		}
		processedStates[i] = s.State
		withLabels[i] = s
	}

	// Write them directly
	if err = ss.writeStates(ctx, dbTX, withLabels); err != nil {
		return nil, err
	}

	dbTX.AddPostCommit(ss.txManager.NotifyStatesDBChanged)
	return processedStates, nil
}

func (ss *stateManager) WriteStateBatch(ctx context.Context, dbTX persistence.DBTX, statesWithLabels []*components.StateWithLabels, nullifiers ...*pldapi.StateNullifier) (err error) {
	states := make([]*pldapi.State, len(statesWithLabels))
	for i, s := range statesWithLabels {
		states[i] = s.State
	}
	log.L(ctx).Debugf("Writing state batch states=%d nullifiers=%d", len(states), len(nullifiers))
	if log.IsTraceEnabled() {
		for _, s := range states {
			log.L(ctx).Tracef("Writing state for contract %s, data=%s, domain=%s, created=%s", s.ContractAddress, s.Data, s.DomainName, s.Created)
		}
	}

	if len(states) > 0 {
		err = ss.writeStates(ctx, dbTX, statesWithLabels)
	}
	if err == nil && len(nullifiers) > 0 {
		err = dbTX.DB(ctx).
			Table("state_nullifiers").
			Clauses(clause.OnConflict{
				DoNothing: true, // immutable
			}).
			Create(newPersistedStateNullifiers(nullifiers)).
			Error
	}
	return err
}

func (ss *stateManager) writeStates(ctx context.Context, dbTX persistence.DBTX, statesWithLabels []*components.StateWithLabels) (err error) {
	states := make([]*pldapi.State, len(statesWithLabels))
	persisted := make([]*persistedState, len(statesWithLabels))
	for i, s := range statesWithLabels {
		states[i] = s.State
		persisted[i] = newPersistedState(s.State)
	}

	if len(states) > 0 {
		err = dbTX.DB(ctx).
			Table("states").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "domain_name"}, {Name: "id"}},
				DoNothing: true, // immutable
			}).
			Create(persisted).
			Error
		// The insert stamps Created, which callers read from the states they passed in.
		for i, ps := range persisted {
			states[i].Created = ps.Base.Created
		}
	}

	// Label rows denormalize the state's identity columns, and Created is only stamped onto the
	// state by the insert above, so they are filled in after it. The availability flags are left
	// false here: setStateAvailableFromSpendConfirmRecords below reconciles them on all tables,
	// under the same lock that keeps them consistent when confirm/spend records arrive first.
	var labels []*persistedStateLabel
	var int64Labels []*persistedStateInt64Label
	for _, s := range statesWithLabels {
		for _, l := range s.Labels {
			labels = append(labels, &persistedStateLabel{
				DomainName: s.DomainName, State: s.ID, Label: l.Label, Value: l.Value,
				Schema: s.Schema, ContractAddress: s.ContractAddress, Created: s.Created,
			})
		}
		for _, l := range s.Int64Labels {
			int64Labels = append(int64Labels, &persistedStateInt64Label{
				DomainName: s.DomainName, State: s.ID, Label: l.Label, Value: l.Value,
				Schema: s.Schema, ContractAddress: s.ContractAddress, Created: s.Created,
			})
		}
	}
	if err == nil && len(labels) > 0 {
		err = dbTX.DB(ctx).
			Table("state_labels").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "domain_name"}, {Name: "state"}, {Name: "label"}},
				DoNothing: true, // immutable
			}).
			Create(labels).
			Error
	}
	if err == nil && len(int64Labels) > 0 {
		err = dbTX.DB(ctx).
			Table("state_int64_labels").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "domain_name"}, {Name: "state"}, {Name: "label"}},
				DoNothing: true, // immutable
			}).
			Create(int64Labels).
			Error
	}

	// Reconcile availability flags for the just-written states, then update the
	// completion index. A confirm/spend record may have been written before the state row
	// existed; setStateAvailableFromSpendConfirmRecords sets the flags now from the
	// authoritative record tables.
	if err == nil && len(states) > 0 {
		err = ss.setStateAvailableFromSpendConfirmRecords(ctx, dbTX, states)
		if err == nil {
			arrivedIDs := make([]pldtypes.HexBytes, len(states))
			for i, s := range states {
				arrivedIDs[i] = s.ID
			}
			err = ss.updatePendingPrivateStateData(ctx, dbTX, arrivedIDs)
		}
	}
	return err
}

func (ss *stateManager) getStateIDsMissingPrivateData(ctx context.Context, dbTX persistence.DBTX, domainName string, stateIDs []pldtypes.HexBytes) ([]pldtypes.HexBytes, error) {
	if len(stateIDs) == 0 {
		return nil, nil
	}
	var found []pldtypes.HexBytes
	if err := dbTX.DB(ctx).Table("states").
		Where("domain_name = ?", domainName).
		Where("id IN ?", stateIDs).
		Pluck("id", &found).Error; err != nil {
		return nil, err
	}
	foundSet := make(map[string]bool, len(found))
	for _, id := range found {
		foundSet[id.String()] = true
	}
	var missing []pldtypes.HexBytes
	for _, id := range stateIDs {
		if !foundSet[id.String()] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		log.L(ctx).Debugf("states missing private data (domain=%s): %v", domainName, missing)
	}
	return missing, nil
}

func (ss *stateManager) GetStatesByID(ctx context.Context, dbTX persistence.DBTX, domainName string, contractAddress *pldtypes.EthAddress, stateIDs []pldtypes.HexBytes, failNotFound bool) ([]*pldapi.State, error) {
	ctx = log.WithComponent(ctx, "statemanager")
	q := dbTX.DB(ctx).Table("states")
	var persisted []*persistedState
	q = q.
		Where("domain_name = ?", domainName).
		Where("id IN ?", stateIDs)
	if contractAddress != nil {
		q = q.Where("contract_address = ?", contractAddress)
	}
	err := q.
		Find(&persisted).
		Error
	if err == nil && len(persisted) != len(stateIDs) && failNotFound {
		return nil, i18n.NewError(ctx, msgs.MsgStateNotFound, stateIDs)
	}
	return mapStatesToAPI(persisted), err
}

// Built in fields all start with "." as that prevents them
// clashing with variable names in ABI structs ($ and _ are valid leading chars there)
var baseStateFields = map[string]filters.FieldResolver{
	".id":             filters.HexBytesField(`"states"."id"`),
	".created":        filters.TimestampField(`"states"."created"`),
	"contractAddress": filters.HexBytesField(`"states"."contract_address"`),
}

func addStateBaseLabels(labelValues filters.PassthroughValueSet, id pldtypes.HexBytes, createdAt pldtypes.Timestamp) filters.PassthroughValueSet {
	labelValues[".id"] = id.HexString()
	labelValues[".created"] = int64(createdAt)
	return labelValues
}

// sortFieldBaseName reduces a sort instruction ("[-]field[ asc|desc]") to the field name it orders on.
func sortFieldBaseName(sortInstruction string) string {
	name, _ := strings.CutPrefix(strings.SplitN(sortInstruction, " ", 2)[0], "-")
	return name
}

type trackingLabelSet struct {
	labels map[string]*schemaLabelInfo
	used   map[string]*schemaLabelInfo
}

func (ft trackingLabelSet) ResolverFor(fieldName string) filters.FieldResolver {
	baseField := baseStateFields[fieldName]
	if baseField != nil {
		return baseField
	}
	f := ft.labels[fieldName]
	if f != nil {
		ft.used[fieldName] = f
		return f.resolver
	}
	return nil
}

func (ss *stateManager) labelSetFor(schema components.Schema) *trackingLabelSet {
	tls := trackingLabelSet{labels: make(map[string]*schemaLabelInfo), used: make(map[string]*schemaLabelInfo)}
	for _, fi := range schema.(labelInfoAccess).labelInfo() {
		tls.labels[fi.label] = fi
	}
	return &tls
}

type stateQueryScope struct {
	// availableOnly scopes to available states with predicates that hold on a label row as well as a
	// states row, so the query can be driven from the label tables' available-state indexes.
	availableOnly bool
	apply         func(q *gorm.DB, source string) *gorm.DB
}

// statusScope scopes a states query to a status qualifier and excludes the given state IDs. The
// confirmed qualifier is served from the maintained confirmed/spent flags and their partial indexes,
// so it needs neither the Confirmed/Spent joins nor whereClauseForQual. Every other qualifier
// expresses status via those joins. With confirmedBefore, the confirmed qualifier takes only the
// states confirmed before that block.
func statusScope(ctx context.Context, dbTX persistence.DBTX, status pldapi.StateStatusQualifier, excludedIDs []pldtypes.HexBytes, confirmedBefore *int64) stateQueryScope {
	apply := func(q *gorm.DB, source string) *gorm.DB {
		var whereClause *gorm.DB
		if status == pldapi.StateStatusConfirmed {
			// source is the label table when the query is driven from its available-state index.
			whereClause = dbTX.DB(ctx).Where(fmt.Sprintf(`%[1]s."confirmed" AND NOT %[1]s."spent"`, source))
			if confirmedBefore != nil {
				whereClause = whereClause.Where(fmt.Sprintf(confirmedBeforeClause, source), *confirmedBefore)
			}
		} else {
			whereClause = whereClauseForQual(dbTX.DB(ctx), status, "Spent")
			q = q.Joins("Confirmed", dbTX.DB(ctx).Select("transaction")).
				Joins("Spent", dbTX.DB(ctx).Select("transaction"))
		}
		if len(excludedIDs) > 0 {
			q = q.Not(`"states"."id" IN(?)`, excludedIDs)
		}
		return q.Where(whereClause)
	}
	return stateQueryScope{
		availableOnly: status == pldapi.StateStatusConfirmed,
		apply:         apply,
	}
}

// A state confirmed with the zero transaction ID has no confirmed block because no Paladin transaction created it. No remote
// view will ever supply it, so it must always be queried from the database
const confirmedBeforeClause = `(%[1]s."confirmed_block" IS NULL OR %[1]s."confirmed_block" < ?)`

// findStates reads states from the local DB, scoped by the given status qualifier and query,
// excluding any of the given state IDs and, with confirmedBefore, any confirmed at or after that block.
// confirmedBefore is an optimisation added in v1.2.0 to limit duplication with the remote view.
// If an assemble request is received from coordinator running an older version, the originator must query
// all available states from the statestore and then deduplicate with the remote view.
func (ss *stateManager) findStates(
	ctx context.Context,
	dbTX persistence.DBTX,
	domainName string,
	contractAddress *pldtypes.EthAddress,
	schemaID pldtypes.Bytes32,
	jq *query.QueryJSON,
	status pldapi.StateStatusQualifier,
	excludedIDs []pldtypes.HexBytes,
	confirmedBefore *int64,
) (components.Schema, []*pldapi.State, error) {
	return ss.findStatesCommon(ctx, dbTX, domainName, contractAddress, schemaID, jq, statusScope(ctx, dbTX, status, excludedIDs, confirmedBefore))
}

// findNullifierBackedStates reads states that carry a nullifier, dropping those that do not. Status is
// expressed through the Confirmed join and the nullifier's own spend join, since the chain records the
// spend against the nullifier rather than against the state it consumes.
func (ss *stateManager) findNullifierBackedStates(
	ctx context.Context,
	dbTX persistence.DBTX,
	domainName string,
	contractAddress *pldtypes.EthAddress,
	schemaID pldtypes.Bytes32,
	jq *query.QueryJSON,
	status pldapi.StateStatusQualifier,
	excludedIDs []pldtypes.HexBytes,
) (components.Schema, []*pldapi.State, error) {
	whereClause := whereClauseForQual(dbTX.DB(ctx), status, "Nullifier__Spent")
	return ss.findStatesCommon(ctx, dbTX, domainName, contractAddress, schemaID, jq, stateQueryScope{apply: func(q *gorm.DB, _ string) *gorm.DB {
		hasNullifier := dbTX.DB(ctx).Where(`"Nullifier"."id" IS NOT NULL`)

		q = q.Joins("Confirmed", dbTX.DB(ctx).Select("transaction")).
			Joins("Nullifier", dbTX.DB(ctx).Select(`"Nullifier"."id"`)).
			Joins("Nullifier.Spent", dbTX.DB(ctx).Select("transaction")).
			Where(hasNullifier)

		if len(excludedIDs) > 0 {
			q = q.Not(`"states"."id" IN(?)`, excludedIDs)
		}
		return q.Where(whereClause)
	}})
}

// findStatesCommon runs a states query. The label tables have extra indexes to optimise the
// available state queries made during assembly, and availableOnly queries are driven from them.
func (ss *stateManager) findStatesCommon(
	ctx context.Context,
	dbTX persistence.DBTX,
	domainName string,
	contractAddress *pldtypes.EthAddress,
	schemaID pldtypes.Bytes32,
	jq *query.QueryJSON,
	scope stateQueryScope,
) (schema components.Schema, s []*pldapi.State, err error) {
	if len(jq.Sort) == 0 {
		jq.Sort = []string{".created"}
	}

	schema, err = ss.getSchemaByID(ctx, dbTX, domainName, schemaID, true)
	if err != nil {
		return nil, nil, err
	}

	// The label indexes of available states are keyed by contract, so only a contract-scoped query can seek them.
	var driver *schemaLabelInfo
	if scope.availableOnly && contractAddress != nil {
		driver = labelIndexDriver(jq, ss.labelSetFor(schema).labels)
	}
	q := ss.buildStatesQuery(ctx, dbTX.DB(ctx), domainName, contractAddress, schema, jq, driver, scope.apply)
	if q.Error != nil {
		return nil, nil, q.Error
	}

	var persisted []*persistedState
	dbStart := time.Now()
	err = q.Find(&persisted).Error
	ss.metrics.ObserveStateQueryDB(domainName, time.Since(dbStart))
	if err != nil {
		return nil, nil, err
	}
	return schema, mapStatesToAPI(persisted), nil
}

func (ss *stateManager) buildStatesQuery(
	ctx context.Context,
	db *gorm.DB,
	domainName string,
	contractAddress *pldtypes.EthAddress,
	schema components.Schema,
	jq *query.QueryJSON,
	driver *schemaLabelInfo,
	modifyQuery func(q *gorm.DB, source string) *gorm.DB,
) *gorm.DB {
	tracker := ss.labelSetFor(schema)
	source, idColumn := `"states"`, `"id"`
	var fieldSet filters.FieldSet = tracker
	if driver != nil {
		source, idColumn = driver.virtualColumn, `"state"`
		db = db.Table(fmt.Sprintf(`%s AS %s`, labelTable(driver), source)).
			Select(`"states".*`).
			Joins(fmt.Sprintf(`INNER JOIN states ON "states"."domain_name" = %[1]s."domain_name" AND "states"."id" = %[1]s."state"`, source))
		fieldSet = labelRowFieldSet{trackingLabelSet: tracker, row: source}
	} else {
		db = db.Table("states")
	}

	// Build the query
	q := filters.BuildGORM(ctx, jq, db, fieldSet)
	if q.Error != nil {
		return q
	}

	// Add joins only for the fields actually used in the query
	for _, fi := range tracker.used {
		if fi == driver {
			continue
		}
		// Include domain_name so the join matches the state_labels PK/FK and Postgres can use (domain_name, label, value) indexes.
		q = q.Joins(fmt.Sprintf(`INNER JOIN %[1]s AS %[2]s ON %[2]s.state = %[3]s.%[4]s AND %[2]s.domain_name = %[3]s."domain_name" AND %[2]s.label = ?`, labelTable(fi), fi.virtualColumn, source, idColumn), fi.label)
	}

	if driver != nil {
		q = q.Where(source+`."label" = ?`, driver.label)
	}
	q = q.Where(source+`."domain_name" = ?`, domainName).
		Where(source+`."schema" = ?`, schema.Persisted().ID)
	if contractAddress != nil {
		q = q.Where(source+`."contract_address" = ?`, contractAddress)
	}
	return modifyQuery(q, source)
}

// cacheGetValidatedStateWithLabels returns a caller-owned shallow copy of a cache entry, with labels.
// The shallow copy means that the caller may mutate Created or do a wholesale replacement of labels,
// without modifying the cache entry.
//
// All other fields must still be treated as read only. This is fragile as it relies on an
// unenforced and difficult to document contract, but taking a deep copy of the state would be expensive,
// and modifications to any of the remaining fields in the state or partial modifications to its labels are
// destroy the integrity/self consistency of the state, making it effectively unusable, so the risk of
// future changes not respecting this contract is low enough to justify the performance benefits of not
// making a deep copy.
func (ss *stateManager) cacheGetValidatedStateWithLabels(cacheKey string) (*components.StateWithLabels, bool) {
	cached, ok := ss.validatedStateCache.Get(cacheKey)
	if !ok {
		return nil, false
	}
	stateCopy := *cached.State
	return &components.StateWithLabels{
		State:       &stateCopy,
		Labels:      cached.Labels,
		Int64Labels: cached.Int64Labels,
		LabelValues: cached.LabelValues,
	}, true
}

// cacheGetValidatedState returns just the validated content (id, schema, normalized data) of
// a full cache entry. The same unenforced contract about all fields other than Created being
// immutable applies as for cacheGetValidatedStateWithLabels.
func (ss *stateManager) cacheGetValidatedState(cacheKey string) (*pldapi.State, bool) {
	cached, ok := ss.validatedStateCache.Get(cacheKey)
	if !ok {
		return nil, false
	}
	stateCopy := *cached.State
	return &stateCopy, true
}

// validatedCacheParams parses a proto state's id/schema and derives its validatedStateCache key. An
// empty cacheKey means the state is not addressable in the cache: only content-addressed states whose
// claimed ID is hash-verified against content in ProcessState may be cached, and customHashFunction
// states pre-verify their own hash so are never cached.
func (ss *stateManager) validatedCacheParams(ctx context.Context, domainName string, contractAddress pldtypes.EthAddress, customHashFunction bool, es *prototk.EndorsableState) (schemaID pldtypes.Bytes32, stateID pldtypes.HexBytes, cacheKey string, err error) {
	if schemaID, err = pldtypes.ParseBytes32Ctx(ctx, es.GetSchemaId()); err != nil {
		return
	}
	if idStr := es.GetId(); idStr != "" {
		if stateID, err = pldtypes.ParseHexBytes(ctx, idStr); err != nil {
			return
		}
	}
	if !customHashFunction && stateID != nil {
		cacheKey = validatedStateCacheKey(domainName, contractAddress, stateID)
	}
	return
}

// validateStateWithLabels returns the validated, full StateWithLabels form of a proto-native state,
// reading through validatedStateCache. It is the only path that builds complete StateWithLabel types,
// so it is the only path that seeds the cache.
func (ss *stateManager) validateStateWithLabels(ctx context.Context, domainName string, contractAddress pldtypes.EthAddress, customHashFunction bool, dbTX persistence.DBTX, es *prototk.EndorsableState) (*components.StateWithLabels, error) {
	schemaID, stateID, cacheKey, err := ss.validatedCacheParams(ctx, domainName, contractAddress, customHashFunction, es)
	if err != nil {
		return nil, err
	}
	if cacheKey != "" {
		if cached, ok := ss.cacheGetValidatedStateWithLabels(cacheKey); ok {
			return cached, nil
		}
	}
	schema, err := ss.getSchemaByID(ctx, dbTX, domainName, schemaID, true)
	if err != nil {
		return nil, err
	}
	vs, err := schema.ProcessStateWithLabels(ctx, &contractAddress, pldtypes.RawJSON(es.GetStateDataJson()), stateID, customHashFunction)
	if err != nil {
		return nil, err
	}
	if cacheKey != "" {
		// ProcessStateWithLabels does not set Created, so the shared cache never holds Created timestamp.
		// Each caller stamps the created its context needs on the copy it receives.
		ss.validatedStateCache.Set(cacheKey, vs)
		stateCopy := *vs.State
		return &components.StateWithLabels{
			State:       &stateCopy,
			Labels:      vs.Labels,
			Int64Labels: vs.Int64Labels,
			LabelValues: vs.LabelValues,
		}, nil
	}
	return vs, nil
}

func validatedStateCacheKey(domainName string, contractAddress pldtypes.EthAddress, stateID pldtypes.HexBytes) string {
	// Build "domain:0x<address>:0x<stateID>" into a single buffer, hex-encoding the
	// address and state ID directly to avoid the intermediate String() allocations.
	buf := make([]byte, 0, len(domainName)+6+len(contractAddress)*2+len(stateID)*2)
	buf = append(buf, domainName...)
	buf = append(buf, ':', '0', 'x')
	buf = hex.AppendEncode(buf, contractAddress[:])
	buf = append(buf, ':', '0', 'x')
	buf = hex.AppendEncode(buf, stateID)
	return string(buf)
}

// ValidateStates validates and normalizes state data against the state's schema, and computes the state ID.
func (ss *stateManager) ValidateStates(ctx context.Context, dbTX persistence.DBTX, domain components.Domain, contractAddress pldtypes.EthAddress, states ...*prototk.EndorsableState) ([]*pldapi.State, error) {
	domainName := domain.Name()
	customHashFunction := domain.CustomHashFunction()
	validated := make([]*pldapi.State, len(states))
	for i, es := range states {
		schemaID, stateID, cacheKey, err := ss.validatedCacheParams(ctx, domainName, contractAddress, customHashFunction, es)
		if err != nil {
			return nil, err
		}
		if cacheKey != "" {
			if cached, ok := ss.cacheGetValidatedState(cacheKey); ok {
				validated[i] = cached
				continue
			}
		}
		schema, err := ss.getSchemaByID(ctx, dbTX, domainName, schemaID, true)
		if err != nil {
			return nil, err
		}
		if validated[i], err = schema.ProcessState(ctx, &contractAddress, pldtypes.RawJSON(es.GetStateDataJson()), stateID, customHashFunction); err != nil {
			return nil, err
		}
	}
	return validated, nil
}

// ValidateStatesWithLabels is ValidateStates, additionally extracting label values.
func (ss *stateManager) ValidateStatesWithLabels(ctx context.Context, dbTX persistence.DBTX, domain components.Domain, contractAddress pldtypes.EthAddress, states ...*prototk.EndorsableState) ([]*components.StateWithLabels, error) {
	withLabels := make([]*components.StateWithLabels, len(states))
	for i, es := range states {
		vs, err := ss.validateStateWithLabels(ctx, domain.Name(), contractAddress, domain.CustomHashFunction(), dbTX, es)
		if err != nil {
			return nil, err
		}
		withLabels[i] = vs
	}
	return withLabels, nil
}
