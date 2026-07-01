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

package org.lfdt.paladin.sdk.client.ptx;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

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
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.transaction.PreparedTransaction;
import org.lfdt.paladin.sdk.core.transaction.PublicTxWithBinding;
import org.lfdt.paladin.sdk.core.transaction.Transaction;
import org.lfdt.paladin.sdk.core.transaction.TransactionCall;
import org.lfdt.paladin.sdk.core.transaction.TransactionFull;
import org.lfdt.paladin.sdk.core.transaction.TransactionInput;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceiptFull;
import org.lfdt.paladin.sdk.core.transaction.TransactionStates;
import org.lfdt.paladin.sdk.core.transaction.TransactionType;

class PtxClientTest {

  private static final String UUID1 = "00000000-0000-0000-0000-000000000001";
  private static final String UUID2 = "00000000-0000-0000-0000-000000000002";

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

  private static TransactionInput sampleInput() {
    return TransactionInput.builder()
        .type(TransactionType.PUBLIC)
        .from("alice")
        .function("foo()")
        .build();
  }

  @Test
  void sendTransaction() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"" + UUID1 + "\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      UUID id = new PtxClient(rpc).sendTransaction(sampleInput()).join();
      assertEquals(UUID.fromString(UUID1), id);
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_sendTransaction", req.get("method").asText());
      JsonNode tx = req.get("params").get(0);
      assertEquals("public", tx.get("type").asText());
      assertEquals("alice", tx.get("from").asText());
    }
  }

  @Test
  void sendTransactions() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[\"" + UUID1 + "\"]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<UUID> ids = new PtxClient(rpc).sendTransactions(List.of(sampleInput())).join();
      assertEquals(List.of(UUID.fromString(UUID1)), ids);
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_sendTransactions", req.get("method").asText());
      assertEquals(1, req.get("params").get(0).size());
    }
  }

  @Test
  void prepareTransaction() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"" + UUID1 + "\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      UUID id = new PtxClient(rpc).prepareTransaction(sampleInput()).join();
      assertEquals(UUID.fromString(UUID1), id);
      assertEquals("ptx_prepareTransaction", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void prepareTransactions() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[\"" + UUID1 + "\"]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<UUID> ids = new PtxClient(rpc).prepareTransactions(List.of(sampleInput())).join();
      assertEquals(1, ids.size());
      assertEquals("ptx_prepareTransactions", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void updateTransaction() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"" + UUID1 + "\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      UUID id =
          new PtxClient(rpc).updateTransaction(UUID.fromString(UUID1), sampleInput()).join();
      assertEquals(UUID.fromString(UUID1), id);
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_updateTransaction", req.get("method").asText());
      assertEquals(2, req.get("params").size());
      assertEquals(UUID1, req.get("params").get(0).asText());
    }
  }

  @Test
  void callUnwrapsInputAndAppendsOptions() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("{\"output\":\"0x1\"}")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      TransactionCall call =
          TransactionCall.builder(sampleInput()).block("latest").dataFormat("mode=array").build();
      JsonNode result = new PtxClient(rpc).call(call).join();
      assertEquals("0x1", result.get("output").asText());
      JsonNode params0 = server.requests().get(0).get("params").get(0);
      assertEquals("ptx_call", server.requests().get(0).get("method").asText());
      // The embedded TransactionInput is unwrapped flat alongside block/dataFormat.
      assertEquals("public", params0.get("type").asText());
      assertEquals("latest", params0.get("block").asText());
      assertEquals("mode=array", params0.get("dataFormat").asText());
    }
  }

  @Test
  void getTransaction() throws IOException {
    String tx =
        "{\"id\":\"" + UUID1 + "\",\"submitMode\":\"auto\",\"type\":\"public\",\"from\":\"alice\"}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(tx)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      Transaction result = new PtxClient(rpc).getTransaction(UUID.fromString(UUID1)).join();
      assertEquals(UUID.fromString(UUID1), result.id());
      assertEquals(TransactionType.PUBLIC, result.type());
      assertEquals("alice", result.from());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getTransaction", req.get("method").asText());
      assertEquals(UUID1, req.get("params").get(0).asText());
    }
  }

  @Test
  void getTransactionFull() throws IOException {
    String tx =
        "{\"id\":\""
            + UUID1
            + "\",\"type\":\"private\",\"domain\":\"noto\",\"dependsOn\":[\""
            + UUID2
            + "\"],\"receipt\":{\"success\":true},\"public\":[{\"nonce\":\"0x1\"}]}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(tx)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      TransactionFull result =
          new PtxClient(rpc).getTransactionFull(UUID.fromString(UUID1)).join();
      assertEquals(TransactionType.PRIVATE, result.type());
      assertEquals(List.of(UUID.fromString(UUID2)), result.dependsOn());
      assertTrue(result.receipt().success());
      assertEquals(1, result.publicTransactions().size());
      assertEquals("ptx_getTransactionFull", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getTransactionByIdempotencyKey() throws IOException {
    String tx = "{\"id\":\"" + UUID1 + "\",\"type\":\"public\"}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(tx)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      Transaction result =
          new PtxClient(rpc).getTransactionByIdempotencyKey("key-1").join();
      assertEquals(UUID.fromString(UUID1), result.id());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getTransactionByIdempotencyKey", req.get("method").asText());
      assertEquals("key-1", req.get("params").get(0).asText());
    }
  }

  @Test
  void queryTransactions() throws IOException {
    String txs = "[{\"id\":\"" + UUID1 + "\",\"type\":\"public\"}]";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(txs)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      QueryJSON query = QueryJSON.builder().limit(5).equal("type", "public").build();
      List<Transaction> result = new PtxClient(rpc).queryTransactions(query).join();
      assertEquals(1, result.size());
      assertEquals(UUID.fromString(UUID1), result.get(0).id());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_queryTransactions", req.get("method").asText());
      assertEquals(5, req.get("params").get(0).get("limit").asInt());
    }
  }

  @Test
  void queryTransactionsFull() throws IOException {
    String txs = "[{\"id\":\"" + UUID1 + "\",\"type\":\"public\",\"receipt\":{\"success\":true}}]";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(txs)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<TransactionFull> result =
          new PtxClient(rpc)
              .queryTransactionsFull(QueryJSON.builder().limit(5).build())
              .join();
      assertEquals(1, result.size());
      assertTrue(result.get(0).receipt().success());
      assertEquals("ptx_queryTransactionsFull", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getTransactionReceipt() throws IOException {
    String receipt = "{\"id\":\"" + UUID1 + "\",\"success\":true,\"sequence\":5}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(receipt)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      TransactionReceipt result =
          new PtxClient(rpc).getTransactionReceipt(UUID.fromString(UUID1)).join();
      assertEquals(UUID.fromString(UUID1), result.id());
      assertTrue(result.success());
      assertEquals(5, result.sequence());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getTransactionReceipt", req.get("method").asText());
      assertEquals(UUID1, req.get("params").get(0).asText());
    }
  }

  @Test
  void getTransactionReceiptFull() throws IOException {
    String receipt =
        "{\"id\":\""
            + UUID1
            + "\",\"success\":true,\"states\":{\"confirmed\":[{\"id\":\"0x01\"}]},"
            + "\"domainReceipt\":{\"noto\":{\"burn\":true}},\"domainReceiptError\":\"\","
            + "\"public\":[{\"nonce\":\"0x1\"}]}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(receipt)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      TransactionReceiptFull result =
          new PtxClient(rpc).getTransactionReceiptFull(UUID.fromString(UUID1)).join();
      assertEquals(UUID.fromString(UUID1), result.id());
      assertTrue(result.success());
      assertEquals(1, result.states().confirmed().size());
      assertTrue(result.domainReceipt().get("noto").get("burn").asBoolean());
      assertEquals(1, result.publicTransactions().size());
      assertEquals(
          "ptx_getTransactionReceiptFull", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getDomainReceipt() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("{\"transfers\":[]}")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      JsonNode result =
          new PtxClient(rpc).getDomainReceipt("noto", UUID.fromString(UUID1)).join();
      assertTrue(result.get("transfers").isArray());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getDomainReceipt", req.get("method").asText());
      assertEquals(2, req.get("params").size());
      assertEquals("noto", req.get("params").get(0).asText());
      assertEquals(UUID1, req.get("params").get(1).asText());
    }
  }

  @Test
  void getStateReceipt() throws IOException {
    String states = "{\"none\":false,\"spent\":[{\"id\":\"0x01\"}],\"read\":[{\"id\":\"0x02\"}]}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(states)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      TransactionStates result =
          new PtxClient(rpc).getStateReceipt(UUID.fromString(UUID1)).join();
      assertEquals(1, result.spent().size());
      assertEquals(1, result.read().size());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getStateReceipt", req.get("method").asText());
      assertEquals(UUID1, req.get("params").get(0).asText());
    }
  }

  @Test
  void queryTransactionReceipts() throws IOException {
    String receipts = "[{\"id\":\"" + UUID1 + "\",\"success\":true}]";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(receipts)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<TransactionReceipt> result =
          new PtxClient(rpc)
              .queryTransactionReceipts(QueryJSON.builder().limit(5).build())
              .join();
      assertEquals(1, result.size());
      assertTrue(result.get(0).success());
      assertEquals("ptx_queryTransactionReceipts", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getPreparedTransaction() throws IOException {
    String prepared =
        "{\"id\":\""
            + UUID1
            + "\",\"domain\":\"noto\",\"transaction\":{\"type\":\"private\",\"from\":\"alice\"},"
            + "\"states\":{\"confirmed\":[{\"id\":\"0x01\"}]}}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(prepared)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      PreparedTransaction result =
          new PtxClient(rpc).getPreparedTransaction(UUID.fromString(UUID1)).join();
      assertEquals(UUID.fromString(UUID1), result.id());
      assertEquals("noto", result.domain());
      assertEquals(TransactionType.PRIVATE, result.transaction().type());
      assertEquals(1, result.states().confirmed().size());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getPreparedTransaction", req.get("method").asText());
      assertEquals(UUID1, req.get("params").get(0).asText());
    }
  }

  @Test
  void queryPreparedTransactions() throws IOException {
    String prepared = "[{\"id\":\"" + UUID1 + "\",\"domain\":\"noto\"}]";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(prepared)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<PreparedTransaction> result =
          new PtxClient(rpc)
              .queryPreparedTransactions(QueryJSON.builder().limit(5).build())
              .join();
      assertEquals(1, result.size());
      assertEquals("noto", result.get(0).domain());
      assertEquals(
          "ptx_queryPreparedTransactions", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getPublicTransaction() throws IOException {
    String publicTx =
        "{\"from\":\"0x0000000000000000000000000000000000000001\",\"nonce\":\"0x2a\","
            + "\"transaction\":\""
            + UUID1
            + "\",\"transactionType\":\"private\",\"sender\":\"alice\"}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(publicTx)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      PublicTxWithBinding result = new PtxClient(rpc).getPublicTransaction(42L).join();
      assertEquals(UUID.fromString(UUID1), result.transaction());
      assertEquals(TransactionType.PRIVATE, result.transactionType());
      assertEquals("alice", result.sender());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getPublicTransaction", req.get("method").asText());
      assertEquals(42, req.get("params").get(0).asLong());
    }
  }

  @Test
  void rpcErrorPropagates() throws IOException {
    String body =
        "{\"jsonrpc\":\"2.0\",\"id\":\"1\",\"error\":{\"code\":-32000,\"message\":\"no such tx\"}}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, body));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      CompletionException ex =
          assertThrows(
              CompletionException.class,
              () -> new PtxClient(rpc).getTransaction(UUID.fromString(UUID1)).join());
      assertInstanceOf(PaladinRpcException.class, ex.getCause());
    }
  }
}
