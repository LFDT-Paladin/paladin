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

/**
 * How a state is locked to a transaction. Serializes to its lower-case JSON token; parsing is
 * case-insensitive.
 */
public enum StateLockType {

  /** An optimistic lock recording the creation of a state not yet confirmed on chain. */
  CREATE("create"),
  /** A lock recording that a transaction read the state. */
  READ("read"),
  /** A lock recording that a transaction spends the state. */
  SPEND("spend");

  private final String jsonValue;

  StateLockType(final String jsonValue) {
    this.jsonValue = jsonValue;
  }

  /**
   * The JSON token for this lock type.
   *
   * @return the lower-case JSON token
   */
  @JsonValue
  public String jsonValue() {
    return jsonValue;
  }

  /**
   * Resolves a lock type from its JSON token, case-insensitively.
   *
   * @param s the JSON token to resolve
   * @return the matching lock type
   * @throws IllegalArgumentException if {@code s} is null or not a known lock type
   */
  @JsonCreator
  public static StateLockType fromJson(final String s) {
    if (s != null) {
      for (StateLockType t : values()) {
        if (t.jsonValue.equalsIgnoreCase(s)) {
          return t;
        }
      }
    }
    throw new IllegalArgumentException("unknown state lock type: " + s);
  }
}
