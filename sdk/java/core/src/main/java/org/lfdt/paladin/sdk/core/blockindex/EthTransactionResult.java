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
import com.fasterxml.jackson.annotation.JsonValue;

/**
 * The outcome of an indexed Ethereum transaction. Serializes to its lower-case JSON token; parsing
 * is case-insensitive.
 */
public enum EthTransactionResult {

  /** The transaction reverted or otherwise failed. */
  FAILURE("failure"),
  /** The transaction executed successfully. */
  SUCCESS("success");

  private final String jsonValue;

  EthTransactionResult(final String jsonValue) {
    this.jsonValue = jsonValue;
  }

  /**
   * The JSON token for this result.
   *
   * @return the lower-case JSON token
   */
  @JsonValue
  public String jsonValue() {
    return jsonValue;
  }

  /**
   * Resolves a result from its JSON token, case-insensitively.
   *
   * @param s the JSON token to resolve
   * @return the matching result
   * @throws IllegalArgumentException if {@code s} is null or not a known result
   */
  @JsonCreator
  public static EthTransactionResult fromJson(final String s) {
    if (s != null) {
      for (EthTransactionResult r : values()) {
        if (r.jsonValue.equalsIgnoreCase(s)) {
          return r;
        }
      }
    }
    throw new IllegalArgumentException("unknown transaction result: " + s);
  }
}
