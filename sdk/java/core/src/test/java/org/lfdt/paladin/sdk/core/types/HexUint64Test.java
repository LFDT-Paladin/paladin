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

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.math.BigInteger;
import org.junit.jupiter.api.Test;

class HexUint64Test {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final String MAX_HEX = "0xffffffffffffffff";
    private static final BigInteger MAX = BigInteger.TWO.pow(64).subtract(BigInteger.ONE);

    @Test
    void formatsAs0xHex() {
        assertEquals("0xff", HexUint64.of(255).to0xHex());
        assertEquals("0x0", HexUint64.of(0).to0xHex());
    }

    @Test
    void supportsFullUnsignedRange() {
        HexUint64 max = HexUint64.fromString(MAX_HEX);
        assertEquals(MAX, max.bigIntegerValue());
        assertEquals(MAX_HEX, max.to0xHex());
        assertEquals(-1L, max.asUnsignedLong()); // all-ones bit pattern
    }

    @Test
    void serializesAndRoundTrips() throws Exception {
        HexUint64 original = HexUint64.fromString(MAX_HEX);
        assertEquals("\"" + MAX_HEX + "\"", MAPPER.writeValueAsString(original));
        assertEquals(original, MAPPER.readValue(MAPPER.writeValueAsString(original), HexUint64.class));
    }

    @Test
    void deserializesFromStringAndNumber() throws Exception {
        assertEquals(HexUint64.of(255), MAPPER.readValue("\"0xff\"", HexUint64.class));
        assertEquals(HexUint64.of(255), MAPPER.readValue("\"255\"", HexUint64.class));
        assertEquals(HexUint64.of(255), MAPPER.readValue("255", HexUint64.class));
    }

    @Test
    void rejectsOutOfRange() {
        assertThrows(IllegalArgumentException.class, () -> HexUint64.fromString("-1"));
        assertThrows(IllegalArgumentException.class, () -> HexUint64.fromString("0x10000000000000000")); // 2^64
        assertThrows(IllegalArgumentException.class, () -> HexUint64.of(-1));
    }
}
