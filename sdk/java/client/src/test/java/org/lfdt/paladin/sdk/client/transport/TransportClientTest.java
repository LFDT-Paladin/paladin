/*
 * Copyright contributors to Paladin, an LFDT project
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
package org.lfdt.paladin.sdk.client.transport;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import java.io.IOException;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.config.RetryPolicy;
import org.lfdt.paladin.sdk.client.config.RpcClientConfig;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.rpc.HttpRpcClient;
import org.lfdt.paladin.sdk.client.rpc.MockJsonRpcServer;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.transport.PeerInfo;
import org.lfdt.paladin.sdk.core.transport.ReliableMessage;
import org.lfdt.paladin.sdk.core.transport.ReliableMessageAck;
import org.lfdt.paladin.sdk.core.transport.ReliableMessageType;

class TransportClientTest {

  private static final String PEER_JSON =
      "{\"name\":\"node2\",\"activated\":\"2024-01-01T00:00:00Z\","
          + "\"outboundTransport\":\"grpc\",\"outbound\":{\"endpoint\":\"dns:node2:9000\"},"
          + "\"stats\":{\"sentMsgs\":5,\"receivedMsgs\":3,\"sentBytes\":100,\"receivedBytes\":80,"
          + "\"reliableHighestSent\":5,\"reliableAckBase\":2}}";

  private static final String RELIABLE_MSG_JSON =
      "{\"sequence\":7,\"id\":\"3b8c1a2e-0000-0000-0000-000000000001\","
          + "\"created\":\"2024-01-01T00:00:00Z\",\"node\":\"node2\",\"messageType\":\"state\","
          + "\"metadata\":{\"foo\":\"bar\"},"
          + "\"ack\":{\"time\":\"2024-01-01T00:00:01Z\",\"error\":\"\"}}";

  private static final String ACK_JSON =
      "{\"messageId\":\"3b8c1a2e-0000-0000-0000-000000000001\","
          + "\"time\":\"2024-01-01T00:00:01Z\",\"error\":\"boom\"}";

  private static String success(final String resultJson) {
    return "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"result\":" + resultJson + "}";
  }

  private RpcClientConfig config(final String url) {
    return RpcClientConfig.builder(url)
        .connectTimeout(Duration.ofSeconds(5))
        .requestTimeout(Duration.ofSeconds(5))
        .retryPolicy(
            RetryPolicy.builder()
                .maxAttempts(1)
                .initialDelay(Duration.ofMillis(1))
                .maxDelay(Duration.ofMillis(5))
                .build())
        .build();
  }

  @Test
  void nodeName() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"node1\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      assertEquals("node1", new TransportClient(rpc).nodeName().join());
      assertEquals("transport_nodeName", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void localTransports() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[\"grpc\"]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      assertEquals(List.of("grpc"), new TransportClient(rpc).localTransports().join());
      assertEquals("transport_localTransports", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void localTransportDetails() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"dns:node1:9000\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      assertEquals("dns:node1:9000", new TransportClient(rpc).localTransportDetails("grpc").join());
      final JsonNode req = server.requests().get(0);
      assertEquals("transport_localTransportDetails", req.get("method").asText());
      assertEquals("grpc", req.get("params").get(0).asText());
    }
  }

  @Test
  void peers() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + PEER_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<PeerInfo> peers = new TransportClient(rpc).peers().join();
      assertEquals(1, peers.size());
      final PeerInfo peer = peers.get(0);
      assertEquals("node2", peer.name());
      assertEquals("grpc", peer.outboundTransport());
      assertEquals("dns:node2:9000", peer.outbound().get("endpoint"));
      assertEquals(5L, peer.stats().sentMsgs());
      assertEquals(80L, peer.stats().receivedBytes());
      // An absent timestamp deserializes to the zero timestamp, not null.
      assertTrue(peer.stats().createdAt().isZero());
      assertEquals("transport_peers", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void peerInfo() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(PEER_JSON)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PeerInfo peer = new TransportClient(rpc).peerInfo("node2").join();
      assertEquals("node2", peer.name());
      final JsonNode req = server.requests().get(0);
      assertEquals("transport_peerInfo", req.get("method").asText());
      assertEquals("node2", req.get("params").get(0).asText());
    }
  }

  @Test
  void queryReliableMessages() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(200, success("[" + RELIABLE_MSG_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<ReliableMessage> msgs =
          new TransportClient(rpc)
              .queryReliableMessages(QueryJSON.builder().limit(5).build())
              .join();
      assertEquals(1, msgs.size());
      final ReliableMessage msg = msgs.get(0);
      assertEquals(7L, msg.sequence());
      assertEquals("node2", msg.node());
      assertEquals(ReliableMessageType.STATE, msg.messageType());
      assertEquals("bar", msg.metadata().get("foo").asText());
      assertEquals("", msg.ack().error());
      assertEquals(
          "transport_queryReliableMessages", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void queryReliableMessageAcks() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + ACK_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<ReliableMessageAck> acks =
          new TransportClient(rpc).queryReliableMessageAcks(QueryJSON.builder().build()).join();
      assertEquals(1, acks.size());
      assertEquals("boom", acks.get(0).error());
      assertEquals(
          "transport_queryReliableMessageAcks", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void propagatesRpcError() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(
                        200,
                        "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"error\":{\"code\":-32000,"
                            + "\"message\":\"no such peer\"}}"));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final CompletionException ex =
          assertThrows(
              CompletionException.class, () -> new TransportClient(rpc).peerInfo("nope").join());
      assertInstanceOf(PaladinRpcException.class, ex.getCause());
    }
  }
}
