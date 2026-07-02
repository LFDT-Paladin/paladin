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

// Immutable; mirrors pldapi.ABIDecodedData (the result of decoding revert/call/event data against a
// stored ABI). The decoded data mirrors Go pldtypes.RawJSON and is surfaced as raw JSON; the matched
// definition is a typed AbiEntry. Returned by ptx_decodeError / ptx_decodeCall / ptx_decodeEvent.
@JsonPropertyOrder({"data", "summary", "definition", "signature"})
public final class ABIDecodedData {

  private final JsonNode data;
  private final String summary;
  private final AbiEntry definition;
  private final String signature;

  @JsonCreator
  ABIDecodedData(
      @JsonProperty("data") JsonNode data,
      @JsonProperty("summary") String summary,
      @JsonProperty("definition") AbiEntry definition,
      @JsonProperty("signature") String signature) {
    this.data = data;
    this.summary = summary;
    this.definition = definition;
    this.signature = signature;
  }

  @JsonProperty("data")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode data() {
    return data;
  }

  @JsonProperty("summary")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String summary() {
    return summary;
  }

  @JsonProperty("definition")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public AbiEntry definition() {
    return definition;
  }

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
