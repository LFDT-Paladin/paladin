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
package org.lfdt.paladin.sdk.core.types;

import com.fasterxml.jackson.core.JsonGenerator;
import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.core.JsonToken;
import com.fasterxml.jackson.databind.DeserializationContext;
import com.fasterxml.jackson.databind.JsonDeserializer;
import com.fasterxml.jackson.databind.JsonSerializer;
import com.fasterxml.jackson.databind.SerializerProvider;
import com.fasterxml.jackson.databind.annotation.JsonDeserialize;
import com.fasterxml.jackson.databind.annotation.JsonSerialize;
import java.io.IOException;
import java.time.Instant;
import java.time.OffsetDateTime;
import java.time.format.DateTimeFormatter;
import java.time.format.DateTimeParseException;

/**
 * A Unix timestamp with nanosecond resolution, mirroring {@code pldtypes.Timestamp}. Serializes to
 * JSON as an RFC 3339 / ISO-8601 UTC string with nanosecond precision; a zero timestamp serializes
 * to JSON {@code null} (and JSON {@code null} deserializes back to zero). Deserializes from an RFC
 * 3339 string, a numeric string, or a JSON integer; numeric inputs are interpreted as seconds,
 * milliseconds, or nanoseconds based on magnitude.
 */
@JsonSerialize(using = Timestamp.Serializer.class)
@JsonDeserialize(using = Timestamp.Deserializer.class)
public final class Timestamp {

  private static final long NANOS_PER_SECOND = 1_000_000_000L;

  /** The shared zero value (Unix epoch), used as the round-trip target for JSON {@code null}. */
  public static final Timestamp ZERO = new Timestamp(0L);

  private final long unixNano;

  private Timestamp(long unixNano) {
    this.unixNano = unixNano;
  }

  /** Creates a timestamp from a raw nanoseconds-since-epoch value. */
  public static Timestamp ofUnixNano(long unixNano) {
    return unixNano == 0L ? ZERO : new Timestamp(unixNano);
  }

  /** Creates a timestamp from an {@link Instant}. */
  public static Timestamp ofInstant(Instant instant) {
    return ofUnixNano(
        Math.addExact(
            Math.multiplyExact(instant.getEpochSecond(), NANOS_PER_SECOND), instant.getNano()));
  }

  /**
   * Creates a timestamp from a Unix value whose resolution is inferred from its magnitude: seconds,
   * milliseconds, or nanoseconds (matching {@code pldtypes.TimestampFromUnix}).
   */
  public static Timestamp fromUnix(long unixTime) {
    long t = unixTime;
    if (t < 10_000_000_000L) {
      t *= 1_000L; // seconds -> milliseconds
    }
    if (t < 1_000_000_000_000_000L) {
      t *= 1_000_000L; // milliseconds -> nanoseconds
    }
    return ofUnixNano(t);
  }

  /** Parses an RFC 3339 string or a numeric (seconds/millis/nanos) string. */
  public static Timestamp fromString(String s) {
    String t = s.trim();
    try {
      return ofInstant(Instant.parse(t));
    } catch (DateTimeParseException ignored) {
      // not an ISO instant with a 'Z' offset; fall through
    }
    try {
      return ofInstant(OffsetDateTime.parse(t).toInstant());
    } catch (DateTimeParseException ignored) {
      // not an offset date-time; try a bare unix number
    }
    try {
      return fromUnix(Long.parseLong(t));
    } catch (NumberFormatException e) {
      throw new IllegalArgumentException("invalid timestamp: \"" + s + "\"", e);
    }
  }

  /** Nanoseconds since the Unix epoch. */
  public long unixNano() {
    return unixNano;
  }

  public boolean isZero() {
    return unixNano == 0L;
  }

  /** Converts to an {@link Instant}. */
  public Instant toInstant() {
    return Instant.ofEpochSecond(
        Math.floorDiv(unixNano, NANOS_PER_SECOND), Math.floorMod(unixNano, NANOS_PER_SECOND));
  }

  /**
   * RFC 3339 / ISO-8601 UTC representation with nanosecond precision; empty for a zero timestamp.
   */
  public String toRfc3339() {
    return unixNano == 0L ? "" : DateTimeFormatter.ISO_INSTANT.format(toInstant());
  }

  @Override
  public String toString() {
    return toRfc3339();
  }

  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof Timestamp other && unixNano == other.unixNano;
  }

  @Override
  public int hashCode() {
    return Long.hashCode(unixNano);
  }

  static final class Serializer extends JsonSerializer<Timestamp> {
    @Override
    public void serialize(Timestamp v, JsonGenerator gen, SerializerProvider provider)
        throws IOException {
      if (v.unixNano == 0L) {
        gen.writeNull();
      } else {
        gen.writeString(v.toRfc3339());
      }
    }
  }

  static final class Deserializer extends JsonDeserializer<Timestamp> {
    @Override
    public Timestamp deserialize(JsonParser p, DeserializationContext ctx) throws IOException {
      JsonToken t = p.currentToken();
      if (t == JsonToken.VALUE_NUMBER_INT) {
        return fromUnix(p.getLongValue());
      }
      if (t != null && t.isScalarValue()) {
        String s = p.getValueAsString();
        return (s == null || s.isEmpty()) ? ZERO : fromString(s);
      }
      return (Timestamp) ctx.handleUnexpectedToken(Timestamp.class, p);
    }

    @Override
    public Timestamp getNullValue(DeserializationContext ctx) {
      return ZERO;
    }
  }
}
