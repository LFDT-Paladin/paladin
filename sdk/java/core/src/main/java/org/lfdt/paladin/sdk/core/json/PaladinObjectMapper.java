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
package org.lfdt.paladin.sdk.core.json;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;

/**
 * Factory for the SDK-wide Jackson {@link ObjectMapper}.
 *
 * <p>The Paladin Java SDK's primitive value types (in {@code org.lfdt.paladin.sdk.core.types}) are
 * self-serializing — they carry their own Jackson annotations and round-trip through any plain
 * {@code ObjectMapper}. This class layers on the cross-cutting defaults that higher-level modules
 * (client, domains, txbuilder) rely on so that request and response bodies behave consistently:
 *
 * <ul>
 *   <li><b>{@link JavaTimeModule}</b> registered, with dates written as ISO-8601 strings rather
 *       than numeric timestamps (mirrors the RFC 3339 convention used throughout Paladin's JSON).
 *   <li><b>Unknown properties ignored</b> on deserialization, so a newer node returning extra
 *       fields does not break an older client (forward compatibility).
 *   <li><b>Nulls omitted</b> on serialization ({@link JsonInclude.Include#NON_NULL}), keeping
 *       JSON-RPC request payloads minimal and matching Go's {@code omitempty} conventions.
 *   <li><b>{@code BigInteger}/{@code BigDecimal} for untyped numbers</b>, mirroring Go's {@code
 *       Decoder.UseNumber()} so that large integers bound to {@code Object}/{@code Map} targets do
 *       not lose precision through {@code double}.
 * </ul>
 *
 * <p>Use {@link #shared()} for the common case. It returns a single canonical instance, which is
 * thread-safe for reading and writing but must not be reconfigured — call {@link #create()} for an
 * independently customizable mapper instead.
 */
public final class PaladinObjectMapper {

  private static final ObjectMapper SHARED = create();

  private PaladinObjectMapper() {}

  /**
   * Returns the canonical shared mapper carrying the SDK-wide defaults. Safe for concurrent reads
   * and writes; do not mutate its configuration — use {@link #create()} if you need a variant.
   *
   * @return the shared, pre-configured {@link ObjectMapper}
   */
  public static ObjectMapper shared() {
    return SHARED;
  }

  /**
   * Builds a new, independently-configurable {@link ObjectMapper} with the SDK-wide defaults
   * applied. Callers that need to layer on additional modules or features should start here rather
   * than reconfiguring the {@link #shared()} instance.
   *
   * @return a new {@link ObjectMapper} with the SDK-wide defaults applied
   */
  public static ObjectMapper create() {
    return new ObjectMapper()
        .registerModule(new JavaTimeModule())
        .setSerializationInclusion(JsonInclude.Include.NON_NULL)
        .disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS)
        .disable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES)
        .enable(DeserializationFeature.USE_BIG_INTEGER_FOR_INTS)
        .enable(DeserializationFeature.USE_BIG_DECIMAL_FOR_FLOATS);
  }
}
