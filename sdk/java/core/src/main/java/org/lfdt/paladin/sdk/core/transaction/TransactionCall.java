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

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonUnwrapped;
import java.util.Objects;

/**
 * The input to {@code ptx_call}, mirroring {@code pldapi.TransactionCall} — an embedded {@link
 * TransactionInput} plus the {@code PublicCallOptions} block and the {@code JSONFormatOptions} data
 * format. Immutable; build one with the {@linkplain #builder(TransactionInput) fluent builder}.
 *
 * <p>The input fields are unwrapped onto the flat JSON wire form. The {@link #block()} is a number
 * or a special string such as {@code "latest"}.
 */
public final class TransactionCall {

  private final TransactionInput input;
  private final String block;
  private final String dataFormat;

  private TransactionCall(TransactionInput input, String block, String dataFormat) {
    this.input = Objects.requireNonNull(input, "input");
    this.block = block;
    this.dataFormat = dataFormat;
  }

  /**
   * The transaction to call, unwrapped onto the flat JSON wire form.
   *
   * @return the call input
   */
  @JsonUnwrapped
  public TransactionInput input() {
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
   * Starts a builder for the given call input.
   *
   * @param input the transaction to call
   * @return a new builder
   */
  public static Builder builder(TransactionInput input) {
    return new Builder(input);
  }

  @Override
  public String toString() {
    return "TransactionCall{input="
        + input
        + ", block="
        + block
        + ", dataFormat="
        + dataFormat
        + "}";
  }

  /** Fluent builder for {@link TransactionCall}. */
  public static final class Builder {
    private final TransactionInput input;
    private String block;
    private String dataFormat;

    private Builder(TransactionInput input) {
      this.input = input;
    }

    /**
     * Sets the block to execute the call against.
     *
     * @param block a number or a special string such as {@code "latest"}
     * @return this builder
     */
    public Builder block(String block) {
      this.block = block;
      return this;
    }

    /**
     * Sets the output data format requested for the result.
     *
     * @param dataFormat the data format
     * @return this builder
     */
    public Builder dataFormat(String dataFormat) {
      this.dataFormat = dataFormat;
      return this;
    }

    /**
     * Builds the immutable {@link TransactionCall}.
     *
     * @return a new {@link TransactionCall} with the configured values
     */
    public TransactionCall build() {
      return new TransactionCall(input, block, dataFormat);
    }
  }
}
