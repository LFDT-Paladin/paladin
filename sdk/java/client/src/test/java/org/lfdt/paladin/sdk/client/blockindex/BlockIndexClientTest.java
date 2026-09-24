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
package org.lfdt.paladin.sdk.client.blockindex;

import static org.junit.jupiter.api.Assertions.assertEquals;
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
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.blockindex.EthTransactionResult;
import org.lfdt.paladin.sdk.core.blockindex.EventWithData;
import org.lfdt.paladin.sdk.core.blockindex.IndexedBlock;
import org.lfdt.paladin.sdk.core.blockindex.IndexedEvent;
import org.lfdt.paladin.sdk.core.blockindex.IndexedTransaction;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexUint64;

class BlockIndexClientTest {

  private static final String BLOCK_JSON =
      "{\"number\":42,\"hash\":\"0x"
          + "1111111111111111111111111111111111111111111111111111111111111111\","
          + "\"timestamp\":\"2024-01-01T00:00:00Z\"}";

  private static final String TX_JSON =
      "{\"hash\":\"0x"
          + "2222222222222222222222222222222222222222222222222222222222222222\","
          + "\"blockNumber\":42,\"transactionIndex\":1,"
          + "\"from\":\"0x1111111111111111111111111111111111111111\","
          + "\"to\":\"0x2222222222222222222222222222222222222222\","
          + "\"nonce\":9,\"result\":\"success\"}";

  private static final String EVENT_JSON =
      "{\"blockNumber\":42,\"transactionIndex\":1,\"logIndex\":0,"
          + "\"transactionHash\":\"0x"
          + "2222222222222222222222222222222222222222222222222222222222222222\","
          + "\"signature\":\"0x"
          + "3333333333333333333333333333333333333333333333333333333333333333\"}";

  private static final String EVENT_WITH_DATA_JSON =
      "{\"blockNumber\":42,\"transactionIndex\":1,\"logIndex\":0,"
          + "\"transactionHash\":\"0x"
          + "2222222222222222222222222222222222222222222222222222222222222222\","
          + "\"signature\":\"0x"
          + "3333333333333333333333333333333333333333333333333333333333333333\","
          + "\"soliditySignature\":\"Transfer(address,address,uint256)\","
          + "\"address\":\"0x2222222222222222222222222222222222222222\","
          + "\"data\":{\"value\":\"100\"}}";

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
  void getBlockByNumber() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(BLOCK_JSON)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final IndexedBlock block =
          new BlockIndexClient(rpc).getBlockByNumber(HexUint64.of(42)).join();
      assertEquals(42L, block.number());
      assertEquals(
          Bytes32.fromString("0x1111111111111111111111111111111111111111111111111111111111111111"),
          block.hash());
      assertEquals("bidx_getBlockByNumber", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getTransactionByHash() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(TX_JSON)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final IndexedTransaction tx =
          new BlockIndexClient(rpc)
              .getTransactionByHash(
                  Bytes32.fromString(
                      "0x2222222222222222222222222222222222222222222222222222222222222222"))
              .join();
      assertEquals(42L, tx.blockNumber());
      assertEquals(1L, tx.transactionIndex());
      assertEquals(9L, tx.nonce());
      assertEquals(EthTransactionResult.SUCCESS, tx.result());
      assertEquals(EthAddress.fromString("0x1111111111111111111111111111111111111111"), tx.from());
      assertEquals("bidx_getTransactionByHash", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getTransactionByNonce() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success(TX_JSON)));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final IndexedTransaction tx =
          new BlockIndexClient(rpc)
              .getTransactionByNonce(
                  EthAddress.fromString("0x1111111111111111111111111111111111111111"),
                  HexUint64.of(9))
              .join();
      assertEquals(9L, tx.nonce());
      final JsonNode req = server.requests().get(0);
      assertEquals("bidx_getTransactionByNonce", req.get("method").asText());
      assertEquals("0x1111111111111111111111111111111111111111", req.get("params").get(0).asText());
    }
  }

  @Test
  void getBlockTransactionsByNumber() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + TX_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<IndexedTransaction> txs =
          new BlockIndexClient(rpc).getBlockTransactionsByNumber(HexUint64.of(42)).join();
      assertEquals(1, txs.size());
      assertEquals(
          "bidx_getBlockTransactionsByNumber", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getTransactionEventsByHash() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + EVENT_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<IndexedEvent> events =
          new BlockIndexClient(rpc)
              .getTransactionEventsByHash(
                  Bytes32.fromString(
                      "0x2222222222222222222222222222222222222222222222222222222222222222"))
              .join();
      assertEquals(1, events.size());
      assertEquals(0L, events.get(0).logIndex());
      assertEquals(
          "bidx_getTransactionEventsByHash", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void queryIndexedBlocks() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + BLOCK_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<IndexedBlock> blocks =
          new BlockIndexClient(rpc).queryIndexedBlocks(QueryJSON.builder().limit(5).build()).join();
      assertEquals(1, blocks.size());
      assertEquals(42L, blocks.get(0).number());
      assertEquals("bidx_queryIndexedBlocks", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void queryIndexedTransactions() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + TX_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<IndexedTransaction> txs =
          new BlockIndexClient(rpc).queryIndexedTransactions(QueryJSON.builder().build()).join();
      assertEquals(1, txs.size());
      assertEquals(
          "bidx_queryIndexedTransactions", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void queryIndexedEvents() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("[" + EVENT_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<IndexedEvent> events =
          new BlockIndexClient(rpc).queryIndexedEvents(QueryJSON.builder().build()).join();
      assertEquals(1, events.size());
      assertEquals("bidx_queryIndexedEvents", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void getConfirmedBlockHeight() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) -> MockJsonRpcServer.Response.of(200, success("\"0x2a\"")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final HexUint64 height = new BlockIndexClient(rpc).getConfirmedBlockHeight().join();
      assertEquals(HexUint64.of(42), height);
      assertEquals("bidx_getConfirmedBlockHeight", server.requests().get(0).get("method").asText());
    }
  }

  @Test
  void decodeTransactionEvents() throws IOException {
    try (MockJsonRpcServer server =
            new MockJsonRpcServer(
                (n, req) ->
                    MockJsonRpcServer.Response.of(200, success("[" + EVENT_WITH_DATA_JSON + "]")));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final List<EventWithData> events =
          new BlockIndexClient(rpc)
              .decodeTransactionEvents(
                  Bytes32.fromString(
                      "0x2222222222222222222222222222222222222222222222222222222222222222"),
                  List.of(AbiEntry.event("Transfer").build()),
                  "mode=object")
              .join();
      assertEquals(1, events.size());
      final EventWithData event = events.get(0);
      assertEquals("Transfer(address,address,uint256)", event.soliditySignature());
      assertEquals(
          EthAddress.fromString("0x2222222222222222222222222222222222222222"), event.address());
      assertEquals("100", event.data().get("value").asText());

      final JsonNode req = server.requests().get(0);
      assertEquals("bidx_decodeTransactionEvents", req.get("method").asText());
      assertEquals("mode=object", req.get("params").get(2).asText());
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
                            + "\"message\":\"not found\"}}"));
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      final CompletionException ex =
          assertThrows(
              CompletionException.class,
              () -> new BlockIndexClient(rpc).getBlockByNumber(HexUint64.of(1)).join());
      assertInstanceOf(PaladinRpcException.class, ex.getCause());
    }
  }
}
