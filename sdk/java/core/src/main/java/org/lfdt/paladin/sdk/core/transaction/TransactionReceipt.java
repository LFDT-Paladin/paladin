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

  /**
   * The transaction ID this receipt belongs to.
   *
   * @return the transaction id, or {@code null} if unset
   */
  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID id() {
    return id;
  }

  /**
   * The time this receipt was indexed, or {@code null} if unset.
   *
   * @return the indexed timestamp, or {@code null} if unset
   */
  @JsonProperty("indexed")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp indexed() {
    return indexed;
  }

  /**
   * Local ordering sequence, used by receipt listeners. Always present.
   *
   * @return the local ordering sequence
   */
  @JsonProperty("sequence")
  public long sequence() {
    return sequence;
  }

  /**
   * Domain name; set only on private-transaction receipts.
   *
   * @return the domain name, or an empty string for public-transaction receipts
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /**
   * Whether the transaction succeeded.
   *
   * @return {@code true} if the transaction succeeded
   */
  @JsonProperty("success")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean success() {
    return success;
  }

  /**
   * Base-ledger transaction hash; set only once finalized on-chain.
   *
   * @return the transaction hash, or {@code null} if not yet finalized on-chain
   */
  @JsonProperty("transactionHash")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Bytes32 transactionHash() {
    return transactionHash;
  }

  /**
   * Block number; set only once finalized on-chain.
   *
   * @return the block number, or {@code null} if not yet finalized on-chain
   */
  @JsonProperty("blockNumber")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long blockNumber() {
    return blockNumber;
  }

  /**
   * Index of the transaction within its block; set only once finalized on-chain.
   *
   * @return the transaction index, or {@code null} if not yet finalized on-chain
   */
  @JsonProperty("transactionIndex")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long transactionIndex() {
    return transactionIndex;
  }

  /**
   * Log index of the finalizing event; set only when finalized by a blockchain event.
   *
   * @return the log index, or {@code null} if not finalized by a blockchain event
   */
  @JsonProperty("logIndex")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long logIndex() {
    return logIndex;
  }

  /**
   * Emitting contract of the finalizing event; set only when finalized by a blockchain event.
   *
   * @return the emitting contract address, or {@code null} if not finalized by a blockchain event
   */
  @JsonProperty("source")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress source() {
    return source;
  }

  /**
   * Detail of why the transaction reverted; non-empty only on failure.
   *
   * @return the failure message, or an empty string on success
   */
  @JsonProperty("failureMessage")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String failureMessage() {
    return failureMessage;
  }

  /**
   * Encoded revert data, if available.
   *
   * @return the encoded revert data, or {@code null} if unavailable
   */
  @JsonProperty("revertData")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes revertData() {
    return revertData;
  }

  /**
   * Address of a newly deployed contract; {@code null} when this transaction was an invoke.
   *
   * @return the deployed contract address, or {@code null} for an invoke
   */
  @JsonProperty("contractAddress")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress contractAddress() {
    return contractAddress;
  }

  /**
   * Starts an empty builder.
   *
   * @return a new builder
   */
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

    /**
     * Sets the transaction id this receipt belongs to.
     *
     * @param id the transaction id
     * @return this builder
     */
    public Builder id(UUID id) {
      this.id = id;
      return this;
    }

    /**
     * Sets the indexed timestamp.
     *
     * @param indexed the time the receipt was indexed
     * @return this builder
     */
    public Builder indexed(Timestamp indexed) {
      this.indexed = indexed;
      return this;
    }

    /**
     * Sets the local ordering sequence.
     *
     * @param sequence the local ordering sequence
     * @return this builder
     */
    public Builder sequence(long sequence) {
      this.sequence = sequence;
      return this;
    }

    /**
     * Sets the domain name.
     *
     * @param domain the domain name (private-transaction receipts only)
     * @return this builder
     */
    public Builder domain(String domain) {
      this.domain = domain;
      return this;
    }

    /**
     * Sets whether the transaction succeeded.
     *
     * @param success {@code true} if the transaction succeeded
     * @return this builder
     */
    public Builder success(boolean success) {
      this.success = success;
      return this;
    }

    /**
     * Sets the base-ledger transaction hash.
     *
     * @param transactionHash the on-chain transaction hash
     * @return this builder
     */
    public Builder transactionHash(Bytes32 transactionHash) {
      this.transactionHash = transactionHash;
      return this;
    }

    /**
     * Sets the block number.
     *
     * @param blockNumber the on-chain block number
     * @return this builder
     */
    public Builder blockNumber(Long blockNumber) {
      this.blockNumber = blockNumber;
      return this;
    }

    /**
     * Sets the index of the transaction within its block.
     *
     * @param transactionIndex the on-chain transaction index
     * @return this builder
     */
    public Builder transactionIndex(Long transactionIndex) {
      this.transactionIndex = transactionIndex;
      return this;
    }

    /**
     * Sets the log index of the finalizing event.
     *
     * @param logIndex the finalizing event's log index
     * @return this builder
     */
    public Builder logIndex(Long logIndex) {
      this.logIndex = logIndex;
      return this;
    }

    /**
     * Sets the emitting contract of the finalizing event.
     *
     * @param source the finalizing event's emitting contract address
     * @return this builder
     */
    public Builder source(EthAddress source) {
      this.source = source;
      return this;
    }

    /**
     * Sets the failure message.
     *
     * @param failureMessage detail of why the transaction reverted
     * @return this builder
     */
    public Builder failureMessage(String failureMessage) {
      this.failureMessage = failureMessage;
      return this;
    }

    /**
     * Sets the encoded revert data.
     *
     * @param revertData the encoded revert data
     * @return this builder
     */
    public Builder revertData(HexBytes revertData) {
      this.revertData = revertData;
      return this;
    }

    /**
     * Sets the deployed contract address.
     *
     * @param contractAddress the address of a newly deployed contract
     * @return this builder
     */
    public Builder contractAddress(EthAddress contractAddress) {
      this.contractAddress = contractAddress;
      return this;
    }

    /**
     * Builds the immutable {@link TransactionReceipt}.
     *
     * @return a new {@link TransactionReceipt} with the configured values
     */
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
