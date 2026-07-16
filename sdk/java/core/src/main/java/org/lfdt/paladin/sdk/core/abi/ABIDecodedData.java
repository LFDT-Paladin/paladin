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
package org.lfdt.paladin.sdk.core.abi;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import com.fasterxml.jackson.databind.JsonNode;

/**
 * The result of decoding revert, call, or event data against a stored ABI. Immutable. Returned by
 * {@code ptx_decodeError}, {@code ptx_decodeCall}, and {@code ptx_decodeEvent}.
 *
 * <p>The decoded {@link #data()} is surfaced as raw JSON; the matched {@link #definition()} is a
 * typed {@link AbiEntry}.
 */
@JsonPropertyOrder({"data", "summary", "definition", "signature"})
public final class ABIDecodedData {

  private final JsonNode data;
  private final String summary;
  private final AbiEntry definition;
  private final String signature;

  @JsonCreator
  ABIDecodedData(
      @JsonProperty("data") final JsonNode data,
      @JsonProperty("summary") final String summary,
      @JsonProperty("definition") final AbiEntry definition,
      @JsonProperty("signature") final String signature) {
    this.data = data;
    this.summary = summary;
    this.definition = definition;
    this.signature = signature;
  }

  /**
   * The decoded data as raw JSON.
   *
   * @return the decoded data, or {@code null} if unset
   */
  @JsonProperty("data")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode data() {
    return data;
  }

  /**
   * A human-readable summary of the decoded data (for example, a formatted error message).
   *
   * @return the summary, or {@code null} if unset
   */
  @JsonProperty("summary")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String summary() {
    return summary;
  }

  /**
   * The ABI entry that matched and was used to decode the data.
   *
   * @return the matched definition, or {@code null} if unset
   */
  @JsonProperty("definition")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public AbiEntry definition() {
    return definition;
  }

  /**
   * The canonical signature of the matched definition.
   *
   * @return the signature, or {@code null} if unset
   */
  @JsonProperty("signature")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public String signature() {
    return signature;
  }

  @Override
  public String toString() {
    return "ABIDecodedData{signature=" + signature + "}";
  }
}
