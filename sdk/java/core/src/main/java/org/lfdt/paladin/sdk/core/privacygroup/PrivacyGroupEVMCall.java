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

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonUnwrapped;
import com.fasterxml.jackson.databind.annotation.JsonDeserialize;
import com.fasterxml.jackson.databind.annotation.JsonPOJOBuilder;
import java.util.Objects;

/**
 * A read-only Ethereum-style call executed inside a privacy group. Immutable; build one with the
 * {@linkplain #builder(PrivacyGroupEVMTXInput) fluent builder}.
 *
 * <p>The {@link #input()} fields are unwrapped onto the flat JSON wire form, alongside the call
 * options {@link #block()} and {@link #dataFormat()}. Submission-only fields ({@code
 * idempotencyKey} and {@code publicTxOptions}) are omitted from calls. Supports deserialization of
 * the flat wire form through its builder.
 */
@JsonDeserialize(builder = PrivacyGroupEVMCall.Builder.class)
public final class PrivacyGroupEVMCall {

  private final PrivacyGroupEVMTXInput input;
  private final String block;
  private final String dataFormat;

  private PrivacyGroupEVMCall(
      final PrivacyGroupEVMTXInput input, final String block, final String dataFormat) {
    this.input = Objects.requireNonNull(input, "input");
    this.block = block;
    this.dataFormat = dataFormat;
  }

  /**
   * The privacy-group EVM input, unwrapped onto the flat JSON wire form.
   *
   * @return the call input
   */
  @JsonUnwrapped
  @JsonIgnoreProperties({"idempotencyKey", "publicTxOptions"})
  public PrivacyGroupEVMTXInput input() {
    return input;
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
   * Starts a builder for the given privacy-group EVM input.
   *
   * @param input the EVM input to call
   * @return a new builder
   */
  public static Builder builder(final PrivacyGroupEVMTXInput input) {
    return new Builder(input);
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof PrivacyGroupEVMCall other
        && Objects.equals(input, other.input)
        && Objects.equals(block, other.block)
        && Objects.equals(dataFormat, other.dataFormat);
  }

  @Override
  public int hashCode() {
    return Objects.hash(input, block, dataFormat);
  }

  @Override
  public String toString() {
    return "PrivacyGroupEVMCall{input="
        + input
        + ", block="
        + block
        + ", dataFormat="
        + dataFormat
        + "}";
  }

  /** Fluent builder for {@link PrivacyGroupEVMCall}. */
  @JsonPOJOBuilder(withPrefix = "")
  public static final class Builder {
    @JsonUnwrapped private PrivacyGroupEVMTXInput transaction;
    private String block;
    private String dataFormat;

    private Builder() {}

    private Builder(final PrivacyGroupEVMTXInput input) {
      this.transaction = input;
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
      return new PrivacyGroupEVMCall(transaction, block, dataFormat);
    }
  }
}
