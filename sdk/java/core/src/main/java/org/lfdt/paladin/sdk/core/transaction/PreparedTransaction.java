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

/**
 * A transaction that has been assembled and prepared for submission, mirroring {@code
 * pldapi.PreparedTransaction} — the embedded {@code PreparedTransactionBase} ({@code id}, {@code
 * domain}, {@code to}, the assembled {@link TransactionInput}, and optional metadata) plus the
 * states it will produce. Immutable.
 *
 * <p>The {@link #metadata()} mirrors Go's {@code pldtypes.RawJSON} and is surfaced as raw JSON.
 */
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

  /**
   * The id of the original transaction that was prepared.
   *
   * @return the transaction id, or {@code null} if unset
   */
  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID id() {
    return id;
  }

  /**
   * The domain that prepared the transaction.
   *
   * @return the domain name, or an empty string when unset
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /**
   * The target contract address the prepared transaction will be submitted to.
   *
   * @return the target contract address, or {@code null} if unset
   */
  @JsonProperty("to")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress to() {
    return to;
  }

  /**
   * The assembled transaction ready for submission.
   *
   * @return the assembled transaction input, or {@code null} if unset
   */
  @JsonProperty("transaction")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionInput transaction() {
    return transaction;
  }

  /**
   * Optional domain-supplied metadata, surfaced as raw JSON.
   *
   * @return the metadata, or {@code null} if unset
   */
  @JsonProperty("metadata")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode metadata() {
    return metadata;
  }

  /**
   * The states the prepared transaction will produce.
   *
   * @return the transaction states, or {@code null} if unset
   */
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
