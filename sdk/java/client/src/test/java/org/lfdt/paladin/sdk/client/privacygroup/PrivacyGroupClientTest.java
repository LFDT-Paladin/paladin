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
package org.lfdt.paladin.sdk.client.privacygroup;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
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
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroup;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupEVMCall;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupEVMTXInput;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupInput;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessage;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessageInput;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessageListener;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessageListenerFilters;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessageListenerOptions;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class PrivacyGroupClientTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  private static final String GROUP_ID = "0xfeed";
  private static final String ADDRESS = "0x1234567890123456789012345678901234567890";

  private static final String GROUP_JSON =
      "{\"id\":\""
          + GROUP_ID
          + "\",\"domain\":\"pente\",\"name\":\"g1\",\"members\":[\"me@node1\",\"you@node2\"],"
          + "\"contractAddress\":\""
          + ADDRESS
          + "\"}";

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

  private MockJsonRpcServer serverReturning(final String resultJson) throws IOException {
    return new MockJsonRpcServer(
        (n, req) -> MockJsonRpcServer.Response.of(200, success(resultJson)));
  }

  // ---- groups -------------------------------------------------------------

  @Test
  void createGroup() throws IOException {
    try (MockJsonRpcServer server = serverReturning(GROUP_JSON);
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroupInput spec =
          PrivacyGroupInput.builder("pente").name("g1").member("me@node1").build();

      final PrivacyGroup group = new PrivacyGroupClient(rpc).createGroup(spec).join();

      assertEquals(HexBytes.fromString(GROUP_ID), group.id());
      assertEquals("g1", group.name());
      assertEquals(List.of("me@node1", "you@node2"), group.members());
      assertEquals(EthAddress.fromString(ADDRESS), group.contractAddress());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_createGroup", req.get("method").asText());
      assertEquals(1, req.get("params").size());
      assertEquals("pente", req.get("params").get(0).get("domain").asText());
    }
  }

  @Test
  void getGroupById() throws IOException {
    try (MockJsonRpcServer server = serverReturning(GROUP_JSON);
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroup group =
          new PrivacyGroupClient(rpc).getGroupById("pente", HexBytes.fromString(GROUP_ID)).join();

      assertEquals("pente", group.domain());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_getGroupById", req.get("method").asText());
      final JsonNode params = req.get("params");
      assertEquals(2, params.size());
      assertEquals("pente", params.get(0).asText());
      assertEquals(GROUP_ID, params.get(1).asText());
    }
  }

  @Test
  void getGroupByAddress() throws IOException {
    try (MockJsonRpcServer server = serverReturning(GROUP_JSON);
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroup group =
          new PrivacyGroupClient(rpc).getGroupByAddress(EthAddress.fromString(ADDRESS)).join();

      assertEquals(EthAddress.fromString(ADDRESS), group.contractAddress());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_getGroupByAddress", req.get("method").asText());
      assertEquals(1, req.get("params").size());
      assertEquals(ADDRESS, req.get("params").get(0).asText());
    }
  }

  @Test
  void queryGroups() throws IOException {
    try (MockJsonRpcServer server = serverReturning("[" + GROUP_JSON + "]");
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final QueryJSON query = QueryJSON.builder().limit(10).equal("domain", "pente").build();

      final List<PrivacyGroup> groups = new PrivacyGroupClient(rpc).queryGroups(query).join();

      assertEquals(1, groups.size());
      assertEquals("g1", groups.get(0).name());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_queryGroups", req.get("method").asText());
      assertEquals(1, req.get("params").size());
      assertEquals(10, req.get("params").get(0).get("limit").asInt());
    }
  }

  @Test
  void queryGroupsWithMemberSendsMemberBeforeQuery() throws IOException {
    try (MockJsonRpcServer server = serverReturning("[" + GROUP_JSON + "]");
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final QueryJSON query = QueryJSON.builder().limit(5).build();

      final List<PrivacyGroup> groups =
          new PrivacyGroupClient(rpc).queryGroupsWithMember("me@node1", query).join();

      assertEquals(1, groups.size());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_queryGroupsWithMember", req.get("method").asText());
      final JsonNode params = req.get("params");
      assertEquals(2, params.size());
      assertEquals("me@node1", params.get(0).asText());
      assertEquals(5, params.get(1).get("limit").asInt());
    }
  }

  // ---- transactions and calls ---------------------------------------------

  @Test
  void sendTransaction() throws IOException {
    final UUID txId = UUID.randomUUID();
    try (MockJsonRpcServer server = serverReturning("\"" + txId + "\"");
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroupEVMTXInput tx =
          PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID))
              .from("me@node1")
              .to(EthAddress.fromString(ADDRESS))
              .input(MAPPER.readTree("{\"amount\":\"10\"}"))
              .build();

      assertEquals(txId, new PrivacyGroupClient(rpc).sendTransaction(tx).join());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_sendTransaction", req.get("method").asText());
      assertEquals(1, req.get("params").size());
      final JsonNode sent = req.get("params").get(0);
      assertEquals(GROUP_ID, sent.get("group").asText());
      assertEquals("me@node1", sent.get("from").asText());
    }
  }

  @Test
  void call() throws IOException {
    try (MockJsonRpcServer server = serverReturning("{\"balance\":\"100\"}");
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroupEVMCall evmCall =
          PrivacyGroupEVMCall.builder("pente", HexBytes.fromString(GROUP_ID))
              .from("me@node1")
              .to(EthAddress.fromString(ADDRESS))
              .block("latest")
              .build();

      final JsonNode data = new PrivacyGroupClient(rpc).call(evmCall).join();

      assertEquals("100", data.get("balance").asText());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_call", req.get("method").asText());
      assertEquals("latest", req.get("params").get(0).get("block").asText());
    }
  }

  // ---- messages -----------------------------------------------------------

  @Test
  void sendMessage() throws IOException {
    final UUID msgId = UUID.randomUUID();
    try (MockJsonRpcServer server = serverReturning("\"" + msgId + "\"");
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroupMessageInput message =
          PrivacyGroupMessageInput.builder("pente", HexBytes.fromString(GROUP_ID))
              .topic("orders")
              .data(MAPPER.readTree("{\"hello\":\"world\"}"))
              .build();

      assertEquals(msgId, new PrivacyGroupClient(rpc).sendMessage(message).join());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_sendMessage", req.get("method").asText());
      final JsonNode sent = req.get("params").get(0);
      assertEquals("orders", sent.get("topic").asText());
      assertEquals("world", sent.get("data").get("hello").asText());
    }
  }

  @Test
  void getMessageById() throws IOException {
    final UUID msgId = UUID.randomUUID();
    final String messageJson =
        "{\"id\":\""
            + msgId
            + "\",\"localSequence\":7,\"node\":\"node2\",\"domain\":\"pente\",\"group\":\""
            + GROUP_ID
            + "\",\"topic\":\"orders\",\"data\":{\"hello\":\"world\"}}";
    try (MockJsonRpcServer server = serverReturning(messageJson);
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroupMessage message = new PrivacyGroupClient(rpc).getMessageById(msgId).join();

      assertEquals(msgId, message.id());
      assertEquals(7L, message.localSequence());
      assertEquals("node2", message.node());
      assertEquals("world", message.data().get("hello").asText());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_getMessageById", req.get("method").asText());
      assertEquals(msgId.toString(), req.get("params").get(0).asText());
    }
  }

  @Test
  void queryMessages() throws IOException {
    final String messageJson =
        "[{\"id\":\"" + UUID.randomUUID() + "\",\"localSequence\":1,\"topic\":\"orders\"}]";
    try (MockJsonRpcServer server = serverReturning(messageJson);
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final QueryJSON query = QueryJSON.builder().limit(25).equal("topic", "orders").build();

      final List<PrivacyGroupMessage> messages =
          new PrivacyGroupClient(rpc).queryMessages(query).join();

      assertEquals(1, messages.size());
      assertEquals("orders", messages.get(0).topic());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_queryMessages", req.get("method").asText());
      assertEquals(25, req.get("params").get(0).get("limit").asInt());
    }
  }

  // ---- message listeners --------------------------------------------------

  @Test
  void createMessageListener() throws IOException {
    try (MockJsonRpcServer server = serverReturning("true");
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroupMessageListener listener =
          PrivacyGroupMessageListener.builder("orders-listener")
              .started(true)
              .filters(
                  PrivacyGroupMessageListenerFilters.builder()
                      .domain("pente")
                      .topic("orders")
                      .build())
              .options(PrivacyGroupMessageListenerOptions.builder().excludeLocal(true).build())
              .build();

      assertTrue(new PrivacyGroupClient(rpc).createMessageListener(listener).join());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_createMessageListener", req.get("method").asText());
      final JsonNode sent = req.get("params").get(0);
      assertEquals("orders-listener", sent.get("name").asText());
      assertEquals("orders", sent.get("filters").get("topic").asText());
      assertTrue(sent.get("options").get("excludeLocal").asBoolean());
      // `created` is server-assigned and must not be sent on create.
      assertFalse(sent.has("created"));
    }
  }

  @Test
  void queryMessageListeners() throws IOException {
    try (MockJsonRpcServer server = serverReturning("[{\"name\":\"l1\",\"started\":true}]");
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final QueryJSON query = QueryJSON.builder().limit(1).build();

      final List<PrivacyGroupMessageListener> listeners =
          new PrivacyGroupClient(rpc).queryMessageListeners(query).join();

      assertEquals(1, listeners.size());
      assertEquals("l1", listeners.get(0).name());
      assertTrue(listeners.get(0).started());

      assertEquals("pgroup_queryMessageListeners", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getMessageListener() throws IOException {
    try (MockJsonRpcServer server = serverReturning("{\"name\":\"l1\",\"started\":false}");
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroupMessageListener listener =
          new PrivacyGroupClient(rpc).getMessageListener("l1").join();

      assertEquals("l1", listener.name());
      assertFalse(listener.started());

      final JsonNode req = server.requests().get(0);
      assertEquals("pgroup_getMessageListener", req.get("method").asText());
      assertEquals("l1", req.get("params").get(0).asText());
    }
  }

  @Test
  void startStopAndDeleteMessageListener() throws IOException {
    try (MockJsonRpcServer server = serverReturning("true");
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final PrivacyGroupClient client = new PrivacyGroupClient(rpc);

      assertTrue(client.startMessageListener("l1").join());
      assertTrue(client.stopMessageListener("l1").join());
      assertTrue(client.deleteMessageListener("l1").join());

      assertEquals("pgroup_startMessageListener", server.requests().get(0).get("method").asText());
      assertEquals("pgroup_stopMessageListener", server.requests().get(1).get("method").asText());
      assertEquals("pgroup_deleteMessageListener", server.requests().get(2).get("method").asText());
      assertEquals("l1", server.requests().get(2).get("params").get(0).asText());
    }
  }

  @Test
  void rpcErrorPropagates() throws IOException {
    final String body =
        "{\"jsonrpc\":\"2.0\",\"id\":\"1\",\"error\":{\"code\":-32000,\"message\":\"no such group\"}}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, body));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final CompletionException ex =
          assertThrows(
              CompletionException.class,
              () ->
                  new PrivacyGroupClient(rpc)
                      .getGroupById("pente", HexBytes.fromString("0x00"))
                      .join());
      assertInstanceOf(PaladinRpcException.class, ex.getCause());
    }
  }
}
