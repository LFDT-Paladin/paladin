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
import com.fasterxml.jackson.annotation.JsonValue;

/**
 * Whether a transaction targets the base ledger directly or is masked through a Paladin domain,
 * mirroring {@code pldapi.TransactionType}. Serializes to its lower-case JSON token; parsing is
 * case-insensitive.
 */
public enum TransactionType {

  /** A private transaction that uses a Paladin domain to mask the on-chain data. */
  PRIVATE("private"),
  /** A public transaction that goes straight to a base ledger EVM smart contract. */
  PUBLIC("public");

  private final String jsonValue;

  TransactionType(final String jsonValue) {
    this.jsonValue = jsonValue;
  }

  /**
   * The JSON token for this transaction type.
   *
   * @return the lower-case JSON token
   */
  @JsonValue
  public String jsonValue() {
    return jsonValue;
  }

  /**
   * Resolves a transaction type from its JSON token, case-insensitively.
   *
   * @param s the JSON token to resolve
   * @return the matching transaction type
   * @throws IllegalArgumentException if {@code s} is null or not a known transaction type
   */
  @JsonCreator
  public static TransactionType fromJson(final String s) {
    if (s != null) {
      for (TransactionType t : values()) {
        if (t.jsonValue.equalsIgnoreCase(s)) {
          return t;
        }
      }
    }
    throw new IllegalArgumentException("unknown transaction type: \"" + s + "\"");
  }
}
