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
 * The binding of a Paladin transaction to the public transaction it was dispatched as. Immutable.
 * Returned by {@code ptx_getDispatch} and {@code ptx_queryDispatches}.
 */
@JsonPropertyOrder({"id", "transactionID", "publicTransactionID"})
public final class Dispatch {

  private final String id;
  private final String transactionID;
  private final long publicTransactionID;

  @JsonCreator
  Dispatch(
      @JsonProperty("id") final String id,
      @JsonProperty("transactionID") final String transactionID,
      @JsonProperty("publicTransactionID") final long publicTransactionID) {
    this.id = id;
    this.transactionID = transactionID;
    this.publicTransactionID = publicTransactionID;
  }

  /**
   * The id of the dispatch record.
   *
   * @return the dispatch id, or an empty string when unset
   */
  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String id() {
    return id;
  }

  /**
   * The id of the Paladin transaction that was dispatched.
   *
   * @return the transaction id, or an empty string when unset
   */
  @JsonProperty("transactionID")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String transactionID() {
    return transactionID;
  }

  /**
   * The node-local id of the public transaction it was dispatched as.
   *
   * @return the public transaction id
   */
  @JsonProperty("publicTransactionID")
  public long publicTransactionID() {
    return publicTransactionID;
  }

  @Override
  public String toString() {
    return "Dispatch{id=" + id + ", transactionID=" + transactionID + "}";
  }
}
