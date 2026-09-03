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
package org.lfdt.paladin.sdk.core.registry;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonValue;

/**
 * Selects whether a registry query returns active entries, inactive entries, or both. Serializes to
 * its lower-case JSON token; parsing is case-insensitive.
 */
public enum ActiveFilter {

  /** Return only entries that are currently active. */
  ACTIVE("active"),
  /** Return only entries that have been deactivated. */
  INACTIVE("inactive"),
  /** Return both active and inactive entries. */
  ANY("any");

  private final String jsonValue;

  ActiveFilter(final String jsonValue) {
    this.jsonValue = jsonValue;
  }

  /**
   * The JSON token for this filter.
   *
   * @return the lower-case JSON token
   */
  @JsonValue
  public String jsonValue() {
    return jsonValue;
  }

  /**
   * Resolves a filter from its JSON token, case-insensitively.
   *
   * @param filterString the JSON token to resolve
   * @return the matching filter
   * @throws IllegalArgumentException if {@code filterString} is null or not a known filter
   */
  @JsonCreator
  public static ActiveFilter fromJson(final String filterString) {
    for (ActiveFilter filter : values()) {
      if (filter.jsonValue.equalsIgnoreCase(filterString)) {
        return filter;
      }
    }
    throw new IllegalArgumentException("unknown active filter: " + filterString);
  }
}
