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
package org.lfdt.paladin.sdk.client.config;

import java.time.Duration;
import java.util.Objects;

/**
 * Exponential-backoff retry policy for the HTTP transport, mirroring Go's {@code retry.Retry}.
 *
 * <p>Immutable; build with {@link #builder()} or take the SDK defaults via {@link #defaults()}.
 * Only transient failures (connection errors, timeouts, and transient HTTP statuses such as {@code
 * 429}/{@code 5xx}) are retried — JSON-RPC application errors and other {@code 4xx} responses are
 * not. The delay before the retry that follows a given failed attempt is {@code initialDelay *
 * factor^(attempt-1)}, capped at {@code maxDelay}; see {@link #delayForAttempt(int)}.
 *
 * <p>Defaults mirror Go's {@code GenericRetryDefaults}/{@code DefaultHTTPConfig}: initial delay
 * 250ms, max delay 30s, factor 2.0, with a bounded {@code maxAttempts} of 5.
 */
public final class RetryPolicy {

  /** The default number of attempts (the initial try plus retries). */
  public static final int DEFAULT_MAX_ATTEMPTS = 5;

  /** The default delay before the first retry. */
  public static final Duration DEFAULT_INITIAL_DELAY = Duration.ofMillis(250);

  /** The default ceiling on the per-retry delay. */
  public static final Duration DEFAULT_MAX_DELAY = Duration.ofSeconds(30);

  /** The default multiplier applied to the delay after each failed attempt. */
  public static final double DEFAULT_FACTOR = 2.0;

  private static final RetryPolicy DEFAULTS = builder().build();

  private final int maxAttempts;
  private final Duration initialDelay;
  private final Duration maxDelay;
  private final double factor;

  private RetryPolicy(final Builder b) {
    this.maxAttempts = b.maxAttempts;
    this.initialDelay = b.initialDelay;
    this.maxDelay = b.maxDelay;
    this.factor = b.factor;
  }

  /**
   * Returns the shared policy carrying the SDK defaults.
   *
   * @return the default retry policy
   */
  public static RetryPolicy defaults() {
    return DEFAULTS;
  }

  /**
   * Returns a new builder pre-populated with the SDK defaults.
   *
   * @return a new builder
   */
  public static Builder builder() {
    return new Builder();
  }

  /**
   * The maximum number of attempts, including the initial try; {@code <= 1} disables retries.
   *
   * @return the maximum number of attempts
   */
  public int maxAttempts() {
    return maxAttempts;
  }

  /**
   * The delay before the first retry.
   *
   * @return the initial backoff delay
   */
  public Duration initialDelay() {
    return initialDelay;
  }

  /**
   * The ceiling on the per-retry delay.
   *
   * @return the maximum backoff delay
   */
  public Duration maxDelay() {
    return maxDelay;
  }

  /**
   * The multiplier applied to the delay after each failed attempt.
   *
   * @return the backoff multiplier
   */
  public double factor() {
    return factor;
  }

  /**
   * Returns the backoff delay to wait before retrying, given the 1-based number of the attempt that
   * just failed. The first failure ({@code failedAttempt == 1}) waits {@link #initialDelay()}; each
   * subsequent failure multiplies by {@link #factor()}, capped at {@link #maxDelay()}. Mirrors Go's
   * {@code Retry.WaitDelay}.
   *
   * @param failedAttempt the 1-based number of the attempt that failed
   * @return the delay to wait before the next attempt
   */
  public Duration delayForAttempt(final int failedAttempt) {
    double delayMs = initialDelay.toMillis();
    final double capMs = maxDelay.toMillis();
    for (int i = 0; i < failedAttempt - 1; i++) {
      delayMs *= factor;
      if (delayMs >= capMs) {
        delayMs = capMs;
        break;
      }
    }
    return Duration.ofMillis((long) delayMs);
  }

  /**
   * Whether another attempt is permitted after the given 1-based attempt has failed.
   *
   * @param failedAttempt the 1-based number of the attempt that failed
   * @return {@code true} if {@code maxAttempts} has not yet been reached
   */
  public boolean shouldRetry(final int failedAttempt) {
    return failedAttempt < maxAttempts;
  }

  /** Builder for {@link RetryPolicy}, pre-populated with the SDK defaults. */
  public static final class Builder {

    private int maxAttempts = DEFAULT_MAX_ATTEMPTS;
    private Duration initialDelay = DEFAULT_INITIAL_DELAY;
    private Duration maxDelay = DEFAULT_MAX_DELAY;
    private double factor = DEFAULT_FACTOR;

    private Builder() {}

    /**
     * Sets the maximum number of attempts including the initial try ({@code <= 1} disables
     * retries).
     *
     * @param maxAttempts the attempt ceiling
     * @return this builder
     */
    public Builder maxAttempts(final int maxAttempts) {
      this.maxAttempts = maxAttempts;
      return this;
    }

    /**
     * Sets the delay before the first retry.
     *
     * @param initialDelay the initial backoff delay (must be non-negative)
     * @return this builder
     */
    public Builder initialDelay(final Duration initialDelay) {
      this.initialDelay = requireNonNegative(initialDelay, "initialDelay");
      return this;
    }

    /**
     * Sets the ceiling on the per-retry delay.
     *
     * @param maxDelay the maximum backoff delay (must be non-negative)
     * @return this builder
     */
    public Builder maxDelay(final Duration maxDelay) {
      this.maxDelay = requireNonNegative(maxDelay, "maxDelay");
      return this;
    }

    /**
     * Sets the multiplier applied to the delay after each failed attempt (must be {@code >= 1.0}).
     *
     * @param factor the backoff multiplier
     * @return this builder
     */
    public Builder factor(final double factor) {
      if (factor < 1.0) {
        throw new IllegalArgumentException("factor must be >= 1.0, was " + factor);
      }
      this.factor = factor;
      return this;
    }

    /**
     * Builds the immutable {@link RetryPolicy}.
     *
     * @return the configured retry policy
     */
    public RetryPolicy build() {
      return new RetryPolicy(this);
    }

    private static Duration requireNonNegative(final Duration value, final String name) {
      Objects.requireNonNull(value, name);
      if (value.isNegative()) {
        throw new IllegalArgumentException(name + " must not be negative, was " + value);
      }
      return value;
    }
  }
}
