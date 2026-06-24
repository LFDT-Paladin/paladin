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
package org.lfdt.paladin.sdk.core.query;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Objects;

/**
 * A multi-value filter operand, mirroring {@code query.OpMultiVal} (Go's {@code Op} embedded plus a
 * {@code values} array). Backs the {@code in} and {@code nin} (not-in) operators.
 *
 * <p>Immutable and self-serializing. Each value is held as a {@link JsonNode} (mirroring Go's
 * {@code RawJSON}); the {@code values} list is never null (empty when unset, in which case it is
 * omitted).
 */
@JsonPropertyOrder({"field", "not", "caseInsensitive", "values"})
public final class OpMultiVal {

  private final String field;
  private final boolean not;
  private final boolean caseInsensitive;
  private final List<JsonNode> values;

  @JsonCreator
  public OpMultiVal(
      @JsonProperty("field") String field,
      @JsonProperty("not") boolean not,
      @JsonProperty("caseInsensitive") boolean caseInsensitive,
      @JsonProperty("values") List<JsonNode> values) {
    this.field = field == null ? "" : field;
    this.not = not;
    this.caseInsensitive = caseInsensitive;
    this.values = values == null ? List.of() : List.copyOf(values);
  }

  /** The field the operand applies to. */
  @JsonProperty("field")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String field() {
    return field;
  }

  /** Whether the operand is negated. */
  @JsonProperty("not")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean not() {
    return not;
  }

  /** Whether the comparison is case-insensitive. */
  @JsonProperty("caseInsensitive")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean caseInsensitive() {
    return caseInsensitive;
  }

  /** The set of values to compare against, as raw JSON nodes. Never null. */
  @JsonProperty("values")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<JsonNode> values() {
    return values;
  }

  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof OpMultiVal other
        && not == other.not
        && caseInsensitive == other.caseInsensitive
        && field.equals(other.field)
        && values.equals(other.values);
  }

  @Override
  public int hashCode() {
    return Objects.hash(field, not, caseInsensitive, values);
  }

  @Override
  public String toString() {
    return "OpMultiVal{field=" + field + ", values=" + values.size() + (not ? ", not" : "") + "}";
  }
}
