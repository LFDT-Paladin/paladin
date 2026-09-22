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

package grpctransport

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/transports/grpc/internal/msgs"
	"github.com/LFDT-Paladin/paladin/transports/grpc/pkg/proto"
	"google.golang.org/grpc"
)

var grpcNewClient = grpc.NewClient

type outboundConn struct {
	t            *grpcTransport
	nodeName     string
	conn         *grpc.ClientConn
	client       proto.PaladinGRPCTransportClient
	peerInfo     PeerInfo
	sendLock     sync.Mutex
	stream       grpc.ClientStreamingClient[proto.Message, proto.Empty]
	streamCtx    context.Context
	streamCancel context.CancelFunc
}

func (t *grpcTransport) newConnection(ctx context.Context, nodeName string, transportDetailsJSON string) (oc *outboundConn, peerInfoJSON []byte, err error) {

	// Parse the connection details
	var transportDetails PublishedTransportDetails
	err = json.Unmarshal([]byte(transportDetailsJSON), &transportDetails)
	if err == nil {
		oc = &outboundConn{
			t:        t,
			nodeName: nodeName,
			peerInfo: PeerInfo{
				Endpoint: transportDetails.Endpoint,
			},
		}
		peerInfoJSON, err = json.Marshal(&oc.peerInfo)
	}
	if err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgInvalidTransportDetails, nodeName)
	}

	// Create the gRPC connection (it's not actually connected until we use it)
	individualNodeVerifier := oc.t.peerVerifier.Clone().(*tlsVerifier)
	individualNodeVerifier.expectedNode = oc.nodeName
	oc.conn, err = grpcNewClient(transportDetails.Endpoint,
		grpc.WithTransportCredentials(individualNodeVerifier),
	)
	if err == nil {
		oc.client = proto.NewPaladinGRPCTransportClient(oc.conn)
		if err = oc.ensureStream(); err != nil {
			// The ClientConn owns background goroutines and (once dialed) a TCP connection,
			// so it must be released here or nothing else ever will.
			oc.close(ctx)
		}
	}
	if err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgConnectionFailed, transportDetails.Endpoint)
	}

	return oc, peerInfoJSON, nil
}

// close tears down the stream and the underlying ClientConn. Safe to call more than once,
// and with a nil ClientConn (which happens when grpc.NewClient itself failed).
func (oc *outboundConn) close(ctx context.Context) {
	oc.sendLock.Lock()
	defer oc.sendLock.Unlock()

	log.L(ctx).Debugf("cleaning up connection to %s", oc.nodeName)

	oc.closeStream()
	if oc.conn != nil {
		_ = oc.conn.Close()
		oc.conn = nil
	}
}

// closeStream closes the current stream (if any). Must be called with sendLock held.
func (oc *outboundConn) closeStream() {
	if oc.stream == nil {
		return
	}
	// CloseSend tells the server we have finished sending, but it does not finish the stream on
	// our side. In grpc-go a client stream stays open until its context is cancelled, the
	// ClientConn is closed, or the application reads an error from Recv. While it is open it
	// counts as an active call (so the ClientConn never goes idle) and keeps a goroutine alive.
	// We never read from the stream, so cancelling the context is what releases it.
	_ = oc.stream.CloseSend()
	oc.streamCancel()
	oc.stream = nil
	oc.streamCtx = nil
	oc.streamCancel = nil
}

// ensureStream opens a stream if there is not one already. Must be called with sendLock held
// after first creation
func (oc *outboundConn) ensureStream() error {
	if oc.stream != nil {
		return nil
	}
	log.L(oc.t.bgCtx).Infof("GRPC establishing new stream to peer %s (endpoint=%s)", oc.nodeName, oc.peerInfo.Endpoint)
	// Each stream gets its own context so that closeStream can cancel it without affecting the
	// transport or the ClientConn
	streamCtx, streamCancel := context.WithCancel(oc.t.bgCtx)
	stream, err := oc.client.ConnectSendStream(streamCtx)
	if err != nil {
		streamCancel()
		return err
	}
	oc.stream = stream
	oc.streamCtx = streamCtx
	oc.streamCancel = streamCancel
	return nil
}

func (oc *outboundConn) send(message *proto.Message) error {
	oc.sendLock.Lock()
	defer oc.sendLock.Unlock()

	err := oc.ensureStream()

	if err == nil {
		err = oc.stream.Send(message)
	}

	if err != nil {
		log.L(oc.t.bgCtx).Warnf("send failed, err %s", err)
		// Clean up the stream only - the ClientConn reconnects on its own, and the next
		// send opens a new stream over it.
		if oc.stream != nil {
			log.L(oc.t.bgCtx).Warnf("closing stream")
			oc.closeStream()
		} else {
			log.L(oc.t.bgCtx).Tracef("no stream to close")
		}
	}
	return err
}
