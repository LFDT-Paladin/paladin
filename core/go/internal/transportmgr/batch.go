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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// RMHMessageTypeBatch carries several messages for one peer in a single transport message. Its
// payload is each message's protobuf encoding, length prefixed.
const RMHMessageTypeBatch = "batch"

type batchEntry struct {
	msg *prototk.PaladinMsg
	seq *uint64
}

type batchToSend struct {
	msg *prototk.PaladinMsg
	// seq is the highest reliable sequence in the batch, nil when it carries none.
	seq *uint64
	// count is how many of the entries, in order, the batch carries.
	count int
}

// buildBatches packs the entries, in order, into as few messages as the batch limits allow. A
// batch of one is sent as that message unwrapped, as is every entry when the peer cannot read
// batches.
func (tm *transportManager) buildBatches(entries []batchEntry, peerReadsBatches bool) []batchToSend {
	var batches []batchToSend
	if !peerReadsBatches {
		for _, e := range entries {
			batches = append(batches, batchToSend{msg: e.msg, seq: e.seq, count: 1})
		}
		return batches
	}
	// Sizing first lets each payload be allocated once and every message encoded straight into it.
	// The encode then reuses the size the sizing pass cached on the message.
	encode := proto.MarshalOptions{UseCachedSize: true}
	for start := 0; start < len(entries); {
		end, payloadLen := start, 0
		for ; end < len(entries) && end-start < tm.sendBatchMaxSize; end++ {
			entryLen := protowire.SizeBytes(proto.Size(entries[end].msg))
			if end > start && payloadLen+entryLen > tm.sendBatchMaxBytes {
				break
			}
			payloadLen += entryLen
		}
		var payload []byte
		var seq *uint64
		if end-start > 1 {
			payload = make([]byte, 0, payloadLen)
			for i := start; i < end; i++ {
				msgStart := len(payload)
				payload = protowire.AppendVarint(payload, uint64(encode.Size(entries[i].msg)))
				var err error
				if payload, err = encode.MarshalAppend(payload, entries[i].msg); err != nil {
					// A message that cannot be encoded goes on its own, so its send fails as it would unbatched.
					payload = payload[:msgStart]
					end = max(i, start+1)
					break
				}
				if s := entries[i].seq; s != nil && (seq == nil || *s > *seq) {
					seq = s
				}
			}
		}
		if end-start == 1 {
			batches = append(batches, batchToSend{msg: entries[start].msg, seq: entries[start].seq, count: 1})
		} else {
			batches = append(batches, batchToSend{
				msg: &prototk.PaladinMsg{
					MessageId:   uuid.NewString(),
					Component:   prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
					MessageType: RMHMessageTypeBatch,
					Payload:     payload,
				},
				seq:   seq,
				count: end - start,
			})
		}
		start = end
	}
	return batches
}

func decodeBatch(ctx context.Context, payload []byte) ([]*prototk.PaladinMsg, error) {
	var batch []*prototk.PaladinMsg
	for len(payload) > 0 {
		b, n := protowire.ConsumeBytes(payload)
		if n < 0 {
			return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
		}
		msg := &prototk.PaladinMsg{}
		if err := proto.Unmarshal(b, msg); err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessage)
		}
		batch = append(batch, msg)
		payload = payload[n:]
	}
	return batch, nil
}
