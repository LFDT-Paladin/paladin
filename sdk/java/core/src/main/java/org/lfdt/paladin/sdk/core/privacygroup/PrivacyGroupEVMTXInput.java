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
import com.fasterxml.jackson.databind.JsonNode;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.transaction.PublicTxOptions;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;

/**
 * An Ethereum-style transaction executed inside a privacy group. Immutable; build one with the
 * {@linkplain #builder(String, HexBytes) fluent builder}.
 *
 * <p>The EVM transaction fields ({@link #from()} through {@link #bytecode()}) are flattened onto
 * the flat JSON wire form, while the base-ledger submission options are carried as a nested {@link
 * #publicTxOptions()} object.
 *
 * <p>{@link #input()} may be hex-encoded calldata or a JSON object/array of argument values; {@link
 * #function()} is required in the latter case. {@link #to()} is {@code null} for a deploy, which
 * also needs {@link #bytecode()}.
 */
@JsonPropertyOrder({
  "idempotencyKey",
  "domain",
  "group",
  "from",
  "to",
  "gas",
  "value",
  "input",
  "function",
  "bytecode",
  "publicTxOptions"
})
public final class PrivacyGroupEVMTXInput {

  private final String idempotencyKey;
  private final String domain;
  private final HexBytes group;
  private final String from;
  private final EthAddress to;
  private final HexUint64 gas;
  private final HexUint256 value;
  private final JsonNode input;
  private final AbiEntry function;
  private final HexBytes bytecode;
  private final PublicTxOptions publicTxOptions;

  @JsonCreator
  PrivacyGroupEVMTXInput(
      @JsonProperty("idempotencyKey") final String idempotencyKey,
      @JsonProperty("domain") final String domain,
      @JsonProperty("group") final HexBytes group,
      @JsonProperty("from") final String from,
      @JsonProperty("to") final EthAddress to,
      @JsonProperty("gas") final HexUint64 gas,
      @JsonProperty("value") final HexUint256 value,
      @JsonProperty("input") final JsonNode input,
      @JsonProperty("function") final AbiEntry function,
      @JsonProperty("bytecode") final HexBytes bytecode,
      @JsonProperty("publicTxOptions") final PublicTxOptions publicTxOptions) {
    this.idempotencyKey = idempotencyKey;
    this.domain = domain;
    this.group = group;
    this.from = from;
    this.to = to;
    this.gas = gas;
    this.value = value;
    this.input = input;
    this.function = function;
    this.bytecode = bytecode;
    this.publicTxOptions = publicTxOptions;
  }

  /**
   * Externally supplied unique identifier; a re-submit with the same key yields 409 Conflict.
   *
   * @return the idempotency key, or an empty string when unset
   */
  @JsonProperty("idempotencyKey")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String idempotencyKey() {
    return idempotencyKey;
  }

  /**
   * The domain the target group belongs to.
   *
   * @return the domain name, or an empty string when unset
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /**
   * The identifier of the group to execute the transaction in.
   *
   * @return the group id, or {@code null} if unset
   */
  @JsonProperty("group")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes group() {
    return group;
  }

  /**
   * The local signing identity used to submit the transaction.
   *
   * @return the signing identity locator, or an empty string when unset
   */
  @JsonProperty("from")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String from() {
    return from;
  }

  /**
   * The target contract address inside the group.
   *
   * @return the target address, or {@code null} for a deploy
   */
  @JsonProperty("to")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress to() {
    return to;
  }

  /**
   * The gas limit for the transaction inside the group.
   *
   * @return the gas limit, or {@code null} to let the node estimate
   */
  @JsonProperty("gas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 gas() {
    return gas;
  }

  /**
   * The native value to transfer within the group.
   *
   * @return the native value, or {@code null} if unset
   */
  @JsonProperty("value")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 value() {
    return value;
  }

  /**
   * The call inputs — hex-encoded calldata, or a JSON object/array of argument values.
   *
   * @return the call inputs, or {@code null} if unset
   */
  @JsonProperty("input")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode input() {
    return input;
  }

  /**
   * The ABI entry for the function being invoked; required when {@link #input()} is a JSON
   * object/array rather than pre-encoded calldata.
   *
   * @return the function ABI entry, or {@code null} if unset
   */
  @JsonProperty("function")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public AbiEntry function() {
    return function;
  }

  /**
   * Deploy bytecode, prepended to the encoded inputs.
   *
   * @return the deploy bytecode, or {@code null} for an invoke
   */
  @JsonProperty("bytecode")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes bytecode() {
    return bytecode;
  }

  /**
   * The base-ledger submission options for the transaction that carries this group transaction.
   *
   * @return the public transaction options, or {@code null} to use the node's defaults
   */
  @JsonProperty("publicTxOptions")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public PublicTxOptions publicTxOptions() {
    return publicTxOptions;
  }

  /**
   * Starts a builder for a transaction in the given group.
   *
   * @param domain the domain the group belongs to
   * @param group the identifier of the group to execute in
   * @return a new builder
   */
  public static Builder builder(final String domain, final HexBytes group) {
    return new Builder(domain, group);
  }

  @Override
  public String toString() {
    return "PrivacyGroupEVMTXInput{domain="
        + domain
        + ", group="
        + group
        + ", from="
        + from
        + ", to="
        + to
        + "}";
  }

  /** Fluent builder for {@link PrivacyGroupEVMTXInput}. */
  public static final class Builder {
    private final String domain;
    private final HexBytes group;
    private String idempotencyKey;
    private String from;
    private EthAddress to;
    private HexUint64 gas;
    private HexUint256 value;
    private JsonNode input;
    private AbiEntry function;
    private HexBytes bytecode;
    private PublicTxOptions publicTxOptions;

    private Builder(final String domain, final HexBytes group) {
      this.domain = domain;
      this.group = group;
    }

    /**
     * Sets the idempotency key.
     *
     * @param idempotencyKey the externally supplied unique identifier
     * @return this builder
     */
    public Builder idempotencyKey(final String idempotencyKey) {
      this.idempotencyKey = idempotencyKey;
      return this;
    }

    /**
     * Sets the signing identity locator.
     *
     * @param from the local signing identity used to submit the transaction
     * @return this builder
     */
    public Builder from(final String from) {
      this.from = from;
      return this;
    }

    /**
     * Sets the target contract address inside the group.
     *
     * @param to the target address, or {@code null} for a deploy
     * @return this builder
     */
    public Builder to(final EthAddress to) {
      this.to = to;
      return this;
    }

    /**
     * Sets the gas limit for the transaction inside the group.
     *
     * @param gas the gas limit, or {@code null} to let the node estimate
     * @return this builder
     */
    public Builder gas(final HexUint64 gas) {
      this.gas = gas;
      return this;
    }

    /**
     * Sets the native value to transfer within the group.
     *
     * @param value the native value to transfer
     * @return this builder
     */
    public Builder value(final HexUint256 value) {
      this.value = value;
      return this;
    }

    /**
     * Sets the call inputs.
     *
     * @param input hex-encoded calldata, or a JSON object/array of argument values
     * @return this builder
     */
    public Builder input(final JsonNode input) {
      this.input = input;
      return this;
    }

    /**
     * Sets the ABI entry for the function being invoked.
     *
     * @param function the function ABI entry; required for JSON object/array inputs
     * @return this builder
     */
    public Builder function(final AbiEntry function) {
      this.function = function;
      return this;
    }

    /**
     * Sets the deploy bytecode.
     *
     * @param bytecode the deploy bytecode, or {@code null} for an invoke
     * @return this builder
     */
    public Builder bytecode(final HexBytes bytecode) {
      this.bytecode = bytecode;
      return this;
    }

    /**
     * Sets the base-ledger submission options.
     *
     * @param publicTxOptions the public transaction options
     * @return this builder
     */
    public Builder publicTxOptions(final PublicTxOptions publicTxOptions) {
      this.publicTxOptions = publicTxOptions;
      return this;
    }

    /**
     * Builds the immutable {@link PrivacyGroupEVMTXInput}.
     *
     * @return a new {@link PrivacyGroupEVMTXInput} with the configured values
     */
    public PrivacyGroupEVMTXInput build() {
      return new PrivacyGroupEVMTXInput(
          idempotencyKey,
          domain,
          group,
          from,
          to,
          gas,
          value,
          input,
          function,
          bytecode,
          publicTxOptions);
    }
  }
}
