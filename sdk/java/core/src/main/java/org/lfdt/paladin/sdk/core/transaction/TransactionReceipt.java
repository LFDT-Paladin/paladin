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
import java.util.Objects;
import java.util.UUID;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * The receipt returned once a transaction reaches a final state, mirroring {@code
 * pldapi.TransactionReceipt} (which embeds {@code TransactionReceiptData} and its inlined {@code
 * TransactionReceiptDataOnchain} / {@code TransactionReceiptDataOnchainEvent} blocks).
 *
 * <p>Immutable and self-serializing. The on-chain fields ({@link #transactionHash()}, {@link
 * #blockNumber()}, {@link #transactionIndex()}, {@link #logIndex()}, {@link #source()}) are
 * populated only when the result was finalized by the blockchain — they are {@code null} otherwise.
 * The boxed {@code Long} on-chain indices distinguish "not finalized on-chain" (null, omitted) from
 * a genuine zero. Round-trips through any {@code ObjectMapper}.
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
  "contractAddress"
})
public final class TransactionReceipt {

  private final UUID id;
  private final Timestamp indexed;
  private final long sequence;
  private final String domain;
  private final boolean success;
  private final Bytes32 transactionHash;
  private final Long blockNumber;
  private final Long transactionIndex;
  private final Long logIndex;
  private final EthAddress source;
  private final String failureMessage;
  private final HexBytes revertData;
  private final EthAddress contractAddress;

  @JsonCreator
  TransactionReceipt(
      @JsonProperty("id") UUID id,
      @JsonProperty("indexed") Timestamp indexed,
      @JsonProperty("sequence") long sequence,
      @JsonProperty("domain") String domain,
      @JsonProperty("success") boolean success,
      @JsonProperty("transactionHash") Bytes32 transactionHash,
      @JsonProperty("blockNumber") Long blockNumber,
      @JsonProperty("transactionIndex") Long transactionIndex,
      @JsonProperty("logIndex") Long logIndex,
      @JsonProperty("source") EthAddress source,
      @JsonProperty("failureMessage") String failureMessage,
      @JsonProperty("revertData") HexBytes revertData,
      @JsonProperty("contractAddress") EthAddress contractAddress) {
    this.id = id;
    // A zero timestamp is "unset" (Go omitempty); the Timestamp deserializer yields a zero rather
    // than null for an absent field, so normalize here to keep round-trips and equality clean.
    this.indexed = (indexed == null || indexed.isZero()) ? null : indexed;
    this.sequence = sequence;
    this.domain = domain;
    this.success = success;
    this.transactionHash = transactionHash;
    this.blockNumber = blockNumber;
    this.transactionIndex = transactionIndex;
    this.logIndex = logIndex;
    this.source = source;
    this.failureMessage = failureMessage;
    this.revertData = revertData;
    this.contractAddress = contractAddress;
  }

  /** The transaction ID this receipt belongs to. */
  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID id() {
    return id;
  }

  /** The time this receipt was indexed, or {@code null} if unset. */
  @JsonProperty("indexed")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp indexed() {
    return indexed;
  }

  /** Local ordering sequence, used by receipt listeners. Always present. */
  @JsonProperty("sequence")
  public long sequence() {
    return sequence;
  }

  /** Domain name; set only on private-transaction receipts. */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /** Whether the transaction succeeded. */
  @JsonProperty("success")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean success() {
    return success;
  }

  /** Base-ledger transaction hash; set only once finalized on-chain. */
  @JsonProperty("transactionHash")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Bytes32 transactionHash() {
    return transactionHash;
  }

  /** Block number; set only once finalized on-chain. */
  @JsonProperty("blockNumber")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long blockNumber() {
    return blockNumber;
  }

  /** Index of the transaction within its block; set only once finalized on-chain. */
  @JsonProperty("transactionIndex")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long transactionIndex() {
    return transactionIndex;
  }

  /** Log index of the finalizing event; set only when finalized by a blockchain event. */
  @JsonProperty("logIndex")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long logIndex() {
    return logIndex;
  }

  /** Emitting contract of the finalizing event; set only when finalized by a blockchain event. */
  @JsonProperty("source")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress source() {
    return source;
  }

  /** Detail of why the transaction reverted; non-empty only on failure. */
  @JsonProperty("failureMessage")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String failureMessage() {
    return failureMessage;
  }

  /** Encoded revert data, if available. */
  @JsonProperty("revertData")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes revertData() {
    return revertData;
  }

  /** Address of a newly deployed contract; {@code null} when this transaction was an invoke. */
  @JsonProperty("contractAddress")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress contractAddress() {
    return contractAddress;
  }

  /** Starts an empty builder. */
  public static Builder builder() {
    return new Builder();
  }

  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof TransactionReceipt other
        && sequence == other.sequence
        && success == other.success
        && Objects.equals(id, other.id)
        && Objects.equals(indexed, other.indexed)
        && Objects.equals(domain, other.domain)
        && Objects.equals(transactionHash, other.transactionHash)
        && Objects.equals(blockNumber, other.blockNumber)
        && Objects.equals(transactionIndex, other.transactionIndex)
        && Objects.equals(logIndex, other.logIndex)
        && Objects.equals(source, other.source)
        && Objects.equals(failureMessage, other.failureMessage)
        && Objects.equals(revertData, other.revertData)
        && Objects.equals(contractAddress, other.contractAddress);
  }

  @Override
  public int hashCode() {
    return Objects.hash(
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
  }

  @Override
  public String toString() {
    return "TransactionReceipt{id="
        + id
        + ", success="
        + success
        + (failureMessage == null ? "" : ", failureMessage=" + failureMessage)
        + "}";
  }

  /** Fluent builder for {@link TransactionReceipt}. */
  public static final class Builder {
    private UUID id;
    private Timestamp indexed;
    private long sequence;
    private String domain;
    private boolean success;
    private Bytes32 transactionHash;
    private Long blockNumber;
    private Long transactionIndex;
    private Long logIndex;
    private EthAddress source;
    private String failureMessage;
    private HexBytes revertData;
    private EthAddress contractAddress;

    private Builder() {}

    public Builder id(UUID id) {
      this.id = id;
      return this;
    }

    public Builder indexed(Timestamp indexed) {
      this.indexed = indexed;
      return this;
    }

    public Builder sequence(long sequence) {
      this.sequence = sequence;
      return this;
    }

    public Builder domain(String domain) {
      this.domain = domain;
      return this;
    }

    public Builder success(boolean success) {
      this.success = success;
      return this;
    }

    public Builder transactionHash(Bytes32 transactionHash) {
      this.transactionHash = transactionHash;
      return this;
    }

    public Builder blockNumber(Long blockNumber) {
      this.blockNumber = blockNumber;
      return this;
    }

    public Builder transactionIndex(Long transactionIndex) {
      this.transactionIndex = transactionIndex;
      return this;
    }

    public Builder logIndex(Long logIndex) {
      this.logIndex = logIndex;
      return this;
    }

    public Builder source(EthAddress source) {
      this.source = source;
      return this;
    }

    public Builder failureMessage(String failureMessage) {
      this.failureMessage = failureMessage;
      return this;
    }

    public Builder revertData(HexBytes revertData) {
      this.revertData = revertData;
      return this;
    }

    public Builder contractAddress(EthAddress contractAddress) {
      this.contractAddress = contractAddress;
      return this;
    }

    public TransactionReceipt build() {
      return new TransactionReceipt(
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
    }
  }
}
