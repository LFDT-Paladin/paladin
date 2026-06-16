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
 * An unsigned integer up to 256 bits, mirroring {@code pldtypes.HexUint256}. Serializes to JSON as
 * lower-case hex with a {@code 0x} prefix, padded to an even number of digits. Deserializes from either a
 * JSON string (hex with {@code 0x}, or decimal) or a JSON integer, without loss of precision.
 */
@JsonSerialize(using = HexUint256.Serializer.class)
@JsonDeserialize(using = HexUint256.Deserializer.class)
public final class HexUint256 {

    private final BigInteger value;

    private HexUint256(BigInteger value) {
        this.value = value;
    }

    public static HexUint256 of(BigInteger value) {
        if (value == null) {
            throw new IllegalArgumentException("value must not be null");
        }
        return new HexUint256(value);
    }

    public static HexUint256 of(long value) {
        return new HexUint256(BigInteger.valueOf(value));
    }

    /** Parses a hex ({@code 0x}-prefixed) or decimal string. */
    public static HexUint256 fromString(String s) {
        return new HexUint256(Hex.parseBigInteger(s));
    }

    public BigInteger bigIntegerValue() {
        return value;
    }

    /** Lower-case, even-length hex of the absolute value without a {@code 0x} prefix. */
    public String toHex() {
        String hex = value.abs().toString(16);
        return (hex.length() & 1) == 1 ? "0" + hex : hex;
    }

    /** Lower-case, even-length hex with a {@code 0x} prefix — the JSON representation. */
    public String to0xHex() {
        return "0x" + toHex();
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
        return o instanceof HexUint256 other && value.equals(other.value);
    }

    @Override
    public int hashCode() {
        return Objects.hashCode(value);
    }

    static final class Serializer extends JsonSerializer<HexUint256> {
        @Override
        public void serialize(HexUint256 v, JsonGenerator gen, SerializerProvider provider) throws IOException {
            gen.writeString(v.to0xHex());
        }
    }

    static final class Deserializer extends JsonDeserializer<HexUint256> {
        @Override
        public HexUint256 deserialize(JsonParser p, DeserializationContext ctx) throws IOException {
            JsonToken t = p.currentToken();
            if (t == JsonToken.VALUE_NUMBER_INT) {
                return new HexUint256(p.getBigIntegerValue());
            }
            if (t != null && t.isScalarValue()) {
                return fromString(p.getValueAsString());
            }
            return (HexUint256) ctx.handleUnexpectedToken(HexUint256.class, p);
        }
    }
}
