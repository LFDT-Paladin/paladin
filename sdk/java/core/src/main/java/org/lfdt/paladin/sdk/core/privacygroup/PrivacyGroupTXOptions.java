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
import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.transaction.PublicTxOptions;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;

/**
 * The submission options applied to the genesis transaction of a new privacy group. Immutable;
 * build one with the {@linkplain #builder() fluent builder}.
 *
 * <p>The {@link #publicTxOptions()} fields are flattened onto the flat JSON wire form alongside the
 * idempotency key.
 */
@JsonPropertyOrder({"idempotencyKey", "gas", "value", "maxPriorityFeePerGas", "maxFeePerGas"})
public final class PrivacyGroupTXOptions {

  private final String idempotencyKey;
  private final PublicTxOptions publicTxOptions;

  @JsonCreator
  PrivacyGroupTXOptions(
      @JsonProperty("idempotencyKey") final String idempotencyKey,
      @JsonProperty("gas") final HexUint64 gas,
      @JsonProperty("value") final HexUint256 value,
      @JsonProperty("maxPriorityFeePerGas") final HexUint256 maxPriorityFeePerGas,
      @JsonProperty("maxFeePerGas") final HexUint256 maxFeePerGas) {
    this(
        idempotencyKey,
        PublicTxOptions.builder()
            .gas(gas)
            .value(value)
            .maxPriorityFeePerGas(maxPriorityFeePerGas)
            .maxFeePerGas(maxFeePerGas)
            .build());
  }

  private PrivacyGroupTXOptions(
      final String idempotencyKey, final PublicTxOptions publicTxOptions) {
    this.idempotencyKey = idempotencyKey;
    this.publicTxOptions =
        publicTxOptions == null ? PublicTxOptions.builder().build() : publicTxOptions;
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
   * The public-transaction options.
   *
   * @return the public-transaction options; all fields are unset when no options were supplied
   */
  @JsonIgnore
  public PublicTxOptions publicTxOptions() {
    return publicTxOptions;
  }

  /**
   * The gas limit for the genesis transaction.
   *
   * @return the gas limit, or {@code null} to let the node estimate
   */
  @JsonProperty("gas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 gas() {
    return publicTxOptions.gas();
  }

  /**
   * The native value to transfer with the genesis transaction.
   *
   * @return the native value, or {@code null} if unset
   */
  @JsonProperty("value")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 value() {
    return publicTxOptions.value();
  }

  /**
   * The EIP-1559 max priority fee per gas.
   *
   * @return the max priority fee per gas, or {@code null} if unset
   */
  @JsonProperty("maxPriorityFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxPriorityFeePerGas() {
    return publicTxOptions.maxPriorityFeePerGas();
  }

  /**
   * The EIP-1559 max fee per gas.
   *
   * @return the max fee per gas, or {@code null} if unset
   */
  @JsonProperty("maxFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxFeePerGas() {
    return publicTxOptions.maxFeePerGas();
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
    return o instanceof PrivacyGroupTXOptions other
        && Objects.equals(idempotencyKey, other.idempotencyKey)
        && Objects.equals(publicTxOptions, other.publicTxOptions);
  }

  @Override
  public int hashCode() {
    return Objects.hash(idempotencyKey, publicTxOptions);
  }

  @Override
  public String toString() {
    return "PrivacyGroupTXOptions{idempotencyKey="
        + idempotencyKey
        + ", publicTxOptions="
        + publicTxOptions
        + "}";
  }

  /** Fluent builder for {@link PrivacyGroupTXOptions}. */
  public static final class Builder {
    private String idempotencyKey;
    private PublicTxOptions publicTxOptions;

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
     * Sets the public-transaction options for the genesis transaction.
     *
     * @param publicTxOptions the public-transaction options
     * @return this builder
     */
    public Builder publicTxOptions(final PublicTxOptions publicTxOptions) {
      this.publicTxOptions = publicTxOptions;
      return this;
    }

    /**
     * Builds the immutable {@link PrivacyGroupTXOptions}.
     *
     * @return a new {@link PrivacyGroupTXOptions} with the configured values
     */
    public PrivacyGroupTXOptions build() {
      return new PrivacyGroupTXOptions(idempotencyKey, publicTxOptions);
    }
  }
}
