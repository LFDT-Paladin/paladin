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
import com.fasterxml.jackson.databind.JsonNode;
import java.util.Collections;
import java.util.List;

// Immutable; mirrors pldapi.TransactionStates (the spent/read/confirmed/info state buckets a
// transaction touched, plus an unavailable block listing states whose data we don't hold). The
// individual states mirror Go []*pldapi.StateBase and the unavailable block mirrors
// *pldapi.UnavailableStates; neither is ported yet, so both are surfaced as raw JSON to keep
// round-trips exact.
@JsonPropertyOrder({"none", "spent", "read", "confirmed", "info", "unavailable"})
public final class TransactionStates {

  private final boolean none;
  private final List<JsonNode> spent;
  private final List<JsonNode> read;
  private final List<JsonNode> confirmed;
  private final List<JsonNode> info;
  private final JsonNode unavailable;

  @JsonCreator
  TransactionStates(
      @JsonProperty("none") boolean none,
      @JsonProperty("spent") List<JsonNode> spent,
      @JsonProperty("read") List<JsonNode> read,
      @JsonProperty("confirmed") List<JsonNode> confirmed,
      @JsonProperty("info") List<JsonNode> info,
      @JsonProperty("unavailable") JsonNode unavailable) {
    this.none = none;
    this.spent = spent == null ? Collections.emptyList() : List.copyOf(spent);
    this.read = read == null ? Collections.emptyList() : List.copyOf(read);
    this.confirmed = confirmed == null ? Collections.emptyList() : List.copyOf(confirmed);
    this.info = info == null ? Collections.emptyList() : List.copyOf(info);
    this.unavailable = unavailable;
  }

  @JsonProperty("none")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean none() {
    return none;
  }

  @JsonProperty("spent")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> spent() {
    return spent;
  }

  @JsonProperty("read")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> read() {
    return read;
  }

  @JsonProperty("confirmed")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> confirmed() {
    return confirmed;
  }

  @JsonProperty("info")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> info() {
    return info;
  }

  @JsonProperty("unavailable")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode unavailable() {
    return unavailable;
  }

  @Override
  public String toString() {
    return "TransactionStates{none="
        + none
        + ", spent="
        + spent.size()
        + ", read="
        + read.size()
        + ", confirmed="
        + confirmed.size()
        + ", info="
        + info.size()
        + "}";
  }
}
