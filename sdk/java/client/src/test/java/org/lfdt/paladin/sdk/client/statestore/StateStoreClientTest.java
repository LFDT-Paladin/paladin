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
package org.lfdt.paladin.sdk.client.statestore;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;

import com.fasterxml.jackson.databind.JsonNode;
import java.io.IOException;
import java.time.Duration;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.config.RetryPolicy;
import org.lfdt.paladin.sdk.client.config.RpcClientConfig;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.rpc.HttpRpcClient;
import org.lfdt.paladin.sdk.client.rpc.MockJsonRpcServer;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.statestore.Schema;
import org.lfdt.paladin.sdk.core.statestore.State;
import org.lfdt.paladin.sdk.core.statestore.StateStatusQualifier;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class StateStoreClientTest {

  private static final String SCHEMA_ID =
      "0x1111111111111111111111111111111111111111111111111111111111111111";
  private static final String CONTRACT = "0x2222222222222222222222222222222222222222";

  private static final String SCHEMA_JSON =
      "{\"id\":\""
          + SCHEMA_ID
          + "\",\"created\":\"2024-01-01T00:00:00Z\",\"domain\":\"noto\",\"type\":\"abi\","
          + "\"signature\":\"type=Coin\",\"definition\":{\"type\":\"tuple\"},"
          + "\"labels\":[\"owner\"]}";

  private static final String STATE_JSON =
      "{\"id\":\"0xaa\",\"created\":\"2024-01-01T00:00:00Z\",\"domain\":\"noto\",\"schema\":\""
          + SCHEMA_ID
          + "\",\"contractAddress\":\""
          + CONTRACT
          + "\",\"data\":{\"amount\":\"100\"},"
          + "\"confirmed\":{\"transaction\":\"3b8c1a2e-0000-0000-0000-000000000001\"}}";

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
  void listSchemas() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + SCHEMA_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<Schema> schemas = new StateStoreClient(rpc).listSchemas("noto").join();
      assertEquals(1, schemas.size());
      assertEquals("noto", schemas.get(0).domain());
      assertEquals("type=Coin", schemas.get(0).signature());
      final JsonNode req = server.requests().get(0);
      assertEquals("pstate_listSchemas", req.get("method").asText());
      assertEquals("noto", req.get("params").get(0).asText());
    }
  }

  @Test
  void storeState() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(STATE_JSON)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final JsonNode data = PaladinObjectMapper.shared().readTree("{\"amount\":\"100\"}");
      final State state =
          new StateStoreClient(rpc)
              .storeState(
                  "noto", EthAddress.fromString(CONTRACT), Bytes32.fromString(SCHEMA_ID), data)
              .join();
      assertEquals(HexBytes.fromString("0xaa"), state.id());
      assertEquals("100", state.data().get("amount").asText());
      assertEquals(
          UUID.fromString("3b8c1a2e-0000-0000-0000-000000000001"), state.confirmed().transaction());

      final JsonNode req = server.requests().get(0);
      assertEquals("pstate_storeState", req.get("method").asText());
      assertEquals("noto", req.get("params").get(0).asText());
      assertEquals(CONTRACT, req.get("params").get(1).asText());
      assertEquals(SCHEMA_ID, req.get("params").get(2).asText());
    }
  }

  @Test
  void queryStatesSendsQualifier() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + STATE_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<State> states =
          new StateStoreClient(rpc)
              .queryStates(
                  "noto",
                  Bytes32.fromString(SCHEMA_ID),
                  QueryJSON.builder().limit(5).build(),
                  StateStatusQualifier.AVAILABLE)
              .join();
      assertEquals(1, states.size());

      final JsonNode req = server.requests().get(0);
      assertEquals("pstate_queryStates", req.get("method").asText());
      // Unlike the Go client, the qualifier is always sent as the fourth parameter.
      assertEquals(4, req.get("params").size());
      assertEquals("noto", req.get("params").get(0).asText());
      assertEquals(SCHEMA_ID, req.get("params").get(1).asText());
      assertEquals("available", req.get("params").get(3).asText());
    }
  }

  @Test
  void queryContractStates() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + STATE_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<State> states =
          new StateStoreClient(rpc)
              .queryContractStates(
                  "noto",
                  EthAddress.fromString(CONTRACT),
                  Bytes32.fromString(SCHEMA_ID),
                  QueryJSON.builder().build(),
                  StateStatusQualifier.CONFIRMED)
              .join();
      assertEquals(1, states.size());

      final JsonNode req = server.requests().get(0);
      assertEquals("pstate_queryContractStates", req.get("method").asText());
      assertEquals(5, req.get("params").size());
      assertEquals(CONTRACT, req.get("params").get(1).asText());
      assertEquals("confirmed", req.get("params").get(4).asText());
    }
  }

  @Test
  void queryNullifiers() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + STATE_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final UUID txId = UUID.fromString("3b8c1a2e-0000-0000-0000-000000000009");
      final List<State> states =
          new StateStoreClient(rpc)
              .queryNullifiers(
                  "noto",
                  Bytes32.fromString(SCHEMA_ID),
                  QueryJSON.builder().build(),
                  StateStatusQualifier.forTransaction(txId))
              .join();
      assertEquals(1, states.size());

      final JsonNode req = server.requests().get(0);
      assertEquals("pstate_queryNullifiers", req.get("method").asText());
      assertEquals(4, req.get("params").size());
      assertEquals(txId.toString(), req.get("params").get(3).asText());
    }
  }

  @Test
  void queryContractNullifiersSendsQualifier() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + STATE_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<State> states =
          new StateStoreClient(rpc)
              .queryContractNullifiers(
                  "noto",
                  EthAddress.fromString(CONTRACT),
                  Bytes32.fromString(SCHEMA_ID),
                  QueryJSON.builder().build(),
                  StateStatusQualifier.ALL)
              .join();
      assertEquals(1, states.size());

      final JsonNode req = server.requests().get(0);
      assertEquals("pstate_queryContractNullifiers", req.get("method").asText());
      // Unlike the Go client, the qualifier is always sent as the fifth parameter.
      assertEquals(5, req.get("params").size());
      assertEquals("all", req.get("params").get(4).asText());
    }
  }

  @Test
  void transferPrivateState() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(
                        200, success("\"3b8c1a2e-0000-0000-0000-000000000002\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final UUID msgId =
          new StateStoreClient(rpc)
              .transferPrivateState("noto", HexBytes.fromString("0xaa"), "recipient@node2")
              .join();
      assertEquals(UUID.fromString("3b8c1a2e-0000-0000-0000-000000000002"), msgId);

      final JsonNode req = server.requests().get(0);
      assertEquals("pstate_transferPrivateState", req.get("method").asText());
      assertEquals("noto", req.get("params").get(0).asText());
      assertEquals("0xaa", req.get("params").get(1).asText());
      assertEquals("recipient@node2", req.get("params").get(2).asText());
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
                            + "\"message\":\"no such schema\"}}"));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final CompletionException ex =
          assertThrows(
              CompletionException.class,
              () -> new StateStoreClient(rpc).listSchemas("noto").join());
      assertInstanceOf(PaladinRpcException.class, ex.getCause());
    }
  }
}
