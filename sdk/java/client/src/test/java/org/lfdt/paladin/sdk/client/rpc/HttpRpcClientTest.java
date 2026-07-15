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
package org.lfdt.paladin.sdk.client.rpc;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import java.io.IOException;
import java.net.ServerSocket;
import java.time.Duration;
import java.util.Map;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.config.RetryPolicy;
import org.lfdt.paladin.sdk.client.config.RpcClientConfig;
import org.lfdt.paladin.sdk.client.exception.PaladinConnectionException;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.exception.PaladinTimeoutException;

class HttpRpcClientTest {

  private static String success(final String resultJson) {
    return "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"result\":" + resultJson + "}";
  }

  private static String error(final long code, final String message, final String dataJson) {
    final String data = dataJson == null ? "" : ",\"data\":" + dataJson;
    return "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"error\":{\"code\":"
        + code
        + ",\"message\":\""
        + message
        + "\""
        + data
        + "}}";
  }

  private RpcClientConfig config(final String url, final int maxAttempts) {
    return RpcClientConfig.builder(url)
        .connectTimeout(Duration.ofSeconds(5))
        .requestTimeout(Duration.ofSeconds(5))
        .retryPolicy(
            RetryPolicy.builder()
                .maxAttempts(maxAttempts)
                .initialDelay(Duration.ofMillis(1))
                .maxDelay(Duration.ofMillis(5))
                .build())
        .build();
  }

  @Test
  void deserializesTypedResult() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("{\"value\":\"0x1f\"}")));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      final JsonNode result = client.callRpc(JsonNode.class, "ptx_call", "a", 1).join();
      assertEquals("0x1f", result.get("value").asText());
      // The request reached the server as a well-formed JSON-RPC envelope.
      final JsonNode request = server.requests().get(0);
      assertEquals("2.0", request.get("jsonrpc").asText());
      assertEquals("ptx_call", request.get("method").asText());
      assertEquals("000000001", request.get("id").asText());
      assertEquals(2, request.get("params").size());
    }
  }

  @Test
  void deserializesGenericResultViaTypeReference() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("{\"a\":1,\"b\":2}")));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      final Map<String, Object> result =
          client.callRpc(new TypeReference<Map<String, Object>>() {}, "m").join();
      assertEquals(2, result.size());
    }
  }

  @Test
  void nullResultDeserializesToNull() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success("null")));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      assertNull(client.callRpc(JsonNode.class, "m").join());
    }
  }

  @Test
  void businessErrorOn200IsSurfacedAndNotRetried() throws IOException {
    // Paladin #1183 makes the node JSON-RPC-spec compliant: application errors are returned as HTTP
    // 200 with a JSON-RPC error body (issue #1001 exercised the idempotency-key clash, PD012220,
    // returned with RPCCodeConflict -32001). The transport must surface the error from the error
    // member regardless of the 200 status, and must not retry it.
    final String body =
        error(
            // -32001 == RPCCodeConflict; the JsonRpcErrorCode.CONFLICT constant is deferred to the
            // TxBuilder work (see sdk/java/CLAUDE.md), so the raw code is used here.
            -32001,
            "PD012220: idempotencyKey already used by submitted transaction mint-cash=abc",
            "{\"hint\":\"idempotencyKey\"}");
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, body));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      final CompletionException ex =
          assertThrows(
              CompletionException.class,
              () -> client.callRpc(JsonNode.class, "ptx_sendTransaction").join());
      final PaladinRpcException rpc = assertInstanceOf(PaladinRpcException.class, ex.getCause());
      assertEquals(-32001, rpc.code());
      assertEquals(200, rpc.httpStatus());
      assertTrue(rpc.getMessage().contains("PD012220"));
      assertTrue(rpc.data().isPresent());
      // A JSON-RPC application error on HTTP 200 is not retried.
      assertEquals(1, server.requestCount());
    }
  }

  @Test
  void authErrorStatusIsNotRetried() throws IOException {
    // Paladin #1183 returns auth failures as HTTP 403. As a 4xx status it must not be retried.
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(403, "forbidden"));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      final CompletionException ex =
          assertThrows(CompletionException.class, () -> client.callRpc(JsonNode.class, "m").join());
      final PaladinRpcException rpc = assertInstanceOf(PaladinRpcException.class, ex.getCause());
      assertEquals(403, rpc.httpStatus());
      assertEquals(1, server.requestCount());
    }
  }

  @Test
  void errorBodyTakesPrecedenceOverStatus() throws IOException {
    // Paladin #1183: even on a non-2xx status, when the body carries a JSON-RPC error the transport
    // surfaces that error's code/message rather than a bare "HTTP request failed with status".
    final String body = error(-32000, "PD020000: request is not authorized", null);
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(403, body));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 1))) {
      final CompletionException ex =
          assertThrows(CompletionException.class, () -> client.callRpc(JsonNode.class, "m").join());
      final PaladinRpcException rpc = assertInstanceOf(PaladinRpcException.class, ex.getCause());
      assertEquals(-32000, rpc.code());
      assertTrue(rpc.getMessage().contains("not authorized"));
      assertEquals(403, rpc.httpStatus());
    }
  }

  @Test
  void nonRetryableHttpErrorBecomesPaladinRpcException() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(400, "bad request"));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      final CompletionException ex =
          assertThrows(CompletionException.class, () -> client.callRpc(JsonNode.class, "m").join());
      final PaladinRpcException rpc = assertInstanceOf(PaladinRpcException.class, ex.getCause());
      assertEquals(400, rpc.httpStatus());
      // 4xx is not retried.
      assertEquals(1, server.requestCount());
    }
  }

  @Test
  void retriesTransientStatusThenSucceeds() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    n == 1
                        ? MockJsonRpcServer.Response.of(503, "unavailable")
                        : MockJsonRpcServer.Response.of(200, success("\"ok\"")));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      final String result = client.callRpc(String.class, "m").join();
      assertEquals("ok", result);
      assertEquals(2, server.requestCount());
    }
  }

  @Test
  void retriesTooManyRequestsStatus() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    n == 1
                        ? MockJsonRpcServer.Response.of(429, "slow down")
                        : MockJsonRpcServer.Response.of(200, success("\"ok\"")));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      assertEquals("ok", client.callRpc(String.class, "m").join());
      assertEquals(2, server.requestCount());
    }
  }

  @Test
  void retryExhaustionSurfacesLastError() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(503, "unavailable"));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      final CompletionException ex =
          assertThrows(CompletionException.class, () -> client.callRpc(JsonNode.class, "m").join());
      final PaladinRpcException rpc = assertInstanceOf(PaladinRpcException.class, ex.getCause());
      assertEquals(503, rpc.httpStatus());
      assertEquals(3, server.requestCount());
    }
  }

  @Test
  void requestTimeoutBecomesPaladinTimeoutException() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(200, success("\"slow\"")).withDelayMillis(1000));
        HttpRpcClient client =
            new HttpRpcClient(
                RpcClientConfig.builder(server.baseUrl())
                    .requestTimeout(Duration.ofMillis(150))
                    .retryPolicy(RetryPolicy.builder().maxAttempts(1).build())
                    .build())) {
      final CompletionException ex =
          assertThrows(CompletionException.class, () -> client.callRpc(JsonNode.class, "m").join());
      assertInstanceOf(PaladinTimeoutException.class, ex.getCause());
    }
  }

  @Test
  void connectionRefusedBecomesPaladinConnectionException() throws IOException {
    final int closedPort;
    try (ServerSocket socket = new ServerSocket(0)) {
      closedPort = socket.getLocalPort();
    }
    try (HttpRpcClient client =
        new HttpRpcClient(
            RpcClientConfig.builder("http://127.0.0.1:" + closedPort)
                .connectTimeout(Duration.ofSeconds(2))
                .retryPolicy(RetryPolicy.builder().maxAttempts(1).build())
                .build())) {
      final CompletionException ex =
          assertThrows(CompletionException.class, () -> client.callRpc(JsonNode.class, "m").join());
      assertInstanceOf(PaladinConnectionException.class, ex.getCause());
    }
  }

  @Test
  void unparseableBodyOnSuccessStatusBecomesConnectionException() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, "not json at all"));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 1))) {
      final CompletionException ex =
          assertThrows(CompletionException.class, () -> client.callRpc(JsonNode.class, "m").join());
      assertInstanceOf(PaladinConnectionException.class, ex.getCause());
    }
  }

  @Test
  void configuredHeadersAreSent() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"ok\"")));
        HttpRpcClient client =
            new HttpRpcClient(
                RpcClientConfig.builder(server.baseUrl())
                    .header("Authorization", "Bearer secret")
                    .build())) {
      client.callRpc(String.class, "m").join();
      assertEquals("Bearer secret", server.requestHeader("Authorization"));
    }
  }

  @Test
  void unserializableParamFailsFast() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"ok\"")));
        HttpRpcClient client = new HttpRpcClient(config(server.baseUrl(), 3))) {
      final CompletionException ex =
          assertThrows(
              CompletionException.class,
              () -> client.callRpc(JsonNode.class, "m", new Unserializable()).join());
      final PaladinRpcException rpc = assertInstanceOf(PaladinRpcException.class, ex.getCause());
      assertEquals(JsonRpcErrorCode.INVALID_REQUEST, rpc.code());
      // The request was never sent.
      assertEquals(0, server.requestCount());
    }
  }

  /** A bean whose getter throws, so Jackson fails to serialize it. */
  static final class Unserializable {
    public String getBoom() {
      throw new IllegalStateException("cannot serialize");
    }
  }
}
