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
import org.lfdt.paladin.sdk.core.abi.ABIDecodedData;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.StoredABI;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.transaction.BlockchainEventListener;
import org.lfdt.paladin.sdk.core.transaction.BlockchainEventListenerSource;
import org.lfdt.paladin.sdk.core.transaction.BlockchainEventListenerStatus;
import org.lfdt.paladin.sdk.core.transaction.ChainedDispatch;
import org.lfdt.paladin.sdk.core.transaction.Dispatch;
import org.lfdt.paladin.sdk.core.transaction.PreparedTransaction;
import org.lfdt.paladin.sdk.core.transaction.PublicTxWithBinding;
import org.lfdt.paladin.sdk.core.transaction.Transaction;
import org.lfdt.paladin.sdk.core.transaction.TransactionCall;
import org.lfdt.paladin.sdk.core.transaction.TransactionFull;
import org.lfdt.paladin.sdk.core.transaction.TransactionInput;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceiptFull;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceiptListener;
import org.lfdt.paladin.sdk.core.transaction.TransactionStates;
import org.lfdt.paladin.sdk.core.transaction.TransactionType;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.HexBytes;

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
      UUID id = new PtxClient(rpc).updateTransaction(UUID.fromString(UUID1), sampleInput()).join();
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
      TransactionFull result = new PtxClient(rpc).getTransactionFull(UUID.fromString(UUID1)).join();
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
      Transaction result = new PtxClient(rpc).getTransactionByIdempotencyKey("key-1").join();
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
          new PtxClient(rpc).queryTransactionsFull(QueryJSON.builder().limit(5).build()).join();
      assertEquals(1, result.size());
      assertTrue(result.get(0).receipt().success());
      assertEquals("ptx_queryTransactionsFull", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getTransactionReceipt() throws IOException {
    String receipt = "{\"id\":\"" + UUID1 + "\",\"success\":true,\"sequence\":5}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(receipt)));
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
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(receipt)));
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
      JsonNode result = new PtxClient(rpc).getDomainReceipt("noto", UUID.fromString(UUID1)).join();
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
      TransactionStates result = new PtxClient(rpc).getStateReceipt(UUID.fromString(UUID1)).join();
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
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(receipts)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<TransactionReceipt> result =
          new PtxClient(rpc).queryTransactionReceipts(QueryJSON.builder().limit(5).build()).join();
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
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(prepared)));
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
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(prepared)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<PreparedTransaction> result =
          new PtxClient(rpc).queryPreparedTransactions(QueryJSON.builder().limit(5).build()).join();
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
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(publicTx)));
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

  private static final String HASH =
      "0x1111111111111111111111111111111111111111111111111111111111111111";

  @Test
  void storeABI() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"" + HASH + "\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      Bytes32 hash = new PtxClient(rpc).storeABI(List.of(AbiEntry.function("foo").build())).join();
      assertEquals(Bytes32.fromString(HASH), hash);
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_storeABI", req.get("method").asText());
      assertEquals("foo", req.get("params").get(0).get(0).get("name").asText());
    }
  }

  @Test
  void getStoredABI() throws IOException {
    String stored =
        "{\"hash\":\"" + HASH + "\",\"abi\":[{\"type\":\"function\",\"name\":\"foo\"}]}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(stored)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      StoredABI result = new PtxClient(rpc).getStoredABI(Bytes32.fromString(HASH)).join();
      assertEquals(Bytes32.fromString(HASH), result.hash());
      assertEquals(1, result.abi().size());
      assertEquals("foo", result.abi().get(0).name());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getStoredABI", req.get("method").asText());
      assertEquals(HASH, req.get("params").get(0).asText());
    }
  }

  @Test
  void queryStoredABIs() throws IOException {
    String stored = "[{\"hash\":\"" + HASH + "\"}]";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(stored)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<StoredABI> result =
          new PtxClient(rpc).queryStoredABIs(QueryJSON.builder().limit(5).build()).join();
      assertEquals(1, result.size());
      assertEquals(Bytes32.fromString(HASH), result.get(0).hash());
      assertEquals("ptx_queryStoredABIs", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void decodeError() throws IOException {
    String decoded =
        "{\"signature\":\"Error(string)\",\"summary\":\"boom\","
            + "\"definition\":{\"type\":\"error\",\"name\":\"Error\"}}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(decoded)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      ABIDecodedData result =
          new PtxClient(rpc).decodeError(HexBytes.fromString("0xdead"), "mode=array").join();
      assertEquals("Error(string)", result.signature());
      assertEquals("boom", result.summary());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_decodeError", req.get("method").asText());
      assertEquals("0xdead", req.get("params").get(0).asText());
      assertEquals("mode=array", req.get("params").get(1).asText());
    }
  }

  @Test
  void decodeCall() throws IOException {
    String decoded = "{\"signature\":\"foo()\"}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(decoded)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      ABIDecodedData result =
          new PtxClient(rpc).decodeCall(HexBytes.fromString("0xbeef"), "").join();
      assertEquals("foo()", result.signature());
      assertEquals("ptx_decodeCall", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void decodeEvent() throws IOException {
    String decoded = "{\"signature\":\"Transfer(address,address,uint256)\"}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(decoded)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      ABIDecodedData result =
          new PtxClient(rpc)
              .decodeEvent(List.of(Bytes32.fromString(HASH)), HexBytes.fromString("0x01"), "")
              .join();
      assertEquals("Transfer(address,address,uint256)", result.signature());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_decodeEvent", req.get("method").asText());
      assertEquals(3, req.get("params").size());
      assertEquals(HASH, req.get("params").get(0).get(0).asText());
    }
  }

  @Test
  void resolveVerifier() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"0xabc\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      String verifier =
          new PtxClient(rpc).resolveVerifier("alice", "ecdsa:secp256k1", "eth_address").join();
      assertEquals("0xabc", verifier);
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_resolveVerifier", req.get("method").asText());
      assertEquals(3, req.get("params").size());
      assertEquals("alice", req.get("params").get(0).asText());
      assertEquals("eth_address", req.get("params").get(2).asText());
    }
  }

  @Test
  void createReceiptListener() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success("true")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      TransactionReceiptListener listener =
          TransactionReceiptListener.builder().name("l1").started(true).build();
      Boolean ok = new PtxClient(rpc).createReceiptListener(listener).join();
      assertTrue(ok);
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_createReceiptListener", req.get("method").asText());
      assertEquals("l1", req.get("params").get(0).get("name").asText());
    }
  }

  @Test
  void queryReceiptListeners() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[{\"name\":\"l1\"}]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<TransactionReceiptListener> result =
          new PtxClient(rpc).queryReceiptListeners(QueryJSON.builder().limit(5).build()).join();
      assertEquals(1, result.size());
      assertEquals("l1", result.get(0).name());
      assertEquals("ptx_queryReceiptListeners", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getReceiptListener() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(
                        200, success("{\"name\":\"l1\",\"started\":true}")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      TransactionReceiptListener result = new PtxClient(rpc).getReceiptListener("l1").join();
      assertEquals("l1", result.name());
      assertTrue(result.started());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getReceiptListener", req.get("method").asText());
      assertEquals("l1", req.get("params").get(0).asText());
    }
  }

  @Test
  void startStopDeleteReceiptListener() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success("true")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      PtxClient client = new PtxClient(rpc);
      assertTrue(client.startReceiptListener("l1").join());
      assertTrue(client.stopReceiptListener("l1").join());
      assertTrue(client.deleteReceiptListener("l1").join());
      assertEquals("ptx_startReceiptListener", server.requests().get(0).get("method").asText());
      assertEquals("ptx_stopReceiptListener", server.requests().get(1).get("method").asText());
      assertEquals("ptx_deleteReceiptListener", server.requests().get(2).get("method").asText());
    }
  }

  @Test
  void createBlockchainEventListener() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success("true")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      BlockchainEventListener listener =
          BlockchainEventListener.builder()
              .name("be1")
              .source(
                  BlockchainEventListenerSource.builder()
                      .abiEntry(AbiEntry.event("Transfer").build())
                      .build())
              .build();
      Boolean ok = new PtxClient(rpc).createBlockchainEventListener(listener).join();
      assertTrue(ok);
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_createBlockchainEventListener", req.get("method").asText());
      assertEquals("be1", req.get("params").get(0).get("name").asText());
      assertEquals(
          "Transfer",
          req.get("params").get(0).get("sources").get(0).get("abi").get(0).get("name").asText());
    }
  }

  @Test
  void queryBlockchainEventListeners() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[{\"name\":\"be1\"}]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<BlockchainEventListener> result =
          new PtxClient(rpc)
              .queryBlockchainEventListeners(QueryJSON.builder().limit(5).build())
              .join();
      assertEquals(1, result.size());
      assertEquals("be1", result.get(0).name());
      assertEquals(
          "ptx_queryBlockchainEventListeners", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getBlockchainEventListener() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("{\"name\":\"be1\"}")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      BlockchainEventListener result = new PtxClient(rpc).getBlockchainEventListener("be1").join();
      assertEquals("be1", result.name());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getBlockchainEventListener", req.get("method").asText());
      assertEquals("be1", req.get("params").get(0).asText());
    }
  }

  @Test
  void startStopDeleteBlockchainEventListener() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success("true")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      PtxClient client = new PtxClient(rpc);
      assertTrue(client.startBlockchainEventListener("be1").join());
      assertTrue(client.stopBlockchainEventListener("be1").join());
      assertTrue(client.deleteBlockchainEventListener("be1").join());
      assertEquals(
          "ptx_startBlockchainEventListener", server.requests().get(0).get("method").asText());
      assertEquals(
          "ptx_stopBlockchainEventListener", server.requests().get(1).get("method").asText());
      assertEquals(
          "ptx_deleteBlockchainEventListener", server.requests().get(2).get("method").asText());
    }
  }

  @Test
  void getBlockchainEventListenerStatus() throws IOException {
    String status = "{\"catchup\":true,\"checkpoint\":{\"blockNumber\":100}}";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer((n, req) -> MockJsonRpcServer.Response.of(200, success(status)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      BlockchainEventListenerStatus result =
          new PtxClient(rpc).getBlockchainEventListenerStatus("be1").join();
      assertTrue(result.catchup());
      assertEquals(100, result.checkpoint().blockNumber());
      assertEquals(
          "ptx_getBlockchainEventListenerStatus", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void queryDispatches() throws IOException {
    String dispatches = "[{\"id\":\"d1\",\"transactionID\":\"t1\",\"publicTransactionID\":5}]";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(dispatches)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<Dispatch> result =
          new PtxClient(rpc).queryDispatches(QueryJSON.builder().limit(5).build()).join();
      assertEquals(1, result.size());
      assertEquals("d1", result.get(0).id());
      assertEquals(5, result.get(0).publicTransactionID());
      assertEquals("ptx_queryDispatches", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getDispatch() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(
                        200, success("{\"id\":\"d1\",\"transactionID\":\"t1\"}")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      Dispatch result = new PtxClient(rpc).getDispatch("d1").join();
      assertEquals("d1", result.id());
      assertEquals("t1", result.transactionID());
      JsonNode req = server.requests().get(0);
      assertEquals("ptx_getDispatch", req.get("method").asText());
      assertEquals("d1", req.get("params").get(0).asText());
    }
  }

  @Test
  void queryChainedDispatches() throws IOException {
    String chained = "[{\"id\":\"c1\",\"transactionID\":\"t1\",\"chainedTransactionID\":\"t2\"}]";
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(chained)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      List<ChainedDispatch> result =
          new PtxClient(rpc).queryChainedDispatches(QueryJSON.builder().limit(5).build()).join();
      assertEquals(1, result.size());
      assertEquals("t2", result.get(0).chainedTransactionID());
      assertEquals("ptx_queryChainedDispatches", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getChainedDispatch() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(
                        200, success("{\"id\":\"c1\",\"chainedTransactionID\":\"t2\"}")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      ChainedDispatch result = new PtxClient(rpc).getChainedDispatch("c1").join();
      assertEquals("c1", result.id());
      assertEquals("t2", result.chainedTransactionID());
      assertEquals("ptx_getChainedDispatch", server.requests().get(0).get("method").asText());
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
