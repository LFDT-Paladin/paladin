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
import java.util.UUID;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A {@link TransactionReceipt} enriched with its related states, the domain receipt, and any
 * associated public transactions. Immutable.
 *
 * <p>The base receipt fields are re-declared and flattened here to match the flat JSON wire form,
 * exactly as {@link TransactionFull} does over {@link Transaction}. The {@link #domainReceipt()}
 * and the {@link #publicTransactions()} block are surfaced as raw JSON.
 */
@JsonPropertyOrder({
  "id",
  "indexed",
  "sequence",
  "domain",
  "success",
  "transactionHash",
  "blockNumber",
  "transactionIndex",
  "logIndex",
  "source",
  "failureMessage",
  "revertData",
  "contractAddress",
  "states",
  "domainReceipt",
  "domainReceiptError",
  "public"
})
public final class TransactionReceiptFull extends TransactionReceipt {

  private final TransactionStates states;
  private final JsonNode domainReceipt;
  private final String domainReceiptError;
  private final List<JsonNode> publicTransactions;

  @JsonCreator
  TransactionReceiptFull(
      @JsonProperty("id") final UUID id,
      @JsonProperty("indexed") final Timestamp indexed,
      @JsonProperty("sequence") final long sequence,
      @JsonProperty("domain") final String domain,
      @JsonProperty("success") final boolean success,
      @JsonProperty("transactionHash") final Bytes32 transactionHash,
      @JsonProperty("blockNumber") final Long blockNumber,
      @JsonProperty("transactionIndex") final Long transactionIndex,
      @JsonProperty("logIndex") final Long logIndex,
      @JsonProperty("source") final EthAddress source,
      @JsonProperty("failureMessage") final String failureMessage,
      @JsonProperty("revertData") final HexBytes revertData,
      @JsonProperty("contractAddress") final EthAddress contractAddress,
      @JsonProperty("states") final TransactionStates states,
      @JsonProperty("domainReceipt") final JsonNode domainReceipt,
      @JsonProperty("domainReceiptError") final String domainReceiptError,
      @JsonProperty("public") final List<JsonNode> publicTransactions) {
    super(
        id,
        indexed,
        sequence,
        domain,
        success,
        transactionHash,
        blockNumber,
        transactionIndex,
        logIndex,
        source,
        failureMessage,
        revertData,
        contractAddress);
    this.states = states;
    this.domainReceipt = domainReceipt;
    this.domainReceiptError = domainReceiptError;
    this.publicTransactions =
        publicTransactions == null ? Collections.emptyList() : List.copyOf(publicTransactions);
  }

  /**
   * The states related to the transaction.
   *
   * @return the transaction states, or {@code null} if unset
   */
  @JsonProperty("states")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionStates states() {
    return states;
  }

  /**
   * The domain-specific receipt, surfaced as raw JSON.
   *
   * @return the domain receipt, or {@code null} if unset
   */
  @JsonProperty("domainReceipt")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode domainReceipt() {
    return domainReceipt;
  }

  /**
   * The error encountered while building the domain receipt, if any.
   *
   * @return the domain receipt error, or an empty string when none
   */
  @JsonProperty("domainReceiptError")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domainReceiptError() {
    return domainReceiptError;
  }

  /**
   * The associated public transactions, surfaced as raw JSON.
   *
   * @return the public transactions, never {@code null} (empty when unset)
   */
  @JsonProperty("public")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> publicTransactions() {
    return publicTransactions;
  }

  @Override
  public String toString() {
    return "TransactionReceiptFull{id=" + id() + ", success=" + success() + "}";
  }
}
