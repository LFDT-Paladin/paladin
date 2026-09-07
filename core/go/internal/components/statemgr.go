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

package components

import (
	"context"

	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"gorm.io/gorm"
)

type StateManager interface {
	ManagerLifecycle

	// Create a new domain query context - caller is responsible for closing it.
	NewDomainQueryContext(ctx context.Context, domain Domain, contractAddress pldtypes.EthAddress) DomainQueryContext

	// Create a domain query context that answers queries by merging the local DB view with a
	// remote in-memory view: view serves its matching unconfirmed states and spent state IDs on demand.
	// Caller is responsible for closing it.
	NewDomainQueryContextWithRemoteView(ctx context.Context, domain Domain, contractAddress pldtypes.EthAddress, view RemoteStateView) DomainQueryContext

	// FindMatchingInMemoryStates evaluates a domain state query against caller-supplied snapshot
	// candidates.
	FindMatchingInMemoryStates(ctx context.Context, domainName string, schemaID pldtypes.Bytes32, query *query.QueryJSON, candidates []*prototk.SnapshotState) ([]*prototk.QueriedState, error)

	// Create a new domain state writer
	NewDomainStateWriter(ctx context.Context, domain Domain, contractAddress pldtypes.EthAddress) DomainStateWriter

	// Get a previously created domain query context
	GetDomainQueryContext(ctx context.Context, id uuid.UUID) DomainQueryContext

	// Ensure ABI schemas upserts all the specified schemas, using the given DB transaction
	EnsureABISchemas(ctx context.Context, dbTX persistence.DBTX, domainName string, defs []*abi.Parameter) ([]Schema, error)

	// Get an individual schema by ID
	GetSchemaByID(ctx context.Context, dbTX persistence.DBTX, domainName string, schemaID pldtypes.Bytes32, failNotFound bool) (*pldapi.Schema, error)

	// State finalizations are written on the DB context of the block indexer, by the domain manager.
	WriteStateFinalizations(ctx context.Context, dbTX persistence.DBTX, spends []*pldapi.StateSpendRecord, reads []*pldapi.StateReadRecord, confirms []*pldapi.StateConfirmRecord, infoRecords []*pldapi.StateInfoRecord) (err error)

	// ValidateStates validates and normalizes each state's data against its schema, and calculates its state ID.
	ValidateStates(ctx context.Context, dbTX persistence.DBTX, domain Domain, contractAddress pldtypes.EthAddress, states ...*prototk.EndorsableState) ([]*pldapi.State, error)

	// ValidateStatesWithLabels performs the same validation and normalization as ValidateStates, additionally
	// extracting the state's label values.
	ValidateStatesWithLabels(ctx context.Context, dbTX persistence.DBTX, domain Domain, contractAddress pldtypes.EthAddress, states ...*prototk.EndorsableState) ([]*StateWithLabels, error)

	// MUST NOT be called for states received over a network from another node.
	// Writes a batch of states that have been pre-verified BY THIS NODE so can bypass domain hash verification.
	WritePreVerifiedStates(ctx context.Context, dbTX persistence.DBTX, domainName string, states []*StateUpsertOutsideContext) ([]*pldapi.State, error)

	// Write a batch of states that have been received over the network. ID hash calculation will be validated by the domain as prior to storage
	WriteReceivedStates(ctx context.Context, dbTX persistence.DBTX, domainName string, states []*StateUpsertOutsideContext) ([]*pldapi.State, error)

	// Write a batch of nullifiers that correspond to states just received
	WriteNullifiersForReceivedStates(ctx context.Context, dbTX persistence.DBTX, domainName string, nullifiers []*NullifierUpsert) error

	// Find states from outside of a domain context (noting you can reference a domain context by ID)
	FindStates(ctx context.Context, dbTX persistence.DBTX, domainName string, schemaID pldtypes.Bytes32, query *query.QueryJSON, extQueryOptions *StateQueryOptions) (s []*pldapi.State, err error)

	// GetState returns state by ID, with optional labels
	GetStatesByID(ctx context.Context, dbTX persistence.DBTX, domainName string, contractAddress *pldtypes.EthAddress, stateIDs []pldtypes.HexBytes, failNotFound, withLabels bool) ([]*pldapi.State, error)

	// Get all states created, read or spent by a confirmed transaction
	GetTransactionStates(ctx context.Context, dbTX persistence.DBTX, txID uuid.UUID) (*pldapi.TransactionStates, error)

	// WritePendingPrivateStateDataBatch writes rows for a batch of states whose private data has not yet arrived.
	WritePendingPrivateStateDataBatch(ctx context.Context, dbTX persistence.DBTX, domainName string, states []PendingPrivateStateDataEntry) error

	// CheckPendingPrivateStateDataForContract returns true if there are no outstanding rows for the given contract at or below the given block number.
	CheckPendingPrivateStateDataForContract(ctx context.Context, dbTX persistence.DBTX, contract string, block int64) (complete bool, err error)
}

// RemoteStateView is on-demand read access to a remote view of available and spent states. Implementations
// block until the response arrives (or ctx expires). Callers are responsible for validating data
// returned by QueryAvailableStates against the claimed state IDs — the source is not trusted.
type RemoteStateView interface {
	QueryAvailableStates(ctx context.Context, schemaID string, queryJSON string) ([]*prototk.QueriedState, error)
	GetSpentStateIDs(ctx context.Context) ([]pldtypes.HexBytes, error)
}

type PendingPrivateStateDataEntry struct {
	StateID     pldtypes.HexBytes
	Contract    pldtypes.EthAddress
	BlockNumber int64
}

type StateQueryOptions struct {
	StatusQualifier      pldapi.StateStatusQualifier
	ExcludedIDs          []pldtypes.HexBytes
	ExcludedNullifierIDs []pldtypes.HexBytes
	QueryModifier        func(db persistence.DBTX, query *gorm.DB) *gorm.DB
}

// DomainStateWriter is a long-lived write buffer used for flushing domain states and nullifiers to the DB.
type DomainStateWriter interface {
	// StageWrites validates the nullifiers against the supplied states and, only if the whole batch is
	// consistent, atomically appends both the states and their nullifiers to the in-memory write buffer.
	// The nullified state must be present in the states passed to this same call. Written on the next flush.
	StageWrites(ctx context.Context, states []*StateWithLabels, nullifiers ...*NullifierUpsert) error

	// Flush writes all pending states and nullifiers to the database within the given transaction.
	// Must be called within an active DB transaction. Returns an error if a flush is already in
	// progress or if the write fails.
	Flush(ctx context.Context, dbTX persistence.DBTX) error

	// Reset clears all un-flushed writes and in-memory locks, restoring to DB state.
	// Used for error recovery after a failed flush.
	Reset()
}

// DomainQueryContext is the state query interface exposed outside of the statestore package. It may
// be backed by a remote view, in which case queries are executed against both the remote view and
// the local persisted state store, with the results merged together.
//
// A DomainQueryContext is typically short-lived and must be closed by its consumer when no longer needed
// to avoid leaking resources.
type DomainQueryContext interface {
	// ID returns the UUID that identifies this context in the state manager registry.
	ID() uuid.UUID

	// FindAvailableStates is the primary query function, returning only available states.
	// For contexts created with a remote view, results merge the view's matches and respect
	// its spend exclusions.
	FindAvailableStates(ctx context.Context, dbTX persistence.DBTX, schemaID pldtypes.Bytes32, query *query.QueryJSON) (Schema, []*pldapi.State, error)

	// FindAvailableNullifiers is similar to FindAvailableStates for nullifier-based domains.
	FindAvailableNullifiers(ctx context.Context, dbTX persistence.DBTX, schemaID pldtypes.Bytes32, query *query.QueryJSON) (Schema, []*pldapi.State, error)

	// GetStatesByID retrieves states by ID regardless of confirmation/spend status,
	// including states pending in memory.
	GetStatesByID(ctx context.Context, dbTX persistence.DBTX, schemaID pldtypes.Bytes32, ids []string) (Schema, []*pldapi.State, error)

	// ContractAddress returns the contract address this context was opened for.
	ContractAddress() pldtypes.EthAddress

	// Close deregisters the context from the state manager and prevents further use.
	Close(ctx context.Context)
}

type StateUpsertOutsideContext struct {
	ID              pldtypes.HexBytes
	SchemaID        pldtypes.Bytes32
	ContractAddress *pldtypes.EthAddress
	Data            pldtypes.RawJSON
}

// StateWithLabels is a newly prepared state that has not yet been persisted
type StateWithLabels struct {
	*pldapi.State
	LabelValues filters.ValueSet
}

func (s *StateWithLabels) ValueSet() filters.ValueSet {
	return s.LabelValues
}

// ProtoLabels converts the resolved label values to their proto wire form.
func (s *StateWithLabels) ProtoLabels() *prototk.StateLabels {
	pl := &prototk.StateLabels{
		Labels:      make([]*prototk.StateLabel, len(s.Labels)),
		Int64Labels: make([]*prototk.StateInt64Label, len(s.Int64Labels)),
	}
	for i, l := range s.Labels {
		pl.Labels[i] = &prototk.StateLabel{Label: l.Label, Value: l.Value}
	}
	for i, l := range s.Int64Labels {
		pl.Int64Labels[i] = &prototk.StateInt64Label{Label: l.Label, Value: l.Value}
	}
	return pl
}

type NullifierUpsert struct {
	ID    pldtypes.HexBytes `json:"id"              gorm:"primaryKey"`
	State pldtypes.HexBytes `json:"-"`
}

type Schema interface {
	Type() pldapi.SchemaType
	ID() pldtypes.Bytes32
	Signature() string
	Persisted() *pldapi.Schema
	ProcessState(ctx context.Context, contractAddress *pldtypes.EthAddress, data pldtypes.RawJSON, id pldtypes.HexBytes, customHash bool) (*pldapi.State, error)
	ProcessStateWithLabels(ctx context.Context, contractAddress *pldtypes.EthAddress, data pldtypes.RawJSON, id pldtypes.HexBytes, customHash bool) (*StateWithLabels, error)
	RecoverLabels(ctx context.Context, s *pldapi.State) (*StateWithLabels, error)
}
