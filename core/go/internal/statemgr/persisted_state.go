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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type persistedSchema struct {
	ID         pldtypes.Bytes32   `gorm:"primaryKey"`
	Created    pldtypes.Timestamp `gorm:"autoCreateTime:false"` // we calculate the created time ourselves due to complex in-memory caching
	DomainName string
	Type       pldtypes.Enum[pldapi.SchemaType]
	Signature  string
	Definition pldtypes.RawJSON
	Labels     []string `gorm:"type:text[]; serializer:json"`
}

func (persistedSchema) TableName() string {
	return "schemas"
}

func newPersistedSchema(s *pldapi.Schema) *persistedSchema {
	return &persistedSchema{
		ID:         s.ID,
		Created:    s.Created,
		DomainName: s.DomainName,
		Type:       s.Type,
		Signature:  s.Signature,
		Definition: s.Definition,
		Labels:     s.Labels,
	}
}

func (ps *persistedSchema) mapToAPI() *pldapi.Schema {
	return &pldapi.Schema{
		ID:         ps.ID,
		Created:    ps.Created,
		DomainName: ps.DomainName,
		Type:       ps.Type,
		Signature:  ps.Signature,
		Definition: ps.Definition,
		Labels:     ps.Labels,
	}
}

type persistedStateBase struct {
	ID              pldtypes.HexBytes  `gorm:"primaryKey"`
	Created         pldtypes.Timestamp `gorm:"autoCreateTime:nano"`
	DomainName      string             `gorm:"primaryKey"`
	Schema          pldtypes.Bytes32
	ContractAddress *pldtypes.EthAddress
	Data            pldtypes.RawJSON
}

type persistedState struct {
	Base      persistedStateBase           `gorm:"embedded"`
	Confirmed *persistedStateConfirmRecord `gorm:"foreignKey:DomainName,State;references:DomainName,ID"`
	Spent     *persistedStateSpendRecord   `gorm:"foreignKey:DomainName,State;references:DomainName,ID"`
	Nullifier *persistedStateNullifier     `gorm:"foreignKey:DomainName,State;references:DomainName,ID"`
}

func (persistedState) TableName() string {
	return "states"
}

func newPersistedState(s *pldapi.State) *persistedState {
	return &persistedState{
		Base: persistedStateBase{
			ID:              s.ID,
			Created:         s.Created,
			DomainName:      s.DomainName,
			Schema:          s.Schema,
			ContractAddress: s.ContractAddress,
			Data:            s.Data,
		},
	}
}

func (ps *persistedStateBase) mapToAPI() pldapi.StateBase {
	return pldapi.StateBase{
		ID:              ps.ID,
		Created:         ps.Created,
		DomainName:      ps.DomainName,
		Schema:          ps.Schema,
		ContractAddress: ps.ContractAddress,
		Data:            ps.Data,
	}
}

func (ps *persistedState) mapToAPI() *pldapi.State {
	s := &pldapi.State{StateBase: ps.Base.mapToAPI()}
	if ps.Confirmed != nil {
		s.Confirmed = ps.Confirmed.mapToAPI()
	}
	if ps.Spent != nil {
		s.Spent = ps.Spent.mapToAPI()
	}
	if ps.Nullifier != nil {
		s.Nullifier = ps.Nullifier.mapToAPI()
	}
	return s
}

func mapStatesToAPI(persisted []*persistedState) []*pldapi.State {
	states := make([]*pldapi.State, len(persisted))
	for i, ps := range persisted {
		states[i] = ps.mapToAPI()
	}
	return states
}

// Label rows denormalize the identity and availability columns of the state they belong to,
// so available-state queries with a label predicate can be served from the label table alone.
// The state manager keeps them in lockstep with the states row on every write path.
type persistedStateLabel struct {
	DomainName      string            `gorm:"primaryKey"`
	State           pldtypes.HexBytes `gorm:"primaryKey"`
	Label           string
	Value           string
	Schema          pldtypes.Bytes32
	ContractAddress *pldtypes.EthAddress
	Created         pldtypes.Timestamp
	Confirmed       bool
	Spent           bool
}

func (persistedStateLabel) TableName() string {
	return "state_labels"
}

type persistedStateInt64Label struct {
	DomainName      string            `gorm:"primaryKey"`
	State           pldtypes.HexBytes `gorm:"primaryKey"`
	Label           string
	Value           int64
	Schema          pldtypes.Bytes32
	ContractAddress *pldtypes.EthAddress
	Created         pldtypes.Timestamp
	Confirmed       bool
	Spent           bool
}

func (persistedStateInt64Label) TableName() string {
	return "state_int64_labels"
}

type persistedStateNullifier struct {
	DomainName string `gorm:"primaryKey"`
	State      pldtypes.HexBytes
	ID         pldtypes.HexBytes          `gorm:"primaryKey"`
	Spent      *persistedStateSpendRecord `gorm:"foreignKey:state;references:id;"`
}

func (persistedStateNullifier) TableName() string {
	return "state_nullifiers"
}

func newPersistedStateNullifiers(nullifiers []*pldapi.StateNullifier) []*persistedStateNullifier {
	persisted := make([]*persistedStateNullifier, len(nullifiers))
	for i, n := range nullifiers {
		persisted[i] = &persistedStateNullifier{
			DomainName: n.DomainName,
			State:      n.State,
			ID:         n.ID,
		}
	}
	return persisted
}

func (pn *persistedStateNullifier) mapToAPI() *pldapi.StateNullifier {
	n := &pldapi.StateNullifier{
		DomainName: pn.DomainName,
		State:      pn.State,
		ID:         pn.ID,
	}
	if pn.Spent != nil {
		n.Spent = pn.Spent.mapToAPI()
	}
	return n
}

type transactionStateRecord struct {
	Base           persistedStateBase `gorm:"embedded"`
	State          pldtypes.HexBytes  `gorm:"column:state"`
	RecordType     string             `gorm:"column:record_type"`
	Index          *int               `gorm:"column:idx"` // nil for records written before indexes were recorded
	SpentState     pldtypes.HexBytes  `gorm:"column:spent_state"`
	ReadState      pldtypes.HexBytes  `gorm:"column:read_state"`
	ConfirmedState pldtypes.HexBytes  `gorm:"column:confirmed_state"`
}

func (transactionStateRecord) TableName() string {
	return "states"
}

type persistedStateSpendRecord struct {
	DomainName  string            `gorm:"primaryKey"`
	State       pldtypes.HexBytes `gorm:"primaryKey"`
	Transaction uuid.UUID
	Index       int `gorm:"column:idx"`
}

func (persistedStateSpendRecord) TableName() string {
	return "state_spend_records"
}

func newPersistedStateSpendRecords(records []*pldapi.StateSpendRecord) []*persistedStateSpendRecord {
	persisted := make([]*persistedStateSpendRecord, len(records))
	for i, r := range records {
		persisted[i] = &persistedStateSpendRecord{
			DomainName:  r.DomainName,
			State:       r.State,
			Transaction: r.Transaction,
			Index:       r.Index,
		}
	}
	return persisted
}

func (pr *persistedStateSpendRecord) mapToAPI() *pldapi.StateSpendRecord {
	return &pldapi.StateSpendRecord{DomainName: pr.DomainName, State: pr.State, Transaction: pr.Transaction, Index: pr.Index}
}

type persistedStateConfirmRecord struct {
	DomainName  string            `gorm:"primaryKey"`
	State       pldtypes.HexBytes `gorm:"primaryKey"`
	Transaction uuid.UUID
	Index       int    `gorm:"column:idx"`
	BlockNumber *int64 `gorm:"column:block_number"`
}

func (persistedStateConfirmRecord) TableName() string {
	return "state_confirm_records"
}

func newPersistedStateConfirmRecords(records []*pldapi.StateConfirmRecord) []*persistedStateConfirmRecord {
	persisted := make([]*persistedStateConfirmRecord, len(records))
	for i, r := range records {
		persisted[i] = &persistedStateConfirmRecord{
			DomainName:  r.DomainName,
			State:       r.State,
			Transaction: r.Transaction,
			Index:       r.Index,
			BlockNumber: r.BlockNumber,
		}
	}
	return persisted
}

func (pr *persistedStateConfirmRecord) mapToAPI() *pldapi.StateConfirmRecord {
	return &pldapi.StateConfirmRecord{DomainName: pr.DomainName, State: pr.State, Transaction: pr.Transaction, Index: pr.Index, BlockNumber: pr.BlockNumber}
}

type persistedStateReadRecord struct {
	DomainName  string            `gorm:"primaryKey"`
	State       pldtypes.HexBytes `gorm:"primaryKey"`
	Transaction uuid.UUID         `gorm:"primaryKey"`
	Index       int               `gorm:"column:idx"`
}

func (persistedStateReadRecord) TableName() string {
	return "state_read_records"
}

func newPersistedStateReadRecords(records []*pldapi.StateReadRecord) []*persistedStateReadRecord {
	persisted := make([]*persistedStateReadRecord, len(records))
	for i, r := range records {
		persisted[i] = &persistedStateReadRecord{
			DomainName:  r.DomainName,
			State:       r.State,
			Transaction: r.Transaction,
			Index:       r.Index,
		}
	}
	return persisted
}

type persistedStateInfoRecord struct {
	DomainName  string            `gorm:"primaryKey"`
	State       pldtypes.HexBytes `gorm:"primaryKey"`
	Transaction uuid.UUID         `gorm:"primaryKey"`
	Index       int               `gorm:"column:idx"`
}

func (persistedStateInfoRecord) TableName() string {
	return "state_info_records"
}

func newPersistedStateInfoRecords(records []*pldapi.StateInfoRecord) []*persistedStateInfoRecord {
	persisted := make([]*persistedStateInfoRecord, len(records))
	for i, r := range records {
		persisted[i] = &persistedStateInfoRecord{
			DomainName:  r.DomainName,
			State:       r.State,
			Transaction: r.Transaction,
			Index:       r.Index,
		}
	}
	return persisted
}
