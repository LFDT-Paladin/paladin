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
import com.fasterxml.jackson.annotation.JsonValue;
import java.util.Objects;
import java.util.UUID;

/**
 * Selects which states a state-store query returns: one of the standard qualifiers, or the id of a
 * transaction whose in-flight domain context the query is evaluated against.
 *
 * <p>This is not a closed enumeration — alongside the standard qualifiers ({@link #AVAILABLE},
 * {@link #CONFIRMED}, {@link #UNCONFIRMED}, {@link #SPENT}, {@link #ALL}) a query may be qualified
 * by a transaction id via {@link #forTransaction(UUID)}. Serializes to a bare string. Immutable.
 */
public final class StateStatusQualifier {

  /** States that are confirmed, have their private data, and are not spent. */
  public static final StateStatusQualifier AVAILABLE = new StateStatusQualifier("available");

  /** States that have a confirm record on chain. */
  public static final StateStatusQualifier CONFIRMED = new StateStatusQualifier("confirmed");

  /** States that do not yet have a confirm record on chain. */
  public static final StateStatusQualifier UNCONFIRMED = new StateStatusQualifier("unconfirmed");

  /** States that have a spend record. */
  public static final StateStatusQualifier SPENT = new StateStatusQualifier("spent");

  /** All states, regardless of status. */
  public static final StateStatusQualifier ALL = new StateStatusQualifier("all");

  private final String value;

  private StateStatusQualifier(final String value) {
    this.value = value;
  }

  /**
   * Returns a qualifier that evaluates the query against the in-flight domain context of a
   * transaction.
   *
   * @param transactionId the transaction whose domain context to query against
   * @return a qualifier for that transaction
   */
  public static StateStatusQualifier forTransaction(final UUID transactionId) {
    return new StateStatusQualifier(
        Objects.requireNonNull(transactionId, "transactionId").toString());
  }

  /**
   * Resolves a qualifier from its string form: a standard qualifier (case-insensitively) or a
   * transaction id.
   *
   * @param s the string to resolve
   * @return the matching qualifier
   * @throws IllegalArgumentException if {@code s} is null or is neither a standard qualifier nor a
   *     valid transaction id
   */
  @JsonCreator
  public static StateStatusQualifier fromString(final String s) {
    if (s != null) {
      final String lower = s.toLowerCase(java.util.Locale.ROOT);
      switch (lower) {
        case "available":
          return AVAILABLE;
        case "confirmed":
          return CONFIRMED;
        case "unconfirmed":
          return UNCONFIRMED;
        case "spent":
          return SPENT;
        case "all":
          return ALL;
        default:
          return forTransaction(UUID.fromString(s));
      }
    }
    throw new IllegalArgumentException("state status qualifier must not be null");
  }

  /**
   * The string form of this qualifier.
   *
   * @return the qualifier value
   */
  @JsonValue
  public String value() {
    return value;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof StateStatusQualifier other && value.equals(other.value);
  }

  @Override
  public int hashCode() {
    return value.hashCode();
  }

  @Override
  public String toString() {
    return value;
  }
}
