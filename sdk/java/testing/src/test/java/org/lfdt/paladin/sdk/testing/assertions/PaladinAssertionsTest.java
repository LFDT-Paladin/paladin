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
package org.lfdt.paladin.sdk.testing.assertions;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Duration;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.testing.fixtures.PaladinFixtures;
import org.lfdt.paladin.sdk.testing.mock.MockRpcClient;

class PaladinAssertionsTest {

  private static final UUID ID = UUID.randomUUID();

  @Test
  void fixturesProduceTheExpectedShapes() {
    final EthAddress contract = PaladinFixtures.randomAddress();
    final TransactionReceipt ok = PaladinFixtures.successReceipt(ID).build();
    final TransactionReceipt deployed = PaladinFixtures.deployReceipt(ID, contract).build();
    final TransactionReceipt failed = PaladinFixtures.failedReceipt(ID, "reverted").build();

    assertTrue(ok.success());
    assertEquals(1L, ok.blockNumber());
    assertNull(ok.contractAddress());
    assertEquals(contract, deployed.contractAddress());
    assertEquals("reverted", failed.failureMessage());
    assertNotEquals(PaladinFixtures.randomBytes32(), PaladinFixtures.randomBytes32());
    assertTrue(PaladinFixtures.uniqueIdentity("node1").matches("test\\.[0-9a-f-]{36}\\.node1"));
  }

  @Test
  void assertSuccessOnReceipts() {
    final TransactionReceipt ok = PaladinFixtures.successReceipt(ID).build();
    assertSame(ok, PaladinAssertions.assertSuccess(ok));
    assertThrows(
        AssertionError.class, () -> PaladinAssertions.assertSuccess((TransactionReceipt) null));
    final AssertionError failed =
        assertThrows(
            AssertionError.class,
            () ->
                PaladinAssertions.assertSuccess(
                    PaladinFixtures.failedReceipt(ID, "out of gas").build()));
    assertTrue(failed.getMessage().contains("out of gas"));
  }

  @Test
  void assertSuccessOnSentTransactions() {
    final MockRpcClient rpc = new MockRpcClient();
    rpc.on("ptx_sendTransaction").thenReturn(ID);
    rpc.on("ptx_getTransactionReceipt").thenReturn(PaladinFixtures.successReceipt(ID).build());
    assertEquals(ID, PaladinAssertions.assertSuccess(send(rpc), Duration.ofSeconds(1)).id());

    rpc.on("ptx_sendTransaction").thenFail(-32603, "send failed");
    final AssertionError error =
        assertThrows(
            AssertionError.class,
            () -> PaladinAssertions.assertSuccess(send(rpc), Duration.ofSeconds(1)));
    assertTrue(error.getCause() instanceof PaladinRpcException);
  }

  private static SentTransaction send(final MockRpcClient rpc) {
    return rpc.paladinClient()
        .newTx()
        .publicTx()
        .from("alice")
        .to("0x0102030405060708090a0b0c0d0e0f1011121314")
        .function("transfer")
        .send();
  }

  @Test
  void assertFailedChecksTheMessage() {
    final TransactionReceipt failed =
        PaladinFixtures.failedReceipt(ID, "execution reverted").build();
    assertSame(failed, PaladinAssertions.assertFailed(failed, "reverted"));
    assertSame(failed, PaladinAssertions.assertFailed(failed, null));
    assertThrows(AssertionError.class, () -> PaladinAssertions.assertFailed(failed, "gas"));
    assertThrows(AssertionError.class, () -> PaladinAssertions.assertFailed(null, null));
    assertThrows(
        AssertionError.class,
        () -> PaladinAssertions.assertFailed(PaladinFixtures.successReceipt(ID).build(), null));
    assertThrows(
        AssertionError.class,
        () ->
            PaladinAssertions.assertFailed(
                PaladinFixtures.successReceipt(ID).success(false).build(), "x"));
  }

  @Test
  void assertRpcErrorChecksTypeAndCode() {
    final PaladinRpcException error = new PaladinRpcException(-32602, "bad", null, 200);
    assertSame(
        error, PaladinAssertions.assertRpcError(CompletableFuture.failedFuture(error), -32602));
    assertThrows(
        AssertionError.class,
        () -> PaladinAssertions.assertRpcError(CompletableFuture.failedFuture(error), -32601));
    assertThrows(
        AssertionError.class,
        () ->
            PaladinAssertions.assertRpcError(
                CompletableFuture.failedFuture(new IllegalStateException()), -32602));
    assertThrows(
        AssertionError.class,
        () -> PaladinAssertions.assertRpcError(CompletableFuture.completedFuture("ok"), -32602));
  }

  @Test
  void assertRpcErrorReportsInterruption() {
    Thread.currentThread().interrupt();
    try {
      assertThrows(
          AssertionError.class,
          () -> PaladinAssertions.assertRpcError(new CompletableFuture<>(), -32602));
      assertTrue(Thread.interrupted());
    } finally {
      Thread.interrupted();
    }
  }

  @Test
  void awaitReturnsOnceTheConditionHolds() {
    final AtomicInteger counter = new AtomicInteger();
    final int value =
        Await.until(
            "counter to reach 3",
            Duration.ofSeconds(5),
            Duration.ofMillis(1),
            () -> {
              final int n = counter.incrementAndGet();
              if (n == 1) {
                throw new IllegalStateException("not ready");
              }
              return n;
            },
            n -> n >= 3);
    assertEquals(3, value);
    assertEquals("x", Await.until("immediate", () -> "x", "x"::equals));
  }

  @Test
  void awaitTimesOutWithTheLastValueAndError() {
    final AssertionError timedOut =
        assertThrows(
            AssertionError.class,
            () ->
                Await.until(
                    "never", Duration.ofMillis(20), Duration.ofMillis(5), () -> 7, n -> false));
    assertTrue(timedOut.getMessage().contains("never"));
    assertTrue(timedOut.getMessage().contains("7"));

    final AssertionError failing =
        assertThrows(
            AssertionError.class,
            () ->
                Await.until(
                    "throws",
                    Duration.ofMillis(20),
                    Duration.ofMillis(5),
                    () -> {
                      throw new IllegalStateException("down");
                    },
                    n -> true));
    assertTrue(failing.getCause() instanceof IllegalStateException);
  }

  @Test
  void awaitReportsInterruption() {
    Thread.currentThread().interrupt();
    try {
      assertThrows(
          AssertionError.class,
          () ->
              Await.until("x", Duration.ofSeconds(1), Duration.ofMillis(50), () -> 1, n -> false));
    } finally {
      Thread.interrupted();
    }
  }
}
