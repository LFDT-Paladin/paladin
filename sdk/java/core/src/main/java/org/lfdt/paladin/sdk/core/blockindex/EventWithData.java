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
package org.lfdt.paladin.sdk.core.blockindex;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;

/**
 * An indexed event together with its ABI-decoded data, as returned by {@code
 * bidx_decodeTransactionEvents}.
 *
 * <p>Carries the same on-chain location fields as {@link IndexedEvent}, plus the emitting {@link
 * #address()}, the decoded {@link #data()}, and the {@link #soliditySignature()} of the ABI entry
 * used to decode it. Immutable.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({
  "blockNumber",
  "transactionIndex",
  "logIndex",
  "transactionHash",
  "signature",
  "transaction",
  "block",
  "soliditySignature",
  "address",
  "data"
})
public final class EventWithData {

  private final long blockNumber;
  private final long transactionIndex;
  private final long logIndex;
  private final Bytes32 transactionHash;
  private final Bytes32 signature;
  private final IndexedTransaction transaction;
  private final IndexedBlock block;
  private final String soliditySignature;
  private final EthAddress address;
  private final JsonNode data;

  @JsonCreator
  EventWithData(
      @JsonProperty("blockNumber") final long blockNumber,
      @JsonProperty("transactionIndex") final long transactionIndex,
      @JsonProperty("logIndex") final long logIndex,
      @JsonProperty("transactionHash") final Bytes32 transactionHash,
      @JsonProperty("signature") final Bytes32 signature,
      @JsonProperty("transaction") final IndexedTransaction transaction,
      @JsonProperty("block") final IndexedBlock block,
      @JsonProperty("soliditySignature") final String soliditySignature,
      @JsonProperty("address") final EthAddress address,
      @JsonProperty("data") final JsonNode data) {
    this.blockNumber = blockNumber;
    this.transactionIndex = transactionIndex;
    this.logIndex = logIndex;
    this.transactionHash = transactionHash;
    this.signature = signature;
    this.transaction = transaction;
    this.block = block;
    this.soliditySignature = soliditySignature;
    this.address = address;
    this.data = data;
  }

  /**
   * The number of the block that contains this event.
   *
   * @return the block number
   */
  @JsonProperty("blockNumber")
  public long blockNumber() {
    return blockNumber;
  }

  /**
   * The index of the emitting transaction within its block.
   *
   * @return the transaction index
   */
  @JsonProperty("transactionIndex")
  public long transactionIndex() {
    return transactionIndex;
  }

  /**
   * The index of this log within its transaction.
   *
   * @return the log index
   */
  @JsonProperty("logIndex")
  public long logIndex() {
    return logIndex;
  }

  /**
   * The hash of the transaction that emitted this event.
   *
   * @return the transaction hash
   */
  @JsonProperty("transactionHash")
  public Bytes32 transactionHash() {
    return transactionHash;
  }

  /**
   * The topic-0 event signature hash.
   *
   * @return the event signature
   */
  @JsonProperty("signature")
  public Bytes32 signature() {
    return signature;
  }

  /**
   * The transaction that emitted this event, if it was included in the response; otherwise {@code
   * null}.
   *
   * @return the emitting transaction, or {@code null}
   */
  @JsonProperty("transaction")
  public IndexedTransaction transaction() {
    return transaction;
  }

  /**
   * The block that contains this event, if it was included in the response; otherwise {@code null}.
   *
   * @return the containing block, or {@code null}
   */
  @JsonProperty("block")
  public IndexedBlock block() {
    return block;
  }

  /**
   * The full Solidity signature of the ABI entry used to decode the event, including variable
   * names.
   *
   * @return the Solidity signature
   */
  @JsonProperty("soliditySignature")
  public String soliditySignature() {
    return soliditySignature;
  }

  /**
   * The address of the contract that emitted the event.
   *
   * @return the emitting contract address
   */
  @JsonProperty("address")
  public EthAddress address() {
    return address;
  }

  /**
   * The decoded event data, in the requested output format.
   *
   * @return the decoded event data
   */
  @JsonProperty("data")
  public JsonNode data() {
    return data;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof EventWithData other
        && blockNumber == other.blockNumber
        && transactionIndex == other.transactionIndex
        && logIndex == other.logIndex
        && Objects.equals(transactionHash, other.transactionHash)
        && Objects.equals(signature, other.signature)
        && Objects.equals(transaction, other.transaction)
        && Objects.equals(block, other.block)
        && Objects.equals(soliditySignature, other.soliditySignature)
        && Objects.equals(address, other.address)
        && Objects.equals(data, other.data);
  }

  @Override
  public int hashCode() {
    return Objects.hash(
        blockNumber,
        transactionIndex,
        logIndex,
        transactionHash,
        signature,
        transaction,
        block,
        soliditySignature,
        address,
        data);
  }

  @Override
  public String toString() {
    return "EventWithData{blockNumber="
        + blockNumber
        + ", logIndex="
        + logIndex
        + ", soliditySignature="
        + soliditySignature
        + ", address="
        + address
        + "}";
  }
}
