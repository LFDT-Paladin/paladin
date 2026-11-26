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

package transportmgr

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func testBatchEntryMsg(payload string) *prototk.PaladinMsg {
	return &prototk.PaladinMsg{
		MessageId:   uuid.NewString(),
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		MessageType: "test-message-type",
		Payload:     []byte(payload),
	}
}

func testBatchLimits(maxSize, maxBytes int) *transportManager {
	return &transportManager{sendBatchMaxSize: maxSize, sendBatchMaxBytes: maxBytes}
}

func testBatchMsg(t *testing.T, msgs ...*prototk.PaladinMsg) *prototk.PaladinMsg {
	entries := make([]batchEntry, len(msgs))
	for i, m := range msgs {
		entries[i] = batchEntry{msg: m}
	}
	batches := testBatchLimits(len(msgs), 1024).buildBatches(entries, true)
	require.Len(t, batches, 1)
	return batches[0].msg
}

func receiveFromNode2(t *testing.T, ctx context.Context, tp *testPlugin, msg *prototk.PaladinMsg) {
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{FromNode: "node2", Message: msg})
	require.NoError(t, err)
}

func TestBuildBatches(t *testing.T) {
	s1, s2, s3 := uint64(5), uint64(9), uint64(7)
	entries := []batchEntry{
		{msg: testBatchEntryMsg("a"), seq: &s1},
		{msg: testBatchEntryMsg("b"), seq: &s2},
		{msg: testBatchEntryMsg("c"), seq: &s3},
	}
	tm := testBatchLimits(3, 1024)

	batches := tm.buildBatches(entries, true)
	require.Len(t, batches, 1)
	assert.Equal(t, prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER, batches[0].msg.Component)
	assert.Equal(t, RMHMessageTypeBatch, batches[0].msg.MessageType)
	assert.Equal(t, uint64(9), *batches[0].seq)
	assert.Equal(t, 3, batches[0].count)
	decoded, err := decodeBatch(t.Context(), batches[0].msg.Payload)
	require.NoError(t, err)
	require.Len(t, decoded, 3)
	for i := range decoded {
		assert.Equal(t, entries[i].msg.MessageId, decoded[i].MessageId)
		assert.Equal(t, entries[i].msg.Payload, decoded[i].Payload)
	}

	batches = tm.buildBatches(entries, false)
	require.Len(t, batches, 3)
	for i, b := range batches {
		assert.Same(t, entries[i].msg, b.msg)
		assert.Same(t, entries[i].seq, b.seq)
		assert.Equal(t, 1, b.count)
	}

	batches = testBatchLimits(2, 1024).buildBatches(entries, true)
	require.Len(t, batches, 2)
	assert.Equal(t, 2, batches[0].count)
	assert.Equal(t, uint64(9), *batches[0].seq)
	assert.Same(t, entries[2].msg, batches[1].msg)

	big := strings.Repeat("x", 600)
	bigMsg := testBatchEntryMsg(big)
	batches = tm.buildBatches([]batchEntry{{msg: bigMsg}, {msg: testBatchEntryMsg(big)}, {msg: testBatchEntryMsg("small")}}, true)
	require.Len(t, batches, 2)
	assert.Same(t, bigMsg, batches[0].msg)
	assert.Equal(t, RMHMessageTypeBatch, batches[1].msg.MessageType)
	assert.Equal(t, 2, batches[1].count)
	assert.Nil(t, batches[1].seq)
}

func TestBatchLimitsConfig(t *testing.T) {
	_, tm, _, done := newTestTransport(t, false)
	defer done()
	assert.Equal(t, 200, tm.sendBatchMaxSize)
	assert.Equal(t, 1024*1024, tm.sendBatchMaxBytes)

	_, tm, _, done = newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
		conf.SendBatchMaxSize = confutil.P(10)
		conf.SendBatchMaxBytes = confutil.P("64Kb")
	})
	defer done()
	assert.Equal(t, 10, tm.sendBatchMaxSize)
	assert.Equal(t, 64*1024, tm.sendBatchMaxBytes)
}

func TestBuildBatchesSendsUnencodableMessageAlone(t *testing.T) {
	m1, m2, m3, m4 := testBatchEntryMsg("1"), testBatchEntryMsg("2"), testBatchEntryMsg("3"), testBatchEntryMsg("4")
	bad := testBatchEntryMsg("bad")
	bad.MessageType = "\xff"

	batches := testBatchLimits(5, 1024).buildBatches([]batchEntry{{msg: m1}, {msg: m2}, {msg: bad}, {msg: m3}, {msg: m4}}, true)
	require.Len(t, batches, 3)
	assert.Equal(t, RMHMessageTypeBatch, batches[0].msg.MessageType)
	assert.Equal(t, 2, batches[0].count)
	assert.Same(t, bad, batches[1].msg)
	assert.Equal(t, RMHMessageTypeBatch, batches[2].msg.MessageType)
	assert.Equal(t, 2, batches[2].count)
}

func TestDecodeBatchInvalid(t *testing.T) {
	_, err := decodeBatch(t.Context(), []byte{0xff})
	assert.Regexp(t, "PD012000", err)

	_, err = decodeBatch(t.Context(), protowire.AppendBytes(nil, []byte{0xff}))
	assert.Regexp(t, "PD012000", err)
}

func TestSendStampsTransportVersion(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockEmptyReliableMsgs,
		mockGoodTransport)
	defer done()

	sentMessages := make(chan *prototk.PaladinMsg, 1)
	mockActivateDeactivateOk(tp)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sentMessages <- req.Message
		return nil, nil
	}

	err := tm.Send(ctx, testMessage())
	require.NoError(t, err)

	sent := <-sentMessages
	assert.Equal(t, uint32(transportVersion), sent.TransportVersion)
}

func TestReceiveMessageRecordsLatestTransportVersion(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
		mc.sequencerManager.On("HandlePaladinMsg", mock.Anything, mock.Anything).Return()
	})
	defer done()

	msg := testBatchEntryMsg("some data")
	msg.TransportVersion = transportVersionBatching
	receiveFromNode2(t, ctx, tp, msg)
	assert.Equal(t, uint32(transportVersionBatching), tm.getPeerInfo("node2").TransportVersion)
	assert.True(t, tm.getActivePeer("node2").readsBatches())

	receiveFromNode2(t, ctx, tp, testBatchEntryMsg("some data"))
	assert.Equal(t, uint32(0), tm.getPeerInfo("node2").TransportVersion)
	assert.False(t, tm.getActivePeer("node2").readsBatches())
}

func TestReceiveBatchDeliversEachMessage(t *testing.T) {
	received := make(chan *components.ReceivedMessage, 3)
	ctx, tm, tp, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
		mc.sequencerManager.On("HandlePaladinMsg", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
			received <- args[1].(*components.ReceivedMessage)
		})
	})
	defer done()

	m1, m2, m3 := testBatchEntryMsg("one"), testBatchEntryMsg("two"), testBatchEntryMsg("three")
	batch := testBatchMsg(t, m1, testBatchMsg(t, m2, m3))
	batch.TransportVersion = transportVersionBatching
	receiveFromNode2(t, ctx, tp, batch)

	for _, expected := range []*prototk.PaladinMsg{m1, m2, m3} {
		r := <-received
		assert.Equal(t, expected.MessageId, r.MessageID.String())
		assert.Equal(t, "node2", r.FromNode)
		assert.Equal(t, expected.Payload, r.Payload)
	}
	// The envelope's version is recorded, not the inner messages' unset one.
	assert.Equal(t, uint32(transportVersionBatching), tm.getPeerInfo("node2").TransportVersion)
}

func TestReceiveBatchInvalid(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
		mc.sequencerManager.On("HandlePaladinMsg", mock.Anything, mock.Anything).Return()
	})
	defer done()

	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		FromNode: "node2",
		Message: &prototk.PaladinMsg{
			MessageId:   uuid.NewString(),
			Component:   prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
			MessageType: RMHMessageTypeBatch,
			Payload:     []byte{0xff},
		},
	})
	assert.Regexp(t, "PD012000", err)

	noPayload := testBatchEntryMsg("")
	_, err = tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		FromNode: "node2",
		Message:  testBatchMsg(t, testBatchEntryMsg("ok"), noPayload),
	})
	assert.Regexp(t, "PD012000", err)

	unknownComponent := testBatchEntryMsg("unknown component")
	unknownComponent.Component = prototk.PaladinMsg_Component(99)
	_, err = tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		FromNode: "node2",
		Message:  testBatchMsg(t, testBatchEntryMsg("ok"), unknownComponent),
	})
	assert.Regexp(t, "PD012011", err)
}

func TestFireAndForgetSendsBatchedOnlyWhenPeerReadsBatches(t *testing.T) {
	for _, peerVersion := range []uint32{0, transportVersionBatching} {
		ctx, tm, tp, done := newTestTransport(t, false,
			mockEmptyReliableMsgs,
			mockGoodTransport,
			func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
				mc.sequencerManager.On("HandlePaladinMsg", mock.Anything, mock.Anything).Return()
			})

		msg := testBatchEntryMsg("from node2")
		msg.TransportVersion = peerVersion
		receiveFromNode2(t, ctx, tp, msg)

		// The first send is held in the plugin until the others are queued behind it.
		sentMessages := make(chan *prototk.PaladinMsg)
		release := make(chan struct{})
		mockActivateDeactivateOk(tp)
		tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
			sentMessages <- req.Message
			<-release
			return nil, nil
		}

		sends := []*components.FireAndForgetMessageSend{testMessage(), testMessage(), testMessage()}
		require.NoError(t, tm.Send(ctx, sends[0]))
		first := <-sentMessages
		require.NoError(t, tm.Send(ctx, sends[1]))
		require.NoError(t, tm.Send(ctx, sends[2]))
		close(release)

		assert.Equal(t, sends[0].CorrelationID.String(), *first.CorrelationId)
		if peerVersion == 0 {
			for _, s := range sends[1:] {
				sent := <-sentMessages
				assert.Equal(t, s.CorrelationID.String(), *sent.CorrelationId)
			}
		} else {
			sent := <-sentMessages
			require.Equal(t, RMHMessageTypeBatch, sent.MessageType)
			decoded, err := decodeBatch(ctx, sent.Payload)
			require.NoError(t, err)
			require.Len(t, decoded, 2)
			for i, s := range sends[1:] {
				assert.Equal(t, s.CorrelationID.String(), *decoded[i].CorrelationId)
			}
		}

		done()
	}
}

func TestAcksBatchedOnlyWhenPeerReadsBatches(t *testing.T) {
	for _, peerVersion := range []uint32{0, transportVersionBatching} {
		ctx, tm, tp, done := newTestTransport(t, false,
			mockGoodTransport,
			mockEmptyReliableMsgs,
			func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
				mc.db.Mock.ExpectBegin()
				mc.db.Mock.ExpectCommit()
				mc.sequencerManager.On("HandlePaladinMsg", mock.Anything, mock.Anything).Return()
			},
		)

		msg := testBatchEntryMsg("from node2")
		msg.TransportVersion = peerVersion
		receiveFromNode2(t, ctx, tp, msg)

		sentMessages := make(chan *prototk.PaladinMsg, 2)
		mockActivateDeactivateOk(tp)
		tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
			sentMessages <- req.Message
			return nil, nil
		}

		p := tm.getActivePeer("node2")
		received := []*components.ReceivedMessage{
			testReceivedReliableMsg("unknown", struct{}{}),
			testReceivedReliableMsg("unknown", struct{}{}),
		}
		err := tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
				{p: p, msg: received[0]},
				{p: p, msg: received[1]},
			})
			return err
		})
		require.NoError(t, err)

		var nacks []*prototk.PaladinMsg
		if peerVersion == 0 {
			nacks = []*prototk.PaladinMsg{<-sentMessages, <-sentMessages}
		} else {
			sent := <-sentMessages
			require.Equal(t, RMHMessageTypeBatch, sent.MessageType)
			nacks, err = decodeBatch(ctx, sent.Payload)
			require.NoError(t, err)
		}
		require.Len(t, nacks, 2)
		for i, nack := range nacks {
			assert.Equal(t, RMHMessageTypeNack, nack.MessageType)
			assert.Equal(t, received[i].MessageID.String(), *nack.CorrelationId)
		}

		done()
	}
}

func TestReliableMessagesBatchedWhenPeerReadsBatchesRealDB(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		mockGetStateOk,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.sequencerManager.On("HandlePaladinMsg", mock.Anything, mock.Anything).Return()
		},
	)
	defer done()

	tm.reliableMessageResend = 1 * time.Hour

	msg := testBatchEntryMsg("from node2")
	msg.TransportVersion = transportVersionBatching
	receiveFromNode2(t, ctx, tp, msg)

	sentMessages := make(chan *prototk.PaladinMsg, 1)
	mockActivateDeactivateOk(tp)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sentMessages <- req.Message
		return nil, nil
	}

	sds := []*components.StateDistribution{
		{Domain: "domain1", ContractAddress: pldtypes.RandAddress().String(), SchemaID: pldtypes.RandHex(32), StateID: pldtypes.RandHex(32)},
		{Domain: "domain1", ContractAddress: pldtypes.RandAddress().String(), SchemaID: pldtypes.RandHex(32), StateID: pldtypes.RandHex(32)},
	}
	err := tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		for _, sd := range sds {
			if err := tm.SendReliable(ctx, dbTX, &pldapi.ReliableMessage{
				MessageType: pldapi.RMTState.Enum(),
				Node:        "node2",
				Metadata:    pldtypes.JSONString(sd),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)

	sent := <-sentMessages
	require.Equal(t, RMHMessageTypeBatch, sent.MessageType)
	decoded, err := decodeBatch(ctx, sent.Payload)
	require.NoError(t, err)
	require.Len(t, decoded, 2)
	for i, d := range decoded {
		assert.Equal(t, RMHMessageTypeStateDistribution, d.MessageType)
		var sd components.StateDistributionWithData
		require.NoError(t, json.Unmarshal(d.Payload, &sd))
		assert.Equal(t, sds[i].StateID, sd.StateID)
	}
}
