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

// Immutable; mirrors pldapi.TransactionReceiptFull (a TransactionReceipt enriched with the states
// it
// touched, the domain receipt and any associated public transactions). The base receipt fields are
// re-declared and flattened here to match the flat JSON wire form, exactly as TransactionFull does
// over Transaction. The domainReceipt mirrors Go pldtypes.RawJSON and the public block mirrors Go
// []*PublicTx (not yet ported); both are surfaced as raw JSON.
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
      @JsonProperty("contractAddress") EthAddress contractAddress,
      @JsonProperty("states") TransactionStates states,
      @JsonProperty("domainReceipt") JsonNode domainReceipt,
      @JsonProperty("domainReceiptError") String domainReceiptError,
      @JsonProperty("public") List<JsonNode> publicTransactions) {
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

  @JsonProperty("states")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionStates states() {
    return states;
  }

  @JsonProperty("domainReceipt")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode domainReceipt() {
    return domainReceipt;
  }

  @JsonProperty("domainReceiptError")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domainReceiptError() {
    return domainReceiptError;
  }

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
