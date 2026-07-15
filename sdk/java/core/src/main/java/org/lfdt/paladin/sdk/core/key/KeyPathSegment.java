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
package org.lfdt.paladin.sdk.core.key;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Objects;

/**
 * One named, indexed segment of the hierarchical path that resolves to a key. Immutable; mirrors
 * {@code pldapi.KeyPathSegment}.
 */
@JsonPropertyOrder({"name", "index"})
public final class KeyPathSegment {

  private final String name;
  private final long index;

  @JsonCreator
  KeyPathSegment(@JsonProperty("name") final String name, @JsonProperty("index") final long index) {
    this.name = name;
    this.index = index;
  }

  /**
   * The segment name.
   *
   * @return the segment name
   */
  @JsonProperty("name")
  public String name() {
    return name;
  }

  /**
   * The zero-based index allocated to this segment within its parent.
   *
   * @return the segment index
   */
  @JsonProperty("index")
  public long index() {
    return index;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof KeyPathSegment other
        && index == other.index
        && Objects.equals(name, other.name);
  }

  @Override
  public int hashCode() {
    return Objects.hash(name, index);
  }

  @Override
  public String toString() {
    return "KeyPathSegment{name=" + name + ", index=" + index + "}";
  }
}
