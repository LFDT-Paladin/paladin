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
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A {@link Transaction} enriched with dependencies, receipt, and associated
 * public/history/sequencer detail, mirroring {@code pldapi.TransactionFull}. Immutable.
 *
 * <p>The {@link #dependsOn()} list and the {@link #receipt()} are typed; the {@link
 * #publicTransactions()}, {@link #history()}, and {@link #sequencerActivity()} blocks are surfaced
 * as raw JSON for now (they mirror Go's {@code []*PublicTx} / {@code []*TransactionHistory} /
 * {@code []*SequencerActivity}, which are not yet ported).
 */
@JsonPropertyOrder({
  "id",
  "created",
  "submitMode",
  "idempotencyKey",
  "type",
  "domain",
  "function",
  "abiReference",
  "from",
  "to",
  "data",
  "gas",
  "value",
  "maxPriorityFeePerGas",
  "maxFeePerGas",
  "dependsOn",
  "receipt",
  "public",
  "history",
  "sequencerActivity"
})
public final class TransactionFull extends Transaction {

  private final List<UUID> dependsOn;
  private final TransactionReceipt receipt;
  private final List<JsonNode> publicTransactions;
  private final List<JsonNode> history;
  private final List<JsonNode> sequencerActivity;

  @JsonCreator
  TransactionFull(
      @JsonProperty("id") UUID id,
      @JsonProperty("created") Timestamp created,
      @JsonProperty("submitMode") SubmitMode submitMode,
      @JsonProperty("idempotencyKey") String idempotencyKey,
      @JsonProperty("type") TransactionType type,
      @JsonProperty("domain") String domain,
      @JsonProperty("function") String function,
      @JsonProperty("abiReference") Bytes32 abiReference,
      @JsonProperty("from") String from,
      @JsonProperty("to") EthAddress to,
      @JsonProperty("data") JsonNode data,
      @JsonProperty("gas") HexUint64 gas,
      @JsonProperty("value") HexUint256 value,
      @JsonProperty("maxPriorityFeePerGas") HexUint256 maxPriorityFeePerGas,
      @JsonProperty("maxFeePerGas") HexUint256 maxFeePerGas,
      @JsonProperty("dependsOn") List<UUID> dependsOn,
      @JsonProperty("receipt") TransactionReceipt receipt,
      @JsonProperty("public") List<JsonNode> publicTransactions,
      @JsonProperty("history") List<JsonNode> history,
      @JsonProperty("sequencerActivity") List<JsonNode> sequencerActivity) {
    super(
        id,
        created,
        submitMode,
        idempotencyKey,
        type,
        domain,
        function,
        abiReference,
        from,
        to,
        data,
        gas,
        value,
        maxPriorityFeePerGas,
        maxFeePerGas);
    this.dependsOn = dependsOn == null ? Collections.emptyList() : List.copyOf(dependsOn);
    this.receipt = receipt;
    this.publicTransactions =
        publicTransactions == null ? Collections.emptyList() : List.copyOf(publicTransactions);
    this.history = history == null ? Collections.emptyList() : List.copyOf(history);
    this.sequencerActivity =
        sequencerActivity == null ? Collections.emptyList() : List.copyOf(sequencerActivity);
  }

  /**
   * The ids of transactions this one depends on.
   *
   * @return the dependency ids, never {@code null} (empty when unset)
   */
  @JsonProperty("dependsOn")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<UUID> dependsOn() {
    return dependsOn;
  }

  /**
   * The receipt for this transaction, if it has reached a final state.
   *
   * @return the receipt, or {@code null} if not yet available
   */
  @JsonProperty("receipt")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionReceipt receipt() {
    return receipt;
  }

  /**
   * The associated public transactions, surfaced as raw JSON (mirrors Go's {@code []*PublicTx}).
   *
   * @return the public transactions, never {@code null} (empty when unset)
   */
  @JsonProperty("public")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> publicTransactions() {
    return publicTransactions;
  }

  /**
   * The transaction history, surfaced as raw JSON (mirrors Go's {@code []*TransactionHistory}).
   *
   * @return the history entries, never {@code null} (empty when unset)
   */
  @JsonProperty("history")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> history() {
    return history;
  }

  /**
   * The sequencer activity, surfaced as raw JSON (mirrors Go's {@code []*SequencerActivity}).
   *
   * @return the sequencer activity entries, never {@code null} (empty when unset)
   */
  @JsonProperty("sequencerActivity")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> sequencerActivity() {
    return sequencerActivity;
  }

  @Override
  public String toString() {
    return "TransactionFull{id="
        + id()
        + ", type="
        + type()
        + ", dependsOn="
        + dependsOn.size()
        + "}";
  }
}
