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
 * An in-memory record locking a state to a transaction while it is being assembled, endorsed, and
 * submitted. Immutable.
 */
@JsonPropertyOrder({"transaction", "type"})
public final class StateLock {

  private final UUID transaction;
  private final StateLockType type;

  @JsonCreator
  StateLock(
      @JsonProperty("transaction") final UUID transaction,
      @JsonProperty("type") final StateLockType type) {
    this.transaction = transaction;
    this.type = type;
  }

  /**
   * The transaction the state is locked to.
   *
   * @return the locking transaction id
   */
  @JsonProperty("transaction")
  public UUID transaction() {
    return transaction;
  }

  /**
   * The kind of lock.
   *
   * @return the lock type
   */
  @JsonProperty("type")
  public StateLockType type() {
    return type;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof StateLock other
        && Objects.equals(transaction, other.transaction)
        && type == other.type;
  }

  @Override
  public int hashCode() {
    return Objects.hash(transaction, type);
  }

  @Override
  public String toString() {
    return "StateLock{transaction=" + transaction + ", type=" + type + "}";
  }
}
