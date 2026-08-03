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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const localNodeName = "node1"

func setupTransferStateWidget(t *testing.T, ctx context.Context, ss *stateManager, m *mockComponents) *pldapi.State {
	_ = mockDomain(t, m, "domain1", false)
	mockStateCallback(m)

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, widgetABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema.Schema})
	require.NoError(t, err)

	contractAddress := pldtypes.RandAddress()
	return makeWidgets(t, ctx, ss, "domain1", contractAddress, schema.ID(), []string{
		`{"size": 11111, "color": "red", "price": 100}`,
	})[0]
}

func TestTransferStateInvalidRecipient(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	m.transportManager.On("LocalNodeName").Return(localNodeName)

	stateID := pldtypes.Bytes32Keccak(([]byte)("state1")).Bytes()
	_, err := ss.TransferState(ctx, ss.p.NOTX(), "domain1", stateID, pldtypes.PrivateIdentityLocator("_@"))
	assert.Regexp(t, "PD020005", err)
}

func TestTransferStateMissingState(t *testing.T) {
	ctx, ss, db, m, done := newDBMockStateManager(t)
	defer done()

	m.transportManager.On("LocalNodeName").Return(localNodeName)

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	stateID := pldtypes.Bytes32Keccak(([]byte)("state1")).Bytes()
	_, err := ss.TransferState(ctx, ss.p.NOTX(), "domain1", stateID, pldtypes.PrivateIdentityLocator("alice@node2"))
	assert.Regexp(t, "PD010112", err)
}

func TestTransferStateLocalNoOp(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	m.transportManager.On("LocalNodeName").Return(localNodeName).Maybe()

	state := setupTransferStateWidget(t, ctx, ss, m)

	msgID, err := ss.TransferState(ctx, ss.p.NOTX(), "domain1", state.ID, pldtypes.PrivateIdentityLocator("alice@node1"))
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, msgID)

	// identity without explicit node resolves to local node
	msgID, err = ss.TransferState(ctx, ss.p.NOTX(), "domain1", state.ID, pldtypes.PrivateIdentityLocator("alice"))
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, msgID)
}

func TestTransferStateRemoteOk(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	m.transportManager.On("LocalNodeName").Return(localNodeName).Maybe()

	expectedMsgID := uuid.New()
	m.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			msgs := args.Get(2).([]*pldapi.ReliableMessage)
			msgs[0].ID = expectedMsgID
		}).Return(nil).Once()

	state := setupTransferStateWidget(t, ctx, ss, m)
	recipient := pldtypes.PrivateIdentityLocator("alice@node2")

	msgID, err := ss.TransferState(ctx, ss.p.NOTX(), "domain1", state.ID, recipient)
	require.NoError(t, err)
	assert.Equal(t, expectedMsgID, msgID)
}

func TestTransferStateRemoteMessage(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	m.transportManager.On("LocalNodeName").Return(localNodeName).Maybe()

	state := setupTransferStateWidget(t, ctx, ss, m)
	recipient := pldtypes.PrivateIdentityLocator("alice@node2")

	var capturedMsg *pldapi.ReliableMessage
	m.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			capturedMsg = args.Get(2).([]*pldapi.ReliableMessage)[0]
			capturedMsg.ID = uuid.New()
		}).Return(nil).Once()

	_, err := ss.TransferState(ctx, ss.p.NOTX(), "domain1", state.ID, recipient)
	require.NoError(t, err)
	require.NotNil(t, capturedMsg)

	assert.Equal(t, "node2", capturedMsg.Node)
	assert.Equal(t, pldapi.RMTState, capturedMsg.MessageType.V())

	var sd components.StateDistribution
	err = json.Unmarshal([]byte(capturedMsg.Metadata), &sd)
	require.NoError(t, err)
	assert.Equal(t, state.ID.String(), sd.StateID)
	assert.Equal(t, recipient.String(), sd.IdentityLocator)
	assert.Equal(t, "domain1", sd.Domain)
	assert.Equal(t, state.ContractAddress.String(), sd.ContractAddress)
	assert.Equal(t, state.Schema.String(), sd.SchemaID)
}

func TestTransferStateSendReliableFail(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	m.transportManager.On("LocalNodeName").Return(localNodeName).Maybe()
	m.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).
		Return(fmt.Errorf("pop")).Once()

	state := setupTransferStateWidget(t, ctx, ss, m)

	_, err := ss.TransferState(ctx, ss.p.NOTX(), "domain1", state.ID, pldtypes.PrivateIdentityLocator("alice@node2"))
	assert.Regexp(t, "pop", err)
}

func TestTransferStateNodeLookupFail(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	m.transportManager.On("LocalNodeName").Return("").Maybe()

	// Locator without an explicit node requires a default node from LocalNodeName.
	_, err := ss.TransferState(ctx, ss.p.NOTX(), "domain1", pldtypes.RandBytes(32), pldtypes.PrivateIdentityLocator("alice"))
	assert.Regexp(t, "PD020017", err)
}

func TestTransferStateInTransaction(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	m.transportManager.On("LocalNodeName").Return(localNodeName).Maybe()

	expectedMsgID := uuid.New()
	m.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			msgs := args.Get(2).([]*pldapi.ReliableMessage)
			msgs[0].ID = expectedMsgID
		}).Return(nil).Once()

	state := setupTransferStateWidget(t, ctx, ss, m)

	var msgID uuid.UUID
	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		var txErr error
		msgID, txErr = ss.TransferState(ctx, dbTX, "domain1", state.ID, pldtypes.PrivateIdentityLocator("alice@node2"))
		return txErr
	})
	require.NoError(t, err)
	assert.Equal(t, expectedMsgID, msgID)
}
