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
import java.util.Objects;

/**
 * A field-only filter operand, mirroring {@code query.Op}. Used directly for {@code null}/not-null
 * checks and as the common shape (field + {@code not} + {@code caseInsensitive} flags) shared with
 * {@link OpSingleVal} and {@link OpMultiVal}.
 *
 * <p>Immutable and self-serializing: {@code field} is always emitted, while {@code not} and {@code
 * caseInsensitive} follow the Go {@code omitempty} convention and are omitted when false.
 */
@JsonPropertyOrder({"field", "not", "caseInsensitive"})
public final class Op {

  private final String field;
  private final boolean not;
  private final boolean caseInsensitive;

  /**
   * Creates a field-only operand.
   *
   * @param field the field the operand applies to
   * @param not whether the operand is negated
   * @param caseInsensitive whether the comparison is case-insensitive
   */
  @JsonCreator
  public Op(
      @JsonProperty("field") final String field,
      @JsonProperty("not") final boolean not,
      @JsonProperty("caseInsensitive") final boolean caseInsensitive) {
    this.field = field == null ? "" : field;
    this.not = not;
    this.caseInsensitive = caseInsensitive;
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

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof Op other
        && not == other.not
        && caseInsensitive == other.caseInsensitive
        && field.equals(other.field);
  }

  @Override
  public int hashCode() {
    return Objects.hash(field, not, caseInsensitive);
  }

  @Override
  public String toString() {
    return "Op{field="
        + field
        + (not ? ", not" : "")
        + (caseInsensitive ? ", caseInsensitive" : "")
        + "}";
  }
}
