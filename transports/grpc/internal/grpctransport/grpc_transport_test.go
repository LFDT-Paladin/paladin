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
	"crypto/x509/pkix"
	"fmt"
	"net"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/LFDT-Paladin/paladin/transports/grpc/pkg/proto"
)

type testCallbacks struct {
	getTransportDetails func(context.Context, *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error)
	receiveMessage      func(context.Context, *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error)
}

func (tc *testCallbacks) GetTransportDetails(ctx context.Context, req *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
	return tc.getTransportDetails(ctx, req)
}

func (tc *testCallbacks) ReceiveMessage(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	return tc.receiveMessage(ctx, req)
}

func TestPluginLifecycle(t *testing.T) {
	pb := NewPlugin(context.Background())
	assert.NotNil(t, pb)
}

func TestBadConfigJSON(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{!!!!`,
	})
	assert.Regexp(t, "PD030001", err)

}

func TestMissingListenerPort(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{}`,
	})
	assert.Regexp(t, "PD030000", err)

}

func TestBadCertSubjectMatcher(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "127.0.0.1", "port": 0, "certSubjectMatcher": "[[[[[[[badness"}`,
	})
	assert.Regexp(t, "PD030003", err)

}

func TestBadTLSConf(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "127.0.0.1", "port": 0, "tls": { "caFile": "` + t.TempDir() + `" }}`,
	})
	assert.Regexp(t, "PD020401", err)

}

func TestBadDirectCertVerificationConf(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "127.0.0.1", "port": 0, "tls": { "requiredDNAttributes": {"cn":"anything"} }}`,
	})
	assert.Regexp(t, "PD030002", err)

}

func TestBadListenerConf(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "::::::::", "port": 0}`,
	})
	assert.Regexp(t, "listen", err)
}

func TestReceiveFail(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			return nil, fmt.Errorf("pop")
		}
	})
	defer done()

	// Send and we should get an error as the server fails
	var err error
	for err == nil {
		_, err = plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
			Node: "node2",
			Message: &prototk.PaladinMsg{
				Component: prototk.PaladinMsg_TRANSACTION_ENGINE,
			},
		})
	}
	assert.Error(t, err)

}

func TestConnectFail(t *testing.T) {

	ctx := context.Background()

	plugin1, plugin2, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			require.Equal(t, "node1", rmr.FromNode)
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	oc := plugin1.getConnection("node2")
	require.NotNil(t, oc)
	streamCtx := oc.streamCtx
	require.NotNil(t, streamCtx)

	plugin2.grpcServer.Stop()

	// gRPC does not guarantee we get the error immediately
	var err error
	for err == nil {
		_, err = plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
			Node: "node2",
			Message: &prototk.PaladinMsg{
				Component: prototk.PaladinMsg_TRANSACTION_ENGINE,
			},
		})
	}
	assert.Error(t, err)

	// The failed stream is finished (its context cancelled), but the ClientConn is kept
	// so the next send can open a new stream over it once the peer is back
	require.Error(t, streamCtx.Err())
	require.Nil(t, oc.stream)
	require.NotNil(t, oc.conn)
	require.NotEqual(t, connectivity.Shutdown, oc.conn.GetState())

}

func TestSendNotActivated(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	_, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Node: "node3",
		Message: &prototk.PaladinMsg{
			Component: prototk.PaladinMsg_TRANSACTION_ENGINE,
		},
	})
	assert.Regexp(t, "PD030016", err)

}

func TestActivateBadTransportDetails(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: `{"endpoint": false}`,
	})
	assert.Regexp(t, "PD030014", err)

}

func TestConnectBadTransport(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t)
	defer done()

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: `{"endpoint": "WRONG:::::::"}`,
	})
	assert.Regexp(t, "WRONG", err)

}

func TestConnectSendStreamBadSecurityCtx(t *testing.T) {

	plugin, _, _, done := newTestGRPCTransport(t, "", "", &Config{})
	defer done()

	// Create an unsecured server to the plugin using an unsecured server,
	// and check that the stream loop closes rather than accepting messages.
	unsecuredServer := grpc.NewServer()
	proto.RegisterPaladinGRPCTransportServer(unsecuredServer, plugin)

	serverDone := make(chan struct{})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		defer close(serverDone)
		_ = unsecuredServer.Serve(l)
	}()

	conn, err := grpc.NewClient("dns:///"+l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	client := proto.NewPaladinGRPCTransportClient(conn)
	s, err := client.ConnectSendStream(context.Background())
	require.NoError(t, err)

	for err == nil {
		err = s.Send(&proto.Message{
			Component: int32(prototk.PaladinMsg_TRANSACTION_ENGINE),
		})
	}
	assert.Error(t, err)
}

func TestStopTransportIsIdempotentAndReleasesListener(t *testing.T) {
	nodeCert, nodeKey := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin, _, _, done := newTestGRPCTransport(t, nodeCert, nodeKey, &Config{})
	defer done()

	listenAddr := plugin.listener.Addr().String()

	_, err := plugin.StopTransport(context.Background(), &prototk.StopTransportRequest{})
	require.NoError(t, err)
	_, err = plugin.StopTransport(context.Background(), &prototk.StopTransportRequest{})
	require.NoError(t, err)

	reboundListener, err := net.Listen("tcp", listenAddr)
	require.NoError(t, err)
	_ = reboundListener.Close()
}

func TestDeactivatePeerClosesClientConn(t *testing.T) {
	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	// The connection and its stream are live after activation
	oc1 := plugin1.getConnection("node2")
	require.NotNil(t, oc1)
	conn1 := oc1.conn
	require.NotNil(t, conn1)
	require.NotEqual(t, connectivity.Shutdown, conn1.GetState())
	streamCtx1 := oc1.streamCtx
	require.NotNil(t, streamCtx1)
	require.NoError(t, streamCtx1.Err())

	// Deactivate: the entry goes away, the stream is finished, and the ClientConn is shut down
	_, err := plugin1.DeactivatePeer(ctx, &prototk.DeactivatePeerRequest{NodeName: "node2"})
	require.NoError(t, err)
	require.Nil(t, plugin1.getConnection("node2"))
	require.Error(t, streamCtx1.Err())
	require.Nil(t, oc1.stream)
	require.Equal(t, connectivity.Shutdown, conn1.GetState())

	// close is safe to call again on an already-closed connection
	oc1.close(ctx)

	// Deactivating a peer that is not active is a no-op
	_, err = plugin1.DeactivatePeer(ctx, &prototk.DeactivatePeerRequest{NodeName: "node2"})
	require.NoError(t, err)

	// Re-activate: a fresh ClientConn that works
	details := pldtypes.JSONString(&PublishedTransportDetails{Endpoint: oc1.peerInfo.Endpoint}).Pretty()
	_, err = plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{NodeName: "node2", TransportDetails: details})
	require.NoError(t, err)
	oc2 := plugin1.getConnection("node2")
	require.NotNil(t, oc2)
	conn2 := oc2.conn
	require.NotNil(t, conn2)
	require.NotSame(t, conn1, conn2)
	require.NotEqual(t, connectivity.Shutdown, conn2.GetState())
	_, err = plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Node:    "node2",
		Message: &prototk.PaladinMsg{Component: prototk.PaladinMsg_TRANSACTION_ENGINE},
	})
	require.NoError(t, err)

	// Activate again while active: the replaced ClientConn is shut down, the new one is live
	_, err = plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{NodeName: "node2", TransportDetails: details})
	require.NoError(t, err)
	oc3 := plugin1.getConnection("node2")
	require.NotNil(t, oc3)
	require.NotSame(t, oc2, oc3)
	require.Equal(t, connectivity.Shutdown, conn2.GetState())
	require.NotEqual(t, connectivity.Shutdown, oc3.conn.GetState())
}

func TestStopTransportClosesClientConns(t *testing.T) {
	plugin1, _, done := newSuccessfulVerifiedConnection(t)
	defer done()

	oc := plugin1.getConnection("node2")
	require.NotNil(t, oc)
	conn := oc.conn
	require.NotNil(t, conn)

	_, err := plugin1.StopTransport(context.Background(), &prototk.StopTransportRequest{})
	require.NoError(t, err)
	require.Equal(t, connectivity.Shutdown, conn.GetState())
}

func TestActivatePeerStreamFailureClosesClientConn(t *testing.T) {
	ctx := context.Background()

	nodeCert, nodeKey := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin, _, _, done := newTestGRPCTransport(t, nodeCert, nodeKey, &Config{})
	defer done()

	// Capture the ClientConn that newConnection builds
	var captured *grpc.ClientConn
	origNewClient := grpcNewClient
	grpcNewClient = func(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
		conn, err := origNewClient(target, opts...)
		captured = conn
		return conn, err
	}
	defer func() { grpcNewClient = origNewClient }()

	// Point at a port nothing is listening on, so opening the stream fails
	closedListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedAddr := closedListener.Addr().String()
	require.NoError(t, closedListener.Close())

	_, err = plugin.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: `{"endpoint":"dns:///` + closedAddr + `"}`,
	})
	require.Regexp(t, "PD030015", err)

	// No entry is left behind, and the ClientConn was released
	require.Nil(t, plugin.getConnection("node2"))
	require.NotNil(t, captured)
	require.Equal(t, connectivity.Shutdown, captured.GetState())
}
