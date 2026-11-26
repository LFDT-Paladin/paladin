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
import java.util.Objects;
import java.util.function.Predicate;
import java.util.function.Supplier;

/**
 * Polls until a value satisfies a condition — for state that settles asynchronously on a node, such
 * as a privacy group arriving on a member or a balance propagating.
 *
 * <pre>{@code
 * PrivacyGroup group = Await.until(
 *     "privacy group on node2",
 *     () -> node2.privacyGroups().getGroupById("pente", id).join(),
 *     g -> g != null && g.contractAddress() != null);
 * }</pre>
 */
public final class Await {

  /** Default overall wait. */
  public static final Duration DEFAULT_TIMEOUT = Duration.ofSeconds(30);

  /** Default pause between probes. */
  public static final Duration DEFAULT_INTERVAL = Duration.ofMillis(200);

  private Await() {}

  /**
   * Polls with the default timeout and interval.
   *
   * @param description what is awaited, for the failure message
   * @param probe reads the current value
   * @param done the condition the value must reach
   * @param <T> the value type
   * @return the first value that satisfied {@code done}
   * @throws AssertionError if the condition is not met in time; carries the last value seen
   */
  public static <T> T until(
      final String description, final Supplier<T> probe, final Predicate<? super T> done) {
    return until(description, DEFAULT_TIMEOUT, DEFAULT_INTERVAL, probe, done);
  }

  /**
   * Polls until {@code done} accepts the probed value or {@code timeout} passes. A probe that
   * throws counts as "not yet" and is retried; its last exception is attached to the failure.
   *
   * @param description what is awaited, for the failure message
   * @param timeout the overall wait
   * @param interval the pause between probes
   * @param probe reads the current value
   * @param done the condition the value must reach
   * @param <T> the value type
   * @return the first value that satisfied {@code done}
   * @throws AssertionError if the condition is not met in time; carries the last value seen
   */
  public static <T> T until(
      final String description,
      final Duration timeout,
      final Duration interval,
      final Supplier<T> probe,
      final Predicate<? super T> done) {
    Objects.requireNonNull(probe, "probe");
    Objects.requireNonNull(done, "done");
    final long deadline = System.nanoTime() + timeout.toNanos();
    T last = null;
    RuntimeException lastError = null;
    while (true) {
      try {
        last = probe.get();
        lastError = null;
        if (done.test(last)) {
          return last;
        }
      } catch (final RuntimeException e) {
        lastError = e;
      }
      if (System.nanoTime() >= deadline) {
        throw new AssertionError(
            "timed out after "
                + timeout.toMillis()
                + "ms waiting for "
                + description
                + "; last value: "
                + last,
            lastError);
      }
      sleep(interval);
    }
  }

  private static void sleep(final Duration interval) {
    try {
      Thread.sleep(interval.toMillis());
    } catch (final InterruptedException e) {
      Thread.currentThread().interrupt();
      throw new AssertionError("interrupted while waiting", e);
    }
  }
}
