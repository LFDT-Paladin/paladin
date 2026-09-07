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
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;

/**
 * A read-only Ethereum-style call executed inside a privacy group. Immutable; build one with the
 * {@linkplain #builder(String, HexBytes) fluent builder}.
 *
 * <p>The EVM transaction fields ({@link #from()} through {@link #bytecode()}) are flattened onto
 * the flat JSON wire form, as are the call options {@link #block()} and {@link #dataFormat()}.
 *
 * <p>{@link #input()} may be hex-encoded calldata or a JSON object/array of argument values; {@link
 * #function()} is required in the latter case.
 */
@JsonPropertyOrder({
  "domain",
  "group",
  "from",
  "to",
  "gas",
  "value",
  "input",
  "function",
  "bytecode",
  "block",
  "dataFormat"
})
public final class PrivacyGroupEVMCall {

  private final String domain;
  private final HexBytes group;
  private final String from;
  private final EthAddress to;
  private final HexUint64 gas;
  private final HexUint256 value;
  private final JsonNode input;
  private final AbiEntry function;
  private final HexBytes bytecode;
  private final String block;
  private final String dataFormat;

  @JsonCreator
  PrivacyGroupEVMCall(
      @JsonProperty("domain") final String domain,
      @JsonProperty("group") final HexBytes group,
      @JsonProperty("from") final String from,
      @JsonProperty("to") final EthAddress to,
      @JsonProperty("gas") final HexUint64 gas,
      @JsonProperty("value") final HexUint256 value,
      @JsonProperty("input") final JsonNode input,
      @JsonProperty("function") final AbiEntry function,
      @JsonProperty("bytecode") final HexBytes bytecode,
      @JsonProperty("block") final String block,
      @JsonProperty("dataFormat") final String dataFormat) {
    this.domain = domain;
    this.group = group;
    this.from = from;
    this.to = to;
    this.gas = gas;
    this.value = value;
    this.input = input;
    this.function = function;
    this.bytecode = bytecode;
    this.block = block;
    this.dataFormat = dataFormat;
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
   * The identifier of the group to execute the call in.
   *
   * @return the group id, or {@code null} if unset
   */
  @JsonProperty("group")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes group() {
    return group;
  }

  /**
   * The identity the call executes as.
   *
   * @return the identity locator, or an empty string when unset
   */
  @JsonProperty("from")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String from() {
    return from;
  }

  /**
   * The target contract address inside the group.
   *
   * @return the target address, or {@code null} if unset
   */
  @JsonProperty("to")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress to() {
    return to;
  }

  /**
   * The gas limit applied while executing the call.
   *
   * @return the gas limit, or {@code null} to let the node decide
   */
  @JsonProperty("gas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 gas() {
    return gas;
  }

  /**
   * The native value the call is executed with.
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
   * The ABI entry for the function being called; required when {@link #input()} is a JSON
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
   * The block to execute the call against — a number or a special string such as {@code "latest"}.
   *
   * @return the block, or an empty string when unset
   */
  @JsonProperty("block")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String block() {
    return block;
  }

  /**
   * The output data format requested for the result.
   *
   * @return the data format, or an empty string when unset
   */
  @JsonProperty("dataFormat")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String dataFormat() {
    return dataFormat;
  }

  /**
   * Starts a builder for a call in the given group.
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
    return "PrivacyGroupEVMCall{domain=" + domain + ", group=" + group + ", to=" + to + "}";
  }

  /** Fluent builder for {@link PrivacyGroupEVMCall}. */
  public static final class Builder {
    private final String domain;
    private final HexBytes group;
    private String from;
    private EthAddress to;
    private HexUint64 gas;
    private HexUint256 value;
    private JsonNode input;
    private AbiEntry function;
    private HexBytes bytecode;
    private String block;
    private String dataFormat;

    private Builder(final String domain, final HexBytes group) {
      this.domain = domain;
      this.group = group;
    }

    /**
     * Sets the identity the call executes as.
     *
     * @param from the identity locator
     * @return this builder
     */
    public Builder from(final String from) {
      this.from = from;
      return this;
    }

    /**
     * Sets the target contract address inside the group.
     *
     * @param to the target address
     * @return this builder
     */
    public Builder to(final EthAddress to) {
      this.to = to;
      return this;
    }

    /**
     * Sets the gas limit applied while executing the call.
     *
     * @param gas the gas limit, or {@code null} to let the node decide
     * @return this builder
     */
    public Builder gas(final HexUint64 gas) {
      this.gas = gas;
      return this;
    }

    /**
     * Sets the native value the call is executed with.
     *
     * @param value the native value
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
     * Sets the ABI entry for the function being called.
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
     * Sets the block to execute the call against.
     *
     * @param block a number or a special string such as {@code "latest"}
     * @return this builder
     */
    public Builder block(final String block) {
      this.block = block;
      return this;
    }

    /**
     * Sets the output data format requested for the result.
     *
     * @param dataFormat the data format
     * @return this builder
     */
    public Builder dataFormat(final String dataFormat) {
      this.dataFormat = dataFormat;
      return this;
    }

    /**
     * Builds the immutable {@link PrivacyGroupEVMCall}.
     *
     * @return a new {@link PrivacyGroupEVMCall} with the configured values
     */
    public PrivacyGroupEVMCall build() {
      return new PrivacyGroupEVMCall(
          domain, group, from, to, gas, value, input, function, bytecode, block, dataFormat);
    }
  }
}
