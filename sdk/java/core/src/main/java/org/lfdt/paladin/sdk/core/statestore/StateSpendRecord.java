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
package org.lfdt.paladin.sdk.core.statestore;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Objects;
import java.util.UUID;

/**
 * A join record written when a state is spent, linking the state to the transaction that spent it.
 * Immutable.
 */
@JsonPropertyOrder({"transaction"})
public final class StateSpendRecord {

  private final UUID transaction;

  @JsonCreator
  StateSpendRecord(@JsonProperty("transaction") final UUID transaction) {
    this.transaction = transaction;
  }

  /**
   * The transaction that spent the state.
   *
   * @return the spending transaction id
   */
  @JsonProperty("transaction")
  public UUID transaction() {
    return transaction;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof StateSpendRecord other && Objects.equals(transaction, other.transaction);
  }

  @Override
  public int hashCode() {
    return Objects.hash(transaction);
  }

  @Override
  public String toString() {
    return "StateSpendRecord{transaction=" + transaction + "}";
  }
}
