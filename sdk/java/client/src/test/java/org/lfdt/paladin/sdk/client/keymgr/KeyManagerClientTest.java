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
package org.lfdt.paladin.sdk.client.keymgr;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;

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
import org.lfdt.paladin.sdk.core.key.KeyMappingAndVerifier;
import org.lfdt.paladin.sdk.core.key.KeyQueryEntry;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class KeyManagerClientTest {

  private static final String MAPPING_JSON =
      "{\"identifier\":\"key1\",\"wallet\":\"w1\",\"keyHandle\":\"h1\","
          + "\"path\":[{\"name\":\"key1\",\"index\":0}],"
          + "\"verifier\":{\"verifier\":\"0xabc\",\"type\":\"eth_address\",\"algorithm\":\"ecdsa\"}}";

  private static String success(String resultJson) {
    return "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"result\":" + resultJson + "}";
  }

  private RpcClientConfig config(String url) {
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
  void wallets() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[\"w-a\",\"w-b\"]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<String> wallets = new KeyManagerClient(rpc).wallets().join();
      assertEquals(List.of("w-a", "w-b"), wallets);
      JsonNode req = server.requests().get(0);
      assertEquals("keymgr_wallets", req.get("method").asText());
      // No-arg calls omit the params array (JSON-RPC envelope uses NON_EMPTY inclusion).
      assertFalse(req.has("params") && req.get("params").size() > 0);
    }
  }

  @Test
  void resolveKey() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(MAPPING_JSON)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      KeyMappingAndVerifier mapping =
          new KeyManagerClient(rpc).resolveKey("key1", "ecdsa", "eth_address").join();
      assertEquals("key1", mapping.identifier());
      assertEquals("w1", mapping.wallet());
      assertEquals("h1", mapping.keyHandle());
      assertEquals(1, mapping.path().size());
      assertEquals("key1", mapping.path().get(0).name());
      assertEquals("0xabc", mapping.verifier().verifier());
      JsonNode req = server.requests().get(0);
      assertEquals("keymgr_resolveKey", req.get("method").asText());
      JsonNode params = req.get("params");
      assertEquals(3, params.size());
      assertEquals("key1", params.get(0).asText());
      assertEquals("ecdsa", params.get(1).asText());
      assertEquals("eth_address", params.get(2).asText());
    }
  }

  @Test
  void resolveEthAddress() throws IOException {
    String addr = "0x1234567890123456789012345678901234567890";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"" + addr + "\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      EthAddress result = new KeyManagerClient(rpc).resolveEthAddress("key1").join();
      assertEquals(EthAddress.fromString(addr), result);
      JsonNode req = server.requests().get(0);
      assertEquals("keymgr_resolveEthAddress", req.get("method").asText());
      assertEquals(1, req.get("params").size());
      assertEquals("key1", req.get("params").get(0).asText());
    }
  }

  @Test
  void reverseKeyLookup() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(MAPPING_JSON)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      KeyMappingAndVerifier mapping =
          new KeyManagerClient(rpc).reverseKeyLookup("ecdsa", "eth_address", "0xabc").join();
      assertEquals("key1", mapping.identifier());
      JsonNode req = server.requests().get(0);
      assertEquals("keymgr_reverseKeyLookup", req.get("method").asText());
      JsonNode params = req.get("params");
      assertEquals(3, params.size());
      assertEquals("ecdsa", params.get(0).asText());
      assertEquals("eth_address", params.get(1).asText());
      assertEquals("0xabc", params.get(2).asText());
    }
  }

  @Test
  void queryKeys() throws IOException {
    String result =
        "[{\"isKey\":true,\"hasChildren\":false,\"parent\":\"\",\"path\":\"key1\","
            + "\"name\":\"key1\",\"index\":0,\"wallet\":\"w1\",\"keyHandle\":\"h1\","
            + "\"verifiers\":[{\"verifier\":\"0xabc\",\"type\":\"t\",\"algorithm\":\"a\"}]}]";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(result)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      QueryJSON query = QueryJSON.builder().limit(10).equal("path", "key1").build();
      List<KeyQueryEntry> keys = new KeyManagerClient(rpc).queryKeys(query).join();
      assertEquals(1, keys.size());
      assertEquals("key1", keys.get(0).path());
      assertEquals(1, keys.get(0).verifiers().size());
      JsonNode req = server.requests().get(0);
      assertEquals("keymgr_queryKeys", req.get("method").asText());
      assertEquals(1, req.get("params").size());
      assertEquals(10, req.get("params").get(0).get("limit").asInt());
    }
  }

  @Test
  void sign() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"0xdeadbeef\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      HexBytes payload = HexBytes.fromString("0x0011");
      HexBytes signature =
          new KeyManagerClient(rpc).sign("key1", "ecdsa", "eth_address", "opaque", payload).join();
      assertEquals(HexBytes.fromString("0xdeadbeef"), signature);
      JsonNode req = server.requests().get(0);
      assertEquals("keymgr_sign", req.get("method").asText());
      JsonNode params = req.get("params");
      assertEquals(5, params.size());
      assertEquals("key1", params.get(0).asText());
      assertEquals("ecdsa", params.get(1).asText());
      assertEquals("eth_address", params.get(2).asText());
      assertEquals("opaque", params.get(3).asText());
      assertEquals("0x0011", params.get(4).asText());
    }
  }

  @Test
  void rpcErrorPropagates() throws IOException {
    String body =
        "{\"jsonrpc\":\"2.0\",\"id\":\"1\",\"error\":{\"code\":-32000,\"message\":\"no such key\"}}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, body));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      CompletionException ex =
          assertThrows(
              CompletionException.class,
              () -> new KeyManagerClient(rpc).resolveEthAddress("missing").join());
      assertInstanceOf(PaladinRpcException.class, ex.getCause());
    }
  }
}
