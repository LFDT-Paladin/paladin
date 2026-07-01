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
import java.util.UUID;
import org.lfdt.paladin.sdk.core.types.EthAddress;

// Immutable; mirrors pldapi.PreparedTransaction (the embedded PreparedTransactionBase — id, domain,
// to, the assembled TransactionInput and optional metadata — plus the states it will produce). The
// metadata mirrors Go pldtypes.RawJSON and is surfaced as raw JSON.
@JsonPropertyOrder({"id", "domain", "to", "transaction", "metadata", "states"})
public final class PreparedTransaction {

  private final UUID id;
  private final String domain;
  private final EthAddress to;
  private final TransactionInput transaction;
  private final JsonNode metadata;
  private final TransactionStates states;

  @JsonCreator
  PreparedTransaction(
      @JsonProperty("id") UUID id,
      @JsonProperty("domain") String domain,
      @JsonProperty("to") EthAddress to,
      @JsonProperty("transaction") TransactionInput transaction,
      @JsonProperty("metadata") JsonNode metadata,
      @JsonProperty("states") TransactionStates states) {
    this.id = id;
    this.domain = domain;
    this.to = to;
    this.transaction = transaction;
    this.metadata = metadata;
    this.states = states;
  }

  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID id() {
    return id;
  }

  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  @JsonProperty("to")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress to() {
    return to;
  }

  @JsonProperty("transaction")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionInput transaction() {
    return transaction;
  }

  @JsonProperty("metadata")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode metadata() {
    return metadata;
  }

  @JsonProperty("states")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionStates states() {
    return states;
  }

  @Override
  public String toString() {
    return "PreparedTransaction{id=" + id + ", domain=" + domain + "}";
  }
}
