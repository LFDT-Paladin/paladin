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
package org.lfdt.paladin.sdk.client.registry;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNull;
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
import org.lfdt.paladin.sdk.core.registry.ActiveFilter;
import org.lfdt.paladin.sdk.core.registry.RegistryEntry;
import org.lfdt.paladin.sdk.core.registry.RegistryEntryWithProperties;
import org.lfdt.paladin.sdk.core.registry.RegistryProperty;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class RegistryClientTest {

  private static final String ENTRY_JSON =
      "{\"registry\":\"reg1\",\"id\":\"0x01\",\"name\":\"acme\",\"parentId\":\"0x00\","
          + "\"blockNumber\":10,\"transactionIndex\":2,\"logIndex\":1,\"active\":true}";

  private static final String ENTRY_WITH_PROPS_JSON =
      "{\"registry\":\"reg1\",\"id\":\"0x01\",\"name\":\"acme\","
          + "\"properties\":{\"url\":\"https://acme.example\",\"tier\":\"gold\"}}";

  private static final String PROPERTY_JSON =
      "{\"registry\":\"reg1\",\"entryId\":\"0x01\",\"name\":\"url\","
          + "\"value\":\"https://acme.example\",\"blockNumber\":10}";

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
  void registries() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[\"reg1\",\"reg2\"]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<String> names = new RegistryClient(rpc).registries().join();
      assertEquals(List.of("reg1", "reg2"), names);
      final JsonNode req = server.requests().get(0);
      assertEquals("reg_registries", req.get("method").asText());
    }
  }

  @Test
  void queryEntries() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + ENTRY_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<RegistryEntry> entries =
          new RegistryClient(rpc)
              .queryEntries("reg1", QueryJSON.builder().limit(10).build(), ActiveFilter.ACTIVE)
              .join();
      assertEquals(1, entries.size());
      final RegistryEntry entry = entries.get(0);
      assertEquals("reg1", entry.registry());
      assertEquals("acme", entry.name());
      assertEquals(HexBytes.fromString("0x01"), entry.id());
      assertEquals(10L, entry.blockNumber());
      assertEquals(2L, entry.transactionIndex());
      assertEquals(1L, entry.logIndex());
      assertTrue(entry.active());

      final JsonNode req = server.requests().get(0);
      assertEquals("reg_queryEntries", req.get("method").asText());
      assertEquals("reg1", req.get("params").get(0).asText());
      assertEquals("active", req.get("params").get(2).asText());
    }
  }

  @Test
  void queryEntriesWithProps() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(
                        200, success("[" + ENTRY_WITH_PROPS_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<RegistryEntryWithProperties> entries =
          new RegistryClient(rpc)
              .queryEntriesWithProps("reg1", QueryJSON.builder().build(), ActiveFilter.ANY)
              .join();
      assertEquals(1, entries.size());
      final RegistryEntryWithProperties entry = entries.get(0);
      assertEquals("acme", entry.name());
      assertEquals("gold", entry.properties().get("tier"));
      assertEquals("https://acme.example", entry.properties().get("url"));
      assertNull(entry.blockNumber());

      final JsonNode req = server.requests().get(0);
      assertEquals("reg_queryEntriesWithProps", req.get("method").asText());
      assertEquals("any", req.get("params").get(2).asText());
    }
  }

  @Test
  void getEntryProperties() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(200, success("[" + PROPERTY_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<RegistryProperty> props =
          new RegistryClient(rpc)
              .getEntryProperties("reg1", HexBytes.fromString("0x01"), ActiveFilter.INACTIVE)
              .join();
      assertEquals(1, props.size());
      final RegistryProperty prop = props.get(0);
      assertEquals("url", prop.name());
      assertEquals("https://acme.example", prop.value());
      assertEquals(HexBytes.fromString("0x01"), prop.entryId());
      assertEquals(10L, prop.blockNumber());

      final JsonNode req = server.requests().get(0);
      assertEquals("reg_getEntryProperties", req.get("method").asText());
      assertEquals("reg1", req.get("params").get(0).asText());
      assertEquals("0x01", req.get("params").get(1).asText());
      assertEquals("inactive", req.get("params").get(2).asText());
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
                            + "\"message\":\"no such registry\"}}"));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final CompletionException ex =
          assertThrows(
              CompletionException.class, () -> new RegistryClient(rpc).registries().join());
      assertInstanceOf(PaladinRpcException.class, ex.getCause());
    }
  }
}
