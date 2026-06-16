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
import java.util.Objects;

/**
 * A signed integer up to 256 bits, mirroring {@code pldtypes.HexInt256}. Serializes to JSON as lower-case
 * hex with a {@code 0x} prefix, with a leading {@code -} for negative values. Deserializes from either a
 * JSON string (hex with {@code 0x}, or decimal) or a JSON integer, without loss of precision.
 */
@JsonSerialize(using = HexInt256.Serializer.class)
@JsonDeserialize(using = HexInt256.Deserializer.class)
public final class HexInt256 {

    private final BigInteger value;

    private HexInt256(BigInteger value) {
        this.value = value;
    }

    public static HexInt256 of(BigInteger value) {
        if (value == null) {
            throw new IllegalArgumentException("value must not be null");
        }
        return new HexInt256(value);
    }

    public static HexInt256 of(long value) {
        return new HexInt256(BigInteger.valueOf(value));
    }

    /** Parses a hex ({@code 0x}-prefixed, optionally signed) or decimal string. */
    public static HexInt256 fromString(String s) {
        return new HexInt256(Hex.parseBigInteger(s));
    }

    public BigInteger bigIntegerValue() {
        return value;
    }

    /** Lower-case hex of the absolute value without a {@code 0x} prefix (no sign). */
    public String toHex() {
        return value.abs().toString(16);
    }

    /** Signed, lower-case hex with a {@code 0x} prefix — the JSON representation. */
    public String to0xHex() {
        return (value.signum() < 0 ? "-0x" : "0x") + value.abs().toString(16);
    }

    @Override
    public String toString() {
        return to0xHex();
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) {
            return true;
        }
        return o instanceof HexInt256 other && value.equals(other.value);
    }

    @Override
    public int hashCode() {
        return Objects.hashCode(value);
    }

    static final class Serializer extends JsonSerializer<HexInt256> {
        @Override
        public void serialize(HexInt256 v, JsonGenerator gen, SerializerProvider provider) throws IOException {
            gen.writeString(v.to0xHex());
        }
    }

    static final class Deserializer extends JsonDeserializer<HexInt256> {
        @Override
        public HexInt256 deserialize(JsonParser p, DeserializationContext ctx) throws IOException {
            JsonToken t = p.currentToken();
            if (t == JsonToken.VALUE_NUMBER_INT) {
                return new HexInt256(p.getBigIntegerValue());
            }
            if (t != null && t.isScalarValue()) {
                return fromString(p.getValueAsString());
            }
            return (HexInt256) ctx.handleUnexpectedToken(HexInt256.class, p);
        }
    }
}
