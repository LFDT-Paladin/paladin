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
import java.util.Objects;

/**
 * A single-value filter operand, mirroring {@code query.OpSingleVal} (Go's {@code Op} embedded plus
 * a {@code value}). Backs the {@code eq}, {@code neq}, {@code like}, {@code lt}, {@code lte},
 * {@code gt}, and {@code gte} operators.
 *
 * <p>Immutable and self-serializing. The {@code value} is held as a {@link JsonNode} (mirroring
 * Go's {@code RawJSON}) so any JSON scalar/structure round-trips without precision loss.
 */
@JsonPropertyOrder({"field", "not", "caseInsensitive", "value"})
public final class OpSingleVal {

  private final String field;
  private final boolean not;
  private final boolean caseInsensitive;
  private final JsonNode value;

  /**
   * Creates a single-value operand.
   *
   * @param field the field the operand applies to
   * @param not whether the operand is negated
   * @param caseInsensitive whether the comparison is case-insensitive
   * @param value the value to compare against, as a raw JSON node
   */
  @JsonCreator
  public OpSingleVal(
      @JsonProperty("field") String field,
      @JsonProperty("not") boolean not,
      @JsonProperty("caseInsensitive") boolean caseInsensitive,
      @JsonProperty("value") JsonNode value) {
    this.field = field == null ? "" : field;
    this.not = not;
    this.caseInsensitive = caseInsensitive;
    this.value = value;
  }

  /**
   * The field the operand applies to.
   *
   * @return the field name
   */
  @JsonProperty("field")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String field() {
    return field;
  }

  /**
   * Whether the operand is negated.
   *
   * @return {@code true} if the operand is negated
   */
  @JsonProperty("not")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean not() {
    return not;
  }

  /**
   * Whether the comparison is case-insensitive.
   *
   * @return {@code true} if the comparison is case-insensitive
   */
  @JsonProperty("caseInsensitive")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean caseInsensitive() {
    return caseInsensitive;
  }

  /**
   * The value to compare against, as a raw JSON node.
   *
   * @return the comparison value, or {@code null} when unset
   */
  @JsonProperty("value")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode value() {
    return value;
  }

  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof OpSingleVal other
        && not == other.not
        && caseInsensitive == other.caseInsensitive
        && field.equals(other.field)
        && Objects.equals(value, other.value);
  }

  @Override
  public int hashCode() {
    return Objects.hash(field, not, caseInsensitive, value);
  }

  @Override
  public String toString() {
    return "OpSingleVal{field=" + field + ", value=" + value + (not ? ", not" : "") + "}";
  }
}
