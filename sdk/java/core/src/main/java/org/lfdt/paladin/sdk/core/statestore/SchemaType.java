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
 * The kind of a state schema. Serializes to its lower-case JSON token; parsing is case-insensitive.
 */
public enum SchemaType {

  /** An ABI schema, defining indexed fields with the same semantics as event parameters. */
  ABI("abi");

  private final String jsonValue;

  SchemaType(final String jsonValue) {
    this.jsonValue = jsonValue;
  }

  /**
   * The JSON token for this schema type.
   *
   * @return the lower-case JSON token
   */
  @JsonValue
  public String jsonValue() {
    return jsonValue;
  }

  /**
   * Resolves a schema type from its JSON token, case-insensitively.
   *
   * @param typeString the JSON token to resolve
   * @return the matching schema type
   * @throws IllegalArgumentException if {@code typeString} is null or not a known schema type
   */
  @JsonCreator
  public static SchemaType fromJson(final String typeString) {
    for (SchemaType type : values()) {
      if (type.jsonValue.equalsIgnoreCase(typeString)) {
        return type;
      }
    }
    throw new IllegalArgumentException("unknown schema type: \"" + typeString + "\"");
  }
}
