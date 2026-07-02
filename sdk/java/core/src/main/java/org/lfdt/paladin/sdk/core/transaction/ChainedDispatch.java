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

// Immutable; mirrors pldapi.ChainedDispatch (the binding of a paladin transaction to a chained
// transaction it triggered). Returned by ptx_getChainedDispatch / ptx_queryChainedDispatches.
@JsonPropertyOrder({"id", "transactionID", "chainedTransactionID"})
public final class ChainedDispatch {

  private final String id;
  private final String transactionID;
  private final String chainedTransactionID;

  @JsonCreator
  ChainedDispatch(
      @JsonProperty("id") String id,
      @JsonProperty("transactionID") String transactionID,
      @JsonProperty("chainedTransactionID") String chainedTransactionID) {
    this.id = id;
    this.transactionID = transactionID;
    this.chainedTransactionID = chainedTransactionID;
  }

  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String id() {
    return id;
  }

  @JsonProperty("transactionID")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String transactionID() {
    return transactionID;
  }

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
