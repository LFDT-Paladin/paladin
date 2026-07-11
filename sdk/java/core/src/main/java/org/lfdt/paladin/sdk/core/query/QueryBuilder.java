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
 * <p>Each filter method appends to the corresponding operator list (it never removes existing ones) and
 * returns {@code this} for chaining. Values may be any object — they are converted to their JSON form via the
 * SDK-wide mapper, so SDK value types (addresses, hashes, hex integers) serialize through their own
 * representation. Terminate the chain with {@link #build()}.
 *
 * <p>Not thread-safe; build a query on a single thread.
 */
public final class QueryBuilder {

    /**
     * Plain mapper for converting filter values to JSON nodes. SDK value types are self-serializing, so no
     * configuration is needed; deliberately NOT the big-integer mapper, whose {@code BigIntegerNode}s would
     * not equal the {@code IntNode}/{@code LongNode}s produced when a query is parsed back with a plain mapper.
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

    QueryBuilder() {
    }

    /** Sets the maximum number of items to return. */
    public QueryBuilder limit(final int limit) {
        this.limit = limit;
        return this;
    }

    /** Appends one or more sort fields (each optionally suffixed with {@code " DESC"}/{@code " ASC"}). */
    public QueryBuilder sort(final String... fields) {
        for (String field : fields) {
            this.sort.add(field);
        }
        return this;
    }

    /** Adds an equality filter ({@code eq}). */
    public QueryBuilder equal(final String field, final Object value, final QueryModifier... modifiers) {
        eq.add(singleVal(field, value, modifiers));
        return this;
    }

    /** Adds a not-equal filter ({@code neq}). */
    public QueryBuilder notEqual(final String field, final Object value, final QueryModifier... modifiers) {
        neq.add(singleVal(field, value, modifiers));
        return this;
    }

    /** Adds a greater-than filter ({@code gt}). */
    public QueryBuilder greaterThan(final String field, final Object value) {
        gt.add(singleVal(field, value));
        return this;
    }

    /** Adds a greater-than-or-equal filter ({@code gte}). */
    public QueryBuilder greaterThanOrEqual(final String field, final Object value) {
        gte.add(singleVal(field, value));
        return this;
    }

    /** Adds a less-than filter ({@code lt}). */
    public QueryBuilder lessThan(final String field, final Object value) {
        lt.add(singleVal(field, value));
        return this;
    }

    /** Adds a less-than-or-equal filter ({@code lte}). */
    public QueryBuilder lessThanOrEqual(final String field, final Object value) {
        lte.add(singleVal(field, value));
        return this;
    }

    /** Adds a membership filter ({@code in}). */
    public QueryBuilder in(final String field, final List<?> values, final QueryModifier... modifiers) {
        in.add(multiVal(field, values, modifiers));
        return this;
    }

    /** Adds a not-in filter ({@code nin}). */
    public QueryBuilder notIn(final String field, final List<?> values, final QueryModifier... modifiers) {
        nin.add(multiVal(field, values, modifiers));
        return this;
    }

    /** Adds an is-null filter. */
    public QueryBuilder isNull(final String field) {
        isNull.add(new Op(field, false, false));
        return this;
    }

    /** Adds an is-not-null filter (a {@code null} operand carrying {@code not: true}). */
    public QueryBuilder isNotNull(final String field) {
        isNull.add(new Op(field, true, false));
        return this;
    }

    /** Adds a pattern-match filter ({@code like}). */
    public QueryBuilder like(final String field, final Object value) {
        like.add(singleVal(field, value));
        return this;
    }

    /** Adds a negated pattern-match filter (a {@code like} operand carrying {@code not: true}). */
    public QueryBuilder notLike(final String field, final Object value) {
        like.add(singleVal(field, value, QueryModifier.NOT));
        return this;
    }

    /**
     * Adds OR branches. Each child builder's filter statements become one branch; the children's
     * {@code limit}/{@code sort} are ignored (only the root query paginates), mirroring the Go builder.
     */
    public QueryBuilder or(final QueryBuilder... branches) {
        for (QueryBuilder branch : branches) {
            or.add(branch.buildStatements());
        }
        return this;
    }

    /** Builds the immutable {@link QueryJSON}. */
    public QueryJSON build() {
        return new QueryJSON(limit, sort, or, eq, neq, like, lt, lte, gt, gte, in, nin, isNull);
    }

    /** Builds a statement-only query (no limit/sort) for use as an OR branch. */
    private QueryJSON buildStatements() {
        return new QueryJSON(null, null, or, eq, neq, like, lt, lte, gt, gte, in, nin, isNull);
    }

    private static OpSingleVal singleVal(final String field, final Object value, final QueryModifier... modifiers) {
        final boolean[] flags = flags(modifiers);
        return new OpSingleVal(field, flags[0], flags[1], toNode(value));
    }

    private static OpMultiVal multiVal(final String field, final List<?> values, final QueryModifier... modifiers) {
        final boolean[] flags = flags(modifiers);
        final List<JsonNode> nodes = new ArrayList<>();
        if (values != null) {
            for (Object value : values) {
                nodes.add(toNode(value));
            }
        }
        return new OpMultiVal(field, flags[0], flags[1], nodes);
    }

    /** Resolves the modifier varargs into {@code [not, caseInsensitive]} flags. */
    private static boolean[] flags(final QueryModifier... modifiers) {
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
     * round-tripping through its serialized form. This keeps the node identical to one parsed from the same
     * JSON, so a built query equals its plain-mapper round trip regardless of the value's declared width.
     */
    private static JsonNode toNode(final Object value) {
        try {
            return VALUE_MAPPER.readTree(VALUE_MAPPER.writeValueAsString(value));
        } catch (final JsonProcessingException e) {
            throw new IllegalArgumentException("query value is not JSON-serializable: " + value, e);
        }
    }
}
