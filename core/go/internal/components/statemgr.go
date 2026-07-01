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
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"gorm.io/gorm"
)

type StateManager interface {
	ManagerLifecycle

	// Create a new domain query context - caller is responsible for closing it.
	NewDomainQueryContext(ctx context.Context, domain Domain, contractAddress pldtypes.EthAddress) DomainQueryContext

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

	// Validate a set of state upserts against their schemas
	ValidateStates(ctx context.Context, dbTX persistence.DBTX, domainName string, contractAddress pldtypes.EthAddress, customHashFunction bool, stateUpserts ...*StateUpsert) ([]*pldapi.StateBase, error)

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

type PendingPrivateStateDataEntry struct {
	StateID     pldtypes.HexBytes
	Contract    pldtypes.EthAddress
	BlockNumber int64
}

type StateQueryOptions struct {
	StatusQualifier pldapi.StateStatusQualifier
	ExcludedIDs     []pldtypes.HexBytes
	QueryModifier   func(db persistence.DBTX, query *gorm.DB) *gorm.DB
}

// DomainStateWriter is a long-lived write buffer used for flushing domain states and nullifiers to the DB.
type DomainStateWriter interface {
	// UpsertStates creates or updates states in the in-memory write buffer.
	// States are visible immediately for queries on this writer (even before flush).
	// If a non-nil CreatedBy is set on a state, an in-memory create lock is registered.
	UpsertStates(ctx context.Context, dbTX persistence.DBTX, states ...*StateUpsert) (s []*pldapi.State, err error)

	// UpsertNullifiers creates nullifier records associated with states.
	// Nullifiers will be written to the DB on the next flush.
	UpsertNullifiers(ctx context.Context, nullifiers ...*NullifierUpsert) error

	// Flush writes all pending states and nullifiers to the database within the given transaction.
	// Must be called within an active DB transaction. Returns an error if a flush is already in
	// progress or if the write fails.
	Flush(ctx context.Context, dbTX persistence.DBTX) error

	// Reset clears all un-flushed writes and in-memory locks, restoring to DB state.
	// Used for error recovery after a failed flush.
	Reset()
}

// DomainQueryContext is the state query interface exposed outside of the statestore package. It may
// optionally import a snapshot representing a domain instance's ahead of chain view, allowing queries
// to be executed in context of this view.
//
// A DomainQueryContext is typically short-lived and must be closed by its consumer when no longer needed
// to avoid leaking resources.
type DomainQueryContext interface {
	// ID returns the UUID that identifies this context in the state manager registry.
	ID() uuid.UUID

	// ImportSnapshot hydrates this context with a domain instance's ahead of chain view.
	ImportSnapshot(ctx context.Context, stateLocksJSON []byte) error

	// FindAvailableStates is the primary query function, returning only available states.
	// For snapshot-loaded contexts, results include in-memory creating states and respect locks.
	// The dbTX is passed to allow connection re-use during read operations.
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

type StateUpsert struct {
	ID        pldtypes.HexBytes `json:"id"`
	Schema    pldtypes.Bytes32  `json:"schema"`
	Data      pldtypes.RawJSON  `json:"data"`
	CreatedBy *uuid.UUID        `json:"createdBy,omitempty"` // not exported
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

type NullifierUpsert struct {
	ID    pldtypes.HexBytes `json:"id"              gorm:"primaryKey"`
	State pldtypes.HexBytes `json:"-"`
}

type Schema interface {
	Type() pldapi.SchemaType
	ID() pldtypes.Bytes32
	Signature() string
	Persisted() *pldapi.Schema
	ProcessState(ctx context.Context, contractAddress *pldtypes.EthAddress, data pldtypes.RawJSON, id pldtypes.HexBytes, customHash bool) (*StateWithLabels, error)
	RecoverLabels(ctx context.Context, s *pldapi.State) (*StateWithLabels, error)
}
