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

func genWidget(t *testing.T, schemaID pldtypes.Bytes32, withoutSalt string) *prototk.EndorsableState {
	var ij map[string]interface{}
	err := json.Unmarshal([]byte(withoutSalt), &ij)
	require.NoError(t, err)
	ij["salt"] = pldtypes.RandHex(32)
	withSalt, err := json.Marshal(ij)
	require.NoError(t, err)
	return &prototk.EndorsableState{
		SchemaId:      schemaID.String(),
		StateDataJson: string(withSalt),
	}
}

func makeWidgets(t *testing.T, ctx context.Context, ss *stateManager, domainName string, contractAddress *pldtypes.EthAddress, schemaID pldtypes.Bytes32, withoutSalt []string) []*pldapi.State {
	states := make([]*pldapi.State, len(withoutSalt))
	for i, w := range withoutSalt {
		withSalt := genWidget(t, schemaID, w)
		var newStates []*pldapi.State
		err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
			newStates, err = ss.WritePreVerifiedStates(ctx, dbTX, domainName, []*components.StateUpsertOutsideContext{
				{
					ContractAddress: contractAddress,
					SchemaID:        schemaID,
					Data:            pldtypes.RawJSON(withSalt.StateDataJson),
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

// newTestAssemblyContext opens an assembly domain context wired to a remote view onto the view
// owner's in-memory states; the view's spent state IDs are fetched lazily as the spend exclusion
// set on the first query. The caller supplies the contract address (needed to build the view
// beforehand) and owns Close.
func newTestAssemblyContext(t *testing.T, ctx context.Context, ss *stateManager, name string, customHashFunction bool, contractAddress *pldtypes.EthAddress, view components.RemoteStateView) *domainQueryContext {
	md := componentsmocks.NewDomain(t)
	md.On("Name").Return(name)
	md.On("CustomHashFunction").Return(customHashFunction)
	dqc := ss.NewDomainQueryContextWithRemoteView(ctx, md, *contractAddress, view)
	return dqc.(*domainQueryContext)
}

// testStateResolver validates states against the writer's domain and contract, standing in for the
// coordinator's call to StateManager.ValidateStatesWithLabels ahead of StageWrites.
type testStateResolver func(states ...*prototk.EndorsableState) ([]*components.StateWithLabels, error)

func newTestDomainStateWriter(t *testing.T, ctx context.Context, ss *stateManager, name string, customHashFunction bool) (*pldtypes.EthAddress, *domainStateWriter, testStateResolver) {
	md := componentsmocks.NewDomain(t)
	md.On("Name").Return(name)
	md.On("CustomHashFunction").Return(customHashFunction).Maybe()
	contractAddress := pldtypes.RandAddress()
	dsw := ss.NewDomainStateWriter(ctx, md, *contractAddress)
	sw := dsw.(*domainStateWriter)
	// Reads sw.contractAddress on each call, so tests that re-point the writer at another contract
	// resolve against that same contract.
	resolve := func(states ...*prototk.EndorsableState) ([]*components.StateWithLabels, error) {
		return ss.ValidateStatesWithLabels(ctx, ss.p.NOTX(), md, sw.contractAddress, states...)
	}
	return contractAddress, sw, resolve
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
	seqQual := pldapi.StateStatusQualifier(dqc.ID().String())

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

	// setTestRemoteView closes the current context and opens a fresh one whose remote view serves
	// the given ahead-of-chain candidates and spent-state exclusion set, updating dqc and its
	// seqQual (a view is fixed for a context's life). Which states the coordinator serves as
	// candidates vs. exclusions is its business, not under test here — each call just states the
	// response the view gives.
	setTestRemoteView := func(candidateStates []*prototk.EndorsableState, spentStateIDs ...pldtypes.HexBytes) {
		var candidates []*prototk.SnapshotState
		for _, u := range candidateStates {
			id, err := pldtypes.ParseHexBytes(ctx, u.Id)
			require.NoError(t, err)
			sw, err := schema.ProcessStateWithLabels(ctx, contractAddress, pldtypes.RawJSON(u.StateDataJson), id, false)
			require.NoError(t, err)
			candidates = append(candidates, snapshotStateOf(sw, 0))
		}
		dqc.Close(ctx)
		dqc = newTestAssemblyContext(t, ctx, ss, "domain1", false, contractAddress,
			&testRemoteView{ss: ss, domainName: "domain1", candidates: candidates, spentStateIDs: spentStateIDs})
		seqQual = pldapi.StateStatusQualifier(dqc.ID().String())
	}

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

	// Write widget[5] to DB (unconfirmed) via WritePreVerifiedStates, then serve it via a fresh
	// DomainQueryContext remote view so the seqQual query can see the creating state.
	// This mirrors what the coordinator does: the DSW flushes the state to DB, then the
	// assembler opens an assembly context wired to the coordinator's remote state view.
	widget5State := genWidget(t, schemaID, `{"size": 66666, "color": "blue", "price": 600}`)
	var widget5States []*pldapi.State
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		widget5States, err = ss.WritePreVerifiedStates(ctx, dbTX, "domain1", []*components.StateUpsertOutsideContext{
			{ContractAddress: contractAddress, SchemaID: schemaID, Data: pldtypes.RawJSON(widget5State.StateDataJson)},
		})
		return err
	})
	require.NoError(t, err)
	widgets = append(widgets, widget5States[0])
	widget5State.Id = widgets[5].ID.String() // ID is computed by WritePreVerifiedStates; set here so setTestRemoteView doesn't see a zero "0x" ID

	setTestRemoteView([]*prototk.EndorsableState{widget5State})

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // added 5
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3, 5)     // added 5
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)                     // added 5 (via the remote view)

	// The coordinator spend-locks widget[5]: its view stops serving it as a candidate and its ID
	// joins the spent exclusion set.
	setTestRemoteView(nil, widgets[5].ID)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3, 5)     // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4)                        // removed 5

	// The spend lock is released: the new view serves widget[5] as a candidate again, with no
	// exclusions.
	setTestRemoteView([]*prototk.EndorsableState{widget5State})

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

	// Close the old DQC and open a fresh one with no remote view.
	// Widget[5] is now confirmed in DB so it is visible via DB-available queries without a remote view.
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

	// Serve widget[3] via a new remote view: it's unconfirmed in DB (never confirmed above) but
	// the seqQual can see it once the coordinator's view serves it as a candidate.
	setTestRemoteView([]*prototk.EndorsableState{{SchemaId: schemaID.String(), StateDataJson: string(widgets[3].Data), Id: widgets[3].ID.String()}})

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3)        // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 3, 4, 5)                  // added 3 (via the remote view)

	// check a sub-select
	checkQuery(query.NewQueryBuilder().Equal("color", "pink").Query(), seqQual, 3)
	checkQuery(query.NewQueryBuilder().Equal("color", "pink").Query(), pldapi.StateStatusAvailable)

}
