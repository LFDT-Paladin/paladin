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

/**
 * Query filter, sort, and pagination types for the Paladin Java SDK, mirroring {@code sdk/go/pkg/query}.
 *
 * <p>{@link org.lfdt.paladin.sdk.core.query.QueryJSON} is the serializable query document sent to a node's
 * {@code query}-style RPC methods, and {@link org.lfdt.paladin.sdk.core.query.QueryBuilder} is the fluent
 * builder that assembles one (mirroring Go's {@code QueryBuilder}). The filter operands are modelled by
 * {@link org.lfdt.paladin.sdk.core.query.Op} (field-only, for {@code null} checks),
 * {@link org.lfdt.paladin.sdk.core.query.OpSingleVal} (single-value operators), and
 * {@link org.lfdt.paladin.sdk.core.query.OpMultiVal} ({@code in}/{@code nin}), with
 * {@link org.lfdt.paladin.sdk.core.query.QueryModifier} carrying the {@code not}/{@code caseInsensitive}
 * flags.
 *
 * <p>All types are immutable and round-trip through any Jackson {@code ObjectMapper}, following the
 * {@code omitempty} conventions of the Go reference (the canonical short operator keys {@code eq}, {@code neq},
 * {@code lt}, {@code lte}, {@code gt}, {@code gte}, {@code like}, {@code in}, {@code nin}, {@code null}).
 */
package org.lfdt.paladin.sdk.core.query;
