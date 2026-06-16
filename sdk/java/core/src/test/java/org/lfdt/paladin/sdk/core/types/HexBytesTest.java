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

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

/** A plain ObjectMapper is used throughout to prove no consumer-side Jackson configuration is required. */
class HexBytesTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Test
    void parsesWithAndWithoutPrefixAndAnyCase() {
        byte[] expected = {(byte) 0xde, (byte) 0xad, (byte) 0xbe, (byte) 0xef};
        assertArrayEquals(expected, HexBytes.fromString("0xdeadbeef").toByteArray());
        assertArrayEquals(expected, HexBytes.fromString("deadbeef").toByteArray());
        assertArrayEquals(expected, HexBytes.fromString("0xDEADBEEF").toByteArray());
    }

    @Test
    void emptyStringDecodesToEmpty() {
        assertTrue(HexBytes.fromString("").isEmpty());
        assertTrue(HexBytes.fromString("0x").isEmpty());
        assertEquals("0x", HexBytes.fromString("0x").to0xHex());
    }

    @Test
    void serializesAsLowerCase0xPrefixedString() throws Exception {
        HexBytes value = HexBytes.fromString("0xDEADBEEF");
        assertEquals("\"0xdeadbeef\"", MAPPER.writeValueAsString(value));
    }

    @Test
    void roundTripsThroughJson() throws Exception {
        HexBytes original = HexBytes.wrap(new byte[] {0, 1, 2, 3, (byte) 0xff});
        String json = MAPPER.writeValueAsString(original);
        assertEquals(original, MAPPER.readValue(json, HexBytes.class));
    }

    @Test
    void rejectsInvalidHex() {
        assertThrows(IllegalArgumentException.class, () -> HexBytes.fromString("0xnothex"));
        assertThrows(IllegalArgumentException.class, () -> HexBytes.fromString("abc")); // odd length
    }

    @Test
    void equalsAndHashCodeByValue() {
        assertEquals(HexBytes.fromString("0x0102"), HexBytes.fromString("0102"));
        assertEquals(HexBytes.fromString("0x0102").hashCode(), HexBytes.fromString("0102").hashCode());
    }

    @Test
    void wrapCopiesInput() {
        byte[] backing = {1, 2, 3};
        HexBytes value = HexBytes.wrap(backing);
        backing[0] = 9;
        assertEquals("0x010203", value.to0xHex());
    }
}
