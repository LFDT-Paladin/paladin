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
import java.math.BigInteger;

/**
 * An unsigned 64-bit integer, mirroring {@code pldtypes.HexUint64}. Serializes to JSON as
 * lower-case hex with a {@code 0x} prefix. Deserializes from either a JSON string (hex with {@code
 * 0x}, or decimal) or a JSON integer. The full unsigned range is supported; the value is held in a
 * {@code long} with unsigned semantics.
 */
@JsonSerialize(using = HexUint64.Serializer.class)
@JsonDeserialize(using = HexUint64.Deserializer.class)
public final class HexUint64 {

  /** The largest value representable, {@code 2^64 - 1}. */
  private static final BigInteger MAX = BigInteger.ONE.shiftLeft(64).subtract(BigInteger.ONE);

  private final long value;

  private HexUint64(final long value) {
    this.value = value;
  }

  /**
   * Wraps a non-negative {@code long}. Use {@link #fromString(String)} for values above {@code
   * Long.MAX_VALUE}.
   *
   * @param value the non-negative value to wrap
   * @return a {@code HexUint64} holding {@code value}
   * @throws IllegalArgumentException if {@code value} is negative
   */
  public static HexUint64 of(final long value) {
    if (value < 0) {
      throw new IllegalArgumentException("value must not be negative: " + value);
    }
    return new HexUint64(value);
  }

  /**
   * Parses a hex ({@code 0x}-prefixed) or decimal string in the range {@code [0, 2^64 - 1]}.
   *
   * @param s the hex or decimal string to parse
   * @return the parsed {@code HexUint64}
   * @throws IllegalArgumentException if {@code s} is not valid or is outside the unsigned 64-bit
   *     range
   */
  public static HexUint64 fromString(final String s) {
    return fromBigInteger(Hex.parseBigInteger(s), s);
  }

  private static HexUint64 fromBigInteger(final BigInteger v, final String source) {
    if (v.signum() < 0 || v.compareTo(MAX) > 0) {
      throw new IllegalArgumentException("value out of uint64 range: " + source);
    }
    return new HexUint64(v.longValue());
  }

  /**
   * The value as a {@code long} with unsigned bit semantics (may be negative for values {@code >
   * Long.MAX_VALUE}).
   *
   * @return the raw {@code long} value with unsigned bit semantics
   */
  public long asUnsignedLong() {
    return value;
  }

  /**
   * The value as a non-negative {@link BigInteger}.
   *
   * @return the unsigned value as a {@code BigInteger}
   */
  public BigInteger bigIntegerValue() {
    return new BigInteger(Long.toUnsignedString(value));
  }

  /**
   * Lower-case hex without a {@code 0x} prefix.
   *
   * @return the value as lower-case hex characters
   */
  public String toHex() {
    return Long.toUnsignedString(value, 16);
  }

  /**
   * Lower-case hex with a {@code 0x} prefix — the JSON representation.
   *
   * @return the value as {@code 0x}-prefixed lower-case hex
   */
  public String to0xHex() {
    return "0x" + Long.toUnsignedString(value, 16);
  }

  @Override
  public String toString() {
    return to0xHex();
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof HexUint64 other && value == other.value;
  }

  @Override
  public int hashCode() {
    return Long.hashCode(value);
  }

  static final class Serializer extends JsonSerializer<HexUint64> {
    @Override
    public void serialize(
        final HexUint64 v, final JsonGenerator gen, final SerializerProvider provider)
        throws IOException {
      gen.writeString(v.to0xHex());
    }
  }

  static final class Deserializer extends JsonDeserializer<HexUint64> {
    @Override
    public HexUint64 deserialize(final JsonParser p, final DeserializationContext ctx)
        throws IOException {
      final JsonToken t = p.currentToken();
      if (t == JsonToken.VALUE_NUMBER_INT) {
        return fromBigInteger(p.getBigIntegerValue(), p.getText());
      }
      if (t != null && t.isScalarValue()) {
        return fromString(p.getValueAsString());
      }
      return (HexUint64) ctx.handleUnexpectedToken(HexUint64.class, p);
    }
  }
}
