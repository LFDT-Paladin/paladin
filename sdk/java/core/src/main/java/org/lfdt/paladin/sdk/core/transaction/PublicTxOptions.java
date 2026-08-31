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
package org.lfdt.paladin.sdk.core.transaction;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;

/**
 * The base-ledger submission options for a transaction — gas limit, native value, and EIP-1559 gas
 * pricing. Immutable; build one with the {@linkplain #builder() fluent builder}.
 *
 * <p>Supplying either gas-pricing field fixes pricing for the transaction, disabling the node's
 * gas-pricing engine. Every field is omitted from the JSON when unset.
 *
 * <p>Some request bodies flatten these fields onto their own wire form rather than nesting them;
 * this type is for the bodies that carry them as a nested {@code publicTxOptions} object.
 */
@JsonPropertyOrder({"gas", "value", "maxPriorityFeePerGas", "maxFeePerGas"})
public final class PublicTxOptions {

  private final HexUint64 gas;
  private final HexUint256 value;
  private final HexUint256 maxPriorityFeePerGas;
  private final HexUint256 maxFeePerGas;

  @JsonCreator
  PublicTxOptions(
      @JsonProperty("gas") final HexUint64 gas,
      @JsonProperty("value") final HexUint256 value,
      @JsonProperty("maxPriorityFeePerGas") final HexUint256 maxPriorityFeePerGas,
      @JsonProperty("maxFeePerGas") final HexUint256 maxFeePerGas) {
    this.gas = gas;
    this.value = value;
    this.maxPriorityFeePerGas = maxPriorityFeePerGas;
    this.maxFeePerGas = maxFeePerGas;
  }

  /**
   * The gas limit for the transaction.
   *
   * @return the gas limit, or {@code null} to let the node estimate
   */
  @JsonProperty("gas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 gas() {
    return gas;
  }

  /**
   * The native value to transfer with the transaction.
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
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof PublicTxOptions other
        && Objects.equals(gas, other.gas)
        && Objects.equals(value, other.value)
        && Objects.equals(maxPriorityFeePerGas, other.maxPriorityFeePerGas)
        && Objects.equals(maxFeePerGas, other.maxFeePerGas);
  }

  @Override
  public int hashCode() {
    return Objects.hash(gas, value, maxPriorityFeePerGas, maxFeePerGas);
  }

  @Override
  public String toString() {
    return "PublicTxOptions{gas=" + gas + ", value=" + value + "}";
  }

  /** Fluent builder for {@link PublicTxOptions}. */
  public static final class Builder {
    private HexUint64 gas;
    private HexUint256 value;
    private HexUint256 maxPriorityFeePerGas;
    private HexUint256 maxFeePerGas;

    private Builder() {}

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
     * Builds the immutable {@link PublicTxOptions}.
     *
     * @return a new {@link PublicTxOptions} with the configured values
     */
    public PublicTxOptions build() {
      return new PublicTxOptions(gas, value, maxPriorityFeePerGas, maxFeePerGas);
    }
  }
}
