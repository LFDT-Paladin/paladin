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

/**
 * The binding of a Paladin transaction to a chained transaction it triggered, mirroring {@code
 * pldapi.ChainedDispatch}. Immutable. Returned by {@code ptx_getChainedDispatch} and {@code
 * ptx_queryChainedDispatches}.
 */
@JsonPropertyOrder({"id", "transactionID", "chainedTransactionID"})
public final class ChainedDispatch {

  private final String id;
  private final String transactionID;
  private final String chainedTransactionID;

  @JsonCreator
  ChainedDispatch(
      @JsonProperty("id") final String id,
      @JsonProperty("transactionID") final String transactionID,
      @JsonProperty("chainedTransactionID") final String chainedTransactionID) {
    this.id = id;
    this.transactionID = transactionID;
    this.chainedTransactionID = chainedTransactionID;
  }

  /**
   * The id of the chained-dispatch record.
   *
   * @return the dispatch id, or an empty string when unset
   */
  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String id() {
    return id;
  }

  /**
   * The id of the Paladin transaction that triggered the chained transaction.
   *
   * @return the transaction id, or an empty string when unset
   */
  @JsonProperty("transactionID")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String transactionID() {
    return transactionID;
  }

  /**
   * The id of the chained transaction that was triggered.
   *
   * @return the chained transaction id, or an empty string when unset
   */
  @JsonProperty("chainedTransactionID")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String chainedTransactionID() {
    return chainedTransactionID;
  }

  @Override
  public String toString() {
    return "ChainedDispatch{id=" + id + ", transactionID=" + transactionID + "}";
  }
}
