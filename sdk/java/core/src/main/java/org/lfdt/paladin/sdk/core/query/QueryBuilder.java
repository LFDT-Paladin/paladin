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

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.ArrayList;
import java.util.List;

/**
 * Fluent builder for a {@link QueryJSON}, mirroring Go's {@code query.QueryBuilder}.
 *
 * <p>Each filter method appends to the corresponding operator list (it never removes existing ones)
 * and returns {@code this} for chaining. Values may be any object — they are converted to their
 * JSON form via the SDK-wide mapper, so SDK value types (addresses, hashes, hex integers) serialize
 * through their own representation. Terminate the chain with {@link #build()}.
 *
 * <p>Not thread-safe; build a query on a single thread.
 */
public final class QueryBuilder {

  /**
   * Plain mapper for converting filter values to JSON nodes. SDK value types are self-serializing,
   * so no configuration is needed; deliberately NOT the big-integer mapper, whose {@code
   * BigIntegerNode}s would not equal the {@code IntNode}/{@code LongNode}s produced when a query is
   * parsed back with a plain mapper.
   */
  private static final ObjectMapper VALUE_MAPPER = new ObjectMapper();

  private Integer limit;
  private final List<String> sort = new ArrayList<>();
  private final List<QueryJSON> or = new ArrayList<>();
  private final List<OpSingleVal> eq = new ArrayList<>();
  private final List<OpSingleVal> neq = new ArrayList<>();
  private final List<OpSingleVal> like = new ArrayList<>();
  private final List<OpSingleVal> lt = new ArrayList<>();
  private final List<OpSingleVal> lte = new ArrayList<>();
  private final List<OpSingleVal> gt = new ArrayList<>();
  private final List<OpSingleVal> gte = new ArrayList<>();
  private final List<OpMultiVal> in = new ArrayList<>();
  private final List<OpMultiVal> nin = new ArrayList<>();
  private final List<Op> isNull = new ArrayList<>();

  QueryBuilder() {}

  /**
   * Sets the maximum number of items to return.
   *
   * @param limit the maximum number of items to return
   * @return this builder
   */
  public QueryBuilder limit(int limit) {
    this.limit = limit;
    return this;
  }

  /**
   * Appends one or more sort fields (each optionally suffixed with {@code " DESC"}/{@code " ASC"}).
   *
   * @param fields the sort fields to append
   * @return this builder
   */
  public QueryBuilder sort(String... fields) {
    for (String field : fields) {
      this.sort.add(field);
    }
    return this;
  }

  /**
   * Adds an equality filter ({@code eq}).
   *
   * @param field the field to filter on
   * @param value the value to compare against
   * @param modifiers optional modifiers (e.g. {@link QueryModifier#NOT}, case sensitivity)
   * @return this builder
   */
  public QueryBuilder equal(String field, Object value, QueryModifier... modifiers) {
    eq.add(singleVal(field, value, modifiers));
    return this;
  }

  /**
   * Adds a not-equal filter ({@code neq}).
   *
   * @param field the field to filter on
   * @param value the value to compare against
   * @param modifiers optional modifiers (e.g. {@link QueryModifier#NOT}, case sensitivity)
   * @return this builder
   */
  public QueryBuilder notEqual(String field, Object value, QueryModifier... modifiers) {
    neq.add(singleVal(field, value, modifiers));
    return this;
  }

  /**
   * Adds a greater-than filter ({@code gt}).
   *
   * @param field the field to filter on
   * @param value the value to compare against
   * @return this builder
   */
  public QueryBuilder greaterThan(String field, Object value) {
    gt.add(singleVal(field, value));
    return this;
  }

  /**
   * Adds a greater-than-or-equal filter ({@code gte}).
   *
   * @param field the field to filter on
   * @param value the value to compare against
   * @return this builder
   */
  public QueryBuilder greaterThanOrEqual(String field, Object value) {
    gte.add(singleVal(field, value));
    return this;
  }

  /**
   * Adds a less-than filter ({@code lt}).
   *
   * @param field the field to filter on
   * @param value the value to compare against
   * @return this builder
   */
  public QueryBuilder lessThan(String field, Object value) {
    lt.add(singleVal(field, value));
    return this;
  }

  /**
   * Adds a less-than-or-equal filter ({@code lte}).
   *
   * @param field the field to filter on
   * @param value the value to compare against
   * @return this builder
   */
  public QueryBuilder lessThanOrEqual(String field, Object value) {
    lte.add(singleVal(field, value));
    return this;
  }

  /**
   * Adds a membership filter ({@code in}).
   *
   * @param field the field to filter on
   * @param values the set of values to match against
   * @param modifiers optional modifiers (e.g. {@link QueryModifier#NOT}, case sensitivity)
   * @return this builder
   */
  public QueryBuilder in(String field, List<?> values, QueryModifier... modifiers) {
    in.add(multiVal(field, values, modifiers));
    return this;
  }

  /**
   * Adds a not-in filter ({@code nin}).
   *
   * @param field the field to filter on
   * @param values the set of values to exclude
   * @param modifiers optional modifiers (e.g. {@link QueryModifier#NOT}, case sensitivity)
   * @return this builder
   */
  public QueryBuilder notIn(String field, List<?> values, QueryModifier... modifiers) {
    nin.add(multiVal(field, values, modifiers));
    return this;
  }

  /**
   * Adds an is-null filter.
   *
   * @param field the field that must be null
   * @return this builder
   */
  public QueryBuilder isNull(String field) {
    isNull.add(new Op(field, false, false));
    return this;
  }

  /**
   * Adds an is-not-null filter (a {@code null} operand carrying {@code not: true}).
   *
   * @param field the field that must not be null
   * @return this builder
   */
  public QueryBuilder isNotNull(String field) {
    isNull.add(new Op(field, true, false));
    return this;
  }

  /**
   * Adds a pattern-match filter ({@code like}).
   *
   * @param field the field to filter on
   * @param value the pattern to match against
   * @return this builder
   */
  public QueryBuilder like(String field, Object value) {
    like.add(singleVal(field, value));
    return this;
  }

  /**
   * Adds a negated pattern-match filter (a {@code like} operand carrying {@code not: true}).
   *
   * @param field the field to filter on
   * @param value the pattern that must not match
   * @return this builder
   */
  public QueryBuilder notLike(String field, Object value) {
    like.add(singleVal(field, value, QueryModifier.NOT));
    return this;
  }

  /**
   * Adds OR branches. Each child builder's filter statements become one branch; the children's
   * {@code limit}/{@code sort} are ignored (only the root query paginates), mirroring the Go
   * builder.
   *
   * @param branches the child builders whose statements become OR branches
   * @return this builder
   */
  public QueryBuilder or(QueryBuilder... branches) {
    for (QueryBuilder branch : branches) {
      or.add(branch.buildStatements());
    }
    return this;
  }

  /**
   * Builds the immutable {@link QueryJSON}.
   *
   * @return a new {@link QueryJSON} with the configured filters, sort, and limit
   */
  public QueryJSON build() {
    return new QueryJSON(limit, sort, or, eq, neq, like, lt, lte, gt, gte, in, nin, isNull);
  }

  /** Builds a statement-only query (no limit/sort) for use as an OR branch. */
  private QueryJSON buildStatements() {
    return new QueryJSON(null, null, or, eq, neq, like, lt, lte, gt, gte, in, nin, isNull);
  }

  private static OpSingleVal singleVal(String field, Object value, QueryModifier... modifiers) {
    boolean[] flags = flags(modifiers);
    return new OpSingleVal(field, flags[0], flags[1], toNode(value));
  }

  private static OpMultiVal multiVal(String field, List<?> values, QueryModifier... modifiers) {
    boolean[] flags = flags(modifiers);
    List<JsonNode> nodes = new ArrayList<>();
    if (values != null) {
      for (Object value : values) {
        nodes.add(toNode(value));
      }
    }
    return new OpMultiVal(field, flags[0], flags[1], nodes);
  }

  /** Resolves the modifier varargs into {@code [not, caseInsensitive]} flags. */
  private static boolean[] flags(QueryModifier... modifiers) {
    boolean not = false;
    boolean caseInsensitive = false;
    for (QueryModifier modifier : modifiers) {
      switch (modifier) {
        case NOT -> not = true;
        case CASE_INSENSITIVE -> caseInsensitive = true;
        case CASE_SENSITIVE -> caseInsensitive = false;
      }
    }
    return new boolean[] {not, caseInsensitive};
  }

  /**
   * Converts a filter value to a JSON node, normalized to the smallest-fitting numeric node type by
   * round-tripping through its serialized form. This keeps the node identical to one parsed from
   * the same JSON, so a built query equals its plain-mapper round trip regardless of the value's
   * declared width.
   */
  private static JsonNode toNode(Object value) {
    try {
      return VALUE_MAPPER.readTree(VALUE_MAPPER.writeValueAsString(value));
    } catch (JsonProcessingException e) {
      throw new IllegalArgumentException("query value is not JSON-serializable: " + value, e);
    }
  }
}
