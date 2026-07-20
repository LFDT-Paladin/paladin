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
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapLock is a test-only convenience for describing a state lock before conversion to its proto
// wire form in ImportSnapshot.
type snapLock struct {
	State       pldtypes.HexBytes
	Transaction uuid.UUID
	Type        prototk.SnapshotStateLock_StateLockType
}

const widgetABI = `{
	"type": "tuple",
	"internalType": "struct Widget",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "size",
			"type": "int64"
		},
		{
			"name": "color",
			"type": "string",
			"indexed": true
		},
		{
			"name": "price",
			"type": "uint256",
			"indexed": true
		}
	]
}`

func genWidget(t *testing.T, schemaID pldtypes.Bytes32, txID *uuid.UUID, withoutSalt string) *components.StateUpsert {
	var ij map[string]interface{}
	err := json.Unmarshal([]byte(withoutSalt), &ij)
	require.NoError(t, err)
	ij["salt"] = pldtypes.RandHex(32)
	withSalt, err := json.Marshal(ij)
	require.NoError(t, err)
	return &components.StateUpsert{
		Schema:    schemaID,
		Data:      withSalt,
		CreatedBy: txID,
	}
}

func makeWidgets(t *testing.T, ctx context.Context, ss *stateManager, domainName string, contractAddress *pldtypes.EthAddress, schemaID pldtypes.Bytes32, withoutSalt []string) []*pldapi.State {
	states := make([]*pldapi.State, len(withoutSalt))
	for i, w := range withoutSalt {
		withSalt := genWidget(t, schemaID, nil, w)
		var newStates []*pldapi.State
		err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
			newStates, err = ss.WritePreVerifiedStates(ctx, dbTX, domainName, []*components.StateUpsertOutsideContext{
				{
					ContractAddress: contractAddress,
					SchemaID:        schemaID,
					Data:            withSalt.Data,
				},
			})
			return err
		})
		require.NoError(t, err)
		states[i] = newStates[0]
		fmt.Printf("widget[%d]: %s\n", i, states[i].Data)
	}
	return states
}

func syncFlushWriter(t *testing.T, ctx context.Context, sw *domainStateWriter) {
	err := sw.ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return sw.Flush(ctx, dbTX)
	})
	require.NoError(t, err)
}

func newTestDomainContext(t *testing.T, ctx context.Context, ss *stateManager, name string, customHashFunction bool) (*pldtypes.EthAddress, *domainQueryContext) {
	md := componentsmocks.NewDomain(t)
	md.On("Name").Return(name)
	md.On("CustomHashFunction").Return(customHashFunction)
	contractAddress := pldtypes.RandAddress()
	dqc := ss.NewDomainQueryContext(ctx, md, *contractAddress)
	return contractAddress, dqc.(*domainQueryContext)
}

func newTestDomainStateWriter(t *testing.T, ctx context.Context, ss *stateManager, name string, customHashFunction bool) (*pldtypes.EthAddress, *domainStateWriter) {
	md := componentsmocks.NewDomain(t)
	md.On("Name").Return(name)
	md.On("CustomHashFunction").Return(customHashFunction)
	contractAddress := pldtypes.RandAddress()
	dsw := ss.NewDomainStateWriter(ctx, md, *contractAddress)
	return contractAddress, dsw.(*domainStateWriter)
}

func TestStateLockingQuery(t *testing.T) {

	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)
	mockStateCallback(m)

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, widgetABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema.Schema})
	require.NoError(t, err)
	schemaID := schema.ID()

	contractAddress, dqc := newTestDomainContext(t, ctx, ss, "domain1", false)

	widgets := makeWidgets(t, ctx, ss, "domain1", contractAddress, schemaID, []string{
		`{"size": 11111, "color": "red",  "price": 100}`,
		`{"size": 22222, "color": "red",  "price": 150}`,
		`{"size": 33333, "color": "blue", "price": 199}`,
		`{"size": 44444, "color": "pink", "price": 199}`,
		`{"size": 55555, "color": "blue", "price": 500}`,
	})

	checkQuery := func(jq *query.QueryJSON, status pldapi.StateStatusQualifier, expected ...int) {
		states, err := ss.FindContractStates(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, jq, status)
		require.NoError(t, err)
		assert.Len(t, states, len(expected))
		for _, wIndex := range expected {
			found := false
			for _, state := range states {
				if state.ID.Equals(widgets[wIndex].ID) {
					assert.False(t, found)
					found = true
					break
				}
			}
			assert.True(t, found, fmt.Sprintf("Widget %d missing", wIndex))
		}
	}

	// importSnapshot is a helper that calls ImportSnapshot with the given states and locks.
	importSnapshot := func(states []*components.StateUpsert, locks []*snapLock) {
		protoStates := make([]*prototk.SnapshotState, len(states))
		for i, u := range states {
			protoStates[i] = &prototk.SnapshotState{State: &prototk.EndorsableState{
				Id:            u.ID.String(),
				SchemaId:      u.Schema.String(),
				StateDataJson: string(u.Data),
			}}
		}
		protoLocks := make([]*prototk.SnapshotStateLock, len(locks))
		for i, l := range locks {
			txStr := l.Transaction.String()
			protoLocks[i] = &prototk.SnapshotStateLock{
				StateId:     l.State.String(),
				Transaction: &txStr,
				Type:        l.Type,
			}
		}
		require.NoError(t, dqc.ImportSnapshot(ctx, &prototk.StateSnapshot{
			States: protoStates,
			Locks:  protoLocks,
		}))
	}

	seqQual := pldapi.StateStatusQualifier(dqc.ID().String())
	all := query.NewQueryBuilder().Query()

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4)
	checkQuery(all, pldapi.StateStatusAvailable)
	checkQuery(all, pldapi.StateStatusConfirmed)
	checkQuery(all, pldapi.StateStatusUnconfirmed, 0, 1, 2, 3, 4)
	checkQuery(all, pldapi.StateStatusSpent)
	checkQuery(all, seqQual)

	// Mark them all confirmed apart from one
	for i, w := range widgets {
		if i != 3 {
			err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(), []*pldapi.StateSpendRecord{}, []*pldapi.StateReadRecord{},
				[]*pldapi.StateConfirmRecord{
					{DomainName: "domain1", State: w.ID, Transaction: uuid.New()},
				}, []*pldapi.StateInfoRecord{})
			require.NoError(t, err)
		}
	}

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 0, 1, 2, 4) // added all but 3
	checkQuery(all, pldapi.StateStatusConfirmed, 0, 1, 2, 4) // added all but 3
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3)        // added 3
	checkQuery(all, pldapi.StateStatusSpent)                 // unchanged
	checkQuery(all, seqQual, 0, 1, 2, 4)                     // added all but 3

	// Mark one spent
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{
			{DomainName: "domain1", State: widgets[0].ID, Transaction: uuid.New()},
		}, []*pldapi.StateReadRecord{}, []*pldapi.StateConfirmRecord{}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4) // removed 0
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4) // removed 0
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3)     // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)           // added 0
	checkQuery(all, seqQual, 1, 2, 4)                     // unchanged

	// Write widget[5] to DB (unconfirmed) via WritePreVerifiedStates, then import it into
	// the DomainQueryContext snapshot so the seqQual query can see the creating state.
	// This mirrors what the coordinator does: the DSW flushes the state to DB, then the
	// assembler's DQC receives a snapshot via ImportSnapshot.
	txID1 := uuid.New()
	widget5Upsert := genWidget(t, schemaID, &txID1, `{"size": 66666, "color": "blue", "price": 600}`)
	var widget5States []*pldapi.State
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		widget5States, err = ss.WritePreVerifiedStates(ctx, dbTX, "domain1", []*components.StateUpsertOutsideContext{
			{ContractAddress: contractAddress, SchemaID: schemaID, Data: widget5Upsert.Data},
		})
		return err
	})
	require.NoError(t, err)
	widgets = append(widgets, widget5States[0])
	widget5Upsert.ID = widgets[5].ID // ID is computed by WritePreVerifiedStates; set here so importSnapshot doesn't see a zero "0x" ID

	importSnapshot(
		[]*components.StateUpsert{widget5Upsert},
		[]*snapLock{{State: widgets[5].ID, Transaction: txID1, Type: prototk.SnapshotStateLock_CREATE}},
	)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // added 5
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3, 5)     // added 5
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)                     // added 5 (via snapshot)

	// Add a spend lock for widget[5]: re-import the full snapshot with both create and spend locks.
	txID2 := uuid.New()
	importSnapshot(
		[]*components.StateUpsert{widget5Upsert},
		[]*snapLock{
			{State: widgets[5].ID, Transaction: txID1, Type: prototk.SnapshotStateLock_CREATE},
			{State: widgets[5].ID, Transaction: txID2, Type: prototk.SnapshotStateLock_SPEND},
		},
	)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3, 5)     // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4)                        // removed 5

	// Cancel the spend lock by re-importing without it (ImportSnapshot replaces the whole snapshot).
	importSnapshot(
		[]*components.StateUpsert{widget5Upsert},
		[]*snapLock{{State: widgets[5].ID, Transaction: txID1, Type: prototk.SnapshotStateLock_CREATE}},
	)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3, 5)     // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)                     // added 5 back

	// Mark widget[5] confirmed in DB
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{},
		[]*pldapi.StateReadRecord{
			{DomainName: "domain1", State: widgets[1].ID, Transaction: uuid.New()}, // this is inert
		},
		[]*pldapi.StateConfirmRecord{
			{DomainName: "domain1", State: widgets[5].ID, Transaction: uuid.New()},
		}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	// Close the old DQC and open a fresh one with an empty snapshot.
	// Widget[5] is now confirmed in DB so it is visible via DB-available queries without a snapshot.
	dqc.Close(ctx)
	md2 := componentsmocks.NewDomain(t)
	md2.On("Name").Return("domain1")
	md2.On("CustomHashFunction").Return(false)
	dqc = ss.NewDomainQueryContext(ctx, md2, *contractAddress).(*domainQueryContext)
	defer dqc.Close(ctx)
	seqQual = pldapi.StateStatusQualifier(dqc.ID().String())

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4, 5) // added 5
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4, 5) // added 5
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3)        // removed 5
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)                     // unchanged (5 now confirmed in DB)

	// Import widget[3] into the snapshot: it's unconfirmed in DB (never confirmed above) but
	// the seqQual can see it once the coordinator sends its snapshot.
	txID13 := uuid.New()
	importSnapshot(
		[]*components.StateUpsert{{Schema: schemaID, Data: widgets[3].Data, ID: widgets[3].ID}},
		[]*snapLock{{State: widgets[3].ID, Transaction: txID13, Type: prototk.SnapshotStateLock_CREATE}},
	)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3)        // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 3, 4, 5)                  // added 3 (via snapshot)

	// check a sub-select
	checkQuery(query.NewQueryBuilder().Equal("color", "pink").Query(), seqQual, 3)
	checkQuery(query.NewQueryBuilder().Equal("color", "pink").Query(), pldapi.StateStatusAvailable)

}
