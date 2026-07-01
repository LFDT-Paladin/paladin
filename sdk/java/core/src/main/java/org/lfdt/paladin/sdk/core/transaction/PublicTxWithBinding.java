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

// Immutable; mirrors pldapi.PublicTxWithBinding (an embedded PublicTx — including its inlined
// PublicTxOptions/PublicTxGasPricing gas fields — plus the PublicTxBinding back to the paladin
// transaction). Both embedded structs are flattened onto the flat JSON wire form. The submissions
// and activity lists mirror Go []*PublicTxSubmissionData / []TransactionActivityRecord (not yet
// ported) and are surfaced as raw JSON.
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
      @JsonProperty("localId") Long localId,
      @JsonProperty("to") EthAddress to,
      @JsonProperty("data") HexBytes data,
      @JsonProperty("from") EthAddress from,
      @JsonProperty("nonce") HexUint64 nonce,
      @JsonProperty("created") Timestamp created,
      @JsonProperty("dispatcher") String dispatcher,
      @JsonProperty("completedAt") Timestamp completedAt,
      @JsonProperty("transactionHash") Bytes32 transactionHash,
      @JsonProperty("success") Boolean success,
      @JsonProperty("revertData") HexBytes revertData,
      @JsonProperty("submissions") List<JsonNode> submissions,
      @JsonProperty("activity") List<JsonNode> activity,
      @JsonProperty("gas") HexUint64 gas,
      @JsonProperty("value") HexUint256 value,
      @JsonProperty("maxPriorityFeePerGas") HexUint256 maxPriorityFeePerGas,
      @JsonProperty("maxFeePerGas") HexUint256 maxFeePerGas,
      @JsonProperty("transaction") UUID transaction,
      @JsonProperty("transactionType") TransactionType transactionType,
      @JsonProperty("sender") String sender,
      @JsonProperty("contractAddress") String contractAddress) {
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

  @JsonProperty("localId")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long localId() {
    return localId;
  }

  @JsonProperty("to")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress to() {
    return to;
  }

  @JsonProperty("data")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes data() {
    return data;
  }

  @JsonProperty("from")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress from() {
    return from;
  }

  @JsonProperty("nonce")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 nonce() {
    return nonce;
  }

  @JsonProperty("created")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp created() {
    return created;
  }

  @JsonProperty("dispatcher")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String dispatcher() {
    return dispatcher;
  }

  @JsonProperty("completedAt")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp completedAt() {
    return completedAt;
  }

  @JsonProperty("transactionHash")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Bytes32 transactionHash() {
    return transactionHash;
  }

  @JsonProperty("success")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Boolean success() {
    return success;
  }

  @JsonProperty("revertData")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes revertData() {
    return revertData;
  }

  @JsonProperty("submissions")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> submissions() {
    return submissions;
  }

  @JsonProperty("activity")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> activity() {
    return activity;
  }

  @JsonProperty("gas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 gas() {
    return gas;
  }

  @JsonProperty("value")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 value() {
    return value;
  }

  @JsonProperty("maxPriorityFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxPriorityFeePerGas() {
    return maxPriorityFeePerGas;
  }

  @JsonProperty("maxFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxFeePerGas() {
    return maxFeePerGas;
  }

  @JsonProperty("transaction")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID transaction() {
    return transaction;
  }

  @JsonProperty("transactionType")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionType transactionType() {
    return transactionType;
  }

  @JsonProperty("sender")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String sender() {
    return sender;
  }

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
