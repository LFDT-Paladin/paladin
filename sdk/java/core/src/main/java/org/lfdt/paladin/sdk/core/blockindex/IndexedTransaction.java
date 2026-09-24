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
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;

/** A transaction as recorded by the node's block indexer. Immutable. */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({
  "hash",
  "blockNumber",
  "transactionIndex",
  "from",
  "to",
  "nonce",
  "contractAddress",
  "result",
  "block"
})
public final class IndexedTransaction {

  private final Bytes32 hash;
  private final long blockNumber;
  private final long transactionIndex;
  private final EthAddress from;
  private final EthAddress to;
  private final long nonce;
  private final EthAddress contractAddress;
  private final EthTransactionResult result;
  private final IndexedBlock block;

  @JsonCreator
  IndexedTransaction(
      @JsonProperty("hash") final Bytes32 hash,
      @JsonProperty("blockNumber") final long blockNumber,
      @JsonProperty("transactionIndex") final long transactionIndex,
      @JsonProperty("from") final EthAddress from,
      @JsonProperty("to") final EthAddress to,
      @JsonProperty("nonce") final long nonce,
      @JsonProperty("contractAddress") final EthAddress contractAddress,
      @JsonProperty("result") final EthTransactionResult result,
      @JsonProperty("block") final IndexedBlock block) {
    this.hash = hash;
    this.blockNumber = blockNumber;
    this.transactionIndex = transactionIndex;
    this.from = from;
    this.to = to;
    this.nonce = nonce;
    this.contractAddress = contractAddress;
    this.result = result;
    this.block = block;
  }

  /**
   * The transaction hash.
   *
   * @return the transaction hash
   */
  @JsonProperty("hash")
  public Bytes32 hash() {
    return hash;
  }

  /**
   * The number of the block that contains this transaction.
   *
   * @return the block number
   */
  @JsonProperty("blockNumber")
  public long blockNumber() {
    return blockNumber;
  }

  /**
   * The index of this transaction within its block.
   *
   * @return the transaction index
   */
  @JsonProperty("transactionIndex")
  public long transactionIndex() {
    return transactionIndex;
  }

  /**
   * The sender address.
   *
   * @return the sender address, or {@code null} if not recorded
   */
  @JsonProperty("from")
  public EthAddress from() {
    return from;
  }

  /**
   * The recipient address, or {@code null} for a contract-creation transaction.
   *
   * @return the recipient address, or {@code null}
   */
  @JsonProperty("to")
  public EthAddress to() {
    return to;
  }

  /**
   * The sender's transaction nonce.
   *
   * @return the nonce
   */
  @JsonProperty("nonce")
  public long nonce() {
    return nonce;
  }

  /**
   * The address of the contract created by this transaction, or {@code null} if it created none.
   *
   * @return the created contract address, or {@code null}
   */
  @JsonProperty("contractAddress")
  public EthAddress contractAddress() {
    return contractAddress;
  }

  /**
   * The execution outcome, or {@code null} if not recorded.
   *
   * @return the transaction result, or {@code null}
   */
  @JsonProperty("result")
  public EthTransactionResult result() {
    return result;
  }

  /**
   * The block that contains this transaction, if it was included in the response; otherwise {@code
   * null}.
   *
   * @return the containing block, or {@code null}
   */
  @JsonProperty("block")
  public IndexedBlock block() {
    return block;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof IndexedTransaction other
        && blockNumber == other.blockNumber
        && transactionIndex == other.transactionIndex
        && nonce == other.nonce
        && Objects.equals(hash, other.hash)
        && Objects.equals(from, other.from)
        && Objects.equals(to, other.to)
        && Objects.equals(contractAddress, other.contractAddress)
        && result == other.result
        && Objects.equals(block, other.block);
  }

  @Override
  public int hashCode() {
    return Objects.hash(
        hash, blockNumber, transactionIndex, from, to, nonce, contractAddress, result, block);
  }

  @Override
  public String toString() {
    return "IndexedTransaction{hash="
        + hash
        + ", blockNumber="
        + blockNumber
        + ", transactionIndex="
        + transactionIndex
        + ", result="
        + result
        + "}";
  }
}
