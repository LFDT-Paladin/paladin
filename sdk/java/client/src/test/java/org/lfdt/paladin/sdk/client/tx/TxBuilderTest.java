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
package org.lfdt.paladin.sdk.client.tx;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import java.io.IOException;
import java.math.BigInteger;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.config.RetryPolicy;
import org.lfdt.paladin.sdk.client.config.RpcClientConfig;
import org.lfdt.paladin.sdk.client.exception.PaladinInvalidTransactionException;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.exception.PaladinTimeoutException;
import org.lfdt.paladin.sdk.client.ptx.PtxClient;
import org.lfdt.paladin.sdk.client.rpc.HttpRpcClient;
import org.lfdt.paladin.sdk.client.rpc.MockJsonRpcServer;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.AbiParameter;
import org.lfdt.paladin.sdk.core.transaction.TransactionInput;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.transaction.TransactionType;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class TxBuilderTest {

  private static final String TX_ID = "00000000-0000-0000-0000-0000000000aa";
  private static final String CONTRACT = "0x0102030405060708090a0b0c0d0e0f1011121314";

  // -----------------------------------------------------------------------------------------
  // Harness
  // -----------------------------------------------------------------------------------------

  private static String success(final String resultJson) {
    return "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"result\":" + resultJson + "}";
  }

  private static String receiptJson(final boolean ok) {
    return "{\"id\":\""
        + TX_ID
        + "\",\"sequence\":1,\"success\":"
        + ok
        + (ok ? ",\"blockNumber\":42" : ",\"failureMessage\":\"reverted: nope\"")
        + "}";
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

  /**
   * Answers {@code ptx_sendTransaction} with a fixed id and {@code ptx_getTransactionReceipt} with
   * a null result until {@code nullReceipts} polls have been served, then with {@code receipt}.
   */
  private static MockJsonRpcServer.Responder node(final int nullReceipts, final String receipt) {
    final int[] receiptCalls = {0};
    return (n, req) -> {
      final String method = req.get("method").asText();
      if ("ptx_sendTransaction".equals(method)) {
        return MockJsonRpcServer.Response.of(200, success("\"" + TX_ID + "\""));
      }
      receiptCalls[0]++;
      final boolean stillPending = receiptCalls[0] <= nullReceipts;
      return MockJsonRpcServer.Response.of(
          200, success(stillPending || receipt == null ? "null" : receipt));
    };
  }

  /** Runs {@code body} against a mock node, closing both the server and the client afterwards. */
  private static void withNode(final MockJsonRpcServer.Responder responder, final NodeTest body)
      throws IOException {
    try (MockJsonRpcServer server = new MockJsonRpcServer(responder);
        HttpRpcClient rpc = new HttpRpcClient(config(server.baseUrl()))) {
      body.run(server, rpc);
    }
  }

  @FunctionalInterface
  private interface NodeTest {
    void run(MockJsonRpcServer server, HttpRpcClient rpc) throws IOException;
  }

  /** A builder pre-populated to the point where it would validate cleanly. */
  private static TxBuilder validInvoke(final HttpRpcClient rpc) {
    return TxBuilder.on(rpc).publicTx().from("alice").to(CONTRACT).function("transfer");
  }

  private static Throwable causeOf(final Executable e) {
    final CompletionException thrown = assertThrows(CompletionException.class, e::execute);
    return thrown.getCause();
  }

  @FunctionalInterface
  private interface Executable {
    void execute();
  }

  // -----------------------------------------------------------------------------------------
  // Construction
  // -----------------------------------------------------------------------------------------

  @Test
  void buildAssemblesEveryField() throws IOException {
    final UUID dependency = UUID.randomUUID();
    final Bytes32 abiRef = Bytes32.fromString("0x" + "11".repeat(32));
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TransactionInput tx =
              TxBuilder.on(rpc)
                  .privateTx()
                  .domain("noto")
                  .from("alice")
                  .to(EthAddress.fromString(CONTRACT))
                  .function("transfer")
                  .idempotencyKey("biz-1")
                  .abiReference(abiRef)
                  .inputs(Map.of("amount", 100))
                  .gas(21_000L)
                  .value(7L)
                  .maxFeePerGas(BigInteger.valueOf(1_000))
                  .maxPriorityFeePerGas(BigInteger.valueOf(500))
                  .dependsOn(dependency)
                  .build();

          assertEquals(TransactionType.PRIVATE, tx.type());
          assertEquals("noto", tx.domain());
          assertEquals("alice", tx.from());
          assertEquals(EthAddress.fromString(CONTRACT), tx.to());
          assertEquals("transfer", tx.function());
          assertEquals("biz-1", tx.idempotencyKey());
          assertEquals(abiRef, tx.abiReference());
          assertEquals(100, tx.data().get("amount").asInt());
          assertEquals(21_000L, tx.gas().asUnsignedLong());
          assertEquals(BigInteger.valueOf(7), tx.value().bigIntegerValue());
          assertEquals(BigInteger.valueOf(1_000), tx.maxFeePerGas().bigIntegerValue());
          assertEquals(BigInteger.valueOf(500), tx.maxPriorityFeePerGas().bigIntegerValue());
          assertEquals(List.of(dependency), tx.dependsOn());
        });
  }

  @Test
  void buildIsRepeatableAndLeavesTheBuilderReusable() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TxBuilder builder = validInvoke(rpc);
          assertEquals(builder.build(), builder.build());
          assertEquals("mint", builder.function("mint").build().function());
        });
  }

  @Test
  void abiIsCollectedFromEntriesAndJson() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TransactionInput tx =
              validInvoke(rpc)
                  .abiEntry(
                      AbiEntry.function("transfer").input(AbiParameter.of("to", "address")).build())
                  .abi(List.of(AbiEntry.function("burn").build()))
                  .abiJson("[{\"type\":\"function\",\"name\":\"mint\",\"inputs\":[]}]")
                  .build();

          assertEquals(3, tx.abi().size());
          assertEquals("transfer", tx.abi().get(0).name());
          assertEquals("burn", tx.abi().get(1).name());
          assertEquals("mint", tx.abi().get(2).name());
        });
  }

  @Test
  void dependsOnAcceptsVarargsAndList() throws IOException {
    final UUID one = UUID.randomUUID();
    final UUID two = UUID.randomUUID();
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) ->
            assertEquals(
                List.of(one, two),
                validInvoke(rpc).dependsOn(one).dependsOn(List.of(two)).build().dependsOn()));
  }

  @Test
  void inputsJsonAcceptsPositionalArrays() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final JsonNode data = validInvoke(rpc).inputsJson("[\"0xabc\", 42]").build().data();
          assertTrue(data.isArray());
          assertEquals(42, data.get(1).asInt());
        });
  }

  @Test
  void constructorClearsFunctionAndTarget() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TransactionInput tx = validInvoke(rpc).bytecode("0xdeadbeef").constructor().build();
          assertNull(tx.to());
          assertNull(tx.function());
        });
  }

  @Test
  void onPtxClientUsesTheGivenClient() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TransactionReceipt receipt =
              TxBuilder.on(new PtxClient(rpc))
                  .publicTx()
                  .from("alice")
                  .to(CONTRACT)
                  .function("transfer")
                  .send()
                  .join();
          assertTrue(receipt.success());
        });
  }

  // -----------------------------------------------------------------------------------------
  // Deferred errors — chaining never throws
  // -----------------------------------------------------------------------------------------

  @Test
  void malformedAddressDoesNotThrowUntilBuild() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          // The whole chain completes without throwing, even though 'to' is nonsense.
          final TxBuilder builder =
              TxBuilder.on(rpc).publicTx().from("alice").to("not-an-address").function("transfer");

          final PaladinInvalidTransactionException e =
              assertThrows(PaladinInvalidTransactionException.class, builder::build);
          assertTrue(e.getMessage().contains("invalid 'to' address"));
          assertNotNull(e.getCause());
        });
  }

  @Test
  void malformedAbiJsonIsDeferred() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TxBuilder builder = validInvoke(rpc).abiJson("{not json");
          final PaladinInvalidTransactionException e =
              assertThrows(PaladinInvalidTransactionException.class, builder::build);
          assertEquals("invalid ABI JSON", e.getMessage());
        });
  }

  @Test
  void malformedInputsJsonIsDeferred() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TxBuilder builder = validInvoke(rpc).inputsJson("{oops");
          assertEquals(
              "invalid transaction inputs JSON",
              assertThrows(PaladinInvalidTransactionException.class, builder::build).getMessage());
        });
  }

  @Test
  void unserializableInputsAreDeferred() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          // A bean with no properties and no serializer configured cannot be turned into JSON.
          final TxBuilder builder = validInvoke(rpc).inputs(new Object());
          assertEquals(
              "invalid transaction inputs",
              assertThrows(PaladinInvalidTransactionException.class, builder::build).getMessage());
        });
  }

  @Test
  void malformedBytecodeIsDeferred() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TxBuilder builder = validInvoke(rpc).bytecode("zzzz");
          assertEquals(
              "invalid bytecode",
              assertThrows(PaladinInvalidTransactionException.class, builder::build).getMessage());
        });
  }

  @Test
  void nonPositivePollingSettingsAreDeferred() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          assertTrue(
              assertThrows(
                      PaladinInvalidTransactionException.class,
                      () -> validInvoke(rpc).pollingInterval(Duration.ZERO).build())
                  .getMessage()
                  .contains("polling interval must be positive"));
          assertTrue(
              assertThrows(
                      PaladinInvalidTransactionException.class,
                      () -> validInvoke(rpc).receiptTimeout(Duration.ofSeconds(-1)).build())
                  .getMessage()
                  .contains("receipt timeout must be positive"));
          assertTrue(
              assertThrows(
                      PaladinInvalidTransactionException.class,
                      () -> validInvoke(rpc).receiptTimeout(null).build())
                  .getMessage()
                  .contains("receipt timeout must be positive"));
          // The two setters reject the same shapes, so check the mirrored cases too.
          assertTrue(
              assertThrows(
                      PaladinInvalidTransactionException.class,
                      () -> validInvoke(rpc).pollingInterval(null).build())
                  .getMessage()
                  .contains("polling interval must be positive"));
          assertTrue(
              assertThrows(
                      PaladinInvalidTransactionException.class,
                      () -> validInvoke(rpc).pollingInterval(Duration.ofMillis(-1)).build())
                  .getMessage()
                  .contains("polling interval must be positive"));
          assertTrue(
              assertThrows(
                      PaladinInvalidTransactionException.class,
                      () -> validInvoke(rpc).receiptTimeout(Duration.ZERO).build())
                  .getMessage()
                  .contains("receipt timeout must be positive"));
        });
  }

  @Test
  void valueAcceptsBigIntegersBeyondLongRange() throws IOException {
    final BigInteger huge = BigInteger.TWO.pow(70);
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) ->
            assertEquals(huge, validInvoke(rpc).value(huge).build().value().bigIntegerValue()));
  }

  @Test
  void firstDeferredErrorWins() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TxBuilder builder =
              TxBuilder.on(rpc)
                  .publicTx()
                  .from("alice")
                  .to("not-an-address")
                  .function("transfer")
                  .abiJson("{not json")
                  .bytecode("zzzz");
          assertTrue(
              assertThrows(PaladinInvalidTransactionException.class, builder::build)
                  .getMessage()
                  .contains("invalid 'to' address"));
        });
  }

  @Test
  void deferredErrorFailsTheFutureRatherThanThrowing() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TxBuilder builder =
              TxBuilder.on(rpc).publicTx().from("alice").to("not-an-address").function("transfer");

          // Neither terminal throws synchronously; both hand the error to the future.
          assertInstanceOf(
              PaladinInvalidTransactionException.class, causeOf(() -> builder.submit().join()));
          assertInstanceOf(
              PaladinInvalidTransactionException.class, causeOf(() -> builder.send().join()));
          assertEquals(0, server.requestCount(), "nothing should reach the node");
        });
  }

  // -----------------------------------------------------------------------------------------
  // Validation
  // -----------------------------------------------------------------------------------------

  @Test
  void typeIsRequired() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) ->
            assertTrue(
                assertThrows(
                        PaladinInvalidTransactionException.class,
                        () -> TxBuilder.on(rpc).from("alice").to(CONTRACT).function("f").build())
                    .getMessage()
                    .contains("transaction type is required")));
  }

  @Test
  void signingIdentityIsRequired() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) ->
            assertTrue(
                assertThrows(
                        PaladinInvalidTransactionException.class,
                        () -> TxBuilder.on(rpc).publicTx().to(CONTRACT).function("f").build())
                    .getMessage()
                    .contains("signing identity is required")));
  }

  @Test
  void privateTransactionRequiresADomain() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) ->
            assertTrue(
                assertThrows(
                        PaladinInvalidTransactionException.class,
                        () ->
                            TxBuilder.on(rpc)
                                .privateTx()
                                .from("alice")
                                .to(CONTRACT)
                                .function("f")
                                .build())
                    .getMessage()
                    .contains("domain is required")));
  }

  @Test
  void invokeRequiresATarget() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) ->
            assertTrue(
                assertThrows(
                        PaladinInvalidTransactionException.class,
                        () -> TxBuilder.on(rpc).publicTx().from("alice").function("f").build())
                    .getMessage()
                    .contains("target address is required to invoke function 'f'")));
  }

  @Test
  void targetWithoutAFunctionIsRejected() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) ->
            assertTrue(
                assertThrows(
                        PaladinInvalidTransactionException.class,
                        () -> TxBuilder.on(rpc).publicTx().from("alice").to(CONTRACT).build())
                    .getMessage()
                    .contains("function is required when a target address is set")));
  }

  @Test
  void publicDeployRequiresBytecode() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          assertTrue(
              assertThrows(
                      PaladinInvalidTransactionException.class,
                      () -> TxBuilder.on(rpc).publicTx().from("alice").build())
                  .getMessage()
                  .contains("bytecode is required for a public deploy"));
          // Empty bytecode counts as absent, matching the Go builder.
          assertTrue(
              assertThrows(
                      PaladinInvalidTransactionException.class,
                      () ->
                          TxBuilder.on(rpc)
                              .publicTx()
                              .from("alice")
                              .bytecode(HexBytes.wrap(new byte[0]))
                              .build())
                  .getMessage()
                  .contains("bytecode is required for a public deploy"));
        });
  }

  @Test
  void privateDeployRejectsBytecode() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) ->
            assertTrue(
                assertThrows(
                        PaladinInvalidTransactionException.class,
                        () ->
                            TxBuilder.on(rpc)
                                .privateTx()
                                .domain("noto")
                                .from("alice")
                                .bytecode("0xdeadbeef")
                                .build())
                    .getMessage()
                    .contains("bytecode cannot be supplied for a private deploy")));
  }

  @Test
  void validDeploysBuild() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          assertEquals(
              HexBytes.fromString("0xdeadbeef"),
              TxBuilder.on(rpc).publicTx().from("alice").bytecode("0xdeadbeef").build().bytecode());
          assertNull(TxBuilder.on(rpc).privateTx().domain("noto").from("alice").build().bytecode());
        });
  }

  // -----------------------------------------------------------------------------------------
  // Submit and receipt polling
  // -----------------------------------------------------------------------------------------

  @Test
  void submitSendsWithoutPolling() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final UUID id = validInvoke(rpc).submit().join();
          assertEquals(UUID.fromString(TX_ID), id);
          assertEquals(1, server.requestCount(), "submit must not poll for a receipt");
          assertEquals("ptx_sendTransaction", server.requests().get(0).get("method").asText());
          final JsonNode body = server.requests().get(0).get("params").get(0);
          assertEquals("public", body.get("type").asText());
          assertEquals("alice", body.get("from").asText());
          assertEquals("transfer", body.get("function").asText());
        });
  }

  @Test
  void sendReturnsTheReceiptOnTheFirstPoll() throws IOException {
    withNode(
        node(0, receiptJson(true)),
        (server, rpc) -> {
          final TransactionReceipt receipt = validInvoke(rpc).send().join();
          assertTrue(receipt.success());
          assertEquals(42L, receipt.blockNumber());
          assertEquals(2, server.requestCount(), "one send plus one receipt poll");
          assertEquals(
              "ptx_getTransactionReceipt", server.requests().get(1).get("method").asText());
          assertEquals(TX_ID, server.requests().get(1).get("params").get(0).asText());
        });
  }

  @Test
  void sendPollsUntilTheReceiptLands() throws IOException {
    withNode(
        node(2, receiptJson(true)),
        (server, rpc) -> {
          final TransactionReceipt receipt =
              validInvoke(rpc).pollingInterval(Duration.ofMillis(5)).send().join();
          assertTrue(receipt.success());
          assertEquals(4, server.requestCount(), "one send plus three receipt polls");
        });
  }

  @Test
  void sendCompletesNormallyForARevertedTransaction() throws IOException {
    withNode(
        node(0, receiptJson(false)),
        (server, rpc) -> {
          final TransactionReceipt receipt = validInvoke(rpc).send().join();
          assertFalse(receipt.success());
          assertEquals("reverted: nope", receipt.failureMessage());
        });
  }

  @Test
  void sendTimesOutWhenNoReceiptArrives() throws IOException {
    withNode(
        node(Integer.MAX_VALUE, null),
        (server, rpc) -> {
          final long start = System.nanoTime();
          final Throwable cause =
              causeOf(
                  () ->
                      validInvoke(rpc)
                          .pollingInterval(Duration.ofMillis(5))
                          .receiptTimeout(Duration.ofMillis(60))
                          .send()
                          .join());
          final Duration elapsed = Duration.ofNanos(System.nanoTime() - start);

          final PaladinTimeoutException timeout =
              assertInstanceOf(PaladinTimeoutException.class, cause);
          assertTrue(timeout.getMessage().contains("no receipt for transaction " + TX_ID));
          assertTrue(timeout.getMessage().contains("60ms"));
          assertTrue(
              elapsed.toMillis() >= 60,
              "must wait out the full timeout, waited " + elapsed.toMillis() + "ms");
          assertTrue(server.requestCount() > 2, "should have polled repeatedly");
        });
  }

  @Test
  void aPollingIntervalCoarserThanTheTimeoutStillTimesOutPromptly() throws IOException {
    withNode(
        node(Integer.MAX_VALUE, null),
        (server, rpc) -> {
          final long start = System.nanoTime();
          assertInstanceOf(
              PaladinTimeoutException.class,
              causeOf(
                  () ->
                      validInvoke(rpc)
                          .pollingInterval(Duration.ofSeconds(30))
                          .receiptTimeout(Duration.ofMillis(50))
                          .send()
                          .join()));
          // The sleep is clamped to the time remaining, so we do not wait out the 30s interval.
          assertTrue(Duration.ofNanos(System.nanoTime() - start).toSeconds() < 5);
        });
  }

  @Test
  void sendPropagatesATransportFailureFromPolling() throws IOException {
    withNode(
        (n, req) ->
            "ptx_sendTransaction".equals(req.get("method").asText())
                ? MockJsonRpcServer.Response.of(200, success("\"" + TX_ID + "\""))
                : MockJsonRpcServer.Response.of(
                    200,
                    "{\"jsonrpc\":\"2.0\",\"id\":\"x\",\"error\":{\"code\":-32000,"
                        + "\"message\":\"PD012345: boom\"}}"),
        (server, rpc) ->
            assertInstanceOf(
                PaladinRpcException.class, causeOf(() -> validInvoke(rpc).send().join())));
  }

  @Test
  void pollingSettingsAreCapturedWhenSendIsCalled() throws IOException {
    withNode(
        node(1, receiptJson(true)),
        (server, rpc) -> {
          final TxBuilder builder = validInvoke(rpc).pollingInterval(Duration.ofMillis(5));
          final TransactionReceipt receipt = builder.send().join();
          // Mutating the builder afterwards must not disturb the in-flight send.
          assertSame(builder, builder.pollingInterval(Duration.ofMinutes(5)));
          assertTrue(receipt.success());
        });
  }
}
