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

import java.time.Duration;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CompletionException;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;

/**
 * Assertions for receipts and RPC failures. Each throws {@link AssertionError}, so they work under
 * any test framework, and returns the checked value so a test can keep going with it.
 */
public final class PaladinAssertions {

  private PaladinAssertions() {}

  /**
   * Asserts that a receipt reports success.
   *
   * @param receipt the receipt
   * @return the same receipt
   * @throws AssertionError if the receipt is missing or failed; the message carries the node's
   *     failure message
   */
  public static TransactionReceipt assertSuccess(final TransactionReceipt receipt) {
    if (receipt == null) {
      throw new AssertionError("expected a successful receipt but there was none");
    }
    if (!receipt.success()) {
      throw new AssertionError(
          "expected transaction " + receipt.id() + " to succeed: " + receipt.failureMessage());
    }
    return receipt;
  }

  /**
   * Waits for a sent transaction's receipt and asserts that it reports success.
   *
   * @param sent the transaction
   * @param timeout how long to wait for the receipt
   * @return the receipt
   * @throws AssertionError if no receipt arrives in time, the send failed, or the receipt failed
   */
  public static TransactionReceipt assertSuccess(
      final SentTransaction sent, final Duration timeout) {
    final TransactionReceipt receipt;
    try {
      receipt = sent.waitForReceipt(timeout).join();
    } catch (final CompletionException e) {
      throw new AssertionError("transaction did not complete: " + e.getCause(), e.getCause());
    }
    return assertSuccess(receipt);
  }

  /**
   * Asserts that a receipt reports failure with a message containing {@code expectedMessagePart}.
   *
   * @param receipt the receipt
   * @param expectedMessagePart text the failure message must contain; {@code null} skips the check
   * @return the same receipt
   * @throws AssertionError if the receipt is missing, succeeded, or carries a different message
   */
  public static TransactionReceipt assertFailed(
      final TransactionReceipt receipt, final String expectedMessagePart) {
    if (receipt == null) {
      throw new AssertionError("expected a failed receipt but there was none");
    }
    if (receipt.success()) {
      throw new AssertionError(
          "expected transaction " + receipt.id() + " to fail but it succeeded");
    }
    final String message = receipt.failureMessage();
    if (expectedMessagePart != null
        && (message == null || !message.contains(expectedMessagePart))) {
      throw new AssertionError(
          "expected failure message containing '" + expectedMessagePart + "' but was: " + message);
    }
    return receipt;
  }

  /**
   * Asserts that a future fails with a JSON-RPC error carrying {@code expectedCode}.
   *
   * @param future the call under test
   * @param expectedCode the JSON-RPC error code
   * @return the exception, for further checks on its message or data
   * @throws AssertionError if the future succeeds, fails differently, or does not complete in 30s
   */
  public static PaladinRpcException assertRpcError(
      final CompletableFuture<?> future, final long expectedCode) {
    final Throwable failure = failureOf(future);
    if (!(failure instanceof PaladinRpcException rpcError)) {
      throw new AssertionError("expected a PaladinRpcException but got " + failure, failure);
    }
    if (rpcError.code() != expectedCode) {
      throw new AssertionError(
          "expected JSON-RPC error " + expectedCode + " but got " + rpcError.code(), rpcError);
    }
    return rpcError;
  }

  private static Throwable failureOf(final CompletableFuture<?> future) {
    final Object value;
    try {
      value = future.get(30, TimeUnit.SECONDS);
    } catch (final ExecutionException e) {
      return e.getCause();
    } catch (final TimeoutException e) {
      throw new AssertionError("expected the call to fail but it did not complete in 30s", e);
    } catch (final InterruptedException e) {
      Thread.currentThread().interrupt();
      throw new AssertionError("interrupted waiting for the call", e);
    }
    throw new AssertionError("expected the call to fail but it returned " + value);
  }
}
