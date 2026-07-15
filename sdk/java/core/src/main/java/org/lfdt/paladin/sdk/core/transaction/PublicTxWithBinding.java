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
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A public transaction together with its binding back to the owning Paladin transaction, mirroring
 * {@code pldapi.PublicTxWithBinding} — an embedded {@code PublicTx} (including its inlined {@code
 * PublicTxOptions}/{@code PublicTxGasPricing} gas fields) plus the {@code PublicTxBinding}.
 * Immutable.
 *
 * <p>Both embedded structs are flattened onto the flat JSON wire form. The {@link #submissions()}
 * and {@link #activity()} lists mirror Go's {@code []*PublicTxSubmissionData} / {@code
 * []TransactionActivityRecord} (not yet ported) and are surfaced as raw JSON.
 */
@JsonPropertyOrder({
  "localId",
  "to",
  "data",
  "from",
  "nonce",
  "created",
  "dispatcher",
  "completedAt",
  "transactionHash",
  "success",
  "revertData",
  "submissions",
  "activity",
  "gas",
  "value",
  "maxPriorityFeePerGas",
  "maxFeePerGas",
  "transaction",
  "transactionType",
  "sender",
  "contractAddress"
})
public final class PublicTxWithBinding {

  private final Long localId;
  private final EthAddress to;
  private final HexBytes data;
  private final EthAddress from;
  private final HexUint64 nonce;
  private final Timestamp created;
  private final String dispatcher;
  private final Timestamp completedAt;
  private final Bytes32 transactionHash;
  private final Boolean success;
  private final HexBytes revertData;
  private final List<JsonNode> submissions;
  private final List<JsonNode> activity;
  private final HexUint64 gas;
  private final HexUint256 value;
  private final HexUint256 maxPriorityFeePerGas;
  private final HexUint256 maxFeePerGas;
  private final UUID transaction;
  private final TransactionType transactionType;
  private final String sender;
  private final String contractAddress;

  @JsonCreator
  PublicTxWithBinding(
      @JsonProperty("localId") final Long localId,
      @JsonProperty("to") final EthAddress to,
      @JsonProperty("data") final HexBytes data,
      @JsonProperty("from") final EthAddress from,
      @JsonProperty("nonce") final HexUint64 nonce,
      @JsonProperty("created") final Timestamp created,
      @JsonProperty("dispatcher") final String dispatcher,
      @JsonProperty("completedAt") final Timestamp completedAt,
      @JsonProperty("transactionHash") final Bytes32 transactionHash,
      @JsonProperty("success") final Boolean success,
      @JsonProperty("revertData") final HexBytes revertData,
      @JsonProperty("submissions") final List<JsonNode> submissions,
      @JsonProperty("activity") final List<JsonNode> activity,
      @JsonProperty("gas") final HexUint64 gas,
      @JsonProperty("value") final HexUint256 value,
      @JsonProperty("maxPriorityFeePerGas") final HexUint256 maxPriorityFeePerGas,
      @JsonProperty("maxFeePerGas") final HexUint256 maxFeePerGas,
      @JsonProperty("transaction") final UUID transaction,
      @JsonProperty("transactionType") final TransactionType transactionType,
      @JsonProperty("sender") final String sender,
      @JsonProperty("contractAddress") final String contractAddress) {
    this.localId = localId;
    this.to = to;
    this.data = data;
    this.from = from;
    this.nonce = nonce;
    // A zero timestamp is "unset" (Go omitempty); normalize to null to keep round-trips clean.
    this.created = (created == null || created.isZero()) ? null : created;
    this.dispatcher = dispatcher;
    this.completedAt = (completedAt == null || completedAt.isZero()) ? null : completedAt;
    this.transactionHash = transactionHash;
    this.success = success;
    this.revertData = revertData;
    this.submissions = submissions == null ? Collections.emptyList() : List.copyOf(submissions);
    this.activity = activity == null ? Collections.emptyList() : List.copyOf(activity);
    this.gas = gas;
    this.value = value;
    this.maxPriorityFeePerGas = maxPriorityFeePerGas;
    this.maxFeePerGas = maxFeePerGas;
    this.transaction = transaction;
    this.transactionType = transactionType;
    this.sender = sender;
    this.contractAddress = contractAddress;
  }

  /**
   * The node-local id of the public transaction.
   *
   * @return the local id, or {@code null} if unset
   */
  @JsonProperty("localId")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long localId() {
    return localId;
  }

  /**
   * The target contract address, or {@code null} for a deploy.
   *
   * @return the target address, or {@code null} if unset
   */
  @JsonProperty("to")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress to() {
    return to;
  }

  /**
   * The pre-encoded calldata submitted on-chain.
   *
   * @return the calldata, or {@code null} if unset
   */
  @JsonProperty("data")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes data() {
    return data;
  }

  /**
   * The signing address the transaction was submitted from.
   *
   * @return the sender address, or {@code null} if unset
   */
  @JsonProperty("from")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress from() {
    return from;
  }

  /**
   * The nonce the transaction was submitted with.
   *
   * @return the nonce, or {@code null} if unset
   */
  @JsonProperty("nonce")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 nonce() {
    return nonce;
  }

  /**
   * The time the public transaction was created.
   *
   * @return the created timestamp, or {@code null} if unset
   */
  @JsonProperty("created")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp created() {
    return created;
  }

  /**
   * The identity of the dispatcher that submitted the transaction.
   *
   * @return the dispatcher, or an empty string when unset
   */
  @JsonProperty("dispatcher")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String dispatcher() {
    return dispatcher;
  }

  /**
   * The time the transaction completed on-chain.
   *
   * @return the completed timestamp, or {@code null} if not yet complete
   */
  @JsonProperty("completedAt")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp completedAt() {
    return completedAt;
  }

  /**
   * The hash of the mined transaction.
   *
   * @return the transaction hash, or {@code null} if not yet mined
   */
  @JsonProperty("transactionHash")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Bytes32 transactionHash() {
    return transactionHash;
  }

  /**
   * Whether the transaction succeeded on-chain.
   *
   * @return the success flag, or {@code null} if not yet complete
   */
  @JsonProperty("success")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Boolean success() {
    return success;
  }

  /**
   * The revert data returned when the transaction failed.
   *
   * @return the revert data, or {@code null} if none
   */
  @JsonProperty("revertData")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes revertData() {
    return revertData;
  }

  /**
   * The on-chain submissions made for this transaction, surfaced as raw JSON.
   *
   * @return the submissions, never {@code null} (empty when unset)
   */
  @JsonProperty("submissions")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> submissions() {
    return submissions;
  }

  /**
   * The activity records for this transaction, surfaced as raw JSON.
   *
   * @return the activity records, never {@code null} (empty when unset)
   */
  @JsonProperty("activity")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> activity() {
    return activity;
  }

  /**
   * The gas limit, or {@code null} if the node estimated it.
   *
   * @return the gas limit, or {@code null} if unset
   */
  @JsonProperty("gas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 gas() {
    return gas;
  }

  /**
   * The native value transferred with the transaction, or {@code null} for none.
   *
   * @return the value, or {@code null} if unset
   */
  @JsonProperty("value")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 value() {
    return value;
  }

  /**
   * The EIP-1559 max priority fee per gas.
   *
   * @return the max priority fee per gas, or {@code null} if unset
   */
  @JsonProperty("maxPriorityFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxPriorityFeePerGas() {
    return maxPriorityFeePerGas;
  }

  /**
   * The EIP-1559 max fee per gas.
   *
   * @return the max fee per gas, or {@code null} if unset
   */
  @JsonProperty("maxFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxFeePerGas() {
    return maxFeePerGas;
  }

  /**
   * The id of the Paladin transaction this public transaction is bound to.
   *
   * @return the bound transaction id, or {@code null} if unset
   */
  @JsonProperty("transaction")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID transaction() {
    return transaction;
  }

  /**
   * The type of the bound Paladin transaction.
   *
   * @return the transaction type, or {@code null} if unset
   */
  @JsonProperty("transactionType")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionType transactionType() {
    return transactionType;
  }

  /**
   * The sender identity of the bound Paladin transaction.
   *
   * @return the sender, or an empty string when unset
   */
  @JsonProperty("sender")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String sender() {
    return sender;
  }

  /**
   * The contract address deployed by the bound Paladin transaction, if any.
   *
   * @return the contract address, or an empty string when unset
   */
  @JsonProperty("contractAddress")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String contractAddress() {
    return contractAddress;
  }

  @Override
  public String toString() {
    return "PublicTxWithBinding{from="
        + from
        + ", nonce="
        + nonce
        + ", transaction="
        + transaction
        + "}";
  }
}
