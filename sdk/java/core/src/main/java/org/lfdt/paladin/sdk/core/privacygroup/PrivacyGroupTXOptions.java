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
package org.lfdt.paladin.sdk.core.privacygroup;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;

/**
 * The submission options applied to the genesis transaction of a new privacy group. Immutable;
 * build one with the {@linkplain #builder() fluent builder}.
 *
 * <p>The public-transaction options ({@link #gas()}, {@link #value()}, {@link
 * #maxPriorityFeePerGas()}, {@link #maxFeePerGas()}) are flattened onto the flat JSON wire form
 * alongside the idempotency key. Supplying either gas-pricing field fixes pricing for the
 * transaction, disabling the node's gas-pricing engine.
 */
@JsonPropertyOrder({"idempotencyKey", "gas", "value", "maxPriorityFeePerGas", "maxFeePerGas"})
public final class PrivacyGroupTXOptions {

  private final String idempotencyKey;
  private final HexUint64 gas;
  private final HexUint256 value;
  private final HexUint256 maxPriorityFeePerGas;
  private final HexUint256 maxFeePerGas;

  @JsonCreator
  PrivacyGroupTXOptions(
      @JsonProperty("idempotencyKey") final String idempotencyKey,
      @JsonProperty("gas") final HexUint64 gas,
      @JsonProperty("value") final HexUint256 value,
      @JsonProperty("maxPriorityFeePerGas") final HexUint256 maxPriorityFeePerGas,
      @JsonProperty("maxFeePerGas") final HexUint256 maxFeePerGas) {
    this.idempotencyKey = idempotencyKey;
    this.gas = gas;
    this.value = value;
    this.maxPriorityFeePerGas = maxPriorityFeePerGas;
    this.maxFeePerGas = maxFeePerGas;
  }

  /**
   * Externally supplied unique identifier for the genesis transaction; a re-submit with the same
   * key yields 409 Conflict.
   *
   * @return the idempotency key, or an empty string when unset
   */
  @JsonProperty("idempotencyKey")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String idempotencyKey() {
    return idempotencyKey;
  }

  /**
   * The gas limit for the genesis transaction.
   *
   * @return the gas limit, or {@code null} to let the node estimate
   */
  @JsonProperty("gas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 gas() {
    return gas;
  }

  /**
   * The native value to transfer with the genesis transaction.
   *
   * @return the native value, or {@code null} if unset
   */
  @JsonProperty("value")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 value() {
    return value;
  }

  /**
   * The EIP-1559 max priority fee per gas.
   *
   * @return the max priority fee per gas, or {@code null} if unset
   */
  @JsonProperty("maxPriorityFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxPriorityFeePerGas() {
    return maxPriorityFeePerGas;
  }

  /**
   * The EIP-1559 max fee per gas.
   *
   * @return the max fee per gas, or {@code null} if unset
   */
  @JsonProperty("maxFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxFeePerGas() {
    return maxFeePerGas;
  }

  /**
   * Starts an empty builder.
   *
   * @return a new builder
   */
  public static Builder builder() {
    return new Builder();
  }

  @Override
  public String toString() {
    return "PrivacyGroupTXOptions{idempotencyKey=" + idempotencyKey + ", gas=" + gas + "}";
  }

  /** Fluent builder for {@link PrivacyGroupTXOptions}. */
  public static final class Builder {
    private String idempotencyKey;
    private HexUint64 gas;
    private HexUint256 value;
    private HexUint256 maxPriorityFeePerGas;
    private HexUint256 maxFeePerGas;

    private Builder() {}

    /**
     * Sets the idempotency key for the genesis transaction.
     *
     * @param idempotencyKey the externally supplied unique identifier
     * @return this builder
     */
    public Builder idempotencyKey(final String idempotencyKey) {
      this.idempotencyKey = idempotencyKey;
      return this;
    }

    /**
     * Sets the gas limit.
     *
     * @param gas the gas limit, or {@code null} to let the node estimate
     * @return this builder
     */
    public Builder gas(final HexUint64 gas) {
      this.gas = gas;
      return this;
    }

    /**
     * Sets the native value to transfer.
     *
     * @param value the native value to transfer
     * @return this builder
     */
    public Builder value(final HexUint256 value) {
      this.value = value;
      return this;
    }

    /**
     * Sets the EIP-1559 max priority fee per gas.
     *
     * @param maxPriorityFeePerGas the max priority fee per gas; supplying it fixes gas pricing
     * @return this builder
     */
    public Builder maxPriorityFeePerGas(final HexUint256 maxPriorityFeePerGas) {
      this.maxPriorityFeePerGas = maxPriorityFeePerGas;
      return this;
    }

    /**
     * Sets the EIP-1559 max fee per gas.
     *
     * @param maxFeePerGas the max fee per gas; supplying it fixes gas pricing
     * @return this builder
     */
    public Builder maxFeePerGas(final HexUint256 maxFeePerGas) {
      this.maxFeePerGas = maxFeePerGas;
      return this;
    }

    /**
     * Builds the immutable {@link PrivacyGroupTXOptions}.
     *
     * @return a new {@link PrivacyGroupTXOptions} with the configured values
     */
    public PrivacyGroupTXOptions build() {
      return new PrivacyGroupTXOptions(
          idempotencyKey, gas, value, maxPriorityFeePerGas, maxFeePerGas);
    }
  }
}
