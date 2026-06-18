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

/**
 * Optional modifiers applied to a filter operand, mirroring the variadic {@code addOns} of Go's query
 * builder (e.g. {@code query.Not}, {@code query.CaseInsensitive}, {@code query.CaseSensitive}).
 *
 * <p>Passed to the value-bearing {@link QueryBuilder} methods ({@code equal}, {@code notEqual}, {@code in},
 * {@code notIn}) to set the corresponding {@code not}/{@code caseInsensitive} flags on the emitted operand.
 */
public enum QueryModifier {

    /** Negates the operand (sets {@code not: true}). */
    NOT,
    /** Makes the comparison case-insensitive (sets {@code caseInsensitive: true}). */
    CASE_INSENSITIVE,
    /** Forces a case-sensitive comparison (clears {@code caseInsensitive}); the default. */
    CASE_SENSITIVE
}
