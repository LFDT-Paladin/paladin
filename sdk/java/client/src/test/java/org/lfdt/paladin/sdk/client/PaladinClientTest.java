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
package org.lfdt.paladin.sdk.client;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.core.type.TypeReference;
import java.io.IOException;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.config.RetryPolicy;
import org.lfdt.paladin.sdk.client.config.RpcClientConfig;
import org.lfdt.paladin.sdk.client.rpc.MockJsonRpcServer;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.client.tx.TxBuilder;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.AbiParameter;
import org.lfdt.paladin.sdk.core.abi.EntryType;
import org.lfdt.paladin.sdk.core.transaction.TransactionInput;
import org.lfdt.paladin.sdk.core.types.EthAddress;

class PaladinClientTest {

  private static final String GROUP_ADDRESS = "0x0102030405060708090a0b0c0d0e0f1011121314";

  /** Replies to each method with the canned result the corresponding namespace client expects. */
  private static final Map<String, String> RESULTS =
      Map.of(
          "ptx_getTransaction", "{\"id\":\"00000000-0000-0000-0000-0000000000aa\"}",
          "keymgr_wallets", "[{\"name\":\"signer-1\",\"keySelector\":\".*\"}]",
          "bidx_getConfirmedBlockHeight", "\"0x2a\"",
          "reg_registries", "[\"registry-1\"]",
          "pstate_listSchemas",
              "[{\"id\":\"0x1111111111111111111111111111111111111111111111111111111111111111\",\"domain\":\"noto\",\"type\":\"abi\"}]",
          "transport_nodeName", "\"node1\"",
          "pgroup_getGroupByAddress", "{\"domain\":\"pente\",\"name\":\"group-1\"}");

  private static MockJsonRpcServer serverForAllNamespaces() throws IOException {
    return new MockJsonRpcServer(
        (n, req) -> {
          final String method = req.get("method").asText();
          final String result = RESULTS.get(method);
          if (result == null) {
            return MockJsonRpcServer.Response.of(
                200,
                "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"error\":{\"code\":-32601,"
                    + "\"message\":\"unexpected method "
                    + method
                    + "\"}}");
          }
          return MockJsonRpcServer.Response.of(
              200, "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"result\":" + result + "}");
        });
  }

  private static RpcClientConfig config(final String url) {
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
  void everyNamespaceIsReachableOverTheOneTransport() throws IOException {
    try (MockJsonRpcServer server = serverForAllNamespaces();
        PaladinClient paladin = PaladinClient.http(config(server.baseUrl()))) {

      assertNotNull(
          paladin.ptx().getTransaction(java.util.UUID.randomUUID()).join(),
          "ptx namespace should route through the shared transport");
      assertEquals("signer-1", paladin.keyManager().wallets().join().get(0).name());
      assertEquals(42L, paladin.blockIndex().getConfirmedBlockHeight().join().asUnsignedLong());
      assertEquals(List.of("registry-1"), paladin.registry().registries().join());
      assertEquals("noto", paladin.stateStore().listSchemas("noto").join().get(0).domain());
      assertEquals("node1", paladin.transport().nodeName().join());
      assertEquals(
          "group-1",
          paladin
              .privacyGroups()
              .getGroupByAddress(EthAddress.fromString(GROUP_ADDRESS))
              .join()
              .name());

      final List<String> methods = new ArrayList<>();
      server.requests().forEach(r -> methods.add(r.get("method").asText()));
      assertEquals(
          List.of(
              "ptx_getTransaction",
              "keymgr_wallets",
              "bidx_getConfirmedBlockHeight",
              "reg_registries",
              "pstate_listSchemas",
              "transport_nodeName",
              "pgroup_getGroupByAddress"),
          methods);
    }
  }

  @Test
  void namespaceAccessorsReturnTheSameClientEveryTime() {
    final RecordingRpcClient rpc = new RecordingRpcClient();
    final PaladinClient paladin = PaladinClient.wrap(rpc);

    assertSame(rpc, paladin.rpc());
    assertSame(paladin.ptx(), paladin.ptx());
    assertSame(paladin.keyManager(), paladin.keyManager());
    assertSame(paladin.blockIndex(), paladin.blockIndex());
    assertSame(paladin.registry(), paladin.registry());
    assertSame(paladin.stateStore(), paladin.stateStore());
    assertSame(paladin.transport(), paladin.transport());
    assertSame(paladin.privacyGroups(), paladin.privacyGroups());
  }

  @Test
  void wrapDoesNotCloseTheCallersTransport() {
    final RecordingRpcClient rpc = new RecordingRpcClient();
    final PaladinClient paladin = PaladinClient.wrap(rpc);

    paladin.close();

    assertFalse(rpc.closed, "a borrowed transport must outlive the client that wrapped it");
  }

  @Test
  void closeReleasesATransportThisClientCreated() throws IOException {
    try (MockJsonRpcServer server = serverForAllNamespaces()) {
      final PaladinClient paladin = PaladinClient.http(config(server.baseUrl()));
      assertEquals("node1", paladin.transport().nodeName().join());

      paladin.close();
      paladin.close(); // idempotent

      // The transport is gone, so further calls fail rather than reaching the node.
      assertThrows(Exception.class, () -> paladin.transport().nodeName().join());
      assertEquals(1, server.requests().size());
    }
  }

  @Test
  void httpFromUrlUsesTheTransportDefaults() throws IOException {
    try (MockJsonRpcServer server = serverForAllNamespaces();
        PaladinClient paladin = PaladinClient.http(server.baseUrl())) {
      assertEquals("node1", paladin.transport().nodeName().join());
    }
  }

  @Test
  void factoriesRejectMissingArguments() {
    assertThrows(NullPointerException.class, () -> PaladinClient.http((RpcClientConfig) null));
    assertThrows(NullPointerException.class, () -> PaladinClient.http((String) null));
    assertThrows(NullPointerException.class, () -> PaladinClient.wrap(null));
  }

  @Test
  void newTxBuildsAgainstThisClient() {
    final PaladinClient paladin = PaladinClient.wrap(new RecordingRpcClient());

    final TransactionInput tx =
        paladin.newTx().publicTx().from("alice").to(GROUP_ADDRESS).function("transfer").build();

    assertEquals("alice", tx.from());
    assertEquals(EthAddress.fromString(GROUP_ADDRESS), tx.to());
  }

  @Test
  void forAbiPreSetsTheAbiOnTheBuilder() {
    final PaladinClient paladin = PaladinClient.wrap(new RecordingRpcClient());
    final List<AbiEntry> abi =
        List.of(
            AbiEntry.builder(EntryType.FUNCTION)
                .name("transfer")
                .inputs(List.of(AbiParameter.of("amount", "uint256")))
                .build());

    final TxBuilder builder = paladin.forAbi(abi);
    final TransactionInput tx =
        builder.publicTx().from("alice").to(GROUP_ADDRESS).function("transfer").build();

    assertEquals(abi, tx.abi());
    assertTrue(tx.abi().get(0).name().equals("transfer"));
  }

  /** An {@link RpcClient} that answers nothing and only records whether it was closed. */
  private static final class RecordingRpcClient implements RpcClient {
    private boolean closed;

    @Override
    public <T> CompletableFuture<T> callRpc(
        final Class<T> resultType, final String method, final Object... params) {
      return CompletableFuture.completedFuture(null);
    }

    @Override
    public <T> CompletableFuture<T> callRpc(
        final TypeReference<T> resultType, final String method, final Object... params) {
      return CompletableFuture.completedFuture(null);
    }

    @Override
    public void close() {
      closed = true;
    }
  }
}
