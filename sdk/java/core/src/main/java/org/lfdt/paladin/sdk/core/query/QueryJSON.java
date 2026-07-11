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
import com.fasterxml.jackson.core.JsonProcessingException;
import java.util.List;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;

/**
 * A query document — filter statements plus sort and pagination — mirroring {@code query.QueryJSON} (which
 * embeds {@code Statements}). This is the body sent to a node's {@code query}-style RPC methods.
 *
 * <p>Immutable and self-serializing, following the Go {@code omitempty} convention: every field is omitted
 * when null/empty, so an empty query serializes to {@code {}}. All operand lists are never null (empty when
 * unset). Use {@link #builder()} to assemble one fluently.
 *
 * <p>Only the canonical short operator keys are modelled ({@code eq}, {@code neq}, {@code lt}, {@code lte},
 * {@code gt}, {@code gte}, {@code like}, {@code in}, {@code nin}, {@code null}) — these are exactly what the
 * Go builder emits. Nested {@code or} branches are themselves {@link QueryJSON} documents (their {@code limit}
 * and {@code sort} are ignored by the node, and dropped by {@link QueryBuilder#or}).
 */
@JsonPropertyOrder({
    "limit", "sort", "or", "eq", "neq", "like", "lt", "lte", "gt", "gte", "in", "nin", "null"
})
public final class QueryJSON {

    private final Integer limit;
    private final List<String> sort;
    private final List<QueryJSON> or;
    private final List<OpSingleVal> eq;
    private final List<OpSingleVal> neq;
    private final List<OpSingleVal> like;
    private final List<OpSingleVal> lt;
    private final List<OpSingleVal> lte;
    private final List<OpSingleVal> gt;
    private final List<OpSingleVal> gte;
    private final List<OpMultiVal> in;
    private final List<OpMultiVal> nin;
    private final List<Op> isNull;

    @JsonCreator
    QueryJSON(
            @JsonProperty("limit") final Integer limit,
            @JsonProperty("sort") final List<String> sort,
            @JsonProperty("or") final List<QueryJSON> or,
            @JsonProperty("eq") final List<OpSingleVal> eq,
            @JsonProperty("neq") final List<OpSingleVal> neq,
            @JsonProperty("like") final List<OpSingleVal> like,
            @JsonProperty("lt") final List<OpSingleVal> lt,
            @JsonProperty("lte") final List<OpSingleVal> lte,
            @JsonProperty("gt") final List<OpSingleVal> gt,
            @JsonProperty("gte") final List<OpSingleVal> gte,
            @JsonProperty("in") final List<OpMultiVal> in,
            @JsonProperty("nin") final List<OpMultiVal> nin,
            @JsonProperty("null") final List<Op> isNull) {
        this.limit = limit;
        this.sort = copyOf(sort);
        this.or = copyOf(or);
        this.eq = copyOf(eq);
        this.neq = copyOf(neq);
        this.like = copyOf(like);
        this.lt = copyOf(lt);
        this.lte = copyOf(lte);
        this.gt = copyOf(gt);
        this.gte = copyOf(gte);
        this.in = copyOf(in);
        this.nin = copyOf(nin);
        this.isNull = copyOf(isNull);
    }

    private static <T> List<T> copyOf(final List<T> list) {
        return list == null ? List.of() : List.copyOf(list);
    }

    /** Starts an empty query builder. */
    public static QueryBuilder builder() {
        return new QueryBuilder();
    }

    /** Maximum number of items to return, or {@code null} for the node default. */
    @JsonProperty("limit")
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public Integer limit() {
        return limit;
    }

    /** Sort fields, each optionally suffixed with {@code " DESC"}/{@code " ASC"}. Never null. */
    @JsonProperty("sort")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<String> sort() {
        return sort;
    }

    /** Alternative branches combined with logical OR. Never null. */
    @JsonProperty("or")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<QueryJSON> or() {
        return or;
    }

    /** Equality operands. Never null. */
    @JsonProperty("eq")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<OpSingleVal> eq() {
        return eq;
    }

    /** Not-equal operands. Never null. */
    @JsonProperty("neq")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<OpSingleVal> neq() {
        return neq;
    }

    /** Like (pattern match) operands. Never null. */
    @JsonProperty("like")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<OpSingleVal> like() {
        return like;
    }

    /** Less-than operands. Never null. */
    @JsonProperty("lt")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<OpSingleVal> lt() {
        return lt;
    }

    /** Less-than-or-equal operands. Never null. */
    @JsonProperty("lte")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<OpSingleVal> lte() {
        return lte;
    }

    /** Greater-than operands. Never null. */
    @JsonProperty("gt")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<OpSingleVal> gt() {
        return gt;
    }

    /** Greater-than-or-equal operands. Never null. */
    @JsonProperty("gte")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<OpSingleVal> gte() {
        return gte;
    }

    /** In (membership) operands. Never null. */
    @JsonProperty("in")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<OpMultiVal> in() {
        return in;
    }

    /** Not-in operands. Never null. */
    @JsonProperty("nin")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<OpMultiVal> nin() {
        return nin;
    }

    /** Is-null / is-not-null operands (the latter carry {@code not: true}). Never null. */
    @JsonProperty("null")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<Op> isNull() {
        return isNull;
    }

    /** Serializes this query to its JSON string form using the SDK-wide mapper. */
    public String toJson() {
        try {
            return PaladinObjectMapper.shared().writeValueAsString(this);
        } catch (final JsonProcessingException e) {
            throw new IllegalStateException("failed to serialize query", e);
        }
    }

    @Override
    public boolean equals(final Object o) {
        if (this == o) {
            return true;
        }
        return o instanceof QueryJSON other
                && Objects.equals(limit, other.limit)
                && sort.equals(other.sort)
                && or.equals(other.or)
                && eq.equals(other.eq)
                && neq.equals(other.neq)
                && like.equals(other.like)
                && lt.equals(other.lt)
                && lte.equals(other.lte)
                && gt.equals(other.gt)
                && gte.equals(other.gte)
                && in.equals(other.in)
                && nin.equals(other.nin)
                && isNull.equals(other.isNull);
    }

    @Override
    public int hashCode() {
        return Objects.hash(limit, sort, or, eq, neq, like, lt, lte, gt, gte, in, nin, isNull);
    }

    @Override
    public String toString() {
        return toJson();
    }
}
